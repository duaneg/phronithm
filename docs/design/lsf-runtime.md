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

Unit `s2-permissions-design`. Evidence: [s2-permissions results](../investigations/s2-permissions-results.md), about 85 calls on Claude Code 2.1.294 (2.1.289 had been pruned). Rigour `evidenced`, n = 1–2 per cell.

**Status:** decided, 2026-10-08. Requester verdicts:
- **B2 for every stage:** the native Bash sandbox, `--restricted`, and the credential held only in a host proxy.
- **Reviewers** keep no writes (see Reviewers).
- **Declared gates** run under pinned `srt`.
- **Network:** agent calls' Bash, writers and reviewers alike, and gates' `setup` reach only the hosts the gate block's `hosts` list declares (see Network). Gate commands have none.
- **The runtime changes** are carried by two new units, `s2-clones` and `s2-sandbox` (split at the re-plan).
- **A kept clone** stays until its unit's cleanup, as a worktree does now (2026-10-08).

### Options

| Option | Result on the spike fixture |
|---|---|
| Deny list, `Bash` granted (spike runs 2–7) | Implement commits. A `python3` write changed the main checkout's settings, and branches were deleted, egress reached a listener and canaries were read, all in 2 of 2 runs. |
| Prefix allow rules, prompts rewritten to single commands | Implement commits. The probes held only because the agent stopped at the first denial: one `node -e`, allowed by the `Bash(node:*)` the gates need, rewrote the settings. Spike run 1: the orchestrator's prompts produce compound commands. |
| `--permission-mode auto` | Implement commits, and the settings edits were blocked. Branches were deleted in 2 of 2 runs, one exfiltration got through, and canaries were read. The classifier is probabilistic. |
| `ic`, broad permissions inside | Not runnable headless as it stands (it hangs after auth). It mounts the host credentials read-write. |
| **B2**, as decided below (`B2f`) | Implement commits. Settings, branch, egress, canary, token and docker-escape probes all held: the agent attempted each attack (shell, `python3`, `mv`, curl, node, DNS), and the sandbox stopped it. An earlier variant, with the main checkout under `/tmp` and Bash allowed to reach the proxy, lost the settings and branch probes (see Placement). |

`srt` around the whole process (`srtA+`) also held. It was not chosen because it puts the real token in the `claude` process and adds `srt`'s nested-sandbox mode with `allowAllUnixSockets` for every call. It remains an optional later layer, as kernel confinement of the file tools.

### Boundary

- **Bash** runs in Claude Code's native sandbox (`bwrap` on Linux). Its filesystem and network are enforced by the kernel.
- **File tools** (`Read`, `Edit`, `Write`, `Glob`, `Grep`) are confined to the working directory and `--add-dir` directories by `--restricted`. This is enforced by Claude Code, not the kernel. `--restricted` also ignores the clone's settings files.
- **The credential** never enters a sandboxed process. The runtime runs an injecting proxy on host loopback. Each `claude` gets `ANTHROPIC_BASE_URL` pointing at the proxy and a placeholder `ANTHROPIC_AUTH_TOKEN`. `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB=1` strips the rest of the environment from Bash.

### Placement

