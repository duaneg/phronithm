# Substrate spike: results

Problem statement: [substrate-spike-problem-statement.md](substrate-spike-problem-statement.md). Spike code: `workflows/lsf/spike/`.

- **Versions.** Claude Code 2.1.289; every headless transcript records `"version":"2.1.289"`. Node v26.9.0.
- **Fixture.** Every run used a repository from `spike/fixture.sh`: one feature unit, `add-mean`, with declared gates `node --test` and `node scripts/lint.js`, at the default tiers. The unit is trivial, so the runs test wiring, fault handling and resume, not stage quality.
- **Retained data.** Each run's telemetry, configuration and outcome are kept in [substrate-spike-data/](substrate-spike-data/). Fixtures, worktrees and full transcripts were in the session scratchpad and are not.
- **Achieved rigour.** `evidenced`, below the target's "transcripts kept": a figure can be checked against telemetry, not against the agent's full transcript. Each condition ran once unless a count is given.

## Status: complete — verdict `confirmed`

Every overturn check (1, 2, 5, 6, 7, 9) passed. The verdict carries three conditions. Each is an adapter requirement, not a change to the orchestrator:

1. **Check 2.** It passes only when a skill-naming prompt is sent as the skill's slash command and the driver checks that the skill loaded.
2. **Check 7.** It passes only with code steps journaled and orphaned calls reaped. The first resume design failed it (run 4); the corrected one passed (run 6).
3. **Check 9 and P-C6.** They passed on agent behaviour. The permission design that would enforce them is an open decision:
   - prefix allow rules cannot run the orchestrator's prompts (run 1);
   - the deny list the spike used can be bypassed;
   - `--permission-mode auto` was not run.

`implement.js`, `scheduling.js`, `graph.js` and `schema.js` ran unchanged under the driver in every run. That shows viability, not the production shape: the build estimate below edits all four.

## Runs

| Run | Injected | Outcome | Evidence for |
|---|---|---|---|
| 1 | none; prefix allow rules | stopped `done-condition-unevaluable`: every implement Bash call denied | the permissions requirement |
| 2 | none | `complete` in 2 min 48 s, 9 calls | C11; checks 1, 6, 8 |
| 3 | altered SHA and fabricated commit, omitted gate, 529, requester edit, untracked and ignored files, rejected ledger commit | stopped at `ledger`: no commit | P-C2, P-C7, P-C8, P-C9, R11, requester edit |
| 4 | kill mid-implement; kill after the ledger commit | stopped: spurious `agent-null` | check 7, first design (failed) |
| 5 | `git commit` denied to implement | stopped `gate-non-convergence` | check 9 |
| 6 | as run 4, plus an ignored file | `complete` | check 7, corrected; the P-C9 fix |
| 7 | a decision clearing a gate, with awkward scalars | `complete` | P-C4 |

## End-to-end path (C11)

Run 2 ran the whole path with `implement.js` unchanged.

| Stage | How it ran |
|---|---|
| load | code |
| plan | opus/high |
| plan critique | model |
| implement | model |
| code critique and lens review | model, concurrently |
| completion report | code |
| merge with plan bookkeeping | code |
| wave gate | opus/medium |
| cleanup | code |
| re-plan | opus/high |
| ledger append | code |
| termination | model |
| obligations | model |

**Branches.** The unit branch carried a plan commit, the implementation commit and the completion-report commit.

**`main` gained, in order:**

- the unit's merge commit;
- `docs(stats): record wave 1 completions` (entry archived, stub, Plan Maintenance line, `complete: true`);
- the re-plan's architecture wording correction;
- the ledger commit (heading plus 6 wave-1 rows).

No worktree or branch remained.

## Spike checks

