# declared-gates: implementation summary

- **Working branch:** `lsf-workflow`. Base `31a924b` (`declared-gates: plan`).
- **Plan:** `docs/plans/declared-gates-plan.md`.
- **External-gate re-probe:** `lsf-toolchain` (environment, pinned in
  `lsf-runtime/go.mod`). It was confirmed before step 1: `toolchain go1.27.1`, and `go version` reports
  `go1.27.1`. No drift.

## Files modified

`CLAUDE.md`, `docs/design/lsf-runtime.md`, `docs/design/lsf-workflow.md`,
`docs/plan-graph.md`, `skills/breakdown/SKILL.md`;
`lsf-runtime/plan/{gates.go, gates_test.go, schema.go}`;
`lsf-runtime/runtime/{gates.go, gates_test.go, children.go, config.go, runtime.go, journal_test.go}`;
`lsf-runtime/orchestrator/{gates.go, gates_test.go, merge.go, merge_gates_test.go, loop.go, runtime.go, unit.go, prompts.go, report.go, report_test.go, results.go, wave.go, doc.go}`;
`lsf-runtime/orchestrator/orchtest/{orchtest.go, fixtures.go}`.

## Per-step record

### Step 1: declaration parse (`1c59d75`, fixes `c253332`)

`plan.ParseDeclaration` and the `Declaration` and `DeclaredGate` types were
added as designed. `declared-gates-changed` and `gate-misreported` were added
as Go-only kinds.

**Decisions beyond the plan.** The parser additionally rejects:

- a key that is present with a null value (`timeout:`);
- a `setup` that is not a string;
- a gate name with surrounding space, control characters or `/`, because the
  name flows into log names;
- a command that repeats another gate's (added in step 5 from step 4's
  critique).

Values compare as written, so `1h` and `60m` are different declarations.

**Critique** (0 Critical, 2 Significant, 8 Minor):

| Finding | Disposition |
|---|---|
| Significant: null treated as absent | Addressed |
| Significant: non-string or colliding name | Addressed |
| Minor: regex recompiled | Addressed |
| Minor: `ok` reuse | Addressed |
| Minor: name hygiene | Addressed |
| Minor: missing tests | Addressed |
| Minor: identity textual | Documented |
| Minor: dangling D27 and plan-graph references | Resolved in step 6 |
| Minor: first-load trust | Documented in plan-graph.md, step 6 |

### Step 2: runtime gate runner (`e8e0d24`, fixes `e13953b`, `6d967de`)

`Runtime.RunGates`, `Config.GateTimeout` (default 2 h) and
`Config.GateConcurrency` (default 1) were added. Orphan reaping now matches
the `lsf-gate:` marker.

**Surprise.** `sh -c "<cmd>" marker` execs a lone simple command, which drops
the marker from the process arguments. A test confirms this with a plain
`sh -c "sleep 30" marker`. The command therefore reaches the shell through
`LSF_GATE_COMMAND`, with the fixed two-command script
`eval "$LSF_GATE_COMMAND"; exit $?`.

**Other decisions:**

- The group is SIGKILLed after every command, not only a cancelled one, so
  leftover background processes cannot hold the checkout.
- A log file that cannot be created makes its gate `not-run` rather than
  stopping the launch.

**Critique** (0 Critical, 3 Significant, 8 Minor):

| Finding | Disposition |
|---|---|
| Significant 1: a first-signal stop kills in-flight gates, unlike calls | Kept as planned (S2: "a stop kills the process group"). **Open question for the requester**, below. |
| Significant 2: test gaps (registration, TERM-ignoring timeout, background leftover) | Addressed with a new test |
| Significant 3: log-create failure stopped the launch | Addressed |
| Minor: pid-reuse window, setup daemons killed, `exec` escapes reaping, label uniqueness, environment trust | Documented in code |
| Minor: remaining | Noted |

### Step 3: orchestrator plumbing (`3657fa8`; fixes in `a022349`)

