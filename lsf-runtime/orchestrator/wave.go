package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// step is one pass of the loop: a pause, a schedule stop, termination, or a
// wave with its merge, gate and re-plan. ret means return out and err; brk
// means every unit is done and termination has run.
func (l *launch) step(ctx context.Context) (out json.RawMessage, ret, brk bool, err error) {
	c := l.ctx
	// Everything the wave produced is committed by here; a pause returns
	// like a wave cap, from the plan as it stands.
	if l.pauseRequested {
		if out, halt, err := l.scribeOrStop(ctx, ""); halt {
			return out, true, false, err
		}
		l.rt.Log(fmt.Sprintf("PAUSED after wave %d on %s", l.wave, c.pauseRef))
		out, err := l.outcome("paused", nil, fmt.Sprintf("paused after %d wave(s): %s exists; %d unit(s) remain. To resume: delete the ref (git update-ref -d %s) and relaunch from planSha with no decisions, passing unscribed back when non-empty",
			l.wave, c.pauseRef, len(l.plan.Units)-len(l.completed), c.pauseRef))
		return out, true, false, err
	}
	allReady := plan.ReadySet(l.plan, l.completed)
	ready := jsSlice(allReady, c.maxUnitsPerWave)
	if len(ready) < len(allReady) {
		ids := []string{}
		for _, u := range allReady[len(ready):] {
			ids = append(ids, u.ID)
		}
		l.rt.Log(fmt.Sprintf("wave %d: %d ready, capped at %d: %s deferred", l.wave+1, len(allReady), len(ready), strings.Join(ids, ", ")))
	}
	if len(ready) == 0 {
		if blocked := plan.BlockedUnits(l.plan, l.completed); len(blocked) > 0 {
			stops := []*plan.Signal{}
			for _, b := range blocked {
				st, ref := "waiting on "+strings.Join(b.WaitingOn, ", ")+" with nothing ready", b.Unit
				if b.Reason == "gated" {
					st, ref = "uncleared gate(s): "+strings.Join(b.Gates, ", "), strings.Join(b.Gates, ",")
				}
				stops = append(stops, plan.NewSignal(plan.Signal{Kind: "no-ready-unit", Source: "structural", Stage: "schedule", Unit: b.Unit, Wave: plan.W(l.wave + 1), Statement: st, Referent: ref}))
			}
			out, err := l.stopped(ctx, "schedule", stops, nil)
			return out, true, false, err
		}
		if l.terminationRan {
			return nil, false, true, nil
		}
		out, ret, err := l.terminate(ctx)
		return out, ret, false, err
	}

	l.wave++
	wave := l.wave
	if float64(wave) > c.maxWaves {
		out, err := l.outcome("wave-cap", nil, fmt.Sprintf("stopped after %s waves (args.maxWaves); %d unit(s) remain", jsNumber(c.maxWaves), len(l.plan.Units)-len(l.completed)))
		return out, true, false, err
	}
	ids := []string{}
	for _, u := range ready {
		ids = append(ids, u.ID)
	}
	l.rt.Log(fmt.Sprintf("wave %d: %s", wave, strings.Join(ids, ", ")))

	// Units, then barrier: merge needs every branch.
	wavePlan := l.plan
	slots := make([]*unitResult, len(ready))
	thunks := []func(context.Context) (json.RawMessage, error){}
	for i, u := range ready {
		thunks = append(thunks, func(ctx context.Context) (json.RawMessage, error) {
			r, err := l.runUnit(ctx, wavePlan, u, wave)
			if err != nil {
				return nil, err
			}
			slots[i] = r
			return json.RawMessage("true"), nil
		})
	}
	raws, err := l.rt.Parallel(ctx, thunks)
	if err != nil {
		return nil, true, false, err
	}
	results := make([]*unitResult, len(ready))
	waveSignals := []*plan.Signal{}
	byUnit := map[string]*unitResult{}
	mergeable := []string{}
	for i, u := range ready {
		r := slots[i]
		if raws[i] == nil || r == nil {
			r = &unitResult{unit: u.ID, typ: u.Type, aborted: "unit",
				signals: []*plan.Signal{plan.NullReturnSignal(plan.At{Stage: "unit", Unit: u.ID, Wave: plan.W(wave)})}}
		}
		results[i] = r
		waveSignals = append(waveSignals, r.signals...)
		if r.branch != "" {
			byUnit[r.unit] = r
			mergeable = append(mergeable, r.unit)
		}
	}
	order := []unitResult{}
	for _, id := range plan.MergeOrder(l.plan, mergeable) {
		order = append(order, *byUnit[id])
	}

	// Merge, in plan execution order. The pin moves to the merged text
	// before the gate; the gate judges the units' own changes against
	// prevArch.
	prevArch := c.architectureSha
	merged := []mergedUnit{}
	conflicts := []plan.Merge{}
	if len(order) > 0 {
		t := c.tierFor("default")
		label := "merge:w" + itoa(wave)
		m, cs, err := call[mergeResult](ctx, l.orch, site{at: plan.At{Stage: "merge", Wave: plan.W(wave)}, writer: true}, mergePrompt(c, l.ref(), wave, order), lsfrt.Opts{Label: label, Model: t.Model, Effort: t.Effort, Phase: "Merge", Schema: mergeSchema})
		if err != nil {
			return nil, true, false, err
		}
		// A merge stop keeps what it merged: those commits exist, and the
		// wave goes on to the re-plan as any stopping wave with units merged.
		waveSignals = append(waveSignals, cs...)
		if m == nil {
			waveSignals = append(waveSignals, plan.NullReturnSignal(plan.At{Stage: "merge", Wave: plan.W(wave)}))
		} else {
			l.notePause(m.Result.PauseRequested, label)
			waveSignals = append(waveSignals, plan.SignalsFromMerge(m.Result.MergeResult, plan.At{Wave: plan.W(wave)})...)
			for _, x := range m.Result.Merges {
				if x.Status == "clean" || x.Status == "resolved" {
					l.completed[x.Unit] = true
					var mu mergedUnit
					if r := byUnit[x.Unit]; r != nil {
						mu.unitResult = *r
					}
					mu.mergeSha = x.Sha
					merged = append(merged, mu)
				}
				if x.Status == "resolved" {
					conflicts = append(conflicts, x)
				}
			}
			np := *l.plan
			np.Sha = m.Result.PlanSha
			np.Units = nil
			for _, u := range l.plan.Units {
				if l.completed[u.ID] && !u.Complete {
					cu := *u
					cu.Complete = true
					u = &cu
				}
				np.Units = append(np.Units, u)
			}
			l.plan = &np
			l.refreshPin(m.Result.ArchitectureSha)
		}
	}

	// The wave gate: one question, does the wave invalidate the
	// architecture. Merged worktrees are removed alongside it; the gate
	// reads the main checkout only. Both are readers. Cleanup's null is
	// logged, not signalled.
	if len(merged) > 0 {
		gt, mt := c.tierFor("gate"), c.tierFor("mechanical")
		gl, cl := "review-gate:w"+itoa(wave), "cleanup:w"+itoa(wave)
		res, err := l.parallelAgents(ctx, []agentReq{
			{gatePrompt(c, l.ref(), wave, merged, prevArch), lsfrt.Opts{Label: gl, Model: gt.Model, Effort: gt.Effort, Phase: "Gate", Schema: gateSchema}},
			{cleanupPrompt(c, l.ref(), wave), lsfrt.Opts{Label: cl, Model: mt.Model, Effort: mt.Effort, Phase: "Merge", Schema: cleanupSchema}},
		})
		if err != nil {
			return nil, true, false, err
		}
		gv, gs := l.callSigs(site{at: plan.At{Stage: "review-gate", Wave: plan.W(wave)}, writer: false}, res[0].cr, res[0].label)
		cv, cs := l.callSigs(site{at: plan.At{Stage: "cleanup", Wave: plan.W(wave)}, writer: false}, res[1].cr, res[1].label)
		waveSignals = append(append(waveSignals, gs...), cs...)
		g := decode[plan.GateResult](l.rt, res[0].label, gv)
		cu := decode[cleanupResult](l.rt, res[1].label, cv)
		if g == nil {
			waveSignals = append(waveSignals, plan.NullReturnSignal(plan.At{Stage: "review-gate", Wave: plan.W(wave)}))
		} else {
			waveSignals = append(waveSignals, plan.SignalsFromGate(g.Result, plan.At{Wave: plan.W(wave)})...)
		}
		if cu != nil {
			l.rt.Log(fmt.Sprintf("wave %d: removed %d merged worktree(s), kept %d", wave, cu.Result.Removed, len(cu.Result.Kept)))
		} else {
			l.rt.Log(fmt.Sprintf("wave %d: worktree cleanup returned null", wave))
		}
	}

	evalA := plan.EvaluateWave(waveSignals)
	l.record(evalA)
	mergedIDs := mergedUnits(merged)
	mergedText := strings.Join(mergedIDs, ", ")
	if mergedText == "" {
		mergedText = "nothing"
	}
	l.rt.Log(fmt.Sprintf("wave %d: merged %s; %d conflict(s); %d recorded, %d stop signal(s)", wave, mergedText, len(conflicts), len(evalA.Records), len(evalA.Stops)))
	// A wave stop still re-plans over whatever merged, unless nothing did
	// or the stop supersedes the plan (its relaunch is Phase 2's re-entry).
	// A relaunch re-plans over decisions alone, so this is the only pass
	// that absorbs the merged reports.
	superseded := false
	for _, s := range evalA.Stops {
		if slices.Contains(plan.PlanSupersedingStopKinds, s.Kind) {
			superseded = true
		}
	}
	if evalA.Stop && (len(merged) == 0 || superseded) {
		out, err := l.stopped(ctx, "wave", evalA.Stops, nil)
		return out, true, false, err
	}

	// Re-plan from the wave's completion reports. Items go in with ids; one
	// the re-plan cannot absorb, or does not answer, is a stop.
	items := []replanItem{}
	rpItems := []rpItem{}
	for _, s := range evalA.Records {
		if !slices.Contains(plan.ReplanItemKinds, s.Kind) {
			continue
		}
		it := replanItem{ID: fmt.Sprintf("rp-%d", len(items)+1), Unit: s.Unit, Stage: s.Stage, Kind: s.Kind, Statement: s.Statement, Referent: s.Referent}
		items = append(items, it)
		rpItems = append(rpItems, rpItem{id: it.ID, unit: it.Unit, stopText: describeItem(it), referent: s.Referent})
	}
	// Nothing to absorb — no item and no claimed-disjoint conflict — and the
	// re-plan has no work: the plan already carries the merge's SHA and
	// completion flags, so the next wave schedules from it as it stands.
	if len(items) == 0 && len(conflicts) == 0 {
		l.rt.Log(fmt.Sprintf("wave %d: nothing to absorb; re-plan skipped, plan @ %s", wave, l.plan.Sha))
		if evalA.Stop {
			out, err := l.stopped(ctx, "wave", evalA.Stops, nil)
			return out, true, false, err
		}
		if out, halt, err := l.scribeOrStop(ctx, ""); halt {
			return out, true, false, err
		}
		l.markComplete()
		return nil, false, false, nil
	}
	pt := c.tierFor("planning")
	rp, cs, err := call[replanResult](ctx, l.orch, site{at: plan.At{Stage: "replan", Wave: plan.W(wave)}, writer: true}, replanPrompt(c, l.ref(), wave, merged, conflicts, items, evalA.Stops),
		lsfrt.Opts{Label: "replan:w" + itoa(wave), Model: pt.Model, Effort: pt.Effort, Schema: replanSchema, Phase: "Re-plan"})
	if err != nil {
		return nil, true, false, err
	}
	replanSigs := append([]*plan.Signal{}, cs...)
	nextPlan := l.plan
	if rp == nil {
		replanSigs = append(replanSigs, plan.NullReturnSignal(plan.At{Stage: "replan", Wave: plan.W(wave)}))
	} else {
		replanSigs = append(replanSigs, replanSignals(&rp.Result, rpItems, "replan", wave, l.ledger)...)
		// Before the load, and on a stopping wave too, so the edits are not
		// lost with the stop.
		edits, err := l.applyDirectEdits(ctx, planRef{l.plan.Path, rp.Result.PlanSha}, rp.Result.DirectEdits, fmt.Sprintf("wave %d re-plan", wave), "replan", plan.W(wave), "Re-plan")
		if err != nil {
			return nil, true, false, err
		}
		replanSigs = append(replanSigs, edits...)
		re, err := l.loadPlan(ctx, rp.Result.PlanSha, "load:w"+itoa(wave), plan.W(wave))
		if err != nil {
			return nil, true, false, err
		}
		if re.copy != nil {
			l.refreshPin(re.copy.Result.ArchitectureSha)
		}
		replanSigs = append(replanSigs, re.stops...)
		if re.plan != nil {
			nextPlan = re.plan
		}
	}
	evalB := plan.EvaluateWave(replanSigs)
	l.record(evalB)
	// The re-plan committed its absorbed amendments before the stop; the
	// record carries that SHA as the plan to relaunch from.
	if evalA.Stop || evalB.Stop {
		stage := "replan"
		if evalA.Stop {
			stage = "wave"
		}
		var shas *stopShas
		if rp != nil {
			shas = &stopShas{rp.Result.PlanSha, l.plan.Sha}
		}
		out, err := l.stopped(ctx, stage, append(append([]*plan.Signal{}, evalA.Stops...), evalB.Stops...), shas)
		return out, true, false, err
	}
	if out, halt, err := l.scribeOrStop(ctx, ""); halt {
		return out, true, false, err
	}
	l.plan = nextPlan
	l.markComplete()
	l.rt.Log(fmt.Sprintf("wave %d: re-plan %d amendment(s) → plan @ %s; %s", wave, len(rp.Result.Amendments), l.plan.Sha, rp.Result.Summary))
	return nil, false, false, nil
}

