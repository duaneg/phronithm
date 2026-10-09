package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
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
// cannot go to the next round (reason fix-stop); its error is a stop.
func critiqueLoop(
	at func(stage string) plan.At,
	sigs *[]*plan.Signal,
	round func(n int) ([]reviewer, error),
	fix func(n int, criticals []critical) (*ret[fixResult], []*plan.Signal, error),
	onFix func(fixed *ret[fixResult], n int) ([]*plan.Signal, error),
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
		stops, err := onFix(fixed, n)
		if err != nil {
			return gateOutcome{}, err
		}
		if len(stops) > 0 {
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
// replaced by it, with a sha-corrected record. Nothing reads these SHAs
// yet: the check keeps the ledger's SHAs git's. unresolved reports a
// branch that does not resolve (exit 1), leaving the SHA as reported; a
// branch that is not a plain branch name is left unchecked. Another git
// error records that the SHA was not checked. With no repository there is
// nothing to check; with ctx done the error is the cancellation's stop.
func (o *orch) checkSha(ctx context.Context, at plan.At, branch, reported string) (sha string, sig *plan.Signal, unresolved bool, err error) {
	return o.checkShaFrom(ctx, at, branch, reported, o.liveTip)
}

// branchTipRead is a branch's tip as read once: Unresolved for exit 1,
// Error for another git error.
type branchTipRead struct {
	Tip        string `json:"tip,omitempty"`
	Unresolved bool   `json:"unresolved,omitempty"`
	Error      string `json:"error,omitempty"`
}

// liveTip reads branch's tip now; err is only the cancellation's stop.
func (o *orch) liveTip(ctx context.Context, branch string) (branchTipRead, error) {
	tip, err := gitOps{o.git}.out(ctx, o.repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch+"^{commit}")
	switch {
	case err != nil && ctx.Err() != nil:
		return branchTipRead{}, &lsfrt.StopError{Reason: lsfrt.StopCancelled}
	case err != nil && exitCode(err) == 1:
		return branchTipRead{Unresolved: true}, nil
	case err != nil:
		return branchTipRead{Error: err.Error()}, nil
	}
	return branchTipRead{Tip: tip}, nil
}

// journaledTip reads a writer call's branch tip as a code step,
// branch-tip:<call label> (D23): a replay gets the tip the call returned
// to, so the check, its correction and its ledger rows are the same
// whether or not cleanup has since deleted the branch. The input holds the
// SHA the call reported, so a call that reran (and committed again) reads
// afresh. Only a resolved tip is journaled: an unresolved branch or a git
// error is read again next launch. The tip is kept for the completion
// report's fallback (tipAt).
func (o *orch) journaledTip(callLabel, reported string) func(context.Context, string) (branchTipRead, error) {
	return func(ctx context.Context, branch string) (branchTipRead, error) {
		var unjournaled branchTipRead
		raw, _, err := o.runStep(ctx, "branch-tip:"+callLabel, map[string]string{"branch": branch, "reported": reported}, func(ctx context.Context, _ string, _ bool) (json.RawMessage, error) {
			r, err := o.liveTip(ctx, branch)
			if err != nil {
				return nil, err
			}
			if r.Tip == "" {
				unjournaled = r
				return nil, errNotJournaled
			}
			return json.Marshal(r)
		})
		if unjournaled != (branchTipRead{}) {
			return unjournaled, nil
		}
		var se *stageErr
		if errors.As(err, &se) {
			return branchTipRead{Error: se.reason}, nil
		}
		if err != nil {
			return branchTipRead{}, err
		}
		var r branchTipRead
		if err := json.Unmarshal(raw, &r); err != nil {
			return branchTipRead{Error: "journaled tip: " + err.Error()}, nil
		}
		if r.Tip != "" {
			o.mu.Lock()
			o.tips[branch] = r.Tip
			o.mu.Unlock()
		}
		return r, nil
	}
}

// errNotJournaled fails a tip step whose read is not to be kept.
var errNotJournaled = errors.New("tip not resolved; not journaled")

// tipAt is the tip a writer call's check read for branch this launch, ""
// when none did.
func (o *orch) tipAt(branch string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.tips[branch]
}

func (o *orch) checkShaFrom(ctx context.Context, at plan.At, branch, reported string, read func(context.Context, string) (branchTipRead, error)) (sha string, sig *plan.Signal, unresolved bool, err error) {
	if o.git == nil || branch == "" || strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, "~^:@{\\ \t\n") || strings.Contains(branch, "..") {
		return reported, nil, false, nil
	}
	t, err := read(ctx, branch)
	switch {
	case err != nil:
		return reported, nil, false, err
	case t.Unresolved:
		return reported, nil, true, nil
	case t.Error != "":
		return reported, plan.ShaCorrectedSignal(at, branch, reported, "", t.Error), false, nil
	}
	tip := t.Tip
	if r := strings.ToLower(jsTrim(reported)); shaHex.MatchString(r) && strings.HasPrefix(tip, r) {
		return r, nil, false, nil
	}
	return tip, plan.ShaCorrectedSignal(at, branch, reported, tip, ""), false, nil
}

// checkReturn checks a worktree writer's reported branch and SHA (D23).
// With the runtime's branch for the call (rtBranch), a reported branch
// that differs, or a branch that does not resolve, is a branch-mismatch
// stop, and the SHA is checked against rtBranch, which replaces the
// reported branch (the report may differ by whitespace). Without one (a
// stub, a seed, a record journaled before the branch was), the SHA is
// checked against the reported branch, and a branch that does not resolve
// leaves it as reported with no signal. stop reports a branch-mismatch.
func (o *orch) checkReturn(ctx context.Context, at plan.At, call callMeta, branchField, sha *string, sigs *[]*plan.Signal) (stop bool, err error) {
	rtBranch := call.branch
	reported := *branchField
	branch := reported
	read := o.liveTip
	if rtBranch != "" {
		if jsTrim(reported) != rtBranch {
			*sigs = append(*sigs, plan.BranchMismatchSignal(at, reported, rtBranch, ""))
			return true, nil
		}
		branch = rtBranch
		read = o.journaledTip(call.label, reported+" "+*sha)
	}
	fixed, sig, unresolved, err := o.checkShaFrom(ctx, at, branch, *sha, read)
	if err != nil {
		return false, err
	}
	if unresolved && rtBranch != "" {
		*sigs = append(*sigs, plan.BranchMismatchSignal(at, reported, rtBranch, "does not resolve"))
		return true, nil
	}
	*sha, *branchField = fixed, branch
	if sig != nil {
		*sigs = append(*sigs, sig)
	}
	return false, nil
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

// fixCall is a plan or code fix's call, its branch and SHA checked
// (checkReturn). A branch-mismatch is among the signals, so the critique
// loop stops before taking the fix's branch.
func (o *orch) fixCall(ctx context.Context, at plan.At, prompt string, opts lsfrt.Opts) (*ret[fixResult], []*plan.Signal, error) {
	r, sigs, call, err := callBranch[fixResult](ctx, o, site{at: at, writer: true}, prompt, opts)
	if err != nil || r == nil {
		return r, sigs, err
	}
	_, err = o.checkReturn(ctx, at, call, &r.Result.Branch, &r.Result.Sha, &sigs)
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
	mark := o.worktreeMark(u.ID)
	abort := func(stage string) (*unitResult, error) {
		res.aborted = stage
		res.signals = sigs
		return res, nil
	}
	signal := func(stage, kind, statement, referent string) *plan.Signal {
		return plan.NewSignal(plan.Signal{Kind: kind, Source: "structural", Stage: stage, Unit: u.ID, Wave: plan.W(wave), Statement: statement, Referent: referent})
	}

	// 1. plan
	planned, cs, planCall, err := callBranch[unitPlanResult](ctx, o, site{at: at("plan"), writer: true}, unitPlanPrompt(c, pr, u), o.unitOpts(u, "plan", "planning", lsfrt.Opts{Isolation: "worktree", Schema: unitPlanSchema}))
	if err != nil {
		return nil, err
	}
	sigs = append(sigs, cs...)
	if planned == nil {
		sigs = append(sigs, plan.NullReturnSignal(at("plan")))
		return abort("plan")
	}
	mismatch, err := o.checkReturn(ctx, at("plan"), planCall, &planned.Result.Branch, &planned.Result.Sha, &sigs)
	if err != nil {
		return nil, err
	}
	pa := at("plan")
	pa.Path = planned.Result.Path
	sigs = append(sigs, plan.SignalsFromReadings(len(planned.Result.Readings), pa)...)
	pa.TargetKB = unitPlanTargetKB
	sigs = append(sigs, plan.SignalsFromDocumentSize(float64(planned.Result.Bytes), pa)...)
	if hasStop(cs) || mismatch {
		return abort("plan")
	}
	branch := planned.Result.Branch

	// 2. critique gate over the plan
	planGate, err := critiqueLoop(at, &sigs,
		func(n int) ([]reviewer, error) {
			r, rs, err := call[findingsResult](ctx, o, site{at: at("critique-plan"), writer: false}, planCritiquePrompt(c, pr, u, branch, n),
				o.unitOpts(u, fmt.Sprintf("critique-plan:r%d", n), "critique", lsfrt.Opts{AgentType: "phronithm:critic", Schema: findingsSchema}))
			return []reviewer{{"critique-plan", r, rs}}, err
		},
		func(n int, criticals []critical) (*ret[fixResult], []*plan.Signal, error) {
			return o.fixCall(ctx, at("fix"), fixPrompt(c, pr, u, branch, "plan", criticals, n, nil, o.decl),
				o.unitOpts(u, fmt.Sprintf("fix-plan:r%d", n), "", lsfrt.Opts{Isolation: "worktree", Schema: fixSchema}))
		},
		func(fixed *ret[fixResult], n int) ([]*plan.Signal, error) {
			branch = fixed.Result.Branch
			return nil, nil
		},
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
	impl, cs, implCall, err := callBranch[implementResult](ctx, o, site{at: at("implement"), writer: true}, implementPrompt(c, pr, u, branch, o.decl), o.unitOpts(u, "implement", "", lsfrt.Opts{Isolation: "worktree", Schema: implementSchema}))
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
	// The branch is checked first, so a drifted return on a foreign branch
	// records both.
	mismatch, err = o.checkReturn(ctx, at("implement"), implCall, &impl.Result.Branch, &impl.Result.Sha, &sigs)
	if err != nil {
		return nil, err
	}
	if drifted {
		return abort("implement")
	}
	if !mismatch {
		branch = impl.Result.Branch
	}
	sigs = append(sigs, plan.SignalsFromImplement(impl.Result.ImplementResult, at("implement"))...)
	sigs = append(sigs, plan.DistressSignals(impl.Result.Distress.plan(), plan.At{Unit: u.ID, Wave: plan.W(wave)}, c.config)...)
	if hasStop(cs) || mismatch {
		return abort("implement")
	}
	design := u.Type == "design"
	gr := gatesRun{by: "The implement stage", gates: impl.Result.Gates}
	// A design unit's review reads no gates.
	switch {
	case design:
	case o.decl.Gated():
		run, gsigs, stop, err := o.declaredGates(ctx, at("implement"), impl.Result.Sha, impl.Result.Gates)
		if err != nil {
			return nil, err
		}
		sigs = append(sigs, gsigs...)
		if stop {
			return abort("implement")
		}
		gr, res.gatedTree = run.gr, run.tree
	default:
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
				v, rs := o.callSigs(site{at: at(r.stage), writer: false}, res[i].cr, res[i].label)
				out = append(out, reviewer{r.stage, decode[findingsResult](o.rt, res[i].label, v), rs})
			}
			return out, nil
		},
		func(n int, criticals []critical) (*ret[fixResult], []*plan.Signal, error) {
			schema := fixCodeSchema
			if design {
				schema = fixSchema
			}
			return o.fixCall(ctx, at("fix"), fixPrompt(c, pr, u, branch, "code", criticals, n, impl.Result.Gates, o.decl),
				o.unitOpts(u, fmt.Sprintf("fix-code:r%d", n), "", lsfrt.Opts{Isolation: "worktree", Schema: schema}))
		},
		func(fixed *ret[fixResult], n int) ([]*plan.Signal, error) {
			branch = fixed.Result.Branch
			if design {
				return nil, nil
			}
			if o.decl.Gated() {
				run, gsigs, stop, err := o.declaredGates(ctx, at("fix"), fixed.Result.Sha, fixed.Result.Gates)
				if err != nil {
					return nil, err
				}
				if stop {
					return gsigs, nil
				}
				sigs = append(sigs, gsigs...)
				gr, res.gatedTree, fixedGates = run.gr, run.tree, true
				return nil, nil
			}
			// A gate the implement stage ran and the fix did not report is
			// omitted: a dropped gate would otherwise leave the reviewers a
			// shrunken result.
			if gs := gateSignals(at("fix"), fixed.Result.Gates, impl.Result.Gates); len(gs) > 0 {
				return gs, nil
			}
			gr = gatesRun{by: fmt.Sprintf("The round-%d fix stage", n), gates: fixed.Result.Gates}
			fixedGates = true
			return nil, nil
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
	review := reviewOutcome{rounds: reviewGate.rounds, findings: reviewGate.findings, fixedGates: fixedGates, declared: o.decl.Gated() && !design}
	if fixedGates || review.declared {
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
		// Exit 1 is an unresolved branch; any other git error is reported
		// as itself, as checkSha separates them.
		fault := func(reason string) (*unitResult, error) {
			sigs = append(sigs, stageFaultSignal(plan.At{Stage: "completion-report", Unit: u.ID, Wave: plan.W(wave), Path: completionPath(u)}, reason))
			return abort("completion-report")
		}
		if strings.HasPrefix(branch, "-") {
			return fault(fmt.Sprintf("unit branch %q is not a branch name", branch))
		}
		sha, err := gitOps{o.git}.out(ctx, o.repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch+"^{commit}")
		switch {
		case err != nil && ctx.Err() != nil:
			return nil, &lsfrt.StopError{Reason: lsfrt.StopCancelled}
		case err != nil && exitCode(err) == 1:
			// Cleanup deletes a merged unit's branches: a replay of it takes
			// the tip its last writer's check journaled (D25).
			if sha = o.tipAt(branch); sha == "" {
				return fault(fmt.Sprintf("unit branch %q does not resolve", branch))
			}
		case err != nil:
			return fault(fmt.Sprintf("unit branch %q: %v", branch, err))
		}
		in.BaseSha = sha
	}
	report, fault, reportLabel, err := stepLabel[completionResult](ctx, o, "completion-report:"+u.ID, in, o.completionReport(in))
	if err != nil {
		return nil, err
	}
	// Noted here, not in the step's fn, which a replay does not run.
	o.noteWorktree(reportLabel)
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
	res.reportSha = report.Result.Sha
	res.worktrees = o.worktreesSince(u.ID, mark)
	res.completionPath = completionPath(u)
	res.signals = sigs
	return res, nil
}
