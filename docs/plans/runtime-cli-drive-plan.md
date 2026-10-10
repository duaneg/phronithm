# runtime-cli-drive — plan (phronithm:feature, work-unit mode, compressed Phases 1–3)

Base commit `b56f427` on `lsf-workflow`. Saved after approval to `docs/plans/runtime-cli-drive-plan.md`.

## Context

The Go runtime (`lsf-runtime/runtime`, `orchestrator`) runs Phase 3 end to end, but only `tools/fixturerun` launches it. `lsf-runtime/cli/main.go` is an empty stub, and `phronithm:lsf-drive` knows only the Workflow tool. This unit builds the `lsf-runtime` CLI (launch, status, pause, resume, watch, plus stop) and gives `phronithm:lsf-drive` a CLI route beside the Workflow route. It also disposes of P-F1 (relay of the launch turn's message to agents): headless calls carry no relay, so the evidence is a fixture run launched from a turn the requester typed in.

## Problem statement

### Unit entry (verbatim, `docs/plans/lsf-improvements-breakdown.md` @ `b56f427`)

### runtime-cli-drive
- **Type**: integration
- **Risk**: medium
- **Clarity**: exploratory — one open decision: how pause and status are exposed (pause ref, kill file, state file)
- **Iteration**: write-once
- **Effort**: medium
- **Scope**: `lsf-runtime/cli`, `skills/lsf-drive/SKILL.md::launch`, `skills/lsf-drive/SKILL.md::relaunch`, `skills/lsf-drive/SKILL.md::startup`, `docs/design/lsf-runtime.md::cli`
- **Publishes**: the runtime CLI (launch, status, pause, resume, watch)
- **Depends on**: s2-sandbox (logical, the container launch, its configuration and launch checks); s2-egress-gates (logical, the egress proxy a launch holds and the bounded gates); s2-boundary-decision (design, the decided boundary); plan-preflight-check (logical, serialises `skills/lsf-drive/SKILL.md::startup`); relay-probe (knowledge, relay behaviour); runtime-core (logical, `Run` and the state-dir layout: `status.json`, `summary.json`, `lock`); orchestrator-port-loop (logical, `orchestrator.Body`; knowledge, `tools/fixturerun`'s launch sequence)
- **External gates**: `claude-code-headless` (environment, Claude Code 2.1.294); `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`); `git` (environment, 2.38 or later; 2.54.0 probed)
- **Done condition**: `phronithm:lsf-drive` launches, pauses, resumes and reads the status of a run through the CLI, under the decided permission configuration. A fixture run launched from a turn in which the requester typed a message shows no agent transcript carrying a relayed message, cited by transcript (P-F1). The driver outlives the interactive session that launched it, or the skill says it runs from the requester's terminal. The Relaunch section keeps `gate-not-run`'s route: an environment or project change, not a decision, whether a stage or the runtime's own gate run raised it. It keeps the `declared-gates-changed` row as `s2-boundary-common` rewrote it: the old and new blocks shown, a relaunch at the escalation's plan SHA only on the requester's explicit approval, otherwise a commit restoring the block and a relaunch at its SHA; no decision changes the block. It gives the route for a code-stage integrity stop, which repeats at every launch until a requester repair commit lands: the repair commit, and exempting the code stages' paths from any hook that rewrites or stages files. Its stop-kind table lists `auth-failed`, answered by renewing the credential and relaunching, `uncommitted-at-return`, `skill-not-loaded`, `schema-violation`, `gate-omitted`, `stage-fault` and `branch-mismatch` with the routes `lsf-workflow.md`'s Relaunch target gives; `uncommitted-at-return` is answered by committing or removing the named paths, not by a decision, and the section says a requester edit in the main checkout stops the run at the next fast-forward. It names the merge faults the requester clears by hand, as the runtime raises them after `s2-clones` moved the merge into a merge clone, each by the remedy its statement gives: a recorded merge SHA that no longer resolves after a history rewrite, a stale `index.lock`, a foreign merge in progress in the main checkout, paths a hook staged after a merge, a `pre-merge-commit` hook included, and a red gate result that the environment caused, which replays from the journal until `journal/gates_<tree prefix>-*.json` is removed (requester decision, 2026-10-08). It names the clone faults the requester clears by hand, each by the remedy its fault text gives: a writer clone or merge clone a fault left, a merge clone whose integration branch diverged from main's, a kept clone left on another branch, a fetch-back failed by a branch the agent renamed, and a fetch-back refused because the branch's tree holds a gitlink (a nested repository the agent committed; requester decision, 2026-10-10: stays `runtime-error`, cleared by hand). Status reports the state dir's `clone/`, `scratch/`, `merge/` and `*.partial` directories, a `refs/lsf/<plan8>/xfer/w<n>` ref present in the main repo, and containers carrying the state dir's label. The CLI's configuration carries the container configuration `s2-sandbox` adds (`image` by digest, `claudeBin`, `claudeVersion`, `podmanBin`, `pluginDir`, `proxy`, `env` passthrough), and the runtime's refusal of an older configuration (one carrying `allow`, `deny` or `permissionMode`) or a launch-check refusal, a dry run's included, reaches the requester with its message. The CLI fills `claudeBin` once per launch (configuration, `LSF_CLAUDE_BIN` or `PATH`), resolved to its versioned path, with `claudeVersion` to match; a test shows a binary replaced mid-launch is not used (requester decision, 2026-10-06). The configuration's documentation says `setup` and gate commands see only the image's environment and the `env` passthrough, so a private registry's credential is passed by name through `env`, and that their toolchain must be in the image. A launch refuses a `git` older than 2.38, naming the version found (requester decision, 2026-10-07). The tests pass.
- **Description**: Replaces `relay-mitigation`: headless calls carry no relay (results, check 3), so P-F1 reduces to the launch path. `tools/fixturerun` is a working launch: configuration from a file, args, `DISABLE_AUTOUPDATER=1` with a versioned `claude` checked before spending, a fresh state dir, then `lsfrt.Run(ctx, cfg, orchestrator.Body)`. A code-stage stop arrives as `stage-fault` with the statement `"<stage> failed: <reason>"`. Under 2.1.294 a 401 is retried ten times, about 3 minutes, before `auth-failed`. Since `s2-sandbox`, a launch, a dry run included, needs rootless podman and the image (smoke container), refuses to start if reaping fails, and the runtime checks `claudeVersion` against the smoke container's `claude --version`. **Flag:** three surfaces exceed the surface bound; the skill and design changes are sections of text, so the excess is not context.

Standing obligations naming this unit:
- **Register dispositions**: P-F1.
- **Headless pin**: checked against `~/.local/share/claude/versions/2.1.294` run with `DISABLE_AUTOUPDATER=1`, or by a transcript's `version`; `claude --version` does not discharge it.
- **Design of record**: the unit amends `lsf-runtime.md::cli` in the same commit as the behaviour.
- R35: "the tests pass" needs a quiet-machine run of the suite.

### Requester decisions (2026-10-10, this session)

1. **Both routes.** `phronithm:lsf-drive` gains a CLI route beside the Workflow route; it uses the CLI when a runtime host config exists and the Workflow otherwise. `script-retirement` deletes the Workflow route.
2. **Detached driver + `watch`.** `launch` re-execs itself in a new session and returns once the launch checks have passed or failed. `watch` blocks until the driver exits; the skill runs it as a background Bash, so the session is re-invoked when the run ends.
3. **Pause = ref, stop = signal** (the unit's open decision). Pause stays the pause ref the ported orchestrator already reads. A new `stop` sends SIGTERM to the driver. `status` reads `status.json`, `summary.json` and the lock.
4. **Plugin defaults + host file.** The plugin ships the label-bound config (roles, roleRules, skills, defaultRole); the user's host config supplies the host settings and may override them. The CLI fills repo, stateDir and args per launch.

### Success criteria (target rigour)

| # | Criterion | Target |
|---|---|---|
| S1 | The unit's done condition, verbatim above, in its parts S1a–S1k below | as below |
| S1a | `lsf-drive` launches, pauses, resumes (a paused run, through `launch -args` from the outcome) and reads status through the CLI; `resume` (replay) proven by tests | evidenced (a fixture run driven by the skill's commands; skill text cannot be automated) |
| S1b | P-F1: no call transcript of a fixture run launched from a requester-typed turn carries the message or the relay marker `Workflow harness` | evidenced (paid run, cited transcripts) |
| S1c | The driver outlives the launching session (detached, own session) | proven (test: driver keeps running after the `launch` process exits; it is in a session of its own) |
| S1d | Relaunch section content: `gate-not-run`, `declared-gates-changed`, code-stage integrity stop, stop-kind table, merge and clone faults | evidenced (critique-skill pass and a read-through against the fault texts in code) |
| S1e | Status reports `clone/`, `scratch/`, `merge/`, `*.partial`, xfer refs and labelled containers | proven |
| S1f | Config carries the container fields; the old-config and launch-check refusals (dry run included) reach the requester with their message | proven |
| S1g | `claudeBin` filled once per launch, resolved to its versioned path, `claudeVersion` to match; a binary replaced mid-launch is not used | proven |
| S1h | Config documentation: setup and gates see only the image's environment and `env`; toolchain in the image | evidenced (doc text) |
| S1i | A launch refuses git < 2.38, naming the version | proven |
| S1j | `go test -race ./...` passes, on a quiet machine (R35) | proven |
| S1k | Headless pin confirmed | evidenced |

### Scope adjustments (deviations from the entry, for the completion report)

- `stop` is added to the published subcommands: the CLI route needs a halt to replace TaskStop (decision 3).
- `skills/lsf-drive/SKILL.md` *Pause*, *Read the return* and *Recovery* get a few lines each for the CLI route (`pause`, `stop`, the return in `outcome`, `resume`). They are outside the entry's `::launch/::relaunch/::startup` scope, but a CLI route without them leaves the skill inconsistent. `drive-mechanics` owns the whole file later.
- `CLAUDE.md` Structure: `cli/` stops being "still empty" (feature Phase 5, step 4).
- The CLI's embedded defaults copy the roles, rules and skills of `testdata/config.example.json`. A test keeps the two equal; `fixturerun` is not changed.

### Exploration findings

- `lsfrt.Run(ctx, cfg, body)` (`runtime/runtime.go`) does the prepare, repo check, state dir (0700, symlinks resolved), the `flock` on `lock` (LOCK_EX|NB; a held lock is a setup error), the launch check (`checkContainerHost` + smoke container, ≤2 min), signal handlers, orphan reaping, then the proxy. It writes `status.json` `{pid,state:"running",at,launch}`, runs the body, writes `summary.json`, then `status.json` with `state` = stop_reason. Setup failures are returned errors; how the body ended is `Summary.StopReason`.
- `LoadConfig` decodes strictly; old-config keys fail as `unknown field "allow"` etc. (`runtime/config_test.go:208-210`). `prepare` applies `LSF_CLAUDE_BIN` over the config. `Run` resolves `ClaudeBin` again with `resolveBin` (unexported; `LookPath`, `EvalSymlinks`, regular file, no `:`).
- `orchestrator.Body` reads args (`orchestrator/args.go`): required `name`, `planSha`, `architectureDoc`, `integrationBranch`; `pauseRef` defaults to `refs/lsf/<name>/pause`. The pause ref is read live after steps (`stages.go: liveReads`); a set ref returns `paused` at the wave boundary (`wave.go`). Load, decision log, ledger and bookkeeping are git code steps, so a launch paused at load with no decisions spawns no agent.
- The outcome in `summary.json` is the same object the Workflow return carries (`status`, `workflow`, `escalation`, `digest`, `planSha`, …).
- `lsfrt.StateLabel(stateDir)` gives the `lsf.state` label value. Test stubs: `testdata/claude` (`--version` → `0.0.0-stub (Claude Code)`), `testdata/podman` (`run` execs on the host; `ps`, `rm`, `info`, `image exists`; `STUB_PODMAN_NOIMAGE`, `STUB_PODMAN_SMOKE_EXIT`), and `internal/testcfg` (image, version, plugin dir, credentials).
- Images present: `localhost/lsf-ref@sha256:949f2220…`, `localhost/lsf-fixture@sha256:344c825a…` (the node-extended fixture image).
- `runtime-distribution` owns the launcher. Until then the skill invokes the CLI as `go -C ${CLAUDE_PLUGIN_ROOT}/lsf-runtime run ./cli`.

### Candidate approaches (decided by the requester; see decisions)

- Detach: (A) self re-exec with `Setsid` + `watch` on the lock, chosen; (B) the requester's terminal, foreground, rejected: no re-invocation, and the session would only see status on request.
- Pause/stop: (A) pause ref + SIGTERM, chosen: the mechanisms already exist in the runtime and the ported orchestrator; (B) a stop file polled by the runtime, rejected: it needs runtime hooks and changes the orchestrator's pause reads, which are frozen to JS parity until `script-retirement`.
- Config: (A) embedded plugin defaults + host file, chosen; (B) one user-maintained file, rejected: plugin role changes would need hand copying.

## Design

### Commands

Binary `lsf-runtime` (`lsf-runtime/cli`, package `main`). A run is addressed by `-state DIR`, or by `-repo DIR` (default: the toplevel of the cwd's repo) plus `-name NAME`. The default state dir is `${XDG_STATE_HOME:-$HOME/.local/state}/lsf-runtime/<first 12 hex of sha256(repo toplevel, symlinks resolved)>/<name>`. `name` must match `^[A-Za-z0-9][A-Za-z0-9._-]*$`, and `refs/lsf/<name>/pause` must pass `git check-ref-format`. `launch` prints the state dir, and the skill passes `-state` to every later command.

| Command | Does |
|---|---|
| `launch -args FILE\|- [-repo] [-state] [-config FILE] [-plugin-dir DIR] [-dry-run]` | Pre-checks, composes the config, starts the detached driver, waits (≤5 min) until it has passed or failed the launch checks. `-dry-run` runs in the foreground against a scratch state dir and prints the summary. |
| `resume [addr] [-config] [-plugin-dir]` | Replays the last launch: its stored args, re-composed against the current host config, so a renewed credential or a raised cap applies. The journal replays completed calls. It serves runs that ended with no outcome to act on (a runtime stop, a signal, a cap, a crash, a failed start). It refuses when the last summary is `returned`: an outcome (paused, wave-cap, stopped) is relaunched with `launch -args`, from the outcome's `planSha` and with its `unscribed`, as the skill's Relaunch says. It refuses while a driver runs. |
| `watch [addr]` | Blocks until no driver holds the lock, then prints the status report. |
| `status [addr]` | Prints the status report (JSON) without blocking. |
| `pause [-lift] [addr]` | Sets the pause ref to the repo's HEAD (`git update-ref`), or with `-lift` deletes it, then verifies with `git show-ref --verify`. It prints that the pause is honoured at the next wave boundary, not at once. |
| `stop [addr]` | SIGTERM to the running driver. The first lets in-flight calls finish, which can take as long as the longest call; a second `stop` cancels them. |
| `drive -state DIR -launch FILE` (hidden) | The driver: `LoadConfig(FILE)`, then `lsfrt.Run(ctx, cfg, body)`, where `body` first renames FILE to `<state>/launch.json` (the lock is held by then), then calls `orchestrator.Body`. |

Exit codes: 0 success; 1 a refusal or error, with its message on stderr; 2 usage. For `watch` and `status`: 0 for `ended`, 3 for `crashed` or `failed-to-start`, 4 for `none`. The status JSON is authoritative. The skill runs a built binary, not `go run`, since `go run` reports every non-zero exit as 1: at Startup, `go -C ${CLAUDE_PLUGIN_ROOT}/lsf-runtime build -o ${XDG_CACHE_HOME:-~/.cache}/lsf-runtime/lsf-runtime ./cli`. This is interim until `runtime-distribution`'s launcher. It also gives the detached driver a stable executable.

### Config composition (`compose`)

1. Defaults: `cli/defaults.json`, embedded with `go:embed` — `defaultRole`, `roles`, `roleRules`, `skills`, as in `testdata/config.example.json`.
2. Host file: `-config`, else `${XDG_CONFIG_HOME:-$HOME/.config}/lsf-runtime/config.json`. A missing file is refused, naming the path and the doc section. It is decoded as a JSON object; the keys `stateDir`, `repo`, `args` and `dryRun` are refused ("set by the CLI"). Each top-level key replaces the default's.
3. CLI fields: `repo`, `stateDir`, `args` (from `-args`), `dryRun`; `pluginDir` from `-plugin-dir` when given (it beats the file); `claudeBin` and `claudeVersion` (below).
4. The host file's relative `pluginDir` and `claudeBin` (when it holds a `/`) are made absolute against the host file's directory. The merged object is written to `<state>/launch.<launcher pid>.json` and loaded with `lsfrt.LoadConfig` (strict; this runs `prepare`). The driver renames it to `launch.json` once it holds the lock, so a refused or failed launch never replaces the last good `launch.json`; `launch` removes its file on failure. A load error goes to the requester as is. When it names `allow`, `deny` or `permissionMode`, the CLI adds: "this config predates the container boundary (lsf-runtime.md, Permissions); remove the key".
5. Args: a JSON object whose `name` is a string matching the name rule, and equal to `-name` when both are given. The orchestrator checks the rest.

**claudeBin, once per launch.** Source: `LSF_CLAUDE_BIN`, else the host file's `claudeBin`, else `claude` (the same precedence as `prepare`). It is resolved by `LookPath`, `filepath.Abs` and `EvalSymlinks` to a regular file: the versioned path. That path is run with `--version` and `DISABLE_AUTOUPDATER=1`, under a 30 s bound, and the first field is the version. If the host file sets `claudeVersion` and it differs, the launch is refused, naming both; otherwise the field is filled. `launch.json` stores the resolved path. The driver's environment drops `LSF_CLAUDE_BIN` and sets `DISABLE_AUTOUPDATER=1`, so the driver's own resolve (`Run`) finds the same file. Repointing `claude` after `launch` resolved it changes nothing. `resolveBin` is unexported in `lsfrt`, so the CLI keeps its own ten-line copy rather than widen the runtime's API outside this unit's scope.

**git ≥ 2.38.** `git version` is parsed (`git version X.Y[.Z]…`). Below 2.38, or unparseable, the launch is refused: `git X.Y.Z is older than 2.38 (git merge-tree --write-tree)`. This runs before composing.

### launch → driver → watch

1. Pre-checks: args and the name rule; git version; `MkdirAll(state, 0700)`; no driver running on the state dir (lock probe, below); compose.
2. Open `<state>/driver.log` (append, 0600) and note its size as the offset.
3. Start `os.Executable()` with `drive -state <state> -launch <file>` and `SysProcAttr{Setsid: true}`: stdin `/dev/null`, stdout and stderr to `driver.log`, the environment adjusted as above. Write `<state>/driver.json` `{pid, started, launchFile}` (temp file and rename). This is the CLI's own record of the current launch, and `status` and `stop` key on it.
4. Wait, polling `status.json` every 200 ms, until one of:
   - its `pid` equals the child's: the launch checks passed. Print `{"stateDir","pid","launch"}` and exit 0.
   - the child exits (reaped by a `cmd.Wait` goroutine): re-read `status.json` first. If its pid is the child's, the run started and already ended (a pre-paused run can finish within one poll), which counts as success. Otherwise it is a setup refusal (old config, launch check, reaping, proxy): print `driver.log` from the offset to stderr and exit 1. A refusal reading "in use by another launch" while no driver runs (a `status` or `watch` held the lock for an instant) is retried once.
   - 5 minutes pass: print "driver <pid> still starting; see `status`" and exit 1. The driver is left running.
   - SIGINT to `launch`: leave the driver running and say so.
5. `-dry-run`: no detach. The state dir is the scratch sibling `<state>.dry-run`, wiped first, so the run's `launch.json`, `status.json` and `summary.json` stay as they were. `LSF_CLAUDE_BIN` is unset in-process before `Run`. A setup error is printed and exits 1. The summary is printed, and the exit is 0 only when `stop_reason` is `dry-run`.

**Lock probe.** A non-blocking `flock(LOCK_SH)` on `<state>/lock`, released at once, fails while a driver holds `LOCK_EX`. It can collide with a driver's `LOCK_EX|LOCK_NB` for an instant; `launch`'s retry covers that.

`watch`: open `<state>/lock`, take a blocking `flock(LOCK_SH)` (granted when the driver's `LOCK_EX` is released, at exit or crash), release it, and print the report.

### Status report (JSON, stable keys)

```
{ "stateDir", "repo", "name",
  "driver": "starting" | "running" | "ended" | "crashed" | "failed-to-start" | "none",
  "driverLog": <driver.log tail, for failed-to-start and crashed>,
  "pid", "launch", "state", "at",               // from status.json
  "summary": {stop_reason, detail, error, calls, cost_usd, started, finished},   // when ended
  "outcome": <summary.json outcome, raw>,       // when ended
  "pause": {"ref", "set"},
  "leftovers": {"clone": [], "scratch": [], "merge": [], "partial": []},
  "xferRefs": [],
  "containers": [], "containersError": "" }
```

- `driver`, from `driver.json` (D), `status.json` (S) and the lock probe:

  | Lock | D | S's pid | S's state | `driver` |
  |---|---|---|---|---|
  | — | none | — | — | `none` |
  | held | yes | ≠ D's pid | — | `starting` |
  | held | yes | = D's pid | — | `running` |
  | free | yes | = D's pid | not `running` | `ended` |
  | free | yes | = D's pid | `running` | `crashed` |
  | free | yes | ≠ D's pid | — | `failed-to-start` |

  `summary` and `outcome` are shown only for `ended`, so the previous launch's outcome is never reported as the current one.
- `leftovers`: the entries of `clone/`, `scratch/` and `merge/`; `partial` is every `*.partial` under `clone/`, `scratch/`, `merge/` and `gates/`.
- `xferRefs`: `git -C <repo> for-each-ref --format=%(refname) refs/lsf/`, filtered to `^refs/lsf/[^/]+/xfer/w[0-9]+$`.
- `containers`: `<podmanBin> ps -a --filter label=lsf.state=<StateLabel> --format {{.Names}}`. A failure is reported in `containersError` and is not fatal.
- `repo`, `podmanBin` and `pauseRef` come from `launch.json`; without one, from the flags and the default ref.

### stop

`stop` signals only when the status is `running`: the lock is held and `status.json`'s pid equals `driver.json`'s. `status.json` is written after `Run` installs its signal handlers, so a SIGTERM is always caught. Before signalling, `/proc/<pid>/cmdline` must also be `drive -state <state>`. Then `kill(pid, SIGTERM)`. During `starting`, `stop` refuses with "driver is starting (launch check); retry", since the default SIGTERM action would kill the driver with no summary. Every other status refuses: there is no driver.

### Docs and skill

- **`lsf-runtime.md` — new `## CLI` section** (the `::cli` scope): the commands; the config layering and the host file's location; the configuration notes `s2-sandbox` and `s2-egress-gates` hand on (container fields, `setup` and gate commands see only the image's environment and the `env` passthrough, so a private registry's credential is passed by name through `env`, and their toolchain must be in the image); the state-dir default and the CLI's files (`launch.json`, `driver.log`); detach and watch; the status fields; pause and stop; claudeBin resolution; the git check.
- **`lsf-drive` SKILL.md**:
  - **Startup:** route choice (the CLI when the host config exists), shared steps 1–3, 5 and 6; step 4 is the Workflow route only; for the CLI route, `status` shows no running driver for the name.
  - **Launch:** a CLI subsection. Args JSON with the same keys, via `-args -` and a heredoc, `-plugin-dir ${CLAUDE_PLUGIN_ROOT}`; record the state dir; run `watch` as a background Bash; don't poll. The driver outlives the session: a new session runs `status`, then `watch` again while the driver runs.
  - **Relaunch:**
    - the existing table gains the rows below. Routes are taken from `lsf-workflow.md`'s *Relaunch target*.
      - `auth-failed`: renew the credential, then `resume`.
      - The other runtime `stop_reason`s: `runtime-error` (the detail names the fault: clear it by hand, then `resume`); `signal-*` (`resume`); `max-*` (raise the cap in the host config, then `resume`); `crashed` and `failed-to-start` (give `driver.log` and `log.txt` to the requester; resume only on their instruction).
      - Escalation kinds: `uncommitted-at-return` gets its own row (commit or remove the named paths; a relaunch with no decision); `skill-not-loaded`, `schema-violation`, `gate-omitted`, `stage-fault` and `branch-mismatch` join the same-workflow row; the code-stage integrity stop gets its route.
      - Step 5 ends with a check: the table's kinds are diffed against the done condition's list.
    - The outcome-bearing returns (paused, wave-cap, stopped) relaunch with `launch -args`, from the outcome's `planSha` and with its `unscribed`, never with `resume`.
    - a requester edit in the main checkout stops the run at the next fast-forward;
    - the hand-cleared merge faults and clone faults, each with the remedy from its statement in code;
    - `gate-not-run` (a stage or the runtime's gate run) and `declared-gates-changed` kept.
  - **Pause / Read the return / Recovery (deviation):** CLI equivalents in a line or two each.

### Error handling

Every refusal names its cause and exits 1 before any spawn. The driver's own setup errors reach `launch` through `driver.log`. `status` never fails on a missing piece: it reports `none`, or puts an error string in the field. `stop` refuses without a held lock.

### Risks and unknowns

| Risk | Resolution | When |
|---|---|---|
| Background Bash in Claude Code may be killed at session end, and its behaviour for a multi-hour `watch` is assumed (unverified) | The driver is in its own session (`Setsid`); only `watch` dies, and a new session runs `status`, then `watch` again. S1c's test covers the driver; step 6 exercises the background `watch` once | steps 2, 6 |
| The skill's foreground `launch` vs the Bash tool's timeout | `launch` bounds itself at 5 min; the skill gives the call a 600000 ms timeout | step 5 |
| The skill detects the CLI route only at the default host-config path | The skill also takes a host config path from the requester, passed as `-config` | step 5 |
| A pid match against stale state, or pid reuse | `driver.json` names the current launch's pid; `stop` also checks `/proc/<pid>/cmdline` | — |
| A fault statement differs from the entry's wording | Take each remedy from the code's statement text (grep) when writing Relaunch | step 5 |
| Code-stage integrity stop: kind and text | Read `code-stage-interface-completion.md` and the orchestrator code | step 5 |

API verification: every runtime and orchestrator symbol used (`Run`, `LoadConfig`, `StateLabel`, `Summary`, `orchestrator.Body`, the `status.json` fields, the pause default) was read in source this session. `flock` LOCK_SH against LOCK_EX semantics are POSIX/Linux and are exercised by the tests.

### Plan critique (round 1, `phronithm:critic`, sonnet, critique-design): passed, 0 Critical

Findings and dispositions:

| # | Finding | Disposition |
|---|---|---|
| S1 | Fast-exit race in the launch wait | fixed |
| S2 | `launch.json` written before the lock check, and the dir not 0700 | fixed: per-launch file, renamed by the driver |
| S3 | Stale state reported as current | fixed: `driver.json`, `starting` and `failed-to-start` |
| S4 | `stop` could signal a stale pid | fixed: `driver.json` pid and a `/proc` cmdline check |
| S5 | `resume` loses the outcome's `planSha` and `unscribed` | fixed: `resume` replays only no-outcome endings and refuses after `returned`; the skill uses `launch -args` |
| S6 | `go run` erases exit codes | fixed: a built binary; the JSON is authoritative |
| S7 | The `stop` test needs a second signal | fixed |
| S8 | Stop-kind table source and placement | fixed: `lsf-workflow.md` named, a kinds diff |
| S9 | The mid-launch test is vacuous | added: the in-place overwrite case |
| S10 | Dry run clobbers the state | fixed: scratch dir |
| S11 | Lock collision with status or watch | fixed: one retry |
| S12 | SIGTERM before the handlers | fixed: refused while `starting` |
| S13 | No launch bound | fixed: 5 min, and a Bash timeout |
| M14 | Ref-invalid names | fixed: `git check-ref-format` |
| M15 | Pause is deferred | fixed: the message says so |
| M16 | Evidence ambiguity | fixed: explicit sequence and marker |
| M17 | Design of record timing | fixed: per step |
| M18 | Files unnamed | fixed: `control.go` |
| M19 | Dry run keeps `LSF_CLAUDE_BIN` | fixed |
| M20 | Relative paths | fixed |
| M21 | `-repo` default | fixed: the skill passes `-state` |
| M22 | Route detection | fixed: risk row |

API spec vs code block check: not applicable. The only block is the status JSON shape, consistent with the field list.

## Implementation steps

Phase 4 entry: re-probe the gates.
- `~/.local/share/claude/versions/2.1.294 --version` with `DISABLE_AUTOUPDATER=1`.
- `go version` under `GOTOOLCHAIN` auto in `lsf-runtime` is 1.27.1.
- `git version` ≥ 2.38.

Record the outcome.

1. **CLI core** (`cli/main.go`, `cli/config.go`, `cli/defaults.json`, tests). Subcommand dispatch, run addressing and the state-dir default, the name rule, `compose`, the claudeBin resolve and version check, the git check.
   Tests:
   - layering;
   - the refused keys;
   - the old-config hint (`allow`, `deny`, `permissionMode` inside a role);
   - missing host file;
   - `-plugin-dir` precedence;
   - claudeVersion fill and mismatch;
   - `LSF_CLAUDE_BIN` precedence;
   - git version parse and refusal (a fake `git` on `PATH`);
   - defaults equal to `config.example.json`'s roles, rules and skills;
   - **binary replaced mid-launch:**
     - (i) A `claude` symlink → v1; compose; repoint → v2; run `drive` from the launch file under the stub podman. Assert that the recorded run argv mounts v1.
     - (ii) Overwrite the resolved v1 file in place after compose, with a stub whose `--version` differs. Expect the runtime's smoke-time version refusal, if the stub podman's smoke runs the mounted path; if it does not, record that and name the live container test that covers it.

   Accept: `go test -race ./cli/` and `go vet` green. `lsf-runtime.md` `## CLI` gets the config and launch-check part in the same commit (design of record).
2. **launch / drive / watch / status** (`cli/launch.go`, `cli/status.go`, tests). Tests, all over the stub claude and podman and a real git fixture repo with a plan (built as the orchestrator integration tests build theirs):
   - (a) A launch with the pause ref set before it exits 0. This covers the fast run: the driver ends within a poll and the exit order varies. `watch` exits 0 with `outcome.status` `paused` and zero calls; `status` says `ended`.
   - (b) A launch-check refusal (`STUB_PODMAN_NOIMAGE=1`) makes `launch` exit 1 with the runtime's message. `status` then says `failed-to-start`, not the previous launch's `ended`, and `launch.json` is unchanged. `-dry-run` refuses the same way and leaves the run's state files untouched.
   - (c) An old config is refused with the hint.
   - (d) The driver survives: its session id differs from the test's, and it is still running after `launch` returns (blocked on a hanging stub call), until stopped.
   - (e) `status` lists a planted `clone/x`, `scratch/y`, `merge/w1`, `clone/z.partial`, `gates/g.partial`, a `refs/lsf/abcd1234/xfer/w1` ref, and a stub-podman container carrying the label. With no launch it reports `none`, exit 4.
   - (f) A crash (SIGKILL to the driver) gives `watch` exit 3 and `driver: crashed`.
   - (g) A launch refused because a driver is running leaves `launch.json` and `driver.json` unchanged.

   Accept: green under `-race`. `## CLI` amended for launch, watch and status in the same commit.
3. **pause / stop / resume** (`cli/control.go`, tests).
   - `pause` sets the ref at HEAD, and `-lift` deletes it, verified.
   - `stop` sent twice against a run blocked in a hanging stub call: `watch` reports `signal-SIGTERM`. `stop` with no driver refuses. `stop` while `starting` refuses (a stub smoke made slow).
   - `resume` relaunches with the stored args, and the second launch's journal replays (the stub call count does not grow for completed calls). It is refused with a driver running, and refused after a `returned` summary.

   Accept: green. `## CLI` and the `CLAUDE.md` Structure line amended in the same commit.
4. *(folded into steps 1–3: each amends `## CLI` for its own behaviour.)*
5. **`lsf-drive` skill**: the Startup, Launch and Relaunch edits, and the Pause, Read-the-return and Recovery lines. Before writing, grep the merge, clone and code-stage fault statements and read `code-stage-interface-completion.md`. End with the kinds diff (above). Critique with critique-skill.
6. **Evidence run (paid; ask before spending).**
   - Host config at a scratch path: image `localhost/lsf-fixture@sha256:344c825a…`, the 2.1.294 versioned claude, proxy with the live credentials file.
   - Build the fixture repo (`workflows/lsf/spike/fixture.sh`).
   - Following the skill's commands:
     1. Build the binary.
     2. `pause`, then `launch`: `paused`, no calls (free).
     3. `status`.
     4. `pause -lift`.
     5. In a turn into which the requester has typed a distinctive marker phrase (I'll propose one), relaunch with `launch -args` from the outcome's `planSha`. This is the resume of a paused run.
     6. `watch` in the background until `complete`.
     7. Then `resume`, which must refuse (`returned`).
   - Grep every `calls/*.jsonl` for the marker phrase and `Workflow harness`, expecting none. Cite the transcripts and their `claude_code_version` (headless pin).
   - Then the full `go test -race ./...` on a quiet machine (R35), with the load average recorded.

Each step: format (`gofmt`), `go vet`, commit, critique-code gate (sonnet `phronithm:critic`) spawned after the commit.

## Verification

- `cd lsf-runtime && go test -race ./...` (quiet machine, load recorded).
- Step 6's run: `status` JSON before and after; `outcome.status` `complete`; the transcript grep for P-F1; `podman ps` shows no labelled containers left.
- A manual check of detach: `launch` from a shell, close that shell, `status` from another shows `running`.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/runtime-cli-drive-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit runtime-cli-drive of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim (Phase 4 re-verifies the external gates against these
pins before implementing, recording the outcome in the implementation-summary): see "Unit entry (verbatim)" above, with the standing obligations listed under it (Register dispositions: P-F1; Headless pin; Design of record; R35 quiet-machine run).
External gates: `claude-code-headless` (environment, Claude Code 2.1.294); `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`); `git` (environment, 2.38 or later; 2.54.0 probed).
After Phase 5, produce the completion-report (contents per the
phronithm:feature skill's Produces) at docs/plans/runtime-cli-drive-completion.md,
then present this continuation prompt to the requester:
Run `/phronithm:breakdown` in re-plan mode with docs/plans/lsf-improvements-breakdown.md and docs/plans/runtime-cli-drive-completion.md before starting any dependent unit.
