# LSP: Java

Java language server configuration for [LSP integration](integration.md).

## Server

`jdtls` (the Eclipse JDT Language Server, `eclipse.jdt.ls`) wraps the Eclipse Java compiler and JDT core, exposing them via the Language Server Protocol. It is the de facto standard Java language server and backs most editor integrations. No widely-used alternative provides comparable cross-file accuracy.

## Prerequisites

- **Server must be running**: The skill does not start a language server. The server must be available before the analysis begins.
- **Build configuration**: The project must declare its build to the server — `pom.xml` (Maven), `build.gradle` / `build.gradle.kts` (Gradle), or Eclipse `.project` / `.classpath` files. `jdtls` uses this to derive the classpath and project root. Without it, the server falls back to a single-file mode and cross-file operations (`textDocument/references`, `callHierarchy`) return incomplete results.
- **JDK**: A JDK must be installed and match (or exceed) the project's target release. `jdtls` itself requires a recent JDK to run; a mismatch with the project's source/target level produces resolution errors.
- **Resolved dependencies**: Dependencies should be downloadable or already cached (`mvn dependency:resolve` / `gradle dependencies`). `jdtls` resolves types from the classpath; unresolved artefacts produce incomplete type information.
- **Project import complete**: On first contact `jdtls` imports and builds the project into a workspace data directory and indexes it. Cross-file queries issued before import finishes return partial results. Allow the initial build to settle before relying on reference or call-hierarchy output.

## Invocation methods

Ranked by token efficiency and latency (see [integration](integration.md) for rationale):

1. **MCP tool** — an MCP server wrapping `jdtls`. Provides tool calls like `find_references`, `find_implementations`, `incoming_calls`. Most efficient when available.
2. **Tool script** — a script that connects to a running `jdtls` instance and sends LSP requests. Returns structured results (file, line, character, context).
3. **Raw protocol** — JSON-RPC over stdio to `jdtls`. Not recommended; the initialisation handshake (and `jdtls`'s import/build notifications) plus message framing are token-expensive.

## Known quirks

- **Generated sources**: Annotation processors (Lombok, MapStruct, immutables, Dagger) emit code the language server only sees if annotation processing is configured in the build. References to Lombok-generated members (getters, setters, builders) frequently fail to resolve — a major source of false negatives, analogous to path aliases in TypeScript. When tracing impact through a type that uses such processors, corroborate with grep.
- **Multi-module builds**: In multi-module Maven reactors or multi-project Gradle builds, `jdtls` must import every module before cross-module operations work — analogous to TypeScript project references. If `textDocument/references` returns incomplete results across modules, check whether all modules imported cleanly.
- **Dynamic dispatch**: Java leans heavily on interfaces and abstract classes. `textDocument/implementation` finds concrete implementors and overriding methods; grep on the method name misses overrides in subclasses and matches unrelated methods of the same name. This operation matters more for Java than for most languages.
- **Type erasure and overloads**: Generic type parameters are erased at the bytecode level, but the language server resolves references against source-level signatures, so it distinguishes overloads correctly where grep cannot.
- **Classpath gaps**: A missing or unresolved dependency silently degrades resolution for any code touching that dependency. Unlike a compile, the server may return partial results rather than an obvious error — treat sparse reference results on dependency-heavy code with suspicion.
