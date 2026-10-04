// Orchestrator body for the lsf-implement workflow: Phase 3 of
// phronithm:large-scale-feature as a wave loop. See
// docs/design/lsf-workflow.md, "Wave loop", "Escalation", "Persistence".
//
// `run(rt)` takes the Workflow runtime — { agent, parallel, log, args } — so
// the loop is testable with a stub. build.js inlines this file and the other
// modules into workflows/lsf-implement.js; that file is generated, this one
// is the source of truth.
//
// The plan graph is read from the plan's ```yaml lsf-plan block and gate
// clearances from the decision log's ```yaml lsf-decisions block
// (docs/plan-graph.md). A copy agent fetches the blocks; graph.js parses
// them. No agent interprets the plan's prose on the script's behalf.
//
// The script stops only on structural signals (merge, tests, gate cap,
// null returns, plan validity) and on two adjudicated ones: the wave gate's
// complete architecture invalidation and the re-plan's `cannot` verdict.
// Everything else agents report is recorded in the ledger and handed to the
// re-plan.
//
// args:
//   name                feature name; default paths derive from it
//   planSha             commit SHA of the decomposition plan to load (required)
//   planPath            default docs/plans/<name>-breakdown.md
//   architectureDoc     path (required)
//   architectureSha     the load-time pin; default the document's last commit
//                       on the integration branch. It advances to the
//                       merge stage's report after each wave's merge and to
//                       the copy stage's on each reload
//   requestRecord       the architecture brief; default docs/architecture/<name>-brief.md.
//                       Its requirements and success criteria are the request
//                       record the prompts name
//   integrationBranch   the main checkout must be on this branch (required)
//   decisionLogPath     default docs/plans/<name>-decisions.md
//   decisionLogSha      SHA of the decision log already committed, optional
//   decisions           [{ id, question, decision, clears? }] to commit before
//                       loading; `clears` lists `unit/gate` names. A re-plan
//                       stage then applies the decisions to the plan, and the
//                       run loads the SHA it returns
//   ledgerPath          default docs/plans/<name>-signals/<planSha>.md: one
//                       ledger file per launch, named by the plan SHA the
//                       launch started from
//   archivePath         default docs/plans/<name>-archive.md; where the merge
//                       moves a completed unit's entry and the re-plan moves
//                       a closed risk
//   config              overrides for scheduling DEFAULT_CONFIG
//   maxWaves            default 20
//   maxUnitsPerWave     cap on units started per wave, in execution order; default unbounded
//   pauseRef            default refs/lsf/<name>/pause. While this git ref
//                       exists the run returns `paused` at the next wave
//                       boundary — after the wave's merge, re-plan and ledger
//                       commits — or at load, after any decisions and carried
//                       rows are committed and before any unit starts. Set it
//                       with `git update-ref <ref> HEAD`; delete it with
//                       `git update-ref -d <ref>` before relaunching. The
//                       merge, ledger and plan-copy stages report it; a ref
//                       set after the last of them in a wave is seen next wave.
//                       A stop outranks a pause and leaves the ref set. A
//                       seeded return's report is ignored
//   unscribed           ledger rows a previous run returned under `unscribed`,
//                       passed back unchanged; a scribe appends them before
//                       anything else runs. They are not re-evaluated and
//                       do not count in this run's digest
//   seed                { "<agent label>": { result, commits } }: a stage return
//                       to use in place of spawning that agent — e.g. a unit
//                       plan salvaged from a run whose journal is gone (label
//                       plan:<unit>). Matched by label alone; the caller
//                       vouches that the seed was produced against the same
//                       plan SHA and inputs
//   tiers               per-role { model, effort } overrides; roles:
//                       critique (unit plan critique, code critique, lens
//                       review), gate (wave gate), planning
//                       (unit plan, re-plan, termination), mechanical
//                       (plan-block copy, ledger and decision-log appends),
//                       default (everything else)
//
// returns, in this order:
//   status              complete | stopped | wave-cap | paused
//   workflow            lsf-implement
//   escalation          stopped only: { workflow, stage, signals,
//                       decision_needed, plan_sha, pre_replan_sha?, wave? };
//                       decision_needed is what the session presents
//   digest              counts and attention items over the whole ledger
//   note                wave-cap and paused only: what stopped the run and
//                       how to relaunch
//   planSha, integrationBranch, waves
//   ledgerPath          the signal ledger, where every scribed row lives
//   unscribed           ledger rows no scribe committed (a null scribe return),
//                       including carried rows the scribe failed again on

import {
  stageSchema, MERGE_RESULT_SCHEMA, DISTRESS_SCHEMA, INVALIDATION_SCHEMA, AMENDMENT_KINDS, signal, escalation,
} from './schema.js';
import { loadGraph } from './graph.js';
import {
  DEFAULT_CONFIG, REPLAN_ITEM_KINDS, PLAN_SUPERSEDING_STOP_KINDS, validatePlan, readySet, blockedUnits, mergeOrder, wavePlan,
  nullReturnSignal, noCommitSignal, signalsFromReadings, signalsFromDocumentSize, signalsFromImplement, signalsFromFindings, signalsFromGate,
  signalsFromMerge, distressSignals, evaluateWave, digest, renderLedger, ledgerLabel, ledgerHeading,
} from './scheduling.js';

const WORKFLOW = 'lsf-implement';
const GATE_MAX_ROUNDS = 4; // critique gate hard cap
const DEFAULT_MAX_WAVES = 20;
// Size targets for the per-unit documents every later stage reads whole.
// Targets, not ceilings: the plan stage writes its document in one call and
// never measures or trims it (a ceiling turned each plan into an
// eighty-edit trim loop costing more than the readers it saved); the
// completion report is rendered by the script from the stage returns. The
// script records a document past twice its target (scheduling.js).
const UNIT_PLAN_TARGET_KB = 12;
const COMPLETION_REPORT_TARGET_KB = 8;
// Model and effort per role.
const DEFAULT_TIERS = Object.freeze({
  critique: { model: 'opus', effort: 'low' },
  gate: { model: 'opus', effort: 'medium' },
  planning: { model: 'opus', effort: 'high' },
  default: { model: 'opus', effort: 'low' },
  // Copy and append stages: the script has rendered the content or pinned
  // it to a SHA; the agent only moves it into the repo, and the parser
  // checks the result on the next load.
  mechanical: { model: 'haiku', effort: 'low' },
});
const tier = (ctx, role) => ctx.tiers[role] ?? ctx.tiers.default;

// Review lenses per unit type. Design units produce a decision record, which
// gets critique-design rather than lens review.
const LENSES_BY_TYPE = {
  foundation: ['general', 'error-handling', 'data-structures'],
  feature: ['general', 'error-handling'],
  integration: ['general', 'error-handling'],
  design: [],
  refactor: ['general', 'complexity'],
  'bug-fix': ['general', 'error-handling'],
  polish: ['general'],
  meta: ['general'],
};

// ---------------------------------------------------------------- schemas

const str = { type: 'string' };
const bool = { type: 'boolean' };
const strings = { type: 'array', items: str };
const obj = (properties, required = Object.keys(properties)) => ({ type: 'object', properties, required });
const arr = (items) => ({ type: 'array', items });

// Agents have composed SHAs from memory; the field says where the value
// comes from.
const SHORT_SHA = 'commit SHA as `git rev-parse --short HEAD` prints it after the commit, copied from that output';
const SHA_FIELD = { ...str, description: `The ${SHORT_SHA}` };
const COMMIT_FIELDS = { branch: str, sha: SHA_FIELD };

const DECISION_LOG_SCHEMA = stageSchema(obj({ path: str, sha: SHA_FIELD }));
const CLEANUP_SCHEMA = stageSchema(obj({ removed: { type: 'integer' }, kept: strings }));

const PAUSE_FIELD = { pauseRequested: { type: 'boolean', description: 'Whether the pause ref named in the prompt exists' } };

const COPY_SCHEMA = stageSchema(obj({
  ...PAUSE_FIELD,
  planBlock: { ...str, description: 'The plan document\'s ```yaml lsf-plan fenced block, verbatim, fences included' },
  decisionsBlock: { ...str, description: 'The decision log\'s ```yaml lsf-decisions fenced block, verbatim, fences included; empty string when there is no decision log' },
  headBranch: { ...str, description: 'Branch the main checkout is on' },
  architectureSha: { ...str, description: 'Commit SHA of the architecture document on the integration branch' },
}));

const UNIT_PLAN_SCHEMA = stageSchema(obj({
  ...COMMIT_FIELDS,
  path: str,
  bytes: { type: 'integer', description: 'Size of the committed plan file in bytes (wc -c)' },
  readings: arr(obj({
    clause: { ...str, description: 'The clause of the unit entry or architecture document that admitted more than one reading, or named something that does not exist as named' },
    reading: { ...str, description: 'The reading the plan takes' },
  })),
}));

const FINDING = obj({
  severity: { type: 'string', enum: ['Critical', 'Significant', 'Minor'] },
  statement: str,
  location: str,
});

const FINDINGS_SCHEMA = stageSchema(obj({
  findings: arr(FINDING),
  systemic: { ...bool, description: 'True when the findings indicate a structural defect iteration will not resolve' },
  summary: str,
}));

const FIX_SCHEMA = stageSchema(obj({
  ...COMMIT_FIELDS,
  applied: arr(obj({ finding: str, fix: str })),
  notApplied: arr(obj({ finding: str, reason: str })),
}));

const GATE_RUN_SCHEMA = obj({
  command: str,
  status: { type: 'string', enum: ['green', 'red', 'not-run'] },
  summary: { ...str, description: 'One line: counts, or what failed' },
});

const IMPLEMENT_SCHEMA = stageSchema(obj({
  ...COMMIT_FIELDS,
  gateProbe: arr(obj({ name: str, pin: str, observed: str, drifted: bool })),
  gates: { ...arr(GATE_RUN_SCHEMA), description: 'The project\'s gates — the test suite and whatever else its own instructions name (linters, type checks) — as last run at HEAD' },
  distress: DISTRESS_SCHEMA,
  deviations: { ...strings, description: 'Departures from the unit entry\'s scope or publishes, one statement each' },
  writesOutsideScope: { ...strings, description: 'Files changed that the unit entry\'s Scope line does not name' },
  deferrals: arr(obj({ statement: str, belongsTo: { ...str, description: 'The unit the deferred work belongs to' } })),
  discoveries: arr(obj({
    statement: str,
    affects: { ...str, description: 'The unit whose scope, dependencies, or done condition this changes, or the architecture section it bears on' },
  })),
  doneCondition: obj({ evaluable: bool, met: bool, evidence: { ...str, description: 'The evaluation, clause by clause where the done condition has clauses: what was checked and what it showed, citing tests and figures; not a narrative of the implementation. Rendered verbatim into the completion report' } }),
  standingObligations: arr(obj({ statement: str, status: str })),
  summary: { ...str, description: 'One paragraph: what the unit delivered. Rendered verbatim into the completion report' },
}));

