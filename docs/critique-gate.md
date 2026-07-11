# Critique Gate

How phronithm skills invoke a critique pass as a quality gate on their own output.
Covers spawn mechanics and pass criteria only — for critique axes and severity
definitions, see [critique](${CLAUDE_PLUGIN_ROOT}/docs/critique.md).

## Spawning

Spawn the `phronithm:critic` agent directly, with `model: "sonnet"` overriding its
`model: inherit` default, in a separate context from the artefact's author. Never
spawn a generic-purpose agent and instruct it to invoke `/phronithm:critique`
itself — its unrestricted toolset lets it decide to re-spawn `phronithm:critic` on
its own initiative, doubling the hop for no benefit. `phronithm:critic`'s own
toolset excludes Agent, so it structurally cannot do this.

Pass the artefact and its critique type/appendix (e.g. `critique-design`,
`critique-code`); leave type unset to let the skill infer it.

## Pass bar

Default: no Critical or Significant findings remain. A skill needing a different
bar (e.g. a per-finding cap, a pass-count limit) must state so explicitly, with
why — this is the exception, not the norm.

Re-run after fixes per the invoking skill's own iteration policy.
