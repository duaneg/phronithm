# LSF empirical work

How lsf plans, runs and records work whose outcome is a measurement. Sections are owned by units of the [LSF improvements programme](../plans/lsf-improvements-breakdown.md): Criteria (`empirical-criteria-design`), Units (`empirical-unit-design`), Retained artefacts (`retained-artefacts-design`).

## Criteria

Unit `empirical-criteria-design`, plan @ `14f2a07`.

**Status:** decided, 2026-10-10. Requester verdict: **C1–C5 accepted as proposed**, with C4 and C5 adopted generally. P-A1's geometric part is declined. After the [C4 probe](../investigations/c4-probe-results.md), the requester chose to structure C4 along fixed axes; that change is untested.

### Problem

From the [register](lsf-problem-register.md):
- **P-A1.** Three archipeligo gate designs had defects that arithmetic shows without a run, and each was found only after a stop.
- **P-A2.** The architecture cited, as evidence, a figure measured on another engine with an instrument the brief had called invalid.
- **P-A3.** Criterion 3's "persists" had no horizon until 2026-10-01.
- **P-A11.** No version of criterion 3 measured dynamics, and the certified world was frozen in 22 of 40 runs.

The large-scale-feature skill asks only for "measurable success criteria (confirmed, or provisional with explicit flags)" (Phase 1 step 6). Nothing in Phases 1–2 or in `critique-design` asks for a criterion's sample, decision rule or pass probability, where cited evidence was measured, or what a criterion stands for.

### Evidence

Archipeligo is cited at `c220c62`, with blob paths at the commits named; cartomancer-v2 at `26184a6`.

**Archipeligo criterion 3.**
- **Versions.** The brief changed criterion 3 eight times after its first draft (`7f4fb94`, 2026-08-22). It has no change log; amendments are recorded only inline and in commit messages.
- **Relaxations.** Three:
  - the failure allowance went from j = 0 to 7 (`f90da9f`, 09-27);
  - the per-seed stage-2 rule was replaced (`a01fe05`, 09-28);
  - "persists" was fixed at the 20 000-tick horizon (`b2d4748`, 10-01).
- **Missing elements.** Before `83808a2` (09-24) no version had a statistic, a confidence level or a horizon. `83808a2` stated a statistic, a sample, a rule and a confidence level. `f90da9f` was the first to add pass probabilities at stated levels, and its "gate horizon" was still undefined.

**The three gates.**

