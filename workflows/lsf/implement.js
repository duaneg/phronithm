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
//   tiers               per-role { model, effort } overrides; roles: gate
//                       (critique and review), planning (unit plan, re-plan,
//                       termination), mechanical (plan-block copy, ledger and
//                       decision-log appends),
//                       default (everything else)

import {
  stageSchema, MERGE_RESULT_SCHEMA, DISTRESS_SCHEMA, RECORD_KINDS, signal, escalation,
} from './schema.js';
import { loadGraph } from './graph.js';
import {
  DEFAULT_CONFIG, validatePlan, readySet, blockedUnits, mergeOrder, wavePlan,
  signalsFromStageReturn, nullReturnSignal, signalsFromMerge, distressSignals, evaluateWave, digest, renderLedger, ledgerHeading,
} from './scheduling.js';

const WORKFLOW = 'lsf-implement';
const GATE_MAX_ROUNDS = 4; // critique gate hard cap
const DEFAULT_MAX_WAVES = 20;
// Model and effort per role. Static analysis inherits the session model.
const DEFAULT_TIERS = Object.freeze({
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
  notApplied: arr(obj({ finding: str, reason: str, altersRequirement: bool })),
  planText: { ...str, description: 'The amended plan verbatim, when the subject is a plan; empty otherwise' },
}));

const IMPLEMENT_SCHEMA = stageSchema(obj({
  ...COMMIT_FIELDS,
  gateProbe: arr(obj({ name: str, pin: str, observed: str, drifted: bool })),
  distress: DISTRESS_SCHEMA,
  deviations: strings,
  deferrals: arr(obj({ statement: str, belongsTo: str })),
  discoveries: strings,
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
  invalidatesArchitecture: bool,
  handoff: { type: 'string', enum: ['sufficient', 'partial', 'insufficient'] },
  exitCriteria: arr(obj({ unit: str, met: bool, statement: str })),
}));

const AMENDMENT_KINDS = RECORD_KINDS.filter((k) => !['assumption', 'design-ambiguity'].includes(k));