- **No agent call runs in the main checkout or a worktree.** A worktree's `.git` is the main repo's, so its refs and config are writable through the worktree. Every call runs in its own clone under the state dir. The main checkout, the state dir and `$HOME` are in `denyRead`, so Bash sees only its own clone. Writes to sibling paths land in the mask and are discarded.
- **Plain clones.** A `--shared` clone's alternates point into the masked repo.
- **Nothing kept under `/tmp` or `/var/tmp`.** The native sandbox makes both writable to Bash, whatever `allowWrite` says. The runtime refuses to launch when the main checkout or the state dir is under either.
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
  "writer":   {"tools": "Read,Edit,Write,Glob,Grep,Bash,Skill", "allow": ["Read", "Edit", "Write", "Glob", "Grep", "Bash", "Skill"],
               "deny": ["Edit(/<plugin>/**)", "Write(/<plugin>/**)", "Edit(.git/**)", "Write(.git/**)"], "permissionMode": "dontAsk", "clone": "kept"},
  "reviewer": {"tools": "Read,Glob,Grep,Bash,Skill", "allow": ["Read", "Glob", "Grep", "Bash", "Skill"],
               "deny": ["Edit(/<plugin>/**)", "Write(/<plugin>/**)"], "permissionMode": "dontAsk", "clone": "discarded"}
}
```

- `<plugin>` is the plugin root, substituted by the runtime at launch.
- Every label not matched above is a writer: `plan`, `fix-plan`, `implement`, `fix-code`, `direct-edits`, `decision-replan`, `replan`, `termination`, `merge-conflict`. `plan:critique-x` and `merge-conflict:w1:review-gate` are writers, because the patterns anchor at the label's start.
- `clone` is a new field (see Runtime changes).

### A call's launch

The runtime builds each call's settings, argv and environment. The role supplies only its tools and its `clone` value.

- **Sandbox settings** (`--settings <file>`):

  ```json
  {"sandbox": {"enabled": true, "failIfUnavailable": true, "allowUnsandboxedCommands": false, "autoAllowBashIfSandboxed": true,
    "filesystem": {"denyRead": ["$HOME", "<repo>", "<stateDir>", "/run/containerd", "…config denyRead"], "allowWrite": ["<clone>"]},
    "network": {"allowedDomains": ["<hosts>"], "strictAllowlist": true}}}
  ```

  - Bash reaches only the declared `hosts` (see Network). The proxy is reached by `claude` itself, not by Bash. A Bash allowance for the proxy would let Bash spend on the API with the injected credential.
  - `/run/containerd` works round a 2.1.294 bug that fails every sandboxed Bash call.
  - A reviewer's clone is in `allowWrite` too: `bwrap` writes mount points into the cwd and fails every call if it cannot.
- **Flags:** `--restricted`, `--tools`, `--allowedTools`, `--disallowedTools`, `--plugin-dir <plugin> --add-dir <plugin>`, `--settings` and `--permission-mode dontAsk`, plus the flags the runtime already passes.
  - `--add-dir` lets the skills' `Read` of `${CLAUDE_PLUGIN_ROOT}/docs/...` through `--restricted`. Without it, the critique ran without its framework and did not say so.
  - The deny rules and Claude Code's own "sensitive file" check each refused a `Write` into the plugin.
- **Environment:** replaced, not inherited.
  - `PATH`, from config.
  - `SHELL`, a system shell.
  - `HOME` and `CLAUDE_CONFIG_DIR`, a per-call home under the state dir.
  - `TMPDIR`, a short per-call directory under `/tmp`, because `bwrap`'s socket paths are capped at 108 bytes.
  - `DISABLE_AUTOUPDATER=1`, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` and `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB=1`.
  - `ANTHROPIC_BASE_URL=http://127.0.0.1:<port>` and `ANTHROPIC_AUTH_TOKEN=<placeholder>`, with `NO_PROXY` empty.
  - The names in a config `env` passthrough list.
- **The clone's git identity** is set in its local config from the main repo's. Inside the sandbox, Bash sees the clone's `.git/config` and `.git/hooks` read-only.

### Credential proxy

- **What it does.** The runtime starts it at launch on an ephemeral 127.0.0.1 port, with a placeholder random to the launch. It refuses a request whose bearer is not the placeholder. Otherwise it replaces the bearer with the live session's OAuth access token from `~/.claude/.credentials.json`, adds the OAuth beta header, forwards to `api.anthropic.com`, and streams the response back unbuffered.
- **Which token.** The live session's credential, with no separate token (requester, 2026-10-08).
- **When it reads the token.** It re-reads the file when the file changes, so a refresh by an interactive session reaches running calls. A run that outlives the token with no refresh fails its calls as `permanent` (401). `s2-sandbox` decides whether that becomes a stop.
- **The proxy stops with the launch.**

