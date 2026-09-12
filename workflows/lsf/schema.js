// Shared schema for the lsf-* workflows. Dependency-free; inlined into the
// workflow scripts (the script cannot import). This file is the source of
// truth. See docs/design/lsf-workflow.md, "Shared stage schema".

// Signal kinds. The stop rule (scheduling.js) classifies by kind only; it
// never reads a signal's statement. A STOP kind is either computed by the
// script from a mechanical check, or adjudicated once per wave by the gate
// or the re-plan in a form the script can check for completeness.
const STOP_KINDS = Object.freeze([
  'architecture-invalidated', // gate: section, decision, premise, evidence, amendment — all present
  'plan-defect-unabsorbed', // re-plan: verdict `cannot`, or no verdict
  'done-condition-unevaluable',
  'cycle',
  'unit-over-budget',
  'gate-non-convergence',
  'merge-conflict-unresolvable',
  'merge-regression',
  'environment-gate-drift',
  'no-ready-unit',
  'standing-obligation-undischarged',
  'agent-null',
  'precondition',
  'invalid-plan',
  'unserialised-overlap',
]);

// RECORD kinds reach the ledger and, where the re-plan can act on them, the
// re-plan stage. None stops the run.
const RECORD_KINDS = Object.freeze([
  'reading', // plan stage: a clause that admits more than one reading, and the reading taken
  'deviation', // implement stage: departure from the unit entry, including writes outside Scope
  'deferral', // implement stage: in-scope work deliberately left to a named unit
  'discovery', // implement stage: something that changes another unit or a document
  'execution-distress', // implement stage: breakdown's distress counts over threshold
  'finding', // review stages: a Significant or Minor finding, left as is
  'critical-fixed', // fix stage: a Critical finding and its applied fix
  'architecture-note', // gate: an observation short of an invalidation
  'claimed-disjoint-conflict', // merge: conflict between units the plan called disjoint, resolved mechanically
  'amendment', // re-plan: a plan amendment
  'verdict', // re-plan: amended / no-change verdict on a re-plan item
  'over-budget-repaired',
]);

const SOURCES = Object.freeze(['structural', 'reported', 'gate']);

const GATE_KINDS = Object.freeze(['environment', 'organisational', 'release', 'data']);

// phronithm:breakdown's unit types, plus `meta` for units the termination
// stage appends from the plan's Meta-Tasks section.
const UNIT_TYPES = Object.freeze(['foundation', 'feature', 'integration', 'design', 'refactor', 'bug-fix', 'polish', 'meta']);

// Re-plan amendment kinds, as breakdown's re-plan mode names them.
const AMENDMENT_KINDS = Object.freeze(['deferral', 'scope-reassignment', 'new-edge', 'done-condition', 'execution-order', 'unit-added']);

// The one field every agent() in the three workflows returns beside `result`.
const STAGE_COMMON_PROPERTIES = Object.freeze({
  commits: {
    type: 'array',
    items: {
      type: 'object',
      properties: { branch: { type: 'string' }, sha: { type: 'string' } },
      required: ['branch', 'sha'],
    },
  },
});

const STAGE_COMMON_REQUIRED = Object.freeze(['commits']);

// Build the JSON Schema for one stage's return: `result` plus `commits`.
function stageSchema(resultSchema) {
  if (!resultSchema || resultSchema.type !== 'object') {
    throw new Error('stageSchema: resultSchema must be a JSON Schema object type');
  }
  return {
    type: 'object',
    properties: { result: resultSchema, ...STAGE_COMMON_PROPERTIES },
    required: ['result', ...STAGE_COMMON_REQUIRED],
  };
}

// An architecture invalidation as the wave gate returns it. The script stops
// only when every field is non-empty; anything less is a note.
const INVALIDATION_FIELDS = Object.freeze(['section', 'decision', 'premise', 'evidence', 'amendment']);
const INVALIDATION_SCHEMA = Object.freeze({
  type: 'object',
  properties: {
    section: { type: 'string', description: 'Heading of the architecture section whose decision no longer stands' },
    decision: { type: 'string', description: 'What that section decides, quoted or closely paraphrased' },
    premise: { type: 'string', description: 'What the decision rests on' },
    evidence: { type: 'string', description: 'File and line, test, or completion-report line showing the premise false' },
    amendment: { type: 'string', description: 'What the section should now say' },
  },
  required: INVALIDATION_FIELDS,
});

