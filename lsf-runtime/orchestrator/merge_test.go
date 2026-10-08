package orchestrator

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

const (
	mPlan    = "docs/plans/f-breakdown.md"
	mArchive = "docs/plans/f-archive.md"
	mArch    = "docs/architecture/f.md"
)

// mergeFixture is a repository whose plan has units ids, each with a
// completion-report-like branch report-<id> one commit ahead of the
// integration branch, writing files[id].
type mergeFixture struct {
	r    *gitRepo
	shas map[string]string
	arch string
	wt   string // the gitRT root every launch over the fixture shares
}

// mc is the wave's merge clone, made by the step (or makeClone); its
// directory may not exist.
func (f *mergeFixture) mc() *gitRepo { return &gitRepo{f.r.t, filepath.Join(f.wt, "merge", "w1")} }

// makeClone makes the merge clone as the step would, for a test that
// builds the state a killed step leaves in it.
func (f *mergeFixture) makeClone() *gitRepo {
	f.r.t.Helper()
	if err := mergeCloneAt(f.r.dir, f.mc().dir); err != nil {
		f.r.t.Fatal(err)
	}
	return f.mc()
}

func newMergeFixture(t *testing.T, files map[string]map[string]string, ids ...string) *mergeFixture {
	t.Helper()
	r := newGitRepo(t)
	f := &mergeFixture{r: r, shas: map[string]string{}, wt: t.TempDir()}
	f.arch = r.commit(mArch, "# Architecture\n", "architecture")
	units := []*plan.Unit{}
	for _, id := range ids {
		units = append(units, orchtest.Unit(id))
	}
	r.commit(mPlan, planDocFor(orchtest.Graph("", units...)), "plan")
	for _, id := range ids {
		r.git("checkout", "-q", "-b", "report-"+id, "integration")
		for p, c := range files[id] {
			r.write(p, c)
			r.git("add", "--", p)
		}
		r.git("commit", "-q", "--allow-empty", "-m", "unit "+id)
		f.shas[id] = r.head()
	}
	r.git("checkout", "-q", "integration")
	return f
}

func (f *mergeFixture) in(ids ...string) mergeInput {
	in := mergeInput{Wave: 1, Name: "f", IntegrationBranch: "integration", PlanPath: mPlan, ArchivePath: mArchive,
		LedgerPath: "docs/plans/f-signals/x.md", ArchitectureDoc: mArch, PauseRef: "refs/lsf/f/pause", Units: []mergeUnitIn{}}
	for _, id := range ids {
		in.Units = append(in.Units, mergeUnitIn{Unit: id, Branch: "report-" + id, Sha: f.shas[id], Report: "docs/plans/" + id + "-completion.md"})
	}
	return in
}

// mergeOrch is an orch over f's repository whose conflict calls go to h.
func mergeOrch(t *testing.T, f *mergeFixture, g gitRunner, h map[string]orchtest.Handler) *orch {
	t.Helper()
	s := orchtest.New(t, orchtest.TwoWaves(), h, nil)
	o := newOrch(gitRT{s, f.r.dir, nil, f.wt}, runCtxFor(t, nil), nil)
	if g != nil {
		o.git = g
	}
	o.today = func() string { return "2026-10-07" }
	return o
}

func runMerge(t *testing.T, o *orch, in mergeInput, interrupted bool) (mergeResult, error) {
	t.Helper()
	raw, err := o.mergeStage(in)(context.Background(), "merge:w1", interrupted)
	if err != nil {
		return mergeResult{}, err
	}
	r := decode[mergeResult](orchtest.New(t, orchtest.TwoWaves(), nil, nil), "merge:w1", raw)
	if r == nil {
		t.Fatalf("merge value does not decode: %s", raw)
	}
	return r.Result, nil
}

func statuses(r mergeResult) string {
	out := []string{}
	for _, m := range r.Merges {
		out = append(out, m.Unit+"="+m.Status)
	}
	return strings.Join(out, " ")
}

func (r *gitRepo) mergeInProgress() bool {
	_, err := os.Stat(filepath.Join(r.dir, ".git", "MERGE_HEAD"))
	return err == nil
}

func notContains(t *testing.T, s, sub string, msg ...any) {
	t.Helper()
	if strings.Contains(s, sub) {
		t.Errorf("%v: %q contains %q", msg, s, sub)
	}
}

func regexpQuote(s string) string { return regexp.QuoteMeta(s) }

var distinct = map[string]map[string]string{"A": {"src/a.go": "package a\n"}, "B": {"src/b.go": "package b\n"}, "C": {"src/c.go": "package c\n"}}

