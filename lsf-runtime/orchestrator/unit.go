package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// lensesByType are the review lenses per unit type. Design units produce a
// decision record, which gets critique-design rather than lens review.
var lensesByType = map[string][]string{
	"foundation":  {"general", "error-handling", "data-structures"},
	"feature":     {"general", "error-handling"},
	"integration": {"general", "error-handling"},
	"design":      {},
	"refactor":    {"general", "complexity"},
	"bug-fix":     {"general", "error-handling"},
	"polish":      {"general"},
	"meta":        {"general"},
}

// reviewer is one reviewer's return in a critique round.
type reviewer struct {
	stage string
	ret   *ret[findingsResult]
}

// gateOutcome is a critique loop's result. Significant and Minor findings
// accumulate in findings, each stamped with its stage and round, for the
// caller to record once per gate.
type gateOutcome struct {
	ok       bool
	reason   string
	rounds   int
	findings []plan.Finding
}

// critiqueLoop is the bounded critique loop shared by the plan gate and the
// code review gate. round(n) returns the round's reviewers; fix(n,
// criticals) the fix stage's return. It appends signals as it goes. A
// Critical the fix stage declines, a systemic verdict, or the round cap is a
// structural stop. onFix may return stop signals for a fix whose return
// cannot go to the next round (reason fix-stop).
func critiqueLoop(
	at func(stage string) plan.At,
	sigs *[]*plan.Signal,
	round func(n int) ([]reviewer, error),
	fix func(n int, criticals []critical) (*ret[fixResult], error),
	onFix func(fixed *ret[fixResult], n int) []*plan.Signal,
) (gateOutcome, error) {
	unitID := at("").Unit
	signal := func(stage, kind, source, statement, referent string) *plan.Signal {
		a := at(stage)
		return plan.NewSignal(plan.Signal{Kind: kind, Source: source, Stage: a.Stage, Unit: a.Unit, Wave: a.Wave, Statement: statement, Referent: referent})
	}
	findings := []plan.Finding{}
	for n := 1; n <= gateMaxRounds; n++ {
		rets, err := round(n)
		if err != nil {
			return gateOutcome{}, err
		}
		for _, r := range rets {
			if r.ret == nil {
				*sigs = append(*sigs, plan.NullReturnSignal(at(r.stage)))
				return gateOutcome{reason: "agent-null", rounds: n, findings: findings}, nil
			}
		}
		criticals := []critical{}
		for _, r := range rets {
			for _, f := range r.ret.Result.Findings {
				if f.Severity == "Critical" {
					criticals = append(criticals, critical{f.Severity, f.Statement, f.Location, r.stage})
				} else {
					f.Stage, f.Round = r.stage, n
					findings = append(findings, f)
				}
			}
		}
		for _, r := range rets {
			if r.ret.Result.Systemic {
				*sigs = append(*sigs, signal(r.stage, "gate-non-convergence", "gate",
					fmt.Sprintf("systemic problem reported in round %d: %s", n, r.ret.Result.Summary), unitID))
				return gateOutcome{reason: "systemic", rounds: n, findings: findings}, nil
			}
		}
		if len(criticals) == 0 {
			return gateOutcome{ok: true, rounds: n, findings: findings}, nil
		}
		if n == gateMaxRounds {
			*sigs = append(*sigs, signal(rets[0].stage, "gate-non-convergence", "structural",
				fmt.Sprintf("%d Critical finding(s) remain after round %d (hard cap)", len(criticals), n), unitID))
			return gateOutcome{reason: "hard-cap", rounds: n, findings: findings}, nil
		}
		fixed, err := fix(n, criticals)
		if err != nil {
			return gateOutcome{}, err
		}
		if fixed == nil {
			*sigs = append(*sigs, plan.NullReturnSignal(at("fix")))
			return gateOutcome{reason: "agent-null", rounds: n, findings: findings}, nil
		}
		for _, a := range fixed.Result.Applied {
			*sigs = append(*sigs, signal("fix", "critical-fixed", "structural", fmt.Sprintf("round %d: %s — fixed: %s", n, a.Finding, a.Fix), a.Finding))
		}
		if len(fixed.Result.NotApplied) > 0 {
			// A declined Critical stays Critical; the loop cannot pass, so
			// stop here rather than burn rounds to the cap.
			for _, a := range fixed.Result.NotApplied {
				*sigs = append(*sigs, signal("fix", "gate-non-convergence", "structural",
					fmt.Sprintf("round %d: Critical finding declined by the fix stage — %s: %s", n, a.Finding, a.Reason), a.Finding))
			}
			return gateOutcome{reason: "declined", rounds: n, findings: findings}, nil
		}
		if stops := onFix(fixed, n); len(stops) > 0 {
			*sigs = append(*sigs, stops...)
			return gateOutcome{reason: "fix-stop", rounds: n, findings: findings}, nil
		}
	}
	return gateOutcome{reason: "hard-cap", rounds: gateMaxRounds, findings: findings}, nil
}

