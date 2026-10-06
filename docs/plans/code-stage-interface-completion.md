# code-stage-interface — completion report

**Unit:** `code-stage-interface` of `docs/plans/lsf-improvements-breakdown.md` @ `492b524`.
**Branch:** `lsf-workflow`; commits `6a58ac4..f9a842e`.
**Other documents:**
- plan: `docs/plans/code-stage-interface-plan.md`
- implementation summary: `docs/plans/code-stage-interface-implementation.md`

## Done-condition verdict

**Met, at `proven`.** Every clause is held by tests that run under `go test -race ./...`.

- **Four stages as code.** Load, decision log, ledger append and completion report run as journaled code steps through `orchestrator.Runtime.Step` (`orchestrator/stages.go`, `git.go`). The orchestrator has no prompt builder or schema for them (`TestCodeStagesMakeNoAgentCall`; `TestOnRuntimeCodeStages` checks the telemetry).
- **Injected faults.**
  - Load: placeholder and truncation.
  - Ledger: overwritten row, deleted row, own rows altered, heading dropped, wave section deleted.
  - Decision log: dropped line, dropped fence, shortened text, invented `clears`.
  - Completion report: dropped line, altered phrase.
  - Every committing stage: a rejected commit.

  Each fault ends in a stop that names the commit, or in `invalid-plan` for load. None is accepted silently: a relaunch stops on the guard until a requester repair commit lands. The completion report's own branch is reset instead.
- **SHAs.** A fresh SHA passes `verifyCommit`: commit, parent, only path, branch tip, exact blob, `Lsf-Content` trailer. An adopted or deduplicated SHA passes `checkReachable`. A fabricated `rev-parse` output is rejected for all three committing stages.
- **`CallResult`.** It reaches every agent call site with its value, including null calls and parallel branches. The journal keeps denials alongside flags.
- **promptparity.** It reports 55 scenarios identical, with only `load`, `load:*`, `decision-log`, `ledger:*` and `completion-report:*` excluded from the call comparison. Each excluded call's Go step input is checked against the JS prompt it replaces (`stepParity`).
- **The tests pass.** `go vet ./...` is clean. No staticcheck or golangci-lint is installed.

## Deviations from the unit entry's scope and publishes

- **Out of Scope:** `docs/design/lsf-runtime.md` (Interface lines for `Repo` and `Worktree`, planned in §2.9) and the `CLAUDE.md` structure line were edited.
- **`Runtime` interface** (publishes, as planned): `Call`, `Parallel`, `Step`, `Repo`, `Worktree`, `Log`, `Args`. `lsfrt.Runtime` gained `Repo()` and `Worktree(ctx, label)`.
- **Code-stage helpers** (orchestrator-internal):
  - `step[T]` returns `(r, fault, err)`;
  - `runStep` is the plan's `orch.step`;
  - stage functions take the step's final label (`stageFn`).

## Deferrals

- **`CallResult` consumption → `runtime-signals`.** No call site reads flags or denials yet; they are discarded as `_` and through `parallelAgents`. `TestCallResultReachesCallSites` proves the plumbing only.
- **Code-stage fault kind → `runtime-signals`.** Code-stage faults use kind `agent-null` (D19), because `plan` is outside Scope.
- **Cancellation of code-stage git → `runtime-signals`.** The runtime's repeated-signal kill (`killCh`) does not reach code-stage git; only `ctx` cancellation and the per-command bound do. The plan's interface has no channel for it.
- **Stale design text → `script-retirement`, or whichever unit next owns those sections.** `docs/design/lsf-workflow.md` outside `::model-tiers` still describes the JS mechanics: "committed on the unit branch by a copy stage at the mechanical tier" (line ~212), along with the load and ledger prose. Stale comments also remain in `plan/graph.go:11` (outside Scope), `orchestrator/report.go`, `prompts.go` and `orchtest/fixtures.go`.

