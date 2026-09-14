# Large-Scale Feature as a Workflow

Design for running the `phronithm:large-scale-feature` lifecycle end-to-end
under the Claude Code Workflow tool: a deterministic script orchestrates a fleet
of agents, holds no judgement, and is restartable from any point. Human
attention is spent only on two things: signals the script can verify
mechanically, and one adjudicated judgement per wave — that the architecture
document no longer stands — delivered in a form the script checks for
completeness and the requester can act on. Never procedural sign-off, and
never a free-text discrepancy an agent chose to file under a heading.

Status: `lsf-implement` exists (`workflows/lsf/implement.js`, built into
`workflows/lsf-implement.js`) and is tested against a stubbed runtime. The
plugin loader registers it as `phronithm:lsf-implement` (confirmed 2026-09-10
against Claude Code 2.1.267: the loader reads `workflows/` non-recursively,
so `workflows/lsf/` is ignored). The signal model below has not yet run
under the Workflow tool. `lsf-brief` and `lsf-architecture` do not exist yet.

## Constraints from the Workflow tool

Documented behaviour this design relies on:

- **No mid-run user input.** Workflow agents have AskUserQuestion and
  EnterPlanMode stripped; only permission prompts pause a run. Sign-off between
  stages means one workflow invocation per stage.
- **Resume replays the longest unchanged prefix of `agent()` calls**, keyed on
  prompt text in call order. The first call whose prompt differs re-runs, and so
  does everything after it.
- **The script has no filesystem, shell, module or clock access.** Durable
  state is what agents commit and what the run journal records.
- **Worktree agents' commits persist.** Probed 2026-09-10: a worktree agent
  runs in `.claude/worktrees/<runId>-<agentIndex>` on branch
  `worktree-<runId>-<agentIndex>`, based on HEAD at spawn; its commits survive
  the run. The branch is subject to the periodic worktree sweep, so the merge
  stage runs in the same workflow, never a later one.
- **On resume, a re-run agent reuses its worktree with prior state intact**
  (same probe). An implement stage must begin by resetting its worktree to the
  integration branch, or a re-plan-triggered re-run inherits the superseded
  attempt.
- **An `args` change alone does not invalidate cached calls** whose prompt text
  is unchanged (same probe). Decisions handed back must therefore appear in
  the prompts of the calls they affect — the plan or decision-log SHA does
  this — not merely in `args`.
- **Plugins ship workflows** from a `workflows/` directory, namespaced
  `phronithm:<meta.name>`.

## Architecture

Two layers.

**Script** — the orchestrator. Plain JavaScript run by the Workflow tool.
Holds the wave loop, the ready-set computation, the stop rule, and nothing
else. All state is agent return values (structured via schema) plus the
commits agents make. Restart = relaunch with `resumeFromRunId`.

**Session** — a presenter and relauncher, any model tier. Launches the
workflow, shows an escalation record when the script returns one, collects the
requester's decision, and relaunches with the decision as an argument. It
holds no loop state and makes no orchestration decisions.

The judgement the lifecycle needs lives in agents, each a fresh context with a
schema-typed return. Every gate the skills describe (critique, review-gate
question, re-plan) is a top-level `agent()` call, so the nesting policy in
[subagent-protocol](${CLAUDE_PLUGIN_ROOT}/docs/subagent-protocol.md) is satisfied structurally — no
agent spawns another.

**The principle the stop rule follows.** The script stops only on a signal
that is mechanically verified, or adjudicated once per wave against a
concrete referent with a required concrete consequence. Agents are not asked
to classify what they notice; free-text discrepancies never reach the stop
rule. They reach the ledger and the re-plan. Measured recall on premise-level
defects at the gate's tier is near zero (Escalation below); per-stage
discovery fields, measured over three runs, yield literal but immaterial
document-versus-code discrepancies restated by every stage that reads the
same documents, and nothing material the wave gate does not also raise.

## Lifecycle mapping

