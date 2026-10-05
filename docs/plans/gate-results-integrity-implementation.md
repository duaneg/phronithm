# gate-results-integrity: implementation summary

- **Branch:** `lsf-workflow`
- **Base:** `c901531`
- **Plan:** `docs/plans/gate-results-integrity-plan.md` (`73055ec`)

## External-gate re-probe

`node`: pinned v26.9.0, observed v26.9.0. Confirmed; no drift.

## Files modified

- `workflows/lsf/implement.js`
- `workflows/lsf/schema.js`
- `workflows/lsf/test/implement.test.js`
- `workflows/lsf-implement.js` (generated)
- `docs/design/lsf-workflow.md`: status paragraph and Lifecycle mapping stages 3–5
- `skills/lsf-drive/SKILL.md`: Relaunch section. This file is outside the Scope line.

## Per-step record

| Step | Commit | Record |
|---|---|---|
| 0 | `73055ec` | Plan saved; gate probed. |
| 1 | `5035e19` | Adds STOP kind `gate-not-run`. An implement return with a `not-run` gate, or with no gates, stops before review. Design units are exempt. The reviewers' "run the gates yourselves" fallback is removed. lsf-drive routes the new stop. Surprise: on a stop, the escalation does not carry the implement stage's records, so the test reads them from the ledger prompt instead. |
| 2 | `a7ba546` | Adds `FIX_CODE_SCHEMA` (a fix return with gates). `critiqueLoop`'s `onFix` may now return STOP signals. A `gateRun` record tracks which stage ran the gates, so the reviewers read the latest runs. The completion report shows the gates at the final HEAD and labels the verdict "(implement stage)". A design unit's fix keeps `FIX_SCHEMA`. Adds a red-gate boundary test (step-1 critique). |
| 3 | `6ac23eb` | Adds the `LONG_GATES` prompt text. Surprise: the planned poll put `$D` inside single quotes passed to `sh -c`, where it never expands, so the poll would never have finished. The quoting was corrected before commit. |
| 2 fixes | `b82a6c7` | Step-2 critique: the code fix is now given the commands to re-run. Tests cover a second fix round and a red fix gate. Status paragraph updated. |
| 3 fixes | `82cb6b9` | Step-3 critique: in the recipe, the detach redirects the subshell's fds, a PID check ends the poll when the gate dies, a compound command is grouped, and the exit status maps to green or red. A test runs the recipe under bash. |
| 4 | — | Rebuild leaves the tree clean (not stale). 136/136 tests pass. Lifecycle mapping read end to end. |

**Deviations from the plan:**
- The fix prompt lists the gate commands to re-run; the plan had only the phrase "every gate the implement stage ran".
- The recipe also records a PID and has an executable test.
- `LONG_GATES` is exported for that test.

## Per-step critique findings (critique-code, sonnet)

| Step | Severity | Finding | Disposition |
|---|---|---|---|
| 1 | Minor | The new helpers split the `gateResults` comment | Addressed in step 2 |
| 1 | Minor | The `FIX_CODE_SCHEMA` comment described behaviour that arrived only in step 2 | Resolved by step 2 |
| 1 | Minor | No test that a red gate still reaches review | Addressed (`a7ba546`) |
| 1 | Minor | The `referent` is joined with commas, which is ambiguous for commands that contain commas | Noted for Phase 5 |
| 1 | Minor | `lsf-improvements.md:80` still says a not-run gate "retries the stage" | Noted for Phase 5 (it is a note of candidate options, not the design of record) |
| 2 | Significant | The fix cannot see the implement stage's gate list | Addressed (`b82a6c7`) |
| 2 | Significant | Tests for a second round, a red fix gate, and a subset | The first two addressed (`b82a6c7`). Subset is a known limit (plan §2). |
| 2 | Minor | The verdict comes from the implement stage while "Gates at HEAD" comes from the fix | Labelled, per the plan's known limit |
| 2 | Minor | The design unit's `gateRun` is never updated, a latent coupling | No change: design reviewers get no gates and the report guards against empty |
| 2 | Minor | The status paragraph (line 19) is stale | Addressed (`b82a6c7`) |
| 3 | Significant | The detach holds the tool's fds; no `setsid` | Fds redirected; the test checks the detach returns at once. Harness process-group behaviour is not probed in workflow agents (R1). |
| 3 | Significant | The poll is unbounded if the gate dies | Addressed: PID and `kill -0`; a missing status file means `not-run` |
| 3 | Significant | A compound `<command>` is only partly redirected | Addressed: `{ <command>; }`. A command containing `exit` would still kill the subshell before it writes the status file; the result is then reported `not-run`, so it fails safe. |
| 3 | Minor | `.rc` race (empty file) | Not changed: negligible. The poll sees the file only after `echo` has run, and the agent reads it afterwards. |
| 3 | Minor | Status-to-colour mapping, gate numbering, "a few minutes" | Addressed |
| 3 | Minor | A sandbox may forbid writes to `.git` | Noted for Phase 5. The failure is safe (no `.rc`, so `not-run`). |
| 3 | Minor | The prompt test checks text only | Addressed: executable test |

## Success criteria

| # | Status | Target | Achieved | Evidence / upgrade path |
|---|---|---|---|---|
| S1: review after a fix reads the fix's gates | pass | proven | proven | Tests "a review round after a code fix reads the fix stage's gate runs…" and "a second fix round…" |
| S2: a not-run or missing gate never reaches review (implement and fix) | pass | proven | proven | Tests "an implement return with a gate not run…", "…no gate reported…", "a code fix whose gate did not run, or that reports none…". Design exemption and the red boundary are also tested. |
| S3: tests pass; built script not stale | pass | proven | proven | `node --test test/` 136/136, including `build.test.js`. Rebuild leaves the tree clean. |
| S4: prompts tell the stage how to wait out a long gate | pass | proven | proven for the text and the recipe's shell semantics under bash; `conjectured` for live workflow agents | Prompt-inclusion test plus the executable recipe test. Upgrade path: `shakedown-fixture`, with a gate longer than 600 s. |
| S5: the report's gates are those at the final HEAD | pass | proven | proven | `renderCompletionReport` test (`afterFix`) and the completion-report prompt assertion in the S1 test |
| S6: design of record and lsf-drive updated | pass | evidenced | evidenced | Read end to end in step 4 |

## Remaining phases
Complete Phase 5 (Review) as defined in the phronithm:feature skill.
