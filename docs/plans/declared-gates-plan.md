# declared-gates: plan

Work unit `declared-gates` of `docs/plans/lsf-improvements-breakdown.md`. Base commit `98e2d66`, branch `lsf-workflow`. Phases 1–3 ran compressed in one plan-mode session. On approval, copy this file to `docs/plans/declared-gates-plan.md` and commit it as `declared-gates: plan`.

## Context

R11: a unit stage that reports `green` for a gate it did not run passes every check, because the only gate results are the ones the stage reports. R12: a gate longer than the Bash tool's 600 s limit can only be run through the detached-process recipe, and if the harness kills it the gate stops as `gate-not-run`. Under this unit the runtime runs a project's declared gate commands itself, on a clean checkout, with no tool limit. Reviewers, the completion report and the merge read the runtime's results, not the stage's.

Requester decisions (2026-10-07, this session):

1. **Declaration:** a separate `yaml lsf-gates` block in the plan document. The frozen JS parser ignores it, so the Workflow script is unaffected. The runtime pins it at launch, and a change between loads within a launch stops the run.
2. **Run once:** gates are not repeated over an identical input. Implement and fix stages no longer run the declared gates at their final HEAD; the runtime's run is the only one. Results are keyed on the checked-out tree plus the declaration, and journaled, so they survive a resume.
3. **Disagreement:** where a stage still reports a declared gate, the runtime's result supersedes it, and a difference is recorded as a `gate-misreported` row.
4. **Not-run:** a declared gate the stage reports `not-run` does not stop the run; the runtime's result stands. `gate-not-run` stops only when the runtime itself could not complete a gate (timeout, setup failure, checkout failure).
5. **Timeout:** a declaration may give a timeout per gate. Otherwise the runtime config's `gateTimeout` applies, default 2h. A gate killed at its timeout is `not-run`.
6. **Deviation accepted:** because stages are no longer asked to report declared gates, `gate-omitted` is not raised for them. A plan without the block keeps today's behaviour, `gate-omitted` included.
7. **Merge reuse:** a merge whose tree differs from the unit's last gated tree only at the unit's completion-report path reuses that result.
8. **Gate before commit:** merge gates run on the merge's tree before anything is committed. A red tree is `regressed`, and nothing is committed (a resolved merge in progress is aborted). This replaces the entry's commit-then-undo. The done condition's kill test becomes a kill mid-gate: the resume neither adopts nor merges twice.

## Success criteria

Target rigour is `proven` (an automated test) for all criteria except S9.

- **S1** — The `lsf-gates` block parses. A missing block means "no declaration". Malformed blocks (two blocks, a missing or duplicate name, an empty command, a bad timeout, an unknown key) become `invalid-plan` load stops. *proven*
- **S2** — The runtime runs the declared gates on a clean detached checkout of a given commit:
  - setup first, then each gate in order, through `sh -c` in its own process group;
  - exit 0 is `green` and anything else `red`; a timeout is `not-run`; a failed setup makes every gate `not-run`;
  - a stop kills the process group;
  - the checkout is removed afterwards, and a full log is kept under the state dir.

  *proven*
- **S3** — Gates run after implement (non-design units), after each code fix, and before each merge commit. Reviewers read the runtime's results. *proven*
- **S4** — A stage that reports `green` for a declared gate the runtime found `red` gets a `gate-misreported` record, and the reviewers see `red`. A stage that omits a declared gate, or reports it `not-run`, is not stopped. A runtime `not-run` stops as `gate-not-run`. *proven*
- **S5** — An identical tree is gated once per launch (memo) and across launches (journal replay). The merge-reuse rule (decision 7) holds. *proven*
- **S6** — A merge whose gates fail leaves nothing committed: a clean merge is never started, and a resolved one is aborted. The unit is `regressed`, which is a stop, and later units still merge. *proven*
- **S7** — Merge-gate resume:
  - **Invariant:** for both a clean and a resolved merge, a resume makes at most one merge per unit, never adopts a regressed merge, and never commits an ungated tree.
  - **Kill during the gate:** the gate reruns once on resume; the killed run is not journaled, so it is not counted.
  - **Kill after the gate record and before the merge:** the gate result replays, and no gate is re-run.
  - **Layout `[regressed A, resolved B killed mid-gate]`:** resumes without wedging.

  *proven*
