package lsfrt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/internal/gitenv"
)

// GateSpec is one declared gate. Timeout 0 is the config's gateTimeout.
type GateSpec struct {
	Name    string
	Command string
	Timeout time.Duration
}

// GateRun is one gate's result. Status is green (exit 0), red (any other
// exit) or not-run (timed out, or setup failed). Summary is the last
// non-blank output line, or why the gate did not run; Tail is the last 4 KB
// of output for a gate that is not green. Log is the full output's path.
type GateRun struct {
	Name     string  `json:"name"`
	Command  string  `json:"command"`
	Status   string  `json:"status"`
	ExitCode int     `json:"exitCode"`
	Summary  string  `json:"summary"`
	Tail     string  `json:"tail,omitempty"`
	Log      string  `json:"log"`
	Secs     float64 `json:"secs"`
}

const (
	GateGreen  = "green"
	GateRed    = "red"
	GateNotRun = "not-run"

	gateMarkerPrefix = "lsf-gate:"
	gateTailBytes    = 4096
	gateSummaryMax   = 300
)

// gateScript runs the command from the environment, so the shell's
// arguments are fixed and carry the marker as $0. Two commands keep the
// shell from exec-ing the gate command, which would drop the marker from
// the process's arguments.
const gateScript = `eval "$LSF_GATE_COMMAND"; exit $?`

// RunGates runs a project's declared gates on a clean detached checkout of
// commit at <stateDir>/gates/<key> (a leftover directory there is removed
// first): setup, then each gate in order, each through sh in its own process
// group with stdin /dev/null and its output in <stateDir>/gates/logs. A
// failed setup makes every gate not-run. The checkout is removed afterwards.
// A stop or cancellation kills the running group and is returned; no runs
// are returned with it. A checkout failure is an error. Runs are bounded
// by gateConcurrency, and are not journaled here. The checkout and log
// names derive from label, which must be unique in the launch.
//
// Every process a command leaves behind is killed when it ends, setup's
// included: a service the gates need is started by the gate command
// itself. A command that begins with `exec` replaces the marked shell, so a
// launch killed during it leaves an orphan the next launch cannot find.
// Commands run with the runtime's environment, credentials included: the
// declaration is trusted as the plan document is.
func (rt *Runtime) RunGates(ctx context.Context, label, commit, setup string, gates []GateSpec) ([]GateRun, error) {
	if err := rt.boundary(ctx); err != nil {
		return nil, err
	}
	if rt.cfg.DryRun {
		rt.Log("dry run: gates " + label + " not run")
		return nil, rt.setStop(StopDryRun, label)
	}
	select {
	case rt.gateSem <- struct{}{}:
	case <-rt.stopCh:
		return nil, rt.stopErr()
	case <-ctx.Done():
		return nil, &StopError{Reason: StopCancelled}
	}
	defer func() { <-rt.gateSem }()

	key := journalKey(label)
	dir := filepath.Join(rt.cfg.StateDir, "gates", key)
	if err := rt.gateCheckout(ctx, label, dir, commit); err != nil {
		return nil, err
	}
	defer rt.gateRemove(ctx, label, dir)
	rt.event("gates-start", map[string]any{"label": label, "commit": commit, "gates": len(gates)})
	t0 := time.Now()
	logBase := filepath.Join(rt.cfg.StateDir, "gates", "logs", fmt.Sprintf("%s-%s-%d", key, rt.launchID, rt.gateSeq.Add(1)))

	runs := make([]GateRun, len(gates))
	notRun := ""
	if setup != "" {
		r, err := rt.gateProcess(ctx, dir, logBase+"-setup.log", setup, rt.cfg.GateTimeout.D())
		if err != nil {
			return nil, err
		}
		switch r.Status {
		case GateRed:
			notRun = fmt.Sprintf("setup failed (exit %d): %s", r.ExitCode, r.Summary)
		case GateNotRun:
			notRun = "setup " + r.Summary
		}
	}
	for i, g := range gates {
		slug := unsafeChars.ReplaceAllString(g.Name, "_")
		if len(slug) > 40 { // the file name stays under NAME_MAX
			slug = slug[:40]
		}
		log := fmt.Sprintf("%s-%d-%s.log", logBase, i+1, slug)
		if notRun != "" {
			runs[i] = GateRun{Name: g.Name, Command: g.Command, Status: GateNotRun, ExitCode: -1, Summary: notRun, Log: log}
			continue
		}
		timeout := g.Timeout
		if timeout <= 0 {
			timeout = rt.cfg.GateTimeout.D()
		}
		r, err := rt.gateProcess(ctx, dir, log, g.Command, timeout)
		if err != nil {
			return nil, err
		}
		r.Name, r.Command = g.Name, g.Command
		runs[i] = r
	}
	statuses := make([]string, len(runs))
	for i, r := range runs {
		statuses[i] = r.Name + "=" + r.Status
	}
	rt.event("gates", map[string]any{"label": label, "commit": commit, "secs": time.Since(t0).Seconds(), "statuses": statuses})
	return runs, nil
}

