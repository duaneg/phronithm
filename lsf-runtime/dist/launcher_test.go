package dist

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// root is a plugin root holding a copy of the launcher, and its env.
type root struct {
	t     *testing.T
	dir   string // the plugin root
	cache string // LSF_RUNTIME_CACHE
	env   []string
}

// newRoot makes a plugin root with the launcher, a dist/ and a fresh
// cache. The environment is the test's, less the launcher's variables.
func newRoot(t *testing.T) *root {
	t.Helper()
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skipf("the launcher runs on amd64 and arm64, not %s", runtime.GOARCH)
	}
	r := &root{t: t, dir: t.TempDir(), cache: t.TempDir()}
	mkdir(t, filepath.Join(r.dir, "lsf-runtime", "dist"))
	copyFile(t, "../lsf-runtime", filepath.Join(r.dir, "lsf-runtime", "lsf-runtime"), 0o755)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "LSF_RUNTIME_") {
			r.env = append(r.env, e)
		}
	}
	r.env = append(r.env, "LSF_RUNTIME_CACHE="+r.cache)
	return r
}

func (r *root) launcher() string { return filepath.Join(r.dir, "lsf-runtime", "lsf-runtime") }

// result is a launcher run's output.
type result struct {
	stdout, stderr string
	code           int
}

func (r *root) run(env []string, args ...string) result {
	r.t.Helper()
	cmd := exec.Command(r.launcher(), args...)
	cmd.Env = append(append([]string{}, r.env...), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		r.t.Fatal(err)
	}
	return result{out.String(), errb.String(), code}
}

// release publishes a stub binary under a file:// URL as tag's asset for
// this machine, and pins it (sha256 of pinned, which defaults to the
// stub's). It returns the run env.
func (r *root) release(tag string, pinned []byte) []string {
	r.t.Helper()
	stub := []byte("#!/bin/sh\necho \"stub $*\"\n")
	if pinned == nil {
		pinned = stub
	}
	pub := r.t.TempDir()
	mkdir(r.t, filepath.Join(pub, tag))
	write(r.t, filepath.Join(pub, tag, "lsf-runtime-linux-"+runtime.GOARCH), stub, 0o644)
	r.pin(fmt.Sprintf("tag %s\ncommit 0123\nsha256 %s %s\n", tag, runtime.GOARCH, hash(pinned)))
	return []string{"LSF_RUNTIME_RELEASE_URL=file://" + pub}
}

func (r *root) pin(text string) {
	write(r.t, filepath.Join(r.dir, "lsf-runtime", "dist", "release"), []byte("# pin\n"+text), 0o644)
}

func (r *root) cached(tag string) string {
	return filepath.Join(r.cache, "release", tag, "lsf-runtime-linux-"+runtime.GOARCH)
}

func TestBinOverride(t *testing.T) {
	r := newRoot(t)
	bin := filepath.Join(t.TempDir(), "bin")
	write(t, bin, []byte("#!/bin/sh\necho \"override $*\"\nexit 3\n"), 0o755)
	res := r.run([]string{"LSF_RUNTIME_BIN=" + bin}, "status", "-name", "a b")
	eq(t, res.code, 3)
	eq(t, res.stdout, "override status -name a b\n")

	write(t, bin, []byte("#!/bin/sh\n"), 0o644)
	res = r.run([]string{"LSF_RUNTIME_BIN=" + bin}, "version")
	eq(t, res.code, 125)
	contains(t, res.stderr, "lsf-runtime launcher: LSF_RUNTIME_BIN="+bin+" is not an executable file")

	res = r.run([]string{"LSF_RUNTIME_BIN=" + filepath.Join(t.TempDir(), "absent")}, "version")
	eq(t, res.code, 125)
	contains(t, res.stderr, "is not an executable file")
	res = r.run([]string{"LSF_RUNTIME_BIN=sh"}, "version")
	eq(t, res.code, 125)
	contains(t, res.stderr, "LSF_RUNTIME_BIN=sh is not a path")
}

// A failed download exits 125 and leaves nothing in the cache.
func TestDownloadFails(t *testing.T) {
	r := newRoot(t)
	r.release("v1", nil)
	res := r.run([]string{"LSF_RUNTIME_RELEASE_URL=file:///nonexistent"}, "version")
	eq(t, res.code, 125)
	contains(t, res.stderr, "lsf-runtime launcher: downloading file:///nonexistent/v1/lsf-runtime-linux-"+runtime.GOARCH+" failed")
	ents, _ := os.ReadDir(filepath.Join(r.cache, "release", "v1"))
	eq(t, len(ents), 0, "nothing left in the cache")
}

