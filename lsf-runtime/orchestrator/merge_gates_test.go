package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// journal is a step journal shared by the launches of one test: a step
// with a record of the same label and input replays it; a step whose fn
// fails, or whose goroutine is killed, records nothing.
type journal struct {
	mu   sync.Mutex
	recs map[string]journalRec
}

type journalRec struct {
	hash [32]byte
	ret  json.RawMessage
}

// journalRT is gitRT with steps journaled in j.
type journalRT struct {
	gitRT
	j *journal
}

func (r journalRT) Step(ctx context.Context, label string, input []byte, fn func(context.Context, bool) (json.RawMessage, error)) (json.RawMessage, error) {
	h := sha256.Sum256(input)
	r.j.mu.Lock()
	rec, ok := r.j.recs[label]
	r.j.mu.Unlock()
	if ok && rec.hash == h {
		return rec.ret, nil
	}
	ret, err := fn(ctx, false)
	if err == nil {
		r.j.mu.Lock()
		r.j.recs[label] = journalRec{h, ret}
		r.j.mu.Unlock()
	}
	return ret, err
}

// gateFixture is a merge fixture over a declared plan whose gates fail on
// any tree holding a file named bad (gate "tests") or slow (gate "lint",
// not run), with every RunGates call counted.
type gateFixture struct {
	*mergeFixture
	j     *journal
	runs  []string // the candidate commits gated, in order
	mu    sync.Mutex
	kill  func(g orchtest.GateCall) bool // kills the launch inside RunGates
	moved func()                         // runs inside RunGates
}

func newGateFixture(t *testing.T, files map[string]map[string]string, ids ...string) *gateFixture {
	return &gateFixture{mergeFixture: newMergeFixture(t, files, ids...), j: &journal{recs: map[string]journalRec{}}}
}

func (f *gateFixture) has(commit, name string) bool {
	return strings.Contains("\n"+f.r.git("ls-tree", "-r", "--name-only", commit)+"\n", "/"+name+"\n")
}

// orch is a fresh launch's orch: fresh memo, the shared journal.
func (f *gateFixture) orch(t *testing.T, g gitRunner, h map[string]orchtest.Handler) *orch {
	t.Helper()
	s := orchtest.New(t, orchtest.TwoWaves(), h, nil)
	s.Gates = func(gc orchtest.GateCall) ([]lsfrt.GateRun, error) {
		f.mu.Lock()
		f.runs = append(f.runs, gc.Commit)
		f.mu.Unlock()
		if f.moved != nil {
			f.moved()
		}
		if f.kill != nil && f.kill(gc) {
			runtime.Goexit()
		}
		return orchtest.GateRuns(gc.Gates, func(spec lsfrt.GateSpec) string {
			switch {
			case spec.Name == "tests" && f.has(gc.Commit, "bad"):
				return lsfrt.GateRed
			case spec.Name == "lint" && f.has(gc.Commit, "slow"):
				return lsfrt.GateNotRun
			}
			return lsfrt.GateGreen
		}, gc.Commit[:7]), nil
	}
	o := newOrch(journalRT{gitRT{s, f.r.dir, nil, f.wt}, f.j}, runCtxFor(t, nil), nil)
	if g != nil {
		o.git = g
	}
	o.today = func() string { return "2026-10-07" }
	o.decl = mustDecl(t, "gates:\n  - name: tests\n    command: make test\n  - name: lint\n    command: make lint\n")
	return o
}

func (f *gateFixture) declared(ids ...string) mergeInput {
	in := f.in(ids...)
	in.Declared = true
	return in
}

func (f *gateFixture) gated() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.runs)
}

// mergeKilled runs the merge step on a goroutine that a kill ends: it reports
// whether the step returned.
func mergeKilled(t *testing.T, o *orch, in mergeInput) (returned bool) {
	t.Helper()
	done := make(chan bool)
	go func() {
		defer func() { done <- returned }()
		o.mergeStage(in)(context.Background(), "merge:w1", false)
		returned = true
	}()
	return <-done
}