const COMPLETION_SCHEMA = stageSchema(obj({ ...COMMIT_FIELDS, path: str }));

const MERGE_SCHEMA = stageSchema(obj({
  ...MERGE_RESULT_SCHEMA.properties,
  ...PAUSE_FIELD,
  planSha: { ...str, description: 'Plan SHA after completion entries were recorded' },
  architectureSha: { ...str, description: 'Commit SHA of the architecture document on the integration branch after this stage\'s last commit' },
}, [...MERGE_RESULT_SCHEMA.required, 'planSha', 'pauseRequested', 'architectureSha']));

const GATE_SCHEMA = stageSchema(obj({
  invalidations: arr(INVALIDATION_SCHEMA),
  notes: arr(obj({ statement: str, referent: { ...str, description: 'A document section, unit, symbol, or file' } })),
  amendments: {
    ...arr(obj({ unit: str, section: str, statement: { ...str, description: 'One clause: what the unit changed' } })),
    description: 'A merged unit\'s own change to the architecture document that moves no decision; an empty array when the document did not move',
  },
}));

const REPLAN_SCHEMA = stageSchema(obj({
  planSha: { ...str, description: 'Plan SHA after the amendments were committed; the plan SHA given when nothing changed' },
  amendments: arr(obj({ kind: { type: 'string', enum: AMENDMENT_KINDS }, unit: str, statement: str })),
  overBudget: arr(obj({
    unit: str,
    disposition: { type: 'string', enum: ['extracted', 'flagged', 'unrepaired'], description: 'extracted: a preceding design unit now carries the excess; flagged: the excess is wall-clock, not context, and the unit carries the rationale; unrepaired: neither' },
    statement: str,
  })),
  verdicts: arr(obj({
    id: str,
    verdict: { type: 'string', enum: ['amended', 'no-change', 'document-text', 'cannot'] },
    statement: { ...str, description: 'What was amended, why the plan already answers it, what wording in the architecture document was corrected, or why the plan cannot absorb it' },
  })),
  directEdits: arr(obj({ path: str, description: str })),
  summary: str,
}));

const TERMINATION_SCHEMA = stageSchema(obj({
  planSha: { ...str, description: 'Plan SHA after the appended units were committed; the plan SHA given when nothing changed' },
  appendedUnits: strings,
  directEdits: arr(obj({ path: str, description: str })),
}));

const OBLIGATIONS_SCHEMA = stageSchema(obj({
  obligations: arr(obj({
    statement: str,
    disposition: { type: 'string', enum: ['discharged', 'carried', 'undischarged'] },
    carrier: { ...str, description: 'Where the obligation is now recorded outside this plan: file and row or section; empty unless carried' },
    evidence: str,
  })),
}));

const SCRIBE_SCHEMA = stageSchema(obj({ path: str, sha: SHA_FIELD }));
const LEDGER_SCHEMA = stageSchema(obj({ path: str, sha: SHA_FIELD, ...PAUSE_FIELD }));

// An append stage's return names the commit it made; an empty SHA is no commit.
const committed = (r) => Boolean(r.result.sha?.trim());

// ---------------------------------------------------------------- prompts

const RETURN_FIELDS = `
Besides \`result\`, return \`commits\`: every commit you made, branch and SHA; an empty array when none. A SHA is what \`git rev-parse --short HEAD\` prints after the commit, copied from that output. Nothing else: no questions or remarks outside the fields named. The return is one flat object.`;

// The stages that run in the main checkout at a wave boundary report the
// pause ref; the script acts on it at the loop top.
const pauseClause = (ctx) => `Run \`git show-ref --verify --quiet ${ctx.pauseRef} && echo present || echo absent\` and return \`pauseRequested\` true exactly when it prints \`present\`. Do not create, delete, or read the ref beyond that.`;

// Commit message form for the append stages: the subject alone on the
// first line, a blank line, then any trailers.
const commitMessage = (subject) => `The commit message is the subject line "${subject}", then a blank line, then any trailer lines; nothing on the subject line but the subject.`;

// The main checkout is shared with the requester, who may edit it while the
// run is live. A stage there commits only the files it wrote, by name, so an
// edit it did not make is never swept into its commit or discarded.
const ownFilesOnly = (what) => `The main checkout may hold uncommitted changes that are not yours: the requester can edit it while this run is live. Commit only the files this stage wrote (${what}), naming them: \`git add -- <paths>\`, then \`git commit -m <message> -- <paths>\`, options before the \`--\`, whose pathspec keeps everything else out of the commit. If the commit fails, return an empty SHA; never retry it without the pathspec. Never run \`git add -A\`, \`git add .\`, \`git add -u\`, \`git commit -a\`, \`git stash\`, \`git clean\` or \`git reset --hard\` in the main checkout, and never stage, revert, or discard a change you did not make.`;

function header(ctx, plan) {
  const lines = [
    `Feature: ${ctx.name}`,
    `Decomposition plan: ${plan.path} @ ${plan.sha}`,
    `Architecture document: ${ctx.architectureDoc} @ ${ctx.architectureSha ?? 'HEAD'}`,
    `Request record: ${ctx.requestRecord} (the architecture brief; its requirements and success criteria)`,
    `Decision log: ${ctx.decisionLogSha ? `${ctx.decisionLogPath} @ ${ctx.decisionLogSha}` : 'none'}`,
    `Integration branch: ${ctx.integrationBranch}`,
    `Read documents at the stated SHA (\`git show <sha>:<path>\`), not the working tree.`,
  ];
  return lines.filter(Boolean).join('\n');
}

function worktree(base) {
  return `You are running in a git worktree of the repository. Before anything else run \`git reset --hard ${base} && git clean -fd\` so the worktree matches that branch exactly, with no leftover untracked files from an earlier attempt. Make all changes and commits here; never touch other worktrees or the main checkout. Report your branch (\`git rev-parse --abbrev-ref HEAD\`) and HEAD SHA in \`result\`.`;
}

// What the unit entry's fields mean to a stage. Scope is read by the
// scheduler for overlap between units in flight; it is not a permission
// list, and a stage that treats it as one reports a defect that is not there.
const ENTRY_SEMANTICS = `The entry's Scope line declares the files this unit writes that another unit in flight might also write; the scheduler reads it for overlap. It does not limit what the work may edit. A file the work needs to change outside the Scope line is changed, and reported as such where a stage asks for it — never treated as a question. A clause of the entry or of the architecture document that admits more than one reading, or names something that does not exist as named (a file, a test, a function), is read the plainest way consistent with the code; the unit plan records the reading and later stages hold to it.`;

// The unit entry and the obligations naming the unit, by extraction command
// rather than a read of the whole plan: the plan is large and the stage
// needs one section of it. The prompt embeds the SHA, so the commands are
// stable across a wave.
function unitBlock(plan, unit) {
  return `Work unit: ${unit.id} (type: ${unit.type})
Unit entry: the \`### ${unit.id}\` section under \`## Work Units\` in ${plan.path} @ ${plan.sha}. Read it first, with this command verbatim, and take it as requirement text:
\`git show ${plan.sha}:${plan.path} | sed -n '/^### ${unit.id}$/,/^##/p'\`
Standing obligations naming the unit, with this command:
\`git show ${plan.sha}:${plan.path} | sed -n '/^## Standing Obligations/,/^## /p' | grep -n '${unit.id}'\`
Do not read the rest of the plan; the entry and the obligations are what this stage needs from it. ${ENTRY_SEMANTICS}`;
}

function decisionLogPrompt(ctx, decisions) {
  return `${header(ctx, { path: ctx.planPath, sha: ctx.planSha })}

Append the following requester decisions to ${ctx.decisionLogPath} and commit on ${ctx.integrationBranch}. The file holds one \`\`\`yaml lsf-decisions fenced block; each decision is one entry in its \`decisions\` sequence with \`id\`, \`date\` (today, \`date -I\`), \`question\`, \`decision\`, and \`clears\` (a flow sequence of \`unit/gate\` names; omit the key when empty). The entries go inside the fenced block as the last items of the \`decisions\` sequence, before the closing \`\`\` line, which stays the file's last fenced line; after writing, confirm with \`tail -n 3 ${ctx.decisionLogPath}\` that the closing fence is present. Quote any scalar containing \`: \`, \` #\`, or leading punctuation with double quotes. Nothing else in the block: no comments, anchors, or multi-line scalars. If the file does not exist, create it as:

# Decisions: ${ctx.name}

\`\`\`yaml lsf-decisions
decisions:
  - id: <id>
    date: <YYYY-MM-DD>
    question: <question>
    decision: <decision>
    clears: [<unit>/<gate>]
\`\`\`

Decisions to append:
${JSON.stringify(decisions, null, 2)}

${ownFilesOnly(ctx.decisionLogPath)} ${commitMessage(`docs(${ctx.name}): append decision(s) ${decisions.map((d) => d.id).join(', ')}`)} Return the file path and the ${SHORT_SHA}.
${RETURN_FIELDS}`;
}

function copyPrompt(ctx, sha) {
  const decisions = ctx.decisionLogSha
    ? `Run \`git show ${ctx.decisionLogSha}:${ctx.decisionLogPath}\` and return its \`\`\`yaml lsf-decisions fenced block the same way in \`decisionsBlock\`.`
    : 'There is no decision log; return an empty string in `decisionsBlock`.';
  return `${header(ctx, { path: ctx.planPath, sha })}

Copy stage. Run \`git show ${sha}:${ctx.planPath}\` and return, in \`planBlock\`, its \`\`\`yaml lsf-plan fenced block verbatim: from the opening fence line to the closing fence line inclusive, every character unchanged. A block with no closing fence is returned as it stands, to the end of the file, without adding one; the script's parser reports it. ${decisions} Also return the branch the main checkout is on (\`git rev-parse --abbrev-ref HEAD\`) and the commit SHA of ${ctx.architectureDoc} on ${ctx.integrationBranch} (\`git rev-list -1 ${ctx.integrationBranch} -- ${ctx.architectureDoc}\`).

Do not read, assess, or summarise the prose. Do not correct, reformat, or complete anything in the blocks; do not add units, edges, or gates. This stage copies; the script parses and checks the blocks itself. ${pauseClause(ctx)}
${RETURN_FIELDS}`;
}

