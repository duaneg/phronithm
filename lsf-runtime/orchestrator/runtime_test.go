package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

func stubOrch(t *testing.T, s *orchtest.Stub, seed map[string]json.RawMessage) *orch {
	t.Helper()
	a, err := parseArgs(s.Args())
	if err != nil {
		t.Fatal(err)
	}
	c, err := newRunCtx(a)
	if err != nil {
		t.Fatal(err)
	}
	return newOrch(s, c, seed)
}

func TestRequiredArgsInJSOrder(t *testing.T) {
	cases := []struct {
		args string
		want string
	}{
		{`{}`, "args.name is required"},
		{`null`, "args.name is required"},
		{`{"name":"f"}`, "args.planSha is required"},
		{`{"name":"f","planSha":""}`, "args.planSha is required"},
		{`{"name":"f","planSha":"p"}`, "args.architectureDoc is required"},
		{`{"name":"f","planSha":"p","architectureDoc":"d","integrationBranch":null}`, "args.integrationBranch is required"},
	}
	for _, c := range cases {
		a, err := parseArgs(json.RawMessage(c.args))
		if err != nil {
			t.Fatalf("%s: %v", c.args, err)
		}
		if _, err := newRunCtx(a); err == nil || err.Error() != "lsf-implement: "+c.want {
			t.Errorf("%s: %v, want %s", c.args, err, c.want)
		}
	}
	if _, err := parseArgs(json.RawMessage(`{"name":5}`)); err == nil {
		t.Error("a non-string name decodes (D12)")
	}
}

func TestDefaultsDeriveFromName(t *testing.T) {
	s := orchtest.New(t, orchtest.TwoWaves(), nil, nil)
	c := stubOrch(t, s, nil).ctx
	want := []string{"docs/plans/f-breakdown.md", "docs/architecture/f-brief.md", "docs/plans/f-decisions.md", "docs/plans/f-signals/plan0.md", "docs/plans/f-archive.md", "refs/lsf/f/pause"}
	got := []string{c.planPath, c.requestRecord, c.decisionLogPath, c.ledgerPath, c.archivePath, c.pauseRef}
	if !slices.Equal(got, want) || c.maxWaves != 20 || c.maxUnitsPerWave != nil || c.architectureSha.set {
		t.Errorf("%v %v %v %+v", got, c.maxWaves, c.maxUnitsPerWave, c.architectureSha)
	}
	if c.config.DistressDeviationThreshold != 4 {
		t.Errorf("config %+v", c.config)
	}
}

func TestTiersOverrideByRole(t *testing.T) {
	s := orchtest.New(t, orchtest.TwoWaves(), nil, orchtest.M{"tiers": orchtest.M{"critique": orchtest.M{"model": "sonnet"}, "gate": nil}})
	c := stubOrch(t, s, nil).ctx
	// A role that omits effort keeps the default's on unit stages, as
	// {...tier(default), ...tier(role)} does; alone it has none.
	if got := c.tierFor("default").over(c.tierFor("critique")); got != (tier{"sonnet", "low"}) {
		t.Errorf("unit critique tier %+v", got)
	}
	if got := c.tierFor("critique"); got != (tier{Model: "sonnet"}) {
		t.Errorf("critique tier %+v", got)
	}
	// A null role falls to default, as `?? ctx.tiers.default` does.
	if got := c.tierFor("gate"); got != (tier{"opus", "low"}) {
		t.Errorf("gate tier %+v", got)
	}
	if got := c.tierFor("mechanical"); got != (tier{"haiku", "low"}) {
		t.Errorf("mechanical tier %+v", got)
	}
}

