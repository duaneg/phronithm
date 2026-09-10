# Large-Scale Feature as a Workflow

Design for running the `phronithm:large-scale-feature` lifecycle end-to-end
under the Claude Code Workflow tool: a deterministic script orchestrates a fleet
of agents, holds no judgement, and is restartable from any point. Human
attention is spent only on meta-level signals — invalidated assumptions,
contradictory or incomplete requirements, divergence, scope and goal ambiguity
— never on procedural sign-off.

Status: `lsf-implement` exists (`workflows/lsf/implement.js`, built into
`workflows/lsf-implement.js`) and is tested against a stubbed runtime. The
plugin loader registers it as `phronithm:lsf-implement` (confirmed 2026-09-10
against Claude Code 2.1.267: the loader reads `workflows/` non-recursively,
so `workflows/lsf/` is ignored); it has not been run under the Workflow tool.
`lsf-brief` and `lsf-architecture` do not exist yet.

Detection of these meta-level signals by agents cannot be made reliable at
present (measured, see Escalation). The design therefore treats surfacing as
the primary control: every signal, absorbed or not, reaches the requester.

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
questions, re-plan) is a top-level `agent()` call, so the nesting policy in
[subagent-protocol](${CLAUDE_PLUGIN_ROOT}/docs/subagent-protocol.md) is satisfied structurally — no
agent spawns another.

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
the Critical-only bar, in both `lsf-brief` and `lsf-architecture`. The
requester touchpoint is replaced by the escalation channel: the fix agent,
before applying a fix for a Critical, classifies under the shared schema
whether the fix alters a requirement, the request record, or the goal; if so
it applies nothing and the finding surfaces as a contradiction signal against
the request record or architecture document (the `contradictions` field, not
`ambiguities`) and stops the run. A Critical repairable within the document is fixed without
stopping. This follows the requester's stated preference that procedural
sign-offs be removed and only meta-level signals surface. The compensating
control is a self-reported judgement of unmeasured reliability, in the family
the Escalation section measures as weakest. The residual risk — a
requirement-altering fix misclassified as document-internal and applied
silently — is not detectable by the run; the mitigation is that every Critical
and its applied fix is written to the signal ledger (Surfacing below) so the
requester can review the fixes the loop applied (kind `critical-fixed`,
RECORD; also emitted by `lsf-implement`'s plan and code gates). It is the
first thing the signal-layer measurement should target.

Three workflows rather than one so each phase break stays a fresh-context
boundary, which the skill already requires, and so that an escalation in one
phase relaunches only that phase. The session runs them in sequence, pausing
only on an escalation record.

`phronithm:feature` is not invoked inside the workflow — it needs
ExitPlanMode approval, which no workflow agent can obtain. Its work-unit-mode
phases are decomposed into stages instead:

1. **plan** agent: implementation plan from the architecture document and the
   unit entry, returned inline and committed by the same agent.
2. **critique** loop over the plan (Critical-only bar, round number in prompt,
   hard cap per the [critique gate](${CLAUDE_PLUGIN_ROOT}/docs/critique-gate.md)).
3. **implement** agent, `isolation: 'worktree'`: re-probes the unit's
   environment gates against their plan-time pins (drift is a STOP signal, see
   Stop rule); resets its worktree to the integration branch; executes the
   plan single-shot, with no per-step critique loop — a deliberate
   simplification of feature's Phase 4; commits on its branch; returns
   branch, path, and the completion-report fields.
4. **review** stage over the branch diff: the static-analysis pre-scan, then
   one critique-code gate at the Critical-only bar, then lens review per unit
   type. Feature's per-step critique is dropped, not moved here.
5. **completion-report** is assembled from the implement and review returns and
   committed by a scribe agent.

Organisational and release gates cannot be probed by an agent; a unit carrying
one is never ready until the decision log records the gate as cleared, and a
wave with no ready unit for that reason is a STOP.

Each stage is a separate cache entry, so a restart after a crash in review does
not re-run implementation.

## Wave loop

```
plan = load(decomposition-plan @ sha)          // stops if any overlapping-scope pair lacks an edge
loop:
  ready   = units with all dependencies complete and no uncleared organisational/release gate
  if ready is empty and units remain: return escalation(blocked units)
  if ready is empty: break
  results = parallel(ready.map(unit => runUnit(unit, plan.sha)))   // barrier
  merged  = merge(results, in plan execution order)               // agent, sequential
  signals = structural(results, merged) ∪ reported(results, merged)
  if any signal is STOP: return escalation(signals, plan.sha)
  plan    = replan(plan, results.reports, merged.conflicts, signals)  // agent, commits
  if replan yields STOP signal: return escalation(...)
```

**The script reads only the dependency graph.** Breakdown may reconcile
overlapping-scope parallel units by a prose coordination protocol; the
workflow does not read prose. Every such protocol is encoded in the plan as a
logical edge (serialise = an edge from the earlier unit to the later), and
`load` checks that every pair of units with overlapping scope has an edge
between them, stopping otherwise. This is a constraint on the plan
`lsf-architecture` produces, stated in its breakdown prompt.

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
conflict between units the plan claimed disjoint is a plan defect: it is passed
to re-plan as a discovery and recorded in Plan Maintenance. A conflict the
merge agent cannot resolve mechanically is a STOP signal. Merge order is a
function of the plan, so a re-plan can change the order of later waves; the
merge stage never chooses.

**Prompts embed the plan's commit SHA** (and the unit entry verbatim), never a
path alone. A re-plan produces a new SHA, every downstream prompt changes, and
resume re-runs exactly the work the amendment affects. A prompt referencing a
path only would replay a cached result against a plan that no longer exists.

## Escalation

The script stops on a deterministic rule over signals. It never judges a
signal's content; it classifies by source and declared kind.

### Signal sources

In descending reliability:

1. **Structural** — computed by the script or by mechanical checks in the merge
   and re-plan stages: a done condition that fails or cannot be evaluated in
   position; a merge conflict; a cycle in the amended graph; critique-gate
   non-convergence or hard cap; agent returned null; an environment gate
   probe failing against its pin.
2. **Reported** — mandatory fields in every stage's return schema (below). Each
   agent must state what it assumed, what it found ambiguous, and what it saw
   that contradicts the architecture document or the request record. Forced
   fields beat volunteered remarks; they also invite boilerplate, which the
   measurement plan addresses. The re-plan agent's size-budget verdicts belong
   here too: breakdown defines the budget by judged load factors, not a
   mechanical threshold.
3. **Gate agents** — the critique gate and the LSF review-gate questions run as
   an agent per wave ("do this wave's discoveries invalidate the architecture
   document?"). Measured recall on premise- and requirements-level defects at
   the sonnet pin is 0/14 (see
   [critic-fidelity-summary](${CLAUDE_PLUGIN_ROOT}/docs/investigations/critic-fidelity-summary.md)); this
   source is the weakest on exactly the signals that matter most.

### Stop rule

RECORD (autonomous; amend the plan, note in Plan Maintenance, continue):
anything the re-plan can absorb — a deferral, a scope reassignment, a new
edge, a claimed-disjoint conflict resolved mechanically, an over-budget unit
repaired by extracting a preceding `design` unit or flagged with wall-clock
rationale — breakdown's two sanctioned repairs.

STOP (return an escalation record): anything above the plan —

- an assumption in the architecture document or request record reported
  invalidated, by any source;
- a requirement reported contradictory or incomplete;
- scope or goal ambiguity reported by any stage;
- a done condition that cannot be evaluated as written;
- a re-plan producing a cycle, or a unit over budget that extraction cannot
  repair;
- critique-gate non-convergence or hard cap;
- an unresolvable merge conflict, or a clean merge after which the test suite
  fails (the merge stage undoes it and reports the unit regressed);
- a review-gate handoff rated insufficient (partial is RECORD);
- a plan that fails the load checks: unknown or duplicate units, an
  overlapping-scope pair with no edge, or the main checkout not on the
  integration branch;
- an environment gate that has drifted from its plan-time pin, or a wave with
  no ready unit because of uncleared organisational or release gates;
- an undischarged standing obligation at termination;
- an agent returning null (skipped or terminal error).

STOP on volume, independent of any single signal's content — these rules
need no detection quality, only counting:

- a completion report carrying breakdown's execution-distress signals
  (context near exhaustion, mid-implementation design decisions, heavy
  deviation or deferral count);
- the same referent named in signals from two different waves or two
  different sources;
- a unit's assumptions or ambiguities exceeding a per-unit threshold set in
  the workflow's arguments;
- a wave in which no stage reports any assumption, ambiguity, or contradiction
  — silence across a whole wave is more likely under-reporting than
  cleanliness, given the recall figures above.

The rule biases towards stopping on weak signals. Detection of meta-level
defects by agents cannot be made reliable at present; the mitigation is to
surface everything so the requester can see a run going wrong before the
script can know it (Surfacing below).

### Surfacing

Every signal, RECORD or STOP, is surfaced — the requester's ability to notice
drift is the control the design actually relies on.

- **Signal ledger.** Append-only file in the repo,
  `docs/plans/<name>-signals.md`, committed by the scribe agent at the end of
  every wave and every `lsf-brief` / `lsf-architecture` stage: wave, unit,
  stage, source, kind, statement, referent, disposition (recorded / stopped /
  decided-by-requester with the decision-log entry). The plan's Plan
  Maintenance section records amendments; the ledger records why.
- **Live narration.** The script calls `log()` at every wave boundary with the
  counts by kind and source, the units merged, conflicts, and the re-plan's
  amendment summary, so the run is readable in `/workflows` while in flight.
- **Return value.** Every workflow returns the full ledger and a digest,
  whether it completed or stopped. The session presents the digest after every
  run, not only on escalation, with three trend flags computed by the script:
  rising assumption count per unit across waves; the same referent recurring;
  and silent waves. A clean completion is presented with the same digest and
  the explicit note that silence is not evidence.
- **Decision log.** Requester decisions live in the repo
  (`docs/plans/<name>-decisions.md`), each linked from the ledger entry it
  resolved and referenced by SHA in the prompts it affects.

### Relaunch target

The session does not always relaunch the workflow that stopped. Per STOP kind:

| STOP kind | Relaunch | Inputs |
|-----------|----------|--------|
| Requirement contradictory or incomplete; goal ambiguity | `lsf-brief` | Decision log SHA; the amended request record. The session then runs `lsf-architecture` and `lsf-implement` in sequence without a further prompt, handing the Phase 3 state below to `lsf-architecture`. |
| Architecture assumption invalidated; scope ambiguity; done condition unevaluable as written | `lsf-architecture` | Decision log SHA; Phase 3 state if any — existing decomposition plan, completion reports, integration branch |
| Cycle or unrepairable budget excess; gate non-convergence or cap; unresolvable merge conflict; null agent | Same workflow | Decision log SHA |

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

Breakdown runs grounded in what was actually built, rather than planning blind
and being patched afterwards. Any new unit breakdown marks as already
satisfied by standing work is a claim, not a completion: it is recorded
complete only after a verify agent evaluates its done condition against the
integration branch. A failed condition makes it new work; an unevaluable one
is a STOP. `lsf-implement` is then relaunched against the new plan SHA; every
prompt changes, so resume has nothing to replay and it is a fresh run in
effect.

### Escalation record

Returned by the workflow; presented verbatim by the session.

```
{ workflow, plan_sha?, wave?, stage,
  signals: [{ kind, source, unit?, stage, statement, referent, raised_by }],
  decision_needed: [ { id, question, options?, affects: [units | stages] } ] }
```

`plan_sha`, `wave`, and `unit` are set by `lsf-implement` only; `stage` names
the research, synthesis, critique, or breakdown stage in the other two.

Signals in one wave surface together. The requester's decisions are committed
to a decision log in the repo by a scribe agent at the start of the relaunch,
then referenced by SHA in affected prompts — state never lives only in `args`.

## Shared stage schema

Every `agent()` in the three workflows returns at least:

```
{ result: <stage-specific>,
  assumptions: [ { statement, referent } ],
  ambiguities: [ { statement, about: "scope" | "goal" | "requirement" | "design" } ],
  contradictions: [ { statement, against: <doc path + SHA> } ],
  commits: [ { branch, sha } ] }
```

Empty arrays are valid. The script maps `ambiguities` and `contradictions`
straight into signals; `assumptions` are recorded and passed to the review-gate
agent for the wave.

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
stop rule — is a dependency-free JavaScript module (`workflows/lsf/scheduling.js`, with
the shared schema in `schema.js`), unit-tested outside Claude Code with
`node --test`, and inlined into the script (the script cannot import). The
inlining is a copy step and the only place drift can enter; the module file
is the source of truth.

The module reads a plan reduced to its graph (`PLAN_SCHEMA`): unit ids,
typed dependencies, written scope paths, gates, and execution order. Two
units overlap when a scope path of one contains or equals a scope
path of the other; they count as serialised when a dependency path of any
type runs between them in either direction. Signals from `ambiguities` map by
`about`: scope and goal to their STOP kinds, requirement to
`requirement-incomplete`, design to a RECORD kind. Every `contradictions`
entry is `assumption-invalidated`. Volume thresholds (per-unit signal count,
distress deviation and design-decision counts) are the module's config
object, overridable from the workflow's `args`.

**Signal layer** (tentative): a synthetic feature in the bench repo with
injected defects — a deferral, a claimed-disjoint conflict, a unit whose
completion invalidates an architecture assumption, a contradictory
requirement, a requirement-altering critique fix that should have stopped the
run — run end-to-end and scored on whether each defect reaches an
escalation record, at which wave, and from which source. Same shape as the
critic-fidelity measurement. Not yet committed to.

## Model tiers

Each stage call carries a role tier (`args.tiers` overrides per role):
gate (critique, lens review, architecture gate, obligations) runs opus;
planning (unit plan, re-plan, termination) runs fable at medium effort;
everything else runs sonnet. Static analysis inherits the session model.

## Open questions

- Whether `phronithm:feature`'s work-unit mode should itself be restructured
  to match the five stages above, so the two paths stay one contract, or the
  workflow carries its own stage prompts.
- Whether the three workflows are exposed as separate plugin workflows or one
  with a phase argument.
