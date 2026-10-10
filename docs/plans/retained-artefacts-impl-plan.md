# retained-artefacts-impl — plan (phronithm:feature, work-unit mode, compressed Phases 1–3)

Base commit `2a9d785` on `lsf-workflow`. Saved after approval to `docs/plans/retained-artefacts-impl-plan.md`.

## Context

`lsf-empirical-work.md` §Retained artefacts (decided 2026-10-07) and §Units U8 (decided `984e248`) set the policy: cleanup deletes every undeclared untracked or ignored file in a merged unit's clones; a unit declares the paths it keeps with `retains: [paths]` in its `lsf-plan` block entry; the project declares one store, `store:` in the `lsf-gates` block; at cleanup the runtime moves declared paths to `<store>/<unit>/` before the clone goes, as a journaled step. Today the Go runtime has the deletion half (`code-merge-cleanup`, `orchestrator/cleanup.go`, `lsfrt.Runtime.RemoveWorktree`) but no declaration, so a runtime run keeps no ignored file (R20). This unit adds the parse, the store check and the move. Register row P-D5 (store and move) is disposed of here.

## Problem statement

### Unit entry (verbatim, `docs/plans/lsf-improvements-breakdown.md` @ `2a9d785`)

### retained-artefacts-impl
- **Type**: feature
- **Risk**: high — a fault between the move and the worktree's removal deletes data a unit declared kept, P-C9's loss again
- **Clarity**: exploratory — one open decision: how a move across filesystems copies and verifies before the source goes
- **Iteration**: write-once
- **Effort**: medium
- **Scope**: `lsf-runtime/plan`, `lsf-runtime/orchestrator`, `lsf-runtime/runtime`, `docs/design/lsf-runtime.md::runtime`, `docs/design/lsf-workflow.md::wave-loop`
- **Publishes**: the `Retains:` declaration's parse and store check; the journaled move to `<store>/<unit>/`
- **Depends on**: retained-artefacts-design (design, the policy); empirical-unit-design (design, U8: `retains` per unit in the `lsf-plan` block, `store:` in the `lsf-gates` block); code-merge-cleanup (logical, cleanup as a code stage); s2-clones (logical, kept clones and their removal at cleanup); s2-egress-gates (logical, serialises the runtime files)
- **External gates**: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`)
- **Done condition**: A unit's `retains: [paths]` is read from its `lsf-plan` block entry and the project's store from a `store:` key in the `lsf-gates` block, an absolute host path outside the repository, pinned at launch with the rest of the block (`lsf-empirical-work.md` §Units, U8). A `retains` path that is absolute or contains `..` fails the load, and the move refuses one that resolves outside the clone. A `retains` entry in a plan whose `lsf-gates` block has no `store` fails the load, and so the pre-flight command. A block holding `store` alone loads and declares no gates and no hosts. At cleanup, a merged unit's kept clones' `Retains:` paths are moved to `<store>/<unit>/`, `<unit>` being the plan's unit id, before the clone is removed; every other ignored or untracked file goes with it, listed in the report. The move is a journaled runtime step: a test kills it mid-move and the resume completes it without stopping on its own partial destination or losing a path, and a destination that exists before the move stops it with the clone kept. A test moves across filesystems and shows the source is removed only after the copy verifies by the method the unit decides. `code-merge-cleanup`'s cleanup tests pass. The tests pass.
- **Description**: The runtime half of `lsf-empirical-work.md`'s Retained artefacts. The parse and the move land together, so no interim rule keeps a worktree for an unmoved declaration. Cleanup's removal is `lsfrt.Runtime.RemoveWorktree` (name kept by `s2-clones`), which removes a label's kept clone `clone/<key>` or a code step's worktree `wt/<key>`, keeps one with commits not fetched, deletes everything untracked or ignored and lists it, and every branch of a merged unit is deleted with it; the move runs before that call, per clone label. `lsf-empirical-work.md` states the policy for worktrees; a kept clone takes the worktree's place one for one (requester verdict, 2026-10-08). The runtime moves on the host; an agent's container mounts no path outside its clone. **Flag:** three code surfaces exceed the surface bound; the `plan` change is one field and its load check, so the excess is not context.

Standing obligations naming this unit:
- **Register dispositions**: P-D5 (store and move).
- **Guarded host git in clones**: any host git command added in a clone goes through `cloneCmd`/`execGit` or the `gitenv` guards; `status`/`diff` there passes `--ignore-submodules=all`.
- **Design of record**: amends `lsf-runtime.md::runtime` and `lsf-workflow.md::wave-loop` in the same commit as the behaviour.
- **R35**: "the tests pass" needs a quiet-machine run of the suite.
- **R20**: the mitigation is the kill-mid-move and cross-filesystem tests.

### Requester decisions (this session, 2026-10-10)

1. **Verification (the open decision):** sha256 the source bytes while copying; fsync each file and each created directory; re-read the destination and compare sha256, size and permission bits; symlinks copied as links with their targets compared; any other non-regular file refused. A declared path's source is removed only after its whole tree verifies. A same-filesystem move is a `rename(2)` and needs no check.
2. **Several clones holding a path:** sources are the unit's writer clones only, ranked `plan` < `fix-plan:rN` < `implement` < `fix-code:rN` (by N; a `#n` suffix later than none, by n). Per declared path, the latest writer clone holding it is the source. Copies in other clones, reviewers' included, go with their clones and are listed.
3. **A declared path in no clone:** logged and listed as `missing` in the cleanup value; the other paths move and the clones are removed as usual. No signal.
4. **Store existence:** pinning the block checks the store is an existing directory whose resolved path is outside the repository and outside the state dir; otherwise the load stops (`invalid-plan`). The pre-flight command checks only the path's form.

