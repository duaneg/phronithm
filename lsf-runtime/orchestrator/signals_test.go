package orchestrator

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// withCall is the default handler for prefix (or h), its value carried in
// a CallResult with extra's driver events.
func withCall(prefix string, h H, extra lsfrt.CallResult) H {
	if h == nil {
		h = orchtest.Defaults()[prefix]
	}
	return func(c Call, s *State) any {
		cr := extra
		if v := h(c, s); v != nil {
			b, err := json.Marshal(v)
			if err != nil {
				panic(err)
			}
			cr.Value = b
		}
		return cr
	}
}

var (
	dirty     = lsfrt.CallResult{Uncommitted: []string{"src/a.go"}}
	unchecked = lsfrt.CallResult{Flags: []string{"uncommitted-unchecked"}}
	noSkill   = lsfrt.CallResult{Flags: []string{"skill-not-loaded"}}
	denied    = lsfrt.CallResult{Denials: []string{"Bash: git commit -m x"}}
	guardKill = lsfrt.CallResult{Flags: []string{"guard-killed-after-result"}}
)

// S1, S3: each driver event at implement (a writer, a unit stage) reaches
// the ledger as its kind; the stop kinds abort the unit before review,
// keeping the implement stage's own records; the record kinds do not stop.
func TestDriverEventsAtImplement(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cr    lsfrt.CallResult
		kind  string
		stops bool
	}{
		{"uncommitted", dirty, "uncommitted-at-return", true},
		{"unchecked", unchecked, "uncommitted-at-return", true},
		{"skill", noSkill, "skill-not-loaded", true},
		{"denial", denied, "permission-denied", false},
		{"guard", guardKill, "guard-killed-after-result", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			impl := implemented(M{"deviations": []string{"kept the old name"}})
			s := stub(t, twoWaves(), Hs{"implement": withCall("implement", impl, tc.cr)}, nil)
			out := run(t, s)
			ledger := ledgerText(t, s, "ledger:w1")
			contains(t, ledger, "| "+tc.kind+" |", "ledger row")
			contains(t, ledger, "| implement:A |", "referent is the call's label")
			if !tc.stops {
				eq(t, out.Status, "complete", out.stopKinds())
				return
			}
			eq(t, out.Status, "stopped")
			eqs(t, out.stopKinds(), []string{tc.kind})
			if hasPrefix(s.Labels(), "critique-code") || hasPrefix(s.Labels(), "lens-review") {
				t.Errorf("reviewed after a stop: %v", s.Labels())
			}
			contains(t, ledger, "kept the old name", "the implement stage's deviation is kept")
		})
	}
}

// A reader's uncommitted changes are a record, not a stop.
func TestReaderResidueRecords(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"critique-code": withCall("critique-code", nil, dirty)}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	contains(t, ledgerText(t, s, "ledger:w1"), "| residue-at-return |")
}

// A null call keeps its driver events beside agent-null.
func TestNullCallKeepsDriverEvents(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"plan": func(Call, *State) any { return lsfrt.CallResult{Flags: []string{"skill-not-loaded"}} }}, nil)
	out := run(t, s)
	eqs(t, out.stopKinds(), []string{"skill-not-loaded", "agent-null"})
}

