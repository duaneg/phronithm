package lsfrt

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// stubCwd is invocation n's working directory.
func stubCwd(t *testing.T, stub string, n int) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(stub, "cwd."+strconv.Itoa(n)))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func resolved(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// withRepoCallsInClones runs a test with writer repo calls in kept clones.
func withRepoCallsInClones(t *testing.T) {
	keptRepoCallsInRepo = false
	t.Cleanup(func() { keptRepoCallsInRepo = true })
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// An isolated writer runs in its own kept clone, never the main checkout or
// a worktree: a plain clone with every main branch local, the main repo's
// hooks and identity, and no remote. Its commit comes back as main's
// lsf/<plan8>/<key>, without its tag; the main checkout's branch is
// unmoved. A lost clone is remade from that branch; an existing one is
// reused.
func TestCloneIsolatedWriter(t *testing.T) {
	cfg, stub := testConfig(t)
	git(t, cfg.Repo, "branch", "other")
	git(t, cfg.Repo, "tag", "main-tag")
	marker := filepath.Join(t.TempDir(), "hook-ran")
	hook := filepath.Join(cfg.Repo, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho ran >> '"+marker+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	mainTip := git(t, cfg.Repo, "rev-parse", "main")
	t.Setenv("STUB_SCRIPT", "commit:work.txt")
	o := vOpts
	o.Isolation = "worktree"
	key := journalKey(o.Label)
	clone := filepath.Join(cfg.StateDir, "clone", key)
	branch := "lsf/01234567/" + key

	res, err := call(t, cfg, o)
	if err != nil || res.Value == nil || res.Clone != CloneKept || res.Branch != branch {
		t.Fatalf("%+v %v", res, err)
	}
	if got := stubCwd(t, stub, 1); got != resolved(t, clone) {
		t.Fatalf("cwd %s, want %s", got, clone)
	}
	if fi, err := os.Lstat(filepath.Join(clone, ".git")); err != nil || !fi.IsDir() {
		t.Fatalf("not a plain clone: %v", err)
	}
	if git(t, clone, "remote") != "" || git(t, clone, "tag", "-l", "main-tag") != "" {
		t.Error("the clone has a remote or main's tag")
	}
	for _, b := range []string{"main", "other", branch} {
		if git(t, clone, "branch", "--list", b) == "" {
			t.Errorf("branch %s not local in the clone", b)
		}
	}
	if tip := git(t, cfg.Repo, "rev-parse", branch); tip != git(t, clone, "rev-parse", "HEAD") || tip != res.CloneHead {
		t.Errorf("main's %s at %s, clone HEAD %s, CloneHead %s", branch, tip, git(t, clone, "rev-parse", "HEAD"), res.CloneHead)
	}
	if git(t, cfg.Repo, "log", "-1", "--format=%an", branch) != "repo-local" {
		t.Error("the clone's commit does not carry the main repo's local identity")
	}
	if b, err := os.ReadFile(marker); err != nil || string(b) != "ran\n" {
		t.Errorf("main's pre-commit hook did not run in the clone: %q %v", b, err)
	}
	if git(t, cfg.Repo, "tag", "-l", "t-work.txt") != "" {
		t.Error("the clone's tag was fetched")
	}
	if git(t, cfg.Repo, "rev-parse", "main") != mainTip || strings.Contains(git(t, cfg.Repo, "worktree", "list"), cfg.StateDir) {
		t.Error("main moved, or a worktree was made")
	}

	// A re-run (not a replay) reuses the clone and its commit.
	o.Effort = "high"
	t.Setenv("STUB_SCRIPT", "ok")
	tip := git(t, clone, "rev-parse", "HEAD")
	if _, err := call(t, cfg, o); err != nil {
		t.Fatal(err)
	}
	if git(t, clone, "rev-parse", "HEAD") != tip || countEvents(telemetry(t, cfg), "clone") != 1 {
		t.Errorf("not reused: HEAD %s, %d clone events", git(t, clone, "rev-parse", "HEAD"), countEvents(telemetry(t, cfg), "clone"))
	}
	// A lost clone is remade from main's branch, keeping the commit.
	os.RemoveAll(clone)
	o.Effort = "medium"
	if _, err := call(t, cfg, o); err != nil {
		t.Fatal(err)
	}
	if git(t, clone, "rev-parse", "HEAD") != tip {
		t.Errorf("remade clone at %s, want %s", git(t, clone, "rev-parse", "HEAD"), tip)
	}
}

// A reviewer (a discarded role) runs in scratch/, gets no uncommitted
// check, and its clone is gone after the call; nothing comes back.
func TestCloneDiscardedReviewer(t *testing.T) {
	cfg, stub := testConfig(t)
	t.Setenv("STUB_SCRIPT", "dirty:out.txt")
	for _, iso := range []string{"", "worktree"} {
		o := vOpts
		o.Label = "critique-code:r1:u" + iso
		o.Isolation = iso
		res, err := call(t, cfg, o)
		n := stubCount(t, stub)
		scratch := filepath.Join(cfg.StateDir, "scratch", journalKey(o.Label))
		if err != nil || res.Value == nil || res.Clone != CloneDiscarded || res.Branch != "" || len(res.Uncommitted) != 0 || slices.Contains(res.Flags, FlagUncommittedUnchecked) {
			t.Fatalf("%q: %+v %v", iso, res, err)
		}
		if cwd := stubCwd(t, stub, n); !strings.HasSuffix(cwd, "/scratch/"+journalKey(o.Label)) {
			t.Errorf("%q: cwd %s", iso, cwd)
		}
		if exists(scratch) {
			t.Errorf("%q: scratch clone left", iso)
		}
		if git(t, cfg.Repo, "branch", "--list", "lsf/*") != "" {
			t.Errorf("%q: a branch came back: %s", iso, git(t, cfg.Repo, "branch", "--list", "lsf/*"))
		}
	}
	// A crashed launch's scratch clone is wiped at the next launch.
	left := filepath.Join(cfg.StateDir, "scratch", "left")
	os.MkdirAll(left, 0o755)
	launch(t, cfg, func(context.Context, *Runtime) (json.RawMessage, error) { return nil, nil })
	if exists(left) {
		t.Error("scratch not wiped at launch")
	}
}

// With writer repo calls in clones, a repo writer's clone is on the
// integration branch and its commit comes back as lsf/<plan8>/<key>, the
// integration branch unmoved. A dirty path is reported; a requester commit
// in the clone reaches the main repo at the replay, which clears it. A
// stale clone (main moved) is remade.
func TestCloneRepoWriter(t *testing.T) {
	withRepoCallsInClones(t)
	cfg, stub := testConfig(t)
	mainTip := git(t, cfg.Repo, "rev-parse", "main")
	t.Setenv("STUB_SCRIPT", "commit:plan.md")
	t.Setenv("STUB_DIRTY", "loose.txt")
	o := vOpts
	o.Label = "replan:w1"
	key := journalKey(o.Label)
	clone := filepath.Join(cfg.StateDir, "clone", key)
	branch := "lsf/01234567/" + key
	res, err := call(t, cfg, o)
	if err != nil || res.Clone != CloneKept || res.Branch != branch || !slices.Equal(res.Uncommitted, []string{"loose.txt"}) {
		t.Fatalf("%+v %v", res, err)
	}
	if stubCwd(t, stub, 1) != resolved(t, clone) || git(t, clone, "symbolic-ref", "--short", "HEAD") != "main" {
		t.Fatal("not a clone on main")
	}
	if git(t, cfg.Repo, "rev-parse", "main") != mainTip || git(t, cfg.Repo, "log", "-1", "--format=%s", branch) != "plan.md" {
		t.Fatal("main moved, or the commit did not come back")
	}
	// The requester commits the loose path in the clone; the replay
	// fetches it and finds the clone clean.
	git(t, clone, "add", "loose.txt")
	git(t, clone, "commit", "-q", "-m", "requester")
	t.Setenv("STUB_DIRTY", "")
	again, err := call(t, cfg, o)
	if err != nil || !again.Replayed || len(again.Uncommitted) != 0 || git(t, cfg.Repo, "log", "-1", "--format=%s", branch) != "requester" {
		t.Fatalf("replay %+v %v; branch at %q", again, err, git(t, cfg.Repo, "log", "-1", "--format=%s", branch))
	}
	// Main moves: the re-run's clone is stale, so it is remade from main.
	git(t, cfg.Repo, "commit", "-q", "--allow-empty", "-m", "moved")
	o.Effort = "high"
	t.Setenv("STUB_SCRIPT", "ok")
	if _, err := call(t, cfg, o); err != nil {
		t.Fatal(err)
	}
	if git(t, clone, "rev-parse", "HEAD") != git(t, cfg.Repo, "rev-parse", "main") || countEvents(telemetry(t, cfg), "clone-stale") != 1 {
		t.Errorf("stale clone not remade: HEAD %s", git(t, clone, "rev-parse", "HEAD"))
	}
}

// With the switch on, a writer repo call still runs in the main checkout.
func TestCloneSwitchKeepsRepoWriterInRepo(t *testing.T) {
	cfg, stub := testConfig(t)
	t.Setenv("STUB_SCRIPT", "ok")
	res, err := call(t, cfg, Opts{Label: "replan:w1", Schema: json.RawMessage(verdictSchema)})
	if err != nil || res.Clone != "" || res.Branch != "" || stubCwd(t, stub, 1) != resolved(t, cfg.Repo) {
		t.Fatalf("%+v %v", res, err)
	}
}

// Parallel kept clones each get their own directory.
func TestCloneParallel(t *testing.T) {
	cfg, stub := testConfig(t)
	t.Setenv("STUB_SCRIPT", "ok")
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		var thunks []func(context.Context) (json.RawMessage, error)
		for i := range 4 {
			o := vOpts
			o.Label = "implement:u" + strconv.Itoa(i)
			o.Isolation = "worktree"
			thunks = append(thunks, func(ctx context.Context) (json.RawMessage, error) { return rt.Agent(ctx, "x", o) })
		}
		out, err := rt.Parallel(ctx, thunks)
		for i, v := range out {
			if v == nil {
				t.Errorf("branch %d null", i)
			}
		}
		return nil, err
	})
	seen := map[string]bool{}
	for n := 1; n <= 4; n++ {
		seen[stubCwd(t, stub, n)] = true
	}
	if entries, _ := os.ReadDir(filepath.Join(cfg.StateDir, "clone")); len(entries) != 4 || len(seen) != 4 {
		t.Fatalf("%d clones, %d cwds", len(entries), len(seen))
	}
}

// A directory at the clone's path that is not a clone on its branch stops
// the run before any spawn; so does missing args.
func TestCloneStaleDirectoryStops(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{"plain directory", func(t *testing.T, dir string) { os.MkdirAll(filepath.Join(dir, "x"), 0o755) }},
		{"worktree-like .git file", func(t *testing.T, dir string) {
			os.MkdirAll(dir, 0o755)
			os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /nowhere\n"), 0o644)
		}},
	} {
		cfg, stub := testConfig(t)
		o := vOpts
		o.Isolation = "worktree"
		tc.setup(t, filepath.Join(cfg.StateDir, "clone", journalKey(o.Label)))
		_, err := call(t, cfg, o)
		if r, ok := IsStop(err); !ok || r != StopRuntimeError || stubCount(t, stub) != 0 {
			t.Errorf("%s: err %v, spawns %d", tc.name, err, stubCount(t, stub))
		}
	}
	cfg, stub := testConfig(t)
	cfg.Args = json.RawMessage(`{"integrationBranch":"main"}`)
	_, err := call(t, cfg, Opts{Label: "critique-code:r1:u1", Schema: json.RawMessage(verdictSchema)})
	if r, ok := IsStop(err); !ok || r != StopRuntimeError || stubCount(t, stub) != 0 {
		t.Errorf("no planSha: err %v, spawns %d", err, stubCount(t, stub))
	}
}

