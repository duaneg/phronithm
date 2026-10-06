// Command graphparity checks the Go plan model against the JavaScript it was
// ported from (workflows/lsf/{schema,graph,scheduling}.js): it runs the same
// cases through both and compares the JSON byte for byte. Run from
// lsf-runtime/: go run ./tools/graphparity. It needs node.
//
// Cases: every fixture body in plan/testdata/graph (parseYaml,
// parsePlanGraph, parseDecisions); every fixture plan document there,
// paired with its decision log when one exists, and the repository's plan
// blocks (loadGraph; then validatePlan, and on a valid plan readySet,
// blockedUnits and wavePlan); and the exported constants.
//
// Inputs reach node as JSON, so invalid UTF-8 arrives there as U+FFFD while
// Go parses the raw bytes; the fixtures are valid UTF-8.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

// A case is one call, run in both languages.
type jsCase struct {
	Name      string  `json:"name"`
	Op        string  `json:"op"`
	Text      string  `json:"text"`
	Decisions *string `json:"decisions"`
}

const snippet = `
import { readFileSync } from 'node:fs';
const { parseYaml, parsePlanGraph, parseDecisions, loadGraph } = await import(@GRAPH@);
const S = await import(@SCHEDULING@);
const C = await import(@SCHEMA@);
const run = (c) => {
  switch (c.op) {
    case 'yaml':
      try { return { value: parseYaml(c.text) }; } catch (e) { return { error: e.message }; }
    case 'planGraph': return parsePlanGraph(c.text);
    case 'planProblems': return parsePlanGraph(c.text).problems;
    case 'decisions': return parseDecisions(c.text);
    case 'load': return loadGraph({ planMarkdown: c.text, decisionsMarkdown: c.decisions });
    case 'schedule': {
      const { plan } = loadGraph({ planMarkdown: c.text, decisionsMarkdown: c.decisions });
      if (!plan) return { validate: null };
      const validate = S.validatePlan(plan);
      if (validate.length) return { validate };
      return { validate, ready: S.readySet(plan).map((u) => u.id), blocked: S.blockedUnits(plan), waves: S.wavePlan(plan) };
    }
    case 'constants': return {
      STOP_KINDS: C.STOP_KINDS, RECORD_KINDS: C.RECORD_KINDS, SOURCES: C.SOURCES, GATE_KINDS: C.GATE_KINDS,
      UNIT_TYPES: C.UNIT_TYPES, AMENDMENT_KINDS: C.AMENDMENT_KINDS, INVALIDATION_FIELDS: C.INVALIDATION_FIELDS,
      STAGE_COMMON_REQUIRED: C.STAGE_COMMON_REQUIRED, STAGE_COMMON_PROPERTIES: C.STAGE_COMMON_PROPERTIES,
      INVALIDATION_SCHEMA: C.INVALIDATION_SCHEMA, PLAN_SCHEMA: C.PLAN_SCHEMA, MERGE_RESULT_SCHEMA: C.MERGE_RESULT_SCHEMA,
      DISTRESS_SCHEMA: C.DISTRESS_SCHEMA, stageSchema: C.stageSchema(JSON.parse(c.text)),
      BLOCKING_GATE_KINDS: S.BLOCKING_GATE_KINDS, REPLAN_ITEM_KINDS: S.REPLAN_ITEM_KINDS,
      PLAN_SUPERSEDING_STOP_KINDS: S.PLAN_SUPERSEDING_STOP_KINDS, DEFAULT_CONFIG: S.DEFAULT_CONFIG,
      OVERSIZE_FACTOR: S.OVERSIZE_FACTOR,
    };
  }
  throw new Error('unknown op ' + c.op);
};
const cases = JSON.parse(readFileSync(0, 'utf8'));
process.stdout.write(JSON.stringify(cases.map((c) => {
  try { return JSON.stringify(run(c), null, 2); } catch (e) { return JSON.stringify({ threw: e.message }, null, 2); }
})));
`

func goRun(c jsCase) any {
	switch c.Op {
	case "yaml":
		v, err := plan.ParseYAML(c.Text)
		if err != nil {
			return struct {
				Error string `json:"error"`
			}{err.Error()}
		}
		return struct {
			Value any `json:"value"`
		}{v}
	case "planGraph":
		p, problems := plan.ParsePlanGraph(c.Text)
		return struct {
			Plan     *plan.Plan     `json:"plan"`
			Problems []plan.Problem `json:"problems"`
		}{p, problems}
	case "planProblems":
		_, problems := plan.ParsePlanGraph(c.Text)
		return problems
	case "decisions":
		ds, problems := plan.ParseDecisions(&c.Text)
		return struct {
			Decisions []plan.Decision `json:"decisions"`
			Problems  []plan.Problem  `json:"problems"`
		}{ds, problems}
	case "load":
		return plan.LoadGraph(c.Text, c.Decisions)
	case "schedule":
		return schedule(c)
	case "constants":
		return constants(c)
	}
	panic("unknown op " + c.Op)
}

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

