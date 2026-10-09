package lsfrt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// globalFilterConfig points GIT_CONFIG_GLOBAL at a file defining filter x,
// whose clean and smudge each leave .git/filter-ran in the repository they
// run in, plus extra; the repo's committed .gitattributes selects it for
// *.txt (committed first, so no filter ran then).
func globalFilterConfig(t *testing.T, cfg Config, extra string) {
	t.Helper()
	commitFile(t, cfg.Repo, ".gitattributes", "*.txt filter=x\n")
	commitFile(t, cfg.Repo, "a.txt", "a\n")
	global := filepath.Join(t.TempDir(), "gitconfig")
	body := "[filter \"x\"]\n\tclean = \"sh -c 'touch .git/filter-ran; cat'\"\n\tsmudge = \"sh -c 'touch .git/filter-ran; cat'\"\n" + extra
	if err := os.WriteFile(global, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
}

// The runtime's git in a clone reads no global config: a filter defined
// globally and selected by the clone's .gitattributes runs neither at the
// clone's checkout nor in the uncommitted check's status, while the same
// status under host config runs it. Identity, signing and the global
// excludes and attributes files still reach the clone.
func TestCloneIgnoresGlobalConfig(t *testing.T) {
	cfg, _ := testConfig(t)
	excludes, attrs := filepath.Join(t.TempDir(), "ignore"), filepath.Join(t.TempDir(), "attributes")
	os.WriteFile(excludes, []byte("*.env\n"), 0o644)
	os.WriteFile(attrs, []byte("*.bin binary\n"), 0o644)
	globalFilterConfig(t, cfg, "[user]\n\tsigningkey = KEY1\n[gpg]\n\tprogram = /opt/fake-gpg\n[gpg \"ssh\"]\n\tprogram = /opt/fake-ssh\n"+
		"[core]\n\texcludesFile = "+excludes+"\n\tattributesFile = "+attrs+"\n")
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		d, err := rt.MergeClone(ctx, "w1")
		if err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(d, ".git", "filter-ran")
		if exists(marker) {
			t.Error("the filter ran at the clone's checkout")
		}
		for k, want := range map[string]string{"user.signingkey": "KEY1", "gpg.program": "/opt/fake-gpg", "gpg.ssh.program": "/opt/fake-ssh", "user.name": "repo-local"} {
			if got := git(t, d, "config", "--local", "--get", k); got != want {
				t.Errorf("%s: %q, want %q", k, got, want)
			}
		}
		if got := git(t, d, "check-attr", "binary", "x.bin"); got != "x.bin: binary: set" {
			t.Errorf("global attributes: %q", got)
		}
		os.WriteFile(filepath.Join(d, "secret.env"), []byte("x"), 0o644)
		// .gitignore still outranks the global excludes, as in the main repo.
		commitFile(t, d, ".gitignore", "!keep.env\n")
		os.Remove(marker) // the test's own commit there ran under host config
		os.WriteFile(filepath.Join(d, "keep.env"), []byte("x"), 0o644)
		future := time.Now().Add(time.Hour)
		if err := os.Chtimes(filepath.Join(d, "a.txt"), future, future); err != nil {
			t.Fatal(err)
		}
		paths, err := rt.dirtyPaths(ctx, d, true)
		if err != nil || !slices.Equal(paths, []string{"keep.env"}) {
			t.Errorf("isolated status: %q %v", paths, err)
		}
		if exists(marker) {
			t.Error("the filter ran in the isolated status")
		}
		// Control: host config runs the filter on the stat-dirty file.
		if _, err := rt.dirtyPaths(ctx, d, false); err != nil {
			t.Fatal(err)
		}
		if !exists(marker) {
			t.Error("control: the filter did not run under host config, so the isolated check proves nothing")
		}
		return nil, nil
	})
}

// A clone takes no excludes or attributes file when the main repo's
// config names none, and keeps its own info files.
func TestCloneInfoFilesWithoutGlobal(t *testing.T) {
	cfg, _ := testConfig(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	os.WriteFile(filepath.Join(cfg.Repo, ".git", "info", "exclude"), []byte("local.tmp"), 0o644)
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		d, err := rt.MergeClone(ctx, "w1")
		if err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(filepath.Join(d, ".git", "info", "exclude")); string(b) != "local.tmp" {
			t.Errorf("info/exclude: %q", b)
		}
		if exists(filepath.Join(d, ".git", "info", "attributes")) {
			t.Error("info/attributes made from nothing")
		}
		os.WriteFile(filepath.Join(d, "local.tmp"), nil, 0o644)
		if paths, err := rt.dirtyPaths(ctx, d, true); err != nil || slices.Contains(paths, "local.tmp") {
			t.Errorf("%q %v", paths, err)
		}
		return nil, nil
	})
}