// unrunGates are the gates a stage did not run: each not-run command, or a
// placeholder when it reported none. Empty when every gate has a result. A
// gate not run never reaches review: the reviewers would meet the same tool
// timeout.
func unrunGates(gates []gateRun) []string {
	if len(gates) == 0 {
		return []string{"(no gate reported)"}
	}
	out := []string{}
	for _, g := range gates {
		if g.Status == "not-run" {
			out = append(out, g.Command)
		}
	}
	return out
}

func gateNotRunSignal(at plan.At, missing []string) *plan.Signal {
	return plan.NewSignal(plan.Signal{
		Kind: "gate-not-run", Source: "structural", Stage: at.Stage, Unit: at.Unit, Wave: at.Wave,
		Statement: "gate(s) not run: " + strings.Join(missing, ", "), Referent: strings.Join(missing, ","),
	})
}

// unitOpts are a unit stage's options: label `<label>:<unit>`, phase Units,
// the default tier with role's set fields over it.
func (o *orch) unitOpts(u *plan.Unit, label, role string, opts lsfrt.Opts) lsfrt.Opts {
	opts.Label = label + ":" + u.ID
	opts.Phase = "Units"
	t := o.ctx.tierFor("default")
	if role != "" {
		t = t.over(o.ctx.tierFor(role))
	}
	opts.Model, opts.Effort = t.Model, t.Effort
	return opts
}

