package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// ffCase runs one writer repo call (direct-edits) on the real runtime over
// a real repository with the fake claude answering a, under role clone
// kind; @main in a's shell stands for the main checkout. It returns the
// call's value, its signals, the repo and the state dir.
func ffCase(t *testing.T, kind string, a answer) (*ret[scribeResult], []*plan.Signal, *gitRepo, string) {
	t.Helper()
	f := newFFLaunch(t, kind, a)
	res, sigs := f.launch()
	return res, sigs, f.r, f.cfg.StateDir
}

// ffLaunch is ffCase's launch, repeatable over the same state dir.
type ffLaunch struct {
	t            *testing.T
	r            *gitRepo
	cfg          lsfrt.Config
	label, stage string // the call's; direct-edits when empty
}

func newFFLaunch(t *testing.T, kind string, a answer) *ffLaunch {
	t.Helper()
	r := newGitRepo(t)
	a.Contains = "Make these edits"
	a.Shell = strings.ReplaceAll(a.Shell, "@main", r.dir)
	dir := t.TempDir()
	tablePath := filepath.Join(dir, "table.json")
	b, err := plan.MarshalJS([]answer{a}, "")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(tablePath, b, 0o644)
	t.Setenv("LSF_FAKE_CLAUDE_TABLE", tablePath)
	t.Setenv("LSF_FAKE_CLAUDE_STATE", "")
	t.Setenv("LSF_CLAUDE_BIN", "")
	args, _ := plan.MarshalJS(M{"name": "f", "planSha": r.head(), "architectureDoc": "docs/architecture/f.md", "integrationBranch": "integration"}, "")
	return &ffLaunch{t, r, lsfrt.Config{
		StateDir: filepath.Join(dir, "state"), Repo: r.dir, ClaudeBin: os.Args[0], Args: args,
		Roles: map[string]lsfrt.Role{"w": {Tools: "default", PermissionMode: "dontAsk", Clone: kind}}, DefaultRole: "w",
		Backoff: []lsfrt.Duration{lsfrt.Duration(10 * time.Millisecond)},
	}, "", ""}
}

func (f *ffLaunch) launch() (*ret[scribeResult], []*plan.Signal) {
	t := f.t
	t.Helper()
	var res *ret[scribeResult]
	var sigs []*plan.Signal
	sum, err := lsfrt.Run(context.Background(), f.cfg, func(ctx context.Context, rt *lsfrt.Runtime) (json.RawMessage, error) {
		a, err := parseArgs(rt.Args())
		if err != nil {
			return nil, err
		}
		c, err := newRunCtx(a)
		if err != nil {
			return nil, err
		}
		o := newOrch(rt, c, nil)
		label, stage := "direct-edits", "direct-edits"
		if f.label != "" {
			label, stage = f.label, f.stage
		}
		res, sigs, err = call[scribeResult](ctx, o, site{at: plan.At{Stage: stage}, writer: true}, "Make these edits", lsfrt.Opts{Label: label, Schema: scribeSchema})
		return nil, err
	})
	if err != nil || sum.StopReason != lsfrt.StopReturned {
		t.Fatalf("%v %+v", err, sum)
	}
	return res, sigs
}

// theClone is the call's clone directory.
func (f *ffLaunch) theClone() *gitRepo {
	f.t.Helper()
	m, _ := filepath.Glob(filepath.Join(f.cfg.StateDir, "clone", lsfrtKey("direct-edits")+"*"))
	if len(m) != 1 {
		f.t.Fatalf("clones %v", m)
	}
	return &gitRepo{f.t, m[0]}
}

const commitEdit = "echo edit > e.txt && git add e.txt && git commit -q -m edit"

var scribeHead = R(M{"path": "docs/plans/f-breakdown.md", "sha": "@head"})

// lsfrtKey is the runtime's journal key for label, found by its slug.
func lsfrtKey(label string) string {
	return strings.NewReplacer(":", "_", "#", "_").Replace(label) + "-"
}

func cloneExists(t *testing.T, state, label string) bool {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(state, "clone", lsfrtKey(label)+"*"))
	return len(m) > 0
}

// A writer repo call's commit reaches the integration branch by the ff:
// step after it returns, and its clone and branch then go.
func TestFastForwardWriterRepoCall(t *testing.T) {
	res, sigs, r, state := ffCase(t, "kept", answer{Return: scribeHead, Shell: commitEdit})
	if res == nil || len(sigs) != 0 {
		t.Fatalf("%+v %+v", res, sigs)
	}
	eq(t, r.head(), res.Result.Sha, "integration fast-forwarded to the call's commit")
	eq(t, r.git("log", "-1", "--format=%s"), "edit")
	if !journaled(t, state, "ff_direct-edits") {
		t.Error("no ff: step record")
	}
	if cloneExists(t, state, "direct-edits") || r.git("branch", "--list", "lsf/*") != "" {
		t.Error("the clone or its branch is left")
	}
	eq(t, r.git("status", "--porcelain"), "")
}