// terminate is the termination stage: meta-tasks become units; small ones
// are direct edits.
func (l *launch) terminate(ctx context.Context) (json.RawMessage, bool, error) {
	c := l.ctx
	l.terminationRan = true
	t := c.tierFor("planning")
	term, cs, err := call[terminationResult](ctx, l.orch, site{at: plan.At{Stage: "termination"}, writer: true}, terminationPrompt(c, l.ref()), lsfrt.Opts{Label: "termination", Model: t.Model, Effort: t.Effort, Phase: "Termination", Schema: terminationSchema})
	if err != nil {
		return nil, true, err
	}
	if term == nil {
		ev := plan.EvaluateWave(slices.Concat(cs, []*plan.Signal{plan.NullReturnSignal(plan.At{Stage: "termination"})}))
		l.record(ev)
		out, err := l.stopped(ctx, "termination", ev.Stops, nil)
		return out, true, err
	}
	re, err := l.loadPlan(ctx, term.Result.PlanSha, "load:termination", plan.W(l.wave))
	if err != nil {
		return nil, true, err
	}
	if re.copy != nil {
		l.refreshPin(re.copy.Result.ArchitectureSha)
	}
	termEval := plan.EvaluateWave(slices.Concat(cs, re.stops))
	l.record(termEval)
	if termEval.Stop {
		out, err := l.stopped(ctx, "termination", termEval.Stops, nil)
		return out, true, err
	}
	l.plan = re.plan
	edits, err := l.applyDirectEdits(ctx, l.ref(), term.Result.DirectEdits, "termination stage (the plan's Meta-Tasks section)", "termination", nil, "Termination")
	if err != nil {
		return nil, true, err
	}
	editEval := plan.EvaluateWave(edits)
	l.record(editEval)
	if editEval.Stop {
		out, err := l.stopped(ctx, "direct-edits", editEval.Stops, nil)
		return out, true, err
	}
	l.rt.Log(fmt.Sprintf("termination: %d meta unit(s) appended, %d direct edit(s)", len(term.Result.AppendedUnits), len(term.Result.DirectEdits)))
	return nil, false, nil
}

