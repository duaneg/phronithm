# Substrate spike: problem statement

Unit `substrate-spike`, [LSF improvements programme](../plans/lsf-improvements-breakdown.md) @ `e2db84f`. Options record: [lsf-substrate.md](../design/lsf-substrate.md) (S2 selected 2026-10-05).

## Claim

S2 is viable: a Node program on the host runs `run(runtime)` from `workflows/lsf/implement.js` unchanged, with `agent()` as a headless `claude -p` call and the mechanical stages as code. Specifically:

- **C1–C10:** each of `lsf-substrate.md`'s ten spike checks has the stated outcome. Checks 1, 2, 5, 6, 7 and 9 are overturn checks; one failing makes the verdict `overturned`.
- **C11:** one end-to-end path runs on a fixture plan: load, one unit's stages (plan, critique, implement, review, completion report), merge, ledger append.
- **C12:** each postmortem §C fault kind that S2 claims to remove or mitigate, injected at its stage, is prevented or detected. Plus a failed headless call, a permission denial and a kill mid-stage.
- **C13:** an estimate of the host runtime's build.

Load-bearing: C1, C2, C5, C6, C7, C9 (the overturn checks) and C11. C12 is load-bearing for the claims the options record makes per row. C3, C4, C8, C10 and C13 enter the cost table and do not change the verdict.

"Every stage schema in `schema.js`" (check 1) is read as every schema the orchestrator passes to `agent()`. They are built in `implement.js` with `schema.js`'s `stageSchema`.

## Rigour target

`evidenced`: each check is run at least once against Claude Code 2.1.289 and Node v26.9.0, with transcripts kept. Determinism is not claimed. A check run once is recorded as such.

## Known results

- The orchestrator already takes its runtime as `{ agent, parallel, log, args }` (`implement.js:922`). Tests drive it with a stub (`test/implement.test.js`). Each call's `opts` carries `label`, `schema`, `model`, `effort`, `isolation`, `agentType` and `phase`.
- So mechanical stages can become code in the adapter without editing `implement.js`. The adapter dispatches on the label prefix (`load`, `ledger`, `completion-report`, `cleanup`, `decision-log`) and returns a value that fits the stage's schema. This is the spike's first design choice: an adapter-only S2. Whether the build needs orchestrator edits is part of C13.
- `claude -p` at 2.1.289 offers `--json-schema`, `--model`, `--effort`, `--agent`, `--tools`, `--allowedTools`/`--disallowedTools`, `--permission-mode` (incl. `dontAsk`), `--permission-prompts none`, `--plugin-dir`, `--session-id`, `--output-format json|stream-json`, `--max-budget-usd` and `-w/--worktree`.
- Relay (P-F1) is a Workflow-tool frame added at launch ([relay-probe results](relay-probe-results.md)). A headless process has no Workflow launch, so check 3 is expected to pass. That is a hypothesis.
- The exemplar `~/src/maths-proofs/tools/campaign/run.py` supplies the loop shape: caps, a kill file, telemetry per call, and transcripts from `stream-json`. Its resume is iteration numbering.

## Attack ladder

Cheapest-to-falsify first. Each rung lists the checks it can kill.

1. **Single headless probes** (checks 1, 2, 3, 4, 5, 9). One `claude -p` per stage schema with a synthetic prompt. One `--agent phronithm:critic` call. One denied command. Inspect each transcript for the model, effort, relay frame and token floor.
   - *Kill:* a schema-invalid or absent `structured_output`; the critic agent not resolvable headless, or not read-only; model or effort absent from the transcript; a hang or a settings edit on denial.
2. **Adapter skeleton** (checks 6, 7, 8). `workflows/lsf/spike/`: `agent()` over `claude -p` in a driver-made worktree, a prompt-keyed call cache on disk, concurrent `parallel()`, and code handlers for the mechanical labels.
   - *Kill:* a worktree branch that cannot merge; a resume that re-runs a completed call or duplicates a commit; concurrency failures the driver cannot retry.
3. **End-to-end on a fixture** (C11). A throwaway repo with one small feature unit, an architecture document, a brief and declared gate commands, run at the default tiers.
   - *Kill:* the orchestrator cannot complete one wave under the adapter without editing `implement.js`. That would not overturn S2, but it moves C13.
4. **Fault injection** (C12, check 7's kill points, check 10's omitted gate). An adapter fault hook:
   - mutates agent returns: a fabricated SHA, an omitted gate;
   - forces command failures: a rejected commit, a permission denial, a failed headless call;
   - kills the driver mid-stage, and between a code step's commit and its record.
5. **Detachment** (check 10). The driver launched from a headless session that then exits.

Rungs 1 and 2 are independent. Rung 3 needs 2. Rung 4 needs 3. Rung 5 is independent.

## Spike code

The code lives in `workflows/lsf/spike/`. It is throwaway unless the results record adopts it. Fixture repos and run state live in the session scratchpad, outside the checkout.
