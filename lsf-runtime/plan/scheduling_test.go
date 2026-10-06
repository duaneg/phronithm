package plan

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

type unitOpt func(*Unit)

func deps(ids ...string) unitOpt {
	return func(u *Unit) {
		for _, id := range ids {
			u.Dependencies = append(u.Dependencies, Dependency{Unit: id})
		}
	}
}

func scope(paths ...string) unitOpt { return func(u *Unit) { u.Scope = paths } }
func gates(gs ...Gate) unitOpt      { return func(u *Unit) { u.Gates = gs } }
func complete() unitOpt             { return func(u *Unit) { u.Complete = true } }
func typ(t string) unitOpt          { return func(u *Unit) { u.Type = t } }

func unit(id string, opts ...unitOpt) *Unit {
	u := &Unit{ID: id, Type: "feature", Dependencies: []Dependency{}, Scope: []string{}, Publishes: []string{}, Gates: []Gate{}}
	for _, o := range opts {
		o(u)
	}
	return u
}

// mkPlan builds a plan; order defaults to the units' order.
func mkPlan(units []*Unit, order ...string) *Plan {
	if order == nil {
		for _, u := range units {
			order = append(order, u.ID)
		}
	}
	return &Plan{Sha: "deadbeef", Path: "docs/plans/x.md", Units: units, ExecutionOrder: order}
}

// diamond: A foundation; B, C depend on A; D depends on B and C.
func diamond() *Plan {
	return mkPlan([]*Unit{
		unit("A", typ("foundation"), scope("src/core")),
		unit("B", deps("A"), scope("src/b")),
		unit("C", deps("A"), scope("src/c")),
		unit("D", deps("B", "C"), scope("src/d")),
	})
}

func ids(us []*Unit) []string {
	out := []string{}
	for _, u := range us {
		out = append(out, u.ID)
	}
	return out
}

func done(ids ...string) Completed {
	c := Completed{}
	for _, id := range ids {
		c[id] = true
	}
	return c
}

func eqStrings(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s: got %q, want %q", what, got, want)
	}
}

// js: findCycle returns null for a DAG and the cycle otherwise
func TestFindCycle(t *testing.T) {
	if c := FindCycle(diamond()); c != nil {
		t.Errorf("diamond: %v", c)
	}
	eqStrings(t, "cycle", FindCycle(mkPlan([]*Unit{unit("A", deps("B")), unit("B", deps("A"))})), []string{"A", "B", "A"})
}

// js: pathsOverlap is containment or equality, not prefix
func TestPathsOverlapContainment(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"src/core", "src/core/x.js", true},
		{"src/core/", "src/core", true},
		{"src/core", "src/corelib", false},
		{"src/a", "src/b", false},
	} {
		if PathsOverlap(c.a, c.b) != c.want {
			t.Errorf("%s ~ %s: want %v", c.a, c.b, c.want)
		}
	}
}

// js: pathsOverlap treats file::symbol as a narrowing of the file
func TestPathsOverlapSymbols(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"src/svg.py::emit_solid", "src/svg.py::emit_stairs", false},
		{"src/svg.py::emit_solid", "src/svg.py::emit_solid", true},
		{"src/svg.py", "src/svg.py::emit_solid", true},
		{"src/", "src/svg.py::emit_solid", true},
		{"src/other.py::f", "src/svg.py::f", false},
	} {
		if PathsOverlap(c.a, c.b) != c.want {
			t.Errorf("%s ~ %s: want %v", c.a, c.b, c.want)
		}
	}
}

// js: overlappingPairs reports serialisation via any dependency path
func TestOverlappingPairsSerialisation(t *testing.T) {
	p := mkPlan([]*Unit{
		unit("A", scope("src/x")),
		unit("B", deps("A"), scope("src/y")),
		unit("C", deps("B"), scope("src/x/file.js")),
		unit("D", scope("src/x")),
	})
	byKey := map[string]bool{}
	for _, pr := range OverlappingPairs(p, nil) {
		byKey[pr.Units[0]+"+"+pr.Units[1]] = pr.Serialised
	}
	if s, ok := byKey["A+C"]; !ok || !s {
		t.Error("A -> B -> C serialises A and C")
	}
	if s, ok := byKey["A+D"]; !ok || s {
		t.Error("A+D")
	}
	if s, ok := byKey["C+D"]; !ok || s {
		t.Error("C+D")
	}
}

