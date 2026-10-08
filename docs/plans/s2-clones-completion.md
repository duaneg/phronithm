# s2-clones — completion report

- **Work unit:** `s2-clones` of `docs/plans/lsf-improvements-breakdown.md` @ `e327632`.
- **Branch:** `lsf-workflow`.
- **Plan:** `docs/plans/s2-clones-plan.md`.
- **Implementation summary:** `docs/plans/s2-clones-implementation.md`.

## Done-condition verdict

**Met (proven).** The Phase 5 full-diff critique checked each clause against the code and the tests and found none unmet. The implementation summary lists the test behind each clause. In brief:
- **Placement.** Every agent call runs in its own plain clone under the state dir: `clone/<key>` for a kept role, `scratch/<key>` for a discarded one, or the wave's merge clone for the conflict call. None runs in the main checkout or a worktree. The end-to-end test records every spawn's cwd.
- **Launch refusals.** A launch with `repo` or `stateDir` under `/tmp` or `/var/tmp`, written so or through a symlink, is refused.
- **Kept clones.** The branch is fetched into main as `lsf/<plan8>/<key>` without tags. The clone stays until its unit's cleanup, which removes it through `RemoveWorktree` and lists its untracked and ignored files (requester verdict, 2026-10-08). A writer's untracked file survives to cleanup and is listed there.
- **Discarded clones.** Removed after the call, with no uncommitted check. A reviewer's clone is gone after its call.
- **Roles.** Roles carry `clone`, and `<plugin>` is substituted in `deny`.
- **Writer repo calls.** `direct-edits`, `decision-replan`, `replan` and `termination` reach the integration branch only by the `ff:` step. The step checks the fetched branch, and stops the run when the main checkout is not clean at the expected tip. A requester edit stops the run at the next fast-forward.
- **The merge step.** It runs in `<stateDir>/merge/w<n>` with every Merge-conflicts check, and the conflict call takes that clone as its cwd. The integration branch fast-forwards after each verified merge commit, and the clone stays until the wave's merges finish. A step killed mid-merge resumes in the merge clone and adopts its merge by trailers.
- **Branches.** Every branch a prompt names is local in the clone.
- **Snapshot.** The repo-call snapshot is gone, and the Runtime section says so.
- **Tests.** `go test -race ./...` passes. `go vet` is clean. promptparity reports 55 scenarios identical and graphparity 145 cases, both with node required. code-merge-cleanup's and runtime-signals' tests pass, amended where they asserted the main checkout. The merge tests' amendments are listed under step 4 of the implementation summary.

Phase 5 ran an all-lens review and a full-diff critique gate, both on sonnet. Neither found anything Critical. Their Significant findings were fixed:
- an isolated writer on a discarded role was silent;
- the conflict call could plant git config or hooks in the merge clone;
- a global `git lfs install` refused every launch;
- a kill during clone creation blocked the relaunch.

One finding was handed to `s2-sandbox` (see Discoveries).

## Deviations from the unit entry

The deviations agreed at planning:
- `CLAUDE.md` and `testdata/config.example.json` edited outside Scope.
- The D30 prompt rewording (requester decision (d)). It changes those calls' input hash, so a run journaled before this unit re-runs them.
- New runtime API beyond Publishes: `Opts.Dir`, `MergeClone`, `RemoveMergeClone`, `Config.PluginDir`, `CallResult.Clone` and `CloneHead`.
- `RemoveWorktree` keeps its name; it is the removal `retained-artefacts-impl` reads as "the clone removal s2-clones puts in place of RemoveWorktree".
- A repo writer's clone is removed at its fast-forward, since it has no unit's cleanup (a reading of the clone-lifetime verdict).
- Clone creation copies the identity and line-ending config and `info/exclude`, and the launch refuses a shallow, submodule or LFS repo.
- The callers' null paths skip `agent-null` after the `ff:` step's fault.

Deviations taken during implementation:
- **Scope.** These files are outside the entry's Scope:
  - `lsf-runtime/internal/tmproot` and `lsf-runtime/internal/testenv` (new);
  - `lsf-runtime/tools/promptparity` (the D30 table);
  - `lsf-runtime/tools/fixturerun`: its default work dir is moved out of `/tmp`, and it passes `pluginDir`;
  - `lsf-runtime/testdata/claude` (stub modes);
  - `.gitignore`.
- **Replay re-fetch.** A replay re-fetches a kept clone's branch whenever the clone exists. The plan said only when the record had uncommitted paths or `unchecked`. Without this, a fault's remedy made in the clone never reached main on relaunch. A replay that moves the branch is logged with both tips.
- **LFS refusal.** It reads the repo's own use: `filter.lfs` in its local config, or `filter=lfs` in `.gitattributes` at the integration tip. The plan said "`filter.lfs` in config", which includes the global config and so refused every launch on a machine with git-lfs installed.
- **Signing config.** `configCopied` adds `commit.gpgsign`, `tag.gpgsign`, `user.signingkey` and `gpg.format`.
- **Reviewer clone.** A reviewer repo call's kept clone (a misconfigured role) is removed straight after the call rather than noted for cleanup, since it has no unit.
- **Further faults.** Each is a `stage-fault` that nulls the value, beyond the plan's list:
  - any writer, isolated or not, on a discarded role (its record re-runs once the role keeps the clone);
  - a writer repo call whose commits are held back by uncommitted changes in its clone;
  - a conflict call that changes the merge clone's git config, hooks or info files;
  - a merge, cherry-pick or revert in progress in the main checkout.
