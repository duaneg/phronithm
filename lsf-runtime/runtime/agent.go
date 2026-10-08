package lsfrt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"
)

// Call runs one headless call (design §2.5), or replays it from the journal.
// The only non-nil error is *StopError. A replay is served even after a stop:
// it costs nothing, and the stop returns at the first call that would spawn.
func (rt *Runtime) Call(ctx context.Context, prompt string, o Opts) (CallResult, error) {
	key := journalKey(o.Label)
	skill := rt.cfg.skillFor(o.Label)
	hash := callHash(prompt, skill, o)
	rt.noteLabel(key, hash, o.Label)
	if res, unchecked, ok := rt.replayCall(key, hash, o.Label, o.Schema); ok {
		rt.recheckUncommitted(ctx, o, key, unchecked, &res)
		return res, nil
	}
	if err := rt.boundary(ctx); err != nil {
		return CallResult{}, err
	}
	if !json.Valid(o.Schema) || len(o.Schema) == 0 || o.Schema[0] != '{' {
		return CallResult{}, rt.setStop(StopRuntimeError, o.Label+": opts.schema is not a JSON object")
	}
	c := &callState{o: o, key: key, skill: skill, sent: withSkill(skill, prompt)}
	c.roleName, c.role = rt.cfg.roleFor(o)
	if o.Isolation != "" || rt.usesClone(o) { // a dry run checks args too
		if _, err := rt.cloneArgs(o); err != nil {
			return CallResult{}, err
		}
	}
	if rt.cfg.DryRun {
		argv := buildArgv(o, c.role, rt.cfg.PluginDir, "<session-id>")
		rt.event("dry-run", map[string]any{"label": o.Label, "role": c.roleName, "argv": argv, "stdin": truncateRunes(c.sent, 200)})
		rt.Log(fmt.Sprintf("dry run: %s would run %s %q", o.Label, rt.cfg.ClaudeBin, argv))
		return CallResult{}, rt.setStop(StopDryRun, o.Label)
	}

	// A discarded clone goes whatever the call's end; afterCall removes it
	// on a return, this on a stop.
	defer func() {
		if c.placed && c.place.kind == CloneDiscarded {
			rt.discard(o.Label, c.place)
		}
	}()
	res := &c.res
	var retried, transient, waits int // shared invalid/skill-missing retry; transient retries; session-limit waits
	for {
		if err := rt.waitLimit(ctx); err != nil {
			return *res, err
		}
		a, v, err := rt.attempt(ctx, c)
		if errors.Is(err, errLimitPending) { // the limit moved while this call queued
			continue
		}
		if err != nil {
			return *res, err
		}
		switch v.class {
		case classOK, classSkillMissing:
			if v.class == classSkillMissing && retried == 0 && rt.stopped() == nil {
				retried++
				continue
			}
			res.Value, _ = compactJSON(v.output) // validated, so JSON
			if v.class == classSkillMissing {
				res.Flags = append(res.Flags, FlagSkillNotLoaded)
				rt.event("skill-not-loaded", map[string]any{"label": o.Label, "skill": skill})
				rt.Log(fmt.Sprintf("%s: skill %s not loaded after %d attempt(s); return accepted and flagged", o.Label, skill, res.Attempts))
			}
			if v.guardKilled {
				res.Flags = append(res.Flags, FlagGuardKilledAfterResult)
				rt.Log(o.Label + ": the guard timeout killed the call after its result; the result stands")
			}
			if err := rt.afterCall(ctx, c, res); err != nil {
				return *res, err
			}
			rt.recordCall(key, hash, o.Label, *res)
			return *res, nil
		case classStopped:
			return *res, rt.stopErr()
		case classTimeout, classPermanent, classSpawnError:
			err := rt.checkNull(ctx, c, res)
			return *res, err
		case classInvalid:
			if retried == 0 {
				retried++
				continue
			}
			rt.Log(fmt.Sprintf("%s: no valid return after %d attempt(s) (%s); null", o.Label, res.Attempts, firstOr(v.errors, "invalid")))
			res.Violations = v.errors
			err := rt.checkNull(ctx, c, res)
			return *res, err
		case classTransient:
			if transient < len(rt.cfg.Backoff) {
				d := rt.cfg.Backoff[transient].D()
				transient++
				rt.Log(fmt.Sprintf("%s: attempt %d failed (exit %d); retrying in %v", o.Label, res.Attempts, a.exitCode, d))
				if err := rt.sleep(ctx, d); err != nil {
					return *res, err
				}
				continue
			}
			rt.Log(fmt.Sprintf("%s: transient failure persisted over %d attempt(s); null", o.Label, res.Attempts))
			err := rt.checkNull(ctx, c, res)
			return *res, err
		case classSessionLimit:
			if waits < rt.cfg.MaxSessionWaits {
				waits++
				continue
			}
			rt.Log(fmt.Sprintf("%s: session limit persisted over %d wait(s); null", o.Label, waits))
			err := rt.checkNull(ctx, c, res)
			return *res, err
		}
		panic("unreachable: class " + v.class)
	}
}

