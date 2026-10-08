package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// gitRepo is a temporary repository on branch integration with one commit.
type gitRepo struct {
	t   *testing.T
	dir string
}

func newGitRepo(t *testing.T) *gitRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	r := &gitRepo{t, t.TempDir()}
	r.git("init", "-q", "-b", "integration")
	r.git("config", "user.name", "t")
	r.git("config", "user.email", "t@example.invalid")
	r.git("config", "commit.gpgsign", "false")
	r.commit("README.md", "readme\n", "init")
	return r
}

func (r *gitRepo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *gitRepo) write(path, content string) {
	r.t.Helper()
	p := filepath.Join(r.dir, path)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *gitRepo) read(path string) string {
	r.t.Helper()
	b, err := os.ReadFile(filepath.Join(r.dir, path))
	if err != nil {
		r.t.Fatal(err)
	}
	return string(b)
}

// commit writes path and commits it; it returns the commit.
func (r *gitRepo) commit(path, content, msg string) string {
	r.t.Helper()
	r.write(path, content)
	r.git("add", "--", path)
	r.git("commit", "-q", "-m", msg, "--", path)
	return r.git("rev-parse", "HEAD")
}

func (r *gitRepo) head() string { return r.git("rev-parse", "HEAD") }

// hook installs an executable hook script; "" removes it.
func (r *gitRepo) hook(name, script string) {
	r.t.Helper()
	p := filepath.Join(r.dir, ".git", "hooks", name)
	if script == "" {
		os.Remove(p)
		return
	}
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		r.t.Fatal(err)
	}
}

// rewriteGit decorates a runner: fn may replace any command's output.
type rewriteGit struct {
	inner gitRunner
	mu    sync.Mutex
	fn    func(args []string, out []byte) []byte
}

func (g *rewriteGit) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	out, err := g.inner.run(ctx, dir, args...)
	g.mu.Lock()
	defer g.mu.Unlock()
	if err == nil && g.fn != nil {
		out = g.fn(args, out)
	}
	return out, err
}

// fabricateAfterCommit rewrites `rev-parse HEAD` once a commit has run, in
// the shape of P-C8's fabricated tails.
func fabricateAfterCommit() *rewriteGit {
	committed := false
	return &rewriteGit{inner: execGit{}, fn: func(args []string, out []byte) []byte {
		if args[0] == "commit" {
			committed = true
		}
		if committed && args[0] == "rev-parse" && args[len(args)-1] == "HEAD" {
			s := strings.TrimSpace(string(out))
			return []byte(s[:len(s)-3] + "fff\n")
		}
		return out
	}}
}

func runCtxFor(t *testing.T, extra M) *runCtx {
	t.Helper()
	a, err := parseArgs(orchtest.New(t, orchtest.TwoWaves(), nil, extra).Args())
	if err != nil {
		t.Fatal(err)
	}
	c, err := newRunCtx(a)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// gitOrch is an orch over r with git g (execGit when nil) and a fixed date.
func gitOrch(t *testing.T, r *gitRepo, g gitRunner) *orch {
	t.Helper()
	if g == nil {
		g = execGit{}
	}
	o := newOrch(orchtest.New(t, orchtest.TwoWaves(), nil, nil), runCtxFor(t, nil), nil)
	o.git, o.repo = g, r.dir
	o.today = func() string { return "2026-10-07" }
	return o
}

func ptr[T any](v T) *T { return &v }

func dec(id, q, d string, clears ...string) decision {
	x := decision{ID: id, Question: q, Decision: d}
	if clears != nil {
		x.Clears = &clears
	}
	return x
}

const decPath = "docs/plans/f-decisions.md"

func decIn(ds ...decision) decisionLogInput {
	return decisionLogInput{Path: decPath, Name: "f", IntegrationBranch: "integration", Decisions: ds, Subject: "docs(f): append decision(s)"}
}

// runStage runs a stage function and decodes its scribe-shaped result.
func runStage(t *testing.T, fn stageFn, label string, interrupted bool) (scribeResult, error) {
	t.Helper()
	raw, err := fn(context.Background(), label, interrupted)
	if err != nil {
		return scribeResult{}, err
	}
	r := decode[scribeResult](orchtest.New(t, orchtest.TwoWaves(), nil, nil), label, raw)
	if r == nil {
		t.Fatalf("%s: %s does not decode", label, raw)
	}
	return r.Result, nil
}

func errMatch(t *testing.T, err error, re string, msg ...any) {
	t.Helper()
	if err == nil {
		t.Errorf("%v: no error, want /%s/", msg, re)
		return
	}
	match(t, err.Error(), re, msg...)
}

// --- git.go ---

func TestVerifyCommitChecks(t *testing.T) {
	r := newGitRepo(t)
	g := gitOps{execGit{}}
	ctx := context.Background()
	pre := r.head()
	sha := r.commit("a.md", "A\n", "a")
	if err := g.verifyCommit(ctx, r.dir, "integration", sha, pre, "a.md", []byte("A\n")); err != nil {
		t.Fatalf("a good commit: %v", err)
	}
	blobOid := r.git("rev-parse", sha+":a.md")
	errMatch(t, g.verifyCommit(ctx, r.dir, "integration", blobOid, pre, "a.md", []byte("A\n")), `not a commit`)
	errMatch(t, g.verifyCommit(ctx, r.dir, "integration", sha, sha, "a.md", []byte("A\n")), `parent`)
	errMatch(t, g.verifyCommit(ctx, r.dir, "integration", sha, pre, "a.md", []byte("B\n")), `differs from the content written`)
	r.write("b.md", "B\n")
	r.write("c.md", "C\n")
	r.git("add", "b.md", "c.md")
	r.git("commit", "-q", "-m", "two")
	two := r.head()
	errMatch(t, g.verifyCommit(ctx, r.dir, "integration", two, sha, "b.md", []byte("B\n")), `changes .*want only b\.md`)
	errMatch(t, g.verifyCommit(ctx, r.dir, "integration", sha, pre, "a.md", []byte("A\n")), `not the tip of integration`)

	root := newGitRepo(t)
	root.git("checkout", "-q", "--orphan", "fresh")
	root.git("rm", "-q", "-r", "--cached", ".")
	os.Remove(filepath.Join(root.dir, "README.md"))
	first := root.commit("x.md", "X\n", "root")
	if err := g.verifyCommit(ctx, root.dir, "fresh", first, "", "x.md", []byte("X\n")); err != nil {
		t.Errorf("a root commit with pre \"\": %v", err)
	}
	errMatch(t, g.verifyCommit(ctx, root.dir, "fresh", first, pre, "x.md", []byte("X\n")), `parent`)
}

// --- load ---

const planDoc = "# Plan\n\nProse.\n\n```yaml lsf-plan\nunits:\n  - id: A\n    type: feature\n    complete: false\n    dependencies: []\n    scope: [\"src/A\"]\n    publishes: []\n    gates: []\nexecution_order: [\"A\"]\n```\n\n## After\n"

// gitRT is the stub runtime over a real repository: steps run their fn
// directly, or return a scripted value (a replay) when one is set.
type gitRT struct {
	*orchtest.Stub
	repo     string
	replayed map[string]json.RawMessage
	wtRoot   string
}

func (r gitRT) Repo() string { return r.repo }

// Worktree is a step's worktree on lsf/t/<label>, created from integration
// on first use.
func (r gitRT) Worktree(ctx context.Context, label string) (string, string, error) {
	safe := strings.NewReplacer(":", "_", "#", "_").Replace(label)
	path, branch := filepath.Join(r.wtRoot, safe), "lsf/t/"+safe
	if _, err := os.Stat(path); err == nil {
		return path, branch, nil
	}
	out, err := exec.Command("git", "-C", r.repo, "worktree", "add", "-q", "-b", branch, path, "integration").CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("worktree add: %v %s", err, out)
	}
	return path, branch, nil
}

