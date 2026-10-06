package orchestrator

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

type M = orchtest.M

// implOf is the implement result Implemented(u, over) decodes to.
func implOf(t *testing.T, u string, over M) *implementResult {
	t.Helper()
	b, err := plan.MarshalJS(orchtest.Implemented(u, over), "")
	if err != nil {
		t.Fatal(err)
	}
	var r implementResult
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return &r
}

// js: renderCompletionReport renders the implement and review returns section by section, with finding counts pointing at the ledger
func TestRenderCompletionReport(t *testing.T) {
	c := &runCtx{ledgerPath: "docs/plans/f-signals/plan0.md"}
	u := &plan.Unit{ID: "A", Type: "feature"}
	impl := implOf(t, "A", M{
		"gateProbe":  []M{{"name": "fontforge", "pin": "20230101", "observed": "20230101", "drifted": false}},
		"deviations": []string{"sibling predicate instead of widening"}, "writesOutsideScope": []string{"CLAUDE.md", "src/x.py"},
		"deferrals":           []M{{"statement": "export left unrotated", "belongsTo": "B"}},
		"discoveries":         []M{{"statement": "B needs an edge to C", "affects": "B"}},
		"doneCondition":       M{"evaluable": true, "met": true, "evidence": "clause (i): test_a green; clause (ii): 3 of 3 fixtures."},
		"standingObligations": []M{{"statement": "the diff of a regenerated artefact is read", "status": "discharged"}},
		"summary":             "Widened the predicate and pinned it.",
	})
	review := reviewOutcome{rounds: 2, findings: []plan.Finding{
		{Severity: "Significant", Statement: "fragile", Location: "a.py:2", Stage: "critique-code", Round: 1},
		{Severity: "Minor", Statement: "name", Location: "a.py:9"}, {Severity: "Minor", Statement: "doc", Location: "a.py:12"},
	}}
	text := renderCompletionReport(c, u, 1, "impl-A", impl, review)
	want := strings.Join([]string{
		"# A — completion report", "",
		"Unit `A` (feature), wave 1, branch `impl-A`.", "",
		"Widened the predicate and pinned it.", "",
		"## Done-condition verdict", "",
		"**Met**. clause (i): test_a green; clause (ii): 3 of 3 fixtures.", "",
		"External gates re-verified: fontforge pinned 20230101, observed 20230101.", "",
		"Gates at HEAD:", "",
		"- `pytest -q` — green: 412 passed\n- `ruff check .` — green", "",
		"## Deviations and writes outside Scope", "",
		"- sibling predicate instead of widening", "",
		"Written outside the Scope line: `CLAUDE.md`, `src/x.py`.", "",
		"## Deferrals", "",
		"- export left unrotated → `B`", "",
		"## Discoveries", "",
		"- B needs an edge to C — affects `B`", "",
		"## Standing obligations", "",
		"- the diff of a regenerated artefact is read — discharged", "",
		"Review: 2 round(s); 1 Significant and 2 Minor finding(s) left unfixed are recorded in docs/plans/f-signals/plan0.md, wave 1.", "",
	}, "\n")
	if text != want {
		t.Errorf("report\n%s\nwant\n%s", text, want)
	}
	if regexp.MustCompile(`fragile|a\.py`).MatchString(text) {
		t.Error("findings are counts; the rows live in the ledger")
	}

	bare := renderCompletionReport(c, u, 2, "impl-A", implOf(t, "A", M{"doneCondition": M{"evaluable": false, "met": false, "evidence": "the clause names a file that does not exist"}}), reviewOutcome{rounds: 1})
	if !strings.Contains(bare, "**Not met** (not evaluable as written). the clause names a file that does not exist") {
		t.Error(bare)
	}
	if n := len(regexp.MustCompile(`(?m)^None\.$`).FindAllString(bare, -1)); n != 4 {
		t.Errorf("every empty section says so: %d", n)
	}
	if regexp.MustCompile(`External gates|Written outside|breakdown|re-plan mode`).MatchString(bare) {
		t.Error(bare)
	}
	if strings.Contains(renderCompletionReport(c, u, 2, "impl-A", implOf(t, "A", M{"gates": []any{}}), reviewOutcome{rounds: 1}), "Gates at HEAD") {
		t.Error("no gates, no section")
	}
	afterFix := renderCompletionReport(c, u, 1, "impl-A", impl, reviewOutcome{rounds: 2, gates: []gateRun{{"pytest -q", "green", "99 passed (fix)"}}, fixedGates: true})
	if !strings.Contains(afterFix, "Gates at HEAD:\n\n- `pytest -q` — green: 99 passed (fix)\n") || strings.Contains(afterFix, "412 passed") {
		t.Error("after a fix, the fix's runs are the gates at HEAD:\n" + afterFix)
	}
	if !strings.Contains(afterFix, "**Met**, as the implement stage evaluated it; the gates below are the last fix's. clause (i)") {
		t.Error(afterFix)
	}
}

