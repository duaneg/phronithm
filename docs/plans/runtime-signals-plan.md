# runtime-signals — plan

Work unit `runtime-signals` of `docs/plans/lsf-improvements-breakdown.md` @ `85dced0`. Base commit `85dced0`, branch `lsf-workflow`. Phases 1–3 ran compressed in one plan-mode session, so this document is the problem statement, the design and the implementation plan. Phase 4 step 0 saves it as `docs/plans/runtime-signals-plan.md`.

## 1. Problem statement

### Context

In the substrate spike, driver-detected events (a corrected SHA, a permission denial, uncommitted work at return, an omitted gate, a skill not loaded) reached telemetry only. The Go runtime now returns each call's `CallResult` (`Flags`: `skill-not-loaded`, `guard-killed-after-result`; `Denials`) to every orchestrator call site, but every site discards it (`_`, and `parallelAgents` slots that nothing reads). Nothing detects uncommitted changes at a call's return. Nothing checks a returned SHA against its branch. The ported stop rule fails open on an out-of-schema enum: an unknown merge status becomes `claimed-disjoint-conflict`, and an unknown finding severity is dropped. A code-stage fault is an `agent-null` stop (D19), because `plan` had no kind for it. A repeated signal (`killCh`) does not end a code step's git.

### Unit entry, verbatim

