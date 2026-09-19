// Scheduling logic for the lsf-implement workflow: plan validation, ready-set
// computation, merge order, signal mapping, the stop rule, and the digest.
// Dependency-free; inlined into the workflow script. This file is the source
// of truth. See docs/design/lsf-workflow.md, "Wave loop" and "Escalation".

import { STOP_KINDS, INVALIDATION_FIELDS, signal } from './schema.js';

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

// Signals from stage returns. Only three stages report anything beyond
// their result — the unit plan (readings), implement (deviations,
// deferrals, discoveries, distress), and the wave gate (invalidations and
// notes) — and every one of those is a record except a complete
// invalidation. The script never classifies free text.

function nullReturnSignal({ stage, unit, wave }) {
  return signal({
    kind: 'agent-null', source: 'structural', stage, unit, wave,
    statement: `${stage} returned null (skipped or terminal error)`, referent: unit ?? stage,
  });
}

// Unit plan stage: each clause that admitted more than one reading, and the
// reading the plan took.
function signalsFromReadings(readings, { unit, wave }) {
  return (readings ?? []).map((r) => signal({
    kind: 'reading', source: 'reported', stage: 'plan', unit, wave,
    statement: `${r.clause} — read as: ${r.reading}`, referent: r.clause,
  }));
}

// Implement stage: deviations (including writes outside the Scope line),
// deferrals, and discoveries, as breakdown's completion report defines them.
function signalsFromImplement(result, { unit, wave }) {
  const base = { source: 'reported', stage: 'implement', unit, wave };
  const out = [];
  for (const d of result.deviations ?? []) out.push(signal({ ...base, kind: 'deviation', statement: d, referent: unit }));
  const outside = result.writesOutsideScope ?? [];
  if (outside.length) {
    out.push(signal({ ...base, kind: 'deviation', statement: `writes outside the Scope line: ${outside.join(', ')}`, referent: outside.join(',') }));
  }
  for (const d of result.deferrals ?? []) out.push(signal({ ...base, kind: 'deferral', statement: d.statement, referent: d.belongsTo }));
  for (const d of result.discoveries ?? []) out.push(signal({ ...base, kind: 'discovery', statement: d.statement, referent: d.affects }));
  return out;
}

// Review stages: Significant and Minor findings are recorded as they stand.
function signalsFromFindings(findings, { stage, unit, wave, round }) {
  return (findings ?? []).filter((f) => f.severity !== 'Critical').map((f) => signal({
    kind: 'finding', source: 'gate', stage, unit, wave,
    statement: `${f.severity}${round ? ` (round ${round})` : ''}: ${f.statement}`, referent: f.location,
  }));
}

// Wave gate: an invalidation with every field present is a STOP; one with a
// field missing, and every note, is a record.
function invalidationComplete(inv) {
  return INVALIDATION_FIELDS.every((k) => typeof inv[k] === 'string' && inv[k].trim() !== '');
}

function signalsFromGate(result, { wave }) {
  const base = { source: 'gate', stage: 'review-gate', wave };
  const out = [];
  for (const inv of result.invalidations ?? []) {
    const statement = INVALIDATION_FIELDS.map((k) => `${k}: ${String(inv[k] ?? '').trim() || '—'}`).join('; ');
    out.push(signal({
      ...base, kind: invalidationComplete(inv) ? 'architecture-invalidated' : 'architecture-note',
      statement: invalidationComplete(inv) ? statement : `incomplete invalidation (${INVALIDATION_FIELDS.filter((k) => !String(inv[k] ?? '').trim()).join(', ')} missing) — ${statement}`,
      referent: inv.section || '',
    }));
  }
  for (const n of result.notes ?? []) {
    out.push(signal({ ...base, kind: 'architecture-note', statement: n.statement, referent: n.referent }));
  }
  return out;
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
  distressDeviationThreshold: 4, // deviations + deferrals
  distressDesignDecisionThreshold: 3, // mid-implementation design decisions
});

// Execution-distress record from a completion report's distress fields.
// Re-plan input: breakdown re-examines the size budget where a unit showed
// distress.
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
    kind: 'execution-distress', source: 'reported', stage: 'implement', unit, wave,
    statement: reasons.join('; '), referent: unit,
  })];
}

// ---------------------------------------------------------------- stop rule

// Kinds the re-plan is handed with ids and must return a verdict on.
const REPLAN_ITEM_KINDS = Object.freeze(['deferral', 'discovery', 'claimed-disjoint-conflict', 'architecture-note', 'execution-distress']);
// Stop kinds whose relaunch target is lsf-architecture: the plan is
// superseded there, so a wave stopping on one skips the re-plan.
const PLAN_SUPERSEDING_STOP_KINDS = Object.freeze(['architecture-invalidated', 'done-condition-unevaluable']);

