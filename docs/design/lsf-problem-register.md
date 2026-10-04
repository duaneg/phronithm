# LSF problem register

The problems the [archipeligo postmortem](lsf-postmortem-archipeligo-foundation.md) and [lsf-improvements](lsf-improvements.md) §1 record, stated without remedies and tested against cartomancer-v2's quality-bar-interiors stream (41 units, 2026-09-10 to 09-19, 18 `lsf-implement` runs). Units of the [LSF improvements programme](../plans/lsf-improvements-breakdown.md) cite these ids. Postmortem line numbers are at `phr@f3a395a`; `arch@` SHAs are archipeligo-evolved, `carto:` paths are `~/src/cartomancer-v2`.

## Columns

- **Problem**: the observed event and its consequence. It names no remedy and does not describe the friction as a remedy's absence.
- **Cost**: a quantity, run, SHA or event from the postmortem or the cited record.
- **Generality**: one class, by precedence:
  1. `harness-imposed`: the root cause is Claude Code or Workflow-tool behaviour, whether or not lsf could mitigate it.
  2. `context-free defect`: a defect or gap in lsf's own code or text that occurs on any project; the cell cites the line.
  3. `recurs on cartomancer-v2`: a cartomancer probe found an instance; the cell cites it.
  4. `archipeligo only so far`: the probe found none. `(not testable: …)` marks a row cartomancer could not have shown.

  Appendix B records every probe, including cartomancer instances of rows classed higher.
- **Topic**: non-binding grouping for consumers: `mechanical-stage`, `long-running`, `relay`, `driving`, `empirical-design`, `findings`, `state`, `other`.
- **Disposition**: empty until `programme-close` fills it: `fixed` with the commit, `declined` with the reason, or `deferred` to a named successor.

## §A Empirical work

| Id | Problem | Cost | Generality | Topic | Disposition |
|---|---|---|---|---|---|
| P-A1 | Gate designs whose defects arithmetic could show reached Phase 3 and were found only after a stop: a 3-seed gate certifying π ≥ 0.37; its 41-seed replacement passing a π ≈ 0.995 cell with probability 0.81; a per-seed stage-2 rule passing with P ≈ 0.002 at the measured rate. | Re-entries R3 (`arch@4a75e94`), R4 (`arch@0569c92`), R5 (`arch@a01fe05`) | recurs on cartomancer-v2: geometric, not statistical; §4.11's anchor fractions left no room for panel headroom on the 1820×1540 sheet, found at the wave-2 review-gate stop (carto: `docs/plans/quality-bar-interiors-decisions.md` D13, D14) | empirical-design | |
| P-A2 | The architecture cited, as evidence, a Phase 1 figure measured on the pre-tissue engine with a classifier the brief had called invalid. | `arch@a169bae`; re-entry R1 | recurs on cartomancer-v2: panel sizing used advance widths calibrated on Liberation Serif for roles that render in DejaVu Serif (carto D17; `quality-bar-interiors-breakdown.md:7300`) | empirical-design | |
| P-A3 | Brief criterion 3's "persists" had no horizon until 10-01; run #16's return flagged it as a requester matter two days before it stopped run #22. | Run #22 stop | archipeligo only so far (not testable: cartomancer's criteria were binary or count checks) | empirical-design | |
| P-A4 | Outcomes that needed runs to find (R1, R2, R6, R8) arrived as stops and Phase 2 re-entries, not as planned branches. | 4 of 8 re-entries; ~92 h stop-to-relaunch across all 8 (upper bound) | archipeligo only so far (not testable: cartomancer had no run-dependent outcomes besides planned requester verdicts) | empirical-design | |
| P-A5 | `stage1-gate` was rated "Risk: high — first evaluation of the new regime; nobody has run it" and "Clarity: specified", with no seed count or confidence level, and went to Phase 3 so. | `arch@dc88280`; R3 | archipeligo only so far (not testable: no sampled units; four high-risk specified units existed, e.g. `gate-port`) | empirical-design | |
| P-A6 | Breakdown collapsed `stage1-gate`'s companion tuning-infrastructure unit because a one-parameter-at-a-time harness existed; the companion units were added later anyway. | `arch@ec1ec02`, `arch@6921509` | archipeligo only so far | empirical-design | |
| P-A7 | Campaigns of hours, typed `feature` or `integration`, ran inside one single-shot implement stage, the longest on one core of eight. | 18 stages, 39.7 agent-hours; longest 9.1 h | archipeligo only so far (not testable: no campaign units) | long-running | |
| P-A8 | stage2-horizon-confirm's 80 runs outlived its implement stage; review found "the record has no results" and the run stopped `gate-non-convergence`. | stage2-horizon-confirm stop | archipeligo only so far (not testable: no campaign units) | long-running | |
| P-A9 | Units with uncertain outcomes or several independent deliverables passed the size check: the requester split weighted-contest-rung by eye, and its successor, passed at the decision re-plan, was split again at R7. | weighted-contest-rung; R7 | recurs on cartomancer-v2: `tactical-interior-tier` kept as planned at D7 and D10, then split at the wave-1 re-plan (carto `afb311c`, rp-7) | empirical-design | |
| P-A10 | Measured values superseded figures the architecture cited as evidence, with no decision moving, and each cost a requester round trip; R1 and R2 went through Phase 2 for §13 evidence paragraphs. | R1, R2 | recurs on cartomancer-v2: D5, D6, D9, D16 were requester decisions for stale or superseded text, each "No decision moves" (carto `docs/plans/quality-bar-interiors-decisions.md`) | empirical-design | |
| P-A11 | No version of criterion 3, original or relaxed, measured dynamics; the post-close vision review found the certified world frozen in 22 of 40 runs. | Six redefinitions, three relaxations; `arch@d72e998` | archipeligo only so far | empirical-design | |
| P-A12 | Re-entries R4, R5 and R6 each repaired a defect an earlier re-entry introduced. | 3 re-entries | archipeligo only so far (not testable: no Phase 2 re-entries) | empirical-design | |
| P-A13 | A stage2-horizon-confirm process kept running after its stage, with no workflow record of it, until the session relaunched with D12. | Orphan run; D12 | archipeligo only so far (not testable: no campaign units) | long-running | |