| LSF phase | Where it runs | Notes |
|-----------|---------------|-------|
| Phase 1 steps 1–2a (brainstorm, clarify, constraints) | Interactive session | Genuinely conversational; stays as-is. Output: a committed request record the workflow reads. |
| Phase 1 steps 3–9 (research, synthesis, brief, critique, commit) | Workflow `lsf-brief` | Research fan-out via `pipeline`; brief by a synthesis agent; critique gate as a bounded loop, fixes applied autonomously (see below). |
| Phase 2 (architecture document, critique, doc updates, breakdown) | Workflow `lsf-architecture` | Ends by committing the decomposition plan. |
| Phase 3 (unit loop) | Workflow `lsf-implement` | The wave loop below. |
| Phase termination (meta-tasks, standing obligations) | Tail of `lsf-implement` | Termination stage, see Wave loop. |

**Deliberate divergence from the skill.** LSF Phase 1 step 7 and Phase 2
step 3 require requester sign-off on the fix for each Critical critique
finding. This design drops that: the gate loop applies fixes autonomously, at
the Critical-only bar, in all three workflows. The fix stage applies every
Critical it can repair within the artefact under critique; a Critical it
declines — because the repair would change the unit entry, the architecture
document, or the request record, or because it cannot repair it — is a
structural stop (`gate-non-convergence`, with the finding and the reason in
the escalation record). The requester sees the finding and decides. Every
applied fix is written to the signal ledger (kind `critical-fixed`) so the
requester can review what the loop changed.

Three workflows rather than one so each phase break stays a fresh-context
boundary, which the skill already requires, and so that an escalation in one
phase relaunches only that phase. The session runs them in sequence, pausing
only on an escalation record.

`phronithm:feature` is not invoked inside the workflow — it needs
ExitPlanMode approval, which no workflow agent can obtain. The workflow
carries its own unit plan template and stages instead:

1. **plan** agent: implementation plan from the architecture document and the
   unit entry, in the workflow's own template (Goal, Context, Readings,
   Design, Steps, Verification), returned inline and committed by the same
   agent. The Readings section records every clause of the entry or the
   architecture document that admitted more than one reading, or named
   something that does not exist as named, with the reading taken. The plan
   takes the reading; it does not raise a question.
2. **critique** loop over the plan (Critical-only bar, round number in prompt,
   hard cap per the [critique gate](${CLAUDE_PLUGIN_ROOT}/docs/critique-gate.md)).
   A reading the critic judges wrong is a finding at the severity its
   consequence warrants.
3. **implement** agent, `isolation: 'worktree'`: re-probes the unit's
   environment gates against their plan-time pins (drift is a STOP signal, see
   Stop rule); resets its worktree to the integration branch; executes the
   plan single-shot, with no per-step critique loop; commits on its branch;
   returns branch, path, and the completion-report fields: deviations,
   files written outside the Scope line, deferrals with the receiving unit,
   discoveries with the unit or architecture section they affect, distress
   counts, the done-condition verdict.
4. **review** stage over the branch diff: one critique-code gate at the
   Critical-only bar and lens review per unit type, in parallel. No separate
   static-analysis stage: the project's own gates (linters, type checks) run
   in the implement stage, whose done conditions require them green, and
   again after each merge. The code critique evaluates the done condition under the plan's
   readings; an unmet condition, or a reading the code does not bear out, is
   Critical. Significant and Minor findings are recorded, not fixed.
5. **completion-report** is assembled from the implement and review returns and
   committed by a scribe agent.

**What a unit entry's fields mean to a stage.** Scope is a coordination
declaration: the files this unit writes that another unit in flight might
also write. The scheduler reads it for overlap. It does not limit what the
work may edit; a file the work needs outside the Scope line is changed and
reported, and the re-plan corrects the line where a later unit shares the
file. Every unit-stage prompt states this.

Organisational and release gates cannot be probed by an agent; a unit carrying
one is never ready until the decision log records the gate as cleared, and a
wave with no ready unit for that reason is a STOP.

Each stage is a separate cache entry, so a restart after a crash in review does
not re-run implementation.

## Wave loop

```
if decisions: commit to the decision log; replan(plan, decisions)   // a verdict per decision; cannot stops
plan = load(decomposition-plan @ sha)          // copy agent + parser; stops on any structural defect
loop:
  ready   = units with all dependencies complete and no uncleared organisational/release gate
  if ready is empty and units remain: return escalation(blocked units)
  if ready is empty: break
  results = parallel(ready.map(unit => runUnit(unit, plan.sha)))   // barrier
  merged  = merge(results, in plan execution order)               // agent, sequential
  gate    = invalidations and notes from the wave gate             // agent, one question
  signals = structural(results, merged) ∪ gate
  if any signal is STOP and (nothing merged or the stop supersedes the plan): return escalation(signals, plan.sha)
  plan    = replan(plan, results.reports, merged.conflicts, items, stops)  // agent, commits; a verdict per item
  if any signal is STOP, replan yields STOP, or an item is unabsorbed: return escalation(..., plan.sha)
```