```
### runtime-signals
- **Type**: feature
- **Risk**: medium
- **Clarity**: exploratory — one open decision: which driver-detected events stop the run
- **Iteration**: write-once
- **Effort**: medium
- **Scope**: `lsf-runtime/plan`, `lsf-runtime/runtime`, `lsf-runtime/orchestrator`, `docs/design/lsf-workflow.md::escalation`
- **Publishes**: driver-event signal kinds
- **Depends on**: code-stage-interface (logical, `CallResult` in the orchestrator; serialises the runtime and orchestrator files); runtime-core (logical, `CallResult` and telemetry)
- **External gates**: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`)
- **Done condition**: A corrected SHA, a permission denial, uncommitted changes at a stage's return, a gate the agent omitted, a skill not loaded, a guard kill after a valid result (`guard-killed-after-result`, requester decision 2026-10-06), and a stage result carrying an enum value outside its schema each reach the ledger as a signal, and the decided ones stop the run. The runtime detects uncommitted changes at a call's return, which `runtime-core` does not. Tests reproduce postmortem §C's permission faults (a denied commit reported with HEAD as its SHA; a denied branch delete) and run #2's transient null, and show none yields its recorded outcome. A `gate-not-run` stop is not retried (requester decision, 2026-10-05). A test shows an out-of-schema merge status is not recorded as `claimed-disjoint-conflict` and an out-of-schema finding severity is not dropped. Every agent call site reads its `CallResult`, which is discarded as `_` and through `parallelAgents` today. A code-stage fault carries a signal kind of its own in place of D19's `agent-null`, with the reason it now carries in the statement. A repeated signal ends a code step's git, as `gitContext` does for worktree git: the `ctx` that `rt.Step` passes to its function ends on `killCh`. The tests pass.
- **Description**: Replaces `stage-failure-handling`. In the spike these events reached telemetry only. The ported stop rule fails open on a schema-violating enum, faithfully to the JavaScript (orchestrator-port-plan report). The orchestrator writes the ledger rows, so the signals cross into it. The `CallResult`, fault-kind and `killCh` clauses are `code-stage-interface`'s deferrals. **Flag:** four surfaces exceed the surface bound; the `plan` and `orchestrator` changes are signal kinds and their ledger mapping, so the excess is not context. About eight planned items exceed the step bound; each is an event read from a `CallResult` or a step and mapped to a kind, so the excess is breadth of one pattern.
```

Standing obligations, programme-wide: `workflows/` is not edited (frozen JavaScript); `go.mod` stays stdlib-only; the design of record (`lsf-workflow.md::escalation`) is amended in the commit that changes the behaviour it describes. No paid run is planned; the unit has no `claude-code-headless` gate.

### Decisions confirmed with the requester (2026-10-07)

The open decision, which events stop:

| Event | Disposition | Kind |
|---|---|---|
| Uncommitted changes at return, stage that commits ("writer") | **stop** | `uncommitted-at-return` |
| Uncommitted changes at return, read-only stage ("reader") | record | `residue-at-return` |
| Permission denial | record | `permission-denied` |
| Skill not loaded (after the runtime's one retry) | **stop** | `skill-not-loaded` |
| Returned SHA not the reported branch's tip | record (corrected from git) | `sha-corrected` |

Stated to the requester and not objected to:

| Event | Disposition | Kind |
|---|---|---|
| Guard kill after a valid result | record (the result stands, 2026-10-06) | `guard-killed-after-result` |
| Stage result outside its schema (enum or otherwise) | **stop** | `schema-violation` |
| Code-stage fault | **stop** | `stage-fault` (replaces D19's `agent-null`) |
| Gate omitted: none reported, or a fix drops a gate implement ran | **stop** | `gate-omitted` (split from `gate-not-run`, so `declared-gates` can emit it for a declared gate missing) |

After the round-1 critique (2026-10-07):

| Question | Decision |
|---|---|
| A requester edit to the main checkout during a repo writer call is indistinguishable from the agent's | **Stop anyway.** The statement lists the paths; worktree writers are unaffected |
| The uncommitted check itself fails (git error, timeout) at a writer | **Stop** (`uncommitted-at-return`, statement says the check failed). The failure is not journaled |
| Merge stops on uncommitted changes with units merged | **Continue** through gate, cleanup and re-plan, as for every other wave stop with units merged; the stop is returned after |

The stop rule stays kind-only (`EvaluateWave`), so the writer/reader split is two kinds, chosen by the orchestrator at each call site. "Writer" means the stage commits, so `cleanup` (removes worktrees, commits nothing) is a reader.

### Success criteria

Target `proven` unless stated.

| # | Criterion | Target |
|---|---|---|
| S1 | **Kinds.** `plan.StopKinds` gains `uncommitted-at-return`, `skill-not-loaded`, `schema-violation`, `stage-fault`, `gate-omitted`; `plan.RecordKinds` gains `residue-at-return`, `permission-denied`, `sha-corrected`, `guard-killed-after-result`. Each new kind reaches the ledger from every kind of site that produces it, and each stop kind stops the run while each record kind does not.
- The `CallResult` kinds (uncommitted and residue, skill, denial, guard kill, `schema-violation`) are shown from a unit stage and from a loop stage, through the orchtest stub.
- `gate-omitted` is shown at implement and at a code fix.
- `stage-fault` is shown at a code step.
- `sha-corrected` is shown at implement, on the real runtime, since orchtest has no repo. Each stop kind stops the run and each record kind does not. | proven |
| S2 | **Uncommitted detection in the runtime.** `lsfrt.Runtime.Call` reports `CallResult.Uncommitted`, tested against the stub `claude` and real git. An isolated call reports every path left modified or untracked (not ignored) in its worktree. A repo call reports only paths whose status or content changed during the call: dirt present before the call is not reported, and a file already dirty and changed further is. The before-state is taken once, at the first admitted attempt, so dirt left by a failed attempt before a retry is reported. A staged rename is parsed as one entry. Paths under the state dir are never reported or hashed. A failed check (before or after) sets the flag `uncommitted-unchecked`, which is not journaled. A replayed call re-checks its journaled paths (§2.1). | proven |
| S3 | **Postmortem §C, reproduced on the real runtime** (fake `claude`, real repo). (a) An implement whose `git commit` is denied, leaving its change uncommitted and reporting HEAD as its SHA, stops with `uncommitted-at-return`, records `permission-denied`, and runs no review call. In the postmortem, the stage recorded the bad SHA. (b) A cleanup whose `git branch -d` is denied records `permission-denied` in the ledger and the run continues. In the postmortem, the branches were left with no signal. | proven |
| S4 | **Run #2's transient null.** On the real runtime, an implement call whose first attempt fails transiently (API 529) is retried and the run continues: no `agent-null`. | proven |
| S5 | **`gate-not-run` is not retried.** A test shows an implement return with a `not-run` gate stops after exactly one implement call. | proven |
| S6 | **Schema.** A live call whose last attempt failed validation returns null with `CallResult.Violations` (journaled nowhere: a null is not journaled). The orchestrator maps it to a `schema-violation` stop naming the violations, beside the stage's `agent-null`. A seeded or stubbed return, which the runtime never validated, is checked by the orchestrator against its schema, with the same outcome. `plan.SignalsFromMerge` maps an unknown status to `schema-violation`, not `claimed-disjoint-conflict`. `plan.SignalsFromFindings` maps an unknown severity to `schema-violation`, not a drop. The mapper layer is defence in depth: through the orchestrator an invalid value never reaches it, so it is proven at plan level. Tests: on the real runtime, a merge status outside the enum (both attempts) gives `schema-violation` and no `claimed-disjoint-conflict`; through orchtest, an out-of-schema merge status and finding severity do the same; plan tests cover the mappers. | proven |
| S7 | **`CallResult` read at every site.** Every agent call site (13: eleven `call[T]` sites and two `parallelAgents` batches) passes its `CallResult` to the driver-event mapping. A test shows a flag on each kind of site reaches the ledger: plain, null, parallel reviewer, wave gate, cleanup. | proven |
| S8 | **SHA correction.** For the unit plan, implement, plan-fix and code-fix returns, a SHA that is not a prefix of the reported branch's tip is replaced by the tip, and `sha-corrected` is recorded. No code reads these SHAs today, so the check protects the ledger and any future reader. It does not fire in postmortem §C's denied-commit case (HEAD is the tip); the uncommitted check covers that. Tested against real git. | proven |
| S9 | **Code-stage fault kind.** `stageFaultSignal` emits `stage-fault`, keeping the `"<stage> failed: <reason>"` statement. D19 is amended. | proven |
| S10 | **`killCh` ends a step.** The `ctx` that `lsfrt.Runtime.Step` passes to its function ends on a repeated signal. Test: a step blocked on its ctx returns once `killCh` closes. | proven |
| S11 | **Design of record.** `lsf-workflow.md::escalation` names the driver-detected events (as structural signals) and the new kinds in the stop rule, each amended in the commit that turns the behaviour on. Divergences D20 onward are recorded in `orchestrator/doc.go`, and D19 is amended. | evidenced |
| S12 | `go vet ./...` and `go test -race ./...` pass from `lsf-runtime/`. `go run ./tools/promptparity` and `go run ./tools/graphparity` pass, each with only documented normalisations (Go-only kinds; the fail-closed mapper cases). `go.mod` is unchanged and `git diff --stat 85dced0 -- workflows/` is empty. | proven (tests), evidenced (diffs) |

### Candidate approaches

**A. The runtime reports facts; the orchestrator maps them to kinds (chosen).** The runtime adds `Uncommitted` to `CallResult` beside `Flags` and `Denials`. It knows the call's cwd and the before-state, and nothing about stages. A plan-package mapper turns a call's events into signals, given whether the stage is a writer. The orchestrator owns the writer/reader choice, the schema check (it holds the schemas) and the SHA check (it has git and the stage's result fields). This follows the existing split: `plan.SignalsFromX` mappers and orchestrator call sites.

**B. The runtime emits signals.** The runtime would need the stage taxonomy (writer/reader, which fields are SHAs) and a dependency on `plan`. Rejected: it couples the runtime to the orchestrator's stages, and `rt.Call` cannot reach the ledger anyway.

**C. Per-mapper fail-closed only, for the schema clause.** Fix the two named mappers (`SignalsFromMerge`, `SignalsFromFindings`) and nothing else. Rejected as the only defence: other enums fail open too (a gate status other than `not-run` counts as run). It is kept as a second layer, because the mappers are the place the port report names.

### Exploration findings

- `runtime/api.go` `CallResult{Value, Class, Flags, Denials, Attempts, CostUSD, Replayed}`. `runtime/agent.go` sets `skill-not-loaded` and `guard-killed-after-result`. `attempt` copies `Denials` from the last attempt's result event. `runtime/journal.go` `record` journals `Flags` and `Denials`, and `replayCall` returns them.
- `runtime/validate.go` `validateOutput(schema, raw) []string` is unexported; live calls are validated (an invalid value is retried once, then null). Only seeds (`orch.agent`), and stubs in tests, reach the orchestrator unvalidated.
- `runtime/worktree.go`: `cwdFor` gives isolated calls `<state>/wt/<key>`; repo calls run in `cfg.Repo`. `gitContext` ends on `killCh`; `Step` (`journal.go:136`) passes the caller's ctx through unchanged.
- Isolation by stage: worktree for `plan`, `fix-plan`, `implement`, `fix-code`, `critique-code`, `lens-review`, `critique-design`; the repo for `critique-plan`, `merge`, `review-gate`, `cleanup`, `replan`, `decision-replan`, `termination`, `obligations`, `direct-edits`. In the unit phase, concurrent repo calls are readers only. `review-gate` and `cleanup` run in parallel.
- `orchestrator/runtime.go`: `call[T]` returns `(*ret[T], CallResult, error)`; `decode` turns an undecodable return into nil (D9). `parallelAgents` fills `agentRes{cr, label}`.
- Call sites: `unit.go` (plan, critique-plan, fix-plan, implement, the reviewer batch, fix-code), `wave.go` (merge, gate+cleanup batch, replan, termination, obligations), `loop.go` (direct-edits, decision-replan).
- `unit.go` `critiqueLoop` takes `round` and `fix` closures; `unrunGates` folds "no gate reported" into `gate-not-run`; `onFix` adds dropped gates to the same list.
- `stages.go:375` `stageFaultSignal` uses kind `agent-null`.
- `results.go`: `unitPlanResult`, `implementResult` and `fixResult` carry `Branch` and `Sha`; no code reads those `Sha` fields.
- `orchtest.Stub` handlers can return an `lsfrt.CallResult` (flags, denials); `Stub.Repo()` is "", so git-dependent checks are tested on the real runtime (`integration_test.go`, whose fake `claude` answers from a table and runs in the call's cwd) or against temp repos as in `stages_test.go`.
- `skills/lsf-drive/SKILL.md` lists every STOP kind for the JS workflow. It is outside Scope, and the Go runtime is not yet the supported path: a discovery for `runtime-cli-drive`.

## 2. Design

### 2.1 Runtime: uncommitted at return, and violations

`CallResult` gains `Uncommitted []string` and `Violations []string`. `record` gains `Uncommitted` (omitempty), which `replayCall` returns. `Violations` is never journaled, because only a null return carries it, and a null is not recorded.

**Violations.** When `Call` returns null after an exhausted `classInvalid`, it sets `res.Violations` to the last verdict's `errors`. These are the schema violations, or `no structured_output`. The latter also covers `max_output_tokens`, so the mapper words it as a missing return, not a schema violation (§2.3).

**Uncommitted.** In `Call`:
- **Before-state, repo calls only.** Take `snapshot(cwd)` once, at the first admitted attempt (where `c.cwd` is resolved), and store it in `callState.before`.
  - The snapshot runs `git status --porcelain=v1 -z --untracked-files=all` from the toplevel of `cwd`'s work tree.
  - Each entry is parsed to path → XY status code. A rename or copy entry (`R`/`C`) carries a second NUL-separated origin path, which is consumed and kept as part of the entry.
  - Paths under `cfg.StateDir` are dropped next, before any hashing.
  - Then `git hash-object --no-filters` is run on each remaining dirty path that is a regular file.

  A failed before-snapshot marks the call unchecked.

  Isolated calls take no before-state. A worktree is the call's own, so any dirt in it is the call's, including dirt left by an earlier failed attempt of the same call: no branch carries that either.
- **At return.** Compute `after` the same way on every path that returns a value or null after at least one spawn:
  - `classOK` and `classSkillMissing`;
  - `classTimeout`, `classPermanent` and `classSpawnError`;
  - exhausted `classInvalid`, `classTransient` and `classSessionLimit`.

  An isolated call reports every path in `after`. A repo call reports each path in `after` that is absent from `before`, or whose status or hash differs.

  No check is made on a stop (`classStopped`, a `StopError`) or a dry run. A replay re-checks, as described under *Replay and relaunch*.
- **Failure.** A git error or timeout, before or after, sets the flag `uncommitted-unchecked` and leaves `Uncommitted` empty. Git runs under `gitContext`.
- **Journal.**
  - The check runs before `recordCall`, so the record holds `Uncommitted`.
  - The flag `uncommitted-unchecked` is stripped from the recorded `Flags`, so a resume does not replay a transient git failure. A replay re-checks instead (below).
- **No repo.** When `cfg.Repo == ""` there is no isolation either, so no check is made and no flag is set.
- **Telemetry.** An `uncommitted` event carries the label and the count.

**Replay and relaunch.**
- A relaunch with decisions runs fresh: `header()` embeds the plan SHA and the decision-log SHA in every prompt (`orchestrator/prompts.go:56-68`), and the call hash covers the prompt.
- A plain resume replays the call. Its journaled paths are re-checked rather than re-raised blind:
  - an isolated call runs the full check on its worktree now;
  - a repo call reports only the journaled paths that are still dirty.

  The requester's recovery for `uncommitted-at-return` is therefore to commit or remove the paths and resume. A re-check that fails sets `uncommitted-unchecked`, as a live check does.
- A repo writer that returned null with dirt is not journaled. On resume it re-runs with that dirt in its baseline, so the dirt is not reported again. The stop that ended the earlier launch named the paths (`agent-null` and `uncommitted-at-return`). This is a known limit.

**Known limits, recorded in `lsf-runtime.md`:**
- A requester edit to the main checkout during a repo writer call is attributed to that call, and stops the run (requester decision). The statement lists the paths.
- For the parallel `review-gate`/`cleanup` pair, both readers, a change made by either call is attributed to both. Both rows are records.

### 2.2 Runtime: validator export, and `killCh` in `Step`

- **`ValidateOutput(schema, raw json.RawMessage) []string`** is exported as a wrapper over `validateOutput`. An empty schema means no check: it returns nil.
- **`Step`** passes `fn` a ctx that also ends on `killCh`:
  - `sctx, cancel := rt.killContext(ctx)`, then `defer cancel()`;
  - `killContext` is the goroutine half of `gitContext` without its timeout;
  - `gitContext` is rebuilt on `killContext`.

### 2.3 Plan: kinds and mappers

**`schema.go`.** Add the kinds in S1, each commented in the existing style, e.g. `"uncommitted-at-return", // driver: a stage that commits returned with changes no commit holds`. `Sources` is unchanged: driver events are `structural`, which the design defines as computed by the script or by mechanical checks.

