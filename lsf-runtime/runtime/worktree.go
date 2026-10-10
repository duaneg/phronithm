package lsfrt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/internal/gitenv"
)

// Repo is the main checkout.
func (rt *Runtime) Repo() string { return rt.cfg.Repo }

// Worktree is a code step's own worktree wt/<key> on lsf/<plan8>/<key>,
// created from the integration branch and reused on resume, with its
// branch. Code steps are runtime code, so a worktree serves; agent calls
// get clones (clone.go). A failure is a runtime-error stop.
func (rt *Runtime) Worktree(ctx context.Context, label string) (path, branch string, err error) {
	o, key := Opts{Label: label, Isolation: "worktree"}, journalKey(label)
	args, err := rt.isolationArgs(o)
	if err != nil {
		return "", "", err
	}

	// Concurrent `git worktree add` calls contend on git's locks, so worktree
	// work is serialised; the wait yields to a stop.
	select {
	case rt.wtSem <- struct{}{}:
	case <-rt.stopCh:
		return "", "", rt.stopErr()
	case <-ctx.Done():
		return "", "", &StopError{Reason: StopCancelled}
	}
	defer func() { <-rt.wtSem }()
	if err := rt.boundary(ctx); err != nil {
		return "", "", err
	}
	gctx, cancel := rt.gitContext(ctx)
	defer cancel()
	path = filepath.Join(rt.cfg.StateDir, "wt", key)
	branch = branchName(args, key)
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		// Reuse only a worktree of this branch: a directory left by a crash
		// mid-add, or one whose metadata was pruned, is not one.
		if !isWorktreeOn(gctx, path, branch) {
			return "", "", rt.setStop(StopRuntimeError, fmt.Sprintf("%s: %s exists but is not a worktree on %s; remove it and relaunch", o.Label, path, branch))
		}
		return path, branch, nil
	}
	add := []string{"worktree", "add", "-b", branch, path, args.IntegrationBranch}
	if gitRun(gctx, rt.cfg.Repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch) == nil {
		// The branch outlived its worktree directory: keep its commits.
		add = []string{"worktree", "add", path, branch}
	}
	out, err := gitOut(gctx, rt.cfg.Repo, add...)
	if err != nil && strings.Contains(string(out), "already registered") {
		// A deleted worktree directory is still registered; prune and retry.
		gitRun(gctx, rt.cfg.Repo, "worktree", "prune")
		out, err = gitOut(gctx, rt.cfg.Repo, add...)
	}
	if err != nil {
		return "", "", rt.setStop(StopRuntimeError, fmt.Sprintf("%s: git %s: %v: %s", o.Label, strings.Join(add[:2], " "), err, strings.TrimSpace(string(out))))
	}
	rt.event("worktree", map[string]any{"label": o.Label, "path": path, "branch": branch})
	return path, branch, nil
}

// isWorktreeOn reports whether path is the top of a worktree whose HEAD is
// on branch: the check that keeps worktree reuse and removal off a
// directory that is not the label's own.
func isWorktreeOn(ctx context.Context, path, branch string) bool {
	top, err1 := gitStdout(ctx, path, "rev-parse", "--show-toplevel")
	head, err2 := gitStdout(ctx, path, "symbolic-ref", "--short", "HEAD")
	resolved, _ := filepath.EvalSymlinks(path)
	return err1 == nil && err2 == nil && strings.TrimSpace(string(top)) == resolved && strings.TrimSpace(string(head)) == branch
}

// branchName is the branch of the worktree for journal key key.
func branchName(args worktreeArgs, key string) string {
	return "lsf/" + args.PlanSha[:8] + "/" + refSafe(key)
}

