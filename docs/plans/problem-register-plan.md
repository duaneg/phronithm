# problem-register — problem statement, design and plan (compressed mode)

Base commit `f3a395a` (branch `lsf-workflow`). Phases 1–3 run in one plan-mode session; the critique gate runs once on this combined document (Phase 3 step 5). After ExitPlanMode this file is copied to `docs/plans/problem-register-plan.md` and committed.

## 1. Problem statement

### Unit entry (verbatim)

### problem-register
- **Type**: foundation
- **Risk**: medium — the generality classes rest on one comparison project
- **Clarity**: specified
- **Iteration**: write-once
- **Effort**: medium
- **Scope**: `docs/design/lsf-problem-register.md`
- **Publishes**: problem ids; each problem's solution-free statement and generality class
- **Depends on**: none
- **External gates**: `archipeligo-records` (data, `~/src/archipeligo-evolved` @ `c220c62`); `cartomancer-records` (data, `~/src/cartomancer-v2` `docs/plans/quality-bar-interiors-signals.md` and 41 session transcripts from 2026-09-10, retained 92 days)
- **Done condition**: The register is committed with one row per friction item in postmortem §A–§G and improvements §1, and an appendix mapping every postmortem bullet and table row to a register id. Each row has a statement that names no remedy, a cost citation, a generality class (`context-free defect`, `recurs on cartomancer-v2` with a citation, `archipeligo only so far`, or `harness-imposed`), and an empty disposition. The postmortem §D "Retained data" row states the problem as records that call regenerable data (about 8 h to regenerate) must-keep, and the disk that retention costs. It does not state the problem as cleanup destroying data. A further row records that resuming a run resets an interrupted implement attempt's worktree (`implement.js:295`), discarding its commits and untracked files; it is classed `context-free defect` and has no cost citation. `phronithm:critique` (critique-design) passes at the gate bar.
- **Description**: Separate each problem from the remedy the notes attach to it, and test each against cartomancer-v2's 41-unit stream. Later units cite register ids, not improvements-note section numbers.

Decomposition plan: `docs/plans/lsf-improvements-breakdown.md` @ `f3a395a`. Standing obligations naming this unit: none. No architecture document; the postmortem and the plan's Summary stand in.

### Gate probe (planning time)
- `archipeligo-records`: `~/src/archipeligo-evolved` HEAD = `c220c627…` — matches pin.
- `cartomancer-records`: `quality-bar-interiors-signals.md` present (1864 lines); `~/.claude/projects/-home-duaneg-src-cartomancer-v2/` holds 41 `.jsonl` transcripts dated 2026-09-10 to 09-23, plus 18 workflow run JSONs (`*/workflows/wf_*.json`) and subagent transcripts.

### Success criteria
| # | Criterion | Target rigour |
|---|---|---|
| S1 | The unit's done condition, verbatim (above). | evidenced (document properties; no automated check exists for a write-once doc — checked by a scratch script plus the critique gate) |
| S2 | Every non-`context-free`/non-`harness-imposed` row's class is backed by a recorded cartomancer probe (what was searched, where, result), so `archipeligo only so far` is auditable. | evidenced |
| S3 | Ids are unique and match `P-[A-GI]\d+`. | evidenced (scratch script assertion) |

### Exploration findings
- Postmortem: 154 lines; §A–§G hold ~45 bullets and two tables (Headline numbers, §C stage faults). Non-friction sections: Evidence base, What worked, Project-side items, Data gaps.
- `implement.js:294-295`: `worktree(base)` prompt — `git reset --hard ${base} && git clean -fd` at the start of every worktree stage.
- cleanup-data-safety completion (`d2172e1`): requester's restatement of §D Retained data; ~83 GB vs regenerable in ~8 h.
- Cartomancer evidence: signals ledger rows have `Kind` and `Disposition` columns (greppable: e.g. five `unserialised-overlap` stops at wave 0 → recurs for the plan-checks-at-load row); run JSONs have `result`, `error`, `logs`, `summary`.

