package orchestrator

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

// The merge's plan bookkeeping (D24): for each merged unit, its `### <id>`
// entry under `## Work Units` moves verbatim to the archive, a one-line
// stub takes its place, a two-line entry is appended to `## Plan
// Maintenance`, and the unit's `complete:` in the lsf-plan block becomes
// true. The forms are the JS merge prompt's.

// bookIn is what the rendering needs besides the texts.
type bookIn struct {
	Name, ArchivePath, LedgerPath string
	Wave                          int
}

// bookUnit is a merged unit: its merge commit and completion report.
type bookUnit struct {
	Unit, Sha, Report string
}

func stubLine(in bookIn, u bookUnit) string {
	return "### " + u.Unit + " — complete at " + u.Sha + "; entry archived to " + in.ArchivePath + "; report " + u.Report
}

func maintenanceEntry(in bookIn, u bookUnit, date string) string {
	return "### " + date + " — " + u.Unit + " complete\nReport: " + u.Report + " @ " + u.Sha + ". Ledger: " + in.LedgerPath + " wave " + itoa(in.Wave) + ".\n"
}

// mdLine is one line of a Markdown document, with its newline.
type mdLine struct {
	text    string
	level   int // heading level outside a fence; 0 otherwise
	heading string
}

var headingRE = regexp.MustCompile(`^(#{1,6})[ \t]+(.*?)[ \t]*\r?$`)

// mdLines splits text into lines, marking headings outside fenced blocks.
// A fence opens on a line starting with three or more backticks or tildes
// and closes on a line of at least as many of the same character.
func mdLines(text string) []mdLine {
	out := []mdLine{}
	fence := ""
	for _, l := range strings.SplitAfter(text, "\n") {
		if l == "" {
			continue
		}
		body := strings.TrimRight(l, "\r\n")
		if f := fenceOf(body); f != "" {
			switch {
			case fence == "":
				fence = f
			case strings.HasPrefix(f, fence) && strings.TrimSpace(strings.TrimLeft(body, f[:1])) == "":
				fence = ""
			}
			out = append(out, mdLine{text: l})
			continue
		}
		m := mdLine{text: l}
		if fence == "" {
			if h := headingRE.FindStringSubmatch(body); h != nil {
				m.level, m.heading = len(h[1]), h[2]
			}
		}
		out = append(out, m)
	}
	return out
}

// fenceOf is the run of backticks or tildes a fence line opens with, ""
// when the line is not a fence.
func fenceOf(line string) string {
	s := strings.TrimLeft(line, " ")
	if len(line)-len(s) > 3 || len(s) < 3 || (s[0] != '`' && s[0] != '~') {
		return ""
	}
	n := 0
	for n < len(s) && s[n] == s[0] {
		n++
	}
	if n < 3 {
		return ""
	}
	return s[:n]
}

func joinLines(ls []mdLine) string {
	var b strings.Builder
	for _, l := range ls {
		b.WriteString(l.text)
	}
	return b.String()
}

// section is the line range [start, end) of the level-2 section headed
// title: from its heading to the next heading of level 1 or 2.
func section(ls []mdLine, title string) (start, end int, ok bool) {
	for i, l := range ls {
		if l.level == 2 && l.heading == title {
			end = len(ls)
			for j := i + 1; j < len(ls); j++ {
				if ls[j].level == 1 || ls[j].level == 2 {
					end = j
					break
				}
			}
			return i, end, true
		}
	}
	return 0, 0, false
}

// entryRange is unit id's `### <id>` entry within [from, to): its heading
// to the next heading of level 3 or less.
func entryRange(ls []mdLine, from, to int, id string) (start, end int, ok bool) {
	for i := from; i < to; i++ {
		if ls[i].level == 3 && ls[i].heading == id {
			end = to
			for j := i + 1; j < to; j++ {
				if ls[j].level >= 1 && ls[j].level <= 3 {
					end = j
					break
				}
			}
			return i, end, true
		}
	}
	return 0, 0, false
}

// isBlank reports whether a line is whitespace.
func isBlank(l mdLine) bool { return strings.TrimSpace(l.text) == "" }

// withNewline is s ending in a newline.
func withNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

// appendToSection inserts text at the end of the section [start, end) of
// ls, after its last non-blank line and one blank line, keeping the blank
// lines that separated it from what follows.
func appendToSection(ls []mdLine, start, end int, text string) []mdLine {
	last := end
	for last > start+1 && isBlank(ls[last-1]) {
		last--
	}
	tail := ls[last:end]
	ins := []mdLine{}
	if last > 0 {
		ls[last-1].text = withNewline(ls[last-1].text)
	}
	ins = append(ins, mdLine{text: "\n"}, mdLine{text: withNewline(text)})
	if end < len(ls) && len(tail) == 0 {
		tail = []mdLine{{text: "\n"}}
	}
	return slices.Concat(ls[:last], ins, tail, ls[end:])
}

