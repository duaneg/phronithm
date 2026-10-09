package lsfrt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Every agent call runs in its own plain clone of the main repo under the
// state dir (docs/design/lsf-runtime.md, Permissions → Placement), never in
// the main checkout or a worktree, whose .git is the main repo's:
//
//	clone/<key>   a kept clone, fetched back after each return
//	scratch/<key> a discarded clone, removed after the call; wiped at launch
//	merge/<name>  a merge clone (MergeClone), the merge step's
//
// A clone is made with --no-local, so no object file is hardlinked to the
// main repo's, and has every main branch as a local branch, the main repo's
// effective hooks, its identity and line-ending config, and its
// info/exclude and info/attributes. It has no remote.

// placement is where a call runs.
type placement struct {
	dir    string // the call's cwd: a clone, or without a repo an empty scratch directory
	git    bool   // dir is a clone (a merge clone included)
	kind   string // CloneKept, CloneDiscarded, or "" for no clone the call owns
	branch string // the branch the clone has checked out
	dest   string // the main repo's branch a kept clone is fetched to
}

// configCopied are the config keys a clone takes from the main repo's
// effective config (local or global): the identity and signing, so a commit
// made in the clone is attributed and signed as one made in the main
// checkout, and the line-ending normalisation, so the work tree matches.
// The runtime's own git in a clone reads no global config, so whatever it
// needs from there is here.
var configCopied = []string{"user.name", "user.email", "core.autocrlf", "core.eol", "core.safecrlf", "commit.gpgsign", "tag.gpgsign", "user.signingkey", "gpg.format",
	"gpg.program", "gpg.openpgp.program", "gpg.x509.program", "gpg.ssh.program", "gpg.ssh.defaultKeyCommand"}

// usesClone reports whether a call with these options runs in a clone the
// runtime makes.
func (rt *Runtime) usesClone(o Opts) bool {
	return o.Dir == "" && rt.cfg.Repo != ""
}

// placeCall resolves a call's working directory at its first admitted
// attempt: an explicit Dir under merge/, or a clone by the role's kind. A
// failure is a runtime-error stop, never a fallback to the main checkout.
func (rt *Runtime) placeCall(ctx context.Context, o Opts, key string, role Role) (placement, error) {
	switch {
	case o.Dir != "":
		if o.Isolation != "" {
			return placement{}, rt.setStop(StopRuntimeError, o.Label+": opts.dir and opts.isolation are exclusive")
		}
		dir, err := rt.mergeDirOf(o.Dir)
		if err != nil {
			return placement{}, rt.setStop(StopRuntimeError, o.Label+": "+err.Error())
		}
		return placement{dir: dir, git: true}, nil
	case rt.cfg.Repo == "":
		if o.Isolation != "" {
			_, err := rt.cloneArgs(o)
			return placement{}, err
		}
		// The runtime's own tests only: an empty directory, never the
		// launcher's.
		dir := filepath.Join(rt.cfg.StateDir, "scratch", key)
		if err := os.RemoveAll(dir); err != nil {
			return placement{}, rt.setStop(StopRuntimeError, fmt.Sprintf("%s: %v", o.Label, err))
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return placement{}, rt.setStop(StopRuntimeError, fmt.Sprintf("%s: %v", o.Label, err))
		}
		return placement{dir: dir}, nil
	}
	args, err := rt.cloneArgs(o)
	if err != nil {
		return placement{}, err
	}
	p := placement{git: true, kind: role.Clone, dest: branchName(args, key), branch: args.IntegrationBranch}
	if o.Isolation != "" {
		p.branch = p.dest
	}
	sub := "clone"
	if p.kind == CloneDiscarded {
		sub = "scratch"
	}
	p.dir = filepath.Join(rt.cfg.StateDir, sub, key)
	// The call is admitted, so it spawns: the wait does not yield to a stop
	// set since, only to the context.
	select {
	case rt.wtSem <- struct{}{}:
	case <-ctx.Done():
		return placement{}, &StopError{Reason: StopCancelled}
	}
	defer func() { <-rt.wtSem }()
	gctx, cancel := rt.gitContext(ctx)
	defer cancel()
	stop := func(err error) (placement, error) {
		if ctx.Err() != nil {
			return placement{}, &StopError{Reason: StopCancelled}
		}
		return placement{}, rt.setStop(StopRuntimeError, fmt.Sprintf("%s: %v", o.Label, err))
	}
	if p.kind == CloneDiscarded {
		if err := os.RemoveAll(p.dir); err != nil {
			return stop(err)
		}
	} else if _, err := os.Lstat(p.dir); err == nil {
		fresh, err := rt.reuseClone(gctx, o.Label, p, args.IntegrationBranch)
		if err != nil {
			return stop(err)
		}
		if !fresh {
			return p, nil
		}
	}
	if err := rt.makeClone(gctx, p.dir, p.branch, args.IntegrationBranch); err != nil {
		return stop(err)
	}
	rt.event("clone", map[string]any{"label": o.Label, "path": p.dir, "branch": p.branch, "clone": p.kind})
	return p, nil
}

