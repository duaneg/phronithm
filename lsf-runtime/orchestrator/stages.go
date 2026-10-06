package orchestrator

import (
	"bytes"
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
)

// The code stages (D18): load, decision log, ledger append and completion
// report. Each is a journaled step whose value has the agent-return shape,
// so decode and the scripted returns serve both. Their inputs are the
// journal's hash.

// stageValue is a step's value: an agent return with no commits listed.
func stageValue(result any) (json.RawMessage, error) {
	return plan.MarshalJS(map[string]any{"result": result, "commits": []any{}}, "")
}

// gitFor is the code stages' git, or an error when the runtime has no
// repository (a stub's steps never run their fn).
func (o *orch) gitFor() (gitOps, error) {
	if o.git == nil {
		return gitOps{}, errors.New("no repository")
	}
	return gitOps{o.git}, nil
}

type loadInput struct {
	Sha               string `json:"sha"`
	PlanPath          string `json:"planPath"`
	DecisionLogSha    string `json:"decisionLogSha"`
	DecisionLogPath   string `json:"decisionLogPath"`
	IntegrationBranch string `json:"integrationBranch"`
	ArchitectureDoc   string `json:"architectureDoc"`
	PauseRef          string `json:"pauseRef"`
}

// load reads the plan and the decision log at their SHAs, whole: LoadGraph
// extracts the blocks, so there is no second extractor. HEAD's branch and
// the pause ref are journaled too, but liveLoad re-reads them.
func (o *orch) load(in loadInput) stageFn {
	return func(ctx context.Context, _ string, _ bool) (json.RawMessage, error) {
		g, err := o.gitFor()
		if err != nil {
			return nil, err
		}
		read := func(sha, path string) (string, error) {
			b, ok, err := g.blob(ctx, o.repo, sha, path)
			if err == nil && !ok {
				err = fmt.Errorf("%s:%s does not exist", sha, path)
			}
			return string(b), err
		}
		r := copyResult{}
		if r.PlanBlock, err = read(in.Sha, in.PlanPath); err != nil {
			return nil, err
		}
		if in.DecisionLogSha != "" {
			if r.DecisionsBlock, err = read(in.DecisionLogSha, in.DecisionLogPath); err != nil {
				return nil, err
			}
		}
		if r.ArchitectureSha, err = g.out(ctx, o.repo, "rev-list", "-1", "refs/heads/"+in.IntegrationBranch, "--", in.ArchitectureDoc); err != nil {
			return nil, err
		}
		if r.HeadBranch, err = g.headBranch(ctx, o.repo); err != nil {
			return nil, err
		}
		if r.PauseRequested, err = g.pauseRequested(ctx, o.repo, in.PauseRef); err != nil {
			return nil, err
		}
		return stageValue(r)
	}
}

// liveReads re-reads HEAD's branch (headBranch non-nil) and the pause ref
// after a step, replayed or not, so a relaunch sees the requester's fix.
// Without a repository the scripted values stand.
func (o *orch) liveReads(ctx context.Context, headBranch *string, pause *bool) error {
	if o.git == nil {
		return nil
	}
	g := gitOps{o.git}
	var err error
	if headBranch != nil {
		if *headBranch, err = g.headBranch(ctx, o.repo); err != nil {
			return err
		}
	}
	*pause, err = g.pauseRequested(ctx, o.repo, o.ctx.pauseRef)
	return err
}

// mainPrecheck is what a main-checkout stage checks before it writes: HEAD
// on the integration branch, and path clean, or, on an interrupted rerun,
// holding what ownWrite accepts; then the guard.
func (o *orch) mainPrecheck(ctx context.Context, g gitOps, branch, path string, interrupted bool, ownWrite func([]byte) bool) error {
	ref, err := g.headRef(ctx, o.repo)
	if err != nil {
		return err
	}
	if ref != "refs/heads/"+branch {
		on := strings.TrimPrefix(ref, "refs/heads/")
		if ref == "" {
			on = "a detached HEAD"
		}
		return fmt.Errorf("the main checkout is on %s, not %s", on, branch)
	}
	clean, err := g.pathClean(ctx, o.repo, path)
	if err != nil {
		return err
	}
	if !clean {
		own := false
		if interrupted {
			if b, err := readFile(o.repo, path); err == nil && b != nil {
				own = ownWrite(b)
			}
		}
		if !own {
			return fmt.Errorf("%s has uncommitted changes this stage did not make; commit or discard them and relaunch", path)
		}
	}
	return g.guard(ctx, o.repo, branch, path)
}

