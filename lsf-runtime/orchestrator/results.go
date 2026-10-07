package orchestrator

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

// Stage results, decoded from the stage returns (D9). JSON tags follow the
// schemas; unknown fields are ignored, as in JS.

// jsInt is a schema `integer`: a finite integral number, 3.0 included, as
// the runtime validator accepts, within ±2^53 (past it float64 is not
// exact; the validator has no bound, see D9).
type jsInt int

func (n *jsInt) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return fmt.Errorf("null is not an integer")
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	if f != math.Trunc(f) || math.IsInf(f, 0) || math.Abs(f) > 1<<53 {
		return fmt.Errorf("%s is not an integer", b)
	}
	*n = jsInt(f)
	return nil
}

type copyResult struct {
	PauseRequested  bool   `json:"pauseRequested"`
	PlanBlock       string `json:"planBlock"`
	DecisionsBlock  string `json:"decisionsBlock"`
	HeadBranch      string `json:"headBranch"`
	ArchitectureSha string `json:"architectureSha"`
}

type reading struct {
	Clause  string `json:"clause"`
	Reading string `json:"reading"`
}

type unitPlanResult struct {
	Branch   string    `json:"branch"`
	Sha      string    `json:"sha"`
	Path     string    `json:"path"`
	Bytes    jsInt     `json:"bytes"`
	Readings []reading `json:"readings"`
}

type findingsResult struct {
	Findings []plan.Finding `json:"findings"`
	Systemic bool           `json:"systemic"`
	Summary  string         `json:"summary"`
}

// gateRun is one project gate as a stage ran it.
type gateRun struct {
	Command string `json:"command"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Tail    string `json:"-"` // the runtime's output tail for a gate not green (D27)
}

type applied struct {
	Finding string `json:"finding"`
	Fix     string `json:"fix"`
}

type notApplied struct {
	Finding string `json:"finding"`
	Reason  string `json:"reason"`
}

// fixResult is a plan or code fix; Gates only for a code fix.
type fixResult struct {
	Branch     string       `json:"branch"`
	Sha        string       `json:"sha"`
	Applied    []applied    `json:"applied"`
	NotApplied []notApplied `json:"notApplied"`
	Gates      []gateRun    `json:"gates"`
}

type gateProbe struct {
	Name     string `json:"name"`
	Pin      string `json:"pin"`
	Observed string `json:"observed"`
	Drifted  bool   `json:"drifted"`
}

type doneCondition struct {
	Evaluable bool   `json:"evaluable"`
	Met       bool   `json:"met"`
	Evidence  string `json:"evidence"`
}

type obligationStatus struct {
	Statement string `json:"statement"`
	Status    string `json:"status"`
}

type implementResult struct {
	plan.ImplementResult
	Branch              string             `json:"branch"`
	Sha                 string             `json:"sha"`
	GateProbe           []gateProbe        `json:"gateProbe"`
	Gates               []gateRun          `json:"gates"`
	Distress            distress           `json:"distress"`
	DoneCondition       doneCondition      `json:"doneCondition"`
	StandingObligations []obligationStatus `json:"standingObligations"`
	Summary             string             `json:"summary"`
}

// distress is plan.Distress with the counts read as schema integers.
type distress struct {
	ContextNearExhaustion            bool  `json:"contextNearExhaustion"`
	MidImplementationDesignDecisions jsInt `json:"midImplementationDesignDecisions"`
	Deviations                       jsInt `json:"deviations"`
	Deferrals                        jsInt `json:"deferrals"`
}

func (d distress) plan() plan.Distress {
	return plan.Distress{
		ContextNearExhaustion:            d.ContextNearExhaustion,
		MidImplementationDesignDecisions: int(d.MidImplementationDesignDecisions),
		Deviations:                       int(d.Deviations),
		Deferrals:                        int(d.Deferrals),
	}
}

type completionResult struct {
	Branch string `json:"branch"`
	Sha    string `json:"sha"`
	Path   string `json:"path"`
}

type mergeResult struct {
	plan.MergeResult
	PauseRequested  bool   `json:"pauseRequested"`
	PlanSha         string `json:"planSha"`
	ArchitectureSha string `json:"architectureSha"`
	// Signals are the conflict calls' driver events (D24), in the value so
	// a replay keeps them. Go only.
	Signals []*plan.Signal `json:"signals,omitempty"`
	// GateNotRun are the units regressed because the runtime could not run
	// a merge gate: their stop is gate-not-run (D27). Go only.
	GateNotRun []string `json:"gateNotRun,omitempty"`
}

type amendment struct {
	Kind      string `json:"kind"`
	Unit      string `json:"unit"`
	Statement string `json:"statement"`
}

type overBudget struct {
	Unit        string `json:"unit"`
	Disposition string `json:"disposition"`
	Statement   string `json:"statement"`
}

type verdict struct {
	ID        string `json:"id"`
	Verdict   string `json:"verdict"`
	Statement string `json:"statement"`
}

// directEdit is a small edit a planning stage returned as outside the plan.
type directEdit struct {
	Path        string `json:"path"`
	Description string `json:"description"`
}

type replanResult struct {
	PlanSha     string       `json:"planSha"`
	Amendments  []amendment  `json:"amendments"`
	OverBudget  []overBudget `json:"overBudget"`
	Verdicts    []verdict    `json:"verdicts"`
	DirectEdits []directEdit `json:"directEdits"`
	Summary     string       `json:"summary"`
}

type terminationResult struct {
	PlanSha       string       `json:"planSha"`
	AppendedUnits []string     `json:"appendedUnits"`
	DirectEdits   []directEdit `json:"directEdits"`
}

type obligation struct {
	Statement   string `json:"statement"`
	Disposition string `json:"disposition"`
	Carrier     string `json:"carrier"`
	Evidence    string `json:"evidence"`
}

type obligationsResult struct {
	Obligations []obligation `json:"obligations"`
}

// scribeResult is an append stage's return; PauseRequested only from the
// ledger scribe.
type scribeResult struct {
	Path           string `json:"path"`
	Sha            string `json:"sha"`
	PauseRequested bool   `json:"pauseRequested"`
}

// cleanupResult is the cleanup step's value (D25): worktrees removed, the
// ones kept with why, and the paths each removal deleted.
type cleanupResult struct {
	Removed jsInt          `json:"removed"`
	Kept    []string       `json:"kept"`
	Paths   []removedPaths `json:"paths,omitempty"`
}

// committed is an append stage that made its commit: an empty SHA is none.
func committed(sha string) bool { return jsTrim(sha) != "" }
