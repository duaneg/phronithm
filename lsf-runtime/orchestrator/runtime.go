package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// Runtime is what the orchestrator needs from its host: *lsfrt.Runtime, or
// a stub in tests. Call's only error is a *lsfrt.StopError, and its nil
// Value is null. Parallel recovers a branch's panic or non-stop error to a
// nil slot and returns the first stop after every branch finishes. Step
// journals a code step (lsfrt.Runtime.Step). Repo is the main checkout ("" when
// there is none); Worktree is a code step's own worktree and its branch.
type Runtime interface {
	Call(ctx context.Context, prompt string, o lsfrt.Opts) (lsfrt.CallResult, error)
	Parallel(ctx context.Context, thunks []func(context.Context) (json.RawMessage, error)) ([]json.RawMessage, error)
	Step(ctx context.Context, label string, input []byte, fn func(ctx context.Context, interrupted bool) (json.RawMessage, error)) (json.RawMessage, error)
	Repo() string
	Worktree(ctx context.Context, label string) (path, branch string, err error)
	Log(msg string)
	Args() json.RawMessage
}

// Body is Run with lsfrt.Run's body signature.
func Body(ctx context.Context, rt *lsfrt.Runtime) (json.RawMessage, error) {
	return Run(ctx, rt)
}

// orch is one launch: the runtime, the context every prompt reads, and the
// label counter.
type orch struct {
	rt   Runtime
	ctx  *runCtx
	seed map[string]json.RawMessage

	// The code stages' repository: git is nil when the runtime has none.
	git   gitRunner
	repo  string
	today func() string // the decision log's entry date

	mu   sync.Mutex
	uses map[string]int // real calls and steps per label this launch
}

// newOrch is a launch's orch over rt.
func newOrch(rt Runtime, c *runCtx, seed map[string]json.RawMessage) *orch {
	o := &orch{rt: rt, ctx: c, seed: seed, uses: map[string]int{}, today: func() string { return time.Now().Format(time.DateOnly) }}
	if repo := rt.Repo(); repo != "" {
		o.git, o.repo = execGit{}, repo
	}
	return o
}

// agent is the orchestrator's agent call (D10). A seeded label stands in
// for the agent on every use, as JS's seed wrapper does; a seed reports the
// pause ref as it stood when salvaged, so its report is dropped. A real call
// gets a launch-unique label (unique): the nth use of a label is `label#n`.
// It returns the call's result and the label used.
func (o *orch) agent(ctx context.Context, prompt string, opts lsfrt.Opts) (lsfrt.CallResult, string, error) {
	if s, ok := o.seed[opts.Label]; ok {
		o.rt.Log(opts.Label + ": seeded from args.seed; agent not spawned")
		cr := lsfrt.CallResult{Value: stripSeedPause(s)}
		// The runtime validates every return it makes; a seed it never saw.
		if errs := lsfrt.ValidateOutput(opts.Schema, cr.Value); len(cr.Value) > 0 && string(cr.Value) != "null" && len(errs) > 0 {
			o.rt.Log(fmt.Sprintf("%s: seed outside its schema (%s); treated as null", opts.Label, strings.Join(errs, "; ")))
			cr.Value, cr.Violations = nil, errs
		}
		return cr, opts.Label, nil
	}
	opts.Label = o.unique(opts.Label)
	cr, err := o.rt.Call(ctx, prompt, opts)
	return cr, opts.Label, err
}

// unique is label for its first use in the launch and `label#n` for its
// nth. Calls and code steps share the count, so their labels never collide.
func (o *orch) unique(label string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.uses[label]++
	if n := o.uses[label]; n > 1 {
		return fmt.Sprintf("%s#%d", label, n)
	}
	return label
}

// agentReq is one agent call in a parallel batch.
type agentReq struct {
	prompt string
	opts   lsfrt.Opts
}

// agentRes is one parallel call's result and the label it ran under. Its
// Value is null when the branch failed.
type agentRes struct {
	cr    lsfrt.CallResult
	label string
}

// parallelAgents runs the calls on rt.Parallel; each branch's CallResult
// lands in its own slot.
func (o *orch) parallelAgents(ctx context.Context, reqs []agentReq) ([]agentRes, error) {
	out := make([]agentRes, len(reqs))
	thunks := []func(context.Context) (json.RawMessage, error){}
	for i, r := range reqs {
		thunks = append(thunks, func(ctx context.Context) (json.RawMessage, error) {
			cr, label, err := o.agent(ctx, r.prompt, r.opts)
			if err == nil { // a failed branch's slot stays empty: no flags without a call
				out[i] = agentRes{cr, label}
			}
			return cr.Value, err
		})
	}
	raws, err := o.rt.Parallel(ctx, thunks)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].cr.Value = raws[i]
	}
	return out, nil
}

// stageFn is a code step's body; label is the step's final label.
type stageFn func(ctx context.Context, label string, interrupted bool) (json.RawMessage, error)

// stageErr is a code step's failure: the reason its stop states.
type stageErr struct{ reason string }

func (e *stageErr) Error() string { return e.reason }

