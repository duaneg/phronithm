package lsfrt

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
		{Name: "silent", Command: "exit 4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		status  string
		code    int
		summary string
		tail    bool
		// The shell's own report of a killed child varies (bash prints
		// nothing, dash prints "Killed"), so only status and code count.
		anyOutput bool
	}{
		{GateGreen, 0, "all good", false, false},
		{GateRed, 3, "last line", true, false},
		{GateNotRun, -1, "timed out after 200ms", false, false},
		{GateGreen, 0, "", false, false},
		{GateNotRun, -1, "killed by signal killed", false, false},
		{GateRed, 137, "", false, true},
		{GateRed, 4, "", false, false}, // no output: no tail
	}
	for i, w := range want {
		r := runs[i]
		output := w.anyOutput || (r.Summary == w.summary && (r.Tail != "") == w.tail)
		if r.Status != w.status || r.ExitCode != w.code || !output {
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

// A stop mid-gate removes the gate's container, which ends a
// TERM-ignoring grandchild, and is returned with no runs.
func TestRunGatesStopKillsGroup(t *testing.T) {
	cfg, stub := testConfig(t)
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
	if !strings.Contains(podmanCalls(stub), "rm -f -t 0 --ignore "+flagValue(gateRuns(t, stub)[0], "--name")) {
		t.Error("stopped gate's container not removed")
	}
	if _, err := os.Stat(filepath.Join(cfg.StateDir, "gates", journalKey("gates:x"))); !os.IsNotExist(err) {
		t.Errorf("checkout left behind after a stop: %v", err)
	}
}

// gateRuns are the stub podman's recorded gate runs, in order.
func gateRuns(t *testing.T, stub string) [][]string {
	t.Helper()
	var runs [][]string
	for n := 1; ; n++ {
		b, err := os.ReadFile(filepath.Join(stub, "podman", "run."+strconv.Itoa(n)))
		if err != nil {
			return runs
		}
		if a := strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00"); strings.Contains(flagValue(a, "--name"), "-gate-") {
			runs = append(runs, a)
		}
	}
}

// podmanCalls are the stub podman's recorded non-run invocations.
func podmanCalls(stub string) string {
	b, _ := os.ReadFile(filepath.Join(stub, "podman", "calls"))
	return string(b)
}

// A gate that times out has its container removed; one that leaves a
// background process behind has it ended.
func TestRunGatesTimeoutRemovesContainer(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.KillGrace = Duration(200 * time.Millisecond)
	sha := gateRepo(t, &cfg)
	bg := filepath.Join(t.TempDir(), "bg")
	runs, err, _ := runGates(t, cfg, "gates:x", sha, "", []GateSpec{
		{Name: "stubborn", Command: "trap '' TERM; sleep 30", Timeout: 700 * time.Millisecond},
		{Name: "leaves", Command: "sleep 30 & echo $! > " + bg},
	})
	if err != nil || runs[0].Status != GateNotRun || runs[0].Secs > 5 || runs[1].Status != GateGreen {
		t.Fatalf("%v %+v", err, runs)
	}
	name := flagValue(gateRuns(t, stub)[0], "--name")
	if !strings.Contains(podmanCalls(stub), "rm -f -t 0 --ignore "+name+"\n") {
		t.Errorf("timed-out gate %s not removed:\n%s", name, podmanCalls(stub))
	}
	b, _ := os.ReadFile(bg)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	waitFor(t, 2*time.Second, "background process to die", func() bool { return !alive(pid) })
}

// setup and the gates run in named, labelled containers: setup spliced to
// the egress proxy with hosts pinned and with no network otherwise, gate
// commands with no network; both bind the checkout and the shared home at
// their host paths, and the home is removed with the checkout.
func TestRunGatesContainers(t *testing.T) {
	for _, hosts := range [][]string{nil, {"registry.npmjs.org"}} {
		cfg, stub := testConfig(t)
		sha := gateRepo(t, &cfg)
		var port string
		var runs []GateRun
		var err error
		launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
			rt.PinHosts(hosts)
			port = rt.egressPort()
			runs, err = rt.RunGates(ctx, "gates:x", sha, "echo dep > \"$HOME/x\"", []GateSpec{{Name: "a", Command: "test \"$(cat \"$HOME/x\")\" = dep && test ! -t 0"}})
			return nil, err
		})
		if err != nil || runs[0].Status != GateGreen {
			t.Fatalf("hosts %v: %v %+v", hosts, err, runs)
		}
		dir := filepath.Join(cfg.StateDir, "gates", journalKey("gates:x"))
		home := dir + ".home"
		h, _ := stateHashOf(cfg.StateDir)
		argvs := gateRuns(t, stub)
		if len(argvs) != 2 {
			t.Fatalf("%d gate runs", len(argvs))
		}
		for i, a := range argvs {
			net := flagValue(a, "--network")
			switch {
			case i == 0 && hosts != nil:
				if net != "pasta:--splice-only,-T,"+port || !slices.Contains(a, "HTTPS_PROXY=http://127.0.0.1:"+port) {
					t.Errorf("setup with hosts: %q", a)
				}
			case net != "none" || slices.ContainsFunc(a, func(s string) bool { return strings.Contains(s, "_PROXY=http") || strings.Contains(s, "_proxy=http") }):
				t.Errorf("run %d (hosts %v): network %q: %q", i, hosts, net, a)
			}
			if !slices.Contains(a, dir+":"+dir+":rw") || !slices.Contains(a, home+":"+home+":rw") || flagValue(a, "-w") != dir ||
				!slices.Contains(a, "HOME="+home) || !slices.Contains(a, "lsf.state="+h) || slices.Contains(a, "-i") {
				t.Errorf("run %d: %q", i, a)
			}
		}
		if exists(dir) || exists(home) {
			t.Errorf("checkout or home left")
		}
	}
}

