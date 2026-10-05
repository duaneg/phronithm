# Relay probe: results

Problem statement: [relay-probe-problem-statement.md](relay-probe-problem-statement.md). Transcripts under `~/.claude/projects/-home-duaneg-src-phronithm/<host session>/subagents/workflows/<runId>/`. Probe script: one haiku agent, prompt `Relay probe, condition <c>. Reply with the single word OK …`.

Turn boundaries are from the host transcript: a turn starts at a `queue-operation dequeue` followed by a user entry whose `origin.kind` is `human` or `task-notification`. A notification delivered mid-turn is a `queue-operation remove` and does not start a turn.

## Status: 2.1.289 re-run in progress

## Observations at 2.1.289

Host session `21d5d59b-b891-4722-8a41-8bd4a526256d`; host and agent transcripts record `"version":"2.1.289"`.

| Launch | Run | Turn started by | Launch position | Agent line 0 |
|---|---|---|---|---|
| after a `Bash` result | `wf_119eb6c3-fab` | human (host line 4) | mid-turn | relay of the turn's prompt, verbatim |
| first tool call of a notification-started turn | `wf_90db02e7-63b` | task notification (dequeue line 63, entry line 64) | first tool call | no relay; computed task |
| after an AskUserQuestion answer, same notification-started turn | `wf_956ed621-4cd` | task notification (as above) | mid-turn, answer at host line 84 | no relay; computed task |

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
- **H3** (the prompt that started the current turn; none if a notification started it): consistent with every run at both versions. An AskUserQuestion answer does not start a turn and is not relayed (`wf_956ed621-4cd`). This matches postmortem §F: launches from an AskUserQuestion answer or a task notification carried no relay, if those were in notification-started turns.

## Remaining runs at 2.1.289

1. **AskUserQuestion in a human-started turn**: launch on the answer.
2. **Typed message**: launch as the first tool call.
3. **Pasted prompt**: launch as the first tool call; a collapsed `[Pasted text #N]` paste if one can be produced, otherwise record that it was not.

## Open obligations

- The remaining runs above.
- One run per condition; determinism is not shown.
