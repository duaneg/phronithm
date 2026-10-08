# plan-preflight-check — implementation summary

- **Branch**: `lsf-workflow` (base `2c98baf`)
- **Plan**: `docs/plans/plan-preflight-check-plan.md` (committed `07ed7ef`)
- **External gate re-probe** (Phase 4 entry): `lsf-toolchain` — `go version` → go1.27.1, `lsf-runtime/go.mod` `go 1.27` / `toolchain go1.27.1`. Pin confirmed, no drift.

## Files modified

- `lsf-runtime/validate/validate.go` — `Check`
- `lsf-runtime/validate/entries.go` — document parser
- `lsf-runtime/validate/agreement.go` — `Agreement`, checks 1–8
- `lsf-runtime/validate/cmd/lsf-validate/main.go` — the command
- `lsf-runtime/validate/{entries,agreement}_test.go`, `cmd/lsf-validate/main_test.go`
- `lsf-runtime/validate/testdata/` — `ok`, `ok-variants`, `overlap`, `cycle`, `bad-clears` (+ decisions), `bad-gates` plans; `ok.decisions.md`
- `skills/breakdown/SKILL.md` — Phase 2 step 4, Graph-agreement bullet
- `skills/lsf-drive/SKILL.md` — Startup steps 2–3

## Per-step record

### Step 1 — document parser (`9dccec5`)
- Decision: a field given twice in one entry is recorded (first value kept) and reported by `Agreement`, rather than silently comparing one value. Not in the plan.
- Decision: a heading of any level ends an entry's fields; a level-1 heading also ends a `##` section.

### Step 2 — agreement checks (`c8de51d`)
- Deviation: `Agreement` lives in `agreement.go`, not `validate.go`/`entries.go`.
- Decision: a unit never mentioned in Execution Order is reported once, as an omission, and left out of the order comparison, so one cause gives one problem.
- Decision: pin tokens have surrounding `,;:()"'`` ` `` trimmed before the containment test (a pin like `"x.md; transcripts …"` would otherwise never match). Containment is tested against the entry's pin text (after the kind), not the whole gate segment.
- Decision: `setProblems` also reports a value repeated in the entry.
- Surprise: the rationale-mention trap needs the early mention to come *before* a predecessor's first mention; a mention after it changes nothing. The test was corrected to exercise it.
- Folded in step 1 critique fixes (see below).

### Step 3 — `Check` and the command (`c5a5c80`, `31ffcf7`)
- Manual: `go -C lsf-runtime run ./validate/cmd/lsf-validate "$PWD/docs/plans/lsf-improvements-breakdown.md"` → `decision log: none`, exit 0.
- Surprise: `go run` reports any non-zero child exit as 1 (printing `exit status N` on stderr), so exit codes 1 and 2 are distinguishable only when the binary is run directly. The skill texts tell problems from failures by stdout instead.
- Folded in step 2 critique fixes.

### Step 4 — skill text (`a5cb6b2`, `d4a1e8c`)
- Deviation: paths are absolutised with `"$(git rev-parse --show-toplevel)/…"`, not `"$PWD/…"` (robust to a cwd below the repo root; step 4 critique F3).
- Deviation: breakdown passes `-decisions` explicitly when a log exists rather than relying on sibling derivation (critique F6).
- Deviation: the lsf-drive sentence on the D1–D4 divergences was cut (critique F4: no action for the reader); the `lsf-gates` note stays.

## Per-step critique findings

### Step 1 (sonnet, critique-code) — passed, no Critical
- S1 no panic test → addressed (degenerate-input table, `FuzzParseDocument`, 30 s fuzz run clean).
- S2 live plan never parsed; rule gaps → addressed (`TestParseLivePlan`, CRLF continuation, empty sections, heading levels).
- M3 fence length / closing rules → addressed (CommonMark run length, info string, backtick-in-info).
- M4 fence after blank didn't end a field → addressed.
- M5 quadratic blank-line lookahead → noted for Phase 5.
- M6 tautological test, nil deref in test → addressed.
- M7 nil-vs-empty section → addressed (`found` bool).
- M8 unspecified forms (`**Type:**`, duplicate sections, backticked heading ids) → partly addressed in step 2 (text-but-no-fields problem); rest noted for Phase 5.
- M9 unbalanced backticks → addressed (splitters fail).

### Step 2 (sonnet, critique-code) — passed, no Critical
- S1 malformed field syntax diagnosed as an incomplete stub → addressed (`has text but no - **Name**: value field lines`).
- S2 unreadable gate segment cascaded into a "missing" problem; empty segments → addressed.
- M3 pin containment against pin text, not whole gate text → kept; recorded above as an interpretation.
- M4 `none.` / `none (…)` not accepted; M5 trailing `.` in Scope → noted for Phase 5 (strictness; no live plan uses these).
- M6 error text named only parentheses → addressed.
- M7 heading ids with backticks / empty → noted for Phase 5.
- M8 `Agreement(md, nil)` panicked → addressed.
- M9 bold mention with trailing punctuation → noted for Phase 5.
- M10 coverage gaps → mostly addressed (unbalanced Scope/gates, gate without kind, repeated gate, syntax variant); block duplicate ids left to `ValidatePlan`.

### Step 3 (sonnet, critique-code) — passed, no Critical
- M1 output format vs spec → no change: the plan specifies `<kind> [<referent>]: <statement>`; the critic misread it.
- M2 sibling-stat-error branch untested → addressed (sibling is a directory → exit 2).
- M3 `-decisions ""` equals absent; `-h` exits 2 → noted for Phase 5.
- M4 unreadable-file test portability, ignored setup errors → addressed (probe-and-skip, `mustWrite`).
- M5 live-plan skip only on not-exist → noted for Phase 5.
- M6 malformed decision log untested → addressed (exit 1 with the load problem).
- M7 `Check` mirrors load → confirmed, no change.

### Step 4 (sonnet, critique-skill) — passed, no Critical
- S1 no way to tell build/I-O failure from problems; `go run` collapses exit codes → addressed (verified; stdout rule).
- S2 working-tree-equals-SHA claim overstated → addressed (scoped to a clean-tree pre-flight on the integration branch).
- S3 `$PWD` fragility → addressed (`git rev-parse --show-toplevel`).
- M4 rationale prose → addressed. M5 breakdown bullet overloaded → trimmed. M6 implicit log derivation in breakdown → addressed. M7 `<log-path>` undefined → addressed.

## Success criteria

| # | Criterion | Status | Target | Achieved | Evidence |
|---|---|---|---|---|---|
| S1 | Non-zero with the load stage's problems on each defect fixture | pass | proven | proven | `TestLoadStageFixtures` (overlap, cycle, bad clears, bad gates, disagreement through `run`); `TestAgreementDisagreements` (one mutation per aspect, exact lists) |
| S2 | Zero on `lsf-improvements-breakdown.md` | pass | proven + evidenced | proven + evidenced | `TestLivePlan` (run, both halves), `TestAgreementLivePlan`; manual `go -C … run` exit 0 |
| S3 | Both skills name the command | pass | evidenced | evidenced | diff of `a5cb6b2` + `d4a1e8c` read |
| S4 | `go test -race ./...` and `go vet ./...` pass | pass | proven | proven | full suite run after step 3 (all packages ok); validate packages re-run after each later commit |
| S5 | P-D4 disposition recorded | pass | evidenced | evidenced | `plan-preflight-check-completion.md`, Standing obligations |

## Remaining phases
Complete Phase 5 (Review) as defined in the phronithm:feature skill.
