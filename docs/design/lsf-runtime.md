# LSF runtime

The S2 host runtime ([substrate record](lsf-substrate.md), [spike results](../investigations/substrate-spike-results.md)) and the orchestrator ported to run on it, under `lsf-runtime/`. Sections are owned by units of the [LSF improvements programme](../plans/lsf-improvements-breakdown.md): Language (`runtime-language-design`), Permissions (`s2-permissions-design`), Runtime (`runtime-core`), CLI (`runtime-cli-drive`).

## Language

Unit `runtime-language-design`, plan @ `ed6f273`.

**Status:** decided, 2026-10-06. Requester verdict: **Go**, targeting Linux and macOS on x64 and arm64 first. Windows outside WSL is deferred until a unit adds a non-POSIX kill path.

### What the program does

The program has three parts:

- the runtime: the spike's [adapter requirements](../investigations/substrate-spike-results.md#adapter-requirements) plus the exemplar's caps, guard timeout, signal stop and dry run (`runtime-core`);
- the plan model: a YAML-subset parser, plan validation, the ready set and the stop rule (`orchestrator-port-plan`);
- the wave loop and its prompt builders (`orchestrator-port-loop`).

The JavaScript is about 2,350 lines with 2,200 lines of tests. The runtime is estimated at 1,500–2,500 lines including tests (spike, `conjectured`). The exemplar (`~/src/maths-proofs/tools/campaign/run.py`) already has the runtime's controls in Python stdlib: caps, SIGINT and SIGTERM handling, a guard timeout, `stop_reason` and a dry run. It is 624 lines, with 564 lines of tests.

The work is I/O-bound: concurrent `claude -p` subprocesses, line-delimited JSON on stdout, process-group kills, git, and an on-disk journal keyed by a stable hash. The logic classifies closed enumerations by kind. The stop rule reads 16 stop kinds. Alongside them are 17 record kinds, gate kinds, unit types and amendment kinds (`schema.js`), and `runtime-signals` adds driver-event kinds. Per-stage JSON Schemas go to `--json-schema` and are validated on return.

Agents write the code. The requester reviews it. Human readability is not one of the unit's criteria; the requester's stated preference order is taken as their view of it.

### Probes

Sources are in [runtime-language-probes](../investigations/runtime-language-probes/).

Each probe is the same small module, tested against one stub `claude` that emits stream-json. The tests check four cases:

- a result is parsed;
- a `success` with `structured_output: null` is classed null;
- a hung call is killed at a 1 s guard timeout, grandchild included (a process-group kill);
- eight calls run concurrently.

They do not exercise the runtime's harder parts: journal resume, orphan reaping after SIGKILL, signal handling, session-limit waits. Passing all four cases shows each toolchain works on this host. It does not separate the candidates.

Figures are from 2026-10-06 on this host (Arch Linux, x86_64), with toolchains installed in isolation where the system one would not do. Python ran on 3.14.7 only; nothing ran at a lower floor.

| | Python | Go | Rust | Haskell |
|---|---|---|---|---|
| Toolchain | system 3.14.7 | system 1.27.1 | system 1.98.1 (no rustup); rustup 1.98.1, 13 s and 577 MB to install | system GHC 9.6.6; ghcup GHC 9.10.3 and cabal 3.16.1, 123 s and 2.8 GB to install |
| Dependencies | stdlib | stdlib | `serde_json`, `libc` | `aeson`, `async`, `process`, `unix`; tests `tasty`, `tasty-hunit`, `temporary` |
| Tests | pass | pass | pass | pass under ghcup. System GHC: Cabal cannot build Hackage dependencies against Arch's dynamic-only GHC, static or dynamic; the module passed under plain `ghc -dynamic` |
| Build, cold → warm | none | 7.6 s → 1.4 s to rebuild and test after a module change | 6.0 s, crate fetch included → 1.2 s; a two-crate probe, so the runtime's fuller crate set will lengthen it | 224 s (dependencies) → 6 s, then 2 s to rebuild after a module change |
| Compile errors on first write | — | 0 | 0 | 4, in three rounds: name clashes with `Data.Aeson.Null` and `Test.Tasty.Timeout`, and `OverloadedStrings` missing twice |
| Cross-build from this host | n/a | `CGO_ENABLED=0`: linux and darwin, amd64 and arm64, static, about 4 MB each | `x86_64-unknown-linux-musl`, static, 0.8 MB, runs. `aarch64-unknown-linux-musl` fails without a cross-linker; both darwin targets fail for want of the macOS SDK | not probed |

