package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// With LSF_FAKE_CLAUDE_TABLE set, the test binary is a fake `claude -p`: it
// answers the prompt on stdin from a table keyed by the prompt's SHA-256,
// and counts its invocations in LSF_FAKE_CLAUDE_STATE/count.
func TestMain(m *testing.M) {
	if table := os.Getenv("LSF_FAKE_CLAUDE_TABLE"); table != "" {
		os.Exit(fakeClaude(table))
	}
	os.Exit(m.Run())
}

func fakeClaude(table string) int {
	session := "fake"
	for i, a := range os.Args {
		if a == "--session-id" && i+1 < len(os.Args) {
			session = os.Args[i+1]
		}
	}
	if dir := os.Getenv("LSF_FAKE_CLAUDE_STATE"); dir != "" {
		// Calls may run concurrently: one file per call, counted after.
		f, err := os.CreateTemp(dir, "call-")
		if err == nil {
			f.Close()
		}
	}
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake claude: stdin:", err)
		return 3
	}
	sum := sha256.Sum256(prompt)
	var answers map[string]json.RawMessage
	b, err := os.ReadFile(table)
	if err == nil {
		err = json.Unmarshal(b, &answers)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake claude: table:", err)
		return 3
	}
	fmt.Printf(`{"type":"system","subtype":"init","session_id":%q,"tools":["Read"],"slash_commands":[]}`+"\n", session)
	fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"working"}]}}`)
	v, ok := answers[hex.EncodeToString(sum[:])]
	if !ok {
		fmt.Println(`{"type":"result","subtype":"success","is_error":true,"api_error_status":400,"total_cost_usd":0}`)
		return 1
	}
	fmt.Printf(`{"type":"result","subtype":"success","is_error":false,"structured_output":%s,"total_cost_usd":0.01}`+"\n", v)
	return 0
}

// events counts telemetry.jsonl's events by kind.
func events(t *testing.T, stateDir string) map[string]int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(stateDir, "telemetry.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var e struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal([]byte(line), &e) != nil {
			t.Fatalf("telemetry line %q", line)
		}
		out[e.Kind]++
	}
	return out
}

func calls(t *testing.T, dir string) int {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(es)
}

// The orchestrator on the real runtime: a multi-wave launch ending in a stop
// whose scribe repeats a ledger label logs no label-reused, and a relaunch
// replays every call from the journal to the same outcome.
func TestOnRuntimeLabelsUniqueAndReplay(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	const planSha = "0123456789abcdef"
	scenario := func() *orchtest.Stub {
		p := orchtest.TwoWaves()
		p.Sha = planSha
		return orchtest.New(t, p, merged(discovering(), Hs{
			"obligations": obligationsReturning(M{"statement": "perf numbers collected", "disposition": "undischarged", "carrier": "", "evidence": "none"}),
		}), M{"planSha": planSha})
	}

	// The stub run gives the expected outcome and the answer to each prompt.
	s := scenario()
	want, err := Run(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if !has(s.Labels(), "ledger:w2#2") {
		t.Fatalf("the scenario does not repeat a ledger label: %v", s.Labels())
	}
	table := map[string]json.RawMessage{}
	for _, c := range s.Calls {
		sum := sha256.Sum256([]byte(c.Prompt))
		k := hex.EncodeToString(sum[:])
		if prev, ok := table[k]; ok && string(prev) != string(s.Returns[c.Label]) {
			t.Fatalf("%s: prompt shared with another label that has a different return", c.Label)
		}
		table[k] = s.Returns[c.Label]
	}
	dir := t.TempDir()
	tablePath := filepath.Join(dir, "table.json")
	b, _ := json.Marshal(table)
	if err := os.WriteFile(tablePath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	callDir := filepath.Join(dir, "calls")
	os.Mkdir(callDir, 0o755)
	t.Setenv("LSF_FAKE_CLAUDE_TABLE", tablePath)
	t.Setenv("LSF_FAKE_CLAUDE_STATE", callDir)
	t.Setenv("LSF_CLAUDE_BIN", "")

	repo := filepath.Join(dir, "repo")
	for _, args := range [][]string{
		{"init", "-q", "-b", "integration", repo},
		{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	cfg := lsfrt.Config{
		StateDir:    filepath.Join(dir, "state"),
		Repo:        repo,
		ClaudeBin:   os.Args[0],
		Args:        s.ArgsJSON,
		Roles:       map[string]lsfrt.Role{"worker": {Tools: "default", PermissionMode: "dontAsk"}},
		DefaultRole: "worker",
	}
	launch := func() lsfrt.Summary {
		t.Helper()
		sum, err := lsfrt.Run(context.Background(), cfg, Body)
		if err != nil {
			t.Fatal(err)
		}
		return sum
	}
	sameOutcome := func(got json.RawMessage) {
		t.Helper()
		var a, b any
		if json.Unmarshal(got, &a) != nil || json.Unmarshal(want, &b) != nil || !reflect.DeepEqual(a, b) {
			t.Errorf("outcome\n%s\nwant\n%s", got, want)
		}
	}

	first := launch()
	if first.StopReason != lsfrt.StopReturned {
		t.Fatalf("stop reason %s: %s %s", first.StopReason, first.Detail, first.Error)
	}
	sameOutcome(first.Outcome)
	eq(t, first.Calls, len(s.Calls), "one spawn per label: no retries")
	eq(t, calls(t, callDir), len(s.Calls))
	for _, f := range []string{"log.txt", "telemetry.jsonl"} {
		b, err := os.ReadFile(filepath.Join(cfg.StateDir, f))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "label-reused") || strings.Contains(string(b), "label reused") {
			t.Errorf("%s reports a reused label", f)
		}
	}

	records, _ := filepath.Glob(filepath.Join(cfg.StateDir, "journal", "*.json"))
	eq(t, len(records), len(s.Calls), "one journal record per label")
	for _, key := range []string{"ledger_w2-", "ledger_w2_2-"} {
		if m, _ := filepath.Glob(filepath.Join(cfg.StateDir, "journal", key+"*.json")); len(m) != 1 {
			t.Errorf("journal record %s*: %v", key, m)
		}
	}
	firstEvents := events(t, cfg.StateDir)
	eq(t, firstEvents["label-reused"], 0)

	again := launch()
	sameOutcome(again.Outcome)
	eq(t, again.Calls, 0, "the relaunch replays every call")
	eq(t, calls(t, callDir), len(s.Calls), "no spawn on the relaunch")
	all := events(t, cfg.StateDir)
	eq(t, all["cache-hit"]-firstEvents["cache-hit"], len(s.Calls), "every call replayed from the journal")
	eq(t, all["cache-stale"], 0)
}
