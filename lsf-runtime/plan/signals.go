package plan

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Signals from stage returns, the stop rule, the digest and the ledger. See
// docs/design/lsf-workflow.md, "Escalation". Only three stages report
// anything beyond their result: the unit plan (readings), implement
// (deviations, deferrals, discoveries, distress) and the wave gate
// (invalidations and notes). Every one of those is a record except a
// complete invalidation. Nothing here classifies free text.

// At is where a signal was raised. Each mapper reads the fields it needs.
type At struct {
	Stage    string
	Unit     string
	Wave     *int
	Path     string
	TargetKB int
}

// NullReturnSignal is the STOP for a stage that returned null.
func NullReturnSignal(at At) *Signal {
	referent := at.Unit
	if referent == "" {
		referent = at.Stage
	}
	return NewSignal(Signal{
		Kind: "agent-null", Source: "structural", Stage: at.Stage, Unit: at.Unit, Wave: at.Wave,
		Statement: at.Stage + " returned null (skipped or terminal error)", Referent: referent,
	})
}

// NoCommitSignal is the STOP for an append stage that returned without a
// commit SHA: its write may sit uncommitted in the main checkout.
func NoCommitSignal(at At) *Signal {
	return NewSignal(Signal{
		Kind: "agent-null", Source: "structural", Stage: at.Stage, Wave: at.Wave,
		Statement: fmt.Sprintf("%s returned no commit SHA; %s may hold an uncommitted change in the main checkout", at.Stage, at.Path),
		Referent:  at.Path,
	})
}

// SignalsFromReadings is the unit plan stage's one row per unit counting
// the n clauses that admitted more than one reading; the readings live in
// the unit plan's Readings section at at.Path.
func SignalsFromReadings(n int, at At) []*Signal {
	if n == 0 {
		return []*Signal{}
	}
	return []*Signal{NewSignal(Signal{
		Kind: "reading", Source: "reported", Stage: "plan", Unit: at.Unit, Wave: at.Wave,
		Statement: fmt.Sprintf("%d reading(s); see %s Readings", n, at.Path), Referent: at.Path,
	})}
}

// OversizeFactor: the unit plan and completion report prompts state a size
// target the stage writes to once and never trims towards; a document past
// OversizeFactor times its target is recorded. A record, not a stop.
const OversizeFactor = 2

// SignalsFromDocumentSize records a document of bytes past OversizeFactor
// times at.TargetKB. A non-finite size (unknown) records nothing.
func SignalsFromDocumentSize(bytes float64, at At) []*Signal {
	if math.IsNaN(bytes) || math.IsInf(bytes, 0) || bytes <= float64(OversizeFactor*at.TargetKB*1024) {
		return []*Signal{}
	}
	kb := math.Floor(bytes/1024 + 0.5) // JS Math.round
	return []*Signal{NewSignal(Signal{
		Kind: "document-oversize", Source: "structural", Stage: at.Stage, Unit: at.Unit, Wave: at.Wave,
		Statement: fmt.Sprintf("%s is %s KB against a %d KB target", at.Path, strconv.FormatFloat(kb, 'f', -1, 64), at.TargetKB),
		Referent:  at.Path,
	})}
}

type Deferral struct {
	Statement string `json:"statement"`
	BelongsTo string `json:"belongsTo"`
}

type Discovery struct {
	Statement string `json:"statement"`
	Affects   string `json:"affects"`
}

// ImplementResult is the implement stage's reported fields.
type ImplementResult struct {
	Deviations         []string    `json:"deviations"`
	WritesOutsideScope []string    `json:"writesOutsideScope"`
	Deferrals          []Deferral  `json:"deferrals"`
	Discoveries        []Discovery `json:"discoveries"`
}

