#!/bin/sh
# Spike only: create the fixture repository for the end-to-end run at $1.
set -eu
R=$1
rm -rf "$R"; mkdir -p "$R"; cd "$R"
git init -q -b main
git config user.name "lsf spike"; git config user.email "spike@example.invalid"
mkdir -p src test scripts docs/architecture docs/plans
cat > CLAUDE.md <<'EOF'
# stats fixture

A tiny JavaScript statistics library (Node, no dependencies, ES modules).

Gates, run from the repository root, both must pass before a commit is merged:
- `node --test` (the test suite)
- `node scripts/lint.js` (the linter)

Commit discipline: one logical change per commit, conventional subject line.
EOF
cat > package.json <<'EOF'
{ "name": "stats-fixture", "type": "module", "private": true }
EOF
cat > src/stats.js <<'EOF'
// Descriptive statistics over arrays of finite numbers.

export function sum(xs) {
  let s = 0;
  for (const x of xs) s += x;
  return s;
}
EOF
cat > test/stats.test.js <<'EOF'
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { sum } from '../src/stats.js';

test('sum', () => {
  assert.equal(sum([]), 0);
  assert.equal(sum([1, 2, 3]), 6);
});
EOF
cat > scripts/lint.js <<'EOF'
// Fixture linter: every exported function in src/ has a comment line above it.
import { readdirSync, readFileSync } from 'node:fs';
let bad = 0;
for (const f of readdirSync('src')) {
  const lines = readFileSync(`src/${f}`, 'utf8').split('\n');
  lines.forEach((l, i) => {
    if (/^export function/.test(l) && !/^\s*\/\//.test(lines[i - 1] ?? '')) { console.error(`src/${f}:${i + 1}: exported function without a comment`); bad++; }
  });
}
console.log(`lint: ${bad} problem(s)`);
process.exit(bad ? 1 : 0);
EOF
sed -i 's|^export function sum|// Sum of xs; 0 for an empty array.\nexport function sum|' src/stats.js
cat > docs/architecture/stats.md <<'EOF'
# Architecture: stats

## Module

All functions live in `src/stats.js` as named exports over arrays of finite numbers. No classes, no dependencies.

## Errors

A function undefined on its input (for example, the mean of an empty array) throws a `RangeError` naming the function. It never returns `NaN`.
EOF
cat > docs/architecture/stats-brief.md <<'EOF'
# Brief: stats

Requirements: add descriptive statistics to the library, starting with the arithmetic mean.

Success criteria: each function is tested, including its error case; both gates pass.
EOF
cat > docs/plans/stats-breakdown.md <<'EOF'
# Breakdown: stats

## Work Units

### add-mean
- **Type**: feature
- **Scope**: `src/stats.js`, `test/stats.test.js`
- **Publishes**: `mean`
- **Depends on**: none
- **Done condition**: `mean(xs)` is exported from `src/stats.js`, returns the arithmetic mean of a non-empty array, and throws a `RangeError` for an empty array; tests cover both; `node --test` and `node scripts/lint.js` pass.
- **Description**: Implement on top of `sum`.

## Standing Obligations

- Every exported function has a test of its error case. Carried by: add-mean.

## Execution Order

1. add-mean

## Plan Maintenance

## Unit graph

```yaml lsf-plan
units:
  - id: add-mean
    type: feature
    complete: false
    dependencies: []
    scope: [src/stats.js, test/stats.test.js]
    publishes: [mean]
    gates: []
execution_order: [add-mean]
```
EOF
printf 'node_modules/\n.lsf/\n' > .gitignore
git add -A; git commit -qm "stats fixture"
node --test >/dev/null && node scripts/lint.js
git rev-parse HEAD
