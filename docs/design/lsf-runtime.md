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

- `runtime` does not depend on `plan`. It reports driver events in `CallResult`, and the orchestrator maps them to signal kinds.
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

## Permissions

Units `s2-permissions-design` and `s2-boundary-decision`. Evidence: [s2-permissions results](../investigations/s2-permissions-results.md), about 85 calls, and [container-probe results](../investigations/container-probe-results.md), 11 paid calls and scripted cells, both on Claude Code 2.1.294. Rigour `evidenced`, n = 1–2 per cell. Linux only.

**Status:** decided, 2026-10-10. It supersedes B2, decided 2026-10-08. Requester verdicts:
- **A rootless podman container per agent call and per gate command** (see Boundary).
- **Bash can reach the credential proxy.** This is accepted as a documented residual, with no mitigation (see Known limits). The cheaper closure, a proxy socket Bash cannot reach, was declined.
- **The image is named in the runtime config by digest.** The user builds it. `claude` and the plugin are bind-mounted from the host (see Image).
- **`hosts` gets no address rule beyond `s2-boundary-common`'s** loopback and unspecified refusal. A private or link-local entry is the requester's choice (see Network).

Earlier verdicts that stand:
- **Reviewers** keep no writes (2026-10-08).
- **A kept clone** stays until its unit's cleanup (2026-10-08).
- **Network:** agent calls' Bash, writers and reviewers alike, and gates' `setup` reach only the gate block's `hosts`. Gate commands have none (2026-10-08).
- **401 with no refresh** stops the run with `auth-failed` (2026-10-09).
- **The gate checkout** is a plain clone (2026-10-09).
- **`hosts` and the gate block** are pinned from the plan at the launch's `planSha` before any agent call (2026-10-09).

### Options

| Option | Result on the spike fixture |
|---|---|
| Deny list, `Bash` granted (spike runs 2–7) | Implement commits. A `python3` write changed the main checkout's settings. In 2 of 2 runs, branches were deleted, egress reached a listener and canaries were read. |
| Prefix allow rules, prompts rewritten to single commands | The probes held only because the agent stopped at the first denial. One `node -e`, allowed by the `Bash(node:*)` the gates need, rewrote the settings. |
| `--permission-mode auto` | Settings edits blocked. Branches were deleted in 2 of 2 runs, one exfiltration got through, and canaries were read. The classifier is probabilistic. |
| `ic`, broad permissions inside | Not runnable headless as it stands, and it mounts the host credentials read-write. |
| B2: native Bash sandbox, `--restricted`, host credential proxy | Every probe held: settings, branch, egress, canary, token and docker escape. Bash could not reach the proxy (`B2n`). |
| **Container**, as decided below | Implement commits, and the stream reaches the host intact. The settings and branch probes held: writes inside the container failed on the read-only root, and the host paths were not mounted. Egress, canary, token and engine-socket probes held; scripted cells show the enforcement. A declared registry was reached and an undeclared host refused. A hostile gate was confined. **Bash reaches the credential proxy** (F1). |

### Weighing

The goal (requester, 2026-10-09) is "whatever works, is simple, and robust".

| | B2 | Container |
|---|---|---|
| Moving parts the runtime owns (indicative, counted at the same grain) | Call: settings file with sandbox, `denyRead` and network blocks; `--restricted` with `--add-dir`; deny rules with `<plugin>` substitution; env construction with scrub and short `TMPDIR`; the `/run/containerd` workaround; the `/tmp` placement refusal. Gates: `srt` with its prerequisites. Shared: inject proxy. | Call: `podman run` argv (mounts, user namespace, read-only `.git` binds); env construction; `pasta` splice; teardown by name and label. Gates: the same argv, narrowed. Shared: inject proxy, egress proxy (new Go work), image and launch checks, reference `Containerfile`. |
| Who confines | The kernel confines Bash. Claude Code confines the file tools (`--restricted`). | The kernel confines the whole process tree. |
| Tied to Claude Code | Its sandbox: the `/run/containerd` bug, `/tmp` writability, mount-point files and `--restricted` behaviour are re-checked at each pin move | Only its CLI flags (`--tools`, `--dangerously-skip-permissions`) and that `claude` runs in the image |
| Credential use | Only `claude` itself reaches the proxy | Bash also reaches it |
| Known limits left | File-tool confinement is Claude Code's, not the kernel's; `/tmp` and `/var/tmp` shared between calls; mount-point files in each clone; allowed hosts an exfiltration channel; only proxy-aware tools get network; macOS unprobed (Known limits at d26f71c) | See Known limits |
| Cost | — | About 0.4–0.5 s per call to start the container. A user-built image. Podman as a prerequisite. |

The container was chosen. None of its parts depends on Claude Code's sandbox internals, and confinement is the kernel's throughout. Its one gap, credential use from Bash, is accepted (Known limits).

### Boundary

- **One container per agent call, and one per gate command.** The confinement comes from rootless podman's namespaces and mounts: a read-only root, nothing from the host but the mounts listed under A call's launch, no capabilities, and `no-new-privileges`.
- **The network is closed** except for loopback ports that `pasta` splices to host loopback: the credential proxy's, and the egress proxy's when `hosts` is set. The container has no default route, no LAN and no DNS.
- **The credential** never enters a container (see Credential proxy).
- **Claude Code's own sandbox is off.** The call runs with `--dangerously-skip-permissions`; `--tools` still sets each role's tool set.

### Placement