// SignalsFromImplement records deviations (including writes outside the
// Scope line), deferrals and discoveries, as breakdown's completion report
// defines them.
func SignalsFromImplement(r ImplementResult, at At) []*Signal {
	out := []*Signal{}
	add := func(kind, statement, referent string) {
		out = append(out, NewSignal(Signal{Kind: kind, Source: "reported", Stage: "implement", Unit: at.Unit, Wave: at.Wave, Statement: statement, Referent: referent}))
	}
	for _, d := range r.Deviations {
		add("deviation", d, at.Unit)
	}
	if len(r.WritesOutsideScope) > 0 {
		add("deviation", "writes outside the Scope line: "+strings.Join(r.WritesOutsideScope, ", "), strings.Join(r.WritesOutsideScope, ","))
	}
	for _, d := range r.Deferrals {
		add("deferral", d.Statement, d.BelongsTo)
	}
	for _, d := range r.Discoveries {
		add("discovery", d.Statement, d.Affects)
	}
	return out
}

// Finding is one review finding. Stage and Round are those of the review
// gate round that raised it; Round 0 is unknown.
type Finding struct {
	Severity  string `json:"severity"`
	Statement string `json:"statement"`
	Location  string `json:"location"`
	Stage     string `json:"stage,omitempty"`
	Round     int    `json:"round,omitempty"`
}

// MinorLocations is how many Minor-finding locations a count row names.
const MinorLocations = 5

// SignalsFromFindings records a review gate's findings over every round: a
// Significant finding is one row, against the stage and round that raised
// it; Minor findings are one count row, naming up to MinorLocations
// locations and no text. Critical findings are fixed, not recorded here.
// Any other severity is outside the schema and stops (D20).
func SignalsFromFindings(findings []Finding, at At) []*Signal {
	out := []*Signal{}
	var minors []Finding
	for _, f := range findings {
		switch f.Severity {
		case "Significant":
			stage := f.Stage
			if stage == "" {
				stage = at.Stage
			}
			round := ""
			if f.Round != 0 {
				round = fmt.Sprintf(" (round %d)", f.Round)
			}
			out = append(out, NewSignal(Signal{
				Kind: "finding", Source: "gate", Stage: stage, Unit: at.Unit, Wave: at.Wave,
				Statement: "Significant" + round + ": " + f.Statement, Referent: f.Location,
			}))
		case "Minor":
			minors = append(minors, f)
		case "Critical":
		default: // D20: JS drops any other severity
			stage := f.Stage
			if stage == "" {
				stage = at.Stage
			}
			out = append(out, NewSignal(Signal{
				Kind: "schema-violation", Source: "structural", Stage: stage, Unit: at.Unit, Wave: at.Wave,
				Statement: fmt.Sprintf("finding severity %s is outside the schema: %s", strconv.Quote(f.Severity), f.Statement), Referent: f.Location,
			}))
		}
	}
	if len(minors) > 0 {
		var locs []string
		for _, f := range minors[:min(len(minors), MinorLocations)] {
			locs = append(locs, f.Location)
		}
		out = append(out, NewSignal(Signal{
			Kind: "finding", Source: "gate", Stage: at.Stage, Unit: at.Unit, Wave: at.Wave,
			Statement: fmt.Sprintf("%d Minor finding(s)", len(minors)), Referent: strings.Join(locs, ", "),
		}))
	}
	return out
}

// Invalidation is an architecture invalidation as the wave gate returns it
// (InvalidationSchema).
type Invalidation struct {
	Section   string `json:"section"`
	Decision  string `json:"decision"`
	Premise   string `json:"premise"`
	Evidence  string `json:"evidence"`
	Amendment string `json:"amendment"`
}

func (inv Invalidation) field(k string) string {
	switch k {
	case "section":
		return inv.Section
	case "decision":
		return inv.Decision
	case "premise":
		return inv.Premise
	case "evidence":
		return inv.Evidence
	case "amendment":
		return inv.Amendment
	}
	panic("invalidation: unknown field " + k)
}

type Note struct {
	Statement string `json:"statement"`
	Referent  string `json:"referent"`
}

