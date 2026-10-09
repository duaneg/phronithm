# s2-sandbox: plan (phronithm:feature, work-unit mode, compressed Phases 1–3)

## Context

The decided boundary (`docs/design/lsf-runtime.md::permissions`, c4fe2c1) is one rootless podman container per agent call. This unit puts every agent call in its own container. It covers the container config and launch checks, teardown and reaping by container name and label, the `127.0.0.1` restriction on the proxy's listen address, read-only `.git` binds, and a reference `Containerfile`. The fixture run then goes end to end in containers. The egress proxy and gates in containers belong to `s2-egress-gates`. Base commit `3e7b8bb`, branch `lsf-workflow`.

This file is saved after ExitPlanMode as `docs/plans/s2-sandbox-plan.md`, which serves as problem statement, design and plan together, as in earlier units.

## Unit entry (verbatim, docs/plans/lsf-improvements-breakdown.md @ 3e7b8bb)

### s2-sandbox
- **Type**: feature
- **Risk**: high — a launch fault fails every call; the runtime's first code driving podman
- **Clarity**: exploratory — one open decision: how the tests exercise the container launch (a stub `podman` beside `testdata/claude`, live tests that skip without podman, or both)
- **Iteration**: write-once
- **Effort**: large
- **Scope**: `lsf-runtime/runtime`, `lsf-runtime/internal`, `lsf-runtime/orchestrator`, `lsf-runtime/testdata`, `lsf-runtime/tools/fixturerun`, `lsf-runtime/container`, `docs/design/lsf-runtime.md::runtime`, `docs/design/lsf-runtime.md::permissions`
- **Publishes**: the container call launch; the container configuration (image digest, `claude` path, `env` passthrough) and launch checks; teardown and reaping by container name and label; the reference `Containerfile`
- **Depends on**: s2-boundary-decision (design, the decided launch); s2-boundary-common (logical, the credential proxy and `Proxy()`; serialises the runtime files); s2-clones (logical, the clones mounted at `/work`); container-probe (knowledge, the reference harness `run.sh`, `Containerfile` and `killgroup.py`)
- **External gates**: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`); `claude-code-headless` (environment, Claude Code 2.1.294); `podman` (environment, 6.1.3 rootless, `pasta` 2026_10_02.cba3570)
- **Done condition**: Every agent call runs in its own rootless podman container, launched as the Permissions section's A call's launch states. The runtime config names the image by digest, refusing a tag alone, the `claude` binary path and an `env` passthrough list; roles carry only `tools` and `clone`, and `allow`, `deny`, `permissionMode` and the `<plugin>` substitution are gone. A launch refuses to start when podman or `pasta` is missing, podman is not rootless, the image is absent, or the smoke container fails to report the pinned `claude --version` or to run `bash`, `git` and `rg`. A `podman run` exiting 125, 126 or 127 before the stream starts stops the run with `runtime-error`, not retried. Each call reaches the API only through the credential proxy spliced by `pasta`, with the placeholder bearer, and a `proxy.listen` host other than `127.0.0.1` is refused. In every clone, the merge clone included, `.git/config`, `.git/hooks` and `.git/info` are read-only inside the container, a missing `hooks` or `info` is created before the launch, and the conflict call can write the merge clone. A cancel sends SIGTERM to the `podman run` group and, after `killGrace`, runs `podman rm -f <name>`; fetch-back, the uncommitted check and removal wait for it, and a test killing a call on the grace-expiry path finds no container left. A launch reaps containers carrying its state dir's label, and call reaping by `--session-id` is gone. The `/tmp` and `/var/tmp` placement refusal is dropped; the shallow, submodule and LFS refusals stay. A reference `Containerfile` (digest-pinned Debian slim with `bash`, `ca-certificates`, `curl`, `git`, `procps` and `ripgrep`) ships under `lsf-runtime/container`. Tests cover each call's argv and environment. `tools/fixturerun` runs the spike fixture end to end in containers on 2.1.294 with no `hosts`, its `pin` constant is 2.1.294, and the streams in `lsf-runtime/testdata/streams/` are re-checked against 2.1.294 (R17). The Permissions section's Runtime changes names this unit and `s2-egress-gates` as carriers. The tests pass.
- **Description**: Runtime changes 1, 2, 6, 7 and 8 of `lsf-runtime.md`'s Permissions section, 3 for calls, and 4's `listen` restriction; the egress proxy, gates in containers and their teardown go to `s2-egress-gates`. Disposes of P-C5 and P-C6. Until `s2-egress-gates`, a call's Bash has no network even with `hosts` set, and gates run on the host (R14). Hand-offs from `s2-boundary-common`: `Runtime.Proxy()` returns `{URL, Placeholder}`, or the zero value with no `proxy` block; `Hosts()` returns the pinned hosts, nil before the pin and for a plan with none. **Flag:** six surfaces exceed the surface bound; the orchestrator edit is its tests' role configs, `internal` the temp-root check, `testdata` a stub and the `Containerfile` a dozen lines, so the context is the runtime's launch and teardown.

Standing obligations carried:
- **Register dispositions:** P-C5 and P-C6, recorded in the completion report.
- **Headless pin:** check against `~/.local/share/claude/versions/2.1.294` with `DISABLE_AUTOUPDATER=1`, or cite a transcript's `version`.
- **Design of record:** amend `lsf-runtime.md::runtime` and `::permissions` in the same commits that change the behaviour they describe.

## Requester decisions (this session, 2026-10-10)

- **D-test: both.** A stub `testdata/podman` turns `podman run` into a host exec, so every existing runtime and orchestrator test keeps running without podman. A few live tests run when podman and `LSF_TEST_IMAGE` (an image by digest) are present, and skip otherwise; `LSF_LIVE_REQUIRE=1` makes a skip a failure.
- **D-pin: config field.** A required `claudeVersion` beside `claudeBin`. The first field of the smoke container's `claude --version` output must equal it.
- **D-image: env digest.** The live tests take their image from `LSF_TEST_IMAGE`, built by the user from the reference `Containerfile`. Today `localhost/lsf-probe@sha256:a214c2fe…` exists, and the fixture needs node.
- **D-path: mount the clone at its host path.** `-v <clone>:<clone>` and `-w <clone>`, not `/work`. The clone's local config holds absolute host paths: `core.hooksPath`, and `core.excludesFile` and `core.attributesFile` under `.git/lsf-global/`. At `/work` those paths would not exist, and git in the container would silently skip the hooks and the global excludes, so `git add -A` could commit files the host ignores. **Deviation:** the design's `/work`, recorded in the completion report, and `lsf-runtime.md` amended.

## Success criteria (target rigour)

| # | Criterion | Target |
|---|---|---|
| S1 | The done condition, clause by clause: container per call and argv (S1a); config, refusals and dropped role fields (S1b); launch checks (S1c); 125/126/127 → `runtime-error` (S1d); proxy-only API route and `listen` restriction (S1e); read-only `.git` binds, created `hooks`/`info`, conflict call writes the merge clone (S1f); teardown on the grace-expiry path, with no container left (S1g); label reaping, session-id reaping gone (S1h); placement refusal dropped, the others kept (S1i); `Containerfile` (S1j); argv and env tests (S1k); fixturerun on 2.1.294 in containers (S1l); streams re-checked (S1m); carriers named (S1n); tests pass (S1o) | proven under the stub, plus evidenced live (S3), for S1a, S1e, S1f, S1g, S1i (the stub cannot replace the environment, mount, or run `pasta`); proven for S1b, S1c, S1d, S1h, S1k; evidenced for S1j, S1n (files and prose) and S1l, S1m (a paid run and a one-off comparison) |
| S2 | `promptparity` and `graphparity` still pass | proven |
| S3 | The live tests pass on this host (podman 6.1.3, `LSF_TEST_IMAGE` set) under `LSF_LIVE_REQUIRE=1` | evidenced (environment-gated; the skip keeps `go test` green elsewhere) |
| S4 | No call's stream persists outside the state dir: `--log-driver=none`, verified live | proven by a live test |

## Exploration findings

- **Launch path.** `runtime/call.go`: `buildArgv`, and `spawn` (Setpgid, Cancel = SIGTERM to the group, WaitDelay = KillGrace, then group SIGKILL). `runtime/agent.go` `attempt` builds the `spawnSpec`. `runtime/clone.go` `placeCall` gives `placement{dir, kind}`: `dir` is empty for the repo-less test mode, and `opts.dir` is the merge clone.
- **Config.** `runtime/config.go`: strict JSON. `Role{Tools, Allow, Deny, PermissionMode, Clone}`; `pluginToken`; `ClaudeBin` defaults to `claude`, and `LSF_CLAUDE_BIN` overrides it.
- **Reaping.** `runtime/children.go`: `children.json` maps session or gate marker to pid; `reapOrphans` matches with `ps`. Gates stay on the host in this unit, so the marker path stays and only the session-id path goes.
- **Placement refusal.** `runtime/runtime.go:68` uses `internal/tmproot`. `internal/testenv` relocates `TMPDIR` only because of that refusal, and `tools/fixturerun` checks `tmproot` too.
- **Proxy.** `runtime/proxy.go` `ProxyConfig.prepare` and `Proxy()` give `{URL, Placeholder}`, with the URL taken from the bound listener.
- **Tests.** `runtime/helpers_test.go` `testConfig` uses the stub `testdata/claude`. The orchestrator tests (`integration*_test.go`, `ff_test.go`, `gates_test.go`) build `lsfrt.Config` with `PermissionMode` and `ClaudeBin: os.Args[0]`, and `TestMain` runs `fakeClaude` when `LSF_FAKE_CLAUDE_TABLE` is set and `--session-id` is present.
- **Probe harness.** `docs/investigations/container-probe/run.sh` `container()` is the reference argv. On this host `podman info` gives `Host.Security.Rootless` = true and `Host.Pasta.Executable` = `/usr/bin/pasta`, `podman rm` has `-t` and `-i/--ignore`, and the log driver is **journald**.
- **Found while exploring:** podman's default log driver here is journald, so every probe call's stdout stream went to the user journal. `--log-driver=none` stops that; `-i` attach does not depend on the log driver, and S4 verifies that live.

## Candidate approaches

1. **Wrap at the spawn (chosen).** `buildArgv` becomes the inner `claude` argv. A new `containerArgv` wraps it in `podman run …`, and `spawn` executes the podman binary. The process-group cancel, stream tee and classification are unchanged, and teardown adds `podman rm -f` after the wait. Small diff, and every caller of `spawn` is unchanged.
2. **A container abstraction** (an interface with podman and host implementations, chosen by config). The host implementation would be a test-only bypass of the boundary. The design says every call runs in a container, so a bypass weakens the guarantee the tests should hold. Rejected: the stub `podman` gives the tests the same reach without a bypass in production code.
3. **The podman REST API over its socket** instead of the CLI. This adds the socket to the runtime's dependencies, and Go is stdlib-only, so it means a hand-rolled client. The probe validated the CLI. Rejected.

## Design

### 1. Config (`runtime/config.go`)
- **`Role`** becomes `{Tools, Clone}`. The `pluginToken` substitution and its checks go. An old config with `allow`, `deny` or `permissionMode` fails strict decoding as an unknown field, which is the intended refusal.
- **New fields:**
  - `image` (required): must match `^[^@\s]+@sha256:[0-9a-f]{64}$`. A tag alone is refused with a message naming the digest form.
  - `claudeVersion` (required): non-empty.
  - `env` (`[]string`, optional): each a name matching `^[A-Za-z_][A-Za-z0-9_]*$`. Refused: `PATH`, `HOME`, `SHELL`, `DISABLE_AUTOUPDATER`, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC`, `NO_PROXY`/`no_proxy`, the four proxy variables, and every `ANTHROPIC_*` and `CLAUDE_CODE_OAUTH_TOKEN`. The runtime sets these, or they carry a credential.
  - `podmanBin` (optional, default `podman`; tests point it at the stub). The config is trusted input, like `claudeBin` and `LSF_CLAUDE_BIN`: pointing it elsewhere is the operator's choice, not a bypass the runtime defends against.