// MergeClone is a clone of the repo at <wtRoot>/merge/<name> on
// integration, made by mergeCloneAt, or the existing one.
func (r gitRT) MergeClone(ctx context.Context, name string) (string, error) {
	dir := filepath.Join(r.wtRoot, "merge", name)
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	}
	return dir, mergeCloneAt(r.repo, dir)
}

// mergeCloneAt makes a clone of repo at dir on integration as the runtime
// makes one: every branch local, no remote, the test identity, and the
// repo's hooks (here by core.hooksPath, so a test's later hook change
// reaches it).
func mergeCloneAt(repo, dir string) error {
	for _, args := range [][]string{
		{"clone", "--no-local", "--no-checkout", "--no-tags", "-q", repo, dir},
		{"-C", dir, "fetch", "--no-tags", "--update-head-ok", "-q", repo, "+refs/heads/*:refs/heads/*"},
		{"-C", dir, "checkout", "-q", "integration"},
		{"-C", dir, "remote", "remove", "origin"},
		{"-C", dir, "config", "user.name", "t"},
		{"-C", dir, "config", "user.email", "t@example.invalid"},
		{"-C", dir, "config", "commit.gpgsign", "false"},
		{"-C", dir, "config", "core.hooksPath", filepath.Join(repo, ".git", "hooks")},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("merge clone: git %v: %v %s", args, err, out)
		}
	}
	return nil
}

func (r gitRT) RemoveMergeClone(ctx context.Context, name string) error {
	return os.RemoveAll(filepath.Join(r.wtRoot, "merge", name))
}

func (r gitRT) Step(ctx context.Context, label string, input []byte, fn func(context.Context, bool) (json.RawMessage, error)) (json.RawMessage, error) {
	if v, ok := r.replayed[label]; ok {
		return v, nil
	}
	return fn(ctx, false)
}

// loadLaunch is a launch over r whose load steps read git through g.
func loadLaunch(t *testing.T, r *gitRepo, g gitRunner, replayed map[string]json.RawMessage) *launch {
	t.Helper()
	rt := gitRT{orchtest.New(t, orchtest.TwoWaves(), nil, nil), r.dir, replayed, t.TempDir()}
	o := newOrch(rt, runCtxFor(t, M{"planPath": "docs/plans/f-breakdown.md"}), nil)
	if g != nil {
		o.git = g
	}
	return &launch{orch: o, completed: plan.Completed{}}
}

func TestLoadReadsBlobsVerbatim(t *testing.T) {
	r := newGitRepo(t)
	sha := r.commit("docs/plans/f-breakdown.md", planDoc, "plan")
	logText := "# Decisions\n\n```yaml lsf-decisions\ndecisions:\n```\n"
	logSha := r.commit(decPath, logText, "log")
	r.commit("docs/architecture/f.md", "arch\n", "arch")
	arch := r.head()
	l := loadLaunch(t, r, nil, nil)
	l.ctx.decisionLogSha = logSha
	got, err := l.loadPlan(context.Background(), sha, "load", nil)
	if err != nil || len(got.stops) != 0 || got.plan == nil {
		t.Fatalf("%+v %v", got, err)
	}
	eq(t, got.copy.Result.PlanBlock, r.git("cat-file", "blob", sha+":docs/plans/f-breakdown.md")+"\n", "LoadGraph gets the blob byte for byte")
	eq(t, got.copy.Result.PlanBlock, planDoc)
	eq(t, got.copy.Result.DecisionsBlock, logText)
	eq(t, got.copy.Result.ArchitectureSha, arch)
	eq(t, got.copy.Result.HeadBranch, "integration")
	eq(t, got.plan.Units[0].ID, "A")
}

