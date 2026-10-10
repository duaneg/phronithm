# s2-egress-gates: egress proxy, setup and gates in containers, bare-429 session limit

Work unit `s2-egress-gates` of `docs/plans/lsf-improvements-breakdown.md` (plan at f6f0aef, branch `lsf-workflow`, base commit f6f0aef). Phases 1–3 compressed in plan mode; one critique gate on this combined document.

## Context

R14: declared gates still run on the host with the runtime's environment and network, and a call's Bash has no network even when the plan's gate block sets `hosts`. The decided container design (`docs/design/lsf-runtime.md` → Permissions, c4fe2c1) gives this unit three pieces: the egress allow-list proxy (change 4), `setup` and gate commands in their own containers (change 5), and gate teardown and reaping by container name and label (change 3, for gates). It also adds the requester decision that a bare 429 is `session-limit` (R18).

## Unit entry (verbatim)

### s2-egress-gates
- **Type**: feature
- **Risk**: high — gate commands run code the stages wrote; the egress proxy is new Go work with a Python reference only
- **Clarity**: exploratory — one open decision: whether the gate checkout stays mounted at `/work` or moves to its host path, as calls' clones did
- **Iteration**: write-once
- **Effort**: medium
- **Scope**: `lsf-runtime/runtime`, `lsf-runtime/testdata`, `docs/design/lsf-runtime.md::runtime`, `docs/design/lsf-runtime.md::permissions`
- **Publishes**: the egress allow-list proxy; `setup` and gate commands in containers; the `session-limit` class for a bare 429
- **Depends on**: s2-sandbox (logical, the container launch, its configuration, image checks, teardown by name, the stub `podman` and the `gitenv` guards; serialises the runtime files); s2-boundary-common (logical, `Hosts()` and its pin, the plain-clone gate checkout); declared-gates (logical, `RunGates`); s2-boundary-decision (design, the egress rule and the gate launch); container-probe (knowledge, `egress-proxy.py` and cells P8, Z3, Z8)
- **External gates**: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`); `claude-code-headless` (environment, Claude Code 2.1.294); `podman` (environment, 6.1.3 rootless, `pasta` 2026_10_02.cba3570)
- **Done condition**: With `hosts` set, the runtime starts an egress proxy with the launch on `127.0.0.1` at an ephemeral port. It allows `CONNECT` to port 443 of a pinned host, matched case-insensitively with a trailing dot ignored and an IP literal only by the same literal, and refuses everything else, plain HTTP included. Every agent call's container splices it and carries `HTTPS_PROXY`, `HTTP_PROXY`, their lower-case forms and `NO_PROXY=127.0.0.1`; with `hosts` unset there is no egress proxy or splice. `setup` and each gate command run in their own container as the Declared gates section states: `setup` with the egress splice when `hosts` is set and `--network none` otherwise, gate commands with `--network none`, and a per-checkout gate home shared by both, removed with the checkout after both trees are made user-writable; a test removes a Go module cache written read-only. Gate containers are named and labelled, a timeout or stop removes them by `podman rm -f`, launch reaping by label covers them, and reaping by the `lsf-gate:` marker is gone. No host git runs in a gate checkout once a container has run there, or any that does goes through the `gitenv` guards; a test whose gate writes a filter or `core.hooksPath` into the checkout's `.git/config` and commits a gitlink finds nothing of it run on the host through the checkout's removal. The Declared gates section states the gate checkout's mount path and why. A call that ends with no clean result, status 429 and no assistant error is `session-limit`, waiting `sessionLimitFallback`, not `transient` (requester decision, 2026-10-10); a test pins it. Tests cover the proxy's allow and refuse cases and the gate argv. A live check shows Bash in an agent call fetching from a declared package registry and refused an undeclared host, and `setup` under `hosts` installing a dependency the gates then use. The tests pass.
- **Description**: Runtime changes 4's egress proxy, 5, and 3 for gates. `setup` under `hosts` and the shared gate home were never run in a container (Permissions, Known limits), so the live check is their first evidence. Closes R14. From `s2-sandbox`: calls' clones mount at their host path because a clone's local config holds absolute host paths; a plain-clone gate checkout holds none, which is the case for `/work`. The read-only `.git` binds alone were bypassable (renamed `.git`, `.git/commondir`, a gitlinked nested repo), hence the guard clause. The 429 clause is there because no container call emitted a `rate_limit_event` (R18); it lives here as the next unit writing `lsf-runtime/runtime`. **Flag:** three surfaces with the design sections; `testdata` is the stub `podman`'s gate cases, so the context is the runtime's gate launch and the proxy.

Standing obligations carried: **Headless pin** (check `~/.local/share/claude/versions/2.1.294` with `DISABLE_AUTOUPDATER=1`); **Guarded host git in clones**; **Design of record** (amend `lsf-runtime.md` Permissions/Runtime in the same commit as the behaviour); **Register dispositions** (no rows assigned to this unit; report says so).

## Requester decision (this session)

- **Mount path: the host path** (requester, 2026-10-10). The unit description's premise ("a plain-clone gate checkout holds none") is false: `copyRepoSetup` (`runtime/clone.go:258`) writes `core.hooksPath` naming the checkout's host path, and `core.excludesFile`/`core.attributesFile` naming copies under its `.git/lsf-global/`. At `/work` they would dangle, so git that a gate itself runs (tests that call `git status`, builds that call `git describe`) would ignore the global excludes and attributes. The host path also reuses the call mount code and the stub `podman` unchanged. The cost is that gate code sees the state dir's host path. This is a deviation from the description's premise, to be recorded in the completion report.
- **`*.` wildcard entries match subdomains only** (requester, 2026-10-10): `*.github.com` matches `api.github.com` and `a.b.github.com` (any depth), not `github.com`. The parser (`plan/gates.go:hostProblem`, `docs/plan-graph.md`) already accepts the form; no unit had defined its match.
- **The live check includes one paid call** (requester, 2026-10-10): a real Claude Code 2.1.294 call (haiku, low effort) whose Bash curls the declared registry and an undeclared host, judged from the egress proxy's telemetry, not the model's report; plus the stub-claude live test for the scripted cells.
- **`NO_PROXY=127.0.0.1,localhost,::1`** and lower-case (requester, 2026-10-10), for calls and `setup`: loopback stays in the container's own netns, so it widens nothing. Deviation from the done condition's literal value, recorded.
- Derived from it: the **gate home is also mounted at its host path**, `HOME=<that path>` (not `/home/lsf`). One mount convention throughout, and the stub `podman` (which exports `-e` on the host) works without remapping.

## Success criteria

| # | Criterion | Target rigour |
|---|---|---|
| S1 | The unit's done condition, verbatim above, clause by clause (S1a–S1k below) | as below |
| S1a | Egress proxy on `127.0.0.1:0` starts when the pinned `hosts` is non-empty; none otherwise | proven |
| S1b | CONNECT to `<pinned host>:443` allowed (case-insensitive, trailing dot ignored, IP literal only by the identical literal); everything else, including plain HTTP, other ports and unlisted hosts, refused | proven |
| S1c | Call argv: `-T <egress port>` splice plus `HTTPS_PROXY`, `HTTP_PROXY`, `https_proxy`, `http_proxy`, `NO_PROXY`/`no_proxy=127.0.0.1` with hosts; none of the proxy variables or splice without | proven |
| S1d | Gate argv: `setup` with the egress splice and proxy variables when hosts are set, `--network none` otherwise; gate commands `--network none`; checkout and home bound at host paths; named and labelled | proven |
| S1e | Gate home shared by `setup` and the gates, removed with the checkout after a chmod pass; a read-only Go-style module cache is removed | proven |
| S1f | Timeout and stop remove gate containers by `podman rm -f`; reaping by label covers gate containers; the `lsf-gate:` marker and `children.json` machinery are gone | proven |
| S1g | Hostile `.git` test: a gate that writes a filter and `core.hooksPath` into the checkout's `.git/config` and commits a gitlink finds no canary written by host git through the checkout's removal | proven |
| S1h | Declared gates section states the mount path and why | evidenced (doc review) |
| S1i | Bare 429 (no clean result, `api_error_status` 429, no assistant error) is `session-limit`, waits `sessionLimitFallback` | proven |
| S1j | Live: Bash in an agent call fetches from a declared registry and is refused an undeclared host (one paid haiku call plus the stub live test); `setup` under hosts installs a dependency a `--network none` gate then uses | proven (live tests, `LSF_LIVE_REQUIRE=1` run); the paid cell evidenced (n = 1, opt-in) |
| S1k | `go test -race ./...` passes (quiet-machine run, R35) | proven |
| S2 | No host git in a gate checkout after its first container, without the guards (standing obligation) | proven by S1g plus code reading |

S1j: the paid call shows the real CLI's Bash inherits the proxy variables and reaches only declared hosts (telemetry `egress-allowed`/`egress-refused`); the stub-claude live test covers plain HTTP and the direct route deterministically.

## Exploration findings

- `runtime/gates.go`: `RunGates` → `gateCheckout` (plain clone at `<stateDir>/gates/<key>`) → `gateProcess` (host `sh -c`, process group, `lsf-gate:` marker in `children.json`) → `gateRemove` (`os.RemoveAll` plus `git worktree list/prune` in the **main repo**, not in the checkout). No host git runs in the checkout after its commands today.
- `runtime/container.go`: `containerSpec` and `containerArgv` (always binds claude and the plugin, call env, and a `pasta` splice to the credential proxy unless `network` is set), `removeContainer` (rm -f with a 60 s bound and a `container exists` race check), and `smoke`.
- `runtime/call.go` `spawn`: the cancel/remove pattern to mirror. SIGTERM to the podman group, `WaitDelay` = KillGrace, `removeContainer` after a cancel or a non-zero exit, then SIGKILL to the group.
- `runtime/children.go`: `reapContainers` (label) plus marker-based gate reaping (`processes`, `killGroup`, `matchesChild`, `carryForward`, `addChild`…), used only by gates and their tests.
- `runtime/runtime.go`: `PinHosts` and `Hosts`. PinHosts is called once by the orchestrator's `gates-pin` step before any agent call (`orchestrator/loop.go:347`), *after* `Run` started the credential proxy. The orchestrator's `Runtime` interface has `PinHosts(hosts []string)` with no error return.
- `runtime/proxy.go`: the credential proxy's pattern (listener, `http.Server`, refusal counting, an event once, close at launch end).
- `runtime/classify.go`: a 429 result with no assistant error falls through to `transient`.
- `testdata/podman`: `run` without `-i` execs the command with `</dev/null`, cd's to `-w`, exports `-e K=V`, and accepts `--network` values unchecked. Host-path binds need no remapping.
- `testdata/claude`: modes; no mode runs an arbitrary shell command.
- Python reference: `docs/investigations/container-probe/egress-proxy.py`.

## Candidate approaches

**Egress proxy implementation**
1. **In-process Go `http.Server` handling CONNECT by hijack** (chosen). Stdlib only, same lifecycle and logging as the credential proxy, and testable with an injected dialer.
2. Ship the Python reference as a sidecar process. Rejected: it adds a Python runtime dependency and a second process to supervise and reap, and the module is Go stdlib-only by decision.
3. Enforce by IP in pasta or nftables. Rejected: names resolve to changing CDN addresses, there is no DNS in the container, and rootless podman cannot set host nftables.

**When the egress proxy starts**
1. **In `PinHosts`, when hosts are non-empty** (chosen). That is the first point the hosts are known, and it precedes every agent call. A bind failure sets the sticky `runtime-error` stop, so the next call boundary ends the run. The orchestrator interface stays unchanged, which is in scope.
2. At `Run` start. Rejected: the hosts are not known until the body pins them.
3. Lazily at the first call or gate. Rejected: it puts a start race and an error path in two places for no gain.

"Starts with the launch" is read as "before any agent call of the launch". The report records this.

**Gate launch**
1. **Generalise `containerSpec` with a kind (call | gate | smoke), sharing argv building** (chosen). Isolation flags stay in one place.
2. A separate `gateArgv` builder. Rejected: it duplicates the isolation flags that must stay identical.

## Design

### Egress proxy (`runtime/egress.go`, new)

```go
type egressProxy struct {
    rt    *Runtime
    ln    net.Listener
    srv   *http.Server
    done  chan struct{}
    allow map[string]bool              // normalised entries
    dial  func(ctx context.Context, network, addr string) (net.Conn, error) // net.Dialer.DialContext; tests inject
    mu       sync.Mutex
    conns    map[net.Conn]struct{}     // both ends of every tunnel, closed at close()
    wg       sync.WaitGroup            // tunnel copy goroutines
    refusals, allows int
}
func normaliseHost(h string) string        // lower-case, one trailing '.' trimmed
func (rt *Runtime) startEgress(hosts []string) (*egressProxy, error) // listens 127.0.0.1:0
func (e *egressProxy) allowed(target string) (host string, ok bool)  // SplitHostPort; port must be "443"; host normalised; exact entry, or a `*.` entry whose suffix ("." + rest) ends the host with at least one label before it
func (e *egressProxy) ServeHTTP(w, r)      // non-CONNECT → 403; disallowed → 403; dial (30 s) of net.JoinHostPort(<normalised target host>, "443") failure → 502; else hijack, write "HTTP/1.1 200 Connection established\r\n\r\n", flush the bufio reader's buffered bytes upstream, then two io.Copy goroutines with CloseWrite half-close; both conns closed when both directions end
func (e *egressProxy) close()              // srv.Close, close both ends of all tunnels, wg.Wait, wait for done; log refusal and allow counts
func (rt *Runtime) egressPort() string     // "" when none
```

- Matching: the parser keeps entries as written (`plan/gates.go:156`), so each entry is normalised here: brackets stripped from an IPv6 literal, lower-cased, one trailing dot trimmed. The target is normalised the same way (`SplitHostPort` already unbrackets). An IP-literal target matches only an identical normalised entry string, never a wildcard; no resolution happens before matching. The normalised target name is dialled, so resolution happens on the host (a Known limit, as already recorded).
- Events, bounded: an `egress` event at start (port, host count); the first refusal and the first allow of the launch are logged and emitted (`egress-refused`, `egress-allowed`, with target); after that only counts, logged at close. The live tests read the first-event records plus the counts; the paid test's assertions use a test-only hook (`e.onDecision`) to collect every decision.
- `ReadHeaderTimeout` is 30 s. Tunnels have no idle timeout; they end with either side or at launch end.
- The Runtime field is `egress atomic.Pointer[egressProxy]`. `PinHosts` starts it when hosts are non-empty. A second `PinHosts` with the same hosts does nothing; with different hosts it sets `runtime-error` (the orchestrator pins once per launch, so this never fires in practice). A start failure calls `rt.setStop(StopRuntimeError, "egress proxy: …")`; `Hosts()` then holds hosts with no port, so `setup` fails closed (`--network none`) and the next call boundary ends the run. `Run` closes the proxy after the body, next to the credential proxy.

### Call argv (`container.go`)

- Network for kind call: `pasta:--splice-only,-T,<inject>` plus `,-T,<egress>` when `egressPort() != ""`.
- With egress: `-e HTTPS_PROXY=http://127.0.0.1:<p>`, plus `HTTP_PROXY`, `https_proxy` and `http_proxy`. `NO_PROXY`/`no_proxy=127.0.0.1,localhost,::1`, always.
- The egress port is read once per container build, so argv and env agree.

