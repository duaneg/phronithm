package lsfrt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// isolatedConfig is testConfig over a real repository, for worktree calls.
func isolatedConfig(t *testing.T) (Config, string) {
	cfg, stub := testConfig(t)
	cfg.Args = json.RawMessage(`{"planSha":"0123456789ab","integrationBranch":"main"}`)
	return cfg, stub
}

func commitFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", name)
	git(t, dir, "commit", "-q", "-m", "add "+name)
}

// A writer's kept clone reports its tracked edit and untracked file, not
// its ignored one. A resume replays the call and re-checks the clone as it
// stands: cleaned, it is clean; removed, it is clean.
func TestUncommittedIsolated(t *testing.T) {
	cfg, _ := isolatedConfig(t)
	commitFile(t, cfg.Repo, "tracked.txt", "a\n")
	commitFile(t, cfg.Repo, ".gitignore", "ignored.txt\n")
	t.Setenv("STUB_SCRIPT", "modify:tracked.txt,dirty:ignored.txt")
	t.Setenv("STUB_DIRTY", "new.txt")
	o := vOpts
	o.Isolation = "worktree"
	res, err := call(t, cfg, o)
	if err != nil || !slices.Equal(res.Uncommitted, []string{"new.txt", "tracked.txt"}) || slices.Contains(res.Flags, FlagUncommittedUnchecked) {
		t.Fatalf("%+v %v", res, err)
	}
	clone := filepath.Join(cfg.StateDir, "clone", journalKey(o.Label))
	git(t, clone, "checkout", "--", "tracked.txt")
	os.Remove(filepath.Join(clone, "new.txt"))
	again, _ := call(t, cfg, o)
	if !again.Replayed || len(again.Uncommitted) != 0 {
		t.Fatalf("replay after cleaning %+v", again)
	}
	// Dirty again, then the clone removed: the replay finds nothing.
	os.WriteFile(filepath.Join(clone, "new.txt"), []byte("n"), 0o644)
	o.Effort = "high"
	t.Setenv("STUB_DIRTY", "")
	t.Setenv("STUB_SCRIPT", "dirty:other.txt")
	if r, _ := call(t, cfg, o); !slices.Equal(r.Uncommitted, []string{"new.txt", "other.txt"}) {
		t.Fatalf("second run %+v", r)
	}
	os.RemoveAll(clone)
	if r, _ := call(t, cfg, o); !r.Replayed || len(r.Uncommitted) != 0 {
		t.Fatalf("replay after removal %+v", r)
	}
}

func TestUncommittedReplayStillDirty(t *testing.T) {
	cfg, _ := isolatedConfig(t)
	t.Setenv("STUB_SCRIPT", "dirty:left.txt")
	o := vOpts
	o.Isolation = "worktree"
	first, _ := call(t, cfg, o)
	again, _ := call(t, cfg, o)
	if !slices.Equal(first.Uncommitted, []string{"left.txt"}) || !again.Replayed || !slices.Equal(again.Uncommitted, []string{"left.txt"}) {
		t.Fatalf("first %+v replay %+v", first, again)
	}
}

func TestUncommittedFailedAttemptsDirt(t *testing.T) {
	cfg, stub := testConfig(t)
	// The first attempt dirties the clone and fails transiently; the retry
	// runs in the same clone and succeeds. The dirt is the call's.
	t.Setenv("STUB_SCRIPT", "dirty-transient:partial.txt,ok")
	res, _ := call(t, cfg, vOpts)
	if stubCount(t, stub) != 2 || !slices.Equal(res.Uncommitted, []string{"partial.txt"}) {
		t.Fatalf("spawns %d, %+v", stubCount(t, stub), res)
	}
}

