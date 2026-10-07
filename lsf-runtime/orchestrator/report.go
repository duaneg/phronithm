package orchestrator

import (
	"strings"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

// reviewOutcome is the review gate's result as the completion report reads
// it. gates is the last code fix's runs, set (fixedGates) only when a fix
// re-ran them: they are then the gates at the unit's final HEAD. With
// declared gates (declared, D27) gates is always the runtime's last run, and
// fixedGates says a fix's tree was gated.
type reviewOutcome struct {
	rounds     int
	findings   []plan.Finding
	gates      []gateRun
	fixedGates bool
	declared   bool
}

// renderCompletionReport renders the unit's completion report from the
// implement and review returns; the gate and the re-plan read it by path,
// and a copy stage commits it on the unit branch. Findings left unfixed are
// ledger rows, so the report carries their counts and the ledger's path,
// not the rows.
func renderCompletionReport(c *runCtx, u *plan.Unit, wave int, branch string, impl *implementResult, review reviewOutcome) string {
	items := func(xs []string) []string {
		if len(xs) == 0 {
			return []string{"None."}
		}
		out := []string{}
		for _, x := range xs {
			out = append(out, "- "+x)
		}
		return out
	}
	count := func(severity string) int {
		n := 0
		for _, f := range review.findings {
			if f.Severity == severity {
				n++
			}
		}
		return n
	}
	dc := impl.DoneCondition
	headGates := impl.Gates
	if review.fixedGates || review.declared {
		headGates = review.gates
	}

	lines := []string{"# " + u.ID + " — completion report", "", "Unit `" + u.ID + "` (" + u.Type + "), wave " + itoa(wave) + ", branch `" + branch + "`.", ""}
	if impl.Summary != "" {
		lines = append(lines, impl.Summary, "")
	}
	verdict := "**Not met**"
	if dc.Met {
		verdict = "**Met**"
	}
	if !dc.Evaluable {
		verdict += " (not evaluable as written)"
	}
	switch {
	case review.fixedGates && review.declared:
		verdict += ", as the implement stage evaluated it; the gates below are the runtime's at the last fix"
	case review.fixedGates:
		verdict += ", as the implement stage evaluated it; the gates below are the last fix's"
	}
	lines = append(lines, "## Done-condition verdict", "", strings.TrimRightFunc(verdict+". "+dc.Evidence, isJSSpace))
	if len(impl.GateProbe) > 0 {
		ps := []string{}
		for _, g := range impl.GateProbe {
			ps = append(ps, g.Name+" pinned "+g.Pin+", observed "+g.Observed)
		}
		lines = append(lines, "", "External gates re-verified: "+strings.Join(ps, "; ")+".")
	}
	if len(headGates) > 0 {
		lines = append(lines, "", "Gates at HEAD:", "", renderGates(headGates))
	}
	lines = append(lines, "", "## Deviations and writes outside Scope", "")
	lines = append(lines, items(impl.Deviations)...)
	if len(impl.WritesOutsideScope) > 0 {
		fs := []string{}
		for _, f := range impl.WritesOutsideScope {
			fs = append(fs, "`"+f+"`")
		}
		lines = append(lines, "", "Written outside the Scope line: "+strings.Join(fs, ", ")+".")
	}
	var deferrals, discoveries, obligations []string
	for _, d := range impl.Deferrals {
		deferrals = append(deferrals, d.Statement+" → `"+d.BelongsTo+"`")
	}
	for _, d := range impl.Discoveries {
		discoveries = append(discoveries, d.Statement+" — affects `"+d.Affects+"`")
	}
	for _, o := range impl.StandingObligations {
		obligations = append(obligations, o.Statement+" — "+o.Status)
	}
	lines = append(lines, "", "## Deferrals", "")
	lines = append(lines, items(deferrals)...)
	lines = append(lines, "", "## Discoveries", "")
	lines = append(lines, items(discoveries)...)
	lines = append(lines, "", "## Standing obligations", "")
	lines = append(lines, items(obligations)...)
	lines = append(lines, "",
		"Review: "+itoa(review.rounds)+" round(s); "+itoa(count("Significant"))+" Significant and "+itoa(count("Minor"))+" Minor finding(s) left unfixed are recorded in "+c.ledgerPath+", wave "+itoa(wave)+".",
		"")
	return strings.Join(lines, "\n")
}

// unitResult is one unit's outcome in a wave. A stage that returned null or
// raised a stop signal aborts the unit, and aborted names the stage.
type unitResult struct {
	unit, typ      string
	branch         string   // "" until the completion report is committed
	reportSha      string   // the completion report's commit, the merge's input
	worktrees      []string // this run's worktree labels, the report's last (D25)
	completionPath string
	signals        []*plan.Signal
	aborted        string
	gatedTree      string // the tree the runtime last gated, declared gates only (D27)
}

// mergedUnit is a unit merged this wave, with its merge commit.
type mergedUnit struct {
	unitResult
	mergeSha string
}

// critical is a Critical finding as the fix stage is given it.
type critical struct {
	Severity  string `json:"severity"`
	Statement string `json:"statement"`
	Location  string `json:"location"`
	Stage     string `json:"stage"`
}

// replanItem is an item the re-plan must absorb.
type replanItem struct {
	ID        string `json:"id"`
	Unit      string `json:"unit,omitempty"`
	Stage     string `json:"stage"`
	Kind      string `json:"kind"`
	Statement string `json:"statement"`
	Referent  string `json:"referent"`
}

// decisionItem is a requester decision as the decision re-plan is given it.
type decisionItem struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Decision string `json:"decision"`
}
