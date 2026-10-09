package orchestrator

import (
	"fmt"
	"strings"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

var (
	unit      = orchtest.Unit
	graph     = orchtest.Graph
	twoWaves  = orchtest.TwoWaves
	noFinding = orchtest.NoFindings
)

func discovering() Hs { return orchtest.Discovering() }

func merged(h Hs, more Hs) Hs {
	out := Hs{}
	for k, v := range h {
		out[k] = v
	}
	for k, v := range more {
		out[k] = v
	}
	return out
}

// js: maxUnitsPerWave caps each wave in execution order and defers the rest
func TestMaxUnitsPerWave(t *testing.T) {
	s := stub(t, graph("plan0", unit("A"), unit("B"), unit("C")), nil, M{"maxUnitsPerWave": 2})
	out := run(t, s)
	eq(t, out.Status, "complete")
	eq(t, out.Waves, 2)
	eqs(t, filterPrefix(s.Labels(), "implement"), []string{"implement:A", "implement:B", "implement:C"})
	before(t, s.Labels(), "merge:w1", "implement:C")
}

// js: clean run: two waves, termination, obligations, complete; silence stops nothing and skips the re-plan
func TestCleanRun(t *testing.T) {
	s := stub(t, twoWaves(), nil, nil)
	out := run(t, s)
	eq(t, out.Status, "complete")
	eq(t, out.Waves, 2)
	l := s.Labels()
	eqs(t, filterPrefix(l, "implement"), []string{"implement:A", "implement:B"})
	before(t, l, "merge:w1", "review-gate:w1")
	before(t, l, "review-gate:w1", "plan:B")
	if hasPrefix(l, "replan:") {
		t.Error("nothing to absorb: no re-plan")
	}
	if hasPrefix(l, "load:w") {
		t.Error("no re-plan, nothing to reload")
	}
	if !has(s.Logs, "wave 1: nothing to absorb; re-plan skipped, plan @ merged1") {
		t.Error(s.Logs)
	}
	match(t, s.Prompt("plan:B"), `@ merged1`, "the next wave schedules from the merge's SHA")
	eq(t, out.PlanSha, "merged2")
	if !has(l, "termination") || !has(l, "obligations") || has(l, "direct-edits") {
		t.Error(l)
	}
	if has(l, "ledger:w1") {
		t.Error("nothing to scribe on a wave with no signals")
	}
	eq(t, len(out.Unscribed), 0)
	if out.Escalation != nil {
		t.Error("escalation")
	}
	match(t, out.Digest.Note, `not evidence`)
}

// js: a wave with an item to absorb re-plans, reloads, and the next wave schedules from the re-plan
func TestItemReplans(t *testing.T) {
	s := stub(t, twoWaves(), discovering(), nil)
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	l := s.Labels()
	before(t, l, "review-gate:w1", "replan:w1")
	before(t, l, "replan:w1", "load:w1")
	before(t, l, "load:w1", "plan:B")
	if !has(l, "ledger:w1") {
		t.Error(l)
	}
}

// js: a wave with only records — readings, deviations, findings — skips the re-plan but scribes them
func TestRecordsOnlySkipReplan(t *testing.T) {
	s := stub(t, twoWaves(), Hs{
		"plan": func(c Call, _ *State) any {
			return R(M{"branch": "plan-" + c.Unit, "sha": "p", "path": "docs/plans/" + c.Unit + "-plan.md", "readings": []M{{"clause": "c", "reading": "r"}}})
		},
		"implement": implemented(M{"deviations": []string{"d"}}),
		"lens-review": func(Call, *State) any {
			return R(M{"findings": []plan.Finding{orchtest.Finding("Minor", "naming", "a.py:3")}, "systemic": false, "summary": "s"})
		},
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	if hasPrefix(s.Labels(), "replan:") || !has(s.Labels(), "ledger:w1") {
		t.Error(s.Labels())
	}
	eq(t, fmt.Sprint(out.Digest.ByKind), fmt.Sprint(map[string]int{"reading": 2, "deviation": 2, "finding": 2}))
	eq(t, fmt.Sprint(out.Digest.Attention.MinorFindings), fmt.Sprint(map[string]int{"A": 1, "B": 1}))
}

func unresolvableA(extra ...plan.Merge) H {
	return func(Call, *State) any {
		ms := append([]plan.Merge{{Unit: "A", Status: "unresolvable", Files: []string{"src/x.js"}}}, extra...)
		return R(M{"integrationBranch": "integration", "merges": ms, "planSha": "m1"})
	}
}

// js: a stop on a wave with nothing to absorb carries the merge's SHA and no pre_replan_sha
func TestStopNothingToAbsorb(t *testing.T) {
	s := stub(t, orchtest.TwoParallel(), Hs{"merge": unresolvableA(plan.Merge{Unit: "B", Status: "clean", Files: []string{}, Sha: "mb"})}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	eq(t, out.Escalation.Stage, "wave")
	eqs(t, out.stopKinds(), []string{"merge-conflict-unresolvable"})
	eq(t, out.Escalation.PlanSha, "m1")
	if out.Escalation.PreReplanSha != nil {
		t.Error("pre_replan_sha")
	}
	if has(s.Labels(), "replan:w1") || !has(s.Labels(), "ledger:w1") {
		t.Error("the stop is scribed, no re-plan", s.Labels())
	}
}

// js: the return leads with status and escalation; scribed rows live in the signals file, unscribed ones in the return
func TestReturnShape(t *testing.T) {
	conflict := Hs{"merge": unresolvableA()}
	scribed := run(t, stub(t, twoWaves(), conflict, nil))
	if !strings.HasPrefix(scribed.raw, `{"status":"stopped","workflow":"lsf-implement","escalation":{`) || !strings.Contains(scribed.raw, `},"digest":{`) {
		t.Errorf("key order %s", trunc(scribed.raw))
	}
	eq(t, scribed.LedgerPath, "docs/plans/f-signals/plan0.md", "one ledger file per launch, named by the starting plan SHA")
	if strings.Contains(scribed.raw, `"ledger":`) || len(scribed.Unscribed) != 0 {
		t.Error(scribed.raw)
	}
	ns := stub(t, twoWaves(), merged(conflict, Hs{"ledger": func(Call, *State) any { return nil }}), nil)
	unscribed := run(t, ns)
	if !has(ns.Labels(), "ledger:w1") {
		t.Error(ns.Labels())
	}
	eqs(t, kindsOf(unscribed.Unscribed), []string{"merge-conflict-unresolvable"})
}

func deviating() H {
	return func(c Call, _ *State) any {
		return R(orchtest.Implemented(c.Unit, M{"deviations": []string{"dev-" + c.Unit}}))
	}
}

// js: a null scribe does not stop; its rows ride in the next append, or stay in unscribed when none follows
func TestNullScribe(t *testing.T) {
	scribes := 0
	s := stub(t, twoWaves(), Hs{
		"implement": deviating(),
		"ledger": func(Call, *State) any {
			scribes++
			if scribes == 1 {
				return nil
			}
			return R(M{"path": "docs/plans/f-signals.md", "sha": "l"})
		},
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete")
	eq(t, len(out.Unscribed), 0)
	match(t, ledgerText(t, s, "ledger:w2"), `(?s)dev-A.*dev-B`, "wave 1 rows the null scribe left ride in the wave 2 append")

	never := run(t, stub(t, twoWaves(), Hs{"implement": deviating(), "ledger": func(Call, *State) any { return nil }}, nil))
	eq(t, never.Status, "complete")
	devs := []string{}
	for _, r := range never.Unscribed {
		if r.Kind == "deviation" {
			devs = append(devs, r.Statement)
		}
	}
	eqs(t, devs, []string{"dev-A", "dev-B"})
}

// js: a ledger scribe that returns no commit SHA stops the run at that append, and no later scribe runs
func TestLedgerNoCommitStops(t *testing.T) {
	s := stub(t, twoWaves(), Hs{
		"implement": deviating(),
		"ledger": func(Call, *State) any {
			return R(M{"path": "docs/plans/f-signals.md", "sha": " ", "pauseRequested": false})
		},
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	eq(t, out.Escalation.Stage, "ledger")
	st := out.stops()
	if len(st) != 1 || st[0].Kind != "agent-null" || st[0].Stage != "ledger" || st[0].Referent != "docs/plans/f-signals/plan0.md" {
		t.Errorf("%+v", st)
	}
	match(t, st[0].Statement, `no commit SHA`)
	if has(s.Labels(), "merge:w2") {
		t.Error("the next wave does not merge over the leftover append")
	}
	eqs(t, filterPrefix(s.Labels(), "ledger"), []string{"ledger:w1"})
	eqs(t, kindsOf(out.Unscribed), []string{"deviation", "agent-null"})
}

// js: a ledger scribe on a stop path that returns no commit SHA joins the escalation
func TestStopPathLedgerNoCommit(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"merge": unresolvableA(), "ledger": func(Call, *State) any { return R(M{"path": "x", "sha": ""}) }}, nil)
	out := run(t, s)
	eq(t, out.Escalation.Stage, "wave")
	eqs(t, out.stopKinds(), []string{"merge-conflict-unresolvable", "agent-null"})
	for _, x := range out.stops() {
		eq(t, x.Disposition, "stopped")
	}
	eqs(t, filterPrefix(s.Labels(), "ledger"), []string{"ledger:w1"})
}

// js: a decision-log or direct-edits scribe that returns no commit SHA stops like a null return
func TestScribeNoCommitStops(t *testing.T) {
	decisions := M{"decisions": []M{{"id": "d1", "question": "q", "decision": "yes"}}}
	dec := run(t, stub(t, twoWaves(), Hs{"decision-log": func(Call, *State) any { return R(M{"path": "docs/plans/f-decisions.md", "sha": ""}) }}, decisions))
	eq(t, dec.Escalation.Stage, "decision-log")
	if st := dec.stops(); len(st) != 1 || st[0].Kind != "agent-null" || st[0].Referent != "docs/plans/f-decisions.md" {
		t.Errorf("%+v", st)
	}
	edits := run(t, stub(t, twoWaves(), Hs{
		"termination": func(_ Call, s *State) any {
			return R(M{"planSha": s.Plan.Sha, "appendedUnits": []any{}, "directEdits": []orchtest.DirectEdit{{Path: "README.md", Description: "mention A"}}})
		},
		"direct-edits": func(Call, *State) any { return R(M{"path": "README.md", "sha": ""}) },
	}, nil))
	eq(t, edits.Escalation.Stage, "direct-edits")
	if st := edits.stops(); len(st) != 1 || st[0].Kind != "agent-null" || st[0].Referent != "README.md" {
		t.Errorf("%+v", st)
	}
}

// js: the re-planned SHA appears in the next wave's prompts
func TestReplannedShaInPrompts(t *testing.T) {
	s := stub(t, twoWaves(), discovering(), nil)
	run(t, s)
	match(t, s.Prompt("plan:B"), `@ replan\d`)
	match(t, s.Prompt("plan:B"), "`### B` section")
	match(t, s.Prompt("plan:A"), `@ plan0`)
}

// js: every unit stage is told what the Scope line means; only commits are asked for beside result
func TestEntrySemanticsAndReturnFields(t *testing.T) {
	s := stub(t, twoWaves(), nil, nil)
	run(t, s)
	for _, c := range agentCalls(s) {
		noMatch(t, c.Prompt, `assumptions|ambiguities|contradictions`, c.Label)
		match(t, c.Prompt, "return `commits`", c.Label)
	}
	for _, l := range []string{"plan:A", "critique-plan:r1:A", "implement:A", "critique-code:r1:A", "lens-review:r1:A"} {
		match(t, s.Prompt(l), `(?s)Scope line declares the files this unit writes.*does not limit what the work may edit`, l)
	}
	p := s.Prompt("plan:A")
	match(t, p, `\*\*Readings\*\*`)
	noMatch(t, p, `phronithm:feature`)
	gate := s.Prompt("review-gate:w1")
	for _, re := range []string{`Answer one question`, `all five fields`, `Do not re-evaluate the units' done conditions`} {
		match(t, gate, re)
	}
}

// js: the merge archives each completed entry behind a stub and writes the compact completion entry; the re-plan writes the compact maintenance entry and archives closed risks
// Go: the merge is a code step (D24), given the paths the bookkeeping writes;
// TestRenderBookkeeping holds the forms.
func TestMergeAndReplanBookkeeping(t *testing.T) {
	s := stub(t, twoWaves(), discovering(), M{"archivePath": "docs/plans/f-old.md"})
	run(t, s)
	m := input(t, s, "merge:w1")
	eq(t, m["planPath"], any("docs/plans/f-breakdown.md"))
	eq(t, m["archivePath"], any("docs/plans/f-old.md"))
	eq(t, m["ledgerPath"], any("docs/plans/f-signals/plan0.md"))
	eq(t, m["wave"], any(float64(1)))
	rp := s.Prompt("replan:w1")
	match(t, rp, `### <date, date -I> — re-plan wave 1 \(<item ids>\)`)
	match(t, rp, `<kind> <unit> — <one clause>`)
	match(t, rp, "moves to docs/plans/f-old\\.md under `## Closed risks`")
	match(t, rp, `Parallel Streams, Critical Path, and Dependency Graph sections are not maintained`)
	noMatch(t, rp, `record every amendment in Plan Maintenance`)
	d := stub(t, twoWaves(), nil, nil)
	out := run(t, d)
	eq(t, input(t, d, "merge:w1")["archivePath"], any("docs/plans/f-archive.md"), "default archive path")
	eq(t, out.LedgerPath, "docs/plans/f-signals/plan0.md")
}

// js: unit stages extract their entry and obligations by command; the plan and report carry size budgets; critique rounds read the plan by path
func TestUnitStagePrompts(t *testing.T) {
	round := 0
	s := stub(t, twoWaves(), Hs{"critique-plan": func(Call, *State) any {
		round++
		if round == 1 {
			return critical1("plan")
		}
		return noFinding()
	}}, nil)
	run(t, s)
	// The range ends at any ##-prefixed heading, so the last entry does not
	// run on into `## Graph`.
	entry := "git show plan0:docs/plans/f-breakdown.md | sed -n '/^### A$/,/^##/p'"
	obligations := "git show plan0:docs/plans/f-breakdown.md | sed -n '/^## Standing Obligations/,/^## /p' | grep -n 'A'"
	for _, l := range []string{"plan:A", "critique-plan:r1:A", "fix-plan:r1:A", "implement:A", "critique-code:r1:A", "lens-review:r1:A"} {
		p := s.Prompt(l)
		contains(t, p, entry, l)
		contains(t, p, obligations, l)
		noMatch(t, p, "Read it first \\(`git show plan0:docs/plans/f-breakdown\\.md`\\)", l)
	}
	p := s.Prompt("plan:A")
	for _, re := range []string{`about 12 KB is the target`, `in one call and commit it\. Do not trim it to length afterwards`, `at most a Minor finding`,
		`A clause with one plain reading is not listed`, `\*\*Goal\*\* — one sentence`, `travel with every stage prompt; cite them, do not copy them`,
		`\*\*Context\*\* — the architecture sections relied on, by heading\.\n`, `not the plan text`} {
		match(t, p, re)
	}
	noMatch(t, p, `at or under|by their text|self-contained`)
	match(t, s.Prompt("implement:A"), "(?s)Run the project's gates.*once at your final HEAD and report each under `gates`")
	for _, l := range []string{"critique-code:r1:A", "lens-review:r1:A"} {
		p := s.Prompt(l)
		match(t, p, "ran the project's gates at this branch's HEAD:\n- `pytest -q` — green: 412 passed\n- `ruff check \\.` — green\n", l)
		match(t, p, `Do not run the whole suite, the project's CI job, or a lint pass; run a single targeted test only`, l)
		noMatch(t, p, `If no gate is listed`, l)
	}
	match(t, s.Prompt("critique-code:r1:A"), `from the code and the gate results above`)
	r1, r2 := s.Prompt("critique-plan:r1:A"), s.Prompt("critique-plan:r2:A")
	match(t, r1, `git show plan-A:docs/plans/A-plan\.md`)
	match(t, r2, `git show fixed-A:docs/plans/A-plan\.md`, "round 2 reads the fixed branch")
	noMatch(t, r1, `--- implementation plan`)
	fix := s.Prompt("fix-plan:r1:A")
	noMatch(t, fix, `12 KB|planText`)
	match(t, fix, `do not shorten the plan to length`)
	report := input(t, s, "completion-report:A")
	eq(t, report["path"], any("docs/plans/A-completion.md"))
	eq(t, report["unit"], any("A"))
	eq(t, report["base"], any("impl-A"), "the report goes on top of the unit's branch")
	match(t, report["text"].(string), `^# A — completion report\n`, "the rendered report is the step's input")
	eq(t, report["subject"], any("docs(f): A completion report"))
	noMatch(t, report["text"].(string), `Assemble it|phronithm:feature`)
}

func gateInvalidation(over M) M {
	inv := M{"section": "§4.8", "decision": "two-tone families are two-part groups with named parts", "premise": "nested art classifies to a GlyphGroup",
		"evidence": "svgpath.py:_classify; brazier ring+fire parses to a bare Glyph", "amendment": "nested art is authored as siblings; _classify keeps ids on holes"}
	for k, v := range over {
		inv[k] = v
	}
	return inv
}

// js: a complete architecture invalidation from the gate stops the wave with the amendment in the escalation
func TestInvalidationStops(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"review-gate": func(Call, *State) any { return R(M{"invalidations": []M{gateInvalidation(nil)}, "notes": []any{}}) }}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	eq(t, out.Escalation.Stage, "wave")
	eq(t, *out.Escalation.Wave, 1)
	eq(t, out.Escalation.PlanSha, "merged1", "merge recorded completion before the stop")
	if st := out.stops(); len(st) != 1 || st[0].Kind != "architecture-invalidated" || st[0].Source != "gate" {
		t.Errorf("%+v", st)
	}
	eq(t, len(out.Escalation.DecisionNeeded), 1)
	match(t, out.Escalation.DecisionNeeded[0].Question, `amendment: nested art is authored as siblings`)
	l := s.Labels()
	if !has(l, "ledger:w1") || has(l, "replan:w1") || has(l, "plan:B") {
		t.Error("ledger scribed; no re-plan after a stop that supersedes the plan", l)
	}
}

// js: an invalidation missing a field, and every note, is recorded and handed to the re-plan as an item
func TestIncompleteInvalidationIsAnItem(t *testing.T) {
	n := 0
	s := stub(t, twoWaves(), Hs{
		"review-gate": func(Call, *State) any {
			return R(M{"invalidations": []M{gateInvalidation(M{"amendment": ""})}, "notes": []M{{"statement": "N3 trigger fired; no unit owns it", "referent": "§6 N3"}}})
		},
		"replan": func(c Call, s *State) any {
			n++
			s.Plan = completeFrom(s, fmt.Sprintf("replan-x%d", n))
			return R(M{"planSha": s.Plan.Sha, "amendments": []M{{"kind": "unit-added", "unit": "N3", "statement": "owner for N3"}}, "overBudget": []any{}, "verdicts": orchtest.VerdictsFor(c.Prompt, "amended"), "summary": "absorbed"})
		},
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	eq(t, out.Digest.ByKind["architecture-note"], 4, "two notes per wave, both waves")
	eq(t, out.Digest.ByKind["verdict"], 4)
	eq(t, out.Digest.ByKind["amendment"], 2)
	notes := out.Digest.Attention.ArchitectureNotes
	if len(notes) < 2 || !strings.HasPrefix(notes[0], "incomplete invalidation ") || !strings.HasPrefix(notes[1], "N3 trigger fired; no uni") {
		t.Error(notes)
	}
	w1 := s.Prompt("replan:w1")
	match(t, w1, `(?s)Items this wave.*"id": "rp-1".*"kind": "architecture-note".*"id": "rp-2"`)
	noMatch(t, w1, `Other records this wave`)
}

// completeFrom is the stub plan at sha with the merged units complete.
func completeFrom(s *State, sha string) *plan.Plan {
	np := *s.Plan
	np.Sha = sha
	np.Units = nil
	for _, u := range s.Plan.Units {
		cu := *u
		cu.Complete = s.Completed[u.ID]
		np.Units = append(np.Units, &cu)
	}
	return &np
}

// js: an implement return with a gate not run stops before review, naming the gate, and keeps its records
func TestGateNotRunStops(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"implement": implemented(M{
		"gates":       gates(g("pytest -q", "green", "412 passed"), g("xtask ci", "not-run", "outlived the tool timeout")),
		"discoveries": []M{{"statement": "note from the stopped unit", "affects": "B"}},
	})}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	st := out.stops()
	if len(st) != 1 || st[0].Kind != "gate-not-run" || st[0].Stage != "implement" || st[0].Source != "structural" {
		t.Fatalf("%+v", st)
	}
	match(t, st[0].Statement, `gate\(s\) not run: xtask ci$`)
	eq(t, st[0].Referent, "xtask ci")
	if hasPrefix(s.Labels(), "critique-code") || hasPrefix(s.Labels(), "lens-review") {
		t.Error(s.Labels())
	}
	match(t, ledgerText(t, s, "ledger:w1"), `note from the stopped unit`, "the stopped unit keeps its discovery record")
}

// js: an implement return with no gate reported stops before review as a
// gate not run; Go: a gate omitted (D22)
func TestNoGateReportedStops(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"implement": implemented(M{"gates": []any{}})}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	eqs(t, out.stopKinds(), []string{"gate-omitted"})
	match(t, out.stops()[0].Statement, `\(no gate reported\)`)
	if hasPrefix(s.Labels(), "critique-code") || hasPrefix(s.Labels(), "lens-review") {
		t.Error(s.Labels())
	}
}

// js: a red gate is a result, not a gate not run: the unit reaches review
func TestRedGateReachesReview(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"implement": implemented(M{"gates": gates(g("pytest -q", "red", "2 failed"))})}, nil)
	out := run(t, s)
	if has(out.stopKinds(), "gate-not-run") || !has(s.Labels(), "critique-code:r1:A") {
		t.Error(out.stopKinds(), s.Labels())
	}
}

// js: a design unit with no gate reported is not stopped: its review reads no gates
func TestDesignUnitNoGates(t *testing.T) {
	s := stub(t, one("D", "design"), Hs{"implement": implemented(M{"gates": []any{}})}, nil)
	out := run(t, s)
	if has(out.stopKinds(), "gate-not-run") || !has(s.Labels(), "critique-design:r1:D") {
		t.Error(out.stopKinds(), s.Labels())
	}
}

func fixCode(gs ...orchtest.Gate) H {
	return func(c Call, _ *State) any {
		return R(M{"branch": "fixed-code-" + c.Unit, "sha": "f", "applied": []M{{"finding": "bad", "fix": "ok"}}, "notApplied": []any{}, "gates": gs})
	}
}

// js: a review round after a code fix reads the fix stage's gate runs, not the implement stage's
func TestReviewReadsFixGates(t *testing.T) {
	s := stub(t, graph("plan0", unit("A")), Hs{"critique-code": criticalOnce(), "fix-code": fixCode(g("pytest -q", "green", "99 passed (fix)"), g("ruff check .", "green", ""))}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete")
	match(t, s.Prompt("critique-code:r1:A"), "The implement stage ran the project's gates at this branch's HEAD:\n- `pytest -q` — green: 412 passed")
	for _, l := range []string{"critique-code:r2:A", "lens-review:r2:A"} {
		match(t, s.Prompt(l), "The round-1 fix stage ran the project's gates at this branch's HEAD:\n- `pytest -q` — green: 99 passed \\(fix\\)\n", l)
		noMatch(t, s.Prompt(l), `412 passed`, l)
	}
	fix := s.CallOf("fix-code:r1:A")
	match(t, fix.Prompt, "every gate the implement stage ran, below, with its command unchanged, not only those the fix touches:\n- `pytest -q`\n- `ruff check \\.`\nand report each under `gates`", "the fix is given the gate set to re-run")
	if !has(schemaOf(t, fix).Properties.Result.Required, "gates") {
		t.Error("a code fix must return its gate runs")
	}
	match(t, input(t, s, "completion-report:A")["text"].(string), "Gates at HEAD:\n\n- `pytest -q` — green: 99 passed \\(fix\\)")
}

// js: a second fix round: review reads the round-2 fix's runs, each fix is given the implement stage's set, and a red fix gate still reaches review
func TestSecondFixRound(t *testing.T) {
	round := 0
	s := stub(t, graph("plan0", unit("A")), Hs{
		"critique-code": func(Call, *State) any {
			round++
			if round <= 2 {
				return critical1("a.py")
			}
			return noFinding()
		},
		"fix-code": func(c Call, _ *State) any {
			n := strings.Split(c.Label, ":")[1]
			gs := gates(g("pytest -q", "green", "5 passed (r2)"), g("ruff check .", "green", ""))
			if n == "r1" {
				gs = gates(g("pytest -q", "red", "1 failed (r1)"), g("ruff check .", "green", ""))
			}
			return R(M{"branch": "fixed-" + n + "-" + c.Unit, "sha": "f", "applied": []M{{"finding": "bad", "fix": "ok"}}, "notApplied": []any{}, "gates": gs})
		},
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete")
	match(t, s.Prompt("critique-code:r2:A"), "(?s)The round-1 fix stage ran.*`pytest -q` — red: 1 failed \\(r1\\)", "a red fix gate is a result and reaches review")
	match(t, s.Prompt("fix-code:r2:A"), "not only those the fix touches:\n- `pytest -q`\n- `ruff check \\.`\n")
	match(t, s.Prompt("critique-code:r3:A"), `(?s)The round-2 fix stage ran.*5 passed \(r2\)`)
	noMatch(t, s.Prompt("critique-code:r3:A"), `\(r1\)|412 passed`)
}

// js: a code fix whose gate did not run, that reports none, or that drops
// one the implement stage ran, stops before the next review round; Go: the
// last two are a gate omitted (D22)
func TestFixGateNotRun(t *testing.T) {
	for i, gs := range [][]orchtest.Gate{gates(g("xtask ci", "not-run", "timed out")), {}, gates(g("pytest -q", "green", "1 passed"))} {
		s := stub(t, graph("plan0", unit("A")), Hs{"critique-code": criticalOnce(), "fix-code": fixCode(gs...)}, nil)
		out := run(t, s)
		eq(t, out.Status, "stopped")
		kind := []string{"gate-not-run", "gate-omitted", "gate-omitted"}[i]
		var stop *plan.Signal
		for _, x := range out.stops() {
			if x.Kind == kind {
				stop = &x
			}
		}
		if stop == nil {
			t.Fatalf("%d: %v", i, out.stopKinds())
		}
		eq(t, stop.Stage, "fix")
		want := []string{`^gate\(s\) not run: xtask ci$`, `^gate\(s\) omitted: \(no gate reported\), pytest -q, ruff check \.$`, `^gate\(s\) omitted: ruff check \.$`}[i]
		match(t, stop.Statement, want)
		if has(s.Labels(), "critique-code:r2:A") {
			t.Error("no round-2 review")
		}
	}
}

// js: the implement and code-fix prompts say how to wait out a long gate; the plan fix is not told
func TestLongGateInPrompts(t *testing.T) {
	round := 0
	s := stub(t, graph("plan0", unit("A")), Hs{
		"critique-plan": func(Call, *State) any {
			round++
			if round == 1 {
				return critical1("plan")
			}
			return noFinding()
		},
		"critique-code": criticalOnce(),
		"fix-code":      fixCode(g("pytest -q", "green", "1 passed"), g("ruff check .", "green", "")),
	}, nil)
	run(t, s)
	for _, l := range []string{"implement:A", "fix-code:r1:A"} {
		p := s.Prompt(l)
		match(t, p, `ending your turn while it runs returns this stage with the gate unrun`, l)
		contains(t, p, longGates, l)
		match(t, p, `Bash tool's timeout set to 600000 ms`, l)
		match(t, p, "Report a gate `not-run`, with why in its summary, only when it could not be completed", l)
	}
	noMatch(t, s.Prompt("fix-plan:r1:A"), `lsf-gate|not-run`)
}

// js: a design unit's fix keeps the plain fix schema and is not asked for gates
func TestDesignFixSchema(t *testing.T) {
	round := 0
	s := stub(t, one("D", "design"), Hs{
		"critique-design": func(Call, *State) any {
			round++
			if round == 1 {
				return critical1("d.md")
			}
			return noFinding()
		},
		"fix-code": func(c Call, _ *State) any {
			return R(M{"branch": "fixed-code-" + c.Unit, "sha": "f", "applied": []M{{"finding": "bad", "fix": "ok"}}, "notApplied": []any{}})
		},
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete")
	fix := s.CallOf("fix-code:r1:D")
	if _, ok := schemaOf(t, fix).Properties.Result.Properties["gates"]; ok {
		t.Error("design fix schema asks for gates")
	}
	noMatch(t, fix.Prompt, "report each under `gates`")
}

// js: a plan or report past twice its size target is a document-oversize ledger row, never a stop or a re-plan item
func TestDocumentOversize(t *testing.T) {
	s := stub(t, twoWaves(), Hs{
		"plan": func(c Call, _ *State) any {
			b := 20 * 1024
			if c.Unit == "A" {
				b = 30 * 1024
			}
			return R(M{"branch": "plan-" + c.Unit, "sha": "p", "path": "docs/plans/" + c.Unit + "-plan.md", "bytes": b, "readings": []any{}})
		},
		"implement": func(c Call, _ *State) any {
			over := M{}
			if c.Unit == "B" {
				over["summary"] = strings.Repeat("x", 17*1024)
			}
			return R(orchtest.Implemented(c.Unit, over))
		},
		"ledger": func(Call, *State) any { return nil },
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	rows := [][3]string{}
	for _, r := range out.Unscribed {
		if r.Kind == "document-oversize" {
			rows = append(rows, [3]string{r.Unit, r.Stage, r.Statement})
			eq(t, r.Disposition, "recorded")
		}
	}
	eq(t, fmt.Sprint(rows), fmt.Sprint([][3]string{
		{"A", "plan", "docs/plans/A-plan.md is 30 KB against a 12 KB target"},
		{"B", "completion-report", "docs/plans/B-completion.md is 17 KB against a 8 KB target"},
	}))
	for _, w := range []string{"replan:w1", "replan:w2"} {
		if c := s.CallOf(w); c != nil {
			noMatch(t, c.Prompt, `document-oversize`, w+": not a re-plan item")
		}
	}
}

// js: deferrals, discoveries, and distress reach the re-plan with ids; readings, deviations, and findings go to the ledger only
func TestReplanItems(t *testing.T) {
	distress := orchtest.NoDistress()
	distress["midImplementationDesignDecisions"] = 3
	s := stub(t, twoWaves(), Hs{
		"plan": func(c Call, _ *State) any {
			return R(M{"branch": "plan-" + c.Unit, "sha": "p", "path": "docs/plans/" + c.Unit + "-plan.md",
				"readings": []M{{"clause": "Scope: tests/test_missing.py", "reading": "tests/test_present.py"}}})
		},
		"implement": implemented(M{
			"deviations": []string{"sibling predicate instead of widening"}, "writesOutsideScope": []string{"CLAUDE.md"},
			"deferrals":   []M{{"statement": "export left unrotated", "belongsTo": "B"}},
			"discoveries": []M{{"statement": "B needs an edge to C", "affects": "B"}},
			"distress":    distress,
		}),
		"critique-code": func(Call, *State) any {
			return R(M{"findings": []plan.Finding{orchtest.Finding("Significant", "fragile", "a.py:2")}, "systemic": false, "summary": "s"})
		},
		"ledger": func(Call, *State) any { return nil },
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	w1 := s.Prompt("replan:w1")
	items := strings.Split(strings.Split(w1, "Items this wave")[1], "Merge conflicts")[0]
	for _, k := range []string{`"kind": "deferral"`, `"kind": "discovery"`, `"kind": "execution-distress"`} {
		contains(t, items, k)
	}
	eqs(t, orchtest.ItemIDs(w1), []string{"rp-1", "rp-2", "rp-3"})
	for _, k := range []string{`"kind": "reading"`, `"kind": "deviation"`, `"kind": "finding"`, "Other records", "test_present", "CLAUDE.md", "fragile"} {
		if strings.Contains(w1, k) {
			t.Errorf("re-plan prompt has %s", k)
		}
	}
	kinds := map[string]bool{}
	verdicts := []string{}
	var outside, reading []string
	for _, r := range out.Unscribed {
		if r.Wave != nil && *r.Wave == 1 && r.Unit == "A" {
			kinds[r.Kind] = true
			switch {
			case r.Kind == "deviation" && strings.Contains(r.Statement, "outside"):
				outside = append(outside, r.Statement)
			case r.Kind == "reading":
				reading = append(reading, r.Statement)
			case r.Kind == "verdict":
				verdicts = append(verdicts, r.Statement)
			}
		}
	}
	if len(outside) == 0 || len(reading) == 0 {
		t.Fatalf("outside %v, reading %v", outside, reading)
	}
	match(t, outside[0], `writes outside the Scope line: CLAUDE\.md`)
	eq(t, reading[0], "1 reading(s); see docs/plans/A-plan.md Readings")
	eq(t, fmt.Sprint(kinds), fmt.Sprint(map[string]bool{"deferral": true, "deviation": true, "discovery": true, "execution-distress": true, "finding": true, "reading": true, "verdict": true}))
	eqs(t, verdicts, []string{"rp-1 amended: ok", "rp-2 amended: ok", "rp-3 amended: ok"}, "a verdict row carries the item id, not its text")
	for _, r := range out.Unscribed {
		eq(t, r.Disposition, "recorded")
	}
	eq(t, fmt.Sprint(out.Digest.Attention.UnfixedSignificant), fmt.Sprint(map[string]int{"A": 1, "B": 1}))
	eqs(t, out.Digest.Attention.DistressedUnits, []string{"A", "B"})
}

// js: an item the re-plan cannot absorb, or does not answer, stops after the re-plan
func TestReplanCannotAbsorb(t *testing.T) {
	for _, mode := range []string{"cannot", "missing"} {
		s := stub(t, twoWaves(), Hs{
			"implement": implemented(M{"discoveries": []M{{"statement": "B needs C first", "affects": "B"}, {"statement": "C is redundant", "affects": "C"}}}),
			"replan": func(Call, *State) any {
				vs := []M{}
				if mode == "cannot" {
					vs = []M{{"id": "rp-1", "verdict": "cannot", "statement": "needs the requester"}, {"id": "rp-2", "verdict": "amended", "statement": "dropped"}}
				}
				return R(M{"planSha": "replanned-stop", "amendments": []any{}, "overBudget": []any{}, "verdicts": vs, "summary": "s"})
			},
			"ledger": func(Call, *State) any { return nil },
		}, nil)
		out := run(t, s)
		eq(t, out.Status, "stopped", mode)
		eq(t, out.Escalation.Stage, "replan")
		stops := []plan.Signal{}
		verdicts := 0
		for _, r := range out.Unscribed {
			if r.Disposition == "stopped" {
				stops = append(stops, r)
			}
			if r.Kind == "verdict" {
				verdicts++
			}
		}
		want := []string{"plan-defect-unabsorbed"}
		re := `cannot absorb: needs the requester — \[implement\] discovery: B needs C first`
		if mode == "missing" {
			want = append(want, "plan-defect-unabsorbed")
			re = `no verdict — \[implement\] discovery: B needs C first`
		}
		eqs(t, kindsOf(stops), want, mode)
		eq(t, stops[0].Referent, "B")
		match(t, stops[0].Statement, re)
		if !has(s.Labels(), "replan:w1") || has(s.Labels(), "plan:B") {
			t.Error(s.Labels())
		}
		eq(t, out.Escalation.PlanSha, "replanned-stop", "relaunch from the re-plan commit")
		eq(t, *out.Escalation.PreReplanSha, "merged1")
		if mode == "cannot" {
			eq(t, verdicts, 1)
		}
	}
}

func overBudgetReplan(disposition func() string) H {
	return func(c Call, s *State) any {
		s.Plan = completeFrom(s, "replanned")
		return R(M{"planSha": "replanned", "amendments": []any{}, "overBudget": []M{{"unit": "B", "disposition": disposition(), "statement": "three written paths"}},
			"verdicts": orchtest.VerdictsFor(c.Prompt, "amended"), "summary": "s"})
	}
}

// js: a flagged over-budget unit is recorded and the run continues; an unrepaired one stops
func TestOverBudget(t *testing.T) {
	for _, d := range []string{"flagged", "unrepaired"} {
		s := stub(t, twoWaves(), merged(discovering(), Hs{"replan": overBudgetReplan(func() string { return d }), "ledger": func(Call, *State) any { return nil }}), nil)
		out := run(t, s)
		if d == "flagged" {
			eq(t, out.Status, "complete", "flag discharges the budget check")
			flags := []plan.Signal{}
			for _, r := range out.Unscribed {
				if r.Kind == "over-budget-flagged" {
					flags = append(flags, r)
				}
			}
			if len(flags) != 1 || flags[0].Disposition != "recorded" || flags[0].Unit != "B" || *flags[0].Wave != 1 {
				t.Errorf("reported each re-plan, recorded once while it stands: %+v", flags)
			}
		} else {
			eq(t, out.Status, "stopped")
			eqs(t, out.stopKinds(), []string{"unit-over-budget"})
		}
	}
}

// js: an over-budget unit is recorded again when its disposition changes, and an unrepaired one is never deduplicated
func TestOverBudgetDispositionChange(t *testing.T) {
	pass := 0
	s := stub(t, twoWaves(), merged(discovering(), Hs{"replan": overBudgetReplan(func() string {
		pass++
		if pass == 1 {
			return "flagged"
		}
		return "extracted"
	}), "ledger": func(Call, *State) any { return nil }}), nil)
	out := run(t, s)
	eq(t, out.Status, "complete")
	rows := []string{}
	for _, r := range out.Unscribed {
		if strings.Contains(r.Kind, "over-budget") {
			rows = append(rows, fmt.Sprintf("%d %s", *r.Wave, r.Kind))
		}
	}
	eqs(t, rows, []string{"1 over-budget-flagged", "2 over-budget-repaired"})
}

// js: a document-text verdict records the architecture amendment and does not stop
func TestDocumentTextVerdict(t *testing.T) {
	s := stub(t, twoWaves(), Hs{
		"review-gate": func(Call, *State) any {
			return R(M{"invalidations": []any{}, "notes": []M{{"statement": "U5 row is stale", "referent": "§6 U5"}}})
		},
		"replan": func(c Call, s *State) any {
			s.Plan = completeFrom(s, fmt.Sprintf("rp-%d", len(s.Completed)))
			vs := []M{}
			for _, id := range orchtest.ItemIDs(c.Prompt) {
				vs = append(vs, M{"id": id, "verdict": "document-text", "statement": "§6 row U5 now reads resolved"})
			}
			return R(M{"planSha": s.Plan.Sha, "amendments": []any{}, "overBudget": []any{}, "verdicts": vs, "summary": "s"})
		},
		"ledger": func(Call, *State) any { return nil },
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete")
	amendments := []plan.Signal{}
	var verdict string
	for _, r := range out.Unscribed {
		if r.Kind == "architecture-amendment" {
			amendments = append(amendments, r)
		}
		if r.Kind == "verdict" && verdict == "" {
			verdict = r.Statement
		}
	}
	eq(t, len(amendments), 2, "one per wave")
	eq(t, amendments[0].Referent, "§6 U5")
	eq(t, amendments[0].Disposition, "recorded")
	eqs(t, out.Digest.Attention.ArchitectureAmendments, []string{"§6 row U5 now reads resolved", "§6 row U5 now reads resolved"})
	match(t, verdict, `rp-1 document-text: §6 row U5 now reads resolved`)
}

// js: critique gate hits the hard cap: fixes on rounds 1-3, stop on round 4, no implement
func TestHardCap(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"critique-plan": func(Call, *State) any { return critical1("plan") }}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	if !has(out.stopKinds(), "gate-non-convergence") {
		t.Error(out.stopKinds())
	}
	l := s.Labels()
	eq(t, len(filterPrefix(l, "critique-plan")), gateMaxRounds)
	eq(t, len(filterPrefix(l, "fix-plan")), gateMaxRounds-1)
	if has(l, "implement:A") || has(l, "merge:w1") {
		t.Error(l)
	}
}

// js: a Critical the fix stage declines stops the gate at once
func TestDeclinedCritical(t *testing.T) {
	round := 0
	s := stub(t, twoWaves(), Hs{
		"critique-plan": func(Call, *State) any {
			round++
			if round == 1 {
				return critical1("plan")
			}
			return noFinding()
		},
		"fix-plan": func(c Call, _ *State) any {
			return R(M{"branch": "fixed-" + c.Unit, "sha": "f", "applied": []any{}, "notApplied": []M{{"finding": "bad", "reason": "changes the entry"}}})
		},
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	if st := out.stops(); len(st) != 1 || st[0].Kind != "gate-non-convergence" || st[0].Source != "structural" {
		t.Fatalf("%+v", st)
	}
	match(t, out.stops()[0].Statement, `declined by the fix stage — bad: changes the entry`)
	eq(t, len(filterPrefix(s.Labels(), "critique-plan")), 1)
}

// js: environment gate drift aborts the unit before implementation
func TestEnvironmentDrift(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"implement": implemented(M{"branch": "", "sha": "",
		"gateProbe":     []M{{"name": "node", "pin": "22.1", "observed": "20.0", "drifted": true}},
		"doneCondition": M{"evaluable": false, "met": false, "evidence": ""}})}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	eqs(t, out.stopKinds(), []string{"environment-gate-drift"})
	if hasPrefix(s.Labels(), "critique-code") {
		t.Error(s.Labels())
	}
}

// js: an unevaluable done condition stops the unit before review
func TestUnevaluableDoneCondition(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"implement": implemented(M{"doneCondition": M{"evaluable": false, "met": false, "evidence": "names a test that does not exist"}})}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	eqs(t, out.stopKinds(), []string{"done-condition-unevaluable"})
	if hasPrefix(s.Labels(), "critique-code") {
		t.Error(s.Labels())
	}
}

// js: an unresolvable merge conflict stops; the unit is not completed; nothing merged, so no re-plan
func TestUnresolvableMerge(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"merge": unresolvableA()}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	eqs(t, out.stopKinds(), []string{"merge-conflict-unresolvable"})
	eq(t, out.Escalation.PlanSha, "m1")
	if has(s.Labels(), "replan:w1") {
		t.Error(s.Labels())
	}
}

// js: a wave stop with a unit merged and an item to absorb still re-plans over the merged report, then stops from the re-plan SHA
func TestWaveStopStillReplans(t *testing.T) {
	s := stub(t, orchtest.TwoParallel(), merged(discovering(), Hs{"merge": unresolvableA(plan.Merge{Unit: "B", Status: "clean", Files: []string{}, Sha: "mb"})}), nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	eq(t, out.Escalation.Stage, "wave")
	eqs(t, out.stopKinds(), []string{"merge-conflict-unresolvable"})
	eq(t, out.Escalation.PlanSha, "replan1", "the re-plan commit is the plan to relaunch from")
	eq(t, *out.Escalation.PreReplanSha, "m1")
	before(t, s.Labels(), "review-gate:w1", "replan:w1")
	if hasPrefix(s.Labels(), "plan:C") {
		t.Error("no unit starts after the stop")
	}
	rp := s.Prompt("replan:w1")
	match(t, rp, `docs/plans/B-completion\.md`)
	match(t, rp, `Merge conflicts: none`, "the unresolvable conflict is not an item to absorb")
	match(t, rp, `(?s)The run stops after this re-plan.*"kind": "merge-conflict-unresolvable"`)
	eq(t, strings.Count(ledgerText(t, s, "ledger:w1"), "merge-conflict-unresolvable"), 1, "the stop is scribed once")
}

// js: a re-plan cannot verdict during a wave stop joins the same escalation
func TestReplanCannotDuringWaveStop(t *testing.T) {
	s := stub(t, orchtest.TwoParallel(), Hs{
		"implement": func(c Call, _ *State) any {
			over := M{}
			if c.Unit == "B" {
				over["deferrals"] = []M{{"statement": "later", "to": "C"}}
			}
			return R(orchtest.Implemented(c.Unit, over))
		},
		"merge": func(Call, *State) any {
			return R(M{"integrationBranch": "integration", "merges": []plan.Merge{
				{Unit: "A", Status: "regressed", Files: []string{"test/a.test.js"}, Sha: "ma"},
				{Unit: "B", Status: "clean", Files: []string{}, Sha: "mb"},
			}, "planSha": "m1"})
		},
		"replan": func(c Call, _ *State) any {
			return R(M{"planSha": "replan1", "amendments": []any{}, "overBudget": []any{}, "verdicts": orchtest.VerdictsFor(c.Prompt, "cannot"), "summary": "no"})
		},
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	eq(t, out.Escalation.Stage, "wave")
	eqs(t, out.stopKinds(), []string{"merge-regression", "plan-defect-unabsorbed"})
}

// js: a mechanically resolved conflict between claimed-disjoint units is a re-plan item, not a stop
func TestResolvedConflictIsItem(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"merge": func(_ Call, s *State) any {
		ms := []plan.Merge{}
		for _, id := range s.MergeOrder {
			ms = append(ms, plan.Merge{Unit: id, Status: "resolved", Files: []string{"CLAUDE.md"}, Sha: "m"})
		}
		return R(M{"integrationBranch": "integration", "merges": ms, "planSha": "m1"})
	}}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	w1 := s.Prompt("replan:w1")
	match(t, w1, `"kind": "claimed-disjoint-conflict"`)
	match(t, w1, `(?s)Merge conflicts: \[.*"status": "resolved"`)
}

// js: load: wrong branch is a precondition stop; invalid plan stops before any unit
func TestLoadStops(t *testing.T) {
	a := stub(t, twoWaves(), Hs{
		"load":   func(_ Call, s *State) any { return orchtest.Copied(s.Plan, M{"headBranch": "main"}) },
		"ledger": func(Call, *State) any { return nil },
	}, nil)
	outA := run(t, a)
	eq(t, outA.Status, "stopped")
	eq(t, outA.Escalation.Stage, "load")
	eqs(t, outA.stopKinds(), []string{"precondition"})
	if !has(a.Labels(), "ledger:w0") {
		t.Error("stop at load is scribed")
	}
	for _, r := range outA.Unscribed {
		eq(t, r.Disposition, "stopped")
	}
	ua, ub := unit("A"), unit("B")
	ua.Scope, ub.Scope = []string{"src"}, []string{"src/b"}
	b := stub(t, graph("plan0", ua, ub), nil, nil)
	outB := run(t, b)
	eq(t, outB.Status, "stopped")
	eqs(t, outB.stopKinds(), []string{"unserialised-overlap"})
	if hasPrefix(b.Labels(), "plan:") {
		t.Error(b.Labels())
	}
}

// js: no ready unit because of an uncleared organisational gate stops
func TestGatedNoReadyUnit(t *testing.T) {
	u := unit("A")
	u.Gates = []plan.Gate{{Name: "legal sign-off", Kind: "organisational"}}
	out := run(t, stub(t, graph("plan0", u), nil, nil))
	eq(t, out.Status, "stopped")
	eq(t, out.Escalation.Stage, "schedule")
	eqs(t, out.stopKinds(), []string{"no-ready-unit"})
}

// js: termination appends meta units, which run as a further wave; direct edits are scribed
func TestTerminationAppendsMetaUnits(t *testing.T) {
	terminated := false
	s := stub(t, graph("plan0", unit("A")), Hs{"termination": func(_ Call, s *State) any {
		if terminated {
			return R(M{"planSha": s.Plan.Sha, "appendedUnits": []any{}, "directEdits": []any{}})
		}
		terminated = true
		meta := orchtest.Deps(orchtest.Typed(unit("M"), "meta"), "A")
		np := *s.Plan
		np.Sha, np.Units, np.ExecutionOrder = "term1", append(append([]*plan.Unit{}, s.Plan.Units...), meta), append(append([]string{}, s.Plan.ExecutionOrder...), "M")
		s.Plan = &np
		return R(M{"planSha": "term1", "appendedUnits": []string{"M"}, "directEdits": []orchtest.DirectEdit{{Path: "README.md", Description: "mention M"}}})
	}}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete")
	eq(t, out.Waves, 2)
	l := s.Labels()
	if !has(l, "implement:M") {
		t.Error(l)
	}
	before(t, l, "direct-edits", "plan:M")
	match(t, s.Prompt("direct-edits"), `(?s)termination stage.*README\.md.*mention M`)
	match(t, ledgerText(t, s, "ledger:w2"), `\| termination \| reported \| direct-edit \| mention M \| README\.md \| recorded \|`)
	eqs(t, out.Digest.Attention.DirectEdits, []string{"README.md — mention M"})
}

// js: a re-plan's direct edits are made by a scribe before the load, recorded, and listed in the digest
func TestReplanDirectEdits(t *testing.T) {
	edit := orchtest.DirectEdit{Path: "src/A/README.md", Description: "note the widened signature"}
	s := stub(t, twoWaves(), merged(discovering(), Hs{"replan": func(c Call, s *State) any {
		s.Plan = completeFrom(s, fmt.Sprintf("replan-%d", len(s.Completed)))
		edits := []orchtest.DirectEdit{}
		if len(s.Completed) == 1 {
			edits = append(edits, edit)
		}
		return R(M{"planSha": s.Plan.Sha, "amendments": []any{}, "overBudget": []any{}, "verdicts": orchtest.VerdictsFor(c.Prompt, "amended"), "directEdits": edits, "summary": "s"})
	}}), nil)
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	l := s.Labels()
	before(t, l, "replan:w1", "direct-edits:w1")
	before(t, l, "direct-edits:w1", "load:w1")
	before(t, l, "load:w1", "plan:B")
	if has(l, "direct-edits:w2") {
		t.Error(l)
	}
	de := s.CallOf("direct-edits:w1")
	match(t, de.Prompt, `wave 1 re-plan`)
	match(t, de.Prompt, `(?s)src/A/README\.md.*note the widened signature`)
	match(t, de.Prompt, `@ replan-1`, "the header names the re-planned SHA")
	eq(t, de.Opts.Phase, "Re-plan")
	match(t, ledgerText(t, s, "ledger:w1"), `\| 1 \| — \| replan \| reported \| direct-edit \| note the widened signature \| src/A/README\.md \| recorded \|`)
	eqs(t, out.Digest.Attention.DirectEdits, []string{"src/A/README.md — note the widened signature"})
	match(t, s.Prompt("replan:w1"), "(?s)`directEdits`.*not made by the re-plan|not made by the re-plan.*`directEdits`")
}

// js: a null direct-edits return after a re-plan is a stop; a stopping wave still makes its edits
func TestNullDirectEdits(t *testing.T) {
	withEdit := func(c Call, _ *State) any {
		return R(M{"planSha": "replan-x", "amendments": []any{}, "overBudget": []any{}, "verdicts": orchtest.VerdictsFor(c.Prompt, "amended"),
			"directEdits": []orchtest.DirectEdit{{Path: "docs/guide.md", Description: "g"}}, "summary": "s"})
	}
	out := run(t, stub(t, twoWaves(), merged(discovering(), Hs{"replan": withEdit, "direct-edits": func(Call, *State) any { return nil }}), nil))
	eq(t, out.Status, "stopped")
	eq(t, out.Escalation.Stage, "replan")
	if st := out.stops(); len(st) != 1 || st[0].Kind != "agent-null" || st[0].Stage != "direct-edits" || *st[0].Wave != 1 {
		t.Errorf("%+v", st)
	}
	s := stub(t, orchtest.TwoParallel(), merged(discovering(), Hs{"replan": withEdit, "merge": unresolvableA(plan.Merge{Unit: "B", Status: "clean", Files: []string{}, Sha: "mb"})}), nil)
	outS := run(t, s)
	eq(t, outS.Status, "stopped")
	eqs(t, outS.stopKinds(), []string{"merge-conflict-unresolvable"})
	if !has(s.Labels(), "direct-edits:w1") {
		t.Error("the edits are made before the stop")
	}
	match(t, ledgerText(t, s, "ledger:w1"), `direct-edit \| g \| docs/guide\.md \| recorded`)
}

func decisionsBlock(date, decision string) string {
	return strings.Join([]string{"```yaml lsf-decisions", "decisions:", "  - id: d1", "    date: " + date, "    question: q", "    decision: " + decision, "```"}, "\n")
}

func loadWith(extra M) H {
	return func(_ Call, s *State) any { return orchtest.Copied(s.Plan, extra) }
}

// js: a decision re-plan's direct edits run before load at wave 0, and a null return stops there
func TestDecisionReplanDirectEdits(t *testing.T) {
	decided := func(edits ...orchtest.DirectEdit) H {
		return func(Call, *State) any {
			return R(M{"planSha": "decided1", "amendments": []any{}, "overBudget": []any{}, "verdicts": []M{{"id": "d1", "verdict": "amended", "statement": "row filed"}}, "directEdits": edits, "summary": "s"})
		}
	}
	decisions := M{"decisions": []M{{"id": "d1", "question": "q", "decision": "file it"}}}
	s := stub(t, twoWaves(), Hs{
		"load":            loadWith(M{"decisionsBlock": decisionsBlock("2026-09-19", "file it")}),
		"decision-replan": decided(orchtest.DirectEdit{Path: "docs/debt.md", Description: "file the D7 row"}),
		"ledger":          func(Call, *State) any { return nil },
	}, decisions)
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	eqs(t, s.Labels()[:5], []string{"gates-pin", "decision-log", "decision-replan", "direct-edits:w0", "ledger:w0"})
	match(t, s.Calls[2].Prompt, "(?s)`directEdits`.*not made by the re-plan|not made by the re-plan.*`directEdits`")
	match(t, s.Calls[3].Prompt, `(?s)decision re-plan.*docs/debt\.md.*file the D7 row`)
	w0 := []string{}
	for _, r := range out.Unscribed {
		if r.Wave != nil && *r.Wave == 0 {
			w0 = append(w0, r.Stage+"/"+r.Kind+"/"+r.Referent)
		}
	}
	eqs(t, w0, []string{"decision-replan/verdict/d1", "decision-replan/direct-edit/docs/debt.md"})

	nul := run(t, stub(t, twoWaves(), Hs{"decision-replan": decided(orchtest.DirectEdit{Path: "docs/debt.md", Description: "d"}), "direct-edits": func(Call, *State) any { return nil }}, decisions))
	eq(t, nul.Status, "stopped")
	eq(t, nul.Escalation.Stage, "decision-replan")
	if st := nul.stops(); len(st) != 1 || st[0].Kind != "agent-null" || st[0].Stage != "direct-edits" || *st[0].Wave != 0 {
		t.Errorf("%+v", st)
	}
}

func obligationsReturning(obs ...M) H {
	return func(Call, *State) any { return R(M{"obligations": obs}) }
}

// js: an undischarged standing obligation at termination stops
func TestUndischargedObligation(t *testing.T) {
	out := run(t, stub(t, graph("plan0", unit("A")), Hs{"obligations": obligationsReturning(M{"statement": "perf numbers collected", "disposition": "undischarged", "carrier": "", "evidence": "none"})}, nil))
	eq(t, out.Status, "stopped")
	eq(t, out.Escalation.Stage, "obligations")
	eqs(t, out.stopKinds(), []string{"standing-obligation-undischarged"})
}

// js: an obligation carried by its own terms is a record with its carrier, and a stop without one
func TestCarriedObligation(t *testing.T) {
	obligation := func(carrier string) M {
		return M{"statement": "perf numbers carried until a later stream reads them", "disposition": "carried", "carrier": carrier, "evidence": "D12 says a later stream reads them"}
	}
	s := stub(t, graph("plan0", unit("A")), Hs{"obligations": obligationsReturning(obligation("docs/debt.md row 7"))}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	eq(t, out.Digest.ByKind["standing-obligation-carried"], 1)
	scribe := ledgerText(t, s, "ledger:w1")
	match(t, scribe, `append termination signal ledger entries`)
	match(t, scribe, "\n## Termination\n")
	match(t, scribe, `\| obligations \| reported \| standing-obligation-carried \| perf numbers carried until a later stream reads them \| docs/debt\.md row 7 \| recorded \|`)
	match(t, s.Prompt("obligations"), "recorded nowhere outside this plan is `undischarged`")

	un := run(t, stub(t, graph("plan0", unit("A")), Hs{"obligations": obligationsReturning(obligation(""))}, nil))
	eq(t, un.Status, "stopped")
	eqs(t, un.stopKinds(), []string{"standing-obligation-undischarged"})
	match(t, un.stops()[0].Statement, `^carried without a carrier: perf numbers`)
}

// js: every main-checkout stage that commits names its own files and is barred from sweeping or discarding anyone else's
func TestOwnFilesOnly(t *testing.T) {
	terminated := false
	s := stub(t, twoWaves(), merged(discovering(), Hs{
		"load": loadWith(M{"decisionsBlock": decisionsBlock("2026-09-24", "yes")}),
		"termination": func(_ Call, s *State) any {
			edits := []orchtest.DirectEdit{}
			if !terminated {
				edits = append(edits, orchtest.DirectEdit{Path: "README.md", Description: "mention A"})
			}
			terminated = true
			return R(M{"planSha": s.Plan.Sha, "appendedUnits": []any{}, "directEdits": edits})
		},
	}), M{"decisions": []M{{"id": "d1", "question": "q", "decision": "yes"}}})
	run(t, s)
	committing := []string{"decision-replan", "replan", "termination", "direct-edits"} // merge is code (D24)
	seen := map[string]bool{}
	for _, c := range agentCalls(s) {
		if !has(committing, c.Prefix) {
			continue
		}
		seen[c.Prefix] = true
		match(t, c.Prompt, "git add -- <paths>`, then `git commit -m <message> -- <paths>`, options before the `--`", c.Label)
		match(t, c.Prompt, `never retry it without the pathspec`, c.Label)
		match(t, c.Prompt, "Never run `git add -A`, `git add \\.`, `git add -u`, `git commit -a`, `git stash`, `git clean` or `git reset --hard`, and never stage", c.Label)
		match(t, c.Prompt, "You are in a clone of the repository on integration, made for this stage; the runtime fast-forwards integration to your commits after you return", c.Label)
	}
	eq(t, len(seen), len(committing), seen)
	at := func(prefix string) string {
		for _, c := range agentCalls(s) {
			if strings.HasPrefix(c.Label, prefix) {
				return c.Prompt
			}
		}
		return ""
	}
	match(t, at("replan"), "Commit only the files this stage wrote \\(docs/plans/f-breakdown\\.md, and docs/architecture/f\\.md if you made a `document-text` correction\\)")
}

// js: decisions are committed first, applied by a re-plan, and both SHAs reach every later prompt
func TestDecisionsFirst(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"load": loadWith(M{"decisionsBlock": decisionsBlock("2026-09-12", "yes")}), "ledger": func(Call, *State) any { return nil }},
		M{"decisions": []M{{"id": "d1", "question": "q", "decision": "yes"}}})
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	eqs(t, s.Labels()[:4], []string{"gates-pin", "decision-log", "decision-replan", "ledger:w0"})
	for _, c := range agentCalls(s) {
		match(t, c.Prompt, `Decision log: docs/plans/f-decisions\.md @ dec1`, c.Label)
	}
	for _, l := range filterPrefix(s.Labels(), "load") {
		eq(t, input(t, s, l)["decisionLogSha"], any("dec1"), l)
	}
	rp := s.Prompt("decision-replan")
	match(t, rp, `Decomposition plan: docs/plans/f-breakdown\.md @ plan0`)
	match(t, rp, `(?s)"id": "d1".*"decision": "yes"`)
	noMatch(t, rp, `"clears"`)
	eq(t, input(t, s, "load")["sha"], any("decided1"), "loads the re-planned SHA")
	eq(t, out.LedgerPath, "docs/plans/f-signals/plan0.md", "the ledger is named by the SHA the launch started from, not the decision re-plan's")
	eq(t, input(t, s, "ledger:w0")["path"], any("docs/plans/f-signals/plan0.md"))
	match(t, rp, `### <date, date -I> — decisions d1`)
	match(t, rp, `<kind> <unit> — <one clause>`)
	match(t, s.Prompt("plan:A"), `@ decided1`)
	w0 := []string{}
	for _, r := range out.Unscribed {
		if r.Wave != nil && *r.Wave == 0 {
			w0 = append(w0, r.Stage+"/"+r.Kind)
		}
	}
	eqs(t, w0, []string{"decision-replan/verdict", "decision-replan/amendment"})
	match(t, ledgerText(t, s, "ledger:w0"), `(?s)## Wave 0.*decision-replan \| reported \| verdict \| d1 amended: edge added — q`)
}

// js: a decision the re-plan cannot carry out stops before load, with both SHAs in the record
func TestDecisionCannot(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"decision-replan": func(Call, *State) any {
		return R(M{"planSha": "decided1", "amendments": []any{}, "overBudget": []any{}, "verdicts": []M{{"id": "d1", "verdict": "cannot", "statement": "changes §4"}}, "summary": "s"})
	}}, M{"decisions": []M{{"id": "d1", "question": "q", "decision": "rewrite the seam"}}})
	out := run(t, s)
	eq(t, out.Status, "stopped")
	eq(t, out.Escalation.Stage, "decision-replan")
	eqs(t, out.stopKinds(), []string{"plan-defect-unabsorbed"})
	match(t, out.stops()[0].Statement, `cannot absorb: changes §4 — q`)
	eq(t, out.Escalation.PlanSha, "decided1")
	eq(t, *out.Escalation.PreReplanSha, "plan0")
	if has(s.Labels(), "load") {
		t.Error(s.Labels())
	}
}

// js: a standing over-budget flag reported at decision-replan does not stop the relaunch
func TestDecisionReplanStandingFlag(t *testing.T) {
	s := stub(t, twoWaves(), Hs{
		"load": loadWith(M{"decisionsBlock": decisionsBlock("2026-09-12", "proceed")}),
		"decision-replan": func(Call, *State) any {
			return R(M{"planSha": "decided1", "amendments": []any{}, "overBudget": []M{{"unit": "B", "disposition": "flagged", "statement": "D7 stands"}},
				"verdicts": []M{{"id": "d1", "verdict": "no-change", "statement": "D7 reaffirmed"}}, "summary": "s"})
		},
		"ledger": func(Call, *State) any { return nil },
	}, M{"decisions": []M{{"id": "d1", "question": "q", "decision": "proceed, no split"}}})
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	if !has(s.Labels(), "plan:A") {
		t.Error(s.Labels())
	}
	w0 := []string{}
	for _, r := range out.Unscribed {
		if r.Wave != nil && *r.Wave == 0 {
			w0 = append(w0, r.Kind)
		}
	}
	eqs(t, w0, []string{"verdict", "over-budget-flagged"})
	eq(t, out.Waves, 2)
	eq(t, out.Digest.Waves, 0, "silent waves leave no ledger entries; wave 0 is not counted")
}

// js: a null agent return is a stop
func TestNullReturnStops(t *testing.T) {
	out := run(t, stub(t, twoWaves(), Hs{"implement": func(Call, *State) any { return nil }}, nil))
	eq(t, out.Status, "stopped")
	if !has(out.stopKinds(), "agent-null") {
		t.Error(out.stopKinds())
	}
}

// js: required args are enforced
func TestRequiredArgsEnforced(t *testing.T) {
	s := stub(t, twoWaves(), nil, M{"planSha": nil})
	if _, err := Run(t.Context(), s); err == nil || !strings.Contains(err.Error(), "planSha") {
		t.Errorf("err %v", err)
	}
}

// js: a clean merge that regresses the suite stops and does not complete the unit
func TestMergeRegression(t *testing.T) {
	out := run(t, stub(t, twoWaves(), Hs{"merge": func(Call, *State) any {
		return R(M{"integrationBranch": "integration", "merges": []plan.Merge{{Unit: "A", Status: "regressed", Files: []string{"test/a.test.js"}, Sha: "m"}}, "planSha": "m1"})
	}}, nil))
	eq(t, out.Status, "stopped")
	eqs(t, out.stopKinds(), []string{"merge-regression"})
}

// js: fixed Critical findings reach the ledger on a passing gate
func TestFixedCriticalsInLedger(t *testing.T) {
	round := 0
	s := stub(t, twoWaves(), Hs{"critique-plan": func(Call, *State) any {
		round++
		if round%2 == 1 {
			return critical1("plan")
		}
		return noFinding()
	}}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	eq(t, out.Digest.ByKind["critical-fixed"], 2)
	match(t, ledgerText(t, s, "ledger:w1"), `critical-fixed`)
}

// js: role tiers reach every call: critique opus/low, gate opus/medium, planning opus/high, mechanical haiku/low, default opus/low
func TestRoleTiers(t *testing.T) {
	s := stub(t, twoWaves(), discovering(), nil)
	run(t, s)
	seen := map[string]bool{}
	for _, c := range agentCalls(s) {
		role := c.Prefix
		seen[role] = true
		got := c.Opts.Model + "/" + c.Opts.Effort
		want := "opus/low"
		switch role {
		case "review-gate":
			want = "opus/medium"
		case "plan", "replan", "termination":
			want = "opus/high"
		case "load", "ledger", "decision-log", "cleanup", "completion-report":
			want = "haiku/low"
		}
		eq(t, got, want, c.Label)
	}
	for _, r := range []string{"critique-code", "plan", "implement", "review-gate", "replan", "obligations"} {
		if !seen[r] {
			t.Error(r)
		}
	}
}

// js: a wave stop scribes once, with the wave's records and stops in one rendered table
func TestWaveStopScribesOnce(t *testing.T) {
	s := stub(t, twoWaves(), Hs{
		"implement":   implemented(M{"deviations": []string{"a | b"}}),
		"review-gate": func(Call, *State) any { return R(M{"invalidations": []M{gateInvalidation(nil)}, "notes": []any{}}) },
	}, nil)
	run(t, s)
	eqs(t, filterPrefix(s.Labels(), "ledger:w1"), []string{"ledger:w1"})
	p := ledgerText(t, s, "ledger:w1")
	match(t, p, "\n## Wave 1\n")
	match(t, p, `\| 1 \| A \| implement \| reported \| deviation \| a \\\| b \| A \| recorded \|`)
	match(t, p, `\| architecture-invalidated \| section: §4\.8; [^|]* \| §4\.8 \| stopped \|`)
	eq(t, input(t, s, "ledger:w1")["name"], any("f"), "a new ledger starts with the heading for the name")
}

// js: args.tiers overrides a role without disturbing the others
func TestTiersOverride(t *testing.T) {
	s := stub(t, twoWaves(), nil, M{"tiers": M{"critique": M{"model": "sonnet"}}})
	run(t, s)
	first := func(prefix string) Call {
		for _, c := range s.Calls {
			if strings.HasPrefix(c.Label, prefix) {
				return c
			}
		}
		t.Fatal(prefix)
		return Call{}
	}
	eq(t, first("critique-code").Opts.Model, "sonnet")
	eq(t, first("implement").Opts.Model, "opus")
	eq(t, first("review-gate").Opts.Model, "opus")
	eq(t, first("plan:").Opts.Model, "opus")
}

// js: load: a bad block stops before any unit; the copy stage returns only the blocks
func TestLoadBadBlock(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"load": func(Call, *State) any {
		return R(M{"planBlock": "units: [", "decisionsBlock": "", "headBranch": "integration", "architectureSha": "a"})
	}}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	eq(t, out.Escalation.Stage, "load")
	if st := out.stops(); len(st) != 1 || st[0].Kind != "invalid-plan" || st[0].Source != "structural" {
		t.Errorf("%+v", st)
	}
	if hasPrefix(s.Labels(), "plan:") {
		t.Error(s.Labels())
	}
}

// js: load: a decision log clears an organisational gate; clearing an environment gate is a stop
func TestLoadDecisionClears(t *testing.T) {
	gated := func() *plan.Plan {
		u := unit("A")
		pin := "22"
		u.Gates = []plan.Gate{{Name: "legal", Kind: "organisational"}, {Name: "node", Kind: "environment", Pin: &pin}}
		return graph("plan0", u)
	}
	log := func(clears string) string {
		return strings.Join([]string{"```yaml lsf-decisions", "decisions:", "  - id: D1", "    date: 2026-09-11", "    question: q", "    decision: d", "    clears: [" + clears + "]", "```"}, "\n")
	}
	ok := stub(t, gated(), Hs{"load": loadWith(M{"decisionsBlock": log("A/legal")})}, M{"decisionLogSha": "dec0"})
	outOk := run(t, ok)
	eq(t, outOk.Status, "complete", outOk.stopKinds())
	eq(t, input(t, ok, "load")["decisionLogSha"], any("dec0"))
	eq(t, input(t, ok, "load")["decisionLogPath"], any("docs/plans/f-decisions.md"))

	outBad := run(t, stub(t, gated(), Hs{"load": loadWith(M{"decisionsBlock": log("A/node")})}, M{"decisionLogSha": "dec0"}))
	eq(t, outBad.Status, "stopped")
	match(t, outBad.stops()[0].Statement, `environment gate A/node`)

	outMissing := run(t, stub(t, gated(), Hs{"load": loadWith(nil)}, M{"decisionLogSha": "dec0"}))
	eq(t, outMissing.Status, "stopped")
	eq(t, outMissing.stops()[0].Referent, "lsf-decisions")
}

// js: the re-planned and terminated plans are re-read from their SHAs, not returned inline
func TestReloadsFromSha(t *testing.T) {
	s := stub(t, twoWaves(), discovering(), nil)
	run(t, s)
	eqs(t, filterPrefix(s.Labels(), "load"), []string{"load", "load:w1", "load:w2", "load:termination"})
	match(t, input(t, s, "load:w1")["sha"].(string), `^replan\d$`)
	match(t, s.Prompt("plan:B"), `@ replan\d`)
}

// js: the append stages are told the commit form and the fence placement; every stage copies the short SHA from git
func TestAppendStagePrompts(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"load": loadWith(M{"decisionsBlock": decisionsBlock("2026-09-19", "yes")}), "implement": implemented(M{"deviations": []string{"d"}})},
		M{"decisions": []M{{"id": "d1", "question": "q", "decision": "yes"}}})
	run(t, s)
	match(t, ledgerText(t, s, "ledger:w1"), `^docs\(f\): append wave 1 signal ledger entries\n`)
	log := input(t, s, "decision-log")
	eq(t, log["subject"], any("docs(f): append decision(s) d1"))
	eq(t, log["path"], any("docs/plans/f-decisions.md"))
	for _, c := range agentCalls(s) {
		match(t, c.Prompt, `git rev-parse --short HEAD`, c.Label)
	}
}

// js: a seeded stage return stands in for the agent, matched by label
func TestSeededReturn(t *testing.T) {
	planned := R(M{"branch": "salvaged-A", "sha": "s", "path": "docs/plans/A-plan.md", "bytes": 0, "readings": []any{}})
	s := stub(t, twoWaves(), nil, M{"seed": M{"plan:A": planned}})
	out := run(t, s)
	eq(t, out.Status, "complete")
	if has(s.Labels(), "plan:A") || !has(s.Labels(), "plan:B") {
		t.Error("the seeded agent is not spawned; unseeded labels run", s.Labels())
	}
	match(t, s.Prompt("critique-plan:r1:A"), `git show salvaged-A:docs/plans/A-plan\.md`, "the critique reads the seeded plan from its branch")
	match(t, s.Prompt("implement:A"), `salvaged-A`)
	if !hasPrefix(s.Logs, "plan:A: seeded") {
		t.Error(s.Logs)
	}
}

// js: args.unscribed rows are scribed first, keep their wave, and stay out of the digest
func TestCarriedRows(t *testing.T) {
	carried := []M{{"kind": "deviation", "source": "reported", "stage": "implement", "unit": "Z", "wave": 3, "statement": "dev-Z", "referent": "src/Z", "raisedBy": "implement", "disposition": "recorded"}}
	s := stub(t, twoWaves(), nil, M{"unscribed": carried})
	out := run(t, s)
	eq(t, out.Status, "complete")
	eqs(t, s.Labels()[:2], []string{"gates-pin", "ledger:carried"})
	match(t, ledgerText(t, s, "ledger:carried"), `(?s)## Wave 3.*dev-Z`)
	eq(t, len(out.Unscribed), 0)
	if _, ok := out.Digest.ByKind["deviation"]; ok {
		t.Error("carried rows stay out of the digest")
	}
	again := run(t, stub(t, twoWaves(), Hs{"ledger": func(Call, *State) any { return nil }}, M{"unscribed": carried}))
	if len(again.Unscribed) != 1 || again.Unscribed[0].Statement != "dev-Z" {
		t.Error(again.Unscribed)
	}
	_, err := Run(t.Context(), stub(t, twoWaves(), nil, M{"unscribed": []M{{"statement": "no kind"}}}))
	if err == nil || !strings.Contains(err.Error(), "kind is required") {
		t.Errorf("err %v", err)
	}
}

// pausingMerge is the default merge with pauseRequested set.
func pausingMerge() H {
	base := orchtest.Defaults()["merge"]
	return func(c Call, s *State) any {
		r := base(c, s).(M)
		r["result"].(M)["pauseRequested"] = true
		return r
	}
}

// js: a pause ref seen by the merge stage pauses the run at the wave boundary, after the ledger is scribed
func TestPauseAtMerge(t *testing.T) {
	s := stub(t, twoWaves(), merged(Hs{"merge": pausingMerge()}, discovering()), nil)
	out := run(t, s)
	eq(t, out.Status, "paused")
	eq(t, out.Waves, 1)
	match(t, out.Note, `refs/lsf/f/pause`)
	match(t, out.Note, `1 unit\(s\) remain`)
	match(t, out.PlanSha, `^replan`, "the wave's re-plan committed before the pause")
	if out.Escalation != nil {
		t.Error("escalation")
	}
	l := s.Labels()
	if !has(l, "replan:w1") || !has(l, "ledger:w1") || has(l, "plan:B") || has(l, "implement:B") {
		t.Error(l)
	}
	if !hasPrefix(s.Logs, "pause requested (refs/lsf/f/pause seen by merge:w1)") {
		t.Error(s.Logs)
	}
	eq(t, input(t, s, "merge:w1")["pauseRef"], any("refs/lsf/f/pause"))
	eq(t, input(t, s, "ledger:w1")["pauseRef"], any("refs/lsf/f/pause"))
}

// js: a pause ref present at load pauses before any unit starts; args.pauseRef names the ref
func TestPauseAtLoad(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"load": loadWith(M{"pauseRequested": true})}, M{"pauseRef": "refs/custom/pause"})
	out := run(t, s)
	eq(t, out.Status, "paused")
	eq(t, out.Waves, 0)
	eq(t, out.PlanSha, "plan0")
	match(t, out.Note, `refs/custom/pause`)
	if hasPrefix(s.Labels(), "plan:") || hasPrefix(s.Labels(), "implement:") {
		t.Error(s.Labels())
	}
	eq(t, input(t, s, "load")["pauseRef"], any("refs/custom/pause"))
}

// js: a pause ref seen by the ledger scribe on a wave with nothing to absorb pauses the run
func TestPauseAtLedger(t *testing.T) {
	s := stub(t, twoWaves(), Hs{
		"implement": deviating(),
		"ledger": func(Call, *State) any {
			return R(M{"path": "docs/plans/f-signals.md", "sha": "l", "pauseRequested": true})
		},
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "paused")
	eq(t, out.Waves, 1)
	eq(t, len(out.Unscribed), 0)
	if has(s.Labels(), "implement:B") {
		t.Error(s.Labels())
	}
}

// js: a stop outranks a pause seen in the same wave
func TestStopOutranksPause(t *testing.T) {
	out := run(t, stub(t, twoWaves(), Hs{"merge": func(Call, *State) any {
		return R(M{"integrationBranch": "integration", "merges": []plan.Merge{{Unit: "A", Status: "regressed", Files: []string{"test/a.test.js"}, Sha: "m"}}, "planSha": "m1", "pauseRequested": true})
	}}, nil))
	eq(t, out.Status, "stopped")
	eqs(t, out.stopKinds(), []string{"merge-regression"})
}

// js: a pause at load still commits the decisions and returns the decision re-plan's SHA
func TestPauseAtLoadAfterDecisions(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"load": loadWith(M{"decisionsBlock": decisionsBlock("2026-09-12", "yes"), "pauseRequested": true})},
		M{"decisions": []M{{"id": "d1", "question": "q", "decision": "yes"}}})
	out := run(t, s)
	eq(t, out.Status, "paused")
	eq(t, out.PlanSha, "decided1")
	eqs(t, s.Labels()[:3], []string{"gates-pin", "decision-log", "decision-replan"})
	match(t, out.Note, `no decisions`)
	if hasPrefix(s.Labels(), "plan:") {
		t.Error(s.Labels())
	}
}

// js: a pause seen by a reload or the termination load pauses before the next wave
func TestPauseAtReload(t *testing.T) {
	for _, at := range []string{"load:w1", "load:termination"} {
		s := stub(t, twoWaves(), merged(discovering(), Hs{"load": func(c Call, s *State) any {
			return orchtest.Copied(s.Plan, M{"pauseRequested": c.Label == at})
		}}), nil)
		out := run(t, s)
		eq(t, out.Status, "paused", at)
		if at == "load:w1" && has(s.Labels(), "plan:B") || at == "load:termination" && hasPrefix(s.Labels(), "obligations") {
			t.Error(at, s.Labels())
		}
	}
}

// js: a paused return carries a null scribe's rows in unscribed and says to pass them back
func TestPausedReturnsUnscribed(t *testing.T) {
	out := run(t, stub(t, twoWaves(), Hs{"merge": pausingMerge(), "implement": deviating(), "ledger": func(Call, *State) any { return nil }}, nil))
	eq(t, out.Status, "paused")
	if len(out.Unscribed) != 1 || out.Unscribed[0].Statement != "dev-A" {
		t.Error(out.Unscribed)
	}
	match(t, out.Note, `passing unscribed back`)
}

// js: a seeded return's pause report is ignored
func TestSeededPauseIgnored(t *testing.T) {
	st := &State{Plan: twoWaves(), Completed: map[string]bool{}, MergeOrder: []string{"A"}}
	seeded := orchtest.Defaults()["merge"](Call{}, st).(M)
	seeded["result"].(M)["pauseRequested"] = true
	out := run(t, stub(t, twoWaves(), nil, M{"seed": M{"merge:w1": seeded}}))
	eq(t, out.Status, "complete", out.stopKinds())
}

// ---------------------------------------------------------------- architecture pin

const doc = "docs/architecture/f.md"

func archHeader(sha string) string { return "Architecture document: " + doc + " @ " + sha }

// mergeMoving is a merge reporting the architecture SHA after each wave
// (absent when the wave has no entry), and per-unit merge SHAs (none when
// shaOf returns "").
func mergeMoving(archByWave map[int]any, shaOf func(id string) string) H {
	if shaOf == nil {
		shaOf = func(id string) string { return "m-" + id }
	}
	n := 0
	return func(_ Call, s *State) any {
		n++
		in := map[string]bool{}
		for _, id := range s.MergeOrder {
			in[id] = true
		}
		np := *s.Plan
		np.Sha = fmt.Sprintf("merged%d", n)
		np.Units = nil
		for _, u := range s.Plan.Units {
			cu := *u
			cu.Complete = s.Completed[u.ID] || in[u.ID]
			np.Units = append(np.Units, &cu)
		}
		s.Plan = &np
		ms := []plan.Merge{}
		for _, id := range s.MergeOrder {
			ms = append(ms, plan.Merge{Unit: id, Status: "clean", Files: []string{}, Sha: shaOf(id)})
		}
		r := M{"integrationBranch": "integration", "merges": ms, "planSha": np.Sha}
		if a, ok := archByWave[n]; ok {
			r["architectureSha"] = a
		}
		return R(r)
	}
}

// js: a unit that amends the architecture document: the gate reads the amended text and judges the unit's own diff against the pre-wave text; later stages read the amended text
func TestArchitectureAmendedByUnit(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"merge": mergeMoving(map[int]any{1: "arch2", 2: "arch2"}, nil)}, nil)
	out := run(t, s)
	eq(t, out.Status, "complete")
	gate := s.Prompt("review-gate:w1")
	contains(t, gate, archHeader("arch2"), "the gate header names the amended text")
	if strings.Contains(gate, archHeader("arch1")) {
		t.Error("stale pin in the gate")
	}
	match(t, gate, `changed during this wave, from arch1 to the SHA above`)
	contains(t, gate, "- A (foundation): `git diff m-A^1 m-A -- "+doc+"`")
	match(t, gate, `Judge each unit's change against the text it was merged onto \(the diff's first commit\), which includes changes merged before it this wave`)
	match(t, gate, "alters what a section decided beyond that is an invalidation, all five fields filled: `section`; `decision`, what the section decided before the change; `premise`, what that decision rested on; `evidence`, the merge SHA and the lines of the diff; `amendment`, the text the unit wrote")
	match(t, gate, "goes under `amendments`")
	for _, l := range []string{"plan:B", "critique-plan:r1:B", "implement:B", "critique-code:r1:B", "review-gate:w2"} {
		p := s.Prompt(l)
		contains(t, p, archHeader("arch2"), l+" reads the amended text")
		if strings.Contains(p, "@ arch1") {
			t.Error(l + " names a stale pin")
		}
	}
	if strings.Contains(s.Prompt("review-gate:w2"), "changed during this wave") {
		t.Error("wave 2 left the document where wave 1 put it")
	}
	contains(t, s.Prompt("plan:A"), archHeader("arch1"), "wave 1 ran against the load-time pin")
}

// js: a unit-carried change to a decision reaches the gate as a candidate invalidation and stops the run
func TestUnitChangeToDecisionInvalidates(t *testing.T) {
	s := stub(t, twoWaves(), Hs{
		"merge": mergeMoving(map[int]any{1: "arch2"}, nil),
		"review-gate": func(c Call, _ *State) any {
			invs := []M{}
			if strings.Contains(c.Prompt, "git diff m-A^1 m-A") {
				invs = append(invs, M{"section": "§5", "decision": "ABI is undisclosed", "premise": "no consumer needs it", "evidence": "m-A", "amendment": "ABI is disclosed per §7.2"})
			}
			return R(M{"invalidations": invs, "notes": []any{}, "amendments": []any{}})
		},
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	eq(t, out.Escalation.PlanSha, "merged1")
	if st := out.stops(); len(st) != 1 || st[0].Kind != "architecture-invalidated" || st[0].Referent != "§5" {
		t.Errorf("%+v", st)
	}
	match(t, out.Escalation.DecisionNeeded[0].Question, `evidence: m-A; amendment: ABI is disclosed per §7\.2`)
	if has(s.Labels(), "plan:B") {
		t.Error(s.Labels())
	}
}

func gateAmending(statement string) H {
	return func(c Call, _ *State) any {
		as := []M{}
		if strings.Contains(c.Prompt, "changed during this wave") {
			as = append(as, M{"unit": "A", "section": "§7.2", "statement": statement})
		}
		return R(M{"invalidations": []any{}, "notes": []any{}, "amendments": as})
	}
}

// js: a unit's amendment that moves no decision is recorded with its unit, listed in the digest, and stops nothing
func TestUnitAmendmentRecorded(t *testing.T) {
	out := run(t, stub(t, twoWaves(), Hs{"merge": mergeMoving(map[int]any{1: "arch2"}, nil), "review-gate": gateAmending("records the decision A settled"), "ledger": func(Call, *State) any { return nil }}, nil))
	eq(t, out.Status, "complete")
	rows := []string{}
	for _, r := range out.Unscribed {
		if r.Kind == "architecture-amendment" {
			rows = append(rows, fmt.Sprintf("%s %s %s %s %d", r.Unit, r.Source, r.Referent, r.Disposition, *r.Wave))
		}
	}
	eqs(t, rows, []string{"A gate §7.2 recorded 1"})
	eqs(t, out.Digest.Attention.ArchitectureAmendments, []string{"A: records the decision A settled"})
}

// js: a document that did not move leaves the gate prompt as before; an empty or missing report keeps the pin
//
// The {1: 42} case is a wrong-typed field: the merge return does not
// decode, so the merge is null and the run stops there (D9); in JS the
// number is ignored and the pin kept.
func TestUnmovedDocumentKeepsPin(t *testing.T) {
	for _, arch := range []map[int]any{{1: "arch1", 2: "arch1"}, {1: "", 2: "  "}, {}} {
		s := stub(t, twoWaves(), Hs{"merge": mergeMoving(arch, nil)}, nil)
		out := run(t, s)
		eq(t, out.Status, "complete")
		for _, l := range []string{"review-gate:w1", "plan:B", "review-gate:w2"} {
			p := s.Prompt(l)
			contains(t, p, archHeader("arch1"), fmt.Sprint(arch), l, " keeps arch1")
			if strings.Contains(p, "changed during this wave") {
				t.Error(arch, l)
			}
		}
	}
	s := stub(t, twoWaves(), Hs{"merge": mergeMoving(map[int]any{1: 42}, nil)}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	if st := out.stops(); len(st) != 1 || st[0].Kind != "agent-null" || st[0].Stage != "merge" {
		t.Errorf("%+v", st)
	}
	if !hasPrefix(s.Logs, "merge:w1: return does not decode (") {
		t.Error(s.Logs)
	}
}

// js: a re-plan reload and the termination reload refresh the pin from the copy stage
func TestReloadRefreshesPin(t *testing.T) {
	s := stub(t, twoWaves(), merged(discovering(), Hs{
		"load": func(c Call, s *State) any {
			a := map[string]string{"load:w1": "arch3", "load:termination": "arch4"}[c.Label]
			if a == "" {
				a = "arch1"
			}
			return orchtest.Copied(s.Plan, M{"architectureSha": a})
		},
		"merge": mergeMoving(map[int]any{1: "arch1", 2: "arch3"}, nil),
	}), nil)
	out := run(t, s)
	eq(t, out.Status, "complete")
	contains(t, s.Prompt("replan:w1"), archHeader("arch1"))
	contains(t, s.Prompt("plan:B"), archHeader("arch3"), "the re-plan's document-text correction reaches wave 2")
	if strings.Contains(s.Prompt("review-gate:w2"), "changed during this wave") {
		t.Error("review-gate:w2")
	}
	contains(t, s.Prompt("obligations"), archHeader("arch4"))
}

// js: a merge with no SHA for a unit falls back to the whole-wave diff; args.architectureSha is the load-time pin and the baseline
func TestMergeWithoutSha(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"merge": mergeMoving(map[int]any{1: "arch2"}, func(string) string { return "" })}, M{"architectureSha": "argpin"})
	out := run(t, s)
	eq(t, out.Status, "complete")
	contains(t, s.Prompt("plan:A"), archHeader("argpin"))
	gate := s.Prompt("review-gate:w1")
	match(t, gate, `from argpin to the SHA above`)
	contains(t, gate, "- A (foundation): merge SHA not reported; `git diff argpin arch2 -- "+doc+"` holds its change among any others, judged against the document @ argpin")
}

// js: the merge stage reports the architecture SHA after its last commit, and the gate returns amendments
// Go: the merge step is given the document and reads its SHA from git (D24).
func TestMergeReportsArchitectureSha(t *testing.T) {
	s := stub(t, twoWaves(), nil, nil)
	run(t, s)
	eq(t, input(t, s, "merge:w1")["architectureDoc"], any(doc))
	gate := s.CallOf("review-gate:w1")
	if !has(schemaOf(t, gate).Properties.Result.Required, "amendments") {
		t.Error("gate schema")
	}
	match(t, gate.Prompt, "Return `amendments`: a merged unit's own change to the architecture document that moves no decision, as described above when the document moved; an empty array when it did not\\.")
}

// js: the pin stays on a null merge and advances, ungated, when nothing merged; with no load-time pin each unit is judged against its merge's first parent
func TestPinOnNullMergeAndUnpinned(t *testing.T) {
	nullMerge := stub(t, twoWaves(), Hs{"merge": func(Call, *State) any { return nil }}, nil)
	eq(t, run(t, nullMerge).Status, "stopped")
	if hasPrefix(nullMerge.Labels(), "review-gate") {
		t.Error(nullMerge.Labels())
	}
	refused := stub(t, twoWaves(), Hs{"merge": func(_ Call, s *State) any {
		ms := []plan.Merge{}
		for _, id := range s.MergeOrder {
			ms = append(ms, plan.Merge{Unit: id, Status: "unresolvable", Files: []string{"x"}})
		}
		return R(M{"integrationBranch": "integration", "merges": ms, "planSha": s.Plan.Sha, "architectureSha": "arch2"})
	}}, nil)
	eq(t, run(t, refused).Status, "stopped")
	if hasPrefix(refused.Labels(), "review-gate") {
		t.Error("nothing merged, no gate")
	}
	unpinned := stub(t, twoWaves(), Hs{"load": loadWith(M{"architectureSha": ""}), "merge": mergeMoving(map[int]any{1: "arch2"}, nil)}, nil)
	eq(t, run(t, unpinned).Status, "complete")
	gate := unpinned.Prompt("review-gate:w1")
	match(t, gate, `from an unrecorded pin to the SHA above`)
	contains(t, gate, "- A (foundation): `git diff m-A^1 m-A -- "+doc+"`")
}

// js: a gate amendment already prefixed with its unit is not prefixed twice
func TestGateAmendmentPrefix(t *testing.T) {
	out := run(t, stub(t, twoWaves(), Hs{"merge": mergeMoving(map[int]any{1: "arch2"}, nil), "review-gate": gateAmending("A: settled §7.2"), "ledger": func(Call, *State) any { return nil }}, nil))
	eqs(t, out.Digest.Attention.ArchitectureAmendments, []string{"A: settled §7.2"})
}

// A stage result with a wrong-typed field is that stage's null (D9): the
// run stops with an escalation, not an error.
func TestDecodeFailureIsAgentNull(t *testing.T) {
	s := stub(t, twoWaves(), Hs{"implement": implemented(M{"branch": 5})}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	if st := out.stops(); len(st) != 1 || st[0].Kind != "agent-null" || st[0].Stage != "implement" || st[0].Unit != "A" {
		t.Errorf("%+v", st)
	}
	if !hasPrefix(s.Logs, "implement:A: return does not decode (") {
		t.Error(s.Logs)
	}
}

// An integral float for a schema integer decodes, as the validator passes it.
func TestIntegralFloatDecodes(t *testing.T) {
	s := stub(t, twoWaves(), Hs{
		"plan": func(c Call, _ *State) any {
			return R(M{"branch": "plan-" + c.Unit, "sha": "p", "path": "docs/plans/" + c.Unit + "-plan.md", "bytes": 30720.0, "readings": []any{}})
		},
		"ledger": func(Call, *State) any { return nil },
	}, M{"maxUnitsPerWave": 2.0})
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	eq(t, out.Digest.ByKind["document-oversize"], 2)
}

// Concurrent unit branches give the sequential run's outcome.
func TestConcurrentParallelMatchesSequential(t *testing.T) {
	outs := []string{}
	for _, concurrent := range []bool{false, true} {
		s := stub(t, orchtest.TwoParallel(), discovering(), nil)
		s.Concurrent = concurrent
		outs = append(outs, run(t, s).raw)
	}
	eq(t, outs[1], outs[0])
}

// Labels stay unique when a stop scribes after the wave's own scribe.
func TestStopAfterScribeRelabelsLedger(t *testing.T) {
	s := stub(t, twoWaves(), Hs{
		"implement":   deviating(),
		"obligations": obligationsReturning(M{"statement": "s", "disposition": "undischarged", "carrier": "", "evidence": ""}),
	}, nil)
	out := run(t, s)
	eq(t, out.Status, "stopped")
	eqs(t, filterPrefix(s.Labels(), "ledger"), []string{"ledger:w1", "ledger:w2", "ledger:w2#2"})
}

// A unit thunk that panics is that unit's null (JS: parallel's catch); the
// other unit in the wave still merges.
func TestPanickingUnitIsUnitNull(t *testing.T) {
	s := stub(t, orchtest.TwoParallel(), Hs{"implement": func(c Call, _ *State) any {
		if c.Unit == "A" {
			panic("boom")
		}
		return R(orchtest.Implemented(c.Unit, nil))
	}}, nil)
	s.ExpectPanic = true
	out := run(t, s)
	eq(t, out.Status, "stopped")
	if st := out.stops(); len(st) != 1 || st[0].Kind != "agent-null" || st[0].Stage != "unit" || st[0].Unit != "A" {
		t.Errorf("%+v", st)
	}
	units := input(t, s, "merge:w1")["units"].([]any)
	if len(units) != 1 || units[0].(map[string]any)["unit"] != "B" {
		t.Errorf("merge units %v", units)
	}
}

// A runtime stop from any call, in a unit thunk or the main lane, ends Run
// with the stop and no outcome.
func TestRuntimeStopPropagates(t *testing.T) {
	stop := &lsfrt.StopError{Reason: lsfrt.StopSigint}
	for _, at := range []string{"implement", "lens-review", "merge", "review-gate", "ledger", "obligations"} {
		s := stub(t, twoWaves(), merged(discovering(), Hs{at: func(Call, *State) any { return stop }}), nil)
		out, err := Run(t.Context(), s)
		if err != stop || out != nil {
			t.Errorf("%s: %s %v", at, out, err)
		}
	}
}

// Every role reaches the stages it governs.
func TestEveryRoleTier(t *testing.T) {
	tiers := M{}
	for _, r := range []string{"critique", "gate", "planning", "default", "mechanical"} {
		tiers[r] = M{"model": "m-" + r, "effort": "e-" + r}
	}
	round := 0
	s := stub(t, twoWaves(), merged(discovering(), Hs{
		"critique-plan": func(Call, *State) any {
			round++
			if round == 1 {
				return critical1("plan")
			}
			return noFinding()
		},
		"critique-code": criticalOnce(),
		"fix-code":      fixCode(g("pytest -q", "green", "1"), g("ruff check .", "green", "")),
		"replan": func(c Call, s *State) any {
			s.Plan = completeFrom(s, "rp")
			return R(M{"planSha": "rp", "amendments": []any{}, "overBudget": []any{}, "verdicts": orchtest.VerdictsFor(c.Prompt, "amended"),
				"directEdits": []orchtest.DirectEdit{{Path: "a", Description: "b"}}, "summary": "s"})
		},
	}), M{"tiers": tiers})
	run(t, s)
	role := map[string]string{
		"critique-plan": "critique", "critique-code": "critique", "lens-review": "critique",
		"review-gate": "gate", "plan": "planning", "replan": "planning", "termination": "planning",
		"load": "mechanical", "gates-pin": "mechanical", "ledger": "mechanical", "cleanup": "mechanical", "completion-report": "mechanical",
		"implement": "default", "fix-plan": "default", "fix-code": "default", "merge": "default", "direct-edits": "default", "obligations": "default",
	}
	for _, c := range s.Calls {
		r, ok := role[c.Prefix]
		if !ok {
			t.Errorf("%s: no role listed", c.Label)
			continue
		}
		want := "m-" + r + "/e-" + r
		if c.Step {
			want = "/" // a code step has no tier
		}
		eq(t, c.Opts.Model+"/"+c.Opts.Effort, want, c.Label)
	}
}

// A wave cap below 1 is an args error (D16).
func TestMaxUnitsPerWaveBelowOne(t *testing.T) {
	for _, n := range []int{0, -1} {
		_, err := Run(t.Context(), stub(t, twoWaves(), nil, M{"maxUnitsPerWave": n}))
		if err == nil || !strings.Contains(err.Error(), "maxUnitsPerWave must be at least 1") {
			t.Errorf("%d: %v", n, err)
		}
	}
}

// A completion report with no branch or SHA aborts the unit (D17).
func TestCompletionReportWithoutBranch(t *testing.T) {
	for _, r := range []M{{"branch": "", "sha": "c", "path": "p"}, {"branch": "b", "sha": " ", "path": "p"}} {
		s := stub(t, twoWaves(), Hs{"completion-report": func(Call, *State) any { return R(r) }}, nil)
		out := run(t, s)
		eq(t, out.Status, "stopped")
		if st := out.stops(); len(st) != 1 || st[0].Kind != "agent-null" || st[0].Stage != "completion-report" || st[0].Referent != "docs/plans/A-completion.md" {
			t.Errorf("%+v", st)
		}
		if has(s.Labels(), "merge:w1") {
			t.Error("nothing to merge")
		}
	}
}