| Gate | Design | Defect, by arithmetic | Where the arithmetic first appeared |
|---|---|---|---|
| R3, `05e3827` (09-24) | committed seed plus two held out; no confidence level, no π* | certifies only π ≥ 0.368 at 95% (0.224 on the held-out pair) | research note `ef0c0d3`, 9.5 h after the gate was committed; the sonnet critic and the opus adversary on `05e3827` did not raise it |
| R4, `83808a2` / `dce7f41` (09-24) | 41 seeds, j = 0, π ≥ 0.928 at 95% (the source's figure; 0.05^(1/41) = 0.9295), "G = 0.8" | a cell at π = 0.928 passes with probability 0.047; G = 0.8 holds only at π ≥ 0.995 | already in `tuning-search-model.md`'s δ column; "no committed record draws that consequence" (`foundation-reconciliation.md`@`0569c92`:29-35) |
| R5, `05e3827` §10 | stage 2 passes all 34 seeds stage 1 passed | P(all pass) ≈ 0.0017 at stage 1's own π̂ = 0.83 (`a01fe05`:36-45) | raised by the Phase 3 `stage2-gate-plan` critique; stopped at `done-condition-unevaluable` |

After R4 the brief adopted the form itself (`b7cee77`:56-65): "n and j are a requester decision taken from a stated required per-seed pass rate π* and a stated P(pass | π) at a plausible π, at α = 0.05". D9 then fixed n = 41 and j = 7, giving π* = 0.70 with P(pass) of 0.87 at π = 0.875 (`f90da9f`).

**Evidence from another system.** R5's "118 of 132 cells persistent" (`a169bae`) was measured on the MVP engine with the label-survival classifier. The brief had already said, the same day, that this classifier "measures survival of genesis kind labels under the level ontology this increment deletes" (`def2ed9`:39-45). The brief then cited the figure as satisfying its gate (`fb8d781`), and so did architecture §2, §3, §6.1 and §13 (`653c652`).
- **Criterion 2 satisfied by an invalid figure.** Criterion 2, "every architecture choice that depends on a measurable claim cites an R4/R5 probe result", was met by citing this figure.
- **Re-based result.** `classifier-rebase` found the regime eating nothing but ambient stock. The architecture caveated the figure (`18bc270`), and archipeligo's breakdown added a rule: "date a regime verdict against the classifier that produced it as well as against the engine" (`foundation-breakdown.md:2077`).
- **Brief never caveated.** The brief still cites "118/132 cells" without a caveat (`foundation-brief.md:20, 27`).

**Vision against criterion.**
- **The vision.** `docs/vision.md:9-10`: "the world must be interesting and self-sustaining"; `:27-28`: "watch evolve over long timeframes".
- **Every version passes a frozen world.** Each version of criterion 3 reads label survival or form evenness. A world whose predator count is constant for 80 000 ticks passes all of them.
- **No mapping, no check.** No brief text ties "persists with diversity" to "evolving" or "interesting". No Phase 1 or Phase 2 critique raised the gap: `def2ed9` and `653c652` were each a sonnet critic plus an opus adversary.
- **Where it was found.** The post-close vision review (`foundation-vision-review.md`, 10-02) found it. Its question B2: "The current clauses pass a frozen world."

**Cartomancer-v2 `quality-bar-interiors`.**
- **No measured criterion.** SC1 is the requester's literal "commit". SC2–SC7 are binary or count checks (brief:44-72), and the brief calls them "what make SC1 repeatable" (brief:74).
- **Undefined parameters.**
  - SC4's "within the simplification tolerance" names no value. At the default (1e-6), blob interiors could not pass by construction; they pass at 0.02. This was found when `gate-port` ran it (`quality-bar-interiors.md:987`).
  - SC2's "target ≤ 150 with 120 typical" sits beside the brief's "~120 lines is a typical target, not a ceiling" (brief:37), which leaves open whether 150 binds.
  - SC6's "non-content" is left to the implementer.
- **D17.** §4.11 sized panels from a function calibrated on Liberation Serif alone, for roles that render in DejaVu Serif. The architecture document cites no font metric at all, so a rule about cited evidence would not have caught it.
- **D13 and D14.** These are geometric arithmetic: 0.02 × 1680 = 33.6 px of headroom against about 51 px needed. Both were found at the wave-2 review gate.
- **SC1 passed with known art defects carried forward** (F12, F16, F17; `docs/guides/quality-bar.md:140-147`), by the requester's recorded choice.

**Outside the register's probe.** The register's P-A3 probe read only `quality-bar-interiors`. Two other briefs have measured criteria:
- **cartomancer-v2 `map-toolkit-mvp`.**
  - SC1 asked for a "median correction-to-render under 2 minutes". It measured 3:42 over 13 rounds, and the requester accepted the miss because SC4 and SC6 "are what actually gate acceptance" (`shrine-acceptance-completion.md:18-27`). Neither the brief nor the plan said what a miss meant; the plan called it a target.
  - SC2 asked for "≥ 80%". It came in at 57% raw, The schema freeze rested on "a refined per-round reading" chosen after the data. It was disclosed as "the interpretive call it is" (`style-audit-freeze-completion.md:15-20`), but the brief never fixed the reading beforehand.
- **dungeon-assistant `session-transcription`** (Phase 1 only, `464b1c1`).
  - Most criteria give a statistic, a sample and a threshold, flagged provisional.
  - S4 requires per-term recall "≥ 95% over ≥ 150 mentions", read as a point-estimate floor. Treated as one pooled binomial, it passes with probability 0.52 at a true recall of exactly 95%, and 0.92 at 97% (exact, 143 of 150).
  - Whether 0.52 is a defect depends on the level the requester needs, and the brief does not state it. S4 is not R4's defect; it shows a measured criterion with no stated pass probability.

### Options

**Measured criteria.**

| Option | Requires | Against the evidence |
|---|---|---|
| Nothing | as now | R3–R5 and P-A3 recur. dungeon-assistant S4 already shows the gap |
| Note §3 item 1 | statistic, sample, horizon, decision rule | catches R3 and the horizon. R4 stated its statistic, sample and rule, and its defect is in none of them |
| Item 1 plus probabilities and a fixed reading | as item 1, plus a model; pass probability at a needed level; false-pass probability at an unacceptable level; a cited plausible level; the reading fixed before the data | R3, R4, R5 and S4 would be flagged on recompute, and map-toolkit SC2's reading becomes an amendment. `b7cee77` is close to this form, adopted by archipeligo after two stops |
| Route to the critic alone | a `critique-design` item, no skill text | the critic would have to discover what nobody wrote down; both Phase 1 and Phase 2 gates, critic plus opus adversary, missed all three |
| Recompute tool | a shipped helper (e.g. a binomial pass-probability script in `tools/`) | removes unaided LLM arithmetic. Not proposed as a shipped tool: archipeligo's figures came from its own research notes, and the missing step was drawing the consequence into the brief. C2 instead asks the author to commit a reproducible computation (Checking) |

**Evidence.**

| Option | Requires | Against the evidence |
|---|---|---|
| Nothing | as now | P-A2 recurs |
| Note §3 item 1 | cited evidence names the system and instrument; evidence from another system is a premise for Phase 3 | catches 118/132 at Phase 1. Leaves the brief uncaveated after an invalidation |
| Item 1 plus re-dating | as item 1, plus: replacing an instrument or system re-dates every citation of its results in the brief and the architecture | also closes the stale brief. Archipeligo's own breakdown arrived at the dating rule |
| Calibration register | a register beside the plan holding measured values | the note's larger form of §3 item 4 (superseded evidence), which belongs to `empirical-unit-design` (P-A10). Not needed for provenance |

**Criterion against vision.**

| Option | Requires | Against the evidence |
|---|---|---|
| Nothing | as now | P-A11 recurs; found only by a post-close review |
| Note §9 item 3, every criterion | "describe a system that passes the criterion and fails the vision" for each criterion | on `quality-bar-interiors` it bites on SC2–SC7, which are deliberate proxies, so six answers restate what brief:74 already says |
| Question for vision-standing criteria | each criterion names what it stands for: a vision passage, or the criterion it serves. The question is asked of the first kind only | one question on `quality-bar-interiors` (SC1), one on archipeligo (criterion 3) |
| Post-relaxation review only | change log; after a second relaxation, recommend a vision review | archipeligo's second relaxation (09-28) precedes the review by four days, but the gap was there from 08-22 |

**The C4 probe** ([results](../investigations/c4-probe-results.md)): eight opus calls answered the unstructured question on archipeligo's v0 criteria.
- **Frozen world:** 0 of 4 fresh calls named it. Of the 4 author calls, 1 named it on a strict reading and 2 on a lenient one.
- **Missing horizon:** 3 of the 4 author calls named it, which is P-A3's defect.
- **Other gaps:** of the eight gap classes the calls raised, five are real in archipeligo's record, one was addressed, two are unknown, and none is contradicted. All eight calls found the vision review's finding 3, "The economy is mechanism, not yet an economy".
- **Reading:** the unstructured question reliably finds real criterion–vision gaps, but not reliably the one it was aimed at; which gap it finds depends on which failure the author happens to imagine.
- **Planted cases:** a test of the axes on two invented cases hit its ceiling (12/12), so the axes remain untested (plan risk R42).

### Proposed decision

**Terms.**
- A criterion is *measured* when its verdict is read from a sample of runs, observations or judgements, so that another sample could give another verdict. Medians over rounds, rates and seed pass counts are measured. A grep, a test suite and a single deterministic build are not.
- A criterion *stands for* a passage of the vision, or *serves* another criterion as a proxy for it.
- *The vision* is the project's vision document where one exists, and otherwise the brief's request record. Of the three projects cited here, only archipeligo has a vision document. Against a request line the C4 check is weaker, since a request line restates less than a vision does.

**C1, defined terms (every criterion).** Every threshold, tolerance, horizon, classifier and instrument a criterion relies on is either given its value or cited. The brief may carry an undefined term as provisional. Phase 2 exits only when none remains, unless the requester defers one by recorded decision; the decision names the unit that resolves it, and that unit precedes every unit that reads the criterion.

**C2, measured criteria.** A measured criterion states:
- **Statistic:** what is computed.
- **Sample:** the population, the size, how it is drawn, and what is held out.
- **Horizon:** over what length or period.
- **Decision rule:** what passes.
- **Model:** the distribution the probabilities below are computed under, including any independence or conditioning assumption (R5's 0.0017 assumes independent stage-2 draws at stage 1's rate).
- **Two levels the requester states:**
  - the *needed* level: the effect the requester needs the criterion to pass reliably at. It is not the certified bar, where a correctly sized gate passes with probability about α by construction ([note](lsf-improvements.md) §3 item 2);
  - the *unacceptable* level: one at which a pass would be wrong.
- **Two probabilities, computed before any run, with the arithmetic shown or cited:**
  - the pass probability at the needed level;
  - the false-pass probability at the unacceptable level.

  Any assurance the criterion states, such as "G = 0.8", is recomputed from these.
- **The plausible level:** where the system is expected to be, cited from an earlier measurement or a probe, or stated as an assumption. The pass probability there is also given.
- **Binding:** whether a miss stops for a requester decision or is only reported.

**If the probabilities are low.** C2 sets no minimum. The requester accepts each probability as stated or changes the design, and the brief records the decision.

**No model.** Where no model gives the probabilities (a median of timed rounds, an aggregate share), the criterion says so. A feasibility probe that measures the spread is then an open unknown, resolved before the unit that reads the criterion is planned. Naming the probe unit discharges C2 at Phase 2 exit.

**Fixed reading.** The statistic and rule are fixed before any data are read. A different reading after the data is an amendment (C5), not an interpretation.

**Provisional criteria.** A criterion the brief flags provisional owes its C2 statement by Phase 2 exit, or the same deferral C1 allows: a recorded requester decision naming the resolving unit.

**C3, evidence.** Evidence cited for an architecture or brief claim names the system (commit or version) and the instrument (name and version) it was measured on.
- **Another system or instrument.** Evidence measured on a different system or instrument from the one the claim concerns is a premise. It is listed among the open unknowns, together with the unit that tests it.
  - **Criteria.** A criterion that cites the premise is satisfied by the testing unit's result, not by the premise.
  - **Phase 2 exit.** Phase 2 may exit with the premise listed, so a rewrite or migration, whose prior evidence all comes from the old system, is not blocked. The architecture records each premise-dependent criterion as open at exit, naming the testing unit; the criterion is judged on that unit's result.
- **Disclaimed instruments.** Evidence from an instrument that the brief or architecture itself calls invalid for the claim is not cited in support of a criterion at all.
- **Re-dating.** When an instrument or system is replaced, every citation of its results in the brief and the architecture is re-dated in the same amendment.

**C4, against the vision.** Each criterion names the vision passage it stands for, or the criterion it serves. For each criterion that stands for a passage, the brief answers: *describe a system that passes this criterion and fails that passage.* The answer either adds a clause to the criterion or is recorded as a gap the requester accepts. The question is asked again of every amended criterion (re-entry step a).
- **Axes.** The question is answered once along each axis, with a failing system or "none found" for each:
  - **time:** beyond the horizon, or stationary within it;
  - **starting conditions:** other than those tuned or tested;
  - **instrument:** passing by satisfying the measure, not the passage.

  The axes come from the probe's own hits: horizon in A1–A3, stationarity in A4. Whether they raise the hit rate on another case is untested.
- **Chains.** Every "serves" chain must end in a criterion that stands for a passage. Each vision passage the brief claims to address must have at least one criterion that stands for it.
- **Not answered from the record.** An answer that only restates a gap already on record does not discharge the question.

**C5, change log.**
- **Relaxation.** An amendment is a relaxation when it passes some system that the previous version failed, or that the previous version could not judge.
  - Fixing "persists" at 20 000 ticks is one: it accepts the six runs that collapse between 38k and 92k.
  - The author classifies each amendment. The critic checks the classification by comparing the old and new pass conditions: is there a system the new text passes and the old failed?
- **The log.** The brief keeps a change log of criterion amendments. Each entry gives the date, the old and new text, the decision that made the change, and whether the change is a relaxation.
  - Git history does not give this. Archipeligo's criterion 3 changed in 8 of the brief's 10 commits, and no commit message classifies a change as a relaxation.
  - The register's "six redefinitions, three relaxations" comes from the postmortem's own count, which spans the brief and architecture §10. The 8 here is the brief's history alone.
- **Vision-review prompt.** After a criterion's second relaxation, the session recommends a vision review before the next launch. This is a prompt, not a stop.

**Checking.** Phase 1 step 7 and Phase 2 step 3 tell the critic to:
- check C1–C5 for presence;
- check each C2 computation's inputs and structure against the criterion, and spot-check its output;
- check that the plausible level is cited;
- check the C4 chains and the C5 classifications.

The author commits each C2 computation as a reproducible command with its output, for example a one-line `python3 -c` binomial sum. `phronithm:critic` has no shell, and unaided arithmetic on a j = 7 of 41 tail is unreliable.

Presence is checkable where discovery failed. No Phase 1 or Phase 2 critique on archipeligo raised the arithmetic, but each figure was a short computation once the inputs were written down. How reliably a critic checks a committed computation is unmeasured.

**Not covered.**
- **Geometric and sizing arithmetic on design figures** (cartomancer D13, D14). These are claim checks, not criteria. P-A1's cartomancer analogue therefore stays open under this decision.
- **Reliance on a measured property that is never cited** (D17). C3 binds only what is cited.

### Walk

| Case | Under C1–C5 |
|---|---|
| 3-seed gate (`05e3827`) | C2 asks for both levels. At a needed 0.9 the gate passes with probability 0.73. At an unacceptable 0.7 (where D9 later set π*) it falsely passes with probability 0.34, and at 0.5 with 0.125. Equivalently, it certifies only π ≥ 0.368 at 95%. All of this is visible before the gate unit is planned |
| 41-seed gate (`83808a2`) | The stated G = 0.8 is recomputed: it holds only at π ≥ 0.995. At a needed level of 0.95 the gate passes with probability 0.12, and at 0.9 with 0.013. The figure was already in `tuning-search-model.md`; C2 puts it in the brief |
| Per-seed stage-2 rule | C2's plausible level must be cited, and stage 1's π̂ = 34/41 is the citation. Under independence P(pass) ≈ 0.002 there; under the correlated model `a01fe05` also gives, 0.03–0.50. The model must be stated, which makes that assumption visible. Raised at Phase 2 rather than by the Phase 3 plan critique |
| "Persists" | C1: an undefined horizon blocks Phase 2 exit (`653c652`, 2026-08-23), 39 days before `b2d4748`, unless the requester defers it to a named unit |
| 118/132 | C3: MVP engine and label classifier, which the brief called invalid, and the disclaimed-instrument rule bars citing it for criterion 2. As a premise its testing unit is `classifier-rebase`. The re-based figure, 4 of 12 grid cells persistent, came later, from a review-gate note (`foundation-signals/53fcff1a….md:42`). Re-dating at `18bc270` reaches the brief too |
| Frozen world | C4: criterion 3 stands for `vision.md:9-10, 27-28`; it cannot be tagged as serving criterion 1, a binary critique-gate check that stands for no passage. Unstructured, the question named the static world in 1–2 of 4 author calls and 0 of 4 fresh calls. It named the missing horizon in 3 of 4 author calls. The time axis asks for stationarity directly, so on this case it is not an independent test |
| Relaxations | C5: the log has 8 entries, 3 of them relaxations. The prompt fires at `a01fe05` (09-28), four days before the vision review was run |
| `quality-bar-interiors` | C1: three short definitions (SC2's cap, SC4's tolerance, SC6's "non-content"); SC4's would have exposed 1e-6 vs 0.02 before `gate-port`. C2: none. C3: a clause each for the architecture's measured figures (texture timings :502-508, the 70 px shadow comparison :708, the headroom arithmetic :806-809, N1 and N2 :987-988). C4: SC1 stands for "maps of the tier-2 exemplars' quality" and SC2–SC7 serve SC1 (brief:74), so one question asked at Phase 1, before F12 and F16 existed. C5: an entry per amendment |
| `map-toolkit-mvp` SC1, SC2 | C2's binding states in advance whether SC1's median is a gate or a target, so the accepted 3:42 is not a disposition made after the fact. No model gives a median's pass probability over 13 rounds, so C2 asks for a probe of round-time spread first. SC2's "refined reading" was disclosed as an interpretive call (`style-audit-freeze-completion.md:15-20`). Under the fixed-reading rule it becomes a recorded amendment |

**Burden on a programme with no measured criterion:**
- C1: a line for each undefined parameter.
- C3: a clause for each cited measured figure.
- C4: a tag on each criterion, plus one question for each that stands for the vision.
- C5: an entry for each amendment.
- C2: nothing.

On `quality-bar-interiors` that is roughly 20–30 lines, plus one requester decision for each C1 deferral or C4 gap the requester accepts.

Where a criterion is measured, C2 costs a model, two requester-stated levels and two computed probabilities for each criterion. Where no model fits, it also costs a feasibility probe before the gate unit.

### Generality

- **P-A1** (cartomancer: analogue, geometric).
  - Measured criteria with no stated pass probability occur outside archipeligo: dungeon-assistant S4, which is Phase 1 only and has never run.
  - No second project has stopped on gate arithmetic.
  - The geometric analogue is not covered.
  - C2 is adopted generally. Its burden falls only where a criterion is measured.
- **P-A2** (cartomancer: analogue, another font, not another system). D17's reliance was not cited, so C3 would not have caught it. C3 binds only cited measured evidence, so it is self-scoped as retained-artefacts' citation rule is: a programme citing none bears nothing. Its general adoption rests on archipeligo's evidence.
- **P-A3** (register: not testable on cartomancer). `map-toolkit-mvp` shows a criterion with no binding (SC1) and a reading chosen after the data (SC2), and `quality-bar-interiors` shows an undefined tolerance (SC4). The register's "archipeligo only so far" understates the row. C1 and C2's fixed reading are adopted generally.
- **P-A11** (archipeligo only so far). No second project shows a criterion that passed while failing its vision unnoticed: `quality-bar-interiors`' SC1 gaps were known and accepted. Adopting C4 and C5 generally rests on low cost, not on second-project evidence. Scoping them to measurement-heavy programmes is the alternative, and the requester's to choose.

### Plan consequences

- **`empirical-criteria-impl`** carries C1–C5 and the critic instruction into the large-scale-feature skill: Phase 1 steps 6–7, Phase 2 steps 2–3 and exit, and re-entry step a. Its scope already covers both phases.
- **Register dispositions:**
  - P-A1: `deferred` to `empirical-criteria-impl` for the statistical part. The geometric part (design-figure arithmetic, cartomancer D13 and D14) is `declined` by requester verdict, with no carrier.
  - P-A2: `deferred` to `empirical-criteria-impl`. Its uncited-reliance case (D17) is `declined`.
  - P-A3, P-A11: `deferred` to `empirical-criteria-impl`.
- **Re-plan:** `empirical-criteria-impl` (rated low risk, small, provisional) now carries C1–C5 and the critic checks, and should be re-sized.
- **`empirical-unit-design`:** a measurement unit's registration cites C2's form.
- **`programme-close`:** the register's P-A3 cartomancer cell understates the row; the `map-toolkit-mvp` evidence above belongs in it.

## Units

Unit `empirical-unit-design`, plan @ `9b92ee4`.

**Status:** decided, 2026-10-10. Requester verdict: **U1–U9 accepted as proposed**: the `measurement` type, open-until-measured decisions, nothing for P-A10, and the defaults listed under Accepted defaults.

### Problem

From the [register](lsf-problem-register.md):
- **P-A4.** Outcomes that needed runs to find (R1, R2, R6, R8) arrived as stops and Phase 2 re-entries.
- **P-A5.** `stage1-gate` was rated high risk ("nobody has run it") and `specified`, with no seed count or confidence level.
- **P-A6.** A companion tuning-infrastructure unit was declined because a harness existed, and the companion units were added later anyway.
- **P-A9.** Units with uncertain outcomes or several independent deliverables passed the size check.
- **P-A10.** Measured values superseded cited figures with no decision moving, and each cost a requester round trip.

Two further questions from the plan:
- **Gate.** A non-design unit whose stage reports no gate stops with `gate-omitted`, and the Workflow script's equivalent is `gate-not-run` ([gate-results-integrity](../plans/gate-results-integrity-completion.md)). A project with no gate commands therefore cannot run a docs or measurement unit. A declared `lsf-gates` block may not have an empty `gates` list.
- **Store.** Retained artefacts left open where a unit's `Retains:` line and a project's store are declared.

`phronithm:breakdown` has no measurement type. Its tuning companion "may collapse to a verification note" when the project already has tuning infrastructure that covers the need (`skills/breakdown/SKILL.md:91`). Its size budget counts surfaces, open decisions and steps, not outcomes.

### Evidence

Archipeligo is cited at `b706e80`. Re-entries are numbered by their reconciliation commits: R1 `465110f`, R2 `d35698b`, R3 `4a75e94`, R4 `0569c92`, R5 `a01fe05`, R6 `a904738`, R7 `2b5beee`, R8 `c49f13c`. Cartomancer-v2 is cited at `26184a6`.

**How measurement units were typed.** Archipeligo had 19 units whose outcome was a measurement, a campaign, a tuning search or a gate read:
- **Types:** 7 `feature`, 6 `integration`, 3 `polish`, 2 `meta`, 1 `design`.
- **What the entries carried:** 8 entries stated a sample or decision rule. 7 named a fail branch, 4 of them in prose only.
- **No fields of their own:** the plan invented 11 ad hoc field names across 38 occurrences, and none is specific to measurement. Retention appears only inside done-condition prose.

**Fail branches did not prevent the stops.** R1, R2, R6 and R8 each stopped on a review-gate `architecture-invalidated` row with all five fields:

| Re-entry | Unit | Plan's branch for the outcome | Architecture premise falsified |
|---|---|---|---|
| R1 | `stage1-seed-check` | "filed as a backlog issue … reported … as a requester decision" | §10's stage-1 gate on the committed seed; §13 "seed sensitivity is marginal" |
| R2 | `stage1-retune` | (iii) "A failure on a held-out seed is **not** tuned against … files it for a requester decision" (`969830e`) | §13 "the rungs find a cell that passes"; §12 "the grid is a sufficient resolution mechanism" |
| R6 | `stabilisation-screen` | none for the outcome that occurred (baseline passes); only `screen-registration.md:56` covered it | §10 stage-2 cause 3, "No stabiliser" |
| R8 | `bid-era-rung` | "If the thresholds do not hold, it reports that to the requester rather than re-committing them" (D13) | §12 "Demurrage trigger condition"; §6.5's activation clause |

Three of the four outcomes were planned for in the unit, but the architecture stated the premise unconditionally. A planned outcome therefore reached the gate as a false premise, and Phase 2 re-entry followed. Each branch routed to "the requester" and named no successor unit. The units the requester then chose were drafted after the stop: `stage1-retune` (`b880553`), and `derived-min-investment`, `stage1-widened-retune` and `tuning-search-model` (`ec1ec02`).

**Outcome-dependent work outside archipeligo's plan.** No project has a breakdown-format measurement unit, but four keep pre-registered measurements whose outcome selects later work:
- critic-fidelity-bench P2b fixes a three-way rule before any scored run: ≥ 2/14 gives R4, exactly 1 is a requester call, and 0 proceeds to P3 unchanged (`p2b-plan.md:211-246`).
- archipeligo's kin-provisioning and reserve-gate investigations give each claim a registered prediction, a kill line and a partial band ("21–29 partial, read R1 first"; `kin-provisioning-problem-statement.md:62-80`). Both say "no breakdown is needed" (`:159`; `reserve-gate-problem-statement.md:250`).
- coronal-memory's power rule selects the sample composition (`docs/design.md:960-971`).
- The maths campaign's shakedown has an abort rule and set the next campaign's caps (`notes/campaign-depthlaw-prep.md:94-104`).

**`stage1-gate` at `arch@dc88280`.** The entry (`foundation-breakdown.md:353-381`):
- **What it fused:** the stage's one format-version bump, CI extension, the committed defaults and "the first measured verdict on the new regime", typed `integration`.
- **Its pass bar:** "shows a persistent-with-diversity regime at the committed default cell", with no seed count, confidence level or fail branch.
- **Its tuning infrastructure:** "harness exists".
- **Later versions:** seven commits appended to the entry before it was archived, and none added a pass bar. The seed design came from the brief and the reconciliation records (R3, R4).

**The companion unit.** No commit collapsed a drafted companion unit. Retune entries said "The tuning infrastructure is `stage1-gate`'s `xtask cell`/`rung`/`gate` … so no companion design unit is needed" (`b880553`), and `stage1-widened-retune` said the same, with `tuning-search-model` "its eventual replacement, not a prerequisite" (`ec1ec02`). `tuning-search-instrument` was added at `6921509`. The harness searched one parameter at a time and stated no confidence (postmortem §A).

**`stage2-horizon-confirm`** (added at `28becd0`):
- **Entry:** typed `feature`, "Risk: low — descriptive runs on the shipped world; it decides nothing". It registered its binary, commands, seeds and reported quantities first ("`horizon.md` is committed before any run"), and its record was "labelled descriptive", with CP95 intervals.
- **Fail branch:** only "If `xtask cell` cannot build that world … the unit stops".
- **The question it carried:** criterion 3's undefined "persists" (P-A3), to be put to the requester.
- **Data:** its 48 GB of series sat in a git-ignored worktree path, and a tag (`stage2-horizon-registration`) had to be added to keep the registration commit reachable.
- **Its stop:** the runs outlived the implement stage, and that stop is `long-running-design`'s.

**Sizing.**
- **`weighted-contest-rung`:** it carried four Publishes items, the trigger condition among them, and passed the size check twice (wave 1 and wave 2, 2026-09-29). The requester split it into four units (D13).
- **`bid-gate-open`:** one of the four successors, it passed at D13 ("six steps … budget holds"), and R7 split it again. Its threshold commit, a measurement, sat beside the gate flip that consumed it, and `bid-era-rung` later tested those thresholds (R8).
- **Cartomancer's analogue:** `tactical-interior-tier` was flagged at three surfaces, kept by the requester (D7, D10), and split at rp-7 (`afb311c`). It is a surface-count case, which the existing budget already caught; no outcome selected later units.

**Superseded figures.** The register reads R1 and R2 as §13 evidence paragraphs. In fact each of R1, R2, R6 and R8 also moved a decision: §10's gate, a §12 row, or §10's cause 3. Only R2's first row was an evidence paragraph alone. Of cartomancer's cases:
- **D5, D6 and D9** (2026-09-12) were re-plan `cannot` verdicts raised before the `document-text` verdict existed. It landed at `phr@087ee9b` (2026-09-13), and cartomancer's ledger then holds 20 `document-text` verdicts absorbing the same kind of item (e.g. `quality-bar-interiors-signals.md:1211, 1344`).
- **D16** (2026-09-15) is the remaining case. It was a review-gate `architecture-invalidated` stop for §4.9 wording that misdescribed which family binds, where the shipped guard was already right. The requester's answer was "Wording only, no runtime change".

The gate prompt already says "A discrepancy that changes no decision … is a note at most" (`phr@999c9ff`, 2026-09-12).

**Gates on docs and measurement units.** Archipeligo's measurement units that wrote code ran `xtask ci`. The docs-only ones reported `xtask ci` as "not-run: docs-only diff" (`stage2-horizon-confirm-completion.md:15`, `lp5-disclosure-note-completion.md:15`): under today's rule that is a `gate-not-run` stop. Cartomancer's docs units ran the full suite. No project in the record had no gate commands at all, so the gateless case is reasoned, not observed.

### Options

**Representing measurement work.**

| Option | What changes | Against the evidence |
|---|---|---|
| Nothing | as now | 19 measurement units across five types, 7 with a fail branch, and four Phase 2 re-entries on planned or plannable outcomes |
| Entry fields on any type | a Registration and an Outcomes field, wherever a unit's verdict is measured | carries the design and the branches. Review and the size rules cannot tell a measurement unit from the type, so the runtime would key on a block field instead |
| New type `measurement` | a type whose deliverable is a verdict read from a registered sample, with the two fields | review, rating and sizing rules key on the type, as `design` already is. A unit that builds and measures splits (sizing, below), as `stage1-gate` would |
| Route to `phronithm:investigate` | the unit's executor is an investigation | investigations can carry registered predictions and kill lines (kin-provisioning, reserve-gate). But `investigate` settles each claim at its achieved rigour and lets results reshape the question, and it has its own critique gate and document layout, so the runtime would gain a second unit lifecycle. Both cited investigations ran outside any plan, before the work they selected was planned |

**Where a branch is decided.** The architecture is written before the plan (Phase 2 steps 2–5, then breakdown), so the choice is what the architecture says about a decision that rests on a measurement:
- **As now:** it states the premise. The unit alone holds the branch, and a planned outcome reaches the gate as a false premise.
- **A conditional decision:** "if X, then A; otherwise B". This needs both branches designed in Phase 2.
- **An open design decision:** the architecture's existing Open design decisions list ("questions explicitly deferred, with the information needed to resolve them and when resolution is required", `skills/large-scale-feature/SKILL.md:106`) holds the decision as open until the measurement reports, with the outcomes it expects and, where Phase 2 can design one, the decision for each.
- **A calibration register:** the unit plus a register of measured values beside the plan.

**Superseded evidence (P-A10).**
- **Nothing:** the `document-text` verdict already absorbs re-plan items, so only D16 remains.
- **A gate clause:** misdescription and superseded figures become notes. A clause covering superseded figures would also cover 118/132, which did invalidate decisions; "where the decision stands" is the judgement the gate is already asked to make.
- **A calibration register** beside the plan. It adds a document every programme keeps, against one residual case.

**A project with no gates.**
- **A declared trivial command** (`true`): it tells reviewers a gate passed when none ran.
- **An exemption by unit type:** a measurement unit that writes a harness should not get one.
- **An explicit `gates: []` declaration.**
- **Nothing:** such a project cannot run any non-design unit. A block holding `hosts` alone does not help, since stages then report gates themselves.

**The store.** In the `lsf-plan` block, where a re-plan can rewrite it mid-run; in the `lsf-gates` block, which the runtime pins at launch and a re-plan cannot change without a `declared-gates-changed` stop; or in the runtime config. The pre-flight command reads only the plan, and `retained-artefacts-impl` must fail it on a `Retains:` line with no store, so the runtime config cannot carry it.

### Proposed decision

**U1, the `measurement` type.**
- **What it is:** a unit whose deliverable is a verdict or record read from a sample of runs, observations or judgements (*measured* in the Criteria Terms), where every outcome the reading admits is a valid result routed onward.
- **The type test:** a `feature` unit with a measured acceptance check (a benchmark target, a soak test) fixes a failing result inside the unit. A `measurement` unit reports whatever it finds. Breakdown's quality check asks the test of every unit whose done condition reads a sample.
- **Lenses:** its review runs `critique-code` with the done condition, as other code units do, and the `general` and `numerical` lenses.
- **Gates:** it runs the project's gates like any non-design unit.
- **Cost:** one enum value in `docs/plan-graph.md`, in the Go `plan` package and in `lensesByType`. The frozen Workflow script does not learn it, so a plan with a measurement unit runs on the Go runtime only.
- **Exploratory probes:** a probe whose reading cannot be fixed beforehand is not a measurement unit. It is an open unknown, resolved before planning by `phronithm:investigate` in the requester's session, as kin-provisioning and reserve-gate were. C2's "no model" feasibility probe is one: naming it discharges C2, and it runs before the unit that reads the criterion is planned.

**U2, the entry fields.** A measurement unit's entry carries:
- **Registration:** the C2 elements of the reading it makes: statistic, sample, horizon, decision rule, model, the needed and unacceptable levels, the two probabilities and the plausible level with its probability, and binding. Each element is given or cited from the brief's criterion.
  - A descriptive measurement (binding: reported only) states that it has no decision rule and gives its sample and horizon.
  - The unit commits its registration document, pinning the commit, binary, commands and seeds, before any run, as `stage2-horizon-confirm` did. Its done condition says so, and the code critique checks the commit order.
- **Outcomes:** every outcome the decision rule admits, plus "not completed", each with a route. A descriptive unit's rows are "completed" and "not completed", and a question its record puts to the requester routes from "completed". Each route is one of:
  - a unit in the plan that depends on this one;
  - a `requester-verdict` organisational gate on a named dependant;
  - or Phase 2 re-entry, for an outcome no plan can absorb.
- **Retains:** the paths kept under the retained-artefact policy (U8).

**U3, branches in the architecture.**
- **Open until measured:** Phase 2 lists each decision that rests on a measurement still to run under the architecture's Open design decisions, naming the measurement, its expected outcomes and, for each outcome Phase 2 can design, the decision taken. Breakdown drafts one measurement unit per entry, and its Outcomes rows follow the entry. Every unit that builds on the open decision depends on the measurement unit.
- **The gate:** the outcome of the unit an open decision waits on does not invalidate that decision. The gate returns it as a note naming the section, the Outcomes row and the decision the entry gave for that row. The clause covers only the listed decision: the same outcome falsifying any other decision is an invalidation as today.
- **Write-back:** the re-plan writes the entry's decision for that row into the architecture as `document-text`. Today's definition excludes "a correction that would change what a section decides", so it gains one case: an open decision resolved to the option the architecture itself gave for the reported outcome. An outcome whose decision the entry did not give is `cannot`, which stops the run as today.
- **Reporting the outcome:** the implement stage reports which Outcomes row held, and the code critique checks it against the registration. The completion report carries it, and the re-plan removes the dependants of rows that did not hold.
- **Requester routes:** a requester route is a `requester-verdict` gate on its dependant. It blocks that dependant and everything after it, which may be most of the plan (R2). The run ends with `no-ready-unit` when nothing else is ready, and the digest shows the uncleared gate. What it saves is the Phase 2 re-entry, not the wait for the requester.
- **What it does not prevent:** an outcome whose alternative needs a design Phase 2 could not write in advance still re-enters Phase 2 (R8, and R1 if its retune could not have been drafted).

**U4, rating.**
- **Clarity:** a measurement unit is `specified` only when its Registration and Outcomes are complete. Otherwise it is `exploratory`, with the registration as its one open decision; a second open decision extracts a preceding `design` unit, as the size budget already requires.
- **Risk:** risk is rated by consequence. A unit with an outcome routed to Phase 2 is `high`. A unit with a decision rule gives, on its Risk line, the pass probability at the plausible level (C2). A descriptive unit's risk is that of its "not completed" row.

**U5, sizing.**
- **The outcome seam:** a unit whose measured outcome selects among later units, or sets a value another unit tests, ends at the measurement. Work that consumes the outcome is a dependant.
- **Wall clock:** the wall-clock flag does not exempt a measurement unit from this seam.
- **Publishes:** the note's rule of more than one independently consumed Publishes item is not adopted. `phronithm:breakdown` is used outside lsf, the rule would split any unit publishing a type and its accessor to different consumers, and no case outside archipeligo supports it: cartomancer's was a surface count.

**U6, tuning companions.** Existing tuning infrastructure discharges the companion design unit only when the verification note shows three things:
- it searches the unit's parameters jointly, or states why they are separable;
- it reports a confidence or interval for the value it selects;
- it has run on the system the unit tunes.

Otherwise the companion stays. Infrastructure the measurement unit itself builds is not existing infrastructure, so it never discharges the companion.

**U7, superseded evidence.** Nothing. The re-plan's `document-text` verdict already absorbs the re-plan items behind D5, D6 and D9, and a clause naming superseded figures as notes would invite mislabelling a 118/132-style invalidation. The narrower alternative, declined: a gate clause making *a sentence that misdescribes what the merged code correctly implements* a note, and excluding any figure a decision's premise cites. It covers D16 alone. No calibration register.

**U8, `Retains:` and the store.**
- **`Retains:`:** a unit's `Retains:` paths go in its entry and in the `lsf-plan` block as `retains: [paths]`. Each is relative to the clone root, with no `..` and no leading `/`; the runtime refuses at the move a path that resolves outside the clone.
- **`store:`:** the project's store is a `store:` key in the `lsf-gates` block, an absolute host path outside the repository. It is pinned at launch with the rest of the block, so no re-plan agent can redirect a move. The path is machine-specific, as archipeligo's records citing `~/src/archipeligo-runs/` already are. `docs/plan-graph.md`'s content rule ("`gates`, `hosts` or both") gains `store`.
- **Load failure:** a `retains` entry in a plan whose `lsf-gates` block has no `store` fails the load, and so fails the pre-flight command.
- **Who writes `store:`:** breakdown, on the requester's answer, as for gate commands and hosts. A block holding `store` alone declares no gates and no hosts.
- **Runner:** Go runtime only. The Workflow script's parser rejects an unknown unit key (`workflows/lsf/graph.js`), so a plan carrying `retains`, or a `measurement` unit, fails its load as `invalid-plan`. The frozen script is not changed to accept them.

**U9, a project with no gates.** A plan may declare `gates: []` in its `lsf-gates` block, meaning the project has no gate commands.
- **Stages:** stages are told none are declared and return `gates` empty.
- **Reviewers:** reviewers are told the project declares no gates and nothing ran.
- **No stop:** neither `gate-omitted` nor `gate-not-run` fires.
- **Merges:** merges are not gated either.
- **`setup` and `hosts`:** `setup` is refused with an empty list, and `hosts` is unaffected. A block holding `hosts` alone still means "stages report their own gates".
- **Who writes it:** breakdown writes `gates: []` only on the requester's answer that the project has none, the Phase 1 step 4 question. The launch pin keeps a re-plan from writing it mid-run.
- **Runner:** Go runtime only. Under the Workflow script such a project still stops (R13).

### Accepted defaults

The requester accepted each of these defaults:
1. **The type.** A `measurement` type (U1), rather than fields on any type. Its basis is one programme: no other project has a measurement unit in breakdown format.
2. **Open until measured** (U3), and a requester route as an organisational gate rather than a stop. This changes the gate contract (a note, not an invalidation, for the listed decision) and the re-plan's `document-text` definition (one added case).
3. **Risk by consequence** (U4): only a Phase 2 route makes a unit `high`. A requester route that blocks most of the plan (U3, R2) is left unrated.
4. **The outcome seam** (U5), and not adopting the Publishes-count rule.
5. **The three-part test for tuning companions** (U6), drawn from one harness.
6. **P-A10:** nothing; the narrow misdescription clause (U7) is declined.
7. **`store:` in the `lsf-gates` block** (U8).
8. **`gates: []`** (U9) as the declaration for a project with no gates.

### Walk

| Case | Under U1–U9 |
|---|---|
| `stage1-gate` (`dc88280`) | U5 splits it. An `integration` unit takes the format bump, CI and defaults mechanism. A `measurement` unit takes the verdict, depending on it. U2 asks for a Registration. The entry had none, so U4 makes it `exploratory` until one exists, and C2's arithmetic then shows the 3-seed and 41-seed defects before planning (Criteria walk). U2 asks for an Outcomes row for "fails on held-out seeds", with a route that was drafted only after R1. U3 lists §10's stage-1 gate as open until the measurement reports. Whether R1 is then a planned branch depends on whether Phase 2 could have written the retune decision in advance; if not, it re-enters Phase 2 as before. U6: a one-at-a-time harness with no stated confidence does not discharge the companion, so `tuning-search-model` is drafted at planning, not at `ec1ec02` |
| `stage2-horizon-confirm` | U1: `measurement`, descriptive. U2: its registration-first clause stands as written. C1 defers "persists" to it, and its Outcomes row routes the horizon question to a `requester-verdict` gate on the criterion-3 unit, instead of a question in prose. U4: no outcome routes to Phase 2, so the risk is that of "not completed", which is `long-running-design`'s to bound. U8: `retains: [runs/stage1-gate/cell/stage2-horizon-confirm/]` moves the 48 GB to `<store>/stage2-horizon-confirm/` at cleanup, so no record cites a worktree path. The registration commit is reachable once the unit's branch merges; the tag was needed because the first attempt's branch never merged. U9 does not apply: archipeligo has gates. Its docs-only diff runs them under a declared block, and stops as `gate-not-run` without one, which is `script-retirement`'s declaration question |
| R2 (`stage1-retune`) | U2's Outcomes row "no candidate passes" exists in prose already (iii). U3 lists §12's "the grid is a sufficient resolution mechanism" as open until the retune reports. The outcome routes to a drafted dependant (the widened re-tune) or a requester gate. That gate blocks nearly every remaining unit, so the saving is the Phase 2 re-entry, not the wait |
| R6 (`stabilisation-screen`) | "Baseline passes" was in the registration but not the plan. U2 requires every outcome the rule admits, so the row exists, and U3 lists §10's cause 3 as open until the screen reports, with "nothing is adopted" as the decision for that outcome |
| R8 (`bid-era-rung`) | U5: `bid-trigger-commit`'s thresholds are a value `bid-era-rung` tests, so they are already separate units, as R7 left them. U2 and U3 route "thresholds do not hold", but §6.5 and §12 would change a rule, so the route is Phase 2: U3 does not remove that re-entry |
| D16 (cartomancer) | Under U7, unchanged: a stop and a requester round trip. Under the narrow clause it is a note, absorbed as `document-text` |
| `weighted-contest-rung` | U5 separates the trigger-threshold measurement from the gate flip and the bidding rung that consume it, which is R7's split, at planning |

### Generality

- **P-A4, P-A5, P-A6** (archipeligo only so far).
  - **Outside archipeligo's plans:** critic-fidelity-bench, kin-provisioning, reserve-gate and coronal-memory write registered outcomes that select later work, but none ran inside a plan.
  - **Scope of the burden:** U1, U2 and U4 bind only units typed `measurement`, so a programme with none bears nothing, apart from the type test in the quality check. U3 adds Phase 2 text only where a decision rests on a measurement.
  - **U6:** it tightens an escape clause that binds every plan using breakdown, including plans outside lsf. A plan with no tuning units is unaffected.
- **P-A9** (archipeligo only so far). Cartomancer's analogue is a surface count the existing budget caught. U5's outcome seam rests on archipeligo alone. It binds every plan by description, not by type, but only where an outcome selects later work or sets a value another unit tests.
- **P-A10** (recurs on cartomancer-v2).
  - **The register overstates it:** three of the four cartomancer decisions predate `document-text`, and archipeligo's four re-entries each moved a decision.
  - **U7:** nothing is changed; the declined narrow clause would have covered one case (D16).
- **Gates.** No observed project lacks gates. U9 is reasoned from the runtime's rule and costs nothing where a project declares gates.
- **The store.** It follows the retained-artefact policy's scoping. A project with no `Retains:` line declares no store.

### Plan consequences

- **`empirical-unit-impl`** carries U1, U2, U4, U5, U6 and U8's entry fields into `skills/breakdown/SKILL.md`, and the type, `retains`, `store` and `gates: []` into `docs/plan-graph.md`.
- **U3's Phase 2 clause** (open-until-measured entries) needs a carrier in `skills/large-scale-feature/SKILL.md::phase-2`. `empirical-criteria-impl` owns that section and is ready, so the re-plan either adds the clause to it or gives the section to `empirical-unit-impl` with a serialising edge.
- **`empirical-script-support`**:
  - takes the `measurement` type, its lenses and the Outcomes report in the implement return;
  - removes the dependants of rows that did not hold, at re-plan, and writes back the resolved decision as `document-text`, under the widened definition;
  - takes U3's gate clause and U9's `gates: []`.
- **`retained-artefacts-impl`** parses `retains` and `store` where U8 places them, with U8's path rules.
- **`script-retirement`**: its declaration decision now has U9 as the answer for a project with no gates.
- **`long-running-design`**: a measurement unit's run stage, and the route of an Outcomes row reported after a long run, are its to place.
- **`programme-close`** corrects the register:
  - P-A6: "declined because a harness existed", not "collapsed".
  - P-A10: the cartomancer cells predate `document-text`, and R1/R2 also moved decisions.
- **Register dispositions:** P-A4, P-A5, P-A6 and P-A9 are `deferred` to `empirical-unit-impl`; P-A10 is `declined` (requester verdict: the `document-text` verdict already absorbs re-plan items; the D16 residual is accepted).

## Retained artefacts

Unit `retained-artefacts-design`, plan @ `b7f5332`.

**Status:** decided, 2026-10-07. Requester verdict: **recipe-cited**. A record cites regenerable data by recipe and never calls it must-keep. Only non-regenerable data may be cited by path, and only under a store outside every checkout. Cleanup deletes ignored files unless a unit has declared them kept (requester, same day).

### Problem

P-D5 ([register](lsf-problem-register.md)): archipeligo records treat regenerable run series as must-keep, and cite them through a worktree path. Keeping them costs 80 GB in `~/src/archipeligo-runs/` and the 372 MB worktree `wf_b1de45fb-5d4-20`. Separately, cleanup force-removed a 190-series set (83 GB, P-C9) that nothing in the unit entry or the implement return had named.

### Evidence

Archipeligo citations are at `c220c62`.

- **Regeneration cost.**
  - The 48 GB `stage2-horizon-confirm` series (40 runs at 20k ticks, 40 at 100k) ran in 4 h 27 min (`archipeligo-runs/stage2-horizon-confirm/launch.log`: start 2026-09-29T06:02:34+13:00, finish 10:29:08).
  - The postmortem puts the removed 190-series set at about 8 h.
- **Determinism is shown only on a prefix.**
  - `trigger.md:51-58` records a 2 000-tick run at engine commit `950be81`, byte-identical (`cmp`) to the first 2 001 lines of one retained 20k-tick `population.csv`.
  - `identity.log` shows a 2 000-tick override run `dup-of` the committed run.
  - No full-length 20k or 100k series has been regenerated, and no `flows.csv` or `intake.csv` has been compared.
- **Recipe-cited has been chosen once already.** After the 190 series, their binaries and `launch.sh` were lost, the requester chose to regenerate them only on demand (`bid-era.md:82-92`, erratum 2026-10-01). The recipe given was:
  - build `xtask` at `dd8932c`;
  - run the record's four *Launch* commands;
  - check every file against `bid-era-series.txt`.

  That recipe has never been run. It depends on launch commands inlined in the record, because the launch script is gone.
- **Worktree-path citations.** `trigger.md:54-55` and `bid-era-runs.txt:192` onward cite paths under `wf_b1de45fb-5d4-20`; `bid-era.md:90-92` relies on the worktree's symlink to keep those paths valid. `foundation-reconciliation.md:84-87` keeps the worktree and its branch because "they still anchor `2f220af`". Tag `stage2-horizon-registration` already points at `2f220af` (`foundation-close-out.md:127`).
- **The store's contents.**
  - `stage2-horizon-confirm` is 48 GB, cited by `horizon.md`'s *Series retained* table, by size and blake3.
  - `b1-frozen-regime` is 26 GB, cited by `b1-frozen-regime-results.md:5`.
  - `hoard-cell` is 6.6 GB, cited by `hoard-cell/record.md:120-125`, by realpath and sha256.
  - The store's README asks that anything regenerated for a record go in its own subdirectory, with the record naming the path.
- **Non-regenerable data.** The maths campaign (`~/src/maths-proofs/tools/campaign/README.md`, Telemetry and artefacts) records model transcripts, cost, tokens and latency, which no recipe reproduces. Its runner writes them to a host `--out-dir`, which plays the store's role. The agent, in the `ic` container, can write only the clone. lsf's own call streams and telemetry are already in the runtime's state dir, outside the worktree (`lsf-runtime.md`, Journal and state dir).
- **Cartomancer** cites no retained data (register, P-D5 probe).

### Options

| Option | Record may cite as must-keep | Cost |
|---|---|---|
| Recipe-cited | non-regenerable data only, under a store | each record carries a full recipe. The recipe can rot (toolchain drift, a lost script), and regenerating takes time when a check is wanted and the cache is gone |
| Retained path in a store | any data, under a store outside every checkout | storage grows without bound; records depend on disk state |
| Retained path in a worktree | any data, in the producing worktree | as above, plus cleanup must keep the worktree; this is where the archipeligo records are now |
| Recipe plus cost threshold | regenerable data above a regeneration-cost threshold the plan declares | a threshold no evidence places |
| Nothing beyond cleanup | unconstrained | cleanup stops deleting, but records may still cite worktrees |

Weighing regeneration against storage: under recipe-cited, a record stays checkable whichever cost is paid. Keeping regenerable data as a cache becomes a storage decision the requester can revisit. It is no longer an obligation a record creates. At the measured rate, dropping a 48 GB series costs about 4.5 h of reruns, paid only when a check is wanted.

### Decision

**Classes.**
- *Regenerable:* the record carries a recipe, and a full-length regeneration of at least one output has matched every recorded hash. A prefix or a shorter run does not qualify.
- *Non-regenerable:* everything else.

The burden of proof is on regenerability.

**A recipe** names:
- the source commit, reachable from a tag or a merged branch, not only from a unit branch;
- the toolchain (pinned file or version), the dependency lockfile, and the platform the hashes were taken on;
- the build command, or the binary's hash if a built binary is used;
- the launch command with every seed and input, in version control or inlined in the record;
- each output file's size and hash.

**Records.**
- Regenerable data is cited by recipe. A record may say where a cached copy is, but must not require that copy to exist.
- Non-regenerable data is cited by a path under the project's store.
- No record cites a worktree path or a path inside any checkout, directly or through a symlink.

**Store.** The project declares one store directory outside every checkout; `archipeligo-runs/` is one. A `Retains:` line in a project with no store is a plan error.

**Unit entry.** A `Retains:` line names the worktree paths a unit keeps. Nothing else a unit produces outside version control is kept.

**Cleanup**, for each worktree whose branch merged:
- paths named by `Retains:`: the runtime moves them to `<store>/<unit>/`, then removes the worktree;
- every other ignored or untracked file: removed with the worktree.

Cleanup lists what it removed in its report. Keeping data is a deliberate act, the declaration; the default is deletion.

**The move** is a runtime step, journaled like any other, so a kill mid-move resumes it. `<unit>` is the plan's unit id, not the runtime's worktree key. If the destination already exists, the move stops rather than merge into it. The implementing unit settles cross-filesystem copy and verification.

**On a substrate with no storage outside the worktree** (the maths exemplar's `ic` container): the runtime does the move, on the host; the agent does not. The agent writes under its worktree and declares the paths. The policy holds without the agent getting any write access outside the worktree.

### Walk against P-D5's records

| Record | Under the policy | Change |
|---|---|---|
| `horizon.md` *Series retained* | not yet regenerable: no full-length run has been regenerated | add the recipe: commit `2f220af` (tagged `stage2-horizon-registration`), toolchain, the two launch commands inlined (they are in `launch.sh` in the store, not in git). Regenerate one h100k run and match its three hashes; until then the store copy is must-keep |
| `bid-era.md:51` | the hash-equality condition holds for a regenerated series | none |
| `bid-era.md:90-99`, `bid-era-runs.txt` | non-conforming: paths under the worktree, kept valid by a symlink | cite the store or the recipe, not the worktree path |
| `trigger.md:54-55` | non-conforming: a worktree path | cite the `horizon.md` recipe and the file's hash |
| `bid-era.md:82-92` erratum (190 series) | non-regenerable by the class rule: never regenerated | as written it is the policy's shape; it becomes regenerable after one full-length check |
| `hoard-cell/record.md:120-125` | non-regenerable until checked; cited by store path, which is allowed | none required |
| `b1-frozen-regime-results.md:5` | non-regenerable until checked; cited by store path | none required |
| `foundation-close-out.md:122-126` | keeps the worktree and branch for the path citations and to anchor `2f220af` | once the three path citations change, the worktree and branch can go; the tag anchors `2f220af` |

Under the policy, none of the 80 GB may be dropped before a full-length check. After one passes for a series, that series becomes an optional cache. Changing the archipeligo records is the requester's call and lies outside this programme.

### Generality

P-D5 has not been shown outside archipeligo. Cartomancer produced no retained data. The maths campaign shows that a second project has non-regenerable data, not that it has the problem.

The two parts of the decision are adopted on different bases:
- **The citation rule** is scoped to programmes whose records cite measured data. A project with none declares nothing.
- **The cleanup rule** is adopted generally. Deleting undeclared files is cleanup's present behaviour, so a project with no `Retains:` line sees no change. The 83 GB loss (P-C9) is prevented by the declaration and by the ban on citing worktree paths, not by cleanup keeping what nobody classified.

### Plan consequences

The policy splits across units. The re-plan names or adds carriers:
- **`code-merge-cleanup`:** cleanup deletes every undeclared ignored and untracked file and lists them in its report. Its done condition currently says the opposite ("keeps a worktree holding ignored files unless the retained-artefact policy names them rebuildable") and is rewritten at the re-plan. Its scope also needs `lsf-workflow.md`'s merge-and-cleanup section, where "a failure leaves residue, not risk" is restated: the risk is now confined to undeclared data.
- **`Retains:`, the store and the move:**
  - the `Retains:` field in the plan block and its parse;
  - the store declaration;
  - the runtime's journaled move to `<store>/<unit>/`.

  These need a carrier. `empirical-unit-design` decides where the declaration lives. The move is runtime work that falls after `code-merge-cleanup`; until it lands, a `Retains:` line keeps the worktree.
- **`empirical-unit-impl`:** the recipe as a record's citation form, and the regenerable/non-regenerable classes in the breakdown skill.
- **`long-running-design`:** a job's outputs follow the same classes.
- **`lsf-runtime.md`:** Not in this unit still assigns the ignored-file policy to `code-stage-interface`, so the pointer moves to `code-merge-cleanup`.
