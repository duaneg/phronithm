# Breakdown: fixture

## Summary

A conforming plan for the validate tests.

## Work Units

### base — complete at `abc1234`; report `docs/base-completion.md`

### parser
- **Type**: feature
- **Risk**: low
- **Scope**: `src/parse.go`, `src/shared.go::Lex`
- **Publishes**: `Parse`
- **Depends on**: base (logical, the lexer types)
- **External gates**: `tool` (environment, go 1.27.1, pinned in `go.mod`); `verdict` (organisational)
- **Done condition**: The parser parses.

### docs-pass
- **Type**: design
- **Scope**: `docs/design.md`
- **Depends on**: none
- **External gates**: none
- **Done condition**: The design is written.

### wire-up
- **Type**: integration
- **Scope**: `src/shared.go::Wire` and `cmd/`
- **Depends on**: parser, docs-pass (testing, the parser and the design; both land first)
- **External gates**: `corpus` (data, `corpus` @ `abc123`)
- **Done condition**: The command runs.

## Graph

```yaml lsf-plan
units:
  - id: base
    type: foundation
    complete: true
    dependencies: []
    scope: [src/lex.go]
    publishes: []
    gates: []
  - id: parser
    type: feature
    complete: false
    dependencies: [base]
    scope: [src/parse.go, src/shared.go::Lex]
    publishes: [Parse]
    gates:
      - name: tool
        kind: environment
        pin: "go 1.27.1"
      - name: verdict
        kind: organisational
  - id: docs-pass
    type: design
    complete: false
    dependencies: []
    scope: [docs/design.md]
    publishes: []
    gates: []
  - id: wire-up
    type: integration
    complete: false
    dependencies: [parser, docs-pass]
    scope: [src/shared.go::Wire, cmd]
    publishes: []
    gates:
      - name: corpus
        kind: data
        pin: "corpus @ abc123"
execution_order: [base, docs-pass, parser, wire-up]
```

## Execution Order

1. `base`.
2. `docs-pass`, `parser`. The design first, since it is cheap.
3. `wire-up`.

## Gate commands

```yaml lsf-gates
gates:
  - name: tests
    timeout: soon
```
