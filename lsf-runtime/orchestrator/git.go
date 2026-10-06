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
	"strings"
	"time"
)

// gitRunner runs one git command in dir and returns its stdout; stderr is
// folded into the error. Tests decorate it to rewrite chosen outputs.
type gitRunner interface {
	run(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// gitStageTimeout bounds one code-stage git command. A commit runs the
// project's hooks, which may run its tests.
const gitStageTimeout = 30 * time.Minute

type execGit struct{}

func (execGit) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitStageTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return out.Bytes(), &gitError{args: args, err: err, stderr: strings.TrimSpace(stderr.String())}
	}
	return out.Bytes(), nil
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

// trailer is a commit's last value for key, "" when it has none.
func (g gitOps) trailer(ctx context.Context, dir, sha, key string) (string, error) {
	s, err := g.out(ctx, dir, "log", "-1", "--format=%(trailers:key="+key+",valueonly)", sha, "--")
	if err != nil {
		return "", err
	}
	lines := strings.Split(s, "\n")
	return strings.TrimSpace(lines[len(lines)-1]), nil
}

// intact reports whether sha carries Lsf-Content and its blob at path
// hashes to it; stage reports whether it carries Lsf-Content at all.
func (g gitOps) intact(ctx context.Context, dir, sha, path string) (stage, intact bool, err error) {
	want, err := g.trailer(ctx, dir, sha, trailerContent)
	if err != nil || want == "" {
		return false, false, err
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
// that is intact, carries this step's label, matches its parent and whose
// effect survives at the tip; "" when none does. The SHA is checked
// before it is returned (checkReachable).
func (g gitOps) adopt(ctx context.Context, dir, branch, path string, a adoption) (string, error) {
	ref := "refs/heads/" + branch
	tip, tipOK, err := g.blob(ctx, dir, ref, path)
	if err != nil {
		return "", err
	}
	if !a.survives(tip, tipOK) {
		return "", nil
	}
	list, err := g.out(ctx, dir, "log", "--first-parent", "-n", fmt.Sprint(adoptionWalk), "--format=%H", ref, "--", path)
	if err != nil {
		return "", err
	}
	for _, c := range strings.Fields(list) {
		if _, ok, err := g.intact(ctx, dir, c, path); err != nil || !ok {
			if err != nil {
				return "", err
			}
			continue
		}
		if l, err := g.trailer(ctx, dir, c, trailerStep); err != nil || l != a.label {
			if err != nil {
				return "", err
			}
			continue
		}
		cand, _, err := g.blob(ctx, dir, c, path)
		if err != nil {
			return "", err
		}
		parent, parentOK, err := g.blob(ctx, dir, c+"^", path)
		if err != nil && exitCode(err) != 128 { // a root commit has no c^
			return "", err
		}
		if !a.matches(parent, parentOK, cand) {
			continue
		}
		return c, g.checkReachable(ctx, dir, c, branch, path, true)
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
	b, err := g.r.run(ctx, dir, "diff-tree", "--root", "--no-commit-id", "--name-only", "-r", "-z", sha)
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

// verifyCommit checks a commit the stage has just made: a commit, whose
// only parent is pre (none when pre is ""), changing exactly path, at the
// tip of branch, whose blob at path is want byte for byte.
func (g gitOps) verifyCommit(ctx context.Context, dir, branch, sha, pre, path string, want []byte) error {
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
	if len(files) != 1 || files[0] != path {
		return bad(fmt.Sprintf("changes %q, want only %s", files, path))
	}
	tip, err := g.out(ctx, dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if err != nil || tip != sha {
		return bad("not the tip of " + branch)
	}
	b, ok, err := g.blob(ctx, dir, sha, path)
	if err != nil {
		return err
	}
	if !ok || !bytes.Equal(b, want) {
		return bad(path + " differs from the content written")
	}
	return nil
}

// writeAndCommit writes want to path in dir and commits path alone on
// branch, with the stage trailers; it returns the verified commit. A
// rejected commit makes no commit: path is unstaged and its prior bytes
// restored (a new file removed). A commit that fails verification stays,
// named in the error.
func (g gitOps) writeAndCommit(ctx context.Context, dir, branch, path, subject, label string, want []byte) (string, error) {
	pre, err := g.head(ctx, dir)
	if err != nil {
		return "", err
	}
	file := filepath.Join(dir, path)
	prior, readErr := os.ReadFile(file)
	existed := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return "", readErr
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(file, want, 0o644); err != nil {
		return "", err
	}
	msg := subject + "\n\n" + trailerStep + ": " + label + "\n" + trailerContent + ": " + contentDigest(want) + "\n"
	_, err = g.r.run(ctx, dir, "add", "--", path)
	if err == nil {
		_, err = g.r.run(ctx, dir, "commit", "-q", "-m", msg, "--", path)
	}
	// A hook's own `git add` reaches the commit through its temporary
	// index; resetting path's entry leaves the real index as HEAD has it.
	_, resetErr := g.r.run(context.WithoutCancel(ctx), dir, "reset", "-q", "--", path)
	if err != nil {
		if existed {
			os.WriteFile(file, prior, 0o644)
		} else {
			os.Remove(file)
		}
		return "", fmt.Errorf("commit of %s rejected: %w", path, err)
	}
	sha, err := g.head(ctx, dir)
	if err != nil {
		return "", err
	}
	if err := g.verifyCommit(ctx, dir, branch, sha, pre, path, want); err != nil {
		return "", err
	}
	if resetErr != nil {
		return "", fmt.Errorf("commit %s: %w", sha, resetErr)
	}
	return sha, nil
}

// pathClean reports whether path has no staged or unstaged change.
func (g gitOps) pathClean(ctx context.Context, dir, path string) (bool, error) {
	s, err := g.out(ctx, dir, "status", "--porcelain", "--", path)
	return s == "", err
}