// The decomposition plan reduced to what the script reads — the dependency
// graph, scopes, gates, and execution order — as graph.js produces it from
// the plan's ```yaml lsf-plan block (docs/plan-graph.md). Prose never
// reaches the script. Gate `cleared` comes from the decision log.
const PLAN_SCHEMA = Object.freeze({
  type: 'object',
  properties: {
    sha: { type: 'string' },
    path: { type: 'string' },
    units: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          id: { type: 'string' },
          type: { type: 'string', enum: UNIT_TYPES },
          complete: { type: 'boolean' },
          dependencies: {
            type: 'array',
            items: {
              type: 'object',
              properties: { unit: { type: 'string' } },
              required: ['unit'],
            },
          },
          scope: {
            type: 'array',
            items: { type: 'string' },
            description: 'Files, modules, or directories the unit writes to; `file::symbol` when the unit owns one function or class within a file shared with other units',
          },
          publishes: { type: 'array', items: { type: 'string' } },
          gates: {
            type: 'array',
            items: {
              type: 'object',
              properties: {
                name: { type: 'string' },
                kind: { type: 'string', enum: GATE_KINDS },
                cleared: { type: 'boolean' },
                pin: { type: 'string' },
              },
              required: ['name', 'kind', 'cleared'],
            },
          },
        },
        required: ['id', 'type', 'complete', 'dependencies', 'scope', 'gates'],
      },
    },
    executionOrder: { type: 'array', items: { type: 'string' } },
  },
  required: ['sha', 'path', 'units', 'executionOrder'],
});

// Merge stage result, per unit.
const MERGE_RESULT_SCHEMA = Object.freeze({
  type: 'object',
  properties: {
    integrationBranch: { type: 'string' },
    merges: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          unit: { type: 'string' },
          status: {
            type: 'string',
            enum: ['clean', 'resolved', 'unresolvable', 'regressed'],
            description: 'regressed: merged cleanly but the test suite failed; the merge was undone',
          },
          files: { type: 'array', items: { type: 'string' } },
          sha: { type: 'string' },
        },
        required: ['unit', 'status', 'files'],
      },
    },
  },
  required: ['integrationBranch', 'merges'],
});

// Completion-report fields the distress rule reads (breakdown's
// execution-distress signals). Other report fields are prose for the
// re-plan agent.
const DISTRESS_SCHEMA = Object.freeze({
  type: 'object',
  properties: {
    contextNearExhaustion: { type: 'boolean' },
    midImplementationDesignDecisions: { type: 'integer' },
    deviations: { type: 'integer' },
    deferrals: { type: 'integer' },
  },
  required: ['contextNearExhaustion', 'midImplementationDesignDecisions', 'deviations', 'deferrals'],
});

// One signal, as it appears in the ledger and the escalation record.
function signal({ kind, source, stage, statement, referent, raisedBy, unit, wave, disposition }) {
  if (!kind) throw new Error('signal: kind is required');
  if (!SOURCES.includes(source)) throw new Error(`signal: unknown source ${source}`);
  const s = { kind, source, stage, statement, referent: referent ?? '', raisedBy: raisedBy ?? stage };
  if (unit !== undefined) s.unit = unit;
  if (wave !== undefined) s.wave = wave;
  if (disposition !== undefined) s.disposition = disposition;
  return s;
}

// `planSha` is the plan to relaunch from. After a stop at re-plan it is the
// re-plan's commit, which carries the absorbed amendments; `preReplanSha`
// is then the plan the wave ran against.
function escalation({ workflow, planSha, preReplanSha, wave, stage, signals, decisionNeeded }) {
  const record = { workflow, stage, signals, decision_needed: decisionNeeded ?? [] };
  if (planSha !== undefined) record.plan_sha = planSha;
  if (preReplanSha !== undefined) record.pre_replan_sha = preReplanSha;
  if (wave !== undefined) record.wave = wave;
  return record;
}

export {
  STOP_KINDS, RECORD_KINDS, SOURCES, GATE_KINDS, UNIT_TYPES, AMENDMENT_KINDS,
  STAGE_COMMON_PROPERTIES, STAGE_COMMON_REQUIRED, stageSchema,
  INVALIDATION_FIELDS, INVALIDATION_SCHEMA, PLAN_SCHEMA, MERGE_RESULT_SCHEMA, DISTRESS_SCHEMA,
  signal, escalation,
};
