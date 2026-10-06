package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// Runtime is what the orchestrator needs from its host: *lsfrt.Runtime, or
// a stub in tests. Agent's only error is a *lsfrt.StopError, and its nil
// value is null. Parallel recovers a branch's panic or non-stop error to a
// nil slot and returns the first stop after every branch finishes.
type Runtime interface {
	Agent(ctx context.Context, prompt string, o lsfrt.Opts) (json.RawMessage, error)
	Parallel(ctx context.Context, thunks []func(context.Context) (json.RawMessage, error)) ([]json.RawMessage, error)
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

	mu   sync.Mutex
	uses map[string]int // real calls per label this launch
}

// agent is the orchestrator's agent call (D10). A seeded label stands in
// for the agent on every use, as JS's seed wrapper does; a seed reports the
// pause ref as it stood when salvaged, so its report is dropped. A real call
// gets a launch-unique label: the nth use of a label is `label#n`.
// It returns the label used.
func (o *orch) agent(ctx context.Context, prompt string, opts lsfrt.Opts) (json.RawMessage, string, error) {
	if s, ok := o.seed[opts.Label]; ok {
		o.rt.Log(opts.Label + ": seeded from args.seed; agent not spawned")
		return stripSeedPause(s), opts.Label, nil
	}
	o.mu.Lock()
	o.uses[opts.Label]++
	if n := o.uses[opts.Label]; n > 1 {
		opts.Label = fmt.Sprintf("%s#%d", opts.Label, n)
	}
	o.mu.Unlock()
	raw, err := o.rt.Agent(ctx, prompt, opts)
	return raw, opts.Label, err
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

// commit is one entry of a stage's `commits`.
type commit struct {
	Branch string `json:"branch"`
	Sha    string `json:"sha"`
}

// ret is a stage return: `result` and `commits`.
type ret[T any] struct {
	Result  T        `json:"result"`
	Commits []commit `json:"commits"`
}

// decode reads a stage return into its typed struct. Null is nil; a return
// that does not decode is nil too, with the reason logged (D9), so the
// caller's null path makes it the stage's agent-null.
func decode[T any](rt Runtime, label string, raw json.RawMessage) *ret[T] {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var r struct {
		Result  *T       `json:"result"`
		Commits []commit `json:"commits"`
	}
	err := json.Unmarshal(raw, &r)
	if err == nil && r.Result == nil {
		err = fmt.Errorf("no result")
	}
	if err != nil {
		rt.Log(fmt.Sprintf("%s: return does not decode (%v); treated as null", label, err))
		return nil
	}
	return &ret[T]{Result: *r.Result, Commits: r.Commits}
}

// call is agent plus decode.
func call[T any](ctx context.Context, o *orch, prompt string, opts lsfrt.Opts) (*ret[T], error) {
	raw, label, err := o.agent(ctx, prompt, opts)
	if err != nil {
		return nil, err
	}
	return decode[T](o.rt, label, raw), nil
}
