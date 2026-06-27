#!/usr/bin/env python3
"""Extract exported symbol names from a TypeScript file, one per line."""

import re
import sys


# Matches: export const/let/var/function/class/type/interface/enum/abstract class Name
_DECL_RE = re.compile(
    r'^export\s+(?:declare\s+)?(?:default\s+)?'
    r'(?:abstract\s+)?(?:const|let|var|function\*?|class|type|interface|enum)\s+'
    r'([A-Za-z_$][A-Za-z0-9_$]*)'
)

# Matches: export { Foo, Bar as Baz, ... }
# Captures the full brace content; symbols parsed separately.
_NAMED_RE = re.compile(r'^export\s*\{([^}]+)\}')

# Matches: export * as Foo from '...'
_NAMESPACE_RE = re.compile(r'^export\s+\*\s+as\s+([A-Za-z_$][A-Za-z0-9_$]*)')


def _parse_named(brace_content: str) -> list[str]:
    """Return exported names from a brace list, respecting 'X as Y' aliases."""
    symbols = []
    for item in brace_content.split(','):
        item = item.strip()
        if not item:
            continue
        # 'LocalName as ExportedName' — we want the exported name
        parts = re.split(r'\s+as\s+', item)
        name = parts[-1].strip()
        if re.match(r'^[A-Za-z_$][A-Za-z0-9_$]*$', name):
            symbols.append(name)
    return symbols


def extract(path: str) -> list[str]:
    try:
        with open(path, encoding='utf-8') as f:
            source = f.read()
    except OSError as e:
        print(f'error: {e}', file=sys.stderr)
        sys.exit(1)

    symbols = []
    # Normalise line continuations and collapse multiline export {} blocks
    # so the regex can match them on a single logical line.
    lines = source.splitlines()
    logical_lines = []
    buf = ''
    for line in lines:
        stripped = line.strip()
        if buf:
            buf += ' ' + stripped
            if '}' in stripped:
                logical_lines.append(buf)
                buf = ''
        elif stripped.startswith('export') and '{' in stripped and '}' not in stripped:
            buf = stripped
        else:
            logical_lines.append(stripped)

    for line in logical_lines:
        m = _DECL_RE.match(line)
        if m:
            symbols.append(m.group(1))
            continue
        m = _NAMED_RE.match(line)
        if m:
            symbols.extend(_parse_named(m.group(1)))
            continue
        m = _NAMESPACE_RE.match(line)
        if m:
            symbols.append(m.group(1))

    # Deduplicate, preserving order
    seen = set()
    result = []
    for s in symbols:
        if s not in seen:
            seen.add(s)
            result.append(s)
    return result


def main() -> None:
    if len(sys.argv) != 2 or sys.argv[1] in ('-h', '--help'):
        print('usage: extract-ts-exports.py <source-file>', file=sys.stderr)
        sys.exit(0 if sys.argv[1:] == ['--help'] else 1)

    for symbol in extract(sys.argv[1]):
        print(symbol)


if __name__ == '__main__':
    main()
