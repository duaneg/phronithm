# runtime-core — plan

Work unit `runtime-core` of `docs/plans/lsf-improvements-breakdown.md` @ `e5cfd56` (entry at `b537a01`). Base commit `e5cfd56`, branch `lsf-workflow`. Phases 1–3 ran compressed in one plan-mode session, so this one document is the problem statement, the design and the implementation plan. Phase 4 step 0 saves it as `docs/plans/runtime-core-plan.md`.

## 1. Problem statement

### Context

S2 runs the orchestrator's `run(runtime)` on a host runtime that makes each stage a headless `claude -p` call. The spike's runtime (`workflows/lsf/spike/driver.js`, 584 lines) proved this (results: `docs/investigations/substrate-spike-results.md`, verdict `confirmed`). It has no tests, and its requirements are listed under Adapter requirements. The maths exemplar (`~/src/maths-proofs/tools/campaign/run.py`) has controls the spike lacked: caps, a guard timeout, signal stop, `stop_reason` and a dry run. This unit builds the runtime in Go (`docs/design/lsf-runtime.md` §Language: stdlib only, package `lsfrt` in `lsf-runtime/runtime/`), with tests against a stub `claude`. No orchestrator exists in Go yet (`orchestrator-port-loop` depends on this unit), so tests drive the runtime with small closures in place of `run`.

### Unit entry, verbatim