## §B Findings below Critical

| Id | Problem | Cost | Generality | Topic | Disposition |
|---|---|---|---|---|---|
| P-B1 | Significant findings were recorded and not acted on in-loop, and two came true: stage2-gate's plan critique named the fallback its implement stage took and the rule the requester later replaced; bid-era-rung's §3.2 draw-ceiling finding reached the gate as a note and the requester ruled it an invalidation. The requester carried findings into launches by hand twice. | D18; hand carries D11, D19 | context-free defect: `lsf-workflow.md:172`, `:449`; `implement.js:538`; `scheduling.js:369` | findings | |
| P-B2 | The close-out audit counted 37 rows from completion reports and declared pre-2026-09-20 units out of scope; about 26 review-stage rows on units it did not see stayed unaudited, and two unaudited findings remain on archipeligo `main`. | 26 rows; 2 open findings | recurs on cartomancer-v2: `docs-close-out` swept the 38 completion reports and two acceptance reports, never the signals ledger (0 references), whose 382 `finding` rows are `recorded` (carto `docs/plans/docs-close-out-plan.md:98`, `:519`) | findings | |
| P-B3 | One finding appears as several ledger rows, one per review round, so counts overstate findings. | release-assert-sweep: 7 rows ≈ 2 findings | context-free defect: `implement.js:647-648` stamps rows with stage and round only | findings | |
| P-B4 | The frozen-world defect surfaced only after the requester accepted the final digest and asked for a vision review. | Post-close vision review; 22 of 40 runs frozen | context-free defect: `skills/large-scale-feature/SKILL.md` Termination | findings | |

## §C Mechanical stages and permissions

