# Substrate spike: results

Problem statement: [substrate-spike-problem-statement.md](substrate-spike-problem-statement.md). Spike code: `workflows/lsf/spike/`. Claude Code 2.1.289; every headless transcript records `"version":"2.1.289"`. Node v26.9.0.

## Status: in progress — rungs 1–3 complete; fault injection (rung 4) next

## Spike checks

| # | Kind | Outcome | Rigour | Evidence |
|---|---|---|---|---|
| 1 | overturn | **pass** | evidenced, once | All 16 stage schemas in `implement.js` returned schema-valid `structured_output` on the first call, each at its stage's tier, with no repair (`spike/probe-schemas.js`, synthetic prompts). In run 2, all 9 real model calls (plan, critique-plan, implement, critique-code, lens-review, review-gate, replan, termination, obligations) returned valid output on attempt 0. |
| 2 | overturn | **pass**, given a slash-command prompt | evidenced: 3 of 3 | **Skill invocation.** Prompts sent as written, the headless reviewers ran their skill in 2 of 4 critique-code calls: run 2, then three replays of its prompt. With the critic agent's instructions appended, 0 of 3. Workflow agents ran it in 395 of 397 critique, review and re-plan calls (archipeligo and cartomancer-v2). Prefixed `/phronithm:critique`, the skill body loaded in 3 of 3 replays; it was absent from the run-2 control. The replays ran against a branch already merged into `main`: its diff was empty, so they test skill loading, not review depth. Reading the type appendix after the skill loads is partial on both substrates: by string match, Workflow critique-code agents read it in 22 of 94 calls and critique-plan agents in 77 of 111. **Toolset.**  `--tools Read,Grep,Glob,Skill` with the plugin loaded: the agent called `Skill phronithm:critique`, read `critique.md` and `critique-code.md`, and returned valid findings. `--agent phronithm:critic` resolves headless with tools `[Read, Grep, Glob, Skill]` and runs the skill, but its tool list shuts out `StructuredOutput`: it ended `success` with prose and `structured_output: null`. |
| 3 | cost | no relay | evidenced, once per probe | No headless transcript contains `Workflow harness`; line 0 of each is the prompt. |
| 4 | cost | about +15% floor | evidenced, once per variant | Same trivial haiku prompt, cwd the phronithm checkout. Headless first request: 29,754 tokens; 29,008 with `--strict-mcp-config`. Workflow haiku agents in the same project (relay probes, 2.1.289): 25,552–25,753. |
| 5 | overturn | **pass** | evidenced, once | Headless opus at `--effort low`: each assistant entry records `"model":"claude-opus-5-5"`, `"effort":"low"`, `"perTurnEffort":"low"`. Haiku records `perTurnEffort: null`, as Workflow haiku agents do (archipeligo `wf_b21a2e97-f5f`). |
| 6 | overturn | **pass** | evidenced, once | Run 2. Each worktree stage ran in a driver-created worktree on its own branch (`lsf/<plan>/<label>`) from the integration branch. The unit's branch carried its plan, implementation and completion-report commits, and merged `--no-ff` into `main`. The cleanup step then removed all 5 merged worktrees and their branches. |
| 7 | overturn | pending | — | — |
| 8 | cost | no failure at 16 concurrent | evidenced, once | The 16 check-1 calls ran in parallel: 6–27 s each, all exit 0. Run 2 ran its two reviewers concurrently, and the six replays ran in parallel. Not tested: a session limit under sustained load. |
| 9 | overturn | **pass** | evidenced, once | Opus/low, `--permission-mode dontAsk`, Bash allowed only for listed git subcommands, and asked to run `git branch -d`. The call was denied. The agent read `.git/refs/heads/stale` and returned `deleted: false` with the denial named. It returned in 10 s, created no `.claude/` directory and made no settings edit. The result's `permission_denials` lists the denied call. |
| 10 | cost | (a) a detached driver outlives a headless launcher; (b) pending | (a) evidenced once, by proxy | (a) A headless haiku session ran `setsid -f ./sleeper.sh` and exited; the sleeper kept running, reparented to PID 1. Not tested: the end of an interactive session, which this session cannot cause on itself. |

