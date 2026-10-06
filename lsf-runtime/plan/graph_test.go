package plan

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func statements(ps []Problem) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Statement
	}
	return out
}

// js: extractBlock: exactly one tagged fence
func TestExtractBlock(t *testing.T) {
	md := fixture(t, "extract.md")
	if body, prob := ExtractBlock(md, "lsf-plan"); body != "units: []\nexecution_order: []\n" || prob != "" {
		t.Errorf("one block: %q %q", body, prob)
	}
	if _, prob := ExtractBlock(md, "lsf-decisions"); !strings.Contains(prob, "no ```yaml lsf-decisions block") {
		t.Errorf("none: %q", prob)
	}
	if _, prob := ExtractBlock(md+md, "lsf-plan"); !strings.Contains(prob, "2 ```yaml lsf-plan blocks") {
		t.Errorf("two: %q", prob)
	}
	if _, prob := ExtractBlock("", "lsf-plan"); !strings.Contains(prob, "no ") {
		t.Errorf("empty: %q", prob)
	}
}

// js: parsePlanGraph: maps the block onto the scheduling shape; gates start uncleared
func TestParsePlanGraphShape(t *testing.T) {
	plan, problems := ParsePlanGraph(fixture(t, "plan.yaml"))
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	want := `{"units":[{"id":"a","type":"foundation","complete":true,"dependencies":[],"scope":["src/a.py"],"publishes":["A"],"gates":[]},` +
		`{"id":"b","type":"feature","complete":false,"dependencies":[{"unit":"a"}],"scope":["src/b.py::f","tests/test_b.py"],"publishes":[],` +
		`"gates":[{"name":"legal","kind":"organisational","cleared":false},{"name":"node","kind":"environment","cleared":false,"pin":"22.1"}]}],` +
		`"executionOrder":["a","b"]}`
	if got := jsonOf(t, plan); got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

// js: parsePlanGraph: every shape defect is an invalid-plan problem with a referent
func TestParsePlanGraphDefects(t *testing.T) {
	plan, problems := ParsePlanGraph(fixture(t, "plan-defects.yaml"))
	if plan == nil {
		t.Fatal("a readable block with defects still yields a plan")
	}
	for _, p := range problems {
		if p.Kind != "invalid-plan" || p.Referent == "" {
			t.Errorf("problem %+v", p)
		}
	}
	ss := statements(problems)
	for _, re := range []string{
		`a: type must be one of`, `a: complete must be true or false`, `a: dependencies must be a sequence`,
		`a/g: gate kind must be one of`, `a: duplicate gate name g`, `gates\[2\]: gate missing name`,
		`units\[1\]: missing id`, `c: unknown key extra`, `execution_order names a more than once`, `unknown top-level key stray`,
	} {
		r := regexp.MustCompile(re)
		if !slices.ContainsFunc(ss, r.MatchString) {
			t.Errorf("/%s/ not in %q", re, ss)
		}
	}
}

// js: parsePlanGraph: an unreadable block yields no plan and one problem
func TestParsePlanGraphUnreadable(t *testing.T) {
	plan, problems := ParsePlanGraph(fixture(t, "plan-unreadable-1.yaml"))
	if plan != nil || len(problems) != 1 || !strings.HasPrefix(problems[0].Statement, "plan block: line 3") {
		t.Errorf("unreadable: %v %v", plan, problems)
	}
	for _, name := range []string{"plan-unreadable-2.yaml", "plan-unreadable-3.yaml"} {
		if plan, _ := ParsePlanGraph(fixture(t, name)); plan != nil {
			t.Errorf("%s: plan %v", name, plan)
		}
	}
}

// js: parseDecisions: entries with clears; null body is an empty log
func TestParseDecisions(t *testing.T) {
	decisions, problems := ParseDecisions(ptr(fixture(t, "decisions.yaml")))
	if len(problems) != 0 || len(decisions) != 2 {
		t.Fatalf("%v %v", decisions, problems)
	}
	if !slices.Equal(decisions[0].Clears, []string{"b/legal"}) {
		t.Errorf("clears %v", decisions[0].Clears)
	}
	if got := jsonOf(t, decisions[1]); got != `{"id":"D2","date":"2026-09-11","question":"q","decision":"d","clears":[],"resolves":["sig-3"]}` {
		t.Errorf("D2 %s", got)
	}
	for _, body := range []*string{nil, ptr(fixture(t, "decisions-empty.yaml"))} {
		if ds, ps := ParseDecisions(body); jsonOf(t, ds) != "[]" || jsonOf(t, ps) != "[]" {
			t.Errorf("empty log: %v %v", ds, ps)
		}
	}
}

// js: parseDecisions: missing fields and duplicate ids are problems
func TestParseDecisionsDefects(t *testing.T) {
	_, problems := ParseDecisions(ptr(fixture(t, "decisions-defects.yaml")))
	ss := statements(problems)
	for _, s := range []string{"D1: missing question", "D1: clears must be a sequence", "duplicate decision id D1"} {
		if !slices.ContainsFunc(ss, func(x string) bool { return strings.Contains(x, s) }) {
			t.Errorf("%q not in %q", s, ss)
		}
	}
}

// js: applyClearances: clears named gates; unknown or environment targets are problems
func TestApplyClearances(t *testing.T) {
	plan, _ := ParsePlanGraph(fixture(t, "plan.yaml"))
	problems := ApplyClearances(plan, []Decision{
		{ID: "D1", Clears: []string{"b/legal"}},
		{ID: "D2", Clears: []string{"b/node", "b/nope", "zzz/legal"}},
	})
	b := plan.Units[1]
	if got := jsonOf(t, b.Gates[0]); got != `{"name":"legal","kind":"organisational","cleared":true,"clearedBy":"D1"}` {
		t.Errorf("legal %s", got)
	}
	if b.Gates[1].Cleared {
		t.Error("environment gates are never cleared by decision")
	}
	var refs []string
	for _, p := range problems {
		refs = append(refs, p.Referent)
	}
	slices.Sort(refs)
	if !slices.Equal(refs, []string{"b/node", "b/nope", "zzz/legal"}) {
		t.Errorf("referents %v", refs)
	}
}

// js: loadGraph: plan and decisions join; no decisions document leaves gates uncleared
func TestLoadGraphJoins(t *testing.T) {
	with := LoadGraph(fixture(t, "withlog.plan.md"), ptr(fixture(t, "withlog.decisions.md")))
	if len(with.Problems) != 0 || !with.Plan.Units[1].Gates[0].Cleared || len(with.Decisions) != 1 {
		t.Errorf("with log: %+v", with)
	}
	without := LoadGraph(fixture(t, "nolog.plan.md"), nil)
	if len(without.Problems) != 0 || without.Plan.Units[1].Gates[0].Cleared {
		t.Errorf("without log: %+v", without)
	}
}

// js: loadGraph: a missing plan block is the only problem; a bad decisions block is a problem beside a good plan
func TestLoadGraphMissingBlocks(t *testing.T) {
	none := LoadGraph(fixture(t, "noblock.plan.md"), nil)
	if none.Plan != nil || len(none.Problems) != 1 || none.Problems[0].Referent != "lsf-plan" {
		t.Errorf("no block: %+v", none)
	}
	bad := LoadGraph(fixture(t, "badlog.plan.md"), ptr(fixture(t, "badlog.decisions.md")))
	if bad.Plan == nil || len(bad.Problems) != 1 || bad.Problems[0].Referent != "lsf-decisions" {
		t.Errorf("bad log: %+v", bad)
	}
}

func TestLoadGraphClearanceProblems(t *testing.T) {
	g := LoadGraph(fixture(t, "clearances.plan.md"), ptr(fixture(t, "clearances.decisions.md")))
	want := []string{
		"decision D2 clears environment gate b/node; environment gates are probed, not decided",
		"decision D2 clears unknown gate b/nope",
		"decision D2 clears unknown gate zzz/legal",
	}
	if got := statements(g.Problems); !slices.Equal(got, want) {
		t.Errorf("got %q", got)
	}
}

// The breakdown plan's block, pinned at d88f63b; the live document is
// compared against graph.js by tools/graphparity.
func TestLoadGraphBreakdownPinned(t *testing.T) {
	g := LoadGraph(fixture(t, "breakdown-d88f63b.md"), nil)
	if len(g.Problems) != 0 || g.Plan == nil || len(g.Plan.Units) != 36 {
		t.Fatalf("problems %v, units %d", g.Problems, len(g.Plan.Units))
	}
}

// D3: JS multiline ^/$ also break at a lone \r, U+2028 and U+2029; Go's
// break only at \n, so a closing fence after one of them is not found.
func TestDivergenceD3LineTerminators(t *testing.T) {
	for _, md := range []string{
		"```yaml lsf-plan\nunits: []\r```\n",
		"```yaml lsf-plan\nunits: [] ```\n",
		"```yaml lsf-plan\nunits: []\n``` x\n",
	} {
		if _, prob := ExtractBlock(md, PlanTag); prob != "no ```yaml lsf-plan block found" {
			t.Errorf("%q: %q", md, prob)
		}
	}
	if body, prob := ExtractBlock("```yaml lsf-plan\r\nunits: []\r\n```\r\n", PlanTag); body != "units: []\r\n" || prob != "" {
		t.Errorf("CRLF: %q %q", body, prob)
	}
}

// D4: a non-string type or kind is "" in Go, omitted from JSON; JS keeps
// the raw value. A missing one is omitted in both; a quoted "" is kept by
// JS and omitted by Go. The problem text is the same.
func TestDivergenceD4NonStringFields(t *testing.T) {
	plan, problems := ParsePlanGraph("units:\n  - id: [x, y]\n    type: [a]\n    gates:\n      - name: g\n      - name: h\n        kind: true\n  - id: z\nexecution_order: [x]")
	u := plan.Units[0]
	if u.ID != "x,y" || u.Type != "" || u.Gates[0].Kind != "" || u.Gates[1].Kind != "" || plan.Units[1].Type != "" {
		t.Errorf("unit %+v", u)
	}
	if got := jsonOf(t, plan.Units[1]); got != `{"id":"z","complete":false,"dependencies":[],"scope":[],"publishes":[],"gates":[]}` {
		t.Errorf("missing type: %s", got)
	}
	want := []string{
		"units[0]: missing id",
		`units[0]: type must be one of foundation, feature, integration, design, refactor, bug-fix, polish, meta; got ["a"]`,
		"units[0]/g: gate kind must be one of environment, organisational, release, data; got null",
		"units[0]/h: gate kind must be one of environment, organisational, release, data; got true",
		"z: type must be one of foundation, feature, integration, design, refactor, bug-fix, polish, meta; got null",
	}
	if got := statements(problems); !slices.Equal(got, want) {
		t.Errorf("got %q", got)
	}
}

// D7: encoding/json escapes <, >, &, U+2028 and U+2029; JSON.stringify
// does not. MarshalJS and jsJSON follow JS.
func TestDivergenceD7Encoding(t *testing.T) {
	p := Problem{Statement: "a\u2028b<&>\u2029 \\u2028 \\\u2028"}
	b, _ := json.Marshal(p)
	if !strings.Contains(string(b), `a\u2028b\u003c\u0026\u003e\u2029`) {
		t.Errorf("encoding/json: %s", b)
	}
	js, err := MarshalJS(p, "")
	if want := `{"kind":"","statement":"a` + "\u2028" + `b<&>` + "\u2029" + ` \\u2028 \\` + "\u2028" + `","referent":""}`; err != nil || string(js) != want {
		t.Errorf("MarshalJS:\n got %s\nwant %s", js, want)
	}
	if got := jsJSON("a\u2028b"); got != "\"a\u2028b\"" {
		t.Errorf("jsJSON: %q", got)
	}
	ind, _ := MarshalJS(map[string][]int{"a": {1}}, "  ")
	if string(ind) != "{\n  \"a\": [\n    1\n  ]\n}" {
		t.Errorf("indent: %q", ind)
	}
}
