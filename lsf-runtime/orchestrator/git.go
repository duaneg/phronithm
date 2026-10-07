package orchestrator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

// gitRunner runs one git command in dir and returns its stdout; stderr is
// folded into the error. Tests decorate it to rewrite chosen outputs.
type gitRunner interface {
	run(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// Bounds on one code-stage git command: a commit runs the project's hooks,
// which may run its tests; anything else is quick unless a lock is stuck.
const (
	gitCommitTimeout = 30 * time.Minute
	gitTimeout       = 5 * time.Minute
)

type execGit struct{}

// run runs git in its own process group, so a timeout or cancellation
// kills a hook's children with it.
func (execGit) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	d := gitTimeout
	if len(args) > 0 && args[0] == "commit" {
		d = gitCommitTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	// Paths are literal: a hook-touched path like `a*b` must not match others.
	// LC_ALL=C: the merge reads git's refusal message.
	cmd.Env = append(scrubbedEnv(), "GIT_TERMINAL_PROMPT=0", "GIT_LITERAL_PATHSPECS=1", "LC_ALL=C")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// SIGTERM lets git remove its own lock files; WaitDelay then kills git
	// if it lingers.
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 10 * time.Second
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return out.Bytes(), &gitError{args: args, err: err, stderr: strings.TrimSpace(stderr.String())}
	}
	return out.Bytes(), nil
}

// scrubbedEnv is the environment without the variables that would point
// git at another repository or index (set when run from inside a hook).
func scrubbedEnv() []string {
	return slices.DeleteFunc(os.Environ(), func(kv string) bool {
		for _, k := range []string{"GIT_DIR=", "GIT_INDEX_FILE=", "GIT_WORK_TREE=", "GIT_COMMON_DIR=", "GIT_OBJECT_DIRECTORY="} {
			if strings.HasPrefix(kv, k) {
				return true
			}
		}
		return false
	})
}

// gitError is a failed git command.
type gitError struct {
	args   []string
	err    error
	stderr string
}

func (e *gitError) Error() string {
	s := "git " + strings.Join(e.args, " ") + ": " + e.err.Error()
	if e.stderr != "" {
		s += ": " + e.stderr
	}
	return s
}

func (e *gitError) Unwrap() error { return e.err }

// exitCode is a failed git command's exit status, or -1 when it did not
// exit (not found, killed, timed out).
func exitCode(err error) int {
	var x *exec.ExitError
	if errors.As(err, &x) {
		return x.ExitCode()
	}
	return -1
}

// gitOps are the git operations the code stages share.
type gitOps struct{ r gitRunner }

// out is a command's stdout, trimmed.
func (g gitOps) out(ctx context.Context, dir string, args ...string) (string, error) {
	b, err := g.r.run(ctx, dir, args...)
	return strings.TrimSpace(string(b)), err
}

// status runs a command whose exit status 1 means no: (false, nil).
func (g gitOps) status(ctx context.Context, dir string, args ...string) (bool, error) {
	_, err := g.r.run(ctx, dir, args...)
	switch {
	case err == nil:
		return true, nil
	case exitCode(err) == 1:
		return false, nil
	}
	return false, err
}

// blob is path's content at rev; ok is false when rev has no such path.
func (g gitOps) blob(ctx context.Context, dir, rev, path string) (b []byte, ok bool, err error) {
	if rev == "" { // an unborn branch; ":path" would read the index
		return nil, false, nil
	}
	oid, err := g.out(ctx, dir, "rev-parse", "--verify", "--quiet", rev+":"+path)
	if exitCode(err) == 1 {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	b, err = g.r.run(ctx, dir, "cat-file", "blob", oid)
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// head is HEAD's commit, "" on an unborn branch.
func (g gitOps) head(ctx context.Context, dir string) (string, error) {
	s, err := g.out(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD")
	if exitCode(err) == 1 {
		return "", nil
	}
	return s, err
}

// headRef is the branch HEAD is on (refs/heads/…), "" when detached.
func (g gitOps) headRef(ctx context.Context, dir string) (string, error) {
	s, err := g.out(ctx, dir, "symbolic-ref", "-q", "HEAD")
	if exitCode(err) == 1 {
		return "", nil
	}
	return s, err
}

// headBranch is HEAD's branch name as `git rev-parse --abbrev-ref HEAD`
// prints it: "HEAD" when detached.
func (g gitOps) headBranch(ctx context.Context, dir string) (string, error) {
	ref, err := g.headRef(ctx, dir)
	if err != nil || ref == "" {
		return "HEAD", err
	}
	return strings.TrimPrefix(ref, "refs/heads/"), nil
}

// pauseRequested is whether the pause ref exists.
func (g gitOps) pauseRequested(ctx context.Context, dir, ref string) (bool, error) {
	return g.status(ctx, dir, "show-ref", "--verify", "--quiet", ref)
}

// Commit trailers. Lsf-Content is the digest of the bytes the stage wrote,
// so a stage commit checks itself in any later launch (intact).
const (
	trailerStep    = "Lsf-Step"
	trailerContent = "Lsf-Content"
)

func contentDigest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// trailers are a commit's values for key, in order; none when it has none.
func (g gitOps) trailers(ctx context.Context, dir, sha, key string) ([]string, error) {
	s, err := g.out(ctx, dir, "log", "-1", "--format=%(trailers:key="+key+",valueonly)", sha, "--")
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

// trailer is a commit's last value for key, "" when it has none.
func (g gitOps) trailer(ctx context.Context, dir, sha, key string) (string, error) {
	vs, err := g.trailers(ctx, dir, sha, key)
	if err != nil || len(vs) == 0 {
		return "", err
	}
	return vs[len(vs)-1], nil
}

// contentFor is the Lsf-Content digest a commit's trailers give path: the
// value "<digest> <path>" of a multi-path commit, else the last bare
// "<digest>" of a single-path one (bare true); "" when none does.
func contentFor(values []string, path string) (digest string, bare bool) {
	for _, v := range values {
		d, p, multi := strings.Cut(v, " ")
		switch {
		case multi && p == path:
			return d, false
		case !multi:
			digest, bare = v, true
		}
	}
	return digest, bare
}

// intact reports whether sha carries Lsf-Content for path and its blob at
// path hashes to it; stage reports whether it carries Lsf-Content at all.
func (g gitOps) intact(ctx context.Context, dir, sha, path string) (stage, intact bool, err error) {
	values, err := g.trailers(ctx, dir, sha, trailerContent)
	if err != nil || len(values) == 0 {
		return false, false, err
	}
	want, bare := contentFor(values, path)
	if want == "" {
		return true, false, nil
	}
	if bare {
		// A bare digest covers the one path its commit changed, not any
		// path that happens to hold the same bytes.
		files, err := g.changed(ctx, dir, sha)
		if err != nil || !slices.Contains(files, path) {
			return true, false, err
		}
	}
	b, ok, err := g.blob(ctx, dir, sha, path)
	if err != nil {
		return true, false, err
	}
	return true, ok && contentDigest(b) == want, nil
}

// guard refuses a write over a damaged stage commit: the last first-parent
// commit touching path on branch, when it carries Lsf-Content and is not
// intact. An earlier attempt left it, whatever its label or input.
func (g gitOps) guard(ctx context.Context, dir, branch, path string) error {
	last, err := g.out(ctx, dir, "log", "-1", "--first-parent", "--format=%H", "refs/heads/"+branch, "--", path)
	if err != nil || last == "" {
		return err
	}
	stage, ok, err := g.intact(ctx, dir, last, path)
	if err != nil || !stage || ok {
		return err
	}
	return fmt.Errorf("stage commit %s left %s with content its %s trailer does not match; repair it with a new commit that has no %s trailer (not --amend or --fixup, which keep it) and relaunch", last, path, trailerContent, trailerContent)
}

// adoption is how a stage recognises its own earlier commit.
type adoption struct {
	label string
	// matches: the candidate's blob is this stage's rendering from its
	// parent's blob (parentOK false: no file at the parent).
	matches func(parent []byte, parentOK bool, candidate []byte) bool
	// survives: the stage's effect is still in the branch tip's blob.
	survives func(tip []byte, tipOK bool) bool
}

// adoptionWalk bounds the history an adoption searches.
const adoptionWalk = 50

// adopt returns the newest first-parent commit touching path on branch
// that is intact, carries this step's label, has one parent, changes only
// path, matches its parent and whose effect survives at the tip; "" when
// none does. survived reports whether the effect is at the tip at all. The
// SHA is checked before it is returned (checkReachable).
func (g gitOps) adopt(ctx context.Context, dir, branch, path string, a adoption) (sha string, survived bool, err error) {
	ref := "refs/heads/" + branch
	tip, tipOK, err := g.blob(ctx, dir, ref, path)
	if err != nil {
		return "", false, err
	}
	if !a.survives(tip, tipOK) {
		return "", false, nil
	}
	sha, err = g.findAdoptable(ctx, dir, branch, path, a)
	return sha, true, err
}

func (g gitOps) findAdoptable(ctx context.Context, dir, branch, path string, a adoption) (string, error) {
	return g.adoptSet(ctx, dir, branch, []string{path}, a.label, func(parent, cand map[string]blobAt) bool {
		return a.matches(parent[path].b, parent[path].ok, cand[path].b)
	})
}

// blobAt is a path's content at a commit; ok is false when it has none.
type blobAt struct {
	b  []byte
	ok bool
}

// adoptSet returns the newest of the first adoptionWalk first-parent
// commits on branch touching paths[0] that is intact for every path,
// carries label, has one parent, changes exactly paths, and whose blobs
// matches accepts against its parent's; "" when none does. The SHA is
// checked before it is returned (checkReachable).
func (g gitOps) adoptSet(ctx context.Context, dir, branch string, paths []string, label string, matches func(parent, cand map[string]blobAt) bool) (string, error) {
	ref := "refs/heads/" + branch
	list, err := g.out(ctx, dir, "log", "--first-parent", "-n", fmt.Sprint(adoptionWalk), "--format=%H", ref, "--", paths[0])
	if err != nil {
		return "", err
	}
	want := slices.Sorted(slices.Values(paths))
candidates:
	for _, c := range strings.Fields(list) {
		for _, p := range paths {
			if _, ok, err := g.intact(ctx, dir, c, p); err != nil || !ok {
				if err != nil {
					return "", err
				}
				continue candidates
			}
		}
		if l, err := g.trailer(ctx, dir, c, trailerStep); err != nil || l != label {
			if err != nil {
				return "", err
			}
			continue
		}
		parents, err := g.out(ctx, dir, "log", "-1", "--format=%P", c, "--")
		if err != nil {
			return "", err
		}
		files, err := g.changed(ctx, dir, c)
		if err != nil {
			return "", err
		}
		slices.Sort(files)
		if len(strings.Fields(parents)) > 1 || !slices.Equal(files, want) {
			continue
		}
		parent, cand := map[string]blobAt{}, map[string]blobAt{}
		for _, p := range paths {
			b, _, err := g.blob(ctx, dir, c, p)
			if err != nil {
				return "", err
			}
			cand[p] = blobAt{b, true}
			pb, pok, err := g.blob(ctx, dir, c+"^", p) // not ok for a root commit
			if err != nil {
				return "", err
			}
			parent[p] = blobAt{pb, pok}
		}
		if !matches(parent, cand) {
			continue
		}
		return c, g.checkReachable(ctx, dir, c, branch, paths[0], true)
	}
	return "", nil
}

// checkReachable is the check on a SHA a stage returns without having just
// made it: it resolves to a commit that is an ancestor of branch and is
// intact (wantIntact) or touches path.
func (g gitOps) checkReachable(ctx context.Context, dir, sha, branch, path string, wantIntact bool) error {
	bad := func(what string) error { return fmt.Errorf("commit %s %s", sha, what) }
	if t, err := g.out(ctx, dir, "cat-file", "-t", sha); err != nil || t != "commit" {
		return bad("does not resolve to a commit")
	}
	if ok, err := g.status(ctx, dir, "merge-base", "--is-ancestor", sha, "refs/heads/"+branch); err != nil || !ok {
		if err != nil {
			return err
		}
		return bad("is not on " + branch)
	}
	if wantIntact {
		if _, ok, err := g.intact(ctx, dir, sha, path); err != nil || !ok {
			if err != nil {
				return err
			}
			return bad("is not intact")
		}
		return nil
	}
	files, err := g.changed(ctx, dir, sha)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f == path {
			return nil
		}
	}
	return bad("does not touch " + path)
}

// changed are the paths sha changes against its first parent (all its
// paths for a root commit).
func (g gitOps) changed(ctx context.Context, dir, sha string) ([]string, error) {
	parents, err := g.out(ctx, dir, "log", "-1", "--format=%P", sha, "--")
	if err != nil {
		return nil, err
	}
	args := []string{"diff-tree", "--root", "--no-commit-id", "--name-only", "-r", "-z", sha}
	if ps := strings.Fields(parents); len(ps) > 0 {
		args = []string{"diff-tree", "--no-commit-id", "--name-only", "-r", "-z", ps[0], sha} // a merge too
	}
	b, err := g.r.run(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, f := range strings.Split(string(b), "\x00") {
		if f != "" {
			out = append(out, f)
		}
	}
	return out, nil
}

// fileWrite is one path a stage writes and the bytes it writes there.
type fileWrite struct {
	Path string
	Want []byte
}

func pathsOf(files []fileWrite) []string {
	out := []string{}
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

// verifyCommit checks a commit the stage has just made: a commit, whose
// only parent is pre (none when pre is ""), changing exactly path, at the
// tip of branch, whose blob at path is want byte for byte.
func (g gitOps) verifyCommit(ctx context.Context, dir, branch, sha, pre, path string, want []byte) error {
	return g.verifySet(ctx, dir, branch, sha, pre, []fileWrite{{path, want}})
}

// verifySet is verifyCommit over every path of a commit: it changes
// exactly those paths, each blob byte for byte.
func (g gitOps) verifySet(ctx context.Context, dir, branch, sha, pre string, writes []fileWrite) error {
	bad := func(what string) error { return fmt.Errorf("commit %s fails verification: %s", sha, what) }
	if t, err := g.out(ctx, dir, "cat-file", "-t", sha); err != nil || t != "commit" {
		return bad("not a commit")
	}
	parents, err := g.out(ctx, dir, "log", "-1", "--format=%P", sha, "--")
	if err != nil {
		return err
	}
	if parents != pre {
		return bad(fmt.Sprintf("parent %q, want %q", parents, pre))
	}
	files, err := g.changed(ctx, dir, sha)
	if err != nil {
		return err
	}
	changed, want := slices.Sorted(slices.Values(files)), slices.Sorted(slices.Values(pathsOf(writes)))
	if !slices.Equal(changed, want) {
		if len(want) == 1 {
			return bad(fmt.Sprintf("changes %q, want only %s", files, want[0]))
		}
		return bad(fmt.Sprintf("changes %q, want only %q", files, want))
	}
	tip, err := g.out(ctx, dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if err != nil || tip != sha {
		return bad("not the tip of " + branch)
	}
	for _, w := range writes {
		b, ok, err := g.blob(ctx, dir, sha, w.Path)
		if err != nil {
			return err
		}
		if !ok || !bytes.Equal(b, w.Want) {
			return bad(w.Path + " differs from the content written")
		}
	}
	return nil
}

// writeAndCommit writes want to path in dir and commits path alone on
// branch, with the stage trailers; it returns the verified commit
// (commitFiles with one file).
func (g gitOps) writeAndCommit(ctx context.Context, dir, branch, path, subject, label string, want []byte, pre string) (string, error) {
	return g.commitFiles(ctx, dir, branch, []fileWrite{{path, want}}, subject, label, pre)
}

// contentTrailers are a commit's Lsf-Content lines: a bare digest for one
// file, one "<digest> <path>" per file for several.
func contentTrailers(files []fileWrite) string {
	if len(files) == 1 {
		return trailerContent + ": " + contentDigest(files[0].Want) + "\n"
	}
	var b strings.Builder
	for _, f := range files {
		b.WriteString(trailerContent + ": " + contentDigest(f.Want) + " " + f.Path + "\n")
	}
	return b.String()
}

// commitFiles writes each file's bytes in dir and commits those paths
// alone on branch, with the stage trailers; it returns the verified
// commit. pre is the tip the stage rendered the files from: a branch that
// has moved since is refused. A rejected commit makes no commit: the paths
// are unstaged and their prior bytes restored (new files removed). A commit
// that landed and fails verification stays, named in the error. Every path
// the commit touched, a hook's included, has its index entry reset to
// HEAD's.
func (g gitOps) commitFiles(ctx context.Context, dir, branch string, files []fileWrite, subject, label, pre string) (string, error) {
	paths := pathsOf(files)
	if len(paths) == 0 {
		return "", errors.New("no file to commit")
	}
	for i, p := range paths {
		switch {
		case strings.ContainsAny(p, "\n\r") || strings.TrimSpace(p) != p:
			return "", fmt.Errorf("path %q has a line break or edge whitespace", p)
		case p == "" || filepath.IsAbs(p) || !filepath.IsLocal(p):
			return "", fmt.Errorf("path %q is not a relative path inside the repository", p)
		case slices.Contains(paths[:i], p):
			return "", fmt.Errorf("path %q is given twice", p)
		}
	}
	if cur, err := g.head(ctx, dir); err != nil {
		return "", err
	} else if cur != pre {
		return "", fmt.Errorf("%s moved from %s to %s while the stage ran; relaunch", branch, pre, cur)
	}
	if pre != "" {
		// Every path must change: a commit missing one would fail
		// verification after it landed.
		for _, f := range files {
			if b, ok, err := g.blob(ctx, dir, pre, f.Path); err != nil {
				return "", err
			} else if ok && bytes.Equal(b, f.Want) {
				return "", fmt.Errorf("%s already holds this content at %s; nothing to commit", f.Path, pre)
			}
		}
	}
	type prior struct {
		b       []byte
		existed bool
	}
	priors := []prior{}
	for _, f := range files {
		file := filepath.Join(dir, f.Path)
		b, err := os.ReadFile(file)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		priors = append(priors, prior{b, err == nil})
	}
	restore := func(n int) []error {
		var errs []error
		for i, f := range files[:n] {
			file := filepath.Join(dir, f.Path)
			if priors[i].existed {
				errs = append(errs, os.WriteFile(file, priors[i].b, 0o644))
			} else if e := os.Remove(file); e != nil && !errors.Is(e, os.ErrNotExist) {
				errs = append(errs, e)
			}
		}
		return errs
	}
	for i, f := range files {
		file := filepath.Join(dir, f.Path)
		err := os.MkdirAll(filepath.Dir(file), 0o755)
		if err == nil {
			err = os.WriteFile(file, f.Want, 0o644)
		}
		if err != nil {
			// The files written so far go back as they were.
			return "", errors.Join(append([]error{err}, restore(i)...)...)
		}
	}
	msg := subject + "\n\n" + trailerStep + ": " + label + "\n" + contentTrailers(files)
	_, err := g.r.run(ctx, dir, append([]string{"add", "--"}, paths...)...)
	if err == nil {
		_, err = g.r.run(ctx, dir, append([]string{"commit", "-q", "-m", msg, "--"}, paths...)...)
	}
	sha, headErr := g.head(context.WithoutCancel(ctx), dir)
	// A hook's own `git add` reaches the commit through its temporary
	// index; resetting every path the commit touched leaves the real index
	// as HEAD has it.
	reset := slices.Clone(paths)
	if headErr == nil && sha != pre {
		if touched, cerr := g.changed(context.WithoutCancel(ctx), dir, sha); cerr == nil {
			for _, p := range touched {
				if !slices.Contains(reset, p) {
					reset = append(reset, p)
				}
			}
		}
	}
	_, resetErr := g.r.run(context.WithoutCancel(ctx), dir, append([]string{"reset", "-q", "--"}, reset...)...)
	if err != nil && headErr == nil && sha == pre {
		// No commit landed: put the files back.
		return "", errors.Join(append([]error{fmt.Errorf("commit of %s rejected: %w", strings.Join(paths, " and "), err), resetErr}, restore(len(files))...)...)
	}
	if headErr != nil {
		return "", errors.Join(err, headErr)
	}
	// A commit landed, even if git reported an error after it (a
	// post-commit hook): it is checked like any other.
	if err := g.verifySet(ctx, dir, branch, sha, pre, files); err != nil {
		return "", errors.Join(err, resetErr)
	}
	for _, p := range paths {
		if _, ok, err := g.intact(ctx, dir, sha, p); err != nil || !ok {
			return "", errors.Join(fmt.Errorf("commit %s does not carry the %s trailer of its content (a commit-msg hook may have rewritten the message)", sha, trailerContent), err, resetErr)
		}
	}
	if resetErr != nil {
		return "", fmt.Errorf("commit %s: %w", sha, resetErr)
	}
	return sha, nil
}

// pathClean reports whether path has no staged or unstaged change.
func (g gitOps) pathClean(ctx context.Context, dir, path string) (bool, error) {
	s, err := g.out(ctx, dir, "status", "--porcelain", "--untracked-files=all", "--", path)
	return s == "", err
}