- **S8** — A re-plan that changes the `lsf-gates` block stops the run with `declared-gates-changed`. *proven*
- **S9** — `phronithm:breakdown` emits the block, `docs/plan-graph.md` documents it, and `lsf-workflow.md::lifecycle-mapping` (with the merge paragraph and the stop table) describes the new behaviour. *evidenced*, by reading: these are prose deliverables.
- **S10** — Plans without the block behave as before:
  - every existing test passes;
  - `tools/promptparity` and `tools/graphparity` are byte-identical, so no scenario changes;
  - `go test -race ./...` is green.

  *proven*

## Exploration findings

- **Gate handling today:**
  - `orchestrator/unit.go`: `runUnit`, `gateSignals`/`omittedGates`/`notRunGates` (D22), and the review loop's `gatesRun` and `onFix`.
  - `orchestrator/prompts.go`: `implementPrompt`, `fixTail`, `longGates`, `gateResults`, `renderGates`.
  - `orchestrator/report.go`: "Gates at HEAD".
  - `orchestrator/schemas.go`: `gatesField`, which stays as it is.
- **Merge:** `orchestrator/merge.go` `mergeOne`:
  - `expectedStaged` already returns git's merge tree for `pre`+`sha`;
  - `verifyMerge` checks a clean merge's commit tree against it;
  - after the conflict call, `stagedOK` checks the staged resolution, then the code commits it;
  - `abortOwn` aborts a merge in progress;
  - `adoptMerge` adopts by trailers.

  `plan/signals.go` `SignalsFromMerge` maps `regressed` to the `merge-regression` stop (statement "was clean but the test suite failed in …; merge undone"). `wave.go:121` builds `mergeUnitIn` from `unitResult`.
- **Plan load:** `orchestrator/loop.go` `loadPlan` runs the journaled `load` step, which returns the whole plan text as `PlanBlock`, then calls `plan.LoadGraph`. The declaration parse goes beside `LoadGraph`, not inside it, so the `Graph` JSON and graphparity are unchanged. `plan.ExtractBlock` returns a problem when a block is absent; the declaration treats zero matches as "none".
- **Journal:** `rt.Step` replays on the same label and input hash. `o.runStep` makes labels unique per launch (`#n`), so the tree memo is per launch and the journal label is `gates:<tree[:12]>`, with input `{tree, declaration}`. The commit is not in the input, so the same tree under another commit replays. A nested `rt.Step` inside the merge step's `fn` takes no lock (`journal.go` Step), but step 3 verifies this with a test.
- **Runtime:**
  - `runtime/call.go` `spawn` is the process-group, SIGTERM→SIGKILL pattern to reuse, and `addChild`/`setChildPid` register children for orphan reaping;
  - `runtime/worktree.go` holds the worktree and `gitContext` helpers;
  - `runtime/config.go` `Config` is strict JSON (unknown fields are errors).
- **YAML subset** (`plan/yaml.go`): plain and quoted scalars. A command containing `: ` or `#` must be quoted; `docs/plan-graph.md` says so.
- **Interfaces:** the orchestrator's `Runtime` interface (`orchestrator/runtime.go`) and `orchestrator/orchtest` `Stub` both need the new method.
- **Divergence numbering:** the last divergence is D25 (`orchestrator/doc.go`). New entries are D26 onward.

## Design

### Declaration (`plan` package, new file `plan/gates.go`)

```yaml lsf-gates
setup: "npm ci"                 # optional; run once per checkout before the gates
gates:
  - name: tests
    command: "npm test"
    timeout: 45m                # optional Go duration; default runtime gateTimeout
  - name: lint
    command: "npm run lint"
```