// Amendment is a merged unit's own change to the architecture document that
// moves no decision.
type Amendment struct {
	Unit      string `json:"unit"`
	Section   string `json:"section"`
	Statement string `json:"statement"`
}

// GateResult is the wave gate's reported fields.
type GateResult struct {
	Invalidations []Invalidation `json:"invalidations"`
	Notes         []Note         `json:"notes"`
	Amendments    []Amendment    `json:"amendments"`
}

// InvalidationComplete reports whether every field is non-blank.
func InvalidationComplete(inv Invalidation) bool {
	for _, k := range InvalidationFields {
		if jsTrim(inv.field(k)) == "" {
			return false
		}
	}
	return true
}

// SignalsFromGate maps the wave gate's result: an invalidation with every
// field present is a STOP; one with a field missing, every note and every
// amendment is a record.
func SignalsFromGate(r GateResult, at At) []*Signal {
	out := []*Signal{}
	gate := func(kind, unit, statement, referent string) {
		out = append(out, NewSignal(Signal{Kind: kind, Source: "gate", Stage: "review-gate", Unit: unit, Wave: at.Wave, Statement: statement, Referent: referent}))
	}
	for _, inv := range r.Invalidations {
		var parts, missing []string
		for _, k := range InvalidationFields {
			v := jsTrim(inv.field(k))
			if v == "" {
				missing = append(missing, k)
				v = "—"
			}
			parts = append(parts, k+": "+v)
		}
		statement := strings.Join(parts, "; ")
		if InvalidationComplete(inv) {
			gate("architecture-invalidated", "", statement, inv.Section)
		} else {
			gate("architecture-note", "", fmt.Sprintf("incomplete invalidation (%s missing) — %s", strings.Join(missing, ", "), statement), inv.Section)
		}
	}
	for _, n := range r.Notes {
		gate("architecture-note", "", n.Statement, n.Referent)
	}
	// A merged unit's own change to the architecture document that moves no
	// decision: recorded, so the digest shows the document moved.
	for _, a := range r.Amendments {
		statement := a.Statement
		if a.Unit != "" {
			statement = strings.TrimPrefix(statement, a.Unit+": ")
		}
		var parts []string
		for _, p := range []string{a.Unit, statement} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		gate("architecture-amendment", a.Unit, strings.Join(parts, ": "), a.Section)
	}
	return out
}

// Merge is one unit's merge outcome (MergeResultSchema).
type Merge struct {
	Unit   string   `json:"unit"`
	Status string   `json:"status"`
	Files  []string `json:"files"`
	Sha    string   `json:"sha,omitempty"`
}

type MergeResult struct {
	IntegrationBranch string  `json:"integrationBranch"`
	Merges            []Merge `json:"merges"`
}

// SignalsFromMerge maps the merge stage's result: unresolvable and
// regressed merges stop; a conflict resolved mechanically is a record; any
// other status is outside the schema and stops (D20).
func SignalsFromMerge(r MergeResult, at At) []*Signal {
	out := []*Signal{}
	for _, m := range r.Merges {
		if m.Status == "clean" {
			continue
		}
		files := strings.Join(m.Files, ", ")
		s := Signal{Source: "structural", Stage: "merge", Unit: m.Unit, Wave: at.Wave, Referent: files}
		switch m.Status {
		case "unresolvable":
			s.Kind, s.Statement = "merge-conflict-unresolvable", fmt.Sprintf("merge of %s conflicts in %s", m.Unit, files)
		case "regressed":
			s.Kind, s.Statement = "merge-regression", fmt.Sprintf("merge of %s was clean but the test suite failed in %s; merge undone", m.Unit, files)
		case "resolved":
			s.Kind, s.Statement = "claimed-disjoint-conflict", fmt.Sprintf("merge of %s conflicted in %s; resolved mechanically", m.Unit, files)
		default: // D20: JS records any other status as claimed-disjoint-conflict
			s.Kind, s.Statement, s.Referent = "schema-violation", fmt.Sprintf("merge of %s: status %s is outside the schema", m.Unit, strconv.Quote(m.Status)), m.Unit
		}
		out = append(out, NewSignal(s))
	}
	return out
}

