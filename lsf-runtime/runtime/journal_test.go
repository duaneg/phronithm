package lsfrt

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestResumeReplaysCompletedCalls(t *testing.T) {
	cfg, stub := testConfig(t)
	t.Setenv("STUB_SCRIPT", "ok")
	first, _ := call(t, cfg, vOpts)
	second, _ := call(t, cfg, vOpts)
	if stubCount(t, stub) != 1 || !second.Replayed || string(second.Value) != string(first.Value) || second.Attempts != 0 {
		t.Fatalf("spawns %d, first %+v, second %+v", stubCount(t, stub), first, second)
	}
	if countEvents(telemetry(t, cfg), "cache-hit") != 1 {
		t.Error("want one cache-hit")
	}
	// A changed prompt (here: effort) re-runs.
	changed := vOpts
	changed.Effort = "high"
	if r, _ := call(t, cfg, changed); r.Replayed || stubCount(t, stub) != 2 {
		t.Errorf("changed inputs replayed: %+v", r)
	}
}

func TestNullIsNotJournaled(t *testing.T) {
	cfg, stub := testConfig(t)
	t.Setenv("STUB_SCRIPT", "nullout,nullout,ok")
	if r, _ := call(t, cfg, vOpts); r.Value != nil {
		t.Fatalf("first launch %+v", r)
	}
	if r, _ := call(t, cfg, vOpts); r.Value == nil || r.Replayed || stubCount(t, stub) != 3 {
		t.Fatalf("second launch %+v, spawns %d", r, stubCount(t, stub))
	}
}

func TestFlagsSurviveReplay(t *testing.T) {
	cfg, _ := testConfig(t)
	cfg.Skills = []SkillRule{{Label: "^critique-", Skill: "phronithm:nonexistent-skill"}}
	t.Setenv("STUB_SCRIPT", "fixture:skill-unknown")
	o := Opts{Label: "critique-code:r1:u1", Schema: json.RawMessage(findingsSchema)}
	call(t, cfg, o)
	r, _ := call(t, cfg, o)
	if !r.Replayed || len(r.Flags) != 1 || r.Flags[0] != "skill-not-loaded" {
		t.Fatalf("replay %+v", r)
	}
}

func TestJournalKeysAreDistinct(t *testing.T) {
	if journalKey("a:b") == journalKey("a_b") {
		t.Fatal("a:b and a_b share a key")
	}
	if k := journalKey("critique-code:r1:u/1"); strings.ContainsAny(k, ":/") {
		t.Fatalf("unsafe key %q", k)
	}
}

func TestStepJournal(t *testing.T) {
	cfg, _ := testConfig(t)
	runs := 0
	fn := func(_ context.Context, interrupted bool) (json.RawMessage, error) {
		runs++
		return json.RawMessage(`{"sha":"abc"}`), nil
	}
	for range 2 {
		launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
			v, err := rt.Step(ctx, "ledger:w1", []byte("block"), fn)
			if err != nil || string(v) != `{"sha":"abc"}` {
				t.Errorf("step %s %v", v, err)
			}
			return nil, nil
		})
	}
	if runs != 1 {
		t.Fatalf("fn ran %d times", runs)
	}
	// New input re-runs.
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		rt.Step(ctx, "ledger:w1", []byte("other block"), fn)
		return nil, nil
	})
	if runs != 2 {
		t.Fatalf("fn ran %d times after an input change", runs)
	}
}

