# declared-gates — completion report

- **Work unit:** `declared-gates` of `docs/plans/lsf-improvements-breakdown.md` @ `98e2d66`.
- **Branch:** `lsf-workflow`.
- **Plan:** `docs/plans/declared-gates-plan.md`.
- **Implementation summary:** `docs/plans/declared-gates-implementation.md`.

## Done-condition verdict

**Met, under the deviations agreed at plan time (below).** The plan's success
criteria S1–S8 and S10 are `proven`; S9, the prose deliverables, is
`evidenced`. Evidence:

- `go test -race ./...` is green, and `go vet ./...` is clean;
- graphparity passes and promptparity is identical, both with node required;
- two end-to-end tests run on the real runtime, journal and git:
  - `TestOnRuntimeDeclaredGates`: red at implement, a misreport recorded, green after the fix, the merge by reuse, a relaunch replaying every gate;
  - `TestOnRuntimeRegressedMergeRelaunch`: a merge regression, the documented recovery, then a relaunch that merges.

Clause by clause:

- **Declared where the unit decides.** A ```yaml lsf-gates block in the plan
  (`## Gate commands`), parsed by `plan.ParseDeclaration`, pinned at a launch's
  first load.
- **Runtime runs them on a clean checkout:**
  - after implement and after each code fix, as stated;
  - for merges, before each merge commit rather than after it (deviation 1).
- **Reviewers read its results:** yes, with each failing gate's output tail.
- **A stage reporting `green` for a gate it did not run is caught:** yes. The
  runtime's result supersedes it, and the difference is a `gate-misreported`
  record.
- **An omitted declared gate is `gate-omitted`:** not as written (deviation 2).
  An omission no longer matters, because the runtime runs every declared
  gate.
- **A merge whose gates fail stops with the unit regressed:** yes. Nothing is
  committed, so there is nothing to undo (deviation 1). The kill test is a
  kill mid-gate and a kill after the gate record, clean and resolved: the
  resume neither adopts nor merges twice. S7's kill tests run on a
  journaling test runtime. The nested-step journal they rely on is proven on
  the real runtime (`TestNestedStep`).
- **`phronithm:breakdown` emits the declaration:** yes. It is derived in
  Phase 1 step 4.
- **The tests pass:** yes.

## Deviations from the unit entry

Agreed at plan time:

1. Gates run before the merge commit, not after it, so there is nothing to
   undo. The kill test is a kill mid-gate (decision 8).
2. Declared gates are not stage-reported, so `gate-omitted` is not raised for
   a declared gate. Plans without a declaration keep it (decision 6).
3. `lsf-runtime/orchestrator` is written outside the Scope line.
4. The runtime config gains `gateTimeout` and `gateConcurrency`.
5. `merge-outcome` records also fix the existing resume wedge for an
   `unresolvable` unit before an in-progress merge.

Taken during implementation:

- **Outside Scope:**
  - `skills/lsf-drive/SKILL.md`: a `declared-gates-changed` relaunch row,
    and `gate-not-run` text qualified for declared gates. This was from the
    step 6 critique. `runtime-cli-drive`'s Scope holds that section.
  - `docs/design/lsf-runtime.md`.
  - `CLAUDE.md`.
- **Undeclared behaviour changed in one place.** Non-merge outcomes are
  journaled in either mode, which is the wedge fix in item 5. Prompts and
  steps are otherwise unchanged (promptparity).
- **Merge results.** A merge result with a regressed unit is not journaled,
  so the documented recovery reaches it (Phase 5). An unchanged merge tree
  replays its red record; a moved integration branch gives a new tree,
  which is gated afresh.
- **Re-plan prompt.** Only the wave re-plan prompt forbids editing the
  block. A decision re-plan runs before the first load and is the sanctioned
  path for a requester's change.
- **Publishes:**
  - the declared gate-command field (the `lsf-gates` block, `plan.Declaration`);
  - `lsfrt.Runtime.RunGates`, `GateSpec`, `GateRun` and the config fields;
  - the `Runtime` interface method;
  - the kinds `declared-gates-changed` (stop) and `gate-misreported` (record).

## Deferrals

None. Nothing in scope was deliberately left undone.

## Discoveries affecting other units

- **`runtime-cli-drive`:**
  - New stop kind `declared-gates-changed`. Its relaunch is "again; the
    relaunch takes the changed block", or a decision restoring it. A row is
    already in `lsf-drive`'s table; the unit's Relaunch work should keep it.
  - `gate-not-run` now also comes from the runtime, after a code stage or
    before a merge. The requester's route is unchanged: an environment or
    timeout change, not a decision.
  - A red gate result replays from the journal. The recovery is to remove
    `journal/gates_<tree prefix>-*.json` (`docs/plan-graph.md`). The drive
    should name it beside the other hand-cleared faults.