func buildCases(repo string) ([]jsCase, error) {
	dir := filepath.Join(repo, "lsf-runtime", "plan", "testdata", "graph")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var cases []jsCase
	yamlFiles, planFiles := 0, 0
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		text, err := readFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		switch {
		case strings.HasSuffix(name, ".d4.yaml"):
			// Recorded divergence D4: a non-string or quoted-empty type or kind
			// is kept by JS and "" in Go, so the graph differs by design. The
			// problems (text, order, the JSON of the raw value) must match.
			yamlFiles++
			for _, op := range []string{"yaml", "planProblems", "decisions"} {
				cases = append(cases, jsCase{Name: name + " " + op, Op: op, Text: text})
			}
		case strings.HasSuffix(name, ".yaml"):
			yamlFiles++
			for _, op := range []string{"yaml", "planGraph", "decisions"} {
				cases = append(cases, jsCase{Name: name + " " + op, Op: op, Text: text})
			}
		case strings.HasSuffix(name, ".decisions.md"):
			// Paired with its plan document below.
		case strings.HasSuffix(name, ".md"):
			c := jsCase{Name: name + " load", Op: "load", Text: text}
			stem := strings.TrimSuffix(strings.TrimSuffix(name, ".md"), ".plan")
			if log, err := readFile(filepath.Join(dir, stem+".decisions.md")); err == nil {
				c.Decisions = &log
				c.Name = name + "+" + stem + ".decisions.md load"
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			cases = append(cases, c)
			s := c
			s.Name, s.Op = strings.TrimSuffix(c.Name, " load")+" schedule", "schedule"
			cases = append(cases, s)
			planFiles++
		default:
			return nil, fmt.Errorf("%s: fixture with no case (want .yaml or .md)", name)
		}
	}
	if yamlFiles == 0 || planFiles == 0 {
		return nil, fmt.Errorf("%s: %d YAML and %d plan fixtures; want some of each", dir, yamlFiles, planFiles)
	}
	breakdown, err := readFile(filepath.Join(repo, "docs", "plans", "lsf-improvements-breakdown.md"))
	if err != nil {
		return nil, fmt.Errorf("required input: %w", err)
	}
	cases = append(cases,
		jsCase{Name: "docs/plans/lsf-improvements-breakdown.md load", Op: "load", Text: breakdown},
		jsCase{Name: "docs/plans/lsf-improvements-breakdown.md schedule", Op: "schedule", Text: breakdown})
	example, err := readFile(filepath.Join(repo, "docs", "plan-graph.md"))
	if err != nil {
		return nil, fmt.Errorf("required input: %w", err)
	}
	cases = append(cases,
		jsCase{Name: "docs/plan-graph.md load", Op: "load", Text: example, Decisions: &example},
		jsCase{Name: "docs/plan-graph.md schedule", Op: "schedule", Text: example, Decisions: &example},
		jsCase{Name: "constants", Op: "constants", Text: `{"type":"object","properties":{"x":{"type":"string"}},"required":["x"]}`})
	return cases, nil
}

func runNode(repo string, cases []jsCase) ([]string, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}
	module := func(name string) string {
		u := url.URL{Scheme: "file", Path: filepath.Join(repo, "workflows", "lsf", name)}
		b, _ := json.Marshal(u.String())
		return string(b)
	}
	in, err := json.Marshal(cases)
	if err != nil {
		return nil, err
	}
	script := strings.NewReplacer("@GRAPH@", module("graph.js"), "@SCHEDULING@", module("scheduling.js"), "@SCHEMA@", module("schema.js")).Replace(snippet)
	cmd := exec.Command(node, "--input-type=module", "-e", script)
	cmd.Stdin = bytes.NewReader(in)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("node: %w", err)
	}
	var results []string
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("node output: %w", err)
	}
	if len(results) != len(cases) {
		return nil, fmt.Errorf("node returned %d results for %d cases", len(results), len(cases))
	}
	return results, nil
}

// firstDiff returns the first line where a and b differ, 1-based.
func firstDiff(a, b string) (int, string, string) {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) || i < len(bl); i++ {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y || i >= len(al) || i >= len(bl) {
			return i + 1, x, y
		}
	}
	return 0, "", ""
}

