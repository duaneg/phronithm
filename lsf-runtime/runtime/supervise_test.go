package lsfrt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Subprocess tests re-exec the test binary as a launch: LSFRT_HELPER names
// the scenario, LSFRT_HELPER_CONFIG the config file.
func TestMain(m *testing.M) {
	if s := os.Getenv("LSFRT_HELPER"); s != "" {
		os.Exit(helperMain(s))
	}
	os.Exit(m.Run())
}

func helperMain(scenario string) int {
	cfg, err := LoadConfig(os.Getenv("LSFRT_HELPER_CONFIG"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 3
	}
	o := Opts{Label: "implement:a", Schema: json.RawMessage(verdictSchema)}
	other := Opts{Label: "implement:b", Schema: json.RawMessage(verdictSchema)}
	_, err = Run(context.Background(), cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		switch scenario {
		case "two-calls":
			if _, err := rt.Agent(ctx, "a", o); err != nil {
				return nil, err
			}
			_, err := rt.Agent(ctx, "b", other)
			return nil, err
		case "swallow":
			rt.Agent(ctx, "a", o)
			rt.Agent(ctx, "b", other)
			return json.RawMessage(`{"status":"complete"}`), nil
		case "three-hangs":
			var thunks []func(context.Context) (json.RawMessage, error)
			for i := range 3 {
				oi := o
				oi.Label = "implement:h" + strconv.Itoa(i)
				thunks = append(thunks, func(ctx context.Context) (json.RawMessage, error) { return rt.Agent(ctx, "x", oi) })
			}
			_, err := rt.Parallel(ctx, thunks)
			return nil, err
		}
		return nil, errors.New("unknown scenario " + scenario)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 3
	}
	return 0
}

// startHelper launches the scenario in a subprocess.
func startHelper(t *testing.T, cfg Config, scenario string) *exec.Cmd {
	t.Helper()
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "LSFRT_HELPER="+scenario, "LSFRT_HELPER_CONFIG="+p)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	return cmd
}

// liveChildren is children.json's entries that have a pid.
func liveChildren(t *testing.T, cfg Config) map[string]int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(cfg.StateDir, "children.json"))
	if err != nil {
		return nil
	}
	var m map[string]int
	json.Unmarshal(b, &m)
	for k, v := range m {
		if v == 0 {
			delete(m, k)
		}
	}
	return m
}

func waitExit(t *testing.T, cmd *exec.Cmd, within time.Duration) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("helper: %v", err)
		}
	case <-time.After(within):
		t.Fatalf("helper did not exit within %v", within)
	}
}

func TestSignalStopsAtNextBoundary(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			cfg, stub := testConfig(t)
			t.Setenv("STUB_SCRIPT", "slow:1.5")
			cmd := startHelper(t, cfg, "two-calls")
			waitFor(t, 5*time.Second, "the first call to start", func() bool { return len(liveChildren(t, cfg)) == 1 })
			cmd.Process.Signal(sig)
			waitExit(t, cmd, 10*time.Second)
			s := readSummary(t, cfg)
			want := map[syscall.Signal]StopReason{syscall.SIGINT: StopSigint, syscall.SIGTERM: StopSigterm}[sig]
			if s.StopReason != want || stubCount(t, stub) != 1 {
				t.Fatalf("summary %+v, spawns %d", s, stubCount(t, stub))
			}
			if _, err := os.Stat(filepath.Join(cfg.StateDir, "journal", journalKey("implement:a")+".json")); err != nil {
				t.Errorf("the in-flight call was not journaled: %v", err)
			}
		})
	}
}

func TestSignalDuringBackoff(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.Backoff = []Duration{Duration(20 * time.Second)}
	t.Setenv("STUB_SCRIPT", "transient,ok")
	cmd := startHelper(t, cfg, "two-calls")
	waitFor(t, 5*time.Second, "the first attempt to fail", func() bool {
		return stubCount(t, stub) == 1 && len(liveChildren(t, cfg)) == 0 && strings.Contains(readFile(t, cfg, "log.txt"), "retrying in")
	})
	start := time.Now()
	cmd.Process.Signal(syscall.SIGINT)
	waitExit(t, cmd, 5*time.Second)
	if time.Since(start) > 3*time.Second || stubCount(t, stub) != 1 || readSummary(t, cfg).StopReason != StopSigint {
		t.Fatalf("waited %v, spawns %d, summary %+v", time.Since(start), stubCount(t, stub), readSummary(t, cfg))
	}
}