function unitPlanPrompt(ctx, plan, unit) {
  return `${header(ctx, plan)}
${worktree(ctx.integrationBranch)}

${unitBlock(plan, unit)}

Write the implementation plan for this unit. The architecture document is the primary design input; the unit's done condition, adopted verbatim, is a success criterion; each standing obligation naming the unit is a requirement. Do not re-explore what the architecture document and unit entry settle. Keep it within the unit's size budget: about six steps, at most one open design decision. The plan is read whole by every critique round and by the implement stage; about ${UNIT_PLAN_TARGET_KB} KB is the target. Sections, in order:

1. **Goal** — one sentence: what the unit delivers. The unit entry, its done condition, and the standing obligations naming the unit travel with every stage prompt; cite them, do not copy them.
2. **Context** — the architecture sections relied on, by heading.
3. **Readings** — only the clauses of the entry or the architecture document that admitted more than one reading, or named something that does not exist as named, each with the reading taken and the evidence for it, one or two lines each. A clause with one plain reading is not listed. Return the same list in \`readings\`. Take the reading; do not raise a question and do not stop.
4. **Design** — the decisions the plan makes and why, citing the architecture sections they rest on rather than restating them; the one open design decision at most.
5. **Steps** — about six, each a heading and one paragraph: what it changes and how it is verified, in an order that keeps the project's gates green after every step.
6. **Verification** — how the done condition will be evaluated exactly as written, under the readings above.

The plan and the unit entry are all the implement stage works from; the plan needs nothing else. It carries no "Remaining phases" section and mandates no implementation summary document; this workflow's own stages run the plan single-shot, review the result, and write the unit's completion report. Write the whole file to docs/plans/${unit.id}-plan.md in one call and commit it. Do not trim it to length afterwards: length past the target is at most a Minor finding for the critique round, never a defect for this stage to repair. Return branch, SHA, path, the file's size in bytes (one \`wc -c\`, for the record), and the readings; not the plan text.
${RETURN_FIELDS}`;
}

function planCritiquePrompt(ctx, plan, unit, branch, round) {
  return `${header(ctx, plan)}

${unitBlock(plan, unit)}

Critique the implementation plan at docs/plans/${unit.id}-plan.md on branch ${branch} (\`git show ${branch}:docs/plans/${unit.id}-plan.md\`) with type critique-design. This is round ${round} of the critique gate. Pass bar: no Critical findings. Report Significant and Minor findings; they are recorded, not fixed. The plan's Readings section records how it read clauses that admitted more than one reading; a reading you judge wrong is a finding at the severity its consequence warrants, not a separate remark. Set \`systemic\` when the findings indicate a structural defect that iteration will not resolve. Continue through to completion — do not stop to wait for instruction.
${RETURN_FIELDS}`;
}

function fixPrompt(ctx, plan, unit, branch, subject, criticals, round) {
  return `${header(ctx, plan)}
${worktree(branch)}

${unitBlock(plan, unit)}

Critique gate round ${round} found these Critical findings against the ${subject}:

${JSON.stringify(criticals, null, 2)}

Fix each finding that is repairable within the ${subject}, then commit. A finding you decline to fix — because the repair would change the unit entry, the architecture document, or the request record, or because you cannot repair it — goes under \`notApplied\` with the reason. The run stops on a declined finding and the requester decides, so decline only when the repair is genuinely outside the ${subject}. ${subject === 'plan' ? `The plan is docs/plans/${unit.id}-plan.md on this branch. Repair the findings; do not shorten the plan to length.` : 'Run the tests the fix touches as you go, then the project\'s gates once at the end; a fix that breaks a test is not applied.'}
${RETURN_FIELDS}`;
}

function implementPrompt(ctx, plan, unit, planBranch) {
  return `${header(ctx, plan)}
${worktree(planBranch)}

${unitBlock(plan, unit)}

Implementation plan: docs/plans/${unit.id}-plan.md on this branch. Hold to its Readings section.

First re-probe every external gate in the unit entry against its plan-time pin: probe environment gates mechanically (binaries and versions, fonts, services, accounts, data sets); organisational and release gates are already cleared per the decision log and need no probe. Report each under \`gateProbe\`. If any gate has drifted, implement nothing — return the probe results with every other \`result\` field empty, zero, or false, and stop.

Otherwise execute the plan in full, single-shot, committing as you go with the project's commit discipline. There is no per-step critique here, and no implementation summary document: the completion report a later stage writes from your return is that record, so do not write \`docs/plans/${unit.id}-implementation.md\` even if the plan text mentions one. Run the project's gates — the test suite and whatever else the project's own instructions name (linters, type checks) — once at your final HEAD and report each under \`gates\` with its command, status, and a one-line summary; the review stages take this as the test result and do not re-run the suite. Then evaluate the done condition exactly as written, under the plan's readings, and report whether it is evaluable in position and whether it is met, with evidence. Record as they arise: deviations from the unit entry's scope or publishes; the files you changed that the Scope line does not name, under \`writesOutsideScope\`; deferrals — work in scope deliberately not implemented, naming the unit it belongs to; discoveries — anything you learned that changes another unit's scope, dependencies, or done condition, or that the architecture document states otherwise, naming the unit or the section; the status of each standing obligation the unit carries. Report execution distress honestly: whether context is near exhaustion, how many design decisions you took mid-implementation, the deviation and deferral counts.
${RETURN_FIELDS}`;
}

// The implement stage ran the gates once at HEAD; the reviewers read the
// result instead of re-running the suite, which in a compiled project cost
// more of their wall-clock than the review itself.
const renderGates = (gates) => (gates?.length ? gates.map((g) => `- \`${g.command}\` — ${g.status}${g.summary ? `: ${g.summary}` : ''}`).join('\n') : '- none reported');
function gateResults(gates) {
  return `The implement stage ran the project's gates at this branch's HEAD:
${renderGates(gates)}
Take that as the test and lint result. Do not run the whole suite, the project's CI job, or a lint pass; run a single targeted test only to check a specific claim of a finding, and name the test in the finding.${gates?.length ? '' : ' If no gate is listed above, run the project\'s gates once and treat the result as the implement stage\'s.'}`;
}

function codeCritiquePrompt(ctx, plan, unit, branch, round, gates) {
  return `${header(ctx, plan)}
${worktree(branch)}

${unitBlock(plan, unit)}

${gateResults(gates)}

Run the phronithm:critique skill with type critique-code over this branch's diff against ${ctx.integrationBranch} (\`git diff ${ctx.integrationBranch}...HEAD\`), reading surrounding code as needed. This is round ${round} of the critique gate. Pass bar: no Critical findings. Also evaluate the unit's done condition against this branch under the readings in docs/plans/${unit.id}-plan.md, from the code and the gate results above; a done condition that is not met, or a reading the code does not bear out, is a Critical finding. Set \`systemic\` when the findings indicate a structural defect that iteration will not resolve. Do not modify anything and do not spawn subagents. Continue through to completion — do not stop to wait for instruction.
${RETURN_FIELDS}`;
}

function lensReviewPrompt(ctx, plan, unit, branch, lenses, round, gates) {
  return `${header(ctx, plan)}
${worktree(branch)}

${unitBlock(plan, unit)}

${gateResults(gates)}

Run the phronithm:review skill over this branch's diff against ${ctx.integrationBranch} through these lenses: ${lenses.join(', ')}. Round ${round} of the review gate. Return each finding with a severity; a finding is Critical only when it would be wrong to merge without fixing it. Do not modify anything and do not spawn subagents.
${RETURN_FIELDS}`;
}

function designCritiquePrompt(ctx, plan, unit, branch, round) {
  return `${header(ctx, plan)}
${worktree(branch)}

${unitBlock(plan, unit)}

This is a design unit: its deliverable is a decision record, not code. Run the phronithm:critique skill with type critique-design over the documents this branch adds or changes relative to ${ctx.integrationBranch}. Round ${round} of the critique gate. Pass bar: no Critical findings. A done condition that is not met is a Critical finding. Do not modify anything and do not spawn subagents.
${RETURN_FIELDS}`;
}

// The completion report is rendered here from the implement and review
// returns — the gate and the re-plan read it by path — and a copy stage
// commits it on the unit branch. Findings left unfixed are ledger rows, so
// the report carries their counts and the ledger's path, not the rows.
const completionPath = (unit) => `docs/plans/${unit.id}-completion.md`;
const byteLength = (text) => (typeof TextEncoder === 'function' ? new TextEncoder().encode(text).length : text.length);

function renderCompletionReport(ctx, unit, wave, branch, impl, review) {
  const items = (xs, render) => (xs?.length ? xs.map((x) => `- ${render(x)}`) : ['None.']);
  const count = (severity) => (review.findings ?? []).filter((f) => f.severity === severity).length;
  const dc = impl.doneCondition ?? {};
  const outside = impl.writesOutsideScope ?? [];
  const probes = impl.gateProbe ?? [];
  const lines = [
    `# ${unit.id} — completion report`,
    '',
    `Unit \`${unit.id}\` (${unit.type}), wave ${wave}, branch \`${branch}\`.`,
    '',
    ...(impl.summary ? [impl.summary, ''] : []),
    '## Done-condition verdict',
    '',
    `**${dc.met ? 'Met' : 'Not met'}**${dc.evaluable === false ? ' (not evaluable as written)' : ''}. ${dc.evidence ?? ''}`.trimEnd(),
    ...(probes.length ? ['', `External gates re-verified: ${probes.map((g) => `${g.name} pinned ${g.pin}, observed ${g.observed}`).join('; ')}.`] : []),
    ...(impl.gates?.length ? ['', 'Gates at HEAD:', '', renderGates(impl.gates)] : []),
    '',
    '## Deviations and writes outside Scope',
    '',
    ...items(impl.deviations, (d) => d),
    ...(outside.length ? ['', `Written outside the Scope line: ${outside.map((f) => `\`${f}\``).join(', ')}.`] : []),
    '',
    '## Deferrals',
    '',
    ...items(impl.deferrals, (d) => `${d.statement} → \`${d.belongsTo}\``),
    '',
    '## Discoveries',
    '',
    ...items(impl.discoveries, (d) => `${d.statement} — affects \`${d.affects}\``),
    '',
    '## Standing obligations',
    '',
    ...items(impl.standingObligations, (o) => `${o.statement} — ${o.status}`),
    '',
    `Review: ${review.rounds} round(s); ${count('Significant')} Significant and ${count('Minor')} Minor finding(s) left unfixed are recorded in ${ctx.ledgerPath}, wave ${wave}.`,
    '',
  ];
  return lines.join('\n');
}

