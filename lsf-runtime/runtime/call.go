package lsfrt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// buildArgv is a headless call's arguments. The prompt goes on stdin, so it
// is bounded by neither MAX_ARG_STRLEN nor a variadic flag swallowing it.
func buildArgv(o Opts, role Role, pluginDir, sessionID string) []string {
	a := []string{"-p", "--output-format", "stream-json", "--verbose", "--session-id", sessionID}
	if o.Model != "" {
		a = append(a, "--model", o.Model)
	}
	if o.Effort != "" {
		a = append(a, "--effort", o.Effort)
	}
	a = append(a, "--strict-mcp-config", "--permission-mode", role.PermissionMode, "--tools", role.Tools)
	if len(role.Allow) > 0 {
		a = append(append(a, "--allowedTools"), role.Allow...)
	}
	if len(role.Deny) > 0 {
		a = append(a, "--disallowedTools")
		for _, d := range role.Deny {
			a = append(a, strings.ReplaceAll(d, pluginToken, pluginDir))
		}
	}
	return append(a, "--json-schema", string(o.Schema))
}

// withSkill is the prompt as sent: a skill's slash command puts the skill
// body into the prompt before the model runs.
func withSkill(skill, prompt string) string {
	if skill == "" {
		return prompt
	}
	return "/" + skill + " " + prompt
}

// spawnSpec is one attempt's inputs.
type spawnSpec struct {
	prompt     string // as sent (skill prefix applied)
	argv       []string
	sessionID  string
	cwd        string
	transcript string
}

// spawn runs one attempt and waits for it. A guard timeout or a repeated
// signal cancels it: SIGTERM to its process group, SIGKILL to the leader
// after KillGrace, then SIGKILL to the group once the leader is reaped, which
// reaches survivors such as a TERM-ignoring grandchild. While a member lives
// the pgid cannot be reused; if none does, the pid is free and a new group
// leader could take it between Wait and the kill. That window is accepted:
// pids are allocated in sequence, so reuse within it needs a wrapped pid space.
func (rt *Runtime) spawn(ctx context.Context, sp spawnSpec) *attempt {
	a := &attempt{}
	actx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	if g := rt.cfg.GuardTimeout.D(); g > 0 {
		t := time.AfterFunc(g, func() { cancel(errGuard) })
		defer t.Stop()
	}
	go func() {
		select {
		case <-rt.killCh:
			cancel(errKilled)
		case <-actx.Done():
		}
	}()

	out, err := os.Create(sp.transcript)
	if err != nil {
		a.spawnErr = fmt.Errorf("transcript: %w", err)
		return a
	}
	defer out.Close()
	lw := &lineWriter{file: out, acc: &a.acc}
	stderr := &tailBuffer{max: 2048}

	cmd := exec.CommandContext(actx, rt.cfg.ClaudeBin, sp.argv...)
	cmd.Dir = sp.cwd
	cmd.Stdin = strings.NewReader(sp.prompt)
	cmd.Stdout = lw
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var cancelledRunning atomic.Bool // set when the context ends while the process runs
	cmd.Cancel = func() error {
		cancelledRunning.Store(true)
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = rt.cfg.KillGrace.D()

	rt.addChild(sp.sessionID)
	defer rt.removeChild(sp.sessionID)
	if err := cmd.Start(); err != nil {
		if actx.Err() != nil { // cancelled before it started: not a spawn failure
			a.cause = context.Cause(actx)
			return a
		}
		a.spawnErr = err
		return a
	}
	pid := cmd.Process.Pid
	rt.setChildPid(sp.sessionID, pid)
	rt.event("spawn", map[string]any{"pid": pid, "session": sp.sessionID, "cwd": sp.cwd})
	werr := cmd.Wait()
	lw.flush()
	if lw.fileErr != nil {
		rt.Log(fmt.Sprintf("transcript %s: %v", sp.transcript, lw.fileErr))
	}
	if cancelledRunning.Load() {
		a.cause = context.Cause(actx)
		if err := syscall.Kill(-pid, syscall.SIGKILL); err == nil {
			rt.event("group-killed", map[string]any{"pid": pid, "session": sp.sessionID})
		}
	}
	if cmd.ProcessState != nil {
		a.exitCode = cmd.ProcessState.ExitCode()
	} else if werr != nil {
		a.exitCode = -1
	}
	a.stderr = stderr.String()
	return a
}

// lineWriter tees the stream to the transcript and feeds complete lines to
// the accumulator. exec's copier calls Write from one goroutine.
// A file error stops the tee, not the parse: returning it would stop exec's
// copier and leave the child blocked on a full pipe.
type lineWriter struct {
	file    *os.File
	fileErr error
	acc     *streamAcc
	buf     []byte
}

// maxLine bounds a buffered line; a longer one is dropped as a bad line.
const maxLine = 64 << 20

func (w *lineWriter) Write(p []byte) (int, error) {
	if w.fileErr == nil {
		if _, err := w.file.Write(p); err != nil {
			w.fileErr = err
		}
	}
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.acc.add(w.buf[:i])
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) > maxLine {
		w.acc.BadLines++
		w.buf = nil
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		w.acc.add(w.buf)
		w.buf = nil
	}
}

// tailBuffer keeps the last max bytes written.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	b   []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > t.max {
		t.b = t.b[len(t.b)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}
