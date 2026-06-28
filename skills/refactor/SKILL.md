---
description: "Improve internal code structure without changing external behaviour — iterative deduplication (similar code to shared abstraction) and structural decomposition (monolithic function to focused functions). Use when consolidating duplication, splitting an oversized function, or extracting a clean abstraction."
allowed-tools: Read, Grep, Glob, Bash, Edit, Write, Task, Skill
---

# Refactor

Improve internal code structure without changing external behaviour. Two patterns: iterative deduplication (similar code → shared abstraction) and structural decomposition (monolithic function → focused functions with orchestrator).

## Interface

### Requires

- **scope**: What to refactor. One of: a file list, a directory, a module, a description of the code region, or an escalation context from phronithm:diagnose — a `root_causes` entry: `{file, lines: [start, end], description}`. Full schema: phronithm:diagnose skill Structured Output.

### Produces

- **generalisations**: List of applied generalisations, each with:
  - source-locations: [{file, lines: [start, end], function}] — all instances that were unified (lines are inclusive ranges)
  - abstraction-name: name of the extracted function
  - abstraction-location: {file, lines: [start, end]}
  - parameters: what varies between instances and how it is parameterised
  - rationale: why the abstraction is justified (shared intent, not coincidental similarity)
  - (candidates assessed but not applied are recorded in commit messages per step 5, not in this structured output)
- **decomposition-map**: List, each with:
  - source-function: {file, lines: [start, end], name} — the monolithic function
  - stages: [{name, inputs: [name], outputs: [name]}] — data flow boundaries
  - extracted-functions: [{name, signature, file, lines: [start, end]}]
  - orchestrator-function: {name, file, lines: [start, end]}
- **termination-rationale**: Which stop condition was hit, or that no more patterns were found. When the stop condition is the complexity-assessment threshold, include metric-deltas at termination: `{metric: {before, after, delta_pct}}` — this data feeds threshold calibration (see [complexity-assessment](${CLAUDE_PLUGIN_ROOT}/docs/lenses/complexity-assessment.md)).
- **changes**: Commit range (base..HEAD) covering all refactoring commits. Base is recorded by pre-flight; HEAD is current at workflow completion.
- **assessment-log**: For multi-file scopes, the per-file assessment signals (yes/no/maybe), rationales, and routing decisions. Absent for single-file scopes.

### Consumes

- **impact-map** (from phronithm:impact-analysis): Used in step 4 to determine what tests are relevant after a generalisation. When absent, test relevance is assessed manually.
- **static-analysis-findings** (from phronithm:static-analysis): Used in step 4 to verify no new issues introduced by refactoring. When absent, verification relies on test results only.
- **code-risk-assessment** (from phronithm:diagnose): When present, provides context on historically fragile areas — informs which duplications to prioritise and which to handle with extra caution.
- **characterisation-oracle** (from phronithm:pin-behaviour): When coverage of the scope is sparse or absent, establish a characterisation suite first — it is the behaviour-preservation oracle refactoring relies on. When absent, preservation rests on existing tests plus manual verification.

### Complexity drivers

- **Code volume**: <500 lines → low. 500–2000 → moderate. >2000 → high.
- **Coupling density**: Isolated module with few callers → low. Widely-used abstraction with many consumers → high. Use `${CLAUDE_PLUGIN_ROOT}/tools/map-symbol-consumers.py` to quantify, or LSP `textDocument/references` when available. When coupling density cannot be assessed at plan time (scope not yet resolved to specific symbols), default to moderate.
- **Test coverage**: Strong test coverage of affected code → low. Sparse or absent → high (refactoring without tests is high risk).
- **Behavioural subtlety**: Pure functions with no side effects → low. Functions with side effects, error handling differences, or performance-sensitive paths → high.
- **Scope size** (assessment phase): 1 file → no assessment overhead. 2–10 files → low (fast parallel haiku assessment). 10+ files → moderate (worth noting expected assessment time before starting).

Early-termination risk: complexity-assessment stop conditions cannot be predicted before the refactor runs. High code volume combined with high coupling density is the strongest pre-invocation signal that the skill may terminate early — treat it as a planning risk, not only a model-selection input.

## Workflow

### Startup

Follow the [pre-flight](${CLAUDE_PLUGIN_ROOT}/docs/pre-flight.md) check. Do not continue until it passes.

