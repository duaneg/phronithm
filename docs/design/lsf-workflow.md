# Large-Scale Feature Phase 3 as a Workflow

Design for running Phase 3 of `phronithm:large-scale-feature` — the unit loop
over a committed decomposition plan — under the Claude Code Workflow tool: a
deterministic script orchestrates a fleet of agents, holds no judgement, and
is restartable from any point. Phases 1–2 stay interactive in the skill. Human
attention in Phase 3 is spent on three things: signals the script can verify
mechanically; one adjudicated judgement per wave — that the architecture
document no longer stands — delivered in a form the script checks for
completeness and the requester can act on; and the digest the session puts
to the requester after every run. Never procedural sign-off, and never a
free-text discrepancy an agent chose to file under a heading.

Status: `lsf-implement` exists (`workflows/lsf/implement.js`, built into
`workflows/lsf-implement.js`), is tested against a stubbed runtime, and has
run on cartomancer-v2 (nine runs to 2026-09-19, a 41-unit stream closed).
The rules added since — the archive at merge, the ledger row shapes, the
re-plan skip on a wave with nothing to absorb, the script-rendered completion
report, reviewers taking the gate runs at HEAD (the implement stage's or
the last code fix's), the `gate-not-run` stop and the long-gate recipe —
have not run. The plugin loader registers it as `phronithm:lsf-implement`
(confirmed 2026-09-10
against Claude Code 2.1.267: the loader reads `workflows/` non-recursively,
so `workflows/lsf/` is ignored); the bare name below denotes the script.

## Constraints from the Workflow tool

Documented behaviour this design relies on:

- **No mid-run user input.** Workflow agents have AskUserQuestion and
  EnterPlanMode stripped; only permission prompts pause a run. Sign-off between
  stages means one workflow invocation per stage.
- **No pause.** TaskStop kills the run; an agent in flight is re-run from
  the start of its prompt on resume. The script cannot read the filesystem.
  The wave loop's pause ref works around both.
- **The main checkout is shared with the requester**, who may edit it while
  a run is live. Every stage that commits there (decision log, merge, re-plans,
  termination, direct edits, ledger) stages and commits only its own files by
  pathspec. It never uses `add -A`, `commit -a`, `stash`, `clean` or
  `reset --hard` there: an unscoped commit once swept a requester's
  `CLAUDE.md` edit into a ledger commit. A regressed merge is undone with
  `git reset --merge`, which keeps unrelated uncommitted changes.
- **Resume replays the longest unchanged prefix of `agent()` calls**, keyed on
  prompt text in call order. The first call whose prompt differs re-runs, and so
  does everything after it.
- **The script has no filesystem, shell, module or clock access.** Durable
  state is what agents commit and what the run journal records.
- **Worktree agents' commits persist.** Probed 2026-09-10: a worktree agent
  runs in `.claude/worktrees/<runId>-<agentIndex>` on branch
  `worktree-<runId>-<agentIndex>`, based on HEAD at spawn; its commits survive
  the run. The branch is subject to the periodic worktree sweep, so the merge
  stage runs in the same workflow, never a later one. The sweep removes only
  worktrees with no changes; every agent that committed leaves its worktree
  and branch behind (one run on cartomancer-v2 left 30, at 5 GB across nine
  runs), so a mechanical cleanup stage after each merge removes the
  `.claude/worktrees/` entries whose `worktree-wf_*` branch is fully merged
  into the integration branch. Unmerged branches stay: they are a stopped
  unit's work in flight.
- **On resume, a re-run agent reuses its worktree with prior state intact**
  (same probe). An implement stage must begin by resetting its worktree to the
  integration branch, or a re-plan-triggered re-run inherits the superseded
  attempt.
- **An `args` change alone does not invalidate cached calls** whose prompt text
  is unchanged (same probe). Decisions handed back must therefore appear in
  the prompts of the calls they affect — the plan or decision-log SHA does
  this — not merely in `args`.
- **The latest human message of the launch turn is relayed to the run's
  agents**, under a frame stating it outranks their task. Probed at Claude
  Code 2.1.289 (2026-10-05, one single-agent run per condition,
  [results](../investigations/relay-probe-results.md)): the agent's first
  user turn is a `[Workflow harness — user request]` block quoting that
  message, indented. The turn's starting message is relayed whether the
  launch is the turn's first tool call (typed `wf_8286cecd-21c`; collapsed
  paste `wf_a00b2346-6b4`, relayed in full, not as its placeholder) or
  follows a tool result (`Bash` `wf_119eb6c3-fab`; AskUserQuestion answer
  `wf_a61f647b-bc6`, which relays the turn's message, not the answer). A
  slash command relays as the command and its arguments, not the skill body
  (`wf_689da7e8-1ad`). A human message delivered mid-turn replaces it
  (`wf_d2c2719c-453`). In one turn started by a task notification, two runs
  carried no relay, the second after an AskUserQuestion answer
  (`wf_90db02e7-63b`, `wf_956ed621-4cd`); in another, a human message
  delivered mid-turn was relayed (`wf_d08c2e20-b27`). Earlier runs agree (2.1.274–2.1.283;
  none before 2.1.274 carried a relay): every agent of a run launched in a
  human-started turn carried it, and a resume relays by its own turn.
  Unprobed: a turn started by a subagent hand-back.
- **Plugins ship workflows** from a `workflows/` directory, namespaced
  `phronithm:<meta.name>`.
- **Every agent pays a floor before its first read**: the harness prompt
  plus the target project's CLAUDE.md, which the harness injects into every
  agent. Measured on cartomancer-v2 (2026-09-19): about 61k tokens on haiku
  and 83k on opus, of which the workflow prompt is 0.4–47 KB; the CLAUDE.md
  grew from 92 KB to 141 KB over the stream and moved the floor by about 12k
  tokens. The plan is not in the floor. Every extra agent per wave costs at
  least the floor, so the design adds none for convenience; `lsf-drive`
  reports the CLAUDE.md size at startup against a 40 KB threshold and
  leaves trimming to the project.

## Architecture

Two layers.

**Script** — the orchestrator. Plain JavaScript run by the Workflow tool.
Holds the wave loop, the ready-set computation, the stop rule, and nothing
else. All state is agent return values (structured via schema) plus the
commits agents make. Restart = relaunch with `resumeFromRunId`.

**Session** — a presenter and relauncher, any model tier, running the
`phronithm:lsf-drive` skill. Launches the workflow, shows an escalation record
when the script returns one, collects the requester's decision, and relaunches
with the decision as an argument. It holds no loop state, makes no
orchestration decisions, and does no unit work.

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
defects is 0/14 at the sonnet pin and unmeasured at the gate's tier
(Escalation below); per-stage
discovery fields, measured over three runs, yield literal but immaterial
document-versus-code discrepancies restated by every stage that reads the
same documents, and nothing material the wave gate does not also raise.

## Lifecycle mapping

| LSF phase | Where it runs | Notes |
|-----------|---------------|-------|
| Phase 1 (brainstorm, research, architecture brief) | Interactive session (`phronithm:large-scale-feature`) | The request record the workflow names is the architecture brief's requirements and success criteria. |
| Phase 2 (architecture document, critique, doc updates, breakdown) | Interactive session (`phronithm:large-scale-feature`) | Ends by committing the decomposition plan and naming the integration branch Phase 3 merges into. Phase 2 re-entry after a Phase 3 stop is the skill's re-entry sub-procedure. |
| Phase 3 (unit loop) | Workflow `lsf-implement` | The wave loop below. |
| Phase termination (meta-tasks, standing obligations) | Tail of `lsf-implement` | Termination stage, see Wave loop. |

**Deliberate divergence from the skill, in Phase 3 only.** LSF Phase 1
step 7 and Phase 2 step 3 require requester sign-off on the fix for each
Critical critique finding; that stands, since those phases are interactive.
The workflow's critique gates (unit plan, code) apply fixes autonomously, at
the Critical-only bar, because no workflow agent can obtain sign-off. The fix
stage applies every Critical it can repair within the artefact under
critique; a Critical it declines — because the repair would change the unit
entry, the architecture document, or the request record, or because it cannot
repair it — is a structural stop (`gate-non-convergence`, with the finding
and the reason in the escalation record). The requester sees the finding and decides. Every
applied fix is written to the signal ledger (kind `critical-fixed`) so the
requester can review what the loop changed.

Phases 1–2 are not workflows: their gates require requester sign-off on
Critical findings, which no workflow agent can obtain (Constraints above),
and premise-level defects — the kind a scripted critique gate has not been
shown to catch (Escalation below) — are concentrated there.

`phronithm:feature` is not invoked inside the workflow — it needs plan-mode
approval, and EnterPlanMode is stripped (Constraints above). The workflow
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
   runs the project's gates once at its final HEAD; returns branch, path,
   the gate runs (command, status, one-line summary), and the
   completion-report fields: deviations, files written outside the Scope
   line, deferrals with the receiving unit, discoveries with the unit or
   architecture section they affect, distress counts, the done-condition
   verdict. The prompt tells the stage how to wait out a gate longer than
   the Bash tool's limit without ending its turn: run it detached with its
   output, exit status and PID in per-gate files under the git directory,
   and poll in calls shorter than the limit until the gate ends or dies (no
   status file: `not-run`). A gate reported `not-run` is a STOP
   (`gate-not-run`) before review, and no gate reported is a STOP
   (`gate-omitted`; `gate-not-run` in the Workflow script): the reviewers would meet the
   same tool timeout, and a relaunch meets the same gate, so the requester
   changes the environment or the project until the gate completes. A design
   unit is exempt; its review reads no gates.
4. **review** stage over the branch diff: one critique-code gate at the
   Critical-only bar and lens review per unit type, in parallel. No separate
   static-analysis stage: the project's own gates (linters, type checks) run
   in the implement stage, whose done conditions require them green, and
   again after each merge. Both reviewers are given the gate runs at the
   branch's HEAD — the implement stage's, or after a code fix the fix
   stage's, which re-runs every gate the implement stage ran and returns the
   runs (a code fix whose gate did not run stops with `gate-not-run`, and
   one that omits one of those gates with `gate-omitted`, as at implement) — and take
   them as the test and lint result: no suite, CI job, or
   lint pass of their own, a single targeted test only to check a finding's
   claim (in a compiled project the two suite re-runs cost more of the
   reviewers' wall-clock than the review). The code critique evaluates the
   done condition under the plan's readings from the code and those gate
   runs; an unmet condition, or a reading the code does not bear out, is
   Critical. Significant and Minor findings are recorded, not fixed.
5. **completion-report** is rendered by the script from the implement and
   review returns — summary, done-condition verdict and evidence, deviations
   and writes outside Scope, deferrals, discoveries, standing obligations,
   gate re-verification, the gate runs at the unit's final HEAD (the last
   code fix's, when there was one; the verdict stays the implement
   stage's), and the counts of findings left unfixed with the
   ledger's path — and committed on the unit branch by a copy stage at the
   mechanical tier. No agent authors it; the rows the findings became are in
   the ledger, not the report.

**What a unit entry's fields mean to a stage.** Scope is a coordination
declaration: the files this unit writes that another unit in flight might
also write. The scheduler reads it for overlap. It does not limit what the
work may edit; a file the work needs outside the Scope line is changed and
reported as a deviation, and the next re-plan that runs corrects the line
where a later unit shares the file. A deviation does not itself trigger a
re-plan (Wave loop below), so the correction waits for a wave that has an
item to absorb; until then the merge stage's conflict detection is the
backstop. Every unit-stage prompt states this.

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
  gate    = invalidations, notes, amendments from the wave gate    // agent, one question; units' own architecture diffs vs the pre-wave pin
  signals = structural(results, merged) ∪ gate
  if any signal is STOP and (nothing merged or the stop supersedes the plan): return escalation(signals, plan.sha)
  if no item and no claimed-disjoint conflict: if STOP return escalation(signals, plan.sha); continue   // re-plan skipped
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
The stage then gives every standing obligation a disposition. One carried by
its own terms — its text places its discharge outside this plan — is a
record when the stage names where it is now recorded outside the plan (a
debt-register row, another plan's Standing Obligations); otherwise, and for
one that is undischarged, a STOP. Without the carrier the obligation can
only ever stop the run, since no decision clears it.

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

A cleanup agent on the mechanical tier runs beside the wave gate and removes
the worktrees whose branches merged. It never signals; a failure leaves
residue, not risk.

**The merge archives what it completes.** In the commit that marks a unit
`complete: true`, the merge stage moves the unit's `### <id>` entry to
`docs/plans/<name>-archive.md` and leaves a one-line stub naming the merge
SHA, the archive, and the completion report; its Plan Maintenance entry is
the fixed two-line form (heading; report path and SHA, ledger file and
wave), never restated report content. The plan block is untouched apart from
the flag ([plan graph](${CLAUDE_PLUGIN_ROOT}/docs/plan-graph.md), Archive).
The plan therefore stops growing with completions; on cartomancer-v2 it had
reached 531 KB, 240 KB of it Plan Maintenance prose and 173 KB completed
entries, before this rule (measured 2026-09-19).

**A wave with nothing to absorb skips the re-plan.** The re-plan's inputs are
the items — deferrals, discoveries, gate notes, distress — and claimed-disjoint
merge conflicts. When a wave produces none, the re-plan has no work and is
not spawned: no reload either, since the plan already carries the merge's SHA
and completion flags. Readings, deviations, and findings are ledger rows
only and do not trigger it. A stop on such a wave escalates with the merge's
SHA and no `pre_replan_sha`. The merge stage is never skipped by this rule.

**A wave stop still re-plans over whatever merged.** A relaunch re-plans
over the requester's decisions alone, so the wave's re-plan is the only pass
that absorbs the merged units' reports. The stop signals go to the re-plan as
context it must not amend for; the unit each names stays incomplete. The
re-plan is skipped when nothing merged, when the wave has nothing to absorb
(below), and when a stop supersedes the plan (architecture invalidated — the
relaunch is Phase 2's re-entry).

**The wave gate asks one question**: does anything this wave invalidate the
architecture document — does any decision it makes rest on a premise the
merged code or the completion reports show false. An invalidation is
returned in a fixed form: section, decision, premise, evidence, amendment
(what the section should now say). The script stops only when all five
fields are present; an invalidation missing a field, and every note the gate
returns besides, is recorded and handed to the re-plan. The gate is told not
to re-evaluate done conditions or review findings (the unit stages did, at
the bar this workflow sets) and not to report the workflow's own process.

**The architecture pin follows the integration branch.** The pin is the
architecture SHA every prompt names. Load sets it from `args.architectureSha`
or the copy stage's report. The merge stage reports the document's SHA after
its last commit, and the pin moves there before the gate, so the gate, the
re-plan and every later stage read the merged text. A reload after the
re-plan or termination moves it to the copy stage's report, which carries
any `document-text` correction. A unit's own change to the document must
not become its own baseline. When the pin moved in a wave, the gate is given
each merged unit's change — its merge commit against the first parent, so
the requester's commits drop out — and judges it against the text it was
merged onto, which includes the wave's earlier merges.
A change that records the decision a design unit was charged to settle,
settles a question the document left open to that unit, or corrects wording
is returned under `amendments` and recorded as an architecture amendment
with the unit; the re-plan's `document-text` corrections are the other
source of that record. A change that moves a decision beyond that is an
invalidation, its five fields filled as any other. An empty or missing
report keeps the pin; a merge stage that ran and merged nothing still
advances it, since any change is then the requester's own. A
gate that returns null judges nothing: the wave stops after its re-plan,
the relaunch pins the branch tip, and the unjudged change is the baseline
from then on, with the stop as its only record.

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

The re-plan writes those two documents and nothing else. An item whose
remedy is text elsewhere — a comment, a figure in a completion report or
the debt register, a register row to file — goes to a remaining unit's done
condition where one's Scope covers the file; otherwise, where it changes no
behaviour and no test, the re-plan returns it under `directEdits` and a
scribe stage makes and commits it after the re-plan; where it does, a unit
is added. The termination stage's small meta-tasks take the same route.
Direct edits pass no gate, so the ledger records each with its path and
the digest lists them.

**A unit over its size budget stops only when neither repair applies.** The
re-plan reports each over-budget unit with a disposition: `extracted` (a
preceding design unit carries the excess), `flagged` (the excess is
wall-clock, not context, and the unit carries the rationale, which
breakdown's size budget accepts as discharging the check), or `unrepaired`.
A flag stands across re-plans, including the decision re-plan before load;
a stop kind derived from standing plan state rather than from the wave's
events could not be cleared by any decision. The re-plan reports every
standing flag on every pass; the script records one only when the unit's
disposition differs from the last recorded in the run (an unrepaired unit is
always recorded, since it stops). A relaunch starts with an empty in-memory
ledger, so each standing flag lands once more per launch.

**Prompts embed the plan's commit SHA** (and the unit entry verbatim), never a
path alone. A re-plan produces a new SHA, every downstream prompt changes, and
resume re-runs exactly the work the amendment affects. A prompt referencing a
path only would replay a cached result against a plan that no longer exists.

**A pause lands at a wave boundary.** The merge, ledger and plan-copy
stages report whether the git ref `args.pauseRef` (default
`refs/lsf/<name>/pause`) exists; the script latches the first report and,
at the loop top, once the wave's merge, re-plan and ledger commits are in,
returns `paused`. A ref set after the last of those stages in a wave is seen
the wave after, so the latency is up to one wave. At load the decisions and
carried rows are committed first, and the return's `planSha` is the decision
re-plan's. A stop outranks a pause and leaves the ref for the session to
report. A seeded return's report is ignored; a resumed run's replayed stages
report the ref as it stood when they first ran. A ref dirties no working
tree, so the breakdown pre-flight inside the re-plan stays green.

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
   its pin; an invalid plan at load. Structural signals also include the
   driver-detected events: what the runtime observes about a call, rather
   than reads from its result. These are permission denials, changes left
   uncommitted at a call's return, a skill that did not load, a guard
   kill after a valid result, a result outside its stage's schema, and a
   reported SHA that is not its branch's tip (corrected from git).
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
`document-text` with the architecture amendment it made, an over-budget
unit repaired by extraction or flagged with rationale, a permission
denial, changes a read-only stage left in its checkout, a guard kill
after a valid result (the result stands), and a unit stage's reported SHA
corrected to its branch's tip.

STOP (return an escalation record):

- an architecture invalidation from the wave gate with all five fields
  present;
- a re-plan item the re-plan cannot absorb, or gives no verdict on;
- a done condition that cannot be evaluated as written (the relaunch carries
  the requester's rewrite as a decision);
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
- an agent returning null (skipped or terminal error);
- a stage that commits returning with changes no commit holds, or whose
  uncommitted check failed. In the main checkout a requester's own edit
  during the call counts too: the statement names the paths;
- a skill that did not load after the runtime's retry;
- a gate a stage left out of its return (`gate-omitted`): none reported, or
  one the implement stage ran that a code fix did not report;
- a code stage's own failure (`stage-fault`), its reason in the statement;
- a stage result outside its schema (an enum value, a missing field), even
  where its mapping would otherwise record it: an unknown merge status is
  not a claimed-disjoint conflict, and an unknown finding severity is not
  dropped.

There are no volume or recurrence rules; counting reported signals measures
verbosity, not trouble in the work.

### Surfacing

Every signal, RECORD or STOP, is surfaced — the requester's ability to notice
drift is a control the design relies on alongside the stop rule.

- **Signal ledger.** Append-only files in the repo, one per launch:
  `docs/plans/<name>-signals/<plan SHA the launch started from>.md`, named
  in the return's `ledgerPath`. Columns: wave, unit, stage, source, kind,
  statement, referent, disposition. The script renders each wave's rows as a
  Markdown table; a haiku scribe agent appends that text verbatim and
  commits, once per wave (a stop rides in the same append; a wave with no
  signals appends nothing). Row shapes keep the file an index, not a copy of
  the artefacts it points at: readings are one count row per unit whose
  referent is the unit plan, where the Readings section holds the text;
  Minor findings are one count row per gate per unit with up to five
  locations as the referent and no text; a Significant finding is one row;
  a verdict row carries the item's id and verdict, not the item's text,
  which is already in the row it answers (a decision re-plan's verdicts keep
  the question, having no prior row). No stage reads the ledger; it is the
  requester's audit trail and the scribe's input. Measured on cartomancer-v2
  (2026-09-19): 1.3 MB over 1864 rows, 394 KB of it finding rows (268 Minor),
  reading rows duplicating the unit plans' Readings sections, and verdict
  rows repeating the item text they answered. The plan's Plan
  Maintenance section records amendments; the ledger records why.
- **Live narration.** The script calls `log()` at every wave boundary with the
  counts by kind and source, the units merged, conflicts, and the re-plan's
  amendment summary, so the run is readable in `/workflows` while in flight.
- **Return value.** The workflow returns, in this order, its status
  (`complete`, `stopped`, `wave-cap`: `args.maxWaves`, default 20, reached
  with units remaining, or `paused`: the pause ref seen, see *Wave loop*;
  both relaunched from the returned `planSha` with no decisions, a `paused`
  one only once the requester lifts the pause), the escalation record
  when it stopped, the digest, and the
  ledger rows no scribe committed; the ledger itself lives in the signals
  file. Rows
  returned unscribed go back to the next launch as `args.unscribed`; a scribe
  appends them before the run does anything else, and they stay out of that
  run's digest. The session presents the escalation's `decision_needed` first on a stop, and the digest
  after every run, not only on escalation: counts by kind, source and
  disposition, and what deserves attention on a run that did not stop — the
  gate's architecture notes, unfixed Significant findings per unit, units
  that showed distress, and every direct edit made without a gate. A clean
  completion is presented with the same digest and the explicit note that
  silence is not evidence.
- **Decision log.** Requester decisions live in the repo
  (`docs/plans/<name>-decisions.md`) as a `\`\`\`yaml lsf-decisions` block,
  each linked from the ledger entry it resolved and referenced by SHA in the
  prompts it affects. A decision's `clears` entries are the only way an
  organisational, release, or data gate becomes cleared; Plan Maintenance is
  agent-written and never clears a gate.

### Relaunch target

A stop does not always relaunch the workflow. Per STOP kind:

| STOP kind | Relaunch | Inputs |
|-----------|----------|--------|
| Architecture invalidated; a re-plan item unabsorbed because the answer changes a decision in the architecture document or the request record | `phronithm:large-scale-feature` Phase 2, re-entry sub-procedure (fresh session; the skill's startup prompt carries the inputs) | Per `phronithm:lsf-drive`'s *Return to Phase 2* prompt: plan SHA, architecture document and request record, integration branch, ledger path, the stop with the gate's five fields, the requester's decision, unscribed ledger rows |
| Everything else: done condition unevaluable as written (the requester's rewrite is a decision the decision re-plan applies); cycle or unrepairable budget excess; gate non-convergence; unresolvable merge conflict or regression; environment drift; gated units; null agent; load failure; undischarged obligation; uncommitted at return; skill not loaded; schema violation; gate omitted; code-stage fault | Same workflow | Decision log SHA |

For an uncommitted-at-return stop, the requester commits or removes the named paths and relaunches. A relaunch without decisions replays the call, which re-checks the paths and passes once they are clean. With decisions, every prompt changes and the stage runs again. An isolated writer's worktree is reused, so dirt still in it is reported again.

Returning to an earlier phase is the skill's own rule ("return to the earliest
affected phase; do not patch forward"). Phase 3 work already merged is not
discarded, but breakdown's re-plan mode cannot absorb it — that mode diffs
completion reports against the existing plan and never reads an architecture
document. The skill's re-entry sub-procedure (Phase 2) does the absorbing:
a decision amending the request record is its first step; it then amends the
architecture document, commits a reconciliation record
(`docs/plans/<name>-reconciliation.md`, contents per the skill's Produces
entry: each merged unit `stands` or `superseded` under the amended
architecture with evidence, the unmerged units and their branches, and a
disposition for each standing obligation), and runs a fresh
`phronithm:breakdown` against the amended architecture document with the
old plan, the reconciliation record, and the integration branch as inputs.
Standing work is existing code the new units build on; superseded work
yields units that adapt or revert it. Work on a branch that never merged is
abandoned and redone under the new plan; the record names the branch so it
can be salvaged by hand, and nothing reads it. The fresh breakdown retires unit ids,
so the re-entry then retires the decision log's `clears` entries naming
them, which would otherwise fail every later load. The unscribed ledger
rows travel through the prompt into the relaunch's `args.unscribed`.

Any new unit breakdown marks as already satisfied by standing work is a
claim, not a completion: `phronithm:breakdown` records it complete only after
evaluating its done condition against the integration branch, and reports an
unevaluable condition to the requester. A failed condition makes it new work.
`lsf-implement` is then relaunched against the new plan SHA; every prompt
changes, so resume has nothing to replay and it is a fresh run in effect.

### Escalation record

Returned by the workflow; presented verbatim by the session.

```
{ workflow, plan_sha?, pre_replan_sha?, wave?, stage,
  signals: [{ kind, source, unit?, stage, statement, referent, raised_by }],
  decision_needed: [ { id, question, options?, affects: [units | stages] } ] }
```

`workflow` is the constant `lsf-implement`, kept so a second workflow would
not change the shape. `plan_sha` is the plan to relaunch from: after a stop at a re-plan, or a wave
stop with units merged and an item for the re-plan to absorb, it is the
re-plan's own commit, which carries the amendments it did absorb, and
`pre_replan_sha` is the plan the wave ran against. A wave stop with nothing
to absorb, or one that supersedes the plan, runs no re-plan: `plan_sha` is
the merge's commit and there is no `pre_replan_sha`. An
architecture invalidation's statement carries the gate's five fields, so the
question the requester sees includes the proposed amendment.

Signals in one wave surface together. The requester's decisions are committed
to a decision log in the repo by a scribe agent at the start of the relaunch,
then referenced by SHA in affected prompts — state never lives only in `args`.

## Shared stage schema

Every `agent()` in the workflow returns:

```
{ result: <stage-specific>,
  commits: [ { branch, sha } ] }
```

Nothing else. No stage returns a document inline: the unit plan and the
completion report are committed and read by path from the branch, so a
critique round's prompt names the plan, never carries it. The stages that
report beyond their result do so in stage-specific `result` fields with
defined shapes:

- **plan**: `readings: [{ clause, reading }]`, `bytes`. The plan itself stays
  on the branch; the prompt states a size target (12 KB), written in one call
  and never trimmed to length, and admits to Readings only clauses that
  admitted two readings. Goal and Context cite the unit entry and the
  standing obligations rather than restating them: every stage prompt
  carries them. The completion report's target is 8 KB, measured by the
  script on the text it rendered. A document past twice its target is a
  `document-oversize` ledger row. A ceiling in the prompt made the plan stage
  trim by dozens of edits, at more cost than the readers it saved, so the
  script records size and never enforces it.
- **implement**: `gates: [{ command, status, summary }]`, `deviations`,
  `writesOutsideScope`, `deferrals: [{ statement, belongsTo }]`,
  `discoveries: [{ statement, affects }]`, `distress`,
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

The script cannot write. Every artefact the workflow produces — the
decomposition plan and its archive (`docs/plans/<name>-archive.md`:
completed entries and closed risks), the architecture-document corrections
the re-plan makes, implementation plans, completion reports, the decision
log, signal ledgers — is committed by the agent that produces it, on a
branch the agent reports back. Where an agent's output is inline data
rather than a file (critique findings, review-gate answers), a scribe agent
commits it. This supersedes, for workflow agents only, the file-writes policy
in [subagent-protocol](${CLAUDE_PLUGIN_ROOT}/docs/subagent-protocol.md); worktree
commits are known to survive (Constraints above).

When a run's journal is gone, `lsf-implement` takes `args.seed`: stage
returns keyed by agent label, used in place of spawning that agent. The
caller recovers a return from the subagent transcript and vouches it was
produced against the same plan SHA.

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
critique (unit plan critique, code critique, lens review) runs opus at low
effort; gate (the wave gate alone) runs opus at medium effort;
planning (unit plan, re-plan, termination) runs opus at high effort;
mechanical (worktree cleanup) runs haiku at low effort; everything else —
implement, merge, the direct edits the re-plan and termination stages
return, the obligation dispositions — runs opus at low effort. The load
(the plan and decision-log reads), the decision-log and ledger appends and
the completion-report commit are code in the Go orchestrator, not agent
calls, and take no tier (`lsf-runtime/orchestrator`, D18). The JavaScript
keeps them as mechanical-tier agents until `script-retirement`.