// WorktreeRemoval is what RemoveWorktree did with one label's clone or
// worktree. Paths are what the removal deleted, relative to it: untracked
// and ignored files (a wholly untracked directory as one "dir/" entry) and
// tracked files with uncommitted changes. Kept says why nothing was removed.
type WorktreeRemoval struct {
	Label   string   `json:"label"`
	Path    string   `json:"path"`
	Branch  string   `json:"branch"`
	Removed bool     `json:"removed"`
	Paths   []string `json:"paths,omitempty"`
	Kept    string   `json:"kept,omitempty"`
}

// RemoveWorktree removes label's kept clone (clone/<key>), or a code step's
// worktree (wt/<key>), when its branch in the main repo is merged into
// integration, the branch the caller merged into (its tip an ancestor of
// that branch), with every untracked, ignored and uncommitted file in it,
// and, with deleteBranch, the branch. Kept, with the reason:
//   - both a clone and a worktree for the label;
//   - a directory that is not a worktree on the label's branch, or not a
//     clone on it or on integration (a repo call's clone);
//   - a clone whose HEAD is not the main repo's branch tip: commits not
//     fetched, which only a listing could show;
//   - a branch not merged, and a directory whose branch is gone.
//
// The ancestor check is about commits: it does not protect uncommitted
// work, so the caller must name only labels with no live call and whose
// uncommitted work may go. A stop or cancellation before the removal starts
// is returned; once it starts, one 5-minute bound covers the sequence. A git
// failure is an error, not a stop; after a partial removal the result still
// says what went, and the event is emitted.
func (rt *Runtime) RemoveWorktree(ctx context.Context, label, integration string, deleteBranch bool) (out WorktreeRemoval, err error) {
	o := Opts{Label: label, Isolation: "worktree"}
	out = WorktreeRemoval{Label: label}
	args, err := rt.isolationArgs(o)
	if err != nil {
		return out, err
	}
	if strings.HasPrefix(integration, "-") || integration == "" {
		return out, fmt.Errorf("%s: integration branch %q is not a branch name", label, integration)
	}
	select {
	case rt.wtSem <- struct{}{}:
	case <-rt.stopCh:
		return out, rt.stopErr()
	case <-ctx.Done():
		return out, &StopError{Reason: StopCancelled}
	}
	defer func() { <-rt.wtSem }()
	if err := rt.boundary(ctx); err != nil {
		return out, err
	}
	gctx, cancel := rt.gitContext(ctx)
	defer cancel()
	t, err := rt.removal(gctx, args, label, integration, deleteBranch)
	out.Path, out.Branch, out.Kept = t.path, t.branch, t.kept
	if err != nil || t.kept != "" {
		return out, err
	}
	isClone, tip, ref := t.isClone, t.tip, "refs/heads/"+t.branch
	defer func() {
		if out.Removed {
			ev := map[string]any{"label": label, "path": out.Path, "branch": out.Branch, "paths": len(out.Paths), "clone": isClone}
			if err != nil {
				ev["error"] = err.Error()
			}
			rt.event("worktree-removed", ev)
		}
	}()
	if t.exists {
		git := gitStdout
		if isClone {
			git = cloneStdout
		}
		others, err := git(gctx, out.Path, "ls-files", "-z", "--others", "--directory")
		if err != nil {
			return out, fmt.Errorf("%s: ls-files: %w", label, err)
		}
		dirty, err := git(gctx, out.Path, "diff", "-z", "--name-only", "HEAD")
		if err != nil {
			return out, fmt.Errorf("%s: diff: %w", label, err)
		}
		for _, p := range strings.Split(string(others)+string(dirty), "\x00") {
			if p != "" {
				out.Paths = append(out.Paths, p)
			}
		}
		if isClone {
			if err := os.RemoveAll(out.Path); err != nil {
				return out, fmt.Errorf("%s: remove clone: %w", label, err)
			}
		} else if b, err := gitOut(gctx, rt.cfg.Repo, "worktree", "remove", "--force", out.Path); err != nil {
			return out, fmt.Errorf("%s: worktree remove: %w: %s", label, err, strings.TrimSpace(string(b)))
		}
		out.Removed = true
	}
	if deleteBranch {
		if b, err := gitOut(gctx, rt.cfg.Repo, "update-ref", "-d", ref, tip); err != nil {
			return out, fmt.Errorf("%s: delete %s: %w: %s", label, out.Branch, err, strings.TrimSpace(string(b)))
		}
		out.Removed = true
	}
	return out, nil
}