// A shallow repo, one with submodules and one with an LFS filter are
// refused at launch, by name.
func TestCloneRepoPreconditions(t *testing.T) {
	cfg, _ := testConfig(t)
	commitFile(t, cfg.Repo, "a.txt", "a\n")
	shallow := filepath.Join(t.TempDir(), "shallow")
	if out, err := exec.Command("git", "clone", "-q", "--depth", "1", "file://"+cfg.Repo, shallow).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	sub, _ := testConfig(t)
	commitFile(t, sub.Repo, ".gitmodules", "[submodule \"x\"]\n\tpath = x\n\turl = ../x\n")
	lfs, _ := testConfig(t)
	git(t, lfs.Repo, "config", "filter.lfs.clean", "git-lfs clean -- %f")
	for repo, want := range map[string]string{shallow: "is shallow", sub.Repo: "has submodules", lfs.Repo: "has an LFS filter"} {
		c, _ := testConfig(t)
		c.Repo = repo
		_, err := Run(context.Background(), c, func(context.Context, *Runtime) (json.RawMessage, error) { return nil, nil })
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", repo, err, want)
		}
	}
}

// Opts.Dir runs a call in an existing merge clone as found: no clone, no
// fetch-back, no uncommitted check. A Dir outside merge/, symlink
// included, is refused.
func TestCallDir(t *testing.T) {
	cfg, stub := testConfig(t)
	t.Setenv("STUB_SCRIPT", "dirty:resolved.txt")
	var res CallResult
	var dir string
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		var err error
		if dir, err = rt.MergeClone(ctx, "w1"); err != nil {
			t.Fatal(err)
		}
		o := Opts{Label: "merge-conflict:w1:u1", Dir: dir, Schema: json.RawMessage(verdictSchema)}
		if res, err = rt.Call(ctx, "resolve", o); err != nil {
			t.Fatal(err)
		}
		return nil, nil
	})
	if res.Value == nil || res.Clone != "" || res.Branch != "" || len(res.Uncommitted) != 0 || stubCwd(t, stub, 1) != resolved(t, dir) || !exists(filepath.Join(dir, "resolved.txt")) {
		t.Fatalf("%+v cwd %s", res, stubCwd(t, stub, 1))
	}
	link := filepath.Join(cfg.StateDir, "merge", "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	for i, d := range []string{t.TempDir(), link} {
		_, err := call(t, cfg, Opts{Label: "merge-conflict:w1:x" + strconv.Itoa(i), Dir: d, Schema: json.RawMessage(verdictSchema)})
		if r, ok := IsStop(err); !ok || r != StopRuntimeError {
			t.Errorf("%s: err %v", d, err)
		}
	}
	if stubCount(t, stub) != 1 {
		t.Errorf("%d spawns", stubCount(t, stub))
	}
}

