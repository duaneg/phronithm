// Package gitenv is the environment the runtime's and the orchestrator's own
// git commands run with.
package gitenv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Scrubbed is the process environment without the variables that would
// point git at another repository or index.
func Scrubbed() []string {
	return slices.DeleteFunc(os.Environ(), func(kv string) bool {
		for _, k := range []string{"GIT_DIR=", "GIT_INDEX_FILE=", "GIT_WORK_TREE=", "GIT_COMMON_DIR=", "GIT_OBJECT_DIRECTORY="} {
			if strings.HasPrefix(kv, k) {
				return true
			}
		}
		return false
	})
}

// Env is the process environment without the variables that would point
// git at another repository or index (set when run from inside a hook),
// with GIT_TERMINAL_PROMPT=0. With clone set it is for git in a clone the
// runtime made: no global or system config and no config from the
// environment, so a driver a clone's .gitattributes selects (a filter, a
// diff or merge driver) runs only if the clone's local config defines it.
func Env(clone bool) []string {
	env := slices.DeleteFunc(Scrubbed(), func(kv string) bool {
		if clone {
			for _, k := range []string{"GIT_CONFIG=", "GIT_CONFIG_PARAMETERS=", "GIT_CONFIG_COUNT=", "GIT_CONFIG_KEY_", "GIT_CONFIG_VALUE_"} {
				if strings.HasPrefix(kv, k) {
					return true
				}
			}
		}
		return false
	})
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	if clone {
		env = append(env, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	}
	return env
}

// CloneGitDir checks a clone's git directory before the runtime's git runs
// at the clone's top, dir: dir/.git must be a directory holding HEAD and no
// commondir. An agent can write its clone's .git, though not its config,
// hooks or info; a commondir there would point git at a config the agent
// wrote, a .git file at a directory it made, and an emptied .git would
// send git's discovery up to an enclosing repository.
func CloneGitDir(dir string) error {
	g := filepath.Join(filepath.Clean(dir), ".git")
	fi, err := os.Lstat(g)
	switch {
	case err != nil:
		return fmt.Errorf("%s: %w", g, err)
	case !fi.IsDir():
		return fmt.Errorf("%s is not a directory; the runtime's git does not follow it", g)
	}
	if _, err := os.Lstat(filepath.Join(g, "commondir")); err == nil {
		return fmt.Errorf("%s/commondir exists; the runtime's git does not follow it", g)
	}
	if fi, err := os.Lstat(filepath.Join(g, "HEAD")); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("%s has no HEAD file; it is not the clone's git directory", g)
	}
	for _, d := range []string{"objects", "refs"} {
		if fi, err := os.Lstat(filepath.Join(g, d)); err != nil || !fi.IsDir() {
			return fmt.Errorf("%s has no %s directory; it is not the clone's git directory", g, d)
		}
	}
	return nil
}

// CloneGitlinks refuses a clone whose index or HEAD holds a gitlink: a
// repository nested in it, which an agent made (a main repo with
// submodules is refused at launch). A command comparing the work tree
// looks inside a gitlink, a .gitmodules ignore setting overriding the
// config's default, and runs the nested repository's own filter on the
// host. ls-files and ls-tree read only the index and objects.
func CloneGitlinks(ctx context.Context, dir string) error {
	git := func(args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, "git", append(slices.Clone(CloneArgs), args...)...)
		cmd.Dir = dir
		cmd.Env = append(Env(true), Ceiling(dir))
		return cmd
	}
	for _, args := range [][]string{{"ls-files", "--stage", "-z"}, {"ls-tree", "-r", "-z", "HEAD"}} {
		out, err := git(args...).Output()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// An unborn HEAD (a symbolic ref to a branch with no commit) has
			// no tree; any other failure fails closed.
			if args[0] == "ls-tree" && git("symbolic-ref", "-q", "HEAD").Run() == nil && git("rev-parse", "-q", "--verify", "HEAD").Run() != nil {
				continue
			}
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
			}
			return fmt.Errorf("git %s in %s: %w", args[0], dir, err)
		}
		if path, ok := gitlink(out); ok {
			return fmt.Errorf("%s holds a nested repository (gitlink %s), which an agent made (a main repo with one is refused at launch); the runtime's git does not run there", dir, path)
		}
	}
	return nil
}

// RevGitlinks refuses a clone whose commit rev holds a gitlink, reading
// only its tree: for a fetch from the clone, which takes the commit, not
// the index.
func RevGitlinks(ctx context.Context, dir, rev string) error {
	cmd := exec.CommandContext(ctx, "git", append(slices.Clone(CloneArgs), "ls-tree", "-r", "-z", "--end-of-options", rev)...)
	cmd.Dir = dir
	cmd.Env = append(Env(true), Ceiling(dir))
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return fmt.Errorf("git ls-tree %s in %s: %w", rev, dir, err)
	}
	if path, ok := gitlink(out); ok {
		return fmt.Errorf("%s holds a nested repository (gitlink %s at %s), which an agent made (a main repo with one is refused at launch); the runtime does not fetch it", dir, path, rev)
	}
	return nil
}

// gitlink finds a 160000 record in ls-files --stage -z or ls-tree -z
// output, returning its path.
func gitlink(out []byte) (string, bool) {
	for _, rec := range strings.Split(string(out), "\x00") {
		if strings.HasPrefix(rec, "160000 ") {
			_, path, _ := strings.Cut(rec, "\t")
			return path, true
		}
	}
	return "", false
}

// IgnoreSubmodules adds --ignore-submodules=all after a status or diff
// subcommand: on the command line it overrides a .gitmodules ignore
// setting, which the config default does not.
func IgnoreSubmodules(args []string) []string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-c" || a == "-C" {
			i++ // its value
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		if a == "status" || a == "diff" {
			return slices.Concat(args[:i+1], []string{"--ignore-submodules=all"}, args[i+1:])
		}
		return args
	}
	return args
}

// CloneArgs are the options the runtime's git takes in a clone, before the
// command: no fsmonitor command, and no look inside a nested repository an
// agent made (a gitlink), whose own config's filter git would otherwise run
// while computing status or a diff.
var CloneArgs = []string{"-c", "core.fsmonitor=false", "-c", "diff.ignoreSubmodules=all", "-c", "submodule.recurse=false"}

// Ceiling is the environment entry that stops git's discovery above dir.
func Ceiling(dir string) string {
	return "GIT_CEILING_DIRECTORIES=" + filepath.Dir(filepath.Clean(dir))
}
