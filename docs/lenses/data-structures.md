# Lens: Data Structures

Late changes to data structures are costly.

**Assumes sequential code.** Flag concurrency-relevant issues (shared mutable collections, missing synchronisation) for [phronithm:concurrency](../../skills/concurrency/SKILL.md), but complete this review first — selection, invariants, and representation apply regardless.

**Tool support**: [phronithm:static-analysis](../../skills/static-analysis/SKILL.md) pre-scan can detect mutable map keys, type mismatches in generic collections, unused collections.

**Out of scope**: ownership/lifetime deep analysis (planned lens), structural duplication ([code-patterns](../code-patterns.md)), error propagation ([error-handling](error-handling.md)), system design (versioning, migration, schema governance), algorithm *design*. [general](general.md) flags gross mismatches; this lens provides deeper analysis. The algorithmic complexity implied by a data structure choice is in scope: this lens covers the choice and its consequences.

## Concerns

### Selection

- **Favour immutability.** Default to immutable; require justification for mutability. Enforce as strictly as the language allows (`const`, `final`, `frozen`, `readonly`, deep freeze). Shallow immutability is insufficient — flag it. **Red flag:** collections populated once then stored as mutable types.
- Is the structure appropriate for the access pattern? Common mismatches:
  - List where a set/map would give O(1) lookup.
  - Map where a sorted structure would support range queries.
  - Array of pairs where a struct/record would give named access.
  - Mutable collection where immutable would prevent bugs.
- **Red flag:** `List<Object>` or equivalent when a typed structure exists.
- Growth: will performance hold at expected scale? **Red flag:** O(n) operations inside loops over the same collection (hidden quadratic).
- Concurrency: if accessed from multiple threads/tasks, flag. Note obviously unsafe choices (e.g. unsynchronised mutable map); defer full analysis to [phronithm:concurrency](../../skills/concurrency/SKILL.md).

### Invariants

- Does the structure maintain the invariants the code depends on? (Sorted order after mutation, uniqueness, non-emptiness.)
- Nullability: `List<User?>` ≠ `List<User>`; callers rarely account for the difference.
- Equality and hashing: correct for types used as map keys / set elements? **Red flag:** mutable key types, broken/missing contracts — silent data loss.
- Invariants enforced at the type level (best), at construction (acceptable), or by convention (fragile)?

### Usage

- Iteration: natural for the data structure, or fighting it? (Indexed access into a linked list, repeated lookup in an unsorted array.)
- Mutation during iteration: flag it.
- Ownership and aliasing: who owns it? Unexpected aliases (two references to the same mutable collection)? Defensive copies are a smell but sometimes necessary.
- Lifetime: appropriate? **Red flag:** unbounded growth without eviction or rotation.
- **Red flag:** parallel arrays — use a struct/record. See [code-patterns](../code-patterns.md).

### Representation

- Right level of abstraction? Raw maps/arrays as domain objects obscure intent and forgo type checking.
- Deeply nested maps/dicts are hard to query, validate, and evolve — usually a proper type is needed.
- Serialisation boundaries: explicit conversion, not implicit structural coupling. **Red flag:** internal domain types exposed directly across API or serialisation boundaries.

### Shared and persistent data

Data exposed across boundaries (module APIs, IPC, shared memory, message passing) or persisted (database rows, files, serialised state). Synchronisation is [phronithm:concurrency](../../skills/concurrency/SKILL.md)'s concern; this section covers whether the *structure* is suitable.

- **Immutability is the default.** Data crossing a boundary should be immutable. **Red flag:** mutable data passed across boundaries without copying or freezing.
- **Minimality**: expose only what consumers need.
- **Contract clarity**: shared data schemas must be explicit.
- **Validation at boundaries**: validate on receipt. The sender's invariants are not the receiver's.
- **Persistence adds time as a dimension.** Can the stored format be migrated? Does serialisation preserve semantics (type information, ordering, precision, nullability)? Optimised for read patterns? **Red flag:** no versioning or migration strategy.