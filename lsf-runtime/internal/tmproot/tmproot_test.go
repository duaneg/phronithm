package tmproot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnder(t *testing.T) {
	for p, want := range map[string]bool{
		"/tmp": true, "/tmp/a": true, "/var/tmp/a/b": true, "/private/tmp/x": true, "/private/var/tmp": true,
		"/tmpx/a": false, "/var/tmpdir": false, "/srv/tmp": false, "/home/u/tmp": false,
	} {
		if Under(p) != want {
			t.Errorf("Under(%s) = %v", p, !want)
		}
	}
	base, err := os.MkdirTemp(os.Getenv("HOME"), ".tmproot-test-")
	if err != nil {
		t.Skip(err)
	}
	defer os.RemoveAll(base)
	if Under(base) {
		t.Skipf("%s is under a temp root", base)
	}
	if err := os.Symlink("loop", filepath.Join(base, "loop")); err != nil {
		t.Fatal(err)
	}
	if !Under(filepath.Join(base, "loop", "x")) {
		t.Error("a symlink cycle is not refused")
	}
	for name, target := range map[string]string{"live": "/tmp", "dangling": "/tmp/tmproot-no-such-dir/x", "relative": "../../../../../../../../../tmp"} {
		l := filepath.Join(base, name)
		if err := os.Symlink(target, l); err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{l, filepath.Join(l, "not", "made")} {
			if !Under(p) {
				t.Errorf("Under(%s) is false; it links to %s", p, target)
			}
		}
	}
}