// Clean merges in order: a merge commit each, parents and trailers, then
// one bookkeeping commit of the plan and the archive; every SHA full.
func TestMergeClean(t *testing.T) {
	f := newMergeFixture(t, distinct, "A", "B")
	pre := f.r.head()
	res, err := runMerge(t, mergeOrch(t, f, nil, nil), f.in("A", "B"), false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(res), "A=clean B=clean")
	ma, mb := res.Merges[0].Sha, res.Merges[1].Sha
	eq(t, f.r.git("log", "-1", "--format=%P", ma), pre+" "+f.shas["A"])
	eq(t, f.r.git("log", "-1", "--format=%P", mb), ma+" "+f.shas["B"])
	eq(t, f.r.git("log", "-1", "--format=%s%n%(trailers:key=Lsf-Unit,valueonly)%(trailers:key=Lsf-Merge,valueonly)", mb), "merge(f): B\nB\nclean")
	eq(t, res.PlanSha, f.r.head())
	eq(t, f.r.git("log", "-1", "--format=%P", res.PlanSha), mb)
	eq(t, f.r.git("show", "--name-only", "--format=", res.PlanSha), mArchive+"\n"+mPlan)
	eq(t, res.ArchitectureSha, f.arch)
	for _, s := range []string{ma, mb, res.PlanSha, res.ArchitectureSha} {
		eq(t, len(s), 40, s)
	}
	contains(t, f.r.read(mPlan), "### B — complete at "+mb+"; entry archived to "+mArchive+"; report docs/plans/B-completion.md")
	contains(t, f.r.read(mPlan), "### 2026-10-07 — A complete\nReport: docs/plans/A-completion.md @ "+ma+". Ledger: docs/plans/f-signals/x.md wave 1.\n")
	eq(t, f.r.git("status", "--porcelain"), "")
	eq(t, len(res.Signals), 0)
}

// A conflict goes to the model call; a resolution it stages is checked
// and committed by the stage, recorded resolved with the files.
func TestMergeConflictResolved(t *testing.T) {
	both := map[string]map[string]string{"A": {"shared.txt": "from A\n"}, "B": {"shared.txt": "from B\n"}}
	f := newMergeFixture(t, both, "A", "B")
	var prompt string
	o := mergeOrch(t, f, nil, map[string]orchtest.Handler{"merge-conflict": func(c orchtest.Call, _ *orchtest.State) any {
		prompt = c.Prompt
		f.mc().write("shared.txt", "from A\nfrom B\n")
		f.mc().git("add", "shared.txt")
		return R(M{"status": "resolved", "files": []string{"shared.txt"}, "statement": "both lines kept"})
	}})
	res, err := runMerge(t, o, f.in("A", "B"), false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(res), "A=clean B=resolved")
	eqs(t, res.Merges[1].Files, []string{"shared.txt"})
	mb := res.Merges[1].Sha
	eq(t, f.r.git("log", "-1", "--format=%(trailers:key=Lsf-Merge,valueonly)%(trailers:key=Lsf-Conflict,valueonly)", mb), "resolved\nshared.txt")
	eq(t, f.r.git("show", mb+":shared.txt"), "from A\nfrom B")
	contains(t, prompt, "Never commit")
	sigs := plan.SignalsFromMerge(res.MergeResult, plan.At{Wave: plan.W(1)})
	if len(sigs) != 1 || sigs[0].Kind != "claimed-disjoint-conflict" {
		t.Errorf("signals %+v", sigs)
	}
	eq(t, f.r.git("status", "--porcelain"), "")
}

// Resolutions the stage does not accept, and an unresolvable report, end
// in an aborted merge: no merge in progress, only the requester's own
// untracked file left; the next unit still merges.
func TestMergeConflictNotAccepted(t *testing.T) {
	conflicting := map[string]map[string]string{"A": {"shared.txt": "from A\n"}, "B": {"shared.txt": "from B\n", "src/b2.go": "package b\n"}, "C": {"src/c.go": "package c\n"}}
	for name, h := range map[string]orchtest.Handler{
		"unresolvable": func(orchtest.Call, *orchtest.State) any {
			return R(M{"status": "unresolvable", "files": []string{"shared.txt"}, "statement": "needs judgement"})
		},
		"markers left": func(orchtest.Call, *orchtest.State) any {
			return R(M{"status": "resolved", "files": []string{"shared.txt"}, "statement": "s"})
		},
		"markers staged":         nil, // set below
		"a clean path rewritten": nil,
	} {
		t.Run(name, func(t *testing.T) {
			f := newMergeFixture(t, conflicting, "A", "B", "C")
			resolve := func(extra func()) orchtest.Handler {
				return func(orchtest.Call, *orchtest.State) any {
					f.mc().write("shared.txt", "from A\nfrom B\n")
					f.mc().git("add", "shared.txt")
					extra()
					return R(M{"status": "resolved", "files": []string{"shared.txt"}, "statement": "s"})
				}
			}
			switch name {
			case "markers staged":
				h = func(orchtest.Call, *orchtest.State) any {
					f.mc().git("add", "shared.txt")
					return R(M{"status": "resolved", "files": []string{"shared.txt"}, "statement": "s"})
				}
			case "a clean path rewritten":
				// src/b2.go merged clean; the call stages other content.
				h = resolve(func() { f.mc().write("src/b2.go", "tampered\n"); f.mc().git("add", "src/b2.go") })
			}
			f.r.write("notes.txt", "requester's\n")
			res, err := runMerge(t, mergeOrch(t, f, nil, map[string]orchtest.Handler{"merge-conflict": h}), f.in("A", "B", "C"), false)
			if err != nil {
				t.Fatal(err)
			}
			eq(t, statuses(res), "A=clean B=unresolvable C=clean")
			eqs(t, res.Merges[1].Files, []string{"shared.txt"})
			if f.r.mergeInProgress() {
				t.Error("merge in progress")
			}
			eq(t, f.r.git("status", "--porcelain"), "?? notes.txt")
			notContains(t, f.r.read(mPlan), "### B — complete")
		})
	}
}

