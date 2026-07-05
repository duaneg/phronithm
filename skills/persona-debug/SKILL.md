---
description: "Find and fix defects in LLM agent personas and skill instructions when they produce wrong behaviour. The `phronithm:diagnose` skill applied to natural-language instruction specs rather than code. Use when an agent or skill misbehaves and you need to locate the instruction defect."
allowed-tools: Read, Grep, Glob, Bash, Agent, Edit
---

# Persona Debug: Systematic Instruction-Spec Fault Isolation

Find and fix defects in LLM agent personas and skill instructions when they produce wrong behaviour. Applied to natural-language instruction specs rather than code — but follows the same discipline: observe, trace, hypothesise, fix, verify.

This is the `phronithm:diagnose` skill applied to a different subject. Where diagnose targets code, this targets the instruction spec that governs an agent's decisions. The methodology is the same; the execution environment is natural language rather than a runtime. Same boundary as diagnose: when the question is what is *true* about an agent's behaviour rather than which instruction to fix, use `phronithm:investigate` instead.

## Interface

### Requires

- **persona**: Path to the agent or skill definition being debugged (`agents/*.md`, `skills/*/SKILL.md`, or similar). Inline text is acceptable for short personas.
- **failure**: Description of the observed wrong behaviour. Must include: what was expected, what happened instead.

### Produces

- **execution-trace** (Phase 2): Step-by-step record of what the persona does at each decision point, with the divergence identified.
- **diagnosis**: Defect type, specific location in the instruction spec, evidence.
- **fix**: Minimal change to the instruction spec, with rationale.
- **verification-result** (Phase 4): Pass/fail result of running the fixed persona on the triggering case, with evidence.

### Entry points

Phases can be entered independently:

- **Full cycle (default)**: all four phases.
- **Diagnose only (Phases 1–3)**: failure is observed, root cause is unknown.
- **Verify only (Phase 4)**: fix has been applied, needs validation. Requires a known triggering case.

### Complexity drivers

- **Failure clarity**: specific, reproducible case → low. Vague report → high (significant Phase 1 work).
- **Persona size**: short agent definition → low. Long skill with many phases → high.
- **Defect type**: missing instruction or lost precondition → low. Unreachable escape hatch or suppressed escape → high.
- **External impact surface**: single well-defined action → low. Many possible interactions → high.

## Workflow

### Startup

Follow the [pre-flight](${CLAUDE_PLUGIN_ROOT}/docs/pre-flight.md) check. Do not continue until it passes.

Read the persona in full. If it references other documents (critique frameworks, lens documents, pre-flight checks, referenced skills), read those too — defects sometimes live in consumed documents rather than the persona itself.

### Phase 1: Identify

Establish what is wrong and a reliable way to trigger it.

1. **Read the failure report.** Extract: what was expected, what happened instead, which step of the persona produced the wrong output.
2. **Clarify ambiguities.** Ask early: Is the wrong behaviour deterministic or intermittent? Under what input conditions? Is there a specific case that reliably triggers it?
3. **Construct or confirm the triggering case.** The minimal scenario that reliably demonstrates the wrong behaviour. It must specify: the input (issue number, feature request, scenario description), any relevant context (repo state, prior conversation), and the expected vs observed action. If a case is provided, verify it is specific enough to reproduce the behaviour. If it is underspecified, narrow it before proceeding.
4. **Assess scope.** Which section(s) of the persona are likely responsible? Identify candidates before stepping through — this focuses the trace.

Exit criteria: A triggering case exists that reliably demonstrates the wrong behaviour. Candidate sections are identified.

### Phase 2: Step-through

Manually trace the persona's execution on the triggering case. This is interactive execution logging — used when the root cause is unknown.

**Protocol:** Adopt the persona for the triggering case. At each decision point:

1. State which instruction governs this step (quote or reference it).
2. State what that instruction implies for the current situation.
3. State what the persona would do.
4. **Before any action with external impact** (filing an issue, opening a PR, pushing, posting a comment, sending a message, writing to a file): pause and describe what would happen. Do not execute. Await user acknowledgement if interactive; describe and stop if non-interactive.

Continue until:
- The persona would produce the wrong action — note the exact instruction and why it produces this outcome.
- The persona reaches a decision point with no governing instruction — note the gap.
- The persona completes without producing the expected outcome — note what was missing.

After the trace, state: where did the execution diverge from expected? What instruction (or absence of one) caused the divergence?

**Delta debugging (escalation, when step-through leaves genuine ambiguity):** Identify the candidate sections. Disable them one at a time (working from most suspect), tracing execution after each removal. The minimal set of instructions whose removal makes the behaviour correct is the defect location. This is expensive — use only when the step-through trace is genuinely inconclusive.

Exit criteria: The divergence point is identified with a specific instruction (or gap) as the cause.

### Phase 3: Fix

Apply the minimal change that corrects the defect.

