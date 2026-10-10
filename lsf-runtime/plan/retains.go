package plan

import (
	"path"
	"strings"
)

// A unit's retains are the paths cleanup moves out of its clones into the
// project's store, <store>/<unit>/, before removing them (orchestrator D33);
// the store is the lsf-gates block's `store`. Both are Go-only (D32).

// retainsProblems checks a unit's retains entries: each relative, clean
// (no trailing /, no . or .. component), not ".", not under .git, and none
// a duplicate of or inside another.
func retainsProblems(ref string, retains []string) []Problem {
	var problems []Problem
	bad := func(p, why string) {
		problems = append(problems, problem(ref+": retains "+jsJSON(p)+": "+why, ref))
	}
	var ok []string
	for _, p := range retains {
		switch {
		case strings.HasPrefix(p, "/"):
			bad(p, "must be a path relative to the clone, not absolute")
		case p == "." || path.Clean(p) != p || strings.HasPrefix(p, "../") || p == "..":
			bad(p, "must be a clean relative path, without . or .. components or a trailing /")
		case strings.ContainsFunc(p, func(r rune) bool { return r < 0x20 || r == 0x7f }):
			bad(p, "must not contain control characters")
		case p == ".git" || strings.HasPrefix(p, ".git/"):
			bad(p, "must not be in .git")
		default:
			ok = append(ok, p)
		}
	}
	for i, p := range ok {
		for _, q := range ok[:i] {
			switch {
			case p == q:
				bad(p, "is listed twice")
			case strings.HasPrefix(p, q+"/"):
				bad(p, "is inside "+jsJSON(q))
			case strings.HasPrefix(q, p+"/"):
				bad(p, "contains "+jsJSON(q))
			}
		}
	}
	return problems
}

// SafeUnitDir reports whether a unit id can name one directory in the
// store: non-empty, at most 100 bytes, not . or .., no leading . or -, and
// no /, :, #, NUL or control character (: and # would also break a clone
// label's parse).
func SafeUnitDir(id string) bool {
	return id != "" && len(id) <= 100 && id[0] != '.' && id[0] != '-' &&
		!strings.ContainsFunc(id, func(r rune) bool { return r < 0x20 || r == 0x7f || r == '/' || r == ':' || r == '#' })
}

// CheckRetains checks the plan's retains against the declaration: a unit
// with retains needs the block's store, and an id SafeUnitDir accepts.
func CheckRetains(p *Plan, d *Declaration) []Problem {
	var problems []Problem
	for _, u := range p.Units {
		if len(u.Retains) == 0 {
			continue
		}
		if d == nil || d.Store == "" {
			problems = append(problems, problem(u.ID+": retains needs a `store` in the ```yaml "+GatesTag+" block", u.ID))
		}
		if !SafeUnitDir(u.ID) {
			problems = append(problems, problem(u.ID+": a unit with retains needs an id usable as a directory name (at most 100 bytes; no /, :, #, control characters, or leading . or -)", u.ID))
		}
	}
	return problems
}
