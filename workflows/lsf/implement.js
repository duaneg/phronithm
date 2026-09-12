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
//   architectureDoc     path (required); architectureSha its commit SHA
//   requestRecord       path, optional
//   integrationBranch   the main checkout must be on this branch (required)
//   decisionLogPath     default docs/plans/<name>-decisions.md
//   decisionLogSha      SHA of the decision log already committed, optional
//   decisions           [{ id, question, decision, clears? }] to commit before
//                       loading; `clears` lists `unit/gate` names
//   ledgerPath          default docs/plans/<name>-signals.md
//   config              overrides for scheduling DEFAULT_CONFIG
//   maxWaves            default 20
//   maxUnitsPerWave     cap on units started per wave, in execution order; default unbounded
//   tiers               per-role { model, effort } overrides; roles:
//                       critique (unit plan critique, code critique, lens
//                       review), gate (wave gate, obligations), planning
//                       (unit plan, re-plan, termination), mechanical
//                       (plan-block copy, ledger and decision-log appends),
//                       default (everything else)

import {
  stageSchema, MERGE_RESULT_SCHEMA, DISTRESS_SCHEMA, INVALIDATION_SCHEMA, AMENDMENT_KINDS, signal, escalation,
} from './schema.js';
import { loadGraph } from './graph.js';
import {
  DEFAULT_CONFIG, REPLAN_ITEM_KINDS, validatePlan, readySet, blockedUnits, mergeOrder, wavePlan,
  nullReturnSignal, signalsFromReadings, signalsFromImplement, signalsFromFindings, signalsFromGate,
  signalsFromMerge, distressSignals, evaluateWave, digest, renderLedger, ledgerHeading,
} from './scheduling.js';

