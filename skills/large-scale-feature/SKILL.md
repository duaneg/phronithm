---
description: "Multi-session feature delivery: parallel research, iterated architecture document, then per-increment implementation. Use when the feature spans multiple systems, the design space is open, or scope exceeds a single `phronithm:feature` invocation."
allowed-tools: Read, Grep, Glob, Bash, Edit, Write, Task, Agent, Skill, AskUserQuestion
---

# Large-Scale Feature

Deliver a large, complex, multi-session feature from initial idea to architecture document and implementation plan. Distinct from `/phronithm:feature` in three ways: parallel research before design (subagents for domain investigation), an architecture document phase (standalone design doc iterated until critique findings diminish to marginal), and an explicit multi-session structure (deliberate context handoffs between phases prevent accumulated brainstorming context from contaminating design thinking).

Use this skill when: the feature requires architectural decisions that span multiple existing systems, the design space is genuinely open (not just applying an established pattern), or the scope is large enough that a single `/phronithm:feature` invocation would be impractical.

## Interface

### Requires

- **request**: Feature request, goal, or problem statement. Vaguer input leads to more research in Phase 1.

### Produces

- **architecture-brief** (Phase 1): Structured writing plan for the architecture document — requirements, research synthesis, candidate approaches, and architectural direction. Self-contained enough for Phase 2 to proceed from it alone. Committed to the project repo at `docs/architecture/<name>-brief.md` or project equivalent.
- **research-notes** (Phase 1): Findings from parallel research subagents, addressing specific unknowns identified during brainstorming. Committed to the project repo under `docs/research/` or project equivalent.
- **architecture-document** (Phase 2): High-level architecture overview, iterated until critique findings diminish to marginal. Committed to version control under `docs/architecture/` or project equivalent.
- **decomposition-plan** (Phase 2): Work decomposition with dependency ordering, parallelism analysis, and execution guidance. Produced by `/phronithm:breakdown`. Committed to the repo.
- **implementation-plan** (Phase 3): Via `/phronithm:feature` Phases 1–3 for the first increment.

### Consumes

- **architecture-brief** (Phase 2 input): Primary design input for the architecture document.
- **architecture-document** (Phase 3 input): Design input for `/phronithm:feature`.
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
7. **Critique the architecture brief** — this step catches structural bugs that are expensive to discover later. Do not skip or compress it. Spawn a Sonnet Agent to invoke `/phronithm:critique` on the architecture brief. The agent prompt must include an explicit completion requirement: "Continue through to completion, including all critique passes and fixes — do not stop mid-task to wait for further instruction." Present critical and significant findings to the requester and confirm the proposed fixes before implementing them. Fix all critical and significant findings. Minor findings may be fixed without review. When the first critique round returns Critical or Significant findings, run a second pass after applying fixes — each structural fix can introduce new inconsistencies that a single pass won't catch. Stop when all remaining findings are minor — those can be addressed inline without another full pass. Do not run another full critique pass to verify inline fixes; the fix is the exit. If the critique summary indicates a "systemic problem", stop immediately and present the issue to the requester — iterating will not resolve a structural defect. If significant findings persist after 4 passes without a systemic-problem summary, stop and present the remaining findings to the requester.
8. **Document capture**: Before committing, verify that all decisions, research findings, and the architecture brief are present as files in the repo. Nothing significant should exist only in memory or conversation context — write it to a file first.
9. **Commit** the architecture brief and research notes. This is the point where files become version-controlled.
10. **Save** environmental learnings only (tooling quirks, environment-specific discoveries) to project memory — they benefit future sessions but do not belong in version control.

