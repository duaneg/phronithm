# Relay probe: problem statement

Unit `relay-probe`, [LSF improvements programme](../plans/lsf-improvements-breakdown.md) @ `83bebd1`. Register row P-F1; [postmortem §F](../design/lsf-postmortem-archipeligo-foundation.md#f-harness-relay-8).

## Claims

At Claude Code 2.1.289, for a Workflow-tool run launched from this host session, does the first user turn of a workflow agent's transcript carry a `[Workflow harness — user request]` block, and if so with what text, when the launch turn follows:

- **C-a** a typed message;
- **C-b** a tool result. Split, since the postmortem's two instances differ in what triggered the turn:
  - **C-b1** a tool result inside a turn a human message started (the launch is not the turn's first tool call);
  - **C-b2** an AskUserQuestion answer;
  - **C-b3** a task notification (a turn no human message started);
- **C-c** a pasted prompt (Claude Code's collapsed `[Pasted text #N]` paste).

All are load-bearing: `relay-mitigation` and `substrate-options` read each one.

## Rigour target

`evidenced`: one probe run per condition, its agent transcript's first user turn quoted, cited by run id. A single run per condition cannot show the behaviour is deterministic; that residual is stated, not tested.

## Known results

- Postmortem §F (archipeligo, 2026-09-19 to 10-02, versions before 2.1.289): 20 of 28 runs carried a relay; runs launched from an AskUserQuestion answer or a task notification carried none. Typed and pasted launches both carried one.
- Register Appendix B (cartomancer-v2): 3 of 18 runs carried one, each a short typed message.
- Observed form (archipeligo `wf_3478a815-312`): agent transcript line 0 is a user turn opening `[Workflow harness — user request] The harness relays, verbatim …`; line 1 is `[Workflow harness — computed task]`. Transcripts are at `~/.claude/projects/<project>/<session>/subagents/workflows/<runId>/agent-*.jsonl`.

Open mechanism question the claims bear on: is the relayed text the most recent human message (H1), or the user-role message immediately before the launch, so that any intervening tool result suppresses it (H2)? C-b1 separates them: H1 predicts a relay, H2 predicts none. The postmortem's lsf-drive launches mostly followed pre-flight tool calls yet carried relays, which favours H1.

## Attack ladder

Each rung is one minimal probe run: a one-agent haiku script whose agent replies `OK`; its transcript is read directly, not through the agent's report. Kill test for each: the first user turn of the agent transcript does or does not open with `[Workflow harness — user request]`.

1. **C-b1**: launch now, after this turn's tool calls. No requester action. Establishes C-b1 and separates H1/H2.
2. **C-b3**: start a background shell command, end the turn, launch on the notification. No requester action.
3. **C-b2**: launch immediately on the answer to an AskUserQuestion.
4. **C-a**: the requester types a short distinctive message; launch as the first tool call.
5. **C-c**: the requester pastes a distinctive multi-line block; launch as the first tool call. Also records whether the relay carries the paste's text or its placeholder.

Rungs 3–5 need the requester; rungs 1–2 are run first.