const WORKFLOW = 'lsf-implement';
const GATE_MAX_ROUNDS = 4; // critique gate hard cap
const DEFAULT_MAX_WAVES = 20;
// Model and effort per role.
const DEFAULT_TIERS = Object.freeze({
  critique: { model: 'sonnet' },
  gate: { model: 'opus' },
  planning: { model: 'fable', effort: 'medium' },
  default: { model: 'sonnet' },
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

const COMMIT_FIELDS = { branch: str, sha: str };

const DECISION_LOG_SCHEMA = stageSchema(obj({ path: str, sha: str }));

const COPY_SCHEMA = stageSchema(obj({
  planBlock: { ...str, description: 'The plan document\'s ```yaml lsf-plan fenced block, verbatim, fences included' },
  decisionsBlock: { ...str, description: 'The decision log\'s ```yaml lsf-decisions fenced block, verbatim, fences included; empty string when there is no decision log' },
  headBranch: { ...str, description: 'Branch the main checkout is on' },
  architectureSha: { ...str, description: 'Commit SHA of the architecture document on the integration branch' },
}));

const UNIT_PLAN_SCHEMA = stageSchema(obj({
  ...COMMIT_FIELDS,
  path: str,
  planText: { ...str, description: 'The implementation plan verbatim' },
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
  planText: { ...str, description: 'The amended plan verbatim, when the subject is a plan; empty otherwise' },
}));

const IMPLEMENT_SCHEMA = stageSchema(obj({
  ...COMMIT_FIELDS,
  gateProbe: arr(obj({ name: str, pin: str, observed: str, drifted: bool })),
  distress: DISTRESS_SCHEMA,
  deviations: { ...strings, description: 'Departures from the unit entry\'s scope or publishes, one statement each' },
  writesOutsideScope: { ...strings, description: 'Files changed that the unit entry\'s Scope line does not name' },
  deferrals: arr(obj({ statement: str, belongsTo: { ...str, description: 'The unit the deferred work belongs to' } })),
  discoveries: arr(obj({
    statement: str,
    affects: { ...str, description: 'The unit whose scope, dependencies, or done condition this changes, or the architecture section it bears on' },
  })),
  doneCondition: obj({ evaluable: bool, met: bool, evidence: str }),
  standingObligations: arr(obj({ statement: str, status: str })),
  summary: str,
}));

const COMPLETION_SCHEMA = stageSchema(obj({ ...COMMIT_FIELDS, path: str }));

const MERGE_SCHEMA = stageSchema(obj({
  ...MERGE_RESULT_SCHEMA.properties,
  planSha: { ...str, description: 'Plan SHA after completion entries were recorded' },
}, [...MERGE_RESULT_SCHEMA.required, 'planSha']));

const GATE_SCHEMA = stageSchema(obj({
  invalidations: arr(INVALIDATION_SCHEMA),
  notes: arr(obj({ statement: str, referent: { ...str, description: 'A document section, unit, symbol, or file' } })),
}));

const REPLAN_SCHEMA = stageSchema(obj({
  planSha: { ...str, description: 'Plan SHA after the amendments were committed; the plan SHA given when nothing changed' },
  amendments: arr(obj({ kind: { type: 'string', enum: AMENDMENT_KINDS }, unit: str, statement: str })),
  overBudget: arr(obj({ unit: str, repaired: bool, statement: str })),
  verdicts: arr(obj({
    id: str,
    verdict: { type: 'string', enum: ['amended', 'no-change', 'cannot'] },
    statement: { ...str, description: 'What was amended, why the plan already answers it, or why the plan cannot absorb it' },
  })),
  summary: str,
}));

const TERMINATION_SCHEMA = stageSchema(obj({
  planSha: { ...str, description: 'Plan SHA after the appended units were committed; the plan SHA given when nothing changed' },
  appendedUnits: strings,
  directEdits: arr(obj({ path: str, description: str })),
}));

const OBLIGATIONS_SCHEMA = stageSchema(obj({
  obligations: arr(obj({ statement: str, discharged: bool, evidence: str })),
}));

const SCRIBE_SCHEMA = stageSchema(obj({ path: str, sha: str }));

// ---------------------------------------------------------------- prompts

const RETURN_FIELDS = `
Besides \`result\`, return \`commits\`: every commit you made, branch and SHA; an empty array when none. Nothing else: no questions or remarks outside the fields named. The return is one flat object.`;

function header(ctx, plan) {
  const lines = [
    `Feature: ${ctx.name}`,
    `Decomposition plan: ${plan.path} @ ${plan.sha}`,
    `Architecture document: ${ctx.architectureDoc} @ ${ctx.architectureSha ?? 'HEAD'}`,
    ctx.requestRecord ? `Request record: ${ctx.requestRecord}` : null,
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

function unitBlock(plan, unit) {
  return `Work unit: ${unit.id} (type: ${unit.type})
Unit entry: the \`### ${unit.id}\` section under \`## Work Units\` in ${plan.path} @ ${plan.sha}. Read it first (\`git show ${plan.sha}:${plan.path}\`); it is requirement text, taken verbatim. ${ENTRY_SEMANTICS}`;
}

function decisionLogPrompt(ctx, decisions) {
  return `${header(ctx, { path: ctx.planPath, sha: ctx.planSha })}

Append the following requester decisions to ${ctx.decisionLogPath} and commit on ${ctx.integrationBranch}. The file holds one \`\`\`yaml lsf-decisions fenced block; each decision is one entry in its \`decisions\` sequence with \`id\`, \`date\` (today, \`date -I\`), \`question\`, \`decision\`, and \`clears\` (a flow sequence of \`unit/gate\` names; omit the key when empty). Quote any scalar containing \`: \`, \` #\`, or leading punctuation with double quotes. Nothing else in the block: no comments, anchors, or multi-line scalars. If the file does not exist, create it as:

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

Return the file path and the commit SHA.
${RETURN_FIELDS}`;
}

function copyPrompt(ctx, sha) {
  const decisions = ctx.decisionLogSha
    ? `Run \`git show ${ctx.decisionLogSha}:${ctx.decisionLogPath}\` and return its \`\`\`yaml lsf-decisions fenced block the same way in \`decisionsBlock\`.`
    : 'There is no decision log; return an empty string in `decisionsBlock`.';
  return `${header(ctx, { path: ctx.planPath, sha })}

Copy stage. Run \`git show ${sha}:${ctx.planPath}\` and return, in \`planBlock\`, its \`\`\`yaml lsf-plan fenced block verbatim: from the opening fence line to the closing fence line inclusive, every character unchanged. ${decisions} Also return the branch the main checkout is on (\`git rev-parse --abbrev-ref HEAD\`) and the commit SHA of ${ctx.architectureDoc} on ${ctx.integrationBranch} (\`git rev-list -1 ${ctx.integrationBranch} -- ${ctx.architectureDoc}\`).

Do not read, assess, or summarise the prose. Do not correct, reformat, or complete anything in the blocks; do not add units, edges, or gates. This stage copies; the script parses and checks the blocks itself.
${RETURN_FIELDS}`;
}

function unitPlanPrompt(ctx, plan, unit) {
  return `${header(ctx, plan)}
${worktree(ctx.integrationBranch)}

${unitBlock(plan, unit)}

Write the implementation plan for this unit. The architecture document is the primary design input; the unit's done condition, adopted verbatim, is a success criterion; each standing obligation naming the unit is a requirement. Do not re-explore what the architecture document and unit entry settle. Keep it within the unit's size budget: about six steps, at most one open design decision. Sections, in order:

1. **Goal** — the unit entry verbatim, and the done condition as the success criterion.
2. **Context** — the architecture sections relied on, the decomposition plan path and SHA, standing obligations naming the unit.
3. **Readings** — every clause of the entry or the architecture document that admitted more than one reading, or named something that does not exist as named, with the reading taken and the evidence for it. Return the same list in \`readings\`. Take the reading; do not raise a question and do not stop.
4. **Design** — the decisions the plan makes, with the one open design decision at most, and why.
5. **Steps** — about six, each with what it changes and how it is verified, in an order that keeps the project's gates green after every step.
6. **Verification** — how the done condition will be evaluated exactly as written, under the readings above.

The plan must be self-contained: a fresh context can implement from it alone. It carries no "Remaining phases" section and mandates no implementation summary document; this workflow's own stages run the plan single-shot, review the result, and write the unit's completion report. Save it to docs/plans/${unit.id}-plan.md and commit. Return branch, SHA, path, the plan text verbatim, and the readings.
${RETURN_FIELDS}`;
}

function planCritiquePrompt(ctx, plan, unit, planText, round) {
  return `${header(ctx, plan)}

${unitBlock(plan, unit)}

Critique the implementation plan below with type critique-design. This is round ${round} of the critique gate. Pass bar: no Critical findings. Report Significant and Minor findings; they are recorded, not fixed. The plan's Readings section records how it read clauses that admitted more than one reading; a reading you judge wrong is a finding at the severity its consequence warrants, not a separate remark. Set \`systemic\` when the findings indicate a structural defect that iteration will not resolve. Continue through to completion — do not stop to wait for instruction.

--- implementation plan (docs/plans/${unit.id}-plan.md) ---
${planText}
--- end ---
${RETURN_FIELDS}`;
}

function fixPrompt(ctx, plan, unit, branch, subject, criticals, round) {
  return `${header(ctx, plan)}
${worktree(branch)}

${unitBlock(plan, unit)}

Critique gate round ${round} found these Critical findings against the ${subject}:

${JSON.stringify(criticals, null, 2)}

Fix each finding that is repairable within the ${subject}, then commit. A finding you decline to fix — because the repair would change the unit entry, the architecture document, or the request record, or because you cannot repair it — goes under \`notApplied\` with the reason. The run stops on a declined finding and the requester decides, so decline only when the repair is genuinely outside the ${subject}. ${subject === 'plan' ? 'Return the amended plan text verbatim in `planText`.' : 'Run the tests after fixing; a fix that breaks a test is not applied. Leave `planText` empty.'}
${RETURN_FIELDS}`;
}

function implementPrompt(ctx, plan, unit, planBranch) {
  return `${header(ctx, plan)}
${worktree(planBranch)}

${unitBlock(plan, unit)}

Implementation plan: docs/plans/${unit.id}-plan.md on this branch. Hold to its Readings section.

First re-probe every external gate in the unit entry against its plan-time pin: probe environment gates mechanically (binaries and versions, fonts, services, accounts, data sets); organisational and release gates are already cleared per the decision log and need no probe. Report each under \`gateProbe\`. If any gate has drifted, implement nothing — return the probe results with every other \`result\` field empty, zero, or false, and stop.

Otherwise execute the plan in full, single-shot, committing as you go with the project's commit discipline. There is no per-step critique here, and no implementation summary document: the completion report a later stage writes from your return is that record, so do not write \`docs/plans/${unit.id}-implementation.md\` even if the plan text mentions one. Then evaluate the done condition exactly as written, under the plan's readings, and report whether it is evaluable in position and whether it is met, with evidence. Record as they arise: deviations from the unit entry's scope or publishes; the files you changed that the Scope line does not name, under \`writesOutsideScope\`; deferrals — work in scope deliberately not implemented, naming the unit it belongs to; discoveries — anything you learned that changes another unit's scope, dependencies, or done condition, or that the architecture document states otherwise, naming the unit or the section; the status of each standing obligation the unit carries. Report execution distress honestly: whether context is near exhaustion, how many design decisions you took mid-implementation, the deviation and deferral counts.
${RETURN_FIELDS}`;
}

function codeCritiquePrompt(ctx, plan, unit, branch, round) {
  return `${header(ctx, plan)}
${worktree(branch)}

${unitBlock(plan, unit)}

Run the phronithm:critique skill with type critique-code over this branch's diff against ${ctx.integrationBranch} (\`git diff ${ctx.integrationBranch}...HEAD\`), reading surrounding code as needed. This is round ${round} of the critique gate. Pass bar: no Critical findings. Also evaluate the unit's done condition against this branch under the readings in docs/plans/${unit.id}-plan.md; a done condition that is not met, or a reading the code does not bear out, is a Critical finding. Set \`systemic\` when the findings indicate a structural defect that iteration will not resolve. Do not modify anything and do not spawn subagents. Continue through to completion — do not stop to wait for instruction.
${RETURN_FIELDS}`;
}

function lensReviewPrompt(ctx, plan, unit, branch, lenses, round) {
  return `${header(ctx, plan)}
${worktree(branch)}

${unitBlock(plan, unit)}

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

function completionPrompt(ctx, plan, unit, branch, impl, review) {
  return `${header(ctx, plan)}
${worktree(branch)}

${unitBlock(plan, unit)}

Write the completion report for this unit to docs/plans/${unit.id}-completion.md and commit. Contents, per phronithm:feature's completion-report: done-condition verdict with achieved rigour; deviations from the unit entry's scope and publishes, including files written outside the Scope line; deferrals, each naming the receiving unit; discoveries affecting sibling or downstream units or the architecture document; status of each standing obligation; external-gate re-verification results (state "none" when the entry names no gates); execution signals; review findings left unfixed, with severity; and the footer "Run \`/phronithm:breakdown\` in re-plan mode with the decomposition plan and this report before starting any dependent unit." Assemble it from the implement and review stage returns below; do not re-derive or soften them.

--- implement stage return ---
${JSON.stringify(impl, null, 2)}
--- review stage summary ---
${JSON.stringify(review, null, 2)}
--- end ---
${RETURN_FIELDS}`;
}

function mergePrompt(ctx, plan, wave, merges) {
  return `${header(ctx, plan)}

You are in the main checkout on ${ctx.integrationBranch}. Merge each unit branch below into ${ctx.integrationBranch}, in exactly this order, with a merge commit each (\`git merge --no-ff\`). For each unit report clean, resolved, unresolvable, or regressed, with the files involved and the merge SHA. Resolve a conflict only when the resolution is mechanical — both sides' changes apply without judgement about intent; otherwise abort that merge (\`git merge --abort\`), report it unresolvable, and continue with the next unit. Run the project's gates after each merge — the test suite and whatever else the project's own instructions name as a gate (linters, type checks). If a gate fails after a clean or resolved merge, undo that merge (\`git reset --hard\` to the pre-merge tip of ${ctx.integrationBranch}), report the unit as regressed with the failing test files, and continue with the next unit.

Then, for every unit merged clean or resolved, record a Plan Maintenance completion entry in ${plan.path} naming its completion report and set \`complete: true\` on that unit in the plan's \`\`\`yaml lsf-plan block; one commit on ${ctx.integrationBranch}; return the new plan SHA. Do not amend anything else in the plan; the re-plan stage does that.

Wave ${wave} merge order:
${merges.map((m) => `- ${m.unit}: branch ${m.branch} (completion report ${m.completionPath})`).join('\n')}
${RETURN_FIELDS}`;
}

function gatePrompt(ctx, plan, wave, merged) {
  return `${header(ctx, plan)}

Wave ${wave} of Phase 3 has merged these units into ${ctx.integrationBranch}: ${merged.map((m) => m.unit).join(', ')}. Their completion reports are at ${merged.map((m) => m.completionPath).join(', ')}.

Answer one question: do this wave's results invalidate the architecture document at ${ctx.architectureDoc} @ ${ctx.architectureSha ?? 'HEAD'} — does any decision the document makes rest on a premise the merged code or the completion reports show to be false? Read the reports, then the merged code the reports and the document point at.

For each such case return an invalidation with all five fields: \`section\` (the heading), \`decision\` (what the section decides, quoted or closely paraphrased), \`premise\` (what the decision rests on), \`evidence\` (file and line, test, or report line showing the premise false), and \`amendment\` (what the section should now say). An invalidation stops the run and is put to the requester with the amendment you propose, so fill every field; a case where you cannot say what the section should now say is a note, not an invalidation. A discrepancy that changes no decision — a stale line number, a signature widened compatibly, a count that does not match — is a note at most.

Return under \`notes\` what the re-plan or the requester should know that is not an invalidation: an open decision whose trigger has fired and which no unit owns; a discrepancy between a document and the code; a deviation or deferral the reports disclose that bears on a unit not yet started. Each note names a referent.

Do not re-evaluate the units' done conditions or review findings: the unit stages did that at the bar this workflow sets (Critical findings fixed, Significant and Minor recorded), and the completion reports carry the outcome. Do not report this workflow's own process as a defect. Do not modify anything.
${RETURN_FIELDS}`;
}

function replanPrompt(ctx, plan, wave, merged, conflicts, items, records) {
  return `${header(ctx, plan)}

You are in the main checkout on ${ctx.integrationBranch}. Run the phronithm:breakdown skill in re-plan mode over ${plan.path} with the completion reports of wave ${wave}: ${merged.map((m) => m.completionPath).join(', ')}. Diff their deviations, deferrals, and discoveries against every remaining unit's scope, dependencies, and done condition; amend the plan; re-run the quality check over the remaining units, examining the size budget first where a completed unit showed distress; record every amendment in Plan Maintenance. Treat each merge conflict below between units the plan claimed disjoint as a plan defect: record it and add the missing edge or scope correction. A file a unit wrote outside its Scope line is a Scope correction where a later unit shares the file.

Items this wave, each with an id — deferrals, discoveries, claimed-disjoint conflicts, the gate's notes, and distress. Each is yours to absorb: amend the plan (a Scope or Publishes line, a done condition, an edge, the execution order, a unit added), or show the plan as it stands already answers it. Return a verdict for every id: \`amended\`, \`no-change\` (with what in the plan answers it), or \`cannot\` (with why the plan cannot absorb it — the answer lies in the architecture document or the request record, or needs the requester). An id you do not answer is treated as \`cannot\` and stops the run.
${items.length ? JSON.stringify(items, null, 2) : 'none'}

Merge conflicts: ${conflicts.length ? JSON.stringify(conflicts, null, 2) : 'none'}
Other records this wave (readings, deviations, unfixed findings): ${records.length ? JSON.stringify(records, null, 2) : 'none'}

Every amendment to a unit's type, dependencies, scope, publishes, gates, or completion, and to the execution order, is made in both the unit's entry and the plan's \`\`\`yaml lsf-plan block, which must agree; the script reads the block. Commit the amended plan on ${ctx.integrationBranch}. Return the new plan SHA; each amendment with its kind (${AMENDMENT_KINDS.join(', ')}), unit, and statement; a verdict per item id; and every remaining unit over its size budget with whether you repaired it by extracting a preceding design unit or flagging wall-clock rationale. Do not answer whether the architecture document still stands — the gate did. Do not select or execute any unit.
${RETURN_FIELDS}`;
}

function terminationPrompt(ctx, plan) {
  return `${header(ctx, plan)}

You are in the main checkout on ${ctx.integrationBranch}. Every work unit in ${plan.path} is complete. Read its Meta-Tasks and Standing Obligations sections. Turn each substantial meta-task (integration testing, tuning infrastructure, anything needing its own implementation plan) into a work unit entry of type meta with a verifiable done condition, scope, dependencies, and gates, following phronithm:breakdown's unit conventions; append these to the plan's Work Units, to its execution order, and to its \`\`\`yaml lsf-plan block (entry and block must agree; the script reads the block), record the amendment in Plan Maintenance, and commit. List each small meta-task (documentation updates and the like) under \`directEdits\` with the path and what to change; do not make those edits. Return the new plan SHA and the ids of the appended units. If no meta-task remains, return the plan SHA given, unchanged.
${RETURN_FIELDS}`;
}

function directEditsPrompt(ctx, plan, edits) {
  return `${header(ctx, plan)}

You are in the main checkout on ${ctx.integrationBranch}. Make these small meta-task edits from the plan's Meta-Tasks section, commit them with the project's commit discipline, and return the path of the plan and the final commit SHA:

${JSON.stringify(edits, null, 2)}
${RETURN_FIELDS}`;
}

function obligationsPrompt(ctx, plan) {
  return `${header(ctx, plan)}

Every work unit and meta-task in ${plan.path} is complete. For each standing obligation in the plan's Standing Obligations section, determine from the completion reports and ${ctx.integrationBranch} whether it is discharged, and cite the evidence. Do not modify anything.
${RETURN_FIELDS}`;
}

function ledgerPrompt(ctx, plan, wave, entries) {
  return `${header(ctx, plan)}

Append stage. You are in the main checkout on ${ctx.integrationBranch}. Append the Markdown between the markers below to the end of ${ctx.ledgerPath}, every character unchanged, preceded by one blank line. If the file does not exist, create it with this heading first:

${ledgerHeading(ctx.name)}
Do not reword, reorder, merge, or drop rows, and never edit earlier content. Commit on ${ctx.integrationBranch} with the message "docs(${ctx.name}): append wave ${wave} signal ledger entries" and return the path and SHA.

<<<LEDGER
${renderLedger(entries, wave)}LEDGER>>>
${RETURN_FIELDS}`;
}

// ---------------------------------------------------------------- critique gate

// Bounded critique loop shared by the plan gate and the code review gate.
// `round(n)` returns [{ stage, ret }] for the round's reviewers; `fix(n,
// criticals)` returns the fix stage's return. Returns { ok, reason, rounds,
// findings } and pushes signals as it goes. Significant and Minor findings
// are recorded; a Critical the fix stage declines, a systemic verdict, or
// the round cap is a structural stop.
async function critiqueLoop({ round, fix, onFix, stageCtx, signals }) {
  const findings = [];
  for (let n = 1; n <= GATE_MAX_ROUNDS; n++) {
    const rets = await round(n);
    for (const { stage, ret } of rets) {
      if (!ret) {
        signals.push(nullReturnSignal(stageCtx(stage)));
        return { ok: false, reason: 'agent-null', rounds: n, findings };
      }
      signals.push(...signalsFromFindings(ret.result.findings, { ...stageCtx(stage), round: n }));
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
  signals.push(...signalsFromReadings(planned.result.readings, stageCtx('plan')));
  let branch = planned.result.branch;
  let planText = planned.result.planText;

  // 2. critique gate over the plan
  const planGate = await critiqueLoop({
    stageCtx, signals,
    round: async (n) => [{
      stage: 'critique-plan',
      ret: await rt.agent(planCritiquePrompt(ctx, plan, unit, planText, n),
        opts(`critique-plan:r${n}`, { agentType: 'phronithm:critic', schema: FINDINGS_SCHEMA, ...tier(ctx, 'critique') })),
    }],
    fix: (n, criticals) => rt.agent(fixPrompt(ctx, plan, unit, branch, 'plan', criticals, n),
      opts(`fix-plan:r${n}`, { isolation: 'worktree', schema: FIX_SCHEMA })),
    onFix: (fixed) => { branch = fixed.result.branch; planText = fixed.result.planText; },
  });
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
          ['critique-code', codeCritiquePrompt(ctx, plan, unit, branch, n)],
          ...(lenses.length ? [['lens-review', lensReviewPrompt(ctx, plan, unit, branch, lenses, n)]] : []),
        ];
      const rets = await rt.parallel(reviewers.map(([stage, prompt]) => () =>
        rt.agent(prompt, opts(`${stage}:r${n}`, { isolation: 'worktree', schema: FINDINGS_SCHEMA, ...tier(ctx, 'critique') }))));
      return reviewers.map(([stage], i) => ({ stage, ret: rets[i] ?? null }));
    },
    fix: (n, criticals) => rt.agent(fixPrompt(ctx, plan, unit, branch, 'code', criticals, n),
      opts(`fix-code:r${n}`, { isolation: 'worktree', schema: FIX_SCHEMA })),
    onFix: (fixed) => { branch = fixed.result.branch; },
  });
  if (!reviewGate.ok) return abort('review');

  // 5. completion report
  const review = { rounds: reviewGate.rounds, findings: reviewGate.findings };
  const report = await rt.agent(completionPrompt(ctx, plan, unit, branch, impl.result, review), opts('completion-report', { isolation: 'worktree', schema: COMPLETION_SCHEMA }));
  if (!report) { signals.push(nullReturnSignal(stageCtx('completion-report'))); return abort('completion-report'); }
  result.branch = report.result.branch;
  result.completionPath = report.result.path;
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

function requireArg(args, key) {
  if (args[key] === undefined || args[key] === null || args[key] === '') throw new Error(`lsf-implement: args.${key} is required`);
  return args[key];
}

async function run(rt) {
  const { agent, parallel, log } = rt;
  const args = rt.args ?? {};
  const name = requireArg(args, 'name');
  const ctx = {
    name,
    planSha: requireArg(args, 'planSha'),
    planPath: args.planPath ?? `docs/plans/${name}-breakdown.md`,
    architectureDoc: requireArg(args, 'architectureDoc'),
    architectureSha: args.architectureSha,
    requestRecord: args.requestRecord,
    integrationBranch: requireArg(args, 'integrationBranch'),
    decisionLogPath: args.decisionLogPath ?? `docs/plans/${name}-decisions.md`,
    decisionLogSha: args.decisionLogSha,
    ledgerPath: args.ledgerPath ?? `docs/plans/${name}-signals.md`,
    config: { ...DEFAULT_CONFIG, ...(args.config ?? {}) },
    maxWaves: args.maxWaves ?? DEFAULT_MAX_WAVES,
    maxUnitsPerWave: args.maxUnitsPerWave ?? Infinity,
    tiers: { ...DEFAULT_TIERS, ...(args.tiers ?? {}) },
  };

  const ledger = [];
  let plan = { path: ctx.planPath, sha: ctx.planSha, units: [], executionOrder: [] };
  let wave = 0;

  const outcome = (status, extra = {}) => ({
    status, workflow: WORKFLOW, planSha: plan.sha, integrationBranch: ctx.integrationBranch, waves: wave,
    ledger, digest: digest(ledger), ...extra,
  });
  const scribed = new Set();
  const scribeLedger = async (entries) => {
    const fresh = entries.filter((e) => !scribed.has(e));
    if (!fresh.length) return;
    for (const e of fresh) scribed.add(e);
    const r = await agent(ledgerPrompt(ctx, plan, wave, fresh), { label: `ledger:w${wave}`, ...tier(ctx, 'mechanical'), phase: 'Ledger', schema: SCRIBE_SCHEMA });
    if (!r) log(`ledger scribe returned null for wave ${wave}; entries are in the return value only`);
  };
  // Every stop path: stamp, ledger, scribe, escalate. `stops` may already be
  // in the ledger (from evaluateWave); the rest are appended here. Records
  // not yet scribed ride in the same append, so a stop costs one scribe.
  const stopped = async (stage, stops, records = []) => {
    for (const s of stops) s.disposition = 'stopped';
    ledger.push(...stops.filter((s) => !ledger.includes(s)));
    await scribeLedger([...records, ...stops]);
    log(`STOP at ${stage}: ${stops.map((s) => s.kind).join(', ')}`);
    return outcome('stopped', {
      escalation: escalation({ workflow: WORKFLOW, planSha: plan.sha, wave: wave || undefined, stage, signals: stops, decisionNeeded: decisionsFor(stops) }),
    });
  };
  const stopOn = stopped;

  // Decisions handed back by the requester go into the repo first, so the
  // SHA can appear in every prompt they affect.
  if (Array.isArray(args.decisions) && args.decisions.length) {
    const d = await agent(decisionLogPrompt(ctx, args.decisions), { label: 'decision-log', ...tier(ctx, 'mechanical'), phase: 'Load', schema: DECISION_LOG_SCHEMA });
    if (!d) return stopOn('decision-log', [nullReturnSignal({ stage: 'decision-log' })]);
    ctx.decisionLogSha = d.result.sha;
    ctx.decisionLogPath = d.result.path;
  }

  // Plan graph at a SHA. A copy agent returns the fenced blocks verbatim;
  // graph.js parses and validates. Structural problems (unreadable block,
  // unknown unit, cycle, unserialised overlap, bad clearance) are STOP
  // signals.
  const loadPlan = async (sha, label, w) => {
    const copy = await agent(copyPrompt(ctx, sha), { label, ...tier(ctx, 'mechanical'), phase: 'Load', schema: COPY_SCHEMA });
    if (!copy) return { plan: null, copy: null, stops: [nullReturnSignal({ stage: 'load', wave: w })] };
    const g = loadGraph({ planMarkdown: copy.result.planBlock, decisionsMarkdown: ctx.decisionLogSha ? copy.result.decisionsBlock : null });
    const stops = g.problems.map((p) => signal({ ...p, source: 'structural', stage: 'load', wave: w }));
    let loadedPlan = null;
    if (g.plan) {
      loadedPlan = { ...g.plan, path: ctx.planPath, sha };
      stops.push(...validatePlan(loadedPlan).map((p) => signal({ ...p, source: 'structural', stage: 'load', wave: w })));
    }
    return { plan: loadedPlan, copy, stops };
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
  if (loadEval.stop || !first.plan) return stopped('load', loadEval.stops, loadEval.records);
  plan = first.plan;
  log(`loaded ${plan.units.length} units @ ${plan.sha}; ${ctx.decisionLogSha ? 'decision log applied' : 'no decision log'}; projected waves: ${wavePlan(plan).map((w) => w.join('+')).join(' | ') || 'none'}`);

  const completed = new Set(plan.units.filter((u) => u.complete).map((u) => u.id));
  let terminationRan = false;

  for (;;) {
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
      const termEval = evaluateWave(re.stops);
      ledger.push(...termEval.all);
      if (termEval.stop) return stopped('termination', termEval.stops, termEval.records);
      plan = re.plan;
      if (term.result.directEdits.length) {
        const edits = await agent(directEditsPrompt(ctx, plan, term.result.directEdits), { label: 'direct-edits', ...tier(ctx, 'default'), phase: 'Termination', schema: SCRIBE_SCHEMA });
        if (!edits) return stopOn('direct-edits', [nullReturnSignal({ stage: 'direct-edits' })]);
      }
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

    // Merge, in plan execution order.
    let merged = [];
    let conflicts = [];
    if (order.length) {
      const m = await agent(mergePrompt(ctx, plan, wave, order), { label: `merge:w${wave}`, ...tier(ctx, 'default'), phase: 'Merge', schema: MERGE_SCHEMA });
      if (!m) {
        waveSignals.push(nullReturnSignal({ stage: 'merge', wave }));
      } else {
        waveSignals.push(...signalsFromMerge(m.result, { wave }));
        for (const x of m.result.merges) {
          if (x.status === 'clean' || x.status === 'resolved') { completed.add(x.unit); merged.push(byUnit.get(x.unit)); }
          if (x.status !== 'clean') conflicts.push(x);
        }
        plan = { ...plan, sha: m.result.planSha, units: plan.units.map((u) => (completed.has(u.id) ? { ...u, complete: true } : u)) };
      }
    }

    // The wave gate: one question, does the wave invalidate the architecture.
    if (merged.length) {
      const g = await agent(gatePrompt(ctx, plan, wave, merged), { label: `review-gate:w${wave}`, ...tier(ctx, 'gate'), phase: 'Gate', schema: GATE_SCHEMA });
      if (!g) waveSignals.push(nullReturnSignal({ stage: 'review-gate', wave }));
      else waveSignals.push(...signalsFromGate(g.result, { wave }));
    }

    const evalA = evaluateWave(waveSignals);
    ledger.push(...evalA.all);
    log(`wave ${wave}: merged ${merged.map((r) => r.unit).join(', ') || 'nothing'}; ${conflicts.length} conflict(s); ${evalA.records.length} recorded, ${evalA.stops.length} stop signal(s)`);
    if (evalA.stop) return stopped('wave', evalA.stops, evalA.records);

    // Re-plan from the wave's completion reports. Items go in with ids; one
    // the re-plan cannot absorb, or does not answer, is a stop.
    const flat = ({ unit, stage, kind, statement, referent }) => ({ unit, stage, kind, statement, referent });
    const itemSignals = evalA.records.filter((s) => REPLAN_ITEM_KINDS.includes(s.kind));
    const items = itemSignals.map((s, i) => ({ id: `rp-${i + 1}`, ...flat(s) }));
    const records = evalA.records.filter((s) => !REPLAN_ITEM_KINDS.includes(s.kind)).map(flat);
    const rp = await agent(replanPrompt(ctx, plan, wave, merged, conflicts, items, records), { label: `replan:w${wave}`, ...tier(ctx, 'planning'), schema: REPLAN_SCHEMA, phase: 'Re-plan' });
    const replanSignals = [];
    let nextPlan = plan;
    if (!rp) {
      replanSignals.push(nullReturnSignal({ stage: 'replan', wave }));
    } else {
      const verdicts = new Map((rp.result.verdicts ?? []).map((v) => [v.id, v]));
      items.forEach((it, i) => {
        const v = verdicts.get(it.id);
        if (v && v.verdict !== 'cannot') {
          replanSignals.push(signal({
            kind: 'verdict', source: 'reported', stage: 'replan', unit: it.unit, wave,
            statement: `${it.id} ${v.verdict}: ${v.statement} — [${it.stage}] ${it.kind}: ${it.statement}`, referent: itemSignals[i].referent,
          }));
          return;
        }
        replanSignals.push(signal({
          kind: 'plan-defect-unabsorbed', source: 'reported', stage: 'replan', unit: it.unit, wave,
          statement: `${v ? `re-plan cannot absorb: ${v.statement}` : 're-plan gave no verdict'} — [${it.stage}] ${it.kind}: ${it.statement}`,
          referent: itemSignals[i].referent,
        }));
      });
      for (const a of rp.result.amendments) {
        replanSignals.push(signal({ kind: 'amendment', source: 'reported', stage: 'replan', unit: a.unit, wave, statement: `${a.kind}: ${a.statement}`, referent: a.unit }));
      }
      for (const b of rp.result.overBudget) {
        replanSignals.push(signal({
          kind: b.repaired ? 'over-budget-repaired' : 'unit-over-budget', source: 'reported', stage: 'replan', unit: b.unit, wave,
          statement: b.statement, referent: b.unit,
        }));
      }
      const re = await loadPlan(rp.result.planSha, `load:w${wave}`, wave);
      replanSignals.push(...re.stops);
      if (re.plan) nextPlan = re.plan;
    }
    const evalB = evaluateWave(replanSignals);
    ledger.push(...evalB.all);
    const waveRecords = [...evalA.all, ...evalB.records];
    if (evalB.stop) return stopped('replan', evalB.stops, waveRecords);
    await scribeLedger(waveRecords);
    plan = nextPlan;
    for (const u of plan.units) if (u.complete) completed.add(u.id);
    log(`wave ${wave}: re-plan ${rp.result.amendments.length} amendment(s) → plan @ ${plan.sha}; ${rp.result.summary}`);
  }

  // Standing obligations.
  const ob = await agent(obligationsPrompt(ctx, plan), { label: 'obligations', ...tier(ctx, 'gate'), phase: 'Termination', schema: OBLIGATIONS_SCHEMA });
  if (!ob) return stopOn('obligations', [nullReturnSignal({ stage: 'obligations' })]);
  const obSignals = [];
  for (const o of ob.result.obligations.filter((o) => !o.discharged)) {
    obSignals.push(signal({
      kind: 'standing-obligation-undischarged', source: 'structural', stage: 'obligations',
      statement: o.statement, referent: o.statement,
    }));
  }
  const obEval = evaluateWave(obSignals);
  ledger.push(...obEval.all);
  if (obEval.stop) return stopped('obligations', obEval.stops, obEval.records);

  log(`complete: ${completed.size} unit(s) over ${wave} wave(s); silence is not evidence`);
  return outcome('complete');
}

export { run, critiqueLoop, runUnit, LENSES_BY_TYPE, GATE_MAX_ROUNDS };