```go
const GatesTag = "lsf-gates"
type DeclaredGate struct { Name, Command string; Timeout string `json:"timeout,omitempty"` }
type Declaration struct { Setup string `json:"setup,omitempty"`; Gates []DeclaredGate `json:"gates"` }
// ParseDeclaration: nil, nil when the document has no lsf-gates block.
func ParseDeclaration(markdown string) (*Declaration, []Problem)
```

- **Problems** (kind `invalid-plan`): more than one block; a top-level key other than `setup` or `gates`; `gates` empty or not a sequence; a gate key other than `name`, `command` or `timeout`; a missing or duplicate name; an empty command; a `timeout` that `time.ParseDuration` rejects or that is ≤ 0.
- **Identity:** the declaration's identity is its `MarshalJS` JSON.
- **Stop kinds:** `declared-gates-changed` joins `StopKinds` as a Go-only kind (as `gate-omitted` did). `gate-misreported` joins `RecordKinds`. graphparity's `goOnly` already filters Go-only kinds.

### Runtime gate runner (`runtime/gates.go`)

```go
type GateSpec struct { Name, Command string; Timeout time.Duration }
type GateRun struct {
    Name, Command, Status string  // green | red | not-run
    ExitCode int; Summary string   // last non-blank output line, or why not run
    Tail string `json:",omitempty"` // last 4 KB of output, red/not-run only
    Log string; Secs float64
}
// RunGates checks out commit detached at <stateDir>/gates/<key> (a leftover
// directory is force-removed first), runs setup then each gate, removes the
// checkout, and returns one GateRun per gate. A stop or cancellation kills the
// running group and is returned as the stop error; nothing is returned.
func (rt *Runtime) RunGates(ctx context.Context, label, commit, setup string, gates []GateSpec) ([]GateRun, error)
```

- **Config:**
  - `Config.GateTimeout Duration` (`gateTimeout`, default 2h; 0 means the default, not "none").
  - `Config.GateConcurrency int` (default 1).
  - Gate time is wall-clock, so it counts toward `Caps.MaxWallClock`.
- **Checkout path:** each run uses `<stateDir>/gates/<label key>`. A leftover directory is removed with `git worktree remove --force`, then `git worktree prune`, before `git worktree add --detach` (critique finding 14).
- **Orphan reaping:** each gate runs as `sh -c <cmd> lsf-gate:<session>`, which makes `$0` a marker. `reapOrphans` (`runtime/children.go`) is extended to match that marker alongside `--session-id`, and a test covers it. A SIGKILLed launch's gates are then reaped on relaunch (critique finding 6).
- **Process handling:** each command runs as `sh -c <cmd>` with `Setpgid`, cwd at the checkout, the runtime's environment, stdin `/dev/null`, and combined output to `<stateDir>/gates/logs/<key>-<n>.log`. The timeout and stop path is `spawn`'s: SIGTERM to the group, `KillGrace`, SIGKILL. The group is registered for orphan reaping as `spawn` registers its child.
- **Setup failure:** every gate is `not-run`, with summary `setup failed (exit N): <last line>`.
- **Checkout failure:** an error (the orchestrator turns it into a stage fault).
- **Not journaled here:** the orchestrator journals.
- **Orchestrator interface:** the `Runtime` interface gains `RunGates`. The `orchtest.Stub` gains a scripted gate runner: per-commit or per-tree results, with a hook that can stop mid-run.

### Orchestrator: one gate run per tree (`orchestrator/gates.go`)

- **Launch baseline:** `loadPlan` calls `plan.ParseDeclaration(cp.Result.PlanBlock)`. The first load fixes `l.decl` (problems are load stops). Each later load compares its declaration with the baseline; a difference is a `declared-gates-changed` stop at stage `load`, with both forms in the statement.
- **Gate run:** `o.gates(ctx, commit) (gateSet, fault string, err error)`:
  1. Resolve the tree: `git rev-parse <commit>^{tree}`.
  2. Check the memo `o.gateMemo[tree]`. A per-tree single-flight means concurrent units with the same tree wait on one run.
  3. Otherwise run the step `gates:<tree[:12]>` with input `{tree, declaration}`; its `fn` calls `rt.RunGates(ctx, label, commit, …)`.

  `gateSet` is `{tree, runs []lsfrt.GateRun}`. The helper `gateSet.failed()` lists the gates not `green`.