- **Clone creation.** A clone is built at `<dir>.partial` and renamed into place, with config and hooks path set before the checkout. The launch removes leftover `.partial` directories.
- **Head-moves loop.** The merge step's "HEAD moved during the gates" loop is removed: only the step moves the merge clone's HEAD, and a requester commit to main faults at the fast-forward.
- **Publishes:** as entered, plus the API listed above.

## Deferrals

None.

## Discoveries affecting other units

- **`s2-sandbox`:**
  - **Read-only `.git` in every clone.** The runtime's own git (the uncommitted check's `status`, the removal's `ls-files` and `diff`, the fetch-back) runs in clones an agent can write. `core.fsmonitor` is disabled for the runtime's and the orchestrator's git, and the merge clone's config, hooks and info are checked around the conflict call. A kept clone has no such check. An agent can define a filter in `.git/config` and select it from `.gitattributes`, which is in the work tree and stays writable; the runtime's next `git status` would then run that filter outside any sandbox. The sandbox should make `.git/config`, `.git/hooks` and `.git/info` read-only in every clone, the merge clone included. Its done condition should also say how `.gitattributes`-selected filters are contained.
  - **Merge clone write access.** The conflict call's cwd is `<stateDir>/merge/w<n>`, under the state dir that `denyRead` names. Its `allowWrite` must name the merge clone, which is not the call's own clone.
  - **`TMPDIR` and the `/tmp` refusal.** Per-call `TMPDIR` under `/tmp` (A call's launch) does not conflict with the refusal: nothing kept is under it. `internal/tmproot.Roots` is the list the sandbox's writable-temp findings rest on.
  - **`pluginDir`.** It is made absolute but not resolved through symlinks or checked for glob characters before it goes into `Edit(//<plugin>/**)` rules.
- **`retained-artefacts-impl`:** the removal it names is `lsfrt.Runtime.RemoveWorktree` (name kept), which removes a label's kept clone (`clone/<key>`) or a code step's worktree (`wt/<key>`), keeping one with commits not fetched.
- **`runtime-cli-drive`:** the state dir now holds `clone/`, `scratch/`, `merge/` and `*.partial` directories, and the xfer ref `refs/lsf/<plan8>/xfer/w<n>` in the main repo exists only during a merge step. Status readers should know them. A fault leaves a writer clone or a merge clone for the requester to act on; the fault text names the remedy.
- **`programme-close`:** none beyond the `CLAUDE.md` line, which this unit already updated.

## Standing obligations

None names this unit.

## External-gate re-verification

- `lsf-toolchain` (environment, Go 1.27.1): `go version` reports go1.27.1, matching `lsf-runtime/go.mod`. Probed at plan time, at Phase 4 entry, and again at Phase 5. No drift.
- `git` (environment, 2.38 or later): `git --version` reports 2.54.0. No drift.

## Execution signals

- **Context:** comfortable at completion.
- **Design decisions taken mid-implementation**, rather than at the design pass: about ten, each recorded per step in the implementation summary. The ones that changed behaviour:
  - the replay re-fetch;
  - the LFS rule;
  - the held-commit and discarded-writer faults;
  - the merge clone's git-setup check;
  - placement not yielding to a later stop;
  - partial-clone building;
  - the per-unit adoption in the merge sync.

  Most came from critique findings. One per-step Critical (step 4's fast-forward target) was fixed and confirmed in round 2.
- **Size:** the unit is large, as estimated: 47 files and about 3,100 lines added, half of them tests.

## Known limitations

These are recorded in `docs/design/lsf-runtime.md` Known limits, or noted from review:
- **Leftover clones.** A repo writer's clone that never fast-forwards stays, with its branch, until the call's re-run reuses or remakes it. A merge clone left by a fault stays for the relaunch. One whose integration branch diverged from main's must be removed by hand.
- **The main checkout** must stay on the integration branch and clean while the run is live. A requester edit faults at the next fast-forward.
- **Clone cost.** Every call copies the repo (`--no-local`), serialised. This is unmeasured, and a clone over 5 minutes stops the run.
- **Config a clone lacks.** It gets the identity, signing and line-ending keys only. A repo's own `filter.*` or `merge.*` drivers and `core.fileMode` do not reach it.
- **Branch moves.** A forced fetch-back overwrites `lsf/<plan8>/<key>` while the clone exists; a branch moved in main is kept only by the reflog, and the replay logs the move.
- **Agent-caused stops.** A fetch-back fails when an agent renames its branch, and that stops the run (runtime-error). A kept clone left on another branch stops its re-run.
- **Visibility.** Repo calls do not see the main checkout's untracked or ignored files.
- **Leftover files.** An untracked leftover in a writer's clone stops the run (uncommitted-at-return), as the old main-checkout snapshot did. The prompts do not say so.
- **Kill coverage.** The kill-and-resume evidence for the merge step uses built states and injected git failures. No test kills a step while a real conflict call is pending.
- **Temp roots.** The refusal uses a fixed list (`/tmp`, `/var/tmp` and their macOS forms). macOS is unprobed.

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
