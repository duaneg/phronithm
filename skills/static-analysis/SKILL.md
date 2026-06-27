---
description: "Orchestrate language-specific static analysis tools (ruff/mypy for Python, spotbugs for Java, clang-tidy for C, others) to catch bugs before runtime. Use to lint or type-check a project, set up progressive strictness adoption, or triage existing analyser output."
allowed-tools: Read, Grep, Glob, Bash
---

# phronithm:static-analysis: Static Code Analysis

Run a project's static-analysis tools to catch mechanical defects before runtime, and emit normalised findings that `phronithm:review`, `phronithm:critique`, `phronithm:diagnose`, `phronithm:refactor`, and `phronithm:feature` consume — so their manual passes can skip what tooling already caught.

This is a thin orchestration layer over tools the model already knows how to drive. Its value is consistency: curated strict configs, a normalised finding shape, and a categorisation vocabulary shared with the skills that consume it.

## When to use

- Lint or type-check a file, directory, or whole project.
- Pre-scan before `phronithm:review` — mechanical issues caught here free manual review for judgement-heavy concerns.
- Prevention check after `phronithm:diagnose` — find sibling instances of a just-fixed bug.
- Safety net after `phronithm:refactor` — confirm no new issues were introduced.

## How

1. **Detect languages** in scope and select tools:
   - Python: `ruff check --output-format=json`, `mypy --show-column-numbers`
   - Java: `javac -Xlint:all`, `spotbugs -textui -xml:withMessages`
   - C/C++: `clang-tidy`, `gcc`/`clang -Wall -Wextra -Wpedantic -fsyntax-only`
   - For languages without a tool configured here (Go, Rust, TypeScript, …), substitute the project's own lint/type-check commands (`package.json` scripts, `Makefile`).

2. **Configs**: prefer the project's own (`ruff.toml`, `.mypy.ini`, `.clang-tidy`, …). Fall back to `${CLAUDE_PLUGIN_ROOT}/configs/static-analysis/*` when the project has none — see that directory's README for precedence and the curated flag sets.

3. **Run** with `LC_ALL=C` for stable output parsing. `clang-tidy` needs `compile_commands.json` (CMake: `-DCMAKE_EXPORT_COMPILE_COMMANDS=ON`; Make: `bear -- make`); if absent, say so and report reduced coverage rather than failing.

4. **Exclusions**: honour inline tool-native suppressions (`# noqa`, `@SuppressFBWarnings`, `// NOLINT`) and the hand-maintained `.claude/static-analysis-exclusions.yaml`. Skip matching findings and report how many active exclusions are past their `review_by` date. Schema in [static-analysis](${CLAUDE_PLUGIN_ROOT}/docs/static-analysis.md).

5. **Normalise** each surviving finding to `{file, line, column, rule:"tool:name", severity, category, message, fix?}` and group by file with a summary. Category vocabulary and the `--format=json` shape are in [static-analysis](${CLAUDE_PLUGIN_ROOT}/docs/static-analysis.md).

## Discipline

- **Suggest fixes, never auto-apply.** Offer a unified diff plus a one-line rationale only for mechanical transforms (add null check, `strcpy`→`strncpy` with bounds, try-with-resources, remove unused import/variable). The user applies.
- **Strict by default.** Manage noise through the exclusions file, not by relaxing tools.
- **Multi-language**: run all applicable tools; one tool failing doesn't block the others — report it skipped with a reason.
- **Findings are not failure.** Default exit is success; `--strict` exits non-zero when findings remain (CI gate). Exit non-zero only for skill errors (missing tools, invalid config) otherwise.

## Flags

`--strict` (non-zero exit if findings remain), `--warnings-as-errors`, `--errors-only`, `--language=<python|java|c|cpp>`, `--format=json`. Combinable; contradictory combinations are rejected.
