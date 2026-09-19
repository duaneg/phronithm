import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  findCycle, pathsOverlap, overlappingPairs, validatePlan,
  readySet, blockedUnits, mergeOrder, wavePlan,
  nullReturnSignal, signalsFromReadings, signalsFromImplement, signalsFromFindings, signalsFromGate, invalidationComplete,
  signalsFromMerge, distressSignals, evaluateWave, digest, renderLedger, ledgerWave, ledgerLabel, ledgerHeading, REPLAN_ITEM_KINDS,
} from '../scheduling.js';
import { STOP_KINDS, RECORD_KINDS } from '../schema.js';
import { signal } from '../schema.js';

const unit = (id, opts = {}) => ({
  id, type: 'feature', complete: false, dependencies: [], scope: [], gates: [], entry: id, ...opts,
});
const dep = (u) => ({ unit: u });
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

test('pathsOverlap treats file::symbol as a narrowing of the file', () => {
  assert.ok(!pathsOverlap('src/svg.py::emit_solid', 'src/svg.py::emit_stairs'));
  assert.ok(pathsOverlap('src/svg.py::emit_solid', 'src/svg.py::emit_solid'));
  assert.ok(pathsOverlap('src/svg.py', 'src/svg.py::emit_solid'));
  assert.ok(pathsOverlap('src/', 'src/svg.py::emit_solid'));
  assert.ok(!pathsOverlap('src/other.py::f', 'src/svg.py::f'));
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

test('overlappingPairs and validatePlan ignore completed units', () => {
  const p = plan([
    unit('A', { complete: true, scope: ['src/x'] }),
    unit('B', { scope: ['src/x'] }),
    unit('C', { scope: ['src/x'] }),
  ]);
  assert.deepEqual(overlappingPairs(p).map((pr) => pr.units), [['B', 'C']]);
  assert.deepEqual(overlappingPairs(p, new Set(['C'])), []);
  assert.equal(validatePlan(p).length, 1);
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

test('signalsFromReadings records each reading against its clause', () => {
  const sigs = signalsFromReadings([{ clause: 'Scope: tests/test_core_geom_svgpath.py', reading: 'tests/test_svgpath_document.py, the file that owns _classify' }], { unit: 'B', wave: 1 });
  assert.equal(sigs.length, 1);
  assert.equal(sigs[0].kind, 'reading');
  assert.equal(sigs[0].source, 'reported');
  assert.equal(sigs[0].stage, 'plan');
  assert.equal(sigs[0].referent, 'Scope: tests/test_core_geom_svgpath.py');
  assert.match(sigs[0].statement, /read as: tests\/test_svgpath_document\.py/);
  assert.deepEqual(signalsFromReadings(undefined, { unit: 'B', wave: 1 }), []);
});

test('signalsFromImplement records deviations, writes outside Scope, deferrals, and discoveries', () => {
  const sigs = signalsFromImplement({
    deviations: ['added a sibling predicate instead of widening'],
    writesOutsideScope: ['CLAUDE.md', 'tests/stubs.py'],
    deferrals: [{ statement: 'export path left unrotated', belongsTo: 'render-export' }],
    discoveries: [{ statement: 'loophole-compound has no edge to render-export', affects: 'loophole-compound' }],
  }, { unit: 'B', wave: 2 });
  assert.deepEqual(sigs.map((s) => s.kind), ['deviation', 'deviation', 'deferral', 'discovery']);
  assert.ok(sigs.every((s) => s.source === 'reported' && s.stage === 'implement' && s.unit === 'B' && s.wave === 2));
  assert.match(sigs[1].statement, /writes outside the Scope line: CLAUDE\.md, tests\/stubs\.py/);
  assert.equal(sigs[2].referent, 'render-export');
  assert.equal(sigs[3].referent, 'loophole-compound');
  assert.deepEqual(signalsFromImplement({}, { unit: 'B', wave: 2 }), []);
});

test('signalsFromFindings records Significant and Minor findings and skips Critical', () => {
  const sigs = signalsFromFindings([
    { severity: 'Critical', statement: 'wrong', location: 'a.py:1' },
    { severity: 'Significant', statement: 'fragile', location: 'a.py:2' },
    { severity: 'Minor', statement: 'naming', location: 'a.py:3' },
  ], { stage: 'critique-code', unit: 'B', wave: 1, round: 2 });
  assert.deepEqual(sigs.map((s) => [s.kind, s.source, s.referent]), [['finding', 'gate', 'a.py:2'], ['finding', 'gate', 'a.py:3']]);
  assert.match(sigs[0].statement, /^Significant \(round 2\): fragile/);
});

const inv = (over = {}) => ({ section: '§4.8', decision: 'two-tone families are two-part groups', premise: 'nested art classifies to a group', evidence: 'svgpath.py:120; brazier parses to a bare Glyph', amendment: 'nested art must be authored as siblings, or _classify keeps ids on holes', ...over });

test('signalsFromGate: a complete invalidation stops; an incomplete one and every note is recorded', () => {
  assert.ok(invalidationComplete(inv()));
  assert.ok(!invalidationComplete(inv({ amendment: '' })));
  assert.ok(!invalidationComplete(inv({ evidence: '  ' })));
  const sigs = signalsFromGate({
    invalidations: [inv(), inv({ amendment: '' })],
    notes: [{ statement: 'N3 trigger fired, no owner', referent: 'architecture §6 N3' }],
  }, { wave: 1 });
  assert.deepEqual(sigs.map((s) => s.kind), ['architecture-invalidated', 'architecture-note', 'architecture-note']);
  assert.ok(sigs.every((s) => s.source === 'gate' && s.stage === 'review-gate' && s.wave === 1));
  assert.equal(sigs[0].referent, '§4.8');
  assert.match(sigs[0].statement, /section: §4\.8; decision: .*; premise: .*; evidence: .*; amendment: nested art/);
  assert.match(sigs[1].statement, /^incomplete invalidation \(amendment missing\)/);
  assert.equal(sigs[2].referent, 'architecture §6 N3');
  const ev = evaluateWave(sigs);
  assert.equal(ev.stop, true);
  assert.deepEqual(ev.stops.map((s) => s.kind), ['architecture-invalidated']);
  assert.deepEqual(ev.records.map((s) => s.disposition), ['recorded', 'recorded']);
});

test('nullReturnSignal is a structural STOP', () => {
  const s = nullReturnSignal({ stage: 'review', unit: 'C', wave: 1 });
  assert.equal(s.kind, 'agent-null');
  assert.equal(evaluateWave([s]).stop, true);
});

test('signalsFromMerge distinguishes resolved from unresolvable conflicts', () => {
  const sigs = signalsFromMerge({
    integrationBranch: 'lsf/x',
    merges: [
      { unit: 'B', status: 'clean', files: [] },
      { unit: 'C', status: 'resolved', files: ['src/shared.js'] },
      { unit: 'D', status: 'unresolvable', files: ['src/shared.js'] },
      { unit: 'E', status: 'regressed', files: ['test/e.test.js'] },
    ],
  }, { wave: 2 });
  assert.deepEqual(sigs.map((s) => s.kind), ['claimed-disjoint-conflict', 'merge-conflict-unresolvable', 'merge-regression']);
  assert.ok(sigs.every((s) => s.source === 'structural'));
  const ev = evaluateWave(sigs);
  assert.deepEqual(ev.records.map((s) => s.kind), ['claimed-disjoint-conflict']);
});

test('distressSignals is a record on any breakdown distress signal and quiet otherwise', () => {
  const calm = { contextNearExhaustion: false, midImplementationDesignDecisions: 0, deviations: 1, deferrals: 1 };
  assert.deepEqual(distressSignals(calm, { unit: 'B', wave: 1 }), []);
  const cases = [
    { ...calm, contextNearExhaustion: true },
    { ...calm, midImplementationDesignDecisions: 3 },
    { ...calm, deviations: 2, deferrals: 2 },
  ];
  for (const c of cases) {
    const sigs = distressSignals(c, { unit: 'B', wave: 1 });
    assert.equal(sigs.length, 1);
    assert.equal(sigs[0].kind, 'execution-distress');
    assert.equal(evaluateWave(sigs).stop, false);
  }
  assert.deepEqual(distressSignals({ ...calm, deviations: 3 }, { unit: 'B', wave: 1 }, { distressDeviationThreshold: 10 }), []);
  assert.deepEqual(distressSignals({ ...calm, midImplementationDesignDecisions: 2 }, { unit: 'B', wave: 1 }), [], 'two design decisions are ordinary');
});

test('the stop rule reads kind alone: every STOP kind stops, every RECORD kind is recorded, and re-plan items are records', () => {
  for (const k of STOP_KINDS) assert.equal(evaluateWave([signal({ kind: k, source: 'structural', stage: 's', statement: k })]).stop, true, k);
  for (const k of RECORD_KINDS) assert.equal(evaluateWave([signal({ kind: k, source: 'reported', stage: 's', statement: k })]).stop, false, k);
  for (const k of REPLAN_ITEM_KINDS) assert.ok(RECORD_KINDS.includes(k), k);
  const empty = evaluateWave([]);
  assert.equal(empty.stop, false);
  assert.deepEqual(empty.all, []);
});

test('renderLedger is one table under a wave heading, one line per row, pipes and newlines neutralised', () => {
  const rows = [
    { wave: 2, unit: 'B', stage: 'plan', source: 'reported', kind: 'reading', statement: 'a | b\nc', referent: 'x', disposition: 'recorded' },
    { unit: null, stage: 'merge', source: 'structural', kind: 'merge-regression', statement: 's', referent: 'B', disposition: 'stopped' },
  ];
  const md = renderLedger(rows, 2);
  const lines = md.split('\n');
  assert.equal(lines[0], '## Wave 2');
  assert.equal(lines[2], '| Wave | Unit | Stage | Source | Kind | Statement | Referent | Disposition |');
  assert.equal(lines[4], '| 2 | B | plan | reported | reading | a \\| b c | x | recorded |');
  assert.equal(lines[5], '| 2 | — | merge | structural | merge-regression | s | B | stopped |');
  assert.equal(lines.length, 7); // trailing newline
  assert.match(ledgerHeading('f'), /^# Signal ledger: f\n/);
});

test('renderLedger heads a schedule stop with the wave its rows name, and a mixed append with the loop wave', () => {
  const stop = { wave: 3, unit: 'C', stage: 'schedule', source: 'structural', kind: 'no-ready-unit', statement: 's', referent: 'C', disposition: 'stopped' };
  assert.equal(renderLedger([stop], 2).split('\n')[0], '## Wave 3');
  assert.equal(ledgerWave([stop], 2), 3);
  const rec = { wave: 2, unit: 'B', stage: 'plan', source: 'reported', kind: 'reading', statement: 'r', referent: 'x', disposition: 'recorded' };
  assert.equal(renderLedger([rec, stop], 2).split('\n')[0], '## Wave 2');
  assert.equal(renderLedger([], 2).split('\n')[0], '## Wave 2');
});

test('ledgerLabel is termination when no row carries a wave, and the wave otherwise', () => {
  const term = { unit: null, stage: 'obligations', source: 'reported', kind: 'standing-obligation-carried', statement: 's', referent: 'debt.md', disposition: 'recorded' };
  const rec = { wave: 2, unit: 'B', stage: 'plan', source: 'reported', kind: 'reading', statement: 'r', referent: 'x', disposition: 'recorded' };
  assert.equal(ledgerLabel([term], 2), 'termination');
  assert.equal(renderLedger([term], 2).split('\n')[0], '## Termination');
  assert.equal(ledgerLabel([rec, term], 2), 'wave 2');
  assert.equal(ledgerLabel([rec], 2), 'wave 2');
  assert.equal(ledgerLabel([], 2), 'wave 2');
});

test('digest counts by kind, source and disposition, and lists what needs attention', () => {
  const rec = (kind, opts) => signal({ source: 'reported', stage: 'implement', statement: kind, disposition: 'recorded', ...opts, kind });
  const ledger = [
    rec('reading', { unit: 'A', wave: 1, stage: 'plan' }),
    signal({ kind: 'finding', source: 'gate', stage: 'critique-code', unit: 'A', wave: 1, statement: 'Significant: x', disposition: 'recorded' }),
    signal({ kind: 'finding', source: 'gate', stage: 'lens-review', unit: 'A', wave: 1, statement: 'Minor: y', disposition: 'recorded' }),
    signal({ kind: 'finding', source: 'gate', stage: 'critique-code', unit: 'B', wave: 2, statement: 'Significant: z', disposition: 'recorded' }),
    signal({ kind: 'architecture-note', source: 'gate', stage: 'review-gate', wave: 2, statement: 'N3 has no owner', referent: '§6', disposition: 'recorded' }),
    rec('execution-distress', { unit: 'B', wave: 2 }),
    rec('execution-distress', { unit: 'B', wave: 2 }),
    signal({ kind: 'verdict', source: 'reported', stage: 'decision-replan', wave: 0, statement: 'd1 amended', disposition: 'recorded' }),
  ];
  const d = digest(ledger);
  assert.equal(d.waves, 2, 'wave 0 (decision re-plan) is not counted');
  assert.equal(d.byKind.finding, 3);
  assert.deepEqual(d.bySource, { reported: 4, gate: 4 });
  assert.deepEqual(d.byDisposition, { recorded: 8 });
  assert.deepEqual(d.attention.architectureNotes, ['N3 has no owner']);
  assert.deepEqual(d.attention.architectureAmendments, []);
  assert.deepEqual(d.attention.unfixedSignificant, { A: 1, B: 1 });
  assert.deepEqual(d.attention.distressedUnits, ['B']);
  assert.match(d.note, /not evidence/);
});