- **What is journaled:** only a set whose gates are all `green` or `red`. A set with any `not-run` returns `errNotJournaled`, the `journaledTip` pattern (`unit.go:238-273`). The relaunch after the requester fixes the environment then re-runs it, rather than replaying the failure (critique finding 3).
- **Concurrency:** `runUnit` runs under `rt.Parallel`, so gate runs are bounded by a runtime semaphore, `Config.GateConcurrency` (default 1, meaning serialised). Full suites in parallel compete for CPU, ports and caches.
- **Not-run statements:** the runtime's own `not-run` produces `gate-not-run` statements, through the existing `gateSignals` shape with a new `by runtime` statement.

### Unit stages (`unit.go`, `prompts.go`, `report.go`)

Declared mode is `o.decl != nil`. Undeclared code paths are untouched.

- **Implement prompt:** the "Run the project's gates … " + `longGates` sentence is replaced by a sentence naming the declared gates. It tells the stage:
  - the runtime runs these gates on a clean checkout of the commit it returns, so they see only committed files;
  - not to run them as a final check (tests while working are its own business);
  - to return `gates` as an empty array.
- **Fix prompts:** `fixTail` gets the same replacement.
- **After implement** (non-design units, after the drift, branch and `hasStop` checks): `o.gates(ctx, impl.Result.Sha)`.
  - A fault is a stage-fault stop at `implement`.
  - Any runtime `not-run` is a `gate-not-run` stop.
  - Each stage-reported gate whose command equals a declared command (trimmed) and whose status differs is a `gate-misreported` record (stage, command, reported → runtime).
  - Stage reports of undeclared commands are ignored.

  The stage-side `gateSignals` call is skipped in declared mode.
- **After each code fix** (`onFix`): the same, at stage `fix`, on `fixed.Result.Sha`. `critiqueLoop`'s `onFix` becomes `func(fixed, n) ([]*plan.Signal, error)`, so a stop or cancellation inside the gate run propagates. The plan gate's `onFix` returns a nil error, and undeclared behaviour is unchanged.
- **Reviewers:** `gatesRun{by: "The runtime, on a clean checkout of this branch's HEAD,", gates: …}`. `renderGates` gains each red or `not-run` gate's `Tail` in a fenced block under its line. Its input becomes the orchestrator's `gateRun` plus an optional tail, so undeclared rendering is unchanged.
- **Completion report:** "Gates at HEAD" renders the runtime's last results. The verdict suffix reads "…the gates below are the runtime's at the last fix" when a fix ran.
- **`unitResult.gatedTree`** is set to the last gated tree, and flows through `wave.go` into `mergeUnitIn.GatedTree`. This is a new journaled input field (`omitempty`, so undeclared inputs hash as before).

### Merge gates before commit (`merge.go`)

This applies in declared mode only. `mergeInput` gains `Declared bool` (`omitempty`).

- **Clean merge** (`!resume`, no conflicts per `expectedStaged`): before `git merge`, gate `tree`.
  - **Reuse:** when `u.GatedTree` is set, and `git diff-tree -r --name-only u.GatedTree tree` lists nothing or only `u.Report`, and the memo or journal holds `u.GatedTree`'s result, use that result. On a miss, fall through to a normal run (critique finding 11).
  - **Otherwise** create a candidate commit with `git commit-tree tree -p pre -p u.Sha -m <merge message>`. This commit is dangling and is not on any ref. Gate it with `o.gates`.
  - **After the gate:** re-read HEAD. If it is no longer `pre` (the requester committed during the gate), recompute `expectedStaged` against the new HEAD and gate again. On a second move, fault: "the integration branch moved during the gate; relaunch". `git merge` therefore never runs on an ungated `pre` (critique finding 1).
  - **Green:** `git merge` as today. `verifyMerge` already checks the commit's tree equals `tree` (merge.go:211-217).
  - **Red:** return `Status "regressed"`, `Files` = the failed gates as `` `name`: status — summary ``. Nothing was started. Continue with the next unit, as the JS does.
  - **Any `not-run`:** nothing is merged, and the merge reports the unit as `regressed`. The orchestrator then rewrites that unit's signal to a `gate-not-run` stop, as decision 4 requires (critique finding 10).
