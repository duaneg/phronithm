# code-merge-cleanup — implementation summary

- **Working branch:** `lsf-workflow`. Base `196ddb3`.
- **Plan:** `docs/plans/code-merge-cleanup-plan.md` (`a0c0afd`).
- **External gate re-probe (Phase 4 entry):** `lsf-toolchain`. `go version` reports go1.27.1, and `lsf-runtime/go.mod` pins `toolchain go1.27.1`. The pin is confirmed, with no drift.

## Files modified

- **Runtime:** `lsf-runtime/runtime/{api.go, agent.go, journal.go, worktree.go, worktree_test.go}`.
- **Plan package:** `lsf-runtime/plan/{schema.go, signals.go, doc.go}` (a Scope deviation, by requester decision).
- **Orchestrator:**
  - new files: `merge.go`, `bookkeeping.go`, `cleanup.go`, `merge_test.go`, `bookkeeping_test.go`, `cleanup_test.go`, `branch_test.go`, `git_test.go`;
  - changed: `git.go`, `unit.go`, `wave.go`, `runtime.go`, `report.go`, `results.go`, `prompts.go`, `schemas.go`, `doc.go`, `orchtest/orchtest.go`, `run_test.go`, `runtime_test.go`, `signals_test.go`, `integration_test.go`, `integration_signals_test.go`.
- **Tools:** `lsf-runtime/tools/promptparity/{main.go, scenarios.go}`.
- **Docs:**
  - `docs/design/lsf-workflow.md`: `::wave-loop`, `::model-tiers`, and `::escalation` (a deviation);
  - `docs/design/lsf-runtime.md` (`::runtime`);
  - `CLAUDE.md` (a deviation).

## Per-step record

### Step 0: plan (`a0c0afd`)
- The plan critique ran two rounds.
- Round 1 raised one Critical: cleanup deleted the branches that replay reads. The fix was to merge the journaled completion-report SHA and delete only completion-report branches. It also raised 7 Significant and 10 Minor findings.
- Round 2 raised 0 Critical and 3 Significant findings, all folded in.

### Step 1: `CallResult.Branch`, `RemoveWorktree` (`b84a218`, fixes `9cadf79`)
- Decision: `Branch` is journaled in the call record, so a replay returns it unchanged under another launch's `planSha`.
- Deviation from the plan's `branchFor(label)`: the code uses `branchName(args, key)`.
- Critique: 0 Critical, 4 Significant, all addressed in `9cadf79`:
  - the stop boundary after the semaphore;
  - the event on partial removal;
  - `branch retained` in place of a misleading `absent`;
  - test gaps.
- The minors on the TOCTOU window and the sequence-wide timeout were documented.

### Step 2: `branch-mismatch` (`a19f691`, fixes `7448e35`)
- The new stop kind is in `plan` (requester decision).
- `checkReturn` holds the logic, and `callBranch` returns the runtime's branch.
- The completion-report tip read now separates exit 1 from other git errors.
- Surprises:
  - Every real-runtime fixture reported `work-A`. The fake `claude` gained `@branch`/`@head`.
  - The scripted-steps test drops the branch.
  - The transient test needed its fixture SHA kept, so the ledger exists.
- Deviation: `lsf-workflow.md::escalation` gained the stop bullet, under the design-of-record obligation.
- Critique: 0 Critical, 2 Significant, both addressed in `7448e35`:
  - the untrimmed reported branch was carried forward, so the runtime's branch is now carried;
  - a fix-code mismatch test was added.
- Minors addressed: the drift abort now comes after the branch check; D23 wording.
- Noted, not changed: `critical-fixed` rows precede a fix's mismatch stop (runtime-signals rule 3).

### Step 3: multi-path commits (`3242233`, fixes `24227a9`)
- Decision: a single-path commit keeps the bare `Lsf-Content` digest, and a multi-path commit carries `<digest> <path>` per path. `intact` reads both, so existing commits stay intact.
- The index is reset for every path the commit touched, a hook's included.
- Critique: 0 Critical, 2 Significant, both addressed in `24227a9`:
  - a partially unchanged set could land a commit that then fails verification; such a set is now refused up front;
  - a write failing mid-set now restores the files already written.
- Minors addressed:
  - input validation;
  - `GIT_LITERAL_PATHSPECS`;
  - the reset error is joined, not dropped;
  - a bare digest is scoped to its path;
  - D18 documents the trailer forms.
- The test file was committed unformatted and gofmt'd in `24227a9`.

### Step 4: bookkeeping render (`c39cc5b`, fixes `054f6e0`)
- The archive section keeps the existing headings. A missing Plan Maintenance section is an error; this was provisional, and the plan was approved with it.
- Critique: 0 Critical, 4 Significant, all addressed in `054f6e0`:
  - an indented closing fence;
  - the archive check was vacuous (`Contains` was true by construction); it is now an exact inserted-run check at any valid split;
  - setext and indented headings are refused rather than guessed at;
  - empty headings likewise.