Added the `loadPlan` baseline and change stop, `orch.gates` (per-tree
single-flight memo plus the journaled `gates:<tree[:12]>` step), the
`Runtime.RunGates` interface method, and the `orchtest.Stub` scripted runner.
The `TestNestedStep` runtime test proves the nested-step journal through
kills.

**Deviation.** S8's test drives the change through a wave re-plan and through
the termination reload: a clean wave runs no re-plan.

**Critique** (0 Critical, 4 Significant, 6 Minor):

| Finding | Disposition |
|---|---|
| Significant 1: an owner's panic hangs waiters | Addressed (`fly` defers) |
| Significant 2: `#n` labels broke cross-launch replay after a retry | Addressed (`runStepAs`, stable label) |
| Significant 3: waiters ignored their own context or inherited the owner's stop | Addressed (select; rerun after an unkept flight) |
| Significant 4: red sets replay forever | By design (S5). **Open question for the requester**, below. |
| Minor: memo test sensitivity | Addressed (callers parked on a live flight) |
| Minor: stub lock juggling | Addressed (defer relock) |
| Minor: remaining | Noted |

### Step 4: unit stages (`a022349`; fixes in `8e9cfce`)

The runtime gates the implement commit (non-design units) and each code-fix
commit, using the SHA as corrected by `checkReturn`. The rest of the step was
implemented as designed: `gate-misreported` records, runtime `not-run` stops,
reviewers reading the runtime's results with tails, the declared-gates
prompts, "Gates at HEAD" from the runtime, and `unitResult.gatedTree`.
`critiqueLoop`'s `onFix` now returns an error. Undeclared prompts are
byte-identical (promptparity).

**Critique** (0 Critical, 2 Significant, 8 Minor):

| Finding | Disposition |
|---|---|
| Significant 1: tail fence malformed in Markdown; tails in the committed report | Addressed: tails indented, and kept out of the report |
| Significant 2: red replay | As step 3; open question |
| Minor: stop swallowed by the not-run path | Addressed |
| Minor: duplicate commands | Addressed (parse problem) |
| Minor: empty commit with no git | Addressed |
| Minor: stale `gate-not-run` comment | Addressed |
| Minor: sequential waiter reruns, design-unit prompt wording, schema description | Noted |

### Step 5: merge gates before commit (`8e9cfce`, fixes `2c63ce3`)

Implemented as designed:

- A clean merge is gated through a dangling `commit-tree` candidate before
  `git merge`, with the HEAD-moved loop.
- A resolved merge is gated via `write-tree` before its commit.
- A red result is `regressed` with nothing committed; a resolved merge is
  aborted.
- A gate `not-run` is reported as `regressed`, and the wave rewrites it to
  `gate-not-run` (`mergeResult.GateNotRun`).
- D26 statement rewrite.
- Merge reuse.
- `merge-outcome` records.

**Decisions beyond the plan:**

- `gatedMerge`, from critique: every merge commit landed or adopted in
  declared mode must have a gated tree (a free memo or journal hit normally).
  An uncovered tree is gated then, and a red one faults.
- A red gate wins over a not-run gate in one merge.
- Candidates are unsigned.

**Test method.** Kill tests use a goroutine ended by `runtime.Goexit`, so no
abort runs, as with SIGKILL. They run over `journalRT`, a journaling test
runtime.

**Critique** (0 Critical, 4 Significant, 7 Minor):

| Finding | Disposition |
|---|---|
| Significant 1–2: an ungated commit could be committed in a race or by a hook, then adopted | Addressed (`gatedMerge` on land and adopt; a test adopts a raced red merge and faults) |
| Significant 3: `journalRT` passes no `interrupted`, and the outer step is not journaled | Noted for Phase 5; the nested steps ignore `interrupted` |
| Significant 4: a stop during a resolved merge's gates aborts the merge | Kept, consistent with a conflict-call stop; documented in D26 |
| Minor: dead assertion | Addressed |
| Minor: red-plus-not-run | Addressed |
| Minor: gpgsign | Addressed |
| Minor: kill-point-dependent re-evaluation | Documented in D26 |
| Minor: probe-as-step noise | Noted |
| Minor: undeclared outcome recording | Intended; promptparity unaffected, since the stub's merge is a handler |

