// Shared schema for the lsf-* workflows. Dependency-free; inlined into the
// workflow scripts (the script cannot import). This file is the source of
// truth. See docs/design/lsf-workflow.md, "Shared stage schema".

// Signal kinds. The stop rule (scheduling.js) classifies by kind and source
// only; it never reads a signal's statement.
const STOP_KINDS = Object.freeze([
  'assumption-invalidated',
  'requirement-contradictory',
  'requirement-incomplete',
  'scope-ambiguity',
  'goal-ambiguity',
  'done-condition-unevaluable',
  'cycle',
  'unit-over-budget',
  'gate-non-convergence',
  'merge-conflict-unresolvable',
  'environment-gate-drift',
  'no-ready-unit',
  'standing-obligation-undischarged',
  'agent-null',
  'merge-regression',
  'handoff-insufficient',
  'precondition',
  'invalid-plan',
  'unserialised-overlap',
  'execution-distress',
  'referent-recurrence',
  'signal-volume',
  'silent-wave',
]);

const RECORD_KINDS = Object.freeze([
  'assumption',
  'design-ambiguity',
  'deferral',
  'scope-reassignment',
  'new-edge',
  'claimed-disjoint-conflict',
  'over-budget-repaired',
  'critical-fixed',
  'handoff-partial',
]);

const SOURCES = Object.freeze(['structural', 'reported', 'gate']);

const AMBIGUITY_ABOUT = Object.freeze(['scope', 'goal', 'requirement', 'design']);

const DEPENDENCY_TYPES = Object.freeze([
  'logical', 'testing', 'design', 'knowledge', 'organisational', 'release', 'data',
]);

const GATE_KINDS = Object.freeze(['environment', 'organisational', 'release', 'data']);

// phronithm:breakdown's unit types, plus `meta` for units the termination
// stage appends from the plan's Meta-Tasks section.
const UNIT_TYPES = Object.freeze(['foundation', 'feature', 'integration', 'design', 'refactor', 'bug-fix', 'polish', 'meta']);

// Fields every agent() in the three workflows returns in addition to `result`.
const STAGE_COMMON_PROPERTIES = Object.freeze({
  assumptions: {
    type: 'array',
    items: {
      type: 'object',
      properties: { statement: { type: 'string' }, referent: { type: 'string' } },
      required: ['statement', 'referent'],
    },
  },
  ambiguities: {
    type: 'array',
    items: {
      type: 'object',
      properties: {
        statement: { type: 'string' },
        about: { type: 'string', enum: AMBIGUITY_ABOUT },
      },
      required: ['statement', 'about'],
    },
  },
  contradictions: {
    type: 'array',
    items: {
      type: 'object',
      properties: {
        statement: { type: 'string' },
        against: { type: 'string', description: 'Document path and commit SHA' },
      },
      required: ['statement', 'against'],
    },
  },
  commits: {
    type: 'array',
    items: {
      type: 'object',
      properties: { branch: { type: 'string' }, sha: { type: 'string' } },
      required: ['branch', 'sha'],
    },
  },
});

const STAGE_COMMON_REQUIRED = Object.freeze(['assumptions', 'ambiguities', 'contradictions', 'commits']);

// Build the JSON Schema for one stage's return: the shared fields plus a
// stage-specific `result`.
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

// What the load stage returns: the decomposition plan reduced to what the
// script reads — the dependency graph, scopes, gates, and execution order.
// Prose (coordination protocols, done conditions) never reaches the script.
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
              properties: {
                unit: { type: 'string' },
                type: { type: 'string', enum: DEPENDENCY_TYPES },
              },
              required: ['unit', 'type'],
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
          entry: { type: 'string', description: 'The unit entry verbatim, for downstream prompts' },
        },
        required: ['id', 'type', 'complete', 'dependencies', 'scope', 'gates', 'entry'],
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

// Completion-report fields the stop rule reads (breakdown's execution-distress
// signals). Other report fields are prose for the re-plan agent.
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

function escalation({ workflow, planSha, wave, stage, signals, decisionNeeded }) {
  const record = { workflow, stage, signals, decision_needed: decisionNeeded ?? [] };
  if (planSha !== undefined) record.plan_sha = planSha;
  if (wave !== undefined) record.wave = wave;
  return record;
}

export {
  STOP_KINDS, RECORD_KINDS, SOURCES, AMBIGUITY_ABOUT, DEPENDENCY_TYPES, GATE_KINDS, UNIT_TYPES,
  STAGE_COMMON_PROPERTIES, STAGE_COMMON_REQUIRED, stageSchema,
  PLAN_SCHEMA, MERGE_RESULT_SCHEMA, DISTRESS_SCHEMA,
  signal, escalation,
};
