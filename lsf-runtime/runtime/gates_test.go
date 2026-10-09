package lsfrt

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// gateRepo is a repo whose HEAD holds a file and a .gitignore for junk/.
func gateRepo(t *testing.T, cfg *Config) string {
	t.Helper()
	os.WriteFile(filepath.Join(cfg.Repo, "f.txt"), []byte("tracked\n"), 0o644)
	os.WriteFile(filepath.Join(cfg.Repo, ".gitignore"), []byte("junk/\n"), 0o644)
	git(t, cfg.Repo, "add", ".")
	git(t, cfg.Repo, "commit", "-q", "-m", "files")
	return git(t, cfg.Repo, "rev-parse", "HEAD")
}

// runGates runs RunGates once under a launch.
func runGates(t *testing.T, cfg Config, label, commit, setup string, gates []GateSpec) ([]GateRun, error, Summary) {
	t.Helper()
	var runs []GateRun
	var err error
	s := launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		runs, err = rt.RunGates(ctx, label, commit, setup, gates)
		return nil, nil
	})
	return runs, err, s
}

func TestRunGatesStatuses(t *testing.T) {
	cfg, _ := testConfig(t)
	sha := gateRepo(t, &cfg)
	runs, err, _ := runGates(t, cfg, "gates:x", sha, "", []GateSpec{
		{Name: "ok", Command: "grep -q tracked f.txt && echo all good\necho"},
		{Name: "fails", Command: "echo first; echo 'last line'; exit 3"},
		{Name: "slow", Command: "sleep 5", Timeout: 200 * time.Millisecond},
		{Name: "after", Command: "true"},
		{Name: "shell killed", Command: "kill -KILL $$"},
		{Name: "child killed", Command: "sh -c 'kill -KILL $$'"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		status  string
		code    int
		summary string
		tail    bool
	}{
		{GateGreen, 0, "all good", false},
		{GateRed, 3, "last line", true},
		{GateNotRun, -1, "timed out after 200ms", false},
		{GateGreen, 0, "", false},
		{GateNotRun, -1, "killed by signal killed", false},
		{GateRed, 137, "", false}, // no output: no tail
	}
	for i, w := range want {
		r := runs[i]
		if r.Status != w.status || r.ExitCode != w.code || r.Summary != w.summary || (r.Tail != "") != w.tail {
			t.Errorf("run %d: %+v", i, r)
		}
		if _, err := os.Stat(r.Log); err != nil {
			t.Errorf("run %d: log: %v", i, err)
		}
	}
	if !strings.Contains(runs[1].Tail, "first\nlast line") {
		t.Errorf("tail %q", runs[1].Tail)
	}
	if runs[2].Secs > 3 {
		t.Errorf("timeout did not end the gate: %.1fs", runs[2].Secs)
	}
	if _, err := os.Stat(filepath.Join(cfg.StateDir, "gates", journalKey("gates:x"))); !os.IsNotExist(err) {
		t.Errorf("checkout left behind: %v", err)
	}
	if out := git(t, cfg.Repo, "worktree", "list"); strings.Count(out, "\n") != 0 {
		t.Errorf("worktree still registered:\n%s", out)
	}
}

func TestRunGatesSetupFailure(t *testing.T) {
	cfg, _ := testConfig(t)
	sha := gateRepo(t, &cfg)
	runs, err, _ := runGates(t, cfg, "gates:x", sha, "echo no deps; exit 2", []GateSpec{{Name: "a", Command: "true"}, {Name: "b", Command: "true"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range runs {
		if r.Status != GateNotRun || r.Summary != "setup failed (exit 2): no deps" {
			t.Errorf("%+v", r)
		}
	}
	// Setup runs in the checkout before the gates and its effects are seen.
	runs, _, _ = runGates(t, cfg, "gates:y", sha, "mkdir -p junk && touch junk/built", []GateSpec{{Name: "a", Command: "test -e junk/built"}})
	if runs[0].Status != GateGreen {
		t.Errorf("setup effect not seen: %+v", runs[0])
	}
}

// Each run is a clean checkout: untracked and ignored files a gate leaves
// are gone, a leftover directory is replaced, and a stale registration is
// pruned.
func TestRunGatesCleanCheckout(t *testing.T) {
	cfg, _ := testConfig(t)
	sha := gateRepo(t, &cfg)
	dirty := []GateSpec{{Name: "dirty", Command: "mkdir junk && touch junk/x untracked && echo changed > f.txt"}}
	clean := []GateSpec{{Name: "clean", Command: "test ! -e junk && test ! -e untracked && test \"$(cat f.txt)\" = tracked && test -z \"$(git status --porcelain)\""}}
	if runs, err, _ := runGates(t, cfg, "gates:x", sha, "", dirty); err != nil || runs[0].Status != GateGreen {
		t.Fatalf("%v %+v", err, runs)
	}
	if runs, _, _ := runGates(t, cfg, "gates:x", sha, "", clean); runs[0].Status != GateGreen {
		t.Fatalf("second run not clean: %+v", runs[0])
	}

	dir := filepath.Join(cfg.StateDir, "gates", journalKey("gates:x"))
	// A leftover registered worktree with changes in it.
	git(t, cfg.Repo, "worktree", "add", "--detach", dir, sha)
	os.WriteFile(filepath.Join(dir, "untracked"), nil, 0o644)
	if runs, err, _ := runGates(t, cfg, "gates:x", sha, "", clean); err != nil || runs[0].Status != GateGreen {
		t.Fatalf("leftover worktree: %v %+v", err, runs)
	}
	// A leftover plain directory, and a registration whose directory is gone.
	other := filepath.Join(cfg.StateDir, "gates", journalKey("gates:y"))
	git(t, cfg.Repo, "worktree", "add", "--detach", other, sha)
	os.RemoveAll(other)
	os.MkdirAll(filepath.Join(dir, "junk"), 0o755)
	if runs, err, _ := runGates(t, cfg, "gates:x", sha, "", clean); err != nil || runs[0].Status != GateGreen {
		t.Fatalf("leftover directory: %v %+v", err, runs)
	}
	if out := git(t, cfg.Repo, "worktree", "list"); strings.Contains(out, other) {
		t.Errorf("stale registration not pruned:\n%s", out)
	}
}

func TestRunGatesCheckoutFailure(t *testing.T) {
	cfg, _ := testConfig(t)
	gateRepo(t, &cfg)
	_, err, s := runGates(t, cfg, "gates:x", "0123456789abcdef0123456789abcdef01234567", "", []GateSpec{{Name: "a", Command: "true"}})
	if _, isStop := IsStop(err); err == nil || isStop || !strings.Contains(err.Error(), "gate checkout") {
		t.Fatalf("err %v", err)
	}
	if s.StopReason != StopReturned {
		t.Errorf("a checkout failure is an error, not a stop: %+v", s)
	}
}

// A stop mid-gate kills the gate's group, a TERM-ignoring grandchild
// included, and is returned with no runs.
func TestRunGatesStopKillsGroup(t *testing.T) {
	cfg, _ := testConfig(t)
	sha := gateRepo(t, &cfg)
	pidFile := filepath.Join(t.TempDir(), "pid")
	cmd := "trap '' TERM; (trap '' TERM; sleep 30) & echo $! > " + pidFile + "; sleep 30"
	var runs []GateRun
	var err error
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		go func() {
			for {
				if b, _ := os.ReadFile(pidFile); len(strings.TrimSpace(string(b))) > 0 {
					rt.setStop(StopRuntimeError, "test stop")
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
		}()
		runs, err = rt.RunGates(ctx, "gates:x", sha, "", []GateSpec{{Name: "hang", Command: cmd}})
		return nil, nil
	})
	if r, ok := IsStop(err); !ok || r != StopRuntimeError || runs != nil {
		t.Fatalf("err %v runs %v", err, runs)
	}
	b, _ := os.ReadFile(pidFile)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	waitFor(t, 3*time.Second, "grandchild to die", func() bool { return !alive(pid) })
	if _, err := os.Stat(filepath.Join(cfg.StateDir, "gates", journalKey("gates:x"))); !os.IsNotExist(err) {
		t.Errorf("checkout left behind after a stop: %v", err)
	}
}

// A gate's shell keeps its marker in its arguments, however simple the
// command, so the next launch reaps a gate a SIGKILLed launch left running.
func TestOrphanGateReaped(t *testing.T) {
	cfg, _ := testConfig(t)
	os.MkdirAll(cfg.StateDir, 0o755)
	marker := gateMarkerPrefix + "test-1"
	cmd := exec.Command("sh", "-c", gateScript, marker)
	cmd.Env = append(os.Environ(), "LSF_GATE_COMMAND=sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	go cmd.Wait()
	waitFor(t, 2*time.Second, "sleep under the marked shell", func() bool {
		out, _ := exec.Command("pgrep", "-g", strconv.Itoa(cmd.Process.Pid), "sleep").Output()
		return len(out) > 0
	})
	procs, err := processes()
	if err != nil {
		t.Fatal(err)
	}
	if !matchesChild(procs[cmd.Process.Pid], marker) {
		t.Fatalf("marker not in the shell's arguments: %q", procs[cmd.Process.Pid])
	}
	b, _ := json.Marshal(map[string]int{marker: cmd.Process.Pid})
	os.WriteFile(filepath.Join(cfg.StateDir, "children.json"), b, 0o644)
	launch(t, cfg, func(context.Context, *Runtime) (json.RawMessage, error) { return nil, nil })
	waitFor(t, 3*time.Second, "orphaned gate to die", func() bool { return !alive(cmd.Process.Pid) })
	if n := countEvents(telemetry(t, cfg), "orphan-terminated"); n != 1 {
		t.Errorf("%d orphan-terminated events", n)
	}
	if matchesChild("sh -c x lsf-gate:test-10", marker) || matchesChild("claude --session-id "+marker, "abc") {
		t.Error("marker matching is too loose")
	}
}

// A real gate is registered for reaping under a marker its live shell's
// arguments match; a TERM-ignoring command is still ended at its timeout,
// and a background process a green command left is killed.
func TestRunGatesRegistersAndCleansUp(t *testing.T) {
	cfg, _ := testConfig(t)
	cfg.KillGrace = Duration(200 * time.Millisecond)
	sha := gateRepo(t, &cfg)
	dir := t.TempDir()
	bg, ready := filepath.Join(dir, "bg"), filepath.Join(dir, "ready")
	var matched bool
	var runs []GateRun
	checked := make(chan struct{})
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		go func() {
			defer close(checked)
			waitFor(t, 5*time.Second, "gate to start", func() bool { _, err := os.Stat(ready); return err == nil })
			var kids map[string]int
			json.Unmarshal([]byte(readFile(t, cfg, "children.json")), &kids)
			procs, _ := processes()
			for marker, pid := range kids {
				matched = matched || (strings.HasPrefix(marker, gateMarkerPrefix) && matchesChild(procs[pid], marker))
			}
		}()
		var err error
		runs, err = rt.RunGates(ctx, "gates:x", sha, "", []GateSpec{
			{Name: "stubborn", Command: "touch " + ready + "; trap '' TERM; sleep 30", Timeout: 700 * time.Millisecond},
			{Name: "leaves", Command: "sleep 30 & echo $! > " + bg},
		})
		return nil, err
	})
	<-checked
	if !matched {
		t.Error("no live gate shell matched its children.json marker")
	}
	if runs[0].Status != GateNotRun || runs[0].Secs > 5 || runs[1].Status != GateGreen {
		t.Fatalf("%+v", runs)
	}
	b, _ := os.ReadFile(bg)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	waitFor(t, 2*time.Second, "background process to die", func() bool { return !alive(pid) })
	if strings.Contains(readFile(t, cfg, "children.json"), gateMarkerPrefix) {
		t.Error("finished gates left in children.json")
	}
}

// gateConcurrency 1 serialises concurrent runs.
func TestRunGatesConcurrency(t *testing.T) {
	cfg, _ := testConfig(t)
	sha := gateRepo(t, &cfg)
	busy := filepath.Join(t.TempDir(), "busy")
	gate := []GateSpec{{Name: "excl", Command: "mkdir " + busy + " || exit 9; sleep 0.3; rmdir " + busy}}
	var mu sync.Mutex
	var all []GateRun
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		var wg sync.WaitGroup
		for i := range 3 {
			wg.Go(func() {
				runs, err := rt.RunGates(ctx, "gates:"+strconv.Itoa(i), sha, "", gate)
				if err != nil {
					t.Error(err)
				}
				mu.Lock()
				all = append(all, runs...)
				mu.Unlock()
			})
		}
		wg.Wait()
		return nil, nil
	})
	for _, r := range all {
		if r.Status != GateGreen {
			t.Errorf("overlapping runs: %+v", r)
		}
	}
}

func TestGateConfigDefaults(t *testing.T) {
	cfg, _ := testConfig(t)
	if err := cfg.prepare(); err != nil {
		t.Fatal(err)
	}
	if cfg.GateTimeout.D() != 2*time.Hour || cfg.GateConcurrency != 1 {
		t.Errorf("defaults: %v %d", cfg.GateTimeout.D(), cfg.GateConcurrency)
	}
	bad, _ := testConfig(t)
	bad.GateConcurrency = -1
	if err := bad.prepare(); err == nil || !strings.Contains(err.Error(), "gateConcurrency") {
		t.Errorf("negative gateConcurrency: %v", err)
	}
	bad, _ = testConfig(t)
	bad.GateTimeout = -1
	if err := bad.prepare(); err == nil || !strings.Contains(err.Error(), "durations") {
		t.Errorf("negative gateTimeout: %v", err)
	}
}

// The gate checkout is a plain clone, not a worktree: its .git is a
// directory, and the main repo lists no gate worktree during or after the
// run. A commit reachable only from a refs/lsf transfer ref, or from no ref
// at all, is gated.
func TestRunGatesPlainClone(t *testing.T) {
	cfg, _ := testConfig(t)
	gateRepo(t, &cfg)
	git(t, cfg.Repo, "checkout", "-q", "-b", "candidate")
	os.WriteFile(filepath.Join(cfg.Repo, "cand.txt"), []byte("candidate\n"), 0o644)
	git(t, cfg.Repo, "add", "cand.txt")
	git(t, cfg.Repo, "commit", "-q", "-m", "candidate")
	xfer := git(t, cfg.Repo, "rev-parse", "HEAD")
	git(t, cfg.Repo, "update-ref", "refs/lsf/01234567/xfer/w1", xfer)
	os.WriteFile(filepath.Join(cfg.Repo, "cand.txt"), []byte("dangling\n"), 0o644)
	git(t, cfg.Repo, "commit", "-q", "-am", "dangling")
	dangling := git(t, cfg.Repo, "rev-parse", "HEAD")
	git(t, cfg.Repo, "checkout", "-q", "main")
	git(t, cfg.Repo, "branch", "-q", "-D", "candidate")
	git(t, cfg.Repo, "reflog", "expire", "--expire=now", "--all")

	listed := filepath.Join(t.TempDir(), "worktrees")
	for _, c := range []struct{ commit, want string }{{xfer, "candidate"}, {dangling, "dangling"}} {
		runs, err, _ := runGates(t, cfg, "gates:x", c.commit, "", []GateSpec{
			{Name: "plain", Command: "test -d .git && test \"$(cat cand.txt)\" = " + c.want + " && test \"$(git rev-parse HEAD)\" = " + c.commit},
			{Name: "unlisted", Command: "git -C " + cfg.Repo + " worktree list --porcelain > " + listed},
		})
		if err != nil || runs[0].Status != GateGreen || runs[1].Status != GateGreen {
			t.Fatalf("%s: %v %+v", c.want, err, runs)
		}
		if b, _ := os.ReadFile(listed); strings.Count(string(b), "worktree ") != 1 {
			t.Errorf("%s: a gate worktree is registered during the run:\n%s", c.want, b)
		}
	}
	if out := git(t, cfg.Repo, "worktree", "list", "--porcelain"); strings.Count(out, "worktree ") != 1 {
		t.Errorf("a gate worktree is registered after the run:\n%s", out)
	}
	if entries, _ := os.ReadDir(filepath.Join(cfg.StateDir, "gates")); len(entries) != 1 || entries[0].Name() != "logs" {
		t.Errorf("gate checkouts left: %v", entries)
	}
}

// A gate checkout half-built by a killed launch goes at the next launch.
func TestGatePartialWiped(t *testing.T) {
	cfg, _ := testConfig(t)
	partial := filepath.Join(cfg.StateDir, "gates", "gates_x-0123"+partialSuffix)
	os.MkdirAll(partial, 0o755)
	launch(t, cfg, func(context.Context, *Runtime) (json.RawMessage, error) { return nil, nil })
	if exists(partial) {
		t.Error("partial gate checkout kept")
	}
}

// A commit named by a branch gates; so does one in a SHA-256 repository,
// whose gate checkout takes its object format.
func TestRunGatesCommitForms(t *testing.T) {
	cfg, _ := testConfig(t)
	gateRepo(t, &cfg)
	if runs, err, _ := runGates(t, cfg, "gates:x", "main", "", []GateSpec{{Name: "a", Command: "test -e f.txt"}}); err != nil || runs[0].Status != GateGreen {
		t.Fatalf("branch name: %v %+v", err, runs)
	}
	cfg, _ = testConfig(t)
	cfg.Repo = t.TempDir()
	git(t, cfg.Repo, "init", "-q", "-b", "main", "--object-format=sha256")
	sha := gateRepo(t, &cfg)
	if len(sha) != 64 {
		t.Fatalf("not a SHA-256 repo: %s", sha)
	}
	if runs, err, _ := runGates(t, cfg, "gates:x", sha, "", []GateSpec{{Name: "a", Command: "test \"$(git rev-parse HEAD)\" = " + sha}}); err != nil || runs[0].Status != GateGreen {
		t.Fatalf("sha256: %v %+v", err, runs)
	}
}
