# Code Patterns: Structural Duplication

Common patterns of structural duplication to look for when scanning code. Useful for refactoring (finding duplication to eliminate) and code review (spotting duplication as a smell).

Exact textual matches are easy; the harder and more valuable finds are blocks that differ only in names, constants, types, minor structural variations, or abstractable functionality.

## Patterns

- **Loops over similar domains**: Including streams, recursion, goto loops, etc. The body can often be called through a first-class function or equivalent.
- **Builder / configuration sequences**: Repeated chains of `.set(x).set(y).build()` with slight variations.
- **Structural mapping**: Repeated transform-and-assign blocks.
- **Guard clause sequences**: Repeated validation/early-return blocks at function entries.
- **Resource lifecycle**: Repeated open/try/use/finally/close patterns.
- **Dispatch tables**: Switch/if-else chains where each branch has the same shape but different values. Consider replacement with a data structure.

## Detection heuristics

- Search within a given language only — cross-language duplication is rarely actionable.
- Exclude generated code, vendored code, example code.
- Prioritise most frequently duplicated blocks.
- Always consider other files that might contain similar patterns.
- Duplication in test utility/helper code is a smell. Duplication in test cases themselves is often justified — clarity over DRY.