// gateCheckout makes a clean detached checkout of commit at dir: a plain
// clone holding commit's history alone, made with the clone helpers (no
// global or system config) at dir.partial and renamed into place. The
// commit is resolved in the main repo and fetched by its full object ID,
// so one reachable only from a refs/lsf transfer ref, or from no ref, is
// gated too.
func (rt *Runtime) gateCheckout(ctx context.Context, label, dir, commit string) (err error) {
	if strings.HasPrefix(commit, "-") || commit == "" {
		return fmt.Errorf("%s: %q is not a commit", label, commit)
	}
	if err := rt.withWorktreeLock(ctx); err != nil {
		return err
	}
	defer func() { <-rt.wtSem }()
	gctx, cancel := rt.gitContext(ctx)
	defer cancel()
	rt.removeGateDir(gctx, dir)
	tmp := dir + partialSuffix
	defer func() {
		if err == nil {
			return
		}
		os.RemoveAll(tmp)
		if s := rt.stopped(); s != nil {
			err = s
		} else if ctx.Err() != nil {
			err = &StopError{Reason: StopCancelled}
		}
	}()
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	b, err := gitStdout(gctx, rt.cfg.Repo, "rev-parse", "--verify", "-q", "--end-of-options", commit+"^{commit}")
	sha := strings.TrimSpace(string(b))
	if err != nil || !fullObjectID(sha) {
		return fmt.Errorf("%s: gate checkout of %s: it does not resolve to a commit", label, commit)
	}
	run := func(in string, args ...string) error {
		if out, err := cloneOut(gctx, in, args...); err != nil {
			return fmt.Errorf("%s: gate checkout of %s: git %s: %w: %s", label, commit, args[0], err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if err := run(filepath.Dir(dir), "init", "-q", "--", tmp); err != nil {
		return err
	}
	// The line-ending config and attributes shape the work tree; no hook
	// runs, the hooks path naming dir, which does not yet exist.
	if err := rt.copyRepoSetup(gctx, tmp, dir); err != nil {
		return fmt.Errorf("%s: gate checkout of %s: %w", label, commit, err)
	}
	if err := run(tmp, "fetch", "--no-tags", "-q", "--end-of-options", rt.cfg.Repo, sha); err != nil {
		return err
	}
	if err := run(tmp, "rev-parse", "--verify", "-q", "--end-of-options", sha+"^{commit}"); err != nil {
		return err
	}
	if err := run(tmp, "-c", "advice.detachedHead=false", "checkout", "-q", "--detach", sha, "--"); err != nil {
		return err
	}
	return os.Rename(tmp, dir)
}

// fullObjectID reports whether s is a full SHA-1 or SHA-256 object ID.
func fullObjectID(s string) bool {
	return (len(s) == 40 || len(s) == 64) && strings.Trim(s, "0123456789abcdef") == ""
}

// gateRemove removes the checkout. A failure is logged: the results stand,
// and the next run at dir removes it first.
func (rt *Runtime) gateRemove(ctx context.Context, label, dir string) {
	// Removal runs after a stop too, so it waits on the lock without
	// yielding to one; ctx without cancellation keeps git running.
	rt.wtSem <- struct{}{}
	defer func() { <-rt.wtSem }()
	gctx, cancel := rt.gitContext(context.WithoutCancel(ctx))
	defer cancel()
	if err := rt.removeGateDir(gctx, dir); err != nil {
		rt.Log(fmt.Sprintf("%s: gate checkout %s not removed: %v", label, dir, err))
	}
}

// removeGateDir removes a gate checkout at dir and prunes the main repo's
// stale worktree registrations, which a gate checkout made as a worktree
// before plain clones may have left. The caller holds wtSem.
func (rt *Runtime) removeGateDir(ctx context.Context, dir string) error {
	err := os.RemoveAll(dir)
	gitRun(ctx, rt.cfg.Repo, "worktree", "prune")
	return err
}

// withWorktreeLock takes wtSem, yielding to a stop or cancellation.
func (rt *Runtime) withWorktreeLock(ctx context.Context) error {
	select {
	case rt.wtSem <- struct{}{}:
	case <-rt.stopCh:
		return rt.stopErr()
	case <-ctx.Done():
		return &StopError{Reason: StopCancelled}
	}
	if err := rt.boundary(ctx); err != nil {
		<-rt.wtSem
		return err
	}
	return nil
}

var errGateTimeout = errors.New("gate timeout")

// gateProcess runs one command in dir. Its status is green, red, or not-run
// for a timeout; a stop or cancellation is returned as the error. The group
// is killed as spawn kills a call's: SIGTERM, KillGrace, SIGKILL to the
// leader, then SIGKILL to the group, which also ends background processes a
// command that finished left behind. The pid-reuse window spawn accepts for
// a cancelled call is accepted here for every command: a reuse needs the
// pid space to wrap between Wait and the kill. A file that cannot be
// created for the log makes the command not-run.
func (rt *Runtime) gateProcess(ctx context.Context, dir, logPath, command string, timeout time.Duration) (GateRun, error) {
	r := GateRun{Status: GateNotRun, ExitCode: -1, Log: logPath}
	out, err := os.Create(logPath)
	if err != nil {
		r.Summary = "no log: " + err.Error()
		return r, nil
	}
	defer out.Close()
	pctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	t := time.AfterFunc(timeout, func() { cancel(errGateTimeout) })
	defer t.Stop()
	go func() {
		select {
		case <-rt.stopCh:
			cancel(rt.stopErr())
		case <-rt.killCh:
			cancel(errKilled)
		case <-pctx.Done():
		}
	}()

	marker := fmt.Sprintf("%s%s-%d", gateMarkerPrefix, rt.launchID, rt.gateSeq.Add(1))
	cmd := exec.CommandContext(pctx, "sh", "-c", gateScript, marker)
	cmd.Dir = dir
	cmd.Env = append(gitenv.Scrubbed(), "LSF_GATE_COMMAND="+command)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var cancelled atomic.Bool // the context ended while the command ran
	cmd.Cancel = func() error {
		cancelled.Store(true)
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = rt.cfg.KillGrace.D()

	rt.addChild(marker)
	defer rt.removeChild(marker)
	t0 := time.Now()
	if err := cmd.Start(); err != nil {
		if pctx.Err() == nil {
			r.Summary = "did not start: " + err.Error()
			return r, nil
		}
		if s := rt.stopped(); s != nil {
			return r, s
		}
		return r, &StopError{Reason: StopCancelled}
	} else {
		pid := cmd.Process.Pid
		rt.setChildPid(marker, pid)
		cmd.Wait()
		syscall.Kill(-pid, syscall.SIGKILL)
	}
	r.Secs = time.Since(t0).Seconds()
	// A command reaped before a timeout or stop reached it keeps its result
	// (one that exits in the instant before it is reaped may still count as
	// stopped). A shell killed by a signal is not a verdict; a command the
	// shell runs that is killed is the shell's exit 128+N, red.
	ws, _ := cmd.ProcessState.Sys().(syscall.WaitStatus)
	switch cause := context.Cause(pctx); {
	case cancelled.Load() && errors.Is(cause, errGateTimeout):
		r.Summary = "timed out after " + timeout.String()
	case cancelled.Load():
		if s := rt.stopped(); s != nil {
			return r, s
		}
		return r, &StopError{Reason: StopCancelled}
	case ws.Signaled():
		r.Summary = "killed by signal " + ws.Signal().String()
	default:
		r.ExitCode = cmd.ProcessState.ExitCode()
		r.Status = GateRed
		if r.ExitCode == 0 {
			r.Status = GateGreen
		}
	}
	tail := fileTail(logPath, gateTailBytes)
	if r.Summary == "" {
		r.Summary = lastLine(tail)
	}
	if r.Status != GateGreen {
		r.Tail = tail
	}
	return r, nil
}

// fileTail is the last n bytes of a file, "" when unreadable.
func fileTail(path string, n int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > n {
		f.Seek(-n, io.SeekEnd)
	}
	b, _ := io.ReadAll(f)
	return strings.ToValidUTF8(string(b), "�")
}

// lastLine is s's last non-blank line, bounded.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, " \t\r\n"), "\n")
	l := strings.TrimSpace(lines[len(lines)-1])
	if len(l) > gateSummaryMax {
		l = strings.ToValidUTF8(l[:gateSummaryMax], "") + "…"
	}
	return l
}
