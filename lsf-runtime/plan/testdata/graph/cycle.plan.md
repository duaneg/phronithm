```yaml lsf-plan
units:
  - id: p
    type: feature
    dependencies: [q]
  - id: q
    type: feature
    dependencies: [r]
  - id: r
    type: feature
    dependencies: [q]
execution_order: [p, q, r]
```