// decisionLogInput: decisions are as the requester gave them (D11).
type decisionLogInput struct {
	Path              string     `json:"path"`
	Name              string     `json:"name"`
	IntegrationBranch string     `json:"integrationBranch"`
	Decisions         []decision `json:"decisions"`
	Subject           string     `json:"subject"`
}

// decisionKey identifies a decision's content: an id with other content
// is another decision.
func decisionKey(id, question, dec string, clears []string) string {
	if clears == nil {
		clears = []string{}
	}
	b, _ := json.Marshal([]any{id, question, dec, clears})
	return string(b)
}

func (d decision) key() string {
	var clears []string
	if d.Clears != nil {
		clears = *d.Clears
	}
	return decisionKey(d.ID, d.Question, d.Decision, clears)
}

func keysOf(ds []plan.Decision) []string {
	out := []string{}
	for _, d := range ds {
		out = append(out, decisionKey(d.ID, d.Question, d.Decision, d.Clears))
	}
	return out
}

// decisionsIn parses a decision log's block; a log that does not parse is
// an error.
func decisionsIn(text string) ([]string, error) {
	body, prob := plan.ExtractBlock(text, plan.DecisionsTag)
	if prob != "" {
		return nil, errors.New(prob)
	}
	ds, problems := plan.ParseDecisions(&body)
	if len(problems) > 0 {
		return nil, errors.New(problems[0].Statement)
	}
	return keysOf(ds), nil
}

// decisionsBlockRE is plan's block pattern for the decision log; the
// rendered text is parsed back, so a divergence cannot be committed.
var decisionsBlockRE = regexp.MustCompile("(?m)^```yaml[ \\t]+" + regexp.QuoteMeta(plan.DecisionsTag) + "[ \\t]*\\r?\\n((?s:.*?))^```[ \\t]*\\r?$")

// yq is a double-quoted scalar of the YAML subset.
func yq(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}

func decisionSkeleton(name string) []byte {
	return []byte("# Decisions: " + name + "\n\n```yaml lsf-decisions\ndecisions:\n```\n")
}

// renderDecisions appends todo, dated date, as the last entries of prior's
// block, and checks the result parses to prior's decisions plus todo.
func renderDecisions(prior []byte, todo []decision, date string) ([]byte, error) {
	before, err := decisionsIn(string(prior))
	if err != nil {
		return nil, fmt.Errorf("the existing decision block does not parse (%v); it is never appended to", err)
	}
	m := decisionsBlockRE.FindSubmatchIndex(prior)
	if m == nil {
		return nil, errors.New("no decisions block")
	}
	var e strings.Builder
	for _, d := range todo {
		fmt.Fprintf(&e, "  - id: %s\n    date: %s\n    question: %s\n    decision: %s\n", yq(d.ID), date, yq(d.Question), yq(d.Decision))
		if d.Clears != nil && len(*d.Clears) > 0 {
			qs := []string{}
			for _, c := range *d.Clears {
				qs = append(qs, yq(c))
			}
			fmt.Fprintf(&e, "    clears: [%s]\n", strings.Join(qs, ", "))
		}
	}
	end := m[3] // the body's end: the closing fence's line
	next := slices.Concat(prior[:end], []byte(e.String()), prior[end:])
	after, err := decisionsIn(string(next))
	want := slices.Clone(before)
	for _, d := range todo {
		want = append(want, d.key())
	}
	if err != nil || !slices.Equal(after, want) {
		reason := "content differs"
		if err != nil {
			reason = err.Error()
		}
		return nil, fmt.Errorf("the rendered decision block does not parse back to the decisions given (%s)", reason)
	}
	return next, nil
}

// pending are the decisions not already in a log holding have, in order.
func pending(ds []decision, have []string) []decision {
	out := []decision{}
	for _, d := range ds {
		if !slices.Contains(have, d.key()) {
			out = append(out, d)
		}
	}
	return out
}

