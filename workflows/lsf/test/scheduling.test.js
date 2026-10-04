import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  findCycle, pathsOverlap, overlappingPairs, validatePlan,
  readySet, blockedUnits, mergeOrder, wavePlan,
  nullReturnSignal, signalsFromReadings, signalsFromDocumentSize, OVERSIZE_FACTOR, signalsFromImplement, signalsFromFindings, signalsFromGate, invalidationComplete,
  signalsFromMerge, distressSignals, evaluateWave, digest, renderLedger, ledgerWave, ledgerLabel, ledgerHeading, REPLAN_ITEM_KINDS, PLAN_SUPERSEDING_STOP_KINDS,
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

test('signalsFromReadings is one count row per unit pointing at the plan\'s Readings section', () => {
  const readings = [
    { clause: 'Scope: tests/test_core_geom_svgpath.py', reading: 'tests/test_svgpath_document.py, the file that owns _classify' },
    { clause: 'Done: the export path rotates', reading: 'rotates about the origin' },
  ];
  const sigs = signalsFromReadings(readings, { unit: 'B', wave: 1, path: 'docs/plans/B-plan.md' });
  assert.equal(sigs.length, 1);
  assert.equal(sigs[0].kind, 'reading');
  assert.equal(sigs[0].source, 'reported');
  assert.equal(sigs[0].stage, 'plan');
  assert.equal(sigs[0].referent, 'docs/plans/B-plan.md');
  assert.equal(sigs[0].statement, '2 reading(s); see docs/plans/B-plan.md Readings');
  assert.doesNotMatch(sigs[0].statement, /_classify/, 'no reading text in the row');
  assert.deepEqual(signalsFromReadings(undefined, { unit: 'B', wave: 1, path: 'p' }), []);
  assert.deepEqual(signalsFromReadings([], { unit: 'B', wave: 1, path: 'p' }), []);
});