// A conflict call that commits is a fault, with no merge left in progress.
func TestMergeConflictCallCommits(t *testing.T) {
	both := map[string]map[string]string{"A": {"shared.txt": "from A\n"}, "B": {"shared.txt": "from B\n"}}
	f := newMergeFixture(t, both, "A", "B")
	o := mergeOrch(t, f, nil, map[string]orchtest.Handler{"merge-conflict": func(orchtest.Call, *orchtest.State) any {
		f.mc().write("shared.txt", "x\n")
		f.mc().git("commit", "-q", "-am", "agent's own commit")
		return R(M{"status": "resolved", "files": []string{"shared.txt"}, "statement": "s"})
	}})
	_, err := runMerge(t, o, f.in("A", "B"), false)
	errMatch(t, err, `the conflict call committed [0-9a-f]{40}`)
	if f.r.mergeInProgress() {
		t.Error("merge in progress")
	}
}

// The requester's untracked file in a merge's way in the main checkout: the
// merge is made in the merge clone, and git refuses the fast-forward over
// the file, a fault; the file is untouched, the integration branch
// unmoved, and the merge waits in the clone for the relaunch to adopt.
func TestMergeRequesterFileAtFastForward(t *testing.T) {
	f := newMergeFixture(t, distinct, "A", "B")
	pre := f.r.head()
	f.r.write("src/a.go", "requester's draft\n")
	o := mergeOrch(t, f, nil, nil)
	_, err := runMerge(t, o, f.in("A", "B"), false)
	errMatch(t, err, `(?s)git refused to fast-forward integration to [0-9a-f]{40}.*\tsrc/a\.go`)
	eq(t, f.r.read("src/a.go"), "requester's draft\n")
	eq(t, f.r.head(), pre, "nothing fast-forwarded")
	merged := f.mc().head()
	os.Remove(filepath.Join(f.r.dir, "src/a.go"))
	res, err := runMerge(t, o, f.in("A", "B"), true)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(res), "A=clean B=clean")
	eq(t, res.Merges[0].Sha, merged, "the clone's merge adopted")
}

// A requester's tracked edit in the main checkout stops the step before
// the first merge; one made while the step runs (here, during a conflict
// call) stops it at the next fast-forward, with the merge kept in the
// clone.
func TestMergeRequesterEditInMain(t *testing.T) {
	f := newMergeFixture(t, distinct, "A")
	f.r.write("README.md", "requester's edit\n")
	_, err := runMerge(t, mergeOrch(t, f, nil, nil), f.in("A"), false)
	errMatch(t, err, `the main checkout has uncommitted changes to \["README\.md"\]`)
	if _, statErr := os.Stat(f.mc().dir); statErr == nil {
		t.Error("a merge clone was made")
	}

	both := map[string]map[string]string{"A": {"shared.txt": "from A\n"}, "B": {"shared.txt": "from B\n"}}
	g := newMergeFixture(t, both, "A", "B")
	o := mergeOrch(t, g, nil, map[string]orchtest.Handler{"merge-conflict": func(orchtest.Call, *orchtest.State) any {
		g.r.write("README.md", "edited meanwhile\n")
		g.mc().write("shared.txt", "from A\nfrom B\n")
		g.mc().git("add", "shared.txt")
		return R(M{"status": "resolved", "files": []string{"shared.txt"}, "statement": "s"})
	}})
	afterA := ""
	_, err = runMerge(t, o, g.in("A", "B"), false)
	errMatch(t, err, `the main checkout has uncommitted changes to \["README\.md"\]`)
	afterA = g.r.head()
	eq(t, g.r.git("log", "-1", "--format=%(trailers:key=Lsf-Unit,valueonly)", afterA), "A", "A fast-forwarded before the edit")
	eq(t, g.mc().git("log", "-1", "--format=%(trailers:key=Lsf-Unit,valueonly)"), "B", "B's merge kept in the clone")
}

