---
description: "Structure an open investigation from question to result-with-rigour: frame the claim and a rigour target, attack it cheapest-test-first, let results reshape the question, and settle each claim at its achieved rigour through a critique gate. Use when the deliverable is a finding — a research question, a code/perf spike, the root cause of an open question, or unfamiliar-API discovery — rather than a known build."
allowed-tools: Read, Grep, Glob, Bash, Write, Edit, Task, Agent, Skill
---

# Investigate: Settle an Open Claim

Take a question or claim and drive it to a **result with an explicit rigour
level**. The investigative sibling of `phronithm:feature`: it shares phronithm:feature's epistemic
backbone but reshapes the core around a **moving target** and a
**claim-plus-evidence** deliverable instead of a fixed spec and a diff.

Domain-general. Research and mathematics are one instance; code and performance
spikes, the root cause of an open question, and unfamiliar-API discovery are
others. Domain specifics — the compute stack, source/paper conventions, a finer
rigour ladder, document save locations — are deferred to the project's
`CLAUDE.md`, the same deferral pattern `phronithm:feature` uses.

The core is the [investigation loop](${CLAUDE_PLUGIN_ROOT}/docs/investigation-loop.md), not a
linear pipeline. Where `phronithm:feature` advances through phases toward a known target,
`investigate` runs the loop until a claim is settled or the question is sharpened
into a better one.

## Interface

### Requires

