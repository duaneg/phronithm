# LSF postmortem: archipeligo foundation programme

The process postmortem of `phronithm:large-scale-feature` as used to deliver archipeligo's foundation programme: charter 2026-08-22, Phase 3 complete 2026-10-02 (58 work units, 5 termination units). It examines lsf as a delivery method, not archipeligo's engine. Proposals are in [lsf-improvements](lsf-improvements.md) §2–§9; this document holds the evidence.

SHAs are prefixed with their repository: `arch@` for archipeligo, `phr@` for phronithm. Line numbers in `implement.js`, `scheduling.js` and `lsf-workflow.md` are at `phr@1e3dcec`.

## Evidence base

- archipeligo's git history, Plan Maintenance log (`docs/plans/foundation-breakdown.md`, and the pre-migration log at `arch@94d3ee9`), 21 signal ledgers (`docs/plans/foundation-signals/`), 8 reconciliation records, the decision log, and the close-out record (`docs/plans/foundation-close-out.md`).
- 28 `lsf-implement` launches, 2026-09-19 to 10-02, in 31 run directories (three runs outlived a `/clear` and span two session directories): run JSONs, journals and 665 agent transcripts, tabulated mechanically.
- 42 session transcripts from 2026-09-19 on. Earlier transcripts were deleted under the 30-day retention default, so Phases 1–2 and the pre-Workflow Phase 3 (08-23 to 09-08, 10 units) rest on commits and the plan log.
- phronithm's lsf commits since 2026-08-15, to separate fixed faults from open ones.

Wall-clock gaps below include requester absence and are upper bounds on process cost. Agent-hours are first-to-last transcript timestamp and include waits on long commands.

## Headline numbers

| Measure | Value |
|---|---|
| Phase 3 launches | 28; 27 ended without `complete` |
| Endings by class | requester or organisational gate 8, architecture-invalidated 4, structural 5, tooling fault 7, requester-initiated 4 (one ending counted in two classes) |
| Phase 2 re-entries | 8, each after a stop on a measurement, tuning or gate unit; about 92 h stop-to-relaunch (upper bound) |
| Run time vs window | runs live 91.7 h of a 305.6 h window |
| Agent-hours | 104.5; implement 62.1, of which measurement and campaign units 39.7; merge 13.6 |
| Review yield (ledger rows, all rounds) | critique-plan 82 Significant; code review 63 Significant (critique-code 29, lens-review 22, critique-design 12). Rows repeat across rounds, so distinct findings are fewer |
| Haiku mechanical stages | at least 14 faults in 218 invocations |
| Data lost | 83 GB of run series, deleted by merge cleanup |
| Units after the original plan | 38 added and 1 retired (21 → 58); 20 of the additions descend from the two empirical gate units |

Throughput rose with the Workflow: 48 units in 13 days, against 10 units in 17 days for the serial interactive loop before it.

## Findings

Ordered by cost. Each names its proposal.

### A. Empirical work has no first-class shape (§3, §4)

The dominant cost. Every Phase 2 re-entry followed a stop on a measurement, tuning or gate unit, and in each the architecture had committed a claim with no test design or fail branch. Three kinds recur:

- **Gate designs whose defect arithmetic shows.** A 3-seed gate certified only π ≥ 0.37 (re-entry R3, `arch@4a75e94`). Its replacement, 41 seeds at j = 0 certifying π ≥ 0.928, needed a cell with π ≈ 0.995 to pass with the 0.8 probability the requester intended (0.995⁴¹ ≈ 0.81; R4, `arch@0569c92`). A per-seed stage-2 rule passed with P ≈ 0.002 at the measured rate (R5, `arch@a01fe05`). None needed a run to find; each was found after a stop.
- **Evidence measured on another system.** Phase 1's "118 of 132 cells persistent" (`arch@a169bae`) used the classifier the brief itself called invalid, on the pre-tissue engine. Architecture §13 cited it, and R1 invalidated it.
- **An undefined criterion.** Brief criterion 3's "persists" had no horizon until 10-01. Run #16's return flagged it as "still a requester matter" two days before it stopped run #22.

The other re-entries (R1, R2, R6, R8) needed runs to find, but would have been planned branches rather than stops had the plan held a fail branch. R4, R5 and R6 each repaired a defect an earlier re-entry introduced.