// js: overlappingPairs and validatePlan ignore completed units
func TestOverlappingPairsIgnoreCompleted(t *testing.T) {
	p := mkPlan([]*Unit{unit("A", complete(), scope("src/x")), unit("B", scope("src/x")), unit("C", scope("src/x"))})
	if got := OverlappingPairs(p, nil); !reflect.DeepEqual(got, []Pair{{Units: [2]string{"B", "C"}}}) {
		t.Errorf("pairs %v", got)
	}
	if got := OverlappingPairs(p, done("C")); len(got) != 0 {
		t.Errorf("with C done: %v", got)
	}
	if got := ValidatePlan(p); len(got) != 1 {
		t.Errorf("validate %v", got)
	}
}

// js: validatePlan accepts the diamond
func TestValidatePlanDiamond(t *testing.T) {
	if got := ValidatePlan(diamond()); len(got) != 0 {
		t.Errorf("%v", got)
	}
}

// js: validatePlan reports unknown dependencies, order gaps, cycles, and unserialised overlap
func TestValidatePlanReports(t *testing.T) {
	p := mkPlan([]*Unit{
		unit("A", deps("Z"), scope("src/x")),
		unit("B", deps("A"), scope("src/y")),
		unit("C", deps("B")),
		unit("D", scope("src/x/a.js")),
	}, "A", "B", "C")
	p.Units[0].Dependencies = append(p.Units[0].Dependencies, Dependency{Unit: "C"})
	var kinds []string
	for _, pr := range ValidatePlan(p) {
		kinds = append(kinds, pr.Kind)
	}
	slices.Sort(kinds)
	eqStrings(t, "kinds", kinds, []string{"cycle", "invalid-plan", "invalid-plan", "unserialised-overlap"})
}

// js: validatePlan flags duplicate ids
func TestValidatePlanDuplicateIDs(t *testing.T) {
	p := mkPlan([]*Unit{unit("A"), unit("A")}, "A")
	if !slices.ContainsFunc(ValidatePlan(p), func(pr Problem) bool { return strings.Contains(pr.Statement, "duplicate") }) {
		t.Error("no duplicate problem")
	}
}

// js: readySet follows dependencies and honours plan order
func TestReadySetOrder(t *testing.T) {
	p := diamond()
	eqStrings(t, "none", ids(ReadySet(p, nil)), []string{"A"})
	eqStrings(t, "A", ids(ReadySet(p, done("A"))), []string{"B", "C"})
	eqStrings(t, "AB", ids(ReadySet(p, done("A", "B"))), []string{"C"})
	eqStrings(t, "ABC", ids(ReadySet(p, done("A", "B", "C"))), []string{"D"})
	eqStrings(t, "ABCD", ids(ReadySet(p, done("A", "B", "C", "D"))), []string{})
}

// js: readySet respects the plan complete flag
func TestReadySetCompleteFlag(t *testing.T) {
	p := diamond()
	p.Units[0].Complete = true
	eqStrings(t, "ready", ids(ReadySet(p, nil)), []string{"B", "C"})
}

// js: readySet excludes units with uncleared organisational or release gates, not environment ones
func TestReadySetGates(t *testing.T) {
	p := mkPlan([]*Unit{
		unit("A", gates(Gate{Name: "security sign-off", Kind: "organisational"})),
		unit("B", gates(Gate{Name: "node", Kind: "environment", Pin: ptr("26.8.1")})),
		unit("C", gates(Gate{Name: "v2 shipped", Kind: "release", Cleared: true})),
	})
	eqStrings(t, "ready", ids(ReadySet(p, nil)), []string{"B", "C"})
}

