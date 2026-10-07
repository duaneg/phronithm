package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// Cleanup as code (D25): after a wave merges, each merged unit's worktrees
// from this launch go (lsfrt.Runtime.RemoveWorktree), with every untracked,
// ignored and uncommitted file in them, listed; its completion-report
// branch goes too. Its other branches stay: a resume reads the unit
// branch's tip.

type cleanupUnitIn struct {
	Unit      string   `json:"unit"`
	Worktrees []string `json:"worktrees"`
}

type cleanupInput struct {
	Wave              int             `json:"wave"`
	IntegrationBranch string          `json:"integrationBranch"`
	Units             []cleanupUnitIn `json:"units"`
}

// cleanupRet is the cleanup step's decoded value (step's result names
// shadow ret in wave.go).
type cleanupRet = ret[cleanupResult]

// removedPaths are the paths one worktree's removal deleted.
type removedPaths struct {
	Label string   `json:"label"`
	Paths []string `json:"paths"`
}

// cleanupStage is the cleanup step's body. A removal that fails does not
// stop the others; the failures are the step's error once all have run.
func (o *orch) cleanupStage(in cleanupInput) stageFn {
	return func(ctx context.Context, label string, _ bool) (json.RawMessage, error) {
		g, err := o.gitFor()
		if err != nil {
			return nil, err
		}
		res := cleanupResult{Kept: []string{}, Paths: []removedPaths{}}
		var errs []error
		for _, u := range in.Units {
			for _, wl := range u.Worktrees {
				r, err := o.rt.RemoveWorktree(ctx, wl, in.IntegrationBranch, strings.HasPrefix(wl, "completion-report:"))
				if _, stop := lsfrt.IsStop(err); stop {
					return nil, err
				}
				switch {
				case err != nil:
					errs = append(errs, err)
					res.Kept = append(res.Kept, wl+": "+err.Error())
				case r.Removed:
					res.Removed++
					if len(r.Paths) > 0 {
						res.Paths = append(res.Paths, removedPaths{Label: wl, Paths: r.Paths})
					}
				case r.Kept != "absent":
					res.Kept = append(res.Kept, wl+": "+r.Kept)
				}
			}
		}
		if _, err := g.r.run(ctx, o.repo, "worktree", "prune"); err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			return nil, fmt.Errorf("%s: %w", label, errors.Join(errs...))
		}
		return stageValue(res)
	}
}
