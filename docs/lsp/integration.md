# LSP Integration

Language Server Protocol integration for dependency tracing in [phronithm:impact-analysis](../../skills/impact-analysis/SKILL.md). LSP provides compiler-grade accuracy for operations that grep approximates — particularly dynamic dispatch, structural typing, and transitive call chains.

## Operation catalogue

| LSP operation | Impact-analysis use | Replaces |
|---|---|---|
| `textDocument/references` | Direct dependents (Phase 2 step 1), verify exclusions (Phase 4) | grep / `tools/map-symbol-consumers.py` |
| `textDocument/implementation` | Interface implementations, dynamic dispatch (Phase 2 step 1) | Manual search — currently a documented gap |
| `callHierarchy/incomingCalls` | Transitive call chain tracing (Phase 2 step 2) | LLM reasoning only |
| `textDocument/typeDefinition` | Type dependency tracing (Phase 2 step 1) | grep for type names |
| `textDocument/definition` | Navigate to definitions during tracing | Manual file reading |

### When LSP matters most

- **Dynamic dispatch**: `textDocument/implementation` finds concrete implementations of interfaces and abstract methods. grep searches for the method name, which may miss implementations or return false positives from unrelated methods with the same name.
- **Structural typing**: In languages with structural types (TypeScript, Go), code can depend on a type's shape without naming it. `textDocument/references` finds these shape-based dependents; grep cannot.
- **Transitive call chains**: `callHierarchy/incomingCalls` mechanically traces the incoming call chain. Without LSP, transitive tracing relies on LLM reasoning — slower and less reliable for deep chains.
- **Overloaded names**: grep matches all occurrences of a name regardless of scope. LSP resolves to the specific declaration and returns only genuine references.

## Availability detection

Attempt a real LSP operation (e.g. `textDocument/references` on the first element of the change surface). If it succeeds, LSP is available for this session. If it fails or no LSP mechanism is configured, fall back to grep-based methods. No separate detection step — the first real query doubles as the availability test.

## Invocation methods

Ranked by token efficiency and latency (best first):

1. **MCP tool** — ~30 tokens per call, persistent server connection, lowest latency. Requires MCP server configuration.
2. **Tool script** — ~50-80 tokens per call, script startup per invocation. Self-contained, no MCP configuration needed.
3. **Raw LSP protocol** — ~200-500 tokens per call. Not recommended; protocol verbosity dominates.

The phronithm:impact-analysis skill text is method-agnostic. Use whatever mechanism is available.

## Fallback behaviour

When LSP is unavailable, all phases fall back to their pre-LSP methods:

- Phase 2 step 1 (direct dependents): grep / `tools/map-symbol-consumers.py`
- Phase 2 step 2 (transitive dependents): LLM reasoning with heuristics
- Phase 4: grep-based verification

The impact map remains valid without LSP — the confidence and gaps sections should note the reduced mechanical coverage.

## Language appendices

- [TypeScript](typescript.md)
- [Java](java.md)
