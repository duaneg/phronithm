// Spike only: a validator for the JSON Schema subset the stage schemas use
// (type, properties, required, items, enum). Returns a list of errors.
export function validate(schema, value, path = '$') {
  const errs = [];
  const t = schema.type;
  const is = {
    object: (v) => v !== null && typeof v === 'object' && !Array.isArray(v),
    array: Array.isArray,
    string: (v) => typeof v === 'string',
    boolean: (v) => typeof v === 'boolean',
    integer: Number.isInteger,
    number: (v) => typeof v === 'number',
  };
  if (t && !is[t](value)) return [`${path}: expected ${t}, got ${JSON.stringify(value)?.slice(0, 40)}`];
  if (schema.enum && !schema.enum.includes(value)) errs.push(`${path}: ${JSON.stringify(value)} not in enum`);
  if (t === 'object') {
    for (const k of schema.required ?? []) if (!(k in value)) errs.push(`${path}.${k}: required`);
    for (const [k, s] of Object.entries(schema.properties ?? {})) if (k in value) errs.push(...validate(s, value[k], `${path}.${k}`));
  }
  if (t === 'array' && schema.items) value.forEach((v, i) => errs.push(...validate(schema.items, v, `${path}[${i}]`)));
  return errs;
}