// Faults before anything merges: staged changes, a merge in progress the
// stage did not start, a unit already merged, a plan the bookkeeping
// cannot record, a stale index.lock.
func TestMergeFaultsBeforeMerging(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(f *mergeFixture)
		want  string
	}{
		{"staged", func(f *mergeFixture) { f.r.write("README.md", "x\n"); f.r.git("add", "README.md") }, `the main checkout has uncommitted changes to \["README\.md"\]`},
		{"foreign merge in progress", func(f *mergeFixture) {
			f.r.git("checkout", "-q", "-b", "other", "integration~1")
			f.r.commit(mPlan, "other\n", "other")
			f.r.git("checkout", "-q", "integration")
			cmd := exec.Command("git", "merge", "other")
			cmd.Dir = f.r.dir
			cmd.Run()
		}, `a merge is in progress in the main checkout; finish or abort it`},
		{"foreign merge in progress in the merge clone", func(f *mergeFixture) {
			f.r.git("checkout", "-q", "-b", "other", "integration~1")
			f.r.commit(mPlan, "other\n", "other")
			f.r.git("checkout", "-q", "integration")
			cmd := exec.Command("git", "merge", "other")
			cmd.Dir = f.makeClone().dir
			cmd.Run()
		}, `a merge is in progress in the merge clone .* that this stage did not start`},
		{"already in integration", func(f *mergeFixture) { f.r.git("merge", "-q", "--no-ff", "-m", "by hand", "report-A") }, `is already in HEAD, merged outside this stage`},
		{"merged before under another commit", func(f *mergeFixture) {
			f.r.git("merge", "-q", "--no-ff", "-m", "merge(f): A\n\nLsf-Step: merge:w1\nLsf-Unit: A\nLsf-Merge: clean", "report-B")
		}, `unit A was already merged at`},
		{"no entry", func(f *mergeFixture) {
			f.r.commit(mPlan, strings.Replace(f.r.read(mPlan), "### A\n", "### Z\n", 1), "rename")
		}, `nothing is merged: unit A has no .### A. entry`},
		{"stale index.lock", func(f *mergeFixture) { os.WriteFile(filepath.Join(f.r.dir, ".git", "index.lock"), nil, 0o644) }, `index\.lock`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMergeFixture(t, distinct, "A", "B")
			tc.setup(f)
			head, inProgress := f.r.head(), f.r.mergeInProgress()
			_, err := runMerge(t, mergeOrch(t, f, nil, nil), f.in("A", "B"), false)
			errMatch(t, err, tc.want)
			eq(t, f.r.head(), head, "nothing committed")
			eq(t, f.r.mergeInProgress(), inProgress, "a foreign merge is left as it is")
		})
	}
}

// A pre-merge-commit hook's `git add` does not reach the merge commit on
// git 2.54, but leaves its path staged for a later commit to sweep in: a
// fault naming it. Where the hook's change did reach the commit, the tree
// check names it (verifyMerge, shown on a hand-made commit).
func TestMergeHookStaging(t *testing.T) {
	f := newMergeFixture(t, distinct, "A")
	f.r.hook("pre-merge-commit", "echo hooked >> README.md && git add README.md")
	_, err := runMerge(t, mergeOrch(t, f, nil, nil), f.in("A"), false)
	errMatch(t, err, `after the merge of A at [0-9a-f]{40}, \["README\.md"\] is staged`)

	g := newMergeFixture(t, distinct, "A")
	pre := g.r.head()
	g.r.git("merge", "-q", "--no-ff", "--no-commit", g.shas["A"])
	g.r.write("README.md", "altered\n")
	g.r.git("add", "README.md")
	g.r.git("commit", "-q", "-m", mergeMessage(g.in("A"), "merge:w1", "A", "clean", nil))
	_, _, err = mergeGit{gitOps: gitOps{execGit{}}, dir: g.r.dir}.verifyMerge(context.Background(), g.r.head(), pre, g.shas["A"], "merge:w1", "A")
	errMatch(t, err, `fails verification: its tree is not git's merge`)
}

