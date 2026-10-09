package lsfrt

import (
	"encoding/json"
	"errors"
	"time"
)

// Opts mirrors the orchestrator's agent options. Phase is recorded in
// telemetry only.
type Opts struct {
	Label     string          `json:"label"`
	Phase     string          `json:"phase,omitempty"`
	Model     string          `json:"model,omitempty"`
	Effort    string          `json:"effort,omitempty"`
	AgentType string          `json:"agentType,omitempty"`
	Isolation string          `json:"isolation,omitempty"`
	Dir       string          `json:"dir,omitempty"` // an existing directory under <stateDir>/merge/ to run in, with no clone; not in the call's hash
	Schema    json.RawMessage `json:"schema"`
}

// StopReason is a launch's recorded stop_reason.
type StopReason string

const (
	StopReturned     StopReason = "returned" // body returned, no stop pending
	StopSigint       StopReason = "signal-SIGINT"
	StopSigterm      StopReason = "signal-SIGTERM"
	StopCancelled    StopReason = "cancelled" // Run's parent context ended
	StopMaxCalls     StopReason = "max-calls"
	StopMaxWallClock StopReason = "max-wall-clock"
	StopMaxCost      StopReason = "max-cost"
	StopDryRun       StopReason = "dry-run"
	StopRuntimeError StopReason = "runtime-error" // the runtime cannot continue
	StopAuthFailed   StopReason = "auth-failed"   // a call failed authentication (401): refresh the login and relaunch
	StopCrashed      StopReason = "crashed"       // body returned a non-stop error or panicked
)

// StopError ends the run at a call boundary.
type StopError struct {
	Reason StopReason
	Detail string
}

func (e *StopError) Error() string {
	if e.Detail != "" {
		return "lsf-runtime: stopped: " + string(e.Reason) + ": " + e.Detail
	}
	return "lsf-runtime: stopped: " + string(e.Reason)
}

// IsStop reports whether err's chain holds a StopError, and its reason.
func IsStop(err error) (StopReason, bool) {
	var s *StopError
	if errors.As(err, &s) && s != nil {
		return s.Reason, true
	}
	return "", false
}

// CallResult flags. plan.SignalsFromCall maps them by these names.
const (
	FlagSkillNotLoaded         = "skill-not-loaded"
	FlagGuardKilledAfterResult = "guard-killed-after-result"
	FlagUncommittedUnchecked   = "uncommitted-unchecked"
)

// CallResult is one logical call's outcome. Value nil is null.
type CallResult struct {
	Value       json.RawMessage
	Class       string   // class of the last attempt
	Flags       []string // "skill-not-loaded", "guard-killed-after-result", "uncommitted-unchecked"
	Denials     []string // the last attempt's permission_denials, "tool: input" truncated
	Uncommitted []string // paths the call left uncommitted (see uncommitted.go); re-checked on a replay
	Violations  []string // a null after invalid returns: the last attempt's schema violations
	Branch      string   // a kept clone's branch in the main repo, lsf/<plan8>/<key>, journaled; "" otherwise
	Clone       string   // the clone the call ran in: CloneKept, CloneDiscarded, or "" (none: Dir, no repo, the main checkout, a seed)
	CloneHead   string   // a kept clone's HEAD when the branch was fetched back, journaled
	Attempts    int      // spawns for this call this launch; 0 when replayed
	CostUSD     float64
	Replayed    bool
}

// Summary is written to summary.json when a launch ends.
type Summary struct {
	StopReason StopReason      `json:"stop_reason"`
	Detail     string          `json:"detail,omitempty"`
	Started    time.Time       `json:"started"`
	Finished   time.Time       `json:"finished"`
	Calls      int             `json:"calls"` // spawns this launch
	CostUSD    float64         `json:"cost_usd"`
	Outcome    json.RawMessage `json:"outcome,omitempty"`
	Error      string          `json:"error,omitempty"`
}