| Id | Problem | Cost | Generality | Topic | Disposition |
|---|---|---|---|---|---|
| P-C1 | Load stages returned a placeholder (`"see tool results file"`) or a truncated plan block, and the run stopped `invalid-plan` though the committed plan was valid. | 2 of 59 loads (`wf_405a2c0f-0ec`, `wf_b21a2e97-f5f`); a requester round trip and relaunch each | recurs on cartomancer-v2: 2 of 33 load returns differed from the committed block, one moving a line and one rewriting characters; neither was a placeholder or truncation, and neither stopped the run (sessions `1f6243e9`, 2026-09-11T11:17Z; `4e0cd888`, 2026-09-12T06:48Z) | mechanical-stage | |
| P-C2 | Ledger appends overwrote waves 0–2, deleted D9's Wave 0 section with the append-only header, deleted a prior row, and filed wave-3 rows under "wave 4". | 4 faults: `arch@daf9289` (restored `arch@363a312`), `arch@1250655` (never restored), `arch@5a15fd1`, `arch@4953b1a` | recurs on cartomancer-v2: an append dropped its `## Wave 1` heading, filing 64 rows under Wave 2 (carto `cdf14ff`); 6 of 27 appends altered or dropped rows (e.g. `60ffa29`, `21a61c8`) | mechanical-stage | |
| P-C3 | Completion-report copy stages dropped text: release-assert-sweep's "7 Significant … left unfixed" line, hiding them from the close-out audit, and a phrase in tuning-search-instrument's report. | 2 of 47 checked; `arch@8100213` | archipeligo only so far (not testable: cartomancer's completion-report stage wrote the report from stage returns; it had no verbatim text to copy) | mechanical-stage | |
| P-C4 | The decision log, transcribed by an agent, carried an invented `clears` entry, and the next load stopped. | 1 of 13; `arch@87f88ae` | recurs on cartomancer-v2: decision-log agents dropped the block's closing fence three times (carto `df7cd8c`, `f7bed16`, `89c7187`), and a load stopped `invalid-plan` (2026-09-15T03:14Z); D18's decision text was shortened from 1297 to 1038 chars (`b44df42`) | mechanical-stage | |
| P-C5 | Cleanup's `git branch -d` was denied, leaving branches for the requester to delete. | 3 of 37 cleanups | archipeligo only so far | mechanical-stage | |
| P-C6 | After a denial, a cleanup agent tried to write `.claude/settings.json` to allow itself `git worktree remove` and `git branch -d`, and was denied as self-modification. | 1 attempt | archipeligo only so far | mechanical-stage | |
| P-C7 | A transient API 529 returned `agent-null` and stopped run #2. | Run #2 stop | context-free defect: the first null raises, `implement.js:641`, `:669`; `scheduling.js:214`, `:223` | mechanical-stage | |
| P-C8 | A ledger stage whose commit was denied reported success: once with HEAD as its SHA, so wave-3 rows landed under "wave 4"; once with `""`, read as a conflict. | `arch@4953b1a`; a fake conflict (stop added `phr@1e3dcec`) | archipeligo only so far: no haiku commit was denied; 3 ledger agents returned 40-char SHAs with fabricated tails after real commits (e.g. `e87c394a3e4e…`, not an object) | mechanical-stage | |
| P-C9 | The permissions in force denied `git branch -d` but allowed `git worktree remove --force`. | 3 denials; the `--force` removals took 83 GB of untracked series with them | archipeligo only so far | mechanical-stage | |

## §D Workflow state

| Id | Problem | Cost | Generality | Topic | Disposition |
|---|---|---|---|---|---|
| P-D1 | The architecture SHA a run judges against is fixed at load, so a unit's legitimate amendment, already on main, was judged as an invalidation. | Run #8 stop on `arch@522ed9b`; five requester commits; phronithm#2 | context-free defect: `implement.js:1019` | state | |
| P-D2 | Review rounds after a fix were given the implement stage's gate results, not the fix stage's re-run; release-assert-sweep spent two fix rounds re-running CI against the same "CI not observed" Critical. | 2 fix rounds | context-free defect: `implement.js:759-760` | state | |
| P-D3 | Gate commands running past the Bash tool's 600 s limit moved to the background; agents ended a turn waiting and returned with gates not run. | 7 agents across code, docs and measurement units | harness-imposed | long-running | |
| P-D4 | Plans fresh from Phase 2 breakdown stopped at load on `unserialised-overlap`. | Runs #10, #14 | context-free defect: breakdown's overlap check is prose (`skills/breakdown/SKILL.md`), load's is code (`scheduling.js`) | state | |
| P-D5 | Three records call a worktree's untracked run series must-keep, though the data is regenerable in about 8 h; keeping it costs 83 GB of disk. | 83 GB; records `arch:docs/architecture/foundation.md:2459`, `arch:docs/plans/foundation-breakdown.md:1830`, `arch:docs/plans/foundation-close-out.md:124` | archipeligo only so far | state | |
| P-D6 | Resuming a run resets an interrupted implement attempt's reused worktree (`git reset --hard <base> && git clean -fd`, `implement.js:295`), discarding its commits and untracked files. | none | context-free defect: `implement.js:295`; worktree reuse on resume, `lsf-workflow.md:57-60` | state | |

P-D6 has no postmortem locus; it was found in planning `cleanup-data-safety` (`docs/plans/cleanup-data-safety-completion.md`). The worktree reuse it depends on is harness behaviour, but the reset is lsf's; the unit entry fixes the class. The same reset opens all seven worktree stage prompts (`implement.js:353`, `:381`, `:395`, `:419`, `:431`, `:443`, `:501`); this row covers the implement attempt.

## §E The requester interface

| Id | Problem | Cost | Generality | Topic | Disposition |
|---|---|---|---|---|---|
| P-E1 | Most requester input while driving was mechanics: launch, pause, retry, recovery, handoff. | ~60% of requester inputs (09-19 to 09-26); 76–90% of Bash calls in pure driving sessions (09-27 to 10-02) | recurs on cartomancer-v2: about 2/3 of ~53 requester inputs across 18 driving sessions (09-10 to 09-19) were launch, resume, pause or status | driving | |
| P-E2 | A decision's options omitted the consequence it turned on: "Size to 41 seeds (π* ≥ 0.928)" without a good cell's pass probability. Twice the requester took a decision to a more capable model, which reframed it; four of the five framing sessions ran on sonnet and both rethinks on Fable (a correlation). | The 41-seed choice: a Phase 2 re-entry and a 4.85 h run, reversed at R4 | archipeligo only so far: no model change in any cartomancer driving session; closest, one recommended option omitted the consequence the critique named (session `771809f3`, 2026-09-18T13:13:43Z) | driving | |
| P-E3 | "Keep baseline (Recommended)" was offered after the adversary showed the alternative winning every row, with no stated basis (n = 1). | One requester query | recurs on cartomancer-v2: "Doc-only: accept as a named non-pass (Recommended)" against a critique calling the defect systemic; the requester chose the code fix, D18 (session `771809f3`, 2026-09-18T13:13:43Z) | driving | |
| P-E4 | Three stage-1 stops each offered options inside the same tuned surface; three investigations ran by hand outside lsf and settled what the stops could not. | 3 stops; 3 out-of-loop investigations | recurs on cartomancer-v2: `unit-over-budget` stops recurred across D7, D10, D11 with the same options; the fix came from a phronithm change outside the loop (session `4e0cd888`, 2026-09-12; `phr@087ee9b`) | driving | |
| P-E5 | Organisational gates that needed no in-run evidence each cost a stop: a scope verdict flagged four times, an audit-reading task, criterion 3. | 3 stops | recurs on cartomancer-v2: `wf_2d6d890f-c2d` was launched with organisational gates `requester-verdict` and `roll20-visit` uncleared and stopped after 3 min with six `no-ready-unit` signals (session `7bfcdbeb`, 2026-09-17) | driving | |
| P-E6 | Re-entries and relaunches were handed over as pasted prompts of up to about 3 KB, until the requester asked for them to go in memory. | Up to ~3 KB per handoff | recurs on cartomancer-v2: agent-authored launch prompts of 0.4–1.6 KB pasted into fresh sessions, and a 78 KB seed file (sessions `206ae46f` to `3a4bdf06`, 09-10 to 09-14) | driving | |
| P-E7 | Handoffs asserted state that was false: "keep worktree -22" after cleanup had deleted it; a stale TUI URL. | 2 events | recurs on cartomancer-v2: a session said clearing was safe while a resumed run was live, and the run aborted without its journal (session `f9da76de`, 2026-09-14T08:33Z); a memory-prescribed `scriptPath` launch was rejected (session `5292ac3d`) | driving | |
| P-E8 | Decision D12 lived only in the args of a run stopped within two minutes, and D13 had to restate it. | D13 restatement | archipeligo only so far: decisions were re-passed in launch args after being committed (session `4e0cd888`), but none lived only there | driving | |
| P-E9 | A recovering session attributed to the requester a kill that its predecessor's own TaskStop caused, and the wrong cause entered memory. | 1 event | recurs on cartomancer-v2: a session stated that no workflow was running because the resume "failed immediately", though the run had gone 16 min and aborted; the cause was never established, and nothing was written to memory (session `19236536`, 2026-09-14T09:07Z; `wf_c6a850cc-23e`) | driving | |

## §F Harness relay

| Id | Problem | Cost | Generality | Topic | Disposition |
|---|---|---|---|---|---|
| P-F1 | The message that triggered the launch turn reaches every agent, ranked above its computed task; agents acted on it: two implement agents answered a question about plugin installation and the wave stopped, a load agent began on "Update CLAUDE.md", 24 haiku agents invoked `phronithm:large-scale-feature`, and a cleanup agent attempted a nested launch. | 20 of 28 runs carried a relay; run #6 stopped `done-condition-unevaluable` | harness-imposed | relay | |

## §G Phase 2 gate discipline

| Id | Problem | Cost | Generality | Topic | Disposition |
|---|---|---|---|---|---|
| P-G1 | R3, gated by a sonnet critic alone, passed the 41-seed design that R4 reversed about 25 h later. | R3 → R4, ~25 h | archipeligo only so far (not testable: no Phase 2 critique gate; the requester declined one, carto D2) | empirical-design | |
| P-G2 | The R6 static-check instrument existed from Phase 1 but first ran as Phase 3's first unit, nine hours after the architecture gate closed; it returned three FAILs. | 4 amendment rounds; 2 design units extracted; architecture 63 KB → 90 KB in a day | archipeligo only so far | empirical-design | |
| P-G3 | lsf-implement had run only on code units before archipeligo; its stop taxonomy met measurement units for the first time in a live programme. | The 8 measurement-unit stops behind re-entries R1–R8 | recurs on cartomancer-v2: its runs met `invalid-plan`, `unserialised-overlap`, `claimed-disjoint-conflict` and `merge-conflict-unresolvable` stops while delivering (carto signals ledger lines 13-17, 537, 665, 1161) | other | |
| P-G4 | Adversary-gated re-entries also introduced defects later reversed. | R1, R5 | archipeligo only so far (not testable: no adversary-gated re-entries) | empirical-design | |

## Improvements §1

| Id | Problem | Cost | Generality | Topic | Disposition |
|---|---|---|---|---|---|
| P-I1 | While a run was live the requester asked for an out-of-plan tooling fix (`b3sum` in the devshell's `flake.nix`); Phase 3 commit discipline forbids session commits and gives such a request no path. | No cost quantified: the occasion, `arch@0da91c5` | context-free defect: `skills/lsf-drive/SKILL.md:207-209` | driving | |

P-I1 has no postmortem locus.

## Appendix A: postmortem mapping

| Line | Item | Id(s) |
|---|---|---|
| 3 | Document scope | — (preamble) |
| 5 | SHA and line-number conventions | — (preamble) |
| 9 | Evidence: archipeligo records | — (evidence base) |
| 10 | Evidence: 28 launches, run directories | — (evidence base) |
| 11 | Evidence: session transcripts; earlier deleted | — (evidence base) |
| 12 | Evidence: phronithm commits | — (evidence base) |
| 14 | Wall-clock and agent-hour caveat | — (evidence limit) |
| 20 | Phase 3 launches: 27 of 28 not `complete` | — (aggregate of all stop rows) |
| 21 | Endings by class | — (aggregate: P-A4, P-C1, P-C4, P-C7, P-C8, P-D1, P-E5, P-F1) |
| 22 | Phase 2 re-entries: 8, ~92 h | P-A1, P-A2, P-A4, P-A12 |
| 23 | Run time vs window | — (aggregate) |
| 24 | Agent-hours | P-A7 |
| 25 | Review yield | P-B1, P-B3 |
| 26 | Haiku faults 14/218 | P-C1, P-C2, P-C3, P-C4, P-C5, P-C6, P-C8 |
| 27 | Data lost: 83 GB | P-D5 (the "lost" framing is not adopted) |
| 28 | Units added after the plan | P-A4, P-A9 |
| 30 | Throughput rose | — (what worked) |
| 34 | Findings ordering note | — (preamble) |
| 38 | §A: every re-entry followed a measurement stop | P-A4 |
| 40 | Gate designs whose defect arithmetic shows | P-A1 |
| 41 | Evidence measured on another system | P-A2 |
| 42 | An undefined criterion | P-A3 |
| 44 | Other re-entries; R4–R6 repaired earlier defects | P-A4, P-A12 |
| 46 | stage1-gate risk vs clarity; companion unit collapsed | P-A5, P-A6 |
| 48 | Lead-in to unit-level cases | — (lead-in) |
| 50 | Campaigns in single-shot implement stages | P-A7 |
| 51 | Campaign outlived its stage; orphan process | P-A8, P-A13 |
| 52 | Unit size by open decisions | P-A9 |
| 54 | Measured values in the architecture | P-A10 |
| 56 | Criterion did not test the vision | P-A11 |
| 60 | §B: the loop drops sub-Critical findings | P-B1 |
| 62 | Fix stage gets Criticals only | P-B1 |
| 63 | Implement never sees plan critique | P-B1 |
| 64 | Wave gate does not re-evaluate findings | P-B1 |
| 65 | Re-plan item kinds exclude them | P-B1 |
| 66 | No stage reads the ledger | P-B1 |
| 68 | "Consequences:" | — (lead-in) |
| 70 | Findings came true unaddressed | P-B1 |
| 71 | Requester patched the gap by hand | P-B1 |
| 72 | Close-out audit coverage; rows repeat | P-B2, P-B3 |
| 74 | Precision unmeasured | — (evidence limit; `significant-precision-sample`) |
| 76 | No programme-level close-out | P-B4 |
| 80 | §C: load echo among 14 faults | P-C1 |
| 84 | load | P-C1 |
| 85 | ledger append | P-C2, P-C8 |
| 86 | completion-report copy | P-C3 |
| 87 | decision log | P-C4 |
| 88 | cleanup | P-C5, P-C6 |
| 89 | relay | P-F1 |
| 91 | `--force` allowed, deleted 83 GB | P-C9, P-D5 |
| 93 | Proposal coverage note | — (about proposals, not a problem) |
| 95 | 529 → `agent-null`; decision log transcribed by an agent | P-C7, P-C4 |
| 99 | Architecture pin | P-D1 |
| 100 | Gate results after a fix | P-D2 |
| 101 | Gates that outlive the tool timeout | P-D3 |
| 102 | Plan checks only at load | P-D4 |
| 103 | Retained data | P-D5 |
| 107 | §E: mechanics share of requester input | P-E1 |
| 109 | Compressed decisions | P-E2 |
| 110 | Recommendation without its basis | P-E3 |
| 111 | Options inside a failing frame | P-E4 |
| 112 | Pre-clearable gates | P-E5 |
| 113 | Handoff | P-E6, P-E7, P-E9 |
| 114 | Launch-argument decisions | P-E8 |
| 118 | §F: harness relay | P-F1 |
| 120 | Relay: off-topic | P-F1 |
| 121 | Relay: launch instruction | P-F1 |
| 122 | Relay: acknowledgements | P-F1 |
| 124 | Tool-result launches carried none | P-F1 |
| 128 | Adversary at Phase 2 gates; R3 sonnet-only | P-G1, P-G4 |
| 129 | Phase 1 instrument ran in Phase 3 | P-G2 |
| 130 | Shaken down on code units only | P-G3 |
| 134 | What worked: invalidation stops real | — (what worked) |
| 135 | What worked: opus adversary | — (what worked) |
| 136 | What worked: critique-plan, fix loop | — (what worked) |
| 137 | What worked: seeded relaunch | — (what worked) |
| 138 | What worked: upstream fixes | — (what worked) |
| 139 | What worked: per-item re-plan verdicts | — (what worked) |
| 140 | What worked: reconciliation, decision log | — (what worked) |
| 141 | What worked: manual `/clear` | — (what worked) |
| 145 | Project-side items | — (project-side; archipeligo's tracker) |
| 149 | Data gap: early transcripts deleted | — (data gap) |
| 150 | Data gap: wall-clock attribution | — (data gap) |
| 151 | Data gap: hand classifications | — (data gap) |
| 152 | Data gap: precision unmeasured | — (data gap) |
| 153 | Data gap: AskUserQuestion notes | — (data gap) |
| 154 | Data gap: cartomancer run identification | — (data gap) |

## Appendix B: generality probes

Sources: carto signals ledger (`docs/plans/quality-bar-interiors-signals.md`, 1864 lines), decisions log and breakdown; 18 run JSONs and 466 workflow-agent transcripts under `~/.claude/projects/-home-duaneg-src-cartomancer-v2/`; 41 main-session transcripts. Mechanical probes were scripts over these; judgement probes were read-only searches with citations.

| Id | Probe | Source | Result |
|---|---|---|---|
| P-A1 | Gate designs with arithmetic-visible defects found after a stop | decisions D13, D14, D17; breakdown | Found, geometric: D13's anchor fractions vs panel headroom (RenderError on the 1820×1540 sheet); D14 x = −95.9. No statistical gates |
| P-A2 | Evidence from another system or instrument | breakdown:7300-7312; signals:1322 | Found: D17's Liberation Serif calibration applied to DejaVu Serif roles |
| P-A3 | Criterion without horizon or decision rule causing a stop | brief SC1–SC7; signals:1814 | None: criteria are binary or counts; a "commit" wording ambiguity surfaced as a finding, not a stop |
| P-A4 | Run-dependent outcomes arriving as stops | breakdown risk registry; signals:1533-1549 | Not testable: only planned requester verdicts were run-dependent |
| P-A5 | High-risk units rated specified | breakdown:83-88, 1539-1542, 1847-1850, 2314-2317 | Four high-risk specified units, none sampled; `gate-port` led to D13, D14, D17 |
| P-A6 | Companion unit collapsed at breakdown, re-added | breakdown: "collaps", "companion", "already exists" | None |
| P-A7 | Hour-long campaigns in one stage | breakdown unit types; 13 `run_in_background` calls in sessions | None: no campaign units; background runs were test suites |
| P-A8 | Work outliving its stage | completion reports; signals | None |
| P-A9 | Unit kept by size check, later split | decisions D7, D10; `afb311c` | Found: `tactical-interior-tier` |
| P-A10 | Superseded figures costing a decision | decisions D5, D6, D9, D16; signals:118, 613 | Found: four "No decision moves" decisions |
| P-A11 | Criteria passable while failing the vision, found after close | acceptance verdicts; `docs/guides/quality-bar.md:140-147`; post-close sessions | None |
| P-A12 | A re-entry repairing an earlier re-entry's defect | git log of the architecture; decisions | No re-entries (22 amendments via re-plan and gate decisions); closest: D14 a second anchor defect after D13 |
| P-A13 | Orphan process after a stage | sessions: "orphan", "still running", `pkill` | None for this stream; one stray test process in another feature's session |
| P-B1 | Significant finding unaddressed, later true or hand-carried | digests; breakdown Plan Maintenance; `docs-close-out-completion.md` | Found: an unfixed Significant ("filing count is wrong at four sites") shipped a wrong figure (`42b4d51`), corrected at `df241b9`; re-plans, not the requester, carried unfixed Significants |
| P-B2 | Close-out audit missing findings | `docs-close-out-plan.md:98`, `:519`; ledger-reference grep | Found: close-out scope excluded the ledger |
| P-B3 | Repeated rows per round | signals | Context-free; rows carry no finding id in cartomancer either |
| P-B4 | Defect found after programme acceptance | post-close sessions (probe a) | None found |
| P-C1 | Load placeholder or truncation | 33 load agents' `planBlock` vs `git show <planSha>:<plan>`; "see tool results" / "persisted-output" greps | 29 byte-identical, 2 altered (not truncated), 4 no return (session limit or interrupt); 0 placeholder hits |
| P-C2 | Ledger overwrite, deletion, misfiling | `git log --numstat` of the signals file; `## Wave N` heading vs Wave column; 27 append prompts vs commits | 42 commits, 0 deleted lines; 76 rows misfiled: 64 by a dropped heading (`cdf14ff`), 12 because the script's prompt carried a heading that did not match its rows (`902a9f8`, `af2ad55`) |
| P-C3 | Copy stage dropping text | ~113 completion-report agent prompts | Not testable: the stage authored reports (sonnet/opus), with no finished text in the prompt. The haiku copy stages that existed dropped text (P-C2, P-C4) |
| P-C4 | Malformed decision log stopping load | signals:1161; git log -p of the decisions file; decision-log agent edits | Found: haiku decision-log `Edit`s removed the closing fence (`df7cd8c`, agent in run `wf_fc673c17-268`); the load stopped (session `f4f02e4d`, 2026-09-15T03:14:44Z); fixed `09bb115`; recurred at `89c7187`, fixed `75c8f50`. 5 of 6 decision texts verbatim; D18 shortened |
| P-C5 | `git branch -d` denied | workflow-agent tool results | None: 36 `branch -d` calls in 4 agents, none denied |
| P-C6 | Agent editing settings after a denial | workflow-agent `Write`/`Edit` of `settings*.json` | None |
| P-C7 | `agent-null` stops | run JSONs; API-error entries | 17 API errors, all session-limit messages, not 529s |
| P-C8 | Denied commit reported as success | `git commit` results in 86 haiku agents; returned SHAs vs `git cat-file` | None: no haiku commit denied. Commit denials (24, sonnet/opus agents isolated in worktrees) were retried; one returned SHA does not resolve. 3 haiku ledger agents fabricated SHA tails |
| P-C9 | `-d` denied while `--force` allowed | workflow-agent tool results | None: both ran without denial (wf_58847e5b-beb, wf_ea1ae00c-488) |
| P-D1 | Pin staleness | signals architecture-invalidated rows 618, 878, 1137, 1435 | All four were design invalidations, not stale pins |
| P-D2 | Gate results after fix | signals fix rows | Context-free; not separately probed |
| P-D3 | Backgrounded gates returning unrun | workflow-agent tool results "Command running in background" | 20 backgrounded commands in 15 agents; each ID referenced again (polled); none found returning unrun |
| P-D4 | Load overlap stops | signals:13-17 | Found: five `unserialised-overlap` stops at wave 0 (`wf_6fa27665-839`) |
| P-D5 | Records calling regenerable data must-keep | carto docs: `wf_` paths with keep/retain/preserve | None |
| P-D6 | Reset of a reused worktree | workflow-agent prompts | The reset prompt appears in 227 agents; no interrupted-attempt resume identified |
| P-E1 | Mechanics share of driving input | typed turns and AskUserQuestion calls in 18 driving sessions | Found: ~24 launch/resume, ~9 pause/status, ~6 substantive typed turns; 14 AskUserQuestion, 3 mechanics. `lsf-drive` existed only from 09-19 and was invoked in none |
| P-E2 | Options omitting the decisive consequence | `/model` and model-name greps over user turns; AskUserQuestion options | None for the model hand-off; every driving session from 09-09 23:53 ran sonnet. One partial omission (session `771809f3`) |
| P-E3 | Recommendation without basis | AskUserQuestion options | Found (weak, n = 1) |
| P-E4 | Options inside a failing frame | session `4e0cd888`; run `wf_5fcdc7c1-e5b` | Found: a 3-minute run with zero implementation stopped on the same decision; also hand salvage outside the loop (sessions `3a4bdf06`, `771809f3`) |
| P-E5 | Pre-clearable gates costing a stop | session `7bfcdbeb`; plan YAML gate kinds | Found |
| P-E6 | Pasted handoff prompts | launch-prompt sizes in 12 sessions | Found: 407–1586 chars; later sessions resumed from a 21 KB memory file with 15-char prompts |
| P-E7 | False handoff state | sessions `f9da76de`, `19236536`, `4e0cd888`, `5292ac3d` | Found: three cases, including a relaunch prompt carrying a decision id already taken (renumbered D8, `e583692`). No deleted-worktree or stale-URL case |
| P-E8 | Decisions only in launch args | launch args vs decision log | None: D8 and D18 entered by args but were committed by the run |
| P-E9 | Misattributed run ending | recovery statements vs run JSONs | Found (partial: run state misstated, cause unestablished, not written to memory) |
| P-F1 | Relay in workflow-agent first prompts | first user turn "[Workflow harness — user request]" | Found in 3 of 18 runs: "Resume the work" (`wf_e50c9431-6d1`), "yes" (`wf_58847e5b-beb`), "Yes, continue. Then is there anything we need to capture before clearing context?" (`wf_ea1ae00c-488`); no Skill invocation of lsf skills in any workflow agent |
| P-G1 | Phase 2 critique gate reversed later | decisions D2 | Not testable: requester declined a Phase 2 review |
| P-G4 | Adversary-gated re-entries | — | Not testable: none |
| P-G2 | Phase 1 instrument first run in Phase 3 | decisions D2–D5; brief §3.7 | None: the first wave-1 review gate forced amendments, but no Phase 1 instrument existed |
| P-G3 | Stop kinds first met in a live programme | signals ledger stop rows | Found: `invalid-plan`, `unserialised-overlap`, `claimed-disjoint-conflict`, `merge-conflict-unresolvable`, `no-ready-unit`, `signal-volume`, `referent-recurrence`, `execution-distress`, all met while delivering |
| P-I1 | Out-of-plan change requested during a live run | typed turns while a run was live | None: live-run requests were status or pause; out-of-plan fixes (test time, session `f07c9a8d`; phronithm script fixes) came between runs |
