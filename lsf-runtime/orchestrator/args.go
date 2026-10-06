package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

const (
	workflowName    = "lsf-implement"
	gateMaxRounds   = 4 // critique gate hard cap
	defaultMaxWaves = 20
	// Size targets for the per-unit documents every later stage reads whole.
	// Targets, not ceilings: the plan stage writes its document in one call
	// and never measures or trims it; the completion report is rendered here
	// from the stage returns. A document past twice its target is recorded
	// (plan.SignalsFromDocumentSize).
	unitPlanTargetKB         = 12
	completionReportTargetKB = 8
)

// tier is a role's model and effort; "" is unset (D12).
type tier struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

// defaultTiers are the model and effort per role. Copy and append stages
// are mechanical: the content is rendered here or pinned to a SHA, the
// agent only moves it into the repo, and the parser checks the result on
// the next load.
func defaultTiers() map[string]*tier {
	return map[string]*tier{
		"critique":   {Model: "opus", Effort: "low"},
		"gate":       {Model: "opus", Effort: "medium"},
		"planning":   {Model: "opus", Effort: "high"},
		"default":    {Model: "opus", Effort: "low"},
		"mechanical": {Model: "haiku", Effort: "low"},
	}
}

// optStr is a string JS may hold as undefined: set distinguishes "" from
// unset, which `??` does and truthiness does not.
type optStr struct {
	v   string
	set bool
}

func some(s string) optStr { return optStr{v: s, set: true} }

// Equal is JS ===.
func (a optStr) Equal(b optStr) bool { return a.set == b.set && a.v == b.v }

// or is `?? def`.
func (a optStr) or(def string) string {
	if a.set {
		return a.v
	}
	return def
}

// decision is one requester decision passed in args.decisions (D11).
type decision struct {
	ID       string    `json:"id"`
	Question string    `json:"question"`
	Decision string    `json:"decision"`
	Clears   *[]string `json:"clears,omitempty"`
}

// args is the launch's arguments as the caller passes them (see run).
type args struct {
	Name              *string                    `json:"name"`
	PlanSha           *string                    `json:"planSha"`
	PlanPath          *string                    `json:"planPath"`
	ArchitectureDoc   *string                    `json:"architectureDoc"`
	ArchitectureSha   *string                    `json:"architectureSha"`
	RequestRecord     *string                    `json:"requestRecord"`
	IntegrationBranch *string                    `json:"integrationBranch"`
	DecisionLogPath   *string                    `json:"decisionLogPath"`
	DecisionLogSha    *string                    `json:"decisionLogSha"`
	Decisions         []decision                 `json:"decisions"`
	LedgerPath        *string                    `json:"ledgerPath"`
	ArchivePath       *string                    `json:"archivePath"`
	Config            json.RawMessage            `json:"config"`
	MaxWaves          *float64                   `json:"maxWaves"`
	MaxUnitsPerWave   *jsInt                     `json:"maxUnitsPerWave"`
	PauseRef          *string                    `json:"pauseRef"`
	Unscribed         json.RawMessage            `json:"unscribed"`
	Seed              map[string]json.RawMessage `json:"seed"`
	Tiers             map[string]*tier           `json:"tiers"`
}

// runCtx is the context every prompt reads.
type runCtx struct {
	name              string
	planSha           string
	planPath          string
	architectureDoc   string
	architectureSha   optStr
	requestRecord     string
	integrationBranch string
	decisionLogPath   string
	decisionLogSha    string
	ledgerPath        string
	archivePath       string
	config            plan.Config
	maxWaves          float64
	maxUnitsPerWave   *int // nil: unbounded
	pauseRef          string
	tiers             map[string]*tier
}

func requireArg(v *string, key string) (string, error) {
	if v == nil || *v == "" {
		return "", fmt.Errorf("lsf-implement: args.%s is required", key)
	}
	return *v, nil
}

func strOr(v *string, def string) string {
	if v != nil {
		return *v
	}
	return def
}

// parseArgs decodes the launch's args; nil or null is {}.
func parseArgs(raw json.RawMessage) (*args, error) {
	var a args
	if len(raw) == 0 || string(raw) == "null" {
		return &a, nil
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("lsf-implement: args: %w", err)
	}
	return &a, nil
}

// newRunCtx applies the defaults and checks the required args, in the JS
// order: name, planSha, architectureDoc, integrationBranch.
func newRunCtx(a *args) (*runCtx, error) {
	name, err := requireArg(a.Name, "name")
	if err != nil {
		return nil, err
	}
	c := &runCtx{name: name}
	if c.planSha, err = requireArg(a.PlanSha, "planSha"); err != nil {
		return nil, err
	}
	c.planPath = strOr(a.PlanPath, "docs/plans/"+name+"-breakdown.md")
	if c.architectureDoc, err = requireArg(a.ArchitectureDoc, "architectureDoc"); err != nil {
		return nil, err
	}
	if a.ArchitectureSha != nil {
		c.architectureSha = some(*a.ArchitectureSha)
	}
	c.requestRecord = strOr(a.RequestRecord, "docs/architecture/"+name+"-brief.md")
	if c.integrationBranch, err = requireArg(a.IntegrationBranch, "integrationBranch"); err != nil {
		return nil, err
	}
	c.decisionLogPath = strOr(a.DecisionLogPath, "docs/plans/"+name+"-decisions.md")
	c.decisionLogSha = strOr(a.DecisionLogSha, "")
	// One ledger file per launch, named by the plan SHA the launch starts
	// from; a decision re-plan moves the plan SHA on, the ledger stays put.
	c.ledgerPath = strOr(a.LedgerPath, "docs/plans/"+name+"-signals/"+c.planSha+".md")
	c.archivePath = strOr(a.ArchivePath, "docs/plans/"+name+"-archive.md")
	if c.config, err = plan.ConfigFrom(a.Config); err != nil {
		return nil, fmt.Errorf("lsf-implement: args.config: %w", err)
	}
	c.maxWaves = defaultMaxWaves
	if a.MaxWaves != nil {
		c.maxWaves = *a.MaxWaves
	}
	if a.MaxUnitsPerWave != nil {
		n := int(*a.MaxUnitsPerWave)
		c.maxUnitsPerWave = &n
	}
	c.pauseRef = strOr(a.PauseRef, "refs/lsf/"+name+"/pause")
	c.tiers = defaultTiers()
	for role, t := range a.Tiers {
		c.tiers[role] = t
	}
	return c, nil
}

// tierFor is `ctx.tiers[role] ?? ctx.tiers.default`.
func (c *runCtx) tierFor(role string) tier {
	if t := c.tiers[role]; t != nil {
		return *t
	}
	if t := c.tiers["default"]; t != nil {
		return *t
	}
	return tier{}
}

// over is JS `{...base, ...t}`: t's set fields win.
func (base tier) over(t tier) tier {
	if t.Model != "" {
		base.Model = t.Model
	}
	if t.Effort != "" {
		base.Effort = t.Effort
	}
	return base
}

// jsSlice is xs.slice(0, n) for an integer n.
func jsSlice[T any](xs []T, n *int) []T {
	if n == nil {
		return xs
	}
	k := *n
	if k < 0 {
		k += len(xs)
		if k < 0 {
			k = 0
		}
	}
	if k > len(xs) {
		k = len(xs)
	}
	return xs[:k]
}

// jsTrim is String.prototype.trim: JS whitespace and line terminators,
// not Go's (TrimSpace also strips U+0085 and keeps U+FEFF).
func jsTrim(s string) string { return strings.TrimFunc(s, isJSSpace) }

func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}
