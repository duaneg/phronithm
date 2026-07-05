---
description: "Given a code change, determine what is affected directly, transitively, and implicitly. Produces an impact map consumed by `phronithm:review`, `phronithm:refactor`, `phronithm:concurrency`, or `phronithm:diagnose`. Use before changing a widely-used API, shared component, or anything with non-obvious consumers."
allowed-tools: Read, Grep, Glob, Bash, Write, Skill
---

# Impact Analysis

Given a change, determine what code is affected — directly, transitively, and implicitly.

## Interface

### Requires

- **change**: One of: a diff/commit range (completed change), a description of intended modifications (proposed change), or a hypothesis about what changed (diagnose investigation). More concrete input (diff) produces more reliable output.
- **focus**: What the invoking skill needs. One of: full-impact, test-relevance, concurrency-scope. Default: full-impact.

### Produces

- **impact-map**: Structured list of affected code sites, each with:
  - location (file, line, function)
  - relationship (how it depends on the changed element)
  - distance (direct / transitive / implicit)
  - coupling (structural / behavioural / implicit)
  - risk (high / moderate / low)
  - test-coverage (covered / not covered / partially covered)
- **boundaries**: Where tracing stopped and why.
- **gaps**: What could not be traced and why.
- **confidence**: Limitations, skipped steps, overall confidence rating.

### Complexity drivers

- **Change surface size**: 1–3 elements → low. 4–10 → moderate. >10 → high.
- **Dependency depth**: Shallow call tree → low. Deep transitive chains or widely-used abstractions → high.
- **Implicit dependencies**: Presence of reflection, dynamic dispatch, convention-based coupling, or undocumented contracts → high.
- **Domain knowledge required**: Generic library code → low. Domain-specific invariants or cross-service contracts → high.

## Purpose

Impact analysis is a prerequisite skill. Other skills invoke it to scope their work:

- **phronithm:review** uses it to identify code that needs examination.
- **phronithm:concurrency** uses it to limit the concurrency model to what's affected by a change.
- **phronithm:refactor** uses it to determine what tests are relevant after a generalisation.
- **phronithm:diagnose** uses it during investigation to find code affected by a suspect change.

The skill produces an impact map — a structured inventory of affected code with risk classification — not fixes or recommendations. For low-risk changes, invoking skills may treat the map as a prioritisation guide — examine mapped sites first, use judgement about whether to look further. For high-risk changes (public interface modifications, data schema changes, security-sensitive code), invoking skills should not use the map to exclude code from examination.

## Inputs

The invoking skill must provide:

- **The change**: one of: a diff or commit range (completed change), a description of intended modifications (proposed change), or a hypothesis about what changed (diagnose investigation). The more concrete, the better — a diff is unambiguous; a vague description of intent forces Phase 1 to do more interpretive work and increases the risk of misclassifying the change surface.
- **Focus**: what the invoking skill needs from the map. Default is full impact. Narrower focus reduces effort and noise:
  - *Test relevance*: which tests exercise affected behaviour. (Refactor use case.)
  - *Concurrency scope*: which shared state and synchronisation are affected. (Concurrency use case.)
  - *Full impact*: all affected code sites with risk classification. (Review, diagnose use cases.)

The invoking skill should also provide, when available:

- **Known external consumers**: services, tools, or repositories that depend on the changed code. These cannot be traced mechanically but should be recorded as gaps.
- **Domain context**: implicit contracts, undocumented behavioural expectations, historical fragility in the affected area. This directly improves the quality of Phase 2 implicit dependency tracing and Phase 4 behavioural assumption probing. Without it, those steps produce gaps rather than findings.

## Scoping

This skill operates within a single repository. Cross-repository and cross-service impact cannot be traced mechanically — note known external consumers as gaps.

## Scaling

For trivial changes (rename a private function, fix a typo in a string literal): find references and verify each one, done. The full workflow is unnecessary. When LSP is available, `textDocument/references` gives precise results directly. Otherwise, when the change surface is a module's exports, `${CLAUDE_PLUGIN_ROOT}/tools/extract-ts-exports.py <file> | ${CLAUDE_PLUGIN_ROOT}/tools/map-symbol-consumers.py --src-dir src/ --exclude <file>` enumerates all consumers in one pass and seeds Phase 2 tracing.

