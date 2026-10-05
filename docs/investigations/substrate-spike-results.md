# Substrate spike: results

Problem statement: [substrate-spike-problem-statement.md](substrate-spike-problem-statement.md). Spike code: `workflows/lsf/spike/`.

- Claude Code 2.1.289; every headless transcript records `"version":"2.1.289"`. Node v26.9.0.
- Runs used fixture repositories from `spike/fixture.sh`: one feature unit, `add-mean`, with declared gates `node --test` and `node scripts/lint.js`, at the default tiers.
- Run state and fixtures were in the session scratchpad and are not retained. The figures below are quoted from their telemetry.

## Status: complete — verdict `confirmed`

Every overturn check (1, 2, 5, 6, 7, 9) passed. The verdict carries three conditions. Each is an adapter requirement, not a change to the orchestrator:

1. Check 2 passes only when a skill-naming prompt is sent as the skill's slash command.
2. Check 7 passes only with code steps journaled and orphaned calls reaped. The first resume design failed it (run 4); the corrected one passed (run 6).
3. Worker stages need a permission design that admits the orchestrator's compound shell commands (run 1).

`implement.js`, `scheduling.js`, `graph.js` and `schema.js` ran unchanged under the driver in every run.

## Spike checks

| # | Kind | Outcome | Evidence |
|---|---|---|---|
| 1 | overturn | **pass** | All 16 stage schemas returned schema-valid `structured_output` on the first call at their stage's tier, using synthetic prompts (`spike/probe-schemas.js`). Over runs 1–7, 51 of 52 real calls were valid on attempt 0. The other was the injected 529, valid on retry. |
| 2 | overturn | **pass, conditional** (slash command) | See [Skill invocation](#skill-invocation). Skill body loaded in 3 of 3 slash-command replays. Prompt as written: 2 of 4. Workflow baseline: 395 of 397. Read-only toolset: `--tools Read,Grep,Glob,Skill`. `--agent phronithm:critic` fails: the agent's tool list shuts out `StructuredOutput`, so the call ends `success` with `structured_output: null`. |
| 3 | cost | no relay | No headless transcript (probes, runs, replays) contains `Workflow harness`. |
| 4 | cost | about +15% floor | Same trivial haiku prompt, cwd the phronithm checkout. Headless first request: 29,754 tokens; 29,008 with `--strict-mcp-config`. Workflow haiku agents in the same project: 25,552–25,753. Mechanical stages pay no floor under S2. |
| 5 | overturn | **pass** | Headless opus at `--effort low` records `"model":"claude-opus-5-5"`, `"effort":"low"` and `"perTurnEffort":"low"` on every assistant entry. Haiku records `perTurnEffort: null`, as Workflow haiku agents do. |
| 6 | overturn | **pass** | Runs 2, 3, 6, 7: each worktree stage ran in a driver-created worktree on branch `lsf/<plan>/<label>` from the integration branch. The unit branch, carrying plan, implementation and report commits, merged `--no-ff`. Run 2's cleanup removed all 5 worktrees and branches. |
| 7 | overturn | **pass, after a fix** | See [Kill and resume](#kill-and-resume). |
| 8 | cost | no failure | 16 concurrent probe calls; concurrent reviewer pairs in every run; three drivers ran at once (runs 3–5). A session limit under sustained load was not reached. |
| 9 | overturn | **pass** | Probe: a denied `git branch -d` returned in 10 s with the denial named and no settings edit. Run 5: the implement stage was denied `git commit`, tried twice, and returned `met: false` naming the denial. It made no settings edit and no plumbing workaround. The result's `permission_denials` lists each denied call. |
| 10 | cost | (a) yes, by proxy; (b) yes | (a) A `setsid -f` process outlived the headless session that launched it, reparented to PID 1. The end of an interactive session was not tested. (b) Run 3: the implement return was stripped of its lint gate; the driver ran both declared gates on a clean checkout of the returned commit. |

### Skill invocation

Under the Workflow tool, critique, review and re-plan agents called their skill in 395 of 397 calls, across archipeligo and cartomancer-v2.

| Headless condition | Skill called or loaded |
|---|---|
| Run 2, prompts as written | re-plan `phronithm:breakdown` yes; critique-plan, critique-code and lens-review no |
| critique-code prompt replayed as written | 2 of 3 |
| critique-code prompt replayed, critic agent's instructions appended | 0 of 3 |
| critique-code prompt replayed, prefixed `/phronithm:critique` | 3 of 3 (skill body present in the transcript; absent from the run-2 control) |

- The replays reviewed a branch already merged into `main`, so their diff was empty. They test skill loading, not review depth.
- Whether the agent reads the type appendix after the skill loads is model behaviour on either substrate. By string match, Workflow agents read it in 22 of 94 critique-code calls and 77 of 111 critique-plan calls.

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

| Fault | Stage | Injection | Outcome |
|---|---|---|---|
| P-C1 placeholder or truncated block | load | none possible: no transcription. Code returns the fenced block from `git show` | removed by construction |
| P-C2 rows overwritten or deleted; own rows altered | ledger | the commit rejected by a pre-commit hook (run 3); kill after commit (runs 4, 6) | New content is the committed file plus the rendered block, compared byte for byte after the commit. The rejected commit returned an empty SHA, and the orchestrator stopped `ledger`. A duplicate append on resume was removed by journaling (run 6). |
| P-C3 copy dropped text | completion report | none possible: code writes the rendered text | removed by construction; committed text is compared with the rendered text |
| P-C4 invented `clears`; dropped fence | decision log | awkward scalars (`: `, ` #`, `"`, `\`) in the decision (run 7) | rendered from the decision objects and parsed back before the commit; the parse equals prior plus new. The clearance applied and the gated unit ran. |
| P-C5 `git branch -d` denied | cleanup | — | removed: cleanup is code (runs 2, 3, 6, 7) |
| P-C6 settings edit after a denial | worker stages | denial (probe; run 5) | not observed. Deny rules cover `Edit`/`Write` under `.claude/` and `.git/` |
| P-C7 transient 529 | critique-code | injected failure (run 3) | retried after 5 s; the run continued |
| P-C8 fabricated or wrong SHA | implement | `result.sha` tail altered and a fabricated `commits[]` entry added (run 3) | corrected from the branch tip and the fabricated commit dropped; both recorded in telemetry. A denied commit is detected by git, not reported (runs 3, 5). |
| P-C9 data deleted at cleanup | cleanup | untracked `runs/` file (run 3); ignored `node_modules/` file (runs 3, 6) | Untracked: kept. Ignored: **deleted in run 3**. `git worktree remove` without `--force` still deletes ignored files. After a `status --ignored` check was added: kept (run 6). |
| R11 omitted gate | implement | lint gate removed from the return (run 3) | the driver ran every declared gate itself |
| Requester edit to the shared checkout | merge | uncommitted `CLAUDE.md` change (run 3) | survived the merge and bookkeeping commits uncommitted |
| Failed headless call | critique-code | as P-C7 | as P-C7 |
| Permission denial | implement | `git commit` denied (run 5) | reported by the agent and in `permission_denials`. The review stopped the run `gate-non-convergence`: empty diff, systemic. |
| Kill mid-stage | implement | SIGKILL 12 s into the call (runs 4, 6) | see [Kill and resume](#kill-and-resume) |

## Adapter requirements

These are what the host runtime must do beyond the options record's list. Each was found by a probe or run above.

- **Null on success.** A call can end `success` with `structured_output: null`, for instance after a denied `Skill`. The driver validates against the schema and treats a missing or invalid object as a failed attempt: retried once, then null.
- **Toolsets, not `--agent`.** Agent types map to `--tools` lists. `--strict-mcp-config` keeps the user's MCP connectors out; without it, a restricted call listed eleven, including Claude Docs write tools.
- **Skill prompts as slash commands** (check 2), by stage label.
- **Permissions.** Under `dontAsk`, allow rules match per subcommand. The orchestrator's prompts need compound commands: `&&`, pipes, assignments, `$(…)`. With prefix rules, run 1's implement stage had every Bash call denied and stopped `done-condition-unevaluable`.
  - Runs 2–7 granted `Bash` whole, with deny rules for `git push`, `git worktree`, `git branch -d/-D`, `git config`, `git update-ref`, `rm -rf`, and edits under `.git/` and `.claude/`. Reviewers additionally deny commit, merge and rebase.
  - The alternatives are rewriting prompts to single commands, or running worker stages under `--permission-mode auto`.
- **Denials are driver-visible.** `permission_denials` names each denied call. The denial text invites "other tools", so no stage may write under `.git/`.
- **Gates run on a clean checkout of the returned commit.** Run 5's agent left its implementation uncommitted in its worktree; gates run there would have tested code no branch carries.
- **SHA checks.** `result.sha` is checked against the reported branch's tip and corrected from git. `commits[]` entries must resolve. `planSha` must resolve, or the return is null.
- **Journal.** Model calls and code steps are both keyed on label and prompt hash. A code step writes an intent first and a record after. Only an interrupted step re-executes, and its effect check stops it acting twice. `load` is not journaled, so a relaunch sees the pause ref.
- **Orphans.** Child pids and session ids are recorded; a launch terminates live children of its predecessor.
- **Cleanup keeps ignored files.** A worktree with any `!!` entry in `git status --ignored` is kept.
- **A failed commit unstages its own paths.** Run 3 left the rejected ledger file staged in the main checkout.

## Build estimate (C13)

The spike driver is 584 lines. The production runtime needs the parts below. The figures are estimates, from the spike's measured size and the sizes of the units they replace; the build itself has not been timed.

| Part | Spike | Production delta |
|---|---|---|
| Headless call: spawn, stream parse, validate, retry with backoff and session-limit wait, telemetry | ~150 lines | wait on `rate_limit_event`, tests against a stub `claude` binary (exemplar `fake_ic.sh`) |
| Journal, resume, orphan reaping, kill file, status file | ~90 | — |
| Worktrees and cleanup | ~60 | — |
| Code stages: load, decision log, ledger, completion report, merge with bookkeeping, cleanup | ~230, parsing 9 values out of prompt text | An orchestrator interface (`rt.code.*` or typed stage calls) in place of prompt parsing. This removes five mechanical prompts from `implement.js` and their tests. Merge conflicts go to a model call, which the spike reports `unresolvable`. |
| Driver-detected events as ledger signals: SHA corrected, denial, uncommitted at return, omitted gate | telemetry only | new signal kinds in `schema.js` and `scheduling.js` |
| Declared gate commands (R11) | driver config | a plan-schema or project-configuration field, `graph.js`, `lsf-workflow.md` |
| Per-stage permission design | one choice | a decision among the three options above |
| CLI and `phronithm:lsf-drive` integration: launch detached, status, pause, resume, watch | none | new |

The estimate is 1,500–2,000 lines including tests, as six to eight units: two large (orchestrator interface plus code stages; runtime and journal), three or four medium (signals, declared gates, permissions, CLI and lsf-drive), one or two small (docs, packaging).

That is about the size of the middle of the plan it replaces: `mechanical-stage-integrity` (large), `stage-failure-handling` (medium), `relay-mitigation` (small), and part of `drive-mechanics` (medium). So R1's "too large for this plan" is not borne out on size. It adds a dependency, though: Node at run time for plugin users.

## Pivots

The claim did not move. Two design assumptions did:

- "Code steps are idempotent" became "code steps are journaled; idempotence covers only the interrupted step" (run 4).
- "Never `--force` keeps worktrees holding ignored files" was false (run 3). That sentence is in `lsf-substrate.md`'s P-C9 row.

## Open obligations

- **Interactive-session detachment** (check 10a). Upgrade path: launch the driver with `setsid -f` from an interactive session, end the session, and check the driver's status file.
- **Session limits under load** (check 8). Upgrade path: a multi-unit fixture with four or more concurrent units, or a long run that reaches the limit.
- **Review depth with the slash prefix.** Only skill loading was replayed, on an empty diff. Upgrade path: run 2's fixture with the prefix, comparing findings with run 2.
- **Merge-conflict resolution by a model call** is not built. Upgrade path: a two-unit fixture with an overlapping edit.
- **Every result above is from one run** of each condition unless counts are given.
