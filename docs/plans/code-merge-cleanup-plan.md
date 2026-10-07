# code-merge-cleanup — plan

Work unit `code-merge-cleanup` of `docs/plans/lsf-improvements-breakdown.md` @ `196ddb3`. Base commit `196ddb3`, branch `lsf-workflow`. Phases 1–3 ran compressed in one plan-mode session, so this document is the problem statement, the design and the implementation plan. Phase 4 step 0 saves it as `docs/plans/code-merge-cleanup-plan.md`.

## 1. Problem statement

### Context

Merge and cleanup are the two loop stages still run as agents in the Go orchestrator (`orchestrator/wave.go`: `mergePrompt`, `cleanupPrompt`). The merge agent reports whatever SHAs and branches it likes (P-C8, fabricated merge SHAs). The bookkeeping commit it makes (moving the entry to the archive, adding a stub, a Plan Maintenance entry, and `complete: true`) is unverified. The cleanup agent's permissions denied `git branch -d` but allowed `git worktree remove --force` (P-C9). It also matches JS-era `worktree-wf_*` branches, which the runtime never creates. The runtime's worktrees are `<state>/wt/<key>` on `lsf/<plan8>/<key>`.

Deferrals into this unit:
- From `code-stage-interface`: multi-path verify, index reset under a staging hook, completion-report branch cleanup, `stepParity`.
- From `runtime-signals`: merge SHAs checked by `checkSha`'s rule, the Wave loop cleanup paragraph, and the completion-report tip read separating exit 1.

The 2026-10-07 re-plans add: cleanup deletes every ignored and untracked file and lists them; a reported branch that is not the runtime's stops at the stage's return.

### Unit entry, verbatim

```
### code-merge-cleanup
- **Type**: refactor
- **Risk**: high — a merge done in code commits to the integration branch; a resume fault merges twice
- **Clarity**: specified
- **Iteration**: write-once
- **Effort**: large
- **Scope**: `lsf-runtime/orchestrator`, `lsf-runtime/runtime`, `lsf-runtime/tools/promptparity`, `docs/design/lsf-workflow.md::model-tiers`, `docs/design/lsf-workflow.md::wave-loop`, `docs/design/lsf-runtime.md::runtime`
- **Publishes**: merge and cleanup as code stages
- **Depends on**: code-stage-interface (logical, the code-stage interface); runtime-signals (logical, serialises the runtime and orchestrator files); arch-pin-refresh (knowledge, the merge stage's `architectureSha` and `merges[].sha`); retained-artefacts-design (design, cleanup deletes every undeclared ignored and untracked file)
- **External gates**: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`)
- **Done condition**: Merge with its plan bookkeeping, and cleanup, run as code through the interface; the ported orchestrator has no agent prompt for them. Tests inject truncation, a dropped fence and a rejected commit into the bookkeeping and show none is committed silently. Every path of the bookkeeping commit (plan and archive) is verified as `verifyCommit` verifies one, and a test shows a pre-commit hook that stages another path leaves neither in the commit nor the index behind HEAD. A merge conflict goes to a model call and an unresolvable one is reported. A returned SHA that does not resolve, including `architectureSha` and each `merges[].sha`, is rejected by `checkSha`'s rule (prefix, normalised, revision syntax rejected), and a clean merge without its SHA is not attributed to the whole wave. A stage's reported branch that does not resolve, or is not the worktree branch the runtime created for the call, stops the run at that stage's return, and `checkSha` checks against the runtime's branch (requester decision, 2026-10-07). The completion-report tip read separates an unresolved branch (exit 1) from other git errors, as `checkSha` does. A merge stop leaves the integration checkout with no merge in progress and no path the merge wrote uncommitted. A kill between the merge step's commit and its record resumes with no second merge or bookkeeping commit. Cleanup removes the runtime's merged worktrees (`<state>/wt/<key>` on `lsf/<plan8>/<key>`) and their completion-report branches, with every ignored and untracked file in them, and lists the removed paths in its report. The Wave loop's cleanup paragraph no longer says a failure leaves residue, not risk, and `lsf-runtime.md`'s Not in this unit assigns the ignored-file policy to this unit and the `Retains:` move to `retained-artefacts-impl`. `runtime-signals`' tests that drove the merge or cleanup prompt pass against a stage that is still an agent. `tools/promptparity` passes with the merge and cleanup labels also removed from the JavaScript comparison, and `stepParity` checks their step inputs against the JavaScript prompts. The tests pass.
- **Description**: The merge and cleanup half of `mechanical-stage-integrity`'s replacement. The merge stage reports whatever branch the agent names; unit-stage branches are `lsf/<plan8>/<journal key>` with a hash suffix, and D17 aborts a unit whose completion report lacks a branch. The merge receives the completion-report branch, `lsf/<plan8>/<key of completion-report:<unit>>`, one commit on the unit's work branch. Build on `gitOps` in `orchestrator/git.go` (`writeAndCommit`, `verifyCommit`, `guard`, `adopt`, `checkReachable`); `writeAndCommit` resets the path's index entry after each commit because a pre-commit hook's `git add` reaches a `git commit -- path` commit on git 2.54. Cleanup as code runs outside the agents' permissions, so P-C5's denial and P-C9's `--force` removal stop arising. A code-stage fault is `stage-fault`; the agent cleanup's `residue-at-return` and denial rows have no code counterpart, and the removed-path list takes their place. A merge stop continues to the re-plan, whose commit is guarded against merge residue only by `ownFilesOnly`. No unit can declare `Retains:` until `retained-artefacts-impl`, so this unit's cleanup keeps nothing; the Workflow script stays the supported path until `script-retirement`, which follows that unit. **Flag:** four code surfaces exceed the surface bound, as for `code-stage-interface`, whose interface it extends; the two design sections are a paragraph and a line.
```

Standing obligations carried by this unit:
- **Register dispositions:** P-C8 (fabricated merge SHAs) and P-C9.
- **Design of record:** amend `lsf-workflow.md::wave-loop`, `::model-tiers` and `lsf-runtime.md::runtime` in the commit that changes the behaviour each describes.
- **Frozen JavaScript:** `workflows/` is not edited.
- **`go.mod`:** stays stdlib-only.

No paid run is planned, and the unit has no `claude-code-headless` gate. `tools/fixturerun` is not run.

### Decisions confirmed with the requester (2026-10-07)

1. **No gates at merge until `declared-gates`.** The code merge runs no project gates, so `regressed` cannot arise. This is a deferral to `declared-gates`, whose done condition already runs declared gates after each merge.
2. **New stop kind `branch-mismatch` in `plan`.** It covers a reported branch that is not the runtime's worktree branch for the call, or that does not resolve. It is added to `StopKinds` and `GoOnlyKinds`. This is a Scope deviation into `lsf-runtime/plan`.
3. **Cleanup's removed paths go in the journaled step value and the run log.** No ledger rows, and no `plan` change for this.
4. **On a conflict, the agent edits and stages; code commits.** The model call resolves mechanically and stages, or reports `unresolvable`. Code checks for unmerged paths and conflict markers, then commits with trailers. Anything else gets `git merge --abort` and a clean-checkout check.
5. **A staging hook on the bookkeeping commit is caught, and the index is reset.** The commit lands with the hook's path in it, verification stops naming it (D19), and the index is reset to HEAD for every path the commit touched, the hook's path included. "Neither in the commit nor the index behind HEAD" is read as: not silently in the commit, and no index entry left behind HEAD.

### Success criteria (target rigour: `proven` for all; the rung definitions are in the phronithm:feature skill, Phase 1 step 2)

- C1. The unit's done condition, verbatim above, clause by clause. Each clause maps to the tests named in §3.
- C2. Standing obligations:
  - the register dispositions are recorded in the completion report;
  - the design of record is amended in each behaviour's commit;
  - `git diff --stat 196ddb3 -- workflows/` is empty;
  - `go.mod` is unchanged.
- C3. `cd lsf-runtime && go test -race ./...` passes, `go vet ./...` is clean, and promptparity and graphparity pass with node present (`PROMPTPARITY_REQUIRE_NODE=1 GRAPHPARITY_REQUIRE_NODE=1`).

### Exploration findings (key code)

- `orchestrator/git.go`:
  - `gitOps.writeAndCommit` is single-path only. It refuses a moved branch, restores the file on a rejected commit, verifies the commit (`verifyCommit`, which checks exactly one path), checks the `Lsf-Content` trailer (`intact`, which reads the last trailer value only) and resets that path's index entry.
  - `adopt`/`findAdoptable` require a commit changing exactly one path.
  - `checkReachable` and `guard` are also here.
- `orchestrator/stages.go`:
  - the code stages, including `mainPrecheck` (HEAD on the integration branch, path clean or the stage's own write, then the guard), `completionReport` and `stageFaultSignal`;
  - `liveReads` re-reads the pause ref after a step.
- `orchestrator/runtime.go`: the `Runtime` interface (`Call`, `Parallel`, `Step`, `Repo`, `Worktree`, `Log`, `Args`), `o.agent` (seeds; the `#n` unique labels), `runStep`, `step[T]`, `call[T]`, `callSigs`, and `parallelAgents`.
- `orchestrator/wave.go`:
  - the merge call: `mergePrompt`, `mergeResult`, then `SignalsFromMerge`, `completed`, `np.Sha = PlanSha` and `refreshPin(ArchitectureSha)`;
  - the gate and cleanup run in `parallelAgents`. Cleanup's null is logged.
