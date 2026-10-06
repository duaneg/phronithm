package plan

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// JavaScript semantics the port depends on: what counts as whitespace, and
// how String(), JSON.stringify and object key order behave on the YAML
// value model (nil, bool, string, []any, *Map).

// isJSSpace reports whether r is in JS's \s class (WhiteSpace plus
// LineTerminator), which is also what String.prototype.trim strips. It
// differs from unicode.IsSpace: U+FEFF is in, U+0085 is out.
func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// jsSpaceClass is isJSSpace as a regexp character-class body.
const jsSpaceClass = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`

func jsTrim(s string) string      { return strings.TrimFunc(s, isJSSpace) }
func jsTrimStart(s string) string { return strings.TrimLeftFunc(s, isJSSpace) }
func jsTrimEnd(s string) string   { return strings.TrimRightFunc(s, isJSSpace) }

// jsString is JS String(v).
func jsString(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(x)
	case string:
		return x
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			if e != nil {
				parts[i] = jsString(e)
			}
		}
		return strings.Join(parts, ",")
	case *Map:
		return "[object Object]"
	}
	panic(fmt.Sprintf("jsString: unexpected %T", v))
}

// jsJSON is JS JSON.stringify(v), compact.
func jsJSON(v any) string {
	var b strings.Builder
	writeJSJSON(&b, v)
	return b.String()
}

func writeJSJSON(b *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case string:
		writeJSQuote(b, x)
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			writeJSJSON(b, e)
		}
		b.WriteByte(']')
	case *Map:
		b.WriteByte('{')
		for i, k := range x.Keys() {
			if i > 0 {
				b.WriteByte(',')
			}
			writeJSQuote(b, k)
			b.WriteByte(':')
			writeJSJSON(b, x.vals[k])
		}
		b.WriteByte('}')
	default:
		panic(fmt.Sprintf("jsJSON: unexpected %T", v))
	}
}

// writeJSQuote quotes s as JSON.stringify does: only `"`, `\` and controls
// below U+0020 are escaped.
func writeJSQuote(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}

// isArrayIndex reports whether k is a canonical array index (0 to 2^32-2),
// which JS orders before other property keys.
func isArrayIndex(k string) bool {
	if k == "" || len(k) > 10 || (len(k) > 1 && k[0] == '0') {
		return false
	}
	n, err := strconv.ParseUint(k, 10, 64)
	return err == nil && n < 1<<32-1
}

// jsKeyOrder returns keys (in insertion order) in JS property order: array
// indices ascending, then the rest in insertion order.
func jsKeyOrder(keys []string) []string {
	var idx, rest []string
	for _, k := range keys {
		if isArrayIndex(k) {
			idx = append(idx, k)
		} else {
			rest = append(rest, k)
		}
	}
	slices.SortStableFunc(idx, func(a, b string) int {
		x, _ := strconv.ParseUint(a, 10, 64)
		y, _ := strconv.ParseUint(b, 10, 64)
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
		return 0
	})
	return append(idx, rest...)
}

// lastRune returns the rune ending s, or utf8.RuneError when s is empty.
func lastRune(s string) rune {
	r, _ := utf8.DecodeLastRuneInString(s)
	return r
}
