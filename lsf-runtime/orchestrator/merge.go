package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// The merge stage as code (D24): each unit's completion-report commit is
// merged into the integration branch in the wave's merge clone (D29), in
// plan merge order, a conflict going to a model call that stages a
// resolution there which code commits; then one commit records the
// bookkeeping (bookkeeping.go). After each verified commit the main repo
// fetches it and fast-forwards its integration branch (ff.go). Every commit
// carries trailers, so an interrupted rerun adopts its own commits instead
// of merging or recording twice; the merge clone stays until the wave's
// merges finish, so the rerun finds a merge it left in progress.

// mergeUnitIn is one unit to merge: its completion-report branch and the
// commit the completion report returned (a journaled value: nothing here
// needs the branch to resolve on a replay).
type mergeUnitIn struct {
	Unit   string `json:"unit"`
	Branch string `json:"branch"`
	Sha    string `json:"sha"`
	Report string `json:"report"`
	// GatedTree is the tree the runtime last gated for the unit (declared
	// gates only, D27): a merge that differs from it only at Report reuses
	// that result.
	GatedTree string `json:"gatedTree,omitempty"`
}

type mergeInput struct {
	Wave              int           `json:"wave"`
	Name              string        `json:"name"`
	IntegrationBranch string        `json:"integrationBranch"`
	PlanPath          string        `json:"planPath"`
	ArchivePath       string        `json:"archivePath"`
	LedgerPath        string        `json:"ledgerPath"`
	ArchitectureDoc   string        `json:"architectureDoc"`
	PauseRef          string        `json:"pauseRef"`
	Units             []mergeUnitIn `json:"units"`
	// Declared: the runtime gates each merge's tree before it is committed
	// (D27); a merge whose gates fail is regressed, with nothing committed.
	Declared bool `json:"declared,omitempty"`
}

// Merge commit trailers.
const (
	trailerUnit     = "Lsf-Unit"
	trailerMerge    = "Lsf-Merge"
	trailerConflict = "Lsf-Conflict"
)

func mergeMessage(in mergeInput, label, unit, status string, conflicts []string) string {
	var b strings.Builder
	b.WriteString("merge(" + in.Name + "): " + unit + "\n\n")
	b.WriteString(trailerStep + ": " + label + "\n" + trailerUnit + ": " + unit + "\n" + trailerMerge + ": " + status + "\n")
	for _, c := range conflicts {
		b.WriteString(trailerConflict + ": " + c + "\n")
	}
	return b.String()
}

// conflictResult is the conflict call's return.
type conflictResult struct {
	Status    string   `json:"status"`
	Files     []string `json:"files"`
	Statement string   `json:"statement"`
}

var conflictSchema = stage(object(obj{
	{"status", obj{{"type", "string"}, {"enum", []string{"resolved", "unresolvable"}}}},
	{"files", desc(strs, "The conflicted files, each resolved and staged, or the ones that could not be")},
	{"statement", desc(str, "One sentence: how the conflicts were resolved, or why they could not be")},
}))

// conflictMarker is a conflict marker line a resolution must not keep.
var conflictMarker = regexp.MustCompile(`(?m)^(<{7}|>{7}|\|{7})( |$)`)

// mergeFault is the merge step's own failure, with what it did so far.
type mergeFault struct{ reason string }

func (e *mergeFault) Error() string { return e.reason }

// mergeGit is the merge step's git in the merge clone dir; main is the
// main checkout, and xfer the main repo's ref a commit crosses by.
type mergeGit struct {
	gitOps
	dir        string
	main, xfer string
}

// toMain fetches sha from the merge clone into the main repo as m.xfer, so
// gates can check it out and the integration branch fast-forward to it.
func (m mergeGit) toMain(ctx context.Context, sha string) error {
	if m.main == "" || m.main == m.dir {
		return nil
	}
	if _, err := m.r.run(ctx, m.dir, "update-ref", "refs/lsf/xfer", sha); err != nil {
		return err
	}
	_, err := m.r.run(ctx, m.main, "fetch", "--no-tags", "-q", m.dir, "+refs/lsf/xfer:"+m.xfer)
	return err
}

// gitFile is a file under the git dir (MERGE_HEAD, MERGE_MSG), "" when
// absent.
func (m mergeGit) gitFile(ctx context.Context, name string) (string, error) {
	p, err := m.out(ctx, m.dir, "rev-parse", "--git-path", name)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(m.dir, p)
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}