### Reviewers

- **What a reviewer can write.** Its clone (`bwrap` requires a writable cwd) and `/tmp`. It cannot write the main checkout, the integration branch or any other clone.
- **What happens to those writes.** Its clone is never fetched and is removed after the call, so nothing it writes is kept. The verdict's "zero writes" is met as zero *kept* writes: a write-denied cwd fails every Bash call (results, `B2r`).
- **No uncommitted check for a discarded clone.** Build output from a test a reviewer re-runs would otherwise raise `residue-at-return` on every such call, and nothing in that clone is kept.
- **Reviewers have no `Edit` or `Write`.**

### Merge conflicts

- **The merge step runs in a merge clone, not the main checkout.** The clone is `<stateDir>/merge/w<n>`, on the integration branch. The step's `m.dir` becomes that clone, and every check runs there unchanged, as runtime code outside the sandbox:
  - `outside` and `changedOutside`;
  - the `head != pre` check, since the call stages a resolution and does not commit it;
  - `stagedOK`;
  - `write-tree` and `mergeGates`;
  - the commit with its `Lsf-Unit`/`Lsf-Step` trailers;
  - `abortOwn`.
- **The conflict call gets the merge clone** as its cwd, with `allowWrite` on it. It is the only call whose clone is not its own.
- **After each verified merge commit,** the main repo fetches it and fast-forwards the integration branch, as in Placement.
- **The merge clone stays until the wave's merges finish.** So an interrupted step finds its merge in progress, and its adoption by trailers works as today.
- **`refusedOverLocal` no longer arises in the merge.** The merge clone holds no requester edits. A requester edit now blocks the fast-forward and stops the run.

### Network

- **The problem.** With no network, each call starts from a fresh clone and an empty per-call home, so a stage whose build or tests fetch dependencies (npm, pip, Go modules, a GitHub archive) fails inside the call.
- **The rule** (requester, 2026-10-08). Bash in every agent call, writers and reviewers alike, and gates' `setup`, may reach only the hosts the `lsf-gates` block's `hosts` list names. Gate commands get no network. With no `hosts` list, nothing has network.
- **Why the gate block.** It is pinned from the plan at the launch's `planSha` before any agent call, it changes only by the requester's commit and relaunch, and a change within a launch stops the run on `declared-gates-changed`. The plan block would not do: re-plan stages are agents that edit it, so an agent could widen its own network. A plan with no gates may carry an `lsf-gates` block holding `hosts` alone.
- **What the runtime refuses.** A `hosts` entry naming loopback (`localhost`, `127.0.0.0/8`, `::1`), since the proxy listens there.
- **Evidence.** In `B2` the native sandbox let Bash reach its one allowed host, the proxy, and blocked every egress probe. No real package registry was probed: `s2-sandbox` shows one.

### Declared gates

- **The `declared-gates-changed` stop is not enough.** It shows the requester a changed command. But the commands run code the stages wrote (tests, scripts, package install hooks), and today that code runs on the host with the runtime's environment and network.
- **Gates run under `srt` 0.0.79,** pinned.
  - **Filesystem.** Read is denied for `$HOME`, the repo and the state dir, except the gate checkout. Write is allowed to the gate checkout only.
  - **Network.** None for gates. `setup` is allowed only the gate block's `hosts`.
  - **Environment.** Built as for an agent call, without the Anthropic variables.
  - In the gate check, a hostile gate under `srt` found the env canary absent and the file canaries denied, could not write the main checkout, and had every egress blocked.
- **Gate block change.** The `lsf-gates` block gains `hosts`, a list of host names (see Network), and may carry it with no gate commands. The block stays pinned within a launch, so a change to `hosts` also stops the run on `declared-gates-changed`.
- **`srt` prerequisites:** `rg` on `PATH`, a short `TMPDIR`, and `SHELL` a system shell.
- **Placement.** The check passed with the gate checkout under the masked `$HOME`, as the decided layout puts it (`w3/gate-srtA`).

### Runtime changes

