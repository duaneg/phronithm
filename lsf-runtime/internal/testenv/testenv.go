// Package testenv keeps tests' temporary directories out of /tmp and
// /var/tmp, under which the runtime refuses to launch.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Main runs m with TMPDIR pointing at a fresh directory, removed afterwards,
// under the user's cache directory, or under the module's .testtmp when that
// is unavailable or itself under a temp root. A package's TestMain calls it.
func Main(m *testing.M) int {
	dir, err := tempBase()
	if err != nil {
		fmt.Fprintln(os.Stderr, "testenv:", err)
		return 2
	}
	defer os.RemoveAll(dir)
	if err := os.Setenv("TMPDIR", dir); err != nil {
		fmt.Fprintln(os.Stderr, "testenv:", err)
		return 2
	}
	return m.Run()
}

func tempBase() (string, error) {
	var bases []string
	if c, err := os.UserCacheDir(); err == nil {
		bases = append(bases, filepath.Join(c, "lsf-runtime-test"))
	}
	if h, err := os.UserHomeDir(); err == nil {
		bases = append(bases, filepath.Join(h, ".cache", "lsf-runtime-test"))
	}
	if root, err := moduleRoot(); err == nil {
		bases = append(bases, filepath.Join(root, ".testtmp"))
	}
	for _, b := range bases {
		if UnderTempRoot(b) {
			continue
		}
		if err := os.MkdirAll(b, 0o755); err != nil {
			continue
		}
		d, err := os.MkdirTemp(b, "run-")
		if err != nil {
			continue
		}
		if UnderTempRoot(d) { // a symlink into a temp root
			os.RemoveAll(d)
			continue
		}
		return d, nil
	}
	return "", fmt.Errorf("no temp base outside /tmp and /var/tmp among %v", bases)
}

func moduleRoot() (string, error) {
	d, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d, nil
		}
		p := filepath.Dir(d)
		if p == d {
			return "", fmt.Errorf("no go.mod above %s", d)
		}
		d = p
	}
}

// UnderTempRoot mirrors the runtime's launch check, for the test base.
func UnderTempRoot(p string) bool {
	check := func(p string) bool {
		for _, r := range []string{"/tmp", "/var/tmp", "/private/tmp", "/private/var/tmp"} {
			if p == r || strings.HasPrefix(p, r+"/") {
				return true
			}
		}
		return false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return true
	}
	if check(abs) {
		return true
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil && check(r) {
		return true
	}
	return false
}
