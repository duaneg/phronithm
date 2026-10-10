# s2-egress-gates: implementation summary

- **Working branch:** `lsf-workflow` (base f6f0aef).
- **Plan:** `docs/plans/s2-egress-gates-plan.md`.
- **Commits:** cc3a72e (step 1), a7ce185 (step 2), bf9d5bf (step 3a), d3155b4 (steps 3b and 3c), 00713d4 (step 4), 3b52c3b (step 5 critique fixes and the CLAUDE.md clause).

## External gates (Phase 4 entry)

All pins were confirmed on 2026-10-10. There was no drift.

| Gate | Pin | Probed |
|---|---|---|
| `lsf-toolchain` | Go 1.27.1 (`lsf-runtime/go.mod`) | `go version go1.27.1` |
| `claude-code-headless` | 2.1.294 | `DISABLE_AUTOUPDATER=1 ~/.local/share/claude/versions/2.1.294 --version` → `2.1.294 (Claude Code)` |
| `podman` | 6.1.3 rootless, `pasta` 2026_10_02.cba3570 | `podman version 6.1.3`, `pasta 2026_10_02.cba3570` |

## Files modified

- `CLAUDE.md`
- `docs/design/lsf-runtime.md`
- `docs/plans/s2-egress-gates-plan.md` (new; the plan, saved from plan mode)
- `docs/plans/s2-egress-gates-implementation.md` (new)
- `lsf-runtime/runtime/agent_test.go`
- `lsf-runtime/runtime/call_test.go`
- `lsf-runtime/runtime/children.go`
- `lsf-runtime/runtime/classify.go`
- `lsf-runtime/runtime/classify_test.go`
- `lsf-runtime/runtime/clone.go`
- `lsf-runtime/runtime/container.go`
- `lsf-runtime/runtime/container_live_test.go`
- `lsf-runtime/runtime/container_test.go`
- `lsf-runtime/runtime/egress.go` (new)
- `lsf-runtime/runtime/egress_test.go` (new)
- `lsf-runtime/runtime/gates.go`
- `lsf-runtime/runtime/gates_test.go`
- `lsf-runtime/runtime/runtime.go`
- `lsf-runtime/runtime/supervise_test.go`
- `lsf-runtime/testdata/claude`

## Per-step record

### Step 1: a bare 429 is `session-limit`

- **What landed.** A bare 429 now shares the `rate_limit` branch in `classify.go`.
- **Deviation.** The plan gave a zero `resetsAt`. The code instead takes the reset from a rejected `rate_limit_event` when the stream carries one, and falls back to `sessionLimitFallback` otherwise, as the `rate_limit` path already did. A test pins this. The step critique judged it acceptable.
- **Tests.** A stub mode `bare429`; a `TestRetryClasses` row (`minDelay` = the fallback); `TestBare429WaitsFallback`.
- **A 429 with another assistant error** (for example `overloaded`) is classed by that error. This was added after the critique: one test and one doc-row clause.
- **Surprise.** A transient `index.lock` held by another git process made a `git stash` fail. No work was lost.

### Step 2: the egress proxy and the call splice

- **What landed.** `egress.go`.
  - `PinHosts` starts the proxy when the hosts are not empty.
  - A second pin with identical hosts is a no-op. One with different hosts sets `runtime-error`.
  - A failed start sets `runtime-error`. The hosts stay pinned with no port, so `setup` fails closed.
  - `Run` closes the proxy after the body.
  - Calls get the splice and the four proxy variables. `NO_PROXY`/`no_proxy` is `127.0.0.1,localhost,::1` in every container.
- **Decisions.**
  - Tunnels are tracked. A handler counts in the `WaitGroup` until it hands off, so `close()` waits for both handlers and tunnels.
  - A copy error closes both ends; EOF half-closes.
  - Targets containing `*`, `%`, brackets around a name, or empty or leading/trailing-dot labels are refused.
  - The first allow and the first refusal of a launch are logged and emitted, and the counts are logged at close.
  - `rt.egressHook` is a test-only hook that sees every decision.
- **API verification.** `http.NewResponseController(w).Hijack()` and `CloseWrite` on `*net.TCPConn` were compiled against and exercised by the tests under Go 1.27.1. For CONNECT, `r.Host` is the request-target's authority, not the Host header; the Host-mismatch test pins this.

