# retained-artefacts-impl — implementation summary

- **Working branch:** `lsf-workflow`
- **Plan:** `docs/plans/retained-artefacts-impl-plan.md`
- **External gate re-probe:** `lsf-toolchain`: `go version` reports `go1.27.1` and `lsf-runtime/go.mod` pins `toolchain go1.27.1`, so the pin is confirmed.

## Files modified

- `lsf-runtime/plan/`:
  - `graph.go`, `gates.go`, `retains.go` (new), `retains_test.go` (new), `gates_test.go`;
  - `schema.go`, `signals_test.go`, `doc.go`.
- `lsf-runtime/validate/`: `validate.go`, `cmd/lsf-validate/main_test.go`.
- `lsf-runtime/runtime/`: `worktree.go`, `retain.go` (new), `retain_test.go` (new), `supervise_test.go`, `clone_test.go` (a test helper moved to `retain.go`).
- `lsf-runtime/orchestrator/`:
  - `loop.go`, `wave.go`, `cleanup.go`, `results.go`, `runtime.go`, `doc.go`;
  - `orchtest/orchtest.go`, `orchtest/fixtures.go`;
  - `cleanup_test.go`, `retain_test.go` (new).
- `docs/design/lsf-runtime.md` (Runtime) and `docs/design/lsf-workflow.md` (wave loop).
- `docs/plans/retained-artefacts-impl-plan.md`, and this file.

## Per-step record

### Step 1: plan parse and validate (`5bae08f`, critique fixes `b52d2cc`)

- **Decisions.** Validation is in a new `plan/retains.go`, not in `graph.go`. Invalid entries stay in `Unit.Retains`, so a bad entry with no store reports two problems. Control characters are refused in retains entries, and in the store after critique.
- **Deviations.** The tests are in `retains_test.go`, not `graph_test.go` or `gates_test.go`. `validate` tests are in `cmd/lsf-validate/main_test.go`, where the existing `Check` tests are.
- **Surprises.** None.

### Step 2: runtime move (`dafadfb`, critique fixes `ecb8b76`)

- **Decisions.**
  - The pre-pass also walks each source tree, to refuse special files and to count bytes.
  - An all-missing move writes a `done` record without creating staging.
  - `ClonePath` was added for the orchestrator's failure record. The orchestrator cannot compute `journalKey`.
  - The test seam `deviceOf` was added, so the free-space refusal is testable on one filesystem.
  - The `retained-moved` event fires once, on the move. A rerun that returns the record does not emit it again.
- **Deviations.** Beyond the plan:
  - `ClonePath`.
  - `CheckRetainsPaths` in `plan`: the runtime re-checks the paths (critique M2).
  - `renameDir` and `removeTree`: read-only directories (critique S1).
- **Surprises.** The plan's design said a same-filesystem move is just `rename(2)`. `rename(2)` cannot move a directory without owner write permission to another parent, which covers a read-only tree such as a module cache. Copied read-only trees also defeat `os.RemoveAll`. Both are fixed: the move grants `u+w` for the rename only, and removal chmods first.

### Step 3: orchestrator and design docs (`049db54`)

- **Decisions.**
  - The `retains` baseline comes from the gates-pin step's value, the plan document at `args.planSha`. A decision re-plan that drops a path is therefore caught at the first load.
  - The failure record names clone paths through `Runtime.ClonePath`. It names branches only as "the unit's lsf/ branches named for the clones": the orchestrator does not know the runtime's plan8 after a decision re-plan.
  - `cleanupStage` now resolves git only for `worktree prune`, after the removals. Behaviour with a repository is unchanged. The change lets a stub run without a repository exercise the move and the record.
  - The wave summary log adds retained counts only when a move ran, so an undeclared plan's logs stay identical to the JS. Promptparity caught the first version.
- **Deviations.**
  - `orchtest/fixtures.go`'s `ToBlock` renders `retains`.
  - `Runtime` gains `ClonePath`.
  - D32 and D33 are both in `orchestrator/doc.go`, with a pointer in `plan/doc.go`.
- **Surprises.** `TestLoadReadsBlobsVerbatim` builds a launch without the pin, which left the baseline map nil. `retainsChanged` now initialises it.

## Per-step critique findings

### Step 1 (sonnet, round 1): no Critical

All findings were Minor:
- **M1, control characters in store:** addressed before exit.
- **M2, invalid UTF-8 or case-insensitive ids:** noted. The runtime is Linux-only.
- **M3, duplicate ids:** noted. `ValidatePlan` covers them.
- **M4, test gaps:** the broken-block guard and the `.gitignore` case were addressed. The valid store-and-retains case was already covered.
- **M5, redundant conditions:** noted, kept for readability.

### Step 2 (sonnet, round 1): no Critical

Significant:
- **S1, read-only directories wedge copies and resumes; a read-only source dir cannot be renamed:** addressed (`renameDir`, `removeTree`). Tests now use a 0555 subtree in every kill and resume case, and a 0555 declared directory in the rename test.
- **S2, a `done` record ignored newly declared paths:** addressed. That is now an error, with a test.
- **S3, kill tests asserted only end states:** addressed in part.
  - Added: after every kill, each path is whole in its clone, in staging or at the destination, and the destination exists only after the staging rename.
  - Added: a two-path refusal, valid path first, and refusals assert that no record was written.
  - Not added: a multi-clone resume test. Noted for Phase 5.

Minor:
- **M1, record staging and scratch trusted for removal:** addressed (`ownRetainDirs`), with a test.
- **M2, paths not re-checked:** addressed (`plan.CheckRetainsPaths`), with a `.git` test.
- **M3, header claim:** addressed (comment).
- **M4:**
  - `deviceOf` test fragility: addressed.
  - Free-space overhead: noted.
  - Directory mtimes and intermediate modes: added to Known limits.
  - A staged path with no record: now adopted.
  - Staging orphaned when the state dir moves: noted.