The compile-error row is one author on one probe. It is anecdote, not a measure of agent cost.

**Windows.** The Go build for `windows/*` fails: `Setpgid` and `syscall.Kill` are POSIX. Every candidate's process-group kill (`os.killpg`, `libc::kill`, `System.Posix`) is POSIX-only, so Windows needs a second kill path, such as job objects, in any language. The spike ran on Linux only.

### Distribution

Facts from the Claude Code plugin docs (code.claude.com/docs/en/plugins, fetched 2026-10-06):

- **Install.**
  - A marketplace plugin is cloned or copied into a versioned cache directory.
  - There is no install or postinstall hook. The only automatic step is `npm install --ignore-scripts` when a lockfile is present.
  - Git LFS content is not fetched.
  - No plugin source type fetches GitHub release assets. An `archive` source (an HTTPS zip, sha256-pinned, up to 256 MiB) or a `command` source can supply a prebuilt plugin directory.
- **`bin/`.**
  - Files in a top-level `bin/` are on the Bash tool's `PATH`.
  - claude.ai and Cowork organisation sync refuse a plugin with a top-level `bin/`.
  - `${CLAUDE_PLUGIN_ROOT}` is substituted inline in skill bodies, so a skill can run `${CLAUDE_PLUGIN_ROOT}/lsf-runtime/…` without `bin/`.
- **State.** `${CLAUDE_PLUGIN_DATA}` (`~/.claude/plugins/data/<id>/`) persists across updates and is the documented home for installed dependencies. The docs' pattern installs them from a `SessionStart` hook.
- **Platforms.** Claude Code runs on macOS 13+, Linux (glibc and musl) and Windows 10+, each on x64 and arm64. A static Linux binary serves glibc and musl alike, so without Windows there are four targets: linux and darwin, each on x64 and arm64.

**Routes for a compiled runtime:**

- **D1: download on first use.**
  - A shell script at `${CLAUDE_PLUGIN_ROOT}/lsf-runtime/` downloads the platform's binary from a GitHub release into `${CLAUDE_PLUGIN_DATA}`. It checks a sha256 that is pinned in the script.
  - CI builds the binaries.
  - The user needs `sh`, `curl`, and `sha256sum` or `shasum`, and must be online at first use.
  - Every release edits the pinned hashes. A plugin version whose release is missing cannot run.
- **D2: build from source on first use,** if the user has the toolchain. Plausible only for users who already have the Go or Rust toolchain (rustup, 577 MB); that any plugin user does is unevidenced. For Haskell it is not: a 2.8 GB toolchain and a 224 s cold dependency build.
- **D3: commit the binaries.** History grows by the full binary set every release; rejected.
- **D4: ship a CI-built archive.** The marketplace entry switches to an `archive` source built by CI with every target inside. This changes how the whole plugin ships.

**Python needs no route.** It needs a `python3` on the user's machine at a version floor. Status of the floor (not probed):

- On macOS, `python3` comes with the Command Line Tools. Those tools also supply `git`, which the programme already requires, so a `python3` is likely present, though reportedly at 3.9.
- Ubuntu 20.04 and 22.04 ship 3.8 and 3.10; Debian 10 and 11 ship 3.7 and 3.9.
- A floor of 3.9 keeps the stock macOS interpreter, at the cost of `match` and runtime `X | None`. A higher floor makes macOS users install Python.
- `tools/` is already Python stdlib, by project convention.

### Assessment

