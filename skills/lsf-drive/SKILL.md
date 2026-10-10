---
description: "Drive Phase 3 of phronithm:large-scale-feature, through the lsf-runtime CLI or the Workflow tool: check the plan and repo are ready, launch phronithm:lsf-implement, read its return, present the stop or the digest, collect requester decisions, and relaunch. Use after the decomposition plan is committed, after every stop, and to recover a crashed run."
allowed-tools: Read, Grep, Glob, Bash, Workflow, TaskStop, AskUserQuestion, Skill, ToolSearch
---

# LSF Drive

The session side of the `phronithm:lsf-implement` workflow: a presenter and relauncher. It launches the workflow (on the Go runtime through the `lsf-runtime` CLI, or under the Workflow tool), reads what came back, shows the requester what needs a decision, records the decision as a workflow argument, and launches again. It holds no loop state and makes no orchestration decisions — every judgement the loop needs runs inside the workflow as an agent stage with a schema-typed return.

Design: [lsf-workflow](${CLAUDE_PLUGIN_ROOT}/docs/design/lsf-workflow.md), sections *Architecture*, *Escalation*, and *Relaunch target*; the CLI: [lsf-runtime](${CLAUDE_PLUGIN_ROOT}/docs/design/lsf-runtime.md), section *CLI*. Workflow arguments and return shape: the header comment of `${CLAUDE_PLUGIN_ROOT}/workflows/lsf/implement.js`.

## Interface

### Requires

- **name**: the feature name; default paths derive from it (`docs/plans/<name>-breakdown.md`, `-decisions.md`, `-archive.md`, and `-signals/<plan SHA>.md`, one ledger file per launch).
- **architecture-document**: path, committed on the integration branch.
- **integration-branch**: the branch units merge into. The main checkout must be on it.
- **request-record**: the architecture brief (`docs/architecture/<name>-brief.md`), whose requirements and success criteria are what the relaunch table's request-record clause refers to; passed as `requestRecord`, defaulting to that path.
- **CLI route**: a runtime host config (default `${XDG_CONFIG_HOME:-~/.config}/lsf-runtime/config.json`, or a path the requester gives; `lsf-runtime config init` writes a starter); a relaunch or recovery also needs the run's state dir, which `launch` prints.
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

**Route.** The CLI route runs the orchestrator on the Go runtime, each agent call in a container. Take it when a runtime host config exists: `${XDG_CONFIG_HOME:-~/.config}/lsf-runtime/config.json`, or a path the requester gives, passed as `-config` to every `launch` and `resume`. The CLI runs through its launcher, `${CLAUDE_PLUGIN_ROOT}/lsf-runtime/lsf-runtime`, written `lsf-runtime` below and always run by that full path. The launcher runs a build of the plugin's working tree when it has one, and otherwise downloads the pinned release on first use (network and `curl` needed once; offline, `LSF_RUNTIME_BIN` names a binary). Exit 125 with `lsf-runtime launcher:` on stderr, from any command, is the launcher's failure, not the runtime's: give it to the requester verbatim.

When no host config exists, or the one there still holds a `REQUIRED:` value, offer the requester a starter before the Workflow route: `lsf-runtime config init -plugin-dir ${CLAUDE_PLUGIN_ROOT} [-config <path>]` writes one (never over an existing file) and prints its path. The requester builds an image from `${CLAUDE_PLUGIN_ROOT}/lsf-runtime/container/Containerfile` with the project's toolchain and fills in the fields marked `REQUIRED:` (the image digest, and the credentials file when `~/.claude/.credentials.json` is absent); see the design's *Distribution*. `launch` refuses a config still holding a `REQUIRED:` value. With the config filled in, take the CLI route. Only when the requester declines, take the Workflow route.

A run keeps the route it started on: name it in the run report, and relaunch and recover on the same route. A host config created mid-run does not move a Workflow-route run, whose journal and `worktree-wf_*` branches the Go runtime cannot read. To move a run, the requester decides; it is then a fresh launch from the last committed plan SHA (Recovery, fresh launch).

Then confirm the workflow's preconditions from the shell, not from memory:

