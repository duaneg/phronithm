import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  findCycle, pathsOverlap, overlappingPairs, validatePlan,
  readySet, blockedUnits, mergeOrder, wavePlan,
  signalsFromStageReturn, nullReturnSignal, signalsFromMerge, distressSignals,
  volumeSignals, evaluateWave, digest,
} from '../scheduling.js';
import { signal } from '../schema.js';

const unit = (id, opts = {}) => ({
  id, type: 'feature', complete: false, dependencies: [], scope: [], gates: [], entry: id, ...opts,
});
const dep = (u, type = 'logical') => ({ unit: u, type });
const plan = (units, executionOrder = units.map((u) => u.id)) => ({ sha: 'deadbeef', path: 'docs/plans/x.md', units, executionOrder });

// A: foundation; B, C depend on A; D depends on B and C.
const diamond = () => plan([
  unit('A', { type: 'foundation', scope: ['src/core'] }),
  unit('B', { dependencies: [dep('A')], scope: ['src/b'] }),
  unit('C', { dependencies: [dep('A')], scope: ['src/c'] }),
  unit('D', { dependencies: [dep('B'), dep('C', 'testing')], scope: ['src/d'] }),
]);

test('findCycle returns null for a DAG and the cycle otherwise', () => {
  assert.equal(findCycle(diamond()), null);
  const p = plan([unit('A', { dependencies: [dep('B')] }), unit('B', { dependencies: [dep('A', 'testing')] })]);
  assert.deepEqual(findCycle(p), ['A', 'B', 'A']);
});

test('pathsOverlap is containment or equality, not prefix', () => {
  assert.ok(pathsOverlap('src/core', 'src/core/x.js'));
  assert.ok(pathsOverlap('src/core/', 'src/core'));
  assert.ok(!pathsOverlap('src/core', 'src/corelib'));
  assert.ok(!pathsOverlap('src/a', 'src/b'));
});

test('overlappingPairs reports serialisation via any dependency path', () => {
  const p = plan([
    unit('A', { scope: ['src/x'] }),
    unit('B', { dependencies: [dep('A')], scope: ['src/y'] }),
    unit('C', { dependencies: [dep('B')], scope: ['src/x/file.js'] }),
    unit('D', { scope: ['src/x'] }),
  ]);
  const pairs = overlappingPairs(p);
  const key = (pr) => pr.units.join('+');
  const byKey = Object.fromEntries(pairs.map((pr) => [key(pr), pr.serialised]));
  assert.equal(byKey['A+C'], true, 'A -> B -> C serialises A and C');
  assert.equal(byKey['A+D'], false);
  assert.equal(byKey['C+D'], false);
});

test('validatePlan accepts the diamond', () => {
  assert.deepEqual(validatePlan(diamond()), []);
});

test('validatePlan reports unknown dependencies, order gaps, cycles, and unserialised overlap', () => {
  const p = plan([
    unit('A', { dependencies: [dep('Z')], scope: ['src/x'] }),
    unit('B', { dependencies: [dep('A')], scope: ['src/y'] }),
    unit('C', { dependencies: [dep('B')] }),
    unit('D', { scope: ['src/x/a.js'] }),
  ], ['A', 'B', 'C']);
  p.units[0].dependencies.push(dep('C', 'testing'));
  const kinds = validatePlan(p).map((x) => x.kind).sort();
  assert.deepEqual(kinds, ['cycle', 'invalid-plan', 'invalid-plan', 'unserialised-overlap']);
});

test('validatePlan flags duplicate ids', () => {
  const p = plan([unit('A'), unit('A')], ['A']);
  assert.ok(validatePlan(p).some((x) => x.statement.includes('duplicate')));
});

test('readySet follows dependencies and honours plan order', () => {
  const p = diamond();
  assert.deepEqual(readySet(p).map((u) => u.id), ['A']);
  assert.deepEqual(readySet(p, new Set(['A'])).map((u) => u.id), ['B', 'C']);
  assert.deepEqual(readySet(p, new Set(['A', 'B'])).map((u) => u.id), ['C']);
  assert.deepEqual(readySet(p, new Set(['A', 'B', 'C'])).map((u) => u.id), ['D']);
  assert.deepEqual(readySet(p, new Set(['A', 'B', 'C', 'D'])), []);
});

test('readySet respects the plan complete flag', () => {
  const p = diamond();
  p.units[0].complete = true;
  assert.deepEqual(readySet(p).map((u) => u.id), ['B', 'C']);
});

test('readySet excludes units with uncleared organisational or release gates, not environment ones', () => {
  const p = plan([
    unit('A', { gates: [{ name: 'security sign-off', kind: 'organisational', cleared: false }] }),
    unit('B', { gates: [{ name: 'node', kind: 'environment', cleared: false, pin: '26.8.1' }] }),
    unit('C', { gates: [{ name: 'v2 shipped', kind: 'release', cleared: true }] }),
  ]);
  assert.deepEqual(readySet(p).map((u) => u.id), ['B', 'C']);
});

