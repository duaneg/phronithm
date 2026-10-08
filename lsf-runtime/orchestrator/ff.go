package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

// The main checkout's integration branch moves only by fast-forward (D29):
// to a writer repo call's fetched branch (the ff: step) and to each of the
// merge step's verified commits. The fast-forward updates the checkout, so
// it needs the checkout on the branch and clean: no staged or unstaged
// change to a tracked file. Untracked files are the requester's; git
// refuses the fast-forward over one in its way.

// ffFault is a fast-forward the orchestrator will not make, or git refused.
type ffFault struct{ reason string }

func (e *ffFault) Error() string { return e.reason }

// trackedDirt is dir's tracked paths with staged or unstaged changes,
// sorted.
func (g gitOps) trackedDirt(ctx context.Context, dir string) ([]string, error) {
	out := []string{}
	for _, args := range [][]string{{"diff", "--name-only", "-z"}, {"diff", "--cached", "--name-only", "-z", "HEAD"}} {
		b, err := g.r.run(ctx, dir, args...)
		if err != nil {
			return nil, err
		}
		for _, p := range strings.Split(string(b), "\x00") {
			if p != "" && !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	slices.Sort(out)
	return out, nil
}

// mainOn checks the main checkout is on integration and clean; a fault
// names what is not.
func (o *orch) mainOn(ctx context.Context, g gitOps, integration string) error {
	if err := o.onBranch(ctx, g, integration); err != nil {
		return err
	}
	return o.mainClean(ctx, g)
}

// onBranch checks the main checkout is on integration.
func (o *orch) onBranch(ctx context.Context, g gitOps, integration string) error {
	ref, err := g.headRef(ctx, o.repo)
	if err != nil {
		return err
	}
	if ref != "refs/heads/"+integration {
		on := "a detached HEAD"
		if ref != "" {
			on = strings.TrimPrefix(ref, "refs/heads/")
		}
		return &ffFault{fmt.Sprintf("the main checkout is on %s, not %s", on, integration)}
	}
	return nil
}

// mainClean checks the main checkout has no merge, cherry-pick or revert
// in progress and no change to a tracked file.
func (o *orch) mainClean(ctx context.Context, g gitOps) error {
	for _, op := range []struct{ ref, what string }{{"MERGE_HEAD", "merge"}, {"CHERRY_PICK_HEAD", "cherry-pick"}, {"REVERT_HEAD", "revert"}} {
		busy, err := g.status(ctx, o.repo, "rev-parse", "-q", "--verify", op.ref)
		if err != nil {
			return err
		}
		if busy {
			return &ffFault{"a " + op.what + " is in progress in the main checkout; finish or abort it and relaunch"}
		}
	}
	dirt, err := g.trackedDirt(ctx, o.repo)
	if err != nil {
		return err
	}
	if len(dirt) > 0 {
		return &ffFault{fmt.Sprintf("the main checkout has uncommitted changes to %q; stash or discard them and relaunch (a commit would move the integration branch under the run)", dirt)}
	}
	return nil
}

// fastForward moves the main checkout's integration branch to target, a
// commit in the main repo. A target already in the branch is nothing to
// apply (applied, or adopted after an interrupted step). A branch that
// moved off target's history, a checkout not on the branch or not clean,
// and a fast-forward git refused are faults.
func (o *orch) fastForward(ctx context.Context, g gitOps, integration, target, remedy string) error {
	tip, err := g.out(ctx, o.repo, "rev-parse", "--verify", "--quiet", target+"^{commit}")
	if err != nil {
		return fmt.Errorf("fast-forward: %s does not resolve: %w", target, err)
	}
	if err := o.onBranch(ctx, g, integration); err != nil {
		return err
	}
	head, err := g.head(ctx, o.repo)
	if err != nil {
		return err
	}
	if done, err := g.status(ctx, o.repo, "merge-base", "--is-ancestor", tip, head); err != nil || done {
		return err
	}
	if err := o.mainClean(ctx, g); err != nil {
		return err
	}
	ahead, err := g.status(ctx, o.repo, "merge-base", "--is-ancestor", head, tip)
	if err != nil {
		return err
	}
	if !ahead {
		return &ffFault{fmt.Sprintf("%s moved to %s while %s was made on an earlier tip; %s", integration, head, tip, remedy)}
	}
	// Not cancelled mid-checkout: a half-updated work tree would read as
	// the requester's changes.
	if _, err := g.r.run(context.WithoutCancel(ctx), o.repo, "merge", "--ff-only", "-q", tip); err != nil {
		return &ffFault{fmt.Sprintf("git refused to fast-forward %s to %s: %v", integration, tip, err)}
	}
	if now, err := g.head(ctx, o.repo); err != nil || now != tip {
		return &ffFault{fmt.Sprintf("after the fast-forward of %s to %s, HEAD is %s (%v)", integration, tip, now, err)}
	}
	o.rt.Log(fmt.Sprintf("fast-forwarded %s from %s to %s", integration, head, tip))
	return nil
}

// ffInput is the ff: step's journaled input.
type ffInput struct {
	Branch      string `json:"branch"`
	Integration string `json:"integration"`
	CloneHead   string `json:"cloneHead"`
}

// ffStep fast-forwards the integration branch to a writer repo call's
// fetched branch. A clone whose HEAD was not on the branch fetched (the
// call detached or switched it) is a fault, so work on HEAD is never
// dropped silently; so is a fast-forward fastForward refuses.
func (o *orch) ffStep(branch, cloneHead string) stageFn {
	return func(ctx context.Context, label string, interrupted bool) (json.RawMessage, error) {
		g, err := o.gitFor()
		if err != nil {
			return nil, err
		}
		tip, err := g.out(ctx, o.repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch+"^{commit}")
		if err != nil {
			return nil, fmt.Errorf("%s does not resolve in the main repo: %w", branch, err)
		}
		if cloneHead != "" && cloneHead != tip {
			return nil, fmt.Errorf("the clone's HEAD %s is not %s's tip %s, which was fetched: in the call's clone (under the state dir's clone/), commit the work on %s or drop it, then relaunch", cloneHead, branch, tip, o.ctx.integrationBranch)
		}
		before, err := g.head(ctx, o.repo)
		if err != nil {
			return nil, err
		}
		remedy := "rebase those commits onto " + o.ctx.integrationBranch + " in the call's clone (under the state dir's clone/), or drop them, then relaunch"
		if err := o.fastForward(ctx, g, o.ctx.integrationBranch, "refs/heads/"+branch, remedy); err != nil {
			return nil, err
		}
		after, err := g.head(ctx, o.repo)
		if err != nil {
			return nil, err
		}
		return plan.MarshalJS(map[string]any{"from": before, "to": after, "applied": before != after}, "")
	}
}
