# Subagent Protocol

Policy for spawning and using subagents (Task/Agent tools) across phronithm
skills and agents.

## Nesting

Subagents can spawn their own subagents (the Agent tool is available in
subagent sessions). Policy (uncontrolled nesting drains tokens fast): ordinary
data-gathering, `phronithm:critic`, and verify subagents must not spawn
further subagents — they return findings inline and leave delegation to the
orchestrating session.

Full-workflow orchestrators (phronithm:feature / phronithm:large-scale-feature
teammates) are exempt: they may spawn the specific critique/review agents
their skill prescribes, but must not recurse beyond that.

Enforce this with an explicit prohibition in agent persona files and subagent
spawn prompts. Invoke Agent-dependent skills (e.g. the [critique
gate](${CLAUDE_PLUGIN_ROOT}/docs/critique-gate.md)) from the orchestrating
session, not from a data-gathering subagent.

## File writes

Subagents cannot reliably write files — sandbox write permissions are not
inherited. Instruct subagents to return results inline in their response. For
structured data, use JSON. The orchestrating session extracts the result and
writes it to disk. Never instruct a subagent to write to a file path.
