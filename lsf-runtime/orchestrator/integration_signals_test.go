package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// answer is one fake-claude table entry.
type answer struct {
	Contains  string   `json:"contains"`
	Return    any      `json:"return"`
	Shell     string   `json:"shell,omitempty"`
	Denials   []string `json:"denials,omitempty"`
	FailFirst bool     `json:"failFirst,omitempty"`
}

// realRun is one unit, A, run end to end on the real runtime over a real
// repository with the fake claude. over replaces the default answer whose
// Contains it shares. It returns the outcome, the integration branch's
// ledger and the state dir.
func realRun(t *testing.T, over ...answer) (view, string, string, *gitRepo) {
	t.Helper()
	r := newGitRepo(t)
	const planPath = "docs/plans/f-breakdown.md"
	p := orchtest.Graph("", orchtest.Unit("A"))
	doc := func(p *plan.Plan) string { return "# Plan: f\n\n" + orchtest.ToBlock(p) + "\n" }
	r.commit("docs/architecture/f.md", "# Architecture\n", "architecture")
	planSha := r.commit(planPath, doc(p), "plan")
	r.git("checkout", "-q", "-b", "work-A")
	workTip := r.commit("src/A/a.go", "package a\n", "unit A")
	r.git("checkout", "-q", "-b", "side", "integration")
	done := *p
	done.Units = []*plan.Unit{orchtest.Unit("A")}
	done.Units[0].Complete = true
	completeSha := r.commit(planPath, doc(&done), "A complete")
	r.git("checkout", "-q", "integration")

	answers := []answer{
		{Contains: "Write the implementation plan for this unit", Return: R(M{"branch": "work-A", "sha": workTip[:7], "path": "docs/plans/A-plan.md", "bytes": 0, "readings": []any{}})},
		{Contains: "Critique the implementation plan at", Return: orchtest.NoFindings()},
		{Contains: "Run the phronithm:critique skill with type critique-code", Return: orchtest.NoFindings()},
		{Contains: "Run the phronithm:review skill", Return: orchtest.NoFindings()},
		{Contains: "Implementation plan: docs/plans/", Return: R(orchtest.Implemented("A", M{"branch": "work-A", "sha": workTip}))},
		{Contains: "Merge each unit branch below into", Return: R(M{"integrationBranch": "integration", "merges": []plan.Merge{{Unit: "A", Status: "clean", Files: []string{}, Sha: "m"}},
			"planSha": completeSha, "pauseRequested": false, "architectureSha": ""})},
		{Contains: "has merged these units into", Return: R(M{"invalidations": []any{}, "notes": []any{}, "amendments": []any{}})},
		{Contains: "Cleanup stage, wave", Return: R(M{"removed": 0, "kept": []any{}})},
		{Contains: "Read its Meta-Tasks and Standing Obligations sections", Return: R(M{"planSha": completeSha, "appendedUnits": []any{}, "directEdits": []any{}})},
		{Contains: "For each standing obligation", Return: R(M{"obligations": []any{}})},
	}
	for _, o := range over {
		for i := range answers {
			if answers[i].Contains == o.Contains {
				answers[i] = o
			}
		}
	}
	dir := t.TempDir()
	tablePath := filepath.Join(dir, "table.json")
	b, err := plan.MarshalJS(answers, "")
	if err != nil {
		t.Fatal(err)
	}
	b = []byte(strings.ReplaceAll(string(b), "@workTip", workTip)) // an answer's stand-in for work-A's tip
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
		Roles:       map[string]lsfrt.Role{"worker": {Tools: "default", PermissionMode: "dontAsk"}},
		DefaultRole: "worker",
		Backoff:     []lsfrt.Duration{lsfrt.Duration(10 * time.Millisecond)},
	}
	sum, err := lsfrt.Run(context.Background(), cfg, Body)
	if err != nil {
		t.Fatal(err)
	}
	if sum.StopReason != lsfrt.StopReturned {
		t.Fatalf("stop reason %s: %s %s", sum.StopReason, sum.Detail, sum.Error)
	}
	var out view
	if err := json.Unmarshal(sum.Outcome, &out); err != nil {
		t.Fatal(err)
	}
	out.raw = string(sum.Outcome)
	return out, r.git("show", "integration:docs/plans/f-signals/"+planSha+".md"), cfg.StateDir, r
}

func journaled(t *testing.T, stateDir, prefix string) bool {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(stateDir, "journal", prefix+"*.json"))
	return len(m) > 0
}

// S3(a), postmortem §C, simulated from its evidence: an implement whose
// git commit is denied leaves its change uncommitted and reports HEAD (its
// branch's tip) as its SHA, which matches, so no SHA check can catch it. In
// the postmortem the SHA was recorded as the stage's commit; here the run
// stops before review, the denial in the ledger.
func TestOnRuntimeDeniedCommit(t *testing.T) {
	out, ledger, state, _ := realRun(t, answer{
		Contains: "Implementation plan: docs/plans/",
		Return:   R(orchtest.Implemented("A", M{"branch": "work-A", "sha": "@workTip"})),
		Shell:    "echo 'package a // more' > a2.go",
		Denials:  []string{"git commit -m 'feat(A): implement'"},
	})
	eq(t, out.Status, "stopped", out.raw)
	eqs(t, out.stopKinds(), []string{"uncommitted-at-return"})
	contains(t, out.stops()[0].Statement, "a2.go")
	match(t, ledger, `\| implement \| structural \| permission-denied \| 1 permission denial\(s\): Bash: \{"command":"git commit`)
	if journaled(t, state, "critique-code") || journaled(t, state, "lens-review") {
		t.Error("reviewed after the stop")
	}
	noMatch(t, ledger, `sha-corrected`, "HEAD is the tip: the SHA matches")
}

