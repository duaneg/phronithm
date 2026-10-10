package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/internal/testcfg"
	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

func TestPause(t *testing.T) {
	f := newFixture(t)
	r := f.cli(nil, "pause", "-name", "f")
	eq(t, r.code, 0, r.stderr)
	contains(t, r.stdout, "next wave boundary, not at once")
	eq(t, f.git("rev-parse", "refs/lsf/f/pause"), f.git("rev-parse", "HEAD"))
	s, _ := f.decode(f.cli(nil, "status", "-name", "f"))
	if s.Pause == nil || !s.Pause.Set || s.Pause.Ref != "refs/lsf/f/pause" {
		t.Errorf("pause %+v", s.Pause)
	}
	r = f.cli(nil, "pause", "-lift", "-name", "f")
	eq(t, r.code, 0, r.stderr)
	if err := exec.Command("git", "-C", f.repo, "show-ref", "--verify", "--quiet", "refs/lsf/f/pause").Run(); err == nil {
		t.Error("the pause ref is still set")
	}
	r = f.cli(nil, "pause", "-state", t.TempDir())
	eq(t, r.code, exitError, "a state dir with no launch names no ref")
}

// stop twice against a run blocked in a call: the run ends
// signal-SIGTERM. stop and resume refuse while it runs, and stop refuses
// with no driver.
func TestStopTwice(t *testing.T) {
	f := newFixture(t)
	r := f.cli(nil, "stop", "-state", f.stateDir())
	eq(t, r.code, exitError)
	contains(t, r.stderr, "no driver is running")
	r = f.launch([]string{"STUB_MODE=hang"})
	eq(t, r.code, 0, r.stderr)
	d, _ := readDriver(f.stateDir())
	t.Cleanup(func() { killRun(t, f, d.PID) })
	waitFor(t, 20*time.Second, "the call's container", func() bool {
		s, _ := f.status()
		return len(s.Containers) > 0
	})
	r = f.cli(nil, "resume", "-state", f.stateDir())
	eq(t, r.code, exitError)
	contains(t, r.stderr, "a driver is running")
	r = f.cli(nil, "stop", "-state", f.stateDir())
	eq(t, r.code, 0, r.stderr)
	contains(t, r.stdout, "stop again to cancel them")
	time.Sleep(300 * time.Millisecond)
	if s, _ := f.status(); s.Driver != driverRunning {
		t.Fatalf("one SIGTERM ended a run whose call hangs: %s", s.Driver)
	}
	r = f.cli(nil, "stop", "-state", f.stateDir())
	eq(t, r.code, 0, r.stderr)
	w, code := f.decode(f.cli(nil, "watch", "-state", f.stateDir()))
	eq(t, code, 0)
	eq(t, w.Driver, driverEnded)
	eq(t, w.Summary.StopReason, "signal-SIGTERM")
	eq(t, len(w.Containers), 0, "the call's container removed")
}

// stop refuses while the driver is in its launch checks, when SIGTERM
// would kill it before the runtime's handlers are installed.
func TestStopWhileStarting(t *testing.T) {
	f := newFixture(t)
	f.git("update-ref", "refs/lsf/f/pause", "HEAD")
	// claude --version is fast for compose and slow for the smoke container.
	slow := filepath.Join(t.TempDir(), "claude")
	count := filepath.Join(t.TempDir(), "n")
	os.WriteFile(slow, []byte(`#!/bin/sh
n=$(cat "`+count+`" 2>/dev/null || echo 0); echo $((n + 1)) > "`+count+`"
[ "$n" -ge 1 ] && sleep 3
echo '`+testcfg.ClaudeVersion+` (Claude Code)'
`), 0o755)
	f.writeHost(M{"claudeBin": slow})
	cmd := exec.Command(os.Args[0], "launch", "-args", f.argsFile(), "-config", f.host)
	cmd.Dir, cmd.Env = f.repo, append(os.Environ(), "LSF_CLI_AS_MAIN=1")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, "the smoke container", func() bool {
		b, _ := os.ReadFile(count)
		return strings.TrimSpace(string(b)) == "2"
	})
	s, _ := f.status()
	eq(t, s.Driver, driverStarting)
	r := f.cli(nil, "stop", "-state", f.stateDir())
	eq(t, r.code, exitError)
	contains(t, r.stderr, "the driver is starting")
	if err := cmd.Wait(); err != nil {
		t.Fatalf("launch: %v %s", err, out.String())
	}
	w, _ := f.decode(f.cli(nil, "watch", "-state", f.stateDir()))
	eq(t, outcomeStatus(t, w), "paused", "the refused stop did not signal")
}

