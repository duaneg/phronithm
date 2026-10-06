package orchestrator

import (
	"encoding/json"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
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
