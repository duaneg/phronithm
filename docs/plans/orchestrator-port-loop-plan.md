# orchestrator-port-loop — plan

Work unit `orchestrator-port-loop` of `docs/plans/lsf-improvements-breakdown.md` @ `cfaf279`. Base commit `cfaf279`, branch `lsf-workflow`. Phases 1–3 ran compressed in one plan-mode session, so this document is the problem statement, the design and the implementation plan. Phase 4 step 0 saves it as `docs/plans/orchestrator-port-loop-plan.md`.

## 1. Problem statement

### Context

S2 runs the orchestrator on the Go host runtime (`lsf-runtime/runtime`, package `lsfrt`, unit `runtime-core`). The plan model is ported (`lsf-runtime/plan`, unit `orchestrator-port-plan`). The wave loop and its prompt builders are still JavaScript: `workflows/lsf/implement.js`, 1,293 lines, with 83 tests in `test/implement.test.js` (1,512 lines). This unit ports them to `lsf-runtime/orchestrator`. The JavaScript is frozen until `script-retirement`: a defect found in it is recorded, not fixed.

### Unit entry, verbatim

```
### orchestrator-port-loop
- **Type**: foundation
- **Risk**: high — the wave loop and its prompts; a divergence changes every stage
- **Clarity**: specified
- **Iteration**: write-once
- **Effort**: large
- **Scope**: `lsf-runtime/orchestrator`, `lsf-runtime/tools`, `CLAUDE.md::lsf-runtime`
- **Publishes**: the orchestrator `run(runtime)` in Go
- **Depends on**: orchestrator-port-plan (logical, the plan model); runtime-core (logical, `Run`, `rt.Call` and worktree isolation)
- **External gates**: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`); `claude-code-headless` (environment, Claude Code 2.1.289)
- **Done condition**: `implement.js` is ported to `lsf-runtime/orchestrator`, with every test in `implement.test.js` ported and passing. A script shows each prompt builder renders byte-identical text in both languages for the same inputs. The spike fixture's end-to-end path (run 2) completes on the ported orchestrator and the runtime. Every call label is unique within a launch, and a multi-wave test logs no `label-reused`. A stage result that fails to decode into its typed struct, including a wrong-typed field JavaScript would coerce, is that stage's `agent-null`, not a run error; a test shows it, and the divergence is recorded with the plan package's D1–D8.
- **Description**: About 1,300 lines and 1,500 lines of tests in JavaScript. The JavaScript stays the Workflow script's source, unchanged, until `script-retirement`. The cleanup prompt, ported unchanged, names the Workflow tool's `worktree-wf_*` branches, so it removes none of the runtime's `lsf/<plan8>/<key>` worktrees until `code-stage-interface`. **Flag:** steps exceed the bound; the JavaScript and its tests are the behavioural reference, so the excess is not context.
```

Standing obligations naming the unit: **Headless pin** (check `claude-code-headless` against `~/.local/share/claude/versions/2.1.289` run with `DISABLE_AUTOUPDATER=1`, or cite a transcript's `version`). Programme-wide: **Register dispositions** (no row is assigned to this unit; the report says so), **Frozen JavaScript** (`workflows/` not edited), **`go.mod` stdlib-only**. Design of record does not apply: Scope has no `lsf-workflow.md::` or `lsf-runtime.md::` entry.

Upstream discoveries adopted (`orchestrator-port-plan-completion.md`, `runtime-core-completion.md`): prompt JSON through `plan.MarshalJS(v, "  ")`; decode failure → `agent-null`; result types embed the narrow `plan` input structs; a body panic ends the launch `crashed`; aggregate signals on one goroutine; `ConfigFrom(args.config)`; the load sequence runs `ValidatePlan` on a plan returned with problems; `WavePlan`'s narration keeps the JS behaviour; labels must be unique per call.

### Decisions confirmed with the requester (2026-10-06)

- **Label uniqueness: generic `#n` suffix.** `implement.js` reuses `ledger:w<n>` when a stop or pause scribes after the wave's own scribe: a `no-ready-unit` stop, a termination or obligations stop, a pause after a null scribe, and the load and decision-replan scribes at wave 0. The port's agent wrapper renames the nth use of a label in a launch to `<label>#n` (n ≥ 2). The parity script applies the same rule to the JS call transcript.
- **Divergences: `orchestrator/doc.go`, D9 onward**, with a one-line pointer added to `plan/doc.go`. That line is outside Scope and is recorded as a deviation.
- **End-to-end launch: `tools/fixturerun`**, a Go command, not part of `go test`. Every stage is a model call until `code-stage-interface`: about 14 calls, a few USD.

### Success criteria

Target `proven` unless stated.