| Criterion | Python | Go | Rust | Haskell |
|---|---|---|---|---|
| Plugin-user install | no build; a `python3` at the floor | D1 or D2. All four targets cross-build from one host (probed) | D1 or D2. From a Linux host, only the host arch builds as-is (probed). arm64 Linux needs a cross-linker or an arm64 runner; darwin needs a macOS runner or an SDK with `cargo-zigbuild` (unprobed) | D1. Each OS and arch needs its own CI runner (unprobed); a static Linux build needs a musl GHC (unprobed) |
| Stdlib fit | covers the program; the exemplar shows it | covers the program: `crypto/sha256`, `os/signal`, `time`, `os/exec`, `context` | lacks a stable hash (`DefaultHasher` is unspecified across releases), signal handling, calendar time and read timeouts. Needs crates such as `sha2`, `signal-hook` and `time`, or `unsafe` `libc` | `process`, `async` and `unix` cover signals and subprocesses; hashing and time need packages such as `cryptohash-sha256` and `time` |
| Agent build and iterate | no build step; type errors surface only under mypy or pyright | fast builds; a small language | builds as fast as Go's in the probe. Borrow-checker friction on this program is unprobed | heaviest toolchain (probed); the most compile errors in the probe (anecdote); fluency unmeasured |
| Types for plan, signals and stop rule | `Literal` unions, dataclasses and `match`. Exhaustiveness and strictness come from mypy or pyright with `assert_never`, as dev dependencies | no sum types: kinds are named constants (switches over them checked by the `exhaustive` linter) or sealed interfaces with a type switch (checked by `go-check-sumtype`). Decoding is strict only if written to be: `DisallowUnknownFields` plus explicit missing-field checks | enums with exhaustive `match`. `serde` decodes, strict with `deny_unknown_fields`; a missing non-`Option` field is an error | ADTs with exhaustive matching under `-Wall` (warnings) or `-Werror`. `aeson` decodes; unknown fields are ignored unless rejected |
| Subprocess, streams, concurrency | `subprocess` and threads; adequate for I/O-bound work | `os/exec`, `context` timeouts, goroutines | `std::process` and threads (or tokio); a timeout needs a reader thread | `process`, `async`, green threads |
| Test tooling | `unittest` built in | `go test` built in | `cargo test` built in | `tasty` or `hspec`, packages |

**Schema validation.** No candidate has a JSON Schema validator in its stdlib. The spike hand-rolled the subset it uses (`spike/validate.js`, 22 lines). A typed decoder checks shape but not every schema constraint, such as enums carried as strings or `minLength`, so each candidate needs the subset validator or a package. Deriving the schemas from the types (`schemars`, `autodocodec`) is unprobed. Deriving them would also replace `schema.js`'s hand-written schemas, which the port's oracle compares against.


### Weighing

**Install** favours Python outright: no build, binaries, release or first-use download. Its costs are an interpreter floor that no one has tested and optional typing (mypy). Among the compiled candidates, Go builds every target from one host (probed). Rust needs a CI runner matrix (probed). Haskell's distribution shape is unknown: not probed, rather than shown to be worse. Every compiled verdict also needs a distribution unit (see Plan consequences), which Python does not.

**Iteration** favours Python (no build) and Go (about a second to rebuild and test).

**Types** give less here than the candidates' type systems suggest. The JavaScript has no dispatch on kinds: no `switch` anywhere in `workflows/lsf/`.

- Stop kinds are classified by list membership (`STOP_KINDS.includes`, `scheduling.js:387`, and two superseding/re-plan lists in `implement.js`).
- Record kinds are picked by string filters (`scheduling.js:443–459`).

A new kind is a list entry, not an unhandled case. A port in any language can keep that shape, and then exhaustive matching catches nothing. What types do carry here is the shape of decoded agent returns and of the plan graph. That shape needs the schema validator in every candidate anyway (see Schema validation).

**Where kinds come from.** Signal kinds are set by the orchestrator's own code: 36 sites, by `grep -o "signal({" scheduling.js implement.js`. They are not decoded from agent output. The kinds agents emit (amendment kinds, gate kinds) are constrained by an `enum` in the stage schema, and a return that fails its schema is a failed attempt in every language.

A closed enum catches a misspelt kind at a construction site in Rust and Haskell. Go does not: an untyped string literal converts silently to a named `Kind` type, so the check holds only if the code uses the named constants. Python needs mypy with `Literal` for the same check. Kinds read back from the journal and ledger on resume are decoded, and a closed enum rejects an unknown one there too.

**Probe depth differed.** The probes ran subprocesses, process-group kills, timeouts and concurrency, and Go's and Rust's cross-builds. The stdlib-fit claims (hashing, signals, time) are argued from documentation, for every candidate. The probes did not separate:

- the port's harder shapes: the ledger and journal shared across parallel stages, the closure-passing `run(runtime)` and `parallel`, and dynamic JSON passthrough;
- byte-identical prompt rendering and stable hashing. These are exposed to JSON encoder defaults in every candidate: Go's `encoding/json` HTML-escapes `<`, `>` and `&`, and encoders differ in key order;
- agent fluency in each language.

