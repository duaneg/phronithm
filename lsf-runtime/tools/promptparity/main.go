// Command promptparity checks the Go orchestrator against the JavaScript it
// was ported from (workflows/lsf/implement.js): it runs the same scenarios
// through both and compares, byte for byte, every call's prompt and options
// (schema included), the order of calls in each lane, the logs and the
// return. Run from lsf-runtime/: go run ./tools/promptparity. It needs node.
//
// Each scenario runs first on the Go orchestrator over the orchtest stub,
// which records the return it gave each label. Node then runs implement.js's
// run() on a replay stub that answers each label with the recorded return.
// Both rename the nth use of a label `label#n` (the Go orchestrator's D10),
// so the JS reuse of ledger:w<n> compares too. A unit stage's lane is its
// unit; every other call is the main lane. JS starts concurrent units
// interleaved and the stub runs them in turn, but each lane's order is
// fixed in both.
//
// Load, the decision log, ledger appends, completion reports, the merge and
// cleanup are code steps in Go (orchestrator D18, D24, D25) and agent calls
// in the JS: the JS calls are
// answered from the Go step's recorded value; each Go step's input must
// appear verbatim in the JS prompt it replaces (stepParity), and the calls
// are then left out of the comparison. The coverage checks count the Go
// steps.
//
// The Go side's gate-omitted is rewritten to the JS's gate-not-run before
// comparing, statement included (toJSKinds; orchestrator D22). No scenario
// reaches a code-stage fault, so stage-fault (D19) is not rewritten: one
// would show as a difference.
//
// Node runs under precise coverage. The check fails unless every named
// function in implement.js ran and every block that did not is listed in
// unreachable, with the reason run() cannot reach it.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator"
	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

// call is one agent call as either side made it. Opts holds the options as
// JSON, schema embedded.
type call struct {
	Label  string          `json:"label"`
	Prompt string          `json:"prompt"`
	Opts   json.RawMessage `json:"opts"`
}

// step is a Go code step: its label and input.
type step struct {
	Label string
	Input json.RawMessage
}

// result is one scenario's run on one side. Steps are the Go side's code
// steps, which are not calls.
type result struct {
	Calls   []call          `json:"calls"`
	Steps   []step          `json:"-"`
	Logs    []string        `json:"logs"`
	Outcome json.RawMessage `json:"outcome"` // null on error
	Error   string          `json:"error"`
}

// jsScenario is what node gets: the args and the recorded returns.
type jsScenario struct {
	Name    string                     `json:"name"`
	Args    json.RawMessage            `json:"args"`
	Returns map[string]json.RawMessage `json:"returns"`
}

// unitStages are the label prefixes of the per-unit stages.
var unitStages = []string{"plan", "critique-plan", "fix-plan", "implement", "critique-code", "lens-review", "critique-design", "fix-code", "completion-report"}

// opts is a call's options in a fixed shape for comparison; the schema is
// compared as bytes, both sides keeping the JS key order.
type opts struct {
	Label     string          `json:"label"`
	Phase     string          `json:"phase,omitempty"`
	Model     string          `json:"model,omitempty"`
	Effort    string          `json:"effort,omitempty"`
	AgentType string          `json:"agentType,omitempty"`
	Isolation string          `json:"isolation,omitempty"`
	Schema    json.RawMessage `json:"schema"`
}

func runGo(sc scenario) (result, jsScenario, []string) {
	p, hs, args := sc.build()
	s := orchtest.New(nil, p, hs, args)
	out, err := orchestrator.Run(context.Background(), s)
	r := result{Logs: s.Logs, Outcome: json.RawMessage("null")}
	if r.Logs == nil {
		r.Logs = []string{}
	}
	if err != nil {
		r.Error = err.Error()
	} else {
		r.Outcome = out
	}
	for _, c := range s.Calls {
		if c.Step {
			// gates-pin is Go's own (D31): JS has no gate block.
			if c.Label != "gates-pin" {
				r.Steps = append(r.Steps, step{c.Label, c.Input})
			}
			continue
		}
		o, _ := plan.MarshalJS(opts{c.Opts.Label, c.Opts.Phase, c.Opts.Model, c.Opts.Effort, c.Opts.AgentType, c.Opts.Isolation, c.Opts.Schema}, "")
		r.Calls = append(r.Calls, call{c.Label, c.Prompt, o})
	}
	return r, jsScenario{Name: sc.name, Args: s.ArgsJSON, Returns: s.Returns}, s.Failures
}

