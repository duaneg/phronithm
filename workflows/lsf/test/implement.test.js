import { test } from 'node:test';
import assert from 'node:assert/strict';
import { run, renderCompletionReport, GATE_MAX_ROUNDS } from '../implement.js';

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
  branch: `impl-${u}`, sha: 'i', gateProbe: [], gates: [{ command: 'pytest -q', status: 'green', summary: '412 passed' }, { command: 'ruff check .', status: 'green', summary: '' }],
  distress: NO_DISTRESS, deviations: [], writesOutsideScope: [], deferrals: [], discoveries: [],
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
    plan: (p, o, u) => ret({ branch: `plan-${u}`, sha: 'p', path: `docs/plans/${u}-plan.md`, readings: [] }),
    'critique-plan': () => NO_FINDINGS,
    'fix-plan': (p, o, u) => ret({ branch: `fixed-${u}`, sha: 'f', applied: [{ finding: `x-${u}`, fix: 'y' }], notApplied: [] }),
    implement: (p, o, u) => ret(implemented(u)),
    'critique-code': () => NO_FINDINGS,
    'lens-review': () => NO_FINDINGS,
    'fix-code': (p, o, u) => ret({ branch: `fixed-code-${u}`, sha: 'f', applied: [], notApplied: [], gates: [{ command: 'pytest -q', status: 'green', summary: '413 passed' }] }),
    'completion-report': (p, o, u) => ret({ branch: `done-${u}`, sha: 'c', path: `docs/plans/${u}-completion.md` }),
    cleanup: () => ret({ removed: 0, kept: [] }),
    merge: (p, o, u, state) => {
      state.plan = { ...state.plan, sha: `merged${++sha}`, units: state.plan.units.map((x) => ({ ...x, complete: state.completed.has(x.id) || state.mergeOrder.includes(x.id) })) };
      return ret({ integrationBranch: 'integration', merges: state.mergeOrder.map((id) => ({ unit: id, status: 'clean', files: [], sha: 'm' })), planSha: state.plan.sha, architectureSha: 'arch1' });
    },
    'review-gate': () => ret({ invalidations: [], notes: [] }),
    replan: (p, o, u, state) => {
      state.plan = { ...state.plan, sha: `replan${++sha}`, units: state.plan.units.map((x) => ({ ...x, complete: state.completed.has(x.id) })) };
      return ret({ planSha: state.plan.sha, amendments: [], overBudget: [], verdicts: verdictsFor(p, 'amended'), directEdits: [], summary: 'no change' });
    },
    ledger: () => ret({ path: 'docs/plans/f-signals.md', sha: 'l' }),
    termination: (p, o, u, state) => ret({ planSha: state.plan.sha, appendedUnits: [], directEdits: [] }),
    'direct-edits': () => ret({ path: 'docs/plans/f-breakdown.md', sha: 'e' }),
    obligations: () => ret({ obligations: [] }),
    'decision-log': () => ret({ path: 'docs/plans/f-decisions.md', sha: 'dec1' }),
    'decision-replan': (p, o, u, state) => ret({ planSha: 'decided1', amendments: [{ kind: 'new-edge', unit: 'B', statement: 'B after A' }], overBudget: [],
      verdicts: [...p.matchAll(/"id": "(d\d+)"/g)].map((m) => ({ id: m[1], verdict: 'amended', statement: 'edge added' })), directEdits: [], summary: 'applied' }),
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
// An implement stage that reports one discovery, so the wave has an item to
// absorb and the re-plan runs; a wave with nothing to absorb skips it.
const discovering = { implement: (p, o, u) => ret(implemented(u, { discoveries: [{ statement: `note from ${u}`, affects: 'B' }] })) };

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

test('clean run: two waves, termination, obligations, complete; silence stops nothing and skips the re-plan', async () => {
  const { rt, calls, labels, logs } = runtime(twoWaves());
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  assert.equal(out.waves, 2);
  const l = labels();
  assert.deepEqual(l.filter((x) => x.startsWith('implement')), ['implement:A', 'implement:B']);
  assert.ok(l.indexOf('merge:w1') < l.indexOf('review-gate:w1'));
  assert.ok(l.indexOf('review-gate:w1') < l.indexOf('plan:B'));
  assert.ok(!l.some((x) => x.startsWith('replan:')), 'nothing to absorb: no re-plan');
  assert.ok(!l.some((x) => /^load:w\d/.test(x)), 'no re-plan, nothing to reload');
  assert.ok(logs.some((m) => /wave 1: nothing to absorb; re-plan skipped, plan @ merged1/.test(m)));
  assert.match(calls.find((c) => c.label === 'plan:B').prompt, /@ merged1/, 'the next wave schedules from the merge\'s SHA');
  assert.equal(out.planSha, 'merged2');
  assert.ok(l.includes('termination') && l.includes('obligations'));
  assert.ok(!l.includes('direct-edits'));
  assert.ok(!l.includes('ledger:w1'), 'nothing to scribe on a wave with no signals');
  assert.deepEqual(out.unscribed, []);
  assert.equal(out.escalation, undefined);
  assert.match(out.digest.note, /not evidence/);
});

