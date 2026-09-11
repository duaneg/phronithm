import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseYaml, extractBlock, parsePlanGraph, parseDecisions, applyClearances, loadGraph } from '../graph.js';

// ---------------------------------------------------------------- YAML subset

test('parseYaml: mappings, sequences, flow sequences, scalars, comments', () => {
  const doc = parseYaml(`
# leading comment
units:
  - id: a          # trailing comment
    type: foundation
    complete: true
    dependencies: []
    scope: [src/a.py, "src/b.py::Klass", 'x y']
    gates:
      - name: rsvg-convert
        kind: environment
        pin: "2.62.3"
  - id: b
    dependencies: [a]
    note: 'it''s # not a comment'
execution_order:
  - a
  - b
empty:
`);
  assert.deepEqual(doc, {
    units: [
      { id: 'a', type: 'foundation', complete: true, dependencies: [], scope: ['src/a.py', 'src/b.py::Klass', 'x y'], gates: [{ name: 'rsvg-convert', kind: 'environment', pin: '2.62.3' }] },
      { id: 'b', dependencies: ['a'], note: "it's # not a comment" },
    ],
    execution_order: ['a', 'b'],
    empty: null,
  });
});

test('parseYaml: scalars stay strings except booleans and null', () => {
  assert.deepEqual(parseYaml('a: 2026-09-11\nb: 3\nc: false\nd: ~\ne: "true"'), { a: '2026-09-11', b: '3', c: false, d: null, e: 'true' });
});

test('parseYaml: rejects what the subset excludes, with a line number', () => {
  for (const [src, re] of [
    ['a: &x 1', /line 1: unsupported YAML syntax: &/],
    ['a: |\n  text', /unsupported YAML syntax: \|/],
    ['a: {b: 1}', /flow mappings/],
    ['a: [[1]]', /nested flow/],
    ['a: 1\na: 2', /line 2: duplicate key a/],
    ['a:\n\tb: 1', /tab/],
    ['- a\nb: 1', /expected a sequence item/],
    ['a: "open', /unterminated/],
    ['a: 1\n  b: 2', /line 2: unexpected indent/],
    ['a: [1, ]', /empty item/],
  ]) {
    assert.throws(() => parseYaml(src), re, src);
  }
});

test('parseYaml: empty document is null', () => {
  assert.equal(parseYaml('\n# nothing\n'), null);
});

// ---------------------------------------------------------------- blocks

test('extractBlock: exactly one tagged fence', () => {
  const md = 'intro\n\n```yaml lsf-plan\nunits: []\nexecution_order: []\n```\n\n```yaml\nother: 1\n```\n';
  assert.deepEqual(extractBlock(md, 'lsf-plan'), { body: 'units: []\nexecution_order: []\n', problem: null });
  assert.match(extractBlock(md, 'lsf-decisions').problem, /no ```yaml lsf-decisions block/);
  assert.match(extractBlock(md + md, 'lsf-plan').problem, /2 ```yaml lsf-plan blocks/);
  assert.match(extractBlock(null, 'lsf-plan').problem, /no /);
});

// ---------------------------------------------------------------- plan graph

const PLAN = `
units:
  - id: a
    type: foundation
    complete: true
    scope: [src/a.py]
    publishes: [A]
  - id: b
    type: feature
    dependencies: [a]
    scope: [src/b.py::f, tests/test_b.py]
    gates:
      - name: legal
        kind: organisational
      - name: node
        kind: environment
        pin: "22.1"
execution_order: [a, b]
`;

test('parsePlanGraph: maps the block onto the scheduling shape; gates start uncleared', () => {
  const { plan, problems } = parsePlanGraph(PLAN);
  assert.deepEqual(problems, []);
  assert.deepEqual(plan, {
    units: [
      { id: 'a', type: 'foundation', complete: true, dependencies: [], scope: ['src/a.py'], publishes: ['A'], gates: [] },
      {
        id: 'b', type: 'feature', complete: false, dependencies: [{ unit: 'a' }], scope: ['src/b.py::f', 'tests/test_b.py'], publishes: [],
        gates: [{ name: 'legal', kind: 'organisational', cleared: false }, { name: 'node', kind: 'environment', cleared: false, pin: '22.1' }],
      },
    ],
    executionOrder: ['a', 'b'],
  });
});