test('blockedUnits distinguishes waiting from gated', () => {
  const p = plan([
    unit('A'),
    unit('B', { dependencies: [dep('A')] }),
    unit('C', { gates: [{ name: 'legal', kind: 'organisational', cleared: false }] }),
  ]);
  assert.deepEqual(blockedUnits(p), [
    { unit: 'B', reason: 'waiting', waitingOn: ['A'], gates: [] },
    { unit: 'C', reason: 'gated', waitingOn: [], gates: ['legal'] },
  ]);
});

test('mergeOrder is plan execution order regardless of input order', () => {
  const p = diamond();
  p.executionOrder = ['A', 'C', 'B', 'D'];
  assert.deepEqual(mergeOrder(p, ['B', 'C']), ['C', 'B']);
});

test('wavePlan partitions the diamond into three waves and stops at a gate', () => {
  assert.deepEqual(wavePlan(diamond()), [['A'], ['B', 'C'], ['D']]);
  const p = diamond();
  p.units[3].gates.push({ name: 'release 1.0', kind: 'release', cleared: false });
  assert.deepEqual(wavePlan(p), [['A'], ['B', 'C']]);
});

test('signalsFromStageReturn maps the shared fields by kind', () => {
  const ret = {
    result: {},
    assumptions: [{ statement: 'schema is stable', referent: 'docs/arch.md#schema' }],
    ambiguities: [
      { statement: 'which tenants', about: 'scope' },
      { statement: 'is latency a goal', about: 'goal' },
      { statement: 'missing SLA', about: 'requirement' },
      { statement: 'retry policy', about: 'design' },
    ],
    contradictions: [{ statement: 'arch says sync', against: 'docs/arch.md@abc' }],
    commits: [],
  };
  const sigs = signalsFromStageReturn(ret, { stage: 'implement', unit: 'B', wave: 2 });
  assert.deepEqual(sigs.map((s) => s.kind), [
    'assumption', 'scope-ambiguity', 'goal-ambiguity', 'requirement-incomplete', 'design-ambiguity', 'assumption-invalidated',
  ]);
  assert.ok(sigs.every((s) => s.source === 'reported' && s.unit === 'B' && s.wave === 2));
  assert.equal(sigs[5].referent, 'docs/arch.md@abc');
});

test('signalsFromStageReturn tolerates missing arrays', () => {
  assert.deepEqual(signalsFromStageReturn({ result: {} }, { stage: 'plan' }), []);
});

test('nullReturnSignal is a structural STOP', () => {
  const s = nullReturnSignal({ stage: 'review', unit: 'C', wave: 1 });
  assert.equal(s.kind, 'agent-null');
  assert.equal(evaluateWave([s], [], { wave: 1, unitsInWave: 1 }).stop, true);
});

test('signalsFromMerge distinguishes resolved from unresolvable conflicts', () => {
  const sigs = signalsFromMerge({
    integrationBranch: 'lsf/x',
    merges: [
      { unit: 'B', status: 'clean', files: [] },
      { unit: 'C', status: 'resolved', files: ['src/shared.js'] },
      { unit: 'D', status: 'unresolvable', files: ['src/shared.js'] },
    ],
  }, { wave: 2 });
  assert.deepEqual(sigs.map((s) => s.kind), ['claimed-disjoint-conflict', 'merge-conflict-unresolvable']);
  assert.ok(sigs.every((s) => s.source === 'structural'));
});

test('distressSignals fires on any breakdown distress signal and is quiet otherwise', () => {
  const calm = { contextNearExhaustion: false, midImplementationDesignDecisions: 0, deviations: 1, deferrals: 1 };
  assert.deepEqual(distressSignals(calm, { unit: 'B', wave: 1 }), []);
  const cases = [
    { ...calm, contextNearExhaustion: true },
    { ...calm, midImplementationDesignDecisions: 1 },
    { ...calm, deviations: 2, deferrals: 2 },
  ];
  for (const c of cases) {
    const sigs = distressSignals(c, { unit: 'B', wave: 1 });
    assert.equal(sigs.length, 1);
    assert.equal(sigs[0].kind, 'execution-distress');
  }
  assert.deepEqual(distressSignals({ ...calm, deviations: 3 }, { unit: 'B', wave: 1 }, { distressDeviationThreshold: 10 }), []);
});

const reported = (kind, opts) => signal({ source: 'reported', stage: 'implement', statement: kind, ...opts, kind });

test('evaluateWave records assumptions and design ambiguities, stops on the rest', () => {
  const sigs = [
    reported('assumption', { unit: 'B', wave: 1, referent: 'r1' }),
    reported('design-ambiguity', { unit: 'B', wave: 1, referent: 'r2' }),
  ];
  const r = evaluateWave(sigs, [], { wave: 1, unitsInWave: 1 });
  assert.equal(r.stop, false);
  assert.deepEqual(r.records.map((s) => s.disposition), ['recorded', 'recorded']);
  const r2 = evaluateWave([reported('scope-ambiguity', { unit: 'B', wave: 1, referent: 'r3' })], [], { wave: 1, unitsInWave: 1 });
  assert.equal(r2.stop, true);
  assert.equal(r2.stops[0].disposition, 'stopped');
});

