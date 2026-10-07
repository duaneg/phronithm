package orchestrator

import (
	"strings"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

// bookPlan is a plan whose B entry holds a fence with heading-like lines,
// whose block gives A a complete: line and B none, and whose Plan
// Maintenance is followed by another section.
const bookPlan = "# Plan: f\n\n## Work Units\n\n### A\n- **Type**: feature\n- **Done condition**: tests pass.\n\n### B\n- **Type**: refactor\n\n```text\n## not a heading\n### nor this\n```\n\nB's last line.\n\n### C\n- **Type**: feature\n\n## Graph\n\n```yaml lsf-plan\nunits:\n  - id: A\n    type: feature\n    complete: false # set at merge\n    dependencies: []\n    scope: [a]\n    publishes: []\n    gates: []\n  - type: refactor\n    id: \"B\"\n    dependencies: [A]\n    scope: [b]\n    publishes: []\n    gates:\n      - name: legal\n        kind: organisational\n  - id: C\n    type: feature\n    dependencies: []\n    scope: [c]\n    publishes: []\n    gates: []\nexecution_order: [A, B, C]\n```\n\n## Plan Maintenance\n\n### 2026-10-01 — re-plan\nnew-edge B — after A\n\n## Risk Registry\n\n- R1 none.\n"

var bookCtx = bookIn{Name: "f", ArchivePath: "docs/plans/f-archive.md", LedgerPath: "docs/plans/f-signals/x.md", Wave: 2}

func mergedBook(units ...string) []bookUnit {
	out := []bookUnit{}
	for _, u := range units {
		out = append(out, bookUnit{Unit: u, Sha: strings.Repeat(strings.ToLower(u), 40), Report: "docs/plans/" + u + "-completion.md"})
	}
	return out
}

func TestRenderBookkeeping(t *testing.T) {
	p, a, err := renderBookkeeping([]byte(bookPlan), nil, false, bookCtx, mergedBook("A", "B"), "2026-10-07")
	if err != nil {
		t.Fatal(err)
	}
	wantPlan := strings.NewReplacer(
		"### A\n- **Type**: feature\n- **Done condition**: tests pass.\n\n", "### A — complete at "+strings.Repeat("a", 40)+"; entry archived to docs/plans/f-archive.md; report docs/plans/A-completion.md\n\n",
		"### B\n- **Type**: refactor\n\n```text\n## not a heading\n### nor this\n```\n\nB's last line.\n\n", "### B — complete at "+strings.Repeat("b", 40)+"; entry archived to docs/plans/f-archive.md; report docs/plans/B-completion.md\n\n",
		"    complete: false # set at merge\n", "    complete: true # set at merge\n",
		"    id: \"B\"\n", "    id: \"B\"\n    complete: true\n",
		"new-edge B — after A\n\n## Risk", "new-edge B — after A\n\n### 2026-10-07 — A complete\nReport: docs/plans/A-completion.md @ "+strings.Repeat("a", 40)+". Ledger: docs/plans/f-signals/x.md wave 2.\n\n### 2026-10-07 — B complete\nReport: docs/plans/B-completion.md @ "+strings.Repeat("b", 40)+". Ledger: docs/plans/f-signals/x.md wave 2.\n\n## Risk",
	).Replace(bookPlan)
	eq(t, string(p), wantPlan)
	eq(t, string(a), "# Archive: f\n\n## Work Units\n\n### A\n- **Type**: feature\n- **Done condition**: tests pass.\n\n### B\n- **Type**: refactor\n\n```text\n## not a heading\n### nor this\n```\n\nB's last line.\n")

	g := plan.LoadGraph(string(p), nil)
	if g.Plan == nil || !g.Plan.Units[0].Complete || !g.Plan.Units[1].Complete || g.Plan.Units[2].Complete {
		t.Errorf("flags after: %+v", g)
	}
}

// Archives that exist: entries go at the end of `## Work Units`, before a
// later section; an archive with no such section gains one at the end.
func TestRenderBookkeepingArchives(t *testing.T) {
	entryC := "### C\n- **Type**: feature\n"
	for _, tc := range []struct{ name, old, want string }{
		{"work units then closed risks",
			"# Archive: f\n\n## Work Units\n\n### Z\nold\n\n## Closed risks\n\n- R0 gone.\n",
			"# Archive: f\n\n## Work Units\n\n### Z\nold\n\n" + entryC + "\n## Closed risks\n\n- R0 gone.\n"},
		{"closed risks only",
			"# Archive: f\n\n## Closed risks\n\n- R0 gone.\n",
			"# Archive: f\n\n## Closed risks\n\n- R0 gone.\n\n## Work Units\n\n" + entryC},
		{"no trailing newline",
			"# Archive: f\n\n## Work Units\n\n### Z\nold",
			"# Archive: f\n\n## Work Units\n\n### Z\nold\n\n" + entryC},
	} {
		_, a, err := renderBookkeeping([]byte(bookPlan), []byte(tc.old), true, bookCtx, mergedBook("C"), "2026-10-07")
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		eq(t, string(a), tc.want, tc.name)
	}
}

// Refusals: no entry, no Plan Maintenance, no Work Units, an id the block
// lacks, and a block that would not parse back.
func TestRenderBookkeepingRefusals(t *testing.T) {
	for _, tc := range []struct{ name, plan, unit, err string }{
		{"no entry", bookPlan, "Z", "unit Z has no `### Z` entry under `## Work Units`"},
		{"no plan maintenance", strings.Replace(bookPlan, "## Plan Maintenance", "## Maintenance", 1), "A", "no `## Plan Maintenance` section"},
		{"no work units", strings.Replace(bookPlan, "## Work Units", "## Units", 1), "A", "no `## Work Units` section"},
		{"entry but no block item", strings.Replace(bookPlan, "  - id: C\n", "  - id: D\n", 1), "C", "unit C has no item in the plan block"},
		{"two items", strings.Replace(bookPlan, "  - id: C\n", "  - id: A\n", 1), "A", "more than one item"},
		{"broken block", strings.Replace(bookPlan, "    scope: [a]\n", "    scope: [a\n", 1), "A", "does not parse"},
		{"two blocks", bookPlan + "\n```yaml lsf-plan\nunits: []\n```\n", "A", "exactly one is allowed"},
	} {
		_, _, err := renderBookkeeping([]byte(tc.plan), nil, false, bookCtx, mergedBook(tc.unit), "2026-10-07")
		if err == nil || !strings.Contains(err.Error(), tc.err) {
			t.Errorf("%s: %v, want %q", tc.name, err, tc.err)
		}
	}
}

// The stub and maintenance forms are the JS merge prompt's.
func TestBookkeepingFormsMatchJSPrompt(t *testing.T) {
	u := mergedBook("A")[0]
	eq(t, stubLine(bookCtx, u), "### A — complete at "+u.Sha+"; entry archived to docs/plans/f-archive.md; report docs/plans/A-completion.md")
	eq(t, maintenanceEntry(bookCtx, u, "2026-10-07"), "### 2026-10-07 — A complete\nReport: docs/plans/A-completion.md @ "+u.Sha+". Ledger: docs/plans/f-signals/x.md wave 2.\n")
}