function completionScribePrompt(ctx, plan, unit, branch, text) {
  return `${header(ctx, plan)}
${worktree(branch)}

Copy stage. Write the Markdown between the markers below to ${completionPath(unit)}, every character unchanged, replacing the file if it exists. Do not reword, reorder, or drop anything, and change nothing else. Commit on this branch. ${commitMessage(`docs(${ctx.name}): ${unit.id} completion report`)} Return the branch, the path, and the ${SHORT_SHA}.

<<<REPORT
${text}REPORT>>>
${RETURN_FIELDS}`;
}

function mergePrompt(ctx, plan, wave, merges) {
  return `${header(ctx, plan)}

You are in the main checkout on ${ctx.integrationBranch}. Merge each unit branch below into ${ctx.integrationBranch}, in exactly this order, with a merge commit each (\`git merge --no-ff -m "merge(${ctx.name}): <unit id>"\`). For each unit report clean, resolved, unresolvable, or regressed, with the files involved and the merge SHA. Resolve a conflict only when the resolution is mechanical — both sides' changes apply without judgement about intent; otherwise abort that merge (\`git merge --abort\`), report it unresolvable, and continue with the next unit. A merge git refuses because it would overwrite an uncommitted change that is not yours is also unresolvable: report it with the files named, leave the change as it is, and continue with the next unit. Run the project's gates after each merge — the test suite and whatever else the project's own instructions name as a gate (linters, type checks). If a gate fails after a clean or resolved merge, undo that merge with \`git reset --merge <pre-merge tip of ${ctx.integrationBranch}>\`, which keeps uncommitted changes that are not yours (never \`git reset --hard\`, which discards them), report the unit as regressed with the failing test files, and continue with the next unit.

Then, for every unit merged clean or resolved, in one commit on ${ctx.integrationBranch}:
- Move its \`### <unit id>\` entry under \`## Work Units\` in ${plan.path} to ${ctx.archivePath}, appended verbatim as its own \`### <unit id>\` section under \`## Work Units\`. The archive is append-only; create it with the heading \`# Archive: ${ctx.name}\` and a \`## Work Units\` section if it does not exist.
- Leave in the entry's place the one-line stub \`### <unit id> — complete at <merge SHA>; entry archived to ${ctx.archivePath}; report <completion report path>\`.
- Append to the plan's \`## Plan Maintenance\` the entry \`### <date, date -I> — <unit id> complete\` with the single line \`Report: <completion report path> @ <merge SHA>. Ledger: ${ctx.ledgerPath} wave ${wave}.\` and nothing else: no restated report content.
- Set \`complete: true\` on the unit in the \`\`\`yaml lsf-plan block; the block is otherwise untouched.
${ownFilesOnly(`${plan.path} and ${ctx.archivePath}`)} Return the new plan SHA, and, after your last commit (or at the current tip if you made none), the commit SHA of ${ctx.architectureDoc} on ${ctx.integrationBranch} (\`git rev-list -1 ${ctx.integrationBranch} -- ${ctx.architectureDoc}\`) as \`architectureSha\`. Do not amend anything else in the plan; the re-plan stage does that. ${pauseClause(ctx)}

Wave ${wave} merge order:
${merges.map((m) => `- ${m.unit}: branch ${m.branch} (completion report ${m.completionPath})`).join('\n')}
${RETURN_FIELDS}`;
}

// The pin has already moved to the merged text; a unit's own change to the
// document is judged against the text the wave started from, so the change
// cannot become its own baseline. Each unit's change is its merge commit
// against the first parent, which leaves the requester's commits out.
// With no pre-wave pin, each unit's change is judged against its merge's
// first parent.
function amendedClause(ctx, merged, prevArch) {
  if (prevArch === ctx.architectureSha) return '';
  const doc = ctx.architectureDoc;
  const base = prevArch || 'the first parent of the unit\'s merge';
  const fallback = prevArch
    ? `\`git diff ${prevArch} ${ctx.architectureSha} -- ${doc}\` holds its change among any others, and attributing them is yours`
    : 'no baseline either; judge its change from its completion report';
  const diffs = merged.map((m) => `- ${m.unit} (${m.type}): ${m.mergeSha
    ? `\`git diff ${m.mergeSha}^1 ${m.mergeSha} -- ${doc}\``
    : `merge SHA not reported; ${fallback}`}`);
  return `
The architecture document changed during this wave, from ${prevArch || 'an unrecorded pin'} to the SHA above. Each merged unit's own change to it is listed below; an empty diff means the unit did not touch it, and changes outside these diffs are the requester's own and are not judged. Judge each unit's change against the document @ ${base}, the text the wave started from, reading that unit's completion report first. A change that records the decision a design unit was charged to settle, that settles a question the document left open to the unit that made it, or that corrects wording, goes under \`amendments\`: the unit, the section, and one clause on what changed. A change that alters what a section decided @ ${base} beyond that is an invalidation: \`decision\` is the text @ ${base}, \`evidence\` the merge SHA, \`amendment\` the text the unit wrote. Report each change once: an invalidation for a unit's change is not reported again as a premise invalidation of the amended text.
${diffs.join('\n')}
`;
}

function gatePrompt(ctx, plan, wave, merged, prevArch) {
  return `${header(ctx, plan)}

Wave ${wave} of Phase 3 has merged these units into ${ctx.integrationBranch}: ${merged.map((m) => m.unit).join(', ')}. Their completion reports are at ${merged.map((m) => m.completionPath).join(', ')}.

Answer one question: do this wave's results invalidate the architecture document at ${ctx.architectureDoc} @ ${ctx.architectureSha ?? 'HEAD'} — does any decision the document makes rest on a premise the merged code or the completion reports show to be false? Read the reports, then the merged code the reports and the document point at.
${amendedClause(ctx, merged, prevArch)}
For each such case return an invalidation with all five fields: \`section\` (the heading), \`decision\` (what the section decides, quoted or closely paraphrased), \`premise\` (what the decision rests on), \`evidence\` (file and line, test, or report line showing the premise false), and \`amendment\` (what the section should now say). An invalidation stops the run and is put to the requester with the amendment you propose, so fill every field; a case where you cannot say what the section should now say is a note, not an invalidation. A discrepancy that changes no decision — a stale line number, a signature widened compatibly, a count that does not match — is a note at most.

Return \`amendments\`: a merged unit's own change to the architecture document that moves no decision, as described above when the document moved; an empty array when it did not. Return under \`notes\` what the re-plan or the requester should know that is not an invalidation: an open decision whose trigger has fired and which no unit owns; a discrepancy between a document and the code; a deviation or deferral the reports disclose that bears on a unit not yet started. Each note names a referent.

Do not re-evaluate the units' done conditions or review findings: the unit stages did that at the bar this workflow sets (Critical findings fixed, Significant and Minor recorded), and the completion reports carry the outcome. Do not report this workflow's own process as a defect. Do not modify anything.
${RETURN_FIELDS}`;
}

// The compact Plan Maintenance entry a re-plan writes, and the Risk Registry
// archive rule: the plan records what changed and where, never restated
// statements. `heading` is the text after the date.
const maintenanceForm = (ctx, heading) => `Record the pass in \`## Plan Maintenance\` as one entry: the heading \`### <date, date -I> — ${heading}\`, then one line per amendment in the form \`<kind> <unit> — <one clause>\`, and nothing else — no restated items, reports, or decisions; the ledger holds those. A Risk Registry entry this pass closes — its risk retired by a completion or an amendment — moves to ${ctx.archivePath} under \`## Closed risks\`, cited by id and one clause; a risk that stands keeps its id and one clause. Parallel Streams, Critical Path, and Dependency Graph sections are not maintained: the script computes readiness and the wave plan from the \`\`\`yaml lsf-plan block.`;

// What the re-plan writes, and where an edit anywhere else goes instead.
const replanBoundary = (subject) => `The re-plan writes two documents: the plan, and under \`document-text\` the architecture document. ${subject} whose remedy is text elsewhere — a comment or docstring, a figure in a completion report or the debt register, a guide sentence, a register row to file — is not made by the re-plan: where a remaining unit's Scope covers the file, amend that unit's done condition (\`amended\`); where none does and the edit changes no behaviour and no test, return it under \`directEdits\` (path, what to change) with the verdict \`amended\` — a scribe stage makes and commits it after this re-plan and the ledger records each; where it changes behaviour or a test, add a unit (\`unit-added\`).`;

function replanPrompt(ctx, plan, wave, merged, conflicts, items, stops) {
  return `${header(ctx, plan)}

You are in the main checkout on ${ctx.integrationBranch}. Run the phronithm:breakdown skill in re-plan mode over ${plan.path} with the completion reports of wave ${wave}: ${merged.map((m) => m.completionPath).join(', ')}. Diff their deviations, deferrals, and discoveries against every remaining unit's scope, dependencies, and done condition; amend the plan; re-run the quality check over the remaining units, examining the size budget first where a completed unit showed distress. ${maintenanceForm(ctx, `re-plan wave ${wave} (<item ids>)`)} Treat each merge conflict below between units the plan claimed disjoint as a plan defect: record it and add the missing edge or scope correction. A file a unit wrote outside its Scope line is a Scope correction where a later unit shares the file.

Items this wave, each with an id — deferrals, discoveries, claimed-disjoint conflicts, the gate's notes, and distress. Each is yours to absorb: amend the plan (a Scope or Publishes line, a done condition, an edge, the execution order, a unit added or removed), or show the plan as it stands already answers it. Return a verdict for every id: \`amended\`, \`no-change\` (with what in the plan answers it), \`document-text\`, or \`cannot\` (with why the plan cannot absorb it — the answer changes a decision in the architecture document or the request record, or needs the requester). An id you do not answer is treated as \`cannot\` and stops the run.

\`document-text\` is for a gate note whose only remedy is wording in the architecture document and that moves no decision — a resolved unknown still marked open, a measured figure replacing an estimate, a named accessor or signature the code settled. Make that correction in the document, commit it, and put the section and the new wording in the verdict statement. A correction that would change what a section decides is not text; that is \`cannot\`.

${replanBoundary('An item')}
${items.length ? JSON.stringify(items, null, 2) : 'none'}

Merge conflicts: ${conflicts.length ? JSON.stringify(conflicts, null, 2) : 'none'}
${stops.length ? `
The run stops after this re-plan on the signals below; they go to the requester. Do not amend the plan for them — no edge, scope, or order change on their account — and leave every unit they name as it is: its branch is unmerged and it stays incomplete. Absorb the merged units' reports as above.
${JSON.stringify(stops.map(({ kind, unit, stage, statement, referent }) => ({ kind, unit, stage, statement, referent })), null, 2)}
` : ''}
Every amendment to a unit's type, dependencies, scope, publishes, gates, or completion, and to the execution order, is made in both the unit's entry and the plan's \`\`\`yaml lsf-plan block, which must agree; the script reads the block. Commit the amended plan on ${ctx.integrationBranch}. ${ownFilesOnly(`${plan.path}, and ${ctx.architectureDoc} if you made a \`document-text\` correction`)} Return the new plan SHA; each amendment with its kind (${AMENDMENT_KINDS.join(', ')}), unit, and statement; a verdict per item id; \`directEdits\` (an empty array when none); and every remaining unit over its size budget with its disposition: \`extracted\` (a preceding design unit now carries the excess), \`flagged\` (the excess is wall-clock, not context, and the unit carries the rationale — breakdown's size budget discharges it), or \`unrepaired\`. A flag already on the unit from an earlier pass stands unless the unit's scope has grown since; report it as \`flagged\`. Do not answer whether the architecture document still stands — the gate did. Do not select or execute any unit.
${RETURN_FIELDS}`;
}