- Minors addressed: CRLF, duplicates, backtick info strings, input validation, the `parsedUnits` name.

### Step 5: merge as a code step (`2f80c62`, fixes `3f7acc8`)
- Decisions mid-implementation:
  - **refusal versus fault:** only a "would be overwritten" refusal is `unresolvable`; any other failure to start is a fault (found writing the stale-lock test);
  - **hook-staged paths:** on git 2.54 a `pre-merge-commit` hook's `git add` does not reach the merge commit but leaves the path staged (probed). That is now a fault (`leftStaged`). The tree check stays for git versions where the change does reach the commit;
  - **seeded merge scenario:** the promptparity scenario seeded the merge, so the seed moved to obligations to keep the JS seed-pause branch covered;
  - **tests driving the merge agent:** retargeted per the done condition.
    - `TestMergeCallStop` now drives the step value's `Signals`. The writer cases are the re-plan's.
    - The schema-violation seed is the re-plan's.
    - The live schema violation is the obligations stage's.
- Critique: 0 Critical, 4 Significant, all addressed in `3f7acc8`:
  - the dry run now covers only the pending units, and bookkeeping skips units already recorded, which fixes the permanent fault after a partly recorded wave;
  - a hook-refused merge (`MERGE_HEAD`, no conflicts) is a fault;
  - non-conflicted staged paths must equal git's own merge-tree;
  - the checkout outside the merge's paths is snapshotted before the conflict call, and any change to it is a fault naming the paths.
- Minors addressed:
  - `merge-tree -z`;
  - the deletion check asks the index;
  - the `merge-base` error is reported as itself;
  - the dirt check runs before merging;
  - the duplicate `pre` read is removed.
- Noted in D24: a conflicted file's content is the call's judgement, and only its markers are checked.

### Step 6: cleanup as a code step (`393ad47`, fixes `7beb2ea`, branches `baf8450`)
- Surprise: a relaunch reran the cleanup step about one launch in four. The parallel reviewers record their worktree labels in either order, so the step's input differed. The labels are now sorted, and the test passes over 20 repetitions.
- `stepLabel` exposes a step's final label, so the completion-report worktree is noted outside the replayed fn.
- Tests retargeted: the reader residue case is now the obligations stage's, and the denied branch delete is the wave gate's.
- Critique: 0 Critical, 3 Significant, all addressed in `7beb2ea`:
  - the deleted-path list is logged as each removal happens, so a later failure no longer loses it;
  - seeded stages' worktrees are not cleaned up;
  - the missing tests were added.
- Minors addressed:
  - `branch retained` is filtered from the kept list;
  - units with no worktrees are left out of the input;
  - the Known limits wording.
