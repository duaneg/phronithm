# LSF empirical work

How lsf plans, runs and records work whose outcome is a measurement. Sections are owned by units of the [LSF improvements programme](../plans/lsf-improvements-breakdown.md): Criteria (`empirical-criteria-design`), Units (`empirical-unit-design`), Retained artefacts (`retained-artefacts-design`).

## Criteria

Unit `empirical-criteria-design`, plan @ `14f2a07`.

**Status:** decided, 2026-10-10. Requester verdict: **C1–C5 accepted as proposed**, with C4 and C5 adopted generally. P-A1's geometric part is declined.

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

None of the vision options has been tested. Whether a Phase 1 session, asked the question about criterion 3, would have named a world that persists without moving has not been measured. A probe of 3–5 fresh-context sonnet calls, given only `vision.md` and criterion 3's text, would measure it.

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
| Frozen world | C4: criterion 3 stands for `vision.md:9-10, 27-28`; it cannot be tagged as serving criterion 1, a binary critique-gate check that stands for no passage. The question asks for a world that persists and is not interesting or evolving. Whether a Phase 1 session would have answered with a static world is untested |
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