// callState is one Call's fixed inputs and running result.
type callState struct {
	o          Opts
	key, skill string
	sent       string // the prompt as sent
	roleName   string
	role       Role
	place      placement // resolved at the first admitted attempt
	placed     bool
	res        CallResult
}

// attempt takes a concurrency slot, admits, spawns and classifies one
// attempt, holding the slot throughout; the deferred release survives a
// panic. The clone is made at the first admitted attempt, so a call
// refused by a cap or stop leaves none. A session limit moves the gate before
// the slot frees, so a call queued for the slot cannot spawn into it.
func (rt *Runtime) attempt(ctx context.Context, c *callState) (*attempt, verdict, error) {
	if err := rt.acquire(ctx); err != nil {
		return nil, verdict{}, err
	}
	defer rt.release()
	if err := rt.admit(ctx); err != nil {
		return nil, verdict{}, err
	}
	if !c.placed {
		p, err := rt.placeCall(ctx, c.o, c.key, c.role)
		if err != nil {
			return nil, verdict{}, err
		}
		c.place, c.placed = p, true
		c.res.Clone = p.kind
	}
	sid := newUUID()
	t0 := time.Now()
	a := rt.spawn(ctx, spawnSpec{
		prompt:     c.sent,
		argv:       buildArgv(c.o, c.role, rt.cfg.PluginDir, sid),
		sessionID:  sid,
		cwd:        c.place.dir,
		transcript: filepath.Join(rt.cfg.StateDir, "calls", fmt.Sprintf("%s.%s.a%d.%s.jsonl", c.key, rt.launchID, c.res.Attempts, sid[:8])),
	})
	c.res.Attempts++
	if r := a.acc.Result; r != nil {
		c.res.CostUSD += r.CostUSD
		for _, d := range r.Denials { // across attempts: a retried attempt's denial still happened
			if !slices.Contains(c.res.Denials, d) {
				c.res.Denials = append(c.res.Denials, d)
			}
		}
		rt.addCost(r.CostUSD)
	}
	if a.cause != nil && !errors.Is(a.cause, errGuard) && !errors.Is(a.cause, errKilled) {
		return a, verdict{}, &StopError{Reason: StopCancelled}
	}
	v := classify(a, c.o.Schema, c.skill, c.skill != "")
	if v.class == classSessionLimit {
		// A reset already past (a stale event) still waits the margin; other
		// calls learn of the limit even when this one gives up.
		until := time.Now().Add(rt.cfg.SessionLimitFallback.D())
		if !v.resetsAt.IsZero() {
			until = maxTime(v.resetsAt, time.Now()).Add(rt.cfg.SessionLimitMargin.D())
		}
		rt.extendLimit(until, c.o.Label)
	}
	c.res.Class = string(v.class)
	rt.recordAttempt(c.o, c.roleName, sid, c.res.Attempts-1, a, v, time.Since(t0))
	return a, v, nil
}

// Agent is Call's value: the orchestrator's agent(). (nil, nil) is null.
func (rt *Runtime) Agent(ctx context.Context, prompt string, o Opts) (json.RawMessage, error) {
	res, err := rt.Call(ctx, prompt, o)
	return res.Value, err
}

// boundary is the check at a call boundary: a sticky stop or an ended parent
// context ends the run.
func (rt *Runtime) boundary(ctx context.Context) error {
	if s := rt.stopped(); s != nil {
		return s
	}
	if ctx.Err() != nil {
		return &StopError{Reason: StopCancelled}
	}
	return nil
}

// sleep blocks for d, or until a stop or the end of ctx.
func (rt *Runtime) sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-rt.stopCh:
		return rt.stopErr()
	case <-ctx.Done():
		return &StopError{Reason: StopCancelled}
	}
}