- **Conflicted merge:** after `stagedOK` accepts the resolution, run `git write-tree` (the index holds the resolution), then commit-tree and gate it the same way.
  - **Green:** run `git write-tree` again just before `git commit` and require the same tree, then commit as today. After the commit, check that `HEAD^{tree}` equals the gated tree; a mismatch is a fault, as `verifyMerge`'s are (critique finding 9).
  - **Red:** `abortOwn(pre, staged)`, then `regressed`.
- **Durable non-merge outcomes:** each unit that ends without a merge (`regressed`, `unresolvable`, refused) is recorded as a nested step `merge-outcome:<label>:<unit>`, with input `{unit, sha}` and the `plan.Merge` as its value.
  - On resume, a unit before the in-progress index (`mustAdopt`) with no merge to adopt replays its outcome record instead of failing at merge.go:615-617 (critique finding 2).
  - The replay uses the `errNotJournaled` probe pattern: with no record, the existing error stands.
  - This also fixes the same wedge for `unresolvable` units, which exists today; it is recorded as a discovery.
- **Resume path** (`resume`, own merge in progress): it reaches the same gate before commit. The gate result replays from the journal.
- **Kill mid-gate** (S7):
  - *Clean:* nothing is in the checkout. The rerun's `adoptMerge` finds nothing, `merge-base --is-ancestor` is false, and the gate replays (or reruns, if the kill preceded the record). Then it merges or regresses: one merge at most.
  - *Resolved:* the merge is in progress, so `ownMergeInProgress` resumes it. The conflict call replays from the journal, the gate replays, and the merge commits or aborts.

  A regressed merge never has a commit to adopt.
- **Design units:** these run no unit-level gates, so their merges always run the full gates. This is intended, because documentation can break doc tests and lints.
- **Regressed statement:** `plan.SignalsFromMerge` is unchanged, so graphparity holds (critique finding 5). In declared mode the orchestrator rewrites the statement of each `merge-regression` signal to "merge of X failed its gates (…); nothing committed". Undeclared Go never produces `regressed`. This is recorded as D26.

### Re-plan prompts

In declared mode only, the re-plan and decision re-plan prompts get one added sentence: "Do not edit the plan's ```yaml lsf-gates block; a change to it stops the run." (Undeclared prompts are unchanged, so promptparity holds.)

### Rejected

- **A key in the `lsf-plan` block:** the JS rejects unknown keys.
- **Runtime config or a repo file:** breakdown could not emit runtime config, and units could edit a repo file.
- **Commit-then-undo:** it leaves a kill window and rewrites the shared branch.
- **Reusing one gate worktree per launch:** that is not a clean checkout.
- **Keeping stage gate runs:** they duplicate the runtime's (decision 2).

### Risks

| Risk | Resolution | When |
|---|---|---|
| Clean checkouts lose in-tree build caches (`target/`, `node_modules`), so every gate run is a cold build | `setup` covers dependency install; cost is a discovery for `long-running-design`/re-plan | after (completion report) |
| `commit-tree` candidate commits are dangling objects | gc removes them; no ref names them | accepted |
| Nested `rt.Step` inside the merge step | step 3 test exercises it before merge work | during |
| The stub's `Step`/kill semantics differ from the real runtime's for the kill tests | S7 tests use the stub's existing kill/relaunch harness (as `merge_test.go` does); one real-runtime test of `RunGates` kill in step 2 | during |
| A gate that reads `docs/plans/<unit>-completion.md` makes merge reuse unsound | requester accepted (decision 7); documented in plan-graph.md | accepted |
| Merge reuse hits about once per wave: later units merge onto a moved tree, so most merges run a second full gate | recorded as a cost discovery in the completion report | after |

## Steps

Each step ends green on `cd lsf-runtime && go test -race ./...`. It is committed as `declared-gates step N: …`, gets the critique gate (critique-code) run on its commit, and any fixes are committed as `declared-gates step N: critique fixes`.

