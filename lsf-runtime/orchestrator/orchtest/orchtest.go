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

// Call is one agent call as the stub saw it.
type Call struct {
	Label  string // final label
	Prefix string // before the first ':', with any #n suffix dropped
	Unit   string // last ':' component, with any #n suffix dropped
	Prompt string
	Opts   lsfrt.Opts
}

// State is what handlers share across a run: the plan the stub's copy stage
// serves, the units merged so far and the current merge's order.
type State struct {
	Plan       *plan.Plan
	Completed  map[string]bool
	MergeOrder []string
	Seq        int // SHA counter for merge and re-plan
}

// Handler answers one call: nil is null; json.RawMessage is sent as is; an
// error is returned from Agent (a *lsfrt.StopError to stop the run);
// anything else is marshalled.
type Handler func(c Call, s *State) any

// Stub is the runtime. Agent holds the stub's lock while a handler runs, so
// Concurrent branches serialise inside the stub (the orchestrator code
// between calls still runs concurrently), and a handler must not call the
// stub's methods. Labels, Prompt and CallOf read without the lock: call them
// after the run.
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
	Returns  map[string]json.RawMessage // by final label; null included
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

// Agent answers from the handler for the label's prefix.
func (s *Stub) Agent(ctx context.Context, prompt string, o lsfrt.Opts) (json.RawMessage, error) {
	base, _, _ := strings.Cut(o.Label, "#")
	parts := strings.Split(base, ":")
	prefix := parts[0]
	c := Call{Label: o.Label, Prefix: prefix, Unit: parts[len(parts)-1], Prompt: prompt, Opts: o}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, prev := range s.Calls {
		if prev.Label == o.Label {
			s.fail("label %q repeated within the launch", o.Label)
		}
	}
	s.Calls = append(s.Calls, c)
	if prefix == "merge" {
		s.State.MergeOrder = nil
		for _, m := range mergeLine.FindAllStringSubmatch(prompt, -1) {
			s.State.MergeOrder = append(s.State.MergeOrder, m[1])
		}
	}
	h := s.Handlers[prefix]
	if h == nil {
		s.fail("no stub for %s", o.Label)
		return nil, nil
	}
	out := h(c, s.State)
	var raw json.RawMessage
	switch v := out.(type) {
	case nil:
	case error:
		return nil, v
	case json.RawMessage:
		raw = v
	default:
		b, err := plan.MarshalJS(v, "")
		if err != nil {
			s.fail("%s: marshal return: %v", o.Label, err)
			return nil, nil
		}
		raw = b
	}
	if raw == nil {
		s.Returns[o.Label] = json.RawMessage("null")
	} else {
		s.Returns[o.Label] = raw
	}
	if prefix == "merge" && raw != nil {
		var r struct {
			Result *struct {
				Merges []struct {
					Unit   string `json:"unit"`
					Status string `json:"status"`
				} `json:"merges"`
			} `json:"result"`
		}
		if json.Unmarshal(raw, &r) == nil && r.Result != nil {
			for _, m := range r.Result.Merges {
				if m.Status != "unresolvable" {
					s.State.Completed[m.Unit] = true
				}
			}
		}
	}
	return raw, nil
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
