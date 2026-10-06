```yaml lsf-plan
units:
  - id: a
    type: feature
    dependencies: [c, ghost]
    scope: [src/x]
  - id: b
    type: feature
    dependencies: [a]
    scope: [src/y]
  - id: c
    type: feature
    dependencies: [b]
  - id: d
    type: feature
    scope: [src/x/a.go]
  - id: e
    type: feature
    scope: [src/x/a.go::f]
  - id: a
    type: design
execution_order: [a, b, c, e, phantom]
```