// Distress is the completion-report fields the distress rule reads
// (DistressSchema).
type Distress struct {
	ContextNearExhaustion            bool `json:"contextNearExhaustion"`
	MidImplementationDesignDecisions int  `json:"midImplementationDesignDecisions"`
	Deviations                       int  `json:"deviations"`
	Deferrals                        int  `json:"deferrals"`
}

// Config holds the scheduling thresholds. Build it with ConfigFrom: the
// zero Config has both thresholds at 0, which marks every unit distressed.
type Config struct {
	DistressDeviationThreshold      int `json:"distressDeviationThreshold"`      // deviations + deferrals
	DistressDesignDecisionThreshold int `json:"distressDesignDecisionThreshold"` // mid-implementation design decisions
}

var DefaultConfig = Config{DistressDeviationThreshold: 4, DistressDesignDecisionThreshold: 3}

// ConfigFrom decodes overrides onto DefaultConfig, as JS
// {...DEFAULT_CONFIG, ...config} does: a field the overrides omit keeps
// its default. Empty raw is the default. Unlike JS, a null field keeps its
// default (JS compares against null, i.e. 0) and a non-integer is an
// error, returned with DefaultConfig.
func ConfigFrom(raw json.RawMessage) (Config, error) {
	cfg := DefaultConfig
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return DefaultConfig, err
	}
	return cfg, nil
}

// DistressSignals is the execution-distress record from a completion
// report's distress fields. Re-plan input: breakdown re-examines the size
// budget where a unit showed distress.
func DistressSignals(d Distress, at At, cfg Config) []*Signal {
	var reasons []string
	if d.ContextNearExhaustion {
		reasons = append(reasons, "context near exhaustion")
	}
	if d.MidImplementationDesignDecisions >= cfg.DistressDesignDecisionThreshold {
		reasons = append(reasons, fmt.Sprintf("%d mid-implementation design decision(s)", d.MidImplementationDesignDecisions))
	}
	if d.Deviations+d.Deferrals >= cfg.DistressDeviationThreshold {
		reasons = append(reasons, fmt.Sprintf("%d deviation(s) and %d deferral(s)", d.Deviations, d.Deferrals))
	}
	if len(reasons) == 0 {
		return []*Signal{}
	}
	return []*Signal{NewSignal(Signal{
		Kind: "execution-distress", Source: "reported", Stage: "implement", Unit: at.Unit, Wave: at.Wave,
		Statement: strings.Join(reasons, "; "), Referent: at.Unit,
	})}
}

// CallEvents are the driver-detected events of one agent call.
type CallEvents struct {
	Label       string   // the call's final label: the referent; it locates the transcript under calls/
	Writer      bool     // the stage commits: uncommitted changes stop
	Flags       []string // CallResult.Flags
	Denials     []string
	Uncommitted []string
	Violations  []string // CallResult.Violations, or the orchestrator's own schema check
}

// Listed is how many paths, denials or violations a driver row names.
const Listed = 5

// ItemRunes bounds each listed item, so a row stays one short cell.
const ItemRunes = 200

// listed is up to Listed items, each cut to ItemRunes, joined by sep, then
// "(and N more)".
func listed(items []string, sep string) string {
	cut := []string{}
	for _, it := range items[:min(len(items), Listed)] {
		if r := []rune(it); len(r) > ItemRunes {
			it = string(r[:ItemRunes]) + "…"
		}
		cut = append(cut, it)
	}
	s := strings.Join(cut, sep)
	if n := len(items) - Listed; n > 0 {
		s += fmt.Sprintf(" (and %d more)", n)
	}
	return s
}

// noOutput is the runtime's violation for a result with no structured
// output, which an output-token cut-off also produces.
const noOutput = "no structured_output"

