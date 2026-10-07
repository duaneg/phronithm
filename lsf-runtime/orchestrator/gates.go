package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// Declared gates (D27): with a ```yaml lsf-gates block in the plan, the
// runtime runs the gates on a clean checkout, once per tree, and its
// results supersede what a stage reports.

// gateSet is one tree's gate results.
type gateSet struct {
	Tree string          `json:"tree"`
	Runs []lsfrt.GateRun `json:"runs"`
}

// failed are the runs that are not green.
func (g gateSet) failed() []lsfrt.GateRun {
	out := []lsfrt.GateRun{}
	for _, r := range g.Runs {
		if r.Status != lsfrt.GateGreen {
			out = append(out, r)
		}
	}
	return out
}

// notRun are the runs the runtime could not complete.
func (g gateSet) notRun() []lsfrt.GateRun {
	out := []lsfrt.GateRun{}
	for _, r := range g.Runs {
		if r.Status == lsfrt.GateNotRun {
			out = append(out, r)
		}
	}
	return out
}

// gateFlight is one tree's run this launch; waiters block on done.
type gateFlight struct {
	done  chan struct{}
	set   gateSet
	fault string
	err   error
}

// gateInput is a gate step's journal input. The commit is left out, so the
// same tree under another commit replays.
type gateInput struct {
	Tree        string            `json:"tree"`
	Declaration *plan.Declaration `json:"declaration"`
}

// gates runs the declared gates on commit's tree, once per tree: a run in
// this launch is shared (concurrent callers wait on it), and a journaled
// run replays. Only a set with every gate green or red is kept; one with a
// gate not run is returned but reruns next time. fault is the stage's
// failure (the tree did not resolve, the checkout failed); err is a stop.
//
// A red set is kept and replays like a green one: an identical tree is
// gated once. A waiter whose own context ends stops; a waiter on a run that
// was not kept (a stop, a fault, a gate not run) runs the gates itself.
func (o *orch) gates(ctx context.Context, commit string) (gateSet, string, error) {
	if o.decl == nil {
		return gateSet{}, "gates: the plan declares none", nil
	}
	tree, fault, err := o.treeOf(ctx, commit)
	if fault != "" || err != nil {
		return gateSet{}, fault, err
	}
	for {
		o.mu.Lock()
		f, ok := o.gateMemo[tree]
		if !ok {
			f = &gateFlight{done: make(chan struct{})}
			o.gateMemo[tree] = f
			o.mu.Unlock()
			o.fly(ctx, f, tree, commit)
			return f.set, f.fault, f.err
		}
		o.mu.Unlock()
		select {
		case <-f.done:
		case <-ctx.Done():
			return gateSet{}, "", &lsfrt.StopError{Reason: lsfrt.StopCancelled}
		}
		if f.kept() {
			return f.set, "", nil
		}
	}
}

// kept reports a finished flight whose set the memo keeps.
func (f *gateFlight) kept() bool {
	return f.err == nil && f.fault == "" && len(f.set.notRun()) == 0
}

// fly runs f's gate step. A flight that is not kept leaves the memo before
// its waiters wake, and a panic is the flight's fault, so a waiter never
// hangs on it.
func (o *orch) fly(ctx context.Context, f *gateFlight, tree, commit string) {
	defer func() {
		if p := recover(); p != nil {
			f.fault = fmt.Sprintf("gates: %v", p)
			o.rt.Log(fmt.Sprintf("gates %s panicked: %v", tree, p))
		}
		if !f.kept() {
			o.mu.Lock()
			delete(o.gateMemo, tree)
			o.mu.Unlock()
		}
		close(f.done)
	}()
	f.set, f.fault, f.err = o.gateStep(ctx, tree, commit)
}

// gatedEarlier is tree's kept result from a run this launch, if any; it
// waits for a run in flight.
func (o *orch) gatedEarlier(ctx context.Context, tree string) (gateSet, bool) {
	o.mu.Lock()
	f, ok := o.gateMemo[tree]
	o.mu.Unlock()
	if !ok {
		return gateSet{}, false
	}
	select {
	case <-f.done:
	case <-ctx.Done():
		return gateSet{}, false
	}
	return f.set, f.kept()
}

