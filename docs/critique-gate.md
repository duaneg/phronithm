# Critique Gate

How phronithm skills invoke a critique pass as a quality gate on their own output.
Covers spawn mechanics, pass criteria, and iteration policy — for critique axes and
severity definitions, see [critique](${CLAUDE_PLUGIN_ROOT}/docs/critique.md).

## Spawning

Spawn the `phronithm:critic` agent directly, with `model: "sonnet"` overriding its
`model: inherit` default, in a separate context from the artefact's author. Never
spawn a generic-purpose agent and instruct it to invoke `/phronithm:critique`
itself — its unrestricted toolset lets it decide to re-spawn `phronithm:critic` on
its own initiative, doubling the hop for no benefit. `phronithm:critic`'s own
toolset excludes Agent, so it structurally cannot do this.

Pass the artefact and its critique type/appendix (e.g. `critique-design`,
`critique-code`); leave type unset to let the skill infer it. Include an explicit
completion requirement in the spawn prompt: "Continue through to completion,
including all critique passes and fixes — do not stop mid-task to wait for
further instruction." A gate can run several rounds (see Iteration policy); without
this, the agent may stop early to check in.

When a spawn is a re-run within an iteration loop, state the round number in the
prompt — a freshly spawned critic has no memory of prior rounds and cannot apply
[critique](${CLAUDE_PLUGIN_ROOT}/docs/critique.md)'s round-dependent convergence
guidance (see Iteration policy) without being told which round it's on.

## Pass bar

Default: no Critical or Significant findings remain. A skill needing a different
bar — the severity threshold itself — must state so explicitly, with why. This is
the exception, not the norm: no current gate consumer needs one, and none does it.

**Calibration** is a separate, narrower thing: telling the critic what does or
doesn't count as a finding for this artefact type, without changing the severity
threshold. `phronithm:pin-behaviour` is the example — it tells the critic that a
characterisation suite's deliberate absence of correctness judgement is not a
defect, but an uncontrolled non-deterministic assertion still is. State the
calibration in the spawn prompt (verbatim, if the invoking skill provides fixed
wording — paraphrasing risks losing the distinction), and why it's needed. Same
justification requirement as a pass-bar change.

## Iteration policy

Default: re-run the gate after each fix pass, applying the convergence profile
from [phronithm:critique](${CLAUDE_PLUGIN_ROOT}/skills/critique/SKILL.md)'s
Iterative use section — one source for round-count expectations, not one invented
per skill:

- **Pass**: no Critical or Significant findings remain.
- **Escalate immediately** if a pass's summary reports a systemic problem —
  iterating will not resolve a structural defect.
- **Escalate** if the non-convergence signal fires (Significant or Critical
  findings persisting past round 3).
- **Hard cap**: do not iterate past round 4 without explicit requester
  instruction to continue.

Escalate means: present the remaining findings — and, for a non-convergence stop,
the suspected cause — to the requester, and wait for direction rather than
continuing to iterate unprompted. A skill with no synchronous requester to
escalate to must define its own equivalent (e.g. `phronithm:investigate` records
an open obligation with an upgrade path instead) and say what it is.

This is the default for every critique-gate consumer; very few skills should need
anything else. A skill needing a tighter or looser iteration policy must state so
explicitly, with why — same requirement as a pass-bar deviation, and the same
exception-not-norm expectation.