### Step 3a: container kinds

- **What landed.** `containerSpec.kind` (`kindUnset` panics, then call, smoke, gate). The gate argv is built as the plan states. Call and smoke argv are unchanged, as the existing argv tests pin.

### Steps 3b and 3c: gates in containers

The two were committed together. This is a deviation from the plan's separate commits: removing the marker from `gateProcess` left `children.go` uncompilable without 3c's deletion. One critique gate covered both.

- **Paths.** The checkout is at `gates/<key>` and the home at `gates/<key>.home` (made `0700`), both bound at their host paths, with `HOME=<home>`.
- **The command** reaches the container as `-e LSF_GATE_COMMAND` from podman's environment.
- **The started marker.** It is `$HOME/.lsf-started-<seq>`.
  - A container that exits non-zero with no marker stops the run with `runtime-error` and returns no verdict.
  - Before the read, the runtime makes the home user-accessible, because a command may `chmod` it. Only a missing marker counts as not started.
- **Removal.**
  - After a cancel or a non-zero exit, the container is removed with `podman rm -f`.
  - A failed removal returns a `containerLeftError` (`runtime-error`), and the checkout and home are kept.
  - Trees are removed by `removeGateTree`, which applies a `u+rwx` chmod walk and then `RemoveAll`, without taking `wtSem`. Only the main repo's worktree prune takes it.
- **Reaping.**
  - Reaping is by label alone. The `children` map, the marker, `processes`, `killGroup`, `carryForward` and `children.json` writing are deleted.
  - A leftover `children.json` is logged (`children-dropped`) and removed. The pid it names is not signalled, and a test checks this.
  - The launch wipes every `gates/` entry but `logs` after reaping. A dry run skips this (critique fix).
- **Live smoke (`TestLiveGate`)** showed the plan's risks resolved:
  - attach without `-i` under `--log-driver=none` delivers both streams;
  - the home bind under `--read-only` works;
  - files are the user's;
  - a gate has no DNS;
  - git runs in the checkout;
  - a read-only tree is removed.
- **Test changes.**
  - `TestOrphanGateReaped`, `TestRunGatesRegistersAndCleansUp` and the two `children.json` tests were replaced.
  - New tests: `TestRunGatesContainers`, `TestRunGatesTimeoutRemovesContainer`, `TestRunGatesRemovesReadOnlyTrees`, `TestRunGatesLaunchFailure`, `TestRunGatesUnreadableHomeIsRed`, `TestRunGatesContainerLeft`, `TestOrphanGateContainerReaped` (a SIGKILLed helper launch), `TestRunGatesHostileGit` and `TestLegacyChildrenDropped`.
  - The unwritable-home launch-failure case was dropped. Once the runtime restores the home's mode after each command, `setup` can no longer make the next command's marker unwritable. A podman exit 125 covers the no-marker path.
- **Surprise.** None of the plan's design changed.

### Step 4: live checks

- **What landed.** Stub `claude` mode `sh` (runs `$STUB_SH`), and four live tests:
  - `TestLiveEgress`;
  - `TestLiveGatesSetupHosts`, which uses the npm tarball `is-number-7.0.0.tgz`;
  - `TestLiveGateTimeout`;
  - `TestLivePaidEgress`, gated on `LSF_LIVE_PAID=1`.
- **Decision.** curl reports a CONNECT refusal as `http_code` 000. The probe therefore prints `%{http_code}/%{http_connect}`.
- **The paid run, once.** Real Claude Code 2.1.294, haiku, low effort.
  - Cost $0.0037.
  - The proxy decisions were `allowed registry.npmjs.org:443` and `refused example.com:443`.
  - The call returned class ok with `{"registry":"200","example":"000 (curl failed: exit 56, \"CONNECT tunnel failed, response 403\"…)"}`.
- **The call with `hosts` unset.** The existing `TestLiveProbe` environment check already refuses any variable not on its list, so it already fails on a proxy variable. No new assertion was needed.

### Step 5: critique fixes, CLAUDE.md, the final suite

The CLAUDE.md clause and the step 3b/3c and step 4 critique fixes landed in 3b52c3b. The suite results are under Success criteria.

## Per-step critique findings

The critic was sonnet with the critique-code appendix. No round raised a Critical finding.

