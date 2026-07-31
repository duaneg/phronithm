# Subagent Protocol

Policy for spawning and using subagents (Task/Agent tools) across phronithm
skills and agents.

## Nesting

Subagents can spawn their own subagents (the Agent tool is available in
subagent sessions). Policy (uncontrolled nesting drains tokens fast): ordinary
data-gathering subagents, the single-pass advisory agents
(`phronithm:critic`, `phronithm:adversary`, `phronithm:editor` — none edits
its target directly; each returns findings, cases, or a change log for the
orchestrating session to act on), and ad hoc
verification subagents (spawned solely to check a result or reproduce a
finding) must not spawn further subagents — they return findings inline and
leave delegation to the orchestrating session. A subagent run to *exercise a
persona under test* (e.g. phronithm:persona-debug's supervised run) is not a
verification subagent in this sense; it follows the tested persona's own
nesting allowance.

Full-workflow orchestrators (sessions running phronithm:feature /
phronithm:large-scale-feature) are exempt: they may spawn the specific
critique/review agents their skill prescribes, but must not recurse beyond
that.

Enforce this with an explicit prohibition in agent persona files and subagent
spawn prompts. Invoke Agent-dependent skills (e.g. the [critique
gate](${CLAUDE_PLUGIN_ROOT}/docs/critique-gate.md)) from the orchestrating
session, not from a data-gathering subagent.

## File writes

Subagents cannot reliably write **deliverables** — sandbox write permissions
are not inherited, so a file a subagent produces as its output may not
survive outside its sandbox. Instruct subagents to return deliverable
results inline in their response. For structured data, use JSON. The
orchestrating session extracts the result and writes it to disk. Never
instruct a subagent to write a deliverable to a file path.

This does not bar a subagent from writing to a scratch/temp directory as
part of its own execution — e.g. `phronithm:adversary` re-running a cited
procedure via Bash. That write need not survive the subagent's session; only
the returned findings need to.
