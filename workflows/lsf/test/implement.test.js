import { test } from 'node:test';
import assert from 'node:assert/strict';
import { run, GATE_MAX_ROUNDS } from '../implement.js';

// ---------------------------------------------------------------- stub runtime

const ret = (result, extra = {}) => ({ result, assumptions: [], ambiguities: [], contradictions: [], commits: [], ...extra });
const unit = (id, opts = {}) => ({ id, type: 'feature', complete: false, dependencies: [], scope: [`src/${id}`], gates: [], entry: `entry ${id}`, ...opts });
const graph = (sha, units) => ({ sha, path: 'docs/plans/f-breakdown.md', units, executionOrder: units.map((u) => u.id) });

const NO_DISTRESS = { contextNearExhaustion: false, midImplementationDesignDecisions: 0, deviations: 0, deferrals: 0 };
const NO_FINDINGS = ret({ findings: [], systemic: false, summary: 'clean' });

// Default handlers by label prefix (the part before the first ':'). A test
// overrides individual handlers; each receives (prompt, opts, unitId).
function defaults(plan) {
  let sha = 0;
  return {
    load: () => ret({ plan, headBranch: 'integration', architectureSha: 'arch1' }),
    plan: (p, o, u) => ret({ branch: `plan-${u}`, sha: 'p', path: `docs/plans/${u}-plan.md`, planText: `plan for ${u}` }),
    'critique-plan': () => NO_FINDINGS,
    'fix-plan': (p, o, u) => ret({ branch: `fixed-${u}`, sha: 'f', applied: [{ finding: `x-${u}`, fix: 'y' }], notApplied: [], planText: 'fixed' }),
    implement: (p, o, u) => ret({
      branch: `impl-${u}`, sha: 'i', gateProbe: [], distress: NO_DISTRESS, deviations: [], deferrals: [], discoveries: [],
      doneCondition: { evaluable: true, met: true, evidence: 'tests pass' }, standingObligations: [], summary: 'done',
    }, { assumptions: [{ statement: `assumed for ${u}`, referent: u }] }),
    'static-analysis': () => NO_FINDINGS,
    'critique-code': () => NO_FINDINGS,
    'lens-review': () => NO_FINDINGS,
    'fix-code': (p, o, u) => ret({ branch: `fixed-code-${u}`, sha: 'f', applied: [], notApplied: [], planText: '' }),
    'completion-report': (p, o, u) => ret({ branch: `done-${u}`, sha: 'c', path: `docs/plans/${u}-completion.md` }),
    merge: (p, o, u, state) => ret({
      integrationBranch: 'integration',
      merges: state.mergeOrder.map((id) => ({ unit: id, status: 'clean', files: [], sha: 'm' })),
      planSha: `merged${++sha}`,
    }),
    'review-gate': () => ret({ invalidatesArchitecture: false, handoff: 'sufficient', exitCriteria: [] }),
    replan: (p, o, u, state) => ret({
      plan: { ...state.plan, sha: `replan${++sha}`, units: state.plan.units.map((x) => ({ ...x, complete: state.completed.has(x.id) })) },
      amendments: [], overBudget: [], summary: 'no change',
    }),
    ledger: () => ret({ path: 'docs/plans/f-signals.md', sha: 'l' }),
    termination: (p, o, u, state) => ret({ plan: state.plan, appendedUnits: [], directEdits: [] }),
    'direct-edits': () => ret({ path: 'docs/plans/f-breakdown.md', sha: 'e' }),
    obligations: () => ret({ obligations: [] }),
    'decision-log': () => ret({ path: 'docs/plans/f-decisions.md', sha: 'dec1' }),
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
    if (prefix === 'replan' && out) state.plan = out.result.plan;
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

test('clean run: two waves, termination, obligations, complete', async () => {
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
  assert.equal(out.digest.trends.silentWaves.length, 0);
  assert.equal(out.escalation, undefined);
});

test('the re-planned SHA appears in the next wave\'s prompts', async () => {
  const { rt, calls } = runtime(twoWaves());
  await run(rt);
  const planB = calls.find((c) => c.label === 'plan:B');
  assert.match(planB.prompt, /@ replan\d/);
  assert.match(planB.prompt, /entry B/);
  const planA = calls.find((c) => c.label === 'plan:A');
  assert.match(planA.prompt, /@ plan0/);
});

test('a contradiction from implement stops the wave with an escalation record', async () => {
  const { rt, labels } = runtime(twoWaves(), {
    implement: (p, o, u) => ret({
      branch: `impl-${u}`, sha: 'i', gateProbe: [], distress: NO_DISTRESS, deviations: [], deferrals: [], discoveries: [],
      doneCondition: { evaluable: true, met: true, evidence: 'ok' }, standingObligations: [], summary: 'done',
    }, { contradictions: [{ statement: 'schema differs', against: 'docs/architecture/f.md @ arch1' }] }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.equal(out.escalation.stage, 'wave');
  assert.equal(out.escalation.wave, 1);
  assert.equal(out.escalation.plan_sha, 'merged1', 'merge recorded completion before the stop');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['assumption-invalidated']);
  assert.equal(out.escalation.decision_needed.length, 1);
  const l = labels();
  assert.ok(l.includes('ledger:w1'), 'ledger scribed before returning');
  assert.ok(!l.includes('replan:w1'), 'no re-plan after a stop');
  assert.ok(!l.includes('plan:B'));
});

test('a silent wave stops the run', async () => {
  const { rt } = runtime(twoWaves(), {
    implement: (p, o, u) => ret({
      branch: `impl-${u}`, sha: 'i', gateProbe: [], distress: NO_DISTRESS, deviations: [], deferrals: [], discoveries: [],
      doneCondition: { evaluable: true, met: true, evidence: 'ok' }, standingObligations: [], summary: 'done',
    }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['silent-wave']);
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

test('a fix that would alter a requirement stops the gate', async () => {
  let round = 0;
  const { rt } = runtime(twoWaves(), {
    'critique-plan': () => (++round === 1
      ? ret({ findings: [{ severity: 'Critical', statement: 'bad', location: 'plan' }], systemic: false, summary: 's' })
      : NO_FINDINGS),
    'fix-plan': (p, o, u) => ret({ branch: `fixed-${u}`, sha: 'f', applied: [], notApplied: [{ finding: 'bad', reason: 'changes the entry', altersRequirement: true }], planText: '' },
      { contradictions: [{ statement: 'entry wrong', against: 'docs/plans/f-breakdown.md @ plan0' }] }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.ok(out.escalation.signals.every((s) => s.kind === 'assumption-invalidated'));
  assert.ok(out.escalation.signals.some((s) => s.source === 'structural'));
});

test('environment gate drift aborts the unit before implementation', async () => {
  const { rt, labels } = runtime(twoWaves(), {
    implement: (p, o, u) => ret({
      branch: '', sha: '', gateProbe: [{ name: 'node', pin: '22.1', observed: '20.0', drifted: true }], distress: NO_DISTRESS,
      deviations: [], deferrals: [], discoveries: [], doneCondition: { evaluable: false, met: false, evidence: '' }, standingObligations: [], summary: '',
    }, { assumptions: [{ statement: 'x', referent: 'y' }] }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['environment-gate-drift']);
  assert.ok(!labels().some((x) => x.startsWith('critique-code')));
});

test('an unresolvable merge conflict stops; the unit is not completed', async () => {
  const { rt } = runtime(twoWaves(), {
    merge: () => ret({ integrationBranch: 'integration', merges: [{ unit: 'A', status: 'unresolvable', files: ['src/x.js'] }], planSha: 'm1' }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['merge-conflict-unresolvable']);
});

test('load: wrong branch is a precondition stop; invalid plan stops before any unit', async () => {
  const a = runtime(twoWaves(), { load: (p, o, u, s) => ret({ plan: s.plan, headBranch: 'main', architectureSha: 'a' }) });
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
      if (terminated) return ret({ plan: s.plan, appendedUnits: [], directEdits: [] });
      terminated = true;
      const meta = unit('M', { type: 'meta', dependencies: [{ unit: 'A' }] });
      return ret({
        plan: { ...s.plan, sha: 'term1', units: [...s.plan.units, meta], executionOrder: [...s.plan.executionOrder, 'M'] },
        appendedUnits: ['M'], directEdits: [{ path: 'README.md', description: 'mention M' }],
      });
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

test('decisions are committed first and their SHA reaches every prompt', async () => {
  const { rt, calls } = runtime(twoWaves(), {}, { decisions: [{ id: 'd1', question: 'q', decision: 'yes' }] });
  await run(rt);
  assert.equal(calls[0].label, 'decision-log');
  for (const c of calls.slice(1)) assert.match(c.prompt, /Decision log: docs\/plans\/f-decisions\.md @ dec1/, c.label);
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

test('review-gate dedicated fields become signals even when the generic arrays are empty', async () => {
  const { rt } = runtime(twoWaves(), {
    'review-gate': () => ret({ invalidatesArchitecture: true, handoff: 'insufficient', exitCriteria: [] }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind).sort(), ['assumption-invalidated', 'handoff-insufficient']);
  assert.ok(out.escalation.signals.every((s) => s.source === 'gate'));
});

test('a partial handoff is recorded, not stopped', async () => {
  const { rt } = runtime(twoWaves(), {
    'review-gate': () => ret({ invalidatesArchitecture: false, handoff: 'partial', exitCriteria: [] }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  assert.equal(out.digest.byKind['handoff-partial'], 2);
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

test('role tiers reach every call: gate opus, planning fable/medium, default sonnet; static analysis inherits', async () => {
  const { rt, calls } = runtime(twoWaves());
  await run(rt);
  const seen = new Set();
  for (const c of calls) {
    const role = c.label.replace(/:.*$/, '');
    seen.add(role);
    if (/^static-analysis/.test(c.label)) { assert.equal(c.opts.model, undefined, c.label); continue; }
    if (/^(critique-plan|critique-code|lens-review|review-gate|obligations)$/.test(role)) assert.equal(c.opts.model, 'opus', c.label);
    else if (/^(plan|replan|termination)$/.test(role)) assert.deepEqual([c.opts.model, c.opts.effort], ['fable', 'medium'], c.label);
    else assert.equal(c.opts.model, 'sonnet', c.label);
  }
  for (const r of ['critique-code', 'plan', 'implement', 'merge', 'review-gate', 'replan']) assert.ok(seen.has(r), r);
});

test('args.tiers overrides a role without disturbing the others', async () => {
  const { rt, calls } = runtime(twoWaves(), {}, { tiers: { gate: { model: 'sonnet' } } });
  await run(rt);
  const gate = calls.find((c) => c.label.startsWith('critique-code'));
  const impl = calls.find((c) => c.label.startsWith('implement'));
  assert.equal(gate.opts.model, 'sonnet');
  assert.equal(impl.opts.model, 'sonnet');
  assert.equal(calls.find((c) => c.label.startsWith('plan:')).opts.model, 'fable');
});
