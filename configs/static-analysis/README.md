# Static Analysis Default Configurations

Default tool configurations for the phronithm:static-analysis skill. These are used when project-specific configs are not found.

## Configuration Precedence

1. **Project-local configs** (`.mypy.ini`, `ruff.toml`, `.clang-tidy` in project root) - highest priority
2. **Skill-provided defaults** (these files) - used if no project config exists
3. **Tool defaults** - lowest priority

## Files

- `ruff.toml`: Python linting configuration for ruff
- `mypy.ini`: Python type checking configuration for mypy
- `spotbugs-include.xml`: Java static analysis filter for SpotBugs
- `.clang-tidy`: C/C++ analysis configuration for clang-tidy
- `gcc-flags.txt`: Warning flags for GCC/Clang compiler analysis

## Philosophy

These defaults are intentionally strict:
- Enable all available checks
- Treat code quality seriously
- Use exclusions mechanism for legitimate suppressions

Projects can override by creating their own config files in the project root.
