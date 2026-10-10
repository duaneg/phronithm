# s2-sandbox: implementation summary

- **Working branch:** `lsf-workflow` (base `3e7b8bb`).
- **Plan:** `docs/plans/s2-sandbox-plan.md`.
- **Commits:** `3cfaba3`, `0600335` (step 1); `db5b5dd` (2a); `cf2dde1` (2b); `524faf6` (3); `6776b42` (4); `6c90a5f` (5); step 6 below.

## External-gate re-probe (Phase 4 entry, 2026-10-10)

| Gate | Pin | Probed | Outcome |
|---|---|---|---|
| `lsf-toolchain` | Go 1.27.1 | `go version` 1.27.1; `go.mod` `toolchain go1.27.1` | confirmed |
| `claude-code-headless` | 2.1.294 | `DISABLE_AUTOUPDATER=1 ~/.local/share/claude/versions/2.1.294 --version` → `2.1.294 (Claude Code)` | confirmed |
| `podman` | 6.1.3 rootless, `pasta` 2026_10_02.cba3570 | `podman --version` 6.1.3; `podman info` Rootless true, Pasta `/usr/bin/pasta`; `pasta --version` 2026_10_02.cba3570 | confirmed (log driver `journald`, as the plan found) |

## Files modified

`CLAUDE.md`; `docs/design/lsf-runtime.md`; `docs/plans/s2-sandbox-plan.md` (new), this file and the completion report; under `lsf-runtime/`:
- new: `container/Containerfile`, `internal/testcfg/{testcfg.go,podman_test.go}`, `runtime/{container.go,container_test.go,container_live_test.go}`, `orchestrator/container_test.go`, `testdata/podman`, `testdata/streams/2.1.294/*.jsonl`;
- deleted: `internal/tmproot/`, `internal/testenv/`;
- changed: `internal/gitenv/{gitenv.go,gitenv_test.go}`; `runtime/{agent.go,call.go,children.go,classify.go,clone.go,config.go,proxy.go,runtime.go,stream.go,worktree.go}` and their tests (`agent_test.go`, `call_test.go`, `classify_test.go`, `clone_test.go`, `config_test.go`, `helpers_test.go`, `supervise_test.go`); `orchestrator/{git.go,merge.go}` and test configs (`ff_test.go`, `gates_test.go`, `integration*_test.go`, `stages_test.go`); `testdata/{claude,config.example.json,streams/README.md}`; `tools/fixturerun/{main.go,main_test.go}`.

## Per-step record

### Step 1: stubs and test config (`3cfaba3`, `0600335`)
- `testdata/podman` emulates `run` (bind substitution, `-e` overlay, `-w`, argv record, smoke emulation, detached conmon-like life), `info`, `image exists`, `ps` by label, `rm -f`, and later `container exists`; it refuses unknown flags (125). The stub claudes answer `--version`. `internal/testcfg` gives the stub paths, a fake digest, a plugin dir and a credentials file.
- Critique fixes: records written atomically; `rm` kills the group and the pid; reused live names exit 125; no shared fallback state dir.

### Step 2a: config and launch argv (`db5b5dd`)
- As planned, with: `claudeBin` resolved at `Run`, not `prepare`, so `LoadConfig` stays pure (deviation); only `:` refused in paths, since a `,` works in a podman bind source (checked live) and test names contain commas (deviation); `tools/fixturerun`'s container settings moved forward from step 6, as its dry-run test broke.
- Surprise: the critic call runs twice under the stub (skill-missing retry), shifting run indices.
- Live L1 (commit call, `-i` under `--log-driver=none`, ownership, journal empty), L2 (probe: cwd, uid, read-only binds, writable index, exact env, proxy swaps the placeholder), L7 (conflict call stages a resolution mid-merge).