func TestUncommittedRenameIsOneEntry(t *testing.T) {
	cfg, _ := testConfig(t)
	commitFile(t, cfg.Repo, "old.txt", "a\n")
	git(t, cfg.Repo, "mv", "old.txt", "new.txt")
	rt := &Runtime{cfg: cfg, killCh: make(chan struct{})}
	paths, err := rt.dirtyPaths(context.Background(), cfg.Repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(paths, []string{"new.txt"}) {
		t.Fatalf("%q", paths)
	}
}

func TestUncommittedUncheckedNotJournaled(t *testing.T) {
	cfg, _ := testConfig(t)
	// The stub corrupts its clone's index: the check fails, the fetch-back
	// (refs and objects only) does not.
	t.Setenv("STUB_DIRTY", ".git/index")
	t.Setenv("STUB_SCRIPT", "ok")
	o := vOpts
	o.Isolation = "worktree"
	res, err := call(t, cfg, o)
	if err != nil || !slices.Contains(res.Flags, FlagUncommittedUnchecked) || len(res.Uncommitted) != 0 || res.Branch == "" {
		t.Fatalf("%+v %v", res, err)
	}
	b, err := os.ReadFile(filepath.Join(cfg.StateDir, "journal", journalKey(o.Label)+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var r record
	if err := json.Unmarshal(b, &r); err != nil || slices.Contains(r.Flags, FlagUncommittedUnchecked) || !r.Unchecked {
		t.Fatalf("record %s", b)
	}
	// A replay checks afresh: still corrupt, still unchecked.
	again, _ := call(t, cfg, o)
	if !again.Replayed || !slices.Contains(again.Flags, FlagUncommittedUnchecked) {
		t.Fatalf("replay %+v", again)
	}
}

// Every return after a spawn is checked: a value, a null after invalid
// returns, an exhausted transient, a permanent error and a guard timeout.
func TestUncommittedEveryReturnPath(t *testing.T) {
	for _, tc := range []struct{ name, script string }{
		{"ok", "ok"},
		{"invalid", "invalid"},
		{"transient", "transient"},
		{"permanent", "fixture:permanent"},
		{"timeout", "hang"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _ := isolatedConfig(t)
			cfg.GuardTimeout = Duration(300 * time.Millisecond)
			t.Setenv("STUB_SCRIPT", tc.script)
			t.Setenv("STUB_DIRTY", "partial.txt")
			o := vOpts
			o.Isolation = "worktree"
			res, err := call(t, cfg, o)
			if err != nil || !slices.Equal(res.Uncommitted, []string{"partial.txt"}) {
				t.Fatalf("%v %+v", err, res)
			}
		})
	}
}

func TestViolationsOnInvalidNull(t *testing.T) {
	cfg, _ := testConfig(t)
	t.Setenv("STUB_SCRIPT", "invalid")
	res, _ := call(t, cfg, vOpts)
	if res.Value != nil || len(res.Violations) == 0 {
		t.Fatalf("%+v", res)
	}
	t.Setenv("STUB_SCRIPT", "nullout")
	o := vOpts
	o.Label = "implement:u2"
	res, _ = call(t, cfg, o)
	if !slices.Equal(res.Violations, []string{NoOutput}) {
		t.Fatalf("no output %+v", res)
	}
}

func TestValidateOutputEmptySchema(t *testing.T) {
	if v := ValidateOutput(nil, json.RawMessage(`{"x":1}`)); v != nil {
		t.Fatalf("%v", v)
	}
	if v := ValidateOutput(json.RawMessage(verdictSchema), json.RawMessage(`{"wrong":1}`)); len(v) == 0 {
		t.Fatal("invalid passes")
	}
}

func TestStepCtxEndsOnKill(t *testing.T) {
	cfg, _ := testConfig(t)
	done := make(chan error, 1)
	ready := make(chan struct{})
	var t0 time.Time
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		go func() {
			<-ready // the step is running
			t0 = time.Now()
			rt.mu.Lock()
			rt.killed = true
			close(rt.killCh)
			rt.mu.Unlock()
		}()
		_, err := rt.Step(ctx, "ledger:w1", []byte("in"), func(sctx context.Context, _ bool) (json.RawMessage, error) {
			close(ready)
			select {
			case <-sctx.Done():
				done <- sctx.Err()
				return nil, sctx.Err()
			case <-time.After(5 * time.Second):
				done <- nil
				return nil, nil
			}
		})
		return nil, err
	})
	if err := <-done; err == nil {
		t.Fatal("the step's ctx did not end on killCh")
	}
	if d := time.Since(t0); d > 2*time.Second {
		t.Fatalf("the step took %v to end", d)
	}
}

// A repo call that returned null with dirt is not journaled; its re-run on
// resume reuses the clone, the dirt still in it, and reports it again.
func TestUncommittedNullReRunKeepsDirt(t *testing.T) {
	cfg, _ := testConfig(t)
	t.Setenv("STUB_SCRIPT", "invalid")
	t.Setenv("STUB_DIRTY", "half.txt")
	first, _ := call(t, cfg, vOpts)
	if first.Value != nil || !slices.Equal(first.Uncommitted, []string{"half.txt"}) {
		t.Fatalf("first %+v", first)
	}
	t.Setenv("STUB_SCRIPT", "ok")
	t.Setenv("STUB_DIRTY", "")
	again, _ := call(t, cfg, vOpts)
	if again.Replayed || !slices.Equal(again.Uncommitted, []string{"half.txt"}) {
		t.Fatalf("re-run %+v", again)
	}
}

// Denials from a retried attempt are kept with the final one's.
func TestDenialsAcrossAttempts(t *testing.T) {
	cfg, _ := testConfig(t)
	dir := t.TempDir()
	denied := filepath.Join(dir, "denied.jsonl")
	os.WriteFile(denied, []byte(`{"type":"system","subtype":"init","session_id":"s","tools":["Read"],"slash_commands":[]}`+"\n"+
		`{"type":"result","subtype":"success","is_error":false,"structured_output":{"wrong":1},"total_cost_usd":0,"permission_denials":[{"tool_name":"Bash","tool_input":{"command":"git commit"}}]}`+"\n"), 0o644)
	t.Setenv("STUB_SCRIPT", "file:"+denied+",ok")
	res, _ := call(t, cfg, vOpts)
	if res.Value == nil || !slices.Equal(res.Denials, []string{`Bash: {"command":"git commit"}`}) {
		t.Fatalf("%+v", res)
	}
}