// removalTarget is what removal found for a label: its directory (a kept
// clone or a code step's worktree), branch and branch tip, and Kept, the
// reason RemoveWorktree would keep it, "" when it would remove it.
type removalTarget struct {
	path, branch, tip string
	isClone, exists   bool
	kept              string
}

// removal runs RemoveWorktree's checks on label without changing anything
// (see RemoveWorktree for the reasons a label is kept).
func (rt *Runtime) removal(gctx context.Context, args worktreeArgs, label, integration string, deleteBranch bool) (t removalTarget, err error) {
	key := journalKey(label)
	isDir := func(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }
	wtPath, clonePath := filepath.Join(rt.cfg.StateDir, "wt", key), filepath.Join(rt.cfg.StateDir, "clone", key)
	t.isClone = isDir(clonePath)
	t.path = wtPath
	if t.isClone {
		t.path = clonePath
	}
	t.branch = branchName(args, key)
	if t.isClone && isDir(wtPath) {
		t.kept = "both a clone and a worktree"
		return t, nil
	}
	ref := "refs/heads/" + t.branch
	tipB, err := gitStdout(gctx, rt.cfg.Repo, "rev-parse", "--verify", "--quiet", ref)
	t.tip = strings.TrimSpace(string(tipB))
	if err != nil && exitStatus(err) != 1 {
		return t, fmt.Errorf("%s: rev-parse %s: %w", label, ref, err)
	}
	t.exists = isDir(t.path)
	switch {
	case !t.exists && t.tip == "":
		t.kept = "absent"
		return t, nil
	case t.exists && t.tip == "":
		t.kept = "branch missing"
		return t, nil
	case !t.exists && !deleteBranch:
		t.kept = "branch retained"
		return t, nil
	}
	merged := gitRun(gctx, rt.cfg.Repo, "merge-base", "--is-ancestor", t.tip, "refs/heads/"+integration)
	if merged != nil {
		if exitStatus(merged) != 1 {
			return t, fmt.Errorf("%s: merge-base: %w", label, merged)
		}
		t.kept = "not merged into " + integration
		return t, nil
	}
	if !t.exists {
		return t, nil
	}
	if t.isClone {
		if isCloneOn(gctx, t.path, t.branch) != nil && isCloneOn(gctx, t.path, integration) != nil {
			t.kept = "not a clone on " + t.branch
			return t, nil
		}
		head, err := cloneStdout(gctx, t.path, "rev-parse", "-q", "--verify", "HEAD")
		if err != nil || strings.TrimSpace(string(head)) != t.tip {
			t.kept = "clone has commits not fetched"
		}
	} else if !isWorktreeOn(gctx, t.path, t.branch) {
		t.kept = "not a worktree on " + t.branch
	}
	return t, nil
}

// exitStatus is a command's exit status, -1 when it did not exit.
func exitStatus(err error) int {
	var x *exec.ExitError
	if errors.As(err, &x) {
		return x.ExitCode()
	}
	return -1
}

// gitTimeout bounds one worktree git command; a hung git (a stale lock, a
// slow hook) must not hold the run.
const gitTimeout = 5 * time.Minute

// gitContext ends at gitTimeout, at a repeated signal, or with ctx.
func (rt *Runtime) gitContext(ctx context.Context) (context.Context, context.CancelFunc) {
	kctx, kcancel := rt.killContext(ctx)
	gctx, cancel := context.WithTimeout(kctx, gitTimeout)
	return gctx, func() { cancel(); kcancel() }
}