### Interpretations recorded (deviations to report)

- **Stricter symlink refusal.** The move refuses a declared path if it or any parent component inside the clone is a symlink (Lstat per component), not only one that resolves outside. A link inside the clone would move as a dangling link; refusing is the safe reading. Symlinks *inside* a retained tree are moved as links, never followed.
- **Path form.** A `retains` entry must be relative, in clean form (`path.Clean(p) == p`, so no trailing `/`, no `.`/`..` components), not `.`, its first component not `.git`; entries are unique and none is inside another. All are load problems (`invalid-plan`).
- **The "journaled runtime step".** The move runs inside the journaled `cleanup:w<n>` step (`Runtime.Step`) and keeps its own durable record, `<statedir>/retained/<key>.json`, for within-step progress. This is the unit's reading of "a journaled runtime step" (round-2 M-f).
- **Requester decision 5 (2026-10-10):** a load that removes a path from an incomplete unit's `retains` stops with `retains-changed` (see orchestrator).
- **Unit id as a path component.** A unit carrying `retains` must have an id usable as one directory name: no `/`, `:`, `#`, NUL or control character (`:` and `#` break `noteWorktree`'s label parse), not `.` or `..`, no leading `.` or `-`, at most 100 bytes. A load problem; `MoveRetained` re-checks it and requires `filepath.Dir(dest)` to equal the store (critique C1: a re-plan writes unit ids, and U8 pins the store so no re-plan can redirect a move).
- **A failed move is surfaced, not only logged.** A refused or failed move emits a Go-only record kind `retained-move-failed` (non-stopping; ledger and digest) naming the unit, the error, the kept clones' paths and the remedy. A relaunch from an outcome is a new launch whose cleanup covers only its own clones, so a failed move is not retried automatically; only `resume` of the same launch reruns the failed `cleanup:w<n>` step (its intent stays, no record). Requester decision 3 keeps `missing` silent; a failed move is not `missing`.
- **Same-filesystem rename is fsynced too** (critique S4): every directory between a moved path and the store is fsynced before the next record write or source removal.
- **Kill test.** One real SIGKILL test (the runtime's `LSFRT_HELPER` subprocess pattern) mid-copy, plus fault injection at every checkpoint of the move, each followed by a resume.
- **Scope outside the entry:** `lsf-runtime/validate/validate.go` gains one call (the pre-flight must fail on `retains` without `store`). Record as a scope deviation.
- **Discoveries to report:**
  - `phronithm:lsf-drive` presents records from the digest generically. Its stop-kind table needs a `retains-changed` row, and possibly a row for `retained-move-failed`'s by-hand remedy. Both belong to the drive units (`drive-presentation-impl`).
  - `lsf-empirical-work.md` Plan consequences ("until it lands, a `Retains:` line keeps the worktree") becomes stale. That file is outside this unit's scope, so the re-plan should carry the change.
  - A re-plan agent must preserve `retains`. The re-plan prompt and `docs/plan-graph.md` (`empirical-unit-impl`) should say so.
- **Not done here:** `docs/plan-graph.md` (`retains`, `store` documented as Go-only) is `empirical-unit-impl`'s scope. The implement prompt is not changed: the unit's `### entry` already carries its `Retains:` line, and the clone-reset clause (`git clean -fd`, no `-x`) keeps ignored data on a reused clone.

## Success criteria (target rigour)

| # | Criterion | Target |
|---|---|---|
| S1 | `retains` parsed into `plan.Unit.Retains`; absolute, `..`, unclean, duplicate, nested, `.git`-rooted entries fail the load | proven (plan tests) |
| S2 | `store` parsed into `plan.Declaration.Store`; must be absolute and clean, not `/`; a block with `store` alone loads, `Gated()` false, no hosts | proven |
| S3 | `retains` with no `store` fails the load (`loadPlan`) and `validate.Check` (pre-flight) | proven |
| S4 | Store pinned at launch: part of `Identity()`, so a re-plan changing it stops with `declared-gates-changed`; pin checks existing dir outside repo and state dir | proven (orchestrator tests) |
| S5 | At cleanup, a merged unit's declared paths move to `<store>/<unit>/` from the latest writer clone, before `RemoveWorktree`; everything else removed and listed; moved and missing paths in the cleanup value and log | proven |
| S6 | Move refuses a path through a symlink or resolving outside the clone; clones kept | proven |
| S7 | Kill mid-move (real SIGKILL and every injected checkpoint) → resume completes, no stop on its own partial destination, no path lost | proven |
| S8 | Destination existing before the move → move stops for that unit, its clones all kept, step error logged and a `retained-move-failed` record emitted; other units unaffected | proven |
| S9 | Cross-filesystem move: copy verified (hash/size/mode/link) before source removal; a forced verify mismatch keeps the source | proven (forced-EXDEV test always; real cross-fs test where two devices exist, skip otherwise unless `LSF_LIVE_REQUIRE=1`) |
| S10 | Existing cleanup tests (`code-merge-cleanup`) pass; `graphparity` and `promptparity` pass (no change for plans without `retains`/`store`) | proven |
| S13 | A load that removes a path from an incomplete unit's `retains` stops with `retains-changed`; additions do not (requester decision 5) | proven |
| S11 | `cd lsf-runtime && go test -race ./...` passes on a quiet machine (R35) | evidenced (one quiet-machine run) |
| S12 | Design docs amended in the same commit as the behaviour | evidenced (diff review) |

## Candidate approaches (Phase 1) and selection (Phase 2)

**A. Staging directory in the store, atomic final rename, move record in the state dir (selected).** Paths are moved into `<store>/.lsf-retain-<h8>-<journalKey(unit)>.partial/` (`h8` = first 8 hex of sha256 of the absolute state dir at the first attempt), each path landing at its final name only once complete; a cross-filesystem copy is assembled in a sibling scratch dir `<staging minus .partial>.copy/`, never inside staging (critique C2). Then the staging dir is renamed to `<store>/<unit>`. A record `<statedir>/retained/<journalKey(unit)>.json`, written with `writeFileAtomic` (which fsyncs its directory), is the source of truth for the staging and scratch paths and per-path progress (critique S1): phase `moving` before the first mutation, updated after each path, `renaming` just before the final rename, `done` (with `dest`'s device and inode) after. A resume adopts only the staging the record names. Builds on `Runtime.Step`'s intent/rerun, `writeFileAtomic`, `journalKey`.

**B. Move straight into `<store>/<unit>/` with a per-path journal in the state dir.** No staging, but `<store>/<unit>` exists mid-move, so telling own-partial from pre-existing rests entirely on the journal, and a reader of the store sees half-moved units. Rejected: a crash leaves a partial unit directory that looks final, and the existence check is weaker.

**C. Each unit's move as its own `Runtime.Step` (`retain:w<n>:<unit>`) nested in cleanup.** Gives a per-unit journal record, but the step machinery alone cannot distinguish own-partial from foreign destination (the intent says only that an attempt started), so A's record is still needed; nesting steps adds label/replay coupling for no gain. Rejected; A runs inside the existing `cleanup:w<n>` step, whose rerun after a kill calls the idempotent move again.

Stances converge on A: minimal change (one runtime function, one helper factored from `RemoveWorktree`, fields threaded through cleanup) with a clean crash model.

## Design

### plan package

- `Unit.Retains []string \`json:"retains,omitempty"\`` — omitempty keeps the JSON (and graphparity) unchanged for plans without it. `"retains"` added to the unit key allow-list in `plan/graph.go`; validated by `retainsProblems(ref, []string)` (rules in Interpretations).
- `Declaration.Store string \`json:"store,omitempty"\``; `"store"` added to the allow-list in `plan/gates.go`. Rule: string, `filepath.IsAbs`, `filepath.Clean(s) == s`, not `/`. The content rule becomes "`gates`, `hosts`, `store` or a combination"; `setup` still needs `gates`. `Gated()` unchanged.
- `func CheckRetains(p *Plan, d *Declaration) []Problem` — one `invalid-plan` problem per unit with `Retains` when `d == nil || d.Store == ""`, and per unit with `Retains` whose id is not a safe directory name (Interpretations), referent the unit id. Exported `SafeUnitDir(id string) bool` shared with the runtime.
- Divergence entry (next free D number, in `orchestrator/doc.go`'s list, with `plan/doc.go`'s pointer line): `retains` and `store` are Go-only; the frozen JS rejects them as unknown keys (U8 Runner).

### validate

`validate.Check` calls `plan.CheckRetains(g.Plan, decl)` when `g.Plan != nil && len(ps) == 0` (`decl` may be nil: a plan with `retains` and no gates block must fail).

### runtime package (`runtime/retain.go`, new)

```go
// RetainedPath is one declared path's outcome.
type RetainedPath struct {
	Path   string `json:"path"`
	From   string `json:"from,omitempty"`   // source clone label; "" when moved by an earlier attempt
	Method string `json:"method,omitempty"` // "rename" | "copy"
	Files  int    `json:"files,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
}

type RetainedMove struct {
	Unit    string         `json:"unit"`
	Dest    string         `json:"dest"`
	Moved   []RetainedPath `json:"moved"`
	Missing []string       `json:"missing,omitempty"`
}

// MoveRetained moves unit's declared paths from its writer clones (sources,
// oldest first) to <store>/<unit>. Idempotent across kills; see below.
func (rt *Runtime) MoveRetained(ctx context.Context, unit, store, integration string, paths, sources []string) (RetainedMove, error)

// CheckStore checks a declared store at the pin: an existing directory
// neither equal to, under, nor an ancestor of the repo or the state dir.
func (rt *Runtime) CheckStore(store string) error
```

- **Eligibility.** Factor from `RemoveWorktree` a helper `rt.removal(gctx, label, integration) (path, branch, tip string, isClone bool, kept string, err error)` holding the existing checks (both clone and worktree; branch missing; not merged; not a clone on its branch or integration; HEAD ≠ tip). `RemoveWorktree`'s behaviour is unchanged. `MoveRetained` moves only from a clone the helper would remove. If the latest writer clone holding a path is one it would keep, the move errors (step 4) and does not fall back to an older copy. Git in clones goes through `cloneStdout` (guarded-git obligation).
- **Locking/timeouts.** No `wtSem` during the move (it only reads git and touches non-live clones and the store; holding it through a 48 GB copy would block clone creation). Git checks use `gitContext`; the copy uses `killContext` only (no 5-minute bound).
- **Algorithm.**
  1. Check the unit id (Interpretations) and that `filepath.Dir(dest) == store`. Load `rec` if present; an unreadable or corrupt `rec` is a `runtime-error` stop, as `Step` does for its own record (`journal.go:165`). `staging`/`scratch` come from `rec` when it exists, else are computed.
  2. `rec.Phase == "done"` → return `rec.Result`. If it names a `dest` that is now absent or has another device/inode, log a warning (the requester may have moved or archived it), but do not fail: the data already left the clones (round-2 Sig-1).
  3. `dest` exists: if `rec.Phase == "renaming"` and `staging` absent → our rename landed: fsync `store`, write `done`, return. Otherwise → error `"<unit>: destination <dest> exists"` (S8), nothing touched.
  4. **Pre-pass, before any mutation** (critique S2, S3). Store is a directory. For each declared path `p` (sorted):
     - `rec` says moved, or `staging/p` exists (Lstat: a rename landed before its record write) → already moved.
     - `rec` says moved but `staging/p` is absent → error (the staging was disturbed), clones kept.
     - Otherwise find the latest writer clone (eligible or not) holding `p` (Lstat). None → `missing`. If that clone is ineligible (`removal` would keep it) → error naming the clone and its kept reason: no fallback to an older copy, and nothing in the unit moves.
     - Refuse if any component of `p` in that clone, the last included, is a symlink, or the resolved path is outside the clone root (S6).
     - For a source on another device than the store, sum its regular-file bytes. If the copy total exceeds the store's free space (`syscall.Statfs`, `Bavail*Bsize`) → error.

     Any error here returns with nothing mutated.
  5. If there is no `rec`: write `rec{Phase: "moving", staging, scratch}`. Then `Mkdir(staging)` tolerating `EEXIST` (a resume), and fsync `store`. A resume keeps the record's per-path progress; it never rewrites `moving` over it (round-2 Sig-1). A `renaming` record with `staging` present resumes at step 7's rename.
  6. For each path to move:
     - `MkdirAll(staging/dir(p))`, giving intermediate directories the modes of their clone counterparts.
     - Try `renameFn(clone/p, staging/p)` (method `rename`).
     - On `EXDEV`, copy (method `copy`): `RemoveAll(scratch)`; copy the tree to `scratch/x`; verify; `rename(scratch/x, staging/p)`.
     - fsync every directory from `staging/dir(p)` up to `staging`, then update `rec` (path moved, source, method).
     - Only then, for `copy`, `RemoveAll(clone/p)`. A kill before that leaves the source, which goes with the clone at removal (listed). The copy at its final name is verified.
  7. If nothing moved (all missing) → `RemoveAll(staging)`, write `done` with the result and no `dest` (step 2 returns it on any rerun). Else `RemoveAll(scratch)`, write `rec{Phase: "renaming"}`, `rename(staging, dest)`, fsync `store`, write `rec{Phase: "done", dev, ino}`.
  8. The move path has no cleanup `defer`s: an error leaves the on-disk state exactly as a kill at that point would. Any error whose staging is non-empty names the staging dir, the scratch dir and the record file. A cancelled `ctx` (a repeated signal) returns `&StopError{Reason: StopCancelled}`, as `RemoveWorktree` does, so `cleanupStage` stops rather than failing each later unit (round-2 M-b).
  9. A rename failing with `EACCES` or `EPERM` (for example a read-only directory) is an error, not a copy fallback. Same-`st_dev` sources skip the free-space check; an `EXDEV` from them (btrfs subvolumes, bind mounts) falls back to the copy, whose `ENOSPC` is an error with the source intact (round-2 M-d).
- **Copy and verify** (`copyTree`, `verifyTree`):
  - The walk uses `filepath.WalkDir`, which does not follow links.
  - **Directories:** `Mkdir`; the source permission is set after the children are copied. Each directory is fsynced.
  - **Regular files:** `io.Copy` through a sha256 `TeeReader` in 1 MiB chunks, checking `ctx` between chunks; then `File.Sync`, `Chmod`, `Chtimes` (mtime).
  - **Symlinks:** `Readlink` → `Symlink`.
  - **Anything else:** an error.
  - **Verify:** re-walk the copy. For each file, reopen it and compare sha256, size and mode with what the copy recorded. For each link, compare `Readlink`. The set of entries must be equal. Any mismatch is an error and the source is untouched (S9).
  - **Out of scope, stated in Known limits:** ownership (`--userns=keep-id` makes clone files the user's), xattrs, hard links (copied as separate files), sparseness.
- **Cancellation.** The first stop signal waits for the step to finish, because `Step` checks the boundary only at entry, so a long copy delays a graceful stop and the parallel wave gate. A repeated signal cancels `killContext`, and the copy returns between chunks with its scratch dir left over. The resume then removes the scratch dir and copies again.
- **Test seams** (unexported package vars, set only in tests):
  - `renameFn` forces `EXDEV`.
  - `retainHook func(point string)` runs at named checkpoints. A test hook `panic`s with a sentinel that the test recovers. This is valid only because rule 8 forbids cleanup defers: the post-panic disk state is the post-kill state (critique S5). The checkpoints are `after-moving-record`, `after-path-rename`, `mid-copy`, `after-copy-verify`, `after-final-name`, `after-path-record`, `after-source-remove`, `after-renaming-record`, `after-staging-rename` and `before-done-record`.
  - `verifyCorrupt` forces a mismatch.
  - `statfsFn` fakes free space.
  - These seams are reachable only from `lsfrt`'s own tests, so the kill and resume coverage lives entirely in `runtime/retain_test.go`. Orchestrator tests use the stub runtime and cover only the input, the error and stop returns, the kept clones and the record (round-2 Sig-4).
- **Events.** `retained-moved` (unit, dest, paths, missing) and `retained-refused` (unit, error) via `rt.event`.
- `CheckStore(store string) error` on `Runtime`: Stat is a dir; `EvalSymlinks(store)` is neither equal to, under, nor an ancestor of `EvalSymlinks(repo)` or `EvalSymlinks(stateDir)` (critique M6).

### orchestrator

- **Pin** (`loop.go`, `pinGates`): after `pinned`, if `decl.Store != ""` call `l.rt.CheckStore`; failure → `invalid-plan` load stop with the reason. Runs on each launch (resume rechecks).
- **Load** (`loadPlan`): append `plan.CheckRetains(out.plan, l.decl)` problems as stops (the pinned declaration: a re-plan cannot add `retains` without the pinned store).
- **Cleanup input**: `cleanupUnitIn` gains `Retains []string \`json:"retains,omitempty"\``; `cleanupInput` gains `Store string \`json:"store,omitempty"\``. omitempty keeps the step input hash, so existing journals replay. `wave.go` fills `Retains` from `l.plan`'s unit and `Store` from `l.decl`.
- **Ranking**: `writerSources(labels []string) []string` keeps labels whose stage is `plan`, `fix-plan`, `implement`, `fix-code` (the shapes `unit.go:391, 423, 440, 539` produce), ordered by (stage rank, round N, `#n`). It re-derives the chronological order that `wave.go`'s sort for hash stability discards; within one `runUnit` a (stage, round) pair does not repeat, and `#n` only orders the labels of a unit run twice in a launch (critique M1).
- **cleanupStage**: per unit with `Retains`: `rt.MoveRetained(ctx, u.Unit, in.Store, in.IntegrationBranch, u.Retains, writerSources(u.Worktrees))`.
  - A stop returns.
  - An error is logged as `"<label>: <unit>: retained move failed, its clones kept: <err>"` and appended to errs. `RemoveWorktree` is skipped for every label of that unit, and `res.Kept` gains each label with the reason `retained move failed`. The failure (unit, error, clone paths under `<statedir>/clone/`) is noted on `o` for `wave.go`.
  - On success, `res.Retained` gains the move, the moved and missing paths are logged, and the removals run as today.
- `wave.go`: after the parallel gate and cleanup, each noted failure becomes a `retained-move-failed` record in `waveSignals`.
  - **The statement** gives the error, the kept clones (`<statedir>/clone/<key>`), any staging and scratch dirs, the record file and the unit's surviving branches.
  - **The remedy, by hand (the normal route):**
    - put the declared paths at `<store>/<unit>/`, taking each from the staging dir if it is there, else from the latest writer clone;
    - then remove the clones, the staging, scratch and record files, and the unit's `lsf/<plan8>/…` branches.
  - **`resume`** retries only if this launch ends without an outcome (a crash or a runtime stop; `lsf-runtime.md` Resume). An outcome relaunch is a new launch that does not see these clones (round-2 Sig-2).
  - The kind is added to `plan.RecordKinds` and `plan.GoOnlyKinds`, with the `schema.go` comment pointers and the explicit list in `signals_test.go`'s `TestGoOnlyKindsAreKinds`.
- **`retains` pinned against removal** (round-2 Sig-3, requester decision 5).
  - The orchestrator keeps the first load's `retains` per unit as `l.retains`, from the plan at `args.planSha`.
  - Each later load compares every incomplete unit present in both loads. A path the earlier load had and the new one lacks, including emptied `retains`, is a Go-only stop kind `retains-changed` (`StopKinds`, `GoOnlyKinds`). The statement shows the old and new lists.
  - A unit removed from the plan is not compared, since it will not run. Added paths, and new units with `retains`, are accepted and join the baseline.
  - The route is as for `declared-gates-changed`: relaunch at the escalation's plan SHA only on requester approval.
  - Known limit: a change committed between launches is outside the runtime's view, because each launch's baseline is its own `args.planSha`.
- `cleanupResult` gains `Retained []lsfrt.RetainedMove \`json:"retained,omitempty"\``. `wave.go`'s summary log adds the moved/missing counts.
- Divergence entry D-next: the move at cleanup and the `retained-move-failed` record (Go-only; the JS cleanup agent has no store).

### Docs (same commit as the behaviour)

- `lsf-runtime.md` `## Runtime`: Interface gains `MoveRetained`, `CheckStore`; Journal and state dir gains `retained/<key>.json` and the store's `.lsf-retain-*.partial` / `.copy` dirs; Not in this unit loses the `Retains:` line; Known limits gains: verification re-reads through the page cache (detects copy faults, not media faults); ownership, xattrs, hard links and sparseness are not preserved by a copy; an abandoned staging or scratch dir is left in the store for the requester (not reported by `status`); a tracked file under a retained path is moved and also appears in the removal listing as a deleted tracked file; a failed move is retried only by `resume` of the same launch.
- `lsf-workflow.md` wave loop, cleanup paragraph: replace "until then nothing is kept" with the move: declared paths to `<store>/<unit>/` before removal, latest writer clone, missing listed, failed move keeps the unit's clones.

### Risks and unknowns

- **Data loss between move and removal (the entry's risk).** Mitigated by ordering (source removed only after the path is at its final staging name, verified when copied), the move failing closed (error → all the unit's clones kept), and the kill/injection tests. Resolved during implementation by S7–S9.
- **`EXDEV` detection**: `os.Rename` returns `*os.LinkError` wrapping `syscall.EXDEV`; check with `errors.Is(err, syscall.EXDEV)`. Verify by the real cross-fs test (`/dev/shm` vs `t.TempDir()` are distinct tmpfs mounts here; candidates `/dev/shm`, `/var/tmp`, `os.UserCacheDir()`; first with a different `st_dev`).
- **Rename of a non-empty dir onto an existing `dest`** fails (`EEXIST`/`ENOTEMPTY`): a foreign `dest` appearing between check and rename leaves data in `staging`; the error names it. Accepted.
- No third-party APIs; stdlib only. API verification: `os.Rename`/`EXDEV` and `WalkDir` non-following behaviour checked by tests in step 2.

## Plan critique record (critique gate, critique-design, sonnet)

- **Round 1** found 2 Critical, 6 Significant and 8 Minor findings.
  - Critical: C1, an unvalidated unit id used as a path component; C2, the scratch `.tmp` inside staging.
  - Significant: S1, misclassified missing; S2, mutation before refusal; S3, stale fallback; S4, fsync ordering; S5, error-return injection; S6, failure only logged.
  - All were fixed in the plan.
- **Round 2** found no Critical and converged. Its four Significant findings were fixed in the plan:
  - resume logic in steps 2, 5 and 7;
  - the remedy text and the claim about `resume`;
  - silent dropping of `retains` (requester decision 5);
  - orchestrator-level seams and missing files.
  - Its Minor findings M-a to M-f were also fixed.
- The API spec / code block check passes: the runtime block declares every exported runtime symbol the prose names, `MoveRetained`, `CheckStore`, `RetainedMove` and `RetainedPath`. The plan symbols have no code block.

## Implementation steps

Each step: `go test -race ./...` in `lsf-runtime`, `gofmt`, commit, then a `phronithm:critic` (critique-code, sonnet) on the step's diff, running while the next step proceeds.

The branch is unreleased between steps: after step 1 a plan with `retains` passes the pre-flight but nothing moves it, until step 3. The design-of-record edits (both docs) land in step 3's commit, where the behaviour becomes reachable end to end (critique M3).

1. **plan: `retains`, `store`, `CheckRetains`, `SafeUnitDir`; validate wiring.**
   - Files: `plan/graph.go`, `plan/gates.go`, `plan/graph_test.go`, `plan/gates_test.go`, `validate/validate.go` and its test.
   - Acceptance: S1–S3 tests:
     - each path-rejection rule, including `.tmp` accepted as an ordinary name;
     - unit-id safety;
     - `store` alone loads, `Gated()` is false and `Hosts` is nil;
     - `CheckRetains` hit and miss, with no gates block and with a block lacking `store`;
     - `validate.Check` reports it.
   - graphparity is unchanged.
2. **runtime: `removal` helper, `MoveRetained`, `CheckStore`.**
   - Files: `runtime/worktree.go` (factor), `runtime/retain.go`, `runtime/retain_test.go`, `runtime/supervise_test.go` (helper scenario).
   - **Moves and copies:**
     - a same-fs rename;
     - a forced-EXDEV copy of a tree with files, nested dirs, a symlink, modes and mtimes;
     - a real cross-fs move, recording the device pair (`st_dev` of both) in the test log;
     - a verify mismatch keeps the source;
     - the ordering assertion: at `after-copy-verify` the source exists, and after `after-source-remove` it does not;
     - a declared path named `.tmp` survives a copy-method move of another path (C2).
   - **Refusals:**
     - a special file;
     - a symlinked path or parent;
     - a path resolving outside the clone;
     - an unsafe unit id;
     - insufficient free space (via a `statfsFn` seam).
     - Each leaves every clone byte-identical (pre-pass, S2).
   - **Source selection:**
     - the latest writer clone is chosen;
     - an ineligible latest holder errors with no fallback (S3);
     - missing paths are reported;
     - all-missing creates no dest;
     - a pre-existing dest errors with the sources intact;
     - a read-only source directory gives `EACCES`, an error with no copy fallback;
     - a cancelled ctx mid-copy returns `StopCancelled`, and the next call completes.
   - **Kills and resumes:**
     - a panic injected at each checkpoint, then a second call, completes with every path present at `dest` exactly once and no error; this includes the all-missing branch and a resume after a `moving` record with some paths moved, which keeps their progress;
     - a corrupt record gives a `runtime-error` stop;
     - a `done` record whose `dest` was moved away returns the result with a warning;
     - a staging dir renamed away after a moved record errors (S1);
     - a real SIGKILL of the `LSFRT_HELPER` subprocess at `mid-copy` (the helper sets `renameFn` to force EXDEV and blocks on its hook after writing a ready file), then an in-process resume, completes.
   - `CheckStore` cases cover missing, file, under repo, ancestor of repo, under state dir, and fine.
   - The existing `RemoveWorktree` tests are unchanged.
3. **orchestrator: pin check, load check, cleanup move, record kind; design docs.**
   - Files:
     - orchestrator: `loop.go`, `wave.go` (including the stale "hold nothing unmerged" comment), `cleanup.go` (including the header comment), `results.go`, `runtime.go` (the runtime interface gains `MoveRetained` and `CheckStore`), `orchtest/orchtest.go` (the stub), `doc.go`;
     - plan: `doc.go` (pointer), `schema.go` (the record and stop kinds), `signals_test.go`;
     - tests: `cleanup_test.go`, `run_test.go`;
     - docs: `docs/design/lsf-runtime.md` (Runtime), `docs/design/lsf-workflow.md` (wave loop; "A failure is logged, not signalled" becomes a statement of the record).
   - Further test: a re-plan removing an incomplete unit's retains path gives a `retains-changed` stop; adding a path does not.
   - Load and pin tests:
     - a store that is missing, inside the repo, an ancestor of the repo, or inside the state dir gives a load stop;
     - a re-plan changing `store` gives `declared-gates-changed`;
     - a re-plan adding `retains` without a store gives a load stop.
   - Cleanup tests:
     - the cleanup input carries `retains`/`store`, and its JSON is unchanged without them;
     - `writerSources` orders correctly (r2 < r10, `#n`);
     - cleanup moves, then removes, and its value lists moved, missing and removed paths;
     - a failed move keeps that unit's clones, not another unit's, and emits `retained-move-failed` with the clone paths;
     - the stub's `MoveRetained` returns a stop, and cleanup stops without failing later units;
     - the stub returns an error, the step fails, and a rerun of the same launch's step calls the move again.
   - The existing cleanup tests, graphparity and promptparity pass.
4. **Phase 5**: full-diff `phronithm:review` (all applicable lenses, static-analysis pre-scan) and critique gate; quiet-machine `go test -race ./...` (R35, record load average); implementation-summary; completion report; commit.

Steps are sequential (2 depends on nothing from 1 but 3 depends on both; 1 and 2 could run in parallel but share no files — sequential keeps one working tree).

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/retained-artefacts-impl-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit retained-artefacts-impl of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim (Phase 4 re-verifies the external gates against these
pins before implementing, recording the outcome in the implementation-summary):
see "Unit entry (verbatim)" above — scope, publishes, depends-on, external gates
(`lsf-toolchain`: Go 1.27.1, pinned in `lsf-runtime/go.mod`), done condition —
and "Standing obligations naming this unit" above.
After Phase 5, produce the completion-report (contents per the
phronithm:feature skill's Produces) at docs/plans/retained-artefacts-impl-completion.md,
then present this continuation prompt to the requester:
Run `/phronithm:breakdown` in re-plan mode with docs/plans/lsf-improvements-breakdown.md and docs/plans/retained-artefacts-impl-completion.md before starting any dependent unit.
