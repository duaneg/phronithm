---
description: "Drive Phase 3 of phronithm:large-scale-feature under the Workflow tool: check the plan and repo are ready, launch phronithm:lsf-implement, read its return, present the stop or the digest, collect requester decisions, and relaunch. Use after the decomposition plan is committed, after every stop, and to recover a crashed run."
allowed-tools: Read, Grep, Glob, Bash, Workflow, TaskStop, AskUserQuestion, Skill, ToolSearch
---

# LSF Drive

The session side of the `phronithm:lsf-implement` workflow: a presenter and relauncher. It launches the workflow, reads what came back, shows the requester what needs a decision, records the decision as a workflow argument, and launches again. It holds no loop state and makes no orchestration decisions — every judgement the loop needs runs inside the workflow as an agent stage with a schema-typed return.

Design: [lsf-workflow](${CLAUDE_PLUGIN_ROOT}/docs/design/lsf-workflow.md), sections *Architecture*, *Escalation*, and *Relaunch target*. Workflow arguments and return shape: the header comment of `${CLAUDE_PLUGIN_ROOT}/workflows/lsf/implement.js`.

## Interface

### Requires

- **name**: the feature name; default paths derive from it (`docs/plans/<name>-breakdown.md`, `-decisions.md`, `-archive.md`, and `-signals/<plan SHA>.md`, one ledger file per launch).
- **architecture-document**: path, committed on the integration branch.
- **integration-branch**: the branch units merge into. The main checkout must be on it.
- **request-record**: the architecture brief (`docs/architecture/<name>-brief.md`), whose requirements and success criteria are what the relaunch table's request-record clause refers to; passed as `requestRecord`, defaulting to that path.
- **unscribed** (optional): ledger rows a previous return or a Phase 2 re-entry prompt carried; passed as `args.unscribed` unchanged.
- One of:
  - a committed **decomposition-plan** (first launch);
  - a previous run's **return value** or run id (relaunch after a stop, or recovery).

### Produces

- **run report**: per launch, what the requester sees — the escalation's `decision_needed` on a stop, the digest on every run, and the relaunch arguments used.
- **decisions**: requester answers, passed as `args.decisions`; the workflow's own scribe commits them to the decision log. This skill never writes the log.

### Consumes

- **decomposition-plan** (from `phronithm:breakdown`, via `phronithm:large-scale-feature` Phase 2): must carry a `yaml lsf-plan` block ([plan graph](${CLAUDE_PLUGIN_ROOT}/docs/plan-graph.md)).
- **decision-log** (`docs/plans/<name>-decisions.md`): read only to find the next decision id and the SHA to pass.

### Complexity drivers

- Low for the session itself: it reads one return per launch and asks the requester a bounded set of questions.
- The workflow's cost is per unit, and a wave carries fixed overhead of the same order as one unit. Keep `maxUnitsPerWave` at the parallelism the machine and the requester's attention can absorb, not lower.

## Workflow

### Startup

Follow the [pre-flight](${CLAUDE_PLUGIN_ROOT}/docs/pre-flight.md) check. Do not continue until it passes.

Then confirm the workflow's preconditions from the shell, not from memory:

1. `git rev-parse --abbrev-ref HEAD` is the integration branch. The workflow stops at load otherwise.
2. The plan at the SHA to launch from (`git rev-list -1 <branch> -- <plan-path>` on a first launch) has a `yaml lsf-plan` block with a closing fence: `git show <sha>:<plan-path> | grep -n 'lsf-plan'`, then check the fence after it. A plan without the block needs `phronithm:breakdown` re-plan mode run once to add it, committed, before launch.
3. The decision log, if it exists, has a `yaml lsf-decisions` block with a closing fence (`tail -n 3`), and its SHA is `git rev-list -1 <branch> -- <path>`.
4. `git worktree list` shows no stale `.claude/worktrees/wf_*` entries with unmerged work from a run the requester has abandoned. Leave entries from a run being resumed. A relaunch after a Phase 2 re-entry resumes nothing; its worktrees are stale unless the reconciliation record names them.
5. `wc -c CLAUDE.md` in the target project. This step never blocks a launch; it is a report. The harness injects the file into every agent the workflow spawns, so its size is part of every agent's floor before the first read: roughly 2.5k tokens per 10 KB. Report the size and the tokens it implies per agent; above 40 KB — about 10k tokens on every agent — say so. Do not trim it — that is the project's call, outside this skill.

Launch by `name: 'phronithm:lsf-implement'` when the plugin is installed; otherwise by `scriptPath: ${CLAUDE_PLUGIN_ROOT}/workflows/lsf-implement.js`, absolute, since the session's cwd is the target project.

### Launch

Call the Workflow tool with the arguments as JSON values, not strings. First launch:

```
name: <name>
planSha: <plan SHA>
architectureDoc: <path>
integrationBranch: <branch>
maxUnitsPerWave: <n>
maxWaves: <n>              # default 20; a wave-cap return relaunches
decisionLogSha: <SHA>        # only when the log exists
requestRecord: <path>        # the architecture brief; default docs/architecture/<name>-brief.md
unscribed: <rows>            # only after a Phase 2 re-entry whose prompt carried them
```

Record the run id from the tool result. The workflow runs in the background and re-invokes this session when it returns; do not poll. `/workflows` shows it in flight.

### Read the return

The return is a JSON object whose keys are in presentation order: `status`, `workflow`, `escalation` (stopped runs only), `digest`, `planSha`, `integrationBranch`, `waves`, `ledgerPath`, `unscribed`. Read `status` first. A return too large to read whole goes to the scratchpad and is read with a script by key; never infer the outcome from the tail of the output.

| `status` | Meaning |
|---|---|
| `stopped` | The stop rule fired. `escalation.decision_needed` is the agenda. |
| `complete` | Every unit merged, termination ran, every standing obligation discharged or carried. |
| `wave-cap` | `maxWaves` reached with units remaining; `note` says how many. Relaunch from the return's top-level `planSha` with no decisions. |

A tool result with no return value, or a task notification reporting the run died, is a crash: go to Recovery.

### Present

On every run, in this order:

1. **Status**, and on a stop the stage and wave it stopped at.
2. **`escalation.decision_needed`**, verbatim, one item per question. Each carries a `d<n>` id, the question (the signal's kind, source, stage, unit, and statement) and what it affects. An `architecture-invalidated` question includes the gate's proposed amendment; present the five fields as the gate returned them.
3. **The digest**: counts by kind, source, and disposition; then `attention` in full — architecture notes and amendments, unfixed Significant and Minor finding counts per unit, distressed units, and every direct edit with its path. Direct edits passed no gate; name each so the requester can review it. Ask the requester whether any architecture note amounts to an invalidation the gate did not call; if one does, treat the run as a first-row stop of the relaunch table and go to *Return to Phase 2*.
4. **`unscribed`**: ledger rows no scribe committed. If non-empty, say so and include the rows; until the next launch carries them, they exist nowhere else. Name `ledgerPath` too: each launch writes its own file under `docs/plans/<name>-signals/`, so carried rows land in the next launch's file, not the one they were raised in.
5. On `complete`, the digest's note: silence is not evidence.

Do not summarise a stop as a clean close, and do not rank or filter the agenda. The requester adjudicates; the session presents.

### Decide

Collect one decision per `decision_needed` item. Use AskUserQuestion only where the question has bounded options; most stops need the requester's free text. Each decision becomes one entry:

```
{ id: "D<next>", question: <the item's question>, decision: <the answer>, clears?: ["<unit>/<gate>"] }
```

`D<next>` continues the decision log's numbering. `clears` is set only for a `no-ready-unit` stop on an organisational, release, or data gate the requester has cleared; it is the only way such a gate becomes cleared. Environment gates are re-probed against their pin and never appear in `clears`.

Some decisions are not the plan's to carry out. Before relaunching, classify each stop by the relaunch table below; a stop whose answer changes the architecture document's decisions or the request record leaves this skill.

### Relaunch

| Stop kind | Relaunch |
|---|---|
| `architecture-invalidated`; `plan-defect-unabsorbed` where the answer changes a decision in the architecture document or the request record | Return to `phronithm:large-scale-feature` Phase 2 (below) |
| `wave-cap` return (no escalation) | `phronithm:lsf-implement` again, `planSha` from the return's top-level `planSha`, no decisions |
| Everything else: `cycle`, `unit-over-budget`, `gate-non-convergence`, `merge-conflict-unresolvable`, `merge-regression`, `environment-gate-drift`, `no-ready-unit`, `standing-obligation-undischarged`, `agent-null`, `precondition`, `invalid-plan`, `unserialised-overlap`, `done-condition-unevaluable` (the decision is the rewritten condition), and `plan-defect-unabsorbed` otherwise | `phronithm:lsf-implement` again, with the decisions |

A mixed agenda takes the first matching row: the run returns to Phase 2, and every other decision travels in the startup prompt — one `Decision:` line per item — for the Phase 2 session to carry into the plan through `phronithm:breakdown`.

A relaunch is a fresh Workflow call, not a resume:

```
name, architectureDoc, integrationBranch, maxUnitsPerWave, maxWaves, requestRecord   # as before
planSha: <escalation.plan_sha>
decisions: [ ...the entries above... ]
decisionLogSha: <current SHA, when the log exists>
unscribed: <the previous return's unscribed array, verbatim, when non-empty>
```

The relaunch starts a new ledger file, named by its `planSha`; the previous launch's file stays as it was returned.

`escalation.plan_sha` is the plan to relaunch from. After a stop at a re-plan, or a wave stop with units merged and an item for the re-plan to absorb, it is the re-plan's own commit and already carries the amendments absorbed; `pre_replan_sha` is then the plan as it stood after the wave's merge — completions marked, entries archived — and before the re-plan's amendments; it is not relaunched. A wave stop with nothing to absorb skips the re-plan: `plan_sha` is the merge's commit when something merged, or the plan unchanged when nothing did, with no `pre_replan_sha`. The workflow commits the decisions to the log, runs a re-plan over them, and loads the SHA that re-plan returns; a decision the re-plan cannot carry out stops the run again before any unit starts, and that stop goes back to the requester.

An `environment-gate-drift` stop is not answered by a decision: either the environment is restored to the pin the plan records, and the relaunch carries no decision, or the requester re-pins the gate, and the decision says so for the re-plan to amend the plan's gate entry (amendment kind `gate-repin`).

A `precondition` or `invalid-plan` stop names a defect the session can usually see from the shell (wrong branch, a block without its fence, a scope pair with no edge). Fix the repo state through the tool that owns it — `phronithm:breakdown` re-plan mode for the plan, a decision for the decision log, a checkout for the branch — commit, and relaunch with the new SHA and no decision.

### Recovery

A run that died (usage limit, machine crash, killed) has its journal under the session's transcript directory. In order of preference:

1. **Resume**: `Workflow({ scriptPath | name, resumeFromRunId: <run id> })` with the same args. Every unchanged agent call replays from the journal; the first call whose prompt differs runs live. Stop the prior run first with TaskStop if it is still listed.
2. **Seed**: when the journal is gone but a stage's return survives in an agent transcript, pass `args.seed: { "<agent label>": { result, commits } }` on a fresh launch. The label is the stage's display label (`plan:<unit>`, `implement:<unit>`, `merge:w<n>`, …). Seed only a return produced against the plan SHA being launched; the caller vouches for that.
3. **Fresh launch** from the last committed plan SHA. Units whose branches merged are marked complete in the plan; units mid-flight are re-run. Unmerged `worktree-wf_*` branches from the dead run hold that work and can be seeded from or discarded.

Never finish a unit, a re-plan, or a termination stage by hand to recover a run. See *What the session never does*.

### Return to Phase 2

A stop in the first row of the relaunch table is handled in a fresh session by `phronithm:large-scale-feature` Phase 2's re-entry step. The startup prompt carries what the step needs. Present it in a fenced code block and wait:

```
Load the phronithm:large-scale-feature skill and re-enter Phase 2 after a phronithm:lsf-implement stop.
Architecture document: docs/architecture/<name>.md
Architecture brief (request record): docs/architecture/<name>-brief.md
Decomposition plan: docs/plans/<name>-breakdown.md @ <escalation.plan_sha>
Integration branch: <branch>
Signal ledger: <ledgerPath>
Stop: <kind>, wave <n>, stage <stage>
Decision: <the requester's answer; one line per agenda item>
Invalidation: <the gate's five fields verbatim — section, decision, premise, evidence, amendment — when the stop is architecture-invalidated>
Unscribed ledger rows: <the return's unscribed array verbatim, or none>
```

Entered from an architecture note rather than a gate stop: the plan is the return's top-level `planSha`, `Stop:` reads `architecture note (requester-judged), wave <the note's wave>`, the `Invalidation:` line is omitted, and one line is added — `Note: <the note's statement and referent, verbatim from the digest>`.

A fresh breakdown retires unit ids; Phase 2's re-entry step retires the decision log's stale `clears` entries before handing off, so the load has nothing to fail on. The relaunch of `phronithm:lsf-implement` against the new plan is a first launch in effect, with `unscribed` passed from the prompt.

### Termination

A `complete` return ends Phase 3 unless the requester judges an architecture note an invalidation (*Present*, step 3). Present the digest and the note, confirm `git log` on the integration branch shows the merge commits the return implies, and hand back to `phronithm:large-scale-feature`'s termination: the feature is complete when the requester accepts the digest.

## What the session never does

- **No unit work in the session.** Not a small one, not a documentation unit, not a termination meta-task. Every unit goes through the workflow's five stages; a unit too small to be worth a wave is still worth a wave.
- **No plan or architecture-document edits in the session.** A correction is a decision; the re-plan applies it (`amended`, `document-text`) or declines (`cannot`), and the ledger records which.
- **No decision-log writes in the session.** Decisions travel as `args.decisions`; the workflow's scribe commits them. Phase 2's re-entry step, not this skill, retires stale `clears` entries.
- **No ledger writes in the session.** `unscribed` rows go back to the next launch as `args.unscribed`, unchanged; the workflow's first scribe appends them.
- **No reading the outcome from partial output.** `status` and `escalation` come first in the return by design.

## Commit discipline

The session commits nothing in Phase 3; every commit is an agent's. See [commit discipline](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#commit-discipline) for what those agents follow.
