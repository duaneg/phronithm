package lsfrt

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var vOpts = Opts{Label: "implement:u1", Model: "opus", Effort: "low", Schema: json.RawMessage(verdictSchema)}

func stubInt(t *testing.T, stub, name string) int64 {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(stub, name))
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// call runs one Call in its own launch.
func call(t *testing.T, cfg Config, o Opts) (CallResult, error) {
	t.Helper()
	var res CallResult
	var err error
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		res, err = rt.Call(ctx, "do the thing", o)
		return nil, err
	})
	return res, err
}

func TestRetryClasses(t *testing.T) {
	for _, tc := range []struct {
		script   string
		spawns   int
		value    bool
		class    class
		minDelay time.Duration
	}{
		{"invalid,ok", 2, true, classOK, 0},
		{"invalid,invalid,ok", 2, false, classInvalid, 0},
		{"nullout,ok", 2, true, classOK, 0},
		{"transient,transient,ok", 3, true, classOK, 20 * time.Millisecond},
		{"transient", 4, false, classTransient, 30 * time.Millisecond}, // len(Backoff)+1
		{"fixture:permanent,ok", 1, false, classPermanent, 0},
		{"session-noreset,ok", 2, true, classOK, 50 * time.Millisecond},
	} {
		t.Run(tc.script, func(t *testing.T) {
			cfg, stub := testConfig(t)
			cfg.SessionLimitFallback = Duration(50 * time.Millisecond)
			t.Setenv("STUB_SCRIPT", tc.script)
			start := time.Now()
			res, err := call(t, cfg, vOpts)
			if err != nil {
				t.Fatal(err)
			}
			if n := stubCount(t, stub); n != tc.spawns || res.Attempts != tc.spawns {
				t.Errorf("spawns %d, attempts %d, want %d", n, res.Attempts, tc.spawns)
			}
			if (res.Value != nil) != tc.value || res.Class != string(tc.class) {
				t.Errorf("value %s class %s, want value=%v class %s", res.Value, res.Class, tc.value, tc.class)
			}
			if d := time.Since(start); d < tc.minDelay {
				t.Errorf("took %v, want at least %v of backoff", d, tc.minDelay)
			}
		})
	}
}

func TestSessionLimitWaitsForReset(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.SessionLimitMargin = Duration(time.Millisecond)
	cfg.Backoff = []Duration{Duration(time.Millisecond)} // one transient retry: a wait must not spend it
	t.Setenv("STUB_SCRIPT", "session,ok,ok")
	t.Setenv("STUB_RESET_IN", "1")
	var a, b CallResult
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		_, err := rt.Parallel(ctx, []func(context.Context) (json.RawMessage, error){
			func(ctx context.Context) (json.RawMessage, error) {
				var err error
				a, err = rt.Call(ctx, "first", vOpts)
				return a.Value, err
			},
			func(ctx context.Context) (json.RawMessage, error) {
				// Starts after the first call has met the limit.
				waitFor(t, 2*time.Second, "first call's session limit", func() bool { return stubCount(t, stub) >= 1 })
				time.Sleep(200 * time.Millisecond)
				var err error
				b, err = rt.Call(ctx, "second", Opts{Label: "implement:u2", Schema: json.RawMessage(verdictSchema)})
				return b.Value, err
			},
		})
		return nil, err
	})
	if a.Value == nil || b.Value == nil || a.Attempts != 2 || b.Attempts != 1 {
		t.Fatalf("a %+v b %+v", a, b)
	}
	reset := stubInt(t, stub, "reset.1")
	for _, n := range []string{"started.2", "started.3"} {
		if s := stubInt(t, stub, n); s < reset {
			t.Errorf("%s = %d, before the reset %d", n, s, reset)
		}
	}
	if countEvents(telemetry(t, cfg), "session-limit-wait") != 1 {
		t.Error("want one session-limit-wait event")
	}
}

func TestSessionLimitWaitsAreCapped(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.SessionLimitFallback = Duration(10 * time.Millisecond)
	cfg.MaxSessionWaits = 2
	t.Setenv("STUB_SCRIPT", "session-noreset")
	res, err := call(t, cfg, vOpts)
	if err != nil || res.Value != nil || stubCount(t, stub) != 3 {
		t.Fatalf("res %+v err %v spawns %d", res, err, stubCount(t, stub))
	}
}