// A merge clone is reused as found, a merge in progress included, and goes
// with RemoveMergeClone.
func TestMergeCloneReuse(t *testing.T) {
	cfg, _ := testConfig(t)
	git(t, cfg.Repo, "checkout", "-q", "-b", "feat")
	commitFile(t, cfg.Repo, "f.txt", "f\n")
	git(t, cfg.Repo, "checkout", "-q", "main")
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		d1, err := rt.MergeClone(ctx, "w1")
		if err != nil {
			t.Fatal(err)
		}
		git(t, d1, "merge", "-q", "--no-commit", "--no-ff", "feat")
		d2, err := rt.MergeClone(ctx, "w1")
		if err != nil || d2 != d1 || !exists(filepath.Join(d1, ".git", "MERGE_HEAD")) {
			t.Fatalf("reuse: %s %s %v", d1, d2, err)
		}
		if _, err := rt.MergeClone(ctx, "../x"); err == nil {
			t.Error("a name with a slash was taken")
		}
		if err := rt.RemoveMergeClone(ctx, "w1"); err != nil || exists(d1) {
			t.Errorf("remove: %v", err)
		}
		return nil, nil
	})
}

// cloneLaunch is worktreeLaunch for clones: one launch over a repository
// whose main branch ignores *.log, with writer repo calls in clones.
func cloneLaunch(t *testing.T, script string, body func(ctx context.Context, rt *Runtime)) Config {
	t.Helper()
	withRepoCallsInClones(t)
	t.Setenv("STUB_SCRIPT", script)
	cfg, _ := testConfig(t)
	commitFile(t, cfg.Repo, ".gitignore", "*.log\n")
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		body(ctx, rt)
		return nil, nil
	})
	return cfg
}