// SignalsFromCall maps a call's driver events (orchestrator D21), in this
// order: schema-violation (stop); skill-not-loaded (stop);
// uncommitted-at-return for a writer (stop) or residue-at-return for a
// reader (record), also for flag uncommitted-unchecked; permission-denied
// (record); guard-killed-after-result (record). Every row is structural,
// its referent the call's label. Flags it does not know are returned for
// the caller to log.
func SignalsFromCall(ev CallEvents, at At) (sigs []*Signal, unknownFlags []string) {
	sigs = []*Signal{}
	add := func(kind, statement string) {
		sigs = append(sigs, NewSignal(Signal{Kind: kind, Source: "structural", Stage: at.Stage, Unit: at.Unit, Wave: at.Wave, Statement: statement, Referent: ev.Label}))
	}
	if len(ev.Violations) > 0 {
		if len(ev.Violations) == 1 && ev.Violations[0] == noOutput {
			add("schema-violation", "no structured output after retry (missing, or cut off at the output limit)")
		} else {
			add("schema-violation", fmt.Sprintf("%d schema violation(s): %s", len(ev.Violations), listed(ev.Violations, "; ")))
		}
	}
	has := func(f string) bool { return slices.Contains(ev.Flags, f) }
	for _, f := range ev.Flags {
		if !slices.Contains(callFlags, f) && !slices.Contains(unknownFlags, f) {
			unknownFlags = append(unknownFlags, f)
		}
	}
	if has("skill-not-loaded") {
		add("skill-not-loaded", "skill not loaded after retry; return accepted unverified")
	}
	uncommitted := "residue-at-return"
	if ev.Writer {
		uncommitted = "uncommitted-at-return"
	}
	if len(ev.Uncommitted) > 0 {
		add(uncommitted, fmt.Sprintf("%d path(s) uncommitted at return: %s", len(ev.Uncommitted), listed(ev.Uncommitted, ", ")))
	} else if has("uncommitted-unchecked") {
		add(uncommitted, "uncommitted check failed")
	}
	if len(ev.Denials) > 0 {
		add("permission-denied", fmt.Sprintf("%d permission denial(s): %s", len(ev.Denials), listed(ev.Denials, "; ")))
	}
	if has("guard-killed-after-result") {
		add("guard-killed-after-result", "the guard timeout killed the call after its result; the result stands")
	}
	return sigs, unknownFlags
}

// callFlags are the CallResult flags SignalsFromCall maps.
var callFlags = []string{"skill-not-loaded", "uncommitted-unchecked", "guard-killed-after-result"}

// ShaCorrectedSignal records a returned SHA that was not branch's tip.
// tip "" means the check could not be made; err says why.
func ShaCorrectedSignal(at At, branch, reported, tip, err string) *Signal {
	if reported == "" {
		reported = "no SHA"
	}
	statement := fmt.Sprintf("%s: reported %s, tip %s; corrected", branch, reported, tip)
	if tip == "" {
		if err == "" {
			err = "unknown error"
		}
		statement = fmt.Sprintf("%s: reported %s; not checked: %s", branch, reported, err)
	}
	return NewSignal(Signal{Kind: "sha-corrected", Source: "structural", Stage: at.Stage, Unit: at.Unit, Wave: at.Wave, Statement: statement, Referent: branch})
}

// ReplanItemKinds are the kinds the re-plan is handed with ids and must
// return a verdict on.
var ReplanItemKinds = []string{"deferral", "discovery", "claimed-disjoint-conflict", "architecture-note", "execution-distress"}

// PlanSupersedingStopKinds are the stop kinds whose relaunch is Phase 2's
// re-entry: the plan is superseded there, so a wave stopping on one skips
// the re-plan. An unevaluable done condition is not one: its rewrite is a
// decision the decision re-plan applies.
var PlanSupersedingStopKinds = []string{"architecture-invalidated"}

