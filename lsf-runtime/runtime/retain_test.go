package lsfrt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// panicAt is the sentinel a test hook panics with: a kill at that point, as
// the move has no cleanup defers.
type panicAt string

// killAt runs f with the hook panicking at the nth hit of point, and
// reports whether it did.
func killAt(t *testing.T, point string, n int, f func()) (hit bool) {
	t.Helper()
	count := 0
	retainHook = func(p string) {
		if p == point {
			if count++; count == n {
				panic(panicAt(p))
			}
		}
	}
	defer func() {
		retainHook = func(string) {}
		if r := recover(); r != nil {
			if _, ok := r.(panicAt); !ok {
				panic(r)
			}
			hit = true
		}
	}()
	f()
	return false
}

// forceEXDEV makes every path move a copy for the test's duration.
func forceEXDEV(t *testing.T) {
	t.Helper()
	renameFn = func(a, b string) error { return &os.LinkError{Op: "rename", Old: a, New: b, Err: syscall.EXDEV} }
	t.Cleanup(func() { renameFn = os.Rename })
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, body := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// snapshot is a tree's entries: each one's type and mode, and a file's
// content or a link's target.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if _, err := os.Lstat(root); errors.Is(err, fs.ErrNotExist) {
		return out
	}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		fi, _ := d.Info()
		v := fi.Mode().String()
		switch {
		case fi.Mode()&fs.ModeSymlink != 0:
			l, _ := os.Readlink(p)
			v += " -> " + l
		case fi.Mode().IsRegular():
			b, _ := os.ReadFile(p)
			v += " " + string(b)
		}
		out[rel] = v
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// retainLaunch runs body in one launch whose repo ignores *.log and out/;
// store is a fresh directory outside the repo and state dir.
func retainLaunch(t *testing.T, body func(ctx context.Context, rt *Runtime, store string)) Config {
	t.Helper()
	t.Setenv("STUB_SCRIPT", "ok")
	cfg, _ := testConfig(t)
	commitFile(t, cfg.Repo, ".gitignore", "*.log\nout/\n")
	store := t.TempDir()
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		body(ctx, rt, store)
		return nil, nil
	})
	return cfg
}