No probe ported an orchestrator slice.

### Decision: Go

The requester's words (2026-10-06): "Perhaps Haskell. Rust or go possibilities. Python if we must." This record does not infer reasons for that order.

Go was recommended, and chosen, on the probed cross-builds and iteration speed, and on a stdlib that, by documentation, covers the runtime with no dependencies. That departs from the first-named option, Haskell. Haskell's case would rest on types, which this code uses little (see Weighing), and its distribution shape is unprobed.

Go's type weaknesses (no sum types, lenient decoding by default) fall on code that classifies by list membership. Strict decoding comes from the schema validator plus `DisallowUnknownFields`.

The decision carries the cost of the distribution unit (`runtime-distribution`): CI for four targets, a release, and the D1-or-D4 choice.

**Alternatives put to the verdict:**

- **Python, if no distribution step is wanted.** Python's install advantage is real. It is traded away here for the requester's ordering ("if we must") and for typing that is checked only when mypy runs.
- **Rust or Haskell, if strong types are wanted for their own sake.** Either means accepting a CI runner matrix and crates or packages for hashing, signals and time. Rust has the smaller toolchain (577 MB against 2.8 GB) and the faster cold build (6 s against 224 s), and its static Linux build is probed. Haskell's static build and CI shape are unprobed.

**Toolchain pin (`lsf-toolchain`).**

- `go.mod` declares `go 1.27` and `toolchain go1.27.1`. These are fetched automatically unless `GOTOOLCHAIN=local`; it is `auto` on this host.
- Stdlib only. A dependency is a decision recorded in the unit that adds it.
- Lint: `go vet`. Tool dependencies such as linters are pinned with `tool` directives in `go.mod`.

**Layout.** A single Go module, `lsf-runtime/go.mod`, with one package per subdirectory the plan names.

| Directory | Holds |
|---|---|
| `plan/` | the port of `schema.js`, `graph.js` and `scheduling.js` |
| `runtime/` | headless calls, the journal, roles and caps, and the runtime interface (`agent`, `parallel`, `log`, `args`) that the orchestrator takes and tests fake |
| `orchestrator/` | the wave loop and prompt builders; depends on `plan` and `runtime` |
| `cli/` | the one executable, `lsf-runtime` |
| `testdata/` | the stub `claude` |
| `tools/` | the two port units' cross-language comparison scripts, in Go; they run `node` on the JavaScript side until `script-retirement` |

- `runtime` does not depend on `plan` until `runtime-signals` turns driver events into signal kinds; that unit edits both.
- `go test ./...` from `lsf-runtime/` is the suite that `script-retirement` and `programme-close` name.
- The `runtime/` directory's package is named `lsfrt`, so it does not shadow the stdlib's `runtime`.

### Plan consequences

- **Distribution.** The route, the CI and the release are owned by `runtime-distribution`, which runs before `script-retirement` and chooses between D1 and D4:
  - **D1** keeps the git marketplace source. It needs a first-use download and a pinned hash per target. Its release order is: build at commit X, release, then commit the hashes at X+1 and version the plugin there.
  - **D4** needs no network at first use and no hash commit. The cost is that the whole plugin ships as a CI-built archive.
  - **Development (either route).** `claude --plugin-dir` and the shakedown fixture must run the working tree's build, not a release. The launcher prefers a locally built binary, or one named by an environment variable, over a downloaded one.
- **Shared root files.**
  - The skeleton (`go.mod`, the four package stubs and `testdata/claude`) is committed at `c3330d2`, so `runtime-core` and `orchestrator-port-plan` run side by side.
  - `testdata/` is in `runtime-core`'s Scope, and `tools/` in the two port units', which are serial.
  - `go.mod` changes only when a unit adds a dependency or a `tool` directive, which that unit records as a decision.
- **No top-level `bin/`.** Skills invoke the runtime through `${CLAUDE_PLUGIN_ROOT}`, so organisation-sync installs keep working.

## Runtime

Unit `runtime-core`, plan `docs/plans/runtime-core-plan.md`. Package `lsfrt` in `lsf-runtime/runtime/`; tests run against the stub `lsf-runtime/testdata/claude` and streams recorded from Claude Code 2.1.289 (`testdata/streams/`).

