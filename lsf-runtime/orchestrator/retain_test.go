package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// retaining is TwoWaves with A retaining out/data and B retaining res.
func retaining() *plan.Plan {
	p := orchtest.TwoWaves()
	p.Units[0].Retains = []string{"out/data"}
	p.Units[1].Retains = []string{"res"}
	return p
}

// withRetains replaces unit id's retains in the stub's plan, for a stage
// that rewrites the plan.
func withRetains(st *State, id string, retains []string) {
	np := *st.Plan
	np.Units = nil
	for _, u := range st.Plan.Units {
		cu := *u
		if cu.ID == id {
			cu.Retains = retains
		}
		np.Units = append(np.Units, &cu)
	}
	st.Plan = &np
}

// D33: the cleanup step's input carries each merged unit's retains and the
// pinned store; without them its JSON is as before.
func TestRetainsCleanupInput(t *testing.T) {
	s := stub(t, retaining(), nil, nil)
	s.State.GatesBlock = "store: /srv/lsf-store\n"
	v := run(t, s)
	eq(t, v.Status, "complete", v.stopKinds())
	in := input(t, s, "cleanup:w1")
	eq(t, in["store"], any("/srv/lsf-store"))
	u := in["units"].([]any)[0].(map[string]any)
	eq(t, u["unit"], any("A"))
	eq(t, len(u["retains"].([]any)), 1)
	eq(t, u["retains"].([]any)[0], any("out/data"))

	plain := stub(t, orchtest.TwoWaves(), nil, nil)
	run(t, plain)
	raw := string(plain.Input("cleanup:w1"))
	if strings.Contains(raw, "retains") || strings.Contains(raw, "store") {
		t.Errorf("an undeclared plan's cleanup input changed: %s", raw)
	}
}

// D32: retains need the pinned store, at the first load and at a re-plan's.
func TestRetainsNeedStore(t *testing.T) {
	s := stub(t, retaining(), nil, nil)
	v := run(t, s)
	eq(t, strings.Join(v.stopKinds(), ","), "invalid-plan,invalid-plan")
	contains(t, v.stops()[0].Statement, "A: retains needs a `store`")
	eq(t, v.Escalation.Stage, "load")

	replan := orchtest.Defaults()["replan"]
	s = stub(t, orchtest.TwoWaves(), merged(discovering(), Hs{"replan": func(c Call, st *State) any {
		out := replan(c, st)
		withRetains(st, "B", []string{"res"})
		return out
	}}), nil)
	s.State.GatesBlock = "hosts: [proxy.golang.org]\n"
	v = run(t, s)
	eq(t, strings.Join(v.stopKinds(), ","), "invalid-plan")
	contains(t, v.stops()[0].Statement, "B: retains needs a `store`")
	eq(t, v.Escalation.Stage, "replan")
}

// D32: the store is pinned with the block; a re-plan that changes it stops.
func TestStoreChangeStops(t *testing.T) {
	replan := orchtest.Defaults()["replan"]
	s := stub(t, retaining(), merged(discovering(), Hs{"replan": func(c Call, st *State) any {
		st.GatesBlock = "store: /srv/other\n"
		return replan(c, st)
	}}), nil)
	s.State.GatesBlock = "store: /srv/lsf-store\n"
	v := run(t, s)
	eq(t, strings.Join(v.stopKinds(), ","), "declared-gates-changed")
	contains(t, v.stops()[0].Statement, `"store":"/srv/other"`)
}

// Requester decision 5: a load that drops a path from an incomplete unit's
// retains stops; one that adds a path does not, and the addition is pinned
// in turn.
func TestRetainsChanged(t *testing.T) {
	replan := orchtest.Defaults()["replan"]
	for _, c := range []struct {
		name string
		now  []string
		want string
	}{
		{"dropped", []string{}, "retains-changed"},
		{"replaced", []string{"other"}, "retains-changed"},
		{"added", []string{"res", "more"}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := stub(t, retaining(), merged(discovering(), Hs{"replan": func(cl Call, st *State) any {
				out := replan(cl, st)
				withRetains(st, "B", c.now)
				// The completed unit's retains are not compared.
				withRetains(st, "A", nil)
				return out
			}}), nil)
			s.State.GatesBlock = "store: /srv/lsf-store\n"
			v := run(t, s)
			if c.want == "" {
				eq(t, v.Status, "complete", v.stopKinds())
				eqs(t, input(t, s, "cleanup:w2")["units"].([]any)[0].(map[string]any)["retains"].([]any), []any{"res", "more"})
				return
			}
			eq(t, strings.Join(v.stopKinds(), ","), c.want)
			st := v.stops()[0]
			eq(t, st.Unit, "B")
			eq(t, st.Stage, "load")
			contains(t, st.Statement, "was [res], now ["+strings.Join(c.now, ", ")+"]")
		})
	}

	// A decision re-plan is compared with the plan at args.planSha.
	dr := orchtest.Defaults()["decision-replan"]
	s := stub(t, retaining(), Hs{
		"load": func(_ Call, st *State) any {
			return orchtest.Copied(st.Plan, M{"decisionsBlock": decisionsBlock("2026-10-09", "yes"), "planBlock": orchtest.ToBlock(st.Plan) + orchtest.GatesBlock(st.GatesBlock)})
		},
		"decision-replan": func(cl Call, st *State) any {
			withRetains(st, "A", nil)
			return dr(cl, st)
		},
	}, M{"decisions": []M{{"id": "d1", "question": "q", "decision": "yes"}}})
	s.State.GatesBlock = "store: /srv/lsf-store\n"
	v := run(t, s)
	eq(t, strings.Join(v.stopKinds(), ","), "retains-changed", v.stops())
	eq(t, v.Escalation.Stage, "load")
}

