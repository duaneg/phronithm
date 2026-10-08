package validate

import (
	"os"
	"reflect"
	"testing"
)

func TestParseDocumentSections(t *testing.T) {
	doc := parseDocument("# P\r\n\r\n## Work Units\r\n\r\n### a — complete at `x`\r\n\r\n### b\r\n- **Type**: feature\r\n\r\n## Execution Order\r\n\r\n1. `a`, `b`.\r\n\r\n## Other\r\n- `c`\r\n")
	if !doc.WorkUnits || len(doc.Entries) != 2 {
		t.Fatalf("entries: %+v", doc.Entries)
	}
	if a := doc.Entries[0]; a.ID != "a" || !a.stub() {
		t.Errorf("a: %+v", a)
	}
	if b := doc.Entries[1]; b.ID != "b" || b.stub() || b.Fields["Type"] != "feature" {
		t.Errorf("b: %+v", b)
	}
	if doc.ExecutionOrder == nil {
		t.Fatal("no execution order")
	}
	if *doc.ExecutionOrder != "\n1. `a`, `b`.\n" {
		t.Errorf("execution order: %q", *doc.ExecutionOrder)
	}
}

func TestParseDocumentAbsentSections(t *testing.T) {
	doc := parseDocument("# P\n\n### a\n- **Type**: x\n")
	if doc.WorkUnits || doc.Entries != nil || doc.ExecutionOrder != nil {
		t.Errorf("%+v", doc)
	}
}

func TestParseDocumentFences(t *testing.T) {
	doc := parseDocument("```yaml lsf-plan\n## Work Units\n```\n## Work Units\n### a\n- **Type**: x\n~~~\n### not-a-unit\n- **Scope**: y\n```\n~~~\n- **Scope**: `z`\n```mermaid\n## Execution Order\n```\n")
	if !doc.WorkUnits || len(doc.Entries) != 1 {
		t.Fatalf("entries: %+v", doc.Entries)
	}
	if got := doc.Entries[0].Fields; !reflect.DeepEqual(got, map[string]string{"Type": "x", "Scope": "`z`"}) {
		t.Errorf("fields: %v", got)
	}
	if doc.ExecutionOrder != nil {
		t.Errorf("heading inside a fence taken: %q", *doc.ExecutionOrder)
	}
}

func TestParseEntriesFieldValues(t *testing.T) {
	doc := parseDocument(`## Work Units
Preamble.
- **Type**: not in an entry
### u1 heading text
Prose before fields.
- **Type**:   feature
- **Scope**: ` + "`a`," + `
  ` + "`b`" + `

    ` + "`c`" + `
- **Depends on**: x

Trailing prose.
- not a field
- **Type**: again
#### sub
- **Effort**: low
### u2
`)
	if len(doc.Entries) != 2 {
		t.Fatalf("entries: %+v", doc.Entries)
	}
	u1 := doc.Entries[0]
	want := map[string]string{"Type": "feature", "Scope": "`a`, `b` `c`", "Depends on": "x"}
	if !reflect.DeepEqual(u1.Fields, want) || !reflect.DeepEqual(u1.Duplicates, []string{"Type"}) {
		t.Errorf("u1: %+v", u1)
	}
	if !doc.Entries[1].stub() {
		t.Errorf("u2: %+v", doc.Entries[1])
	}
}

func TestParseDocumentEmptySections(t *testing.T) {
	doc := parseDocument("## Work Units\n## Execution Order\n# End\n### x\n")
	if !doc.WorkUnits || len(doc.Entries) != 0 || doc.ExecutionOrder == nil || *doc.ExecutionOrder != "" {
		t.Errorf("%+v", doc)
	}
}

func TestFenceRules(t *testing.T) {
	for _, c := range []struct {
		name, doc string
		ids       []string
	}{
		{"longer fence holds a shorter one", "## Work Units\n````markdown\n```\n### hidden\n```\n````\n### a\n", []string{"a"}},
		{"info string does not close", "## Work Units\n```\n```mermaid\n### hidden\n```\n### a\n", []string{"a"}},
		{"other character does not close", "## Work Units\n~~~\n```\n### hidden\n~~~\n### a\n", []string{"a"}},
		{"backtick in info string is not a fence", "## Work Units\n```x` y\n### a\n", []string{"a"}},
		{"four spaces is not a fence", "## Work Units\n    ```\n### a\n", []string{"a"}},
		{"unclosed fence hides the rest", "## Work Units\n### a\n```\n### hidden\n", []string{"a"}},
	} {
		doc := parseDocument(c.doc)
		var ids []string
		for _, e := range doc.Entries {
			ids = append(ids, e.ID)
		}
		if !reflect.DeepEqual(ids, c.ids) {
			t.Errorf("%s: %q", c.name, ids)
		}
	}
}