The plan saw the risk: `stage1-gate` at `arch@dc88280` was "Risk: high — first evaluation of the new regime; nobody has run it", yet "Clarity: specified", with no seed count, confidence level or fallback. Breakdown collapsed its companion tuning-infrastructure unit because a harness existed, though the harness searched one parameter at a time with no stated confidence; the companion units were added later anyway (`arch@ec1ec02`, `arch@6921509`).

In Phase 3 the same shape recurs at unit level:

- `breakdown` has no measurement type, so campaigns were typed `feature` or `integration` and run in one single-shot implement stage. 18 such stages took 39.7 agent-hours, the longest 9.1 h on one core of eight.
- stage2-horizon-confirm's 80 runs outlived its implement stage. Review found "the record has no results", the run stopped on `gate-non-convergence`, and an orphan process the workflow had no record of kept running until the session relaunched with D12.
- Unit size is judged by open decisions and context load, not by outcome uncertainty or independent deliverables. The requester split weighted-contest-rung by eye ("That sounds like too much for a single work item"). Its successor passed breakdown's size check in the decision re-plan and was still over budget, so R7 split it again.

Measured values also accumulated in the architecture document. R1 and R2 invalidated §13 evidence paragraphs rather than decisions, and plausibly needed only a decision re-plan. The other invalidations, including R8 (an in-sample threshold rule and a false §3.2 claim), changed rules and needed Phase 2.

A separate fact belongs here because a criterion change is easy to blame for it. The post-close vision review found the certified world frozen in 22 of 40 runs (`arch:docs/plans/foundation-vision-review.md` §2), and B.1 traced the freeze to the predators' eat-first policy (`arch@d72e998`). Criterion 3 was redefined six times and relaxed three, but neither the original criterion nor any relaxation measured dynamics, so no version of the gate could have caught it. Phase 1 wrote a criterion that did not test the vision.

### B. Significant findings have no consumer (§5)

Review is cheap: critique-plan ran 60 times in 1.0 agent-hour. The loop then drops what it finds below Critical:

- The fix stage receives only Critical findings (`lsf-workflow.md:172`, "recorded, not fixed").
- The implement stage never sees the plan critique.
- The wave gate is told not to re-evaluate review findings (`implement.js:538`).
- The re-plan's item kinds exclude them (`scheduling.js:369`, `REPLAN_ITEM_KINDS`).
- No stage reads the ledger, by design (`lsf-workflow.md:449`); the digest and lsf-drive show counts.

Consequences:

- Findings came true unaddressed. stage2-gate's plan critique named both the fallback its implement stage took before stopping, and the rule the requester later replaced. A bid-era-rung code-review Significant (the §3.2 draw-ceiling claim) reached the gate as a note; the requester ruled it an invalidation (D18).
- The requester patched the gap by hand twice mid-run (D11's carried finding, and D19 carrying every matter flagged).
- Close-out needed a separate audit. It counted 37 rows from completion reports and declared the pre-2026-09-20 units out of scope. The ledgers hold 26 more review-stage rows on units the audit did not see, about 18 of them pre-09-20 and 7 hidden by a copy-stage fault (§C), plus 82 plan-stage rows. Rows repeat across rounds (release-assert-sweep's 7 rows are about 2 findings), so the distinct count is lower. Two unaudited findings are still on archipeligo `main`.

The precision of Significant findings is unmeasured; the case above is two predictions that came true plus the audit's 7 open defects.

lsf has no programme-level close-out. Its per-feature retrospective (`phr@fcabec2`) never covered a programme. The requester had to ask for the vision review after accepting the final digest.

### C. Mechanical stages and permissions (§2)

The load-stage echo (§2) is one of at least 14 faults in 218 haiku invocations:

| Stage | Runs | Faults |
|---|---|---|
| load | 59 | 2: plan block replaced by a placeholder, and truncated |
| ledger append | 62 | 5: waves 0–2 overwritten (`arch@daf9289`, restored at `arch@363a312`); D9's whole Wave 0 section and the append-only header deleted (`arch@1250655`, never restored); a prior row deleted (`arch@5a15fd1`); a denied commit reported with HEAD as its SHA, so wave-3 rows landed under "wave 4" (`arch@4953b1a`); a denied commit reported as `""` (now a stop, `phr@1e3dcec`) |
| completion-report copy | 47 checked | 2: release-assert-sweep's "7 Significant … left unfixed" line dropped (`arch@8100213`), hiding them from the close-out audit; a phrase dropped from tuning-search-instrument's report |
| decision log | 13 | 1: an invented `clears` entry (`arch@87f88ae`), which stopped the next load |
| cleanup | 37 | 3: `git branch -d` denied three times, leaving branches for the requester; in one, the agent tried to `Write` `.claude/settings.json` to allow itself `git worktree remove` and `git branch -d`, and was denied as self-modification |
| relay (§F) | — | 1+: a cleanup agent, given the pasted launch prompt, invoked `phronithm:lsf-drive` and searched for the Workflow tool, an attempted nested launch |

Meanwhile `git worktree remove --force` was allowed and deleted 83 GB (§D).

Proposals §2.1 and §2.2 cover the load echo only. The ledger, copy, decision-log and cleanup faults above are outside them; they are the case for the host-side driver (§2.3).

Two adjacent faults in agent-call handling: a transient API 529 returned `agent-null` and stopped run #2 (`implement.js:641`, `:669` raise on the first null); and the decision log is transcribed by an agent though the script holds the decision objects.

### D. Workflow state that goes stale or is destroyed (§6)

- **Architecture pin.** `ctx.architectureSha` is set once at load (`implement.js:1019`). A unit that legitimately amended the architecture was then judged against the old text: run #8 stopped `architecture-invalidated` on an amendment already on main at `arch@522ed9b`, and the requester made five commits by hand. Filed as phronithm#2, still open. Later launches carried a manual pin in their startup prompts.
- **Gate results after a fix.** Review rounds receive the implement stage's gate results (`implement.js:759-760`), never the fix stage's re-run. release-assert-sweep spent two fix rounds re-running CI against the same "CI not observed" Critical.
- **Gates that outlive the tool timeout.** Agent Bash calls time out at 600 s and move the command to the background. `xtask ci` exceeds that, so seven agents across code, docs and measurement units (vm-executor, docs-close-out, predation-assert-audit, tui-checkpoint, release-assert-sweep, and stage2-horizon-confirm's implement and fix) ended a turn waiting and were forced to return with gates not run. This is a gate-timeout fault on every unit type, distinct from the campaign case in §A.
- **Plan checks only at load.** Two plans fresh from Phase 2 breakdown stopped at load on `unserialised-overlap` (runs #10, #14). breakdown's overlap check is prose; load's is code; lsf-drive's pre-flight checks only that the plan fence exists.
- **Retained data.** Cleanup force-removes every merged unit worktree (`implement.js:609`); the design says cleanup failure "leaves residue, not risk" (`lsf-workflow.md:266-268`). Untracked `runs/` went with it: 190 series, 83 GB, about 8 h to regenerate. Nothing in the unit entry or implement return names artefacts to keep. Three records now cite an unmerged worktree path that must be kept indefinitely.

### E. The requester interface (§7)

lsf-drive is where the requester meets the loop. In 2026-09-19 to 09-26, about 60% of requester inputs were mechanics (launch, pause, retry, recovery, handoff); in 09-27 to 10-02, 76–90% of Bash calls in pure driving sessions were. Both are hand or keyword classifications.

- **Compressed decisions.** The gate-sizing question offered "Size to 41 seeds (π* ≥ 0.928)" without the pass probability a good cell would have, though its header used that metric against the old gate. lsf-drive already says "Use AskUserQuestion only where the question has bounded options; most stops need the requester's free text" (`skills/lsf-drive/SKILL.md:124`). The rule was not followed. The requester chose 41; a Phase 2 re-entry and a 4.85 h run built on it, and a later rethink showed the design near-unpassable. Twice the requester took a decision out of the loop to a more capable model ("we need more capable and thoughtful eyes on this"; "this needs a more careful and considered rethink"), and both sessions reframed it. Four of the five sessions that framed these decisions ran on sonnet; both rethinks ran on Fable. The design lets the session run "at any model tier" (`lsf-workflow.md:86-90`).
- **A recommendation without its basis.** Once (n = 1), "Keep baseline (Recommended)" was offered after the adversary showed the alternative won every row. The requester asked "Is there a reason to *prefer* A0 over A1 aside from convenience?" They kept A0 with A1 as named fallback, and stage 2 then passed 41/41 on A0.
- **Options inside a failing frame.** Three stage-1 stops each offered options within the same tuned surface. lsf has no route from a stop to an investigation; three investigations ran by hand outside it and settled what the stops could not.
- **Pre-clearable gates.** Three organisational gates needed no in-run evidence (a scope verdict flagged four times, an audit-reading task, criterion 3), yet each cost a stop.
- **Handoff.** Re-entries and relaunches were handed over as pasted prompts up to about 3 KB, until the requester asked for them to go in memory. State in handoffs went unverified: "keep worktree -22" was asserted after cleanup had deleted it; a TUI URL was stale; a recovering session attributed to the requester a kill its predecessor's own TaskStop caused, and the wrong cause entered memory.
- **Launch-argument decisions.** D12 lived only in the args of a run stopped within two minutes; D13 had to restate it.

### F. Harness relay (§8)

The Workflow harness passes the message that triggered the launch turn to every agent as "the only user voice in this task … Where the computed task conflicts with this request, this request wins". 20 of the 28 runs carried one:

- about 4 off-topic: in run #6, two implement agents answered the requester's question about plugin installation instead of implementing, and the wave stopped `done-condition-unevaluable`; a load agent began acting on "Update CLAUDE.md";
- 9 the launch instruction itself; in the five of these launched from a pasted startup prompt, 24 haiku mechanical agents invoked `phronithm:large-scale-feature`, and one cleanup agent went on to `phronithm:lsf-drive` and a Workflow tool search;
- about 7 acknowledgements ("Agreed", "Relaunch").

Runs launched from a tool result (an AskUserQuestion answer or a task notification) carried no relay. A harness report went to Claude Code at the time.

### G. Phase 2 gate discipline (§9)

- The opus adversary ran beside the critic at every re-entry gate but R3. Its confirmation rates were high (Phase 2 15/15, R6 9/9, R1 8/8 confirmed by re-run), and it found R2's one Critical; R5, R7 and R8 had no Critical. R3, gated by a sonnet critic alone, passed the 41-seed design that R4 reversed about 25 h later. Adversary-gated re-entries also introduced defects later reversed (R1, R5), so this is evidence for the opus re-run `critique-gate.md` waits on, not a measurement.
- The R6 static-check instrument existed from Phase 1 but ran as Phase 3's first unit, nine hours after the architecture gate closed. It returned three FAILs; four amendment rounds followed, and two design units were extracted. That, not measured values, drove the architecture's day-1 growth from 63 KB to 90 KB.
- lsf-implement was shaken down on another project's code units (about 19 cartomancer-v2 runs, 09-10 to 09-19). archipeligo was its first plan dominated by measurement units, and the stop taxonomy met those cases there first.

## What worked

- **Architecture-invalidated stops were real.** Apart from the stale pin, all four were accepted and became amendments. Stopping rather than absorbing was right each time.
- **The opus adversary at Phase 2 gates**, with the caveats in §G.
- **Critique-plan**, the cheapest stage; and the Critical-only fix loop with a round cap, which converged in every case but one genuine unmet done condition.
- **Seeded relaunch** (`phr@26733aa`, `args.seed`): recovered the 09-22 kill and the D11 fake conflict, and reused 80 finished runs under D12.
- **Upstream fixes**: the pause ref (`phr@f2073a2`, 09-24, about 38 h after the harmful kill) ended TaskStop pauses; own-files commits (`phr@a86af62`) ended collateral commits; the empty-SHA stop (`phr@1e3dcec`) ended fake conflicts.
- **Per-item re-plan verdicts** (`phr@999c9ff`, `phr@5de8972`) ended the pre-Workflow failure where findings landed only in the plan log, three passes running.
- **Reconciliation records and the decision log with `clears`**: an audit trail complete enough to reconstruct this postmortem.
- **Manual `/clear` at phase boundaries**: no compaction in 14 sessions.

## Project-side items

The postmortem also found archipeligo defects, which belong to archipeligo's tracker, not here: two unaudited Significant findings still on `main`, the unaudited ledger rows, and the lost D9 ledger section.

## Data gaps

- Transcripts before 2026-09-19 are deleted (retention is now 92 days). Phases 1–2 and the first 10 units rest on commits.
- Wall-clock gaps cannot separate requester think-time, investigation and absence.
- The mechanics shares are hand or keyword classifications.
- Significant-finding precision is unmeasured; ledger rows were not deduplicated into findings.
- AskUserQuestion free-text notes were not extracted.
- The cartomancer-v2 runs were identified by citation and directory time only.
