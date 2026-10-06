package lsfrt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A call's uncommitted changes (docs/design/lsf-runtime.md, "Uncommitted at
// return"): an isolated call reports every path left modified or untracked
// (not ignored) in its worktree; a repo call reports the paths whose status
// or content changed between its first admitted attempt and its return.
// Paths under the state dir are never reported.

// FlagUncommittedUnchecked marks a call whose uncommitted check failed; the
// record keeps Unchecked instead, so a replay checks afresh.

// dirt is one porcelain entry: its XY status, the origin of a rename or
// copy, and, for a repo call, what the path holds (content hash, symlink
// target, mode).
type dirt struct {
	xy, orig, hash string
}

// snapshot is a work tree's dirty paths, relative to its toplevel.
type snapshot map[string]dirt

// snapshot reads dir's work tree. hash adds each regular file's content
// hash, which a repo call needs to see a dirty file changed further.
func (rt *Runtime) takeSnapshot(ctx context.Context, dir string, hash bool) (snapshot, error) {
	gctx, cancel := rt.gitContext(ctx)
	defer cancel()
	top, err := gitStdout(gctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("rev-parse --show-toplevel: %w", err)
	}
	root := strings.TrimSpace(string(top))
	// No optional locks: the check must not take the index lock from an
	// agent's own git running in parallel.
	out, err := gitStdout(gctx, root, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}
	skip := rt.stateDirIn(root)
	if skip == "" && rt.mayHoldStateDir(root) {
		return nil, fmt.Errorf("cannot place the state dir %s relative to %s", rt.cfg.StateDir, root)
	}
	s := snapshot{}
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f == "" {
			continue // the trailing empty field
		}
		if len(f) < 4 || f[2] != ' ' {
			return nil, fmt.Errorf("status: unparsable entry %q", f)
		}
		d, path := dirt{xy: f[:2]}, f[3:]
		if strings.ContainsAny(d.xy, "RC") { // a rename or copy: its origin follows
			if i+1 >= len(fields) {
				return nil, fmt.Errorf("status: %q has no origin", f)
			}
			i++
			d.orig = fields[i]
		}
		if skip != "" && (path == skip || strings.HasPrefix(path, skip+"/")) {
			continue
		}
		if hash {
			h, err := contentOf(gctx, filepath.Join(root, path))
			if err != nil {
				return nil, err
			}
			d.hash = h
		}
		s[path] = d
	}
	return s, nil
}

// hashWhole is the largest file whose content is hashed; a larger one is
// compared by size and modification time, so a call never reads a huge
// unignored artefact.
const hashWhole = 64 << 20

// contentOf is what a path holds, for comparing a dirty path before and
// after a call: a regular file's mode and content hash (size and
// modification time past hashWhole), a symlink's target, or "missing". A
// file that vanishes mid-read is "missing", not an error; ctx ending is.
func contentOf(ctx context.Context, p string) (string, error) {
	fi, err := os.Lstat(p)
	switch {
	case err != nil:
		return "missing", nil
	case fi.Mode()&os.ModeSymlink != 0:
		t, err := os.Readlink(p)
		if err != nil {
			return "missing", nil
		}
		return "link:" + t, nil
	case !fi.Mode().IsRegular():
		return fi.Mode().String(), nil
	case fi.Size() > hashWhole:
		return fmt.Sprintf("%s:%d@%d", fi.Mode(), fi.Size(), fi.ModTime().UnixNano()), nil
	}
	f, err := os.Open(p)
	if err != nil {
		return "missing", nil
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, ctxReader{ctx, f}); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "missing", nil
	}
	return fi.Mode().String() + ":" + hex.EncodeToString(h.Sum(nil)), nil
}

