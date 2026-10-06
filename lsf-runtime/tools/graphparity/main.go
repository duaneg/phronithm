// Command graphparity checks the Go plan model against the JavaScript it was
// ported from (workflows/lsf/{schema,graph,scheduling}.js): it runs the same
// cases through both and compares the JSON byte for byte. Run from
// lsf-runtime/: go run ./tools/graphparity. It needs node.
//
// Cases: every fixture body in plan/testdata/graph (parseYaml,
// parsePlanGraph, parseDecisions); every fixture plan document there,
// paired with its decision log when one exists, and the repository's plan
// blocks (loadGraph; then validatePlan, and on a valid plan readySet,
// blockedUnits and wavePlan); both blocks' extraction from every plan
// document; the signal mappers, stop rule, digest and ledger over
// plan/testdata/signals; and the exported constants.
//
// Two documented divergences are normalised: the Go-only kinds
// (plan.GoOnlyKinds) are left out of the constants, and a merge status
// outside the schema is checked to be Go's schema-violation (plan D20),
// then compared as the JS maps it.
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
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
const { parseYaml, extractBlock, parsePlanGraph, parseDecisions, loadGraph, PLAN_TAG, DECISIONS_TAG } = await import(@GRAPH@);
const S = await import(@SCHEDULING@);
const C = await import(@SCHEMA@);
const run = (c) => {
  switch (c.op) {
    case 'yaml':
      try { return { value: parseYaml(c.text) }; } catch (e) { return { error: e.message }; }
    case 'planGraph': return parsePlanGraph(c.text);
    case 'planGraphD4': {
      const r = parsePlanGraph(c.text);
      const drop = (o, k) => { if (typeof o[k] !== 'string' || o[k] === '') delete o[k]; };
      for (const u of r.plan?.units ?? []) { drop(u, 'type'); for (const g of u.gates) drop(g, 'kind'); }
      return r;
    }
    case 'decisions': return parseDecisions(c.text);
    case 'load': return loadGraph({ planMarkdown: c.text, decisionsMarkdown: c.decisions });
    case 'schedule': {
      const { plan } = loadGraph({ planMarkdown: c.text, decisionsMarkdown: c.decisions });
      if (!plan) return { validate: null };
      const validate = S.validatePlan(plan);
      if (validate.length) return { validate };
      const done = new Set(S.readySet(plan).map((u) => u.id));
      return {
        validate, ready: S.readySet(plan).map((u) => u.id), blocked: S.blockedUnits(plan), waves: S.wavePlan(plan),
        mergeOrder: S.mergeOrder(plan, plan.units.map((u) => u.id).reverse()),
        afterFirstWave: {
          done: [...done], ready: S.readySet(plan, done).map((u) => u.id), blocked: S.blockedUnits(plan, done),
          waves: S.wavePlan(plan, done), pairs: S.overlappingPairs(plan, done), donePreserved: done.size,
        },
      };
    }
    case 'extract': return [PLAN_TAG, DECISIONS_TAG].map((t) => extractBlock(c.text, t));
    case 'signals': {
      const f = JSON.parse(c.text);
      const sigs = [];
      for (const k of f.calls) {
        const at = k.at ?? {};
        switch (k.fn) {
          case 'null': sigs.push(S.nullReturnSignal(at)); break;
          case 'noCommit': sigs.push(S.noCommitSignal(at)); break;
          case 'readings': sigs.push(...S.signalsFromReadings(k.result, at)); break;
          case 'documentSize': sigs.push(...S.signalsFromDocumentSize(k.result, at)); break;
          case 'implement': sigs.push(...S.signalsFromImplement(k.result, at)); break;
          case 'findings': sigs.push(...S.signalsFromFindings(k.result, at)); break;
          case 'gate': sigs.push(...S.signalsFromGate(k.result, at)); break;
          case 'merge': sigs.push(...S.signalsFromMerge(k.result, at)); break;
          case 'distress': sigs.push(...S.distressSignals(k.result, at, k.config)); break;
          default: throw new Error('unknown fn ' + k.fn);
        }
      }
      const ev = S.evaluateWave(sigs);
      return {
        stop: ev.stop, stops: ev.stops.length, all: ev.all, digest: S.digest(ev.all),
        label: S.ledgerLabel(ev.all, f.wave), ledgerWave: S.ledgerWave(ev.all, f.wave), ledger: S.renderLedger(ev.all, f.wave),
      };
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
	case "planGraphD4":
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
	case "schedule":
		return schedule(c)
	case "constants":
		return constants(c)
	case "extract":
		type block struct {
			Body    *string `json:"body"`
			Problem *string `json:"problem"`
		}
		var out []block
		for _, tag := range []string{plan.PlanTag, plan.DecisionsTag} {
			body, problem := plan.ExtractBlock(c.Text, tag)
			if problem != "" {
				out = append(out, block{Problem: &problem})
			} else {
				out = append(out, block{Body: &body})
			}
		}
		return out
	case "signals":
		return signals(c)
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
			// is kept by JS and "" (omitted) in Go. The JS side drops those
			// values before comparing; everything else is compared.
			yamlFiles++
			for _, op := range []string{"yaml", "planGraphD4", "decisions"} {
				cases = append(cases, jsCase{Name: name + " " + op, Op: op, Text: text})
			}
		case strings.HasSuffix(name, ".yaml"):
			yamlFiles++
			for _, op := range []string{"yaml", "planGraph", "decisions"} {
				cases = append(cases, jsCase{Name: name + " " + op, Op: op, Text: text})
			}
		case strings.HasSuffix(name, ".decisions.md"):
			// Paired with its plan document below.
			stem := strings.TrimSuffix(name, ".decisions.md")
			if !slices.Contains(names, stem+".plan.md") && !slices.Contains(names, stem+".md") {
				return nil, fmt.Errorf("%s: decision log with no plan document", name)
			}
		case strings.HasSuffix(name, ".md"):
			c := jsCase{Name: name + " load", Op: "load", Text: text}
			stem := strings.TrimSuffix(strings.TrimSuffix(name, ".md"), ".plan")
			if log, err := readFile(filepath.Join(dir, stem+".decisions.md")); err == nil {
				c.Decisions = &log
				c.Name = name + "+" + stem + ".decisions.md load"
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			cases = append(cases, c, jsCase{Name: name + " extract", Op: "extract", Text: text})
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
	sigDir := filepath.Join(repo, "lsf-runtime", "plan", "testdata", "signals")
	sigFiles, err := filepath.Glob(filepath.Join(sigDir, "*.json"))
	if err != nil || len(sigFiles) == 0 {
		return nil, fmt.Errorf("%s: no signal fixtures (%v)", sigDir, err)
	}
	for _, f := range sigFiles {
		text, err := readFile(f)
		if err != nil {
			return nil, err
		}
		cases = append(cases, jsCase{Name: filepath.Base(f) + " signals", Op: "signals", Text: text})
	}
	// Live repository documents, compared when present; the pinned copy of
	// the breakdown plan's block in testdata outlives the document.
	for _, doc := range []struct {
		path    string
		withLog bool
	}{{"docs/plans/lsf-improvements-breakdown.md", false}, {"docs/plan-graph.md", true}} {
		text, err := readFile(filepath.Join(repo, doc.path))
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "graphparity: %s absent; not compared\n", doc.path)
			continue
		} else if err != nil {
			return nil, err
		}
		var log *string
		if doc.withLog {
			log = &text
		}
		cases = append(cases,
			jsCase{Name: doc.path + " load", Op: "load", Text: text, Decisions: log},
			jsCase{Name: doc.path + " schedule", Op: "schedule", Text: text, Decisions: log},
			jsCase{Name: doc.path + " extract", Op: "extract", Text: text})
	}
	cases = append(cases, jsCase{Name: "constants", Op: "constants", Text: `{"type":"object","properties":{"x":{"type":"string"}},"required":["x"]}`})
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

// marshalCase runs c in Go, turning a panic into an error so the other
// cases still run.
func marshalCase(c jsCase) (b []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return plan.MarshalJS(goRun(c), "  ")
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
		b, err := marshalCase(c)
		if err != nil {
			fmt.Fprintf(w, "FAIL %s: %v\n", c.Name, err)
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
	ids := func(us []*plan.Unit) []string {
		out := []string{}
		for _, u := range us {
			out = append(out, u.ID)
		}
		return out
	}
	ready := ids(plan.ReadySet(g.Plan, nil))
	done := plan.Completed{}
	for _, id := range ready {
		done[id] = true
	}
	var all []string
	for i := len(g.Plan.Units) - 1; i >= 0; i-- {
		all = append(all, g.Plan.Units[i].ID)
	}
	type after struct {
		Done          []string       `json:"done"`
		Ready         []string       `json:"ready"`
		Blocked       []plan.Blocked `json:"blocked"`
		Waves         [][]string     `json:"waves"`
		Pairs         []plan.Pair    `json:"pairs"`
		DonePreserved int            `json:"donePreserved"`
	}
	a := after{Done: ready, Ready: ids(plan.ReadySet(g.Plan, done)), Blocked: plan.BlockedUnits(g.Plan, done),
		Waves: plan.WavePlan(g.Plan, done), Pairs: plan.OverlappingPairs(g.Plan, done)}
	a.DonePreserved = len(done)
	return struct {
		Validate       []plan.Problem `json:"validate"`
		Ready          []string       `json:"ready"`
		Blocked        []plan.Blocked `json:"blocked"`
		Waves          [][]string     `json:"waves"`
		MergeOrder     []string       `json:"mergeOrder"`
		AfterFirstWave after          `json:"afterFirstWave"`
	}{validate, ready, plan.BlockedUnits(g.Plan, nil), plan.WavePlan(g.Plan, nil), plan.MergeOrder(g.Plan, all), a}
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
		goOnly(plan.StopKinds), goOnly(plan.RecordKinds), plan.Sources, plan.GateKinds, plan.UnitTypes, plan.AmendmentKinds,
		plan.InvalidationFields, plan.StageCommonRequired, plan.StageCommonProperties, plan.InvalidationSchema,
		plan.PlanSchema, plan.MergeResultSchema, plan.DistressSchema, plan.StageSchema(json.RawMessage(c.Text)),
		plan.BlockingGateKinds, plan.ReplanItemKinds, plan.PlanSupersedingStopKinds, plan.DefaultConfig, plan.OversizeFactor,
	}
}

// mergeStatuses are MergeResultSchema's status enum.
var mergeStatuses = []string{"clean", "resolved", "unresolvable", "regressed"}

// d20Merge checks plan D20 on r, then returns r as the JS maps it: a
// status outside the schema is a schema-violation stop in Go and a
// claimed-disjoint-conflict record in the JS, whose row for it is the
// row for "resolved" (the statement does not name the status).
func d20Merge(r plan.MergeResult, at plan.At) plan.MergeResult {
	sigs := plan.SignalsFromMerge(r, at)
	out := r
	out.Merges = slices.Clone(r.Merges)
	for i, m := range out.Merges {
		if slices.Contains(mergeStatuses, m.Status) {
			continue
		}
		if !slices.ContainsFunc(sigs, func(s *plan.Signal) bool { return s.Kind == "schema-violation" && s.Unit == m.Unit }) {
			panic("D20: merge of " + m.Unit + " with status " + m.Status + " is not a schema-violation")
		}
		out.Merges[i].Status = "resolved"
	}
	return out
}

// goOnly is kinds without the Go-only ones (plan.GoOnlyKinds: orchestrator
// D19, D21, D22), which the JS lists do not have.
func goOnly(kinds []string) []string {
	return slices.DeleteFunc(slices.Clone(kinds), func(k string) bool { return slices.Contains(plan.GoOnlyKinds, k) })
}

// signals runs a signal fixture: mapper calls, then the stop rule, the
// digest and the ledger.
func signals(c jsCase) any {
	var f struct {
		Wave  int `json:"wave"`
		Calls []struct {
			Fn     string          `json:"fn"`
			Result json.RawMessage `json:"result"`
			At     plan.At         `json:"at"`
			Config json.RawMessage `json:"config"`
		} `json:"calls"`
	}
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	must(json.Unmarshal([]byte(c.Text), &f))
	sigs := []*plan.Signal{}
	for _, k := range f.Calls {
		switch k.Fn {
		case "null":
			sigs = append(sigs, plan.NullReturnSignal(k.At))
		case "noCommit":
			sigs = append(sigs, plan.NoCommitSignal(k.At))
		case "readings":
			var rs []json.RawMessage
			must(json.Unmarshal(k.Result, &rs))
			sigs = append(sigs, plan.SignalsFromReadings(len(rs), k.At)...)
		case "documentSize":
			bytes := math.NaN()
			if string(k.Result) != "null" {
				must(json.Unmarshal(k.Result, &bytes))
			}
			sigs = append(sigs, plan.SignalsFromDocumentSize(bytes, k.At)...)
		case "implement":
			var r plan.ImplementResult
			must(json.Unmarshal(k.Result, &r))
			sigs = append(sigs, plan.SignalsFromImplement(r, k.At)...)
		case "findings":
			var r []plan.Finding
			must(json.Unmarshal(k.Result, &r))
			sigs = append(sigs, plan.SignalsFromFindings(r, k.At)...)
		case "gate":
			var r plan.GateResult
			must(json.Unmarshal(k.Result, &r))
			sigs = append(sigs, plan.SignalsFromGate(r, k.At)...)
		case "merge":
			var r plan.MergeResult
			must(json.Unmarshal(k.Result, &r))
			sigs = append(sigs, plan.SignalsFromMerge(d20Merge(r, k.At), k.At)...)
		case "distress":
			var r plan.Distress
			must(json.Unmarshal(k.Result, &r))
			cfg, err := plan.ConfigFrom(k.Config)
			must(err)
			sigs = append(sigs, plan.DistressSignals(r, k.At, cfg)...)
		default:
			panic("unknown fn " + k.Fn)
		}
	}
	ev := plan.EvaluateWave(sigs)
	return struct {
		Stop       bool              `json:"stop"`
		Stops      int               `json:"stops"`
		All        []*plan.Signal    `json:"all"`
		Digest     plan.DigestResult `json:"digest"`
		Label      string            `json:"label"`
		LedgerWave int               `json:"ledgerWave"`
		Ledger     string            `json:"ledger"`
	}{ev.Stop, len(ev.Stops), ev.All, plan.Digest(ev.All), plan.LedgerLabel(ev.All, f.Wave), plan.LedgerWave(ev.All, f.Wave), plan.RenderLedger(ev.All, f.Wave)}
}
