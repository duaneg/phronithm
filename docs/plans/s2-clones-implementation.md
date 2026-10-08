# s2-clones — implementation summary

- **Branch**: `lsf-workflow` (base `e327632`)
- **Plan**: `docs/plans/s2-clones-plan.md` (committed `7cac386`)
- **External gate re-probe** (Phase 4 entry):
  - `lsf-toolchain`: `go version` → go1.27.1, matching `lsf-runtime/go.mod` (`go 1.27`, `toolchain go1.27.1`).
  - `git`: `git --version` → 2.54.0, within 2.38 or later.
  - No drift.

## Files modified

- Runtime:
  - `lsf-runtime/runtime/clone.go` (new: placement, creation, reuse, fetch-back, discard, merge clones, repo preconditions, launch wipe)
  - `lsf-runtime/runtime/{agent,api,call,config,journal,runtime,uncommitted,worktree}.go`
- Orchestrator:
  - `lsf-runtime/orchestrator/ff.go` (new: main-checkout checks, `fastForward`, the `ff:` step)
  - `lsf-runtime/orchestrator/{args,doc,git,loop,merge,prompts,runtime,wave}.go`
  - `lsf-runtime/orchestrator/orchtest/orchtest.go`
- Shared: `lsf-runtime/internal/tmproot/` (new), `lsf-runtime/internal/testenv/` (new)
- Tools:
  - `lsf-runtime/tools/promptparity/main.go` (the D30 table)
  - `lsf-runtime/tools/fixturerun/{main,main_test}.go`
- Test data: `lsf-runtime/testdata/claude` (stub: `cwd.<n>`, `commit:` mode), `lsf-runtime/testdata/config.example.json`
- Tests:
  - runtime: `clone_test.go` (new), `config_test.go`, `uncommitted_test.go`, `worktree_test.go`, `journal_test.go`, `supervise_test.go`, `helpers_test.go`, `call_test.go`
  - orchestrator: `ff_test.go` (new), `merge_test.go`, `merge_gates_test.go`, `integration_test.go`, `integration_signals_test.go`, `integration_gates_test.go`, `gates_test.go`, `stages_test.go`, `run_test.go`
- Docs and config: `docs/design/lsf-runtime.md` (Runtime section), `CLAUDE.md`, `.gitignore`

## Per-step record

### Step 1: git probe, test temp relocation, launch refusal (`c69fbea`)

- **Probe (git 2.54)**, every assumption confirmed:
  - `rev-parse --git-path hooks` honours `core.hooksPath`. It returns the path as configured, which may be relative, so it is resolved against the repo top.
  - `--update-head-ok` fetches into a `--no-checkout` clone's HEAD branch.
  - A negative refspec skips the checked-out branch.
  - A non-head ref fetches from a clone path into main without tags.
  - `merge --ff-only` *succeeds* over an unrelated dirty tracked file, so the explicit clean check is needed.
  - `merge --ff-only` refuses over an untracked file in the way.
- **Decision**: `validate` gets no `TestMain`: it makes no git repos and does not launch.
- **Deviation**: the temp-root check is shared code in `internal/tmproot`, imported by the runtime and `internal/testenv`. Fixing a step 1 critique finding, it moved there in `d8ea25f`.

### Step 2: role field and plugin substitution (`33b26b9`, `d8ea25f`)

- **Decision**: `<plugin>` in `allow` is a config error. It is substituted in `deny` only, as the design writes it.
- **Decision**: the example-config test uses the orchestrator's real label shape (`lens-review:r1:u1`), not the old `lens-review:general:r1:u1`.
- **Deviation**: `fixturerun`'s default work dir moved from `os.MkdirTemp` (which is under `/tmp`) to the user cache dir. The command refuses a `-dir` under a temp root.

### Step 3: runtime clones (`1704656`; critique fixes in `cc8ab7d`)

