---
description: "Multi-session feature delivery: parallel research, iterated architecture document, then per-increment implementation. Use when the feature spans multiple systems, the design space is open, or scope exceeds a single `phronithm:feature` invocation."
allowed-tools: Read, Grep, Glob, Bash, Edit, Write, Task, Agent, Skill, AskUserQuestion, ToolSearch, EnterPlanMode, ExitPlanMode
---

# Large-Scale Feature

Deliver a large, complex, multi-session feature from initial idea to architecture document and implementation plan. Distinct from `/phronithm:feature` in three ways: parallel research before design (subagents for domain investigation), an architecture document phase (standalone design doc iterated until critique findings diminish to marginal), and an explicit multi-session structure (deliberate context handoffs between phases prevent accumulated brainstorming context from contaminating design thinking; Phase 3 loops per work unit through committed-document handoffs).

Use this skill when: the feature requires architectural decisions that span multiple existing systems, the design space is genuinely open (not just applying an established pattern), or the scope is large enough that a single `/phronithm:feature` invocation would be impractical.

## Interface

### Requires

- **request**: Feature request, goal, or problem statement. Vaguer input leads to more research in Phase 1.

### Produces

- **architecture-brief** (Phase 1): Structured writing plan for the architecture document — requirements, research synthesis, candidate approaches, and architectural direction. Self-contained enough for Phase 2 to proceed from it alone. Committed to the project repo at `docs/architecture/<name>-brief.md` or project equivalent.
- **research-notes** (Phase 1): Findings from parallel research subagents, addressing specific unknowns identified during brainstorming. Committed to the project repo under `docs/research/` or project equivalent.
- **architecture-document** (Phase 2): High-level architecture overview, iterated until critique findings diminish to marginal. Committed to version control under `docs/architecture/` or project equivalent.
- **decomposition-plan** (Phase 2): Work decomposition produced by `/phronithm:breakdown`: work units with done conditions, external gates with plan-time pins, dependency graph, execution order, parallel streams, critical path, meta-tasks, standing obligations, risk registry, and a plan-maintenance record. Committed to the repo. A living document through Phase 3 — amended by breakdown's re-plan mode as units complete.
- **implementation-plan** (Phase 3): Via `/phronithm:feature` Phases 1–3, one per work unit, each carrying the unit's work-unit context in its handoff footer.
- **completion-report** (Phase 3): One per completed work unit, produced by `/phronithm:feature` Phase 5 in work-unit mode. Input to breakdown's re-plan mode at the next Phase 3 pass.

### Consumes

- **architecture-brief** (Phase 2 input): Primary design input for the architecture document.
- **architecture-document** (Phase 3 input): Design input for `/phronithm:feature`.
- **decomposition-plan** (Phase 3 input): Source of work units, execution order, and completion state (via its plan-maintenance record).
- **completion-report** (Phase 3 re-entry input, from `/phronithm:feature` Phase 5): Consumed by `/phronithm:breakdown` re-plan mode before the next unit is selected.
- **impact-map** (from phronithm:impact-analysis): Consumed in Phase 1 when the change involves renaming, type-changing, or removing exported or serialised fields — run before identifying candidate approaches.

### Complexity drivers

- This is an inherently high-complexity, multi-session workflow.
- Research volume: the more novel the domain, the more research passes needed.
- Codebase integration: the more existing systems the feature must coordinate with, the deeper Phase 1 exploration needs to go.
- Architectural novelty: no direct precedent in the codebase means more critique passes in Phase 2.

## Workflow

Each phase ends with a version-controlled commit and a handoff document. **Start a new session at the phase breaks marked below.** This prevents accumulated context from biasing subsequent design thinking — it is a feature, not a limitation.

### Startup

Follow the [pre-flight](${CLAUDE_PLUGIN_ROOT}/docs/pre-flight.md) check. Do not continue until it passes.

Before proceeding:

1. Read `CLAUDE.md` (project instructions — architecture overview, conventions, key files).
2. Check dependency manifests (`package.json`, `pyproject.toml`, `pom.xml`, or equivalent) to identify the tech stack and runtime dependencies.

### Phase 1: Brainstorm

Understand the problem space thoroughly before proposing solutions.