## Discoveries affecting other units

- **`code-merge-cleanup`:**
  - Build on `gitOps` (`writeAndCommit`, `verifyCommit`, `guard`, `adopt`, `checkReachable`, `changed`).
  - On git 2.54, a pre-commit hook's `git add` **does** reach a `git commit -- path` commit, and leaves the real index behind HEAD. `writeAndCommit` resets the path's index entry after every commit; a multi-path bookkeeping commit needs the same treatment.
  - `verifyCommit` checks a single path; the merge's bookkeeping commit touches two (plan and archive).
  - Completion-report branches are now `lsf/<plan8>/<key of completion-report:<unit>>`, one commit on top of the unit's work branch. Merge receives that branch.
  - D17 stays for scripted values. With a real stage, a bad base surfaces as a stage fault.
- **`runtime-signals`:**
  - `CallResult` is at every site.
  - Code-stage failures carry their reason in the statement (`"<stage> failed: <reason>"`). A dedicated kind would be a `plan` change.
- **Promptparity:**
  - Step inputs are now compared by substring presence in the JS prompt.
  - `orchtest.Stub.Call` records steps (`Step`, `Input`) and keeps them in `Labels()`.
  - Tests read step inputs with `input(t, s, label)` and `ledgerText`.
- **Hooks that rewrite these files** (formatters, end-of-file fixers) or stage other paths make every launch stop until the project exempts the paths. This is intended (D19); requesters with such hooks need to know.
- **State dirs journaled before this unit are not resumable** for these labels (D18).

## Standing obligations

- **Register dispositions.**
  - **P-C1** (load: placeholder, truncation, alteration): **disposed.**
    - There is no transcription: the documents are read whole from git, and `LoadGraph` extracts the blocks.
    - A placeholder or truncated block injected into the git read ends `invalid-plan`. A clean read is byte-equal to the blob.
    - A parse-valid alteration needs a transcription to arise. It is undetectable by any reader of the same bytes, and cannot now arise.
  - **P-C2 to P-C4** (decision log and ledger appends; damaged rows, fences, headings, invented `clears`; completion-report copy): **disposed.**
    - The text is rendered and committed by code, and verified byte for byte.
    - Damage introduced by a hook is caught and stops.
    - The guard blocks building on a damaged earlier stage commit.
  - **P-C8** (fabricated ledger SHAs): **disposed.** Returned SHAs are full, read from git and verified; a fabricated one is rejected. Merge SHAs remain with `code-merge-cleanup`.
- **Design of record.** `lsf-workflow.md::model-tiers` was amended in `54652d5` (load, decision log) and `1819fed` (all four stages), each in the same commit as the behaviour. The wording was trimmed in `f9a842e`.
- **Frozen JavaScript.** `git diff --stat 492b524 -- workflows/` is empty.
- **`go.mod` stdlib-only.** `go.mod` is unchanged.

## External-gate re-verification

`lsf-toolchain`: `go version` reports go1.27.1, and `go.mod` pins `toolchain go1.27.1`. The pin is confirmed, with no drift.

## Execution signals

- **Context:** comfortable at completion.
- **Mid-implementation design decisions** (not taken at a design pass):
  - the step's final label is passed to stage functions;
  - a copy of `plan`'s block regex locates the decision log's closing fence;
  - the path's index is reset after every commit (probe finding);
  - the ledger live-read and scribed-rows ordering;
  - an interrupted ledger rerun refuses to re-append rows already at the tip;
  - the unit branch's tip goes in the completion report's input;
  - git runs in its own process group;
  - step inputs are compared against the JS prompts in promptparity.

  Most of these came from critique findings rather than gaps in the design.
- **Critique load:**
  - Per-step gates on steps 1–4 raised no Critical findings and 9 Significant ones.
  - The full-diff critique and lens review raised no Critical findings and 5 Significant ones. All were addressed except the deferrals above.

## Next step

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
