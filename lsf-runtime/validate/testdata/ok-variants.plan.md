# Breakdown: variants

## Work Units

### done-full
- **Type**: foundation
- **Scope**: `lib/`
- **Depends on**: none
- **External gates**: NONE
- **Done condition**: kept in full though complete.

### alpha
- **Type**: feature
- **Scope**: `lib/a.go::Run`,
  `lib/b.go`
  (and nothing else)
- **Depends on**: done-full, `beta` (testing, the fixtures; see `x; y`); gamma
- **External gates**: `git` (environment, 2.38 or later; 2.54.0 probed)

  ```text
  ### not-a-unit
  ```
- **Done condition**: Runs.

### beta
- **Type**: design
- **Scope**: `docs/beta.md`
- **Depends on**: none
- **External gates**: `node` (environment, `node` v26.9.0 or later)

### gamma
- **Type**: refactor
- **Scope**: `lib/c.go`
- **Depends on**: none
- **External gates**: none

## Graph

```yaml lsf-plan
units:
  - id: done-full
    type: foundation
    complete: true
    dependencies: []
    scope: [lib]
    publishes: []
    gates: []
  - id: alpha
    type: feature
    complete: false
    dependencies: [done-full, beta, gamma]
    scope: [lib/a.go::Run, lib/b.go/]
    publishes: []
    gates:
      - name: git
        kind: environment
        pin: ">= 2.38"
  - id: beta
    type: design
    complete: false
    dependencies: []
    scope: [docs/beta.md]
    publishes: []
    gates:
      - name: node
        kind: environment
        pin: "v26.9.0"
  - id: gamma
    type: refactor
    complete: false
    dependencies: []
    scope: [lib/c.go]
    publishes: []
    gates: []
execution_order: [done-full, beta, gamma, alpha]
```

## Execution Order

```text
`alpha` is not a mention here.
```

1. **beta** and `gamma`, then `alpha`; `done-full` is already merged.