// A null writer is not fast-forwarded: its commit stays on its fetched
// branch and in its clone.
func TestFastForwardNotOnNull(t *testing.T) {
	res, _, r, state := ffCase(t, "kept", answer{Return: nil, Shell: commitEdit})
	if res != nil {
		t.Fatalf("%+v", res)
	}
	eq(t, r.git("log", "-1", "--format=%s"), "init", "integration unmoved")
	if !cloneExists(t, state, "direct-edits") || !strings.Contains(r.git("branch", "--list", "lsf/*"), "direct-edits") {
		t.Error("the clone or its branch is gone")
	}
}

// A requester edit to a tracked file in the main checkout while the call
// runs stops it at the fast-forward: a stage-fault naming the file, no
// agent-null, the clone and its branch kept. An untracked requester file
// does not stop it.
func TestFastForwardRequesterEdit(t *testing.T) {
	res, sigs, r, state := ffCase(t, "kept", answer{Return: scribeHead, Shell: commitEdit + " && echo mine >> @main/README.md"})
	if res != nil || len(sigs) != 1 || sigs[0].Kind != "stage-fault" {
		t.Fatalf("%+v %+v", res, sigs)
	}
	contains(t, sigs[0].Statement, `the main checkout has uncommitted changes to ["README.md"]`)
	if s := nullUnlessFaulted(sigs, plan.At{Stage: "direct-edits"}); len(s) != 1 {
		t.Errorf("agent-null added after the fault: %+v", s)
	}
	eq(t, r.git("log", "-1", "--format=%s"), "init")
	if !cloneExists(t, state, "direct-edits") || !strings.Contains(r.git("branch", "--list", "lsf/*"), "direct-edits") {
		t.Error("the clone or its branch is gone")
	}

	res, sigs, r, _ = ffCase(t, "kept", answer{Return: scribeHead, Shell: commitEdit + " && echo mine > @main/notes.txt"})
	if res == nil || len(sigs) != 0 || r.git("log", "-1", "--format=%s") != "edit" {
		t.Fatalf("untracked: %+v %+v", res, sigs)
	}
}

// The integration branch moving under the call (a requester commit) is a
// fault: the call's commits sit on an earlier tip.
func TestFastForwardDiverged(t *testing.T) {
	res, sigs, r, _ := ffCase(t, "kept", answer{Return: scribeHead, Shell: commitEdit + " && git -C @main commit -q --allow-empty -m moved"})
	if res != nil || len(sigs) != 1 || sigs[0].Kind != "stage-fault" {
		t.Fatalf("%+v %+v", res, sigs)
	}
	match(t, sigs[0].Statement, `integration moved to [0-9a-f]{40} while [0-9a-f]{40} was made on an earlier tip`)
	eq(t, r.git("log", "-1", "--format=%s"), "moved")
}

// A writer repo call on a discarded role lost its commits: a fault naming
// the role fix, never a silent skip.
func TestFastForwardDiscardedWriter(t *testing.T) {
	res, sigs, r, _ := ffCase(t, "discarded", answer{Return: scribeHead, Shell: commitEdit})
	if res != nil || len(sigs) != 1 || sigs[0].Kind != "stage-fault" {
		t.Fatalf("%+v %+v", res, sigs)
	}
	contains(t, sigs[0].Statement, "writer direct-edits ran in a discarded clone, so its commits were not kept: give its role clone: kept")
	eq(t, r.git("log", "-1", "--format=%s"), "init")
}

// The ff: step's body re-run after an interrupted step (the fast-forward
// made, the record not) applies nothing; a clone HEAD off the fetched tip
// is a fault.
func TestFastForwardStepRerun(t *testing.T) {
	r := newGitRepo(t)
	o := gitOrch(t, r, nil)
	o.ctx.integrationBranch = "integration"
	r.git("checkout", "-q", "-b", "lsf/x/w")
	tip := r.commit("e.txt", "edit\n", "edit")
	r.git("checkout", "-q", "integration")
	fn := o.ffStep("lsf/x/w", tip)
	if _, err := fn(context.Background(), "ff:w", false); err != nil {
		t.Fatal(err)
	}
	eq(t, r.head(), tip)
	v, err := fn(context.Background(), "ff:w", true)
	if err != nil || !strings.Contains(string(v), tip) {
		t.Fatalf("rerun: %s %v", v, err)
	}
	eq(t, r.head(), tip, "nothing more applied")

	_, err = o.ffStep("lsf/x/w", strings.Repeat("0", 40))(context.Background(), "ff:w", false)
	errMatch(t, err, `the clone's HEAD 0{40} is not lsf/x/w's tip`)
}

