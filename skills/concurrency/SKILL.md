---
description: "Verify concurrent code correctness by building and validating a concurrency model. Use when reviewing code with shared state, locks, async/threads, or analysing a suspected race condition or deadlock."
allowed-tools: Read, Grep, Glob, Bash, Task
---

# Systematic Concurrency Analysis

Verify concurrent code correctness by building and validating a concurrency model.

## Interface

### Requires

- **scope**: What to analyse. One of: a file list, a module, a directory, or a change set (diff/commit range).

### Produces

- **concurrency-model**: Shared state inventory with synchronisation classified by model (locks, atomics, message-passing, async, immutability) and access patterns (which threads read/write, under what conditions).
- **findings**: List, each with location, concern, severity (per critique), concurrency model violated, and suggested fix if clear.
- **stress-test-results** (when applicable): Test parameters, iteration count, tool used, pass/fail, reproduction details for failures.
- **gaps**: Shared state that could not be fully verified, with reason (aliased references, DI-shared state, missing test harness).
- **confidence**: Limitations, skipped steps, coverage boundary, items flagged for manual review.

### Consumes

- **impact-map** (from phronithm:impact-analysis): Used in scoping to limit analysis to concurrency affected by a change. When absent, the full scope is analysed.

### Signals

- **request → phronithm:diagnose**: During Phase 3, stress test failure may require phronithm:diagnose skill to isolate the failure before re-verifying in model context.

### Complexity drivers

- **Shared state count**: 1–3 items → low. 4–10 → moderate. >10 → high.
- **Concurrency models**: Single model (e.g., all locks) → low. Mixed models → high.
- **Lock-free / unsafe code**: Absent → lower. Present → high (requires memory ordering analysis, formal verification consideration).
- **System scope**: Single module → low. Cross-module shared state → high.

## Scoping

Single-process systems only. Abort with explanation if asked to review distributed systems, microservices, RPC.

For change reviews, use phronithm:impact-analysis to limit scope to concurrency affected by the change.

## Scaling

- **Trivial** (single lock, lone background task with no shared state beyond a channel): verify synchronisation at access sites, check relevant defect patterns, record findings. Skip Phase 3 unless hot path or history of bugs.
- **Moderate** (few shared resources, one concurrency model, localised synchronisation): full Phases 1-2, scope Phase 3 to high-risk patterns.
- **Complex** (multiple shared resources, mixed models, lock-free algorithms, complex shutdown): full workflow.

When in doubt, start Phase 1. The model size tells you which path you're on.

## Workflow

### Startup

Follow the [pre-flight](${CLAUDE_PLUGIN_ROOT}/docs/pre-flight.md) check. Do not continue until it passes. This analysis is read-only through Phase 2; the check matters once Phase 3 stress-test harnesses or fixes are committed.

### Phase 1: Model

Build a concurrency model:

1. **Trace spawn points**: find every thread/goroutine/task creation. For each: what data flows in (parameters, captures, referenced objects)? Trace mutable fields one level deep; go deeper when a field references shared mutable state.
2. **Scan for global mutable state**: mutables at global/static/module scope, including behind accessors: singletons, lazy instances, caches, pools, registries. If a function returns mutable state it didn't create, trace where it lives.
3. **Classify synchronisation**: for each shared state item, what controls access? Classify by concurrency model. Multiple models in one system is complex — flag it. Unprotected state is a finding.
4. **Map access patterns**: which threads read, which write, in what order, under what conditions?

**Prioritise by risk** (highest first):

1. Mutable state with no obvious synchronisation
2. State accessed from hot paths or error-handling paths
3. Complex access patterns (multiple writers, conditional access)
4. Simple, well-encapsulated synchronisation

Checkpoint after each tier. If findings exist and the system is large, proceed to Phase 2 with the partial model. If clean, continue — absence of obvious risks makes subtle ones (aliased references, DI-shared state) relatively more likely.

Document coverage boundary and search methodology limitations (spawn-point tracing and global scanning miss aliased references, DI-shared state, non-obvious closures).

**Exit**: high-priority shared state (tiers 1–2 minimum) catalogued, synchronisation classified, access patterns mapped, coverage boundary documented.

### Phase 2: Verify

For each shared state item, verify against its concurrency model (see below).

Then check cross-cutting concerns:

- **Atomicity**: TOCTOU: are compound check-then-act sequences protected as a unit?
- **Liveness**: deadlock, livelock, starvation across all code paths.
- **Cancellation/shutdown**: resource cleanup, shutdown deadlocks, state consistency.

Document candidate runtime assertions and synchronisation dependencies.

**Exit**: each shared state item verified against its model; cross-cutting concerns checked or finding recorded for each.

### Phase 3: Stress test

If a test harness exists:

1. **Design**: exercise concurrent paths under contention. Use barriers to force simultaneous arrival at race windows. Minimise work between synchronisation operations.
   - Thread count: 2x cores baseline, 8-16x for lock contention.
   - With sanitiser/race detector: 100+ iterations.
   - Without: 1000+ iterations (10000 preferred), 30+ seconds wall-clock.
2. **Run** repeatedly with available tools (ThreadSanitizer, `-race`, Helgrind, Miri).
3. **Record** parameters and results. If Phase 2 identified a plausible race and stress passes, increase iterations 10x before accepting.