// Without LSF_RUNTIME_RELEASE_URL, curl fetches the default https URL with
// only https allowed, redirects included.
func TestDefaultURL(t *testing.T) {
	r := newRoot(t)
	r.release("v1", nil)
	shim := t.TempDir()
	// A curl that reports its arguments and fails.
	write(t, filepath.Join(shim, "curl"), []byte("#!/bin/sh\necho \"curl $*\" >&2\nexit 7\n"), 0o755)
	res := r.run([]string{"PATH=" + shim + ":" + os.Getenv("PATH")}, "version")
	eq(t, res.code, 125)
	contains(t, res.stderr, "--proto =https --proto-redir =https --tlsv1.2")
	contains(t, res.stderr, "https://github.com/duaneg/phronithm/releases/download/v1/lsf-runtime-linux-"+runtime.GOARCH)
}

// The cache defaults to $XDG_CACHE_HOME/lsf-runtime, else
// $HOME/.cache/lsf-runtime; with neither usable it is refused.
func TestCacheDefaults(t *testing.T) {
	r := newRoot(t)
	env := r.release("v1", nil)
	var base []string
	for _, e := range r.env {
		if !strings.HasPrefix(e, "LSF_RUNTIME_CACHE=") && !strings.HasPrefix(e, "XDG_CACHE_HOME=") && !strings.HasPrefix(e, "HOME=") {
			base = append(base, e)
		}
	}
	r.env = base
	xdg, home := t.TempDir(), t.TempDir()
	rel := filepath.Join("lsf-runtime", "release", "v1", "lsf-runtime-linux-"+runtime.GOARCH)

	eq(t, r.run(append(env, "XDG_CACHE_HOME="+xdg, "HOME="+home), "version").code, 0)
	if _, err := os.Stat(filepath.Join(xdg, rel)); err != nil {
		t.Error(err)
	}
	fi, err := os.Stat(filepath.Join(xdg, "lsf-runtime"))
	if err == nil {
		eq(t, fi.Mode().Perm(), os.FileMode(0o700))
	}
	eq(t, r.run(append(env, "XDG_CACHE_HOME=relative", "HOME="+home), "version").code, 0)
	if _, err := os.Stat(filepath.Join(home, ".cache", rel)); err != nil {
		t.Error(err)
	}
	res := r.run(env, "version")
	eq(t, res.code, 125)
	contains(t, res.stderr, "lsf-runtime launcher: HOME is unset or not absolute")
}

// A missing utility is the launcher's failure, exit 125.
func TestMissingTool(t *testing.T) {
	r := newRoot(t)
	env := r.release("v1", nil)
	path := t.TempDir()
	for _, tool := range []string{"sh", "dirname", "uname"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s not installed", tool)
		}
		if err := os.Symlink(p, filepath.Join(path, tool)); err != nil {
			t.Fatal(err)
		}
	}
	res := r.run(append(env, "PATH="+path), "version")
	eq(t, res.code, 125)
	contains(t, res.stderr, "lsf-runtime launcher: awk is not on PATH")
}

// A cache another user could write is refused before its binary runs.
func TestCacheNotPrivate(t *testing.T) {
	r := newRoot(t)
	env := r.release("v1", nil)
	eq(t, r.run(env, "version").code, 0)
	dir := filepath.Dir(r.cached("v1"))
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	res := r.run(env, "version")
	eq(t, res.code, 125)
	eq(t, res.stdout, "")
	contains(t, res.stderr, dir+" is writable by group or others")
}

func TestBadPin(t *testing.T) {
	for name, sum := range map[string]string{
		"upper": strings.Repeat("A", 64),
		"short": strings.Repeat("a", 63),
	} {
		r := newRoot(t)
		r.pin("tag v1\nsha256 " + runtime.GOARCH + " " + sum + "\n")
		res := r.run(nil, "version")
		eq(t, res.code, 125, name)
		contains(t, res.stderr, "bad sha256 "+sum, name)
	}
}

func TestReleaseDownload(t *testing.T) {
	r := newRoot(t)
	env := r.release("v1", nil)
	res := r.run(env, "validate", "x y")
	eq(t, res.code, 0, res.stderr)
	eq(t, res.stdout, "stub validate x y\n")
	fi, err := os.Stat(r.cached("v1"))
	if err != nil {
		t.Fatal(err)
	}
	eq(t, fi.Mode().Perm(), os.FileMode(0o755))
	// Cached: the published copy is no longer read.
	res = r.run([]string{"LSF_RUNTIME_RELEASE_URL=file:///nonexistent"}, "again")
	eq(t, res.stdout, "stub again\n")
	left, _ := filepath.Glob(filepath.Join(r.cache, "release", "v1", ".download.*"))
	eq(t, len(left), 0, "no temp files left")
}