test('parsePlanGraph: every shape defect is an invalid-plan problem with a referent', () => {
  const { plan, problems } = parsePlanGraph(`
units:
  - id: a
    type: widget
    complete: yes
    dependencies: a
    gates:
      - name: g
        kind: legal
      - name: g
        kind: release
      - kind: data
  - type: feature
  - id: c
    type: feature
    extra: 1
execution_order: [a, a]
stray: 1
`);
  assert.ok(plan, 'a readable block with defects still yields a plan');
  const statements = problems.map((p) => p.statement);
  assert.ok(problems.every((p) => p.kind === 'invalid-plan' && p.referent));
  for (const re of [
    /a: type must be one of/, /a: complete must be true or false/, /a: dependencies must be a sequence/,
    /a\/g: gate kind must be one of/, /a: duplicate gate name g/, /gates\[2\]: gate missing name/,
    /units\[1\]: missing id/, /c: unknown key extra/, /execution_order names a more than once/, /unknown top-level key stray/,
  ]) {
    assert.ok(statements.some((s) => re.test(s)), `${re} in ${JSON.stringify(statements, null, 1)}`);
  }
});

test('parsePlanGraph: an unreadable block yields no plan and one problem', () => {
  const bad = parsePlanGraph('units:\n  - id: a\n   type: x');
  assert.equal(bad.plan, null);
  assert.equal(bad.problems.length, 1);
  assert.match(bad.problems[0].statement, /plan block: line 3/);
  assert.equal(parsePlanGraph('- a').plan, null);
  assert.equal(parsePlanGraph('units: 3').plan, null);
});

// ---------------------------------------------------------------- decisions

test('parseDecisions: entries with clears; null body is an empty log', () => {
  const { decisions, problems } = parseDecisions(`
decisions:
  - id: D1
    date: 2026-09-11
    question: Is the legal review done?
    decision: Yes, signed off by counsel.
    clears: [b/legal]
  - id: D2
    date: 2026-09-11
    question: q
    decision: d
    resolves: [sig-3]
`);
  assert.deepEqual(problems, []);
  assert.equal(decisions.length, 2);
  assert.deepEqual(decisions[0].clears, ['b/legal']);
  assert.deepEqual(decisions[1], { id: 'D2', date: '2026-09-11', question: 'q', decision: 'd', clears: [], resolves: ['sig-3'] });
  assert.deepEqual(parseDecisions(null), { decisions: [], problems: [] });
  assert.deepEqual(parseDecisions('# empty\n'), { decisions: [], problems: [] });
});

test('parseDecisions: missing fields and duplicate ids are problems', () => {
  const { problems } = parseDecisions('decisions:\n  - id: D1\n    date: x\n  - id: D1\n    date: x\n    question: q\n    decision: d\n    clears: b/legal\n');
  const s = problems.map((p) => p.statement);
  assert.ok(s.some((x) => /D1: missing question/.test(x)));
  assert.ok(s.some((x) => /D1: clears must be a sequence/.test(x)));
  assert.ok(s.some((x) => /duplicate decision id D1/.test(x)));
});

test('applyClearances: clears named gates; unknown or environment targets are problems', () => {
  const { plan } = parsePlanGraph(PLAN);
  const problems = applyClearances(plan, [
    { id: 'D1', clears: ['b/legal'] },
    { id: 'D2', clears: ['b/node', 'b/nope', 'zzz/legal'] },
  ]);
  const b = plan.units[1];
  assert.deepEqual(b.gates[0], { name: 'legal', kind: 'organisational', cleared: true, clearedBy: 'D1' });
  assert.equal(b.gates[1].cleared, false, 'environment gates are never cleared by decision');
  assert.deepEqual(problems.map((p) => p.referent).sort(), ['b/node', 'b/nope', 'zzz/legal']);
});

// ---------------------------------------------------------------- loadGraph

const fence = (tag, body) => `# Doc\n\nprose\n\n\`\`\`yaml ${tag}\n${body.trim()}\n\`\`\`\n\nmore prose\n`;

test('loadGraph: plan and decisions join; no decisions document leaves gates uncleared', () => {
  const planMd = fence('lsf-plan', PLAN);
  const withLog = loadGraph({ planMarkdown: planMd, decisionsMarkdown: fence('lsf-decisions', 'decisions:\n  - id: D1\n    date: d\n    question: q\n    decision: a\n    clears: [b/legal]') });
  assert.deepEqual(withLog.problems, []);
  assert.equal(withLog.plan.units[1].gates[0].cleared, true);
  assert.equal(withLog.decisions.length, 1);

  const without = loadGraph({ planMarkdown: planMd, decisionsMarkdown: null });
  assert.deepEqual(without.problems, []);
  assert.equal(without.plan.units[1].gates[0].cleared, false);
});

test('loadGraph: a missing plan block is the only problem; a bad decisions block is a problem beside a good plan', () => {
  const none = loadGraph({ planMarkdown: '# no block\n', decisionsMarkdown: null });
  assert.equal(none.plan, null);
  assert.equal(none.problems.length, 1);
  assert.equal(none.problems[0].referent, 'lsf-plan');

  const badLog = loadGraph({ planMarkdown: fence('lsf-plan', PLAN), decisionsMarkdown: '# log without block\n' });
  assert.ok(badLog.plan);
  assert.deepEqual(badLog.problems.map((p) => p.referent), ['lsf-decisions']);
});
