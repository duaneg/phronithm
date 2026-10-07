# code-merge-cleanup — completion report

Work unit `code-merge-cleanup` of `docs/plans/lsf-improvements-breakdown.md` @ `196ddb3`. Branch `lsf-workflow`. Plan: `docs/plans/code-merge-cleanup-plan.md`. Implementation summary: `docs/plans/code-merge-cleanup-implementation.md`.

## Done-condition verdict

**Met.** The achieved rigour is `proven` for every clause except the two documentation clauses, which are `evidenced` by the diff. The clause-by-clause evidence is the implementation summary's success-criteria table:
- `go test -race ./...` is green, and `go vet ./...` is clean;
- promptparity: 55 scenarios identical;
- graphparity passes;
- both parity checks were run with node required.

Two readings, taken with the requester:
- **A staging hook on the bookkeeping commit.** On git 2.54 the hook's path does land in the commit. Verification then stops the run, naming the commit, and the index is reset to HEAD for every path the commit touched. "Neither in the commit nor the index behind HEAD" is read as: not silently in the commit, and no index entry left behind HEAD. Requester decision 5, at plan time.
- **A clean merge without its SHA.** It is rejected only with a repository. Under the stub, which has no repository, the JS fallback text stays. The production runtime always has a repository.

## Deviations from the unit entry's scope and publishes

- **Outside Scope:**
  - `lsf-runtime/plan`: the `branch-mismatch` stop kind and `BranchMismatchSignal`. This was the requester's decision at plan time.
  - `docs/design/lsf-workflow.md::escalation`: the `branch-mismatch` bullet and table entry, made under the design-of-record obligation.
  - The `CLAUDE.md` `lsf-runtime` line.
- **Beyond the done condition (requester decision, 2026-10-07):** cleanup deletes every worktree branch of a merged unit, not only its completion-report branch, because a complete unit's branches serve no purpose. Replay was made branch-independent for this:
  - the writer check's tip read is a journaled `branch-tip:<call label>` step;
  - the completion report's base tip falls back to that journaled tip.
- **Publishes:**
  - merge and cleanup as code stages (`mergeStage`, `cleanupStage`, D24, D25);
  - `lsfrt.Runtime.RemoveWorktree` and `lsfrt.WorktreeRemoval`;
  - `CallResult.Branch`, journaled;
  - `gitOps.commitFiles`, `verifySet` and `adoptSet`, with the per-path `Lsf-Content` trailer form;
  - the `branch-mismatch` kind;
  - `merge-conflict:w<n>:<unit>`, a new agent call label.
- **Against the plan:** none beyond the branch deletion above. The plan's `branchFor(label)` is `branchName(args, key)`.

## Deferrals

- **Project gates after each merge → `declared-gates`.** This was the requester's decision at plan time. The code merge runs no gates, so a `regressed` merge cannot arise until then. `declared-gates`' done condition already runs declared gates after each merge.
- **The ignored-file `Retains:` move → `retained-artefacts-impl`.** As planned. Until that unit, cleanup keeps no ignored file.

## Discoveries affecting other units

- **`declared-gates`:**
  - The merge step is the place to run gates after each merge (`mergeOne`, after a clean or resolved merge commit is verified).
  - A regression undo must be compatible with adoption. An undone merge commit must not be adopted by an interrupted rerun, because adoption keys on the `Lsf-Unit`/`Lsf-Step` trailers.
- **`s2-permissions-design`:**
  - The new agent call `merge-conflict:w<n>:<unit>` runs in the main checkout and needs edit and `git add` there. It runs under the default role today.
  - The stage enforces its scope after the fact: it refuses changes outside the merge's paths, and aborts the merge.
- **`runtime-cli-drive`:**
  - New stop kind `branch-mismatch`; the drive's stop-kind list needs it.
  - Faults the requester may need to clear by hand:
    - a merge SHA that no longer resolves after a history rewrite (remove `journal/merge_w<n>-*.json`);
    - a stale `index.lock`;
    - a foreign merge in progress in the main checkout;
    - hook-staged paths after a merge.

    Each fault's statement names its remedy.
