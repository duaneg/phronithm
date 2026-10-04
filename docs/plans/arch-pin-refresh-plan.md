# arch-pin-refresh — problem statement, design and plan

Work unit `arch-pin-refresh` of `docs/plans/lsf-improvements-breakdown.md` (plan @ `0cacac9`). Fixes phronithm#2 (register row P-D1). Base commit `0cacac9`, branch `lsf-workflow`. Phases 1–3 ran in compressed mode. After approval this document is saved as `docs/plans/arch-pin-refresh-plan.md`.

## Context

`ctx.architectureSha` is set once at load (`workflows/lsf/implement.js:1019`) and never moves. In archipeligo run #8 (`wf_f97d8e41-bd7`), the design unit `abi-disclosure-design` amended §5 and §7.2 within its own scope (`522ed9b`). The wave gate then judged the merged text against the load-time pin (`b77c21e`) and raised `architecture-invalidated`, proposing the text that was already on main. The digest also showed `architectureAmendments: []`, because only the re-plan's `document-text` verdicts produce that kind. The register classes P-D1 as a context-free defect. The cartomancer-v2 probe found no stale-pin stop there: its four invalidations were real design invalidations. That count is not evidence against the defect, because no cartomancer unit amended the architecture document.

## Phase 1 — problem statement

### Unit entry, verbatim

```
### arch-pin-refresh
- **Type**: bug-fix
- **Risk**: medium — touches what the wave gate judges against
- **Clarity**: exploratory — one open decision: how a unit-carried architecture change reaches the gate without becoming its baseline
- **Iteration**: write-once
- **Effort**: medium
- **Scope**: `workflows/lsf/implement.js`, `workflows/lsf/test/implement.test.js`, `workflows/lsf-implement.js`, `docs/design/lsf-workflow.md::wave-loop`
- **Publishes**: none
- **Depends on**: none
- **External gates**: `node` (environment, v26.9.0)
- **Done condition**: A test shows that after a wave merges a unit that amends the architecture document, later stages are judged against the amended text, and that a unit-carried change to a decision still reaches the wave gate as a candidate invalidation. The tests pass, the built script is not stale, and phronithm#2 is closed by the commit.
- **Description**: Postmortem §D "Architecture pin". A context-free script defect: the pin is set once at load.
```

Standing obligations naming the unit:
- **Register dispositions.** The completion report names P-D1 and its disposition.
- **Design of record.** Amend `lsf-workflow.md::wave-loop` in the same commit as the behaviour change.

### Success criteria (target rigour)

- **S1** (done condition, clause 1), target `proven`. Once a wave merges a unit that amends the architecture document, every later stage is told the amended SHA: that wave's gate, its re-plan, and the next wave's unit stages. A test in `implement.test.js` shows this.
- **S2** (done condition, clause 2). This criterion has two halves. The script half, target `proven`: when the document moved, the gate prompt sets the pre-wave text as the baseline for each merged unit's own diff, and names a decision-moving change as an invalidation; a gate invalidation still stops the run. The gate half, target `evidenced` and achievable here only as `conjectured`: the live gate follows that instruction. The gate is an LLM stage and the stub can't show it does. The upgrade path is the `shakedown-fixture` unit, whose fixture includes a unit that amends the architecture document. The done condition says "a test shows … reaches the wave gate as a candidate invalidation", and the script half meets that as written.
- **S3**, target `proven`. Changes a unit makes that move no decision become `architecture-amendment` records carrying the unit, and they appear in the digest. They do not stop the run. This was the issue's second ask.
- **S4**, target `proven`. Reloads after a re-plan or termination refresh the pin from the copy stage, which picks up `document-text` corrections. A wave whose document did not move leaves the gate prompt as it is today.
- **S5** (done condition, clause 2), target `proven`. `cd workflows/lsf && node --test test/` passes, and `build.test.js` shows the built script is not stale.
- **S6** (done condition, clause 3), target `evidenced`. The behaviour commit carries `Closes #2`. The requester decided on 2026-10-05: trailer only, with no `gh` action. The issue closes when `lsf-workflow` merges to main. The criterion can't be automated because closure depends on a future merge. Record this as a deviation in the completion report.

### Scope and constraints