| # | Criterion | Target |
|---|---|---|
| S1 | `implement.js` ported: every prompt builder, the schemas, `critiqueLoop`, `runUnit`, `renderCompletionReport`, `replanSignals`, `decisionsFor`, `run`; `LENSES_BY_TYPE`, `GATE_MAX_ROUNDS`, `LONG_GATES` | proven |
| S2 | All 83 tests in `implement.test.js` have Go counterparts that pass. Each carries the JS title in a `// js:` comment; a one-off check matches the 83 titles and records the count | proven that the Go tests pass. `evidenced` that they faithfully port the JS assertions: the title match, plus a side-by-side read of the assertions of every test with loops or more than five asserts |
| S3 | **Prompt parity**: `go run ./tools/promptparity` exits 0. It runs the same scenarios through both orchestrators with the same args and the same return per label. It compares, byte for byte, every call's prompt, the logs and the outcome JSON. It compares each call's opts semantically, schema included, and the order of calls within each lane (§2.6). Builder coverage is measured on the JS side: the run fails unless every function in `implement.js` executes. Block coverage of `implement.js` and Go statement coverage of `prompts.go`, `report.go` and `loop.go` are printed, and the uncovered blocks are reviewed and recorded | proven for the scenarios' prompts and for function coverage; `evidenced` for branch exhaustiveness (the recorded block-coverage review) |
| S4 | **End to end**: `go run ./tools/fixturerun` on the spike fixture returns `complete` under `~/.local/share/claude/versions/2.1.289` with `DISABLE_AUTOUPDATER=1`, within tight caps (§2.9). Also checked: <br>• every `calls/` transcript's init `claude_code_version` is 2.1.289; <br>• `log.txt` holds no `label reused`; <br>• the fixture's `main` holds the `add-mean` merge, and `node --test` and `node scripts/lint.js` pass on it. <br>The count of calls flagged `skill-not-loaded` is reported | evidenced (a real paid run, not automatable in `go test`) |
| S5 | **Label uniqueness**: <br>• The `orchtest` recorder fails any test whose launch repeats a final label, so all 83 ported tests and every parity scenario check it. <br>• promptparity also checks the JS transcript after renaming. <br>• On the real runtime, an integration test runs a multi-wave scenario with a fake `claude`, ending in a stop whose scribe is `ledger:w2#2`. Its `log.txt` and `telemetry.jsonl` hold no `label-reused`. <br>• A relaunch on the same state dir replays every call (no spawn; `summary.json` `calls` 0) to the same outcome | proven |
| S6 | **Decode failure**: a stage result that passes no Go decode, a wrong-typed field (`branch: 5`, or a merge's `architectureSha: 42`, the latter from JS test "a document that did not move…"), is that stage's `agent-null` with a log line, not a run error (test). Integral floats (`3.0`) decode as integers, matching the runtime validator. D9+ recorded in `orchestrator/doc.go` | proven |
| S7 | `go vet ./...` and `go test -race ./...` pass from `lsf-runtime/`; `go.mod` unchanged | proven |
| S8 | `git diff --stat cfaf279 -- workflows/` is empty | evidenced |
| S9 | `CLAUDE.md`'s `lsf-runtime/` entry describes `orchestrator/`, `orchestrator/orchtest`, `tools/promptparity` and `tools/fixturerun` | evidenced |

### Candidate approaches

**A. Faithful port over a narrow runtime interface, typed stage results, and parity by whole-run replay.** One Go function per JS function, in the JS order. The orchestrator depends on an interface (`Agent`, `Parallel`, `Log`, `Args`), which `*lsfrt.Runtime` satisfies and a test stub implements. Results decode into typed structs that embed the `plan` input types. Parity is a whole-run comparison: Go runs each scenario on the stub and records the return per label; node runs `implement.js` on a replay stub fed those returns. Every prompt, the logs and the outcome are compared. Builds on: `lsfrt.Runtime`'s method set (`runtime/agent.go`, `parallel.go`, `runtime.go`), `plan.MarshalJS`, the graphparity pattern (`tools/graphparity`: inline node snippet, `go test` wrapper that skips without node).

**B. As A, but per-builder parity by loading `implement.js` source with its internal functions exported** (read the file, append `export { … }`, import from a data URL). It compares each builder in isolation over hand-built inputs. But it needs input fixtures for some 20 builders, and it edits the module text at load time, which is a fragile coupling to the source layout. It does not check that the loop calls the builders with the same inputs. Rejected: A checks the builders and their call sites together, and needs no fixtures beyond the scenarios.

**C. Generated port: transpile the prompt template literals mechanically.** It is unlikely to be cheaper than transliteration for about 20 builders, and a generator is code that has to be retired with the JS. Rejected.

A, with its parity method, is chosen; the alternatives are weaker on oracle coverage (B) or cost (C).

### Exploration findings

- `lsfrt.Runtime` methods: `Agent(ctx, prompt, Opts) (json.RawMessage, error)`, whose only error is a `*StopError` and whose nil value is null; `Parallel(ctx, []func(ctx)(json.RawMessage, error)) ([]json.RawMessage, error)`, which runs branches concurrently, recovers a panic or non-stop error to a nil slot and returns the first stop after all branches; `Log(string)`; `Args() json.RawMessage`. `Run(ctx, cfg, body)` is the launch entry. A body panic or non-stop error yields `crashed`.
- `Opts{Label, Phase, Model, Effort, AgentType, Isolation, Schema}`. Isolation `worktree` gets `<state>/wt/<key>` on `lsf/<args.planSha[:8]>/<key>`, created from `args.integrationBranch`. The worktree prompt's `git reset --hard <base>` then moves that branch to the previous stage's branch, as in the spike.
- The journal is keyed by label: `noteLabel` logs `label-reused` and emits a `label-reused` telemetry event when a label recurs with a different input hash. Null returns are not journaled, so they re-run on resume.
- The stub `claude` (`testdata/claude`) has fixed modes and no per-prompt dispatch. The integration test therefore uses the test binary re-executed as a fake `claude` (TestMain guarded by an env var). It answers by the SHA-256 of the prompt from a table the stub run recorded, so the real runtime's concurrency does not matter.
- `plan` provides `NullReturnSignal`, `NoCommitSignal`, `SignalsFrom*`, `DistressSignals`, `EvaluateWave`, `Digest`, `RenderLedger`, `LedgerLabel`, `LedgerHeading`, `LoadGraph`, `ValidatePlan`, `ReadySet`, `BlockedUnits`, `MergeOrder`, `WavePlan`, `NewSignal`, `NewEscalation`, `StageSchema`, `MergeResultSchema`, `DistressSchema`, `InvalidationSchema`, `AmendmentKinds`, `ReplanItemKinds`, `PlanSupersedingStopKinds`, `ConfigFrom` and `MarshalJS`.
- The spike fixture is `workflows/lsf/spike/fixture.sh <dir>`: one unit `add-mean`, gates `node --test` and `node scripts/lint.js`, and the plan SHA printed last. Run 2's stage roles are `testdata/config.example.json`'s.

## 2. Design

### 2.1 Package layout (`lsf-runtime/orchestrator`)

| File | Contents |
|---|---|
| `doc.go` | Package doc; divergence register D9+ |
| `runtime.go` | `Runtime` interface; `Run(ctx, rt) (json.RawMessage, error)`; the `orch` struct (ctx fields, label counter, seeds) and its `agent`/`call` wrappers |
| `args.go` | args decoding, `requireArg`, defaults, tiers |
| `schemas.go` | every stage schema as `json.RawMessage` via `plan.StageSchema` |
| `results.go` | typed stage results; `decode[T]` |
| `prompts.go` | the 20 prompt builders and their shared clauses (`RETURN_FIELDS`, `pauseClause`, `ownFilesOnly`, `commitMessage`, `header`, `worktree`, `ENTRY_SEMANTICS`, `unitBlock`, `LONG_GATES`, `gateResults`, `renderGates`, `amendedClause`, `maintenanceForm`, `replanBoundary`) |
| `report.go` | `renderCompletionReport` |
| `unit.go` | `critiqueLoop`, `runUnit`, gate helpers (`unrunGates`, `gateNotRunSignal`) |
| `loop.go` | the launch body: decisions, load, wave loop, termination, obligations; `replanSignals`, `decisionsFor` |
| `orchtest/` | exported test harness used by the orchestrator tests, promptparity and the integration test. It imports `lsfrt` and `plan`, never `orchestrator`. It contains: <br>• a stub runtime that records calls and fails on a repeated final label; <br>• a `Parallel` that fails on an unexpected recovered panic; <br>• default handlers; <br>• fixtures: `Unit`, `Graph`, `ToBlock`, `Implemented`, … |

### 2.2 Runtime interface and entry

```go
type Runtime interface {
	Agent(ctx context.Context, prompt string, o lsfrt.Opts) (json.RawMessage, error)
	Parallel(ctx context.Context, thunks []func(context.Context) (json.RawMessage, error)) ([]json.RawMessage, error)
	Log(msg string)
	Args() json.RawMessage
}

// Run is run(runtime).
func Run(ctx context.Context, rt Runtime) (json.RawMessage, error)

// Body is Run with lsfrt.Run's body signature.
func Body(ctx context.Context, rt *lsfrt.Runtime) (json.RawMessage, error)
```

`Args()` that is nil or `null` is read as `{}`, as `runtime.args ?? {}` does.

- **Errors.** An `Agent` error (a `*StopError`) returns from `Run` at once, wrapped with nothing, so `lsfrt` records the stop reason. A missing required arg returns `fmt.Errorf("lsf-implement: args.%s is required", key)`, which `lsfrt` records as `crashed`, as the JS throw ends the Workflow. A `ConfigFrom` error or an args decode error does the same, with the decode error in the message.
- **Outcome** is `plan.MarshalJS(outcome, "")` of a struct with the JS key order: `status, workflow, escalation (omitempty pointer), digest, planSha, integrationBranch, waves, ledgerPath, unscribed (never nil), note (omitempty)`.
- **Wave units under `Parallel`.** Each thunk runs `runUnit`, stores its `unitResult` in a slice slot captured by index, and returns `json.RawMessage("true")`. A nil slot after `Parallel` (panic or error) becomes the JS fallback result: `aborted: "unit"` and `NullReturnSignal{stage: unit}`. The stored slot is ignored then. A stop from any branch is returned after all finish (`Parallel`'s contract). Signals are flattened in ready order on the calling goroutine. Each `runUnit` owns its own signal slice, and `plan` is read-only during the wave.

### 2.3 Agent wrapper: labels, seeds, decoding

- `o.agent(ctx, prompt, opts)`, in order. This matches JS, where the seed wrapper sits above `runtime.agent`. The node replay stub renames beneath the seed wrapper in the same way:
  1. If `args.seed` has the label, every use of it, log `<label>: seeded from args.seed; agent not spawned` and return the seed. If the seed's `result.pauseRequested` is true, it is set false first (re-marshalled through `map[string]any`).
  2. Otherwise make the label unique: a mutex-guarded `map[string]int` counts real calls, and the nth (n ≥ 2) becomes `label#n`.
  3. Call `rt.Agent`.
- `call[T](o, ctx, prompt, opts) (*ret[T], error)`, where `ret[T] = {Result T "result"; Commits []commit "commits"}`. It returns nil for a nil or `null` value. On a decode failure it logs `<label>: return does not decode (<err>); treated as null` and returns nil. The caller's existing null path then produces the stage's `agent-null`, so no new signal kind is introduced. Decoding uses plain `json.Unmarshal`, so unknown fields are ignored, as in JS.
- Reviewer and gate pairs call `o.agent` inside `Parallel` thunks and decode after the barrier, on the calling goroutine.

### 2.4 Args and context

`args` decodes into a struct of pointers where JS distinguishes `??` from truthiness:

| Field | Go | Rule (JS) |
|---|---|---|
| `name`, `planSha`, `architectureDoc`, `integrationBranch` | `*string` | required: nil or `""` errors |
| `planPath`, `requestRecord`, `decisionLogPath`, `ledgerPath`, `archivePath`, `pauseRef` | `*string` | `??` default |
| `architectureSha` | `optStr{v string; set bool}` with a value `Equal` | Unset until the load sets it, even to `""` (`??` keeps `""`). `header` renders unset as `HEAD` and `""` as empty. `amendedClause`'s `prevArch === ctx.architectureSha` is `Equal`; `prevArch \|\| …` and `prevArch ? …` test `v != ""`. `refreshPin` sets only a non-empty value after `jsTrim` |
| `decisionLogSha` | `string` | truthiness |
| `config` | `json.RawMessage` | `plan.ConfigFrom` |
| `maxWaves` | `*float64` | `??` 20; `wave > maxWaves`; rendered in the wave-cap note with JS number formatting (`strconv.FormatFloat(f, 'f', -1, 64)`) |
| `maxUnitsPerWave` | `*int` | `??` unbounded; JS `slice(0, n)` semantics, negative counting from the end |
| `tiers` | `map[string]*tier` | `{...DEFAULT_TIERS, ...args.tiers}`; a null role falls to `default`, as `?? ctx.tiers.default` does |
| `decisions` | `[]decision` (`id, question, decision, clears *[]string omitempty`) | used when non-empty |
| `unscribed` | `json.RawMessage` | decoded to `[]plan.Signal` only when a JSON array; each through `NewSignal` |
| `seed` | `map[string]json.RawMessage` | |

Other JS semantics handled explicitly:
- **Trim.** `committed()`, `refreshPin` and `carrier.trim()` use a local `jsTrim`, which duplicates the plan package's unexported JS whitespace class. `strings.TrimSpace` differs on U+FEFF and U+0085.
- **Optional wave.** `applyDirectEdits`'s wave is `*int`: the termination call has none (label `direct-edits`), and the decision re-plan's is 0 (label `direct-edits:w0`).
- **Wave 0 is falsy.** `wave || undefined` is nil when the wave is 0.

`tier{Model, Effort string}`. Unit-stage opts apply the `default` tier, then the role tier's non-empty fields: JS `{...tier(default), ...tier(role)}` keeps the default effort when the role omits it. Other stages take the named tier alone.

### 2.5 Typed results (`results.go`)

The JSON tags follow the schemas.

| Type | Fields |
|---|---|
| `copyResult` | PauseRequested, PlanBlock, DecisionsBlock, HeadBranch, ArchitectureSha |
| `unitPlanResult` | Branch, Sha, Path, Bytes `int`, Readings `[]{Clause, Reading}` |
| `findingsResult` | Findings `[]plan.Finding`, Systemic, Summary |
| `fixResult` | Branch, Sha, Applied `[]{Finding, Fix}`, NotApplied `[]{Finding, Reason}`, Gates `[]gateRun` |
| `implementResult` | embeds `plan.ImplementResult`; Branch, Sha, GateProbe `[]{Name, Pin, Observed, Drifted}`, Gates, Distress `plan.Distress`, DoneCondition `{Evaluable, Met, Evidence}`, StandingObligations `[]{Statement, Status}`, Summary |
| `completionResult` | Branch, Sha, Path |
| `mergeResult` | embeds `plan.MergeResult`; PauseRequested, PlanSha, ArchitectureSha |
| `gateResult` | `= plan.GateResult` |
| `replanResult` | PlanSha, Amendments `[]{Kind, Unit, Statement}`, OverBudget `[]{Unit, Disposition, Statement}`, Verdicts `[]{ID, Verdict, Statement}`, DirectEdits `[]directEdit{Path, Description}`, Summary |
| `terminationResult` | PlanSha, AppendedUnits, DirectEdits |
| `obligationsResult` | Obligations `[]{Statement, Disposition, Carrier, Evidence}` |
| `scribeResult` | Path, Sha, PauseRequested |
| `cleanupResult` | Removed `int`, Kept |

A gate run is `gateRun{Command, Status, Summary}`. Integer fields (`Bytes`, `Removed`, and the `plan.Distress` counts, which are plan's own) use a `jsInt` that accepts what the runtime validator's `integer` accepts (`runtime/validate.go`: a finite integral number, `3.0` included). `plan.Distress` takes plain `int`. Its integral-float case is a D-entry if the port finds it reachable; the validator lets `3.0` through.

JS identity tests become explicit state:
- `gateRun.gates !== impl.result.gates` becomes a `fixedGates bool`;
- `ledger.includes(s)` and the `Set`s of signals compare `*plan.Signal` pointers.

### 2.6 Prompts and parity

- **Builders** are transliterated function by function, as string concatenation with double-quoted Go strings, because the prompts are full of backticks. JS semantics that need explicit handling:
  - `JSON.stringify(x, null, 2)` → `plan.MarshalJS(x, "  ")`, over structs whose field order is the JS object's: criticals `{severity, statement, location, stage}`; items `{id, unit?, stage, kind, statement, referent}`; stops `{kind, unit?, stage, statement, referent}`; conflicts `plan.Merge`; decisions; direct edits.
  - The `${x ?? 'HEAD'}` and truthiness tests follow the pointer and string rules in §2.4.
  - `byteLength` is `len(text)` (UTF-8, as `TextEncoder`).
- **promptparity** (`tools/promptparity`):
  - **Scenarios** are Go functions over `orchtest`: args, a plan graph and handler overrides. They are taken from the ported tests so the variants are known to fire.
  - **Go side.** Each scenario runs on the stub runtime, which records every call `{label, prompt, opts}`, the value returned per label, the logs and the outcome or error.
  - **Node side.** One inline snippet (graphparity's pattern) imports `workflows/lsf/implement.js`. For each scenario it calls `run()` on a replay stub: `agent` renames repeated labels `#n`, the same rule as Go, and returns the recorded value for the label, or throws `no recorded return for <label>`. `parallel` is `Promise.all` with a catch to null; `log` records.
  - **Comparison.**
    - Call sets keyed by label: prompts byte for byte; opts by parsing both to JSON values and deep-equalling them. A repeated final label on either side fails the scenario.
    - Call order within each lane. A unit stage's lane is its unit (the label's last component). Every other call is in the main lane. JS interleaves units under `Promise.all`, but each lane's order is deterministic in both languages, including the reviewer pair and the gate/cleanup pair, which start in array order.
    - Logs, in order, byte for byte.
    - The outcome: JS `JSON.stringify(out)` against Go `MarshalJS`, byte for byte, or the error message.
  - **Coverage.**
    - The node side runs under `node:inspector` `Profiler.startPreciseCoverage({detailed: true})`. The run fails unless every function in `implement.js` has a non-zero count. It prints the uncovered block ranges, which are reviewed and recorded in the implementation summary (S3).
    - `go test -coverprofile` statement coverage of `prompts.go`, `report.go` and `loop.go` under the parity scenarios is printed.
    - The scenarios cover at least:
      - every label prefix, including `direct-edits` (termination), `direct-edits:w0` (decision re-plan) and `direct-edits:w<n>` (re-plan);
      - fix-plan, fix-code with gates, and fix-code for a design unit;
      - critique-design;
      - lenses for foundation, refactor, polish and the fallback;
      - review-gate with no amended clause, with per-unit diffs, with the missing-merge-SHA fallback, and with no prior pin;
      - replan with stops and conflicts, and replan without;
      - decision-log and decision-replan;
      - a header with and without `decisionLogSha`, and with `architectureSha` unset, `""` and set;
      - every `renderCompletionReport` branch, including probes, outside-scope, fix gates, `evaluable: false` and an empty summary;
      - `renderGates`' empty forms;
      - a wave-0 ledger no-commit stop;
      - carried rows;
      - termination with direct edits;
      - obligations, cleanup and a seeded label;
      - string data with `<`, `>`, `&`, U+2028, a non-BMP character, backticks and newlines.
  - **`go test` wrapper.** It skips without node; `PROMPTPARITY_REQUIRE_NODE=1` makes the skip a failure.

### 2.7 Divergence register (D9+, `orchestrator/doc.go`)

- **D9.** Stage results are typed. A return that fails to decode becomes that stage's null, logged: a wrong-typed field such as a number for a string, or `"3"` or `2.5` for an integer. JS either ignores the type (`typeof` tests), or fails later with a TypeError that `parallel` turns into a unit null. A missing field decodes to its zero value, where JS reads `undefined`. In production the runtime's schema validation rejects both before the orchestrator sees them: it retries the call, and then returns an unjournaled null. Integral floats (`3.0`) pass the validator and decode as integers.
- **D10.** The nth real call (n ≥ 2) with a label in a launch is `label#n`. JS reuses `ledger:w<n>` (sites listed in §1). Seeds match the label before renaming and apply to every use of it, as in JS.
- **D11.** Prompt JSON carries objects as typed structs: schema fields only, in schema order. This covers agent-returned objects (criticals, conflicts, direct edits) and the caller's `args.decisions`, where `clears` is kept when present, including `[]`, and dropped when absent or null. JS embeds each object with any extra keys, in the key order it arrived with.
- **D12.** Args are typed. A non-string string field, a fractional `maxUnitsPerWave`, or a non-object `seed` is a decode error, which crashes the launch with the message. JS coerces or ignores. A tier field that is null or `""` counts as unset.
- **D13.** A merge row naming a unit outside the wave still completes the unit (as JS). Its `merged` entry has empty fields, where JS renders `undefined` in the gate and re-plan prompts.

Further divergences found during the port are added here and reported. `plan/doc.go` gains: "D9 onward are the orchestrator's: see `orchestrator/doc.go`."

### 2.8 Integration test (S5)

`orchestrator/integration_test.go`. TestMain: when `LSF_FAKE_CLAUDE_TABLE` is set, the binary acts as `claude`.
- It reads stdin and hashes it with SHA-256.
- It looks the hash up in the table (`{hash: value}`).
- It prints an init line and a `result` line with `structured_output` set to the value, then exits 0. A miss prints an `is_error` result and exits 1.
- It increments a counter file in `LSF_FAKE_CLAUDE_STATE`.

The test:
1. Runs the scenario on the stub to build the table. The scenario is two waves and a re-plan, ending at an undischarged obligation, whose stop scribe is `ledger:w2#2`. Every return is schema-valid; the runtime validates them, and an invalid one would respawn. Table building fails if two labels share a prompt hash with different returns.
2. Creates a temp git repo with branch `integration`.
3. Runs `lsfrt.Run` with `ClaudeBin: os.Args[0]` and no skill rules, so prompts are sent unprefixed, and `orchestrator.Body` as the body.
4. Asserts:
   - the returned `Summary.Outcome` equals the stub run's, compared as parsed JSON;
   - `Summary.Calls` equals the number of distinct labels, so no retries happened;
   - neither `log.txt` nor `telemetry.jsonl` mentions `label-reused`/`label reused`;
   - `ledger:w2#2` was called.
5. Relaunches on the same state dir and asserts that the counter has not moved, `Summary.Calls` is 0, and the outcome is identical.

### 2.9 fixturerun (S4)

`tools/fixturerun`: flags `-claude` (default `~/.local/share/claude/versions/2.1.289`), `-dir` (work dir, default a temp dir that is kept) and `-config` (default `testdata/config.example.json`).
1. Runs `workflows/lsf/spike/fixture.sh <dir>/repo` and takes the last output line as `planSha`.
2. Writes a config from the example, with `stateDir`, `repo`, `claudeBin` and args `{name: stats, planSha, architectureDoc: docs/architecture/stats.md, integrationBranch: main}`. The caps are tightened to `maxCalls` 40, `maxCostUSD` 25 and `maxWallClock` 1h. The expected run is about 14 calls.
3. Sets `DISABLE_AUTOUPDATER=1` in its environment, which the runtime's spawns inherit: `spawn` leaves `cmd.Env` unset (`runtime/call.go`).
4. Runs `lsfrt.Run(ctx, cfg, orchestrator.Body)`.
5. Prints the summary, the outcome's status and the count of `skill-not-loaded` events in telemetry.
6. Exits non-zero unless all of the following hold:
   - `status` is `complete`;
   - `log.txt` holds no `label reused`;
   - every `calls/` transcript's init `claude_code_version` is the pinned one;
   - the repo's `main` has a merge commit for `add-mean`;
   - `node --test` and `node scripts/lint.js` pass in the main checkout.

### 2.10 Risks and unknowns

| Risk | Resolution | When |
|---|---|---|
| A transliteration slip in a prompt | promptparity, byte for byte, with measured coverage. It needs `Run`, so it follows the loop step; steps 1–3 rely on the ported tests' substring assertions until then | step 4 |
| A port bug that panics inside a unit thunk is masked as a unit null | the `orchtest` stub's `Parallel` fails the test on a recovered panic, unless the test declares it expected | step 1 |
| The JS harness's `Promise.all` interleaving differs from Go's sequential stub parallel | comparison keyed by label, not sequence; logs come only from barrier-level code, whose order is deterministic | design (done) |
| Model stages in the end-to-end run behave differently from run 2's code stages (haiku copy, agent merge) | the Workflow tool ran these prompts in the archipeligo and cartomancer streams; if run 2 fails, diagnose it and record a discovery, not a port defect, when the cause is the stage prompt | during step 8 |
| A JS test relies on JS-only behaviour (identity, `undefined` text) | port to the Go-observable equivalent and note it in the test comment; a D-entry if it is a behaviour change | during |

No third-party APIs. The undocumented surfaces are the `claude` stream shape, already recorded by runtime-core in `testdata/streams` and used by the fake, and git.

## 3. Implementation plan

Each step ends with `go vet ./... && go test -race ./...` green from `lsf-runtime/`, a commit, and a per-step critique gate (critique-code), per the skill.

**Step 0. Save the plan.** Copy this document to `docs/plans/orchestrator-port-loop-plan.md` and commit it. Re-probe the external gates:
- `go version` reports go1.27.1;
- `~/.local/share/claude/versions/2.1.289 --version`, run with `DISABLE_AUTOUPDATER=1`, reports 2.1.289.

Record both in the implementation summary.

**Step 1. Skeleton: runtime interface, args, tiers, schemas, results, agent wrapper, orchtest.**
- **Changes:**
  - `runtime.go`, `args.go`, `schemas.go`, `results.go`;
  - `orchtest/`: a stub runtime with label-prefix handlers, default handlers ported from the JS `defaults()`, a recorder, sequential `Parallel` with panic recovery and an optional concurrent mode, and the fixtures.
- **Tests:**
  - required args (JS test "required args are enforced");
  - tier defaults and `args.tiers` override (the two tier tests run in step 4, once stages exist, but the tier function is unit-tested here);
  - label suffixing;
  - the seed lookup, with its pause-report strip;
  - `decode` of null, valid, wrong-typed (`branch: 5`) and fractional-integer values.
- **Acceptance:** the tests pass, and the schemas are semantically equal to their JS counterparts by a spot check of three (the full check is promptparity's).

**Step 2. Prompt builders and the completion report.**
- **Changes:** `prompts.go`, `report.go`.
- **Tests:** the standalone JS tests:
  - `renderCompletionReport` section by section;
  - the long-gate recipe, executed in `sh` as the JS test does;
  - `LONG_GATES` placement, asserted on builder output.
- **Acceptance:** those tests pass.

**Step 3. `critiqueLoop`, `runUnit` and the loop.** One step, because the JS unit tests drive `run()`.
- **Changes:** `unit.go`, `loop.go`.
- **Tests:** the unit-level JS tests, run through `Run` on single-wave plans as the JS tests do:
  - hard cap;
  - declined Critical;
  - environment drift;
  - unevaluable done condition;
  - gate not run, no gate reported, a red gate, and a design unit with no gates;
  - review reading fix gates, the second fix round, and a fix dropping a gate;
  - the design fix schema;
  - fixed Criticals reaching the ledger.

  Then the remaining run-level JS tests:
  - waves and caps;
  - re-plan, its items and verdicts;
  - the ledger and scribes;
  - merge, gate, amendments and the pin;
  - load and decisions;
  - termination and obligations;
  - pause;
  - seeds and unscribed rows;
  - tiers;
  - the main-checkout file-ownership prompt clauses.

  Also new tests:
  - a decode failure at implement (`branch: 5`) is `agent-null` at `implement`, with the log line, and the run stops with an escalation rather than an error (S6). The `{1: 42}` case of "a document that did not move…" is ported as a merge `agent-null` stop, citing D9;
  - an integral float (`bytes: 3.0`) decodes;
  - a concurrent-parallel run of the two-wave scenario under `-race` gives the same outcome as the sequential run.
- **Acceptance:** all 83 JS tests are ported. A one-off script matches the `// js:` comments against the JS titles and prints 83/83, recorded in the summary.

**Step 4. promptparity.**
- **Changes:** `tools/promptparity/main.go` (scenarios, node snippet, comparison, coverage) and `main_test.go`.
- **Acceptance:**
  - `go run ./tools/promptparity` reports N scenarios identical, with every `implement.js` function covered, and exits 0.
  - The uncovered JS blocks and the Go coverage figures are reviewed and recorded.
  - Mutation check: altering one character in three different builders, and one log message, and swapping two sequential main-lane stages, each makes it fail. That is recorded, not committed.

**Step 5. Integration test with the real runtime (§2.8).**
- **Acceptance:** S5 passes under `-race`.

**Step 6. Divergence register and docs.**
- **Changes:**
  - `orchestrator/doc.go` D9 onward, with the D9 test (step 3) named in it;
  - the pointer line in `plan/doc.go`;
  - `CLAUDE.md`'s `lsf-runtime/` entry: `orchestrator/` and `orchtest/`, `tools/promptparity` (run with `go run ./tools/promptparity`, like graphparity) and `tools/fixturerun` (a paid run).
- **Acceptance:** S6 (register), S9.

**Step 7. fixturerun and the end-to-end run (§2.9).**
- **Changes:** `tools/fixturerun/main.go` and a small `main_test.go` that checks the config it builds with no spawn: dry-run config, `-dry-run` flag passes `DryRun`, and it asserts the `StopDryRun` summary.
- **Run:** once against 2.1.289. Record in the implementation summary: wall clock, calls, cost, the outcome status, the transcript versions, and the fixture repo's `main` log.
- **Acceptance:** S4. If the run fails on a stage prompt's behaviour rather than the port, diagnose it and record it as a discovery; the requester decides whether S4 stands.

Order: 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7. Steps 4, 5 and 7 depend on 3 and are independent of each other. They are run in that order to keep one change in flight.

## 4. Critique gate record

Round 1 (sonnet, critique-design) found no Critical findings: pass. Its 5 Significant and 7 Minor findings were folded into this plan before ExitPlanMode:

| Finding | Disposition |
|---|---|
| S1: coverage hand-listed | Measured JS function coverage, with a hard gate and a block review; variants extended; S3 rigour split (§1, §2.6) |
| S2: D9 vs the journal and validator; the `3.0` claim | `jsInt` accepts integral floats; D9 reworded, missing-field behaviour added; the S6 test uses `branch: 5` and `42` |
| S3: `*string` pointer identity | `optStr` with value `Equal`; truthiness rules stated; the `{1: 42}` test case named |
| S4: uniqueness shown once | The orchtest recorder and promptparity fail on any repeat; S5 also checks this on the real runtime |
| S5: fake claude preconditions | Schema-valid returns; `Calls` equals the distinct labels; a hash-collision check; the outcome compared from `Summary` as parsed JSON |
| M6: `Run` vs the `lsfrt.Run` body | `Body` added |
| M7: ordering, risk row, S2 rigour | Steps 3 and 4 merged; risk row rewritten; S2 rigour split |
| M8: seed and rename asymmetry | Seed above the rename, on both sides; D10 |
| M9: order-insensitive comparison | Per-lane order compared |
| M10: typed-args gaps | Null args read as `{}`; decisions folded into D11; `maxWaves` formatting; `jsTrim` |
| M11: panic masking | The stub `Parallel` fails on an unexpected panic |
| M12: end-to-end bounds and checks | Tight caps; merge and gates verified on `main`; `skill-not-loaded` count; field name aligned |

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/orchestrator-port-loop-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit orchestrator-port-loop of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim (Phase 4 re-verifies the external gates against these
pins before implementing, recording the outcome in the implementation-summary): see §1 "Unit entry, verbatim" above. External gates: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`); `claude-code-headless` (environment, Claude Code 2.1.289). Standing obligations naming this unit: Headless pin (check against `~/.local/share/claude/versions/2.1.289` run with `DISABLE_AUTOUPDATER=1`, or cite the `version` of a transcript it produced; `claude --version` does not discharge it). Also: Register dispositions (none assigned), Frozen JavaScript, `lsf-runtime/go.mod` stdlib-only.
After Phase 5, produce the completion-report (contents per the
phronithm:feature skill's Produces) at docs/plans/orchestrator-port-loop-completion.md,
then present this continuation prompt to the requester:
Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
