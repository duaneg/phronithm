package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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

// The same tree is gated once in a launch, concurrent callers included.
func TestGatesMemoPerTree(t *testing.T) {
	s := stub(t, orchtest.TwoWaves(), nil, nil)
	s.Concurrent = true
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
	wg.Wait()
	o.gates(context.Background(), "c2")
	o.gates(context.Background(), "c1")
	eq(t, len(s.GateRuns), 2)
	eq(t, strings.Join(s.Labels(), ","), "gates:c1,gates:c2")
	eq(t, s.GateRuns[0].Gates[0].Command, "make test")
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
	cfg := lsfrt.Config{StateDir: state, Repo: r.dir, Roles: map[string]lsfrt.Role{"w": {Tools: "Read", PermissionMode: "dontAsk"}}, DefaultRole: "w"}
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
