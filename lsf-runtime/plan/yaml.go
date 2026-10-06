package plan

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The restricted YAML subset the plan graph and decision log are written in
// (docs/plan-graph.md): block mappings and sequences, flow sequences of
// scalars, plain and quoted scalars, comments. Values are nil, bool, string,
// []any or *Map; every unquoted scalar other than true, false, null, ~ and
// empty is a string.

// YamlError is a parse failure; Line is 1-based, 0 when unknown.
type YamlError struct {
	Line int
	Msg  string
}

func (e *YamlError) Error() string {
	if e.Line == 0 {
		return e.Msg
	}
	return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
}

func yamlErr(line int, format string, args ...any) error {
	return &YamlError{Line: line, Msg: fmt.Sprintf(format, args...)}
}

// Map is a YAML mapping. Keys returns JS property order: array-index keys
// ascending, then the rest in insertion order.
type Map struct {
	keys []string
	vals map[string]any
}

func newMap() *Map { return &Map{vals: map[string]any{}} }

func (m *Map) set(k string, v any) {
	m.keys = append(m.keys, k)
	m.vals[k] = v
}

// Get returns the value at k and whether k is present.
func (m *Map) Get(k string) (any, bool) {
	v, ok := m.vals[k]
	return v, ok
}

// Keys returns the keys in JS property order.
func (m *Map) Keys() []string { return jsKeyOrder(m.keys) }

// MarshalJSON writes the mapping in JS key order. Its escaping is that of
// the caller's encoder: use MarshalJS for JSON.stringify output.
func (m *Map) MarshalJSON() ([]byte, error) { return []byte(jsJSON(m)), nil }

type yamlLine struct {
	indent  int
	content string
	line    int
}

var (
	lineSplitRE   = regexp.MustCompile(`\r?\n`)
	keyRE         = regexp.MustCompile(`^([A-Za-z0-9_][A-Za-z0-9_.-]*):(?:[` + jsSpaceClass + `]+([^\n\r\x{2028}\x{2029}]*))?$`)
	dashRE        = regexp.MustCompile(`^-([` + jsSpaceClass + `]|$)`)
	unsupportedRE = regexp.MustCompile("^[&*!|>%@`]")
	dqEscapeRE    = regexp.MustCompile(`\\(["\\nt])`)
)

// stripComment strips a trailing comment: `#` preceded by whitespace,
// outside quotes.
func stripComment(s string) string {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#' && (i == 0 || isJSSpace(lastRune(s[:i]))):
			return s[:i]
		}
	}
	return s
}

func tokenise(text string) ([]yamlLine, error) {
	var out []yamlLine
	for idx, raw := range lineSplitRE.Split(text, -1) {
		if strings.Contains(raw, "\t") {
			return nil, yamlErr(idx+1, "tab character; indent with spaces")
		}
		content := jsTrimEnd(stripComment(raw))
		if jsTrim(content) == "" {
			continue
		}
		rest := jsTrimStart(content)
		indent := utf8.RuneCountInString(content) - utf8.RuneCountInString(rest)
		out = append(out, yamlLine{indent: indent, content: rest, line: idx + 1})
	}
	return out, nil
}