- **`long-running-design`:**
  - Cost: gate checkouts are clean, so every gate run is a cold build, and
    `setup` covers dependency installs.
  - Cost: merge reuse hits only a merge onto an unmoved integration branch,
    in practice the first merge of a wave. Every other merge runs the full
    gates again.
  - With `gateConcurrency` 1, a wave's units gate one after another.
  - Gate time counts toward `maxWallClock`.
  - A broken environment costs each waiting unit its own full timeout in
    turn.
- **`s2-permissions-design`:**
  - Gate commands run on the host with the runtime's environment and
    credentials, outside every agent permission. Their trust is the plan
    document's.
  - The block is pinned within a launch. A re-plan agent that edits it
    stops the run, and the relaunch takes the edit, after the requester has
    seen the stop.
  - Failing output tails, up to 4 KB per gate, reach reviewer prompts and the
    journal.
- **`empirical-unit-impl`** (serialised on the same files):
  - `skills/breakdown/SKILL.md` now has a `## Gate commands` template section,
    a derivation paragraph in Phase 1 step 4, a quality-check item and a
    re-plan rule.
  - `docs/plan-graph.md` has a Gate block section.
  - The `gate-*` amendment kinds are scoped to external gates.
- **`script-retirement`:**
  - The Workflow script ignores the block. Undeclared plans keep
    stage-reported gates, `gate-omitted` and the long-gate recipe. Once the
    script is retired, decide whether a plan must declare its gates.
- **`retained-artefacts-impl`:** gate checkouts are detached worktrees under
  `<stateDir>/gates/`, removed after each run. They never reach cleanup or
  `RemoveWorktree`.
- **Testing method, for later units:** `runtime.Goexit` in a test goroutine
  emulates SIGKILL inside a step: no deferred abort in the code under test
  runs. The merge kill tests use it with a journaling test runtime.

## Standing obligations

- **Register dispositions.** No problem-register rows were assigned to this
  unit. It closes risks R11 and R12:
  - **R11 (a stage reporting `green` for a gate it did not run):** closed for
    declared plans. The runtime runs the gates itself, and a contradicting
    report is a `gate-misreported` record.
  - **R12 (a gate over the Bash tool's limit):** closed for declared plans.
    Gates run with no tool limit, under `gateTimeout` (default 2 h) or the
    gate's own timeout.

  Both stand for undeclared plans and for the Workflow script.
- **Design of record.** `lsf-workflow.md::lifecycle-mapping` (stages 3–5),
  the merge paragraph, the stop rule and the relaunch table were amended in
  `aaccca4`, step 6, the last behavioural step. They were corrected in
  `6de225c`, and with each Phase 5 behaviour change (`1ed1a82`, `784baba`,
  `7e88e58`).

## External-gate re-verification

`lsf-toolchain` (environment, pinned in `lsf-runtime/go.mod`): `toolchain go1.27.1`, and `go version` reports `go1.27.1`. Confirmed before step 1; no drift.

## Execution signals

- **Context at completion:** comfortable.
- **About 15 design decisions taken mid-implementation, most from critique:**
  - the environment-variable command and fixed script, which keep the
    orphan marker;
  - group kill after every command;
  - a log failure as not-run;
  - the stable gate label (`runStepAs`);
  - waiter reruns after an unkept flight;
  - tails kept out of the committed report;
  - duplicate commands rejected;
  - `gatedMerge` on land and adopt;
  - red preferred over not-run;
  - a regressed merge left unjournaled;
  - a signal death as not-run;
  - a result that beat a stop kept;
  - the frozen-block sentence on the wave re-plan only;
  - the decision re-plan as the change path;
  - the derivation placed in Phase 1.

  None moved the design's approach.
- **Size:** about 3,300 lines over 31 files, two thirds of them tests.

## Open questions for the requester

1. **Stops and gates.** A stop of any kind (a first signal, a cap, another
   unit's runtime error) kills an in-flight gate, as S2 specifies, and the
   killed run reruns on relaunch. Calls, by contrast, finish on a first
   signal. Keep this, or match calls?
2. **Red replay.** A set whose gates all completed, red included, replays for
   the same tree and declaration. A red result caused by the environment
   (exit 126 or 127, a missing service, a command OOM-killed, which the
   shell reports as exit 128+N) needs its record removed by hand. A gate
   whose own shell is killed is already not-run. Keep this, treat those
   exits as not-run, or add an invalidation switch?

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
