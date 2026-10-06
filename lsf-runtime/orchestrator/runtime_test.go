package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

func newOrch(t *testing.T, s *orchtest.Stub, seed map[string]json.RawMessage) *orch {
	t.Helper()
	a, err := parseArgs(s.Args())
	if err != nil {
		t.Fatal(err)
	}
	c, err := newRunCtx(a)
	if err != nil {
		t.Fatal(err)
	}
	return &orch{rt: s, ctx: c, seed: seed, uses: map[string]int{}}
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
	c := newOrch(t, s, nil).ctx
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
	c := newOrch(t, s, nil).ctx
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
	o := newOrch(t, s, nil)
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
	o := newOrch(t, s, seed)
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
	want := `{"type":"object","properties":{"result":{"type":"object","properties":{"path":{"type":"string"},"sha":{"type":"string","description":"The commit SHA as ` +
		"`git rev-parse --short HEAD`" + ` prints it after the commit, copied from that output"},"pauseRequested":{"type":"boolean","description":"Whether the pause ref named in the prompt exists"}},"required":["path","sha","pauseRequested"]},"commits":{"type":"array","items":{"type":"object","properties":{"branch":{"type":"string"},"sha":{"type":"string"}},"required":["branch","sha"]}}},"required":["result","commits"]}`
	if string(ledgerSchema) != want {
		t.Errorf("ledger schema\n%s\nwant\n%s", ledgerSchema, want)
	}
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

// S4: the flags and denials a call returns reach the call site with its
// value: a plain call, a null call, and a parallel reviewer.
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
	o := newOrch(t, s, nil)
	ctx := context.Background()
	shows := func(what string, cr lsfrt.CallResult) {
		t.Helper()
		if !slices.Equal(cr.Flags, []string{"skill-not-loaded"}) || !slices.Equal(cr.Denials, []string{"Bash: git push"}) {
			t.Errorf("%s: %+v", what, cr)
		}
	}

	r, cr, err := call[implementResult](ctx, o, "p", lsfrt.Opts{Label: "implement:A"})
	if err != nil || r == nil {
		t.Fatal(r, err)
	}
	shows("plain call", cr)
	r2, cr, err := call[unitPlanResult](ctx, o, "p", lsfrt.Opts{Label: "plan:A"})
	if err != nil || r2 != nil {
		t.Fatal(r2, err)
	}
	shows("null call", cr)
	res, err := o.parallelAgents(ctx, []agentReq{{"p", lsfrt.Opts{Label: "critique-code:r1:A"}}, {"p", lsfrt.Opts{Label: "lens-review:r1:A"}}})
	if err != nil {
		t.Fatal(err)
	}
	shows("parallel reviewer", res[0].cr)
	shows("parallel null reviewer", res[1].cr)
	if res[0].cr.Value == nil || res[1].cr.Value != nil || res[0].label != "critique-code:r1:A" {
		t.Errorf("%+v", res)
	}
}

// Calls and code steps share the label counter, so a label used by both
// stays launch-unique; a seed naming a step is logged and not used.
func TestStepSharesLabelCounterAndIgnoresSeed(t *testing.T) {
	s := orchtest.New(t, orchtest.TwoWaves(), nil, nil)
	o := newOrch(t, s, map[string]json.RawMessage{"ledger:w1": json.RawMessage(`{"result":{"sha":"seeded"}}`)})
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
		"ledger": func(Call, *State) any { return errors.New("git: boom") },
		"load":   func(Call, *State) any { return stop },
	}, nil)
	o := newOrch(t, s, nil)
	if _, fault, err := step[scribeResult](context.Background(), o, "ledger:w1", M{}, nil); err != nil || fault != "git: boom" {
		t.Errorf("fault %q err %v", fault, err)
	}
	if _, fault, err := step[copyResult](context.Background(), o, "load", M{}, nil); err != stop || fault != "" {
		t.Errorf("fault %q err %v", fault, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, fault, err := step[scribeResult](ctx, o, "ledger:w1", M{}, nil)
	if r, ok := lsfrt.IsStop(err); !ok || r != lsfrt.StopCancelled || fault != "" {
		t.Errorf("fault %q err %v", fault, err)
	}
}
