# orchestrator-port-plan — plan

Work unit `orchestrator-port-plan` of `docs/plans/lsf-improvements-breakdown.md` @ `d88f63b`. Base commit `d88f63b`, branch `lsf-workflow`. Phases 1–3 ran compressed in one plan-mode session, so this document is the problem statement, the design and the implementation plan. Phase 4 step 0 saves it as `docs/plans/orchestrator-port-plan-plan.md`.

## 1. Problem statement

### Context

S2 runs the orchestrator on the Go host runtime (`lsf-runtime/runtime`, unit `runtime-core`, complete). The orchestrator is still JavaScript: `workflows/lsf/{schema,graph,scheduling,implement}.js`. This unit ports the plan model (`schema.js` 225 lines, `graph.js` 357, `scheduling.js` 471; tests 55 + 220 + 365 lines) to package `plan` in `lsf-runtime/plan/`. `orchestrator-port-loop` ports `implement.js` on top of it. `plan-preflight-check` uses the ported parser. The JavaScript is frozen until `script-retirement`: a defect found in it is recorded, not fixed.

### Unit entry, verbatim

```
### orchestrator-port-plan
- **Type**: foundation
- **Risk**: medium — a parser and stop rule ported by hand; the JavaScript tests are the oracle
- **Clarity**: specified
- **Iteration**: write-once
- **Effort**: large
- **Scope**: `lsf-runtime/plan`, `lsf-runtime/tools`
- **Publishes**: the plan graph, schema and scheduling rules in Go
- **Depends on**: runtime-language-design (design, the language and layout)
- **External gates**: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`)
- **Done condition**: `schema.js`, `graph.js` and `scheduling.js` are ported to `lsf-runtime/plan`, with every test in their three test files ported and passing. The ported parser and `graph.js` read this plan's block and the fixture blocks of `graph.test.js` to the same graph and the same problems, checked by a script that runs both.
- **Description**: The plan model of the orchestrator: about 1,050 lines and 640 lines of tests in JavaScript. **Flag:** steps exceed the bound; the JavaScript and its tests are the behavioural reference, so the excess is not context.
```

Standing obligations that apply: **Register dispositions** (this unit disposes of no register row; the completion report says so). **Frozen JavaScript** (`workflows/` is not edited). **`lsf-runtime/go.mod`** stays stdlib-only. Headless pin and Design of record do not name this unit: its Scope has no `lsf-workflow.md::` or `lsf-runtime.md::` entry.

### Decisions confirmed with the requester (2026-10-06)

- **JS defects: fix and record.** Go behaves correctly where the JS has an error-path defect. Each divergence is listed in `plan/doc.go` and in the completion report. Parity fixtures avoid the divergent inputs. Known so far (probed with node 26.9.0):
  - D1. `parseMap`'s `key in out` sees inherited `Object.prototype` members, so a key named `constructor`, `toString`, `__proto__`, `valueOf` (etc.) is reported as `duplicate key`. Go reports a duplicate only for a key seen earlier.
  - D2. `readySet`/`blockedUnits` throw a TypeError on a dependency naming an unknown unit, and `readySet`/`mergeOrder` sort with a NaN comparator for an id missing from the execution order. `validatePlan` rejects both before the loop runs. Go treats an unknown dependency as incomplete and sorts a missing id last, stably.
- **Parity tool: a Go program**, `lsf-runtime/tools/graphparity`. It runs node once with a short inline JS snippet and compares JSON byte for byte. No new JS file.
- **Parity scope: done condition plus extras.** The extras are the schema constants' JSON, and `validatePlan`, `readySet`, `blockedUnits` and `wavePlan` over every plan block in the repo.

### Success criteria

Target `proven` (a `go test` assertion, or the committed parity program exiting 0) unless stated.

| # | Criterion | Target |
|---|---|---|
| S1 | `schema.js` ported: every exported constant, `stageSchema`, `signal`, `escalation` | proven |
| S2 | `graph.js` ported: `parseYaml`, `extractBlock`, `parsePlanGraph`, `parseDecisions`, `applyClearances`, `loadGraph`, `YamlError` | proven |
| S3 | `scheduling.js` ported: every export | proven |
| S4 | Every test in `schema.test.js` (8), `graph.test.js` (13) and `scheduling.test.js` (29) has a Go counterpart, and they pass. Traceability: each Go test carries the JS test title (unescaped, as node parses it) in a `// js:` comment. A one-off check in Step 7 matches them and records the counts | proven (tests). The title count is `evidenced` (one-off, recorded) |
| S5 | **Done condition, parity**: `go run ./tools/graphparity` exits 0. It shows that the following give byte-identical JSON (graph, decisions, problems, error text) in both languages: `loadGraph` on `docs/plans/lsf-improvements-breakdown.md`; `parsePlanGraph`/`parseDecisions`/`parseYaml` on every `graph.test.js` fixture body; and `loadGraph` on every fixture plan/decisions pair, which exercises `applyClearances` | proven (re-runnable program; needs node) |
| S6 | Parity extras, all byte-identical: the schema constants, kind lists and `DEFAULT_CONFIG`; `validatePlan` on every loaded plan; `readySet`, `blockedUnits` and `wavePlan` on each loaded plan that `validatePlan` accepts (as `implement.js` gates them). The repo blocks are the breakdown plan and the `docs/plan-graph.md` example (plan and decisions fences). `docs/plan-graph.md` fails validation (unknown dependencies), so only its `validatePlan` result is compared | proven (same program) |
| S7 | Every divergence D1–D4 and D7 (and any found during the port) has a Go test pinning the Go behaviour. D5 and D6 cannot be represented in Go, so they get no test. All are listed in `plan/doc.go` | proven |
| S8 | `go vet ./...` and `go test -race ./...` pass from `lsf-runtime/`. `go.mod` gains no dependency | proven |
| S9 | `workflows/` unchanged: `git diff --stat d88f63b -- workflows/` is empty | evidenced |

