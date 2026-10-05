// Spike check 1: one headless call per stage schema at the stage's tier,
// all concurrently (also check 8). Output: one JSON line per schema.
import { spawn } from 'node:child_process';
import { SCHEMAS } from './schemas.js';
import { validate } from './validate.js';

const TIER = {
  COPY_SCHEMA: ['haiku'], LEDGER_SCHEMA: ['haiku'], DECISION_LOG_SCHEMA: ['haiku'], COMPLETION_SCHEMA: ['haiku'], CLEANUP_SCHEMA: ['haiku'],
  UNIT_PLAN_SCHEMA: ['opus', 'high'], REPLAN_SCHEMA: ['opus', 'high'], TERMINATION_SCHEMA: ['opus', 'high'],
  FINDINGS_SCHEMA: ['opus', 'low'], GATE_SCHEMA: ['opus', 'medium'],
  FIX_SCHEMA: ['opus', 'low'], FIX_CODE_SCHEMA: ['opus', 'low'], IMPLEMENT_SCHEMA: ['opus', 'low'], MERGE_SCHEMA: ['opus', 'low'],
  OBLIGATIONS_SCHEMA: ['opus', 'low'], SCRIBE_SCHEMA: ['opus', 'low'],
};
const cwd = process.argv[2];

function call(name) {
  const [model, effort] = TIER[name];
  const args = ['-p', `Schema conformance probe for the ${name.replace('_SCHEMA', '').toLowerCase()} stage of a workflow. Do not use any tool except the structured output. Return a plausible, fully populated example that fits the output schema, as if the stage had run on a small project: every array has at least one element.`,
    '--model', model, ...(effort ? ['--effort', effort] : []), '--output-format', 'json', '--no-session-persistence',
    '--strict-mcp-config', '--permission-mode', 'dontAsk', '--tools', '', '--json-schema', JSON.stringify(SCHEMAS[name])];
  return new Promise((resolve) => {
    const t0 = Date.now();
    const p = spawn('claude', args, { cwd, stdio: ['ignore', 'pipe', 'pipe'] });
    let out = '', err = '';
    p.stdout.on('data', (d) => { out += d; });
    p.stderr.on('data', (d) => { err += d; });
    p.on('close', (code) => {
      let r = null; try { r = JSON.parse(out); } catch {}
      const so = r?.structured_output ?? null;
      const errors = so ? validate(SCHEMAS[name], so) : ['no structured_output'];
      resolve({ name, model, effort, code, secs: (Date.now() - t0) / 1000, subtype: r?.subtype, is_error: r?.is_error, valid: errors.length === 0, errors: errors.slice(0, 5), cost: r?.total_cost_usd, stderr: err.slice(0, 200) });
    });
  });
}
const names = Object.keys(TIER);
const results = await Promise.all(names.map(call));
for (const r of results) console.log(JSON.stringify(r));