test('a wave with units and no reported signal stops as silent', () => {
  const r = evaluateWave([], [], { wave: 1, unitsInWave: 2 });
  assert.deepEqual(r.stops.map((s) => s.kind), ['silent-wave']);
  const empty = evaluateWave([], [], { wave: 1, unitsInWave: 0 });
  assert.equal(empty.stop, false);
});

test('the same referent from two waves stops', () => {
  const prior = [reported('assumption', { unit: 'A', wave: 1, referent: 'Auth Token Format' })];
  const now = [reported('assumption', { unit: 'B', wave: 2, referent: 'auth token format ' })];
  const v = volumeSignals(now, prior, { wave: 2, unitsInWave: 1 });
  assert.deepEqual(v.map((s) => s.kind), ['referent-recurrence']);
  assert.equal(v[0].referent, 'auth token format');
});

test('the same referent from two sources in one wave stops; one source in one wave does not', () => {
  const now = [
    reported('assumption', { unit: 'B', wave: 1, referent: 'x' }),
    signal({ kind: 'claimed-disjoint-conflict', source: 'structural', stage: 'merge', unit: 'B', wave: 1, statement: 's', referent: 'x' }),
  ];
  assert.deepEqual(volumeSignals(now, [], { wave: 1, unitsInWave: 1 }).map((s) => s.kind), ['referent-recurrence']);
  const single = [
    reported('assumption', { unit: 'B', wave: 1, referent: 'x' }),
    reported('assumption', { unit: 'C', wave: 1, referent: 'x' }),
  ];
  assert.deepEqual(volumeSignals(single, [], { wave: 1, unitsInWave: 2 }), []);
});

test('recurrence only fires for referents this wave touched', () => {
  const prior = [
    reported('assumption', { unit: 'A', wave: 1, referent: 'x' }),
    reported('assumption', { unit: 'A', wave: 2, referent: 'x' }),
  ];
  const now = [reported('assumption', { unit: 'B', wave: 3, referent: 'y' })];
  assert.deepEqual(volumeSignals(now, prior, { wave: 3, unitsInWave: 1 }), []);
});

test('per-unit signal threshold is configurable and counts assumptions and ambiguities only', () => {
  const now = Array.from({ length: 3 }, (_, i) => reported('assumption', { unit: 'B', wave: 1, referent: `r${i}` }));
  now.push(reported('design-ambiguity', { unit: 'B', wave: 1, referent: 'r9' }));
  assert.deepEqual(volumeSignals(now, [], { wave: 1, unitsInWave: 1 }), []);
  const v = volumeSignals(now, [], { wave: 1, unitsInWave: 1 }, { perUnitSignalThreshold: 3 });
  assert.deepEqual(v.map((s) => s.kind), ['signal-volume']);
  assert.equal(v[0].unit, 'B');
});

test('digest computes counts and the three trend flags', () => {
  const ledger = [
    reported('assumption', { unit: 'A', wave: 1, referent: 'a' }),
    reported('assumption', { unit: 'B', wave: 2, referent: 'b' }),
    reported('assumption', { unit: 'B', wave: 2, referent: 'c' }),
    reported('assumption', { unit: 'C', wave: 3, referent: 'd' }),
    reported('assumption', { unit: 'C', wave: 3, referent: 'e' }),
    reported('assumption', { unit: 'C', wave: 3, referent: 'f' }),
    signal({ kind: 'referent-recurrence', source: 'structural', stage: 'stop-rule', wave: 3, statement: 's', referent: 'b' }),
    signal({ kind: 'silent-wave', source: 'structural', stage: 'stop-rule', wave: 1, statement: 's', referent: 'wave-1' }),
  ];
  const d = digest(ledger);
  assert.equal(d.waves, 3);
  assert.equal(d.byKind.assumption, 6);
  assert.deepEqual(d.bySource, { reported: 6, structural: 2 });
  assert.equal(d.trends.risingAssumptions, true);
  assert.deepEqual(d.trends.recurringReferents, ['b']);
  assert.deepEqual(d.trends.silentWaves, [1]);
  assert.match(d.note, /not evidence/);
});

test('digest uses unitsPerWave when given', () => {
  const ledger = [
    reported('assumption', { unit: 'A', wave: 1, referent: 'a' }),
    reported('assumption', { unit: 'B', wave: 2, referent: 'b' }),
  ];
  const d = digest(ledger, { unitsPerWave: { 1: 1, 2: 4 } });
  assert.deepEqual(d.trends.assumptionsPerUnitByWave, [1, 0.25]);
  assert.equal(d.trends.risingAssumptions, false);
});
