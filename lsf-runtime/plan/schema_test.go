package plan

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

// mustPanic runs f and fails the test unless it panics.
func mustPanic(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: no panic", name)
		}
	}()
	f()
}

func decodeObject(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("not a JSON object: %v: %s", err, raw)
	}
	return m
}

// js: stageSchema wraps a result schema with the shared fields
func TestStageSchemaWraps(t *testing.T) {
	raw := StageSchema(json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]}`))
	want := `{"type":"object","properties":{"result":{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]},"commits":{"type":"array","items":{"type":"object","properties":{"branch":{"type":"string"},"sha":{"type":"string"}},"required":["branch","sha"]}}},"required":["result","commits"]}`
	if string(raw) != want {
		t.Fatalf("StageSchema:\n got %s\nwant %s", raw, want)
	}
	s := decodeObject(t, raw)
	props := s["properties"].(map[string]any)
	for _, r := range s["required"].([]any) {
		if _, ok := props[r.(string)]; !ok {
			t.Errorf("%v not declared", r)
		}
	}
	if len(props) != 2 {
		t.Errorf("commits is the only shared field: %v", props)
	}
}

// js: an invalidation requires all five fields
func TestInvalidationRequiresFive(t *testing.T) {
	s := decodeObject(t, InvalidationSchema)
	var req []string
	for _, r := range s["required"].([]any) {
		req = append(req, r.(string))
	}
	if !slices.Equal(req, InvalidationFields) {
		t.Errorf("required %v != fields %v", req, InvalidationFields)
	}
	if !slices.Equal(InvalidationFields, []string{"section", "decision", "premise", "evidence", "amendment"}) {
		t.Errorf("fields %v", InvalidationFields)
	}
}

// js: stageSchema rejects a non-object result schema
func TestStageSchemaRejects(t *testing.T) {
	mustPanic(t, "string type", func() { StageSchema(json.RawMessage(`{"type":"string"}`)) })
	mustPanic(t, "nil", func() { StageSchema(nil) })
	mustPanic(t, "array", func() { StageSchema(json.RawMessage(`[{"type":"object"}]`)) })
}

// js: signal requires a kind and a known source
func TestSignalRequiresKindAndSource(t *testing.T) {
	mustPanic(t, "no kind", func() { NewSignal(Signal{Source: "reported", Stage: "plan"}) })
	mustPanic(t, "bad source", func() { NewSignal(Signal{Kind: "reading", Source: "volunteered", Stage: "plan"}) })
	s := NewSignal(Signal{Kind: "reading", Source: "reported", Stage: "plan", Statement: "x"})
	if s.Referent != "" || s.RaisedBy != "plan" || s.Unit != "" {
		t.Errorf("defaults: %+v", s)
	}
	b, _ := json.Marshal(s)
	if string(b) != `{"kind":"reading","source":"reported","stage":"plan","statement":"x","referent":"","raisedBy":"plan"}` {
		t.Errorf("json: %s", b)
	}
}

// js: stop and record kinds are disjoint
func TestStopRecordDisjoint(t *testing.T) {
	for _, k := range RecordKinds {
		if IsStopKind(k) {
			t.Errorf("%s is both", k)
		}
	}
}

// js: escalation omits plan_sha and wave when unset
func TestEscalationOmitsUnset(t *testing.T) {
	keys := func(e Escalation) []string {
		var m map[string]any
		if err := json.Unmarshal([]byte(jsonOf(t, e)), &m); err != nil {
			t.Fatal(err)
		}
		var ks []string
		for k := range m {
			ks = append(ks, k)
		}
		slices.Sort(ks)
		return ks
	}
	e := NewEscalation(Escalation{Workflow: "lsf-implement", Stage: "critique", Signals: []*Signal{}})
	if got := keys(e); !reflect.DeepEqual(got, []string{"decision_needed", "signals", "stage", "workflow"}) {
		t.Errorf("keys %v", got)
	}
	b, _ := json.Marshal(e)
	if string(b) != `{"workflow":"lsf-implement","stage":"critique","signals":[],"decision_needed":[]}` {
		t.Errorf("json %s", b)
	}
	f := NewEscalation(Escalation{Workflow: "lsf-implement", PlanSha: "abc", Wave: W(2), Stage: "merge"})
	if f.PlanSha != "abc" || *f.Wave != 2 || slices.Contains(keys(f), "pre_replan_sha") {
		t.Errorf("f %+v", f)
	}
	if g := NewEscalation(Escalation{Workflow: "w", Stage: "replan", PlanSha: "b", PreReplanSha: "a"}); g.PreReplanSha != "a" {
		t.Errorf("g %+v", g)
	}
}

// js: gate-repin is an amendment kind
func TestGateRepinIsAmendment(t *testing.T) {
	if !slices.Contains(AmendmentKinds, "gate-repin") {
		t.Error("gate-repin missing")
	}
}

// js: unit removal and gate addition and removal are amendment kinds
func TestUnitAndGateAmendmentKinds(t *testing.T) {
	for _, k := range []string{"unit-removed", "gate-added", "gate-removed"} {
		if !slices.Contains(AmendmentKinds, k) {
			t.Error(k)
		}
	}
}

func TestSchemaLiteralsAreJSONAndMatchLists(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"StageCommonProperties": StageCommonProperties, "InvalidationSchema": InvalidationSchema,
		"PlanSchema": PlanSchema, "MergeResultSchema": MergeResultSchema, "DistressSchema": DistressSchema,
	} {
		if !json.Valid(raw) {
			t.Errorf("%s is not valid JSON", name)
		}
	}
	enum := func(v any) []string {
		var out []string
		for _, x := range v.(map[string]any)["enum"].([]any) {
			out = append(out, x.(string))
		}
		return out
	}
	unit := decodeObject(t, PlanSchema)["properties"].(map[string]any)["units"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if got := enum(unit["type"]); !slices.Equal(got, UnitTypes) {
		t.Errorf("PlanSchema unit types %v != %v", got, UnitTypes)
	}
	gate := unit["gates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if got := enum(gate["kind"]); !slices.Equal(got, GateKinds) {
		t.Errorf("PlanSchema gate kinds %v != %v", got, GateKinds)
	}
}

func TestSignalAndEscalationJSON(t *testing.T) {
	s := NewSignal(Signal{Kind: "verdict", Source: "reported", Stage: "decision-replan", Statement: "s", Unit: "u", Wave: W(0), Disposition: "recorded"})
	if got := jsonOf(t, s); got != `{"kind":"verdict","source":"reported","stage":"decision-replan","statement":"s","referent":"","raisedBy":"decision-replan","unit":"u","wave":0,"disposition":"recorded"}` {
		t.Errorf("signal %s", got)
	}
	e := NewEscalation(Escalation{Workflow: "w", Stage: "replan", PlanSha: "b", PreReplanSha: "a", Wave: W(0)})
	if got := jsonOf(t, e); got != `{"workflow":"w","stage":"replan","signals":[],"decision_needed":[],"plan_sha":"b","pre_replan_sha":"a","wave":0}` {
		t.Errorf("escalation %s", got)
	}
}
