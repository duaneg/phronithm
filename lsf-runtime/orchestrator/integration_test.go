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

	"github.com/duaneg/phronithm/lsf-runtime/internal/testenv"
	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// With LSF_FAKE_CLAUDE_TABLE set, the test binary is a fake `claude -p`: it
// answers the prompt on stdin from a table, and counts its invocations in
// LSF_FAKE_CLAUDE_STATE. The table is an object keyed by the prompt's
// SHA-256, or an ordered list of {contains, return} answered by the first
// entry whose substring the prompt contains. No answer is an API error.
// A list entry may also carry shell (a bash command run in the call's cwd
// before answering), denials (Bash commands reported in the result's
// permission_denials), failFirst (the entry's first call is an API 529)
// and once (the entry answers one call; later ones fall to the next match).
// In a return, @branch and @head stand for the cwd's branch and HEAD after
// the shell command, so a worktree stage reports the runtime's branch.
func TestMain(m *testing.M) {
	if table := os.Getenv("LSF_FAKE_CLAUDE_TABLE"); table != "" && slices.Contains(os.Args, "--session-id") {
		os.Exit(fakeClaude(table))
	}
	os.Exit(testenv.Main(m))
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
	var entry struct {
		Shell     string   `json:"shell"`
		Denials   []string `json:"denials"`
		FailFirst bool     `json:"failFirst"`
	}
	entryIdx := -1
	b, err := os.ReadFile(table)
	if err == nil && strings.HasPrefix(strings.TrimSpace(string(b)), "[") {
		var list []struct {
			Contains  string          `json:"contains"`
			Return    json.RawMessage `json:"return"`
			Shell     string          `json:"shell"`
			Denials   []string        `json:"denials"`
			FailFirst bool            `json:"failFirst"`
			Once      bool            `json:"once"`
		}
		err = json.Unmarshal(b, &list)
		for i, e := range list {
			if e.Contains == "" || len(e.Return) == 0 {
				fmt.Fprintln(os.Stderr, "fake claude: table entry without contains or return")
				return 3
			}
			if !strings.Contains(string(prompt), e.Contains) {
				continue
			}
			if e.Once {
				used := filepath.Join(os.Getenv("LSF_FAKE_CLAUDE_STATE"), fmt.Sprintf("once-%d", i))
				if _, err := os.Stat(used); err == nil {
					continue
				}
				os.WriteFile(used, nil, 0o644)
			}
			{
				v, ok = e.Return, true
				entry.Shell, entry.Denials, entry.FailFirst, entryIdx = e.Shell, e.Denials, e.FailFirst, i
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
	if entry.FailFirst {
		marker := filepath.Join(os.Getenv("LSF_FAKE_CLAUDE_STATE"), fmt.Sprintf("failed-%d", entryIdx))
		if _, err := os.Stat(marker); err != nil {
			os.WriteFile(marker, nil, 0o644)
			fmt.Println(`{"type":"assistant","error":"overloaded","message":{"content":[{"type":"text","text":"Overloaded"}]}}`)
			fmt.Println(`{"type":"result","subtype":"success","is_error":true,"api_error_status":529,"total_cost_usd":0}`)
			return 1
		}
	}
	if entry.Shell != "" {
		if out, err := exec.Command("bash", "-c", entry.Shell).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "fake claude: shell: %v %s\n", err, out)
			return 3
		}
	}
	for _, sub := range []struct{ token, arg string }{{"@branch", "--abbrev-ref"}, {"@head", ""}} {
		if !strings.Contains(string(v), sub.token) {
			continue
		}
		args := []string{"rev-parse", "HEAD"}
		if sub.arg != "" {
			args = []string{"rev-parse", sub.arg, "HEAD"}
		}
		out, err := exec.Command("git", args...).Output()
		if err != nil {
			fmt.Fprintln(os.Stderr, "fake claude: "+sub.token+":", err)
			return 3
		}
		v = json.RawMessage(strings.ReplaceAll(string(v), sub.token, strings.TrimSpace(string(out))))
	}
	denials := []map[string]any{}
	for _, d := range entry.Denials {
		denials = append(denials, map[string]any{"tool_name": "Bash", "tool_use_id": "t", "tool_input": map[string]string{"command": d}})
	}
	db, _ := json.Marshal(denials)
	fmt.Printf(`{"type":"result","subtype":"success","is_error":false,"structured_output":%s,"total_cost_usd":0.01,"permission_denials":%s}`+"\n", v, db)
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
	es, err := filepath.Glob(filepath.Join(dir, "call-*")) // not the once/failed markers
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
// stub run's value instead of touching git. Agent calls still run in the
// runtime's repository.
type scriptedSteps struct {
	*lsfrt.Runtime
	values map[string]json.RawMessage
}

// Repo is "": the scripted values name branches and SHAs the repository
// does not have, so the orchestrator must not read git around the steps.
func (r scriptedSteps) Repo() string { return "" }

// Call drops the worktree branch: the scripted returns name the stub run's
// branches, and the branch check is not this test's subject.
func (r scriptedSteps) Call(ctx context.Context, prompt string, o lsfrt.Opts) (lsfrt.CallResult, error) {
	cr, err := r.Runtime.Call(ctx, prompt, o)
	cr.Branch = ""
	return cr, err
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
	doc := func(p *plan.Plan) string { return planDocFor(p) }
	r.commit(".gitignore", "*.log\n", "ignore logs")
	r.commit("docs/architecture/f.md", "# Architecture\n", "architecture")
	planSha := r.commit(planPath, doc(p), "plan")
	r.git("checkout", "-q", "-b", "work-A")
	r.commit("src/A/a.go", "package a\n", "unit A")
	r.git("checkout", "-q", "integration")

	answers := []answer{
		{Contains: "The requester has answered the questions", Return: R(M{"planSha": planSha, "amendments": []any{}, "overBudget": []any{},
			"verdicts": []M{{"id": "d1", "verdict": "amended", "statement": "noted"}}, "directEdits": []any{}, "summary": "applied"})},
		{Contains: "Write the implementation plan for this unit", Shell: "git reset -q --hard work-A", Return: R(M{"branch": "@branch", "sha": "@head", "path": "docs/plans/A-plan.md", "bytes": 0, "readings": []any{}})},
		{Contains: "Critique the implementation plan at", Return: orchtest.NoFindings()},
		// One Critical in round 1: a code fix on its own branch, which the
		// completion report builds on and a replay after cleanup reads the
		// journaled tip of.
		{Contains: "Run the phronithm:critique skill with type critique-code", Once: true,
			Return: R(M{"findings": []plan.Finding{orchtest.Finding("Critical", "bad", "src/A/a.go:1")}, "systemic": false, "summary": "s"})},
		{Contains: "Run the phronithm:critique skill with type critique-code", Return: orchtest.NoFindings()},
		{Contains: "Run the phronithm:review skill", Return: orchtest.NoFindings()},
		{Contains: "Critique gate round 1 found these Critical findings against the code", Shell: "git reset -q --hard work-A && echo '// fixed' >> src/A/a.go && git -c user.name=t -c user.email=t@x commit -qam fix",
			Return: R(M{"branch": "@branch", "sha": "@head", "applied": []M{{"finding": "bad", "fix": "fixed"}}, "notApplied": []any{}, "gates": orchtest.GreenGates("1 passed")})},
		{Contains: "Implementation plan: docs/plans/", Shell: "git reset -q --hard work-A && echo built > build.log", Return: R(orchtest.Implemented("A", M{"branch": "@branch", "sha": "@head"}))},
		{Contains: "has merged these units into", Return: R(M{"invalidations": []any{}, "notes": []any{}, "amendments": []any{}})},
		{Contains: "Read its Meta-Tasks and Standing Obligations sections", Return: R(M{"planSha": "@head", "appendedUnits": []any{}, "directEdits": []any{}})},
		{Contains: "For each standing obligation", Return: R(M{"obligations": []any{}})},
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
	eq(t, r.git("status", "--porcelain"), "", "the main checkout is clean")

	// The merge (D24): a merge commit of the report's commit, then the
	// bookkeeping commit over the plan and the archive.
	mergeSha, reportSha := "", ""
	for _, line := range strings.Split(r.git("log", "--first-parent", "--format=%H %P", "integration"), "\n") {
		if f := strings.Fields(line); len(f) == 3 {
			mergeSha, reportSha = f[0], f[2]
			break
		}
	}
	if mergeSha == "" {
		t.Fatal("no merge commit on integration")
	}
	eq(t, trailers(reportSha), "completion-report:A", "the merge's second parent is the report's commit")
	eq(t, r.git("log", "-1", "--format=%s", reportSha+"^"), "fix", "the report builds on the code fix")
	match(t, r.git("show", reportSha+":docs/plans/A-completion.md"), `^# A — completion report\n`)

	// Cleanup (D25): every worktree of A's stages is gone, the ignored file
	// the implement stage left with it, listed, and every branch: A is
	// complete. The relaunch below replays A without them.
	if ents, _ := os.ReadDir(filepath.Join(cfg.StateDir, "wt")); len(ents) != 0 {
		t.Errorf("worktrees left: %v", ents)
	}
	eq(t, r.git("for-each-ref", "--format=%(refname:short)", "refs/heads/lsf/"), "", "branches after cleanup")
	logText, _ := os.ReadFile(filepath.Join(cfg.StateDir, "log.txt"))
	match(t, string(logText), `cleanup:w1: implement:A: removed .*build\.log`)
	eq(t, r.git("log", "-1", "--format=%(trailers:key=Lsf-Unit,valueonly)", mergeSha), "A")
	match(t, trailers(mergeSha), `^merge:w1$`)
	book := r.git("log", "-1", "--format=%H", "integration", "--", "docs/plans/f-archive.md")
	match(t, trailers(book), `^merge:w1$`)
	eq(t, r.git("show", "--name-only", "--format=", book), "docs/plans/f-archive.md\ndocs/plans/f-breakdown.md")
	contains(t, r.git("show", book+":docs/plans/f-breakdown.md"), "### A — complete at "+mergeSha+"; entry archived to docs/plans/f-archive.md; report docs/plans/A-completion.md")
	contains(t, r.git("show", book+":docs/plans/f-breakdown.md"), "    complete: true")
	match(t, r.git("show", book+":docs/plans/f-archive.md"), `^# Archive: f\n\n## Work Units\n\n### A\n`)

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
			if slices.Contains([]string{"load", "decision-log", "ledger", "completion-report", "merge"}, prefix) {
				t.Errorf("agent call %s", e.Label)
			}
		case "step":
			stepsRun[prefix] = true
		}
	}
	for _, p := range []string{"load", "decision-log", "ledger", "completion-report", "merge"} {
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