// reuseClone checks an existing kept clone and refreshes its other branches
// from the main repo. fresh reports that it was stale and is gone: a
// repo-call clone whose HEAD no longer descends from the main repo's
// integration tip. The caller holds wtSem.
func (rt *Runtime) reuseClone(ctx context.Context, label string, p placement, integration string) (fresh bool, err error) {
	if err := isCloneOn(ctx, p.dir, p.branch); err != nil {
		return false, fmt.Errorf("%s exists but %v; remove it and relaunch", p.dir, err)
	}
	if p.branch == integration {
		if b, err := cloneOut(ctx, p.dir, "fetch", "--no-tags", "-q", rt.cfg.Repo, "refs/heads/"+integration); err != nil {
			return false, fmt.Errorf("fetch %s into %s: %v: %s", integration, p.dir, err, strings.TrimSpace(string(b)))
		}
		switch err := cloneRun(ctx, p.dir, "merge-base", "--is-ancestor", "FETCH_HEAD", "HEAD"); {
		case err == nil:
		case exitStatus(err) == 1:
			// Only a clone whose HEAD came back, with nothing uncommitted,
			// may go: a failed fetch-back or a null's dirt would be lost.
			head, herr := cloneStdout(ctx, p.dir, "rev-parse", "HEAD")
			fetched, ferr := gitStdout(ctx, rt.cfg.Repo, "rev-parse", "-q", "--verify", "refs/heads/"+p.dest)
			dirt, derr := cloneStdout(ctx, p.dir, "status", "--porcelain", "--untracked-files=normal")
			if herr != nil || ferr != nil || derr != nil || strings.TrimSpace(string(head)) != strings.TrimSpace(string(fetched)) || len(dirt) > 0 {
				return false, fmt.Errorf("%s is stale (%s moved since) but holds commits not fetched to %s or uncommitted changes; recover them, remove it and relaunch", p.dir, integration, p.dest)
			}
			rt.Log(fmt.Sprintf("%s: clone %s is stale (%s moved since); recreated, abandoning its HEAD %s", label, p.dir, integration, strings.TrimSpace(string(head))))
			rt.event("clone-stale", map[string]any{"label": label, "path": p.dir, "head": strings.TrimSpace(string(head))})
			return true, os.RemoveAll(p.dir)
		default:
			return false, fmt.Errorf("merge-base in %s: %v", p.dir, err)
		}
	}
	if b, err := cloneOut(ctx, p.dir, "fetch", "--no-tags", "-q", rt.cfg.Repo, "+refs/heads/*:refs/heads/*", "^refs/heads/"+p.branch); err != nil {
		return false, fmt.Errorf("refresh %s: %v: %s", p.dir, err, strings.TrimSpace(string(b)))
	}
	return false, nil
}