// runStep is the orchestrator's code step (D18): input, marshalled, is the
// journal's hash; fn runs only when no record replays. A seed for the label
// is logged and not used: a code stage has no agent to stand in for. The
// label counter is the agent calls'. An error is a *lsfrt.StopError, or a
// *stageErr for the stage's own failure; an error while ctx is done is the
// cancellation reaching git, so a stop.
func (o *orch) runStep(ctx context.Context, label string, input any, fn stageFn) (json.RawMessage, string, error) {
	if _, ok := o.seed[label]; ok {
		o.rt.Log(label + ": args.seed ignored; code stage")
	}
	label = o.unique(label)
	in, err := plan.MarshalJS(input, "")
	if err != nil {
		return nil, label, &stageErr{fmt.Sprintf("input: %v", err)}
	}
	raw, err := o.rt.Step(ctx, label, in, func(ctx context.Context, interrupted bool) (json.RawMessage, error) {
		return fn(ctx, label, interrupted)
	})
	switch {
	case err == nil:
		return raw, label, nil
	case isStop(err):
		return nil, label, err
	case ctx.Err() != nil:
		return nil, label, &lsfrt.StopError{Reason: lsfrt.StopCancelled}
	default:
		reason := err.Error()
		if reason == "" {
			reason = "stage failed"
		}
		o.rt.Log(label + ": " + reason)
		return nil, label, &stageErr{reason}
	}
}

func isStop(err error) bool {
	_, ok := lsfrt.IsStop(err)
	return ok
}

// stripSeedPause sets a seed's result.pauseRequested false when it is true.
func stripSeedPause(s json.RawMessage) json.RawMessage {
	var m map[string]any
	if json.Unmarshal(s, &m) != nil {
		return s
	}
	r, ok := m["result"].(map[string]any)
	if !ok || r["pauseRequested"] != true {
		return s
	}
	r["pauseRequested"] = false
	b, err := plan.MarshalJS(m, "")
	if err != nil {
		return s
	}
	return b
}

// ret is a stage return's `result`; its `commits` is not read.
type ret[T any] struct {
	Result T
}

// decode reads a stage return into its typed struct. Null is nil; a return
// that does not decode is nil too, with the reason logged (D9), so the
// caller's null path makes it the stage's agent-null.
func decode[T any](rt Runtime, label string, raw json.RawMessage) *ret[T] {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	// commits is not read, so it is not decoded either: a wrong-typed one
	// must not null a good stage.
	var r struct {
		Result *T `json:"result"`
	}
	err := json.Unmarshal(raw, &r)
	if err == nil && r.Result == nil {
		err = fmt.Errorf("no result")
	}
	if err != nil {
		rt.Log(fmt.Sprintf("%s: return does not decode (%v); treated as null", label, err))
		return nil
	}
	return &ret[T]{Result: *r.Result}
}

// site is where an agent call is made: its ledger position, and whether
// its stage commits (a writer's uncommitted changes stop; a reader's are
// recorded).
type site struct {
	at     plan.At
	writer bool
}

// callSigs maps a call's CallResult to its driver-event signals (D21) and
// returns the value to decode. Violations (the runtime's, or a seed's from
// agent) are a schema-violation; the value is then null.
func (o *orch) callSigs(s site, cr lsfrt.CallResult, label string, schema json.RawMessage) (json.RawMessage, []*plan.Signal) {
	sigs, unknown := plan.SignalsFromCall(plan.CallEvents{
		Label: label, Writer: s.writer, Flags: cr.Flags, Denials: cr.Denials, Uncommitted: cr.Uncommitted, Violations: cr.Violations,
	}, s.at)
	if len(unknown) > 0 {
		o.rt.Log(label + ": unknown call flag(s): " + strings.Join(unknown, ", "))
	}
	if len(cr.Violations) > 0 {
		return nil, sigs
	}
	return cr.Value, sigs
}

// hasStop reports whether any of sigs is a stop kind.
func hasStop(sigs []*plan.Signal) bool {
	return slices.ContainsFunc(sigs, func(s *plan.Signal) bool { return plan.IsStopKind(s.Kind) })
}

// call is agent, the driver-event mapping and decode. sigs are the call's
// driver-event signals; the caller appends them to its stage's signals.
func call[T any](ctx context.Context, o *orch, s site, prompt string, opts lsfrt.Opts) (*ret[T], []*plan.Signal, error) {
	cr, label, err := o.agent(ctx, prompt, opts)
	if err != nil {
		return nil, nil, err
	}
	v, sigs := o.callSigs(s, cr, label, opts.Schema)
	return decode[T](o.rt, label, v), sigs, nil
}

// step is runStep plus decode. fault is the stage's failure reason, "" when
// it ran; err is a stop.
func step[T any](ctx context.Context, o *orch, label string, input any, fn stageFn) (r *ret[T], fault string, err error) {
	raw, final, err := o.runStep(ctx, label, input, fn)
	var se *stageErr
	if errors.As(err, &se) {
		return nil, se.reason, nil
	}
	if err != nil {
		return nil, "", err
	}
	return decode[T](o.rt, final, raw), "", nil
}