### Interface

- **`Run(ctx, cfg, body)`** is one launch. It takes an `flock` on the state dir, reaps orphans, installs the signal handlers, runs `body(ctx, rt)`, and writes `summary.json`. Its error is for setup failures only; how the body ended is the summary's `stop_reason`.
- **`rt.Agent(ctx, prompt, opts)`** is the orchestrator's `agent()`. `(nil, nil)` is null. The only error is a `*StopError`, which the orchestrator propagates.
- **`rt.Call`** is the same call, returning a `CallResult`: the value, the last attempt's class, flags such as `skill-not-loaded`, permission denials, attempts, cost, and whether it was replayed. It is the channel through which `runtime-signals` reads driver events.
- **`rt.Parallel(ctx, thunks)`** runs every thunk. A branch's error or panic becomes a nil slot. A stop in any branch is returned once all branches finish.
- **`rt.Step(ctx, label, input, fn)`** journals a code step (see Journal).
- **`rt.Log`** and **`rt.Args`**: `Args` is the config's `args`, passed through.

`opts` mirrors the JavaScript: `label`, `phase` (telemetry only), `model`, `effort`, `agentType`, `isolation`, `schema`.

### Configuration

The JSON config is decoded strictly. `testdata/config.example.json` carries the spike's roles and rules as a worked example; it is not a default.

- **Roles.** Each role is `tools`, `allow`, `deny` and `permissionMode`. They become `--tools`, `--allowedTools`, `--disallowedTools` and `--permission-mode`. The runtime has no built-in role; `s2-permissions-design` decides their content.
- **Role rules.** `roleRules` map `agentType` and a label regex (both must match when both are set) to a role. The first match wins; otherwise `defaultRole` applies.
- **Skills.** `skills` map a label regex to a skill. A matching call's prompt is sent as `/<skill> <prompt>`.
- **Caps** are per launch: `maxCalls` (spawns), `maxWallClock`, `maxCostUSD` (summed `total_cost_usd`; in-flight calls can overshoot).
- **Timing.**
  - `guardTimeout`: per attempt, none by default.
  - `killGrace`: 10 s.
  - `backoff`: 5 s, 20 s, 60 s.
  - `sessionLimitMargin`: 30 s past the reset.
  - `sessionLimitFallback`: 15 min when the stream gives no reset.
  - `maxSessionWaits`: 4 per call.
- **`dryRun`.** The first call that would spawn logs its argv and stops the launch with `dry-run`. A step that is not replayed stops without running. Orphans are reported, not killed.

### A call

The prompt goes on stdin. Every call passes `-p --output-format stream-json --verbose --session-id <uuid> --strict-mcp-config --json-schema <schema>`, plus `--model`, `--effort` and the role's flags. Each call runs in its own process group.

Order inside a call:

1. The journal is checked; a replay returns at once and does not count against the caps.
2. The stop is checked.
3. The dry-run check runs.
4. The worktree is created, if the call is isolated.
5. Each attempt: wait out the session-limit gate, take a concurrency slot, then admit or stop in one critical section (stop, parent context, gate again, caps), then spawn.

Classification reads the `result` event first, because the CLI can recover from a mid-stream error and still return a valid result. First match wins:

| Class | When | Then |
|---|---|---|
| `stopped` | a repeated signal cancelled the attempt | the stop |
| `timeout` | the guard timeout fired | null |
| `spawn-error` | the process did not start | null |
| `ok` | valid `structured_output`; skill loaded or none required | the value |
| `skill-missing` | as `ok`, but no skill load | retry; on the second miss, the value, flagged `skill-not-loaded` |
| `invalid` | clean result, `structured_output` missing, null or failing the schema | retry, then null |
| `session-limit` | no clean result, and an assistant `error: rate_limit` | wait until `resetsAt` (from a `rejected` `rate_limit_event`, else the fallback) plus margin, then retry; null after `maxSessionWaits` |
| `permanent` | assistant error `model_not_found`, `authentication_failed`, `billing_error`, `invalid_request` and the like, or status 400/401/403/404 | null |
| `transient` | anything else | back off, then null |

