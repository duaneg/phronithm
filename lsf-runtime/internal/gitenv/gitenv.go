// Package gitenv is the environment the runtime's and the orchestrator's own
// git commands run with.
package gitenv

import (
	"os"
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