- **Decision**: every runtime test config carries `args`, since every clone needs them. With no `repo`, a call keeps the empty cwd; the runtime's own tests are the only users of that mode.
- **Decision**: `CloneHead` is recorded for every kept call, so the `ff:` step can compare against it.
- **Decision**: a fetch-back takes the lock without yielding to a first-signal stop, because an in-flight call's return is journaled after one.
- **Deviation**: `configCopied` adds `commit.gpgsign`, `tag.gpgsign`, `user.signingkey` and `gpg.format`, so the merge clone's commits sign as main's would.
- **Deviation**: test-only stub additions: a `commit:<path>` mode and a `cwd.<n>` record. The runtime test repo sets a repo-local identity.
- **Surprise**: the `clone` event's `kind` field overwrote the telemetry record's `kind`. The field was renamed to `clone`.

### Step 4: merge step in a merge clone (`cc8ab7d`)

- **Decision**: the sync at step start checks `h..c` structurally:
  - each commit carries the step's `Lsf-Step`;
  - a merge must be of a wave unit, with that unit's SHA as its second parent;
  - a single-parent commit must be the bookkeeping, come last, and touch only the plan and archive.

  `verifyMerge`, `gatedMerge` and the fast-forward to main are left to the merge loop's adoption, unit by unit.
- **Decision**: the sync also refreshes the clone's other branches, so the report branches and their objects are current on reuse. A clone behind main is reset to main's tip.
- **Decision**: the main checks at step start are:
  - on the integration branch;
  - no tracked changes;
  - the book paths clean, untracked files included.
- **Test amendments**:
  - Conflict handlers write in the call's merge clone, and killed-step states are built in the clone.
  - Refused-over-local became requester-file-at-fast-forward, with adoption on relaunch. A new requester-edit-in-main test covers an edit before the first merge and one mid-step.
  - A foreign merge in progress moved into the clone; one in main is now a main-checkout fault.
  - The conflict-call-outside requester cases became tracked changes in the clone.
  - The interrupted-edges third part uses unresolvable-then-resolved.
  - `failNth`'s `merge` matcher was narrowed to the step's own `--no-ff`, because a fast-forward is also a `git merge`.
  - The head-moves test became a requester commit to main during gates, which faults at the fast-forward.

### Step 5: the `ff:` step; switch removed (`ae13b68`; critique fixes `1c950ef`)

- **Decision**: a reviewer repo call's kept clone (a misconfigured role) is logged and removed straight away, since it has no unit's cleanup. The review gate gets the same treatment on its own path.
- **Decision**: `agent()` notes an isolated label for cleanup after the call, and only when its clone is not discarded.
- **Decision**: `nullUnlessFaulted` keys on the `ff:` step's `stage-fault` at the call's stage, not on any stop kind. A schema-violation null still gets `agent-null`, as in JS, which keeps parity.
- **Decision**: once a call is admitted, clone placement does not yield to a sticky stop set since, so an admitted call spawns. `TestCapsUnderConcurrencyAreSticky` caught this regression.
- **Decision**: a clone HEAD that differs from the fetched tip is checked before the fast-forward: a fault, with nothing applied.
- **Deleted**:
  - `keptRepoCallsInRepo`;
  - the main-checkout snapshot (before-state, content hashing, `.dirt`, the state-dir exclusion);
  - the tests of those.

  Tests reworked: `FailedAttemptsDirt`, `RenameIsOneEntry` (now on `dirtyPaths`), `NullReRunKeepsDirt` (via clone reuse), `TestCallResultBranch` (a kept repo call has a branch).
- **Deviation**: a replay re-fetches a kept clone's branch whenever the clone exists, not only when the record had paths or `unchecked`, so a fault's remedy made in the clone reaches main on relaunch. A replay that moves the branch is logged with both tips.
- **Deviation**:
  - a call recorded from a discarded clone re-runs once its role keeps the clone;
  - a writer whose commits are held back by uncommitted changes has its value nulled by a `stage-fault`;
  - the value is decoded before the fast-forward;
  - the ancestor check precedes the clean check;
  - a cherry-pick or revert in progress in main is a fault.

### Step 6: prompt rewording, D30 (`bcdf45e`)

