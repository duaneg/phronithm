package plan

import (
	"slices"
	"strings"
	"testing"
)

func retainsPlan(id, retains string) string {
	return "units:\n  - id: " + id + "\n    type: feature\n    retains: " + retains + "\nexecution_order: [" + id + "]\n"
}

func TestParseRetains(t *testing.T) {
	p, problems := ParsePlanGraph(retainsPlan("u", "[out/data, results.csv, .tmp, a/.git]"))
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	if got := p.Units[0].Retains; !slices.Equal(got, []string{"out/data", "results.csv", ".tmp", "a/.git"}) {
		t.Errorf("retains: %q", got)
	}
	b, _ := MarshalJS(p.Units[0], "")
	if !strings.Contains(string(b), `"retains":["out/data","results.csv",".tmp","a/.git"]`) {
		t.Errorf("JSON: %s", b)
	}
	// Absent and empty both leave the unit's JSON as it was.
	for _, r := range []string{"[]", "~"} {
		p, problems := ParsePlanGraph(retainsPlan("u", r))
		if len(problems) != 0 || p.Units[0].Retains != nil {
			t.Fatalf("%s: %v %q", r, problems, p.Units[0].Retains)
		}
		if b, _ := MarshalJS(p.Units[0], ""); strings.Contains(string(b), "retains") {
			t.Errorf("%s: JSON: %s", r, b)
		}
	}
}

func TestParseRetainsProblems(t *testing.T) {
	for _, c := range []struct{ name, retains, want string }{
		{"not a sequence", "out", "u: retains must be a sequence of paths"},
		{"empty entry", `[""]`, "u: retains must be a sequence of paths"},
		{"absolute", "[/data]", `u: retains "/data": must be a path relative to the clone, not absolute`},
		{"dotdot", "[../data]", `u: retains "../data": must be a clean relative path`},
		{"bare dotdot", "[..]", `u: retains "..": must be a clean relative path`},
		{"inner dotdot", "[a/../b]", `u: retains "a/../b": must be a clean relative path`},
		{"trailing dotdot", "[a/..]", `u: retains "a/..": must be a clean relative path`},
		{"dot", "[.]", `u: retains ".": must be a clean relative path`},
		{"dot component", "[./a]", `u: retains "./a": must be a clean relative path`},
		{"trailing slash", "[a/]", `u: retains "a/": must be a clean relative path`},
		{"double slash", "[a//b]", `u: retains "a//b": must be a clean relative path`},
		{"control", `["a\tb"]`, `u: retains "a\tb": must not contain control characters`},
		{"git", "[.git]", `u: retains ".git": must not be in .git`},
		{"under git", "[.git/hooks]", `u: retains ".git/hooks": must not be in .git`},
		{"duplicate", "[a, a]", `u: retains "a": is listed twice`},
		{"nested", "[a, a/b]", `u: retains "a/b": is inside "a"`},
		{"nesting", "[a/b, a]", `u: retains "a": contains "a/b"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, problems := ParsePlanGraph(retainsPlan("u", c.retains))
			if !slices.ContainsFunc(problems, func(p Problem) bool {
				return p.Kind == "invalid-plan" && p.Referent == "u" && strings.Contains(p.Statement, c.want)
			}) {
				t.Errorf("want %q in %q", c.want, statements(problems))
			}
		})
	}
	// A prefix that is not a parent is not nesting.
	if _, problems := ParsePlanGraph(retainsPlan("u", "[a, ab, a.b]")); len(problems) != 0 {
		t.Errorf("siblings: %v", problems)
	}
}

func TestSafeUnitDir(t *testing.T) {
	for id, want := range map[string]bool{
		"retained-artefacts-impl": true, "u_1.2": true, "a-": true, strings.Repeat("a", 100): true,
		"": false, ".": false, "..": false, ".hidden": false, "-x": false, "a/b": false, "a:b": false,
		"a#1": false, "a\x00b": false, "a\nb": false, "a\x7fb": false, strings.Repeat("a", 101): false,
	} {
		if got := SafeUnitDir(id); got != want {
			t.Errorf("SafeUnitDir(%q) = %v", id, got)
		}
	}
}

func TestCheckRetains(t *testing.T) {
	p := &Plan{Units: []*Unit{{ID: "a", Retains: []string{"x"}}, {ID: "b"}, {ID: "c:1", Retains: []string{"y"}}}}
	if got := statements(CheckRetains(p, &Declaration{Store: "/srv/store"})); len(got) != 1 || !strings.HasPrefix(got[0], "c:1: a unit with retains needs an id usable as a directory name") {
		t.Errorf("with store: %q", got)
	}
	for name, d := range map[string]*Declaration{"no block": nil, "no store": {Hosts: []string{"example.com"}}} {
		ps := CheckRetains(p, d)
		if got := statements(ps); len(got) != 3 || got[0] != "a: retains needs a `store` in the ```yaml lsf-gates block" || !strings.HasPrefix(got[1], "c:1: retains needs a `store`") {
			t.Errorf("%s: %q", name, got)
		}
		if ps[0].Kind != "invalid-plan" || ps[0].Referent != "a" {
			t.Errorf("%s: shape %+v", name, ps[0])
		}
	}
	if ps := CheckRetains(&Plan{Units: []*Unit{{ID: "c:1"}}}, nil); len(ps) != 0 {
		t.Errorf("no retains: %v", ps)
	}
}

func TestParseDeclarationStore(t *testing.T) {
	d, problems := ParseDeclaration(gatesDoc("store: /srv/lsf-store\n"))
	if len(problems) != 0 {
		t.Fatalf("store alone: %v", problems)
	}
	if d.Store != "/srv/lsf-store" || d.Gated() || d.Hosts != nil || d.Gates != nil {
		t.Errorf("store alone: %+v gated %v", d, d.Gated())
	}
	if got := d.Identity(); got != `{"store":"/srv/lsf-store"}` {
		t.Errorf("identity: %s", got)
	}
	d2, problems := ParseDeclaration(gatesDoc("store: /srv/other\n"))
	if len(problems) != 0 || d2.Identity() == d.Identity() {
		t.Errorf("a different store is a different declaration: %v %s", problems, d2.Identity())
	}
	d3, problems := ParseDeclaration(gatesDoc("hosts: [example.com]\nstore: /srv/s\ngates:\n  - name: a\n    command: x\n"))
	if len(problems) != 0 || d3.Identity() != `{"gates":[{"name":"a","command":"x"}],"hosts":["example.com"],"store":"/srv/s"}` {
		t.Errorf("combined: %v %s", problems, d3.Identity())
	}
	for _, c := range []struct{ name, body, want string }{
		{"relative", "store: srv/s\n", `store must be a clean absolute directory path other than /; got "srv/s"`},
		{"root", "store: /\n", `got "/"`},
		{"unclean", "store: /srv/../s\n", `got "/srv/../s"`},
		{"trailing slash", "store: /srv/s/\n", `got "/srv/s/"`},
		{"null", "store:\n", "got null"},
		{"not a string", "store: [/a]\n", `got ["/a"]`},
		{"setup and store", "setup: x\nstore: /srv/s\n", "setup needs a non-empty `gates` sequence"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d, problems := ParseDeclaration(gatesDoc(c.body))
			if d != nil || !slices.ContainsFunc(problems, func(p Problem) bool { return strings.Contains(p.Statement, c.want) }) {
				t.Errorf("want %q: %v %q", c.want, d, statements(problems))
			}
		})
	}
}
