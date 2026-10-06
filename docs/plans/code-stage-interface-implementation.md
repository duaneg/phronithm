# code-stage-interface — implementation summary

- **Branch:** `lsf-workflow`. Base `492b524`; plan commit `514655d`; implementation `6a58ac4..78c8ce3`.
- **Plan:** `docs/plans/code-stage-interface-plan.md`.

## External-gate re-probe (work-unit mode)

`lsf-toolchain`: `go version` reports go1.27.1, and `lsf-runtime/go.mod` pins `toolchain go1.27.1`. The pin is confirmed, with no drift. Git 2.54.0 is installed; the plan pins no git version.

## Files modified

- **Docs:** `CLAUDE.md`, `docs/design/lsf-runtime.md` (Interface), `docs/design/lsf-workflow.md` (Model tiers).
- **`lsf-runtime/orchestrator/`:** `args.go`, `doc.go`, `git.go` (new), `stages.go` (new), `loop.go`, `prompts.go`, `runtime.go`, `schemas.go`, `unit.go`, `wave.go`. Tests: `helpers_test.go`, `integration_test.go`, `run_test.go`, `runtime_test.go`, `stages_test.go` (new).
- **`lsf-runtime/orchestrator/orchtest/`:** `orchtest.go`, `orchtest_test.go`.
- **`lsf-runtime/runtime/`:** `agent.go`, `api.go`, `journal.go`, `journal_test.go`, `worktree.go`.
- **`lsf-runtime/tools/promptparity/`:** `main.go`.

## Per-step record

### Step 1: the seam (`6a58ac4`)

**Probe.** On git 2.54, a pre-commit hook that rewrites the file and runs `git add` changes the commit that `git commit -- path` makes. Every hook-shaped content fault therefore uses a real hook, and no decorator is involved. The probe also showed that such a hook leaves the real index behind HEAD (`MM`), so the stage resets the path's index entry after every commit.

**Decisions:**
- The plan's `orch.step` is named `runStep`, because `launch.step` is already the loop pass.
- Parallel call sites go through a new `parallelAgents`, which records each branch's `CallResult` in its slot.
- The label counter is shared through `orch.unique`.

### Step 2: git helper; load and decision log (`54652d5`)

**Decisions:**
- Stage functions receive the step's final label (`stageFn`).
- The decision-log closing fence is located with a copy of `plan`'s block regex, because `plan` is outside Scope. The parse-back check guards against the copy drifting.
- `orchtest.Stub.Call` has `Step`/`Input` fields, so steps stay in `Labels()` and order assertions are unchanged.

**Folded in from the step 1 critique:**
- An empty fault reason becomes `stage failed`.
- `lsfrt.Step` returns the sticky stop when `fn` fails during a stop.
- `IsStop` handles a nil `*StopError`.

### Step 3: ledger and completion report (`1819fed`)

**Decision.** A live pause-read error after a ledger commit marks the rows scribed and then stops. Otherwise the outcome would report committed rows as unscribed, and the caller would append them again.

**Folded in from the step 2 critique:**
- `writeAndCommit` takes the tip the stage rendered from, and refuses if the branch has moved.
- A commit that landed despite a git error is verified rather than restored over.
- Restore and reset errors are reported.
- The `Lsf-Content` trailer is checked.
- Status uses `--untracked-files=all`.
- git runs without `GIT_DIR`, `GIT_INDEX_FILE` or `GIT_WORK_TREE`.

### Step 4: the real runtime (`f06557d`)

As planned.

### Step 5: records (`4684c7c`)

D18 and D19 are recorded, and the package comment and the `CLAUDE.md` structure line are updated.

### Critique fixes (`78c8ce3`)

**Step 3 critique:**
- Adoption requires a single parent and a commit that changes only the path.
- An interrupted ledger rerun refuses to re-append when its block is at the tip but in no adoptable commit.
- The SHA check runs before rows are marked scribed.
- The in-step pause-read error is ignored.
- The base resolves as `refs/heads/<branch>`.

**Step 4 critique:** the relaunch assertions are stronger, and the fake rejects empty table entries.

### Surprises

- **Index after a hook.** A hook's `git add` leaves the real index behind HEAD, as the step 1 probe found.
- **Live pause read.** The live pause read needs ordering against the SHA check (critique, step 3).

### Deviations from the plan

- **Scope.** `docs/design/lsf-runtime.md` changed, as planned (§2.9), and `CLAUDE.md` changed. Both are outside the unit's Scope.
- **`plan` regex.** `plan` was not touched; its block regex is copied instead (see Step 2 decisions).
- **D17 check.** It remains for scripted values. With a real completion report, a bad base surfaces as a stage fault.

## Per-step critique findings

The gate ran `phronithm:critic` (sonnet, critique-code) once per step on steps 1–4. No step had a Critical finding. Step 5 changed only docs, and its content is covered by the Phase 5 full-diff review.

