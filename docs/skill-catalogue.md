# Skill Catalogue

Inventory of implemented and planned skills.

## Implemented

- **[phronithm:breakdown](../skills/breakdown/SKILL.md)**: Work decomposition and execution planning. Decomposes a feature into tracked work units with dependency ordering (logical, testing, design, knowledge), parallelism analysis, and execution guidance. Invoked from `/phronithm:large-scale-feature` Phase 2, or standalone.
- **[phronithm:concurrency](../skills/concurrency/SKILL.md)**: Systematic concurrency analysis. Standalone skill (not a review lens) because it requires building and verifying a concurrency model, not checklist review.
- **[phronithm:critique](../skills/critique/SKILL.md)**: Direct evaluation of any artefact (code, design, skill document) against the critique framework axes. Lighter-weight than phronithm:review — no lens workflow, no phases. Produces structured findings with severity.
- **[phronithm:diagnose](../skills/diagnose/SKILL.md)**: Systematic fault isolation: triage, investigate, reproduce, diagnose, fix.
- **[phronithm:feature](../skills/feature/SKILL.md)**: End-to-end feature delivery: brainstorm, design, plan, implement, review.
- **[phronithm:impact-analysis](../skills/impact-analysis/SKILL.md)**: Prerequisite skill: given a change, determine what code is affected directly, transitively, and implicitly. Uses [LSP](lsp/integration.md) for compiler-grade tracing when available; falls back to grep. Produces an impact map consumed by other skills (phronithm:review, phronithm:refactor, phronithm:concurrency, phronithm:diagnose).
- **[phronithm:investigate](../skills/investigate/SKILL.md)**: Domain-general investigation — settle an open claim to a result-with-rigour. The investigative sibling of `phronithm:feature`: shares its epistemic backbone but reshapes the core around a moving target (the [investigation loop](investigation-loop.md)) and a claim-plus-evidence deliverable. Covers research, code/perf spikes, open-question root-cause, and unfamiliar-API discovery, with domain specifics (e.g. a maths rigour ladder) deferred to project `CLAUDE.md`.
- **[phronithm:large-scale-feature](../skills/large-scale-feature/SKILL.md)**: Multi-session architecture-first feature delivery: parallel research, architecture document (critique-iterated), then `/phronithm:feature` for each increment.
- **[phronithm:persona-debug](../skills/persona-debug/SKILL.md)**: Systematic fault isolation for LLM agent personas and skill instructions. Step-through execution tracing, defect classification, minimal fix, supervised verification. The phronithm:diagnose skill applied to natural-language instruction specs rather than code.
- **[phronithm:pin-behaviour](../skills/pin-behaviour/SKILL.md)**: Generate characterisation tests (golden-master / approval testing, Feathers sense) that pin code's *current* observable behaviour — bugs and all — so a later change can be proven behaviour-preserving. Produces a suite green against current code plus a manifest of what was pinned and what could not be. The behaviour-preservation oracle `phronithm:refactor` relies on when coverage is sparse.
- **[phronithm:refactor](../skills/refactor/SKILL.md)**: Iterative deduplication and structural decomposition. Reference implementation for skill structure.
- **[phronithm:review](../skills/review/SKILL.md)**: Focused code review through composable lenses. Lenses are reference documents in `docs/lenses/`, not skills — the phronithm:review skill defines the process, lenses define what to look for.

Planned:

- **bisect**: Regression localisation. Wraps `git bisect run` with test-script generation: given a known-good commit, a known-bad commit, and a failure description, constructs a test predicate and runs binary search. For code regressions, not instruction-spec defects (use phronithm:persona-debug for those).
- **mutation-test**: Test quality evaluation via deliberate code mutation. Identifies undertested code paths — the complement of coverage, which tells you what was executed but not whether tests would detect changes. Outputs a per-module sensitivity map.
- **write-script**: Script generation for admin/ops/systems automation and dev tooling. Covers language selection (Python/shell/jq), safety checklist (dry-run, idempotency, privilege awareness), and auditability. Serves both standalone ops scripts (network management, service automation, system config) and in-repo mechanical transforms (data migration, bulk edits, fixture generation). See [#32](https://github.com/duaneg/llm-assisted-programming/issues/32).

## Review lenses

Reference documents loaded by the phronithm:review skill. Each lens targets a specific class of defect or concern. See any existing lens for the format.

Implemented:

- **[error-handling](lenses/error-handling.md)**: Error handling analysis: coverage, handler correctness, propagation, consistency, taxonomy. General detects gaps; this lens analyses how to handle them.
- **[complexity-assessment](lenses/complexity-assessment.md)**: Metrics: LoC, cyclomatic complexity, cognitive complexity. Measures a caller-defined code region before and after a change.
- **[general](lenses/general.md)**: Correctness, clarity, naming, structure.
- **[data-structures](lenses/data-structures.md)**: Appropriate choice and use of data structures.

- **[style](lenses/style.md)**: Craft, convention, and idiomatic language use. What automated tools cannot enforce.
- **[comments](lenses/comments.md)**: Inline and doc comment content.
- **[io](lenses/io.md)**: IO and filesystem operations. Failure modes, resource management, blocking behaviour, atomicity. (Merges the previously separate IO and Filesystems entries — they share enough failure-mode logic to be one lens.)
- **[numerical](lenses/numerical.md)**: Floating-point and numeric code. Cancellation, summation, conditioning vs stability, comparison/tolerance, special values, representation, cross-platform reproducibility, and numerical testing. The deep-dive checklist behind the [critique-maths](critique/critique-maths.md) appendix.

Planned:

- **ownership**: Ownership, lifetime, and resource management clarity.

All lenses should reference [code-patterns](code-patterns.md) for structural duplication detection where relevant. When a planned lens *would* have been useful, note this.

### Design decisions

**Lenses are documents, not skills.** A lens answers "what should I look for?" — domain knowledge for a review process. The phronithm:review skill provides the workflow (scope, examine, synthesise). This follows the precedent set by [code-patterns](code-patterns.md), which is a reference document loaded by the phronithm:refactor skill.

**phronithm:concurrency is a standalone skill, not a lens.** Concurrency review requires building a model of shared state and synchronisation, then systematically verifying it. That's a multi-phase diagnostic workflow with backtracking — a different shape from "scan the code against this checklist".

**Functional core is not a lens.** As a review concern ("is core logic pure?") it's a few lines within the general or style lens. As a constructive activity (restructuring code for purity) it's a refactoring pattern. It doesn't warrant its own lens.

**IO and Filesystems are merged.** They share failure-mode logic (partial operations, blocking, metadata unreliability) and reviewing them separately would produce overlapping findings.

## Testing

- **[phronithm:static-analysis](../skills/static-analysis/SKILL.md)**: Static code analysis. Orchestrates language-specific tools (ruff/mypy for Python, spotbugs/javac for Java, clang-tidy/gcc for C). Finds bugs before runtime with fast feedback, no test coverage required. Supports exclusion workflow for progressive strictness adoption.
- **[phronithm:pin-behaviour](../skills/pin-behaviour/SKILL.md)**: The implemented characterisation-testing technique — generates golden-master/approval tests pinning current observable behaviour. Complementary to the planned `mutation-test` (which measures suite *adequacy*, not creation) and the planned property-based/fuzzing skills below (which generate inputs for *classes* of behaviour, where this pins *specific observed* behaviour). Listed in full under Implemented above.

Planned skills and guidance. These are practices and techniques, not review lenses.

- **Property-based testing**: Test for *classes* of bugs. Technique skill: given code, design property-based tests for it.
- **Fuzzing**: Powerful enough to use everywhere possible. Technique skill.
- **Dynamic program analysis**: Tool usage guidance. Valgrind, sanitisers, profilers.
- **Tracing**: System-level observation. strace, perf, language-specific tracers.

General testing practices (unit, integration, regression) are covered by the phronithm:feature skill's implementation and review phases.

## Not in scope

Areas deliberately excluded or deferred:

- **Architecture and system design**: Too broad for a single skill. May revisit as the skill collection matures.
- **Security review**: Deserves dedicated treatment but not yet designed.
- **Performance**: Beyond complexity-assessment metrics. Profiling, benchmarking, hot-path analysis. Deferred.
- **Deployment and operations**: Out of scope for a programming-focused collection. Revisit if/as scope widens.
