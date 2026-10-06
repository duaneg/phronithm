package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
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
}

func (r gitRT) Repo() string { return r.repo }

func (r gitRT) Step(ctx context.Context, label string, input []byte, fn func(context.Context, bool) (json.RawMessage, error)) (json.RawMessage, error) {
	if v, ok := r.replayed[label]; ok {
		return v, nil
	}
	return fn(ctx, false)
}

// loadLaunch is a launch over r whose load steps read git through g.
func loadLaunch(t *testing.T, r *gitRepo, g gitRunner, replayed map[string]json.RawMessage) *launch {
	t.Helper()
	rt := gitRT{orchtest.New(t, orchtest.TwoWaves(), nil, nil), r.dir, replayed}
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
	eq(t, st.Kind, "agent-null")
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