- **The env denylist** guards only the names the runtime sets and those that carry a credential or a route (`ALL_PROXY`/`all_proxy` included, as `hosts` is not wired yet). It is not a closed list of safe names: the requester owns the config.
- **`stateDir`** must not contain `:` or `,`, since it appears in `-v` specs. It is hashed after `EvalSymlinks`, so every spelling gets the same label.
- **`claudeBin`** keeps its `LSF_CLAUDE_BIN` override. After prepare it is resolved to an absolute regular file (LookPath, then EvalSymlinks): it is bind-mounted, so a PATH name or a symlink that moves mid-launch is not.
- **`pluginDir`** becomes required. HOME in the container is empty, so the plugin reaches `claude` only by `--plugin-dir`.
- **`proxy`** becomes required. Without it no call can reach the API. `proxy.listen`'s host must be exactly `127.0.0.1` (in `ProxyConfig.prepare`), and the "not loopback, logged" path goes.

### 2. The call's launch (new `runtime/container.go`)
- **`claudeArgv(o, role, sessionID)`:** `-p --output-format stream-json --verbose --session-id <sid> [--model] [--effort] --strict-mcp-config --plugin-dir /opt/plugin --tools <role.Tools> --dangerously-skip-permissions --json-schema <schema>`.
- **`containerArgv(spec)`**, `[]string` after the podman binary:
  ```
  run --rm -i --init --pull=never --log-driver=none --name <name> --label lsf.state=<stateHash>
  --userns=keep-id --cap-drop=all --security-opt no-new-privileges --read-only --tmpfs /tmp:rw,exec
  --network pasta:--splice-only,-T,<proxy port>
  -v <dir>:<dir>:rw [-v <dir>/.git/config:<dir>/.git/config:ro -v …/hooks:ro -v …/info:ro]
  -v <claudeBin>:/opt/claude/claude:ro -v <pluginDir>:/opt/plugin:ro
  -w <dir> -e HOME=/tmp/home -e SHELL=/bin/bash -e DISABLE_AUTOUPDATER=1
  -e CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 -e ANTHROPIC_BASE_URL=http://127.0.0.1:<port>
  -e ANTHROPIC_AUTH_TOKEN -e NO_PROXY=127.0.0.1 -e no_proxy=127.0.0.1 [-e NAME …]
  <image> /opt/claude/claude <claudeArgv>
  ```
  - `<stateHash>` is the first 16 hex digits of sha256(stateDir).
  - `<name>` is `lsf-<stateHash[:8]>-<session id>`, unique per attempt.
  - The proxy port comes from the bound listener.
  - **The placeholder is not in the argv.** `-e ANTHROPIC_AUTH_TOKEN` takes its value from the `podman` child's environment (`cmd.Env` = the runtime's environment plus that one variable). In argv it would show in `ps` to every local user, who could spend the credential through the loopback proxy. The dry-run log and telemetry never carry it.
  - The `-e NAME` passthrough takes its value from the runtime's own environment. `podman run` inherits that environment, but the container receives only the `-e` list.
