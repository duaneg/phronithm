# C4 probe: does the vision question elicit archipeligo's frozen world?

For [`lsf-empirical-work.md`](../design/lsf-empirical-work.md) §Criteria, C4. Run 2026-10-10.

## Protocol (fixed before any run)

**Question.** Does C4's question elicit the frozen-world gap from archipeligo's criterion 3 as written at Phase 1 v0? The question is "describe a system that passes this criterion and fails that passage".

**Inputs.** `docs/vision.md` and `docs/architecture/foundation-brief.md` at `arch@7f4fb94`, which is the pre-critique v0. Neither file mentions frozen, static or immobile worlds.

**Runs.** Each arm used model opus and headless `claude` 2.1.294, with `--tools ""`, `--safe-mode` and `--strict-mcp-config`, in an empty working directory, with no session persistence.
- **F (fresh), 4 calls:** `vision.md` plus the brief's success-criteria list only (v0 lines 32–45), asked C4 for criterion 3.
- **A (author), 4 calls:** `vision.md` plus the whole v0 brief, framed as the brief's author finishing Phase 1 step 6, asked C4 for every criterion.

**Scoring,** on the criterion-3 answer:
- **Hit:** a world that passes criterion 3 while showing no or negligible change over time.
- **Miss:** anything else.

**Reading:**
- A ≥ 3/4: C4 stands.
- A ≤ 2/4 and F ≥ 3/4: anchoring defeats the question.
- Both ≤ 2/4: C4 is weakened or reconsidered.

## Results

Total cost $0.95. The raw outputs are not committed, because the prompts carry archipeligo's brief and vision.

| Call | Frozen world | Other criterion-3 gaps named |
|---|---|---|
| F1–F4 | miss ×4 | all four: compute is a quota or fuel and never changes hands, so it is not a currency |
| A1 | miss | mindless entities; compute not exchanged; single tuned genesis; **short horizon** ("a horizon a declared multiple of the classifier window") |
| A2 | miss | single genesis plus **classifier run length** ("Run it 10× longer and it condenses"); no `Transfer` |
| A3 | borderline, scored miss | "native-mind policies that mostly `Idle` dominate" (framed as compute spend, not stasis); **slow condensation outlasting the classifier window**; native-minds-only staging |
| A4 | **hit** | "System A: frozen coexistence … Nothing evolves after the transient", with the clause "composition turnover … exceeds the classifier's null-model floor (no fixed point counts as persistence)"; mindless dominance; no exchange |

- **Frozen world:** F 0/4; A 1/4 strict, 2/4 lenient. Both arms are ≤ 2/4, so C4 is reconsidered.
- **Missing horizon:** 3/4 A calls named it, which is P-A3's defect. Archipeligo found it late: 6 of 40 runs collapse between 38k and 92k ticks.
- **Other gaps:** checked against the record below.

## Precision against archipeligo's record

A read-only pass over `arch@c220c62` classified each criterion-3 gap the eight calls raised. VR is `docs/plans/foundation-vision-review.md`.

| Gap | Calls | Record |
|---|---|---|
| Compute never changes hands, so it is fuel, not a currency | 8/8 | **Real.** VR finding 3, "The economy is mechanism, not yet an economy": "No shipped mind issues `Transfer`, and every shipped mind bids zero" (VR:83-85) |
| Frozen or stationary world | 1–2/8 | **Real.** VR finding 2 (VR:52-66) |
| Persistence only within the horizon | 3/8 | **Real and accepted.** 6/40 collapse at 38k–92k; "accepted as a known hazard" (`foundation-brief.md:74-79`) |
| Single tuned genesis | 3/8 | **Real, partly.** One committed cell, certified on held-out seeds only; "held by choice … the world and genesis cell" (`foundation.md:2039`) |
| Uniform ontology unchecked (system actors act off-grammar) | 4/8 | **Real.** "The uniform ontology is nominal. Treasuries are accounting rows, not entities" (VR §5) |
| Selection for mindless entities | 4/8 | **Unknown.** Named in the brief as a risk and priced at stage 2; selection never tested |
| "Applicable" R6 requirements self-assessed | 3/8 | **Addressed.** `r6-static-check.md` uses a closed verdict vocabulary and records each deferral |
| Native minds read beyond percepts | 2/8 | **Unknown.** The signature takes a percept; no audit |

None of the gaps is contradicted by the record. Five are real, one was addressed, and two cannot be settled from the record. Of the vision review's four numbered findings, the question at Phase 1 found finding 3 in every call and finding 2 in at most two. It did not find findings 1 or 4.

## Planted cases

The C4 axes came from the runs above, so they were tested on two invented cases. The protocol was fixed before the cases were generated.

**Cases.** A separate headless opus call wrote two cases, each a vision, a brief with six criteria, and an answer key that respondents did not see:
- **ON:** a food-bank demand forecast scored one week ahead, against a vision that needs six weeks' lead. The gap is on the time axis.
- **OFF:** a tenant-filing tool scored on clerk acceptance, against a vision that says acceptance "is not the goal". The generator was asked for a gap off all three axes. As C4 words the instrument axis ("passing by satisfying the measure, not the passage"), the OFF gap arguably falls under it.

**Runs.** 3 calls per case without the axes (U) and 3 with them (S): 12 opus calls, same settings as above, about $2.3 in total.

| Case | U | S |
|---|---|---|
| ON | 3/3 | 3/3 |
| OFF | 3/3 | 3/3 |

**Reading.** Every cell hit its ceiling, so the protocol reads this as no detectable effect at this n. The one thing shown is that the axes did not narrow the search on OFF.

**Why the cases were too easy.** In both generated visions, the key passage all but contradicts the criterion outright: "A forecast for next week tells me something I mostly can't act on"; "Getting a tenant's answer through the clerk's window is not the goal". Archipeligo's vision states no such thing about criterion 3. A retest needs a gap that the vision implies but does not state.

## Limits

- n = 4 per arm, on one project, and the case was chosen with hindsight.
- Opus only.
- The author arm had the brief but not the real author's session, so this is not a test of anchoring. More context helped rather than hurt.

## Disposition

The requester chose to structure C4 along three fixed axes: time, starting conditions and instrument (§Criteria, C4). The axes come from these runs, so this case cannot test them, and the planted cases hit their ceiling. Their effect on a real case is untested; plan risk R42 carries it until a programme's vision review supplies one.
