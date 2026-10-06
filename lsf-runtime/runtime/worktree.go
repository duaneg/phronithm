package lsfrt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cwdFor is a call's working directory: the repo, or for isolation
// "worktree" the call's own worktree on lsf/<plan8>/<key>, created from the
// integration branch and reused on resume. A failure is a runtime-error stop,
// never a silent fallback to the repo.
func (rt *Runtime) cwdFor(ctx context.Context, o Opts, key string) (string, error) {
	switch o.Isolation {
	case "":
		return rt.cfg.Repo, nil
	case "worktree":
	default:
		return "", rt.setStop(StopRuntimeError, fmt.Sprintf("%s: unknown isolation %q", o.Label, o.Isolation))
	}
	var args struct {
		PlanSha           string `json:"planSha"`
		IntegrationBranch string `json:"integrationBranch"`
	}
	if len(rt.cfg.Args) > 0 {
		if err := json.Unmarshal(rt.cfg.Args, &args); err != nil {
			return "", rt.setStop(StopRuntimeError, fmt.Sprintf("%s: args: %v", o.Label, err))
		}
	}
	if len(args.PlanSha) < 8 || args.IntegrationBranch == "" || rt.cfg.Repo == "" {
		return "", rt.setStop(StopRuntimeError, fmt.Sprintf("%s: a worktree needs repo, args.planSha (8+ characters) and args.integrationBranch", o.Label))
	}

	// Concurrent `git worktree add` calls contend on git's locks.
	rt.wtMu.Lock()
	defer rt.wtMu.Unlock()
	if err := rt.boundary(ctx); err != nil { // a stop while queued here
		return "", err
	}
	path := filepath.Join(rt.cfg.StateDir, "wt", key)
	branch := "lsf/" + args.PlanSha[:8] + "/" + refSafe(key)
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		// Reuse only a worktree of this branch: a directory left by a crash
		// mid-add, or one whose metadata was pruned, is not one.
		top, err1 := gitOut(path, "rev-parse", "--show-toplevel")
		head, err2 := gitOut(path, "symbolic-ref", "--short", "HEAD")
		real, _ := filepath.EvalSymlinks(path)
		if err1 != nil || err2 != nil || strings.TrimSpace(string(top)) != real || strings.TrimSpace(string(head)) != branch {
			return "", rt.setStop(StopRuntimeError, fmt.Sprintf("%s: %s exists but is not a worktree on %s; remove it and relaunch", o.Label, path, branch))
		}
		return path, nil
	}
	var out []byte
	var err error
	if gitRun(rt.cfg.Repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch) == nil {
		// The branch outlived its worktree directory: keep its commits.
		gitRun(rt.cfg.Repo, "worktree", "prune")
		out, err = gitOut(rt.cfg.Repo, "worktree", "add", path, branch)
	} else {
		out, err = gitOut(rt.cfg.Repo, "worktree", "add", "-b", branch, path, args.IntegrationBranch)
	}
	if err != nil {
		return "", rt.setStop(StopRuntimeError, fmt.Sprintf("%s: git worktree add: %v: %s", o.Label, err, strings.TrimSpace(string(out))))
	}
	rt.event("worktree", map[string]any{"label": o.Label, "path": path, "branch": branch})
	return path, nil
}

func gitOut(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func gitRun(dir string, args ...string) error {
	_, err := gitOut(dir, args...)
	return err
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