func TestSecondSignalKillsInFlight(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.KillGrace = Duration(300 * time.Millisecond)
	t.Setenv("STUB_SCRIPT", "hang-ignore-term")
	cmd := startHelper(t, cfg, "two-calls")
	waitFor(t, 5*time.Second, "the call to start", func() bool {
		_, err := os.Stat(filepath.Join(stub, "pids.1"))
		return err == nil
	})
	cmd.Process.Signal(syscall.SIGINT)
	time.Sleep(200 * time.Millisecond)
	cmd.Process.Signal(syscall.SIGINT)
	waitExit(t, cmd, 5*time.Second)
	for _, pid := range stubPids(t, stub, 1) {
		waitFor(t, 2*time.Second, "pid to die", func() bool { return !alive(pid) })
	}
	if s := readSummary(t, cfg); s.StopReason != StopSigint || stubCount(t, stub) != 1 {
		t.Fatalf("summary %+v", s)
	}
}

func TestSwallowedStopIsRecorded(t *testing.T) {
	cfg, _ := testConfig(t)
	t.Setenv("STUB_SCRIPT", "slow:1")
	cmd := startHelper(t, cfg, "swallow")
	waitFor(t, 5*time.Second, "the first call to start", func() bool { return len(liveChildren(t, cfg)) == 1 })
	cmd.Process.Signal(syscall.SIGINT)
	waitExit(t, cmd, 10*time.Second)
	s := readSummary(t, cfg)
	if out, _ := compactJSON(s.Outcome); s.StopReason != StopSigint || string(out) != `{"status":"complete"}` {
		t.Fatalf("summary %+v", s)
	}
}

func TestOrphansReapedByNextLaunch(t *testing.T) {
	cfg, stub := testConfig(t)
	t.Setenv("STUB_SCRIPT", "hang")
	cmd := startHelper(t, cfg, "three-hangs")
	waitFor(t, 5*time.Second, "three calls", func() bool {
		_, err := os.Stat(filepath.Join(stub, "pids.3"))
		return err == nil && len(liveChildren(t, cfg)) == 3
	})
	cmd.Process.Signal(syscall.SIGKILL)
	cmd.Wait()
	var pids []int
	for n := 1; n <= 3; n++ {
		pids = append(pids, stubPids(t, stub, n)...)
	}
	for _, pid := range pids {
		if !alive(pid) {
			t.Fatalf("precondition: pid %d died with its launch", pid)
		}
	}
	launch(t, cfg, func(context.Context, *Runtime) (json.RawMessage, error) { return nil, nil })
	for _, pid := range pids {
		waitFor(t, 2*time.Second, "orphan to die", func() bool { return !alive(pid) })
	}
	if n := countEvents(telemetry(t, cfg), "orphan-terminated"); n != 3 {
		t.Errorf("%d orphan-terminated events, want 3", n)
	}
}

func TestTornChildrenFile(t *testing.T) {
	cfg, _ := testConfig(t)
	os.MkdirAll(cfg.StateDir, 0o755)
	os.WriteFile(filepath.Join(cfg.StateDir, "children.json"), []byte(`{"abc`), 0o644)
	if s := launch(t, cfg, func(context.Context, *Runtime) (json.RawMessage, error) { return nil, nil }); s.StopReason != StopReturned {
		t.Fatalf("summary %+v", s)
	}
	if countEvents(telemetry(t, cfg), "children-unreadable") != 1 {
		t.Error("torn children.json not reported")
	}
}

func readFile(t *testing.T, cfg Config, name string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(cfg.StateDir, name))
	return string(b)
}

