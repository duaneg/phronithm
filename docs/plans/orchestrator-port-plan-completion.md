# orchestrator-port-plan — completion report

Work unit `orchestrator-port-plan` of `docs/plans/lsf-improvements-breakdown.md` (entry at `d88f63b`). Branch `lsf-workflow`, commits `c59dd04..`. Related documents: plan `docs/plans/orchestrator-port-plan-plan.md`, implementation summary `docs/plans/orchestrator-port-plan-implementation.md`.

## Done condition: met, `proven`

- **The three modules are ported.** `schema.js`, `graph.js` and `scheduling.js` are ported to `lsf-runtime/plan`.
- **Every JS test is ported, and they pass.** That is all 50 tests: 8 from `schema.test.js`, 13 from `graph.test.js` and 29 from `scheduling.test.js`. Each Go test carries its JS title in a `// js:` comment, and a one-off check matched all 50.
- **The parity script runs both languages.** `go run ./tools/graphparity` (also `TestParity` under `go test` when node is installed) reports 145 cases identical, byte for byte. The cases include:
  - this plan's block, both live and pinned at `d88f63b`;
  - every `graph.test.js` fixture block, through `parseYaml`, `parsePlanGraph`, `parseDecisions` and `loadGraph`.
- **The extras are compared too:**
  - plan validation;
  - the ready set, blocked units, waves and merge order, before and after a completed wave;
  - block extraction;
  - the signal mappers, stop rule, digest and ledger;
  - every exported constant.
- **Mutation checks pass.** Each mutation applied to a message, an ordering rule or a skip makes the check fail.

## Deviations from the unit entry

- **Scope.** `CLAUDE.md`'s `lsf-runtime` entry was updated (describing `plan/` and graphparity). This is outside the Scope line, as was runtime-core's identical update.
- **Publishes, beyond "the plan graph, schema and scheduling rules in Go":**
  - `plan.MarshalJS`, a JSON encoder faithful to `JSON.stringify`. `encoding/json` escapes `<>&` and U+2028/9, and JS does not (D7).
  - The stage-result input types `ImplementResult`, `GateResult`, `Finding`, `MergeResult`, `Distress` and `Config`/`ConfigFrom`.
  - The `At` context struct.
- **Test placement.** The parity check is also a Go test (`tools/graphparity/main_test.go`). It skips without node, and fails instead when `GRAPHPARITY_REQUIRE_NODE=1` is set.

## Deferrals

None. Nothing in Scope was left undone.

## Discoveries for sibling and downstream units

**`orchestrator-port-loop`**
- **Prompt JSON.** Use `plan.MarshalJS(v, "  ")` for every `JSON.stringify(x, null, 2)` embedded in a prompt. `json.Marshal` is not byte-identical (D7).
- **Decode failures.** Stage results decode into typed structs, and a wrong-typed field fails the decode where JS coerces (D8). The loop must map that failure to `agent-null` for the stage, not to a run error.
- **Narrow input structs.** The mapper input structs are narrower than the stage results. For example, implement also returns `branch`, `gates`, `distress` and `summary`, and merge returns `planSha` and `architectureSha`. The loop's result types should embed them.
- **Panics.** `NewSignal` and `StageSchema` panic on programmer error. A panic in the run body ends the launch as `crashed` (`lsfrt.StopCrashed`).
- **Signal identity.** Signals are `*Signal`, and `EvaluateWave` stamps them in place. `NewSignal` copies `Wave`, so passing `&wave` is safe. A `Plan` and its signals are not safe for concurrent use: aggregate on one goroutine.
- **Config.** Build it with `ConfigFrom(args.config)`. A zero `Config` marks every unit distressed.
- **Load sequence.** `LoadGraph` may return a non-nil plan together with problems. The load stage stops on the problems and still runs `ValidatePlan` on the plan, as `implement.js` lines 1083–1090 do.
- **`WavePlan`.** It stops at the first uncleared organisational or release gate, so the "projected waves" narration (`implement.js` line 1115) omits gated units and everything downstream of them. The JS comment says the opposite. The port keeps the JS behaviour.

**`runtime-signals`, or whichever unit owns the stop rule's inputs**
- **Fail-open stop rule.** The stop rule fails open on a schema-violating enum, faithfully to the JS:
  - a merge status other than `clean`/`unresolvable`/`regressed` becomes `claimed-disjoint-conflict`, a record;
  - a finding severity other than `Significant`/`Minor` is dropped.
- **Digest wording.** The digest finds unfixed findings by their statement text (`Significant…`, `N Minor finding(s)`). A test pins the round trip, but the wording is a cross-site contract.

**`plan-preflight-check`**
- `plan.LoadGraph` followed by `plan.ValidatePlan` is the parser plus checks the command needs. graphparity shows they agree with `graph.js` on this plan.

**`script-retirement`**
- When the JS is deleted, delete `tools/graphparity` (and its node dependency) too.
- `plan/doc.go`'s divergence register (D1–D8) then becomes the only record of where Go departs from the retired reference.
- The live-document parity inputs are optional. The pinned `testdata/graph/breakdown-d88f63b.md` keeps the plan-block case after the breakdown document is deleted.

**Frozen-JS defects found.** These are recorded, not fixed, under the Frozen JavaScript rule:
- **D1.** `parseMap` reports a key named after an `Object.prototype` member as a duplicate.
- **D2.** `readySet` and `blockedUnits` throw on an unknown dependency.
- **`wavePlan`.** The comment contradicts the code, as described above.

## Standing obligations

- **Register dispositions.** This unit disposes of no register row; none is assigned to it.
- **Frozen JavaScript.** Discharged: `git diff --stat d88f63b -- workflows/` is empty.
- **`lsf-runtime/go.mod` stdlib-only.** Discharged: `go.mod` is unchanged, with no dependency and no `tool` directive.
- **Headless pin, Design of record.** These do not name this unit.

## External gates

- **`lsf-toolchain` (Go 1.27.1).** Confirmed at Phase 4 entry and unchanged at completion. `go version` reports `go1.27.1-X:nodwarf5` (the suffix is a local `GOEXPERIMENT`, not drift), and `go.mod` has `toolchain go1.27.1`.

## Execution signals

- **Context.** Comfortable at completion.
- **Design decisions taken mid-implementation, rather than at the design pass.** There were five, all prompted by per-step critiques or the first parity run, and all recorded in the implementation summary:
  - `MarshalJS`;
  - `omitempty` on `Type` and `Kind`, which narrowed D4;
  - `ClearedBy *string`;
  - `NewSignal` copying `Wave`;
  - the D4 parity normalisation in place of problems-only comparison.
- **Size.** About 5,800 lines added across 70 files, including the plan documents and 53 fixtures. The unit's flag held: the JS and its tests were the reference, so the size did not strain context.

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