// run compares every case, writing a line per mismatch (and per match
// when verbose) to w. It returns the number of cases that differ and the
// number run.
func run(repo string, verbose bool, w io.Writer) (failed, total int, err error) {
	cases, err := buildCases(repo)
	if err != nil {
		return 0, 0, err
	}
	js, err := runNode(repo, cases)
	if err != nil {
		return 0, 0, err
	}
	for i, c := range cases {
		b, err := plan.MarshalJS(goRun(c), "  ")
		if err != nil {
			fmt.Fprintf(w, "FAIL %s: marshal: %v\n", c.Name, err)
			failed++
			continue
		}
		if string(b) != js[i] {
			line, g, j := firstDiff(string(b), js[i])
			fmt.Fprintf(w, "FAIL %s: line %d\n  go: %s\n  js: %s\n", c.Name, line, g, j)
			failed++
		} else if verbose {
			fmt.Fprintf(w, "ok   %s\n", c.Name)
		}
	}
	return failed, len(cases), nil
}

func main() {
	repo := flag.String("repo", "..", "repository root")
	verbose := flag.Bool("v", false, "list every case")
	flag.Parse()
	root, err := filepath.Abs(*repo)
	if err == nil {
		var failed, total int
		failed, total, err = run(root, *verbose, os.Stdout)
		if err == nil {
			if failed > 0 {
				fmt.Printf("graphparity: %d of %d cases differ\n", failed, total)
				os.Exit(1)
			}
			fmt.Printf("graphparity: %d cases identical\n", total)
			return
		}
	}
	fmt.Fprintln(os.Stderr, "graphparity:", err)
	os.Exit(2)
}

// schedule is the load stage's validation and, on a valid plan, the
// scheduling views.
func schedule(c jsCase) any {
	g := plan.LoadGraph(c.Text, c.Decisions)
	if g.Plan == nil {
		return struct {
			Validate []plan.Problem `json:"validate"`
		}{nil}
	}
	validate := plan.ValidatePlan(g.Plan)
	if len(validate) > 0 {
		return struct {
			Validate []plan.Problem `json:"validate"`
		}{validate}
	}
	var ready []string
	for _, u := range plan.ReadySet(g.Plan, nil) {
		ready = append(ready, u.ID)
	}
	if ready == nil {
		ready = []string{}
	}
	return struct {
		Validate []plan.Problem `json:"validate"`
		Ready    []string       `json:"ready"`
		Blocked  []plan.Blocked `json:"blocked"`
		Waves    [][]string     `json:"waves"`
	}{validate, ready, plan.BlockedUnits(g.Plan, nil), plan.WavePlan(g.Plan, nil)}
}

func constants(c jsCase) any {
	return struct {
		StopKinds                []string        `json:"STOP_KINDS"`
		RecordKinds              []string        `json:"RECORD_KINDS"`
		Sources                  []string        `json:"SOURCES"`
		GateKinds                []string        `json:"GATE_KINDS"`
		UnitTypes                []string        `json:"UNIT_TYPES"`
		AmendmentKinds           []string        `json:"AMENDMENT_KINDS"`
		InvalidationFields       []string        `json:"INVALIDATION_FIELDS"`
		StageCommonRequired      []string        `json:"STAGE_COMMON_REQUIRED"`
		StageCommonProperties    json.RawMessage `json:"STAGE_COMMON_PROPERTIES"`
		InvalidationSchema       json.RawMessage `json:"INVALIDATION_SCHEMA"`
		PlanSchema               json.RawMessage `json:"PLAN_SCHEMA"`
		MergeResultSchema        json.RawMessage `json:"MERGE_RESULT_SCHEMA"`
		DistressSchema           json.RawMessage `json:"DISTRESS_SCHEMA"`
		StageSchema              json.RawMessage `json:"stageSchema"`
		BlockingGateKinds        []string        `json:"BLOCKING_GATE_KINDS"`
		ReplanItemKinds          []string        `json:"REPLAN_ITEM_KINDS"`
		PlanSupersedingStopKinds []string        `json:"PLAN_SUPERSEDING_STOP_KINDS"`
		DefaultConfig            plan.Config     `json:"DEFAULT_CONFIG"`
		OversizeFactor           int             `json:"OVERSIZE_FACTOR"`
	}{
		plan.StopKinds, plan.RecordKinds, plan.Sources, plan.GateKinds, plan.UnitTypes, plan.AmendmentKinds,
		plan.InvalidationFields, plan.StageCommonRequired, plan.StageCommonProperties, plan.InvalidationSchema,
		plan.PlanSchema, plan.MergeResultSchema, plan.DistressSchema, plan.StageSchema(json.RawMessage(c.Text)),
		plan.BlockingGateKinds, plan.ReplanItemKinds, plan.PlanSupersedingStopKinds, plan.DefaultConfig, plan.OversizeFactor,
	}
}
