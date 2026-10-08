package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

const twoGates = "gates:\n  - name: tests\n    command: make test\n  - name: lint\n    command: make lint\n"

func mustDecl(t *testing.T, body string) *plan.Declaration {
	t.Helper()
	d, problems := plan.ParseDeclaration(orchtest.GatesBlock(body))
	if len(problems) > 0 || d == nil {
		t.Fatalf("declaration: %v", problems)
	}
	return d
}

// S8: a planning stage that changes the gate block stops the run at its
// reload (the termination stage's here; a re-plan's reload is the same
// loadPlan).
func TestDeclaredGatesChangedStops(t *testing.T) {
	term := orchtest.Defaults()["termination"]
	s := stub(t, orchtest.TwoWaves(), Hs{"termination": func(c Call, st *State) any {
		st.GatesBlock = strings.Replace(twoGates, "make lint", "make lint-all", 1)
		return term(c, st)
	}}, nil)
	s.State.GatesBlock = twoGates
	v := run(t, s)
	eq(t, strings.Join(v.stopKinds(), ","), "declared-gates-changed")
	st := v.stops()[0]
	eq(t, st.Stage, "load")
	eq(t, v.Escalation.Stage, "termination")
	contains(t, st.Statement, `"command":"make lint"`)
	contains(t, st.Statement, `"command":"make lint-all"`)
}

// S8 through a wave's re-plan.
func TestDeclaredGatesChangedByReplan(t *testing.T) {
	replan := orchtest.Defaults()["replan"]
	s := stub(t, orchtest.TwoWaves(), merged(discovering(), Hs{"replan": func(c Call, st *State) any {
		st.GatesBlock = "setup: make deps\n" + twoGates
		return replan(c, st)
	}}), nil)
	s.State.GatesBlock = twoGates
	v := run(t, s)
	eq(t, strings.Join(v.stopKinds(), ","), "declared-gates-changed")
	eq(t, v.Escalation.Stage, "replan")
	eq(t, *v.stops()[0].Wave, 1)
}

// An unchanged block, or none, reloads without a stop; a malformed block
// is an invalid-plan load stop.
func TestDeclaredGatesLoad(t *testing.T) {
	s := stub(t, orchtest.TwoWaves(), nil, nil)
	s.State.GatesBlock = twoGates
	if v := run(t, s); v.Status != "complete" {
		t.Fatalf("unchanged block: %s", v.raw)
	}
	s = stub(t, orchtest.TwoWaves(), nil, nil)
	s.State.GatesBlock = "gates: []\n"
	v := run(t, s)
	eq(t, strings.Join(v.stopKinds(), ","), "invalid-plan")
	contains(t, v.stops()[0].Statement, "gates block: `gates` must be a non-empty sequence")
}

// The same tree is gated once in a launch: callers that arrive while the
// run is in flight wait for it.
func TestGatesMemoPerTree(t *testing.T) {
	s := stub(t, orchtest.TwoWaves(), nil, nil)
	release, started := make(chan struct{}), make(chan struct{}, 8)
	s.Gates = func(g orchtest.GateCall) ([]lsfrt.GateRun, error) {
		started <- struct{}{}
		<-release
		return orchtest.GateRuns(g.Gates, func(lsfrt.GateSpec) string { return lsfrt.GateGreen }, g.Commit), nil
	}
	o := stubOrch(t, s, nil)
	o.decl = mustDecl(t, twoGates)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			set, fault, err := o.gates(context.Background(), "c1")
			if err != nil || fault != "" || len(set.Runs) != 2 || len(set.failed()) != 0 {
				t.Errorf("%+v %q %v", set, fault, err)
			}
		})
	}
	<-started
	waitUntil(t, func() bool { // the other three are parked on the flight
		o.mu.Lock()
		defer o.mu.Unlock()
		return len(o.gateMemo) == 1
	})
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	o.gates(context.Background(), "c2")
	o.gates(context.Background(), "c1")
	eq(t, len(s.GateRuns), 2)
	eq(t, strings.Join(s.Labels(), ","), "gates:c1,gates:c2")
	eq(t, s.GateRuns[0].Gates[0].Command, "make test")
}

