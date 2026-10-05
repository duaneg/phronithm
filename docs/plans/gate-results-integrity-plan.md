# gate-results-integrity — plan

Work unit `gate-results-integrity` of `docs/plans/lsf-improvements-breakdown.md` @ `c901531`. Base commit `c901531`, branch `lsf-workflow`. Phases 1–3 ran compressed in one plan-mode session, so this one document is the problem statement, the design and the implementation plan. Phase 4 step 0 saves it as `docs/plans/gate-results-integrity-plan.md`.

## 1. Problem statement

### Context

Postmortem §D (`docs/design/lsf-postmortem-archipeligo-foundation.md:100-101`) reports two problems. The register (`docs/design/lsf-problem-register.md:65-66`) records them as two rows:

- **P-D2. Review after a fix reads stale gate results.** Every review round is given `impl.result.gates` (`workflows/lsf/implement.js:787-788`). The code-fix prompt tells the stage to run the project's gates at the end (`implement.js:398`), but `FIX_SCHEMA` (`implement.js:187-191`) has no `gates` field, so the fix's run is lost. In release-assert-sweep this cost two fix rounds against the same "CI not observed" Critical.
- **P-D3. A gate that did not run reaches review as though it had.** When a Bash call passes 600 s, the harness moves the command to the background. Seven agents ended their turn waiting and returned with gates not run. Today:
  - a `status: 'not-run'` gate is rendered next to green ones, under "Take that as the test and lint result" (`implement.js:420-424`);
  - an empty `gates` list tells the reviewers to run the gates themselves, which runs into the same limit.

### Unit entry, verbatim

```
### gate-results-integrity
- **Type**: bug-fix
- **Risk**: low
- **Clarity**: exploratory — one open decision: what a stage does when a project gate outlives the tool timeout
- **Iteration**: write-once
- **Effort**: medium
- **Scope**: `workflows/lsf/implement.js`, `workflows/lsf/schema.js`, `workflows/lsf/test/implement.test.js`, `workflows/lsf-implement.js`, `docs/design/lsf-workflow.md::lifecycle-mapping`
- **Publishes**: the fix stage's return shape
- **Depends on**: arch-pin-refresh (logical, serialises the script files)
- **External gates**: `node` (environment, v26.9.0)
- **Done condition**: Tests show (a) a review round after a fix reads the gate results of the fix, not of the implement stage, and (b) an implement or fix return whose gate did not run does not reach review as though it had. The tests pass and the built script is not stale.
- **Description**: Postmortem §D "Gate results after a fix" and "Gates that outlive the tool timeout". The second affected seven agents across code, docs and measurement units.
```

Plan block: `{id: gate-results-integrity, type: bug-fix, dependencies: [arch-pin-refresh], scope: [same as above], publishes: [fix return shape], gates: [{name: node, kind: environment, pin: "v26.9.0"}]}`.

Standing obligations naming the unit:
- **Register dispositions:** the completion report gives a disposition for P-D2 and for P-D3.
- **Design of record:** `lsf-workflow.md::lifecycle-mapping` is amended in the same commit as the behaviour change.
- **Build:** run `node workflows/lsf/build.js` before the tests.

### Success criteria

| # | Criterion | Target rigour |
|---|---|---|
| S1 | Done condition (a): a test shows a review round after a code fix receives the fix stage's gate runs, not the implement stage's. | proven |
| S2 | Done condition (b): tests show that an implement return, and separately a code-fix return, with a `not-run` gate or an empty `gates` list does not reach review. The run stops with `gate-not-run`, and that signal names the gates. | proven |
| S3 | Done condition: `cd workflows/lsf && node --test test/` passes, including `build.test.js` (the staleness check). | proven |
| S4 | The implement and code-fix prompts tell the stage how to run a gate that outlasts the Bash limit without ending its turn. | proven for the prompt text. Whether live agents follow it is `conjectured`; `shakedown-fixture` is the upgrade path. |
| S5 | The completion report's gate runs are those at the unit's final HEAD (the last fix, if there was one). | proven |
| S6 | `lsf-workflow.md::lifecycle-mapping` and the stop-kind list in `skills/lsf-drive/SKILL.md` describe the new behaviour. | evidenced: prose, checked by reading |