1. **Capture** the request. Restate it in your own words. Confirm understanding with the requester.
2. **Clarify** scope, constraints, and success criteria. Start with open discussion to surface context the requester wants to share — do not open with a structured questionnaire. Use AskUserQuestion for specific trade-off decisions only after the open discussion has run its course, not as the opening move; users often have richer context than structured options can capture. Establish what specific, observable outcomes distinguish "done" from "not done". When a requirement is stated, probe for acceptable deviations: "Is that a hard ceiling or a typical target?" — the nuance shapes the design space significantly. Flag unresolved ambiguities explicitly — do not proceed with silent assumptions. Steps 1–2 constitute the brainstorming discussion; complete them before moving to Research.
2a. **Elicit implementation constraints.** Before spawning research agents, explicitly ask about constraints that shape the design space but are easy to overlook in a requirements discussion:
   - Language / runtime preferences (especially if non-Python or non-default for the project)
   - Performance targets (latency, throughput, memory)
   - Build system or deployment constraints
   - Dependencies to avoid or prefer

   These are cheap to state early and expensive to discover late. Prompt: "Any implementation-level preferences — language, performance targets, build constraints — that should shape the research?"
3. **Research**. From the brainstorming discussion, identify the specific unknowns that must be resolved before design can proceed. Spawn one subagent per unknown — targeted questions, not a standard template of research dimensions. Do not spawn agents for things already well understood. Before spawning agents, create any output directories that don't yet exist (e.g. `docs/research/`). Research agent prompts must include: "Return all findings inline in your response. Do not attempt to write files — background agents lack write permissions and file writes will be silently denied. Do not spawn further subagents — gather what you need directly and report back." When each research agent completes, the orchestrating session writes the findings to `docs/research/` (or project equivalent — check `CLAUDE.md` for the project's documentation conventions) immediately — not to memory.
   Flag all domain-specific findings (game mechanics, API behaviour, framework internals) as "unverified — requires runtime/in-game/API confirmation" in the research output. Research agents can produce plausible but incorrect domain facts. Do not build architecture decisions on unverified domain claims without flagging the verification gap.
   If the change involves **renaming, type-changing, or removing exported or serialised fields**, invoke `/phronithm:impact-analysis` before research — the consumer map shapes the design.
4. **Synthesise**. Read agent outputs yourself — do not rely on summaries alone. Identify: candidate architectural approaches (minimum two; if only one is viable, document explicit justification explaining why alternatives are infeasible), key integration constraints, risks and unknowns, and any library or API assumptions that require runtime verification.
5. **Evaluate** candidates using deliberate competing stances: minimal change (smallest diff, maximum reuse), clean architecture, and pragmatic balance. Collapse when stances converge; the stances are a check, not a ritual.
6. **Document** the architecture brief at `docs/architecture/<name>-brief.md` (or project equivalent). This document is a writing plan for the architecture document — it sets direction and scope for Phase 2. Required contents:
   - Requirements and measurable success criteria (confirmed, or provisional with explicit flags)
   - Scope boundaries and constraints
   - Research synthesis: domain findings, library evaluations, relevant prior art
   - Candidate approaches: for each, tradeoffs, risks, and relevant existing code or patterns
   - Preferred approach with rationale; rejected alternatives with reasons
   - Open unknowns: for each, what information is needed and when it must be resolved (before architecture, before implementation, or during)
7. **Critique the architecture brief** — this step catches structural bugs that are expensive to discover later. Do not skip or compress it. Run the [critique gate](${CLAUDE_PLUGIN_ROOT}/docs/critique-gate.md) on the architecture brief with `critique-design`, applying its default iteration policy. Present Critical findings to the requester and confirm the proposed fixes before implementing them — this skill's multi-session, higher-stakes structure warrants requester sign-off on fixes that the single-session `phronithm:feature` doesn't require. Report Significant and Minor findings; fix them only at the requester's direction, without re-running the gate.
8. **Document capture**: Before committing, verify that all decisions, research findings, and the architecture brief are present as files in the repo. Nothing significant should exist only in memory or conversation context — write it to a file first.
9. **Commit** the architecture brief and research notes. This is the point where files become version-controlled.
10. **Save** environmental learnings only (tooling quirks, environment-specific discoveries) to project memory — they benefit future sessions but do not belong in version control.

Exit criteria: The problem is understood, candidate approaches are documented, research is committed to the repo, and the architecture brief is self-contained enough for a fresh session to proceed with Phase 2 from the document alone. **Architecture brief critique has run** (step 7) and no Critical findings remain — do not proceed to handoff without completing the critique step.

**Phase 1 complete. Recommend starting a new session for Phase 2.** The accumulated brainstorming context will bias architectural thinking. Hand off via the committed architecture brief and research notes only.

Present the following startup prompt to the requester in a fenced code block:

```
Load the phronithm:large-scale-feature skill and begin Phase 2. Architecture brief: docs/architecture/<name>-brief.md
```

Substitute the actual feature name and path. Wait for user confirmation before continuing.

### Phase 2: Architecture

**Begin this phase in a fresh session.** Load the architecture brief and research notes from Phase 1.

