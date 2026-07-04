# Lens: General

Correctness, clarity, naming, structure. Flags but does not deeply analyse specialist concerns — defer to [error-handling](error-handling.md), [data-structures](data-structures.md), [code-style](code-style.md). If reviewing with only this lens, treat flagged concerns as open findings.

**Assumes sequential code.** Escalate to [phronithm:concurrency](../../skills/concurrency/SKILL.md) if shared mutable state, threads/tasks, or synchronisation primitives are present.

**Pre-scan with [phronithm:static-analysis](../../skills/static-analysis/SKILL.md)** for mechanical concerns (dead code, unused variables, type errors, unreachable branches).

## Concerns

### Correctness

- Does the code do what it claims? Trace key paths manually.
- Off-by-one errors, boundary conditions, null/nil/undefined handling.
- Input assumptions: documented and enforced, or silent and fragile?
- Arithmetic: overflow, underflow, division by zero, floating-point comparison.
- Boolean logic: De Morgan violations, short-circuit side effects, negation errors.
- Type safety: wrong casts, implicit conversions, generic type erasure, type confusion at serialisation boundaries.
- Unhandled failures and partial failure (state left inconsistent). Flag gaps; handler and rollback analysis is [error-handling](error-handling.md).
- Magic numbers and strings without explanation.

### Resource management

- Acquired and released on all paths including error paths? Prefer language-level guarantees (RAII, `using`, `with`, `defer`) over manual cleanup.
- Early returns, breaks, and exceptions that bypass cleanup.
- Lifetime: resources that outlive usefulness — unbounded collections, connections held across unrelated operations, locks held too long.
- Ownership: clear who releases? Explicit on transfer?

### Clarity

- Can a reader understand intent without cross-referencing unrelated files?
- Distant coupling and implicit invariants: does the code rely on guarantees or conventions established elsewhere, making it unreadable in isolation?
- Control flow: obvious which path executes under which conditions? Deep nesting (>3 levels) is a red flag; reduction techniques are [code-style](code-style.md).
- Abstraction level: single level per function, not mixed orchestration and detail.
- Expression and statement density is [code-style](code-style.md).

### Naming

Whether names communicate the right meaning. Mechanical conventions (casing, prefix/suffix rules, concision, shadowing) are [code-style](code-style.md).

- Describe *what*, not *how*. Implementation details in names couple callers to internals.
- Consistent vocabulary: one term per concept.
- Scope-appropriate length: short for loop variables, descriptive for module-level.
- Booleans read as predicates: `is_valid`, `has_permission`.

### Structure

- Function length: long functions usually do too much, but splitting needs a reason.
- Cohesion: do module/class elements belong together? Methods that don't share state suggest a namespace, not a class.
- Coupling: depending on concrete implementations when an interface would do, or reaching through layers.
- Duplication: structural repetition within or across functions. See [code-patterns](../code-patterns.md).
- Boolean parameters that switch behaviour — usually two functions in a trenchcoat.
- Single responsibility: if you can't describe what it does without "and", it does too much.
- Mutable state shared across concerns without clear ownership.