**`signals.go`:**

```go
// CallEvents are the driver-detected events of one agent call.
type CallEvents struct {
	Label       string   // the call's final label: the referent; it locates the transcript under calls/
	Writer      bool     // the stage commits: uncommitted changes stop
	Flags       []string // CallResult.Flags
	Denials     []string
	Uncommitted []string
	Violations  []string // CallResult.Violations, or the orchestrator's own schema check
}

// SignalsFromCall maps a call's driver events, in this order:
//   - schema-violation (stop);
//   - skill-not-loaded (stop);
//   - uncommitted-at-return (writer, stop) or residue-at-return (reader,
//     record), also raised for flag uncommitted-unchecked, whose statement
//     says the check failed;
//   - permission-denied (record; one row listing up to DenialsListed denials);
//   - guard-killed-after-result (record).
// Source structural; referent ev.Label. Unknown flags are returned for
// the caller to log.
func SignalsFromCall(ev CallEvents, at At) (sigs []*Signal, unknownFlags []string)

func ShaCorrectedSignal(at At, branch, reported, tip string) *Signal // record
```

Statements:

| Kind | Statement |
|---|---|
| `schema-violation` | `"<n> schema violation(s): v1; v2 … (and N more)"`, listing up to 5; for `no structured_output` alone, `"no structured output after retry (missing, or cut off at the output limit)"` |
| `skill-not-loaded` | `"skill not loaded after retry; return accepted unverified"` |
| `uncommitted-at-return` / `residue-at-return` | `"<n> path(s) uncommitted at return: a, b … (and N more)"`, listing up to 5; or `"uncommitted check failed"` |
| `permission-denied` | `"<n> permission denial(s): d1; d2; d3"` |
| `guard-killed-after-result` | `"the guard timeout killed the call after its result; the result stands"` |
| `sha-corrected` | `"<branch>: reported <r>, tip <t>; corrected"`, or `"<branch>: reported <r>; not checked: <err>"` (nothing was corrected) |

