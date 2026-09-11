// The plan graph and decision log as machine-readable YAML blocks, and the
// restricted YAML subset they are written in. Dependency-free; inlined into
// the workflow scripts. Format: docs/plan-graph.md.
//
// The workflow script has no file access. A copy agent runs `git show` and
// returns the fenced blocks verbatim; this module parses and validates them.
// Everything the load stage used to ask an agent to interpret is either a
// field here or a structural problem reported as a signal.

import { GATE_KINDS, UNIT_TYPES } from './schema.js';

const PLAN_TAG = 'lsf-plan';
const DECISIONS_TAG = 'lsf-decisions';

// ---------------------------------------------------------------- YAML subset

class YamlError extends Error {
  constructor(message, line) {
    super(line === undefined ? message : `line ${line}: ${message}`);
    this.line = line;
  }
}

// Strip a trailing comment: `#` preceded by whitespace, outside quotes.
function stripComment(s) {
  let quote = null;
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (quote) {
      if (c === '\\' && quote === '"') i++;
      else if (c === quote) quote = null;
    } else if (c === '"' || c === "'") {
      quote = c;
    } else if (c === '#' && (i === 0 || /\s/.test(s[i - 1]))) {
      return s.slice(0, i);
    }
  }
  return s;
}

function tokenise(text) {
  const out = [];
  text.split(/\r?\n/).forEach((raw, idx) => {
    if (raw.includes('\t')) throw new YamlError('tab character; indent with spaces', idx + 1);
    const content = stripComment(raw).trimEnd();
    if (!content.trim()) return;
    const indent = content.length - content.trimStart().length;
    out.push({ indent, content: content.trim(), line: idx + 1 });
  });
  return out;
}

