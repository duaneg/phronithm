package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// outcome is the launch's return, in presentation order: status and
// escalation first; the ledger stays in the signals file.
type outcome struct {
	Status            string            `json:"status"`
	Workflow          string            `json:"workflow"`
	Escalation        *plan.Escalation  `json:"escalation,omitempty"`
	Digest            plan.DigestResult `json:"digest"`
	PlanSha           string            `json:"planSha"`
	IntegrationBranch string            `json:"integrationBranch"`
	Waves             int               `json:"waves"`
	LedgerPath        string            `json:"ledgerPath"`
	Unscribed         []*plan.Signal    `json:"unscribed"`
	Note              string            `json:"note,omitempty"`
}

// decisionsFor are the questions an escalation puts to the requester.
func decisionsFor(stops []*plan.Signal) []plan.DecisionNeeded {
	out := []plan.DecisionNeeded{}
	for i, s := range stops {
		unit := ""
		affects := []string{s.Stage}
		if s.Unit != "" {
			unit = ", " + s.Unit
			affects = []string{s.Unit}
		}
		out = append(out, plan.DecisionNeeded{
			ID:       fmt.Sprintf("d%d", i+1),
			Question: fmt.Sprintf("%s (%s, %s%s): %s", s.Kind, s.Source, s.Stage, unit, s.Statement),
			Affects:  affects,
		})
	}
	return out
}

// rpItem is an item a re-plan answers: its id, its unit, the text a verdict
// row appends (decision re-plan only; a wave item's own row is already in
// the ledger), the text a stop carries, and its ledger referent.
type rpItem struct {
	id, unit, describe, stopText, referent string
}

func describeItem(it replanItem) string { return "[" + it.Stage + "] " + it.Kind + ": " + it.Statement }

var budgetKind = map[string]string{"extracted": "over-budget-repaired", "flagged": "over-budget-flagged", "unrepaired": "unit-over-budget"}

// replanSignals are the signals from a re-plan return: a verdict per item
// (cannot or missing is a stop; document-text also records the architecture
// amendment), each amendment, and each over-budget unit by disposition
// (unrepaired is a stop). A standing flag is reported on every pass but
// recorded once: ledger supplies the last over-budget kind per unit, and a
// repaired or flagged unit whose kind is unchanged adds no row. An
// unrepaired unit always does; it stops.
func replanSignals(r *replanResult, items []rpItem, stage string, wave int, ledger []*plan.Signal) []*plan.Signal {
	out := []*plan.Signal{}
	w := plan.W(wave)
	verdicts := map[string]verdict{}
	for _, v := range r.Verdicts {
		verdicts[v.ID] = v
	}
	for _, it := range items {
		v, ok := verdicts[it.id]
		if ok && v.Verdict != "cannot" {
			st := it.id + " " + v.Verdict + ": " + v.Statement
			if it.describe != "" {
				st += " — " + it.describe
			}
			out = append(out, plan.NewSignal(plan.Signal{Kind: "verdict", Source: "reported", Stage: stage, Unit: it.unit, Wave: w, Statement: st, Referent: it.referent}))
			if v.Verdict == "document-text" {
				out = append(out, plan.NewSignal(plan.Signal{Kind: "architecture-amendment", Source: "reported", Stage: stage, Wave: w, Statement: v.Statement, Referent: it.referent}))
			}
			continue
		}
		why := "re-plan gave no verdict"
		if ok {
			why = "re-plan cannot absorb: " + v.Statement
		}
		out = append(out, plan.NewSignal(plan.Signal{Kind: "plan-defect-unabsorbed", Source: "reported", Stage: stage, Unit: it.unit, Wave: w, Statement: why + " — " + it.stopText, Referent: it.referent}))
	}
	for _, a := range r.Amendments {
		out = append(out, plan.NewSignal(plan.Signal{Kind: "amendment", Source: "reported", Stage: stage, Unit: a.Unit, Wave: w, Statement: a.Kind + ": " + a.Statement, Referent: a.Unit}))
	}
	standing := map[string]string{}
	for _, s := range ledger {
		for _, k := range budgetKind {
			if s.Kind == k {
				standing[s.Unit] = s.Kind
			}
		}
	}
	for _, b := range r.OverBudget {
		kind, ok := budgetKind[b.Disposition]
		if !ok {
			kind = "unit-over-budget"
		}
		if kind != "unit-over-budget" && standing[b.Unit] == kind {
			continue
		}
		out = append(out, plan.NewSignal(plan.Signal{Kind: kind, Source: "reported", Stage: stage, Unit: b.Unit, Wave: w, Statement: b.Statement, Referent: b.Unit}))
	}
	return out
}