**Fail-closed mappers (divergence D20):**
- **`SignalsFromMerge`.** `resolved` maps explicitly to `claimed-disjoint-conflict`. Any other non-`clean` status is `schema-violation`: ``merge of <unit>: status "<s>" is outside the schema``.
- **`SignalsFromFindings`.** `Critical` is skipped, as now. `Significant` and `Minor` map as now. Any other severity is `schema-violation`, with source `structural` (as every `schema-violation` row) and the finding's location as referent.

### 2.4 Orchestrator: one path for every agent call

`call[T]` gains the call site and returns the call's signals:

```go
// site is where a call is made: its ledger position, and whether its stage commits.
type site struct {
	at     plan.At
	writer bool
}

// call is agent, the driver-event mapping and decode. A return outside
// opts.Schema decodes as nil.
func call[T any](ctx context.Context, o *orch, s site, prompt string, opts lsfrt.Opts) (*ret[T], []*plan.Signal, error)

// callSigs is the mapping that call and the parallel batches share. A
// non-null value is checked against schema, which catches seeds and stubs
// (the runtime validated live returns). Violations, from cr.Violations or
// from that check, null the value.
func (o *orch) callSigs(s site, cr lsfrt.CallResult, label string, schema json.RawMessage) (value json.RawMessage, sigs []*plan.Signal)
```

