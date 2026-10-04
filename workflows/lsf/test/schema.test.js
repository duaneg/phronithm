import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  AMENDMENT_KINDS,
  stageSchema, signal, escalation, STOP_KINDS, RECORD_KINDS, STAGE_COMMON_REQUIRED, INVALIDATION_SCHEMA, INVALIDATION_FIELDS,
} from '../schema.js';

test('stageSchema wraps a result schema with the shared fields', () => {
  const s = stageSchema({ type: 'object', properties: { ok: { type: 'boolean' } }, required: ['ok'] });
  assert.equal(s.type, 'object');
  assert.deepEqual(s.required, ['result', ...STAGE_COMMON_REQUIRED]);
  for (const r of s.required) assert.ok(r in s.properties, `${r} declared`);
  assert.deepEqual(Object.keys(s.properties), ['result', 'commits'], 'commits is the only shared field');
});

test('an invalidation requires all five fields', () => {
  assert.deepEqual(INVALIDATION_SCHEMA.required, INVALIDATION_FIELDS);
  assert.deepEqual([...INVALIDATION_FIELDS], ['section', 'decision', 'premise', 'evidence', 'amendment']);
});

test('stageSchema rejects a non-object result schema', () => {
  assert.throws(() => stageSchema({ type: 'string' }));
  assert.throws(() => stageSchema());
});

test('signal requires a kind and a known source', () => {
  assert.throws(() => signal({ source: 'reported', stage: 'plan' }));
  assert.throws(() => signal({ kind: 'reading', source: 'volunteered', stage: 'plan' }));
  const s = signal({ kind: 'reading', source: 'reported', stage: 'plan', statement: 'x' });
  assert.equal(s.referent, '');
  assert.equal(s.raisedBy, 'plan');
  assert.ok(!('unit' in s));
});

test('stop and record kinds are disjoint', () => {
  for (const k of RECORD_KINDS) assert.ok(!STOP_KINDS.includes(k), k);
});

test('escalation omits plan_sha and wave when unset', () => {
  const e = escalation({ workflow: 'lsf-implement', stage: 'critique', signals: [] });
  assert.deepEqual(Object.keys(e).sort(), ['decision_needed', 'signals', 'stage', 'workflow']);
  const f = escalation({ workflow: 'lsf-implement', planSha: 'abc', wave: 2, stage: 'merge', signals: [] });
  assert.equal(f.plan_sha, 'abc');
  assert.equal(f.wave, 2);
  assert.ok(!('pre_replan_sha' in f));
  assert.equal(escalation({ workflow: 'w', stage: 'replan', signals: [], planSha: 'b', preReplanSha: 'a' }).pre_replan_sha, 'a');
});

test('gate-repin is an amendment kind', () => {
  assert.ok(AMENDMENT_KINDS.includes('gate-repin'));
});

test('unit removal and gate addition and removal are amendment kinds', () => {
  for (const kind of ['unit-removed', 'gate-added', 'gate-removed']) assert.ok(AMENDMENT_KINDS.includes(kind), kind);
});
