---
description: "Find and fix the root cause of a bug, not the symptom. Systematic fault isolation: triage, reproduce, investigate, fix, verify. Use when chasing a defect rather than building a feature."
allowed-tools: Read, Grep, Glob, Bash, Edit, Write, Task
---

# Diagnose: Systematic Fault Isolation

Find and fix the root cause, not the symptom. Leave the code more observable than you found it.

## Interface

### Requires

- **report**: Bug report, error message, stack trace, assertion failure, or user complaint. Must include: what was expected, what happened instead.
- **format** (optional): `json` for structured output (Phases 1–4 only, no code changes) or `human` for full workflow with fixes. Default: `human`.

### Produces

- **diagnosis**: Root cause analysis: `root_causes` (file, lines, category, description, confidence, evidence), `related_locations`, `suggested_fix`, `confidence_factors`. See Structured Output for full schema.
- **code-risk-assessment** (optional): Risk level, bug history, change patterns. Signals whether to escalate to refactor.
- **fix** (human format only): code changes, regression test, assertions added, commit(s).

### Consumes

- **impact-map** (from phronithm:impact-analysis): Trace impact of a suspect change. When absent, rely on manual code reading and git history.
- **static-analysis-findings** (from phronithm:static-analysis): Find similar issues in Phase 6. When absent, prevention check is manual or skipped.

### Signals

- **escalate → phronithm:refactor**: When code-risk-assessment reveals bug clustering, fix-refix cycles, or defensive coding accumulation.
- **request → phronithm:impact-analysis**: When investigation needs to trace impact of a suspect change.
- **hand off → phronithm:investigate**: When triage shows there is no defect to fix — the question is what is *true* about the system, not what is broken. `phronithm:investigate`'s success condition is a settled claim, not a verified fix.

### Complexity drivers

- **Report clarity**: Clear stacktrace with reproduction → low. Vague complaint → high.
- **Code complexity**: Isolated function → low. Multiple subsystems, shared state → high.
- **Reproducibility**: Deterministic → low. Intermittent or environment-specific → high.
- **Scope**: Single file → low. Cross-module with transitive dependencies → high.

## Workflow

### Startup

Follow the [pre-flight](${CLAUDE_PLUGIN_ROOT}/docs/pre-flight.md) check. Do not continue until it passes.

### Phase 1: Triage

Understand what is claimed to be wrong before touching code.

1. **Read the report**: extract what was expected, what happened instead, when it started, what changed.
2. **Clarify ambiguities**: ask questions early. Establish: reproducible? Under what conditions? Minimal case? Severity and urgency?
3. **Assess scope**: how much code is involved? How entangled with other subsystems? This informs whether to escalate.

Exit criteria: The problem is understood well enough to search for. Severity and scope are known.

### Phase 2: Investigate

The observe → hypothesise → cheapest-test → update cycle here follows the [investigation loop](${CLAUDE_PLUGIN_ROOT}/docs/investigation-loop.md); the steps below specialise it for fault isolation.

1. **Read the code** around the reported error. Locate where the fault **originates**, not where the symptom **appears** — the fix site is rarely the crash site. This applies across:
   - **Control flow**: trace callers and callees. Read error handling paths, not just the happy path.
   - **Data flow**: in multi-stage pipelines or transformation chains, find the stage where correct input first produces incorrect output. Changes upstream of the actual defect treat symptoms.
   - **State**: when the symptom is unexpected state (wrong value, wrong enum, missing flag), trace who **writes** the state, not just who reads it.
   - **Concurrency**: the defect may originate in a different thread, task, or process — trace shared state and ordering dependencies, not just the failing execution.
