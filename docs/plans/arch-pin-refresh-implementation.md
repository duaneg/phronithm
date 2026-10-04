# arch-pin-refresh — implementation summary

- **Branch:** `lsf-workflow`. Base `0cacac9`. Plan: `docs/plans/arch-pin-refresh-plan.md` (`e31d258`).
- **External gate re-probe:** `node` was pinned at v26.9.0 and observed at v26.9.0. No drift.
- **Files modified:**
  - `workflows/lsf/implement.js`
  - `workflows/lsf/scheduling.js`
  - `workflows/lsf/schema.js`
  - `workflows/lsf/test/implement.test.js`
  - `workflows/lsf/test/scheduling.test.js`
  - `workflows/lsf-implement.js` (generated)
  - `docs/design/lsf-workflow.md` (Wave loop)

  `scheduling.js`, `schema.js` and `test/scheduling.test.js` lie outside the unit's Scope line. This was planned.

## Steps

### Step 1 — gate amendments become records (`a77645b`)
`signalsFromGate` maps `amendments` entries to recorded `architecture-amendment` signals that carry the unit. The kind comment names both producers. There were no deviations.

Critique (code, round 1) found no Critical issues. Minor findings:
- Malformed entries garble the record. Fixed in `667913b`.
- The digest mixes prefixed and bare statements. Accepted: the prefix is the only way the unit reaches the digest's list.
- Amendments-only coverage. Covered by the implement tests.
- No schema for amendment entries. Covered by `GATE_SCHEMA` in step 2.

### Step 2 — the pin follows the branch (`667913b`, carrying `Closes #2`; follow-up `14f8de7`)
Implemented as designed:
- The merge reports `architectureSha`.
- The pin moves before the gate.
- `amendedClause` gives the gate one diff per merged unit against the pre-wave pin.
- `GATE_SCHEMA.amendments` added.
- `refreshPin` runs on the re-plan and termination reloads.
- Design-of-record paragraph added in the same commit.

Decision taken: the merged entries carry `mergeSha`, which comes from the merge return's per-unit `sha`.

Critique (code, round 1) found no Critical or Significant issues. Minor findings, fixed in `14f8de7` unless noted:
1. The merge prompt had no SHA instruction when nothing merged. Fixed.
2. With no load-time pin the clause was suppressed. Fixed: the baseline falls back to each merge's first parent.
3. Test (b) alone is weaker than it reads. It stands, because tests (a) and (b) together cover the script half.
4. The null-merge and nothing-merged paths were untested. Tests added.
5. The design paragraph omitted three behaviours. Fixed.
6. "as above" pointed at an absent clause. Fixed.
7. Double unit prefix. Fixed.
8. The report-once rule can under-report a change that is both a moved decision and a new premise falsity. Accepted: this is the trade-off the design chose.

## Success criteria

| Criterion | Result | Target | Achieved | Evidence |
|---|---|---|---|---|
| S1: later stages read the amended text | pass | proven | proven | tests "a unit that amends the architecture document…" and "a re-plan reload and the termination reload…" |
| S2: script half — unit diffs reach the gate against the pre-wave text, and an invalidation stops the run | pass | proven | proven | tests (a) and (b), plus "the pin stays on a null merge…" |
| S2: gate half — the live gate follows the instruction | — | evidenced | conjectured | The gate is an LLM stage. Upgrade path: the `shakedown-fixture` unit's fixture with a unit that amends the architecture document |
| S3: amendments recorded with their unit, shown in the digest, stop nothing | pass | proven | proven | scheduling test and the implement amendment tests |
| S4: reload refresh; an unmoved document leaves the prompt unchanged | pass | proven | proven | tests (d) and (e) |
| S5: suite green and build not stale | pass | proven | proven | `node --test test/` passes 127/127, including `build.test.js` |
| S6: phronithm#2 closed by the commit | pending | evidenced | evidenced | `Closes #2` is in `667913b`. The issue closes when `lsf-workflow` merges to main, by requester decision on 2026-10-05 |

## Remaining phases
Complete Phase 5 (Review) as defined in the phronithm:feature skill.