// storeRT fails the store check.
type storeRT struct {
	*orchtest.Stub
	err error
}

func (r storeRT) CheckStore(store string) error { return r.err }

// D32: the pin checks the store; a failure is an invalid-plan load stop
// before any agent call.
func TestPinChecksStore(t *testing.T) {
	s := stub(t, retaining(), nil, nil)
	s.State.GatesBlock = "store: /srv/lsf-store\n"
	out, err := Run(context.Background(), storeRT{s, errors.New("store /srv/lsf-store: no such file or directory")})
	if err != nil {
		t.Fatal(err)
	}
	var v view
	json.Unmarshal(out, &v)
	eq(t, strings.Join(v.stopKinds(), ","), "invalid-plan")
	eq(t, v.stops()[0].Referent, "store")
	contains(t, v.stops()[0].Statement, "gates block: store /srv/lsf-store: no such file")
	eq(t, len(agentCalls(s)), 0)
	eq(t, len(s.Pins), 0)
}

func TestWriterSources(t *testing.T) {
	got := writerSources([]string{
		"completion-report:A", "critique-code:r1:A", "fix-code:r10:A", "fix-code:r2:A", "fix-plan:r1:A", "implement:A",
		"implement:A#2", "lens-review:r1:A", "plan:A", "plan:A#2", "fix-code:r2:A#2", "replan:w1",
	})
	eqs(t, got, []string{"plan:A", "plan:A#2", "fix-plan:r1:A", "implement:A", "implement:A#2", "fix-code:r2:A", "fix-code:r2:A#2", "fix-code:r10:A"})
}

// retainRT is removals with MoveRetained answering from a table. With
// stepped, cleanup steps run their fn on the stub (which has no
// repository); every other step is the stub's.
type retainRT struct {
	*removals
	moves   map[string]lsfrt.RetainedMove
	moveErr map[string]error
	order   []string
}

func (r *retainRT) MoveRetained(ctx context.Context, unit, store, integration string, paths, sources []string) (lsfrt.RetainedMove, error) {
	r.order = append(r.order, "move "+unit+" @"+store+" "+strings.Join(paths, ",")+" from "+strings.Join(sources, ","))
	if err := r.moveErr[unit]; err != nil {
		return lsfrt.RetainedMove{}, err
	}
	return r.moves[unit], nil
}

func (r *retainRT) RemoveWorktree(ctx context.Context, label, integration string, deleteBranch bool) (lsfrt.WorktreeRemoval, error) {
	r.order = append(r.order, "remove "+label)
	return r.removals.RemoveWorktree(ctx, label, integration, deleteBranch)
}

func newRetainRT(t *testing.T, p *plan.Plan) *retainRT {
	r := newGitRepo(t)
	return &retainRT{removals: &removals{gitRT: gitRT{orchtest.New(t, p, nil, nil), r.dir, nil, t.TempDir()}, answer: map[string]lsfrt.WorktreeRemoval{}}}
}

// steppedRT is retainRT over the bare stub, running cleanup steps' fn.
type steppedRT struct {
	*orchtest.Stub
	r *retainRT
}

func (s steppedRT) MoveRetained(ctx context.Context, unit, store, integration string, paths, sources []string) (lsfrt.RetainedMove, error) {
	return s.r.MoveRetained(ctx, unit, store, integration, paths, sources)
}

func (s steppedRT) RemoveWorktree(ctx context.Context, label, integration string, deleteBranch bool) (lsfrt.WorktreeRemoval, error) {
	return s.r.RemoveWorktree(ctx, label, integration, deleteBranch)
}

func (s steppedRT) Step(ctx context.Context, label string, input []byte, fn func(context.Context, bool) (json.RawMessage, error)) (json.RawMessage, error) {
	if strings.HasPrefix(label, "cleanup:") {
		return fn(ctx, false)
	}
	return s.Stub.Step(ctx, label, input, fn)
}