const snippet = `
if (Number(process.versions.node.split('.')[0]) < 20) throw new Error('promptparity needs node 20 or later, not ' + process.versions.node);
const { Session } = await import('node:inspector/promises');
import { readFileSync } from 'node:fs';
const session = new Session();
session.connect();
await session.post('Profiler.enable');
await session.post('Profiler.startPreciseCoverage', { callCount: true, detailed: true });
const IMPLEMENT = @IMPLEMENT@;
const { run } = await import(IMPLEMENT);
const scenarios = JSON.parse(readFileSync(0, 'utf8'));
const KNOWN = ['label', 'phase', 'model', 'effort', 'agentType', 'isolation', 'schema'];
const results = [];
for (const sc of scenarios) {
  const calls = [];
  const logs = [];
  const uses = new Map();
  const agent = async (prompt, o) => {
    const n = (uses.get(o.label) ?? 0) + 1;
    uses.set(o.label, n);
    const label = n > 1 ? o.label + '#' + n : o.label;
    const extra = Object.keys(o).filter((k) => !KNOWN.includes(k));
    if (extra.length) throw new Error('unknown opts keys on ' + label + ': ' + extra.join(', '));
    calls.push({ label, prompt, opts: JSON.stringify({ label, phase: o.phase, model: o.model, effort: o.effort, agentType: o.agentType, isolation: o.isolation, schema: o.schema }) });
    if (!Object.hasOwn(sc.returns, label)) throw new Error('no recorded return for ' + label);
    return JSON.parse(JSON.stringify(sc.returns[label]));
  };
  const parallel = async (thunks) => Promise.all(thunks.map((t) => t().catch(() => null)));
  const rt = { agent, parallel, log: (m) => logs.push(m), args: sc.args };
  let outcome = null, error = '';
  try { outcome = JSON.stringify(await run(rt)); } catch (e) { error = e.message; }
  results.push({ calls: calls.map((c) => ({ ...c, opts: JSON.parse(c.opts) })), logs, outcome: outcome === null ? null : JSON.parse(outcome), outcomeText: outcome, error });
}
const { result } = await session.post('Profiler.takePreciseCoverage');
const src = readFileSync(new URL(IMPLEMENT), 'utf8');
const line = (off) => src.slice(0, off).split('\n').length;
const cov = result.find((s) => s.url === IMPLEMENT);
if (!cov) throw new Error('no coverage recorded for ' + IMPLEMENT);
const uncoveredFunctions = [], uncoveredBlocks = [];
for (const f of cov.functions) {
  const [whole, ...blocks] = f.ranges;
  if (whole.count === 0) {
    (f.functionName ? uncoveredFunctions : uncoveredBlocks).push({ name: f.functionName || '(anonymous)', line: line(whole.startOffset) });
    continue;
  }
  for (const b of blocks) if (b.count === 0) uncoveredBlocks.push({ name: f.functionName || '(anonymous)', line: line(b.startOffset), text: src.slice(b.startOffset, Math.min(b.endOffset, b.startOffset + 70)).replace(/\s+/g, ' ') });
}
const functions = cov.functions.filter((f) => f.functionName).length;
process.stdout.write(JSON.stringify({ results, coverage: { functions, uncoveredFunctions, uncoveredBlocks } }));
`

type jsResult struct {
	Calls       []call          `json:"calls"`
	Logs        []string        `json:"logs"`
	OutcomeText *string         `json:"outcomeText"`
	Error       string          `json:"error"`
	Outcome     json.RawMessage `json:"outcome"`
}

type coverage struct {
	Functions          int `json:"functions"`
	UncoveredFunctions []struct {
		Name string `json:"name"`
		Line int    `json:"line"`
	} `json:"uncoveredFunctions"`
	UncoveredBlocks []struct {
		Name string `json:"name"`
		Line int    `json:"line"`
		Text string `json:"text"`
	} `json:"uncoveredBlocks"`
}