const REPLAN_SCHEMA = stageSchema(obj({
  planSha: { ...str, description: 'Plan SHA after the amendments were committed; the plan SHA given when nothing changed' },
  amendments: arr(obj({ kind: { type: 'string', enum: AMENDMENT_KINDS }, unit: str, statement: str })),
  overBudget: arr(obj({ unit: str, repaired: bool, statement: str })),
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
Besides \`result\`, return: \`assumptions\` — each thing you took as given that the inputs do not state, with the referent it concerns; \`ambiguities\` — each point where the architecture document, the request record, the decomposition plan, or the unit entry leaves open what is built, in what order, or with what scope, classified as scope, goal, requirement, or design, with the referent it concerns (a section, symbol, unit, or gate). Only those documents can be ambiguous. An open point in these instructions, in the return schema, or in how to report (which enum value fits, how many commits, whether a template field applies) is not an ambiguity: resolve it the plainest way and record it as an assumption. No item goes under both; \`contradictions\` — each thing you observed that contradicts the architecture document, the request record, or the unit entry, citing the document path and SHA; \`commits\` — every commit you made, branch and SHA. Empty arrays are valid. Do not pad them and do not omit a real one. An item already listed under "Already recorded" in these instructions is in the ledger: do not report it again in any field, in any wording; report only what is new. The return is one flat object with these four fields beside \`result\`, never inside it.`;

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

function unitBlock(plan, unit) {
  return `Work unit: ${unit.id} (type: ${unit.type})\nUnit entry: the \`### ${unit.id}\` section under \`## Work Units\` in ${plan.path} @ ${plan.sha}. Read it first (\`git show ${plan.sha}:${plan.path}\`); it is requirement text, taken verbatim.`;
}

// Signals earlier stages recorded, handed to later stages so each finding
// enters the ledger once instead of once per stage.
function priorBlock(signals, scope) {
  if (!signals.length) return '';
  const rows = signals.map((s) => `- [${s.stage}] ${s.kind}: ${s.statement} (referent: ${s.referent})`);
  return `

Already recorded ${scope} (in the ledger; do not report these again, resolved or not — only what is new):
${rows.join('\n')}`;
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

Do not read, assess, or summarise the prose. Do not correct, reformat, or complete anything in the blocks; do not add units, edges, or gates. This stage copies; the script parses and checks the blocks itself. Anything you would report about the plan's content is outside this stage's scope: leave \`assumptions\`, \`ambiguities\`, and \`contradictions\` empty unless the copy itself could not be made faithfully.
${RETURN_FIELDS}`;
}

function unitPlanPrompt(ctx, plan, unit) {
  return `${header(ctx, plan)}
${worktree(ctx.integrationBranch)}

${unitBlock(plan, unit)}

Write the implementation plan for this unit following the phronithm:feature skill's Phase 3 in work-unit mode: the architecture document is the primary design input; the unit's done condition, adopted verbatim, is a success criterion; each standing obligation naming the unit is a requirement. Do not re-explore what the architecture document and unit entry settle. The plan must be self-contained — a fresh context can implement from it alone — and carry the work-unit context section (unit entry verbatim, decomposition plan path and SHA, standing obligations). Keep it within the unit's size budget: about six steps, at most one open design decision. This workflow's own stages replace feature's later phases: the implement stage runs the plan single-shot, a critique gate and lens review follow, and a completion report is written for the unit — so the plan carries no "Remaining phases" section and mandates no implementation summary (\`docs/plans/${unit.id}-implementation.md\`); the completion report is that record. Save it to docs/plans/${unit.id}-plan.md and commit. Return branch, SHA, path, and the plan text verbatim.
${RETURN_FIELDS}`;
}

function planCritiquePrompt(ctx, plan, unit, planText, round) {
  return `${header(ctx, plan)}

${unitBlock(plan, unit)}

Critique the implementation plan below with type critique-design. This is round ${round} of the critique gate. Pass bar: no Critical findings. Report Significant and Minor findings; they are recorded, not fixed. Set \`systemic\` when the findings indicate a structural defect that iteration will not resolve. Continue through to completion — do not stop to wait for instruction.

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

Fix each finding that is repairable within the ${subject}, then commit. Before applying any fix, classify it: if the fix would alter the unit entry, the architecture document, or the request record, apply nothing for that finding, list it under \`notApplied\` with \`altersRequirement: true\`, and record it under \`contradictions\` against the document it would alter. ${subject === 'plan' ? 'Return the amended plan text verbatim in `planText`.' : 'Run the tests after fixing; a fix that breaks a test is not applied. Leave `planText` empty.'}
${RETURN_FIELDS}`;
}

function implementPrompt(ctx, plan, unit, planBranch) {
  return `${header(ctx, plan)}
${worktree(planBranch)}

${unitBlock(plan, unit)}

Implementation plan: docs/plans/${unit.id}-plan.md on this branch.

First re-probe every external gate in the unit entry against its plan-time pin: probe environment gates mechanically (binaries and versions, fonts, services, accounts, data sets); organisational and release gates are already cleared per the decision log and need no probe. Report each under \`gateProbe\`. If any gate has drifted, implement nothing — return the probe results with every other \`result\` field empty, zero, or false, and stop.

Otherwise execute the plan in full, single-shot, committing as you go with the project's commit discipline. There is no per-step critique here, and no implementation summary document: the completion report a later stage writes from your return is that record, so do not write \`docs/plans/${unit.id}-implementation.md\` even if the plan text mentions one. Then evaluate the done condition exactly as written and report whether it is evaluable in position and whether it is met, with evidence. Record as they arise: deviations from the unit entry's scope or publishes; deferrals — work in scope deliberately not implemented, naming the unit it belongs to; discoveries that add work to, reassign work between, or invalidate an assumption of another unit; the status of each standing obligation the unit carries. Report execution distress honestly: whether context is near exhaustion, how many design decisions you took mid-implementation, the deviation and deferral counts.
${RETURN_FIELDS}`;
}

function staticAnalysisPrompt(ctx, plan, unit, branch) {
  return `${header(ctx, plan)}
${worktree(branch)}

${unitBlock(plan, unit)}

Run the phronithm:static-analysis skill over the files this branch changes relative to ${ctx.integrationBranch} (\`git diff --name-only ${ctx.integrationBranch}...HEAD\`). Return each finding with a severity: Critical for a defect the analyser is confident of (type error, definite bug pattern, security rule), Significant for a probable issue, Minor for style. Do not modify anything.
${RETURN_FIELDS}`;
}

function codeCritiquePrompt(ctx, plan, unit, branch, round) {
  return `${header(ctx, plan)}
${worktree(branch)}

${unitBlock(plan, unit)}

Run the phronithm:critique skill with type critique-code over this branch's diff against ${ctx.integrationBranch} (\`git diff ${ctx.integrationBranch}...HEAD\`), reading surrounding code as needed. This is round ${round} of the critique gate. Pass bar: no Critical findings. Also evaluate the unit's done condition against this branch; a done condition that is not met is a Critical finding. Set \`systemic\` when the findings indicate a structural defect that iteration will not resolve. Do not modify anything and do not spawn subagents. Continue through to completion — do not stop to wait for instruction.
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

Write the completion report for this unit to docs/plans/${unit.id}-completion.md and commit. Contents, per phronithm:feature's completion-report: done-condition verdict with achieved rigour; deviations from the unit entry's scope and publishes; deferrals, each naming the receiving unit; discoveries affecting sibling or downstream units; status of each standing obligation; external-gate re-verification results; execution signals; and the footer "Run \`/phronithm:breakdown\` in re-plan mode with the decomposition plan and this report before starting any dependent unit." Assemble it from the implement and review stage returns below; do not re-derive or soften them.

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

Then, for every unit merged clean or resolved, record a Plan Maintenance completion entry in ${plan.path} naming its completion report and set \`complete: true\` on that unit in the plan's \`\`\`yaml lsf-plan block; commit on ${ctx.integrationBranch} and return the new plan SHA. Do not amend anything else in the plan.

Wave ${wave} merge order:
${merges.map((m) => `- ${m.unit}: branch ${m.branch} (completion report ${m.completionPath})`).join('\n')}
${RETURN_FIELDS}`;
}

function gatePrompt(ctx, plan, wave, merged, waveSignals) {
  return `${header(ctx, plan)}

Wave ${wave} of Phase 3 has merged these units into ${ctx.integrationBranch}: ${merged.map((m) => m.unit).join(', ')}. Their completion reports are at ${merged.map((m) => m.completionPath).join(', ')}.

Answer the phronithm:large-scale-feature review-gate questions for this wave. Do the units' discoveries, deviations, and deferrals invalidate any assumption or decision in the architecture document? Report each invalidation under \`contradictions\` against the architecture document, and set \`invalidatesArchitecture\`. Does each unit meet its exit criteria? Exit criteria are the unit entry's done condition as written, evaluated at the rigour it states, plus a completion report that is complete; a higher bar you would prefer is a contradiction or an ambiguity to report, not an unmet criterion. Is the handoff state self-contained — could a fresh context continue Phase 3 from the committed documents alone? Report new ambiguities under \`ambiguities\`. Do not modify anything.

How this workflow runs, so you do not report its design as a defect: each unit's critique gates pass on no Critical findings; Significant and Minor findings are recorded in the ledger and not fixed. The re-plan stage runs after this gate, from the completion reports and your answers, so its absence now is expected. The units of this wave were selected by the script from the plan's ready set, capped by argument, not chosen by any agent.${priorBlock(waveSignals, 'by this wave\'s stages')}
${RETURN_FIELDS}`;
}

function replanPrompt(ctx, plan, wave, merged, conflicts, recorded) {
  return `${header(ctx, plan)}

You are in the main checkout on ${ctx.integrationBranch}. Run the phronithm:breakdown skill in re-plan mode over ${plan.path} with the completion reports of wave ${wave}: ${merged.map((m) => m.completionPath).join(', ')}. Diff their deviations, deferrals, and discoveries against every remaining unit's scope, dependencies, and done condition; amend the plan; re-run the quality check over the remaining units, examining the size budget first where a completed unit showed distress; record every amendment in Plan Maintenance. Treat each merge conflict below between units the plan claimed disjoint as a plan defect: record it and add the missing edge or scope correction.

Merge conflicts: ${conflicts.length ? JSON.stringify(conflicts, null, 2) : 'none'}
Recorded signals this wave: ${recorded.length ? JSON.stringify(recorded, null, 2) : 'none'}

Every amendment to a unit's type, dependencies, scope, publishes, gates, or completion, and to the execution order, is made in both the unit's entry and the plan's \`\`\`yaml lsf-plan block, which must agree; the script reads the block. Commit the amended plan on ${ctx.integrationBranch}. Return the new plan SHA; each amendment with its kind (${AMENDMENT_KINDS.join(', ')}), unit, and statement; and every remaining unit over its size budget with whether you repaired it by extracting a preceding design unit or flagging wall-clock rationale. Do not answer whether the architecture document still stands — a separate stage does that. Do not select or execute any unit.
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
Do not reword, reorder, merge, or drop rows, and never edit earlier content. Commit on ${ctx.integrationBranch} with the message "docs(${ctx.name}): append wave ${wave} signal ledger entries" and return the path and SHA. Leave \`assumptions\`, \`ambiguities\`, and \`contradictions\` empty unless the append itself could not be made faithfully.

<<<LEDGER
${renderLedger(entries, wave)}LEDGER>>>
${RETURN_FIELDS}`;
}

// ---------------------------------------------------------------- critique gate

// Bounded critique loop shared by the plan gate and the code review gate.
// `round(n)` returns [{ stage, ret }] for the round's reviewers; `fix(n,
// criticals)` returns the fix stage's return. Returns { ok, reason, rounds,
// findings } and pushes signals as it goes.
async function critiqueLoop({ round, fix, onFix, collect, stageCtx, signals }) {
  const findings = [];
  for (let n = 1; n <= GATE_MAX_ROUNDS; n++) {
    const rets = await round(n);
    for (const { stage, ret } of rets) {
      if (!ret) {
        signals.push(nullReturnSignal(stageCtx(stage)));
        return { ok: false, reason: 'agent-null', rounds: n, findings };
      }
      collect(ret, stage);
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
    collect(fixed, 'fix');
    for (const a of fixed.result.applied) {
      signals.push(signal({
        ...stageCtx('fix'), kind: 'critical-fixed', source: 'structural',
        statement: `round ${n}: ${a.finding} — fixed: ${a.fix}`, referent: a.finding,
      }));
    }
    const altering = fixed.result.notApplied.filter((x) => x.altersRequirement);
    if (altering.length) {
      // The fix agent also reports these under contradictions; this is the
      // structural guarantee that they stop the loop.
      for (const a of altering) {
        signals.push(signal({
          ...stageCtx('fix'), kind: 'assumption-invalidated', source: 'structural',
          statement: `fix not applied, would alter a requirement: ${a.finding} — ${a.reason}`,
          referent: stageCtx().unit,
        }));
      }
      return { ok: false, reason: 'requirement-altering-fix', rounds: n, findings };
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
  const collect = (ret, stage) => signals.push(...signalsFromStageReturn(ret, stageCtx(stage)));
  const result = { unit: unit.id, type: unit.type, branch: null, completionPath: null, distress: null, signals, aborted: null };
  const abort = (stage) => { result.aborted = stage; return result; };
  const opts = (label, extra = {}) => ({ label: `${label}:${unit.id}`, phase: 'Units', ...tier(ctx, 'default'), ...extra });
  // Every stage after the plan sees what the unit's earlier stages recorded.
  const prior = (prompt) => prompt + priorBlock(signals, `for this unit by earlier stages`);

  // 1. plan
  const planned = await rt.agent(unitPlanPrompt(ctx, plan, unit), opts('plan', { isolation: 'worktree', schema: UNIT_PLAN_SCHEMA, ...tier(ctx, 'planning') }));
  if (!planned) { signals.push(nullReturnSignal(stageCtx('plan'))); return abort('plan'); }
  collect(planned, 'plan');
  let branch = planned.result.branch;
  let planText = planned.result.planText;

  // 2. critique gate over the plan
  const planGate = await critiqueLoop({
    collect, stageCtx, signals,
    round: async (n) => [{
      stage: 'critique-plan',
      ret: await rt.agent(prior(planCritiquePrompt(ctx, plan, unit, planText, n)),
        opts(`critique-plan:r${n}`, { agentType: 'phronithm:critic', schema: FINDINGS_SCHEMA, ...tier(ctx, 'gate') })),
    }],
    fix: (n, criticals) => rt.agent(prior(fixPrompt(ctx, plan, unit, branch, 'plan', criticals, n)),
      opts(`fix-plan:r${n}`, { isolation: 'worktree', schema: FIX_SCHEMA })),
    onFix: (fixed) => { branch = fixed.result.branch; planText = fixed.result.planText; },
  });
  if (!planGate.ok) return abort('critique-plan');

  // 3. implement
  const impl = await rt.agent(prior(implementPrompt(ctx, plan, unit, branch)), opts('implement', { isolation: 'worktree', schema: IMPLEMENT_SCHEMA }));
  if (!impl) { signals.push(nullReturnSignal(stageCtx('implement'))); return abort('implement'); }
  collect(impl, 'implement');
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
    collect, stageCtx, signals,
    round: async (n) => {
      const reviewers = unit.type === 'design'
        ? [['critique-design', designCritiquePrompt(ctx, plan, unit, branch, n)]]
        : [
          ['static-analysis', staticAnalysisPrompt(ctx, plan, unit, branch)],
          ['critique-code', codeCritiquePrompt(ctx, plan, unit, branch, n)],
          ...(lenses.length ? [['lens-review', lensReviewPrompt(ctx, plan, unit, branch, lenses, n)]] : []),
        ];
      const rets = await rt.parallel(reviewers.map(([stage, prompt]) => () =>
        rt.agent(prior(prompt), opts(`${stage}:r${n}`, {
          isolation: 'worktree', schema: FINDINGS_SCHEMA,
          ...(stage === 'static-analysis' ? { model: undefined, effort: undefined } : tier(ctx, 'gate')),
        }))));
      return reviewers.map(([stage], i) => ({ stage, ret: rets[i] ?? null }));
    },
    fix: (n, criticals) => rt.agent(prior(fixPrompt(ctx, plan, unit, branch, 'code', criticals, n)),
      opts(`fix-code:r${n}`, { isolation: 'worktree', schema: FIX_SCHEMA })),
    onFix: (fixed) => { branch = fixed.result.branch; },
  });
  if (!reviewGate.ok) return abort('review');

  // 5. completion report
  const review = { rounds: reviewGate.rounds, findings: reviewGate.findings };
  const report = await rt.agent(prior(completionPrompt(ctx, plan, unit, branch, impl.result, review)), opts('completion-report', { isolation: 'worktree', schema: COMPLETION_SCHEMA }));
  if (!report) { signals.push(nullReturnSignal(stageCtx('completion-report'))); return abort('completion-report'); }
  collect(report, 'completion-report');
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
  const unitsPerWave = {};
  let plan = { path: ctx.planPath, sha: ctx.planSha, units: [], executionOrder: [] };
  let wave = 0;

  const outcome = (status, extra = {}) => ({
    status, workflow: WORKFLOW, planSha: plan.sha, integrationBranch: ctx.integrationBranch, waves: wave,
    ledger, digest: digest(ledger, { unitsPerWave }), ...extra,
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
  // graph.js parses and validates. The copy agent has no interpretive
  // authority: its reported ambiguities and contradictions are recorded,
  // never stops. Structural problems (unreadable block, unknown unit, cycle,
  // unserialised overlap, bad clearance) are STOP signals.
  const loadPlan = async (sha, label, w) => {
    const copy = await agent(copyPrompt(ctx, sha), { label, ...tier(ctx, 'mechanical'), phase: 'Load', schema: COPY_SCHEMA });
    if (!copy) return { plan: null, copy: null, records: [], stops: [nullReturnSignal({ stage: 'load', wave: w })] };
    const records = signalsFromStageReturn(copy, { stage: 'load', wave: w }).map((x) => ({ ...x, disposition: 'recorded' }));
    const g = loadGraph({ planMarkdown: copy.result.planBlock, decisionsMarkdown: ctx.decisionLogSha ? copy.result.decisionsBlock : null });
    const stops = g.problems.map((p) => signal({ ...p, source: 'structural', stage: 'load', wave: w }));
    let loadedPlan = null;
    if (g.plan) {
      loadedPlan = { ...g.plan, path: ctx.planPath, sha };
      stops.push(...validatePlan(loadedPlan).map((p) => signal({ ...p, source: 'structural', stage: 'load', wave: w })));
    }
    return { plan: loadedPlan, copy, records, stops };
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
  ledger.push(...first.records);
  const loadEval = evaluateWave(loadSignals, [], { wave: 0, unitsInWave: 0 }, ctx.config);
  ledger.push(...loadEval.all);
  if (loadEval.stop || !first.plan) return stopped('load', loadEval.stops, [...first.records, ...loadEval.records]);
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
      const termSignals = signalsFromStageReturn(term, { stage: 'termination' });
      const re = await loadPlan(term.result.planSha, 'load:termination', wave);
      ledger.push(...re.records);
      termSignals.push(...re.stops);
      const termEval = evaluateWave(termSignals, ledger, { wave, unitsInWave: 0 }, ctx.config);
      ledger.push(...termEval.all);
      if (termEval.stop) return stopped('termination', termEval.stops, termEval.records);
      plan = re.plan;
      if (term.result.directEdits.length) {
        const edits = await agent(directEditsPrompt(ctx, plan, term.result.directEdits), { label: 'direct-edits', ...tier(ctx, 'default'), phase: 'Termination', schema: SCRIBE_SCHEMA });
        if (!edits) return stopOn('direct-edits', [nullReturnSignal({ stage: 'direct-edits' })]);
        ledger.push(...signalsFromStageReturn(edits, { stage: 'direct-edits' }).map((s) => ({ ...s, disposition: 'recorded' })));
      }
      await scribeLedger(termEval.records);
      log(`termination: ${term.result.appendedUnits.length} meta unit(s) appended, ${term.result.directEdits.length} direct edit(s)`);
      continue;
    }

    wave += 1;
    if (wave > ctx.maxWaves) {
      return outcome('wave-cap', { note: `stopped after ${ctx.maxWaves} waves (args.maxWaves); ${plan.units.length - completed.size} unit(s) remain` });
    }
    unitsPerWave[wave] = ready.length;
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
        waveSignals.push(...signalsFromStageReturn(m, { stage: 'merge', wave }));
        waveSignals.push(...signalsFromMerge(m.result, { wave }));
        for (const x of m.result.merges) {
          if (x.status === 'clean' || x.status === 'resolved') { completed.add(x.unit); merged.push(byUnit.get(x.unit)); }
          if (x.status !== 'clean') conflicts.push(x);
        }
        plan = { ...plan, sha: m.result.planSha, units: plan.units.map((u) => (completed.has(u.id) ? { ...u, complete: true } : u)) };
      }
    }

    // Review-gate questions for the wave.
    if (merged.length) {
      const g = await agent(gatePrompt(ctx, plan, wave, merged, waveSignals), { label: `review-gate:w${wave}`, ...tier(ctx, 'gate'), phase: 'Gate', schema: GATE_SCHEMA });
      if (!g) waveSignals.push(nullReturnSignal({ stage: 'review-gate', wave }));
      else {
        waveSignals.push(...signalsFromStageReturn(g, { stage: 'review-gate', wave, source: 'gate' }));
        if (g.result.invalidatesArchitecture && !(g.contradictions ?? []).length) {
          waveSignals.push(signal({
            kind: 'assumption-invalidated', source: 'gate', stage: 'review-gate', wave,
            statement: 'review gate reports the wave invalidates the architecture document (no contradiction itemised)',
            referent: `${ctx.architectureDoc} @ ${ctx.architectureSha}`,
          }));
        }
        if (g.result.handoff !== 'sufficient') {
          waveSignals.push(signal({
            kind: g.result.handoff === 'insufficient' ? 'handoff-insufficient' : 'handoff-partial', source: 'gate', stage: 'review-gate', wave,
            statement: `handoff state rated ${g.result.handoff}`, referent: `wave-${wave}`,
          }));
        }
        for (const c of g.result.exitCriteria.filter((c) => !c.met)) {
          waveSignals.push(signal({
            kind: 'done-condition-unevaluable', source: 'gate', stage: 'review-gate', unit: c.unit, wave,
            statement: `exit criteria not met: ${c.statement}`, referent: c.statement,
          }));
        }
      }
    }

    const evalA = evaluateWave(waveSignals, ledger, { wave, unitsInWave: ready.length }, ctx.config);
    ledger.push(...evalA.all);
    log(`wave ${wave}: merged ${merged.map((r) => r.unit).join(', ') || 'nothing'}; ${conflicts.length} conflict(s); ${evalA.records.length} recorded, ${evalA.stops.length} stop signal(s)`);
    if (evalA.stop) return stopped('wave', evalA.stops, evalA.records);

    // Re-plan from the wave's completion reports.
    const recorded = evalA.records.map(({ unit, stage, kind, statement, referent }) => ({ unit, stage, kind, statement, referent }));
    const rp = await agent(replanPrompt(ctx, plan, wave, merged, conflicts, recorded), { label: `replan:w${wave}`, ...tier(ctx, 'planning'), phase: 'Re-plan', schema: REPLAN_SCHEMA });
    const replanSignals = [];
    let nextPlan = plan;
    if (!rp) {
      replanSignals.push(nullReturnSignal({ stage: 'replan', wave }));
    } else {
      replanSignals.push(...signalsFromStageReturn(rp, { stage: 'replan', wave }));
      for (const a of rp.result.amendments) {
        replanSignals.push(signal({ kind: a.kind, source: 'reported', stage: 'replan', unit: a.unit, wave, statement: a.statement, referent: a.unit }));
      }
      for (const b of rp.result.overBudget) {
        replanSignals.push(signal({
          kind: b.repaired ? 'over-budget-repaired' : 'unit-over-budget', source: 'reported', stage: 'replan', unit: b.unit, wave,
          statement: b.statement, referent: b.unit,
        }));
      }
      const re = await loadPlan(rp.result.planSha, `load:w${wave}`, wave);
      ledger.push(...re.records);
      replanSignals.push(...re.stops);
      if (re.plan) nextPlan = re.plan;
    }
    const evalB = evaluateWave(replanSignals, ledger, { wave, unitsInWave: 0 }, ctx.config);
    ledger.push(...evalB.all);
    const waveRecords = [...evalA.all, ...evalB.records, ...ledger.filter((x) => x.stage === 'load' && x.wave === wave && x.disposition === 'recorded')];
    if (evalB.stop) return stopped('replan', evalB.stops, waveRecords);
    await scribeLedger(waveRecords);
    plan = nextPlan;
    for (const u of plan.units) if (u.complete) completed.add(u.id);
    log(`wave ${wave}: re-plan ${rp.result.amendments.length} amendment(s) → plan @ ${plan.sha}; ${rp.result.summary}`);
  }

  // Standing obligations.
  const ob = await agent(obligationsPrompt(ctx, plan), { label: 'obligations', ...tier(ctx, 'gate'), phase: 'Termination', schema: OBLIGATIONS_SCHEMA });
  if (!ob) return stopOn('obligations', [nullReturnSignal({ stage: 'obligations' })]);
  const obSignals = signalsFromStageReturn(ob, { stage: 'obligations' });
  for (const o of ob.result.obligations.filter((o) => !o.discharged)) {
    obSignals.push(signal({
      kind: 'standing-obligation-undischarged', source: 'structural', stage: 'obligations',
      statement: o.statement, referent: o.statement,
    }));
  }
  const obEval = evaluateWave(obSignals, ledger, { wave, unitsInWave: 0 }, ctx.config);
  ledger.push(...obEval.all);
  if (obEval.stop) return stopped('obligations', obEval.stops, obEval.records);
  await scribeLedger(obEval.records);

  log(`complete: ${completed.size} unit(s) over ${wave} wave(s); silence is not evidence`);
  return outcome('complete');
}

export { run, critiqueLoop, runUnit, LENSES_BY_TYPE, GATE_MAX_ROUNDS };