- **Wording**:
  - `worktree()`: "a clone of the repository, made for this stage; every branch named here is a local branch in it … the runtime fetches that branch when you return".
  - `inClone()`, which opens the repo writers' prompts: "You are in a clone of the repository on <int>, made for this stage; the runtime fast-forwards <int> to your commits after you return."
  - `ownFilesOnly` drops the requester sentence and "in the main checkout".
  - The conflict prompt: "a clone of the repository on <int>, made for this merge step". This prompt is Go-only, so it has no table entry.
- **Promptparity**: a `d30` table of 5 regex pairs over the compared JS agent calls. Code steps' JS prompts are untouched, and every pair must match at least once.
  - 55 scenarios identical.
  - A deliberately unmapped Go change failed 30 of 55.

### Step 7: docs and the end-to-end check (`a2b18d9`, `02e3e3f`)

- **End-to-end**: `TestOnRuntimeCodeStages` runs with the design's reviewer role and rules. The fake claude records its cwd, and every call's cwd is under `clone/` or `scratch/`; both are used, and `scratch/` is empty afterwards. The existing assertions show `build.log` listed by cleanup and `clone/` and `wt/` emptied.
- **Docs**:
  - `lsf-runtime.md` Runtime: Interface, Configuration, A call (with a Clones table), Uncommitted at return, Journal, Supervision and Known limits;
  - `doc.go`: D23–D25 brought to current state, D28–D30 added;
  - `CLAUDE.md`.

### Phase 5 fixes (`baa244f`, `bd30609`)

- **Discarded role**: every writer, isolated or not, on a discarded role is a `stage-fault`.
- **Conflict-call hardening**: the orchestrator's git runs with `core.fsmonitor=false`. A conflict call that changes the merge clone's `.git` config, hooks or info files is a fault before any more git runs there.
- **Deviation, LFS**: the refusal reads the repo's own use (local `filter.lfs`, or `filter=lfs` in `.gitattributes` at the tip), not the plan's "`filter.lfs` in config". With git's global config included, a global `git lfs install` refused every launch.
- **Partial clones**: a clone is built at `<dir>.partial` and renamed into place, and the launch removes leftovers, so a kill during creation no longer blocks the relaunch.
- **Order of setup**: the config and hooks path are set before the checkout, so no hook runs at creation and line endings shape the work tree.
- **Remedy text**: the merge step's fast-forward fault names the merge clone's remedy.
- **New test**: a `replan` fast-forward on the real runtime.

## Critique findings and dispositions

Gate: `phronithm:critic`, sonnet, critique-code, one round per step; step 7 used critique-design. One Critical in total, fixed.

| Step | Critical | Significant | Minor |
|---|---|---|---|
| 1 | — | fixturerun default under `/tmp`: **fixed** | duplicated temp-root check, weak assertion, dangling symlink: **fixed**. Other temp roots (`/dev/shm`, macOS `/var/folders`, case-insensitive paths), testenv leftovers on kill: **noted** |
| 2 | — | symlink cycle recursion in `tmproot.Under`: **fixed** | fail-open errors, `<plugin>` in `allow`, `-dir` under `/tmp`, role error order: **fixed**. `PluginDir` symlinks and glob characters: **noted for s2-sandbox** |
| 3 | — | stale repo-call clone removed with unfetched work: **fixed**. Hooks subdirectories not copied: **fixed**. Fetch-back failure (agent renamed its branch) stops the run: **kept per plan, noted**. Clone cost under one lock: **known limit** | fsmonitor, discard on stop, signing config: **fixed**. HEAD off branch halts re-run, other clone branches unlisted at removal: **noted** |
| 4 | fast-forward to the clone's HEAD could pass unverified commits on relaunch: **fixed** (`ff(sha)`, bookkeeping sha, final clone-versus-main check, test), confirmed in round 2 | fast-forward cancellable mid-checkout: **fixed** | dead head-moves loop removed, `PlanSha` from the branch, xfer ref deleted on exit, main merge-in-progress check, hook-copy depth bound: **fixed**. Reused merge clone keeps creation-time hooks: **noted** |
| 5 | — | remedies unreachable on relaunch, discarded-writer fault permanent, held-commit SHAs built on: **fixed** | decode before fast-forward, ancestor before clean, cherry-pick and revert: **fixed**. Pre-clone main-checkout records lose their paths on replay: **noted** (the D30 hash change re-runs those calls) |
| 6 | — | an untracked leftover in a writer's clone stops the run and the prompt does not say so: **noted** (the old snapshot did the same). Forced re-fetch can rewind a branch moved in main: **mitigated** (logged) | `mainClean` error text: **fixed**. Reviewers told their branch is fetched; "stay on the branch" not said; `lostToDiscard` re-runs readers: **noted** |
| 7 | — | per-operation bound, stale large-files limit, repo-less mode, replay semantics, writer-clone limit, `args` wording, main-checkout obligations: **fixed** | journal fields, history wording, D23–D25, smaller omissions: **fixed**. Section-header narrative, e2e not mapping calls to roles: **noted** |
| Phase 5 review (all lenses) | — | isolated discarded writer, merge-clone git hardening, global LFS: **fixed** | comments, remedy text, dropped errors, ff result: **fixed**. Config allow-list, a switched kept clone: **known limits**. Hook-symlink copying, small duplication, long functions, recreate vs reset, the blocking lock in fetch-back: **noted** |
| Phase 5 full-diff critique | — | kill during clone creation: **fixed**. Runtime git in agent-writable kept clones: **hand-off to s2-sandbox** | `replan` real-runtime test, setup before checkout, merge remedy, repo-call visibility limit, stale comment: **fixed**. No real-conflict-call kill test, repo-less mode, fixed temp roots, re-fetch cost: **noted** |