func runNode(repo string, in []jsScenario) ([]jsResult, coverage, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return nil, coverage{}, fmt.Errorf("node not found: %w", err)
	}
	u := url.URL{Scheme: "file", Path: filepath.Join(repo, "workflows", "lsf", "implement.js")}
	mod, _ := json.Marshal(u.String())
	b, err := json.Marshal(in)
	if err != nil {
		return nil, coverage{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "--input-type=module", "-e", strings.Replace(snippet, "@IMPLEMENT@", string(mod), 1))
	cmd.Stdin = bytes.NewReader(b)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, coverage{}, fmt.Errorf("node: %w", err)
	}
	var r struct {
		Results  []jsResult `json:"results"`
		Coverage coverage   `json:"coverage"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, coverage{}, fmt.Errorf("node output: %w", err)
	}
	if len(r.Results) != len(in) {
		return nil, coverage{}, fmt.Errorf("node returned %d results for %d scenarios", len(r.Results), len(in))
	}
	return r.Results, r.Coverage, nil
}

// lane is the call's lane: its unit for a unit stage, else main.
func lane(label string) string {
	base, _, _ := strings.Cut(label, "#")
	parts := strings.Split(base, ":")
	if slices.Contains(unitStages, parts[0]) {
		return "unit " + parts[len(parts)-1]
	}
	return "main"
}

// barriers is, per label, how many main-lane calls precede it: where each
// unit stage falls among the waves' merges, gates, re-plans and scribes.
func barriers(calls []call) map[string]int {
	out := map[string]int{}
	n := 0
	for _, c := range calls {
		out[c.Label] = n
		if lane(c.Label) == "main" {
			n++
		}
	}
	return out
}

func lanes(calls []call) map[string][]string {
	out := map[string][]string{}
	for _, c := range calls {
		out[lane(c.Label)] = append(out[lane(c.Label)], c.Label)
	}
	return out
}

// normOpts re-encodes a JS opts object in the Go field order, dropping
// undefined fields (JSON.stringify omits them).
func normOpts(raw json.RawMessage) (string, error) {
	var o opts
	if err := json.Unmarshal(raw, &o); err != nil {
		return "", err
	}
	b, err := plan.MarshalJS(o, "")
	return string(b), err
}

// codeStep reports whether a JS call is one the Go orchestrator makes as a
// code step.
func codeStep(label string) bool {
	base, _, _ := strings.Cut(label, "#")
	return base == "load" || base == "decision-log" ||
		strings.HasPrefix(base, "load:") || strings.HasPrefix(base, "ledger:") || strings.HasPrefix(base, "completion-report:") ||
		strings.HasPrefix(base, "merge:") || strings.HasPrefix(base, "cleanup:")
}

// stepParity checks a Go step's input against the JS prompt for the same
// label: each field the JS embeds in its prompt — the block or report it
// copies, the commit subject, the SHA it reads, the decisions — appears
// there verbatim. It returns what is missing.
func stepParity(s step, prompt string) []string {
	var in struct {
		Sha, PlanPath, DecisionLogSha, DecisionLogPath, PauseRef string
		Path, Subject, Block, Text, Base                         string
		Decisions                                                json.RawMessage
		Wave                                                     int
		Name, IntegrationBranch, ArchivePath, LedgerPath         string
		ArchitectureDoc                                          string
		Units                                                    []struct{ Unit, Branch, Report string }
	}
	if err := json.Unmarshal(s.Input, &in); err != nil {
		return []string{"input: " + err.Error()}
	}
	base, _, _ := strings.Cut(s.Label, "#")
	var want []string
	subject := `subject line "` + in.Subject + `"`
	switch {
	case base == "load" || strings.HasPrefix(base, "load:"):
		want = []string{"Run `git show " + in.Sha + ":" + in.PlanPath + "`", "git show-ref --verify --quiet " + in.PauseRef + " "}
		if in.DecisionLogSha != "" {
			want = append(want, "Run `git show "+in.DecisionLogSha+":"+in.DecisionLogPath+"`")
		} else {
			want = append(want, "There is no decision log")
		}
	case base == "decision-log":
		var b bytes.Buffer
		if err := json.Indent(&b, in.Decisions, "", "  "); err != nil {
			return []string{"decisions: " + err.Error()}
		}
		want = []string{"Append the following requester decisions to " + in.Path + " ", "Decisions to append:\n" + b.String() + "\n\n", subject}
	case strings.HasPrefix(base, "ledger:"):
		want = []string{"<<<LEDGER\n" + in.Block + "LEDGER>>>", "to the end of " + in.Path + ",", subject, "git show-ref --verify --quiet " + in.PauseRef + " "}
	case strings.HasPrefix(base, "completion-report:"):
		want = []string{"<<<REPORT\n" + in.Text + "REPORT>>>", "below to " + in.Path + ",", subject, "git reset --hard " + in.Base + " &&"}
	case strings.HasPrefix(base, "cleanup:"):
		want = []string{"Cleanup stage, wave " + strconv.Itoa(in.Wave) + ". You are in the main checkout on " + in.IntegrationBranch + "."}
	case strings.HasPrefix(base, "merge:"):
		w := strconv.Itoa(in.Wave)
		lines := []string{}
		for _, u := range in.Units {
			lines = append(lines, "- "+u.Unit+": branch "+u.Branch+" (completion report "+u.Report+")")
		}
		want = []string{
			"You are in the main checkout on " + in.IntegrationBranch + ". Merge each unit branch below into " + in.IntegrationBranch + ",",
			"`git merge --no-ff -m \"merge(" + in.Name + "): <unit id>\"`",
			"entry under `## Work Units` in " + in.PlanPath + " to " + in.ArchivePath + ",",
			"Ledger: " + in.LedgerPath + " wave " + w + ".`",
			"(`git rev-list -1 " + in.IntegrationBranch + " -- " + in.ArchitectureDoc + "`)",
			"git show-ref --verify --quiet " + in.PauseRef + " ",
			"Wave " + w + " merge order:\n" + strings.Join(lines, "\n") + "\n",
		}
	}
	missing := []string{}
	for _, w := range want {
		if !strings.Contains(prompt, w) {
			missing = append(missing, trunc(w))
		}
	}
	return missing
}