| # | Kind | Outcome | Evidence |
|---|---|---|---|
| 1 | overturn | **pass** | All 16 stage schemas returned schema-valid `structured_output` on the first call at their stage's tier, using synthetic prompts (`check1-schemas.jsonl`). Over runs 1–7, 51 of 52 real calls were valid on attempt 0. The other was the injected 529, valid on retry. |
| 2 | overturn | **pass, conditional** | See [Skill invocation](#skill-invocation). |
| 3 | cost | no relay | No headless transcript (probes, runs, replays) contains the relay frame's marker, `Workflow harness`. |
| 4 | cost | about +13% floor | Same trivial haiku prompt, cwd the phronithm checkout, with `--strict-mcp-config` as the driver uses it: 29,008 tokens on the first request; 29,754 without the flag. Workflow haiku agents in the same project: 25,552–25,753. Mechanical stages pay no floor under S2. |
| 5 | overturn | **pass** | Every assistant entry records the requested model and effort. Seen at opus `--effort low` (probe), and at medium and high (run 2's wave gate and plan). Haiku records `perTurnEffort: null`, as Workflow haiku agents do. |
| 6 | overturn | **pass** | Runs 2, 3, 6, 7. Each worktree stage ran in a driver-created worktree on branch `lsf/<plan>/<label>` from the integration branch, and the unit branch merged `--no-ff`. |
| 7 | overturn | **pass, after a fix; scoped** | See [Kill and resume](#kill-and-resume). Covered: a kill mid-implement, and a kill between the ledger commit and its record. Not covered: a kill between the merge step's commit and its record, and a kill while concurrent reviewers run. |
| 8 | cost | no failure | 16 concurrent probe calls; concurrent reviewer pairs in every run; three drivers ran at once (runs 3–5). No session limit was reached. |
| 9 | overturn | **pass, on agent behaviour** | Probe: a denied `git branch -d` returned in 10 s with the denial named and no settings edit. Run 5: three Bash calls containing `git commit` were denied; the agent returned `met: false` naming the denial, with no settings edit and no plumbing workaround. In both cases `permission_denials` lists each denied call. The deny list in force does not stop `echo >> .claude/settings.json`, `git -C <dir> branch -d`, `sh -c` or `rm -fr`, so the property held by restraint (n = 2), not enforcement. |
| 10 | cost | (a) yes, by proxy; (b) yes | (a) A `setsid -f` process outlived the headless session that launched it, reparented to PID 1. The end of an interactive session was not tested. (b) Run 3: the lint gate was removed from the implement return; the driver ran both declared gates on a clean checkout of the returned commit. |

### Skill invocation

Check 2 has three parts.

**Read-only toolset with skill and appendix.** Met once, by a probe with `--tools Read,Grep,Glob,Skill`: the agent called `phronithm:critique`, read `critique.md` and `critique-code.md`, and returned valid findings.

- `--agent phronithm:critic` fails. Its tool list shuts out `StructuredOutput`, so the call ends `success` with `structured_output: null`.
- The orchestrator's review prompts need git commands, so the driver's reviewer role adds Bash, with commit, merge, rebase and the common list denied. That role is not read-only (`sed -i` and redirects pass).
- Under the Workflow tool, code critique and lens review ran as general agents, so they were not read-only either.

**Skill invocation.** Workflow agents called their skill in 354 of 356 critique, review and re-plan calls, across archipeligo and cartomancer-v2. Headless:

| Condition | Skill called or loaded |
|---|---|
| Run 2, prompts as written | re-plan yes; critique-plan, critique-code and lens-review no |
| critique-code prompt replayed as written | 2 of 3 |
| replayed with the critic agent's instructions appended | 0 of 3 |
| replayed, prefixed `/phronithm:critique` | 3 of 3: skill body in the transcript, absent from the run-2 control |

- 3 of 3 against 2 of 3 does not distinguish the conditions by count. The case for the prefix is its mechanism: the slash command puts the skill body into the prompt before the model runs.
- A skipped skill is silent: run 2's reviewers returned valid findings without it. The driver must check the stream for the skill's load and retry or flag a call without one.
- The replays reviewed a branch already merged into `main`, so their diff was empty. They test skill loading, not review depth.

**Type appendix.** Reading it after the skill loads is partial on both substrates. By string match, Workflow agents read it in 22 of 94 critique-code calls and 77 of 111 critique-plan calls. Headless, only the probe is evidence.

### Kill and resume

Each step below is a resume (launch 2 or 3) after an injected SIGKILL of the driver.

| Step | Run 4: first design | Run 6: corrected design |
|---|---|---|
| Completed model calls | replayed from cache (label + prompt hash) | same |
| Orphaned `claude -p` child of the killed driver | still running, PPID 1, while the resume re-spawned implement in the same worktree | terminated at start-up, matched by pid and session id |
| Completed code steps | re-executed; the completion-report step failed on a branch cleanup had deleted, giving a spurious `agent-null` stop | replayed from their journal record |
| Ledger append killed after its commit, before its record | the replayed run appended a second wave-1 block (`a7fc8c1`) | the intent record marked it interrupted; the step found its block committed and skipped. One wave heading; no duplicate commit |
| Outcome | `stopped` | `complete` |

## Fault injection (C12)

Each row is a postmortem §C fault kind at the stage S2 claims to protect, plus the three extra kinds the done condition names.

Limits:

- Each injection ran once.
- Run 3 carried seven faults at once, so their interactions are not separated.
- Each detector was written knowing its fault, so each row shows an anticipated fault caught; no unanticipated fault was probed.
- Rows marked *not injected* hold by construction: no agent transcription exists to fault.

| Fault | Stage | Injection | Outcome |
|---|---|---|---|
| P-C1 placeholder or truncated block | load | *not injected* | code returns the fenced block from `git show` |
| P-C2 rows overwritten or deleted; own rows altered | ledger | commit rejected by a pre-commit hook (run 3); kill after commit (runs 4, 6) | The new content is the committed file plus the rendered block, compared byte for byte after the commit. The rejected commit returned an empty SHA and the orchestrator stopped at `ledger`. The resume duplicate was removed by journaling (run 6). "Own rows altered" was not injected. |
| P-C3 copy dropped text | completion report | *not injected* | code writes the rendered text and compares the committed text with it |
| P-C4 invented `clears`; dropped fence | decision log | awkward scalars (`: `, ` #`, `"`, `\`) in the decision (run 7) | rendered from the decision objects and parsed back before the commit; the parse equals prior plus new. The clearance applied and the gated unit ran. |
| P-C5 `git branch -d` denied | cleanup | *not injected* | cleanup is code (runs 2, 3, 6, 7) |
| P-C6 settings edit after a denial | worker stages | denial (probe; run 5) | not observed (n = 2); not enforced (check 9) |
| P-C7 transient error | critique-code | a 529 injected at the driver (run 3) | The retry path ran: retried after 5 s and the run continued. A genuine failed call (unknown model) exits 1 with `terminal_reason: api_error`; the driver would retry it three times, so it needs error classification. Archipeligo's actual errors were 17 session limits; waiting one out is unbuilt. |
| P-C8 fabricated or wrong SHA | implement | `result.sha` tail altered and a fabricated `commits[]` entry added (run 3) | corrected from the branch tip and the fabricated commit dropped; both recorded in telemetry. A denied commit is detected by git, not reported (runs 3, 5). |
| P-C9 data deleted at cleanup | cleanup | untracked `runs/` file (run 3); ignored `node_modules/` file (runs 3, 6) | Untracked: kept. Ignored: **deleted in run 3**: `git worktree remove` without `--force` deletes ignored files. With a `status --ignored` check: kept (run 6). That check keeps every worktree in a project with ignored build output, so the cleanup policy is open. |
| R11 omitted gate | implement | lint gate removed from the return (run 3) | the driver ran every declared gate itself |
| Requester edit to the shared checkout | merge | uncommitted `CLAUDE.md` change (run 3) | survived the merge and bookkeeping commits uncommitted |
| Failed headless call | critique-code | as P-C7 | as P-C7 |
| Permission denial | implement | `git commit` denied (run 5) | reported by the agent and in `permission_denials`; the review stopped the run on the empty diff |
| Kill mid-stage | implement | SIGKILL 12 s into the call (runs 4, 6) | see [Kill and resume](#kill-and-resume) |

## Adapter requirements

These are what the host runtime must do beyond the options record's list. Each was found by a probe or run above.

- **Null on success.** A call can end `success` with `structured_output: null`, for instance after a denied `Skill`. The driver validates against the schema and treats a missing or invalid object as a failed attempt: retried once, then null.
- **Error classification.** Retry transient failures (overload, session limit) with backoff. Do not retry permanent ones such as `unrecognized_model`.
- **Toolsets, not `--agent`.** Agent types map to `--tools` lists. `--strict-mcp-config` keeps the user's MCP connectors out; without it, a restricted call listed twelve, including Claude Docs write tools.
- **Skill prompts as slash commands, with a load check** (check 2), by stage label.
- **Permissions.** Under `dontAsk`, allow rules match per subcommand. The orchestrator's prompts need compound commands: `&&`, pipes, assignments, `$(…)`. With prefix rules, run 1's implement stage had every Bash call denied. Runs 2–7 granted `Bash` whole and denied:
  - `git push`, `git worktree`, `git branch -d/-D`, `git config`, `git update-ref` and `rm -rf`;
  - edits under `.git/` and `.claude/`;
  - for reviewers only, commit, merge and rebase.

  That list is bypassable (check 9). The decision among it, single-command prompts (an orchestrator edit) and `--permission-mode auto` is open.
- **Denials are driver-visible.** `permission_denials` names each denied call. The denial text invites "other tools".
- **Gates run on a clean checkout of the returned commit.** Run 5's agent left its implementation uncommitted in its worktree; gates run there would have tested code no branch carries.
- **SHA checks.** `result.sha` is checked against the reported branch's tip and corrected from git. `commits[]` entries must resolve. `planSha` must resolve, or the return is null.
- **Journal.** Model calls and code steps are both keyed on label and prompt hash. A code step writes an intent first and a record after. Only an interrupted step re-executes, and its effect check stops it acting twice. That was shown for the ledger step only. `load` is not journaled, so a relaunch sees the pause ref.
- **Orphans.** Child pids and session ids are recorded; a launch terminates live children of its predecessor.
- **Cleanup never deletes ignored files** unless a policy names them rebuildable. That policy belongs with `retained-artefacts-design`.
- **A failed commit unstages its own paths.** Run 3 left the rejected ledger file staged in the main checkout.

## Build estimate (C13)

`conjectured`: the build has not been done or timed.

The spike driver is 584 lines; the parts in the table account for about 530 of them. The range below scales each part by what production adds. It is not derived from a measured ratio.

| Part | Spike | Production delta |
|---|---|---|
| Headless call: spawn, stream parse, validate, retry, telemetry | ~150 lines | error classification, session-limit wait on `rate_limit_event`, skill-load check, tests against a stub `claude` binary (exemplar `fake_ic.sh`) |
| Journal, resume, orphan reaping, kill file, status file | ~90 | merge-step and concurrent-kill coverage |
| Worktrees and cleanup | ~60 | the ignored-file policy |
| Code stages: load, decision log, ledger, completion report, merge with bookkeeping, cleanup | ~230, parsing 9 values out of prompt text | An orchestrator interface (`rt.code.*` or typed stage calls) in place of prompt parsing. This removes five mechanical prompts from `implement.js` and rewrites their tests. Merge conflicts go to a model call, which the spike reports `unresolvable`. |
| Driver-detected events as ledger signals: SHA corrected, denial, uncommitted at return, omitted gate, skill not loaded | telemetry only | new signal kinds in `schema.js` and `scheduling.js` |
| Declared gate commands (R11) | driver config | a plan-schema or project-configuration field, `graph.js`, `lsf-workflow.md` |
| Per-stage permission design | one choice | the open decision above |
| CLI and `phronithm:lsf-drive` integration: launch detached, status, pause, resume, watch | none | new |

The estimate is 1,500–2,500 lines including tests, as six to eight units:

- two large: the orchestrator interface plus code stages; the runtime and journal;
- three or four medium: signals, declared gates, permissions, CLI and lsf-drive;
- one or two small: docs, packaging.

**Not counted:**

- maintaining both substrates through the transition;
- retiring the generated `workflows/lsf-implement.js`, `build.js` and their test;
- Node at run time for plugin users, a new dependency.

**Language.** The estimate assumes the JavaScript orchestrator runs unchanged. The requester excluded JavaScript and TypeScript (2026-10-06), so the orchestrator is ported as well; the plan's R1 records the added size.

**R1.** The units S2 replaces carry effort labels, not sizes: `mechanical-stage-integrity` (large), `stage-failure-handling` (medium), `relay-mitigation` (small), and part of `drive-mechanics` (medium). On that label comparison, R1's "too large for this plan" is not clearly borne out.

## Open obligations

- **Permission design** (condition 3). Upgrade path: run the fixture under each of the three options, with a probe agent instructed to edit `.claude/settings.json` by shell.
- **Skill-load check** (condition 1). Upgrade path: build it and run run 2's fixture with the prefix, comparing findings with run 2. That also tests review depth, which the empty-diff replays did not.
- **Cleanup policy for ignored files** (P-C9), with `retained-artefacts-design`.
- **Kill coverage** (check 7). Upgrade path: kill between the merge step's commit and its record, and during concurrent reviewers.
- **Interactive-session detachment** (check 10a). Upgrade path: launch the driver with `setsid -f` from an interactive session, end the session, and read the driver's status file.
- **Session limits and error classification** (check 8, P-C7). Upgrade path: a multi-unit fixture with four or more concurrent units, or a run that reaches the limit.
- **Merge-conflict resolution by a model call** is not built. Upgrade path: a two-unit fixture with an overlapping edit.