**WIP/parked code scan**: Before committing to any scope, scan the proposed targets for indicators of parked or WIP status:

- Run a grep for `TODO|WIP|FIXME` across all target files (including body and header comments, function names, and file-level notes).
- Check for a coverage heuristic: if no test file in the project references the target module or file by name, treat it as having no runnable tests.
- Check git log for the target files — if there has been no recent activity (no commits in the last 90 days) combined with no test coverage, treat this as a signal the code is parked.

If any target file or function matches two or more of these indicators (WIP markers present, no test coverage, no recent git activity), flag as:

> Target contains parked/WIP code with no tests — high risk to decompose. Confirm scope with user before proceeding, or down-scope to exclude these targets.

Do not proceed with refactoring until the user either confirms the scope explicitly or provides a narrowed scope that excludes the flagged targets.

### Assessment Phase

**Trigger**: When scope is a directory, module, file list with more than one file, or a description that resolves to multiple source files. For a single file, skip directly to Pattern Selection — but still apply the WIP and test-coverage check in Step 1 before proceeding.

**Step 1 — Enumerate candidates**

Expand the scope to a flat file list. Exclude:
- Generated, vendored, and build-output paths (`node_modules/`, `dist/`, `__pycache__/`, `build/`, `.git/`, etc.)
- Files under ~30 lines — too small to have meaningful refactoring opportunities.

Before proceeding, check for WIP markers (TODO/WIP/FIXME in function or class names, or file-level status comments) and test coverage. If any file in scope is WIP or has no runnable tests covering it, flag as:

> Target contains parked/WIP code with no tests — high risk to decompose. Confirm scope with user before proceeding, or down-scope to exclude these targets.

Do not begin the assessment until the user confirms or narrows the scope. Refactoring untestable code cannot be verified and risks silent regressions.

**Step 2 — Parallel assessment**

Spawn one haiku Task agent per candidate file, all in parallel. Each agent receives the file content and this instruction:

> Assess whether this file has refactoring opportunities — structural duplication (similar code blocks that could be abstracted) or monolithic functions with identifiable stages. Produce a signal only:
>
> - `signal`: yes / no / maybe
> - `rationale`: one sentence giving the basis for the signal
>
> Do NOT suggest specific refactoring approaches. Do NOT describe how the code should be changed. Signal only.

Expected output from each assessment agent:
```yaml
signal: yes
rationale: "Three nearly-identical validation blocks differing only in field name and error message."
```

If an agent's output cannot be parsed as a signal, treat it as `maybe`.

**Step 3 — Routing**

Aggregate the signals:

| Signal  | Default action                                |
|---------|-----------------------------------------------|
| `yes`   | Add to active set                             |
| `maybe` | Add to active set (flag as uncertain)         |
| `no`    | Skip — log the rationale                      |

If the active set is empty (all files returned `no`), terminate with an assessment summary.

**Step 4 — Report and proceed**

Log the full assessment results before proceeding:
- Included files: signal and rationale for each.
- Skipped files: rationale for each.

For active sets larger than 5 files, present this summary to the user for awareness before proceeding. The default is to proceed automatically.

### Pattern Selection

After assessment (or pre-flight for single-file invocations), assess the active set holistically (files that passed assessment, or the original scope for single-file invocations):

