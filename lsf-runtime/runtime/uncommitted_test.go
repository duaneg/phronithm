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

func TestUncommittedIsolated(t *testing.T) {
	cfg, _ := isolatedConfig(t)
	commitFile(t, cfg.Repo, "tracked.txt", "a\n")
	commitFile(t, cfg.Repo, ".gitignore", "ignored.txt\n")
	// The stub runs in the worktree: a tracked edit, an untracked file and
	// an ignored one.
	t.Setenv("STUB_SCRIPT", "modify:tracked.txt")
	o := vOpts
	o.Isolation = "worktree"
	var res CallResult
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		wt, _, err := rt.Worktree(ctx, o.Label)
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(wt, "new.txt"), []byte("n"), 0o644)
		os.WriteFile(filepath.Join(wt, "ignored.txt"), []byte("i"), 0o644)
		res, err = rt.Call(ctx, "do the thing", o)
		return nil, err
	})
	if !slices.Equal(res.Uncommitted, []string{"new.txt", "tracked.txt"}) || slices.Contains(res.Flags, FlagUncommittedUnchecked) {
		t.Fatalf("%+v", res)
	}

	// A resume replays the call and re-checks its worktree: cleaned, it is clean.
	wt := filepath.Join(cfg.StateDir, "wt", journalKey(o.Label))
	git(t, wt, "checkout", "--", "tracked.txt")
	os.Remove(filepath.Join(wt, "new.txt"))
	again, _ := call(t, cfg, o)
	if !again.Replayed || len(again.Uncommitted) != 0 {
		t.Fatalf("replay after cleaning %+v", again)
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

func TestUncommittedRepoCallComparesBefore(t *testing.T) {
	cfg, _ := testConfig(t)
	commitFile(t, cfg.Repo, "tracked.txt", "a\n")
	commitFile(t, cfg.Repo, "other.txt", "o\n")
	// Dirt before the call: an untracked file and a modified tracked one
	// (which the call changes further), and a modified one it leaves alone.
	os.WriteFile(filepath.Join(cfg.Repo, "before.txt"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(cfg.Repo, "tracked.txt"), []byte("a\nb\n"), 0o644)
	os.WriteFile(filepath.Join(cfg.Repo, "other.txt"), []byte("o\np\n"), 0o644)
	t.Setenv("STUB_SCRIPT", "modify:tracked.txt")
	res, _ := call(t, cfg, vOpts)
	if !slices.Equal(res.Uncommitted, []string{"tracked.txt"}) {
		t.Fatalf("further change only: %+v", res)
	}

	// A replay keeps the journaled paths that are still dirty.
	again, _ := call(t, cfg, vOpts)
	if !again.Replayed || !slices.Equal(again.Uncommitted, []string{"tracked.txt"}) {
		t.Fatalf("replay %+v", again)
	}
	git(t, cfg.Repo, "checkout", "--", "tracked.txt")
	clean, _ := call(t, cfg, vOpts)
	if !clean.Replayed || len(clean.Uncommitted) != 0 {
		t.Fatalf("replay after commit or removal %+v", clean)
	}
}

func TestUncommittedBeforeStateTakenOnce(t *testing.T) {
	cfg, stub := testConfig(t)
	// The first attempt dirties the tree and fails transiently; the retry
	// succeeds. The dirt is the call's.
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
	snap, err := rt.takeSnapshot(context.Background(), cfg.Repo, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap) != 1 || snap["new.txt"].orig != "old.txt" || snap["new.txt"].xy[0] != 'R' {
		t.Fatalf("%+v", snap)
	}
}

func TestUncommittedSkipsStateDir(t *testing.T) {
	cfg, _ := isolatedConfig(t)
	commitFile(t, cfg.Repo, ".keep", "")
	cfg.StateDir = filepath.Join(cfg.Repo, ".lsf-state")
	t.Setenv("STUB_SCRIPT", "ok")
	// A repo call (journal, telemetry, logs under the state dir) and an
	// isolated one (its worktree under it).
	res, _ := call(t, cfg, vOpts)
	o := vOpts
	o.Label = "plan:u1"
	o.Isolation = "worktree"
	iso, _ := call(t, cfg, o)
	if len(res.Uncommitted) != 0 || len(iso.Uncommitted) != 0 || slices.Contains(res.Flags, FlagUncommittedUnchecked) {
		t.Fatalf("repo %+v isolated %+v", res, iso)
	}
	rt := &Runtime{cfg: cfg, killCh: make(chan struct{})}
	if got := rt.stateDirIn(cfg.Repo); got != ".lsf-state" {
		t.Fatalf("stateDirIn %q", got)
	}
}

func TestUncommittedUncheckedNotJournaled(t *testing.T) {
	cfg, _ := testConfig(t)
	cfg.Repo = t.TempDir() // not a repository: the check fails
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(cfg.Repo))
	t.Setenv("STUB_SCRIPT", "ok")
	res, _ := call(t, cfg, vOpts)
	if !slices.Contains(res.Flags, FlagUncommittedUnchecked) || len(res.Uncommitted) != 0 {
		t.Fatalf("%+v", res)
	}
	b, err := os.ReadFile(filepath.Join(cfg.StateDir, "journal", journalKey(vOpts.Label)+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var r record
	if err := json.Unmarshal(b, &r); err != nil || slices.Contains(r.Flags, FlagUncommittedUnchecked) || !r.Unchecked {
		t.Fatalf("record %s", b)
	}
	// A replay checks afresh: still not a repository, still unchecked.
	again, _ := call(t, cfg, vOpts)
	if !again.Replayed || !slices.Contains(again.Flags, FlagUncommittedUnchecked) {
		t.Fatalf("replay %+v", again)
	}
}

// A repo call whose check failed replays with every dirty path, having no
// before-state to compare.
func TestUncommittedUncheckedReplayRepo(t *testing.T) {
	cfg, _ := testConfig(t)
	t.Setenv("STUB_SCRIPT", "ok")
	key := journalKey(vOpts.Label)
	os.MkdirAll(filepath.Join(cfg.StateDir, "journal"), 0o755)
	rec := record{Label: vOpts.Label, Kind: "call", Hash: callHash("do the thing", "", vOpts), Ret: json.RawMessage(`{"verdict":"pass","n":3}`), Unchecked: true}
	b, _ := json.Marshal(rec)
	os.WriteFile(filepath.Join(cfg.StateDir, "journal", key+".json"), b, 0o644)
	os.WriteFile(filepath.Join(cfg.Repo, "left.txt"), []byte("x"), 0o644)
	res, _ := call(t, cfg, vOpts)
	if !res.Replayed || !slices.Equal(res.Uncommitted, []string{"left.txt"}) {
		t.Fatalf("%+v", res)
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

// A state dir reached through a symlink, inside the repository, is still
// excluded.
func TestUncommittedSkipsSymlinkedStateDir(t *testing.T) {
	cfg, _ := testConfig(t)
	commitFile(t, cfg.Repo, ".keep", "")
	real := filepath.Join(cfg.Repo, "state")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "state-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	cfg.StateDir = link
	t.Setenv("STUB_SCRIPT", "ok")
	res, _ := call(t, cfg, vOpts)
	if len(res.Uncommitted) != 0 || slices.Contains(res.Flags, FlagUncommittedUnchecked) {
		t.Fatalf("%+v", res)
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
// resume still reports the dirt, which its before-state would otherwise
// hold. Once recorded, the kept paths are forgotten.
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
	if _, err := os.Stat(filepath.Join(cfg.StateDir, "journal", journalKey(vOpts.Label)+".dirt")); !os.IsNotExist(err) {
		t.Errorf("kept paths not forgotten after the record: %v", err)
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