// WaveEval is a wave's signals classified by kind.
type WaveEval struct {
	Stop    bool      `json:"stop"`
	Stops   []*Signal `json:"stops"`
	Records []*Signal `json:"records"`
	All     []*Signal `json:"all"`
}

// EvaluateWave classifies a wave's signals by kind alone, stamping each
// signal's Disposition in place. All holds the same pointers.
func EvaluateWave(sigs []*Signal) WaveEval {
	ev := WaveEval{Stops: []*Signal{}, Records: []*Signal{}, All: slices.Clone(sigs)}
	if ev.All == nil {
		ev.All = []*Signal{}
	}
	for _, s := range sigs {
		if IsStopKind(s.Kind) {
			s.Disposition = "stopped"
			ev.Stops = append(ev.Stops, s)
		} else {
			s.Disposition = "recorded"
			ev.Records = append(ev.Records, s)
		}
	}
	ev.Stop = len(ev.Stops) > 0
	return ev
}

// Counts is a count per key, kept in JS object key order.
type Counts struct {
	keys []string
	n    map[string]int
}

func (c *Counts) add(k string, n int) {
	if c.n == nil {
		c.n = map[string]int{}
	}
	if _, ok := c.n[k]; !ok {
		c.keys = append(c.keys, k)
	}
	c.n[k] += n
}

// Keys returns the keys in JS object key order.
func (c Counts) Keys() []string { return jsKeyOrder(c.keys) }

// Get returns the count for k.
func (c Counts) Get(k string) int { return c.n[k] }

// Map returns the counts as a map.
func (c Counts) Map() map[string]int {
	out := map[string]int{}
	for k, v := range c.n {
		out[k] = v
	}
	return out
}

func (c Counts) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range c.Keys() {
		if i > 0 {
			b.WriteByte(',')
		}
		writeJSQuote(&b, k)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(c.n[k]))
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

// jsKey is a field as a JS object key: a missing value is "undefined".
func jsKey(s string) string {
	if s == "" {
		return "undefined"
	}
	return s
}

func countBy(ledger []*Signal, key func(*Signal) string) Counts {
	var c Counts
	for _, s := range ledger {
		c.add(jsKey(key(s)), 1)
	}
	return c
}

// Attention is what deserves the requester's attention on a run that did
// not stop.
type Attention struct {
	ArchitectureNotes      []string `json:"architectureNotes"`
	ArchitectureAmendments []string `json:"architectureAmendments"`
	UnfixedSignificant     Counts   `json:"unfixedSignificant"`
	MinorFindings          Counts   `json:"minorFindings"`
	DistressedUnits        []string `json:"distressedUnits"`
	DirectEdits            []string `json:"directEdits"`
}

// DigestResult summarises a ledger.
type DigestResult struct {
	Waves         int       `json:"waves"`
	ByKind        Counts    `json:"byKind"`
	BySource      Counts    `json:"bySource"`
	ByDisposition Counts    `json:"byDisposition"`
	Attention     Attention `json:"attention"`
	Note          string    `json:"note"`
}

var minorRE = regexp.MustCompile(`^(\d+) Minor finding`)

