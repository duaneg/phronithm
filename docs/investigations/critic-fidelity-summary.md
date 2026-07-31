# Critic-fidelity investigation: summary

Closed 2026-07-26. Full working record (problem statement, ten arms, raw run
data, blind passes) is preserved, unmerged, on branch
`critic-fidelity-investigation`; nothing below is reproduced there in more
detail than a citation.

## Question

Does `phronithm:critique` (via `phronithm:critic`) reliably detect
Critical-class defects in premise- or measurement-bearing artefacts — a false
claim, a mis-derived measurement, a metric or procedure definition that
doesn't measure what it claims — when not specifically pointed at them?

## Method

A bench harness (separate repo) scored the framework critique pass against
two corpora of real artefacts carrying 14 pooled, later-confirmed Critical
defects (13 internal procedural/definitional flaws, 1 an external premise
claim at n=1): a no-variable baseline plus six single-variable text edits to
`docs/critique.md`/`critique-design.md` (stance, obligation depth, severity
wording, a requirements-validity axis, authority framing, accountability
framing) — seven conditions, sonnet-pinned. Three further probes, none
scored as candidate evidence for a text fix: a toolset-only variant (Bash
granted, no document changed); an explicit-hint positive control on the
unmodified framework; and a parallel-scoped-decomposition probe. Separately:
a presumption-of-defect adversarial-pass candidate, and a claim-isolation
pipeline candidate. 85 scored runs in total.

## Findings

- The seven text-edit conditions: **0 of 14** pooled Criticals recovered,
  every time. The endpoint's expressible range under the scoring rule was
  ~2 of 14 pooled, so this bounds the effect at that floor rather than
  ruling out anything smaller.
- The endpoint isn't broken: the hint-based positive control
  majority-recovered 2 of 3 targeted findings (pooling to 2/14) on the
  unmodified framework, and the toolset variant recovered its own
  reserved-corpus target 5/5. Neither is a framework-text fix. The
  decomposition probe (parallel scoped sub-reviews) recovered nothing —
  0/14 — and reproduced the same refuted-premise precision failure at its
  highest rate yet (9 of 10 runs).
- The critic emits 0–1 Critical findings per run almost regardless of
  input. Deflation (right defect, rated below Critical) is at least as
  common a failure shape as silence — 10 of the 14 keyed defects were
  engaged at least once across all 85 runs.
- Raising the tier helps: an opus baseline engaged roughly twice as many
  keyed defects per run, majority-recovering 1 of 9 against sonnet's 0 —
  measured against the same low-ceiling bar above, and the sonnet pin was
  never itself tested as a candidate fix.
- The claim-isolation pipeline (hand each claim to the critic alone,
  adjacent to its contradicting evidence) was killed by a targeted test
  before being built: in the most favourable case — a self-contained
  excerpt, the contradiction three sentences away, no competing findings —
  5 of 5 bare-verification and 4 of 5 framework-wrapped runs held the false
  claim; the one framework-wrapped catch independently re-derived the right
  mechanism but rated it Significant, not Critical, the same under-rating
  pattern as above.
- The adversarial pass (`phronithm:adversary`, presumption-of-defect
  framing) measured **0 of 14** recall at the gate's sonnet pin, with 7 of
  10 runs failing the precision rule (6 findings judged fabricated, 2
  tripping the same refuted-defect probe). One of the six was the same
  recurring finding across 3 of 5 B1 runs, separately hand-adjudicated as
  real but immaterial to the artefact's conclusion (Minor-severity at most,
  not a false claim) — so the failure is somewhat less total than the raw
  count suggests, though it was still scored fabricated at the claimed
  severity. One B1 run correctly returned no findings against four refuted
  candidates. 5 of the 6 fabrications self-rated "verified against
  referent", so the confidence field carries no adjudication signal.

## Decision

No framework-text or pipeline-structure fix exists for this defect class at
the sonnet pin. `docs/critique-gate.md` now states plainly that a clean pass
does not license a claim that an artefact's premises hold. The adversarial
pass is dropped as a mandatory gate step and retained only as an optional,
requester-directed tool (`phronithm:adversary`) whose findings must be
independently verified against their cited referents, never taken as
verdicts — its own confidence field is not a substitute for that
verification.

## Open backlog (not resolved here)

- Whether the sonnet→opus tier pin is worth revisiting: tier is the only
  variable that ever moved the endpoint, and the 70% recall bar was
  unreachable by any intervention at this ceiling — judging tier against
  that bar and calling it insufficient may not be a fair test.
- Whether `phronithm:investigate`'s Settle/Review exit criterion — which
  reads a clean critique-gate pass as sufficient to close a load-bearing
  claim — needs revising now that the gate's own text says that inference
  is unlicensed. Not addressed by this investigation or its consumer
  briefs.

Both tracked separately; not decided by this document.