1. **Declaration parse.**
   - Work: `plan/gates.go` (`ParseDeclaration`, types); `declared-gates-changed` in `StopKinds`, `gate-misreported` in `RecordKinds`; tests for every problem in the Design list, an absent block, and a round trip.
   - Acceptance: S1 tests pass, and graphparity is unchanged.
2. **Runtime gate runner.**
   - Work: `runtime/gates.go` `RunGates` and `Config.GateTimeout`.
   - Tests use a real git repo and `sh`:
     - green/red/exit code, timeout→`not-run`, setup failure;
     - untracked and ignored files from a gate are gone after the run, and a leftover directory is replaced;
     - a stop mid-gate kills a TERM-ignoring grandchild;
     - `reapOrphans` matches the `lsf-gate:` marker;
     - a stale worktree registration is pruned;
     - `GateConcurrency` bounds concurrent runs.
   - Acceptance: S2.
3. **Orchestrator plumbing.**
   - Work: the `Runtime` interface method, the `orchtest.Stub` scripted runner, `loadPlan` baseline and change check, `o.gates` memo and journal step.
   - Tests:
     - a change across loads gives a `declared-gates-changed` stop (S8);
     - the same tree twice, concurrently and serially, is one run in a launch and replays on relaunch (S5, first half);
     - a set with a `not-run` is not journaled and reruns on relaunch;
     - a nested step inside a step journals and replays, including after a kill that leaves the inner step's intent.
   - Acceptance: S5 (first half), S8.
4. **Unit stages.**
   - Work: prompts, the runtime gate runs after implement and fixes, `gate-misreported`, runtime `not-run` stops, reviewers' and report rendering, `gatedTree`.
   - Tests:
     - stage green / runtime red gives a record and reviewers see red with the tail;
     - a stage omitting a declared gate or reporting it `not-run` gets no stop;
     - runtime `not-run` stops;
     - a design unit runs no unit-level gates;
     - an undeclared plan's prompts are byte-identical (promptparity).
   - Acceptance: S3 (unit part), S4, S10.
5. **Merge gates before commit.**
   - Work: clean and resolved paths, reuse, `regressed`, D26.
   - Tests, against real git as in `merge_test.go`:
     - green merges; red clean merge: nothing committed, HEAD unchanged, unit regressed, next unit merged;
     - red resolved merge: aborted, the checkout's other changes intact;
     - reuse when only the report path differs, and no reuse when the integration branch moved;
     - kill during the gate (the gate reruns once) and kill after the gate record (it replays), each for clean and resolved: at most one merge, and no adoption of a regressed merge;
     - `[regressed A, resolved B killed]` resumes, and A replays its outcome record;
     - HEAD moves during the gate: the new tree is re-gated, and the merge commit's tree is the gated one;
     - a merge-gate `not-run` stops as `gate-not-run` with nothing committed;
     - an `unresolvable` A before an in-progress B resumes (the existing wedge).
   - Acceptance: S3 (merge part), S5 (reuse), S6, S7.
6. **Docs and skill.**
   - `docs/plan-graph.md`: a `## Gate block` section, with quoting, the merge-reuse caveat and "the Workflow script ignores it".
   - `skills/breakdown/SKILL.md::graph`: emit the block from the project's own gate commands (CLAUDE.md, CI config), asking the requester when they are unclear; omit it when the project has no gates; and an Interface/Produces mention.
   - `docs/design/lsf-workflow.md`: lifecycle-mapping stages 3–5, the merge paragraph, the stop table (`declared-gates-changed`, whose relaunch takes the changed block as the new baseline; `gate-misreported` among records), and the Constraints bullet on `git reset --merge`, as Workflow-script-only.
   - `orchestrator/doc.go`: D26 (regressed statement), D27 (declared mode: stage gates superseded, no `gate-omitted`), and an update to D22's and D24's "until declared-gates" text.
   - `lsf-runtime.md`'s config section, if it lists config fields.
   - `CLAUDE.md`, if the `lsf-runtime/` entry needs it.
   - Acceptance: S9; the design-of-record obligation, since the `lsf-workflow.md` amendment is in this step's commit.

