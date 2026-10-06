// Package orchtest is the orchestrator's test harness: a stub runtime that
// answers agent calls from handlers keyed by label prefix and records every
// call, plus the plan and stage-return fixtures. Ported from the stub
// runtime in workflows/lsf/test/implement.test.js. Used by the orchestrator
// tests, tools/promptparity and the integration test.
package orchtest

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// M is a JSON object in a stage return. Its key order is not kept: use the
// typed values (plan.Finding, plan.Merge, DirectEdit) for objects a prompt
// embeds, so their JSON key order is the schema's.
type M = map[string]any

// DirectEdit is a direct edit as a planning stage returns it.
type DirectEdit struct {
	Path        string `json:"path"`
	Description string `json:"description"`
}

// Ret is a stage return: result plus an empty commits list, with extra
// top-level fields merged in.
func Ret(result any, extra ...M) M {
	r := M{"result": result, "commits": []any{}}
	for _, e := range extra {
		for k, v := range e {
			r[k] = v
		}
	}
	return r
}

// Call is one agent call or code step as the stub saw it.
type Call struct {
	Label  string // final label
	Prefix string // before the first ':', with any #n suffix dropped
	Unit   string // last ':' component, with any #n suffix dropped
	Prompt string // agent calls only
	Opts   lsfrt.Opts
	Step   bool            // a code step
	Input  json.RawMessage // a step's input
}

// State is what handlers share across a run: the plan the stub's copy stage
// serves, the units merged so far and the current merge's order.
type State struct {
	Plan       *plan.Plan
	Completed  map[string]bool
	MergeOrder []string
	Seq        int // SHA counter for merge and re-plan
}

// Handler answers one call or step: nil is null; json.RawMessage is sent as
// is; an lsfrt.CallResult is a call's whole result, flags and denials
// included; an error is returned from Call or Step (a *lsfrt.StopError to
// stop the run; from Step, any other error is the stage's failure);
// anything else is marshalled. A step's fn is never run: its handler is its
// value.
type Handler func(c Call, s *State) any

// Stub is the runtime. Call and Step hold the stub's lock while a handler
// runs, so Concurrent branches serialise inside the stub (the orchestrator
// code between calls still runs concurrently), and a handler must not call
// the stub's methods. Labels, Prompt, Input and CallOf read without the
// lock: call them after the run. The stub has no repository: Repo is "" and
// Worktree fails.
type Stub struct {
	Handlers   map[string]Handler
	ArgsJSON   json.RawMessage
	State      *State
	Concurrent bool // run Parallel branches on goroutines
	// ExpectPanic allows a recovered panic in a Parallel branch.
	ExpectPanic bool

	mu       sync.Mutex
	Calls    []Call
	Logs     []string
	Returns  map[string]json.RawMessage // by final label, calls and steps; null included
	Failures []string                   // harness failures: repeated label, missing handler, unexpected panic
}

// New is a stub over p with the default handlers, overrides applied, and
// args {name: f, planSha: plan0, architectureDoc: docs/architecture/f.md,
// integrationBranch: integration} plus extra. With t, harness failures fail
// the test at cleanup.
func New(t testing.TB, p *plan.Plan, overrides map[string]Handler, extra M) *Stub {
	h := Defaults()
	for k, v := range overrides {
		h[k] = v
	}
	a := M{"name": "f", "planSha": "plan0", "architectureDoc": "docs/architecture/f.md", "integrationBranch": "integration"}
	for k, v := range extra {
		a[k] = v
	}
	b, err := plan.MarshalJS(a, "")
	if err != nil {
		panic(err)
	}
	s := &Stub{
		Handlers: h, ArgsJSON: b,
		State:   &State{Plan: p, Completed: map[string]bool{}},
		Returns: map[string]json.RawMessage{},
	}
	if t != nil {
		t.Cleanup(func() {
			for _, f := range s.Failures {
				t.Errorf("orchtest: %s", f)
			}
		})
	}
	return s
}

func (s *Stub) fail(format string, a ...any) {
	s.Failures = append(s.Failures, fmt.Sprintf(format, a...))
}

var mergeLine = regexp.MustCompile(`(?m)^- (\S+): branch`)

// newCall is a call record for label, with its prefix and unit.
func newCall(label string) Call {
	base, _, _ := strings.Cut(label, "#")
	parts := strings.Split(base, ":")
	return Call{Label: label, Prefix: parts[0], Unit: parts[len(parts)-1]}
}

