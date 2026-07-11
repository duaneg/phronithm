---
description: "Generate characterisation tests that pin code's current observable behaviour — bugs and all — so a later change can be proven behaviour-preserving. Produces a test suite green against current code plus a manifest of what was pinned and what could not be. Use before refactoring or changing unfamiliar code, to establish a behavioural oracle."
allowed-tools: Read, Grep, Glob, Bash, Write, Edit, Task, Agent, Skill
---

# Pin Behaviour: Characterisation Tests

**Characterisation ≠ correctness.** This skill generates *characterisation*
tests — golden-master / approval tests in the Feathers sense: they assert what
the code **does now**, bugs and all, not what it *should* do. The deliverable is
a test suite that is green against the code as-is, plus a manifest of what was
pinned and what could not be. That suite is the oracle a later change is measured
against: if it stays green, the change preserved observable behaviour.

The suite pins *current* behaviour. It is never a specification, and the report
must never present it as one — a pinned bug is a pinned bug, recorded faithfully.

## Interface

### Requires

- **target**: the surface to pin — entry points (functions, methods, CLI, HTTP,
  a module API) as symbols or a scope. The unit of "behaviour".
- **runtime**: how to build and run the target's tests — the harness the
  generated tests join. Project-specific build/run commands are deferred to the
  project's `CLAUDE.md` (the same deferral pattern `phronithm:investigate` uses), not
  hardcoded here.
- **oracle-inventory** (optional; derived if absent): what observable outputs and
  effects to capture for the surface (return values, thrown errors, emitted
  events, writes, stdout) and, crucially, what is *not* observable or *not*
  deterministic. When absent, the skill derives a first cut by inspecting the
  target and surfaces the gaps for awareness.

### Produces

- **characterisation-suite**: the generated tests, green against current code, in
  the working directory. Labelled as pinning *current behaviour*, not a spec.
- **manifest** (structured): written as JSON to `characterisation-manifest.json`,
  co-located with the generated test suite. It is a file, not an in-session data
  structure — the Phase 4 critique agent runs in a separate context and must be
  able to read it. Shape:
  - `pinned[]`: `{entry-point, oracle, capture-kind, evidence}` where
    `capture-kind` is one of `value | snapshot | exception | event` — distinct
    oracle shapes (a return value, a golden-master snapshot, a thrown error, an
    emitted event). Auto-accepted snapshots are recorded here with
    `capture-kind: snapshot`, so the critique gate and downstream review can
    scrutinise them.
  - `unpinnable[]`: `{entry-point-or-behaviour, reason}` where `reason` is one of
    `non-determinism | external-dependency | unreachable | unobservable`.
  - `surface-coverage`: `{pinned-count, total, gap-count, summary}` — the honest
    metric (see Invariants).
- **critique-verdict**: the result of the Phase 4 critique gate.

### Consumes

- **impact-map** (from impact-analysis, optional): completes the observable
  surface — finds entry points and implicit consumers that inspection alone
  misses. When absent, the surface is derived from the target by inspection.
- **project `CLAUDE.md`**: runtime (build/run) invocation and any project-specific
  snapshot/approval tooling.

### Signals

- **consumed by → phronithm:refactor**: when coverage of a refactoring scope is sparse or
  absent, run this skill first to establish the behaviour-preservation oracle
  before deduplication or decomposition.
- **consumed by → extension adapters**: thin callers (e.g. module-scale or rewrite
  refactoring extensions) that supply an `oracle-inventory` derived from their own
  analysis. The adapter owns the extension's concerns (environment/cwd setup, run
  lifecycle); this skill stays generic and stateless and owns neither.

### Complexity drivers

- **Surface size**: few entry points with small return surfaces → low. Many entry
  points, or large/structured output → high.
- **Determinism**: pure, deterministic functions → low. Clocks, RNG, IO,
  ordering, or concurrency in the observable path → high (each is a seam that must
  be controlled or quarantined).