// Classify a wave's signals by kind. Returns { stop, stops, records, all },
// each signal stamped with its disposition.
function evaluateWave(waveSignals) {
  const stops = [];
  const records = [];
  for (const s of waveSignals) {
    if (STOP_KINDS.includes(s.kind)) {
      s.disposition = 'stopped';
      stops.push(s);
    } else {
      s.disposition = 'recorded';
      records.push(s);
    }
  }
  return { stop: stops.length > 0, stops, records, all: [...waveSignals] };
}

// ---------------------------------------------------------------- digest

function countBy(items, key) {
  const counts = {};
  for (const it of items) counts[it[key]] = (counts[it[key]] ?? 0) + 1;
  return counts;
}

// Markdown for the signal ledger: the scribe appends this verbatim. Cell
// text is one line with pipes escaped so a statement cannot break the table.
const LEDGER_COLUMNS = ['Wave', 'Unit', 'Stage', 'Source', 'Kind', 'Statement', 'Referent', 'Disposition'];
function ledgerCell(v) {
  return String(v ?? '').replace(/\r?\n+/g, ' ').replace(/\|/g, '\\|').trim() || '—';
}
function ledgerHeading(name) {
  return `# Signal ledger: ${name}\n\nAppend-only. Never edit earlier entries — add a later wave's row instead.\n`;
}
// The heading follows the rows: a schedule stop reports the wave it could not
// start, one past the loop's counter; rows from the termination stages
// carry no wave.
function ledgerWave(entries, wave) {
  const waves = new Set(entries.map((e) => e.wave ?? wave));
  return waves.size === 1 ? [...waves][0] : wave;
}
function ledgerLabel(entries, wave) {
  return entries.length && entries.every((e) => e.wave === undefined) ? 'termination' : `wave ${ledgerWave(entries, wave)}`;
}

function renderLedger(entries, wave) {
  const label = ledgerLabel(entries, wave);
  const head = [`## ${label[0].toUpperCase()}${label.slice(1)}`, '', `| ${LEDGER_COLUMNS.join(' | ')} |`, `| ${LEDGER_COLUMNS.map(() => '---').join(' | ')} |`];
  const rows = entries.map((e) => `| ${[e.wave ?? wave, e.unit, e.stage, e.source, e.kind, e.statement, e.referent, e.disposition].map(ledgerCell).join(' | ')} |`);
  return [...head, ...rows].join('\n') + '\n';
}

// Counts by kind, source, and disposition, plus what deserves the
// requester's attention on a run that did not stop: architecture notes,
// unfixed Significant findings per unit, units that showed distress, and
// the edits made without a gate.
function digest(ledger) {
  // Wave 0 is the decision re-plan before load, not a wave of units.
  const waves = [...new Set(ledger.map((s) => s.wave).filter((w) => w !== undefined && w > 0))].sort((a, b) => a - b);
  const unfixedSignificant = {};
  for (const s of ledger) {
    if (s.kind === 'finding' && /^Significant/.test(s.statement)) unfixedSignificant[s.unit] = (unfixedSignificant[s.unit] ?? 0) + 1;
  }
  return {
    waves: waves.length,
    byKind: countBy(ledger, 'kind'),
    bySource: countBy(ledger, 'source'),
    byDisposition: countBy(ledger, 'disposition'),
    attention: {
      architectureNotes: ledger.filter((s) => s.kind === 'architecture-note').map((s) => s.statement),
      architectureAmendments: ledger.filter((s) => s.kind === 'architecture-amendment').map((s) => s.statement),
      unfixedSignificant,
      distressedUnits: [...new Set(ledger.filter((s) => s.kind === 'execution-distress').map((s) => s.unit))],
      directEdits: ledger.filter((s) => s.kind === 'direct-edit').map((s) => `${s.referent} — ${s.statement}`),
    },
    note: 'Silence is not evidence: reported-signal recall on premise-level defects is near zero.',
  };
}

export {
  DEFAULT_CONFIG, BLOCKING_GATE_KINDS, REPLAN_ITEM_KINDS, PLAN_SUPERSEDING_STOP_KINDS,
  findCycle, ancestors, pathsOverlap, scopesOverlap, overlappingPairs, validatePlan,
  readySet, blockedUnits, mergeOrder, wavePlan,
  nullReturnSignal, signalsFromReadings, signalsFromImplement, signalsFromFindings, signalsFromGate, invalidationComplete,
  signalsFromMerge, distressSignals, evaluateWave, digest, renderLedger, ledgerWave, ledgerLabel, ledgerHeading,
};