For moderate changes (modify a public function's behaviour, change a type definition): run Phases 1–2 fully, lightweight Phase 3 classification, lightweight Phase 4 verification (co-change history check, quick scan of exclusions).

For significant changes (alter a public interface, change a data schema, modify a widely-used abstraction): full workflow. The cost of missing an affected site is high.

When in doubt, start with Phase 1. The size of the change surface tells you which path you're on.

## Workflow

### Phase 1: Scope the change

Define what changed and classify its nature.

1. **Identify the change surface**: the specific functions, types, interfaces, data structures, or behaviours that are modified, added, or removed. For a completed change, diff against the base. For a proposed change, define the intended modifications.

2. **Classify the change**: each element of the change surface falls into one or more categories:
   - **Signature change**: parameters, return types, or error types changed. All callers must adapt.
   - **Behavioural change**: same signature, different observable behaviour. Callers that depend on the prior behaviour are affected; callers that don't are not. Distinguishing which is which is the hard part.
   - **Data shape change**: structure of data at rest or in transit is modified. Affects all readers, writers, and validators.
   - **Contract change**: preconditions, postconditions, invariants, or guarantees are strengthened or weakened. Affects all code that relies on the prior contract.
   - **Removal**: something that existed no longer does. All references break.
   - **Addition**: new code, no existing dependents. Impact is zero unless the addition shadows, overrides, or alters resolution order of existing code.

   A single change often combines categories. A function that gains a parameter is both a signature change and possibly a behavioural change.

Exit criteria: The change surface is enumerated. Each element is classified. The classification determines which dependency types to trace in Phase 2.

**Classification-to-dependency-type mapping**: general guidance, not exhaustive:

| Change classification | Primary dependency types to trace |
|---|---|
| Signature change | Call dependencies (all callers must adapt), type dependencies (if parameter/return types changed) |
| Behavioural change | Behavioural dependencies (code relying on prior behaviour), call dependencies (to find candidates) |
| Data shape change | Data dependencies (all readers, writers, validators, migrations) |
| Contract change | Behavioural dependencies (code relying on prior guarantees) |
| Removal | Call dependencies, type dependencies (all references break) |
| Addition | Type dependencies (shadowing, resolution order changes) |

Most changes combine classifications — trace the union of the relevant dependency types.

### Phase 2: Trace

Follow dependencies outward from each element of the change surface. Use [dependency-types](${CLAUDE_PLUGIN_ROOT}/docs/dependency-types.md) to guide the search.

1. **Trace direct dependents**: code that references an element of the change surface. Every dependent found is included in the map by default. For each dependent:
   - How does it use the changed element? (calls it, implements it, embeds it, reads its output)
   - To *exclude* a dependent, state positive evidence for why it is unaffected (e.g., "caller ignores the changed return value", "interface contract unchanged and verified by test X"). Without such evidence, the dependent stays in the map.
   - Record the dependent and its relationship to the change.

   **LSP** (preferred when available): Use `textDocument/references` for precise direct dependents. Use `textDocument/implementation` to find concrete implementations of interfaces and abstract methods — this addresses the dynamic dispatch gap that grep cannot close. See [LSP integration](${CLAUDE_PLUGIN_ROOT}/docs/lsp/integration.md) for the operation catalogue and availability detection.

   **Fallback** (when LSP is unavailable): When the change surface spans multiple exports of a single module, `${CLAUDE_PLUGIN_ROOT}/tools/map-symbol-consumers.py` can enumerate direct dependents mechanically. For non-TypeScript projects, pass symbols explicitly rather than piping from `extract-ts-exports.py`.

2. **Trace transitive dependents**: for each affected direct dependent, does its behaviour change in a way that affects *its* dependents? If yes, repeat from step 1, one level further out. Continue until you hit a boundary (see below).

   Heuristics for deciding whether a change propagates through a dependent:

   - If the dependent's public interface is unchanged and its tests cover the affected behaviour, its dependents are likely unaffected — record the dependent as a boundary.
   - If the dependent transforms or wraps the changed behaviour (e.g., reformats output, maps error types), the transformation may absorb or amplify the change. Check which.
   - If the dependent passes the changed value through unmodified (e.g., returns it, stores it, forwards it), the change propagates — continue tracing.
   - When uncertain, note the uncertainty in the map rather than guessing. A gap flagged is more useful than a confident wrong answer.

   When LSP is available, `callHierarchy/incomingCalls` provides a mechanical backbone for transitive tracing — enumerate callers of each affected dependent rather than relying solely on reasoning about propagation. When LSP is unavailable, apply the heuristics above.

3. **Identify implicit dependents**: code that depends on behaviour without a direct code reference. See the implicit dependencies section in the dependency type catalogue. Most implicit dependencies cannot be found by mechanical search — they require understanding of how the system is used. Exception: in languages with structural typing (TypeScript, Go), LSP's `textDocument/references` finds shape-based dependents that have no import relationship — these would otherwise be implicit gaps. If you lack domain context for a specific area, flag it as a gap rather than speculating — a fabricated implicit dependency wastes more effort than an acknowledged gap.

4. **Note test coverage**: for each affected site, does a test exercise the affected behaviour? Having a test that *calls* the affected code is not sufficient — the test must verify the aspect that changed. Tests that touch the code but not the changed behaviour provide false confidence.

**Boundary detection — when to stop tracing:**

- **Stable interface**: a public API, module boundary, or abstraction layer whose contract is unchanged by this change. Dependents of the interface are not affected. But verify the interface is *actually* stable — undocumented side effects, leaked implementation details, and implicit contracts can make an interface unstable in practice.
- **Independent code**: code that uses the changed element but is demonstrably unaffected by this particular change. Requires positive evidence (e.g., caller ignores the changed return value and this is verifiable). Without evidence, the dependent remains in the map.
- **Repository boundary**: external consumers cannot be traced. Record them as gaps.

If the change surface is large and the dependency graph is deep, prioritise: trace high-risk elements first (signature changes, removals, data shape changes), then behavioural and contract changes.

Exit criteria: All elements of the change surface have been traced. Affected code sites are recorded with their relationship to the change. Boundaries are documented. Implicit dependencies are noted where identified.

### Phase 3: Classify

Risk-rank the impact map for consumption by the invoking skill.

For each affected site:

1. **Distance**: how far from the change surface?
   - *Direct*: references the changed element.
   - *Transitive*: affected through one or more intermediaries.
   - *Implicit*: depends on behaviour without a code reference.

2. **Coupling strength**: how tightly is the affected code coupled to the change?
   - *Structural*: compile-time dependency (type, signature). Breakage is caught by the compiler or type checker.
   - *Behavioural*: runtime dependency on specific behaviour. Breakage is caught by tests if they exist, silent otherwise.
   - *Implicit*: undocumented or unenforced dependency. Breakage is silent.

3. **Risk**: combining distance, coupling, and context:
   - *High*: direct + behavioural or implicit coupling. Or: any distance + no test coverage for the affected behaviour. Or: historically fragile code.
   - *Moderate*: direct + structural coupling (compiler will catch it). Or: transitive with test coverage.
   - *Low*: transitive + structural. Or: independent code recorded for completeness.

   **Data shape changes**: for changes affecting persistent or serialised data, "no test coverage for the affected behaviour" includes backward compatibility — old recordings replayed by new code, old serialised state deserialised by new code, old configuration read by new code. If no test exercises old-format data against new code, the risk is HIGH. Aggregate test suite passage is irrelevant; the gap is precisely that no test covers this path. See [dependency-types](${CLAUDE_PLUGIN_ROOT}/docs/dependency-types.md) §Data dependencies.

Exit criteria: Each affected site has a distance, coupling, and risk classification. The map is ordered by risk.

### Phase 4: Verify

Challenge the map. Actively seek affected code that was missed. Co-change history is a discovery mechanism, not just a verification step — it catches dependencies invisible to static analysis and grep: tools hardcoded to specific values, test fixtures assuming old shapes, configuration referencing removed entities. Treat Phase 4 findings as first-class discoveries, not confirmations of earlier phases.

1. **Check co-change history**: use the [co-change](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#co-change) operation to find files that historically change together with the change surface. Sites that frequently co-change but aren't in the map are suspicious — investigate and either add them or document why they're unaffected.

2. **Challenge exclusions**: for each dependent excluded in Phase 2, restate what would have to be true for it to be affected. Is that condition actually impossible, or merely not obvious? When LSP is available, use `textDocument/references` to verify that no references were missed by earlier tracing.

3. **Probe behavioural assumptions**: for behavioural and contract changes specifically: what assumptions might callers be making that aren't enforced by the type system? Check for code that parses error messages, depends on ordering, relies on timing, or assumes the absence of failure.

Exit criteria: Each exclusion has been challenged. Co-change history has been checked. Any newly identified affected sites are added to the map and classified per Phase 3.

## Dependency types

See [dependency-types](${CLAUDE_PLUGIN_ROOT}/docs/dependency-types.md) for the reference catalogue used in Phase 2 tracing.

## LSP integration

When a language server is available, it provides compiler-grade accuracy for dependency tracing operations that grep approximates. See [LSP integration](${CLAUDE_PLUGIN_ROOT}/docs/lsp/integration.md) for the full operation catalogue, invocation methods, and language-specific appendices.

### Availability

Attempt a real LSP operation on the first element of the change surface. If it succeeds, use LSP throughout. If it fails or no LSP mechanism is configured, fall back to grep-based methods and note the reduced mechanical coverage in the confidence section.

### Method strengths

Grep-based and LSP/MCP-based analysis have complementary strengths, corroborated across multiple controlled comparisons:

- **Grep** excels at broad discovery and co-change-based risk calibration. String-level pattern matching finds hardcoded values, configuration references, and co-change patterns that LSP's semantic model doesn't surface. Stronger at surfacing implicit dependencies through textual co-occurrence.
- **LSP/MCP** excels at deep behavioural reasoning within found code. Semantic understanding of type relationships, call hierarchies, and implementation chains. Stronger at tracing precise propagation paths and identifying behavioural coupling.

Neither method dominates. For high-stakes changes, note which method's strengths apply to each finding. A grep run that finds a string-typing gap and an LSP run that finds a behavioural drift are both correct — they found different things.

### LSP recommendation

After completing the analysis, if LSP was not available, recommend installing a language server to the user (once per project):

1. Check `~/.claude/projects/<project-dir>/memory/impact-analysis-lsp-recommended.md`. If the file exists, skip — already recommended.
2. Surface the recommendation in the analysis output: "Impact analysis ran without LSP — installing a language server would enable more precise dependency tracing. See ${CLAUDE_PLUGIN_ROOT}/docs/lsp/integration.md."
3. Write `~/.claude/projects/<project-dir>/memory/impact-analysis-lsp-recommended.md` with a timestamp to prevent repeat recommendations.

## Outputs

Present as a structured list, grouped by risk (high first), with one entry per affected site:

- **Impact map**: affected code sites, each with:
  - Location (file, line, function).
  - Relationship to the change (how it depends on the changed element).
  - Distance (direct / transitive / implicit).
  - Coupling (structural / behavioural / implicit).
  - Risk (high / moderate / low).
  - Test coverage for the affected behaviour (covered / not covered / partially covered).
- **Boundaries**: where tracing stopped and why.
- **Gaps**: what couldn't be traced (reflection, external consumers, implicit dependencies suspected but not confirmed) and why.
- **Confidence**: report on limitations, skipped steps, areas where domain knowledge was lacking, uncertainty in transitive propagation judgments, and implicit dependency coverage. Provide an overall confidence rating, acknowledging it will be a crude heuristic. Aggregate test suite passage is not a confidence factor — all tests passing is the expected baseline for any commit, not evidence of safety. Only specific test coverage for the affected behaviour is meaningful. In particular, test passage does not mitigate implicit dependencies — backward-compat gaps, migration gaps, and serialisation gaps are high risk precisely because no existing test covers them.

## Commit discipline

Impact analysis produces a map, not code changes. The map is consumed by the invoking skill and does not require its own commit. If the analysis is substantial and would be valuable to future maintainers, the invoking skill may choose to commit it as a design document or code comment.
