package gitenv

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestEnv(t *testing.T) {
	t.Setenv("GIT_DIR", "/elsewhere")
	t.Setenv("GIT_CONFIG_PARAMETERS", "'filter.x.clean'='touch m'")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "filter.x.clean")
	t.Setenv("GIT_CONFIG_VALUE_0", "touch m")
	t.Setenv("GIT_CONFIG_GLOBAL", "/home/u/.gitconfig")
	has := func(env []string, prefix string) bool {
		return slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, prefix) })
	}
	last := func(env []string, k string) string {
		v := ""
		for _, kv := range env {
			if s, ok := strings.CutPrefix(kv, k+"="); ok {
				v = s
			}
		}
		return v
	}
	host := Env(false)
	if has(host, "GIT_DIR=") || last(host, "GIT_TERMINAL_PROMPT") != "0" {
		t.Errorf("host env: %v", host)
	}
	if last(host, "GIT_CONFIG_GLOBAL") != "/home/u/.gitconfig" || !has(host, "GIT_CONFIG_PARAMETERS=") {
		t.Error("host env keeps the user's config")
	}
	clone := Env(true)
	for _, k := range []string{"GIT_DIR=", "GIT_CONFIG_PARAMETERS=", "GIT_CONFIG_COUNT=", "GIT_CONFIG_KEY_0=", "GIT_CONFIG_VALUE_0="} {
		if has(clone, k) {
			t.Errorf("clone env has %s", k)
		}
	}
	if last(clone, "GIT_CONFIG_GLOBAL") != "/dev/null" || last(clone, "GIT_CONFIG_NOSYSTEM") != "1" {
		t.Errorf("clone env: %v", clone)
	}
}

func TestCloneGitDir(t *testing.T) {
	d := t.TempDir()
	sub := filepath.Join(d, "a", "b")
	os.MkdirAll(filepath.Join(d, ".git"), 0o755)
	os.MkdirAll(sub, 0o755)
	if err := CloneGitDir(sub); err != nil {
		t.Errorf("plain: %v", err)
	}
	os.WriteFile(filepath.Join(d, ".git", "commondir"), []byte("x\n"), 0o644)
	if err := CloneGitDir(sub); err == nil || !strings.Contains(err.Error(), "commondir exists") {
		t.Errorf("commondir: %v", err)
	}
	f := t.TempDir()
	os.WriteFile(filepath.Join(f, ".git"), []byte("gitdir: /x\n"), 0o644)
	if err := CloneGitDir(f); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("gitfile: %v", err)
	}
}
