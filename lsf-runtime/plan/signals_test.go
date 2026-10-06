package plan

import (
	"encoding/json"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func kinds(sigs []*Signal) []string {
	out := []string{}
	for _, s := range sigs {
		out = append(out, s.Kind)
	}
	return out
}

// js: signalsFromReadings is one count row per unit pointing at the plan's Readings section
func TestSignalsFromReadings(t *testing.T) {
	sigs := SignalsFromReadings(2, At{Unit: "B", Wave: W(1), Path: "docs/plans/B-plan.md"})
	if len(sigs) != 1 {
		t.Fatalf("%v", sigs)
	}
	s := sigs[0]
	if s.Kind != "reading" || s.Source != "reported" || s.Stage != "plan" || s.Referent != "docs/plans/B-plan.md" ||
		s.Statement != "2 reading(s); see docs/plans/B-plan.md Readings" {
		t.Errorf("%+v", s)
	}
	if got := SignalsFromReadings(0, At{Unit: "B", Wave: W(1), Path: "p"}); len(got) != 0 {
		t.Errorf("none: %v", got)
	}
}

// js: signalsFromDocumentSize records a document past twice its target and nothing else
func TestSignalsFromDocumentSize(t *testing.T) {
	at := At{Unit: "B", Wave: W(1), Stage: "plan", Path: "docs/plans/B-plan.md", TargetKB: 12}
	if OversizeFactor != 2 {
		t.Fatal(OversizeFactor)
	}
	for _, b := range []float64{12 * 1024, 24 * 1024, math.NaN(), math.Inf(1)} {
		if got := SignalsFromDocumentSize(b, at); len(got) != 0 {
			t.Errorf("%v: %v", b, got)
		}
	}
	sigs := SignalsFromDocumentSize(30*1024+100, at)
	if len(sigs) != 1 {
		t.Fatalf("%v", sigs)
	}
	s := sigs[0]
	if s.Kind != "document-oversize" || s.Source != "structural" || s.Stage != "plan" || s.Unit != "B" || s.Referent != "docs/plans/B-plan.md" ||
		s.Statement != "docs/plans/B-plan.md is 30 KB against a 12 KB target" {
		t.Errorf("%+v", s)
	}
	rep := SignalsFromDocumentSize(20*1024, At{Unit: "B", Wave: W(1), Stage: "completion-report", Path: "docs/plans/B-completion.md", TargetKB: 8})
	if rep[0].Stage != "completion-report" || rep[0].Statement != "docs/plans/B-completion.md is 20 KB against a 8 KB target" {
		t.Errorf("%+v", rep[0])
	}
}

// js: signalsFromImplement records deviations, writes outside Scope, deferrals, and discoveries
func TestSignalsFromImplement(t *testing.T) {
	sigs := SignalsFromImplement(ImplementResult{
		Deviations:         []string{"added a sibling predicate instead of widening"},
		WritesOutsideScope: []string{"CLAUDE.md", "tests/stubs.py"},
		Deferrals:          []Deferral{{Statement: "export path left unrotated", BelongsTo: "render-export"}},
		Discoveries:        []Discovery{{Statement: "loophole-compound has no edge to render-export", Affects: "loophole-compound"}},
	}, At{Unit: "B", Wave: W(2)})
	eqStrings(t, "kinds", kinds(sigs), []string{"deviation", "deviation", "deferral", "discovery"})
	for _, s := range sigs {
		if s.Source != "reported" || s.Stage != "implement" || s.Unit != "B" || *s.Wave != 2 {
			t.Errorf("%+v", s)
		}
	}
	if !regexp.MustCompile(`writes outside the Scope line: CLAUDE\.md, tests/stubs\.py`).MatchString(sigs[1].Statement) {
		t.Errorf("%q", sigs[1].Statement)
	}
	if sigs[2].Referent != "render-export" || sigs[3].Referent != "loophole-compound" {
		t.Errorf("referents %q %q", sigs[2].Referent, sigs[3].Referent)
	}
	if got := SignalsFromImplement(ImplementResult{}, At{Unit: "B", Wave: W(2)}); len(got) != 0 {
		t.Errorf("empty: %v", got)
	}
}

// js: signalsFromFindings: Significant one row each, Minor one count row with up to five locations, Critical skipped
func TestSignalsFromFindings(t *testing.T) {
	findings := []Finding{
		{Severity: "Critical", Statement: "wrong", Location: "a.py:1", Stage: "critique-code", Round: 1},
		{Severity: "Significant", Statement: "fragile", Location: "a.py:2", Stage: "critique-code", Round: 2},
	}
	for _, i := range []string{"10", "11", "12", "13", "14", "15", "16"} {
		findings = append(findings, Finding{Severity: "Minor", Statement: "naming " + i, Location: "a.py:" + i, Stage: "lens-review", Round: 1})
	}
	sigs := SignalsFromFindings(findings, At{Stage: "review", Unit: "B", Wave: W(1)})
	var rows [][4]string
	for _, s := range sigs {
		rows = append(rows, [4]string{s.Kind, s.Source, s.Stage, s.Referent})
	}
	want := [][4]string{
		{"finding", "gate", "critique-code", "a.py:2"},
		{"finding", "gate", "review", "a.py:10, a.py:11, a.py:12, a.py:13, a.py:14"},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows %v", rows)
	}
	if sigs[0].Statement != "Significant (round 2): fragile" || sigs[1].Statement != "7 Minor finding(s)" || strings.Contains(sigs[1].Statement, "naming") {
		t.Errorf("statements %q %q", sigs[0].Statement, sigs[1].Statement)
	}
	gateStage := SignalsFromFindings([]Finding{{Severity: "Significant", Statement: "s", Location: "l"}}, At{Stage: "critique-plan", Unit: "B", Wave: W(1)})
	if len(gateStage) != 1 || gateStage[0].Stage != "critique-plan" {
		t.Errorf("a finding without a stage takes the gate's: %+v", gateStage)
	}
	if got := SignalsFromFindings(nil, At{Stage: "review", Unit: "B", Wave: W(1)}); len(got) != 0 {
		t.Errorf("empty: %v", got)
	}
}

func inv(f func(*Invalidation)) Invalidation {
	i := Invalidation{
		Section: "§4.8", Decision: "two-tone families are two-part groups", Premise: "nested art classifies to a group",
		Evidence: "svgpath.py:120; brazier parses to a bare Glyph", Amendment: "nested art must be authored as siblings, or _classify keeps ids on holes",
	}
	if f != nil {
		f(&i)
	}
	return i
}

// js: signalsFromGate: a complete invalidation stops; an incomplete one and every note is recorded
func TestSignalsFromGate(t *testing.T) {
	if !InvalidationComplete(inv(nil)) || InvalidationComplete(inv(func(i *Invalidation) { i.Amendment = "" })) ||
		InvalidationComplete(inv(func(i *Invalidation) { i.Evidence = "  " })) {
		t.Error("InvalidationComplete")
	}
	sigs := SignalsFromGate(GateResult{
		Invalidations: []Invalidation{inv(nil), inv(func(i *Invalidation) { i.Amendment = "" })},
		Notes:         []Note{{Statement: "N3 trigger fired, no owner", Referent: "architecture §6 N3"}},
	}, At{Wave: W(1)})
	eqStrings(t, "kinds", kinds(sigs), []string{"architecture-invalidated", "architecture-note", "architecture-note"})
	for _, s := range sigs {
		if s.Source != "gate" || s.Stage != "review-gate" || *s.Wave != 1 {
			t.Errorf("%+v", s)
		}
	}
	if sigs[0].Referent != "§4.8" ||
		!regexp.MustCompile(`section: §4\.8; decision: .*; premise: .*; evidence: .*; amendment: nested art`).MatchString(sigs[0].Statement) ||
		!strings.HasPrefix(sigs[1].Statement, "incomplete invalidation (amendment missing)") ||
		sigs[2].Referent != "architecture §6 N3" {
		t.Errorf("%+v", sigs)
	}
	ev := EvaluateWave(sigs)
	if !ev.Stop {
		t.Error("no stop")
	}
	eqStrings(t, "stops", kinds(ev.Stops), []string{"architecture-invalidated"})
	for _, r := range ev.Records {
		if r.Disposition != "recorded" {
			t.Errorf("%+v", r)
		}
	}
}

// js: signalsFromGate: a unit's amendment to the architecture document is a record carrying the unit, listed in the digest
func TestSignalsFromGateAmendment(t *testing.T) {
	sigs := SignalsFromGate(GateResult{
		Invalidations: []Invalidation{inv(nil)},
		Notes:         []Note{{Statement: "N3 trigger fired, no owner", Referent: "architecture §6 N3"}},
		Amendments:    []Amendment{{Unit: "abi-design", Section: "§7.2", Statement: "records the ABI disclosure decision the unit settled"}},
	}, At{Wave: W(2)})
	eqStrings(t, "kinds", kinds(sigs), []string{"architecture-invalidated", "architecture-note", "architecture-amendment"})
	a := sigs[2]
	if a.Source != "gate" || a.Stage != "review-gate" || a.Unit != "abi-design" || a.Referent != "§7.2" ||
		a.Statement != "abi-design: records the ABI disclosure decision the unit settled" {
		t.Errorf("%+v", a)
	}
	ev := EvaluateWave(sigs)
	eqStrings(t, "stops", kinds(ev.Stops), []string{"architecture-invalidated"})
	eqStrings(t, "amendments", Digest(ev.All).Attention.ArchitectureAmendments, []string{"abi-design: records the ABI disclosure decision the unit settled"})
	// A statement already prefixed with the unit is not prefixed twice.
	again := SignalsFromGate(GateResult{Amendments: []Amendment{{Unit: "u", Statement: "u: s"}, {Statement: "only"}}}, At{})
	if again[0].Statement != "u: s" || again[1].Statement != "only" || again[1].Unit != "" {
		t.Errorf("%+v %+v", again[0], again[1])
	}
}

// js: nullReturnSignal is a structural STOP
func TestNullReturnSignal(t *testing.T) {
	s := NullReturnSignal(At{Stage: "review", Unit: "C", Wave: W(1)})
	if s.Kind != "agent-null" || !EvaluateWave([]*Signal{s}).Stop {
		t.Errorf("%+v", s)
	}
}

// js: signalsFromMerge distinguishes resolved from unresolvable conflicts
func TestSignalsFromMerge(t *testing.T) {
	sigs := SignalsFromMerge(MergeResult{IntegrationBranch: "lsf/x", Merges: []Merge{
		{Unit: "B", Status: "clean", Files: []string{}},
		{Unit: "C", Status: "resolved", Files: []string{"src/shared.js"}},
		{Unit: "D", Status: "unresolvable", Files: []string{"src/shared.js"}},
		{Unit: "E", Status: "regressed", Files: []string{"test/e.test.js"}},
	}}, At{Wave: W(2)})
	eqStrings(t, "kinds", kinds(sigs), []string{"claimed-disjoint-conflict", "merge-conflict-unresolvable", "merge-regression"})
	for _, s := range sigs {
		if s.Source != "structural" {
			t.Errorf("%+v", s)
		}
	}
	eqStrings(t, "records", kinds(EvaluateWave(sigs).Records), []string{"claimed-disjoint-conflict"})
}

// js: distressSignals is a record on any breakdown distress signal and quiet otherwise
func TestDistressSignals(t *testing.T) {
	at := At{Unit: "B", Wave: W(1)}
	calm := Distress{MidImplementationDesignDecisions: 0, Deviations: 1, Deferrals: 1}
	if got := DistressSignals(calm, at, DefaultConfig); len(got) != 0 {
		t.Errorf("calm %v", got)
	}
	for _, c := range []Distress{
		{ContextNearExhaustion: true, Deviations: 1, Deferrals: 1},
		{MidImplementationDesignDecisions: 3, Deviations: 1, Deferrals: 1},
		{Deviations: 2, Deferrals: 2},
	} {
		sigs := DistressSignals(c, at, DefaultConfig)
		if len(sigs) != 1 || sigs[0].Kind != "execution-distress" || EvaluateWave(sigs).Stop {
			t.Errorf("%+v: %v", c, sigs)
		}
	}
	cfg, err := ConfigFrom(json.RawMessage(`{"distressDeviationThreshold":10}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := DistressSignals(Distress{Deviations: 3, Deferrals: 1}, at, cfg); len(got) != 0 {
		t.Errorf("raised threshold %v", got)
	}
	if got := DistressSignals(Distress{MidImplementationDesignDecisions: 2, Deviations: 1, Deferrals: 1}, at, DefaultConfig); len(got) != 0 {
		t.Errorf("two design decisions are ordinary: %v", got)
	}
}

func TestConfigFromKeepsDefaults(t *testing.T) {
	cfg, err := ConfigFrom(json.RawMessage(`{"distressDeviationThreshold":10}`))
	if err != nil || cfg != (Config{DistressDeviationThreshold: 10, DistressDesignDecisionThreshold: 3}) {
		t.Errorf("partial: %+v %v", cfg, err)
	}
	cfg, err = ConfigFrom(json.RawMessage(`{"distressDesignDecisionThreshold":0}`))
	if err != nil || cfg != (Config{DistressDeviationThreshold: 4, DistressDesignDecisionThreshold: 0}) {
		t.Errorf("explicit zero: %+v %v", cfg, err)
	}
	if cfg, err := ConfigFrom(nil); err != nil || cfg != DefaultConfig {
		t.Errorf("nil: %+v %v", cfg, err)
	}
}

// js: the stop rule reads kind alone: every STOP kind stops, every RECORD kind is recorded, and re-plan items are records
func TestStopRuleKindAlone(t *testing.T) {
	for _, k := range StopKinds {
		if !EvaluateWave([]*Signal{NewSignal(Signal{Kind: k, Source: "structural", Stage: "s", Statement: k})}).Stop {
			t.Errorf("%s does not stop", k)
		}
	}
	for _, k := range RecordKinds {
		if EvaluateWave([]*Signal{NewSignal(Signal{Kind: k, Source: "reported", Stage: "s", Statement: k})}).Stop {
			t.Errorf("%s stops", k)
		}
	}
	for _, k := range ReplanItemKinds {
		if !slices.Contains(RecordKinds, k) {
			t.Errorf("%s is not a record kind", k)
		}
	}
	empty := EvaluateWave(nil)
	if empty.Stop || empty.All == nil || len(empty.All) != 0 {
		t.Errorf("%+v", empty)
	}
}

func TestEvaluateWaveKeepsIdentity(t *testing.T) {
	a := NewSignal(Signal{Kind: "agent-null", Source: "structural", Stage: "s"})
	b := NewSignal(Signal{Kind: "reading", Source: "reported", Stage: "s"})
	ledger := []*Signal{a, b}
	ev := EvaluateWave(ledger)
	if ev.Stops[0] != a || ev.Records[0] != b || ev.All[0] != a || a.Disposition != "stopped" || b.Disposition != "recorded" {
		t.Errorf("%+v", ev)
	}
	ev.All[0] = b
	if ledger[0] != a {
		t.Error("All aliases the caller's slice")
	}
}

// js: renderLedger is one table under a wave heading, one line per row, pipes and newlines neutralised
func TestRenderLedger(t *testing.T) {
	rows := []*Signal{
		{Wave: W(2), Unit: "B", Stage: "plan", Source: "reported", Kind: "reading", Statement: "a | b\nc", Referent: "x", Disposition: "recorded"},
		{Stage: "merge", Source: "structural", Kind: "merge-regression", Statement: "s", Referent: "B", Disposition: "stopped"},
	}
	lines := strings.Split(RenderLedger(rows, 2), "\n")
	if lines[0] != "## Wave 2" ||
		lines[2] != "| Wave | Unit | Stage | Source | Kind | Statement | Referent | Disposition |" ||
		lines[4] != `| 2 | B | plan | reported | reading | a \| b c | x | recorded |` ||
		lines[5] != "| 2 | — | merge | structural | merge-regression | s | B | stopped |" ||
		len(lines) != 7 {
		t.Errorf("%q", lines)
	}
	if !strings.HasPrefix(LedgerHeading("f"), "# Signal ledger: f\n") {
		t.Error(LedgerHeading("f"))
	}
	if got := ledgerCell("a\r\n\r\nb\rc  "); got != "a  b\rc" {
		t.Errorf("CRLF cell %q", got)
	}
}

// js: renderLedger heads a schedule stop with the wave its rows name, and a mixed append with the loop wave
func TestRenderLedgerWaveHeading(t *testing.T) {
	stop := &Signal{Wave: W(3), Unit: "C", Stage: "schedule", Source: "structural", Kind: "no-ready-unit", Statement: "s", Referent: "C", Disposition: "stopped"}
	if h := strings.Split(RenderLedger([]*Signal{stop}, 2), "\n")[0]; h != "## Wave 3" {
		t.Error(h)
	}
	if LedgerWave([]*Signal{stop}, 2) != 3 {
		t.Error("LedgerWave")
	}
	rec := &Signal{Wave: W(2), Unit: "B", Stage: "plan", Source: "reported", Kind: "reading", Statement: "r", Referent: "x", Disposition: "recorded"}
	if h := strings.Split(RenderLedger([]*Signal{rec, stop}, 2), "\n")[0]; h != "## Wave 2" {
		t.Error(h)
	}
	if h := strings.Split(RenderLedger(nil, 2), "\n")[0]; h != "## Wave 2" {
		t.Error(h)
	}
}

// js: ledgerLabel is termination when no row carries a wave, and the wave otherwise
func TestLedgerLabel(t *testing.T) {
	term := &Signal{Stage: "obligations", Source: "reported", Kind: "standing-obligation-carried", Statement: "s", Referent: "debt.md", Disposition: "recorded"}
	rec := &Signal{Wave: W(2), Unit: "B", Stage: "plan", Source: "reported", Kind: "reading", Statement: "r", Referent: "x", Disposition: "recorded"}
	for _, c := range []struct {
		entries []*Signal
		want    string
	}{
		{[]*Signal{term}, "termination"},
		{[]*Signal{rec, term}, "wave 2"},
		{[]*Signal{rec}, "wave 2"},
		{nil, "wave 2"},
	} {
		if got := LedgerLabel(c.entries, 2); got != c.want {
			t.Errorf("got %q want %q", got, c.want)
		}
	}
	if h := strings.Split(RenderLedger([]*Signal{term}, 2), "\n")[0]; h != "## Termination" {
		t.Error(h)
	}
}

// js: digest counts by kind, source and disposition, and lists what needs attention
func TestDigest(t *testing.T) {
	rec := func(kind string, s Signal) *Signal {
		s.Kind, s.Source, s.Disposition = kind, "reported", "recorded"
		if s.Stage == "" {
			s.Stage = "implement"
		}
		if s.Statement == "" {
			s.Statement = kind
		}
		return NewSignal(s)
	}
	gate := func(s Signal) *Signal {
		s.Kind, s.Source, s.Disposition = "finding", "gate", "recorded"
		return NewSignal(s)
	}
	ledger := []*Signal{
		rec("reading", Signal{Unit: "A", Wave: W(1), Stage: "plan"}),
		gate(Signal{Stage: "critique-code", Unit: "A", Wave: W(1), Statement: "Significant: x"}),
		gate(Signal{Stage: "review", Unit: "A", Wave: W(1), Statement: "3 Minor finding(s)", Referent: "a.py:1, a.py:2, a.py:3"}),
		gate(Signal{Stage: "critique-code", Unit: "B", Wave: W(2), Statement: "Significant: z"}),
		NewSignal(Signal{Kind: "architecture-note", Source: "gate", Stage: "review-gate", Wave: W(2), Statement: "N3 has no owner", Referent: "§6", Disposition: "recorded"}),
		rec("execution-distress", Signal{Unit: "B", Wave: W(2)}),
		rec("execution-distress", Signal{Unit: "B", Wave: W(2)}),
		rec("verdict", Signal{Stage: "decision-replan", Wave: W(0), Statement: "d1 amended"}),
		rec("direct-edit", Signal{Stage: "replan", Wave: W(2), Statement: "note the figure", Referent: "docs/debt.md"}),
	}
	d := Digest(ledger)
	if d.Waves != 2 {
		t.Errorf("wave 0 (decision re-plan) is not counted: %d", d.Waves)
	}
	if d.ByKind.Get("finding") != 3 {
		t.Errorf("byKind %v", d.ByKind.Map())
	}
	if !reflect.DeepEqual(d.BySource.Map(), map[string]int{"reported": 5, "gate": 4}) || !reflect.DeepEqual(d.ByDisposition.Map(), map[string]int{"recorded": 9}) {
		t.Errorf("bySource %v byDisposition %v", d.BySource.Map(), d.ByDisposition.Map())
	}
	a := d.Attention
	eqStrings(t, "notes", a.ArchitectureNotes, []string{"N3 has no owner"})
	eqStrings(t, "amendments", a.ArchitectureAmendments, []string{})
	if !reflect.DeepEqual(a.UnfixedSignificant.Map(), map[string]int{"A": 1, "B": 1}) || !reflect.DeepEqual(a.MinorFindings.Map(), map[string]int{"A": 3}) {
		t.Errorf("significant %v minor %v", a.UnfixedSignificant.Map(), a.MinorFindings.Map())
	}
	eqStrings(t, "distressed", a.DistressedUnits, []string{"B"})
	eqStrings(t, "edits", a.DirectEdits, []string{"docs/debt.md — note the figure"})
	if !strings.Contains(d.Note, "not evidence") {
		t.Error(d.Note)
	}
	js, _ := MarshalJS(d.BySource, "")
	if string(js) != `{"reported":5,"gate":4}` {
		t.Errorf("Counts order %s", js)
	}
}

func TestCountsJSKeyOrderAndUndefined(t *testing.T) {
	ledger := []*Signal{
		{Kind: "finding", Source: "gate", Unit: "u", Statement: "Significant: a"},
		{Kind: "finding", Source: "gate", Unit: "10", Statement: "Significant: a"},
		{Kind: "finding", Source: "gate", Unit: "2", Statement: "Significant: a"},
		{Kind: "finding", Source: "gate", Statement: "Significant: a"},
	}
	d := Digest(ledger)
	js, _ := MarshalJS(d.Attention.UnfixedSignificant, "")
	if string(js) != `{"2":1,"10":1,"u":1,"undefined":1}` {
		t.Errorf("unfixed %s", js)
	}
	if js, _ := MarshalJS(d.ByDisposition, ""); string(js) != `{"undefined":4}` {
		t.Errorf("byDisposition %s", js)
	}
}

// js: only an architecture invalidation supersedes the plan; an unevaluable done condition is a decision for the decision re-plan
func TestPlanSupersedingStopKinds(t *testing.T) {
	eqStrings(t, "superseding", PlanSupersedingStopKinds, []string{"architecture-invalidated"})
}
