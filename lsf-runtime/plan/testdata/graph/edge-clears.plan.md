# Plan

```yaml lsf-plan
units:
  - id: a
    type: feature
    gates:
      - name: ""
        kind: release
      - name: legal
        kind: organisational
  - id: a
    type: feature
    gates:
      - name: legal
        kind: data
execution_order: [a]
```
