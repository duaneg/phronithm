---
description: "Apply the documentation prose style guide to a document or provided text — trims rationale restatement, design-history asides, and other verbosity patterns while preserving informative, load-bearing content. Use when tightening a document or block of text to house style, not for restructuring or fact-checking."
allowed-tools: Read, Edit, Write, Grep, Glob
---

# Edit: Prose Style Application

Apply the [docs-style](${CLAUDE_PLUGIN_ROOT}/docs/lenses/docs-style.md) lens to a document or provided text. Trims prose that violates its patterns while preserving everything informative, readable, and load-bearing. An editing pass, not a review — it fixes what phronithm:critique's critique-phronithm appendix would otherwise only flag.

## Interface

### Requires

- **artefact**: What to edit — a file path, a list of file paths, or literal text provided in the request.

### Produces

- **change log**: each edit made — location, the pattern it corrects (per the style guide's Examples section), and the exact before text and after text (or "cut entirely" with the exact removed span), quoted verbatim from the source. Verbatim quoting matters: the change log is what gets applied as an exact string replacement, whether by this skill directly or by the caller. If the before text recurs elsewhere in the artefact, include enough surrounding context (or a file:line-range) to make it uniquely locatable.
- **flagged**: passages that resembled a violation but were ambiguous enough to leave in place, with the reason.

## Workflow

1. **Load the guide**: Read [docs-style](${CLAUDE_PLUGIN_ROOT}/docs/lenses/docs-style.md) in full. Its five goals and Examples section are the only criteria applied — this skill does not add house-style rules beyond what the guide states.
2. **Read the artefact** in full before editing. Understand what it is for, so a trim doesn't remove load-bearing content mistaken for narrative.
3. **Scan** for each pattern in the guide's Examples section — rationale restatement, design-history asides, cross-reference justification, announcing honesty, explained metaphor — plus the general rules: unnecessary verbs/adverbs, and the Current goal's historical notes/past decisions/process narration.
4. **Edit**: apply the minimal cut for each violation. Some patterns reduce to a shorter sentence (see the guide's "after" examples); others cut entirely. Do not touch passages that don't match a named pattern. If Edit/Write are among your available tools, apply each edit to the file directly. If they are not, do not attempt to write — build the change log instead (see Report).
5. **Verify**: with Edit/Write, re-read the edited artefact; without them, re-read the change log's before-text against the original artefact. Either way, confirm each entry's before-text is accurate and its after-text preserves meaning. If a cut is ambiguous — could be scaffolding or could be genuinely load-bearing (the guide's Informative/Readable goals) — do not guess. Leave it in place and record it under `flagged` instead.
6. **Report**: the change log and any `flagged` items. Never reproduce the full edited file as a deliverable — quoted before/after snippets in the change log are the applicable unit of output, and full-file transcription in a subagent response is unreliable (observed corruption of characters like `<` and `>` in transit).

## Boundaries

- Prose economy only. Does not fact-check content, restructure sections, or change technical meaning.
- For skill files (`skills/*/SKILL.md`) and agent personas, style trims apply to the prose within sections, not to frontmatter, YAML fields, or required structure.

## Relationship to other skills

phronithm:critique's critique-phronithm appendix flags verbosity in skill and CLAUDE.md-style artefacts as findings; this skill fixes them directly instead of reporting them. Run phronithm:critique first when correctness or design also need evaluation — this skill only edits, it never assesses.

## Scaling

Single document → one pass through the workflow. Multiple files → apply the workflow per file, then a final read-through across all edited files to catch the same rationale duplicated in more than one place.
