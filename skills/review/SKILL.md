---
description: "Focused code review through one or more composable lenses (error-handling, complexity, style, comments, io, numerical, general, data-structures). Use when reviewing a diff or PR with specific concerns in mind, rather than open-ended evaluation."
allowed-tools: Read, Grep, Glob, Task
---

# Review: Focused Code Review

Examine code through one or more focused lenses. Each lens targets a specific class of defect or concern.

## Interface

### Requires

- **scope**: What to review. One of: a file list, a diff/commit range, or a description of the code region.
- **lenses** (optional): Which lenses to apply. Default: all applicable. Can be narrowed by the requester.

### Produces

- **findings**: List, each with:
  - location (file, line, function)
  - concern (what is wrong or risky)
  - consequence (why it matters — concrete, not abstract)
  - severity (critical / significant / minor)
  - suggested-fix (if clear; otherwise states uncertainty)
- **systemic-concerns**: Structural or design problems that span individual findings.
- **lenses-applied**: Which lenses were used and which were skipped, with rationale.

### Consumes

- **impact-map** (from phronithm:impact-analysis): Used in Phase 1 to scope the review. When available, the review focuses on mapped sites ordered by risk. When absent, the review scopes from the raw diff or file list.
- **static-analysis-findings** (from phronithm:static-analysis): Used in Phase 2 pre-scan. When available, mechanical issues are already identified and manual review focuses on judgement-heavy concerns.

### Signals

- **escalate → phronithm:concurrency**: When review identifies concurrent code, it should be analysed by the phronithm:concurrency skill rather than reviewed through a lens. Includes the identified code scope.

### Complexity drivers

- **Code volume**: <200 lines → low. 200–1000 → moderate. >1000 → high.
- **Domain complexity**: Standard CRUD → low. Concurrency, security boundaries, novel algorithms → high.
- **Number of lenses**: 1–2 → low. 3+ → moderate. All → high.
- **Coupling density**: Isolated module → low. Code with many cross-cutting concerns → high.
- **Finding count**: <5 critical/significant → low. 5–15 → moderate. >15 → high. When critical/significant findings reach 5+, the Phase 3 validation step uses parallel subagents.

## Workflow

### Phase 1: Scope

1. **Identify** the code under review — files, functions, change set, impacted code using phronithm:impact-analysis.
2. **Triage scope**: before selecting lenses, check whether the review is warranted at its current scope.
   - If the scope is a mechanically verifiable no-op (rename with no unresolved references, typo in a string literal, version bump, regenerated code matching its generator input), output an empty findings list with a lenses-applied entry noting the triage decision, and stop — the Phase 3 exit gate (including postmortem) does not apply. If uncertain whether a change is trivially correct, proceed with the review.
   - If concurrency escalation applies to the entire scope, escalate rather than partially reviewing.
   - If static-analysis findings were consumed and comprehensively cover the scope, note the mechanical concerns already addressed — Phase 2 can skip those.
3. **Select lenses**: choose from the lens catalogue below based on the code's characteristics. Default: all applicable lenses. The reviewer or requester may narrow the selection.
   - Code with concurrency → use the phronithm:concurrency skill instead. Concurrency requires diagnostic reasoning, not checklist review.
   - Diff includes LLM instruction artefacts → always apply the phronithmic critique: [critique](${CLAUDE_PLUGIN_ROOT}/docs/critique.md) + [critique-skill](${CLAUDE_PLUGIN_ROOT}/docs/critique/critique-skill.md) + [critique-phronithm](${CLAUDE_PLUGIN_ROOT}/docs/critique/critique-phronithm.md). Detection: files matching `agents/*.md`, `skills/*/SKILL.md`, `*protocol*.md`, or files containing system-prompt-like instruction patterns (imperative directives to an LLM, role assignments, behavioural constraints). This catches obedience failure modes, triggering clarity issues, and adversarial edge cases that standard code review lenses miss.
4. **Read** the code. Understand what it does before looking for what's wrong.

Exit criteria: Scope is defined. Lenses are selected. The code is understood.

### Phase 2: Examine