// runUnit runs the five stages for one unit. A stage that returns null or
// raises a stop-kind signal aborts the unit, and the result says where. The
// only error is a runtime stop.
func (o *orch) runUnit(ctx context.Context, p *plan.Plan, u *plan.Unit, wave int) (*unitResult, error) {
	c := o.ctx
	pr := planRef{p.Path, p.Sha}
	sigs := []*plan.Signal{}
	at := func(stage string) plan.At { return plan.At{Stage: stage, Unit: u.ID, Wave: plan.W(wave)} }
	res := &unitResult{unit: u.ID, typ: u.Type}
	abort := func(stage string) (*unitResult, error) {
		res.aborted = stage
		res.signals = sigs
		return res, nil
	}
	signal := func(stage, kind, statement, referent string) *plan.Signal {
		return plan.NewSignal(plan.Signal{Kind: kind, Source: "structural", Stage: stage, Unit: u.ID, Wave: plan.W(wave), Statement: statement, Referent: referent})
	}

	// 1. plan
	planned, _, err := call[unitPlanResult](ctx, o, unitPlanPrompt(c, pr, u), o.unitOpts(u, "plan", "planning", lsfrt.Opts{Isolation: "worktree", Schema: unitPlanSchema}))
	if err != nil {
		return nil, err
	}
	if planned == nil {
		sigs = append(sigs, plan.NullReturnSignal(at("plan")))
		return abort("plan")
	}
	pa := at("plan")
	pa.Path = planned.Result.Path
	sigs = append(sigs, plan.SignalsFromReadings(len(planned.Result.Readings), pa)...)
	pa.TargetKB = unitPlanTargetKB
	sigs = append(sigs, plan.SignalsFromDocumentSize(float64(planned.Result.Bytes), pa)...)
	branch := planned.Result.Branch

	// 2. critique gate over the plan
	planGate, err := critiqueLoop(at, &sigs,
		func(n int) ([]reviewer, error) {
			r, _, err := call[findingsResult](ctx, o, planCritiquePrompt(c, pr, u, branch, n),
				o.unitOpts(u, fmt.Sprintf("critique-plan:r%d", n), "critique", lsfrt.Opts{AgentType: "phronithm:critic", Schema: findingsSchema}))
			return []reviewer{{"critique-plan", r}}, err
		},
		func(n int, criticals []critical) (*ret[fixResult], error) {
			r, _, err := call[fixResult](ctx, o, fixPrompt(c, pr, u, branch, "plan", criticals, n, nil),
				o.unitOpts(u, fmt.Sprintf("fix-plan:r%d", n), "", lsfrt.Opts{Isolation: "worktree", Schema: fixSchema}))
			return r, err
		},
		func(fixed *ret[fixResult], n int) []*plan.Signal { branch = fixed.Result.Branch; return nil },
	)
	if err != nil {
		return nil, err
	}
	sigs = append(sigs, plan.SignalsFromFindings(planGate.findings, at("critique-plan"))...)
	if !planGate.ok {
		return abort("critique-plan")
	}

	// 3. implement
	impl, _, err := call[implementResult](ctx, o, implementPrompt(c, pr, u, branch), o.unitOpts(u, "implement", "", lsfrt.Opts{Isolation: "worktree", Schema: implementSchema}))
	if err != nil {
		return nil, err
	}
	if impl == nil {
		sigs = append(sigs, plan.NullReturnSignal(at("implement")))
		return abort("implement")
	}
	drifted := false
	for _, g := range impl.Result.GateProbe {
		if g.Drifted {
			drifted = true
			sigs = append(sigs, plan.NewSignal(plan.Signal{Kind: "environment-gate-drift", Source: "structural", Stage: "implement", Unit: u.ID, Wave: plan.W(wave),
				Statement: "gate " + g.Name + " pinned at " + g.Pin + ", observed " + g.Observed, Referent: g.Name}))
		}
	}
	if drifted {
		return abort("implement")
	}
	branch = impl.Result.Branch
	sigs = append(sigs, plan.SignalsFromImplement(impl.Result.ImplementResult, at("implement"))...)
	sigs = append(sigs, plan.DistressSignals(impl.Result.Distress.plan(), plan.At{Unit: u.ID, Wave: plan.W(wave)}, c.config)...)
	design := u.Type == "design"
	// A design unit's review reads no gates.
	if !design {
		if unrun := unrunGates(impl.Result.Gates); len(unrun) > 0 {
			sigs = append(sigs, gateNotRunSignal(at("implement"), unrun))
			return abort("implement")
		}
	}
	if !impl.Result.DoneCondition.Evaluable {
		sigs = append(sigs, signal("implement", "done-condition-unevaluable", "done condition cannot be evaluated as written: "+impl.Result.DoneCondition.Evidence, u.ID))
		return abort("implement")
	}

	// 4. review gate over the branch diff
	lenses, ok := lensesByType[u.Type]
	if !ok {
		lenses = lensesByType["feature"]
	}
	gr := gatesRun{by: "The implement stage", gates: impl.Result.Gates}
	fixedGates := false
	reviewGate, err := critiqueLoop(at, &sigs,
		func(n int) ([]reviewer, error) {
			type rv struct{ stage, prompt string }
			var rvs []rv
			if design {
				rvs = []rv{{"critique-design", designCritiquePrompt(c, pr, u, branch, n)}}
			} else {
				rvs = []rv{{"critique-code", codeCritiquePrompt(c, pr, u, branch, n, gr)}}
				if len(lenses) > 0 {
					rvs = append(rvs, rv{"lens-review", lensReviewPrompt(c, pr, u, branch, lenses, n, gr)})
				}
			}
			reqs := []agentReq{}
			for _, r := range rvs {
				reqs = append(reqs, agentReq{r.prompt, o.unitOpts(u, fmt.Sprintf("%s:r%d", r.stage, n), "critique", lsfrt.Opts{Isolation: "worktree", Schema: findingsSchema})})
			}
			res, err := o.parallelAgents(ctx, reqs)
			if err != nil {
				return nil, err
			}
			out := []reviewer{}
			for i, r := range rvs {
				out = append(out, reviewer{r.stage, decode[findingsResult](o.rt, res[i].label, res[i].cr.Value)})
			}
			return out, nil
		},
		func(n int, criticals []critical) (*ret[fixResult], error) {
			schema := fixCodeSchema
			if design {
				schema = fixSchema
			}
			r, _, err := call[fixResult](ctx, o, fixPrompt(c, pr, u, branch, "code", criticals, n, impl.Result.Gates),
				o.unitOpts(u, fmt.Sprintf("fix-code:r%d", n), "", lsfrt.Opts{Isolation: "worktree", Schema: schema}))
			return r, err
		},
		func(fixed *ret[fixResult], n int) []*plan.Signal {
			branch = fixed.Result.Branch
			if design {
				return nil
			}
			// A gate the implement stage ran and the fix did not report is
			// not run: a dropped gate would otherwise leave the reviewers a
			// shrunken result.
			reported := map[string]bool{}
			for _, g := range fixed.Result.Gates {
				reported[g.Command] = true
			}
			unrun := []string{}
			seen := map[string]bool{}
			add := func(cmd string) {
				if !seen[cmd] {
					seen[cmd] = true
					unrun = append(unrun, cmd)
				}
			}
			for _, cmd := range unrunGates(fixed.Result.Gates) {
				add(cmd)
			}
			for _, g := range impl.Result.Gates {
				if !reported[g.Command] {
					add(g.Command)
				}
			}
			if len(unrun) > 0 {
				return []*plan.Signal{gateNotRunSignal(at("fix"), unrun)}
			}
			gr = gatesRun{by: fmt.Sprintf("The round-%d fix stage", n), gates: fixed.Result.Gates}
			fixedGates = true
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	sigs = append(sigs, plan.SignalsFromFindings(reviewGate.findings, at("review"))...)
	if !reviewGate.ok {
		return abort("review")
	}

	// 5. completion report: rendered here, committed by a copy stage
	review := reviewOutcome{rounds: reviewGate.rounds, findings: reviewGate.findings, fixedGates: fixedGates}
	if fixedGates {
		review.gates = gr.gates
	}
	text := renderCompletionReport(c, u, wave, branch, &impl.Result, review)
	ca := at("completion-report")
	ca.Path, ca.TargetKB = completionPath(u), completionReportTargetKB
	sigs = append(sigs, plan.SignalsFromDocumentSize(float64(len(text)), ca)...)
	report, _, err := call[completionResult](ctx, o, completionScribePrompt(c, pr, u, branch, text),
		o.unitOpts(u, "completion-report", "mechanical", lsfrt.Opts{Isolation: "worktree", Schema: completionSchema}))
	if err != nil {
		return nil, err
	}
	if report == nil {
		sigs = append(sigs, plan.NullReturnSignal(at("completion-report")))
		return abort("completion-report")
	}
	// Without a branch the unit would be left out of the merge with no
	// signal and run again next wave (D17).
	if jsTrim(report.Result.Branch) == "" || !committed(report.Result.Sha) {
		sigs = append(sigs, plan.NoCommitSignal(plan.At{Stage: "completion-report", Wave: plan.W(wave), Path: completionPath(u)}))
		return abort("completion-report")
	}
	res.branch = report.Result.Branch
	res.completionPath = completionPath(u)
	res.signals = sigs
	return res, nil
}