function decisionReplanPrompt(ctx, plan, decisions) {
  return `${header(ctx, plan)}

You are in the main checkout on ${ctx.integrationBranch}. The requester has answered the questions the last run stopped on; the answers are in the decision log at the SHA above. Run the phronithm:breakdown skill in re-plan mode over ${plan.path} with these decisions as the input: apply each to the plan (a Scope or Publishes line, a done condition, an edge, the execution order, a unit added or removed), and re-run the quality check over the remaining units. ${maintenanceForm(ctx, `decisions ${decisions.map((d) => d.id).join(', ')}`)} Return a verdict for every decision id: \`amended\`, \`no-change\` (with what in the plan already reflects it), \`document-text\` (the decision only corrects wording in the architecture document; make the correction, commit it, and state the section), or \`cannot\` (with why the plan cannot carry it out — it changes a decision in the architecture document or the request record, or is not a plan matter). An id you do not answer is treated as \`cannot\` and stops the run. Report any remaining unit over its size budget with its disposition — \`extracted\`, \`flagged\` (a flag with wall-clock rationale, standing or new), or \`unrepaired\`.

${replanBoundary('A decision')}
${JSON.stringify(decisions, null, 2)}

Every amendment to a unit's type, dependencies, scope, publishes, gates, or completion, and to the execution order, is made in both the unit's entry and the plan's \`\`\`yaml lsf-plan block, which must agree; the script reads the block. Commit on ${ctx.integrationBranch}. ${ownFilesOnly(`${plan.path}, and ${ctx.architectureDoc} if you made a \`document-text\` correction`)} Return the new plan SHA (the SHA given when nothing changed), each amendment with its kind (${AMENDMENT_KINDS.join(', ')}), unit, and statement, a verdict per decision id, and \`directEdits\` (an empty array when none). Do not select or execute any unit.
${RETURN_FIELDS}`;
}

function terminationPrompt(ctx, plan) {
  return `${header(ctx, plan)}

You are in the main checkout on ${ctx.integrationBranch}. Every work unit in ${plan.path} is complete. Read its Meta-Tasks and Standing Obligations sections. Turn each substantial meta-task (integration testing, tuning infrastructure, anything needing its own implementation plan) into a work unit entry of type meta with a verifiable done condition, scope, dependencies, and gates, following phronithm:breakdown's unit conventions; append these to the plan's Work Units, to its execution order, and to its \`\`\`yaml lsf-plan block (entry and block must agree; the script reads the block), record the amendment in Plan Maintenance, and commit. ${ownFilesOnly(plan.path)} List each small meta-task (documentation updates and the like) under \`directEdits\` with the path and what to change; do not make those edits. Return the new plan SHA and the ids of the appended units. If no meta-task remains, return the plan SHA given, unchanged.
${RETURN_FIELDS}`;
}

function directEditsPrompt(ctx, plan, what, edits) {
  return `${header(ctx, plan)}

You are in the main checkout on ${ctx.integrationBranch}. Make these small edits, which the ${what} returned as outside the plan; commit them with the project's commit discipline, and return the path of the plan and the final commit SHA. ${ownFilesOnly('the paths the edits below name')}

${JSON.stringify(edits, null, 2)}
${RETURN_FIELDS}`;
}

function obligationsPrompt(ctx, plan) {
  return `${header(ctx, plan)}

Every work unit and meta-task in ${plan.path} is complete. For each standing obligation in the plan's Standing Obligations section, return its disposition with the evidence, from the completion reports and ${ctx.integrationBranch}: \`discharged\` — every carrying unit did what the obligation asks, cited; \`carried\` — the obligation's own text places its discharge outside this plan, and \`carrier\` names the file and row or section where it is now recorded outside this plan (a debt-register row, another plan's Standing Obligations); \`undischarged\` — neither. An obligation recorded nowhere outside this plan is \`undischarged\` whatever its text says. Do not modify anything.
${RETURN_FIELDS}`;
}

function cleanupPrompt(ctx, plan, wave) {
  return `${header(ctx, plan)}

Cleanup stage, wave ${wave}. You are in the main checkout on ${ctx.integrationBranch}. Remove the worktrees this workflow's finished agents left behind. For each entry of \`git worktree list --porcelain\` whose path is under \`.claude/worktrees/\` and whose branch is named \`worktree-wf_*\` and is fully merged into ${ctx.integrationBranch} (it appears in \`git branch --merged ${ctx.integrationBranch}\`): run \`git worktree remove --force <path>\`, then \`git branch -d <branch>\`. Never touch the main checkout, a worktree on any other branch, or a branch not fully merged — those hold work still in flight. Finish with \`git worktree prune\`. Commit nothing. Return \`removed\` (the count of worktrees removed) and \`kept\` (the \`worktree-wf_*\` branches left in place).
${RETURN_FIELDS}`;
}

function ledgerPrompt(ctx, plan, wave, entries) {
  return `${header(ctx, plan)}

Append stage. You are in the main checkout on ${ctx.integrationBranch}. Append the Markdown between the markers below to the end of ${ctx.ledgerPath}, every character unchanged, preceded by one blank line. If the file does not exist, create its directory (\`mkdir -p $(dirname ${ctx.ledgerPath})\`) and the file with this heading first:

${ledgerHeading(ctx.name)}
Do not reword, reorder, merge, or drop rows, and never edit earlier content. Commit on ${ctx.integrationBranch}. ${ownFilesOnly(ctx.ledgerPath)} ${commitMessage(`docs(${ctx.name}): append ${ledgerLabel(entries, wave)} signal ledger entries`)} Return the path and the ${SHORT_SHA}. ${pauseClause(ctx)}

<<<LEDGER
${renderLedger(entries, wave)}LEDGER>>>
${RETURN_FIELDS}`;
}

// ---------------------------------------------------------------- critique gate

// Bounded critique loop shared by the plan gate and the code review gate.
// `round(n)` returns [{ stage, ret }] for the round's reviewers; `fix(n,
// criticals)` returns the fix stage's return. Returns { ok, reason, rounds,
// findings } and pushes signals as it goes. Significant and Minor findings
// accumulate in `findings`, each stamped with its stage and round, for the
// caller to record once per gate; a Critical the fix stage declines, a
// systemic verdict, or the round cap is a structural stop.
async function critiqueLoop({ round, fix, onFix, stageCtx, signals }) {
  const findings = [];
  for (let n = 1; n <= GATE_MAX_ROUNDS; n++) {
    const rets = await round(n);
    for (const { stage, ret } of rets) {
      if (!ret) {
        signals.push(nullReturnSignal(stageCtx(stage)));
        return { ok: false, reason: 'agent-null', rounds: n, findings };
      }
    }
    const criticals = rets.flatMap(({ stage, ret }) =>
      ret.result.findings.filter((f) => f.severity === 'Critical').map((f) => ({ ...f, stage })));
    findings.push(...rets.flatMap(({ stage, ret }) =>
      ret.result.findings.filter((f) => f.severity !== 'Critical').map((f) => ({ ...f, stage, round: n }))));
    const systemic = rets.filter(({ ret }) => ret.result.systemic);
    if (systemic.length) {
      signals.push(signal({
        ...stageCtx(systemic[0].stage), kind: 'gate-non-convergence', source: 'gate',
        statement: `systemic problem reported in round ${n}: ${systemic[0].ret.result.summary}`,
        referent: stageCtx().unit,
      }));
      return { ok: false, reason: 'systemic', rounds: n, findings };
    }
    if (!criticals.length) return { ok: true, rounds: n, findings };
    if (n === GATE_MAX_ROUNDS) {
      signals.push(signal({
        ...stageCtx(rets[0].stage), kind: 'gate-non-convergence', source: 'structural',
        statement: `${criticals.length} Critical finding(s) remain after round ${n} (hard cap)`,
        referent: stageCtx().unit,
      }));
      return { ok: false, reason: 'hard-cap', rounds: n, findings };
    }
    const fixed = await fix(n, criticals);
    if (!fixed) {
      signals.push(nullReturnSignal(stageCtx('fix')));
      return { ok: false, reason: 'agent-null', rounds: n, findings };
    }
    for (const a of fixed.result.applied) {
      signals.push(signal({
        ...stageCtx('fix'), kind: 'critical-fixed', source: 'structural',
        statement: `round ${n}: ${a.finding} — fixed: ${a.fix}`, referent: a.finding,
      }));
    }
    if (fixed.result.notApplied.length) {
      // A declined Critical stays Critical; the loop cannot pass, so stop
      // here rather than burn rounds to the cap.
      for (const a of fixed.result.notApplied) {
        signals.push(signal({
          ...stageCtx('fix'), kind: 'gate-non-convergence', source: 'structural',
          statement: `round ${n}: Critical finding declined by the fix stage — ${a.finding}: ${a.reason}`,
          referent: a.finding,
        }));
      }
      return { ok: false, reason: 'declined', rounds: n, findings };
    }
    onFix(fixed);
  }
  return { ok: false, reason: 'hard-cap', rounds: GATE_MAX_ROUNDS, findings };
}

// ---------------------------------------------------------------- unit

