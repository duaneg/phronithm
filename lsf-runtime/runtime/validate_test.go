package lsfrt

import (
	"strings"
	"testing"
)

func TestValidateOutput(t *testing.T) {
	const schema = `{"type":"object","required":["status","items"],"properties":{
		"status":{"type":"string","enum":["met","unmet"]},
		"n":{"type":"integer"},
		"x":{"type":"number"},
		"ok":{"type":"boolean"},
		"items":{"type":"array","items":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}}}`
	for _, tc := range []struct {
		value string
		want  []string // substrings, one per expected error; nil means valid
	}{
		{`{"status":"met","items":[]}`, nil},
		{`{"status":"met","items":[{"id":"a"}],"n":3,"x":1.5,"ok":true,"extra":1}`, nil},
		{`{"status":"met","items":[],"n":3.0}`, nil}, // JavaScript's Number.isInteger(3.0)
		{`{"status":"done","items":[]}`, []string{`$.status: "done" not in enum`}},
		{`{"items":[]}`, []string{"$.status: required"}},
		{`{"status":"met","items":[{}]}`, []string{"$.items[0].id: required"}},
		{`{"status":"met","items":[{"id":7}]}`, []string{"$.items[0].id: expected string"}},
		{`{"status":"met","items":[],"n":2.5}`, []string{"$.n: expected integer"}},
		{`{"status":"met","items":[],"ok":"yes"}`, []string{"$.ok: expected boolean"}},
		{`{"status":"met","items":{}}`, []string{"$.items: expected array"}},
		{`[]`, []string{"$: expected object"}},
		{`null`, []string{"$: expected object"}},
		{`{"status":1,"items":[]}`, []string{"$.status: expected string"}},
	} {
		got := validateOutput([]byte(schema), []byte(tc.value))
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %q, want %d error(s) %q", tc.value, got, len(tc.want), tc.want)
			continue
		}
		for i, w := range tc.want {
			if !strings.Contains(got[i], w) {
				t.Errorf("%s: error %d is %q, want it to contain %q", tc.value, i, got[i], w)
			}
		}
	}
}

func TestValidateUnsupportedType(t *testing.T) {
	got := validateOutput([]byte(`{"type":"tuple"}`), []byte(`1`))
	if len(got) != 1 || !strings.Contains(got[0], "unsupported schema type") {
		t.Fatalf("got %q", got)
	}
}

func TestValidateEnumNumbersAndTypes(t *testing.T) {
	if got := validateOutput([]byte(`{"enum":[1,2]}`), []byte(`1.0`)); len(got) != 0 {
		t.Errorf("1.0 in enum [1,2]: %q", got)
	}
	if got := validateOutput([]byte(`{"enum":[1,2]}`), []byte(`"1"`)); len(got) != 1 {
		t.Errorf(`"1" in enum [1,2]: %q`, got)
	}
	if got := validateOutput([]byte(`{"type":["string","null"]}`), []byte(`"x"`)); len(got) != 1 || !strings.Contains(got[0], "unsupported schema type") {
		t.Errorf("type list: %q", got)
	}
	if got := abbrev(strings.Repeat("é", 50)); !strings.HasSuffix(got, "…") || !strings.Contains(got, "é") {
		t.Errorf("abbrev: %q", got)
	}
	if got := abbrev("<a&b>"); got != `"<a&b>"` {
		t.Errorf("abbrev escapes HTML: %q", got)
	}
}