### Step 1 (cc3a72e)

- **Significant:** the comment contradicted the reset behaviour. *Fixed* in a7ce185.
- **Minor:** the reset-from-event deviation. *Kept*, judged acceptable.
- **Minor:** a 429 with another assistant error was untested. *Fixed*: test and doc row.
- **Minor:** a guard-killed 429 classes as `timeout`. *Noted for Phase 5*; the path predates this unit.
- **Minor:** the fallback wait is not timed in `TestBare429WaitsFallback`. *Noted*: covered by the `TestRetryClasses` `minDelay` row.
- **Minor:** a bare 429 could be an API per-minute limit. *Noted for Phase 5*: the class follows the requester's decision; a distinct telemetry tag was suggested.

### Step 2 (a7ce185)

- **Significant:** stale doc lines and code comment. *Fixed* (bf9d5bf).
- **Significant:** adversarial target forms were untested. *Fixed*: refusal rules, a table, Host-mismatch and absolute-form tests.
- **Significant:** no race tests. *Fixed*: `TestEgressCloseRacesHandlers`.
- **Minor:** no idle bound on a half-closed tunnel. *Partly fixed*: an error closes both ends. *Noted for Phase 5*: an idle half-closed tunnel lives until the container or the launch ends.
- **Minor:** handlers outlived `close()`. *Fixed* with `enter()`.
- **Minor:** counter wording. *Fixed*.
- **Minor:** empty labels. *Fixed*.
- **Minor:** numeric wildcard entries. *Noted*: parser side, outside Scope.
- **Minor:** re-pin uses exact list equality. *Kept*: the requirement says identical.
- **Minor:** the proxy is unauthenticated on loopback. *Documented* as a Known limit.

### Step 3a (bf9d5bf)

- **Significant:** the Declared gates argv in the doc was stale. *Fixed* in d3155b4, as planned.
- **Minor:** the zero kind was fail-open. *Fixed*: `kindUnset` panics.
- **Minor:** a gate spec with an empty home. *Fixed*: it panics.
- **Minor:** negative argv assertions were missing. *Fixed*.
- **Minor:** the bracket checks duplicate `SplitHostPort`'s. *Comment corrected*.
- **Minor:** idle tunnels. *Noted*, as in step 2.

### Steps 3b and 3c (d3155b4)

- **Significant:** a gate that chmods its home could turn red into a run stop. *Fixed* (3b52c3b).
  - The home's mode is restored before the marker read, and only ENOENT means not started.
  - The forge or delete case is documented.
  - A separate marker bind was not adopted, since a command could still forge it.
- **Minor:** a nil `ProcessState` deref. *Fixed*.
- **Minor:** a symlink swapped in during the chmod walk. *Noted*: unreachable, since no container runs during removal, and the stdlib `Fchmodat` has no NOFOLLOW.
- **Minor:** a dry run wiped trees under a live orphan. *Fixed*.
- **Minor:** the group SIGKILL after reaping (pgid reuse window). *Noted*: shared with calls and accepted in `s2-sandbox`.
- **Minor:** doc accuracy. *Fixed*.
- **Minor:** the hostile-.git test covered the green path only. *Fixed*: the timeout path and a leftover tree wiped at launch.
- **Minor:** `TestLiveGate`'s offline gate could be red for a missing curl. *Fixed*: it asserts the resolve failure.
- **Minor:** the not-run case for a client killed by a signal is reachable only through the stub. *Noted for Phase 5*.

### Step 4 (00713d4)

- **Significant:** the direct-route check only showed that DNS was absent. *Fixed*: name (exit 6) and literal IP (exit 7) are both asserted, and the doc wording follows.
- **Minor:** the paid test did not tie the traffic to the call. *Fixed*: it now asserts class ok and a value, and the doc says "the proxy saw". The test was not re-run (n = 1 by decision); the logged run shows class ok.
- **Minor:** the paid test failed instead of skipping without the pin or the credentials. *Fixed*.
- **Minor:** the exact decision count is brittle. *Kept* deliberately.
- **Minor:** the offline gate could be red for any reason. *Fixed*.
- **Minor:** `egressHook` race, `liveNet` semantics, slice aliasing. *Noted*: no race exists today.

## Success criteria