// resume replays the last launch with its stored args against the current
// host config: a run stopped by a cap resumes with the cap raised, its
// completed call replayed from the journal. After it returns, resume
// refuses.
func TestResume(t *testing.T) {
	f := newFixture(t)
	r := f.cli(nil, "resume", "-state", f.stateDir())
	eq(t, r.code, exitError)
	contains(t, r.stderr, "nothing to resume")

	const planCall = "Write the implementation plan for this unit"
	answers := []answer{
		{Contains: planCall, Return: orchtest.Ret(M{"branch": "@branch", "sha": f.workTip[:7], "path": "docs/plans/A-plan.md", "bytes": 0, "readings": []any{}}), Shell: "git reset -q --hard work-A"},
		{Contains: "Critique the implementation plan at", Return: orchtest.NoFindings()},
		{Contains: "Run the phronithm:critique skill with type critique-code", Return: orchtest.NoFindings()},
		{Contains: "Run the phronithm:review skill", Return: orchtest.NoFindings()},
		{Contains: "Implementation plan: docs/plans/", Return: orchtest.Ret(orchtest.Implemented("A", M{"branch": "@branch", "sha": f.workTip})), Shell: "git reset -q --hard work-A"},
		{Contains: "has merged these units into", Return: orchtest.Ret(M{"invalidations": []any{}, "notes": []any{}, "amendments": []any{}})},
		{Contains: "Read its Meta-Tasks and Standing Obligations sections", Return: orchtest.Ret(M{"planSha": "@head", "appendedUnits": []any{}, "directEdits": []any{}})},
		{Contains: "For each standing obligation", Return: orchtest.Ret(M{"obligations": []any{}})},
	}
	dir := t.TempDir()
	table := filepath.Join(dir, "table.json")
	b, err := plan.MarshalJS(answers, "")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(table, b, 0o644)
	calls := filepath.Join(dir, "calls")
	os.Mkdir(calls, 0o755)
	env := []string{"LSF_CLI_FAKE_TABLE=" + table, "LSF_CLI_FAKE_CALLS=" + calls}
	countOf := func(i int) int {
		m, _ := filepath.Glob(filepath.Join(calls, "call-"+string(rune('0'+i))+"-*"))
		return len(m)
	}
	t.Setenv("LSF_CLI_FAKE_TABLE", table)                                                // compose's claude --version is the test binary
	f.writeHost(M{"claudeBin": os.Args[0], "caps": M{"maxCalls": 1}, "skills": []any{}}) // the fake loads no skill

	r = f.launch(env)
	eq(t, r.code, 0, r.stderr)
	w, _ := f.decode(f.cli(nil, "watch", "-state", f.stateDir()))
	eq(t, w.Summary.StopReason, "max-calls", w.Summary.Detail+w.Summary.Error)
	eq(t, w.Summary.Calls, 1)
	eq(t, countOf(0), 1, "the plan call ran")

	f.writeHost(M{"claudeBin": os.Args[0], "caps": M{"maxCalls": 40}, "skills": []any{}})
	r = f.cli(env, "resume", "-state", f.stateDir(), "-config", f.host)
	eq(t, r.code, 0, r.stderr)
	w, _ = f.decode(f.cli(nil, "watch", "-state", f.stateDir()))
	eq(t, w.Summary.StopReason, "returned", w.Summary.Error)
	eq(t, outcomeStatus(t, w), "complete", string(w.Outcome))
	eq(t, countOf(0), 1, "the plan call replayed, not re-run")
	var launched struct {
		Args json.RawMessage `json:"args"`
	}
	json.Unmarshal([]byte(readFile(t, filepath.Join(f.stateDir(), launchFile))), &launched)
	var stored bytes.Buffer
	json.Compact(&stored, launched.Args)
	eq(t, stored.String(), string(f.args()), "the stored args")
	contains(t, readFile(t, filepath.Join(f.stateDir(), "telemetry.jsonl")), `"kind":"cache-hit"`)

	r = f.cli(env, "resume", "-state", f.stateDir(), "-config", f.host)
	eq(t, r.code, exitError)
	contains(t, r.stderr, "returned an outcome (status complete)")
	contains(t, r.stderr, "launch -args")
}