func parseScalar(s string, line int) (any, error) {
	switch s {
	case "", "~", "null":
		return nil, nil
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	switch s[0] {
	case '"':
		if len(s) < 2 || s[len(s)-1] != '"' {
			return nil, yamlErr(line, "unterminated double-quoted string")
		}
		return dqEscapeRE.ReplaceAllStringFunc(s[1:len(s)-1], func(m string) string {
			switch m[1] {
			case 'n':
				return "\n"
			case 't':
				return "\t"
			}
			return m[1:]
		}), nil
	case '\'':
		if len(s) < 2 || s[len(s)-1] != '\'' {
			return nil, yamlErr(line, "unterminated single-quoted string")
		}
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'"), nil
	}
	if unsupportedRE.MatchString(s) {
		return nil, yamlErr(line, "unsupported YAML syntax: %c", s[0])
	}
	return s, nil
}

// parseFlowSeq parses `[a, b, "c, d"]`, a flow sequence of scalars.
func parseFlowSeq(s string, line int) (any, error) {
	inner := jsTrim(s[1 : len(s)-1])
	if inner == "" {
		return []any{}, nil
	}
	var items []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		switch {
		case quote != 0:
			cur.WriteByte(c)
			if c == '\\' && quote == '"' {
				// JS appends inner[++i], `undefined` past the end; the quote
				// stays open either way and the sequence is rejected below.
				if i+1 < len(inner) {
					i++
					cur.WriteByte(inner[i])
				}
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
			cur.WriteByte(c)
		case c == ',':
			items = append(items, jsTrim(cur.String()))
			cur.Reset()
		case c == '[' || c == '{':
			return nil, yamlErr(line, "nested flow collections are not supported")
		default:
			cur.WriteByte(c)
		}
	}
	if quote != 0 {
		return nil, yamlErr(line, "unterminated quoted string in flow sequence")
	}
	items = append(items, jsTrim(cur.String()))
	out := make([]any, 0, len(items))
	for _, it := range items {
		if it == "" {
			return nil, yamlErr(line, "empty item in flow sequence")
		}
		v, err := parseScalar(it, line)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func parseValue(s string, line int) (any, error) {
	switch s[0] {
	case '[':
		if s[len(s)-1] != ']' {
			return nil, yamlErr(line, "unterminated flow sequence")
		}
		return parseFlowSeq(s, line)
	case '{':
		return nil, yamlErr(line, "flow mappings are not supported")
	}
	return parseScalar(s, line)
}

func parseBlock(lines []yamlLine, i, indent int) (any, int, error) {
	if i >= len(lines) {
		return nil, i, nil
	}
	first := lines[i]
	if first.indent != indent {
		return nil, i, yamlErr(first.line, "unexpected indent %d, expected %d", first.indent, indent)
	}
	if dashRE.MatchString(first.content) {
		return parseSeq(lines, i, indent)
	}
	return parseMap(lines, i, indent)
}

// nested is the value of a key or dash with nothing after it: a block at
// deeper indent, or null.
func nested(lines []yamlLine, i, indent int) (any, int, error) {
	if i < len(lines) && lines[i].indent > indent {
		return parseBlock(lines, i, lines[i].indent)
	}
	return nil, i, nil
}

func parseSeq(lines []yamlLine, i, indent int) (any, int, error) {
	out := []any{}
	for i < len(lines) && lines[i].indent == indent {
		content, line := lines[i].content, lines[i].line
		if !dashRE.MatchString(content) {
			return nil, i, yamlErr(line, "expected a sequence item")
		}
		rest := jsTrimStart(content[1:])
		var v any
		var err error
		switch {
		case rest == "":
			v, i, err = nested(lines, i+1, indent)
		case keyRE.MatchString(rest):
			// Inline mapping start: `- id: x`; continuation keys align with `id`.
			col := indent + utf8.RuneCountInString(content) - utf8.RuneCountInString(rest)
			lines[i] = yamlLine{indent: col, content: rest, line: line}
			v, i, err = parseMap(lines, i, col)
		default:
			v, err = parseValue(rest, line)
			i++
		}
		if err != nil {
			return nil, i, err
		}
		out = append(out, v)
	}
	if i < len(lines) && lines[i].indent > indent {
		return nil, i, yamlErr(lines[i].line, "unexpected indent")
	}
	return out, i, nil
}

func parseMap(lines []yamlLine, i, indent int) (any, int, error) {
	out := newMap()
	for i < len(lines) && lines[i].indent == indent {
		content, line := lines[i].content, lines[i].line
		m := keyRE.FindStringSubmatchIndex(content)
		if m == nil {
			return nil, i, yamlErr(line, "expected `key: value`, got `%s`", content)
		}
		key := content[m[2]:m[3]]
		raw := ""
		if m[4] >= 0 {
			raw = content[m[4]:m[5]]
		}
		if _, dup := out.vals[key]; dup {
			return nil, i, yamlErr(line, "duplicate key %s", key)
		}
		var v any
		var err error
		if raw == "" {
			v, i, err = nested(lines, i+1, indent)
		} else {
			v, err = parseValue(raw, line)
			i++
		}
		if err != nil {
			return nil, i, err
		}
		out.set(key, v)
	}
	if i < len(lines) && lines[i].indent > indent {
		return nil, i, yamlErr(lines[i].line, "unexpected indent")
	}
	return out, i, nil
}

// ParseYAML parses text in the YAML subset. An empty document is nil. The
// error is a *YamlError.
func ParseYAML(text string) (any, error) {
	lines, err := tokenise(text)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, nil
	}
	v, next, err := parseBlock(lines, 0, lines[0].indent)
	if err != nil {
		return nil, err
	}
	if next < len(lines) {
		return nil, yamlErr(lines[next].line, "content after the top-level block")
	}
	return v, nil
}