### Candidate approaches
1. **Probe-per-row (recommended).** Draft rows from the postmortem; for each, define a concrete search signal (ledger `Kind`, run-JSON `error`/stop kind, transcript keyword or tool-error pattern); run the probes mechanically (scripts over JSON/JSONL, parallel data-gathering subagents for transcript reading); classify with a fixed precedence; record every probe in an appendix. Cost: more work; benefit: negative classes are auditable and R3's risk is visible per row.
2. **Read-and-judge.** Read the cartomancer ledger and a sample of transcripts, classify each row by recollection of what was seen, cite only positives. Cheaper, but `archipeligo only so far` is unfalsifiable from the document and depends on sample coverage.

### Requester decisions at scope confirmation (2026-10-04)
- Appendix maps non-friction bullets (Evidence base, What worked, Project-side items, Data gaps) to `—` with a reason; no rows for them.
- Granularity: one row per distinct failure mechanism (~40 rows).
- Every row gets a recorded cartomancer probe; class precedence harness-imposed > context-free defect > recurs on cartomancer-v2 > archipeligo only so far.
- Ids `P-<section><n>` (`P-A1`…, `P-I1` for improvements §1).

- (After plan critique, S-7) Add a non-binding **Topic** column: `mechanical-stage`, `long-running`, `relay`, `driving`, `empirical-design`, `findings`, `state`, `other`. It lets `substrate-options` select rows by id. **This deviates from Publishes** (it is additive); record it in the completion report.

No other deviation from the unit entry's scope or publishes.

## 2. Design

### Selected approach
Probe-per-row (candidate 1), as decided. Read-and-judge rejected: its negative classes cannot be audited, which matters because R3 (generality rests on one project) is the unit's named risk and six design units must state generality from this register.

### Class definitions (written into the register)
- **harness-imposed** — the root cause is a Claude Code / Workflow-tool behaviour (relay; Bash backgrounding after 600 s). Whether lsf could mitigate it does not change the class. The register gives each claim's source, and pins it to the version where the source supports that. A design choice lsf made *because of* a harness constraint (e.g. mechanical stages as agents) is not harness-imposed; its failures are classed on their own evidence.
- **context-free defect** — a defect or gap in lsf's own code or text that inspection shows occurs on any project, independent of workload kind or plan size. Cite the code/design line.
- **recurs on cartomancer-v2** — not context-free, and a cartomancer probe found an instance. Cite it (ledger line, run id, or transcript uuid + timestamp).
- **archipeligo only so far** — the probe found none. When cartomancer could not have shown it (no measurement units, no comparable session), the cell carries a qualifier: `archipeligo only so far (not testable: <reason>)`. That way consumers don't read "untestable" as "doesn't generalise" (critique S-3).
- **Done-condition override:** P-D6 is `context-free defect` because the entry says so. The resume worktree reuse is a harness behaviour (`lsf-workflow.md:57-60`), but the reset at `implement.js:295` is lsf's. The register states the override.

### Statement discipline ("names no remedy")
- A statement records the observed event and its consequence.
- It doesn't name the mechanism the notes propose, and it doesn't describe the friction as that mechanism's absence. For example, it doesn't say "there is no measurement unit type"; it says "campaigns of hours ran inside one single-shot implement stage".
- It doesn't present a postmortem correlation as a cause. P-E2 states the tier correlation as a correlation.
- Quantified cost goes in the Cost column only. The single exception is P-D5, which the done condition requires to name the disk cost.
- A Cost cell carries a quantity, a run id, a SHA or a named event. A row with none says "no cost quantified in the postmortem: <what is recorded>". A bare section letter is not a cost citation.

### Register layout (`docs/design/lsf-problem-register.md`)
1. Two-line purpose; class definitions and precedence; disposition vocabulary (`fixed` + commit, `declined` + reason, `deferred` + successor — from the plan Summary).
2. One table per postmortem section §A–§G plus improvements §1: `| Id | Problem | Cost | Generality | Topic | Disposition |`, Disposition empty. A row with no postmortem locus (P-D6, P-I1) says so in a note under its table.
3. Appendix A — postmortem mapping: `| Line | Item (short) | Id(s) |`, keyed by postmortem line number at `f3a395a`, for every bullet (nested included), table row and §A–§G body paragraph. Non-friction items map to `— (reason)`. Headline rows with no single cost row (launches, endings, run time) also map to `—`, with the ids they aggregate. "Data lost" maps to P-D5, noting that the "lost" framing was not adopted.
4. Appendix B — generality probes: `| Id | Probe | Source | Result |`.

