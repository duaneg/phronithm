# Critique Gate

How phronithm skills invoke a critique pass as a quality gate on their own output.
Covers spawn mechanics, pass criteria, the adversarial pass, and iteration policy —
for critique axes and severity definitions, see
[critique](${CLAUDE_PLUGIN_ROOT}/docs/critique.md).

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

Default: no Critical findings remain. Significant and Minor findings are
reported alongside the pass, not fixed-and-re-run: in practice they are mostly
documentation nits and corner-case clarifications, cheaper to resolve if and
when hit than to verify exhaustively up front. A skill needing a different bar
must state so explicitly, with why.

**Calibration** is a separate, narrower thing: telling the critic what does or
doesn't count as a finding for this artefact type, without changing the severity
threshold. `phronithm:pin-behaviour` is the example — it tells the critic that a
characterisation suite's deliberate absence of correctness judgement is not a
defect, but an uncontrolled non-deterministic assertion still is. State the
calibration in the spawn prompt (verbatim, if the invoking skill provides fixed
wording — paraphrasing risks losing the distinction), and why it's needed. Same
justification requirement as a pass-bar change.

## What a pass does not mean

**A pass is not evidence the artefact is sound.** On artefacts whose
load-bearing content is a premise, a measurement, or a metric or procedure
definition, a clean framework pass carries close to no information about
whether such a defect is present. This is measured, not conjectured — see
[critic-fidelity-summary](${CLAUDE_PLUGIN_ROOT}/docs/investigations/critic-fidelity-summary.md):

- Against two corpora of real artefacts with known, later-confirmed
  Critical-class defects (mostly internal procedural/definitional flaws,
  one an external premise claim), clean-room recall was **0 of 14** at the
  pinned sonnet tier across a no-variable baseline and six further
  single-variable text interventions (85 scored runs total). The endpoint's
  own expressible range under the scoring rule was ~2 of 14 pooled — a
  positive control (an explicit hint, unmodified framework) recovered 2 of
  3 targeted findings, so the null bounds the effect at that floor rather
  than ruling out anything smaller.
- The critic reports **0–1 Critical findings per run** almost regardless of
  what it is given. Deflation — the right defect named but rated below
  Critical — is at least as common a failure shape as silence: 10 of the 14
  keyed defects were engaged at least once across all 85 runs.
- Raising the tier helps: an opus baseline engaged roughly twice as many
  keyed defects per run and majority-recovered 1 of 9, against sonnet's 0 —
  measured against the same low-ceiling bar above, and the sonnet pin itself
  was never tested as a candidate fix (open question, tracked separately).

Read a pass as *"no defect of the kind this gate reliably catches was found"*
— a floor, not a certificate. The gate still earns its place: it catches
structural, consistency and clarity problems cheaply and early, and its
findings are worth acting on. What it cannot do is license a claim that the
artefact's premises hold.

**When the artefact is premise- or measurement-bearing, the gate does not
license its soundness, and no gate step closes that gap.** The investigation
is closed on the fix side: seven text-edit conditions plus a toolset variant
and a decomposition probe found no framework-text or pipeline-structure fix
that closes it; a separate claim-isolation pipeline mechanism was killed
before build; and the presumption-of-defect adversarial pass did not
reproduce the recall the mandate decision assumed, under control (see
Adversarial pass below). A consumer needing premise soundness must obtain it
outside the gate, by requester-directed verification against the artefact's
referents — and even then, only for the narrower claim that the artefact
faithfully reports its referent; a flawed premise or experimental design
correctly transcribed is not caught by that check either.
`phronithm:investigate` applies this narrower fidelity check on itself: see its
[Settle / Review](${CLAUDE_PLUGIN_ROOT}/skills/investigate/SKILL.md#settle--review) Verify step.

## Adversarial pass

**Not a gate step.** A presumption-of-defect pass over premise- or
measurement-bearing artefacts (the `phronithm:adversary` agent) is not part
of the gate: clean-room validation at the gate's sonnet pin (bench
`adversary` arm, 2026-07-26) measured pooled Critical recall 0/14 at
majority — identical to the framework baseline — and 7 of 10 runs failing
the precision rule (6 findings judged fabricated, 2 tripping the same
refuted-defect probe). One of the six was the same recurring finding across
3 of 5 B1 runs, separately hand-adjudicated as real but immaterial to the
artefact's conclusion (Minor-severity at most, not a false claim), so the
failure is somewhat less total than the raw count suggests; one B1 run
correctly returned no findings against four refuted candidates. 5 of the 6
fabrications self-rated "verified against referent", so the confidence
field carries no adjudication signal on the one it got wrong. Full record:
`adversary-validation.md` (bench repo) and
[critic-fidelity-summary](${CLAUDE_PLUGIN_ROOT}/docs/investigations/critic-fidelity-summary.md).

Whether the persona detects at opus is unmeasured — the framework pass's
own tier effect (above) was never shown to transfer to this persona, which
has only run at sonnet. Reinstatement waits on an opus-tier re-run, not on
precedent; until that is run and passes, no adversarial step belongs to the
gate. Whether that re-run — or the sonnet pin generally — is worth
pursuing, given the recall bar above was never reachable by any
intervention, is an open question tracked separately, not resolved here.

A requester may still direct an adversarial pass explicitly — spawn at
`model: "opus"` rather than the sonnet pin above. Untested for this persona
(as above), but a single directed pass is cheap enough to pay the tier
premium on the chance it transfers. Its findings
are cases to adjudicate, never verdicts: settle each by **independent**
verification against its cited referent, without relying on its confidence
field; discard
findings citing no checkable referent; never let an unadjudicated finding
fail the gate or drive a fix. A fix applied for a confirmed finding
re-enters the framework gate's iteration loop before shipping.

## Iteration policy

Default: re-run the gate after each fix pass, applying the convergence profile
from [phronithm:critique](${CLAUDE_PLUGIN_ROOT}/skills/critique/SKILL.md)'s
Iterative use section — one source for round-count expectations, not one invented
per skill:

- **Pass**: no Critical findings remain (see Pass bar). Report any Significant
  or Minor findings; do not fix-and-re-run on them.
- **Escalate immediately** if a pass's summary reports a systemic problem —
  iterating will not resolve a structural defect.
- **Escalate** if the non-convergence signal fires (Critical findings
  persisting past round 3).
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