### Decisions taken with the requester (2026-10-05)

- **Open decision (unit Clarity).** A gate the stage could not complete is a **structural stop**: a new STOP kind, `gate-not-run`, raised before review. The requester decides what happens next. The prompts also tell the stage how to wait out a long gate.
- **An empty `gates` list counts as did-not-run** and takes the same stop. The instruction telling reviewers to run the gates themselves is removed.

### Candidate approaches (not-run disposition)

1. **Structural stop** (selected). The check is mechanical, it gives the requester the decision, and no reviewer meets the timeout.
2. **Review, flagged "not run".** Rejected. The reviewers would raise a Critical, and the fix stage would hit the same timeout. That is the P-D2 loop.
3. **Prompt guidance only.** Rejected. No script check means done condition (b) cannot be met.

### Exploration findings

- `implement.js`:
  - `critiqueLoop` (`:663-721`) is shared by the plan gate and the review gate. `onFix(fixed)` only updates `branch`.
  - `runUnit` (`:727-810`) runs implement, then the drift check, then the done-condition check, then review.
  - `renderGates` and `gateResults` are at `:419-424`. `renderCompletionReport` (`:467-506`) renders `impl.gates`.
- `schema.js:9-25` holds `STOP_KINDS`. `scheduling.test.js:294` checks that every STOP kind stops a wave, so a new kind is covered automatically.
- Test harness, `test/implement.test.js`:
  - `defaults()` stubs by label prefix. The `fix-code` stub returns no `gates`.
  - The test at `:404-410` pins the reviewer-runs-gates fallback; it will be rewritten.
  - The tests at `:620-638` show the pattern for a stop before review.
- `skills/lsf-drive/SKILL.md:141` lists every STOP kind and how each is answered. It needs `gate-not-run`.
- Harness probe (main session, 2026-10-05): a bounded polling loop `timeout N sh -c 'until [ -f F ]; do sleep 1; done'` is permitted. Workflow agents have not been probed (risk R1).

## 2. Design

### Schema (`implement.js`)

- `FIX_SCHEMA` (plan fix) is unchanged.
- New `FIX_CODE_SCHEMA = stageSchema(obj({ ...COMMIT_FIELDS, ...FIX_FIELDS, gates: GATES_FIELD }))`. `FIX_FIELDS` holds the `applied` and `notApplied` shapes, which `FIX_SCHEMA` now also uses. `GATES_FIELD` is the existing `{ ...arr(GATE_RUN_SCHEMA), description: '…as last run at HEAD' }`, extracted and shared with `IMPLEMENT_SCHEMA`. This is the published "fix stage's return shape".
- **Placement:** `GATE_RUN_SCHEMA` is a `const` at `:193`, after `FIX_SCHEMA`. Move `GATE_RUN_SCHEMA` and `GATES_FIELD` above `FIX_SCHEMA`; otherwise the module hits the temporal dead zone and fails to load.
- `schema.js`: add `'gate-not-run'` to `STOP_KINDS`, with the comment `// implement or code-fix stage: a gate reported not-run, or no gate reported`.

### Detecting a gate that did not run

```js
// The gates a stage did not run: each `not-run` command, or a placeholder when
// it reported none. Empty when every gate has a result.
const unrunGates = (gates) => (gates?.length ? gates.filter((g) => g.status === 'not-run').map((g) => g.command) : ['(no gate reported)']);
const gateNotRunSignal = (ctx, missing) => signal({ ...ctx, kind: 'gate-not-run', source: 'structural',
  statement: `gate(s) not run: ${missing.join(', ')}`, referent: missing.join(',') });
```

