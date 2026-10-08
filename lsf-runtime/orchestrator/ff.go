package orchestrator

import (
	"context"
	"fmt"
	"slices"
	"strings"
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
	dirt, err := g.trackedDirt(ctx, o.repo)
	if err != nil {
		return err
	}
	if len(dirt) > 0 {
		return &ffFault{fmt.Sprintf("the main checkout has uncommitted changes to %q; commit, stash or discard them and relaunch", dirt)}
	}
	return nil
}

// fastForward moves the main checkout's integration branch to target, a
// commit in the main repo. A target already in the branch is nothing to
// apply (applied, or adopted after an interrupted step). A branch that
// moved off target's history, a checkout not on the branch or not clean,
// and a fast-forward git refused are faults.
func (o *orch) fastForward(ctx context.Context, g gitOps, integration, target string) error {
	tip, err := g.out(ctx, o.repo, "rev-parse", "--verify", "--quiet", target+"^{commit}")
	if err != nil {
		return fmt.Errorf("fast-forward: %s does not resolve: %w", target, err)
	}
	if err := o.mainOn(ctx, g, integration); err != nil {
		return err
	}
	head, err := g.head(ctx, o.repo)
	if err != nil {
		return err
	}
	if done, err := g.status(ctx, o.repo, "merge-base", "--is-ancestor", tip, head); err != nil || done {
		return err
	}
	ahead, err := g.status(ctx, o.repo, "merge-base", "--is-ancestor", head, tip)
	if err != nil {
		return err
	}
	if !ahead {
		return &ffFault{fmt.Sprintf("%s moved to %s while %s was made on an earlier tip; rebase or drop those commits, then relaunch", integration, head, tip)}
	}
	if _, err := g.r.run(ctx, o.repo, "merge", "--ff-only", "-q", tip); err != nil {
		return &ffFault{fmt.Sprintf("git refused to fast-forward %s to %s: %v", integration, tip, err)}
	}
	if now, err := g.head(ctx, o.repo); err != nil || now != tip {
		return &ffFault{fmt.Sprintf("after the fast-forward of %s to %s, HEAD is %s (%v)", integration, tip, now, err)}
	}
	o.rt.Log(fmt.Sprintf("fast-forwarded %s from %s to %s", integration, head, tip))
	return nil
}
