// Scheduling logic for the lsf-implement workflow: plan validation, ready-set
// computation, merge order, signal mapping, the stop rule, and trend flags.
// Dependency-free; inlined into the workflow script. This file is the source
// of truth. See docs/design/lsf-workflow.md, "Wave loop" and "Escalation".

import { STOP_KINDS, signal } from './schema.js';

const BLOCKING_GATE_KINDS = ['organisational', 'release'];

// ---------------------------------------------------------------- graph

function unitIndex(plan) {
  const byId = new Map();
  for (const u of plan.units) byId.set(u.id, u);
  return byId;
}

// Return a cycle as a list of unit ids, or null. Edges are dependencies of
// every type — the union must be acyclic.
function findCycle(plan) {
  const byId = unitIndex(plan);
  const state = new Map(); // id -> 'active' | 'done'
  const stack = [];
  const visit = (id) => {
    const s = state.get(id);
    if (s === 'done') return null;
    if (s === 'active') return stack.slice(stack.indexOf(id)).concat(id);
    state.set(id, 'active');
    stack.push(id);
    for (const d of byId.get(id)?.dependencies ?? []) {
      if (!byId.has(d.unit)) continue;
      const cycle = visit(d.unit);
      if (cycle) return cycle;
    }
    stack.pop();
    state.set(id, 'done');
    return null;
  };
  for (const u of plan.units) {
    const cycle = visit(u.id);
    if (cycle) return cycle;
  }
  return null;
}

// Set of unit ids reachable from `id` along dependency edges (excluding id).
function ancestors(plan, id) {
  const byId = unitIndex(plan);
  const seen = new Set();
  const queue = [id];
  while (queue.length) {
    const cur = queue.shift();
    for (const d of byId.get(cur)?.dependencies ?? []) {
      if (!seen.has(d.unit) && byId.has(d.unit)) {
        seen.add(d.unit);
        queue.push(d.unit);
      }
    }
  }
  seen.delete(id);
  return seen;
}

// A scope entry is a path, optionally narrowed to one symbol: `path::symbol`.
function splitScope(entry) {
  const i = entry.indexOf('::');
  const path = (i < 0 ? entry : entry.slice(0, i)).replace(/\/+$/, '');
  return { path, symbol: i < 0 ? null : entry.slice(i + 2) };
}

// True when one entry contains or equals the other. A directory contains the
// paths under it; a bare file contains its symbols; two symbols of one file
// overlap only when they are the same symbol.
function pathsOverlap(a, b) {
  const sa = splitScope(a);
  const sb = splitScope(b);
  if (sa.path !== sb.path) return sa.path.startsWith(sb.path + '/') || sb.path.startsWith(sa.path + '/');
  return sa.symbol === null || sb.symbol === null || sa.symbol === sb.symbol;
}

function scopesOverlap(a, b) {
  return a.scope.some((pa) => b.scope.some((pb) => pathsOverlap(pa, pb)));
}

// Pairs of incomplete units with overlapping scope. Each pair is
// [earlierId, laterId] by execution order, with `serialised` true when a
// dependency path in either direction orders them. A completed unit is
// already merged and cannot conflict, so it pairs with nothing.
function overlappingPairs(plan, completed = new Set()) {
  const order = new Map(plan.executionOrder.map((id, i) => [id, i]));
  const pairs = [];
  for (let i = 0; i < plan.units.length; i++) {
    for (let j = i + 1; j < plan.units.length; j++) {
      const a = plan.units[i];
      const b = plan.units[j];
      if (isComplete(a, completed) || isComplete(b, completed)) continue;
      if (!scopesOverlap(a, b)) continue;
      const serialised = ancestors(plan, a.id).has(b.id) || ancestors(plan, b.id).has(a.id);
      const [x, y] = (order.get(a.id) ?? Infinity) <= (order.get(b.id) ?? Infinity) ? [a, b] : [b, a];
      pairs.push({ units: [x.id, y.id], serialised });
    }
  }
  return pairs;
}