// killContext ends at a repeated signal, or with ctx.
func (rt *Runtime) killContext(ctx context.Context) (context.Context, context.CancelFunc) {
	kctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-rt.killCh:
			cancel()
		case <-kctx.Done():
		}
	}()
	return kctx, cancel
}

type worktreeArgs struct {
	PlanSha           string `json:"planSha"`
	IntegrationBranch string `json:"integrationBranch"`
}

// isolationArgs checks, without touching git, what an isolated call needs;
// a dry run checks it too.
func (rt *Runtime) isolationArgs(o Opts) (worktreeArgs, error) {
	var args worktreeArgs
	if o.Isolation != "worktree" {
		return args, rt.setStop(StopRuntimeError, fmt.Sprintf("%s: unknown isolation %q", o.Label, o.Isolation))
	}
	if len(rt.cfg.Args) > 0 {
		if err := json.Unmarshal(rt.cfg.Args, &args); err != nil {
			return args, rt.setStop(StopRuntimeError, fmt.Sprintf("%s: args: %v", o.Label, err))
		}
	}
	if len(args.PlanSha) < 8 || args.IntegrationBranch == "" || strings.HasPrefix(args.IntegrationBranch, "-") || rt.cfg.Repo == "" {
		return args, rt.setStop(StopRuntimeError, fmt.Sprintf("%s: a worktree needs repo, args.planSha (8+ characters) and args.integrationBranch", o.Label))
	}
	return args, nil
}

// gitStdout is a git command's stdout alone, so warnings cannot pollute it.
func gitStdout(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return gitCmd(ctx, dir, args...).Output()
}

func gitOut(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return gitCmd(ctx, dir, args...).CombinedOutput()
}

func gitRun(ctx context.Context, dir string, args ...string) error {
	_, err := gitOut(ctx, dir, args...)
	return err
}

func gitCmd(ctx context.Context, dir string, args ...string) *exec.Cmd {
	return newGitCmd(ctx, dir, false, args)
}

// cloneStdout, cloneOut, cloneRun and cloneCmd are the git helpers for a
// clone the runtime made (gitenv.Env(true)): no global or system config, so
// a driver the clone's .gitattributes selects runs only if its local
// config defines one, and none at all in a clone whose git directory an
// agent redirected (gitenv.CloneGitDir).
func cloneStdout(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return cloneCmd(ctx, dir, args...).Output()
}

func cloneOut(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return cloneCmd(ctx, dir, args...).CombinedOutput()
}

func cloneRun(ctx context.Context, dir string, args ...string) error {
	_, err := cloneOut(ctx, dir, args...)
	return err
}

func cloneCmd(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append(slices.Clone(gitenv.CloneArgs), gitenv.IgnoreSubmodules(args)...)...)
	cmd.Dir = dir
	cmd.Env = append(gitenv.Env(true), gitenv.Ceiling(dir))
	if len(args) == 0 || (args[0] != "clone" && args[0] != "init") { // these two make the clone, from its parent
		cmd.Err = gitenv.CloneGitDir(dir) // Start returns it: git never runs
		if cmd.Err == nil {
			cmd.Err = gitenv.CloneGitlinks(ctx, dir)
		}
	}
	return cmd
}

func newGitCmd(ctx context.Context, dir string, clone bool, args []string) *exec.Cmd {
	// An agent can write its clone's config: no fsmonitor command runs
	// under the runtime's git.
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.fsmonitor=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = gitenv.Env(clone)
	return cmd
}

// refSafe makes a journal key a valid git ref component: no "..", no leading
// "." or "-", no trailing ".lock" (the key's hash suffix keeps it distinct).
func refSafe(key string) string {
	for strings.Contains(key, "..") {
		key = strings.ReplaceAll(key, "..", "_")
	}
	key = strings.TrimLeft(key, ".-")
	return strings.TrimSuffix(key, ".lock")
}