func TestFenceAfterBlankEndsField(t *testing.T) {
	doc := parseDocument("## Work Units\n### a\n- **Scope**: `x`\n\n```\ncode\n```\n  indented prose\n")
	if got := doc.Entries[0].Fields["Scope"]; got != "`x`" {
		t.Errorf("Scope: %q", got)
	}
}

func TestCRLFFieldContinuation(t *testing.T) {
	doc := parseDocument("## Work Units\r\n### a\r\n- **Scope**:\r\n  `x`,\r\n\r\n  `y`\r\n")
	if got := doc.Entries[0].Fields["Scope"]; got != "`x`, `y`" {
		t.Errorf("Scope: %q", got)
	}
}

func TestHeadingLevel(t *testing.T) {
	for in, want := range map[string]int{"#": 0, "# ": 1, "##\tx": 2, "#x": 0, "####### x": 0, "###### x": 6, "": 0} {
		if got := headingLevel(in); got != want {
			t.Errorf("headingLevel(%q) = %d", in, got)
		}
	}
}

// Degenerate input is parsed without a panic.
func TestParseDocumentDegenerate(t *testing.T) {
	for _, in := range []string{"", "#", "##", "###", "### ", "## Work Units", "## Work Units\n### ", "## Work Units\n###\t\n- **A**:", "\r", "```", "## Work Units\n### a\n- **A**: x\n\n"} {
		parseDocument(in)
	}
}

func FuzzParseDocument(f *testing.F) {
	for _, s := range []string{"## Work Units\n### a\n- **Scope**: `x` (y; z)\n\n  w\n## Execution Order\n1. `a`\n", "```\n## Work Units\n```\n"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		doc := parseDocument(s)
		for _, e := range doc.Entries {
			for _, v := range e.Fields {
				splitTop(v, ';')
				stripParens(v)
				backticked(v)
			}
		}
	})
}

func TestParseLivePlan(t *testing.T) {
	b, err := os.ReadFile("../../docs/plans/lsf-improvements-breakdown.md")
	if os.IsNotExist(err) {
		t.Skip("plan absent")
	}
	if err != nil {
		t.Fatal(err)
	}
	doc := parseDocument(string(b))
	if len(doc.Entries) < 10 || doc.ExecutionOrder == nil {
		t.Fatalf("%d entries", len(doc.Entries))
	}
	for _, e := range doc.Entries {
		if !e.stub() && len(e.Fields) < 4 {
			t.Errorf("%s: %v", e.ID, e.Fields)
		}
	}
}

func TestSplitTop(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
		ok   bool
	}{
		{"a; b", []string{"a", " b"}, true},
		{"a (x; y); b", []string{"a (x; y)", " b"}, true},
		{"`a;b`; c", []string{"`a;b`", " c"}, true},
		{"`(`; c", []string{"`(`", " c"}, true},
		{"a (b", nil, false},
		{"`a; b", nil, false},
		{"a) b", nil, false},
		{"", []string{""}, true},
	} {
		got, ok := splitTop(c.in, ';')
		if ok != c.ok || (ok && !reflect.DeepEqual(got, c.want)) {
			t.Errorf("splitTop(%q) = %q, %v", c.in, got, ok)
		}
	}
}

func TestStripParens(t *testing.T) {
	for _, c := range []struct {
		in, want string
		ok       bool
	}{
		{"a (x, `y`) b", "a  b", true},
		{"a (x (y) z) b", "a  b", true},
		{"`f(x)` b", "`f(x)` b", true},
		{"a (`)`) b", "a  b", true},
		{"a (b", "", false},
		{"a) b", "", false},
		{"`a (b", "", false},
	} {
		got, ok := stripParens(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("stripParens(%q) = %q, %v", c.in, got, ok)
		}
	}
}

func TestBackticked(t *testing.T) {
	tokens, residue := backticked("`a`, `b::c` and d")
	if !reflect.DeepEqual(tokens, []string{"a", "b::c"}) || residue != " ,   and d" {
		t.Errorf("%q %q", tokens, residue)
	}
}
