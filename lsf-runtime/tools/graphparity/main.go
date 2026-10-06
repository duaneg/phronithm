// Command graphparity checks the Go plan model against the JavaScript it was
// ported from (workflows/lsf/{schema,graph,scheduling}.js): it runs the same
// cases through both and compares the JSON byte for byte. Run from
// lsf-runtime/: go run ./tools/graphparity. It needs node.
//
// Cases: every fixture body in plan/testdata/graph (parseYaml,
// parsePlanGraph, parseDecisions); every fixture plan document there,
// paired with its decision log when one exists, and the repository's plan
// blocks (loadGraph).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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
const { parseYaml, parsePlanGraph, parseDecisions, loadGraph } = await import(GRAPH);
const run = (c) => {
  switch (c.op) {
    case 'yaml':
      try { return { value: parseYaml(c.text) }; } catch (e) { return { error: e.message }; }
    case 'planGraph': return parsePlanGraph(c.text);
    case 'decisions': return parseDecisions(c.text);
    case 'load': return loadGraph({ planMarkdown: c.text, decisionsMarkdown: c.decisions });
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
	case "decisions":
		ds, problems := plan.ParseDecisions(&c.Text)
		return struct {
			Decisions []plan.Decision `json:"decisions"`
			Problems  []plan.Problem  `json:"problems"`
		}{ds, problems}
	case "load":
		return plan.LoadGraph(c.Text, c.Decisions)
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
		case strings.HasSuffix(name, ".yaml"):
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
		}
	}
	breakdown, err := readFile(filepath.Join(repo, "docs", "plans", "lsf-improvements-breakdown.md"))
	if err != nil {
		return nil, fmt.Errorf("required input: %w", err)
	}
	cases = append(cases, jsCase{Name: "docs/plans/lsf-improvements-breakdown.md load", Op: "load", Text: breakdown})
	example, err := readFile(filepath.Join(repo, "docs", "plan-graph.md"))
	if err != nil {
		return nil, fmt.Errorf("required input: %w", err)
	}
	cases = append(cases, jsCase{Name: "docs/plan-graph.md load", Op: "load", Text: example, Decisions: &example})
	return cases, nil
}

func runNode(repo string, cases []jsCase) ([]string, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}
	graph := url.URL{Scheme: "file", Path: filepath.Join(repo, "workflows", "lsf", "graph.js")}
	graphJSON, _ := json.Marshal(graph.String())
	in, err := json.Marshal(cases)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(node, "--input-type=module", "-e", strings.Replace(snippet, "GRAPH", string(graphJSON), 1))
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

func main() {
	repo := flag.String("repo", "..", "repository root")
	verbose := flag.Bool("v", false, "list every case")
	flag.Parse()
	root, err := filepath.Abs(*repo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "graphparity:", err)
		os.Exit(2)
	}
	cases, err := buildCases(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "graphparity:", err)
		os.Exit(2)
	}
	js, err := runNode(root, cases)
	if err != nil {
		fmt.Fprintln(os.Stderr, "graphparity:", err)
		os.Exit(2)
	}
	failed := 0
	for i, c := range cases {
		b, err := plan.MarshalJS(goRun(c), "  ")
		if err != nil {
			fmt.Printf("FAIL %s: marshal: %v\n", c.Name, err)
			failed++
			continue
		}
		if string(b) != js[i] {
			line, g, j := firstDiff(string(b), js[i])
			fmt.Printf("FAIL %s: line %d\n  go: %s\n  js: %s\n", c.Name, line, g, j)
			failed++
		} else if *verbose {
			fmt.Printf("ok   %s\n", c.Name)
		}
	}
	if failed > 0 {
		fmt.Printf("graphparity: %d of %d cases differ\n", failed, len(cases))
		os.Exit(1)
	}
	fmt.Printf("graphparity: %d cases identical\n", len(cases))
}
