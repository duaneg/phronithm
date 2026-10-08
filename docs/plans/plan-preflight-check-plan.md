# plan-preflight-check — problem statement, design and plan

Compressed mode: phronithm:feature Phases 1–3 in one plan-mode session. On approval this file is saved to `docs/plans/plan-preflight-check-plan.md` and committed before step 1.

## Context

Postmortem §D, "Plan checks only at load" (register row P-D4): two archipeligo plans fresh from `phronithm:breakdown` stopped at the workflow's load stage, after launch. `plan.LoadGraph` + `plan.ValidatePlan` (the Go port) already catch overlap, cycle and `clears` defects, but only inside a run, and nothing anywhere compares the `## Graph` block with the prose entries it is meant to mirror (breakdown's quality check asks for "a short script" each time). This unit adds a command runnable before launch that does both, and points `phronithm:breakdown` (at commit) and `phronithm:lsf-drive` (at launch) at it.

Base commit `2c98baf`, branch `lsf-workflow`. External gate `lsf-toolchain`: `go version` → go1.27.1, matching `lsf-runtime/go.mod` (probed at startup; re-probed at Phase 4 entry).

## Unit entry (verbatim, from `docs/plans/lsf-improvements-breakdown.md` @ `2c98baf`)

### plan-preflight-check
- **Type**: feature
- **Risk**: low — `plan.LoadGraph` and `plan.ValidatePlan` already catch the overlap, cycle and `clears` cases; the block-against-entries comparison is new code with no reference implementation, specified by `phronithm:breakdown`'s quality check
- **Clarity**: specified
- **Iteration**: write-once
- **Effort**: medium
- **Scope**: `lsf-runtime/validate`, `skills/breakdown/SKILL.md::quality-check`, `skills/lsf-drive/SKILL.md::startup`
- **Publishes**: a plan-validation command runnable outside a workflow
- **Depends on**: orchestrator-port-plan (logical, `plan.LoadGraph` and `plan.ValidatePlan`)
- **External gates**: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`)
- **Done condition**: A command runnable outside a workflow exits non-zero, printing the load stage's problems, on fixtures for an unserialised overlap, a cycle, a bad `clears` entry, a malformed `lsf-gates` block and a plan block that disagrees with its entries, and exits zero on this plan. The agreement check covers what the quality check names: every unit, its type, its Depends-on units, its Scope paths, its gates with kind and pin, and the execution order; a completed unit's one-line stub is checked against the block's `complete: true` only. `phronithm:breakdown` and `phronithm:lsf-drive` name it where a plan is committed and where one is launched. The tests pass.
- **Description**: Postmortem §D "Plan checks only at load": two plans fresh from breakdown stopped at load. The check is independent of the substrate decision, since both a script and a driver need it before launch. Neither `graph.js` nor its port compares the block with the entries; that check is this unit's. `CLAUDE.md` is outside its Scope: `orchestrator-port-loop`, which may run beside it, owns the `lsf-runtime` entry, and `programme-close` lists the command. **Flag:** three surfaces written; two are a few lines of skill text, so the excess is not context.

Standing obligations naming this unit: **Register dispositions** (P-D4). Not carried: Generality stated (not a design unit), Headless pin, Design of record (no `lsf-workflow.md::`/`lsf-runtime.md::` scope).

## Requester decisions (2026-10-08, this session)

1. Execution order: check **incomplete units only** — the first mention of each incomplete unit in the Execution Order section, in written order, equals the block's `execution_order` restricted to incomplete units, and every incomplete unit is mentioned. Completed units' positions are not checked. (Deviation in wording from breakdown's "one id per unit"; recorded for the completion report.)
2. Command shape: **library package `lsf-runtime/validate` plus a thin main `lsf-runtime/validate/cmd/lsf-validate`**.
3. Gate pins: **token containment** — kind matches exactly; when the block gate has a pin, every whitespace token of the pin containing a letter or digit occurs in the entry's gate text (backticks removed).

## Success criteria

| # | Criterion | Target rigour |
|---|---|---|
| S1 | The command exits non-zero and prints the load stage's problems on fixtures for: an unserialised overlap, a cycle, a bad `clears` entry, a malformed `lsf-gates` block, and a block disagreeing with its entries (one fixture per agreement aspect: unit set, type, Depends-on, Scope, gate name/kind/pin, execution order, stub vs `complete`) | proven (Go tests driving the library and the main) |
| S2 | It exits zero on `docs/plans/lsf-improvements-breakdown.md` | proven (test reading the repo plan, skipped only if absent) + evidenced (manual run) |
| S3 | `phronithm:breakdown`'s quality check and `phronithm:lsf-drive`'s Startup name the command | evidenced (diff read; skills are prose) |
| S4 | `cd lsf-runtime && go test -race ./...` and `go vet ./...` pass | proven |
| S5 | P-D4 disposition (`fixed`) recorded in the completion report | evidenced |

Prototype evidence (Python, this session, stdout only): with these rules this plan has exactly one disagreement, `script-retirement`'s entry `workflows/` against block `workflows` — handled by normalising a trailing `/`. archipeligo's `foundation-breakdown.md` gives one (a retired stub, `weighted-contest-rung`, with no block unit — a genuine non-conformance with breakdown's current `unit-removed` rule). cartomancer-v2's plan predates the literal `file::symbol` Scope form and prose gate syntax, and fails widely; it is not a target.

## Design

### Candidates considered

- **A. Agreement check in package `validate`, reusing `plan` for everything else (chosen).** `plan` stays the port's surface (scope of other units); the new Markdown-entry parser is this unit's alone.
- **B. Add the agreement check to `plan` and call it from the orchestrator's load.** Rejected: `plan`/`orchestrator` are outside this unit's scope and serialised to other units; making disagreement a load stop is a behaviour change the plan has not decided. Recorded as an option for re-plan.
- **C. Python script under `tools/`.** Rejected: the unit entry scopes `lsf-runtime/validate`, and the load half must be the Go loader itself, not a reimplementation.

### Package `validate` (`lsf-runtime/validate/validate.go`, `entries.go`)

```go
// Check runs the load stage's checks and the entry agreement check on a
// plan document. decisions is nil when there is no decision log.
func Check(planMarkdown string, decisions *string) []plan.Problem
```

Order, mirroring `orchestrator/loop.go` load (lines ~351–377):
1. `g := plan.LoadGraph(planMarkdown, decisions)` → `g.Problems`.
2. `_, ps := plan.ParseDeclaration(planMarkdown)` → `ps`.
3. If `g.Plan != nil`: `plan.ValidatePlan(g.Plan)`, then `Agreement(planMarkdown, g.Plan)`.

```go
// Agreement compares the plan document's Work Units entries and Execution
// Order section with its parsed block.
func Agreement(markdown string, p *plan.Plan) []plan.Problem
```

Problems from `Agreement` have `Kind: "plan-disagreement"` (a command-only kind; not a stop kind, never fed to the orchestrator), statement prefixed `entries: `, referent the unit id (or `unit/gate`, or `execution-order`).

**Document parsing** (CRLF tolerated: normalise `\r\n`→`\n` first; fenced code blocks are skipped when locating `##`/`###` headings so the `lsf-plan` block and mermaid blocks cannot be mistaken for headings):
- *Work Units section*: from a line `## Work Units` to the next `## ` heading. Missing → one problem, no further entry checks.
- *Entries*: each `### ` heading; id = first whitespace-delimited token of the heading text. Fields: lines `- **<Name>**: <value>`, value continuing over following lines until the next `- **` field line, a blank line followed by non-indented text, or the next heading; whitespace collapsed. An entry with no field lines is a **stub**.
- *Execution Order section*: `## Execution Order` to the next `## `. Missing → one problem.

