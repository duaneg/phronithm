# runtime-core — completion report

Unit `runtime-core` of `docs/plans/lsf-improvements-breakdown.md` (entry at `b537a01`). Base `e5cfd56`, branch `lsf-workflow`. Plan: `docs/plans/runtime-core-plan.md`. Implementation summary: `docs/plans/runtime-core-implementation.md`.

## Done condition

**Met.** The done condition is achieved at `proven`: every clause has a passing test against the stub `claude` (55 tests, `go test -race ./...` from `lsf-runtime/`). Two parts are below `proven`:

- **Session-limit stream shape** (`conjectured`). The stub stream is built from 2.1.289's schema and from interactive transcripts; a real limit under `-p` has not been observed. Upgrade path: the first real limit's transcript is kept under `calls/`.
- **Orphan reaping on macOS** (`conjectured`). It is proven on Linux only. Upgrade path: `runtime-distribution`'s CI on macOS.

**"`unrecognized_model`."** The done condition's example permanent error does not occur in the stream. A real 2.1.289 unknown-model call carries assistant `error: model_not_found` and a result with `is_error`, 404. The test uses that recorded stream.

**End to end.** A real 2.1.289 run (an isolated call and a skill-prefixed critique call), and its replay, passed (`evidenced`).

## Deviations from the entry's scope and publishes

- **Added: worktree isolation.** A call with `isolation: 'worktree'` gets a worktree on `lsf/<plan8>/<key>`. The requester agreed to this at scope confirmation.
- **Added to publishes:**
  - `rt.Call` / `CallResult` (value, class, flags, denials, attempts, cost, replayed), the channel through which `runtime-signals` reads driver events;
  - `rt.Step`, the journal's code-step API.
- **Skill-load evidence.** It differs from the plan's candidates: a slash-command expansion is not on stdout. A prefixed call counts as loaded when init's `slash_commands` lists the skill; a Skill `tool_use` also counts. Evidence: step 0, 2.1.289, 3/3 and 0/1.
- **Guard timeout after a valid result** keeps the value, flagged `guard-killed-after-result`. The requester's decision ("null, no retry") is kept for a call killed before its result. **Requester to confirm this refinement.**
- **Outside scope:** `CLAUDE.md` gains an `lsf-runtime/` entry.

## Deferrals

- SHA verification is not implemented. It belongs to `code-stage-interface` (rejection) and `runtime-signals` (the corrected-SHA signal).
- Gates on a clean checkout of the returned commit are not implemented. They belong to `declared-gates`.
- Driver events as ledger signals (uncommitted at return, permission denial, `skill-not-loaded`, `guard-killed-after-result`) are not implemented. They belong to `runtime-signals`, which reads them from `CallResult` and telemetry. Uncommitted-at-return is not detected at all yet.
- Cleanup's ignored-file policy is not implemented. It belongs to `code-stage-interface` with `retained-artefacts-design`.
- The stop file, status readers and detaching are not implemented. They belong to `runtime-cli-drive`.
- The roles' content is not implemented. It belongs to `s2-permissions-design`; the runtime takes roles as input.

## Discoveries for sibling and downstream units

- **Claude Code auto-updates** (the `claude-code-headless` gate). The installed default moved 2.1.289 → 2.1.290 mid-session, and `claude --version` before the session showed the old pin. Affected units:
  - Every unit gated on `claude-code-headless` should call the versioned binary (`~/.local/share/claude/versions/<v>`) with `DISABLE_AUTOUPDATER=1`, or confirm the pin from a transcript's `version`.
  - `runtime-cli-drive` should decide whether launches pin `claudeBin` to a versioned path.
  - The re-plan may re-pin to 2.1.290.
- **Labels must be unique per call** (`orchestrator-port-loop`). The journal keeps one record per label, and a label reused with different inputs in one launch is logged `label-reused`. Identical label and inputs replay, within a launch too. The port should confirm that `implement.js`'s labels are unique.
- **Role shape** (`s2-permissions-design`). A role is `{tools, allow, deny, permissionMode}`; `tools` must be non-empty. Rules match `agentType` AND a label regex (unanchored); express OR as two rules.
- **Step API** (`code-stage-interface`). `Step(ctx, label, input, fn(ctx, interrupted))` replays a recorded step. A failed `fn` keeps its intent, so the next run of the step gets `interrupted=true`. An unreadable step record stops the run.
- **Launch entry and state-dir layout** (`runtime-cli-drive`).
  - `Run(ctx, cfg, body)` is the launch entry.
  - It writes `status.json`, `summary.json` (`stop_reason`), `log.txt` and `telemetry.jsonl`, and holds an `flock` on `lock`.
  - A first signal stops at the next boundary; a second kills in-flight calls.
- **Detached and lingering processes** (`long-running-design`). A call that exits normally is not group-killed, so a descendant holding stdout lingers and the call returns `killGrace` late. A retry after partial work reruns in the same, dirty, worktree.
- **Size** (re-plan, R1). The runtime alone is about 2,190 lines of code and 1,730 of tests. The spike estimated 1,500–2,500 lines, tests included, for all of S2 (runtime, code stages, signals, CLI). The remaining S2 units are likely larger than estimated.
- **Session limits** are assumed to apply across models: one runtime-wide gate. Weekly or model-specific limits (`rateLimitType`) also pause other models' calls, and a wait is bounded only by the wall-clock cap.

## Standing obligations

None named this unit.

## External gates

- **`lsf-toolchain`** (Go 1.27.1): confirmed.
- **`claude-code-headless`** (2.1.289): **drift found** (2.1.290, auto-update). Resolution, the requester's choice: all fixtures and the end-to-end run were re-recorded on the 2.1.289 binary, called directly (`703b295`). The installed default remains 2.1.290.

## Execution signals

- **Context at completion:** comfortable.
- **Design decisions taken mid-implementation rather than at the design pass:**
  - the skill-load marker (from step-0 evidence);
  - steps 3 and 4 merged;
  - the gate moving before the slot is released (a race found by a test);
  - the worktree created after admission;
  - `guard-killed-after-result`;
  - orphan carry-forward on `ps` failure;
  - the label-uniqueness assumption, made explicit.
- **Review findings.** The per-step and full-diff critiques found no Critical issues. They found 22 Significant findings, all fixed.

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