// A tree written read-only, as Go writes its module cache, is removed with
// the home; so is a home a gate made unreadable.
func TestRunGatesRemovesReadOnlyTrees(t *testing.T) {
	cfg, _ := testConfig(t)
	sha := gateRepo(t, &cfg)
	runs, err, _ := runGates(t, cfg, "gates:x", sha,
		"mkdir -p \"$HOME/mod/a/b\" junk/c && touch \"$HOME/mod/a/b/f\" junk/c/f && chmod -R a-w \"$HOME/mod\" junk && chmod 0 \"$HOME/mod/a\"",
		[]GateSpec{{Name: "a", Command: "true"}})
	if err != nil || runs[0].Status != GateGreen {
		t.Fatalf("%v %+v", err, runs)
	}
	dir := filepath.Join(cfg.StateDir, "gates", journalKey("gates:x"))
	if exists(dir) || exists(dir+".home") {
		t.Error("read-only tree left")
	}
}

// A container that fails before its command starts (no started marker)
// stops the run with runtime-error and returns no verdict; a command that
// itself exits 125 is red, and so is a command that made its home
// unreadable (TestRunGatesUnreadableHomeIsRed).
func TestRunGatesLaunchFailure(t *testing.T) {
	cfg, _ := testConfig(t)
	sha := gateRepo(t, &cfg)
	if runs, err, _ := runGates(t, cfg, "gates:x", sha, "", []GateSpec{{Name: "a", Command: "exit 125"}}); err != nil || runs[0].Status != GateRed || runs[0].ExitCode != 125 {
		t.Fatalf("command exit 125: %v %+v", err, runs)
	}
	t.Setenv("STUB_PODMAN_RUN_EXIT", "125")
	runs, err, s := runGates(t, cfg, "gates:y", sha, "", []GateSpec{{Name: "a", Command: "true"}})
	if r, ok := IsStop(err); !ok || r != StopRuntimeError || runs != nil || s.StopReason != StopRuntimeError || !strings.Contains(s.Detail, "before its command started") {
		t.Fatalf("podman exit 125: %v %+v %+v", err, runs, s)
	}
}

// A gate container whose removal fails stops the run with runtime-error,
// and its checkout and home are kept while it may run.
func TestRunGatesContainerLeft(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.KillGrace = Duration(200 * time.Millisecond)
	sha := gateRepo(t, &cfg)
	t.Setenv("STUB_PODMAN_DETACH", "1")
	t.Setenv("STUB_PODMAN_RM_EXIT", "1")
	t.Cleanup(func() { killStubs(os.Getenv("STUB_STATE")) })
	runs, err, s := runGates(t, cfg, "gates:x", sha, "", []GateSpec{{Name: "a", Command: "sleep 30", Timeout: 300 * time.Millisecond}})
	var left *containerLeftError
	if r, ok := IsStop(err); !ok || r != StopRuntimeError || !errors.As(err, &left) || runs != nil || s.StopReason != StopRuntimeError {
		t.Fatalf("%v %+v %+v", err, runs, s)
	}
	dir := filepath.Join(cfg.StateDir, "gates", journalKey("gates:x"))
	if !exists(dir) || !exists(dir+".home") {
		t.Error("checkout removed while its container may run")
	}
	if !strings.Contains(readFile(t, cfg, "log.txt"), left.name) {
		t.Error("the log does not name the container")
	}
	_ = stub
}