// gitKill is a gitRunner whose launch is killed at the first command match
// accepts, before it runs.
type gitKill struct{ match func([]string) bool }

func (g gitKill) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if g.match(args) {
		runtime.Goexit()
	}
	return execGit{}.run(ctx, dir, args...)
}

var resolveShared = map[string]map[string]string{"A": {"shared.txt": "from A\n"}, "B": {"shared.txt": "from B\n"}}

func resolveBoth(f *gateFixture) map[string]orchtest.Handler {
	return map[string]orchtest.Handler{"merge-conflict": func(orchtest.Call, *orchtest.State) any {
		f.mc().write("shared.txt", "from A\nfrom B\n")
		f.mc().git("add", "shared.txt")
		return R(M{"status": "resolved", "files": []string{"shared.txt"}, "statement": "both"})
	}}
}

// S3 (merge part), S6: green merges commit; a red clean merge commits
// nothing and is regressed, and the next unit still merges.
func TestMergeGatesClean(t *testing.T) {
	f := newGateFixture(t, map[string]map[string]string{"A": {"src/a.go": "a\n", "src/bad": "x\n"}, "B": {"src/b.go": "b\n"}}, "A", "B")
	pre := f.r.head()
	res, err := runMerge(t, f.orch(t, nil, nil), f.declared("A", "B"), false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(res), "A=regressed B=clean")
	eqs(t, res.Merges[0].Files, []string{"`tests`: red — red at " + f.runs[0][:7]})
	eq(t, f.r.git("log", "-1", "--format=%P", res.Merges[1].Sha), pre+" "+f.shas["B"], "B merged onto the unchanged HEAD")
	eq(t, f.r.git("log", "--merges", "--format=%(trailers:key=Lsf-Unit,valueonly)", pre+"..HEAD"), "B", "no merge of A")
	eq(t, f.gated(), 2)
	eq(t, f.r.git("rev-parse", f.runs[1]+"^{tree}"), f.r.git("rev-parse", res.Merges[1].Sha+"^{tree}"), "B's merge commit is the tree gated")
	eq(t, f.r.git("status", "--porcelain"), "")
	sigs := declaredMergeSignals(plan.SignalsFromMerge(res.MergeResult, plan.At{Wave: plan.W(1)}), res, true)
	eq(t, len(sigs), 1)
	eq(t, sigs[0].Kind, "merge-regression")
	eq(t, sigs[0].Statement, "merge of A failed its gates (`tests`: red — red at "+f.runs[0][:7]+"); nothing committed")
}

// A red resolved merge is aborted, with the requester's other changes left
// as they were.
func TestMergeGatesResolvedRed(t *testing.T) {
	f := newGateFixture(t, map[string]map[string]string{"A": {"shared.txt": "from A\n"}, "B": {"shared.txt": "from B\n", "src/bad": "x\n"}}, "A", "B")
	f.r.write("notes.txt", "mine\n")
	res, err := runMerge(t, f.orch(t, nil, resolveBoth(f)), f.declared("A", "B"), false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(res), "A=clean B=regressed")
	if f.r.mergeInProgress() {
		t.Error("the merge is still in progress")
	}
	eq(t, f.r.read("notes.txt"), "mine\n")
	eq(t, f.r.git("status", "--porcelain"), "?? notes.txt")
}

// A merge-gate not-run commits nothing and stops as gate-not-run.
func TestMergeGatesNotRun(t *testing.T) {
	f := newGateFixture(t, map[string]map[string]string{"A": {"src/slow": "x\n"}}, "A")
	head := f.r.head()
	res, err := runMerge(t, f.orch(t, nil, nil), f.declared("A"), false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(res), "A=regressed")
	eq(t, f.r.head(), head)
	eqs(t, res.GateNotRun, []string{"A"})
	sigs := declaredMergeSignals(plan.SignalsFromMerge(res.MergeResult, plan.At{Wave: plan.W(1)}), res, true)
	eq(t, sigs[0].Kind, "gate-not-run")
	contains(t, sigs[0].Statement, "the runtime could not run its gates")
}

