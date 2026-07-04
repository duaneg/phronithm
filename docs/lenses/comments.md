# Lens: Comments

Inline comments, doc comments, and prose conventions.

**Out of scope:** file-level boilerplate (licence headers, copyright notices).

**Defers to:** [code-style](code-style.md) for formatter-enforced conventions; [docs-style](docs-style.md) for prose economy; [general](general.md) for naming and structural clarity that removes the need for comments.

## Prose Conventions

Applies [docs-style](docs-style.md), plus comment-specific nuances:

- Longer passages (design rationale, trade-off discussion) may express more personality than terse inline comments.
- Spell-check all prose. Domain terms and identifiers are not typos but should be consistent and defined on first use where non-obvious.
- Higher-level context first, then implementation details.

## Inline Comments

Minimal. If a comment restates the code, delete it. If code cannot be made obvious without a comment, the comment is load-bearing — its absence is a bug. A comment that contradicts the code is worse than no comment; stale comments are bugs.

- **Why, not what.** Intent, constraints, non-obvious invariants. Do not narrate control flow.
- **Distant coupling and implicit invariants** (flagged by [general](general.md) lens): prefer eliminating the dependency (e.g. retest a condition locally rather than relying on a guarantee established far away) over commenting it. A comment is appropriate when simplification would be too verbose, harm readability, or impose a cost in a hot path.
- **External references.** Link RFCs, specs, tickets for non-trivial implementations.
- **Caveats and workarounds.** Reference the upstream issue where possible.
- **No commented-out code.** Version control remembers. Exception: a brief commented alternative explaining why the current approach was chosen, if non-obvious.
- **Verify checkable claims.** Complexity assertions, behavioural descriptions, API/library references — verify against the implementation. Flag claims that cannot be verified from available context.

### TODO / FIXME / HACK

- Format: `TODO(who or reference): description`. Every marker must be traceable.
- HACK markers must explain what is wrong and what a proper fix looks like.

## Doc Comments

Public interface contract — audience is callers, not maintainers.

- **Public symbols need doc comments.** Omit only when name and signature make the contract completely obvious. Internal symbols: only when intent is non-obvious.
- **Lead with a single-sentence summary** meaningful in isolation (indexes, tooltips, search results).
- **Document the contract, not the implementation.** Parameters, return values, error conditions, preconditions, postconditions, side effects. Omit internal details the caller cannot rely on.
- **Examples earn their keep.** Include when the calling pattern is non-obvious. Remove examples that restate the signature.
- Tooling-specific formatting (Javadoc, rustdoc, JSDoc, etc.) is [code-style](code-style.md).