### Gates in containers (`gates.go`)

- **Paths:** checkout `<stateDir>/gates/<key>` and home `<stateDir>/gates/<key>.home`, both bound rw at their host paths, with `-w <checkout>`.
- **Argv (kind gate):** `run --rm --init --pull=never --log-driver=none --name lsf-<hash8>-gate-<launchID>-<seq> --label lsf.state=<hash> --userns=keep-id --cap-drop=all --security-opt no-new-privileges --read-only --tmpfs /tmp:rw,exec --network <none | pasta:--splice-only,-T,<egress>> -v <checkout>:<checkout>:rw -v <home>:<home>:rw -w <checkout> -e HOME=<home> -e LSF_GATE_COMMAND [-e proxy vars] -e NO_PROXY… [-e passthrough…] <image> /bin/sh -c <gateScript>`.
  - There is no `-i`, so stdin is `/dev/null`.
  - There are no claude or plugin binds, no `ANTHROPIC_*`, and no `SHELL`, `DISABLE_AUTOUPDATER` or `CLAUDE_CODE_*`.
  - `LSF_GATE_COMMAND`'s value comes through podman's environment, so the command stays out of argv and the argv stays fixed.
- **gateScript:** `: > "$HOME/.lsf-started-$LSF_GATE_SEQ" && eval "$LSF_GATE_COMMAND"`. `LSF_GATE_SEQ` is passed with `-e`.
  - **Marker absent and exit non-zero, whatever the code** → the command never ran (an image, mount, pasta or name fault, or an unwritable home). That sets the `runtime-error` stop, quoting the log tail, and returns no verdict, since under R23 a red verdict would replay from the journal forever. Marker absent with exit 0 (a gate that deleted it) counts as ran.
  - The marker is read and removed after every command, whatever its exit.
  - A gate that deletes the marker and fails can only turn its own red into a run stop.
