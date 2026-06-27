# phronithm — workflow skills for LLM-assisted programming

Structured workflows for LLM agents, focussed on programming.

## Skills

- **phronithm:breakdown** — work decomposition and execution planning with dependency ordering and parallelism analysis
- **phronithm:concurrency** — systematic concurrency analysis and model verification
- **phronithm:critique** — direct evaluation of any artefact against the critique framework axes
- **phronithm:diagnose** — systematic fault isolation: triage, investigate, reproduce, diagnose, fix
- **phronithm:feature** — end-to-end feature delivery: brainstorm, design, plan, implement, review
- **phronithm:impact-analysis** — maps direct, transitive, and implicit effects of a change
- **phronithm:refactor** — iterative deduplication and structural decomposition
- **phronithm:review** — focused code review through composable lenses (error-handling, style, IO, numerical, data structures, complexity, general)

See `docs/skill-catalogue.md` for the full inventory.

## Installation

This repo ships as a Claude Code plugin. Add it as a marketplace, then install:

```
/plugin marketplace add duaneg/phronithm
/plugin install phronithm
```

Skills become available as namespaced slash commands: `/phronithm:feature`, `/phronithm:refactor`, etc.

For local development (editing skills in place):

```
claude --plugin-dir /path/to/this/repo
```

## Structure

- `skills/<name>/SKILL.md` — workflow skills (slash commands)
- `agents/` — plugin subagents (`phronithm:critic` runs the phronithm:critique skill as a read-only subagent)
- `docs/` — reference material, design documents, critique templates, lens definitions
- `docs/lenses/` — review lens documents loaded by the phronithm:review skill
- `docs/critique/` — critique appendices loaded by the phronithm:critique skill
- `docs/lsp/` — LSP integration guides used by phronithm:impact-analysis
- `configs/static-analysis/` — default linter/type-checker configs
- `tools/` — Python helpers invoked by skills at runtime
- `.claude-plugin/plugin.json` — plugin manifest