// The five stages for one unit. Never throws; a stage that returns null or
// raises a STOP-kind signal aborts the unit, and the result says where.
async function runUnit(rt, ctx, plan, unit, wave) {
  const signals = [];
  const stageCtx = (stage) => ({ stage, unit: unit.id, wave });
  const result = { unit: unit.id, type: unit.type, branch: null, completionPath: null, distress: null, signals, aborted: null };
  const abort = (stage) => { result.aborted = stage; return result; };
  const opts = (label, extra = {}) => ({ label: `${label}:${unit.id}`, phase: 'Units', ...tier(ctx, 'default'), ...extra });

  // 1. plan
  const planned = await rt.agent(unitPlanPrompt(ctx, plan, unit), opts('plan', { isolation: 'worktree', schema: UNIT_PLAN_SCHEMA, ...tier(ctx, 'planning') }));
  if (!planned) { signals.push(nullReturnSignal(stageCtx('plan'))); return abort('plan'); }
  signals.push(...signalsFromReadings(planned.result.readings, { ...stageCtx('plan'), path: planned.result.path }));
  signals.push(...signalsFromDocumentSize(planned.result.bytes, { ...stageCtx('plan'), path: planned.result.path, targetKb: UNIT_PLAN_TARGET_KB }));
  let branch = planned.result.branch;

  // 2. critique gate over the plan
  const planGate = await critiqueLoop({
    stageCtx, signals,
    round: async (n) => [{
      stage: 'critique-plan',
      ret: await rt.agent(planCritiquePrompt(ctx, plan, unit, branch, n),
        opts(`critique-plan:r${n}`, { agentType: 'phronithm:critic', schema: FINDINGS_SCHEMA, ...tier(ctx, 'critique') })),
    }],
    fix: (n, criticals) => rt.agent(fixPrompt(ctx, plan, unit, branch, 'plan', criticals, n),
      opts(`fix-plan:r${n}`, { isolation: 'worktree', schema: FIX_SCHEMA })),
    onFix: (fixed) => { branch = fixed.result.branch; },
  });
  signals.push(...signalsFromFindings(planGate.findings, stageCtx('critique-plan')));
  if (!planGate.ok) return abort('critique-plan');

  // 3. implement
  const impl = await rt.agent(implementPrompt(ctx, plan, unit, branch), opts('implement', { isolation: 'worktree', schema: IMPLEMENT_SCHEMA }));
  if (!impl) { signals.push(nullReturnSignal(stageCtx('implement'))); return abort('implement'); }
  const drifted = impl.result.gateProbe.filter((g) => g.drifted);
  for (const g of drifted) {
    signals.push(signal({
      ...stageCtx('implement'), kind: 'environment-gate-drift', source: 'structural',
      statement: `gate ${g.name} pinned at ${g.pin}, observed ${g.observed}`, referent: g.name,
    }));
  }
  if (drifted.length) return abort('implement');
  branch = impl.result.branch;
  result.distress = impl.result.distress;
  signals.push(...signalsFromImplement(impl.result, stageCtx('implement')));
  signals.push(...distressSignals(impl.result.distress, { unit: unit.id, wave }, ctx.config));
  if (!impl.result.doneCondition.evaluable) {
    signals.push(signal({
      ...stageCtx('implement'), kind: 'done-condition-unevaluable', source: 'structural',
      statement: `done condition cannot be evaluated as written: ${impl.result.doneCondition.evidence}`, referent: unit.id,
    }));
    return abort('implement');
  }

  // 4. review gate over the branch diff
  const lenses = LENSES_BY_TYPE[unit.type] ?? LENSES_BY_TYPE.feature;
  const reviewGate = await critiqueLoop({
    stageCtx, signals,
    round: async (n) => {
      const reviewers = unit.type === 'design'
        ? [['critique-design', designCritiquePrompt(ctx, plan, unit, branch, n)]]
        : [
          ['critique-code', codeCritiquePrompt(ctx, plan, unit, branch, n, impl.result.gates)],
          ...(lenses.length ? [['lens-review', lensReviewPrompt(ctx, plan, unit, branch, lenses, n, impl.result.gates)]] : []),
        ];
      const rets = await rt.parallel(reviewers.map(([stage, prompt]) => () =>
        rt.agent(prompt, opts(`${stage}:r${n}`, { isolation: 'worktree', schema: FINDINGS_SCHEMA, ...tier(ctx, 'critique') }))));
      return reviewers.map(([stage], i) => ({ stage, ret: rets[i] ?? null }));
    },
    fix: (n, criticals) => rt.agent(fixPrompt(ctx, plan, unit, branch, 'code', criticals, n),
      opts(`fix-code:r${n}`, { isolation: 'worktree', schema: FIX_SCHEMA })),
    onFix: (fixed) => { branch = fixed.result.branch; },
  });
  signals.push(...signalsFromFindings(reviewGate.findings, stageCtx('review')));
  if (!reviewGate.ok) return abort('review');

  // 5. completion report: rendered here, committed by a copy stage
  const review = { rounds: reviewGate.rounds, findings: reviewGate.findings };
  const text = renderCompletionReport(ctx, unit, wave, branch, impl.result, review);
  signals.push(...signalsFromDocumentSize(byteLength(text), { ...stageCtx('completion-report'), path: completionPath(unit), targetKb: COMPLETION_REPORT_TARGET_KB }));
  const report = await rt.agent(completionScribePrompt(ctx, plan, unit, branch, text), opts('completion-report', { isolation: 'worktree', schema: COMPLETION_SCHEMA, ...tier(ctx, 'mechanical') }));
  if (!report) { signals.push(nullReturnSignal(stageCtx('completion-report'))); return abort('completion-report'); }
  result.branch = report.result.branch;
  result.completionPath = completionPath(unit);
  return result;
}

// ---------------------------------------------------------------- run

function decisionsFor(stops) {
  return stops.map((s, i) => ({
    id: `d${i + 1}`,
    question: `${s.kind} (${s.source}, ${s.stage}${s.unit ? `, ${s.unit}` : ''}): ${s.statement}`,
    affects: s.unit ? [s.unit] : [s.stage],
  }));
}

// Signals from a re-plan return: a verdict per item (cannot or missing is a
// stop; document-text also records the architecture amendment), each
// amendment, and each over-budget unit by disposition (unrepaired is a
// stop). A verdict row carries the item's id, not its text: the item's own
// row is already in the ledger. `describe`, when given, renders the item
// into the verdict row too — the decision re-plan's items have no prior
// row. A stop always carries the item text, since the requester adjudicates
// it. `referentOf` gives the ledger referent. A standing flag is reported
// on every pass but recorded once: `ledger` supplies the last over-budget
// kind per unit, and a repaired or flagged unit whose kind is unchanged adds
// no row. An unrepaired unit always does; it stops.
const describeItem = (it) => `[${it.stage}] ${it.kind}: ${it.statement}`;
function replanSignals(result, items, { stage, wave, ledger = [] }, describe, referentOf = (it) => it.unit ?? it.id) {
  const out = [];
  const verdicts = new Map((result.verdicts ?? []).map((v) => [v.id, v]));
  items.forEach((it, i) => {
    const v = verdicts.get(it.id);
    if (v && v.verdict !== 'cannot') {
      out.push(signal({
        kind: 'verdict', source: 'reported', stage, unit: it.unit, wave,
        statement: `${it.id} ${v.verdict}: ${v.statement}${describe ? ` — ${describe(it)}` : ''}`, referent: referentOf(it, i),
      }));
      if (v.verdict === 'document-text') {
        out.push(signal({ kind: 'architecture-amendment', source: 'reported', stage, wave, statement: v.statement, referent: referentOf(it, i) }));
      }
      return;
    }
    out.push(signal({
      kind: 'plan-defect-unabsorbed', source: 'reported', stage, unit: it.unit, wave,
      statement: `${v ? `re-plan cannot absorb: ${v.statement}` : 're-plan gave no verdict'} — ${(describe ?? describeItem)(it)}`, referent: referentOf(it, i),
    }));
  });
  for (const a of result.amendments ?? []) {
    out.push(signal({ kind: 'amendment', source: 'reported', stage, unit: a.unit, wave, statement: `${a.kind}: ${a.statement}`, referent: a.unit }));
  }
  const budgetKind = { extracted: 'over-budget-repaired', flagged: 'over-budget-flagged', unrepaired: 'unit-over-budget' };
  const budgetKinds = Object.values(budgetKind);
  const standing = new Map(ledger.filter((s) => budgetKinds.includes(s.kind)).map((s) => [s.unit, s.kind]));
  for (const b of result.overBudget ?? []) {
    const kind = budgetKind[b.disposition] ?? 'unit-over-budget';
    if (kind !== 'unit-over-budget' && standing.get(b.unit) === kind) continue;
    out.push(signal({ kind, source: 'reported', stage, unit: b.unit, wave, statement: b.statement, referent: b.unit }));
  }
  return out;
}

function requireArg(args, key) {
  if (args[key] === undefined || args[key] === null || args[key] === '') throw new Error(`lsf-implement: args.${key} is required`);
  return args[key];
}

