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
  - A list of existing work items to re-plan (issue numbers, TODO items, or equivalent)

**Pre-flight check**: Before beginning analysis, verify the goal is specific enough to decompose. A valid goal must have a clear done condition — you must be able to answer "how would we know this is complete?" If the goal is too vague to answer that question, stop and ask for clarification. Do not begin analysis on an underspecified goal.

### Produces

- **decomposition-plan**: Document at `docs/plans/<feature-name>-breakdown.md` (or project equivalent). Contains:
  - Work units with classification metadata and done conditions
  - Dependency graph (multiple dependency types, Mermaid format preferred)
  - Execution order with rationale
  - Parallel streams for 2-worker and N-worker scenarios
  - Critical path analysis
  - Meta-tasks (test harness, debug tooling, verification checkpoints)
  - Risk registry

### Consumes

- **architecture-document** (from phronithm:large-scale-feature Phase 2): When present, requirements and design are already settled — Phase 1 step 1 is abbreviated to reading the document and extracting components. Steps 2–5 run in full.
- **impact-map** (from phronithm:impact-analysis): When present, informs scope boundaries and identifies cross-cutting concerns.

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
| `phronithm:feature` | Independent functionality |
| `integration` | Connects other units or systems |
| `design` | A decision that must be resolved before dependent units proceed |
| `phronithm:refactor` | Restructuring existing code to support the feature |
| `bug-fix` | Fixing a latent bug exposed or required by the feature |
| `polish` | Iterative tuning, visual/UX refinement |

**Risk** — `low` / `medium` / `high` with rationale. Low: well-specified, mechanical. Medium: some unknowns or integration surface. High: creative work, novel mechanics, cross-cutting concerns.

**Clarity** — how well-understood the approach is:
- `specified`: Clear done condition, known implementation approach
- `exploratory`: Requires investigation, prototyping, or design work before the implementation approach is clear

Exploratory units have higher effort variance. When a unit is exploratory, consider whether a preceding `design` unit should resolve the uncertainty first.

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

*Structural iteration* is better addressed by the `exploratory` clarity flag, higher effort estimates, and risk flagging. Pre-designing away structural iteration is often wasted effort — you're designing before you have the information that drives the changes.

**Effort** — rough guide, not a commitment:
- `small`: <1h focused work
- `medium`: 1–4h
- `large`: 4h+

### Dependency types

Dependencies between work units come in several forms. The first four are always analysed; the remainder are prompted for when relevant.

**Always analysed:**

| Type | Description | Example |
|------|-------------|---------|
| **Logical** | A defines types/APIs that B consumes. Strict ordering. | Types & plumbing → arena physics |
| **Testing** | A must be functional to *verify* B, even if B doesn't logically depend on A's internals. Plans that look parallel on paper but are serial in practice create false expectations — record testing dependencies and plan around them, don't wish them away. | Transitions → arena physics (can't test arena without entering it) |
| **Design** | A decision must be made before B can proceed. | Visual effects design → renderer changes |
| **Knowledge** | Completing A produces understanding needed to design/implement B. | Prototype/spike → production implementation |

**Prompted — external factors:**

| Type | Prompt |
|------|--------|
| **Organisational** | "Are any units gated by specific people, teams, or external approvals?" |
| **Release** | "Do any units depend on a prior release or deployment?" |
| **Data** (runtime state, migrations, infrastructure) | "Do any units depend on migrations, data pipelines, or environment setup?" |

These cannot be discovered from the codebase — ask explicitly in Phase 1 step 4. If running non-interactively, record as unknown and flag as risks.

### Work unit done conditions

Every work unit must have a verifiable done condition — a statement that can be evaluated true/false without the work unit's author making a judgement call. Examples:
- "All tests in `tests/arena/` pass"
- "The configuration schema validates against the fixture suite with zero errors"
- "Design question resolved and decision recorded in plan document"

Exploratory units may have a process done condition: "Spike complete: approach decided and documented." The decision itself is the artefact.

### Scope

Each work unit lists the files, modules, or directories it will touch. For teams (human or agent), scope boundaries determine:
- Which units can be worked on concurrently without conflicts
- The boundary of responsibility for a given worker
- What to review when the unit is complete

Units with overlapping scope must either be serialised or have a concrete coordination protocol defined (e.g., "Unit B waits for Unit A's merge before starting", or "file X is owned by Unit A — Unit B uses the interface but does not modify the file"). Noting overlap without specifying the protocol is not sufficient.

