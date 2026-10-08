package validate

import (
	"regexp"
	"strings"
)

// The plan document's prose, as phronithm:breakdown writes it: the Work
// Units entries and the Execution Order section. Parsing is line-based and
// lenient; anything a check needs and cannot find is a problem, never a
// panic.

// entry is one `### <id>` heading under Work Units. Fields holds each
// `- **<Name>**: <value>` field with its value's whitespace collapsed; an
// entry with no field lines is a stub.
type entry struct {
	ID         string
	Fields     map[string]string
	Duplicates []string // field names given more than once, first value kept
}

func (e *entry) stub() bool { return len(e.Fields) == 0 && len(e.Duplicates) == 0 }

// document is the parsed prose. A nil section means the heading is absent.
type document struct {
	Entries        []*entry
	WorkUnits      bool
	ExecutionOrder *string
}

var (
	fieldRE = regexp.MustCompile(`^- \*\*([^*]+)\*\*:[ \t]*(.*)$`)
	fenceRE = regexp.MustCompile("^ {0,3}(```|~~~)")
)

// line is a document line outside any fenced code block.
type line struct {
	text  string
	fence bool // inside a fence, or a fence delimiter
}

func splitLines(markdown string) []line {
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	raw := strings.Split(markdown, "\n")
	out := make([]line, len(raw))
	var open string
	for i, t := range raw {
		if m := fenceRE.FindStringSubmatch(t); m != nil {
			switch {
			case open == "":
				open = m[1]
			case open == m[1]:
				open = ""
			}
			out[i] = line{t, true}
			continue
		}
		out[i] = line{t, open != ""}
	}
	return out
}

// headingLevel returns the ATX level of a heading line, or 0.
func headingLevel(t string) int {
	n := 0
	for n < len(t) && t[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n == len(t) || (t[n] != ' ' && t[n] != '\t') {
		return 0
	}
	return n
}

// section returns the lines under the `## <title>` heading up to the next
// level-2 or level-1 heading, or nil when the heading is absent.
func section(lines []line, title string) []line {
	for i, l := range lines {
		if l.fence || headingLevel(l.text) != 2 || strings.TrimSpace(l.text[2:]) != title {
			continue
		}
		end := i + 1
		for end < len(lines) {
			if lv := headingLevel(lines[end].text); !lines[end].fence && lv > 0 && lv <= 2 {
				break
			}
			end++
		}
		return lines[i+1 : end]
	}
	return nil
}

func parseDocument(markdown string) document {
	lines := splitLines(markdown)
	var doc document
	if wu := section(lines, "Work Units"); wu != nil {
		doc.WorkUnits = true
		doc.Entries = parseEntries(wu)
	}
	if eo := section(lines, "Execution Order"); eo != nil {
		texts := make([]string, 0, len(eo))
		for _, l := range eo {
			if !l.fence {
				texts = append(texts, l.text)
			}
		}
		s := strings.Join(texts, "\n")
		doc.ExecutionOrder = &s
	}
	return doc
}

// parseEntries reads the `### ` entries of a Work Units section. A field's
// value continues over the following lines until the next field line, a
// heading, or a blank line followed by non-indented text.
func parseEntries(lines []line) []*entry {
	var entries []*entry
	var cur *entry
	var name string // the field being read, "" when none
	var value []string
	flush := func() {
		if cur != nil && name != "" {
			v := strings.Join(strings.Fields(strings.Join(value, " ")), " ")
			if _, dup := cur.Fields[name]; dup {
				cur.Duplicates = append(cur.Duplicates, name)
			} else {
				cur.Fields[name] = v
			}
		}
		name, value = "", nil
	}
	for i, l := range lines {
		if l.fence {
			continue
		}
		if lv := headingLevel(l.text); lv > 0 {
			flush()
			cur = nil
			if lv == 3 {
				cur = &entry{ID: firstToken(l.text[3:]), Fields: map[string]string{}}
				entries = append(entries, cur)
			}
			continue
		}
		if cur == nil {
			continue
		}
		if m := fieldRE.FindStringSubmatch(l.text); m != nil {
			flush()
			name, value = strings.TrimSpace(m[1]), []string{m[2]}
			continue
		}
		if name == "" {
			continue
		}
		if strings.TrimSpace(l.text) == "" {
			if !continuesAfterBlank(lines[i+1:]) {
				flush()
			}
			continue
		}
		value = append(value, l.text)
	}
	flush()
	return entries
}

func firstToken(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

// continuesAfterBlank reports whether the next non-blank, unfenced line is
// indented, so a field's value runs on past a blank line.
func continuesAfterBlank(rest []line) bool {
	for _, l := range rest {
		if l.fence || strings.TrimSpace(l.text) == "" {
			continue
		}
		return l.text[0] == ' ' || l.text[0] == '\t'
	}
	return false
}

// splitTop splits s on sep where sep is outside parentheses and backticks.
// ok is false when the parentheses do not balance.
func splitTop(s string, sep byte) (parts []string, ok bool) {
	depth, tick, start := 0, false, 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '`':
			tick = !tick
		case tick:
		case c == '(':
			depth++
		case c == ')':
			if depth == 0 {
				return nil, false
			}
			depth--
		case c == sep && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:]), depth == 0
}

// stripParens removes parenthesised text (outside backticks), keeping the
// rest. ok is false when the parentheses do not balance.
func stripParens(s string) (out string, ok bool) {
	var b strings.Builder
	depth, tick := 0, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '`':
			tick = !tick
		case tick:
		case c == '(':
			depth++
			continue
		case c == ')':
			if depth == 0 {
				return "", false
			}
			depth--
			continue
		}
		if depth == 0 {
			b.WriteByte(c)
		}
	}
	if depth != 0 {
		return "", false
	}
	return b.String(), true
}

var tickRE = regexp.MustCompile("`([^`]*)`")

// backticked returns the backticked tokens of s and what is left outside
// them.
func backticked(s string) (tokens []string, residue string) {
	for _, m := range tickRE.FindAllStringSubmatch(s, -1) {
		tokens = append(tokens, m[1])
	}
	return tokens, tickRE.ReplaceAllString(s, " ")
}
