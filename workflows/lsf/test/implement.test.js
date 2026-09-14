import { test } from 'node:test';
import assert from 'node:assert/strict';
import { run, GATE_MAX_ROUNDS } from '../implement.js';

// ---------------------------------------------------------------- stub runtime

const ret = (result, extra = {}) => ({ result, commits: [], ...extra });
const unit = (id, opts = {}) => ({ id, type: 'feature', complete: false, dependencies: [], scope: [`src/${id}`], gates: [], ...opts });
const graph = (sha, units) => ({ sha, path: 'docs/plans/f-breakdown.md', units, executionOrder: units.map((u) => u.id) });

// Serialise a graph fixture as the plan's fenced block, as the copy agent
// returns it.
const q = (x) => JSON.stringify(String(x));
const list = (xs) => `[${(xs ?? []).map(q).join(', ')}]`;
function toBlock(plan) {
  const lines = ['```yaml lsf-plan', 'units:'];
  for (const u of plan.units) {
    lines.push(`  - id: ${u.id}`, `    type: ${u.type}`, `    complete: ${u.complete}`,
      `    dependencies: ${list(u.dependencies.map((d) => d.unit))}`, `    scope: ${list(u.scope)}`, `    publishes: ${list(u.publishes)}`);
    if (u.gates.length) {
      lines.push('    gates:');
      for (const g of u.gates) {
        lines.push(`      - name: ${q(g.name)}`, `        kind: ${g.kind}`);
        if (g.pin) lines.push(`        pin: ${q(g.pin)}`);
      }
    } else lines.push('    gates: []');
  }
  lines.push(`execution_order: ${list(plan.executionOrder)}`, '```');
  return lines.join('\n');
}
const copied = (plan, extra = {}) => ret({ planBlock: toBlock(plan), decisionsBlock: '', headBranch: 'integration', architectureSha: 'arch1', ...extra });

const NO_DISTRESS = { contextNearExhaustion: false, midImplementationDesignDecisions: 0, deviations: 0, deferrals: 0 };
const implemented = (u, over = {}) => ({
  branch: `impl-${u}`, sha: 'i', gateProbe: [], distress: NO_DISTRESS, deviations: [], writesOutsideScope: [], deferrals: [], discoveries: [],
  doneCondition: { evaluable: true, met: true, evidence: 'tests pass' }, standingObligations: [], summary: 'done', ...over,
});
// Ids of the re-plan items in a re-plan prompt, each given the same verdict.
const itemIds = (prompt) => [...prompt.matchAll(/"id": "(rp-\d+)"/g)].map((m) => m[1]);
const verdictsFor = (prompt, verdict) => itemIds(prompt).map((id) => ({ id, verdict, statement: 'ok' }));
const NO_FINDINGS = ret({ findings: [], systemic: false, summary: 'clean' });

// Default handlers by label prefix (the part before the first ':'). A test
// overrides individual handlers; each receives (prompt, opts, unitId).
function defaults(plan) {
  let sha = 0;
  return {
    load: (p, o, u, state) => copied(state.plan),
    plan: (p, o, u) => ret({ branch: `plan-${u}`, sha: 'p', path: `docs/plans/${u}-plan.md`, planText: `plan for ${u}`, readings: [] }),
    'critique-plan': () => NO_FINDINGS,
    'fix-plan': (p, o, u) => ret({ branch: `fixed-${u}`, sha: 'f', applied: [{ finding: `x-${u}`, fix: 'y' }], notApplied: [], planText: 'fixed' }),
    implement: (p, o, u) => ret(implemented(u)),
    'critique-code': () => NO_FINDINGS,
    'lens-review': () => NO_FINDINGS,
    'fix-code': (p, o, u) => ret({ branch: `fixed-code-${u}`, sha: 'f', applied: [], notApplied: [], planText: '' }),
    'completion-report': (p, o, u) => ret({ branch: `done-${u}`, sha: 'c', path: `docs/plans/${u}-completion.md` }),
    merge: (p, o, u, state) => ret({
      integrationBranch: 'integration',
      merges: state.mergeOrder.map((id) => ({ unit: id, status: 'clean', files: [], sha: 'm' })),
      planSha: `merged${++sha}`,
    }),
    'review-gate': () => ret({ invalidations: [], notes: [] }),
    replan: (p, o, u, state) => {
      state.plan = { ...state.plan, sha: `replan${++sha}`, units: state.plan.units.map((x) => ({ ...x, complete: state.completed.has(x.id) })) };
      return ret({ planSha: state.plan.sha, amendments: [], overBudget: [], verdicts: verdictsFor(p, 'amended'), summary: 'no change' });
    },
    ledger: () => ret({ path: 'docs/plans/f-signals.md', sha: 'l' }),
    termination: (p, o, u, state) => ret({ planSha: state.plan.sha, appendedUnits: [], directEdits: [] }),
    'direct-edits': () => ret({ path: 'docs/plans/f-breakdown.md', sha: 'e' }),
    obligations: () => ret({ obligations: [] }),
    'decision-log': () => ret({ path: 'docs/plans/f-decisions.md', sha: 'dec1' }),
    'decision-replan': (p, o, u, state) => ret({ planSha: 'decided1', amendments: [{ kind: 'new-edge', unit: 'B', statement: 'B after A' }], overBudget: [],
      verdicts: [...p.matchAll(/"id": "(d\d+)"/g)].map((m) => ({ id: m[1], verdict: 'amended', statement: 'edge added' })), summary: 'applied' }),
  };
}