// A waiter on a run that was not kept runs the gates itself; one whose own
// context ends stops; a panicking run is a fault, not a hang.
func TestGatesFlightFailures(t *testing.T) {
	s := stub(t, orchtest.TwoWaves(), nil, nil)
	release, started := make(chan struct{}), make(chan struct{})
	n := 0
	s.Gates = func(g orchtest.GateCall) ([]lsfrt.GateRun, error) {
		n++ // calls are serial: the first blocks every other on its flight
		switch n {
		case 1:
			close(started)
			<-release
			return orchtest.GateRuns(g.Gates, func(lsfrt.GateSpec) string { return lsfrt.GateNotRun }, g.Commit), nil
		case 3:
			panic("boom")
		}
		return orchtest.GateRuns(g.Gates, func(lsfrt.GateSpec) string { return lsfrt.GateRed }, g.Commit), nil
	}
	o := stubOrch(t, s, nil)
	o.decl = mustDecl(t, twoGates)
	var owner, waiter gateSet
	var wg sync.WaitGroup
	wg.Go(func() { owner, _, _ = o.gates(context.Background(), "c1") })
	<-started
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := o.gates(cctx, "c1"); !isStop(err) {
		t.Errorf("cancelled waiter: %v", err)
	}
	wg.Go(func() { waiter, _, _ = o.gates(context.Background(), "c1") })
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	eq(t, len(owner.notRun()), 2)
	eq(t, len(waiter.failed()), 2)
	eq(t, len(waiter.notRun()), 0, "the waiter ran its own")
	if _, fault, err := o.gates(context.Background(), "c2"); err != nil || !strings.Contains(fault, "boom") {
		t.Errorf("panic: %q %v", fault, err)
	}
	eq(t, len(s.GateRuns), 3)
	// The stable label: a rerun of a tree keeps it.
	eq(t, strings.Join(s.Labels(), ","), "gates:c1,gates:c1,gates:c2")
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	for range 200 {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}

// A set with a gate not run is returned, not kept: the next ask reruns it.
func TestGatesNotRunNotKept(t *testing.T) {
	s := stub(t, orchtest.TwoWaves(), nil, nil)
	n := 0
	s.Gates = func(g orchtest.GateCall) ([]lsfrt.GateRun, error) {
		n++
		return orchtest.GateRuns(g.Gates, func(lsfrt.GateSpec) string {
			if n == 1 {
				return lsfrt.GateNotRun
			}
			return lsfrt.GateGreen
		}, g.Commit), nil
	}
	o := stubOrch(t, s, nil)
	o.decl = mustDecl(t, twoGates)
	set, _, _ := o.gates(context.Background(), "c1")
	eq(t, len(set.notRun()), 2)
	set, _, _ = o.gates(context.Background(), "c1")
	eq(t, len(set.failed()), 0)
	eq(t, len(s.GateRuns), 2)
}

// realGateLaunch runs body against a real runtime whose repo is r.
func realGateLaunch(t *testing.T, r *gitRepo, state string, body func(o *orch)) {
	t.Helper()
	cfg := lsfrt.Config{StateDir: state, Repo: r.dir, Roles: map[string]lsfrt.Role{"w": {Tools: "Read", PermissionMode: "dontAsk", Clone: "kept"}}, DefaultRole: "w"}
	s, err := lsfrt.Run(context.Background(), cfg, func(ctx context.Context, rt *lsfrt.Runtime) (json.RawMessage, error) {
		body(newOrch(rt, runCtxFor(t, nil), nil))
		return nil, nil
	})
	if err != nil || s.StopReason != lsfrt.StopReturned {
		t.Fatalf("launch: %v %+v", err, s)
	}
}

// S5, first half: a tree gated in one launch replays in the next, under
// another commit with the same tree too; a set with a gate not run is not
// journaled, so the next launch runs it again.
func TestGatesJournalReplay(t *testing.T) {
	r := newGitRepo(t)
	c1 := r.head()
	r.git("commit", "-q", "--allow-empty", "-m", "same tree")
	c2 := r.head()
	state, counter := t.TempDir(), filepath.Join(t.TempDir(), "count")
	decl := mustDecl(t, "gates:\n  - name: count\n    command: \"echo x >> "+counter+"\"\n  - name: flag\n    command: \"test -e "+counter+".ok || sleep 5\"\n    timeout: 300ms\n")
	runs := func() int {
		b, _ := os.ReadFile(counter)
		return strings.Count(string(b), "x")
	}
	gate := func(commit string) gateSet {
		var set gateSet
		realGateLaunch(t, r, state, func(o *orch) {
			o.decl = decl
			var fault string
			var err error
			set, fault, err = o.gates(context.Background(), commit)
			if fault != "" || err != nil {
				t.Fatalf("%q %v", fault, err)
			}
		})
		return set
	}
	if set := gate(c1); set.Runs[1].Status != lsfrt.GateNotRun || runs() != 1 {
		t.Fatalf("first: %+v, %d runs", set, runs())
	}
	if set := gate(c1); set.Runs[1].Status != lsfrt.GateNotRun || runs() != 2 {
		t.Fatalf("not-run journaled: %+v, %d runs", set, runs())
	}
	os.WriteFile(counter+".ok", nil, 0o644)
	if set := gate(c1); set.Runs[1].Status != lsfrt.GateGreen || runs() != 3 {
		t.Fatalf("third: %+v, %d runs", set, runs())
	}
	if set := gate(c2); set.Runs[1].Status != lsfrt.GateGreen || runs() != 3 || set.Tree != r.git("rev-parse", c1+"^{tree}") {
		t.Fatalf("same tree, other commit: %+v, %d runs", set, runs())
	}
}

// The declaration whose commands match the stub's GreenGates.
const stubGates = "gates:\n  - name: tests\n    command: pytest -q\n  - name: lint\n    command: ruff check .\n"

// statusAt answers RunGates with status(commit, gate name).
func statusAt(status func(commit, gate string) string) func(orchtest.GateCall) ([]lsfrt.GateRun, error) {
	return func(g orchtest.GateCall) ([]lsfrt.GateRun, error) {
		return orchtest.GateRuns(g.Gates, func(s lsfrt.GateSpec) string { return status(g.Commit, s.Name) }, g.Commit), nil
	}
}

func declaredStub(t *testing.T, p *plan.Plan, h Hs) *orchtest.Stub {
	t.Helper()
	s := stub(t, p, h, nil)
	s.State.GatesBlock = stubGates
	return s
}

// S3 (unit part), S4: the runtime gates the implement commit; a stage's
// green for a gate the runtime found red is a gate-misreported record, and
// the reviewers read the runtime's red, with its tail.
func TestDeclaredGatesSupersedeStage(t *testing.T) {
	s := declaredStub(t, graph("plan0", unit("A")), nil)
	s.Gates = statusAt(func(_, gate string) string {
		if gate == "tests" {
			return lsfrt.GateRed
		}
		return lsfrt.GateGreen
	})
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	eq(t, len(s.GateRuns), 1)
	eq(t, s.GateRuns[0].Commit, "i")
	eq(t, out.Digest.ByKind["gate-misreported"], 1)
	for _, l := range []string{"critique-code:r1:A", "lens-review:r1:A"} {
		match(t, s.Prompt(l), "The runtime, on a clean checkout, ran the project's gates at this branch's HEAD:\n- `pytest -q` — red: red at i\n\n  ```\n  FAILED tests\n  ```\n- `ruff check \\.` — green: green at i\n", l)
	}
	impl := s.Prompt("implement:A")
	contains(t, impl, "The runtime runs the project's declared gates on a clean checkout of the commit you return")
	contains(t, impl, "- tests: `pytest -q`\n- lint: `ruff check .`\nDo not run them as a final check")
	notContains(t, impl, "Run each gate detached", "no long-gate recipe")
	text := input(t, s, "completion-report:A")["text"].(string)
	match(t, text, "Gates at HEAD:\n\n- `pytest -q` — red: red at i\n- ")
	notContains(t, text, "FAILED tests", "no tail in the committed report")
}

// S4: a stage that omits a declared gate, reports it not-run, or reports
// none at all is not stopped; a runtime not-run is gate-not-run.
func TestDeclaredGatesStageOmissions(t *testing.T) {
	for name, gates := range map[string][]orchtest.Gate{
		"none":    {},
		"omitted": {g("pytest -q", "green", "ok")},
		"not-run": {g("pytest -q", "not-run", "timeout"), g("ruff check .", "green", "")},
	} {
		t.Run(name, func(t *testing.T) {
			s := declaredStub(t, graph("plan0", unit("A")), Hs{"implement": func(c Call, _ *State) any {
				return R(orchtest.Implemented(c.Unit, M{"gates": gates}))
			}})
			out := run(t, s)
			eq(t, out.Status, "complete", out.stopKinds())
			eq(t, out.Digest.ByKind["gate-omitted"]+out.Digest.ByKind["gate-not-run"], 0)
		})
	}
	s := declaredStub(t, graph("plan0", unit("A")), nil)
	s.Gates = statusAt(func(_, gate string) string {
		if gate == "lint" {
			return lsfrt.GateNotRun
		}
		return lsfrt.GateGreen
	})
	out := run(t, s)
	eq(t, strings.Join(out.stopKinds(), ","), "gate-not-run")
	st := out.stops()[0]
	eq(t, st.Stage, "implement")
	contains(t, st.Statement, "not run by the runtime on i: ruff check . (not-run at i)")
	if has(s.Labels(), "critique-code:r1:A") {
		t.Error("a runtime not-run reaches review")
	}
}

// A code fix's commit is gated, and the next round reads those results; a
// fix whose gates the runtime could not run stops.
func TestDeclaredGatesAfterFix(t *testing.T) {
	s := declaredStub(t, graph("plan0", unit("A")), Hs{"critique-code": criticalOnce(), "fix-code": fixCode(g("pytest -q", "red", "lies"))})
	s.Gates = statusAt(func(commit, _ string) string {
		if commit == "i" {
			return lsfrt.GateRed
		}
		return lsfrt.GateGreen
	})
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	eq(t, len(s.GateRuns), 2)
	eq(t, s.GateRuns[1].Commit, "f")
	match(t, s.Prompt("critique-code:r2:A"), "The runtime, on a clean checkout, ran the project's gates at this branch's HEAD:\n- `pytest -q` — green: green at f\n")
	eq(t, out.Digest.ByKind["gate-misreported"], 3, "implement's two greens vs red, the fix's red vs green")
	fix := s.Prompt("fix-code:r1:A")
	contains(t, fix, "The runtime runs the project's declared gates")
	notContains(t, fix, "every gate the implement stage ran")
	text := input(t, s, "completion-report:A")["text"].(string)
	match(t, text, "the gates below are the runtime's at the last fix")
	match(t, text, "Gates at HEAD:\n\n- `pytest -q` — green: green at f")

	s = declaredStub(t, graph("plan0", unit("A")), Hs{"critique-code": criticalOnce(), "fix-code": fixCode()})
	s.Gates = statusAt(func(commit, _ string) string {
		if commit == "f" {
			return lsfrt.GateNotRun
		}
		return lsfrt.GateGreen
	})
	out = run(t, s)
	eq(t, strings.Join(out.stopKinds(), ","), "gate-not-run")
	eq(t, out.stops()[0].Stage, "fix")
}

// A design unit runs no unit-level gates, and its prompts are unchanged.
func TestDeclaredGatesDesignUnit(t *testing.T) {
	d := orchtest.Typed(unit("D"), "design")
	s := declaredStub(t, graph("plan0", d), nil)
	run(t, s)
	eq(t, len(s.GateRuns), 0)
	plain := stub(t, graph("plan0", d), nil, nil)
	run(t, plain)
	eq(t, s.Prompt("implement:D"), plain.Prompt("implement:D"))
}

// The wave hands the merge its declared mode and each unit's gated tree;
// an undeclared merge input is as before.
func TestDeclaredMergeInput(t *testing.T) {
	s := declaredStub(t, graph("plan0", unit("A")), nil)
	run(t, s)
	in := input(t, s, "merge:w1")
	eq(t, in["declared"], any(true))
	eq(t, in["units"].([]any)[0].(map[string]any)["gatedTree"], any("i"))
	plain := stub(t, graph("plan0", unit("A")), nil, nil)
	run(t, plain)
	in = input(t, plain, "merge:w1")
	if _, ok := in["declared"]; ok {
		t.Error("undeclared input has declared")
	}
	if _, ok := in["units"].([]any)[0].(map[string]any)["gatedTree"]; ok {
		t.Error("undeclared input has gatedTree")
	}
}

// A declared plan's re-plan is told the gate block is pinned; an
// undeclared one's prompt is unchanged (promptparity).
func TestDeclaredReplanPrompt(t *testing.T) {
	s := declaredStub(t, orchtest.TwoWaves(), discovering())
	run(t, s)
	contains(t, s.Prompt("replan:w1"), "Do not edit the plan's ```yaml lsf-gates block; a change to it stops the run.\n")
	plain := stub(t, orchtest.TwoWaves(), discovering(), nil)
	run(t, plain)
	notContains(t, plain.Prompt("replan:w1"), "lsf-gates")
}
