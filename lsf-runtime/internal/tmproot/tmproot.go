// Package tmproot says whether a path lies under a directory the native Bash
// sandbox leaves writable whatever its allowWrite says.
package tmproot

import (
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
// check). A dangling symlink is judged by its target.
func Under(p string) bool {
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
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if t, err := os.Readlink(p); err == nil {
				if !filepath.IsAbs(t) {
					t = filepath.Join(filepath.Dir(p), t)
				}
				return Under(filepath.Join(t, rest))
			}
		}
		parent := filepath.Dir(p)
		if parent == p {
			return false
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}
