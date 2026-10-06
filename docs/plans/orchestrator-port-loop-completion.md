# orchestrator-port-loop — completion report

Work unit `orchestrator-port-loop` of `docs/plans/lsf-improvements-breakdown.md` (entry at `cfaf279`). Branch `lsf-workflow`, commits `74ef669..`. Related documents: plan `docs/plans/orchestrator-port-loop-plan.md`, implementation summary `docs/plans/orchestrator-port-loop-implementation.md`.

## Done condition: met

Each clause is established at the rigour below.

- **`implement.js` is ported, with every test.** The port is in `lsf-runtime/orchestrator`. All 83 tests in `implement.test.js` are ported and pass (`proven`). Each Go test carries its JS title in a `// js:` comment, and a one-off check matched all 83. One test's assertion changes with D9: the `{1: 42}` case now stops at the merge.
- **Each prompt builder renders byte-identical text.** `go run ./tools/promptparity` (also `go test`) runs 55 scenarios through both orchestrators. Every prompt, option, schema, log and return is identical, byte for byte, as is each lane's call order and position among the barrier calls (`proven`).
  - All 68 named functions in `implement.js` run.
  - The 17 uncovered blocks are a reviewed allowlist, enforced both ways, each with why `run()` cannot reach it (`evidenced`).
  - Mutated builders, log text and call order all fail the check (recorded, not committed).
- **The spike fixture's end-to-end path completes** on the ported orchestrator and the runtime (`evidenced`: `go run ./tools/fixturerun`, 2026-10-06).
  - Status `complete`: 16 calls, $3.41, 5 min.
  - Every transcript records `claude_code_version` 2.1.289.
  - The fixture's `main` has the `add-mean` merge, `mean` behaves as its done condition says, and both gates pass.
  - Every stage was a model call, where in run 2 five of them were code.
- **Every call label is unique within a launch.**
  - The `orchtest` stub fails any test that repeats a label, so all tests and parity scenarios check it.
  - `TestOnRuntimeLabelsUniqueAndReplay` runs a two-wave launch ending in a stop whose scribe is `ledger:w2#2`, on the real `lsfrt.Run`. It logs no `label-reused` and records one journal record per label. A relaunch replays every call (`proven`).
- **A stage result that fails to decode is that stage's `agent-null`.** This includes a wrong-typed field that JS would coerce. `TestDecodeFailureIsAgentNull` shows it. The divergence is recorded as D9 in `orchestrator/doc.go`, which `plan/doc.go` points to (`proven`).

## Deviations from the unit entry

- **Scope.**
  - `plan/doc.go` gains one pointer line to the orchestrator's register.
  - `CLAUDE.md`'s `lsf-runtime` entry was planned, and was also updated.
- **Publishes, beyond `run(runtime)` in Go:**
  - `orchestrator.Body`, the `lsfrt.Run` body;
  - `orchestrator/orchtest`, the stub-runtime harness;
  - `tools/promptparity`;
  - `tools/fixturerun`, a paid end-to-end launcher that `shakedown-fixture` can reuse.
- **D16 and D17** fix two JS defects in Go rather than porting them, following `orchestrator-port-plan`'s fix-and-record decision. That decision was the requester's for the plan package, and I applied it here. **Requester to confirm.**
  - D16: `maxUnitsPerWave` below 1 is an args error. JS runs nothing and reports `complete`.
  - D17: a completion report without a branch or commit aborts the unit. JS drops the unit from the merge silently and re-runs it every wave.

## Deferrals

None. Nothing in Scope is left undone.

## Discoveries for sibling and downstream units