### Step 6: docs and skill (`aaccca4`)

- **plan-graph.md:** a Gate block section; a plan test parses its example.
- **Breakdown skill:** derives the gate commands in Phase 1 step 4 and emits a `## Gate commands` block, checks it in the quality
  check, and leaves it alone at re-plan.
- **lsf-workflow.md:** lifecycle stages 3–5, the merge paragraph, the stop
  rule, the records, the relaunch table, and the constraint.
- **Other docs:** lsf-runtime.md, CLAUDE.md, and `doc.go` (D22, D24, D26,
  D27).

**Deviation.** The plan's "Re-plan prompts" design item had no step and was
done here. Only the wave re-plan prompt carries the sentence: the decision
re-plan runs before the first load, so it cannot know the mode. A gate block
a decision re-plan changes becomes the baseline (D27).

**Critique** (0 Critical, 5 Significant, 8 Minor; fixes in the step 6 critique-fixes commit):

| Finding | Disposition |
|---|---|
| S1: the docs imply the shipped Workflow script runs the gates | Addressed: "the Go runtime runs them; the Workflow script ignores the block" |
| S2: the derivation sat in the quality check, which re-plan re-runs | Addressed: moved to Phase 1 step 4; the check is skipped at re-plan |
| S3: no sanctioned path to change a gate command | Addressed: a requester's decision, applied by the decision re-plan before the first load (skill, plan-graph.md, D27) |
| S4: `lsf-drive` had no relaunch row for `declared-gates-changed` | Addressed: row added; the `gate-not-run` text is qualified |
| S5: red replay undocumented | Documented, with the recovery: remove `journal/gates_<tree prefix>-*.json`. Open question 2 stands. |
| M1: reuse qualifiers | Addressed |
| M2: stale framing | Addressed (plan-graph.md intro, CLAUDE.md) |
| M3: stale "Not in this unit" line; environment wording | Addressed |
| M4: review-stage paragraph | Addressed |
| M5: undocumented merge-path behaviours; D27 "stage-fault" wording | Addressed |
| M6: "gates" terminology collision | Addressed: section `## Gate commands`; the `gate-*` amendment kinds are scoped |
| M7: planner guidance (setup, baseline-green probe, timeout) | Addressed |
| M8: prose | Addressed: the rationale sentence and the constraint remark are removed; "launch" wording; `1h`≠`60m` documented |

`skills/lsf-drive/SKILL.md` was edited outside the step's file list (S4).

## Phase 5

- **Lens review.** All applicable lenses. The pre-scan found `go vet` and
  `gofmt` clean; staticcheck and golangci-lint are not installed. It raised
  0 Critical, 2 Significant and 6 Minor findings.
- **Full-diff critique.** Round 1 raised 0 Critical, 3 Significant and 6
  Minor. Round 2, on the fixes, raised 0 Critical, 3 Significant and 6 Minor.
  The gate passes.
- **Addressed:**
  - **A regressed merge's result was journaled** (round 1, S-1). A relaunch
    replayed the stop without re-gating. The result is now not journaled
    (`launch.mergeStep`, declared mode). A real-runtime test fails with the
    old behaviour.
  - **Signal-killed gates were classed red.** A gate whose shell is killed is
    now not-run. A killed child of the shell remains exit 128+N, red; this is
    pinned by a test and documented, and folded into open question 2.
  - **A gate that finished as a stop arrived was discarded.** Its result is
    now kept unless the cancellation reached the process.
  - **A stop before `Start` hit a nil `ProcessState`** (round 2, S1). It now
    returns the stop.
  - **Gate log names** now carry a per-run sequence.
  - **Docs:** the tree-only invariant, submodules in `setup`, failing tails
    reaching prompts and the journal, fenced examples counting as blocks,
    when a regressed merge's gates replay or rerun, and D26's note that
    outcome records are kept in either mode.
