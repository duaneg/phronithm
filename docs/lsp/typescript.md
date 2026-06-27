# LSP: TypeScript

TypeScript language server configuration for [LSP integration](integration.md).

## Server

`typescript-language-server` wraps `tsserver` (the TypeScript compiler's language service) and exposes it via the Language Server Protocol. Alternative: `tsserver` directly, but the LSP wrapper is more widely supported.

## Prerequisites

- **Server must be running**: The skill does not start a language server. The server must be available before the analysis begins.
- **tsconfig.json**: The project must have a `tsconfig.json` (or `jsconfig.json` for JavaScript). The language server uses this to determine the project root and compilation settings. Without it, cross-file operations (`textDocument/references`, `callHierarchy`) may return incomplete results.
- **node_modules**: Dependencies should be installed (`npm install` / `yarn` / `pnpm install`). The language server resolves types from `node_modules`; missing dependencies produce incomplete type information.

## Invocation methods

Ranked by token efficiency and latency (see [integration](integration.md) for rationale):

1. **MCP tool** — an MCP server wrapping the TypeScript language server. Provides tool calls like `find_references`, `find_implementations`, `incoming_calls`. Most efficient when available.
2. **Tool script** — a script that connects to a running `typescript-language-server` instance and sends LSP requests. Returns structured results (file, line, character, context).
3. **Raw protocol** — JSON-RPC over stdio to `typescript-language-server --stdio`. Not recommended; initialisation handshake and message framing are token-expensive.

## Known quirks

- **Project references**: In monorepos using TypeScript project references (`references` in tsconfig.json), the language server may need to build referenced projects before cross-project operations work. If `textDocument/references` returns incomplete results in a monorepo, check whether referenced projects are built.
- **Declaration files**: References in `.d.ts` files may point to type declarations rather than runtime code. When tracing impact for runtime behaviour changes, filter or deprioritise `.d.ts` results.
- **Path aliases**: If the project uses path aliases (`paths` in tsconfig.json), the language server resolves them. grep does not — another source of false negatives in grep-based tracing.