- **Implement:** in `runUnit`, after `signalsFromImplement` and `distressSignals` (so a stopped unit keeps its deviation, deferral and discovery records, as the done-condition stop already does) and before the done-condition check. If `unit.type !== 'design'` and `unrunGates(impl.result.gates)` is not empty, push the signal and `abort('implement')`. The not-run check comes before the done-condition check because an unrun gate commonly makes the done condition unevaluable, and one stop names the cause.
  - **Design units are exempt.** Their review (`designCritiquePrompt`) never reads gates, so stopping them on a field nobody reads would be a false stop. Test: a design unit with `gates: []` reaches `critique-design`.
- **Code fix:** `critiqueLoop`'s `onFix(fixed, n)` may now return an array of stop signals; the loop reads it as `onFix(fixed, n) ?? []`, so a caller returning `undefined` is unaffected. When it is not empty, the loop pushes them and returns `{ ok: false, reason: 'fix-stop', rounds: n, findings }`. Design units also go through `fix-code` (`implement.js:794`), so the review gate selects the fix by unit type:
  - Non-design: `FIX_CODE_SCHEMA`, a prompt that asks for `gates`, and the gate check.
  - Design: `FIX_SCHEMA` and today's prompt, unchanged. The plan gate's `onFix` returns `[]`. The review gate's `onFix` updates `branch`. When `unrunGates(fixed.result.gates)` is empty it also sets the current gate runs; otherwise it returns `[gateNotRunSignal(stageCtx('fix'), missing)]`.

### Feeding reviewers the latest runs (P-D2)

- `runUnit` keeps `let gateRun = { by: 'The implement stage', gates: impl.result.gates }`. The review gate's `onFix` replaces it with `{ by: \`The round-${n} fix stage\`, gates: fixed.result.gates }`.
- `gateResults(run)` renders "`${run.by}` ran the project's gates at this branch's HEAD:" followed by the list and the existing "Take that as the test and lint result…" sentence. The empty-list fallback sentence is deleted, because an empty list never reaches review.
- `codeCritiquePrompt` and `lensReviewPrompt` take `gateRun` in place of `gates`.
- `review` gains `gates: gateRun.gates`. `renderCompletionReport` renders `review.gates ?? impl.gates` under "Gates at HEAD". `?? impl.gates` keeps existing direct callers working.

### Prompt text

- **Shared constant `LONG_GATES`**, appended to the implement prompt's gate sentence and to the code-fix prompt:

  > A gate can outlast the Bash tool's time limit, which moves the command to the background; ending your turn while it runs returns this stage with the gate unrun. Run each gate that may take more than a few minutes detached, one at a time, with its own output and status files under the git directory: `D=$(git rev-parse --absolute-git-dir); rm -f "$D"/lsf-gate-<n>.*; (cmd > "$D/lsf-gate-<n>.log" 2>&1; echo $? > "$D/lsf-gate-<n>.rc") &`, where `<n>` is the gate's position. Then wait for it with the Bash tool's timeout set to 600000 ms: `timeout 540 sh -c 'until [ -f "$D/lsf-gate-<n>.rc" ]; do sleep 15; done'`, re-deriving `D` in each call and repeating until the status file exists. Report from that gate's files. Report a gate `not-run`, with why in its summary, only when it could not be completed. The run stops on a gate not run, and the requester decides.

  The git directory is outside the work tree, so `git clean` never removes these files and they are never committed. Per-gate names stop one gate's status file from being read as another's. The wording is fixed in Step 3.
- **Code-fix prompt (non-design):** "…then the project's gates once at your final HEAD — every gate the implement stage ran, not only those the fix touches — reporting each under `gates` with its command, status, and a one-line summary; the next review round reads these, not the implement stage's." The `gates` request lands in Step 2, with the schema that requires it; `LONG_GATES` follows in Step 3.

### Known limits (recorded, not fixed)

- **A fix that returns a green subset of the gates passes the check.** Matching commands across stages by string would false-stop on a reworded command; the prompt asks for the full set. A fabricated or partial return belongs with `mechanical-stage-integrity` (R2).
- **The completion report's verdict and evidence still come from the implement stage.** After a fix, "Gates at HEAD" shows the fix's runs. The report labels the done-condition verdict "(implement stage)" so the two commits aren't conflated.

