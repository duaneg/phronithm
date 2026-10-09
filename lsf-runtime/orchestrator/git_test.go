package orchestrator

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var twoFiles = []fileWrite{{"docs/plan.md", []byte("plan v2\n")}, {"docs/archive.md", []byte("archive v1\n")}}

// A two-path commit: one parent, exactly the two paths, each blob as
// written, a per-path Lsf-Content trailer each; both paths are intact and
// guard passes on each, until a later trailer-bearing commit alters one.
func TestCommitFilesTwoPaths(t *testing.T) {
	r := newGitRepo(t)
	r.commit("docs/plan.md", "plan v1\n", "plan")
	pre := r.head()
	g := gitOps{execGit{}}
	ctx := context.Background()
	sha, err := g.commitFiles(ctx, r.dir, "integration", twoFiles, "docs(f): bookkeeping", "merge:w1", pre)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, r.git("log", "-1", "--format=%P", sha), pre)
	eq(t, r.git("show", "--name-only", "--format=", sha), "docs/archive.md\ndocs/plan.md")
	vs, err := g.trailers(ctx, r.dir, sha, trailerContent)
	if err != nil || len(vs) != 2 || !strings.HasSuffix(vs[0], " docs/plan.md") || !strings.HasSuffix(vs[1], " docs/archive.md") {
		t.Fatalf("trailers %q %v", vs, err)
	}
	for _, p := range []string{"docs/plan.md", "docs/archive.md"} {
		if _, ok, err := g.intact(ctx, r.dir, sha, p); err != nil || !ok {
			t.Errorf("%s not intact: %v", p, err)
		}
		if err := g.guard(ctx, r.dir, "integration", p); err != nil {
			t.Errorf("guard %s: %v", p, err)
		}
	}
	eq(t, r.git("status", "--porcelain"), "")
	if err := g.verifySet(ctx, r.dir, "integration", sha, pre, twoFiles[:1]); err == nil || !strings.Contains(err.Error(), "want only docs/plan.md") {
		t.Errorf("a one-path check of a two-path commit: %v", err)
	}

	// A later stage commit damaging the archive: guard refuses on it, not
	// on the plan.
	r.write("docs/archive.md", "damaged\n")
	r.git("add", "docs/archive.md")
	r.git("commit", "-q", "-m", "x\n\nLsf-Step: other\nLsf-Content: sha256:00 docs/archive.md\n")
	errMatch(t, g.guard(ctx, r.dir, "integration", "docs/archive.md"), `docs/archive\.md with content its Lsf-Content trailer does not match`)
	if err := g.guard(ctx, r.dir, "integration", "docs/plan.md"); err != nil {
		t.Errorf("guard plan: %v", err)
	}
}

// A single-path commit keeps the bare trailer, which intact still reads.
func TestCommitFilesOnePathTrailer(t *testing.T) {
	r := newGitRepo(t)
	g := gitOps{execGit{}}
	sha, err := g.writeAndCommit(context.Background(), r.dir, "integration", "a.md", "s", "l", []byte("A\n"), r.head())
	if err != nil {
		t.Fatal(err)
	}
	eq(t, r.git("log", "-1", "--format=%(trailers:key=Lsf-Content,valueonly)", sha), contentDigest([]byte("A\n")))
}

// A rejected commit restores both files (the new one removed) and leaves
// the index clean.
func TestCommitFilesRejected(t *testing.T) {
	r := newGitRepo(t)
	r.commit("docs/plan.md", "plan v1\n", "plan")
	pre := r.head()
	r.hook("pre-commit", "exit 1")
	_, err := gitOps{execGit{}}.commitFiles(context.Background(), r.dir, "integration", twoFiles, "s", "merge:w1", pre)
	errMatch(t, err, `commit of docs/plan\.md and docs/archive\.md rejected`)
	eq(t, r.head(), pre)
	eq(t, r.read("docs/plan.md"), "plan v1\n")
	if _, err := os.Stat(filepath.Join(r.dir, "docs/archive.md")); !os.IsNotExist(err) {
		t.Errorf("new file left: %v", err)
	}
	eq(t, r.git("status", "--porcelain"), "")
}

// A pre-commit hook that stages another path is caught by verification,
// which names the commit; afterwards the index matches HEAD for every
// path, the hook's included (done condition).
func TestCommitFilesStagingHook(t *testing.T) {
	r := newGitRepo(t)
	r.commit("docs/plan.md", "plan v1\n", "plan")
	pre := r.head()
	r.write("wip.md", "requester work\n")
	r.hook("pre-commit", "git add wip.md")
	_, err := gitOps{execGit{}}.commitFiles(context.Background(), r.dir, "integration", twoFiles, "s", "merge:w1", pre)
	errMatch(t, err, `commit [0-9a-f]{40} fails verification: changes .*wip\.md`)
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = r.dir
	if err := cmd.Run(); err != nil {
		t.Errorf("the index differs from HEAD: %s", r.git("diff", "--cached", "--name-status"))
	}
	eq(t, r.git("status", "--porcelain"), "", "nothing staged or modified")
}