func TestRepeatedLabelsGetSuffixes(t *testing.T) {
	s := orchtest.New(t, orchtest.TwoWaves(), nil, nil)
	o := stubOrch(t, s, nil)
	for range 3 {
		if _, _, err := o.agent(context.Background(), "p", lsfrt.Opts{Label: "ledger:w1"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, label, _ := o.agent(context.Background(), "p", lsfrt.Opts{Label: "ledger:w2"}); label != "ledger:w2" {
		t.Errorf("label %s", label)
	}
	if got := s.Labels(); !slices.Equal(got, []string{"ledger:w1", "ledger:w1#2", "ledger:w1#3", "ledger:w2"}) {
		t.Errorf("labels %v", got)
	}
}

func TestSeedStandsInOnEveryUseAndDropsPauseReport(t *testing.T) {
	s := orchtest.New(t, orchtest.TwoWaves(), nil, nil)
	seed := map[string]json.RawMessage{
		"merge:w1": json.RawMessage(`{"result":{"planSha":"s","pauseRequested":true},"commits":[]}`),
	}
	o := stubOrch(t, s, seed)
	for range 2 {
		cr, label, err := o.agent(context.Background(), "p", lsfrt.Opts{Label: "merge:w1"})
		if err != nil || label != "merge:w1" {
			t.Fatal(err, label)
		}
		r := decode[mergeResult](s, label, cr.Value)
		if r == nil || r.Result.PlanSha != "s" || r.Result.PauseRequested {
			t.Errorf("%s", cr.Value)
		}
	}
	if len(s.Calls) != 0 || len(s.Logs) != 2 || s.Logs[0] != "merge:w1: seeded from args.seed; agent not spawned" {
		t.Errorf("calls %v logs %v", s.Labels(), s.Logs)
	}
}

func TestDecode(t *testing.T) {
	s := orchtest.New(t, orchtest.TwoWaves(), nil, nil)
	if decode[unitPlanResult](s, "x", nil) != nil || decode[unitPlanResult](s, "x", json.RawMessage("null")) != nil {
		t.Error("null decodes")
	}
	r := decode[unitPlanResult](s, "plan:A", json.RawMessage(`{"result":{"branch":"b","bytes":3.0,"extra":1},"commits":[]}`))
	if r == nil || r.Result.Branch != "b" || r.Result.Bytes != 3 {
		t.Errorf("%+v", r)
	}
	for _, bad := range []string{`{"result":{"branch":5}}`, `{"result":{"bytes":2.5}}`, `{"result":{"bytes":"3"}}`} {
		if decode[unitPlanResult](s, "plan:A", json.RawMessage(bad)) != nil {
			t.Errorf("%s decodes", bad)
		}
	}
	if len(s.Logs) != 3 || !strings.HasPrefix(s.Logs[0], "plan:A: return does not decode (") || !strings.HasSuffix(s.Logs[0], "); treated as null") {
		t.Errorf("logs %q", s.Logs)
	}
}

func TestSchemasAreJSKeyOrdered(t *testing.T) {
	// Spot check against implement.js's objects; promptparity compares all.
	var m struct {
		Properties struct {
			Result struct {
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
			} `json:"result"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(mergeSchema, &m); err != nil {
		t.Fatal(err)
	}
	if got := m.Properties.Result.Required; !slices.Equal(got, []string{"integrationBranch", "merges", "planSha", "pauseRequested", "architectureSha"}) {
		t.Errorf("merge required %v", got)
	}
	if !strings.Contains(string(fixCodeSchema), `"notApplied":{"type":"array","items":{"type":"object","properties":{"finding":{"type":"string"},"reason":{"type":"string"}},"required":["finding","reason"]}},"gates":{"type":"array","items":`) {
		t.Errorf("fix-code schema %s", fixCodeSchema)
	}
}

func TestJSSlice(t *testing.T) {
	xs := []int{1, 2, 3}
	n := func(i int) *int { return &i }
	for _, c := range []struct {
		n    *int
		want []int
	}{{nil, xs}, {n(2), []int{1, 2}}, {n(0), []int{}}, {n(5), xs}, {n(-1), []int{1, 2}}, {n(-5), []int{}}} {
		if got := jsSlice(xs, c.n); !slices.Equal(got, c.want) {
			t.Errorf("%v: %v", c.n, got)
		}
	}
}

func TestMergeResultSchemaHasTwoProperties(t *testing.T) {
	// mergeResultSchema copies these by name; a new property must be added.
	var m struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(plan.MergeResultSchema, &m); err != nil || len(m.Properties) != 2 {
		t.Errorf("%v %v", m.Properties, err)
	}
}

func TestDecodeNullsAndMissingResult(t *testing.T) {
	s := orchtest.New(t, orchtest.TwoWaves(), nil, nil)
	for _, bad := range []string{`{"result":{"bytes":null}}`, `{"commits":[]}`, `{"result":null}`} {
		if decode[unitPlanResult](s, "plan:A", json.RawMessage(bad)) != nil {
			t.Errorf("%s decodes", bad)
		}
	}
}

// S7: the flags and denials a call returns become its driver-event
// signals at the call site: a plain call, a null call, and a parallel
// reviewer, including one whose branch returned null.
func TestCallResultReachesCallSites(t *testing.T) {
	flagged := func(v any) H {
		return func(Call, *State) any {
			cr := lsfrt.CallResult{Flags: []string{"skill-not-loaded"}, Denials: []string{"Bash: git push"}}
			if v != nil {
				b, _ := json.Marshal(v)
				cr.Value = b
			}
			return cr
		}
	}
	s := orchtest.New(t, orchtest.TwoWaves(), Hs{"implement": flagged(orchtest.Ret(orchtest.Implemented("A", nil))), "plan": flagged(nil),
		"critique-code": flagged(orchtest.NoFindings()), "lens-review": flagged(nil)}, nil)
	o := stubOrch(t, s, nil)
	ctx := context.Background()
	at := site{plan.At{Stage: "implement", Unit: "A", Wave: plan.W(1)}, true}
	shows := func(what, label string, sigs []*plan.Signal) {
		t.Helper()
		var got []string
		for _, sg := range sigs {
			got = append(got, sg.Kind+"@"+sg.Referent)
		}
		if want := []string{"skill-not-loaded@" + label, "permission-denied@" + label}; !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", what, got, want)
		}
	}

	r, sigs, err := call[implementResult](ctx, o, at, "p", lsfrt.Opts{Label: "implement:A"})
	if err != nil || r == nil {
		t.Fatal(r, err)
	}
	shows("plain call", "implement:A", sigs)
	r2, sigs, err := call[unitPlanResult](ctx, o, at, "p", lsfrt.Opts{Label: "plan:A"})
	if err != nil || r2 != nil {
		t.Fatal(r2, err)
	}
	shows("null call", "plan:A", sigs)
	res, err := o.parallelAgents(ctx, []agentReq{{"p", lsfrt.Opts{Label: "critique-code:r1:A"}}, {"p", lsfrt.Opts{Label: "lens-review:r1:A"}}})
	if err != nil {
		t.Fatal(err)
	}
	v0, s0 := o.callSigs(at, res[0].cr, res[0].label, nil)
	v1, s1 := o.callSigs(at, res[1].cr, res[1].label, nil)
	shows("parallel reviewer", "critique-code:r1:A", s0)
	shows("parallel null reviewer", "lens-review:r1:A", s1)
	if v0 == nil || v1 != nil {
		t.Errorf("%+v", res)
	}
}

// Calls and code steps share the label counter, so a label used by both
// stays launch-unique; a seed naming a step is logged and not used.
func TestStepSharesLabelCounterAndIgnoresSeed(t *testing.T) {
	s := orchtest.New(t, orchtest.TwoWaves(), nil, nil)
	o := stubOrch(t, s, map[string]json.RawMessage{"ledger:w1": json.RawMessage(`{"result":{"sha":"seeded"}}`)})
	ctx := context.Background()
	if _, _, err := o.runStep(ctx, "ledger:w2", M{"a": 1}, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := o.agent(ctx, "p", lsfrt.Opts{Label: "ledger:w2"}); err != nil {
		t.Fatal(err)
	}
	r, fault, err := step[scribeResult](ctx, o, "ledger:w1", M{}, nil)
	if err != nil || fault != "" || r == nil || r.Result.Sha != "l" {
		t.Fatalf("%+v %q %v: the seed stood in", r, fault, err)
	}
	eqs(t, s.Labels(), []string{"ledger:w2", "ledger:w2#2", "ledger:w1"})
	if !s.Calls[0].Step || s.Calls[1].Step || string(s.Input("ledger:w2")) != `{"a":1}` {
		t.Errorf("%+v", s.Calls)
	}
	eqs(t, s.Logs, []string{"ledger:w1: args.seed ignored; code stage"})
}

// A step's stop propagates; any other error is the stage's fault, unless
// the context has ended, when it is a cancellation.
func TestStepErrors(t *testing.T) {
	stop := &lsfrt.StopError{Reason: lsfrt.StopMaxCalls}
	s := orchtest.New(t, orchtest.TwoWaves(), Hs{
		"ledger":            func(Call, *State) any { return errors.New("git: boom") },
		"load":              func(Call, *State) any { return stop },
		"decision-log":      func(Call, *State) any { return errors.New("") },
		"completion-report": func(Call, *State) any { return fmt.Errorf("wrapped: %w", stop) },
	}, nil)
	o := stubOrch(t, s, nil)
	if _, fault, err := step[scribeResult](context.Background(), o, "ledger:w1", M{}, nil); err != nil || fault != "git: boom" {
		t.Errorf("fault %q err %v", fault, err)
	}
	if _, fault, err := step[copyResult](context.Background(), o, "load", M{}, nil); err != stop || fault != "" {
		t.Errorf("fault %q err %v", fault, err)
	}
	if _, fault, err := step[scribeResult](context.Background(), o, "decision-log", M{}, nil); err != nil || fault != "stage failed" {
		t.Errorf("empty reason: fault %q err %v", fault, err)
	}
	if _, fault, err := step[completionResult](context.Background(), o, "completion-report:A", M{}, nil); !errors.Is(err, stop) || fault != "" {
		t.Errorf("wrapped stop: fault %q err %v", fault, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, fault, err := step[scribeResult](ctx, o, "ledger:w1", M{}, nil)
	if r, ok := lsfrt.IsStop(err); !ok || r != lsfrt.StopCancelled || fault != "" {
		t.Errorf("fault %q err %v", fault, err)
	}
}