```
### runtime-core
- **Type**: foundation
- **Risk**: high — every later script unit runs on it
- **Clarity**: specified
- **Iteration**: write-once
- **Effort**: large
- **Scope**: `lsf-runtime/runtime`, `lsf-runtime/testdata`, `docs/design/lsf-runtime.md::runtime`
- **Publishes**: the host runtime `{ agent, parallel, log, args }` for `run(runtime)`; the journal; stage roles as configuration
- **Depends on**: substrate-spike (knowledge, the spike driver and its adapter requirements); runtime-language-design (design, the language and layout)
- **External gates**: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`); `claude-code-headless` (environment, Claude Code 2.1.289)
- **Done condition**: Tests against a stub `claude` binary show that: a missing or schema-invalid `structured_output` after a clean exit is retried once, then null; a transient error is retried with backoff, a session-limit error waits for its reset, and a permanent one (`unrecognized_model`) is not retried; a prompt naming a skill is sent as its slash command, and a call whose stream shows no skill load is retried or flagged; `--strict-mcp-config` and the role's toolset are passed; a resume replays completed calls and journaled code steps without re-running them; and children left by a SIGKILL during concurrent calls are terminated by the next launch. Tests also show the exemplar's controls: caps on calls, wall-clock and cost; a per-call guard timeout that kills a hung call; SIGINT or SIGTERM stopping at the next call boundary; a recorded `stop_reason`; and a dry run that spawns nothing. Role permissions are an input, not constants. The tests pass.
- **Description**: The spike's adapter (`workflows/lsf/spike/driver.js`; results, Adapter requirements), rebuilt with tests in Go, with the controls of the maths exemplar (`~/src/maths-proofs/tools/campaign/run.py`), which the spike did not take. The code stages are `code-stage-interface`'s.
```

No standing obligations name this unit.

### Scope confirmed with the requester (2026-10-06)

- **Added: worktree isolation.** A call with `isolation: 'worktree'` runs in a runtime-created worktree (the spike's `ensureWorktree`). The unit entry does not name it. It is a deviation for the completion report.
- **Deferred:**
  - SHA verification: rejection to `code-stage-interface`, the corrected-SHA signal to `runtime-signals`.
  - Gates on a clean checkout of the returned commit: `declared-gates`.
  - Uncommitted-at-return, permission-denial and skill-not-loaded signals: `runtime-signals`. This unit records these events in telemetry and in the call record only.
  - Cleanup's ignored-file policy: `code-stage-interface` with `retained-artefacts-design`.
  - Stop file, status consumers, detach: `runtime-cli-drive`.
  - Permission values: `s2-permissions-design`. This unit takes them as input.
- **Decisions:**
  - A call killed by the guard timeout returns null and is not retried.
  - Caps count per launch.
  - A permanent error returns null and does not stop the run. Which driver events stop the run is `runtime-signals`' decision.

### Success criteria

Target rigour `proven` (a `go test` assertion) unless stated.

| # | Criterion | Target |
|---|---|---|
| S1 | Missing or schema-invalid `structured_output` after a clean exit: retried once, then null | proven |
| S2 | Transient error: retried with backoff, then null | proven |
| S3 | Session-limit error: waits for its reset, then retries | proven against a stub stream built from the 2.1.289 schema. A real session limit cannot be triggered on demand, so the stream shape under `-p` stays `conjectured` (§2.6) |
| S4 | Permanent error (unknown model) is not retried | proven, on a stub stream recorded from a real 2.1.289 call (probe P1) |
| S5 | A skill-naming label's prompt is sent as `/<skill> <prompt>`. A call whose stream shows no skill load is retried once (the budget is shared with `invalid`), then accepted and flagged | proven, on streams recorded from real calls (probe P2) |
| S6 | `--strict-mcp-config` and the role's `--tools`, `--allowedTools`, `--disallowedTools` and `--permission-mode` are passed. The role comes from configuration | proven (stub argv dump) |
| S7 | A resume replays completed calls and journaled code steps without re-running them. An interrupted step re-runs with `interrupted=true` | proven |
| S8 | Children left by a SIGKILLed launch during concurrent calls, grandchildren included, are terminated by the next launch | proven on Linux. On macOS `conjectured`: same code path through `ps`, not run |
| S9 | Caps on calls, wall-clock and cost stop at the next call boundary | proven |
| S10 | Guard timeout kills a hung call and its process group; null | proven |
| S11 | SIGINT or SIGTERM: in-flight calls finish, the next call boundary stops | proven |
| S12 | `stop_reason` recorded in `summary.json` for every way a launch ends | proven |
| S13 | A dry run spawns nothing | proven |
| S14 | Role permissions are an input: no role, tool list or rule is a constant in non-test code | proven (a test runs two configs with different roles). Code reading confirms no defaults (`evidenced`) |
| S15 | Worktree isolation: an isolated call's cwd is a worktree on `lsf/<plan8>/<key>` from the integration branch, reused on resume | proven |
| S16 | `go vet ./...` and `go test ./...` pass from `lsf-runtime/` | proven |
| S17 | `docs/design/lsf-runtime.md` gains its Runtime section | evidenced (document review) |

### Candidate approaches

**A. Port the driver's shape: one `Runtime` struct with methods, files-per-key journal.** The spike's structure (call → retry → journal → replay), with the exemplar's supervision added around it. Builds on proven behaviour. Risk: one large type. Mitigated by splitting it into files by concern, with small unexported helpers.

**B. Layered components: `Caller` (spawn plus classify), `Journal`, `Supervisor` (caps, signals, children), composed by `Runtime`.** Each can be tested alone and replaced by later units (`runtime-signals` decorates `Caller`). Cost: interfaces between the parts that only one implementation will ever fill. That is an abstraction for an unknown future, and the later units edit this package anyway (their Scope names `lsf-runtime/runtime`).

**Journal alternative: one append-only JSONL file instead of files per key.** A single file. A torn last line is ignorable. Replay needs a scan and an index. Files per key, written by atomic rename, are what the spike proved (run 6), and a person can inspect them one call at a time.

**Not viable: an Agent SDK.** No Go SDK exists, and the language decision excludes JS/TS.

### Exploration findings

- `implement.js` calls `rt.agent(prompt, opts)` with `opts = {label, phase, model, effort, schema, isolation?, agentType?}` (`implement.js:762-854`), and `rt.parallel(thunks)` over agent calls only (`:828`). `run` wraps `agent` for `args.seed` (`:922-937`). Tier roles are in `DEFAULT_TIERS` (`:112-121`).
- Stage schemas use only `type`, `properties`, `required`, `items` and `enum` (grep of `schema.js` and `implement.js`). The spike's 22-line validator (`spike/validate.js`) covers that subset.
- The spike's call path (`driver.js:166-228`): argv, stream parse for the `result` event, validate, retry (transient `[5,20,60]` s; invalid once), telemetry per attempt.
  - Journal (`:501-562`): the key is the label slug; the hash is sha256(prompt, schema, model/effort). Nulls are not cached. Code steps write an intent, then a record.
  - Orphans (`:77-91`): `children.json` of pid → session id; `/proc` cmdline match; SIGTERM.
- The exemplar (`run.py`):
  - caps checked before each iteration;
  - signal handlers set a flag read at the boundary;
  - `stop_reason` written to `summary.json`;
  - guard timeout: terminate, 30 s grace, kill;
  - dry run: prints the command and stops with `stop_reason: dry-run`.
- The language probe (`docs/investigations/runtime-language-probes/go/call.go`) has the Go process-group pattern: `Setpgid`, plus `cmd.Cancel` killing `-pid`.

### API verification record (Claude Code 2.1.289)

| Assumption | Verified how | Result |
|---|---|---|
| Unknown-model failure shape | `docs/investigations/substrate-spike-data/failed-call.json` (real 2.1.289 result event) | `type: result`, `subtype: success`, `is_error: true`, `api_error_status: 404`, `terminal_reason: api_error`. **The stream has no `unrecognized_model` value.** That string is a model-switch telemetry name in the binary. The assistant-message error enum (below) has `model_not_found`. Probe P1 (step 0) records the full stream. |
| Assistant-message error codes | installed binary, zod schema `ro` | `authentication_failed, oauth_org_not_allowed, account_on_hold, verification_required, billing_error, rate_limit, overloaded, invalid_request, model_not_found, server_error, unknown, max_output_tokens, cloud_credential_error` |
| Rate-limit event | installed binary, schema `C9r` and the `rate_limit_event` message schema | `{type: "rate_limit_event", rate_limit_info: {status: allowed\|allowed_warning\|rejected, resetsAt?: int unix seconds, rateLimitType?: five_hour\|seven_day\|…}}` |
| Session-limit message | real transcripts (archipeligo workflow agent, 2.1.278; cartomancer-v2, 2.1.269) | assistant entry `error: "rate_limit"`, `apiErrorStatus: 429`, text "You've hit your session limit · resets 3:20am (Pacific/Auckland)". Interactive and Workflow transcripts, not `-p` stream-json: the combination under `-p` is **unverified**. |
| Skill-load evidence in the stream | — | **Unverified.** The spike saw "skill body in the transcript" for a slash-command prompt. Probe P2 (step 0) records the stream and fixes the marker. |
| Go process groups, `cmd.Cancel` | language probe, run 2026-10-06 | works on Linux |
| `ps -A -o pid=,args=` and `flock` on macOS | POSIX `ps`; BSD `flock(2)` | unverified on macOS (no host) |
| `-p` with the prompt on stdin, slash command included | — | **Unverified.** Every step-0 probe uses stdin; fallback per §2.4 |

## 2. Design

### 2.1 Selected approach

A, with B's seams kept as files rather than interfaces: `Runtime` is one struct, and each concern lives in its own file with its own tests. Rejected:

- B, for interfaces nothing else implements.
- A JSONL journal, as unproven where files per key are proven.

The per-call result record (`Call`, §2.2) is the one seam added now. `runtime-signals` needs call metadata at every orchestrator call site, and adding it later would change the signature those call sites use.

### 2.2 Public interface (`package lsfrt`)

```go
// Opts mirrors the orchestrator's agent options. Phase is recorded in telemetry only.
type Opts struct {
	Label, Phase, Model, Effort, AgentType, Isolation string
	Schema json.RawMessage
}

