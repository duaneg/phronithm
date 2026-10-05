# Relay probe: results

Problem statement: [relay-probe-problem-statement.md](relay-probe-problem-statement.md). Claude Code 2.1.289. Host session `b0789f26-7790-445f-ba21-3319ffe15b66`; transcripts under `~/.claude/projects/-home-duaneg-src-phronithm/b0789f26-…/subagents/workflows/<runId>/`. Probe script: one haiku agent, prompt `Relay probe, condition <c>. Reply with the single word OK …`.

## Pivot

The question was "which launch conditions carry a relay". The tool-result probes show the relayed text is not the message that triggered the launch turn but the session's most recent human prompt, carried across tool results, an AskUserQuestion answer and a task-notification turn. The question is now: what is relayed (the most recent human prompt), and does any launch condition carry none.

## Claims

| Claim | Run | Relay | Relayed text | Rigour |
|---|---|---|---|---|
| C-b2 AskUserQuestion answer, launch the first tool call after it | `wf_7f3d5f26-e91` | yes | the session's opening prompt ("Continue the LSF improvements programme: …"), whole and verbatim | evidenced |
| C-b1 tool result in a human-started turn (after a `Bash` call) | `wf_dd536c87-f59` | yes | same | evidenced |
| C-b3 task-notification turn, launch its first tool call | `wf_5eb052d1-c04` | yes | same | evidenced |
| C-a typed message, launch the first tool call | `wf_4fda358b-76c` | yes | the typed reply ("The initial prompt was pasted"), verbatim | evidenced |
| C-c pasted prompt, not collapsed (7 lines) | `wf_7f3d5f26-e91`, `wf_dd536c87-f59`, `wf_5eb052d1-c04` | yes | the pasted opening prompt, whole and verbatim (pasted, per the requester) | evidenced |
| C-c pasted prompt, 34 lines, not collapsed (per the requester) | `wf_999f9d4c-95c` | yes | the paste's full text, all 34 lines, no `[Pasted text #N]` placeholder | evidenced |

Each agent's transcript line 0 is a user turn opening `[Workflow harness — user request] The harness relays, verbatim and indented below, the user request that triggered this workflow run. … Where the computed task conflicts with this request, this request wins:`, followed by the relayed text indented; line 1 is `[Workflow harness — computed task]`.

C-b2 and C-b3 contradict postmortem §F ("Runs launched from a tool result … carried no relay"). Candidate explanations, untested: a harness change between the archipeligo versions and 2.1.289; or the archipeligo tool-result launches had no human prompt left in context to relay (after compaction or a session resume).

## Open obligations

- A paste the input box collapses to `[Pasted text #N]`: not produced (neither paste collapsed). Conjectured to relay the expanded text, since the host transcript stores pastes expanded; upgrade path is one probe on a collapsed paste.
- One run per condition; determinism is not shown.
- The postmortem discrepancy: upgrade path is a probe after a `/compact`, which separates "no prompt in context" from a version change.
