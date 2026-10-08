---
description: "Decompose a large feature or goal into tracked work units with dependency ordering, parallelism analysis, and execution guidance. Use when planning multi-step work or preparing parallel execution."
allowed-tools: Read, Grep, Glob, Bash, Write, Edit, Task, Agent
---

# Breakdown: Work Decomposition and Execution Planning

Decompose a large feature or goal into tracked work units with dependency ordering, parallelism analysis, and execution guidance. Produces a decomposition plan document.

Serves human teams planning work and agent teams needing structured task assignments.

## Interface

### Requires

- **goal**: What to decompose. One of:
  - A feature description or goal statement (most general — the skill does the full analysis)
  - An architecture document path (from `/phronithm:large-scale-feature` Phase 2 — requirements and design already settled; Phase 1 step 1 is abbreviated)
  - An architecture document plus a reconciliation record and an integration branch (`/phronithm:large-scale-feature` Phase 2 re-entry after a Phase 3 stop — a fresh decomposition over merged work)
  - A list of existing work items to re-plan (issue numbers, TODO items, or equivalent)
  - An existing decomposition plan plus one or more unit completion reports (re-plan mode — see [Re-plan on unit completion](#re-plan-on-unit-completion))

**Goal specificity check**: Before beginning analysis, verify the goal is specific enough to decompose. A valid goal must have a clear done condition — you must be able to answer "how would we know this is complete?" If the goal is too vague to answer that question, stop and ask for clarification. Do not begin analysis on an underspecified goal.

### Produces

- **decomposition-plan**: Document at `docs/plans/<feature-name>-breakdown.md` (or project equivalent). Contains:
  - Work units with classification metadata and done conditions
  - Graph block: the units, dependencies, scope, gates, and execution order as one machine-readable YAML block (format: [plan graph](${CLAUDE_PLUGIN_ROOT}/docs/plan-graph.md)), kept in step with the entries
  - Gate commands block, when the project has gates: its gate commands (test suite, linters, type checks) as a ```yaml lsf-gates block (same format document). The Go runtime (`lsf-runtime`) runs them itself; the Workflow script ignores the block
  - Execution order with rationale
  - Dependency graph (Mermaid format preferred), parallel streams for 2-worker and N-worker scenarios, and critical path analysis — omitted from a plan drafted for `phronithm:large-scale-feature`, whose Phase 3 workflow computes them from the Graph block
  - Meta-tasks (test harness, debug tooling, integration testing)
  - Standing obligations spanning units
  - Risk registry
  - Plan maintenance record

### Consumes

- **architecture-document** (from phronithm:large-scale-feature Phase 2): When present, requirements and design are already settled — Phase 1 step 1 is abbreviated to reading the document and extracting components. Steps 2–5 run in full.
- **reconciliation-record** (from phronithm:large-scale-feature Phase 2 re-entry): for each unit merged under the previous plan, whether it stands under the amended architecture or is superseded, with evidence. When present, Phase 1 step 1 reads the integration branch as the codebase, not abbreviated: standing work is existing code the new units build on; superseded work yields units that adapt or revert it; the previous plan's standing obligations and open risks carry into the new plan per the record's dispositions, and any requester decisions supplied alongside are applied as in re-plan mode. A unit whose done condition standing work already satisfies is marked `complete: true` only after that condition is evaluated against the integration branch, the evaluation cited in the entry; a failed condition leaves it new work, and an unevaluable one is reported to the requester rather than marked.
- **impact-map** (from phronithm:impact-analysis): When present, informs scope boundaries and identifies cross-cutting concerns.
- **completion-report** (from the unit's executor — `phronithm:feature` Phase 5 produces one in work-unit mode; free-form from other executors): the deviations, deferrals, discoveries, and execution signals (context consumption, decisions taken mid-implementation) recorded when a unit completed. Required for re-plan mode — the re-plan diff runs off it.

### Complexity drivers

- **Scope breadth**: Single module → low. Multiple interacting systems → high.
- **Dependency density**: Mostly independent units → low. Deep chains or mutual dependencies → high.
- **Domain novelty**: Well-understood pattern (CRUD, pipeline) → low. Novel mechanics or creative work → high (iteration profile harder to predict).
- **Audience**: Single developer → simpler plan. Multi-agent or multi-developer team → parallelism analysis needed.

## Concepts

### Work unit classification

Each work unit is classified on five axes:

**Type** — the unit's role in the feature:

| Type | Description |
|------|-------------|
| `foundation` | Other units depend on it — types, interfaces, plumbing |
| `feature` | Independent functionality |
| `integration` | Connects other units or systems |
| `design` | A decision that must be resolved before dependent units proceed |
| `refactor` | Restructuring existing code to support the feature |
| `bug-fix` | Fixing a latent bug exposed or required by the feature |
| `polish` | Iterative tuning, visual/UX refinement |

**Risk** — `low` / `medium` / `high` with rationale. Low: well-specified, mechanical. Medium: some unknowns or integration surface. High: creative work, novel mechanics, cross-cutting concerns.

**Clarity** — how well-understood the approach is:
- `specified`: Clear done condition, known implementation approach
- `exploratory`: Requires investigation, prototyping, or design work before the implementation approach is clear

A unit carrying an open design decision is `exploratory`, however settled the rest of its approach. Exploratory units have higher effort variance. When a unit is exploratory, consider whether a preceding `design` unit should resolve the uncertainty first.

**Iteration profile** — `write-once` vs. `iterative`:
- `write-once`: Implement and done. The initial implementation is the final form.
- `iterative`: Requires multiple passes — parameter tuning, visual adjustment, performance optimisation, or structural evolution based on runtime feedback.

Iterative units are systematically underestimated. When a unit is marked `iterative`, the skill must determine whether the iteration is **tuning** (known structure, adjusting parameters) or **structural** (the implementation approach itself evolves). These have different mitigations:

*Tuning iteration* triggers a companion `design` unit for tuning infrastructure. That design unit should consider, in preference order:

1. **Automated optimisation** — objective function + parameter search (e.g., energy conservation, frame time budget)
2. **Data-augmented manual** — instrumentation to inform decisions (e.g., distribution heatmaps, A/B metrics)
3. **Efficient manual** — hot-reload, sliders, config files
4. **Edit-rebuild-test** — fallback; minimise iteration time

If the project already has tuning infrastructure that covers this need, the design unit may collapse to a verification note.

*Structural iteration* is better addressed by the `exploratory` clarity flag, higher effort estimates, and risk flagging. Pre-designing away structural iteration is often wasted effort — you're designing before you have the information that drives the changes. This covers only decisions that cannot surface until implementation: a question identifiable at planning time counts against the [size budget](#unit-size-budget)'s open-design-decision bound, whose extraction rule governs. Structural iteration in a unit that others consume usually signals consumer-determined content — apply the mechanism/content split from Phase 1 step 2; the flag alone only adjusts the effort estimate.

**Effort** — rough guide, not a commitment:
- `small`: <1h focused work
- `medium`: 1–4h
- `large`: 4h+

Effort is wall-clock, a planning convenience. The binding session bound is the [size budget](#unit-size-budget), which is context, not hours — the two do not convert.

### Unit size budget

A unit must complete within one agent context window. The budget is context, not output volume or wall-clock hours: a large port with a behavioural reference costs less context than a smaller unit holding several surfaces in view with none. Judge it by structural load factors:

- **Surfaces written**: at most two of the modules, packages, or directories in the unit's [Scope](#scope) are written to; surfaces only read do not count.
- **Open design decisions**: at most one — a unit with one open question can take it carefully; a unit with several takes the rest under context pressure. An open decision is a question identifiable now with no designed answer in the plan or its inputs. Decisions beyond the first are extracted into a preceding `design` unit.
- **Planned steps**: at most about six.
- **Behavioural reference**: a unit with no existing implementation or oracle to code against spends context faster at the same output size — apply the other factors more strictly.

These are drafting-time proxies, not hard thresholds. A unit exceeding one may instead carry a flag with rationale where the excess is wall-clock rather than context (work paced by an external review cadence, say); exceeding the design-decision budget is repaired by extraction, never flagged. Where completed units with comparable commit discipline exist — always the case in re-plan mode — calibrate numeric backstops (line counts against context consumed) from them rather than assuming universal numbers.

### Dependency types

Dependencies between work units come in several forms. The first four are always analysed; the remainder are prompted for when relevant.

**Always analysed:**

| Type | Description | Example |
|------|-------------|---------|
| **Logical** | A defines types/APIs that B consumes. Strict ordering. | Types & plumbing → arena physics |
| **Testing** | A must be functional to *verify* B, even if B doesn't logically depend on A's internals. Plans that look parallel on paper but are serial in practice create false expectations — record testing dependencies and plan around them, don't wish them away. A mechanism whose only realistic verifier is its first consumer puts a testing edge against the logical edge — a cycle. Treat that as a signal to split the producer (Phase 2 step 4), not an order to pick. | Transitions → arena physics (can't test arena without entering it) |
| **Design** | A decision must be made before B can proceed. | Visual effects design → renderer changes |
| **Knowledge** | Completing A produces understanding needed to design/implement B. | Prototype/spike → production implementation |

**Prompted — external factors:**

| Type | Prompt |
|------|--------|
| **Organisational** | "Are any units gated by specific people, teams, or external approvals?" |
| **Release** | "Do any units depend on a prior release or deployment?" |
| **Data** (runtime state, migrations, infrastructure) | "Do any units depend on migrations, data pipelines, or environment setup?" |

Organisational and release gates cannot be discovered from the codebase — ask explicitly, per unit, in Phase 1 step 4. Environment gates often can be: probe the machine (step 4) rather than asking.

### Work unit done conditions

Every work unit must have a verifiable done condition — a statement that can be evaluated true/false without the work unit's author making a judgement call. Examples:
- "All tests in `tests/arena/` pass"
- "The configuration schema validates against the fixture suite with zero errors"
- "Design question resolved and decision recorded in plan document"

A done condition must also be **evaluable in position**: every artefact it names is produced by the unit itself, by a declared dependency, or by a declared external gate. If it names anything else, either a dependency is missing or the condition belongs to a different unit — check both directions, since the done condition records what the unit needs while the dependency list records what its author remembered. Where a condition cannot be made evaluable in position, split it: assert now what is checkable at the unit's position, defer the rest to a named later unit, and flag the deferral on this unit.

Exploratory units may have a process done condition: "Spike complete: approach decided, documented, and exercised through one end-to-end path." The decision plus the driven path is the artefact.

### Scope

Each work unit lists the files, modules, or directories it will touch, and names what it **publishes** — the APIs, protocols, schemas, formats, or measured constants other units code against. Scope is a coordination declaration for parallel execution, not a limit on what the executor may edit: a unit that must touch a file outside its Scope does so and reports it in its completion report, and re-plan corrects the line where a later unit shares the file. For each dependency edge, name the artefact that crosses it and the unit that owns it. An artefact more than one unit reads is a named deliverable of exactly one unit. An artefact more than one unit writes is either a unit of its own or a scope conflict requiring a coordination protocol.

For teams (human or agent), scope boundaries determine:
- Which units can be worked on concurrently without conflicts
- The boundary of responsibility for a given worker
- What to review when the unit is complete

Units with overlapping scope must either be serialised or have a concrete coordination protocol defined (e.g., "Unit B waits for Unit A's merge before starting", or "file X is owned by Unit A — Unit B uses the interface but does not modify the file"). Noting overlap without specifying the protocol is not sufficient. Record an ownership protocol in each unit's Scope as `file::symbol` entries, so a scheduler reading only scope sees the units as disjoint.

## Workflow

### Startup

Follow the [pre-flight](${CLAUDE_PLUGIN_ROOT}/docs/pre-flight.md) check. Do not continue until it passes.

Read `CLAUDE.md` for the target project. Check dependency manifests to identify the tech stack.

**Goal validation**: Confirm the goal passes the goal specificity check stated in [Requires](#requires). If the goal is a vague description that cannot pass the "how would we know this is complete?" test, stop here and ask for clarification before proceeding.

### Phase 1: Analyse

Understand the feature's structure before decomposing it.

1. **Load context**. Depending on input type:
   - *Feature description*: Read relevant project documentation and codebase. Identify the major components, integration points, and boundaries.
   - *Architecture document*: Read the document. Extract the component list, data model, and integration points. The requirements and design are settled — proceed directly to step 2. With a reconciliation record, read the integration branch as well, per the record's Consumes entry.
   - *Existing work items*: Fetch each item with full history. Build a picture of the intended scope and decisions already taken.
   - *Existing plan plus completion reports*: re-plan mode — skip Phases 1 and 2 and go directly to [Re-plan on unit completion](#re-plan-on-unit-completion).

2. **Identify work units**. Each unit must be:
   - **Individually completable**: within the [size budget](#unit-size-budget) — one agent context window, judged by the load factors there
   - **Verifiable**: done condition stated now (see [Work unit done conditions](#work-unit-done-conditions))
   - **Clearly scoped**: files/modules enumerable upfront (see [Scope](#scope))

   Record testing dependencies explicitly in step 4 — a unit may be implementable without others but not verifiable until a dependency is complete.

   Split on **responsibility boundaries**, not size — up to the size budget. Prefer fewer, larger units — coordination overhead grows faster than task-size overhead — but a unit over budget splits regardless, on the best available seam: a preceding `design` unit extracting its open decisions, or the mechanism/content split below.

   When a unit's completeness is determined by its consumers (schemas, vocabularies, plugin protocols, public APIs), split it on the mechanism/content seam: a mechanism unit with a fixture-checkable done condition and an extension protocol; a content contribution in each consumer's scope and done condition; and a closure unit that owns the completeness criterion, depending on every consumer. Record each consumer's contribution and the deferred completeness criterion in the plan's Standing Obligations section.

   **Convergence**: Complete when every component from step 1 has at least one unit and every integration point is covered. If new components keep surfacing after 2–3 passes, the feature needs splitting into phases.

3. **Classify each unit** on all five axes (type, risk, clarity, iteration profile, effort). For each unit marked `iterative`, determine whether the iteration is tuning or structural, and whether a companion `design` unit is needed for tuning infrastructure.

4. **Map dependencies**. For each unit, check which other units it depends on across all four always-analysed types. For small decompositions (≤8 units), check all pairs. For larger decompositions, scope logical-edge checking to the units that logically precede each unit — full pairwise checking at scale is impractical. Only logical edges are precedence-scoped. For every unit, regardless of position, ask: what must be functional to verify this (testing)? what decision or understanding must exist first (design, knowledge)? Reverse-direction edges are what form cycles, and precedence-scoped checking cannot find them. External gates are per-unit by construction (below).

   Ask the external-factor question per unit, as each done condition is written, not once for the decomposition: is this unit gated by specific people or teams, a prior release, a data migration, or environment setup? A done condition that invokes a binary, a service, an account, an installed font or data set, or another team's environment has an external gate whatever the decomposition-wide answer is. Record each gate in that unit's External gates field, labelled with its kind, and as a node in the dependency graph — the risk registry holds only residual risk.

   Probe environment gates now rather than recording them on trust: check that the binaries, versions, fonts, services, and data sets the done conditions rely on exist and behave as assumed, and pin versions in the plan. If running non-interactively and a gate cannot be resolved, record it as unknown on the unit and flag it in the risk registry.

   Derive the project's gate commands (its test suite, linters, type checks) from its instructions (CLAUDE.md or equivalent) and its CI configuration, each exactly as the project runs it. A plan drafted for `phronithm:large-scale-feature` records them in the `## Gate commands` block; these are not the units' external gates. Where the sources disagree, or name no command for a check the project evidently has, ask the requester; non-interactively, take the CI's command and flag the choice in the risk registry. The runtime runs each through `sh` on a clean checkout of a commit: only committed files, no build caches. Put a dependency install, a build or `git submodule update --init` the gates need in `setup`. Run each command once at the base commit: a gate red there makes every merge regress, so it is a question for the requester. Give a `timeout` to a gate known to outlast the runtime default of 2 h. Omit the block when the project has no gates.

5. **Identify meta-tasks**. Debug tooling, test harnesses, and verification checkpoints are real work — anticipating them prevents surprise scope growth and ensures they're budgeted. A verification checkpoint that gates a specific unit is not a meta-task — it is that unit's dependency and a clause of its done condition; only work belonging to no single unit stays here. Work that falls outside any single unit but is necessary:
   - Test harness or debug tooling (e.g. shortcuts to reach the feature under development)
   - Manual verification checkpoints (when automated testing is insufficient)
   - Integration testing after units are combined
   - Documentation updates
   - Tuning infrastructure (surfaced by step 3's iteration profile analysis)

### Phase 2: Plan

Build the execution plan from the analysis.

1. **Compute execution order**. Not just a topological sort — shaped by three additional factors:
   - **Risk reduction**: High-risk and exploratory units early, when there's room to adjust.
   - **Testing feasibility**: Order so each unit can be tested when implemented, not deferred until later units provide test infrastructure. Where testing dependencies make this impossible, note it explicitly.
   - **Foundation first**: Units of type `foundation` before all others.

2. **Identify parallel streams**. Group units that can be executed concurrently:
   - **Minimum parallelism** (2 workers): Which two streams give the best wall-clock improvement? Usually: critical path in one stream, everything else in the other.
   - **Full parallelism**: Maximum concurrent units at each phase. Note synchronisation points where streams must reconverge.
   - For each stream, state the **scope boundary**. Streams with overlapping scope must be either serialised or assigned a concrete coordination protocol — do not leave overlap unresolved. Disjointness claims are load-bearing: concurrent executors cannot see each other's adaptations, so verify disjointness against both files touched and artefacts published.

3. **Build the critical path**. The longest chain of dependent units — the minimum wall-clock time regardless of parallelism. Flag it explicitly.

4. **Quality check** before drafting the document. A check that cannot be satisfied may instead be discharged by an explicit flag on the affected unit stating what is unresolved — a flagged unknown directs executor judgement, a silent gap misdirects it — unless the item states its own repair, in which case only the repair discharges it. Two checks must always be satisfied outright: acyclicity, and the overlap status of every parallel-stream pair — disjoint (verified per step 2), serialised, or under a concrete coordination protocol, never left open. Verify:
   - Every major component identified in Phase 1 is covered by at least one work unit.
   - The union of all dependency edges — logical, testing, design, knowledge, external — is acyclic. Check mechanically (`tsort` or a short script over the edge list), not by eye. Where a logical edge and a testing edge oppose each other between two units, resolve it: split the producer on a mechanism/content or protocol/implementation seam, merge the pair, or define a vertical-slice protocol (one end-to-end path through both units before either completes). Recording both edges and choosing an order is not a resolution.
   - Every work unit has a verifiable done condition, evaluable in position from the unit's own output plus its declared dependencies' and external gates' outputs (see [Work unit done conditions](#work-unit-done-conditions)). A condition failing this check is repaired by the split procedure there — a flag without the split does not discharge it.
   - Every work unit is within the [size budget](#unit-size-budget), including units whose scope an amendment has enlarged since they were drafted. A unit over the open-design-decision budget is repaired by extracting a preceding `design` unit — a flag does not discharge it. Other excesses are flaggable only with rationale that the excess is wall-clock, not context.
   - Every artefact named in a `Depends on` entry is published by exactly one unit, positioned no later than its first consumer.
   - Every success criterion or acceptance measure in the governing goal is named in at least one unit's done condition, at the scale it will be judged. Where a later audit unit measures a criterion from data produced earlier, the producing units' done conditions carry the obligation to produce it — data that must be collected while work happens cannot be reconstructed by the unit that consumes it. Record such obligations in Standing Obligations, naming the carrying units.
   - Every `high`-risk unit is either placed early in execution order or has a risk mitigation note in the Risk Registry.
   - Every `foundation`-type unit appears before its dependants in the execution order.
   - All overlapping-scope parallel streams have a coordination protocol.
   - The `## Graph` block agrees with the entries: every unit, its type, every Depends-on unit, every Scope path, every gate with its kind and pin, and the execution order — one id per unit, flattened from the Execution Order steps in written order. Discharge this on the drafted document, before step 7's commit and on every re-plan, interactive or not: `go -C ${CLAUDE_PLUGIN_ROOT}/lsf-runtime run ./validate/cmd/lsf-validate "$PWD/<plan path>"` must exit zero. It also runs the workflow load stage's checks (overlap, cycle, `clears` entries, the `lsf-gates` block), and compares the execution order of incomplete units only: the first backticked or bold mention of each in the Execution Order section, in written order, so a rationale naming a later unit early fails it. A completed unit's one-line stub is checked against `complete: true` alone. Where Go is not installed, check with a short script over the block and the entry fields, not by eye.
   - The `## Gate commands` block holds the commands Phase 1 step 4 derived (skipped in re-plan mode: the block is fixed for the run).

5. **Draft the decomposition plan document**. Structure below. A plan drafted from an architecture document for `phronithm:large-scale-feature` is driven by its Phase 3 workflow (`phronithm:lsf-implement`): omit Dependency Graph, Parallel Streams, and Critical Path from it, since the workflow computes readiness and the wave plan from the `## Graph` block and the re-plan step does not maintain the prose. The analysis in steps 2–3 still runs: the Summary and the commit message keep citing the critical path length, and the overlapping-scope check in step 4 is satisfied by the edges and `file::symbol` Scope entries the block carries rather than by a written coordination protocol. Every other plan keeps the three sections.

   ```
   # Breakdown: <feature name>

   ## Summary
   <1–2 sentence overview. Total units, critical path length, key risks.>

   ## Work Units

   ### <unit-name>
   - **Type**: foundation | feature | integration | design | refactor | bug-fix | polish
   - **Risk**: low | medium | high — <rationale>
   - **Clarity**: specified | exploratory
   - **Iteration**: write-once | iterative (tuning | structural)
   - **Effort**: small | medium | large
   - **Scope**: <files/modules; `file::symbol` for a function or class owned within a file other units also write>
   - **Publishes**: <artefacts other units code against, or "none">
   - **Depends on**: <unit-name (dependency-type, artefact consumed) or "none">
   - **External gates**: <named gates, each labelled `(environment | organisational | release | data)` — binary and version, service, person/team, migration — or "none">
   - **Done condition**: <verifiable statement>
   - **Description**: <what this unit does>

   ## Graph
   <one ```yaml lsf-plan block — units (id, type, complete, dependencies,
    scope, publishes, gates with kind and pin) and execution_order — per
    ${CLAUDE_PLUGIN_ROOT}/docs/plan-graph.md. The machine-read form of the
    entries above; amend both together>

   ## Gate commands
   <one ```yaml lsf-gates block — the project's gate commands (setup, then
    each gate's name, command and optional timeout) per
    ${CLAUDE_PLUGIN_ROOT}/docs/plan-graph.md. Omitted when the project has
    no gates>

   ## Dependency Graph
   <Mermaid diagram preferred. Mark dependency types: solid arrows for logical,
    dashed for testing, dotted for design/knowledge. Draw external gates as nodes
    with solid arrows to the units they gate. ASCII acceptable when Mermaid is not
    renderable in the target context.>

   ## Execution Order
   <Ordered list with rationale for non-obvious ordering choices.>

   ## Parallel Streams

   ### 2 workers
   <stream assignments, scope boundaries, and synchronisation points>

   ### N workers
   <maximum parallelism at each phase>

   ## Critical Path
   <longest dependency chain with total effort estimate>

   ## Meta-Tasks
   <test harness, debug tooling, tuning infrastructure — work belonging to no
    single unit>

   ## Standing Obligations
   <obligations spanning units — data kept while work happens, content
    contributions, deferred completeness criteria — each naming the units that
    carry it; an obligation no unit in this plan can discharge is a
    debt-register row or another plan's obligation, not a standing obligation
    here>

   ## Risk Registry
   <residual per-unit and cross-cutting risks, one line each: `R<n> <unit or
    cross-cutting> — <one clause>`; gates recorded as edges do not repeat
    here. A risk closed by a completion or an amendment moves to the archive
    at the next re-plan.>

   ## Plan Maintenance
   <amendment record in a fixed compact form, one entry per event, no restated
    statements — the completion reports and the signal ledger hold those:
    `### <date> — <unit> complete` with the one line
    `Report: <path> @ <sha>.`, followed by ` Ledger: <file> wave <n>.` where
    a signal ledger exists;
    `### <date> — re-plan wave <n> (<item ids>)` or
    `### <date> — decisions <ids>`, each followed by one line per amendment
    `<kind> <unit> — <one clause>`; outside the workflow, also one line
    `check <unit> — <outcome>` per re-plan check that ran and changed
    nothing. On each unit completion,
    run the re-plan step before starting its dependants.>
   ```

   Amendment kinds: `deferral`, `scope-reassignment`, `new-edge`, `done-condition`, `execution-order`, `unit-added`, `unit-removed`, `gate-added`, `gate-removed`, `gate-repin`. The workflow reads the same vocabulary. The `gate-*` kinds concern units' external gates, never the gate commands block. Three carry rules:

   - `unit-removed` applies to an incomplete unit only; a completed unit's stub stays. Delete the entry, the block unit, and both execution-order entries. Each dependant's edge on it moves to the removed unit's own dependencies wherever ordering or a scope overlap needs it. The clause says why, where its scope and publishes went, and names any unmerged branch it leaves. Re-home or close each risk and standing obligation that names it, and re-run the quality check over its dependants.
   - `gate-added` and `gate-removed` apply to incomplete units, entry and block together; an added gate's clause gives its kind, and its pin for an environment gate.
   - `gate-repin`: a decision re-pins an environment gate.

   A unit or gate that a decision-log `clears` entry names is not removed: the load stops on a clearance whose target is gone. Under the workflow, the re-plan's verdict on that item is `cannot`.

   A plan the workflow drives has a companion archive, `docs/plans/<name>-archive.md`, append-only: completed unit entries under `## Work Units`, closed risks under `## Closed risks`. Nothing in the plan is struck through or annotated as done.

6. **Present to the requester** for review if running interactively. The plan is the primary output — work items derive from it, not the other way around.

   If the requester requests changes: apply them and re-present. If after two rounds of revision the goal itself is changing materially (new components, changed scope), re-run Phase 1 on the revised goal rather than patching the plan incrementally. If running non-interactively, proceed directly to step 7 (Commit) after the quality check.

7. **Commit** the decomposition plan document — see [Commit discipline](#commit-discipline).

### Re-plan on unit completion

When a unit completes, diff its recorded deviations, deferrals, and discoveries against every downstream unit's scope, dependencies, and done condition. A deviation that adds work to a later unit, reassigns work between units, or leaves an interface unimplemented is a plan change, not an implementation note: amend the plan (entry and `## Graph` block together), re-run the Phase 2 step 4 quality check over the remaining units, and record the amendment in Plan Maintenance. Deferrals are the highest-yield case: "X is deliberately not here, it belongs to unit Y" is simultaneously a scope addition to Y, a possible new edge, and — if Y shares a file with a unit scheduled beside it — a broken parallel-stream claim.

A completion report showing distress — context near exhaustion, design decisions taken mid-implementation, a heavy deviation or deferral count — triggers this step even when no deviation touches downstream scope: re-run the quality check over the remaining units, examining the [size budget](#unit-size-budget) item first for units that resemble the completed one on its load factors. Record the outcome as the re-plan's verdict on the distress item (held in the ledger) under the workflow, otherwise as a `check` line in Plan Maintenance.

Invoked with an existing plan and completion reports (re-plan mode), this step is the whole workflow: apply the diff, re-run the quality check, re-present.

Under the `lsf-implement` workflow the plan's size is bounded by moving what is finished out of it:

- **Completed entries are archived at merge.** The merge stage moves a completed unit's `### <id>` entry verbatim to `docs/plans/<name>-archive.md` and leaves the one-line stub `### <id> — complete at <merge SHA>; entry archived to <archive>; report <path>`. The unit's `## Graph` block entry is untouched apart from `complete: true`. A re-plan never restores or rewrites a stub.
- **Closed risks are archived at re-plan.** A Risk Registry entry retired by this pass's completions or amendments moves to the archive under `## Closed risks`, cited by id and one clause.
- **The gate commands block is fixed for the run.** A wave re-plan never edits it: the Go runtime stops with `declared-gates-changed` when it changes. The only sanctioned change is a requester's decision, which the decision re-plan applies at the next launch, before the block is read.
- **Plan Maintenance takes the compact form** in the template above: heading, one line per amendment, nothing restated.

## Commit discipline

See [commit discipline](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#commit-discipline). Additional breakdown-specific guidance:

- Commit message should summarise: number of work units, critical path length, and key risks identified.
- The `## Graph` block and the unit entries change in the same commit.