- **gateProcess:** `exec.CommandContext(podman, argv…)` with `Setpgid`. Cancel sends SIGTERM to the group, with `WaitDelay` = KillGrace.
  - After Wait: if the run was cancelled (timeout, stop or kill) or the exit is non-zero, `removeContainer(name)` runs; then SIGKILL goes to the group.
  - A removal failure sets `runtime-error`. `gateProcess` returns a `containerLeftError`; `RunGates` replaces its deferred `gateRemove` with a check of a `left` flag, so the checkout and home are **not** removed while the container may run (logged). The error is returned even when an earlier stop already holds the sticky slot (first-wins), and the log names the container. The next launch's reaping, then its leftover-gates wipe, clear them. Test: `STUB_PODMAN_RM_EXIT` with `STUB_PODMAN_DETACH` → the checkout survives and the run stops `runtime-error`.
  - Statuses: green (exit 0), red (any other exit from a command that ran, including 137 for a container OOM or kill, which R23 already accepts as replaying red), not-run for a timeout or a podman client killed by a signal. The log holds podman's own stderr interleaved with the gate's, since attached mode cannot separate them; the summary may quote a podman line.
- **Network:** `setup` gets the egress splice and proxy variables when `rt.egressPort() != ""`, and `--network none` otherwise. Gates always get `--network none`.
- **Home creation:** `gateCheckout` creates `<key>.home` (0700) after its stale removal and before the rename into place.
- **Removal (`removeGateTree`):** for both the checkout and the home, walk the tree and chmod each directory u+rwx (no symlinks followed, `Lstat`), then `os.RemoveAll`. It is used by `removeGateDir` for stale and final removal and by `gateCheckout`'s stale removal. The tree walk runs outside `wtSem`; only the main-repo `worktree list/prune` takes it (M-12).
- **Leftover gates wipe at launch:** after reaping, `wipeScratch`'s pass also removes every `gates/*` entry except `logs` (checkouts, homes, `.partial`s left by a crashed launch), through `removeGateTree`.
- **Before the first command,** `gateCheckout` is the only host git in the checkout, and it already runs through the clone helpers. After any container has run, only chmod and `RemoveAll` touch it, so no host git runs (S2).
- **Reaping:** `reapOrphans` reduces to `reapContainers`. The gate-marker code is deleted: `children` map, `addChild`, `setChildPid`, `removeChild`, `saveChildrenLocked`, `carryForward`, `processes`, `killGroup`, `running`, `matchesChild`, `gateMarkerPrefix` (`gateSeq` stays, numbering gate containers). A leftover `children.json` from an earlier runtime is logged once (`children-dropped`, its entries) and removed at launch; it is not acted on. Tests that depend on it are rewritten or deleted: `gates_test.go` `TestOrphanGateReaped`, `TestRunGatesRegistersAndCleansUp`, `TestRunGatesStopKillsGroup`; `supervise_test.go:226` and `:493-501` (`children-unreadable`/`children-dropped`); `call_test.go:124`.
- **Launch check:** the smoke container stays a call-kind container. No gate-specific smoke is added; the gates rely on `sh`, which the smoke already exercises.