`callSigs` logs unknown flags and violations. Callers of `parallelAgents` call `callSigs` once per slot, with that slot's options' schema and their own site.

**One precedence rule, at every site:**
1. The call's signals are appended to the stage's signal list.
2. The existing path runs unchanged: decode, null → `agent-null`, and the stage's own reported records and stops (readings, implement's records, distress, drift, gates, done condition, findings).
3. A **unit stage** checks the call's stops at one point per stage, after the stage's reported records are appended. The existing own-stop aborts stay where they are and run first where they come earlier. The check points:
   - **plan:** after the readings and size rows;
   - **implement:** null, then drift (an existing abort, before the records, as now), then `SignalsFromImplement` and distress, **then the call-stop check**, then gates and the done condition. A valid implement with `uncommitted-at-return` keeps its deviations, deferrals and discoveries; its call rows are in the ledger whichever abort fires;
   - **reviewers and fixes:** inside `critiqueLoop` (below).
4. **Loop stages** (merge, gate, cleanup, re-plan, decision re-plan, termination, obligations, direct-edits) keep processing their value through the existing path. The call's signals join that stage's signal list and surface through its existing evaluation, so a stop is returned where the stage's own stops are today, with the same `stopShas`.
   - A merge stop keeps what the merge did and continues through gate, cleanup and re-plan, as any wave stop with units merged does (requester decision).
   - A re-plan or decision re-plan stop keeps today's escalation: `plan_sha` is the stage's own commit, which carries the amendments it absorbed. A relaunch re-plans over decisions alone (`wave.go:195-198`), so stopping earlier would lose the merged reports' items and the committed decisions.
   - Termination's call signals join `termEval`.

   The design's escalation-record paragraph (`lsf-workflow.md:587-593`) is unchanged. Where a stage's commit is uncommitted, the stop names the paths, and the requester decides at the relaunch.

A null value adds `agent-null` wherever it does today, including beside a `schema-violation` (S6) and beside `uncommitted-at-return` (a writer that timed out mid-edit). Each row states a different fact. Cleanup is the exception, as today: its null is logged, not signalled.

**Writer and reader, per site.** A writer is a stage that commits.

| Writer | Reader |
|---|---|
| plan, fix-plan, implement, fix-code, merge, replan, decision-replan, termination, direct-edits | critique-plan, critique-code, lens-review, critique-design, review-gate, cleanup, obligations |

**`critiqueLoop`:**
- `reviewer` gains `sigs []*plan.Signal`, and `fix` returns `(*ret[fixResult], []*plan.Signal, error)`.
- Each round, the loop appends the reviewers' signals, then runs the existing null check, which still adds `agent-null`.
- It returns `gateOutcome{reason: "call-stop"}` if any appended signal is a stop. The check runs before findings are read. A round's findings come from reviewers that may have stopped, so they are not used.
- The fix's signals are handled the same way.

**Findings stops.** After `SignalsFromFindings`, a `schema-violation` aborts the unit at that gate, consistent with rule 3. Under orchtest it is unreachable through `call`, because `callSigs` nulls an invalid value. It is reachable only if a decoded `Finding` slips past validation, and it is kept for consistency.

**Cleanup.** Its call signals join `waveSignals`. Its null stays a log line. As a reader, its uncommitted changes are only a record, but a `schema-violation` or `skill-not-loaded` at cleanup stops the run, as those kinds do everywhere. The comment "Cleanup never signals: a failure leaves residue, not risk" becomes "Cleanup's null is logged, not signalled". The design paragraph is `code-merge-cleanup`'s.