- **`retained-artefacts-impl`:**
  - Cleanup's removal is `RemoveWorktree`, which deletes everything untracked or ignored in the worktree and lists it.
  - The `Retains:` move belongs before that call, per worktree label.
  - Branches are now deleted too, so a retained path must be moved out before cleanup runs, not recovered from the branch later.
- **`script-retirement`:**
  - `lsf-workflow.md` still describes the Workflow script's merge and cleanup agents beside the Go behaviour; the wording marks each as the script's.
  - Stale Go comments referring to the merge agent are gone, but `orchtest`'s `Stub.Call` keeps the merge-prompt branch for the JS-era call shape (unused by Go).
- **Requesters with hooks:**
  - A `pre-merge-commit` hook that stages paths makes every merge stop, naming them.
  - A hook that rewrites or stages paths on the bookkeeping commit stops at verification, as D19 already does for the other code stages.
- **Branch accumulation:** resolved. Every branch of a merged unit is deleted. Branches of attempts that did not merge stay with their worktrees (Known limits).
- **Git version:** the merge needs `git merge-tree --write-tree` (git 2.38 or later), and its refusal detection assumes `LC_ALL=C`, which is now set. No launch-time version check exists; a later unit may want one.

## Standing obligations

- **Register dispositions:**
  - **P-C8, fabricated merge SHAs: disposed.** Every merge SHA, `planSha` and `architectureSha` is read from git by the stage. `checkMergeShas` rejects one that is not hex, does not resolve, or is not on the integration branch, live or replayed, and a clean merge without its SHA. The resolved full SHA is carried on.
  - **P-C9 (`branch -d` denied, `--force` removal allowed): disposed.** Cleanup is code, outside agent permissions, so neither the denial nor an agent's `--force` arises. The data-loss half (untracked series removed) is P-D5's, with `retained-artefacts-impl` and `empirical-unit-impl`. This unit's cleanup deletes and lists those files, by requester rule.
- **Design of record:** each section was amended in the commit that changed its behaviour:
  - `lsf-workflow.md::wave-loop`, the merge and archive paragraphs: `2f80c62`, `3f7acc8`;
  - the cleanup paragraph: `393ad47`, `7beb2ea`, `baf8450`;
  - `::model-tiers`: `2f80c62`, `393ad47`;
  - `::escalation`: `a19f691`, then the Phase 5 fixes;
  - `lsf-runtime.md::runtime`: `b84a218`, `393ad47`, `7beb2ea`, `baf8450`, and the Phase 5 fixes.
- **Frozen JavaScript:** `git diff --stat 196ddb3 -- workflows/` is empty.
- **`go.mod` stdlib-only:** unchanged.

## External-gate re-verification

`lsf-toolchain`: `go version` reports go1.27.1, and `lsf-runtime/go.mod` pins `toolchain go1.27.1`. The pin is confirmed, with no drift.

## Execution signals

- **Context:** comfortable at completion.
- **Mid-implementation design decisions** (not taken at a design pass):
  - a refusal over local changes is `unresolvable`, and any other failure to start a merge is a fault;
  - hook-staged paths after a merge are a fault (probed on git 2.54);
  - non-conflicted staged paths must equal git's merge-tree;
  - the checkout outside the merge is snapshotted around the conflict call;
  - bookkeeping skips units already recorded, and the dry run covers only pending units;
  - the cleanup input is sorted (found from a flaky relaunch);
  - seeded stages' worktrees are not cleaned up;
  - removed paths are logged as they happen;
  - every merged branch is deleted, with journaled branch tips (requester decision);
  - the tip step's input includes the reported SHA;
  - code-stage git runs with `LC_ALL=C`.

  Most came from critique findings rather than gaps in the design.
- **Critique load:**
  - Plan critique converged in round 2: round 1 raised 1 Critical, 7 Significant and 10 Minor findings; round 2 raised 0 Critical and 3 Significant.
  - Per-step gates on steps 1–6 raised 0 Critical and 19 Significant findings, all addressed.
  - Phase 5: the lens review raised 0 Critical and 2 Significant; the full-diff critique 0 Critical and 3 Significant. All were addressed except the notes in the implementation summary.
- **Size:** about 4,600 lines added (tests included) across 37 files. The flagged surface excess held: the excess was breadth of one pattern, not context.

## Next step

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
