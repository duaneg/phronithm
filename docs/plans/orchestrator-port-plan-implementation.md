# orchestrator-port-plan — implementation summary

- **Branch:** `lsf-workflow`. Base `d88f63b`. Commits `c59dd04..` (listed per step below).
- **Plan:** `docs/plans/orchestrator-port-plan-plan.md`.
- **Work unit:** `orchestrator-port-plan` of `docs/plans/lsf-improvements-breakdown.md`.

## Gate re-probe (Phase 4 entry)

`lsf-toolchain`: `go version` reports `go1.27.1-X:nodwarf5 linux/amd64`, and `lsf-runtime/go.mod` has `toolchain go1.27.1`. The pin is confirmed. The `X:nodwarf5` suffix is the local `GOEXPERIMENT` and is not drift.

## Files

- `lsf-runtime/plan/`
  - `doc.go`
  - `schema.go`, `jsvalue.go`, `yaml.go`, `graph.go`, `scheduling.go`, `signals.go`
  - `schema_test.go`, `yaml_test.go`, `graph_test.go`, `scheduling_test.go`, `signals_test.go`
  - `testdata/graph/`: 50 fixtures; `testdata/signals/`: 3
- `lsf-runtime/tools/graphparity/main.go`, `main_test.go`
- `docs/plans/orchestrator-port-plan-plan.md`, this file
- `CLAUDE.md` (the `lsf-runtime` entry; outside the unit's Scope)

## Per step

| Step | Commit | Record |
|---|---|---|
| 0 | `c59dd04` | Plan saved. |
| 1 | `3355516` | Schema. Schemas are compact JSON literals, byte-identical to JS (confirmed later by the parity constants case). |
| 2 | `3b3bc72` | YAML parser, JS helpers and fixtures. Probing node confirmed D1. |
| 3 | `a63fa1b` | Graph loader and the first half of graphparity (90 cases). **Surprise:** `json.Marshal` escapes `<>&` as well as U+2028/9. **Deviation:** added `MarshalJS`, a JS-faithful encoder that the plan did not list. `Type` and `Kind` became `omitempty`, so a missing type matches JS, which narrows D4. A test file had picked up a literal U+2028 from the editor; it now uses escapes. |
| 3–4 fixes | `3cccdb1` | `ClearedBy *string`, because JS keeps an explicit empty decision id. `MergeOrder` never returns nil. Tests added for D3 U+2029, duplicate order, self-dependency, and absent-from-order. |
| 4 | `c2eeb5f` | Scheduling. |
| 5 | `6a145ba` | Signal mappers, stop rule, digest and ledger. |
| 5 fixes | `54e5bf2` | `NewSignal` copies `Wave`, so a caller's `&wave` counter cannot move ledger rows. `ConfigFrom` returns `DefaultConfig` on error. More tests. |
| 6 | `e91bfce` | Second half of graphparity, plus the `go test` wrapper. **Deviation:** D4 fixtures (`*.d4.yaml`) first compared problems only. The step 6 fixes replaced that with full-graph comparison after JS-side normalisation. |
| 6 fixes | `77171da` | Fixtures reaching every `validatePlan` kind. Views computed after the first wave (the completed set). Per-case panic recovery. An orphaned decision log is an error. `GRAPHPARITY_REQUIRE_NODE`. 124 cases. |
| 7 | `58e0967` | Divergence list in `doc.go`, Signal/Escalation JSON tests, and the traceability check. **Deviation:** no per-step critique gate on this step (docs and tests only). It is covered by the Phase 5 full-diff gate. |

## Per-step critique findings

The gate ran at the sonnet pin, round 1 every time. All passed with no Critical findings.

| Step | Significant | Disposition |
|---|---|---|
| plan | 7 (signal identity, partial config, parity inputs, decision-log coverage, wrong SKILL.md input, pinned count, U+2028) | Fixed in the plan before exit |
| 1 | — | Minor: D5-class empty-string notes go in `doc.go`; wave-0 JSON tests added (step 7) |
| 2 | `json.Marshal` is not JS-faithful | Fixed: `MarshalJS` (step 3) |
| 3 | `ClearedBy` omitempty; parity overstated | Fixed (`3cccdb1`, step 6) |
| 4 | — | Minor fixed (`3cccdb1`); exported mutable lists accepted |
| 5 | `Wave` pointer aliasing | Fixed (`54e5bf2`) |
| 6 | `validatePlan` kinds, completed set, `doc.go` overclaim | Fixed (`77171da`) |

Noted for Phase 5 and the completion report. These are loop-port discoveries from the step 5 critique:
- Decoding an agent result into `ImplementResult`/`GateResult`/… fails on a wrong-typed field that the JS would tolerate. The loop must map that failure to `agent-null`.
- The mapper input structs are narrower than the stage results, so the loop embeds them or decodes twice.

Accepted as is:
- Mutable exported lists.
- Trivial `schedule` cases counted in the 124.
- The live docs used as parity inputs, alongside the pinned breakdown fixture.

Two of the critics had no Bash, so their findings came from reading alone. One of their claims was wrong and is not acted on: that two of the D3 cases were not divergences. Checked with node, JS does find a block in all three.

## Success criteria

| # | Target | Achieved | Evidence |
|---|---|---|---|
| S1 schema.js ported | proven | proven, pass | `schema_test.go`; parity constants case |
| S2 graph.js ported | proven | proven, pass | `yaml_test.go`, `graph_test.go`; parity |
| S3 scheduling.js ported | proven | proven, pass | `scheduling_test.go`, `signals_test.go`; parity scheduling cases |
| S4 every JS test ported | proven / evidenced | proven (tests pass), evidenced (titles) | 8/13/29 JS titles. All 50 appear verbatim in `// js:` comments (one-off Python check using `ast.literal_eval` titles, run at `58e0967`) |
| S5 done condition, parity | proven | proven, pass | `go run ./tools/graphparity` reports "145 cases identical" at `0f4313b`. This includes `loadGraph` on the live breakdown plan and every `graph.test.js` fixture through parseYaml, parsePlanGraph, parseDecisions and loadGraph. `TestParity` runs it under `go test` when node is present. Mutation checks: a changed problem message, the cycle/overlap/order messages, and removing the completed-unit skip each fail it |
| S6 parity extras | proven | proven, pass | Constants case (kind lists, schemas, stageSchema, DEFAULT_CONFIG, OVERSIZE_FACTOR). `validatePlan` on every loaded plan. Ready/blocked/waves/mergeOrder, and the same after the first wave, on valid plans |
| S7 divergences tested and listed | proven | proven, pass | D1 `TestDivergenceD1PrototypeKeys`; D2 `TestDivergenceD2`, `TestSchedulingDuplicatesAndSelfDependency`; D3 `TestDivergenceD3LineTerminators`; D4 `TestDivergenceD4NonStringFields` and the `*.d4.yaml` parity normalisation; D7 `TestDivergenceD7Encoding`. D5 and D6 are not representable, as planned. All are listed in `plan/doc.go` |
| S8 vet and race tests; no dependency | proven | proven, pass | `go vet ./...`, `go test -race ./...` (plan, runtime, graphparity). `go.mod` unchanged since `d88f63b` |
| S9 `workflows/` unchanged | evidenced | evidenced, pass | `git diff --stat d88f63b -- workflows/` is empty |

## Phase 5

The gates ran in subagents at the sonnet pin: `phronithm:review` on `d88f63b..HEAD` (all lenses; `go vet` clean; staticcheck not installed), plus the critique gate on the full diff. Neither had Critical findings. The critique had one Significant finding: the `doc.go` divergence register overclaimed and was incomplete. Fixes are in `0f4313b`:
- The register was rewritten. D1–D4 arise only on a plan that loads with problems; D5–D8 can arise on any run. D8, input typing, is new.
- Panics and concurrency are now stated.
- Parity now covers the signal layer and block extraction (145 cases). Mutation checks on the new cases first left Counts key order and the CRLF regex undetected. Two fixture additions fixed that, and every applied mutation now fails.
- The live documents are optional.
- `WavePlan` comment corrected. `NewEscalation` copies Wave. `LoadGraph` documents the validate step. `ApplyClearances` uses `unitIndex`. A findings→digest round-trip test was added.
- `CLAUDE.md` updated.

Accepted:
- the zero-value `Config` hazard (documented; `ConfigFrom` required);
- exported mutable lists;
- quadratic `OverlappingPairs`;
- the `*string`/`string` asymmetry between ParseDecisions and ParsePlanGraph.

Carried to the completion report as discoveries:
- fail-open on an unknown merge status or finding severity (JS-faithful);
- panics at the stage boundary;
- typed-decode failure mapping to `agent-null`;
- the mapper structs being narrower than the stage results;
- `WavePlan` stopping at gates (the JS comment disagrees with the JS code).

End-to-end: graphparity is the end-to-end check. It runs the real breakdown plan, `docs/plan-graph.md` and every fixture through both languages.