## Adapter requirements found

Each item below is something the host runtime must do that the options record did not list.

- **A null return can be a success.** Two calls ended `subtype: success`, `is_error: false`, with `structured_output: null`:
  - a call whose `Skill` use was denied under `dontAsk`;
  - a call whose agent type left out `StructuredOutput`.

  The adapter treats a missing `structured_output` as a null return and validates it against the schema itself.
- **Agent types map to a toolset, not `--agent`.** The critic's role is `--tools Read,Grep,Glob,Skill` plus an `--allowedTools` entry for `Skill`. If the critic's own instructions are wanted, they need `--append-system-prompt`.
- **MCP connectors load unless `--strict-mcp-config`.** `--tools` restricts built-ins only. With the user's configuration a restricted call still listed eleven claude.ai and arXiv MCP tools, including Claude Docs write tools.
- **`dontAsk` needs an explicit allow list per stage.** Without one, every tool but the structured output is denied, `Skill` included.
- **Denials are visible to the driver.** `permission_denials` in the result names each denied call, so the driver can record or stop on a denial without relying on the agent's report (P-C5, P-C6, P-C8).
- **The denial text invites a workaround.** It reads "You *may* attempt to accomplish this action using other tools". With Edit or Write allowed, an agent could rewrite `.git/refs` directly. The probe agent only read the ref. A stage's allow list should not grant file writes under `.git/`.
- **Prefix allow rules cannot express the orchestrator's prompts.** Under `dontAsk`, allow rules are matched per subcommand. The prompts require compound commands, such as `git reset --hard <base> && git clean -fd`, `git show <sha>:<path> | sed -n …`, shell variable assignments, and the long-gate recipe's `$(…)` and `&`. Run 1 listed `Bash(git *)`, `Bash(node *)` and similar: every Bash call of the implement stage was denied, and it returned `doneCondition.evaluable: false`, so the run stopped `done-condition-unevaluable`. The build has three ways out:
  - grant `Bash` whole and restrict with deny rules (the spike's choice from run 2);
  - rewrite the prompts to single commands;
  - run worker stages under `--permission-mode auto`, the classifier, as Workflow agents do today.
- **Gates run on a clean checkout of the reported commit.** The driver runs declared gates in a detached worktree at the returned SHA, not in the agent's worktree, where uncommitted changes could turn a gate green.
- **A prompt naming a skill goes as its slash command** (check 2). The driver prefixes `/phronithm:critique`, `/phronithm:review` or `/phronithm:breakdown` by stage label.

## End-to-end run (C11)

Run 2: fixture `spike/fixture.sh`, one feature unit (`add-mean`), declared gates `node --test` and `node scripts/lint.js`, default tiers. Implement.js is unchanged.

**Outcome:** `complete`, 1 unit over 1 wave, in 2 min 48 s.

**Model calls.** 9, all valid on the first attempt, $2.36 in total:

| Stage | Duration | Cost |
|---|---|---|
| plan, opus/high | 44 s | $0.31 |
| critique-plan | 15 s | — |
| implement | 17 s | — |
| two reviewers, concurrent | 14 s | — |
| wave gate, opus/medium | 16 s | — |
| re-plan, opus/high | 33 s | $0.49 |
| termination | — | — |
| obligations | — | — |

**Code steps.** Each took under 0.25 s: load ×3, completion report, merge with plan bookkeeping, cleanup, ledger.

**Git on `main`, in order:**

- the unit's merge commit;
- `docs(stats): record wave 1 completions` (entry archived, stub, Plan Maintenance line, `complete: true`);
- the re-plan's `document-text` correction to the architecture document;
- the ledger commit.

No worktree or branch remained.

The ledger holds the heading and the wave-1 rows. The digest shows 7 Minor findings, 1 architecture note and 1 architecture amendment.

Run 1, the same path with prefix allow rules, stopped at implement (see the prefix-rule finding above).

