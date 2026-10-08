package validate

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// agreement loads a document's block and returns its Agreement problems
// as "referent: statement" lines.
func agreement(t *testing.T, doc string) []string {
	t.Helper()
	g := plan.LoadGraph(doc, nil)
	if g.Plan == nil {
		t.Fatalf("block unreadable: %v", g.Problems)
	}
	var out []string
	for _, p := range Agreement(doc, g.Plan) {
		if p.Kind != DisagreementKind {
			t.Errorf("kind %q", p.Kind)
		}
		out = append(out, p.Referent+": "+p.Statement)
	}
	return out
}

// mutate replaces exactly one occurrence of each old string.
func mutate(t *testing.T, doc string, pairs ...string) string {
	t.Helper()
	for i := 0; i < len(pairs); i += 2 {
		if n := strings.Count(doc, pairs[i]); n != 1 {
			t.Fatalf("%q occurs %d times", pairs[i], n)
		}
		doc = strings.Replace(doc, pairs[i], pairs[i+1], 1)
	}
	return doc
}

func TestAgreementOK(t *testing.T) {
	if got := agreement(t, readFixture(t, "ok.plan.md")); len(got) != 0 {
		t.Errorf("ok.plan.md: %q", got)
	}
}

func TestAgreementVariants(t *testing.T) {
	doc := readFixture(t, "ok-variants.plan.md")
	for name, d := range map[string]string{"LF": doc, "CRLF": strings.ReplaceAll(doc, "\n", "\r\n")} {
		if got := agreement(t, d); len(got) != 0 {
			t.Errorf("%s: %q", name, got)
		}
	}
}