- `orchestrator/unit.go`:
  - `runUnit`. Plan, fix-plan, implement, critique-code, lens-review and fix-code run with `Isolation: "worktree"`.
  - `checkSha` / `correctSha` check against the *reported* branch.
  - The completion-report tip read (about line 452) treats every git error as "does not resolve".
- `runtime/worktree.go`: `cwdFor` creates `<state>/wt/<journalKey(label)>` on `lsf/<planSha[:8]>/<refSafe(key)>` from the integration branch, under `wtSem`, with `gitContext`. `Worktree(label)` exposes it for code steps.
- `runtime/journal.go`, `Step`: an intent file, then fn(ctx, interrupted); the record replaces the intent; a replay skips fn. `Call` replays from the journal by label and hash, and `CallResult` (in `api.go`) has no branch.
- `orchestrator/orchtest`: `Stub.Step` answers steps from the prefix handler and never runs fn. `Stub.Call` derives `State.MergeOrder` from the merge prompt's `- <id>: branch` lines.
- `tools/promptparity`: `codeStep(label)` decides which JS calls are code steps on the Go side; `stepParity` checks each Go step's input fields for verbatim presence in the JS prompt with the same label. The JS labels are `merge:w<n>` and `cleanup:w<n>`.
- The fake `claude` (`orchestrator/integration_test.go`, `TestMain`/`fakeClaude`) answers from a substring table. An entry may run `shell` in the call's cwd. Scripted returns name `work-A` as the branch.
- `plan/schema.go`: `StopKinds`, `GoOnlyKinds`, `IsStopKind`. `plan.Signal` round-trips through JSON.

### Candidate approaches

