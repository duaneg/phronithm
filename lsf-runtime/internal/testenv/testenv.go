// Package testenv keeps tests' temporary directories out of /tmp and
// /var/tmp, under which the runtime refuses to launch.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/internal/tmproot"
)

// Main runs m with TMPDIR, whatever it was, pointing at a fresh directory,
// removed afterwards (a killed test binary leaves it behind),
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
		if tmproot.Under(b) {
			continue
		}
		if err := os.MkdirAll(b, 0o755); err != nil {
			continue
		}
		d, err := os.MkdirTemp(b, "run-")
		if err != nil {
			continue
		}
		if tmproot.Under(d) { // a symlink into a temp root
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
