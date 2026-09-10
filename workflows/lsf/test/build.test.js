import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { build, strip, OUTPUT } from '../build.js';

const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;

test('strip removes import and export statements only', () => {
  const src = `import { a,\n  b } from './x.js';\nconst y = 1;\nexport {\n  y,\n};\n`;
  assert.equal(strip(src), 'const y = 1;');
});

test('built script starts with the meta literal and parses as an async body', () => {
  const script = build();
  assert.ok(script.startsWith('export const meta = {'));
  assert.ok(!/^import\s/m.test(script), 'no import statements remain');
  assert.ok(!/^export\s*\{/m.test(script), 'no export statements remain');
  const body = script.replace(/^export const meta/, 'const meta');
  // Throws on a syntax error; the runtime globals are declared as parameters.
  new AsyncFunction('agent', 'parallel', 'log', 'args', 'phase', 'pipeline', 'budget', 'workflow', body);
});

test('generated workflows/lsf-implement.js is up to date', () => {
  let current = '';
  try { current = readFileSync(OUTPUT, 'utf8'); } catch { /* missing counts as stale */ }
  assert.equal(current, build(), 'run `node workflows/lsf/build.js`');
});