| Step | Severity | Finding | Disposition |
|---|---|---|---|
| 1 | Significant | `stageErr` with an empty reason looks like a null; a runtime stop that ends `fn` reads as a fault | Addressed (step 2) |
| 1 | Significant | `TestCallResultReachesCallSites` proves the helpers, not consumption at the sites | Noted for Phase 5. No site consumes the result yet; `runtime-signals` will |
| 1 | Minor | Typed-nil `StopError`; `Returns` comment; denials from the last attempt only; stale `Agent` text | Addressed (step 2: comment wording, `IsStop`) |
| 1 | Minor | Test gaps: concurrent `parallelAgents`, wrapped stop; `Worktree` compared with an agent call's | Wrapped stop and empty reason added. Others noted for Phase 5 |
| 2 | Significant | Restore after a landed commit; untracked draft missed; hook staging another path | Addressed (step 3). The hook case is tested |
| 2 | Minor | Stale prior (TOCTOU); reset error dropped; trailer not checked; environment; regex duplication; test gaps (detached HEAD, unborn branch, merge in progress, decision-log replay) | Addressed: TOCTOU, reset, trailer, environment. Regex duplication accepted (parse-back). Test gaps noted for Phase 5 |
| 3 | Significant | Relaunch after a landed-but-failed commit re-appends or adopts a commit that failed verification | Addressed (`78c8ce3`) and tested |
| 3 | Significant | Live read marks rows scribed before the SHA check; in-step pause-read error | Addressed (`78c8ce3`) |
| 3 | Significant | Completion-report concurrency, unresolvable base and dirty worktree untested | Noted for Phase 5 |
| 3 | Minor | Shell-out environment in `lsfrt`; hook-timeout process group; post-commit error swallowed; stale `lsf-workflow.md` line 212 (outside Scope) | Noted for Phase 5 / completion report |
| 4 | Significant | Ledger and report content not asserted exactly; trimmed `git` helper | Noted for Phase 5. The stage itself verifies byte equality with the rendering |
| 4 | Significant | Replay evidence thin; refs compared only under `refs/heads/` | Addressed (`78c8ce3`) |
| 4 | Minor | Contains-table robustness, date at midnight, branch lookup | Empty entries rejected. Others noted |

## Success criteria

| # | Status | Target | Achieved | Evidence |
|---|---|---|---|---|
| S1 | pass | proven | proven | The four prompt builders and schemas are deleted. `TestCodeStagesMakeNoAgentCall` (orchtest) and `TestOnRuntimeCodeStages` (telemetry) show no agent call for the four prefixes |
| S2 | pass | proven | proven | See S2 detail below |
| S3 | pass | proven | proven | `verifyCommit` checks the five conditions; `TestVerifyCommitChecks` makes each fail and includes a root commit. A fabricated `rev-parse` is rejected for the decision log, the ledger and the completion report. Adopted and dedup SHAs go through `checkReachable` |
| S4 | pass | proven | proven (helpers) | See S4 detail below |
| S5 | pass | proven | proven | See S5 detail below |
| S6 | pass | proven | proven | `TestDecisionLogMainCheckoutSafety` (off-branch, dirty path, staged and unstaged changes survive); `TestLedgerRefusals`; `TestMainCheckoutEdgeCases` (hook staging another path; untracked draft under `showUntrackedFiles=no`) |
| S7 | pass | proven | proven | `go run ./tools/promptparity`: 55 identical. Exclusion via `codeStep`. Coverage counts calls and steps. `unreachable` unchanged |
| S8 | pass | proven | proven | `TestOnRuntimeLabelsUniqueAndReplay` (steps scripted through `rt.Step`); `TestOnRuntimeCodeStages` (real git: complete, trailers, no agent call; relaunch with no spawn, no ref moved, every call and step replayed) |
| S9 | pass | evidenced | evidenced | D18 and D19 in `doc.go`. Model tiers amended in `54652d5` (load, decision log) and `1819fed` (all four) |
| S10 | pass | proven / evidenced | proven / evidenced | `go vet ./...` and `go test -race ./...` pass. `git diff --stat 492b524 -- workflows/ lsf-runtime/go.mod` is empty |

**S2 detail.** Each fault is tested through the stage function against a temporary git repository:
- **Load:** a placeholder and a truncated block each end `invalid-plan`; a clean read is byte-equal to the blob (`TestLoadFaultsStop`, `TestLoadReadsBlobsVerbatim`).
- **Decision log:** an invented clears, a dropped fence, a dropped line and shortened text each stop, naming the commit. The guard holds on relaunch, including after an amend; a repair commit proceeds (`TestDecisionLogContentFaultsStop`).
- **Ledger:** an overwritten row, a deleted row, its own rows altered, a dropped heading and a deleted section (`TestLedgerContentFaultsStop`).
- **Completion report:** a dropped line and an altered phrase (`TestCompletionReport`).
- **Rejected commits:** tested for every committing stage.

**S4 detail.** `TestCallResultReachesCallSites` covers the plain, null and parallel paths through `call[T]` and `parallelAgents`; `TestDenialsSurviveReplay` covers the journal. No call site consumes the result in this unit. The upgrade path is the `runtime-signals` unit's tests at the sites.

**S5 detail:**
- `TestLedgerInterruptedAdopts` runs on the real runtime: kill-after-commit, an intent is left, the interrupted rerun adopts with no second commit, a requester commit lands after it, two scribes share a wave, and an earlier launch's same-label commit is not adopted.
- `TestDecisionLogInterruptedAdopts` covers midnight and a requester commit.
- Completion-report reuse is covered in `TestCompletionReport`.
- `TestLoadLiveReadsOverrideReplay` covers HEAD fixed and pause cleared.
- `TestLedgerLivePause` covers a replayed ledger step.

## Remaining phases

Complete Phase 5 (Review) as defined in the phronithm:feature skill.
