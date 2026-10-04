# Completion report: cleanup-data-safety

Unit `cleanup-data-safety` of [lsf-improvements-breakdown](lsf-improvements-breakdown.md). Base commit `463cd93`, branch `lsf-workflow`.

## Done-condition verdict

**Not attempted.** At Phase 1 scope confirmation (2026-10-04) the requester disputed the unit's premise and returned the unit for re-plan. Nothing was implemented, and no rigour level was achieved.

## Deviations

None. No files in the unit's Scope were changed.

## Deferrals

None.

## Discoveries

1. **The requester disputes the premise.** Quoted: "was that 80gb *really* that valuable? … the problem is not that we lost 80gb of data, and more that it is indirectly characterised as valuable. Is that data worth 10% of total system capacity? This is not observational data that cannot be replicated."
   - Evidence that bears on this:
     - The postmortem gives the regeneration cost as about 8 h (§D "Retained data").
     - The same bullet records three archipeligo records citing an unmerged worktree path "that must be kept indefinitely".
   - The unit's Risk, Description and done condition all frame the loss as destroyed data.
2. **Storage outside the worktree is not generally available (requester).** Some substrates do not offer it; one is the maths-proofs campaign exemplar (`~/src/maths-proofs/tools/campaign/`). This bears on the `Retains:` destination in improvements §6.5 and on `substrate-options`.
3. **A plain `git worktree remove` deletes ignored files silently.** Verified on git 2.54.0 in a scratch repo:
   - Without `--force`, untracked files block removal (rc 128).
   - An ignored `runs/` directory is removed with rc 0 and no warning.
   - So dropping `--force` alone protects nothing that is gitignored.
   - `git status --porcelain --ignored` reports both kinds, collapsed per directory (`?? target/`, `!! runs/`).
4. **The reused-worktree reset deletes untracked content silently.** It runs `git clean -fd` at `workflows/lsf/implement.js:294`. This is the same class of loss as cleanup; the plan critique raised it.
5. **Options considered before the re-plan decision.** These are for the re-plan's reference; none was adopted:
   - Keep any merged worktree whose `git status --porcelain --ignored` is non-empty.
   - The same, plus a `disposablePaths` launch arg for regenerable caches (`target/`, `node_modules/`).
   - A size threshold (improvements §6.5).
   - Move the data out, then remove.
   - A deterministic committed cleanup tool (`substrate-options` territory).
   - Name, then remove: record in the ledger and the digest what each removal deletes.

   The plan critique (`phronithm:critic`, sonnet, round 1) rated keep-with-filter Critical:
   - Its safety rests on a haiku agent parsing porcelain output, prefix-matching it, and running `--force`.
   - The stub runtime cannot test that.
   - Malformed `disposablePaths` entries (`""`, prefixes without a path boundary) would themselves delete data.

   With name-then-remove, the same agent error loses only a name.
6. **Candidate for `empirical-unit-design`.** Records would cite a regeneration recipe (commit plus command) rather than a path in a checkout. This is untested, and the requester has not endorsed it.

## Standing obligations

- **Design of record:** not triggered; no behaviour changed.
- **Register dispositions:** not binding; this unit precedes `problem-register`. For the register: the unit addresses postmortem §D "Retained data".

## External gates

`node`: probed `v26.9.0` at planning, which matches the pin. Phase 4 was not entered, so there was no implementation-time re-probe.

## Execution signals

- Context was comfortable at completion.
- The unit's one open design decision was reversed at Phase 1 by the requester, not at a design pass.

---

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
