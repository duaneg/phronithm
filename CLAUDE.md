# Project Overview

A Claude Code plugin (`phronithm`) shipping workflow skills for LLM-assisted programming.

# Communication style and behaviour

- Be concise and professional.
- Challenge unevidenced assumptions and raise potentially overlooked issues and alternatives.
- Ask for clarification if something doesn't make sense, seems impractical, or unwise. Do not guess in the face of ambiguity.
- Do not rationalise user decisions or invent reasons or motivations beyond any explicitly stated.
- Use New Zealand English spelling and conventions.
- Whakamahia tika ngā tohutō i te reo Māori.

# Structure

- `.claude-plugin/plugin.json`: Plugin manifest. `.claude-plugin/marketplace.json`: Marketplace metadata.
- `skills/<name>/SKILL.md`: Workflow skills. Each skill is a directory with a `SKILL.md` file. See `docs/skill-catalogue.md` for the full inventory and design decisions.
- `agents/<name>.md`: Plugin subagents (auto-discovered, namespaced `phronithm:<name>` on install). `phronithm:critic` runs the phronithm:critique skill as a read-only subagent. `phronithm:editor` runs the phronithm:edit skill, returning a change log inline.
- `docs/lenses/`: Review lens documents loaded by the phronithm:review skill.
- `docs/critique.md`: Core critique template. Compose with a type appendix from `docs/critique/`: `critique-code`, `critique-design`, `critique-phronithm`, `critique-skill`, `critique-maths`.
- `docs/critique-gate.md`: How skills invoke `phronithm:critique` as a quality gate on their own output — spawn mechanics, model pin, pass bar. Referenced by phronithm:feature, phronithm:large-scale-feature, phronithm:investigate, and phronithm:pin-behaviour.
- `docs/subagent-protocol.md`: Nesting and file-write policy for Task/Agent subagents, shared across skills.
- `docs/code-patterns.md`: Structural duplication patterns. Referenced by phronithm:refactor and phronithm:review lenses.
- `docs/investigation-loop.md`: Canonical, domain-general investigation micro-loop (observe → multi-hypothesis → cheapest-falsify-first → update → verify). Referenced (not re-described) by phronithm:diagnose, phronithm:feature, phronithm:investigate, and phronithm:persona-debug.
- `docs/vcs.md`: Version-control operations and conventions in one file — named history-inspection operations (recent-changes, fix-history, churn, co-change, last-activity, …) and commit discipline. Skills reference sections by anchor.
- `docs/lsp/`: LSP integration guides. `integration.md` is the method-agnostic operation catalogue for phronithm:impact-analysis. Language-specific appendices: `typescript.md`, `java.md`.
- `configs/static-analysis/`: Default linter/type-checker configs (ruff, mypy, gcc flags, spotbugs, clang-tidy) used by the phronithm:static-analysis skill as fallbacks when target projects lack their own.
- `tools/`: Python helpers (stdlib only) invoked at runtime by phronithm:impact-analysis and phronithm:refactor skills.
  - `extract-ts-exports.py`: Extract exported symbol names from a TypeScript file. Usage: `python tools/extract-ts-exports.py <source-file>`
  - `map-symbol-consumers.py`: Map consumers of exported symbols across files.

# Conventions

File naming: skill and doc files use lowercase-with-dashes. Standard project files (CLAUDE.md, README.md) use their conventional names.
Document authorship: files record current state only. Completed items are deleted, not struck through or annotated as done. Version control tracks history; files do not.
Skill structure:
- Each skill lives in `skills/<name>/SKILL.md`. The directory name becomes the slash command (namespaced as `/phronithm:<name>` when installed via the plugin).
- The phronithm:refactor skill is the reference implementation.
- Every skill must have YAML frontmatter (`---` delimiters) with `allowed-tools` listing all tools needed during execution. Without this, permission prompts interrupt mid-workflow.
- Cross-references to docs/tools/configs in skill bodies use `${CLAUDE_PLUGIN_ROOT}/...` (substituted by Claude Code at load time). Relative paths (`../docs/...`) are not portable across install locations.
Subagent policy and the critique gate: see `docs/subagent-protocol.md` and `docs/critique-gate.md`. Skills reference sections by anchor, per the `docs/vcs.md` pattern.
Cross-references between skills and agents: use the fully-qualified `phronithm:<name>` form (e.g. `phronithm:review`, `phronithm:critic`), matching how they are invoked once installed. This applies to prose, slash commands, backticked mentions, and Markdown link text. Do not qualify review lens names (`error-handling`, `general`, `numerical`, etc. — they are documents, not skills), literal file paths and directory names, or YAML frontmatter `name:` fields.
Scope: language and target agnostic unless specified otherwise.

# Tool usage

## Bulk file edits — prefer scripts over manual edits

When applying a repeated mechanical transform across many files or occurrences, write a short script (python/jq via Bash) instead of individual Read+Edit calls. Scripts are more reliable, more consistent, and more auditable. Reserve manual Edit for changes where each site needs different reasoning.

# Distribution

The repo ships as a Claude Code plugin via `.claude-plugin/plugin.json`. Users install with:

    /plugin marketplace add duaneg/phronithm
    /plugin install phronithm

Slash commands are namespaced: `/phronithm:refactor`, `/phronithm:review`, etc.

For local development against an editable copy:

    claude --plugin-dir /path/to/this/repo

After adding a new skill, restart any active Claude Code session — the Skill tool does not pick up newly installed skills mid-session.

# Issue Tracker

GitHub — repo `duaneg/phronithm`.
