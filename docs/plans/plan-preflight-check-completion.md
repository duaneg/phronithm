# plan-preflight-check — completion report

- **Work unit:** `plan-preflight-check` of `docs/plans/lsf-improvements-breakdown.md` @ `2c98baf`.
- **Branch:** `lsf-workflow`.
- **Plan:** `docs/plans/plan-preflight-check-plan.md`.
- **Implementation summary:** `docs/plans/plan-preflight-check-implementation.md`.

## Done-condition verdict

**Met (proven), with the execution-order deviation agreed at plan time.**

- **Runnable outside a workflow; non-zero, printing the load stage's problems, on each fixture:** `lsf-validate [-decisions PATH] PLAN` (`lsf-runtime/validate/cmd/lsf-validate`). `TestLoadStageFixtures` drives it on fixtures for an unserialised overlap, a cycle, a bad `clears` entry, a malformed `lsf-gates` block and a block that disagrees with its entries. Each test asserts the exact output and exit 1. `TestAgreementDisagreements` covers every agreement aspect with one mutation each and an exact problem list.
- **Zero on this plan:** `TestLivePlan` and `TestAgreementLivePlan` (skipped only if the file is absent), plus a manual `go -C lsf-runtime run ./validate/cmd/lsf-validate "$PWD/docs/plans/lsf-improvements-breakdown.md"`, which exits 0.
- **Agreement covers what the quality check names:**
  - every unit (both directions, and duplicate headings);
  - type;
  - Depends-on units;
  - Scope paths, including the `::symbol` part and with a trailing `/` normalised;
  - gates by name, kind and pin;
  - the execution order of incomplete units.
  A completed unit's stub is checked against `complete: true` only.
- **Named where a plan is committed and where one is launched:** `phronithm:breakdown` Phase 2 step 4 (discharged before step 7's commit and on every re-plan); `phronithm:lsf-drive` Startup step 2.
- **The tests pass:** `cd lsf-runtime && go test -race ./...` and `go vet ./...` are clean.

Phase 5 ran a lens review and a full-diff critique. Neither found anything Critical. Fixed from the review:
- S1: the plan SHA lookup had been dropped from lsf-drive Startup and is restored.
- S2: the plan and log must now be committed before the working tree is validated.
- Critique F1: a command failure with Go present goes to the requester.
- Several Minor parsing leniencies.

The remaining Minor findings are listed under Known limitations.

## Deviations from the unit entry

- **Execution-order check covers incomplete units only** (requester decision 1, 2026-10-08). The quality check's wording is "one id per unit". The check instead compares the first backticked or bold mention of each incomplete unit, in written order, with the block's order restricted to incomplete units. `phronithm:breakdown`'s bullet now states this rule.
- **Scope:** within Scope. The `lsf-runtime/validate` package also holds `cmd/lsf-validate` (requester decision 2).
- **Publishes:** the command, as entered. The library API is published too:
  - `validate.Check(plan, decisions)`;
  - `validate.Agreement(markdown, *plan.Plan)`;
  - the problem kind `plan-disagreement`. It is command-only, not a stop kind, and never reaches the orchestrator.

## Deferrals

None.

## Discoveries affecting other units

- **`runtime-distribution`:**
  - The launcher should ship and run `lsf-validate`, as its done condition already says ("the plan-validation command through it").
  - Until then both skills call `go -C ${CLAUDE_PLUGIN_ROOT}/lsf-runtime run ./validate/cmd/lsf-validate`, which needs a Go toolchain and compiles on each call.
  - `go run` reports every non-zero exit as 1 (checked). The skill texts therefore tell problems (lines on stdout) from a command failure (empty stdout). A shipped binary restores distinct exit codes 1 and 2, and the texts can then key on those.
- **`runtime-cli-drive`:**
  - May expose `validate.Check` as a CLI subcommand.
  - This unit rewrote `skills/lsf-drive/SKILL.md::startup` steps 2–3. That unit's Scope also holds this section, and its edge on this unit already serialises it.
- **`programme-close`:** owns listing the command and `lsf-runtime/validate` in `CLAUDE.md`, which this unit left unchanged as its entry requires.
- **Option for re-plan (undecided):** the orchestrator's load stage could also run `validate.Agreement` and stop on disagreement. That is a behaviour change, so candidate B was rejected for this unit.
- **`phronithm:breakdown` rule gap (outside this programme):**
  - archipeligo-evolved's `foundation-breakdown.md` fails the check on one problem: a stub `weighted-contest-rung` with no block unit.
  - Breakdown's `unit-removed` rule keeps completed units' stubs and deletes removed incomplete units outright, so this stub is non-conformant.
  - Whether a retired unit may keep a stub is undecided. Today the check rejects it.

## Standing obligations

- **Register dispositions:** **P-D4 ("Plan checks only at load") — `fixed`.**
  - The load stage's checks now run before launch, at the breakdown commit and at the lsf-drive launch.
  - The block-against-entries comparison exists for the first time.
  - Residual risk: when Go is not installed, both skills fall back to the manual checks.

## External-gate re-verification

`lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`):
- `go version` reports go1.27.1, matching `toolchain go1.27.1`.
- Probed at plan time and again at Phase 4 entry. No drift.

## Known limitations

These are recorded, not fixed. Each one fails in the false-positive direction unless it says otherwise.

- **Pin containment uses substrings** (requester decision 3).
  - A pin token `1.2` is found inside `1.27`, so some drift can pass (false negative).
  - Token-equality would break pins written as part of a path, such as `archipeligo-evolved` against `~/src/archipeligo-evolved`.
- **Pins are compared in one direction.** An entry pin with no pin in the block is not reported.
- **Any mention counts as a unit's position.** The first backticked or bold mention of a unit anywhere in the Execution Order section sets it, rationale prose included. A parallelism note that names a later unit early makes the check report an ordering problem that isn't there; `TestAgreementDisagreements/execution_order_rationale_mention` pins this.
- **Field grammar is strict.** The parser takes only `- **Name**: value`. An entry in another syntax is reported as such, not compared.
- **Unhandled CLI edge cases:**
  - `-decisions ""` is treated as absent.
  - `-h` exits 2.
- **The blank-line lookahead is quadratic** in a run of blank lines. This is negligible on real plans.

## Execution signals

- **Context at completion:** comfortable.
- **Design decisions taken mid-implementation:**
  - A repeated field or Scope value is reported.
  - A unit never mentioned in Execution Order gives one omission problem, not also a divergence.
  - Pin tokens have list punctuation trimmed, and containment is tested against the pin text after the kind.
  - Heading ids drop backticks and a trailing colon.
  - `none` takes a parenthesised reason and a full stop.
  - Fences follow the CommonMark run-length and info-string rules.
  - Skill-text paths use `git rev-parse --show-toplevel`, not `$PWD`.

  None changed the design's approach.
- **Size:** about 2,100 lines over 19 files, including the plan documents; most of the code is tests and fixtures.

## Open questions for the requester

1. Should an entry pin with no pin in the block be a disagreement? Today it is not reported.
2. May a retired unit keep a stub? The archipeligo case above depends on the answer.

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
