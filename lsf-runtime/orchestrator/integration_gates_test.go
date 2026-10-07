package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// Declared gates end to end on the real runtime: the runtime gates the
// implement commit (red: the stage's green is misreported), the code fix's
// commit (green) and, by reuse, the merge, on clean checkouts; the reviewers read its
// results; a relaunch replays every gate run from the journal.
func TestOnRuntimeDeclaredGates(t *testing.T) {
	r := newGitRepo(t)
	const planPath = "docs/plans/f-breakdown.md"
	counter := filepath.Join(t.TempDir(), "gate-runs")
	gates := "gates:\n  - name: fixed\n    command: \"echo run >> " + counter + "; grep -q fixed src/A/a.go\"\n"
	p := orchtest.Graph("", orchtest.Unit("A"))
	r.commit(".gitignore", "*.log\n", "ignore logs")
	r.commit("docs/architecture/f.md", "# Architecture\n", "architecture")
	planSha := r.commit(planPath, planDocFor(p)+orchtest.GatesBlock(gates), "plan")
	r.git("checkout", "-q", "-b", "work-A")
	r.commit("src/A/a.go", "package a\n", "unit A")
	r.git("checkout", "-q", "integration")

	answers := []answer{
		{Contains: "Write the implementation plan for this unit", Shell: "git reset -q --hard work-A", Return: R(M{"branch": "@branch", "sha": "@head", "path": "docs/plans/A-plan.md", "bytes": 0, "readings": []any{}})},
		{Contains: "Critique the implementation plan at", Return: orchtest.NoFindings()},
		{Contains: "Run the phronithm:critique skill with type critique-code", Once: true,
			Return: R(M{"findings": []plan.Finding{orchtest.Finding("Critical", "gate red", "src/A/a.go:1")}, "systemic": false, "summary": "s"})},
		{Contains: "Run the phronithm:critique skill with type critique-code", Return: orchtest.NoFindings()},
		{Contains: "Run the phronithm:review skill", Return: orchtest.NoFindings()},
		{Contains: "Critique gate round 1 found these Critical findings against the code", Shell: "git reset -q --hard work-A && echo '// fixed' >> src/A/a.go && git -c user.name=t -c user.email=t@x commit -qam fix",
			Return: R(M{"branch": "@branch", "sha": "@head", "applied": []M{{"finding": "gate red", "fix": "fixed"}}, "notApplied": []any{}, "gates": []any{}})},
		{Contains: "Implementation plan: docs/plans/", Shell: "git reset -q --hard work-A", Return: R(orchtest.Implemented("A", M{"branch": "@branch", "sha": "@head",
			"gates": []orchtest.Gate{{Command: "echo run >> " + counter + "; grep -q fixed src/A/a.go", Status: "green", Summary: "trust me"}}}))},
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
	args, _ := plan.MarshalJS(M{"name": "f", "planSha": planSha, "architectureDoc": "docs/architecture/f.md", "integrationBranch": "integration"}, "")
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
	gateRuns := func() int {
		b, _ := os.ReadFile(counter)
		return strings.Count(string(b), "run")
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
	eq(t, gateRuns(), 2, "the implement commit and the fix's; the merge reuses the fix's result")
	logText, _ := os.ReadFile(filepath.Join(cfg.StateDir, "log.txt"))
	match(t, string(logText), `merge of A: its gates at [0-9a-f]+ stand \(the merge differs only at docs/plans/A-completion\.md\)`)
	eq(t, out.Digest.ByKind["gate-misreported"], 1, "the implement stage's green against the runtime's red")

	ledger := r.git("show", "integration:docs/plans/f-signals/"+planSha+".md")
	contains(t, ledger, "gate-misreported")
	mergeSha := ""
	for _, line := range strings.Split(r.git("log", "--first-parent", "--format=%H %P", "integration"), "\n") {
		if f := strings.Fields(line); len(f) == 3 {
			mergeSha = f[0]
			break
		}
	}
	if mergeSha == "" {
		t.Fatal("no merge commit on integration")
	}
	report := r.git("show", mergeSha+":docs/plans/A-completion.md")
	contains(t, report, "the gates below are the runtime's at the last fix")
	contains(t, report, "— green")
	if ents, _ := os.ReadDir(filepath.Join(cfg.StateDir, "gates")); len(ents) != 1 || ents[0].Name() != "logs" {
		t.Errorf("gate checkouts left: %v", ents)
	}
	if strings.Contains(r.git("worktree", "list"), "/gates/") {
		t.Error("a gate worktree is still registered")
	}
	eq(t, r.git("status", "--porcelain"), "", "the main checkout is clean")

	tips := r.git("for-each-ref", "--format=%(refname) %(objectname)") + "\nHEAD " + r.head()
	again := launch()
	eq(t, again.StopReason, lsfrt.StopReturned)
	eq(t, string(again.Outcome), string(first.Outcome))
	eq(t, again.Calls, 0, "the relaunch replays every call")
	eq(t, gateRuns(), 2, "every gate run replayed")
	eq(t, r.git("for-each-ref", "--format=%(refname) %(objectname)")+"\nHEAD "+r.head(), tips, "no new commit")
}

// A merge that regresses is not journaled: a relaunch reruns the merge
// step. Its gates replay for an unchanged merge tree, and once the
// requester fixes the environment and removes the red record
// (docs/plan-graph.md), the relaunch gates again and merges.
func TestOnRuntimeRegressedMergeRelaunch(t *testing.T) {
	r := newGitRepo(t)
	const planPath = "docs/plans/f-breakdown.md"
	allow := filepath.Join(t.TempDir(), "allow")
	counter := filepath.Join(t.TempDir(), "gate-runs")
	// Red only on a tree holding the decision log, which the unit's branch
	// predates: the unit gates green, its merge red.
	gates := "gates:\n  - name: env\n    command: \"echo run >> " + counter + "; test -e " + allow + " || ! test -e docs/plans/f-decisions.md\"\n"
	p := orchtest.Graph("", orchtest.Unit("A"))
	r.commit("docs/architecture/f.md", "# Architecture\n", "architecture")
	planSha := r.commit(planPath, planDocFor(p)+orchtest.GatesBlock(gates), "plan")
	r.git("checkout", "-q", "-b", "work-A")
	r.commit("src/A/a.go", "package a\n", "unit A")
	r.git("checkout", "-q", "integration")

	answers := []answer{
		{Contains: "The requester has answered the questions", Return: R(M{"planSha": planSha, "amendments": []any{}, "overBudget": []any{},
			"verdicts": []M{{"id": "d1", "verdict": "no-change", "statement": "noted"}}, "directEdits": []any{}, "summary": "applied"})},
		{Contains: "Write the implementation plan for this unit", Shell: "git reset -q --hard work-A", Return: R(M{"branch": "@branch", "sha": "@head", "path": "docs/plans/A-plan.md", "bytes": 0, "readings": []any{}})},
		{Contains: "Critique the implementation plan at", Return: orchtest.NoFindings()},
		{Contains: "Run the phronithm:critique skill with type critique-code", Return: orchtest.NoFindings()},
		{Contains: "Run the phronithm:review skill", Return: orchtest.NoFindings()},
		{Contains: "Implementation plan: docs/plans/", Shell: "git reset -q --hard work-A", Return: R(orchtest.Implemented("A", M{"branch": "@branch", "sha": "@head", "gates": []any{}}))},
		{Contains: "has merged these units into", Return: R(M{"invalidations": []any{}, "notes": []any{}, "amendments": []any{}})},
		{Contains: "Read its Meta-Tasks and Standing Obligations sections", Return: R(M{"planSha": "@head", "appendedUnits": []any{}, "directEdits": []any{}})},
		{Contains: "For each standing obligation", Return: R(M{"obligations": []any{}})},
	}
	dir := t.TempDir()
	tablePath := filepath.Join(dir, "table.json")
	b, _ := plan.MarshalJS(answers, "")
	os.WriteFile(tablePath, b, 0o644)
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
	launch := func() view {
		t.Helper()
		sum, err := lsfrt.Run(context.Background(), cfg, Body)
		if err != nil || sum.StopReason != lsfrt.StopReturned {
			t.Fatalf("launch: %v %+v", err, sum)
		}
		var v view
		if err := json.Unmarshal(sum.Outcome, &v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	first := launch()
	eq(t, strings.Join(first.stopKinds(), ","), "merge-regression", first.raw)
	contains(t, first.stops()[0].Statement, "failed its gates")
	eq(t, r.git("log", "--merges", "--oneline", "integration"), "", "nothing merged")
	runs := func() int { b, _ := os.ReadFile(counter); return strings.Count(string(b), "run") }
	gated := runs()

	// Unchanged, relaunches regress again. The first stop's ledger commit
	// may move the branch (a new tree, gated); once it stands still the red
	// record replays.
	launch()
	gated = runs()
	unchanged := launch()
	eq(t, strings.Join(unchanged.stopKinds(), ","), "merge-regression")
	eq(t, runs(), gated, "the red gate record replays")

	os.WriteFile(allow, nil, 0o644)
	records, _ := filepath.Glob(filepath.Join(cfg.StateDir, "journal", "gates_*.json"))
	for _, rec := range records {
		if b, _ := os.ReadFile(rec); strings.Contains(string(b), `"status":"red"`) {
			os.Remove(rec)
		}
	}
	again := launch()
	eq(t, again.Status, "complete", again.raw)
	eq(t, r.git("log", "--merges", "--format=%(trailers:key=Lsf-Unit,valueonly)", "integration"), "A")
}