**The script reads only the dependency graph, and reads it from a block.**
The plan carries its graph as a `\`\`\`yaml lsf-plan` block and the decision
log its decisions as a `\`\`\`yaml lsf-decisions` block
([plan graph](${CLAUDE_PLUGIN_ROOT}/docs/plan-graph.md)). `load` is a copy agent
that runs `git show` and returns the blocks verbatim, then `graph.js` parses,
joins clearances onto gates, and validates. The copy agent returns nothing
but the blocks. Every structural defect — unreadable block, unknown or
duplicate unit, bad gate kind, a decision clearing an unknown or environment
gate, a cycle, an overlapping-scope pair with no dependency path — is a STOP.
The same path re-reads the plan after re-plan and termination, from the SHA
those stages return; no stage returns a graph inline.

Breakdown may reconcile overlapping-scope parallel units by a prose
coordination protocol; the workflow does not read prose. Every such protocol
is encoded in the block either as an edge (serialise = an edge from the
earlier unit to the later) or as symbol-level scope (`file::symbol` for each
function or class a unit owns within a shared file). Symbol ownership is a
claim the plan makes; a merge conflict between two units the plan called
disjoint is a re-plan item.

**Termination stage.** When no units remain, a breakdown-mode agent reads the
plan's Meta-Tasks and Standing Obligations sections and returns two lists:
substantial meta-tasks as unit entries with done conditions, and small ones
(documentation updates and the like) as direct edits. The unit entries are
appended to the plan and the loop runs again over them through the full five
stages; the direct edits are made by one scribe agent without the pipeline.
The stage then confirms every standing obligation is discharged; one that is
not is a STOP.

**Wave, not per-completion.** Units in a wave execute against a pinned plan
version; the plan is amended once per wave from all the wave's completion
reports (breakdown's re-plan mode accepts several). Per-completion re-plan
would amend the plan under units still in flight. The barrier also bounds the
cost of an escalation to one wave: in-flight units finish, nothing new starts.

**Merge is an explicit stage**, sequenced after the wave and before re-plan.
It merges each unit's branch into the integration branch in plan execution
order and reports, per unit, clean or conflicting with the files involved. A
conflict between units the plan claimed disjoint, resolved mechanically, is
a re-plan item. A conflict the merge agent cannot resolve mechanically is a
STOP signal, as is a clean merge after which the project's gates fail (the
stage undoes it and reports the unit regressed). Merge order is a function of
the plan, so a re-plan can change the order of later waves; the merge stage
never chooses.

**A wave stop still re-plans over whatever merged.** A relaunch re-plans
over the requester's decisions alone, so the wave's re-plan is the only pass
that absorbs the merged units' reports. The stop signals go to the re-plan as
context it must not amend for; the unit each names stays incomplete. The
re-plan is skipped when nothing merged, and when a stop supersedes the plan
(architecture invalidated, done condition unevaluable — the relaunch target
is `lsf-architecture`).

**The wave gate asks one question**: does anything this wave invalidate the
architecture document — does any decision it makes rest on a premise the
merged code or the completion reports show false. An invalidation is
returned in a fixed form: section, decision, premise, evidence, amendment
(what the section should now say). The script stops only when all five
fields are present; an invalidation missing a field, and every note the gate
returns besides, is recorded and handed to the re-plan. The gate is told not
to re-evaluate done conditions or review findings (the unit stages did, at
the bar this workflow sets) and not to report the workflow's own process.

**Decisions are applied before any unit starts.** A relaunch that carries
requester decisions commits them to the decision log, then runs a re-plan
over the plan with the decisions as input; each decision gets a verdict
(amended, no change, document-text, cannot), and `cannot` or none stops the
run before load. The plan is then loaded from the SHA the re-plan returns. Without this
stage the decision would sit in the log while the next wave ran against an
unamended plan.

