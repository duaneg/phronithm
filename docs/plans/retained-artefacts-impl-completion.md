# retained-artefacts-impl: completion report

Unit `retained-artefacts-impl` of `docs/plans/lsf-improvements-breakdown.md`, on branch `lsf-workflow`, base `2a9d785`.

- **Plan:** `docs/plans/retained-artefacts-impl-plan.md`.
- **Implementation summary:** `docs/plans/retained-artefacts-impl-implementation.md`.
- **Commits:** 5bae08f, b52d2cc, dafadfb, ecb8b76, 049db54, 2164da2, and the commit that carries this report.

## Done-condition verdict

**Met**, with one exception: the suite's quiet-machine condition (R35) is not met. Each clause is proven by a test.

- **`retains` and `store` are read and pinned.** A unit's `retains: [paths]` is read from its `lsf-plan` entry (`plan.Unit.Retains`). The project's `store:` is read from the `lsf-gates` block (`plan.Declaration.Store`): an absolute, clean path, pinned with the block, so a change is `declared-gates-changed`. Pinning checks it is an existing directory outside the repository and the state dir (`Runtime.CheckStore`); otherwise the load stops with `invalid-plan`.
- **Path rules.** An absolute path, or one containing `..`, fails the load. The move refuses a path through a symlink, which covers one that resolves outside the clone.
- **Retains need a store.** `retains` with no `store` fails the load (`plan.CheckRetains` in `loadPlan`), and so the pre-flight (`validate.Check`).
- **A store-only block** loads, is not gated and has no hosts.
- **The move.** At cleanup, a merged unit's declared paths move to `<store>/<unit>/` before its clones are removed. Each comes from the latest writer clone holding it. Everything else goes with the clones, listed.
- **The move is journaled.**
  - It runs inside the journaled `cleanup:w<n>` step, with its own durable record `<statedir>/retained/<key>.json`.
  - A panic injected at each of the 10 checkpoints (17 cases, rename and copy), and a real SIGKILL mid-copy, each followed by a resume, complete the move. The resume never stops on its own partial destination and loses no path; a no-loss check runs after every kill.
  - A destination that exists before the move stops it, with the clones kept.
- **Cross-filesystem.** A move across filesystems removes the source only after the copy verifies by the decided method: sha256 while copying, fsync, then a re-read comparing sha256, size, mode and link targets.
  - Forced EXDEV runs everywhere.
  - A real move from `/dev/shm` (st_dev 26) to `/tmp` (st_dev 49) runs where two devices exist.
  - A forced mismatch keeps the source.
- **`code-merge-cleanup`'s cleanup tests pass.**
- **The tests pass.**
  - `go test -race -count=1 ./...` passed at the Phase 5 commit with `LSF_TEST_IMAGE` (lsf-ref), `LSF_LIVE_REQUIRE=1`, `PROMPTPARITY_REQUIRE_NODE=1` and `GRAPHPARITY_REQUIRE_NODE=1`, on 2026-10-11, 02:57–03:04.
  - The host was loaded by an unrelated `xtask` process throughout, using about 7.5 of 8 cores. The load average was 8.76 at the start and 9.60 at the end. R35's quiet-machine condition is **not met**, so this clause is *evidenced*, not proven.
  - Upgrade path: on an idle host, run `cd lsf-runtime && LSF_TEST_IMAGE=localhost/lsf-ref@sha256:949f2220… LSF_LIVE_REQUIRE=1 PROMPTPARITY_REQUIRE_NODE=1 GRAPHPARITY_REQUIRE_NODE=1 go test -race -count=1 ./...`.

## Deviations from the unit entry's scope and publishes

1. **`lsf-runtime/validate/validate.go`** gains the `CheckRetains` call, so the pre-flight fails on `retains` without `store`. It is outside the entry's scope, as the plan recorded.
2. **The holder rule is narrower than requester decision 2 as first written** (requester decision, 2026-10-11, at Phase 5). A writer clone holds a declared path only with an untracked or ignored file at or under it. Tracked content is merged already, so a later fresh clone with only a path's tracked files no longer displaces the clone with the output. An empty directory therefore counts as `missing`.
3. **The orchestrator's `Runtime` interface gains four methods.** `MoveRetained` and `CheckStore` are published. `CloneOf` and `RetainFiles` are added beyond the publishes, so that the failure record can name the files. `plan` gains `CheckRetainsPaths` and `SafeUnitDir`, shared with the runtime.
4. **New Go-only kinds:**
   - `retains-changed`, a stop (requester decision 5);
   - `retained-move-failed`, a record. It is raised by a failed move, and by a merged unit with retains but no writer clone in the launch (seeded writer stages).
