package gitenv

import (
	"context"
	"os"
	"os/exec"
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
	os.MkdirAll(filepath.Join(d, ".git"), 0o755)
	os.MkdirAll(filepath.Join(d, "a"), 0o755)
	if err := CloneGitDir(d); err == nil || !strings.Contains(err.Error(), "no HEAD") {
		t.Errorf("emptied .git: %v", err)
	}
	os.WriteFile(filepath.Join(d, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)
	if err := CloneGitDir(d); err == nil || !strings.Contains(err.Error(), "no objects") {
		t.Errorf("HEAD kept, objects gone: %v", err)
	}
	os.MkdirAll(filepath.Join(d, ".git", "objects"), 0o755)
	os.MkdirAll(filepath.Join(d, ".git", "refs"), 0o755)
	if err := CloneGitDir(d); err != nil {
		t.Errorf("plain: %v", err)
	}
	if err := CloneGitDir(filepath.Join(d, "a")); err == nil {
		t.Error("a directory below the clone's top was accepted")
	}
	os.WriteFile(filepath.Join(d, ".git", "commondir"), []byte("x\n"), 0o644)
	if err := CloneGitDir(d); err == nil || !strings.Contains(err.Error(), "commondir exists") {
		t.Errorf("commondir: %v", err)
	}
	f := t.TempDir()
	os.WriteFile(filepath.Join(f, ".git"), []byte("gitdir: /x\n"), 0o644)
	if err := CloneGitDir(f); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("gitfile: %v", err)
	}
}

func TestIgnoreSubmodules(t *testing.T) {
	for _, tc := range []struct{ in, want []string }{
		{[]string{"--no-optional-locks", "status", "-z"}, []string{"--no-optional-locks", "status", "--ignore-submodules=all", "-z"}},
		{[]string{"diff", "--name-only", "HEAD"}, []string{"diff", "--ignore-submodules=all", "--name-only", "HEAD"}},
		{[]string{"rev-parse", "HEAD"}, []string{"rev-parse", "HEAD"}},
		{[]string{"commit", "-m", "diff"}, []string{"commit", "-m", "diff"}},
		{[]string{"-c", "a=b", "-C", "status", "diff"}, []string{"-c", "a=b", "-C", "status", "diff", "--ignore-submodules=all"}},
	} {
		if got := IgnoreSubmodules(tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("%q: %q", tc.in, got)
		}
	}
}

func TestCloneGitlinks(t *testing.T) {
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	d := t.TempDir()
	git(d, "init", "-q")
	if err := CloneGitlinks(context.Background(), d); err != nil {
		t.Errorf("unborn HEAD: %v", err)
	}
	git(d, "init", "-q", "sub")
	git(filepath.Join(d, "sub"), "commit", "-q", "--allow-empty", "-m", "s")
	git(d, "add", "sub") // staged only
	if err := CloneGitlinks(context.Background(), d); err == nil || !strings.Contains(err.Error(), "gitlink sub") {
		t.Errorf("staged gitlink: %v", err)
	}
	git(d, "commit", "-q", "-m", "c")
	git(d, "rm", "-q", "--cached", "sub") // in HEAD only
	if err := CloneGitlinks(context.Background(), d); err == nil || !strings.Contains(err.Error(), "gitlink sub") {
		t.Errorf("gitlink in HEAD: %v", err)
	}
}