**Null-path stops.** The decision re-plan, termination and obligations currently call `l.stopped` with only the null signal. They now evaluate the call's signals with it (`EvaluateWave`, `l.record`), so records are not lost.

**Gates.** `unrunGates` splits in two:
- **`omittedGates`.** None reported gives `"(no gate reported)"`. For a fix, it also covers each implement gate the fix did not report. Statement: `"gate(s) omitted: …"`.
- **`notRunGates`.** Status `not-run`. Statement: `"gate(s) not run: …"`.

`gateOmittedSignal` is the `gate-omitted` stop. When both apply, both signals are raised.

**SHA correction (S8).** After decode, for the unit plan, implement, fix-plan and fix-code returns, when `o.git != nil`:

```go
// checkSha resolves branch's tip. When reported is not a prefix of it
// (at least 7 characters), it returns the tip and a sha-corrected record.
// An unresolvable branch returns reported unchanged and no signal: the
// stages after it fail on the branch itself. Any other git error, with
// ctx live, returns reported and a sha-corrected record saying the check
// failed. With ctx done, it returns a StopCancelled stop.
func (o *orch) checkSha(ctx context.Context, at plan.At, branch, reported string) (string, *plan.Signal, error)
```

The tip overwrites the result's `Sha`.

**SHAs that are consumed are out of this check.**
- Merge's `planSha`, `architectureSha` and `merges[].sha` are deferred to `code-merge-cleanup`, whose done condition rejects any that does not resolve. This is a deferral for the completion report.
- The re-plan's and termination's `planSha` are read by `loadPlan`. A SHA that does not resolve fails the load step, which is a `stage-fault` stop. A resolving but stale one is caught by the uncommitted check when the commit was not made.

**Code-stage fault.** `stageFaultSignal` emits kind `stage-fault`. D19 is amended.

**Signature fallout.** `runtime_test.go` (`call[T]`, `parallelAgents` with nil schemas) and every test calling `call` are updated. A nil schema means no check (§2.2).

### 2.5 Divergences, and the parity tools

`orchestrator/doc.go` records the divergences, with `plan/doc.go` pointing to D20:
- **D19, amended.** Code-stage faults use kind `stage-fault`.
- **D20.** The fail-closed mappers (§2.3). The JS fails open.
- **D21.** Driver events. Every agent return's `CallResult` is mapped to signals, and every return is checked against its stage schema; a violation is a `schema-violation` stop. The JS has no `CallResult` and no schema check.
- **D22.** `gate-omitted` is split from `gate-not-run`.

**`tools/graphparity`:**
- Before comparing the constants, it filters the Go-only kinds (D19, D21, D22) out of `STOP_KINDS` and `RECORD_KINDS`. Each filtered kind must be absent from the JS lists.
- `plan/testdata/signals/mixed.json:263` has a merge with status `"weird"`, which D20 changes from a `claimed-disjoint-conflict` record to a `schema-violation` stop. That changes the fixture's stop rule, digest and ledger too. Step 1 picks the comparison after reading `graphparity`'s fixture handling, under two constraints:
  - Go's D20 row is asserted;
  - every other comparison stays exact.

  Candidates: rewrite that row to its JS shape before the stop rule runs on the Go side; or split the fixture so the out-of-schema entry is compared alone. Any JS coverage requirement is kept.

**`tools/promptparity`:**
- Before comparing, it rewrites the Go side over the outcome, the logs and every prompt string:
  - kind `gate-omitted` becomes `gate-not-run`, and the statement prefix `gate(s) omitted:` becomes `gate(s) not run:`;
  - kind `stage-fault` becomes `agent-null`.

  This is a documented divergence (D19, D22). Stop text reaches the re-plan prompt and the escalation questions, so the rewrite covers them.
- A scenario whose stub return fails its schema has its fixture fixed. The comparison is not loosened.

### 2.6 Design of record

**`lsf-workflow.md::escalation`:**
- *Signal sources*: item 1 (structural) gains the driver-detected events, which the runtime or orchestrator observes about a call rather than reads from its result: permission denials, uncommitted changes at return, a skill not loaded, a guard kill after a result, a misreported SHA, and a result outside its schema.
- *Stop rule*: RECORD gains denials, a reader's residue, corrected SHAs and guard kills after a result. STOP gains a writer's uncommitted changes, a skill not loaded, a result outside its schema, an omitted gate and a code-stage fault.
- *Relaunch target*: the "Everything else" row gains the new stop kinds. For an uncommitted-change stop, the requester commits or removes the named paths, then relaunches:
  - **without decisions:** the replayed call re-checks the paths and passes once they are clean;
  - **with decisions:** every prompt changes and the stage runs again. An isolated writer's worktree is reused, so any dirt still in it is reported again.

Each bullet is added in the step that turns its behaviour on:

| Step | Bullets |
|---|---|
| 3a | denial, uncommitted, skill, guard kill, Relaunch-target row |
| 3b | schema |
| 3c | omitted gate, code-stage fault |
| 4 | SHA |

