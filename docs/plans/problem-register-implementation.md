# Implementation summary: problem-register

- **Branch:** `lsf-workflow`. Base `f3a395a`.
- **Plan:** [problem-register-plan.md](problem-register-plan.md).
- **Files modified:** `docs/design/lsf-problem-register.md` (new), `docs/plans/problem-register-plan.md` (new), this file.

## External gates (Phase 4 entry)

- `archipeligo-records`: `~/src/archipeligo-evolved` HEAD is `c220c627…`, which matches the pin.
- `cartomancer-records`: the signals ledger is present, along with 41 session transcripts (2026-09-10 to 09-23, within retention). No drift.

## Per-step record

1. **Plan saved** at `0528253`.
2. **Mechanical probes.** Scratch scripts tabulated:
   - the cartomancer ledger's kinds and dispositions;
   - outcomes of 18 run JSONs;
   - signals from 466 workflow-agent transcripts: background gates, API errors, denials, relays and Skill calls.

   Surprises:
   - The API errors were session limits, not 529s.
   - Cartomancer's permission denials came from the auto-mode classifier on vault renames, not from cleanup.
   - Every backgrounded gate was polled to completion.
3. **Judgement probes.** Three read-only sonnet subagents covered §A/§G, §E/§B and the mechanical stages. Key positives were spot-checked against the sources before classing: D5, D6, D9, D16, D17, `afb311c`, `df7cd8c`, `cdf14ff`, a fabricated SHA, and four session quotes.

   Surprises:
   - Cartomancer's haiku decision-log agents dropped the block's closing fence three times. One of those drops stopped a load.
   - A ledger append dropped its heading and misfiled 64 rows.
   - Once, the script's own prompt carried a wave heading that did not match its rows.
   - `lsf-drive` was never used on cartomancer.
4. **Register written** at `b347ee0`: 47 rows, Appendix A over 87 postmortem items, and Appendix B with a probe for every row.

   Deviations:
   - The P-C1 to P-C9 ids are not in order of the postmortem table.
   - A Topic column was added, as decided at confirmation.
5. **Structural check.** `check-register.py` (scratchpad) passes at every commit.
   - Its lexicon flags were `rule` in P-A1 and P-B1 and `retry` in P-E1. Each describes an event, not a remedy, so each was resolved.
   - A matching bug in the check (`regenerat`) was fixed.
   - The P-D5 terms were updated as the row's cost changed.
6. **Critique gate.** Three rounds ran; see the next section. The fixes were committed at `fc2e010` and `497cd5b`.

## Critique findings

Round 1 (`b347ee0`) found no Critical.

| Finding | Severity | Disposition |
|---|---|---|
| S1 P-G3 classed recurs on shakedown evidence | Significant | addressed: archipeligo only so far (not testable) |
| S2 P-C2 bundled denied-commit misfiling; carto evidence a different mechanism | Significant | addressed: narrowed to 3 faults, misfiling moved to P-C8, class archipeligo only so far |
| S3 P-C1 recurs on altered-not-truncated loads | Significant | addressed: archipeligo only so far; the harness check is stated |
| S4 P-E4 cell disagreed with its probe | Significant | addressed: aligned and qualified "different frame" |
| S5 P-D5 cost was the deletion figure | Significant | addressed; refined in round 2 |
| S6 P-A7/A8/A13, P-C8 could be context-free | Significant | P-C8 addressed (`implement.js:260-266`, `:279`). Declined for P-A7, P-A8 and P-A13: they need a campaign workload, which the context-free class excludes |
| S7 P-E9 partial evidence | Significant | addressed: qualifier |
| M1–M6, M8 | Minor | addressed |
| M7 P-E2 bundles option content and the tier correlation | Minor | left: the postmortem presents them as one decision failure, and splitting adds no disposition the consuming unit (`drive-decision-design`) needs |

Round 2 (`fc2e010`) converged with no Critical.

| Finding | Severity | Disposition |
|---|---|---|
| P-D5 measured the checkout, not the retained series | Significant | addressed. Measured `~/src/archipeligo-runs/` at 80 GB, with the worktree at 372 MB. The statement now follows the close-out's "Kept outside `main`" |
| P-A9 cartomancer evidence inverts the mechanism | Significant | addressed: archipeligo only so far, with the analogue in Appendix B |
| P-E4 D11 citation; P-C8 script-heading analogue; Appendix A rows 21, 27, 28; P-A10 D5; code-line SHA | Minor | addressed |

Round 3 and the editor pass: see Phase 5.

## Success criteria

| # | Criterion | Status | Target | Achieved | Evidence |
|---|---|---|---|---|---|
| S1 | Done condition, verbatim | pass | evidenced | evidenced | `check-register.py` passes; the critique gate passed (round 1 and round 2 had no Critical) |
| S2 | Every class backed by a recorded probe | pass | evidenced | evidenced | Appendix B has a row for each of the 47 ids (checked by script) |
| S3 | Ids unique, `P-[A-GI]\d+` | pass | evidenced | evidenced | script assertion |

Upgrade path to `proven`: commit `check-register.py` under `tools/` with a test. That was not done because the register is write-once and `programme-close` edits only its Disposition column.

## Remaining phases

Complete Phase 5 (Review) as defined in the phronithm:feature skill.
