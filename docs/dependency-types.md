# Dependency Types

Reference catalogue for [phronithm:impact-analysis](../skills/impact-analysis/SKILL.md) Phase 2 tracing. Not every type is relevant to every change — the change classification from Phase 1 determines which to trace.

## Call dependencies

Direct function or method calls. Found by searching for references to the changed function.

Subtleties:

- **Dynamic dispatch**: calls through interfaces, virtual methods, function pointers, or closures. Searching for the function name won't find callers that invoke it through an abstraction. Search for implementations and usages of the interface or trait.
- **Reflection and metaprogramming**: calls constructed at runtime from strings. Not findable by static search. Flag as a gap if the codebase uses reflection.

## Type dependencies

Usage of changed types, interfaces, traits, or type aliases.

Subtleties:

- **Structural typing**: in languages with structural types (Go interfaces, TypeScript), code can depend on a type's shape without naming it. Changing a method on a struct can break an interface satisfaction elsewhere without any import relationship.
- **Generic instantiation**: changing a type used as a generic parameter can affect all instantiation sites differently depending on how the generic uses it.

## Data dependencies

Changes to the shape or semantics of data at rest or in transit: database schemas, serialised formats (JSON, protobuf, etc.), configuration structures, file formats.

Code dependencies are spatial — they exist in the current snapshot and can be found by searching. Persistent data dependencies are temporal — data written by a previous version of the code will be read by a future version. The dependency is invisible in the current code graph, which makes it easy to miss and dangerous to get wrong.

**Temporal coupling**: existing data constrains the change. Data at rest was written under the old contract. The change doesn't propagate to it; you either migrate it or maintain backward compatibility. In-memory data shape changes are caught by the compiler; persistent data shape changes are caught by production failures.

**Migration as impact**: a schema migration is itself a change that touches the same data production code reads. It has its own blast radius: readers, writers, validators, and anything downstream of them. When impact analysis identifies a data shape change, the required migration is part of the impact, not an afterthought.

**Implicit schemas**: code that parses data without a formal schema definition. The "schema" is the sum of all code that reads and writes the data, defined implicitly. Tracing impact requires identifying all readers and writers, not just the ones near the change site. Changing the writer without updating all readers is a data shape change; so is changing a reader to expect a shape the writer doesn't produce.

**Serialisation boundaries**: data serialised by one version and deserialised by another. Backward compatibility (new reader, old data) and forward compatibility (old reader, new data) are separate concerns and both need tracing.

## Behavioural dependencies

Code that depends on the *behaviour* of the changed code without a structural dependency on its signature or type. These are the hardest to find and the most dangerous to miss.

Examples:

- Code that parses error messages or log output.
- Code that depends on ordering guarantees (iteration order, event delivery order) that aren't part of the contract.
- Code that depends on timing (polling intervals, timeout values, rate limits).
- Code that depends on side effects (file creation, environment variable mutation, global state changes).
- Code that depends on the *absence* of an error (assumes a function never fails for certain inputs).

These cannot be found by mechanical search. They require understanding the system's runtime behaviour and the conventions (documented or otherwise) that code relies on.

## Build and configuration dependencies

Build scripts, CI configuration, deployment manifests, environment variables, feature flags.

Subtleties:

- **Conditional compilation**: code behind feature flags or build variants may not be visible in a normal search. Trace across all active build configurations.
- **Deployment order**: in systems deployed independently, interface changes may require coordinated deployment. Note when this applies.