// Injections into the bookkeeping commit: a hook that truncates the plan
// or drops the block's closing fence lands a commit the stage names (never
// silently); a rejected commit makes none and leaves the paths and index
// clean, and a rerun adopts the merges and records the bookkeeping once.
func TestMergeBookkeepingInjections(t *testing.T) {
	for _, tc := range []struct{ name, script string }{
		{"truncation", "$!d"},
		{"dropped fence", "/^```$/d"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMergeFixture(t, distinct, "A")
			f.r.hook("pre-commit", sedHook(mPlan, tc.script))
			_, err := runMerge(t, mergeOrch(t, f, nil, nil), f.in("A"), false)
			errMatch(t, err, `commit [0-9a-f]{40} fails verification: `+regexpQuote(mPlan)+` differs from the content written`)
		})
	}

	f := newMergeFixture(t, distinct, "A")
	f.r.hook("pre-commit", "exit 1")
	o := mergeOrch(t, f, nil, nil)
	_, err := runMerge(t, o, f.in("A"), false)
	errMatch(t, err, `commit of `+regexpQuote(mPlan)+` and `+regexpQuote(mArchive)+` rejected`)
	merged := f.r.head()
	eq(t, f.r.git("log", "-1", "--format=%(trailers:key=Lsf-Unit,valueonly)", merged), "A", "the merge stays")
	eq(t, f.r.git("status", "--porcelain"), "")
	f.r.hook("pre-commit", "")
	res, err := runMerge(t, o, f.in("A"), true)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, res.Merges[0].Sha, merged, "adopted, not merged again")
	eq(t, f.r.git("rev-list", "--count", merged+"..HEAD"), "1", "one bookkeeping commit")
}

// A commit in the merge clone past the main repo's tip that the step did
// not make (here, one a conflict call might have made) is a fault naming
// it, before anything merges; one behind is reset.
func TestMergeCloneSync(t *testing.T) {
	f := newMergeFixture(t, distinct, "A")
	c := f.makeClone()
	c.commit("stray.txt", "x\n", "not the step's")
	stray := c.head()
	_, err := runMerge(t, mergeOrch(t, f, nil, nil), f.in("A"), true)
	errMatch(t, err, `holds `+stray+`, which is not this step's merge or bookkeeping; remove .* and relaunch`)

	g := newMergeFixture(t, distinct, "A")
	g.makeClone()
	g.r.commit("README.md", "moved\n", "requester's commit") // main ahead of the clone
	res, err := runMerge(t, mergeOrch(t, g, nil, nil), g.in("A"), true)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(res), "A=clean")
	eq(t, g.r.git("show", res.Merges[0].Sha+":README.md"), "moved", "merged onto main's tip")
}

// isMerge matches the step's own merge in the clone, not a fast-forward.
func isMerge(a []string) bool { return len(a) > 1 && a[0] == "merge" && a[1] == "--no-ff" }

// failNth fails the nth git command match accepts, without running it, as
// a kill would leave it.
type failNth struct {
	inner gitRunner
	match func(args []string) bool
	n     int
	mu    sync.Mutex
	seen  int
}

func (g *failNth) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	g.mu.Lock()
	hit := false
	if g.match(args) {
		g.seen++
		hit = g.seen == g.n
	}
	g.mu.Unlock()
	if hit {
		return nil, &gitError{args: args, err: errors.New("killed")}
	}
	return g.inner.run(ctx, dir, args...)
}

func argIs(i int, v string) func([]string) bool {
	return func(a []string) bool { return len(a) > i && a[i] == v }
}

