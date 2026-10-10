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
- `agents/<name>.md`: Plugin subagents (auto-discovered, namespaced `phronithm:<name>` on install). `phronithm:critic` runs the phronithm:critique skill as a read-only subagent. `phronithm:editor` runs the phronithm:edit skill, returning a change log inline. `phronithm:adversary` runs a requester-directed presumption-of-defect adversarial pass (not a critique-gate step), returning cited cases inline for the caller to adjudicate.
- `docs/lenses/`: Review lens documents loaded by the phronithm:review skill.
- `docs/critique.md`: Core critique template. Compose with a type appendix from `docs/critique/`: `critique-code`, `critique-design`, `critique-phronithm`, `critique-skill`, `critique-maths`.
- `docs/critique-gate.md`: How skills invoke `phronithm:critique` as a quality gate on their own output — spawn mechanics, pass bar, what a pass does and doesn't license, the (optional, requester-directed) adversarial pass, and iteration policy. Referenced by phronithm:feature, phronithm:large-scale-feature, phronithm:investigate, phronithm:pin-behaviour, and phronithm:review.
- `docs/subagent-protocol.md`: Nesting and file-write policy for Task/Agent subagents, shared across skills.
- `docs/code-patterns.md`: Structural duplication patterns. Referenced by phronithm:refactor and phronithm:review lenses.
- `docs/investigation-loop.md`: Canonical, domain-general investigation micro-loop (observe → multi-hypothesis → cheapest-falsify-first → update → verify). Referenced (not re-described) by phronithm:diagnose, phronithm:feature, phronithm:investigate, and phronithm:persona-debug.
- `docs/plan-graph.md`: The `yaml lsf-plan` and `yaml lsf-decisions` block formats (plan graph and decision log) the `lsf-implement` workflow reads, the `yaml lsf-gates` gate commands block only the Go runtime reads, and the YAML subset they are written in. Emitted by phronithm:breakdown; parsed by `workflows/lsf/graph.js` and `lsf-runtime/plan`.
- `docs/vcs.md`: Version-control operations and conventions in one file — named history-inspection operations (recent-changes, fix-history, churn, co-change, last-activity, …) and commit discipline. Skills reference sections by anchor.
- `docs/lsp/`: LSP integration guides. `integration.md` is the method-agnostic operation catalogue for phronithm:impact-analysis. Language-specific appendices: `typescript.md`, `java.md`.
- `configs/static-analysis/`: Default linter/type-checker configs (ruff, mypy, gcc flags, spotbugs, clang-tidy) used by the phronithm:static-analysis skill as fallbacks when target projects lack their own.
- `workflows/lsf-implement.js`: Generated Workflow-tool script (`phronithm:lsf-implement`). Never edit; run `node workflows/lsf/build.js` after changing the modules (a test fails when it is stale).
- `workflows/lsf/`: Source modules for the `lsf-implement` Workflow-tool script (design: `docs/design/lsf-workflow.md`). `schema.js` holds the shared stage-return schema, plan schema, and signal/escalation shapes; `graph.js` holds the YAML-subset parser and the plan-block/decision-log loader; `scheduling.js` holds plan validation, ready-set, merge order, signal mapping, the stop rule, and the digest; `implement.js` holds the lsf-implement orchestrator (`run(runtime)`: stage prompts, schemas, critique loop, wave loop); `build.js` inlines them into the script. Dependency-free; these files are the source of truth. Tests: `cd workflows/lsf && node --test test/`.
- `lsf-runtime/`: Go module (stdlib only) for the S2 host runtime and the orchestrator port (design: `docs/design/lsf-runtime.md`). `runtime/` (package `lsfrt`) runs stages as headless `claude -p` calls: roles from config, retry classes (a 401 stops the run, `auth-failed`), journal and resume, caps, signals, orphan reaping, per-call clones (their git isolated from global config), fetch-back and removal, uncommitted-at-return detection, a plan's declared gates in their own containers on a clean plain-clone checkout (`RunGates`), the pinned `hosts` (`PinHosts`/`Hosts`) and the egress allow-list proxy they start (`egress.go`), the credential proxy (`proxy.go`, opt-in by config; `Proxy()`), and the move of a merged unit's declared `retains` to the plan's store before cleanup (`retain.go`: `MoveRetained`, `CheckStore`). `plan/` is the port of `schema.js`, `graph.js` and `scheduling.js` (divergences D1–D8 in `plan/doc.go`), plus the Go-only gate-block parser (`gates.go`); `orchestrator/` is the port of `implement.js` (`Run`; `Body` for `lsfrt.Run`; divergences D9 onward in `orchestrator/doc.go`; load, decision log, ledger and completion report are git code steps in `stages.go` and `git.go`, merge and cleanup in `merge.go`, `bookkeeping.go` and `cleanup.go`, the integration fast-forwards in `ff.go`), with `orchestrator/orchtest` its stub-runtime test harness; `cli/` is the `lsf-runtime` command (launch, status, watch, pause, stop, resume, validate, config init, version, and the hidden `drive`; its plugin defaults embedded from `cli/defaults.json`), its tests running the test binary as the CLI, the driver and a fake `claude`; `lsf-runtime/lsf-runtime` is the launcher the skills run (`LSF_RUNTIME_BIN`, else a working-tree build into `.build/`, else the release pinned in `dist/release`, downloaded and sha256-checked; its failures exit 125); `dist/` holds the pin, `pin.sh` and `check-pin.sh` (release order and drift check: `docs/design/lsf-runtime.md` Distribution) and the launcher's tests; `.github/workflows/lsf-runtime.yml` is CI (tests on amd64 and arm64, static release builds, the release on an `lsf-runtime-v*` tag, the pin-drift check on `main`); `internal/` holds the git environment and clone checks (`gitenv`: clone-isolated or not, `CloneGitDir`, `CloneGitlinks`) and the tests' container settings (`testcfg`). `tools/graphparity` compares `plan`, and `tools/promptparity` compares `orchestrator` (every prompt, option, log and return over 55 scenarios, under node coverage), with the frozen JS via `node` (`go run ./tools/<name>`; `go test` runs each when node is installed and skips it otherwise, unless `GRAPHPARITY_REQUIRE_NODE=1` / `PROMPTPARITY_REQUIRE_NODE=1`). `tools/fixturerun` runs the orchestrator end to end on the spike fixture against the pinned `claude` (paid; not part of `go test`). `testdata/claude` is the stub `claude` and `testdata/podman` the stub `podman` (a container's command run on the host); live container tests run with podman and `LSF_TEST_IMAGE` (`LSF_LIVE_REQUIRE=1` makes a skip fail); `testdata/streams/` holds recorded 2.1.289 streams, one 2.1.294 stream (`auth-401`) and six 2.1.294 re-recordings under `2.1.294/`. Tests: `cd lsf-runtime && go test -race ./...`.
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
