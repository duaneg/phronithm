---
description: "Multi-session feature delivery: parallel research, iterated architecture document, then a workflow-driven implementation loop over the decomposition plan. Use when the feature spans multiple systems, the design space is open, or scope exceeds a single `phronithm:feature` invocation."
allowed-tools: Read, Grep, Glob, Bash, Edit, Write, Task, Agent, Skill, AskUserQuestion, ToolSearch, Workflow, TaskStop
---

# Large-Scale Feature

Deliver a large, complex, multi-session feature from initial idea to merged implementation. Distinct from `/phronithm:feature` in three ways: parallel research before design (subagents for domain investigation), an architecture document phase (standalone design doc iterated until critique findings diminish to marginal), and an explicit multi-session structure (deliberate context handoffs between phases prevent accumulated brainstorming context from contaminating design thinking; Phase 3 runs as a workflow whose state is committed documents).

Use this skill when: the feature requires architectural decisions that span multiple existing systems, the design space is genuinely open (not just applying an established pattern), or the scope is large enough that a single `/phronithm:feature` invocation would be impractical.

## Interface

### Requires

- **request**: Feature request, goal, or problem statement. Vaguer input leads to more research in Phase 1.

### Produces

- **architecture-brief** (Phase 1): Structured writing plan for the architecture document — requirements, research synthesis, candidate approaches, and architectural direction. Self-contained enough for Phase 2 to proceed from it alone. Committed to the project repo at `docs/architecture/<name>-brief.md` or project equivalent. Its requirements and success criteria are the feature's **request record**: what Phase 3 means by a decision that changes the request rather than the architecture.
- **research-notes** (Phase 1): Findings from parallel research subagents, addressing specific unknowns identified during brainstorming. Committed to the project repo under `docs/research/` or project equivalent.
- **architecture-document** (Phase 2): High-level architecture overview, iterated until critique findings diminish to marginal. Committed to version control under `docs/architecture/` or project equivalent.
- **decomposition-plan** (Phase 2): Work decomposition produced by `/phronithm:breakdown`: work units with done conditions, external gates with plan-time pins, dependency graph, execution order, parallel streams, critical path, meta-tasks, standing obligations, risk registry, and a plan-maintenance record. Committed to the repo. A living document through Phase 3 — amended by breakdown's re-plan mode as units complete.
- **implementation-plan**, **completion-report**, **signal-ledger**, **decision-log** (Phase 3): committed by `phronithm:lsf-implement`'s agents, one plan and one report per unit; the ledger and the decision log per feature. See the design's *Surfacing* section.
- **reconciliation-record** (Phase 2 re-entry only): `docs/plans/<name>-reconciliation.md`. One row per unit the previous plan marks `complete: true` — unit id, `stands` or `superseded` under the amended architecture, the evidence (its completion report), and for `superseded` the architecture section that supersedes it and whether the remedy is to adapt or revert. Then every unit that did not merge with the `worktree-wf_*` branch holding its work, and a closing section giving each standing obligation of the previous plan a disposition: discharged with evidence, or carried into the new plan. Input to the fresh `phronithm:breakdown`.

### Consumes

- **architecture-brief** (Phase 2 input): Primary design input for the architecture document.
- **architecture-document** and **decomposition-plan** (Phase 3 input): read by `phronithm:lsf-implement` at a commit SHA; the plan's `yaml lsf-plan` block is the graph the workflow schedules from.
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

**Begin this phase in a fresh session.** **Re-entering after a Phase 3 stop?** Skip steps 1–2 and follow *Re-entry after a Phase 3 stop* below.

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
5. **Commit** the architecture document and any updated project docs. Name the integration branch — the branch Phase 3's units merge into — and create it from this commit (`git switch -c <branch>`) if it does not exist.
6. **Decompose into work plan**: Invoke `/phronithm:breakdown` with the architecture document as the goal input. If gaps in the architecture document prevent decomposition, stop and report the gaps rather than proceeding with assumptions. Review the decomposition plan — including any quality-check discharge flags on units, which mark unresolved unknowns left to direct executor judgement: confirm each flagged unknown is acceptable to carry into execution or send it back for resolution. Then confirm the plan with the requester before proceeding to Phase 3.

**Re-entry after a Phase 3 stop.** The inputs are in the startup prompt `phronithm:lsf-drive` presented. Work on the integration branch, in this order:

a. When a decision changes the request record, amend the architecture brief's requirements and success criteria first and commit it.
b. Amend the architecture document where the decision or the invalidation fields change what a section decides, and run Phase 2 step 3 over the amendment. A stop that leaves the document unchanged — an unevaluable done condition, a decision that only changes the request record — skips both; record in step d that the architecture stands.
c. Run Phase 2 steps 4–5.
d. Write the reconciliation-record (Produces above) over the plan at the prompt's SHA: its `complete: true` units, whose completion report paths are in its Plan Maintenance entries and archived stubs; its unmerged units and their `worktree-wf_*` branches; its standing obligations. Commit it.
e. Run Phase 2 step 6 with `phronithm:breakdown` given the amended architecture document, the reconciliation record, the previous plan at the prompt's SHA (its Standing Obligations, Risk Registry, and archive), the integration branch, and every other decision the prompt carries. The fresh plan replaces `docs/plans/<name>-breakdown.md` in place; the archive keeps the retired entries under their old ids. Breakdown marks a unit complete on standing work only after evaluating its done condition against the integration branch; confirm each such mark cites that evaluation, and send an unevaluable condition back to the requester.
f. **Retire stale `clears` entries.** Every `clears` entry in the decision log is re-checked against the plan at each load, so one naming a unit the new plan does not have stops the load for good. Edit `docs/plans/<name>-decisions.md`: for each such entry, delete the `clears` key from that decision and append "(clearance retired with unit `<old id>`, <date>)" to its `decision` text. A gate the new plan still carries is re-cleared by a fresh decision at the next stop.
g. Commit the decision-log edit, then hand off to Phase 3 with the new plan SHA (`git rev-list -1 HEAD -- docs/plans/<name>-breakdown.md`) and the unscribed rows from the prompt.

Exit criteria: Architecture document committed. Decomposition plan committed. Reconciliation record committed (re-entry only). **Architecture document critique has run** (step 3) and no Critical findings remain — do not proceed to handoff without completing the critique step. The architecture document addresses any concerns flagged during the Phase 1 critique of the architecture brief.

**Phase 2 complete. Recommend starting a new session for Phase 3.**

Present the following startup prompt to the requester in a fenced code block:

```
Load the phronithm:large-scale-feature skill and begin Phase 3.
Architecture document: docs/architecture/<name>.md
Architecture brief (request record): docs/architecture/<name>-brief.md
Decomposition plan: docs/plans/<name>-breakdown.md @ <plan SHA>
Integration branch: <branch>
Unscribed ledger rows: <verbatim from the re-entry prompt; omit on a first launch>
```

Substitute the actual feature name, paths, and branch; the plan SHA is `git rev-list -1 HEAD -- <plan path>`. Wait for user confirmation before continuing.

### Phase 3: Implement work units

**Begin this phase in a fresh session.** Phase 3 runs as the `phronithm:lsf-implement` workflow, driven by `phronithm:lsf-drive`: wave loop over the decomposition plan — plan, critique, implement, review, merge, wave gate, re-plan — stopping only on structural signals and adjudicated architecture invalidation. The session launches the workflow, presents its return, collects the requester's decisions, and relaunches; it does no unit work itself. Design: [lsf-workflow](${CLAUDE_PLUGIN_ROOT}/docs/design/lsf-workflow.md).

Invoke `phronithm:lsf-drive` with the feature name, the architecture document, the architecture brief as the request record, the decomposition plan's commit SHA, the integration branch, and any unscribed ledger rows the startup prompt carried. It runs until the workflow returns `complete`, or hands back when a stop needs Phase 2: an architecture invalidation, a done condition unevaluable as written, or a re-plan item whose answer changes a decision in the architecture document or the request record; so does an architecture note the requester judges an invalidation. Such a stop is handled by Phase 2's re-entry step in a fresh session; Phase 3 then relaunches from the new plan SHA.

**Phase termination** is the workflow's: meta-tasks become units or direct edits, and every standing obligation is discharged or carried with a named carrier before it returns `complete`. The feature is complete when the requester accepts the final digest.

## Review gates

After Phases 1 and 2:

- Does the output meet the phase's exit criteria?
- Is the handoff document self-contained — could a fresh session proceed from it alone? Rate `sufficient` / `partial` / `insufficient`.
- Have assumptions made in earlier phases been invalidated?
- Are there new ambiguities? Resolve before proceeding.

In Phase 3 the wave gate asks the invalidation question once per wave and stops on a complete invalidation. What it saw short of one is the digest's architecture notes; `phronithm:lsf-drive` puts them to the requester after every return, and a note the requester judges an invalidation re-enters Phase 2 by the same route as a stop.

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

Size the architecture document to what decomposition and Phase 3's wave gate need: where the architecture is already settled (a new module in an established pattern), the required sections are short, but every one is present and the document is committed — Phase 3 reads it at a SHA and adjudicates invalidation against it. Do not abbreviate Phase 1 research — even familiar codebases have integration surprises at scale.

If Phase 2 reveals new questions that require targeted research, return to Phase 1 for a focused research pass before completing the architecture document.

## Termination

The skill terminates when the requester accepts the final digest (Phase 3). It can also terminate early: the requester withdraws the feature, investigation reveals it infeasible, or scope grows beyond what was agreed. On early exit in Phase 3, the last workflow return and the signal ledger already record the state; unmerged `worktree-wf_*` branches hold in-flight work. A later re-entry relaunches from the last plan SHA.

## Commit discipline

See [commit discipline](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#commit-discipline).
