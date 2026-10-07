package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// The cleanup step's input names the merged units' worktree labels from
// this launch, sorted, the completion report's among them; a unit that did
// not merge is not there.
func TestCleanupInputIsMergedUnitsWorktrees(t *testing.T) {
	s := stub(t, orchtest.TwoParallel(), merged(discovering(), Hs{"merge": unresolvableA(plan.Merge{Unit: "B", Status: "clean", Files: []string{}, Sha: "mb"})}), nil)
	run(t, s)
	units := input(t, s, "cleanup:w1")["units"].([]any)
	if len(units) != 1 {
		t.Fatalf("units %v", units)
	}
	u := units[0].(map[string]any)
	eq(t, u["unit"], any("B"))
	got := []string{}
	for _, w := range u["worktrees"].([]any) {
		got = append(got, w.(string))
	}
	eqs(t, got, []string{"completion-report:B", "critique-code:r1:B", "implement:B", "lens-review:r1:B", "plan:B"})
}

// removals is a stub runtime whose RemoveWorktree answers from a table and
// records its calls.
type removals struct {
	gitRT
	answer map[string]lsfrt.WorktreeRemoval
	errs   map[string]error
	calls  []string
}

func (r *removals) RemoveWorktree(ctx context.Context, label, integration string, deleteBranch bool) (lsfrt.WorktreeRemoval, error) {
	r.calls = append(r.calls, label+map[bool]string{true: " +branch", false: ""}[deleteBranch]+" @"+integration)
	if err := r.errs[label]; err != nil {
		return lsfrt.WorktreeRemoval{Label: label}, err
	}
	return r.answer[label], nil
}

// The cleanup step removes each label's worktree, the completion report's
// branch with it; reports what went, what stayed and why, and the paths;
// a failed removal does not stop the others and is the step's failure.
func TestCleanupStage(t *testing.T) {
	r := newGitRepo(t)
	rt := &removals{gitRT: gitRT{orchtest.New(t, orchtest.TwoWaves(), nil, nil), r.dir, nil, t.TempDir()},
		answer: map[string]lsfrt.WorktreeRemoval{
			"plan:A":              {Removed: true},
			"implement:A":         {Removed: true, Paths: []string{"build.log", "out/"}},
			"completion-report:A": {Removed: true},
			"critique-code:r1:A":  {Kept: "absent"},
			"implement:B":         {Kept: "not merged into integration"},
		}}
	o := newOrch(rt, runCtxFor(t, nil), nil)
	in := cleanupInput{Wave: 1, IntegrationBranch: "integration", Units: []cleanupUnitIn{
		{Unit: "A", Worktrees: []string{"completion-report:A", "critique-code:r1:A", "implement:A", "plan:A"}},
		{Unit: "B", Worktrees: []string{"implement:B"}},
	}}
	raw, err := o.cleanupStage(in)(context.Background(), "cleanup:w1", false)
	if err != nil {
		t.Fatal(err)
	}
	eqs(t, rt.calls, []string{"completion-report:A +branch @integration", "critique-code:r1:A @integration", "implement:A @integration", "plan:A @integration", "implement:B @integration"})
	got := decode[cleanupResult](rt, "cleanup:w1", raw)
	eq(t, int(got.Result.Removed), 3)
	eqs(t, got.Result.Kept, []string{"implement:B: not merged into integration"})
	if len(got.Result.Paths) != 1 || got.Result.Paths[0].Label != "implement:A" || strings.Join(got.Result.Paths[0].Paths, ",") != "build.log,out/" {
		t.Errorf("paths %+v", got.Result.Paths)
	}

	rt.calls = nil
	rt.errs = map[string]error{"critique-code:r1:A": errors.New("worktree remove: busy")}
	_, err = o.cleanupStage(in)(context.Background(), "cleanup:w1", false)
	errMatch(t, err, `busy`)
	eq(t, len(rt.calls), 5, "the others still removed")

	rt.errs = map[string]error{"plan:A": &lsfrt.StopError{Reason: lsfrt.StopSigint}}
	_, err = o.cleanupStage(in)(context.Background(), "cleanup:w1", false)
	if _, ok := lsfrt.IsStop(err); !ok {
		t.Errorf("a stop is not the step's failure: %v", err)
	}
}
