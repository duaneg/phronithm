# s2-boundary-common — implementation summary

- **Branch**: `lsf-workflow` (base `e19f366`)
- **Plan**: `docs/plans/s2-boundary-common-plan.md` (committed `19a3ccd`)
- **External gates**: the Phase 4 entry probe's outcome was not recorded. Re-probed at Phase 5 close:
  - `lsf-toolchain`: `go version` → go1.27.1, matching `lsf-runtime/go.mod`.
  - `git`: `git --version` → 2.54.0, within 2.38 or later.
  - No drift.

## Files modified

- Plan: `lsf-runtime/plan/{gates,gates_test,schema}.go`
- Orchestrator:
  - `lsf-runtime/orchestrator/{doc,gates,git,loop,merge,prompts,results,runtime,stages,unit,wave}.go`
  - `lsf-runtime/orchestrator/orchtest/{orchtest,fixtures}.go`
  - tests: `gates_test.go`, `git_test.go`, `merge_test.go`, `run_test.go`
- Runtime:
  - `lsf-runtime/runtime/proxy.go` (new)
  - `lsf-runtime/runtime/{agent,api,classify,clone,config,gates,runtime,stream,uncommitted,worktree}.go`
  - tests: `proxy_test.go`, `gitenv_test.go` (new), `agent_test.go`, `classify_test.go`, `gates_test.go`, `uncommitted_test.go`
- Shared: `lsf-runtime/internal/gitenv/` (new)
- Tools and test data: `lsf-runtime/tools/promptparity/main.go` (the `gates-pin` label), `lsf-runtime/testdata/streams/{auth-401.jsonl,README.md}`, `lsf-runtime/validate/cmd/lsf-validate/main_test.go`
- Docs and skills: `docs/design/lsf-runtime.md`, `docs/design/lsf-workflow.md`, `docs/plan-graph.md`, `skills/breakdown/SKILL.md`, `skills/lsf-drive/SKILL.md`, `CLAUDE.md`

## Per-step record

### Step 1: `hosts` in the gate block (`8794a60`; critique fixes `2aa6631`)

- **Decision**: host entries are checked against an allowlist (LDH labels plus `_`, at most 63 characters a label and 253 a name), not the plan's denylist of characters.
- **Decision**: numeric last labels (`127.1`, `2130706433`, `0x7f`) and IPv4-compatible IPv6 addresses are refused. A trailing dot is stripped before the duplicate check.
- **Deviation**: `Declaration.Gates` became `omitempty`, so a hosts-only block's identity is `{"hosts":[…]}`. Existing blocks' identities are unchanged.

### Step 2: the pin, D31 (`8d03180`; critique fixes `035b63c`)

- **Decision**: the pin's effects come from the step's value, so a replay sets the declaration and calls `PinHosts` without running the step's body (critique round 1, disposition 1).
- **Decision**: a pin fault's reason starts `gates-pin: `, so it can be told apart from a load fault.
- **Deviation**: the termination prompt also carries the frozen-block clause when a block exists. A declared plan's termination prompt changes; an undeclared plan's prompts do not.
- **Deviation**: `skills/lsf-drive/SKILL.md`, `docs/design/lsf-workflow.md` and a `plan/schema.go` comment were edited, outside Scope, so their pin semantics agree.

### Step 3: git config isolation (`b7ec9d0`; critique fixes `d94dc66`)

- **Decision**: the global excludes and attributes files are copied to `.git/lsf-global/` in the clone and set as its own `core.excludesFile` and `core.attributesFile`. The plan appended them to `info/*`, which would rank them above `.gitignore` and `.gitattributes` and could make the uncommitted check report clean wrongly.
- **Decision**: the orchestrator's merge-clone set matches by containment, not by exact path.
- **Deviation**: `configCopied` also takes `gpg.ssh.defaultKeyCommand`.
- **Surprise**: `GIT_CONFIG_GLOBAL` needs git 2.32. The unit's gate pins 2.38 or later, and the runtime does not check the version.

### Step 4: plain-clone gate checkout (`c61e9de`; critique fixes `d94dc66`)

- **Decision**: the checkout's `git init` takes the main repo's `--object-format`, so SHA-256 repos gate.
- **Decision**: a stale gate directory that cannot be removed fails the checkout. The legacy `git worktree prune` runs only when a worktree under `gates/` is registered.

### Step 5: credential proxy (`ca85571`; critique fixes `f6a4c79`)

