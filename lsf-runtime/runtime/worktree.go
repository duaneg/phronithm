package lsfrt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// cwdFor is a call's working directory: the repo, or for isolation
// "worktree" the call's own worktree on lsf/<plan8>/<key>, created from the
// integration branch and reused on resume. A failure is a runtime-error stop,
// never a silent fallback to the repo.
func (rt *Runtime) cwdFor(ctx context.Context, o Opts, key string) (string, error) {
	if o.Isolation == "" {
		return rt.cfg.Repo, nil
	}
	args, err := rt.isolationArgs(o)
	if err != nil {
		return "", err
	}

	// Concurrent `git worktree add` calls contend on git's locks, so worktree
	// work is serialised; the wait yields to a stop.
	select {
	case rt.wtSem <- struct{}{}:
	case <-rt.stopCh:
		return "", rt.stopErr()
	case <-ctx.Done():
		return "", &StopError{Reason: StopCancelled}
	}
	defer func() { <-rt.wtSem }()
	if err := rt.boundary(ctx); err != nil {
		return "", err
	}
	gctx, cancel := rt.gitContext(ctx)
	defer cancel()
	path := filepath.Join(rt.cfg.StateDir, "wt", key)
	branch := "lsf/" + args.PlanSha[:8] + "/" + refSafe(key)
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		// Reuse only a worktree of this branch: a directory left by a crash
		// mid-add, or one whose metadata was pruned, is not one.
		top, err1 := gitStdout(gctx, path, "rev-parse", "--show-toplevel")
		head, err2 := gitStdout(gctx, path, "symbolic-ref", "--short", "HEAD")
		resolved, _ := filepath.EvalSymlinks(path)
		if err1 != nil || err2 != nil || strings.TrimSpace(string(top)) != resolved || strings.TrimSpace(string(head)) != branch {
			return "", rt.setStop(StopRuntimeError, fmt.Sprintf("%s: %s exists but is not a worktree on %s; remove it and relaunch", o.Label, path, branch))
		}
		return path, nil
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
		return "", rt.setStop(StopRuntimeError, fmt.Sprintf("%s: git %s: %v: %s", o.Label, strings.Join(add[:2], " "), err, strings.TrimSpace(string(out))))
	}
	rt.event("worktree", map[string]any{"label": o.Label, "path": path, "branch": branch})
	return path, nil
}

// gitTimeout bounds one worktree git command; a hung git (a stale lock, a
// slow hook) must not hold the run.
const gitTimeout = 5 * time.Minute

// gitContext ends at gitTimeout, at a repeated signal, or with ctx.
func (rt *Runtime) gitContext(ctx context.Context) (context.Context, context.CancelFunc) {
	gctx, cancel := context.WithTimeout(ctx, gitTimeout)
	go func() {
		select {
		case <-rt.killCh:
			cancel()
		case <-gctx.Done():
		}
	}()
	return gctx, cancel
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
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
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