- **No agent call runs in the main checkout or a worktree.** A worktree's `.git` is the main repo's, so its refs and config are writable through the worktree. Every call runs in its own clone under the state dir, mounted at its host path (see A call's launch). The main checkout, the state dir, `$HOME` and other clones are not mounted.
- **Plain `--no-local` clones.** A clone made without `--no-local` hardlinks its objects to the main repo's, so a write through the clone's mount could corrupt the main store. A `--shared` clone's alternates point outside the mounts.
- **Commits come back by fetch.** After a call, the runtime fetches the clone's branch into the main repo as the call's branch (`lsf/<plan8>/<key>`), without tags. Every check the orchestrator makes on a worktree branch applies unchanged. A writer's commits reach the integration branch only through an orchestrator step that checks the branch and fast-forwards to it. The fast-forward updates the main checkout, so it needs that checkout clean and at the expected tip. A requester edit there stops the run, as a requester edit during a repo call does today.
- **Which clone each call gets.**

  | Call | Today | Clone of | Kept |
  |---|---|---|---|
  | `plan`, `fix-plan`, `implement`, `fix-code` | worktree | the repo, on the call's branch, created where the worktree's is today | fetched |
  | `critique-plan`, `critique-design`, `critique-code`, `lens-review` | worktree, or main checkout for `critique-plan` | the repo, on the branch the prompt names | discarded |
  | `direct-edits[:w<n>]`, `decision-replan`, `replan:w<n>`, `termination` | main checkout | the integration tip | fetched, then fast-forwarded |
  | `review-gate:w<n>`, `obligations` | main checkout | the integration tip | discarded |
  | `merge-conflict:w<n>:<unit>` | main checkout, mid-merge | none: the wave's merge clone (see Merge conflicts) | the merge clone |

  Every branch a prompt names is a local branch in the clone, at the main repo's tip for it.

### Roles

The orchestrator issues each label below with D10's `#n` suffix on a repeat. The patterns are prefix-anchored, and each one either ends unanchored or ends `(#|$)`. First match wins.

```json
"defaultRole": "writer",
"roleRules": [
  {"agentType": "phronithm:critic", "role": "reviewer"},
  {"label": "^(critique-(plan|design|code)|lens-review):r[0-9]+:", "role": "reviewer"},
  {"label": "^review-gate:w[0-9]+(#|$)", "role": "reviewer"},
  {"label": "^obligations(#|$)", "role": "reviewer"}
],
"roles": {
  "writer":   {"tools": "Read,Edit,Write,Glob,Grep,Bash,Skill", "clone": "kept"},
  "reviewer": {"tools": "Read,Glob,Grep,Bash,Skill", "clone": "discarded"}
}
```

- Every label not matched above is a writer: `plan`, `fix-plan`, `implement`, `fix-code`, `direct-edits`, `decision-replan`, `replan`, `termination`, `merge-conflict`. `plan:critique-x` and `merge-conflict:w1:review-gate` are writers, because the patterns anchor at the label's start.
- A role is its tool set and its clone kind. The `allow`, `deny` and `permissionMode` fields are gone, with the `<plugin>` substitution: the mounts make the plugin and `.git`'s config read-only. A config that still carries them fails strict decoding.

### A call's launch

The runtime builds each call's argv and environment. The role supplies only its tools and its `clone` value.

```
podman run --rm -i --init --pull=never --log-driver=none --name <name> --label lsf.state=<stateDir hash>
  --userns=keep-id --cap-drop=all --security-opt no-new-privileges
  --read-only --tmpfs /tmp:rw,exec
  --network pasta:--splice-only,-T,<inject port>[,-T,<egress port>]
  -v <clone>:<clone>:rw -v <clone>/.git:<clone>/.git:rw
  -v <clone>/.git/config:<clone>/.git/config:ro -v <clone>/.git/hooks:<clone>/.git/hooks:ro -v <clone>/.git/info:<clone>/.git/info:ro
  -v <claude>:/opt/claude/claude:ro -v <plugin>:/opt/plugin:ro
  -w <clone> -e … <image>
  /opt/claude/claude -p … --plugin-dir /opt/plugin --tools <role tools> --dangerously-skip-permissions
```

- **`<name>`** is `lsf-<hash8>-<session id>`, unique to the call's attempt, so a kill can find the container (see Teardown). The label, the first 16 hex digits of the SHA-256 of the state dir's resolved path, lets a launch find containers an earlier launch left behind.
- **The clone is mounted at its host path** (`s2-sandbox`, a change from the probe's `/work`). Its local config holds absolute host paths: `core.hooksPath`, and `core.excludesFile` and `core.attributesFile` under `.git/lsf-global/`. At `/work` they would not exist, so git in the container would skip the hooks and the global excludes, and `git add -A` could commit files the host ignores. A `stateDir` or `pluginDir` containing `:` is refused, and so is a `claudeBin` whose resolved path does.
- **`--pull=never`:** a missing image is a launch failure, never a registry pull. **`--log-driver=none`:** podman's default driver on the probe host was `journald`, so every probe call's stream went to the user journal; `-i` attach does not depend on the driver.
- **`--userns=keep-id`.** Inside the container the call runs as the invoking user, so the files it writes in the clone are that user's on the host and fetch back.
- **The read-only `.git` binds** stop an agent from writing a filter driver, `core.hooksPath` or `include.path` that the runtime's own git in the clone would later run (R34). Commits still succeed. The runtime creates `hooks` and `info` in a clone that lacks them before the launch, since a missing bind source fails it.
- **`.git` is bound over itself** (`s2-sandbox`, found in its step 2a critique and confirmed live): without it an agent could rename `.git` away and put a `.git` of its own, config included, in its place; a mountpoint cannot be renamed. The rest of `.git` stays writable, so an agent could also write `.git/commondir`, which points git's config lookup at a directory it wrote. The runtime's and orchestrator's git refuses to run in a clone whose `.git` is not a directory, holds a `commondir`, or has no `HEAD` or `objects` and `refs` directories (git would not take it as a repository and would look in an enclosing one; `GIT_CEILING_DIRECTORIES` at the clone's parent stops that too), and so do the fetches from a clone, whose `upload-pack` runs there under host config; the run stops with `runtime-error`.
- **Nested repositories.** An agent can commit or stage a repository inside its clone as a gitlink, with a filter in that repository's own config. The host's `git status` or `diff` in the clone would look into it and run the filter, even under `-c diff.ignoreSubmodules=all`, which a committed `.gitmodules` `ignore = none` overrides (both reproduced, step 2b and 3 critiques). Two layers, each sufficient in the regression test:
  - the runtime's and orchestrator's git refuses to run in a clone whose index or `HEAD` holds a gitlink (`ls-files --stage` and `ls-tree`, which read no work tree); a main repo with submodules is refused at launch, so any gitlink there is an agent's;
  - every `status` and `diff` in a clone gets `--ignore-submodules=all` on the command line, with `-c diff.ignoreSubmodules=all -c submodule.recurse=false` before every command.

  A refused clone's checks fail as git failures do: the uncommitted check flags `uncommitted-unchecked`, a merge step stops. A kept clone's fetch-back refuses a branch whose tree holds a gitlink, so it never reaches the main repo or a merge clone (where the guard would refuse cleanup too): the run stops with `runtime-error` naming the path. An agent that commits a nested repository (a vendored or fixture repo added with `git add -A`) therefore ends the run; routing it back to the unit is not done.
- **The conflict call** gets the merge clone at its host path, with the same binds.
- **A reviewer's clone is mounted read-write too.** A reviewer that re-runs tests writes build output, and the clone is discarded.
- **Environment:** replaced, not inherited.
  - `HOME=/tmp/home`, a per-call home on the tmpfs. `PATH` is the image's; `SHELL=/bin/bash`.
  - `DISABLE_AUTOUPDATER=1` and `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`.
  - `ANTHROPIC_BASE_URL=http://127.0.0.1:<inject port>` and `ANTHROPIC_AUTH_TOKEN=<placeholder>`. The placeholder is given as `-e ANTHROPIC_AUTH_TOKEN`, its value in podman's environment, not its argv, which every local user can read.
  - With `hosts` set: `HTTPS_PROXY`, `HTTP_PROXY` and their lower-case forms at `http://127.0.0.1:<egress port>`. `NO_PROXY` and `no_proxy` are `127.0.0.1,localhost,::1` in every case: loopback stays in the container's own network namespace (requester, 2026-10-10).
  - The names in a config `env` passthrough list, each `-e NAME` taking its value from the runtime's environment. A name the runtime sets (`LSF_GATE_COMMAND` and `LSF_GATE_SEQ` included), a proxy variable, `ANTHROPIC_*` and `CLAUDE_CODE_OAUTH_TOKEN` are refused.
- **The clone's git identity** is set in its local config before the call, from the main repo's.

### Image

- **The runtime config names the image by digest** (`name@sha256:…`). A tag alone is refused. The user builds the image; phronithm ships a reference `Containerfile`, which the user extends with the project's toolchain. It is the probed image (Z1) less the fixture's toolchain: a digest-pinned Debian slim base with `bash`, `ca-certificates`, `curl`, `git`, `procps` and `ripgrep`.
- **`claude` comes from the host.** The config names the pinned binary (`claudeBin`, for example `~/.local/share/claude/versions/2.1.294`) and its version (`claudeVersion`, required). The binary is resolved to an absolute regular file at launch and bind-mounted read-only. The image carries no `claude`, so the pin moves without a rebuild.
- **The plugin comes from the host,** read-only at `/opt/plugin`: the whole of `pluginDir`, not narrowed. It is pinned only by how it was installed; the runtime does not verify its content.
- **Checks at launch.** After the state-dir lock and before orphan reaping, the runtime refuses to launch (a setup error) when:
  - `podmanBin` is not found, or `podman info` does not report `Host.Security.Rootless` true;
  - `podman info` names no `Host.Pasta.Executable`, or it is not a file on the host;
  - `podman image exists <image>` fails;
  - the smoke container fails. It is launched as a call is, labelled `lsf.smoke=1` too, with `--network none` and no API variables, in a throwaway repo at `<stateDir>/smoke` bound with the `.git` binds, under a 2-minute bound (removed on expiry). Its script prints `claude --version` on a line marked `lsf-smoke-claude` (podman may write warnings first), runs `bash`, `git` and `rg`, stages a file, and fails if `.git/config`, `.git/hooks` or `.git/info` can be written or `.git` renamed. The first field after the marker must equal `claudeVersion`. The checks run under `Run`'s context; the `podman` queries are bounded at 60 s.

  A dry run runs the checks too, so it needs podman and starts one smoke container.
- **A failed launch is not a model fault.** A `podman run` that exits 125, 126 or 127 before the stream starts (no line of it decoded: an image, mount, `pasta` or name fault) stops the run with `runtime-error`, quoting podman's stderr; it is not retried as `transient` and not journaled, so a relaunch re-runs the call. The same exit after the stream began is classified as any other.

### Teardown

- **A cancel** sends SIGTERM to the `podman run` process group, as the runtime cancels any call. `--init` passes the signal to the container's processes.
- **After `killGrace`,** the client gets SIGKILL and the runtime runs `podman rm -f -t 0 --ignore <name>`: SIGKILL to the client alone would leave the container running, since its processes run under `conmon`, not the client (F3). `-t 0` because SIGTERM and the grace have already passed. The removal runs whatever the call's context, under its own 60 s bound, and also after any non-zero exit, since a client killed mid-cleanup can leave its container. These are `s2-sandbox` additions.
- **A removal that fails** while `podman container exists` still finds the container (`--rm`'s own removal may have won the race), or that hits the 60 s bound, stops the run with `runtime-error` before anything reads the clone: no fetch-back, no uncommitted check, no discard and no journal record. The next launch's reaping removes the container.
- **Gate containers** (`setup` and each gate command) are named `lsf-<hash8>-gate-<launch>-<n>` and labelled the same way, and a timeout or a stop removes them the same way (Declared gates).
- **Nothing reads a clone or checkout until its container is gone.** Fetch-back, the uncommitted check and removal wait for `podman rm -f` to return.
- **Orphan reaping** at launch removes every container carrying this state dir's label (`podman rm -f -t 0`). A failed `podman ps`, or a container not removed, refuses the launch: a live container may still write a clone the launch would read. Gate containers are reaped the same way, by label; `--session-id` matching and the `lsf-gate:` marker are gone.

### Credential proxy

- **Where it listens.** On `127.0.0.1` at an ephemeral port. A container reaches it only through the `pasta` splice of that port, at `127.0.0.1` inside. The runtime refuses a `proxy.listen` whose host is not `127.0.0.1` (a wildcard and `::1` included): `pasta -T` forwards to host loopback, so another address would serve no call and would expose the proxy beyond the host.
- **What it does.** It refuses a request whose bearer is not the launch's random placeholder. Otherwise it replaces the bearer with the live session's OAuth access token from `~/.claude/.credentials.json`, adds the OAuth beta header, forwards to `api.anthropic.com`, and streams the response back unbuffered (Runtime → Interface, `rt.Proxy()`).
- **Which token.** The live session's credential, with no separate token (requester, 2026-10-08).
- **When it reads the token.** It re-reads the file when the file changes, so a refresh by an interactive session reaches running calls. A call that fails authentication (401) stops the run with `auth-failed` and is not journaled, so a relaunch after the login is refreshed re-runs it.
- **Who can reach it.** `claude` and, sharing its network namespace, every process the call's Bash starts (see Known limits).
- **The proxy stops with the launch.**

### Reviewers

- **What a reviewer can write.** Its own clone and the container's `/tmp`. It cannot write the main checkout, the integration branch or any other clone.
- **What happens to those writes.** Its clone is never fetched and is removed after the call, so nothing it writes is kept: zero *kept* writes.
- **No uncommitted check for a discarded clone.** Build output from a test a reviewer re-runs would otherwise raise `residue-at-return` on every such call.
- **Reviewers have no `Edit` or `Write`.**

### Merge conflicts

- **The merge step runs in a merge clone, not the main checkout.** The clone is `<stateDir>/merge/w<n>`, on the integration branch. The step's `m.dir` becomes that clone, and every check runs there unchanged, as runtime code outside any container:
  - `outside` and `changedOutside`;
  - the `head != pre` check, since the call stages a resolution and does not commit it;
  - `stagedOK`;
  - `write-tree` and `mergeGates`;
  - the commit with its `Lsf-Unit`/`Lsf-Step` trailers;
  - `abortOwn`.
- **The conflict call's container mounts the merge clone** at its host path. It is the only call whose clone is not its own.
- **After each verified merge commit,** the main repo fetches it and fast-forwards the integration branch, as in Placement.
- **The merge clone stays until the wave's merges finish.** So an interrupted step finds its merge in progress, and its adoption by trailers works as today.
- **`refusedOverLocal` no longer arises in the merge.** The merge clone holds no requester edits. A requester edit now blocks the fast-forward and stops the run.

### Network

- **The problem.** Each call starts from a fresh clone and an empty home. A stage whose build or tests fetch dependencies (npm, pip, Go modules, a GitHub archive) fails with no network.
- **The rule** (requester, 2026-10-08). Bash in every agent call, writers and reviewers alike, and gates' `setup`, may reach only the hosts named in the `lsf-gates` block's `hosts` list. Gate commands get no network. With no `hosts` list, nothing has network but `claude`'s route to the credential proxy.
- **Why the gate block.** It is pinned from the plan at the launch's `planSha` before any agent call. It changes only by the requester's commit and a relaunch, and a change within a launch stops the run on `declared-gates-changed`. The plan block would not do: re-plan stages are agents that edit it, so an agent could widen its own network. A plan with no gates may carry an `lsf-gates` block holding `hosts` alone.
- **How it is enforced.** When the hosts are pinned (`PinHosts`, before any agent call) and the list is not empty, the runtime starts an egress proxy on `127.0.0.1` at an ephemeral port for the rest of the launch. It is spliced into the container like the credential proxy, and the proxy variables are set. It allows `CONNECT` to port 443 of a pinned host and refuses everything else with 403, plain HTTP included; a failed dial is 502. A target matches an entry case-insensitively, ignoring a trailing dot; an IP literal (an IPv6 entry bracketed or not) matches only the same literal entry, never a wildcard. A `*.` entry matches names one or more labels below its suffix, not the suffix itself: `*.github.com` matches `api.github.com` and `a.b.github.com`, not `github.com` (requester, 2026-10-10). The normalised target name is dialled from the host. With no other route out, a request that bypasses the proxy variables fails to resolve. A failed start stops the run with `runtime-error`, and `setup` then runs with no network; a second pin with other hosts does too. The first allow and the first refusal of a launch are logged and emitted (`egress-allowed`, `egress-refused`); the counts are logged when the launch ends.
- **What the runtime refuses.** A `hosts` entry naming loopback or the unspecified address (`s2-boundary-common`). Private, link-local and internal names are allowed: the list is the requester's, so an entry such as an internal package mirror is deliberate (requester, 2026-10-10).
- **Evidence.** Bash reached the npm registry through the egress proxy and was refused an undeclared host and plain HTTP; the direct route failed to resolve a name and to connect to a literal IP (P8, Z8; `TestLiveEgress` with the stub `claude`). One real 2.1.294 call (haiku, low effort, $0.0037) under `hosts: [registry.npmjs.org]` was asked to have Bash curl the registry and `example.com`; the proxy saw and allowed `registry.npmjs.org:443` and refused `example.com:443`, and the call returned both results (`TestLivePaidEgress`, opt-in, n = 1). `setup` under `hosts` installed an npm tarball into the gate home, which a `--network none` gate then used, and a gate that reached for the registry was red (`TestLiveGatesSetupHosts`).

### Declared gates

- **The `declared-gates-changed` stop is not enough.** It shows the requester a changed command. But the commands run code the stages wrote (tests, scripts, package install hooks).
- **Each command runs in its own container** from the same image:

  ```
  podman run --rm --init --pull=never --log-driver=none --name lsf-<hash8>-gate-<launch>-<n> --label lsf.state=<stateDir hash>
    --userns=keep-id --cap-drop=all --security-opt no-new-privileges
    --read-only --tmpfs /tmp:rw,exec
    --network none | pasta:--splice-only,-T,<egress port>
    -v <checkout>:<checkout>:rw -v <gate home>:<gate home>:rw
    -w <checkout> -e HOME=<gate home> [-e HTTPS_PROXY=… -e HTTP_PROXY=… -e https_proxy=… -e http_proxy=…]
    -e NO_PROXY=127.0.0.1,localhost,::1 -e no_proxy=… -e LSF_GATE_COMMAND -e LSF_GATE_SEQ=<n> [-e <passthrough>…]
    <image> /bin/sh -c ': > "$HOME/.lsf-started-$LSF_GATE_SEQ" && eval "$LSF_GATE_COMMAND"'
  ```

  - **`setup`** gets `pasta:--splice-only,-T,<egress port>` and the proxy variables when the egress proxy runs (`hosts` set), and `--network none` otherwise.
  - **Gate commands** get `--network none`.
  - **No `-i`:** a command's stdin is `/dev/null`. Its stdout and stderr go to the gate's log, interleaved with podman's own.
  - **The command** reaches the container as `-e LSF_GATE_COMMAND` from podman's environment, so the argv is fixed.
  - **`--userns=keep-id`** keeps what a command writes owned by the invoking user, so the checkout's removal succeeds.
  - **The checkout and home are mounted at their host paths** (requester, 2026-10-10). The checkout's local config holds absolute host paths, as a call's clone's does: `core.hooksPath`, and `core.excludesFile` and `core.attributesFile` under `.git/lsf-global/`. At `/work` they would dangle, and git a gate runs itself (a test calling `git status`, a build calling `git describe`) would skip the global excludes and attributes. The cost is that gate code sees the state dir's host path.
  - **`<gate home>`** is `<stateDir>/gates/<key>.home`, made `0700` beside the checkout and removed with it. Removal first makes every directory in both trees user-writable: Go writes its module cache read-only. `setup` and the gates share it and the checkout, so a dependency `setup` installs reaches the gates whether it lands in the tree (`node_modules`) or in a cache under `HOME` (Go modules, Cargo, pip `--user`).
  - **Environment.** `PATH` is the image's; `HOME`, the proxy variables and the config `env` passthrough list as above; no Anthropic variables.
  - **The started marker.** A container that exits non-zero without having written `$HOME/.lsf-started-<n>` failed before its command started (an image, mount, `pasta` or name fault, or an unwritable home). That stops the run with `runtime-error`, quoting the log's last line, and returns no verdict, which would otherwise be replayed. A command that ran and exited non-zero is red, 137 (a kill in the container) included; a podman client killed by a signal, or a timeout, is not-run. The runtime makes the home user-accessible again after each command (a command may `chmod` it), then reads and removes the marker; only a missing marker means not started. A command can delete or forge its own marker, which can only turn its own red into a run stop or a launch failure into red.
  - **Teardown.** A timeout or a stop sends SIGTERM to the podman client's group and SIGKILL after `killGrace`; after that, or any non-zero exit, the container is removed by name (Teardown). A removal that fails stops the run with `runtime-error`, and the checkout and home are kept while the container may run; the next launch reaps it, then wipes every `gates/` entry but `logs`.
  - **No git runs in a gate checkout after its first command,** only its removal (`chmod` and unlink). A command that writes the checkout's `.git/config`, a filter, `core.hooksPath` or a gitlink is therefore harmless, and the checkout gets no read-only binds. Any host git added there later goes through the clone guards (`gitenv`).
- **Evidence.** The fixture gates passed in a container (Z3). The runtime's own gate containers ran live: the shared home, a read-only tree's removal, output through attach without `-i`, a timeout's removal, and `setup` under `hosts` (`TestLiveGate`, `TestLiveGateTimeout`, `TestLiveGatesSetupHosts`). A hostile gate under `--network none` found the env canary absent and the file canaries unreachable, could not write outside its checkout or `/tmp`, had every egress blocked and found no engine socket (Z3).
- **Gate block.** The `lsf-gates` block carries `hosts`, with or without gate commands (`s2-boundary-common`).

### Runtime changes

`s2-clones` carried the clones, fetch-back, removal, the merge clone and the `clone` role field. `s2-boundary-common` carried the credential proxy, the 401 stop, `hosts`, its parser and its pin, the plain-clone gate checkout and git isolation in clones. `s2-sandbox` carried 1, 2, 3 for calls, 4's `listen` restriction, 6, 7 and 8; `s2-egress-gates` carried 3 for gates, 4's egress proxy and 5. The runtime now does all of:

1. **Container config:** the image digest, the `claude` binary path, the `env` passthrough list, and the launch checks of Image.
2. **The call's launch** as A call's launch states. Roles reduce to `tools` and `clone`; `allow`, `deny`, `permissionMode` and the `<plugin>` substitution are dropped.
3. **Teardown and reaping** by container name and label, for calls and gate commands, replacing process-group reaping by `--session-id` and the `lsf-gate:` marker; podman launch failures stop with `runtime-error`.
4. **Proxies:** the credential proxy's `listen` host must be `127.0.0.1`. An egress proxy (`CONNECT` to port 443 of a pinned host only) starts when `hosts` are pinned, before any agent call.
5. **Gates in containers:** `setup` and each gate command, with the shared gate home, as Declared gates states.
6. **Placement refusals:** the refusal of a repo or state dir under `/tmp` or `/var/tmp` is dropped, since no container sees the host's `/tmp`. The shallow, submodule and LFS refusals stay.
7. **A reference `Containerfile`** shipped with the runtime.
8. **The fixture run under the container.** `tools/fixturerun` runs the spike fixture end to end under the decided configuration.

### Known limits

- **Bash can spend on the API with the real credential** (accepted residual, requester, 2026-10-10). Bash shares `claude`'s network namespace, so it reaches the credential proxy, which injects the token and forwards (F1 showed credentialed forwarding; no request was completed). The token itself never enters the container. What a compromised agent gains is:
  - API spend that neither `--max-budget-usd` nor the runtime's `maxCostUSD` sees, since both count only the call's stream;
  - whatever else the OAuth token is authorised for at `api.anthropic.com`.
- **Allowed hosts are an exfiltration channel.** Agent Bash and code that `setup` runs (package install hooks) can send what they read in the clone or gate checkout to any host in `hosts`. A host that accepts writes, such as GitHub, can receive the repo.
- **The egress proxy takes no credential.** Any local process can reach `127.0.0.1:<egress port>` and tunnel to a pinned host on 443, which gains it nothing it could not reach from the host directly.
- **The egress proxy dials from the host.** A `hosts` name reaches whatever it resolves to on the host: a private or link-local address, or host loopback, where the proxies and any local service listen. Only a listed name is reachable, and only on port 443; a `*.` entry extends this to every name under it, so one under a domain whose DNS others control (a shared hosting suffix) can point at a host-local address.
- **Only proxy-aware tools get network.** A tool that ignores the proxy variables gets none, even to an allowed host.
- **No cache survives a call.** `HOME` is a tmpfs, so each call fetches its dependencies again. A stage that wants a cache must put it in its clone.
- **A call cannot change its clone's `.git/config`, hooks or info.** `git config` fails with `EBUSY`. The identity is set before the call; a stage that needs another local key fails.
- **Image reproducibility.** The digest pins the image, not how it was built. A toolchain from the distribution's current packages differs between builds.
- **A development plugin root is wholly readable.** Under `--plugin-dir` against a working copy, `/opt/plugin` exposes that repo's `.git` and untracked files for reading. An installed plugin holds only the plugin.
- **No resource limits.** A container has no `--memory` or `--pids-limit`, and its `/tmp` (with `HOME`) is a RAM-backed tmpfs: a call can exhaust the host's memory or pids. Out of scope, as in the decided design.
- **The probe's streams are in the user journal.** The container-probe ran under podman's default `journald` log driver, so its calls' streams sit in the user journal until it rotates; the runtime's calls run with `--log-driver=none`.
- **Rootless podman only.** Docker and rootful podman are untried. `keep-id` and `pasta` are Linux-specific.
- **Evidence strength.** Each container cell is n = 1 (R33). The paid egress, canary and engine-socket cells held only because the agent declined to attack. Their enforcement evidence is the scripted cells (F5, Z3, Z7, Z8). No cell tests a readable but forbidden file inside the clone. A call with `hosts` unset (credential proxy only, no proxy variables) and `setup` under `hosts` have run live once each with the stub `claude`; one real call under `hosts` has run (n = 1).
- **Version.** Probed on Claude Code 2.1.294 and podman 6.1.3. At each pin move, check that `claude` runs in the image.

## Runtime

Unit `runtime-core`, plan `docs/plans/runtime-core-plan.md`. Package `lsfrt` in `lsf-runtime/runtime/`; tests run the stub `lsf-runtime/testdata/claude` through the stub `lsf-runtime/testdata/podman`, which execs a container's command on the host, against streams recorded from Claude Code 2.1.289, one (`auth-401.jsonl`) from 2.1.294, and six re-recorded from 2.1.294 under `testdata/streams/2.1.294/` (R17; `testdata/streams/README.md`).

### Interface

- **`Run(ctx, cfg, body)`** is one launch. It takes an `flock` on the state dir, runs the launch checks (Permissions → Image), installs the signal handlers, reaps orphans, runs `body(ctx, rt)`, and writes `summary.json`. Its error is for setup failures only; how the body ended is the summary's `stop_reason`.
- **`rt.Agent(ctx, prompt, opts)`** is the orchestrator's `agent()`. `(nil, nil)` is null. The only error is a `*StopError`, which the orchestrator propagates.
- **`rt.Call`** is the same call, returning a `CallResult`, which carries:
  - the value and the last attempt's class;
  - flags: `skill-not-loaded`, `guard-killed-after-result`, `uncommitted-unchecked`;
  - permission denials;
  - uncommitted paths (see *Uncommitted at return*);
  - for a null after invalid returns, the schema violations;
  - the clone the call ran in: `kept`, `discarded`, or none;
  - for a kept clone, its fetched branch and the clone's HEAD at the fetch;
  - attempts, cost, and whether it was replayed.

  A replay returns the flags, denials, uncommitted paths, branch and clone journaled with the call. While a kept clone exists, the replay fetches its branch again (see Uncommitted at return). The orchestrator calls agents through `rt.Call` and maps these driver events to signals.
- **`ValidateOutput(schema, raw)`** is the validator, exported for the returns the runtime did not see: the orchestrator's seeds. An empty schema is no check.
- **`rt.Parallel(ctx, thunks)`** runs every thunk. A branch's error or panic becomes a nil slot. A stop in any branch is returned once all branches finish.
- **`rt.Step(ctx, label, input, fn)`** journals a code step (see Journal).
- **`rt.Repo()`** is the config's `repo`, the main checkout.
- **`rt.Worktree(ctx, label)`** is a code step's own worktree `wt/<key>` on `lsf/<plan8>/<key>`, created from the integration branch or reused. Code steps are runtime code; agent calls get clones.
- **`rt.RemoveWorktree(ctx, label, integration, deleteBranch)`** removes a label's kept clone, or a code step's worktree, when its branch is merged into `integration` (its tip an ancestor), with every untracked, ignored and uncommitted file in it, and lists those paths (a wholly untracked directory as one `dir/` entry). With `deleteBranch` it also deletes the branch. Kept, with the reason:
  - a label with both a clone and a worktree;
  - a directory that is not a worktree on the label's branch, or not a clone on it or on `integration` (a repo call's clone);
  - a clone whose HEAD is not the branch's tip in the main repo (commits not fetched);
  - a branch not merged, and a directory whose branch is gone.

  The ancestor check is about commits; which labels may lose uncommitted work is the caller's choice.
- **`rt.MoveRetained(ctx, unit, store, integration, paths, sources)`** moves a unit's declared `retains` paths from its writer clones `sources` (oldest first) to `<store>/<unit>/`, before cleanup removes them (see Retained artefacts). It returns the moved paths, each with its source label, method (`rename` or `copy`), file count and bytes, and the paths no clone holds (`missing`). **`rt.CheckStore(store)`** checks a pinned store: an existing directory whose resolved path is not the repository or the state dir, nor under or above either. **`rt.ClonePath(label)`** is where a label's kept clone is.
- **`rt.MergeClone(ctx, name)`** is the merge clone `<stateDir>/merge/<name>` on the integration branch, made or, when it is a clone on that branch, reused as found, a merge in progress included. A directory there that is not one is an error, not a stop. **`rt.RemoveMergeClone(ctx, name)`** removes it.
- **`rt.RunGates(ctx, label, commit, setup, gates)`** runs a plan's declared gate commands on a clean detached checkout of `commit` at `<stateDir>/gates/<key>`, which is removed afterwards. The checkout is a plain clone, not a worktree: `commit` is resolved in the main repo and fetched by its object ID into a new repository of the main repo's object format (no tags; a commit reachable from no branch, such as a merge candidate under `refs/lsf/`, included), which takes the clone setup (line-ending keys, the info files) before the checkout, built at `<key>.partial` and renamed into place. Its git is clone-isolated (Supervision). When the main repo lists a worktree under `gates/` (a gate worktree from before plain clones), removal prunes the stale registrations. A stale checkout that cannot be removed fails the run's checkout. It runs `setup` first, then each gate in order, each in its own container with the checkout and a shared gate home (Permissions → Declared gates), stdin `/dev/null` and its output logged under `<stateDir>/gates/logs`. `setup` has the egress proxy when `hosts` are pinned and no network otherwise; gate commands have none. Setup runs under `gateTimeout`. Exit 0 is `green` and any other exit of a command that started `red`. A timeout is `not-run`, and a failed setup makes every gate `not-run`. A container that fails before its command starts stops the run with `runtime-error`. Any process a command leaves behind ends with its container. A stop removes the running container, and the stop is returned. Runs are bounded by `gateConcurrency`; the caller journals them.
- **`rt.Proxy()`** is the credential proxy's base URL (`http://<listen address>`) and the launch's placeholder bearer, 32 random bytes in hex. The proxy starts after the state-dir lock, before the body, and closes when `Run` returns, its port then refusing. A bind failure or a credentials file with no access token (`claudeAiOauth.accessToken`, or a top-level `accessToken`) is a setup error. For each request it:
  - refuses with 403 and the body `lsf-proxy: placeholder bearer required` unless there is exactly one `Authorization` header and it is `Bearer <placeholder>` (compared in constant time), and refuses `CONNECT`; no refused request reaches the upstream, and the first refusal (method and path quoted) and the launch's count are logged, never a header's value;
  - otherwise forwards to the upstream (`httputil.ReverseProxy`, flushing every write; the default transport's settings, so `HTTPS_PROXY` reaches an https upstream by a tunnel, with a 10-minute bound on response headers) with `Authorization` set to the live token, `x-api-key` and `Proxy-Authorization` dropped and `oauth-2025-04-20` added to `anthropic-beta` when absent; an upstream failure is a 502, logged;
  - re-reads the token first when the file's modification time, size or inode has changed; a failed re-read keeps the previous token and is logged once until a re-read succeeds. A rewrite in place with a token of the same length, within one tick of the filesystem's clock of the last read, is missed until the file next changes. The token is never logged.

  At launch end the proxy waits up to 5 s for requests in flight, then closes, before the summary is written.

  Every call reaches the API through it (Permissions → A call's launch), so `Run` starts it before the body and a body never sees the zero value.
- **`rt.PinHosts(hosts)`** records the gate block's `hosts` as the orchestrator pinned them from the plan at `args.planSha`, before any agent call; **`rt.Hosts()`** reads them (nil when the plan has none). With hosts, it starts the egress proxy that confines agent calls and `setup` to them (Permissions → Network).
- **`rt.Log`** and **`rt.Args`**: `Args` is the config's `args`, passed through.

`opts` mirrors the JavaScript: `label`, `phase` (telemetry only), `model`, `effort`, `agentType`, `isolation`, `schema`. It adds `dir`: an existing directory under `<stateDir>/merge/` to run in as found, with no clone, fetch-back or uncommitted check (the conflict call). `dir` is not in the call's hash, and `dir` with `isolation` is a `runtime-error` stop.

### Configuration

The JSON config is decoded strictly. `testdata/config.example.json` carries the spike's roles and rules as a worked example; it is not a default.

- **Roles.** Each role is `tools` and `clone`, both required. `tools` becomes `--tools`; `clone` (`kept` or `discarded`) is the call's clone kind. The runtime has no built-in role; the Permissions section gives their content.
- **The container** (Permissions → Image, A call's launch). `image` is required, by digest (`name@sha256:<64 hex>`), a tag alone refused. `claudeBin` defaults to `claude` (`LSF_CLAUDE_BIN` overrides it) and is resolved by `PATH` and its symlinks at launch. `claudeVersion` is required. `env` lists variable names passed through. `podmanBin` defaults to `podman`; the config is trusted input, like `claudeBin`. `pluginDir`, the plugin root, is required and mounted at `/opt/plugin`. `stateDir` and `pluginDir` may not contain `:`.
- **Placement refusals.** A repo that is shallow, has submodules (`.gitmodules` at the integration tip, or at `HEAD` when `args` name no integration branch) or a gitlink there without them or uses LFS (`filter.lfs` in its local config, or `filter=lfs` in `.gitattributes` there; a global `git lfs install` alone is not use): clones are untested with them.
- **`repo`** may be left out only by the runtime's own tests: a call then runs in a fresh empty `scratch/<key>`, with no clone, no `.git` binds and no uncommitted check.
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
  - `gateTimeout`: 2 h per declared gate without its own timeout.
- **`gateConcurrency`**: concurrent `RunGates`, 1 by default. Gate time is wall-clock and counts toward `maxWallClock`.
- **`proxy`** (required): `{listen, credentialsFile, upstream}`, the credential proxy. `listen` (`127.0.0.1:port`, port 0 for any; another host is refused) and `credentialsFile` (the live session's `.credentials.json`, no default) are required; `upstream` is an http(s) origin, `https://api.anthropic.com` by default (`http://` serves tests and is refused for a non-loopback host).
- **`dryRun`.** The first call that would spawn logs its podman argv (`<dir>` for the clone, `<session-id>` for the session) and stops the launch with `dry-run`. A step that is not replayed stops without running. Orphans are reported, not killed.

### A call

Each attempt is a `podman run` (Permissions → A call's launch) in its own process group, with the prompt on stdin. `claude` gets `-p --output-format stream-json --verbose --session-id <uuid> --strict-mcp-config --plugin-dir /opt/plugin --tools <role tools> --dangerously-skip-permissions --json-schema <schema>`, plus `--model` and `--effort`. Before each attempt in a clone, a missing `.git/hooks` or `.git/info` is made; a `.git` that is not a directory, or a missing `.git/config`, stops the run with `runtime-error`. The `spawn` telemetry event names the container.

Order inside a call:

1. The journal is checked; a replay returns at once and does not count against the caps.
2. The stop is checked, and so are the schema (a JSON object) and, for every call that uses a clone, `args`.
3. The dry-run check runs.
4. Each attempt waits out the session-limit gate, then takes a concurrency slot and holds it while it:
   - admits or stops in one critical section (stop, parent context, gate again, caps);
   - makes or reuses the call's clone, at its first admitted attempt, so a refused call leaves none;
   - spawns and classifies.

   A session limit moves the gate before the slot frees.
5. After a value or a null (not a stop or a dry run), a kept clone's branch is fetched into the main repo as `lsf/<plan8>/<key>`, without tags, and the clone is checked (Uncommitted at return); a discarded clone is removed. A failed fetch-back stops the run with `runtime-error`, the clone kept.

**Clones.** Every agent call runs in its own plain clone under the state dir, never in the main checkout or a worktree:

| Call | Directory | Checked out |
|---|---|---|
| `opts.dir` set (the conflict call) | that directory, as found | as found |
| isolation `worktree` | `clone/<key>` (kept role) or `scratch/<key>` (discarded) | `lsf/<plan8>/<key>`: the main repo's branch if it exists, else new from the integration branch |
| no isolation | `clone/<key>` or `scratch/<key>` | the integration branch |

- **Making one.** At `<dir>.partial`: `git clone --no-local --no-checkout --no-tags`, a fetch of every main branch as a local branch, the remote removed, then the main repo's effective hooks (copied, `core.hooksPath` set to the copy), its effective `user.name`, `user.email`, signing and line-ending config, its `info/exclude` and `info/attributes`, and a copy of its `core.excludesFile` and `core.attributesFile` set as the clone's own, then the checkout, then a rename to `<dir>`. The runtime's git there is clone-isolated (Supervision). A failure removes the partial directory, and a launch removes one a killed launch left; for a call's clone a failure stops the run with `runtime-error`.
- **Reuse.** A kept clone is reused when it is a clone (its `.git` a directory) at its toplevel with HEAD on its branch; anything else stops the run, naming it. Its other branches are refreshed from the main repo. A repo call's clone whose HEAD no longer descends from the main repo's integration tip is stale: it is remade, unless it holds commits not fetched or uncommitted changes, which stop the run. A discarded clone is always made afresh, and `scratch/` is wiped at launch, after orphan reaping.

Classification reads the `result` event first, because the CLI can recover from a mid-stream error and still return a valid result. First match wins:

| Class | When | Then |
|---|---|---|
| `stopped` | a repeated signal cancelled the attempt | the stop |
| `timeout` | the guard timeout fired before a valid result; one after it keeps the value, flagged `guard-killed-after-result` | null |
| (launch failure) | `podman run` exited 125, 126 or 127 before any stream line (Permissions → Image) | the run stops with `runtime-error`, unjournaled |
| `spawn-error` | the podman process did not start | null |
| `ok` | valid `structured_output`; skill loaded or none required | the value |
| `skill-missing` | as `ok`, but no skill load | retry; on the second miss, the value, flagged `skill-not-loaded` |
| `invalid` | clean result, `structured_output` missing, null or failing the schema | retry, then null |
| `session-limit` | no clean result, and an assistant `error: rate_limit` or a bare 429 (`api_error_status` 429 with no assistant error; a 429 with another assistant error is classed by that error) | wait until `resetsAt` (from a `rejected` `rate_limit_event`, else the fallback) plus margin, then retry; null after `maxSessionWaits` |
| `permanent`, authentication | assistant error `authentication_failed`, or status 401, whatever the cause; also a call the guard timeout kills while the CLI retries a 401 (`api_retry` with `error_status` 401) | the run stops with `auth-failed`; the call is not journaled and a kept clone stays as found, without a fetch-back, so a relaunch re-runs the call in it (one whose integration branch has since moved, holding commits not fetched, stops the relaunch as any stale clone does). Claude Code 2.1.294 retries a 401 ten times itself (about three minutes) before it returns (`testdata/streams/auth-401.jsonl`). A result with no error but no valid output is `invalid` even beside `authentication_failed` |
| `permanent` | assistant error `model_not_found`, `billing_error`, `invalid_request` and the like, or status 400/403/404 | null |
| `transient` | anything else | back off, then null |

- `invalid` and `skill-missing` share one retry.
- A session limit moves a runtime-wide gate that every call waits on. It is assumed to apply across models. It does not spend the transient budget.
- The schema validator covers the subset the stage schemas use: `type`, `properties`, `required`, `items` and `enum`, with JavaScript's integer and number-equality semantics.
- **Skill load.** A slash-command expansion is not echoed on stdout. In probes, the init event's `slash_commands` listed the command exactly when it expanded, and an unknown command was sent as plain text. So a prefixed call loaded its skill if the init event lists it. A `Skill` tool call naming it also counts.
- **Guard timeout, or a second signal.** The `podman run` process group gets SIGTERM, which `--init` and podman's signal proxy carry into the container, and the client SIGKILL after `killGrace`. Then the container is removed (Permissions → Teardown), and the group gets SIGKILL once the leader is reaped, which reaches a client-side survivor. A call that exits 0 is neither removed (`--rm` did) nor group-killed. A removal that fails stops the run, the clone unread.

### Uncommitted at return

Every call in a kept clone that spawned and returns a value or a null reports the paths it left uncommitted: modified, staged or untracked, but not ignored. The check comes from `git status --porcelain=v1 -z --untracked-files=all --no-optional-locks`, run at the clone's toplevel. The clone is the call's own, so every dirty path is the call's, a failed attempt's before a retry included.

- **A discarded clone and an explicit `dir`** get no check.
- **A call ended by a stop or a repeated signal, or a dry run,** gets no check. A check that the stop itself cut short is not a failure.
- **A failed check** (git error or timeout) sets `uncommitted-unchecked`. The record keeps the failure as `unchecked`, not the flag, and a replay checks afresh.
- **The journal** keeps the paths. A replay of a kept call whose clone still exists fetches its branch again; a failed fetch there sets `uncommitted-unchecked` rather than stopping. When the record has paths or `unchecked`, the replay re-checks the clone as it now stands; a removed clone is clean. A record journaled clean is not re-checked. So committing or removing the paths in the clone, then resuming, clears the signal, and the commit reaches the main repo.
- **A record from a worktree** (an isolated call journaled before clones) is re-checked in its `wt/<key>` while that exists.

### Retained artefacts

A unit's `retains` paths (its `lsf-plan` entry) move to `<store>/<unit>/`, the store being the `lsf-gates` block's `store`, pinned at launch (`lsf-empirical-work.md` §Retained artefacts, §Units U8). The move runs inside the journaled `cleanup:w<n>` step, before the unit's clones are removed.

- **Source.** Each path comes from the latest writer clone holding it, ranked `plan` < `fix-plan:rN` < `implement` < `fix-code:rN` (by N, then a `#n` suffix). That clone must be one `RemoveWorktree` would remove; if not, the move fails, and an older copy is never used. Copies in other clones, reviewers' included, go with their clones and are listed. A path in no clone is `missing`: logged, not signalled.
- **Checks first.** Before anything changes: the unit id must name one directory, the destination must not exist, no component of a path in its clone may be a symlink, only regular files, directories and symlinks are moved, and a copy must fit the store's free space. A refusal leaves the clones and the store as they were.
- **Move.** Paths go into `<store>/.lsf-retain-<h8>-<key>.partial/`, each at its final name only once whole, and that directory is renamed to `<store>/<unit>` last. A path on the store's filesystem is renamed. On `EXDEV` it is copied to the sibling `….copy/` scratch directory: each file is hashed (sha256) as it is read, the copy is fsynced, re-read and compared (sha256, size and mode per file, each symlink's target, the set of entries), and renamed into staging. Only then is the source removed. A rename refused for permissions is an error, not a copy. Every directory between a moved path and the store is fsynced before the record is written.
- **Record.** `<statedir>/retained/<key>.json` names the staging and scratch directories and each path moved, through the phases `moving`, `renaming` and `done`. A rerun adopts only the staging directory its record names, so it resumes its own partial move rather than stop on it. Once `done`, it returns the recorded result. An unreadable record stops the run with `runtime-error`. The move has no cleanup on error: an error leaves what a kill at that point would, and names the staging, scratch and record files.
- **Failure.** A failed move keeps all of the unit's clones and branches, fails the cleanup step, and is a `retained-move-failed` record (orchestrator D33) naming the clones and the by-hand remedy. A second signal cancels a copy between 1 MiB chunks (`cancelled`); the first waits for the step.

### Journal and state dir

| Path | Holds |
|---|---|
| `lock` | the launch's `flock` |
| `journal/<key>.json` | a completed call's or step's record: label, kind, input hash, return, flags, denials, uncommitted paths, whether the uncommitted check failed, and a call's branch, clone kind and clone HEAD |
| `journal/<key>.intent` | a step's input hash while it runs |
| `calls/<key>.<launch>.a<n>.<session>.jsonl` | each attempt's stream |
| `clone/<key>` | kept clones |
| `scratch/<key>` | discarded clones, wiped at launch |
| `merge/w<n>` | merge clones |
| `wt/<key>` | code steps' worktrees, on `lsf/<plan8>/<key>` |
| `gates/<key>`, `gates/<key>.home`, `gates/logs` | a declared gate run's checkout and gate home while it runs, and its logs |
| `retained/<key>.json` | a unit's retained-move record (`<key>` from the unit id); its staging and scratch directories are in the store, `.lsf-retain-<h8>-<key>.partial` and `.copy`, `<h8>` from the state dir's path |
| `status.json`, `summary.json`, `log.txt`, `telemetry.jsonl` | launch state, the `stop_reason`, the log, one event per line |

- **Keys.** `<key>` is the label made filename-safe plus 8 hex digits of its hash. Labels must be unique per call: the journal keeps one record per label, and a label reused with other inputs in one launch is logged `label-reused`.
- **Call records.** A call's hash covers the prompt, the skill it is sent as, the schema, the model, the effort and the isolation. Role and permissions are excluded, so changing them does not re-run completed work, with one exception: a repo call recorded from a discarded clone re-runs once its role keeps its clone, since its commits were lost. A null return is not recorded, so a resume retries it. A call with the same label and inputs is replayed within a launch as well as across launches.
- **Step records.** A step writes an intent, runs, records its return (null included) and removes the intent. A step that errors keeps its intent, so the next run of that step, in this launch or a later one, gets `interrupted=true` for its own effect check. An unreadable record of a step stops the run with `runtime-error` rather than rerun it blind.
- **Replays.** A replayed value is byte-identical to the live one (compacted JSON).
- **File writes.** Records, intents, the status and the summary are written by temp file and rename.
- **Mode.** The state dir is made `0700`, and one readable by group or others (an earlier runtime made them `0755`) is changed to `0700`, logged: a transcript can carry the placeholder bearer (an agent's `env`), which spends the credential through the proxy. The launch resolves the state dir's symlinks once and uses that path throughout.

### Supervision

- **Signals.** The first SIGINT or SIGTERM sets the sticky stop `signal-SIGINT` or `signal-SIGTERM`. In-flight attempts finish and their results are journaled. Retries, backoff and gate waits are abandoned, and every later boundary returns the stop. A second signal cancels in-flight attempts and ends the ctx a running code step was given, so its git stops too. A journal replay is still served after a stop, since it costs nothing; the stop returns at the first call that would spawn.
- **`stop_reason` precedence.** A stop the runtime recorded wins, even if the body swallowed it. Then a `StopError` from the body, then `cancelled` (the parent context), then `crashed` (a body error or panic), then `returned`.
- **Caps** are checked at spawn. A cap that passes during a step or a backoff stops the run at the next spawn.
- **Orphans.** At launch, before `scratch/` is wiped, every container carrying the state dir's label is a call or gate left by a killed launch, and is removed with `podman rm -f -t 0`. A failed `podman ps`, or a container not removed, is a setup error: the launch refuses to start. Then every `gates/` entry but `logs` (checkouts, homes, half-built checkouts) is removed. A `children.json` an earlier runtime left is logged (`children-dropped`) and removed, not acted on. In a dry run, orphans are reported, not removed.
- **Clones and worktrees.**
  - Creation, fetch-back and removal are serialised. One 5-minute bound covers each whole operation (a clone's creation, a fetch-back, a removal), and its git ends on a repeated signal; the runtime's git runs with `core.fsmonitor=false`, since an agent can write its clone's config.
  - **Git config isolation.** The runtime's and the orchestrator's own git in a clone (its creation, the uncommitted check, a removal's reads, the merge step in its merge clone, the gate checkout) runs with `GIT_CONFIG_GLOBAL=/dev/null` and `GIT_CONFIG_NOSYSTEM=1`, and without config from the environment (`GIT_CONFIG_PARAMETERS`, `GIT_CONFIG_COUNT` and its keys). A driver a clone's `.gitattributes` selects (a filter, a diff or merge driver) therefore runs only if the clone's local config defines it, and an agent cannot select a host-defined one there (outside clones, see Known limits). This needs git 2.32 or later for `GIT_CONFIG_GLOBAL`; the runtime does not check the version. Git in the main checkout and in code steps' worktrees keeps the host config: commits there need the user's identity, and a worktree shares the main repo's config. A clone takes, besides the `configCopied` keys (identity, signing including the `gpg.*program` keys, line endings), a copy of the files the main repo's `core.excludesFile` and `core.attributesFile` name, under `.git/lsf-global/` and set as the clone's own `core.excludesFile` and `core.attributesFile`, so they keep their place below `.gitignore` and `.gitattributes`.
  - A call admitted to spawn waits for its clone without yielding to a stop set since; a fetch-back runs after a first signal, as the call's result is journaled.
  - A code step's existing `wt/<key>` is reused only if it is a worktree on its branch; otherwise the run stops with `runtime-error`. A branch whose directory is gone is re-attached, not reset.
  - Missing `planSha` or `integrationBranch` in `args` stops the run with `runtime-error`.

### Not in this unit

- Cleanup's ignored-file policy: `code-merge-cleanup`.
- The roles' content: `s2-permissions-design`.

### Known limits

- **Earlier attempts' clones.** Cleanup removes the clones a merged unit's stages used, made or replayed, in the launch that merged it; a relaunch that replays a unit to its merge removes the earlier launch's clones with it. The clones of an attempt that did not merge (an aborted wave, or a unit the launch did not merge) stay, with their branches: they may hold work a stop left uncommitted. So does a seeded stage's.
- **A repo writer's clone that never fast-forwards** (a null, a stop, a fault) stays, with its branch; no unit's cleanup removes it. The call's re-run reuses it as found, or remakes it when the integration branch has moved, unless it then holds commits not fetched or uncommitted changes, which stop the run.
- **The main checkout during a run.** It must stay on the integration branch with no staged or unstaged change to a tracked file and no merge, cherry-pick or revert in progress: every fast-forward checks it, and a requester edit there is a stage-fault at the next one. A fast-forward updates its work tree; git refuses one over an untracked file in the way, also a stage-fault.
- **A merge clone after a fault or stop** stays, possibly mid-merge, for the relaunch to resume; one whose integration branch has diverged from the main repo's must be removed by hand. A failed removal after the wave's merges is only logged.
- **A failed cleanup is not retried.** A merged unit is not revisited, so a clone or worktree whose removal failed stays, with its branch; the log names it.
- **A failed retained move** is retried only by `resume` of the same launch, which reruns the failed `cleanup:w<n>` step. A relaunch from an outcome is a new launch whose cleanup covers only its own clones; the `retained-move-failed` record gives the remedy by hand.
- **What a verified copy checks.** The re-read goes through the page cache, so it catches copy faults, not media faults. Ownership, extended attributes, hard links (copied as separate files), sparseness and directory mtimes are not preserved, and staging directories made above a nested path take their clone counterparts' modes plus owner `rwx`.
- **Abandoned staging.** A staging or scratch directory a failed move leaves is in the store for the requester; `status` does not report it. A destination created between the check and the final rename fails that rename and leaves the data in staging.
- **Tracked files under a retained path** are moved too, and also appear in the removal listing as deleted tracked files.
- **Config a clone lacks.** Only the identity, signing and line-ending keys and the global excludes and attributes files are copied: a repo's own `filter.*` or `merge.*` drivers, or `core.fileMode`, do not reach the clone, so its commits can differ from the main checkout's. Other global config (`url.*.insteadOf`, `safe.directory`, credential helpers, `init.templateDir`) does not apply to the runtime's git in a clone.
- **What an agent can write in its clone's `.git`.** Its `config`, `hooks` and `info` are bound read-only, so it cannot define a filter, a `core.hooksPath` or an `include.path` that the runtime's own git there would run. `.git/lsf-global/` (the copied global excludes and attributes files) stays writable: an agent can change what the clone ignores or which attributes apply, not define a driver.
- **Hooks copied from the host run in the container.** A hook that needs a host-only tool fails there, and the agent's commit with it.
- **The project's hooks run on the host over agent-written trees.** The merge step's commit in the merge clone runs the hooks copied from the main repo (read-only in the container, so their content is the project's), on the host; a hook that runs the project's tests or tools runs code the agents wrote.
- **Isolation needs git 2.32.** On older git `GIT_CONFIG_GLOBAL` is ignored and the runtime's git in a clone reads global config again. `Run` does not check the version; the CLI refuses a git older than 2.38 (CLI → Configuration).
- **Global `safe.directory`.** A main repo owned by another user that relies on a global `safe.directory` fails clone creation and the gate checkout (their fetch runs isolated) with a `runtime-error` stop.
- **Hooks in a clone see no global config.** A hook the merge step's commit or merge runs in its merge clone, and whatever it spawns, inherits the isolated environment. The identity, signing and line-ending keys are local in the clone, so the step's own commits are unaffected; a hook that reads other global config does not see it.
- **Host config outside clones.** Code steps' worktrees and the main checkout's fast-forward read the host's global and system config, so a driver a fast-forwarded commit's `.gitattributes` selects runs there if the host defines it. A main-repo fetch from a clone (a fetch-back, the merge step's transfer) runs `upload-pack` in the clone under host config; it reads the clone's local config but runs no filter or hook.
- **Template hooks.** A hook `git clone` installs from `init.templateDir` that the main repo lacks stays in the clone's hooks directory, unused. No hook runs during a clone's creation.
- **What a repo call sees.** A clone holds committed content and the main repo's `info/exclude`: a repo call (the review gate, the obligations stage, a re-plan) does not see the requester's untracked or ignored files in the main checkout.
- **A switched kept clone.** A call that leaves its kept clone on another branch stops its re-run ("is not on <branch>") until the clone is removed by hand.
- **Clone cost.** Every call copies the repo's objects (`--no-local`) and every main branch, serialised with other clone work. On a large repo this is per-call time and disk, unmeasured; a clone that takes more than 5 minutes stops the run with `runtime-error`.
- **A forced fetch-back.** A kept clone's branch overwrites the main repo's `lsf/<plan8>/<key>` at each return and each replay while the clone exists; a commit made on that branch in the main repo instead is kept only by the reflog.
- **A replay that fails its schema** (a changed validator, an edited record) re-runs the call. For a writer, the re-run can repeat a commit the first run made.
- **`hosts` allows private addresses.** Only loopback, unspecified and IPv4-compatible addresses are refused: link-local (`169.254.169.254`), RFC 1918 and internal names pass. `Hosts()` is nil both before the pin and for a plan with none.
- **Gate checkout cost.** A gate checkout fetches the commit's whole history from the main repo, once per gated tree, under the 5-minute bound of the runtime's git; on a large repo that is per-gate time and disk, unmeasured, and a checkout past the bound fails the gate run. It has no tags or branches, so a gate that runs `git describe` against tags fails there.
- **Large files.** The merge step compares a file over 64 MiB outside the merge's paths, before and after the conflict call, by size and modification time, not content.
- **Session limits.** The shape of a session limit under `-p` is built from the binary's schema and interactive transcripts, not observed. The first real limit's stream is kept under `calls/`.
- **No `rate_limit_event` through the proxy.** In the `s2-sandbox` fixture runs (2.1.294, 16 calls in containers over two runs, the credential reaching the CLI as `ANTHROPIC_AUTH_TOKEN` with a base URL) no transcript carried a `rate_limit_event`, though every host-direct recording does. A session limit's wait would then use `sessionLimitFallback`, not `resetsAt`, and whether the CLI still reports the limit as an assistant `rate_limit` error on this path is unverified; a bare 429 classes `session-limit` for that reason. Cause unverified (likely the CLI tracks the unified limits only for its own login).
- **Retries in a dirty clone.** A retry after a call made partial changes reruns in the same clone, a discarded one included.
- **One podman launch failure ends a run.** A sporadic 125 or 126 (a transient `pasta` or storage fault) is not retried; the relaunch resumes from the journal.
- **Group kill after reaping.** After the leader of a cancelled call is reaped, its pgid could in principle be reused before the group SIGKILL. Sequential pid allocation makes this need a wrapped pid space.
- **Orphan grandchildren.** An orphan whose group leader is gone is not signalled.
- **Descendants of a normal exit.** A call that exits normally while a descendant holds its stdout returns `killGrace` later. The descendant is not killed; what happens to processes a stage detaches is `long-running-design`'s question.
- **Unprobed error results.** `is_error` results with subtypes such as `error_max_turns` and no listed status class as transient and are retried. No real stream of one is recorded.
- **macOS.** `ps -A -ww`, `flock` and process groups have not been run on macOS.

## CLI

Unit `runtime-cli-drive`, plan `docs/plans/runtime-cli-drive-plan.md`. The `lsf-runtime` command (`lsf-runtime/cli`, package `main`) launches a run as a detached driver over a state dir, and reports on it. Until `runtime-distribution` ships a launcher, it is built with `go -C <plugin root>/lsf-runtime build -o <bin> ./cli`.

### Addressing

A run is addressed by `-state DIR`, or by `-repo DIR` (default: the toplevel of the current directory's repository) and `-name NAME`. With `-state`, a `-name` or `-repo` that names another run than its `launch.json` is refused. The default state dir is `${XDG_STATE_HOME:-~/.local/state}/lsf-runtime/<first 12 hex digits of the SHA-256 of the repo toplevel, symlinks resolved>/<name>`. A name matches `^[A-Za-z0-9][A-Za-z0-9._-]*$` and makes a valid `refs/lsf/<name>/pause` (`git check-ref-format`); `args.name` is checked the same way and must equal `-name` when both are given. A relative `XDG_STATE_HOME` or `XDG_CONFIG_HOME` is ignored, and with no absolute `HOME` the default is refused.

### Configuration

A launch's config is composed, then loaded strictly (Runtime → Configuration):

1. **Plugin defaults**, embedded in the binary (`cli/defaults.json`): `defaultRole`, `roles`, `roleRules`, `skills`, the settings the orchestrator's stage labels need. They equal `testdata/config.example.json`'s (a test keeps them equal).
2. **The host file**: `-config FILE`, else `${XDG_CONFIG_HOME:-~/.config}/lsf-runtime/config.json`; a missing file is refused, naming the path. It is a JSON object, and each top-level key replaces the default's whole. It carries the host's settings: `image` (by digest), `claudeBin`, `claudeVersion`, `podmanBin`, `pluginDir`, `proxy`, `env`, caps and timings. `stateDir`, `repo`, `args` and `dryRun` are refused: the CLI sets them per launch. A relative `pluginDir`, and a relative `claudeBin` holding a `/`, are resolved against the host file's directory.
3. **The CLI's fields**: `repo`, `stateDir`, `args` and `dryRun`; `pluginDir` from `-plugin-dir` when given, which beats the host file; `claudeBin` and `claudeVersion` (below).

A load error reaches the requester as the runtime gives it. One naming `allow`, `deny` or `permissionMode` adds that the config predates the container boundary (Permissions) and the key is to be removed.

`setup` and the gate commands see only the image's environment and the `env` passthrough. A private registry's credential is therefore passed by name through `env`, and their toolchain must be in the image.

**claudeBin, once per launch.** The source is `LSF_CLAUDE_BIN`, else the host file's `claudeBin`, else `claude`. It is resolved by `PATH`, made absolute and its symlinks resolved to a regular file, the versioned path, which is run with `--version` under `DISABLE_AUTOUPDATER=1` (30 s bound); the output's first field is the version. A host file's `claudeVersion` that differs is refused, naming both; otherwise the version fills it. The composed config stores the resolved path, and the driver runs with `LSF_CLAUDE_BIN` removed and `DISABLE_AUTOUPDATER=1`, so the runtime resolves the same file: a `claude` repointed after the launch resolved it is not used. A file overwritten in place is caught by the launch check, whose smoke container compares its version with `claudeVersion`.

**git.** A launch refuses a git older than 2.38 (`git merge-tree --write-tree`), or a `git version` it cannot parse, naming the version found.

### Commands

| Command | Does |
|---|---|
| `launch -args FILE\|- [addr] [-config FILE] [-plugin-dir DIR] [-dry-run]` | Checks the args and git, composes the config, starts the detached driver and waits, at most 5 minutes, until it has passed or failed the launch checks. Prints `{"stateDir","pid","launch"}`. |
| `status [addr]` | Prints the status report without blocking. |
| `watch [addr]` | Blocks until no driver holds the state dir's lock, then prints the status report. |
| `pause [-lift] [addr]` | Sets the pause ref to the repo's `HEAD` (`git update-ref`), or with `-lift` deletes it, and verifies it (`git show-ref --verify`). The orchestrator reads it at its next wave boundary, not at once. The ref and repo come from `launch.json`; without one, from the address, with the default ref and a warning that a launch passing another `args.pauseRef` reads that one. |
| `stop [addr]` | SIGTERM to the running driver (Supervision → Signals). The first lets in-flight calls finish, which can take as long as the longest call; a second `stop` cancels them. |
| `resume [addr] [-config FILE] [-plugin-dir DIR]` | Replays the last launch (below). |
| `drive -state DIR -launch FILE` | Hidden: the driver. |

Exit codes: 0 success; 1 a refusal or error, with its message on stderr; 2 usage. `status` and `watch` exit 3 for `crashed` or `failed-to-start` and 4 for `none`; the report is authoritative. Their 0 says the report was made, not that the run succeeded: an `ended` run's `summary.stop_reason` says how it ended. Run a built binary: `go run` reports every non-zero exit as 1, and its binary is removed when it exits, while the driver re-executes it.

### Launch

1. `args` is a JSON object whose `name` follows the name rule. git is checked and the state dir made (`0700`). `launch` takes an exclusive `flock` on `<state>/launch.lock` until it returns, so a second launch or a dry run meanwhile is refused, and a running driver on the state dir refuses the launch. The config is composed into `<state>/launch.<launcher pid>.json`.
2. The driver is the same executable, run as `drive` in a session of its own (`Setsid`), stdin `/dev/null`, stdout and stderr appended to `<state>/driver.log` (kept across launches, not rotated), with `LSF_CLAUDE_BIN` removed and `DISABLE_AUTOUPDATER=1`. `launch` records it in `<state>/driver.json` (`pid`, `started`, `state`, `launchFile`), the CLI's own record of the current launch. The driver runs `lsfrt.Run` with `orchestrator.Body`; its body first renames the composed file to `<state>/launch.json`, as the lock is held by then, so a refused launch never replaces the last good one, and removes any other `launch.*.json` a launch left.
3. `launch` polls `status.json` every 200 ms. Its `pid` equal to the driver's means the launch checks passed: `launch` waits up to 5 s for the rename to `launch.json` (warning on stderr if it has not happened), prints and exits 0, and the driver runs on after it, and after the session that started it. If the driver exits first, `status.json` is read once more, since a run can start and end within one poll. Otherwise the driver's `driver.log` output and its exit are printed and `launch` exits 1, removing the composed file: an old config, a launch-check refusal, a failed reaping or proxy reach the requester this way. A refusal because the lock was in use while no driver runs (a `status` or `watch` probe held it for an instant) is retried, up to four starts in all. After 5 minutes, or on SIGINT, `launch` exits 1 and leaves the driver running, and its composed file with it. Leaving releases `launch.lock`; a launch started before that driver takes the state dir's lock (a few milliseconds after its start) then replaces `driver.json`. When that launch's driver is refused because the earlier driver holds the lock and is running, `launch` restores the earlier `driver.json`. If it too gives up first, `status` recognises the earlier driver by `status.json`'s pid (Status, the table), `stop` signals it, and its end is reported as `failed-to-start` with the run's summary and outcome under `previous`.
4. `-dry-run` runs in the foreground, holding `launch.lock`, in the scratch dir `<state>/dry-run`, wiped first, so the run's own `launch.json`, `status.json` and `summary.json` stay as they were. It prints the summary, and exits 0 only when the `stop_reason` is `dry-run`; a setup refusal exits 1 with its message.

`watch` takes a blocking shared `flock` on `<state>/lock`, granted when the driver's exclusive one is released at its exit, crash included, then reads the status; while that says `starting` or `running` (a driver spawned but not yet holding the lock, or a new launch), it waits again. The probe `launch` and `status` use is the non-blocking form, released at once.

### Resume

`resume` relaunches the state dir's `launch.json`: its stored args and repo, re-composed against the current host config, so a renewed credential or a raised cap applies; `-plugin-dir` defaults to the stored `pluginDir`, which is refused when it no longer exists, and another one is used with a warning that completed calls replay as they were. The journal replays completed calls. It serves a run that ended with no outcome to act on: a runtime stop (`auth-failed`, `runtime-error`, a cap, a signal), a crash, or a failed start. It refuses while a driver runs, without a `launch.json`, and when `launch.json`'s run returned (`status.json`'s state `returned` and `summary.json`'s `stop_reason` `returned`, the summary being that launch's): an outcome (`paused`, `wave-cap`, `stopped`, `complete`) is relaunched with `launch -args`, from the outcome's `planSha` and with its `unscribed`, as `phronithm:lsf-drive`'s Relaunch says. After a crash `status.json` stays `running`, and `summary.json` is an earlier launch's, so it is not read; a failed start after a crash changes neither.

### Stop

`stop` signals only a `running` driver (Status, the table): `status.json`'s pid, whose `/proc/<pid>/cmdline` must also be `drive -state <state>`. `status.json` says `running` only after `Run` has installed its signal handlers, so the SIGTERM is caught; while the driver is `starting` (its launch checks), `stop` refuses and is retried later, as SIGTERM's default action would kill it with no summary. A run not launched by the CLI (no `driver.json`) is not signalled.

### Status

The report is JSON:

```
{ "stateDir", "repo", "name",
  "driver": "starting" | "running" | "ended" | "crashed" | "failed-to-start" | "none",
  "driverLog",                       // driver.log's last 4 KB, for crashed and failed-to-start
  "pid", "launch", "state", "at",    // status.json's
  "summary": {stop_reason, detail, error, calls, cost_usd, started, finished},   // ended only
  "outcome",                         // summary.json's outcome, ended only
  "previous": {launch, summary, outcome},   // failed-to-start only: the last launch that ran
  "pause": {"ref", "set"},
  "leftovers": {"clone", "scratch", "merge", "partial"},
  "xferRefs", "containers", "containersError", "errors" }
```

`driver` is read from `driver.json` (D), `status.json` (S) and the lock:

| Lock | D | S's pid | S's state | `driver` |
|---|---|---|---|---|
| free | none | — | — | `none`, whatever `status.json` or `launch.json` hold |
| held | none | — | — | `running`, or `starting` before S says running (a launch not made by the CLI) |
| held | yes | ≠ D's | `running`, S's pid a live `drive` on this state dir | `running` (an earlier launch's driver won the lock after D was recorded) |
| held | yes | ≠ D's | otherwise | `starting` |
| held | yes | = D's | — | `running` |
| free | yes | = D's | not `running` | `ended` |
| free | yes | = D's | `running` | `crashed` (the lock re-probed after `status.json` is read) |
| free | yes | ≠ D's | — | `starting` while D's pid is a live `drive` on this state dir (spawned, the lock not yet taken), else `failed-to-start` |

`summary` and `outcome` are shown only for `ended`, so an earlier launch's outcome is never reported as the current one. For `failed-to-start`, `previous` shows the last launch that ran, when `status.json`'s state is its summary's `stop_reason`: that outcome is still the one a relaunch starts from. `summary.json` is read only when its start time is the one `status.json`'s launch id is made from (`Run` only logs a failed write of it); another is named in `errors`. `leftovers` lists the entries of `clone/`, `scratch/` and `merge/`, and `partial` the `*.partial` entries of those and `gates/`. `xferRefs` are the main repo's `refs/lsf/<plan8>/xfer/w<n>` refs. `containers` are those carrying the state dir's `lsf.state` label (`podman ps -a`); a failure is reported in `containersError`. `repo`, `name`, `podmanBin` and the pause ref (`args.pauseRef`, else `refs/lsf/<name>/pause`) come from `launch.json`, or without one from the address. A piece that cannot be read is named in `errors` (a missing file is not); each git and podman command is bounded by 10 s, and `status` itself does not fail.
