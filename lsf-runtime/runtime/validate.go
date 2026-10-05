package lsfrt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
)

// validateOutput checks raw JSON against the JSON Schema subset the stage
// schemas use: type, properties, required, items, enum (the spike's
// validate.js). It returns one message per violation; none means valid.
// Properties are visited in sorted key order, not the schema's insertion order
// as in the spike, so messages are deterministic; the set of messages matches.
func validateOutput(schema, raw json.RawMessage) []string {
	var s map[string]any
	if err := json.Unmarshal(schema, &s); err != nil {
		return []string{fmt.Sprintf("schema: %v", err)}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return []string{fmt.Sprintf("$: not JSON: %v", err)}
	}
	return validateValue(s, v, "$")
}

func validateValue(schema map[string]any, v any, path string) []string {
	t, isString := schema["type"].(string)
	if raw, present := schema["type"]; present && !isString {
		return []string{fmt.Sprintf("%s: unsupported schema type %s", path, abbrev(raw))}
	}
	if t != "" {
		ok, known := isType(t, v)
		if !known {
			return []string{fmt.Sprintf("%s: unsupported schema type %q", path, t)}
		}
		if !ok {
			return []string{fmt.Sprintf("%s: expected %s, got %s", path, t, abbrev(v))}
		}
	}
	var errs []string
	if enum, ok := schema["enum"].([]any); ok && !slices.ContainsFunc(enum, func(e any) bool { return jsonEqual(e, v) }) {
		errs = append(errs, fmt.Sprintf("%s: %s not in enum", path, abbrev(v)))
	}
	if t == "object" {
		obj := v.(map[string]any)
		req, _ := schema["required"].([]any)
		for _, k := range req {
			if ks, ok := k.(string); ok {
				if _, present := obj[ks]; !present {
					errs = append(errs, fmt.Sprintf("%s.%s: required", path, ks))
				}
			}
		}
		props, _ := schema["properties"].(map[string]any)
		for _, k := range sortedKeys(props) {
			sub, ok := props[k].(map[string]any)
			if val, present := obj[k]; ok && present {
				errs = append(errs, validateValue(sub, val, path+"."+k)...)
			}
		}
	}
	if t == "array" {
		if items, ok := schema["items"].(map[string]any); ok {
			for i, e := range v.([]any) {
				errs = append(errs, validateValue(items, e, fmt.Sprintf("%s[%d]", path, i))...)
			}
		}
	}
	return errs
}

// isType reports whether v (decoded with UseNumber) has JSON type t, and
// whether t is a type this subset knows.
func isType(t string, v any) (ok, known bool) {
	switch t {
	case "object":
		_, ok = v.(map[string]any)
	case "array":
		_, ok = v.([]any)
	case "string":
		_, ok = v.(string)
	case "boolean":
		_, ok = v.(bool)
	case "number":
		_, ok = v.(json.Number)
	case "integer":
		// JavaScript's Number.isInteger: 3.0 is an integer.
		if n, isNum := v.(json.Number); isNum {
			f, err := n.Float64()
			ok = err == nil && f == math.Trunc(f) && !math.IsInf(f, 0)
		}
	default:
		return false, false
	}
	return ok, true
}

// jsonEqual is enum membership as JavaScript's includes sees it: numbers
// compare by value (1 equals 1.0), anything else by its JSON encoding.
func jsonEqual(a, b any) bool {
	if fa, ok := asFloat(a); ok {
		fb, ok := asFloat(b)
		return ok && fa == fb
	}
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(ja, jb)
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// abbrev renders v as JSON cut to 40 runes; it is fed back to the model.
func abbrev(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return truncateRunes(strings.TrimSuffix(buf.String(), "\n"), 40)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}