func TestLoadFaultsStop(t *testing.T) {
	r := newGitRepo(t)
	sha := r.commit("docs/plans/f-breakdown.md", planDoc, "plan")
	for name, rewrite := range map[string]func(string) string{
		"placeholder": func(string) string { return "# Plan\n\n[plan block omitted for brevity]\n" },
		"truncation":  func(s string) string { return s[:strings.Index(s, "execution_order")] },
	} {
		g := &rewriteGit{inner: execGit{}, fn: func(args []string, out []byte) []byte {
			if args[0] == "cat-file" && args[1] == "blob" {
				return []byte(rewrite(string(out)))
			}
			return out
		}}
		got, err := loadLaunch(t, r, g, nil).loadPlan(context.Background(), sha, "load", nil)
		if err != nil || got.plan != nil || len(got.stops) != 1 || got.stops[0].Kind != "invalid-plan" {
			t.Errorf("%s: %+v %v", name, got, err)
		}
	}
	got, err := loadLaunch(t, r, nil, nil).loadPlan(context.Background(), r.git("rev-parse", "HEAD~1"), "load:w1", plan.W(1))
	if err != nil || got.copy != nil || len(got.stops) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	st := got.stops[0]
	eq(t, st.Kind, "stage-fault")
	match(t, st.Statement, `^load failed: .*docs/plans/f-breakdown\.md does not exist`)
}

// A replayed load reports HEAD's branch and the pause ref as they stand now.
func TestLoadLiveReadsOverrideReplay(t *testing.T) {
	r := newGitRepo(t)
	sha := r.commit("docs/plans/f-breakdown.md", planDoc, "plan")
	stale, _ := stageValue(copyResult{PlanBlock: planDoc, HeadBranch: "other", PauseRequested: true, ArchitectureSha: "a"})
	got, err := loadLaunch(t, r, nil, map[string]json.RawMessage{"load": stale}).loadPlan(context.Background(), sha, "load", nil)
	if err != nil || got.copy == nil {
		t.Fatal(got, err)
	}
	eq(t, got.copy.Result.HeadBranch, "integration")
	eq(t, got.copy.Result.PauseRequested, false)
	r.git("update-ref", "refs/lsf/f/pause", "HEAD")
	r.git("checkout", "-q", "-b", "side")
	got, _ = loadLaunch(t, r, nil, map[string]json.RawMessage{"load": stale}).loadPlan(context.Background(), sha, "load", nil)
	eq(t, got.copy.Result.HeadBranch, "side")
	eq(t, got.copy.Result.PauseRequested, true)
}

// --- decision log ---

func TestDecisionLogAppendsAndDedups(t *testing.T) {
	r := newGitRepo(t)
	o := gitOrch(t, r, nil)
	res, err := runStage(t, o.decisionLog(decIn(dec("d1", "q: one", `say "yes"`, "A/legal"))), "decision-log", false)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Decisions: f\n\n```yaml lsf-decisions\ndecisions:\n  - id: \"d1\"\n    date: 2026-10-07\n    question: \"q: one\"\n    decision: \"say \\\"yes\\\"\"\n    clears: [\"A/legal\"]\n```\n"
	eq(t, r.read(decPath), want)
	eq(t, res.Sha, r.head())
	eq(t, res.Path, decPath)
	eq(t, len(res.Sha), 40)
	match(t, r.git("log", "-1", "--format=%B"), `(?s)^docs\(f\): append decision\(s\)\n\nLsf-Step: decision-log\nLsf-Content: sha256:[0-9a-f]{64}\n?$`)
	eq(t, r.git("status", "--porcelain"), "")

	// d1 again with d2: only d2 is appended.
	first := res.Sha
	res, err = runStage(t, o.decisionLog(decIn(dec("d1", "q: one", `say "yes"`, "A/legal"), dec("d2", "q2", "no"))), "decision-log", false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, strings.Count(r.read(decPath), `id: "d1"`), 1)
	eq(t, r.git("rev-parse", res.Sha+"^"), first)
	// Nothing new: the last commit touching the log stands for the append.
	r.commit("other.md", "x\n", "unrelated")
	again, err := runStage(t, o.decisionLog(decIn(dec("d2", "q2", "no"))), "decision-log", false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, again.Sha, res.Sha)
	// An id already used with other content is refused, and nothing written.
	tip := r.head()
	_, err = runStage(t, o.decisionLog(decIn(dec("d2", "q2", "yes"))), "decision-log", false)
	errMatch(t, err, `does not parse back|duplicate`)
	eq(t, r.head(), tip)
	eq(t, r.git("status", "--porcelain"), "")
}

// sedHook is a pre-commit hook rewriting path with a sed script and adding
// it: the hook's change reaches the commit (probed on git 2.54).
func sedHook(path, script string) string {
	return "sed -i '" + script + "' " + path + " && git add " + path
}

