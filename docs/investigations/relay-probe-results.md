# Relay probe: results

Problem statement: [relay-probe-problem-statement.md](relay-probe-problem-statement.md). Host session `b0789f26-7790-445f-ba21-3319ffe15b66`; transcripts under `~/.claude/projects/-home-duaneg-src-phronithm/b0789f26-…/subagents/workflows/<runId>/`. Probe script: one haiku agent, prompt `Relay probe, condition <c>. Reply with the single word OK …`.

## Status: stopped on the version gate

The runs below exercised **Claude Code 2.1.288**: the host and all agent transcripts record `"version":"2.1.288"`. `claude --version` reported 2.1.289, the installed binary, not the running process. The unit's gate pins 2.1.289, so no row below satisfies the done condition. The pre-flight check for the re-run is the `version` field of the host transcript, not `claude --version`.

## Observations at 2.1.288

Turn boundaries are from the host transcript: a turn starts at a `queue-operation dequeue` followed by a user entry whose `origin.kind` is `human` or `task-notification`. A notification delivered mid-turn is a `queue-operation remove` and does not start a turn.

| Launch | Run | Turn started by | Launch position | Relayed text |
|---|---|---|---|---|
| after an AskUserQuestion answer | `wf_7f3d5f26-e91` | human (opening prompt, host line 11) | mid-turn | the opening prompt, whole and verbatim |
| after a `Bash` result | `wf_dd536c87-f59` | same turn | mid-turn | same |
| after a mid-turn task notification | `wf_5eb052d1-c04` | same turn | mid-turn | same |
| typed message | `wf_4fda358b-76c` | human (host line 171) | first tool call | the typed message, verbatim |
| 34-line paste, not collapsed (requester-reported) | `wf_999f9d4c-95c` | human (host line 200) | first tool call | the paste's full text, all 34 lines |

Each agent's transcript line 0 is a user turn opening `[Workflow harness — user request] The harness relays, verbatim and indented below, the user request that triggered this workflow run. … Where the computed task conflicts with this request, this request wins:`, followed by the relayed text indented.

## Hypotheses

- **H2** (an intervening tool result suppresses the relay): refuted at 2.1.288. Mid-turn tool results, an AskUserQuestion answer and a mid-turn notification did not suppress it.
- **H1** (the session's most recent human prompt) and **H3** (the prompt that started the current turn) predict the same on every run above. They differ on a launch that is the first tool call of a turn started by a task notification, following a human turn: H1 predicts that human prompt is relayed, H3 predicts no relay.

No run tested the postmortem's tool-result case (a turn not started by a human). H3 is consistent with postmortem §F's "runs launched from a tool result … carried no relay"; the AskUserQuestion launch here is a conflict only if the archipeligo AskUserQuestion launches were also mid-turn in a human-started turn, which is unknown.

## Re-run protocol at 2.1.289

In a fresh session whose transcript records 2.1.289:

1. **Notification-started turn** (separates H1/H3): start a background command, end the turn, launch as the first tool call of the turn the notification starts. Confirm `dequeue` + `origin.kind: task-notification` before the launch.
2. **AskUserQuestion**: launch on the answer (mid-turn by construction).
3. **Mid-turn tool result**: launch after a `Bash` call.
4. **Typed message**: launch as the first tool call.
5. **Pasted prompt**: launch as the first tool call; a collapsed `[Pasted text #N]` paste if one can be produced, otherwise record that it was not.

## Open obligations

- The 2.1.289 re-run above.
- One run per condition; determinism is not shown.