**Checks** (`b` = block unit, `e` = entry):
1. Unit set: every block unit has an entry; every entry id is a block unit. Duplicate entry headings → problem.
2. Stub: `b.Complete` must be true; nothing else compared. Full entry: fields compared whatever `b.Complete` is (a full entry for a complete unit is allowed).
3. Required fields on a full entry: Type, Scope, Depends on, External gates — each missing one is a problem.
4. Type: trimmed value equals `b.Type`.
5. Depends on: `none` → empty set. Otherwise split on top-level `;` (paren-depth aware — artefact text inside parentheses can contain `;`), remove parenthesised text, split on `,`, trim, strip backticks. Set equality with `b.Dependencies`; report missing and extra names separately.
6. Scope: backticked tokens outside parentheses; non-backticked residue outside parentheses other than `,`, `and` and whitespace is a problem (catches an unbackticked path). Normalise each path (part before `::`) by stripping one trailing `/`; same on block scope entries. Set equality.
7. External gates: `none` → empty. Otherwise top-level `;` segments, each matching `` `name` `` optionally followed by `(kind[, pin text])`; an unparseable segment is a problem. Name sets equal. Per shared name: kind equal (`b.Gates[i].Kind`); if block `Pin != nil` and non-empty, every whitespace token of the pin with a letter or digit occurs in the pin text with backticks removed.
8. Execution order (decision 1): scan the section for `` `x` `` and `**x**` tokens in order; keep first mentions of ids naming incomplete block units. That list must equal `p.ExecutionOrder` filtered to incomplete units; report the first divergence (index, both ids) and any incomplete unit never mentioned.

Sets are compared via sorted slices so output is deterministic; problems are emitted in block unit order, then execution-order problems.

### Command `lsf-runtime/validate/cmd/lsf-validate/main.go`

```
lsf-validate [-decisions PATH] PLAN
```
- Reads `PLAN`. Decision log: `-decisions` if given; otherwise, when `PLAN` ends in `-breakdown.md` and the sibling `-decisions.md` exists, that file; otherwise none. It prints `decision log: <path>` or `decision log: none` to stderr so the choice is visible.
- Prints each problem as `<kind>: <statement>` on stdout. Exit 0 with no problems, 1 with problems, 2 on usage or I/O error (message on stderr).
- `run(args []string, stdout, stderr io.Writer) int` holds the logic so tests call it without a subprocess.

