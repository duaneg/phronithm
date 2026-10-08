package validate

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

// DisagreementKind is the kind of every Agreement problem. It is this
// package's own kind, not a stop kind; the orchestrator never sees it.
const DisagreementKind = "plan-disagreement"

func disagree(referent, format string, args ...any) plan.Problem {
	return plan.Problem{Kind: DisagreementKind, Statement: "entries: " + fmt.Sprintf(format, args...), Referent: referent}
}

// Agreement compares the plan document's Work Units entries and Execution
// Order section with its parsed block, as phronithm:breakdown's quality
// check asks: every unit, its type, its Depends-on units, its Scope paths,
// its gates with kind and pin, and the execution order of the incomplete
// units. A completed unit's one-line stub is checked against the block's
// `complete: true` only. Problems come in block unit order, then entries
// with no block unit, then the execution order.
func Agreement(markdown string, p *plan.Plan) []plan.Problem {
	problems := []plan.Problem{}
	if p == nil {
		return append(problems, disagree("lsf-plan", "no plan block to compare"))
	}
	doc := parseDocument(markdown)
	if !doc.WorkUnits {
		problems = append(problems, disagree("work-units", "no ## Work Units section"))
	} else {
		problems = append(problems, entryProblems(doc.Entries, p)...)
	}
	if doc.ExecutionOrder == nil {
		problems = append(problems, disagree("execution-order", "no ## Execution Order section"))
	} else {
		problems = append(problems, orderProblems(*doc.ExecutionOrder, p)...)
	}
	return problems
}

func entryProblems(entries []*entry, p *plan.Plan) []plan.Problem {
	var problems []plan.Problem
	byID := map[string][]*entry{}
	for _, e := range entries {
		byID[e.ID] = append(byID[e.ID], e)
	}
	units := map[string]bool{}
	for _, u := range p.Units {
		if units[u.ID] {
			continue // a duplicate block id is ValidatePlan's problem
		}
		units[u.ID] = true
		es := byID[u.ID]
		switch {
		case len(es) == 0:
			problems = append(problems, disagree(u.ID, "block unit %s has no ### entry", u.ID))
			continue
		case len(es) > 1:
			problems = append(problems, disagree(u.ID, "%d ### entries for %s", len(es), u.ID))
		}
		problems = append(problems, compareUnit(es[0], u)...)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if !units[e.ID] && !seen[e.ID] {
			seen[e.ID] = true
			problems = append(problems, disagree(e.ID, "entry %s has no unit in the block", e.ID))
		}
	}
	return problems
}

var requiredFields = []string{"Type", "Scope", "Depends on", "External gates"}

func compareUnit(e *entry, u *plan.Unit) []plan.Problem {
	var problems []plan.Problem
	add := func(format string, args ...any) { problems = append(problems, disagree(u.ID, format, args...)) }
	switch {
	case e.stub():
		if !u.Complete {
			add("%s has a one-line stub but is not complete: true in the block", u.ID)
		}
		return problems
	case len(e.Fields) == 0:
		add("%s has text but no `- **Name**: value` field lines", u.ID)
		return problems
	}
	for _, name := range e.Duplicates {
		add("%s gives %s more than once", u.ID, name)
	}
	for _, name := range requiredFields {
		if _, ok := e.Fields[name]; !ok {
			add("%s has no %s field", u.ID, name)
		}
	}
	if v, ok := e.Fields["Type"]; ok && v != u.Type {
		add("%s Type is %q in its entry, %q in the block", u.ID, v, u.Type)
	}
	if v, ok := e.Fields["Depends on"]; ok {
		block := make([]string, len(u.Dependencies))
		for i, d := range u.Dependencies {
			block[i] = d.Unit
		}
		if deps, err := parseDepends(v); err != "" {
			add("%s Depends on: %s", u.ID, err)
		} else {
			problems = append(problems, setProblems(u.ID, "Depends on", deps, block)...)
		}
	}
	if v, ok := e.Fields["Scope"]; ok {
		block := make([]string, len(u.Scope))
		for i, s := range u.Scope {
			block[i] = scopeKey(s)
		}
		if scope, err := parseScope(v); err != "" {
			add("%s Scope: %s", u.ID, err)
		} else {
			problems = append(problems, setProblems(u.ID, "Scope", scope, block)...)
		}
	}
	if v, ok := e.Fields["External gates"]; ok {
		problems = append(problems, gateProblems(u, v)...)
	}
	return problems
}

