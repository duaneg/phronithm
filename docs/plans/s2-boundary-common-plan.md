# s2-boundary-common: plan (phronithm:feature, work-unit mode, compressed Phases 1–3)

## Context

The `s2-sandbox` unit was stopped on 2026-10-09 and split. This unit is the half that either boundary needs (B2, or a container per call; `s2-boundary-decision` chooses). It covers:
- the credential proxy;
- the 401 stop;
- `hosts` in the gate block, with its `planSha` pin;
- a plain-clone gate checkout;
- git config isolation in clones.

No call is confined here. `s2-sandbox` wires calls to the proxy and `hosts` later. Base commit: `e19f366`, branch `lsf-workflow`.

## Unit entry (verbatim, docs/plans/lsf-improvements-breakdown.md @ e19f366)

### s2-boundary-common
- **Type**: feature
- **Risk**: high — the proxy holds the live credential, and a proxy fault fails every call
- **Clarity**: specified — the 401 decision is taken (requester, 2026-10-09)
- **Iteration**: write-once
- **Effort**: medium
- **Scope**: `lsf-runtime/runtime`, `lsf-runtime/orchestrator`, `lsf-runtime/plan/gates.go`, `docs/plan-graph.md::gate-block`, `skills/breakdown/SKILL.md::phase-1`, `docs/design/lsf-runtime.md::runtime`
- **Publishes**: the credential proxy, started, held and stopped with a launch on a listen address the launch gives; the gate block's `hosts` and its launch pin; the plain-clone gate checkout; git config isolation in clones
- **Depends on**: s2-clones (logical, the clones and the gate checkout; serialises the runtime and orchestrator files); s2-permissions-design (design, Credential proxy, Network, and Declared gates' gate block change)
- **External gates**: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`); `git` (environment, 2.38 or later; 2.54.0 probed)
- **Done condition**: The credential proxy, a Go port of the `s2-permissions` harness's `inject-proxy.py`, starts, holds and stops with the launch on the listen address the launch gives. It refuses a request whose bearer is not the launch's placeholder, an absent or empty bearer included; substitutes the live token; re-reads the credentials file when it changes; and streams unbuffered. A call failing 401 on an expired token with no refresh stops the run and is not journaled, so a relaunch resumes it (requester decision, 2026-10-09). The runtime's and the orchestrator's own git in a clone run with `GIT_CONFIG_GLOBAL=/dev/null` and `GIT_CONFIG_NOSYSTEM=1`, so `.gitattributes` selects only drivers in the clone's local config; a test shows a filter defined globally and selected from a clone's `.gitattributes` does not run in the runtime's `status` (requester decision, 2026-10-09). The declared-gate checkout is a plain clone, not a worktree (requester decision, 2026-10-09). The `lsf-gates` parser takes `hosts`, alone or beside gate commands; a block holding `hosts` alone leaves gate handling as for a plan with no block; a loopback entry fails the load. `hosts` and the gate block are pinned from the plan at the launch's `args.planSha` before any agent call, and every load, the first included, compares against the pin and stops on `declared-gates-changed` (requester decision, 2026-10-09). `docs/plan-graph.md`'s Gate block and `phronithm:breakdown`'s Phase 1 gate paragraph name `hosts`, the paragraph asking for the hosts the project's builds and tests fetch from. Tests cover the proxy's refusal, substitution, re-read and streaming, the 401 stop and its resume, and the pin's first-load comparison. The tests pass.
- **Description**: The half of the former `s2-sandbox` that either boundary needs (`s2-sandbox` completion report, Discoveries; requester decision, 2026-10-09). No call is confined by it: calls use the proxy and `hosts` once `s2-sandbox` launches them inside the boundary. The pin closes the gap where the decision re-plan could widen its own network (D27's first-load baseline); it touches `loop.go`, `gates.go`, the `Runtime` interface and `orchtest`. **Flag:** four surfaces exceed the surface bound; outside `lsf-runtime/runtime` and the orchestrator's pin, the parser change is one field and the documents a few lines each, so the excess is not context.

Standing obligations: Register dispositions (the unit disposes of no row; P-C5/P-C6 stay with `s2-sandbox`). Design of record: amend `lsf-runtime.md::runtime` in the same commits that change the behaviour it describes.

## Success criteria (target rigour)

| # | Criterion | Target |
|---|---|---|
| S1 | The done condition, verbatim, clause by clause: proxy (S1a), 401 stop (S1b), git isolation (S1c), plain-clone gate checkout (S1d), `hosts` parser (S1e), the pin (S1f), docs (S1g), tests pass (S1h) | proven; S1g evidenced (prose) |
| S2 | `promptparity` and `graphparity` still pass: an undeclared plan's prompts are unchanged | proven |
| S3 | A 401 stream recorded from the pinned 2.1.294 against a local 401 server classes as the auth stop | proven (a recorded fixture under test) |

## Decisions taken at scoping

- **401 scope** (requester, 2026-10-09, this session). Any call classed `permanent` with `api_error_status` 401 or assistant error `authentication_failed` stops the run, whatever the cause. The stop reason is the new `auth-failed`. Nothing is journaled (a null is not), so a relaunch re-runs the call.
- **Proxy is opt-in by config.** With no `proxy` block, no proxy starts. That keeps today's behaviour and the tests until `s2-sandbox` wires calls to it.

## Design

### 1. Gate block `hosts` (`plan/gates.go`)
- **The field.** `Declaration` gains `Hosts []string \`json:"hosts,omitempty"\``. With `omitempty`, the identity of an existing block is unchanged, so journaled `gates:<tree>` inputs still replay.
- **What a block may hold.** A block may hold `hosts`, `gates`, or both. A block with neither is a problem. `gates`, when present, must be non-empty as today. `setup` without `gates` is a problem: nothing would run it until `s2-sandbox`.
- **A valid `hosts` entry** is a non-empty string with no whitespace, scheme, `/`, `@` or port. A `*.` wildcard prefix is allowed. Entries are compared lower-cased, and a duplicate is a problem.
- **Loopback is refused.** `localhost`, `*.localhost` and anything under `.localhost` are problems. So is any IP literal (bracketed or not) that is loopback or unspecified (`127.0.0.0/8`, `::1`, `0.0.0.0`, `::`), checked with `net.ParseIP`.
- **`Gated()`.** New method: `d != nil && len(d.Gates) > 0`. A hosts-only block answers false.
- **`validate/`** (lsf-validate) goes through `ParseDeclaration` already. It gets test cases only.

### 2. Orchestrator: hosts-only, the pin, and the hosts hand-off
- **A hosts-only block is handled as no block.** Every `decl != nil` / `o.decl != nil` test that selects gate behaviour becomes `.Gated()`:
  - `prompts.go`: `fixTail`, `implementPrompt`;
  - `unit.go:479,546,579`;
  - `wave.go:119` (`Declared`);
  - `gates.go:73`.

  `declaredGatesFrozen` keeps `decl != nil`: a hosts-only block is still frozen, since `hosts` is the network allowance.
- **The pin.** At the very start of `Run`, before carried-row scribing, the decision log and every agent call, a journaled code step `gates-pin` runs. It takes input `{sha: args.planSha, planPath}` and reads the plan blob at `args.planSha`, the way `load` reads one (`g.blob`).
  - It parses the declaration. Any problem stops the run as a load-stage structural signal, through `l.stopped`.
  - Otherwise it sets `l.decl`, and `l.declSet` goes.
  - It calls `rt.PinHosts(decl.Hosts)`, a new `Runtime` interface method. `lsfrt.Runtime` stores the hosts, and `Hosts()` reads them for `s2-sandbox`. `orchtest` records the call.
- **Every load compares.** `loadPlan` always compares against the pin; the `!l.declSet` baseline branch is removed. A decision re-plan that edits the block now stops at the first load with `declared-gates-changed`. A requester changes `hosts` by committing the plan and relaunching at the new SHA.
- **Decision re-plan prompt.** `decisionReplanPrompt` gains the `declaredGatesFrozen(decl)` clause when a block exists. Without it the re-plan would edit the block and stop the run needlessly. An undeclared plan's prompt is unchanged.
- **Docs.** `doc.go`: D27 is amended (no first-load baseline; hosts-only). D31 is new: the pin, `PinHosts`, and the decision re-plan clause.
- **Without a repository** (stub tests), the pin step uses `gitFor` as `load` does. Whatever `orchtest` does for `load`, it does here.

### 3. Git config isolation in clones
- **One environment, two packages.** The shared environment builder `internal/gitenv`, `Env(clone bool) []string`, does today's scrub plus `GIT_TERMINAL_PROMPT=0`. With `clone` set it adds `GIT_CONFIG_GLOBAL=/dev/null` and `GIT_CONFIG_NOSYSTEM=1`. The orchestrator keeps its own extra `GIT_LITERAL_PATHSPECS`/`LC_ALL` on top. Both packages' `scrubbedEnv` duplicates move to `gitenv`.
- **Runtime.** A second helper family is added: `cloneStdout`, `cloneOut`, `cloneRun`, `cloneCmd`. Every git command whose `dir` is a clone uses it:
  - `clone.go`: the `clone` itself (cwd the state dir) and every command on `tmp`/`p.dir`/`dir`;
  - `uncommitted.go`;
  - `worktree.go` removal reads when the target is a clone;
  - the gate checkout.

  Commands in the main repo and in code-step worktrees keep host config. Those need the user's global identity, and a worktree shares the main repo's config.
- **Signing in clones.** `configCopied` gains `gpg.program`, `gpg.openpgp.program`, `gpg.x509.program` and `gpg.ssh.program`. A clone that ignores global config still signs as the main checkout does.
- **Orchestrator.** `execGit` gains `clones *dirSet`, a mutex-guarded set. `merge.go` adds each merge clone dir that `rt.MergeClone` returns. `execGit.run` isolates when `dir` is in the set. A zero `execGit{}` (tests) isolates nothing.

### 4. Plain-clone gate checkout (`runtime/gates.go`)
- **Making it.** `gateCheckout` runs `git init -q <dir>.partial`, then `git fetch --no-tags -q <repo> <commit>`, then `git -c advice.detachedHead=false checkout -q --detach <commit>`, then renames the directory into place. All of these are clone-isolated. Fetching by SHA works for a dangling commit over the local transport (probed this session on git 2.54.0). The orchestrator already puts merge candidates under `refs/lsf/.../xfer`, so the gate never depends on that behaviour alone.
- **Removing it.** `os.RemoveAll`, plus a one-time `git worktree prune` in the main repo, so registrations a pre-unit launch left are cleared.

### 5. Credential proxy (`runtime/proxy.go`)
- **Config.** `proxy: {listen, credentialsFile, upstream?}`.
  - `listen` is required, e.g. `127.0.0.1:0`; the launch gives the address.
  - `credentialsFile` is required, with no default.
  - `upstream` defaults to `https://api.anthropic.com` and exists for tests.
- **Lifecycle.** `Run` starts the proxy after the lock and before the body, and closes it after the body. A bind failure or a missing token is a setup error. `rt.Proxy()` returns `{URL, Placeholder}`. The placeholder is 32 random bytes in hex, one per launch.
- **The handler.**
  - **Refusal.** It refuses with 403 unless `Authorization` is exactly `Bearer <placeholder>`, compared in constant time. An absent or empty bearer is refused. The proxy logs the refusal without the header's value.
  - **Forwarding.** Otherwise it goes through `httputil.ReverseProxy`:
    - `Rewrite` sets the upstream URL and replaces `Authorization` with the live token;
    - `x-api-key` is dropped;
    - `oauth-2025-04-20` is appended to `anthropic-beta` if missing;
    - `FlushInterval: -1` makes streaming unbuffered;
    - the ErrorHandler returns 502 and logs the error.
- **The token.** Per request, the proxy `stat`s the file. On a change of mtime, size or inode it re-reads `claudeAiOauth.accessToken` (or top-level `accessToken`, as the Python does). A failed re-read keeps the previous token and logs. The token is never logged.

### 6. The 401 stop (`runtime/classify.go`, `agent.go`)
- **Classification.** `classify` marks a `permanent` verdict `auth` when the result's `api_error_status` is 401 or the assistant errors include `authentication_failed`.
- **The stop.** `Call`'s `classPermanent` branch, when the verdict is `auth`, returns `rt.setStop(StopAuthFailed, label+": …")`. The new `StopAuthFailed = "auth-failed"` is set before `recordCall`/`checkNull`, so nothing is journaled. A kept clone stays as found, and a discarded one goes by the existing defer.
- **The stream fixture.** A real stream comes from the pinned binary, at no cost: `DISABLE_AUTOUPDATER=1 ~/.local/share/claude/versions/2.1.294 -p …` with `ANTHROPIC_BASE_URL` set to a local `httptest`-style server that answers every request 401 with an Anthropic error body, and `ANTHROPIC_AUTH_TOKEN` set to a placeholder. It is trimmed per `testdata/streams/README.md` and saved as `auth-401.jsonl`, with a README row naming 2.1.294.

## Steps (each: implement, test, gofmt/vet, commit, `phronithm:critic` gate on the commit at sonnet)

1. **Parser.** `hosts` plus `Gated()` in `plan/gates.go`, with tests (alone, beside gates, loopback forms, wildcard, duplicate, neither, setup-only). `validate` gets test cases. Docs: `plan-graph.md` Gate block; breakdown Phase 1 gate paragraph asking for the hosts builds and tests fetch from.
   - Accept: `go test ./plan ./validate`; `graphparity` passes.
2. **Orchestrator.**
   - The `Gated()` substitutions.
   - The `gates-pin` step, `PinHosts` on the interface, `lsfrt.Runtime` and `orchtest`.
   - Every load comparing against the pin.
   - The decision re-plan clause.
   - D27 and D31.

   Tests:
   - the pin runs before any `Call` (orchtest order);
   - a decision re-plan that changes the block stops at the first load with `declared-gates-changed`;
   - an unchanged block passes;
   - a hosts-only plan runs as undeclared (no `RunGates`, undeclared prompts);
   - a pin problem (loopback) stops before any call.

   Accept: `go test -race ./orchestrator/...`; `promptparity` passes.
3. **Git isolation.** `internal/gitenv`, the runtime clone helpers, `configCopied` additions, and the orchestrator merge-clone set. Tests:
   - a global filter (`GIT_CONFIG_GLOBAL` pointed at a file defining `filter.x.clean` that writes a marker), selected by a committed `.gitattributes` in the clone: the runtime's uncommitted `status` leaves no marker;
   - the same through the orchestrator's merge-clone git;
   - identity and signing keys still reach the clone.
4. **Gate checkout.** Plain clone in `gates.go`. Tests:
   - the checkout's `.git` is a directory, and `git worktree list` in the main repo shows no gate entry during and after a run;
   - a dangling candidate commit, reachable only from `refs/lsf/...`, gates;
   - existing `gates_test.go` passes.
5. **Proxy.** `proxy.go`, the config, and `Run` wiring. Tests against an `httptest.NewTLSServer` upstream:
   - refusal of absent, empty and wrong bearers, with no upstream request made;
   - substitution, `x-api-key` dropped, beta header added once;
   - re-read after the file is replaced (rename) and after an in-place rewrite;
   - streaming: the upstream writes a chunk, flushes and blocks, and the client reads the chunk before the upstream is released;
   - start on the given address and stop at launch end (the port refuses after `Run` returns);
   - setup error on an absent token;
   - the token never appears in `log.txt` or `telemetry.jsonl`.
6. **401 stop.** Record `auth-401.jsonl` from 2.1.294, the stub `fixture:auth-401` mode, and the classification and stop. Tests:
   - the recorded stream gives `auth-failed`, no journal record, summary `stop_reason` `auth-failed`;
   - a relaunch with the stub at `ok` runs the call and returns its value;
   - other permanent errors (`model_not_found` stream) stay null.
7. **Design of record.** `lsf-runtime.md::runtime`:
   - Configuration: `proxy`;
   - Interface: `Proxy`, `Hosts`/`PinHosts`, RunGates' plain clone;
   - A call: the 401 row;
   - Supervision: git isolation;
   - Known limits: global config clones lack (the R29 note, `core.excludesFile`).

   Also the `CLAUDE.md` `lsf-runtime/` line (the proxy).

## Phase 4 entry re-probe
- `go version`, expecting 1.27.1.
- `git --version`, expecting ≥ 2.38 (2.54.0 now).
- Drift is surfaced to you before implementing.

## Phase 5
- `phronithm:review` over the full diff, all applicable lenses, with the static-analysis pre-scan (`go vet`, `staticcheck` if present).
- A `phronithm:critic` code gate on the full diff.
- `cd lsf-runtime && go test -race ./...`, with `PROMPTPARITY_REQUIRE_NODE=1 GRAPHPARITY_REQUIRE_NODE=1`.
- Then `docs/plans/s2-boundary-common-implementation.md` and `docs/plans/s2-boundary-common-completion.md`, committed, and the re-plan instruction.

## Critique round 1 (phronithm:critic, sonnet): passed, no Critical. Dispositions; these amend the design above

1. **The pin's effects come from the step's return value**, not from inside `fn`. A replay and `orchtest` skip `fn`, so `Run` derives `l.decl` from the returned blob or declaration and calls `PinHosts` from it too. A null or fault from the pin step is a load-stage stop, as in `loadPlan`.
2. **Scaffolding is scheduled.** `orchtest.Defaults` gets a `gates-pin` handler. `PinHosts` goes on every Runtime fake (`scriptedSteps`, `gitRT`, `journalRT`). `tools/promptparity`'s `codeStep` list gains `gates-pin`; that is outside the Scope line (see 16).
3. **The isolation test cannot pass vacuously.** It makes a tracked file stat-dirty before `status`. A control run without isolation proves the marker is written. The orchestrator's merge-clone path gets the same test.
4. **Global excludes are kept.** `copyRepoSetup` appends the content of the main repo's effective `core.excludesFile`, when set, to the clone's `info/exclude`. A test shows a globally excluded file is not reported uncommitted.
5. **Hooks inherit the isolated environment.** Accepted and documented as a known limit: a hook in a clone, and what it spawns, sees no global or system git config. The identity, signing and line-ending keys are local in the clone, so the runtime's own commits are unaffected.
6. **The gate clone.** It fetches the commit's full history, so `git describe` against history still works, without tags. That cost is accepted and recorded as a limit; the docs already say a gate depends on the checkout's files alone. The gate clone also gets the `configCopied` line-ending keys before checkout. No hooks run in it, since no commit is made there.
7. **The pin's place.** It runs after the carried-row registration loop and before `scribeOrStop`, so a pin stop keeps the unscribed rows.
8. **`listen` is not restricted to loopback.** The container option needs an address the container can reach, and the done condition says "the listen address the launch gives". A non-loopback `listen` is logged at start. Recorded as a limit for `s2-boundary-decision`.
9. **Refusals are distinguishable.** A refusal is 403 with the body `lsf-proxy: placeholder bearer required`, and every refusal is logged once per launch (count plus first). `s2-sandbox` decides how a call surfaces it.
10. **Test upstream.** `upstream` accepts `http://` for tests. Production keeps the https default.
11. **Proxy hardening adopted.**
    - More than one `Authorization` header is refused.
    - `ReadHeaderTimeout` is set.
    - The server's and `ReverseProxy`'s `ErrorLog` go to `rt.Log`.
    - The token cache sits behind a mutex.
12. **`hosts` validation.**
    - Brackets are stripped before `net.ParseIP`.
    - An entry that parses as IPv6 is exempt from the port rule.
    - `*.` and single-label wildcards (`*.com`) are refused.
    - Accepted, documented limits: `127.1`-style numeric forms, and names that resolve to loopback.
13. **Gate fetch.**
    - The commit must be a full hex object ID.
    - The fetch is `git fetch --no-tags -q --end-of-options <repo> <sha>`, then `rev-parse --verify <sha>^{commit}`.
    - `wipeScratch`'s `.partial` glob adds `gates`.
    - The legacy `git worktree prune` runs at `RunGates`' removal, as today.
14. **Explicit helper sites are kept.** `execGit` becomes a struct whose clone set is behind a pointer, built in `newOrch`; `execGit{}` in tests stays unisolated. `recheckUncommitted` picks the helper by `res.Clone`. Recorded as a residual: code-step worktrees and the main checkout's `merge --ff-only` still read host config.
15. **Design of record per step.** Each step amends its fragment of `lsf-runtime.md::runtime`. Step 7 keeps only the remaining prose and `CLAUDE.md`.
16. **Footprint beyond the Scope line**, for your acceptance:
    - `lsf-runtime/internal/gitenv` (new);
    - `lsf-runtime/testdata/claude` and `testdata/streams/` (the 401 fixture);
    - `lsf-runtime/tools/promptparity` (one label);
    - `lsf-runtime/validate` tests;
    - `CLAUDE.md` (one line).

    These are recorded as deviations in the completion report.

## Discoveries to carry to the completion report
- `auth-failed` is a new stop reason. `runtime-cli-drive` and `phronithm:lsf-drive` should present it as "refresh the login, relaunch".
- `PinHosts`/`Hosts()` and `Proxy()` are the hand-off points for `s2-sandbox`.
- A decision re-plan can no longer change the gate block within a launch.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above, complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/s2-boundary-common-implementation.md
It must include:
- Working branch
- Reference to this plan file (docs/plans/s2-boundary-common-plan.md)
- Files modified
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions
- Success criteria verification: each criterion with pass/fail, target and achieved rigour with evidence, and an upgrade path when achieved < target. Ladder: `proven ▸ evidenced ▸ conjectured`.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit s2-boundary-common of docs/plans/lsf-improvements-breakdown.md. The unit entry is reproduced verbatim above (Phase 4 re-verifies its external gates against those pins and records the outcome in the implementation-summary). After Phase 5, produce the completion-report at docs/plans/s2-boundary-common-completion.md, then tell the requester: run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
