package orchestrator

import (
	"context"
	"fmt"
	"regexp"
	"slices"
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

// reviewer is one reviewer's return in a critique round, and its call's
// signals.
type reviewer struct {
	stage string
	ret   *ret[findingsResult]
	sigs  []*plan.Signal
}

// gateOutcome is a critique loop's result. Significant and Minor findings
// accumulate in findings, each stamped with its stage and round, for the
// caller to record once per gate. Reason call-stop is a stop among a
// round's call signals.
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
	fix func(n int, criticals []critical) (*ret[fixResult], []*plan.Signal, error),
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
		var callSigs []*plan.Signal
		for _, r := range rets {
			callSigs = append(callSigs, r.sigs...)
		}
		*sigs = append(*sigs, callSigs...)
		for _, r := range rets {
			if r.ret == nil {
				*sigs = append(*sigs, plan.NullReturnSignal(at(r.stage)))
				return gateOutcome{reason: "agent-null", rounds: n, findings: findings}, nil
			}
		}
		// A stopping reviewer's findings are not used.
		if hasStop(callSigs) {
			return gateOutcome{reason: "call-stop", rounds: n, findings: findings}, nil
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
		fixed, fixSigs, err := fix(n, criticals)
		if err != nil {
			return gateOutcome{}, err
		}
		*sigs = append(*sigs, fixSigs...)
		if fixed == nil {
			*sigs = append(*sigs, plan.NullReturnSignal(at("fix")))
			return gateOutcome{reason: "agent-null", rounds: n, findings: findings}, nil
		}
		for _, a := range fixed.Result.Applied {
			*sigs = append(*sigs, signal("fix", "critical-fixed", "structural", fmt.Sprintf("round %d: %s — fixed: %s", n, a.Finding, a.Fix), a.Finding))
		}
		if hasStop(fixSigs) {
			return gateOutcome{reason: "call-stop", rounds: n, findings: findings}, nil
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

// notRunGates are the gates a stage reported not run. A gate not run never
// reaches review: the reviewers would meet the same tool timeout.
func notRunGates(gates []gateRun) []string {
	out := []string{}
	for _, g := range gates {
		if g.Status == "not-run" {
			out = append(out, g.Command)
		}
	}
	return out
}

// omittedGates are the gates a stage left out of its return: a placeholder
// when it reported none, and each command in ran (the implement stage's
// gates, for a fix) it did not report (D22).
func omittedGates(gates, ran []gateRun) []string {
	out := []string{}
	if len(gates) == 0 {
		out = append(out, "(no gate reported)")
	}
	reported := map[string]bool{}
	for _, g := range gates {
		reported[g.Command] = true
	}
	for _, g := range ran {
		if !reported[g.Command] && !slices.Contains(out, g.Command) {
			out = append(out, g.Command)
		}
	}
	return out
}

// gateSignals are a stage's gate stops: gate-omitted, then gate-not-run.
func gateSignals(at plan.At, gates, ran []gateRun) []*plan.Signal {
	out := []*plan.Signal{}
	add := func(kind, what string, cmds []string) {
		if len(cmds) > 0 {
			out = append(out, plan.NewSignal(plan.Signal{
				Kind: kind, Source: "structural", Stage: at.Stage, Unit: at.Unit, Wave: at.Wave,
				Statement: "gate(s) " + what + ": " + strings.Join(cmds, ", "), Referent: strings.Join(cmds, ","),
			}))
		}
	}
	add("gate-omitted", "omitted", omittedGates(gates, ran))
	add("gate-not-run", "not run", notRunGates(gates))
	return out
}

// shaHex is a SHA as a stage reports it: at least 7 hex digits.
var shaHex = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// checkSha is the reported SHA of a stage that commits on branch, checked
// against the branch's tip. A SHA that is not a prefix of the tip is
// replaced by it, with a sha-corrected record. An unresolvable branch
// leaves the SHA as reported, with no signal: the stages after it fail on
// the branch itself. Another git error records that the SHA was not
// checked. With no repository there is nothing to check; with ctx done the
// error is the cancellation's stop.
func (o *orch) checkSha(ctx context.Context, at plan.At, branch, reported string) (string, *plan.Signal, error) {
	if o.git == nil || branch == "" || strings.HasPrefix(branch, "-") {
		return reported, nil, nil
	}
	tip, err := gitOps{o.git}.out(ctx, o.repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch+"^{commit}")
	switch {
	case err != nil && ctx.Err() != nil:
		return reported, nil, &lsfrt.StopError{Reason: lsfrt.StopCancelled}
	case err != nil && exitCode(err) == 1:
		return reported, nil, nil
	case err != nil:
		return reported, plan.ShaCorrectedSignal(at, branch, reported, "", err.Error()), nil
	}
	if r := strings.ToLower(jsTrim(reported)); shaHex.MatchString(r) && strings.HasPrefix(tip, r) {
		return reported, nil, nil
	}
	return tip, plan.ShaCorrectedSignal(at, branch, reported, tip, ""), nil
}

// correctSha applies checkSha to a result's SHA field.
func (o *orch) correctSha(ctx context.Context, at plan.At, branch string, sha *string, sigs *[]*plan.Signal) error {
	fixed, sig, err := o.checkSha(ctx, at, branch, *sha)
	if err != nil {
		return err
	}
	*sha = fixed
	if sig != nil {
		*sigs = append(*sigs, sig)
	}
	return nil
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

// fixCall is a plan or code fix's call, its SHA checked against its branch.
func (o *orch) fixCall(ctx context.Context, at plan.At, prompt string, opts lsfrt.Opts) (*ret[fixResult], []*plan.Signal, error) {
	r, sigs, err := call[fixResult](ctx, o, site{at, true}, prompt, opts)
	if err != nil || r == nil {
		return r, sigs, err
	}
	err = o.correctSha(ctx, at, r.Result.Branch, &r.Result.Sha, &sigs)
	return r, sigs, err
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
	planned, cs, err := call[unitPlanResult](ctx, o, site{at("plan"), true}, unitPlanPrompt(c, pr, u), o.unitOpts(u, "plan", "planning", lsfrt.Opts{Isolation: "worktree", Schema: unitPlanSchema}))
	if err != nil {
		return nil, err
	}
	sigs = append(sigs, cs...)
	if planned == nil {
		sigs = append(sigs, plan.NullReturnSignal(at("plan")))
		return abort("plan")
	}
	if err := o.correctSha(ctx, at("plan"), planned.Result.Branch, &planned.Result.Sha, &sigs); err != nil {
		return nil, err
	}
	pa := at("plan")
	pa.Path = planned.Result.Path
	sigs = append(sigs, plan.SignalsFromReadings(len(planned.Result.Readings), pa)...)
	pa.TargetKB = unitPlanTargetKB
	sigs = append(sigs, plan.SignalsFromDocumentSize(float64(planned.Result.Bytes), pa)...)
	if hasStop(cs) {
		return abort("plan")
	}
	branch := planned.Result.Branch

	// 2. critique gate over the plan
	planGate, err := critiqueLoop(at, &sigs,
		func(n int) ([]reviewer, error) {
			r, rs, err := call[findingsResult](ctx, o, site{at("critique-plan"), false}, planCritiquePrompt(c, pr, u, branch, n),
				o.unitOpts(u, fmt.Sprintf("critique-plan:r%d", n), "critique", lsfrt.Opts{AgentType: "phronithm:critic", Schema: findingsSchema}))
			return []reviewer{{"critique-plan", r, rs}}, err
		},
		func(n int, criticals []critical) (*ret[fixResult], []*plan.Signal, error) {
			return o.fixCall(ctx, at("fix"), fixPrompt(c, pr, u, branch, "plan", criticals, n, nil),
				o.unitOpts(u, fmt.Sprintf("fix-plan:r%d", n), "", lsfrt.Opts{Isolation: "worktree", Schema: fixSchema}))
		},
		func(fixed *ret[fixResult], n int) []*plan.Signal { branch = fixed.Result.Branch; return nil },
	)
	if err != nil {
		return nil, err
	}
	planFindings := plan.SignalsFromFindings(planGate.findings, at("critique-plan"))
	sigs = append(sigs, planFindings...)
	if !planGate.ok || hasStop(planFindings) {
		return abort("critique-plan")
	}

	// 3. implement
	impl, cs, err := call[implementResult](ctx, o, site{at("implement"), true}, implementPrompt(c, pr, u, branch), o.unitOpts(u, "implement", "", lsfrt.Opts{Isolation: "worktree", Schema: implementSchema}))
	if err != nil {
		return nil, err
	}
	sigs = append(sigs, cs...)
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
	if err := o.correctSha(ctx, at("implement"), branch, &impl.Result.Sha, &sigs); err != nil {
		return nil, err
	}
	sigs = append(sigs, plan.SignalsFromImplement(impl.Result.ImplementResult, at("implement"))...)
	sigs = append(sigs, plan.DistressSignals(impl.Result.Distress.plan(), plan.At{Unit: u.ID, Wave: plan.W(wave)}, c.config)...)
	if hasStop(cs) {
		return abort("implement")
	}
	design := u.Type == "design"
	// A design unit's review reads no gates.
	if !design {
		if gs := gateSignals(at("implement"), impl.Result.Gates, nil); len(gs) > 0 {
			sigs = append(sigs, gs...)
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
				v, rs := o.callSigs(site{at(r.stage), false}, res[i].cr, res[i].label)
				out = append(out, reviewer{r.stage, decode[findingsResult](o.rt, res[i].label, v), rs})
			}
			return out, nil
		},
		func(n int, criticals []critical) (*ret[fixResult], []*plan.Signal, error) {
			schema := fixCodeSchema
			if design {
				schema = fixSchema
			}
			return o.fixCall(ctx, at("fix"), fixPrompt(c, pr, u, branch, "code", criticals, n, impl.Result.Gates),
				o.unitOpts(u, fmt.Sprintf("fix-code:r%d", n), "", lsfrt.Opts{Isolation: "worktree", Schema: schema}))
		},
		func(fixed *ret[fixResult], n int) []*plan.Signal {
			branch = fixed.Result.Branch
			if design {
				return nil
			}
			// A gate the implement stage ran and the fix did not report is
			// omitted: a dropped gate would otherwise leave the reviewers a
			// shrunken result.
			if gs := gateSignals(at("fix"), fixed.Result.Gates, impl.Result.Gates); len(gs) > 0 {
				return gs
			}
			gr = gatesRun{by: fmt.Sprintf("The round-%d fix stage", n), gates: fixed.Result.Gates}
			fixedGates = true
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	reviewFindings := plan.SignalsFromFindings(reviewGate.findings, at("review"))
	sigs = append(sigs, reviewFindings...)
	if !reviewGate.ok || hasStop(reviewFindings) {
		return abort("review")
	}

	// 5. completion report: rendered here, committed by a code step
	review := reviewOutcome{rounds: reviewGate.rounds, findings: reviewGate.findings, fixedGates: fixedGates}
	if fixedGates {
		review.gates = gr.gates
	}
	text := renderCompletionReport(c, u, wave, branch, &impl.Result, review)
	ca := at("completion-report")
	ca.Path, ca.TargetKB = completionPath(u), completionReportTargetKB
	sigs = append(sigs, plan.SignalsFromDocumentSize(float64(len(text)), ca)...)
	in := completionInput{Unit: u.ID, Base: branch, Path: completionPath(u), Text: text, Subject: "docs(" + c.name + "): " + u.ID + " completion report"}
	if o.git != nil {
		// The tip is in the step's input, so a replay is a report on the
		// branch as it stands: a moved branch reruns the step.
		sha, err := gitOps{o.git}.out(ctx, o.repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch+"^{commit}")
		if err != nil && ctx.Err() != nil {
			return nil, &lsfrt.StopError{Reason: lsfrt.StopCancelled}
		}
		if err != nil || strings.HasPrefix(branch, "-") {
			sigs = append(sigs, stageFaultSignal(plan.At{Stage: "completion-report", Unit: u.ID, Wave: plan.W(wave), Path: completionPath(u)}, fmt.Sprintf("unit branch %q does not resolve", branch)))
			return abort("completion-report")
		}
		in.BaseSha = sha
	}
	report, fault, err := step[completionResult](ctx, o, "completion-report:"+u.ID, in, o.completionReport(in))
	if err != nil {
		return nil, err
	}
	if fault != "" {
		sigs = append(sigs, stageFaultSignal(plan.At{Stage: "completion-report", Unit: u.ID, Wave: plan.W(wave), Path: completionPath(u)}, fault))
		return abort("completion-report")
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
