---
description: "Deliver a feature end-to-end: brainstorm, design, plan, implement, review. Use when adding new functionality. For very large multi-session features needing architectural research first, use `phronithm:large-scale-feature` instead."
allowed-tools: Read, Grep, Glob, Bash, Edit, Write, Task, Agent, Skill, ExitPlanMode
---

# Feature: Add New Functionality

Deliver a feature from initial idea to working, tested, documented code.

## Interface

### Requires

- **request**: Feature request, user story, or goal description. Can range from a vague idea ("make search faster") to a detailed specification. Vaguer input means more work in Phase 1.

### Produces

- **problem-statement** (Phase 1): Phase 1 output document. Must contain:
  - Requirements and measurable success criteria — requester-confirmed, or provisional with explicit flags; each criterion tagged with its target rigour level (see Phase 1, step 2)
  - Scope boundaries and constraints
  - Candidate approaches — minimum two; if only one is documented, an explicit justification statement explaining why alternatives are infeasible or unreasonable (not merely that one approach is dominant): for each candidate, tradeoffs, risks, and relevant existing patterns or code
  - Exploration findings: key files, integration points, codebase patterns relevant to the change
- **design-document** (Phase 2): Phase 2 output document. Must contain:
  - Selected approach with rationale; rejected alternatives with reasons
  - Concrete design artefacts: interfaces, data structures, key algorithms, error handling strategy, integration points
  - Open risks and unknowns: for each, resolution method and timing (before implementation / during / after)
  - API verification record: for each third-party or undocumented API assumption, what was verified and how (source inspection, runtime check, `node_modules`); unverified assumptions flagged explicitly
  - Critique findings from the Phase 2 design review with dispositions
- **implementation-plan** (Phase 3): Phase 3 output document. Must contain:
  - Ordered steps, each with inputs, outputs, and acceptance criteria
  - Success criteria from Phase 1 (embedded or referenced)
  - Dependency ordering and parallelisation opportunities
  - Handoff footer — required text defined in Phase 3, step 7
- **implementation-summary** (Phase 4): Phase 4 structured output document. Must contain:
  - Working branch
  - Reference to the implementation-plan (filename or path)
  - Files modified (list of paths changed during implementation)
  - Per-step record: decisions taken, deviations from plan, surprises encountered
  - Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
  - Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path
  - Handoff footer — required text defined in Phase 4
- **changes** (Phase 4): Code modifications, tests, and commit history. Phase 5 entry condition: the committed Phase 4 changes must be accessible to the Phase 5 session.
- **documentation** (Phase 5, optional): Documentation a future maintainer needs that is not obvious from the code or existing project docs. Produced when Phase 5 steps 3–4 identify gaps; absent otherwise.

### Consumes

- **static-analysis-findings** (from phronithm:static-analysis): Used in Phases 4–5 to catch mechanical issues during and after implementation. When absent, relies on test results and manual review.
- **impact-map** (from phronithm:impact-analysis): Consumed in Phase 1 when the change involves renaming, type-changing, or removing exported or serialized fields — run before identifying candidate approaches. Also useful in Phase 5 review to verify all affected code has been considered. When absent, review scopes from the diff.

### Complexity drivers

- **Request clarity**: Detailed spec with acceptance criteria → low. Vague idea → high (significant Phase 1 work).
- **Domain complexity**: Well-understood domain with existing patterns → low. Novel domain or unfamiliar technology → high.
- **Integration scope**: Isolated new code → low. Modifying existing code with many consumers → high.
- **Unknowns**: All technical questions answerable from codebase → low. External dependencies, undocumented behaviour, or novel algorithms → high.

## Workflow

A pipeline of phases, each with a review gate. Any gate can send you back to an earlier phase. Prefer asking for clarification over making assumptions — the cost of building the wrong thing dwarfs the cost of a question.

**Session model.** Phases 1–3 (brainstorm, design, plan) run in a single session that accumulates context. Phase 4 starts in a **fresh context** after `/clear` — see the Phase 4 handoff block at the end of Phase 3 — reading the implementation-plan as its primary input. Phase 5 continues in the **same session** as Phase 4: there is no further `/clear`. The implementation-plan must therefore carry enough context — including its handoff footer — for Phase 4 to proceed without access to the planning session's history.