func TestAgreementDisagreements(t *testing.T) {
	ok := readFixture(t, "ok.plan.md")
	for _, c := range []struct {
		name  string
		pairs []string
		want  []string
	}{
		{"block unit without entry",
			[]string{"### docs-pass\n", "### docs-pas\n"},
			[]string{
				"docs-pass: entries: block unit docs-pass has no ### entry",
				"docs-pas: entries: entry docs-pas has no unit in the block",
			}},
		{"entry without block unit",
			[]string{"## Graph", "### extra\n- **Type**: polish\n\n## Graph"},
			[]string{"extra: entries: entry extra has no unit in the block"}},
		{"duplicate heading",
			[]string{"## Graph", "### parser\n\n## Graph"},
			[]string{"parser: entries: 2 ### entries for parser"}},
		{"stub for an incomplete unit",
			[]string{"### docs-pass\n- **Type**: design\n- **Scope**: `docs/design.md`\n- **Depends on**: none\n- **External gates**: none\n- **Done condition**: The design is written.\n", "### docs-pass — complete at `x`\n"},
			[]string{"docs-pass: entries: docs-pass has a one-line stub but is not complete: true in the block"}},
		{"missing field",
			[]string{"- **Depends on**: none\n", ""},
			[]string{"docs-pass: entries: docs-pass has no Depends on field"}},
		{"duplicate field",
			[]string{"- **Depends on**: none\n", "- **Depends on**: none\n- **Depends on**: parser\n"},
			[]string{"docs-pass: entries: docs-pass gives Depends on more than once"}},
		{"type",
			[]string{"- **Type**: design", "- **Type**: feature"},
			[]string{`docs-pass: entries: docs-pass Type is "feature" in its entry, "design" in the block`}},
		{"depends on missing",
			[]string{"- **Depends on**: parser, docs-pass (testing", "- **Depends on**: parser (testing"},
			[]string{"wire-up: entries: wire-up Depends on lists docs-pass in the block but not in its entry"}},
		{"depends on extra",
			[]string{"- **Depends on**: none", "- **Depends on**: base (logical, x); `parser`"},
			[]string{"docs-pass: entries: docs-pass Depends on lists base, parser in its entry but not in the block"}},
		{"depends on unbalanced",
			[]string{"- **Depends on**: base (logical, the lexer types)", "- **Depends on**: base (logical, the lexer types"},
			[]string{"parser: entries: parser Depends on: unbalanced parentheses or backticks"}},
		{"scope",
			[]string{"- **Scope**: `docs/design.md`", "- **Scope**: `docs/design.txt`"},
			[]string{
				"docs-pass: entries: docs-pass Scope lists docs/design.md in the block but not in its entry",
				"docs-pass: entries: docs-pass Scope lists docs/design.txt in its entry but not in the block",
			}},
		{"scope residue",
			[]string{"- **Scope**: `docs/design.md`", "- **Scope**: `docs/design.md`, docs/other.md"},
			[]string{`docs-pass: entries: docs-pass Scope: "docs/other.md" is outside backticks`}},
		{"scope symbol",
			[]string{"`src/shared.go::Lex`", "`src/shared.go::Lexer`"},
			[]string{
				"parser: entries: parser Scope lists src/shared.go::Lex in the block but not in its entry",
				"parser: entries: parser Scope lists src/shared.go::Lexer in its entry but not in the block",
			}},
		{"scope repeated",
			[]string{"- **Scope**: `docs/design.md`", "- **Scope**: `docs/design.md`, `docs/design.md/`"},
			[]string{"docs-pass: entries: docs-pass Scope repeats docs/design.md in its entry"}},
		{"gate name",
			[]string{"`verdict` (organisational)", "`approval` (organisational)"},
			[]string{
				"parser: entries: parser External gates lists verdict in the block but not in its entry",
				"parser: entries: parser External gates lists approval in its entry but not in the block",
			}},
		{"gate kind",
			[]string{"`verdict` (organisational)", "`verdict` (release)"},
			[]string{`parser/verdict: entries: parser gate verdict has kind "release" in its entry, "organisational" in the block`}},
		{"gate pin",
			[]string{"(environment, go 1.27.1,", "(environment, go 1.28.0,"},
			[]string{`parser/tool: entries: parser gate tool: the block's pin "go 1.27.1" has "1.27.1", which its entry does not`}},
		{"gate pin absent from entry",
			[]string{"`corpus` (data, `corpus` @ `abc123`)", "`corpus` (data)"},
			[]string{`wire-up/corpus: entries: wire-up gate corpus: the block's pin "corpus @ abc123" has "corpus", which its entry does not`}},
		{"gate unreadable",
			[]string{"- **External gates**: none", "- **External gates**: the toolchain"},
			[]string{`docs-pass: entries: docs-pass External gates: cannot read "the toolchain" as ` + "`name`" + ` (kind, pin)`}},
		{"execution order divergence",
			[]string{"2. `docs-pass`, `parser`.", "2. `parser`, `docs-pass`."},
			[]string{"execution-order: entries: Execution Order section mentions parser at position 1 among incomplete units, where the block has docs-pass"}},
		{"execution order omission",
			[]string{"3. `wire-up`.", "3. The command."},
			[]string{"execution-order: entries: wire-up is not mentioned in the Execution Order section"}},
		{"execution order rationale mention",
			[]string{"2. `docs-pass`, `parser`.", "2. `docs-pass`, which `wire-up` needs, then `parser`."},
			[]string{"execution-order: entries: Execution Order section mentions wire-up at position 2 among incomplete units, where the block has parser"}},
		{"fields in another syntax",
			[]string{"- **Type**: design\n- **Scope**: `docs/design.md`\n- **Depends on**: none\n- **External gates**: none\n- **Done condition**: The design is written.\n", "- **Type:** design\n"},
			[]string{"docs-pass: entries: docs-pass has text but no `- **Name**: value` field lines"}},
		{"gate segment unreadable, name kept out of the comparison",
			[]string{"`verdict` (organisational)", "`verdict` organisational"},
			[]string{"parser: entries: parser External gates: cannot read \"`verdict` organisational\" as `name` (kind, pin)"}},
		{"gate trailing separator",
			[]string{"`verdict` (organisational)", "`verdict` (organisational);"},
			nil},
		{"gate without kind",
			[]string{"`verdict` (organisational)", "`verdict`"},
			[]string{`parser/verdict: entries: parser gate verdict has kind "" in its entry, "organisational" in the block`}},
		{"gate repeated",
			[]string{"`verdict` (organisational)", "`verdict` (organisational); `verdict` (organisational)"},
			[]string{"parser: entries: parser External gates repeats verdict in its entry"}},
		{"scope unbalanced",
			[]string{"- **Scope**: `docs/design.md`", "- **Scope**: `docs/design.md` (the design"},
			[]string{"docs-pass: entries: docs-pass Scope: unbalanced parentheses or backticks"}},
		{"gates unbalanced",
			[]string{"`verdict` (organisational)", "`verdict (organisational)"},
			[]string{"parser: entries: parser External gates: unbalanced parentheses or backticks"}},
		{"no work units",
			[]string{"## Work Units", "## Units"},
			[]string{"work-units: entries: no ## Work Units section"}},
		{"no execution order",
			[]string{"## Execution Order", "## Order"},
			[]string{"execution-order: entries: no ## Execution Order section"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := agreement(t, mutate(t, ok, c.pairs...))
			if !slices.Equal(got, c.want) {
				t.Errorf("\n got %q\nwant %q", got, c.want)
			}
		})
	}
}

// The block's units come first in block order, then entries with no unit,
// then the execution order.
func TestAgreementOrdering(t *testing.T) {
	doc := mutate(t, readFixture(t, "ok.plan.md"),
		"- **Type**: integration", "- **Type**: feature",
		"- **Type**: design", "- **Type**: feature",
		"## Graph", "### zz\n\n## Graph",
		"3. `wire-up`.", "3. Then the rest.",
	)
	want := []string{
		`docs-pass: entries: docs-pass Type is "feature" in its entry, "design" in the block`,
		`wire-up: entries: wire-up Type is "feature" in its entry, "integration" in the block`,
		"zz: entries: entry zz has no unit in the block",
		"execution-order: entries: wire-up is not mentioned in the Execution Order section",
	}
	if got := agreement(t, doc); !slices.Equal(got, want) {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
}

func TestAgreementNilPlan(t *testing.T) {
	got := Agreement("", nil)
	if len(got) != 1 || got[0].Referent != "lsf-plan" {
		t.Errorf("%v", got)
	}
}

// The live plan is the command's first user: it must agree with its block.
func TestAgreementLivePlan(t *testing.T) {
	b, err := os.ReadFile("../../docs/plans/lsf-improvements-breakdown.md")
	if os.IsNotExist(err) {
		t.Skip("plan absent")
	}
	if err != nil {
		t.Fatal(err)
	}
	if got := agreement(t, string(b)); len(got) != 0 {
		t.Errorf("docs/plans/lsf-improvements-breakdown.md disagrees with its block:\n%s", strings.Join(got, "\n"))
	}
}
