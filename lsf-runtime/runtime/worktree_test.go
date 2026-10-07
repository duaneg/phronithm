package lsfrt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// worktreeLaunch runs body in one launch over a repository whose main
// branch ignores *.log.
func worktreeLaunch(t *testing.T, body func(ctx context.Context, rt *Runtime)) Config {
	t.Helper()
	cfg, _ := testConfig(t)
	cfg.Repo = gitRepo(t)
	cfg.Args = json.RawMessage(`{"planSha":"0123456789ab","integrationBranch":"main"}`)
	os.WriteFile(filepath.Join(cfg.Repo, ".gitignore"), []byte("*.log\n"), 0o644)
	os.WriteFile(filepath.Join(cfg.Repo, "tracked.txt"), []byte("one\n"), 0o644)
	git(t, cfg.Repo, "add", ".gitignore", "tracked.txt")
	git(t, cfg.Repo, "commit", "-q", "-m", "ignore logs")
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		body(ctx, rt)
		return nil, nil
	})
	return cfg
}

func mustWorktree(t *testing.T, ctx context.Context, rt *Runtime, label string) (string, string) {
	t.Helper()
	p, b, err := rt.Worktree(ctx, label)
	if err != nil {
		t.Fatal(err)
	}
	return p, b
}

func remove(t *testing.T, ctx context.Context, rt *Runtime, label string, deleteBranch bool) WorktreeRemoval {
	t.Helper()
	r, err := rt.RemoveWorktree(ctx, label, "main", deleteBranch)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A merged worktree goes with its untracked, ignored and modified files,
// listed; its branch goes only when asked; a second removal finds nothing.
func TestRemoveWorktreeMerged(t *testing.T) {
	var first, kept, again WorktreeRemoval
	var wt, branch, keptBranch string
	cfg := worktreeLaunch(t, func(ctx context.Context, rt *Runtime) {
		wt, branch = mustWorktree(t, ctx, rt, "completion-report:A")
		os.WriteFile(filepath.Join(wt, "tracked.txt"), []byte("two\n"), 0o644)
		os.WriteFile(filepath.Join(wt, "new.txt"), []byte("x\n"), 0o644)
		os.WriteFile(filepath.Join(wt, "run.log"), []byte("x\n"), 0o644)
		os.MkdirAll(filepath.Join(wt, "out", "deep"), 0o755)
		os.WriteFile(filepath.Join(wt, "out", "deep", "a.bin"), []byte("x\n"), 0o644)
		first = remove(t, ctx, rt, "completion-report:A", true)
		again = remove(t, ctx, rt, "completion-report:A", true)
		_, keptBranch = mustWorktree(t, ctx, rt, "implement:A")
		kept = remove(t, ctx, rt, "implement:A", false)
	})
	if !first.Removed || first.Kept != "" || first.Branch != branch || first.Path != wt {
		t.Fatalf("first: %+v", first)
	}
	want := []string{"new.txt", "out/", "run.log", "tracked.txt"}
	got := slices.Clone(first.Paths)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("paths %v, want %v", got, want)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("worktree still there: %v", err)
	}
	if out, _ := gitStdout(context.Background(), cfg.Repo, "branch", "--list", branch); len(out) != 0 {
		t.Errorf("branch %s not deleted", branch)
	}
	if again.Removed || again.Kept != "absent" {
		t.Errorf("again: %+v", again)
	}
	if !kept.Removed || git(t, cfg.Repo, "branch", "--list", keptBranch) == "" {
		t.Errorf("deleteBranch false: %+v; branch kept: %q", kept, git(t, cfg.Repo, "branch", "--list", keptBranch))
	}
	if n := countEvents(telemetry(t, cfg), "worktree-removed"); n != 2 {
		t.Errorf("%d worktree-removed events, want 2", n)
	}
}

// An unmerged branch, a directory that is not the label's worktree and a
// directory whose branch is gone are all kept.
func TestRemoveWorktreeKeeps(t *testing.T) {
	var unmerged, foreign, missing WorktreeRemoval
	var wtU, wtF, wtM string
	worktreeLaunch(t, func(ctx context.Context, rt *Runtime) {
		wtU, _ = mustWorktree(t, ctx, rt, "implement:U")
		git(t, wtU, "commit", "-q", "--allow-empty", "-m", "unit work")
		unmerged = remove(t, ctx, rt, "implement:U", true)

		var b string
		wtF, b = mustWorktree(t, ctx, rt, "plan:F")
		git(t, rt.cfg.Repo, "worktree", "remove", wtF)
		os.MkdirAll(wtF, 0o755)
		os.WriteFile(filepath.Join(wtF, "keep.txt"), []byte("x\n"), 0o644)
		foreign = remove(t, ctx, rt, "plan:F", true)
		if git(t, rt.cfg.Repo, "branch", "--list", b) == "" {
			t.Errorf("foreign: branch %s deleted", b)
		}

		wtM, b = mustWorktree(t, ctx, rt, "plan:M")
		git(t, wtM, "checkout", "-q", "--detach")
		git(t, rt.cfg.Repo, "branch", "-D", b)
		missing = remove(t, ctx, rt, "plan:M", true)
	})
	if unmerged.Removed || unmerged.Kept != "not merged into main" {
		t.Errorf("unmerged: %+v", unmerged)
	}
	if _, err := os.Stat(wtU); err != nil {
		t.Errorf("unmerged worktree gone: %v", err)
	}
	if foreign.Removed || foreign.Kept == "" {
		t.Errorf("foreign: %+v", foreign)
	}
	if _, err := os.Stat(filepath.Join(wtF, "keep.txt")); err != nil {
		t.Errorf("foreign directory touched: %v", err)
	}
	if missing.Removed || missing.Kept != "branch missing" {
		t.Errorf("missing: %+v", missing)
	}
}

// An isolated call's result carries its worktree branch, live and on
// replay; a call without isolation carries none.
func TestCallResultBranch(t *testing.T) {
	cfg, _ := testConfig(t)
	cfg.Repo = gitRepo(t)
	cfg.Args = json.RawMessage(`{"planSha":"0123456789ab","integrationBranch":"main"}`)
	t.Setenv("STUB_SCRIPT", "ok")
	o := vOpts
	o.Isolation = "worktree"
	want := "lsf/01234567/" + journalKey(o.Label)
	live, err := call(t, cfg, o)
	if err != nil || live.Branch != want || live.Replayed {
		t.Fatalf("live: %+v %v", live, err)
	}
	// Another launch's planSha does not change a replayed branch.
	cfg.Args = json.RawMessage(`{"planSha":"fedcba9876543","integrationBranch":"main"}`)
	replay, err := call(t, cfg, o)
	if err != nil || !replay.Replayed || replay.Branch != want {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	plain, err := call(t, cfg, vOpts)
	if err != nil || plain.Branch != "" {
		t.Fatalf("no isolation: %+v %v", plain, err)
	}
}
