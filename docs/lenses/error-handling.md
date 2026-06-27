# Lens: Error Handling

Handler correctness, propagation, consistency, and taxonomy. [General](general.md) *detects* gaps; this lens *analyses* how errors should be handled.

**Tool support**: [phronithm:static-analysis](../../skills/static-analysis/SKILL.md) can detect unchecked return values, broad exception catches, resource leaks. Run with `--format=json`, filter for `error-handling` category.

## Concerns

### Coverage

Given a flagged gap (from [general](general.md) or [phronithm:static-analysis](../../skills/static-analysis/SKILL.md)), determine the full set of failure modes. Are all modes addressed? Common blindspots: transient vs permanent failures treated identically, timeout not distinguished from refusal, partial success not handled.

### Handler correctness

- Does the handler actually fix the problem, or log and continue with corrupt state?
- Error type narrowing: broad catches (`Exception`, `error`) when a specific type is appropriate swallow errors that should propagate.
- Re-raising and wrapping: is context preserved? Can the caller distinguish failure modes?
- Resource cleanup on error paths. Broader resource management (lifetime, ownership, RAII preference) is [general](general.md); this concern is specifically about error paths bypassing cleanup.

### Consistency

- Similar operations should handle errors similarly.
- Error messages: actionable, with enough context to diagnose (operation, input, state)?
- Error codes and types: consistent taxonomy, and does this code follow it?

### Propagation

- Does the error reach a point where it can be handled meaningfully, or is it swallowed/logged-and-ignored/converted to a generic failure?
- Layered handling: at each layer, is the error at the right level of abstraction? A database constraint violation should become a domain error before reaching the API layer.
- Sentinel values (returning -1, null, empty string for error): flag. They conflate absence with failure.

## Red flags

- Empty catch/except blocks.
- Broad catches outside top-level handlers.
- Error handling that differs between code paths doing the same operation.
- TODOs in error handlers.
- Retry logic without backoff or attempt limits.
- Logging an error then continuing as if it didn't happen.
- Error messages that discard original context.
- Functions returning both result and error where callers only check one.