// treeOf is commit's tree. With no repository the commit stands for it.
func (o *orch) treeOf(ctx context.Context, commit string) (string, string, error) {
	if commit == "" || strings.HasPrefix(commit, "-") {
		return "", fmt.Sprintf("gates: %q is not a commit", commit), nil
	}
	if o.git == nil {
		return commit, "", nil
	}
	tree, err := gitOps{o.git}.out(ctx, o.repo, "rev-parse", "--verify", "--quiet", commit+"^{tree}")
	switch {
	case err != nil && ctx.Err() != nil:
		return "", "", &lsfrt.StopError{Reason: lsfrt.StopCancelled}
	case err != nil:
		return "", fmt.Sprintf("gates: the tree of %s does not resolve: %v", commit, err), nil
	}
	return tree, "", nil
}

// gateStep is one tree's journaled run, gates:<tree[:12]>.
func (o *orch) gateStep(ctx context.Context, tree, commit string) (gateSet, string, error) {
	specs := make([]lsfrt.GateSpec, len(o.decl.Gates))
	for i, g := range o.decl.Gates {
		specs[i] = lsfrt.GateSpec{Name: g.Name, Command: g.Command}
		if g.Timeout != "" {
			specs[i].Timeout, _ = time.ParseDuration(g.Timeout) // validated at parse
		}
	}
	var unjournaled *gateSet
	short := tree
	if len(short) > 12 {
		short = short[:12]
	}
	// The label is not made launch-unique: a run that was not kept reruns
	// under it, so the journal key is the tree's alone and a relaunch replays
	// whichever run completed.
	raw, _, err := o.runStepAs(ctx, "gates:"+short, gateInput{tree, o.decl}, func(ctx context.Context, label string, _ bool) (json.RawMessage, error) {
		runs, err := o.rt.RunGates(ctx, label, commit, o.decl.Setup, specs)
		if err != nil {
			return nil, err
		}
		set := gateSet{Tree: tree, Runs: runs}
		if len(set.notRun()) > 0 {
			unjournaled = &set
			return nil, errGatesNotJournaled
		}
		return plan.MarshalJS(set, "")
	})
	if isStop(err) {
		return gateSet{}, "", err
	}
	if unjournaled != nil {
		return *unjournaled, "", nil
	}
	var se *stageErr
	if errors.As(err, &se) {
		return gateSet{}, se.reason, nil
	}
	if err != nil {
		return gateSet{}, "", err
	}
	var set gateSet
	if err := json.Unmarshal(raw, &set); err != nil || len(set.Runs) != len(specs) {
		return gateSet{}, fmt.Sprintf("gates: journaled result unreadable: %s", raw), nil
	}
	return set, "", nil
}

// errGatesNotJournaled fails a gate step with a gate not run, so a relaunch
// runs the gates again rather than replay the failure.
var errGatesNotJournaled = errors.New("a gate was not run; not journaled")

// declChanged is the stop for a load whose gate block differs from the one
// the launch started with.
func declChanged(was, now *plan.Declaration, wave *int) *plan.Signal {
	return plan.NewSignal(plan.Signal{Kind: "declared-gates-changed", Source: "structural", Stage: "load", Wave: wave,
		Statement: "the plan's ```yaml " + plan.GatesTag + " block changed during the run: was " + was.Identity() + ", now " + now.Identity() + "; a relaunch takes the new block",
		Referent:  plan.GatesTag})
}

// declaredRun is a code stage's gates as the runtime ran them.
type declaredRun struct {
	gr   gatesRun
	tree string
}

