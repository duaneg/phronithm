# arch-pin-refresh: completion report

The unit `arch-pin-refresh` (bug-fix) of `docs/plans/lsf-improvements-breakdown.md` was carried out on branch `lsf-workflow`, starting from base `0cacac9`. Commits, in order:

| Commit | Content |
|---|---|
| `e31d258` | plan |
| `a77645b` | gate amendments recorded |
| `667913b` | pin follows the branch; `Closes #2` |
| `14f8de7` | step-critique Minors |
| `77d7cce` | Phase 5 review fixes |
| `625eac5` | implementation summary |

## Done-condition verdict

**Met.** Each clause is shown below with the rigour it achieved.

- **Later stages are judged against the amended text.** Rigour: `proven`. The test "a unit that amends the architecture document…" in `workflows/lsf/test/implement.test.js` checks this. After the wave-1 merge reports `arch2`:
  - the wave-1 gate header names `arch2`;
  - wave 2's plan, critique, implement and gate prompts all name `arch2`, with no stale pin.

  The test "a re-plan reload and the termination reload refresh the pin…" covers the other refresh paths. It shows a `document-text` correction reaching wave 2 (`arch3`) and the termination reload reaching the obligations stage (`arch4`).
- **A unit-carried change to a decision still reaches the gate as a candidate invalidation.**
  - **Script half, `proven`.** The gate receives each merged unit's own diff, `git diff <merge>^1 <merge> -- <doc>`. It is told that a decision-moving change is an invalidation with all five fields filled. An invalidation raised this way stops the run with `architecture-invalidated`. Tests (a) and (b) cover this.
  - **Gate half, `conjectured`.** Whether the live gate follows the instruction cannot be shown by a stub. The upgrade path is `shakedown-fixture`, whose fixture includes a unit that amends the architecture document.
- **Tests pass and the built script is not stale.** Rigour: `proven`. `cd workflows/lsf && node --test test/` passes 127/127, and that includes `build.test.js`.
- **phronithm#2 is closed by the commit.** Rigour: `evidenced`. `667913b` carries `Closes #2`. By the requester's decision (2026-10-05) there is no `gh` action, so the issue closes when `lsf-workflow` merges to main. Until then it stays open.

## Deviations

- **Writes outside the Scope line:**
  - `workflows/lsf/scheduling.js` (`signalsFromGate`);
  - `workflows/lsf/schema.js` (comment on the `architecture-amendment` kind);
  - `workflows/lsf/test/scheduling.test.js`.

  These were planned. Every later writer of these files depends on this unit through the serial script chain.
- **Publishes: none.** That is unchanged, but some contracts did change:
  - The merge stage's return gains a required `architectureSha`.
  - The gate's return gains `amendments`.
  - `architecture-amendment` now has two producers: the gate, for a unit's own change, and the re-plan, for `document-text`.
- **Baseline differs from the approved design.** The design judged each unit's change against the pre-wave text. The Phase 5 review and critique both showed that a later unit's diff starts from the text after the wave's earlier merges. Each change is therefore judged against the text it was merged onto. The pre-wave pin remains the baseline only when a merge SHA is missing.
- **Issue closure is deferred to the merge to main**, as recorded above.

## Deferrals

- Validating the SHAs the merge stage reports (`architectureSha`, `merges[].sha`) belongs to `mechanical-stage-integrity` (P-C8). An invented `architectureSha` becomes the pin unchecked. A missing per-unit `sha` falls back to whole-wave attribution, and nothing signals that fallback.

## Discoveries

- **`mechanical-stage-integrity`.** `merges[].sha` is now load-bearing: it attributes each unit's architecture change. The merge's `architectureSha` sets the pin. Both should be among the returned SHAs it validates. Making `merges[].sha` required for clean and resolved merges is a candidate.
- **`shakedown-fixture`.** The fixture's architecture-amending unit is now the upgrade path for clause 2's gate half. Its run should cover two cases: a design unit settling an open decision, which should come back as an amendment and not stop; and a decision-moving change, which should stop.
- **`empirical-script-support`, `long-running-impl`.** Both write `lsf-workflow.md::wave-loop`. The section now has the paragraph "The architecture pin follows the integration branch". There is no conflict, but the text exists.
- **Residual risk.** The run-#8 false stop can recur if the gate misclassifies a design unit's settled decision as a decision move. The per-unit diff, unit type and completion report reduce the chance; they don't remove it.

## Standing obligations

- **Register dispositions:** P-D1 → `fixed` at `667913b`, with follow-ups `14f8de7` and `77d7cce`.
- **Design of record:** discharged. The `lsf-workflow.md` Wave loop was amended in `667913b`, the behaviour commit, and later corrected alongside each fix.

## External-gate re-verification

`node` was pinned at v26.9.0 and observed at v26.9.0. No drift.

## Execution signals

- Context at completion was comfortable.
- Two design decisions were taken after the design pass:
  - attaching each unit's merge SHA to the gate's merged list, which was settled in the plan's critique round;
  - the per-unit baseline change, taken in Phase 5 from review findings.
- Every critique and review pass returned no Critical findings. That was 5 passes: plan, step 1, step 2, lens review and full-diff critique.

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
