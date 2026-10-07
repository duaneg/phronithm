package orchestrator

import (
	"bytes"
	"encoding/json"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

// The stage schemas, in the key order of implement.js's objects (what the
// model sees).

// obj is a JSON object with key order kept; spreading is append.
type obj []kv

type kv struct {
	k string
	v any
}

func (o obj) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, e := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := plan.MarshalJS(e.k, "")
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		v, err := plan.MarshalJS(e.v, "")
		if err != nil {
			return nil, err
		}
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// with is {...o, k: v}: an existing key keeps its place, as in JS.
func (o obj) with(k string, v any) obj {
	out := append(obj{}, o...)
	for i := range out {
		if out[i].k == k {
			out[i].v = v
			return out
		}
	}
	return append(out, kv{k, v})
}

// spread is {...o, ...p}.
func (o obj) spread(p obj) obj {
	out := o
	for _, e := range p {
		out = out.with(e.k, e.v)
	}
	return out
}

func desc(o obj, d string) obj { return o.with("description", d) }

var (
	str     = obj{{"type", "string"}}
	boolean = obj{{"type", "boolean"}}
	integer = obj{{"type", "integer"}}
	strs    = obj{{"type", "array"}, {"items", str}}
)

func arr(items any) obj { return obj{{"type", "array"}, {"items", items}} }

// object is obj(properties, required = Object.keys(properties)).
func object(props obj, required ...string) obj {
	if required == nil {
		required = []string{}
		for _, e := range props {
			required = append(required, e.k)
		}
	}
	return obj{{"type", "object"}, {"properties", props}, {"required", required}}
}

func stage(result obj) json.RawMessage {
	b, err := plan.MarshalJS(result, "")
	if err != nil {
		panic(err)
	}
	return plan.StageSchema(b)
}

// Agents have composed SHAs from memory; the field says where the value
// comes from.
const shortSha = "commit SHA as `git rev-parse --short HEAD` prints it after the commit, copied from that output"

var (
	shaField     = desc(str, "The "+shortSha)
	commitFields = obj{{"branch", str}, {"sha", shaField}}
	pauseField   = obj{{"pauseRequested", obj{{"type", "boolean"}, {"description", "Whether the pause ref named in the prompt exists"}}}}

	finding = object(obj{
		{"severity", obj{{"type", "string"}, {"enum", []string{"Critical", "Significant", "Minor"}}}},
		{"statement", str},
		{"location", str},
	})
	gateRunSchema = object(obj{
		{"command", str},
		{"status", obj{{"type", "string"}, {"enum", []string{"green", "red", "not-run"}}}},
		{"summary", desc(str, "One line: counts, or what failed")},
	})
	gatesField = desc(arr(gateRunSchema), "The project's gates — the test suite and whatever else its own instructions name (linters, type checks) — as last run at HEAD")
	fixFields  = obj{
		{"applied", arr(object(obj{{"finding", str}, {"fix", str}}))},
		{"notApplied", arr(object(obj{{"finding", str}, {"reason", str}}))},
	}
	directEditsField = arr(object(obj{{"path", str}, {"description", str}}))
)

var (
	unitPlanSchema = stage(object(commitFields.spread(obj{
		{"path", str},
		{"bytes", obj{{"type", "integer"}, {"description", "Size of the committed plan file in bytes (wc -c)"}}},
		{"readings", arr(object(obj{
			{"clause", desc(str, "The clause of the unit entry or architecture document that admitted more than one reading, or named something that does not exist as named")},
			{"reading", desc(str, "The reading the plan takes")},
		}))},
	})))

	findingsSchema = stage(object(obj{
		{"findings", arr(finding)},
		{"systemic", desc(boolean, "True when the findings indicate a structural defect iteration will not resolve")},
		{"summary", str},
	}))

	fixSchema = stage(object(commitFields.spread(fixFields)))
	// A code fix re-runs the gates; the next review round reads them.
	fixCodeSchema = stage(object(commitFields.spread(fixFields).spread(obj{{"gates", gatesField}})))

	implementSchema = stage(object(commitFields.spread(obj{
		{"gateProbe", arr(object(obj{{"name", str}, {"pin", str}, {"observed", str}, {"drifted", boolean}}))},
		{"gates", gatesField},
		{"distress", plan.DistressSchema},
		{"deviations", desc(strs, "Departures from the unit entry's scope or publishes, one statement each")},
		{"writesOutsideScope", desc(strs, "Files changed that the unit entry's Scope line does not name")},
		{"deferrals", arr(object(obj{{"statement", str}, {"belongsTo", desc(str, "The unit the deferred work belongs to")}}))},
		{"discoveries", arr(object(obj{
			{"statement", str},
			{"affects", desc(str, "The unit whose scope, dependencies, or done condition this changes, or the architecture section it bears on")},
		}))},
		{"doneCondition", object(obj{
			{"evaluable", boolean},
			{"met", boolean},
			{"evidence", desc(str, "The evaluation, clause by clause where the done condition has clauses: what was checked and what it showed, citing tests and figures; not a narrative of the implementation. Rendered verbatim into the completion report")},
		})},
		{"standingObligations", arr(object(obj{{"statement", str}, {"status", str}}))},
		{"summary", desc(str, "One paragraph: what the unit delivered. Rendered verbatim into the completion report")},
	})))

	gateSchema = stage(object(obj{
		{"invalidations", arr(plan.InvalidationSchema)},
		{"notes", arr(object(obj{{"statement", str}, {"referent", desc(str, "A document section, unit, symbol, or file")}}))},
		{"amendments", desc(arr(object(obj{
			{"unit", str},
			{"section", str},
			{"statement", desc(str, "One clause: what the unit changed")},
		})), "A merged unit's own change to the architecture document that moves no decision; an empty array when the document did not move")},
	}))

	replanSchema = stage(object(obj{
		{"planSha", desc(str, "Plan SHA after the amendments were committed; the plan SHA given when nothing changed")},
		{"amendments", arr(object(obj{{"kind", obj{{"type", "string"}, {"enum", plan.AmendmentKinds}}}, {"unit", str}, {"statement", str}}))},
		{"overBudget", arr(object(obj{
			{"unit", str},
			{"disposition", obj{{"type", "string"}, {"enum", []string{"extracted", "flagged", "unrepaired"}}, {"description", "extracted: a preceding design unit now carries the excess; flagged: the excess is wall-clock, not context, and the unit carries the rationale; unrepaired: neither"}}},
			{"statement", str},
		}))},
		{"verdicts", arr(object(obj{
			{"id", str},
			{"verdict", obj{{"type", "string"}, {"enum", []string{"amended", "no-change", "document-text", "cannot"}}}},
			{"statement", desc(str, "What was amended, why the plan already answers it, what wording in the architecture document was corrected, or why the plan cannot absorb it")},
		}))},
		{"directEdits", directEditsField},
		{"summary", str},
	}))

	terminationSchema = stage(object(obj{
		{"planSha", desc(str, "Plan SHA after the appended units were committed; the plan SHA given when nothing changed")},
		{"appendedUnits", strs},
		{"directEdits", directEditsField},
	}))

	obligationsSchema = stage(object(obj{
		{"obligations", arr(object(obj{
			{"statement", str},
			{"disposition", obj{{"type", "string"}, {"enum", []string{"discharged", "carried", "undischarged"}}}},
			{"carrier", desc(str, "Where the obligation is now recorded outside this plan: file and row or section; empty unless carried")},
			{"evidence", str},
		}))},
	}))

	scribeSchema = stage(object(obj{{"path", str}, {"sha", shaField}}))
)
