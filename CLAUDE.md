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
- `docs/agent-protocols.md`: Shared protocols for autonomous agents (scope rules, stop conditions, security escalation, red-green test discipline, PR discipline).
- `docs/code-patterns.md`: Structural duplication patterns. Referenced by phronithm:refactor and phronithm:review lenses.
- `docs/investigation-loop.md`: Canonical, domain-general investigation micro-loop (observe → multi-hypothesis → cheapest-falsify-first → update → verify). Referenced (not re-described) by phronithm:diagnose, phronithm:feature, phronithm:investigate, and phronithm:persona-debug.
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
Subagent nesting: Subagents can spawn their own subagents (the Agent tool is available in subagent sessions). Policy (rationale in `docs/agent-protocols.md`): ordinary data-gathering, phronithm:critic, and verify subagents must not spawn further subagents — they return findings inline and leave delegation to the orchestrating session. Full-workflow orchestrators (phronithm:feature / phronithm:large-scale-feature teammates) are exempt: they may spawn the specific critique/review agents their skill prescribes, but must not recurse beyond that. Enforce this with an explicit prohibition in agent persona files and subagent spawn prompts, and invoke skills that need Agent (e.g. phronithm:critique for context separation) from the orchestrating session, not from a data-gathering subagent.
Subagent file writes: Subagents cannot reliably write files — sandbox write permissions are not inherited. Instruct subagents to return results inline in their response. For structured data, use JSON. The orchestrating session extracts the result and writes it to disk. Never instruct a subagent to write to a file path.
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