`decisionsFor` (`loop.go:31`) renders any kind generically, so it needs no change.

**`lsf-runtime.md`** (outside Scope; a planned deviation):
- The *Not in this unit* lines that point here are removed.
- Uncommitted detection, violations and their known limits are added under Runtime.

### 2.7 Risks and unknowns

| Risk | Resolution | When |
|---|---|---|
| Existing stub fixtures fail their schemas once every return is validated | Step 3b turns the check on alone. Fixtures are fixed (test only); a fixture that encodes deliberate JS behaviour is recorded | step 3b |
| A project's gates leave unignored files, so writers stop on `uncommitted-at-return` and readers add residue rows | Intended (fails safe); the statement lists the paths. One row per call, not per path. Noted for `declared-gates` and the drive docs | discovery |
| `git status` cost on large repos at every call | One porcelain call, plus hashes of dirty paths only (repo calls), with the state dir excluded before hashing | accepted |
| Parity scenarios exercise a fail-open branch whose output Go now changes | Normalise only the documented kinds; investigate anything else | steps 1, 3c |

No third-party or undocumented API is involved. Git's porcelain v1 `-z` format is stable by contract, and `hash-object` is plumbing.

### 2.8 Critique record

| Round | Critical | Significant | Disposition |
|---|---|---|---|
| 1 | 0 | 8 | All addressed. The writer-stop decisions for repo writers, a failed check and a merge stop went to the requester and are recorded in §1. `cleanup` was reclassified as a reader. Violations come through `CallResult`, there is one precedence rule, signals stay `structural`, the graphparity filter was added, step 3 was split, and design bullets land per step. Of the Minors: an empty schema means no check; renames and toplevel are handled, and the state dir is excluded before hashing; unknown flags are logged; the transient test uses a short backoff. Rejected: one residue row per stage per unit. The design keeps one row per call, which is already bounded. |
| 2 | 0 | 2 | Relaunch replay: verified that `header()` embeds both SHAs, and replay now re-checks. Rule 4 generalisation: answered in round 3. Minors addressed: the implement check point, cleanup's null, `structural` for findings violations, the S1 mapping, the Relaunch-target row, promptparity rewriting kind and statement, the `mixed.json:263` case, and statement wording. Kept as a known limit: a repo-writer null re-run takes the earlier dirt into its baseline. |
| 3 | 0 | 1 | The round-2 early stop for re-plan, decision re-plan and termination would lose merged items and committed decisions. It was withdrawn, so every loop stage keeps its existing path. Minors addressed: the Relaunch-target wording for relaunches with and without decisions, the `stage-fault` kind for an unresolvable SHA, the merge-SHA deferral named, and S1 narrowed to the producers that exist. |

## 3. Implementation steps

For each step:
- run `gofmt`, `go vet ./...` and `go test -race ./...` from `lsf-runtime/`;
- commit;
- run a per-step critique gate (critique-code), concurrently with the next step.

Each step amends the design-of-record bullets for the behaviour it turns on (§2.6).

**Step 0.** Save this document as `docs/plans/runtime-signals-plan.md`, and commit.

**Step 1: plan kinds and mappers.**
- **Changes:**
  - `plan/schema.go`: the new kinds.
  - `plan/signals.go`: `CallEvents`, `SignalsFromCall`, `ShaCorrectedSignal`, and the fail-closed `SignalsFromMerge` and `SignalsFromFindings`.
  - `plan/doc.go`: a pointer to D20.
  - `tools/graphparity`: the kind filter and the D20 normalisation.
- **Tests** (`signals_test.go`):
  - each event maps to its kind, in order;
  - writer vs reader;
  - the unchecked statement;
  - the list caps;
  - unknown flags are returned;
  - an out-of-schema merge status and severity give `schema-violation`;
  - `IsStopKind` holds for the new stop kinds.
- **Run:** graphparity.
- **Design of record:** none yet; no behaviour reaches a run.
- **Accept:** S6 (mapper half), S12 (graphparity). S1's plan-level half: every kind is listed and mapped.

**Step 2: runtime.**
- **Changes:**
  - `CallResult.Uncommitted` and `.Violations`, `record.Uncommitted`, the snapshot and the check (§2.1);
  - `ValidateOutput`;
  - `Step`'s ctx ending on `killCh`.
- **`testdata/claude` modes:**
  - `dirty:<relpath>` writes a file in cwd, then answers ok;
  - `modify:<relpath>` appends to a tracked file, then answers ok;
  - `denied` emits a result event with one `permission_denials` entry.
- **Tests:**
  - an isolated call leaving a tracked edit and an untracked file reports both, and not an ignored one;
  - a repo call with dirt beforehand reports only the new file and the further-changed file;
  - a transient first attempt that dirties the tree before the retry is reported, so the before-state is taken once;
  - a staged rename is one entry;
  - a state dir inside the repo is neither reported nor hashed;
  - a replay re-checks: an isolated worktree cleaned between launches reports nothing; a repo call reports only the journaled paths still dirty;
  - a broken git dir gives `uncommitted-unchecked`, and the flag is absent from the record;
  - an invalid-twice call returns `Violations`;
  - denials are journaled and replayed (the existing test, extended);
  - a step blocked on its ctx returns after `killCh` closes.