- Writes outside the Scope line, recorded in the completion report: `scheduling.js` (`signalsFromGate`), `schema.js` (comment on the `architecture-amendment` kind), and `test/scheduling.test.js`. Every later writer of these files depends on this unit through the serial script chain, so there is no overlap risk.
- `args.architectureSha` keeps its meaning as the load-time pin when given. `phronithm:lsf-drive` doesn't pass it.
- The script has no shell. Every SHA it uses comes from an agent return.

### Candidate approaches

1. **The gate judges the wave's architecture diff.** This is the improvements note §6.1 option, adapted. The merge stage returns the document's SHA after its commits. If the SHA moved, the gate reads the document at the new SHA and judges the unit-made hunks of the diff against the pre-wave text. The pin advances after the gate, and reloads refresh it. This is the selected approach.
2. **Units may not amend the architecture document.** The pin stays fixed, and a unit reports an architecture change as a discovery for the re-plan's `document-text`. Rejected: design units exist to settle decisions the document leaves open, and breakdown's scope conventions would have to change.
3. **Advance the pin before the gate, with no diff.** Rejected: a unit's change to a decision becomes the gate's baseline, which fails clause 2.
4. **Do nothing and keep the manual `args.architectureSha` workaround.** Rejected: it costs a requester action per launch, and a pin can't be moved mid-run.

### Exploration findings

- `implement.js`: `header()` (:285) and `gatePrompt` (:532) both embed `ctx.architectureSha`. `copyPrompt` (:345) already returns `architectureSha` via `git rev-list -1 <branch> -- <doc>` on every load. `loadPlan` (:1002) is called at load, after a re-plan (:1159) and after termination (:1063), but only the first copy's SHA is used. `MERGE_SCHEMA` (:214) is built from `MERGE_RESULT_SCHEMA`. The gate runs only when `merged.length` (:1113). A superseding stop returns before the re-plan (:1131).
- `scheduling.js`: `signalsFromGate` (:296) maps the gate result to signals. `digest` (:450) collects `architecture-amendment` statements. `REPLAN_ITEM_KINDS` doesn't include `architecture-amendment`.
- Test harness: `runtime()` stubs dispatch by label prefix. The `merge` stub returns `merges` and `planSha`. `copied()` returns `architectureSha: 'arch1'`.

## Phase 2 — design

**Merge stage.** `mergePrompt` adds: "After your last commit, return `architectureSha`: the output of `git rev-list -1 ${integrationBranch} -- ${architectureDoc}`." This is the same command and wording as the copy stage. `MERGE_SCHEMA` gains `architectureSha` as a required field.

**Pin advance (wave loop).** Immediately after the merge returns:
```js
const prevArch = ctx.architectureSha;
const reported = m?.result.architectureSha;
if (typeof reported === 'string' && reported.trim()) ctx.architectureSha = reported.trim();
```
This happens before the gate prompt is built. The gate's header, the re-plan and every later stage therefore name the merged text, which avoids the two-SHA conflict critique finding 2 raised. `gatePrompt(ctx, plan, wave, merged, prevArch)` receives the pre-wave pin as the baseline for unit changes. If the merge return is null, empty or not a string, the pin stays put. When nothing merged there is no gate, and the pin advances to what the branch holds: any change then is the requester's own, and the requester is the authority. When a gate return is null, the run stops on `agent-null`. A relaunch then pins the branch tip, so an unjudged unit change becomes the baseline. That was true before this change too, and the design paragraph states it; the stop is the record.

**Merged-unit attribution.** `merged` entries carry the unit's merge SHA (`x.sha` from the merge return) and type. Per-merge diffs attribute exactly, which critique finding 1 called for: a unit's change is `git diff <mergeSha>^1 <mergeSha> -- <doc>`, the merge commit against its first parent, so requester commits drop out by construction. A clean or resolved merge reported without a SHA falls back to `git diff ${prev} ${current} -- ${doc}` for that unit, and the prompt says the attribution there is the gate's own.

**Gate prompt when `ctx.architectureSha !== prevArch`.** The existing question is asked of the document @ current, as the header states. One paragraph is added, with one rule of precedence:

