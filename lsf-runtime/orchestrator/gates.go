package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
func (o *orch) gates(ctx context.Context, commit string) (gateSet, string, error) {
	tree, fault, err := o.treeOf(ctx, commit)
	if fault != "" || err != nil {
		return gateSet{}, fault, err
	}
	o.mu.Lock()
	if f, ok := o.gateMemo[tree]; ok {
		o.mu.Unlock()
		<-f.done
		return f.set, f.fault, f.err
	}
	f := &gateFlight{done: make(chan struct{})}
	o.gateMemo[tree] = f
	o.mu.Unlock()
	f.set, f.fault, f.err = o.gateStep(ctx, tree, commit)
	if f.err != nil || f.fault != "" || len(f.set.notRun()) > 0 {
		o.mu.Lock()
		delete(o.gateMemo, tree)
		o.mu.Unlock()
	}
	close(f.done)
	return f.set, f.fault, f.err
}

// gatedEarlier is tree's kept result from a run this launch, if any.
func (o *orch) gatedEarlier(tree string) (gateSet, bool) {
	o.mu.Lock()
	f, ok := o.gateMemo[tree]
	o.mu.Unlock()
	if !ok {
		return gateSet{}, false
	}
	<-f.done
	if f.err != nil || f.fault != "" || len(f.set.notRun()) > 0 {
		return gateSet{}, false
	}
	return f.set, true
}

// treeOf is commit's tree. With no repository the commit stands for it.
func (o *orch) treeOf(ctx context.Context, commit string) (string, string, error) {
	if o.git == nil {
		return commit, "", nil
	}
	if commit == "" || strings.HasPrefix(commit, "-") {
		return "", fmt.Sprintf("gates: %q is not a commit", commit), nil
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
	raw, _, err := o.runStep(ctx, "gates:"+short, gateInput{tree, o.decl}, func(ctx context.Context, label string, _ bool) (json.RawMessage, error) {
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