### Provisional row inventory (statements finalised in implementation)
Section-letter costs below (e.g. "§A") are locators for drafting only. Step 4 replaces each with a citation per the statement discipline. Ids are final; row order in the register follows id number.
| Id | Problem (gist) | Cost source | Expected class basis |
|---|---|---|---|
| P-A1 | gate designs with arithmetic-visible defects reached Phase 3 and were found after stops | R3–R5 | probe |
| P-A2 | architecture cited evidence measured on another engine with a disowned instrument | R1, `arch@a169bae` | probe |
| P-A3 | a success criterion had no horizon until late; flagged by run #16, stopped run #22 | §A | probe |
| P-A4 | outcomes that needed runs (R1,R2,R6,R8) arrived as stops and Phase 2 re-entries | 4 of 8 re-entries | probe |
| P-A12 | re-entries R4, R5, R6 each repaired a defect an earlier re-entry introduced | 3 re-entries | probe |
| P-A5 | a unit rated "Risk: high — nobody has run it" was rated "Clarity: specified" and went to Phase 3 unsampled | `arch@dc88280`; R3 | probe |
| P-A6 | a companion tuning unit was collapsed on a harness's existence; re-added later | `arch@ec1ec02`, `arch@6921509` | probe |
| P-A7 | campaigns of hours ran in one single-shot implement stage, one core of eight | 39.7 agent-h | probe |
| P-A8 | a campaign's 80 runs outlived its implement stage; review found no results; run stopped `gate-non-convergence` | stage2-horizon-confirm | probe |
| P-A13 | a campaign process kept running after its stage, unknown to the workflow, until relaunch with D12 | stage2-horizon-confirm | probe |
| P-A9 | units with uncertain outcomes or several independent deliverables passed size checks; requester split by eye; R7 split again | weighted-contest-rung | probe |
| P-A10 | superseded evidence paragraphs went through Phase 2 as invalidations (R1, R2) | §A | probe |
| P-A11 | no version of criterion 3 measured dynamics; certified world frozen in 22/40 runs | vision review, `arch@d72e998` | probe |
| P-B1 | Significant findings went unaddressed in-loop: two came true (stage2-gate's fallback and rule; bid-era-rung's §3.2 claim → D18) | 2 events; D11, D19 hand patches | context-free (`lsf-workflow.md:172`, `:449`, `implement.js:538`, `scheduling.js:369`) |
| P-B2 | close-out audit saw completion-report rows only; ~26 review-stage rows unseen; 2 open on `main` | §B | probe |
| P-B3 | the same finding recurs as several ledger rows across rounds, inflating counts | release-assert-sweep 7 rows ≈ 2 findings | context-free (`implement.js:647-648` stamps stage and round only) |
| P-B4 | the frozen-world defect surfaced only after the requester accepted the final digest and asked for a vision review | vision review, 22/40 frozen | context-free (`skills/large-scale-feature/SKILL.md` Termination) |
| P-C1 | load stage returned a placeholder / truncated plan block → spurious `invalid-plan` | 2/59; impr. §2 runs | probe |
| P-C2 | ledger appends overwrote waves, deleted a section and a row, misfiled wave-3 rows | 4 faults, `arch@daf9289`, `arch@1250655`, `arch@5a15fd1`, `arch@4953b1a` | probe |
| P-C8 | a denied ledger commit was reported as success (HEAD as SHA; once `""`) | `arch@4953b1a`; fake conflict (`phr@1e3dcec`) | probe |
| P-C9 | permissions allowed `git worktree remove --force` while denying `git branch -d` | 3 denials; the 83 GB removal | probe |
| P-C3 | completion-report copy dropped lines (hid 7 Significant) | 2/47 | probe |
| P-C4 | decision-log scribe invented a `clears` entry; the next load stopped | 1/13, `arch@87f88ae` | probe |
| P-C5 | cleanup `git branch -d` denied; branches left for requester | 3/37 | probe |
| P-C6 | a denied agent tried to edit `.claude/settings.json` to permit itself | §C | probe |
| P-C7 | a transient API 529 stopped run #2 as `agent-null` | run #2 | context-free on the first-null raise (`implement.js:641`, `:669`; `scheduling.js:214,223`), not on the 529 |
| P-D1 | architecture pin set once at load; a merged amendment judged as invalidation | run #8, phronithm#2 | context-free (`implement.js:1019`) |
| P-D2 | fix rounds re-ran CI against the same "CI not observed" Critical after the fix had run it | release-assert-sweep, 2 fix rounds | context-free (`implement.js:759-760`) |
| P-D3 | gates longer than 600 s were backgrounded; 7 agents returned with gates not run | 7 agents | harness-imposed |
| P-D4 | plans fresh from breakdown stopped at load on overlap | runs #10, #14 | context-free (breakdown prose vs load code); cartomancer ledger wave 0 recurs |
| P-D5 | three records call regenerable data (~8 h to regenerate) must-keep, citing an unmerged worktree path; keeping it costs 83 GB of disk | 83 GB; 3 records | probe |
| P-D6 | resume resets an interrupted implement attempt's worktree (`implement.js:295`), discarding its commits and untracked files (the same reset is in seven stage prompts: `implement.js:353,381,395,419,431,443,501`; scope per the done condition is implement) | none (per done condition) | context-free (override, see above); no postmortem locus |
| P-E1 | most requester inputs and driving Bash calls were mechanics | 60%; 76–90% | probe |
| P-E2 | decision options omitted the consequence the decision turned on (a good cell's pass probability); twice the requester moved a decision to a more capable model, which reframed it; four of five framing sessions ran on sonnet (correlation) | 41-seed choice → R4, 4.85 h run | probe |
| P-E3 | a recommendation contradicted the adversary's evidence without stating its basis (n=1) | §E | probe |
| P-E4 | repeated stops offered options inside a failing frame; three investigations ran by hand outside lsf | §E | probe |
| P-E5 | gates clearable from evidence already in hand each cost a stop | 3 stops | probe |
| P-E6 | handoffs were pasted prompts up to ~3 KB | §E | probe |
| P-E7 | handoffs asserted state that was false (deleted worktree "keep -22", stale TUI URL) | 2 events | probe |
| P-E9 | a recovering session attributed a run kill to the requester that its predecessor's TaskStop caused; the wrong cause entered memory | 1 event | probe |
| P-E8 | a decision lived only in the args of a stopped run and had to be restated | D12/D13 | probe |
| P-F1 | the launch turn's message is relayed to every agent, ranked above its task | 20/28 runs | harness-imposed |
| P-G1 | the sonnet-critic-only gate at R3 passed the 41-seed design, reversed ~25 h later at R4 | R3→R4 | probe |
| P-G4 | adversary-gated re-entries also introduced defects later reversed (R1, R5) | 2 re-entries | probe |
| P-G2 | a Phase 1 instrument first ran as Phase 3's first unit; 3 FAILs, 4 amendment rounds | 63→90 KB | probe |
| P-G3 | the first measurement-dominated plan met untested stop cases | §G | probe |
| P-I1 | the requester asked for an out-of-plan tooling change (b3sum in `flake.nix`) while a run was live; Phase 3 commit discipline (`skills/lsf-drive/SKILL.md:207-209`) forbids session commits and gives that request no path | no cost quantified: the occasion only (find it in archipeligo) | context-free (`SKILL.md:207-209`); no postmortem locus |

The §C relay row maps to P-F1. §C's "decision log is transcribed by an agent though the script holds the decision objects" maps to P-C4, noted as remedy-flavoured (impr. §2.4). §B "precision … unmeasured", §E "hand or keyword classifications" and the §A–§G intro map to `—` (evidence limits). Topic tags are assigned at step 4.

### Probes (Appendix B source)
- **Mechanical (scripts in scratchpad):** tabulate cartomancer ledger `Kind`/`Stage`/`Disposition`; per run JSON `status`, `error`, stop kind from `result`, agent count, duration; grep workflow-subagent transcripts for `agent-null`/`529`/`overloaded`, Bash backgrounding after 600 s, permission denials and writes to `settings.json`, Skill invocations of `large-scale-feature`/`lsf-drive` (relay), `git clean`/`reset --hard` on reused worktrees; main-session transcripts for AskUserQuestion and pasted startup prompts.
- **Judgement (≤3 parallel Explore subagents, `model: sonnet`, read-only, cite file:line or transcript uuid+timestamp):** (a) §A/§G rows — did cartomancer's plan have measurement/gate/tuning units, criteria without decision rules, Phase 1 instruments, superseded evidence; (b) §E rows — decision presentation, handoffs, recovery attributions in driving sessions; (c) §B/§C/I1 rows the scripts flag but cannot settle (finding repetition, close-out coverage, out-of-plan commits during live runs).

### Risks and unknowns
- The 600 s Bash backgrounding (P-D3) has no source beyond the postmortem. Cite one agent transcript showing it at step 3, and record the Claude Code version if it's visible. The relay (P-F1) is pinned by `relay-probe`, which is a separate unit; the register cites the postmortem observation window.
- The gate's "41 session transcripts" and the Description's "41-unit stream" are separate counts. Both are true at the probe (41 transcripts; the stream closed 41/41 per project memory). Verify the unit count from cartomancer's breakdown at step 3.
- Cartomancer is code-only; most §A/§G rows will class `archipeligo only so far` by construction. Mitigation: the probe log says so per row. Resolution: none possible in this unit (R3 stays open).
- Transcript JSONL format: verify field names on one file before writing greps (during step 2).
- P-I1's class depends on what lsf-drive currently says about session commits — read `skills/lsf-drive/SKILL.md` *Commit discipline* in step 4.
- P-D6: resume reuses the worktree per `lsf-workflow.md:57-60` (critic-verified). Cite it alongside `implement.js:295`.

### API verification
No third-party APIs. Code-line citations (`implement.js:294-295`) verified at `f3a395a`; others re-verified when the row is written.

### Plan critique (round 1, `phronithm:critic` sonnet, critique-design): pass, no Critical
| Finding | Disposition |
|---|---|
| S-1 step 5 checks structure, not the done condition | addressed: added a cost-citation rule, a remedy-lexicon scan, the full P-D5 terms, independent Appendix A enumeration keyed by line number, and a re-run after edits |
| S-2 harness-imposed definition; P-D6 and P-C7 bases | addressed: the class is now defined by root cause, the P-D6 override is stated, the P-C7 basis is the first-null raise, and the P-D3 source is to be cited |
| S-3 "archipeligo only so far" conflates untestable with absent | addressed: added the `not testable` qualifier and a driving-session existence check |
| S-4 gists name a remedy or absence (B1, B4, I1, D2, A5, E2) | addressed: rewritten as events; E2's tier stated as a correlation; the discipline rule on cost placement resolved |
| S-5 conflated mechanisms (A4, C2, A8, E7, G1) | addressed: split into A4/A12, C2/C8, A8/A13, E7/E9 and G1/G4 |
| S-6 omissions and mis-mappings | addressed: new row P-C9; the decision-log sentence, headline rows and section intros now mapped; P-D6 and P-I1 marked as having no postmortem locus; P-D6 names the seven reset sites |
| S-7 no category for `substrate-options` | addressed: Topic column (requester decision; deviation from Publishes) |
| M-1 to M-6 | addressed: step 3 ordering and the null-search record; step 5 re-run after edits; S3 reduced to an assertion; the "10%" figure dropped from P-D5; P-B3, P-B1 and P-B4 citations resolved; `:295` used; the two 41 counts reconciled |
| M-7 context-free covers gaps | addressed: the definition now reads "defect or gap" |

API spec / code block consistency check (Phase 3 step 6): not applicable. The plan has no API spec or code block.

## 3. Plan

Phase 4 entry: re-probe both gates (`git -C ~/src/archipeligo-evolved rev-parse HEAD` = `c220c62…`; cartomancer ledger present, 41 transcripts, retention not lapsed). Record in the implementation summary.

1. **Save planning docs.** Copy this file to `docs/plans/problem-register-plan.md`; commit. *Done:* committed.
2. **Mechanical probes.** Scratch scripts over cartomancer ledger, run JSONs and transcripts (scratchpad only). *Out:* per-row result table in scratchpad. *Done:* every row has a mechanical result or "needs judgement".
3. **Judgement probes.** Spawn ≤3 Explore subagents (sonnet) in one message with the row lists above; they return cited findings, no writes. Sets (a) and (b) can start with step 2; set (c) waits for step 2's "needs judgement" list. First confirm which cartomancer transcripts are driving sessions and whether `lsf-drive` existed then (git log of `skills/lsf-drive`); rows with no comparable session get the `not testable` qualifier. Read cited excerpts myself before classifying. *Done:* every row has a cited instance or a null search recording the file set, patterns and counts.
4. **Write the register.** Rows per inventory (merge/split only with a recorded reason), statements per discipline, costs cited from postmortem/archipeligo (verify the three must-keep records and the b3sum occasion in archipeligo @ `c220c62`), classes per precedence, Appendix A covering every postmortem bullet, table row and §A–§G paragraph, Appendix B. *Done:* S1 structure complete.
5. **Structural check (scratch script `check-register.py`, kept in the scratchpad and re-run after every later edit).** It checks:
   - Every row has six columns.
   - Problem and Generality are non-empty.
   - Cost is non-empty except on P-D6, and is not a bare section reference (`^§[A-G]$`).
   - Generality starts with one of the four class names. `recurs on cartomancer-v2` carries a citation, and the `not testable` qualifier is accepted.
   - Topic is one of the tag set. Disposition is empty.
   - Ids are unique and match `P-[A-GI]\d+`.
   - Appendix A coverage is enumerated independently of the appendix. The script lists postmortem line numbers matching `^\s*- ` or `^\|` (excluding separator and header rows), plus the first line of each §A–§G paragraph, and requires each in Appendix A's Line column. Every row id except P-D6 and P-I1 appears in Appendix A.
   - P-D5 contains "must-keep", "regenerat", "8 h" and "83 GB", and none of destroy/lost/loss/deleted.
   - P-D6 is context-free with an empty or "none" cost.
   - A remedy-lexicon scan of the Problem cells flags extractor, retry, driver, owner, consumer, pin refresh, measurement unit type, fail branch, floor, rule and threshold. Each flag is reviewed by hand and the outcome recorded.

   *Done:* the script passes, every lexicon flag is resolved, and the output is recorded in the implementation summary.
6. **Commit**, then **critique gate**: spawn `phronithm:critic` (`model: sonnet`) with critique-design on the register, against the done condition; iterate per the gate's policy (cap 4 rounds), committing fixes. *Done:* no Critical.
7. **Phase 5.** Run the full-diff review via `phronithm:review` (subagent-separated), then a `phronithm:editor` pass on the register. After any register edit, re-run step 5. If an edit touches a Problem cell, re-run the critique gate as well. Then write the implementation summary `docs/plans/problem-register-implementation.md` and the completion report `docs/plans/problem-register-completion.md`, commit, and present the continuation prompt.

Steps 2 and 3 may run concurrently. Success criteria S1–S3 (section 1) are verified at steps 5–6.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/problem-register-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit problem-register of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim (Phase 4 re-verifies the external gates against these
pins before implementing, recording the outcome in the implementation-summary):
see "Unit entry (verbatim)" in section 1 above. Standing obligations naming this unit: none.
After Phase 5, produce the completion-report (contents per the
phronithm:feature skill's Produces) at docs/plans/problem-register-completion.md,
then present this continuation prompt to the requester:
`/phronithm:breakdown re-plan mode: docs/plans/lsf-improvements-breakdown.md with docs/plans/problem-register-completion.md`