var planBlockRE = regexp.MustCompile("(?m)^```yaml[ \\t]+" + regexp.QuoteMeta(plan.PlanTag) + "[ \\t]*\\r?\\n((?s:.*?))^```[ \\t]*\\r?$")

// yamlScalar is a block-map value as written: comment dropped, quotes
// removed.
func yamlScalar(v string) string {
	if i := strings.Index(v, " #"); i >= 0 {
		v = v[:i]
	}
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		v = v[1 : len(v)-1]
	}
	return v
}

var (
	itemRE     = regexp.MustCompile(`^( *)- (.*)$`)
	keyRE      = regexp.MustCompile(`^( *)([A-Za-z_][A-Za-z0-9_]*):(.*)$`)
	completeRE = regexp.MustCompile(`^( *(?:- )?complete:)([ \t]*)([^#\r\n]*?)([ \t]*(?:#.*)?\r?)$`)
)

// setComplete is the plan block body with unit id's `complete:` set true:
// the value replaced when the key is there, else `complete: true` inserted
// after the item's id line at the item's key indent.
func setComplete(body, id string) (string, error) {
	lines := strings.SplitAfter(body, "\n")
	item, itemIndent := -1, -1
	// The units sequence's indent is its first item's, after `units:`.
	units := -1
	for i, l := range lines {
		t := strings.TrimRight(l, "\r\n")
		if units < 0 {
			if k := keyRE.FindStringSubmatch(t); k != nil && k[1] == "" && k[2] == "units" {
				units = i
			}
			continue
		}
		if m := itemRE.FindStringSubmatch(t); m != nil {
			itemIndent = len(m[1])
			break
		}
		if strings.TrimSpace(t) != "" && !strings.HasPrefix(strings.TrimSpace(t), "#") {
			break
		}
	}
	if itemIndent < 0 {
		return "", fmt.Errorf("the plan block has no unit items")
	}
	keyIndent := itemIndent + 2
	// An item runs from its "- " line to the next line at or left of the
	// item indent that is not blank.
	for i := 0; i < len(lines); i++ {
		m := itemRE.FindStringSubmatch(strings.TrimRight(lines[i], "\r\n"))
		if m == nil || len(m[1]) != itemIndent {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			t := strings.TrimRight(lines[j], "\r\n")
			if strings.TrimSpace(t) == "" {
				continue
			}
			if len(t)-len(strings.TrimLeft(t, " ")) <= itemIndent {
				end = j
				break
			}
		}
		idLine, completeLine := -1, -1
		for j := i; j < end; j++ {
			t := strings.TrimRight(lines[j], "\r\n")
			if j == i {
				t = strings.Repeat(" ", keyIndent) + m[2]
			}
			k := keyRE.FindStringSubmatch(t)
			if k == nil || len(k[1]) != keyIndent {
				continue
			}
			switch k[2] {
			case "id":
				if yamlScalar(k[3]) == id {
					idLine = j
				}
			case "complete":
				completeLine = j
			}
		}
		if idLine < 0 {
			continue
		}
		if item >= 0 {
			return "", fmt.Errorf("unit %s has more than one item in the plan block", id)
		}
		item = i
		if completeLine >= 0 {
			l := lines[completeLine]
			nl := l[len(strings.TrimRight(l, "\n")):]
			c := completeRE.FindStringSubmatch(strings.TrimRight(l, "\n"))
			if c == nil {
				return "", fmt.Errorf("unit %s: cannot read its complete: line", id)
			}
			sp := c[2]
			if sp == "" {
				sp = " "
			}
			lines[completeLine] = c[1] + sp + "true" + c[4] + nl
		} else {
			l := lines[idLine]
			nl := "\n"
			if strings.HasSuffix(strings.TrimRight(l, "\n"), "\r") {
				nl = "\r\n"
			}
			lines[idLine] = withNewline(l)
			lines = slices.Insert(lines, idLine+1, strings.Repeat(" ", keyIndent)+"complete: true"+nl)
		}
	}
	if item < 0 {
		return "", fmt.Errorf("unit %s has no item in the plan block", id)
	}
	return strings.Join(lines, ""), nil
}

// parsedUnits is a plan block body's parse, as JSON, or the first problem.
func parsedUnits(body string) ([]byte, error) {
	p, problems := plan.ParsePlanGraph(body)
	if len(problems) > 0 {
		return nil, errors.New(problems[0].Statement)
	}
	return plan.MarshalJS(p, "")
}