test('a wave with an item to absorb re-plans, reloads, and the next wave schedules from the re-plan', async () => {
  const { rt, labels } = runtime(twoWaves(), discovering);
  const out = await run(rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
  const l = labels();
  assert.ok(l.indexOf('review-gate:w1') < l.indexOf('replan:w1'));
  assert.ok(l.indexOf('replan:w1') < l.indexOf('load:w1'));
  assert.ok(l.indexOf('load:w1') < l.indexOf('plan:B'));
  assert.ok(l.includes('ledger:w1'));
});

test('a wave with only records — readings, deviations, findings — skips the re-plan but scribes them', async () => {
  const { rt, labels } = runtime(twoWaves(), {
    plan: (p, o, u) => ret({ branch: `plan-${u}`, sha: 'p', path: `docs/plans/${u}-plan.md`, readings: [{ clause: 'c', reading: 'r' }] }),
    implement: (p, o, u) => ret(implemented(u, { deviations: ['d'] })),
    'lens-review': () => ret({ findings: [{ severity: 'Minor', statement: 'naming', location: 'a.py:3' }], systemic: false, summary: 's' }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
  assert.ok(!labels().some((x) => x.startsWith('replan:')));
  assert.ok(labels().includes('ledger:w1'));
  assert.deepEqual(out.digest.byKind, { reading: 2, deviation: 2, finding: 2 });
  assert.deepEqual(out.digest.attention.minorFindings, { A: 1, B: 1 });
});

test('a stop on a wave with nothing to absorb carries the merge\'s SHA and no pre_replan_sha', async () => {
  const { rt, labels } = runtime(twoParallel(), {
    merge: () => ret({ integrationBranch: 'integration', merges: [
      { unit: 'A', status: 'unresolvable', files: ['src/x.js'] },
      { unit: 'B', status: 'clean', files: [], sha: 'mb' },
    ], planSha: 'm1' }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.equal(out.escalation.stage, 'wave');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['merge-conflict-unresolvable']);
  assert.equal(out.escalation.plan_sha, 'm1');
  assert.equal(out.escalation.pre_replan_sha, undefined);
  assert.ok(!labels().includes('replan:w1'));
  assert.ok(labels().includes('ledger:w1'), 'the stop is scribed');
});

test('the return leads with status and escalation; scribed rows live in the signals file, unscribed ones in the return', async () => {
  const conflict = { merge: () => ret({ integrationBranch: 'integration', merges: [{ unit: 'A', status: 'unresolvable', files: ['src/x.js'] }], planSha: 'm1' }) };
  const scribedRun = await run(runtime(twoWaves(), conflict).rt);
  assert.deepEqual(Object.keys(scribedRun).slice(0, 4), ['status', 'workflow', 'escalation', 'digest']);
  assert.equal(scribedRun.ledgerPath, 'docs/plans/f-signals/plan0.md', 'one ledger file per launch, named by the starting plan SHA');
  assert.ok(!('ledger' in scribedRun));
  assert.deepEqual(scribedRun.unscribed, []);
  assert.equal(JSON.stringify(scribedRun).indexOf('"escalation"'), JSON.stringify(scribedRun).indexOf('"status"') + '"status":"stopped","workflow":"lsf-implement",'.length);

  const nullScribe = runtime(twoWaves(), { ...conflict, ledger: () => null });
  const unscribedRun = await run(nullScribe.rt);
  assert.ok(nullScribe.labels().includes('ledger:w1'));
  assert.deepEqual(unscribedRun.unscribed.map((s) => s.kind), ['merge-conflict-unresolvable']);
});

test('a null scribe does not stop; its rows ride in the next append, or stay in unscribed when none follows', async () => {
  let scribes = 0;
  const { rt, calls } = runtime(twoWaves(), {
    implement: (p, o, u) => ret(implemented(u, { deviations: [`dev-${u}`] })),
    ledger: () => (++scribes === 1 ? null : ret({ path: 'docs/plans/f-signals.md', sha: 'l' })),
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  assert.deepEqual(out.unscribed, []);
  assert.match(calls.find((c) => c.label === 'ledger:w2').prompt, /dev-A[^]*dev-B/, 'wave 1 rows the null scribe left ride in the wave 2 append');

  const never = await run(runtime(twoWaves(), { implement: (p, o, u) => ret(implemented(u, { deviations: [`dev-${u}`] })), ledger: () => null }).rt);
  assert.equal(never.status, 'complete');
  assert.deepEqual(never.unscribed.filter((s) => s.kind === 'deviation').map((s) => s.statement), ['dev-A', 'dev-B']);
});

test('a ledger scribe that returns no commit SHA stops the run at that append, and no later scribe runs', async () => {
  const { rt, labels } = runtime(twoWaves(), {
    implement: (p, o, u) => ret(implemented(u, { deviations: [`dev-${u}`] })),
    ledger: () => ret({ path: 'docs/plans/f-signals.md', sha: ' ', pauseRequested: false }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.equal(out.escalation.stage, 'ledger');
  assert.deepEqual(out.escalation.signals.map((s) => [s.kind, s.stage, s.referent]), [['agent-null', 'ledger', 'docs/plans/f-signals/plan0.md']]);
  assert.match(out.escalation.signals[0].statement, /no commit SHA/);
  assert.ok(!labels().includes('merge:w2'), 'the next wave does not merge over the leftover append');
  assert.deepEqual(labels().filter((l) => l.startsWith('ledger')), ['ledger:w1']);
  assert.deepEqual(out.unscribed.map((s) => s.kind), ['deviation', 'agent-null']);
});

test('a ledger scribe on a stop path that returns no commit SHA joins the escalation', async () => {
  const conflict = { merge: () => ret({ integrationBranch: 'integration', merges: [{ unit: 'A', status: 'unresolvable', files: ['src/x.js'] }], planSha: 'm1' }) };
  const { rt, labels } = runtime(twoWaves(), { ...conflict, ledger: () => ret({ path: 'x', sha: '' }) });
  const out = await run(rt);
  assert.equal(out.escalation.stage, 'wave');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['merge-conflict-unresolvable', 'agent-null']);
  assert.ok(out.escalation.signals.every((s) => s.disposition === 'stopped'));
  assert.deepEqual(labels().filter((l) => l.startsWith('ledger')), ['ledger:w1']);
});

test('a decision-log or direct-edits scribe that returns no commit SHA stops like a null return', async () => {
  const decisions = { decisions: [{ id: 'd1', question: 'q', decision: 'yes' }] };
  const dec = await run(runtime(twoWaves(), { 'decision-log': () => ret({ path: 'docs/plans/f-decisions.md', sha: '' }) }, decisions).rt);
  assert.equal(dec.escalation.stage, 'decision-log');
  assert.deepEqual(dec.escalation.signals.map((s) => [s.kind, s.referent]), [['agent-null', 'docs/plans/f-decisions.md']]);

  const edits = await run(runtime(twoWaves(), {
    termination: (p, o, u, s) => ret({ planSha: s.plan.sha, appendedUnits: [], directEdits: [{ path: 'README.md', description: 'mention A' }] }),
    'direct-edits': () => ret({ path: 'README.md', sha: '' }),
  }).rt);
  assert.equal(edits.escalation.stage, 'direct-edits');
  assert.deepEqual(edits.escalation.signals.map((s) => [s.kind, s.referent]), [['agent-null', 'README.md']]);
});

test('the re-planned SHA appears in the next wave\'s prompts', async () => {
  const { rt, calls } = runtime(twoWaves(), discovering);
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
  for (const l of ['plan:A', 'critique-plan:r1:A', 'implement:A', 'critique-code:r1:A', 'lens-review:r1:A']) {
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

test('the merge archives each completed entry behind a stub and writes the compact completion entry; the re-plan writes the compact maintenance entry and archives closed risks', async () => {
  const { rt, calls } = runtime(twoWaves(), discovering, { archivePath: 'docs/plans/f-old.md' });
  await run(rt);
  const merge = calls.find((c) => c.label === 'merge:w1').prompt;
  assert.match(merge, /Move its `### <unit id>` entry under `## Work Units` in docs\/plans\/f-breakdown\.md to docs\/plans\/f-old\.md/);
  assert.match(merge, /stub `### <unit id> — complete at <merge SHA>; entry archived to docs\/plans\/f-old\.md; report <completion report path>`/);
  assert.match(merge, /### <date, date -I> — <unit id> complete` with the single line `Report: <completion report path> @ <merge SHA>\. Ledger: docs\/plans\/f-signals\/plan0\.md wave 1\.`/);
  assert.match(merge, /no restated report content/);
  const rp = calls.find((c) => c.label === 'replan:w1').prompt;
  assert.match(rp, /### <date, date -I> — re-plan wave 1 \(<item ids>\)/);
  assert.match(rp, /<kind> <unit> — <one clause>/);
  assert.match(rp, /moves to docs\/plans\/f-old\.md under `## Closed risks`/);
  assert.match(rp, /Parallel Streams, Critical Path, and Dependency Graph sections are not maintained/);
  assert.doesNotMatch(rp, /record every amendment in Plan Maintenance/);
  const dflt = runtime(twoWaves());
  const out = await run(dflt.rt);
  assert.match(dflt.calls.find((c) => c.label === 'merge:w1').prompt, /entry archived to docs\/plans\/f-archive\.md/, 'default archive path');
  assert.equal(out.ledgerPath, 'docs/plans/f-signals/plan0.md');
});

test('unit stages extract their entry and obligations by command; the plan and report carry size budgets; critique rounds read the plan by path', async () => {
  let round = 0;
  const { rt, calls } = runtime(twoWaves(), {
    'critique-plan': () => (++round === 1
      ? ret({ findings: [{ severity: 'Critical', statement: 'bad', location: 'plan' }], systemic: false, summary: 's' })
      : NO_FINDINGS),
  });
  await run(rt);
  // The range ends at any ##-prefixed heading, so the last entry does not run on into `## Graph`.
  const entry = "git show plan0:docs/plans/f-breakdown.md | sed -n '/^### A$/,/^##/p'";
  const obligations = "git show plan0:docs/plans/f-breakdown.md | sed -n '/^## Standing Obligations/,/^## /p' | grep -n 'A'";
  for (const l of ['plan:A', 'critique-plan:r1:A', 'fix-plan:r1:A', 'implement:A', 'critique-code:r1:A', 'lens-review:r1:A']) {
    const p = calls.find((c) => c.label === l).prompt;
    assert.ok(p.includes(entry), `${l}: entry command`);
    assert.ok(p.includes(obligations), `${l}: obligations command`);
    assert.doesNotMatch(p, /Read it first \(`git show plan0:docs\/plans\/f-breakdown\.md`\)/, `${l}: no whole-file read`);
  }
  const plan = calls.find((c) => c.label === 'plan:A').prompt;
  assert.match(plan, /about 12 KB is the target/);
  assert.match(plan, /in one call and commit it\. Do not trim it to length afterwards/);
  assert.match(plan, /at most a Minor finding/);
  assert.doesNotMatch(plan, /at or under/);
  assert.match(plan, /A clause with one plain reading is not listed/);
  assert.match(plan, /\*\*Goal\*\* — one sentence/);
  assert.match(plan, /travel with every stage prompt; cite them, do not copy them/);
  assert.match(plan, /\*\*Context\*\* — the architecture sections relied on, by heading\.\n/);
  assert.doesNotMatch(plan, /by their text|self-contained/);
  assert.match(plan, /not the plan text/);
  const impl = calls.find((c) => c.label === 'implement:A').prompt;
  assert.match(impl, /Run the project's gates[^]*once at your final HEAD and report each under `gates`/);
  for (const l of ['critique-code:r1:A', 'lens-review:r1:A']) {
    const p = calls.find((c) => c.label === l).prompt;
    assert.match(p, /ran the project's gates at this branch's HEAD:\n- `pytest -q` — green: 412 passed\n- `ruff check \.` — green\n/, l);
    assert.match(p, /Do not run the whole suite, the project's CI job, or a lint pass; run a single targeted test only/, l);
    assert.doesNotMatch(p, /If no gate is listed/, l);
  }
  assert.match(calls.find((c) => c.label === 'critique-code:r1:A').prompt, /from the code and the gate results above/);
  const r1 = calls.find((c) => c.label === 'critique-plan:r1:A').prompt;
  const r2 = calls.find((c) => c.label === 'critique-plan:r2:A').prompt;
  assert.match(r1, /git show plan-A:docs\/plans\/A-plan\.md/);
  assert.match(r2, /git show fixed-A:docs\/plans\/A-plan\.md/, 'round 2 reads the fixed branch');
  assert.doesNotMatch(r1, /--- implementation plan/);
  const fix = calls.find((c) => c.label === 'fix-plan:r1:A').prompt;
  assert.doesNotMatch(fix, /12 KB/);
  assert.match(fix, /do not shorten the plan to length/);
  assert.doesNotMatch(fix, /planText/);
  const report = calls.find((c) => c.label === 'completion-report:A');
  assert.match(report.prompt, /Copy stage\. Write the Markdown between the markers below to docs\/plans\/A-completion\.md, every character unchanged/);
  assert.match(report.prompt, /<<<REPORT\n# A — completion report\n[^]*REPORT>>>/, 'the rendered report travels in the prompt');
  assert.match(report.prompt, /subject line "docs\(f\): A completion report"/);
  assert.doesNotMatch(report.prompt, /Assemble it|phronithm:feature|KB/);
  assert.deepEqual([report.opts.model, report.opts.effort], ['haiku', 'low'], 'a copy stage, at the mechanical tier');
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
  assert.doesNotMatch(w1, /Other records this wave/);
});

test('an implement return with a gate not run stops before review, naming the gate, and keeps its records', async () => {
  const { rt, labels, calls } = runtime(twoWaves(), {
    implement: (p, o, u) => ret(implemented(u, {
      gates: [{ command: 'pytest -q', status: 'green', summary: '412 passed' }, { command: 'xtask ci', status: 'not-run', summary: 'outlived the tool timeout' }],
      discoveries: [{ statement: 'note from the stopped unit', affects: 'B' }],
    })),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  const stops = out.escalation.signals;
  assert.deepEqual(stops.map((s) => [s.kind, s.stage, s.source]), [['gate-not-run', 'implement', 'structural']]);
  assert.match(stops[0].statement, /gate\(s\) not run: xtask ci$/);
  assert.equal(stops[0].referent, 'xtask ci');
  assert.ok(!labels().some((x) => x.startsWith('critique-code') || x.startsWith('lens-review')));
  assert.match(calls.find((c) => c.label === 'ledger:w1').prompt, /note from the stopped unit/, 'the stopped unit keeps its discovery record');
});

test('an implement return with no gate reported stops before review as a gate not run', async () => {
  const { rt, labels } = runtime(twoWaves(), { implement: (p, o, u) => ret(implemented(u, { gates: [] })) });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['gate-not-run']);
  assert.match(out.escalation.signals[0].statement, /\(no gate reported\)/);
  assert.ok(!labels().some((x) => x.startsWith('critique-code') || x.startsWith('lens-review')));
});

test('a red gate is a result, not a gate not run: the unit reaches review', async () => {
  const { rt, labels } = runtime(twoWaves(), { implement: (p, o, u) => ret(implemented(u, { gates: [{ command: 'pytest -q', status: 'red', summary: '2 failed' }] })) });
  const out = await run(rt);
  assert.ok(!(out.escalation?.signals ?? []).some((s) => s.kind === 'gate-not-run'));
  assert.ok(labels().includes('critique-code:r1:A'));
});

test('a design unit with no gate reported is not stopped: its review reads no gates', async () => {
  const plan = graph('plan0', [unit('D', { type: 'design' })]);
  const { rt, labels } = runtime(plan, { implement: (p, o, u) => ret(implemented(u, { gates: [] })) });
  const out = await run(rt);
  assert.ok(!(out.escalation?.signals ?? []).some((s) => s.kind === 'gate-not-run'));
  assert.ok(labels().includes('critique-design:r1:D'));
});

// A review gate whose round 1 raises one Critical and passes afterwards.
const criticalOnce = () => {
  let round = 0;
  return () => (++round === 1 ? ret({ findings: [{ severity: 'Critical', statement: 'bad', location: 'a.py' }], systemic: false, summary: 's' }) : NO_FINDINGS);
};

test('a review round after a code fix reads the fix stage\'s gate runs, not the implement stage\'s', async () => {
  const { rt, calls } = runtime(graph('plan0', [unit('A')]), {
    'critique-code': criticalOnce(),
    'fix-code': (p, o, u) => ret({ branch: `fixed-code-${u}`, sha: 'f', applied: [{ finding: 'bad', fix: 'ok' }], notApplied: [], gates: [{ command: 'pytest -q', status: 'green', summary: '99 passed (fix)' }] }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  const r1 = calls.find((c) => c.label === 'critique-code:r1:A').prompt;
  assert.match(r1, /The implement stage ran the project's gates at this branch's HEAD:\n- `pytest -q` — green: 412 passed/);
  for (const label of ['critique-code:r2:A', 'lens-review:r2:A']) {
    const p = calls.find((c) => c.label === label).prompt;
    assert.match(p, /The round-1 fix stage ran the project's gates at this branch's HEAD:\n- `pytest -q` — green: 99 passed \(fix\)\n/, label);
    assert.doesNotMatch(p, /412 passed/, label);
  }
  const fix = calls.find((c) => c.label === 'fix-code:r1:A');
  assert.match(fix.prompt, /every gate the implement stage ran[^]*report each under `gates`/);
  assert.ok(fix.opts.schema.properties.result.required.includes('gates'), 'a code fix must return its gate runs');
  assert.match(calls.find((c) => c.label === 'completion-report:A').prompt, /Gates at HEAD:\n\n- `pytest -q` — green: 99 passed \(fix\)/);
});

test('a code fix whose gate did not run, or that reports none, stops before the next review round', async () => {
  for (const gates of [[{ command: 'xtask ci', status: 'not-run', summary: 'timed out' }], []]) {
    const { rt, labels } = runtime(graph('plan0', [unit('A')]), {
      'critique-code': criticalOnce(),
      'fix-code': (p, o, u) => ret({ branch: `fixed-code-${u}`, sha: 'f', applied: [{ finding: 'bad', fix: 'ok' }], notApplied: [], gates }),
    });
    const out = await run(rt);
    assert.equal(out.status, 'stopped');
    const stop = out.escalation.signals.find((s) => s.kind === 'gate-not-run');
    assert.equal(stop.stage, 'fix');
    assert.match(stop.statement, gates.length ? /xtask ci/ : /\(no gate reported\)/);
    assert.ok(!labels().includes('critique-code:r2:A'), 'no round-2 review');
  }
});

test('the implement and code-fix prompts say how to wait out a long gate; the plan fix is not told', async () => {
  let round = 0;
  const { rt, calls } = runtime(graph('plan0', [unit('A')]), {
    'critique-plan': () => (++round === 1 ? ret({ findings: [{ severity: 'Critical', statement: 'bad', location: 'plan' }], systemic: false, summary: 's' }) : NO_FINDINGS),
    'critique-code': criticalOnce(),
    'fix-code': (p, o, u) => ret({ branch: `fixed-code-${u}`, sha: 'f', applied: [{ finding: 'bad', fix: 'ok' }], notApplied: [], gates: [{ command: 'pytest -q', status: 'green', summary: '1 passed' }] }),
  });
  await run(rt);
  const prompt = (label) => calls.find((c) => c.label === label).prompt;
  for (const label of ['implement:A', 'fix-code:r1:A']) {
    const p = prompt(label);
    assert.match(p, /ending your turn while it runs returns this stage with the gate unrun/, label);
    assert.match(p, /D=\$\(git rev-parse --absolute-git-dir\); rm -f "\$D"\/lsf-gate-<n>\.\*; \(<command> > "\$D\/lsf-gate-<n>\.log" 2>&1; echo \$\? > "\$D\/lsf-gate-<n>\.rc"\) &/, label);
    // The poll's outer double quotes let the shell expand $D before sh -c runs.
    assert.match(p, /timeout 540 sh -c "until \[ -f '\$D\/lsf-gate-<n>\.rc' \]; do sleep 15; done"/, label);
    assert.match(p, /Bash tool's timeout set to 600000 ms/, label);
    assert.match(p, /Report a gate `not-run`, with why in its summary, only when it could not be completed/, label);
  }
  assert.doesNotMatch(prompt('fix-plan:r1:A'), /lsf-gate|not-run/);
});

test('a design unit\'s fix keeps the plain fix schema and is not asked for gates', async () => {
  let round = 0;
  const { rt, calls } = runtime(graph('plan0', [unit('D', { type: 'design' })]), {
    'critique-design': () => (++round === 1 ? ret({ findings: [{ severity: 'Critical', statement: 'bad', location: 'd.md' }], systemic: false, summary: 's' }) : NO_FINDINGS),
    'fix-code': (p, o, u) => ret({ branch: `fixed-code-${u}`, sha: 'f', applied: [{ finding: 'bad', fix: 'ok' }], notApplied: [] }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  const fix = calls.find((c) => c.label === 'fix-code:r1:D');
  assert.ok(!('gates' in fix.opts.schema.properties.result.properties));
  assert.doesNotMatch(fix.prompt, /report each under `gates`/);
});

test('renderCompletionReport renders the implement and review returns section by section, with finding counts pointing at the ledger', () => {
  const ctx = { ledgerPath: 'docs/plans/f-signals/plan0.md' };
  const unit = { id: 'A', type: 'feature' };
  const impl = implemented('A', {
    gateProbe: [{ name: 'fontforge', pin: '20230101', observed: '20230101', drifted: false }],
    deviations: ['sibling predicate instead of widening'], writesOutsideScope: ['CLAUDE.md', 'src/x.py'],
    deferrals: [{ statement: 'export left unrotated', belongsTo: 'B' }],
    discoveries: [{ statement: 'B needs an edge to C', affects: 'B' }],
    doneCondition: { evaluable: true, met: true, evidence: 'clause (i): test_a green; clause (ii): 3 of 3 fixtures.' },
    standingObligations: [{ statement: 'the diff of a regenerated artefact is read', status: 'discharged' }],
    summary: 'Widened the predicate and pinned it.',
  });
  const review = { rounds: 2, findings: [
    { severity: 'Significant', statement: 'fragile', location: 'a.py:2', stage: 'critique-code', round: 1 },
    { severity: 'Minor', statement: 'name', location: 'a.py:9' }, { severity: 'Minor', statement: 'doc', location: 'a.py:12' },
  ] };
  const text = renderCompletionReport(ctx, unit, 1, 'impl-A', impl, review);
  assert.equal(text, [
    '# A — completion report', '',
    'Unit `A` (feature), wave 1, branch `impl-A`.', '',
    'Widened the predicate and pinned it.', '',
    '## Done-condition verdict', '',
    '**Met** (implement stage). clause (i): test_a green; clause (ii): 3 of 3 fixtures.', '',
    'External gates re-verified: fontforge pinned 20230101, observed 20230101.', '',
    'Gates at HEAD:', '',
    '- `pytest -q` — green: 412 passed\n- `ruff check .` — green', '',
    '## Deviations and writes outside Scope', '',
    '- sibling predicate instead of widening', '',
    'Written outside the Scope line: `CLAUDE.md`, `src/x.py`.', '',
    '## Deferrals', '',
    '- export left unrotated → `B`', '',
    '## Discoveries', '',
    '- B needs an edge to C — affects `B`', '',
    '## Standing obligations', '',
    '- the diff of a regenerated artefact is read — discharged', '',
    'Review: 2 round(s); 1 Significant and 2 Minor finding(s) left unfixed are recorded in docs/plans/f-signals/plan0.md, wave 1.', '',
  ].join('\n'));
  assert.doesNotMatch(text, /fragile|a\.py/, 'findings are counts; the rows live in the ledger');
  const bare = renderCompletionReport(ctx, unit, 2, 'impl-A', implemented('A', { doneCondition: { evaluable: false, met: false, evidence: 'the clause names a file that does not exist' } }), { rounds: 1, findings: [] });
  assert.match(bare, /\*\*Not met\*\* \(implement stage\) \(not evaluable as written\)\. the clause names a file that does not exist/);
  assert.equal((bare.match(/^None\.$/gm) ?? []).length, 4, 'every empty section says so');
  assert.doesNotMatch(bare, /External gates|Written outside/);
  assert.doesNotMatch(renderCompletionReport(ctx, unit, 2, 'impl-A', implemented('A', { gates: [] }), { rounds: 1, findings: [] }), /Gates at HEAD/);
  assert.doesNotMatch(bare, /breakdown|re-plan mode/, 'no serial-mode footer');
  const afterFix = renderCompletionReport(ctx, unit, 1, 'impl-A', impl, { rounds: 2, findings: [], gates: [{ command: 'pytest -q', status: 'green', summary: '99 passed (fix)' }] });
  assert.match(afterFix, /Gates at HEAD:\n\n- `pytest -q` — green: 99 passed \(fix\)\n/, 'after a fix, the fix\'s runs are the gates at HEAD');
  assert.doesNotMatch(afterFix, /412 passed/);
});

test('a plan or report past twice its size target is a document-oversize ledger row, never a stop or a re-plan item', async () => {
  const { rt, calls } = runtime(twoWaves(), {
    plan: (p, o, u) => ret({ branch: `plan-${u}`, sha: 'p', path: `docs/plans/${u}-plan.md`, bytes: u === 'A' ? 30 * 1024 : 20 * 1024, readings: [] }),
    implement: (p, o, u) => ret(implemented(u, u === 'B' ? { summary: 'x'.repeat(17 * 1024) } : {})),
    ledger: () => null,
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
  const rows = out.unscribed.filter((s) => s.kind === 'document-oversize');
  assert.deepEqual(rows.map((s) => [s.unit, s.stage, s.statement]), [
    ['A', 'plan', 'docs/plans/A-plan.md is 30 KB against a 12 KB target'],
    ['B', 'completion-report', 'docs/plans/B-completion.md is 17 KB against a 8 KB target'],
  ]);
  assert.ok(rows.every((s) => s.disposition === 'recorded'));
  for (const w of ['replan:w1', 'replan:w2']) {
    const c = calls.find((c) => c.label === w);
    if (c) assert.doesNotMatch(c.prompt, /document-oversize/, `${w}: not a re-plan item`);
  }
});

test('deferrals, discoveries, and distress reach the re-plan with ids; readings, deviations, and findings go to the ledger only', async () => {
  const { rt, calls } = runtime(twoWaves(), {
    plan: (p, o, u) => ret({ branch: `plan-${u}`, sha: 'p', path: `docs/plans/${u}-plan.md`,
      readings: [{ clause: 'Scope: tests/test_missing.py', reading: 'tests/test_present.py' }] }),
    implement: (p, o, u) => ret(implemented(u, {
      deviations: ['sibling predicate instead of widening'], writesOutsideScope: ['CLAUDE.md'],
      deferrals: [{ statement: 'export left unrotated', belongsTo: 'B' }],
      discoveries: [{ statement: 'B needs an edge to C', affects: 'B' }],
      distress: { ...NO_DISTRESS, midImplementationDesignDecisions: 3 },
    })),
    'critique-code': () => ret({ findings: [{ severity: 'Significant', statement: 'fragile', location: 'a.py:2' }], systemic: false, summary: 's' }),
    ledger: () => null,
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
  const w1 = calls.find((c) => c.label === 'replan:w1').prompt;
  const items = w1.split('Items this wave')[1].split('Merge conflicts')[0];
  for (const k of ['"kind": "deferral"', '"kind": "discovery"', '"kind": "execution-distress"']) assert.match(items, new RegExp(k));
  assert.deepEqual(itemIds(w1), ['rp-1', 'rp-2', 'rp-3']);
  for (const k of ['"kind": "reading"', '"kind": "deviation"', '"kind": "finding"', 'Other records', 'test_present', 'CLAUDE.md', 'fragile']) assert.doesNotMatch(w1, new RegExp(k));
  const w1rows = out.unscribed.filter((s) => s.wave === 1 && s.unit === 'A');
  assert.deepEqual([...new Set(w1rows.map((s) => s.kind))].sort(), ['deferral', 'deviation', 'discovery', 'execution-distress', 'finding', 'reading', 'verdict']);
  assert.match(w1rows.find((s) => s.kind === 'deviation' && /outside/.test(s.statement)).statement, /writes outside the Scope line: CLAUDE\.md/);
  assert.equal(w1rows.find((s) => s.kind === 'reading').statement, '1 reading(s); see docs/plans/A-plan.md Readings');
  const verdicts = w1rows.filter((s) => s.kind === 'verdict').map((s) => s.statement);
  assert.deepEqual(verdicts, ['rp-1 amended: ok', 'rp-2 amended: ok', 'rp-3 amended: ok'], 'a verdict row carries the item id, not its text');
  assert.ok(out.unscribed.every((s) => s.disposition === 'recorded'));
  assert.deepEqual(out.digest.attention.unfixedSignificant, { A: 1, B: 1 });
  assert.deepEqual(out.digest.attention.distressedUnits, ['A', 'B']);
});

test('an item the re-plan cannot absorb, or does not answer, stops after the re-plan', async () => {
  for (const mode of ['cannot', 'missing']) {
    const { rt, labels } = runtime(twoWaves(), {
      implement: (p, o, u) => ret(implemented(u, { discoveries: [{ statement: 'B needs C first', affects: 'B' }, { statement: 'C is redundant', affects: 'C' }] })),
      replan: (p, o, u, state) => ret({ planSha: 'replanned-stop', amendments: [], overBudget: [],
        verdicts: mode === 'cannot' ? [{ id: 'rp-1', verdict: 'cannot', statement: 'needs the requester' }, { id: 'rp-2', verdict: 'amended', statement: 'dropped' }] : [], summary: 's' }),
      ledger: () => null,
    });
    const out = await run(rt);
    assert.equal(out.status, 'stopped', mode);
    assert.equal(out.escalation.stage, 'replan');
    const stops = out.unscribed.filter((s) => s.disposition === 'stopped');
    assert.deepEqual(stops.map((s) => s.kind), mode === 'cannot' ? ['plan-defect-unabsorbed'] : ['plan-defect-unabsorbed', 'plan-defect-unabsorbed'], mode);
    assert.equal(stops[0].referent, 'B');
    assert.match(stops[0].statement, mode === 'cannot' ? /cannot absorb: needs the requester — \[implement\] discovery: B needs C first/ : /no verdict — \[implement\] discovery: B needs C first/);
    assert.ok(labels().includes('replan:w1') && !labels().includes('plan:B'));
    assert.equal(out.escalation.plan_sha, 'replanned-stop', 'relaunch from the re-plan commit');
    assert.equal(out.escalation.pre_replan_sha, 'merged1');
    if (mode === 'cannot') assert.equal(out.unscribed.filter((s) => s.kind === 'verdict').length, 1);
  }
});

test('a flagged over-budget unit is recorded and the run continues; an unrepaired one stops', async () => {
  for (const disposition of ['flagged', 'unrepaired']) {
    const { rt } = runtime(twoWaves(), {
      ...discovering,
      replan: (p, o, u, state) => {
        state.plan = { ...state.plan, sha: 'replanned', units: state.plan.units.map((x) => ({ ...x, complete: state.completed.has(x.id) })) };
        return ret({ planSha: 'replanned', amendments: [], overBudget: [{ unit: 'B', disposition, statement: 'three written paths' }], verdicts: verdictsFor(p, 'amended'), summary: 's' });
      },
      ledger: () => null,
    });
    const out = await run(rt);
    if (disposition === 'flagged') {
      assert.equal(out.status, 'complete', 'flag discharges the budget check');
      const flags = out.unscribed.filter((s) => s.kind === 'over-budget-flagged');
      assert.equal(flags.length, 1, 'reported each re-plan, recorded once while it stands');
      assert.equal(flags[0].disposition, 'recorded');
      assert.equal(flags[0].unit, 'B');
      assert.equal(flags[0].wave, 1);
    } else {
      assert.equal(out.status, 'stopped');
      assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['unit-over-budget']);
    }
  }
});

test('an over-budget unit is recorded again when its disposition changes, and an unrepaired one is never deduplicated', async () => {
  let pass = 0;
  const { rt } = runtime(twoWaves(), {
    ...discovering,
    replan: (p, o, u, state) => {
      state.plan = { ...state.plan, sha: 'replanned', units: state.plan.units.map((x) => ({ ...x, complete: state.completed.has(x.id) })) };
      const disposition = ++pass === 1 ? 'flagged' : 'extracted';
      return ret({ planSha: 'replanned', amendments: [], overBudget: [{ unit: 'B', disposition, statement: 's' }], verdicts: verdictsFor(p, 'amended'), summary: 's' });
    },
    ledger: () => null,
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  assert.deepEqual(out.unscribed.filter((s) => /over-budget/.test(s.kind)).map((s) => [s.wave, s.kind]), [[1, 'over-budget-flagged'], [2, 'over-budget-repaired']]);
});

test('a document-text verdict records the architecture amendment and does not stop', async () => {
  const { rt } = runtime(twoWaves(), {
    'review-gate': () => ret({ invalidations: [], notes: [{ statement: 'U5 row is stale', referent: '§6 U5' }] }),
    replan: (p, o, u, state) => {
      state.plan = { ...state.plan, sha: `rp-${state.completed.size}`, units: state.plan.units.map((x) => ({ ...x, complete: state.completed.has(x.id) })) };
      return ret({ planSha: state.plan.sha, amendments: [], overBudget: [],
        verdicts: [...p.matchAll(/"id": "(rp-\d+)"/g)].map((m) => ({ id: m[1], verdict: 'document-text', statement: '§6 row U5 now reads resolved' })), summary: 's' });
    },
    ledger: () => null,
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  const amendments = out.unscribed.filter((s) => s.kind === 'architecture-amendment');
  assert.equal(amendments.length, 2, 'one per wave');
  assert.equal(amendments[0].referent, '§6 U5');
  assert.equal(amendments[0].disposition, 'recorded');
  assert.deepEqual(out.digest.attention.architectureAmendments, ['§6 row U5 now reads resolved', '§6 row U5 now reads resolved']);
  assert.match(out.unscribed.find((s) => s.kind === 'verdict').statement, /rp-1 document-text: §6 row U5 now reads resolved/);
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
    'fix-plan': (p, o, u) => ret({ branch: `fixed-${u}`, sha: 'f', applied: [], notApplied: [{ finding: 'bad', reason: 'changes the entry' }] }),
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

test('a wave stop with a unit merged and an item to absorb still re-plans over the merged report, then stops from the re-plan SHA', async () => {
  const { rt, calls, labels } = runtime(twoParallel(), {
    ...discovering,
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
  const a = runtime(twoWaves(), { load: (p, o, u, s) => copied(s.plan, { headBranch: 'main' }), ledger: () => null });
  const outA = await run(a.rt);
  assert.equal(outA.status, 'stopped');
  assert.equal(outA.escalation.stage, 'load');
  assert.deepEqual(outA.escalation.signals.map((s) => s.kind), ['precondition']);
  assert.ok(a.labels().includes('ledger:w0'), 'stop at load is scribed');
  assert.ok(outA.unscribed.every((s) => s.disposition === 'stopped'));

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
  const { rt, calls, labels } = runtime(graph('plan0', [unit('A')]), {
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
  assert.match(calls.find((c) => c.label === 'direct-edits').prompt, /termination stage[^]*README\.md[^]*mention M/);
  assert.match(calls.find((c) => c.label === 'ledger:w2').prompt, /\| termination \| reported \| direct-edit \| mention M \| README\.md \| recorded \|/);
  assert.deepEqual(out.digest.attention.directEdits, ['README.md — mention M']);
});

test('a re-plan\'s direct edits are made by a scribe before the load, recorded, and listed in the digest', async () => {
  const edit = { path: 'src/A/README.md', description: 'note the widened signature' };
  const { rt, calls, labels } = runtime(twoWaves(), {
    ...discovering,
    replan: (p, o, u, state) => {
      state.plan = { ...state.plan, sha: `replan-${state.completed.size}`, units: state.plan.units.map((x) => ({ ...x, complete: state.completed.has(x.id) })) };
      return ret({ planSha: state.plan.sha, amendments: [], overBudget: [], verdicts: verdictsFor(p, 'amended'), directEdits: state.completed.size === 1 ? [edit] : [], summary: 's' });
    },
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
  const l = labels();
  assert.ok(l.indexOf('replan:w1') < l.indexOf('direct-edits:w1'));
  assert.ok(l.indexOf('direct-edits:w1') < l.indexOf('load:w1'));
  assert.ok(l.indexOf('load:w1') < l.indexOf('plan:B'));
  assert.ok(!l.includes('direct-edits:w2'));
  const de = calls.find((c) => c.label === 'direct-edits:w1');
  assert.match(de.prompt, /wave 1 re-plan/);
  assert.match(de.prompt, /src\/A\/README\.md[^]*note the widened signature/);
  assert.match(de.prompt, /@ replan-1/, 'the header names the re-planned SHA');
  assert.equal(de.opts.phase, 'Re-plan');
  assert.match(calls.find((c) => c.label === 'ledger:w1').prompt, /\| 1 \| — \| replan \| reported \| direct-edit \| note the widened signature \| src\/A\/README\.md \| recorded \|/);
  assert.deepEqual(out.digest.attention.directEdits, ['src/A/README.md — note the widened signature']);
  assert.match(calls.find((c) => c.label === 'replan:w1').prompt, /`directEdits`[^]*not made by the re-plan|not made by the re-plan[^]*`directEdits`/);
});

test('a null direct-edits return after a re-plan is a stop; a stopping wave still makes its edits', async () => {
  const withEdit = (p) => ret({ planSha: 'replan-x', amendments: [], overBudget: [], verdicts: verdictsFor(p, 'amended'), directEdits: [{ path: 'docs/guide.md', description: 'g' }], summary: 's' });
  const out = await run(runtime(twoWaves(), { ...discovering, replan: withEdit, 'direct-edits': () => null }).rt);
  assert.equal(out.status, 'stopped');
  assert.equal(out.escalation.stage, 'replan');
  assert.deepEqual(out.escalation.signals.map((s) => [s.kind, s.stage, s.wave]), [['agent-null', 'direct-edits', 1]]);

  const stopping = runtime(twoParallel(), {
    ...discovering,
    replan: withEdit,
    merge: () => ret({ integrationBranch: 'integration', merges: [
      { unit: 'A', status: 'unresolvable', files: ['src/x.js'] },
      { unit: 'B', status: 'clean', files: [], sha: 'mb' },
    ], planSha: 'm1' }),
  });
  const outS = await run(stopping.rt);
  assert.equal(outS.status, 'stopped');
  assert.deepEqual(outS.escalation.signals.map((s) => s.kind), ['merge-conflict-unresolvable']);
  assert.ok(stopping.labels().includes('direct-edits:w1'), 'the edits are made before the stop');
  assert.match(stopping.calls.find((c) => c.label === 'ledger:w1').prompt, /direct-edit \| g \| docs\/guide\.md \| recorded/);
});

test('a decision re-plan\'s direct edits run before load at wave 0, and a null return stops there', async () => {
  const decisionsBlock = ['```yaml lsf-decisions', 'decisions:', '  - id: d1', '    date: 2026-09-19', '    question: q', '    decision: file it', '```'].join('\n');
  const decided = (directEdits) => () => ret({ planSha: 'decided1', amendments: [], overBudget: [], verdicts: [{ id: 'd1', verdict: 'amended', statement: 'row filed' }], directEdits, summary: 's' });
  const decisions = [{ id: 'd1', question: 'q', decision: 'file it' }];
  const { rt, calls, labels } = runtime(twoWaves(), {
    load: (p, o, u, st) => copied(st.plan, { decisionsBlock }),
    'decision-replan': decided([{ path: 'docs/debt.md', description: 'file the D7 row' }]),
    ledger: () => null,
  }, { decisions });
  const out = await run(rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
  assert.deepEqual(labels().slice(0, 4), ['decision-log', 'decision-replan', 'direct-edits:w0', 'ledger:w0']);
  assert.match(calls[1].prompt, /`directEdits`[^]*not made by the re-plan|not made by the re-plan[^]*`directEdits`/);
  assert.match(calls[2].prompt, /decision re-plan[^]*docs\/debt\.md[^]*file the D7 row/);
  assert.deepEqual(out.unscribed.filter((s) => s.wave === 0).map((s) => [s.stage, s.kind, s.referent]), [['decision-replan', 'verdict', 'd1'], ['decision-replan', 'direct-edit', 'docs/debt.md']]);

  const nul = await run(runtime(twoWaves(), { 'decision-replan': decided([{ path: 'docs/debt.md', description: 'd' }]), 'direct-edits': () => null }, { decisions }).rt);
  assert.equal(nul.status, 'stopped');
  assert.equal(nul.escalation.stage, 'decision-replan');
  assert.deepEqual(nul.escalation.signals.map((s) => [s.kind, s.stage, s.wave]), [['agent-null', 'direct-edits', 0]]);
});

test('an undischarged standing obligation at termination stops', async () => {
  const { rt } = runtime(graph('plan0', [unit('A')]), {
    obligations: () => ret({ obligations: [{ statement: 'perf numbers collected', disposition: 'undischarged', carrier: '', evidence: 'none' }] }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.equal(out.escalation.stage, 'obligations');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['standing-obligation-undischarged']);
});

test('an obligation carried by its own terms is a record with its carrier, and a stop without one', async () => {
  const obligation = (carrier) => ({ statement: 'perf numbers carried until a later stream reads them', disposition: 'carried', carrier, evidence: 'D12 says a later stream reads them' });
  const carried = runtime(graph('plan0', [unit('A')]), { obligations: () => ret({ obligations: [obligation('docs/debt.md row 7')] }) });
  const out = await run(carried.rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
  assert.equal(out.digest.byKind['standing-obligation-carried'], 1);
  const scribe = carried.calls.find((c) => c.label === 'ledger:w1').prompt;
  assert.match(scribe, /append termination signal ledger entries/);
  assert.match(scribe, /<<<LEDGER\n## Termination\n/);
  assert.match(scribe, /\| obligations \| reported \| standing-obligation-carried \| perf numbers carried until a later stream reads them \| docs\/debt\.md row 7 \| recorded \|/);
  assert.match(carried.calls.find((c) => c.label === 'obligations').prompt, /recorded nowhere outside this plan is `undischarged`/);

  const uncarried = await run(runtime(graph('plan0', [unit('A')]), { obligations: () => ret({ obligations: [obligation('')] }) }).rt);
  assert.equal(uncarried.status, 'stopped');
  assert.deepEqual(uncarried.escalation.signals.map((s) => s.kind), ['standing-obligation-undischarged']);
  assert.match(uncarried.escalation.signals[0].statement, /^carried without a carrier: perf numbers/);
});

test('every main-checkout stage that commits names its own files and is barred from sweeping or discarding anyone else\'s', async () => {
  const decisionsBlock = ['```yaml lsf-decisions', 'decisions:', '  - id: d1', '    date: 2026-09-24', '    question: q', '    decision: yes', '```'].join('\n');
  let terminated = false;
  const { rt, calls } = runtime(twoWaves(), {
    ...discovering,
    load: (p, o, u, st) => copied(st.plan, { decisionsBlock }),
    termination: (p, o, u, s) => {
      const edits = terminated ? [] : [{ path: 'README.md', description: 'mention A' }];
      terminated = true;
      return ret({ planSha: s.plan.sha, appendedUnits: [], directEdits: edits });
    },
  }, { decisions: [{ id: 'd1', question: 'q', decision: 'yes' }] });
  await run(rt);
  const committing = ['decision-log', 'decision-replan', 'merge', 'replan', 'ledger', 'termination', 'direct-edits'];
  const seen = new Set();
  for (const c of calls) {
    const prefix = c.label.split(':')[0];
    if (!committing.includes(prefix)) continue;
    seen.add(prefix);
    assert.match(c.prompt, /git add -- <paths>`, then `git commit -m <message> -- <paths>`, options before the `--`/, c.label);
    assert.match(c.prompt, /never retry it without the pathspec/, c.label);
    assert.match(c.prompt, /Never run `git add -A`, `git add \.`, `git add -u`, `git commit -a`, `git stash`, `git clean` or `git reset --hard` in the main checkout/, c.label);
  }
  assert.deepEqual([...seen].sort(), [...committing].sort());
  const at = (label) => calls.find((c) => c.label.startsWith(label)).prompt;
  assert.match(at('decision-log'), /Commit only the files this stage wrote \(docs\/plans\/f-decisions\.md\)/);
  assert.match(at('ledger'), /Commit only the files this stage wrote \(docs\/plans\/f-signals\/plan0\.md\)/);
  assert.match(at('replan'), /Commit only the files this stage wrote \(docs\/plans\/f-breakdown\.md, and docs\/architecture\/f\.md if you made a `document-text` correction\)/);
  const merge = at('merge');
  assert.match(merge, /Commit only the files this stage wrote \(docs\/plans\/f-breakdown\.md and docs\/plans\/f-archive\.md\)/);
  assert.match(merge, /undo that merge with `git reset --merge <pre-merge tip of integration>`/);
  assert.match(merge, /would overwrite an uncommitted change that is not yours is also unresolvable/);
});

test('decisions are committed first, applied by a re-plan, and both SHAs reach every later prompt', async () => {
  const decisionsBlock = ['```yaml lsf-decisions', 'decisions:', '  - id: d1', '    date: 2026-09-12', '    question: q', '    decision: yes', '```'].join('\n');
  const { rt, calls, labels } = runtime(twoWaves(), { load: (p, o, u, st) => copied(st.plan, { decisionsBlock }), ledger: () => null }, { decisions: [{ id: 'd1', question: 'q', decision: 'yes' }] });
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
  assert.equal(out.ledgerPath, 'docs/plans/f-signals/plan0.md', 'the ledger is named by the SHA the launch started from, not the decision re-plan\'s');
  assert.match(calls[2].prompt, /mkdir -p \$\(dirname docs\/plans\/f-signals\/plan0\.md\)/);
  assert.match(rp, /### <date, date -I> — decisions d1/);
  assert.match(rp, /<kind> <unit> — <one clause>/);
  assert.match(calls.find((c) => c.label === 'plan:A').prompt, /@ decided1/);
  const w0 = out.unscribed.filter((s) => s.wave === 0).map((s) => [s.stage, s.kind]);
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
    ledger: () => null,
  }, { decisions: [{ id: 'd1', question: 'q', decision: 'proceed, no split' }] });
  const out = await run(rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
  assert.ok(labels().includes('plan:A'));
  assert.deepEqual(out.unscribed.filter((s) => s.wave === 0).map((s) => s.kind), ['verdict', 'over-budget-flagged']);
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

test('role tiers reach every call: critique opus/low, gate opus/medium, planning opus/high, mechanical haiku/low, default opus/low', async () => {
  const { rt, calls } = runtime(twoWaves(), discovering);
  await run(rt);
  const seen = new Set();
  for (const c of calls) {
    const role = c.label.replace(/:.*$/, '');
    seen.add(role);
    if (/^(critique-plan|critique-code|lens-review|critique-design)$/.test(role)) assert.deepEqual([c.opts.model, c.opts.effort], ['opus', 'low'], c.label);
    else if (role === 'review-gate') assert.deepEqual([c.opts.model, c.opts.effort], ['opus', 'medium'], c.label);
    else if (/^(plan|replan|termination)$/.test(role)) assert.deepEqual([c.opts.model, c.opts.effort], ['opus', 'high'], c.label);
    else if (/^(load|ledger|decision-log|cleanup|completion-report)$/.test(role)) assert.deepEqual([c.opts.model, c.opts.effort], ['haiku', 'low'], c.label);
    else assert.deepEqual([c.opts.model, c.opts.effort], ['opus', 'low'], c.label);
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
  const { rt, calls } = runtime(twoWaves(), {}, { tiers: { critique: { model: 'sonnet' } } });
  await run(rt);
  const critique = calls.find((c) => c.label.startsWith('critique-code'));
  const impl = calls.find((c) => c.label.startsWith('implement'));
  assert.equal(critique.opts.model, 'sonnet');
  assert.equal(impl.opts.model, 'opus');
  assert.equal(calls.find((c) => c.label.startsWith('review-gate')).opts.model, 'opus');
  assert.equal(calls.find((c) => c.label.startsWith('plan:')).opts.model, 'opus');
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
  const { rt, calls } = runtime(twoWaves(), discovering);
  await run(rt);
  const loads = calls.filter((c) => c.label.startsWith('load'));
  assert.deepEqual(loads.map((c) => c.label), ['load', 'load:w1', 'load:w2', 'load:termination']);
  assert.match(loads[1].prompt, /git show replan\d:docs\/plans\/f-breakdown\.md/);
  assert.match(calls.find((c) => c.label === 'plan:B').prompt, /@ replan\d/);
});

test('the append stages are told the commit form and the fence placement; every stage copies the short SHA from git', async () => {
  const decisionsBlock = ['```yaml lsf-decisions', 'decisions:', '  - id: d1', '    date: 2026-09-19', '    question: q', '    decision: yes', '```'].join('\n');
  const { rt, calls } = runtime(twoWaves(), {
    load: (p, o, u, st) => copied(st.plan, { decisionsBlock }),
    implement: (p, o, u) => ret(implemented(u, { deviations: ['d'] })),
  }, { decisions: [{ id: 'd1', question: 'q', decision: 'yes' }] });
  await run(rt);
  const ledger = calls.find((c) => c.label === 'ledger:w1').prompt;
  assert.match(ledger, /subject line "docs\(f\): append wave 1 signal ledger entries", then a blank line, then any trailer lines/);
  const log = calls.find((c) => c.label === 'decision-log').prompt;
  assert.match(log, /before the closing ``` line/);
  assert.match(log, /tail -n 3 docs\/plans\/f-decisions\.md/);
  assert.match(log, /subject line "docs\(f\): append decision\(s\) d1"/);
  assert.match(calls.find((c) => c.label === 'load').prompt, /no closing fence is returned as it stands[^.]*without adding one/);
  for (const c of calls) assert.match(c.prompt, /git rev-parse --short HEAD/, c.label);
});

test('a seeded stage return stands in for the agent, matched by label', async () => {
  const planned = ret({ branch: 'salvaged-A', sha: 's', path: 'docs/plans/A-plan.md', readings: [] });
  const { rt, calls, logs } = runtime(twoWaves(), {}, { seed: { 'plan:A': planned } });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  const l = calls.map((c) => c.label);
  assert.ok(!l.includes('plan:A'), 'the seeded agent is not spawned');
  assert.ok(l.includes('plan:B'), 'unseeded labels run');
  assert.match(calls.find((c) => c.label === 'critique-plan:r1:A').prompt, /git show salvaged-A:docs\/plans\/A-plan\.md/, 'the critique reads the seeded plan from its branch');
  assert.match(calls.find((c) => c.label === 'implement:A').prompt, /salvaged-A/);
  assert.ok(logs.some((m) => m.startsWith('plan:A: seeded')));
});

test('args.unscribed rows are scribed first, keep their wave, and stay out of the digest', async () => {
  const carried = [{ kind: 'deviation', source: 'reported', stage: 'implement', unit: 'Z', wave: 3, statement: 'dev-Z', referent: 'src/Z', raisedBy: 'implement', disposition: 'recorded' }];
  const { rt, calls, labels } = runtime(twoWaves(), {}, { unscribed: carried });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  assert.equal(labels()[0], 'ledger:carried');
  assert.match(calls[0].prompt, /## Wave 3[^]*dev-Z/);
  assert.deepEqual(out.unscribed, []);
  assert.equal(out.digest.byKind.deviation, undefined);

  const again = await run(runtime(twoWaves(), { ledger: () => null }, { unscribed: carried }).rt);
  assert.deepEqual(again.unscribed.map((s) => s.statement), ['dev-Z']);

  await assert.rejects(run(runtime(twoWaves(), {}, { unscribed: [{ statement: 'no kind' }] }).rt), /kind is required/);
});

test('a pause ref seen by the merge stage pauses the run at the wave boundary, after the ledger is scribed', async () => {
  const base = defaults(twoWaves());
  const { rt, calls, labels, logs } = runtime(twoWaves(), {
    merge: (...a) => { const r = base.merge(...a); r.result.pauseRequested = true; return r; },
    ...discovering,
  });
  const out = await run(rt);
  assert.equal(out.status, 'paused');
  assert.equal(out.waves, 1);
  assert.match(out.note, /refs\/lsf\/f\/pause/);
  assert.match(out.note, /1 unit\(s\) remain/);
  assert.match(out.planSha, /^replan/, 'the wave\'s re-plan committed before the pause');
  assert.equal(out.escalation, undefined);
  const l = labels();
  assert.ok(l.includes('replan:w1') && l.includes('ledger:w1'), 'the wave finishes: re-plan and ledger run');
  assert.ok(!l.includes('plan:B') && !l.includes('implement:B'), 'no new unit starts');
  assert.ok(logs.some((m) => /pause requested .*merge:w1/.test(m)));
  assert.match(calls.find((c) => c.label === 'merge:w1').prompt, /git show-ref --verify --quiet refs\/lsf\/f\/pause/);
  assert.match(calls.find((c) => c.label === 'ledger:w1').prompt, /refs\/lsf\/f\/pause/);
});

test('a pause ref present at load pauses before any unit starts; args.pauseRef names the ref', async () => {
  const { rt, labels, calls } = runtime(twoWaves(), { load: (p, o, u, state) => copied(state.plan, { pauseRequested: true }) }, { pauseRef: 'refs/custom/pause' });
  const out = await run(rt);
  assert.equal(out.status, 'paused');
  assert.equal(out.waves, 0);
  assert.equal(out.planSha, 'plan0');
  assert.match(out.note, /refs\/custom\/pause/);
  assert.ok(!labels().some((x) => x.startsWith('plan:') || x.startsWith('implement:')));
  assert.match(calls.find((c) => c.label === 'load').prompt, /refs\/custom\/pause/);
});

test('a pause ref seen by the ledger scribe on a wave with nothing to absorb pauses the run', async () => {
  const { rt, labels } = runtime(twoWaves(), {
    implement: (p, o, u) => ret(implemented(u, { deviations: [`dev-${u}`] })),
    ledger: () => ret({ path: 'docs/plans/f-signals.md', sha: 'l', pauseRequested: true }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'paused');
  assert.equal(out.waves, 1);
  assert.deepEqual(out.unscribed, []);
  assert.ok(!labels().includes('implement:B'));
});

test('a stop outranks a pause seen in the same wave', async () => {
  const { rt } = runtime(twoWaves(), {
    merge: () => ret({ integrationBranch: 'integration', merges: [{ unit: 'A', status: 'regressed', files: ['test/a.test.js'], sha: 'm' }], planSha: 'm1', pauseRequested: true }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.deepEqual(out.escalation.signals.map((s) => s.kind), ['merge-regression']);
});

test('a pause at load still commits the decisions and returns the decision re-plan\'s SHA', async () => {
  const decisionsBlock = ['```yaml lsf-decisions', 'decisions:', '  - id: d1', '    date: 2026-09-12', '    question: q', '    decision: yes', '```'].join('\n');
  const { rt, labels } = runtime(twoWaves(), { load: (p, o, u, st) => copied(st.plan, { decisionsBlock, pauseRequested: true }) }, { decisions: [{ id: 'd1', question: 'q', decision: 'yes' }] });
  const out = await run(rt);
  assert.equal(out.status, 'paused');
  assert.equal(out.planSha, 'decided1');
  assert.deepEqual(labels().slice(0, 2), ['decision-log', 'decision-replan']);
  assert.match(out.note, /no decisions/);
  assert.ok(!labels().some((x) => x.startsWith('plan:')));
});

test('a pause seen by a reload or the termination load pauses before the next wave', async () => {
  for (const at of ['load:w1', 'load:termination']) {
    const { rt, labels } = runtime(twoWaves(), {
      ...discovering,
      load: (p, o, u, st) => copied(st.plan, { pauseRequested: o.label === at }),
    });
    const out = await run(rt);
    assert.equal(out.status, 'paused', at);
    if (at === 'load:w1') assert.ok(!labels().includes('plan:B'), at);
    else assert.ok(!labels().some((x) => x.startsWith('obligations')), at);
  }
});

test('a paused return carries a null scribe\'s rows in unscribed and says to pass them back', async () => {
  const base = defaults(twoWaves());
  const { rt } = runtime(twoWaves(), {
    merge: (...a) => { const r = base.merge(...a); r.result.pauseRequested = true; return r; },
    implement: (p, o, u) => ret(implemented(u, { deviations: [`dev-${u}`] })),
    ledger: () => null,
  });
  const out = await run(rt);
  assert.equal(out.status, 'paused');
  assert.deepEqual(out.unscribed.map((s) => s.statement), ['dev-A']);
  assert.match(out.note, /passing unscribed back/);
});

test('a seeded return\'s pause report is ignored', async () => {
  const base = defaults(twoWaves());
  const state = { plan: twoWaves(), completed: new Set(), mergeOrder: ['A'] };
  const seeded = base.merge('', {}, 'w1', state);
  seeded.result.pauseRequested = true;
  const { rt } = runtime(twoWaves(), {}, { seed: { 'merge:w1': seeded } });
  const out = await run(rt);
  assert.equal(out.status, 'complete', JSON.stringify(out.escalation?.signals));
});

// ---------------------------------------------------------------- architecture pin

const DOC = 'docs/architecture/f.md';
const archHeader = (sha) => `Architecture document: ${DOC} @ ${sha}`;
// A merge stub reporting the architecture SHA after the wave, and per-unit merge SHAs.
const mergeMoving = (archByWave, shaOf = (id) => `m-${id}`) => {
  let n = 0;
  return (p, o, u, state) => {
    n += 1;
    state.plan = { ...state.plan, sha: `merged${n}`, units: state.plan.units.map((x) => ({ ...x, complete: state.completed.has(x.id) || state.mergeOrder.includes(x.id) })) };
    const merges = state.mergeOrder.map((id) => ({ unit: id, status: 'clean', files: [], ...(shaOf(id) ? { sha: shaOf(id) } : {}) }));
    return ret({ integrationBranch: 'integration', merges, planSha: state.plan.sha, ...(n in archByWave ? { architectureSha: archByWave[n] } : {}) });
  };
};
const promptOf = (calls, label) => calls.find((c) => c.label === label).prompt;

test('a unit that amends the architecture document: the gate reads the amended text and judges the unit\'s own diff against the pre-wave text; later stages read the amended text', async () => {
  const { rt, calls } = runtime(twoWaves(), { merge: mergeMoving({ 1: 'arch2', 2: 'arch2' }) });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  const gate = promptOf(calls, 'review-gate:w1');
  assert.ok(gate.includes(archHeader('arch2')), 'the gate header names the amended text');
  assert.ok(!gate.includes(archHeader('arch1')));
  assert.match(gate, /changed during this wave, from arch1 to the SHA above/);
  assert.ok(gate.includes(`- A (foundation): \`git diff m-A^1 m-A -- ${DOC}\``));
  assert.match(gate, /Judge each unit's change against the text it was merged onto \(the diff's first commit\), which includes changes merged before it this wave/);
  assert.match(gate, /alters what a section decided beyond that is an invalidation, all five fields filled: `section`; `decision`, what the section decided before the change; `premise`, what that decision rested on; `evidence`, the merge SHA and the lines of the diff; `amendment`, the text the unit wrote/);
  assert.match(gate, /goes under `amendments`/);
  for (const label of ['plan:B', 'critique-plan:r1:B', 'implement:B', 'critique-code:r1:B', 'review-gate:w2']) {
    const p = promptOf(calls, label);
    assert.ok(p.includes(archHeader('arch2')), `${label} reads the amended text`);
    assert.ok(!p.includes('@ arch1'), `${label} names no stale pin`);
  }
  assert.ok(!promptOf(calls, 'review-gate:w2').includes('changed during this wave'), 'wave 2 left the document where wave 1 put it');
  assert.ok(promptOf(calls, 'plan:A').includes(archHeader('arch1')), 'wave 1 ran against the load-time pin');
});

test('a unit-carried change to a decision reaches the gate as a candidate invalidation and stops the run', async () => {
  const { rt, calls } = runtime(twoWaves(), {
    merge: mergeMoving({ 1: 'arch2' }),
    'review-gate': (p) => ret({
      invalidations: p.includes('git diff m-A^1 m-A') ? [{ section: '§5', decision: 'ABI is undisclosed', premise: 'no consumer needs it', evidence: 'm-A', amendment: 'ABI is disclosed per §7.2' }] : [],
      notes: [], amendments: [],
    }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'stopped');
  assert.equal(out.escalation.plan_sha, 'merged1');
  assert.deepEqual(out.escalation.signals.map((s) => [s.kind, s.referent]), [['architecture-invalidated', '§5']]);
  assert.match(out.escalation.decision_needed[0].question, /evidence: m-A; amendment: ABI is disclosed per §7\.2/);
  assert.ok(!calls.some((c) => c.label === 'plan:B'));
});

test('a unit\'s amendment that moves no decision is recorded with its unit, listed in the digest, and stops nothing', async () => {
  const { rt } = runtime(twoWaves(), {
    merge: mergeMoving({ 1: 'arch2' }),
    'review-gate': (p) => ret({ invalidations: [], notes: [], amendments: p.includes('changed during this wave') ? [{ unit: 'A', section: '§7.2', statement: 'records the decision A settled' }] : [] }),
    ledger: () => null,
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  const rows = out.unscribed.filter((s) => s.kind === 'architecture-amendment');
  assert.deepEqual(rows.map((s) => [s.unit, s.source, s.referent, s.disposition, s.wave]), [['A', 'gate', '§7.2', 'recorded', 1]]);
  assert.deepEqual(out.digest.attention.architectureAmendments, ['A: records the decision A settled']);
});

test('a document that did not move leaves the gate prompt as before; an empty or missing report keeps the pin', async () => {
  for (const archByWave of [{ 1: 'arch1', 2: 'arch1' }, { 1: '', 2: '  ' }, {}, { 1: 42 }]) {
    const { rt, calls } = runtime(twoWaves(), { merge: mergeMoving(archByWave) });
    const out = await run(rt);
    assert.equal(out.status, 'complete');
    for (const label of ['review-gate:w1', 'plan:B', 'review-gate:w2']) {
      const p = promptOf(calls, label);
      assert.ok(p.includes(archHeader('arch1')), `${JSON.stringify(archByWave)}: ${label} keeps arch1`);
      assert.ok(!p.includes('changed during this wave'));
    }
  }
});

test('a re-plan reload and the termination reload refresh the pin from the copy stage', async () => {
  const { rt, calls } = runtime(twoWaves(), {
    ...discovering,
    load: (p, o, u, state) => copied(state.plan, { architectureSha: { 'load:w1': 'arch3', 'load:termination': 'arch4' }[o.label] ?? 'arch1' }),
    merge: mergeMoving({ 1: 'arch1', 2: 'arch3' }),
  });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  assert.ok(promptOf(calls, 'replan:w1').includes(archHeader('arch1')));
  assert.ok(promptOf(calls, 'plan:B').includes(archHeader('arch3')), 'the re-plan\'s document-text correction reaches wave 2');
  assert.ok(!promptOf(calls, 'review-gate:w2').includes('changed during this wave'));
  assert.ok(promptOf(calls, 'obligations').includes(archHeader('arch4')));
});

test('a merge with no SHA for a unit falls back to the whole-wave diff; args.architectureSha is the load-time pin and the baseline', async () => {
  const { rt, calls } = runtime(twoWaves(), { merge: mergeMoving({ 1: 'arch2' }, () => undefined) }, { architectureSha: 'argpin' });
  const out = await run(rt);
  assert.equal(out.status, 'complete');
  assert.ok(promptOf(calls, 'plan:A').includes(archHeader('argpin')));
  const gate = promptOf(calls, 'review-gate:w1');
  assert.match(gate, /from argpin to the SHA above/);
  assert.ok(gate.includes(`- A (foundation): merge SHA not reported; \`git diff argpin arch2 -- ${DOC}\` holds its change among any others, judged against the document @ argpin`));
});

test('the merge stage reports the architecture SHA after its last commit, and the gate returns amendments', async () => {
  const { rt, calls } = runtime(twoWaves());
  await run(rt);
  const merge = calls.find((c) => c.label === 'merge:w1');
  assert.ok(merge.prompt.includes(`\`git rev-list -1 integration -- ${DOC}\`) as \`architectureSha\``));
  assert.ok(merge.opts.schema.properties.result.required.includes('architectureSha'));
  const gate = calls.find((c) => c.label === 'review-gate:w1');
  assert.ok(gate.opts.schema.properties.result.required.includes('amendments'));
  assert.match(gate.prompt, /Return `amendments`: a merged unit's own change to the architecture document that moves no decision, as described above when the document moved; an empty array when it did not\./);
});

test('the pin stays on a null merge and advances, ungated, when nothing merged; with no load-time pin each unit is judged against its merge\'s first parent', async () => {
  const nullMerge = runtime(twoWaves(), { merge: () => null });
  assert.equal((await run(nullMerge.rt)).status, 'stopped');
  assert.ok(!nullMerge.labels().some((l) => l.startsWith('review-gate')));

  const refused = runtime(twoWaves(), {
    merge: (p, o, u, state) => ret({ integrationBranch: 'integration', merges: state.mergeOrder.map((id) => ({ unit: id, status: 'unresolvable', files: ['x'] })), planSha: state.plan.sha, architectureSha: 'arch2' }),
  });
  const out = await run(refused.rt);
  assert.equal(out.status, 'stopped');
  assert.ok(!refused.labels().some((l) => l.startsWith('review-gate')), 'nothing merged, no gate');

  const unpinned = runtime(twoWaves(), {
    load: (p, o, u, state) => copied(state.plan, { architectureSha: '' }),
    merge: mergeMoving({ 1: 'arch2' }),
  });
  assert.equal((await run(unpinned.rt)).status, 'complete');
  const gate = promptOf(unpinned.calls, 'review-gate:w1');
  assert.match(gate, /from an unrecorded pin to the SHA above/);
  assert.ok(gate.includes(`- A (foundation): \`git diff m-A^1 m-A -- ${DOC}\``));
});

test('a gate amendment already prefixed with its unit is not prefixed twice', async () => {
  const { rt } = runtime(twoWaves(), {
    merge: mergeMoving({ 1: 'arch2' }),
    'review-gate': (p) => ret({ invalidations: [], notes: [], amendments: p.includes('changed during this wave') ? [{ unit: 'A', section: '§7.2', statement: 'A: settled §7.2' }] : [] }),
    ledger: () => null,
  });
  const out = await run(rt);
  assert.deepEqual(out.digest.attention.architectureAmendments, ['A: settled §7.2']);
});
