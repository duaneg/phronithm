#!/usr/bin/env python3
"""Map symbols to their consumers across a source tree.

Symbols are given as positional arguments, or read one per line from stdin.
Consumer files are classified as production or test based on filename pattern.

Typical usage (TypeScript):
    tools/extract-ts-exports.py src/constants.ts | \\
        tools/map-symbol-consumers.py --src-dir src/ --exclude src/constants.ts
"""

import argparse
import os
import re
import subprocess
import sys


def find_consumers(symbol: str, src_dir: str, include: str, exclude: str | None) -> tuple[list[str], list[str]]:
    """Return (production_files, test_files) that reference symbol."""
    cmd = ['grep', '-rlw', symbol, '--include', include, src_dir]
    try:
        result = subprocess.run(cmd, capture_output=True, text=True)
    except FileNotFoundError:
        print('error: grep not found', file=sys.stderr)
        sys.exit(1)

    prod, test = [], []
    for path in result.stdout.splitlines():
        path = path.strip()
        if not path:
            continue
        if exclude and os.path.abspath(path) == os.path.abspath(exclude):
            continue
        name = os.path.basename(path)
        if re.search(r'\.(test|spec)\.[^.]+$', name):
            test.append(name)
        else:
            prod.append(name)
    return sorted(prod), sorted(test)


def main() -> None:
    parser = argparse.ArgumentParser(
        description='Map symbols to consumers across a source tree.',
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument('symbols', nargs='*', help='Symbols to search for (reads stdin if omitted)')
    parser.add_argument('--src-dir', default='src/', help='Directory to search (default: src/)')
    parser.add_argument('--include', default='*.ts', help='File glob to search (default: *.ts)')
    parser.add_argument('--exclude', metavar='FILE', help='Source file to exclude from results')
    args = parser.parse_args()

    src_dir = args.src_dir
    if not os.path.exists(src_dir):
        print(f'error: --src-dir does not exist: {src_dir}', file=sys.stderr)
        sys.exit(1)
    if not os.path.isdir(src_dir):
        print(f'error: --src-dir is not a directory: {src_dir}', file=sys.stderr)
        sys.exit(1)

    if args.symbols:
        symbols = args.symbols
    else:
        symbols = [line.rstrip('\n') for line in sys.stdin if line.strip()]

    if not symbols:
        print('error: no symbols provided', file=sys.stderr)
        sys.exit(1)

    rows = []
    for symbol in symbols:
        prod, test = find_consumers(symbol, args.src_dir, args.include, args.exclude)
        rows.append((symbol, prod, test))

    # Column widths
    sym_width = max(len(r[0]) for r in rows)
    sym_width = max(sym_width, 6)  # minimum header width

    header = f"{'Symbol':<{sym_width}}  {'Prod':>4}  {'Test':>4}  Production consumers"
    print(header)
    print('-' * len(header))

    multi = single = dead = 0
    for symbol, prod, test in rows:
        prod_count = len(prod)
        test_count = len(test)
        if prod_count == 0 and test_count == 0:
            consumers_str = '(dead)'
            dead += 1
        elif prod_count == 0:
            consumers_str = '(test only)'
            single += 1
        else:
            consumers_str = ', '.join(prod)
            if prod_count == 1:
                single += 1
            else:
                multi += 1
        print(f'{symbol:<{sym_width}}  {prod_count:>4}  {test_count:>4}  {consumers_str}')

    print()
    print(f'{len(rows)} symbols: {multi} multi-consumer, {single} single-consumer (co-locate candidates), {dead} dead')


if __name__ == '__main__':
    main()