// renderBookkeeping is the plan and archive after merged's bookkeeping.
// It checks its own work: the new block parses to the old one with exactly
// the merged units complete, each moved entry is in the new archive
// verbatim, and taking the inserted text out of the new archive gives the
// old. Any failure is an error and nothing is rendered.
func renderBookkeeping(planText, archive []byte, archiveOK bool, in bookIn, merged []bookUnit, date string) ([]byte, []byte, error) {
	if len(merged) == 0 {
		return nil, nil, errors.New("no merged unit")
	}
	ls := mdLines(string(planText))
	if _, _, ok := section(ls, "Plan Maintenance"); !ok {
		return nil, nil, errors.New("the plan has no `## Plan Maintenance` section")
	}
	entries := []string{}
	for _, u := range merged {
		ws, we, ok := section(ls, "Work Units")
		if !ok {
			return nil, nil, errors.New("the plan has no `## Work Units` section")
		}
		s, e, ok := entryRange(ls, ws+1, we, u.Unit)
		if !ok {
			return nil, nil, fmt.Errorf("unit %s has no `### %s` entry under `## Work Units`", u.Unit, u.Unit)
		}
		body := e
		for body > s+1 && isBlank(ls[body-1]) {
			body--
		}
		entries = append(entries, withNewline(joinLines(ls[s:body])))
		ls = slices.Concat(ls[:s], []mdLine{{text: stubLine(in, u) + "\n"}}, ls[body:])
	}
	for _, u := range merged {
		ps, pe, _ := section(ls, "Plan Maintenance")
		ls = appendToSection(ls, ps, pe, maintenanceEntry(in, u, date))
	}
	newPlan := joinLines(ls)

	// The flag, in the block.
	loc := planBlockRE.FindAllStringSubmatchIndex(newPlan, -1)
	if len(loc) != 1 {
		return nil, nil, fmt.Errorf("the plan has %d ```yaml %s blocks; exactly one is allowed", len(loc), plan.PlanTag)
	}
	oldBody, prob := plan.ExtractBlock(string(planText), plan.PlanTag)
	if prob != "" {
		return nil, nil, errors.New(prob)
	}
	body := newPlan[loc[0][2]:loc[0][3]]
	for _, u := range merged {
		var err error
		if body, err = setComplete(body, u.Unit); err != nil {
			return nil, nil, err
		}
	}
	newPlan = newPlan[:loc[0][2]] + body + newPlan[loc[0][3]:]

	// The block parses back to the old one with the merged units complete.
	oldP, problems := plan.ParsePlanGraph(oldBody)
	if len(problems) > 0 {
		return nil, nil, fmt.Errorf("the existing plan block does not parse: %s", problems[0].Statement)
	}
	for _, u := range oldP.Units {
		for _, m := range merged {
			if u.ID == m.Unit {
				u.Complete = true
			}
		}
	}
	want, err := plan.MarshalJS(oldP, "")
	if err != nil {
		return nil, nil, err
	}
	newBody, prob := plan.ExtractBlock(newPlan, plan.PlanTag)
	if prob != "" {
		return nil, nil, fmt.Errorf("the rendered plan block: %s", prob)
	}
	got, err := parsedUnits(newBody)
	if err != nil || !bytes.Equal(got, want) {
		reason := "content differs"
		if err != nil {
			reason = err.Error()
		}
		return nil, nil, fmt.Errorf("the rendered plan block does not parse back to the old one with the merged units complete (%s)", reason)
	}

	// The archive.
	oldArchive := string(archive)
	if !archiveOK {
		oldArchive = ""
	}
	inserted := strings.Join(entries, "\n")
	var newArchive string
	if !archiveOK {
		newArchive = "# Archive: " + in.Name + "\n\n## Work Units\n\n" + inserted
	} else {
		al := mdLines(oldArchive)
		if s, e, ok := section(al, "Work Units"); ok {
			newArchive = joinLines(appendToSection(al, s, e, inserted))
		} else {
			sep := "\n"
			if strings.TrimSpace(oldArchive) == "" {
				sep = ""
			}
			newArchive = withNewline(oldArchive) + sep + "## Work Units\n\n" + inserted
		}
	}
	for _, e := range entries {
		if !strings.Contains(newArchive, e) {
			return nil, nil, errors.New("the rendered archive does not hold a moved entry verbatim")
		}
	}
	if !removedFrom(newArchive, oldArchive) {
		return nil, nil, errors.New("the rendered archive is not the old archive with entries added")
	}
	return []byte(newPlan), []byte(newArchive), nil
}

// removedFrom reports whether old is new with one contiguous run of text
// taken out: the archive is only ever added to.
func removedFrom(new, old string) bool {
	p := 0
	for p < len(old) && p < len(new) && old[p] == new[p] {
		p++
	}
	s := 0
	for s < len(old)-p && s < len(new)-p && old[len(old)-1-s] == new[len(new)-1-s] {
		s++
	}
	return p+s >= len(old)
}
