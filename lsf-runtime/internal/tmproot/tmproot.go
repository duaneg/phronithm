// Package tmproot says whether a path lies under a directory the native Bash
// sandbox leaves writable whatever its allowWrite says.
package tmproot

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Roots are /tmp and /var/tmp, with macOS's /private forms.
var Roots = []string{"/tmp", "/var/tmp", "/private/tmp", "/private/var/tmp"}

func in(p string) bool {
	for _, r := range Roots {
		if p == r || strings.HasPrefix(p, r+"/") {
			return true
		}
	}
	return false
}

// Under reports whether p is under a root, lexically or once the symlinks in
// its longest existing prefix are resolved (a state dir is created after the
// check). A dangling symlink is judged by its target. A path that cannot
// be judged (a symlink cycle, an unreadable component) counts as under.
func Under(p string) bool { return under(p, 0) }

func under(p string, depth int) bool {
	if depth > 40 { // a symlink cycle
		return true
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return true
	}
	if in(abs) {
		return true
	}
	for p, rest := abs, ""; ; {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return in(filepath.Join(r, rest))
		}
		fi, err := os.Lstat(p)
		switch {
		case err == nil && fi.Mode()&os.ModeSymlink != 0:
			t, err := os.Readlink(p)
			if err != nil {
				return true
			}
			if !filepath.IsAbs(t) {
				t = filepath.Join(filepath.Dir(p), t)
			}
			return under(filepath.Join(t, rest), depth+1)
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return true
		}
		parent := filepath.Dir(p)
		if parent == p {
			return false
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}