1. **Classify the defect type** (see Defect taxonomy). This determines the fix pattern.
2. **Propose a fix.** The smallest change that addresses the root cause. Prefer additions to removals — removing an instruction fixes the symptom but may create a new gap. Additions that restructure surrounding text are suspect: they may fix the immediate defect while introducing instruction conflicts elsewhere.
3. **Check for instruction conflicts.** Does the change contradict any existing instruction? Does it create a new gap in an adjacent case?
4. **Apply the fix.**
5. **Verify mentally:** trace the triggering case through the fixed persona. Does it produce the expected outcome? Are there adjacent cases the fix might break?

Exit criteria: The fix is applied and mentally verified on the triggering case.

### Phase 4: Verify (Supervised run)

Run the fixed persona on the triggering case under supervision. This is regression testing for the instruction spec.

**Protocol:** Spawn a subagent with the fixed persona. Include this instruction in the spawn prompt:

> Run this persona on [triggering case]. Before any action with external impact (filing issues, opening PRs, pushing, posting comments, writing to external services), stop and output: (1) what you would do and why, (2) whether this matches the expected behaviour. Do not execute the action. Stop there.

Review the subagent's output:

| Result | Meaning |
|--------|---------|
| **Pass** | Correct action, correct reasoning. |
| **Fail** | Wrong action — return to Phase 3. |
| **Correct action, wrong reason** | Fix is fragile — may fail on adjacent cases. Return to Phase 3. |
| **Correct action with unintended artifact** | Partial fix — the fix addressed the reported defect but a secondary issue remains. Record the new defect and return to Phase 1. |

**Differential check (when regression risk is a concern):** Run the unfixed persona on the same case and compare outputs. Confirms the fix changes only the targeted behaviour.

Exit criteria: The subagent produces the expected action with the correct reasoning. No unintended artifacts.

## Defect taxonomy

Common defect types for natural-language instruction specs:

- **Missing instruction**: No instruction covers the triggering case. The agent fills the gap with default (often wrong) behaviour. Fix: add the instruction. Be specific about the trigger condition — a vague addition may not fire when needed.

- **Ambiguous instruction**: Instruction present but does not uniquely determine the right action. Multiple valid interpretations exist, and the agent picks the wrong one. Fix: tighten with a specific condition, example, or negative constraint.

- **Unreachable escape hatch**: A stop condition or fallback exists but its trigger is never satisfied in the triggering case. Common causes: the condition requires the agent to have already failed (it gets evaluated too late); the condition is ordered after a step that prevents it from being reached; the condition is phrased as "if you cannot..." which the agent satisfies superficially by producing any output. Fix: move the check earlier, broaden the trigger, or reframe as a pre-condition rather than a fallback.

- **Lost precondition**: A key assumption that was once explicit was removed during iterative refinement. Critique loops that add safeguards for edge cases tend to bury or omit the original design invariant. Fix: restore the precondition explicitly — ideally as the opening statement of the persona rather than embedded in a step.

- **Suppressed escape**: The agent satisfies the formal exit condition of an escape hatch (produces a test file, writes output, calls a function) without achieving the intended outcome. The escape hatch checks for an action, not for the quality or correctness of that action. Fix: tighten the escape hatch to check for the intended outcome, not mere existence of the output.

- **Instruction conflict**: Two instructions produce contradictory guidance for the triggering case. The agent follows one (often the more specific or later-appearing) and ignores the other. Fix: reconcile — determine which is authoritative and remove or subordinate the other.

## Commit discipline

See [agent-protocols § Commit discipline](${CLAUDE_PLUGIN_ROOT}/docs/agent-protocols.md). Commit the fixed persona/skill file once Phase 4 verification passes. There is no automated regression test for an instruction-spec change — the verification-result is the regression evidence; record the triggering case and outcome in the commit message.

## Principles

This skill is the [investigation loop](${CLAUDE_PLUGIN_ROOT}/docs/investigation-loop.md) applied to instruction specs — observe, generate and seek alternative hypotheses, test cheapest-first, update belief. The principles below specialise it for persona/skill defects.

**Trace before hypothesising.** Reading the instructions and guessing the defect is insufficient — the execution trace reveals what the agent does step by step, which is often different from what the instruction author intended. Step through first.

**Fix the instruction, not the symptom.** Adding a special case for the triggering case without fixing the underlying instruction structure produces accumulating patches. Fix the structure — identify why the instruction fails, not just that it fails.

**Minimal change.** Large restructuring carries regression risk — new gaps may be introduced where adjacent instructions interact differently. The fix should be as narrow as possible.

**Lost preconditions are the most insidious defect.** Iterative refinement (especially critique loops) systematically buries the original design invariant under layers of safeguards. When a persona was designed for a specific use case and no longer enforces it, look for a removed precondition first.

**Verify on the triggering case, then consider adjacent cases.** Verification on the triggering case is necessary but not sufficient. After Phase 4, ask: are there adjacent cases where the fix might produce new wrong behaviour?
