# LSF substrate: options

Unit `substrate-options`, [LSF improvements programme](../plans/lsf-improvements-breakdown.md) @ `5d5842f`. It assesses where Phase 3's loop runs, against every [problem register](lsf-problem-register.md) row whose Topic is `mechanical-stage`, `long-running`, `relay` or `driving`, and against the plan's risk R11: a stage can report `green` for a gate it did not run.

**Status:** S2 selected by the requester, 2026-10-05; **confirmed** by `substrate-spike` ([results](../investigations/substrate-spike-results.md)), with three conditions recorded there.

## Problem

The Workflow script has no filesystem, shell, module or clock access ([Constraints](lsf-workflow.md#constraints-from-the-workflow-tool)). So every git or file step is an `agent()` call, and so is every wait.

- **Mechanical stages transcribe through a model.** Archipeligo had at least 14 faults in 218 haiku invocations (postmortem §C). Cartomancer-v2's review found 8 wrong SHAs in 367 returns. In `wf_09b12cc9-894`, 26 of 60 agents ran load, merge, cleanup, ledger or similar stages.
- **What the script knows of a gate is what an agent reports.**
- **Work longer than a stage has no supervisor.**
- **Every launch is exposed to the harness relay** ([relay-probe results](../investigations/relay-probe-results.md)).

At least 6 of archipeligo's 28 endings trace to these classes:

- P-C1: 2 endings;
- P-C4, P-C7, P-A8: 1 each;
- P-F1: run #6.

This record assumes the Workflow tool keeps those limits. If it gains shell or clock access, S1 gets cheaper and S2 loses part of its case.

## Options

**S0, status quo.** The script as at `5d5842f`, with no further change. It already has the long-gate recipe and the `gate-not-run` stop (`gate-results-integrity`).

**S1, harden the script.** The plan's provisional script units as written:

- mechanical agents run committed deterministic tools, and the script compares counts or checksums (improvements §2.1, generalised);
- the script renders the decision log (§2.4);
- a null return is retried (§2.5);
- denials are reported, never worked around (§2.6);
- the relay mitigation: `lsf-drive` launches from a turn a task notification started, and each stage gets a preamble;
- `lsf-drive` mechanics rules (§7.3–§7.7).

**S2, host runtime.** A Node program on the host runs the existing orchestrator, `run(runtime)` in `workflows/lsf/implement.js`.

- **The runtime it supplies:**
  - `agent()` as a headless `claude -p` call, which also needs worktree isolation, agent type, schema, the tier mapping, the per-stage permission configuration and a call cache (see Costs);
  - `parallel()` as concurrent processes;
  - `log()` as a file;
  - `args`.
- **Code replaces the mechanical `agent()` calls:** load, ledger append, decision log, completion-report copy, cleanup, and the project-gate runs after implement, after each fix round and after merge. All 218 of archipeligo's haiku calls were mechanical-tier stages of these kinds.
- **Judgement stays with models:** plan, critique, implement, review, fix, merge-conflict resolution, the wave gate, re-plan and termination.
- **Carried over unchanged:** `graph.js`, `scheduling.js`, `schema.js`, the prompt builders and their tests.
- **Structure follows the exemplar** `~/src/maths-proofs/tools/campaign/`: caps, a kill file, resume from on-disk state, and per-call telemetry and transcripts.

**S3, split per wave.** The Workflow script runs one wave per launch: unit stages, merge, gate and re-plan. Between launches the session does the mechanical work with committed host tools: load from `git show` (passed in `args`), decision log, ledger append, report commit and cleanup. It then relaunches on the run's return notification.

## Assessment

Each cell is one of:

- **removed**: the mechanism that produced the row is gone.
- **mitigated**: still possible, but detected, retried or made rarer.
- **conditional**: removed only once a named further design lands.
- **none**: no effect.
- **neutral**: independent of the substrate; the named unit decides it.

A qualified cell, such as "removed for cleanup; mitigated elsewhere", grades each part separately.

| Row | Cartomancer (Appendix B) | S0 | S1 | S2 | S3 |
|---|---|---|---|---|---|
| P-C1 load placeholder or truncation | 29 of 33 loads byte-identical; 2 altered blocks that parsed (a moved line, rewritten characters) | none: stops `invalid-plan` | mitigated: the returned block is checked against the extractor's counts and retried; counts miss a moved line | removed: code reads `git show` | removed: every load is pre-launch, in code |
| P-C2 ledger rows damaged; cartomancer: own new rows altered or misfiled | 0 deleted lines; 6 of 27 appends altered their new rows; 76 rows misfiled, 12 by the script's own mismatched heading | none | mitigated: the tool refuses deletions and checks a checksum; the agent still runs it | removed, if code renders heading and rows from one source | removed, on the same condition |
| P-C3 copy stage dropped text | not testable | none: report rendered by script, committed by a copy agent | mitigated: checksum | removed | removed: host commits the returned report |
| P-C4 invented `clears`; dropped fence; shortened text | 3 dropped fences, 1 load stopped; D18 shortened | none | mitigated: the invented entry is removed (script renders the log); an agent still writes the file, so fences and text can still be lost unless the committed file is checksummed | removed | removed |
| P-C5 `git branch -d` denied | none | none | mitigated: pre-flight permission check (§7.7) | removed for cleanup (code); mitigated elsewhere: headless agents still run git under permission rules | removed for cleanup; as S0 elsewhere |
| P-C6 agent edits settings after a denial | none | none | mitigated: prompt rule | removed for cleanup; mitigated elsewhere, pending spike check 9 | removed for cleanup; as S0 elsewhere |
| P-C7 transient 529 stops the run | 17 session-limit errors, no 529 | none | mitigated: immediate retry only; the script has no clock, so a backoff is an agent call; a burst can outlast it | removed for transients: the driver sleeps between retries and can wait out a session limit | mitigated, as S1 |
| P-C8 denied commit reported as success; fabricated SHA tails | 3 fabricated SHA tails accepted; 8 wrong SHAs in 367 returns | none: the empty-SHA stop turns the fault into a spurious stop | mitigated: the tool binds SHA and checksum for mechanical stages; agent SHAs elsewhere are unchecked | removed for ledger and reports (code reads `rev-parse`); mitigated for implement and merge SHAs, which the driver checks with `git cat-file` before use | removed for ledger and reports; merge and implement SHAs unchecked |
| P-C9 `--force` allowed, 83 GB deleted | none: both ran undenied | none | mitigated: permission pre-flight; the agent still chooses the command | mitigated: cleanup is code and never `--force`, but `git worktree remove` without it still deletes ignored files (spike run 3); removed only with a cleanup policy for ignored files | mitigated: host tool, as S2 |
| P-A7 campaigns in one single-shot stage | none (no campaign units) | none | none | conditional: a registered job can run as a driver-supervised process; the shape is `long-running-design`'s | none |
| P-A8 work outlives its stage | none | none | none: only an agent can poll | conditional, as P-A7: a job an implement agent backgrounds itself is outside the driver's supervision | none |
| P-A13 orphan process | none | none | none | conditional, as P-A7 | none |
| P-D3 gates past the 600 s tool limit | 20 backgrounded commands, all polled | mitigated: long-gate recipe; `gate-not-run` stop; untested live (risk R12: the harness may kill the detached process) | as S0 | mitigated: an implement or fix agent can still return early on a backgrounded gate; the driver's own run before review supplies the result | mitigated: the implement-stage gate as S0; host re-runs after the launch, one launch late |
| R11 green reported for a gate not run | — | none: agent-attested | none | conditional on declared gate commands: the driver runs them at the branch HEAD and records the exit status. If it only re-runs agent-reported commands, it is mitigated: it catches not-run, not omitted or misreported gates | mitigated: as S2's fallback, one launch late |
| P-F1 relay | 3 of 18 runs | none: every launch in a turn a human message started relays | mitigated: notification-turn launches carry none at 2.1.289; rests on undocumented behaviour (risk R8) | removed if headless agents carry no relay frame (spike check 3); S2 launches no Workflow run | mitigated: per-wave relaunches start in a notification turn; a launch after requester input relays, as S1 |
| P-E1 mechanics share of requester input | about 2/3 of ~53 inputs | none | mitigated: `drive-mechanics` rules | mitigated: launch, pause (kill file), resume (re-run) and status (state file) are single commands; the mechanics remain | none: more launches, automated on notification |
| P-E2 options omit the decisive consequence | one partial omission | neutral (`drive-decision-design`) | neutral | neutral | neutral |
| P-E3 recommendation without its basis | n = 1 | neutral (`drive-decision-design`) | neutral | neutral | neutral |
| P-E4 options inside a failing frame | found, different frame | neutral (`drive-decision-design`) | neutral | neutral | neutral |
| P-E5 pre-clearable gates cost a stop | found | neutral (`drive-decision-design`) | neutral | neutral | neutral |
| P-E6 pasted handoff prompts | found: 0.4–1.6 KB prompts | none | mitigated: §7.4 rule | mitigated: resume is the same command against the state directory | none |
| P-E7 false handoff state | three cases | none | mitigated: verify-before-writing rule | mitigated: the state file is the record the session reads | none |
| P-E8 a decision only in launch args | none | none | mitigated: §7.6 rule | removed: decisions are committed in code before any stage | removed: committed before launch |
| P-E9 misattributed run ending | partial | none | mitigated: §7.5 rule | mitigated: the driver records its stop reason (cap, kill file, signal); a crash leaves none, which itself distinguishes it | none |
| P-I1 out-of-plan commit while live | none | neutral (`drive-mechanics`) | neutral | neutral; the driver commits on the shared checkout under the same pathspec-only rules as today's stages | neutral |

## Costs and what each gives up

| | S0 | S1 | S2 | S3 |
|---|---|---|---|---|
| Build | none | the plan's provisional script units; each adds prompt and tool surface | unsized; see below | host tools, a per-wave return shape, and `lsf-drive` relaunch automation |
| Journal resume | kept | kept | rebuilt: an on-disk cache of agent results keyed by call order and prompt text. Code steps (ledger, commits, decision log) are not cached calls and must be idempotent: each checks its effect is present before acting | kept within a wave; across waves, a relaunch from the plan SHA |
| `/workflows` view | kept | kept | lost; replaced by a log file and a watch command (exemplar `watch.py`) | kept per wave |
| Per-agent floor | paid by every agent, mechanical included: about 61k tokens on haiku (cartomancer, 2026-09-19), so about 13M across archipeligo's 218 haiku calls (indicative; another project's floor) | as S0, plus retries | not paid by mechanical stages; the floor of a headless judgement call is unmeasured (spike check 4) | not paid by mechanical stages except merge |
| New dependencies | none | none | Node on the requester's host at run time, new for plugin users (`tools/` is Python stdlib by convention); the project's toolchain environment for gate runs; headless permission configuration per stage | none |
| Plan changes | none | none | gate commands declared in the plan or project configuration, if R11 is to be removed: a plan-schema and `lsf-workflow.md` change. Design and docs units declare none, as `gate-not-run` already exempts design units | none |
| Later units | as planned | as planned | `mechanical-stage-integrity` and `stage-failure-handling` become driver code, keeping their SHA checks; `relay-mitigation` reduces to spike check 3's evidence; `long-running-design` designs against a supervisor; `drive-mechanics` targets a CLI | rewrites `drive-mechanics` and most of `mechanical-stage-integrity`; long-running unchanged |

**S2's build.** The orchestrator's loop, scheduling and prompts carry over. The adapter is new. It needs:

- headless calls with schema and tier mapping;
- worktree isolation: a worktree per call, on a branch from HEAD, reused on resume, as [Constraints](lsf-workflow.md#constraints-from-the-workflow-tool) records for the Workflow tool;
- agent types;
- a prompt-keyed call cache;
- idempotent code steps;
- concurrency under session limits;
- per-stage permissions.

The exemplar provides only the loop shape, caps and telemetry. Its resume is iteration numbering, not a call cache; it runs no parallel worktrees; and its broad permissions are safe only inside the `ic` isolation container. Risk R1 says a host driver is "too large for this plan" and would open a successor programme. That stands until the spike sizes the adapter.

## Generality

The standing obligation asks, per row S2 changes, whether the problem holds outside archipeligo.

- **Holds elsewhere.**
  - P-C4 (three dropped fences; one load stopped).
  - P-C2's adjacent mechanism (6 of 27 appends altered their own rows; 76 rows misfiled).
  - P-C8 (three fabricated SHA tails; 8 wrong SHAs in 367 returns; context-free in the script).
  - P-C7 is context-free (`implement.js:641`, `:669`).
  - P-F1 (harness-imposed; 3 of 18 cartomancer runs).
  - P-E1, P-E6, P-E7 recur; P-E9 recurs only partially.
- **Harness-imposed, but no cartomancer fault.** P-D3: 20 backgrounded commands, all polled.
- **Not shown elsewhere.**
  - P-C1: cartomancer's two altered blocks parsed, the adjacent case.
  - P-C5, P-C6, P-C9: cartomancer's 36 `branch -d` calls and its `--force` removals were all undenied.
  - P-E8.
  - P-C3 is not testable.

  S2 removes these as a side effect of removing their class. It is not adopted on their account.
- **Long-running rows (P-A7, P-A8, P-A13): archipeligo only, not testable.** S2's effect on them is conditional (above), and is claimed only for programmes with campaign units.

S2 is adopted generally because the substrate serves every programme. The case for it rests on the rows that hold elsewhere.

## Selection

**S2 is selected for the spike.**

- **What decides it.** It is the only option that removes mechanical-stage transcription faults rather than detecting them, for every mechanical stage. It is the only one that can close R11, given declared gate commands. It is also the only one under which long-running work can be supervised at all, though that waits on `long-running-design`.
- **Relay.** Its P-F1 effect is likely, but unconfirmed until spike check 3.
- **Cost.** S2's build is unsized and may be a programme of its own (R1). The spike measures it.
- **S1, the fallback.** Its mechanical stages stay agent-attested, R11 stays open, and long-running work has no route.
- **S3.** It removes most transcription faults. But it leaves R11 one launch late, merge and implement SHAs unchecked, and long-running work unaddressed, and it adds a launch per wave.

**Spike checks.** Each check names its outcome. A check marked *overturn* returns the decision to `substrate-options`, with S1 as the default. A check marked *cost* is recorded in the cost table and does not overturn.

1. *Overturn.* `claude -p --json-schema` returns schema-valid output for every stage schema in `schema.js`, on at least one call each, with no hand repair.
2. *Overturn.* A critique call runs the critique skill and its type appendix with a read-only toolset: `--agent phronithm:critic`, or `--tools` restricted with the plugin loaded.
3. *Cost.* A headless agent's transcript carries no `[Workflow harness — user request]` frame or other relayed message. A relay leaves P-F1 as S1 grades it, and the rest of S2 stands.
4. *Cost.* The floor of one headless call, measured at the same model as a Workflow agent's. The cost table records the difference.
5. *Overturn.* Each call's transcript records the requested model and effort.
6. *Overturn.* A unit's stages run in a driver-created worktree on its own branch from HEAD, and the branch merges.
7. *Overturn.* After a kill mid-stage and again after a kill between a code step's commit and its record, a resume re-runs no completed call and duplicates no ledger row or commit.
8. *Cost.* At least three calls run concurrently without a failure the driver cannot retry.
9. *Overturn.* A headless agent denied a command returns with the denial reported. It does not hang, and it does not edit settings.
10. *Cost.* Two further outcomes are recorded:
    - whether the driver outlives the session that launched it. If it does not, it runs from the requester's terminal and lsf-drive reads its state file.
    - with an injected omitted gate, whether declared gate commands catch it.

## Interim policy

The Workflow script stays the supported Phase 3 path until the spike confirms S2.

- No further work goes into its mechanical stages. `mechanical-stage-integrity` and `stage-failure-handling` wait on the spike.
- A fix to judgement-stage behaviour lands in `implement.js` and carries over to S2 unchanged.
- `lsf-drive` relaunches as the first tool call of the turn the run's return notification starts, which carries no relay at 2.1.289, only where the requester has nothing to add (a `wave-cap` return).
- A relaunch after a stop follows the requester's decision, a human message, and relays. That stays `relay-mitigation`'s.

## Requester verdict

2026-10-05: S2 selected for `substrate-spike`. The critique gate passed with no Critical findings (`phronithm:critic`, critique-design, sonnet, round 1), and its Significant findings were applied before the verdict.