func storeEntries(t *testing.T, store string) []string {
	t.Helper()
	es, err := os.ReadDir(store)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

// Declared paths move from the latest writer clone holding each; an older
// clone's copy stays, and goes with its clone; a path in no clone is
// missing. A rerun returns the recorded result.
func TestMoveRetainedRename(t *testing.T) {
	var res, again RetainedMove
	var removal WorktreeRemoval
	var err, err2 error
	var store, older, newer string
	cfg := retainLaunch(t, func(ctx context.Context, rt *Runtime, st string) {
		store = st
		_, older = mustCall(t, ctx, rt, "plan:U", "worktree")
		_, newer = mustCall(t, ctx, rt, "implement:U", "worktree")
		writeTree(t, older, map[string]string{"out/a.txt": "old", "only-old.log": "o"})
		writeTree(t, newer, map[string]string{"out/a.txt": "new", "out/sub/b.txt": "b", "run.log": "r", "keep.log": "k"})
		res, err = rt.MoveRetained(ctx, "U", store, "main", []string{"run.log", "out", "only-old.log", "nothere"}, []string{"plan:U", "implement:U"})
		again, err2 = rt.MoveRetained(ctx, "U", store, "main", []string{"run.log", "out", "only-old.log", "nothere"}, []string{"plan:U", "implement:U"})
		removal = remove(t, ctx, rt, "plan:U", true)
	})
	if err != nil || err2 != nil {
		t.Fatal(err, err2)
	}
	dest := filepath.Join(store, "U")
	want := RetainedMove{Unit: "U", Dest: dest, Missing: []string{"nothere"}, Moved: []RetainedPath{
		{Path: "only-old.log", From: "plan:U", Method: "rename", Files: 1, Bytes: 1},
		{Path: "out", From: "implement:U", Method: "rename", Files: 2, Bytes: 4},
		{Path: "run.log", From: "implement:U", Method: "rename", Files: 1, Bytes: 1},
	}}
	if fmt.Sprint(res) != fmt.Sprint(want) || fmt.Sprint(again) != fmt.Sprint(want) {
		t.Errorf("\n got %+v\nagain %+v\nwant %+v", res, again, want)
	}
	got := snapshot(t, dest)
	if got["out/a.txt"] != "-rw-r--r-- new" || got["out/sub/b.txt"] != "-rw-r--r-- b" || got["run.log"] != "-rw-r--r-- r" || got["only-old.log"] != "-rw-r--r-- o" || len(got) != 7 {
		t.Errorf("dest: %v", got)
	}
	if exists(filepath.Join(newer, "out")) || !exists(filepath.Join(newer, "keep.log")) {
		t.Error("the newer clone's declared paths are not moved, or its others are")
	}
	if !slices.Contains(removal.Paths, "out/") || exists(older) {
		t.Errorf("the older clone's copy goes with it, listed: %+v", removal)
	}
	if es := storeEntries(t, store); !slices.Equal(es, []string{"U"}) {
		t.Errorf("store: %v", es)
	}
	evs := telemetry(t, cfg)
	if countEvents(evs, "retained-moved") != 1 || countEvents(evs, "retained-refused") != 0 {
		t.Errorf("events: %d moved %d refused", countEvents(evs, "retained-moved"), countEvents(evs, "retained-refused"))
	}
}

// A copy (forced EXDEV) keeps files, nested directories, symlinks, modes and
// mtimes; a declared path named .tmp survives another path's copy.
func TestMoveRetainedCopy(t *testing.T) {
	forceEXDEV(t)
	mtime := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	var res RetainedMove
	var err error
	var store, clone string
	var before map[string]string
	retainLaunch(t, func(ctx context.Context, rt *Runtime, st string) {
		store = st
		_, clone = mustCall(t, ctx, rt, "implement:U", "worktree")
		writeTree(t, clone, map[string]string{"out/a.txt": "a", "out/d/e/f.bin": strings.Repeat("x", 3<<20), "out/run.sh": "#!/bin/sh\n", ".tmp": "tmp"})
		os.Chmod(filepath.Join(clone, "out/run.sh"), 0o755)
		os.Chmod(filepath.Join(clone, "out/a.txt"), 0o640)
		os.Symlink("a.txt", filepath.Join(clone, "out/link"))
		os.Symlink("/nonexistent/target", filepath.Join(clone, "out/d/dangling"))
		os.Chtimes(filepath.Join(clone, "out/a.txt"), mtime, mtime)
		os.Chmod(filepath.Join(clone, "out/d"), 0o750)
		before = snapshot(t, filepath.Join(clone, "out"))
		res, err = rt.MoveRetained(ctx, "U", store, "main", []string{"out", ".tmp"}, []string{"implement:U"})
	})
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(store, "U")
	if got := snapshot(t, filepath.Join(dest, "out")); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Errorf("copy differs\n got %v\nwant %v", got, before)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, ".tmp")); string(b) != "tmp" {
		t.Errorf(".tmp: %q", b)
	}
	if fi, _ := os.Stat(filepath.Join(dest, "out/a.txt")); !fi.ModTime().Equal(mtime) {
		t.Errorf("mtime %v", fi.ModTime())
	}
	if exists(filepath.Join(clone, "out")) || exists(filepath.Join(clone, ".tmp")) {
		t.Error("sources not removed after the copy")
	}
	if len(res.Moved) != 2 || res.Moved[1].Method != "copy" || res.Moved[1].Files != 3 || res.Moved[1].Bytes != 3<<20+1+10 {
		t.Errorf("result: %+v", res)
	}
	if es := storeEntries(t, store); !slices.Equal(es, []string{"U"}) {
		t.Errorf("store: %v", es)
	}
}

// crossDevice returns a fresh directory on another filesystem than ref.
func crossDevice(t *testing.T, ref string) string {
	t.Helper()
	rfi, err := os.Stat(ref)
	if err != nil {
		t.Fatal(err)
	}
	cands := []string{"/dev/shm", "/var/tmp"}
	if d, err := os.UserCacheDir(); err == nil {
		cands = append(cands, d)
	}
	for _, c := range cands {
		fi, err := os.Stat(c)
		if err != nil || !fi.IsDir() || devIno(fi)[0] == devIno(rfi)[0] {
			continue
		}
		d, err := os.MkdirTemp(c, "lsf-retain-test-")
		if err != nil {
			continue
		}
		t.Cleanup(func() { os.RemoveAll(d) })
		t.Logf("cross-device store %s (st_dev %d), source side %s (st_dev %d)", d, devIno(fi)[0], ref, devIno(rfi)[0])
		return d
	}
	if os.Getenv("LSF_LIVE_REQUIRE") == "1" {
		t.Fatal("no directory on another filesystem")
	}
	t.Skip("no directory on another filesystem")
	return ""
}

