# runtime-signals — implementation summary

- **Branch:** `lsf-workflow`, commits `aed6079..HEAD` (base `85dced0`).
- **Plan:** `docs/plans/runtime-signals-plan.md`.
- **External gate re-probe:** `lsf-toolchain` reports `go version go1.27.1`, and `go.mod` pins `toolchain go1.27.1`. The pin is confirmed, with no drift.

## Files modified

- **plan:**
  - `plan/schema.go`
  - `plan/signals.go`
  - `plan/doc.go`
  - `plan/signals_test.go`
- **runtime:**
  - `runtime/uncommitted.go` (new)
  - `runtime/uncommitted_test.go` (new)
  - `runtime/agent.go`
  - `runtime/api.go`
  - `runtime/journal.go`
  - `runtime/journal_test.go`
  - `runtime/validate.go`
  - `runtime/classify.go`
  - `runtime/worktree.go`
  - `runtime/helpers_test.go`
  - `testdata/claude`
- **orchestrator:**
  - `runtime.go`
  - `unit.go`
  - `wave.go`
  - `loop.go`
  - `stages.go`
  - `doc.go`
  - `signals_test.go` (new)
  - `integration_signals_test.go` (new)
  - `integration_test.go`
  - `run_test.go`
  - `runtime_test.go`
  - `stages_test.go`
- **tools:**
  - `tools/graphparity/main.go`
  - `tools/promptparity/main.go`
  - `tools/promptparity/scenarios.go`
- **docs:**
  - `docs/design/lsf-workflow.md` (Escalation, plus two `::lifecycle-mapping` parentheticals)
  - `docs/design/lsf-runtime.md`

## Per-step record

| Step | Commits | Decisions, deviations, surprises |
|---|---|---|
| 0 | `aed6079` | Plan saved. |
| 1 | `61317fd` | Kinds, `SignalsFromCall`, fail-closed mappers (D20). `plan.GoOnlyKinds` added so graphparity can filter. `ShaCorrectedSignal` takes the check's error (deviation from the plan's signature). Graphparity's D20 case: the JS row for an unknown merge status is byte-identical to the row for `resolved`, so the Go side checks D20 on the original input, then compares with the status rewritten. |
| 2 | `d784d83`, `5427976` | `CallResult.Uncommitted`, `.Violations`; snapshot and check; `ValidateOutput`; `Step` ctx on `killCh`. **Surprises:** (1) `testConfig`'s `Repo` was a bare temp dir, so it is now a git repository; (2) a failed check that is not journaled made a replay pass unchecked (critique, Significant), so the record now keeps `unchecked` and a replay checks afresh. **Decision:** content is compared by sha256 and mode read directly, not `git hash-object`, so a vanished file is not an error. |
| 3a | `eebedcd` | `site`, the new `call`, `callSigs`; all 13 call sites; precedence per §2.4. A placeholder in an edit script overwrote one line, and was caught and restored before the build. |
| 3b | `09b6bf4` | **Deviation from S6:** the orchestrator re-validates seeds only, not stub returns. Re-validating stubs broke promptparity's deliberately lax scenarios (the JS tolerance branches), and the stub stands in for a runtime that validates every live return. The `Runtime` contract now says a non-null Value is schema-valid (`369d573`). Two seeded fixtures gained `bytes`. |
| 3c | `369d573` | `gate-omitted` split; `stage-fault`. **Surprise:** JS lists the dropped implement gates after "(no gate reported)", and the first Go version did not. Fixed. Replay re-validation added, then moved into `replayCall` (`986aed6`). **Deviation:** two parentheticals in `lsf-workflow.md::lifecycle-mapping` (outside Scope) were made true for the split. |
| 4 | `986aed6` | `checkSha` at four sites. The fake `claude` gains `shell`, `denials` and `failFirst`. Five real-runtime tests. Promptparity's `stage-fault` rewrite was dead (no scenario reaches a code-stage fault), so it is dropped, and the `gate-omitted` rewrite must be live. |

## Per-step critique findings