// js: blockedUnits distinguishes waiting from gated
func TestBlockedUnits(t *testing.T) {
	p := mkPlan([]*Unit{unit("A"), unit("B", deps("A")), unit("C", gates(Gate{Name: "legal", Kind: "organisational"}))})
	want := []Blocked{
		{Unit: "B", Reason: "waiting", WaitingOn: []string{"A"}, Gates: []string{}},
		{Unit: "C", Reason: "gated", WaitingOn: []string{}, Gates: []string{"legal"}},
	}
	if got := BlockedUnits(p, nil); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
}

// js: mergeOrder is plan execution order regardless of input order
func TestMergeOrder(t *testing.T) {
	p := diamond()
	p.ExecutionOrder = []string{"A", "C", "B", "D"}
	eqStrings(t, "merge", MergeOrder(p, []string{"B", "C"}), []string{"C", "B"})
}

// js: wavePlan partitions the diamond into three waves and stops at a gate
func TestWavePlan(t *testing.T) {
	if got := WavePlan(diamond(), nil); !reflect.DeepEqual(got, [][]string{{"A"}, {"B", "C"}, {"D"}}) {
		t.Errorf("diamond %v", got)
	}
	p := diamond()
	p.Units[3].Gates = append(p.Units[3].Gates, Gate{Name: "release 1.0", Kind: "release"})
	if got := WavePlan(p, nil); !reflect.DeepEqual(got, [][]string{{"A"}, {"B", "C"}}) {
		t.Errorf("gated %v", got)
	}
}

func TestWavePlanLeavesDoneAlone(t *testing.T) {
	d := Completed{"A": true, "B": false}
	if got := WavePlan(diamond(), d); !reflect.DeepEqual(got, [][]string{{"B", "C"}, {"D"}}) {
		t.Errorf("waves %v", got)
	}
	if !reflect.DeepEqual(d, Completed{"A": true, "B": false}) {
		t.Errorf("done mutated: %v", d)
	}
}

// D2: an unknown dependency is incomplete (the JS throws), and an id absent
// from the execution order sorts last (the JS comparator yields NaN).
func TestDivergenceD2(t *testing.T) {
	p := mkPlan([]*Unit{unit("A"), unit("B", deps("Z")), unit("C")}, "C", "A")
	eqStrings(t, "ready", ids(ReadySet(p, nil)), []string{"C", "A"})
	if got := BlockedUnits(p, nil); !reflect.DeepEqual(got, []Blocked{{Unit: "B", Reason: "waiting", WaitingOn: []string{"Z"}, Gates: []string{}}}) {
		t.Errorf("blocked %+v", got)
	}
	eqStrings(t, "merge", MergeOrder(p, []string{"X", "A", "Y", "C"}), []string{"C", "A", "X", "Y"})
}

func TestSchedulingDuplicatesAndSelfDependency(t *testing.T) {
	p := mkPlan([]*Unit{unit("A"), unit("B")}, "B", "A", "B")
	eqStrings(t, "last position wins", MergeOrder(p, []string{"A", "B"}), []string{"A", "B"})
	if got := MergeOrder(p, nil); got == nil {
		t.Error("MergeOrder(nil) is nil, not empty")
	}
	self := mkPlan([]*Unit{unit("A", deps("A"))})
	eqStrings(t, "self cycle", FindCycle(self), []string{"A", "A"})
	if got := ValidatePlan(self); len(got) != 1 || got[0].Statement != "dependency cycle A -> A" {
		t.Errorf("validate %v", got)
	}
	if got := BlockedUnits(self, nil); len(got) != 1 || !slices.Equal(got[0].WaitingOn, []string{"A"}) {
		t.Errorf("blocked %+v", got)
	}
	// A ready unit absent from the execution order sorts last (D2).
	eqStrings(t, "absent last", ids(ReadySet(mkPlan([]*Unit{unit("X"), unit("A")}, "A"), nil)), []string{"A", "X"})
}