**The re-plan may correct the architecture document's wording, never its
decisions.** A gate note whose only remedy is text in the architecture
document and that moves no decision — a resolved unknown still marked open,
a measured figure replacing an estimate, an accessor or signature the code
settled — gets the verdict `document-text`: the re-plan makes the
correction, commits it, and the ledger records an architecture amendment
with the section. The gate has already ruled that nothing in the note
invalidates a decision; routing such a note back to the requester as an
unabsorbed defect asked for a decision that was not there. A correction that
would change what a section decides is `cannot`.

**A unit over its size budget stops only when neither repair applies.** The
re-plan reports each over-budget unit with a disposition: `extracted` (a
preceding design unit carries the excess), `flagged` (the excess is
wall-clock, not context, and the unit carries the rationale, which
breakdown's size budget accepts as discharging the check), or `unrepaired`.
A flag stands across re-plans, including the decision re-plan before load;
a stop kind derived from standing plan state rather than from the wave's
events could not be cleared by any decision.

**Prompts embed the plan's commit SHA** (and the unit entry verbatim), never a
path alone. A re-plan produces a new SHA, every downstream prompt changes, and
resume re-runs exactly the work the amendment affects. A prompt referencing a
path only would replay a cached result against a plan that no longer exists.

## Escalation

The script stops on a deterministic rule over signals. It never judges a
signal's content; it classifies by declared kind, and the kinds an agent can
declare are only those whose consequence the script can check.

### Signal sources

1. **Structural** — computed by the script or by mechanical checks in the
   stages: a done condition that cannot be evaluated in position; a merge
   conflict or regression; a cycle in the amended graph; critique-gate
   non-convergence (hard cap, a systemic verdict, or a Critical the fix stage
   declined); agent returned null; an environment gate probe failing against
   its pin; an invalid plan at load.
2. **Adjudicated** — one judgement per wave, in a form the script checks: the
   wave gate's complete invalidation, and the re-plan's verdict on each item
   it was handed. Measured recall on premise- and requirements-level defects
   at the sonnet pin is 0/14 (see
   [critic-fidelity-summary](${CLAUDE_PLUGIN_ROOT}/docs/investigations/critic-fidelity-summary.md));
   the gate runs at the opus tier, and its reliability in this form is
   unmeasured (Testing below).
3. **Recorded** — what the three reporting stages return beyond their
   result: the plan's readings, implement's deviations, deferrals,
   discoveries and distress, the gate's notes, and the review stages'
   Significant and Minor findings. Records reach the ledger; deferrals,
   discoveries, claimed-disjoint conflicts, distress and gate notes also
   reach the re-plan as items with ids.

No other stage returns anything beyond its result and its commits. A
discrepancy a critic notices is a finding with a severity, handled by the
gate loop it belongs to.

### Stop rule

RECORD (ledger; the re-plan absorbs what it can): every reading, deviation,
deferral, discovery, distress record, unfixed Significant or Minor finding,
applied Critical fix, gate note, mechanically resolved claimed-disjoint
conflict, re-plan amendment, re-plan verdict of `amended`, `no-change`, or
`document-text` with the architecture amendment it made, and an over-budget
unit repaired by extraction or flagged with rationale.

STOP (return an escalation record):

- an architecture invalidation from the wave gate with all five fields
  present;
- a re-plan item the re-plan cannot absorb, or gives no verdict on;
- a done condition that cannot be evaluated as written;
- a re-plan producing a cycle, or a unit over budget that is neither
  extracted nor flagged with wall-clock rationale;
- critique-gate non-convergence: hard cap, a systemic verdict, or a Critical
  finding the fix stage declined;
- an unresolvable merge conflict, or a clean merge after which the project's
  gates fail;
- a plan that fails the load checks: unreadable block, unknown or duplicate
  units, an overlapping-scope pair with no edge, a bad clearance, or the main
  checkout not on the integration branch;
- an environment gate that has drifted from its plan-time pin, or a wave with
  no ready unit because of uncleared organisational or release gates;
- an undischarged standing obligation at termination;
- an agent returning null (skipped or terminal error).

There are no volume or recurrence rules; counting reported signals measures
verbosity, not trouble in the work.

### Surfacing

Every signal, RECORD or STOP, is surfaced — the requester's ability to notice
drift is a control the design relies on alongside the stop rule.

- **Signal ledger.** Append-only file in the repo,
  `docs/plans/<name>-signals.md`: wave, unit, stage, source, kind, statement,
  referent, disposition. The script renders each wave's rows as a Markdown
  table; a haiku scribe agent appends that text verbatim and commits, once
  per wave (a stop rides in the same append; a wave with no signals appends
  nothing). The plan's Plan Maintenance section records amendments; the
  ledger records why.
- **Live narration.** The script calls `log()` at every wave boundary with the
  counts by kind and source, the units merged, conflicts, and the re-plan's
  amendment summary, so the run is readable in `/workflows` while in flight.
- **Return value.** Every workflow returns the full ledger and a digest,
  whether it completed or stopped. The session presents the digest after every
  run, not only on escalation: counts by kind, source and disposition, and
  what deserves attention on a run that did not stop — the gate's
  architecture notes, unfixed Significant findings per unit, and units that
  showed distress. A clean completion is presented with the same digest and
  the explicit note that silence is not evidence.
- **Decision log.** Requester decisions live in the repo
  (`docs/plans/<name>-decisions.md`) as a `\`\`\`yaml lsf-decisions` block,
  each linked from the ledger entry it resolved and referenced by SHA in the
  prompts it affects. A decision's `clears` entries are the only way an
  organisational, release, or data gate becomes cleared; Plan Maintenance is
  agent-written and never clears a gate.

### Relaunch target

The session does not always relaunch the workflow that stopped. Per STOP kind:

| STOP kind | Relaunch | Inputs |
|-----------|----------|--------|
| Architecture invalidated; a re-plan item unabsorbed because the answer changes a decision in the architecture document or the request record | `lsf-architecture` (or `lsf-brief` when the requester's decision amends the request record; the session then runs the later workflows in sequence without a further prompt) | Decision log SHA; the amended document; Phase 3 state — existing decomposition plan, completion reports, integration branch |
| Done condition unevaluable as written | `lsf-architecture` | Decision log SHA; Phase 3 state |
| Everything else: cycle or unrepairable budget excess; gate non-convergence; unresolvable merge conflict or regression; environment drift; gated units; null agent; load failure; undischarged obligation | Same workflow | Decision log SHA |

Returning to an earlier phase is the skill's own rule ("return to the earliest
affected phase; do not patch forward"). Phase 3 work already merged is not
discarded, but breakdown's re-plan mode cannot absorb it — that mode diffs
completion reports against the existing plan and never reads an architecture
document. When Phase 3 state is present, `lsf-architecture` runs two stages,
reconciliation first:

1. **Reconcile.** An agent reads the amended architecture document, the
   integration branch, the old plan, its completion reports, and the merge
   stage's conflict records, and returns a reconciliation record: for each
   merged unit, whether it still stands under the amended architecture or is
   superseded, with the evidence. The record is committed.
2. **Fresh breakdown** against the amended architecture document, with the
   reconciliation record and integration branch as inputs — the same footing
   as breakdown's feature-description mode, which reads the codebase. Standing
   work is existing code the new units build on; superseded work yields units
   that adapt or revert it; merge conflicts and other Phase 3 signals inform
   the risk registry and parallel-stream claims.

Any new unit breakdown marks as already satisfied by standing work is a
claim, not a completion: it is recorded complete only after a verify agent
evaluates its done condition against the integration branch. A failed
condition makes it new work; an unevaluable one is a STOP. `lsf-implement` is
then relaunched against the new plan SHA; every prompt changes, so resume has
nothing to replay and it is a fresh run in effect.

### Escalation record

Returned by the workflow; presented verbatim by the session.

```
{ workflow, plan_sha?, pre_replan_sha?, wave?, stage,
  signals: [{ kind, source, unit?, stage, statement, referent, raised_by }],
  decision_needed: [ { id, question, options?, affects: [units | stages] } ] }
```

`plan_sha`, `wave`, and `unit` are set by `lsf-implement` only; `stage` names
the research, synthesis, critique, or breakdown stage in the other two.
`plan_sha` is the plan to relaunch from: after a stop at a re-plan, or a wave
stop with units merged, it is the re-plan's own commit, which carries the
amendments it did absorb, and `pre_replan_sha` is the plan the wave ran
against. An
architecture invalidation's statement carries the gate's five fields, so the
question the requester sees includes the proposed amendment.

Signals in one wave surface together. The requester's decisions are committed
to a decision log in the repo by a scribe agent at the start of the relaunch,
then referenced by SHA in affected prompts — state never lives only in `args`.

## Shared stage schema

Every `agent()` in the three workflows returns:

```
{ result: <stage-specific>,
  commits: [ { branch, sha } ] }
```

Nothing else. The stages that report beyond their result do so in
stage-specific `result` fields with defined shapes:

- **plan**: `readings: [{ clause, reading }]`.
- **implement**: `deviations`, `writesOutsideScope`, `deferrals: [{ statement,
  belongsTo }]`, `discoveries: [{ statement, affects }]`, `distress`,
  `doneCondition: { evaluable, met, evidence }`.
- **review stages**: `findings: [{ severity, statement, location }]`, `systemic`.
- **fix**: `applied`, `notApplied: [{ finding, reason }]`.
- **wave gate**: `invalidations: [{ section, decision, premise, evidence,
  amendment }]`, `notes: [{ statement, referent }]`.
- **re-plan**: `amendments: [{ kind, unit, statement }]`, `verdicts: [{ id,
  verdict: amended | no-change | document-text | cannot, statement }]`,
  `overBudget: [{ unit, disposition: extracted | flagged | unrepaired,
  statement }]`.

## Persistence

The script cannot write. Every artefact the skills name — research notes,
brief, architecture document, decomposition plan, implementation plans,
completion reports, decision log — is committed by the agent that produces it,
on a branch the agent reports back. Where an agent's output is inline data
rather than a file (critique findings, review-gate answers), a scribe agent
commits it. This supersedes, for workflow agents only, the file-writes policy
in [subagent-protocol](${CLAUDE_PLUGIN_ROOT}/docs/subagent-protocol.md); worktree
commits are known to survive (Constraints above).

## Testing

The Workflow tool itself is not tested. What is tested is the script's own
logic and the signal layer.

**Scheduling logic** — ready-set computation, wave partitioning, merge order,
signal mapping, stop rule, digest — is a dependency-free JavaScript module
(`workflows/lsf/scheduling.js`, with the shared schema in `schema.js` and the
block parser in `graph.js`), unit-tested outside Claude Code with
`node --test`, and inlined into the script (the script cannot import). The
inlining is a copy step and the only place drift can enter; the module files
are the source of truth.

`graph.js` parses the YAML subset the blocks are written in, maps the plan
block onto `PLAN_SCHEMA`, applies decision-log clearances, and reports every
defect as an `invalid-plan` problem with a referent.

The module reads a plan reduced to its graph (`PLAN_SCHEMA`): unit ids,
dependencies (the plan's dependency types are not carried — a path of
any type serialises), written scope entries, gates, and execution order. A
scope entry is a path or `path::symbol`. Two units overlap when an entry of
one contains or equals an entry of the other: a directory contains the paths
under it, a bare file contains its symbols, and two symbols of one file
overlap only when they are the same symbol. Completed units pair with
nothing. Units count as serialised when a dependency path runs
between them in either direction. `signalsFromGate` stops on an invalidation
only when all five fields are non-empty. Distress thresholds are the
module's config object, overridable from the workflow's `args`.
`renderLedger` produces the wave's ledger table the scribe appends.

**Signal layer** (tentative): a synthetic feature in the bench repo with
injected defects — a deferral, a claimed-disjoint conflict, a unit whose
completion invalidates an architecture assumption, a contradictory
requirement, a requirement-altering critique fix that should have stopped the
run — run end-to-end and scored on whether each defect reaches an
escalation record, at which wave, and from which source. Same shape as the
critic-fidelity measurement. The wave gate's precision and recall in the
five-field form is the first thing it should measure. Not yet committed to.

## Model tiers

Each stage call carries a role tier (`args.tiers` overrides per role):
critique (unit plan critique, code critique, lens review) runs sonnet;
gate (the wave gate alone) runs opus;
planning (unit plan, re-plan, termination) runs fable at medium effort;
mechanical (plan-block copy, decision-log and ledger appends) runs haiku at
low effort, since the script has already rendered what they write and the
parser checks it on the next load; everything else, including termination's
direct edits to the plan, runs sonnet.

## Open questions

- Whether the three workflows are exposed as separate plugin workflows or one
  with a phase argument.
