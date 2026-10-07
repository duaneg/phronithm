# Plan graph and decision log blocks

A decomposition plan carries its graph in one fenced YAML block, and a decision
log carries requester decisions in another; a plan may also declare its gate
commands in a third. The `lsf-implement` workflow reads only these blocks; the
prose entries are for people and for the agents that plan and implement units.
The graph and decision blocks are parsed by `workflows/lsf/graph.js` and its Go
port, which validate them and report every defect as a structural signal. No
agent interprets the plan's prose on the workflow's behalf.

## Plan block

Section `## Graph` of the plan, immediately after `## Work Units`:

````markdown
## Graph

```yaml lsf-plan
units:
  - id: interior-geometry
    type: foundation
    complete: true
    dependencies: []
    scope: [cartomancer/core/interior.py, cartomancer/core/geom/shapes.py::rect]
    publishes: [Room, Door, Interior]
    gates: []
  - id: gate-port
    type: integration
    complete: false
    dependencies: [content-gate-two-tone, tactical-interior-tier]
    scope: [maps/falls-outpost-gate.py]
    publishes: []
    gates:
      - name: carto-in-vault
        kind: environment
        pin: "packaging-vault-install"
      - name: requester-verdict
        kind: organisational
execution_order: [interior-geometry, gate-port]
```
````

Fields per unit:

| Field | Type | Meaning |
|---|---|---|
| `id` | string | The unit's `### <id>` heading under `## Work Units`. |
| `type` | enum | `foundation`, `feature`, `integration`, `design`, `refactor`, `bug-fix`, `polish`, `meta`. The entry's Type field. |
| `complete` | boolean | Set true by the merge stage when a Plan Maintenance completion entry is recorded. Defaults false. |
| `dependencies` | list of ids | Every unit in the entry's Depends on field, whatever the dependency type. A path of any type serialises. |
| `scope` | list of paths | Every file, module, or directory the unit writes. `path::symbol` when the unit owns one function or class in a file other units also write. |
| `publishes` | list of strings | Artefacts other units code against. Optional. |
| `gates` | list | Each external gate: `name`, `kind` (`environment`, `organisational`, `release`, `data`), and `pin` (plan-time version or reference, optional). No `cleared` field: clearance comes from the decision log. |

`execution_order` lists every unit id once, in the order the `## Execution
Order` prose commits to. Where that prose groups units into steps, the block
flattens the groups in written order.

The block and the entries must agree. Whoever amends one amends the other in
the same commit: breakdown when drafting, the re-plan step after each wave,
the merge stage for `complete`, the termination stage for appended `meta`
units.

## Archive

A plan the `lsf-implement` workflow drives has a companion
`docs/plans/<name>-archive.md`, append-only. At merge, a completed unit's
`### <id>` entry under `## Work Units` moves there verbatim, under a
`## Work Units` section of its own, and a one-line stub stays in the plan: `### <id> — complete
at <merge SHA>; entry archived to <archive>; report <path>`. At re-plan, a
Risk Registry entry the pass closes moves there under `## Closed risks`. The
plan block is unaffected by either move: the unit keeps its entry in `units`
with `complete: true`, so the graph, the ready set, and scope overlap are
computed over the whole plan as before.

## Decision log block

`docs/plans/<name>-decisions.md`:

````markdown
# Decisions: <feature>

```yaml lsf-decisions
decisions:
  - id: D1
    date: 2026-09-10
    question: Does U8 stand as a one-call API with Room/Door value types?
    decision: Yes. No builder object.
    resolves: [U8]
  - id: D2
    date: 2026-09-12
    question: Is the gate level accepted for commit?
    decision: Yes, tier keys as delivered.
    clears: [gate-acceptance/requester-verdict]
```
````

`id`, `date`, `question`, and `decision` are required. `clears` lists
`<unit>/<gate name>` pairs the decision clears; only `organisational`,
`release`, and `data` gates can be cleared this way. `environment` gates are
re-probed against their pin at implement time and never appear in `clears`.
`resolves` is free-form: signal ids, plan question labels, whatever the ledger
entry the decision answered was keyed by.