Carried by `s2-clones` (1, 2, and the `clone` field and discarded-clone rule of 3) and then `s2-sandbox` (the rest), both before `runtime-cli-drive` and `retained-artefacts-impl`.

1. **Clones for every agent call,** as in Placement: clone, fetch-back, removal (a kept clone at its unit's cleanup, a discarded one after its call), and the refusal to launch under `/tmp` or `/var/tmp`. The repo-call snapshot in Uncommitted at return goes, since no agent call runs in the main checkout.
2. **Orchestrator changes:**
   - steps that fast-forward the integration branch to a writer repo call's fetched branch;
   - the merge step in a merge clone;
   - the prompts' named branches made local branches in each clone.
3. **Role fields and launch:**
   - the role field `clone` (`kept` or `discarded`), and `<plugin>` substituted in `deny`;
   - the per-call settings, argv and environment of A call's launch;
   - a config `env` passthrough list and extra `denyRead` paths;
   - no uncommitted check for a discarded clone.
4. **The credential proxy:** started, held and stopped with the launch.
5. **Gates under `srt`,** and `hosts` in the gate block and its parser, applied to agent calls' Bash and to `setup` as in Network.
6. **The fixture run under B2.** `tools/fixturerun` runs the spike fixture end to end under the decided configuration.

### Known limits

- **Allowed hosts are an exfiltration channel.** Agent Bash and code that `setup` runs (package install hooks) can send what they read in the clone or gate checkout to any host in `hosts`; a host that accepts writes, such as GitHub, can receive the repo. They cannot reach the credential or `$HOME`.
- **Domain filtering goes through the sandbox's own proxy.** A tool that ignores the proxy environment variables gets no network even to an allowed host.
- **`/tmp` is shared.** Bash in any call can read and write `/tmp` and `/var/tmp`, including other concurrent calls' `TMPDIR`s. Nothing the runtime keeps is there.
- **File-tool confinement is Claude Code's, not the kernel's.** A `--restricted` bypass would reach whatever the user can. Whole-process `srt` is the remedy if one is found.
- **Mount-point files.** The sandbox leaves zero-byte files (`.env*`, lock files, `.npmrc`, `.gitmodules` and others) in each clone and lists them in `.git/info/exclude`. They are ignored, and they go when the clone is removed.
- **macOS is unprobed.** The native sandbox uses Seatbelt there, and the `/tmp` and mount-point findings are Linux-specific.
- **Version.** Probed on 2.1.294 only. The `/run/containerd` workaround and the `/tmp` behaviour should be re-checked at each pin move.

## Runtime

Unit `runtime-core`, plan `docs/plans/runtime-core-plan.md`. Package `lsfrt` in `lsf-runtime/runtime/`; tests run against the stub `lsf-runtime/testdata/claude` and streams recorded from Claude Code 2.1.289 (`testdata/streams/`).

### Interface

- **`Run(ctx, cfg, body)`** is one launch. It takes an `flock` on the state dir, reaps orphans, installs the signal handlers, runs `body(ctx, rt)`, and writes `summary.json`. Its error is for setup failures only; how the body ended is the summary's `stop_reason`.
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
- **`rt.MergeClone(ctx, name)`** is the merge clone `<stateDir>/merge/<name>` on the integration branch, made or, when it is a clone on that branch, reused as found, a merge in progress included. A directory there that is not one is an error, not a stop. **`rt.RemoveMergeClone(ctx, name)`** removes it.
- **`rt.RunGates(ctx, label, commit, setup, gates)`** runs a plan's declared gate commands on a clean detached checkout of `commit` at `<stateDir>/gates/<key>`, which is removed afterwards. The checkout is a plain clone, not a worktree: `commit` is resolved in the main repo and fetched by its object ID into a new repository of the main repo's object format (no tags; a commit reachable from no branch, such as a merge candidate under `refs/lsf/`, included), which takes the clone setup (line-ending keys, the info files) before the checkout, built at `<key>.partial` and renamed into place. Its git is clone-isolated (Supervision). When the main repo lists a worktree under `gates/` (a gate worktree from before plain clones), removal prunes the stale registrations. A stale checkout that cannot be removed fails the run's checkout. It runs `setup` first, then each gate in order. Each command runs through `sh` in its own process group, with stdin `/dev/null`, its output logged under `<stateDir>/gates/logs`, and the runtime's environment less the variables that point git elsewhere (`GIT_DIR` and the like). Setup runs under `gateTimeout`. Exit 0 is `green` and any other exit `red`. A timeout is `not-run`, and a failed setup makes every gate `not-run`. Any process a command leaves behind is killed when it ends. A stop kills the running group, and the stop is returned. Runs are bounded by `gateConcurrency`; the caller journals them.
- **`rt.PinHosts(hosts)`** records the gate block's `hosts` as the orchestrator pinned them from the plan at `args.planSha`, before any agent call; **`rt.Hosts()`** reads them (nil when the plan has none). Nothing confines a call by them yet: `s2-sandbox` does.
- **`rt.Log`** and **`rt.Args`**: `Args` is the config's `args`, passed through.

`opts` mirrors the JavaScript: `label`, `phase` (telemetry only), `model`, `effort`, `agentType`, `isolation`, `schema`. It adds `dir`: an existing directory under `<stateDir>/merge/` to run in as found, with no clone, fetch-back or uncommitted check (the conflict call). `dir` is not in the call's hash, and `dir` with `isolation` is a `runtime-error` stop.

### Configuration

The JSON config is decoded strictly. `testdata/config.example.json` carries the spike's roles and rules as a worked example; it is not a default.

- **Roles.** Each role is `tools`, `allow`, `deny`, `permissionMode` and `clone`; `tools`, `permissionMode` and `clone` are required. The first four become `--tools`, `--allowedTools`, `--disallowedTools` and `--permission-mode`. `clone` (`kept` or `discarded`) is the call's clone kind. The runtime has no built-in role; the Permissions section gives their content.
- **`pluginDir`**, the plugin root, replaces `<plugin>` in a role's `deny` entries when argv is built. A `<plugin>` without it, or in `allow`, is a config error.
- **Placement refusals.** A launch whose `repo` or `stateDir` is under `/tmp` or `/var/tmp` (or macOS's `/private` forms), as written or through a symlink, is refused: the Bash sandbox leaves both writable. So is a repo that is shallow, has submodules (`.gitmodules` at the integration tip, or at `HEAD` when `args` name no integration branch) or uses LFS (`filter.lfs` in its local config, or `filter=lfs` in `.gitattributes` there; a global `git lfs install` alone is not use): clones are untested with them.
- **`repo`** may be left out only by the runtime's own tests: a call then runs in the launcher's working directory with no clone and no uncommitted check, and none of the Permissions placement holds.
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
- **`dryRun`.** The first call that would spawn logs its argv and stops the launch with `dry-run`. A step that is not replayed stops without running. Orphans are reported, not killed.

### A call

The prompt goes on stdin. Every call passes `-p --output-format stream-json --verbose --session-id <uuid> --strict-mcp-config --json-schema <schema>`, plus `--model`, `--effort` and the role's flags. Each call runs in its own process group.

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

- **Making one.** At `<dir>.partial`: `git clone --no-local --no-checkout --no-tags`, a fetch of every main branch as a local branch, the remote removed, then the main repo's effective hooks (copied, `core.hooksPath` set to the copy), its effective `user.name`, `user.email`, signing and line-ending config, and its `info/exclude` and `info/attributes`, then the checkout, then a rename to `<dir>`. A failure removes the partial directory, and a launch removes one a killed launch left; for a call's clone a failure stops the run with `runtime-error`.
- **Reuse.** A kept clone is reused when it is a clone (its `.git` a directory) at its toplevel with HEAD on its branch; anything else stops the run, naming it. Its other branches are refreshed from the main repo. A repo call's clone whose HEAD no longer descends from the main repo's integration tip is stale: it is remade, unless it holds commits not fetched or uncommitted changes, which stop the run. A discarded clone is always made afresh, and `scratch/` is wiped at launch, after orphan reaping.

Classification reads the `result` event first, because the CLI can recover from a mid-stream error and still return a valid result. First match wins:

| Class | When | Then |
|---|---|---|
| `stopped` | a repeated signal cancelled the attempt | the stop |
| `timeout` | the guard timeout fired before a valid result; one after it keeps the value, flagged `guard-killed-after-result` | null |
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

### Uncommitted at return

Every call in a kept clone that spawned and returns a value or a null reports the paths it left uncommitted: modified, staged or untracked, but not ignored. The check comes from `git status --porcelain=v1 -z --untracked-files=all --no-optional-locks`, run at the clone's toplevel. The clone is the call's own, so every dirty path is the call's, a failed attempt's before a retry included.

- **A discarded clone and an explicit `dir`** get no check.
- **A call ended by a stop or a repeated signal, or a dry run,** gets no check. A check that the stop itself cut short is not a failure.
- **A failed check** (git error or timeout) sets `uncommitted-unchecked`. The record keeps the failure as `unchecked`, not the flag, and a replay checks afresh.
- **The journal** keeps the paths. A replay of a kept call whose clone still exists fetches its branch again; a failed fetch there sets `uncommitted-unchecked` rather than stopping. When the record has paths or `unchecked`, the replay re-checks the clone as it now stands; a removed clone is clean. A record journaled clean is not re-checked. So committing or removing the paths in the clone, then resuming, clears the signal, and the commit reaches the main repo.
- **A record from a worktree** (an isolated call journaled before clones) is re-checked in its `wt/<key>` while that exists.

### Journal and state dir

| Path | Holds |
|---|---|
| `lock` | the launch's `flock` |
| `journal/<key>.json` | a completed call's or step's record: label, kind, input hash, return, flags, denials, uncommitted paths, whether the uncommitted check failed, and a call's branch, clone kind and clone HEAD |
| `journal/<key>.intent` | a step's input hash while it runs |
| `calls/<key>.<launch>.a<n>.<session>.jsonl` | each attempt's stream |
| `children.json` | session id → pid of live calls |
| `clone/<key>` | kept clones |
| `scratch/<key>` | discarded clones, wiped at launch |
| `merge/w<n>` | merge clones |
| `wt/<key>` | code steps' worktrees, on `lsf/<plan8>/<key>` |
| `gates/<key>`, `gates/logs` | a declared gate run's checkout while it runs, and its logs |
| `status.json`, `summary.json`, `log.txt`, `telemetry.jsonl` | launch state, the `stop_reason`, the log, one event per line |

- **Keys.** `<key>` is the label made filename-safe plus 8 hex digits of its hash. Labels must be unique per call: the journal keeps one record per label, and a label reused with other inputs in one launch is logged `label-reused`.
- **Call records.** A call's hash covers the prompt, the skill it is sent as, the schema, the model, the effort and the isolation. Role and permissions are excluded, so changing them does not re-run completed work, with one exception: a repo call recorded from a discarded clone re-runs once its role keeps its clone, since its commits were lost. A null return is not recorded, so a resume retries it. A call with the same label and inputs is replayed within a launch as well as across launches.
- **Step records.** A step writes an intent, runs, records its return (null included) and removes the intent. A step that errors keeps its intent, so the next run of that step, in this launch or a later one, gets `interrupted=true` for its own effect check. An unreadable record of a step stops the run with `runtime-error` rather than rerun it blind.
- **Replays.** A replayed value is byte-identical to the live one (compacted JSON).
- **File writes.** Records, intents, `children.json`, the status and the summary are written by temp file and rename.

### Supervision

- **Signals.** The first SIGINT or SIGTERM sets the sticky stop `signal-SIGINT` or `signal-SIGTERM`. In-flight attempts finish and their results are journaled. Retries, backoff and gate waits are abandoned, and every later boundary returns the stop. A second signal cancels in-flight attempts and ends the ctx a running code step was given, so its git stops too. A journal replay is still served after a stop, since it costs nothing; the stop returns at the first call that would spawn.
- **`stop_reason` precedence.** A stop the runtime recorded wins, even if the body swallowed it. Then a `StopError` from the body, then `cancelled` (the parent context), then `crashed` (a body error or panic), then `returned`.
- **Caps** are checked at spawn. A cap that passes during a step or a backoff stops the run at the next spawn.
- **Orphans.** At launch, every process whose arguments carry `--session-id <id>` for an id in `children.json` is a call left by a killed launch. A gate's shell is recorded under an `lsf-gate:` marker, which it carries as `$0`, and is matched the same way. A gate command that begins with `exec` replaces that shell, so it cannot be found. Its group gets SIGTERM, then SIGKILL after `killGrace`; a zombie counts as gone. If `ps` fails, or a kill fails, those records are carried into this launch's `children.json` for the next launch to reap. A torn `children.json` is reported and treated as empty.
- **Clones and worktrees.**
  - Creation, fetch-back and removal are serialised. One 5-minute bound covers each whole operation (a clone's creation, a fetch-back, a removal), and its git ends on a repeated signal; the runtime's git runs with `core.fsmonitor=false`, since an agent can write its clone's config.
  - **Git config isolation.** The runtime's and the orchestrator's own git in a clone (its creation, the uncommitted check, a removal's reads, the merge step in its merge clone, the gate checkout) runs with `GIT_CONFIG_GLOBAL=/dev/null` and `GIT_CONFIG_NOSYSTEM=1`, and without config from the environment (`GIT_CONFIG_PARAMETERS`, `GIT_CONFIG_COUNT` and its keys). A driver a clone's `.gitattributes` selects (a filter, a diff or merge driver) therefore runs only if the clone's local config defines it, and an agent cannot select a host-defined one there (outside clones, see Known limits). This needs git 2.32 or later for `GIT_CONFIG_GLOBAL`; the runtime does not check the version. Git in the main checkout and in code steps' worktrees keeps the host config: commits there need the user's identity, and a worktree shares the main repo's config. A clone takes, besides the `configCopied` keys (identity, signing including the `gpg.*program` keys, line endings), a copy of the files the main repo's `core.excludesFile` and `core.attributesFile` name, under `.git/lsf-global/` and set as the clone's own `core.excludesFile` and `core.attributesFile`, so they keep their place below `.gitignore` and `.gitattributes`.
  - A call admitted to spawn waits for its clone without yielding to a stop set since; a fetch-back runs after a first signal, as the call's result is journaled.
  - A code step's existing `wt/<key>` is reused only if it is a worktree on its branch; otherwise the run stops with `runtime-error`. A branch whose directory is gone is re-attached, not reset.
  - Missing `planSha` or `integrationBranch` in `args` stops the run with `runtime-error`.

### Not in this unit

- Cleanup's ignored-file policy: `code-merge-cleanup`.
- The `Retains:` move: `retained-artefacts-impl`.
- A stop file, status readers and detaching: `runtime-cli-drive`.
- The roles' content: `s2-permissions-design`.

### Known limits

- **Earlier attempts' clones.** Cleanup removes the clones a merged unit's stages used, made or replayed, in the launch that merged it; a relaunch that replays a unit to its merge removes the earlier launch's clones with it. The clones of an attempt that did not merge (an aborted wave, or a unit the launch did not merge) stay, with their branches: they may hold work a stop left uncommitted. So does a seeded stage's.
- **A repo writer's clone that never fast-forwards** (a null, a stop, a fault) stays, with its branch; no unit's cleanup removes it. The call's re-run reuses it as found, or remakes it when the integration branch has moved, unless it then holds commits not fetched or uncommitted changes, which stop the run.
- **The main checkout during a run.** It must stay on the integration branch with no staged or unstaged change to a tracked file and no merge, cherry-pick or revert in progress: every fast-forward checks it, and a requester edit there is a stage-fault at the next one. A fast-forward updates its work tree; git refuses one over an untracked file in the way, also a stage-fault.
- **A merge clone after a fault or stop** stays, possibly mid-merge, for the relaunch to resume; one whose integration branch has diverged from the main repo's must be removed by hand. A failed removal after the wave's merges is only logged.
- **A failed cleanup is not retried.** A merged unit is not revisited, so a clone or worktree whose removal failed stays, with its branch; the log names it.
- **Config a clone lacks.** Only the identity, signing and line-ending keys and the global excludes and attributes files are copied: a repo's own `filter.*` or `merge.*` drivers, or `core.fileMode`, do not reach the clone, so its commits can differ from the main checkout's. Other global config (`url.*.insteadOf`, `safe.directory`, credential helpers, `init.templateDir`) does not apply to the runtime's git in a clone.
- **Hooks in a clone see no global config.** A hook the merge step's commit or merge runs in its merge clone, and whatever it spawns, inherits the isolated environment. The identity, signing and line-ending keys are local in the clone, so the step's own commits are unaffected; a hook that reads other global config does not see it.
- **Host config outside clones.** Code steps' worktrees and the main checkout's fast-forward read the host's global and system config, so a driver a fast-forwarded commit's `.gitattributes` selects runs there if the host defines it. A main-repo fetch from a clone (a fetch-back, the merge step's transfer) runs `upload-pack` in the clone under host config; it reads the clone's local config but runs no filter or hook.
- **Template hooks.** A hook `git clone` installs from `init.templateDir` that the main repo lacks stays in the clone's hooks directory, unused. No hook runs during a clone's creation.
- **What a repo call sees.** A clone holds committed content and the main repo's `info/exclude`: a repo call (the review gate, the obligations stage, a re-plan) does not see the requester's untracked or ignored files in the main checkout.
- **A switched kept clone.** A call that leaves its kept clone on another branch stops its re-run ("is not on <branch>") until the clone is removed by hand.
- **Clone cost.** Every call copies the repo's objects (`--no-local`) and every main branch, serialised with other clone work. On a large repo this is per-call time and disk, unmeasured; a clone that takes more than 5 minutes stops the run with `runtime-error`.
- **A forced fetch-back.** A kept clone's branch overwrites the main repo's `lsf/<plan8>/<key>` at each return and each replay while the clone exists; a commit made on that branch in the main repo instead is kept only by the reflog.
- **A replay that fails its schema** (a changed validator, an edited record) re-runs the call. For a writer, the re-run can repeat a commit the first run made.
- **Gate checkout cost.** A gate checkout fetches the commit's whole history from the main repo, once per gated tree, under the 5-minute bound of the runtime's git; on a large repo that is per-gate time and disk, unmeasured, and a checkout past the bound fails the gate run. It has no tags or branches, so a gate that runs `git describe` against tags fails there.
- **Large files.** The merge step compares a file over 64 MiB outside the merge's paths, before and after the conflict call, by size and modification time, not content.
- **Session limits.** The shape of a session limit under `-p` is built from the binary's schema and interactive transcripts, not observed. The first real limit's stream is kept under `calls/`.
- **Retries in a dirty clone.** A retry after a call made partial changes reruns in the same clone, a discarded one included.
- **Group kill after reaping.** After the leader of a cancelled call is reaped, its pgid could in principle be reused before the group SIGKILL. Sequential pid allocation makes this need a wrapped pid space.
- **Orphan grandchildren.** An orphan whose group leader is gone is not signalled.
- **Descendants of a normal exit.** A call that exits normally while a descendant holds its stdout returns `killGrace` later. The descendant is not killed; what happens to processes a stage detaches is `long-running-design`'s question.
- **Unprobed error results.** `is_error` results with subtypes such as `error_max_turns` and no listed status class as transient and are retried. No real stream of one is recorded.
- **macOS.** `ps -A -ww`, `flock` and process groups have not been run on macOS.