// A real move across filesystems copies, verifies, then removes the source.
func TestMoveRetainedCrossFilesystem(t *testing.T) {
	var res RetainedMove
	var err error
	var store, clone string
	var sawSource bool
	retainLaunch(t, func(ctx context.Context, rt *Runtime, _ string) {
		store = crossDevice(t, rt.cfg.StateDir)
		_, clone = mustCall(t, ctx, rt, "implement:U", "worktree")
		writeTree(t, clone, map[string]string{"out/a.txt": "a", "out/b/c.txt": "c"})
		retainHook = func(p string) {
			if p == "after-copy-verify" {
				sawSource = exists(filepath.Join(clone, "out/a.txt"))
			}
		}
		defer func() { retainHook = func(string) {} }()
		res, err = rt.MoveRetained(ctx, "U", store, "main", []string{"out"}, []string{"implement:U"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 1 || res.Moved[0].Method != "copy" || !sawSource || exists(filepath.Join(clone, "out")) {
		t.Errorf("result %+v; source at verify %v", res, sawSource)
	}
	if got := snapshot(t, filepath.Join(store, "U", "out")); got["b/c.txt"] != "-rw-r--r-- c" || got["a.txt"] != "-rw-r--r-- a" {
		t.Errorf("dest: %v", got)
	}
}

// A copy that fails verification keeps its source; the move resumes once
// the copy verifies. The source is removed only after the copy verifies.
func TestMoveRetainedVerify(t *testing.T) {
	forceEXDEV(t)
	var err1, err2 error
	var clone, store string
	var afterVerify, afterRemove bool
	retainLaunch(t, func(ctx context.Context, rt *Runtime, st string) {
		store = st
		_, clone = mustCall(t, ctx, rt, "implement:U", "worktree")
		writeTree(t, clone, map[string]string{"out/a.txt": "a"})
		verifyCorrupt = func() bool { return true }
		_, err1 = rt.MoveRetained(ctx, "U", store, "main", []string{"out"}, []string{"implement:U"})
		verifyCorrupt = func() bool { return false }
		if !exists(filepath.Join(clone, "out/a.txt")) {
			t.Error("a failed verification removed the source")
		}
		retainHook = func(p string) {
			switch p {
			case "after-copy-verify":
				afterVerify = exists(filepath.Join(clone, "out/a.txt"))
			case "after-source-remove":
				afterRemove = exists(filepath.Join(clone, "out"))
			}
		}
		defer func() { retainHook = func(string) {} }()
		_, err2 = rt.MoveRetained(ctx, "U", store, "main", []string{"out"}, []string{"implement:U"})
	})
	if err1 == nil || !strings.Contains(err1.Error(), "copy failed verification, source kept") || !strings.Contains(err1.Error(), "staging ") {
		t.Errorf("corrupt copy: %v", err1)
	}
	if err2 != nil || !afterVerify || afterRemove {
		t.Errorf("resume: %v; source at verify %v, after remove %v", err2, afterVerify, afterRemove)
	}
	if b, _ := os.ReadFile(filepath.Join(store, "U", "out", "a.txt")); string(b) != "a" {
		t.Errorf("dest: %q", b)
	}
}

// Refusals change nothing: the clones are byte-identical and the store
// empty.
func TestMoveRetainedRefusals(t *testing.T) {
	outside := t.TempDir()
	writeTree(t, outside, map[string]string{"secret.txt": "s"})
	for _, c := range []struct {
		name, unit string
		paths      []string
		setup      func(t *testing.T, clone string)
		want       string
	}{
		{"special file", "U", []string{"out"}, func(t *testing.T, clone string) {
			if err := syscall.Mkfifo(filepath.Join(clone, "out", "fifo"), 0o644); err != nil {
				t.Skip(err)
			}
		}, "is a special file"},
		{"symlinked path", "U", []string{"out/link"}, func(t *testing.T, clone string) {
			os.Symlink(outside, filepath.Join(clone, "out", "link"))
		}, "out/link is a symlink"},
		{"symlinked parent", "U", []string{"via/secret.txt"}, func(t *testing.T, clone string) {
			os.Symlink(outside, filepath.Join(clone, "via"))
		}, "via is a symlink"},
		{"unsafe unit", "../U", []string{"out"}, nil, `unit id "../U" cannot name a store directory`},
		{"colon unit", "U:1", []string{"out"}, nil, "cannot name a store directory"},
		{"no space", "U", []string{"out"}, func(t *testing.T, clone string) {
			deviceOf = func(fi fs.FileInfo) uint64 {
				if strings.Contains(fi.Name(), "lsf-retain") || fi.Name() == "out" {
					return 1
				}
				return 2
			}
			statfsFn = func(string) (uint64, error) { return 3, nil }
		}, "needs more than its 3 free bytes"},
	} {
		t.Run(c.name, func(t *testing.T) {
			origDev, origStatfs := deviceOf, statfsFn
			t.Cleanup(func() { deviceOf, statfsFn = origDev, origStatfs })
			var err error
			var before, after map[string]string
			var store string
			retainLaunch(t, func(ctx context.Context, rt *Runtime, st string) {
				store = st
				_, clone := mustCall(t, ctx, rt, "implement:"+c.unit, "worktree")
				writeTree(t, clone, map[string]string{"out/a.txt": "aaaa"})
				if c.setup != nil {
					c.setup(t, clone)
				}
				before = snapshot(t, clone)
				_, err = rt.MoveRetained(ctx, c.unit, store, "main", c.paths, []string{"implement:" + c.unit})
				after = snapshot(t, clone)
			})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("want %q, got %v", c.want, err)
			}
			if fmt.Sprint(before) != fmt.Sprint(after) {
				t.Errorf("clone changed\nbefore %v\n after %v", before, after)
			}
			if es := storeEntries(t, store); len(es) != 0 {
				t.Errorf("store: %v", es)
			}
		})
	}
}

// The latest holder decides: one cleanup keeps is an error, with no
// fallback to an older copy. A destination that exists stops the move. A
// rename refused for permissions is an error, not a copy.
func TestMoveRetainedSources(t *testing.T) {
	var errKept, errDest, errPerm error
	var older, newer, store string
	var before map[string]string
	var copied bool
	retainLaunch(t, func(ctx context.Context, rt *Runtime, st string) {
		store = st
		_, older = mustCall(t, ctx, rt, "implement:K", "worktree")
		_, newer = mustCall(t, ctx, rt, "fix-code:r1:K", "worktree")
		writeTree(t, older, map[string]string{"out/a.txt": "old"})
		writeTree(t, newer, map[string]string{"out/a.txt": "new"})
		git(t, newer, "commit", "-q", "--allow-empty", "-m", "after return")
		before = snapshot(t, older)
		_, errKept = rt.MoveRetained(ctx, "K", store, "main", []string{"out"}, []string{"implement:K", "fix-code:r1:K"})
		if fmt.Sprint(snapshot(t, older)) != fmt.Sprint(before) || !exists(filepath.Join(newer, "out/a.txt")) {
			t.Error("a kept latest holder moved something")
		}

		_, c := mustCall(t, ctx, rt, "implement:D", "worktree")
		writeTree(t, c, map[string]string{"out/a.txt": "a"})
		os.Mkdir(filepath.Join(store, "D"), 0o755)
		_, errDest = rt.MoveRetained(ctx, "D", store, "main", []string{"out"}, []string{"implement:D"})
		if !exists(filepath.Join(c, "out/a.txt")) {
			t.Error("an existing destination moved the source")
		}

		_, c = mustCall(t, ctx, rt, "implement:P", "worktree")
		writeTree(t, c, map[string]string{"ro/data.log": "d"})
		os.Chmod(filepath.Join(c, "ro"), 0o555)
		defer os.Chmod(filepath.Join(c, "ro"), 0o755)
		renameFn = func(a, b string) error {
			err := os.Rename(a, b)
			copied = copied || errors.Is(err, syscall.EXDEV)
			return err
		}
		defer func() { renameFn = os.Rename }()
		_, errPerm = rt.MoveRetained(ctx, "P", store, "main", []string{"ro/data.log"}, []string{"implement:P"})
	})
	if errKept == nil || !strings.Contains(errKept.Error(), "the latest copy of out is in fix-code:r1:K") || !strings.Contains(errKept.Error(), "clone has commits not fetched") {
		t.Errorf("kept holder: %v", errKept)
	}
	if errDest == nil || !strings.Contains(errDest.Error(), "D: destination "+filepath.Join(store, "D")+" exists") {
		t.Errorf("existing dest: %v", errDest)
	}
	if os.Geteuid() != 0 && (errPerm == nil || !errors.Is(errPerm, syscall.EACCES) || copied) {
		t.Errorf("read-only source: %v (copied %v)", errPerm, copied)
	}
}

// All missing: no destination, a done result that a rerun returns.
func TestMoveRetainedAllMissing(t *testing.T) {
	var r1, r2 RetainedMove
	var err1, err2 error
	var store string
	retainLaunch(t, func(ctx context.Context, rt *Runtime, st string) {
		store = st
		mustCall(t, ctx, rt, "implement:M", "worktree")
		r1, err1 = rt.MoveRetained(ctx, "M", store, "main", []string{"a", "b"}, []string{"implement:M"})
		r2, err2 = rt.MoveRetained(ctx, "M", store, "main", []string{"a", "b"}, []string{"implement:M"})
	})
	if err1 != nil || err2 != nil || r1.Dest != "" || !slices.Equal(r1.Missing, []string{"a", "b"}) || len(r1.Moved) != 0 || fmt.Sprint(r1) != fmt.Sprint(r2) {
		t.Errorf("%+v %v / %+v %v", r1, err1, r2, err2)
	}
	if es := storeEntries(t, store); len(es) != 0 {
		t.Errorf("store: %v", es)
	}
}

// A cancelled context mid-copy is a cancellation stop; the next call
// completes the move.
func TestMoveRetainedCancelled(t *testing.T) {
	forceEXDEV(t)
	var err1, err2 error
	var store string
	retainLaunch(t, func(ctx context.Context, rt *Runtime, st string) {
		store = st
		_, c := mustCall(t, ctx, rt, "implement:C", "worktree")
		writeTree(t, c, map[string]string{"out/a.txt": "a", "out/b.txt": "b"})
		cctx, cancel := context.WithCancel(ctx)
		retainHook = func(p string) {
			if p == "mid-copy" {
				cancel()
			}
		}
		_, err1 = rt.MoveRetained(cctx, "C", store, "main", []string{"out"}, []string{"implement:C"})
		retainHook = func(string) {}
		_, err2 = rt.MoveRetained(ctx, "C", store, "main", []string{"out"}, []string{"implement:C"})
	})
	if r, ok := IsStop(err1); !ok || r != StopCancelled {
		t.Errorf("cancelled: %v", err1)
	}
	if got := snapshot(t, filepath.Join(store, "C", "out")); err2 != nil || len(got) != 3 {
		t.Errorf("resume: %v %v", err2, got)
	}
}

// A kill at any checkpoint, then a rerun, completes the move: every path
// at the destination once, with its content, and nothing left in the store.
func TestMoveRetainedKillResume(t *testing.T) {
	type kase struct {
		point string
		n     int
		copy  bool
	}
	cases := []kase{
		{"after-moving-record", 1, false}, {"after-path-rename", 1, false}, {"after-path-rename", 2, false},
		{"after-path-record", 1, false}, {"after-renaming-record", 1, false}, {"after-staging-rename", 1, false},
		{"before-done-record", 1, false},
		{"mid-copy", 1, true}, {"mid-copy", 3, true}, {"after-copy-verify", 1, true}, {"after-final-name", 1, true},
		{"after-path-record", 1, true}, {"after-path-record", 2, true}, {"after-source-remove", 1, true},
		{"after-renaming-record", 1, true}, {"after-staging-rename", 1, true}, {"before-done-record", 1, true},
	}
	files := map[string]string{"out/a.txt": "a", "out/b/c.txt": "c", "res.log": "r"}
	for i, c := range cases {
		name := fmt.Sprintf("%s#%d", c.point, c.n)
		if c.copy {
			name = "copy/" + name
		}
		t.Run(name, func(t *testing.T) {
			if c.copy {
				forceEXDEV(t)
			}
			unit := fmt.Sprintf("K%d", i)
			var hit bool
			var res RetainedMove
			var err error
			var store string
			var firstFrom string
			retainLaunch(t, func(ctx context.Context, rt *Runtime, st string) {
				store = st
				_, clone := mustCall(t, ctx, rt, "implement:"+unit, "worktree")
				writeTree(t, clone, files)
				hit = killAt(t, c.point, c.n, func() {
					rt.MoveRetained(ctx, unit, store, "main", []string{"out", "res.log"}, []string{"implement:" + unit})
				})
				if b, err := os.ReadFile(rt.retainRecordPath(unit)); err == nil {
					var rec retainRecord
					json.Unmarshal(b, &rec)
					if len(rec.Moved) > 0 {
						firstFrom = rec.Moved[0].Path + "@" + rec.Moved[0].From
					}
				}
				res, err = rt.MoveRetained(ctx, unit, store, "main", []string{"out", "res.log"}, []string{"implement:" + unit})
			})
			if !hit {
				t.Fatalf("checkpoint %s #%d not reached", c.point, c.n)
			}
			if err != nil {
				t.Fatal(err)
			}
			got := snapshot(t, filepath.Join(store, unit))
			if got["out/a.txt"] != "-rw-r--r-- a" || got["out/b/c.txt"] != "-rw-r--r-- c" || got["res.log"] != "-rw-r--r-- r" || len(got) != 6 {
				t.Errorf("dest: %v", got)
			}
			if es := storeEntries(t, store); !slices.Equal(es, []string{unit}) {
				t.Errorf("store: %v", es)
			}
			if len(res.Moved) != 2 {
				t.Errorf("result: %+v", res)
			}
			// Progress recorded before the kill is kept.
			if firstFrom != "" && res.Moved[0].Path+"@"+res.Moved[0].From != firstFrom {
				t.Errorf("recorded progress lost: %s, result %+v", firstFrom, res.Moved)
			}
		})
	}
}

// Damage between attempts: a corrupt record stops the run; a staging
// directory removed after a recorded move is an error; a done record whose
// destination is gone returns its result.
func TestMoveRetainedDamage(t *testing.T) {
	var errStaging, errDone error
	var done RetainedMove
	cfg := retainLaunch(t, func(ctx context.Context, rt *Runtime, store string) {
		_, c := mustCall(t, ctx, rt, "implement:S", "worktree")
		writeTree(t, c, map[string]string{"a.log": "a", "b.log": "b"})
		killAt(t, "after-path-record", 1, func() {
			rt.MoveRetained(ctx, "S", store, "main", []string{"a.log", "b.log"}, []string{"implement:S"})
		})
		staging, _ := retainDirs(store, rt.cfg.StateDir, "S")
		os.Rename(staging, staging+".moved")
		_, errStaging = rt.MoveRetained(ctx, "S", store, "main", []string{"a.log", "b.log"}, []string{"implement:S"})

		_, c = mustCall(t, ctx, rt, "implement:G", "worktree")
		writeTree(t, c, map[string]string{"a.log": "a"})
		if _, err := rt.MoveRetained(ctx, "G", store, "main", []string{"a.log"}, []string{"implement:G"}); err != nil {
			t.Fatal(err)
		}
		os.Rename(filepath.Join(store, "G"), filepath.Join(store, "G.archived"))
		done, errDone = rt.MoveRetained(ctx, "G", store, "main", []string{"a.log"}, []string{"implement:G"})
	})
	if errStaging == nil || !strings.Contains(errStaging.Error(), "a.log was moved to the staging directory but is not there now") {
		t.Errorf("staging gone: %v", errStaging)
	}
	if errDone != nil || len(done.Moved) != 1 {
		t.Errorf("done, dest gone: %+v %v", done, errDone)
	}
	if b, _ := os.ReadFile(filepath.Join(cfg.StateDir, "log.txt")); !strings.Contains(string(b), "G: retained paths were moved to") {
		t.Error("no warning for a moved destination")
	}

	var errCorrupt error
	retainLaunch(t, func(ctx context.Context, rt *Runtime, store string) {
		p := rt.retainRecordPath("X")
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("{not json"), 0o644)
		_, errCorrupt = rt.MoveRetained(ctx, "X", store, "main", []string{"a"}, nil)
	})
	if r, ok := IsStop(errCorrupt); !ok || r != StopRuntimeError {
		t.Errorf("corrupt record: %v", errCorrupt)
	}
}

// A real SIGKILL mid-copy: the resume in a new launch completes the move.
func TestMoveRetainedSIGKILL(t *testing.T) {
	t.Setenv("STUB_SCRIPT", "ok")
	cfg, _ := testConfig(t)
	commitFile(t, cfg.Repo, ".gitignore", "out/\n")
	store := t.TempDir()
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		_, c := mustCall(t, ctx, rt, "implement:Z", "worktree")
		writeTree(t, c, map[string]string{"out/a.txt": "a", "out/b.txt": "b", "out/c.txt": "c"})
		return nil, nil
	})
	ready := filepath.Join(t.TempDir(), "ready")
	t.Setenv("LSFRT_RETAIN_STORE", store)
	t.Setenv("LSFRT_RETAIN_READY", ready)
	cmd := startHelper(t, cfg, "retain-hang")
	waitFor(t, 20*time.Second, "the helper to reach mid-copy", func() bool { return exists(ready) })
	cmd.Process.Signal(syscall.SIGKILL)
	cmd.Wait()
	staging, scratch := retainDirs(store, cfg.StateDir, "Z")
	if !exists(scratch) || !exists(staging) {
		t.Fatalf("the kill left no partial move: %v", storeEntries(t, store))
	}
	var res RetainedMove
	var err error
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		res, err = rt.MoveRetained(ctx, "Z", store, "main", []string{"out"}, []string{"implement:Z"})
		return nil, nil
	})
	if got := snapshot(t, filepath.Join(store, "Z", "out")); err != nil || len(got) != 4 || len(res.Moved) != 1 {
		t.Errorf("resume: %v %v %+v", err, got, res)
	}
	if es := storeEntries(t, store); !slices.Equal(es, []string{"Z"}) {
		t.Errorf("store: %v", es)
	}
}