// js: the long-gate recipe detaches, captures a compound command's output and status, and ends the poll when the gate dies
func TestLongGateRecipe(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	cmds := regexp.MustCompile("`D=[^`]*`").FindAllString(longGates, -1)
	if len(cmds) != 2 {
		t.Fatalf("recipe commands %q", cmds)
	}
	detach, poll := strings.Trim(cmds[0], "`"), strings.Trim(cmds[1], "`")
	dir := t.TempDir()
	sh := func(cmd string) string {
		t.Helper()
		c := exec.Command("bash", "-c", cmd)
		c.Dir = dir
		out, err := c.Output()
		if err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
		return string(out)
	}
	sh("git init -q")
	D := strings.TrimSpace(sh("git rev-parse --absolute-git-dir"))
	gate := func(n, command string) string {
		return strings.Replace(strings.ReplaceAll(detach, "<n>", n), "<command>", command, 1)
	}
	exists := func(p string) bool { _, err := os.Stat(filepath.Join(D, p)); return err == nil }

	t0 := time.Now()
	sh(gate("1", "sleep 6 && echo a && echo b >&2 && false"))
	if time.Since(t0) > 5*time.Second {
		t.Error("the detach call does not wait for the gate")
	}
	if exists("lsf-gate-1.rc") {
		t.Error("the gate is still running after the detach returns")
	}
	for deadline := time.Now().Add(20 * time.Second); !exists("lsf-gate-1.rc") && time.Now().Before(deadline); {
		time.Sleep(100 * time.Millisecond)
	}
	sh(strings.ReplaceAll(poll, "<n>", "1"))
	if rc, _ := os.ReadFile(filepath.Join(D, "lsf-gate-1.rc")); strings.TrimSpace(string(rc)) != "1" {
		t.Errorf("the status of the compound command: %q", rc)
	}
	if log, _ := os.ReadFile(filepath.Join(D, "lsf-gate-1.log")); string(log) != "a\nb\n" {
		t.Errorf("both parts' output, stdout and stderr: %q", log)
	}
	if s := sh("git status --porcelain"); s != "" {
		t.Errorf("nothing in the work tree: %q", s)
	}

	sh(gate("2", "sleep 30"))
	pid, _ := os.ReadFile(filepath.Join(D, "lsf-gate-2.pid"))
	// Stop the subshell first, so it cannot write the status file between
	// its child's death and its own.
	p := strings.TrimSpace(string(pid))
	sh("kill -STOP " + p + "; pkill -KILL -P " + p + "; kill -KILL " + p)
	t1 := time.Now()
	sh(strings.ReplaceAll(poll, "<n>", "2"))
	if time.Since(t1) > 20*time.Second {
		t.Error("the poll ends when the gate dies")
	}
	if exists("lsf-gate-2.rc") {
		t.Error("a dead gate leaves no status file: not-run")
	}
}
