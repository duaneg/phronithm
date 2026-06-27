# Investigation Loop

The empirical micro-loop for work whose target is a **finding**, not a known
deliverable — or where a result can **reshape the question** itself. Canonical
statement, referenced (not re-described) by the skills that use it:
[phronithm:diagnose](../skills/diagnose/SKILL.md) (find a fault),
[phronithm:feature](../skills/feature/SKILL.md) (a spike, or Phase 4 reassessment),
[phronithm:investigate](../skills/investigate/SKILL.md) (settle an open
claim),
[phronithm:persona-debug](../skills/persona-debug/SKILL.md) (locate an instruction
defect).

It is distinct from the linear phase pipelines those skills also contain. A
pipeline assumes a fixed target and advances through it; the investigation loop
assumes the target is provisional and may move. When discovery is the product,
"return to an earlier phase" under-describes the work — this loop describes it.

## The loop

1. **Observe before hypothesising.** Gather evidence first — logs, traces, code,
   measurements, literature. Premature hypotheses cause confirmation bias: you
   start fitting evidence to a guess instead of reading it. Record what you
   observe; do not rely on memory.
2. **Generate multiple hypotheses.** Always seek alternatives; never commit to the
   first plausible one. Watch for confounds — more than one cause may be in play.
   For each hypothesis, know what observation would *kill* it.
3. **Order tests by cost-to-falsify, cheapest first** — not by likelihood. A cheap
   test that eliminates a hypothesis narrows the search space regardless of which
   hypothesis turns out correct, so the cheapest decisive test is always the best
   next move.
4. **Run the cheapest unresolved test.** A computation, a construction, a probe, a
   bisection, a literature check, a minimal experiment — whatever resolves the most
   uncertainty for the least cost.
5. **Update belief.** A result can confirm a hypothesis, kill it, or **reshape the
   question** — the most valuable and most easily missed outcome. Update the
   current-state document (below) and **re-rank the remaining tests**: a result
   changes which test is now cheapest-decisive. This re-planning is the loop, not a
   deviation from it.
6. **Verify before committing.** Before you take a step you intend to rely on,
   predict its effect. If the prediction is wrong, your *model* is wrong — update
   the understanding, not the artefact. A fix or claim built on an unverified model
   is a guess wearing a result's clothes.

## Bounded retry

The loop must terminate. Cap the number of distinct hypothesis-families (or
backtrack cycles) attempted without new evidence — typically **≤ 3** — then
**escalate to the requester** with what was tried and what was learned. The
reassessment checkpoint at each iteration is what stops an open-ended grind: a
loop that never re-ranks and never escalates is just grinding on a moving target.

Proceeding despite residual uncertainty is legitimate when the stakes justify it,
the surviving hypothesis is consistent with all evidence, and the next step is
cheap and reversible — but say so explicitly.

## Living current-state document

The loop's memory is a **current-state document**: a snapshot of what is true
*now* — each claim or hypothesis with its present status — not a work-log of how
you got here. History belongs in version control and memory; the document is the
standing answer to "where do things stand?", rewritten as results land. This is
what makes the loop resumable after a break or a context reset.

## Caller specialisations

Each caller instantiates the same loop with a different success condition and
domain machinery:

| Skill | Loop target | Success condition | Domain specialisation |
|---|---|---|---|
| `phronithm:diagnose` | the fault | a verified fix | root-vs-proximate cause; reproduction gate; bug-history risk |
| `phronithm:feature` | the unknown (only when a phase's target is a finding, not the whole pipeline) | the unknown resolved | cost-to-test step ordering (Phase 3); per-step reassessment (Phase 4) |
| `phronithm:investigate` | an open claim | a settled claim at target rigour | attack ladder; rigour ladder; `phronithm:critique` review gate |
| `phronithm:persona-debug` | the instruction defect | the triggering case produces correct behaviour | execution-trace reading; instruction-defect taxonomy |
