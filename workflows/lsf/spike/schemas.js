// Spike only: the stage schemas implement.js builds but does not export,
// extracted from its source between the schemas and prompts banners.
import { readFileSync, writeFileSync, mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', 'implement.js'), 'utf8');
const body = src.slice(src.indexOf('// ---------------------------------------------------------------- schemas'), src.indexOf('// ---------------------------------------------------------------- prompts'));
const names = [...body.matchAll(/^const ([A-Z_]+_SCHEMA) =/gm)].map((m) => m[1]);
const dir = mkdtempSync(join(tmpdir(), 'lsf-schemas-'));
const file = join(dir, 'schemas.mjs');
writeFileSync(file, `import { stageSchema, MERGE_RESULT_SCHEMA, DISTRESS_SCHEMA, INVALIDATION_SCHEMA, AMENDMENT_KINDS } from ${JSON.stringify(join(here, '..', 'schema.js'))};\n${body}\nexport { ${names.join(', ')} };\n`);
export const SCHEMAS = await import(file);