// S5 (reuse): a merge that differs from the unit's gated tree only at its
// completion report reuses that result; once the integration branch has
// moved it does not.
func TestMergeGatesReuse(t *testing.T) {
	files := map[string]map[string]string{
		"A": {"src/a.go": "a\n", "docs/plans/A-completion.md": "report\n"},
		"B": {"src/b.go": "b\n", "docs/plans/B-completion.md": "report\n"},
	}
	f := newGateFixture(t, files, "A", "B")
	o := f.orch(t, nil, nil)
	in := f.declared("A", "B")
	base := f.r.head()
	for i, id := range []string{"A", "B"} {
		// The unit's gated tree: its work without the report, as the
		// implement stage left it on a branch from the integration branch.
		lower := strings.ToLower(id)
		f.r.git("checkout", "-q", "-b", "work-"+id, base)
		f.r.write("src/"+lower+".go", lower+"\n")
		f.r.git("add", "src/"+lower+".go")
		f.r.git("commit", "-q", "-m", "work")
		tree := f.r.git("rev-parse", "HEAD^{tree}")
		f.r.git("checkout", "-q", "integration")
		in.Units[i].GatedTree = tree
		if _, _, err := o.gates(context.Background(), f.r.git("commit-tree", tree, "-m", "unit")); err != nil {
			t.Fatal(err)
		}
	}
	eq(t, f.gated(), 2)
	res, err := runMerge(t, o, in, false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(res), "A=clean B=clean")
	eq(t, f.gated(), 3, "A reused its result; B, merging onto A's merge, was gated")
}

// S7: a kill during a clean merge's gates reruns them once on resume; a
// kill after the gate record and before the merge replays it. Either way
// there is one merge.
func TestMergeGatesKillClean(t *testing.T) {
	files := map[string]map[string]string{"A": {"src/a.go": "a\n"}}
	t.Run("during the gates", func(t *testing.T) {
		f := newGateFixture(t, files, "A")
		f.kill = func(orchtest.GateCall) bool { return f.gated() == 1 }
		if mergeKilled(t, f.orch(t, nil, nil), f.declared("A")) {
			t.Fatal("not killed")
		}
		res, err := runMerge(t, f.orch(t, nil, nil), f.declared("A"), true)
		if err != nil {
			t.Fatal(err)
		}
		eq(t, statuses(res), "A=clean")
		eq(t, f.gated(), 2, "the killed run is not counted")
		eq(t, f.r.git("rev-list", "--merges", "--count", "HEAD"), "1")
	})
	t.Run("after the gate record", func(t *testing.T) {
		f := newGateFixture(t, files, "A")
		if mergeKilled(t, f.orch(t, gitKill{argIs(0, "merge")}, nil), f.declared("A")) {
			t.Fatal("not killed")
		}
		res, err := runMerge(t, f.orch(t, nil, nil), f.declared("A"), true)
		if err != nil {
			t.Fatal(err)
		}
		eq(t, statuses(res), "A=clean")
		eq(t, f.gated(), 1, "replayed")
		eq(t, f.r.git("rev-list", "--merges", "--count", "HEAD"), "1")
	})
}

