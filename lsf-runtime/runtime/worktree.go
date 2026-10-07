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
)

// Repo is the main checkout.
func (rt *Runtime) Repo() string { return rt.cfg.Repo }

// Worktree is a code step's own worktree and branch: the ones an agent call
// with this label and isolation "worktree" gets.
func (rt *Runtime) Worktree(ctx context.Context, label string) (path, branch string, err error) {
	return rt.cwdFor(ctx, Opts{Label: label, Isolation: "worktree"}, journalKey(label))
}

// cwdFor is a call's working directory: the repo, or for isolation
// "worktree" the call's own worktree on lsf/<plan8>/<key>, created from the
// integration branch and reused on resume, with its branch. A failure is a
// runtime-error stop, never a silent fallback to the repo.
func (rt *Runtime) cwdFor(ctx context.Context, o Opts, key string) (path, branch string, err error) {
	if o.Isolation == "" {
		return rt.cfg.Repo, "", nil
	}
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
		top, err1 := gitStdout(gctx, path, "rev-parse", "--show-toplevel")
		head, err2 := gitStdout(gctx, path, "symbolic-ref", "--short", "HEAD")
		resolved, _ := filepath.EvalSymlinks(path)
		if err1 != nil || err2 != nil || strings.TrimSpace(string(top)) != resolved || strings.TrimSpace(string(head)) != branch {
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

// branchName is the branch of the worktree for journal key key.
func branchName(args worktreeArgs, key string) string {
	return "lsf/" + args.PlanSha[:8] + "/" + refSafe(key)
}

// WorktreeRemoval is what RemoveWorktree did with one label's worktree.
// Paths are what the removal deleted, relative to the worktree: untracked
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

// RemoveWorktree removes label's worktree when its branch is merged into
// integration (its tip an ancestor of the branch), with every untracked,
// ignored and uncommitted file in it, and, with deleteBranch, the branch.
// A directory that is not a worktree on the label's branch, or a branch
// not merged, is kept. The ancestor check is about commits: it does not
// protect uncommitted work, which the caller's choice of labels must. A
// git failure is an error, not a stop.
func (rt *Runtime) RemoveWorktree(ctx context.Context, label, integration string, deleteBranch bool) (WorktreeRemoval, error) {
	o := Opts{Label: label, Isolation: "worktree"}
	key := journalKey(label)
	out := WorktreeRemoval{Label: label}
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
	gctx, cancel := rt.gitContext(ctx)
	defer cancel()
	out.Path = filepath.Join(rt.cfg.StateDir, "wt", key)
	out.Branch = branchName(args, key)
	ref := "refs/heads/" + out.Branch
	tipB, err := gitStdout(gctx, rt.cfg.Repo, "rev-parse", "--verify", "--quiet", ref)
	tip := strings.TrimSpace(string(tipB))
	if err != nil && exitStatus(err) != 1 {
		return out, fmt.Errorf("%s: rev-parse %s: %w", label, ref, err)
	}
	fi, statErr := os.Stat(out.Path)
	dirExists := statErr == nil && fi.IsDir()
	switch {
	case !dirExists && tip == "":
		out.Kept = "absent"
		return out, nil
	case dirExists && tip == "":
		out.Kept = "branch missing"
		return out, nil
	}
	merged := gitRun(gctx, rt.cfg.Repo, "merge-base", "--is-ancestor", tip, "refs/heads/"+integration)
	if merged != nil {
		if exitStatus(merged) != 1 {
			return out, fmt.Errorf("%s: merge-base: %w", label, merged)
		}
		out.Kept = "not merged into " + integration
		return out, nil
	}
	if dirExists {
		top, err1 := gitStdout(gctx, out.Path, "rev-parse", "--show-toplevel")
		head, err2 := gitStdout(gctx, out.Path, "symbolic-ref", "--short", "HEAD")
		resolved, _ := filepath.EvalSymlinks(out.Path)
		if err1 != nil || err2 != nil || strings.TrimSpace(string(top)) != resolved || strings.TrimSpace(string(head)) != out.Branch {
			out.Kept = "not a worktree on " + out.Branch
			return out, nil
		}
		others, err := gitStdout(gctx, out.Path, "ls-files", "-z", "--others", "--directory")
		if err != nil {
			return out, fmt.Errorf("%s: ls-files: %w", label, err)
		}
		dirty, err := gitStdout(gctx, out.Path, "diff", "-z", "--name-only", "HEAD")
		if err != nil {
			return out, fmt.Errorf("%s: diff: %w", label, err)
		}
		for _, p := range strings.Split(string(others)+string(dirty), "\x00") {
			if p != "" {
				out.Paths = append(out.Paths, p)
			}
		}
		if b, err := gitOut(gctx, rt.cfg.Repo, "worktree", "remove", "--force", out.Path); err != nil {
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
	if !out.Removed {
		out.Kept = "absent"
	}
	rt.event("worktree-removed", map[string]any{"label": label, "path": out.Path, "branch": out.Branch, "removed": out.Removed, "paths": len(out.Paths)})
	return out, nil
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
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(scrubbedEnv(), "GIT_TERMINAL_PROMPT=0")
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

// scrubbedEnv is the environment without the variables that would point
// git at another repository or index (set when run from inside a hook).
func scrubbedEnv() []string {
	return slices.DeleteFunc(os.Environ(), func(kv string) bool {
		for _, k := range []string{"GIT_DIR=", "GIT_INDEX_FILE=", "GIT_WORK_TREE=", "GIT_COMMON_DIR=", "GIT_OBJECT_DIRECTORY="} {
			if strings.HasPrefix(kv, k) {
				return true
			}
		}
		return false
	})
}
