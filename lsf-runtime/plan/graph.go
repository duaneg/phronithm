package plan

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The plan graph and decision log as machine-readable YAML blocks
// (docs/plan-graph.md). A copy stage returns the fenced blocks verbatim;
// this file parses and validates them. Everything the load stage used to ask
// an agent to interpret is either a field here or a structural problem.

const (
	PlanTag      = "lsf-plan"
	DecisionsTag = "lsf-decisions"
)

// Problem is a structural defect, reported as a signal of its Kind.
type Problem struct {
	Kind      string `json:"kind"`
	Statement string `json:"statement"`
	Referent  string `json:"referent"`
}

func problem(statement, referent string) Problem {
	return Problem{Kind: "invalid-plan", Statement: statement, Referent: referent}
}

func blockRE(tag string) *regexp.Regexp {
	// `\r?$` stands in for JS multiline `$` matching before `\r`.
	return regexp.MustCompile("(?m)^```yaml[ \\t]+" + regexp.QuoteMeta(tag) + "[ \\t]*\\r?\\n((?s:.*?))^```[ \\t]*\\r?$")
}

var blockREs = map[string]*regexp.Regexp{PlanTag: blockRE(PlanTag), DecisionsTag: blockRE(DecisionsTag)}

// ExtractBlock returns the body of the one fenced block tagged tag in a
// Markdown document (opened by "```yaml <tag>"), or a problem.
func ExtractBlock(markdown, tag string) (body, problemText string) {
	re, ok := blockREs[tag]
	if !ok {
		re = blockRE(tag)
	}
	matches := re.FindAllStringSubmatch(markdown, -1)
	switch len(matches) {
	case 0:
		return "", fmt.Sprintf("no ```yaml %s block found", tag)
	case 1:
		return matches[0][1], ""
	}
	return "", fmt.Sprintf("%d ```yaml %s blocks found; exactly one is allowed", len(matches), tag)
}

// Plan is the decomposition plan reduced to what the orchestrator reads
// (PlanSchema). Path and Sha are set by the caller. The parser's slices are
// never nil; a caller building these types directly must do the same, or
// the JSON holds null where JS has [].
type Plan struct {
	Units          []*Unit  `json:"units"`
	ExecutionOrder []string `json:"executionOrder"`
	Path           string   `json:"path,omitempty"`
	Sha            string   `json:"sha,omitempty"`
}

type Unit struct {
	ID           string       `json:"id"`
	Type         string       `json:"type,omitempty"` // "" when missing or not a string (an invalid-plan problem)
	Complete     bool         `json:"complete"`
	Dependencies []Dependency `json:"dependencies"`
	Scope        []string     `json:"scope"`
	Publishes    []string     `json:"publishes"`
	Gates        []Gate       `json:"gates"`
}

type Dependency struct {
	Unit string `json:"unit"`
}

// Gate is an external gate. Pin is nil when the block gives none; a pin of
// "" is kept, as in JS.
type Gate struct {
	Name      string  `json:"name"`
	Kind      string  `json:"kind,omitempty"` // as Unit.Type
	Cleared   bool    `json:"cleared"`
	Pin       *string `json:"pin,omitempty"`
	ClearedBy *string `json:"clearedBy,omitempty"` // the clearing decision's id, which may be ""
}

// Decision is one decision-log entry.
type Decision struct {
	ID       string   `json:"id"`
	Date     string   `json:"date"`
	Question string   `json:"question"`
	Decision string   `json:"decision"`
	Clears   []string `json:"clears"`
	Resolves []string `json:"resolves"`
}

// Graph is the load stage's result: Plan is nil only when the plan block
// is unreadable, and Problems says why.
type Graph struct {
	Plan      *Plan      `json:"plan"`
	Decisions []Decision `json:"decisions"`
	Problems  []Problem  `json:"problems"`
}

// get returns m[k] when v is a mapping holding k.
func get(v any, k string) (any, bool) {
	if m, ok := v.(*Map); ok {
		return m.Get(k)
	}
	return nil, false
}

