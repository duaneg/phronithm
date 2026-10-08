package validate

import (
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
	if doc.ExecutionOrder == nil || *doc.ExecutionOrder != "\n1. `a`, `b`.\n" {
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

func TestParseEntriesDuplicateOnlyIsNotStub(t *testing.T) {
	e := &entry{Fields: map[string]string{}, Duplicates: []string{"Type"}}
	if e.stub() {
		t.Error("an entry with fields is not a stub")
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