// carriedRow is a row from args.unscribed as a signal; a row NewSignal
// rejects is an args error, as JS's signal() throw rejects the run.
func carriedRow(row plan.Signal) (s *plan.Signal, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("lsf-implement: args.unscribed: %v", p)
		}
	}()
	return plan.NewSignal(row), nil
}

// jsNumber is a JS number's default string form, for the integral and
// ordinary values args carry.
func jsNumber(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// launch is run's mutable state.
type launch struct {
	*orch
	ledger         []*plan.Signal
	carried        map[*plan.Signal]bool
	scribed        map[*plan.Signal]bool
	plan           *plan.Plan
	wave           int
	pauseRequested bool
	ledgerFault    *plan.Signal
	completed      plan.Completed
	terminationRan bool
}

func (l *launch) ref() planRef { return planRef{l.plan.Path, l.plan.Sha} }

func (l *launch) waveOrNil() *int {
	if l.wave == 0 {
		return nil
	}
	return plan.W(l.wave)
}

func (l *launch) inLedger(s *plan.Signal) bool { return slices.Contains(l.ledger, s) }

func (l *launch) outcome(status string, esc *plan.Escalation, note string) (json.RawMessage, error) {
	digestRows := []*plan.Signal{}
	unscribed := []*plan.Signal{}
	for _, s := range l.ledger {
		if !l.carried[s] {
			digestRows = append(digestRows, s)
		}
		if !l.scribed[s] {
			unscribed = append(unscribed, s)
		}
	}
	return plan.MarshalJS(outcome{
		Status: status, Workflow: workflowName, Escalation: esc, Digest: plan.Digest(digestRows),
		PlanSha: l.plan.Sha, IntegrationBranch: l.ctx.integrationBranch, Waves: l.wave,
		LedgerPath: l.ctx.ledgerPath, Unscribed: unscribed, Note: note,
	}, "")
}

// notePause: a stage that saw the pause ref sets this; the loop top honours
// it once the wave's commits are in.
func (l *launch) notePause(requested bool, label string) {
	if !requested || l.pauseRequested {
		return
	}
	l.pauseRequested = true
	l.rt.Log(fmt.Sprintf("pause requested (%s seen by %s); the run pauses at the next wave boundary", l.ctx.pauseRef, label))
}

// scribeLedger appends the rows no scribe has committed. A null scribe
// leaves its rows for the next append and, at return, for unscribed. A
// scribe that returns without a commit SHA may have left its append
// uncommitted in the main checkout, where the next merge would meet it as a
// conflict: that is a stop, and no later scribe runs over the leftover.
func (l *launch) scribeLedger(ctx context.Context, label string) error {
	if label == "" {
		label = "ledger:w" + itoa(l.wave)
	}
	fresh := []*plan.Signal{}
	for _, s := range l.ledger {
		if !l.scribed[s] {
			fresh = append(fresh, s)
		}
	}
	if l.ledgerFault != nil || len(fresh) == 0 {
		return nil
	}
	t := l.ctx.tierFor("mechanical")
	r, err := call[scribeResult](ctx, l.orch, ledgerPrompt(l.ctx, l.ref(), l.wave, fresh),
		lsfrt.Opts{Label: label, Model: t.Model, Effort: t.Effort, Phase: "Ledger", Schema: ledgerSchema})
	if err != nil {
		return err
	}
	if r == nil {
		l.rt.Log(fmt.Sprintf("ledger scribe returned null (%s); %d row(s) stay unscribed", label, len(fresh)))
		return nil
	}
	l.notePause(r.Result.PauseRequested, label)
	if !committed(r.Result.Sha) {
		l.ledgerFault = plan.NoCommitSignal(plan.At{Stage: "ledger", Wave: l.waveOrNil(), Path: l.ctx.ledgerPath})
		l.rt.Log(fmt.Sprintf("ledger scribe returned no commit SHA (%s); %d row(s) stay unscribed", label, len(fresh)))
		return nil
	}
	for _, s := range fresh {
		l.scribed[s] = true
	}
	return nil
}

// stopShas overrides the escalation's plan SHA and sets the pre-re-plan SHA.
type stopShas struct{ planSha, preReplanSha string }

// stopped is every stop path: stamp, ledger, scribe, escalate. stops may
// already be in the ledger (from EvaluateWave); the rest are appended here.
// Rows not yet scribed ride in the same append, so a stop costs one scribe.
func (l *launch) stopped(ctx context.Context, stage string, stops []*plan.Signal, shas *stopShas) (json.RawMessage, error) {
	for _, s := range stops {
		s.Disposition = "stopped"
		if !l.inLedger(s) {
			l.ledger = append(l.ledger, s)
		}
	}
	if err := l.scribeLedger(ctx, ""); err != nil {
		return nil, err
	}
	if l.ledgerFault != nil && !slices.Contains(stops, l.ledgerFault) {
		l.ledgerFault.Disposition = "stopped"
		l.ledger = append(l.ledger, l.ledgerFault)
		stops = append(append([]*plan.Signal{}, stops...), l.ledgerFault)
	}
	kinds := []string{}
	for _, s := range stops {
		kinds = append(kinds, s.Kind)
	}
	l.rt.Log(fmt.Sprintf("STOP at %s: %s", stage, strings.Join(kinds, ", ")))
	esc := plan.Escalation{Workflow: workflowName, PlanSha: l.plan.Sha, Wave: l.waveOrNil(), Stage: stage, Signals: stops, DecisionNeeded: decisionsFor(stops)}
	if shas != nil {
		esc.PlanSha, esc.PreReplanSha = shas.planSha, shas.preReplanSha
	}
	e := plan.NewEscalation(esc)
	return l.outcome("stopped", &e, "")
}

// scribeOrStop scribes, and stops when the scribe left no commit.
func (l *launch) scribeOrStop(ctx context.Context, label string) (json.RawMessage, bool, error) {
	if err := l.scribeLedger(ctx, label); err != nil {
		return nil, true, err
	}
	if l.ledgerFault != nil {
		out, err := l.stopped(ctx, "ledger", []*plan.Signal{}, nil)
		return out, true, err
	}
	return nil, false, nil
}

// applyDirectEdits has one scribe make and commit the edits a planning
// stage returned as outside the plan; the ledger records each against the
// stage that asked. It returns the signals; the caller evaluates them.
func (l *launch) applyDirectEdits(ctx context.Context, at planRef, edits []directEdit, what, stage string, wave *int, phase string) ([]*plan.Signal, error) {
	if len(edits) == 0 {
		return nil, nil
	}
	label := "direct-edits"
	if wave != nil {
		label = "direct-edits:w" + itoa(*wave)
	}
	t := l.ctx.tierFor("default")
	r, err := call[scribeResult](ctx, l.orch, directEditsPrompt(l.ctx, at, what, edits),
		lsfrt.Opts{Label: label, Model: t.Model, Effort: t.Effort, Phase: phase, Schema: scribeSchema})
	if err != nil {
		return nil, err
	}
	if r == nil {
		return []*plan.Signal{plan.NullReturnSignal(plan.At{Stage: "direct-edits", Wave: wave})}, nil
	}
	if !committed(r.Result.Sha) {
		paths := []string{}
		for _, e := range edits {
			paths = append(paths, e.Path)
		}
		return []*plan.Signal{plan.NoCommitSignal(plan.At{Stage: "direct-edits", Wave: wave, Path: strings.Join(paths, ",")})}, nil
	}
	out := []*plan.Signal{}
	for _, e := range edits {
		out = append(out, plan.NewSignal(plan.Signal{Kind: "direct-edit", Source: "reported", Stage: stage, Wave: wave, Statement: e.Description, Referent: e.Path}))
	}
	return out, nil
}

// loaded is a plan read at a SHA: a copy agent returns the fenced blocks
// verbatim, the plan package parses and validates. Structural problems
// (unreadable block, unknown unit, cycle, unserialised overlap, bad
// clearance) are stop signals.
type loaded struct {
	plan  *plan.Plan
	copy  *ret[copyResult]
	stops []*plan.Signal
}

func (l *launch) loadPlan(ctx context.Context, sha, label string, wave *int) (loaded, error) {
	t := l.ctx.tierFor("mechanical")
	cp, err := call[copyResult](ctx, l.orch, copyPrompt(l.ctx, sha), lsfrt.Opts{Label: label, Model: t.Model, Effort: t.Effort, Phase: "Load", Schema: copySchema})
	if err != nil {
		return loaded{}, err
	}
	if cp == nil {
		return loaded{stops: []*plan.Signal{plan.NullReturnSignal(plan.At{Stage: "load", Wave: wave})}}, nil
	}
	l.notePause(cp.Result.PauseRequested, label)
	var decisions *string
	if l.ctx.decisionLogSha != "" {
		decisions = &cp.Result.DecisionsBlock
	}
	g := plan.LoadGraph(cp.Result.PlanBlock, decisions)
	problem := func(p plan.Problem) *plan.Signal {
		return plan.NewSignal(plan.Signal{Kind: p.Kind, Statement: p.Statement, Referent: p.Referent, Source: "structural", Stage: "load", Wave: wave})
	}
	out := loaded{copy: cp, stops: []*plan.Signal{}}
	for _, p := range g.Problems {
		out.stops = append(out.stops, problem(p))
	}
	if g.Plan != nil {
		p := *g.Plan
		p.Path, p.Sha = l.ctx.planPath, sha
		out.plan = &p
		for _, pr := range plan.ValidatePlan(out.plan) {
			out.stops = append(out.stops, problem(pr))
		}
	}
	return out, nil
}

// refreshPin: the pin follows the integration branch; a reload's copy stage
// reports the document's SHA after any document-text correction.
func (l *launch) refreshPin(reported string) {
	if r := jsTrim(reported); r != "" {
		l.ctx.architectureSha = some(r)
	}
}

func (l *launch) markComplete() {
	for _, u := range l.plan.Units {
		if u.Complete {
			l.completed[u.ID] = true
		}
	}
}

func (l *launch) record(ev plan.WaveEval) { l.ledger = append(l.ledger, ev.All...) }

// Run is run(runtime): Phase 3 of phronithm:large-scale-feature as a wave
// loop. See docs/design/lsf-workflow.md, "Wave loop", "Escalation",
// "Persistence", and workflows/lsf/implement.js's header for the args and
// the return.
//
// The plan graph is read from the plan's ```yaml lsf-plan block and gate
// clearances from the decision log's ```yaml lsf-decisions block
// (docs/plan-graph.md). A copy agent fetches the blocks; package plan parses
// them. No agent interprets the plan's prose on the loop's behalf.
//
// The loop stops only on structural signals (merge, tests, gate cap, null
// returns, plan validity) and on two adjudicated ones: the wave gate's
// complete architecture invalidation and the re-plan's `cannot` verdict.
// Everything else agents report is recorded in the ledger and handed to the
// re-plan.
func Run(ctx context.Context, rt Runtime) (json.RawMessage, error) {
	a, err := parseArgs(rt.Args())
	if err != nil {
		return nil, err
	}
	c, err := newRunCtx(a)
	if err != nil {
		return nil, err
	}
	l := &launch{
		orch:      &orch{rt: rt, ctx: c, seed: a.Seed, uses: map[string]int{}},
		carried:   map[*plan.Signal]bool{},
		scribed:   map[*plan.Signal]bool{},
		plan:      &plan.Plan{Path: c.planPath, Sha: c.planSha, Units: []*plan.Unit{}, ExecutionOrder: []string{}},
		completed: plan.Completed{},
	}

	// Rows a previous run returned unscribed. They keep their own wave and
	// disposition, are scribed first, and stay out of this run's digest.
	var rows []plan.Signal
	if len(a.Unscribed) > 0 && a.Unscribed[0] == '[' {
		if err := json.Unmarshal(a.Unscribed, &rows); err != nil {
			return nil, fmt.Errorf("lsf-implement: args.unscribed: %w", err)
		}
	}
	for _, row := range rows {
		s, err := carriedRow(row)
		if err != nil {
			return nil, err
		}
		l.ledger = append(l.ledger, s)
		l.carried[s] = true
	}
	if len(l.carried) > 0 {
		rt.Log(fmt.Sprintf("%d ledger row(s) carried from a previous run", len(l.carried)))
		if out, halt, err := l.scribeOrStop(ctx, "ledger:carried"); halt {
			return out, err
		}
	}

	// Decisions handed back by the requester go into the repo first, so the
	// SHA can appear in every prompt they affect.
	if len(a.Decisions) > 0 {
		t := c.tierFor("mechanical")
		d, err := call[scribeResult](ctx, l.orch, decisionLogPrompt(c, a.Decisions),
			lsfrt.Opts{Label: "decision-log", Model: t.Model, Effort: t.Effort, Phase: "Load", Schema: decisionLogSchema})
		if err != nil {
			return nil, err
		}
		if d == nil {
			return l.stopped(ctx, "decision-log", []*plan.Signal{plan.NullReturnSignal(plan.At{Stage: "decision-log"})}, nil)
		}
		if !committed(d.Result.Sha) {
			return l.stopped(ctx, "decision-log", []*plan.Signal{plan.NoCommitSignal(plan.At{Stage: "decision-log", Path: c.decisionLogPath})}, nil)
		}
		c.decisionLogSha = d.Result.Sha
		c.decisionLogPath = d.Result.Path

		// Decisions are the plan's to carry out before any unit starts: a
		// re-plan over the decisions, then the run loads the SHA it returns.
		items := []decisionItem{}
		rpItems := []rpItem{}
		for _, d := range a.Decisions {
			items = append(items, decisionItem{d.ID, d.Question, d.Decision})
			rpItems = append(rpItems, rpItem{id: d.ID, describe: d.Question, stopText: d.Question, referent: d.ID})
		}
		t = c.tierFor("planning")
		rp, err := call[replanResult](ctx, l.orch, decisionReplanPrompt(c, l.ref(), items),
			lsfrt.Opts{Label: "decision-replan", Model: t.Model, Effort: t.Effort, Phase: "Load", Schema: replanSchema})
		if err != nil {
			return nil, err
		}
		if rp == nil {
			return l.stopped(ctx, "decision-replan", []*plan.Signal{plan.NullReturnSignal(plan.At{Stage: "decision-replan"})}, nil)
		}
		sigs := replanSignals(&rp.Result, rpItems, "decision-replan", 0, l.ledger)
		edits, err := l.applyDirectEdits(ctx, planRef{l.plan.Path, rp.Result.PlanSha}, rp.Result.DirectEdits, "decision re-plan", "decision-replan", plan.W(0), "Load")
		if err != nil {
			return nil, err
		}
		ev := plan.EvaluateWave(append(sigs, edits...))
		l.record(ev)
		if ev.Stop {
			return l.stopped(ctx, "decision-replan", ev.Stops, &stopShas{rp.Result.PlanSha, l.plan.Sha})
		}
		if out, halt, err := l.scribeOrStop(ctx, ""); halt {
			return out, err
		}
		l.plan = &plan.Plan{Path: l.plan.Path, Sha: rp.Result.PlanSha, Units: l.plan.Units, ExecutionOrder: l.plan.ExecutionOrder}
		c.planSha = l.plan.Sha
		rt.Log(fmt.Sprintf("decisions applied: %d amendment(s) → plan @ %s", len(rp.Result.Amendments), l.plan.Sha))
	}

	// Load.
	first, err := l.loadPlan(ctx, c.planSha, "load", nil)
	if err != nil {
		return nil, err
	}
	if first.copy == nil {
		return l.stopped(ctx, "load", first.stops, nil)
	}
	if !c.architectureSha.set {
		c.architectureSha = some(first.copy.Result.ArchitectureSha)
	}
	loadSignals := append([]*plan.Signal{}, first.stops...)
	if first.copy.Result.HeadBranch != c.integrationBranch {
		loadSignals = append(loadSignals, plan.NewSignal(plan.Signal{
			Kind: "precondition", Source: "structural", Stage: "load",
			Statement: "main checkout is on " + first.copy.Result.HeadBranch + ", not " + c.integrationBranch, Referent: c.integrationBranch,
		}))
	}
	loadEval := plan.EvaluateWave(loadSignals)
	l.record(loadEval)
	if loadEval.Stop || first.plan == nil {
		return l.stopped(ctx, "load", loadEval.Stops, nil)
	}
	l.plan = first.plan
	logSource := "no decision log"
	if c.decisionLogSha != "" {
		logSource = "decision log applied"
	}
	projected := []string{}
	for _, w := range plan.WavePlan(l.plan, nil) {
		projected = append(projected, strings.Join(w, "+"))
	}
	waves := strings.Join(projected, " | ")
	if waves == "" {
		waves = "none"
	}
	rt.Log(fmt.Sprintf("loaded %d units @ %s; %s; projected waves: %s", len(l.plan.Units), l.plan.Sha, logSource, waves))
	l.markComplete()

	for {
		out, ret, brk, err := l.step(ctx)
		if ret {
			return out, err
		}
		if brk {
			break
		}
	}
	return l.finish(ctx)
}