- **Observability**: return values only → low. Side-effects (writes, emitted
  events, stdout) that must be intercepted to observe → high.
- **Oracle clarity**: a supplied `oracle-inventory` → low. Derived from inspection
  with gaps to surface → high.

## Workflow

### Startup

Follow the [pre-flight](${CLAUDE_PLUGIN_ROOT}/docs/pre-flight.md) check. Do not
continue until it passes.

### Phase 0 — Confirm the runtime

Build the target and run its existing suite (or a trivial/empty test) to prove
the harness works and the generated tests will actually execute. **Full toolchain
or halt — no degraded path.** A broken runtime means a "green" generated suite
proves nothing: the tests may not be running at all. Halt and report rather than
proceeding on a runtime you cannot trust (precedent: `phronithm:static-analysis`'s
missing-tool stance).

### Phase 1 — Establish the oracle

For each entry point, determine its observable surface: return values, thrown
errors, emitted events, writes, stdout. Use the supplied `oracle-inventory` or
derive one by inspecting the target. Classify each behaviour:

- **deterministically pinnable** — capture directly.
- **pinnable-with-control** — pinnable once a seam is controlled; record the seam
  (clock / RNG / ordering / IO).
- **not pinnable** — quarantine as a manifest gap (`unpinnable[]`) with its
  reason.

When the inventory is derived rather than supplied, **surface the inventory and
the gaps for user awareness**: present them as an inline summary before continuing
to Phase 2, without waiting for a response — informational, like `phronithm:refactor`'s
assessment summary, not a blocking gate.

### Phase 2 — Capture

**Run the target code** — via Bash or the test harness — and capture the
*actual* outputs. Do **not** infer outputs by reading source: assert only
behaviour you have observed from execution. (An inferred assertion that happens to
be wrong produces a red test at Phase 3, where "fix the capture" would then push
you to patch the assertion to match a bad guess — silently corrupting the oracle.)
Record outputs as:

- **value / return / exception assertions** where the surface is small and stable.
- **golden-master snapshots** where output is large or structured. Snapshots are
  generated from current behaviour and **auto-accepted** as the oracle — there is
  no inline human-approval step. Each is recorded in `pinned[]`
  with `capture-kind: snapshot` so the Phase 4 critique and the downstream
  git-diff review can scrutinise it.
- **Control non-determinism at the seam** — inject the clock/RNG, fix ordering —
  rather than asserting loosely. A behaviour that cannot be made deterministic is
  a manifest gap, not a flaky test.

This non-determinism discipline is the **load-bearing guard** now that snapshots
auto-accept. A snapshot embedding a timestamp, memory address, or iteration order
is the main way auto-accept goes wrong: it must be caught here (seam control) or
at Phase 4 (critique), never asserted loosely.

### Phase 3 — Verify

Run the suite. It is **green by construction** — it was generated from observed
behaviour. A red test means the capture is wrong: over-tight, or uncontrolled
non-determinism. Fix the capture or quarantine the behaviour as a gap. **Never
weaken an assertion to hide non-determinism** — a loosened assertion that passes
on noise pins nothing. Quarantine the behaviour instead: add it to `unpinnable[]`
with `reason: non-determinism` and remove the test.

Compute **surface coverage**: which entry points were pinned, which were not, and
why. This populates `surface-coverage` and the `unpinnable[]` list. **Write the
manifest** to `characterisation-manifest.json` now, before the Phase 4 gate — the
critique agent reads it from there.

### Phase 4 — Critique (gated)

Run the [critique gate](${CLAUDE_PLUGIN_ROOT}/docs/critique-gate.md) on **the
suite + manifest** with the `critique-code` appendix. The orchestrating session owns
this gate because it acts on the findings and re-runs critique after fixes. This
gate is the **primary quality defence** so its snapshot scrutiny is critical.

**Calibration** is critical: if it is dropped or mis-stated, the critique agent
fires on the *intentional* absence of correctness judgement and on
auto-accepted snapshots, producing spurious Criticals that halt the workflow.
Pass it to the agent **verbatim** — copy this block into the spawn prompt
rather than paraphrasing it:

> Critique this artefact as CHARACTERISATION tests — they pin the target's CURRENT
> observable behaviour, bugs and all, NOT what the code should do.
> - The deliberate ABSENCE of correctness judgement is correct by design and MUST
>   NOT be scored as a defect.
> - Do NOT flag these deliberate decisions as omissions: no separate
>   workspace/write-isolation step; golden-master snapshots auto-accepted rather
>   than human-approved inline.
> - DO flag (Significant or worse): an asserted behaviour that is not actually
>   observable; an uncontrolled non-deterministic assertion, OR a snapshot
>   embedding uncontrolled non-determinism (timestamp / address / iteration-order
>   → will flake); an entry point silently left unpinned — present in the target
>   but absent from BOTH the manifest's `pinned[]` and `unpinnable[]` lists.

The gate passes on **no Critical or Significant findings** under this calibration.
**Disposition rule for each finding**: if it can be resolved by controlling the
seam or quarantining the behaviour, do so and re-run critique **once**. If the
same finding recurs after that single re-run and the seam cannot be controlled,
record it as a residual gap in `unpinnable[]` and proceed — do not iterate further
on that finding. This bounds the gate to at most one re-run per finding family. A
finding recorded as a residual gap is documented debt, not a blocker: only an
unresolved *and unrecorded* Critical or Significant finding blocks the gate.

### Phase 5 — Report

Report: the suite path, the surface-coverage summary, the gap list, and the
critique verdict. **Flag every `capture-kind: snapshot` entry as
auto-accepted** — "review the snapshot diff before relying on this oracle". The
snapshots auto-accept, so the report must not imply they were human-vetted. In
fully-autonomous or adapter mode with no downstream reviewer, that review
obligation passes to the caller; say so. Reiterate the
current-behaviour-not-spec label.

Commit the suite and manifest together (see Commit discipline).

## Invariants

The load-bearing principles, recapped for at-a-glance reference (each is applied
in the phase noted; this section is the index, not the definition):

- **Characterisation ≠ correctness** (intro). The suite pins what the code *does*,
  not what it *should*. Never present it as a specification.
- **Non-determinism is controlled or quarantined, never asserted loosely**
  (Phases 1–2). An uncontrollable behaviour is a manifest gap, not a flaky test.
  This is the primary guard against an auto-accepted snapshot going silently
  wrong.
- **Surface coverage is the metric** (Phase 3). "3 of 7 entry points pinned, 4
  unpinnable" beats an implied completeness. Report it; never imply more.

## Deliberately not

- **Refactor or otherwise change the target.** This skill only observes and pins;
  `phronithm:refactor` changes code.
- **Judge whether current behaviour is correct.** Pinning bugs faithfully is the
  point.
- **Measure suite adequacy.** That is the planned `mutation-test` skill — coverage
  of *change-detection*, not creation.
- **Generate inputs for behaviour classes.** Property-based testing and fuzzing
  (planned) generate inputs for *classes* of behaviour; this pins *specific
  observed* behaviour.

## Relationship to other skills

- **phronithm:refactor**: consumes this as a pre-step — the characterisation suite is the
  behaviour-preservation oracle refactoring relies on when coverage is sparse.
  This skill never refactors.
- **mutation-test** (planned): measures the *adequacy* of an existing suite; this
  *creates* one. Complementary, not overlapping.
- **property-based / fuzzing** (planned): generate inputs for classes of
  behaviour; this pins specific observed behaviour. Different tools.
- **phronithm:critique**: gates the generated suite (Phase 4), in a separate context, under
  the characterisation calibration above.
- **phronithm:impact-analysis**: optional input — its impact map completes the observable
  surface and finds entry points inspection alone misses.

## Commit discipline

See [commit discipline](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#commit-discipline).
The suite and its manifest are committed together — the manifest is the record of
what the suite does and does not pin, and is meaningless apart from it.
