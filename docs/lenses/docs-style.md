# Lens: Docs Style

Documentation and instruction prose — SKILL.md bodies, CLAUDE.md, design docs, prompts, and other free-form text loaded as context. For code comments, [comments](comments.md) layers comment-specific conventions on top of this baseline.

Documentation prose should be:
- Written for: intelligent context-aware readers, human and LLM, familiar with the state of the art.
- Informative: provide readers with accurate and relevant information.
- Readable: complex concepts require careful explanation, let them breathe.
- Current: historical notes, past decisions and corrections, things which are not, narration of process should all be cut. They live in git history or dedicated logs.
- Concise: anything not in service to these goals should be cut. Anything required stays, but keep it tight.

## Patterns to avoid and correct

Unnecessary verbs and adverbs in general, beyond the named patterns below.

### Examples

**Rationale restatement** — reasoning already established elsewhere, repeated locally.
- Before: "This step exists because SP-1.3 requires a single authority for values, and re-deriving it here would create two sources of truth (see shared-protocols.md for the full hazard)."
- After: "Per SP-1.3, this is the single authority for values."

**Design-history aside** — why the current approach replaced a former one.
- Before: "(Note: earlier drafts used the existence/mtime signal to detect completion, but this was replaced because it produced false positives across worktrees.)"
- After: cut entirely.

**Cross-reference justification** — explaining a design choice that doesn't change the step.
- Before: "(This is a commitment gate, not a write-safety gate — the planner writes no target; the write gate belongs to pin-behaviour, downstream.)"
- After: "This is a commitment gate; pin-behaviour owns the write-safety gate."

**Announcing honesty** — asserting a quality that is already the default assumption absent contrary evidence.
- Before: "To be completely transparent, this analysis is accurate and reflects the actual state of the code."
- After: cut entirely.

**Explained metaphor** — unpacking a figure of speech the reader already follows.
- Before: "This module is load-bearing — meaning that removing it would cause many other parts of the system to fail, much like a beam holding up a building."
- After: "This module is load-bearing."