### Candidate approaches

**A. Faithful port: one Go file per JS module, typed structs for the graph and signals, a JS-semantics helper layer.** The structure follows the JS one-to-one: the same function order and names in Go case. The plan, signal and decision shapes are structs whose JSON tags and field order reproduce JS `JSON.stringify` output. That matters because `implement.js` embeds decisions in prompts, and `orchestrator-port-loop` must render them byte-identical. The YAML subset parses into a small ordered value model. A few helpers reproduce the JS semantics the error messages and graph values depend on: whitespace class, `String(v)`, `JSON.stringify(v)` and object key order. Builds on: `lsfrt`'s `json.RawMessage` schemas (`runtime/api.go` `Opts.Schema`).

**B. Idiomatic redesign: typed kinds (`type Kind string` constants), a sealed YAML node interface, errors instead of panics everywhere, schemas as Go structs marshalled.** Safer types, but it departs from the reference. Struct-marshalled schemas lose JS key order. Panics become error returns at 25+ `signal()` call sites in the loop port. The language record (`docs/design/lsf-runtime.md` §Weighing) found that typed kinds buy little, because the stop rule classifies by list membership.

**C. Generic `map[string]any` everywhere, mirroring JS dynamism.** It makes byte parity trivial for odd inputs, but the loop port gets no types, Go maps lose insertion order, and every consumer type-asserts.

## 2. Design

### Selection

**A.** Minimal-change and pragmatic stances converge on it: the JS is the oracle, and a one-to-one structure makes each Go function checkable against its JS source. Clean-architecture (B) is rejected for the reasons above. Typed kinds can be added by `runtime-signals` if a consumer needs them. C is rejected: it is untyped for every downstream unit.

### Files (`lsf-runtime/plan/`, package `plan`)