### Step 2b: launch failures, teardown, reaping (`cf2dde1`)
- As planned, plus: **label reaping pulled forward from step 3** (2b stops recording calls in `children.json`, which broke `--session-id` reaping); **`.git` bound over itself and `gitenv.CloneGitDir`** (design addition, from the 2a critique, confirmed live: an agent could rename `.git` away and supply its own, or write `.git/commondir`, and the host's git would run its filter).
- Bug caught by the stub's strictness: the reaper's `ps` filter lacked `label=`.
- Also from the 2a critique: state dir resolved once at `Run`; `:` checked after `Abs`; image may not start with `-`; a state dir the launch makes is `0700` (transcripts can carry the placeholder); stub records podman's `ANTHROPIC_AUTH_TOKEN`.
- Live L3 (grace-expiry kill, no container left), L4 (absent image → `runtime-error`), L6 (reaping).

### Step 3: launch checks (`524faf6`)
- As planned, with: the smoke gets no API variables (it has `--network none`).
- From the 2b critique: **nested repositories** — reproduced that a gitlinked nested repo's own filter ran on the host under the uncommitted check's `git status`; clone git now runs with `-c diff.ignoreSubmodules=all -c submodule.recurse=false`; `CloneGitDir` strict (no ascent) plus `GIT_CEILING_DIRECTORIES`; a failed reap refuses the launch (deviation: the plan had a failed `ps` logged and the launch going on); a failed `rm` whose container is gone is not a failure.
- Live L5 (wrong `claudeVersion`, absent image); the real smoke runs in every live test.

### Step 4: placement refusal (`6776b42`)
- As planned. Live L8 confirmed a clone under `/tmp` is not shadowed by `--tmpfs /tmp`, so the refusal is dropped. `CLAUDE.md` updated (outside the unit's scope, as the plan recorded).
- From the 3 critique (round 2 for the hardening): **a committed `.gitmodules` `ignore = none` overrides the config default** (reproduced). Two layers, each sufficient (mutation-checked): `gitenv.CloneGitlinks` refuses git in a clone whose index or `HEAD` holds a gitlink, and `--ignore-submodules=all` on every `status`/`diff`. Smoke version read from a marked stdout line; launch check under `Run`'s context; `CloneGitDir` requires `objects` and `refs`.

### Step 5: Containerfile, R17 (`6c90a5f`)
- Reference image built (`localhost/lsf-ref@sha256:949f2220…`); live suite passes against it. Fixture image (plus `nodejs`, `npm`): `localhost/lsf-fixture@sha256:344c825a…`.
- R17: six streams re-recorded from 2.1.294 (≈ $0.02) into `testdata/streams/2.1.294/`; no field `stream.go` reads moved. **Deviation:** the 2.1.289 files are kept beside them, not replaced, since other tests depend on their outputs; a new test classifies the 2.1.294 set.
- From the round-3 critique (converged): guard takes context and reports stderr; `checkRepo` refuses a gitlink at the tip; `IgnoreSubmodules` skips `-c`/`-C` values; hooks-on-host Known limit.

### Step 6: fixture run, docs (`a75371c` and the step-6 commit)
- Step 5 critique fixes (`a75371c`): README honest about host-direct recordings and unverified denial shape; scratch path scrubbed; version asserted; `CloneGitlinks` fails closed unless HEAD is genuinely unborn.
- `config.example.json` and `tools/fixturerun` were done in step 2a. The `lsf-runtime.md` amendments landed with each step; `CLAUDE.md` in step 4 and 5.
- Gate re-probe (end of Phase 4): 2.1.294, podman 6.1.3, pasta 2026_10_02.cba3570, unchanged.
- **Paid fixture run 1** (`~/.cache/lsf-fixturerun/run-1522950298`): 7 calls, $1.15, every transcript 2.1.294, `add-mean` merged on main, no container left; `status stopped`: the re-plan escalated a requester question about the fixture brief's wording ("each function is tested, including its error case"), a model judgement, not a container fault. Re-run once, per the plan's risk table.
- **Paid fixture run 2** (`run-4086766268`): `fixturerun: complete`; 9 calls, $1.19, all 2.1.294, merge on main, gates pass, no container left.
- **Container-path key-set comparison** (run 1's transcripts against the 2.1.289 recordings): no field `stream.go` reads moved; new `tool_progress` events and `result.safety_stops`, unread. **Surprise:** no `rate_limit_event` in any of the 16 container-path transcripts, though every host-direct recording has one: a session limit's wait would use `sessionLimitFallback`. Recorded as a Known limit and a discovery.
- Full suite with `LSF_LIVE_REQUIRE=1`, `PROMPTPARITY_REQUIRE_NODE=1`, `GRAPHPARITY_REQUIRE_NODE=1` and `LSF_TEST_IMAGE` the reference image: all pass; `go vet` and `gofmt` clean.

## Critique findings (per step)

Gate: `phronithm:critic`, sonnet, critique-code. No step ended with an open Critical.

| Step | Round | Critical | Significant | Disposition |
|---|---|---|---|---|
| 1 | 1 | 0 | 2 (record race; `rm` false success) | fixed in `0600335`; Minors 3, 7 (stale records, flag values, `-w`), 8, 9, 10, 12, 13, 14 noted for Phase 5 |
| 2a | 1 | 0 | 2 (**S1 `.git` rename/`commondir` bypass, confirmed live**; S2 placeholder in world-readable transcripts) | fixed in `cf2dde1`; M1–M3, M6, M7 fixed; M4 (gitbind stop counts a spawn), M5 (fixed in 3), M8 (golden mirrors impl), M9 (fixturerun filter), M10 (helper unit tests) noted |
| 2b | 1 | 1 (**nested gitlink filter, reproduced**) | 3 (emptied `.git` ascent; reap failure ignored; `rm` racing `--rm`) | fixed in `524faf6`; M5 (`rm` ignores a second signal), M6 (podman vanishing mid-run gives null), M8 (test gaps) noted |
| 3 + 2b r2 | 2 | 1 (**`.gitmodules ignore=none` override, reproduced**) | 3 (smoke stderr; no ctx; HEAD-kept ascent) | fixed in `6776b42`; M1 fixed by refusal, M2 (smoke does not exercise `pasta`), M3 (`rm` race polling) noted |
| 4 + r3 | 3 | 0 (converged) | 5 | Sig 2, 3, 4 (ctx/stderr) fixed in `6c90a5f`; Sig 5 Known limit; Sig 1 (fast-forward under host config: existing Known limit), Sig 4 (guard cost) and Min 7 (git version check, replace refs), Min 9 (stale historical plan files) noted |
| 5 | 1 | 0 | 2 (R17 overstated) | fixed in `a75371c`; M4, M6, M7, M8 noted |

## Success criteria

Rigour ladder: proven ▸ evidenced ▸ conjectured.

| # | Criterion | Status | Target | Achieved | Evidence / upgrade path |
|---|---|---|---|---|---|
| S1a | container per call, argv | pass | proven (stub) + evidenced (live) | proven + evidenced | `TestCallContainerArgv`, `TestRepoLessContainer`; live L1, L2 |
| S1b | config, refusals, roles reduced | pass | proven | proven | `TestLoadConfigRejectsContainer`, `TestLoadConfigRejects` |
| S1c | launch checks | pass | proven | proven | `TestLaunchChecks`, `TestSmokeArgv`; live L5 and the real smoke in every live test |
| S1d | 125/126/127 → `runtime-error` | pass | proven | proven | `TestLaunchFailureStops`; live L4 |
| S1e | proxy-only API route; `listen` 127.0.0.1 | pass | proven + evidenced | proven + evidenced | config tests; live L2 (upstream saw the live token, not the placeholder); fixture runs |
| S1f | read-only `.git` binds, created `hooks`/`info`, conflict call writes merge clone | pass | proven + evidenced | proven + evidenced | `TestGitBindsPrepared`; live L2, L7, smoke; hardened beyond the plan (`.git` mountpoint, `commondir`, gitlinks) |
| S1g | grace-expiry kill leaves no container | pass | proven + evidenced | proven + evidenced | `TestGraceExpiryRemovesContainer`, `TestFailedRemovalLeavesCloneUnread`; live L3 |
| S1h | label reaping; session-id reaping gone | pass | proven | proven | supervise tests, `TestReapFailureRefusesLaunch`, `TestDryRunReportsOrphans`; live L6 |
| S1i | `/tmp` refusal dropped; others kept | pass | proven + evidenced | proven + evidenced | `TestLaunchUnderTempDir`, `TestCloneRepoPreconditions`; live L8 |
| S1j | `Containerfile` | pass | evidenced | evidenced | built; live suite passes against it |
| S1k | argv and env tests | pass | proven | proven | as S1a; env asserted live (L2), the stub records podman's `ANTHROPIC_AUTH_TOKEN` |
| S1l | fixturerun on 2.1.294 in containers, no `hosts`, pin 2.1.294 | pass | evidenced | evidenced | run 2 complete (run 1 stopped on a model judgement) |
| S1m | streams re-checked (R17) | pass | evidenced | evidenced | `2.1.294/` set and its test; container-path key comparison; session-limit stream unverified (existing limit) |
| S1n | carriers named | pass | evidenced | evidenced | `lsf-runtime.md` Runtime changes |
| S1o | tests pass | pass | proven | proven | full suite above |
| S2 | parity tools pass | pass | proven | proven | `*_REQUIRE_NODE=1` run |
| S3 | live tests pass under `LSF_LIVE_REQUIRE=1` | pass | evidenced | evidenced | 8 live tests, probe image and reference image |
| S4 | no stream outside the state dir (`--log-driver=none`) | pass | proven (live) | evidenced | L1's `journalctl` check is empty and fails on a `journalctl` error, but has no positive control under the `journald` driver. Upgrade: a control container with the default driver whose stream the same query finds |

## Remaining phases
Complete Phase 5 (Review) as defined in the phronithm:feature skill.