**Optional pre-scan**: Run phronithm:static-analysis with `--format=json` to gather automated findings. Filter by severity and integrate with manual review. Static analysis catches mechanical issues (null safety, type errors, resource leaks) that would otherwise consume manual review time. For languages not covered by the phronithm:static-analysis skill (TypeScript, Go, Rust, etc.), substitute the project's own type-check and lint commands — check `package.json` scripts, `Makefile`, or project docs for the equivalents (e.g. `npm run typecheck && npm run lint` for TypeScript).

For each selected lens:

1. **Load** the lens document from `docs/lenses/`.
2. **Scan** the code against the lens's concerns. Work systematically — function by function, module by module — not by jumping to what looks suspicious.
3. **Record** findings with:
   - Location (file, line, function).
   - What the concern is.
   - Why it matters (concrete consequence, not abstract principle).
   - Severity: Critical / Significant / Minor (per [critique](${CLAUDE_PLUGIN_ROOT}/docs/critique.md)).
   - Suggested fix, if one is clear. If not, say so.

**Suppress low-signal findings.** Do not record:
- Pre-existing issues outside the change scope that are not newly reachable or activated by the change (when reviewing a diff).
- Issues already covered by consumed static-analysis findings.
- Issues explicitly silenced in code (lint-ignore comments, `# noqa`, `@SuppressWarnings`, etc.) unless the suppression itself is the concern.
- Findings where you cannot articulate a concrete consequence — "this feels wrong" is not a finding.

Cross-cutting: if a finding from one lens is relevant to another, note it. Do not defer — the other lens may not be selected.

### Phase 3: Synthesise

1. **Deduplicate**: the same code site may appear under multiple lenses. Consolidate into a single finding with the highest applicable severity.
2. **Validate**: for each critical or significant finding, construct the concrete failure scenario. If you cannot produce one, downgrade to minor or discard. When validating 5+ findings, run validations in parallel using lightweight subagents — each receives only the finding and its code context, and must be told not to spawn further subagents.
3. **Prioritise**: order findings by severity, then by locality (clustered findings in one area may indicate a deeper structural problem).
4. **Assess**: step back. Is the code fundamentally sound with localised issues, or is there a systemic problem? Flag systemic concerns separately from individual findings.

Exit criteria: Findings are deduplicated, prioritised, any systemic concerns are flagged.

## Lens catalogue

Available lenses (in `docs/lenses/`):

- **[general](${CLAUDE_PLUGIN_ROOT}/docs/lenses/general.md)**: Correctness, clarity, naming, structure.
- **[data-structures](${CLAUDE_PLUGIN_ROOT}/docs/lenses/data-structures.md)**: Appropriate choice and use of data structures.
- **[error-handling](${CLAUDE_PLUGIN_ROOT}/docs/lenses/error-handling.md)**: Error propagation, consistency, and taxonomy.
- **[style](${CLAUDE_PLUGIN_ROOT}/docs/lenses/style.md)**: Craft, convention, and idiomatic language use.
- **[comments](${CLAUDE_PLUGIN_ROOT}/docs/lenses/comments.md)**: Inline and doc comment content. Natural language prose conventions.
- **[complexity-assessment](${CLAUDE_PLUGIN_ROOT}/docs/lenses/complexity-assessment.md)**: LoC, cyclomatic and cognitive complexity metrics.
- **[io](${CLAUDE_PLUGIN_ROOT}/docs/lenses/io.md)**: IO and filesystem operations. Failure modes, resource management, blocking behaviour, atomicity.
- **[numerical](${CLAUDE_PLUGIN_ROOT}/docs/lenses/numerical.md)**: Floating-point and numeric code. Accuracy, stability, special values, cross-platform reproducibility.

Planned lenses (not yet loadable):
- **ownership**: Ownership, lifetime, and resource management clarity.

Adding a lens: create a document in `docs/lenses/` following the lens format (see any existing lens for the structure). Add it to this list.

## Commit discipline

Review findings are not committed as code changes. If the review produces fixes, those follow the normal commit discipline for the relevant skill (phronithm:feature, phronithm:diagnose, phronithm:refactor).

## Scaling

For small changes (single function, few lines), a single pass through relevant lenses is sufficient — Phase 3 synthesis is unnecessary.

For large reviews, consider splitting by module or subsystem and reviewing each independently before synthesising.

For diffs exceeding ~500 lines, run review in a dedicated fresh session — the combination of diff, lens documents, and changed-file reads approaches context capacity before findings are written. Starting fresh avoids a mid-review overflow that forces a continuation session.