// answer records c and runs its handler; the caller holds s.mu. The value
// is recorded in Returns unless the handler errs.
func (s *Stub) answer(c Call) (lsfrt.CallResult, error) {
	for _, prev := range s.Calls {
		if prev.Label == c.Label {
			s.fail("label %q repeated within the launch", c.Label)
		}
	}
	s.Calls = append(s.Calls, c)
	h := s.Handlers[c.Prefix]
	if h == nil {
		s.fail("no stub for %s", c.Label)
		return lsfrt.CallResult{}, nil
	}
	var cr lsfrt.CallResult
	switch v := h(c, s.State).(type) {
	case nil:
	case error:
		return lsfrt.CallResult{}, v
	case lsfrt.CallResult:
		cr = v
	case json.RawMessage:
		cr.Value = v
	default:
		b, err := plan.MarshalJS(v, "")
		if err != nil {
			s.fail("%s: marshal return: %v", c.Label, err)
			return lsfrt.CallResult{}, nil
		}
		cr.Value = b
	}
	if cr.Value == nil {
		s.Returns[c.Label] = json.RawMessage("null")
	} else {
		s.Returns[c.Label] = cr.Value
	}
	return cr, nil
}

// Call answers from the handler for the label's prefix.
func (s *Stub) Call(ctx context.Context, prompt string, o lsfrt.Opts) (lsfrt.CallResult, error) {
	c := newCall(o.Label)
	c.Prompt, c.Opts = prompt, o

	s.mu.Lock()
	defer s.mu.Unlock()
	if c.Prefix == "merge" {
		s.State.MergeOrder = nil
		for _, m := range mergeLine.FindAllStringSubmatch(prompt, -1) {
			s.State.MergeOrder = append(s.State.MergeOrder, m[1])
		}
	}
	cr, err := s.answer(c)
	if err != nil {
		return cr, err
	}
	if c.Prefix == "merge" && cr.Value != nil {
		var r struct {
			Result *struct {
				Merges []struct {
					Unit   string `json:"unit"`
					Status string `json:"status"`
				} `json:"merges"`
			} `json:"result"`
		}
		if json.Unmarshal(cr.Value, &r) == nil && r.Result != nil {
			for _, m := range r.Result.Merges {
				if m.Status != "unresolvable" {
					s.State.Completed[m.Unit] = true
				}
			}
		}
	}
	return cr, nil
}

// Step answers a code step from the handler for the label's prefix; fn is
// not run.
func (s *Stub) Step(ctx context.Context, label string, input []byte, fn func(context.Context, bool) (json.RawMessage, error)) (json.RawMessage, error) {
	c := newCall(label)
	c.Step, c.Input = true, append(json.RawMessage{}, input...)
	s.mu.Lock()
	defer s.mu.Unlock()
	cr, err := s.answer(c)
	return cr.Value, err
}

// Repo is "": the stub has no repository.
func (s *Stub) Repo() string { return "" }

// Worktree fails: the stub has no repository.
func (s *Stub) Worktree(ctx context.Context, label string) (string, string, error) {
	return "", "", fmt.Errorf("orchtest: no worktree for %s", label)
}

// Parallel runs the thunks in order, or on goroutines when Concurrent. A
// branch's panic or non-stop error leaves its slot nil; a panic is a
// harness failure unless ExpectPanic. The first stop is returned after all
// branches finish.
func (s *Stub) Parallel(ctx context.Context, thunks []func(context.Context) (json.RawMessage, error)) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, len(thunks))
	errs := make([]error, len(thunks))
	run := func(i int) {
		defer func() {
			if p := recover(); p != nil {
				out[i], errs[i] = nil, nil
				if !s.ExpectPanic {
					s.mu.Lock()
					s.fail("parallel branch %d panicked: %v\n%s", i, p, debug.Stack())
					s.mu.Unlock()
				}
			}
		}()
		out[i], errs[i] = thunks[i](ctx)
	}
	if s.Concurrent {
		var wg sync.WaitGroup
		for i := range thunks {
			wg.Go(func() { run(i) })
		}
		wg.Wait()
	} else {
		for i := range thunks {
			run(i)
		}
	}
	var stop error
	for i, err := range errs {
		if err == nil {
			continue
		}
		if _, ok := lsfrt.IsStop(err); ok {
			if stop == nil {
				stop = err
			}
			continue
		}
		out[i] = nil
	}
	return out, stop
}

func (s *Stub) Log(msg string) {
	s.mu.Lock()
	s.Logs = append(s.Logs, msg)
	s.mu.Unlock()
}

func (s *Stub) Args() json.RawMessage { return s.ArgsJSON }

// Labels are the final labels in call order.
func (s *Stub) Labels() []string {
	out := []string{}
	for _, c := range s.Calls {
		out = append(out, c.Label)
	}
	return out
}

// Input is the input of the step with label, or nil with a failure.
func (s *Stub) Input(label string) json.RawMessage {
	if c := s.CallOf(label); c != nil && c.Step {
		return c.Input
	}
	s.fail("no step labelled %s", label)
	return nil
}

// Prompt is the prompt of the call with label, or "" with a failure.
func (s *Stub) Prompt(label string) string {
	for _, c := range s.Calls {
		if c.Label == label {
			return c.Prompt
		}
	}
	s.fail("no call labelled %s", label)
	return ""
}

// CallOf is the call with label, or nil.
func (s *Stub) CallOf(label string) *Call {
	for i := range s.Calls {
		if s.Calls[i].Label == label {
			return &s.Calls[i]
		}
	}
	return nil
}