Ordering: 1 → 2 → 3 → 4 → 5 → 6. Steps 1 and 2 are independent, but they are kept serial for commit clarity.

## Verification

- `cd lsf-runtime && go test -race ./...`, with node present: graphparity and promptparity run, and both must report identical.
- `PROMPTPARITY_REQUIRE_NODE=1 GRAPHPARITY_REQUIRE_NODE=1 go test ./tools/...`.
- Read through `docs/plan-graph.md` and the breakdown skill against a sample block, by hand: parse the doc's own example in a test (step 1 adds a test that extracts the example from `docs/plan-graph.md` once it exists, in step 6).
- `tools/fixturerun` (paid) is not required. The spike fixture has no declaration, so it would exercise only S10.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/declared-gates-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit declared-gates of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim (Phase 4 re-verifies the external gates against these
pins before implementing, recording the outcome in the implementation-summary):

### declared-gates
- **Type**: feature
- **Risk**: medium — a plan-format change reaches every plan
- **Clarity**: exploratory — one open decision: whether gate commands are declared in the plan block or in project configuration
- **Iteration**: write-once
- **Effort**: medium
- **Scope**: `lsf-runtime/plan`, `lsf-runtime/runtime`, `docs/plan-graph.md`, `skills/breakdown/SKILL.md::graph`, `docs/design/lsf-workflow.md::lifecycle-mapping`
- **Publishes**: the declared gate-command field
- **Depends on**: code-merge-cleanup (logical, serialises the runtime files; the code merge after which gates run); runtime-signals (logical, the omitted-gate signal)
- **External gates**: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`)
- **Done condition**: A project's gate commands are declared where the unit decides. The runtime runs them on a clean checkout of the returned commit after implement, after each code fix and after each merge, and the reviewers read its results. Tests show a stage that reports `green` for a gate it did not run, or omits one, is caught, an omitted declared gate as `gate-omitted`. A merge whose gates fail stops the run with the unit regressed and the merge undone, and a test kills the run between the merge commit and its undo and shows the resume neither adopts the regressed merge nor merges twice. `phronithm:breakdown` emits the declaration. The tests pass.
- **Description**: Closes R11 (results, check 10b). Gates after a merge run in `mergeOne`, after a clean or resolved merge commit is verified. Adoption keys on the merge commit's `Lsf-Unit` and `Lsf-Step` trailers, so a merge commit that reaches the integration branch before its gates run is adopted by an interrupted rerun unless the undo prevents it.

Standing obligations naming this unit:
- **Register dispositions** (every unit): the completion report names the register rows it disposes of. None are assigned to declared-gates; it closes risks R11 and R12.
- **Design of record**: `lsf-workflow.md::lifecycle-mapping` is in Scope, so the amendment lands in the same commit as the behaviour it describes (step 6, the last behavioural step, carries it; if the reviewer wants it per step, amend per step).

Deviations already agreed (record in the completion report):
- Gates run before the merge commit, not after it, so there is nothing to undo. The kill test is a kill mid-gate (decision 8).
- Declared gates are not stage-reported, so `gate-omitted` is not raised for a declared gate. Plans without a declaration keep it (decision 6).
- `lsf-runtime/orchestrator` is written outside the Scope line (the gates run in `runUnit` and `mergeOne`).
- The runtime config gains `gateTimeout` and `gateConcurrency`.
- `merge-outcome` records also fix the existing resume wedge for an `unresolvable` unit before an in-progress merge.

Plan critique (round 1, phronithm:critic, sonnet): 0 Critical, 8 Significant, 8 Minor.
- Significant 1–8 and Minor 9–11 and 14–16 are folded into the Design and Steps above.
- Minor 12 is added to Risks.
- Minor 13 is answered: design units' merges are gated, which is intended.

After Phase 5, produce the completion-report (contents per the
phronithm:feature skill's Produces) at docs/plans/declared-gates-completion.md,
then present this continuation prompt to the requester:
Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