**`code-stage-interface`**
- **Load and merge work as model calls today.** The end-to-end run's load, merge, ledger and completion-report stages ran as model calls (haiku and opus), and the run completed. The interface replaces stages that work, not stages that fail.
- **Cleanup removes no runtime worktree.** Its prompt names `worktree-wf_*` branches (removed 0, as the entry predicted).
- **`orchestrator.Runtime` is the seam to extend.** It is the interface (`Agent`, `Parallel`, `Log`, `Args`) the code stages hook into. The prompt builders for the six stages are `copyPrompt`, `decisionLogPrompt`, `ledgerPrompt`, `completionScribePrompt`, `mergePrompt` and `cleanupPrompt`. promptparity covers each, so retiring a prompt means retiring its scenarios' JS comparison for that label.
- **Unit-stage worktree branches carry a hash.** A unit stage's worktree branch is `lsf/<plan8>/<journal key>`, where the key includes a hash suffix (for example `completion-report_A-1a2b3c4d`). The merge stage reports whatever branch the agent names.

**`runtime-signals`**
- `CallResult` (flags, denials) is not visible to the orchestrator. `Runtime.Agent` returns only the value. Driver events as ledger rows need either `Call` in the interface or a side channel.
- `#n` labels (D10) appear in telemetry and the journal. A role or skill rule anchored with `$` would not match them.

**`runtime-cli-drive`**
- `cli/` is still empty. `tools/fixturerun` shows the launch sequence the CLI will need:
  - config from a file;
  - args;
  - `DISABLE_AUTOUPDATER=1` with a versioned `claudeBin` and a version check before spending;
  - a fresh state dir;
  - `lsfrt.Run(ctx, cfg, orchestrator.Body)`.

**`shakedown-fixture`**
- `tools/fixturerun` is a starting point. Its caps (40 calls, $25, 1 h) suit one unit; the run used 16 calls and $3.41.

**`plan-preflight-check`**
- No change. The orchestrator uses `plan.LoadGraph` and `plan.ValidatePlan` as the load stage does.

**`script-retirement`**
- When the JS is deleted, delete `tools/promptparity` and its node dependency. `orchestrator/doc.go`'s register (D9–D17) then becomes the record of where Go departs from the retired reference.
- The 83 `// js:` titles cite the deleted tests.

**Frozen-JS defects found.** These are recorded, not fixed, under the Frozen JavaScript rule:
- `maxUnitsPerWave: 0` (D16).
- The silent drop of a branchless completion report (D17).
- JS throws on a null or missing `directEdits`, `amendments` or `appendedUnits` (D14).

## Standing obligations

- **Headless pin.** Discharged. The gate was probed at Phase 4 entry, and the end-to-end run's 16 transcripts record `claude_code_version` 2.1.289. The run called `~/.local/share/claude/versions/2.1.289` with `DISABLE_AUTOUPDATER=1`.
- **Register dispositions.** This unit disposes of no register row; none is assigned to it.
- **Frozen JavaScript.** Discharged: `git diff --stat cfaf279 -- workflows/` is empty.
- **`lsf-runtime/go.mod` stdlib-only.** Discharged: unchanged.
- **Design of record.** Does not name this unit.

## External gates

- **`lsf-toolchain` (Go 1.27.1).** Confirmed at Phase 4 entry and unchanged at completion (`go1.27.1-X:nodwarf5`; the suffix is a local `GOEXPERIMENT`).
- **`claude-code-headless` (2.1.289).** Confirmed: the versioned binary reports `2.1.289 (Claude Code)`, and the transcripts show the same. The installed default is newer and was not used.

## Execution signals

- **Context.** Comfortable at completion.
- **Size.** About 7,900 lines added: about 2,600 orchestrator code, 2,800 tests, 500 in the `orchtest` harness, 1,300 in the two tools (tests included), and the rest plan documents. The unit's flag held: the JS was the reference, so the size cost wall clock, not context.
- **Design decisions taken mid-implementation, rather than at the design pass.** There were six:
  - the ordered-object schema builder (byte-identical schemas);
  - the mechanical template conversion;
  - D14 and D15 (found by parity);
  - D16 and D17 (found by review);
  - the unreachable-block allowlist;
  - the barrier-position comparison (from per-step critiques).
- **Review.** No Critical findings at any gate. Significant findings: 5 at the plan gate and 9 over the per-step gates. Phase 5 found 2 (D16 and D17). All were fixed or are recorded in the implementation summary.

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