1. `git rev-parse --abbrev-ref HEAD` is the integration branch. The workflow stops at load otherwise.
2. The plan passes the pre-flight check: `${CLAUDE_PLUGIN_ROOT}/lsf-runtime/lsf-runtime validate -decisions "$(git rev-parse --show-toplevel)/<log path>" "$(git rev-parse --show-toplevel)/<plan path>"`, leaving out `-decisions` when the decision log (`docs/plans/<name>-decisions.md` by default) does not exist. It reads the working tree, so first `git status --porcelain -- <plan path> <log path>` must print nothing: an uncommitted edit is committed or discarded before launch. The exit status decides:
   - 1: each line on stdout is a problem, and any blocks the launch: give them to the requester for `phronithm:breakdown` re-plan mode, committed before launch.
   - 125 (the launcher cannot run): give the requester its stderr, then check by hand: `grep -n 'lsf-plan' <plan path>` finds the block with a closing fence after it, and the decision log, if any, ends its `yaml lsf-decisions` block with a closing fence (`tail -n 3`).
   - 2 or any other non-zero: the command's own failure: give the requester its stderr and ask how to proceed.

   The command also rejects a malformed `lsf-gates` block, which the Workflow script ignores.
3. The plan's SHA to launch from is `git rev-list -1 <branch> -- <plan path>` on a first launch, and the decision log's, if it exists, `git rev-list -1 <branch> -- <log path>`.
4. Workflow route: `git worktree list` shows no stale `.claude/worktrees/wf_*` entries with unmerged work from a run the requester has abandoned. Leave entries from a run being resumed. A relaunch after a Phase 2 re-entry resumes nothing; its worktrees are stale unless the reconciliation record names them.
   CLI route: `lsf-runtime status -name <name>`, run in the target repo (or `-state <dir>` of the run), prints the report whatever its exit: 0 for a driver `starting`, `running` or `ended`, 3 for `crashed` or `failed-to-start`, 4 for `none` (the expected result before a first launch), 2 for a bad command line. A driver `starting` or `running` blocks the launch: the run is live (see *Launch*). `crashed` or `failed-to-start` goes to Recovery first. Report any `leftovers`, `xferRefs` or `containers` it lists to the requester, with the remedies in *Relaunch*. The first launch on a host config is preceded by a dry run: the *Launch* command with `-dry-run` added. It runs the launch checks in a scratch state dir and stops before the first call, spending nothing; it passes when it exits 0, printing a summary whose `stop_reason` is `dry-run`. Any other result is a refusal to give the requester verbatim.
5. `git show-ref --verify refs/lsf/<name>/pause` (or the `pauseRef` to be passed) finds nothing. If the ref exists, the launch would pause at load: tell the requester and ask whether to delete it (see *Pause*) before launching.
6. `wc -c CLAUDE.md` in the target project. This step never blocks a launch; it is a report. The harness injects the file into every agent the workflow spawns, so its size is part of every agent's floor before the first read: roughly 2.5k tokens per 10 KB. Report the size and the tokens it implies per agent; above 40 KB — about 10k tokens on every agent — say so. Do not trim it — that is the project's call, outside this skill.

Workflow route: launch by `name: 'phronithm:lsf-implement'` when the plugin is installed; otherwise by `scriptPath: ${CLAUDE_PLUGIN_ROOT}/workflows/lsf-implement.js`, absolute, since the session's cwd is the target project.

### Launch

Call the Workflow tool with the arguments as JSON values, not strings. First launch:

```
name: <name>
planSha: <plan SHA>
architectureDoc: <path>
integrationBranch: <branch>
maxUnitsPerWave: <n>
maxWaves: <n>              # default 20; a wave-cap return relaunches
pauseRef: <ref>              # default refs/lsf/<name>/pause; see Pause
decisionLogSha: <SHA>        # only when the log exists
requestRecord: <path>        # the architecture brief; default docs/architecture/<name>-brief.md
unscribed: <rows>            # only after a Phase 2 re-entry whose prompt carried them
```

Record the run id from the tool result. The workflow runs in the background and re-invokes this session when it returns; do not poll. `/workflows` shows it in flight.

**CLI route.** The same arguments, as one JSON object on stdin, in the target repo, with a Bash timeout of 600000 ms:

```
lsf-runtime launch [-config <host config>] -plugin-dir ${CLAUDE_PLUGIN_ROOT} -args - <<'EOF'
{"name": "<name>", "planSha": "<plan SHA>", "architectureDoc": "<path>", "integrationBranch": "<branch>", ...}
EOF
```

It returns once the launch checks have passed or failed, within 5 minutes. Exit 0 prints `{"stateDir","pid","launch"}`: record `stateDir` and pass `-state <stateDir>` to every later command. Exit 1 is a refusal, its message on stderr (an old config, a failed launch check, a `claudeVersion` mismatch, a git older than 2.38, a running driver), except when stderr says the driver was left running (5 minutes passed, or an interrupt): after any exit 1, run `lsf-runtime status` with the same address. A `driver` `starting` or `running` is a live run, to `watch` as below; otherwise give the refusal to the requester verbatim. Then run `lsf-runtime watch -state <stateDir>` as a background Bash; the session is re-invoked when it exits, at the run's end. Do not poll. The driver runs in a session of its own and outlives this one: a later session runs `lsf-runtime status -state <stateDir>`, and runs `watch` again while `driver` is `starting` or `running`.

### Pause

When the requester asks to stop, pause, or hold a live run, set the pause ref in the target repo — the launch's `pauseRef`, or the default:

```
git update-ref refs/lsf/<name>/pause HEAD
git show-ref --verify refs/lsf/<name>/pause
```

Confirm the second command prints the ref, then tell the requester that units already started will finish, merge, and be re-planned first: the pause can take up to a wave. The requester may also set the ref from any shell or worktree; a ref dirties no working tree. On the CLI route, `lsf-runtime pause -state <stateDir>` sets the ref and verifies it, and `pause -lift` deletes it. Use TaskStop (CLI route: `lsf-runtime stop -state <stateDir>`. The first `stop` lets the calls in flight finish, which can take as long as the longest call, and a second cancels them. `stop` is refused while `driver` is `starting`; retry once `status` says `running`) only when the requester asks for an immediate halt and accepts losing the work in flight; that is a kill, and Recovery applies.

The merge, ledger, and plan-copy stages read the ref; one set after the last of them in a wave is seen the wave after. A stop outranks a pause and leaves the ref set. A launch with the ref present commits any `decisions` and `unscribed` rows it was given, runs the decision re-plan, then returns `paused` before any unit starts; its `planSha` is the decision re-plan's.

Only the requester lifts a pause. On their instruction, delete the ref and confirm it is gone before relaunching:

```
git update-ref -d refs/lsf/<name>/pause
git show-ref --verify refs/lsf/<name>/pause   # must fail
```

### Read the return

The return is a JSON object whose keys are in presentation order: `status`, `workflow`, `escalation` (stopped runs only), `digest`, `planSha`, `integrationBranch`, `waves`, `ledgerPath`, `unscribed`. Read `status` first. A return too large to read whole goes to the scratchpad and is read with a script by key; never infer the outcome from the tail of the output.

| `status` | Meaning |
|---|---|
| `stopped` | The stop rule fired. `escalation.decision_needed` is the agenda. |
| `complete` | Every unit merged, termination ran, every standing obligation discharged or carried. |
| `wave-cap` | `maxWaves` reached with units remaining; `note` says how many. Relaunch from the return's top-level `planSha` with no decisions. |
| `paused` | The pause ref was set (see Pause); `note` says how many units remain. Present, then wait for the requester to resume. |

A tool result with no return value, or a task notification reporting the run died, is a crash: go to Recovery.

**CLI route.** `watch` prints the status report (JSON), whose `driver` says how the launch ended:

| `driver` (exit) | Read |
|---|---|
| `ended` (0), `summary.stop_reason` `returned` | `outcome` is the return above |
| `ended` (0), any other `stop_reason` | a runtime stop, with no outcome to present: `summary.detail` and `summary.error` say why; see *Relaunch*, runtime stops |
| `crashed` (3), `failed-to-start` (3) | `driverLog` holds the driver's last output: go to Recovery. A `failed-to-start` report with `previous` also holds the last launch that ran: its `summary` and `outcome` are read as the rows above, and a relaunch starts from that outcome |

The report also lists the pause ref's state and what a fault left (`leftovers`, `xferRefs`, `containers`); present a non-empty one.