// waitLimit holds a spawn until the session limit resets. limitUntil can move
// while a call waits, so it is re-read on wake.
func (rt *Runtime) waitLimit(ctx context.Context) error {
	for {
		rt.mu.Lock()
		until := rt.limitUntil
		rt.mu.Unlock()
		d := time.Until(until)
		if d <= 0 {
			return nil
		}
		if capWall := rt.cfg.Caps.MaxWallClock.D(); capWall > 0 && until.After(rt.started.Add(capWall)) {
			return rt.setStop(StopMaxWallClock, fmt.Sprintf("session limit resets at %s, past the wall-clock cap", until.Format(time.RFC3339)))
		}
		if err := rt.sleep(ctx, d); err != nil {
			return err
		}
	}
}

// extendLimit moves the runtime-wide session-limit gate. The limit is taken
// to apply across models (the five-hour session limit).
func (rt *Runtime) extendLimit(until time.Time, label string) {
	rt.mu.Lock()
	if until.After(rt.limitUntil) {
		rt.limitUntil = until
	}
	until = rt.limitUntil
	rt.mu.Unlock()
	rt.event("session-limit-wait", map[string]any{"label": label, "until": until.UTC().Format(time.RFC3339)})
	rt.Log(fmt.Sprintf("%s: session limit; calls wait until %s (%v)", label, until.Format(time.RFC3339), time.Until(until).Round(time.Second)))
}

// acquire takes a concurrency slot.
func (rt *Runtime) acquire(ctx context.Context) error {
	select {
	case rt.sem <- struct{}{}:
		return nil
	case <-rt.stopCh:
		return rt.stopErr()
	case <-ctx.Done():
		return &StopError{Reason: StopCancelled}
	}
}

func (rt *Runtime) release() { <-rt.sem }

// errLimitPending sends a call that queued for a slot back to the gate.
var errLimitPending = errors.New("session limit pending")

// admit is the per-spawn check, in one critical section: the sticky stop,
// the parent context, the session-limit gate (a call can pass it, then queue
// for a slot while another call meets the limit), then the caps; a spawn it
// admits is counted.
func (rt *Runtime) admit(ctx context.Context) error {
	rt.mu.Lock()
	if rt.stop != nil {
		s := rt.stop
		rt.mu.Unlock()
		return s
	}
	if ctx.Err() != nil {
		rt.mu.Unlock()
		return &StopError{Reason: StopCancelled}
	}
	if time.Now().Before(rt.limitUntil) {
		rt.mu.Unlock()
		return errLimitPending
	}
	c := rt.cfg.Caps
	var reason StopReason
	var detail string
	switch {
	case c.MaxCalls > 0 && rt.calls >= c.MaxCalls:
		reason, detail = StopMaxCalls, fmt.Sprintf("%d calls", rt.calls)
	case c.MaxWallClock > 0 && time.Since(rt.started) >= c.MaxWallClock.D():
		reason, detail = StopMaxWallClock, time.Since(rt.started).Round(time.Second).String()
	case c.MaxCostUSD > 0 && rt.cost >= c.MaxCostUSD:
		reason, detail = StopMaxCost, fmt.Sprintf("$%.4f", rt.cost)
	default:
		rt.calls++
		rt.mu.Unlock()
		return nil
	}
	s, set := rt.setStopLocked(reason, detail)
	rt.mu.Unlock()
	if set {
		rt.stopEvent(s)
	}
	return s
}

func (rt *Runtime) addCost(c float64) {
	rt.mu.Lock()
	rt.cost += c
	rt.mu.Unlock()
}

func (rt *Runtime) recordAttempt(o Opts, role, sid string, n int, a *attempt, v verdict, d time.Duration) {
	f := map[string]any{
		"label": o.Label, "phase": o.Phase, "attempt": n, "class": v.class, "session": sid,
		"model": o.Model, "effort": o.Effort, "role": role, "exit": a.exitCode, "secs": d.Seconds(),
	}
	if r := a.acc.Result; r != nil {
		f["cost"] = r.CostUSD
		f["denials"] = r.Denials
		f["terminal_reason"] = r.TerminalReason
		f["api_error_status"] = r.APIErrorStatus
	}
	if len(a.acc.AssistantErrors) > 0 {
		f["assistant_errors"] = a.acc.AssistantErrors
	}
	if len(v.errors) > 0 {
		f["errors"] = v.errors[:min(3, len(v.errors))]
	}
	if a.acc.BadLines > 0 {
		f["bad_lines"] = a.acc.BadLines
	}
	if a.exitCode != 0 && a.stderr != "" {
		f["stderr"] = a.stderr
	}
	rt.event("call", f)
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func firstOr(xs []string, def string) string {
	if len(xs) > 0 {
		return xs[0]
	}
	return def
}