> The architecture document changed during this wave, from ${prev} to the SHA above. Each merged unit's own change to it is `git diff <merge>^1 <merge> -- ${doc}` (listed per unit below); an empty diff means the unit did not touch it, and changes outside these diffs are the requester's own and not judged. Judge each unit's change against the document @ ${prev}, the text the wave started from, reading that unit's completion report first. A change that records the decision a design unit was charged to settle, that settles a question the document left open to the unit that made it, or that corrects wording, goes under `amendments`: the unit, the section, one clause on what changed. A change that alters what a section decided @ ${prev} beyond that is an invalidation: `decision` is the text @ ${prev}, `evidence` the merge SHA, `amendment` the text the unit wrote. Report a change once: an invalidation for a unit's change is not reported again as a premise invalidation of the amended text.
> <per unit: `- <unit> (<type>): git diff <sha>^1 <sha> -- <doc>`>

When the SHAs are equal, the prompt is unchanged, and the gate returns `amendments: []`. A requester-only mid-wave edit moves the SHA, but every per-unit diff is empty, so no change is judged (critique finding 11).

**Gate schema and signals.** `GATE_SCHEMA` gains `amendments: arr(obj({ unit, section, statement }))`. `signalsFromGate` maps each entry to `signal({ kind: 'architecture-amendment', source: 'gate', stage: 'review-gate', wave, unit, statement: \`${unit}: ${statement}\`, referent: section })`. These are records, not re-plan items. Notes still carry anything the re-plan must act on. The comment on the `architecture-amendment` kind in `schema.js` names both producers.

**Reload refresh.** A helper `refreshPin(copy)` sets `ctx.architectureSha` from a non-empty `copy.result.architectureSha`. The re-plan reload and the termination reload call it. The first load keeps `ctx.architectureSha ?? copy`.

**Header comment** (`implement.js:25`): `architectureSha` is the load-time pin, defaulting to the document's last commit on the integration branch. It advances after each wave's gate and on each reload.

**Design of record.** Add a paragraph after "The wave gate asks one question" in `lsf-workflow.md` Wave loop: "The architecture pin follows the integration branch." It summarises the behaviour above, including the null-gate relaunch caveat, and notes that `architecture-amendment` now has two producers: the gate for unit changes, and the re-plan for `document-text`. The pseudo-code's gate line notes the per-unit diffs. `lsf-drive` needs no change: `SKILL.md:114-115` presents the amendment field and the digest's amendments without naming a producer (checked).

### Risks and unknowns

- **Gate judgement.** Whether a design unit's settled decision is classed as an amendment depends on the gate reading the unit's completion report. A misclassification is a superseding stop, which costs the requester a Phase 2 re-entry: this is the run-#8 failure again. The per-merge diffs and the unit type in the prompt reduce the risk but can't remove it. It stays `conjectured` until `shakedown-fixture` runs.
- **Missing merge SHA.** `merges[].sha` is optional in `MERGE_RESULT_SCHEMA`. When it is missing, the prompt falls back to the whole-wave diff for that unit, with the gate attributing changes itself. Making the field required belongs with the SHA-integrity units, not this one.
- **Fabricated SHAs.** The merge stage could return a made-up SHA (P-C8). The gate's `git diff` would then fail, and the gate would report that in a note. Validating SHAs belongs to `mechanical-stage-integrity`; this unit only notes it.
- **Decision re-plan before load.** When `args.architectureSha` is given and the decision re-plan makes a `document-text` correction, the first load's pin is stale until the wave-1 merge. `phronithm:lsf-drive` never passes the arg. Accepted and recorded as a reading.

### Critique record (round 1, critique-design, sonnet): passed with no Critical findings

| # | Severity | Disposition |
|---|---|---|
| 1 | Significant: `git log` attribution unreliable | Addressed: per-merge diffs |
| 2 | Significant: two SHAs in the gate prompt | Addressed: pin advances before the gate prompt; baseline passed separately |
| 3 | Significant: design-unit settlement indistinguishable from a decision change | Addressed in part: unit type and completion report go to the gate. Residual risk recorded |
| 4 | Significant: S2 rigour overstated | Addressed: S2 split into script half (`proven`) and gate half (`conjectured`), with an upgrade path |
| 5 | Significant: null gate leaves an unjudged baseline | Documented in the design paragraph; already true before this change |
| 6 | Minor: no termination-reload test | Addressed: test (e) |
| 7 | Minor: stubs omit `architectureSha`, `.trim()` on a non-string | Addressed: string guard, test (d). The harness does not validate schemas |
| 8 | Minor: an args pin spans earlier commits | Removed by the per-merge diffs |
| 9 | Minor: cost of a spurious stop understated | Addressed: risk text restated |
| 10 | Minor: two questions with no precedence | Addressed: report-once rule |
| 11 | Minor: a requester mid-wave edit triggers the paragraph | Addressed: per-unit diffs are empty |
| 12 | Minor: downstream docs | Checked: `lsf-drive` unaffected; the design paragraph names both producers |