func isNone(v string) bool { return strings.EqualFold(strings.TrimSpace(v), "none") }

// parseDepends reads a Depends-on value: `none`, or `;`-separated groups
// of comma-separated unit ids, each group optionally followed by a
// parenthesised edge type and artefact.
func parseDepends(v string) (ids []string, err string) {
	if isNone(v) {
		return nil, ""
	}
	groups, ok := splitTop(v, ';')
	if !ok {
		return nil, "unbalanced parentheses or backticks"
	}
	for _, g := range groups {
		bare, _ := stripParens(g) // balanced, since the whole value is
		for _, id := range strings.Split(bare, ",") {
			if id = strings.Trim(strings.TrimSpace(id), "`"); id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids, ""
}

// parseScope reads a Scope value: backticked paths, outside parentheses,
// separated by commas or "and".
func parseScope(v string) (keys []string, err string) {
	if isNone(v) {
		return nil, ""
	}
	bare, ok := stripParens(v)
	if !ok {
		return nil, "unbalanced parentheses or backticks"
	}
	tokens, residue := backticked(bare)
	for _, w := range strings.Fields(strings.ReplaceAll(residue, ",", " ")) {
		if w != "and" {
			return nil, fmt.Sprintf("%q is outside backticks", w)
		}
	}
	for _, t := range tokens {
		keys = append(keys, scopeKey(t))
	}
	return keys, ""
}

// scopeKey normalises a scope entry for comparison: a trailing / on the
// path part is dropped, the ::symbol part is kept as written.
func scopeKey(s string) string {
	path, symbol, hasSymbol := strings.Cut(s, "::")
	if len(path) > 1 {
		path = strings.TrimSuffix(path, "/")
	}
	if hasSymbol {
		return path + "::" + symbol
	}
	return path
}

// setProblems reports the entry's values missing from the block and the
// block's values missing from the entry, each sorted, and repeats in the
// entry.
func setProblems(id, field string, entryValues, blockValues []string) []plan.Problem {
	var problems []plan.Problem
	inEntry, inBlock := map[string]int{}, map[string]bool{}
	for _, v := range entryValues {
		inEntry[v]++
	}
	for _, v := range blockValues {
		inBlock[v] = true
	}
	var extra, missing, repeated []string
	for v, n := range inEntry {
		if !inBlock[v] {
			extra = append(extra, v)
		}
		if n > 1 {
			repeated = append(repeated, v)
		}
	}
	for v := range inBlock {
		if inEntry[v] == 0 {
			missing = append(missing, v)
		}
	}
	slices.Sort(extra)
	slices.Sort(missing)
	slices.Sort(repeated)
	if len(missing) > 0 {
		problems = append(problems, disagree(id, "%s %s lists %s in the block but not in its entry", id, field, strings.Join(missing, ", ")))
	}
	if len(extra) > 0 {
		problems = append(problems, disagree(id, "%s %s lists %s in its entry but not in the block", id, field, strings.Join(extra, ", ")))
	}
	if len(repeated) > 0 {
		problems = append(problems, disagree(id, "%s %s repeats %s in its entry", id, field, strings.Join(repeated, ", ")))
	}
	return problems
}

// entryGate is one External-gates segment: `name` (kind[, pin text]).
type entryGate struct {
	name, kind, pin string
}

func parseGate(seg string) (entryGate, bool) {
	seg = strings.TrimSpace(seg)
	if len(seg) < 3 || seg[0] != '`' {
		return entryGate{}, false
	}
	end := strings.IndexByte(seg[1:], '`')
	if end <= 0 {
		return entryGate{}, false
	}
	g := entryGate{name: seg[1 : end+1]}
	rest := strings.TrimSpace(seg[end+2:])
	if rest == "" {
		return g, true
	}
	if rest[0] != '(' || rest[len(rest)-1] != ')' {
		return entryGate{}, false
	}
	kind, pin, _ := strings.Cut(rest[1:len(rest)-1], ",")
	g.kind, g.pin = strings.TrimSpace(kind), strings.TrimSpace(pin)
	return g, true
}

func gateProblems(u *plan.Unit, v string) []plan.Problem {
	var problems []plan.Problem
	var gates []entryGate
	var unread []string // names of unreadable segments, left out of the name comparison
	if !isNone(v) {
		segs, ok := splitTop(v, ';')
		if !ok {
			return []plan.Problem{disagree(u.ID, "%s External gates: unbalanced parentheses or backticks", u.ID)}
		}
		for _, s := range segs {
			if strings.TrimSpace(s) == "" {
				continue
			}
			g, ok := parseGate(s)
			if !ok {
				problems = append(problems, disagree(u.ID, "%s External gates: cannot read %q as `name` (kind, pin)", u.ID, strings.TrimSpace(s)))
				if tokens, _ := backticked(s); len(tokens) > 0 {
					unread = append(unread, tokens[0])
				}
				continue
			}
			gates = append(gates, g)
		}
	}
	names := make([]string, len(gates))
	for i, g := range gates {
		names[i] = g.name
	}
	var blockNames []string
	for _, g := range u.Gates {
		if !slices.Contains(unread, g.Name) {
			blockNames = append(blockNames, g.Name)
		}
	}
	problems = append(problems, setProblems(u.ID, "External gates", names, blockNames)...)
	for _, b := range u.Gates {
		i := slices.Index(names, b.Name)
		if i < 0 {
			continue
		}
		e, ref := gates[i], u.ID+"/"+b.Name
		if e.kind != b.Kind {
			problems = append(problems, disagree(ref, "%s gate %s has kind %q in its entry, %q in the block", u.ID, b.Name, e.kind, b.Kind))
		}
		if b.Pin == nil {
			continue
		}
		text := strings.ReplaceAll(e.pin, "`", "")
		for _, tok := range pinTokens(*b.Pin) {
			if !strings.Contains(text, tok) {
				problems = append(problems, disagree(ref, "%s gate %s: the block's pin %q has %q, which its entry does not", u.ID, b.Name, *b.Pin, tok))
				break
			}
		}
	}
	return problems
}

// pinTokens returns the whitespace tokens of a pin that hold a letter or a
// digit, with surrounding list punctuation trimmed.
func pinTokens(pin string) []string {
	var out []string
	for _, f := range strings.Fields(pin) {
		f = strings.Trim(f, ",;:()\"'`")
		if strings.IndexFunc(f, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0 {
			out = append(out, f)
		}
	}
	return out
}

var mentionRE = regexp.MustCompile("`([^`\\s]+)`|\\*\\*([^*\\s]+)\\*\\*")

// orderProblems compares the first mention of each incomplete unit in the
// Execution Order section (as `id` or **id**, in written order) with the
// block's execution order restricted to incomplete units. A unit never
// mentioned is reported as such and left out of the order comparison.
func orderProblems(section string, p *plan.Plan) []plan.Problem {
	var problems []plan.Problem
	incomplete := map[string]bool{}
	for _, u := range p.Units {
		if !u.Complete {
			incomplete[u.ID] = true
		}
	}
	var written []string
	mentioned := map[string]bool{}
	for _, m := range mentionRE.FindAllStringSubmatch(section, -1) {
		id := m[1] + m[2]
		if incomplete[id] && !mentioned[id] {
			mentioned[id] = true
			written = append(written, id)
		}
	}
	var want []string
	for _, id := range p.ExecutionOrder {
		if !incomplete[id] || slices.Contains(want, id) {
			continue
		}
		if !mentioned[id] {
			problems = append(problems, disagree("execution-order", "%s is not mentioned in the Execution Order section", id))
			continue
		}
		want = append(want, id)
	}
	for i := range min(len(want), len(written)) {
		if want[i] != written[i] {
			problems = append(problems, disagree("execution-order", "Execution Order section mentions %s at position %d among incomplete units, where the block has %s", written[i], i+1, want[i]))
			break
		}
	}
	return problems
}