// Each fault's remedy works on relaunch, the replay re-fetching the
// clone's branch: uncommitted changes committed in the clone; a diverged
// branch rebased there; a role changed to kept re-runs the call.
func TestFastForwardRecovery(t *testing.T) {
	f := newFFLaunch(t, "kept", answer{Return: scribeHead, Shell: commitEdit + " && echo x > loose.txt"})
	res, sigs := f.launch()
	if res != nil || !slices.Equal(kinds(sigs), []string{"uncommitted-at-return@direct-edits", "stage-fault@direct-edits"}) {
		t.Fatalf("held: %+v %v", res, kinds(sigs))
	}
	contains(t, sigs[1].Statement, "its commits were not fast-forwarded to integration")
	eq(t, f.r.git("log", "-1", "--format=%s"), "init")
	c := f.theClone()
	c.git("add", "loose.txt")
	c.git("commit", "-q", "-m", "requester")
	res, sigs = f.launch()
	if res == nil || len(sigs) != 0 || f.r.git("log", "-1", "--format=%s") != "requester" {
		t.Fatalf("after the commit: %+v %v", res, kinds(sigs))
	}

	f = newFFLaunch(t, "kept", answer{Return: scribeHead, Shell: commitEdit + " && git -C @main commit -q --allow-empty -m moved"})
	if res, _ := f.launch(); res != nil {
		t.Fatalf("diverged: %+v", res)
	}
	c = f.theClone()
	c.git("fetch", "-q", f.r.dir, "integration")
	c.git("rebase", "-q", "FETCH_HEAD")
	res, sigs = f.launch()
	if res == nil || len(sigs) != 0 || f.r.git("log", "--format=%s", "-2") != "edit\nmoved" {
		t.Fatalf("after the rebase: %+v %v %q", res, kinds(sigs), f.r.git("log", "--format=%s", "-2"))
	}

	f = newFFLaunch(t, "discarded", answer{Return: scribeHead, Shell: commitEdit})
	if res, _ := f.launch(); res != nil {
		t.Fatalf("discarded: %+v", res)
	}
	f.cfg.Roles = map[string]lsfrt.Role{"w": {Tools: "default", PermissionMode: "dontAsk", Clone: "kept"}}
	res, sigs = f.launch()
	if res == nil || len(sigs) != 0 || f.r.git("log", "-1", "--format=%s") != "edit" {
		t.Fatalf("after the role fix: %+v %v", res, kinds(sigs))
	}
}

// An isolated writer on a discarded role is the same fault: its commits
// went with the clone.
func TestDiscardedIsolatedWriter(t *testing.T) {
	f := newFFLaunch(t, "discarded", answer{Return: scribeHead, Shell: commitEdit})
	var sigs []*plan.Signal
	var res *ret[scribeResult]
	sum, err := lsfrt.Run(context.Background(), f.cfg, func(ctx context.Context, rt *lsfrt.Runtime) (json.RawMessage, error) {
		a, err := parseArgs(rt.Args())
		if err != nil {
			return nil, err
		}
		c, err := newRunCtx(a)
		if err != nil {
			return nil, err
		}
		res, sigs, _, err = callBranch[scribeResult](ctx, newOrch(rt, c, nil), site{at: plan.At{Stage: "implement", Unit: "A"}, writer: true}, "Make these edits",
			lsfrt.Opts{Label: "implement:A", Isolation: "worktree", Schema: scribeSchema})
		return nil, err
	})
	if err != nil || sum.StopReason != lsfrt.StopReturned {
		t.Fatalf("%v %+v", err, sum)
	}
	if res != nil || len(sigs) != 1 || sigs[0].Kind != "stage-fault" {
		t.Fatalf("%+v %v", res, kinds(sigs))
	}
	contains(t, sigs[0].Statement, "writer implement:A ran in a discarded clone")
}

// The re-plan's commits, too, reach the integration branch only by its ff:
// step: its clone is on integration, which is unmoved until the step.
func TestFastForwardReplan(t *testing.T) {
	f := newFFLaunch(t, "kept", answer{Return: scribeHead, Shell: commitEdit + " && git -C @main log -1 --format=%s > main-at-return.txt && git add main-at-return.txt && git commit -q -m seen"})
	f.label, f.stage = "replan:w1", "replan"
	res, sigs := f.launch()
	if res == nil || len(sigs) != 0 {
		t.Fatalf("%+v %v", res, kinds(sigs))
	}
	eq(t, f.r.git("show", "HEAD:main-at-return.txt"), "init", "integration unmoved while the call ran")
	eq(t, f.r.git("log", "--format=%s", "-2"), "seen\nedit", "fast-forwarded after it")
	if !journaled(t, f.cfg.StateDir, "ff_replan_w1") {
		t.Error("no ff: step record")
	}
}
