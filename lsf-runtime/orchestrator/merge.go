package orchestrator

import (
	"context"
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
// merged into the integration branch in the main checkout, in plan merge
// order, a conflict going to a model call that stages a resolution which
// code commits; then one commit records the bookkeeping (bookkeeping.go).
// Every commit carries trailers, so an interrupted rerun adopts its own
// commits instead of merging or recording twice.

// mergeUnitIn is one unit to merge: its completion-report branch and the
// commit the completion report returned (a journaled value: nothing here
// needs the branch to resolve on a replay).
type mergeUnitIn struct {
	Unit   string `json:"unit"`
	Branch string `json:"branch"`
	Sha    string `json:"sha"`
	Report string `json:"report"`
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

// mergeGit is the merge step's git in the main checkout.
type mergeGit struct {
	gitOps
	dir string
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
	b, err := m.r.run(ctx, m.dir, "merge-tree", "--write-tree", "--name-only", "--no-messages", pre, sha)
	if err != nil && exitCode(err) != 1 { // 1: conflicts
		return "", nil, err
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", nil, fmt.Errorf("git merge-tree %s %s printed no tree", pre, sha)
	}
	for _, l := range lines[1:] {
		if l == "" {
			break
		}
		if !slices.Contains(conflicts, l) {
			conflicts = append(conflicts, l)
		}
	}
	return lines[0], conflicts, nil
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

// commitTrailers are a commit's trailers by key, first value each, and the
// Lsf-Conflict values.
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
	return -1, fmt.Errorf("a merge is in progress in the main checkout that this stage did not start (MERGE_HEAD %s); finish or abort it and relaunch", head)
}

// abortOwn aborts a merge in progress and checks the checkout: no merge in
// progress, HEAD at pre, every path the merge could touch clean.
func (m mergeGit) abortOwn(ctx context.Context, pre string, paths []string) error {
	ctx = context.WithoutCancel(ctx)
	if h, _ := m.gitFile(ctx, "MERGE_HEAD"); h != "" {
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

// stagedOK reports why a staged resolution is not acceptable, "" when it
// is: no unmerged path, no conflict marker in a conflicted file, nothing
// staged outside the expected set, no unstaged change to it, no new
// untracked file.
func (m mergeGit) stagedOK(ctx context.Context, staged, conflicts, untracked []string) (string, error) {
	if u, err := m.nulList(ctx, "diff", "--name-only", "-z", "--diff-filter=U"); err != nil || len(u) > 0 {
		return fmt.Sprintf("paths still unmerged: %q", u), err
	}
	for _, p := range conflicts {
		b, err := m.r.run(ctx, m.dir, "show", ":"+p)
		if err != nil {
			if exitCode(err) == 128 { // resolved by deletion
				continue
			}
			return "", err
		}
		if conflictMarker.Match(b) {
			return p + " still holds conflict markers", nil
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
	now, err := m.nulList(ctx, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return "", err
	}
	for _, p := range now {
		if !slices.Contains(untracked, p) {
			return p + " is a new untracked file", nil
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
		m := mergeGit{g, o.repo}
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
			return nil, fmt.Errorf("the main checkout is not on %s", in.IntegrationBranch)
		}
		resume, err := m.ownMergeInProgress(ctx, label, in.Units, interrupted)
		if err != nil {
			return nil, err
		}
		bookPaths := []string{in.PlanPath, in.ArchivePath}
		if resume < 0 {
			staged, err := m.nulList(ctx, "diff", "--cached", "--name-only", "-z", "HEAD")
			if err != nil {
				return nil, err
			}
			for _, p := range staged {
				if !interrupted || !slices.Contains(bookPaths, p) {
					return nil, fmt.Errorf("%s is staged in the main checkout; a merge would commit it: commit or unstage it and relaunch", p)
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

		dryRan := false
		dryRun := func() error {
			if dryRan {
				return nil
			}
			dryRan = true
			head, err := m.head(ctx, m.dir)
			if err != nil {
				return err
			}
			planText, _, archive, archiveOK, err := m.bookTexts(ctx, head, in)
			if err != nil {
				return err
			}
			units := []bookUnit{}
			for _, u := range in.Units {
				units = append(units, bookUnit{Unit: u.Unit, Sha: strings.Repeat("0", 40), Report: u.Report})
			}
			if _, _, err := renderBookkeeping(planText, archive, archiveOK, bk, units, date); err != nil {
				return fmt.Errorf("the plan bookkeeping cannot be recorded, so nothing is merged: %w", err)
			}
			return nil
		}

		merged := []bookUnit{}
		for i, u := range in.Units {
			r, err := o.mergeOne(ctx, m, in, label, u, i == resume, i < resume, dryRun, &res)
			if err != nil {
				return nil, err
			}
			res.Merges = append(res.Merges, r)
			if r.Status == "clean" || r.Status == "resolved" {
				merged = append(merged, bookUnit{Unit: u.Unit, Sha: r.Sha, Report: u.Report})
			}
		}

		if len(merged) > 0 {
			if err := o.bookkeep(ctx, m, in, bk, label, merged, date, interrupted); err != nil {
				return nil, err
			}
		}
		if res.PlanSha, err = m.head(ctx, m.dir); err != nil {
			return nil, err
		}
		if res.ArchitectureSha, err = m.out(ctx, m.dir, "rev-list", "-1", "refs/heads/"+in.IntegrationBranch, "--", in.ArchitectureDoc); err != nil {
			return nil, err
		}
		if res.PauseRequested, err = m.pauseRequested(ctx, m.dir, in.PauseRef); err != nil {
			return nil, err
		}
		return stageValue(res)
	}
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
func (o *orch) mergeOne(ctx context.Context, m mergeGit, in mergeInput, label string, u mergeUnitIn, resume, mustAdopt bool, dryRun func() error, res *mergeResult) (plan.Merge, error) {
	out := plan.Merge{Unit: u.Unit, Files: []string{}}
	if !resume {
		sha, status, conflicts, err := m.adoptMerge(ctx, label, u)
		if err != nil {
			return out, err
		}
		if sha != "" {
			out.Status, out.Files, out.Sha = status, conflicts, sha
			o.rt.Log(fmt.Sprintf("%s: %s already merged at %s; adopted", label, u.Unit, sha))
			return out, nil
		}
		if mustAdopt {
			return out, fmt.Errorf("unit %s comes before the merge in progress but has no merge of its own to adopt", u.Unit)
		}
		if in, err := m.status(ctx, m.dir, "merge-base", "--is-ancestor", u.Sha, "HEAD"); err != nil || in {
			return out, errors.Join(fmt.Errorf("%s (unit %s) is already in %s, merged outside this stage", u.Sha, u.Unit, "HEAD"), err)
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
	if resume {
		pre, err = m.out(ctx, m.dir, "rev-parse", "HEAD")
		if err != nil {
			return out, err
		}
	}
	staged, conflicts, _, err := m.expectedStaged(ctx, pre, u.Sha)
	if err != nil {
		return out, err
	}
	fail := func(err error) (plan.Merge, error) {
		return out, errors.Join(err, m.abortOwn(ctx, pre, staged))
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
		}
	}
	untracked, err := m.nulList(ctx, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return fail(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		t := o.ctx.tierFor("default")
		cr, cl, err := o.agent(ctx, conflictPrompt(o.ctx, in, u, conflicts), lsfrt.Opts{
			Label: "merge-conflict:w" + itoa(in.Wave) + ":" + u.Unit, Model: t.Model, Effort: t.Effort, Phase: "Merge", Schema: conflictSchema,
		})
		if err != nil {
			return fail(err)
		}
		cr.Uncommitted = nil // the staged resolution is expected; code checks the checkout itself
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
		if r == nil || r.Result.Status != "resolved" {
			break
		}
		why, err := m.stagedOK(ctx, staged, conflicts, untracked)
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
// own commit of it. On an interrupted rerun, dirt on the plan or archive
// must be this step's own rendering.
func (o *orch) bookkeep(ctx context.Context, m mergeGit, in mergeInput, bk bookIn, label string, merged []bookUnit, date string, interrupted bool) error {
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
			return err
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
	// Dirt on the paths is this step's own write only after a kill inside
	// its commit: it must be the rendering, dated as it was.
	for _, p := range paths {
		clean, err := m.pathClean(ctx, m.dir, p)
		if err != nil {
			return err
		}
		if clean {
			continue
		}
		dirty, _ := readFile(m.dir, in.PlanPath)
		if d := maintenanceDate(dirty, merged[0].Unit); interrupted && d != "" {
			date = d
		}
		wantPlan, wantArchive, err := renderBookkeeping(planText, archive, archiveOK, bk, merged, date)
		if err != nil {
			return err
		}
		gotPlan, _ := readFile(m.dir, in.PlanPath)
		gotArchive, _ := readFile(m.dir, in.ArchivePath)
		if !interrupted || string(gotPlan) != string(wantPlan) || (gotArchive != nil && string(gotArchive) != string(wantArchive)) {
			return fmt.Errorf("%s has uncommitted changes this stage did not make; commit or discard them and relaunch", p)
		}
		break
	}
	newPlan, newArchive, err := renderBookkeeping(planText, archive, archiveOK, bk, merged, date)
	if err != nil {
		return err
	}
	units := []string{}
	for _, u := range merged {
		units = append(units, u.Unit)
	}
	_, err = m.commitFiles(ctx, m.dir, in.IntegrationBranch, []fileWrite{{in.PlanPath, newPlan}, {in.ArchivePath, newArchive}},
		"docs("+in.Name+"): wave "+itoa(in.Wave)+" merge bookkeeping ("+strings.Join(units, ", ")+")", label, head)
	return err
}