// orDefault is JS `v ?? d` for a looked-up value: absent and null both
// take the default.
func orDefault(v any, d any) any {
	if v == nil {
		return d
	}
	return v
}

// stringList is JS isStringList: a sequence of non-empty strings.
func stringList(v any) ([]string, bool) {
	xs, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		s, ok := x.(string)
		if !ok || s == "" {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// nonEmptyString returns v when it is a non-empty string (JS
// `typeof v === 'string' && v`).
func nonEmptyString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok && s != ""
}

// repeated returns each value occurring more than once, ordered by its
// second occurrence (JS `new Set(xs.filter((x, i) => xs.indexOf(x) !== i))`).
func repeated(xs []string) []string {
	seen := map[string]int{}
	var out []string
	for _, x := range xs {
		seen[x]++
		if seen[x] == 2 {
			out = append(out, x)
		}
	}
	return out
}

func unknownKeys(m *Map, allowed []string, statement func(k string) Problem) []Problem {
	var out []Problem
	for _, k := range m.Keys() {
		if !slices.Contains(allowed, k) {
			out = append(out, statement(k))
		}
	}
	return out
}

// ParsePlanGraph parses the plan block into the graph the scheduler reads,
// less Path and Sha, which the caller adds. The plan is nil when the block
// cannot be read at all.
func ParsePlanGraph(body string) (*Plan, []Problem) {
	problems := []Problem{}
	doc, err := ParseYAML(body)
	if err != nil {
		return nil, []Problem{problem("plan block: "+err.Error(), PlanTag)}
	}
	top, ok := doc.(*Map)
	if !ok {
		return nil, []Problem{problem("plan block: top level must be a mapping", PlanTag)}
	}
	problems = append(problems, unknownKeys(top, []string{"units", "execution_order"}, func(k string) Problem {
		return problem("plan block: unknown top-level key "+k, k)
	})...)
	rawUnits, _ := top.Get("units")
	units, ok := rawUnits.([]any)
	if !ok {
		return nil, append(problems, problem("plan block: `units` must be a sequence", "units"))
	}
	plan := &Plan{Units: []*Unit{}, ExecutionOrder: []string{}}
	for idx, u := range units {
		id, _ := get(u, "id")
		ref, ok := nonEmptyString(id)
		if !ok {
			ref = fmt.Sprintf("units[%d]", idx)
		}
		um, isMap := u.(*Map)
		if !isMap {
			problems = append(problems, problem(ref+": unit must be a mapping", ref))
			continue
		}
		if _, ok := nonEmptyString(id); !ok {
			problems = append(problems, problem(ref+": missing id", ref))
		}
		problems = append(problems, unknownKeys(um, []string{"id", "type", "complete", "dependencies", "scope", "publishes", "gates"}, func(k string) Problem {
			return problem(ref+": unknown key "+k, ref)
		})...)
		typ, _ := um.Get("type")
		typeStr, isStr := typ.(string)
		if !isStr || !slices.Contains(UnitTypes, typeStr) {
			problems = append(problems, problem(fmt.Sprintf("%s: type must be one of %s; got %s", ref, strings.Join(UnitTypes, ", "), jsJSON(typ)), ref))
		}
		complete, hasComplete := um.Get("complete")
		if _, isBool := complete.(bool); hasComplete && !isBool {
			problems = append(problems, problem(ref+": complete must be true or false", ref))
		}
		listField := func(key, statement string) []string {
			v, _ := um.Get(key)
			xs, ok := stringList(orDefault(v, []any{}))
			if !ok {
				problems = append(problems, problem(ref+": "+statement, ref))
				return []string{}
			}
			return xs
		}
		deps := listField("dependencies", "dependencies must be a sequence of unit ids")
		scope := listField("scope", "scope must be a sequence of paths")
		publishes := listField("publishes", "publishes must be a sequence of strings")
		rawGates, _ := um.Get("gates")
		gates, ok := orDefault(rawGates, []any{}).([]any)
		if !ok {
			problems = append(problems, problem(ref+": gates must be a sequence", ref))
		}
		outGates := []Gate{}
		for gi, g := range gates {
			name, _ := get(g, "name")
			gname, ok := nonEmptyString(name)
			if !ok {
				gname = fmt.Sprintf("gates[%d]", gi)
			}
			gref := ref + "/" + gname
			gm, isMap := g.(*Map)
			if !isMap {
				problems = append(problems, problem(gref+": gate must be a mapping", gref))
				continue
			}
			problems = append(problems, unknownKeys(gm, []string{"name", "kind", "pin"}, func(k string) Problem {
				return problem(gref+": unknown key "+k, gref)
			})...)
			if _, ok := nonEmptyString(name); !ok {
				problems = append(problems, problem(gref+": gate missing name", gref))
			}
			kind, _ := gm.Get("kind")
			kindStr, isStr := kind.(string)
			if !isStr || !slices.Contains(GateKinds, kindStr) {
				problems = append(problems, problem(fmt.Sprintf("%s: gate kind must be one of %s; got %s", gref, strings.Join(GateKinds, ", "), jsJSON(kind)), gref))
			}
			pin, _ := gm.Get("pin")
			pinStr, pinIsStr := pin.(string)
			if pin != nil && !pinIsStr {
				problems = append(problems, problem(gref+": pin must be a string", gref))
			}
			gate := Gate{Kind: kindStr}
			if name != nil {
				gate.Name = jsString(name)
			}
			if pinIsStr {
				gate.Pin = &pinStr
			}
			outGates = append(outGates, gate)
		}
		names := make([]string, len(outGates))
		for i, g := range outGates {
			names[i] = g.Name
		}
		for _, n := range repeated(names) {
			problems = append(problems, problem(ref+": duplicate gate name "+n, ref+"/"+n))
		}
		unit := &Unit{ID: ref, Type: typeStr, Complete: complete == true, Dependencies: []Dependency{}, Scope: scope, Publishes: publishes, Gates: outGates}
		if id != nil {
			unit.ID = jsString(id)
		}
		for _, d := range deps {
			unit.Dependencies = append(unit.Dependencies, Dependency{Unit: d})
		}
		plan.Units = append(plan.Units, unit)
	}
	rawOrder, _ := top.Get("execution_order")
	if order, ok := stringList(rawOrder); ok {
		plan.ExecutionOrder = order
	} else {
		problems = append(problems, problem("plan block: `execution_order` must be a sequence of unit ids", "execution_order"))
	}
	for _, id := range repeated(plan.ExecutionOrder) {
		problems = append(problems, problem("execution_order names "+id+" more than once", id))
	}
	return plan, problems
}

// ParseDecisions parses the decision-log block. A nil body (no log yet) is
// an empty log.
func ParseDecisions(body *string) ([]Decision, []Problem) {
	decisions, problems := []Decision{}, []Problem{}
	if body == nil {
		return decisions, problems
	}
	doc, err := ParseYAML(*body)
	if err != nil {
		return decisions, []Problem{problem("decisions block: "+err.Error(), DecisionsTag)}
	}
	if doc == nil {
		return decisions, problems
	}
	top, isMap := doc.(*Map)
	rawList, _ := get(doc, "decisions")
	list, isList := orDefault(rawList, []any{}).([]any)
	if !isMap || !isList {
		return decisions, []Problem{problem("decisions block: top level must be a mapping with a `decisions` sequence", DecisionsTag)}
	}
	problems = append(problems, unknownKeys(top, []string{"decisions"}, func(k string) Problem {
		return problem("decisions block: unknown top-level key "+k, k)
	})...)
	for idx, d := range list {
		id, _ := get(d, "id")
		ref, ok := nonEmptyString(id)
		if !ok {
			ref = fmt.Sprintf("decisions[%d]", idx)
		}
		dm, isMap := d.(*Map)
		if !isMap {
			problems = append(problems, problem(ref+": decision must be a mapping", ref))
			continue
		}
		problems = append(problems, unknownKeys(dm, []string{"id", "date", "question", "decision", "clears", "resolves"}, func(k string) Problem {
			return problem(ref+": unknown key "+k, ref)
		})...)
		field := func(k, d string) string {
			v, _ := dm.Get(k)
			if v == nil {
				return d
			}
			return jsString(v)
		}
		for _, k := range []string{"id", "date", "question", "decision"} {
			v, _ := dm.Get(k)
			if _, ok := nonEmptyString(v); !ok {
				problems = append(problems, problem(ref+": missing "+k, ref))
			}
		}
		listField := func(key, statement string) []string {
			v, _ := dm.Get(key)
			xs, ok := stringList(orDefault(v, []any{}))
			if !ok {
				problems = append(problems, problem(ref+": "+statement, ref))
				return []string{}
			}
			return xs
		}
		clears := listField("clears", "clears must be a sequence of `unit/gate`")
		resolves := listField("resolves", "resolves must be a sequence of strings")
		decisions = append(decisions, Decision{
			ID: field("id", ref), Date: field("date", ""), Question: field("question", ""), Decision: field("decision", ""),
			Clears: clears, Resolves: resolves,
		})
	}
	ids := make([]string, len(decisions))
	for i, d := range decisions {
		ids[i] = d.ID
	}
	for _, id := range repeated(ids) {
		problems = append(problems, problem("duplicate decision id "+id, id))
	}
	return decisions, problems
}

// ApplyClearances marks the gates the decision log clears. A `clears` entry
// must name an existing unit and gate; environment gates are re-probed,
// never cleared by decision, so clearing one is a problem.
func ApplyClearances(p *Plan, decisions []Decision) []Problem {
	problems := []Problem{}
	byID := unitIndex(p)
	for _, d := range decisions {
		for _, c := range d.Clears {
			unitID, gateName, _ := strings.Cut(c, "/")
			var g *Gate
			if u := byID[unitID]; u != nil {
				for i := range u.Gates {
					if u.Gates[i].Name == gateName {
						g = &u.Gates[i]
						break
					}
				}
			}
			switch {
			case g == nil:
				problems = append(problems, problem(fmt.Sprintf("decision %s clears unknown gate %s", d.ID, c), c))
			case g.Kind == "environment":
				problems = append(problems, problem(fmt.Sprintf("decision %s clears environment gate %s; environment gates are probed, not decided", d.ID, c), c))
			default:
				g.Cleared = true
				g.ClearedBy = &d.ID
			}
		}
	}
	return problems
}

// LoadGraph is the load stage's parse: extract both blocks, parse them and
// apply the clearances. decisionsMarkdown is nil when there is no decision
// log. A non-nil Plan may come with Problems, and is not yet validated: the
// load stage turns every problem into a stop signal and then runs
// ValidatePlan on the Plan, as the orchestrator does.
func LoadGraph(planMarkdown string, decisionsMarkdown *string) Graph {
	g := Graph{Decisions: []Decision{}, Problems: []Problem{}}
	body, prob := ExtractBlock(planMarkdown, PlanTag)
	if prob != "" {
		g.Problems = append(g.Problems, problem(prob, PlanTag))
		return g
	}
	plan, problems := ParsePlanGraph(body)
	g.Problems = append(g.Problems, problems...)
	if plan == nil {
		return g
	}
	g.Plan = plan
	if decisionsMarkdown != nil {
		if db, prob := ExtractBlock(*decisionsMarkdown, DecisionsTag); prob != "" {
			g.Problems = append(g.Problems, problem(prob, DecisionsTag))
		} else {
			ds, problems := ParseDecisions(&db)
			g.Problems = append(g.Problems, problems...)
			g.Decisions = ds
		}
	}
	g.Problems = append(g.Problems, ApplyClearances(plan, g.Decisions)...)
	return g
}
