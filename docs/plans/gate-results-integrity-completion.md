# gate-results-integrity: completion report

The unit `gate-results-integrity` (bug-fix) of `docs/plans/lsf-improvements-breakdown.md` (entry at `c901531`) was carried out on branch `lsf-workflow` from base `c901531`.

| Commit | Content |
|---|---|
| `73055ec` | plan |
| `5035e19` | `gate-not-run` stop at implement (P-D3) |
| `a7ba546` | fix return carries gates; review reads them (P-D2) |
| `6ac23eb` | long-gate prompt recipe (P-D3) |
| `b82a6c7` | step-2 critique fixes |
| `82cb6b9` | step-3 critique fixes; the recipe is executed by a test |
| `f80f78b` | implementation summary |
| `e0ab578` | Phase 5 review fixes |

## Done-condition verdict

**Met.**

- **(a) A review round after a fix reads the fix's gate results.** Rigour: `proven`. Two tests in `workflows/lsf/test/implement.test.js` cover it:
  - "a review round after a code fix reads the fix stage's gate runs…": the round-2 `critique-code` and `lens-review` prompts carry the fix's runs, labelled "The round-1 fix stage", and not the implement stage's.
  - "a second fix round…": round 3 reads the round-2 fix's runs.
- **(b) An implement or fix return whose gate did not run does not reach review.** Rigour: `proven`. Each of these returns stops with `gate-not-run` before any further review:
  - an implement return with a `not-run` gate;
  - an implement return with no gates;
  - a code-fix return with a `not-run` gate, with no gates, or missing a gate the implement stage ran.

  Tests also show that a design unit is exempt and that a red gate still reaches review.
  - Limit: the check sees only what the stage reports. A stage that claims `green` for a gate it never ran is not caught (see Discoveries, `mechanical-stage-integrity`).
- **The tests pass and the built script is not stale.** Rigour: `proven`. `cd workflows/lsf && node --test test/` passes 136/136, including `build.test.js`. A rebuild leaves the tree clean.

External gate `node`: pinned v26.9.0, observed v26.9.0.

## Deviations

- **Written outside the Scope line:** `skills/lsf-drive/SKILL.md`, *Relaunch* section. It now names `gate-not-run`, with one exit: the requester changes the environment or the project so the gate completes. This is the section `relay-mitigation` writes.
- **Publishes: "the fix stage's return shape".** A non-design code fix now returns `gates`, the same shape as the implement stage's, and is required to. A design unit's fix still returns no gates. Other contract changes:
  - new STOP kind `gate-not-run` in `schema.js`;
  - `critiqueLoop`'s `onFix(fixed, n)` may return STOP signals (reason `fix-stop`);
  - the code-fix prompt lists the implement stage's gate commands to re-run;
  - `LONG_GATES` is exported from `implement.js`, for its test.
- **Reversed a known limit from the plan.** The plan accepted a fix that returns only some of the gates. Once the prompt gave the fix the exact commands, string matching no longer risked false stops. Both Phase 5 reviewers also showed that a dropped gate would carry into later rounds as the new baseline. A fix that omits a command the implement stage ran is now `gate-not-run`.
- **The scope adjustment agreed at confirmation** was the requester's two decisions on 2026-10-05: a structural stop for a gate not run, and an empty list treated as not run. Both are within the unit's open decision.

## Deferrals

None.

## Discoveries

- **`shakedown-fixture`.** The long-gate recipe is proven against bash semantics: a test runs it, and it was probed once in the main session. Live workflow agents are untested: whether a detached process survives the end of the agent's Bash call, and whether writes under `.git` are permitted. The fixture should include a gate that takes longer than 600 s. If the harness kills the process group, every long gate becomes a `gate-not-run` stop. That fails safe, but leaves no path through.
- **`long-running-design`.** A `gate-not-run` stop has one exit: an environment or project change. A project whose gate cannot finish within one stage, even with the recipe, has no route through the workflow. That is work outlasting a stage, the subject of this design unit.
- **`mechanical-stage-integrity`.** A stage that reports `green` for a gate it did not run, or invents a summary, passes the check. This sits with that unit's fabricated-return faults.
- **`relay-mitigation`, `drive-mechanics`, `drive-decision-design`.** The lsf-drive *Relaunch* section now lists `gate-not-run`, with a paragraph saying it is not answered by a decision. Any later rewrite of that section must keep the route.
- **`empirical-unit-design`.** An empty gates list counts as not run, so a non-design unit in a project with no gate of its own stops on every launch. The implement prompt does not tell such a stage what to report. Docs and measurement units in a project without a test suite would hit this.
- **`docs/design/lsf-improvements.md:80`** still proposes retrying the stage on a not-run gate. That note holds candidate options, not the design of record, so it was left unchanged; the re-plan may want to mark it superseded.

## Standing obligations

- **Register dispositions:**
  - P-D2 → `fixed` at `a7ba546`, with follow-ups `b82a6c7` and `e0ab578`.
  - P-D3 → `fixed` at `5035e19` (stop) and `6ac23eb` (recipe), with follow-ups `82cb6b9` and `e0ab578`. The detection is proven. Whether live agents can wait out a long gate is `conjectured`; the upgrade path is `shakedown-fixture`.
- **Design of record:** discharged.
  - `lsf-workflow.md` *Lifecycle mapping* stages 3–5 were amended in each behaviour commit (`5035e19`, `a7ba546`, `6ac23eb`), then corrected in `82cb6b9` and `e0ab578`.
  - The status paragraph names the new rules as not yet run live.
- **Build:** discharged. `node workflows/lsf/build.js` was run before every test run, and `build.test.js` passes.

## External-gate re-verification

`node` (environment): pinned v26.9.0, observed v26.9.0 at Phase 4 entry. No drift.

## Execution signals

- **Context:** comfortable at completion.
- **Design decisions taken mid-implementation, not at a design pass (four):**
  1. The code fix is given the implement stage's gate commands (step-2 critique).
  2. The recipe records a PID and ends the poll when the gate dies, and groups compound commands (step-3 critique).
  3. A fix that omits a gate stops, reversing the plan's known limit (Phase 5).
  4. The lsf-drive "complete by hand" exit was dropped (Phase 5): it had no mechanism and the skill forbids hand completion.
- **Plan wording defect:** the planned poll command would never have finished because `$D` sat inside single quotes. It was caught by a probe before commit.
- **Review:**
  - Plan critique: no Critical; 4 Significant, all addressed.
  - Step critiques: no Critical; 5 Significant, all addressed.
  - Phase 5 lens review and full-diff critique: no Critical; 3 Significant, all addressed. The Minor findings left open are the comma-joined `referent` and the design unit's implement prompt carrying the recipe text.

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