test('signalsFromDocumentSize records a document past twice its target and nothing else', () => {
  const at = { unit: 'B', wave: 1, stage: 'plan', path: 'docs/plans/B-plan.md', targetKb: 12 };
  assert.equal(OVERSIZE_FACTOR, 2);
  assert.deepEqual(signalsFromDocumentSize(12 * 1024, at), []);
  assert.deepEqual(signalsFromDocumentSize(24 * 1024, at), [], 'at the factor is not past it');
  assert.deepEqual(signalsFromDocumentSize(undefined, at), []);
  assert.deepEqual(signalsFromDocumentSize(NaN, at), []);
  const sigs = signalsFromDocumentSize(30 * 1024 + 100, at);
  assert.equal(sigs.length, 1);
  assert.equal(sigs[0].kind, 'document-oversize');
  assert.equal(sigs[0].source, 'structural');
  assert.equal(sigs[0].stage, 'plan');
  assert.equal(sigs[0].unit, 'B');
  assert.equal(sigs[0].referent, 'docs/plans/B-plan.md');
  assert.equal(sigs[0].statement, 'docs/plans/B-plan.md is 30 KB against a 12 KB target');
  const rep = signalsFromDocumentSize(20 * 1024, { ...at, stage: 'completion-report', path: 'docs/plans/B-completion.md', targetKb: 8 });
  assert.equal(rep[0].stage, 'completion-report');
  assert.equal(rep[0].statement, 'docs/plans/B-completion.md is 20 KB against a 8 KB target');
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

test('signalsFromFindings: Significant one row each, Minor one count row with up to five locations, Critical skipped', () => {
  const minor = (i) => ({ severity: 'Minor', statement: `naming ${i}`, location: `a.py:${i}`, stage: 'lens-review', round: 1 });
  const sigs = signalsFromFindings([
    { severity: 'Critical', statement: 'wrong', location: 'a.py:1', stage: 'critique-code', round: 1 },
    { severity: 'Significant', statement: 'fragile', location: 'a.py:2', stage: 'critique-code', round: 2 },
    ...[10, 11, 12, 13, 14, 15, 16].map(minor),
  ], { stage: 'review', unit: 'B', wave: 1 });
  assert.deepEqual(sigs.map((s) => [s.kind, s.source, s.stage, s.referent]), [
    ['finding', 'gate', 'critique-code', 'a.py:2'],
    ['finding', 'gate', 'review', 'a.py:10, a.py:11, a.py:12, a.py:13, a.py:14'],
  ]);
  assert.equal(sigs[0].statement, 'Significant (round 2): fragile');
  assert.equal(sigs[1].statement, '7 Minor finding(s)');
  assert.doesNotMatch(sigs[1].statement, /naming/, 'no Minor text survives');
  assert.deepEqual(signalsFromFindings([{ severity: 'Significant', statement: 's', location: 'l' }], { stage: 'critique-plan', unit: 'B', wave: 1 }).map((s) => s.stage), ['critique-plan'], 'a finding without a stage takes the gate\'s');
  assert.deepEqual(signalsFromFindings([], { stage: 'review', unit: 'B', wave: 1 }), []);
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

test('signalsFromGate: a unit\'s amendment to the architecture document is a record carrying the unit, listed in the digest', () => {
  const sigs = signalsFromGate({
    invalidations: [inv()],
    notes: [{ statement: 'N3 trigger fired, no owner', referent: 'architecture §6 N3' }],
    amendments: [{ unit: 'abi-design', section: '§7.2', statement: 'records the ABI disclosure decision the unit settled' }],
  }, { wave: 2 });
  assert.deepEqual(sigs.map((s) => s.kind), ['architecture-invalidated', 'architecture-note', 'architecture-amendment']);
  const a = sigs[2];
  assert.equal(a.source, 'gate');
  assert.equal(a.stage, 'review-gate');
  assert.equal(a.unit, 'abi-design');
  assert.equal(a.referent, '§7.2');
  assert.equal(a.statement, 'abi-design: records the ABI disclosure decision the unit settled');
  const ev = evaluateWave(sigs);
  assert.deepEqual(ev.stops.map((s) => s.kind), ['architecture-invalidated']);
  assert.deepEqual(digest(ev.all).attention.architectureAmendments, ['abi-design: records the ABI disclosure decision the unit settled']);
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
    signal({ kind: 'finding', source: 'gate', stage: 'review', unit: 'A', wave: 1, statement: '3 Minor finding(s)', referent: 'a.py:1, a.py:2, a.py:3', disposition: 'recorded' }),
    signal({ kind: 'finding', source: 'gate', stage: 'critique-code', unit: 'B', wave: 2, statement: 'Significant: z', disposition: 'recorded' }),
    signal({ kind: 'architecture-note', source: 'gate', stage: 'review-gate', wave: 2, statement: 'N3 has no owner', referent: '§6', disposition: 'recorded' }),
    rec('execution-distress', { unit: 'B', wave: 2 }),
    rec('execution-distress', { unit: 'B', wave: 2 }),
    signal({ kind: 'verdict', source: 'reported', stage: 'decision-replan', wave: 0, statement: 'd1 amended', disposition: 'recorded' }),
    signal({ kind: 'direct-edit', source: 'reported', stage: 'replan', wave: 2, statement: 'note the figure', referent: 'docs/debt.md', disposition: 'recorded' }),
  ];
  const d = digest(ledger);
  assert.equal(d.waves, 2, 'wave 0 (decision re-plan) is not counted');
  assert.equal(d.byKind.finding, 3);
  assert.deepEqual(d.bySource, { reported: 5, gate: 4 });
  assert.deepEqual(d.byDisposition, { recorded: 9 });
  assert.deepEqual(d.attention.architectureNotes, ['N3 has no owner']);
  assert.deepEqual(d.attention.architectureAmendments, []);
  assert.deepEqual(d.attention.unfixedSignificant, { A: 1, B: 1 });
  assert.deepEqual(d.attention.minorFindings, { A: 3 });
  assert.deepEqual(d.attention.distressedUnits, ['B']);
  assert.deepEqual(d.attention.directEdits, ['docs/debt.md — note the figure']);
  assert.match(d.note, /not evidence/);
});

test('only an architecture invalidation supersedes the plan; an unevaluable done condition is a decision for the decision re-plan', () => {
  assert.deepEqual([...PLAN_SUPERSEDING_STOP_KINDS], ['architecture-invalidated']);
});
