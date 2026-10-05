# Substrate spike: results

Problem statement: [substrate-spike-problem-statement.md](substrate-spike-problem-statement.md). Spike code: `workflows/lsf/spike/`. Claude Code 2.1.289; every headless transcript records `"version":"2.1.289"`. Node v26.9.0.

## Status: in progress — rung 1 complete; rung 2 (adapter) next

## Spike checks

| # | Kind | Outcome | Rigour | Evidence |
|---|---|---|---|---|
| 1 | overturn | **pass** (synthetic prompts) | evidenced, once | All 16 stage schemas in `implement.js` returned schema-valid `structured_output` on the first call, each at its stage's tier, with no repair (`spike/probe-schemas.js`). Real stage prompts are pending the end-to-end run. |
| 2 | overturn | **pass** via `--tools`; **fails** via `--agent` | evidenced, once | `--tools Read,Grep,Glob,Skill` with the plugin loaded: the agent called `Skill phronithm:critique`, read `critique.md` and `critique-code.md`, and returned valid findings. `--agent phronithm:critic` resolves headless with tools `[Read, Grep, Glob, Skill]` and runs the skill, but its tool list shuts out `StructuredOutput`: it ended `success` with prose and `structured_output: null`. |
| 3 | cost | no relay | evidenced, once per probe | No headless transcript contains `Workflow harness`; line 0 of each is the prompt. |
| 4 | cost | about +15% floor | evidenced, once per variant | Same trivial haiku prompt, cwd the phronithm checkout. Headless first request: 29,754 tokens; 29,008 with `--strict-mcp-config`. Workflow haiku agents in the same project (relay probes, 2.1.289): 25,552–25,753. |
| 5 | overturn | **pass** | evidenced, once | Headless opus at `--effort low`: each assistant entry records `"model":"claude-opus-5-5"`, `"effort":"low"`, `"perTurnEffort":"low"`. Haiku records `perTurnEffort: null`, as Workflow haiku agents do (archipeligo `wf_b21a2e97-f5f`). |
| 6 | overturn | pending | — | — |
| 7 | overturn | pending | — | — |
| 8 | cost | 16 concurrent, no failure (trivial calls) | evidenced, once | The 16 check-1 calls ran in parallel: 6–27 s each, all exit 0. Not yet tested under real stage load. |
| 9 | overturn | **pass** | evidenced, once | Opus/low, `--permission-mode dontAsk`, Bash allowed only for listed git subcommands, and asked to run `git branch -d`. The call was denied. The agent read `.git/refs/heads/stale` and returned `deleted: false` with the denial named. It returned in 10 s, created no `.claude/` directory and made no settings edit. The result's `permission_denials` lists the denied call. |
| 10 | cost | pending | — | — |

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
