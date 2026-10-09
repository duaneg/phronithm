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

// CloneGitDir checks the git directory a clone's git would use from dir:
// the first .git at or above dir must be a directory with no commondir.
// An agent can write its clone's .git, though not its config, hooks or
// info; a commondir there would point git at a config the agent wrote,
// and a .git file at a directory it made, whose drivers the runtime's git
// would then run on the host.
func CloneGitDir(dir string) error {
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		fi, err := os.Lstat(filepath.Join(d, ".git"))
		if err == nil {
			if !fi.IsDir() {
				return fmt.Errorf("%s/.git is not a directory; the runtime's git does not follow it", d)
			}
			if _, err := os.Lstat(filepath.Join(d, ".git", "commondir")); err == nil {
				return fmt.Errorf("%s/.git/commondir exists; the runtime's git does not follow it", d)
			}
			return nil
		}
		if d == filepath.Dir(d) {
			return nil
		}
	}
}