func TestStepInterruptedAndFailed(t *testing.T) {
	cfg, _ := testConfig(t)
	boom := errors.New("commit rejected")
	var sawInterrupted []bool
	fail := true
	fn := func(_ context.Context, interrupted bool) (json.RawMessage, error) {
		sawInterrupted = append(sawInterrupted, interrupted)
		if fail {
			return nil, boom
		}
		return json.RawMessage(`null`), nil
	}
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		if _, err := rt.Step(ctx, "merge:w1", []byte("in"), fn); !errors.Is(err, boom) {
			t.Errorf("err %v", err)
		}
		return nil, nil
	})
	intent := filepath.Join(cfg.StateDir, "journal", journalKey("merge:w1")+".intent")
	if _, err := os.Stat(intent); err != nil {
		t.Fatalf("a failed step's intent is gone: %v", err)
	}
	fail = false
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		_, err := rt.Step(ctx, "merge:w1", []byte("in"), fn)
		return nil, err
	})
	if len(sawInterrupted) != 2 || sawInterrupted[0] || !sawInterrupted[1] {
		t.Fatalf("interrupted flags %v", sawInterrupted)
	}
	if _, err := os.Stat(intent); !os.IsNotExist(err) {
		t.Errorf("intent left after success: %v", err)
	}
	// A null step return is recorded and replayed.
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		rt.Step(ctx, "merge:w1", []byte("in"), fn)
		return nil, nil
	})
	if len(sawInterrupted) != 2 {
		t.Errorf("a recorded null step re-ran")
	}
}

// gitRepo makes a repository with one commit on main and returns its path.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.email=t@x", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@x", "-c", "user.name=t"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestWorktreeIsolation(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.Repo = gitRepo(t)
	cfg.Args = json.RawMessage(`{"planSha":"0123456789ab","integrationBranch":"main"}`)
	t.Setenv("STUB_SCRIPT", "ok")
	o := vOpts
	o.Isolation = "worktree"
	key := journalKey(o.Label)
	wt := filepath.Join(cfg.StateDir, "wt", key)
	branch := "lsf/01234567/" + key

	if r, err := call(t, cfg, o); err != nil || r.Value == nil {
		t.Fatalf("%+v %v", r, err)
	}
	if git(t, wt, "rev-parse", "--abbrev-ref", "HEAD") != branch {
		t.Fatalf("worktree not on %s", branch)
	}
	// The stub ran in the worktree: its argv record is ours, cwd is checked via git.
	if stubCount(t, stub) != 1 {
		t.Fatal("no spawn")
	}
	// Work on the branch, then lose the worktree directory: the next launch
	// keeps the commit rather than resetting the branch.
	git(t, wt, "commit", "-q", "--allow-empty", "-m", "unit work")
	tip := git(t, wt, "rev-parse", "HEAD")
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}
	o.Effort = "high" // a fresh call (no replay) for the same label
	if _, err := call(t, cfg, o); err != nil {
		t.Fatal(err)
	}
	if got := git(t, wt, "rev-parse", "HEAD"); got != tip {
		t.Fatalf("branch reset: HEAD %s, want %s", got, tip)
	}
}

func TestParallelWorktrees(t *testing.T) {
	cfg, _ := testConfig(t)
	cfg.Repo = gitRepo(t)
	cfg.Args = json.RawMessage(`{"planSha":"0123456789ab","integrationBranch":"main"}`)
	t.Setenv("STUB_SCRIPT", "ok")
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		var thunks []func(context.Context) (json.RawMessage, error)
		for i := range 4 {
			o := vOpts
			o.Label = "critique-code:r1:u" + strconv.Itoa(i)
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
	if n := len(strings.Split(git(t, cfg.Repo, "worktree", "list"), "\n")); n != 5 {
		t.Fatalf("%d worktrees, want 5", n)
	}
}

func TestWorktreeNeedsArgs(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.Repo = gitRepo(t)
	cfg.Args = json.RawMessage(`{"integrationBranch":"main"}`)
	o := vOpts
	o.Isolation = "worktree"
	_, err := call(t, cfg, o)
	if r, ok := IsStop(err); !ok || r != StopRuntimeError {
		t.Fatalf("err %v", err)
	}
	if stubCount(t, stub) != 0 || readSummary(t, cfg).StopReason != StopRuntimeError {
		t.Fatal("spawned, or summary not runtime-error")
	}
}