### Docs

- **`lsf-workflow.md::lifecycle-mapping`:**
  - stage 3: a not-run gate, or none reported, is a STOP (`gate-not-run`); the stage waits out a long gate.
  - stage 4: reviewers read the gate runs of the latest stage at HEAD, implement or the last code fix; a code fix that does not run its gates stops.
  - stage 5: the report's gates are those at the final HEAD.
- **`skills/lsf-drive/SKILL.md::relaunch` (`:141`):** add `gate-not-run`, with a sentence on how it is answered. A relaunch alone re-runs the unit and meets the same gate, and the script reads no decision in `runUnit`. The exits are therefore:
  - (i) the requester changes the environment or project so that the gate completes within a stage, and relaunches with no decision;
  - (ii) the requester completes the unit by hand, and a decision has the re-plan mark it complete.

  "Accept as unrun" has no script mechanism and is not offered. This file is outside the Scope line and in `relay-mitigation`'s section, so it is reported as a write outside Scope and as a discovery for `relay-mitigation` and `drive-mechanics`.

### Risks and unknowns

| # | Risk | Resolution | When |
|---|---|---|---|
| R1 | Whether workflow agents can keep a detached process alive across Bash calls and poll it. Unprobed; the main-session probe is not the same harness. | Probe the detached-run recipe once in the scratchpad (evidenced for the main session). Live workflow-agent behaviour stays `conjectured`, with upgrade path `shakedown-fixture`. Record it as a discovery. | Phase 4 Step 3 |
| R2 | A stage reports `green` for a gate it never ran. | Out of scope: fabricated returns belong to `mechanical-stage-integrity`. No action. | — |
| R3 | `gate-not-run` adds a STOP kind that `lsf-drive` must route. | Step 1 edits `lsf-drive/SKILL.md`; record it as a discovery for `drive-decision-design` and `drive-mechanics`. | Phase 4 |

**API verification.** There are no third-party APIs. All internal shapes were read from source at `c901531`: `critiqueLoop`, `runUnit`, the test harness `defaults()` and `scheduling.test.js:294`.

### Plan critique (critique-design, round 1, sonnet): pass, no Critical

| # | Sev | Finding | Disposition |
|---|---|---|---|
| 1 | Sig | The stop's documented answer cannot be realised; a relaunch meets the same gate | Addressed: the lsf-drive text states exits (i) and (ii) only |
| 2 | Sig | A design unit would be stopped on gates nobody reads | Addressed: design exempt, with a test |
| 3 | Sig | LONG_GATES: shared status files, relative git dir, Bash tool timeout | Addressed: per-gate files, `--absolute-git-dir`, 600000 ms tool timeout |
| 4 | Sig | `GATE_RUN_SCHEMA` temporal dead zone | Addressed: moved above `FIX_SCHEMA` |
| 5 | Min | Step 2 schema requires `gates` before the prompt asks for them | Addressed: the request moves into Step 2 |
| 6 | Min | `onFix` returning `undefined` | Addressed: `?? []` |
| 7 | Min | Check placement vs `signalsFromImplement` | Addressed: after, with a test |
| 8 | Min | A fix returning a gate subset | Known limit; the prompt asks for the full set |
| 9 | Min | Report verdict comes from an earlier HEAD than "Gates at HEAD" | Addressed: verdict labelled "(implement stage)" |
| 10 | Min | lsf-drive edit is in `relay-mitigation`'s section | Addressed: section named; discovery recorded |
| 11 | Min | `FIX_CODE_SCHEMA.gates` has no description | Addressed: shared `GATES_FIELD` |

## 3. Implementation steps

0. Save this document as `docs/plans/gate-results-integrity-plan.md` and commit it. Re-probe the `node` gate (`node --version` against v26.9.0) and record the result.

Every step: `node workflows/lsf/build.js`, then `cd workflows/lsf && node --test test/` green, then commit. The per-step critique gate uses `critique-code`.

