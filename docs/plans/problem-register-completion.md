# Completion report: problem-register

Unit `problem-register` of [lsf-improvements-breakdown](lsf-improvements-breakdown.md) @ `f3a395a`. Branch `lsf-workflow`, base `f3a395a`. Deliverable: [lsf-problem-register](../design/lsf-problem-register.md).

## Done-condition verdict

**Met, at `evidenced`.**
- 47 rows: one per friction mechanism in postmortem §A–§G, plus improvements §1 (P-I1) and the resume-reset row (P-D6).
- Every row has a remedy-free statement, a cost citation (except P-D6, which has none by design), one of the four classes, and an empty disposition.
- Appendix A maps all 87 postmortem bullets, table rows and paragraphs.
- P-D5 states records calling regenerable data must-keep, and the disk that costs. P-D6 is `context-free defect` with no cost.
- The scratch `check-register.py` passes.
- `phronithm:critique` (critique-design, sonnet) passed with no Critical in rounds 1 and 2, and in the full-diff round 3 in Phase 5. P-D5 was revised after round 3 to address that round's Significant findings; no fourth round was run.

Class counts:

| Class | Rows |
|---|---|
| `recurs on cartomancer-v2` | 12 |
| `context-free defect` | 10 |
| `harness-imposed` | 2 |
| `archipeligo only so far` | 23 (11 of them qualified `not testable`) |

## Deviations

- **Publishes gains a Topic column**: a non-binding tag per row (`mechanical-stage`, `long-running`, `relay`, `driving`, `empirical-design`, `findings`, `state`, `other`). This is additive. The requester decided it at plan critique so that `substrate-options` can select rows by id.
- **Rows split per mechanism**, as the requester decided. Several postmortem bullets map to more than one id; Appendix A records which.

## Deferrals

None.

## Discoveries

1. **P-D5's cost differs from the postmortem's framing.**
   - What is kept now: 80 GB of run series in `~/src/archipeligo-runs/` (stage2-horizon-confirm 48 GB, b1-frozen-regime 26 GB, hoard-cell 6.6 GB). The close-out lists it under "Kept outside `main`". The must-keep requirement comes from three records: `horizon.md`, `bid-era.md` and `trigger.md`. There is also a 372 MB worktree, kept for commit `2f220af` and the path `trigger.md` cites.
   - The 83 GB the postmortem reports was the set cleanup removed (P-C9).
   - Bears on `empirical-unit-design` (retained artefacts), whose re-planned done condition weighs storage cost.
2. **Haiku transcription faults recur on cartomancer, outside the postmortem's list.**
   - Decision-log agents dropped the closing fence three times, and one of those drops stopped a load (P-C4).
   - Ledger appends altered their own new rows: they dropped a heading, which misfiled 64 rows, and altered rows in 6 of 27 appends.
   - The script once supplied a wave heading that did not match its rows.
   - 3 ledger agents returned SHAs with fabricated tails, which the script accepts (P-C8).
   - Bears on `substrate-options`, `mechanical-stage-integrity` and `stage-failure-handling`. The altered-new-rows and script-heading faults have no row; they are recorded in Appendix B under P-C2 and P-C8.
3. **Most §A and §G rows are untestable on cartomancer.** Cartomancer had no campaign, sampled or Phase 2-gated work, so 11 rows carry `not testable`. Risk R3 stands, and design units stating generality for P-A3, P-A4, P-A5, P-A7, P-A8, P-A12, P-A13, P-C3, P-G1, P-G3 and P-G4 have no second project.
4. **Several of cartomancer's §E instances arose without `lsf-drive`.** The skill was added on 2026-09-19 and never used there. P-E1, P-E3, P-E4, P-E5, P-E6, P-E7 and P-E9 recur under hand driving. This bears on `drive-decision-design` and `drive-mechanics`: some of these rows are not specific to the skill.
5. **Analogue qualifiers.** P-A1, P-A2, P-E4 and P-E9 recur only as analogues or partially, and their cells say so. A design unit citing them as generality evidence should read the qualifier.

## Standing obligations

None named this unit. Register dispositions start with the next unit.

## External gates

Re-probed at Phase 4 entry and unchanged:
- `archipeligo-records`: HEAD `c220c62` matches the pin.
- `cartomancer-records`: the ledger is present, and the 41 transcripts are within retention.

## Execution signals

- Context was comfortable at completion.
- Decisions taken mid-implementation rather than at a design pass: P-D5's cost basis and its cited records, revised in each critique round; the reclassing of P-C1, P-C2, P-G3 and P-A9 after critique showed the cartomancer evidence was a different mechanism; and the scope of three judgement probes.

---

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