## Success criteria verification

Rigour: `proven ▸ evidenced ▸ conjectured`.

1. **Done condition, verbatim**: **pass**, target `proven`, achieved `proven`. The full-diff critique checked it clause by clause, and each clause has a test:
   - every call in a clone: `TestOnRuntimeCodeStages`, `TestCloneIsolatedWriter`, `TestCloneDiscardedReviewer`, `TestCloneRepoWriter`;
   - `/tmp` refusal: `TestLaunchRefusesTempRoots`;
   - fetch as `lsf/<plan8>/<key>` without tags: `TestCloneIsolatedWriter`;
   - kept clone until cleanup, listing its files: `TestRemoveClone`, `TestRemoveCloneKeeps`, and the end-to-end `build.log`;
   - discarded clone gone, unchecked: `TestCloneDiscardedReviewer`;
   - the role field and `<plugin>`: `TestLoadConfigRejects`, `TestPluginSubstitution`, `TestExampleConfigLoads`;
   - the writer fast-forward and its stop on a dirty main: `TestFastForward*`;
   - the merge clone: `merge_test.go`, `merge_gates_test.go`, `TestMergeResume`;
   - local branches: `TestCloneIsolatedWriter`;
   - snapshot gone, Runtime section updated;
   - a writer's untracked file survives to cleanup: `TestRemoveClone`;
   - a reviewer's clone gone: `TestCloneDiscardedReviewer`;
   - a requester edit stops the run: `TestFastForwardRequesterEdit`, `TestMergeRequesterEditInMain`;
   - a kill mid-merge resumes in the merge clone and adopts by trailers: `TestMergeResume/a merge killed after its conflict was staged`, which builds the killed state;
   - code-merge-cleanup's and runtime-signals' tests pass, amended as listed under step 4.

   `go test -race ./...` passes.
2. **`go vet` clean; promptparity and graphparity with node required**: **pass**, `proven`. With `PROMPTPARITY_REQUIRE_NODE=1` and `GRAPHPARITY_REQUIRE_NODE=1`, promptparity reports 55 scenarios identical and graphparity 145 cases identical. An unmapped change fails promptparity.
3. **Decisions (a)–(d)**: **pass**, `proven`.
   - (a) hooks: `TestCloneIsolatedWriter`, `TestCloneHooksTreeAndStaleDirt`.
   - (b) clean means no tracked change: `TestFastForwardRequesterEdit` (a tracked edit stops, an untracked file does not).
   - (c) fast-forward only a value with a clean clone: `TestFastForwardNotOnNull`, `TestFastForwardRecovery`.
   - (d) prompts reworded: promptparity's D30 table, `TestOwnFilesOnly`.

## Remaining phases

Complete Phase 5 (Review) as defined in the phronithm:feature skill.
