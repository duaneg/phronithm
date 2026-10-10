package plan

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
)

// Signal kinds. The stop rule (EvaluateWave) classifies by kind only; it
// never reads a signal's statement. A STOP kind is either computed by the
// orchestrator from a mechanical check, or adjudicated once per wave by the
// gate or the re-plan in a form the orchestrator can check for completeness.
var StopKinds = []string{
	"architecture-invalidated", // gate: section, decision, premise, evidence, amendment — all present
	"plan-defect-unabsorbed",   // re-plan: verdict `cannot`, or no verdict
	"done-condition-unevaluable",
	"cycle",
	"unit-over-budget", // re-plan: over budget, neither extracted nor flagged with rationale
	"gate-non-convergence",
	"merge-conflict-unresolvable",
	"merge-regression",
	"environment-gate-drift",
	"gate-not-run", // implement or code-fix stage: a gate reported not-run (JS: or no gate reported; Go: gate-omitted); declared gates (D27): the runtime could not run one, after a code stage or before a merge
	"no-ready-unit",
	"standing-obligation-undischarged",
	"agent-null",
	"precondition",
	"invalid-plan",
	"unserialised-overlap",
	// Go only (orchestrator D19, D21, D22, D23, D27, D32): driver-detected and code-stage stops.
	"uncommitted-at-return",  // driver: a stage that commits returned with changes no commit holds, or the check failed
	"skill-not-loaded",       // driver: the call's skill did not load after the runtime's retry
	"schema-violation",       // driver: a stage result outside its schema (an enum value, a missing field)
	"stage-fault",            // a code stage failed; the statement gives the reason
	"gate-omitted",           // implement or code-fix stage: no gate reported, or a gate the implement stage ran left out
	"branch-mismatch",        // driver: a worktree stage reported a branch other than the one the runtime gave it, or one that does not resolve
	"declared-gates-changed", // load: the plan's lsf-gates block differs from the one the launch pinned
	"retains-changed",        // load: a path left an incomplete unit's retains during the run
}

// RecordKinds reach the ledger and, where the re-plan can act on them, the
// re-plan stage. None stops the run.
var RecordKinds = []string{
	"reading",                     // plan stage: a clause that admits more than one reading, and the reading taken
	"deviation",                   // implement stage: departure from the unit entry, including writes outside Scope
	"deferral",                    // implement stage: in-scope work deliberately left to a named unit
	"discovery",                   // implement stage: something that changes another unit or a document
	"execution-distress",          // implement stage: breakdown's distress counts over threshold
	"finding",                     // review stages: a Significant or Minor finding, left as is
	"critical-fixed",              // fix stage: a Critical finding and its applied fix
	"architecture-note",           // gate: an observation short of an invalidation
	"claimed-disjoint-conflict",   // merge: conflict between units the plan called disjoint, resolved mechanically
	"amendment",                   // re-plan: a plan amendment
	"verdict",                     // re-plan: amended / no-change / document-text verdict on a re-plan item
	"architecture-amendment",      // re-plan: a wording-only correction to the architecture document, no decision moved; gate: a merged unit's own change to it that moves no decision
	"over-budget-repaired",        // re-plan: a preceding design unit extracted
	"over-budget-flagged",         // re-plan: excess is wall-clock, not context; flagged with rationale
	"standing-obligation-carried", // obligations: discharge lies outside this plan by the obligation's own terms, and the referent names where it is now recorded
	"direct-edit",                 // re-plan or termination: an edit outside the plan, made by a scribe stage without a gate; the referent is the path
	"document-oversize",           // plan or completion-report stage: the committed document is past twice its size target; the stage never trims to length
	// Go only (orchestrator D21, D27, D33): driver-detected and code-stage records.
	"residue-at-return",         // driver: a read-only stage returned with changes in its checkout
	"permission-denied",         // driver: the call had permission denials
	"sha-corrected",             // driver: a returned SHA was not its branch's tip and was replaced by it; also a SHA git could not check (the statement says "not checked")
	"guard-killed-after-result", // driver: the guard timeout killed the call after its result; the result stands
	"gate-misreported",          // implement or code-fix stage: a declared gate reported with a status other than the runtime's
	"retained-move-failed",      // cleanup: a merged unit's retains could not be moved to the store; its clones are kept
}

