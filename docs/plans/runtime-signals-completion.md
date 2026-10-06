# runtime-signals — completion report

**Unit:** `runtime-signals` of `docs/plans/lsf-improvements-breakdown.md` @ `85dced0`.
**Branch:** `lsf-workflow`; commits `aed6079..7571a7b`.
**Other documents:**
- plan: `docs/plans/runtime-signals-plan.md`
- implementation summary: `docs/plans/runtime-signals-implementation.md`

## Done-condition verdict

**Met, at `proven`.** Every clause is held by tests that run under `go test -race ./...`.

- **Each event reaches the ledger, and the decided ones stop.**
  - The requester decided the open question on 2026-10-07:

    | Disposition | Kinds |
    |---|---|
    | **Stop** | `uncommitted-at-return` (a stage that commits; also when the check fails), `skill-not-loaded`, `schema-violation`, `gate-omitted`, `stage-fault` |
    | **Record** | `residue-at-return` (a read-only stage), `permission-denied`, `sha-corrected`, `guard-killed-after-result` |

  - Tests:
    - `TestDriverEventsAtImplement` and `TestMergeCallStop` cover each kind.
    - `TestUncommittedPerSite` covers all 16 call sites, each with uncommitted changes and with a violation, and checks there is exactly one row.
    - `TestOnRuntimeShaCorrected` covers the SHA correction.
- **The runtime detects uncommitted changes at a call's return.** This is `runtime/uncommitted.go`, wired in at every return path after a spawn (`TestUncommittedEveryReturnPath`).
- **Postmortem §C and run #2, on the real runtime:**
  - **The denied commit reported with HEAD as its SHA.** The run stops before review with `uncommitted-at-return`, and the denial is in the ledger (`TestOnRuntimeDeniedCommit`). The test simulates the evidence: the fake `claude` writes the change, reports the denial and returns HEAD.
  - **The denied branch delete.** It is a ledger row, and the run completes (`TestOnRuntimeDeniedBranchDelete`).
  - **The transient 529.** It is retried, and the run completes with no `agent-null` (`TestOnRuntimeTransientRetried`).