func TestWrongPinRefused(t *testing.T) {
	r := newRoot(t)
	env := r.release("v1", []byte("other"))
	res := r.run(env, "version")
	eq(t, res.code, 125)
	contains(t, res.stderr, "lsf-runtime launcher: ")
	contains(t, res.stderr, "pins "+hash([]byte("other"))+"; refused")
	eq(t, res.stdout, "")
	ents, _ := os.ReadDir(filepath.Join(r.cache, "release", "v1"))
	eq(t, len(ents), 0, "the download is removed")
}

func TestTamperedCacheRefused(t *testing.T) {
	r := newRoot(t)
	env := r.release("v1", nil)
	eq(t, r.run(env, "version").code, 0)
	write(t, r.cached("v1"), []byte("#!/bin/sh\necho tampered\n"), 0o755)
	res := r.run(env, "version")
	eq(t, res.code, 125)
	eq(t, res.stdout, "")
	contains(t, res.stderr, "remove "+r.cached("v1")+" and rerun")
}

func TestNoPin(t *testing.T) {
	for name, pin := range map[string]string{
		"no tag":     "",
		"other arch": "tag v1\nsha256 sparc " + strings.Repeat("0", 64) + "\n",
	} {
		r := newRoot(t)
		r.pin(pin)
		res := r.run(nil, "version")
		eq(t, res.code, 125, name)
		contains(t, res.stderr, "no release pinned for linux-"+runtime.GOARCH, name)
		contains(t, res.stderr, "set LSF_RUNTIME_BIN or run from a working tree with Go", name)
	}
}

func TestBadTag(t *testing.T) {
	r := newRoot(t)
	r.pin("tag ../x\nsha256 " + runtime.GOARCH + " " + strings.Repeat("0", 64) + "\n")
	res := r.run(nil, "version")
	eq(t, res.code, 125)
	contains(t, res.stderr, "bad tag ../x")
}

func TestUnknownMachine(t *testing.T) {
	r := newRoot(t)
	env := r.release("v1", nil)
	shim := t.TempDir()
	write(t, filepath.Join(shim, "uname"), []byte("#!/bin/sh\ncase $1 in -s) echo Linux ;; *) echo mips ;; esac\n"), 0o755)
	res := r.run(append(env, "PATH="+shim+":"+os.Getenv("PATH")), "version")
	eq(t, res.code, 125)
	contains(t, res.stderr, "unsupported machine mips")
}

// A Linux binary is never run on another system.
func TestUnknownSystem(t *testing.T) {
	r := newRoot(t)
	env := r.release("v1", nil)
	shim := t.TempDir()
	write(t, filepath.Join(shim, "uname"), []byte("#!/bin/sh\ncase $1 in -s) echo Darwin ;; *) echo arm64 ;; esac\n"), 0o755)
	res := r.run(append(env, "PATH="+shim+":"+os.Getenv("PATH")), "version")
	eq(t, res.code, 125)
	contains(t, res.stderr, "unsupported system Darwin: Linux only")
	if _, err := os.Stat(r.cached("v1")); err == nil {
		t.Error("the release was downloaded")
	}
}

// A working tree without go on PATH runs the release, saying so.
func TestWorkingTreeWithoutGo(t *testing.T) {
	r := newRoot(t)
	mkdir(t, filepath.Join(r.dir, ".git"))
	env := r.release("v1", nil)
	path := t.TempDir()
	for _, tool := range []string{"sh", "dirname", "uname", "awk", "mktemp", "curl", "sha256sum", "chmod", "mv", "mkdir", "rm", "cmp", "stat", "id"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s not installed", tool)
		}
		if err := os.Symlink(p, filepath.Join(path, tool)); err != nil {
			t.Fatal(err)
		}
	}
	res := r.run(append(env, "PATH="+path), "version")
	eq(t, res.code, 0, res.stderr)
	eq(t, res.stdout, "stub version\n")
	contains(t, res.stderr, "lsf-runtime launcher: "+r.dir+" is a working tree but go is not on PATH; running release v1, not the tree")
}

// treeRoot is a working-tree plugin root whose module is this one, each
// top-level entry symlinked but for those named in copy, which are copied.
func treeRoot(t *testing.T, copy ...string) *root {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the CLI")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	r := newRoot(t)
	mkdir(t, filepath.Join(r.dir, ".git"))
	mod, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	ents, err := os.ReadDir(mod)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		name := e.Name()
		dst := filepath.Join(r.dir, "lsf-runtime", name)
		switch {
		case name == "lsf-runtime" || name == "dist" || name == ".build" || strings.HasPrefix(name, "."):
		case contains1(copy, name):
			if out, err := exec.Command("cp", "-R", filepath.Join(mod, name), dst).CombinedOutput(); err != nil {
				t.Fatalf("cp: %v %s", err, out)
			}
		default:
			if err := os.Symlink(filepath.Join(mod, name), dst); err != nil {
				t.Fatal(err)
			}
		}
	}
	return r
}