// Checks the load stage applies before the wave loop starts. Returns a list of
// { kind, statement, referent } problems; empty means the plan is loadable.
function validatePlan(plan) {
  const problems = [];
  const byId = unitIndex(plan);
  const ids = plan.units.map((u) => u.id);
  const dupes = ids.filter((id, i) => ids.indexOf(id) !== i);
  for (const id of new Set(dupes)) {
    problems.push({ kind: 'invalid-plan', statement: `duplicate unit id ${id}`, referent: id });
  }
  for (const u of plan.units) {
    for (const d of u.dependencies) {
      if (!byId.has(d.unit)) {
        problems.push({ kind: 'invalid-plan', statement: `${u.id} depends on unknown unit ${d.unit}`, referent: u.id });
      }
    }
    if (!plan.executionOrder.includes(u.id)) {
      problems.push({ kind: 'invalid-plan', statement: `${u.id} missing from execution order`, referent: u.id });
    }
  }
  for (const id of plan.executionOrder) {
    if (!byId.has(id)) {
      problems.push({ kind: 'invalid-plan', statement: `execution order names unknown unit ${id}`, referent: id });
    }
  }
  const cycle = findCycle(plan);
  if (cycle) {
    problems.push({ kind: 'cycle', statement: `dependency cycle ${cycle.join(' -> ')}`, referent: cycle[0] });
  }
  for (const p of overlappingPairs(plan)) {
    if (!p.serialised) {
      problems.push({
        kind: 'unserialised-overlap',
        statement: `${p.units[0]} and ${p.units[1]} have overlapping scope and no dependency path between them`,
        referent: p.units.join('+'),
      });
    }
  }
  return problems;
}

// ---------------------------------------------------------------- ready set

function isComplete(unit, completed) {
  return unit.complete || completed.has(unit.id);
}

function uncleared(unit) {
  return unit.gates.filter((g) => BLOCKING_GATE_KINDS.includes(g.kind) && !g.cleared);
}

// Units whose dependencies are all complete and which carry no uncleared
// organisational or release gate, in plan execution order.
function readySet(plan, completed = new Set()) {
  const byId = unitIndex(plan);
  const order = new Map(plan.executionOrder.map((id, i) => [id, i]));
  return plan.units
    .filter((u) => !isComplete(u, completed))
    .filter((u) => u.dependencies.every((d) => isComplete(byId.get(d.unit), completed)))
    .filter((u) => uncleared(u).length === 0)
    .sort((a, b) => order.get(a.id) - order.get(b.id));
}

// Incomplete units that are not ready, with why. `gated` units have every
// dependency complete and at least one uncleared gate; `waiting` units have an
// incomplete dependency.
function blockedUnits(plan, completed = new Set()) {
  const byId = unitIndex(plan);
  const ready = new Set(readySet(plan, completed).map((u) => u.id));
  return plan.units
    .filter((u) => !isComplete(u, completed) && !ready.has(u.id))
    .map((u) => {
      const waitingOn = u.dependencies.filter((d) => !isComplete(byId.get(d.unit), completed)).map((d) => d.unit);
      const gates = uncleared(u).map((g) => g.name);
      return { unit: u.id, reason: waitingOn.length ? 'waiting' : 'gated', waitingOn, gates };
    });
}

// Merge order for a wave's units: plan execution order, never a choice.
function mergeOrder(plan, unitIds) {
  const order = new Map(plan.executionOrder.map((id, i) => [id, i]));
  return [...unitIds].sort((a, b) => order.get(a) - order.get(b));
}

// Partition the whole remaining plan into waves, assuming every gate clears
// and nothing is re-planned. For narration and critical-path display only.
function wavePlan(plan, completed = new Set()) {
  const done = new Set(completed);
  const waves = [];
  for (;;) {
    const ready = readySet(plan, done).filter((u) => uncleared(u).length === 0);
    if (!ready.length) break;
    waves.push(ready.map((u) => u.id));
    for (const u of ready) done.add(u.id);
  }
  return waves;
}

// ---------------------------------------------------------------- signals

const AMBIGUITY_KIND = {
  scope: 'scope-ambiguity',
  goal: 'goal-ambiguity',
  requirement: 'requirement-incomplete',
  design: 'design-ambiguity',
};