// D33: a unit's retains move before its clones go, from its writer clones
// oldest first; a failed move keeps every label of that unit, not
// another's, is the step's failure, and is noted; a stop stops.
func TestCleanupMovesRetained(t *testing.T) {
	rt := newRetainRT(t, orchtest.TwoWaves())
	rt.answer = map[string]lsfrt.WorktreeRemoval{"implement:A": {Removed: true, Paths: []string{"keep.log"}}, "plan:A": {Removed: true}, "implement:B": {Removed: true}}
	rt.moves = map[string]lsfrt.RetainedMove{"A": {Unit: "A", Dest: "/srv/s/A", Moved: []lsfrt.RetainedPath{{Path: "out", From: "implement:A", Method: "rename"}}, Missing: []string{"gone"}}}
	o := newOrch(rt, runCtxFor(t, nil), nil)
	in := cleanupInput{Wave: 1, IntegrationBranch: "integration", Store: "/srv/s", Units: []cleanupUnitIn{
		{Unit: "A", Worktrees: []string{"critique-code:r1:A", "implement:A", "plan:A"}, Retains: []string{"out", "gone"}},
		{Unit: "B", Worktrees: []string{"implement:B"}},
	}}
	var failed []retainFailure
	raw, err := o.cleanupStage(in, &failed)(context.Background(), "cleanup:w1", false)
	if err != nil {
		t.Fatal(err)
	}
	eqs(t, rt.order, []string{"move A @/srv/s out,gone from plan:A,implement:A", "remove critique-code:r1:A", "remove implement:A", "remove plan:A", "remove implement:B"})
	got := decode[cleanupResult](rt, "cleanup:w1", raw)
	eq(t, len(got.Result.Retained), 1)
	eq(t, got.Result.Retained[0].Dest, "/srv/s/A")
	eq(t, int(got.Result.Removed), 3)
	eq(t, len(failed), 0)
	if !hasPrefix(rt.Stub.Logs, "cleanup:w1: A: retained out → /srv/s/A; missing gone") {
		t.Errorf("logs %q", rt.Stub.Logs)
	}

	rt.order, rt.moveErr = nil, map[string]error{"A": errors.New("A: destination /srv/s/A exists")}
	_, err = o.cleanupStage(in, &failed)(context.Background(), "cleanup:w1", false)
	errMatch(t, err, `retained move: A: destination /srv/s/A exists`)
	eqs(t, rt.order, []string{"move A @/srv/s out,gone from plan:A,implement:A", "remove implement:B"})
	eq(t, len(failed), 1)
	eq(t, failed[0].unit, "A")
	eqs(t, failed[0].labels, []string{"critique-code:r1:A", "implement:A", "plan:A"})
	// A rerun of the step moves again.
	rt.order = nil
	o.cleanupStage(in, &failed)(context.Background(), "cleanup:w1", false)
	eq(t, rt.order[0], "move A @/srv/s out,gone from plan:A,implement:A")

	rt.order, rt.moveErr = nil, map[string]error{"A": &lsfrt.StopError{Reason: lsfrt.StopCancelled}}
	_, err = o.cleanupStage(in, new([]retainFailure))(context.Background(), "cleanup:w1", false)
	if _, ok := lsfrt.IsStop(err); !ok {
		t.Errorf("a stop is not the step's failure: %v", err)
	}
	eq(t, len(rt.order), 1, "no later unit runs after a stop")
}

// D33: a failed move is a retained-move-failed record naming the clones
// and the remedy; the run goes on.
func TestRetainedMoveFailedRecord(t *testing.T) {
	r := newRetainRT(t, retaining())
	r.moveErr = map[string]error{"A": errors.New("A: destination /srv/lsf-store/A exists")}
	r.moves = map[string]lsfrt.RetainedMove{"B": {Unit: "B", Moved: []lsfrt.RetainedPath{}, Missing: []string{"res"}}}
	s := stub(t, retaining(), nil, nil)
	s.State.GatesBlock = "store: /srv/lsf-store\n"
	rt := steppedRT{s, r}
	out, err := Run(context.Background(), rt)
	if err != nil {
		t.Fatal(err)
	}
	var v view
	json.Unmarshal(out, &v)
	eq(t, v.Status, "complete", v.stops())
	eq(t, v.Digest.ByKind["retained-move-failed"], 1)
	var rec *plan.Signal
	for _, l := range filterPrefix(s.Labels(), "ledger") {
		if strings.Contains(ledgerText(t, s, l), "retained-move-failed") {
			rec = &plan.Signal{Statement: ledgerText(t, s, l)}
		}
	}
	if rec == nil {
		t.Fatal("no retained-move-failed row in the ledger")
	}
	contains(t, rec.Statement, "destination /srv/lsf-store/A exists")
	contains(t, rec.Statement, "<state>/clone/implement:A")
	contains(t, rec.Statement, "By hand: put each declared path at /srv/lsf-store/A/")
	if !hasPrefix(s.Logs, "wave 1: worktree cleanup failed:") {
		t.Errorf("logs %q", s.Logs)
	}
	if hasPrefix(r.order, "remove implement:A") || !hasPrefix(r.order, "remove implement:B") {
		t.Error("a failed move's clones were removed")
	}
}