// StopError ends the run at a call boundary. Reason is the stop_reason.
type StopError struct{ Reason StopReason; Detail string }
func (e *StopError) Error() string
func IsStop(err error) (StopReason, bool) // errors.As; wrapped stops count

type StopReason string
const (
	StopReturned     StopReason = "returned"       // body returned, no stop pending
	StopSigint       StopReason = "signal-SIGINT"
	StopSigterm      StopReason = "signal-SIGTERM"
	StopCancelled    StopReason = "cancelled"      // Run's parent context ended
	StopMaxCalls     StopReason = "max-calls"
	StopMaxWallClock StopReason = "max-wall-clock"
	StopMaxCost      StopReason = "max-cost"
	StopDryRun       StopReason = "dry-run"
	StopRuntimeError StopReason = "runtime-error"  // the runtime cannot continue: worktree creation failed, Args lack planSha/integrationBranch for an isolated call
	StopCrashed      StopReason = "crashed"        // body returned a non-stop error or panicked
)

// CallResult is one logical call's outcome. Value nil is null.
type CallResult struct {
	Value    json.RawMessage
	Class    string    // class of the last attempt (§2.5)
	Flags    []string  // e.g. "skill-not-loaded"
	Denials  []string  // permission_denials, "tool: input" truncated
	Attempts int       // spawns for this call this launch; 0 when replayed
	CostUSD  float64
	Replayed bool
}

type Runtime struct{ /* unexported */ }

// Call runs one headless call, or replays it from the journal. The only
// non-nil error is *StopError.
func (rt *Runtime) Call(ctx context.Context, prompt string, o Opts) (CallResult, error)
// Agent is Call's value: the orchestrator's agent(). (nil, nil) is null.
func (rt *Runtime) Agent(ctx context.Context, prompt string, o Opts) (json.RawMessage, error)

// Parallel runs every thunk and waits for all of them. A branch's non-stop
// error or panic is logged and its slot is nil (the JS parallel's catch). If
// any branch stopped, the first StopError is returned after all branches finish.
func (rt *Runtime) Parallel(ctx context.Context, thunks []func(context.Context) (json.RawMessage, error)) ([]json.RawMessage, error)

// Step journals a code step. A record with the same label and input hash is
// replayed without calling fn. Otherwise: a stop check (Step is a boundary),
// an intent, fn, then the record (null values included), which removes the
// intent. interrupted is true when an intent with this hash was left by an
// earlier launch. If fn errors, the intent stays, so the next launch reruns
// fn with interrupted=true; the error is returned as is. In a dry run, a step
// that does not replay returns StopError{dry-run} without calling fn.
func (rt *Runtime) Step(ctx context.Context, label string, input []byte,
	fn func(ctx context.Context, interrupted bool) (json.RawMessage, error)) (json.RawMessage, error)

func (rt *Runtime) Log(msg string)
func (rt *Runtime) Args() json.RawMessage

type Summary struct {
	StopReason StopReason       `json:"stop_reason"`
	Detail     string           `json:"detail,omitempty"`
	Started    time.Time        `json:"started"`
	Finished   time.Time        `json:"finished"`
	Calls      int              `json:"calls"`     // spawns this launch
	CostUSD    float64          `json:"cost_usd"`
	Outcome    json.RawMessage  `json:"outcome,omitempty"`
	Error      string           `json:"error,omitempty"`
}