### Bare 429 (`classify.go`)

After `classifyResult` and the `rate_limit` assistant-error branch: `len(AssistantErrors) == 0 && r != nil && r.IsError && r.APIErrorStatus == 429` → `verdict{class: classSessionLimit}` with zero `resetsAt`. `agent.go` already waits `SessionLimitFallback` for a zero reset.

### Docs (`lsf-runtime.md`, same commits as the behaviour)

- **Declared gates:** the argv as built; the mount path (host path) and why; the gate home at its host path; the started marker and the launch-failure stop; "No git runs in a gate checkout after its first command" kept, with the guard obligation noted.
- **Teardown:** the `lsf-gate:` marker sentence is replaced with the statement that gates are reaped by label.
- **Network:** the wildcard rule; `NO_PROXY` value; evidence gains the live tests.
- **Known limits:** the Evidence bullet is updated (setup under hosts, the gate home and a call with `hosts` unset are now run live; still n = 1).
- **Runtime → Interface** (line ~457): `PinHosts` now confines calls and `setup`. **Journal and state dir** (~553): the `children.json` row goes; `gates/<key>.home` added. **Not in this unit** (~587): the deferred "confining calls and setup to hosts" item goes.
- Header comments: `runtime.go:48` (children field), `children.go` file comment.
- **Runtime changes:** this unit's items are marked as landed, without history narration (current state: the list states what the runtime does).
- **Runtime section:** classification gains the bare-429 rule.
- `CLAUDE.md`'s `lsf-runtime` paragraph mentions the egress proxy (one clause). That is outside Scope, as `s2-sandbox`'s was, and is recorded as a deviation.