// nulList is a -z command's paths.
func (m mergeGit) nulList(ctx context.Context, args ...string) ([]string, error) {
	b, err := m.r.run(ctx, m.dir, args...)
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

// mergeTree is git's merge of pre and sha computed without touching the
// checkout: its tree, and the paths that conflict.
func (m mergeGit) mergeTree(ctx context.Context, pre, sha string) (tree string, conflicts []string, err error) {
	b, err := m.r.run(ctx, m.dir, "merge-tree", "-z", "--write-tree", "--name-only", "--no-messages", pre, sha)
	if err != nil && exitCode(err) != 1 { // 1: conflicts
		return "", nil, err
	}
	// -z: the tree, then each conflicted path, NUL-terminated.
	fields := strings.Split(strings.TrimSpace(string(b)), "\x00")
	if len(fields) == 0 || fields[0] == "" {
		return "", nil, fmt.Errorf("git merge-tree %s %s printed no tree", pre, sha)
	}
	conflicts = []string{}
	for _, l := range fields[1:] {
		if l == "" {
			break
		}
		if !slices.Contains(conflicts, l) {
			conflicts = append(conflicts, l)
		}
	}
	return strings.TrimSpace(fields[0]), conflicts, nil
}

// expectedStaged is what a merge of sha onto pre may stage: the paths its
// merge tree changes against pre, and the conflicted paths. It is derived
// from the two commits alone, so a resume derives the same set.
func (m mergeGit) expectedStaged(ctx context.Context, pre, sha string) (staged, conflicts []string, tree string, err error) {
	tree, conflicts, err = m.mergeTree(ctx, pre, sha)
	if err != nil {
		return nil, nil, "", err
	}
	changed, err := m.nulList(ctx, "diff", "--name-only", "-z", "--no-renames", pre, tree)
	if err != nil {
		return nil, nil, "", err
	}
	staged = slices.Clone(conflicts)
	for _, p := range changed {
		if !slices.Contains(staged, p) {
			staged = append(staged, p)
		}
	}
	return staged, conflicts, tree, nil
}

// mergeTrailers are a merge commit's Lsf-Step, Lsf-Unit and Lsf-Merge
// trailers by key (the last value of each), and its Lsf-Conflict values.
func (m mergeGit) mergeTrailers(ctx context.Context, sha string) (map[string]string, []string, error) {
	out := map[string]string{}
	for _, k := range []string{trailerStep, trailerUnit, trailerMerge} {
		v, err := m.trailer(ctx, m.dir, sha, k)
		if err != nil {
			return nil, nil, err
		}
		out[k] = v
	}
	cs, err := m.trailers(ctx, m.dir, sha, trailerConflict)
	return out, cs, err
}

// verifyMerge checks a merge commit of unit: parents exactly pre and sha,
// this step's trailers, and its content: for a clean merge the tree git's
// own merge gives; for a resolved one, changed paths within the expected
// set.
func (m mergeGit) verifyMerge(ctx context.Context, c, pre, sha, label, unit string) (status string, conflicts []string, err error) {
	bad := func(what string) error {
		return fmt.Errorf("merge commit %s of %s fails verification: %s", c, unit, what)
	}
	parents, err := m.out(ctx, m.dir, "log", "-1", "--format=%P", c, "--")
	if err != nil {
		return "", nil, err
	}
	if parents != pre+" "+sha {
		return "", nil, bad(fmt.Sprintf("parents %q, want %s %s", parents, pre, sha))
	}
	tr, cs, err := m.mergeTrailers(ctx, c)
	if err != nil {
		return "", nil, err
	}
	if tr[trailerStep] != label || tr[trailerUnit] != unit {
		return "", nil, bad(fmt.Sprintf("trailers %s %q and %s %q, want %q and %q", trailerStep, tr[trailerStep], trailerUnit, tr[trailerUnit], label, unit))
	}
	staged, _, tree, err := m.expectedStaged(ctx, pre, sha)
	if err != nil {
		return "", nil, err
	}
	switch tr[trailerMerge] {
	case "clean":
		got, err := m.out(ctx, m.dir, "rev-parse", c+"^{tree}")
		if err != nil {
			return "", nil, err
		}
		if got != tree {
			return "", nil, bad("its tree is not git's merge of the two (a hook changed it?)")
		}
		return "clean", []string{}, nil
	case "resolved":
		changed, err := m.nulList(ctx, "diff", "--name-only", "-z", "--no-renames", pre, c)
		if err != nil {
			return "", nil, err
		}
		for _, p := range changed {
			if !slices.Contains(staged, p) {
				return "", nil, bad(p + " is not a path the merge touches")
			}
		}
		return "resolved", cs, nil
	}
	return "", nil, bad(fmt.Sprintf("%s %q", trailerMerge, tr[trailerMerge]))
}

// adoptMerge finds unit's merge among the last adoptionWalk first-parent
// commits: this step's merge of sha, verified again (found), or an error
// for a merge of unit that is not this step's of sha. none reports neither.
func (m mergeGit) adoptMerge(ctx context.Context, label string, u mergeUnitIn) (sha, status string, conflicts []string, err error) {
	list, err := m.out(ctx, m.dir, "log", "--first-parent", "-n", fmt.Sprint(adoptionWalk), "--format=%H %P", "HEAD", "--")
	if err != nil {
		return "", "", nil, err
	}
	for _, line := range strings.Split(list, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 {
			continue
		}
		unit, err := m.trailer(ctx, m.dir, f[0], trailerUnit)
		if err != nil {
			return "", "", nil, err
		}
		if unit != u.Unit {
			continue
		}
		step, err := m.trailer(ctx, m.dir, f[0], trailerStep)
		if err != nil {
			return "", "", nil, err
		}
		if f[2] != u.Sha || step != label {
			return "", "", nil, fmt.Errorf("unit %s was already merged at %s (%s %s, merging %s); it is not merged again", u.Unit, f[0], trailerStep, step, f[2])
		}
		status, conflicts, err := m.verifyMerge(ctx, f[0], f[1], u.Sha, label, u.Unit)
		if err != nil {
			return "", "", nil, err
		}
		return f[0], status, conflicts, nil
	}
	return "", "", nil, nil
}

// ownMergeInProgress is the pending unit whose merge this step left in
// progress (MERGE_MSG carries this step's trailers and MERGE_HEAD is the
// unit's commit), -1 when there is no merge in progress; any other merge
// in progress is an error and is left as it is.
func (m mergeGit) ownMergeInProgress(ctx context.Context, label string, units []mergeUnitIn, interrupted bool) (int, error) {
	head, err := m.gitFile(ctx, "MERGE_HEAD")
	if err != nil || strings.TrimSpace(head) == "" {
		return -1, err
	}
	head = strings.TrimSpace(strings.SplitN(head, "\n", 2)[0])
	msg, err := m.gitFile(ctx, "MERGE_MSG")
	if err != nil {
		return -1, err
	}
	stepRE := regexp.MustCompile(`(?m)^` + trailerStep + `: ` + regexp.QuoteMeta(label) + `\r?$`)
	for i, u := range units {
		if interrupted && u.Sha == head && stepRE.MatchString(msg) && regexp.MustCompile(`(?m)^`+trailerUnit+`: `+regexp.QuoteMeta(u.Unit)+`\r?$`).MatchString(msg) {
			return i, nil
		}
	}
	return -1, fmt.Errorf("a merge is in progress in the merge clone %s that this stage did not start (MERGE_HEAD %s); remove the clone and relaunch", m.dir, head)
}

// abortOwn aborts a merge in progress and checks the checkout: no merge in
// progress, HEAD at pre, every path the merge could touch clean.
func (m mergeGit) abortOwn(ctx context.Context, pre string, paths []string) error {
	ctx = context.WithoutCancel(ctx)
	h, err := m.gitFile(ctx, "MERGE_HEAD")
	if err != nil {
		return err
	}
	if h != "" {
		if _, err := m.r.run(ctx, m.dir, "merge", "--abort"); err != nil {
			if _, err2 := m.r.run(ctx, m.dir, "reset", "-q", "--merge"); err2 != nil {
				return errors.Join(err, err2)
			}
		}
	}
	if h, err := m.gitFile(ctx, "MERGE_HEAD"); err != nil || h != "" {
		return errors.Join(errors.New("the merge is still in progress after git merge --abort"), err)
	}
	head, err := m.head(ctx, m.dir)
	if err != nil {
		return err
	}
	if head != pre {
		return fmt.Errorf("HEAD moved from %s to %s during the merge", pre, head)
	}
	for _, p := range paths {
		clean, err := m.pathClean(ctx, m.dir, p)
		if err != nil {
			return err
		}
		if !clean {
			return fmt.Errorf("%s is left uncommitted after the merge was aborted", p)
		}
	}
	return nil
}

// gitSetup is a digest of what in dir's .git makes git run commands: its
// config, its hooks (names, modes, contents) and its info files.
func gitSetup(dir string) (string, error) {
	h := sha256.New()
	root := filepath.Join(dir, ".git")
	for _, sub := range []string{"config", "hooks", "info"} {
		err := filepath.WalkDir(filepath.Join(root, sub), func(p string, d os.DirEntry, err error) error {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "%s\x00%v\x00", strings.TrimPrefix(p, root), fi.Mode())
			if fi.Mode().IsRegular() {
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				h.Write(b)
			} else if fi.Mode()&os.ModeSymlink != 0 {
				t, _ := os.Readlink(p)
				h.Write([]byte(t))
			}
			h.Write([]byte{0})
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// leftStaged is an error naming what a hook staged during a merge: it is
// not in the merge commit (git 2.54 commits the merge from its own index),
// and a later commit would sweep it in.
func (m mergeGit) leftStaged(ctx context.Context, unit, c string) error {
	staged, err := m.nulList(ctx, "diff", "--cached", "--name-only", "-z", "HEAD")
	if err != nil || len(staged) == 0 {
		return err
	}
	return fmt.Errorf("after the merge of %s at %s, %q is staged (a hook?); it is not in the merge commit: commit or unstage it and relaunch", unit, c, staged)
}

// outside is the checkout's state outside the merge's paths: each path
// git status lists (untracked included) with its status and a digest of
// its bytes. The conflict call must leave it as it was.
func (m mergeGit) outside(ctx context.Context, staged []string) (map[string]string, error) {
	b, err := m.r.run(ctx, m.dir, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	fields := strings.Split(string(b), "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 4 {
			continue
		}
		xy, p := f[:2], f[3:]
		if xy[0] == 'R' || xy[0] == 'C' {
			i++ // the source path follows
		}
		if slices.Contains(staged, p) {
			continue
		}
		out[p] = xy + " " + fileState(filepath.Join(m.dir, p))
	}
	return out, nil
}

// outsideHashMax bounds the bytes outside reads of one file; past it a
// file is compared by size and modification time.
const outsideHashMax = 64 << 20

// fileState is what outside compares of one path: its digest, its size
// and mtime when large, "absent", or the error reading it.
func fileState(path string) string {
	fi, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "absent"
	case err != nil:
		return "unreadable: " + err.Error()
	case !fi.Mode().IsRegular():
		return fmt.Sprintf("%v %d %d", fi.Mode().Type(), fi.Size(), fi.ModTime().UnixNano())
	case fi.Size() > outsideHashMax:
		return fmt.Sprintf("size %d mtime %d", fi.Size(), fi.ModTime().UnixNano())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "unreadable: " + err.Error()
	}
	return contentDigest(b)
}

// changedOutside names the paths whose state differs between two outside
// snapshots.
func changedOutside(before, after map[string]string) []string {
	out := []string{}
	for p, v := range after {
		if before[p] != v {
			out = append(out, p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// stagedOK reports why a staged resolution is not acceptable, "" when it
// is: no unmerged path, no conflict marker in a conflicted file, nothing
// staged outside the expected set, every non-conflicted path as git's own
// merge has it (tree), no unstaged change to the set. A conflicted file's
// content is the call's judgement; only its markers are checked.
func (m mergeGit) stagedOK(ctx context.Context, staged, conflicts []string, tree string) (string, error) {
	if u, err := m.nulList(ctx, "diff", "--name-only", "-z", "--diff-filter=U"); err != nil || len(u) > 0 {
		return fmt.Sprintf("paths still unmerged: %q", u), err
	}
	for _, p := range conflicts {
		inIndex, err := m.nulList(ctx, "ls-files", "-z", "--", p)
		if err != nil {
			return "", err
		}
		if len(inIndex) == 0 {
			continue // resolved by deletion
		}
		b, err := m.r.run(ctx, m.dir, "show", ":"+p)
		if err != nil {
			return "", err
		}
		if conflictMarker.Match(b) {
			return p + " still holds conflict markers", nil
		}
	}
	clean := slices.DeleteFunc(slices.Clone(staged), func(p string) bool { return slices.Contains(conflicts, p) })
	if len(clean) > 0 {
		same, err := m.status(ctx, m.dir, append([]string{"diff", "--cached", "--quiet", tree, "--"}, clean...)...)
		if err != nil {
			return "", err
		}
		if !same {
			diff, _ := m.nulList(ctx, append([]string{"diff", "--cached", "--name-only", "-z", tree, "--"}, clean...)...)
			return fmt.Sprintf("%q differ from git's own merge, though they did not conflict", diff), nil
		}
	}
	got, err := m.nulList(ctx, "diff", "--cached", "--name-only", "-z", "--no-renames", "HEAD")
	if err != nil {
		return "", err
	}
	for _, p := range got {
		if !slices.Contains(staged, p) {
			return p + " was staged but is not a path the merge touches", nil
		}
	}
	dirty, err := m.nulList(ctx, "diff", "--name-only", "-z")
	if err != nil {
		return "", err
	}
	for _, p := range dirty {
		if slices.Contains(staged, p) {
			return p + " has unstaged changes", nil
		}
	}
	return "", nil
}

// mergeStage is the merge step's body.
func (o *orch) mergeStage(in mergeInput) stageFn {
	return func(ctx context.Context, label string, interrupted bool) (json.RawMessage, error) {
		g, err := o.gitFor()
		if err != nil {
			return nil, err
		}
		// The main checkout first: a requester edit there stops the step
		// before any merge, gate or conflict call.
		if err := o.mainOn(ctx, g, in.IntegrationBranch); err != nil {
			return nil, err
		}
		bookPaths := []string{in.PlanPath, in.ArchivePath}
		for _, p := range bookPaths {
			if clean, err := g.pathClean(ctx, o.repo, p); err != nil || !clean {
				return nil, errors.Join(err, fmt.Errorf("%s has changes in the main checkout; commit or discard them and relaunch", p))
			}
		}
		name := "w" + itoa(in.Wave)
		dir, err := o.rt.MergeClone(ctx, name)
		if err != nil {
			return nil, err
		}
		m := mergeGit{g, dir, o.repo, "refs/lsf/" + o.ctx.plan8() + "/xfer/" + name}
		if _, err := g.r.run(ctx, o.repo, "update-ref", "-d", m.xfer); err != nil {
			return nil, err
		}
		// The transfer ref goes however the step ends: a commit it pins is
		// in the merge clone, which a fault keeps.
		defer func() {
			if _, err := g.r.run(context.WithoutCancel(ctx), o.repo, "update-ref", "-d", m.xfer); err != nil {
				o.rt.Log(fmt.Sprintf("%s: %s not deleted: %v", label, m.xfer, err))
			}
		}()
		if err := o.syncMergeClone(ctx, m, in, label); err != nil {
			return nil, err
		}
		// ff moves main to sha, a commit this step verified (and gated):
		// never to the clone's HEAD, which a relaunch may find past it.
		ff := func(sha string) error {
			if err := m.toMain(ctx, sha); err != nil {
				return err
			}
			return o.fastForward(ctx, g, in.IntegrationBranch, m.xfer, "remove "+m.dir+" and relaunch, and the step merges again onto the new tip")
		}
		date := o.today()
		bk := bookIn{Name: in.Name, ArchivePath: in.ArchivePath, LedgerPath: in.LedgerPath, Wave: in.Wave}
		res := mergeResult{}
		res.IntegrationBranch = in.IntegrationBranch
		res.Merges = []plan.Merge{}

		ref, err := m.headRef(ctx, m.dir)
		if err != nil {
			return nil, err
		}
		if ref != "refs/heads/"+in.IntegrationBranch {
			return nil, fmt.Errorf("the merge clone %s is not on %s", m.dir, in.IntegrationBranch)
		}
		resume, err := m.ownMergeInProgress(ctx, label, in.Units, interrupted)
		if err != nil {
			return nil, err
		}
		if resume < 0 {
			staged, err := m.nulList(ctx, "diff", "--cached", "--name-only", "-z", "HEAD")
			if err != nil {
				return nil, err
			}
			for _, p := range staged {
				if !interrupted || !slices.Contains(bookPaths, p) {
					return nil, fmt.Errorf("%s is staged in the merge clone %s; a merge would commit it: remove the clone and relaunch", p, m.dir)
				}
			}
			for _, p := range bookPaths {
				clean, err := m.pathClean(ctx, m.dir, p)
				if err != nil {
					return nil, err
				}
				if !clean && !interrupted {
					return nil, fmt.Errorf("%s has uncommitted changes this stage did not make; commit or discard them and relaunch", p)
				}
			}
		}
		for _, p := range bookPaths {
			if err := m.guard(ctx, m.dir, in.IntegrationBranch, p); err != nil {
				return nil, err
			}
		}

		// Before the first merge this run makes: the bookkeeping must be
		// recordable for the units still to merge, and its paths clean (dirt
		// on them here is not this step's: its bookkeeping follows every
		// merge).
		dryRan := false
		dryRun := func(pending []mergeUnitIn) error {
			if dryRan {
				return nil
			}
			dryRan = true
			for _, p := range bookPaths {
				clean, err := m.pathClean(ctx, m.dir, p)
				if err != nil {
					return err
				}
				if !clean {
					return fmt.Errorf("%s has uncommitted changes this stage did not make; commit or discard them and relaunch", p)
				}
			}
			head, err := m.head(ctx, m.dir)
			if err != nil {
				return err
			}
			planText, _, archive, archiveOK, err := m.bookTexts(ctx, head, in)
			if err != nil {
				return err
			}
			units := []bookUnit{}
			for _, u := range pending {
				units = append(units, bookUnit{Unit: u.Unit, Sha: strings.Repeat("0", 40), Report: u.Report})
			}
			if _, _, err := renderBookkeeping(planText, archive, archiveOK, bk, units, date); err != nil {
				return fmt.Errorf("the plan bookkeeping cannot be recorded, so nothing is merged: %w", err)
			}
			return nil
		}

		merged := []bookUnit{}
		for i, u := range in.Units {
			r, err := o.mergeOne(ctx, m, in, label, u, i == resume, i < resume, func() error { return dryRun(in.Units[i:]) }, &res)
			if err != nil {
				return nil, err
			}
			if r.Status != "clean" && r.Status != "resolved" && !r.replayed {
				if err := o.recordOutcome(ctx, label, u, r); err != nil {
					return nil, err
				}
			}
			if r.gateNotRun {
				res.GateNotRun = append(res.GateNotRun, u.Unit)
			}
			res.Merges = append(res.Merges, r.Merge)
			if r.Status == "clean" || r.Status == "resolved" {
				merged = append(merged, bookUnit{Unit: u.Unit, Sha: r.Sha, Report: u.Report})
				if err := ff(r.Sha); err != nil {
					return nil, err
				}
			}
		}

		if len(merged) > 0 {
			book, err := o.bookkeep(ctx, m, in, bk, label, merged, date, interrupted)
			if err != nil {
				return nil, err
			}
			if book != "" {
				if err := ff(book); err != nil {
					return nil, err
				}
			}
		}
		// Every commit in the clone is now in main: one that is not was
		// never adopted, and stays for the requester.
		ch, err := m.head(ctx, m.dir)
		if err != nil {
			return nil, err
		}
		if res.PlanSha, err = m.out(ctx, o.repo, "rev-parse", "refs/heads/"+in.IntegrationBranch); err != nil {
			return nil, err
		}
		if ch != res.PlanSha {
			return nil, fmt.Errorf("the merge clone %s holds %s, which this step did not adopt (%s is at %s); remove the clone and relaunch", m.dir, ch, in.IntegrationBranch, res.PlanSha)
		}
		if res.ArchitectureSha, err = m.out(ctx, o.repo, "rev-list", "-1", "refs/heads/"+in.IntegrationBranch, "--", in.ArchitectureDoc); err != nil {
			return nil, err
		}
		if res.PauseRequested, err = m.pauseRequested(ctx, o.repo, in.PauseRef); err != nil {
			return nil, err
		}
		// The wave's merges are finished: the clone and the transfer ref go.
		if err := o.rt.RemoveMergeClone(ctx, name); err != nil {
			o.rt.Log(fmt.Sprintf("%s: merge clone %s not removed: %v", label, dir, err))
		}
		return stageValue(res)
	}
}

// syncMergeClone brings a reused merge clone's integration branch, c, into
// line with the main repo's, h, before the step reads it, and refreshes the
// clone's other branches from the main repo (the units' report branches; a
// merge in progress is not touched). c ahead of h is a launch that stopped
// between a commit and its fast-forward: each first-parent commit in h..c
// must be one this step makes (a merge of a unit in the wave, or the
// bookkeeping, last), and the merge loop adopts, verifies and gates each by
// its trailers, fast-forwarding main as it goes. c behind h with no merge
// in progress holds nothing, and is reset to h. Anything else is a fault.
func (o *orch) syncMergeClone(ctx context.Context, m mergeGit, in mergeInput, label string) error {
	ib := "refs/heads/" + in.IntegrationBranch
	if _, err := m.r.run(ctx, m.dir, "fetch", "--no-tags", "-q", o.repo, "+refs/heads/*:refs/heads/*", "^"+ib); err != nil {
		return fmt.Errorf("refreshing the merge clone %s: %w", m.dir, err)
	}
	if _, err := m.r.run(ctx, m.dir, "fetch", "--no-tags", "-q", o.repo, ib); err != nil {
		return fmt.Errorf("fetching %s into the merge clone %s: %w", in.IntegrationBranch, m.dir, err)
	}
	h, err := m.out(ctx, m.dir, "rev-parse", "FETCH_HEAD")
	if err != nil {
		return err
	}
	c, err := m.out(ctx, m.dir, "rev-parse", ib)
	if err != nil {
		return err
	}
	remedy := fmt.Sprintf("; remove %s and relaunch, and the step adopts its merges from %s by their trailers", m.dir, in.IntegrationBranch)
	if c == h {
		return nil
	}
	if ahead, err := m.status(ctx, m.dir, "merge-base", "--is-ancestor", h, c); err != nil || ahead {
		if err != nil {
			return err
		}
		list, err := m.out(ctx, m.dir, "rev-list", "--first-parent", "--reverse", "--parents", h+".."+c)
		if err != nil {
			return err
		}
		lines := strings.Split(list, "\n")
		for i, line := range lines {
			f := strings.Fields(line)
			step, err := m.trailer(ctx, m.dir, f[0], trailerStep)
			if err != nil {
				return err
			}
			ok := step == label
			switch {
			case !ok:
			case len(f) == 3:
				unit, err := m.trailer(ctx, m.dir, f[0], trailerUnit)
				if err != nil {
					return err
				}
				ok = slices.ContainsFunc(in.Units, func(u mergeUnitIn) bool { return u.Unit == unit && u.Sha == f[2] })
			case len(f) == 2 && i == len(lines)-1:
				paths, err := m.nulList(ctx, "diff", "--name-only", "-z", "--no-renames", f[1], f[0])
				if err != nil {
					return err
				}
				ok = !slices.ContainsFunc(paths, func(p string) bool { return p != in.PlanPath && p != in.ArchivePath })
			default:
				ok = false
			}
			if !ok {
				return fmt.Errorf("the merge clone's %s holds %s, which is not this step's merge or bookkeeping%s", in.IntegrationBranch, f[0], remedy)
			}
		}
		o.rt.Log(fmt.Sprintf("%s: the merge clone holds %d commit(s) past %s's %s; adopting them", label, len(lines), in.IntegrationBranch, h))
		return nil
	}
	behind, err := m.status(ctx, m.dir, "merge-base", "--is-ancestor", c, h)
	if err != nil {
		return err
	}
	inProgress, err := m.gitFile(ctx, "MERGE_HEAD")
	if err != nil {
		return err
	}
	if !behind || inProgress != "" {
		return fmt.Errorf("the merge clone's %s (%s) and the main repo's (%s) have diverged, or the main repo moved during a merge in progress%s", in.IntegrationBranch, c, h, remedy)
	}
	_, err = m.r.run(ctx, m.dir, "reset", "-q", "--hard", h)
	return err
}

// bookTexts are the plan and archive at rev.
func (m mergeGit) bookTexts(ctx context.Context, rev string, in mergeInput) (planText []byte, planOK bool, archive []byte, archiveOK bool, err error) {
	if planText, planOK, err = m.blob(ctx, m.dir, rev, in.PlanPath); err != nil {
		return
	}
	if !planOK {
		err = fmt.Errorf("%s does not exist at %s", in.PlanPath, rev)
		return
	}
	archive, archiveOK, err = m.blob(ctx, m.dir, rev, in.ArchivePath)
	return
}

// mergeOne merges one unit, or adopts its merge. A fault mid-merge aborts
// the merge this step has in progress before it is returned.
func (o *orch) mergeOne(ctx context.Context, m mergeGit, in mergeInput, label string, u mergeUnitIn, resume, mustAdopt bool, dryRun func() error, res *mergeResult) (unitMerge, error) {
	out := unitMerge{Merge: plan.Merge{Unit: u.Unit, Files: []string{}}}
	if !resume {
		sha, status, conflicts, err := m.adoptMerge(ctx, label, u)
		if err != nil {
			return out, err
		}
		if sha != "" {
			if err := o.gatedMerge(ctx, m, in, u, sha); err != nil {
				return out, err
			}
			out.Status, out.Files, out.Sha = status, conflicts, sha
			o.rt.Log(fmt.Sprintf("%s: %s already merged at %s; adopted", label, u.Unit, sha))
			return out, nil
		}
		if mustAdopt {
			// A unit this step did not merge (regressed, unresolvable,
			// refused) has its outcome recorded.
			if rec, ok, err := o.recordedOutcome(ctx, label, u); err != nil || ok {
				return rec, err
			}
			return out, fmt.Errorf("unit %s comes before the merge in progress but has no merge of its own to adopt", u.Unit)
		}
		already, err := m.status(ctx, m.dir, "merge-base", "--is-ancestor", u.Sha, "HEAD")
		if err != nil {
			return out, fmt.Errorf("unit %s: %w", u.Unit, err)
		}
		if already {
			return out, fmt.Errorf("%s (unit %s) is already in HEAD, merged outside this stage", u.Sha, u.Unit)
		}
	}
	if err := dryRun(); err != nil {
		return out, err
	}
	if tip, err := m.out(ctx, m.dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+u.Branch+"^{commit}"); err == nil && tip != u.Sha {
		return out, fmt.Errorf("unit %s: branch %s is at %s, not the completion report's %s", u.Unit, u.Branch, tip, u.Sha)
	} else if err != nil && exitCode(err) != 1 {
		return out, err
	}
	pre, err := m.head(ctx, m.dir)
	if err != nil {
		return out, err
	}
	staged, conflicts, tree, err := m.expectedStaged(ctx, pre, u.Sha)
	if err != nil {
		return out, err
	}
	fail := func(err error) (unitMerge, error) {
		return out, errors.Join(err, m.abortOwn(ctx, pre, staged))
	}
	if !resume && in.Declared && len(conflicts) == 0 {
		// Gate git's merge of pre and the unit before merging. Only this
		// step moves the merge clone's HEAD; a requester commit to the main
		// repo meanwhile faults at the fast-forward.
		set, err := o.mergeGates(ctx, m, u, pre, tree, mergeMessage(in, label, u.Unit, "clean", nil))
		if err != nil {
			return out, err
		}
		if len(set.failed()) > 0 {
			return regressed(out, set), nil
		}
	}
	if !resume {
		_, mergeErr := m.r.run(ctx, m.dir, "merge", "--no-ff", "--no-edit", "-m", mergeMessage(in, label, u.Unit, "clean", nil), u.Sha)
		if ctx.Err() != nil {
			return fail(ctx.Err())
		}
		head, err := m.head(ctx, m.dir)
		if err != nil {
			return fail(err)
		}
		inProgress, err := m.gitFile(ctx, "MERGE_HEAD")
		if err != nil {
			return fail(err)
		}
		switch {
		case head != pre:
			// A commit landed (a post-merge hook's error notwithstanding).
			status, _, err := m.verifyMerge(ctx, head, pre, u.Sha, label, u.Unit)
			if err == nil {
				err = m.leftStaged(ctx, u.Unit, head)
			}
			if err == nil {
				err = o.gatedMerge(ctx, m, in, u, head)
			}
			if err != nil {
				return out, err
			}
			out.Status, out.Sha = status, head
			return out, nil
		case inProgress == "" && mergeErr != nil && refusedOverLocal(mergeErr):
			// git refused over the requester's changes before starting:
			// the checkout is as it was.
			out.Status, out.Files = "unresolvable", refusedFiles(mergeErr)
			o.rt.Log(fmt.Sprintf("%s: merge of %s refused: %v", label, u.Unit, mergeErr))
			return out, nil
		case inProgress == "" && mergeErr != nil:
			// Another failure to start (a stale index.lock, a hook) is the
			// stage's fault, with git's reason.
			return fail(mergeErr)
		case inProgress == "":
			return fail(fmt.Errorf("git merge of %s neither committed nor left a merge in progress", u.Unit))
		case len(conflicts) == 0:
			// Stopped with nothing in conflict (a pre-merge-commit hook
			// refused it): not the conflict call's to settle.
			return fail(errors.Join(fmt.Errorf("the merge of %s stopped with no conflict", u.Unit), mergeErr))
		}
	}
	before, err := m.outside(ctx, staged)
	if err != nil {
		return fail(err)
	}
	setup, err := gitSetup(m.dir)
	if err != nil {
		return fail(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		t := o.ctx.tierFor("default")
		cr, cl, err := o.agent(ctx, conflictPrompt(o.ctx, in, u, conflicts), lsfrt.Opts{
			Label: "merge-conflict:w" + itoa(in.Wave) + ":" + u.Unit, Model: t.Model, Effort: t.Effort, Phase: "Merge", Schema: conflictSchema, Dir: m.dir,
		})
		if err != nil {
			return fail(err)
		}
		cr.Uncommitted = nil // the call runs in the merge clone as found: code checks it itself
		// Before any more git there: a config or hook the call planted would
		// run under the step's git, outside any sandbox. No abort either.
		if now, err := gitSetup(m.dir); err != nil || now != setup {
			return out, errors.Join(err, fmt.Errorf("the conflict call changed the merge clone's git config, hooks or info files; remove %s and relaunch", m.dir))
		}
		v, sigs := o.callSigs(site{at: plan.At{Stage: "merge", Unit: u.Unit, Wave: plan.W(in.Wave)}, writer: false}, cr, cl)
		res.Signals = append(res.Signals, sigs...)
		r := decode[conflictResult](o.rt, cl, v)
		head, err := m.head(ctx, m.dir)
		if err != nil {
			return fail(err)
		}
		if head != pre {
			return fail(fmt.Errorf("the conflict call committed %s; the stage commits a resolution itself", head))
		}
		after, err := m.outside(ctx, staged)
		if err != nil {
			return fail(err)
		}
		if ch := changedOutside(before, after); len(ch) > 0 {
			// Not a resolution to decline: the call touched the checkout
			// beyond the merge, which the requester must see.
			return fail(fmt.Errorf("the conflict call changed %q, outside the merge of %s; check them", ch, u.Unit))
		}
		if r == nil || r.Result.Status != "resolved" {
			break
		}
		why, err := m.stagedOK(ctx, staged, conflicts, tree)
		if err != nil {
			return fail(err)
		}
		if why != "" {
			o.rt.Log(fmt.Sprintf("%s: resolution of %s not accepted: %s", cl, u.Unit, why))
			if cr.Replayed && attempt == 1 {
				continue // the journaled resolution was lost; ask again
			}
			break
		}
		if in.Declared {
			gated, err := m.out(ctx, m.dir, "write-tree")
			if err != nil {
				return fail(err)
			}
			set, err := o.mergeGates(ctx, m, u, pre, gated, mergeMessage(in, label, u.Unit, "resolved", conflicts))
			if err != nil {
				return fail(err)
			}
			if len(set.failed()) > 0 {
				if err := m.abortOwn(ctx, pre, staged); err != nil {
					return out, err
				}
				return regressed(out, set), nil
			}
			if again, err := m.out(ctx, m.dir, "write-tree"); err != nil || again != gated {
				return fail(errors.Join(fmt.Errorf("the index of %s's merge changed during its gates", u.Unit), err))
			}
		}
		if _, err := m.r.run(ctx, m.dir, "commit", "-q", "-m", mergeMessage(in, label, u.Unit, "resolved", conflicts)); err != nil {
			if h, _ := m.head(context.WithoutCancel(ctx), m.dir); h == pre {
				return fail(fmt.Errorf("commit of the merge of %s rejected: %w", u.Unit, err))
			}
		}
		head, err = m.head(ctx, m.dir)
		if err != nil {
			return out, err
		}
		status, cs, err := m.verifyMerge(ctx, head, pre, u.Sha, label, u.Unit)
		if err == nil {
			err = m.leftStaged(ctx, u.Unit, head)
		}
		if err == nil {
			err = o.gatedMerge(ctx, m, in, u, head)
		}
		if err != nil {
			return out, err
		}
		out.Status, out.Files, out.Sha = status, cs, head
		return out, nil
	}
	if err := m.abortOwn(ctx, pre, staged); err != nil {
		return out, err
	}
	out.Status, out.Files = "unresolvable", conflicts
	return out, nil
}

// unitMerge is mergeOne's result: the unit's merge, whether its gates were
// not run (it is then regressed, and the orchestrator stops on gate-not-run),
// and whether it is a recorded outcome replayed.
type unitMerge struct {
	plan.Merge
	gateNotRun bool
	replayed   bool
}

// regressed is out as a merge whose gates failed: nothing was committed.
// Files are the gates that are not green.
func regressed(out unitMerge, set gateSet) unitMerge {
	out.Status, out.Files = "regressed", []string{}
	red := false
	for _, r := range set.failed() {
		out.Files = append(out.Files, "`"+r.Name+"`: "+r.Status+" — "+r.Summary)
		red = red || r.Status == lsfrt.GateRed
	}
	// A red gate is the regression; a gate not run stops on its own only
	// when nothing failed.
	out.gateNotRun = !red && len(out.Files) > 0
	return out
}

// mergeGates gates tree, the merge of pre and u.Sha, before it is
// committed. The unit's own gated tree is reused when the two differ only
// at its completion report and this launch holds that result; otherwise a
// dangling candidate commit of tree (parents pre and u.Sha, message msg)
// is gated. A gate fault is the step's failure; err is that or a stop.
func (o *orch) mergeGates(ctx context.Context, m mergeGit, u mergeUnitIn, pre, tree, msg string) (gateSet, error) {
	if set, ok := o.reusable(ctx, m, u, tree); ok {
		o.rt.Log(fmt.Sprintf("merge of %s: its gates at %s stand (the merge differs only at %s)", u.Unit, u.GatedTree, u.Report))
		return set, nil
	}
	cand, err := m.out(ctx, m.dir, "-c", "commit.gpgsign=false", "commit-tree", tree, "-p", pre, "-p", u.Sha, "-m", msg)
	if err != nil {
		return gateSet{}, fmt.Errorf("the merge of %s: commit-tree for its gates: %w", u.Unit, err)
	}
	if err := m.toMain(ctx, cand); err != nil {
		return gateSet{}, fmt.Errorf("the merge of %s: its candidate to the main repo for its gates: %w", u.Unit, err)
	}
	set, fault, err := o.gates(ctx, cand)
	if err != nil {
		return gateSet{}, err
	}
	if fault != "" {
		return gateSet{}, fmt.Errorf("the merge of %s: %s", u.Unit, fault)
	}
	return set, nil
}

// gatedMerge checks, with declared gates, that a merge commit on the
// integration branch has a gated tree: the merge's own gate run, replayed
// from the memo or the journal at no cost. A commit whose tree no run
// covers (a commit raced the merge, a hook changed it, an adopted merge of
// an earlier launch) is gated now; if its gates fail it is the step's
// fault, and the commit stays for the requester to revert.
func (o *orch) gatedMerge(ctx context.Context, m mergeGit, in mergeInput, u mergeUnitIn, commit string) error {
	if !in.Declared {
		return nil
	}
	tree, err := m.out(ctx, m.dir, "rev-parse", commit+"^{tree}")
	if err != nil {
		return err
	}
	if set, ok := o.reusable(ctx, m, u, tree); ok && len(set.failed()) == 0 {
		return nil
	}
	if err := m.toMain(ctx, commit); err != nil {
		return fmt.Errorf("merge commit %s of %s to the main repo for its gates: %w", commit, u.Unit, err)
	}
	set, fault, err := o.gates(ctx, commit)
	switch {
	case err != nil:
		return err
	case fault != "":
		return fmt.Errorf("merge commit %s of %s: %s", commit, u.Unit, fault)
	case len(set.failed()) > 0:
		return fmt.Errorf("merge commit %s of %s is on the integration branch, or in the merge clone %s, but its gates do not pass (%s); revert it, or remove the clone, and relaunch", commit, u.Unit, m.dir, strings.Join(regressed(unitMerge{}, set).Files, "; "))
	}
	return nil
}

// reusable is the unit's own gate result for tree, when tree differs from
// the unit's gated tree at most at its completion report and this launch
// holds that result. A diff that fails is no reuse.
func (o *orch) reusable(ctx context.Context, m mergeGit, u mergeUnitIn, tree string) (gateSet, bool) {
	if u.GatedTree == "" {
		return gateSet{}, false
	}
	diff, err := m.nulList(ctx, "diff-tree", "-r", "-z", "--name-only", "--no-renames", u.GatedTree, tree)
	if err != nil || len(diff) > 1 || (len(diff) == 1 && diff[0] != u.Report) {
		return gateSet{}, false
	}
	return o.gatedEarlier(ctx, u.GatedTree)
}

// outcomeLabel is the step that records a unit this merge step did not
// merge, so a resume past it does not wedge.
func outcomeLabel(label string, u mergeUnitIn) string { return "merge-outcome:" + label + ":" + u.Unit }

// recordOutcome journals a unit's non-merge outcome.
func (o *orch) recordOutcome(ctx context.Context, label string, u mergeUnitIn, r unitMerge) error {
	rec := outcomeRecord{Merge: r.Merge, GateNotRun: r.gateNotRun}
	_, _, err := o.runStepAs(ctx, outcomeLabel(label, u), map[string]string{"unit": u.Unit, "sha": u.Sha}, func(context.Context, string, bool) (json.RawMessage, error) {
		return plan.MarshalJS(rec, "")
	})
	var se *stageErr
	if errors.As(err, &se) {
		return errors.New(se.reason)
	}
	return err
}

// recordedOutcome replays a unit's recorded non-merge outcome; ok is false
// when there is none.
func (o *orch) recordedOutcome(ctx context.Context, label string, u mergeUnitIn) (unitMerge, bool, error) {
	raw, _, err := o.runStepAs(ctx, outcomeLabel(label, u), map[string]string{"unit": u.Unit, "sha": u.Sha}, func(context.Context, string, bool) (json.RawMessage, error) {
		return nil, errOutcomeNotRecorded
	})
	var se *stageErr
	switch {
	case errors.As(err, &se):
		return unitMerge{}, false, nil
	case err != nil:
		return unitMerge{}, false, err
	}
	var rec outcomeRecord
	if err := json.Unmarshal(raw, &rec); err != nil || rec.Unit != u.Unit {
		return unitMerge{}, false, fmt.Errorf("unit %s: recorded merge outcome unreadable: %s", u.Unit, raw)
	}
	o.rt.Log(fmt.Sprintf("%s: %s was %s; outcome replayed", label, u.Unit, rec.Status))
	return unitMerge{Merge: rec.Merge, gateNotRun: rec.GateNotRun, replayed: true}, true, nil
}

// outcomeRecord is a recorded non-merge outcome.
type outcomeRecord struct {
	plan.Merge
	GateNotRun bool `json:"gateNotRun,omitempty"`
}

var errOutcomeNotRecorded = errors.New("no recorded outcome")

// refusedOverLocal reports whether git refused a merge because it would
// overwrite local changes or untracked files.
func refusedOverLocal(err error) bool {
	var ge *gitError
	return errors.As(err, &ge) && strings.Contains(ge.stderr, "would be overwritten by merge")
}

// refusedFiles are the paths git names, tab-indented, when it refuses a
// merge over local changes.
func refusedFiles(err error) []string {
	out := []string{}
	var ge *gitError
	if !errors.As(err, &ge) {
		return out
	}
	for _, l := range strings.Split(ge.stderr, "\n") {
		if strings.HasPrefix(l, "\t") {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

// maintenanceDate reads the date of first's Plan Maintenance entry in a
// rendered plan.
func maintenanceDate(planText []byte, first string) string {
	m := regexp.MustCompile(`(?m)^### (\d{4}-\d{2}-\d{2}) — `+regexp.QuoteMeta(first)+` complete\r?$`).FindAllSubmatch(planText, -1)
	if len(m) == 0 {
		return ""
	}
	return string(m[len(m)-1][1])
}

// bookkeep commits the plan bookkeeping for merged, or adopts this step's
// own commit of it, and returns that commit; "" when every unit was
// recorded already. On an interrupted rerun, dirt on the plan or archive
// must be this step's own rendering.
func (o *orch) bookkeep(ctx context.Context, m mergeGit, in mergeInput, bk bookIn, label string, merged []bookUnit, date string, interrupted bool) (string, error) {
	paths := []string{in.PlanPath, in.ArchivePath}
	if interrupted {
		sha, err := m.adoptSet(ctx, m.dir, in.IntegrationBranch, paths, label, func(parent, cand map[string]blobAt) bool {
			d := maintenanceDate(cand[in.PlanPath].b, merged[0].Unit)
			if d == "" || !parent[in.PlanPath].ok {
				return false
			}
			p, a, err := renderBookkeeping(parent[in.PlanPath].b, parent[in.ArchivePath].b, parent[in.ArchivePath].ok, bk, merged, d)
			return err == nil && string(p) == string(cand[in.PlanPath].b) && string(a) == string(cand[in.ArchivePath].b)
		})
		if err != nil || sha != "" {
			if sha != "" {
				o.rt.Log(fmt.Sprintf("%s: bookkeeping already committed at %s; adopted", label, sha))
			}
			return sha, err
		}
	}
	head, err := m.head(ctx, m.dir)
	if err != nil {
		return "", err
	}
	planText, _, archive, archiveOK, err := m.bookTexts(ctx, head, in)
	if err != nil {
		return "", err
	}
	// A unit whose stub at its merge commit is already in the plan was
	// recorded by an earlier run of this step (one that went on to fail):
	// it is not recorded again.
	pending := []bookUnit{}
	for _, u := range merged {
		if strings.Contains(string(planText), stubLine(bk, u)+"\n") || strings.Contains(string(planText), stubLine(bk, u)+"\r\n") {
			o.rt.Log(fmt.Sprintf("%s: %s already recorded in %s; not recorded again", label, u.Unit, in.PlanPath))
			continue
		}
		pending = append(pending, u)
	}
	if len(pending) == 0 {
		return "", nil
	}
	merged = pending
	// Dirt on the paths is this step's own write only after a kill inside
	// its commit, on an interrupted rerun: the plan and archive must then
	// hold the rendering, dated as it was.
	dirty := false
	for _, p := range paths {
		clean, err := m.pathClean(ctx, m.dir, p)
		if err != nil {
			return "", err
		}
		dirty = dirty || !clean
	}
	if dirty {
		gotPlan, err := readFile(m.dir, in.PlanPath)
		if err != nil {
			return "", err
		}
		gotArchive, err := readFile(m.dir, in.ArchivePath)
		if err != nil {
			return "", err
		}
		if d := maintenanceDate(gotPlan, merged[0].Unit); d != "" {
			date = d
		}
		wantPlan, wantArchive, err := renderBookkeeping(planText, archive, archiveOK, bk, merged, date)
		if err != nil {
			return "", err
		}
		if !interrupted || string(gotPlan) != string(wantPlan) || (gotArchive != nil && string(gotArchive) != string(wantArchive)) {
			return "", fmt.Errorf("%s or %s has uncommitted changes this stage did not make; commit or discard them and relaunch", in.PlanPath, in.ArchivePath)
		}
	}
	newPlan, newArchive, err := renderBookkeeping(planText, archive, archiveOK, bk, merged, date)
	if err != nil {
		return "", err
	}
	units := []string{}
	for _, u := range merged {
		units = append(units, u.Unit)
	}
	return m.commitFiles(ctx, m.dir, in.IntegrationBranch, []fileWrite{{in.PlanPath, newPlan}, {in.ArchivePath, newArchive}},
		"docs("+in.Name+"): wave "+itoa(in.Wave)+" merge bookkeeping ("+strings.Join(units, ", ")+")", label, head)
}