// Digest counts a ledger by kind, source and disposition, and lists what
// needs attention: architecture notes and amendments, unfixed Significant
// and Minor findings per unit, units that showed distress, and the edits
// made without a gate.
func Digest(ledger []*Signal) DigestResult {
	d := DigestResult{
		ByKind:        countBy(ledger, func(s *Signal) string { return s.Kind }),
		BySource:      countBy(ledger, func(s *Signal) string { return s.Source }),
		ByDisposition: countBy(ledger, func(s *Signal) string { return s.Disposition }),
		Attention: Attention{
			ArchitectureNotes: []string{}, ArchitectureAmendments: []string{}, DistressedUnits: []string{}, DirectEdits: []string{},
		},
		Note: "Silence is not evidence: reported-signal recall on premise-level defects is near zero.",
	}
	// Wave 0 is the decision re-plan before load, not a wave of units.
	waves := map[int]bool{}
	for _, s := range ledger {
		if s.Wave != nil && *s.Wave > 0 {
			waves[*s.Wave] = true
		}
		switch s.Kind {
		case "finding":
			if strings.HasPrefix(s.Statement, "Significant") {
				d.Attention.UnfixedSignificant.add(jsKey(s.Unit), 1)
			}
			if m := minorRE.FindStringSubmatch(s.Statement); m != nil {
				n, _ := strconv.Atoi(m[1])
				d.Attention.MinorFindings.add(jsKey(s.Unit), n)
			}
		case "architecture-note":
			d.Attention.ArchitectureNotes = append(d.Attention.ArchitectureNotes, s.Statement)
		case "architecture-amendment":
			d.Attention.ArchitectureAmendments = append(d.Attention.ArchitectureAmendments, s.Statement)
		case "execution-distress":
			if !slices.Contains(d.Attention.DistressedUnits, s.Unit) {
				d.Attention.DistressedUnits = append(d.Attention.DistressedUnits, s.Unit)
			}
		case "direct-edit":
			d.Attention.DirectEdits = append(d.Attention.DirectEdits, s.Referent+" — "+s.Statement)
		}
	}
	d.Waves = len(waves)
	return d
}

// LedgerColumns are the signal ledger's table columns.
var LedgerColumns = []string{"Wave", "Unit", "Stage", "Source", "Kind", "Statement", "Referent", "Disposition"}

var newlinesRE = regexp.MustCompile(`\r?\n+`)

// ledgerCell is one table cell: one line, pipes escaped, so a statement
// cannot break the table.
func ledgerCell(v string) string {
	v = jsTrim(strings.ReplaceAll(newlinesRE.ReplaceAllString(v, " "), "|", `\|`))
	if v == "" {
		return "—"
	}
	return v
}

// LedgerHeading is the ledger document's heading.
func LedgerHeading(name string) string {
	return "# Signal ledger: " + name + "\n\nAppend-only. Never edit earlier entries — add a later wave's row instead.\n"
}

// LedgerWave is the wave a ledger append is headed with: the one wave its
// rows name, else the loop's wave. A schedule stop reports the wave it
// could not start, one past the loop's counter; rows from the termination
// stages carry no wave.
func LedgerWave(entries []*Signal, wave int) int {
	waves := map[int]bool{}
	w := wave
	for _, e := range entries {
		if e.Wave != nil {
			w = *e.Wave
		} else {
			w = wave
		}
		waves[w] = true
	}
	if len(waves) == 1 {
		return w
	}
	return wave
}

// LedgerLabel is "termination" when no row carries a wave, else "wave N".
func LedgerLabel(entries []*Signal, wave int) string {
	if len(entries) > 0 && !slices.ContainsFunc(entries, func(e *Signal) bool { return e.Wave != nil }) {
		return "termination"
	}
	return fmt.Sprintf("wave %d", LedgerWave(entries, wave))
}

// RenderLedger is the Markdown a scribe appends verbatim: one table under
// the append's heading, one line per row.
func RenderLedger(entries []*Signal, wave int) string {
	label := LedgerLabel(entries, wave)
	sep := make([]string, len(LedgerColumns))
	for i := range sep {
		sep[i] = "---"
	}
	lines := []string{
		"## " + strings.ToUpper(label[:1]) + label[1:], "",
		"| " + strings.Join(LedgerColumns, " | ") + " |",
		"| " + strings.Join(sep, " | ") + " |",
	}
	for _, e := range entries {
		w := wave
		if e.Wave != nil {
			w = *e.Wave
		}
		cells := []string{strconv.Itoa(w), e.Unit, e.Stage, e.Source, e.Kind, e.Statement, e.Referent, e.Disposition}
		for i, c := range cells {
			cells[i] = ledgerCell(c)
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
	}
	return strings.Join(lines, "\n") + "\n"
}