// Kill and resume: an interrupted rerun adopts what the step committed and
// does the rest once; a rerun after the whole step merges and records
// nothing again.
func TestMergeResume(t *testing.T) {
	t.Run("second merge killed", func(t *testing.T) {
		f := newMergeFixture(t, distinct, "A", "B")
		_, err := runMerge(t, mergeOrch(t, f, &failNth{inner: execGit{}, match: isMerge, n: 2}, nil), f.in("A", "B"), false)
		errMatch(t, err, `killed`)
		first := f.r.head()
		res, err := runMerge(t, mergeOrch(t, f, nil, nil), f.in("A", "B"), true)
		if err != nil {
			t.Fatal(err)
		}
		eq(t, res.Merges[0].Sha, first)
		eq(t, f.r.git("rev-list", "--first-parent", "--count", first+"..HEAD"), "2", "B's merge and one bookkeeping commit")
	})
	t.Run("whole step, then a rerun", func(t *testing.T) {
		f := newMergeFixture(t, distinct, "A", "B")
		o := mergeOrch(t, f, nil, nil)
		a, err := runMerge(t, o, f.in("A", "B"), false)
		if err != nil {
			t.Fatal(err)
		}
		head := f.r.head()
		b, err := runMerge(t, o, f.in("A", "B"), true)
		if err != nil {
			t.Fatal(err)
		}
		eq(t, f.r.head(), head, "no new commit")
		eq(t, b.PlanSha, a.PlanSha)
		eq(t, statuses(b), statuses(a))
		eq(t, b.Merges[1].Sha, a.Merges[1].Sha)
	})
	t.Run("killed after the bookkeeping commit", func(t *testing.T) {
		f := newMergeFixture(t, distinct, "A")
		// The first `reset` is commitFiles' after the bookkeeping commit;
		// failing it is the step dying after the commit, before its
		// fast-forward.
		_, err := runMerge(t, mergeOrch(t, f, &failNth{inner: execGit{}, match: argIs(0, "reset"), n: 1}, nil), f.in("A"), false)
		errMatch(t, err, `killed`)
		head := f.mc().head() // the commit is in the clone, not yet fast-forwarded
		if f.r.head() == head {
			t.Fatal("the bookkeeping commit reached main before the kill")
		}
		res, err := runMerge(t, mergeOrch(t, f, nil, nil), f.in("A"), true)
		if err != nil {
			t.Fatal(err)
		}
		eq(t, f.r.head(), head, "the bookkeeping commit adopted")
		eq(t, res.PlanSha, head)
	})
	t.Run("a merge killed after its conflict was staged", func(t *testing.T) {
		both := map[string]map[string]string{"A": {"shared.txt": "from A\n"}, "B": {"shared.txt": "from B\n"}}
		f := newMergeFixture(t, both, "A", "B")
		calls := 0
		h := map[string]orchtest.Handler{"merge-conflict": func(orchtest.Call, *orchtest.State) any {
			calls++ // a replay of the journaled call: the resolution is staged already
			return R(M{"status": "resolved", "files": []string{"shared.txt"}, "statement": "s"})
		}}
		// The state a kill leaves in the merge clone after A's merge (not
		// yet fast-forwarded) and B's staged resolution, its conflict call
		// pending: a real kill runs no abort, so it is built here as the
		// step would. The resume runs in the clone, adopts A's merge by its
		// trailers and fast-forwards to it, then completes B.
		in := f.in("A", "B")
		pre := f.r.head()
		c := f.makeClone()
		c.git("merge", "-q", "--no-ff", "-m", mergeMessage(in, "merge:w1", "A", "clean", nil), f.shas["A"])
		mergeA := c.head()
		cmd := exec.Command("git", "merge", "--no-ff", "-m", mergeMessage(in, "merge:w1", "B", "clean", nil), f.shas["B"])
		cmd.Dir = c.dir
		cmd.Run()
		c.write("shared.txt", "from A\nfrom B\n")
		c.git("add", "shared.txt")
		if !c.mergeInProgress() || f.r.head() != pre {
			t.Fatal("no merge in progress in the clone, or main moved")
		}
		var cwd string
		h["merge-conflict"] = func(call orchtest.Call, _ *orchtest.State) any {
			calls++
			cwd = call.Opts.Dir
			return R(M{"status": "resolved", "files": []string{"shared.txt"}, "statement": "s"})
		}
		res, err := runMerge(t, mergeOrch(t, f, nil, h), in, true)
		if err != nil {
			t.Fatal(err)
		}
		eq(t, statuses(res), "A=clean B=resolved")
		eq(t, res.Merges[0].Sha, mergeA, "A's merge adopted from the clone")
		eq(t, calls, 1)
		eq(t, cwd, c.dir, "the conflict call ran in the merge clone")
		eq(t, f.r.git("show", "HEAD~1:shared.txt"), "from A\nfrom B")
		if _, err := os.Stat(c.dir); err == nil {
			t.Error("the merge clone outlived the step")
		}
	})
	t.Run("a replayed resolution that is gone", func(t *testing.T) {
		both := map[string]map[string]string{"A": {"shared.txt": "from A\n"}, "B": {"shared.txt": "from B\n"}}
		f := newMergeFixture(t, both, "A", "B")
		labels := []string{}
		h := map[string]orchtest.Handler{"merge-conflict": func(c orchtest.Call, _ *orchtest.State) any {
			labels = append(labels, c.Label)
			v := R(M{"status": "resolved", "files": []string{"shared.txt"}, "statement": "s"})
			if len(labels) == 1 {
				b, _ := plan.MarshalJS(v, "")
				return lsfrt.CallResult{Value: b, Replayed: true}
			}
			f.mc().write("shared.txt", "from A\nfrom B\n")
			f.mc().git("add", "shared.txt")
			return v
		}}
		res, err := runMerge(t, mergeOrch(t, f, nil, h), f.in("A", "B"), false)
		if err != nil {
			t.Fatal(err)
		}
		eq(t, statuses(res), "A=clean B=resolved")
		eqs(t, labels, []string{"merge-conflict:w1:B", "merge-conflict:w1:B#2"})
	})
}