| File | Contents |
|---|---|
| `doc.go` | package doc (exists), extended with the **Divergences from the JavaScript** list |
| `schema.go` | kinds lists, schemas, `StageSchema`, `Signal`/`NewSignal`, `Escalation`/`NewEscalation` |
| `jsvalue.go` | JS-semantics helpers: `isJSSpace`, `jsTrim`/`jsTrimStart`/`jsTrimEnd`, `jsString`, `jsJSON`, `jsKeyOrder` |
| `yaml.go` | `YamlError`, `Map`, `ParseYAML` and its tokeniser and parsers |
| `graph.go` | `ExtractBlock`, `Problem`, `Plan`/`Unit`/`Gate`/`Dependency`, `ParsePlanGraph`, `Decision`, `ParseDecisions`, `ApplyClearances`, `LoadGraph` |
| `scheduling.go` | graph checks, ready set, merge order, wave plan |
| `signals.go` | signal mappers, `EvaluateWave`, `Digest`, ledger rendering |
| `*_test.go` | `schema_test.go`, `yaml_test.go` + `graph_test.go` (from `graph.test.js`), `scheduling_test.go` + `signals_test.go` (from `scheduling.test.js`), `divergence_test.go` |
| `testdata/graph/*.yaml`, `*.md` | the `graph.test.js` fixture bodies, one per file, content identical to the JS string (line numbers in messages must match). They are read by the Go tests and by `graphparity` |

`lsf-runtime/tools/graphparity/main.go`: package `main`.

### Value model and JS semantics (`yaml.go`, `jsvalue.go`)

`ParseYAML(text string) (any, error)` returns `nil | bool | string | []any | *Map`. The error is `*YamlError{Line int; Msg string}`, and its `Error()` is `line N: msg` (only `msg` when `Line == 0`), as JS formats it.

```go
// Map is a YAML mapping. Keys keeps JS property order: array-index keys
// ascending, then the rest in insertion order.
type Map struct { keys []string; vals map[string]any }
func (m *Map) Get(k string) (any, bool)
func (m *Map) Keys() []string           // jsKeyOrder(insertion order)
func (m *Map) MarshalJSON() ([]byte, error) // in Keys() order, JS escaping
```

The port mirrors `graph.js` function by function: `stripComment`, `tokenise`, `parseScalar`, `parseFlowSeq`, `parseValue`, `parseBlock`, `nested`, `parseSeq`, `parseMap`. These rules carry the JS semantics:
- **Whitespace** is the JS `\s`/`trim` set: `\t \n \v \f \r`, space, U+00A0, U+1680, U+2000–U+200A, U+2028, U+2029, U+202F, U+205F, U+3000, U+FEFF. It is not Go's `unicode.IsSpace`, which adds U+0085 and drops U+FEFF. Every JS `trim`/`trimStart`/`trimEnd` and every `\s` in a regex uses it. In Go regexps, `\s` becomes an explicit class built from the same set.
- **Indent** counts the runes of leading JS whitespace. All of them are BMP, so this equals the JS UTF-16 count. The same holds for `col` in `parseSeq`.
- **`KEY_RE`**: `^([A-Za-z0-9_][A-Za-z0-9_.-]*):(?:[<ws>]+([^\n\r  ]*))?$`. JS `.` excludes line terminators. Go's leftmost-first submatch semantics give the same captures for this pattern.
- **`stripComment`**: `#` counts as a comment when the preceding rune (decoded, not the preceding byte) is JS whitespace. A `\` inside double quotes skips one byte; the bytes it skips are never ASCII-significant.
- **Duplicate keys**: only a key already present in this mapping (D1).
- **`parseScalar`** keeps every unquoted value a string except `true`/`false`/`null`/`~`/empty. Double-quoted strings unescape only `\" \\ \n \t`.

`jsString(v)` is JS `String(v)` for the value model: bool to `true`/`false`, `[]any` joined by `,` with nil as empty, `*Map` to `[object Object]`. `jsJSON(v)` is JS `JSON.stringify(v)` for the value model. It uses no spaces and escapes `"`, `\`, `\b \f \n \r \t`, and other controls below U+0020 as lowercase `\u00xx`, and nothing else (no HTML or U+2028 escaping). Maps are written in `Keys()` order. `jsKeyOrder` puts canonical array-index keys (`0`…`4294967294`, no leading zeros) first in ascending order, then the rest in insertion order.

### Graph (`graph.go`)

```go
const PlanTag, DecisionsTag = "lsf-plan", "lsf-decisions"

type Problem struct { Kind string `json:"kind"`; Statement string `json:"statement"`; Referent string `json:"referent"` }