// ctxReader ends a read when ctx does.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// mayHoldStateDir reports whether the state dir lies lexically under root:
// stateDirIn's "" is then a failure to resolve it, not its absence.
func (rt *Runtime) mayHoldStateDir(root string) bool {
	rel, err := filepath.Rel(root, rt.cfg.StateDir)
	return err == nil && !strings.HasPrefix(rel, "..")
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
// content; with before nil (which holds no path), every path in after.
// Sorted.
func changed(before, after snapshot) []string {
	out := []string{}
	for p, d := range after {
		if b, ok := before[p]; !ok || b != d {
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
		rt.uncheckedCall(ctx, c.o.Label, res, c.beforeErr)
		return
	}
	after, err := rt.takeSnapshot(ctx, c.cwd, repo)
	if err != nil {
		rt.uncheckedCall(ctx, c.o.Label, res, err)
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

// recheckUncommitted is a replay's check, made when the record has paths
// or its check failed (unchecked). An isolated call's worktree is checked
// as it stands, when it still exists. A repo call keeps the journaled paths
// that are still dirty; after a failed check, with no before-state to
// compare, it reports every dirty path.
func (rt *Runtime) recheckUncommitted(ctx context.Context, o Opts, key string, unchecked bool, res *CallResult) {
	journaled := res.Uncommitted
	res.Uncommitted = nil
	if (len(journaled) == 0 && !unchecked) || rt.cfg.Repo == "" {
		return
	}
	dir := rt.cfg.Repo
	if o.Isolation != "" {
		dir = filepath.Join(rt.cfg.StateDir, "wt", key)
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			return // the worktree is gone, and its dirt with it
		}
		// Only the call's own worktree: a bare directory left by a crash
		// would resolve to the main checkout.
		gctx, cancel := rt.gitContext(ctx)
		top, err := gitStdout(gctx, dir, "rev-parse", "--show-toplevel")
		cancel()
		resolved, _ := filepath.EvalSymlinks(dir)
		if err != nil || strings.TrimSpace(string(top)) != resolved {
			rt.uncheckedCall(ctx, o.Label, res, fmt.Errorf("%s is not the call's worktree", dir))
			return
		}
	}
	now, err := rt.takeSnapshot(ctx, dir, false)
	if err != nil {
		rt.uncheckedCall(ctx, o.Label, res, err)
		return
	}
	if o.Isolation != "" || unchecked {
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

// A repo call that returns null is not journaled, so its re-run would take
// the dirt it left into its own before-state. The dirt is kept beside the
// journal (<key>.dirt) until the call is recorded, and a re-run's
// before-state leaves those paths out, so they are reported again.

func (rt *Runtime) dirtPath(key string) string {
	return filepath.Join(rt.cfg.StateDir, "journal", key+".dirt")
}

// keepDirt records a null repo call's paths for its re-run.
func (rt *Runtime) keepDirt(key string, paths []string) {
	if len(paths) == 0 {
		return
	}
	prior := rt.priorDirt(key)
	for _, p := range paths {
		if !slices.Contains(prior, p) {
			prior = append(prior, p)
		}
	}
	b, _ := json.Marshal(prior)
	if err := writeFileAtomic(rt.dirtPath(key), b); err != nil {
		rt.Log(fmt.Sprintf("%s: keeping its uncommitted paths for a re-run failed: %v", key, err))
	}
}

func (rt *Runtime) priorDirt(key string) []string {
	var paths []string
	if b, err := os.ReadFile(rt.dirtPath(key)); err == nil {
		json.Unmarshal(b, &paths)
	}
	return paths
}

// forgetDirt is called once the call is recorded.
func (rt *Runtime) forgetDirt(key string) { os.Remove(rt.dirtPath(key)) }

// checkNull is checkUncommitted for a null return: a repo call keeps its
// paths for its re-run.
func (rt *Runtime) checkNull(ctx context.Context, c *callState, res *CallResult) {
	rt.checkUncommitted(ctx, c, res)
	if c.o.Isolation == "" {
		rt.keepDirt(c.key, res.Uncommitted)
	}
}
