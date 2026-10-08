package lsfrt

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeFileAtomic writes data to path so that a reader, or a launch after a
// crash, sees the old content or the new, never a torn file.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp)
		}
	}()
	if err := f.Chmod(0o644); err != nil { // CreateTemp makes 0600; readers include status tools
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return syncDir(dir)
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// newUUID returns a random (version 4) UUID.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand does not fail on supported platforms
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// tempRoots are the directories the native Bash sandbox leaves writable
// whatever its allowWrite says, with macOS's /private forms.
var tempRoots = []string{"/tmp", "/var/tmp", "/private/tmp", "/private/var/tmp"}

// underTempRoot reports whether p is under a temp root, lexically or once
// the symlinks in its longest existing prefix are resolved (a state dir is
// created after the check).
func underTempRoot(p string) bool {
	in := func(p string) bool {
		for _, r := range tempRoots {
			if p == r || strings.HasPrefix(p, r+"/") {
				return true
			}
		}
		return false
	}
	if in(filepath.Clean(p)) {
		return true
	}
	p = filepath.Clean(p)
	for rest := ""; ; {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return in(filepath.Join(r, rest))
		}
		parent := filepath.Dir(p)
		if parent == p {
			return false
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}