- **`gate-not-run` is not retried.** One implement call is made, then the stop (`TestGateNotRunNotRetried`).
- **An out-of-schema merge status and finding severity.** A merge status is never `claimed-disjoint-conflict`, and a severity is never dropped. Both are `schema-violation` stops:
  - live (`TestOnRuntimeLiveSchemaViolation`);
  - seeded, or reported as runtime violations (`TestSchemaViolations`);
  - in the plan mappers (`TestFailClosedMappers`, and graphparity's D20 check).
- **Every agent call site reads its `CallResult`.** It goes through `call` or `callSigs`, and no `_` discard remains.
- **A code-stage fault has its own kind.** It is `stage-fault`, with the `"<stage> failed: <reason>"` statement.
- **A repeated signal ends a code step's git.** `Step`'s ctx ends on `killCh` (`TestStepCtxEndsOnKill`).
- **The tests pass.**
  - `go vet ./...` is clean.
  - promptparity: 55 scenarios identical.
  - graphparity: 145 cases identical.
  - staticcheck and golangci-lint are not installed.

## Deviations from the unit entry's scope and publishes

- **Outside Scope:**
  - `docs/design/lsf-runtime.md`: the runtime's uncommitted detection, the journal rows and the known limits. This was planned.
  - `docs/design/lsf-workflow.md::lifecycle-mapping`: two gate parentheticals, made true for the `gate-omitted` split.
  - `::wave-loop`: the cleanup sentence now says its call's driver events are signalled. The rest of that paragraph stays `code-merge-cleanup`'s.
  - `lsf-runtime/tools/graphparity` and `tools/promptparity`: normalisations for the documented divergences.
  - `CLAUDE.md`'s runtime line.
- **Publishes:**
  - the driver-event kinds (`plan.GoOnlyKinds`) and `plan.SignalsFromCall`;
  - `CallResult.Uncommitted` and `.Violations`;
  - `lsfrt.ValidateOutput`;
  - the flag constants and `lsfrt.NoOutput`.
- **Against the plan's S6:** stub returns are not re-validated by the orchestrator. Only seeds are. The stub stands in for the runtime, which validates every live return. Re-validating stubs broke promptparity's deliberately malformed scenarios (the JS tolerance branches). The `Runtime` interface comment states the contract.
- **Against §1's "the failure is not journaled":** a failed uncommitted check is journaled as `unchecked`, not as the flag, and a replay checks afresh. Leaving it out of the journal made a replay pass unchecked (step 2 critique).

## Deferrals

- **Checking merge SHAs (`merges[].sha`, `planSha`, `architectureSha`) → `code-merge-cleanup`.** That is its done condition. `checkSha` covers the unit plan, implement and fix SHAs, which nothing reads yet.
- **The Wave loop's cleanup paragraph ("a failure leaves residue, not risk") → `code-merge-cleanup`**, per its done condition.
- **`skills/lsf-drive/SKILL.md`'s stop-kind list and its "not answered by a decision" rule → `runtime-cli-drive`.**
  - The list lacks the new kinds.
  - An `uncommitted-at-return` stop is answered by committing or removing the named paths, then relaunching (`lsf-workflow.md`, Relaunch target).

## Discoveries affecting other units

- **`code-merge-cleanup`:**
  - Cleanup is classed as a reader. Its uncommitted changes are `residue-at-return`, and its denials are recorded. A cleanup in code keeps those signals only if it raises them itself.
  - `stage-fault` replaces D19's `agent-null` for every code stage.
  - Merge as code should check its SHAs with the same rule as `checkSha` (prefix, normalised, revision syntax rejected).
  - The completion-report tip read treats every git error as an unresolved branch. `checkSha` separates exit 1 from other errors.
- **`declared-gates`:**
  - Raise `gate-omitted` for a declared gate missing from a return.
  - When a fix both omits a gate and reports one not run, it is two stops: `gate-omitted`, then `gate-not-run`. The JS has one (D22).
  - A project whose gates leave unignored files makes every writer stop on `uncommitted-at-return`, and every reader record residue. The statement names the paths.
- **`runtime-cli-drive`:**
  - The drive must present the new stop kinds.
  - For `uncommitted-at-return` it must give the recovery: clean the named paths, then relaunch.
    - Without decisions, the replay re-checks the paths.
    - With decisions, the stage runs again.
  - In the main checkout, a requester edit made during a writer call stops the run (requester decision). The drive should say so.
- **`s2-permissions-design`:** denials are now ledger rows, and accumulate across attempts. A denial matters only when it leaves work uncommitted, and the uncommitted check catches that case.
- **Unresolvable branches.** A plan or implement stage that reports a branch which does not resolve is silent until the completion report stops on it. A later unit may want a stop earlier.
- **Reported branches are trusted.** `checkSha` checks the SHA against the branch the stage reports, not the worktree branch the runtime created.
- **A merge stop continues to the re-plan (requester decision).** The re-plan, termination, direct-edit and merge prompts commit named paths only (`ownFilesOnly`). That instruction is the only guard against a later commit sweeping in merge residue.
- **Test cost.** `go test -race ./orchestrator` takes about 77 s, up from 40 s. The five real-runtime runs spawn the race-instrumented test binary as the fake `claude`.

## Standing obligations

- **Register dispositions.**
  - **P-C7** (a transient 529 became `agent-null` and stopped run #2): **disposed.** The runtime retries transient errors with backoff, and on the real runtime a 529 at implement is retried with no `agent-null` (`TestOnRuntimeTransientRetried`).
  - **P-C8, denied commits:** **disposed.** The ledger itself has been code since `code-stage-interface`.
    - A writer whose commit is denied leaves its work uncommitted, and the run stops with `uncommitted-at-return` before anything builds on it. The denial is in the ledger.
    - A SHA that does not match its branch is corrected and recorded.
    - A denied commit that reports HEAD matches the tip, so only the uncommitted check catches it, and it does (`TestOnRuntimeDeniedCommit`).
    - Fabricated merge SHAs stay with `code-merge-cleanup`.
- **Design of record.** `lsf-workflow.md::escalation` was amended in the commit that turned on each behaviour:
  - `eebedcd`: denial, uncommitted, skill, guard kill, relaunch;
  - `09b6bf4`: schema;
  - `369d573`: gate-omitted, stage-fault;
  - `986aed6`: SHA.
- **Frozen JavaScript.** `git diff --stat 85dced0 -- workflows/` is empty.
- **`go.mod` stdlib-only.** `go.mod` is unchanged.

## External-gate re-verification

`lsf-toolchain`: `go version` reports go1.27.1, and `go.mod` pins `toolchain go1.27.1`. The pin is confirmed, with no drift.

## Execution signals

- **Context:** comfortable at completion.
- **Mid-implementation design decisions** (not taken at a design pass):
  - re-validate seeds only, not stub returns;
  - journal a failed check as `unchecked`, so a replay checks afresh;
  - compare content by streamed sha256, with size and mtime past 64 MiB, instead of `git hash-object`;
  - re-validate a journaled return on replay;
  - keep a null repo call's dirt in `<key>.dirt` for its re-run;
  - make `testConfig`'s repository a real git repository;
  - drop the dead `stage-fault` rewrite in promptparity and require the `gate-omitted` rewrite to be live;
  - reject revision syntax in reported branch names.

  Most of these came from critique findings rather than gaps in the design.
- **Critique load:**
  - Plan critique converged in round 3 (0 Critical).
  - Per-step gates on steps 1 to 4 raised no Critical findings and 13 Significant ones.
  - The full-diff critique raised 0 Critical and 4 Significant findings, and the lens review 0 Critical, 0 Significant and 18 Minor. All were addressed except the deferrals and discoveries above.

## Next step

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
