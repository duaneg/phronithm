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
	prompt     string   // as sent (skill prefix applied)
	argv       []string // podman's
	sessionID  string
	name       string // the container's
	transcript string
}

// spawn runs one attempt, a podman run, and waits for it. A guard timeout or
// a repeated signal cancels it: SIGTERM to its process group, which --init
// and podman's signal proxy carry into the container, and SIGKILL to the
// leader after KillGrace. The container's processes run under conmon, not
// the client, so the SIGKILL leaves them running: after a cancel, or any
// non-zero exit (a client killed mid-cleanup can leave its container), the
// container is removed with podman rm -f before spawn returns, and only then
// does anything read the clone. A failed removal is the attempt's rmErr.
// The group gets SIGKILL last, once the leader is reaped, which reaches a
// client survivor such as a TERM-ignoring grandchild. While a member lives
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

	cmd := exec.CommandContext(actx, rt.cfg.PodmanBin, sp.argv...)
	cmd.Env = rt.podmanEnv()
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

	if err := cmd.Start(); err != nil {
		if actx.Err() != nil { // cancelled before it started: not a spawn failure
			a.cause = context.Cause(actx)
			return a
		}
		a.spawnErr = err
		return a
	}
	pid := cmd.Process.Pid
	rt.event("spawn", map[string]any{"pid": pid, "session": sp.sessionID, "container": sp.name})
	werr := cmd.Wait()
	lw.flush()
	if lw.fileErr != nil {
		rt.Log(fmt.Sprintf("transcript %s: %v", sp.transcript, lw.fileErr))
	}
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 0 || cancelledRunning.Load() {
		a.rmErr = rt.removeContainer(sp.name)
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
