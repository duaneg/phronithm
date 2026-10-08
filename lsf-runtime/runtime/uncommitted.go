package lsfrt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A call's uncommitted changes (docs/design/lsf-runtime.md, "Uncommitted at
// return"): a call in a kept clone reports every path left modified, staged
// or untracked (not ignored) in it. The clone is the call's own, so every
// dirty path is the call's. A discarded clone and an explicit Dir get no
// check.

// FlagUncommittedUnchecked marks a call whose uncommitted check failed; the
// record keeps Unchecked instead, so a replay checks afresh.

// dirtyPaths is dir's dirty paths, relative to its toplevel, sorted: each
// porcelain entry once (a rename or copy by its new path).
func (rt *Runtime) dirtyPaths(ctx context.Context, dir string) ([]string, error) {
	gctx, cancel := rt.gitContext(ctx)
	defer cancel()
	top, err := gitStdout(gctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("rev-parse --show-toplevel: %w", err)
	}
	// No optional locks: the check must not take the index lock from an
	// agent's own git running in parallel.
	out, err := gitStdout(gctx, strings.TrimSpace(string(top)), "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}
	paths := []string{}
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f == "" {
			continue // the trailing empty field
		}
		if len(f) < 4 || f[2] != ' ' {
			return nil, fmt.Errorf("status: unparsable entry %q", f)
		}
		if strings.ContainsAny(f[:2], "RC") { // a rename or copy: its origin follows
			if i+1 >= len(fields) {
				return nil, fmt.Errorf("status: %q has no origin", f)
			}
			i++
		}
		paths = append(paths, f[3:])
	}
	slices.Sort(paths)
	return paths, nil
}

// afterCall is a returned call's bookkeeping (a value or a null; not a
// stop): a kept clone's branch is fetched back and the clone checked; a
// discarded clone is removed. Only a failed fetch-back is an error, a stop.
func (rt *Runtime) afterCall(ctx context.Context, c *callState, res *CallResult) error {
	switch {
	case !c.placed:
	case c.place.kind == CloneKept:
		if err := rt.fetchBack(ctx, c.o.Label, c.place, res); err != nil {
			if ctx.Err() != nil {
				return &StopError{Reason: StopCancelled}
			}
			return rt.setStop(StopRuntimeError, fmt.Sprintf("%s: %v; the clone is kept", c.o.Label, err))
		}
		rt.checkUncommitted(ctx, c.o.Label, c.place.dir, res)
	case c.place.kind == CloneDiscarded:
		rt.discard(c.o.Label, c.place)
	}
	return nil
}

// checkUncommitted sets res.Uncommitted, or the unchecked flag, from dir.
func (rt *Runtime) checkUncommitted(ctx context.Context, label, dir string, res *CallResult) {
	paths, err := rt.dirtyPaths(ctx, dir)
	if err != nil {
		rt.uncheckedCall(ctx, label, res, err)
		return
	}
	res.Uncommitted = paths
	if len(paths) > 0 {
		rt.event("uncommitted", map[string]any{"label": label, "count": len(paths)})
	}
}

// recheckUncommitted is a replay's settling of its clone. A kept clone
// that still exists has its branch fetched back again, so what the
// requester did there since (a commit clearing the paths, a rebase onto a
// moved integration branch) reaches the main repo; a removed clone is
// clean. The uncommitted check is made again when the record has paths or
// its check failed (unchecked), of a kept clone or a worktree (a record
// from before clones) as it stands. Any other record has nothing to check.
func (rt *Runtime) recheckUncommitted(ctx context.Context, o Opts, key string, unchecked bool, res *CallResult) {
	journaled := res.Uncommitted
	res.Uncommitted = nil
	recheck := len(journaled) > 0 || unchecked
	if rt.cfg.Repo == "" || !recheck && res.Clone != CloneKept {
		return
	}
	var dir string
	switch {
	case res.Clone == CloneKept:
		dir = filepath.Join(rt.cfg.StateDir, "clone", key)
	case res.Clone == "" && o.Isolation != "" && o.Dir == "":
		dir = filepath.Join(rt.cfg.StateDir, "wt", key)
	default:
		return
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return // the clone is gone, and its dirt with it
	}
	// Only the call's own clone or worktree: a bare directory left by a
	// crash would resolve to an enclosing repository.
	gctx, cancel := rt.gitContext(ctx)
	top, err := gitStdout(gctx, dir, "rev-parse", "--show-toplevel")
	cancel()
	resolved, _ := filepath.EvalSymlinks(dir)
	if err != nil || strings.TrimSpace(string(top)) != resolved {
		rt.uncheckedCall(ctx, o.Label, res, fmt.Errorf("%s is not the call's own", dir))
		return
	}
	if res.Clone == CloneKept {
		args, err := rt.parseArgs()
		if err != nil {
			rt.uncheckedCall(ctx, o.Label, res, err)
			return
		}
		p := placement{dir: dir, kind: CloneKept, dest: res.Branch, branch: args.IntegrationBranch}
		if o.Isolation != "" {
			p.branch = res.Branch
		}
		if err := rt.fetchBack(ctx, o.Label, p, res); err != nil {
			rt.uncheckedCall(ctx, o.Label, res, err)
			return
		}
	}
	if recheck {
		rt.checkUncommitted(ctx, o.Label, dir, res)
	}
}

// uncheckedCall flags a failed check. A check ended by ctx or a repeated
// signal is not a failure: the stop is reported instead.
func (rt *Runtime) uncheckedCall(ctx context.Context, label string, res *CallResult, err error) {
	if ctx.Err() != nil || rt.stopped() != nil && errors.Is(err, context.Canceled) {
		return
	}
	if !slices.Contains(res.Flags, FlagUncommittedUnchecked) {
		res.Flags = append(res.Flags, FlagUncommittedUnchecked)
	}
	rt.event("uncommitted-unchecked", map[string]any{"label": label, "error": err.Error()})
	rt.Log(fmt.Sprintf("%s: uncommitted check failed: %v", label, err))
}

// checkNull is afterCall for a null return.
func (rt *Runtime) checkNull(ctx context.Context, c *callState, res *CallResult) error {
	return rt.afterCall(ctx, c, res)
}