// Run is a launch. It takes the state-dir lock, reaps orphans, installs signal
// handlers, writes status.json, runs body, and writes summary.json.
func Run(ctx context.Context, cfg Config,
	body func(ctx context.Context, rt *Runtime) (json.RawMessage, error)) (Summary, error)

func LoadConfig(path string) (Config, error) // strict: DisallowUnknownFields; validates roles and regexes
```

**Error contract.**

- The error returned by `Run` is for setup failures only:
  - the state-dir lock is held by a live launch;
  - an unreadable state dir;
  - a bad config.
- **`stop_reason` precedence,** first match wins:
  1. a sticky stop the runtime recorded (signal, cap, dry run, runtime error), even when the body swallowed it and returned normally;
  2. a `StopError` in the body's error chain;
  3. the parent context ended → `cancelled`;
  4. any other body error, or a panic → `crashed`;
  5. otherwise `returned`.

### 2.3 Configuration (roles are input)

```go
type Config struct {
	StateDir, Repo string
	ClaudeBin string            // default "claude"; env LSF_CLAUDE_BIN overrides
	Args json.RawMessage        // passed through; worktrees read planSha, integrationBranch
	Concurrency int             // default 4; bounds concurrent spawns
	Roles map[string]Role       // required, ≥1; no built-in roles
	RoleRules []RoleRule        // first match wins
	DefaultRole string          // must name a role
	Skills []SkillRule          // label regex → skill
	Caps Caps
	GuardTimeout, KillGrace Duration     // guard 0 = none; grace default 10s
	Backoff []Duration                   // default [5s, 20s, 60s]
	SessionLimitMargin Duration          // default 30s past resetsAt
	SessionLimitFallback Duration        // default 15m, when no resetsAt is usable
	MaxSessionWaits int                  // default 4 per call
	DryRun bool
}
type Role struct {
	Tools string            // passed to --tools verbatim
	Allow, Deny []string    // --allowedTools / --disallowedTools
	PermissionMode string   // --permission-mode; required
}
// RoleRule matches when every non-empty matcher matches (AND); at least one is
// set. Express OR as two rules. Label is an RE2 regex.
type RoleRule struct{ AgentType, Label, Role string }
type SkillRule struct{ Label, Skill string }
type Caps struct{ MaxCalls int; MaxWallClock Duration; MaxCostUSD float64 } // 0 = no cap
type Duration time.Duration // JSON "90s", "2h"
```

`testdata/config.example.json` carries the spike's roles (critic rule written as two rules: agentType, label regex), its skill rules and a non-zero guard timeout, as a worked example. It is not a default: nothing loads it implicitly.

### 2.4 Headless call

- **argv.** In order: `-p --output-format stream-json --verbose --session-id <uuid> --model M [--effort E] --strict-mcp-config --permission-mode R.PermissionMode --tools R.Tools [--allowedTools A…] [--disallowedTools D…] --json-schema <schema>`.
  - The prompt goes on **stdin**, so it is not bounded by `MAX_ARG_STRLEN` (128 KiB on Linux) or macOS's `ARG_MAX`.
  - The prompt is `/<skill> <prompt>` when a `SkillRule` matches the label. Step 0 verifies that `-p` with the prompt on stdin works, slash command included. If it does not, the prompt goes back to argv with a 100 KiB guard that rejects with `spawn-error`.
  - The session id is a random UUIDv4 from `crypto/rand`.
- **Spawn.** `exec.CommandContext` with `Setpgid`.
  - The session id goes into `children.json` before `Start`, and is removed after `Wait`.
  - `cmd.Cancel` sends SIGTERM to `-pgid`; `cmd.WaitDelay = KillGrace`.
  - After `Wait` returns on a cancelled call (guard timeout or second signal), `kill(-pgid, SIGKILL)` runs at once. While any member of the group lives, its pgid cannot be reused, so the kill reaches only survivors such as a TERM-ignoring grandchild. If no member is left, it returns ESRCH.
  - A call that exits normally is not group-killed. What happens to processes a stage deliberately detaches is `long-running-design`'s question.
  - stdout is teed to `calls/<key>.<launch>.a<n>.jsonl`, where `<launch>` is the launch's start time plus pid, and parsed line by line. The tail of stderr is kept.
  - A failure to start (ENOENT, EACCES) is class `spawn-error`.
- **Guard timeout.** Each attempt runs under `context.WithTimeoutCause(GuardTimeout, errGuard)`. The second-signal cancel uses cause `errKilled`. The classifier reads `context.Cause` to tell `timeout` from a stop.
- **Stream parse.** Each line is decoded leniently into a union struct: `type`, `subtype`, `is_error`, `api_error_status`, `structured_output` (raw), `total_cost_usd`, `usage`, `permission_denials`, `terminal_reason`, assistant `message.content` and `error`, and `rate_limit_info`. Unknown fields are ignored: the stream carries many and adds more by version. Strictness is applied to `structured_output` only, by the schema validator. Undecodable lines are counted in telemetry, not fatal.
- **Skill-load detection.** Keyed on the marker probe P2 fixes (step 0). The candidates are:
  - a `user`-type event carrying the expanded command;
  - an assistant `tool_use` named `Skill` with `input.skill == <skill>`.

  The chosen marker must be present in every loaded sample and absent from every absent sample.

### 2.5 Classification and retry

Classification reads the `result` event first. Assistant `error` events are consulted only when the result is missing or `is_error`, because the CLI can recover from a mid-stream error and still return a valid result. Per attempt, first match wins:

| Class | Condition | Action |
|---|---|---|
| `stopped` | context cause `errKilled` (second signal) | return the stop |
| `timeout` | context cause `errGuard` | null; no retry |
| `spawn-error` | the process did not start | null; no retry; telemetry `spawn-error` |
| `ok` | `result` not `is_error`, `structured_output` passes the schema, and the skill loaded or none is required | return |
| `skill-missing` | as `ok`, but a `SkillRule` matched and there is no load marker | retry (shared budget, below). When the budget is spent, accept the value with flag `skill-not-loaded` |
| `invalid` | `result` not `is_error`, `structured_output` missing, null or failing the schema | retry (shared budget). Then null |
| `session-limit` | `result` missing or `is_error`, and the stream has an assistant `error: rate_limit`. Wait until `resetsAt + margin`, taking `resetsAt` from a `rate_limit_event` with `status: rejected`; without one, wait `SessionLimitFallback` | set `limitUntil` (below), wait, retry. Does not consume the transient budget. After `MaxSessionWaits` waits for one call, null |
| `permanent` | assistant `error` ∈ {authentication_failed, oauth_org_not_allowed, account_on_hold, verification_required, billing_error, invalid_request, model_not_found, cloud_credential_error}, or `is_error` with `api_error_status` ∈ {400, 401, 403, 404} | null; no retry |
| `transient` | anything else: assistant `error` ∈ {overloaded, server_error, unknown}; `is_error` with any other status or none; non-zero exit; no `result` event | back off `Backoff[i]`, then null |

- **Budgets.** `invalid` and `skill-missing` share one retry: at most two spawns between them. `transient` has `len(Backoff)` retries. `session-limit` has `MaxSessionWaits`. `max_output_tokens` falls under `invalid`: the result is not `is_error`, and the output is missing.
- **Session-limit gate.** `limitUntil` is runtime-wide. This assumes the five-hour session limit applies across models, which is the limit archipeligo hit 17 times. A model-specific weekly limit would also pause other models' calls. That is accepted and documented.
- **Order inside `Call`:**
  1. Journal lookup. A replay returns at once, emits `cache-hit` with the record's flags, and does not count against caps.
  2. Stop check: Call entry is a boundary.
  3. Dry-run check: log the argv, set the sticky stop `dry-run`, return.
  4. Worktree, if isolated (§2.8).
  5. Per attempt, a loop:
     - wait while `now < limitUntil`, re-reading `limitUntil` on wake;
     - acquire a concurrency slot;
     - then, in one critical section: the stop check, then the cap check, which also increments the spawn count;
     - spawn;
     - release the slot.

  Backoff and gate waits hold no slot. Every wait `select`s on the stop channel and the context.
- **After a stop.** An attempt already running finishes. A valid result is then returned and journaled, so the next boundary stops. Otherwise retries are abandoned and the stop is returned.
- **Cost.** Every attempt's `total_cost_usd` counts. A killed attempt with no `result` event counts 0, which is documented.

### 2.6 Journal

Layout under `StateDir`:

| Path | Holds |
|---|---|
| `lock` | flock'd for the launch's life |
| `journal/<key>.json` | the record `{label, kind: call\|step, hash, ret, flags, at}` |
| `journal/<key>.intent` | the hash, for an interrupted step |
| `calls/<key>.<launch>.a<n>.jsonl` | the transcript of each attempt |
| `children.json` | session ids of live calls |
| `status.json`, `summary.json`, `log.txt`, `telemetry.jsonl` | launch status and records |

- `key = slug(label) + "-" + hex(sha256(label))[:8]`. The suffix stops two labels that slug alike, such as `a:b` and `a_b`, from colliding.
- **Call hash:** sha256 over `prompt ‖ 0x00 ‖ schema ‖ 0x00 ‖ model ‖ "/" ‖ effort`. The prompt is the one before the skill prefix. Role and permissions are excluded, so a permission change does not invalidate completed work.
- **Step hash:** sha256(`input`).
- Records, intents and `children.json` are written with `os.CreateTemp` in the target directory, then fsync, rename and a directory fsync. Each is serialised by a mutex where it is shared.
- Null call returns are not recorded, so a resume retries them (spike parity). Step records are recorded whatever the value.
- A record with another hash is `cache-stale`, and the call re-runs.

### 2.7 Supervision

- **Launch exclusion.** `Run` takes `syscall.Flock(LOCK_EX|LOCK_NB)` on `StateDir/lock` before anything else. If the lock is held, that is a setup error. The lock is released at process exit, including SIGKILL, so a dead predecessor never blocks a launch.
- **Caps.** Checked in the per-attempt critical section (§2.5):
  - spawns this launch ≥ `MaxCalls` → `max-calls`;
  - elapsed time ≥ `MaxWallClock` → `max-wall-clock`;
  - summed cost this launch ≥ `MaxCostUSD` → `max-cost`.

  In-flight calls may overshoot the cost cap, as in the exemplar. A cap stop is sticky. A `limitUntil` past the wall-clock deadline stops with `max-wall-clock` at once.
- **Signals.** `signal.Notify(SIGINT, SIGTERM)`. `signal.Stop` and teardown run when `Run` returns.
  - The first signal sets the sticky stop `signal-SIGINT` or `signal-SIGTERM`.
  - The second and any later signal cancel in-flight attempts (cause `errKilled`), and those calls return the stop.
- **Orphans.** After the lock is taken, `children.json` is read. A torn or unparseable file is logged and treated as empty.
  - `ps -A -o pid=,args=` lists processes. Each process whose args contain a recorded session id gets SIGTERM to its group (`-pid`), a wait up to `KillGrace`, then SIGKILL to the group.
  - Each kill emits `orphan-terminated`, and the file is reset.
  - Matching by session id covers a child recorded before `Start` that has no pid on record.
  - If the leader is gone, its group is not signalled, since nothing identifies it (§2.9). In a dry run, orphans are reported, not killed.
- **Status and summary.**
  - `status.json` is `{pid, state: running|<stop_reason>, at}` and is written at start and at end.
  - `summary.json` is written once at end. Panics are recovered in the body goroutine and in each `Parallel` branch.
  - `log.txt` gets lines with timestamps. `telemetry.jsonl` gets one JSON event per line, appended under a mutex.

### 2.8 Worktrees

`Isolation == "worktree"`:

- A mutex serialises all worktree operations. Concurrent `git worktree add` calls contend on git's locks.
- If `<StateDir>/wt/<key>` exists, it is reused (resume).
- If the branch `lsf/<planSha[:8]>/<key>` exists, it is added without `-B`, so a previous launch's commits survive a deleted worktree directory.
- Otherwise `git worktree add -b <branch> <path> <integrationBranch>`.
- If `Args` lack `planSha` or `integrationBranch`, or git fails, the runtime sets the sticky stop `runtime-error` with the detail, and `Call` returns it. It never falls back silently to the repo.

### 2.9 Risks and unknowns

| Risk | Resolution | When |
|---|---|---|
| Skill-load marker unknown | Probe P2: up to five runs each with and without the prefix, until at least one absent stream is seen. If no absent stream appears, the absent fixture is a loaded stream with the marker events removed, stated as such. If no marker separates loaded from absent, stop and escalate: S5 cannot be met as written | step 0, before implementation |
| Prompt on stdin with `-p` | Verified by every step-0 probe, which uses stdin. Fallback per §2.4 | step 0 |
| Session-limit stream under `-p` unobserved | Stub stream built from the binary's schema plus the real transcript text, and a fallback wait when `resetsAt` is missing. P3 keeps any real `rate_limit_event` line to confirm its field names. S3 is recorded `proven` against the stub and `conjectured` for the real shape. Upgrade path: the per-launch transcripts keep the stream the first time a real run hits a limit | after; in the completion report |
| Retry of a worker call after partial work reruns in a dirty worktree | Spike behaviour, unchanged. Noted for `runtime-signals` and `long-running-design` | after |
| macOS `ps`, `flock` and process groups unrun | Same POSIX calls. `runtime-distribution` CI runs the suite on macOS | after |
| A surviving grandchild of an orphan group whose leader is gone | Not signalled: nothing identifies it. Recorded as a known gap | after |
| No guard timeout by default (0) | A hung call blocks until a second signal. The example config sets one. `runtime-cli-drive` chooses the default it launches with | after |

### 2.10 Plan critique findings and dispositions (round 1, critique-design, sonnet)

The round passed: no Critical findings. Every Significant finding was addressed in this revision, because several change the §2.2 interface.

| # | Finding | Disposition |
|---|---|---|
| 1 | Assistant `error` events ahead of the final result | Result first (§2.5). Step 1 adds an error-then-valid fixture |
| 2 | `Agent` error contract vs worktree failure | Sticky `runtime-error` stop (§2.2, §2.8) |
| 3 | `StopSignal` vs `signal-SIGINT` | Distinct constants, plus `Detail` |
| 4 | Stop-reason precedence | Stated (§2.2). Step 5 tests a swallowed stop |
| 5 | Panics in `Parallel` | Recovered per branch. Step 3 tests it |
| 6 | No launch exclusion | `flock` (§2.7). Step 2 tests it |
| 7 | `children.json` atomicity, and recording before `Start` | Atomic writes, session id before `Start`, match by `ps -A` (§2.4, §2.7). Step 5 tests a torn file |
| 8 | Order of checks, waits and slot; `Concurrency` untested | Order fixed (§2.5). Step 3 tests concurrency and a signal during a wait |
| 9 | Retries after a stop | Abandoned. Waits `select` on stop (§2.5). Step 5 tests SIGINT during backoff |
| 10 | Dry-run side effects | Dry-run check before the worktree. `Step` stops without `fn`. The test checks for no `wt/` and no branch |
| 11 | Argument size; spawn errors | Prompt on stdin; `spawn-error` class. Step 3 tests a missing binary |
| 12 | Untested SIGKILL escalation | Group SIGKILL right after `Wait` on a cancelled call. Stub modes `hang-ignore-term` and `orphan-ignore-term` |
| 13 | Concurrent `worktree add`; `-B` reset | Mutex; no `-B` for an existing branch. Step 4 tests both |
| 14 | `Run` needed before step 5 | Minimal `Run` moved to step 2 |
| 15 | No call metadata channel | `Call`/`CallResult` (§2.2) |
| 16 | Transcript name collisions | Launch id in the name |
| 17 | P2 single sample | Up to five each, with a stated fallback (§2.9) |
| 18 | S3 cannot falsify; no fallback | Fallback wait. P3 keeps a real `rate_limit_event` |
| 19 | Global gate | Assumption stated (§2.5) |
| 20 | `Step` edge cases | Specified (§2.2) |
| 21 | Shared retry budget | Shared (§2.5). Step 3 covers the mixed case |
| 22 | `RoleRule` semantics | AND; OR as two rules |
| 23 | Cost rules; replay not counted | Stated. Step 5 tests that a replay does not count |
| 24 | Signal test readiness; handler teardown | Readiness from `children.json`; `signal.Stop` |
| 25 | JSON tags, `Phase`, temp files | Done |

## 3. Implementation plan

Every step: `gofmt -l` empty, `go vet ./...` and `go test ./...` pass from `lsf-runtime/`, commit, then a critique-code gate run (subagent, concurrent with the next step).

**Step 0 — save, re-probe, record real streams.**

- Save this plan to `docs/plans/runtime-core-plan.md` and commit it.
- Re-probe the gates: `go version` must be 1.27.1; `claude --version` must be 2.1.289, read from a headless transcript's `version`.
- Probes, cwd a scratch git repo, `--strict-mcp-config`, haiku/low, the prompt on stdin:
  - **P1:** `--model claude-nonexistent-9`.
  - **P2a:** `/phronithm:critique <tiny artefact>` with `--json-schema` of a one-field findings shape.
  - **P2b:** the same without the prefix.
  - **P3:** a plain structured-output call.
- Save to `lsf-runtime/testdata/streams/{permanent,skill-loaded,skill-absent,ok}.jsonl`:
  - trim to the events the parser reads, with values unchanged;
  - keep any `rate_limit_event`;
  - replace absolute home paths with `/home/user`.
- **Accept:**
  - stdin delivery works for all probes;
  - the four fixtures are committed;
  - the marker is named, with its present and absent counts, in the implementation summary;
  - or an escalation per §2.9.

**Step 1 — config, validator, stream parsing, classification** (pure functions).

- **Files:** `config.go`, `validate.go`, `stream.go`, `classify.go`, and their tests.
- **Accept:** `LoadConfig` rejects:
  - unknown fields;
  - no roles;
  - a rule naming a missing role;
  - a rule with no matcher;
  - a bad regex;
  - an empty permission mode.

  The validator matches `spike/validate.js` on a table that covers each keyword. Classification yields the §2.5 class on:
  - each step-0 fixture;
  - synthetic `transient` (529);
  - `session-limit`, with and without `resetsAt`;
  - `invalid`;
  - error-then-valid-result, which must be `ok`.

**Step 2 — runtime skeleton and headless call; stub.**

- **Stub.** Extend `testdata/claude`, keeping its existing modes and staying bash-3.2 compatible:
  - it reads the prompt from stdin;
  - `STUB_SCRIPT` is a comma list of modes consumed one per invocation through a counter in `STUB_STATE`, with the last mode repeating;
  - every invocation writes its NUL-separated argv to `$STUB_STATE/argv.<n>` and its stdin to `$STUB_STATE/stdin.<n>`;
  - modes: `ok`, `nullout`, `invalid`, `hang`, `hang-ignore-term` (leader and grandchild trap TERM), `orphan-ignore-term` (leader exits on TERM, grandchild traps it), `slow:<s>`, `cost:<usd>`, `transient`, `session` (emits `resetsAt = now+1`), `session-noreset`, and `fixture:<name>`, which cats `testdata/streams/<name>.jsonl`.
- **Code.**
  - `runtime.go`: the `Runtime` struct; a minimal `Run` (lock, state dirs, body, precedence, `summary.json`, `status.json`); `Log` and `Args`.
  - `call.go`: argv, stdin, spawn, guard timeout, group kill, transcript, `children.json`.
- **Accept (S6, S10, S14, and launch exclusion):**
  - the argv test asserts `--strict-mcp-config` and the configured role's four flags, for two configs with different roles;
  - the stdin holds the skill-prefixed prompt for a matching label;
  - `hang`, `hang-ignore-term` and `orphan-ignore-term` under a 1 s guard each end within guard + grace + 1 s, with the leader and grandchild pids (`STUB_PIDFILE`) dead;
  - a second `Run` on a locked state dir returns a setup error.

**Step 3 — classification loop, gate, concurrency, Parallel.** Files: `agent.go` (`Call`, `Agent`, the attempt loop per §2.5) and `parallel.go`.

- **Accept (S1–S5):** stub-invocation counts and returns:
  - `invalid,ok` → 2 spawns, value; `invalid,invalid` → 2, null; `invalid` then skill-absent → 2, value flagged;
  - `transient,transient,ok` with backoff `[10ms, 10ms]` → 3, value, and measured waits at least the backoff; all transient → `len(Backoff)+1`, null;
  - `session,ok` → waits until at least `resetsAt + margin` (margin 0), then value, with the transient budget untouched;
  - `session-noreset` with a 50 ms fallback → waits, then retries;
  - a second concurrent call does not spawn before `limitUntil`;
  - `fixture:permanent` → 1 spawn, null;
  - skill label with `fixture:skill-absent` twice → 2 spawns, value, flag `skill-not-loaded` in the `CallResult`, the journal record and telemetry; `fixture:skill-loaded` → 1 spawn;
  - a missing `ClaudeBin` → `spawn-error`, 1 attempt, null.
- **Accept (concurrency and Parallel):**
  - `Concurrency 2` with 4 `slow:0.5` calls → at most 2 stubs alive at once, read from `children.json` samples or the stub's start and end stamps;
  - `Parallel` with a panicking thunk → the slot is nil and the others return.

**Step 4 — journal, Step, replay, worktrees.**

- **Files:** `journal.go`, `worktree.go`.
- **Accept (S7, S15):**
  - a second `Run` on the same state dir replays a completed call: 0 spawns, the same value, and the flags re-emitted;
  - a changed prompt re-runs;
  - a null is not replayed;
  - `Step` replays without calling `fn`;
  - with a planted intent, `fn` runs with `interrupted=true`;
  - an erroring `fn` leaves its intent;
  - labels `a:b` and `a_b` get distinct keys;
  - an isolated call in a temp git repo runs with cwd `wt/<key>` on branch `lsf/<plan8>/<key>`, and a second launch reuses it;
  - after deleting `wt/<key>` while its branch carries a commit, the next launch's worktree still has that commit;
  - `Parallel` of 4 isolated calls all get worktrees;
  - missing `planSha` → `runtime-error`.

**Step 5 — supervision.**

- **Files:** `supervise.go` (caps, signals, sticky stop) and `children.go` (reaping).
- Subprocess tests re-exec the test binary through a `TestMain` helper switch, with readiness taken from `children.json`.
- **Accept (S8, S9, S11–S13):**
  - **Caps:**
    - `MaxCalls 2` → the third `Call` returns `max-calls`;
    - `cost:0.6` with `MaxCostUSD 1` → the third returns `max-cost`;
    - `MaxWallClock 1s` with `slow:1.2` → the next returns `max-wall-clock`;
    - a replayed call does not count toward `MaxCalls`;
    - each writes `summary.json` with that `stop_reason`.
  - **Signals:**
    - SIGINT mid-`slow:2` → the call completes and is journaled, the next `Call` stops, and `stop_reason` is `signal-SIGINT`; the same for SIGTERM;
    - SIGINT during a backoff → no further spawn, the stop is returned;
    - a second SIGINT kills the in-flight call and its group;
    - a body that swallows the stop and returns normally still records `signal-SIGINT`.
  - **Dry run:** no stub invocation, no `wt/`, no `lsf/*` branch; `Step` does not call `fn`; `stop_reason` is `dry-run` with the argv logged.
  - **Body outcomes:** returned → `returned`; a panic → `crashed`; a cancelled parent context → `cancelled`.
  - **Orphans:**
    - the helper runs `Parallel` of three `hang` calls; after three `children.json` entries, the test SIGKILLs the helper and checks that the stubs and grandchildren are still alive (the precondition);
    - a new `Run` terminates all of them, emits three `orphan-terminated`, and the pids are dead;
    - a torn `children.json` is logged and the launch proceeds.

**Step 6 — docs.**

- Write `docs/design/lsf-runtime.md` §Runtime: interface, configuration, classification table, journal and state-dir layout, supervision, deferrals.
- Commit `testdata/config.example.json`.
- **Accept (S17):** the section covers §2.2–2.8 without restating code.

Dependency order: 0 → 1 → 2 → 3 → 4 → 5 → 6.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/runtime-core-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit runtime-core of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim (Phase 4 re-verifies the external gates against these
pins before implementing, recording the outcome in the implementation-summary): see §1 "Unit entry, verbatim" above; no standing obligations name this unit.
External gates: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`); `claude-code-headless` (environment, Claude Code 2.1.289).
After Phase 5, produce the completion-report (contents per the
phronithm:feature skill's Produces) at docs/plans/runtime-core-completion.md,
then present this continuation prompt to the requester:
Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
Record for the completion report: the worktree scope addition; the deferrals listed in §1; the `unrecognized_model` → `model_not_found` discovery; S3's and S8's conjectured parts.
