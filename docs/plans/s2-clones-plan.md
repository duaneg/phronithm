# s2-clones: problem statement, design and plan

Work unit `s2-clones` of `docs/plans/lsf-improvements-breakdown.md` (entry at `e327632`). Base commit `e327632`, branch `lsf-workflow`. Design input: `docs/design/lsf-runtime.md` Permissions (Placement, Reviewers, Merge conflicts, Runtime changes 1–3). On approval this file is saved as `docs/plans/s2-clones-plan.md`.

## Context

Under the decided B2 boundary (`s2-sandbox`), sandboxed Bash must not reach the main checkout or the state dir. Today every agent call runs either in the main checkout (repo calls, `critique-plan`, `merge-conflict`) or in a worktree whose `.git` is the main repo's, so its refs and config are writable. This unit moves every agent call into its own plain clone under the state dir. Commits return by fetch, and reach the integration branch only through orchestrator fast-forward steps. The sandbox itself lands in `s2-sandbox`; calls here still run under the configured roles.

## Unit entry (verbatim)

```
### s2-clones
- **Type**: feature
- **Risk**: high — every agent call leaves the main checkout and the worktrees, and every writer's work reaches the integration branch by a new fast-forward step
- **Clarity**: specified — `lsf-runtime.md` Permissions (Placement, Reviewers, Merge conflicts, Runtime changes 1–3) and the clone-lifetime verdict
- **Iteration**: write-once
- **Effort**: large
- **Scope**: `lsf-runtime/runtime`, `lsf-runtime/orchestrator`, `docs/design/lsf-runtime.md::runtime`
- **Publishes**: per-call clones (kept and discarded) and fetch-back; the integration fast-forward step; the merge clone; the role field `clone`
- **Depends on**: s2-permissions-design (design, Placement, Reviewers and Merge conflicts); code-merge-cleanup (logical, the merge and cleanup steps moved into clones, `RemoveWorktree`); declared-gates (logical, serialises the runtime and orchestrator files)
- **External gates**: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`); `git` (environment, 2.38 or later)
- **Done condition**: Every agent call runs in its own plain clone under the state dir, as Placement's table assigns it, and none in the main checkout or a worktree. A launch with the main checkout or the state dir under `/tmp` or `/var/tmp` is refused. A kept clone's branch is fetched into the main repo as `lsf/<plan8>/<key>`, without tags. A kept clone stays until its unit's cleanup, which removes it in place of a worktree and lists its untracked and ignored files as `RemoveWorktree` does (requester verdict, 2026-10-08). A discarded clone is removed after its call and has no uncommitted check. Roles carry the field `clone` (`kept` or `discarded`), and `<plugin>` is substituted in `deny`. A writer repo call (`direct-edits`, `decision-replan`, `replan`, `termination`) reaches the integration branch only through an orchestrator step that checks its fetched branch and fast-forwards; the step stops the run when the main checkout is not clean at the expected tip. The merge step runs in `<stateDir>/merge/w<n>` with every check Merge conflicts lists, the conflict call takes that clone as its cwd, the integration branch fast-forwards after each verified merge commit, and the clone stays until the wave's merges finish. Every branch a prompt names is a local branch in the clone. The repo-call snapshot goes from Uncommitted at return, and the Runtime section says so. Tests show: a writer's untracked file survives to its unit's cleanup; a reviewer's clone is gone after its call; a requester edit in the main checkout stops the run at the next fast-forward; a step killed mid-merge resumes in the merge clone and adopts its merge by trailers. `code-merge-cleanup`'s and `runtime-signals`' tests pass, amended only where they asserted a worktree or the main checkout. The tests pass.
- **Description**: The first half of the Permissions section's Runtime changes, split from a single carrier on the size budget (requester verdict, 2026-10-08). Calls still run under the configured roles: the boundary lands with `s2-sandbox`, and these clones are its precondition, since the sandbox's `denyRead` on the main checkout holds only when no call runs there. The orchestrator half has no reference implementation.
```

No standing obligation names this unit.

## Success criteria

1. **Done condition, verbatim** (above). Target `proven`, by `go test -race ./...` in `lsf-runtime`.
2. `go vet ./...` clean; promptparity (55 scenarios) and graphparity pass with node required (`PROMPTPARITY_REQUIRE_NODE=1`, `GRAPHPARITY_REQUIRE_NODE=1`). The prompt rewording (D30) is applied to the JS side by a named rewrite table, so every other byte still matches. Target `proven`.
3. Requester decisions taken at planning (2026-10-08) are implemented:
   - **(a)** a clone gets the main repo's effective hooks;
   - **(b)** "clean" for a fast-forward means no staged or unstaged change to a tracked file;
   - **(c)** a writer repo call is fast-forwarded only when it returned a value with a clean clone;
   - **(d)** prompts are reworded for clones now.

   Target `proven` (tests).

## Exploration findings

- **Runtime (`lsf-runtime/runtime`).**
  - `cwdFor` (`worktree.go`) picks the repo or a worktree `wt/<key>` on `lsf/<plan8>/<key>`, serialised by `wtSem`.
  - `RemoveWorktree` lists `ls-files --others --directory` plus `diff --name-only HEAD`, then removes.
  - `attempt` (`agent.go`) resolves the cwd at the first admitted attempt and takes the repo-call snapshot.
  - `uncommitted.go` holds the snapshot, `priorDirt` and `.dirt` machinery.
  - `Config.Role` is `{tools, allow, deny, permissionMode}` (`config.go`), and `buildArgv` (`call.go`) emits it.
  - `Opts` (`api.go`) has `Isolation`; `callHash` includes it.
  - `RunGates` (`gates.go`) adds a detached worktree of `commit` from `cfg.Repo`, so the commit must be in the main repo.
- **Orchestrator.**
  - Isolated writers are `plan`, `fix-plan`, `implement` and `fix-code`; isolated reviewers are `critique-code`, `lens-review` and `critique-design` (`unit.go`).
  - `critique-plan` is a repo call with `agentType phronithm:critic`.
  - The repo writers are `direct-edits` (`loop.go`), `decision-replan` (`loop.go`), `replan:w<n>` and `termination` (`wave.go`). Each is `site.writer` with no isolation.
  - The repo reviewers are `review-gate:w<n>` and `obligations`.
  - The merge step (`merge.go`) runs on `mergeGit{g, o.repo}`. Its conflict call has no isolation. `mergeGates` makes a dangling candidate with `commit-tree` in `m.dir`, and `gatedMerge` gates merge commits through `RunGates`.
  - The pause ref is read with `pauseRequested(m.dir, …)`. The other pause reads use `o.repo`.
  - `completion-report` uses `rt.Worktree`, a code step's own worktree. It stays a worktree: it is runtime code, not an agent call.
  - Cleanup calls `RemoveWorktree` over `o.worktrees[unit]`, the labels noted by `noteWorktree`.
- **Prompts.** The `worktree(base)` clause makes an isolated agent `git reset --hard <base>`. `ownFilesOnly` and the repo prompts say "You are in the main checkout". `pauseClause` is unused.
- **Tests.** Real-runtime orchestrator tests (`integration*_test.go`, `gates_test.go`, `stages_test.go`) use `lsfrt.Run` with the stub `testdata/claude`, which has `dirty:`/`modify:` modes. `t.TempDir()` lives under `$TMPDIR`, usually `/tmp`.
- **Divergences.** The last divergence is D27, so this unit takes D28 onwards.

## Candidate approaches

- **A. Runtime owns clones; orchestrator owns integration (chosen).** The runtime creates, reuses, fetches back and removes per-call clones, chosen by role `clone` and `Opts.Isolation`. The orchestrator adds the fast-forward step and moves the merge step into a runtime-provided merge clone. This matches the design's split: the runtime changes are 1 and 3, the orchestrator changes 2.
- **B. Orchestrator owns clones.** The orchestrator would create a clone per call and pass its path as the cwd. Rejected:
  - it duplicates the runtime's journal-keyed reuse, stop and serialisation logic in the orchestrator;
  - the stub-runtime tests and promptparity would see a cwd argument on every call;
  - the uncommitted check and fetch-back need the attempt boundary, which only the runtime sees.
- **C. Keep worktrees with a separate git dir.** Rejected by the design: a worktree's `.git` is the main repo's, and `--shared` alternates point into the masked repo.

## Design

### Runtime: roles and launch refusal (D28 covers the runtime side)

- **`Role.Clone`** (`json:"clone"`): `kept` or `discarded`, required. `prepare` rejects a missing or other value.
- **`Config.PluginDir`** (`pluginDir`, absolute after `prepare`). The text `<plugin>` in a role's `deny` entries is replaced by it when argv is built. A `<plugin>` with no `pluginDir` is a config error.
- **`/tmp` refusal.** `Run` refuses a launch, as a setup error, when `Repo` or `StateDir` is under `/tmp` or `/var/tmp`. The check runs both lexically and after `EvalSymlinks`, and covers `/private/tmp` and `/private/var/tmp` for macOS.
  - Test packages get a `TestMain` that points `TMPDIR` at a fresh directory under `os.UserCacheDir()/lsf-runtime-test/` and removes it afterwards. It lives in the shared helper `internal/testenv`. `t.TempDir()` then never lies under `/tmp`.
  - The stub's `STUB_STATE` dirs follow the same rule.

### Runtime: clones

State dir layout:

| Path | Holds |
|---|---|
| `clone/<key>` | kept clones |
| `scratch/<key>` | discarded clones; the directory is wiped at launch under the flock, so a crashed launch leaves none |
| `merge/<name>` | merge clones |
| `wt/<key>` | stays, for code steps' worktrees (`rt.Worktree`) |

**Placement** (resolved at the call's first admitted attempt, under `wtSem`):

| Call | Directory | Checked out | Fetched back |
|---|---|---|---|
| `Opts.Dir` set (the conflict call) | `Dir`, which must be an existing directory under `<stateDir>/merge/` | as found; no clone, no fetch-back, no uncommitted check | — |
| `Isolation: "worktree"` | `clone/<key>` or `scratch/<key>` by role | `lsf/<plan8>/<key>`: main's branch if it exists, else a new branch from the integration branch, as `cwdFor` does today | kept: the call's branch → main `refs/heads/lsf/<plan8>/<key>` |
| no isolation | `clone/<key>` or `scratch/<key>` by role | the integration branch | kept: the clone's integration branch → main `refs/heads/lsf/<plan8>/<key>` |

**Creation** of a new clone:

1. `git clone --no-local --no-checkout --no-tags -q <repo> <dir>`. `--no-local` avoids hardlinking main's object files, which a sandboxed write could otherwise reach through the shared inode.
2. `git fetch --no-tags --update-head-ok -q <repo> '+refs/heads/*:refs/heads/*'`, so every branch a prompt names is local, at main's tip.
3. `git checkout -q <branch>`, or `-b <branch> <integration>`.
4. `git remote remove origin`.
5. Hooks, per decision (a): copy the main repo's effective hooks directory into the clone's `.git/hooks`. The effective directory is `git rev-parse --git-path hooks` in main, which honours `core.hooksPath`. Executable bits are kept and `*.sample` files skipped.
6. Config: worktrees shared main's config, and a clone has none of it.
   - Copy the effective values (local or global, as `git config --get` in main gives them) of a fixed list into the clone's local config: `user.name`, `user.email`, `core.autocrlf`, `core.eol` and `core.safecrlf`.
   - Copy main's `info/exclude` and `info/attributes`.
   - This is also the design's "git identity set in its local config from the main repo's", which `s2-sandbox` needs once it replaces `HOME`.

**Preconditions.** The main repo must not be shallow, must have no submodules (`.gitmodules` at the integration tip) and no LFS filter (`filter.lfs` in config). The launch refuses otherwise, naming which. All are untested with clones.

A failure removes the partial directory and is a `runtime-error` stop, as a worktree failure is now.

**Reuse.** A kept clone that exists is reused when it is a clone, not a worktree (`.git` is a directory), its toplevel is the directory, and HEAD is on the expected branch. Otherwise the run stops with `runtime-error`, naming it. On reuse every other branch is refreshed from main: `git fetch --no-tags <repo> '+refs/heads/*:refs/heads/*' '^refs/heads/<checked-out>'`. Negative refspecs need git ≥ 2.29, within the 2.38 gate.

A kept repo-call clone (on the integration branch) is reused only when main's integration tip is an ancestor of the clone's HEAD. That holds after an unmoved main, as decision (c)'s re-run in the same clone intends. Otherwise the clone is stale: it is removed and recreated, and the abandoned HEAD SHA is logged. Its commits stay on main's `lsf/<plan8>/<key>`, fetched back at the null, until the re-run's fetch-back replaces them.

A discarded clone is always made fresh: an existing `scratch/<key>` is removed first.

**`CallResult.Clone`** (`clone`, journaled): `kept`, `discarded`, or `""` when no clone was made (an explicit `Dir`, a seed, a stub).

**After a spawned call returns** a value or a null (not on a stop, a repeated signal or a dry run):

- **Kept:**
  1. Fetch back: in main, `git fetch --no-tags -q <clone> +<clone ref>:refs/heads/lsf/<plan8>/<key>`. `CallResult.Branch` is that branch, for repo calls too.
  2. Run the uncommitted check: every dirty path in the clone, as an isolated worktree's is today.

  A fetch-back failure is a `runtime-error` stop naming the clone, which is kept.
- **Discarded:** `os.RemoveAll`, with no uncommitted check. `CallResult.Branch` is `""`. A removal failure is logged; the next launch's wipe removes the clone.

**A clone HEAD off its branch.** Fetch-back always fetches the expected branch. If the clone's HEAD is detached or on another branch, the runtime logs it with the HEAD SHA.
- For an isolated writer, the orchestrator's existing writer check raises `branch-mismatch` on the returned branch.
- For a repo writer, the fast-forward step finds the branch's tip already in the integration branch and applies nothing. It then reports this as a fault ("no commits fetched, clone HEAD off <int>") when the clone's HEAD differs from the fetched tip, so work is never dropped silently. The runtime records that HEAD in `CallResult.CloneHead`, journaled.

**Replay.** A replayed kept call re-fetches its branch from the clone only when the record had uncommitted paths or `unchecked`, and the clone still exists. That is the route by which a requester commit made there to clear `uncommitted-at-return` reaches main. It can never recreate a branch cleanup deleted. The role and clone kind come from the journaled `Clone`, so replay needs no role resolution. Its uncommitted re-check reads the clone as it stands; a removed clone is clean.

**Uncommitted at return** (D28). The repo-call snapshot goes: `takeSnapshot`'s before-state, its content hashing, `priorDirt`, the `.dirt` file and `stateDirIn` exclusion. Every checked call is a kept clone, and every dirty path in it is the call's. The record format keeps `paths` and `unchecked`.

**`Opts.Dir`** (`json:"dir,omitempty"`). It is excluded from `callHash`, since the path is a function of the wave and the state dir. Promptparity's fixed opts shape omits it.

**New runtime API:**
- `rt.MergeClone(ctx, name) (path string, err error)` creates `<stateDir>/merge/<name>` on the integration branch, as above, or returns the existing clone after the reuse check, without the HEAD-branch refresh, since a merge may be in progress there.
- `rt.RemoveMergeClone(ctx, name) error`.
- `RemoveWorktree(ctx, label, integration, deleteBranch)` keeps its name and contract, and handles a label whose directory is `clone/<key>` as well as `wt/<key>`.
  - **Lookup.** A label has one or the other: agent calls get clones, and code steps get worktrees. If both exist, both are kept, with reason "both a clone and a worktree".
  - **Merged check.** It reads main's `lsf/<plan8>/<key>`. A clone whose HEAD differs from that branch, or is not on it, is kept with reason "clone has commits not fetched", so commits the requester made there are never deleted unlisted.
  - **Listing and removal.** A clone is listed as a worktree is (others plus dirty), then removed with `os.RemoveAll`.
  - **Kept reasons.** Otherwise unchanged, with "not a clone on <branch>" for a clone.
  - **Name.** Keeping the name limits churn. `retained-artefacts-impl` reads "the clone removal s2-clones puts in place of RemoveWorktree", and this is that removal.

**Serialisation.** Clone creation, fetch-back and removal of a kept clone take `wtSem`. Each git command keeps the 5-minute `gitContext`.

**Launch wipe and `Dir`.**
- The `scratch/` wipe runs after orphan reaping, so no live orphan call loses its cwd.
- `Opts.Dir` is checked under `merge/` after `EvalSymlinks`.

**Test temp base.** The test `TMPDIR` base falls back to `$HOME/.cache` and then to the repo's `lsf-runtime/.testtmp` (git-ignored) when `os.UserCacheDir()` fails. Every package whose tests make git repos gets the `TestMain`: `runtime`, `orchestrator`, `orchestrator/orchtest` users, `validate` and `tools/*`.

### Orchestrator: fast-forward step (D29)

**Trigger.** In `callBranch`, after `o.agent`, when all of these hold:
- `s.writer` is set;
- `opts.Isolation == ""`;
- `cr.Branch != ""`;
- the value is non-null;
- `cr.Uncommitted` is empty;
- `uncommitted-unchecked` is unset.

This is decision (c). A seed and a stub runtime without branches never trigger it. The step is `runStep("ff:"+label, {branch, integration, cloneHead}, …)`.

**Misconfigured role.** When a writer repo call returns a non-null value with `cr.Clone == "discarded"`, the result is a `stage-fault`: "writer repo call ran in a discarded clone; its commits were not kept: give its role `clone: kept`". It is never a silent skip. A reviewer site with `cr.Clone == "kept"` is logged, and its label is noted for cleanup removal.

**The body, `o.fastForward(ctx, branch)`**, in `o.repo`:
1. Read `tip` as `refs/heads/<branch>`, and HEAD, which must be on `refs/heads/<integration>`.
2. If `tip` is an ancestor of HEAD, there is nothing to apply: it was already applied, or adopted after an interrupted step.
3. If HEAD is an ancestor of `tip`:
   1. Check the checkout is clean, per decision (b): `git diff --quiet HEAD` and `git diff --cached --quiet`.
   2. Run `git merge --ff-only -q <tip>`.
   3. Verify that HEAD equals `tip`.
4. Otherwise the integration branch moved while the call ran: a fault naming both SHAs. The remedy is to rebase or drop the clone's commits, then relaunch.

**After a successful step,** `rt.RemoveWorktree(label, integration, true)` removes the clone and its fetched branch. A removal failure is logged, not a fault.

This is a reading of the clone-lifetime verdict ("stays until its unit's cleanup"): repo writers have no unit, so their clone goes at the fast-forward. Its ignored and untracked files are listed in the log as cleanup lists them. It is recorded as a deviation for the completion report.

A repo-writer clone that never fast-forwards (a null, a stop, a fault) stays with its branch until the call's re-run replaces it. It is a Known limit, not cleaned up by any unit's cleanup.

**A fault** is a `stage-fault` signal (a stop kind, `plan/schema.go` `StopKinds`) at the call's site. The decoded result is nil. So that the ledger does not also record `agent-null`, `callBranch` returns a fault marker, and the four callers' null paths add `NullReturnSignal` only when the call's signals hold no stop kind. Tests assert the `stage-fault` signal and its statement, not just the stop. The clone and branch are kept. Covered faults:
- dirty main checkout;
- wrong branch;
- diverged history;
- git refused the fast-forward, for instance over an untracked file in the way.

### Orchestrator: merge step in a merge clone (D29)

**Setup.** `mergeStage` takes `dir, err := o.rt.MergeClone(ctx, "w"+wave)`, and `m := mergeGit{g, dir}`. Every check runs unchanged on `m.dir`:
- `outside` and `changedOutside`;
- `head != pre`;
- `stagedOK`;
- `write-tree` and `mergeGates`;
- the trailers commit;
- `abortOwn`.

**Main checks at step start,** before any merge: the main checkout is on the integration branch and clean per decision (b). Today's up-front staged-path and book-path checks run on main as well as on the clone, so a requester edit stops the step before any gate or conflict call runs. The checks repeat at each fast-forward.

**Sync at step start,** before `ownMergeInProgress`, with the merge clone's integration as `c` and main's as `h`:
- `c == h`: nothing to do.
- `c` ahead of `h`: a crash between a merge commit and its fast-forward. Each first-parent commit in `h..c` must be one this step made and would accept:
  - a merge that `verifyMerge` accepts for this label and a unit in `in.Units`, then `gatedMerge`, which replays from the memo or journal;
  - or the bookkeeping commit, which the existing `adoptSet` check accepts.

  If all are accepted, fast-forward main through them. Anything else (a commit the conflict call made, a gate-failing merge) is a fault naming the commit, with the clone kept. A merge whose gates failed is never committed in the clone: `regressed` aborts before the commit, and `gatedMerge`'s post-commit failure is already a fault.
- `c` behind `h`, with no merge in progress: refresh the clone to `h` by fetch and `reset --hard`. The clone holds nothing then.
- Otherwise, a fault naming the clone, with the remedy: remove `<stateDir>/merge/w<n>` and relaunch. Adoption by trailers then reads main's history.

A stale `refs/lsf/<plan8>/xfer/w<n>` in main is deleted at step start.

**Transfer to main.** `o.toMain(ctx, m, sha)` sets `refs/lsf/xfer` to `sha` in the merge clone, then runs `git fetch --no-tags -q <clone> +refs/lsf/xfer:refs/lsf/<plan8>/xfer/w<n>` in main. It is used:
- before every `RunGates` on a merge candidate or merge commit (`mergeGates`, `gatedMerge`);
- by each fast-forward.

**Fast-forwards.**
- After each verified `clean` or `resolved` merge (`mergeOne` returning), and after the bookkeeping commit, run `o.toMain`, then the same `fastForward` body against `refs/lsf/<plan8>/xfer/w<n>`.
- A fault is the step's error, which is the existing merge-stage fault path.

**Other changes.**
- `PauseRequested` is read from `o.repo`.
- `PlanSha` and `ArchitectureSha` are read after the final fast-forward. They are equal in either repo.
- **At the end of the step,** after the bookkeeping and the last fast-forward, `RemoveMergeClone` runs and the xfer ref in main is deleted. A removal failure is logged and does not fail a step whose merges are complete.
- **Test impact.** The orchestrator `Runtime` interface gains `MergeClone` and `RemoveMergeClone`. `orchtest.Stub` and the merge tests' `gitRT` implement them. Conflict handlers write into the call's `Dir`, not the main repo. The refused-over-local tests become requester-edit-at-fast-forward tests.
- **The D30 rewording** changes `callHash` for reworded prompts, so a run journaled before this unit re-runs those calls. This is noted in the completion report.
- **On a fault** the clone stays: "the clone stays until the wave's merges finish".
- **The conflict call** gets `Opts.Dir = dir`.
- **`refusedOverLocal`** stays as code. It now guards only the merge clone, which has no requester edits; its tests are amended to show that a requester edit in main stops the run at the fast-forward instead.

### Prompts (D30, decision (d))

- **`worktree(base)`** becomes the clone clause: "You are running in a clone of the repository, made for this stage. Before anything else run `git reset --hard <base> && git clean -fd`… Make all changes and commits here; the runtime fetches your branch when you return. Report your branch…"
- **The repo prompts' "You are in the main checkout on <int>"** becomes "You are in a clone of the repository on <int>, made for this stage; the runtime fast-forwards <int> to your commits after you return."
- **`ownFilesOnly`** keeps the commit-by-name discipline and drops the requester sentence and the "in the main checkout" phrases.
- **The conflict prompt** says "a clone of the repository on <int>, in the middle of `git merge`…".
- **Promptparity.** `tools/promptparity` applies a rewrite table to the JS prompts before comparing (old → new pairs, each tagged D30), so a missed or extra change still fails. The final wording is drafted in step 6, against the code.

### Docs

- **`docs/design/lsf-runtime.md` Runtime:**
  - Interface: `Opts.Dir`, `MergeClone`, `RemoveMergeClone`, clone-aware `RemoveWorktree`, and `CallResult.Branch` for kept repo calls.
  - Configuration: `clone`, `pluginDir`, the `/tmp` refusal.
  - A call: the clone at the first admitted attempt, fetch-back and discarded removal.
  - Uncommitted at return: the snapshot is gone, and the section says so.
  - Journal and state dir: `clone/`, `scratch/`, `merge/` and `wt/` for code steps.
  - Supervision: clones.
  - Known limits: the repo-call attribution limit and the re-run limit go; new limits are a repo-writer clone that never fast-forwards (kept with its branch until the call re-runs), clone cost on large repos (copied per call, serialised), and every main branch copied into every clone.
- **`orchestrator/doc.go`:** D28 to D30.
- **`testdata/config.example.json`:** the design's `roleRules`, roles with `clone`, and `pluginDir`.
- **`CLAUDE.md`'s `lsf-runtime` line:** "worktrees and their removal" becomes "per-call clones, fetch-back and removal". This is outside the unit's scope, as in earlier units; it is recorded.

### Risks and unknowns

| Risk | Resolution | When |
|---|---|---|
| git behaviour assumed above: `--update-head-ok` fetch into a `--no-checkout` clone's HEAD branch; a negative refspec skipping the checked-out branch; fetch from a clone path by a non-head ref into main; `merge --ff-only` succeeding over unrelated dirty tracked files (so the explicit check is needed); `rev-parse --git-path hooks` honouring `core.hooksPath` | one scratch script on git 2.54 | step 1, before the code |
| Clone cost per call on big repos | known limit; measure on the fixture | after |
| `stepParity` or promptparity scenarios that expect no `ff:` or merge-clone steps | stub runtime returns no `Branch`, so no `ff:` step; check merge scenarios | step 6 |
| Tests depending on `/tmp` paths | `TestMain` relocation | step 1 |

**API verification record.** None of the git behaviours in the first row is verified yet; all are flagged and checked in step 1. Everything else rests on the repo's own code, read this session.

## Steps

Each step: implement, `cd lsf-runtime && gofmt -l . && go vet ./... && go test -race ./...` green, commit, then run the critique gate (critique-code, `phronithm:critic` on sonnet) concurrently with the next step.

1. **Git probe, test temp relocation and launch refusal.**
   - Probe script (scratchpad) confirming the risk-table behaviours; results go into the implementation summary.
   - `internal/testenv` with `TestMain` in `runtime` and `orchestrator` (and `validate` if it uses temp git repos).
   - The `/tmp` refusal in `Run`.
   - Accept: a test refuses `Repo`, and another refuses `StateDir`, under `/tmp` and `/var/tmp` (lexical and symlinked); the existing suite is green.
2. **Role field and plugin substitution.**
   - `Role.Clone` validation, `PluginDir`, and `<plugin>` substitution in `buildArgv`.
   - Example config updated; existing test configs gain `clone`.
   - Accept: config tests for a missing or invalid `clone` and for `<plugin>` without `pluginDir`; an argv test shows the substituted deny.
3. **Runtime clones, with clone-aware removal.** This step lands the clone machinery and switches placement for every call except no-isolation calls with a `kept` role (repo writers and the conflict call). Those stay in `cfg.Repo` behind one unexported switch, `keptRepoCallsInRepo`, until step 5, so every existing orchestrator test stays green between steps.
   - Placement, creation (hooks and config copy, preconditions), reuse and refresh, fetch-back, discarded removal, the launch wipe of `scratch/`, `Opts.Dir`, `CallResult.Clone` and `CloneHead`.
   - The repo-call snapshot stays while the switch does, for the calls still in `cfg.Repo`; step 5 deletes it.
   - Replay re-fetch.
   - Clone-aware `RemoveWorktree`, `MergeClone` and `RemoveMergeClone`.
   - Accept (runtime tests, stub claude):
     - every switched call's cwd is a clone (`.git` a directory), never `cfg.Repo` or `wt/`;
     - an isolated writer's commit appears on main's `lsf/<plan8>/<key>` with no tags fetched;
     - a repo writer (with the switch off in the test) gets its commit on main's `lsf/<plan8>/<key>`, with main's integration branch unmoved;
     - a reviewer's clone is gone after its call, with no uncommitted check (a `dirty:` reviewer reports none);
     - a writer's dirty path is reported, and clears on replay after removal;
     - every main branch is local in the clone;
     - a main pre-commit hook runs on a commit in the clone, and a repo-local-only identity is used by a commit in the clone;
     - a shallow main, or one with submodules, is refused;
     - `Opts.Dir` outside `merge/` is refused, symlink included;
     - a kept writer clone holding an ignored file survives to removal, and the removal lists it;
     - the not-merged, branch-missing, not-a-clone and unfetched-commits `Kept` reasons;
     - `MergeClone` reuse keeps a merge in progress;
     - cleanup tests (`cleanup_test.go`) pass, amended only for the `clone/` path.
4. **Merge step in the merge clone.**
   - Main checks at step start, the sync with commit-by-commit acceptance, `toMain`, a fast-forward after each merge and after the bookkeeping, the pause ref read from main, merge clone removal, and `Opts.Dir` on the conflict call. The `Runtime` interface, `orchtest.Stub` and `gitRT` change with it.
   - Accept:
     - `merge_test.go` and `merge_gates_test.go` pass, with the amendments the test-impact note above lists;
     - a test kills the step mid-merge (conflict call pending), relaunches, and shows it resumes in `merge/w<n>` and adopts its merge by trailers;
     - a sync test of a crash after a merge commit: the merge is adopted and fast-forwarded, and an unrecognised commit in the clone faults;
     - main is fast-forwarded after each merge, and the clone is gone after the step;
     - a requester edit in main stops the step before the first merge.
5. **Fast-forward step, and the switch removed.**
   - `fastForward`, the `ff:` step in `callBranch`, the misconfigured-role fault, the callers' null paths, and clone removal after the step.
   - Delete `keptRepoCallsInRepo` and the repo-call snapshot (`takeSnapshot`'s before-state, `priorDirt`, `.dirt`, the state-dir exclusion).
   - Accept (real-runtime orchestrator tests):
     - `replan` commits reach the integration branch only after `ff:`;
     - a null `direct-edits` is not fast-forwarded;
     - a requester edit to a tracked file in the main checkout stops the run at the next fast-forward with a `stage-fault` whose statement names the file, the clone is kept, and no `agent-null` is recorded;
     - an untracked requester file does not stop it;
     - a diverged integration branch faults;
     - a repo writer on a discarded role faults;
     - a stale reused repo-writer clone is recreated;
     - an interrupted `ff:` step re-run is a no-op adoption;
     - `runtime-signals` integration tests are amended only where they asserted a main-checkout repo call.
6. **Prompt rewording.**
   - The D30 wording, and the promptparity rewrite table.
   - Accept: promptparity passes 55 scenarios with node required, as does graphparity; a deliberately unmapped JS change still fails.
7. **Docs and the end-to-end check.**
   - The `lsf-runtime.md` Runtime section, `doc.go` D28–D30, and the `CLAUDE.md` line.
   - One real-runtime orchestrator test on a stub-driven unit through cleanup, showing:
     - no call's cwd is the main checkout or a worktree (the stub records `pwd` per invocation);
     - the writer's ignored file is listed by cleanup's removal;
     - every kept clone is gone after cleanup.
   - Accept: full `go test -race ./...`, `go vet`, both parity checks with node required.

Steps 1–2 are independent of each other; 3 needs 1–2; 4 needs 3; 5 needs 4; 6 needs 5 (the reworded prompts describe clones, which hold for every call only after 5); 7 is last. Every step leaves the suite green.

## Verification

- `cd lsf-runtime && go vet ./... && go test -race ./...`
- `PROMPTPARITY_REQUIRE_NODE=1 GRAPHPARITY_REQUIRE_NODE=1 go test ./tools/...`
- `go run ./tools/promptparity` and `go run ./tools/graphparity`
- External gates re-probed at Phase 4 entry: `go version` against 1.27.1, and `git --version` at 2.38 or later (2.54 at planning).
- `tools/fixturerun` is not run here: it is `s2-sandbox`'s done condition, and its pin (2.1.289) is pruned.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/s2-clones-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit s2-clones of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim: see "Unit entry (verbatim)" above. Phase 4 re-verifies the external
gates against these pins before implementing (`lsf-toolchain`: Go 1.27.1; `git`: 2.38 or later),
recording the outcome in the implementation-summary. No standing obligation names this unit.

Deviations already agreed at planning, for the completion report:
- `CLAUDE.md` and `testdata/config.example.json` edits outside Scope;
- prompt rewording (D30) by requester decision (d);
- the new runtime API (`Opts.Dir`, `MergeClone`, `RemoveMergeClone`, `Config.PluginDir`) beyond Publishes;
- `RemoveWorktree` keeps its name.
- a repo writer's kept clone is removed at its fast-forward, as it has no unit's cleanup (a reading of the clone-lifetime verdict);
- clone creation copies identity and normalisation config and `info/exclude`, and the launch refuses a shallow, submodule or LFS main repo;
- `CallResult.Clone` and `CloneHead`, and the callers' null paths skipping `agent-null` after a stop-kind fault;
- the D30 rewording changes `callHash` for those prompts, so a run journaled before this unit re-runs them.

After Phase 5, produce the completion-report (contents per the phronithm:feature skill's
Produces) at docs/plans/s2-clones-completion.md, then instruct the requester to run
`/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before
starting any dependent unit.