1. **Load** the architecture brief and research notes from Phase 1. Read any project documentation they reference. Work from the committed text, not memory of Phase 1.
2. **Produce** the architecture overview document at `docs/architecture/<name>.md` (or project equivalent). Required contents:
   - Problem statement and scope
   - Selected approach with rationale; rejected alternatives with reasons — this is as important as what was chosen
   - Architectural model: key components, their responsibilities, and how they interact
   - Data model: types, representations, important invariants
   - Extension and integration points: where interfaces are stable vs. internal, how new components plug in
   - Open design decisions: questions explicitly deferred, with the information needed to resolve them and when resolution is required
   - Known risks and mitigation strategies
   The document must be readable by a future maintainer without access to the session history.
3. **Critique the architecture document** — this step catches structural bugs that are expensive to discover during implementation. Do not skip or compress it. Run the [critique gate](${CLAUDE_PLUGIN_ROOT}/docs/critique-gate.md) on the document with `critique-design`, applying its default iteration policy. Present Critical findings to the requester and confirm the proposed fixes before implementing them — this skill's multi-session, higher-stakes structure warrants requester sign-off on fixes that the single-session `phronithm:feature` doesn't require. Report Significant and Minor findings; fix them only at the requester's direction, without re-running the gate.
4. **Update** existing project documentation for consistency: check whether the architecture affects concepts in `CLAUDE.md`, existing design docs, or key-file listings. Update stale references.
5. **Commit** the architecture document and any updated project docs.
6. **Decompose into work plan**: Invoke `/phronithm:breakdown` with the architecture document as the goal input. If gaps in the architecture document prevent decomposition, stop and report the gaps rather than proceeding with assumptions. Review the decomposition plan — including any quality-check discharge flags on units, which mark unresolved unknowns left to direct executor judgement: confirm each flagged unknown is acceptable to carry into execution or send it back for resolution. Then confirm the plan with the requester before proceeding to Phase 3.

Exit criteria: Architecture document committed. Decomposition plan committed. **Architecture document critique has run** (step 3) and no Critical findings remain — do not proceed to handoff without completing the critique step. The architecture document addresses any concerns flagged during the Phase 1 critique of the architecture brief.

**Phase 2 complete. Recommend starting a new session for Phase 3.**

Present the following startup prompt to the requester in a fenced code block:

```
Load the phronithm:large-scale-feature skill and begin Phase 3. Architecture document: docs/architecture/<name>.md
```

Substitute the actual feature name and path. Wait for user confirmation before continuing.

### Phase 3: Implement work units

**Begin this phase in a fresh session, and re-enter it in a fresh session after each unit completes and again after each re-plan.** Each pass through this phase plans one work unit; the phase repeats until the decomposition plan's execution order is exhausted. The loop's state lives entirely in committed documents — the decomposition plan's plan-maintenance record, each unit's completion-report, and each implementation-plan's work-unit context — because no session context survives the boundaries between passes.

