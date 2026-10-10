# s2-egress-gates: completion report

Unit `s2-egress-gates` of `docs/plans/lsf-improvements-breakdown.md`, on branch `lsf-workflow`, base f6f0aef.

- **Plan:** `docs/plans/s2-egress-gates-plan.md`.
- **Implementation summary:** `docs/plans/s2-egress-gates-implementation.md`.
- **Commits:** cc3a72e, a7ce185, bf9d5bf, d3155b4, 00713d4, 3b52c3b, 76e6d08, and the Phase 5 fix commit that carries this report.

## Done-condition verdict

**Met.** Each clause is proven by a test, except two:
- The paid live cell is evidenced (n = 1, opt-in), as the requester decided.
- The quiet-machine condition on the final suite is not met (last bullet below).

- **Egress proxy on `127.0.0.1` at an ephemeral port when `hosts` is set; none otherwise.** It starts at `PinHosts`, which the orchestrator calls before any agent call (see Deviations). Tests: `TestEgressProxy`, `TestPinHostsEgress`.
- **Allow and refuse rules.** CONNECT to port 443 of a pinned host is allowed: matched case-insensitively, trailing dot ignored, an IP literal only by the same literal. Everything else is refused: other ports, unlisted hosts, plain HTTP, wildcard apexes, lookalikes, zone ids, `*` targets and empty labels. Tests: `TestEgressAllowed`, `TestEgressProxy`, `TestEgressCloseRacesHandlers`, `TestEgressCloseEndsTunnels`.
- **Call containers.** With `hosts` they get the splice, `HTTPS_PROXY`, `HTTP_PROXY` and their lower-case forms; with `hosts` unset they get neither. Tests: `TestCallContainerEgressArgv`, `TestCallContainerArgv`, and live `TestLiveProbe`'s environment allow-list.
- **`setup` and gates in their own containers.** `setup` gets the egress splice under `hosts` and `--network none` otherwise. Gates get `--network none`. Tests: `TestGateArgv`, `TestRunGatesContainers`, live `TestLiveGate` and `TestLiveGatesSetupHosts`.
- **The gate home.** `setup` and the gates share it. It is removed with the checkout after both trees are made user-writable, and a read-only module-cache tree is removed. Tests: `TestRunGatesRemovesReadOnlyTrees`, live `TestLiveGate`.
- **Gate containers are named and labelled.** A timeout or a stop removes them by `podman rm -f`. Launch reaping by label covers them. The `lsf-gate:` marker is gone. Tests: `TestRunGatesTimeoutRemovesContainer`, `TestRunGatesStopKillsGroup`, `TestOrphanGateContainerReaped`, `TestLegacyChildrenDropped`, live `TestLiveGateTimeout`.
- **No host git in a gate checkout after a container has run there.** `TestRunGatesHostileGit` covers this. Its gate writes filters (the checkout's and a nested repo's) and `core.hooksPath`, commits a gitlink and dirties the tree. A positive control shows host `git status` touching the canary. The canary stays absent through the green path, the timeout path and a leftover tree wiped at the next launch.
- **The Declared gates section** states the mount path (the host path) and why.
- **A bare 429 is `session-limit`.** Tests: `TestClassifySynthetic`, `TestRetryClasses`, `TestBare429WaitsFallback`.
- **Live checks.**
  - Stub-`claude` Bash got the registry, and was refused `example.com` (403 on CONNECT), plain HTTP (403), the direct route by name (no DNS) and by literal IP (no route) (`TestLiveEgress`).
  - `setup` under `hosts` installed an npm tarball that a `--network none` gate used (`TestLiveGatesSetupHosts`).
  - One real Claude Code 2.1.294 call (haiku, low effort, $0.0037): the egress proxy allowed `registry.npmjs.org:443` and refused `example.com:443`, and the call returned both results (`TestLivePaidEgress`, `LSF_LIVE_PAID=1`).
- **The tests pass.** `go test -race ./...` passes with `LSF_LIVE_REQUIRE=1`, `PROMPTPARITY_REQUIRE_NODE=1` and `GRAPHPARITY_REQUIRE_NODE=1`. Every run was on a host loaded by an unrelated process (load average about 9.7 on 8 cores), so R35's quiet-machine condition is **not met**; the achieved rigour is evidenced. To upgrade it, re-run the same command on an idle host.

## Deviations from the unit entry's scope and publishes

1. **The gate checkout is mounted at its host path, not `/work`** (requester, 2026-10-10).
   - The unit description's premise, "a plain-clone gate checkout holds none", is false. `copyRepoSetup` writes `core.hooksPath` naming the checkout's host path, and `core.excludesFile`/`core.attributesFile` under its `.git/lsf-global/`.
   - The gate home is mounted at its host path too, with `HOME` set to it.
