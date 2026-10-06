package orchestrator

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

type (
	H     = orchtest.Handler
	Call  = orchtest.Call
	State = orchtest.State
	Hs    = map[string]orchtest.Handler
)

var R = orchtest.Ret

// view is the launch's return as the tests read it.
type view struct {
	Status     string `json:"status"`
	Workflow   string `json:"workflow"`
	Escalation *struct {
		Workflow       string                `json:"workflow"`
		Stage          string                `json:"stage"`
		Signals        []plan.Signal         `json:"signals"`
		DecisionNeeded []plan.DecisionNeeded `json:"decision_needed"`
		PlanSha        string                `json:"plan_sha"`
		PreReplanSha   *string               `json:"pre_replan_sha"`
		Wave           *int                  `json:"wave"`
	} `json:"escalation"`
	Digest struct {
		Waves         int            `json:"waves"`
		ByKind        map[string]int `json:"byKind"`
		BySource      map[string]int `json:"bySource"`
		ByDisposition map[string]int `json:"byDisposition"`
		Attention     struct {
			ArchitectureNotes      []string       `json:"architectureNotes"`
			ArchitectureAmendments []string       `json:"architectureAmendments"`
			UnfixedSignificant     map[string]int `json:"unfixedSignificant"`
			MinorFindings          map[string]int `json:"minorFindings"`
			DistressedUnits        []string       `json:"distressedUnits"`
			DirectEdits            []string       `json:"directEdits"`
		} `json:"attention"`
		Note string `json:"note"`
	} `json:"digest"`
	PlanSha           string        `json:"planSha"`
	IntegrationBranch string        `json:"integrationBranch"`
	Waves             int           `json:"waves"`
	LedgerPath        string        `json:"ledgerPath"`
	Unscribed         []plan.Signal `json:"unscribed"`
	Note              string        `json:"note"`
	raw               string
}

// stub is a stub runtime over p with handler overrides and extra args.
func stub(t *testing.T, p *plan.Plan, h Hs, args M) *orchtest.Stub {
	t.Helper()
	return orchtest.New(t, p, h, args)
}

// run runs the orchestrator on s and decodes its return.
func run(t *testing.T, s *orchtest.Stub) view {
	t.Helper()
	out, err := Run(context.Background(), s)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var v view
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("outcome %s: %v", out, err)
	}
	v.raw = string(out)
	return v
}

func (v view) stops() []plan.Signal {
	if v.Escalation == nil {
		return nil
	}
	return v.Escalation.Signals
}

func (v view) stopKinds() []string { return kindsOf(v.stops()) }

func kindsOf(sigs []plan.Signal) []string {
	out := []string{}
	for _, s := range sigs {
		out = append(out, s.Kind)
	}
	return out
}

func eq[T comparable](t *testing.T, got, want T, msg ...any) {
	t.Helper()
	if got != want {
		t.Errorf("%v: got %v, want %v", msg, got, want)
	}
}

func eqs[T comparable](t *testing.T, got, want []T, msg ...any) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%v: got %v, want %v", msg, got, want)
	}
}

func match(t *testing.T, s, re string, msg ...any) {
	t.Helper()
	if !regexp.MustCompile(re).MatchString(s) {
		t.Errorf("%v: %q does not match /%s/", msg, trunc(s), re)
	}
}

func noMatch(t *testing.T, s, re string, msg ...any) {
	t.Helper()
	if regexp.MustCompile(re).MatchString(s) {
		t.Errorf("%v: %q matches /%s/", msg, trunc(s), re)
	}
}

func contains(t *testing.T, s, sub string, msg ...any) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("%v: %q does not contain %q", msg, trunc(s), sub)
	}
}

func trunc(s string) string {
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

// agentCalls are s's agent calls, without its code steps.
func agentCalls(s *orchtest.Stub) []Call {
	out := []Call{}
	for _, c := range s.Calls {
		if !c.Step {
			out = append(out, c)
		}
	}
	return out
}

// input is the decoded input of the step labelled l.
func input(t *testing.T, s *orchtest.Stub, l string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(s.Input(l), &m); err != nil {
		t.Fatalf("%s: input: %v", l, err)
	}
	return m
}

func has(labels []string, l string) bool { return slices.Contains(labels, l) }

func hasPrefix(labels []string, p string) bool {
	return slices.ContainsFunc(labels, func(l string) bool { return strings.HasPrefix(l, p) })
}

func filterPrefix(labels []string, p string) []string {
	out := []string{}
	for _, l := range labels {
		if strings.HasPrefix(l, p) {
			out = append(out, l)
		}
	}
	return out
}

func idx(labels []string, l string) int { return slices.Index(labels, l) }

// before is labels.indexOf(a) < labels.indexOf(b), both present.
func before(t *testing.T, labels []string, a, b string) {
	t.Helper()
	ia, ib := idx(labels, a), idx(labels, b)
	if ia < 0 || ib < 0 || ia >= ib {
		t.Errorf("%s (%d) not before %s (%d) in %v", a, ia, b, ib, labels)
	}
}

func critical1(loc string) M {
	return R(M{"findings": []plan.Finding{orchtest.Finding("Critical", "bad", loc)}, "systemic": false, "summary": "s"})
}

// criticalOnce is a review whose round 1 raises one Critical and passes
// afterwards.
func criticalOnce() H {
	round := 0
	return func(Call, *State) any {
		round++
		if round == 1 {
			return critical1("a.py")
		}
		return orchtest.NoFindings()
	}
}

func implemented(over M) H {
	return func(c Call, s *State) any { return R(orchtest.Implemented(c.Unit, over)) }
}

func gates(gs ...orchtest.Gate) []orchtest.Gate { return gs }

func g(cmd, status, summary string) orchtest.Gate {
	return orchtest.Gate{Command: cmd, Status: status, Summary: summary}
}

func one(id string, typ string) *plan.Plan {
	return orchtest.Graph("plan0", orchtest.Typed(orchtest.Unit(id), typ))
}

// schemaOf decodes a call's schema.
func schemaOf(t *testing.T, c *Call) struct {
	Properties struct {
		Result struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		} `json:"result"`
	} `json:"properties"`
} {
	t.Helper()
	var s struct {
		Properties struct {
			Result struct {
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
			} `json:"result"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(c.Opts.Schema, &s); err != nil {
		t.Fatal(err)
	}
	return s
}
