package lsfrt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

	gateTailBytes  = 4096
	gateSummaryMax = 300
	gateHomeSuffix = ".home"
)

// gateScript runs in a gate container: it marks the command as started in
// the gate home, then runs the command from the environment, so the
// container's arguments stay fixed.
const gateScript = `: > "$HOME/.lsf-started-$LSF_GATE_SEQ" && eval "$LSF_GATE_COMMAND"`

// RunGates runs a project's declared gates on a clean detached checkout of
// commit at <stateDir>/gates/<key> (a leftover directory there is removed
// first): setup, then each gate in order, each in its own container
// (docs/design/lsf-runtime.md, Permissions → Declared gates) with stdin
// /dev/null and its output in <stateDir>/gates/logs. The checkout and a
// gate home beside it, <key>.home, are bound at their host paths and
// shared by setup and the gates. setup has the egress proxy when hosts are
// pinned and no network otherwise; gate commands have no network. A failed
// setup makes every gate not-run. The checkout and home are removed
// afterwards, unless a container could not be removed. A stop or
// cancellation removes the running container and is returned; no runs are
// returned with it. A container that fails before its command starts
// stops the run with runtime-error. A checkout failure is an error. Runs
// are bounded by gateConcurrency, and are not journaled here. The checkout
// and log names derive from label, which must be unique in the launch.
//
// Every process a command leaves behind ends with its container: a service
// the gates need is started by the gate command itself. No host git runs
// in the checkout once a container has run there; only its removal.
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
	left := false // a container may still run in the checkout
	defer func() {
		if left {
			rt.Log(fmt.Sprintf("%s: gate checkout %s kept: a container in it was not removed; the next launch reaps it", label, dir))
			return
		}
		rt.gateRemove(ctx, label, dir)
	}()
	rt.event("gates-start", map[string]any{"label": label, "commit": commit, "gates": len(gates)})
	t0 := time.Now()
	logBase := filepath.Join(rt.cfg.StateDir, "gates", "logs", fmt.Sprintf("%s-%s-%d", key, rt.launchID, rt.gateSeq.Add(1)))

	runs := make([]GateRun, len(gates))
	notRun := ""
	if setup != "" {
		r, err := rt.gateProcess(ctx, dir, logBase+"-setup.log", setup, rt.cfg.GateTimeout.D(), true)
		if err != nil {
			left = errors.As(err, new(*containerLeftError))
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
		r, err := rt.gateProcess(ctx, dir, log, g.Command, timeout, false)
		if err != nil {
			left = errors.As(err, new(*containerLeftError))
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
	home := dir + gateHomeSuffix
	for _, d := range []string{dir, home} {
		if err := removeGateTree(d); err != nil {
			return fmt.Errorf("%s: stale gate checkout %s not removable: %w", label, d, err)
		}
	}
	if err := rt.withWorktreeLock(ctx); err != nil {
		return err
	}
	defer func() { <-rt.wtSem }()
	gctx, cancel := rt.gitContext(ctx)
	defer cancel()
	rt.pruneGateWorktrees(gctx)
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
	switch {
	case exitStatus(err) == 1 || err == nil && !fullObjectID(sha):
		return fmt.Errorf("%s: gate checkout of %s: it does not resolve to a commit", label, commit)
	case err != nil:
		return fmt.Errorf("%s: gate checkout of %s: rev-parse: %w", label, commit, err)
	}
	run := func(in string, args ...string) error {
		if out, err := cloneOut(gctx, in, args...); err != nil {
			sub := args[0]
			if sub == "-c" {
				sub = args[2]
			}
			return fmt.Errorf("%s: gate checkout of %s: git %s: %w: %s", label, commit, sub, err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	format, err := gitStdout(gctx, rt.cfg.Repo, "rev-parse", "--show-object-format")
	if err != nil {
		return fmt.Errorf("%s: gate checkout of %s: object format: %w", label, commit, err)
	}
	if err := run(filepath.Dir(dir), "init", "-q", "--object-format="+strings.TrimSpace(string(format)), "--", tmp); err != nil {
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
	if err := os.Mkdir(home, 0o700); err != nil {
		return fmt.Errorf("%s: gate home: %w", label, err)
	}
	return os.Rename(tmp, dir)
}

// fullObjectID reports whether s is a full SHA-1 or SHA-256 object ID.
func fullObjectID(s string) bool {
	return (len(s) == 40 || len(s) == 64) && strings.Trim(s, "0123456789abcdef") == ""
}

// gateRemove removes the checkout and its home. A failure is logged: the
// results stand, and the next run at dir, or the next launch, removes them.
func (rt *Runtime) gateRemove(ctx context.Context, label, dir string) {
	for _, d := range []string{dir, dir + gateHomeSuffix} {
		if err := removeGateTree(d); err != nil {
			rt.Log(fmt.Sprintf("%s: gate checkout %s not removed: %v", label, d, err))
		}
	}
	// Removal runs after a stop too, so it waits on the lock without
	// yielding to one; ctx without cancellation keeps git running.
	rt.wtSem <- struct{}{}
	defer func() { <-rt.wtSem }()
	gctx, cancel := rt.gitContext(context.WithoutCancel(ctx))
	defer cancel()
	rt.pruneGateWorktrees(gctx)
}

// removeGateTree removes a tree a gate container wrote, making each
// directory user-writable first (Go writes its module cache read-only).
// Symlinks are not followed. No git runs in it.
func removeGateTree(dir string) error {
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			if fi, err := d.Info(); err == nil && fi.Mode().Perm()&0o700 != 0o700 {
				os.Chmod(p, fi.Mode().Perm()|0o700)
			}
		}
		return nil
	})
	return os.RemoveAll(dir)
}

// pruneGateWorktrees prunes the main repo's stale registrations when it
// lists a worktree under gates/ (a gate checkout made as a worktree before
// plain clones). The caller holds wtSem.
func (rt *Runtime) pruneGateWorktrees(ctx context.Context) {
	gates := filepath.Join(rt.cfg.StateDir, "gates")
	if resolved, rerr := filepath.EvalSymlinks(gates); rerr == nil {
		gates = resolved // git lists resolved paths
	}
	list, lerr := gitStdout(ctx, rt.cfg.Repo, "worktree", "list", "--porcelain")
	if lerr == nil && strings.Contains(string(list), "\nworktree "+gates+string(filepath.Separator)) {
		gitRun(ctx, rt.cfg.Repo, "worktree", "prune")
	}
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

// containerLeftError is a gate container that could not be removed: it may
// still run in the checkout, which is therefore kept. It unwraps to the
// stop in force.
type containerLeftError struct {
	name string
	err  error
	stop *StopError
}

func (e *containerLeftError) Error() string {
	return fmt.Sprintf("gate container %s not removed: %v", e.name, e.err)
}

func (e *containerLeftError) Unwrap() error { return e.stop }

// gateProcess runs one command in its own container on the checkout at
// dir (setup is spliced to the egress proxy when one runs). Its status is
// green (exit 0), red (another exit of a command that started), or not-run
// for a timeout or a podman client killed by a signal; a stop or
// cancellation is returned as the error. A cancel sends SIGTERM to the
// podman client's group, SIGKILL after KillGrace; after a cancel or a
// non-zero exit the container is removed with podman rm -f, and the group
// then gets SIGKILL. A container that exits non-zero before the command
// started (no started marker in the gate home: an image, mount, pasta or
// name fault, or an unwritable home) sets the runtime-error stop: it is
// not the gate's verdict, which would be replayed. A removal that fails
// sets runtime-error too and returns a containerLeftError. A file that
// cannot be created for the log makes the command not-run.
func (rt *Runtime) gateProcess(ctx context.Context, dir, logPath, command string, timeout time.Duration, setup bool) (GateRun, error) {
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

	seq := strconv.FormatInt(rt.gateSeq.Add(1), 10)
	home := dir + gateHomeSuffix
	marker := filepath.Join(home, ".lsf-started-"+seq)
	name := "lsf-" + rt.stateHash[:8] + "-gate-" + rt.launchID + "-" + seq
	cmd := exec.CommandContext(pctx, rt.cfg.PodmanBin, rt.containerArgv(containerSpec{
		kind: kindGate, name: name, dir: dir, home: home, egress: setup,
		env: []string{"LSF_GATE_COMMAND", "LSF_GATE_SEQ=" + seq},
		cmd: []string{"/bin/sh", "-c", gateScript},
	})...)
	// The command reaches the container by -e LSF_GATE_COMMAND, from
	// podman's environment.
	cmd.Env = append(gitenv.Scrubbed(), "LSF_GATE_COMMAND="+command)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var cancelled atomic.Bool // the context ended while the command ran
	cmd.Cancel = func() error {
		cancelled.Store(true)
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = rt.cfg.KillGrace.D()

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
	}
	pid := cmd.Process.Pid
	rt.event("gate-spawn", map[string]any{"pid": pid, "container": name, "setup": setup})
	cmd.Wait()
	var rmErr error
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 0 || cancelled.Load() {
		rmErr = rt.removeContainer(name)
	}
	syscall.Kill(-pid, syscall.SIGKILL)
	r.Secs = time.Since(t0).Seconds()
	_, merr := os.Lstat(marker)
	started := merr == nil
	os.Remove(marker)
	if rmErr != nil {
		msg := fmt.Sprintf("gate container %s not removed: %v", name, rmErr)
		rt.Log(msg)
		return r, &containerLeftError{name: name, err: rmErr, stop: rt.setStop(StopRuntimeError, msg)}
	}
	// A command reaped before a timeout or stop reached it keeps its result
	// (one that exits in the instant before it is reaped may still count as
	// stopped). A podman client killed by a signal is not a verdict; a
	// command killed in the container is the container's exit 128+N, red.
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
	case cmd.ProcessState.ExitCode() != 0 && !started:
		msg := fmt.Sprintf("gate container %s failed before its command started (exit %d): %s", name, cmd.ProcessState.ExitCode(), lastLine(fileTail(logPath, gateTailBytes)))
		rt.Log(msg)
		return r, rt.setStop(StopRuntimeError, msg)
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
