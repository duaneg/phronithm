// Package gitenv is the environment the runtime's and the orchestrator's own
// git commands run with.
package gitenv

import (
	"fmt"
	"os"
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
	return nil
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
