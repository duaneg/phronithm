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

// branchRT is gitRT whose isolated calls carry the worktree branch the
// runtime would give them: lsf/t/<prefix> for the label's prefix.
type branchRT struct{ gitRT }

func (r branchRT) Call(ctx context.Context, prompt string, o lsfrt.Opts) (lsfrt.CallResult, error) {
	cr, err := r.gitRT.Call(ctx, prompt, o)
	if o.Isolation == "worktree" {
		cr.Branch = rtBranchOf(o.Label)
	}
	return cr, err
}

func rtBranchOf(label string) string {
	prefix, _, _ := strings.Cut(label, ":")
	return "lsf/t/" + prefix
}

// branchOrch is an orch whose runtime gives branches and runs steps on r.
func branchOrch(t *testing.T, r *gitRepo, h map[string]orchtest.Handler) *orch {
	t.Helper()
	s := orchtest.New(t, orchtest.TwoWaves(), h, nil)
	o := newOrch(branchRT{gitRT{s, r.dir, nil, t.TempDir()}}, runCtxFor(t, M{"planPath": "docs/plans/f-breakdown.md"}), nil)
	o.today = func() string { return "2026-10-07" }
	return o
}

func runA(t *testing.T, o *orch) *unitResult {
	t.Helper()
	p := orchtest.TwoWaves()
	res, err := o.runUnit(context.Background(), p, p.Units[0], 1)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func kinds(sigs []*plan.Signal) []string {
	out := []string{}
	for _, s := range sigs {
		out = append(out, s.Kind+"@"+s.Stage)
	}
	return out
}

func wantSignal(t *testing.T, res *unitResult, kind, stage, stmt string) {
	t.Helper()
	for _, s := range res.signals {
		if s.Kind == kind && s.Stage == stage && strings.Contains(s.Statement, stmt) {
			return
		}
	}
	t.Errorf("no %s at %s containing %q in %v", kind, stage, stmt, kinds(res.signals))
}

// D23: a worktree writer that reports a branch other than the runtime's
// stops at its own return, at plan, implement and a fix; the fix's branch
// is never taken. A stage reporting the runtime's branch passes, its SHA
// checked against that branch.
func TestBranchMismatchStops(t *testing.T) {
	r := newGitRepo(t)
	for _, b := range []string{"plan", "implement", "fix-plan", "fix-code", "completion-report"} {
		r.git("branch", "lsf/t/"+b)
	}
	tip := r.git("rev-parse", "integration")
	honest := func(stage string) orchtest.Handler {
		return func(c orchtest.Call, s *orchtest.State) any {
			v := orchtest.Defaults()[stage](c, s).(M)
			v["result"].(M)["branch"] = rtBranchOf(c.Label)
			v["result"].(M)["sha"] = tip[:7]
			return v
		}
	}

	res := runA(t, branchOrch(t, r, nil))
	if res.aborted != "plan" {
		t.Fatalf("plan: aborted at %q: %v", res.aborted, kinds(res.signals))
	}
	wantSignal(t, res, "branch-mismatch", "plan", `reported branch "plan-A"; the call's worktree branch is lsf/t/plan`)

	res = runA(t, branchOrch(t, r, map[string]orchtest.Handler{"plan": honest("plan")}))
	if res.aborted != "implement" {
		t.Fatalf("implement: aborted at %q: %v", res.aborted, kinds(res.signals))
	}
	wantSignal(t, res, "branch-mismatch", "implement", `"impl-A"`)

	var fixedFrom string
	res = runA(t, branchOrch(t, r, map[string]orchtest.Handler{
		"plan":          honest("plan"),
		"critique-plan": func(orchtest.Call, *orchtest.State) any { return critical1("plan.md:1") },
		"implement": func(c orchtest.Call, s *orchtest.State) any {
			fixedFrom = c.Prompt
			return nil
		},
	}))
	if res.aborted != "critique-plan" || fixedFrom != "" {
		t.Fatalf("fix: aborted at %q, implement ran: %v; %v", res.aborted, fixedFrom != "", kinds(res.signals))
	}
	wantSignal(t, res, "branch-mismatch", "fix", `"fixed-A"`)

	res = runA(t, branchOrch(t, r, map[string]orchtest.Handler{"plan": honest("plan"), "implement": honest("implement")}))
	if res.aborted != "" {
		t.Fatalf("honest: aborted at %q: %v", res.aborted, kinds(res.signals))
	}
	for _, s := range res.signals {
		if s.Kind == "branch-mismatch" || s.Kind == "sha-corrected" {
			t.Errorf("honest run raised %s: %s", s.Kind, s.Statement)
		}
	}
}

// failRevParse fails the completion report's tip read with a git error that
// is not exit 1.
type failRevParse struct{ inner gitRunner }

func (g failRevParse) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if len(args) > 0 && args[0] == "rev-parse" && strings.HasSuffix(args[len(args)-1], "^{commit}") && strings.Contains(args[len(args)-1], "impl-") {
		return nil, &gitError{args: args, err: errors.New("bad object store")}
	}
	return g.inner.run(ctx, dir, args...)
}

// The completion report's tip read separates an unresolved unit branch
// (exit 1) from another git error, as checkSha does.
func TestCompletionTipReadErrors(t *testing.T) {
	r := newGitRepo(t)
	stub := func(g gitRunner) *orch {
		o := loadLaunch(t, r, g, nil).orch
		o.today = func() string { return "2026-10-07" }
		return o
	}
	res := runA(t, stub(nil))
	if res.aborted != "completion-report" {
		t.Fatalf("aborted at %q: %v", res.aborted, kinds(res.signals))
	}
	wantSignal(t, res, "stage-fault", "completion-report", `unit branch "impl-A" does not resolve`)

	res = runA(t, stub(failRevParse{execGit{}}))
	if res.aborted != "completion-report" {
		t.Fatalf("aborted at %q: %v", res.aborted, kinds(res.signals))
	}
	wantSignal(t, res, "stage-fault", "completion-report", `unit branch "impl-A": git rev-parse`)
	for _, s := range res.signals {
		if strings.Contains(s.Statement, "does not resolve") {
			t.Errorf("a git error reported as unresolved: %s", s.Statement)
		}
	}
}
