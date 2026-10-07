package plan

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func gatesDoc(body string) string {
	return "# Plan\n\n```yaml lsf-plan\nunits: []\nexecution_order: []\n```\n\n```yaml lsf-gates\n" + body + "```\n"
}

func TestParseDeclarationAbsent(t *testing.T) {
	d, problems := ParseDeclaration("# Plan\n\n```yaml lsf-plan\nunits: []\n```\n")
	if d != nil || problems != nil {
		t.Fatalf("absent block: %v %v", d, problems)
	}
	if got := d.Identity(); got != "null" {
		t.Errorf("nil identity: %s", got)
	}
}

func TestParseDeclarationRoundTrip(t *testing.T) {
	d, problems := ParseDeclaration(gatesDoc(`setup: "npm ci"   # once per checkout
gates:
  - name: tests
    command: "npm test -- --reporter=dot: quiet"
    timeout: 45m
  - name: lint
    command: npm run lint
`))
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	want := `{"setup":"npm ci","gates":[{"name":"tests","command":"npm test -- --reporter=dot: quiet","timeout":"45m"},{"name":"lint","command":"npm run lint"}]}`
	if got := d.Identity(); got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	d2, _ := ParseDeclaration(gatesDoc("gates:\n  - name: lint\n    command: npm run lint\n"))
	if d2.Identity() == d.Identity() || d2.Setup != "" {
		t.Errorf("different declarations share an identity: %s", d2.Identity())
	}
}

func TestParseDeclarationProblems(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{"not a mapping", "- a\n", "top level must be a mapping"},
		{"yaml error", "gates: [\n", "unterminated flow sequence"},
		{"unknown top key", "gates:\n  - name: a\n    command: x\nextra: 1\n", "unknown top-level key extra"},
		{"no gates", "setup: x\n", "`gates` must be a non-empty sequence"},
		{"empty gates", "gates: []\n", "`gates` must be a non-empty sequence"},
		{"gates not a sequence", "gates: x\n", "`gates` must be a non-empty sequence"},
		{"empty setup", "setup: \"  \"\ngates:\n  - name: a\n    command: x\n", "setup must be a non-empty command"},
		{"gate not a mapping", "gates:\n  - x\n", "gates[0]: gate must be a mapping"},
		{"unknown gate key", "gates:\n  - name: a\n    command: x\n    pin: 1\n", "a: unknown key pin"},
		{"missing name", "gates:\n  - command: x\n", "gates[0]: missing name"},
		{"duplicate name", "gates:\n  - name: a\n    command: x\n  - name: a\n    command: y\n", "duplicate gate name a"},
		{"missing command", "gates:\n  - name: a\n", "a: command must be a non-empty string"},
		{"empty command", "gates:\n  - name: a\n    command: \" \"\n", "a: command must be a non-empty string"},
		{"bad timeout", "gates:\n  - name: a\n    command: x\n    timeout: soon\n", `a: timeout must be a positive duration such as 45m; got "soon"`},
		{"zero timeout", "gates:\n  - name: a\n    command: x\n    timeout: 0s\n", "a: timeout must be a positive duration"},
		{"negative timeout", "gates:\n  - name: a\n    command: x\n    timeout: -5m\n", "a: timeout must be a positive duration"},
		{"null timeout", "gates:\n  - name: a\n    command: x\n    timeout:\n", "a: timeout must be a positive duration"},
		{"null setup", "setup: ~\ngates:\n  - name: a\n    command: x\n", "setup must be a non-empty command"},
		{"setup not a string", "setup: true\ngates:\n  - name: a\n    command: x\n", "setup must be a non-empty command"},
		{"name not a string", "gates:\n  - name: true\n    command: x\n", "gates[0]: name must be a non-empty string"},
		{"name with slash", "gates:\n  - name: a/b\n    command: x\n", `gates[0]: name must be a non-empty string without surrounding space, control characters or /; got "a/b"`},
		{"name with space", "gates:\n  - name: \" a\"\n    command: x\n", "gates[0]: name must be"},
		{"command not a string", "gates:\n  - name: a\n    command: true\n", "a: command must be a non-empty string"},
		{"duplicate command", "gates:\n  - name: a\n    command: x\n  - name: b\n    command: \" x\"\n", "b: command repeats another gate's"},
		{"empty block", "# nothing\n", "top level must be a mapping"},
		{"timeout not a string", "gates:\n  - name: a\n    command: x\n    timeout: [1]\n", "a: timeout must be a positive duration"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d, problems := ParseDeclaration(gatesDoc(c.body))
			if d != nil {
				t.Errorf("a malformed block yields no declaration: %+v", d)
			}
			found := false
			for _, p := range problems {
				if p.Kind != "invalid-plan" || p.Referent == "" {
					t.Errorf("problem shape: %+v", p)
				}
				found = found || strings.Contains(p.Statement, c.want)
			}
			if !found {
				t.Errorf("want %q in %v", c.want, statements(problems))
			}
		})
	}
}

func TestParseDeclarationProblemsAccumulate(t *testing.T) {
	_, problems := ParseDeclaration(gatesDoc("extra: 1\ngates:\n  - name: a\n  - command: x\n"))
	want := []string{"gates block: unknown top-level key extra", "gates block: a: command must be a non-empty string", "gates block: gates[1]: missing name"}
	if got := statements(problems); !slices.Equal(got, want) {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
}

// Gate order is run order, so it is part of the identity.
func TestDeclarationIdentityOrder(t *testing.T) {
	ab, _ := ParseDeclaration(gatesDoc("gates:\n  - name: a\n    command: x\n  - name: b\n    command: y\n"))
	ba, _ := ParseDeclaration(strings.ReplaceAll(gatesDoc("gates:\n  - name: b\n    command: y\n  - name: a\n    command: x\n"), "\n", "\r\n"))
	if ab == nil || ba == nil || ab.Identity() == ba.Identity() {
		t.Fatalf("order: %v %v", ab, ba)
	}
}

func TestParseDeclarationTwoBlocks(t *testing.T) {
	block := "```yaml lsf-gates\ngates:\n  - name: a\n    command: x\n```\n"
	_, problems := ParseDeclaration(block + "\n" + block)
	if len(problems) != 1 || !strings.Contains(problems[0].Statement, "2 ```yaml lsf-gates blocks") {
		t.Fatalf("two blocks: %v", problems)
	}
}

// The plan graph is parsed exactly as before beside a gate block.
func TestLoadGraphIgnoresGatesBlock(t *testing.T) {
	g := LoadGraph(gatesDoc("gates:\n  - name: a\n    command: x\n"), nil)
	if len(g.Problems) != 0 || g.Plan == nil {
		t.Fatalf("graph: %+v", g)
	}
}

// docs/plan-graph.md's example is a valid declaration.
func TestDocumentedGatesExample(t *testing.T) {
	b, err := os.ReadFile("../../docs/plan-graph.md")
	if err != nil {
		t.Skip("docs not in this tree:", err)
	}
	d, problems := ParseDeclaration(string(b))
	if d == nil || len(problems) > 0 {
		t.Fatalf("documented example: %v", problems)
	}
	want := `{"setup":"npm ci","gates":[{"name":"tests","command":"npm test","timeout":"45m"},{"name":"lint","command":"npm run lint"}]}`
	if got := d.Identity(); got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
}