// S7 for a resolved merge: the merge stays in progress across the kill;
// the resume completes it once, its gates rerun (killed during them) or
// replayed (killed after their record).
func TestMergeGatesKillResolved(t *testing.T) {
	for name, c := range map[string]struct {
		kill  func(f *gateFixture) (gitRunner, func(orchtest.GateCall) bool)
		gated int
	}{
		"during the gates": {func(f *gateFixture) (gitRunner, func(orchtest.GateCall) bool) {
			return nil, func(orchtest.GateCall) bool { return f.gated() == 2 }
		}, 3},
		"after the gate record": {func(f *gateFixture) (gitRunner, func(orchtest.GateCall) bool) {
			return gitKill{argIs(0, "commit")}, nil
		}, 2},
	} {
		t.Run(name, func(t *testing.T) {
			f := newGateFixture(t, resolveShared, "A", "B")
			g, k := c.kill(f)
			f.kill = k
			if mergeKilled(t, f.orch(t, g, resolveBoth(f)), f.declared("A", "B")) {
				t.Fatal("not killed")
			}
			if !f.mc().mergeInProgress() {
				t.Fatal("no merge in progress in the merge clone after the kill")
			}
			f.kill = nil
			calls := 0
			h := map[string]orchtest.Handler{"merge-conflict": func(orchtest.Call, *orchtest.State) any {
				calls++ // the stub journals no calls: this stands for the replay
				return R(M{"status": "resolved", "files": []string{"shared.txt"}, "statement": "both"})
			}}
			res, err := runMerge(t, f.orch(t, nil, h), f.declared("A", "B"), true)
			if err != nil {
				t.Fatal(err)
			}
			eq(t, statuses(res), "A=clean B=resolved")
			eq(t, f.gated(), c.gated)
			eq(t, f.r.git("rev-list", "--merges", "--count", "HEAD"), "2")
			eq(t, f.r.git("show", res.Merges[1].Sha+":shared.txt"), "from A\nfrom B")
			eq(t, calls, 1, "one conflict call on resume, the resolution already staged")
		})
	}
}

// S7 layout [regressed A, resolved B killed mid-gate]: the resume replays
// A's recorded outcome instead of wedging, and merges B once.
func TestMergeGatesRegressedThenKilled(t *testing.T) {
	files := map[string]map[string]string{"A": {"src/bad": "x\n"}, "B": {"shared.txt": "from B\n"}, "C": {"shared.txt": "from C\n"}}
	f := newGateFixture(t, files, "A", "B", "C")
	// B merges cleanly; C conflicts with it and is killed during its gates.
	h := map[string]orchtest.Handler{"merge-conflict": func(orchtest.Call, *orchtest.State) any {
		f.mc().write("shared.txt", "from B\nfrom C\n")
		f.mc().git("add", "shared.txt")
		return R(M{"status": "resolved", "files": []string{"shared.txt"}, "statement": "both"})
	}}
	f.kill = func(orchtest.GateCall) bool { return f.gated() == 3 }
	if mergeKilled(t, f.orch(t, nil, h), f.declared("A", "B", "C")) {
		t.Fatal("not killed")
	}
	f.kill = nil
	res, err := runMerge(t, f.orch(t, nil, h), f.declared("A", "B", "C"), true)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(res), "A=regressed B=clean C=resolved")
	eq(t, f.r.git("log", "--format=%(trailers:key=Lsf-Unit,valueonly)", "--merges", "HEAD"), "C\n\nB", "A never merged")
}

// An unresolvable unit before a merge in progress resumes too (a wedge
// that predates declared gates).
func TestMergeUnresolvableThenKilled(t *testing.T) {
	files := map[string]map[string]string{"A": {"shared.txt": "from A\n"}, "B": {"src/b.go": "b\n", "other.txt": "from B\n"}, "C": {"other.txt": "from C\n"}}
	f := newGateFixture(t, files, "A", "B", "C")
	f.r.git("checkout", "-q", "integration")
	f.r.commit("shared.txt", "base\n", "base") // A now conflicts
	in := f.in("A", "B", "C")
	for i := range in.Units {
		in.Units[i].Sha = f.shas[in.Units[i].Unit]
	}
	h := map[string]orchtest.Handler{"merge-conflict": func(c orchtest.Call, _ *orchtest.State) any {
		if strings.HasSuffix(c.Label, ":A") {
			return R(M{"status": "unresolvable", "files": []string{"shared.txt"}, "statement": "no"})
		}
		f.mc().write("other.txt", "from B\nfrom C\n")
		f.mc().git("add", "other.txt")
		return R(M{"status": "resolved", "files": []string{"other.txt"}, "statement": "both"})
	}}
	if mergeKilled(t, f.orch(t, gitKill{argIs(0, "commit")}, h), in) {
		t.Fatal("not killed")
	}
	res, err := runMerge(t, f.orch(t, nil, h), in, true)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(res), "A=unresolvable B=clean C=resolved")
}