"Invoke" in this phase means loading the skill into the current session via the Skill tool — never a Task/Agent dispatch: subagents cannot reliably write files (see [file writes](${CLAUDE_PLUGIN_ROOT}/docs/subagent-protocol.md#file-writes)), and every artefact this loop depends on is a file write.

This loop is serial — one unit per pass. The plan's parallel-stream analysis serves multi-worker execution outside this skill's loop; when units are executed concurrently by other workers, gather all their completion-reports into a single re-plan invocation (breakdown accepts one or more) before selecting any dependent unit.

1. **Load** the architecture document and decomposition plan. Establish completion state: a unit is complete iff a plan-maintenance completion entry names it. The just-completed unit will not yet appear there — its identity and completion-report path come from the continuation prompt that opened this session; absent that, glob `docs/plans/*-completion.md` and diff against the record. If every unit is complete **and** every completion's re-plan is recorded, go to Phase termination below; if a completion's re-plan is outstanding — including the last unit's — run step 2 for it first.
2. **Re-plan** (on re-entry with a completed unit): invoke `/phronithm:breakdown` in re-plan mode with the decomposition plan and the unit's completion-report, review the amended plan, and confirm it is committed before proceeding — an uncommitted amendment is invisible to the next pass. Do this before selecting further work — a deferral or discovery in the report may change scope, dependencies, or ordering of the remaining units. Skip if the plan-maintenance record already shows this completion re-planned — the normal case on re-entry after the pause below, and also the recovery state after a pass crashed or compacted between re-plan and unit selection; either way, reload the committed documents and continue from the first unsatisfied step.

   **When step 2 actually ran** (not skipped), stop once the amended plan is committed — do not select or execute the next unit in the same pass. Re-plan consumes context that the implementation work following selection needs; running both in one pass is the failure this boundary prevents. Present the following prompt to the requester in a fenced code block and wait for confirmation before continuing:

   ```
   Load the phronithm:large-scale-feature skill and continue Phase 3.
   Architecture document: docs/architecture/<name>.md
   Decomposition plan: docs/plans/<name>-breakdown.md
   ```

   Substitute the actual paths.
3. **Select** the next work unit from the (possibly amended) execution order — on the first pass, typically the most foundational unit or the one that reduces the most risk early. The plan's ordering rationale guides the choice.
4. **Invoke `/phronithm:feature`** with the architecture document as the primary design input and the unit's plan entry as its **work-unit-entry**: the entry verbatim (scope, publishes, depends-on, external gates with pins, done condition), the decomposition-plan path, any standing obligations naming the unit, and the continuation prompt below. `/phronithm:feature` Phase 1 runs in work-unit mode — scope confirmation only; requirements are settled by the architecture document and the unit entry.
5. Complete `/phronithm:feature` through Phase 3 (implementation plan approved, saved, and carrying the work-unit context in its handoff footer). `/phronithm:feature`'s session model then governs implementation and review: its Phase 4 re-verifies the unit's external gates, and its Phase 5 produces the completion-report and presents the continuation prompt that re-enters this phase.

Exit criteria (per pass): implementation plan approved, saved, self-contained, and carrying the work-unit context — a fresh session can implement from it without referencing the transcript.

**Recommend starting a new session for implementation.** The transition itself is `/phronithm:feature`'s standard ExitPlanMode flow — this skill adds no bespoke step — but feature presents no startup prompt for the cleared session, so after plan approval present this one to the requester in a fenced code block:

```
Load the phronithm:feature skill and begin Phase 4. Implementation plan: docs/plans/<name>-plan.md
```

Substitute the actual plan path. Wait for user confirmation before continuing.

The continuation prompt supplied in step 4 — what `/phronithm:feature` Phase 5 presents when the unit completes (substitute actual names and paths):

```
Load the phronithm:large-scale-feature skill and continue Phase 3.
Architecture document: docs/architecture/<name>.md
Decomposition plan: docs/plans/<name>-breakdown.md
Completed unit: <unit-name> (completion report: docs/plans/<unit-name>-completion.md)
```

**Phase termination**: when step 1 finds every unit complete and re-planned, run the plan's remaining meta-tasks — substantial ones (integration testing, tuning infrastructure) as work units through `/phronithm:feature` in work-unit mode, deriving a plan entry with a done condition from the Meta-Tasks section; small ones (documentation updates) directly in this session. Then confirm every standing obligation is discharged. The feature is complete.

## Review gates

After each phase — and after every Phase 3 pass, not once for the phase:

- Does the output meet the phase's exit criteria?
- Is the handoff document self-contained — could a fresh session proceed from it alone? Rate `sufficient` / `partial` / `insufficient`.
- Have assumptions made in earlier phases been invalidated? In Phase 3, specifically: do the completed unit's discoveries invalidate the architecture document? Re-plan amends only the decomposition plan and cannot catch that — return to Phase 2 if so.
- Are there new ambiguities? Resolve before proceeding.

If a gate reveals a problem, return to the earliest affected phase. Do not patch forward.

## Clarification principle

Ambiguity is a defect in understanding. Resolve it at the source:

- **Requirements** ambiguity: ask the requester.
- **Technical** ambiguity: investigate the codebase, read documentation, prototype.
- **Tradeoff** ambiguity: present options with tradeoffs to the requester and let them decide.

Never silently assume. Mark provisional assumptions prominently and gate on requester confirmation before the next phase.

## Memory protocol

Memory is for environmental and ephemeral learnings: tooling quirks, session-specific discoveries that benefit future sessions but do not belong in the codebase. Research findings and design documents belong in the project repo. Do not start documents in memory with the intent to migrate — write to the repo from the start.

## Scaling

For features where the architecture is already settled (e.g. adding a new module to a well-established pattern), Phase 2 can be abbreviated to a targeted design review rather than a full architecture document. Do not abbreviate Phase 1 research — even familiar codebases have integration surprises at scale.

If Phase 2 architecture work reveals new questions that require targeted research, return to Phase 1 for a focused research pass before completing the architecture document. One such return is expected for complex domains; more than one signals the architecture brief underspecified the problem.

## Termination

The workflow terminates when Phase 3's termination condition is met. It can also terminate early: the requester withdraws the feature, investigation reveals it infeasible, or scope grows beyond what was agreed. On early exit, commit whatever exists and record the stop — with the state of any in-flight unit — in the plan-maintenance record; a later re-entry starts from the committed documents, exactly as a normal Phase 3 pass does.

## Commit discipline

See [commit discipline](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#commit-discipline).