// The parallel review batch, a fix stage, the wave gate and cleanup each
// pass their CallResult to the ledger.
func TestDriverEventsAtEverySiteKind(t *testing.T) {
	s := stub(t, twoWaves(), Hs{
		"lens-review": withCall("lens-review", nil, denied),
		"review-gate": withCall("review-gate", nil, denied),
		"cleanup":     withCall("cleanup", nil, dirty),
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	l := ledgerText(t, s, "ledger:w1")
	contains(t, l, "| lens-review:r1:A |")
	contains(t, l, "| review-gate:w1 |")
	contains(t, l, "| cleanup | structural | residue-at-return |", "cleanup is a reader")

	fx := stub(t, twoWaves(), Hs{
		"critique-code": func(c Call, s *State) any {
			if c.Label == "critique-code:r1:A" {
				return critical1("a.go:1")
			}
			return orchtest.NoFindings()
		},
		"fix-code": withCall("fix-code", nil, dirty),
	}, nil)
	fout := run(t, fx)
	eqs(t, fout.stopKinds(), []string{"uncommitted-at-return"})
	if has(fx.Labels(), "critique-code:r2:A") {
		t.Errorf("a stopping fix went to another round: %v", fx.Labels())
	}
}

// A merge stop keeps what the merge did: the wave still re-plans over the
// merged unit before it stops. A merge denial is a record.
func TestMergeCallStop(t *testing.T) {
	s := stub(t, twoWaves(), Hs{
		"implement": implemented(M{"discoveries": []M{{"statement": "d", "affects": "B"}}}),
		"merge":     withCall("merge", nil, dirty),
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	eqs(t, out.stopKinds(), []string{"uncommitted-at-return"})
	if !has(s.Labels(), "replan:w1") || !has(s.Labels(), "review-gate:w1") {
		t.Errorf("the merged wave did not re-plan: %v", s.Labels())
	}
	if out.Escalation.PreReplanSha == nil {
		t.Error("no pre_replan_sha: the stop did not come from the re-plan pass")
	}

	d := stub(t, twoWaves(), Hs{"merge": withCall("merge", nil, denied)}, nil)
	eq(t, run(t, d).Status, "complete")
	contains(t, ledgerText(t, d, "ledger:w1"), "| merge | structural | permission-denied |")
}

// A re-plan call stop keeps today's escalation: plan_sha the re-plan's
// commit, pre_replan_sha the wave's plan, with its verdict rows recorded.
func TestReplanCallStop(t *testing.T) {
	s := stub(t, twoWaves(), Hs{
		"implement": implemented(M{"discoveries": []M{{"statement": "d", "affects": "B"}}}),
		"replan":    withCall("replan", nil, dirty),
	}, nil)
	out := run(t, s)
	eqs(t, out.stopKinds(), []string{"uncommitted-at-return"})
	eq(t, out.Escalation.Stage, "replan")
	match(t, out.Escalation.PlanSha, `^replan`)
	if out.Escalation.PreReplanSha == nil || *out.Escalation.PreReplanSha == out.Escalation.PlanSha {
		t.Errorf("pre_replan_sha %v", out.Escalation.PreReplanSha)
	}
	contains(t, ledgerText(t, s, "ledger:w1"), "| verdict |")
}

// A decision re-plan call stop keeps its plan SHA, which carries the
// applied decisions.
func TestDecisionReplanCallStop(t *testing.T) {
	decisions := M{"decisions": []M{{"id": "d1", "question": "q", "decision": "yes"}}}
	s := stub(t, twoWaves(), Hs{
		"load":            loadWith(M{"decisionsBlock": decisionsBlock("2026-09-19", "yes")}),
		"decision-replan": withCall("decision-replan", nil, noSkill),
	}, decisions)
	out := run(t, s)
	eqs(t, out.stopKinds(), []string{"skill-not-loaded"})
	eq(t, out.Escalation.Stage, "decision-replan")
	eq(t, out.Escalation.PlanSha, "decided1")
}

// Termination and obligations: a call stop at termination stops there; a
// null obligations call keeps its records beside agent-null.
func TestTerminationStagesCallSignals(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"termination": withCall("termination", nil, dirty)}, nil)
	out := run(t, s)
	eqs(t, out.stopKinds(), []string{"uncommitted-at-return"})
	eq(t, out.Escalation.Stage, "termination")

	o := stub(t, twoWaves(), Hs{"obligations": func(Call, *State) any { return denied }}, nil)
	oout := run(t, o)
	eqs(t, oout.stopKinds(), []string{"agent-null"})
	contains(t, ledgerText(t, o, "ledger:w2"), "| obligations | structural | permission-denied |")
}

// S6: an out-of-schema merge status, seeded (the runtime never validated
// it) or reported by the runtime as violations, is a schema-violation stop
// beside agent-null, never a claimed-disjoint-conflict; an out-of-schema
// finding severity is a stop naming it, not dropped.
func TestSchemaViolations(t *testing.T) {
	st := &State{Plan: twoWaves(), Completed: map[string]bool{}, MergeOrder: []string{"A"}}
	bad := orchtest.Defaults()["merge"](Call{}, st).(M)
	bad["result"].(M)["merges"] = []M{{"unit": "A", "status": "weird", "files": []string{"a.go"}, "sha": "m"}}
	s := stub(t, twoWaves(), nil, M{"seed": M{"merge:w1": bad}})
	out := run(t, s)
	eqs(t, out.stopKinds(), []string{"schema-violation", "agent-null"})
	match(t, out.stops()[0].Statement, `status: "weird" not in enum`)
	noMatch(t, ledgerText(t, s, "ledger:w1"), `claimed-disjoint-conflict`)

	v := stub(t, twoWaves(), Hs{"merge": func(Call, *State) any {
		return lsfrt.CallResult{Violations: []string{`$.result.merges[0].status: "weird" not in enum`}}
	}}, nil)
	eqs(t, run(t, v).stopKinds(), []string{"schema-violation", "agent-null"})

	sev := R(M{"findings": []M{{"severity": "Major", "statement": "s", "location": "a.go:1"}}, "systemic": false, "summary": "s"})
	f := stub(t, twoWaves(), nil, M{"seed": M{"critique-code:r1:A": sev}})
	fout := run(t, f)
	eqs(t, fout.stopKinds(), []string{"schema-violation", "agent-null"})
	match(t, fout.stops()[0].Statement, `"Major" not in enum`)
}

// S1: every agent call site's writer flag. A call leaving changes stops
// the run at a writer and is recorded at a reader, with exactly one row.
func TestUncommittedPerSite(t *testing.T) {
	decisions := M{"decisions": []M{{"id": "d1", "question": "q", "decision": "yes"}}}
	critical := func(c Call, s *State) any {
		if c.Label == "critique-code:r1:A" || c.Label == "critique-plan:r1:A" {
			return critical1("a.go:1")
		}
		return orchtest.NoFindings()
	}
	edits := func(c Call, s *State) any {
		r := orchtest.Defaults()["replan"](c, s).(M)
		r["result"].(M)["directEdits"] = []orchtest.DirectEdit{{Path: "docs/x.md", Description: "d"}}
		return r
	}
	for _, tc := range []struct {
		prefix, label string
		writer        bool
		p             func() *plan.Plan
		extra         Hs
		args          M
	}{
		{"plan", "plan:A", true, twoWaves, nil, nil},
		{"critique-plan", "critique-plan:r1:A", false, twoWaves, Hs{"critique-plan": nil}, nil},
		{"fix-plan", "fix-plan:r1:A", true, twoWaves, Hs{"critique-plan": critical}, nil},
		{"implement", "implement:A", true, twoWaves, nil, nil},
		{"critique-code", "critique-code:r1:A", false, twoWaves, nil, nil},
		{"lens-review", "lens-review:r1:A", false, twoWaves, nil, nil},
		{"critique-design", "critique-design:r1:D", false, func() *plan.Plan { return one("D", "design") }, nil, nil},
		{"fix-code", "fix-code:r1:A", true, twoWaves, Hs{"critique-code": critical}, nil},
		{"merge", "merge:w1", true, twoWaves, nil, nil},
		{"review-gate", "review-gate:w1", false, twoWaves, nil, nil},
		{"cleanup", "cleanup:w1", false, twoWaves, nil, nil},
		{"replan", "replan:w1", true, twoWaves, Hs{"implement": implemented(M{"discoveries": []M{{"statement": "d", "affects": "B"}}})}, nil},
		{"direct-edits", "direct-edits:w1", true, twoWaves, Hs{"implement": implemented(M{"discoveries": []M{{"statement": "d", "affects": "B"}}}), "replan": edits}, nil},
		{"decision-replan", "decision-replan", true, twoWaves, Hs{"load": loadWith(M{"decisionsBlock": decisionsBlock("2026-09-19", "yes")})}, decisions},
		{"termination", "termination", true, twoWaves, nil, nil},
		{"obligations", "obligations", false, twoWaves, nil, nil},
	} {
		for _, ev := range []struct {
			name             string
			cr               lsfrt.CallResult
			writer, reader   string // the kind at a writer and at a reader
			readerStops, val bool   // val: the value survives
		}{
			{"uncommitted", dirty, "uncommitted-at-return", "residue-at-return", false, true},
			{"violation", lsfrt.CallResult{Violations: []string{"$.result: required"}}, "schema-violation", "schema-violation", true, false},
		} {
			t.Run(tc.prefix+"/"+ev.name, func(t *testing.T) {
				hs := Hs{}
				for k, h := range tc.extra {
					if h != nil {
						hs[k] = h
					}
				}
				cr := ev.cr
				if ev.val {
					hs[tc.prefix] = withCall(tc.prefix, hs[tc.prefix], cr)
				} else {
					hs[tc.prefix] = func(Call, *State) any { return cr }
				}
				s := stub(t, tc.p(), hs, tc.args)
				out := run(t, s)
				if !has(s.Labels(), tc.label) {
					t.Fatalf("%s not called: %v", tc.label, s.Labels())
				}
				kind, stops := ev.reader, ev.readerStops
				if tc.writer {
					kind, stops = ev.writer, true
				}
				row := regexp.MustCompile(`\| ` + kind + ` \| [^|]* \| ` + regexp.QuoteMeta(tc.label) + ` \|`)
				rows := 0
				for _, l := range filterPrefix(s.Labels(), "ledger:") {
					rows += len(row.FindAllString(ledgerText(t, s, l), -1))
				}
				eq(t, rows, 1, "rows of "+kind)
				if stops {
					eq(t, out.Status, "stopped")
					if !has(out.stopKinds(), kind) {
						t.Errorf("stops %v", out.stopKinds())
					}
				} else if has(out.stopKinds(), kind) || has(out.stopKinds(), ev.writer) {
					t.Errorf("a reader stopped: %v", out.stopKinds())
				}
			})
		}
	}
}

// S5: a gate-not-run stop is not retried: one implement call, then the stop.
func TestGateNotRunNotRetried(t *testing.T) {
	s := stub(t, graph("plan0", unit("A")), Hs{"implement": implemented(M{"gates": gates(g("xtask ci", "not-run", "timed out"))})}, nil)
	out := run(t, s)
	eqs(t, out.stopKinds(), []string{"gate-not-run"})
	eq(t, len(filterPrefix(s.Labels(), "implement:")), 1, "implement calls")
}

// D22: a fix with a gate not run and a gate dropped is two stops,
// gate-omitted then gate-not-run, each naming its gates.
func TestFixGateOmittedAndNotRun(t *testing.T) {
	s := stub(t, graph("plan0", unit("A")), Hs{"critique-code": criticalOnce(), "fix-code": fixCode(g("pytest -q", "not-run", "timed out"))}, nil)
	out := run(t, s)
	eqs(t, out.stopKinds(), []string{"gate-omitted", "gate-not-run"})
	eq(t, out.stops()[0].Statement, "gate(s) omitted: ruff check .")
	eq(t, out.stops()[1].Statement, "gate(s) not run: pytest -q")
}

// The runtime's flag names and its no-output violation are the ones
// plan.SignalsFromCall maps: a rename on either side fails here.
func TestRuntimeFlagsAreMapped(t *testing.T) {
	at := plan.At{Stage: "implement", Unit: "A", Wave: plan.W(1)}
	for flag, kind := range map[string]string{
		lsfrt.FlagSkillNotLoaded:         "skill-not-loaded",
		lsfrt.FlagGuardKilledAfterResult: "guard-killed-after-result",
		lsfrt.FlagUncommittedUnchecked:   "uncommitted-at-return",
	} {
		sigs, unknown := plan.SignalsFromCall(plan.CallEvents{Label: "x", Writer: true, Flags: []string{flag}}, at)
		if len(unknown) != 0 || len(sigs) != 1 || sigs[0].Kind != kind {
			t.Errorf("%s: %v %v", flag, sigs, unknown)
		}
	}
	sigs, _ := plan.SignalsFromCall(plan.CallEvents{Label: "x", Violations: []string{lsfrt.NoOutput}}, at)
	contains(t, sigs[0].Statement, "no structured output after retry")
}