func ExtractBlock(markdown, tag string) (body string, problem string) // problem "" on success
```
- `ExtractBlock` uses the regex `(?m)^` + "```yaml[ \t]+" + `regexp.QuoteMeta(tag)` + "[ \t]*\r?\n((?s:.*?))^```[ \t]*\r?$". The trailing `\r?` stands in for JS multiline `$` matching before `\r`, so CRLF documents match as they do in JS. An extra divergence, recorded as D3: JS also treats a lone `\r`, U+2028 and U+2029 as line terminators for `^`/`$`, and Go does not. The JS call passes `markdown ?? ''`; Go's `string` has no null, so the caller passes `""`.
- Struct field order and tags reproduce the JS objects' key order exactly:

```go
type Plan struct {
    Units          []*Unit  `json:"units"`
    ExecutionOrder []string `json:"executionOrder"`
    Path           string   `json:"path,omitempty"` // set by the caller, as in JS
    Sha            string   `json:"sha,omitempty"`
}
type Unit struct {
    ID           string       `json:"id"`
    Type         string       `json:"type"`
    Complete     bool         `json:"complete"`
    Dependencies []Dependency `json:"dependencies"`
    Scope        []string     `json:"scope"`
    Publishes    []string     `json:"publishes"`
    Gates        []Gate       `json:"gates"`
}
type Dependency struct { Unit string `json:"unit"` }
type Gate struct {
    Name      string  `json:"name"`
    Kind      string  `json:"kind"`
    Cleared   bool    `json:"cleared"`
    Pin       *string `json:"pin,omitempty"`       // JS keeps pin "" distinct from absent
    ClearedBy string  `json:"clearedBy,omitempty"`
}
type Decision struct {
    ID, Date, Question, Decision string   // tags id, date, question, decision
    Clears, Resolves []string             // tags clears, resolves
}
type Graph struct { Plan *Plan; Decisions []Decision; Problems []Problem } // tags plan, decisions, problems

func ParsePlanGraph(body string) (*Plan, []Problem)        // nil plan when unreadable
func ParseDecisions(body *string) ([]Decision, []Problem)  // nil body: empty log
func ApplyClearances(p *Plan, ds []Decision) []Problem     // mutates gates
func LoadGraph(planMarkdown string, decisionsMarkdown *string) Graph
```
- Slices are always non-nil (`[]`), so JSON gives `[]`, never `null`.
- The problem statements and their order are the JS ones, verbatim. That includes the first-repeat order of duplicates: an id is reported when its second occurrence is reached, once (`new Set(filter(indexOf != i))`).
- `Type`, `Kind` and the gate `Name` hold the YAML value when it is a string. A non-string or missing value is converted as follows, recorded as D4 (for a missing `type` or `kind`, JS omits the key from the JSON and Go emits `""`): JS keeps `type: [a]` as the array, and Go stores `jsString(v)` for `Name` and `ID` (as JS `String()` does) and `""` for `Type` and `Kind`. An `invalid-plan` problem always accompanies it, with JS-identical text (the `got …` part uses `jsJSON`). The loop stops on any load problem, so no consumer sees the value. Parity fixtures carry only string values here.

### Schema (`schema.go`)

- `StopKinds`, `RecordKinds`, `Sources`, `GateKinds`, `UnitTypes`, `AmendmentKinds`, `InvalidationFields`, `StageCommonRequired` are `[]string` vars holding the JS lists in JS order, plus `IsStopKind(k string) bool`.
- The schemas are `json.RawMessage` vars, written as compact JSON literals equal to JS `JSON.stringify` of each frozen object: `StageCommonProperties`, `InvalidationSchema`, `PlanSchema`, `MergeResultSchema`, `DistressSchema`. Parity checks them byte for byte. `json.RawMessage` is the runtime's schema type (`lsfrt.Opts.Schema`).
- `StageSchema(result json.RawMessage) json.RawMessage` builds `{"type":"object","properties":{"result":<result>,"commits":…},"required":["result","commits"]}` with `result` embedded verbatim. It **panics** when `result` is not a JSON object whose `type` is `"object"`. The JS throws on the same programmer error, and the loop port calls it with literals at 17 sites.
- The signal types:

```go
type Signal struct {
    Kind        string `json:"kind"`
    Source      string `json:"source"`
    Stage       string `json:"stage"`
    Statement   string `json:"statement"`
    Referent    string `json:"referent"`
    RaisedBy    string `json:"raisedBy"`
    Unit        string `json:"unit,omitempty"`  // "" is absent
    Wave        *int   `json:"wave,omitempty"`  // wave 0 is the decision re-plan
    Disposition string `json:"disposition,omitempty"`
}
func NewSignal(s Signal) *Signal // panics on empty Kind or unknown Source; RaisedBy defaults to Stage
func W(n int) *int
```
  **Signals are `*Signal` everywhere**: mapper results, `EvaluateWave`, ledgers and `Escalation.Signals`. `implement.js` relies on object identity: `stopped()` stamps `disposition` on stops already in the ledger, and it uses `ledger.includes` and the `carried`/`scribed` identity sets. Pointers carry that identity over to Go. JS distinguishes `unit: null` from absent, and Go does not. `implement.js` only tests `s.unit` for truthiness, so this changes nothing (D5).
- The escalation record:

```go
type DecisionNeeded struct { ID string `json:"id"`; Question string `json:"question"`; Affects []string `json:"affects"` }
type Escalation struct {
    Workflow       string           `json:"workflow"`
    Stage          string           `json:"stage"`
    Signals        []*Signal        `json:"signals"`
    DecisionNeeded []DecisionNeeded `json:"decision_needed"`
    PlanSha        string           `json:"plan_sha,omitempty"`
    PreReplanSha   string           `json:"pre_replan_sha,omitempty"`
    Wave           *int             `json:"wave,omitempty"`
}
func NewEscalation(e Escalation) Escalation // nil DecisionNeeded and Signals become []
```

### Scheduling (`scheduling.go`)

```go
var BlockingGateKinds = []string{"organisational", "release"}
type Completed map[string]bool // nil is empty
type Pair struct { Units [2]string `json:"units"`; Serialised bool `json:"serialised"` }
type Blocked struct { Unit, Reason string; WaitingOn, Gates []string } // tags unit, reason, waitingOn, gates