// GoOnlyKinds are the kinds the JavaScript does not have (orchestrator D19,
// D21 to D23, D27, D32, D33). tools/graphparity leaves them out of the constants comparison.
var GoOnlyKinds = []string{
	"uncommitted-at-return", "skill-not-loaded", "schema-violation", "stage-fault", "gate-omitted", "branch-mismatch",
	"residue-at-return", "permission-denied", "sha-corrected", "guard-killed-after-result",
	"declared-gates-changed", "gate-misreported", "retains-changed", "retained-move-failed",
}

var Sources = []string{"structural", "reported", "gate"}

var GateKinds = []string{"environment", "organisational", "release", "data"}

// UnitTypes are phronithm:breakdown's unit types, plus `meta` for units the
// termination stage appends from the plan's Meta-Tasks section.
var UnitTypes = []string{"foundation", "feature", "integration", "design", "refactor", "bug-fix", "polish", "meta"}

// AmendmentKinds are the re-plan amendment kinds, as breakdown's re-plan mode
// names them. `unit-removed` deletes an incomplete unit; `gate-added` and
// `gate-removed` change which gates a unit carries. `gate-repin` is the
// workflow's own: a decision re-pins an environment gate.
var AmendmentKinds = []string{"deferral", "scope-reassignment", "new-edge", "done-condition", "execution-order", "unit-added", "unit-removed", "gate-added", "gate-removed", "gate-repin"}

// IsStopKind reports whether kind stops the run.
func IsStopKind(kind string) bool { return slices.Contains(StopKinds, kind) }

// Schemas are compact JSON, byte-identical to JS JSON.stringify of the
// schema.js objects (key order matters: it is what the model sees).

// StageCommonProperties is the one field every stage returns beside `result`.
var StageCommonProperties = json.RawMessage(`{"commits":{"type":"array","items":{"type":"object","properties":{"branch":{"type":"string"},"sha":{"type":"string"}},"required":["branch","sha"]}}}`)

var StageCommonRequired = []string{"commits"}

// StageSchema builds the JSON Schema for one stage's return: `result` plus
// `commits`. It panics when result is not an object-type schema, a
// programmer error (the JS throws).
func StageSchema(result json.RawMessage) json.RawMessage {
	var probe struct {
		Type any `json:"type"`
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, result); err != nil || json.Unmarshal(result, &probe) != nil || probe.Type != "object" {
		panic("stageSchema: resultSchema must be a JSON Schema object type")
	}
	common := StageCommonProperties[1 : len(StageCommonProperties)-1]
	required, _ := json.Marshal(append([]string{"result"}, StageCommonRequired...))
	var b strings.Builder
	b.WriteString(`{"type":"object","properties":{"result":`)
	b.Write(compact.Bytes())
	b.WriteString(",")
	b.Write(common)
	b.WriteString(`},"required":`)
	b.Write(required)
	b.WriteString("}")
	return json.RawMessage(b.String())
}

// InvalidationFields are an architecture invalidation's fields as the wave
// gate returns it. The orchestrator stops only when every field is
// non-empty; anything less is a note.
var InvalidationFields = []string{"section", "decision", "premise", "evidence", "amendment"}

var InvalidationSchema = json.RawMessage(`{"type":"object","properties":{"section":{"type":"string","description":"Heading of the architecture section whose decision no longer stands"},"decision":{"type":"string","description":"What that section decides, quoted or closely paraphrased"},"premise":{"type":"string","description":"What the decision rests on"},"evidence":{"type":"string","description":"File and line, test, or completion-report line showing the premise false"},"amendment":{"type":"string","description":"What the section should now say"}},"required":["section","decision","premise","evidence","amendment"]}`)

// PlanSchema is the decomposition plan reduced to what the orchestrator
// reads, as ParsePlanGraph produces it from the plan's ```yaml lsf-plan
// block (docs/plan-graph.md). Gate `cleared` comes from the decision log.
var PlanSchema = json.RawMessage(`{"type":"object","properties":{"sha":{"type":"string"},"path":{"type":"string"},"units":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"type":{"type":"string","enum":["foundation","feature","integration","design","refactor","bug-fix","polish","meta"]},"complete":{"type":"boolean"},"dependencies":{"type":"array","items":{"type":"object","properties":{"unit":{"type":"string"}},"required":["unit"]}},"scope":{"type":"array","items":{"type":"string"},"description":"Files, modules, or directories the unit writes to; ` + "`file::symbol`" + ` when the unit owns one function or class within a file shared with other units"},"publishes":{"type":"array","items":{"type":"string"}},"gates":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"kind":{"type":"string","enum":["environment","organisational","release","data"]},"cleared":{"type":"boolean"},"pin":{"type":"string"}},"required":["name","kind","cleared"]}}},"required":["id","type","complete","dependencies","scope","gates"]}},"executionOrder":{"type":"array","items":{"type":"string"}}},"required":["sha","path","units","executionOrder"]}`)