// checkMergeShas rejects a merge value, live or replayed, whose SHAs do not
// stand: a clean merge without its SHA, a SHA that does not resolve, one
// with revision syntax, one off the integration branch, a bad
// architectureSha. Without a repository nothing is checked.
func TestCheckMergeShas(t *testing.T) {
	f := newMergeFixture(t, distinct, "A")
	o := mergeOrch(t, f, nil, nil)
	res, err := runMerge(t, o, f.in("A"), false)
	if err != nil {
		t.Fatal(err)
	}
	l := &launch{orch: o, completed: plan.Completed{}}
	ok, err := l.checkMergeShas(context.Background(), &res)
	if err != nil || ok != "" {
		t.Fatalf("a good value: %q %v", ok, err)
	}
	for _, tc := range []struct {
		name string
		edit func(r *mergeResult)
		want string
	}{
		{"clean merge without its SHA", func(r *mergeResult) { r.Merges[0].Sha = "" }, `returned a clean merge of A without its SHA`},
		{"unresolvable", func(r *mergeResult) { r.Merges[0].Sha = "deadbeefdeadbeef" }, `merge SHA of A deadbeefdeadbeef does not resolve`},
		{"revision syntax", func(r *mergeResult) { r.Merges[0].Sha = "HEAD~1" }, `merge SHA of A "HEAD~1" is not a commit SHA`},
		{"bad architectureSha", func(r *mergeResult) { r.ArchitectureSha = "0000000" }, `architectureSha 0000000 does not resolve`},
	} {
		r := res
		r.MergeResult.Merges = append([]plan.Merge{}, res.Merges...)
		tc.edit(&r)
		got, err := l.checkMergeShas(context.Background(), &r)
		if err != nil || (tc.want != "" && !strings.Contains(got, tc.want)) {
			t.Errorf("%s: %q %v", tc.name, got, err)
		}
	}
	off := res
	other := f.r.commitOn("side", "x.md")
	off.PlanSha = other
	got, _ := l.checkMergeShas(context.Background(), &off)
	contains(t, got, "is not on integration")

	nogit := &launch{orch: &orch{ctx: o.ctx}, completed: plan.Completed{}}
	bad := res
	bad.PlanSha = "nope"
	if got, err := nogit.checkMergeShas(context.Background(), &bad); got != "" || err != nil {
		t.Errorf("no repository: %q %v", got, err)
	}
}

// commitOn makes a commit on a new branch off integration and returns it,
// leaving the checkout on integration.
func (r *gitRepo) commitOn(branch, path string) string {
	r.git("checkout", "-q", "-b", branch, "integration")
	sha := r.commit(path, "x\n", "side")
	r.git("checkout", "-q", "integration")
	return sha
}

// A conflict call that touches the merge clone beyond the merge (stages,
// writes, changes or deletes another path) is a fault naming the paths,
// with the merge aborted: not a resolution to decline quietly.
func TestMergeConflictCallOutside(t *testing.T) {
	both := map[string]map[string]string{"A": {"shared.txt": "from A\n"}, "B": {"shared.txt": "from B\n"}}
	for name, tc := range map[string]struct {
		act  func(r *gitRepo)
		want string
	}{
		"another path staged":    {func(r *gitRepo) { r.write("README.md", "changed\n"); r.git("add", "README.md") }, `\["README\.md"\]`},
		"a new untracked file":   {func(r *gitRepo) { r.write("scratch.txt", "x\n") }, `\["scratch\.txt"\]`},
		"a tracked file changed": {func(r *gitRepo) { r.write("README.md", "changed\n") }, `\["README\.md"\]`},
		"a tracked file deleted": {func(r *gitRepo) { os.Remove(filepath.Join(r.dir, "README.md")) }, `\["README\.md"\]`},
	} {
		t.Run(name, func(t *testing.T) {
			f := newMergeFixture(t, both, "A", "B")
			o := mergeOrch(t, f, nil, map[string]orchtest.Handler{"merge-conflict": func(orchtest.Call, *orchtest.State) any {
				f.mc().write("shared.txt", "from A\nfrom B\n")
				f.mc().git("add", "shared.txt")
				tc.act(f.mc())
				return R(M{"status": "resolved", "files": []string{"shared.txt"}, "statement": "s"})
			}})
			_, err := runMerge(t, o, f.in("A", "B"), false)
			errMatch(t, err, `the conflict call changed `+tc.want+`, outside the merge of B`)
			if f.mc().mergeInProgress() {
				t.Error("merge in progress")
			}
		})
	}
}

// A merge git stops without conflict (a pre-merge-commit hook refusing it)
// is a fault with git's reason, not the conflict call's.
func TestMergeHookRefused(t *testing.T) {
	f := newMergeFixture(t, distinct, "A")
	f.r.hook("pre-merge-commit", "echo refused >&2; exit 1")
	calls := 0
	_, err := runMerge(t, mergeOrch(t, f, nil, map[string]orchtest.Handler{"merge-conflict": func(orchtest.Call, *orchtest.State) any {
		calls++
		return nil
	}}), f.in("A"), false)
	errMatch(t, err, `the merge of A stopped with no conflict`)
	eq(t, calls, 0)
	if f.r.mergeInProgress() {
		t.Error("merge in progress")
	}
}