- **A. One journaled step per wave, `merge:w<n>`: every merge plus the bookkeeping commit, with the conflict model call made from inside the step's fn.** Its label and value shape match the JS call, so `stepParity`, the stub and the existing tests carry over. Resume works by adopting the commits by trailer. The risk is a call nested inside a step: its driver events must travel in the step's value.
- **B. A step per unit merge (`merge:w<n>:<unit>`), then a bookkeeping step, with the conflict call made between steps in loop code.** Each step is smaller, and no call is nested. But the JS has one call, so promptparity would have to assemble one answer from several steps. The stub's merge order and every test scripting `merge:w1` would change, and a conflict leaves a merge in progress between steps, which a kill exposes.
- **C. Keep the merge an agent, and verify its return in code.** It fails the done condition ("no agent prompt for them").

## 2. Design (approach A)

### 2.1 Runtime (`lsf-runtime/runtime`)

- **`CallResult.Branch string`:** for an isolated call, the worktree branch the runtime gives the label. It is computed by a new `rt.branchFor(label)`, shared with `cwdFor`, and `""` without isolation. It is journaled in the call record (`record.Branch`), and a replay returns the journaled value, so another launch's `planSha` cannot change it.
- **`RemoveWorktree(ctx, label, integrationBranch string, deleteBranch bool) (WorktreeRemoval, error)`:**
  - `WorktreeRemoval{Label, Path, Branch string; Removed bool; Paths []string; Kept string}`.
  - It runs under `wtSem` and `gitContext`.
  - Path `<state>/wt/<key>`, branch `branchFor(label)`.
  - Neither exists: `{}` with `Kept: "absent"`.
  - The directory is not a worktree on that branch (cwdFor's check): kept, with the reason.
  - The branch tip is not an ancestor of `refs/heads/<integration>`: kept, "not merged".
  - Otherwise:
    - list the paths that will go: `git ls-files -z --others --directory` (untracked and ignored, a wholly untracked directory as one `dir/` entry) plus `git diff -z --name-only HEAD` (tracked changes);
    - run `git worktree remove --force <path>`;
    - with `deleteBranch`, run `git update-ref -d refs/heads/<branch> <tip>` (compare-and-swap).
  - With `deleteBranch`, a branch whose directory is gone (but still merged) is deleted with no paths.
  - A directory present on a branch whose ref is gone is kept ("branch missing"). The ancestor check is vacuous for plan and reader branches, which carry no commits of their own; those are protected by the label scoping in §2.6, not by the check.
  - Tracked uncommitted changes are deleted too. This is a decision: a merged unit's writer stages returned clean or the run would have stopped, so what remains is reader residue already recorded at its call. The ancestor check is about commits; it does not protect uncommitted tracked work, and is not claimed to.
  - A git failure is an error, not a stop. Telemetry event `worktree-removed`.
- **Tests (`runtime/worktree_test.go`):**
  - a merged worktree holding a tracked change, an untracked file and an ignored file is removed, its branch deleted, and all three paths listed;
  - an unmerged one is kept;
  - a foreign directory is kept;
  - a second call reports absent;
  - `CallResult.Branch` is set on a live isolated call and on its replay, and empty without isolation.

### 2.2 `branch-mismatch` and the branch check (`plan`, `orchestrator/unit.go`)

- **`plan`:** add `branch-mismatch` to `StopKinds` and `GoOnlyKinds`, plus `plan.BranchMismatchSignal(at, reported, runtime, why)`. Check that graphparity's `goOnly` handles it.
- **`call[T]`** also returns the call's `CallResult.Branch`; `fixCall` passes it through.
- At each writer unit stage (plan, implement, fix-plan, fix-code), when `rtBranch != ""`:
  - a reported branch ≠ `rtBranch` is a `branch-mismatch` stop, and the unit aborts at that stage;
  - otherwise `checkSha` runs against `rtBranch`. `rtBranch` is the branch carried forward, and so the completion report's base.
  - `checkSha`'s exit-1 case (the branch does not resolve) becomes a `branch-mismatch` stop when `rtBranch != ""`. Today it is silent.
- With `rtBranch == ""` (the stub, no repository, or a seeded stage, which has no call), behaviour is unchanged. promptparity has no branches, so it is unaffected.
- **"Stops the run at that stage's return"** means what every unit-stage stop already means: the stop is raised at that stage's return and the unit aborts there; sibling units in the wave finish, and the wave's stop rule then stops the run. Today an unresolvable branch is silent until the completion report.
- **Exit 1 after a matching branch** covers a branch deleted between the call and the check.
- **Fix stages.** `fixCall` raises the mismatch among the call's signals. `critiqueLoop` returns `call-stop` on a stop in `fixSigs` before `onFix` runs (`unit.go:122`), so `branch` is never updated to a mismatched one.
- **Older journal records** have no `Branch`, so their replays skip the check. D23 says so.
- **Ledger order.** At a mismatched fix, `critical-fixed` rows are already appended before the stop (`unit.go:120`). This is left as it is and noted in D23, consistent with runtime-signals' rule 3.
- **Not checked:** the readers (critique-code, lens-review) report no branch; the main-checkout stages (re-plan, termination, direct edits, obligations) have no `rtBranch`. D23 says so.
- **Completion-report tip read:**
  - exit 1 gives `stage-fault` "unit branch %q does not resolve";
  - any other git error gives `stage-fault` "unit branch %q: <err>";
  - a cancelled ctx stays a stop.
- **`orchestrator/doc.go` D23:** the branch check.

### 2.3 Multi-path commits (`orchestrator/git.go`)

- **`commitFiles(ctx, dir, branch string, files []fileWrite{Path string; Want []byte}, subject, label, pre string) (string, error)`** generalises `writeAndCommit`, which becomes a one-file wrapper (same behaviour, same messages for one path).
  - **Trailers.** A single file keeps `Lsf-Content: sha256:<hex>`. Multiple files carry one `Lsf-Content: sha256:<hex> <path>` per file. `intact(sha, path)` reads all values (a new `trailers()`, returning every value): a bare value covers a single-path commit; a `<digest> <path>` value covers its path. Old commits stay intact.
  - **Commit.** `git add -- <paths>`, then `git commit -q -m msg -- <paths>`.
  - **Index reset.** Afterwards, `git reset -q -- <paths ∪ the commit's changed paths>`. A hook-staged path's entry then matches HEAD, not behind it.
  - **Reject.** Every file is restored (removed if new).
  - **Verification.**
    - `verifySet`: one parent `pre`; changed paths exactly the set; tip of the branch; each blob byte-equal. `verifyCommit` becomes the one-path call.
    - Then each path is `intact`.
- **`adoptSet`:**
  - It finds the newest first-parent commit on the branch touching the first path. That commit must:
    - be intact for every path;
    - carry `Lsf-Step == label`;
    - have one parent;
    - change exactly the set;
    - satisfy `matches(parentBlobs, candBlobs)`.
  - `findAdoptable` keeps its single-path behaviour, built on `adoptSet`.
- **Tests (`stages_test.go` / a new `git_test` section):**
  - a two-path commit verifies, and both paths are intact;
  - `guard` on either path passes, and fails once one path is altered by a later trailer-bearing commit;
  - reject restores both files;
  - a staging hook (`git add wip.md`) gives a verification stop naming the commit, and afterwards `git diff --cached --quiet` holds (the index matches HEAD for every path, `wip.md` included);
  - a single-path commit's trailer is unchanged, and the existing tests pass.

### 2.4 Bookkeeping rendering (`orchestrator/bookkeeping.go`, pure)

**`renderBookkeeping(planText []byte, archive []byte, archiveOK bool, in bookIn, merged []bookUnit{Unit, Sha, Report}, date string) (plan, archive []byte, err error)`.**

Rendering:
- **Line scanner.** Headings are found outside fenced blocks (fences are lines starting with three or more backticks, as `plan.ExtractBlock`'s pattern).
- **Entry.** It is the `### <id>` line (exact id; trailing whitespace allowed) within `## Work Units`, through the line before the next `#`/`##`/`###` heading or EOF. A missing entry is an error: "unit <id> has no `### <id>` entry under `## Work Units`".
- **Stub.** The entry is replaced by `### <id> — complete at <sha>; entry archived to <archivePath>; report <report>\n` followed by one blank line.
- **Plan Maintenance.** `### <date> — <id> complete\nReport: <report> @ <sha>. Ledger: <ledgerPath> wave <n>.\n` is appended at the end of `## Plan Maintenance` (before the next `## ` heading, or EOF), one entry per unit in merge order. A missing section is an error, the same as a missing entry, because breakdown's template always has one. **This is a provisional choice for the requester to confirm at plan approval.**
- **Flag.** In the `yaml lsf-plan` block, the unit item is found by its `id:` key at the item's key indent. An existing `complete:` line has its value replaced with `true`. With no `complete:` line, `complete: true` is inserted after the item's first line.
- **Archive.**
  - A missing file becomes `# Archive: <name>\n\n## Work Units\n\n` plus the entries.
  - An existing one gets the entries appended at the end of its `## Work Units` section.
  - With no such section, `\n## Work Units\n\n` plus the entries is appended at EOF (the re-plan may have created the archive with only `## Closed risks`).

Checks before returning (any failure is an error, so nothing is committed):
- the new plan's block extracts and parses;
- `ParsePlanGraph(new)` equals `ParsePlanGraph(old)` with exactly the merged units' `Complete` set to true, compared through `plan.MarshalJS`;
- each moved entry appears verbatim in the new archive;
- deleting the inserted text from the new archive gives back the old archive.

**Tests (`bookkeeping_test.go`):**
- two units, with and without an existing `complete:` line;
- an archive that is absent, one with `## Work Units`, and one with only `## Closed risks`;
- an entry containing a fenced block with `## ` lines inside it;
- a missing entry; a missing Plan Maintenance section;
- a block where flipping the flag would break the parse (injected), which is refused;
- the stub and maintenance text match the JS prompt's forms.

### 2.5 The merge step (`orchestrator/merge.go`, `wave.go`)

**Input** (`mergeInput`, journal hash):
- `wave`, `name`, `integrationBranch`, `planPath`, `archivePath`, `ledgerPath`, `architectureDoc`, `pauseRef`;
- `units: [{unit, branch, sha, report}]` in plan merge order. `unitResult` gains `reportSha`, set from the completion-report step's value beside `branch` (`unit.go:474`). `sha` is the completion-report step's returned commit: a journaled value, not a live read. Nothing in the input needs a branch to resolve, so a resume replays after cleanup has deleted the completion-report branches (critique C1). A completion report that reruns, because its unit branch moved, returns a new `sha`, so the merge input changes and the merge reruns.

The stage merges `sha`, not the branch name. Inside fn, when the branch still resolves, its tip must equal `sha`, else it is a fault. The date is today's, taken inside fn and kept out of the input, as the decision log does.

**Value:** the agent-return shape `{result: mergeResult, commits: []}`. `mergeResult` gains `Signals []*plan.Signal` (`json:"signals,omitempty"`, Go-only), carrying the conflict calls' driver-event signals so that a replay keeps them.

**Commit message.** `merge(<name>): <unit>` followed by the trailers `Lsf-Step: <label>`, `Lsf-Unit: <unit>`, `Lsf-Merge: clean|resolved` and one `Lsf-Conflict: <path>` per conflicted path.

**fn(ctx, label, interrupted), in the main checkout:**
1. **Precheck**, in this order:
   1. HEAD is on `refs/heads/<integration>`.
   2. `MERGE_HEAD` is checked first; both it and `MERGE_MSG` are located with `git rev-parse --git-path`. A merge in progress is the stage's own only when `MERGE_MSG` carries this step's `Lsf-Step: <label>` and the `Lsf-Unit` of a pending unit whose `sha` equals `MERGE_HEAD`, *and* the run is interrupted. Then it resumes that unit's conflict (step 2.3), skipping the units before it, which are adopted. Any other merge in progress is a fault ("a merge is in progress in the main checkout that this stage did not start"), and is never aborted. `interrupted` is set after any fn error, not only after a kill (critique S3).
   3. With no merge in progress, the index must equal HEAD except, on an interrupted rerun, for staged plan and archive entries (a kill inside the bookkeeping commit). Those are judged in step 3, once the merges are known. Any other staged change is a fault.
   4. Plan and archive are clean (`pathClean`). On an interrupted rerun, dirt on them is deferred to step 3, as above.
   5. `guard` runs on both paths.
   6. **Dry run**, on a run that is not interrupted. `renderBookkeeping` runs over every unit with placeholder SHAs. A missing entry, a missing Plan Maintenance section or a block that will not round-trip is a fault *before* the first merge, so nothing lands on the integration branch (critique S1). An interrupted rerun first adopts (step 2.1, all units, then step 3's `adoptSet`). It dry-runs only the units it has not adopted, and only when no bookkeeping commit was adopted, so an already-moved entry is not mistaken for a missing one (critique round 2, N1).
2. **Per unit, in order.**
   1. *Adopt.* Walk at most `adoptionWalk` first-parent commits from HEAD for two-parent commits carrying `Lsf-Unit: <unit>`.
      - One with parent 2 = `sha` and `Lsf-Step: <label>`: verify it again, exactly as when it was made (tree equal to `merge-tree` for `clean`; changed paths equal to the derived staged set for `resolved`), then adopt it, with status from `Lsf-Merge` and files from the `Lsf-Conflict` trailers. A commit that failed verification stays a fault naming it; a relaunch never launders it into a clean merge (critique round 2, N2).
      - One with another parent 2 or label: a fault ("unit <unit> was already merged at <commit>"), so a relaunch over a changed branch never merges a unit twice (critique M5).
      - None, but `sha` is an ancestor of HEAD: a fault ("<sha> is already in <integration> outside this stage").
   2. *Merge.* `git merge --no-ff --no-edit -m <message, Lsf-Merge: clean> <sha>`.
      - **Success.** Verify the new HEAD: parents exactly `[pre, sha]`; trailers present; its tree equals `git merge-tree --write-tree pre sha`, so a hook's change to a clean merge is caught (critique M6; git 2.54 is installed, and `--write-tree` needs 2.38 or later). A failure is a fault naming the commit, which stays, as D19 does. Status `clean`, files `[]`.
      - **Refused with no merge started** (no `MERGE_HEAD`, HEAD unchanged; for example, local changes would be overwritten): `unresolvable`, with the files parsed from git's tab-indented stderr lines (else `[]`) and the statement carried in the log. The requester's changes are left as they are.
      - **Conflict** (`MERGE_HEAD` set). The conflicted paths and the staged set are derived purely from `(pre, sha)`, so a resume recomputes the same sets however much the agent had staged (critique round 2, N3):
        - the conflicted paths are the file names from `git merge-tree --write-tree --name-only pre sha`;
        - the staged set is those paths plus `git diff --name-only pre <its tree>`.

        The untracked set is recorded before the call. On a resume it is the live set, a known weakness: an untracked file the agent made before the kill is taken in. Go to step 3.
   3. *Conflict.* A model call, label `merge-conflict:w<n>:<unit>`, the default tier, phase Merge, `conflictSchema = {status: enum[resolved, unresolvable], files: [str], statement: str}`. It runs under the runtime's default role. Edit and `git add` permission in the main checkout is assumed; the roles' content is `s2-permissions-design`'s (critique M9). Its prompt (Go-only, in `prompts.go`):
      - resolve these files only when the resolution is mechanical;
      - `git add` each;
      - never commit; never run `merge --abort`, `reset` or `stash`; touch no other path;
      - otherwise report `unresolvable`.

      Its `CallResult` goes through `plan.SignalsFromCall` with `Writer: false` and `Uncommitted` cleared (the staged resolution is expected, and code checks the checkout itself), into `result.signals`. Then:
      - **`resolved`** must pass every check below, which code makes itself (critique S4):
        - no unmerged paths;
        - `git diff --cached --check` clean (conflict markers);
        - HEAD unmoved;
        - the staged set equal to the recorded staged set;
        - no unstaged change to any path in it;
        - the untracked set unchanged.

        Then `git commit -q -m <message, Lsf-Merge: resolved, Lsf-Conflict per path>` (hooks run, as for every code-stage commit). The commit is verified: parents `[pre, sha]`, trailers, and changed paths against `pre` equal to the recorded staged set.
      - **A replayed `resolved` that fails the checks** (the resolution was lost to an abort before a relaunch) is called once more under the next label, `merge-conflict:w<n>:<unit>#2` from `o.unique`, so a fresh call is made, or its own record replays (critique M4). At most two attempts per step run.
      - **Otherwise** (`unresolvable`, null, a violation, or a failed check): `git merge --abort` (`git reset --merge` if the abort fails). Assert no `MERGE_HEAD`, HEAD = pre, and every staged-set path clean. Status `unresolvable`, files = the conflicted paths.
      - **The agent moved HEAD:** a fault naming the commit, after the abort.
   4. **A fault mid-unit** (any git error, including a stale `index.lock` a real kill leaves) aborts a merge this step has in progress, then returns the error. A merge stop therefore leaves no merge in progress and no merge-written path uncommitted. A stale lock is a fault whose reason names it; removing the lock is the requester's (critique M10).
3. **Bookkeeping**, over the `clean`/`resolved` units, when there are any.
   - Read plan and archive at HEAD, render (§2.4), then `commitFiles` with subject `docs(<name>): wave <n> merge bookkeeping` and the label.
   - On an interrupted rerun, `adoptSet` first. `matches`: take the date from the candidate's first `### <YYYY-MM-DD> — <first merged unit> complete` line; the candidate must equal `renderBookkeeping(parent blobs)` with the same merged list and that date. The kill may cross midnight, which is why the date is not today's.
   - With no commit adopted, plan and archive dirt (staged or not) deferred from the precheck must equal the rendering with the date read the same way from the dirty plan. Else it is a fault: the paths hold changes this stage did not make.
   - With no merged unit, there is no commit.
4. **Result.**
   - `planSha` = HEAD; `architectureSha` = `git rev-list -1 refs/heads/<integration> -- <doc>`; `pauseRequested` from the pause ref; `integrationBranch`; `merges` in order.
   - Every SHA is full and read from git.

**`wave.go`:**
- Replace the call with `step[mergeResult](ctx, l.orch, "merge:w<n>", in, l.mergeStage(in))`. A fault is `stageFaultSignal(At{Stage: "merge", Wave})`, and the wave then has nothing merged.
- Then `checkMergeShas`, on live and replayed values alike (with a repository only, as `checkSha`):
  - every `clean`/`resolved` merge must have a non-empty SHA. An empty one is rejected, and the whole value is treated as a fault, so no merge is attributed to the wave;
  - each `merges[].sha`, `planSha` and `architectureSha` (when non-empty) must match `^[0-9a-f]{7,64}$` after `jsTrim` and lower-casing, resolve with `rev-parse --verify --quiet <sha>^{commit}`, and be an ancestor of `refs/heads/<integration>`;
  - a failure is a `stage-fault` "merge failed: returned <field> <sha> <reason>".
  - This is not `checkSha`'s correction: there is no single tip to correct a merge SHA against, so a SHA that fails is rejected, not corrected, and a replayed one is rejected too. A journaled merge SHA that a history rewrite has removed therefore faults on every relaunch until the requester clears the state dir. D24 states this (critique M3).
- The value's `signals` join `waveSignals`.
- Pause: `liveReads(ctx, nil, &m.Result.PauseRequested)` after the step, then `notePause`.
- `mergePrompt`, `mergeSchema`, and `mergeResultSchema` if unused, are deleted from Go. The JS keeps its own.
- Add **D24** to `doc.go`: merge as code; no gates (deferral); conflict call; trailers; adoption; a SHA fault; seeds ignored.

**`orchtest`:**
- `Stub.Step` for prefix `merge` sets `State.MergeOrder` from `input.units[].unit`, and marks `Completed` from the value as `Call` does.
- Default handler for `merge-conflict`: `resolved`. The stub never runs fn.

**Tests (`merge_test.go`, real git, using `gitOrch` with stub handlers for `merge-conflict` that edit and stage in `r.dir`):**
- two clean merges, in order: the commits' parents and trailers, the bookkeeping commit's two paths, and the value's SHAs, all full;
- a conflict resolved by the stub: `resolved`, files, one merge commit, a `claimed-disjoint-conflict` via `SignalsFromMerge`;
- a conflict the stub calls unresolvable: aborted, no `MERGE_HEAD`, `git status --porcelain` shows only the requester's dirt, and the next unit still merges;
- a stub that claims `resolved` but leaves a marker or unmerged path, or that also stages or creates another path: unresolvable and aborted;
- a stub that commits: a fault, with the checkout left with no merge in progress;
- a merge refused over requester dirt: unresolvable, the dirt intact;
- staged changes in the main checkout: a fault;
- a merge in progress that this stage did not start (no trailer in `MERGE_MSG`), interrupted or not: a fault, and the merge is left as it is;
- a unit already merged under another `sha`, and a `sha` already in integration outside the stage: faults, no second merge;
- a plan missing a unit's entry or its Plan Maintenance section: a fault before any merge, HEAD unchanged;
- a clean merge whose tree a `pre-merge-commit` hook altered: a fault naming the commit;
- a stale `index.lock`: a fault naming it, with no merge in progress;
- **bookkeeping injections:**
  - a `sedHook` that truncates the plan, and one that deletes the block's closing fence: each a fault naming the landed commit (not silent);
  - a hook `exit 1` (rejected): a fault, no commit, plan and archive restored, index clean;
  - the staging hook of §2.3;
- **kill and resume:**
  - a `gitRunner` decorator that fails the second `merge` with a cancellation: the interrupted rerun adopts unit 1's merge, merges unit 2, and makes one bookkeeping commit;
  - a full run, then an interrupted rerun: the same value, HEAD unchanged, no new commit, and the dry run not tripped by the moved entries;
  - a merge commit that failed tree verification, then an interrupted rerun: still a fault, not adopted;
  - a conflict resume after the agent staged part of the resolution plus an extra path: the derived sets catch the extra path;
  - a run cancelled after the bookkeeping commit's `commit` returns but before the record (a decorator cancelling at the post-commit `reset`): the rerun adopts;
  - an interrupted rerun with the conflict left in progress and the call already journaled: it resumes the conflict, without a second abort and redo;
  - a replayed `resolved` whose resolution is gone: a second call under `#2`;
- `wave.go` with a stub and a git repo: a merge value with an unresolvable SHA, a revision-syntax SHA, an empty clean SHA, or a bad `architectureSha` is a `stage-fault` stop, with nothing marked complete.

### 2.6 The cleanup step (`orchestrator/cleanup.go`, `wave.go`)

- **`unitResult.worktrees []string`:** the final labels of this `runUnit` invocation's isolated calls and of its completion-report step.
  - The isolated calls are collected through `o.noteWorktree(unit, label)`, called from `o.agent` when `opts.Isolation == "worktree"` (seeded or real). The map is held under `o.mu`, and `runUnit` takes the slice recorded since its start.
  - The completion-report label is noted in `runUnit` from `runStep`'s final label, not inside the step fn, which a replay skips (critique S5).
  - Earlier attempts' worktrees, from aborted waves or earlier launches, are not this unit's to remove: their branches may hold work in flight. This is a known limit, recorded in `lsf-runtime.md`.
- **Branches.** Only the completion-report branches are deleted, which is what the done condition names. The plan, implement, fix and reader branches stay, because a resume's completion-report input reads the unit branch's tip live (`unit.go:443`), and deleting it would turn every replay after a cleanup into a fault (critique C1). Their worktrees are removed, and a later stage that needs one gets it back from the surviving branch (`cwdFor`). The accumulation of `lsf/*` branches is a discovery for the completion report.
- **Input:** `{wave, integrationBranch, units: [{unit, worktrees}]}` for the merged units.
- **fn:** `RemoveWorktree` for each label, with `deleteBranch` for the completion-report label only (the `Runtime` interface gains it, and the stub returns absent); then `git worktree prune`. An interrupted rerun is naturally idempotent, since an absent worktree is reported absent.
- **Value:** `{result: {removed: n, kept: [label: reason], paths: [{label, paths}]}}`. The `cleanupResult` fields are `Removed`, `Kept`, `Paths []removedPaths{Label, Paths}`. Paths are worktree-relative, with a wholly untracked directory as one `dir/` entry (`--directory`), so the journaled value stays small (critique M1). The log carries the same list.
- **`wave.go`:** the gate call and the cleanup step run in one `rt.Parallel` (one agent thunk and one step thunk). The gate's `callSigs` is unchanged. Cleanup:
  - a fault is a log line, not a signal (its failure leaves worktrees, which hold nothing unmerged);
  - otherwise it logs `wave <n>: removed <r> merged worktree(s), kept <k>`, plus one line per worktree listing its removed paths.
- `cleanupPrompt` and `cleanupSchema` are deleted from Go.
- **Tests:** an orchestrator test with a fake `Runtime` wrapping the stub and recording `RemoveWorktree` calls checks that only the merged units' labels (`#n` included) are passed, with an unmerged unit's labels absent. The integration tests (§2.7) check the real removal.

### 2.7 Tests that move, and the real-runtime harness

- **`signals_test.go`.** The `merge` and `cleanup` driver-event cases (writer uncommitted, a denial, a schema violation, a seeded violation, and a reader's residue at cleanup) are retargeted:
  - to `replan` for writer events;
  - to `review-gate` for reader events.

  This satisfies "pass against a stage that is still an agent". The seed-violation case moves to `replan:w1`.
- **`run_test.go` / `runtime_test.go`.**
  - Prompt assertions on `merge:w1` become `Input` assertions: unit order, branches, archive path.
  - The `merge:w1` seed tests are rewritten. A step ignores seeds, so they assert the "args.seed ignored; code stage" log and the handler's value.
  - The tier table drops `merge` and `cleanup`.
  - The "merge SHA not reported" stub tests keep the JS fallback text (no repository, so no SHA check). A new git-backed test shows the rejection.
- **Integration (`integration_test.go`, `integration_signals_test.go`).**
  - The fake `claude` substitutes `@branch` and `@head` in a return with the cwd's `git rev-parse --abbrev-ref HEAD` / `HEAD`.
  - The unit stages' answers use `shell: git reset -q --hard work-A` and report `@branch`/`@head`.
  - The plan documents gain `## Work Units` / `### A` / `## Plan Maintenance` sections.
  - The merge and cleanup table entries are dropped, and termination returns `planSha: @head`.
  - Assertions:
    - the merge commit and bookkeeping commit are on integration;
    - the worktrees under `<state>/wt` and A's completion-report branch are gone, and its other `lsf/*` branches remain;
    - an ignored file a stage's shell wrote (with `.gitignore`) is listed in the cleanup log;
    - a relaunch after cleanup replays every call and step, with no spawn, no stage fault and no new commit (critique C1).
  - A new real-runtime test: a stage reporting `work-A` instead of its runtime branch stops with `branch-mismatch`.

### 2.8 promptparity (`tools/promptparity`)

- `codeStep` adds the `merge:` and `cleanup:` prefixes.
- `stepParity` gains two cases:
  - **merge.** For each unit, `"- <unit>: branch <branch> (completion report <report>)"`; `"Wave <n> merge order:"`; `"in <planPath> to <archivePath>"`; `"Ledger: <ledgerPath> wave <n>."`; `"git rev-list -1 <ib> -- <doc>"`; `"git show-ref --verify --quiet <pauseRef> "`; `"merge(<name>)"`.
  - **cleanup.** `"Cleanup stage, wave <n>. You are in the main checkout on <ib>."`.
- The step input struct gains `Wave`, `Units`, `ArchivePath`, `LedgerPath`, `ArchitectureDoc`, `Name`, `IntegrationBranch`.
- The scenario "carried rows; seeded plan and pausing merge" moves the pause seed off `merge:w1`, which steps ignore, onto the stub handler. Its JS side is answered from the Go step's value, as for every code step.
- Any scenario the Go outcome now differs on is adjusted to keep its coverage purpose, with the reason in a comment; no divergence is papered over.

### 2.9 Docs (in the behaviour commits)

- **`lsf-workflow.md::wave-loop`:**
  - the pseudo-code comment `// agent, sequential` becomes `// code; conflicts to a model call`;
  - the "Merge is an explicit stage" paragraph: code, a conflict model call, no gates until `declared-gates`;
  - the cleanup paragraph is rewritten: a code step removing the merged units' worktrees and branches with every ignored, untracked and uncommitted file, listed in its value and the log; a fault is logged; no "residue, not risk";
  - the archive paragraph: done by code, verified.
- **`::model-tiers`:** merge is code; its conflict call runs at the default tier; mechanical is the JS's cleanup only.
- **`lsf-runtime.md::runtime`:**
  - `CallResult.Branch` and `RemoveWorktree` in the API description;
  - Not in this unit, as the done condition words it: the ignored-file policy line names `code-merge-cleanup` (it now names `code-stage-interface`), and "The `Retains:` move: `retained-artefacts-impl`" is added;
  - Known limits: the review-gate/cleanup attribution line is fixed (cleanup no longer calls), and the earlier attempts' worktrees are kept.
- **`CLAUDE.md`**'s `lsf-runtime` line (merge and cleanup are code steps): a Scope deviation, as in earlier units.
- **`plan/doc.go`:** the new kind. **`orchestrator/doc.go`:** D23, D24, D25 (cleanup).

## 3. Implementation steps

Each step: `gofmt`/`go vet`, then `go test -race ./...` (the affected packages, then all of them), then commit, then a critique gate (`phronithm:critic`, model sonnet, critique-code) spawned after the commit while the next step proceeds.

0. Save this document as `docs/plans/code-merge-cleanup-plan.md` and commit. Re-probe `lsf-toolchain` (`go version` = go1.27.1; `go.mod` `toolchain go1.27.1`).
1. **Runtime (§2.1).** `CallResult.Branch`, `branchFor`, `RemoveWorktree`, and tests. Add `RemoveWorktree` to the orchestrator `Runtime` interface and the stub. Amend `lsf-runtime.md::runtime` (API). *Accept:* the runtime tests in §2.1 pass, and all packages build.
2. **Branch check (§2.2).** The `plan` kind, `call` returning the branch, the stage checks, the completion-report tip read, D23, and the fake `claude` `@branch`/`@head` with the realRun fixtures updated so the existing integration tests report the runtime's branch. Tests:
   - a stub `Runtime` wrapper setting `Branch` (mismatch at plan, implement and fix → a `branch-mismatch` stop, the unit aborted);
   - checkSha against the runtime branch;
   - the tip-read exit-1 / other-error split (decorated `gitRunner`);
   - the real-runtime mismatch test.

   *Accept:* those tests, and the whole suite green.
3. **Multi-path commits (§2.3).** *Accept:* the §2.3 tests and the existing stage tests unchanged.
4. **Bookkeeping render (§2.4).** *Accept:* the §2.4 tests.
5. **Merge step (§2.5)**, with the `wave.go` switch, `checkMergeShas`, stub `MergeOrder`, the merge tests, the moved `signals_test` merge cases (§2.7), the `run_test` adjustments, the merge half of the integration rework (§2.7: `### A` / `## Plan Maintenance` in the fixture plans, the merge table entries dropped, termination's `@head`), the promptparity merge `stepParity` (§2.8), and the `lsf-workflow.md::wave-loop` merge/archive paragraphs and `::model-tiers` (critique S6). *Accept:* the §2.5 tests; promptparity identical across all scenarios with node; the suite green.
6. **Cleanup step (§2.6)**, with worktree tracking, the moved cleanup cases, promptparity cleanup, the cleanup half of the integration rework (§2.7: the cleanup entries dropped, removal and relaunch-after-cleanup assertions), the wave-loop cleanup paragraph, `lsf-runtime.md` Not in this unit and Known limits, and the `CLAUDE.md` line. *Accept:* the §2.6 and §2.7 tests; promptparity and graphparity with `*_REQUIRE_NODE=1`; the full `go test -race ./...`.
7. **Implementation summary** `docs/plans/code-merge-cleanup-implementation.md`, then Phase 5.

Dependency order: 1 → 2 (needs `Branch`); 3 → 4 → 5 (needs 3's commits and 4's render; 5 follows 2 only because both edit `unit.go` and the integration fixtures); then 6 (needs 1's `RemoveWorktree` and 5's wave shape). Steps 3 and 4 are independent of 1 and 2 and may be done first if convenient.

### Risks

- **A call nested inside a step** (`merge-conflict` inside `merge:w<n>`). A runtime stop during it is returned as the step's stop, as `runStep` already maps it. The interrupted-rerun conflict resume (§2.5, step 1) is tested.
- **Integration-test rework.** The fake must now do real git in the worktrees. If `@branch` substitution turns out insufficient (for example, a stage needs the previous stage's branch), the fix is the `shell` entry, and no test-only path is added to production code.
- **`git merge` and staged changes.** It is assumed, and checked in step 5's first test, that `git merge` does not sweep a pre-staged change into the merge commit. The precheck refuses a staged index regardless.
- **Size.** Large, and four surfaces plus `plan` and docs. `code-stage-interface` and `runtime-signals` each finished comfortable at a similar size.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/code-merge-cleanup-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit code-merge-cleanup of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim (Phase 4 re-verifies the external gates against these
pins before implementing, recording the outcome in the implementation-summary):
see §1 "Unit entry, verbatim" above (scope, publishes, depends-on, external gate `lsf-toolchain` Go 1.27.1 pinned in `lsf-runtime/go.mod`, done condition), and the standing obligations listed beneath it (register dispositions P-C8 merge SHAs and P-C9; design of record; frozen JavaScript; `go.mod` stdlib-only).
After Phase 5, produce the completion-report (contents per the
phronithm:feature skill's Produces) at docs/plans/code-merge-cleanup-completion.md,
then present this continuation prompt to the requester:
Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