function runtime(plan, overrides = {}, args = {}) {
  const handlers = { ...defaults(plan), ...overrides };
  const calls = [];
  const logs = [];
  const state = { plan, completed: new Set(), mergeOrder: [] };
  const agent = async (prompt, opts) => {
    const parts = opts.label.split(':');
    const prefix = parts[0];
    const unitId = parts[parts.length - 1];
    calls.push({ label: opts.label, prompt, opts });
    if (prefix === 'merge') {
      state.mergeOrder = [...prompt.matchAll(/^- (\S+): branch/gm)].map((m) => m[1]);
    }
    const h = handlers[prefix];
    if (!h) throw new Error(`no stub for ${opts.label}`);
    const out = await h(prompt, opts, unitId, state);
    if (prefix === 'merge' && out) for (const m of out.result.merges) if (m.status !== 'unresolvable') state.completed.add(m.unit);
    return out;
  };
  const parallel = async (thunks) => Promise.all(thunks.map((t) => t().catch(() => null)));
  const log = (m) => logs.push(m);
  const rt = {
    agent, parallel, log,
    args: { name: 'f', planSha: 'plan0', architectureDoc: 'docs/architecture/f.md', integrationBranch: 'integration', ...args },
  };
  return { rt, calls, logs, labels: () => calls.map((c) => c.label) };
}

const twoWaves = () => graph('plan0', [unit('A', { type: 'foundation' }), unit('B', { dependencies: [{ unit: 'A' }] })]);

// ---------------------------------------------------------------- tests

test('maxUnitsPerWave caps each wave in execution order and defers the rest', async () => {
  const three = graph('plan0', [unit('A'), unit('B'), unit('C')]);
  const { rt, labels } = runtime(three, {}, { maxUnitsPerWave: 2 });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  assert.equal(out.waves, 2);
  assert.deepEqual(labels().filter((x) => x.startsWith('implement')), ['implement:A', 'implement:B', 'implement:C']);
  assert.ok(labels().indexOf('merge:w1') < labels().indexOf('implement:C'));
});

test('clean run: two waves, termination, obligations, complete; silence stops nothing', async () => {
  const { rt, labels } = runtime(twoWaves());
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  assert.equal(out.waves, 2);
  const l = labels();
  assert.deepEqual(l.filter((x) => x.startsWith('implement')), ['implement:A', 'implement:B']);
  assert.ok(l.indexOf('merge:w1') < l.indexOf('review-gate:w1'));
  assert.ok(l.indexOf('review-gate:w1') < l.indexOf('replan:w1'));
  assert.ok(l.indexOf('replan:w1') < l.indexOf('plan:B'));
  assert.ok(l.includes('termination') && l.includes('obligations'));
  assert.ok(!l.includes('direct-edits'));
  assert.ok(!l.includes('ledger:w1'), 'nothing to scribe on a wave with no signals');
  assert.equal(out.ledger.length, 0);
  assert.equal(out.escalation, undefined);
  assert.match(out.digest.note, /not evidence/);
});

test('the re-planned SHA appears in the next wave\'s prompts', async () => {
  const { rt, calls } = runtime(twoWaves());
  await run(rt);
  const planB = calls.find((c) => c.label === 'plan:B');
  assert.match(planB.prompt, /@ replan\d/);
  assert.match(planB.prompt, /`### B` section/);
  const planA = calls.find((c) => c.label === 'plan:A');
  assert.match(planA.prompt, /@ plan0/);
});