### Step 3 (sonnet, round 1): no Critical

All three Significant findings were fixed in Phase 5 (`2164da2`):
- **S1, the remedy relied on the error to name the files:** the record now names the clones, branches, staging, scratch and record (`CloneOf`, `RetainFiles`), and a failure after the staging rename names the record.
- **S2, a merged unit with retains and no worktrees was skipped silently:** generalised to "no writer clone in the launch". Such a unit now gets a record, with a test.
- **S3, dead `Kept` entries:** removed, and the D33 wording corrected.

Minor:
- **M1, termination and removed-unit tests:** added.
- **M2, no journal-level test of a failed cleanup step rerun:** not added. `Step` keeps a failed step's intent (runtime-core tests), and the end-to-end runtime test covers the move. Noted.
- **M3:** the invariant is commented.
- **M4:** the invariant is commented.
- **M5:** fixed.
- **M6:** D32 documents it.

## Phase 5

### Review (`phronithm:review`, all lenses, sonnet subagent)

Static pre-scan: `go vet` and `gofmt` are clean. `staticcheck` found four items, none in the changed files. Seven Minor findings:
1. **The `renaming` resume did not check the declared paths:** fixed.
2. **A `done` record could be reused for a later run of the unit:** fixed. The record ties to its source clones.
3. **Ignored errors:** commented. `removeTree`'s inner errors are best-effort before `RemoveAll`, which reports.
4. **`Bsize` instead of `Frsize`:** fixed.
5. **The baseline updated before a load was accepted:** fixed.
6. **A read-only parent of a retained path:** added to Known limits.
7. **Ranking puts stage before run:** kept as decided (requester decision 2). `writerSources` documents why its input is one run.

### Full-diff critique gate (critique-code, sonnet, round 1): passes, no Critical

Significant:
- **S1, an existence-only holder rule loses output when a retained directory has tracked files:** put to the requester, who decided (2026-10-11) that a holder is a clone with untracked or ignored content there. Implemented, with tests.
- **S2, no end-to-end test across the seam:** added (`TestCleanupRetainsOnRuntime`, real runtime and clones).

Minor:
- **M1, stale `done`:** fixed, as review item 2.
- **M2, a failed step loses the other units' value:** the log keeps the moves. D33 now says so. Noted.
- **M3, silent all-missing for seeded writers:** fixed by the record.
- **M4, signal wording:** fixed.
- **M5, adopting staging with no record:** now refused, and the record directory is fsynced.
- **M6, empty baseline:** logged.
- **M7, ids with `:` in non-retaining units alias labels:** not fixed. It is a plan-wide load rule with JS-parity cost. Discovery for the re-plan.
- **M8, nested `.git`:** fixed. Any `.git` component is a load problem.

## Success criteria verification

| # | Criterion | Target | Achieved | Evidence |
|---|---|---|---|---|
| S1 | `retains` parse and path rules | proven | **pass, proven** | `plan/retains_test.go` (`TestParseRetains`, `TestParseRetainsProblems`) |
| S2 | `store` parse; a store-only block loads, not gated, no hosts | proven | **pass, proven** | `TestParseDeclarationStore` |
| S3 | retains with no store fails load and pre-flight | proven | **pass, proven** | `TestCheckRetains`, `TestRetainsNeedStore` (lsf-validate and orchestrator) |
| S4 | store pinned (identity); the pin checks an existing dir outside the repo and state dir | proven | **pass, proven** | `TestStoreChangeStops`, `TestPinChecksStore`, `runtime.TestCheckStore` (missing, file, under or equal to the repo, symlink to the repo, ancestor, under the state dir) |
| S5 | the move from the latest writer clone before removal; the rest removed and listed; moved and missing reported | proven | **pass, proven** | `TestMoveRetainedRename`, `TestMoveRetainedHolderAndRecord`, `TestCleanupMovesRetained`, `TestCleanupRetainsOnRuntime` |
| S6 | symlink and outside-the-clone refusal, clones kept | proven | **pass, proven** | `TestMoveRetainedRefusals` (byte-identical clones, no record) |
| S7 | kill mid-move, then resume, completes | proven | **pass, proven** | `TestMoveRetainedKillResume` (17 checkpoint cases with a no-loss check after each), `TestMoveRetainedSIGKILL` |
| S8 | a pre-existing destination stops that unit, clones kept, record emitted | proven | **pass, proven** | `TestMoveRetainedSources`, `TestRetainedMoveFailedRecord`, `TestCleanupRetainsOnRuntime` |
| S9 | cross-filesystem verified copy; a mismatch keeps the source | proven | **pass, proven** | forced EXDEV (`TestMoveRetainedCopy`, `TestMoveRetainedVerify`); real `/dev/shm` (st_dev 26) against `/tmp` (st_dev 49), `TestMoveRetainedCrossFilesystem` |
| S10 | existing cleanup tests, graphparity and promptparity pass | proven | **pass, proven** | full suite, with the parity tools required |
| S11 | `go test -race ./...` on a quiet machine | evidenced | **pass on a loaded host; quiet condition not met** | see the completion report. Upgrade path: rerun on an idle host |
| S12 | design docs amended with the behaviour | evidenced | **pass, evidenced** | `049db54` and `2164da2` diffs |
| S13 | dropping a retains path stops; additions do not | proven | **pass, proven** | `TestRetainsChanged`, `TestRetainsNotComparedForDoneOrDropped` |

## Remaining phases

Complete Phase 5 (Review) as defined in the phronithm:feature skill.
