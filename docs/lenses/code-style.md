# Lens: Code Style

Convention adherence, concision, consistency, and mechanical choices in code. Covers what automated formatters cannot.

**Defers to:** [general](general.md) for naming intent and structural clarity; [comments](comments.md) for comment content; [docs-style](docs-style.md) for prose economy in comments and documentation. Formatter-enforced concerns (line length, brace style, indentation width, import ordering, string quoting) are out of scope.

## Consistency

Be consistent. Priority: same file > project > language best practices.

Follow existing local conventions above any rules here. When local convention conflicts with a Principle below, flag it as a finding — follow local convention, but recommend a prerequisite refactoring. Significant inconsistency within a file or with the project may also warrant prerequisite refactoring. Refactoring decisions must be approved by the user.

## Naming

Mechanical naming conventions — casing, concision, convention adherence. Whether a name communicates the right *meaning* is [general](general.md).

- Follow standard language conventions for casing and prefixes. The rules below are general principles and should not contradict language best practices.
- Names should be as concise as possible without sacrificing clarity ([general](general.md)).
- No variable shadowing. Exception: conventional transient names (`i`, `j`, `_`) where short scope makes reuse obvious.

## Principles

- Concise without sacrificing clarity.
- Minimise nesting: [general](general.md) flags the problem; this lens owns the techniques — early return, first-class functions, extraction into wrapper functions.
- Explicit over implicit.
- One statement per line.

## Expressions

- Prefer simple, linear expressions. When an expression requires holding more than two or three operations in mind, break it into named intermediates or restructure as control flow.
- Ternaries: appropriate for simple value selection only. Use if/else when branches have side effects or when any part is non-trivial.
- Method chains should read top-to-bottom as a pipeline. Break into named steps if a chain requires intermediate explanation or branches.
- Avoid boolean parameters that silently alter behaviour. Prefer separate functions, named constants, or an options structure. (See also [general](general.md).)

## Parameters

- Order parameters consistently across functions with similar signatures. Place a single target parameter first.
- When multiple functions share parameters that tend to change together, extract them into structured data.
- Functions with many parameters (5+ perhaps, 7+ definitely) should take structured data instead.

## Whitespace

Whitespace communicates logical grouping. Formatter-enforced rules are out of scope.

- Use blank lines to separate logical groups of statements. Group related statements; separate unrelated ones.
- Blank line before line comments, except at the start of an indented block.
- Avoid vertical alignment across lines; it increases maintenance burden and diff noise.