// isCloneOn is nil when dir is the top of a clone (its .git a directory,
// not a worktree's file) whose HEAD is on branch.
func isCloneOn(ctx context.Context, dir, branch string) error {
	if fi, err := os.Lstat(filepath.Join(dir, ".git")); err != nil || !fi.IsDir() {
		return errors.New("is not a clone")
	}
	top, err := cloneStdout(ctx, dir, "rev-parse", "--show-toplevel")
	resolved, _ := filepath.EvalSymlinks(dir)
	if err != nil || strings.TrimSpace(string(top)) != resolved {
		return errors.New("is not a clone's top")
	}
	head, err := cloneStdout(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD")
	if h := strings.TrimSpace(string(head)); err != nil || h != branch {
		return fmt.Errorf("is not on %s", branch)
	}
	return nil
}

// makeClone creates a clone at dir with branch checked out: the main repo's
// branch when it has one, else a new branch from integration. A failure
// removes the partial directory. The caller holds wtSem.
func (rt *Runtime) makeClone(ctx context.Context, dir, branch, integration string) (err error) {
	// Built at dir.partial and renamed into place, so a kill mid-build
	// leaves no directory that a relaunch would refuse to reuse; the launch
	// removes a .partial left by one. The setup is copied before the
	// checkout: the line-ending config shapes the work tree, and the hooks
	// path, set to dir's own, does not yet exist, so no hook runs then.
	tmp := dir + partialSuffix
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(tmp)
		}
	}()
	run := func(in string, args ...string) error {
		if b, err := cloneOut(ctx, in, args...); err != nil {
			return fmt.Errorf("git %s: %v: %s", strings.Join(args[:min(2, len(args))], " "), err, strings.TrimSpace(string(b)))
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	if err := run(rt.cfg.StateDir, "clone", "--no-local", "--no-checkout", "--no-tags", "-q", "--", rt.cfg.Repo, tmp); err != nil {
		return err
	}
	if err := run(tmp, "fetch", "--no-tags", "--update-head-ok", "-q", rt.cfg.Repo, "+refs/heads/*:refs/heads/*"); err != nil {
		return err
	}
	if err := run(tmp, "remote", "remove", "origin"); err != nil {
		return err
	}
	if err := rt.copyRepoSetup(ctx, tmp, dir); err != nil {
		return err
	}
	if cloneRun(ctx, tmp, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch) == nil {
		err = run(tmp, "checkout", "-q", branch, "--")
	} else {
		err = run(tmp, "checkout", "-q", "-b", branch, integration, "--")
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, dir)
}

// partialSuffix marks a clone being built.
const partialSuffix = ".partial"

// copyRepoSetup gives a new clone at dir, to be renamed to final, the main
// repo's effective hooks, the configCopied values, its info/exclude and
// info/attributes, and a copy of each file its core.excludesFile and
// core.attributesFile name: the clone's own git reads no global config.
func (rt *Runtime) copyRepoSetup(ctx context.Context, dir, final string) error {
	mainPath := func(p string) (string, error) {
		b, err := gitStdout(ctx, rt.cfg.Repo, "rev-parse", "--git-path", p)
		if err != nil {
			return "", fmt.Errorf("rev-parse --git-path %s: %w", p, err)
		}
		s := strings.TrimSpace(string(b))
		if !filepath.IsAbs(s) {
			s = filepath.Join(rt.cfg.Repo, s)
		}
		return s, nil
	}
	hooks, err := mainPath("hooks")
	if err != nil {
		return err
	}
	dst := filepath.Join(dir, ".git", "hooks")
	if err := copyHooks(hooks, dst); err != nil {
		return err
	}
	// The clone runs the copies whatever a global core.hooksPath says.
	if b, err := cloneOut(ctx, dir, "config", "core.hooksPath", filepath.Join(final, ".git", "hooks")); err != nil {
		return fmt.Errorf("config core.hooksPath: %v: %s", err, strings.TrimSpace(string(b)))
	}
	for _, k := range configCopied {
		v, err := gitStdout(ctx, rt.cfg.Repo, "config", "--get", k)
		if exitStatus(err) == 1 {
			continue // unset
		}
		if err != nil {
			return fmt.Errorf("config --get %s: %w", k, err)
		}
		if b, err := cloneOut(ctx, dir, "config", k, strings.TrimSuffix(string(v), "\n")); err != nil {
			return fmt.Errorf("config %s: %v: %s", k, err, strings.TrimSpace(string(b)))
		}
	}
	for _, f := range []string{"info/exclude", "info/attributes"} {
		src, err := mainPath(f)
		if err != nil {
			return err
		}
		b, err := readIfExists(src)
		if err != nil {
			return err
		}
		if b != nil {
			if err := writeGitFile(dir, f, b); err != nil {
				return err
			}
		}
	}
	// The global excludes and attributes files keep their place in git's
	// precedence, below .gitignore and .gitattributes: a copy in the clone
	// set as its local core.excludesFile and core.attributesFile, so the
	// clone needs no read of the user's files.
	for _, f := range []struct{ key, name string }{{"core.excludesFile", "excludes"}, {"core.attributesFile", "attributes"}} {
		v, err := gitStdout(ctx, rt.cfg.Repo, "config", "--type=path", "--get", f.key)
		if exitStatus(err) == 1 {
			continue // unset: git reads its default file in the clone as in the main repo
		}
		if err != nil {
			return fmt.Errorf("config --get %s: %w", f.key, err)
		}
		b, err := readIfExists(strings.TrimSuffix(string(v), "\n"))
		if err != nil {
			return err
		}
		rel := "lsf-global/" + f.name
		if err := writeGitFile(dir, rel, b); err != nil {
			return err
		}
		if out, err := cloneOut(ctx, dir, "config", f.key, filepath.Join(final, ".git", filepath.FromSlash(rel))); err != nil {
			return fmt.Errorf("config %s: %v: %s", f.key, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// writeGitFile writes b to rel under dir's .git directory.
func writeGitFile(dir, rel string, b []byte) error {
	d := filepath.Join(dir, ".git", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(d), 0o755); err != nil {
		return err
	}
	return os.WriteFile(d, b, 0o644)
}

// readIfExists is p's content, nil when it does not exist.
func readIfExists(p string) ([]byte, error) {
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// copyHooks copies src's hooks recursively (a hook may source a sibling
// in a subdirectory, as husky's do), not the *.sample files, keeping
// modes. Symlinks are followed.
func copyHooks(src, dst string) error { return copyTree(src, dst, 0) }

func copyTree(src, dst string, depth int) error {
	if depth > 8 { // a symlink back up the tree
		return fmt.Errorf("hooks: %s is nested too deep", src)
	}
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sample") {
			continue
		}
		s, d := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		fi, err := os.Stat(s)
		switch {
		case err != nil:
			continue
		case fi.IsDir():
			err = copyTree(s, d, depth+1)
		case fi.Mode().IsRegular():
			err = copyFile(s, d, fi.Mode().Perm())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode) // OpenFile's mode is masked by the umask
}

// fetchBack fetches a kept clone's branch into the main repo as the call's
// branch, without tags, and records the clone's HEAD. A clone HEAD off its
// branch is logged: the branch, not HEAD, is what comes back. The error
// names the clone. It runs after a first signal too, as a call that
// returns then is journaled; a repeated signal ends its git.
func (rt *Runtime) fetchBack(ctx context.Context, label string, p placement, res *CallResult) error {
	rt.wtSem <- struct{}{}
	defer func() { <-rt.wtSem }()
	gctx, cancel := rt.gitContext(ctx)
	defer cancel()
	was, _ := gitStdout(gctx, rt.cfg.Repo, "rev-parse", "-q", "--verify", "refs/heads/"+p.dest)
	defer func() {
		now, _ := gitStdout(gctx, rt.cfg.Repo, "rev-parse", "-q", "--verify", "refs/heads/"+p.dest)
		if w, n := strings.TrimSpace(string(was)), strings.TrimSpace(string(now)); w != "" && w != n && res.Replayed {
			rt.event("branch-refetched", map[string]any{"label": label, "branch": p.dest, "from": w, "to": n})
			rt.Log(fmt.Sprintf("%s: the replay moved %s from %s to %s, its clone's tip", label, p.dest, w, n))
		}
	}()
	if b, err := gitOut(gctx, rt.cfg.Repo, "fetch", "--no-tags", "-q", p.dir, "+refs/heads/"+p.branch+":refs/heads/"+p.dest); err != nil {
		return fmt.Errorf("fetching %s from its clone %s: %v: %s", p.branch, p.dir, err, strings.TrimSpace(string(b)))
	}
	res.Branch = p.dest
	head, err := cloneStdout(gctx, p.dir, "rev-parse", "-q", "--verify", "HEAD")
	res.CloneHead = strings.TrimSpace(string(head))
	on, _ := cloneStdout(gctx, p.dir, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil || strings.TrimSpace(string(on)) != p.branch {
		rt.event("clone-head-off-branch", map[string]any{"label": label, "path": p.dir, "branch": p.branch, "head": res.CloneHead})
		rt.Log(fmt.Sprintf("%s: clone %s HEAD %s is not on %s; %s was fetched as it stands", label, p.dir, res.CloneHead, p.branch, p.branch))
	}
	return nil
}

// discard removes a discarded clone after its call. A failure is logged:
// the next launch's wipe of scratch/ removes it.
func (rt *Runtime) discard(label string, p placement) {
	if err := os.RemoveAll(p.dir); err != nil {
		rt.Log(fmt.Sprintf("%s: discarded clone %s not removed: %v", label, p.dir, err))
	}
}

// mergeDirOf is dir resolved, when it is an existing directory under
// <stateDir>/merge.
func (rt *Runtime) mergeDirOf(dir string) (string, error) {
	base, err := filepath.EvalSymlinks(filepath.Join(rt.cfg.StateDir, "merge"))
	if err != nil {
		return "", err
	}
	d, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("opts.dir: %w", err)
	}
	rel, err := filepath.Rel(base, d)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("opts.dir %s is not under %s", dir, base)
	}
	if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("opts.dir %s is not a directory", dir)
	}
	return d, nil
}

// MergeClone is the merge clone <stateDir>/merge/<name> on the integration
// branch: created, or the existing one when it is a clone on that branch.
// An existing clone is not refreshed, since a merge may be in progress in
// it.
func (rt *Runtime) MergeClone(ctx context.Context, name string) (string, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("merge clone name %q", name)
	}
	args, err := rt.cloneArgs(Opts{Label: "merge clone " + name})
	if err != nil {
		return "", err
	}
	dir := filepath.Join(rt.cfg.StateDir, "merge", name)
	if err := rt.withWorktreeLock(ctx); err != nil {
		return "", err
	}
	defer func() { <-rt.wtSem }()
	gctx, cancel := rt.gitContext(ctx)
	defer cancel()
	if _, err := os.Lstat(dir); err == nil {
		if err := isCloneOn(gctx, dir, args.IntegrationBranch); err != nil {
			return "", fmt.Errorf("merge clone %s %v: remove it and relaunch", dir, err)
		}
		return dir, nil
	}
	if err := rt.makeClone(gctx, dir, args.IntegrationBranch, args.IntegrationBranch); err != nil {
		if s := rt.stopped(); s != nil {
			return "", s
		}
		return "", fmt.Errorf("merge clone %s: %w", dir, err)
	}
	rt.event("clone", map[string]any{"label": "merge:" + name, "path": dir, "branch": args.IntegrationBranch, "clone": "merge"})
	return dir, nil
}

// RemoveMergeClone removes <stateDir>/merge/<name>, if it exists.
func (rt *Runtime) RemoveMergeClone(ctx context.Context, name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return fmt.Errorf("merge clone name %q", name)
	}
	if err := rt.withWorktreeLock(ctx); err != nil {
		return err
	}
	defer func() { <-rt.wtSem }()
	return os.RemoveAll(filepath.Join(rt.cfg.StateDir, "merge", name))
}

// cloneArgs is isolationArgs for a call with or without isolation.
func (rt *Runtime) cloneArgs(o Opts) (worktreeArgs, error) {
	if o.Isolation == "" {
		o.Isolation = "worktree"
	}
	return rt.isolationArgs(o)
}

// parseArgs reads args without stopping, for a replay's check.
func (rt *Runtime) parseArgs() (worktreeArgs, error) {
	var args worktreeArgs
	if len(rt.cfg.Args) > 0 {
		if err := json.Unmarshal(rt.cfg.Args, &args); err != nil {
			return args, err
		}
	}
	if args.IntegrationBranch == "" || len(args.PlanSha) < 8 {
		return args, errors.New("args lack planSha or integrationBranch")
	}
	return args, nil
}

// checkRepo refuses a main repo clones are untested with: shallow, with
// submodules (.gitmodules at the integration tip, or HEAD without args), or
// using LFS (in its local config or its .gitattributes there).
func checkRepo(cfg Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	shallow, err := gitStdout(ctx, cfg.Repo, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return fmt.Errorf("repo %s is not a git repository: %v", cfg.Repo, err)
	}
	var errs []error
	if strings.TrimSpace(string(shallow)) == "true" {
		errs = append(errs, errors.New("is shallow"))
	}
	tip := "HEAD"
	var args worktreeArgs
	if json.Unmarshal(cfg.Args, &args) == nil && args.IntegrationBranch != "" && !strings.HasPrefix(args.IntegrationBranch, "-") {
		tip = "refs/heads/" + args.IntegrationBranch
	}
	// The repo's own use of LFS: its local config, or its .gitattributes.
	// A global `git lfs install` alone is not use.
	attrs, _ := gitStdout(ctx, cfg.Repo, "cat-file", "-p", tip+":.gitattributes")
	if gitRun(ctx, cfg.Repo, "config", "--local", "--get-regexp", `^filter\.lfs\.`) == nil || strings.Contains(string(attrs), "filter=lfs") {
		errs = append(errs, errors.New("uses LFS"))
	}
	if gitRun(ctx, cfg.Repo, "cat-file", "-e", tip+":.gitmodules") == nil {
		errs = append(errs, fmt.Errorf("has submodules (.gitmodules at %s)", tip))
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("repo %s: per-call clones are untested with a repo that %w", cfg.Repo, err)
	}
	return nil
}

// wipeScratch removes discarded clones, and clones half-built, that a
// crashed launch left. It runs after orphan reaping, so no live call loses
// its cwd.
func (rt *Runtime) wipeScratch() {
	dir := filepath.Join(rt.cfg.StateDir, "scratch")
	if err := os.RemoveAll(dir); err != nil {
		rt.Log("scratch: " + err.Error())
	}
	os.MkdirAll(dir, 0o755)
	for _, sub := range []string{"clone", "merge", "gates"} {
		partial, _ := filepath.Glob(filepath.Join(rt.cfg.StateDir, sub, "*"+partialSuffix))
		for _, p := range partial {
			if err := os.RemoveAll(p); err != nil {
				rt.Log("half-built clone: " + err.Error())
			}
		}
	}
}