- **Decision**: the proxy uses its own clone of the default transport, with `ResponseHeaderTimeout` 10 minutes, and closes idle connections at the end. `HTTPS_PROXY` is honoured, as Claude Code honours it.
- **Decision**: `CONNECT` and more than one `Authorization` header are refused, and `Proxy-Authorization` is dropped. An `http` upstream must be loopback.
- **Decision**: `Close` shuts the server down (5 s) and then closes it, before the summary is written. A failing re-read is logged once until it recovers.
- **Surprise**: `ReverseProxy` flushes SSE and chunked responses itself, so the streaming test cannot tell `FlushInterval: -1` from the default. It tests the property, unbuffered streaming.

### Step 6: the 401 stop (`f9867ef`; critique fixes `26ff06a`)

- **Stream**: `auth-401.jsonl`, recorded from 2.1.294 against a local server answering 401. The CLI retries a 401 ten times (about 3 minutes) before the result; the trimmed fixture drops those `api_retry` events.
- **Deviation**: a call that the guard timeout kills while the CLI retries a 401 (`api_retry` with `error_status` 401) is an auth stop, not a timeout. Without this, the 10-retry wait under a short guard would hide the 401.
- **Decision**: the stop message says "renew the credential and relaunch", not "log in", since the credential may not be a login.
- **Fixture fidelity**: 2.1.294 is the pin. The other streams in `testdata/streams/` were recorded from 2.1.289.

### Step 7: design of record (`9e87f8a`)

The fragments of `lsf-runtime.md::runtime` went in with each step (disposition 15). This commit added the remaining prose and the `CLAUDE.md` line.

### Phase 5 fixes (`005b34c`, `5f42b04`)

- **Skills**:
  - `phronithm:lsf-drive` relaunches after `declared-gates-changed` only when the requester approves the change it shows them (old and new block).
  - `phronithm:breakdown`'s hosts step has a non-interactive rule: hosts come from CI config and lockfiles only, with no guesses or wildcards, and a gap is flagged in the risk register.
  - Its pin prohibition is scoped to a running workflow, and its quality check covers `hosts`.
- **Code**:
  - `pinGates` is split around a `pinned()` helper.
  - Host duplicates compare IP literals by address, and bracketed IPv4-mapped literals are accepted.
  - An auth retry counts only on the guard-killed path.
  - The gate checkout tells a `rev-parse` failure from an unknown commit, and resolves the state dir before matching legacy worktrees.
  - The proxy logs no client abort and sets an `IdleTimeout`.
  - New tests: `TestProxyConcurrentRotation`, and the isolated `check-attr` in `TestCloneInfoFilesWithoutGlobal`.

## Critique findings and dispositions

Gate: `phronithm:critic`, sonnet, critique-code, one round per step. No Critical anywhere.