// A merge commit whose tree no gate run covers (here, a commit raced the
// merge and was adopted on relaunch) is gated; a red one is the step's
// fault, never accepted.
func TestMergeGatesUngatedCommit(t *testing.T) {
	f := newGateFixture(t, map[string]map[string]string{"A": {"src/a.go": "a\n"}}, "A")
	in := f.declared("A")
	f.r.write("src/bad", "x\n") // the requester's commit, merged under the step's trailers
	f.r.git("add", "src/bad")
	f.r.git("commit", "-q", "-m", "requester")
	f.r.git("merge", "-q", "--no-ff", "-m", mergeMessage(in, "merge:w1", "A", "clean", nil), f.shas["A"])
	_, err := runMerge(t, f.orch(t, nil, nil), in, true)
	errMatch(t, err, `merge commit [0-9a-f]+ of A is on the integration branch, or in the merge clone .*, but its gates do not pass`)
	eq(t, f.gated(), 1)
}

// The main repo's integration branch moving during the gates (a requester
// commit): the merge, made in the clone, does not fast-forward onto it, a
// fault; the relaunch's clone sync faults on the divergence until the
// clone is removed, and then the merge is made and gated again on the new
// tip.
func TestMergeGatesHeadMoves(t *testing.T) {
	files := map[string]map[string]string{"A": {"src/a.go": "a\n"}}
	f := newGateFixture(t, files, "A")
	n := 0
	f.moved = func() {
		if n++; n == 1 {
			f.r.commit("README.md", "moved\n", "requester's commit")
		}
	}
	_, err := runMerge(t, f.orch(t, nil, nil), f.declared("A"), false)
	errMatch(t, err, `integration moved to [0-9a-f]{40} while [0-9a-f]{40} was made on an earlier tip`)
	eq(t, f.gated(), 1)
	_, err = runMerge(t, f.orch(t, nil, nil), f.declared("A"), true)
	errMatch(t, err, `have diverged.*; remove .* and relaunch`)
	os.RemoveAll(f.mc().dir)
	res, err := runMerge(t, f.orch(t, nil, nil), f.declared("A"), true)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, statuses(res), "A=clean")
	eq(t, f.gated(), 2)
	eq(t, f.r.git("show", res.Merges[0].Sha+":README.md"), "moved")
}

// A relaunch that finds several of the step's commits in the merge clone
// past main fast-forwards main to each only once it is adopted, verified
// and gated: B's red merge never reaches main, which stays at A's.
func TestMergeGatesPendingCommitsAdoptedOneByOne(t *testing.T) {
	f := newGateFixture(t, map[string]map[string]string{"A": {"src/a.go": "a\n"}, "B": {"src/bad": "x\n"}}, "A", "B")
	in := f.declared("A", "B")
	pre := f.r.head()
	c := f.makeClone()
	c.git("merge", "-q", "--no-ff", "-m", mergeMessage(in, "merge:w1", "A", "clean", nil), f.shas["A"])
	mergeA := c.head()
	c.git("merge", "-q", "--no-ff", "-m", mergeMessage(in, "merge:w1", "B", "clean", nil), f.shas["B"])
	_, err := runMerge(t, f.orch(t, nil, nil), in, true)
	errMatch(t, err, `merge commit [0-9a-f]+ of B is on the integration branch, or in the merge clone .*, but its gates do not pass`)
	eq(t, f.r.head(), mergeA, "main at A's merge, not B's")
	if f.r.head() == pre {
		t.Error("A's merge was not fast-forwarded")
	}
}