2. **Check history**: [recent-changes](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#recent-changes) touching the affected area. Issue trackers and commit messages for similar past bugs. Recurring bugs in the same code signal structural problems — note them.
3. **Search for siblings**: if the bug is in one instance of a pattern, check other instances.
4. **Gather diagnostics**: logs, traces, profiling data, test output — whatever is available. Record what you observe; do not rely on memory.
5. **Nested-tool / environment conflict**: Does the tool under investigation use the same infrastructure as this environment (sandbox, bwrap, network stack, runtime)? If so, test the nesting hypothesis first — before filesystem-level hypotheses. Script comments, dependency declarations, or error messages mentioning `bwrap`, `seccomp`, `cgroup`, or similar isolation mechanisms are direct signals.
6. **Consider alternative hypotheses**: enumerate and record the plausible alternatives, each with what would rule it out or how to test it (the loop's "generate multiple hypotheses"). This recorded list is what a Phase 4 backtrack returns to — keep it even after settling on a lead.

Exit criteria: You have a working hypothesis about the root cause, grounded in evidence from the code and diagnostics.

### Phase 3: Reproduce

Do not fix what you cannot reproduce.

1. **Create a reproduction**: the smallest, most reliable you can manage. A failing test is ideal. An intermittent reproduction is better than none — note the conditions.
2. **Verify the reproduction**: confirm it fails for the reported reason, not a different one.
3. **Isolate**: narrow from full system to unit test if possible. Each layer removed is information gained.

Exit criteria: A reproduction exists that reliably (or characterisably) demonstrates the fault. If reproduction is impossible, document what was tried and see Backtrack and Termination.

### Phase 4: Diagnose

Confirm root cause before writing a fix.

1. **Test hypotheses**: use the reproduction to verify or falsify your hypothesis. Add logging, assertions, or validation. Bisect if you suspect a regression.
2. **Distinguish root cause from proximate cause**: the crash site is rarely the crime scene. Trace backwards to the earliest point where behaviour diverges from intent. **Self-test**: does your proposed fix address *why* the bad state exists, or accommodate it? Localise the root cause where the contract is first violated — the code that produced the wrong state, not the code that failed to handle it. Do not let fix complexity influence diagnosis; that is a Phase 5 concern.
3. **Verify understanding** — the loop's predict-then-verify ([investigation loop](${CLAUDE_PLUGIN_ROOT}/docs/investigation-loop.md), step 6): predict what a specific small change will do, then make it. If the prediction is wrong, your model is wrong. Return to Phase 2 step 6 (consider alternative hypotheses); if exhausted, return to Phase 2 step 1 with new evidence.

Exit criteria: You can explain the root cause, why the symptom manifests, and why the existing code fails to prevent it. If you cannot explain all three, you do not yet understand the bug.

### Phase 5: Fix

Fix the root cause. Leave evidence that it was fixed.

1. **Implement the fix**: target the root cause. Propose the simplest viable fix first — explain why it is sufficient. Only escalate to a more complex approach if the user identifies inadequacy or the simple fix demonstrably fails. When multiple approaches exist, present them in ascending complexity order. Do not fix symptoms unless deliberately choosing a tactical patch (and documenting why).
2. **Add a regression test**: the reproduction from Phase 3, made permanent. Must fail without the fix and pass with it.
3. **Add assertions and verification**: strengthen the code around the fix site. These are permanent, not debugging scaffolding.
4. **Review collateral**: has the fix changed observable behaviour beyond the bug? Unintended side effects of fixes are a rich source of new bugs.

Exit criteria: The reproduction passes. The full affected test suite passes. Assertions are in place. The fix is minimal and targeted.

### Phase 6: Document and Commit

1. **Annotate** code if the bug revealed a non-obvious invariant. A comment explaining *why* an assertion exists is worth more than the assertion itself.
2. **Update documentation** if the bug exposed a gap.
3. **Flag structural issues**: file them. Do not scope-creep the fix, but do not let findings evaporate.
4. **Prevention check**: run phronithm:static-analysis on the affected area to find similar issues.
5. **Commit** — see [Commit discipline](#commit-discipline).

**Exit gate**: Phase 6 is not complete until the fix is committed.

## Backtrack and Termination

**Backtrack triggers:**

- **Phase 3 → Phase 2**: Reproduction reveals the symptom doesn't match the hypothesis.
- **Phase 4 → Phase 2 step 6**: Hypothesis falsified. Pick next alternative; if exhausted, return to Phase 2 step 1 with new evidence.
- **Phase 5 → Phase 4**: Fix doesn't pass or introduces new failures. Re-examine the diagnosis, not the fix.

**Proceed despite uncertainty** only when: severity justifies the risk, hypothesis is consistent with all evidence, and the fix is low-risk and reversible. Document the uncertainty in the commit message.

**Tactical patch**: accept a symptom-level fix when the root cause is outside your control, disproportionately risky, or time pressure demands it. Document as tactical, describe the root cause, file a tracking item. The regression test must still pass.

**Stop conditions**: root cause fixed; bug outside your control with tactical patch or upstream report; evidence exhausted without convergence (escalate with documentation); three backtrack cycles without new evidence (the [investigation loop](${CLAUDE_PLUGIN_ROOT}/docs/investigation-loop.md)'s bounded-retry, instantiated for debugging).

## Commit discipline

See [agent-protocols § Commit discipline](${CLAUDE_PLUGIN_ROOT}/docs/agent-protocols.md). Commit the fix and regression test together.

## Principles

The general investigation mechanics — observe-first, multiple live hypotheses, cheapest-falsification-first, predict-then-verify — are the [investigation loop](${CLAUDE_PLUGIN_ROOT}/docs/investigation-loop.md); see it for the full loop. The principles below specialise it for fault isolation.

**Reproduce before diagnosing.** Without a reproduction, you are guessing.

**Leave the code more observable.** Every debugging session reveals where observability was lacking. Fix that too.

## Code Risk Assessment

Optional analysis when the bug suggests a pattern, reveals systemic problems, or you're preparing to recommend refactoring. Skip for urgent/time-boxed debugging, isolated one-off bugs, new code, or third-party code.

**Gather:**

1. Bug history for the affected area:
   - [fix-history](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#fix-history) and [full-history](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#full-history) for the file
   - Search issue tracker for file/module name; check PR comments for disputed or revised fixes
2. Related bugs with similar symptoms:
   - Search issue tracker for error messages, exception types
   - [failed-fixes](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#failed-fixes) — reverted fixes indicate complexity
3. Change frequency and author churn:
   - [churn](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#churn) over a six-month window — high churn suggests instability

**Indicators**: bug clustering, fix-refix cycles, cross-cutting failures, defensive coding accumulation, test gaps, author turnover.

**Record** in `code_risk_assessment` (JSON format) or commit messages/tracking issues (human format). JSON is authoritative for automated analysis; commit messages and comments are human-facing summaries.

**Escalate to refactor** when: bug clustering (≥3 bugs in same module within 6 months), fix-refix cycles, or defensive coding accumulation. Protocol:
1. Complete the current bug fix first
2. Document findings in `code_risk_assessment` or tracking issue
3. Recommend refactor in commit message or structured output
4. User/workflow decides whether to invoke phronithm:refactor skill

Diagnose does not perform refactoring — flag the need.

## Patterns

Common root cause categories:

- **State corruption**: data modified unexpectedly, missing or wrong initialisation, aliasing.
- **Incomplete state transitions**: state machine advances partway but skips a step — missing notification, flag not set, callback not fired. Common in cancellation/shutdown paths where the normal transition is performed by a worker that never runs. The root cause is the code that should have completed the transition, not downstream code that assumes it was completed.
- **Race conditions**: TOCTOU, unprotected shared state, ordering assumptions.
- **Boundary violations**: off-by-one, overflow, truncation, encoding mismatches.
- **Error handling gaps**: unchecked return values, swallowed exceptions, missing rollback on partial failure.
- **Assumption drift**: code assumes an invariant that was true when written but was later violated by changes elsewhere.
- **Environment sensitivity**: behaviour depends on configuration, locale, timezone, filesystem semantics, network conditions, or OS version.

## Structured Output

When invoked with `--format=json`, run Phases 1–4 only (no code changes) and print the JSON to stdout.

```json
{
  "root_causes": [
    {
      "file": "src/handlers.py",
      "lines": [42, 45],
      "category": "null-reference",
      "description": "Missing null check before attribute access",
      "confidence": "certain",
      "evidence": ["AttributeError on line 42", "config can be None when get_config() fails"]
    }
  ],
  "related_locations": [
    {"file": "src/config.py", "line": 15, "note": "get_config() returns None on error path"}
  ],
  "suggested_fix": "Add guard clause: if config is not None:",
  "confidence_factors": {
    "stacktrace_available": true,
    "reproduction_successful": true,
    "static_analysis_confirms": false,
    "similar_bugs_found": true,
    "fix_verified": false
  },
  "code_risk_assessment": {
    "risk_level": "high",
    "bug_history": {"prior_bugs_in_file": 3, "fix_refix_cycles": 1, "related_issues": ["#1234", "#2456"]},
    "change_patterns": {"commits_last_6_months": 23, "unique_authors": 5},
    "risk_indicators": ["bug_clustering", "fix_refix_cycle", "high_churn"],
    "recommendations": ["Refactor module — recurring null handling issues"]
  }
}
```

**Fields**: `root_causes` (required) — array, use multiple elements for multi-cause bugs. `related_locations`, `suggested_fix`, `confidence_factors`, `code_risk_assessment` — all optional. When no root cause is found: `{"root_causes": [], "note": "reason"}`.

**Categories** (coarse): `memory`, `concurrency`, `null-reference`, `type-safety`, `error-handling`, `state`, `resource-lifecycle`, `logic`, `security`, `undefined-behaviour`. Use `subcategory` (optional, free-text) for finer-grained classification.

**Confidence**: `certain`, `likely`, `possible`, `uncertain`. Mapping: all factors true → certain; stacktrace + reproduction → likely; hypothesis only → possible; conflicting evidence → uncertain.

**Risk levels**: `high`, `medium`, `low`, `unknown`. **Risk indicators**: `bug_clustering`, `fix_refix_cycle`, `cross_cutting`, `defensive_accumulation`, `test_gaps`, `author_turnover`, `high_churn`, `recent_refactor`.
