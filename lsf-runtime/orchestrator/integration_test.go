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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// With LSF_FAKE_CLAUDE_TABLE set, the test binary is a fake `claude -p`: it
// answers the prompt on stdin from a table, and counts its invocations in
// LSF_FAKE_CLAUDE_STATE. The table is an object keyed by the prompt's
// SHA-256, or an ordered list of {contains, return} answered by the first
// entry whose substring the prompt contains. No answer is an API error.
func TestMain(m *testing.M) {
	if table := os.Getenv("LSF_FAKE_CLAUDE_TABLE"); table != "" && slices.Contains(os.Args, "--session-id") {
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
		if err != nil {
			fmt.Fprintln(os.Stderr, "fake claude: count:", err)
			return 3
		}
		f.Close()
	}
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake claude: stdin:", err)
		return 3
	}
	sum := sha256.Sum256(prompt)
	var v json.RawMessage
	ok := false
	b, err := os.ReadFile(table)
	if err == nil && strings.HasPrefix(strings.TrimSpace(string(b)), "[") {
		var list []struct {
			Contains string          `json:"contains"`
			Return   json.RawMessage `json:"return"`
		}
		err = json.Unmarshal(b, &list)
		for _, e := range list {
			if e.Contains == "" || len(e.Return) == 0 {
				fmt.Fprintln(os.Stderr, "fake claude: table entry without contains or return")
				return 3
			}
			if strings.Contains(string(prompt), e.Contains) {
				v, ok = e.Return, true
				break
			}
		}
	} else if err == nil {
		var answers map[string]json.RawMessage
		err = json.Unmarshal(b, &answers)
		v, ok = answers[hex.EncodeToString(sum[:])]
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake claude: table:", err)
		return 3
	}
	fmt.Printf(`{"type":"system","subtype":"init","session_id":%q,"tools":["Read"],"slash_commands":[]}`+"\n", session)
	fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"working"}]}}`)
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
	steps := map[string]json.RawMessage{}
	for _, c := range s.Calls {
		if c.Step {
			steps[c.Label] = s.Returns[c.Label]
			continue
		}
		sum := sha256.Sum256([]byte(c.Prompt))
		k := hex.EncodeToString(sum[:])
		if r := s.Returns[c.Label]; len(r) == 0 || string(r) == "null" {
			t.Fatalf("%s: no return to serve (the runtime would retry a null)", c.Label)
		}
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
		sum, err := lsfrt.Run(context.Background(), cfg, func(ctx context.Context, rt *lsfrt.Runtime) (json.RawMessage, error) {
			return Run(ctx, scriptedSteps{rt, steps})
		})
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
	nCalls := len(agentCalls(s))
	eq(t, first.Calls, nCalls, "one spawn per label: no retries")
	eq(t, calls(t, callDir), nCalls)
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
	eq(t, len(records), len(s.Calls), "one journal record per call and step")
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
	eq(t, calls(t, callDir), nCalls, "no spawn on the relaunch")
	all := events(t, cfg.StateDir)
	eq(t, all["cache-hit"]-firstEvents["cache-hit"], nCalls, "every call replayed from the journal")
	eq(t, all["step-replayed"]-firstEvents["step-replayed"], len(steps), "every step replayed from the journal")
	eq(t, firstEvents["step"], len(steps), "every step ran on the first launch")
	eq(t, all["cache-stale"], 0)
}

// scriptedSteps is the real runtime with each code step's value scripted:
// the step is journaled and replayed by rt.Step, but its fn returns the
// stub run's value instead of touching git.
type scriptedSteps struct {
	*lsfrt.Runtime
	values map[string]json.RawMessage
}

func (r scriptedSteps) Step(ctx context.Context, label string, input []byte, fn func(context.Context, bool) (json.RawMessage, error)) (json.RawMessage, error) {
	v, ok := r.values[label]
	if !ok {
		return nil, fmt.Errorf("no scripted value for step %s", label)
	}
	return r.Runtime.Step(ctx, label, input, func(context.Context, bool) (json.RawMessage, error) { return v, nil })
}

// The orchestrator on the real runtime with real code stages: decisions,
// load, one unit with its completion report on a real branch, ledger
// appends, ending complete. The four stages make no agent call; a relaunch
// replays every call and step, with no spawn and no new commit.
func TestOnRuntimeCodeStages(t *testing.T) {
	r := newGitRepo(t)
	const planPath = "docs/plans/f-breakdown.md"
	p := orchtest.Graph("", orchtest.Unit("A"))
	doc := func(p *plan.Plan) string { return "# Plan: f\n\n" + orchtest.ToBlock(p) + "\n" }
	r.commit("docs/architecture/f.md", "# Architecture\n", "architecture")
	planSha := r.commit(planPath, doc(p), "plan")
	r.git("checkout", "-q", "-b", "work-A")
	r.commit("src/A/a.go", "package a\n", "unit A")
	r.git("checkout", "-q", "-b", "side", "integration")
	done := *p
	done.Units = []*plan.Unit{orchtest.Unit("A")}
	done.Units[0].Complete = true
	completeSha := r.commit(planPath, doc(&done), "A complete")
	r.git("checkout", "-q", "integration")

	answers := []struct {
		Contains string `json:"contains"`
		Return   any    `json:"return"`
	}{
		{"The requester has answered the questions", R(M{"planSha": planSha, "amendments": []any{}, "overBudget": []any{},
			"verdicts": []M{{"id": "d1", "verdict": "amended", "statement": "noted"}}, "directEdits": []any{}, "summary": "applied"})},
		{"Write the implementation plan for this unit", R(M{"branch": "work-A", "sha": "p", "path": "docs/plans/A-plan.md", "bytes": 0, "readings": []any{}})},
		{"Critique the implementation plan at", orchtest.NoFindings()},
		{"Run the phronithm:critique skill with type critique-code", orchtest.NoFindings()},
		{"Run the phronithm:review skill", orchtest.NoFindings()},
		{"Implementation plan: docs/plans/", R(orchtest.Implemented("A", M{"branch": "work-A"}))},
		{"Merge each unit branch below into", R(M{"integrationBranch": "integration", "merges": []plan.Merge{{Unit: "A", Status: "clean", Files: []string{}, Sha: "m"}},
			"planSha": completeSha, "pauseRequested": false, "architectureSha": ""})},
		{"has merged these units into", R(M{"invalidations": []any{}, "notes": []any{}, "amendments": []any{}})},
		{"Cleanup stage, wave", R(M{"removed": 0, "kept": []any{}})},
		{"Read its Meta-Tasks and Standing Obligations sections", R(M{"planSha": completeSha, "appendedUnits": []any{}, "directEdits": []any{}})},
		{"For each standing obligation", R(M{"obligations": []any{}})},
	}
	dir := t.TempDir()
	tablePath := filepath.Join(dir, "table.json")
	b, err := plan.MarshalJS(answers, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tablePath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	callDir := filepath.Join(dir, "calls")
	os.Mkdir(callDir, 0o755)
	t.Setenv("LSF_FAKE_CLAUDE_TABLE", tablePath)
	t.Setenv("LSF_FAKE_CLAUDE_STATE", callDir)
	t.Setenv("LSF_CLAUDE_BIN", "")
	args, _ := plan.MarshalJS(M{"name": "f", "planSha": planSha, "architectureDoc": "docs/architecture/f.md", "integrationBranch": "integration",
		"decisions": []M{{"id": "d1", "question": "q", "decision": "yes"}}}, "")
	cfg := lsfrt.Config{
		StateDir: filepath.Join(dir, "state"), Repo: r.dir, ClaudeBin: os.Args[0], Args: args,
		Roles: map[string]lsfrt.Role{"worker": {Tools: "default", PermissionMode: "dontAsk"}}, DefaultRole: "worker",
	}
	launch := func() lsfrt.Summary {
		t.Helper()
		sum, err := lsfrt.Run(context.Background(), cfg, Body)
		if err != nil {
			t.Fatal(err)
		}
		return sum
	}

	first := launch()
	if first.StopReason != lsfrt.StopReturned {
		t.Fatalf("stop reason %s: %s %s", first.StopReason, first.Detail, first.Error)
	}
	var out view
	if err := json.Unmarshal(first.Outcome, &out); err != nil {
		t.Fatal(err)
	}
	eq(t, out.Status, "complete", string(first.Outcome))

	// The decision log, the ledger and the report are real commits with
	// exact content and the trailers.
	trailers := func(sha string) string { return r.git("log", "-1", "--format=%(trailers:key=Lsf-Step,valueonly)", sha) }
	decSha := r.git("log", "-1", "--format=%H", "integration", "--", "docs/plans/f-decisions.md")
	eq(t, trailers(decSha), "decision-log")
	eq(t, r.git("show", decSha+":docs/plans/f-decisions.md")+"\n", "# Decisions: f\n\n```yaml lsf-decisions\ndecisions:\n  - id: \"d1\"\n    date: "+
		time.Now().Format(time.DateOnly)+"\n    question: \"q\"\n    decision: \"yes\"\n```\n")
	ledger := r.git("show", "integration:docs/plans/f-signals/"+planSha+".md")
	match(t, ledger, `^# Signal ledger: f\n`)
	match(t, ledger, `(?s)## Wave 0\n.*d1 amended: noted`)
	for _, sha := range strings.Fields(r.git("log", "--format=%H", "integration", "--", "docs/plans/f-signals/"+planSha+".md")) {
		match(t, trailers(sha), `^ledger:`)
	}
	reportBranch := ""
	for _, ref := range strings.Fields(r.git("for-each-ref", "--format=%(refname:short)", "refs/heads/lsf/")) {
		if strings.Contains(ref, "completion-report_A") {
			reportBranch = ref
		}
	}
	if reportBranch == "" {
		t.Fatal("no completion-report branch")
	}
	eq(t, trailers(reportBranch), "completion-report:A")
	eq(t, r.git("rev-parse", reportBranch+"^"), r.git("rev-parse", "work-A"))
	match(t, r.git("show", reportBranch+":docs/plans/A-completion.md"), `^# A — completion report\n`)
	eq(t, r.git("status", "--porcelain"), "", "the main checkout is clean")

	// No agent call for the four stages; each ran as a step.
	tel, err := os.ReadFile(filepath.Join(cfg.StateDir, "telemetry.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	stepsRun := map[string]bool{}
	spawns := 0
	for _, line := range strings.Split(strings.TrimSpace(string(tel)), "\n") {
		var e struct{ Kind, Label string }
		json.Unmarshal([]byte(line), &e)
		prefix := strings.Split(strings.Split(e.Label, "#")[0], ":")[0]
		switch e.Kind {
		case "call":
			spawns++
			if slices.Contains([]string{"load", "decision-log", "ledger", "completion-report"}, prefix) {
				t.Errorf("agent call %s", e.Label)
			}
		case "step":
			stepsRun[prefix] = true
		}
	}
	for _, p := range []string{"load", "decision-log", "ledger", "completion-report"} {
		if !stepsRun[p] {
			t.Errorf("no %s step", p)
		}
	}
	eq(t, first.Calls, spawns)

	refs := func() string {
		return r.git("for-each-ref", "--format=%(refname) %(objectname)") + "\nHEAD " + r.head()
	}
	tips := refs()
	firstEvents := events(t, cfg.StateDir)
	again := launch()
	eq(t, again.StopReason, lsfrt.StopReturned)
	eq(t, string(again.Outcome), string(first.Outcome))
	eq(t, again.Calls, 0, "the relaunch replays every call")
	eq(t, calls(t, callDir), spawns, "no spawn on the relaunch")
	eq(t, refs(), tips, "no new commit, no ref moved")
	eq(t, r.git("status", "--porcelain"), "", "the main checkout is still clean")
	all := events(t, cfg.StateDir)
	eq(t, all["cache-hit"]-firstEvents["cache-hit"], spawns, "every call replayed from the journal")
	eq(t, all["cache-stale"], 0)
	eq(t, all["step-replayed"]-firstEvents["step-replayed"], firstEvents["step"], "every step replayed")
	eq(t, all["step"], firstEvents["step"], "no step ran again")
}