func TestSkillLoadCheck(t *testing.T) {
	findings := Opts{Label: "critique-code:r1:u1", Schema: json.RawMessage(findingsSchema)}
	for _, tc := range []struct {
		name, skill, script string
		spawns              int
		flagged             bool
	}{
		{"loaded", "phronithm:critique", "fixture:skill-loaded", 1, false},
		{"loaded by the Skill tool", "phronithm:critique", "fixture:skill-tool", 1, false},
		{"missing twice: accepted, flagged", "phronithm:nonexistent-skill", "fixture:skill-unknown", 2, true},
		{"missing then loaded", "phronithm:critique", "file:STRIPPED,fixture:skill-loaded", 2, false},
		{"invalid then missing: shared budget", "phronithm:nonexistent-skill", "invalid,fixture:skill-unknown", 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, stub := testConfig(t)
			cfg.Skills = []SkillRule{{Label: "^critique-", Skill: tc.skill}}
			t.Setenv("STUB_SCRIPT", strings.ReplaceAll(tc.script, "STRIPPED", strippedFixture(t)))
			res, err := call(t, cfg, findings)
			if err != nil {
				t.Fatal(err)
			}
			if n := stubCount(t, stub); n != tc.spawns || res.Value == nil {
				t.Fatalf("spawns %d, value %s; want %d spawns and a value", n, res.Value, tc.spawns)
			}
			if got := slices.Contains(res.Flags, "skill-not-loaded"); got != tc.flagged {
				t.Errorf("flags %v", res.Flags)
			}
			if got := countEvents(telemetry(t, cfg), "skill-not-loaded"); got != map[bool]int{true: 1}[tc.flagged] {
				t.Errorf("skill-not-loaded events %d", got)
			}
			stdin, _ := os.ReadFile(filepath.Join(stub, "stdin.1"))
			if !strings.HasPrefix(string(stdin), "/"+tc.skill+" ") {
				t.Errorf("stdin %q not sent as the skill's slash command", stdin)
			}
		})
	}
}

// strippedFixture is skill-absent with its init event's slash_commands
// emptied: a prefixed call whose command is unavailable.
func strippedFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "streams", "skill-absent.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && m["type"] == "system" {
			m["slash_commands"] = []string{}
			nb, _ := json.Marshal(m)
			l = string(nb)
		}
		lines = append(lines, l)
	}
	p := filepath.Join(t.TempDir(), "stripped.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSpawnErrorIsNotRetried(t *testing.T) {
	cfg, _ := testConfig(t)
	cfg.ClaudeBin = filepath.Join(t.TempDir(), "missing")
	res, err := call(t, cfg, vOpts)
	if err != nil || res.Value != nil || res.Attempts != 1 || res.Class != string(classSpawnError) {
		t.Fatalf("res %+v err %v", res, err)
	}
}

func TestGuardTimeoutIsNotRetried(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.GuardTimeout = Duration(300 * time.Millisecond)
	cfg.KillGrace = Duration(100 * time.Millisecond)
	t.Setenv("STUB_SCRIPT", "hang,ok")
	res, err := call(t, cfg, vOpts)
	if err != nil || res.Value != nil || stubCount(t, stub) != 1 || res.Class != string(classTimeout) {
		t.Fatalf("res %+v err %v spawns %d", res, err, stubCount(t, stub))
	}
}

func TestConcurrencyBound(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.Concurrency = 2
	t.Setenv("STUB_SCRIPT", "slow:0.5")
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		var thunks []func(context.Context) (json.RawMessage, error)
		for i := range 4 {
			o := vOpts
			o.Label = "implement:u" + strconv.Itoa(i)
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
	if got := stubInt(t, stub, "maxactive"); got != 2 {
		t.Errorf("max concurrent stubs %d, want 2", got)
	}
}

func TestParallelBranchFailures(t *testing.T) {
	cfg, _ := testConfig(t)
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		out, err := rt.Parallel(ctx, []func(context.Context) (json.RawMessage, error){
			func(context.Context) (json.RawMessage, error) { panic("boom") },
			func(context.Context) (json.RawMessage, error) { return json.RawMessage(`1`), errors.New("bug") },
			func(context.Context) (json.RawMessage, error) { return json.RawMessage(`2`), nil },
			func(context.Context) (json.RawMessage, error) { return nil, &StopError{Reason: StopMaxCalls} },
		})
		if out[0] != nil || out[1] != nil || string(out[2]) != "2" {
			t.Errorf("out %q", out)
		}
		if r, ok := IsStop(err); !ok || r != StopMaxCalls {
			t.Errorf("err %v", err)
		}
		return nil, nil
	})
}