## Phase 3 — plan

### Step 1 — gate amendments become records (`scheduling.js`, `schema.js`, `test/scheduling.test.js`)
Extend `signalsFromGate` to map `amendments`, and update the kind comment. Test: one gate return with an invalidation, a note and an amendment. The amendment yields an `architecture-amendment` record with `source: 'gate'`, the unit, and the section as referent. `evaluateWave` stops only on the invalidation, and `digest().attention.architectureAmendments` lists `unit: statement`. Acceptance: scheduling tests green. Commit.

### Step 2 — pin follows the branch (`implement.js`, `test/implement.test.js`, `lsf-workflow.md::wave-loop`, built script)
Make the merge prompt and schema change, the gate prompt and schema change, the pin advance, `refreshPin` at both reloads, and the header comment. Add the design paragraph. Update the default `merge` stub to return `architectureSha: 'arch1'`. New tests:
- (a) S1/S2: the wave-1 merge returns `architectureSha: 'arch2'` and merge SHA `m1` for unit A. The `review-gate:w1` prompt carries the header `Architecture document: docs/architecture/f.md @ arch2` and the baseline `arch1`. It also carries `- A (foundation): git diff m1^1 m1 -- docs/architecture/f.md` and the invalidation and amendment instructions. The `plan:B` and `implement:B` prompts carry `@ arch2` and not `@ arch1`.
- (b) S2: in the same setup, a gate invalidation stops `architecture-invalidated`, and its escalation has `plan_sha` equal to the merge's.
- (c) S3: a gate `amendments` entry produces a recorded `architecture-amendment` whose `unit` is A and whose statement starts `A: `, and the digest lists it. The run completes.
- (d) S4: a merge returning `arch1` (unchanged) leaves the gate prompt without the paragraph. A merge returning `''`, or no field, leaves the pin at `arch1`.
- (e) S4: a re-plan reload (`load:w1`) returning `arch3` puts `@ arch3` in wave 2's prompts. A termination reload (`load:termination`) returning `arch4` puts `@ arch4` in the obligations prompt.
- (f) Fallback: a merge entry with no `sha` gets the `git diff <prev> <current>` line for that unit. `args.architectureSha: 'argpin'` is the load-time pin and the wave-1 baseline.

Run `node workflows/lsf/build.js`, then `cd workflows/lsf && node --test test/`. Acceptance: all green and the build is not stale. Commit with `Closes #2` in the message; the design-of-record amendment goes in the same commit.

### Step 3 — Phase 5 and completion report
Phase 5 runs `phronithm:review` plus a full-diff critique gate, both in subagents. Then write the completion report at `docs/plans/arch-pin-refresh-completion.md` and commit it. It gives P-D1 the disposition `fixed` with the step 2 commit, lists the writes outside Scope and the S6 deviation, and confirms the `node` gate re-probe. The steps are sequential: step 2's tests rely on step 1's signal mapping.

### Verification
`cd /home/duaneg/src/phronithm/workflows/lsf && node --test test/` must be green, including `build.test.js` (staleness). The done-condition clauses map to tests (a)+(e) for clause 1 and (a)+(b) for clause 2; S6 is checked from the commit message.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/arch-pin-refresh-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit arch-pin-refresh of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim: see "Unit entry, verbatim" above (Phase 4 re-verifies the external gates against these
pins before implementing, recording the outcome in the implementation-summary). External gate: `node` (environment, v26.9.0).
Standing obligations naming this unit: Register dispositions (P-D1); Design of record (`lsf-workflow.md::wave-loop`, same commit).
After Phase 5, produce the completion-report (contents per the
phronithm:feature skill's Produces) at docs/plans/arch-pin-refresh-completion.md,
then present this continuation prompt to the requester:
Run `/phronithm:breakdown` in re-plan mode with docs/plans/lsf-improvements-breakdown.md and docs/plans/arch-pin-refresh-completion.md before starting any dependent unit.