## Workflow

### Startup

Follow the [pre-flight](${CLAUDE_PLUGIN_ROOT}/docs/pre-flight.md) check. Do not continue until it passes.

Read `CLAUDE.md` for the target project. Check dependency manifests to identify the tech stack.

**Goal validation**: Confirm the goal passes the pre-flight check stated in [Requires](#requires). If the goal is a vague description that cannot pass the "how would we know this is complete?" test, stop here and ask for clarification before proceeding.

### Phase 1: Analyse

Understand the feature's structure before decomposing it.

1. **Load context**. Depending on input type:
   - *Feature description*: Read relevant project documentation and codebase. Identify the major components, integration points, and boundaries.
   - *Architecture document*: Read the document. Extract the component list, data model, and integration points. The requirements and design are settled — proceed directly to step 2.
   - *Existing work items*: Fetch each item with full history. Build a picture of the intended scope and decisions already taken.

2. **Identify work units**. Each unit must be:
   - **Individually completable**: bounded session — roughly a half-day or single agent context window
   - **Verifiable**: done condition stated now (see [Work unit done conditions](#work-unit-done-conditions))
   - **Clearly scoped**: files/modules enumerable upfront

   Record testing dependencies explicitly in step 4 — a unit may be implementable without others but not verifiable until a dependency is complete.

   Split on **responsibility boundaries**, not size. Prefer fewer, larger units — coordination overhead grows faster than task-size overhead.

   **Convergence**: Complete when every component from step 1 has at least one unit and every integration point is covered. If new components keep surfacing after 2–3 passes, the feature needs splitting into phases.

3. **Classify each unit** on all five axes (type, risk, clarity, iteration profile, effort). For each unit marked `iterative`, determine whether the iteration is tuning or structural, and whether a companion `design` unit is needed for tuning infrastructure.

4. **Map dependencies**. For each unit, check which other units it depends on across all four always-analysed types. For small decompositions (≤8 units), check all pairs. For larger decompositions, check each unit against all units that logically precede it — full pairwise checking at scale is impractical.

   Then ask: "Are any of these units gated by external factors — specific people or teams, prior releases, data migrations, or environment setup?" If running non-interactively, record external dependency types as unknown and flag them in the risk registry.

5. **Identify meta-tasks**. Debug tooling, test harnesses, and verification checkpoints are real work — anticipating them prevents surprise scope growth and ensures they're budgeted. Work that falls outside any single unit but is necessary:
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
   - For each stream, state the **scope boundary**. Streams with overlapping scope must be either serialised or assigned a concrete coordination protocol — do not leave overlap unresolved.

3. **Build the critical path**. The longest chain of dependent units — the minimum wall-clock time regardless of parallelism. Flag it explicitly.

4. **Quality check** before drafting the document. Verify:
   - Every major component identified in Phase 1 is covered by at least one work unit.
   - Every work unit has a verifiable done condition.
   - Every `high`-risk unit is either placed early in execution order or has a risk mitigation note in the Risk Registry.
   - Every `foundation`-type unit appears before its dependants in the execution order.
   - All overlapping-scope parallel streams have a coordination protocol.

5. **Draft the decomposition plan document**. Structure:

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
   - **Scope**: <files/modules>
   - **Depends on**: <unit-names (dependency-type) or "none">
   - **Done condition**: <verifiable statement>
   - **Description**: <what this unit does>

   ## Dependency Graph
   <Mermaid diagram preferred. Mark dependency types: solid arrows for logical,
    dashed for testing, dotted for design/knowledge. ASCII acceptable when Mermaid
    is not renderable in the target context.>

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
   <test harness, debug tooling, tuning infrastructure, verification checkpoints>

   ## Risk Registry
   <per-unit risks and cross-cutting risks that span multiple units>
   ```

6. **Present to the requester** for review if running interactively. The plan is the primary output — work items derive from it, not the other way around.

   If the requester requests changes: apply them and re-present. If after two rounds of revision the goal itself is changing materially (new components, changed scope), re-run Phase 1 on the revised goal rather than patching the plan incrementally. If running non-interactively, proceed directly to Phase 3 after the quality check.

7. **Commit** the decomposition plan document to the repo on the current branch. Commit message should summarise: number of work units, critical path length, and key risks identified.
