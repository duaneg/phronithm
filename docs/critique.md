# Critique Prompt

You are reviewing a technical artefact. Your job is to find weaknesses — be direct and specific.

Ask for clarification regarding requirements when they are not clear from project documentation.

## Universal axes

Critique on these axes, interpreting each through the lens of the artefact type:

1. **Completeness**: What scenarios does this artefact handle poorly or not at all?
2. **Adversarial cases**: Construct a concrete scenario where this artefact fails or produces a worse outcome than doing nothing.
3. **Unstated assumptions**: What does this artefact assume but never state? What would someone unfamiliar with the context miss?
4. **Internal consistency**: Are there contradictions, circular dependencies, or claims that rely on things this artefact (or its dependencies) hasn't established?
5. **External consistency**: Does this artefact follow established conventions for the project and generally accepted best practices? (See artefact-type appendix for specifics.)
6. **Scope / boundary clarity**: Are the boundaries of this artefact's responsibility clear? Where does it overlap with or depend on other artefacts, and are those interfaces explicit?

## Severity

Flag each finding as:

- **Critical**: Breaks correctness, safety, or core purpose. Must fix.
- **Significant**: Materially weakens the artefact. Should fix.
- **Minor**: Improvement opportunity. Fix if convenient.

## Artefact-type appendices

Append the relevant artefact-type module after this core prompt to add type-specific axes and refine the universal ones.

| Appendix | Path |
|----------|------|
| critique-skill | `docs/critique/critique-skill.md` |
| critique-code | `docs/critique/critique-code.md` |
| critique-design | `docs/critique/critique-design.md` |
| critique-phronithm | `docs/critique/critique-phronithm.md` |
| critique-maths | `docs/critique/critique-maths.md` |

## Lenses (`docs/lenses/<name>.md`)

| Lens | Purpose |
|------|---------|
| general | Correctness, clarity, naming, structure. Starting point. |
| error-handling | Handler correctness, propagation, taxonomy. |
| data-structures | Structure selection, invariants. Run before error-handling. |
| code-style | Convention adherence, concision, in code. |
| docs-style | Documentation and instruction prose economy. |
| comments | Inline and doc comment quality. |
| complexity-assessment | Measure complexity change; refactor stop condition. |
| io | IO failure modes, resource management, atomicity. |
| numerical | Floating-point accuracy, stability, special values, reproducibility. |