Requester decisions are written here and nowhere else. Plan Maintenance is
written by agents and records amendments; it does not clear gates.

## Gate block

A plan may declare the project's gate commands in a third block, in its
`## Gate commands` section. The Go runtime (`lsf-runtime`) runs them; the
Workflow script ignores the block.

````markdown
```yaml lsf-gates
setup: "npm ci"
gates:
  - name: tests
    command: "npm test"
    timeout: 45m
  - name: lint
    command: "npm run lint"
```
````

- `gates` is required and non-empty. Each gate has a unique `name` and a
  non-empty `command`, which no other gate repeats. `timeout` is optional: a
  Go duration (`90s`, `45m`, `2h`) greater than zero. Without one, the
  runtime config's `gateTimeout` (default 2 h) applies.
- `setup` is optional. It runs once per checkout before the gates, for
  example to install dependencies. If it fails, no gate runs.
- Quote a command that contains `: ` or ` #`; see the YAML subset below.
- A plan has at most one gate block, counting any fenced example at line
  start in the plan document. Without one, stages run and report the gates
  themselves.

Each command runs through `sh` on a clean detached checkout of the commit
being gated, so it sees only committed files (submodules are not
initialised: `setup` does that). A command must depend on the checkout's files
alone, not on history, branch names or the machine's state, because results
are kept per tree. A command killed by a signal is not run, not red. The
runtime gates:

- a unit's implement commit (not for design units);
- each code fix's commit;
- each merge's tree, before the merge is committed. A merge whose gates fail
  is not committed.

An identical tree is gated once. A result whose gates all completed, red ones
included, is journaled and replays on relaunch; a gate not run (its timeout, a
failed setup) reruns. To rerun a red result after fixing the environment,
remove its record, `journal/gates_<tree prefix>-*.json`, from the state
directory. A merge that differs from the unit's last gated tree only at its
completion report reuses that result within the launch (in practice, the
first merge of a wave), so a gate that reads the completion report is not
re-run for the merge.

If the integration branch moves while a merge's gates run, the new merge is
gated once more, and a second move faults. A merge commit whose tree no run
covers (one that raced the merge, or a hook's change) is gated when the stage
meets it, and is a fault if its gates fail.

The runtime fixes the block at a launch's first load. Values compare as
written, so `1h` and `60m` differ. A wave re-plan that changes the block stops
the run with `declared-gates-changed`, and the relaunch takes the changed
block. A requester's change goes through a decision, which the decision
re-plan applies before the first load. The commands run with the runtime's
environment, credentials included, and are trusted as the plan document is;
the last 4 KB of a failing gate's output goes to the reviewers' prompts and
the journal.

## YAML subset

The parser is dependency-free and accepts a deliberately small subset:

- Block mappings (`key: value`, `key:` then an indented block) and block
  sequences (`- item`, `- key: value` opening an inline mapping).
- Flow sequences of scalars: `[a, b, "c d"]`. No nested flow collections, no
  flow mappings.
- Plain, single-quoted, and double-quoted scalars. Double quotes support
  `\"`, `\\`, `\n`, `\t`. Everything is a string except `true`, `false`, and
  `null` / `~` / empty.
- Comments (`#` at line start or after whitespace). Indent with spaces only.
- Keys match `[A-Za-z0-9_][A-Za-z0-9_.-]*`.

Not accepted: anchors and aliases, tags, block scalars (`|`, `>`), multi-line
plain scalars, flow mappings, tabs, documents (`---`). A block outside the
subset is a structural `invalid-plan` signal naming the line.

Quote a scalar when it contains `: `, ` #`, or begins with a character the
subset treats specially (`[`, `{`, `&`, `*`, `!`, `|`, `>`, `%`, `@`, `` ` ``,
quotes). Paths with `::` need no quoting.
