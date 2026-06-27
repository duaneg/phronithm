# Lens: Complexity Assessment

Measure code complexity before and after a change to determine whether it improved or degraded.

Used by other skills (notably [phronithm:refactor](../../skills/refactor/SKILL.md)) as a stop condition.

## Metrics

For a given change, measure using tools:
- **Lines of code changed**: Non-blank, non-comment lines.
- **Fan-in/fan-out**: how a change affects the dependency graph.

Before and after change, measure using tools:
- **Cyclomatic complexity**: Independent paths through the code.
- **Parameter count**: Per function/method. High counts signal a missing abstraction.

Before and after change, estimate using LLM:
- **Duplication count**: Number of duplicate blocks.
- **Cognitive complexity**: Nesting depth weighted by control flow — how hard code is to *read*, not just how many paths exist. Score increments for each control flow structure (if/else, switch, loops, try/catch, ternary, short-circuit operators) plus a structural nesting penalty per additional level. Unlike cyclomatic complexity, nesting itself increases the score independently of path count. Full definition: [SonarSource Cognitive Complexity white paper](https://www.sonarsource.com/docs/CognitiveComplexity.pdf).

The six metrics above cover the key dimensions relevant to refactoring stop conditions: code volume (LOC), dependency structure (fan-in/fan-out), branching complexity (cyclomatic), interface complexity (parameter count), structural redundancy (duplication count), and readability (cognitive complexity). Coupling/cohesion metrics (CBO, LCOM and variants) are absent: computing them reliably requires language-specific static analysis tooling not universally available, and fan-in/fan-out already captures the coupling signal relevant to refactoring changes. The metric set is considered sufficient for the current stop-condition use case; add coupling/cohesion only if false negatives appear in postmortem data.

## Scoring

Each metric is computed for the code region before and after the change. Report:

1. Per-metric delta in absolute and relative terms.
2. Direction: improved, unchanged, or degraded.

No composite score. Consumers (e.g. [phronithm:refactor](../../skills/refactor/SKILL.md) stop conditions) define their own thresholds over individual deltas.

*Provisional thresholds* — the 15%/5% figures are starting values chosen to tolerate minor trade-offs while rejecting clear regressions. Deduplication and decomposition may warrant different thresholds once data is available (deduplication's primary signal is duplication count; decomposition's is cognitive complexity), but a single shared threshold is used until evidence justifies splitting.

**Review trigger**: once 5 or more postmortem records exist for refactor invocations where the stop condition fired or nearly fired (queryable via the `stop-condition` field), evaluate whether the thresholds correctly classified those cases.

## Scope

Caller-defined. This lens measures whatever code region it is pointed at.

Test utility and helper code should be measured alongside production code. Test cases themselves are excluded; see [code-patterns](../code-patterns.md) for the distinction.

## Measurement

Metrics must always be measured using tools. Language-specific tooling should be used where possible. If metrics cannot be measured using existing tools, omit them, noting their absence.

## Interesting Results

Cross-lens interaction — e.g. a change that increases cyclomatic complexity but dramatically improves style. Sharply opposite results are a strong signal of *interesting* code. Flag for review; decisions require discernment.

## Red flags

- Cyclomatic complexity increase without new functionality.
- Parameter count growing: function accreting responsibilities.
- Fan-out increase: coupling more things together.
- Duplication count increase: copying instead of reusing.
- Cognitive complexity increase even when cyclomatic complexity is flat: deeper nesting, more interleaved concerns.