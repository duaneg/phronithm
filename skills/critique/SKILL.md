---
description: "Evaluate a technical artefact (code, design, skill instructions) against the critique framework axes. Direct single-pass assessment — lighter-weight than `phronithm:review`. Use when you want focused criticism of one artefact without a multi-phase workflow."
allowed-tools: Read, Grep, Glob, Skill
---

# Critique: Artefact Evaluation

Evaluate a technical artefact against the critique framework axes. Direct, single-pass assessment — not a multi-phase review workflow. For lens-based analysis or larger scope, use the phronithm:review skill instead.

## Interface

### Requires

- **artefact**: What to critique. A file list, diff/commit range, document path, or description of the artefact.
- **type** (optional): Artefact type — `code`, `design`, `skill`, `phronithm`, or `maths`. Determines which appendix to load. Inferred from the artefact when omitted. Disambiguation: for files in `skills/*/`, default to `skill`. For diffs and source files, default to `code`. For CLAUDE.md files, lens documents, and prompts, default to `phronithm`. For `.lean`, `.v`, `.agda` and other proof-assistant sources, and files in projects with mathematical content (Lean/Coq/Agda lakefile, formal proof structure), default to `maths`; for computational code in such projects (e.g. Julia files doing certificate extraction), apply both `code` and `maths`. For other standalone documents outside the skills directory, default to `design`. When genuinely ambiguous, apply multiple appendices — the axes are additive, not exclusive.

### Produces

- **findings**: List, each with:
  - location (file, line, section)
  - axis (which critique axis triggered the finding)
  - concern (what is wrong or risky)
  - severity (critical / significant / minor, per [critique](${CLAUDE_PLUGIN_ROOT}/docs/critique.md))
  - suggested-fix (if clear; otherwise states uncertainty)
- **summary**: Overall assessment — fundamentally sound with localised issues, or systemic problem?
- **axes-applied**: Which universal axes were checked and which type appendix was loaded.

### Consumes

- **static-analysis-findings** (from phronithm:static-analysis): For code artefacts, avoids duplicating mechanical issues already caught by tooling. When absent, all axes are applied without filtering.

### Complexity drivers

- **Artefact size**: Single function or short document → low. Large codebase region or multi-section design → high.
- **Domain familiarity**: Well-understood code or established project patterns → low. Unfamiliar domain or novel technology → high.
- **Type**: Skill documents are typically smaller but require skill-design domain knowledge to evaluate effectively.

## Workflow

1. **Scope**: Identify the artefact. Read it. Understand what it does before looking for what is wrong.

2. **Classify**: Determine the artefact type. Load the core critique template and the appropriate appendix:

   | Type | Appendix |
   |------|----------|
   | code | [critique](${CLAUDE_PLUGIN_ROOT}/docs/critique.md) + [critique-code](${CLAUDE_PLUGIN_ROOT}/docs/critique/critique-code.md) |
   | design | [critique](${CLAUDE_PLUGIN_ROOT}/docs/critique.md) + [critique-design](${CLAUDE_PLUGIN_ROOT}/docs/critique/critique-design.md) |
   | skill | [critique](${CLAUDE_PLUGIN_ROOT}/docs/critique.md) + [critique-skill](${CLAUDE_PLUGIN_ROOT}/docs/critique/critique-skill.md) + [critique-phronithm](${CLAUDE_PLUGIN_ROOT}/docs/critique/critique-phronithm.md) |
   | phronithm | [critique](${CLAUDE_PLUGIN_ROOT}/docs/critique.md) + [critique-phronithm](${CLAUDE_PLUGIN_ROOT}/docs/critique/critique-phronithm.md) |
   | maths | [critique](${CLAUDE_PLUGIN_ROOT}/docs/critique.md) + [critique-maths](${CLAUDE_PLUGIN_ROOT}/docs/critique/critique-maths.md) |

   When multiple appendices are loaded, apply all additional axes additively. For mathematical code (computational artefacts in a mathematical project), load all three: critique + critique-code + critique-maths — the same additive pattern as skill = critique + critique-skill + critique-phronithm.

   When static-analysis-findings are available for code artefacts, scan them before applying axes. Skip findings that are mechanical equivalents of static-analysis results (type errors, unused imports, null safety violations, formatting). Focus manual critique on judgement-heavy concerns: design, naming, abstraction quality, edge cases.

3. **Apply**: Work through each axis systematically — all 6 universal axes, then the type-specific additional axes. For each finding:
   - Identify the location precisely.
   - State the concern concretely — what fails, breaks, or misleads, not abstract principles.
   - Assess severity per the critique framework.
   - Suggest a fix if one is clear. If not, say so.

