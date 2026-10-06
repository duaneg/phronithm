package lsfrt

import (
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
func (rt *Runtime) cwdFor(o Opts, key string) (string, error) {
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
	path := filepath.Join(rt.cfg.StateDir, "wt", key)
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return path, nil
	}
	branch := "lsf/" + args.PlanSha[:8] + "/" + key
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