- **Structural duplication** present (similar code blocks across or within files) → use Iterative Deduplication.
- **Monolithic functions** with identifiable stages and data flow between them → use Structural Decomposition.
- **Both** present → apply Structural Decomposition first (decomposing a monolith may reveal duplication that wasn't visible in the tangled original), then Iterative Deduplication on the result.
  - **Exception**: if the duplicate blocks appear in separate functions or modules (not inside any monolithic function body), dedup first — decomposing multiplies visible duplication before extraction. If duplication is mixed (some blocks external to monoliths, some hidden inside), dedup the external blocks first, then apply Structural Decomposition on monolithic functions, then deduplicate across the full result.
- **Neither** clearly present → report that the scope does not match either pattern and terminate. The user can re-invoke with a more specific scope or use a different skill.

### Pattern: Iterative Deduplication

1. **Scan**: Find code blocks that are substantially similar in structure or intent. See [code-patterns](${CLAUDE_PLUGIN_ROOT}/docs/code-patterns.md) for the pattern catalogue and detection heuristics.

   When refactoring a module with many exports (constants files, barrel re-exports, utility modules), run:
   ```
   ${CLAUDE_PLUGIN_ROOT}/tools/extract-ts-exports.py <file> | ${CLAUDE_PLUGIN_ROOT}/tools/map-symbol-consumers.py --src-dir src/ --exclude <file>
   ```
   to classify symbols by consumer count. Single-consumer exports are co-location candidates. Zero-consumer exports are dead code. For non-TypeScript projects, pass symbols directly: `${CLAUDE_PLUGIN_ROOT}/tools/map-symbol-consumers.py --src-dir src/ FOO BAR`.

2. **Assess**: For each candidate pair/group:
   - What is the shared abstraction?
   - What varies between instances?
   - How would callers distinguish cases after generalisation?
   - What is a good name for the abstracted function? Something short that describes its functionality. Difficulty naming may imply an inappropriate abstraction.
   - Where there are differences, are there good reasons for them? If non-obvious flag it and clarify, do not assume. If there are good reasons, can they be generalised in such a way as to harmonise them?
   - Be wary of code duplicated for performance or other non-obvious reasons. Run benchmarks to verify no regressions if code may be a hot path. Document confirmed performance-sensitive cases in comments.

3. **Generalise**: Extract the common structure. The generalisation should be a clear improvement: less code, clearer intent, single point of change.
   - Behaviour preservation is the invariant, including of side-effects. Differences in behaviour should be considered on a case-by-case basis and justified only when the result does not break backward compatibility policy (NOTE: project specific). Exact log messages and other side-effects not intended to modify *user-visible* behaviour can be rationalised.
   - Error handling: be very careful in verifying error handling behaviour is *exactly* the same. Test this assumption with full test coverage. Add runtime assertions to verify equivalence.
   - Logging: review logging messages, rationalise and generalise as required.

4. **Test**: Run all tests that may be impacted.
   - If test coverage of the affected code is sparse or absent, establish a characterisation oracle first via phronithm:pin-behaviour — behaviour preservation cannot be verified against tests that do not exist.
   - Use phronithm:impact-analysis to determine what tests are relevant after a generalisation.
   - Run phronithm:static-analysis on refactored code to verify no new issues introduced (type errors, null safety violations, resource leaks). Refactoring should not degrade code quality.
   - Multiple test failures should be flagged prominently and logged for further investigation — this is a signal that assumptions are invalid or something unexpectedly complex is happening.

5. **Commit**: Summarise information learned, decisions taken, alternatives considered and rejected, assumptions that cannot currently be validated. Note unusually complex or historically fragile/buggy code that is impacted. Describe testing strategy and results.

6. **Re-scan**: The generalisation may have revealed new patterns. Existing code that looked different before may now match the new abstraction. Return to step 1.

7. **Stop** when further generalisation hits any of these:
   - Increases indirection depth (callers must trace through more layers to understand behaviour).
   - Requires callers to pass configuration or flags to distinguish cases that were previously just inline.
   - The complexity-assessment lens scores the refactored code as net negative by more than 15% on any metric or more than 5% on a majority of metrics (provisional thresholds — see [complexity-assessment](${CLAUDE_PLUGIN_ROOT}/docs/lenses/complexity-assessment.md) for calibration notes).
   - There is no significant progress (5%+ on any metric) for more than three iterations.

### Pattern: Structural Decomposition

**Trigger**: A monolithic function or method — too long for the stages to be clear inline — with identifiable stages and data flow between them. If the function is short enough that the stages are already obvious, decomposition adds indirection without improving readability. Two common shapes:

- *Pipeline*: output of one stage feeds the next (sequential dependency).
- *Fan-out*: multiple independent operations on shared input (parallel, no inter-stage dependency).
- Mixed pipelines with fan-out sub-stages are common.

Stages may be marked by comment headers, descriptive comments, blank-line groups, or nothing visible — data flow is the primary signal, not formatting.

**Steps**:

1. **Map stages** — identify boundaries from data flow. Each stage has identifiable inputs and outputs. Visual markers (comment headers, blank lines) are starting hypotheses; data flow confirms or overrides them.

2. **Assess data coupling** — branch:
   - *Clean handoffs* (each stage consumes only the direct output of the previous, or operates independently on shared input): proceed directly to extraction.
   - *Interleaved mutable state* (stages share and mutate complex state through parent scope, closures, or globals): consolidate into a structured data object (dataclass, typed dict, context struct) *before* extracting. If the resulting structure has many fields from disparate concerns, that signals the function lacks a single coherent responsibility — stop decomposition. The function needs redesign, not mechanical splitting. Report the finding and move on.

3. **Extract** — each stage becomes a focused function. Resolve captured scope: closures that access parent-function state become functions with explicit parameters (or methods on a type, depending on the language). The right scope resolution is language-dependent — the goal is explicit data dependencies, not a specific mechanism.

4. **Choose pipeline shape** — direct function calls with an orchestrator is the default. Use an alternative only when there is a concrete justification:
   - *Streams/iterators*: when stages are lazy, composable, and the data volume makes materialising intermediate results wasteful.
   - *Generator pipelines*: when stages produce elements incrementally and consumers don't need the full collection.
   - *Middleware chains*: when project conventions already use this shape (e.g. HTTP middleware, plugin hooks).

5. **Wire orchestrator** — original function becomes a thin table-of-contents: call extracted functions in order, thread outputs to inputs. No business logic in the orchestrator — it reads as a summary of the full operation.

6. **Test** — verify behaviour preservation. Decomposition-specific risk: closures that mutated parent-scope state (e.g. appending to a shared list) are now pure functions returning values. Verify that all accumulation has moved to the orchestrator and no mutation paths were lost. Run the full affected test suite; regressions are easier to fix immediately. If test coverage is sparse or absent, manually verify all mutation paths have been preserved — closure-to-pure-function conversion is the highest-risk aspect of decomposition — or establish a characterisation oracle first via phronithm:pin-behaviour.

7. **Commit and re-scan** — same commit discipline as the deduplication pattern. After committing, re-scan: the decomposition may have revealed duplication that was hidden in the monolith — if so, switch to the deduplication pattern for a follow-up pass.

**Stop conditions**: Stop when any of these apply:
- An extracted function has fewer than ~5 meaningful lines — it is likely over-decomposed.
- The orchestrator contains business logic rather than reading as a plain call sequence — decomposition has not cleanly separated concerns.
- The complexity-assessment lens scores the decomposed code as net negative (same provisional thresholds as the deduplication pattern — see [complexity-assessment](${CLAUDE_PLUGIN_ROOT}/docs/lenses/complexity-assessment.md) for calibration notes).
- The orchestrator exceeds ~20 lines or no longer reads as a clear summary of the operation.
- Inter-stage data coupling remains excessive despite state consolidation (step 2).

**Worked example**: A 250-line validation function in a Python project (`validate_definition` from [cartomancer](https://github.com/duaneg/cartomancer)). Before: 7 inline sections — mostly fan-out (independent validation checks) with a pipeline gate (schema errors halt spatial checks), closures (`_error`, `_warning`) mutating a parent-scope `findings` list. After: 7 focused functions each returning their own findings list, 4 module-level helpers with explicit `findings` parameter, ~20-line orchestrator. Key decision: functions producing derived state consumed by later stages return tuples (e.g. `(findings, room_ids, room_map)`) — this interface shape threads data through the pipeline without shared mutable state.

## Termination principle

Three similar lines of code is better than a premature abstraction. The goal is removing *accidental* duplication — code that is the same because it does the same thing. Code that merely looks the same but serves different purposes should stay separate. When in doubt, don't generalise.

A 200-line function with clear section comments is better than 15 tiny functions threaded together by a complex context struct. The goal is explicit data flow and testable stages, not maximum decomposition.

After the pattern terminates, perform a documentation check: check whether the refactoring affects concepts documented in CLAUDE.md or project design docs. For structural changes (files moved, renamed, or modules consolidated), update key-file listings. For interface changes (renamed exports, changed signatures, moved responsibilities), update any existing docs that reference the modified concepts.

## Commit discipline

See [agent-protocols § Commit discipline](${CLAUDE_PLUGIN_ROOT}/docs/agent-protocols.md). Additional refactor-specific guidance:

- Branch naming: `git checkout -b refactor/<scope-description>`.
- After each commit, check whether a stop condition has been reached before continuing.
- After context compaction, treat all prior file reads as stale — re-read any file before editing, even if it was read earlier in the session.

## Tests

- Duplication in actual test representations is often justified. Duplication in utility and helper code is not.
