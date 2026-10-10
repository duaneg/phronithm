package lsfrt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

// Retained artefacts (docs/design/lsf-runtime.md, Runtime → Retained
// artefacts): before cleanup removes a merged unit's clones, the paths the
// unit declares in `retains` move to <store>/<unit>/. Paths go into a
// staging directory in the store, each at its final name only once whole,
// and the staging directory is renamed to <store>/<unit> at the end. A move
// record in the state dir (retained/<key>.json) names the staging and
// scratch directories and each path moved, so a rerun after a kill resumes
// its own partial move and never mistakes it for a foreign destination.

// RetainedPath is one declared path's outcome.
type RetainedPath struct {
	Path   string `json:"path"`
	From   string `json:"from,omitempty"`   // source clone label; "" when moved by an earlier attempt
	Method string `json:"method,omitempty"` // "rename" | "copy"
	Files  int    `json:"files,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
}

// RetainedMove is a unit's move. Dest is "" when every path was missing.
type RetainedMove struct {
	Unit    string         `json:"unit"`
	Dest    string         `json:"dest"`
	Moved   []RetainedPath `json:"moved"`
	Missing []string       `json:"missing,omitempty"`
}

// retainRecord is the move's durable progress. Phase is moving (paths go
// to Staging), renaming (Staging is being renamed to Dest) or done.
type retainRecord struct {
	Phase   string         `json:"phase"`
	Unit    string         `json:"unit"`
	Store   string         `json:"store"`
	Staging string         `json:"staging"`
	Scratch string         `json:"scratch"`
	Moved   []RetainedPath `json:"moved"`
	Dev     uint64         `json:"dev,omitempty"`
	Ino     uint64         `json:"ino,omitempty"`
	Result  *RetainedMove  `json:"result,omitempty"`
}

// Test seams: set only by this package's tests.
var (
	renameFn      = os.Rename
	retainHook    = func(point string) {}
	verifyCorrupt = func() bool { return false }
	deviceOf      = func(fi fs.FileInfo) uint64 { return devIno(fi)[0] }
	statfsFn      = func(path string) (uint64, error) {
		var s syscall.Statfs_t
		if err := syscall.Statfs(path, &s); err != nil {
			return 0, err
		}
		return s.Bavail * uint64(s.Bsize), nil
	}
)

const copyChunk = 1 << 20

func (rt *Runtime) retainRecordPath(unit string) string {
	return filepath.Join(rt.cfg.StateDir, "retained", journalKey(unit)+".json")
}

func (rt *Runtime) writeRetainRecord(path string, rec *retainRecord) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'))
}

// MoveRetained moves unit's declared paths from its writer clones (sources,
// oldest first) to <store>/<unit>. Each path comes from the latest source
// holding it, which must be one RemoveWorktree would remove (merged into
// integration, on its branch, nothing unfetched); a path no source holds is
// Missing. Every check runs before anything changes, so a refusal (a
// symlink on the path, a special file, a destination that exists, too
// little space, a latest holder that cleanup keeps) leaves the clones and
// the store as they were. A path on the store's filesystem is renamed; one
// on another (EXDEV) is copied to a scratch directory, verified (sha256,
// size and mode of each file, each link's target, the set of entries) and
// renamed into staging before its source is removed.
//
// The move is idempotent across kills: a rerun reads the move record and
// completes the move, and once done returns the recorded result. It has no
// cleanup on error, so an error leaves the state a kill would. A cancelled
// ctx (a repeated signal) is a StopCancelled stop; an unreadable record is a
// runtime-error stop; any other failure is an error, the caller keeping the
// unit's clones.
func (rt *Runtime) MoveRetained(ctx context.Context, unit, store, integration string, paths, sources []string) (res RetainedMove, err error) {
	defer func() {
		if err != nil {
			if _, stop := IsStop(err); !stop {
				rt.event("retained-refused", map[string]any{"unit": unit, "error": err.Error()})
			}
		}
	}()
	args, err := rt.isolationArgs(Opts{Label: "retain:" + unit, Isolation: "worktree"})
	if err != nil {
		return res, err
	}
	if strings.HasPrefix(integration, "-") || integration == "" {
		return res, fmt.Errorf("%s: integration branch %q is not a branch name", unit, integration)
	}
	if !plan.SafeUnitDir(unit) {
		return res, fmt.Errorf("unit id %q cannot name a store directory", unit)
	}
	if !filepath.IsAbs(store) || filepath.Clean(store) != store {
		return res, fmt.Errorf("%s: store %q is not a clean absolute path", unit, store)
	}
	dest := filepath.Join(store, unit)
	if filepath.Dir(dest) != store {
		return res, fmt.Errorf("%s: destination %s is not directly in the store %s", unit, dest, store)
	}
	if err := rt.boundary(ctx); err != nil {
		return res, err
	}
	kctx, kcancel := rt.killContext(ctx)
	defer kcancel()
	cancelled := func() error {
		if kctx.Err() != nil {
			return &StopError{Reason: StopCancelled}
		}
		return nil
	}

	recPath := rt.retainRecordPath(unit)
	var rec *retainRecord
	switch b, err := os.ReadFile(recPath); {
	case err == nil:
		rec = &retainRecord{}
		if err := json.Unmarshal(b, rec); err != nil || rec.Unit != unit || rec.Staging == "" || rec.Phase == "done" && rec.Result == nil {
			return res, rt.setStop(StopRuntimeError, fmt.Sprintf("%s: retained-move record %s unreadable (%v); inspect it, the store's staging directory and the unit's clones", unit, recPath, err))
		}
	case !errors.Is(err, fs.ErrNotExist):
		return res, rt.setStop(StopRuntimeError, fmt.Sprintf("%s: retained-move record %s: %v", unit, recPath, err))
	}
	if rec != nil && rec.Phase == "done" {
		if rec.Result.Dest != "" {
			if fi, err := os.Lstat(rec.Result.Dest); err != nil || devIno(fi) != [2]uint64{rec.Dev, rec.Ino} {
				rt.Log(fmt.Sprintf("%s: retained paths were moved to %s, which is now absent or replaced", unit, rec.Result.Dest))
			}
		}
		return *rec.Result, nil
	}
	if rec != nil && rec.Store != store {
		return res, fmt.Errorf("%s: retained-move record %s is for store %s, not %s", unit, recPath, rec.Store, store)
	}
	staging, scratch := retainDirs(store, rt.cfg.StateDir, unit)
	if rec != nil {
		staging, scratch = rec.Staging, rec.Scratch
	}
	// wrap names what a failure after the first change leaves behind.
	wrap := func(err error) error {
		if _, stop := IsStop(err); stop || rec == nil {
			return err
		}
		return fmt.Errorf("%w (staging %s, scratch %s, record %s)", err, staging, scratch, recPath)
	}
	exists := func(p string) bool { _, err := os.Lstat(p); return err == nil }

	if exists(dest) {
		if rec != nil && rec.Phase == "renaming" && !exists(staging) {
			return rt.finishRetain(unit, store, dest, recPath, rec, nil)
		}
		return res, wrap(fmt.Errorf("%s: destination %s exists", unit, dest))
	}
	if rec != nil && rec.Phase == "renaming" {
		return rt.renameRetained(unit, store, staging, scratch, dest, recPath, rec, nil, wrap)
	}

	// Pre-pass: nothing changes until every path checks out.
	storeFi, err := os.Stat(store)
	if err != nil || !storeFi.IsDir() {
		return res, fmt.Errorf("%s: store %s is not a directory (%v)", unit, store, err)
	}
	gctx, gcancel := rt.gitContext(kctx)
	defer gcancel()
	targets := map[string]removalTarget{}
	target := func(label string) (removalTarget, error) {
		if t, ok := targets[label]; ok {
			return t, nil
		}
		t, err := rt.removal(gctx, args, label, integration, true)
		if err != nil {
			return t, err
		}
		targets[label] = t
		return t, nil
	}
	type move struct {
		path, label, root, src string
		files                  int
		bytes                  int64
	}
	var moves []move
	var missing []string
	already := map[string]RetainedPath{}
	if rec != nil {
		for _, m := range rec.Moved {
			already[m.Path] = m
		}
	}
	var copyBytes int64
	for _, p := range slices.Sorted(slices.Values(paths)) {
		_, recorded := already[p]
		inStaging := exists(filepath.Join(staging, p))
		switch {
		case inStaging:
			if !recorded {
				already[p] = RetainedPath{Path: p}
			}
			continue
		case recorded:
			return res, wrap(fmt.Errorf("%s: %s was moved to the staging directory but is not there now", unit, p))
		}
		var holder string
		var t removalTarget
		for _, label := range slices.Backward(sources) {
			tt, err := target(label)
			if err != nil {
				return res, wrap(err)
			}
			if tt.exists && exists(filepath.Join(tt.path, p)) {
				holder, t = label, tt
				break
			}
		}
		if err := cancelled(); err != nil {
			return res, err
		}
		if holder == "" {
			missing = append(missing, p)
			continue
		}
		if t.kept != "" {
			return res, wrap(fmt.Errorf("%s: the latest copy of %s is in %s (%s), which cleanup keeps: %s", unit, p, holder, t.path, t.kept))
		}
		src := filepath.Join(t.path, p)
		if err := insideClone(t.path, p); err != nil {
			return res, wrap(fmt.Errorf("%s: %s in %s: %w", unit, p, holder, err))
		}
		files, bytes, err := scanTree(src)
		if err != nil {
			return res, wrap(fmt.Errorf("%s: %s in %s: %w", unit, p, holder, err))
		}
		if fi, err := os.Lstat(src); err == nil && deviceOf(fi) != deviceOf(storeFi) {
			copyBytes += bytes
		}
		moves = append(moves, move{p, holder, t.path, src, files, bytes})
	}
	if copyBytes > 0 {
		free, err := statfsFn(store)
		if err != nil {
			return res, wrap(fmt.Errorf("%s: free space in %s: %w", unit, store, err))
		}
		if uint64(copyBytes) > free {
			return res, wrap(fmt.Errorf("%s: copying %d bytes to %s needs more than its %d free bytes", unit, copyBytes, store, free))
		}
	}

	// Moves.
	if rec == nil {
		if len(moves) == 0 {
			r := RetainedMove{Unit: unit, Moved: []RetainedPath{}, Missing: missing}
			if len(already) > 0 {
				return res, fmt.Errorf("%s: internal: staged paths without a record", unit)
			}
			return rt.finishRetain(unit, store, "", recPath, &retainRecord{Unit: unit, Store: store, Staging: staging, Scratch: scratch, Moved: []RetainedPath{}}, &r)
		}
		rec = &retainRecord{Phase: "moving", Unit: unit, Store: store, Staging: staging, Scratch: scratch, Moved: []RetainedPath{}}
		if err := rt.writeRetainRecord(recPath, rec); err != nil {
			return res, fmt.Errorf("%s: retained-move record: %w", unit, err)
		}
		retainHook("after-moving-record")
	}
	if err := os.Mkdir(staging, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return res, wrap(err)
	}
	if err := syncDir(store); err != nil {
		return res, wrap(err)
	}
	for _, m := range moves {
		if err := cancelled(); err != nil {
			return res, err
		}
		dst := filepath.Join(staging, m.path)
		if err := mkdirLike(staging, m.root, filepath.Dir(m.path)); err != nil {
			return res, wrap(err)
		}
		method := "rename"
		err := renameFn(m.src, dst)
		switch {
		case err == nil:
			retainHook("after-path-rename")
			syncDir(filepath.Dir(m.src))
		case errors.Is(err, syscall.EXDEV):
			method = "copy"
			if err := os.RemoveAll(scratch); err != nil {
				return res, wrap(err)
			}
			if err := os.Mkdir(scratch, 0o700); err != nil {
				return res, wrap(err)
			}
			x := filepath.Join(scratch, "x")
			manifest, err := copyVerified(kctx, m.src, x)
			if err != nil {
				return res, wrap(err)
			}
			if err := verifyTree(x, manifest); err != nil {
				return res, wrap(fmt.Errorf("%s: %s: copy failed verification, source kept: %w", unit, m.path, err))
			}
			retainHook("after-copy-verify")
			if err := os.Rename(x, dst); err != nil {
				return res, wrap(err)
			}
			retainHook("after-final-name")
		default:
			return res, wrap(fmt.Errorf("%s: move %s: %w", unit, m.path, err))
		}
		for d := filepath.Dir(dst); ; d = filepath.Dir(d) {
			if err := syncDir(d); err != nil {
				return res, wrap(err)
			}
			if d == staging {
				break
			}
		}
		rp := RetainedPath{Path: m.path, From: m.label, Method: method, Files: m.files, Bytes: m.bytes}
		rec.Moved = append(rec.Moved, rp)
		already[m.path] = rp
		if err := rt.writeRetainRecord(recPath, rec); err != nil {
			return res, wrap(err)
		}
		retainHook("after-path-record")
		if method == "copy" {
			if err := os.RemoveAll(m.src); err != nil {
				return res, wrap(fmt.Errorf("%s: remove copied source %s: %w", unit, m.src, err))
			}
			retainHook("after-source-remove")
		}
	}
	moved := make([]RetainedPath, 0, len(already))
	for _, p := range slices.Sorted(maps.Keys(already)) {
		moved = append(moved, already[p])
	}
	r := RetainedMove{Unit: unit, Dest: dest, Moved: moved, Missing: missing}
	if len(moved) == 0 {
		if err := os.RemoveAll(staging); err != nil {
			return res, wrap(err)
		}
		os.RemoveAll(scratch)
		r.Dest = ""
		return rt.finishRetain(unit, store, "", recPath, rec, &r)
	}
	return rt.renameRetained(unit, store, staging, scratch, dest, recPath, rec, &r, wrap)
}

// renameRetained renames the staging directory to dest and finishes. r is
// nil on a rerun from the renaming phase, which takes the recorded result.
func (rt *Runtime) renameRetained(unit, store, staging, scratch, dest, recPath string, rec *retainRecord, r *RetainedMove, wrap func(error) error) (RetainedMove, error) {
	if err := os.RemoveAll(scratch); err != nil {
		return RetainedMove{}, wrap(err)
	}
	if rec.Phase != "renaming" {
		rec.Phase, rec.Result = "renaming", r
		if err := rt.writeRetainRecord(recPath, rec); err != nil {
			return RetainedMove{}, wrap(err)
		}
		retainHook("after-renaming-record")
	}
	if err := os.Rename(staging, dest); err != nil {
		return RetainedMove{}, wrap(fmt.Errorf("%s: rename staging to %s: %w", unit, dest, err))
	}
	retainHook("after-staging-rename")
	return rt.finishRetain(unit, store, dest, recPath, rec, nil)
}

// finishRetain fsyncs the store and writes the done record. r is nil when
// the record already holds the result.
func (rt *Runtime) finishRetain(unit, store, dest, recPath string, rec *retainRecord, r *RetainedMove) (RetainedMove, error) {
	if r != nil {
		rec.Result = r
	}
	if rec.Result == nil {
		return RetainedMove{}, fmt.Errorf("%s: retained-move record %s has no result", unit, recPath)
	}
	if dest != "" {
		if err := syncDir(store); err != nil {
			return RetainedMove{}, err
		}
		fi, err := os.Lstat(dest)
		if err != nil {
			return RetainedMove{}, err
		}
		di := devIno(fi)
		rec.Dev, rec.Ino = di[0], di[1]
		retainHook("before-done-record")
	}
	rec.Phase = "done"
	if err := rt.writeRetainRecord(recPath, rec); err != nil {
		return RetainedMove{}, fmt.Errorf("%s: retained-move record: %w", unit, err)
	}
	res := *rec.Result
	ev := map[string]any{"unit": unit, "dest": res.Dest, "paths": len(res.Moved), "missing": res.Missing}
	rt.event("retained-moved", ev)
	return res, nil
}

// retainDirs are the staging and scratch directories for unit's move: in
// the store, hidden, named for the state dir and the unit.
func retainDirs(store, stateDir, unit string) (staging, scratch string) {
	abs, err := filepath.Abs(stateDir)
	if err != nil {
		abs = stateDir
	}
	sum := sha256.Sum256([]byte(abs))
	base := filepath.Join(store, ".lsf-retain-"+hex.EncodeToString(sum[:4])+"-"+journalKey(unit))
	return base + ".partial", base + ".copy"
}

// insideClone checks that rel, a clean relative path, names a file inside
// root through no symlink: no component, the last included, is a link.
func insideClone(root, rel string) error {
	p := root
	for _, c := range strings.Split(rel, "/") {
		p = filepath.Join(p, c)
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink; a retained path must not pass through one", strings.TrimPrefix(p, root+"/"))
		}
	}
	rroot, err1 := filepath.EvalSymlinks(root)
	rp, err2 := filepath.EvalSymlinks(filepath.Join(root, rel))
	if err := errors.Join(err1, err2); err != nil {
		return err
	}
	if !strings.HasPrefix(rp, rroot+"/") {
		return fmt.Errorf("resolves to %s, outside the clone", rp)
	}
	return nil
}

// scanTree counts the regular files and their bytes under src, refusing
// anything but directories, regular files and symlinks.
func scanTree(src string) (files int, bytes int64, err error) {
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch t := d.Type(); {
		case t.IsDir(), t&fs.ModeSymlink != 0:
		case t.IsRegular():
			fi, err := d.Info()
			if err != nil {
				return err
			}
			files++
			bytes += fi.Size()
		default:
			return fmt.Errorf("%s is a special file (%v); only files, directories and symlinks are retained", p, t)
		}
		return nil
	})
	return files, bytes, err
}

// mkdirLike creates staging/rel's missing directories with the modes of
// their counterparts under root.
func mkdirLike(staging, root, rel string) error {
	if rel == "." {
		return nil
	}
	p, q := staging, root
	for _, c := range strings.Split(rel, "/") {
		p, q = filepath.Join(p, c), filepath.Join(q, c)
		if _, err := os.Lstat(p); err == nil {
			continue
		}
		mode := fs.FileMode(0o755)
		if fi, err := os.Stat(q); err == nil {
			mode = fi.Mode().Perm()
		}
		if err := os.Mkdir(p, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(p, mode|0o700); err != nil {
			return err
		}
	}
	return nil
}

// copied is one copied tree entry, keyed by its path relative to the root.
type copied struct {
	mode fs.FileMode // type and permission bits
	size int64
	sum  [sha256.Size]byte
	link string
}

const modeBits = fs.ModeType | fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

// copyVerified copies src to dst, which must not exist, without following
// links, and returns what it wrote: each file's sha256 as read from the
// source while copying. Files and directories are fsynced; a directory's
// mode is set after its children.
func copyVerified(ctx context.Context, src, dst string) (map[string]copied, error) {
	manifest := map[string]copied{}
	var dirs []string
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return &StopError{Reason: StopCancelled}
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dst, rel)
		fi, err := d.Info()
		if err != nil {
			return err
		}
		e := copied{mode: fi.Mode() & modeBits}
		switch t := fi.Mode().Type(); {
		case t.IsDir():
			if err := os.Mkdir(out, 0o700); err != nil {
				return err
			}
			dirs = append(dirs, rel)
		case t&fs.ModeSymlink != 0:
			if e.link, err = os.Readlink(p); err != nil {
				return err
			}
			if err := os.Symlink(e.link, out); err != nil {
				return err
			}
		case t.IsRegular():
			if e.sum, e.size, err = copyHashed(ctx, p, out, fi); err != nil {
				return err
			}
			retainHook("mid-copy")
		default:
			return fmt.Errorf("%s is a special file (%v)", p, t)
		}
		manifest[rel] = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, rel := range slices.Backward(dirs) {
		out := filepath.Join(dst, rel)
		if err := syncDir(out); err != nil {
			return nil, err
		}
		if err := os.Chmod(out, manifest[rel].mode&^fs.ModeType); err != nil {
			return nil, err
		}
	}
	return manifest, nil
}

func copyHashed(ctx context.Context, src, dst string, fi fs.FileInfo) (sum [sha256.Size]byte, n int64, err error) {
	in, err := os.OpenFile(src, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return sum, 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return sum, 0, err
	}
	defer out.Close()
	h := sha256.New()
	buf := make([]byte, copyChunk)
	r := io.TeeReader(in, h)
	for {
		if ctx.Err() != nil {
			return sum, n, &StopError{Reason: StopCancelled}
		}
		k, rerr := io.ReadFull(r, buf)
		if k > 0 {
			if _, err := out.Write(buf[:k]); err != nil {
				return sum, n, err
			}
			n += int64(k)
		}
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
		if rerr != nil {
			return sum, n, rerr
		}
	}
	if err := out.Sync(); err != nil {
		return sum, n, err
	}
	if err := out.Chmod(fi.Mode() & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky)); err != nil {
		return sum, n, err
	}
	if err := out.Close(); err != nil {
		return sum, n, err
	}
	if err := os.Chtimes(dst, fi.ModTime(), fi.ModTime()); err != nil {
		return sum, n, err
	}
	copy(sum[:], h.Sum(nil))
	return sum, n, nil
}

// verifyTree re-reads the copy at dst against the manifest: the same
// entries, each with its mode, each file's size and sha256 and each link's
// target.
func verifyTree(dst string, manifest map[string]copied) error {
	seen := 0
	err := filepath.WalkDir(dst, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dst, p)
		want, ok := manifest[rel]
		if !ok {
			return fmt.Errorf("%s: not in the source", rel)
		}
		seen++
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if got := fi.Mode() & modeBits; got != want.mode {
			return fmt.Errorf("%s: mode %v, source %v", rel, got, want.mode)
		}
		switch {
		case fi.Mode()&fs.ModeSymlink != 0:
			if l, err := os.Readlink(p); err != nil || l != want.link {
				return fmt.Errorf("%s: link %q, source %q (%v)", rel, l, want.link, err)
			}
		case fi.Mode().IsRegular():
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			h := sha256.New()
			n, err := io.Copy(h, f)
			f.Close()
			if err != nil {
				return err
			}
			var sum [sha256.Size]byte
			copy(sum[:], h.Sum(nil))
			if verifyCorrupt() {
				sum[0] ^= 0xff
			}
			if n != want.size || fi.Size() != want.size || sum != want.sum {
				return fmt.Errorf("%s: copy differs from the source (size %d, source %d, or sha256)", rel, n, want.size)
			}
		}
		return nil
	})
	if err == nil && seen != len(manifest) {
		err = fmt.Errorf("copy has %d entries, source %d", seen, len(manifest))
	}
	return err
}

func devIno(fi fs.FileInfo) [2]uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return [2]uint64{uint64(st.Dev), st.Ino}
	}
	return [2]uint64{}
}

// ClonePath is where label's kept clone is, or was.
func (rt *Runtime) ClonePath(label string) string {
	return filepath.Join(rt.cfg.StateDir, "clone", journalKey(label))
}

// CheckStore checks a declared store at the pin: an existing directory
// whose resolved path is neither equal to, under, nor an ancestor of the
// main checkout or the state dir.
func (rt *Runtime) CheckStore(store string) error {
	fi, err := os.Stat(store)
	if err != nil {
		return fmt.Errorf("store %s: %w", store, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("store %s is not a directory", store)
	}
	rs, err := filepath.EvalSymlinks(store)
	if err != nil {
		return fmt.Errorf("store %s: %w", store, err)
	}
	for _, x := range []struct{ what, path string }{{"the repository", rt.cfg.Repo}, {"the state dir", rt.cfg.StateDir}} {
		r, err := filepath.EvalSymlinks(x.path)
		if err != nil {
			return fmt.Errorf("store check: %s %s: %w", x.what, x.path, err)
		}
		if rs == r || strings.HasPrefix(rs, r+"/") || strings.HasPrefix(r, rs+"/") {
			return fmt.Errorf("store %s (%s) overlaps %s %s; it must be outside both the repository and the state dir", store, rs, x.what, r)
		}
	}
	return nil
}