### Risks and unknowns

| Risk | Resolution | When |
|---|---|---|
| Attached stdout and stderr without `-i` under `--log-driver=none` reach the client (gate logs) | live test L-gate | during step 4 |
| podman creates the bind target for the home path under `--read-only` | live test (calls already bind clone host paths) | step 4 |
| `curl` honours `HTTPS_PROXY` from the environment in the image (Debian curl does) | live test | step 4 |
| Live tests need internet and the npm registry | they skip as other live tests do; `LSF_LIVE_REQUIRE=1` run recorded | step 4 |
| A hijacked tunnel outlives `srv.Close` | tunnels tracked and closed in `close()`; unit test | step 2 |
| R35 flaky timing tests | quiet-machine run for the final suite | Phase 5 |

API verification: only stdlib `net/http` Hijacker and `CloseWrite` (`*net.TCPConn`), checked against Go 1.27.1 source during step 2. podman flags are as already used by `s2-sandbox`, except the absence of `-i` in an attached run, which is checked live.

## Plan critique (round 1, phronithm:critic, sonnet): passed, no Critical

| Finding | Disposition |
|---|---|
| S-1 marker only for 125–127 | fixed: marker absent and non-zero → runtime-error |
| S-2 checkout removed under a live container | fixed: `left` flag, rm-failure test |
| S-3 parser forms (wildcard, bracketed IPv6, case) | fixed: normalisation; wildcard rule (requester) |
| S-4 live check narrowed | fixed: one paid haiku call (requester) |
| S-5 step 3 too large, no container evidence before step 4 | fixed: 3a/3b/3c, live smoke after 3b |
| M-1 tunnel close | fixed: both ends, WaitGroup |
| M-2 empty re-pin keeps splice | fixed: changed re-pin → runtime-error |
| M-3 unbounded events | fixed: first of each kind, then counts |
| M-4 dial target | fixed: normalised target name, tested |
| M-5 markers accumulate | fixed: always read and removed |
| M-6 home creation, stale homes | fixed: created in gateCheckout; launch wipe of gates/* |
| M-7 statuses, podman stderr | fixed: statuses stated; interleaving stated |
| M-8 design-of-record lines missed | fixed: added to Docs |
| M-9 children.json-dependent tests | fixed: listed |
| M-10 S1g vacuous | fixed: positive control |
| M-11 NO_PROXY localhost | fixed: extended (requester) |
| M-12 removal under wtSem | fixed: only prune under the lock |

## Implementation plan

Each step: implement, `gofmt`, `go vet`, `go test -race ./...` (runtime package at minimum), commit, then spawn a critique gate (`phronithm:critic`, sonnet, critique-code) on the step diff and continue.

1. **Bare 429 → session-limit.** `classify.go`; a test case in `classify_test.go` (`limitResult` with no assistant error → `classSessionLimit`, zero `resetsAt`); a stub claude mode `bare429`; an agent-level test that the wait uses `SessionLimitFallback`. Doc: the Runtime section's classification. *Accept:* the new tests pass and the 529 case is still transient.
2. **Egress proxy and call splice.** `egress.go` and `egress_test.go`:
   - allow cases: name, mixed-case target, trailing-dot target and entry, IP literal, IPv6 literal with a bracketed entry, `*.` entry for one and two extra labels; the dialled address is the normalised target name plus `:443`;
   - refuse cases: other port, unlisted name, the apex of a `*.` entry, a suffix-lookalike (`evilgithub.com` for `*.github.com`), an IP target against a name or wildcard entry, plain GET, CONNECT without a port, dial failure 502;
   - the tunnel carries bytes both ways via an injected dialer to a local echo listener;
   - `close()` closes both ends and returns after the copy goroutines end;
   - re-pin: identical is a no-op, different is `runtime-error`.

   `PinHosts` starts the proxy, `Run` closes it, and `containerArgv` splices it. Tests: call argv with and without hosts (`container_test.go`); bind failure → `runtime-error`. Docs: Network. *Accept:* S1a–c.
3. **Gates in containers**, in three commits, each with its own critique gate:
   - **3a** generalise `containerSpec` (kind call | gate | smoke) and the gate argv builder, with argv tests; no behaviour change for calls.
   - **3b** `gateProcess` through podman, the home, the marker, removal and the left flag; then a **live smoke**: one `TestLiveGate` (green gate, output in the log, files owned by the user, checkout removed) before going on.
   - **3c** delete the marker reaping and `children.json`; the leftover-gates wipe; the rewritten tests.

   Tests in `gates_test.go`, under the stub podman:
   - the gate argv for setup with and without hosts, and for gates;
   - home shared (setup writes `$HOME/x` and the gate reads it);
   - a read-only module-cache tree is removed;
   - a timeout removes the container (the stub's `rm` is recorded);
   - a stop removes it;
   - reaping covers a leftover gate container (the label in the stub);
   - 125 with no marker → `runtime-error`, while 125 with the marker is red;
   - the hostile `.git` test (S1g): the gate configures `filter.x.smudge/clean` and `core.hooksPath` after committing a gitlink and a `.gitattributes`; the canary files are absent after `RunGates` returns. A positive control in the same test first shows that a host `git status` against an identically prepared checkout writes the canary, so the guard is shown able to fail;
   - a marker-absent gate (`STUB_PODMAN_RUN_EXIT=125`, and a home made unwritable for exit 2) → `runtime-error`, while a command exiting 125 after the marker is red;
   - the rm-failure test (S-2 above).

   The existing gate tests are kept where their semantics stand; the marker and orphan-process tests become container-label tests. The stub `podman` may need nothing; if it does, extend it in `testdata`. Docs: Declared gates and Teardown. *Accept:* S1d–g, S2.
4. **Live checks.**
   - `TestLiveEgress`: a stub claude mode `sh:<script>`, or a STUB_SH env, runs `curl -fsS https://registry.npmjs.org/` (hosts `registry.npmjs.org`), which succeeds; `https://example.com` and `http://registry.npmjs.org` are refused and recorded in the probe output.
   - `TestLiveGatesSetupHosts`: `setup` runs `curl -fsSL -o "$HOME/dep.tgz" https://registry.npmjs.org/<small pkg>/-/<pkg>-<ver>.tgz` and unpacks it into `$HOME`. The gate (`--network none`) reads the unpacked `package.json` and is green. A second gate tries `curl https://registry.npmjs.org/` and is red.
   - `TestLiveGateTimeout`: a sleeping gate is removed, and no container is left.
   - `TestLivePaidEgress`, gated on `LSF_LIVE_PAID=1` (and the live gates): `claudeBin` = `~/.local/share/claude/versions/2.1.294`, the real credentials file, `hosts: [registry.npmjs.org]`, model haiku, effort low, `maxCostUSD` 0.50. The prompt has Bash run `curl -sS -o /dev/null -w '%{http_code}' https://registry.npmjs.org/` and the same for `https://example.com/`, and return both codes in a schema. The assertions come from the proxy's decision hook (allowed `registry.npmjs.org:443`, refused `example.com:443`), not from the model's return. One run, recorded in the report with its cost.
   - The call with `hosts` unset is already covered by the existing live tests (no splice); one assertion is added that its argv has no proxy variables.

   Run with `LSF_LIVE_REQUIRE=1` and `LSF_TEST_IMAGE` (an image built from `container/Containerfile`). Docs: evidence lines. *Accept:* S1j.
5. **Docs completion and the CLAUDE.md clause;** a full quiet-machine `go test -race ./...` run; the implementation summary. *Accept:* S1h, S1k.

Ordering: 1 is independent; 2 must precede 3 (setup's splice) and 4; 3 must precede 4. No parallel steps, since every step writes the same package.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/s2-egress-gates-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit s2-egress-gates of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim: see "Unit entry (verbatim)" above. Phase 4 re-verifies the external gates against these pins before implementing (Go 1.27.1 per `lsf-runtime/go.mod`; Claude Code 2.1.294 via `~/.local/share/claude/versions/2.1.294 --version` with `DISABLE_AUTOUPDATER=1`; podman 6.1.3 rootless with pasta 2026_10_02.cba3570), recording the outcome in the implementation-summary.
Standing obligations naming this unit: Headless pin; Guarded host git in clones; Design of record; Register dispositions (none assigned).
Deviations already known: mount path decided as host path against the description's premise (the checkout's config does hold host paths); egress proxy starts at `PinHosts` (before any agent call) rather than literally at `Run` start; `NO_PROXY` extended with `localhost,::1` (requester); `*.` wildcard rule defined here (requester); `CLAUDE.md::lsf-runtime` clause outside Scope.
After Phase 5, produce the completion-report (contents per the phronithm:feature skill's Produces) at docs/plans/s2-egress-gates-completion.md, then present this continuation prompt to the requester:
Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