// decisionLog appends the requester's decisions to the decision log in the
// main checkout and commits it. A decision already in the log with the same
// content is dropped; when none remain, the last commit touching the log
// is returned. The date is today's, taken here and kept out of the input.
func (o *orch) decisionLog(in decisionLogInput) stageFn {
	return func(ctx context.Context, label string, interrupted bool) (json.RawMessage, error) {
		g, err := o.gitFor()
		if err != nil {
			return nil, err
		}
		branch, path := in.IntegrationBranch, in.Path
		pre, err := g.head(ctx, o.repo)
		if err != nil {
			return nil, err
		}
		prior, ok, err := g.blob(ctx, o.repo, pre, path)
		if err != nil {
			return nil, err
		}
		if !ok {
			prior = decisionSkeleton(in.Name)
		}
		// The interrupted rerun's own write: the prior decisions plus these.
		ownWrite := func(b []byte) bool {
			have, err1 := decisionsIn(string(prior))
			got, err2 := decisionsIn(string(b))
			if err1 != nil || err2 != nil {
				return false
			}
			for _, d := range pending(in.Decisions, have) {
				have = append(have, d.key())
			}
			return slices.Equal(got, have)
		}
		if err := o.mainPrecheck(ctx, g, branch, path, interrupted, ownWrite); err != nil {
			return nil, err
		}
		done := func(sha string) (json.RawMessage, error) {
			return stageValue(scribeResult{Path: path, Sha: sha})
		}
		if interrupted {
			sha, _, err := g.adopt(ctx, o.repo, branch, path, adoption{
				label: label,
				matches: func(parent []byte, parentOK bool, cand []byte) bool {
					if !parentOK {
						parent = decisionSkeleton(in.Name)
					}
					have, err1 := decisionsIn(string(parent))
					got, err2 := decisionsIn(string(cand))
					if err1 != nil || err2 != nil {
						return false
					}
					todo := pending(in.Decisions, have)
					if len(todo) == 0 {
						return false
					}
					for _, d := range todo {
						have = append(have, d.key())
					}
					return slices.Equal(got, have)
				},
				survives: func(tip []byte, ok bool) bool {
					have, err := decisionsIn(string(tip))
					return ok && err == nil && len(pending(in.Decisions, have)) == 0
				},
			})
			if err != nil || sha != "" {
				if err != nil {
					return nil, err
				}
				return done(sha)
			}
		}
		have, err := decisionsIn(string(prior))
		if err != nil {
			return nil, fmt.Errorf("the existing decision block in %s does not parse (%v); it is never appended to", path, err)
		}
		todo := pending(in.Decisions, have)
		if len(todo) == 0 {
			// The log already says what was asked, a requester's repair
			// included: its last commit stands for the append.
			sha, err := g.out(ctx, o.repo, "log", "-1", "--format=%H", "refs/heads/"+branch, "--", path)
			if err != nil {
				return nil, err
			}
			if err := g.checkReachable(ctx, o.repo, sha, branch, path, false); err != nil {
				return nil, err
			}
			return done(sha)
		}
		next, err := renderDecisions(prior, todo, o.today())
		if err != nil {
			return nil, err
		}
		sha, err := g.writeAndCommit(ctx, o.repo, branch, path, in.Subject, label, next, pre)
		if err != nil {
			return nil, err
		}
		got, ok, err := g.blob(ctx, o.repo, sha, path)
		if err != nil {
			return nil, err
		}
		if want, _ := decisionsIn(string(next)); !ok || !bytes.Equal(got, next) || !sameKeys(string(got), want) {
			return nil, fmt.Errorf("commit %s: the committed decision log does not parse back to the decisions given", sha)
		}
		return done(sha)
	}
}

func sameKeys(text string, want []string) bool {
	got, err := decisionsIn(text)
	return err == nil && slices.Equal(got, want)
}

// readFile is path's bytes in dir; nil when it does not exist.
func readFile(dir, path string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, path))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// stageFaultSignal is the stop for a code stage's failure (D19): an
// agent-null-kind stop whose statement gives the reason, its referent the
// path, else the unit, else the stage.
func stageFaultSignal(at plan.At, reason string) *plan.Signal {
	ref := at.Path
	if ref == "" {
		ref = at.Unit
	}
	if ref == "" {
		ref = at.Stage
	}
	return plan.NewSignal(plan.Signal{Kind: "agent-null", Source: "structural", Stage: at.Stage, Unit: at.Unit, Wave: at.Wave,
		Statement: at.Stage + " failed: " + reason, Referent: ref})
}

type ledgerInput struct {
	Path              string `json:"path"`
	Name              string `json:"name"`
	IntegrationBranch string `json:"integrationBranch"`
	Block             string `json:"block"`
	Subject           string `json:"subject"`
	PauseRef          string `json:"pauseRef"`
}

// appendLedger is a ledger's text with block appended after one blank
// line; a new ledger starts with its heading.
func appendLedger(prior []byte, priorOK bool, name, block string) []byte {
	p := string(prior)
	if !priorOK {
		p = plan.LedgerHeading(name)
	}
	return []byte(strings.TrimRight(p, "\n") + "\n\n" + block)
}