// Map a stage's shared-schema fields into signals. `source` defaults to
// 'reported'; gate agents (critique, review-gate) pass 'gate'. A null stage
// return is a STOP the caller raises via nullReturnSignal.
function signalsFromStageReturn(ret, { stage, unit, wave, source = 'reported' }) {
  const out = [];
  const base = { source, stage, unit, wave, raisedBy: stage };
  for (const a of ret.assumptions ?? []) {
    out.push(signal({ ...base, kind: 'assumption', statement: a.statement, referent: a.referent }));
  }
  for (const a of ret.ambiguities ?? []) {
    out.push(signal({ ...base, kind: AMBIGUITY_KIND[a.about] ?? 'scope-ambiguity', statement: a.statement, referent: a.about }));
  }
  for (const c of ret.contradictions ?? []) {
    out.push(signal({ ...base, kind: 'assumption-invalidated', statement: c.statement, referent: c.against }));
  }
  return out;
}

function nullReturnSignal({ stage, unit, wave }) {
  return signal({
    kind: 'agent-null', source: 'structural', stage, unit, wave,
    statement: `${stage} returned null (skipped or terminal error)`, referent: unit ?? stage,
  });
}

// Signals from the merge stage's structured result.
function signalsFromMerge(mergeResult, { wave }) {
  const out = [];
  for (const m of mergeResult.merges) {
    if (m.status === 'clean') continue;
    const files = m.files.join(', ');
    if (m.status === 'unresolvable') {
      out.push(signal({
        kind: 'merge-conflict-unresolvable', source: 'structural', stage: 'merge', unit: m.unit, wave,
        statement: `merge of ${m.unit} conflicts in ${files}`, referent: files,
      }));
    } else if (m.status === 'regressed') {
      out.push(signal({
        kind: 'merge-regression', source: 'structural', stage: 'merge', unit: m.unit, wave,
        statement: `merge of ${m.unit} was clean but the test suite failed in ${files}; merge undone`, referent: files,
      }));
    } else {
      out.push(signal({
        kind: 'claimed-disjoint-conflict', source: 'structural', stage: 'merge', unit: m.unit, wave,
        statement: `merge of ${m.unit} conflicted in ${files}; resolved mechanically`, referent: files,
      }));
    }
  }
  return out;
}

const DEFAULT_CONFIG = Object.freeze({
  perUnitSignalThreshold: 6, // assumptions + ambiguities per unit per wave
  distressDeviationThreshold: 4, // deviations + deferrals
  distressDesignDecisionThreshold: 1,
});

// Execution-distress signals from a completion report's distress fields.
function distressSignals(distress, { unit, wave }, config = DEFAULT_CONFIG) {
  const cfg = { ...DEFAULT_CONFIG, ...config };
  const reasons = [];
  if (distress.contextNearExhaustion) reasons.push('context near exhaustion');
  if (distress.midImplementationDesignDecisions >= cfg.distressDesignDecisionThreshold) {
    reasons.push(`${distress.midImplementationDesignDecisions} mid-implementation design decision(s)`);
  }
  if (distress.deviations + distress.deferrals >= cfg.distressDeviationThreshold) {
    reasons.push(`${distress.deviations} deviation(s) and ${distress.deferrals} deferral(s)`);
  }
  if (!reasons.length) return [];
  return [signal({
    kind: 'execution-distress', source: 'structural', stage: 'implement', unit, wave,
    statement: reasons.join('; '), referent: unit,
  })];
}

// ---------------------------------------------------------------- stop rule

function normaliseReferent(r) {
  return String(r ?? '').trim().toLowerCase();
}