| Step | Major | Minor |
|---|---|---|
| 1 | Host denylist: **fixed** (allowlist) | numeric forms, trailing-dot duplicate: **fixed**. Private and link-local entries: **noted for `s2-boundary-decision`**. Docs ahead of the orchestrator: resolved by step 2. Zone message, setup assigned when invalid: **no change** |
| 2 | — | pin fault indistinct, termination prompt, `lsf-drive` relaunch decisions, stale wording: **fixed**. Pin reports gate-block problems only: **noted in D31**. Replay test: **no change** (orchtest never runs a step's body, so `TestGatesPinFirst` already exercises the replay path) |
| 3 | global files appended to `info/*` invert precedence: **fixed** | `dirSet` exact match, `gpg.ssh.defaultKeyCommand`: **fixed**. Main-repo fetch from a clone under host config, git 2.32 floor: **known limits**. Fast-forward and worktrees on host config: **accepted** (disposition 14) |
| 4 | SHA-256 repos not gateable: **fixed** | stale-dir error, unconditional prune, error text, branch-name commit test, docs: **fixed**. Error-path and hook tests: **not added** (existing failure and stop tests cover them in part) |
| 5 | re-read test varied size only: **fixed** (same-length rotations by mtime and by inode) | transport, `http` upstream, log quoting, shutdown order, re-read log, `Proxy-Authorization`/`CONNECT`, test gaps: **fixed**. Streaming test can't prove the flush interval: **accepted**. Non-loopback `listen`: **no change** (disposition 8). Same-tick same-length rewrite: **known limit** |
| 6 | a guard-timeout 401 was never an auth stop: **fixed**. Tests couldn't tell the stop from the null path: **fixed** | message wording: **fixed**. `invalid` beside `authentication_failed`, stale-clone refusal on relaunch: **noted**. Fixture fidelity (fake server; the other streams are from 2.1.289): **noted**. Rate limit before permanent: **no change** |
| Phase 5 skill gate (critique-skill + critique-phronithm) | `lsf-drive` could ratify a widened `hosts`, no non-interactive hosts rule, pin prohibition against interactive re-plan: **fixed**. "Escalation's plan SHA doesn't exist": **rejected** (`escalation.plan_sha` exists) | quality check, condition wording: **fixed**; prose: **partly** |
| Phase 5 review (all lenses) and full-diff critique | — | adopted as under Phase 5 fixes. Agent-written driver or `hooksPath` in a kept clone's `.git/config` (the runtime reviewer rated it Significant): **known limit, `s2-sandbox` scope**. Git 2.32 floor, global `safe.directory`, private `hosts`, wildcard `listen`: **known limits**. Frozen clause for undeclared plans: **not adopted** (breaks S2 prompt parity; D31) |

## Success criteria verification

Rigour: `proven ▸ evidenced ▸ conjectured`.

1. **S1, the done condition**: **pass**.
   - (a) Proxy, target `proven`, achieved `proven`: `TestProxyRefuses` (absent, empty, wrong and doubled bearers, nothing upstream), `TestProxySubstitutes`, `TestProxyRereads`, `TestProxyConcurrentRotation`, `TestProxyStreams`, `TestProxyLifecycle` (the port refuses after `Run`), `TestProxySetupErrors`, `TestProxyConfig`.
   - (b) 401 stop and resume, `proven`: `TestAuthFailedStops` (no journal record, `stop_reason` `auth-failed`, relaunch returns the value), `TestAuthFailedSticky`, `TestClassifyAuth`.
   - (c) Git isolation, `proven`: `TestCloneIgnoresGlobalConfig` and `TestMergeCloneIgnoresGlobalFilter`, each against a control in which the filter runs; `TestCloneInfoFilesWithoutGlobal`; `TestEnv`; `TestDirSet`.
   - (d) Plain-clone gate checkout, `proven`: `TestRunGatesPlainClone`, `TestRunGatesCommitForms`, `TestGatePartialWiped`.
   - (e) Parser, `proven`: `TestParseDeclarationHosts`, `TestHostsBlocks` (lsf-validate), `TestHostsOnlyRunsUndeclared`.
   - (f) The pin, `proven`: `TestGatesPinFirst`, `TestDecisionReplanGatesPinned`, `TestGatesPinStops`.
   - (g) Docs, target and achieved `evidenced`: `plan-graph.md` Gate block and breakdown Phase 1 name `hosts`, checked by the Phase 5 skill gate.
   - (h) Tests pass, `proven`: the full race suite passed before `5f42b04`. Three timing tests in `./runtime` fail intermittently under heavy load, on the base as on HEAD; see below.
2. **S2, parity**: **pass**, `proven`. With node required, promptparity reports 55 scenarios identical and graphparity 145 cases.
3. **S3, the recorded 401 stream**: **pass**, `proven`. `auth-401.jsonl` is from 2.1.294 against a local 401 server and classes as `auth-failed` under test. 2.1.294 is the programme's `claude-code-headless` pin; the other recorded streams and `fixturerun`'s pin are still 2.1.289 (see the completion report's Discoveries).

### Timing tests under load

Three `./runtime` tests failed once each in a full race run at HEAD under heavy load, and passed alone. On 2026-10-10 they were run interleaved with a worktree at the base `e19f366`, under the same load (load average 9–13 on 8 cores, from another process), with `-race`:

| Test | Base `e19f366` | HEAD |
|---|---|---|
| `TestGuardTimeoutKillsGroup` | 3 / 60 | 2 / 60 |
| `TestRunGatesRegistersAndCleansUp` | 0 / 60 | 0 / 60 |
| `TestSessionLimitWaitsForReset` | 2 / 180 | 2 / 180 |

The flakes predate the unit and their rates are comparable, at achieved rigour `evidenced`. The session test fails at its 2-second wait for the first stub spawn (`agent_test.go:97`), and the guard test in its `hang` and `hang-ignore-term` cases; both are fixed timing bounds under CPU starvation. No base failure of `TestRunGatesRegistersAndCleansUp` was reproduced, and its code changed in this unit (step 4), so its single HEAD failure stays `conjectured` load-only. Upgrade path: a quiet-machine full race run, or a run of it at `-count=200` on both sides.

## Remaining phases

None: Phase 5 is complete. The completion report is `docs/plans/s2-boundary-common-completion.md`.