### Fixtures (`lsf-runtime/validate/testdata/`)

Full plan documents, each conforming except for its one defect, so each test asserts the exact problem list: `ok.plan.md` (+ `ok.decisions.md`), `overlap.plan.md`, `cycle.plan.md`, `bad-clears.plan.md` + `.decisions.md`, `bad-gates.plan.md` (malformed `lsf-gates`), and `disagree-*.plan.md` for each agreement aspect in check 1–8 (unit set both directions, stub incomplete, missing field, type, deps, scope, scope residue, gate name, gate kind, gate pin, execution-order divergence, execution-order omission). Plus `ok-variants.plan.md` covering: CRLF, `none` fields, grouped deps `a, b (testing, …; …)`, a semicolon inside parentheses, trailing-slash scope, `file::symbol` scope, a pin with `>=`, a complete unit with a full entry, a heading inside a fenced block.

### Error handling

Input-driven defects are Problems; only I/O and usage are errors. The library never panics on input.

### Skill text

- `skills/breakdown/SKILL.md` Phase 2 step 4, the `## Graph` agreement bullet: replace "Check mechanically (a short script over the block and the entry fields), not by eye." with: once the document is drafted and before it is committed (step 7, and every re-plan), run `go -C ${CLAUDE_PLUGIN_ROOT}/lsf-runtime run ./validate/cmd/lsf-validate <plan path>` — it also runs the load stage's checks (overlap, cycle, `clears`, the gates block) and must exit zero; where Go is not installed, check with a short script over the block and the entry fields, not by eye. One sentence states the execution-order rule it applies (incomplete units only).
- `skills/lsf-drive/SKILL.md` Startup: step 2's block/fence check becomes running the same command on the plan (and the decision log via `-decisions` when it exists) at the launch SHA's checkout; a non-zero exit blocks the launch and the problems go to the requester, for `phronithm:breakdown` re-plan mode. Step 3 keeps only the decision log SHA lookup (the fence check is now the command's). Same Go-absent fallback (the existing grep checks).

### Risks and unknowns

- Heuristic Markdown parsing of prose fields: mitigated by deriving every rule from this plan's actual syntax and the breakdown template, prototype-checked on three real plans; resolved during implementation by the zero-exit test on this plan.
- `go -C … run` compiles on every call (seconds, needs a Go toolchain and module cache); acceptable until `runtime-distribution` ships binaries — a discovery for that unit.
- No third-party APIs. Go stdlib only (`regexp`, `strings`, `flag`, `os`), consistent with the stdlib-only module rule.

## Implementation plan

Each step: implement, `gofmt -l`/`go vet ./...`/`go test -race ./...` in `lsf-runtime`, commit, spawn the step's critique gate (critique-code, sonnet) and continue.

1. **Document parser** — `validate/entries.go`: section/heading location (fence-aware, CRLF), entry and field extraction, the paren-aware splitter and paren stripper. Tests: unit tests on small strings for each parsing rule. Done: tests pass.
2. **Agreement checks** — `Agreement` with checks 1–8 and deterministic ordering. Tests: `disagree-*` and `ok-variants` fixtures assert exact problem lists; a test runs `Agreement` on `../../docs/plans/lsf-improvements-breakdown.md` (path relative to the package) and expects none (skip if absent). Done: tests pass; zero on this plan.
3. **`Check` and the command** — `validate.go` `Check`; `cmd/lsf-validate/main.go` with `run`. Tests: overlap, cycle, bad-clears, bad-gates fixtures through `run` (exit 1, expected lines), `ok` (exit 0), usage/missing-file (exit 2), decision-log derivation (sibling found, `-decisions` override, none). Manual: `go run ./validate/cmd/lsf-validate ../docs/plans/lsf-improvements-breakdown.md` exits 0. Done: tests pass, manual run exits 0.
4. **Skill text** — breakdown quality-check bullet and lsf-drive Startup steps 2–3 as designed. Done: diff read against S3.

Steps are sequential (2 depends on 1, 3 on 2; 4 needs the final command path). No parallelism worth taking.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/plan-preflight-check-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit plan-preflight-check of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim (Phase 4 re-verifies the external gates against these
pins before implementing, recording the outcome in the implementation-summary):
see "Unit entry (verbatim …)" above — scope, publishes, depends-on, external gate `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`), done condition; standing obligation: Register dispositions (P-D4).
After Phase 5, produce the completion-report (contents per the
phronithm:feature skill's Produces) at docs/plans/plan-preflight-check-completion.md,
then present this continuation prompt to the requester:
Run `/phronithm:breakdown` in re-plan mode with docs/plans/lsf-improvements-breakdown.md and docs/plans/plan-preflight-check-completion.md before starting any dependent unit.

Known items for the completion report so far: deviation — execution-order check covers incomplete units only (requester decision 1); discovery for `runtime-distribution` — ship `lsf-validate` too; discovery for `runtime-cli-drive` — may expose `validate.Check` as a subcommand; option for re-plan — the orchestrator's load could run `validate.Agreement` (behaviour change, undecided); archipeligo's plan has a retired stub the check rejects.
