# orchestrator-port-loop — implementation summary

Branch `lsf-workflow`, base `cfaf279`. Plan: `docs/plans/orchestrator-port-loop-plan.md` (`74ef669`). Commits `a60e0f4..`.

## External gates (Phase 4 entry)

- **`lsf-toolchain`:** confirmed. `go version` reports `go1.27.1-X:nodwarf5`, and `go.mod` has `toolchain go1.27.1`.
- **`claude-code-headless`:** confirmed. `DISABLE_AUTOUPDATER=1 ~/.local/share/claude/versions/2.1.289 --version` reports `2.1.289 (Claude Code)`. The end-to-end run's transcripts are checked against the same pin (S4).

## Files

`lsf-runtime/orchestrator/` (new):
- code: `doc.go`, `runtime.go`, `args.go`, `schemas.go`, `results.go`, `prompts.go`, `report.go`, `unit.go`, `loop.go`, `wave.go`;
- tests: `runtime_test.go`, `report_test.go`, `helpers_test.go`, `run_test.go`, `integration_test.go`;
- harness: `orchtest/orchtest.go`, `orchtest/fixtures.go`, `orchtest/orchtest_test.go`.

`lsf-runtime/tools/promptparity/` (`main.go`, `scenarios.go`, `main_test.go`) and `lsf-runtime/tools/fixturerun/` (`main.go`, `main_test.go`) are new.

`lsf-runtime/plan/doc.go` gains one pointer line, and `CLAUDE.md`'s `lsf-runtime/` entry is updated. Both are outside Scope.

## Per-step record

**Step 1** (`a60e0f4`). Runtime interface, args, tiers, schemas, typed results, agent wrapper, orchtest.
- **Ordered-object schema builder.** Built so the schemas match implement.js byte for byte; the plan needed only semantic equality.
- **`distress` type.** A local type with `jsInt` counts converts to `plan.Distress`, in place of the plan's "plain `int` plus a possible D-entry".
- **`agent` returns its final label**, so decode logs name it.
- **`Run` was an args-only placeholder** until step 3.

**Step 2** (`ae292ed`). Prompt builders and the completion report.
- **Mechanical conversion.** The literal prompt text was converted mechanically from the JS template literals: a script JSON-quoted the literal segments and left `⟦expr⟧` markers, which I translated by hand. This reduced transcription risk across about 400 lines of prompt text.
- **Order of steps.** Step 3 was not separable, as planned (the JS unit tests drive `run()`).

**Step 3** (`f911178`). `critiqueLoop`, `runUnit` and the loop (`unit.go`, `loop.go`, `wave.go`, split by size).
- **Tests.** All 83 tests ported on their first passing run.
- **Surprise.** A malformed `args.unscribed` row panicked through `NewSignal`. It is now an args error, the analogue of the JS throw (D12).
- **The `{1: 42}` case** of "a document that did not move…" now stops at the merge (D9), as planned.

**Step 4** (`316417b`, `b597779`). promptparity.
- **Scenario fixes, not port defects.** Three scenario failures were inputs the schema forbids or that cross the D11 boundary:
  - decisions given as Go maps arrive with sorted keys;
  - `directEdits: null` and a missing `amendments` crash the JS, as recorded in D14;
  - `clears: null` is dropped by Go (D11).
- **Coverage scenarios.** Coverage-driven scenarios brought the uncovered `implement.js` blocks from 51 to 16. A seventeenth listed block (`onFix ?? []`) was found when the allowlist was written. Each of the 17 is on a reviewed allowlist with its reason.
- **Go statement coverage** of `orchestrator` under the scenarios is 94.0%. The rest is mostly stop-error returns.
- **Mutation check, recorded, not committed.** Each of these fails the check:
  - one-character edits in three builders (gate, completion scribe, cleanup);
  - a one-character edit in a log message;
  - swapping the gate and cleanup calls;
  - dropping a scenario that covers a reachable block.

**Step 5** (`cfdc8f8`). Integration test on the real runtime.
- **Fake claude.** The test binary doubles as the fake `claude`, so no build step is needed.
- **Schema validity.** `orchtest.Copied` gained `pauseRequested`: the runtime validates the copy schema, and the stub's return lacked a required field.
- **Timing.** About 27 s under `-race`, because each of about 50 spawns starts the race-instrumented binary; 0.2 s without.

**Steps 6–7** (`36f6df3`). Divergence register, `CLAUDE.md` and fixturerun.
- **New divergences.** D14 (null lists) and D15 (exceptions) were found by the parity work and added to the planned D9–D13.

## Critique findings and dispositions

No Critical findings at any gate.