- `lsf-runtime.md` per §2.6.
- **Accept:** S2, S10, S6 (runtime half).

**Step 3a: orchestrator plumbing.**
- **Changes:**
  - `site`, the new `call[T]`, and `callSigs` without the schema check;
  - every `call[T]` and `parallelAgents` site, with the precedence rule, writer and reader, `critiqueLoop`, cleanup and the null-path stops.
- **Tests** (orchtest, `run_test.go`):
  - for each driver kind, a stub `CallResult` at implement (a unit stage) and at merge (a loop stage) shows the ledger row, and whether the run stops;
  - a valid implement with `uncommitted-at-return` keeps its deviations in the ledger and aborts before review;
  - a merge stop still reaches the re-plan;
  - a re-plan call stop keeps today's escalation (`plan_sha` the re-plan's commit, `pre_replan_sha` the wave's plan) and its verdict rows; a decision re-plan call stop likewise;
  - a flag on a null call, a parallel reviewer, the wave gate and cleanup reaches the ledger;
  - `TestCallResultReachesCallSites` asserts ledger rows.
- **Run:** promptparity, unchanged; no stub sets flags.
- **Design of record:** the denial, uncommitted, skill and guard bullets.
- **Accept:** S7, and S1 for `uncommitted-at-return`, `residue-at-return`, `skill-not-loaded`, `permission-denied` and `guard-killed-after-result`.

**Step 3b: the schema check.**
- **Changes:**
  - `callSigs` validates against the slot's schema, and maps `cr.Violations`;
  - fixtures are fixed as needed;
  - D21 is added.
- **Tests:**
  - an out-of-schema merge status and finding severity from the stub give a `schema-violation` stop beside `agent-null`, with no `claimed-disjoint-conflict` row, and the severity is named;
  - a `cr.Violations` null gives the same.
- **Design of record:** the schema bullet.
- **Accept:** S6 (orchestrator half), and S1 for `schema-violation`.

**Step 3c: gates and `stage-fault`.**
- **Changes:**
  - `gate-omitted` split from `gate-not-run`;
  - `stage-fault`;
  - D19 amended, D22 added;
  - promptparity normalisation.
- **Tests:**
  - `gate-not-run` stops after one implement call;
  - an omitted gate (none reported, or a fix dropping one) gives `gate-omitted`;
  - a code-stage fault gives `stage-fault`.
- **Design of record:** the gate and stage-fault bullets.
- **Accept:** S5, S9, S12 (promptparity), and S1 for `gate-omitted` and `stage-fault`.

**Step 4: SHA correction, and the real-runtime reproductions.**
- **Changes:**
  - `checkSha` at the four sites;
  - fake `claude` table entries gain optional fields: `shell` (a command run in the call's cwd before answering), `denials` (emitted in the result event's `permission_denials`) and `failFirst` (the first attempt returns a 529 error result). `cfg.Backoff` is shortened in these tests.
- **Tests** (`integration_test.go`, a real repo, `lsfrt.Run`):
  - (a) Denied commit. Implement's `shell` writes a file and does not commit; `denials` names `git commit`; the return's `sha` is HEAD. Expected: a stop with `uncommitted-at-return`, a `permission-denied` row in the ledger, and no `critique-code` or `lens-review` call.
  - (b) Denied branch delete. Cleanup's `denials` names `git branch -d`. Expected: a `permission-denied` row at `cleanup`, and the run ends `complete`.
  - Transient. Implement uses `failFirst`. Expected: `complete`, with no `agent-null`.
  - Schema on the live path. Merge returns an out-of-enum status twice. Expected: `schema-violation` and `agent-null`, with no `claimed-disjoint-conflict`.
  - SHA. Implement commits in its worktree and reports a wrong SHA. Expected: `sha-corrected` with the true tip, and the run continues.
  - `checkSha` unit tests against a temp repo: a prefix match, a mismatch, an empty SHA, an unresolvable branch, and a git error.
- **Design of record:** the SHA bullet.
- **Accept:** S3, S4, S6 (live), S8, and S1 for `sha-corrected`, on the real runtime, since orchtest has no repo. S1 is complete here.

**Step 5 (Phase 5).**
- Full-diff `phronithm:review`, with the static-analysis pre-scan and all applicable lenses, and a full-diff critique gate.
- End-to-end checks, docs, cleanup.
- Implementation summary, then the completion report.

Steps 1, 2, 3a, 3b, 3c and 4 run in that order, each depending on the one before. Step 3a needs step 1's mappers and step 2's `CallResult` fields. Step 3b needs `ValidateOutput`.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/runtime-signals-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit runtime-signals of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim (Phase 4 re-verifies the external gates against these
pins before implementing, recording the outcome in the implementation-summary): see §1 "Unit entry, verbatim" above. External gate: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`). Standing obligations: §1.
After Phase 5, produce the completion-report (contents per the
phronithm:feature skill's Produces) at docs/plans/runtime-signals-completion.md,
then present this continuation prompt to the requester:
Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