// declaredGates gates a code stage's returned commit (D27). The runtime's
// results supersede the stage's: a declared gate the stage reported with
// another status is a gate-misreported record, and a stage that omitted a
// declared gate or reported it not-run is not stopped. stop is set, with
// the stop among sigs, when the runtime could not complete a gate
// (gate-not-run) or the run failed (stage-fault); err is a runtime stop.
func (o *orch) declaredGates(ctx context.Context, at plan.At, sha string, reported []gateRun) (declaredRun, []*plan.Signal, bool, error) {
	set, fault, err := o.gates(ctx, sha)
	if err != nil {
		return declaredRun{}, nil, false, err
	}
	if fault != "" {
		return declaredRun{}, []*plan.Signal{stageFaultSignal(at, fault)}, true, nil
	}
	sig := func(kind, statement, referent string) *plan.Signal {
		return plan.NewSignal(plan.Signal{Kind: kind, Source: "structural", Stage: at.Stage, Unit: at.Unit, Wave: at.Wave, Statement: statement, Referent: referent})
	}
	if nr := set.notRun(); len(nr) > 0 {
		whys, cmds := []string{}, []string{}
		for _, r := range nr {
			whys = append(whys, r.Command+" ("+r.Summary+")")
			cmds = append(cmds, r.Command)
		}
		return declaredRun{}, []*plan.Signal{sig("gate-not-run", "gate(s) not run by the runtime on "+sha+": "+strings.Join(whys, ", "), strings.Join(cmds, ","))}, true, nil
	}
	sigs := []*plan.Signal{}
	byCommand := map[string]lsfrt.GateRun{}
	for _, r := range set.Runs {
		byCommand[jsTrim(r.Command)] = r
	}
	for _, g := range reported {
		if r, ok := byCommand[jsTrim(g.Command)]; ok && g.Status != r.Status {
			sigs = append(sigs, sig("gate-misreported", fmt.Sprintf("the %s stage reported `%s` %s; the runtime's run on %s is %s: %s", at.Stage, r.Command, g.Status, sha, r.Status, r.Summary), r.Command))
		}
	}
	gates := make([]gateRun, len(set.Runs))
	for i, r := range set.Runs {
		gates[i] = gateRun{Command: r.Command, Status: r.Status, Summary: r.Summary, Tail: r.Tail}
	}
	return declaredRun{gr: gatesRun{by: "The runtime, on a clean checkout,", gates: gates}, tree: set.Tree}, sigs, false, nil
}

// mergeStep runs the merge step. With declared gates, a result with a
// regressed unit is returned but not journaled, so a relaunch reruns the
// step: it adopts the merges committed, and gates the regressed unit's
// merge again: its record replays for an unchanged tree (a moved
// integration branch gives a new one), and reruns once removed or when
// never kept.
func (l *launch) mergeStep(ctx context.Context, label string, in mergeInput) (*ret[mergeResult], string, error) {
	var unjournaled json.RawMessage
	stage := l.mergeStage(in)
	m, fault, err := step[mergeResult](ctx, l.orch, label, in, func(ctx context.Context, label string, interrupted bool) (json.RawMessage, error) {
		v, err := stage(ctx, label, interrupted)
		if err != nil {
			return nil, err
		}
		var r struct {
			Result mergeResult `json:"result"`
		}
		if in.Declared && json.Unmarshal(v, &r) == nil && slices.ContainsFunc(r.Result.Merges, func(x plan.Merge) bool { return x.Status == "regressed" }) {
			unjournaled = v
			return nil, errMergeNotJournaled
		}
		return v, nil
	})
	if err == nil && unjournaled != nil {
		return decode[mergeResult](l.rt, label, unjournaled), "", nil
	}
	return m, fault, err
}

var errMergeNotJournaled = errors.New("a unit regressed; the merge is not journaled")

// declaredMergeSignals rewrites the merge signals of a declared-gates merge
// (D26): a regressed merge failed its gates before anything was committed,
// and one whose gates the runtime could not run stops as gate-not-run.
// plan.SignalsFromMerge keeps the JS statement for the JS's
// commit-then-test merge, which undeclared Go never reports.
func declaredMergeSignals(sigs []*plan.Signal, r mergeResult, declared bool) []*plan.Signal {
	if !declared {
		return sigs
	}
	for _, s := range sigs {
		if s.Kind != "merge-regression" {
			continue
		}
		s.Statement = "merge of " + s.Unit + " failed its gates (" + s.Referent + "); nothing committed"
		if slices.Contains(r.GateNotRun, s.Unit) {
			s.Kind = "gate-not-run"
			s.Statement = "merge of " + s.Unit + ": the runtime could not run its gates (" + s.Referent + "); nothing committed"
		}
	}
	return sigs
}
