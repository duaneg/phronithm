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
- **Other gaps:** their precision against archipeligo is unchecked.

## Limits

- n = 4 per arm, on one project, and the case was chosen with hindsight.
- Opus only.
- The author arm had the brief but not the real author's session, so this is not a test of anchoring. More context helped rather than hurt.

## Disposition

The requester chose to structure C4 along three fixed axes: time, starting conditions and instrument (§Criteria, C4). The axes are drawn from these runs, so this case cannot test them; their effect on another case is untested.