func TestCaps(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		caps   Caps
		want   StopReason
		spawns int
	}{
		{"calls", "ok", Caps{MaxCalls: 2}, StopMaxCalls, 2},
		{"cost", "cost:0.6", Caps{MaxCostUSD: 1}, StopMaxCost, 2},
		{"wall-clock", "slow:1.2", Caps{MaxWallClock: Duration(time.Second)}, StopMaxWallClock, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, stub := testConfig(t)
			cfg.Caps = tc.caps
			t.Setenv("STUB_SCRIPT", tc.script)
			var errs []error
			s := launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
				for i := range 3 {
					o := vOpts
					o.Label = "implement:u" + strconv.Itoa(i)
					_, err := rt.Agent(ctx, "x", o)
					errs = append(errs, err)
					if err != nil {
						return nil, err
					}
				}
				return nil, nil
			})
			last := errs[len(errs)-1]
			if r, ok := IsStop(last); !ok || r != tc.want || s.StopReason != tc.want || readSummary(t, cfg).StopReason != tc.want {
				t.Fatalf("errs %v, summary %+v", errs, s)
			}
			if stubCount(t, stub) != tc.spawns {
				t.Errorf("spawns %d, want %d", stubCount(t, stub), tc.spawns)
			}
		})
	}
}

func TestReplayDoesNotCountTowardCaps(t *testing.T) {
	cfg, stub := testConfig(t)
	t.Setenv("STUB_SCRIPT", "ok")
	call(t, cfg, vOpts)
	cfg.Caps = Caps{MaxCalls: 1}
	var errs []error
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		for _, label := range []string{"implement:u1", "implement:new", "implement:third"} {
			o := vOpts
			o.Label = label
			_, err := rt.Agent(ctx, "do the thing", o)
			errs = append(errs, err)
		}
		return nil, nil
	})
	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("errs %v", errs)
	}
	if r, ok := IsStop(errs[2]); !ok || r != StopMaxCalls || stubCount(t, stub) != 2 {
		t.Fatalf("errs %v, spawns %d", errs, stubCount(t, stub))
	}
}

func TestDryRunSpawnsNothing(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.DryRun = true
	cfg.Repo = gitRepo(t)
	cfg.Args = json.RawMessage(`{"planSha":"0123456789ab","integrationBranch":"main"}`)
	stepRan := false
	s := launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		if _, err := rt.Step(ctx, "ledger:w1", []byte("x"), func(context.Context, bool) (json.RawMessage, error) {
			stepRan = true
			return nil, nil
		}); err == nil {
			t.Error("dry-run step did not stop")
		}
		o := vOpts
		o.Isolation = "worktree"
		_, err := rt.Agent(ctx, "x", o)
		return nil, err
	})
	if s.StopReason != StopDryRun || stepRan || stubCount(t, stub) != 0 {
		t.Fatalf("summary %+v, step ran %v, spawns %d", s, stepRan, stubCount(t, stub))
	}
	if entries, _ := os.ReadDir(filepath.Join(cfg.StateDir, "wt")); len(entries) != 0 {
		t.Errorf("worktrees created: %v", entries)
	}
	if b := git(t, cfg.Repo, "branch", "--list", "lsf/*"); b != "" {
		t.Errorf("branches created: %s", b)
	}
	// A fresh dry run (the step stop is sticky) shows the call's argv.
	cfg2, _ := testConfig(t)
	cfg2.DryRun = true
	launch(t, cfg2, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		return rt.Agent(ctx, "x", vOpts)
	})
	if log := readFile(t, cfg2, "log.txt"); !strings.Contains(log, "--strict-mcp-config") || countEvents(telemetry(t, cfg2), "dry-run") != 1 {
		t.Errorf("dry-run argv not logged:\n%s", log)
	}
}

func TestBodyOutcomes(t *testing.T) {
	cfg, _ := testConfig(t)
	if s := launch(t, cfg, func(context.Context, *Runtime) (json.RawMessage, error) { panic("boom") }); s.StopReason != StopCrashed || !strings.Contains(s.Error, "boom") {
		t.Errorf("panic: %+v", s)
	}
	if s := launch(t, cfg, func(context.Context, *Runtime) (json.RawMessage, error) { return nil, errors.New("bug") }); s.StopReason != StopCrashed {
		t.Errorf("error: %+v", s)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s, err := Run(ctx, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		cancel()
		return rt.Agent(ctx, "x", vOpts)
	})
	if err != nil || s.StopReason != StopCancelled {
		t.Errorf("cancelled: %+v %v", s, err)
	}
	if s := launch(t, cfg, func(context.Context, *Runtime) (json.RawMessage, error) {
		return json.RawMessage(`{"status":"complete"}`), nil
	}); s.StopReason != StopReturned {
		t.Errorf("returned: %+v", s)
	}
}
