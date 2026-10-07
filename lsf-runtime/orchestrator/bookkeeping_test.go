package orchestrator

import (
	"fmt"
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
		out = append(out, bookUnit{Unit: u, Sha: strings.Repeat(fmt.Sprintf("%x", u[0]%16), 40), Report: "docs/plans/" + u + "-completion.md"})
	}
	return out
}

func TestRenderBookkeeping(t *testing.T) {
	p, a, err := renderBookkeeping([]byte(bookPlan), nil, false, bookCtx, mergedBook("A", "B"), "2026-10-07")
	if err != nil {
		t.Fatal(err)
	}
	wantPlan := strings.NewReplacer(
		"### A\n- **Type**: feature\n- **Done condition**: tests pass.\n\n", "### A — complete at "+mergedBook("A")[0].Sha+"; entry archived to docs/plans/f-archive.md; report docs/plans/A-completion.md\n\n",
		"### B\n- **Type**: refactor\n\n```text\n## not a heading\n### nor this\n```\n\nB's last line.\n\n", "### B — complete at "+mergedBook("B")[0].Sha+"; entry archived to docs/plans/f-archive.md; report docs/plans/B-completion.md\n\n",
		"    complete: false # set at merge\n", "    complete: true # set at merge\n",
		"    id: \"B\"\n", "    id: \"B\"\n    complete: true\n",
		"new-edge B — after A\n\n## Risk", "new-edge B — after A\n\n### 2026-10-07 — A complete\nReport: docs/plans/A-completion.md @ "+mergedBook("A")[0].Sha+". Ledger: docs/plans/f-signals/x.md wave 2.\n\n### 2026-10-07 — B complete\nReport: docs/plans/B-completion.md @ "+mergedBook("B")[0].Sha+". Ledger: docs/plans/f-signals/x.md wave 2.\n\n## Risk",
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

// Inputs whose boundaries the scanner cannot be sure of are refused, not
// guessed at: a setext or indented heading inside an entry or Plan
// Maintenance, a duplicate section or entry, an entry already archived,
// lines that would break a rendered line.
func TestRenderBookkeepingAmbiguity(t *testing.T) {
	for _, tc := range []struct{ name, plan, archive, unit, err string }{
		{"setext heading in the entry", strings.Replace(bookPlan, "- **Done condition**: tests pass.\n", "- **Done condition**: tests pass.\n\nGraph\n-----\n", 1), "", "A", `the entry of A holds a line that could be a heading ("-----")`},
		{"indented heading in the entry", strings.Replace(bookPlan, "- **Done condition**: tests pass.\n", "- **Done condition**: tests pass.\n  ## Graph\n", 1), "", "A", `could be a heading ("  ## Graph")`},
		{"setext heading in Plan Maintenance", strings.Replace(bookPlan, "new-edge B — after A\n", "new-edge B — after A\n\nRisks\n=====\n", 1), "", "A", "`## Plan Maintenance` holds a line that could be a heading"},
		{"two Work Units sections", bookPlan + "\n## Work Units\n", "", "A", "two `## Work Units` sections"},
		{"two entries", strings.Replace(bookPlan, "### C\n", "### A\n", 1), "", "A", "unit A has two `### A` entries"},
		{"already archived", bookPlan, "# Archive: f\n\n## Work Units\n\n### A\nold\n", "A", "the archive already holds a `### A` entry"},
	} {
		_, _, err := renderBookkeeping([]byte(tc.plan), []byte(tc.archive), tc.archive != "", bookCtx, mergedBook(tc.unit), "2026-10-07")
		if err == nil || !strings.Contains(err.Error(), tc.err) {
			t.Errorf("%s: %v, want %q", tc.name, err, tc.err)
		}
	}
	for _, tc := range []struct {
		name string
		u    bookUnit
		date string
	}{
		{"short SHA", bookUnit{"A", "abc1234", "r.md"}, "2026-10-07"},
		{"multi-line report", bookUnit{"A", mergedBook("A")[0].Sha, "r.md\n## X"}, "2026-10-07"},
		{"bad date", mergedBook("A")[0], "7 Oct"},
	} {
		if _, _, err := renderBookkeeping([]byte(bookPlan), nil, false, bookCtx, []bookUnit{tc.u}, tc.date); err == nil {
			t.Errorf("%s: rendered", tc.name)
		}
	}
}

// Fences: an indented closing fence closes; a backtick line whose info
// string holds a backtick is not a fence; a tilde fence hides headings.
func TestRenderBookkeepingFences(t *testing.T) {
	for name, entry := range map[string]string{
		"indented closing fence": "```text\n## inside\n   ```\n",
		"tilde fence":            "~~~\n### inside\n~~~\n",
		"backtick info string":   "```not ``a fence``\n",
	} {
		p := strings.Replace(bookPlan, "- **Done condition**: tests pass.\n", "- **Done condition**: tests pass.\n"+entry, 1)
		np, a, err := renderBookkeeping([]byte(p), nil, false, bookCtx, mergedBook("A"), "2026-10-07")
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		contains(t, string(a), entry, name)
		contains(t, string(np), "### B\n", name, "the next entry stays")
	}
}

// A CRLF plan gets CRLF in every line the bookkeeping adds.
func TestRenderBookkeepingCRLF(t *testing.T) {
	crlf := strings.ReplaceAll(bookPlan, "\n", "\r\n")
	p, _, err := renderBookkeeping([]byte(crlf), nil, false, bookCtx, mergedBook("A"), "2026-10-07")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(p), "\n") != strings.Count(string(p), "\r\n") {
		t.Errorf("mixed line endings: %q", p)
	}
}

// The archive check fails when the archive gained anything but the entries.
func TestAddedOnly(t *testing.T) {
	old := "# Archive: f\n\n## Work Units\n\n### Z\nold\n"
	e := []string{"### A\nbody\n"}
	if !addedOnly(old+"\n### A\nbody\n", old, "f", e) {
		t.Error("an appended entry")
	}
	for name, n := range map[string]string{
		"an extra line":   old + "\n### A\nbody\nextra\n",
		"an altered line": strings.Replace(old, "old", "new", 1) + "\n### A\nbody\n",
		"nothing added":   old,
		"entry truncated": old + "\n### A\nbo",
	} {
		if addedOnly(n, old, "f", e) {
			t.Errorf("%s: accepted", name)
		}
	}
}
