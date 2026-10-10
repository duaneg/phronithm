package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/internal/testcfg"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// outcomeStatus is the outcome's status field.
func outcomeStatus(t *testing.T, r report) string {
	t.Helper()
	var o struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(r.Outcome, &o); err != nil {
		t.Fatalf("outcome %q: %v", r.Outcome, err)
	}
	return o.Status
}

// (a) A launch paused before it starts ends at load with no call; launch
// exits 0 whichever of its exit and the driver's comes first. (b) A
// later launch refused by the launch check exits 1 with the runtime's
// message, leaves launch.json as it was, and status reports it
// failed-to-start rather than the earlier launch's end; a dry run refuses
// the same way and leaves the run's files alone.
func TestLaunchPausedThenRefused(t *testing.T) {
	f := newFixture(t)
	if r, _ := f.status(); r.Driver != driverNone {
		t.Fatalf("before any launch: %s", r.Driver)
	}
	_, code := f.status()
	eq(t, code, exitNoLaunch)
	f.git("update-ref", "refs/lsf/f/pause", "HEAD")
	r := f.launch(nil)
	eq(t, r.code, 0, r.stderr)
	var started struct {
		StateDir string `json:"stateDir"`
		PID      int    `json:"pid"`
		Launch   string `json:"launch"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &started); err != nil {
		t.Fatalf("%q: %v", r.stdout, err)
	}
	eq(t, started.StateDir, f.stateDir())
	if started.PID == 0 || started.Launch == "" {
		t.Errorf("launch output %s", r.stdout)
	}
	w, code := f.decode(f.cli(nil, "watch", "-state", f.stateDir()))
	eq(t, code, 0)
	eq(t, w.Driver, driverEnded)
	eq(t, outcomeStatus(t, w), "paused")
	eq(t, w.Summary.Calls, 0)
	eq(t, w.Summary.StopReason, "returned")
	eq(t, w.Pause.Set, true)
	eq(t, w.Name, "f")
	eq(t, w.Repo, f.repo)
	s, _ := f.status()
	eq(t, s.Driver, driverEnded)
	launchJSON := readFile(t, filepath.Join(f.stateDir(), launchFile))
	if m, _ := filepath.Glob(filepath.Join(f.stateDir(), "launch.*.json")); len(m) != 0 {
		t.Errorf("composed files left: %v", m)
	}

	refused := []string{"STUB_PODMAN_NOIMAGE=1"}
	r = f.launch(refused)
	eq(t, r.code, exitError)
	contains(t, r.stderr, "image localhost/lsf-test@sha256:")
	contains(t, r.stderr, "is not present")
	s, code = f.status()
	eq(t, s.Driver, driverFailed)
	eq(t, code, exitCrashed)
	if s.Summary != nil || s.Outcome != nil {
		t.Error("the earlier launch's outcome is reported as the current one")
	}
	contains(t, s.DriverLog, "is not present")
	eq(t, readFile(t, filepath.Join(f.stateDir(), launchFile)), launchJSON, "launch.json unchanged")
	if m, _ := filepath.Glob(filepath.Join(f.stateDir(), "launch.*.json")); len(m) != 0 {
		t.Errorf("a refused launch's composed file is left: %v", m)
	}

	files := map[string]string{}
	for _, n := range []string{launchFile, driverFile, "status.json", "summary.json"} {
		files[n] = readFile(t, filepath.Join(f.stateDir(), n))
	}
	r = f.launch(refused, "-dry-run")
	eq(t, r.code, exitError)
	contains(t, r.stderr, "is not present")
	for n, want := range files {
		eq(t, readFile(t, filepath.Join(f.stateDir(), n)), want, n+" unchanged by a dry run")
	}
	r = f.launch(nil, "-dry-run")
	eq(t, r.code, 0, r.stderr)
	contains(t, r.stdout, `"stop_reason": "dry-run"`)
}

// (c) A config from before the container boundary is refused with the
// hint, before any driver starts.
func TestLaunchOldConfig(t *testing.T) {
	f := newFixture(t)
	f.writeHost(M{"allow": []string{"Bash(git:*)"}})
	r := f.launch(nil)
	eq(t, r.code, exitError)
	contains(t, r.stderr, `unknown field "allow"`)
	contains(t, r.stderr, "predates the container boundary")
	if _, err := os.Stat(filepath.Join(f.stateDir(), driverFile)); !os.IsNotExist(err) {
		t.Errorf("a driver was started: %v", err)
	}
}

func TestLaunchRefusesOldGit(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "git"), []byte("#!/bin/sh\n[ \"$1\" = version ] && { echo 'git version 2.37.1'; exit 0; }\nexec "+gitPath(t)+" \"$@\"\n"), 0o755)
	r := f.launch([]string{"PATH=" + dir + ":" + os.Getenv("PATH")})
	eq(t, r.code, exitError)
	contains(t, r.stderr, "2.37.1 is older than 2.38")
}

// (d) The driver outlives launch, in a session of its own; (g) a second
// launch meanwhile is refused and changes nothing; (e) status lists the
// call's container; (f) a killed driver is reported crashed, exit 3.
func TestLaunchDetachedCrash(t *testing.T) {
	f := newFixture(t)
	hang := []string{"STUB_MODE=hang"}
	r := f.launch(hang)
	d, err := readDriver(f.stateDir())
	if err == nil {
		t.Cleanup(func() { killRun(t, f, d.PID) })
	}
	eq(t, r.code, 0, r.stderr)
	if err != nil {
		t.Fatal(err)
	}
	sid, ours := sessionOf(t, d.PID), sessionOf(t, os.Getpid())
	if sid != d.PID || sid == ours {
		t.Errorf("driver %d session %d; the test's %d", d.PID, sid, ours)
	}
	// launch has exited; the driver runs on, its call hanging.
	waitFor(t, 20*time.Second, "the call's container", func() bool {
		s, _ := f.status()
		return len(s.Containers) > 0
	})
	s, _ := f.status()
	eq(t, s.Driver, driverRunning)
	eq(t, s.PID, d.PID)
	if !isDriver(d.PID, d.State) {
		t.Error("the driver is not running after launch returned")
	}

	before := map[string]string{}
	for _, n := range []string{launchFile, driverFile} {
		before[n] = readFile(t, filepath.Join(f.stateDir(), n))
	}
	r = f.launch(hang)
	eq(t, r.code, exitError)
	contains(t, r.stderr, "a driver is running on")
	for n, want := range before {
		eq(t, readFile(t, filepath.Join(f.stateDir(), n)), want, n+" unchanged by a refused launch")
	}

	syscall.Kill(d.PID, syscall.SIGKILL)
	w, code := f.decode(f.cli(nil, "watch", "-state", f.stateDir()))
	eq(t, code, exitCrashed)
	eq(t, w.Driver, driverCrashed)
	if w.Summary != nil {
		t.Error("a crashed run reports a summary")
	}
}

// killRun ends a test's driver and the calls it left.
func killRun(t *testing.T, f *fixture, pid int) {
	syscall.Kill(pid, syscall.SIGKILL)
	s, _ := f.status()
	if len(s.Containers) > 0 {
		args := append([]string{"rm", "-f", "-t", "0"}, s.Containers...)
		runStub(t, args...)
	}
}

// (e) Status lists what a fault left: clone, scratch and merge entries,
// half-built directories, xfer refs, and labelled containers.
func TestStatusLeftovers(t *testing.T) {
	f := newFixture(t)
	st := f.stateDir()
	for _, d := range []string{"clone/x", "scratch/y", "merge/w1", "clone/z.partial", "gates/g.partial"} {
		os.MkdirAll(filepath.Join(st, d), 0o755)
	}
	f.git("update-ref", "refs/lsf/abcd1234/xfer/w1", "HEAD")
	f.git("update-ref", "refs/lsf/abcd1234/other", "HEAD")
	name := plantContainer(t, f, st)
	s, code := f.status()
	eq(t, code, exitNoLaunch)
	eq(t, s.Driver, driverNone)
	eq(t, strings.Join(s.Leftovers.Clone, ","), "x,z.partial")
	eq(t, strings.Join(s.Leftovers.Scratch, ","), "y")
	eq(t, strings.Join(s.Leftovers.Merge, ","), "w1")
	eq(t, strings.Join(s.Leftovers.Partial, ","), "clone/z.partial,gates/g.partial")
	eq(t, strings.Join(s.XferRefs, ","), "refs/lsf/abcd1234/xfer/w1")
	eq(t, strings.Join(s.Containers, ","), name)
	eq(t, s.ContainersError, "")
}

func gitPath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	return p
}

// runStub runs the stub podman.
func runStub(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command(testcfg.Podman(), args...).CombinedOutput(); err != nil {
		t.Errorf("stub podman %v: %v %s", args, err, out)
	}
}

// plantContainer starts a stub-podman container carrying the state dir's
// label, and a launch.json naming the stub podman, the repo and the name.
func plantContainer(t *testing.T, f *fixture, state string) string {
	t.Helper()
	label, err := lsfrt.StateLabel(state)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(M{"repo": f.repo, "podmanBin": testcfg.Podman(), "args": M{"name": "f"}})
	os.WriteFile(filepath.Join(state, launchFile), b, 0o600)
	name := "lsf-planted"
	cmd := exec.Command(testcfg.Podman(), "run", "--name", name, "--label", "lsf.state="+label, testcfg.Image, "sleep", "300")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runStub(t, "rm", "-f", "-t", "0", name); cmd.Wait() })
	waitFor(t, 10*time.Second, "the planted container", func() bool {
		_, err := os.Stat(filepath.Join(f.podman, "name."+name))
		return err == nil
	})
	return name
}

// sessionOf is a process's session id, from /proc/<pid>/stat.
func sessionOf(t *testing.T, pid int) int {
	t.Helper()
	b := readFile(t, fmt.Sprintf("/proc/%d/stat", pid))
	f := strings.Fields(b[strings.LastIndex(b, ")")+1:]) // state ppid pgrp session ...
	n, err := strconv.Atoi(f[3])
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// With no poll firing, launch learns of a fast run only from the driver's
// exit: a run that started and ended is still a start, and a refusal
// carries the driver's output and exit.
func TestSpawnExitBranch(t *testing.T) {
	f := newFixture(t)
	f.git("update-ref", "refs/lsf/f/pause", "HEAD")
	t.Setenv("LSF_CLI_AS_MAIN", "1") // the re-exec'd test binary is the CLI
	defer func(p time.Duration) { launchPoll = p }(launchPoll)
	launchPoll = time.Hour
	state := f.stateDir()
	os.MkdirAll(state, 0o700)
	composed := func() string {
		out := filepath.Join(state, "launch.1.json")
		if _, err := compose(launchIn{hostFile: f.host, repo: f.repo, stateDir: state, args: f.args(), out: out}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	r, err := spawn(state, composed())
	if err != nil {
		t.Fatal(err)
	}
	if !r.started || r.launch == "" {
		t.Errorf("a run that ended before any poll: %+v", r)
	}
	t.Setenv("STUB_PODMAN_NOIMAGE", "1")
	r, err = spawn(state, composed())
	if err != nil {
		t.Fatal(err)
	}
	if r.started {
		t.Fatal("a refused driver reported started")
	}
	contains(t, r.log, "is not present")
	eq(t, r.exit, "exit status 1")
}