func trunc(s string) string {
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

// jsKinds rewrites the Go side's gate-omitted to the JS's gate-not-run,
// statement included (orchestrator D22), over the outcome, the logs, every
// prompt and every step input, since stop text reaches the re-plan prompt
// and the ledger. A scenario with both kinds is two Go stops against one JS
// stop, and shows as a difference.
var jsKinds = strings.NewReplacer(
	"gate(s) omitted:", "gate(s) not run:",
	"gate-omitted", "gate-not-run",
)

// rewritten counts the scenarios toJSKinds changed: the rewrite must be live.
var rewritten int

func toJSKinds(g result) result {
	if strings.Contains(string(g.Outcome), "gate-omitted") {
		rewritten++
	}
	g.Outcome = json.RawMessage(jsKinds.Replace(string(g.Outcome)))
	g.Logs = slices.Clone(g.Logs)
	for i, l := range g.Logs {
		g.Logs[i] = jsKinds.Replace(l)
	}
	g.Calls = slices.Clone(g.Calls)
	for i := range g.Calls {
		g.Calls[i].Prompt = jsKinds.Replace(g.Calls[i].Prompt)
	}
	g.Steps = slices.Clone(g.Steps)
	for i := range g.Steps {
		g.Steps[i].Input = json.RawMessage(jsKinds.Replace(string(g.Steps[i].Input)))
	}
	return g
}

// d30 rewrites the frozen JS prompts to the Go wording for per-call clones
// (orchestrator D30): each pair once per match, every pair required to be
// live, so a Go change it does not name, or one it names and Go lacks,
// still shows as a difference. Only the compared calls are rewritten.
var d30 = []struct {
	old  *regexp.Regexp
	new  string
	used int
}{
	{old: regexp.MustCompile(regexp.QuoteMeta("You are running in a git worktree of the repository. Before anything else run `git reset --hard ")),
		new: "You are running in a clone of the repository, made for this stage; every branch named here is a local branch in it. Before anything else run `git reset --hard "},
	{old: regexp.MustCompile(regexp.QuoteMeta("` so the worktree matches that branch exactly, with no leftover untracked files from an earlier attempt. Make all changes and commits here; never touch other worktrees or the main checkout.")),
		new: "` so the clone matches that branch exactly, with no leftover untracked files from an earlier attempt. Make all changes and commits here, on the branch checked out; the runtime fetches that branch when you return."},
	{old: regexp.MustCompile(`You are in the main checkout on ([^ ]+)\. `),
		new: "You are in a clone of the repository on $1, made for this stage; the runtime fast-forwards $1 to your commits after you return. "},
	{old: regexp.MustCompile(regexp.QuoteMeta("The main checkout may hold uncommitted changes that are not yours: the requester can edit it while this run is live. Commit only")),
		new: "Commit only"},
	{old: regexp.MustCompile(regexp.QuoteMeta("`git reset --hard` in the main checkout, and never stage")),
		new: "`git reset --hard`, and never stage"},
}

// toD30 applies d30 to j's agent calls (not the code steps', whose JS
// prompts only stepParity reads).
func toD30(j jsResult) jsResult {
	j.Calls = slices.Clone(j.Calls)
	for i, c := range j.Calls {
		if codeStep(c.Label) {
			continue
		}
		for k := range d30 {
			if d30[k].old.MatchString(c.Prompt) {
				d30[k].used++
				c.Prompt = d30[k].old.ReplaceAllString(c.Prompt, d30[k].new)
			}
		}
		j.Calls[i] = c
	}
	return j
}

// compare returns the differences between the two sides of one scenario.
// The JS calls Go makes as code steps are checked against the steps'
// inputs (stepParity) and dropped.
func compare(g result, j jsResult) []string {
	var diffs []string
	add := func(f string, a ...any) { diffs = append(diffs, fmt.Sprintf(f, a...)) }
	jsSteps := map[string]string{}
	for _, c := range j.Calls {
		if codeStep(c.Label) {
			jsSteps[c.Label] = c.Prompt
		}
	}
	for _, s := range g.Steps {
		prompt, ok := jsSteps[s.Label]
		if !ok {
			add("step %s: go only", s.Label)
			continue
		}
		delete(jsSteps, s.Label)
		if missing := stepParity(s, prompt); len(missing) > 0 {
			add("step %s: the JS prompt lacks the step's input: %q", s.Label, missing)
		}
	}
	for l := range jsSteps {
		add("step %s: js only", l)
	}
	j.Calls = slices.DeleteFunc(slices.Clone(j.Calls), func(c call) bool { return codeStep(c.Label) })
	if g.Error != j.Error {
		add("error: go %q, js %q", g.Error, j.Error)
	}
	if g.Error == "" && j.Error == "" && (j.OutcomeText == nil || string(g.Outcome) != *j.OutcomeText) {
		line, a, b := firstDiff(string(g.Outcome), deref(j.OutcomeText))
		add("outcome differs at line %d:\n    go: %s\n    js: %s", line, a, b)
	}
	if !slices.Equal(g.Logs, j.Logs) {
		for i := 0; i < len(g.Logs) || i < len(j.Logs); i++ {
			a, b := at(g.Logs, i), at(j.Logs, i)
			if a != b {
				add("log %d:\n    go: %s\n    js: %s", i, a, b)
				break
			}
		}
	}
	jsCalls := map[string]call{}
	for _, c := range j.Calls {
		if _, dup := jsCalls[c.Label]; dup {
			add("js label %s repeated after renaming", c.Label)
		}
		jsCalls[c.Label] = c
	}
	goCalls := map[string]call{}
	for _, c := range g.Calls {
		goCalls[c.Label] = c
		jc, ok := jsCalls[c.Label]
		if !ok {
			add("call %s: go only", c.Label)
			continue
		}
		if c.Prompt != jc.Prompt {
			line, a, b := firstDiff(c.Prompt, jc.Prompt)
			add("call %s: prompt differs at line %d:\n    go: %s\n    js: %s", c.Label, line, a, b)
		}
		jo, err := normOpts(jc.Opts)
		if err != nil {
			add("call %s: js opts: %v", c.Label, err)
		} else if jo != string(c.Opts) {
			add("call %s: opts differ:\n    go: %s\n    js: %s", c.Label, c.Opts, jo)
		}
	}
	for _, c := range j.Calls {
		if _, ok := goCalls[c.Label]; !ok {
			add("call %s: js only", c.Label)
		}
	}
	gb, jb := barriers(g.Calls), barriers(j.Calls)
	for label, n := range gb {
		if m, ok := jb[label]; ok && m != n {
			add("call %s: after %d main-lane call(s) in go, %d in js", label, n, m)
		}
	}
	gl, jl := lanes(g.Calls), lanes(j.Calls)
	for name, seq := range gl {
		if !slices.Equal(seq, jl[name]) {
			add("lane %s order:\n    go: %v\n    js: %v", name, seq, jl[name])
		}
	}
	return diffs
}

func deref(s *string) string {
	if s == nil {
		return "<none>"
	}
	return *s
}

func at(xs []string, i int) string {
	if i < len(xs) {
		return xs[i]
	}
	return "<none>"
}

// firstDiff returns the first line where a and b differ, 1-based.
func firstDiff(a, b string) (int, string, string) {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) || i < len(bl); i++ {
		x, y := at(al, i), at(bl, i)
		if x != y {
			return i + 1, x, y
		}
	}
	return 0, "", ""
}

