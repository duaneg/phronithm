package orchestrator

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// Cleanup as code (D25): after a wave merges, each merged unit's worktrees
// from this launch go (lsfrt.Runtime.RemoveWorktree), with every untracked,
// ignored and uncommitted file in them, listed, and their branches with
// them: the unit is complete, and a replay of it reads the tips its checks
// journaled (D23), not the branches. First a unit's retains move to the
// pinned store (lsfrt.Runtime.MoveRetained, D33); a failed move keeps all
// the unit's clones.

type cleanupUnitIn struct {
	Unit      string   `json:"unit"`
	Worktrees []string `json:"worktrees"`
	Retains   []string `json:"retains,omitempty"`
}

type cleanupInput struct {
	Wave              int             `json:"wave"`
	IntegrationBranch string          `json:"integrationBranch"`
	Units             []cleanupUnitIn `json:"units"`
	Store             string          `json:"store,omitempty"`
}

// retainFailure is a unit whose retained move failed: its clones are kept,
// and the wave records retained-move-failed.
type retainFailure struct {
	unit, err string
	labels    []string
}

// writerStages rank the stages whose clones a unit's retains may come
// from, oldest first.
var writerStages = map[string]int{"plan": 0, "fix-plan": 1, "implement": 2, "fix-code": 3}

// writerSources are the writer clones among a unit's labels (plan,
// fix-plan:rN, implement, fix-code:rN, each label:<unit>[#n]), oldest
// first: by stage, then round N, then the #n of a stage run again in the
// launch (none before #2).
func writerSources(labels []string) []string {
	type key struct{ stage, round, n int }
	keys := map[string]key{}
	var out []string
	for _, l := range labels {
		base, suffix, _ := strings.Cut(l, "#")
		parts := strings.Split(base, ":")
		rank, ok := writerStages[parts[0]]
		if !ok {
			continue
		}
		k := key{stage: rank}
		if len(parts) == 3 && strings.HasPrefix(parts[1], "r") {
			k.round, _ = strconv.Atoi(parts[1][1:])
		} else if len(parts) != 2 {
			continue
		}
		if suffix != "" {
			k.n, _ = strconv.Atoi(suffix)
		}
		keys[l] = k
		out = append(out, l)
	}
	slices.SortStableFunc(out, func(a, b string) int {
		ka, kb := keys[a], keys[b]
		return cmp.Or(cmp.Compare(ka.stage, kb.stage), cmp.Compare(ka.round, kb.round), cmp.Compare(ka.n, kb.n))
	})
	return out
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
// stop the others; the failures are the step's error once all have run. A
// unit with retains has them moved first; a failed move is an error, keeps
// every one of the unit's labels, and is noted in failed for the wave's
// record.
func (o *orch) cleanupStage(in cleanupInput, failed *[]retainFailure) stageFn {
	return func(ctx context.Context, label string, _ bool) (json.RawMessage, error) {
		res := cleanupResult{Kept: []string{}, Paths: []removedPaths{}}
		var errs []error
		for _, u := range in.Units {
			if len(u.Retains) > 0 {
				m, err := o.rt.MoveRetained(ctx, u.Unit, in.Store, in.IntegrationBranch, u.Retains, writerSources(u.Worktrees))
				if _, stop := lsfrt.IsStop(err); stop {
					return nil, err
				}
				if err != nil {
					o.rt.Log(fmt.Sprintf("%s: %s: retained move failed, its clones kept: %v", label, u.Unit, err))
					errs = append(errs, fmt.Errorf("%s: retained move: %w", u.Unit, err))
					for _, wl := range u.Worktrees {
						res.Kept = append(res.Kept, wl+": retained move failed")
					}
					*failed = append(*failed, retainFailure{unit: u.Unit, err: err.Error(), labels: slices.Clone(u.Worktrees)})
					continue
				}
				res.Retained = append(res.Retained, m)
				moved := []string{}
				for _, p := range m.Moved {
					moved = append(moved, p.Path)
				}
				o.rt.Log(fmt.Sprintf("%s: %s: retained %s → %s; missing %s", label, u.Unit, orNone(moved), orNone([]string{m.Dest}), orNone(m.Missing)))
			}
			for _, wl := range u.Worktrees {
				r, err := o.rt.RemoveWorktree(ctx, wl, in.IntegrationBranch, true)
				// Logged as it happens: a later failure fails the step, and
				// its value with the list would be lost.
				if len(r.Paths) > 0 {
					o.rt.Log(fmt.Sprintf("%s: %s: removed %s", label, wl, strings.Join(r.Paths, ", ")))
				}
				if _, stop := lsfrt.IsStop(err); stop {
					return nil, err
				}
				switch {
				case err != nil:
					errs = append(errs, err)
					o.rt.Log(fmt.Sprintf("%s: %s: %v", label, wl, err))
				case r.Removed:
					res.Removed++
					if len(r.Paths) > 0 {
						res.Paths = append(res.Paths, removedPaths{Label: wl, Paths: r.Paths})
					}
				case r.Kept != "absent" && r.Kept != "branch retained":
					res.Kept = append(res.Kept, wl+": "+r.Kept)
					o.rt.Log(fmt.Sprintf("%s: kept %s: %s", label, wl, r.Kept))
				}
			}
		}
		if g, err := o.gitFor(); err != nil {
			errs = append(errs, err)
		} else if _, err := g.r.run(ctx, o.repo, "worktree", "prune"); err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			return nil, fmt.Errorf("%s: %w", label, errors.Join(errs...))
		}
		return stageValue(res)
	}
}

// orNone joins xs, "none" when empty.
func orNone(xs []string) string {
	xs = slices.DeleteFunc(slices.Clone(xs), func(x string) bool { return x == "" })
	if len(xs) == 0 {
		return "none"
	}
	return strings.Join(xs, ", ")
}