// S3(b), postmortem §C: a cleanup whose git branch -d is denied. In the
// postmortem the branches were left with no signal; here the ledger
// records the denial and the run completes.
func TestOnRuntimeDeniedBranchDelete(t *testing.T) {
	out, ledger, _, _ := realRun(t, answer{
		Contains: "Cleanup stage, wave",
		Return:   R(M{"removed": 0, "kept": []string{"lsf/x/implement_A"}}),
		Denials:  []string{"git branch -d lsf/x/implement_A"},
	})
	eq(t, out.Status, "complete", out.raw)
	match(t, ledger, `\| cleanup \| structural \| permission-denied \| 1 permission denial\(s\): Bash: \{"command":"git branch -d`)
}

// S4, run #2: an implement whose first attempt is an API 529 is retried;
// in the postmortem it was an agent-null stop.
func TestOnRuntimeTransientRetried(t *testing.T) {
	out, ledger, state, _ := realRun(t, answer{
		Contains:  "Implementation plan: docs/plans/",
		Return:    R(orchtest.Implemented("A", M{"branch": "work-A"})),
		FailFirst: true,
	})
	eq(t, out.Status, "complete", out.raw)
	noMatch(t, ledger, `agent-null`)
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(state), "calls", "failed-*")); len(m) != 1 {
		t.Errorf("the first attempt did not fail: %v", m)
	}
}

// checkSha against a real repository.
func TestCheckSha(t *testing.T) {
	r := newGitRepo(t)
	tip := r.git("rev-parse", "integration")
	o := &orch{git: execGit{}, repo: r.dir}
	at := plan.At{Stage: "implement", Unit: "A", Wave: plan.W(1)}
	ctx := context.Background()
	for _, tc := range []struct {
		name, branch, reported, want string
		kind                         string // "" for no signal
		stmt                         string
	}{
		{"short prefix", "integration", tip[:7], tip[:7], "", ""},
		{"full", "integration", tip, tip, "", ""},
		{"upper case", "integration", strings.ToUpper(tip[:8]), tip[:8], "", ""},
		{"mismatch", "integration", "deadbee", tip, "sha-corrected", "integration: reported deadbee, tip " + tip + "; corrected"},
		{"empty", "integration", "", tip, "sha-corrected", "integration: reported no SHA, tip " + tip + "; corrected"},
		{"too short", "integration", tip[:4], tip, "sha-corrected", ""},
		{"longer than the tip", "integration", tip + "0", tip, "sha-corrected", ""},
		{"unresolvable branch", "nope", "deadbee", "deadbee", "", ""},
		{"revision syntax", "integration~1", "deadbee", "deadbee", "", ""},
	} {
		got, sig, err := o.checkSha(ctx, at, tc.branch, tc.reported)
		if err != nil || got != tc.want || (sig == nil) != (tc.kind == "") || (sig != nil && (sig.Kind != tc.kind || (tc.stmt != "" && sig.Statement != tc.stmt))) {
			t.Errorf("%s: %q %+v %v", tc.name, got, sig, err)
		}
	}
	// A git error other than an unresolved ref records that it could not check.
	broken := &orch{git: execGit{}, repo: filepath.Join(t.TempDir(), "missing")}
	got, sig, err := broken.checkSha(ctx, at, "integration", "deadbee")
	if err != nil || got != "deadbee" || sig == nil || !strings.Contains(sig.Statement, "not checked") {
		t.Errorf("git error: %q %+v %v", got, sig, err)
	}
}

// S6 on the live path: a merge status outside the enum, on every attempt,
// is a schema-violation stop beside agent-null, never a claimed-disjoint
// conflict.
func TestOnRuntimeLiveSchemaViolation(t *testing.T) {
	out, ledger, _, _ := realRun(t, answer{
		Contains: "Merge each unit branch below into",
		Return: R(M{"integrationBranch": "integration", "merges": []plan.Merge{{Unit: "A", Status: "weird", Files: []string{}, Sha: "m"}},
			"planSha": "x", "pauseRequested": false, "architectureSha": ""}),
	})
	eq(t, out.Status, "stopped", out.raw)
	eqs(t, out.stopKinds(), []string{"schema-violation", "agent-null"})
	match(t, out.stops()[0].Statement, `status: "weird" not in enum`)
	noMatch(t, ledger, `claimed-disjoint-conflict`)
}

// S8: an implement reporting a SHA that is not its branch's tip is
// corrected from git and recorded; the run continues.
func TestOnRuntimeShaCorrected(t *testing.T) {
	out, ledger, _, r := realRun(t, answer{
		Contains: "Implementation plan: docs/plans/",
		Return:   R(orchtest.Implemented("A", M{"branch": "work-A", "sha": "deadbee"})),
	})
	eq(t, out.Status, "complete", out.raw)
	tip := r.git("rev-parse", "work-A")
	contains(t, ledger, "| implement | structural | sha-corrected | work-A: reported deadbee, tip "+tip+"; corrected | work-A |")
	noMatch(t, ledger, `\| plan \| structural \| sha-corrected`, "a correct short SHA is not corrected")
}