| Step | Critical | Significant | Disposition |
|---|---|---|---|
| 1 | 0 | 0 | Minors addressed in `d784d83`: record-kind test, edge tests, item cut to 200 runes, D20 description, graphparity D20 strength, empty-error statement, merge referent. |
| 2 | 0 | 2 | Both addressed (`5427976`): unchecked replay, and return-path tests. Minors addressed: renames on either column, vanished files, `--no-optional-locks`, isolated replay worktree check, known limit widened. **Noted for Phase 5:** no log when `stateDirIn` fails `EvalSymlinks`; case-insensitive filesystems (macOS). |
| 3a | 0 | 1 | Addressed: `TestUncommittedPerSite` covers every site. Minors: test comment fixed. **Noted:** a stopping fix still records `critical-fixed` rows (records before the stop check, consistent with rule 3). The cleanup paragraph of `::wave-loop` still says cleanup never signals; that is `code-merge-cleanup`'s. |
| 3b | 0 | 3 | Addressed: the contract stated, per-site violations, replay re-validation. **Noted:** code-step returns are not schema-checked (they are orchestrator-made); the validator's keyword subset (recorded with D21). |
| 3c | 0 | 3 | Addressed: the mixed case documented and tested; statements pinned; `stage-fault` rewrite removed and liveness required; `cache-invalid`. **Noted:** `skills/lsf-drive/SKILL.md` does not list the new kinds (outside Scope; for `runtime-cli-drive`). |
| 4 | 0 | 4 | Addressed: the denied-commit test reports HEAD as its SHA (the postmortem's shape; no SHA check can catch it) and is described as simulated from the evidence; the `checkSha` comment says the correction protects the ledger only. **Deferred, by plan:** merge SHAs are `code-merge-cleanup`'s. **Discovery:** an unresolvable branch at plan or implement is silent until the completion report stops on it. Minors addressed: the normalised SHA is returned; branch names with revision syntax are not resolved; tests for a SHA longer than the tip and for revision syntax. **Noted:** "not checked" shares the `sha-corrected` kind; the completion-report tip read treats every git error as unresolved; a replay re-spawn after a schema change can repeat a writer's commit. |

## Success criteria

| # | Status | Target → achieved | Evidence |
|---|---|---|---|
| S1 | pass | proven → proven | Every kind is listed and mapped (`TestGoOnlyKindsAreKinds`, `TestSignalsFromCall`). `CallResult` kinds are covered at unit and loop stages (`TestDriverEventsAtImplement`, `TestMergeCallStop`, `TestUncommittedPerSite`). `gate-omitted` is covered at implement and fix (`TestNoGateReportedStops`, `TestFixGateNotRun`). `stage-fault` at a code step (`TestCodeStageFaultsStop`, `TestLoadFaultsStop`). `sha-corrected` on the real runtime (`TestOnRuntimeShaCorrected`). |
| S2 | pass | proven → proven | `runtime/uncommitted_test.go`: isolated, repo before/after, before-state taken once, rename, state dir (inside the repo and through a symlink), replay re-check, unchecked journaling and replay, every return path. |
| S3 | pass | proven → proven | `TestOnRuntimeDeniedCommit`, `TestOnRuntimeDeniedBranchDelete`. |
| S4 | pass | proven → proven | `TestOnRuntimeTransientRetried`, with the 529 marker asserted. |
| S5 | pass | proven → proven | `TestGateNotRunNotRetried`. |
| S6 | pass, with deviation | proven → proven | Plan mappers: `TestFailClosedMappers`, graphparity D20. Orchestrator seed and `Violations`: `TestSchemaViolations`, `TestUncommittedPerSite/*/violation`. Live: `TestOnRuntimeLiveSchemaViolation`. Stubs are not re-validated (step 3b). |
| S7 | pass | proven → proven | `TestUncommittedPerSite` (16 sites × 2 events), `TestDriverEventsAtEverySiteKind`. |
| S8 | pass | proven → proven | `TestCheckSha`, `TestOnRuntimeShaCorrected`. |
| S9 | pass | proven → proven | `stageFaultSignal` kind; D19 amended; stage tests. |
| S10 | pass | proven → proven | `TestStepCtxEndsOnKill`. |
| S11 | pass | evidenced → evidenced | `lsf-workflow.md::escalation` amended in commits 3a, 3b, 3c and 4. D20 to D22 recorded, D19 amended. |
| S12 | pass | proven/evidenced → same | `go vet ./...` and `go test -race ./...` pass. Promptparity: 55 scenarios identical. Graphparity: 145 cases identical. `git diff --stat 85dced0 -- workflows/ lsf-runtime/go.mod` is empty. |

## Remaining phases
Complete Phase 5 (Review) as defined in the phronithm:feature skill.