func TestDecisionLogContentFaultsStop(t *testing.T) {
	faults := map[string]string{
		"invented clears": `s/    decision: "yes"/    decision: "yes"\n    clears: ["A\/legal"]/`,
		"dropped fence":   `$d`,
		"dropped line":    `/question: "q2"/d`,
		"shortened text":  `s/"a long answer"/"a long"/`,
	}
	for name, script := range faults {
		t.Run(name, func(t *testing.T) {
			r := newGitRepo(t)
			o := gitOrch(t, r, nil)
			if _, err := runStage(t, o.decisionLog(decIn(dec("d1", "q", "yes"))), "decision-log", false); err != nil {
				t.Fatal(err)
			}
			r.hook("pre-commit", sedHook(decPath, script))
			in := decIn(dec("d2", "q2", "a long answer"))
			_, err := runStage(t, o.decisionLog(in), "decision-log#2", false)
			bad := r.head()
			errMatch(t, err, `^commit `+bad+` fails verification`)
			eq(t, r.git("status", "--porcelain"), "", "the path's index is left as HEAD has it")
			r.hook("pre-commit", "")

			// A relaunch, with the same decisions or others, stops on the guard.
			for _, again := range []decisionLogInput{in, decIn(dec("d3", "q3", "z"))} {
				_, err := runStage(t, o.decisionLog(again), "decision-log", false)
				errMatch(t, err, `stage commit `+bad+` left `+decPath+` with content .* new commit that has no Lsf-Content trailer`)
				eq(t, r.head(), bad)
			}
			// An amend keeps the trailer and stops again.
			r.write(decPath, r.read(decPath)+"<!-- amended -->\n")
			r.git("commit", "-q", "--amend", "--no-edit", "--", decPath)
			_, err = runStage(t, o.decisionLog(in), "decision-log", false)
			errMatch(t, err, `^stage commit `)
			// A repair commit without the trailer: the next launch proceeds.
			r.git("reset", "-q", "--hard", bad+"~1")
			r.write(decPath, r.read(decPath)+"\n<!-- repaired -->\n")
			r.git("commit", "-q", "-am", "repair")
			repair := r.head()
			res, err := runStage(t, o.decisionLog(in), "decision-log", false)
			if err != nil {
				t.Fatal(err)
			}
			eq(t, r.git("rev-parse", res.Sha+"^"), repair)
			// A repair that already holds the decisions is returned.
			again, err := runStage(t, o.decisionLog(in), "decision-log", false)
			if err != nil || again.Sha != res.Sha {
				t.Errorf("%+v %v", again, err)
			}
		})
	}
}

func TestDecisionLogRejectedCommitRestores(t *testing.T) {
	r := newGitRepo(t)
	o := gitOrch(t, r, nil)
	r.hook("pre-commit", "exit 1")
	tip := r.head()
	_, err := runStage(t, o.decisionLog(decIn(dec("d1", "q", "yes"))), "decision-log", false)
	errMatch(t, err, `^commit of `+decPath+` rejected`)
	eq(t, r.head(), tip)
	eq(t, r.git("status", "--porcelain", "--", decPath), "")
	if _, err := os.Stat(filepath.Join(r.dir, decPath)); !os.IsNotExist(err) {
		t.Errorf("the new file was not removed: %v", err)
	}
	r.hook("pre-commit", "")
	if _, err := runStage(t, o.decisionLog(decIn(dec("d1", "q", "yes"))), "decision-log", false); err != nil {
		t.Fatal(err)
	}
	prior := r.read(decPath)
	r.hook("pre-commit", "exit 1")
	tip = r.head()
	_, err = runStage(t, o.decisionLog(decIn(dec("d2", "q", "yes"))), "decision-log#2", false)
	errMatch(t, err, `rejected`)
	eq(t, r.head(), tip)
	eq(t, r.read(decPath), prior)
	eq(t, r.git("status", "--porcelain", "--", decPath), "")
}

func TestDecisionLogFabricatedShaRejected(t *testing.T) {
	r := newGitRepo(t)
	o := gitOrch(t, r, fabricateAfterCommit())
	_, err := runStage(t, o.decisionLog(decIn(dec("d1", "q", "yes"))), "decision-log", false)
	errMatch(t, err, `fff fails verification: not a commit`)
}

func TestDecisionLogMainCheckoutSafety(t *testing.T) {
	r := newGitRepo(t)
	o := gitOrch(t, r, nil)
	r.git("checkout", "-q", "-b", "side")
	_, err := runStage(t, o.decisionLog(decIn(dec("d1", "q", "yes"))), "decision-log", false)
	errMatch(t, err, `the main checkout is on side, not integration`)
	r.git("checkout", "-q", "integration")
	r.write(decPath, "requester draft\n")
	_, err = runStage(t, o.decisionLog(decIn(dec("d1", "q", "yes"))), "decision-log", false)
	errMatch(t, err, `has uncommitted changes this stage did not make`)
	eq(t, r.read(decPath), "requester draft\n")
	os.Remove(filepath.Join(r.dir, decPath))

	// Another staged change and an unstaged one survive the commit.
	r.commit("notes.md", "n\n", "notes")
	r.write("README.md", "staged\n")
	r.git("add", "README.md")
	r.write("notes.md", "unstaged\n")
	res, err := runStage(t, o.decisionLog(decIn(dec("d1", "q", "yes"))), "decision-log", false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, r.git("diff-tree", "--no-commit-id", "--name-only", "-r", res.Sha), decPath)
	eq(t, r.git("diff", "--cached", "--name-only"), "README.md")
	eq(t, r.git("diff", "--name-only"), "notes.md")
	eq(t, r.read("README.md"), "staged\n")
	eq(t, r.read("notes.md"), "unstaged\n")
}