- `invalid` and `skill-missing` share one retry.
- A session limit moves a runtime-wide gate that every call waits on. It is assumed to apply across models. It does not spend the transient budget.
- The schema validator covers the subset the stage schemas use: `type`, `properties`, `required`, `items` and `enum`, with JavaScript's integer and number-equality semantics.
- **Skill load.** A slash-command expansion is not echoed on stdout. In probes, the init event's `slash_commands` listed the command exactly when it expanded, and an unknown command was sent as plain text. So a prefixed call loaded its skill if the init event lists it. A `Skill` tool call naming it also counts.
- **Guard timeout, or a second signal.** The process group gets SIGTERM, the leader SIGKILL after `killGrace`, and the group SIGKILL once the leader is reaped. This reaches a grandchild that ignores SIGTERM. A call that exits normally is not group-killed.

### Journal and state dir

| Path | Holds |
|---|---|
| `lock` | the launch's `flock` |
| `journal/<key>.json` | a completed call's or step's record: label, kind, input hash, return, flags |
| `journal/<key>.intent` | a step's input hash while it runs |
| `calls/<key>.<launch>.a<n>.<session>.jsonl` | each attempt's stream |
| `children.json` | session id → pid of live calls |
| `wt/<key>` | isolated calls' worktrees, on `lsf/<plan8>/<key>` |
| `status.json`, `summary.json`, `log.txt`, `telemetry.jsonl` | launch state, the `stop_reason`, the log, one event per line |

- **Keys.** `<key>` is the label made filename-safe plus 8 hex digits of its hash. Labels must be unique per call: the journal keeps one record per label, and a label reused with other inputs in one launch is logged `label-reused`.
- **Call records.** A call's hash covers the prompt (before the skill prefix), the schema, the model and the effort. Role and permissions are excluded, so changing them does not re-run completed work. A null return is not recorded, so a resume retries it.
- **Step records.** A step writes an intent, runs, records its return (null included) and removes the intent. A step that errors keeps its intent, so the next launch reruns it with `interrupted=true` for its own effect check.
- **Replays.** A replayed value is byte-identical to the live one (compacted JSON).
- **File writes.** Records, intents, `children.json`, the status and the summary are written by temp file and rename.

### Supervision

- **Signals.** The first SIGINT or SIGTERM sets the sticky stop `signal-SIGINT` or `signal-SIGTERM`. In-flight attempts finish and their results are journaled. Retries, backoff and gate waits are abandoned, and every later boundary returns the stop. A second signal cancels in-flight attempts.
- **`stop_reason` precedence.** A stop the runtime recorded wins, even if the body swallowed it. Then a `StopError` from the body, then `cancelled` (the parent context), then `crashed` (a body error or panic), then `returned`.
- **Orphans.** At launch, every process whose arguments carry `--session-id <id>` for an id in `children.json` is a call left by a killed launch. Its group gets SIGTERM, then SIGKILL after `killGrace`. If `ps` fails, `children.json` is kept for the next launch. A torn `children.json` is reported and treated as empty.
- **Worktrees.**
  - Creation is serialised.
  - An existing `wt/<key>` is reused only if it is a worktree on its branch; otherwise the run stops with `runtime-error`.
  - A branch whose directory is gone is re-attached, not reset.
  - Missing `planSha` or `integrationBranch` in `args` stops the run with `runtime-error`.

### Not in this unit

- Checking returned SHAs: `code-stage-interface` (rejection) and `runtime-signals` (the corrected-SHA signal).
- Gates on a clean checkout: `declared-gates`.
- Driver events as ledger signals (uncommitted at return, denials, `skill-not-loaded`): `runtime-signals`, through `CallResult`.
- Cleanup's ignored-file policy: `code-stage-interface`.
- A stop file, status readers and detaching: `runtime-cli-drive`.
- The roles' content: `s2-permissions-design`.

### Known limits

- **Session limits.** The shape of a session limit under `-p` is built from the binary's schema and interactive transcripts, not observed. The first real limit's stream is kept under `calls/`.
- **Retries in a dirty worktree.** A retry after a worker made partial changes reruns in the same worktree.
- **Group kill after reaping.** After the leader of a cancelled call is reaped, its pgid could in principle be reused before the group SIGKILL. Sequential pid allocation makes this need a wrapped pid space.
- **Orphan grandchildren.** An orphan whose group leader is gone is not signalled.
- **macOS.** `ps -A -ww`, `flock` and process groups have not been run on macOS.