// finish runs the standing-obligations stage. One carried by its own terms
// is a record when the stage names where it is recorded outside the plan;
// otherwise a stop.
func (l *launch) finish(ctx context.Context) (json.RawMessage, error) {
	c := l.ctx
	t := c.tierFor("default")
	ob, cs, err := call[obligationsResult](ctx, l.orch, site{at: plan.At{Stage: "obligations"}, writer: false}, obligationsPrompt(c, l.ref()), lsfrt.Opts{Label: "obligations", Model: t.Model, Effort: t.Effort, Phase: "Termination", Schema: obligationsSchema})
	if err != nil {
		return nil, err
	}
	if ob == nil {
		ev := plan.EvaluateWave(slices.Concat(cs, []*plan.Signal{plan.NullReturnSignal(plan.At{Stage: "obligations"})}))
		l.record(ev)
		return l.stopped(ctx, "obligations", ev.Stops, nil)
	}
	sigs := append([]*plan.Signal{}, cs...)
	for _, o := range ob.Result.Obligations {
		if o.Disposition == "discharged" {
			continue
		}
		carrier := jsTrim(o.Carrier)
		if o.Disposition == "carried" && carrier != "" {
			sigs = append(sigs, plan.NewSignal(plan.Signal{Kind: "standing-obligation-carried", Source: "reported", Stage: "obligations", Statement: o.Statement, Referent: carrier}))
			continue
		}
		prefix := ""
		if o.Disposition == "carried" {
			prefix = "carried without a carrier: "
		}
		sigs = append(sigs, plan.NewSignal(plan.Signal{Kind: "standing-obligation-undischarged", Source: "structural", Stage: "obligations", Statement: prefix + o.Statement, Referent: o.Statement}))
	}
	ev := plan.EvaluateWave(sigs)
	l.record(ev)
	if ev.Stop {
		return l.stopped(ctx, "obligations", ev.Stops, nil)
	}
	if out, halt, err := l.scribeOrStop(ctx, ""); halt {
		return out, err
	}
	l.rt.Log(fmt.Sprintf("complete: %d unit(s) over %d wave(s); silence is not evidence", len(l.completed), l.wave))
	return l.outcome("complete", nil, "")
}