// Volume rules over this wave's signals and the ledger so far. Content-blind:
// counting only.
function volumeSignals(waveSignals, prior, { wave, unitsInWave }, config = DEFAULT_CONFIG) {
  const cfg = { ...DEFAULT_CONFIG, ...config };
  const out = [];

  // Same referent from two waves or two sources.
  const all = [...prior, ...waveSignals];
  const byReferent = new Map();
  for (const s of all) {
    const key = normaliseReferent(s.referent);
    if (!key) continue;
    if (!byReferent.has(key)) byReferent.set(key, []);
    byReferent.get(key).push(s);
  }
  for (const [key, sigs] of byReferent) {
    if (!sigs.some((s) => waveSignals.includes(s))) continue; // only fire when this wave touched it
    const waves = new Set(sigs.map((s) => s.wave));
    const sources = new Set(sigs.map((s) => s.source));
    if (waves.size > 1 || sources.size > 1) {
      out.push(signal({
        kind: 'referent-recurrence', source: 'structural', stage: 'stop-rule', wave,
        statement: `referent named ${sigs.length} times across waves ${[...waves].join(',')} and sources ${[...sources].join(',')}`,
        referent: key,
      }));
    }
  }

  // Per-unit threshold on assumptions and ambiguities.
  const perUnit = new Map();
  for (const s of waveSignals) {
    if (s.source !== 'reported' || !s.unit) continue;
    if (s.kind === 'assumption' || s.kind.endsWith('-ambiguity') || s.kind === 'requirement-incomplete') {
      perUnit.set(s.unit, (perUnit.get(s.unit) ?? 0) + 1);
    }
  }
  for (const [unit, n] of perUnit) {
    if (n > cfg.perUnitSignalThreshold) {
      out.push(signal({
        kind: 'signal-volume', source: 'structural', stage: 'stop-rule', unit, wave,
        statement: `${n} assumptions and ambiguities reported for ${unit} (threshold ${cfg.perUnitSignalThreshold})`,
        referent: unit,
      }));
    }
  }

  // Silent wave.
  if (unitsInWave > 0 && !waveSignals.some((s) => s.source === 'reported')) {
    out.push(signal({
      kind: 'silent-wave', source: 'structural', stage: 'stop-rule', wave,
      statement: `no stage in wave ${wave} reported an assumption, ambiguity, or contradiction`,
      referent: `wave-${wave}`,
    }));
  }
  return out;
}

// Classify a wave's signals. `prior` is the ledger from earlier waves.
// Returns { stop, stops, records, all } where `all` is the wave's signals plus
// any volume signals, each stamped with its disposition.
function evaluateWave(waveSignals, prior, ctx, config = DEFAULT_CONFIG) {
  const all = [...waveSignals, ...volumeSignals(waveSignals, prior, ctx, config)];
  const stops = [];
  const records = [];
  for (const s of all) {
    if (STOP_KINDS.includes(s.kind)) {
      s.disposition = 'stopped';
      stops.push(s);
    } else {
      s.disposition = 'recorded';
      records.push(s);
    }
  }
  return { stop: stops.length > 0, stops, records, all };
}

// ---------------------------------------------------------------- digest

function countBy(items, key) {
  const counts = {};
  for (const it of items) counts[it[key]] = (counts[it[key]] ?? 0) + 1;
  return counts;
}

// Three trend flags over the full ledger, plus counts by kind and source.
function digest(ledger, { unitsPerWave = {} } = {}) {
  const waves = [...new Set(ledger.map((s) => s.wave).filter((w) => w !== undefined))].sort((a, b) => a - b);

  // Rising assumption count per unit: strictly increasing over the last three waves.
  const perWave = waves.map((w) => {
    const sigs = ledger.filter((s) => s.wave === w && s.kind === 'assumption' && s.unit);
    const units = unitsPerWave[w] ?? new Set(sigs.map((s) => s.unit)).size;
    return units ? sigs.length / units : 0;
  });
  const tail = perWave.slice(-3);
  const risingAssumptions = tail.length >= 2 && tail.every((v, i) => i === 0 || v > tail[i - 1]);

  const recurring = [...new Set(
    ledger.filter((s) => s.kind === 'referent-recurrence').map((s) => s.referent),
  )];
  const silentWaves = ledger.filter((s) => s.kind === 'silent-wave').map((s) => s.wave);

  return {
    waves: waves.length,
    byKind: countBy(ledger, 'kind'),
    bySource: countBy(ledger, 'source'),
    byDisposition: countBy(ledger, 'disposition'),
    trends: { risingAssumptions, assumptionsPerUnitByWave: perWave, recurringReferents: recurring, silentWaves },
    note: 'Silence is not evidence: reported-signal recall on premise-level defects is near zero.',
  };
}

export {
  DEFAULT_CONFIG, BLOCKING_GATE_KINDS,
  findCycle, ancestors, pathsOverlap, scopesOverlap, overlappingPairs, validatePlan,
  readySet, blockedUnits, mergeOrder, wavePlan,
  signalsFromStageReturn, nullReturnSignal, signalsFromMerge, distressSignals,
  volumeSignals, evaluateWave, digest,
};