1. **Schema and detection.**
   - Changes: `STOP_KINDS += 'gate-not-run'`; `FIX_FIELDS`, `FIX_CODE_SCHEMA`, `unrunGates`, `gateNotRunSignal`; the implement-stage check in `runUnit`.
   - Tests:
     - an implement return with one `not-run` gate stops with `gate-not-run`, naming the command, and no `critique-code` or `lens-review` call is made;
     - an empty `gates` list stops the same way, with the statement "(no gate reported)";
     - a stopped implement keeps its discovery record in the escalation or ledger;
     - a `design` unit with `gates: []` is not stopped and reaches `critique-design`;
     - the old `:404` fallback test is replaced by the empty-list test.
   - Acceptance: S2 for implement.
2. **Fix gates reach review (P-D2) and the fix-side check.**
   - Changes: the `onFix` stop-signal contract in `critiqueLoop` (`?? []`); the review gate's fix is selected by unit type (non-design: `FIX_CODE_SCHEMA` and a prompt asking for every gate under `gates`); `gateRun` tracking; the `gateResults(run)` source label; the completion report renders `review.gates` and labels the verdict "(implement stage)"; the default `fix-code` stub gains `gates`.
   - Tests:
     - round 1 Critical, fix returns `gates` with a distinct summary (`'99 passed (fix)'`); the round-2 `critique-code` and `lens-review` prompts contain the fix's runs and "round-1 fix stage", and do not contain the implement stage's `412 passed`;
     - a fix return with a `not-run` gate stops with `gate-not-run` (stage `fix`) and no round-2 review runs;
     - the completion report after a fix lists the fix's gates;
     - the plan-gate fix path is unchanged (existing tests stay green).
   - Acceptance: S1, S2 for fix, S5.
3. **Long-gate prompt text.**
   - Changes: the `LONG_GATES` constant in the implement and non-design code-fix prompts.
   - Probe R1: run the detached-run and poll recipe once in the scratchpad with a 5 s command and record the result.
   - Tests: the implement and `fix-code` prompts contain the instruction (the git-dir path and "Report a gate `not-run`"); the `fix-plan` prompt does not.
   - Acceptance: S4.
4. **Final verification.**
   - Changes: none. Rebuild; run the full suite; confirm `build.test.js` passes; `git diff c901531 -- workflows/lsf-implement.js` is non-empty and matches the module changes; read lifecycle-mapping end to end for consistency.
   - Acceptance: S3, S6.

**Design of record (S6), per the obligation**, goes in each behaviour commit:
- Step 1: lifecycle-mapping stage 3 (stop on not-run or no gates) and `skills/lsf-drive/SKILL.md:141`, which gains `gate-not-run`.
- Step 2: stages 4–5 (reviewers read the latest stage's runs; a code fix that does not run its gates stops; the report shows the final HEAD's gates).
- Step 3: stage 3's long-gate sentence.

Steps 1→2→3→4 are serial (same files). There is no parallelism.

## 4. Verification

- `cd workflows/lsf && node --test test/`: all pass, including the new tests named in Steps 1–3 and `build.test.js`.
- Done condition (a): the Step 2 test. Done condition (b): the Step 1 and Step 2 stop tests.
- Phase 5: run `phronithm:review` over `c901531..HEAD` with all applicable lenses, plus a full-diff `critique-code` gate in a subagent.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/gate-results-integrity-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit gate-results-integrity of docs/plans/lsf-improvements-breakdown.md (@ c901531).
Unit entry, verbatim (Phase 4 re-verifies the external gates against these
pins before implementing, recording the outcome in the implementation-summary):
- see §1 "Unit entry, verbatim" above. External gate: `node` (environment, pin v26.9.0).
- Standing obligations naming this unit: Register dispositions (P-D2, P-D3); Design of record (`lsf-workflow.md::lifecycle-mapping`, amended in the behaviour commit); Build (run `node workflows/lsf/build.js` before tests).
After Phase 5, produce the completion-report (contents per the
phronithm:feature skill's Produces) at docs/plans/gate-results-integrity-completion.md,
then present this continuation prompt to the requester:
Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
