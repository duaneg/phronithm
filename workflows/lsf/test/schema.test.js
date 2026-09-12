import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  stageSchema, signal, escalation, STOP_KINDS, RECORD_KINDS, STAGE_COMMON_REQUIRED,
} from '../schema.js';

test('stageSchema wraps a result schema with the shared fields', () => {
  const s = stageSchema({ type: 'object', properties: { ok: { type: 'boolean' } }, required: ['ok'] });
  assert.equal(s.type, 'object');
  assert.deepEqual(s.required, ['result', ...STAGE_COMMON_REQUIRED]);
  for (const r of s.required) assert.ok(r in s.properties, `${r} declared`);
  assert.deepEqual(s.properties.ambiguities.items.properties.about.enum, ['scope', 'goal', 'requirement', 'design']);
  assert.deepEqual(s.properties.ambiguities.items.properties.level.enum, ['plan', 'architecture', 'request']);
  assert.deepEqual(s.properties.contradictions.items.properties.level.enum, ['plan', 'architecture', 'request']);
  assert.ok(s.properties.ambiguities.items.required.includes('level'));
  assert.ok(s.properties.contradictions.items.required.includes('level'));
});

test('stageSchema rejects a non-object result schema', () => {
  assert.throws(() => stageSchema({ type: 'string' }));
  assert.throws(() => stageSchema());
});

test('signal requires a kind and a known source', () => {
  assert.throws(() => signal({ source: 'reported', stage: 'plan' }));
  assert.throws(() => signal({ kind: 'assumption', source: 'volunteered', stage: 'plan' }));
  const s = signal({ kind: 'assumption', source: 'reported', stage: 'plan', statement: 'x' });
  assert.equal(s.referent, '');
  assert.equal(s.raisedBy, 'plan');
  assert.ok(!('unit' in s));
});

test('stop and record kinds are disjoint', () => {
  for (const k of RECORD_KINDS) assert.ok(!STOP_KINDS.includes(k), k);
});

test('escalation omits plan_sha and wave when unset', () => {
  const e = escalation({ workflow: 'lsf-brief', stage: 'critique', signals: [] });
  assert.deepEqual(Object.keys(e).sort(), ['decision_needed', 'signals', 'stage', 'workflow']);
  const f = escalation({ workflow: 'lsf-implement', planSha: 'abc', wave: 2, stage: 'merge', signals: [] });
  assert.equal(f.plan_sha, 'abc');
  assert.equal(f.wave, 2);
});