func mustCall(t *testing.T, ctx context.Context, rt *Runtime, label, iso string) (CallResult, string) {
	t.Helper()
	res, err := rt.Call(ctx, "x", Opts{Label: label, Isolation: iso, Schema: json.RawMessage(verdictSchema)})
	if err != nil || res.Value == nil {
		t.Fatalf("%s: %+v %v", label, res, err)
	}
	return res, filepath.Join(rt.cfg.StateDir, "clone", journalKey(label))
}

// RemoveWorktree removes a merged kept clone, listing its ignored and
// untracked files, and its branch; a repo writer's clone (on the
// integration branch) is removed the same way.
func TestRemoveClone(t *testing.T) {
	var unit, repo WorktreeRemoval
	var unitClone, repoClone string
	cfg := cloneLaunch(t, "dirty:build.log", func(ctx context.Context, rt *Runtime) {
		_, unitClone = mustCall(t, ctx, rt, "implement:A", "worktree")
		os.WriteFile(filepath.Join(unitClone, "new.txt"), []byte("x"), 0o644)
		unit = remove(t, ctx, rt, "implement:A", true)
		_, repoClone = mustCall(t, ctx, rt, "replan:w1", "")
		repo = remove(t, ctx, rt, "replan:w1", true)
	})
	got := slices.Clone(unit.Paths)
	slices.Sort(got)
	if !unit.Removed || unit.Path != unitClone || !slices.Equal(got, []string{"build.log", "new.txt"}) || exists(unitClone) {
		t.Errorf("unit: %+v", unit)
	}
	if !repo.Removed || !slices.Equal(repo.Paths, []string{"build.log"}) || exists(repoClone) {
		t.Errorf("repo: %+v", repo)
	}
	if b := git(t, cfg.Repo, "branch", "--list", "lsf/*"); b != "" {
		t.Errorf("branches left: %s", b)
	}
}