- **End-to-end tests on the real runtime:**
  - `TestOnRuntimeDeclaredGates`: implement red, then a misreport, then the
    fix green, then the merge by reuse, then a relaunch replaying everything
    with no gate run.
  - `TestOnRuntimeRegressedMergeRelaunch`: a merge regression; an unchanged
    relaunch replays the red record; the recovery; the merge lands.
- **Noted, not changed:**
  - `mergeOne` is long, about 250 lines.
  - A `regressed` merge's `Files` carry gate descriptions, not paths: a
    latent coupling, since no consumer reads them as paths.
  - A flaky green replays like a red (open question 2).
  - The unjournaled merge rerun is `interrupted`, which relaxes the start
    checks as a crash recovery would.
  - `step-failed` events and log lines on the expected unjournaled paths.
  - A waiter's sequential reruns under a broken environment.
  - The S2 claim, "a stop kills the group", now excludes a gate already
    reaped.

## Success criteria

| | Target | Achieved | Status | Evidence |
|---|---|---|---|---|
| S1 | proven | proven | pass | `plan/gates_test.go`: every problem, absent block, round trip, two blocks, and the doc example |
| S2 | proven | proven | pass | `runtime/gates_test.go`: statuses, timeout, setup failure, clean checkout and leftovers, stop kills a TERM-ignoring grandchild, marker reaping, real-gate registration, concurrency (mutation-checked: concurrency 3 and a plain `sh -c` both fail) |
| S3 | proven | proven | pass | `TestDeclaredGatesSupersedeStage`, `TestDeclaredGatesAfterFix`, `TestMergeGatesClean`, `TestDeclaredMergeInput` |
| S4 | proven | proven | pass | `TestDeclaredGatesSupersedeStage`, `TestDeclaredGatesStageOmissions` |
| S5 | proven | proven | pass | `TestGatesMemoPerTree`, `TestGatesJournalReplay` (real runtime, same tree under another commit), `TestGatesFlightFailures`, `TestMergeGatesReuse` |
| S6 | proven | proven | pass | `TestMergeGatesClean`, `TestMergeGatesResolvedRed`, `TestMergeGatesNotRun` |
| S7 | proven | proven | pass | `TestMergeGatesKillClean`, `TestMergeGatesKillResolved`, `TestMergeGatesRegressedThenKilled` (outcome replay mutation-checked), `TestMergeUnresolvableThenKilled`, `TestMergeGatesUngatedCommit` |
| S8 | proven | proven | pass | `TestDeclaredGatesChangedStops`, `TestDeclaredGatesChangedByReplan` |
| S9 | evidenced | evidenced | pass | Docs written and read against the code; step 6 critique |
| S10 | proven | proven | pass | `PROMPTPARITY_REQUIRE_NODE=1 GRAPHPARITY_REQUIRE_NODE=1 go test -race ./...` green after every step; parity identical |

**Rigour caveat.** S7's kill tests run against a journaling test runtime, not
`lsfrt.Runtime.Step`. The nested-step journal itself is proven against the
real runtime by `TestNestedStep`, and two real-runtime end-to-end tests cover
replay and the regressed-merge relaunch. The upgrade path is a real-runtime
kill of the merge step mid-gate.

## Open questions for the requester

1. **Stops and gates.** A stop of any kind (first signal, a cap, a concurrent
   unit's runtime error) kills an in-flight gate, as S2 specifies. Calls, by
   contrast, finish on a first signal. A killed gate is not journaled, so a
   relaunch reruns it. Keep this, or match calls (kill only on a second
   signal)?
2. **Red replay.** A red set is journaled and replays on relaunch for the
   same tree and declaration. A red result caused by the environment (exit
   127, a missing service) needs a tree or declaration change to rerun. Keep
   this, treat 126/127 as not-run, or add a way to invalidate a journaled set?

## Remaining phases
Phase 5 is complete. The completion report is `docs/plans/declared-gates-completion.md`.