2. **The egress proxy starts at `PinHosts`, not literally at `Run` start.** `PinHosts` is the first point the hosts are known, and it precedes every agent call.
3. **`NO_PROXY`/`no_proxy` is `127.0.0.1,localhost,::1`, not `127.0.0.1`** (requester, 2026-10-10).
4. **The `*.` wildcard rule is defined here** (requester, 2026-10-10): subdomains at any depth, not the apex.
5. **A bare 429 takes its reset from a rejected `rate_limit_event` when the stream carries one,** and the fallback otherwise. The plan said zero.
6. **Two additions outside Scope.**
   - `CLAUDE.md`'s `lsf-runtime` paragraph gains a clause.
   - The config's reserved `env` names gain `LSF_GATE_COMMAND` and `LSF_GATE_SEQ` (in `lsf-runtime/runtime`, so within Scope, but not named in the entry).
7. **Gates' `setup` no longer inherits the runtime's environment.** It runs with the image's environment plus the passthrough list, as the decided design states. This is a behaviour change for any plan whose `setup` relied on the host environment, such as credentials for a private registry.
8. **The gate home's mode is restored after each command** (a marker-read fix). A `setup` that `chmod`s `$HOME` does not pass that mode to the gates.

## Deferrals

None. Everything in the unit's scope is implemented.

## Discoveries affecting sibling or downstream units

1. **Gates now run under the image.** A plan's gate commands need their toolchain in the image (`container/Containerfile`, extended by the user), not on the host. `shakedown-fixture` and any unit that runs declared gates live must build an image with the project's toolchain. `lsf-ref` has `curl`, `git` and `tar`, and no node or Go.
2. **`setup` gets no host credentials or environment.** A private registry needs its token passed by name through the config's `env` list. `runtime-cli-drive` (configuration surface) may want to say so to the requester.
3. **A leftover `children.json` from before this unit is dropped, and its pids are not signalled.** A launch SIGKILLed under the old runtime with host-side gate shells still running leaves those shells running once. The new launch then wipes their checkouts. This is a one-time transition on upgrade; stop old launches before upgrading.
4. **Open hardening candidates, not done.**
   - The egress proxy does not check the resolved address. A `*.` entry under a domain whose DNS others control can reach a host-local address on 443, and the design records this under Known limits.
   - A tunnel has no idle timeout; it ends with its container or the launch.
   - A dry run binds the egress listener.

   These are candidates for `programme-close`'s register, or for the unit that next touches `egress.go`.
5. **Phase 5 notes, minor and not fixed.**
   - A guard-killed call whose stream already holds a 429 classes as `timeout`.
   - The not-run status for a podman client killed by a signal is exercised only through the stub.
   - A failed rename in `gateCheckout` leaves the new home until the next run at that key.
   - A bare 429 could be an API per-minute limit and would gate the whole runtime for `sessionLimitFallback`. That follows the requester's decision; a distinct telemetry tag was suggested.

## Standing obligations

- **Headless pin.** Discharged. Checked `DISABLE_AUTOUPDATER=1 ~/.local/share/claude/versions/2.1.294 --version` → `2.1.294 (Claude Code)`; the paid call ran that binary.
- **Guarded host git in clones.** Discharged. No host git was added in a clone, merge clone or gate checkout. After `gateCheckout`, which uses the clone helpers before any container runs, only `removeGateTree` (chmod and unlink) touches a gate checkout. `pruneGateWorktrees` runs in the main repo. `TestRunGatesHostileGit` holds this.
- **Design of record.** Discharged. `docs/design/lsf-runtime.md` was amended in the same commits as the behaviour:
  - Permissions: A call's launch, Teardown, Network, Declared gates, Runtime changes, Known limits;
  - Runtime: the classification table, Interface (`PinHosts`, `RunGates`), Journal and state dir, Supervision → Orphans, Not in this unit.
- **Register dispositions.** No register rows are assigned to this unit, so there are none to dispose of. The unit closes R14 by its description; the register row itself is `programme-close`'s to write.

## External-gate re-verification

All pins were confirmed at Phase 4 entry (2026-10-10). There was no drift.
- `lsf-toolchain`: Go 1.27.1.
- `claude-code-headless`: 2.1.294 via the pinned binary.
- `podman`: 6.1.3 rootless, `pasta` 2026_10_02.cba3570.

## Execution signals

- **Context at completion:** comfortable.
- **Decisions taken mid-implementation, not at a design pass:**
  - the reset-from-event for a bare 429;
  - folding 3c into 3b's commit;
  - restoring the home's mode before the marker read;
  - skipping the gate wipe in a dry run;
  - reserving `LSF_GATE_*`;
  - making a nil `ProcessState` not-run.

  Each was a fix in response to a critique or a compile dependency, not a change of approach.
- **Critique gates:** seven rounds (five per-step, plus a Phase 5 review and a full-diff critique). No Critical finding. The Significant ones were fixed; the remainder are listed above and in the implementation summary.

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