4. **Synthesise**: Deduplicate findings where the same site triggers multiple axes — consolidate into a single finding with the highest applicable severity. Prioritise by severity, then by locality (clustered findings may indicate a deeper structural problem). Step back: is the artefact fundamentally sound with localised issues, or is there a systemic problem?

   Verify axis coverage: each axis should yield at least one concrete finding or an explicit "no issues found". Findings must reference specific locations, not general impressions. Axis coverage is a floor, not a ceiling.

   **Quality check**: If all findings are minor and the artefact is non-trivial (>50 lines or multi-axis scope), revisit the adversarial axis before concluding — construct a concrete scenario where the artefact fails or produces a worse outcome than doing nothing. If no failure scenario can be constructed, state this explicitly.

Each critique pass is single-pass — after synthesis, output findings. There is no iteration within a pass. For large artefacts with multiple components, see Scaling.

## Commit discipline

Critique findings are not committed as code changes. If the critique produces fixes, those follow the commit discipline of the invoking skill (phronithm:feature, phronithm:diagnose, phronithm:refactor). When invoked standalone, implement fixes via a separate feature or refactor invocation.

## Scaling

For small artefacts (single function, short document), a single pass through steps 1–3 is sufficient — step 4 synthesis is unnecessary.

For large artefacts, split by component or section and critique each independently. After critiquing components, run a final synthesis pass across all component findings — look for patterns that span components, as recurring issues may indicate a systemic problem. Apply the "step back" assessment from step 4 at the whole-artefact level, not just per-component.

## Iterative use

When critique is run repeatedly on the same artefact — applying fixes between passes — the following guidance applies. At the start of each pass, state the current round number. If the user has not provided it, derive from the number of prior critique passes visible in conversation context; if unknown, ask before proceeding.

**Convergence profile**: Critique loops converge at different rates depending on artefact type and stakes. For complex phronithms (autonomous agent prompts, multi-axis skill documents) governing high-stakes behaviour, expect Significant or Critical findings through rounds 6–8, with Minor findings persisting through rounds 8–10; diminishing returns from round 11 onward. For code artefacts and simpler documents, convergence typically occurs earlier (rounds 4–6). After each round, the bar rises: only downgrade a finding's severity if a fix has been applied that partially addresses the concern — not merely because a later round has been reached.

**Stopping criteria**: Stop the loop when either condition is met:
- All remaining findings are Minor severity.
- The same finding (same axis + same location) recurs across two consecutive rounds *and* a targeted fix was applied between rounds (addressing that axis and location) — this indicates a genuine disagreement between the critique framework and the design intent, not an unresolved defect. Note the recurring finding explicitly rather than continuing to iterate. If no fix was applied, or the fix did not target this axis and location, the recurrence is an oversight — include it in the current round's findings with a note that it was not addressed in the prior round.

When the loop terminates, output a summary: all remaining findings (severity and location), and a convergence verdict — one of: "converged" (all Minor), "recurring disagreement on [finding]", or "stopped: non-convergence signal — [cause]".

**Non-convergence signal**: If Significant or Critical findings persist past round 8, treat this as a signal to stop. Two causes:
- (a) The artefact has systemic issues that require redesign, not incremental fixes. Stop the loop and report: "Persistent significant findings after 8 rounds suggest a structural problem. Consider redesigning [specific aspect] rather than continuing to iterate."
- (b) The critique framework is generating false positives for this artefact type. Signal: the same axes produce structurally identical findings across 2+ rounds without corresponding defects in the artefact. Flag this to the user — the framework may need calibration for this context.

**Human judgment cap**: Do not continue an iterative loop beyond round 12 without explicit user instruction to proceed. Before each round beyond round 6, report the current finding severity distribution and note diminishing returns explicitly. Ask the user explicitly whether to continue.

## Relationship to other skills

- **phronithm:review**: Review applies lens-based checklists through a multi-phase workflow. Critique applies the critique axes directly. They are complementary — review uses critique's severity scale, and the phronithm:review skill selectively applies the critique-skill appendix for skill file changes. Use critique for axis-based evaluation of a bounded artefact (a function, a class, a small diff, a design document). Use review when you want lens-based analysis of a larger scope, especially when specific lenses (error-handling, data-structures, IO) add value beyond the universal axes.
- **phronithm:feature**: The phronithm:feature skill loads and applies the critique docs directly during Phases 2, 4, and 5. This skill formalises that same framework as a standalone invocation, usable independently or by other skills.
- **phronithm:investigate**: The phronithm:investigate skill runs critique as its Settle/Review gate, in a separate context: `critique-design` on every load-bearing finding, plus `critique-maths` and the `numerical` lens when the finding is mathematical. A finding passes when critique returns no Critical or Significant findings.