// helperRetainHang is the retain-hang scenario: a copy that blocks at its
// first mid-copy checkpoint after writing the ready file.
func helperRetainHang(ctx context.Context, rt *Runtime) error {
	renameFn = func(a, b string) error { return &os.LinkError{Op: "rename", Old: a, New: b, Err: syscall.EXDEV} }
	retainHook = func(p string) {
		if p == "mid-copy" {
			os.WriteFile(os.Getenv("LSFRT_RETAIN_READY"), nil, 0o644)
			select {}
		}
	}
	_, err := rt.MoveRetained(ctx, "Z", os.Getenv("LSFRT_RETAIN_STORE"), "main", []string{"out"}, []string{"implement:Z"})
	return err
}

func TestCheckStore(t *testing.T) {
	cfg, _ := testConfig(t)
	file := filepath.Join(t.TempDir(), "f")
	os.WriteFile(file, nil, 0o644)
	inRepo := filepath.Join(cfg.Repo, "store")
	os.Mkdir(inRepo, 0o755)
	inState := filepath.Join(cfg.StateDir, "store")
	os.Mkdir(inState, 0o755)
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(cfg.Repo, link)
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		for _, c := range []struct{ name, store, want string }{
			{"fine", t.TempDir(), ""},
			{"missing", filepath.Join(t.TempDir(), "absent"), "no such file"},
			{"file", file, "is not a directory"},
			{"under repo", inRepo, "overlaps the repository"},
			{"repo", cfg.Repo, "overlaps the repository"},
			{"link to repo", link, "overlaps the repository"},
			{"ancestor of repo", filepath.Dir(cfg.Repo), "overlaps the repository"},
			{"under state dir", inState, "overlaps the state dir"},
		} {
			err := rt.CheckStore(c.store)
			if (c.want == "") != (err == nil) || err != nil && !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s: %v, want %q", c.name, err, c.want)
			}
		}
		return nil, nil
	})
}
