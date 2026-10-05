# Relay probe: results

Problem statement: [relay-probe-problem-statement.md](relay-probe-problem-statement.md). Transcripts under `~/.claude/projects/-home-duaneg-src-phronithm/<host session>/subagents/workflows/<runId>/`. Probe script: one haiku agent, prompt `Relay probe, condition <c>. Reply with the single word OK …`.

Turn boundaries are from the host transcript: entries of one turn share a `promptId`. A human turn starts at a user entry whose `origin.kind` is `human`; a notification turn at a `queue-operation dequeue` followed by a `task-notification` entry. A notification delivered mid-turn is a `queue-operation remove` and does not start a turn.

## Status: complete at 2.1.289; recorded in [lsf-workflow.md Constraints](../design/lsf-workflow.md#constraints-from-the-workflow-tool)

## Observations at 2.1.289

Host session `21d5d59b-b891-4722-8a41-8bd4a526256d`; host and agent transcripts record `"version":"2.1.289"`.

| Launch | Run | Turn started by | Launch position | Agent line 0 |
|---|---|---|---|---|
| after a `Bash` result | `wf_119eb6c3-fab` | human (host line 4) | mid-turn | relay of the turn's prompt, verbatim |
| first tool call of a notification-started turn | `wf_90db02e7-63b` | task notification (dequeue line 63, entry line 64) | first tool call | no relay; computed task |
| after an AskUserQuestion answer, same notification-started turn | `wf_956ed621-4cd` | task notification (as above) | mid-turn, answer at host line 84 | no relay; computed task |
| typed message | `wf_8286cecd-21c` | human (host line 140) | first tool call | relay of the typed message, verbatim |
| after an AskUserQuestion answer, same human-started turn | `wf_a61f647b-bc6` | human (as above) | mid-turn, answer at host line 148 | relay of the turn's typed message, not the answer |
| 16-line paste, collapsed to `[Pasted text #N]` (requester-reported) | `wf_a00b2346-6b4` | human (host line 178) | first tool call | relay of the paste's full text inside its `<pasted_content>` wrapper, not the placeholder |

## Observations at 2.1.288

Host session `b0789f26-7790-445f-ba21-3319ffe15b66`. Host and agent transcripts record `"version":"2.1.288"`; `claude --version` reported 2.1.289, the installed binary, not the running process. These rows do not satisfy the unit's 2.1.289 gate.

| Launch | Run | Turn started by | Launch position | Relayed text |
|---|---|---|---|---|
| after an AskUserQuestion answer | `wf_7f3d5f26-e91` | human (opening prompt, host line 11) | mid-turn | the opening prompt, whole and verbatim |
| after a `Bash` result | `wf_dd536c87-f59` | same turn | mid-turn | same |
| after a mid-turn task notification | `wf_5eb052d1-c04` | same turn | mid-turn | same |
| typed message | `wf_4fda358b-76c` | human (host line 171) | first tool call | the typed message, verbatim |
| 34-line paste, not collapsed (requester-reported) | `wf_999f9d4c-95c` | human (host line 200) | first tool call | the paste's full text, all 34 lines |

Each agent's transcript line 0 is a user turn opening `[Workflow harness — user request] The harness relays, verbatim and indented below, the user request that triggered this workflow run. … Where the computed task conflicts with this request, this request wins:`, followed by the relayed text indented.

## Hypotheses

- **H2** (an intervening tool result suppresses the relay): refuted at 2.1.288 and 2.1.289. Mid-turn tool results, an AskUserQuestion answer and a mid-turn notification did not suppress it in a human-started turn.
- **H1** (the session's most recent human prompt): refuted at 2.1.289 by `wf_90db02e7-63b`, which followed the human prompt `Resume relay-probe` with no later human prompt and carried no relay.
- **H3** (the prompt that started the current turn; none if a notification started it): not contradicted by the six 2.1.289 runs or the 2.1.288 runs. An AskUserQuestion answer does not start a turn and is not relayed: in a notification-started turn nothing is relayed (`wf_956ed621-4cd`), in a human-started turn the turn's prompt is (`wf_a61f647b-bc6`). The notification case rests on one turn (two runs sharing a `promptId`).

## Reconciliation with earlier runs

Every workflow run in the archipeligo-evolved and cartomancer-v2 project transcripts, grouped by launch: the Workflow call found in the host transcript, its turn's starting entry found by walking back past tool results and mid-turn notifications (`queue-operation remove`), and each agent's first user turn checked for the relay frame.

- **archipeligo (2.1.278–2.1.283).** All 20 runs launched in a human-started turn carried the relay in every agent, including 6 pasted prompts and 3 `/phronithm:lsf-drive` launches. All 8 launched as the first tool call of a notification-started turn (`wf_1ed25bc2-6be`, `wf_e714da30-985`, `wf_b4372b73-be0`, `wf_f97d8e41-bd7`, `wf_3f1363e4-8cf`, `wf_29189f36-57c`, `wf_78fa5d4d-514`, `wf_35d9d789-ba9`) carried none. This is postmortem §F's 20 of 28.
- **cartomancer-v2 (2.1.267–2.1.276).** All 15 runs launched at 2.1.267–2.1.273 carried no relay, every one from a human-started turn. All 3 launched at 2.1.274 or later carried it (`wf_e50c9431-6d1`, `wf_58847e5b-beb`, `wf_ea1ae00c-488`). This is the register's 3 of 18: the relay first appears between 2.1.273 and 2.1.274.
- **A resume is a launch.** `wf_58847e5b-beb` was launched in a human-started turn (19 agents relayed), then resumed with `resumeFromRunId` in a notification-started turn (cartomancer host `771809f3`, line 432); its 32 later agents carried none.
- **The relay is fixed at launch.** Agents of runs still live across a `/clear` relay the launching session's message (`wf_3478a815-312`, `wf_405a2c0f-0ec`, `wf_a50c2fbb-5a3`, `wf_ea1ae00c-488`). Each of those runs shows one non-relay agent in the post-`/clear` session directory: an agent in flight at the `/clear`, whose file there starts with a tool result, not its first turn.
- **A slash-command launch relays the command, not the skill body**: `<command-message>`, `<command-name>` and `<command-args>` (`wf_ec361be0-a69`, `wf_1078a8f1-b58` at 2.1.278; `wf_95020a23-3cc` at 2.1.281).

No earlier run contradicts H3 at 2.1.274 and later.

## Open obligations

- Unprobed at 2.1.289: a slash-command launch (seen only at 2.1.278–2.1.281, above) and a human message queued mid-turn.
- Multi-agent runs are not re-probed at 2.1.289; at 2.1.278–2.1.283 every agent of a human-turn launch carried the relay.
- One run per condition; determinism is not shown. The 2.1.288 rows were not re-verified by the critique.