// A gate container a SIGKILLed launch left running is removed by the next
// launch's reaping by label, and the checkout and home it left are wiped.
func TestOrphanGateContainerReaped(t *testing.T) {
	cfg, stub := testConfig(t)
	sha := gateRepo(t, &cfg)
	t.Setenv("LSFRT_GATE_COMMIT", sha)
	cmd := startHelper(t, cfg, "gate-hang")
	waitFor(t, 10*time.Second, "the gate container", func() bool { return len(liveCalls(t, stub)) == 1 })
	name := liveCalls(t, stub)[0]
	b, _ := os.ReadFile(filepath.Join(stub, "podman", "name."+name))
	pid, _ := strconv.Atoi(strings.SplitN(string(b), "\n", 2)[0])
	cmd.Process.Signal(syscall.SIGKILL)
	cmd.Wait()
	if !strings.Contains(name, "-gate-") || !alive(pid) {
		t.Fatalf("%s (pid %d) not a live gate container", name, pid)
	}
	launch(t, cfg, func(context.Context, *Runtime) (json.RawMessage, error) { return nil, nil })
	waitFor(t, 3*time.Second, "the orphaned gate to die", func() bool { return !alive(pid) })
	if countEvents(telemetry(t, cfg), "orphan-terminated") != 1 {
		t.Error("no orphan-terminated event")
	}
	if entries, _ := os.ReadDir(filepath.Join(cfg.StateDir, "gates")); len(entries) != 1 || entries[0].Name() != "logs" {
		t.Errorf("leftover gate trees: %v", entries)
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

// hostileGate commits a gitlink to a nested repo and a .gitattributes,
// then configures filters (the checkout's and the nested repo's) and
// core.hooksPath that touch canary, and dirties the tree, so that host git
// run in the tree afterwards would touch canary.
func hostileGate(canary string) string {
	return `set -e
g() { git -c user.name=t -c user.email=t@example.invalid -c core.hooksPath=/dev/null "$@"; }
git init -q sub
echo d > sub/f
g -C sub add -A
g -C sub commit -q -m sub
echo '* filter=x' > .gitattributes
g add .gitattributes sub
g commit -q -m hostile
git config filter.x.clean 'touch ` + canary + `; cat'
git config filter.x.smudge 'touch ` + canary + `; cat'
printf '[filter "x"]\n\tclean = touch %s; cat\n' '` + canary + `' >> sub/.git/config
echo '* filter=x' > sub/.gitattributes
mkdir hooks
for h in post-checkout pre-commit post-index-change reference-transaction; do
  printf '#!/bin/sh\ntouch %s\n' '` + canary + `' > hooks/$h
  chmod +x hooks/$h
done
git config core.hooksPath "$PWD/hooks"
echo more >> f.txt
echo more >> sub/f
test ! -e '` + canary + `'
`
}

// No host git runs in a gate checkout once a container has run there: a
// gate that writes a filter and core.hooksPath into the checkout's
// .git/config and commits a gitlink finds no canary through the
// checkout's removal. The control shows host git in a tree prepared alike
// does touch it.
func TestRunGatesHostileGit(t *testing.T) {
	cfg, _ := testConfig(t)
	sha := gateRepo(t, &cfg)
	tmp := t.TempDir()

	ctl, ctlCanary := filepath.Join(tmp, "ctl"), filepath.Join(tmp, "ctl-canary")
	git(t, tmp, "clone", "-q", cfg.Repo, ctl)
	cmd := exec.Command("sh", "-c", hostileGate(ctlCanary))
	cmd.Dir = ctl
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("control setup: %v %s", err, out)
	}
	git(t, ctl, "status", "--porcelain")
	if !exists(ctlCanary) {
		t.Fatal("control: host git status did not touch the canary; the test cannot fail")
	}

	canary := filepath.Join(tmp, "canary")
	runs, err, _ := runGates(t, cfg, "gates:x", sha, hostileGate(canary), []GateSpec{{Name: "after", Command: "test ! -e " + canary}})
	if err != nil || runs[0].Status != GateGreen {
		t.Fatalf("%v %+v", err, runs)
	}
	if exists(canary) {
		t.Error("host git ran in the gate checkout")
	}
	if dir := filepath.Join(cfg.StateDir, "gates", journalKey("gates:x")); exists(dir) {
		t.Error("checkout left")
	}

	// The timeout path, and a hostile tree a killed launch left, wiped by
	// the next launch.
	runs, err, _ = runGates(t, cfg, "gates:y", sha, hostileGate(canary), []GateSpec{{Name: "slow", Command: "sleep 30", Timeout: 300 * time.Millisecond}})
	if err != nil || runs[0].Status != GateNotRun {
		t.Fatalf("%v %+v", err, runs)
	}
	left := filepath.Join(cfg.StateDir, "gates", "gates_z-01234567")
	git(t, tmp, "clone", "-q", cfg.Repo, left)
	cmd = exec.Command("sh", "-c", hostileGate(canary))
	cmd.Dir = left
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("leftover setup: %v %s", err, out)
	}
	launch(t, cfg, func(context.Context, *Runtime) (json.RawMessage, error) { return nil, nil })
	if exists(left) {
		t.Error("leftover hostile tree not wiped")
	}
	if exists(canary) {
		t.Error("host git ran in a gate checkout on the timeout or wipe path")
	}
}

// A gate that makes its home unreadable after the marker is written, then
// fails, is red: the marker is read after the home is made readable.
func TestRunGatesUnreadableHomeIsRed(t *testing.T) {
	cfg, _ := testConfig(t)
	sha := gateRepo(t, &cfg)
	runs, err, _ := runGates(t, cfg, "gates:x", sha, "", []GateSpec{{Name: "a", Command: `chmod 0 "$HOME"; exit 3`}})
	if err != nil || runs[0].Status != GateRed || runs[0].ExitCode != 3 {
		t.Fatalf("%v %+v", err, runs)
	}
	if dir := filepath.Join(cfg.StateDir, "gates", journalKey("gates:x")); exists(dir + ".home") {
		t.Error("home left")
	}
}