test('every unit stage is told what the Scope line means; only commits are asked for beside result', async () => {
  const { rt, calls } = runtime(twoWaves());
  await run(rt);
  for (const c of calls) {
    assert.doesNotMatch(c.prompt, /assumptions|ambiguities|contradictions/, c.label);
    assert.match(c.prompt, /return `commits`/, c.label);
  }
  for (const l of ['plan:A', 'critique-plan:r1:A', 'implement:A', 'critique-code:r1:A', 'lens-review:r1:A', 'completion-report:A']) {
    assert.match(calls.find((c) => c.label === l).prompt, /Scope line declares the files this unit writes[^]*does not limit what the work may edit/, l);
  }
  const plan = calls.find((c) => c.label === 'plan:A').prompt;
  assert.match(plan, /\*\*Readings\*\*/);
  assert.doesNotMatch(plan, /phronithm:feature/);
  const gate = calls.find((c) => c.label === 'review-gate:w1').prompt;
  assert.match(gate, /Answer one question/);
  assert.match(gate, /all five fields/);
  assert.match(gate, /Do not re-evaluate the units' done conditions/);
});

const gateInvalidation = (over = {}) => ({
  section: '§4.8', decision: 'two-tone families are two-part groups with named parts', premise: 'nested art classifies to a GlyphGroup',
  evidence: 'svgpath.py:_classify; brazier ring+fire parses to a bare Glyph', amendment: 'nested art is authored as siblings; _classify keeps ids on holes', ...over,
});

test('a complete architecture invalidation from the gate stops the wave with the amendment in the escalation', async () => {
  const { rt, labels } = runtime(twoWaves(), {
    'review-gate': () => ret({ invalidations: [gateInvalidation()], notes: [] }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.equal(out.escalation.stage, 'wave');
  assert.equal(out.escalation.wave, 1);
  assert.equal(out.escalation.plan_sha, 'merged1', 'merge recorded completion before the stop');
  assert.deepEqual(out.escalation.signals.map((s) => [s.kind, s.source]), [['architecture-invalidated', 'gate']]);
  assert.equal(out.escalation.decision_needed.length, 1);
  assert.match(out.escalation.decision_needed[0].question, /amendment: nested art is authored as siblings/);
  const l = labels();
  assert.ok(l.includes('ledger:w1'), 'ledger scribed before returning');
  assert.ok(!l.includes('replan:w1'), 'no re-plan after a stop that supersedes the plan');
  assert.ok(!l.includes('plan:B'));
});

test('an invalidation missing a field, and every note, is recorded and handed to the re-plan as an item', async () => {
  const seen = [];
  const { rt, calls } = runtime(twoWaves(), {
    'review-gate': () => ret({ invalidations: [gateInvalidation({ amendment: '' })], notes: [{ statement: 'N3 trigger fired; no unit owns it', referent: '§6 N3' }] }),
    replan: (p, o, u, state) => {
      seen.push(p);
      state.plan = { ...state.plan, sha: `replan-x${seen.length}`, units: state.plan.units.map((x) => ({ ...x, complete: state.completed.has(x.id) })) };
      return ret({ planSha: state.plan.sha, amendments: [{ kind: 'unit-added', unit: 'N3', statement: 'owner for N3' }], overBudget: [], verdicts: verdictsFor(p, 'amended'), summary: 'absorbed' });
    },
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
  assert.equal(out.digest.byKind['architecture-note'], 4, 'two notes per wave, both waves');
  assert.equal(out.digest.byKind.verdict, 4);
  assert.equal(out.digest.byKind.amendment, 2);
  assert.deepEqual(out.digest.attention.architectureNotes.slice(0, 2).map((s) => s.slice(0, 24)), ['incomplete invalidation ', 'N3 trigger fired; no uni']);
  const w1 = calls.find((c) => c.label === 'replan:w1').prompt;
  assert.match(w1, /Items this wave[^]*"id": "rp-1"[^]*"kind": "architecture-note"[^]*"id": "rp-2"/);
  assert.doesNotMatch(w1.split('Other records this wave')[1], /architecture-note/);
});

test('deferrals, discoveries, and distress reach the re-plan with ids; readings, deviations, and findings ride along as records', async () => {
  const { rt, calls } = runtime(twoWaves(), {
    plan: (p, o, u) => ret({ branch: `plan-${u}`, sha: 'p', path: `docs/plans/${u}-plan.md`, planText: `plan for ${u}`,
      readings: [{ clause: 'Scope: tests/test_missing.py', reading: 'tests/test_present.py' }] }),
    implement: (p, o, u) => ret(implemented(u, {
      deviations: ['sibling predicate instead of widening'], writesOutsideScope: ['CLAUDE.md'],
      deferrals: [{ statement: 'export left unrotated', belongsTo: 'B' }],
      discoveries: [{ statement: 'B needs an edge to C', affects: 'B' }],
      distress: { ...NO_DISTRESS, midImplementationDesignDecisions: 3 },
    })),
    'critique-code': () => ret({ findings: [{ severity: 'Significant', statement: 'fragile', location: 'a.py:2' }], systemic: false, summary: 's' }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
  const w1 = calls.find((c) => c.label === 'replan:w1').prompt;
  const items = w1.split('Items this wave')[1].split('Merge conflicts')[0];
  for (const k of ['"kind": "deferral"', '"kind": "discovery"', '"kind": "execution-distress"']) assert.match(items, new RegExp(k));
  assert.deepEqual(itemIds(w1), ['rp-1', 'rp-2', 'rp-3']);
  const records = w1.split('Other records this wave')[1];
  for (const k of ['"kind": "reading"', '"kind": "deviation"', '"kind": "finding"']) assert.match(records, new RegExp(k));
  assert.match(records, /writes outside the Scope line: CLAUDE\.md/);
  const kinds = out.ledger.filter((s) => s.wave === 1 && s.unit === 'A').map((s) => s.kind);
  assert.deepEqual([...new Set(kinds)].sort(), ['deferral', 'deviation', 'discovery', 'execution-distress', 'finding', 'reading', 'verdict']);
  assert.ok(out.ledger.every((s) => s.disposition === 'recorded'));
  assert.deepEqual(out.digest.attention.unfixedSignificant, { A: 1, B: 1 });
  assert.deepEqual(out.digest.attention.distressedUnits, ['A', 'B']);
});

test('an item the re-plan cannot absorb, or does not answer, stops after the re-plan', async () => {
  for (const mode of ['cannot', 'missing']) {
    const { rt, labels } = runtime(twoWaves(), {
      implement: (p, o, u) => ret(implemented(u, { discoveries: [{ statement: 'B needs C first', affects: 'B' }, { statement: 'C is redundant', affects: 'C' }] })),
      replan: (p, o, u, state) => ret({ planSha: 'replanned-stop', amendments: [], overBudget: [],
        verdicts: mode === 'cannot' ? [{ id: 'rp-1', verdict: 'cannot', statement: 'needs the requester' }, { id: 'rp-2', verdict: 'amended', statement: 'dropped' }] : [], summary: 's' }),
    });
    const out = await run(rt);
    assert.equal(out.status, 'stopped', mode);
    assert.equal(out.escalation.stage, 'replan');
    const stops = out.ledger.filter((s) => s.disposition === 'stopped');
    assert.deepEqual(stops.map((s) => s.kind), mode === 'cannot' ? ['plan-defect-unabsorbed'] : ['plan-defect-unabsorbed', 'plan-defect-unabsorbed'], mode);
    assert.equal(stops[0].referent, 'B');
    assert.match(stops[0].statement, mode === 'cannot' ? /cannot absorb: needs the requester — \[implement\] discovery: B needs C first/ : /no verdict — \[implement\] discovery: B needs C first/);
    assert.ok(labels().includes('replan:w1') && !labels().includes('plan:B'));
    assert.equal(out.escalation.plan_sha, 'replanned-stop', 'relaunch from the re-plan commit');
    assert.equal(out.escalation.pre_replan_sha, 'merged1');
    if (mode === 'cannot') assert.equal(out.ledger.filter((s) => s.kind === 'verdict').length, 1);
  }
});

test('a flagged over-budget unit is recorded and the run continues; an unrepaired one stops', async () => {
  for (const disposition of ['flagged', 'unrepaired']) {
    const { rt } = runtime(twoWaves(), {
      replan: (p, o, u, state) => {
        state.plan = { ...state.plan, sha: 'replanned', units: state.plan.units.map((x) => ({ ...x, complete: state.completed.has(x.id) })) };
        return ret({ planSha: 'replanned', amendments: [], overBudget: [{ unit: 'B', disposition, statement: 'three written paths' }], verdicts: [], summary: 's' });
      },
    });
    const out = await run(rt);
    if (disposition === 'flagged') {
      assert.equal(out.status, 'complete', 'flag discharges the budget check');
      const flags = out.ledger.filter((s) => s.kind === 'over-budget-flagged');
      assert.equal(flags.length, 2, 'reported each re-plan');
      assert.equal(flags[0].disposition, 'recorded');
      assert.equal(flags[0].unit, 'B');
    } else {
      assert.equal(out.status, 'stopped');
      assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['unit-over-budget']);
    }
  }
});

test('a document-text verdict records the architecture amendment and does not stop', async () => {
  const { rt } = runtime(twoWaves(), {
    'review-gate': () => ret({ invalidations: [], notes: [{ statement: 'U5 row is stale', referent: '§6 U5' }] }),
    replan: (p, o, u, state) => {
      state.plan = { ...state.plan, sha: `rp-${state.completed.size}`, units: state.plan.units.map((x) => ({ ...x, complete: state.completed.has(x.id) })) };
      return ret({ planSha: state.plan.sha, amendments: [], overBudget: [],
        verdicts: [...p.matchAll(/"id": "(rp-\d+)"/g)].map((m) => ({ id: m[1], verdict: 'document-text', statement: '§6 row U5 now reads resolved' })), summary: 's' });
    },
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  const amendments = out.ledger.filter((s) => s.kind === 'architecture-amendment');
  assert.equal(amendments.length, 2, 'one per wave');
  assert.equal(amendments[0].referent, '§6 U5');
  assert.equal(amendments[0].disposition, 'recorded');
  assert.deepEqual(out.digest.attention.architectureAmendments, ['§6 row U5 now reads resolved', '§6 row U5 now reads resolved']);
  assert.match(out.ledger.find((s) => s.kind === 'verdict').statement, /rp-1 document-text: §6 row U5 now reads resolved/);
});

test('critique gate hits the hard cap: fixes on rounds 1-3, stop on round 4, no implement', async () => {
  const critical = ret({ findings: [{ severity: 'Critical', statement: 'bad', location: 'plan' }], systemic: false, summary: 's' });
  const { rt, labels } = runtime(twoWaves(), { 'critique-plan': () => critical });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.ok(out.escalation.signals.some((s) => s.kind === 'gate-non-convergence'));
  const l = labels();
  assert.equal(l.filter((x) => x.startsWith('critique-plan')).length, GATE_MAX_ROUNDS);
  assert.equal(l.filter((x) => x.startsWith('fix-plan')).length, GATE_MAX_ROUNDS - 1);
  assert.ok(!l.includes('implement:A'));
  assert.ok(!l.includes('merge:w1'), 'nothing to merge');
});

test('a Critical the fix stage declines stops the gate at once', async () => {
  let round = 0;
  const { rt, labels } = runtime(twoWaves(), {
    'critique-plan': () => (++round === 1
      ? ret({ findings: [{ severity: 'Critical', statement: 'bad', location: 'plan' }], systemic: false, summary: 's' })
      : NO_FINDINGS),
    'fix-plan': (p, o, u) => ret({ branch: `fixed-${u}`, sha: 'f', applied: [], notApplied: [{ finding: 'bad', reason: 'changes the entry' }], planText: '' }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.deepEqual(out.escalation.signals.map((s) => [s.kind, s.source]), [['gate-non-convergence', 'structural']]);
  assert.match(out.escalation.signals[0].statement, /declined by the fix stage — bad: changes the entry/);
  assert.equal(labels().filter((x) => x.startsWith('critique-plan')).length, 1);
});

test('environment gate drift aborts the unit before implementation', async () => {
  const { rt, labels } = runtime(twoWaves(), {
    implement: (p, o, u) => ret(implemented(u, { branch: '', sha: '', gateProbe: [{ name: 'node', pin: '22.1', observed: '20.0', drifted: true }], doneCondition: { evaluable: false, met: false, evidence: '' } })),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['environment-gate-drift']);
  assert.ok(!labels().some((x) => x.startsWith('critique-code')));
});

test('an unevaluable done condition stops the unit before review', async () => {
  const { rt, labels } = runtime(twoWaves(), {
    implement: (p, o, u) => ret(implemented(u, { doneCondition: { evaluable: false, met: false, evidence: 'names a test that does not exist' } })),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['done-condition-unevaluable']);
  assert.ok(!labels().some((x) => x.startsWith('critique-code')));
});

test('an unresolvable merge conflict stops; the unit is not completed; nothing merged, so no re-plan', async () => {
  const { rt, labels } = runtime(twoWaves(), {
    merge: () => ret({ integrationBranch: 'integration', merges: [{ unit: 'A', status: 'unresolvable', files: ['src/x.js'] }], planSha: 'm1' }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['merge-conflict-unresolvable']);
  assert.equal(out.escalation.plan_sha, 'm1');
  assert.ok(!labels().includes('replan:w1'));
});

const twoParallel = () => graph('plan0', [unit('A'), unit('B'), unit('C', { dependencies: [{ unit: 'A' }, { unit: 'B' }] })]);

test('a wave stop with a unit merged still re-plans over the merged report, then stops from the re-plan SHA', async () => {
  const { rt, calls, labels } = runtime(twoParallel(), {
    merge: () => ret({ integrationBranch: 'integration', merges: [
      { unit: 'A', status: 'unresolvable', files: ['src/x.js'] },
      { unit: 'B', status: 'clean', files: [], sha: 'mb' },
    ], planSha: 'm1' }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.equal(out.escalation.stage, 'wave');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['merge-conflict-unresolvable']);
  assert.equal(out.escalation.plan_sha, 'replan1', 'the re-plan commit is the plan to relaunch from');
  assert.equal(out.escalation.pre_replan_sha, 'm1');
  const l = labels();
  assert.ok(l.indexOf('review-gate:w1') < l.indexOf('replan:w1'));
  assert.ok(!l.some((x) => x.startsWith('plan:C')), 'no unit starts after the stop');
  const rp = calls.find((c) => c.label === 'replan:w1').prompt;
  assert.match(rp, /docs\/plans\/B-completion\.md/);
  assert.match(rp, /Merge conflicts: none/, 'the unresolvable conflict is not an item to absorb');
  assert.match(rp, /The run stops after this re-plan[\s\S]*"kind": "merge-conflict-unresolvable"/);
  const ledgerEntries = calls.find((c) => c.label === 'ledger:w1').prompt;
  assert.equal((ledgerEntries.match(/merge-conflict-unresolvable/g) ?? []).length, 1, 'the stop is scribed once');
});

test('a re-plan cannot verdict during a wave stop joins the same escalation', async () => {
  const { rt } = runtime(twoParallel(), {
    implement: (p, o, u) => ret(implemented(u, u === 'B' ? { deferrals: [{ statement: 'later', to: 'C' }] } : {})),
    merge: () => ret({ integrationBranch: 'integration', merges: [
      { unit: 'A', status: 'regressed', files: ['test/a.test.js'], sha: 'ma' },
      { unit: 'B', status: 'clean', files: [], sha: 'mb' },
    ], planSha: 'm1' }),
    replan: (p) => ret({ planSha: 'replan1', amendments: [], overBudget: [], verdicts: verdictsFor(p, 'cannot'), summary: 'no' }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.equal(out.escalation.stage, 'wave');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['merge-regression', 'plan-defect-unabsorbed']);
});

test('a mechanically resolved conflict between claimed-disjoint units is a re-plan item, not a stop', async () => {
  const { rt, calls } = runtime(twoWaves(), {
    merge: (p, o, u, state) => ret({ integrationBranch: 'integration', merges: state.mergeOrder.map((id) => ({ unit: id, status: 'resolved', files: ['CLAUDE.md'], sha: 'm' })), planSha: 'm1' }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
  const w1 = calls.find((c) => c.label === 'replan:w1').prompt;
  assert.match(w1, /"kind": "claimed-disjoint-conflict"/);
  assert.match(w1, /Merge conflicts: \[[^]*"status": "resolved"/);
});

test('load: wrong branch is a precondition stop; invalid plan stops before any unit', async () => {
  const a = runtime(twoWaves(), { load: (p, o, u, s) => copied(s.plan, { headBranch: 'main' }) });
  const outA = await run(a.rt);
  assert.equal(outA.status, 'stopped');
  assert.equal(outA.escalation.stage, 'load');
  assert.deepEqual(outA.escalation.signals.map((s) => s.kind), ['precondition']);
  assert.ok(a.labels().includes('ledger:w0'), 'stop at load is scribed');
  assert.ok(outA.ledger.every((s) => s.disposition === 'stopped'));

  const overlap = graph('plan0', [unit('A', { scope: ['src'] }), unit('B', { scope: ['src/b'] })]);
  const b = runtime(overlap);
  const outB = await run(b.rt);
  assert.equal(outB.status, 'stopped');
  assert.deepEqual(outB.escalation.signals.map((s) => s.kind), ['unserialised-overlap']);
  assert.ok(!b.labels().some((x) => x.startsWith('plan:')));
});

test('no ready unit because of an uncleared organisational gate stops', async () => {
  const gated = graph('plan0', [unit('A', { gates: [{ name: 'legal sign-off', kind: 'organisational', cleared: false }] })]);
  const { rt } = runtime(gated);
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.equal(out.escalation.stage, 'schedule');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['no-ready-unit']);
});

test('termination appends meta units, which run as a further wave; direct edits are scribed', async () => {
  let terminated = false;
  const { rt, labels } = runtime(graph('plan0', [unit('A')]), {
    termination: (p, o, u, s) => {
      if (terminated) return ret({ planSha: s.plan.sha, appendedUnits: [], directEdits: [] });
      terminated = true;
      const meta = unit('M', { type: 'meta', dependencies: [{ unit: 'A' }] });
      s.plan = { ...s.plan, sha: 'term1', units: [...s.plan.units, meta], executionOrder: [...s.plan.executionOrder, 'M'] };
      return ret({ planSha: 'term1', appendedUnits: ['M'], directEdits: [{ path: 'README.md', description: 'mention M' }] });
    },
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  assert.equal(out.waves, 2);
  const l = labels();
  assert.ok(l.includes('implement:M'));
  assert.ok(l.includes('direct-edits'));
  assert.ok(l.indexOf('direct-edits') < l.indexOf('plan:M'));
});

test('an undischarged standing obligation at termination stops', async () => {
  const { rt } = runtime(graph('plan0', [unit('A')]), {
    obligations: () => ret({ obligations: [{ statement: 'perf numbers collected', discharged: false, evidence: 'none' }] }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.equal(out.escalation.stage, 'obligations');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['standing-obligation-undischarged']);
});

test('decisions are committed first, applied by a re-plan, and both SHAs reach every later prompt', async () => {
  const decisionsBlock = ['```yaml lsf-decisions', 'decisions:', '  - id: d1', '    date: 2026-09-12', '    question: q', '    decision: yes', '```'].join('\n');
  const { rt, calls, labels } = runtime(twoWaves(), { load: (p, o, u, st) => copied(st.plan, { decisionsBlock }) }, { decisions: [{ id: 'd1', question: 'q', decision: 'yes' }] });
  const out = await run(rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
  assert.deepEqual(labels().slice(0, 3), ['decision-log', 'decision-replan', 'ledger:w0']);
  for (const c of calls.slice(1)) assert.match(c.prompt, /Decision log: docs\/plans\/f-decisions\.md @ dec1/, c.label);
  const rp = calls[1].prompt;
  assert.match(rp, /Decomposition plan: docs\/plans\/f-breakdown\.md @ plan0/);
  assert.match(rp, /"id": "d1"[^]*"decision": "yes"/);
  assert.doesNotMatch(rp, /"clears"/);
  const load = calls.find((c) => c.label === 'load');
  assert.match(load.prompt, /git show decided1:docs\/plans\/f-breakdown\.md/, 'loads the re-planned SHA');
  assert.match(calls.find((c) => c.label === 'plan:A').prompt, /@ decided1/);
  const w0 = out.ledger.filter((s) => s.wave === 0).map((s) => [s.stage, s.kind]);
  assert.deepEqual(w0, [['decision-replan', 'verdict'], ['decision-replan', 'amendment']]);
  assert.match(calls[2].prompt, /## Wave 0[^]*decision-replan \| reported \| verdict \| d1 amended: edge added — q/);
});

test('a decision the re-plan cannot carry out stops before load, with both SHAs in the record', async () => {
  const { rt, labels } = runtime(twoWaves(), {
    'decision-replan': () => ret({ planSha: 'decided1', amendments: [], overBudget: [], verdicts: [{ id: 'd1', verdict: 'cannot', statement: 'changes §4' }], summary: 's' }),
  }, { decisions: [{ id: 'd1', question: 'q', decision: 'rewrite the seam' }] });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.equal(out.escalation.stage, 'decision-replan');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['plan-defect-unabsorbed']);
  assert.match(out.escalation.signals[0].statement, /cannot absorb: changes §4 — q/);
  assert.equal(out.escalation.plan_sha, 'decided1');
  assert.equal(out.escalation.pre_replan_sha, 'plan0');
  assert.ok(!labels().includes('load'));
});

test('a standing over-budget flag reported at decision-replan does not stop the relaunch', async () => {
  const decisionsBlock = ['```yaml lsf-decisions', 'decisions:', '  - id: d1', '    date: 2026-09-12', '    question: q', '    decision: proceed', '```'].join('\n');
  const { rt, labels } = runtime(twoWaves(), {
    load: (p, o, u, st) => copied(st.plan, { decisionsBlock }),
    'decision-replan': () => ret({ planSha: 'decided1', amendments: [], overBudget: [{ unit: 'B', disposition: 'flagged', statement: 'D7 stands' }],
      verdicts: [{ id: 'd1', verdict: 'no-change', statement: 'D7 reaffirmed' }], summary: 's' }),
  }, { decisions: [{ id: 'd1', question: 'q', decision: 'proceed, no split' }] });
  const out = await run(rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
  assert.ok(labels().includes('plan:A'));
  assert.deepEqual(out.ledger.filter((s) => s.wave === 0).map((s) => s.kind), ['verdict', 'over-budget-flagged']);
  assert.equal(out.waves, 2);
  assert.equal(out.digest.waves, 0, 'silent waves leave no ledger entries; wave 0 is not counted');
});

test('a null agent return is a stop', async () => {
  const { rt } = runtime(twoWaves(), { implement: () => null });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.ok(out.escalation.signals.some((s) => s.kind === 'agent-null'));
});

test('required args are enforced', async () => {
  const { rt } = runtime(twoWaves(), {}, { planSha: undefined });
  await assert.rejects(() => run(rt), /planSha/);
});

test('a clean merge that regresses the suite stops and does not complete the unit', async () => {
  const { rt } = runtime(twoWaves(), {
    merge: () => ret({ integrationBranch: 'integration', merges: [{ unit: 'A', status: 'regressed', files: ['test/a.test.js'], sha: 'm' }], planSha: 'm1' }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['merge-regression']);
});

test('fixed Critical findings reach the ledger on a passing gate', async () => {
  let round = 0;
  const { rt, calls } = runtime(twoWaves(), {
    'critique-plan': () => (++round % 2 === 1
      ? ret({ findings: [{ severity: 'Critical', statement: 'bad', location: 'plan' }], systemic: false, summary: 's' })
      : NO_FINDINGS),
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
  assert.equal(out.digest.byKind['critical-fixed'], 2);
  const ledger1 = calls.find((c) => c.label === 'ledger:w1');
  assert.match(ledger1.prompt, /critical-fixed/);
});

test('role tiers reach every call: critique sonnet, gate opus, planning fable/medium, mechanical haiku/low, default sonnet', async () => {
  const { rt, calls } = runtime(twoWaves());
  await run(rt);
  const seen = new Set();
  for (const c of calls) {
    const role = c.label.replace(/:.*$/, '');
    seen.add(role);
    if (/^(critique-plan|critique-code|lens-review|critique-design)$/.test(role)) assert.equal(c.opts.model, 'sonnet', c.label);
    else if (role === 'review-gate') assert.equal(c.opts.model, 'opus', c.label);
    else if (/^(plan|replan|termination)$/.test(role)) assert.deepEqual([c.opts.model, c.opts.effort], ['fable', 'medium'], c.label);
    else if (/^(load|ledger|decision-log)$/.test(role)) assert.deepEqual([c.opts.model, c.opts.effort], ['haiku', 'low'], c.label);
    else assert.equal(c.opts.model, 'sonnet', c.label);
  }
  for (const r of ['critique-code', 'plan', 'implement', 'merge', 'review-gate', 'replan', 'load', 'obligations']) assert.ok(seen.has(r), r);
});

test('a wave stop scribes once, with the wave\'s records and stops in one rendered table', async () => {
  const { rt, calls } = runtime(twoWaves(), {
    implement: (p, o, u) => ret(implemented(u, { deviations: ['a | b'] })),
    'review-gate': () => ret({ invalidations: [gateInvalidation()], notes: [] }),
  });
  await run(rt);
  const scribes = calls.filter((c) => c.label === 'ledger:w1');
  assert.equal(scribes.length, 1);
  const p = scribes[0].prompt;
  assert.match(p, /<<<LEDGER\n## Wave 1\n/);
  assert.match(p, /\| 1 \| A \| implement \| reported \| deviation \| a \\\| b \| A \| recorded \|/);
  assert.match(p, /\| architecture-invalidated \| section: §4\.8; [^|]* \| §4\.8 \| stopped \|/);
  assert.match(p, /# Signal ledger: f/);
});

test('args.tiers overrides a role without disturbing the others', async () => {
  const { rt, calls } = runtime(twoWaves(), {}, { tiers: { critique: { model: 'opus' } } });
  await run(rt);
  const critique = calls.find((c) => c.label.startsWith('critique-code'));
  const impl = calls.find((c) => c.label.startsWith('implement'));
  assert.equal(critique.opts.model, 'opus');
  assert.equal(impl.opts.model, 'sonnet');
  assert.equal(calls.find((c) => c.label.startsWith('review-gate')).opts.model, 'opus');
  assert.equal(calls.find((c) => c.label.startsWith('plan:')).opts.model, 'fable');
});

test('load: a bad block stops before any unit; the copy stage returns only the blocks', async () => {
  const garbled = runtime(twoWaves(), { load: () => ret({ planBlock: 'units: [', decisionsBlock: '', headBranch: 'integration', architectureSha: 'a' }) });
  const outG = await run(garbled.rt);
  assert.equal(outG.status, 'stopped');
  assert.equal(outG.escalation.stage, 'load');
  assert.deepEqual(outG.escalation.signals.map((x) => [x.kind, x.source]), [['invalid-plan', 'structural']]);
  assert.ok(!garbled.labels().some((x) => x.startsWith('plan:')));
  assert.match(garbled.calls[0].prompt, /This stage copies; the script parses/);
});

test('load: a decision log clears an organisational gate; clearing an environment gate is a stop', async () => {
  const gated = graph('plan0', [unit('A', { gates: [{ name: 'legal', kind: 'organisational', cleared: false }, { name: 'node', kind: 'environment', cleared: false, pin: '22' }] })]);
  const log = (clears) => ['```yaml lsf-decisions', 'decisions:', '  - id: D1', '    date: 2026-09-11', '    question: q', '    decision: d', `    clears: [${clears}]`, '```'].join('\n');
  const ok = runtime(gated, { load: (p, o, u, s) => copied(s.plan, { decisionsBlock: log('A/legal') }) }, { decisionLogSha: 'dec0' });
  const outOk = await run(ok.rt);
  assert.equal(outOk.status, 'complete', JSON.stringify(outOk.escalation?.signals));
  assert.match(ok.calls[0].prompt, /git show dec0:docs\/plans\/f-decisions\.md/);

  const bad = runtime(gated, { load: (p, o, u, s) => copied(s.plan, { decisionsBlock: log('A/node') }) }, { decisionLogSha: 'dec0' });
  const outBad = await run(bad.rt);
  assert.equal(outBad.status, 'stopped');
  assert.match(outBad.escalation.signals[0].statement, /environment gate A\/node/);

  const missing = runtime(gated, { load: (p, o, u, s) => copied(s.plan) }, { decisionLogSha: 'dec0' });
  const outMissing = await run(missing.rt);
  assert.equal(outMissing.status, 'stopped');
  assert.equal(outMissing.escalation.signals[0].referent, 'lsf-decisions');
});

test('the re-planned and terminated plans are re-read from their SHAs, not returned inline', async () => {
  const { rt, calls } = runtime(twoWaves());
  await run(rt);
  const loads = calls.filter((c) => c.label.startsWith('load'));
  assert.deepEqual(loads.map((c) => c.label), ['load', 'load:w1', 'load:w2', 'load:termination']);
  assert.match(loads[1].prompt, /git show replan\d:docs\/plans\/f-breakdown\.md/);
  assert.match(calls.find((c) => c.label === 'plan:B').prompt, /@ replan\d/);
});

test('a seeded stage return stands in for the agent, matched by label', async () => {
  const planned = ret({ branch: 'salvaged-A', sha: 's', path: 'docs/plans/A-plan.md', planText: 'salvaged plan for A', readings: [] });
  const { rt, calls, logs } = runtime(twoWaves(), {}, { seed: { 'plan:A': planned } });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  const l = calls.map((c) => c.label);
  assert.ok(!l.includes('plan:A'), 'the seeded agent is not spawned');
  assert.ok(l.includes('plan:B'), 'unseeded labels run');
  assert.match(calls.find((c) => c.label === 'critique-plan:r1:A').prompt, /salvaged plan for A/);
  assert.match(calls.find((c) => c.label === 'implement:A').prompt, /salvaged-A/);
  assert.ok(logs.some((m) => m.startsWith('plan:A: seeded')));
});