async function run(runtime) {
  const args = runtime.args ?? {};
  // Seeded stage returns stand in for the agent; matched by label. A seed
  // reports the pause ref as it stood when salvaged, so its report is dropped.
  const seed = args.seed ?? {};
  const rt = {
    ...runtime,
    agent: async (prompt, opts) => {
      if (Object.hasOwn(seed, opts.label)) {
        runtime.log(`${opts.label}: seeded from args.seed; agent not spawned`);
        const s = seed[opts.label];
        return s?.result?.pauseRequested ? { ...s, result: { ...s.result, pauseRequested: false } } : s;
      }
      return runtime.agent(prompt, opts);
    },
  };
  const { agent, parallel, log } = rt;
  const name = requireArg(args, 'name');
  const ctx = {
    name,
    planSha: requireArg(args, 'planSha'),
    planPath: args.planPath ?? `docs/plans/${name}-breakdown.md`,
    architectureDoc: requireArg(args, 'architectureDoc'),
    architectureSha: args.architectureSha,
    requestRecord: args.requestRecord ?? `docs/architecture/${name}-brief.md`,
    integrationBranch: requireArg(args, 'integrationBranch'),
    decisionLogPath: args.decisionLogPath ?? `docs/plans/${name}-decisions.md`,
    decisionLogSha: args.decisionLogSha,
    // One ledger file per launch, named by the plan SHA the launch starts
    // from; a decision re-plan moves plan.sha on, the ledger stays put.
    ledgerPath: args.ledgerPath ?? `docs/plans/${name}-signals/${args.planSha}.md`,
    archivePath: args.archivePath ?? `docs/plans/${name}-archive.md`,
    config: { ...DEFAULT_CONFIG, ...(args.config ?? {}) },
    maxWaves: args.maxWaves ?? DEFAULT_MAX_WAVES,
    maxUnitsPerWave: args.maxUnitsPerWave ?? Infinity,
    pauseRef: args.pauseRef ?? `refs/lsf/${name}/pause`,
    tiers: { ...DEFAULT_TIERS, ...(args.tiers ?? {}) },
  };

  const ledger = [];
  let plan = { path: ctx.planPath, sha: ctx.planSha, units: [], executionOrder: [] };
  let wave = 0;

  // Rows a previous run returned unscribed. They keep their own wave and
  // disposition, are scribed first, and stay out of this run's digest.
  const carried = new Set((Array.isArray(args.unscribed) ? args.unscribed : []).map((row) => {
    const s = signal(row);
    ledger.push(s);
    return s;
  }));

  // Ledger rows a scribe has committed. A null scribe leaves its rows for
  // the next append and, at return, for `unscribed`.
  const scribed = new Set();
  // Key order is the presentation order: status and escalation first, the
  // ledger stays in the signals file. `escalation` is undefined unless
  // stopped, and JSON drops it.
  const outcome = (status, { escalation: esc, ...extra } = {}) => ({
    status, workflow: WORKFLOW, escalation: esc, digest: digest(ledger.filter((e) => !carried.has(e))),
    planSha: plan.sha, integrationBranch: ctx.integrationBranch, waves: wave,
    ledgerPath: ctx.ledgerPath, unscribed: ledger.filter((e) => !scribed.has(e)), ...extra,
  });
  // A stage that saw the pause ref sets this; the loop top honours it once
  // the wave's commits are in.
  let pauseRequested = false;
  const notePause = (r, label) => {
    if (!r?.result?.pauseRequested || pauseRequested) return;
    pauseRequested = true;
    log(`pause requested (${ctx.pauseRef} seen by ${label}); the run pauses at the next wave boundary`);
  };
  // A scribe that returns without a commit SHA may have left its append
  // uncommitted in the main checkout, where the next merge would meet it as
  // a conflict. That is a stop, and no later scribe runs over the leftover.
  let ledgerFault = null;
  const scribeLedger = async (label = `ledger:w${wave}`) => {
    const fresh = ledger.filter((e) => !scribed.has(e));
    if (ledgerFault || !fresh.length) return;
    const r = await agent(ledgerPrompt(ctx, plan, wave, fresh), { label, ...tier(ctx, 'mechanical'), phase: 'Ledger', schema: LEDGER_SCHEMA });
    if (!r) { log(`ledger scribe returned null (${label}); ${fresh.length} row(s) stay unscribed`); return; }
    notePause(r, label);
    if (!committed(r)) {
      ledgerFault = noCommitSignal({ stage: 'ledger', wave: wave || undefined, path: ctx.ledgerPath });
      log(`ledger scribe returned no commit SHA (${label}); ${fresh.length} row(s) stay unscribed`);
      return;
    }
    for (const e of fresh) scribed.add(e);
  };
  // Every stop path: stamp, ledger, scribe, escalate. `stops` may already be
  // in the ledger (from evaluateWave); the rest are appended here. Rows
  // not yet scribed ride in the same append, so a stop costs one scribe.
  const stopped = async (stage, stops, extra = {}) => {
    for (const s of stops) s.disposition = 'stopped';
    ledger.push(...stops.filter((s) => !ledger.includes(s)));
    await scribeLedger();
    if (ledgerFault && !stops.includes(ledgerFault)) {
      ledgerFault.disposition = 'stopped';
      ledger.push(ledgerFault);
      stops = [...stops, ledgerFault];
    }
    log(`STOP at ${stage}: ${stops.map((s) => s.kind).join(', ')}`);
    return outcome('stopped', {
      escalation: escalation({ workflow: WORKFLOW, planSha: plan.sha, wave: wave || undefined, stage, signals: stops, decisionNeeded: decisionsFor(stops), ...extra }),
    });
  };
  const stopOn = stopped;
  // Scribe, and stop when the scribe left no commit.
  const scribeOrStop = async (label) => {
    await scribeLedger(label);
    return ledgerFault ? stopped('ledger', []) : null;
  };
  if (carried.size) {
    log(`${carried.size} ledger row(s) carried from a previous run`);
    const halt = await scribeOrStop('ledger:carried');
    if (halt) return halt;
  }
  // Edits a planning stage returned as outside the plan: one scribe makes
  // and commits them, and the ledger records each against the stage that
  // asked. Returns the signals; the caller evaluates them.
  const applyDirectEdits = async (at, edits = [], what, { stage, wave: w, phase }) => {
    if (!edits.length) return [];
    const label = w === undefined ? 'direct-edits' : `direct-edits:w${w}`;
    const r = await agent(directEditsPrompt(ctx, at, what, edits), { label, ...tier(ctx, 'default'), phase, schema: SCRIBE_SCHEMA });
    if (!r) return [nullReturnSignal({ stage: 'direct-edits', wave: w })];
    if (!committed(r)) return [noCommitSignal({ stage: 'direct-edits', wave: w, path: edits.map((e) => e.path).join(',') })];
    return edits.map((e) => signal({ kind: 'direct-edit', source: 'reported', stage, wave: w, statement: e.description, referent: e.path }));
  };

  // Decisions handed back by the requester go into the repo first, so the
  // SHA can appear in every prompt they affect.
  if (Array.isArray(args.decisions) && args.decisions.length) {
    const d = await agent(decisionLogPrompt(ctx, args.decisions), { label: 'decision-log', ...tier(ctx, 'mechanical'), phase: 'Load', schema: DECISION_LOG_SCHEMA });
    if (!d) return stopOn('decision-log', [nullReturnSignal({ stage: 'decision-log' })]);
    if (!committed(d)) return stopOn('decision-log', [noCommitSignal({ stage: 'decision-log', path: ctx.decisionLogPath })]);
    ctx.decisionLogSha = d.result.sha;
    ctx.decisionLogPath = d.result.path;

    // Decisions are the plan's to carry out before any unit starts: a
    // re-plan over the decisions, then the run loads the SHA it returns.
    const items = args.decisions.map(({ id, question, decision }) => ({ id, question, decision }));
    const rp = await agent(decisionReplanPrompt(ctx, plan, items), { label: 'decision-replan', ...tier(ctx, 'planning'), phase: 'Load', schema: REPLAN_SCHEMA });
    if (!rp) return stopOn('decision-replan', [nullReturnSignal({ stage: 'decision-replan' })]);
    const sigs = replanSignals(rp.result, items, { stage: 'decision-replan', wave: 0, ledger }, (it) => it.question);
    sigs.push(...await applyDirectEdits({ ...plan, sha: rp.result.planSha }, rp.result.directEdits, 'decision re-plan', { stage: 'decision-replan', wave: 0, phase: 'Load' }));
    const ev = evaluateWave(sigs);
    ledger.push(...ev.all);
    if (ev.stop) return stopped('decision-replan', ev.stops, { planSha: rp.result.planSha, preReplanSha: plan.sha });
    const halt = await scribeOrStop();
    if (halt) return halt;
    plan = { ...plan, sha: rp.result.planSha };
    ctx.planSha = plan.sha;
    log(`decisions applied: ${rp.result.amendments.length} amendment(s) → plan @ ${plan.sha}`);
  }

  // Plan graph at a SHA. A copy agent returns the fenced blocks verbatim;
  // graph.js parses and validates. Structural problems (unreadable block,
  // unknown unit, cycle, unserialised overlap, bad clearance) are STOP
  // signals.
  const loadPlan = async (sha, label, w) => {
    const copy = await agent(copyPrompt(ctx, sha), { label, ...tier(ctx, 'mechanical'), phase: 'Load', schema: COPY_SCHEMA });
    if (!copy) return { plan: null, copy: null, stops: [nullReturnSignal({ stage: 'load', wave: w })] };
    notePause(copy, label);
    const g = loadGraph({ planMarkdown: copy.result.planBlock, decisionsMarkdown: ctx.decisionLogSha ? copy.result.decisionsBlock : null });
    const stops = g.problems.map((p) => signal({ ...p, source: 'structural', stage: 'load', wave: w }));
    let loadedPlan = null;
    if (g.plan) {
      loadedPlan = { ...g.plan, path: ctx.planPath, sha };
      stops.push(...validatePlan(loadedPlan).map((p) => signal({ ...p, source: 'structural', stage: 'load', wave: w })));
    }
    return { plan: loadedPlan, copy, stops };
  };

  // The pin follows the integration branch: a reload's copy stage reports
  // the document's SHA after any document-text correction.
  const refreshPin = (copy) => {
    const reported = copy?.result.architectureSha;
    if (typeof reported === 'string' && reported.trim()) ctx.architectureSha = reported.trim();
  };

  // Load.
  const first = await loadPlan(ctx.planSha, 'load');
  if (!first.copy) return stopOn('load', first.stops);
  ctx.architectureSha = ctx.architectureSha ?? first.copy.result.architectureSha;
  const loadSignals = [...first.stops];
  if (first.copy.result.headBranch !== ctx.integrationBranch) {
    loadSignals.push(signal({
      kind: 'precondition', source: 'structural', stage: 'load',
      statement: `main checkout is on ${first.copy.result.headBranch}, not ${ctx.integrationBranch}`, referent: ctx.integrationBranch,
    }));
  }
  const loadEval = evaluateWave(loadSignals);
  ledger.push(...loadEval.all);
  if (loadEval.stop || !first.plan) return stopped('load', loadEval.stops);
  plan = first.plan;
  log(`loaded ${plan.units.length} units @ ${plan.sha}; ${ctx.decisionLogSha ? 'decision log applied' : 'no decision log'}; projected waves: ${wavePlan(plan).map((w) => w.join('+')).join(' | ') || 'none'}`);

  const completed = new Set(plan.units.filter((u) => u.complete).map((u) => u.id));
  let terminationRan = false;

  for (;;) {
    // Everything the wave produced is committed by here; a pause returns
    // like a wave cap, from the plan as it stands.
    if (pauseRequested) {
      const halt = await scribeOrStop();
      if (halt) return halt;
      log(`PAUSED after wave ${wave} on ${ctx.pauseRef}`);
      return outcome('paused', { note: `paused after ${wave} wave(s): ${ctx.pauseRef} exists; ${plan.units.length - completed.size} unit(s) remain. To resume: delete the ref (git update-ref -d ${ctx.pauseRef}) and relaunch from planSha with no decisions, passing unscribed back when non-empty` });
    }
    const allReady = readySet(plan, completed);
    const ready = allReady.slice(0, ctx.maxUnitsPerWave);
    if (ready.length < allReady.length) log(`wave ${wave + 1}: ${allReady.length} ready, capped at ${ready.length}: ${allReady.slice(ready.length).map((u) => u.id).join(', ')} deferred`);
    if (!ready.length) {
      const blocked = blockedUnits(plan, completed);
      if (blocked.length) {
        const stops = blocked.map((b) => signal({
          kind: 'no-ready-unit', source: 'structural', stage: 'schedule', unit: b.unit, wave: wave + 1,
          statement: b.reason === 'gated' ? `uncleared gate(s): ${b.gates.join(', ')}` : `waiting on ${b.waitingOn.join(', ')} with nothing ready`,
          referent: b.reason === 'gated' ? b.gates.join(',') : b.unit,
        }));
        return stopOn('schedule', stops);
      }
      if (terminationRan) break;
      // Termination stage: meta-tasks become units; small ones are direct edits.
      terminationRan = true;
      const term = await agent(terminationPrompt(ctx, plan), { label: 'termination', ...tier(ctx, 'planning'), phase: 'Termination', schema: TERMINATION_SCHEMA });
      if (!term) return stopOn('termination', [nullReturnSignal({ stage: 'termination' })]);
      const re = await loadPlan(term.result.planSha, 'load:termination', wave);
      refreshPin(re.copy);
      const termEval = evaluateWave(re.stops);
      ledger.push(...termEval.all);
      if (termEval.stop) return stopped('termination', termEval.stops);
      plan = re.plan;
      const editEval = evaluateWave(await applyDirectEdits(plan, term.result.directEdits, 'termination stage (the plan\'s Meta-Tasks section)', { stage: 'termination', phase: 'Termination' }));
      ledger.push(...editEval.all);
      if (editEval.stop) return stopped('direct-edits', editEval.stops);
      log(`termination: ${term.result.appendedUnits.length} meta unit(s) appended, ${term.result.directEdits.length} direct edit(s)`);
      continue;
    }

    wave += 1;
    if (wave > ctx.maxWaves) {
      return outcome('wave-cap', { note: `stopped after ${ctx.maxWaves} waves (args.maxWaves); ${plan.units.length - completed.size} unit(s) remain` });
    }
    log(`wave ${wave}: ${ready.map((u) => u.id).join(', ')}`);

    // Units, then barrier: merge needs every branch.
    const raw = await parallel(ready.map((u) => () => runUnit(rt, ctx, plan, u, wave)));
    const results = ready.map((u, i) => raw[i] ?? {
      unit: u.id, type: u.type, branch: null, completionPath: null, distress: null, aborted: 'unit',
      signals: [nullReturnSignal({ stage: 'unit', unit: u.id, wave })],
    });
    const waveSignals = results.flatMap((r) => r.signals);
    const mergeable = results.filter((r) => r.branch);
    const byUnit = new Map(mergeable.map((r) => [r.unit, r]));
    const order = mergeOrder(plan, mergeable.map((r) => r.unit)).map((id) => byUnit.get(id));

    // Merge, in plan execution order. The pin moves to the merged text before
    // the gate; the gate judges the units' own changes against prevArch.
    const prevArch = ctx.architectureSha;
    let merged = [];
    let conflicts = [];
    if (order.length) {
      const m = await agent(mergePrompt(ctx, plan, wave, order), { label: `merge:w${wave}`, ...tier(ctx, 'default'), phase: 'Merge', schema: MERGE_SCHEMA });
      if (!m) {
        waveSignals.push(nullReturnSignal({ stage: 'merge', wave }));
      } else {
        notePause(m, `merge:w${wave}`);
        waveSignals.push(...signalsFromMerge(m.result, { wave }));
        for (const x of m.result.merges) {
          if (x.status === 'clean' || x.status === 'resolved') { completed.add(x.unit); merged.push({ ...byUnit.get(x.unit), mergeSha: x.sha }); }
          if (x.status === 'resolved') conflicts.push(x);
        }
        plan = { ...plan, sha: m.result.planSha, units: plan.units.map((u) => (completed.has(u.id) ? { ...u, complete: true } : u)) };
        refreshPin(m);
      }
    }

    // The wave gate: one question, does the wave invalidate the architecture.
    // Merged worktrees are removed alongside it; the gate reads the main
    // checkout only. Cleanup never signals: a failure leaves residue, not risk.
    if (merged.length) {
      const [g, c] = await parallel([
        () => agent(gatePrompt(ctx, plan, wave, merged, prevArch), { label: `review-gate:w${wave}`, ...tier(ctx, 'gate'), phase: 'Gate', schema: GATE_SCHEMA }),
        () => agent(cleanupPrompt(ctx, plan, wave), { label: `cleanup:w${wave}`, ...tier(ctx, 'mechanical'), phase: 'Merge', schema: CLEANUP_SCHEMA }),
      ]);
      if (!g) waveSignals.push(nullReturnSignal({ stage: 'review-gate', wave }));
      else waveSignals.push(...signalsFromGate(g.result, { wave }));
      log(`wave ${wave}: ${c ? `removed ${c.result.removed} merged worktree(s), kept ${c.result.kept.length}` : 'worktree cleanup returned null'}`);
    }

    const evalA = evaluateWave(waveSignals);
    ledger.push(...evalA.all);
    log(`wave ${wave}: merged ${merged.map((r) => r.unit).join(', ') || 'nothing'}; ${conflicts.length} conflict(s); ${evalA.records.length} recorded, ${evalA.stops.length} stop signal(s)`);
    // A wave stop still re-plans over whatever merged, unless nothing did
    // or the stop supersedes the plan (its relaunch is Phase 2's re-entry).
    // A relaunch re-plans over decisions alone, so this is the only pass
    // that absorbs the merged reports.
    const superseded = evalA.stops.some((s) => PLAN_SUPERSEDING_STOP_KINDS.includes(s.kind));
    if (evalA.stop && (!merged.length || superseded)) return stopped('wave', evalA.stops);

    // Re-plan from the wave's completion reports. Items go in with ids; one
    // the re-plan cannot absorb, or does not answer, is a stop.
    const flat = ({ unit, stage, kind, statement, referent }) => ({ unit, stage, kind, statement, referent });
    const itemSignals = evalA.records.filter((s) => REPLAN_ITEM_KINDS.includes(s.kind));
    const items = itemSignals.map((s, i) => ({ id: `rp-${i + 1}`, ...flat(s) }));
    // Nothing to absorb — no item and no claimed-disjoint conflict — and the
    // re-plan has no work: the plan already carries the merge's SHA and
    // completion flags, so the next wave schedules from it as it stands.
    if (!items.length && !conflicts.length) {
      log(`wave ${wave}: nothing to absorb; re-plan skipped, plan @ ${plan.sha}`);
      if (evalA.stop) return stopped('wave', evalA.stops);
      const halt = await scribeOrStop();
      if (halt) return halt;
      for (const u of plan.units) if (u.complete) completed.add(u.id);
      continue;
    }
    const rp = await agent(replanPrompt(ctx, plan, wave, merged, conflicts, items, evalA.stops), { label: `replan:w${wave}`, ...tier(ctx, 'planning'), schema: REPLAN_SCHEMA, phase: 'Re-plan' });
    const replanSigs = [];
    let nextPlan = plan;
    if (!rp) {
      replanSigs.push(nullReturnSignal({ stage: 'replan', wave }));
    } else {
      replanSigs.push(...replanSignals(rp.result, items, { stage: 'replan', wave, ledger }, undefined, (it, i) => itemSignals[i].referent));
      // Before the load, and on a stopping wave too, so the edits are not
      // lost with the stop.
      replanSigs.push(...await applyDirectEdits({ ...plan, sha: rp.result.planSha }, rp.result.directEdits, `wave ${wave} re-plan`, { stage: 'replan', wave, phase: 'Re-plan' }));
      const re = await loadPlan(rp.result.planSha, `load:w${wave}`, wave);
      refreshPin(re.copy);
      replanSigs.push(...re.stops);
      if (re.plan) nextPlan = re.plan;
    }
    const evalB = evaluateWave(replanSigs);
    ledger.push(...evalB.all);
    // The re-plan committed its absorbed amendments before the stop; the
    // record carries that SHA as the plan to relaunch from.
    if (evalA.stop || evalB.stop) {
      return stopped(evalA.stop ? 'wave' : 'replan', [...evalA.stops, ...evalB.stops], rp ? { planSha: rp.result.planSha, preReplanSha: plan.sha } : {});
    }
    const halt = await scribeOrStop();
    if (halt) return halt;
    plan = nextPlan;
    for (const u of plan.units) if (u.complete) completed.add(u.id);
    log(`wave ${wave}: re-plan ${rp.result.amendments.length} amendment(s) → plan @ ${plan.sha}; ${rp.result.summary}`);
  }

  // Standing obligations. One carried by its own terms is a record when the
  // stage names where it is recorded outside the plan; otherwise a stop.
  const ob = await agent(obligationsPrompt(ctx, plan), { label: 'obligations', ...tier(ctx, 'default'), phase: 'Termination', schema: OBLIGATIONS_SCHEMA });
  if (!ob) return stopOn('obligations', [nullReturnSignal({ stage: 'obligations' })]);
  const obSignals = [];
  for (const o of ob.result.obligations) {
    if (o.disposition === 'discharged') continue;
    const carrier = (o.carrier ?? '').trim();
    if (o.disposition === 'carried' && carrier) {
      obSignals.push(signal({ kind: 'standing-obligation-carried', source: 'reported', stage: 'obligations', statement: o.statement, referent: carrier }));
      continue;
    }
    obSignals.push(signal({
      kind: 'standing-obligation-undischarged', source: 'structural', stage: 'obligations',
      statement: `${o.disposition === 'carried' ? 'carried without a carrier: ' : ''}${o.statement}`, referent: o.statement,
    }));
  }
  const obEval = evaluateWave(obSignals);
  ledger.push(...obEval.all);
  if (obEval.stop) return stopped('obligations', obEval.stops);
  const halt = await scribeOrStop();
  if (halt) return halt;

  log(`complete: ${completed.size} unit(s) over ${wave} wave(s); silence is not evidence`);
  return outcome('complete');
}

export { run, critiqueLoop, runUnit, renderCompletionReport, LENSES_BY_TYPE, GATE_MAX_ROUNDS };