If infeasible, note the gap and rely on Phase 2.

**Exit**: stress tests pass, or findings recorded with reproduction details.

### Backtracking

- **Phase 2 → 1**: verification reveals unmodelled shared state. Update model, resume.
- **Phase 3 → 2**: stress test failure. Diagnose with phronithm:diagnose, re-verify fix in model context.
- **Bound**: >3 iterations per edge → escalate for manual review with model documented.

## Concurrency models

### Locks and mutual exclusion

**Verify:**

1. All access sites for shared state acquire the same lock. Check for unguarded paths.
2. Lock ordering consistent everywhere. Document all nested lock acquisitions.
3. Granularity appropriate — too coarse = contention, too fine = complexity/deadlock.
4. Memory visibility guarantees sufficient (usually implicit, verify).

**Defect patterns:** lock ordering violations, forgotten unlocks (early return/exception), granularity mismatch, blocking in critical sections (IO, allocation, unknown code), signal/notification races.

**Stress:** maximise contention at specific synchronisation points using barriers.

### Atomics and lock-free

**Verify:**

1. Memory ordering per operation — acquire/release vs seq_cst vs relaxed. Verify sufficiency.
2. Algorithm correctness against published versions. Watch for ABA, missing barriers, incorrect CAS loops.
3. Memory reclamation safety (hazard pointers, epoch-based, RCU).
4. Formal verification where feasible.

**Defect patterns:** insufficient memory ordering (correct on x86/broken on ARM), ABA, missing reclamation (use-after-free under contention).

**Stress:** test on weak memory architectures or with reordering simulation. Sanitisers especially valuable.

### Message-passing (channels, actors, CSP)

**Verify:**

1. Channel dependency graph — trace for circular send/receive dependencies (deadlock).
2. Ordering — verify no cross-channel ordering assumptions (only FIFO within a single channel).
3. Back-pressure — unbounded channels → OOM; bounded → block/drop. Verify which is intended.
4. Shutdown — signal propagation reaches all participants. Messages in flight at shutdown: processed, drained, or lost — verify intent.
5. Error propagation — failures observable by the other side of every channel.
6. Message ownership — mutable state in messages after send = shared mutable state. Flag and verify.

**Defect patterns:** channel dependency cycles, orphaned participants, unbounded queue growth, mutation after send, select starvation, lost messages on shutdown.

**Stress:** vary producer/consumer rate ratios. Test shutdown under load. Inject participant failures.

### Async and cooperative concurrency

**Verify:**

1. **Executor starvation**: find every blocking call in async context (sync IO, CPU-bound work, sleep, lock acquisition). Blocks all other tasks on that executor.
2. **State across awaits**: other tasks run during suspension. Verify no assumptions of stability across await points.
3. **Resources across awaits**: locks, connections, handles held across awaits exhaust pools under load.
4. **Cancellation**: cleanup at await points: do handlers run? Are child tasks cancelled or orphaned? Can cancellation leave inconsistent state?
5. **Error propagation**: fire-and-forget task failures must be observable.
6. **Task lifecycle**: spawned tasks without join/scope boundaries; tasks outliving logical parents.

**Defect patterns:** blocking the executor, await-point races, resource exhaustion from held pools, silent task failure, cancellation leaks, orphaned tasks.

**Stress:** flood executor to expose starvation/pool exhaustion. Cancel tasks at random await points. Monitor task count growth.

### Immutability and ownership

Safety enforced structurally (ownership/borrow systems, persistent data, STM) rather than by runtime synchronisation. Eliminates data races by construction but not all concurrency bugs — focus on boundary escapes.

**Verify:**

1. **Boundary escapes**: every concurrency boundary crossing. Verify type system/runtime enforces guarantees at each. In Rust: `Send`/`Sync` bounds, `unsafe` bypasses.
2. **Interior mutability**: each `Cell`/`RefCell`/`Mutex<T>`/atomic is a deliberate opt-out. Verify primitive matches access pattern (`RefCell` in `Send` type → runtime panic).
3. **Cloning costs**: copies at boundaries intentional and bounded? Deep clones on every send → latency/memory pressure.
4. **Deadlock**: ownership prevents data races, not deadlocks. Apply lock/message-passing deadlock analysis.
5. **Unsafe/FFI**: all mutable state through these boundaries is unprotected. Verify via locks/atomics model.
6. **STM**: transactions side-effect-free (retries re-execute). High-contention transactions → retry storms.

**Defect patterns:** unsafe escape hatch misuse, RefCell in concurrent context, clone avalanche, STM side effects.

**Stress:** contention at boundary crossings. Sanitisers (Miri, ThreadSanitizer) for unsound unsafe. For STM, measure retry rates under contention.

## Patterns

Cross-model defect categories:

- **Unprotected shared state**: no synchronisation. Most common, most damaging.
- **TOCTOU**: non-atomic check-then-act (file existence, map lookup-then-insert, permission-then-access).
- **Shutdown races**: resources freed while another thread still using them. Common with pools and caches.

## Commit discipline

See [agent-protocols § Commit discipline](${CLAUDE_PLUGIN_ROOT}/docs/agent-protocols.md). Additional concurrency-specific guidance:

- Commit fixes with their tests.
- Commit the concurrency model as code comment or design document if the system warrants it.