5. **A `.git` component anywhere in a `retains` path is a load problem.** The plan said only "first component".
6. **`cleanupStage` resolves git only for `worktree prune`, after the removals.** Behaviour with a repository is unchanged.

## Deferrals

- **`docs/plan-graph.md`'s documentation of `retains` and `store`** (as Go-only keys) belongs to `empirical-unit-impl`.
- **Telling the re-plan and termination prompts to preserve a unit's `retains`** belongs to `empirical-unit-impl`, with `plan-graph.md`. The code already stops on a dropped path (`retains-changed`); nothing yet steers the agent away from it.
- **`phronithm:lsf-drive`'s stop-kind table** needs a `retains-changed` row (relaunch at the escalation's plan SHA on requester approval, as for `declared-gates-changed`). Its presentation of `retained-move-failed`'s by-hand remedy also needs adding. Both belong to `drive-presentation-impl`.

## Discoveries affecting sibling or downstream units

1. **`lsf-empirical-work.md` §Plan consequences** (line 565: "until it lands, a `Retains:` line keeps the worktree") is stale now that the move has landed. The file is outside this unit's scope, so the re-plan should carry the change.
2. **Unit ids containing `:` or `#` alias labels.** `noteWorktree` files a label under its last `:`-component, so a unit with id `r1:foo` is read as unit `foo`, round 1. `SafeUnitDir` protects only retaining units. A plan-wide id rule is a load change with a graphparity cost. It is a candidate for whichever unit next owns the plan's load rules.
3. **`status` does not report abandoned `.lsf-retain-*` staging or scratch directories in the store.** A failed move names them in its record. A CLI unit (`runtime-distribution` or a status follow-up) could list them beside the other leftovers.
4. **A failed cleanup step has no value.** The other units' moves and removals survive only in the log. Returning a partial value with the failure would need the step's failure model changed. It is a candidate for the unit that next touches cleanup.
5. **The `Runtime` interface grew by four methods.** A stub-based harness, or any other `Runtime` implementation, must add `MoveRetained`, `CheckStore`, `CloneOf` and `RetainFiles` (the `orchtest` stub has them).
6. **Phase 5 notes, minor and not fixed:**
   - the free-space check ignores directory and metadata overhead;
   - no multi-clone resume test;
   - no journal-level test that a failed cleanup step reruns on `resume`. `Step`'s intent semantics cover it, and so does the end-to-end runtime test of the move.

## Standing obligations

- **Register dispositions: P-D5 (store and move).** *Disposed.* The store is declared and pinned, and declared paths move to `<store>/<unit>/` at cleanup, crash-safely and verified across filesystems. The "lost" framing and the citation rule stay with `empirical-unit-impl` (its P-D5 citation-rule row).
- **Guarded host git in clones.** *Discharged.* The new git in clones goes through `cloneStdout`/`cloneCmd`, with `gitStdout` for a code step's worktree: `hasUntracked` (`ls-files --others`) and the factored `removal()` reads. No `status` or `diff` was added.
- **Design of record.** *Discharged.* `lsf-runtime.md` `## Runtime` and `lsf-workflow.md`'s wave loop were amended in `049db54`, where the behaviour first became reachable end to end, and in `2164da2`. Step 2's runtime-only commit `dafadfb` was unreachable from a run, so its doc text landed with step 3, as the plan specified.
- **R35.** *Not met:* see the done-condition verdict.
- **R20.** *Discharged by tests:*
  - kill and resume at every checkpoint, plus a real SIGKILL;
  - a forced-EXDEV verified copy, plus a real cross-filesystem move;
  - a forced verification mismatch that keeps the source.

## External-gate re-verification

- `lsf-toolchain`: `go version go1.27.1`, as pinned in `lsf-runtime/go.mod`. Confirmed at Phase 4 entry (2026-10-11), with no drift.

## Execution signals

- **Context at completion:** comfortable; no compaction.
- **Decisions taken mid-implementation, not at a design pass:**
  - the holder rule (put to the requester; a Phase 5 critique finding);
  - read-only directory handling (`renameDir`, `removeTree`), which the design missed;
  - the record tied to its source clones, with coverage checked from `renaming` as well as `done`;
  - refusing staging with no record;
  - the record kind for seeded writers;
  - `CloneOf` and `RetainFiles`.

  Each answered a critique or review finding; the approach did not change.
- **Critique gates:**
  - three per-step rounds: step 1 (5 Minor); step 2 (3 Significant, 4 Minor); step 3 (3 Significant, 6 Minor);
  - a Phase 5 multi-lens review: 7 Minor;
  - a full-diff critique: 2 Significant, 8 Minor.

  No round raised a Critical finding. Every Significant finding was fixed; the Minor ones not fixed are listed above and in the implementation summary.

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
