package lsfrt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Runtime is one launch's host runtime: the orchestrator's agent, parallel,
// log and args, plus journaled code steps.
type Runtime struct {
	cfg      Config
	launchID string // names this launch's transcripts
	started  time.Time

	logMu   sync.Mutex
	logFile *os.File
	telMu   sync.Mutex
	telFile *os.File

	childMu  sync.Mutex
	children map[string]int // session id → pid (0 before Start)

	sem chan struct{} // concurrency slots

	mu         sync.Mutex
	stop       *StopError    // sticky; the first stop wins
	stopCh     chan struct{} // closed when stop is set
	killCh     chan struct{} // closed on a repeated signal
	killed     bool
	calls      int     // spawns this launch
	cost       float64 // summed total_cost_usd this launch
	limitUntil time.Time

	wtMu sync.Mutex // serialises git worktree operations

	seenMu sync.Mutex
	seen   map[string]string // journal key → input hash, this launch
}

// Run is a launch. It takes the state-dir lock, reaps orphans, installs
// signal handlers, writes status.json, runs body and writes summary.json.
// The returned error is for setup failures only; how body ended is the
// Summary's stop_reason.
func Run(ctx context.Context, cfg Config, body func(ctx context.Context, rt *Runtime) (json.RawMessage, error)) (Summary, error) {
	if err := cfg.prepare(); err != nil {
		return Summary{}, fmt.Errorf("lsf-runtime: config: %w", err)
	}
	for _, d := range []string{"", "journal", "calls", "wt"} {
		if err := os.MkdirAll(filepath.Join(cfg.StateDir, d), 0o755); err != nil {
			return Summary{}, fmt.Errorf("lsf-runtime: state dir: %w", err)
		}
	}
	lock, err := os.OpenFile(filepath.Join(cfg.StateDir, "lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return Summary{}, fmt.Errorf("lsf-runtime: state dir: %w", err)
	}
	defer lock.Close() // releases the flock
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return Summary{}, fmt.Errorf("lsf-runtime: state dir %s is in use by another launch: %w", cfg.StateDir, err)
	}
	rt := &Runtime{
		cfg:      cfg,
		started:  time.Now(),
		children: map[string]int{},
		seen:     map[string]string{},
		sem:      make(chan struct{}, cfg.Concurrency),
		stopCh:   make(chan struct{}),
		killCh:   make(chan struct{}),
	}
	rt.launchID = fmt.Sprintf("%s-%d", rt.started.UTC().Format("20060102T150405"), os.Getpid())
	open := func(name string) (*os.File, error) {
		return os.OpenFile(filepath.Join(cfg.StateDir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	}
	if rt.logFile, err = open("log.txt"); err != nil {
		return Summary{}, fmt.Errorf("lsf-runtime: %w", err)
	}
	defer rt.logFile.Close()
	if rt.telFile, err = open("telemetry.jsonl"); err != nil {
		return Summary{}, fmt.Errorf("lsf-runtime: %w", err)
	}
	defer rt.telFile.Close()

	stopSignals := rt.watchSignals()
	defer stopSignals()
	rt.reapOrphans()

	rt.Log(fmt.Sprintf("launch %s: state dir %s", rt.launchID, cfg.StateDir))
	rt.event("launch", map[string]any{"launch": rt.launchID, "pid": os.Getpid(), "dryRun": cfg.DryRun})
	rt.writeStatus("running")

	outcome, bodyErr, panicked := runBody(ctx, rt, body)
	s := rt.summarise(ctx, outcome, bodyErr, panicked)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil { // an outcome that is not valid JSON
		s.Error = strings.TrimSpace(s.Error + "; outcome dropped: " + err.Error())
		s.Outcome = nil
		b, _ = json.MarshalIndent(s, "", "  ")
	}
	if err := writeFileAtomic(filepath.Join(cfg.StateDir, "summary.json"), append(b, '\n')); err != nil {
		rt.Log("summary.json: " + err.Error())
	}
	rt.writeStatus(string(s.StopReason))
	rt.event("end", map[string]any{"stop_reason": s.StopReason, "calls": s.Calls, "cost": s.CostUSD})
	rt.Log(fmt.Sprintf("launch %s ended: %s", rt.launchID, s.StopReason))
	return s, nil
}

func runBody(ctx context.Context, rt *Runtime, body func(context.Context, *Runtime) (json.RawMessage, error)) (out json.RawMessage, err error, panicked any) {
	defer func() {
		if p := recover(); p != nil {
			panicked = p
			rt.Log(fmt.Sprintf("body panicked: %v\n%s", p, debug.Stack()))
		}
	}()
	out, err = body(ctx, rt)
	return out, err, nil
}

// summarise applies the stop_reason precedence (design §2.2).
func (rt *Runtime) summarise(ctx context.Context, outcome json.RawMessage, bodyErr error, panicked any) Summary {
	rt.mu.Lock()
	s := Summary{Started: rt.started, Finished: time.Now(), Calls: rt.calls, CostUSD: rt.cost, Outcome: outcome}
	stop := rt.stop
	rt.mu.Unlock()
	var bodyStop *StopError
	switch {
	case stop != nil:
		s.StopReason, s.Detail = stop.Reason, stop.Detail
	case errors.As(bodyErr, &bodyStop):
		s.StopReason, s.Detail = bodyStop.Reason, bodyStop.Detail
	case ctx.Err() != nil:
		s.StopReason = StopCancelled
	case panicked != nil:
		s.StopReason, s.Error = StopCrashed, fmt.Sprintf("panic: %v", panicked)
	case bodyErr != nil:
		s.StopReason, s.Error = StopCrashed, bodyErr.Error()
	default:
		s.StopReason = StopReturned
	}
	if s.Error == "" && bodyErr != nil && s.StopReason != StopReturned {
		s.Error = bodyErr.Error()
	}
	return s
}

// setStop records the sticky stop; the first one wins. It returns the stop
// in force.
func (rt *Runtime) setStop(reason StopReason, detail string) *StopError {
	rt.mu.Lock()
	s, set := rt.setStopLocked(reason, detail)
	rt.mu.Unlock()
	if set {
		rt.stopEvent(s)
	}
	return s
}

// setStopLocked sets the stop under rt.mu; set reports a new stop, whose
// event the caller emits with stopEvent after unlocking.
func (rt *Runtime) setStopLocked(reason StopReason, detail string) (s *StopError, set bool) {
	if rt.stop == nil {
		rt.stop = &StopError{Reason: reason, Detail: detail}
		close(rt.stopCh)
		set = true
	}
	return rt.stop, set
}

func (rt *Runtime) stopEvent(s *StopError) {
	rt.event("stop", map[string]any{"reason": s.Reason, "detail": s.Detail})
	rt.Log("stop: " + s.Error())
}

// stopErr is the sticky stop as an error: nil, not a typed nil, when none.
func (rt *Runtime) stopErr() error {
	if s := rt.stopped(); s != nil {
		return s
	}
	return nil
}

// stopped returns the sticky stop, or nil.
func (rt *Runtime) stopped() *StopError {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.stop
}

// Log appends a timestamped line to log.txt.
func (rt *Runtime) Log(msg string) {
	rt.logMu.Lock()
	defer rt.logMu.Unlock()
	fmt.Fprintf(rt.logFile, "%s %s\n", time.Now().UTC().Format(time.RFC3339Nano), msg)
}

// Args is the orchestrator's arguments, passed through from the config.
func (rt *Runtime) Args() json.RawMessage { return rt.cfg.Args }

// event appends one telemetry record.
func (rt *Runtime) event(kind string, fields map[string]any) {
	rec := map[string]any{"at": time.Now().UTC().Format(time.RFC3339Nano), "kind": kind}
	for k, v := range fields {
		rec[k] = v
	}
	b, err := json.Marshal(rec)
	if err != nil {
		b, _ = json.Marshal(map[string]any{"at": rec["at"], "kind": "telemetry-error", "error": err.Error()})
	}
	rt.telMu.Lock()
	defer rt.telMu.Unlock()
	rt.telFile.Write(append(b, '\n'))
}

func (rt *Runtime) writeStatus(state string) {
	b, _ := json.Marshal(map[string]any{"pid": os.Getpid(), "state": state, "at": time.Now().UTC().Format(time.RFC3339Nano), "launch": rt.launchID})
	if err := writeFileAtomic(filepath.Join(rt.cfg.StateDir, "status.json"), append(b, '\n')); err != nil {
		rt.Log("status.json: " + err.Error())
	}
}
