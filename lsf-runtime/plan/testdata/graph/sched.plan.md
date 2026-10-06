# Scheduling fixture

```yaml lsf-plan
units:
  - id: a
    type: foundation
    complete: true
    scope: [src]
  - id: b
    type: feature
    dependencies: [a]
    scope: [src/b, docs/x.md::intro]
  - id: c
    type: feature
    dependencies: [a]
    scope: [src/c, docs/x.md::outro]
    gates:
      - name: legal
        kind: organisational
  - id: d
    type: integration
    dependencies: [b, c]
    scope: [src/b/d.go, docs/x.md]
    gates:
      - name: ship
        kind: release
      - name: go
        kind: environment
        pin: "1.27.1"
  - id: e
    type: design
    scope: [docs/e.md]
    gates:
      - name: review
        kind: organisational
  - id: f
    type: polish
    dependencies: [e]
execution_order: [a, e, c, b, d, f]
```
