```yaml lsf-plan
units:
  - id: a
    type: foundation
    complete: true
    scope: [src/x]
  - id: b
    type: feature
    scope: [src/x/b.go]
  - id: c
    type: feature
    dependencies: [b]
    scope: [src/c]
    gates:
      - name: legal
        kind: organisational
  - id: d
    type: feature
    dependencies: [b]
    scope: [src/d]
  - id: e
    type: feature
    dependencies: [d]
execution_order: [a, d, b, c, e, b]
```