func FindCycle(p *Plan) []string
func Ancestors(p *Plan, id string) map[string]bool
func PathsOverlap(a, b string) bool
func ScopesOverlap(a, b *Unit) bool
func OverlappingPairs(p *Plan, done Completed) []Pair
func ValidatePlan(p *Plan) []Problem
func ReadySet(p *Plan, done Completed) []*Unit
func BlockedUnits(p *Plan, done Completed) []Blocked
func MergeOrder(p *Plan, ids []string) []string
func WavePlan(p *Plan, done Completed) [][]string
```
- `unitIndex` lets the last duplicate win, as `Map.set` does. `FindCycle` is a recursive DFS in unit order, then dependency order. Its result and every message are the JS ones.
- `overlappingPairs` orders each pair by execution-order index, with a missing id at +∞ and a tie keeping `[a, b]`. The execution-order index map lets the last duplicate win, as `new Map(...)` does.
- In `Completed`, only true entries count: `false` is treated as absent. `WavePlan` copies `done` and never mutates the caller's map.

### Signals (`signals.go`)

`At` collects the context arguments the mappers share:

```go
type At struct { Stage, Unit string; Wave *int; Path string; TargetKB int }
```

The mapper inputs are the stage-result shapes, with JSON tags as the stages return them. `orchestrator-port-loop` decodes agent results into these types:
- `ImplementResult{Deviations, WritesOutsideScope []string; Deferrals []Deferral{Statement, BelongsTo}; Discoveries []Discovery{Statement, Affects}}`
- `Finding{Severity, Statement, Location, Stage string; Round int}`
- `GateResult{Invalidations []Invalidation{Section, Decision, Premise, Evidence, Amendment}; Notes []Note{Statement, Referent}; Amendments []Amendment{Unit, Section, Statement}}`
- `MergeResult{IntegrationBranch string; Merges []Merge{Unit, Status string; Files []string; Sha string}}`
- `Distress{ContextNearExhaustion bool; MidImplementationDesignDecisions, Deviations, Deferrals int}`
- `Config{DistressDeviationThreshold, DistressDesignDecisionThreshold int}` (tags `distressDeviationThreshold`, `distressDesignDecisionThreshold`), with `DefaultConfig = Config{4, 3}`. `ConfigFrom(raw json.RawMessage) (Config, error)` decodes onto a copy of `DefaultConfig`, so a partial override keeps the other default and an explicit 0 is honoured, as JS `{...DEFAULT_CONFIG, ...config}` does. Callers never build a partial `Config` literal

The functions, ported from the JS:
- `NullReturnSignal(at)`, `NoCommitSignal(at)`, `SignalsFromReadings(n int, at)` (the count only, as the JS reads only `length`)
- `SignalsFromDocumentSize(bytes float64, at)`, with `OversizeFactor = 2`. Non-finite bytes give nothing. KB is `math.Floor(x/1024 + 0.5)`, which is JS `Math.round` for positive x.
- `SignalsFromImplement`, `SignalsFromFindings` (`MinorLocations = 5`; Stage `""` takes the gate's stage; round 0 omitted, matching the JS truthiness test), `SignalsFromGate`, `InvalidationComplete`, `SignalsFromMerge`, `DistressSignals(d, at, cfg)`
- `ReplanItemKinds`, `PlanSupersedingStopKinds`
- `EvaluateWave(sigs []*Signal) WaveEval{Stop bool; Stops, Records, All []*Signal}`. It sets `Disposition` through the pointers, as JS mutates the objects. `All` is a new slice of the same pointers.
- `Digest(ledger []*Signal) DigestResult`. Counts use an ordered type, `Counts`, with its own `MarshalJSON` that writes keys in `jsKeyOrder`, to match JS object order. Every `countBy` and per-unit count keys an empty field as `"undefined"`, as JS does for a missing property. `attention` lists are non-nil.
- `LedgerHeading`, `LedgerWave(entries, wave int) int`, `LedgerLabel`, `RenderLedger(entries []*Signal, wave int) string`. `ledgerCell` uses the regex `\r?\n+` → space, `|` → `\|`, `jsTrim`, and `—` when empty. A nil `Wave` stands for both JS `undefined` and `null`. In JS `null` takes the `??` path in `ledgerWave` but fails `=== undefined` in `ledgerLabel`; since `signal()` never stores a null wave, the case cannot arise. This is D6, recorded as not representable.

### Parity program (`lsf-runtime/tools/graphparity`)

`go run ./tools/graphparity [-repo ..]`, run from `lsf-runtime/`.
1. Builds the cases.
   - **Fixtures**: every `plan/testdata/graph/*.yaml`. Each is run through `parseYaml` (value, or error message), `parsePlanGraph` and `parseDecisions`.
   - **Loads**: every `plan/testdata/graph/<name>.plan.md`, paired with `<name>.decisions.md` when that file exists and with null otherwise. Each pair is run through `loadGraph`, which exercises `extractBlock`, `parseDecisions` and `applyClearances`. Also loaded: `docs/plans/lsf-improvements-breakdown.md` (required; a missing file is a failure) with null, and `docs/plan-graph.md` as both plan and decisions document (it holds both fences). `skills/breakdown/SKILL.md` has no fence at line start, so it is not a plan input.
   - **Scheduling**: for each loaded plan, `validatePlan` unconditionally. Then, only when it returns no problems (as `implement.js` lines 1084–1090 gate the loop), `readySet` ids, `blockedUnits` and `wavePlan`. `docs/plan-graph.md`'s example fails validation (unknown dependencies), so it stops at `validatePlan`.
   - **Constants**: the five schemas, `stageSchema` of a sample object schema, the eleven kind lists (`STOP_KINDS`, `RECORD_KINDS`, `SOURCES`, `GATE_KINDS`, `UNIT_TYPES`, `AMENDMENT_KINDS`, `INVALIDATION_FIELDS`, `STAGE_COMMON_REQUIRED`, `BLOCKING_GATE_KINDS`, `REPLAN_ITEM_KINDS`, `PLAN_SUPERSEDING_STOP_KINDS`) and `DEFAULT_CONFIG`.
2. Runs node once: `node --input-type=module -e <snippet>`. The snippet imports `workflows/lsf/{graph,scheduling,schema}.js` by absolute `file://` URL and reads a JSON array of cases on stdin. It writes a JSON array of results, each `JSON.stringify(x, null, 2)`. A case that throws yields `{"threw": message}` instead of ending the run.
3. Computes the Go results in-process and marshals them with an encoder that has `SetEscapeHTML(false)` and a 2-space indent. It byte-compares each case. On a mismatch it prints the case name and the first differing line. It exits non-zero on any mismatch, on a missing node, or on a missing required input. On success it prints the count of cases.

**D7.** `encoding/json` escapes U+2028 and U+2029 and replaces invalid UTF-8 with U+FFFD; `JSON.stringify` does neither. The parity encoder and the struct types therefore match JS except on those characters, and parity inputs avoid them. `jsJSON`, used only in problem text, is JS-faithful. Byte-identical prompt rendering is `orchestrator-port-loop`'s concern, recorded for its completion report as a discovery.

The program is built in two halves: the fixtures-and-loads half after Step 3, and the scheduling-and-constants half in Step 6.

### Errors and panics

Parser and validation errors are values (`error`, `[]Problem`), as in JS. Panics occur only where JS throws on a programmer error: `NewSignal`, `StageSchema`.

### Risks and unknowns

| Risk | Resolution | When |
|---|---|---|
| A JS-semantics subtlety the tests do not exercise: whitespace, key order, `String()`, regex line terminators | Parity over two real docs and every fixture, plus targeted tests for each JS helper | during (steps 3, 6) |
| More frozen-JS defects found while porting | Record as Dn in `doc.go` and the completion report; the requester's fix-and-record rule applies | during |
| node missing on another machine | `graphparity` fails with a clear message. The Go tests do not need node | — |

No third-party APIs. The Go stdlib APIs used (`regexp` leftmost-first submatches, `(?m)`, `(?s:)`; `encoding/json` `SetEscapeHTML`, `RawMessage`) are verified by the parity run itself (from Step 3).

## 3. Implementation plan

Each step: implement, `gofmt -l`, `go vet ./...`, `go test -race ./plan/...`, commit, then spawn the critique gate (`phronithm:critic`, model sonnet, `critique-code`) on the step's diff while the next step proceeds. Commits are prefixed `orchestrator-port-plan step N:`.

**Step 0. Save the plan.** Copy this document to `docs/plans/orchestrator-port-plan-plan.md`, then commit.

**Step 1. `schema.go` + `schema_test.go`.** Port the constants, schemas, `StageSchema`, `NewSignal`, `NewEscalation` and `IsStopKind`, and the 8 `schema.test.js` tests.
*Accept:* tests pass. The schema literals are valid JSON, and the enum lists inside `PlanSchema` equal `UnitTypes` and `GateKinds` (a test unmarshals each and compares).

**Step 2. `jsvalue.go`, `yaml.go` + `yaml_test.go`; fixtures.** Write `testdata/graph/` with each `graph.test.js` fixture body as its own file. Content is identical to the JS string, since line numbers in messages must match.
- `yaml-mappings.yaml`, `yaml-scalars.yaml`, `yaml-empty.yaml`
- `yaml-err-01…10.yaml`, in the JS list's order
- `plan.yaml` (PLAN), `plan-defects.yaml`, `plan-unreadable-{1,2,3}.yaml`
- `decisions.yaml`, `decisions-defects.yaml`, `decisions-empty.yaml`
- `extract.md` (the `extractBlock` test's markdown)
- load pairs:
  - `withlog.plan.md` + `withlog.decisions.md`
  - `nolog.plan.md`
  - `noblock.plan.md`
  - `badlog.plan.md` + `badlog.decisions.md`
  - `clearances.plan.md` + `clearances.decisions.md`: the `applyClearances` test's D1/D2 clears, written as a log, which exercises the unknown and environment-gate problems
- `breakdown-d88f63b.md`: a copy of the breakdown plan's fenced block at `d88f63b`, for a pinned count

Port the four `parseYaml` tests. Add tests of each JS helper: the whitespace set, `jsString`, `jsJSON` escaping, `jsKeyOrder`. Add D1, and `a: ["x\` (a trailing backslash in a flow sequence; the Go port needs a bounds guard where JS reads `undefined`).
*Accept:* tests pass. The error messages match the JS regexes, line numbers included.

**Step 3. `graph.go` + `graph_test.go`; parity, first half.** Port `ExtractBlock`, `ParsePlanGraph`, `ParseDecisions`, `ApplyClearances` and `LoadGraph`, and the remaining 9 `graph.test.js` tests. Add the D3, D4 and D7 tests. A test loads `testdata/graph/breakdown-d88f63b.md` and expects no problems and 36 units. The live document is left to `graphparity`. Write `tools/graphparity` with the Fixtures and Loads cases, and run it.
*Accept:* tests pass, and `graphparity` exits 0 on its first half. A mismatch is handled as in Step 6.

**Step 4. `scheduling.go` + `scheduling_test.go`.** Port the graph checks, ready set, merge order and wave plan, and their 14 tests (`findCycle` … `wavePlan`). Add D2 tests, and a test that `WavePlan` leaves the caller's `Completed` unchanged and ignores `false` entries.
*Accept:* tests pass.

**Step 5. `signals.go` + `signals_test.go`.** Port the signal mappers, `EvaluateWave`, `Digest` and the ledger, and the remaining 15 `scheduling.test.js` tests. Add tests:
- `EvaluateWave` returns the caller's pointers;
- `ConfigFrom` with a partial override keeps the other default;
- `Counts` uses `jsKeyOrder`;
- `ledgerCell` CRLF.

*Accept:* tests pass. `go test -race ./...` passes from `lsf-runtime/`.

**Step 6. `tools/graphparity`, second half.** Add the Scheduling and Constants cases, run, and fix any divergence found:
- a Go bug: fix it in Go;
- a JS defect: record it as Dn with a Go test. If the input is a fixture-only edge, drop it from the fixtures. If it is a repo doc, the comparison is gated as `implement.js` gates the call (the `validatePlan` precondition). Never weaken the comparison otherwise.

*Accept:* exits 0. The case count is recorded in the implementation summary.

**Step 7. Divergences and traceability.** Finish the `doc.go` divergence list: D1–D7 plus any new ones, with D5 and D6 marked not representable. Run the one-off title check: a short Python script captures each `test('…'` title literal in the three JS files and unescapes it with `ast.literal_eval`. Each title must appear verbatim in a `// js:` comment in `plan/*_test.go`.

Record the counts (8/13/29 expected) in the implementation summary. Confirm S9 (`workflows/` unchanged) and that `go.mod` is unchanged.
*Accept:* every title is matched. `go vet`, `go test -race ./...` and `graphparity` pass.

Steps are sequential: each depends on the previous step's types. Steps 4 and 5 could run in parallel, but stay serial; the gain is small.

Phase 5: `phronithm:review` on `d88f63b..HEAD` with all applicable lenses and the static-analysis pre-scan, plus the critique gate on the full diff. Then the completion report.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/orchestrator-port-plan-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit orchestrator-port-plan of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim (Phase 4 re-verifies the external gates against these
pins before implementing, recording the outcome in the implementation-summary):

- Scope: `lsf-runtime/plan`, `lsf-runtime/tools`
- Publishes: the plan graph, schema and scheduling rules in Go
- Depends on: runtime-language-design (design, the language and layout)
- External gates: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`) — probe: `go version` reports go1.27.1, and `go.mod` has `toolchain go1.27.1`
- Done condition: `schema.js`, `graph.js` and `scheduling.js` are ported to `lsf-runtime/plan`, with every test in their three test files ported and passing. The ported parser and `graph.js` read this plan's block and the fixture blocks of `graph.test.js` to the same graph and the same problems, checked by a script that runs both.
- Standing obligations naming this unit: Register dispositions (no rows for this unit; say so); Frozen JavaScript (`workflows/` not edited); `lsf-runtime/go.mod` stdlib-only.

After Phase 5, produce the completion-report (contents per the
phronithm:feature skill's Produces) at docs/plans/orchestrator-port-plan-completion.md,
then present this continuation prompt to the requester:
Run `/phronithm:breakdown` in re-plan mode with docs/plans/lsf-improvements-breakdown.md and docs/plans/orchestrator-port-plan-completion.md before starting any dependent unit.