| # | Criterion | Status | Target | Achieved | Evidence |
|---|---|---|---|---|---|
| S1a | Egress proxy on `127.0.0.1:0` with hosts; none without | pass | proven | proven | `TestEgressProxy` (listen address), `TestPinHostsEgress` (none for nil or empty) |
| S1b | CONNECT :443 to a pinned host allowed (case, trailing dot, IP literal); all else refused | pass | proven | proven | `TestEgressAllowed` (30 cases), `TestEgressProxy` (end to end, plain GET, 502), `TestEgressCloseRacesHandlers` |
| S1c | Call argv: splice and proxy variables with hosts, none without | pass | proven | proven | `TestCallContainerEgressArgv`, `TestCallContainerArgv` (negatives), live `TestLiveProbe` env allow-list |
| S1d | Gate argv: setup splice and variables with hosts, else `none`; gates `none`; host-path binds; name and label | pass | proven | proven | `TestGateArgv`, `TestRunGatesContainers` |
| S1e | Shared gate home; removed after a chmod pass; read-only module cache removed | pass | proven | proven | `TestRunGatesContainers` (setup writes `$HOME/x`, the gate reads it), `TestRunGatesRemovesReadOnlyTrees`, live `TestLiveGate` |
| S1f | Timeout and stop remove by `podman rm -f`; label reaping covers gates; marker and `children.json` gone | pass | proven | proven | `TestRunGatesTimeoutRemovesContainer`, `TestRunGatesStopKillsGroup`, `TestOrphanGateContainerReaped`, `TestLegacyChildrenDropped`, live `TestLiveGateTimeout` |
| S1g | Hostile `.git` gate: no canary through removal, with a positive control | pass | proven | proven | `TestRunGatesHostileGit` (control, green path, timeout path, leftover wipe) |
| S1h | Declared gates states the mount path and why | pass | evidenced | evidenced | `lsf-runtime.md` Declared gates, "The checkout and home are mounted at their host paths" |
| S1i | Bare 429 → `session-limit`, waits `sessionLimitFallback` | pass | proven | proven | `TestClassifySynthetic`, `TestRetryClasses` `bare429,ok`, `TestBare429WaitsFallback` |
| S1j | Live: registry reached and undeclared host refused (paid + stub); setup under hosts installs a dependency a no-network gate uses | pass | proven; paid evidenced | proven (stub live tests under `LSF_LIVE_REQUIRE=1`); paid evidenced (n = 1) | `TestLiveEgress`, `TestLiveGatesSetupHosts`, `TestLivePaidEgress` ($0.0037) |
| S1k | `go test -race ./...` passes on a quiet machine | see below | proven | evidenced | full suite including live and parity, under load (an unrelated `xtask` process on 7 of 8 cores) |
| S2 | No host git in a gate checkout after its first container | pass | proven | proven | S1g, plus code reading: after `gateCheckout` only `removeGateTree` (chmod and unlink) touches the checkout, and `pruneGateWorktrees` runs in the main repo |

**S1k.** The full suite passed with `LSF_LIVE_REQUIRE=1`, `PROMPTPARITY_REQUIRE_NODE=1` and `GRAPHPARITY_REQUIRE_NODE=1` at 3b52c3b. The host was under heavy unrelated load (load average about 9 on 8 cores), so the run does not meet the quiet-machine condition. The upgrade path is to re-run the same command on an idle host: `cd lsf-runtime && LSF_TEST_IMAGE=localhost/lsf-ref@sha256:949f2220… LSF_LIVE_REQUIRE=1 go test -race -count=1 ./...`.

## Deviations (for the completion report)

- **Mount path:** host paths, not `/work` (requester). The unit description's premise was false.
- **Egress proxy start:** it starts at `PinHosts`, before any agent call, not literally at `Run` start.
- **`NO_PROXY`:** extended to `127.0.0.1,localhost,::1` (requester).
- **The `*.` wildcard rule:** defined here (requester).
- **`CLAUDE.md`:** a clause outside Scope.
- **Bare 429 reset:** taken from a rejected event when present, not zero.
- **Steps 3b and 3c:** one commit.
- **The home's mode** is restored after each command (a marker-read fix). A `setup` that `chmod`s `$HOME` no longer carries that mode to the gates.

## Remaining phases
Complete Phase 5 (Review) as defined in the phronithm:feature skill.