// adoptSet adopts its own intact two-path commit and nothing else: not
// another label's, not one changing another set, not one matches refuses.
func TestAdoptSet(t *testing.T) {
	r := newGitRepo(t)
	r.commit("docs/plan.md", "plan v1\n", "plan")
	g := gitOps{execGit{}}
	ctx := context.Background()
	sha, err := g.commitFiles(ctx, r.dir, "integration", twoFiles, "s", "merge:w1", r.head())
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"docs/plan.md", "docs/archive.md"}
	ours := func(parent, cand map[string]blobAt) bool {
		return parent["docs/plan.md"].ok && !parent["docs/archive.md"].ok && bytes.Equal(cand["docs/plan.md"].b, []byte("plan v2\n"))
	}
	got, err := g.adoptSet(ctx, r.dir, "integration", paths, "merge:w1", ours)
	if err != nil || got != sha {
		t.Errorf("own commit: %q %v", got, err)
	}
	for name, try := range map[string]func() (string, error){
		"other label": func() (string, error) { return g.adoptSet(ctx, r.dir, "integration", paths, "merge:w2", ours) },
		"other set":   func() (string, error) { return g.adoptSet(ctx, r.dir, "integration", paths[:1], "merge:w1", ours) },
		"not matching": func() (string, error) {
			return g.adoptSet(ctx, r.dir, "integration", paths, "merge:w1", func(_, _ map[string]blobAt) bool { return false })
		},
	} {
		if got, err := try(); err != nil || got != "" {
			t.Errorf("%s: adopted %q %v", name, got, err)
		}
	}
}

// A hook that deletes a tracked path is caught, and the index still
// matches HEAD afterwards.
func TestCommitFilesDeletingHook(t *testing.T) {
	r := newGitRepo(t)
	r.commit("docs/plan.md", "plan v1\n", "plan")
	r.commit("other.md", "other\n", "other")
	pre := r.head()
	r.hook("pre-commit", "git rm -q other.md")
	_, err := gitOps{execGit{}}.commitFiles(context.Background(), r.dir, "integration", twoFiles, "s", "merge:w1", pre)
	errMatch(t, err, `fails verification: changes .*other\.md`)
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = r.dir
	if err := cmd.Run(); err != nil {
		t.Errorf("the index differs from HEAD: %s", r.git("diff", "--cached", "--name-status"))
	}
}

// Refusals before anything is written or committed: a path already
// holding its bytes, an empty set, a duplicate, an escaping or
// whitespace-edged path. A write failing mid-set puts back the files
// written before it.
func TestCommitFilesRefusals(t *testing.T) {
	r := newGitRepo(t)
	r.commit("docs/plan.md", "plan v2\n", "plan")
	pre := r.head()
	g := gitOps{execGit{}}
	ctx := context.Background()
	commit := func(files []fileWrite) error {
		_, err := g.commitFiles(ctx, r.dir, "integration", files, "s", "l", pre)
		return err
	}
	errMatch(t, commit(twoFiles), `docs/plan\.md already holds this content`)
	errMatch(t, commit(nil), `no file to commit`)
	errMatch(t, commit([]fileWrite{{"a.md", []byte("1")}, {"a.md", []byte("2")}}), `given twice`)
	errMatch(t, commit([]fileWrite{{"../x.md", []byte("1")}}), `not a relative path`)
	errMatch(t, commit([]fileWrite{{"/tmp/x.md", []byte("1")}}), `not a relative path`)
	errMatch(t, commit([]fileWrite{{"x.md ", []byte("1")}}), `edge whitespace`)
	eq(t, r.head(), pre)

	r.write("README.md", "requester draft\n")
	errMatch(t, commit([]fileWrite{{"README.md", []byte("ours\n")}, {"README.md/x", []byte("y")}}), `not a directory`)
	eq(t, r.read("README.md"), "requester draft\n", "the first file is put back")
	eq(t, r.head(), pre)
}

// A bare digest covers the path its commit changed, not another path
// holding the same bytes.
func TestIntactBareDigestScope(t *testing.T) {
	r := newGitRepo(t)
	g := gitOps{execGit{}}
	ctx := context.Background()
	r.commit("copy.md", "A\n", "same bytes, no trailer")
	sha, err := g.writeAndCommit(ctx, r.dir, "integration", "a.md", "s", "l", []byte("A\n"), r.head())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := g.intact(ctx, r.dir, sha, "a.md"); err != nil || !ok {
		t.Errorf("a.md: %v", err)
	}
	if _, ok, err := g.intact(ctx, r.dir, sha, "copy.md"); err != nil || ok {
		t.Errorf("copy.md is intact through a commit that did not touch it: %v", err)
	}
	if err := g.guard(ctx, r.dir, "integration", "a.md"); err != nil {
		t.Errorf("guard: %v", err)
	}
}

func TestDirSet(t *testing.T) {
	var none *dirSet
	none.add("/a")
	if none.has("/a") {
		t.Error("a nil set holds nothing")
	}
	s := &dirSet{}
	s.add("/state/merge/w1/")
	for dir, want := range map[string]bool{"/state/merge/w1": true, "/state/merge/w1/sub/x": true, "/state/merge/w10": false, "/state/merge": false, "/": false} {
		if s.has(dir) != want {
			t.Errorf("has(%s) = %v", dir, !want)
		}
	}
}
