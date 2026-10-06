package plan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

// fixture returns testdata/graph/<name>: the graph.test.js fixture bodies,
// byte for byte, shared with tools/graphparity.
func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "graph", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// jsonOf marshals v; the expected strings in these tests are written in JS
// key order, so equality also checks order.
func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustYAML(t *testing.T, src string) any {
	t.Helper()
	v, err := ParseYAML(src)
	if err != nil {
		t.Fatalf("ParseYAML(%q): %v", src, err)
	}
	return v
}

// js: parseYaml: mappings, sequences, flow sequences, scalars, comments
func TestParseYAMLStructures(t *testing.T) {
	got := jsonOf(t, mustYAML(t, fixture(t, "yaml-mappings.yaml")))
	want := `{"units":[{"id":"a","type":"foundation","complete":true,"dependencies":[],"scope":["src/a.py","src/b.py::Klass","x y"],"gates":[{"name":"rsvg-convert","kind":"environment","pin":"2.62.3"}]},{"id":"b","dependencies":["a"],"note":"it's # not a comment"}],"execution_order":["a","b"],"empty":null}`
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

// js: parseYaml: scalars stay strings except booleans and null
func TestParseYAMLScalars(t *testing.T) {
	got := jsonOf(t, mustYAML(t, fixture(t, "yaml-scalars.yaml")))
	if want := `{"a":"2026-09-11","b":"3","c":false,"d":null,"e":"true"}`; got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

// js: parseYaml: rejects what the subset excludes, with a line number
func TestParseYAMLRejects(t *testing.T) {
	res := []string{
		`line 1: unsupported YAML syntax: &`,
		`unsupported YAML syntax: \|`,
		`flow mappings`,
		`nested flow`,
		`line 2: duplicate key a`,
		`tab`,
		`expected a sequence item`,
		`unterminated`,
		`line 2: unexpected indent`,
		`empty item`,
	}
	for i, re := range res {
		name := "yaml-err-" + []string{"01", "02", "03", "04", "05", "06", "07", "08", "09", "10"}[i] + ".yaml"
		src := fixture(t, name)
		_, err := ParseYAML(src)
		if err == nil {
			t.Errorf("%s %q: no error", name, src)
			continue
		}
		if _, ok := err.(*YamlError); !ok {
			t.Errorf("%s: error is %T", name, err)
		}
		if !regexp.MustCompile(re).MatchString(err.Error()) {
			t.Errorf("%s %q: %q does not match /%s/", name, src, err, re)
		}
	}
}

// js: parseYaml: empty document is null
func TestParseYAMLEmpty(t *testing.T) {
	if v := mustYAML(t, fixture(t, "yaml-empty.yaml")); v != nil {
		t.Fatalf("got %v", v)
	}
}

func TestJSSpace(t *testing.T) {
	for _, r := range []rune{'\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2000, 0x2005, 0x200a, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff} {
		if !isJSSpace(r) {
			t.Errorf("%U should be JS whitespace", r)
		}
	}
	for _, r := range []rune{0x85, 0x200b, 'a', 0x1fff, 0x200b} {
		if isJSSpace(r) {
			t.Errorf("%U should not be JS whitespace", r)
		}
	}
	if got := jsTrim("\ufeff\u00a0 x \u3000"); got != "x" {
		t.Errorf("jsTrim: %q", got)
	}
	// A non-ASCII space before `#` starts a comment, and separates key from value.
	if got := jsonOf(t, mustYAML(t, "a:\u00a0b\u00a0# c")); got != `{"a":"b"}` {
		t.Errorf("nbsp: %s", got)
	}
	// JS `.` excludes U+2028, so a value containing it is not `key: value`.
	if _, err := ParseYAML("a: x\u2028y"); err == nil || err.Error() != "line 1: expected `key: value`, got `a: x\u2028y`" {
		t.Errorf("U+2028: %v", err)
	}
}

func TestJSStringAndJSON(t *testing.T) {
	m := mustYAML(t, "k: [a, ~, true]\nb: x")
	for _, c := range []struct {
		v          any
		str, jsonS string
	}{
		{nil, "null", "null"},
		{true, "true", "true"},
		{"w\"\\\n\t\b\f\r\x01\u2028<&>", "w\"\\\n\t\b\f\r\x01\u2028<&>", `"w\"\\\n\t\b\f\r\u0001` + "\u2028<&>\""},
		{[]any{"a", nil, true, []any{"b", "c"}}, "a,,true,b,c", `["a",null,true,["b","c"]]`},
		{m, "[object Object]", `{"k":["a",null,true],"b":"x"}`},
	} {
		if got := jsString(c.v); got != c.str {
			t.Errorf("jsString(%v) = %q, want %q", c.v, got, c.str)
		}
		if got := jsJSON(c.v); got != c.jsonS {
			t.Errorf("jsJSON(%v) = %s, want %s", c.v, got, c.jsonS)
		}
	}
}

func TestJSKeyOrder(t *testing.T) {
	got := jsKeyOrder([]string{"id", "10", "b", "2", "02", "0", "4294967295", "4294967294"})
	want := []string{"0", "2", "10", "4294967294", "id", "b", "02", "4294967295"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	if got := jsonOf(t, mustYAML(t, "id: x\n10: a\n2: b")); got != `{"2":"b","10":"a","id":"x"}` {
		t.Errorf("Map order: %s", got)
	}
}

// D1: JS reports a key naming an Object.prototype member as a duplicate.
func TestDivergenceD1PrototypeKeys(t *testing.T) {
	got := jsonOf(t, mustYAML(t, "constructor: 1\ntoString: x\n__proto__: y\nvalueOf: z"))
	if want := `{"constructor":"1","toString":"x","__proto__":"y","valueOf":"z"}`; got != want {
		t.Errorf("got %s", got)
	}
	if _, err := ParseYAML("constructor: 1\nconstructor: 2"); err == nil || err.Error() != "line 2: duplicate key constructor" {
		t.Errorf("real duplicate: %v", err)
	}
}

func TestFlowSeqTrailingBackslash(t *testing.T) {
	_, err := ParseYAML(`a: ["x\]`)
	if err == nil || err.Error() != "line 1: unterminated quoted string in flow sequence" {
		t.Errorf("got %v", err)
	}
}