### Present

On every run, in this order:

1. **Status**, and on a stop the stage and wave it stopped at.
2. **`escalation.decision_needed`**, verbatim, one item per question. Each carries a `d<n>` id, the question (the signal's kind, source, stage, unit, and statement) and what it affects. An `architecture-invalidated` question includes the gate's proposed amendment; present the five fields as the gate returned them.
3. **The digest**: counts by kind, source, and disposition; then `attention` in full — architecture notes and amendments, unfixed Significant and Minor finding counts per unit, distressed units, and every direct edit with its path. Direct edits passed no gate; name each so the requester can review it. Ask the requester whether any architecture note amounts to an invalidation the gate did not call; if one does, treat the run as a first-row stop of the relaunch table and go to *Return to Phase 2*.
4. **`unscribed`**: ledger rows no scribe committed. If non-empty, say so and include the rows; until the next launch carries them, they exist nowhere else. Name `ledgerPath` too: each launch writes its own file under `docs/plans/<name>-signals/`, so carried rows land in the next launch's file, not the one they were raised in.
5. On `complete`, the digest's note: silence is not evidence.
6. On `stopped` or `complete`, whether the pause ref is still set (`git show-ref --verify`). A stop outranks a pause, so a pause the requester asked for may not have happened; ask whether to keep or delete the ref before any relaunch.

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
| `paused` return (no escalation) | Nothing until the requester resumes; then delete the ref (see *Pause*) and relaunch as for `wave-cap` |
| Everything else: `cycle`, `unit-over-budget`, `gate-non-convergence`, `merge-conflict-unresolvable`, `merge-regression`, `environment-gate-drift`, `gate-not-run`, `no-ready-unit`, `standing-obligation-undischarged`, `agent-null`, `precondition`, `invalid-plan`, `unserialised-overlap`, `skill-not-loaded`, `schema-violation`, `gate-omitted`, `stage-fault`, `branch-mismatch`, `done-condition-unevaluable` (the decision is the rewritten condition), and `plan-defect-unabsorbed` otherwise | `phronithm:lsf-implement` again, with the decisions |
| `uncommitted-at-return` | The requester commits or removes the paths the statement names; then again with no decision. The relaunch replays the call, which re-checks the paths and passes once they are clean |
| `declared-gates-changed` (Go runtime) | Show the requester the old and new block (both are in the stop's statement); an agent made the change, and a new `hosts` entry widens the calls' network. Only on the requester's explicit approval, again at the escalation's plan SHA with no decisions. Otherwise the requester commits the block restored and the relaunch is at that commit's SHA (Startup step 3). A decision cannot change the block |

A mixed agenda takes the first matching row: the run returns to Phase 2, and every other decision travels in the startup prompt — one `Decision:` line per item — for the Phase 2 session to carry into the plan through `phronithm:breakdown`.

A relaunch is a fresh Workflow call, not a resume (CLI route: `lsf-runtime launch -args` with the same keys, never `resume`):

```
name, architectureDoc, integrationBranch, maxUnitsPerWave, maxWaves, requestRecord, pauseRef   # as before
planSha: <escalation.plan_sha>
decisions: [ ...the entries above... ]
decisionLogSha: <current SHA, when the log exists>
unscribed: <the previous return's unscribed array, verbatim, when non-empty>
```

The relaunch starts a new ledger file, named by its `planSha`; the previous launch's file stays as it was returned.

`escalation.plan_sha` is the plan to relaunch from. After a stop at a re-plan, or a wave stop with units merged and an item for the re-plan to absorb, it is the re-plan's own commit and already carries the amendments absorbed; `pre_replan_sha` is then the plan as it stood after the wave's merge — completions marked, entries archived — and before the re-plan's amendments; it is not relaunched. A wave stop with nothing to absorb skips the re-plan: `plan_sha` is the merge's commit when something merged, or the plan unchanged when nothing did, with no `pre_replan_sha`. The workflow commits the decisions to the log, runs a re-plan over them, and loads the SHA that re-plan returns; a decision the re-plan cannot carry out stops the run again before any unit starts, and that stop goes back to the requester.

An `environment-gate-drift` stop is not answered by a decision: either the environment is restored to the pin the plan records, and the relaunch carries no decision, or the requester re-pins the gate, and the decision says so for the re-plan to amend the plan's gate entry (amendment kind `gate-repin`).

A `gate-not-run` stop, whether a stage or the runtime's own run of the declared gates raised it, is not answered by a decision either: a relaunch re-runs the unit and meets the same gate. The requester changes the environment or the project so the gate completes (within a stage; with declared gates, within the gate's timeout), and the relaunch carries no decision. The unit is not finished by hand (*What the session never does*).

A `stage-fault` from a code stage reads `<stage> failed: <reason>`. Most reasons are cleared by hand as the reason says, then relaunched with no decision. Two need more:

- **Integrity stop**: `stage commit <sha> left <path> with content its Lsf-Content trailer does not match`. It repeats at every launch until the requester lands a repair commit: a new commit restoring the content, without an `Lsf-Content` trailer (not `--amend` or `--fixup`, which keep it). A hook that rewrites or stages files (a formatter, an end-of-file fixer) causes it, so the project also exempts the paths the code stages commit (the decision log, `docs/plans/<name>-signals/`, `docs/plans/<unit>-completion.md`) from such hooks; otherwise the next launch stops again.
- **A requester edit in the main checkout** stops the run at the next fast-forward (`the main checkout has uncommitted changes to …`, or a merge, cherry-pick or revert `in progress in the main checkout`). The requester stashes, discards, finishes or aborts it, and relaunches. Leave the main checkout alone while a run is live.

**Runtime stops (CLI route).** A launch that ends with a `stop_reason` other than `returned` has no outcome and no escalation. It is answered by clearing the cause, then `lsf-runtime resume -state <stateDir>`, never by a decision:

| `stop_reason` | Answer |
|---|---|
| `auth-failed` | The requester renews the credential (logs in again, so the host config's `proxy.credentialsFile` holds a live token); then `resume`. Claude Code 2.1.294 retries a 401 for about three minutes first |
| `runtime-error` | `summary.detail` names the fault; the requester clears it by hand (below); then `resume` |
| `signal-SIGTERM`, `signal-SIGINT`, `cancelled` | `resume`, when the requester wants the run continued |
| `max-calls`, `max-cost`, `max-wall-clock` | The requester raises the cap in the host config's `caps`, or not; then `resume` |
| `crashed` (the orchestrator errored or panicked); `driver` `crashed` or `failed-to-start` (no summary) | Give the requester `summary.error` or `driverLog`, and the state dir's `log.txt`; `resume` only on their instruction |

**Faults cleared by hand.** Each is raised as a `stage-fault` (`merge failed: …`, or the fast-forward's stage) or a `runtime-error` stop, and the requester clears it by the remedy its text gives, then relaunches (a stage-fault) or resumes (a runtime stop). Merge faults, the merge running in a merge clone `<stateDir>/merge/w<n>`:

- `returned <field> <sha> does not resolve to a commit` (`<field>` is a merge's `sha`, `planSha` or `architectureSha`): a history rewrite removed a recorded merge, and the journaled merge result replays it at every relaunch. Remove its journal record, `<stateDir>/journal/merge_w<n>-*.json`. Its siblings (`… is not a commit SHA`, `… is not on <branch>`, `returned a <status> merge of <unit> without its SHA`) replay the same way; the code gives them no remedy, so present them to the requester with the record's path.
- A stale `index.lock`: git's own error, after `merge failed:` or `git refused to fast-forward`. Remove the lock file once no git process holds it.
- `a merge is in progress in the main checkout` (or a cherry-pick or revert): finish or abort it. `a merge is in progress in the merge clone … that this stage did not start`: remove the clone.
- `after the merge of <unit> at <sha>, <path> is staged (a hook?)`: commit or unstage it. `<path> is staged in the merge clone`: remove the clone.
- `the merge of <unit> stopped with no conflict`: the project's `pre-merge-commit` hook, which the merge clone takes from the main repo, refused the merge. The requester fixes the hook or exempts the merge from it; removing the clone alone does not help. Then relaunch.
- A red gate result the environment caused (a merge `regressed` or `unresolvable` on a gate that should pass): the result replays from the journal. After fixing the environment, remove `<stateDir>/journal/gates_<tree prefix>-*.json`.

Clone faults:

- A writer clone or merge clone a fault left: `<dir> exists but …; remove it and relaunch`, `<dir> is stale (… moved since) but holds commits not fetched … or uncommitted changes; recover them, remove it and relaunch`, `merge clone <dir> …: remove it and relaunch`.
- A merge clone whose integration branch diverged from main's: `the merge clone's <branch> (…) and the main repo's (…) have diverged …; remove <dir> and relaunch, and the step adopts its merges … by their trailers`.
- A kept clone left on another branch: `<dir> exists but is not on <branch>; remove it and relaunch`, or at a fast-forward `the clone's HEAD … is not <branch>'s tip …: in the call's clone …, commit the work on <integration> or drop it, then relaunch`.
- A kept clone's branch that cannot fast-forward the integration branch (it moved since): `<integration> moved to <sha> while <tip> was made on an earlier tip; rebase those commits onto <integration> in the call's clone …, or drop them, then relaunch`.
- `the conflict call changed the merge clone's git config, hooks or info files; remove <dir> and relaunch`.
- A fetch-back failed by a branch the agent renamed: `<label>: fetching <branch> from its clone <dir>: …; the clone is kept`. Restore the branch name in the clone, or recover the work and remove the clone.
- A fetch-back refused because the branch's tree holds a gitlink: `fetching <branch> from its clone: <dir> holds a nested repository (gitlink <path> at <rev>) …; remove the nested repository from the branch and relaunch`. It is a `runtime-error`.

`lsf-runtime status` lists what faults leave: `leftovers` (`clone/`, `scratch/`, `merge/` and `*.partial` entries, and in `retain` the `.lsf-retain-*` staging and scratch directories in the plan's store). While `driver` is `starting` or `running` these include the run's own work in flight: present them, never remove them. A `retain` entry from a failed move follows its `retained-move-failed` record's by-hand remedy (a rerun of the cleanup step adopts the staging directory its record names); one no record names is the requester's to inspect and remove, `xferRefs` (`refs/lsf/<plan8>/xfer/w<n>` in the main repo) and `containers` (the state dir's label). Present them with the fault.

A `precondition` or `invalid-plan` stop names a defect the session can usually see from the shell (wrong branch, a block without its fence, a scope pair with no edge). Fix the repo state through the tool that owns it — `phronithm:breakdown` re-plan mode for the plan, a decision for the decision log, a checkout for the branch — commit, and relaunch with the new SHA and no decision.

### Recovery

CLI route: a run with no outcome (a runtime stop, a crash, a failed start) is resumed with `lsf-runtime resume -state <stateDir> [-config <host config>]`, after the cause is cleared (*Relaunch*, runtime stops). It replays the last launch's args against the current host config, and the journal under the state dir replays every completed call. For `crashed` or `failed-to-start`, first give the requester `driverLog` and the state dir's `log.txt`; resume only on their instruction. `resume` refuses after a returned outcome: that relaunches with `launch -args`. Seeding is `args.seed` in a fresh `launch -args`.

Workflow route: a run that died (usage limit, machine crash, TaskStop) has its journal under the session's transcript directory. In order of preference:

1. **Resume**: `Workflow({ scriptPath | name, resumeFromRunId: <run id> })` with the same args. Every unchanged agent call replays from the journal; the first call whose prompt differs runs live. Stop the prior run first with TaskStop if it is still listed.
2. **Seed**: when the journal is gone but a stage's return survives in an agent transcript, pass `args.seed: { "<agent label>": { result, commits } }` on a fresh launch. The label is the stage's display label (`plan:<unit>`, `implement:<unit>`, `merge:w<n>`, …). Seed only a return produced against the plan SHA being launched; the caller vouches for that. A seed's `pauseRequested` is ignored; a resumed run's replayed stages, by contrast, report the pause ref as it stood when they first ran, so a resume can pause again after the ref is deleted — relaunch once more.
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
- **No lifting a pause unasked.** Only the requester's instruction deletes the pause ref or relaunches a `paused` run.
- **No reading the outcome from partial output.** `status` and `escalation` come first in the return by design.

## Commit discipline

The session commits nothing in Phase 3; every commit is an agent's. See [commit discipline](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#commit-discipline) for what those agents follow.