// ledgerAppend appends the rendered rows to the ledger in the main
// checkout and commits it. The prior text's integrity rests on the guard:
// an earlier stage commit must be intact, and text the requester committed
// is theirs. The pause ref is journaled here and re-read live by the
// caller.
func (o *orch) ledgerAppend(in ledgerInput) stageFn {
	return func(ctx context.Context, label string, interrupted bool) (json.RawMessage, error) {
		g, err := o.gitFor()
		if err != nil {
			return nil, err
		}
		branch, path := in.IntegrationBranch, in.Path
		pre, err := g.head(ctx, o.repo)
		if err != nil {
			return nil, err
		}
		prior, priorOK, err := g.blob(ctx, o.repo, pre, path)
		if err != nil {
			return nil, err
		}
		next := appendLedger(prior, priorOK, in.Name, in.Block)
		if err := o.mainPrecheck(ctx, g, branch, path, interrupted, func(b []byte) bool { return bytes.Equal(b, next) }); err != nil {
			return nil, err
		}
		done := func(sha string) (json.RawMessage, error) {
			// The caller re-reads the pause live; an error here must not fail
			// a step whose commit has landed.
			pause, _ := g.pauseRequested(ctx, o.repo, in.PauseRef)
			return stageValue(scribeResult{Path: path, Sha: sha, PauseRequested: pause})
		}
		if interrupted {
			sha, survived, err := g.adopt(ctx, o.repo, branch, path, adoption{
				label: label,
				matches: func(parent []byte, parentOK bool, cand []byte) bool {
					return bytes.Equal(cand, appendLedger(parent, parentOK, in.Name, in.Block))
				},
				survives: func(tip []byte, ok bool) bool { return ok && bytes.Contains(tip, []byte(in.Block)) },
			})
			if err != nil {
				return nil, err
			}
			if sha != "" {
				return done(sha)
			}
			if survived {
				// Appending would repeat the rows: the block is there, but in
				// no commit this step can vouch for (a rewritten message, or a
				// commit that failed verification).
				return nil, fmt.Errorf("%s already holds these rows, but not in an intact commit of %s; check the ledger and relaunch with args.unscribed emptied of them", path, label)
			}
		}
		sha, err := g.writeAndCommit(ctx, o.repo, branch, path, in.Subject, label, next, pre)
		if err != nil {
			return nil, err
		}
		return done(sha)
	}
}

type completionInput struct {
	Unit    string `json:"unit"`
	Base    string `json:"base"`
	Path    string `json:"path"`
	Text    string `json:"text"`
	Subject string `json:"subject"`
}

// completionReport commits the rendered report on the step's own branch,
// one commit on top of the unit's branch (base). A tip that already is
// that commit, intact, is reused; any other tip, an earlier attempt's
// included, is reset away: the branch is the step's alone.
func (o *orch) completionReport(in completionInput) stageFn {
	return func(ctx context.Context, label string, _ bool) (json.RawMessage, error) {
		g, err := o.gitFor()
		if err != nil {
			return nil, err
		}
		wt, branch, err := o.rt.Worktree(ctx, label)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(in.Base, "-") {
			return nil, fmt.Errorf("unit branch %q is not a branch name", in.Base)
		}
		base, err := g.out(ctx, wt, "rev-parse", "--verify", "--quiet", "refs/heads/"+in.Base+"^{commit}")
		if err != nil {
			return nil, fmt.Errorf("unit branch %s does not resolve: %w", in.Base, err)
		}
		done := func(sha string) (json.RawMessage, error) {
			return stageValue(completionResult{Branch: branch, Sha: sha, Path: in.Path})
		}
		text := []byte(in.Text)
		tip, err := g.out(ctx, wt, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
		if err != nil {
			return nil, err
		}
		if _, ok, err := g.intact(ctx, wt, tip, in.Path); err != nil {
			return nil, err
		} else if ok && g.verifyCommit(ctx, wt, branch, tip, base, in.Path, text) == nil {
			return done(tip)
		}
		if _, err := g.r.run(ctx, wt, "reset", "-q", "--hard", base); err != nil {
			return nil, err
		}
		if _, err := g.r.run(ctx, wt, "clean", "-fdq"); err != nil {
			return nil, err
		}
		sha, err := g.writeAndCommit(ctx, wt, branch, in.Path, in.Subject, label, text, base)
		if err != nil {
			return nil, err
		}
		return done(sha)
	}
}