// A working tree builds and runs its CLI; an unchanged rebuild leaves the
// binary in place; the CLI's exit codes pass through.
func TestWorkingTree(t *testing.T) {
	r := treeRoot(t)
	r.pin("") // the tree wins without a pin
	res := r.run(nil, "version")
	eq(t, res.code, 0, res.stderr)
	eq(t, res.stdout, "dev unknown\n")
	bin := filepath.Join(r.dir, "lsf-runtime", ".build", "lsf-runtime")
	ino := func() uint64 {
		fi, err := os.Stat(bin)
		if err != nil {
			t.Fatal(err)
		}
		return fi.Sys().(*syscall.Stat_t).Ino
	}
	before := ino()
	eq(t, r.run(nil, "version").code, 0)
	eq(t, ino(), before, "an unchanged build replaced the binary")

	plans, _ := filepath.Abs("../validate/testdata")
	res = r.run(nil, "validate", filepath.Join(plans, "cycle.plan.md"))
	eq(t, res.code, 1, res.stderr)
	contains(t, res.stdout, "cycle [docs-pass]")
	res = r.run(nil, "validate", filepath.Join(t.TempDir(), "absent.md"))
	eq(t, res.code, 2, res.stderr)
	contains(t, res.stderr, "lsf-runtime validate: ")
	left, _ := filepath.Glob(bin + ".*")
	eq(t, len(left), 0, "no temp builds left")
}

// A run's controls run the last build when the tree no longer builds;
// other commands are refused.
func TestWorkingTreeBrokenControls(t *testing.T) {
	r := treeRoot(t, "cli")
	r.pin("")
	eq(t, r.run(nil, "version").code, 0)
	write(t, filepath.Join(r.dir, "lsf-runtime", "cli", "broken.go"), []byte("package main\n\nfunc {\n"), 0o644)
	for _, c := range []string{"status", "watch", "pause", "stop"} {
		res := r.run(nil, c, "-h")
		if res.code == 125 {
			t.Errorf("%s: exit 125: %s", c, res.stderr)
		}
		contains(t, res.stderr, "failed (above); running the last build", c)
	}
	for _, c := range []string{"launch", "resume", "validate", "config", "version"} {
		res := r.run(nil, c, "-h")
		eq(t, res.code, 125, c)
		contains(t, res.stderr, "lsf-runtime launcher: building the working tree "+r.dir+" failed", c)
	}
	// With no earlier build the controls are refused too.
	os.Remove(filepath.Join(r.dir, "lsf-runtime", ".build", "lsf-runtime"))
	res := r.run(nil, "stop", "-h")
	eq(t, res.code, 125)
}

// A changed tree replaces the binary with a new file.
func TestWorkingTreeRebuilds(t *testing.T) {
	r := treeRoot(t, "cli")
	r.pin("")
	eq(t, r.run(nil, "version").stdout, "dev unknown\n")
	bin := filepath.Join(r.dir, "lsf-runtime", ".build", "lsf-runtime")
	fi, err := os.Stat(bin)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(r.dir, "lsf-runtime", "cli", "version.go")
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	write(t, src, []byte(strings.Replace(string(b), `version = "dev"`, `version = "changed"`, 1)), 0o644)
	eq(t, r.run(nil, "version").stdout, "changed unknown\n")
	fi2, err := os.Stat(bin)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(fi, fi2) {
		t.Error("the changed build was written in place")
	}
}

// A failed build exits 125 with the compiler's output and never runs a
// release.
func TestWorkingTreeBuildFails(t *testing.T) {
	r := treeRoot(t, "cli")
	env := r.release("v1", nil)
	write(t, filepath.Join(r.dir, "lsf-runtime", "cli", "broken.go"), []byte("package main\n\nfunc {\n"), 0o644)
	res := r.run(env, "version")
	eq(t, res.code, 125)
	eq(t, res.stdout, "")
	contains(t, res.stderr, "broken.go")
	contains(t, res.stderr, "lsf-runtime launcher: building the working tree "+r.dir+" failed")
	if _, err := os.Stat(r.cached("v1")); err == nil {
		t.Error("the release was downloaded")
	}
}

func hash(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func mkdir(t *testing.T, d string) {
	t.Helper()
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, p string, b []byte, mode os.FileMode) {
	t.Helper()
	os.Remove(p)
	if err := os.WriteFile(p, b, mode); err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, src, dst string, mode os.FileMode) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	write(t, dst, b, mode)
}

func contains1(xs []string, x string) bool {
	for _, y := range xs {
		if x == y {
			return true
		}
	}
	return false
}

func eq[T comparable](t *testing.T, got, want T, msg ...any) {
	t.Helper()
	if got != want {
		t.Errorf("got %v, want %v %v", got, want, fmt.Sprint(msg...))
	}
}

func contains(t *testing.T, s, sub string, msg ...any) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("%q does not contain %q %v", s, sub, fmt.Sprint(msg...))
	}
}
