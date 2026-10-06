# Doc

prose

```yaml lsf-plan
units:
  - id: a
    type: foundation
    complete: true
    scope: [src/a.py]
    publishes: [A]
  - id: b
    type: feature
    dependencies: [a]
    scope: [src/b.py::f, tests/test_b.py]
    gates:
      - name: legal
        kind: organisational
      - name: node
        kind: environment
        pin: "22.1"
execution_order: [a, b]
```

more prose