// An interrupted rerun after a good commit adopts it: no second commit,
// after midnight too, and after a requester commit on top.
func TestDecisionLogInterruptedAdopts(t *testing.T) {
	r := newGitRepo(t)
	o := gitOrch(t, r, nil)
	in := decIn(dec("d1", "q", "yes"))
	first, err := runStage(t, o.decisionLog(in), "decision-log", false)
	if err != nil {
		t.Fatal(err)
	}
	o.today = func() string { return "2026-10-08" }
	again, err := runStage(t, o.decisionLog(in), "decision-log", true)
	if err != nil || again.Sha != first.Sha || r.head() != first.Sha {
		t.Fatalf("%+v %v", again, err)
	}
	r.commit(decPath, r.read(decPath)+"\nA note.\n", "requester note")
	again, err = runStage(t, o.decisionLog(in), "decision-log", true)
	if err != nil || again.Sha != first.Sha {
		t.Errorf("%+v %v", again, err)
	}
	// Another step's commit (other label) is not adopted.
	in2 := decIn(dec("d2", "q", "no"))
	res, err := runStage(t, o.decisionLog(in2), "decision-log#2", true)
	if err != nil || res.Sha == first.Sha || res.Sha != r.head() {
		t.Errorf("%+v %v", res, err)
	}
	eq(t, strings.Count(r.read(decPath), `id: "d1"`), 1)
}

// --- ledger ---

const ledgerPath = "docs/plans/f-signals/plan0.md"

func ledgerBlock(wave int, statements ...string) string {
	b := fmt.Sprintf("## Wave %d\n\n| Wave | Unit | Stage | Source | Kind | Statement | Referent | Disposition |\n| --- | --- | --- | --- | --- | --- | --- | --- |\n", wave)
	for _, st := range statements {
		b += fmt.Sprintf("| %d | A | implement | reported | deviation | %s | A | recorded |\n", wave, st)
	}
	return b
}

func ledgerIn(block string) ledgerInput {
	return ledgerInput{Path: ledgerPath, Name: "f", IntegrationBranch: "integration", Block: block, Subject: "docs(f): append signal ledger entries", PauseRef: "refs/lsf/f/pause"}
}

func TestLedgerAppends(t *testing.T) {
	r := newGitRepo(t)
	o := gitOrch(t, r, nil)
	b1, b2 := ledgerBlock(1, "one"), ledgerBlock(2, "two")
	res, err := runStage(t, o.ledgerAppend(ledgerIn(b1)), "ledger:w1", false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, r.read(ledgerPath), plan.LedgerHeading("f")+"\n"+b1)
	eq(t, res.Sha, r.head())
	eq(t, res.PauseRequested, false)
	r.git("update-ref", "refs/lsf/f/pause", "HEAD")
	res, err = runStage(t, o.ledgerAppend(ledgerIn(b2)), "ledger:w2", false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, r.read(ledgerPath), plan.LedgerHeading("f")+"\n"+b1+"\n"+b2)
	eq(t, res.PauseRequested, true)
	match(t, r.git("log", "-1", "--format=%B"), `Lsf-Step: ledger:w2\nLsf-Content: sha256:`)
}

func TestLedgerContentFaultsStop(t *testing.T) {
	faults := map[string]string{
		"overwritten earlier row": `s/| one |/| uno |/`,
		"deleted earlier row":     `/| one |/d`,
		"own rows altered":        `s/| two |/| deux |/`,
		"heading dropped":         `1d`,
		"deleted wave section":    `/^## Wave 1$/,/^| 1 |/d`,
	}
	for name, script := range faults {
		t.Run(name, func(t *testing.T) {
			r := newGitRepo(t)
			o := gitOrch(t, r, nil)
			if _, err := runStage(t, o.ledgerAppend(ledgerIn(ledgerBlock(1, "one"))), "ledger:w1", false); err != nil {
				t.Fatal(err)
			}
			r.hook("pre-commit", sedHook(ledgerPath, script))
			_, err := runStage(t, o.ledgerAppend(ledgerIn(ledgerBlock(2, "two"))), "ledger:w2", false)
			bad := r.head()
			errMatch(t, err, `^commit `+bad+` fails verification`)
			r.hook("pre-commit", "")
			for label, block := range map[string]string{"ledger:w2": ledgerBlock(2, "two"), "ledger:w3": ledgerBlock(3, "three")} {
				_, err := runStage(t, o.ledgerAppend(ledgerIn(block)), label, false)
				errMatch(t, err, `^stage commit `+bad+` left `)
				eq(t, r.head(), bad, label)
			}
			r.git("reset", "-q", "--hard", bad+"~1")
			r.commit(ledgerPath, r.read(ledgerPath)+"\n"+ledgerBlock(2, "two"), "repair")
			if _, err := runStage(t, o.ledgerAppend(ledgerIn(ledgerBlock(3, "three"))), "ledger:w3", false); err != nil {
				t.Errorf("after a repair: %v", err)
			}
		})
	}
}

func TestLedgerRefusals(t *testing.T) {
	r := newGitRepo(t)
	o := gitOrch(t, r, nil)
	r.hook("pre-commit", "exit 1")
	tip := r.head()
	_, err := runStage(t, o.ledgerAppend(ledgerIn(ledgerBlock(1, "one"))), "ledger:w1", false)
	errMatch(t, err, `rejected`)
	eq(t, r.head(), tip)
	eq(t, r.git("status", "--porcelain"), "")
	r.hook("pre-commit", "")

	_, err = runStage(t, gitOrch(t, r, fabricateAfterCommit()).ledgerAppend(ledgerIn(ledgerBlock(1, "one"))), "ledger:w1", false)
	errMatch(t, err, `fff fails verification`)

	r.git("checkout", "-q", "-b", "side")
	_, err = runStage(t, o.ledgerAppend(ledgerIn(ledgerBlock(2, "two"))), "ledger:w2", false)
	errMatch(t, err, `on side, not integration`)
	r.git("checkout", "-q", "integration")
	r.write(ledgerPath, r.read(ledgerPath)+"requester edit\n")
	_, err = runStage(t, o.ledgerAppend(ledgerIn(ledgerBlock(2, "two"))), "ledger:w2", false)
	errMatch(t, err, `uncommitted changes this stage did not make`)
	match(t, r.read(ledgerPath), `requester edit\n$`)
}

