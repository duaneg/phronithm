package plan

import (
	"fmt"
	"net"
	"strings"
	"time"
	"unicode"
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

// Declaration is the gate block. Its identity is its MarshalJS JSON. Hosts
// are the network allowance: the hosts the project's builds and tests fetch
// from. A block holding hosts alone declares no gates.
type Declaration struct {
	Setup string         `json:"setup,omitempty"`
	Gates []DeclaredGate `json:"gates,omitempty"`
	Hosts []string       `json:"hosts,omitempty"`
}

// Gated reports whether the runtime runs gate commands for the plan: a
// block holding hosts alone is handled as no block.
func (d *Declaration) Gated() bool {
	return d != nil && len(d.Gates) > 0
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
	problems := unknownKeys(top, []string{"setup", "gates", "hosts"}, func(k string) Problem {
		return gatesProblem("unknown top-level key "+k, k)
	})
	d := &Declaration{}
	rawSetup, hasSetup := top.Get("setup")
	if hasSetup {
		s, ok := rawSetup.(string)
		if !ok || strings.TrimSpace(s) == "" {
			problems = append(problems, gatesProblem("setup must be a non-empty command", "setup"))
		}
		d.Setup = s
	}
	rawHosts, hasHosts := top.Get("hosts")
	if hasHosts {
		var ps []Problem
		d.Hosts, ps = parseHosts(rawHosts)
		problems = append(problems, ps...)
	}
	rawGates, hasGates := top.Get("gates")
	if !hasGates {
		switch {
		case hasSetup:
			problems = append(problems, gatesProblem("setup needs a non-empty `gates` sequence", "setup"))
		case !hasHosts:
			problems = append(problems, gatesProblem("the block must hold `gates`, `hosts` or both", GatesTag))
		}
		if len(problems) > 0 {
			return nil, problems
		}
		return d, nil
	}
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

// parseHosts reads the hosts sequence. Entries compare lower-cased for
// duplicates and are kept as written.
func parseHosts(v any) ([]string, []Problem) {
	seq, ok := v.([]any)
	if !ok || len(seq) == 0 {
		return nil, []Problem{gatesProblem("`hosts` must be a non-empty sequence of host names", "hosts")}
	}
	var problems []Problem
	hosts := make([]string, 0, len(seq))
	seen := map[string]bool{}
	for idx, raw := range seq {
		h, isStr := raw.(string)
		if !isStr {
			problems = append(problems, gatesProblem(fmt.Sprintf("hosts[%d]: must be a host name; got %s", idx, jsJSON(raw)), "hosts"))
			continue
		}
		if why := hostProblem(h); why != "" {
			problems = append(problems, gatesProblem(fmt.Sprintf("hosts[%d]: %s; got %s", idx, why, jsJSON(h)), "hosts"))
			continue
		}
		key := strings.ToLower(strings.TrimSuffix(h, "."))
		if seen[key] {
			problems = append(problems, gatesProblem("duplicate host "+h, "hosts"))
		}
		seen[key] = true
		hosts = append(hosts, h)
	}
	return hosts, problems
}

// hostProblem returns why h is not an acceptable host entry, "" when it is.
// An entry is a name of letters, digits, hyphens and underscores (IDNs in
// their xn-- form), a `*.` wildcard over a name of at least two labels, or
// an IP literal (an IPv6 one optionally bracketed). Loopback, unspecified
// and IPv4-compatible IPv6 addresses are refused, and so is a name whose
// last label is numeric (127.1, 2130706433, 0x7f.1), which a resolver may
// read as an address. A name that resolves to loopback is not caught.
func hostProblem(h string) string {
	switch {
	case h == "":
		return "must be a non-empty host name"
	case strings.ContainsFunc(h, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }):
		return "must not contain whitespace"
	case strings.ContainsAny(h, "/@"):
		return "must be a host name, without scheme, path or user"
	}
	bare := h
	if strings.HasPrefix(h, "[") && strings.HasSuffix(h, "]") {
		bare = h[1 : len(h)-1]
	}
	if ip := net.ParseIP(bare); ip != nil {
		if bare != h && ip.To4() != nil {
			return "only an IPv6 address may be bracketed"
		}
		if ip.IsLoopback() || ip.IsUnspecified() {
			return "must not be a loopback or unspecified address"
		}
		if ip.To4() == nil && net.IP(make([]byte, 12)).Equal(ip[:12]) {
			return "must not be an IPv4-compatible IPv6 address"
		}
		return ""
	}
	if strings.ContainsAny(h, ":[]") {
		return "must be a host name, without port"
	}
	name := strings.ToLower(strings.TrimSuffix(h, "."))
	if name == "localhost" || strings.HasSuffix(name, ".localhost") {
		return "must not be a loopback name"
	}
	labels := strings.Split(strings.TrimPrefix(name, "*."), ".")
	if strings.HasPrefix(name, "*.") && len(labels) < 2 {
		return "a wildcard must cover a name of at least two labels"
	}
	if len(name) > 253 {
		return "must be at most 253 characters"
	}
	for _, l := range labels {
		if l == "" || len(l) > 63 || strings.ContainsFunc(l, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_')
		}) {
			return "must be a host name of letters, digits, - and _ labels (at most 63 each), optionally with a leading *. wildcard"
		}
	}
	last := labels[len(labels)-1]
	if strings.Trim(last, "0123456789") == "" || strings.HasPrefix(last, "0x") && strings.Trim(last[2:], "0123456789abcdef") == "" {
		return "must not be a numeric address form other than a dotted quad"
	}
	return ""
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