**Serialisation discipline**: save each phase output document to disk immediately on completion — do not defer until Phase 3. If you observe that your prior conversation history has been replaced by a summary — indicating context compaction occurred — reload the most recently saved phase document and continue from that phase's exit gate. If the summary shows you were mid-phase with no document yet saved, return to the start of that phase and re-run it. If compaction recurs within that same phase before the document is saved — the most recently saved document is unchanged from the previous recovery — the session cannot safely complete planning. Stop and report to the requester: the planning phases cannot complete in a standalone session due to context pressure.

### Tool usage during the workflow

Use subagents (the Task tool) only for **data-gathering**: codebase exploration, broad searches, reading many files. Subagents are appropriate when you need to survey unfamiliar territory and return structured findings.

Do **not** delegate design, planning, or reasoning to a subagent. These phases depend on the full accumulated context of the session — requirements, codebase findings, requester answers — which a subagent cannot inherit. Design done in a subagent requires manually serialising that context into a prompt (expensive, lossy) and returns opaque text that severs the reasoning thread. Do the thinking yourself with direct tool calls (Glob, Grep, Read) for any targeted lookups you need mid-reasoning.

**Subagent nesting**: Subagents can spawn their own subagents, but by policy data-gathering Task subagents must not spawn further agents (see [nesting](${CLAUDE_PLUGIN_ROOT}/docs/subagent-protocol.md#nesting)). The critique gate needs a subagent for context separation, so spawn it from the orchestrating session — never from a data-gathering Task subagent. Skills that compose other skills must account for this: run the critique gate and other Agent-dependent skills from the orchestrating session.

### Startup

**Pre-condition**: Do not enter plan mode until this skill workflow is the active context — i.e. the user has invoked `/phronithm:feature` or explicitly delegated to this skill. If you recognise a feature request outside this skill context, suggest the user run `/phronithm:feature` rather than entering plan mode directly.

1. **Pre-flight**: Follow the [pre-flight](${CLAUDE_PLUGIN_ROOT}/docs/pre-flight.md) check. Do not continue until it passes.

Before entering plan mode, do a minimal orientation to the project:

2. Read `CLAUDE.md` (project instructions — often contains architecture overview, conventions, key files).
3. Check `package.json` or equivalent to identify the tech stack and runtime dependencies.
4. Call `EnterPlanMode`.

This is cheap and ensures plan mode starts with basic grounding rather than cold.

### Phase 1: Brainstorm

Understand the problem before exploring solutions.

1. **Capture** the request. Restate it in your own words. Confirm understanding with the requester.
2. **Clarify** ambiguities. Ask about: scope boundaries, edge cases, performance requirements, compatibility constraints, affected users. Establish **measurable success criteria**: what specific, observable outcomes distinguish "done" from "not done"? Do not proceed with unresolved ambiguities — flag them explicitly.
   Tag each criterion with a **target rigour level** — how strongly the outcome must be established to count as met — from the ordered ladder **`proven` ▸ `evidenced` ▸ `conjectured`**:
   - *proven* — held by a reproducible automated check (test, CI benchmark) or a formal proof; it re-checks itself and stays true.
   - *evidenced* — actively executed at least once this cycle (manual run, ad-hoc script, one-off measurement), but not locked in by automation.
   - *conjectured* — inferred from code reading or reasoning; the behaviour was not executed this cycle.

   Default the target to `proven`; drop to `evidenced` only for criteria that cannot be automated, recording why. A project may override this default ladder in its `CLAUDE.md` — e.g. a mathematics project substituting a finer formal/computational ladder. Do not hardcode domain-specific levels here.
3. **Explore** the solution space. For targeted lookups, use direct tool calls. For broad exploration of an unfamiliar or large codebase, launch 2–3 parallel subagents with different foci (e.g., existing similar features, architecture in the affected area, integration points). Read key files yourself after agents return — do not rely on summaries alone.
   If the change involves **renaming, type-changing, or removing exported or serialized fields**, invoke `/phronithm:impact-analysis` before identifying candidate approaches — the consumer map is input to the design, not an afterthought.
   Identify at least two candidate approaches. If genuinely only one is viable, document an explicit justification explaining why alternatives are infeasible or unreasonable — not merely that one approach is dominant. For each candidate: what are the tradeoffs? What are the risks? What existing code or patterns does it build on?
4. **Document** the problem statement, requirements, constraints, candidate approaches, and exploration findings (key files, integration points, codebase patterns relevant to the change). The output is the problem-statement handoff document — it must be self-contained enough for Phase 2 to proceed from it alone.

Exit criteria: The problem is correctly understood and the candidate approaches are reasonable. **Confirm with the requester** — do not proceed to Design with unconfirmed requirements. If the requester is unavailable, document assumptions provisionally and flag them explicitly.

**Save the problem statement immediately** to `docs/plans/<name>-problem-statement.md` (or project-appropriate equivalent). Do not defer to Phase 3.

### Phase 2: Design

Settle on an approach and make it concrete. **Do not use the Task tool in this phase.** The design depends on the full accumulated context from Phase 1 — requirements, codebase findings, requester answers — which subagents cannot inherit. Use direct tool calls (Glob, Grep, Read) for any targeted lookups needed mid-reasoning.

1. **Evaluate** candidates from Phase 1 against deliberate competing stances: minimal change (smallest diff, maximum reuse), clean architecture, and pragmatic balance. Collapse when stances converge; the stances are a check, not a ritual. Consider feasibility, complexity (use the [complexity-assessment](${CLAUDE_PLUGIN_ROOT}/docs/lenses/complexity-assessment.md) lens where applicable), alignment with existing architecture, and incremental deliverability.
2. **Select** an approach. State the rationale. State what was rejected and why — this is as important as what was chosen.
3. **Specify** the design: interfaces, data structures, key algorithms, error handling strategy, integration points. The level of detail should match the risk — high-risk areas need more precision. Risk indicators: novel or unfamiliar technology, high fan-in (many callers affected), concurrency, security boundaries, irreversible operations (data migration, external API contracts), and areas with a history of bugs.
4. **Identify** unknowns and risks that remain. For each, state what would resolve it and when it needs to be resolved (before implementation, during, or after). Pay particular attention to undocumented or internal third-party APIs (shader chunk names, private methods, hook signatures, undocumented constructor parameters). These are high-risk unknowns — verify assumptions against the **installed library version** directly (source code, runtime check, or `node_modules` inspection), not documentation alone. Documentation may lag the installed version, and silent failures (e.g., `String.replace` with no match) make these bugs especially hard to detect in review. The same verification obligation applies to any API where similar names or contexts exist — LLM training data conflates related APIs with different signatures (e.g. `window.onerror` vs `DedicatedWorkerGlobalScope.onerror`). Do not rely on model knowledge for API shapes; verify against source or types.
5. **Review** the design: run the [critique gate](${CLAUDE_PLUGIN_ROOT}/docs/critique-gate.md) with [critique-design](${CLAUDE_PLUGIN_ROOT}/docs/critique/critique-design.md), applying its default iteration policy. Do not call ExitPlanMode until it passes or the iteration policy escalates to you.

   **Compressed mode (Phases 1–3 in plan mode)**: when all three planning phases run inside a single plan mode session, skip this critique step — Phase 3 step 5 runs critique on the combined plan document instead. Do not run both.

Exit criteria: The design is concrete enough that it can be broken down into individual implementable steps. **Confirm the approach with the requester** before proceeding to Plan. If the requester is unavailable, document the selected approach as provisional and flag it explicitly.

**Save the design document immediately** to `docs/plans/<name>-design.md` (or project-appropriate equivalent). Do not defer to Phase 3.

### Phase 3: Plan

Break the design into implementable steps. **Do not use the Task tool in this phase.** Same constraint as Phase 2 — see above for reasoning.

1. **Decompose** the design into discrete steps. Each step should be independently testable and committable.
2. **Order** by dependency. Identify what can be parallelised. When the plan contains investigation phases with competing hypotheses, order them by cost-to-test (cheapest first), not by likelihood — a cheap test that eliminates a hypothesis narrows the search space regardless of which hypothesis is correct (the general form is the [investigation loop](${CLAUDE_PLUGIN_ROOT}/docs/investigation-loop.md)).
3. **Define** what "done" means for each step — expected behaviour, test cases, acceptance criteria.
4. **Assess** scope. If the plan exceeds a reasonable size, split into deliverable increments. Each increment should provide user-visible value or reduce risk.
5. **Critique the plan**: run the [critique gate](${CLAUDE_PLUGIN_ROOT}/docs/critique-gate.md) with [critique-design](${CLAUDE_PLUGIN_ROOT}/docs/critique/critique-design.md), applying its default iteration policy. Plan-level critique catches structural bugs — wrong ordering, missing error paths, inconsistent interfaces between steps — that are expensive to discover during implementation; each structural fix can introduce new inconsistencies a single pass won't catch. Do not skip or compress this step.

   **Compressed mode (Phases 1–3 in plan mode)**: when all three planning phases run inside a single plan mode session, this critique step replaces the Phase 2 design critique — run it on the combined plan document before calling ExitPlanMode. Do not run both.

6. **Check** API spec / code block consistency. When the plan contains both a prose API spec (exported functions, constants, or types) and an implementation code block for the same module, verify:
   - Every symbol declared in the prose spec is present in the code block with matching export status.
   - Every exported symbol in the code block is present in the prose spec (no undocumented exported additions).
   - Parameter names, types, and return types agree between the two.
   Correct any inconsistencies in the plan before continuing. This check applies only when both a spec section and a code block are present for the same file.
7. **Append a handoff footer** to the plan document:
   ```
   ## Remaining phases
   This plan was generated by the phronithm:feature skill. After implementing the steps above,
   complete Phase 5 (Review) as defined in the phronithm:feature skill.

   ## Implementation-summary fields
   The implementation-summary must be saved to: docs/plans/<name>-implementation.md
   It must include:
   - Working branch
   - Reference to this plan file (filename or path)
   - Files modified (list of paths changed during implementation)
   - Per-step record: decisions taken, deviations from plan, surprises encountered
   - Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
   - Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
   - Handoff footer for Phase 5
   ```
   This ensures the executing session — which may not have access to earlier session context — knows that Phase 5 is still required and has a complete checklist for the implementation-summary.

Exit criteria: Every step has clear inputs, outputs, and acceptance criteria. The plan covers the full design. **Plan critique has run** and no Critical or Significant findings remain — do not call ExitPlanMode without completing the critique step. API spec / code block consistency check passed (or not applicable — plan has only one of spec or code block).

**Handoff footer required.** The plan document must include the handoff footer before this phase is considered complete. If it is missing, the phase exit gate fails.

**Save the approved plan** to a durable, human-readable location (project memory, `docs/plans/<name>-plan.md`, or project-appropriate equivalent). The plan file must be self-contained — a new session should be able to resume from the file alone without consulting the original transcript.

**Before calling ExitPlanMode**, verify all three:
1. Critique has run (Phase 3 step 5, or Phase 2 step 5 in non-compressed mode) — no Critical or Significant findings remain
2. API spec / code block consistency check passed (step 6) or is not applicable
3. Handoff footer is present in the plan document

Do not call ExitPlanMode until all three are satisfied. This checklist exists because the most common workflow failure is calling ExitPlanMode immediately after writing the plan, skipping the critique step.

**Phase 4 handoff.** The transition to Phase 4 happens through the standard `ExitPlanMode` flow — approving the plan and clearing context to implement it, not a bespoke step this skill constructs. The cleared session has no conversation history, so the saved plan document (with its handoff footer, step 7 above) is what Phase 4 reads to recover the design and requirements.

### Phase 4: Implement

Execute the plan. Test after each step.

**Assess implementation complexity before beginning.** Complex implementations — parsers, multi-file transformations, unfamiliar APIs, or tasks requiring extended coherent reasoning across many files — should target Opus. If the current session is running a lighter model, flag this to the user before starting Phase 4 rather than discovering capability limits mid-implementation.

1. **Implement** one step at a time. When parallelising by category (e.g. lint rule, component, concern), assign each file to exactly one agent — the one whose category has the most planned edits to that file (tie: first-listed category in the plan wins). For files that also need secondary-category fixes, include those in that agent's initial Task prompt rather than dispatching a second agent to the same file. Last-write-wins silently drops fixes from losing agents.
   When delegating steps to agents, include in the task prompt: (a) an explicit completion requirement — "Continue through to completion, including commit and any cleanup — do not stop mid-task to wait for further instruction"; and (b) before reporting done, the agent must run the project's formatter and type-checker on all changed files and fix any new errors.   **For features integrating unfamiliar external APIs**: validate auth with one minimal call before building full provisioning logic. Add one resource type at a time. Expect 3–5 discovery round-trips for niche or poorly-documented APIs — this is normal, not a sign of a flawed approach. Auth format, required fields, field validity constraints, and resource interdependencies are typically not in training data and must be discovered empirically.
2. **Test** the step against its acceptance criteria. Run the full affected test suite — regressions are easier to fix immediately. Consider running phronithm:static-analysis periodically to catch mechanical issues (null safety, type errors, resource leaks) early. For non-code changes: mentally execute at least one concrete example. If the change is conditional (prose with branches, config with overrides), cover all decision paths or document which are untestable and why. Record each criterion's **achieved rigour level** against its Phase 1 target, not just pass/fail: when the achieved level is below target (e.g. demonstrated once but not yet held by an automated check), note the upgrade path rather than treating the criterion as fully done.
3. **Commit** with a message that summarises the step, decisions taken, and anything surprising encountered. Before staging, run the project's formatter on changed files — pre-commit hooks catch format violations, but catching them before staging avoids a fix-restage-recommit cycle.
4. **Critique** the step's changes: run the [critique gate](${CLAUDE_PLUGIN_ROOT}/docs/critique-gate.md) with [critique-code](${CLAUDE_PLUGIN_ROOT}/docs/critique/critique-code.md). Spawn it immediately after the commit; while it runs, proceed to execute the next step concurrently. Critique is slow; you may complete several steps before an earlier critique finishes.
   This gate sees only this step's diff — it cannot detect a later step invalidating an assumption an earlier step made. That cross-increment check is Phase 5's job (step 1), not a duplicate of this one.
5. **Reassess** after each step. Has the implementation revealed something the design missed? If the gap is small, note it and continue. If it changes the approach, return to the appropriate earlier phase. When the target is a *finding* rather than a known deliverable (a spike, a root-cause question, an unfamiliar-API probe), this reassess-and-re-plan step *is* the [investigation loop](${CLAUDE_PLUGIN_ROOT}/docs/investigation-loop.md) — let results reshape the question rather than treating discovery as a deviation; reach for `/phronithm:investigate` when discovery is the product.

Before the exit gate, wait for any outstanding critique subagents to complete. Integrate findings: address Critical and Significant findings, re-running the gate per its default iteration policy until it passes or escalates; note Minor findings for Phase 5 clean-up.

Produce the **implementation-summary** document (see Produces for required contents). This is the Phase 4 handoff artefact, kept for resilience — if context compaction forces a reload, or Phase 5 is picked up in a different session (see **Partial-plan sessions** below), it is read as the primary input. Include the handoff footer:
```
## Remaining phases
Complete Phase 5 (Review) as defined in the phronithm:feature skill.
```
Save to `docs/plans/<feature>-implementation.md` (create the directory if absent). Do not rely on commit messages as the primary save location; a session recovering from compaction has no mechanism to discover the relevant commit hash.

**Exit gate**: Phase 4 is complete only when the implementation is committed, the implementation-summary includes the handoff footer, and all steps pass: tests green, per-step critiques addressed or noted, requirements from Phase 1 met.

**Phase 5 continues in this same session** — there is no `/clear` between Phase 4 and Phase 5. **Context separation is still required**: review and critique must run in subagents regardless. An LLM reviewing its own output in the same context that produced it is unreliable, and with no `/clear` boundary here, subagent dispatch is the only mechanism enforcing that separation. Do not run `/phronithm:review`, `/phronithm:critique`, or any evaluative step directly in this session.

**Partial-plan sessions**: When entering at Phase 4 from a pre-written plan, the plan's handoff footer covers only the phase being executed — it does not supersede the remaining phases. The full phase sequence (through Phase 5) applies regardless of entry point. List Phase 5 as an explicit next step in the implementation summary.

### Phase 5: Review

Validate the complete feature.

1. **Review** the implementation by running the phronithm:review skill — scope is the full feature diff, all applicable lenses, static-analysis pre-scan required. Collect per-step critique findings from Phase 4 into the review scope. Additionally, run the [critique gate](${CLAUDE_PLUGIN_ROOT}/docs/critique-gate.md) with [critique-code](${CLAUDE_PLUGIN_ROOT}/docs/critique/critique-code.md) on the full diff to catch issues that emerge only at feature scope.
   This step is not a re-review of already-reviewed code. Each Phase 4 gate only sees its own step's diff, so none of them can catch a step invalidating an earlier step's assumption — this full-diff pass is the only point in the workflow positioned to catch that class of defect. Do not treat it as a formality because the steps were "already reviewed".
2. **Test** end-to-end. Exercise edge cases identified in Phase 1.
3. **Document** anything a future maintainer needs to know that isn't obvious from the code.
4. **Update project docs**: check whether the changes affect concepts documented in CLAUDE.md or project design docs. For structural changes (files added, moved, or renamed), update key-file listings. For behavioural changes (new data shapes, changed contracts, modified mechanics), check whether existing docs reference the modified concepts and update if stale.
5. **Clean up**: remove dead code, temporary scaffolding, debugging artefacts.

Exit criteria: Code is reviewed, tested, documented, and ready to merge.

## Review gates

After each phase, explicitly check:

- Does the output meet the phase's exit criteria?
- Have assumptions made in earlier phases been invalidated? Are requirements still valid?
- Are there new ambiguities? Resolve them before proceeding.
- **Handoff footer** (Phases 3–4): does the output document include the handoff footer for the next phase?

If a gate reveals a problem, return to the earliest phase affected. Do not patch forward — a design flaw cannot be fixed in the plan, and a requirements misunderstanding cannot be fixed in the code. If the same phase is revisited more than twice, escalate to the requester — repeated returns signal a requirements or design problem that iteration will not resolve.

## Clarification principle

Ambiguity is a defect in understanding. Resolve it at the source:

- **Requirements** ambiguity: ask the requester.
- **Technical** ambiguity: investigate the codebase, read documentation, prototype.
- **Tradeoff** ambiguity: present options with tradeoffs to the requester and let them decide.

Never silently assume. If you make a provisional assumption to unblock progress, mark it prominently and revisit before the next gate.

When the requester is unavailable, document provisional assumptions prominently and gate on your own best judgement. Mark every such decision for review — the requester must validate them before the feature is considered complete.

## Commit discipline

See [commit discipline](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#commit-discipline). Additional feature-specific guidance:

- Commit after each step in Phase 4 and after completing each phase's documentation.
- The commit history is the primary record. Design documents and plans may live as documents or in issue trackers — match the project's conventions.

## Termination

The workflow terminates when Phase 5's exit criteria are met. It can also terminate early:

- The requester withdraws or deprioritises the feature. Commit and document whatever exists.
- Investigation reveals the feature is infeasible or inadvisable. Document the findings — negative results have value.
- Scope grows beyond what was agreed. Stop, reassess with the requester, and either split the work or adjust expectations.

## Scaling

For trivial features (single function, obvious implementation, no design ambiguity), phases 1-3 can be compressed into a brief confirmation with the requester before implementing. Do not skip them entirely — even trivial features benefit from a moment's thought about edge cases and testing.

For trivial features (single-file prose changes, well-specified single-commit work), the implementation-summary may be omitted — record decisions and review findings in the commit message instead.

For large features, consider splitting into multiple passes through this workflow, each delivering an increment.