// onRuntime runs body as a launch of the real runtime over r, with state
// in stateDir.
func onRuntime(t *testing.T, r *gitRepo, stateDir string, body func(ctx context.Context, o *orch)) {
	t.Helper()
	cfg := lsfrt.Config{StateDir: stateDir, Repo: r.dir, Args: json.RawMessage(`{"planSha":"0123456789ab","integrationBranch":"integration"}`),
		Roles: map[string]lsfrt.Role{"worker": {Tools: "default", PermissionMode: "dontAsk", Clone: "kept"}}, DefaultRole: "worker"}
	if _, err := lsfrt.Run(context.Background(), cfg, func(ctx context.Context, rt *lsfrt.Runtime) (json.RawMessage, error) {
		o := newOrch(rt, runCtxFor(t, nil), nil)
		o.today = func() string { return "2026-10-07" }
		body(ctx, o)
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
}

// killed is fn, then a kill before the step's record: the commit lands,
// the intent stays.
func killed(fn stageFn) stageFn {
	return func(ctx context.Context, label string, interrupted bool) (json.RawMessage, error) {
		if _, err := fn(ctx, label, interrupted); err != nil {
			return nil, err
		}
		return nil, errors.New("killed after the commit")
	}
}

// S5: a step killed between its commit and its record reruns interrupted
// and adopts its commit; nothing else is adopted.
func TestLedgerInterruptedAdopts(t *testing.T) {
	r := newGitRepo(t)
	state := t.TempDir()
	b1, b2a, b2b := ledgerBlock(1, "one"), ledgerBlock(2, "two-a"), ledgerBlock(2, "two-b")
	var commit string
	onRuntime(t, r, state, func(ctx context.Context, o *orch) {
		if _, f, err := step[scribeResult](ctx, o, "ledger:w1", ledgerIn(b1), o.ledgerAppend(ledgerIn(b1))); f != "" || err != nil {
			t.Fatal(f, err)
		}
		if _, f, err := step[scribeResult](ctx, o, "ledger:w2", ledgerIn(b2a), o.ledgerAppend(ledgerIn(b2a))); f != "" || err != nil {
			t.Fatal(f, err)
		}
		_, f, err := step[scribeResult](ctx, o, "ledger:w2", ledgerIn(b2b), killed(o.ledgerAppend(ledgerIn(b2b))))
		if err != nil || f != "killed after the commit" {
			t.Fatal(f, err)
		}
		commit = r.head()
	})
	// A requester commit to the ledger after the stage's own.
	r.commit(ledgerPath, r.read(ledgerPath)+"\nA requester note.\n", "note")
	onRuntime(t, r, state, func(ctx context.Context, o *orch) {
		for _, b := range []string{b1, b2a} { // replayed
			if _, f, err := step[scribeResult](ctx, o, map[string]string{b1: "ledger:w1", b2a: "ledger:w2"}[b], ledgerIn(b), nil); f != "" || err != nil {
				t.Fatal(f, err)
			}
		}
		got, f, err := step[scribeResult](ctx, o, "ledger:w2", ledgerIn(b2b), o.ledgerAppend(ledgerIn(b2b)))
		if err != nil || f != "" || got.Result.Sha != commit {
			t.Fatalf("%+v %q %v: want the adopted %s", got, f, err, commit)
		}
	})
	eq(t, strings.Count(r.read(ledgerPath), "two-b"), 1, "no second append")
	evs := events(t, state)
	eq(t, evs["step-interrupted-rerun"], 1)
	eq(t, evs["step-replayed"], 2)

	// An interrupted rerun whose own commit never landed appends: an
	// earlier launch's intact commit with the same label and other rows is
	// not adopted, and is no stop.
	o := gitOrch(t, r, nil)
	b3 := ledgerBlock(1, "a later launch's wave 1")
	res, err := runStage(t, o.ledgerAppend(ledgerIn(b3)), "ledger:w1", true)
	if err != nil || res.Sha != r.head() || res.Sha == commit {
		t.Fatalf("%+v %v", res, err)
	}
	match(t, r.read(ledgerPath), `(?s)one.*two-a.*two-b.*requester note.*a later launch`)
}

// A replayed ledger step reports the pause ref as it stands now.
func TestLedgerLivePause(t *testing.T) {
	r := newGitRepo(t)
	l := loadLaunch(t, r, nil, nil)
	l.scribed = map[*plan.Signal]bool{}
	stale, _ := stageValue(scribeResult{Path: ledgerPath, Sha: r.head(), PauseRequested: false})
	l.rt = gitRT{l.rt.(gitRT).Stub, r.dir, map[string]json.RawMessage{"ledger:w1": stale}, t.TempDir()}
	l.wave = 1
	l.ledger = []*plan.Signal{plan.NewSignal(plan.Signal{Kind: "deviation", Source: "reported", Stage: "implement", Unit: "A", Wave: plan.W(1), Statement: "d", Referent: "A"})}
	r.git("update-ref", "refs/lsf/f/pause", "HEAD")
	if err := l.scribeLedger(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if l.ledgerFault != nil || !l.pauseRequested {
		t.Errorf("fault %+v pause %v", l.ledgerFault, l.pauseRequested)
	}
}

// --- completion report ---

const reportPath = "docs/plans/A-completion.md"

func reportLaunch(t *testing.T, r *gitRepo, g gitRunner) *orch {
	t.Helper()
	l := loadLaunch(t, r, g, nil)
	l.today = func() string { return "2026-10-07" }
	return l.orch
}

func reportIn(text string) completionInput {
	return completionInput{Unit: "A", Base: "unit-A", Path: reportPath, Text: text, Subject: "docs(f): A completion report"}
}

func TestCompletionReport(t *testing.T) {
	r := newGitRepo(t)
	r.git("branch", "unit-A")
	r.git("checkout", "-q", "unit-A")
	r.commit("src/A.go", "package a\n", "unit work")
	base := r.head()
	r.git("checkout", "-q", "integration")
	o := reportLaunch(t, r, nil)
	text := "# A — completion report\n\nDone.\n\nGates: green.\n"
	raw, err := o.completionReport(reportIn(text))(context.Background(), "completion-report:A", false)
	if err != nil {
		t.Fatal(err)
	}
	var v struct{ Result completionResult }
	json.Unmarshal(raw, &v)
	rep := v.Result
	eq(t, rep.Branch, "lsf/t/completion-report_A")
	eq(t, rep.Path, reportPath)
	eq(t, r.git("rev-parse", rep.Branch), rep.Sha)
	eq(t, r.git("rev-parse", rep.Sha+"^"), base)
	eq(t, r.git("show", rep.Sha+":"+reportPath)+"\n", text)
	eq(t, r.git("status", "--porcelain"), "", "the main checkout is untouched")

	// A rerun reuses the matching tip.
	raw, err = o.completionReport(reportIn(text))(context.Background(), "completion-report:A", true)
	json.Unmarshal(raw, &v)
	if err != nil || v.Result.Sha != rep.Sha {
		t.Fatalf("%+v %v", v.Result, err)
	}

	// Content faults through a hook stop, naming the commit.
	for name, script := range map[string]string{"dropped line": `/Gates: green./d`, "altered phrase": `s/Done./Mostly done./`} {
		r.hook("pre-commit", sedHook(reportPath, script))
		text := text + name + "\n"
		_, err := o.completionReport(reportIn(text))(context.Background(), "completion-report:A", false)
		bad := r.git("rev-parse", rep.Branch)
		errMatch(t, err, `^commit `+bad+` fails verification`, name)
		r.hook("pre-commit", "")
		// The bad tip is reset away, never built on.
		raw, err = o.completionReport(reportIn(text))(context.Background(), "completion-report:A", false)
		json.Unmarshal(raw, &v)
		if err != nil || v.Result.Sha == bad || r.git("rev-parse", v.Result.Sha+"^") != base {
			t.Errorf("%s: %+v %v", name, v.Result, err)
		}
	}

	// A stale tip (the unit branch moved) is reset and recommitted.
	r.git("checkout", "-q", "unit-A")
	newBase := r.commit("src/A.go", "package a // fixed\n", "fix")
	r.git("checkout", "-q", "integration")
	raw, err = o.completionReport(reportIn(text))(context.Background(), "completion-report:A", false)
	json.Unmarshal(raw, &v)
	if err != nil || r.git("rev-parse", v.Result.Sha+"^") != newBase {
		t.Errorf("%+v %v", v.Result, err)
	}

	r.hook("pre-commit", "exit 1")
	_, err = o.completionReport(reportIn(text+"x\n"))(context.Background(), "completion-report:A", false)
	errMatch(t, err, `rejected`)
	wt := filepath.Join(o.rt.(gitRT).wtRoot, "completion-report_A")
	eq(t, r.git("-C", wt, "status", "--porcelain"), "")
	eq(t, r.git("rev-parse", rep.Branch), newBase, "no commit on the report branch")
	r.hook("pre-commit", "")

	o.git = fabricateAfterCommit()
	_, err = o.completionReport(reportIn(text+"y\n"))(context.Background(), "completion-report:A", false)
	errMatch(t, err, `fff fails verification`)
}

// --- through Run ---

// A code stage's failure is a stop naming the reason: the ledger's ends the
// run at the ledger, the completion report's aborts its unit.
func TestCodeStageFaultsStop(t *testing.T) {
	boom := func(Call, *State) any { return errors.New("git: boom") }
	out := run(t, stub(t, twoWaves(), Hs{"ledger": boom, "implement": implemented(M{"deviations": []string{"d"}})}, nil))
	eq(t, out.Status, "stopped")
	eq(t, out.Escalation.Stage, "ledger")
	st := out.stops()
	if len(st) != 1 || st[0].Kind != "stage-fault" || st[0].Statement != "ledger failed: git: boom" || st[0].Referent != "docs/plans/f-signals/plan0.md" {
		t.Errorf("%+v", st)
	}

	s := stub(t, one("A", "feature"), Hs{"completion-report": boom}, nil)
	out = run(t, s)
	eq(t, out.Status, "stopped")
	found := false
	for _, sig := range out.stops() {
		if sig.Stage == "completion-report" && sig.Unit == "A" && sig.Statement == "completion-report failed: git: boom" {
			found = true
		}
	}
	if !found || hasPrefix(s.Labels(), "merge:") {
		t.Errorf("%+v %v", out.stops(), s.Labels())
	}

	d := run(t, stub(t, twoWaves(), Hs{"decision-log": boom}, M{"decisions": []M{{"id": "d1", "question": "q", "decision": "yes"}}}))
	eq(t, d.Escalation.Stage, "decision-log")
	eq(t, d.stops()[0].Statement, "decision-log failed: git: boom")
	ld := run(t, stub(t, twoWaves(), Hs{"load": boom}, nil))
	eq(t, ld.Escalation.Stage, "load")
	eq(t, ld.stops()[0].Statement, "load failed: git: boom")
}

// S1: a whole launch makes no agent call for the four code stages; each
// runs as a step.
func TestCodeStagesMakeNoAgentCall(t *testing.T) {
	carried := []M{{"kind": "deviation", "source": "reported", "stage": "implement", "unit": "Z", "wave": 3, "statement": "dev-Z", "referent": "src/Z", "raisedBy": "implement", "disposition": "recorded"}}
	s := stub(t, twoWaves(), merged(discovering(), Hs{"load": loadWith(M{"decisionsBlock": decisionsBlock("2026-09-12", "yes")})}), M{"decisions": []M{{"id": "d1", "question": "q", "decision": "yes"}}, "unscribed": carried})
	out := run(t, s)
	eq(t, out.Status, "complete", out.stopKinds())
	code := []string{"load", "decision-log", "ledger", "completion-report"}
	for _, c := range agentCalls(s) {
		if slices.Contains(code, c.Prefix) {
			t.Errorf("agent call %s", c.Label)
		}
	}
	steps := map[string]bool{}
	for _, c := range s.Calls {
		if c.Step {
			steps[c.Prefix] = true
		}
	}
	for _, p := range code {
		if !steps[p] {
			t.Errorf("no %s step", p)
		}
	}
}

// A hook that stages another path is caught: the commit changes more than
// the stage's path. An untracked draft at a new path is refused whatever
// the status config. A reverted effect is not adopted.
func TestMainCheckoutEdgeCases(t *testing.T) {
	r := newGitRepo(t)
	o := gitOrch(t, r, nil)
	r.write("wip.md", "requester work\n")
	r.hook("pre-commit", "git add wip.md")
	_, err := runStage(t, o.ledgerAppend(ledgerIn(ledgerBlock(1, "one"))), "ledger:w1", false)
	errMatch(t, err, `fails verification: changes .*want only `+ledgerPath)
	r.hook("pre-commit", "")

	r2 := newGitRepo(t)
	r2.git("config", "status.showUntrackedFiles", "no")
	r2.write(decPath, "requester draft\n")
	_, err = runStage(t, gitOrch(t, r2, nil).decisionLog(decIn(dec("d1", "q", "yes"))), "decision-log", false)
	errMatch(t, err, `uncommitted changes this stage did not make`)
	eq(t, r2.read(decPath), "requester draft\n")

	r3 := newGitRepo(t)
	o3 := gitOrch(t, r3, nil)
	b := ledgerBlock(1, "one")
	first, err := runStage(t, o3.ledgerAppend(ledgerIn(b)), "ledger:w1", false)
	if err != nil {
		t.Fatal(err)
	}
	r3.git("revert", "--no-edit", first.Sha)
	again, err := runStage(t, o3.ledgerAppend(ledgerIn(b)), "ledger:w1", true)
	if err != nil || again.Sha == first.Sha {
		t.Errorf("a reverted append was adopted: %+v %v", again, err)
	}
}

// An interrupted ledger rerun neither adopts a commit that changed another
// path nor appends its rows a second time.
func TestLedgerRerunAfterFailedVerification(t *testing.T) {
	r := newGitRepo(t)
	o := gitOrch(t, r, nil)
	r.write("wip.md", "requester work\n")
	r.hook("pre-commit", "git add wip.md")
	b := ledgerBlock(1, "one")
	_, err := runStage(t, o.ledgerAppend(ledgerIn(b)), "ledger:w1", false)
	errMatch(t, err, `want only `+ledgerPath)
	r.hook("pre-commit", "")
	bad := r.head()
	_, err = runStage(t, o.ledgerAppend(ledgerIn(b)), "ledger:w1", true)
	errMatch(t, err, `already holds these rows, but not in an intact commit of ledger:w1`)
	eq(t, r.head(), bad)
	eq(t, strings.Count(r.read(ledgerPath), "| one |"), 1)
}

// Without an intent (a lost state dir), rows already at the tip are not
// appended again; content already committed is not committed again; a
// unit branch that moved since the step's input was taken is refused.
func TestRefusalsWithoutIntent(t *testing.T) {
	r := newGitRepo(t)
	o := gitOrch(t, r, nil)
	b := ledgerBlock(1, "one")
	if _, err := runStage(t, o.ledgerAppend(ledgerIn(b)), "ledger:w1", false); err != nil {
		t.Fatal(err)
	}
	tip := r.head()
	_, err := runStage(t, o.ledgerAppend(ledgerIn(b)), "ledger:w1", false)
	errMatch(t, err, `already holds these rows`)
	eq(t, r.head(), tip)

	_, err = gitOps{execGit{}}.writeAndCommit(context.Background(), r.dir, "integration", "README.md", "s", "l", []byte("readme\n"), tip)
	errMatch(t, err, `README\.md already holds this content at `+tip+`; nothing to commit`)

	r.git("branch", "unit-A")
	in := reportIn("# A — completion report\n")
	in.BaseSha = r.head()
	r.git("checkout", "-q", "unit-A")
	r.commit("src/A.go", "package a\n", "more work")
	r.git("checkout", "-q", "integration")
	_, err = reportLaunch(t, r, nil).completionReport(in)(context.Background(), "completion-report:A", false)
	errMatch(t, err, `unit branch unit-A moved from `)
}