- Kept literal: the "Not in this unit" line names `code-merge-cleanup` (the done condition's wording).
- **Deviation, requester decision (2026-10-07):**
  - *The decision.* The requester's baseline is that a unit's branches are deleted once it is complete, since a relaunch does not redo completed work. The done condition, and plan §2.6, kept every branch except the completion-report one.
  - *Why replay needed the branches.* A relaunch replays a completed unit from the journal, and that replay read the branches in two places: the writer check's tip read, and the completion report's base tip.
  - *How replay now works without them (`baf8450`).*
    - The writer tip read is a journaled step, `branch-tip:<call label>`, and its input includes the reported SHA.
    - The completion report reads its base tip live, and falls back to the journaled tip when the branch no longer resolves.
    - Cleanup deletes every merged unit's worktree branches, still only when they are merged.
  - The real-runtime test shows no `lsf/*` branch is left, and that a relaunch after cleanup replays everything. A code-fix round was added to that test in Phase 5.

### Phase 5
- **Lens review** (all lenses; `go vet` clean; staticcheck and golangci-lint not installed). It raised 0 Critical, 2 Significant and 7 Minor findings.
- **Full-diff critique.** It raised 0 Critical, 3 Significant and 6 Minor findings.
- **Addressed:**
  - **the stale branch-tip replay after a writer rerun** (raised by both). The tip step's input now includes the reported SHA, and only a resolved tip is journaled;
  - **merge refusal read under a non-English locale.** Code-stage git runs with `LC_ALL=C`;
  - **`outside()` reading whole files.** It now compares size and mtime past 64 MiB, and records an unreadable file as such;
  - **the branch-deletion test coverage.** A fix round was added to the real-runtime test, which needed `once` answers in the fake `claude` and the call counter to skip marker files;
  - **`checkMergeShas`** carries the resolved full SHAs forward;
  - errors propagated in `abortOwn` and `bookkeep`, the bookkeeping dirt check restructured, the `in` shadowing removed, comments fixed;
  - **docs:** the escalation table row, a Known limits entry for a cleanup failure not retried, a D25 note on replay re-checking no uncommitted paths after cleanup, D24 naming the journal record to remove, and D24/D25 rewrapped.
- **Noted, not changed:**
  - `mergeOne` is long (about 190 lines); splitting it was not done in this unit;
  - regexes are compiled per call in `ownMergeInProgress` and `maintenanceDate`, at negligible cost;
  - a unit ID containing `:` or `#` is mis-keyed by `noteWorktree`. This fails safe (its worktrees stay);
  - a mid-cleanup kill is untested at the orchestrator level. `RemoveWorktree` is idempotent (absent on a second call), which `TestRemoveWorktreeMerged` covers.

## Success criteria

| Criterion | Status | Target / achieved | Evidence |
|---|---|---|---|
| C1. Merge and cleanup run as code through the interface; no agent prompt for them | pass | proven / proven | `mergePrompt`, `cleanupPrompt` and their schemas are deleted. `TestOnRuntimeCodeStages` shows no merge or cleanup agent call, a step each. |
| C1. Truncation, a dropped fence and a rejected commit injected into the bookkeeping are never committed silently | pass | proven / proven | `TestMergeBookkeepingInjections` (truncation, fence, rejection, then the rerun) |
| C1. Every path of the bookkeeping commit is verified; a staging hook leaves the other path neither silently in the commit nor the index behind HEAD | pass | proven / proven | `TestCommitFilesTwoPaths`, `TestCommitFilesStagingHook`, `TestCommitFilesDeletingHook` |
| C1. A conflict goes to a model call; an unresolvable one is reported | pass | proven / proven | `TestMergeConflictResolved`, `TestMergeConflictNotAccepted` |
| C1. A returned SHA that does not resolve (`architectureSha`, `merges[].sha`) is rejected by `checkSha`'s rule; a clean merge without its SHA is not attributed to the wave | pass | proven / proven | `TestCheckMergeShas`. With a repository, an empty SHA is a stage-fault. Without one (the stub), the JS fallback text stays (D24). |
| C1. A stage's reported branch that does not resolve, or is not the runtime's, stops at its return; `checkSha` checks against the runtime's branch | pass | proven / proven | `TestBranchMismatchStops`, `TestCheckReturn`, `TestOnRuntimeBranchMismatch` |
| C1. The completion-report tip read separates exit 1 from other errors | pass | proven / proven | `TestCompletionTipReadErrors` |
| C1. A merge stop leaves no merge in progress and no merge-written path uncommitted | pass | proven / proven | `TestMergeConflictNotAccepted`, `TestMergeConflictCallCommits`, `TestMergeConflictCallOutside`, `TestMergeHookRefused` |
| C1. A kill between the merge step's commit and its record resumes with no second merge or bookkeeping commit | pass | proven / proven | `TestMergeResume` (5 cases), `TestMergeInterruptedEdges`, `TestMergeResumeExtraPath` |
| C1. Cleanup removes the merged worktrees and their completion-report branches with every ignored and untracked file, and lists the paths (and, by requester decision, every other branch of the merged unit) | pass | proven / proven | `TestRemoveWorktreeMerged`/`Edges`/`Keeps`, `TestCleanupStage`, `TestCleanupInputIsMergedUnitsWorktrees`, `TestOnRuntimeCodeStages` |
| C1. The Wave loop's cleanup paragraph is rewritten; `lsf-runtime.md`'s Not in this unit assigns the policy here and the move to `retained-artefacts-impl` | pass | proven / evidenced | doc diff (`393ad47`). Docs cannot be tested. |
| C1. `runtime-signals`' tests that drove merge or cleanup pass against a stage that is still an agent | pass | proven / proven | `TestDriverEventsAtEverySiteKind` (obligations), `TestSchemaViolations` (replan), `TestOnRuntimeLiveSchemaViolation` (obligations), `TestOnRuntimeDeniedBranchDelete` (review-gate) |
| C1. promptparity passes with merge and cleanup removed from the JS comparison; `stepParity` checks their inputs | pass | proven / proven | `PROMPTPARITY_REQUIRE_NODE=1 go test ./tools/promptparity` (55 scenarios) |
| C2. Frozen JS; `go.mod` | pass | proven / evidenced | `git diff --stat 196ddb3 -- workflows/ lsf-runtime/go.mod` is empty |
| C3. The full suite and parity | pass | proven / proven | `go test -race ./...` green; `go vet ./...` clean; graphparity and promptparity pass with node required |

## Remaining phases
Phase 5 review is done (above). The remaining Phase 5 items are the completion report and the continuation prompt.
