package lsfrt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

var stubBin = func() string {
	p, err := filepath.Abs(filepath.Join("..", "testdata", "claude"))
	if err != nil {
		panic(err)
	}
	return p
}()

// testConfig is a config over the stub with two roles. STUB_STATE is set to a
// fresh directory, returned with the config.
func testConfig(t *testing.T) (Config, string) {
	t.Helper()
	stub := t.TempDir()
	t.Setenv("STUB_STATE", stub)
	t.Setenv("LSF_CLAUDE_BIN", "")
	return Config{
		StateDir:  t.TempDir(),
		Repo:      t.TempDir(),
		ClaudeBin: stubBin,
		Roles: map[string]Role{
			"worker": {Tools: "default", Allow: []string{"Read", "Bash"}, Deny: []string{"Bash(git push*)"}, PermissionMode: "dontAsk"},
			"critic": {Tools: "Read,Grep,Skill", Allow: []string{"Skill"}, Deny: []string{"Bash(git commit*)", "Edit(.git/**)"}, PermissionMode: "acceptEdits"},
		},
		DefaultRole: "worker",
		RoleRules:   []RoleRule{{Label: "^critique-", Role: "critic"}},
		Skills:      []SkillRule{{Label: "^critique-", Skill: "phronithm:critique"}},
		KillGrace:   Duration(500 * time.Millisecond),
		Backoff:     []Duration{Duration(10 * time.Millisecond), Duration(10 * time.Millisecond), Duration(10 * time.Millisecond)},
	}, stub
}

// launch runs body under Run and fails the test on a setup error.
func launch(t *testing.T, cfg Config, body func(ctx context.Context, rt *Runtime) (json.RawMessage, error)) Summary {
	t.Helper()
	s, err := Run(context.Background(), cfg, body)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// stubCount is how many times the stub ran.
func stubCount(t *testing.T, stub string) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(stub, "count"))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

// stubArgv is invocation n's arguments.
func stubArgv(t *testing.T, stub string, n int) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(stub, "argv."+strconv.Itoa(n)))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00")
}

// stubPids is invocation n's leader and grandchild pids (hang modes).
func stubPids(t *testing.T, stub string, n int) []int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(stub, "pids."+strconv.Itoa(n)))
	if err != nil {
		t.Fatal(err)
	}
	var pids []int
	for _, f := range strings.Fields(string(b)) {
		p, _ := strconv.Atoi(f)
		pids = append(pids, p)
	}
	return pids
}

// alive reports whether pid exists and is not a zombie.
func alive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true // no /proc (macOS): kill(0) is the answer
	}
	f := strings.Fields(string(b))
	return len(f) < 3 || f[2] != "Z"
}

// waitFor polls cond until it holds or the timeout passes.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func telemetry(t *testing.T, cfg Config) []map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(cfg.StateDir, "telemetry.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

func countEvents(evs []map[string]any, kind string) int {
	n := 0
	for _, e := range evs {
		if e["kind"] == kind {
			n++
		}
	}
	return n
}

func readSummary(t *testing.T, cfg Config) Summary {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(cfg.StateDir, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s Summary
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}