// Interrupted reruns: a foreign merge in progress is still not this
// step's (no trailer), and is left; a merge commit that failed
// verification is still a fault, not adopted; a unit recorded by an
// earlier run that failed later is not recorded again, nor does the dry
// run trip on its moved entry.
func TestMergeInterruptedEdges(t *testing.T) {
	f := newMergeFixture(t, distinct, "A")
	f.r.git("checkout", "-q", "-b", "other", "integration~1")
	f.r.commit(mPlan, "other\n", "other")
	f.r.git("checkout", "-q", "integration")
	cmd := exec.Command("git", "merge", "other")
	cmd.Dir = f.makeClone().dir
	cmd.Run()
	_, err := runMerge(t, mergeOrch(t, f, nil, nil), f.in("A"), true)
	errMatch(t, err, `a merge is in progress in the merge clone .* that this stage did not start`)
	if !f.mc().mergeInProgress() {
		t.Error("the foreign merge was aborted")
	}

	g := newMergeFixture(t, distinct, "A")
	pre := g.r.head()
	g.r.git("merge", "-q", "--no-ff", "--no-commit", g.shas["A"])
	g.r.write("README.md", "altered\n")
	g.r.git("add", "README.md")
	g.r.git("commit", "-q", "-m", mergeMessage(g.in("A"), "merge:w1", "A", "clean", nil))
	_, err = runMerge(t, mergeOrch(t, g, nil, nil), g.in("A"), true)
	errMatch(t, err, `fails verification: its tree is not git's merge`)
	eq(t, g.r.git("rev-list", "--first-parent", "--count", pre+"..HEAD"), "1", "not adopted, nothing more committed")

	both := map[string]map[string]string{"A": {"src/a.go": "a\n"}, "B": {"README.md": "from B\n"}}
	h := newMergeFixture(t, both, "A", "B")
	h.r.commit("README.md", "base\n", "base") // B now conflicts
	calls := 0
	o := mergeOrch(t, h, nil, map[string]orchtest.Handler{"merge-conflict": func(orchtest.Call, *orchtest.State) any {
		if calls++; calls == 1 { // B is unresolvable this time
			return R(M{"status": "unresolvable", "files": []string{"README.md"}, "statement": "no"})
		}
		h.mc().write("README.md", "base\nfrom B\n")
		h.mc().git("add", "README.md")
		return R(M{"status": "resolved", "files": []string{"README.md"}, "statement": "s"})
	}})
	first, err := runMerge(t, o, h.in("A", "B"), false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(first), "A=clean B=unresolvable")
	second, err := runMerge(t, o, h.in("A", "B"), true)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(second), "A=clean B=resolved")
	eq(t, strings.Count(h.r.read(mPlan), "### A — complete at"), 1, "A recorded once")
	contains(t, h.r.read(mPlan), "### B — complete at "+second.Merges[1].Sha)
}

// Paths that are not plain ASCII come through merge-tree -z unquoted.
func TestMergeConflictNonASCIIPath(t *testing.T) {
	both := map[string]map[string]string{"A": {"ü s.txt": "from A\n"}, "B": {"ü s.txt": "from B\n"}}
	f := newMergeFixture(t, both, "A", "B")
	o := mergeOrch(t, f, nil, map[string]orchtest.Handler{"merge-conflict": func(orchtest.Call, *orchtest.State) any {
		f.mc().write("ü s.txt", "from A\nfrom B\n")
		f.mc().git("add", "ü s.txt")
		return R(M{"status": "resolved", "files": []string{"ü s.txt"}, "statement": "s"})
	}})
	res, err := runMerge(t, o, f.in("A", "B"), false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(res), "A=clean B=resolved")
	eqs(t, res.Merges[1].Files, []string{"ü s.txt"})
}

// A resumed conflict whose staged state holds a path beyond the merge
// (staged before the kill) is caught by the sets derived from the two
// commits, which a resume derives the same: unresolvable, aborted.
func TestMergeResumeExtraPath(t *testing.T) {
	both := map[string]map[string]string{"A": {"shared.txt": "from A\n"}, "B": {"shared.txt": "from B\n"}}
	f := newMergeFixture(t, both, "A", "B")
	in := f.in("A", "B")
	c := f.makeClone()
	c.git("merge", "-q", "--no-ff", "-m", mergeMessage(in, "merge:w1", "A", "clean", nil), f.shas["A"])
	cmd := exec.Command("git", "merge", "--no-ff", "-m", mergeMessage(in, "merge:w1", "B", "clean", nil), f.shas["B"])
	cmd.Dir = c.dir
	cmd.Run()
	c.write("shared.txt", "from A\nfrom B\n")
	c.write("README.md", "slipped in\n")
	c.git("add", "shared.txt", "README.md")
	h := map[string]orchtest.Handler{"merge-conflict": func(orchtest.Call, *orchtest.State) any {
		return R(M{"status": "resolved", "files": []string{"shared.txt"}, "statement": "s"})
	}}
	res, err := runMerge(t, mergeOrch(t, f, nil, h), in, true)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(res), "A=clean B=unresolvable")
	if f.mc().mergeInProgress() {
		t.Error("merge in progress")
	}
}