- **question or claim**: ranges from vague ("is X non-polyhedral?", "is library
  A faster than B here?") to a precise conjecture. Vaguer input means more Frame
  work.
- **rigour target**: what would count as settled, from an ordered ladder. Default
  ladder (shared with `phronithm:feature`, project-overridable in `CLAUDE.md`):
  **`proven` ▸ `evidenced` ▸ `conjectured`** —
  - *proven* — held by a reproducible automated check (test, CI benchmark) or a
    formal/kernel-checked proof; it re-checks itself.
  - *evidenced* — actively established at least once (a run, a construction, a
    measurement, a certificate), but not locked in by automation.
  - *conjectured* — inferred from reading or reasoning; not executed this cycle.

  A maths project may substitute a finer ladder (e.g.
  `kernel-checked ▸ exact-computed ▸ numerical-evidence`). Do not hardcode
  domain-specific rungs here.

### Produces

Save documents under `docs/investigations/<name>-...` (create the directory if
absent) unless the project's `CLAUDE.md` specifies otherwise.

- **problem-statement** (`<name>-problem-statement.md`, frozen at Frame exit): the
  precise statement(s); a survey of known results (prior notes, literature, the
  project's source index); the rigour target; and an **attack ladder** —
  candidate approaches ordered **cheapest-test-first**, each annotated with *what
  it would establish, at what rigour* and a *falsifiable cheap test that could
  kill it*.
- **investigation-results** (`<name>-results.md`, living): the current-state
  document updated throughout the Investigate loop — every claim with its
  **achieved rigour level** and, when below target, an explicit **upgrade path**.
  A snapshot of what is true *now*, not a work-log (history lives in version
  control and memory). This is the document that mutates as findings land; the
  problem-statement is the initial framing snapshot.
- **result**: not a separate file — the **finalised** investigation-results at
  close, with each claim marked settled at its achieved rigour and the sharpened
  open obligations listed. Settle/Review produces it by finalising the living
  document.

### Consumes

- **[investigation-loop](${CLAUDE_PLUGIN_ROOT}/docs/investigation-loop.md)**: the core
  hypothesis → test → update-belief → re-plan micro-loop. This skill runs it; it
  does not re-describe it.
- **review gate** — `phronithm:critique` run in a separate context:
  [critique-design](${CLAUDE_PLUGIN_ROOT}/docs/critique/critique-design.md) is the **primary**
  appendix for any finding (does the experimental or argument design hold —
  independent paths, sufficient evidence, no confound?). **Supplementary**
  [critique-maths](${CLAUDE_PLUGIN_ROOT}/docs/critique/critique-maths.md) + the
  [numerical](${CLAUDE_PLUGIN_ROOT}/docs/lenses/numerical.md) lens *only when the artefact is
  mathematical* (the formal↔computational bridge, verification independence,
  floating-point claims).
- **project `CLAUDE.md`**: compute-stack invocation, source/paper conventions, any
  finer rigour ladder, document save locations.
- **`phronithm:breakdown`**: reuse its dependency-ordering machinery for the attack ladder
  rather than reinventing — the attack ladder is phronithm:breakdown's ordering specialised
  to *order by cost-to-falsify, with explicit kill-criteria*.

## Workflow

### Startup

Follow the [pre-flight](${CLAUDE_PLUGIN_ROOT}/docs/pre-flight.md) check. Do not continue until
it passes.

### Frame

Turn a question into an attackable shape.

1. **State** the claim(s) precisely. An imprecise claim cannot be falsified —
   sharpen it until a result could decide it. Identify which claims are
   **load-bearing**: a claim is load-bearing if, were it false, the result
   changes. (As claims accrue rigour in Investigate, re-evaluate: a claim also
   becomes load-bearing if its falsity would invalidate another claim's upgrade
   path — but upgrade paths do not exist yet at Frame, so judge on the result
   alone here.) These are the claims the review gate must cover.
2. **Set** the rigour target (above). This is the bar every claim is measured
   against; record it.
3. **Survey** known results: prior notes, the project's source/paper index,
   literature, existing code. A cheap literature or git check can settle or kill a
   claim before any compute. Read the **load-bearing** sources inline — framing
   depends on them — but **delegate breadth scans** (which of N notes are
   relevant? does any prior result already settle or kill a sub-claim?) to
   Task/Explore agents, subject to the project's subagent-verification rule (no
   structured consistency checks via Explore — quote-the-evidence or inline-verify
   those instead). On a history-rich project this step is otherwise a large inline
   read.
4. **Build the attack ladder**: candidate approaches ordered cheapest-test-first
   (compute/construct → certify → prove), each annotated with what it establishes,
   at what rigour, and a falsifiable cheap test that could kill it. Use
   `/phronithm:breakdown` when the ladder has **non-linear dependencies or parallelism to
   exploit** — that is where its decomposition machinery earns its cost; skip it
   for a short cost-ordered chain even when the rung count is high. The deciding
   factor is *dependency structure*, not rung count. When used, `/phronithm:breakdown`
   *decomposes and dependency-orders* the candidates; then re-rank that output by
   cost-to-falsify — breakdown supplies the decomposition, the kill-cost ordering
   is this skill's specialisation and is not what breakdown produces.
5. **Save** the problem-statement. It is the framing snapshot — frozen here; the
   living record from now on is investigation-results.

### Investigate

Run the [investigation loop](${CLAUDE_PLUGIN_ROOT}/docs/investigation-loop.md) off the attack
ladder. The skill does not restate the loop's mechanics; it specialises them:

- The **attack ladder is the test queue** — each iteration takes its cheapest
  unresolved rung.
- **investigation-results is the loop's current-state document** — update it after
  every result, recording each claim at its **achieved** rigour with an upgrade
  path when below target.
- When a result **reshapes the question** (the loop's most valuable outcome — e.g.
  "the extreme rays are transient" turns "find fixed witnesses" into "prove
  drift-convergence"), record the pivot *in investigation-results* ("question was
  X, now Y, because Z") and re-rank the ladder. Do **not** edit the frozen
  problem-statement; the original framing has archival value, and the recorded
  pivot is what keeps the two documents reconcilable.

**Bounded retry**: this is the [investigation
loop](${CLAUDE_PLUGIN_ROOT}/docs/investigation-loop.md)'s bounded-retry, with *hypothesis-family*
read as **attack-family** — the same single ≤ 3 counter, not a second budget. An
*attack family* is a group of tests sharing one underlying approach or tool (e.g.
all LP-relaxation tests are one family regardless of parameter variation). After
≤ 3 attack-families without new evidence, escalate to the requester with what was
tried and learned. The default failure mode here is grinding on a moving target;
the reassessment-and-re-rank at each iteration is what stops it.

When delegating a test to a subagent, use the **Task** tool — data-gathering only,
the same Task/Agent split `phronithm:feature` draws (Task for delegation, the
context-separating Agent only for the critique gate below). Include an explicit
completion requirement ("continue through to completion — do not stop mid-task to
wait for further instruction"), tell it not to spawn further subagents, and have
it return results inline (subagents cannot reliably write files — see [file
writes](${CLAUDE_PLUGIN_ROOT}/docs/subagent-protocol.md#file-writes)).

### Settle / Review

1. **Critique** each load-bearing claim: run the [critique gate](${CLAUDE_PLUGIN_ROOT}/docs/critique-gate.md), applying its
   default iteration policy — it runs off this session's window and returns only
   findings, so the gate costs the orchestrating session little. The orchestrating
   session owns this gate (rather than a data-gathering test subagent) because it
   is what acts on the findings and re-runs critique after fixes. Pass
   `critique-design` always; add `critique-maths` + the `numerical` lens when the
   claim is mathematical. The highest-risk surface for computational claims is the
   formal↔computational bridge and verification independence — does each path
   actually check the claim, and is the trusted base explicit? Address findings
   and re-run until the gate passes; at the point the iteration policy would
   otherwise escalate to a requester, record the residual explicitly as an open
   obligation with an upgrade path instead — an investigation has no synchronous
   requester waiting on this loop.
2. **Finalise** investigation-results into the **result**: mark each load-bearing
   claim settled at its achieved rigour, and list the sharpened open obligations.
   When achieved rigour is below target, the upgrade path *is* the next
   investigation's first ladder rung. Commit the finalised document.

Exit criteria: every load-bearing claim is rigour-tagged and has passed critique,
or is explicitly recorded as an open obligation with an upgrade path. The
finalised investigation-results (the result) is self-contained.

## Commit discipline

See [commit discipline](${CLAUDE_PLUGIN_ROOT}/docs/vcs.md#commit-discipline). The
problem-statement and investigation-results documents are the primary record;
commit them as findings land, not only at the end.

## Deliberately not

- **Not a theorem prover or tactic engine.** It *structures* an investigation and
  marshals existing tools (CAS, LP/MILP, SAT, proof assistants, profilers,
  benchmarks); it does not replace them.
- **Not a replacement for `phronithm:feature`** when the deliverable is code. When an
  investigation yields a shipped tool, `phronithm:feature` governs the build and
  `investigate` governs the investigation that motivates and consumes it — they
  interleave. Hand off to `phronithm:feature` once the finding is settled and the work
  becomes building the thing it implies.
- **Not a substitute for `phronithm:diagnose`.** If the question is "what is broken and how
  do I fix it" on a known-broken system, use `phronithm:diagnose` (its success condition is
  a verified fix). Use `investigate` when the question is "what is *true* about
  this system" and the success condition is a settled claim.
- **Not a substitute for `phronithm:persona-debug`.** `phronithm:persona-debug` is `phronithm:diagnose` for
  instruction specs — a known-misbehaving persona or skill where success is a
  verified fix. Same carve-out as `phronithm:diagnose`: reach for `investigate` only when
  the question is what is *true* about the behaviour, not which instruction to fix.