// unreachable are the implement.js blocks no scenario can reach through
// run(), keyed by function and the block's opening text, each with why.
// A block not listed here that no scenario runs fails the check.
var unreachable = map[[2]string]string{
	{"renderGates", ": '- none reported'"}:                        "a stage with no gates stops before review (Go: gate-omitted, D22); a design review renders none",
	{"byteLength", ": text.length"}:                               "node has TextEncoder",
	{"renderCompletionReport", "?? {}"}:                           "doneCondition is schema-required; Go decodes absent as zero (D9)",
	{"renderCompletionReport", "?? []"}:                           "writesOutsideScope and gateProbe are schema-required (D9)",
	{"renderCompletionReport", "? ' (not evaluable as written)'"}: "an unevaluable done condition stops the unit before the report",
	{"renderCompletionReport", "?? ''"}:                           "evidence is schema-required (D9)",
	{"count", "?? []"}:                                            "review.findings is always set by the review gate",
	{"gatePrompt", "?? 'HEAD'"}:                                   "the gate runs after the load has set the pin",
	{"replanPrompt", ": 'none'"}:                                  "a claimed-disjoint conflict is always also a re-plan item",
	{"critiqueLoop", "?? []"}:                                     "both onFix callbacks return an array",
	{"runUnit", "?? LENSES_BY_TYPE.feature"}:                      "validatePlan rejects an unknown unit type",
	{"round", ": []"}:                                             "the design branch builds no lens reviewer",
	{"(anonymous)", "?? null"}:                                    "a reviewer slot is null only on a JS exception in agent(); the Go runtime's Call errors are stops",
	{"onFix", "?? []"}:                                            "an implement return without gates stops before review (Go: gate-omitted, D22)",
	{"run", "?? {}"}:                                              "every runtime supplies args; Go reads nil args as {} (runtime_test.go)",
	{"(anonymous)", "?? { unit: u.id, type: u.type, branch: null, completionPath: nul"}: "a unit is null only on a JS exception in runUnit; Go's analogue, a panic, is TestPanickingUnitIsUnitNull",
}

