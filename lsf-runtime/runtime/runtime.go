package lsfrt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Runtime is one launch's host runtime: the orchestrator's agent, parallel,
// log and args, plus journaled code steps.
type Runtime struct {
	cfg       Config
	launchID  string // names this launch's transcripts
	stateHash string // labels this state dir's containers
	started   time.Time

	logMu   sync.Mutex
	logFile *os.File
	telMu   sync.Mutex
	telFile *os.File

	childMu  sync.Mutex
	children map[string]int // gate marker → pid (0 before Start)

	sem chan struct{} // concurrency slots

	mu         sync.Mutex
	stop       *StopError    // sticky; the first stop wins
	stopCh     chan struct{} // closed when stop is set
	killCh     chan struct{} // closed on a repeated signal
	killed     bool
	calls      int     // spawns this launch
	cost       float64 // summed total_cost_usd this launch
	limitUntil time.Time

	wtSem   chan struct{} // one slot: serialises git worktree and clone operations
	gateSem chan struct{} // GateConcurrency slots: bounds concurrent RunGates
	gateSeq atomic.Int64  // numbers gate processes for their orphan markers

	seenMu sync.Mutex
	seen   map[string]string // journal key → input hash, this launch

	hostsMu     sync.Mutex
	hosts       []string // the plan's network allowance, as PinHosts set it
	hostsPinned bool

	proxy      *proxy                            // the credential proxy, nil without a proxy config
	egress     atomic.Pointer[egressProxy]       // the egress proxy, nil without hosts
	egressHook func(target string, allowed bool) // tests: every egress decision
}

// Run is a launch. It takes the state-dir lock, reaps orphans, installs
// signal handlers, writes status.json, runs body and writes summary.json.
// The returned error is for setup failures only; how body ended is the
// Summary's stop_reason.
func Run(ctx context.Context, cfg Config, body func(ctx context.Context, rt *Runtime) (json.RawMessage, error)) (Summary, error) {
	if err := cfg.prepare(); err != nil {
		return Summary{}, fmt.Errorf("lsf-runtime: config: %w", err)
	}
	if cfg.Repo != "" {
		if err := checkRepo(cfg); err != nil {
			return Summary{}, fmt.Errorf("lsf-runtime: %w", err)
		}
	}
	// The state dir is the user's alone when made here: its transcripts can
	// carry the placeholder bearer, which spends the credential through the
	// proxy.
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		return Summary{}, fmt.Errorf("lsf-runtime: state dir: %w", err)
	}
	tightened := false
	if fi, err := os.Stat(cfg.StateDir); err == nil && fi.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(cfg.StateDir, 0o700); err != nil {
			return Summary{}, fmt.Errorf("lsf-runtime: state dir %s is readable by others and cannot be made private: %w", cfg.StateDir, err)
		}
		tightened = true
	}
	// Resolved once, so a clone's config, its mount and a merge clone's
	// resolved path name it alike.
	resolved, err := filepath.EvalSymlinks(cfg.StateDir)
	if err != nil {
		return Summary{}, fmt.Errorf("lsf-runtime: state dir: %w", err)
	}
	if strings.Contains(resolved, ":") {
		return Summary{}, fmt.Errorf("lsf-runtime: state dir %s: a path with ':' cannot be a bind mount's source", resolved)
	}
	cfg.StateDir = resolved
	for _, d := range []string{"journal", "calls", "wt", "clone", "scratch", "merge", "gates/logs"} {
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
	stateHash, err := stateHashOf(cfg.StateDir)
	if err != nil {
		return Summary{}, fmt.Errorf("lsf-runtime: state dir: %w", err)
	}
	if cfg.ClaudeBin, err = resolveBin(cfg.ClaudeBin); err != nil {
		return Summary{}, fmt.Errorf("lsf-runtime: claudeBin: %w", err)
	}
	rt := &Runtime{
		cfg:       cfg,
		stateHash: stateHash,
		started:   time.Now(),
		children:  map[string]int{},
		seen:      map[string]string{},
		sem:       make(chan struct{}, cfg.Concurrency),
		stopCh:    make(chan struct{}),
		killCh:    make(chan struct{}),
		wtSem:     make(chan struct{}, 1),
		gateSem:   make(chan struct{}, cfg.GateConcurrency),
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

	if err := rt.checkContainerHost(ctx); err != nil {
		rt.Log("launch check: " + err.Error())
		return Summary{}, fmt.Errorf("lsf-runtime: launch check: %w", err)
	}
	stopSignals := rt.watchSignals()
	defer stopSignals()
	if err := rt.reapOrphans(); err != nil {
		rt.Log(err.Error())
		return Summary{}, fmt.Errorf("lsf-runtime: %w", err)
	}
	rt.wipeScratch()

	rt.Log(fmt.Sprintf("launch %s: state dir %s", rt.launchID, cfg.StateDir))
	if tightened {
		rt.Log("state dir was readable by other users; made 0700 (transcripts can carry the placeholder bearer)")
	}
	rt.event("launch", map[string]any{"launch": rt.launchID, "pid": os.Getpid(), "dryRun": cfg.DryRun})
	if rt.proxy, err = rt.startProxy(); err != nil {
		rt.Log("proxy: " + err.Error())
		return Summary{}, fmt.Errorf("lsf-runtime: proxy: %w", err)
	}
	rt.writeStatus("running")

	outcome, bodyErr, panicked := runBody(ctx, rt, body)
	if rt.proxy != nil {
		rt.proxy.close() // calls are done; its count goes in the log before the end
	}
	if e := rt.egress.Load(); e != nil {
		e.close()
	}
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
	case panicked != nil:
		s.StopReason, s.Error = StopCrashed, fmt.Sprintf("panic: %v", panicked)
	case ctx.Err() != nil:
		s.StopReason = StopCancelled
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

// PinHosts records the hosts the plan's gate block allows, as the launch
// pinned them from the plan at args.planSha before any agent call, and
// starts the egress proxy that confines agent calls and setup to them when
// there are any. A failed start, or a second pin with other hosts, sets the
// runtime-error stop; the hosts stay pinned with no egress proxy, so setup
// runs with no network. A second pin with the same hosts does nothing.
func (rt *Runtime) PinHosts(hosts []string) {
	rt.hostsMu.Lock()
	defer rt.hostsMu.Unlock()
	if rt.hostsPinned {
		if !slices.Equal(rt.hosts, hosts) {
			rt.setStop(StopRuntimeError, fmt.Sprintf("hosts pinned again with a different list (%v, was %v)", hosts, rt.hosts))
		}
		return
	}
	rt.hostsPinned = true
	rt.hosts = slices.Clone(hosts)
	if len(hosts) == 0 {
		return
	}
	e, err := rt.startEgress(hosts)
	if err != nil {
		rt.Log("egress: " + err.Error())
		rt.setStop(StopRuntimeError, "egress proxy: "+err.Error())
		return
	}
	rt.egress.Store(e)
}

// Hosts are the pinned hosts, nil before PinHosts or for a plan with none.
func (rt *Runtime) Hosts() []string {
	rt.hostsMu.Lock()
	defer rt.hostsMu.Unlock()
	return slices.Clone(rt.hosts)
}

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