function parseScalar(s, line) {
  if (s === '' || s === '~' || s === 'null') return null;
  if (s === 'true') return true;
  if (s === 'false') return false;
  if (s[0] === '"') {
    if (s[s.length - 1] !== '"' || s.length < 2) throw new YamlError('unterminated double-quoted string', line);
    return s.slice(1, -1).replace(/\\(["\\nt])/g, (m, c) => ({ '"': '"', '\\': '\\', n: '\n', t: '\t' })[c]);
  }
  if (s[0] === "'") {
    if (s[s.length - 1] !== "'" || s.length < 2) throw new YamlError('unterminated single-quoted string', line);
    return s.slice(1, -1).replace(/''/g, "'");
  }
  if (/^[&*!|>%@`]/.test(s)) throw new YamlError(`unsupported YAML syntax: ${s[0]}`, line);
  return s;
}

// `[a, b, "c, d"]` — flow sequence of scalars.
function parseFlowSeq(s, line) {
  const inner = s.slice(1, -1).trim();
  if (inner === '') return [];
  const items = [];
  let cur = '';
  let quote = null;
  for (let i = 0; i < inner.length; i++) {
    const c = inner[i];
    if (quote) {
      cur += c;
      if (c === '\\' && quote === '"') cur += inner[++i];
      else if (c === quote) quote = null;
    } else if (c === '"' || c === "'") {
      quote = c; cur += c;
    } else if (c === ',') {
      items.push(cur.trim()); cur = '';
    } else if (c === '[' || c === '{') {
      throw new YamlError('nested flow collections are not supported', line);
    } else {
      cur += c;
    }
  }
  if (quote) throw new YamlError('unterminated quoted string in flow sequence', line);
  items.push(cur.trim());
  return items.map((it) => {
    if (it === '') throw new YamlError('empty item in flow sequence', line);
    return parseScalar(it, line);
  });
}

function parseValue(s, line) {
  if (s[0] === '[') {
    if (s[s.length - 1] !== ']') throw new YamlError('unterminated flow sequence', line);
    return parseFlowSeq(s, line);
  }
  if (s[0] === '{') throw new YamlError('flow mappings are not supported', line);
  return parseScalar(s, line);
}

const KEY_RE = /^([A-Za-z0-9_][A-Za-z0-9_.-]*):(?:\s+(.*))?$/;

function parseBlock(lines, i, indent) {
  if (i >= lines.length) return { value: null, next: i };
  const first = lines[i];
  if (first.indent !== indent) throw new YamlError(`unexpected indent ${first.indent}, expected ${indent}`, first.line);
  return /^-(\s|$)/.test(first.content) ? parseSeq(lines, i, indent) : parseMap(lines, i, indent);
}

function nested(lines, i, indent) {
  // Value of a key or dash with nothing after it: a block at deeper indent.
  if (i < lines.length && lines[i].indent > indent) return parseBlock(lines, i, lines[i].indent);
  return { value: null, next: i };
}

function parseSeq(lines, i, indent) {
  const out = [];
  while (i < lines.length && lines[i].indent === indent) {
    const { content, line } = lines[i];
    if (!/^-(\s|$)/.test(content)) throw new YamlError('expected a sequence item', line);
    const rest = content.replace(/^-\s*/, '');
    if (rest === '') {
      const r = nested(lines, i + 1, indent);
      out.push(r.value); i = r.next;
    } else if (KEY_RE.test(rest) && !/^["'[]/.test(rest)) {
      // Inline mapping start: `- id: x`; continuation keys align with `id`.
      const col = indent + (content.length - rest.length);
      lines[i] = { indent: col, content: rest, line };
      const r = parseMap(lines, i, col);
      out.push(r.value); i = r.next;
    } else {
      out.push(parseValue(rest, line)); i += 1;
    }
  }
  if (i < lines.length && lines[i].indent > indent) throw new YamlError('unexpected indent', lines[i].line);
  return { value: out, next: i };
}

function parseMap(lines, i, indent) {
  const out = {};
  while (i < lines.length && lines[i].indent === indent) {
    const { content, line } = lines[i];
    const m = KEY_RE.exec(content);
    if (!m) throw new YamlError(`expected \`key: value\`, got \`${content}\``, line);
    const [, key, rawValue] = m;
    if (key in out) throw new YamlError(`duplicate key ${key}`, line);
    if (rawValue === undefined || rawValue === '') {
      const r = nested(lines, i + 1, indent);
      out[key] = r.value; i = r.next;
    } else {
      out[key] = parseValue(rawValue, line); i += 1;
    }
  }
  if (i < lines.length && lines[i].indent > indent) throw new YamlError('unexpected indent', lines[i].line);
  return { value: out, next: i };
}

function parseYaml(text) {
  const lines = tokenise(text);
  if (!lines.length) return null;
  const { value, next } = parseBlock(lines, 0, lines[0].indent);
  if (next < lines.length) throw new YamlError('content after the top-level block', lines[next].line);
  return value;
}

// ---------------------------------------------------------------- blocks

// The one fenced block tagged `tag` in a Markdown document: its body, or a
// problem. `\`\`\`yaml <tag>` opens it.
function extractBlock(markdown, tag) {
  const re = new RegExp(`^\`\`\`yaml[ \\t]+${tag}[ \\t]*\\r?\\n([\\s\\S]*?)^\`\`\`[ \\t]*$`, 'gm');
  const matches = [...(markdown ?? '').matchAll(re)];
  if (matches.length === 0) return { body: null, problem: `no \`\`\`yaml ${tag} block found` };
  if (matches.length > 1) return { body: null, problem: `${matches.length} \`\`\`yaml ${tag} blocks found; exactly one is allowed` };
  return { body: matches[0][1], problem: null };
}

// ---------------------------------------------------------------- plan graph

const problem = (statement, referent) => ({ kind: 'invalid-plan', statement, referent });

const isStringList = (v) => Array.isArray(v) && v.every((x) => typeof x === 'string' && x !== '');

// Parse the plan block into the graph shape scheduling.js reads (PLAN_SCHEMA
// less sha/path, which the caller adds). Returns { plan, problems }; `plan`
// is null when the block cannot be read at all.
function parsePlanGraph(body) {
  const problems = [];
  let doc;
  try {
    doc = parseYaml(body);
  } catch (e) {
    return { plan: null, problems: [problem(`plan block: ${e.message}`, PLAN_TAG)] };
  }
  if (!doc || typeof doc !== 'object' || Array.isArray(doc)) {
    return { plan: null, problems: [problem('plan block: top level must be a mapping', PLAN_TAG)] };
  }
  for (const k of Object.keys(doc)) {
    if (!['units', 'execution_order'].includes(k)) problems.push(problem(`plan block: unknown top-level key ${k}`, k));
  }
  if (!Array.isArray(doc.units)) {
    return { plan: null, problems: [...problems, problem('plan block: `units` must be a sequence', 'units')] };
  }
  const units = [];
  doc.units.forEach((u, idx) => {
    const ref = (u && typeof u.id === 'string' && u.id) || `units[${idx}]`;
    if (!u || typeof u !== 'object' || Array.isArray(u)) { problems.push(problem(`${ref}: unit must be a mapping`, ref)); return; }
    if (typeof u.id !== 'string' || !u.id) problems.push(problem(`${ref}: missing id`, ref));
    for (const k of Object.keys(u)) {
      if (!['id', 'type', 'complete', 'dependencies', 'scope', 'publishes', 'gates'].includes(k)) problems.push(problem(`${ref}: unknown key ${k}`, ref));
    }
    if (!UNIT_TYPES.includes(u.type)) problems.push(problem(`${ref}: type must be one of ${UNIT_TYPES.join(', ')}; got ${JSON.stringify(u.type ?? null)}`, ref));
    if (u.complete !== undefined && typeof u.complete !== 'boolean') problems.push(problem(`${ref}: complete must be true or false`, ref));
    const deps = u.dependencies ?? [];
    if (!isStringList(deps)) problems.push(problem(`${ref}: dependencies must be a sequence of unit ids`, ref));
    const scope = u.scope ?? [];
    if (!isStringList(scope)) problems.push(problem(`${ref}: scope must be a sequence of paths`, ref));
    const publishes = u.publishes ?? [];
    if (!isStringList(publishes)) problems.push(problem(`${ref}: publishes must be a sequence of strings`, ref));
    const gates = u.gates ?? [];
    if (!Array.isArray(gates)) problems.push(problem(`${ref}: gates must be a sequence`, ref));
    const outGates = [];
    (Array.isArray(gates) ? gates : []).forEach((g, gi) => {
      const gref = `${ref}/${(g && typeof g.name === 'string' && g.name) || `gates[${gi}]`}`;
      if (!g || typeof g !== 'object' || Array.isArray(g)) { problems.push(problem(`${gref}: gate must be a mapping`, gref)); return; }
      for (const k of Object.keys(g)) {
        if (!['name', 'kind', 'pin'].includes(k)) problems.push(problem(`${gref}: unknown key ${k}`, gref));
      }
      if (typeof g.name !== 'string' || !g.name) problems.push(problem(`${gref}: gate missing name`, gref));
      if (!GATE_KINDS.includes(g.kind)) problems.push(problem(`${gref}: gate kind must be one of ${GATE_KINDS.join(', ')}; got ${JSON.stringify(g.kind ?? null)}`, gref));
      if (g.pin !== undefined && g.pin !== null && typeof g.pin !== 'string') problems.push(problem(`${gref}: pin must be a string`, gref));
      const gate = { name: String(g.name ?? ''), kind: g.kind, cleared: false };
      if (typeof g.pin === 'string') gate.pin = g.pin;
      outGates.push(gate);
    });
    const names = outGates.map((g) => g.name);
    for (const n of new Set(names.filter((n, i) => names.indexOf(n) !== i))) problems.push(problem(`${ref}: duplicate gate name ${n}`, `${ref}/${n}`));
    units.push({
      id: String(u.id ?? ref),
      type: u.type,
      complete: u.complete === true,
      dependencies: (isStringList(deps) ? deps : []).map((d) => ({ unit: d })),
      scope: isStringList(scope) ? scope : [],
      publishes: isStringList(publishes) ? publishes : [],
      gates: outGates,
    });
  });
  if (!isStringList(doc.execution_order ?? null)) problems.push(problem('plan block: `execution_order` must be a sequence of unit ids', 'execution_order'));
  const executionOrder = isStringList(doc.execution_order ?? null) ? doc.execution_order : [];
  const dupOrder = executionOrder.filter((id, i) => executionOrder.indexOf(id) !== i);
  for (const id of new Set(dupOrder)) problems.push(problem(`execution_order names ${id} more than once`, id));
  return { plan: { units, executionOrder }, problems };
}

// ---------------------------------------------------------------- decisions

// Parse the decision-log block. Returns { decisions, problems }; `decisions`
// is an empty list when the body is null (no log yet).
function parseDecisions(body) {
  if (body === null || body === undefined) return { decisions: [], problems: [] };
  const problems = [];
  let doc;
  try {
    doc = parseYaml(body);
  } catch (e) {
    return { decisions: [], problems: [problem(`decisions block: ${e.message}`, DECISIONS_TAG)] };
  }
  if (doc === null) return { decisions: [], problems: [] };
  if (!doc || typeof doc !== 'object' || Array.isArray(doc) || !Array.isArray(doc.decisions ?? [])) {
    return { decisions: [], problems: [problem('decisions block: top level must be a mapping with a `decisions` sequence', DECISIONS_TAG)] };
  }
  for (const k of Object.keys(doc)) {
    if (k !== 'decisions') problems.push(problem(`decisions block: unknown top-level key ${k}`, k));
  }
  const decisions = [];
  (doc.decisions ?? []).forEach((d, idx) => {
    const ref = (d && typeof d.id === 'string' && d.id) || `decisions[${idx}]`;
    if (!d || typeof d !== 'object' || Array.isArray(d)) { problems.push(problem(`${ref}: decision must be a mapping`, ref)); return; }
    for (const k of Object.keys(d)) {
      if (!['id', 'date', 'question', 'decision', 'clears', 'resolves'].includes(k)) problems.push(problem(`${ref}: unknown key ${k}`, ref));
    }
    for (const k of ['id', 'date', 'question', 'decision']) {
      if (typeof d[k] !== 'string' || !d[k]) problems.push(problem(`${ref}: missing ${k}`, ref));
    }
    const clears = d.clears ?? [];
    if (!isStringList(clears)) problems.push(problem(`${ref}: clears must be a sequence of \`unit/gate\``, ref));
    const resolves = d.resolves ?? [];
    if (!isStringList(resolves)) problems.push(problem(`${ref}: resolves must be a sequence of strings`, ref));
    decisions.push({
      id: String(d.id ?? ref), date: String(d.date ?? ''), question: String(d.question ?? ''), decision: String(d.decision ?? ''),
      clears: isStringList(clears) ? clears : [], resolves: isStringList(resolves) ? resolves : [],
    });
  });
  const ids = decisions.map((d) => d.id);
  for (const id of new Set(ids.filter((x, i) => ids.indexOf(x) !== i))) problems.push(problem(`duplicate decision id ${id}`, id));
  return { decisions, problems };
}

// Mark gates cleared by the decision log. A `clears` entry must name an
// existing unit and gate; environment gates are re-probed, never cleared by
// decision, so clearing one is a problem.
function applyClearances(plan, decisions) {
  const problems = [];
  const byId = new Map(plan.units.map((u) => [u.id, u]));
  for (const d of decisions) {
    for (const c of d.clears) {
      const slash = c.indexOf('/');
      const unitId = slash < 0 ? c : c.slice(0, slash);
      const gateName = slash < 0 ? '' : c.slice(slash + 1);
      const u = byId.get(unitId);
      const g = u?.gates.find((x) => x.name === gateName);
      if (!u || !g) { problems.push(problem(`decision ${d.id} clears unknown gate ${c}`, c)); continue; }
      if (g.kind === 'environment') { problems.push(problem(`decision ${d.id} clears environment gate ${c}; environment gates are probed, not decided`, c)); continue; }
      g.cleared = true;
      g.clearedBy = d.id;
    }
  }
  return problems;
}

// Load stage, script side: extract, parse, join. `planMarkdown` is the plan
// document; `decisionsMarkdown` is the decision log or null. Returns
// { plan, decisions, problems }; `plan` is null only when the block is
// unreadable, in which case `problems` says why.
function loadGraph({ planMarkdown, decisionsMarkdown }) {
  const problems = [];
  const pb = extractBlock(planMarkdown, PLAN_TAG);
  if (pb.problem) return { plan: null, decisions: [], problems: [problem(pb.problem, PLAN_TAG)] };
  const parsed = parsePlanGraph(pb.body);
  problems.push(...parsed.problems);
  if (!parsed.plan) return { plan: null, decisions: [], problems };
  let decisions = [];
  if (decisionsMarkdown !== null && decisionsMarkdown !== undefined) {
    const db = extractBlock(decisionsMarkdown, DECISIONS_TAG);
    if (db.problem) problems.push(problem(db.problem, DECISIONS_TAG));
    else {
      const pd = parseDecisions(db.body);
      problems.push(...pd.problems);
      decisions = pd.decisions;
    }
  }
  problems.push(...applyClearances(parsed.plan, decisions));
  return { plan: parsed.plan, decisions, problems };
}

export {
  PLAN_TAG, DECISIONS_TAG, YamlError,
  parseYaml, extractBlock, parsePlanGraph, parseDecisions, applyClearances, loadGraph,
};