Exit criteria: The problem is understood, candidate approaches are documented, research is committed to the repo, and the architecture brief is self-contained enough for a fresh session to proceed with Phase 2 from the document alone. **Architecture brief critique has run** (step 7) and no Critical or Significant findings remain — do not proceed to handoff without completing the critique step.

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
3. **Critique the architecture document** — this step catches structural bugs that are expensive to discover during implementation. Do not skip or compress it. Spawn a Sonnet Agent to invoke `/phronithm:critique` on the document. The agent prompt must include an explicit completion requirement: "Continue through to completion, including all critique passes and fixes — do not stop mid-task to wait for further instruction." Present critical and significant findings to the requester and confirm the proposed fixes before implementing them. Fix all critical and significant findings. Minor findings may be fixed without review. Findings progress through severity stages across passes: critical issues first, then significant oversights, then minor issues. When the first critique round returns Critical or Significant findings, run a second pass after applying fixes — each structural fix can introduce new inconsistencies that a single pass won't catch. Stop when all remaining findings are minor — those can be addressed inline without another full pass. Do not run another full critique pass to verify inline fixes; the fix is the exit. If the critique summary indicates a "systemic problem", stop immediately and present the issue to the requester — iterating will not resolve a structural defect. If significant findings persist after 4 passes without a systemic-problem summary, stop and present the remaining findings to the requester — continued iteration at that point signals a structural problem that needs requester input, not another pass.
4. **Update** existing project documentation for consistency: check whether the architecture affects concepts in `CLAUDE.md`, existing design docs, or key-file listings. Update stale references.
5. **Commit** the architecture document and any updated project docs.
6. **Decompose into work plan**: Invoke `/phronithm:breakdown` with the architecture document as the goal input. If gaps in the architecture document prevent decomposition, stop and report the gaps rather than proceeding with assumptions. Review the decomposition plan, then confirm it with the requester before proceeding to Phase 3.

Exit criteria: Architecture document committed. Decomposition plan committed. **Architecture document critique has run** (step 3) and no Critical or Significant findings remain — do not proceed to handoff without completing the critique step. The architecture document addresses any concerns flagged during the Phase 1 critique of the architecture brief.

**Phase 2 complete. Recommend starting a new session for Phase 3.**

Present the following startup prompt to the requester in a fenced code block:

```
Load the phronithm:large-scale-feature skill and begin Phase 3. Architecture document: docs/architecture/<name>.md
```

Substitute the actual feature name and path. Wait for user confirmation before continuing.

### Phase 3: Plan MVP

**Begin this phase in a fresh session.** Load the architecture document and decomposition plan from Phase 2.

1. **Load** the architecture document and decomposition plan. Review the work units. The architecture document replaces `/phronithm:feature`'s Phase 1 exploration — do not re-explore what has already been settled.
2. **Select** the first work unit from the decomposition plan's execution order — typically the most foundational unit, or the one that reduces the most risk early. The decomposition plan's ordering rationale should guide this choice.
3. **Invoke `/phronithm:feature`** starting at Phase 1, using the architecture document as the primary design input. Phase 1 of `/phronithm:feature` here is scope confirmation only: load the architecture document, confirm the selected work-unit scope with the requester, and proceed to design. Requirements are already established — do not repeat codebase exploration that the architecture document has already covered.
4. Complete `/phronithm:feature` through Phase 3 (implementation plan approved and saved).
5. **Proceed** with `/phronithm:feature` Phases 4–5 for implementation and review.

Exit criteria: Implementation plan is approved, saved to a durable location, and self-contained — a fresh session can implement from it without referencing the transcript.

**Phase 3 complete. Recommend starting a new session for implementation.**

Present the following startup prompt to the requester in a fenced code block:

```
Load the phronithm:feature skill and begin Phase 4. Implementation plan: docs/plans/<name>-plan.md
```

Substitute the actual feature name and path. Wait for user confirmation before continuing.

Proceed with `/phronithm:feature` Phases 4–5 for implementation and review. `/phronithm:feature` Phase 5 includes the retrospective — ensure it runs. If Phase 5 will not begin in this session, explicitly instruct the requester: "Phase 5 (Review, including retrospective) must be run before this feature is complete."

## Review gates

After each phase:

- Does the output meet the phase's exit criteria?
- Is the handoff document self-contained — could a fresh session proceed from it alone? Rate `sufficient` / `partial` / `insufficient`.
- Have assumptions made in earlier phases been invalidated?
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

## Commit discipline

See [commit discipline](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#commit-discipline).