| Gate | Significant | Disposition |
|---|---|---|
| Plan, round 1 | 5 | Folded into the plan before approval (§4 of the plan) |
| Step 1 | 1: orchtest's own checks untested | Fixed in `f911178`: `orchtest_test.go` |
| Step 2 | 2: typed structs vs pass-through objects (decisions, criticals) | D11, with parity scenarios in schema order; recorded |
| Step 3 | 1: no Run-level test of the panic fallback or stop propagation | Fixed in `316417b`: `TestPanickingUnitIsUnitNull`, `TestRuntimeStopPropagates` |
| Step 4 | 5: cross-lane order; lossy opts; Go coverage not in the tool; coverage gate weak; type-confusion scenarios | Fixed in `b597779`: barrier positions compared, unknown opts keys fail, reviewed unreachable allowlist enforced. Go coverage kept as a recorded figure (descoped from the tool). Type confusion is by design D9/D11/D14, tested in Go and outside parity's reach |
| Steps 5–7 | none (9 Minor) | Fixed in `9e873ae`: journal and cache-hit assertions; fake claude errors; fixturerun pin pre-check, fresh state dir, structural merge and deliverable checks, event-kind counting; D9 and D12 wording. Not acted on: a negative control with renaming disabled (the stub's repeat check is the first line); the long `CLAUDE.md` entry |

Minor findings fixed along the way:
- jsInt rejects null, and a missing `result` is null;
- `maxUnitsPerWave` accepts `2.0`;
- the stub's `Unit` drops `#n`, and its locking is documented;
- the long-gate test's kill race;
- the misplaced `unitBlock` comment;
- `TestReplanItems` fails on a missing row;
- an every-role tier test;
- the empty-question verdict suffix;
- more odd characters in the parity fixture;
- exact label shapes required;
- node version check, timeout and coverage-record error.

Minors not acted on, for Phase 5:
- `stripSeedPause` re-marshals through `map[string]any` (accepted in plan §2.3);
- `mergeResultSchema` copies two properties by name (pinned by a test);
- `jsonIndent` panics on an unmarshalable value (the types always marshal);
- the opts comparison cannot tell `""` from absent (no Go option can be `""` and set).

## Success criteria

| # | Status | Target / achieved | Evidence |
|---|---|---|---|
| S1 | pass | proven / proven | Every builder, schema and loop function ported; exercised by the 83 ported tests and promptparity |
| S2 | pass | proven + evidenced / proven + evidenced | `go test` passes. The title match is 83/83 (one-off script). Assertions of the loop and multi-assert tests were read side by side by the step 3 critique |
| S3 | pass | proven + evidenced / proven + evidenced | `go run ./tools/promptparity`: 55 scenarios identical; all 68 named functions run; 17 uncovered blocks, each on the reviewed unreachable allowlist; mutation check fails as expected |
| S4 | pass | evidenced / evidenced | `go run ./tools/fixturerun` (2026-10-06, the binary at `36f6df3`). Status `complete`, 16 calls, $3.41, 5 min. All 16 transcripts record `claude_code_version` 2.1.289, called through `~/.local/share/claude/versions/2.1.289` with `DISABLE_AUTOUPDATER=1`. No `label-reused`, 0 `skill-not-loaded`, 16 journal records. The fixture's `main` has the merge commit `merge(stats): add-mean`, `mean` and its `RangeError` test, and `node --test` and the linter pass on it. The checks added in `9e873ae` (a merge commit, `mean` and `RangeError`) were applied by hand to this run, which predates them. Stages: load (haiku copy), plan, plan critique, implement, code critique with lens review, completion report, merge, wave gate with cleanup (removed 0: it names `worktree-wf_*` branches, as the unit entry predicts), re-plan, ledger, termination and obligations. Every stage was a model call; in the spike's run 2, load, completion report, merge, cleanup and ledger were code |
| S5 | pass | proven / proven | `TestOnRuntimeLabelsUniqueAndReplay`, plus the stub's repeated-label failure in every test and scenario, plus `TestStopAfterScribeRelabelsLedger` |
| S6 | pass | proven / proven | `TestDecodeFailureIsAgentNull`, `TestIntegralFloatDecodes`, `TestDecode*`; D9–D15 in `orchestrator/doc.go` |
| S7 | pass | proven / proven | `go vet ./...` and `go test -race ./...` pass; `go.mod` unchanged |
| S8 | pass | evidenced / evidenced | `git diff --stat cfaf279 -- workflows/` is empty |
| S9 | pass | evidenced / evidenced | `CLAUDE.md` entry updated (`36f6df3`) |

## Remaining phases
Complete Phase 5 (Review) as defined in the phronithm:feature skill.