// listed reports whether the block is on the allowlist, marking the entry
// used.
func listed(name, text string, used map[[2]string]bool) bool {
	for k := range unreachable {
		if k[0] == name && strings.HasPrefix(text, k[1]) {
			used[k] = true
			return true
		}
	}
	return false
}

// labels are the labels of r's calls and steps.
func (r result) labels() []string {
	out := []string{}
	for _, s := range r.Steps {
		out = append(out, s.Label)
	}
	for _, c := range r.Calls {
		out = append(out, c.Label)
	}
	return out
}

// prefixes are the label prefixes every scenario together must reach, by
// a call or a step.
var prefixes = append(append([]string{}, unitStages...), "decision-log", "decision-replan", "load", "merge", "review-gate", "cleanup",
	"replan", "ledger", "termination", "direct-edits", "obligations")

// labels are exact label shapes some scenario must reach, by a call or a
// step: the variants a prefix alone does not show.
var labels = []string{"direct-edits", "direct-edits:w0", "direct-edits:w1", "ledger:w0", "ledger:carried", "load:termination", "load:w1",
	"fix-plan:r1:A", "fix-code:r1:A", "fix-code:r1:D", "critique-design:r1:D"}

func run(repo string, verbose bool, w io.Writer) (failed, total int, err error) {
	var goResults []result
	var inputs []jsScenario
	for _, sc := range scenarios {
		r, in, failures := runGo(sc)
		if len(failures) > 0 {
			return 0, 0, fmt.Errorf("%s: harness: %s", sc.name, strings.Join(failures, "; "))
		}
		goResults = append(goResults, r)
		inputs = append(inputs, in)
	}
	js, cov, err := runNode(repo, inputs)
	if err != nil {
		return 0, 0, err
	}
	rewritten = 0
	seen := map[string]bool{}      // prefixes reached, by calls and steps
	seenLabel := map[string]bool{} // labels reached, by calls and steps
	for i, sc := range scenarios {
		for _, l := range goResults[i].labels() {
			seenLabel[l] = true
			base, _, _ := strings.Cut(l, "#")
			seen[strings.Split(base, ":")[0]] = true
		}
		if diffs := compare(toJSKinds(goResults[i]), toD30(js[i])); len(diffs) > 0 {
			failed++
			fmt.Fprintf(w, "FAIL %s\n  %s\n", sc.name, strings.Join(diffs, "\n  "))
		} else if verbose {
			fmt.Fprintf(w, "ok   %s (%d calls)\n", sc.name, len(goResults[i].Calls))
		}
	}
	if rewritten == 0 {
		failed++
		fmt.Fprintln(w, "FAIL coverage: no scenario reaches gate-omitted, so its rewrite is untested")
	}
	for _, r := range d30 {
		if r.used == 0 {
			failed++
			fmt.Fprintf(w, "FAIL coverage: the D30 rewrite of %q matches no JS prompt\n", r.old)
		}
	}
	for _, l := range labels {
		if !seenLabel[l] {
			failed++
			fmt.Fprintf(w, "FAIL coverage: no scenario makes call %s\n", l)
		}
	}
	for _, p := range prefixes {
		if !seen[p] {
			failed++
			fmt.Fprintf(w, "FAIL coverage: no scenario reaches %s\n", p)
		}
	}
	for _, f := range cov.UncoveredFunctions {
		failed++
		fmt.Fprintf(w, "FAIL coverage: implement.js function %s (line %d) never ran\n", f.Name, f.Line)
	}
	fmt.Fprintf(w, "implement.js: %d named functions, all run: %t; %d uncovered block(s)\n", cov.Functions, len(cov.UncoveredFunctions) == 0, len(cov.UncoveredBlocks))
	used := map[[2]string]bool{}
	for _, b := range cov.UncoveredBlocks {
		if !listed(b.Name, b.Text, used) {
			failed++
			fmt.Fprintf(w, "FAIL coverage: implement.js line %d in %s never ran and is not listed unreachable: %s\n", b.Line, b.Name, b.Text)
		} else if verbose {
			fmt.Fprintf(w, "  unreachable: line %d in %s: %s\n", b.Line, b.Name, b.Text)
		}
	}
	for k := range unreachable {
		if !used[k] {
			failed++
			fmt.Fprintf(w, "FAIL coverage: unreachable entry %s %q now runs or no longer exists: remove it\n", k[0], k[1])
		}
	}
	return failed, len(scenarios), nil
}

func main() {
	repo := flag.String("repo", "..", "repository root")
	verbose := flag.Bool("v", false, "list every scenario")
	flag.Parse()
	root, err := filepath.Abs(*repo)
	if err == nil {
		var failed, total int
		failed, total, err = run(root, *verbose, os.Stdout)
		if err == nil {
			if failed > 0 {
				fmt.Printf("promptparity: %d failure(s) over %d scenarios\n", failed, total)
				os.Exit(1)
			}
			fmt.Printf("promptparity: %d scenarios identical\n", total)
			return
		}
	}
	fmt.Fprintln(os.Stderr, "promptparity:", err)
	os.Exit(2)
}