// MergeResultSchema is the merge stage's result, per unit.
var MergeResultSchema = json.RawMessage(`{"type":"object","properties":{"integrationBranch":{"type":"string"},"merges":{"type":"array","items":{"type":"object","properties":{"unit":{"type":"string"},"status":{"type":"string","enum":["clean","resolved","unresolvable","regressed"],"description":"regressed: merged cleanly but the test suite failed; the merge was undone"},"files":{"type":"array","items":{"type":"string"}},"sha":{"type":"string"}},"required":["unit","status","files"]}}},"required":["integrationBranch","merges"]}`)

// DistressSchema holds the completion-report fields the distress rule reads
// (breakdown's execution-distress signals).
var DistressSchema = json.RawMessage(`{"type":"object","properties":{"contextNearExhaustion":{"type":"boolean"},"midImplementationDesignDecisions":{"type":"integer"},"deviations":{"type":"integer"},"deferrals":{"type":"integer"}},"required":["contextNearExhaustion","midImplementationDesignDecisions","deviations","deferrals"]}`)

// Signal is one ledger row. Signals are handled as *Signal throughout: the
// orchestrator relies on identity (a stop already in the ledger is stamped
// in place). Unit "" is absent; Wave nil is absent and wave 0 is the
// decision re-plan.
type Signal struct {
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	Stage       string `json:"stage"`
	Statement   string `json:"statement"`
	Referent    string `json:"referent"`
	RaisedBy    string `json:"raisedBy"`
	Unit        string `json:"unit,omitempty"`
	Wave        *int   `json:"wave,omitempty"`
	Disposition string `json:"disposition,omitempty"`
}

// NewSignal checks s, defaults RaisedBy to Stage and copies Wave. It panics on an empty
// Kind or an unknown Source, a programmer error (the JS throws).
func NewSignal(s Signal) *Signal {
	if s.Kind == "" {
		panic("signal: kind is required")
	}
	if !slices.Contains(Sources, s.Source) {
		panic("signal: unknown source " + s.Source)
	}
	if s.RaisedBy == "" {
		s.RaisedBy = s.Stage
	}
	if s.Wave != nil {
		w := *s.Wave // JS copies the number; a caller's counter must not move the row
		s.Wave = &w
	}
	return &s
}

// W returns a pointer to wave n.
func W(n int) *int { return &n }

// DecisionNeeded is one question an escalation puts to the requester.
type DecisionNeeded struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Affects  []string `json:"affects"`
}

// Escalation is the record a stop returns. PlanSha is the plan to relaunch
// from. After a stop at re-plan it is the re-plan's commit, which carries
// the absorbed amendments; PreReplanSha is then the plan the wave ran
// against.
type Escalation struct {
	Workflow       string           `json:"workflow"`
	Stage          string           `json:"stage"`
	Signals        []*Signal        `json:"signals"`
	DecisionNeeded []DecisionNeeded `json:"decision_needed"`
	PlanSha        string           `json:"plan_sha,omitempty"`
	PreReplanSha   string           `json:"pre_replan_sha,omitempty"`
	Wave           *int             `json:"wave,omitempty"`
}

// NewEscalation returns e with nil lists made empty and Wave copied.
func NewEscalation(e Escalation) Escalation {
	if e.Wave != nil {
		w := *e.Wave
		e.Wave = &w
	}
	for i := range e.DecisionNeeded {
		if e.DecisionNeeded[i].Affects == nil {
			e.DecisionNeeded[i].Affects = []string{}
		}
	}
	if e.Signals == nil {
		e.Signals = []*Signal{}
	}
	if e.DecisionNeeded == nil {
		e.DecisionNeeded = []DecisionNeeded{}
	}
	return e
}
