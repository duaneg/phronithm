package lsfrt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A call's uncommitted changes (design §2.1 of the runtime-signals plan): an
// isolated call reports every path left modified or untracked (not ignored)
// in its worktree; a repo call reports the paths whose status or content
// changed between its first admitted attempt and its return. Paths under the
// state dir are never reported.

// flagUnchecked marks a call whose uncommitted check failed. It is not
// journaled: a resume re-checks.
const flagUnchecked = "uncommitted-unchecked"

// dirt is one porcelain entry: its XY status, the origin of a rename or
// copy, and, for a repo call's regular file, its content hash.
type dirt struct {
	xy, orig, hash string
}

// snapshot is a work tree's dirty paths, relative to its toplevel.
type snapshot map[string]dirt

// snapshot reads dir's work tree. hash adds each regular file's content
// hash, which a repo call needs to see a dirty file changed further.
func (rt *Runtime) snapshot(ctx context.Context, dir string, hash bool) (snapshot, error) {
	gctx, cancel := rt.gitContext(ctx)
	defer cancel()
	top, err := gitStdout(gctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("rev-parse --show-toplevel: %w", err)
	}
	root := strings.TrimSpace(string(top))
	out, err := gitStdout(gctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}
	skip := rt.stateDirIn(root)
	s := snapshot{}
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 4 {
			continue // the trailing empty field
		}
		d, path := dirt{xy: f[:2]}, f[3:]
		if d.xy[0] == 'R' || d.xy[0] == 'C' {
			if i+1 < len(fields) {
				i++
				d.orig = fields[i]
			}
		}
		if skip != "" && (path == skip || strings.HasPrefix(path, skip+"/")) {
			continue
		}
		s[path] = d
	}
	if hash {
		if err := s.hash(gctx, root); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// hash fills in each regular file's content hash, in one git call.
func (s snapshot) hash(ctx context.Context, root string) error {
	var paths []string
	for p := range s {
		if fi, err := os.Lstat(filepath.Join(root, p)); err == nil && fi.Mode().IsRegular() {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	slices.Sort(paths)
	cmd := gitCmd(ctx, root, "hash-object", "--no-filters", "--stdin-paths")
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\n") + "\n")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("hash-object: %w", err)
	}
	hashes := strings.Fields(string(out))
	if len(hashes) != len(paths) {
		return fmt.Errorf("hash-object: %d hashes for %d paths", len(hashes), len(paths))
	}
	for i, p := range paths {
		d := s[p]
		d.hash = hashes[i]
		s[p] = d
	}
	return nil
}

// stateDirIn is the state dir relative to root, "" when it is outside it.
func (rt *Runtime) stateDirIn(root string) string {
	r, err1 := filepath.EvalSymlinks(root)
	sd, err2 := filepath.EvalSymlinks(rt.cfg.StateDir)
	if err1 != nil || err2 != nil {
		return ""
	}
	rel, err := filepath.Rel(r, sd)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return ""
	}
	return filepath.ToSlash(rel)
}

// changed is after's paths not in before, or in it with another status or
// content; with before nil, every path in after. Sorted.
func changed(before, after snapshot) []string {
	out := []string{}
	for p, d := range after {
		if b, ok := before[p]; before == nil || !ok || b != d {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// checkUncommitted sets res.Uncommitted, or the unchecked flag, for a call
// that spawned in c.cwd. A repo call compares with its before-state.
func (rt *Runtime) checkUncommitted(ctx context.Context, c *callState, res *CallResult) {
	if c.cwd == "" || rt.cfg.Repo == "" {
		return
	}
	repo := c.o.Isolation == ""
	if repo && c.beforeErr != nil {
		rt.uncheckedCall(c.o.Label, res, c.beforeErr)
		return
	}
	after, err := rt.snapshot(ctx, c.cwd, repo)
	if err != nil {
		rt.uncheckedCall(c.o.Label, res, err)
		return
	}
	if repo {
		res.Uncommitted = changed(c.before, after)
	} else {
		res.Uncommitted = changed(nil, after)
	}
	if len(res.Uncommitted) > 0 {
		rt.event("uncommitted", map[string]any{"label": c.o.Label, "count": len(res.Uncommitted)})
	}
}

// recheckUncommitted is a replay's check: an isolated call's worktree is
// checked as it stands, when it still exists; a repo call keeps the
// journaled paths that are still dirty.
func (rt *Runtime) recheckUncommitted(ctx context.Context, o Opts, key string, res *CallResult) {
	journaled := res.Uncommitted
	res.Uncommitted = nil
	if len(journaled) == 0 || rt.cfg.Repo == "" {
		return
	}
	dir := rt.cfg.Repo
	if o.Isolation != "" {
		dir = filepath.Join(rt.cfg.StateDir, "wt", key)
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			return // the worktree is gone, and its dirt with it
		}
	}
	now, err := rt.snapshot(ctx, dir, false)
	if err != nil {
		rt.uncheckedCall(o.Label, res, err)
		return
	}
	if o.Isolation != "" {
		res.Uncommitted = changed(nil, now)
		return
	}
	res.Uncommitted = []string{}
	for _, p := range journaled {
		if _, ok := now[p]; ok {
			res.Uncommitted = append(res.Uncommitted, p)
		}
	}
}

func (rt *Runtime) uncheckedCall(label string, res *CallResult, err error) {
	if !slices.Contains(res.Flags, flagUnchecked) {
		res.Flags = append(res.Flags, flagUnchecked)
	}
	rt.event("uncommitted-unchecked", map[string]any{"label": label, "error": err.Error()})
	rt.Log(fmt.Sprintf("%s: uncommitted check failed: %v", label, err))
}