- **Binds.** For a clone or the merge clone (`placement.kind != ""` or `opts.dir`), the runtime first creates `.git/hooks` and `.git/info` if missing (mode 0755), then adds the three read-only binds. A `.git` that is not a directory, or a missing `.git/config`, stops the run with `runtime-error` before the spawn.
- **Repo-less mode** (`repo` unset, the runtime's own tests only) mounts a fresh empty `scratch/<key>` and runs there, never the launcher's working directory. The design's "none of the Permissions placement holds" is amended accordingly.
- **Additions to the design's argv**, recorded as deviations:
  - `--pull=never`: a missing digest never triggers a registry pull.
  - `--log-driver=none`: no stream copy in journald (S4).
  - Mounting at the host path (D-path).
- **`spawn`** executes `cfg.PodmanBin` with `containerArgv`, in its own group as now. The `spawn` telemetry event records the container name.
- **Dry run** logs the podman argv in place of the claude argv, with `<dir>` for the unplaced clone.

### 3. Launch failures (`agent.go` `attempt`)
- **Detection.** After `spawn`: exit code 125, 126 or 127, and no stream line parsed (a new `streamAcc.Lines` count, 0). Then `setStop(StopRuntimeError, "<label>: podman run exited <n> before the stream: <stderr tail>")`, with no retry and no journal.
- **Other cases.** A spawn error of the podman binary itself stays `spawn-error`. So does a 125 after a stream began, which is classified as today.

### 4. Teardown (`call.go` `spawn`)
- **After `cmd.Wait`,** when the context cancelled the call while it ran, or the exit code is non-zero: run `podman rm -f -t 0 --ignore <name>` under its own 60 s bound, not the cancelled context. Then group-SIGKILL as today, for any client survivor.
- **Why `-t 0`.** SIGTERM already went to the client group, and `--init` and podman's sig-proxy carry it to the container, so the grace has elapsed.
- **Why on a non-zero exit too.** A podman client killed mid-cleanup can leave the container.
- **A failed `rm`** (an error or its 60 s bound) stops the run with `runtime-error` before anything reads the clone. There is no fetch-back, no uncommitted check, no discard and no journal record, and the clone stays as found. The next launch's label reaping removes the container before any call. This upholds "nothing reads a clone until its container is gone".
- **`-t 0` and the 60 s bound** are design additions, recorded as deviations. On a second signal, exit can be delayed by up to that bound.
- **Ordering.** Fetch-back, the uncommitted check and discard all run after `spawn` returns, so they wait for `rm`.
- **`children.json`** no longer records calls. `addChild`/`setChildPid`/`removeChild` serve gates only, until `s2-egress-gates`.

### 5. Launch checks and reaping (`runtime.go` `Run`, new code in `container.go`)
- **Order after the lock and log files:**
  1. `checkContainerHost`;
  2. `reapOrphans` (containers by label, then gate markers);
  3. `wipeScratch`;
  4. the proxy;
  5. the body.
- **`checkContainerHost`** returns a setup error (Run's error, logged) when:
  - `podmanBin` is not found;
  - `podman info --format {{.Host.Security.Rootless}}` is not `true`;
  - `{{.Host.Pasta.Executable}}` is empty;
  - `podman image exists <image>` fails;
  - the smoke container fails. It is launched through the same `containerArgv` builder as a call, with a label `lsf.smoke=1` and `--network none`. Its directory is a throwaway git repo the runtime makes at `<stateDir>/smoke`, bound at its host path with the three read-only `.git` binds. It runs `/bin/sh -c "$smoke"`, where `smoke` is a runtime constant that:
    - prints `/opt/claude/claude --version`;
    - checks `bash`, `git` and `rg` run;
    - writes a file and `git add`s it;
    - checks that writing `.git/config`, `.git/hooks/x` and `.git/info/x` fails.

    A non-zero exit fails it, and so does a first output line whose first field is not `claudeVersion`. The error quotes the output. It runs under a 2-minute bound, with `podman rm -f` on expiry. `<stateDir>/smoke` is removed afterwards.
  - **`pasta`** is checked on the host as well: the `info` field must be non-empty and name an existing executable. `podman info`'s behaviour without `pasta` is unverified.
- **Dry run.** The checks still run, so a dry run needs podman and starts one smoke container, which removes itself.
- **Container reaping.** `podman ps -a --filter label=lsf.state=<hash> --format {{.Names}}`, then `podman rm -f -t 0 --ignore` on each. In a dry run they are reported, not removed. A failed `ps` is logged and the launch goes on; containers are found by label, so nothing needs carrying forward. Entries in `children.json` that are not gate markers, left by an older launch, are logged and dropped.

### 6. Placement refusal
- `Run`'s `tmproot` check goes, and so does `tools/fixturerun`'s.
- `internal/tmproot` is deleted. `internal/testenv` and its three `TestMain` callers go too, since they relocate `TMPDIR` only because of the refusal. `go test` then uses the default `TMPDIR`.
- The shallow, submodule and LFS refusals are unchanged.

### 7. Stubs (`lsf-runtime/testdata`)
- **`testdata/podman`** (bash). State in `$STUB_PODMAN_STATE`, default `$STUB_STATE/podman`.
  - **`info --format F`:** `{{.Host.Security.Rootless}}` gives `${STUB_PODMAN_ROOTLESS:-true}`, and `{{.Host.Pasta.Executable}}` gives `${STUB_PODMAN_PASTA-/usr/bin/pasta}`.
  - **`image exists R`:** exit 0 unless `STUB_PODMAN_NOIMAGE=1`.
  - **`run …`:**
    - It rejects any flag it does not know (exit 125), so a mistyped flag fails the stub tests.
    - A run labelled `lsf.smoke=1` is emulated: it runs only the mapped `claude --version`, then exits `${STUB_PODMAN_SMOKE_EXIT:-0}`, so the host needs no `rg`. The real smoke script runs only live (L5).
    - It parses the flags above and records its own argv NUL-separated as `run.<n>`.
    - It exits `$STUB_PODMAN_RUN_EXIT` when set, after writing to stderr.
    - Otherwise it builds the inner command: each `-v src:dst` with src ≠ dst is substituted textually in the command arguments, `-e K=V` is overlaid on the inherited environment, `-e NAME` is kept as inherited, and it `cd`s to `-w`. The environment is therefore **not** the container's: stub tests assert the `-e` list in the recorded argv, never the inner process's environment. Environment cleanliness is a live assertion (L2).
    - With `STUB_PODMAN_DETACH=1` it emulates conmon. The inner command runs under `setsid`, its pid is recorded as `name.<name>`, and the stub client ignores SIGTERM and waits, so only `rm -f` ends the inner command.
    - Otherwise it records `name.<name>` and `exec`s the command. Without `-i`, stdin is `/dev/null`.
  - **`ps -a --filter label=… --format …`:** lists names whose recorded pid is alive and whose recorded label matches.
  - **`rm -f [-t N] [--ignore] NAME…`:** kills each recorded pid's process group (TERM, then KILL) and removes the record. It exits 0 when a name is missing and `--ignore` was given.
- **`testdata/claude`** answers `--version` with `0.0.0-stub (Claude Code)` before any state is touched, so `count` is unaffected.
- **The orchestrator's fake claude** (`integration_test.go` `TestMain`) answers `--version` the same way when `LSF_FAKE_CLAUDE_TABLE` is set.
- **Shared test config.** A helper in a new `internal/testcfg` returns the stub podman path and a temporary credentials file. The runtime's `testConfig` and the orchestrator's configs use it for `PodmanBin`, `Image` (a fixed fake digest), `ClaudeVersion: "0.0.0-stub"`, `PluginDir` (a temp dir) and `Proxy` (`127.0.0.1:0`, the temp credentials file).

### 8. Reference `Containerfile` (`lsf-runtime/container/Containerfile`)
- The probe's image less node, npm and python3.
- `FROM docker.io/library/debian:trixie-slim@sha256:918311b7…`, then `apt-get install --no-install-recommends bash ca-certificates curl git procps ripgrep`.
- A header comment: the user extends it with the project's toolchain, builds it, and names it in the config by digest (`podman image inspect --format '{{.Digest}}'`).

### 9. fixturerun and the streams
- **`tools/fixturerun`:**
  - `pin = "2.1.294"`;
  - a required `-image` flag (a digest; the fixture needs node, so the probe image or the reference extended with `nodejs npm`);
  - `claudeVersion = pin`;
  - `proxy {listen 127.0.0.1:0, credentialsFile ~/.claude/.credentials.json}`;
  - no `hosts`;
  - its `tmproot` check dropped.
- **`testdata/config.example.json`** loses `allow`, `deny` and `permissionMode`, and gains `image`, `claudeBin`, `claudeVersion`, `env: []` and `proxy`.
- **The run** is paid, about $3.50 and 16 calls. It happens once, after the stub and live tests pass. Its pass conditions are as today, plus no `lsf.state` container left at the end.
- **R17 re-check, before the paid fixture run.** Re-record `ok`, `permanent`, `skill-loaded`, `skill-tool`, `skill-absent` and `skill-unknown` from the pinned 2.1.294 binary, called directly as the README describes. They are haiku/low calls, a few cents; `permanent` costs nothing. Diff them against the 2.1.289 recordings field by field, then replace them. A session-limit stream cannot be forced and stays unverified (an existing Known limit). After the fixture run, a one-off script also compares, per event type (`system/init`, `assistant`, `user`, `rate_limit_event`, `result`), the key sets in the recorded 2.1.289 streams against the fixture run's 2.1.294 transcripts. That covers the fields `stream.go` reads: `structured_output`, `total_cost_usd`, `is_error`, `api_error_status`, `permission_denials`, `terminal_reason`, `slash_commands`, `rate_limit_info.resetsAt`, and `error`. Differences that matter to `stream.go` are fixed. The README records the re-check, its date and the result. The streams are not re-recorded unless a field moved.

### 10. Live tests (`runtime/container_live_test.go`)
- **Gating.** The tests run when `podman` is on PATH and `LSF_TEST_IMAGE` is set, and skip otherwise. `LSF_LIVE_REQUIRE=1` turns a skip into a failure.
- **Setup.** The real podman, the stub `testdata/claude` as `claudeBin` (bash in the image runs it), and `claudeVersion` `0.0.0-stub`.
- **Cells:**
  - **L1, the call.** A `commit:x` call in a kept clone returns its value. The commit is fetched back, and every file in the clone is owned by the invoking user. Under `--log-driver=none` the stream arrives intact, and `journalctl --user CONTAINER_NAME=<name>` is empty; that part is skipped when `journalctl` is absent (S4).
  - **L2, a probe.** A new stub mode, `probe`, writes `probe.txt` in its cwd, recording:
    - its cwd;
    - `id -u`;
    - whether `.git/config`, `.git/hooks/x` and `.git/info/x` can be written;
    - whether `.git/index` can be written (`git add`);
    - `env`;
    - a `curl` to `$ANTHROPIC_BASE_URL` with the placeholder, against the proxy, whose upstream is an `httptest` server on `127.0.0.1` that records the `Authorization` it receives.

    It asserts read-only `config`, `hooks` and `info`, a writable index, the env exactly as specified (a host-only canary absent, the passthrough present), and that the upstream saw the live token, not the placeholder.
  - **L3, the grace-expiry kill.** A `hang-ignore-term` call with `guardTimeout` 2 s and `killGrace` 1 s. After `Call` returns, `podman ps -a --filter label=lsf.state=<hash>` is empty.
  - **L4, a launch failure.** In-package, the body sets `rt.cfg.Image` to an absent digest. The call stops `runtime-error`, made once.
  - **L5, launch checks.** A wrong `claudeVersion` and an absent image each fail `Run` with a naming error.
  - **L6, reaping.** A container started with the state's label (`podman run -d … sleep 300`) is gone after `Run`.
  - **L7, the merge clone.** A conflict call (`opts.dir` = a `MergeClone` mid-merge) runs `git add` on a resolution (stub mode `stage:<path>`), and the index change is visible on the host. `.git/config` is read-only there too.
  - **L8, temp placement.** A launch with repo and state dir under `/tmp` runs a `commit:` call in a container: the host-path bind is not shadowed by `--tmpfs /tmp`. If it is, the refusal stays, recorded as a deviation.

### Error handling summary
- Config and launch-check faults are `Run` setup errors.
- A launch failure (125/126/127) is a `runtime-error` stop.
- A failed `rm` is logged, and the next launch's reaping removes the container.
- A failed reaping `ps` is logged.

### Docs (`docs/design/lsf-runtime.md`)
- **Permissions.** A call's launch (the host-path mount, `--pull=never`, `--log-driver=none`); Image (the `claudeVersion` field); Teardown; Runtime changes (carriers: `s2-sandbox` for 1, 2, 3 for calls, 4's listen, 6, 7 and 8; `s2-egress-gates` for 3 for gates, 4's egress proxy and 5).
- **Known limits.**
  - Hooks copied from the host run in the container and may need host-only tools.
  - `/work` is gone.
  - The probe's streams went to the journal.
  - `.git/lsf-global/` is writable by the agent (attributes and excludes, not drivers).
  - One sporadic podman 125/126 ends a run (resumable).
  - No `--memory` or `--pids-limit`, and `/tmp` is RAM-backed: out of scope, as in the decided design.
  - A pre-upgrade `children.json` call entry is logged, not killed.
- **Runtime.**
  - Configuration: roles, the new fields, `pluginDir` and `proxy` required, the refusals.
  - A call: the argv, the 125 stop, teardown.
  - Supervision: orphans by label; gate markers until `s2-egress-gates`.
  - Interface: `Proxy()` is never zero now.
  - Known limits: the wildcard `listen` item goes.
- **`CLAUDE.md`'s lsf-runtime paragraph:** the `internal/` contents (the `tmproot` and `testenv` lines) and `testdata/podman`. This is outside the unit's scope and recorded as a deviation.

## Risks and unknowns

| Risk | Resolution | When |
|---|---|---|
| `-i` attach under `--log-driver=none` | Live L1; fallback `--log-driver=k8s-file` (removed with `--rm`) | step 2 |
| `--read-only` with a bind target under `$HOME`, which does not exist in the image (podman creates the mountpoint) | Live L1 | step 2 |
| Bind source a regular file (`claudeBin`) after EvalSymlinks | Live L1/L5 | step 2 |
| The stub's textual `-v` substitution misses an argument form | Stub unit test of the substitution | step 1 |
| A host hook copied into the clone needs a host-only tool, so the agent's commit fails in the container | Documented Known limit; the fixture has no hooks | docs |
| R35 timing tests flaky under load | Run the suite on a quiet machine | step 6 |
| The paid fixture run fails for a model reason | Re-run once; a second failure is a finding for the report | step 5 |

## Steps

Each step: `go vet ./... && go test -race ./...` in `lsf-runtime` green, `gofmt` clean, committed, then a critique-code gate spawned on the commit.

1. **Stubs and test config.** `testdata/podman`; `--version` in `testdata/claude` and the fake claude; `internal/testcfg`. Test: a small Go test runs the stub `podman run` with `-v`/`-e`/`-w` and checks the substitution, env overlay and cwd; `ps`/`rm` over a detached inner command. *Acceptance:* the stub tests pass; existing tests untouched and green.
2a. **Config and launch argv** (`config.go`, new `container.go`, `call.go` `buildArgv`→`claudeArgv`, `agent.go`). Roles reduced; new fields and refusals; `containerArgv`; placeholder by env; `.git/hooks`/`info` creation; repo-less scratch dir; dry-run argv. Every runtime and orchestrator test config moves to `testcfg`. Tests:
   - config refusals (tag-only image, bad env name, a reserved env name, a non-`127.0.0.1` listen, an old `permissionMode` field, `:` in stateDir);
   - the argv of a writer, a reviewer, the merge call and a repo-less call, golden on `run.<n>`, with the placeholder absent from argv and telemetry.

   Live L1, L2, L7 and L8. *Acceptance:* S1a, b, e, f, i (live), k.
2b. **Launch failures and teardown** (`call.go` `spawn`, `agent.go`, `stream.go` `Lines`, `children.go`). 125 stop; `rm -f` teardown; failed `rm` → stop with nothing read; `children.json` for gates only. Tests:
   - 125 → `runtime-error`, one attempt;
   - a detached-stub grace-expiry kill leaves no recorded name;
   - a failing stub `rm` stops the run with no journal record and no fetch-back.

   Live L3 and L4. *Acceptance:* S1d, S1g.
3. **Launch checks and reaping** (`runtime.go`, `children.go`, `container.go`). Stub tests for each refusal (rootless false, pasta empty, no image, version mismatch, a smoke exit), label reaping, and dry-run reporting; live L5 and L6. *Acceptance:* S1c, S1h.
4. **Placement refusal and internal cleanup.** Drop `tmproot`, `testenv` and their callers; fixturerun's check. Test: a launch with its repo and state dir under `os.TempDir()` succeeds. *Acceptance:* S1i.
5. **Containerfile, image, R17 re-recording.**
   - Add `container/Containerfile` and build it. Run the full live suite with `LSF_TEST_IMAGE` set to the reference image's digest.
   - For the fixture, build a fixture image from it plus `nodejs npm`, using the Containerfile shown in fixturerun's doc comment, not shipped.
   - Re-record the six streams from 2.1.294 and diff them against the old ones. Fix and test any `stream.go` change.
   - Update the README.

   *Acceptance:* S1j, S1m (failure classes), S3.
6. **fixturerun, example config, docs, paid run.**
   - `config.example.json`; fixturerun changes; the `lsf-runtime.md` amendments; the `CLAUDE.md` paragraph.
   - The gate re-probe: 2.1.294 `--version` with `DISABLE_AUTOUPDATER=1`, and the podman and pasta versions.
   - The **paid fixture run**: `go run ./tools/fixturerun -image <fixture digest>`, with the key-set comparison over its transcripts.
   - The full suite with `LSF_LIVE_REQUIRE=1`, `PROMPTPARITY_REQUIRE_NODE=1` and `GRAPHPARITY_REQUIRE_NODE=1` on a quiet machine.

   *Acceptance:* S1l, S1n, S1o, S2, S4.

Ordering: steps 1 → 2a → 2b → 3 must run in sequence. Step 4 is independent after 2a. Step 5 needs 2a–4, and step 6 needs 5.

## Plan critique (gate round 1, sonnet: passed, no Critical)

| # | Sev | Finding | Disposition |
|---|---|---|---|
| 1 | Sig | A failed `rm -f` still lets fetch-back read the clone | Fixed: a failed `rm` stops the run before anything reads the clone (§4, step 2b) |
| 2 | Sig | The smoke container skips the risky mounts | Fixed: the smoke binds a throwaway repo at its host path with the ro `.git` binds and checks writes (§5) |
| 3 | Sig | The placeholder bearer is in argv, visible in `ps` | Fixed: passed by env (§2) |
| 4 | Sig | Repo-less mode mounts the cwd, or `-v :` | Fixed: a fresh `scratch/<key>` (§2) |
| 5 | Sig | The stub smoke needs `rg` on the host | Fixed: the stub emulates the labelled smoke (§7) |
| 6 | Sig | "proven" rests on live-only evidence | Fixed: targets relabelled; live L7 (merge clone) and L8 (`/tmp`) added before the paid run; the stub rejects unknown flags and its env is not asserted |
| 7 | Sig | R17 re-check misses the failure-class fields | Fixed: six streams re-recorded from 2.1.294 before the paid run (step 5); session limit stays unverified |
| 8 | Sig | S4 is not tested | Fixed: `journalctl` check in L1 |
| 9 | Sig | Unverified behaviour of `pasta` in `podman info` | Fixed: the host checks the executable exists too |
| 10 | Min | `podmanBin` as a bypass | Noted: config is trusted (§1) |
| 11 | Min | The label hash depends on the path spelling | Fixed: hash after `EvalSymlinks` |
| 12 | Min | The env denylist scope | Fixed: scope stated; `ALL_PROXY` added |
| 13 | Min | `-t 0` and the 60 s bound unlisted | Fixed: recorded as deviations |
| 14 | Min | One sporadic 125 ends the run | Noted: the done condition says not retried; Known limit; raised in the completion report |
| 15 | Min | R17 after the paid run | Fixed: moved before it |
| 16 | Min | The reference image is never built | Fixed: built and the live suite run against it (step 5) |
| 17 | Min | Dry-run gaps | Fixed: `<dir>` placeholder; the smoke in a dry run is stated |
| 18 | Min | `:` or `,` in a path | Fixed: refused in `stateDir` |
| 19 | Min | `lsf-global` is writable | Noted: Known limit |
| 20 | Min | `.git/config` bind source | Fixed: checked, `runtime-error` |
| 21 | Min | Legacy `children.json` call entries | Noted: logged, not killed (Known limit) |
| 22 | Min | The smoke has no timeout | Fixed: a 2-minute bound |
| 23 | Min | No resource limits | Noted: out of scope (Known limit) |

API spec / code block consistency check: not applicable (the plan has an argv sketch, not a code block for an exported API).

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/s2-sandbox-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured`; rung definitions are in phronithm:feature Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit s2-sandbox of docs/plans/lsf-improvements-breakdown.md (@ 3e7b8bb).
Unit entry, verbatim: see "Unit entry" above. Phase 4 re-verifies these external gates against their pins before implementing, recording the outcome in the implementation-summary:
- `lsf-toolchain`: Go 1.27.1, pinned in `lsf-runtime/go.mod`.
- `claude-code-headless`: Claude Code 2.1.294, at `~/.local/share/claude/versions/2.1.294` with `DISABLE_AUTOUPDATER=1`. 2.1.295 and 2.1.296 are also present now; 2.1.294 has not been pruned.
- `podman`: 6.1.3 rootless, `pasta` 2026_10_02.cba3570.

Standing obligations: see "Unit entry" above.

After Phase 5, produce the completion-report (contents per the phronithm:feature skill's Produces) at `docs/plans/s2-sandbox-completion.md`, replacing the 2026-10-09 stopped report. Then present this continuation prompt to the requester:
"Run `/phronithm:breakdown` in re-plan mode with docs/plans/lsf-improvements-breakdown.md and docs/plans/s2-sandbox-completion.md before starting any dependent unit."
