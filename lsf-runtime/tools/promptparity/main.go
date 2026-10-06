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
// Node runs under precise coverage. The check fails unless every named
// function in implement.js ran; the uncovered blocks are printed for review.
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
	"slices"
	"strings"

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

// result is one scenario's run on one side.
type result struct {
	Calls   []call          `json:"calls"`
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
		o, _ := plan.MarshalJS(opts{c.Opts.Label, c.Opts.Phase, c.Opts.Model, c.Opts.Effort, c.Opts.AgentType, c.Opts.Isolation, c.Opts.Schema}, "")
		r.Calls = append(r.Calls, call{c.Label, c.Prompt, o})
	}
	return r, jsScenario{Name: sc.name, Args: s.ArgsJSON, Returns: s.Returns}, s.Failures
}

const snippet = `
import { Session } from 'node:inspector/promises';
import { readFileSync } from 'node:fs';
const session = new Session();
session.connect();
await session.post('Profiler.enable');
await session.post('Profiler.startPreciseCoverage', { callCount: true, detailed: true });
const IMPLEMENT = @IMPLEMENT@;
const { run } = await import(IMPLEMENT);
const scenarios = JSON.parse(readFileSync(0, 'utf8'));
const results = [];
for (const sc of scenarios) {
  const calls = [];
  const logs = [];
  const uses = new Map();
  const agent = async (prompt, o) => {
    const n = (uses.get(o.label) ?? 0) + 1;
    uses.set(o.label, n);
    const label = n > 1 ? o.label + '#' + n : o.label;
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
	cmd := exec.Command(node, "--input-type=module", "-e", strings.Replace(snippet, "@IMPLEMENT@", string(mod), 1))
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

// compare returns the differences between the two sides of one scenario.
func compare(g result, j jsResult) []string {
	var diffs []string
	add := func(f string, a ...any) { diffs = append(diffs, fmt.Sprintf(f, a...)) }
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

// prefixes are the label prefixes every scenario together must reach.
var prefixes = append(append([]string{}, unitStages...), "decision-log", "decision-replan", "load", "merge", "review-gate", "cleanup",
	"replan", "ledger", "termination", "direct-edits", "obligations")

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
	seen := map[string]bool{}
	for i, sc := range scenarios {
		for _, c := range goResults[i].Calls {
			base, _, _ := strings.Cut(c.Label, "#")
			seen[strings.Split(base, ":")[0]] = true
		}
		if diffs := compare(goResults[i], js[i]); len(diffs) > 0 {
			failed++
			fmt.Fprintf(w, "FAIL %s\n  %s\n", sc.name, strings.Join(diffs, "\n  "))
		} else if verbose {
			fmt.Fprintf(w, "ok   %s (%d calls)\n", sc.name, len(goResults[i].Calls))
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
	for _, b := range cov.UncoveredBlocks {
		fmt.Fprintf(w, "  uncovered: line %d in %s: %s\n", b.Line, b.Name, b.Text)
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
