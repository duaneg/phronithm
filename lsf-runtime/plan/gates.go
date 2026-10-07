package plan

import (
	"fmt"
	"strings"
	"time"
)

// The project's declared gate commands (docs/plan-graph.md, Gate block): a
// separate fenced block in the plan document, which the runtime runs on a
// clean checkout itself (orchestrator D27). The Workflow script ignores it.

const GatesTag = "lsf-gates"

// DeclaredGate is one gate command. Timeout is a Go duration, "" for the
// runtime's default.
type DeclaredGate struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Timeout string `json:"timeout,omitempty"`
}

// Declaration is the gate block. Its identity is its MarshalJS JSON.
type Declaration struct {
	Setup string         `json:"setup,omitempty"`
	Gates []DeclaredGate `json:"gates"`
}

var gatesBlockRE = blockRE(GatesTag)

func gatesProblem(statement, referent string) Problem {
	return problem("gates block: "+statement, referent)
}

// ParseDeclaration parses the lsf-gates block of a plan document. It returns
// nil, nil when the document has none; any problem is an invalid-plan
// problem and the declaration is then nil, so a caller checks problems
// before comparing identities. A present key with a null value is a
// problem, not an absent key. Values are compared as written: `1h` and
// `60m` are different declarations.
func ParseDeclaration(markdown string) (*Declaration, []Problem) {
	matches := gatesBlockRE.FindAllStringSubmatch(markdown, -1)
	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
	default:
		return nil, []Problem{gatesProblem(fmt.Sprintf("%d ```yaml %s blocks found; at most one is allowed", len(matches), GatesTag), GatesTag)}
	}
	doc, err := ParseYAML(matches[0][1])
	if err != nil {
		return nil, []Problem{gatesProblem(err.Error(), GatesTag)}
	}
	top, ok := doc.(*Map)
	if !ok {
		return nil, []Problem{gatesProblem("top level must be a mapping", GatesTag)}
	}
	problems := unknownKeys(top, []string{"setup", "gates"}, func(k string) Problem {
		return gatesProblem("unknown top-level key "+k, k)
	})
	d := &Declaration{Gates: []DeclaredGate{}}
	if v, present := top.Get("setup"); present {
		s, ok := v.(string)
		if !ok || strings.TrimSpace(s) == "" {
			problems = append(problems, gatesProblem("setup must be a non-empty command", "setup"))
		}
		d.Setup = s
	}
	rawGates, _ := top.Get("gates")
	gates, ok := rawGates.([]any)
	if !ok || len(gates) == 0 {
		return nil, append(problems, gatesProblem("`gates` must be a non-empty sequence", "gates"))
	}
	seen, commands := map[string]bool{}, map[string]bool{}
	for idx, g := range gates {
		name, _ := get(g, "name")
		ref, hasName := nonEmptyString(name)
		if !hasName || !validGateName(ref) {
			ref = fmt.Sprintf("gates[%d]", idx)
		}
		gm, isMap := g.(*Map)
		if !isMap {
			problems = append(problems, gatesProblem(ref+": gate must be a mapping", ref))
			continue
		}
		problems = append(problems, unknownKeys(gm, []string{"name", "command", "timeout"}, func(k string) Problem {
			return gatesProblem(ref+": unknown key "+k, ref)
		})...)
		switch {
		case name == nil:
			problems = append(problems, gatesProblem(ref+": missing name", ref))
		case !hasName || ref != name:
			problems = append(problems, gatesProblem(fmt.Sprintf("%s: name must be a non-empty string without surrounding space, control characters or /; got %s", ref, jsJSON(name)), ref))
		case seen[ref]:
			problems = append(problems, gatesProblem("duplicate gate name "+ref, ref))
		default:
			seen[ref] = true
		}
		gate := DeclaredGate{Name: ref}
		cmd, _ := gm.Get("command")
		command, isStr := cmd.(string)
		if !isStr || strings.TrimSpace(command) == "" {
			problems = append(problems, gatesProblem(ref+": command must be a non-empty string", ref))
		}
		gate.Command = command
		if isStr && commands[strings.TrimSpace(command)] {
			problems = append(problems, gatesProblem(ref+": command repeats another gate's", ref))
		}
		commands[strings.TrimSpace(command)] = true
		if v, present := gm.Get("timeout"); present {
			s, _ := v.(string)
			if dur, err := time.ParseDuration(s); err != nil || dur <= 0 {
				problems = append(problems, gatesProblem(fmt.Sprintf("%s: timeout must be a positive duration such as 45m; got %s", ref, jsJSON(v)), ref))
			}
			gate.Timeout = s
		}
		d.Gates = append(d.Gates, gate)
	}
	if len(problems) > 0 {
		return nil, problems
	}
	return d, nil
}

// validGateName keeps a name usable in labels and log names.
func validGateName(n string) bool {
	return n == strings.TrimSpace(n) && !strings.ContainsFunc(n, func(r rune) bool { return r < 0x20 || r == 0x7f || r == '/' })
}

// Identity is the declaration's JSON, which two loads compare.
func (d *Declaration) Identity() string {
	if d == nil {
		return "null"
	}
	b, err := MarshalJS(d, "")
	if err != nil {
		panic(err) // strings and slices only
	}
	return string(b)
}