// A clone is kept: unmerged, its branch gone, off its branch, with
// commits not fetched, or beside a worktree for the same label.
func TestRemoveCloneKeeps(t *testing.T) {
	r := map[string]WorktreeRemoval{}
	cloneLaunch(t, "commit:x.txt,ok", func(ctx context.Context, rt *Runtime) {
		mustCall(t, ctx, rt, "implement:U", "worktree") // commits: not merged
		r["unmerged"] = remove(t, ctx, rt, "implement:U", true)

		mustCall(t, ctx, rt, "implement:M", "worktree")
		git(t, rt.cfg.Repo, "branch", "-D", "lsf/01234567/"+journalKey("implement:M"))
		r["missing"] = remove(t, ctx, rt, "implement:M", true)

		_, c := mustCall(t, ctx, rt, "implement:S", "worktree")
		git(t, c, "checkout", "-q", "-b", "elsewhere")
		r["switched"] = remove(t, ctx, rt, "implement:S", true)

		_, c = mustCall(t, ctx, rt, "implement:F", "worktree")
		git(t, c, "commit", "-q", "--allow-empty", "-m", "after return")
		r["unfetched"] = remove(t, ctx, rt, "implement:F", true)

		mustCall(t, ctx, rt, "implement:B", "worktree")
		mustWorktree(t, ctx, rt, "implement:B")
		r["both"] = remove(t, ctx, rt, "implement:B", true)
	})
	for k, want := range map[string]string{
		"unmerged": "not merged into main", "missing": "branch missing", "switched": "not a clone on lsf/01234567/" + journalKey("implement:S"),
		"unfetched": "clone has commits not fetched", "both": "both a clone and a worktree",
	} {
		if r[k].Removed || r[k].Kept != want || !exists(r[k].Path) {
			t.Errorf("%s: %+v, want kept %q", k, r[k], want)
		}
	}
}

// A hooks directory with a subdirectory is copied whole; a stale repo-call
// clone with dirt is not removed.
func TestCloneHooksTreeAndStaleDirt(t *testing.T) {
	withRepoCallsInClones(t)
	cfg, _ := testConfig(t)
	hooks := filepath.Join(cfg.Repo, ".git", "hooks")
	os.MkdirAll(filepath.Join(hooks, "_"), 0o755)
	marker := filepath.Join(t.TempDir(), "sourced")
	os.WriteFile(filepath.Join(hooks, "_", "lib.sh"), []byte("echo ok > '"+marker+"'\n"), 0o644)
	os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\n. \"$(dirname \"$0\")/_/lib.sh\"\n"), 0o755)
	t.Setenv("STUB_SCRIPT", "commit:a.txt")
	t.Setenv("STUB_DIRTY", "loose.txt")
	o := Opts{Label: "replan:w1", Schema: json.RawMessage(verdictSchema)}
	if _, err := call(t, cfg, o); err != nil {
		t.Fatal(err)
	}
	if !exists(marker) {
		t.Error("the hook's sourced file was not copied")
	}
	git(t, cfg.Repo, "commit", "-q", "--allow-empty", "-m", "moved")
	o.Effort = "high"
	_, err := call(t, cfg, o)
	if r, ok := IsStop(err); !ok || r != StopRuntimeError || !exists(filepath.Join(cfg.StateDir, "clone", journalKey(o.Label), "loose.txt")) {
		t.Errorf("stale dirty clone: %v", err)
	}
}
