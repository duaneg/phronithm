# runtime-distribution: problem statement, design and plan

Work unit `runtime-distribution` of `docs/plans/lsf-improvements-breakdown.md` (entry at `c7bf8d2`). Base commit `c7bf8d2`, branch `lsf-workflow`. Phases 1–3 ran in compressed mode, with one critique gate on this document. On approval, step 0 saves this file as `docs/plans/runtime-distribution-plan.md`.

## Context

Plugin users can't run the Go runtime today. Both skills call `go -C ${CLAUDE_PLUGIN_ROOT}/lsf-runtime run ./validate/cmd/lsf-validate`, which needs Go and reports every non-zero exit as 1. `phronithm:lsf-drive`'s CLI route builds the CLI into `~/.cache` with `go build` (runtime-cli-drive's interim) and needs a hand-written host config. This unit ships a launcher, CI, a release build and a starter host config, so that `script-retirement` can make the runtime the only Phase 3 path.

## Requester decisions (2026-10-11)

- **Route: D1, first-use download** (requester verdict). The git marketplace source is kept, a launcher downloads a sha256-pinned release binary, and D4 (CI-built `archive` source) is rejected.
- **GitHub:** pushing `lsf-workflow` to run Actions, and publishing one pre-release tag to exercise the real download, are authorised. I still confirm immediately before each push and tag.
- **CI container tests:** live tests are skipped in CI and named in the Distribution section. CI runs the stub-podman suite.
- **Starter caps:** the starter host config carries no `caps` (no cap).
- **Critique gate:** round 1 passed (sonnet, no Critical). Significant S1–S8 and Minor M1–M7 are folded in above. S3 is narrowed by the record-before-staging order, and S5's "opt-in proxy" premise is false (`proxy` is required).

## Unit entry (verbatim, plan at c7bf8d2)

### runtime-distribution
- **Type**: integration
- **Risk**: medium — decides how plugin users get the runtime; D4 changes how the whole plugin ships
- **Clarity**: exploratory — one open decision: D1 (first-use download of a release binary) or D4 (a CI-built `archive` marketplace source)
- **Iteration**: write-once
- **Effort**: medium
- **Scope**: `lsf-runtime/dist`, `lsf-runtime/cli`, `.github/workflows/lsf-runtime.yml`, `skills/lsf-drive/SKILL.md::launch`, `skills/lsf-drive/SKILL.md::startup`, `skills/breakdown/SKILL.md::quality-check`, `docs/design/lsf-runtime.md::distribution`
- **Publishes**: the launcher the skills invoke; the release build; the starter host config
- **Depends on**: runtime-cli-drive (logical, the CLI the launcher runs; serialises `skills/lsf-drive/SKILL.md::launch` and `::startup`); runtime-language-design (design, the routes); s2-boundary-decision (design, the boundary's prerequisites); s2-sandbox (logical, the reference `Containerfile` and the launch checks); plan-preflight-check (logical, `lsf-validate`; serialises `skills/breakdown/SKILL.md::quality-check`)
- **External gates**: `github-actions` (environment); `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`); `git` (environment, 2.38 or later on every CI runner); `podman` (environment, 6.1.3 rootless, on the CI runner if the container tests run there); `requester-verdict` (organisational)
- **Done condition**: The Distribution section is committed, choosing D1 or D4, with the requester's verdict recorded. CI builds static binaries for linux on amd64 and arm64 with `CGO_ENABLED=0`, and runs the `lsf-runtime` tests on Linux, orphan reaping included. A launcher at `${CLAUDE_PLUGIN_ROOT}/lsf-runtime/` runs the platform's binary, and the plan-validation command through it. `phronithm:breakdown`'s quality check and `phronithm:lsf-drive`'s Startup run `lsf-validate` through the launcher, not `go run`, and tell problems from a command failure by its exit code (1 or 2), not by empty stdout. It prefers a locally built binary or one named by an environment variable, so `--plugin-dir` development and `shakedown-fixture` run the working tree. Under D1 it refuses a binary whose sha256 does not match its pin. The Distribution section names rootless podman with `pasta` as prerequisites, and says how a user builds an image from the reference `Containerfile` with the project's toolchain and records its digest in the runtime config. CI runs the container tests, or the section names those that skip without podman. `phronithm:lsf-drive` invokes the runtime through the launcher, replacing Startup's build into `${XDG_CACHE_HOME:-~/.cache}/lsf-runtime/` (`runtime-cli-drive`'s interim); there is no top-level `bin/`. When no runtime host config exists, the launcher or the CLI writes a starter one with the fields the requester must supply (the image digest) marked, never overwriting an existing file, and `phronithm:lsf-drive`'s Startup offers it before falling back to the Workflow route (requester, 2026-10-10). `lsf-runtime status` lists `.lsf-retain-*` staging and scratch directories left in the pinned store beside its other leftovers (`retained-artefacts-impl`, Discoveries 3; requester, 2026-10-11). A plugin installed from a local marketplace runs the runtime on this host through the launcher. The tests pass.
- **Description**: `lsf-runtime.md` Language, Open for the re-plan: no unit owned the route, the CI or the release, though the spike's estimate counted packaging. Linux only: Windows outside WSL (requester verdict, 2026-10-06; R16) and macOS (requester, 2026-10-09) are out of scope. Until this unit, both skills call `go -C ${CLAUDE_PLUGIN_ROOT}/lsf-runtime run ./validate/cmd/lsf-validate`, which needs Go and reports every non-zero exit as 1, and the CLI route needs a hand-written host config (`lsf-runtime.md` `## CLI` → Configuration). **Flag:** five surfaces exceed the surface bound; the skill and design changes are a few lines of text each and the `cli` change is one subcommand or flag, so the excess is not context.

**Standing obligations carried:** Register dispositions (this unit disposes of no register row; the report says so). Design of record (`lsf-runtime.md::distribution` is amended in the same commits as the behaviour it describes).

## Success criteria

Rigour ladder: `proven` (an automated check re-checks it) ▸ `evidenced` (run once this cycle) ▸ `conjectured` (inferred).

| # | Criterion | Target |
|---|---|---|
| SC1 | The Distribution section is committed, choosing D1, with the verdict (2026-10-11) recorded | evidenced |
| SC2 | CI cross-builds `CGO_ENABLED=0` linux amd64 and arm64 binaries and checks they are static | proven (CI job), evidenced on GitHub |
| SC3 | CI runs `go test -race ./...` for `lsf-runtime` on Linux, orphan reaping included | proven (CI job), evidenced on GitHub |
| SC4 | The launcher runs the platform's binary, and `validate` through it | proven (launcher tests) |
| SC5 | breakdown's quality check and lsf-drive's Startup run `validate` through the launcher, telling exit 1 (problems) from 2 (the command's failure) | evidenced (one run on this plan; text review) |
| SC6 | The launcher prefers `LSF_RUNTIME_BIN`, then a working-tree build, over a release | proven |
| SC7 | The launcher refuses a binary whose sha256 differs from its pin | proven |
| SC8 | The Distribution section names rootless podman plus `pasta`, the image build from the reference Containerfile, and how the digest is recorded | evidenced |
| SC9 | The Distribution section names the live tests that CI skips | evidenced |
| SC10 | lsf-drive invokes the launcher, the interim cache build is gone, and there is no top-level `bin/` | evidenced (grep) |
| SC11 | `lsf-runtime config init` writes a starter host config with `image` marked, never overwrites, and is offered by lsf-drive Startup before the Workflow fallback | proven (CLI test) + evidenced (text) |
| SC12 | `status` lists this state dir's `.lsf-retain-*` staging and scratch directories | proven |
| SC13 | A plugin installed from a local marketplace runs the runtime on this host through the launcher | evidenced |
| SC14 | The `lsf-runtime` tests pass | proven; load noted (R35: machine at ~9.5 load at planning) |

## Candidates

- **D1, first-use download.** Chosen by the requester. A broken release breaks only LSF. Every other skill keeps shipping by git. Costs: curl and network at first use, and a two-step release (tag at X, publish, commit pins at X+1). Skills and binary can drift between releases.
- **D4, CI-built archive.** Rejected. It puts the whole plugin behind a release zip (a broken release breaks every skill), and it is also two-step unless the pin is left off. Whether the exec bit survives zip extraction is undocumented (plugin docs fetched 2026-10-11).
- (D2, a first-use source build, needs Go on every user's machine. D3, committing binaries, was rejected in runtime-language-design.)

## Exploration findings (verified this session)

- `${CLAUDE_PLUGIN_ROOT}` and `${CLAUDE_PLUGIN_DATA}` are substituted inline in skill bodies but are **not** in the Bash tool's environment (plugins-reference, fetched 2026-10-11). So the launcher cannot read `CLAUDE_PLUGIN_DATA` itself, and its cache defaults to `${XDG_CACHE_HOME:-~/.cache}/lsf-runtime/` (a deviation from the D1 text, recorded).
- Copied plugin caches have **no `.git`** (`~/.claude/plugins/cache/phronithm/phronithm/0.3.1/`) and keep exec bits (the `tools/*.py` copies are 755). A directory-source marketplace plugin loads in place from its working tree, which has `.git`. So a `.git` at the plugin root marks a working tree.
- `lsf-runtime/cli/cli` is a committed 13.7 MB dynamic ELF (added in `c4ba790` by mistake). This unit deletes it and gitignores it (the history blob stays).
- `validate/cmd/lsf-validate/main.go` is a `package main` (`run`, exit 0/1/2). The CLI cannot import it.
- The retain staging and scratch dirs are `<store>/.lsf-retain-<h8>-<key>.{partial,copy}`, with h8 the first 4 bytes of sha256(abs state dir). Records are at `<state>/retained/*.json` with `store`, `staging` and `scratch` (`runtime/retain.go:50,482`).
- There is no `.github/` yet. The repo `duaneg/phronithm` is public, with no releases or tags. GitHub's `ubuntu-24.04` runner has git 2.43 (≥ 2.38) and node. Its podman is 4.9 (not the pin; live tests skip).
- Embedded files: `cli/defaults.json` only.

## Design

### Launcher: `lsf-runtime/lsf-runtime` (POSIX sh, mode 755)

Invoked as `${CLAUDE_PLUGIN_ROOT}/lsf-runtime/lsf-runtime <command> [args]` and `exec`s the binary with the args unchanged. Resolution, first match wins:

The script runs with `set -eu` and a `trap` that removes its temp files (download, build output) on exit or interrupt.

1. **`LSF_RUNTIME_BIN`** set: it must be an executable file, else exit 125. exec it.
2. **Working tree:** `<root>/.git` exists (file or dir) and `go` is on `PATH`. Then `go -C <root>/lsf-runtime build -o <root>/lsf-runtime/.build/lsf-runtime.<pid> ./cli`. If the output differs from `.build/lsf-runtime` (`cmp -s`), it is `mv -f`'d into place (atomic). An unchanged tree therefore never replaces a binary a live driver re-executes (critique M4). Then exec. The build runs on every invocation: Go's build cache makes a no-op build quick, and the binary never lags the tree.
   - **A failed build exits 125 with the compiler's output and never falls through** to a release (critique S4).
   - With a working tree but no `go`, it says on stderr that the working tree is not being run and names the release tag it falls through to (step 3).
3. **Release:** read `<root>/lsf-runtime/dist/release` (lines `tag <tag>`, `commit <sha>`, `sha256 <arch> <hex>`; `commit` is for the drift check and records only). `uname -m` maps `x86_64`→`amd64` and `aarch64|arm64`→`arm64`; anything else exits 125. With no tag or no pin for the arch, exit 125: "no release pinned; set LSF_RUNTIME_BIN or run from a working tree with Go". The binary lives at `${LSF_RUNTIME_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/lsf-runtime}/release/<tag>/lsf-runtime-linux-<arch>`. If it is missing, it is downloaded to a `mktemp` file in that directory with `curl -fsSL --proto '=https' --tlsv1.2` from `${LSF_RUNTIME_RELEASE_URL:-https://github.com/duaneg/phronithm/releases/download}/<tag>/lsf-runtime-linux-<arch>`. A non-default URL also allows `file` (`--proto '=https,file'`) for tests. The sha256 is checked (`sha256sum`, else `shasum -a 256`) on **every** run, cached file included.
   - A fresh download that mismatches is removed and refused, naming both hashes.
   - A cached file that mismatches is refused with "remove <path> and rerun" (it is never re-downloaded silently).
   - A good download is `chmod 755`'d and `mv`'d into place.
- Launcher failures exit **125** with a `lsf-runtime launcher:` prefix on stderr. 125 is distinct from every CLI code (0–4) and from `validate`'s 0/1/2 (critique S2). The skills' rule is "1 = problems; 2, 125 or any other non-zero = the command's own failure: report stderr". So the done condition's "1 or 2" holds, with 125 as a further failure code.
- Needs `sh`, `uname`, `mktemp`, `curl`, `sha256sum` or `shasum`, all standard on Linux.
- No top-level `bin/`. `lsf-runtime/.build/` is gitignored.

### Release: `lsf-runtime/dist/`

- `dist/release`: the pin file. It starts with no tag (launcher step 3 refuses). `dist/pin.sh <tag>` fetches the release's `SHA256SUMS` with `gh release download` and rewrites the file (tag, `git rev-list -n1 <tag>` commit, two sha256 lines).
- `dist/doc.go` (package `dist`) and `dist/launcher_test.go`: Go tests that run the launcher script (below).
- **Release order (D1):** push tag `lsf-runtime-v<semver>` at X, and CI builds and publishes the release. Then `dist/pin.sh <tag>`, commit at X+1, and bump the plugin version there.
- **Drift check** (critique S1): `dist/check-pin.sh` fails unless `git diff --quiet <commit> HEAD -- lsf-runtime ':!lsf-runtime/dist' ':!lsf-runtime/lsf-runtime'`. CI runs it as a **failing** job on pushes to `main`, on pull requests to `main` and on plugin-version tags, but not on `lsf-workflow`, where it would fail on every push. So a stale pin cannot reach `main` unnoticed. This unit ends by re-tagging and re-pinning after its last `lsf-runtime/` change (step 10), so the pin it leaves matches HEAD.
- Binary identity: `-ldflags "-s -w -X main.version=<tag> -X main.commit=<sha>"`, `-trimpath`, `CGO_ENABLED=0`, plus a new `lsf-runtime version` subcommand that prints `<version> <commit>` (`dev` for working-tree builds).

### CI: `.github/workflows/lsf-runtime.yml`

- Triggers: push and pull_request on paths `lsf-runtime/**`, `workflows/**` (the parity tools read the JS) and the workflow file itself, plus tag `lsf-runtime-v*`. `permissions: contents: read` by default.
- **test** job, matrix `ubuntu-24.04` and `ubuntu-24.04-arm`:
  - `actions/setup-go` with `go-version-file: lsf-runtime/go.mod`, and a check that git is ≥ 2.38.
  - `go vet ./...` and `go test -race ./...` in `lsf-runtime`, with `GRAPHPARITY_REQUIRE_NODE=1` and `PROMPTPARITY_REQUIRE_NODE=1` (node is on the runner) and `LSF_LIVE_REQUIRE` unset, so live tests skip.
  - Orphan reaping (critique S8): a step reruns `go test -race -count=1 -v -run 'TestOrphanGateContainerReaped|TestOrphansReapedByNextLaunch|TestReaperEscalatesToSigkill' ./runtime` and fails unless each name has a `--- PASS` line. Names are confirmed by grep at implementation.
  - The job also builds the native `CGO_ENABLED=0` binary and runs its `version` (critique M1), so each arch's shipped build is executed on its own runner.
- **build** job: for amd64 and arm64, `CGO_ENABLED=0 GOOS=linux GOARCH=<a> go build -trimpath -ldflags … -o dist-out/lsf-runtime-linux-<a> ./cli`. It checks `file` reports "statically linked" (fails otherwise), writes `SHA256SUMS`, and uploads them as a workflow artefact.
- **release** job, on tag `lsf-runtime-v*` only, never on `pull_request`, needs test and build:
  - It downloads the artefact and has `permissions: contents: write` and `GH_REPO`.
  - It runs `gh release create "$TAG" --verify-tag $PRERELEASE dist-out/*`, where `PRERELEASE` is computed in a shell step: `--prerelease` when the tag matches `*-rc*|*-alpha*|*-beta*`.
- **pin-drift** job: `dist/check-pin.sh` on main, PRs to main and `v*` tags only (above).
- Branch pushes are filtered by path. Tag pushes have no path filter, and the workflow running on the tag push is checked in step 7 (critique M2).
- Actions pinned by full commit SHA, with the version in a comment. The SHAs are looked up at implementation time from the actions' release pages (`gh api`), not recalled.

### CLI changes (`lsf-runtime/cli`)

- **`validate [-decisions PATH] PLAN`**: the same flags, output and exit codes (0 / 1 problems / 2 error) as `lsf-validate`. `run` moves from `validate/cmd/lsf-validate/main.go` into `validate.Command(name string, args []string, stdout, stderr io.Writer) int`. `cmd/lsf-validate` becomes a wrapper and its tests stay where they are (`validate/cmd` is outside Scope, a recorded deviation, and the move keeps one implementation). A CLI test runs a fixture through `validate`.
- **`version`**: prints `main.version main.commit` (defaults `dev`, `unknown`).
- **`config init [-config FILE]`**: writes the starter host config to `-config` or the default `${XDG_CONFIG_HOME:-~/.config}/lsf-runtime/config.json`. It creates the parent dir (0700) and opens with `O_CREATE|O_EXCL` (0600), so it **never overwrites**: an existing file exits 1 naming it. It prints the path. Contents:
  - `"image": "REQUIRED: build an image from <pluginDir>/lsf-runtime/container/Containerfile and put name@sha256:<digest> here"`;
  - `"proxy": {"listen": "127.0.0.1:0", "credentialsFile": …}`. `proxy` is required by `LoadConfig` (`runtime/config.go:197`), since it is a call's only route to the API. `credentialsFile` is `$HOME/.claude/.credentials.json` when that file exists, else a `REQUIRED:` marker;
  - `"env": []`;
  - no `caps` (requester, 2026-10-11: leave out, so no cap until the requester adds them);
  - `claudeBin` is left out (PATH default) and so is `pluginDir` (the skills pass `-plugin-dir`).
- `compose` refuses a host file with any string value starting `REQUIRED:` (`image`, `proxy.credentialsFile`), naming the file and the field, before the runtime's own checks. Test: `launch` on a fresh starter fails with that message.
- `config` is the CLI's first two-level command. Dispatch is `commands["config"]` taking `init` as its first arg, with usage listing `config init`. Its exit 1 for an existing file is a refusal, consistent with the CLI's exit-1 meaning.
- **`status`**: `leftovers.retain` (a new `[]string`, always present). For each `<state>/retained/*.json` record's `store`, glob `<store>/.lsf-retain-<h8>-*` and list what exists. This includes a staging or scratch dir whose record is `done` or names another dir, i.e. abandoned.
  - h8 comes from an exported helper `lsfrt.RetainPrefix(stateDir string) string`, which `retainDirs` also uses. It is computed from `launch.json`'s stored `stateDir`, the spelling the runtime used, not the CLI's address (critique M5).
  - The runtime writes a move's record before creating its staging dir (`retain.go:331`, then `:336`), so every staging or scratch dir the runtime creates has a record naming its store. A store is unsearched only when its record was deleted by hand. The design's Known limits says so (critique S3, scoped).
  - Unreadable records go to `errors`.
  - Status golden or shape tests are updated for the new key.
- Usage text and the `## CLI` Commands table gain the three commands.

### Skills

- **`phronithm:breakdown` quality check:** `${CLAUDE_PLUGIN_ROOT}/lsf-runtime/lsf-runtime validate -decisions … <plan>`. "Exit 1: each stdout line is a problem. Exit 2, 125 or any other non-zero: the command's own failure, so report its stderr." The "only where Go is not installed" fallback becomes "only where the launcher cannot run (exit 125, `lsf-runtime launcher:` on stderr), check by hand …". New cost, stated in Distribution: a breakdown user without a working tree needs network and curl at the first quality check.
- **`phronithm:lsf-drive` Startup:**
  - The route paragraph names the launcher (`${CLAUDE_PLUGIN_ROOT}/lsf-runtime/lsf-runtime`, written `lsf-runtime` below) and drops the `go build` block and the `go run` caveat.
  - When no host config exists, Startup offers `lsf-runtime config init` to the requester: the requester builds the image and fills in `image`, and the CLI route is then taken. Only on decline does it fall back to the Workflow route.
  - Step 2 runs `lsf-runtime validate` with the exit-code rule above.
  - Step 4's leftovers line covers `retain`.
- **`phronithm:lsf-drive`:** the rule is stated once, in Startup where the launcher is introduced: "exit 125 with `lsf-runtime launcher:` on stderr, from any command, is the launcher's failure, not the runtime's: report it verbatim" (critique S2). It covers `status`, `launch`, `watch`, `pause`, `stop` and `resume`. Launch's command lines already say `lsf-runtime`.

### Design doc (`lsf-runtime.md`)

- Written incrementally, per the design-of-record obligation (critique S7). Step 3 writes the launcher and release text, step 4 the CI text, and step 5 the prerequisites and image text. Step 7 adds the observed facts in its pin commit: redirects, the exec bit, the cache.
- Image digest (critique S6): `podman image inspect --format '{{.Digest}}'` on a locally built image is the method the live suite already ran against (`localhost/lsf-ref@sha256:949f2220…`, s2-sandbox). Step 8's dry run re-evidences it, and the section gives the exact `podman build` and `inspect` commands.
- The `### Distribution` subsection under Language is rewritten as the decision: D1 by requester verdict (2026-10-11) with D4's rejection reason, the launcher's resolution order and exit 2, the pin file and release order with the drift check, the cache location and why it is not `CLAUDE_PLUGIN_DATA`, and the prerequisites. The prerequisites are Linux amd64 or arm64, `sh`/`curl`/`sha256sum`, rootless podman with `pasta` (6.1.3 tested), git ≥ 2.38, and `claude` at a pinned version. The section says how to build an image from `container/Containerfile` extended with the project's toolchain and record `name@sha256:<digest>` as `image` (via `config init`), and what CI runs and skips: the live tests gated on `LSF_TEST_IMAGE`, listed by name.
- The Plan consequences line on distribution, and the `## CLI` intro sentence "Until `runtime-distribution` ships a launcher …", are updated (the CLI section is runtime-cli-drive's; a one-line design-of-record fix, recorded).
- `CLAUDE.md` Structure gains the launcher, `dist/` and `.github/workflows/lsf-runtime.yml`, and drops `validate/cmd` as the skills' entry point (Phase 5 doc update; outside Scope, recorded).

### Error handling summary

The launcher's failures are all exit 2 with a prefix, and it never execs an unverified file. `config init` never overwrites. `status` problems go to `errors` and never fail the report. The CI release runs only after test and build pass on the tag.

### Risks and unknowns

| Risk | Resolution | When |
|---|---|---|
| R35 timing flakes on 2–4 core runners | First CI run. Rerun once a failed job. A repeat is reported, not papered over | during (step 7) |
| A dependency of the stub-podman tests missing on the runner (procps, util-linux `setsid`) | First CI run. Add apt packages to the job | during |
| Action SHAs, `gh release` flags (`--verify-tag`) | Verify against `gh release create --help` and the actions' release pages | before writing step 4 |
| GitHub release download redirects (objects.githubusercontent.com) | `curl -L` follows them. Exercised on the rc release | step 7 |
| Plugin cache copy and the exec bit of `lsf-runtime/lsf-runtime` | Already shown for `tools/*.py`. Rechecked in the local-marketplace install | step 8 |
| No release pinned at plugin install before the first real release | The launcher refuses clearly. The rc pins are committed in this unit | step 7 |

## Implementation steps

Each step: tests green (`cd lsf-runtime && go test -race ./...`, plus `gofmt -l`), commit, then a critique gate (critique-code) spawned on the step's diff while the next step proceeds.

0. **Save and clean.** Save this plan as `docs/plans/runtime-distribution-plan.md`. `git rm --cached lsf-runtime/cli/cli`, and add `lsf-runtime/cli/cli` and `lsf-runtime/.build/` to `.gitignore`. Commit.
1. **`validate` subcommand.** Extract `validate.Command`, wrap it in `cmd/lsf-validate`, and add the CLI `validate`. Tests: the existing `cmd/lsf-validate` tests unchanged, plus a CLI test (problems → 1 with stdout, missing file → 2, ok → 0). Accept: `lsf-runtime validate docs/plans/lsf-improvements-breakdown.md` from a built binary exits 0.
2. **`version`, `config init`, the REQUIRED refusal, `status` retain leftovers.** Add `lsfrt.RetainPrefix` and use it in `retainDirs`. Tests: init writes 0600 with the marked image and the parent made 0700, a second init exits 1 leaving the file byte-identical, `launch` on a starter refuses naming `image`, status lists a planted `.lsf-retain-<h8>-u.partial` beside a record and not another state dir's prefix, `version` prints `dev unknown`.
3. **Launcher, pin file and launcher tests.** `lsf-runtime/lsf-runtime`, `dist/release` (unpinned), `dist/pin.sh`, `dist/doc.go`, `dist/launcher_test.go`. Tests run the script with a temp root:
   - `LSF_RUNTIME_BIN` wins;
   - a `.git` root builds and runs `version` → `dev` (skipped under `-short`; uses the real module via a symlinked root);
   - a release root with a `file://` URL, a matching pin, and a stub binary that echoes its args → runs, args passed through;
   - a wrong pin → exit 125, file removed, the binary not run;
   - a cached file tampered after download → exit 125 with "remove … and rerun";
   - no pin → exit 125 with the message;
   - an unknown `uname -m` (stubbed via a `PATH` shim) → exit 125;
   - a `.git` root whose build fails (a planted syntax error in a copied module) → exit 125 with compiler output, no fall-through;
   - a `.git` root with no `go` on `PATH` → falls through to the release with the stderr note;
   - an unchanged rebuild leaves the inode of `.build/lsf-runtime` unchanged;
   - `validate` through the launcher → the CLI's exit codes preserved (1 and 2).
   - The step also writes Distribution's launcher and release text.
4. **CI workflow.** `.github/workflows/lsf-runtime.yml` as designed. Check locally that the YAML parses (`python3 -c 'import yaml…'` if available, else `gh workflow view` after the push) and that the build commands work locally for both arches (`file` says static).
5. **Skills.** breakdown quality-check and lsf-drive Startup/Launch, as designed. Accept: grep shows no `go -C`, `go run` or `go build` left in either skill. Run the quality check's command on the breakdown plan through the launcher (exit 0).
6. **Design doc and CLAUDE.md.** Finish the Distribution section (decision record, the D4 rejection, prerequisites, image and digest commands, CI's skipped live tests), the CLI intro line, Plan consequences, the CLI Commands table, the status Known-limits line and CLAUDE.md Structure. Steps 3–5 have already written their parts.
7. **CI and release on GitHub** (confirm with the requester immediately before each outward action).
   - Push `lsf-workflow` and watch the run (`gh run watch`). Fix and repush until green, rerunning once for a suspected R35 flake.
   - Tag `lsf-runtime-v0.0.1-rc1` at a green commit, push the tag, and watch the release job.
   - `dist/pin.sh lsf-runtime-v0.0.1-rc1`, then commit.
   - Run the launcher from a `git archive HEAD` export (no `.git`) with an empty `LSF_RUNTIME_CACHE`: it downloads, verifies and runs `version` → the rc tag.
   - A tampered cache file is refused.
8. **Local-marketplace install (SC13; needs step 7's pin).** It exercises the release path, not the git-source `.git` path (the working-tree path is covered by step 3's tests and this repo's own in-place marketplace). Export `git archive HEAD` to the scratchpad as a directory marketplace. With `CLAUDE_CONFIG_DIR` pointed at a scratch dir (the user's settings stay untouched), `claude plugin marketplace add <dir>` and `claude plugin install phronithm@phronithm`. Then run the installed copy's launcher: if the plugin loads in place (`.git`-less export), `version` (release path), `validate` on the breakdown plan, `status -name x` (exit 4), and `launch -dry-run` on the shakedown or fixturerun plan with a host config naming the test image `localhost/lsf-ref@sha256:949f2220…`. Record which path ran, and whether exec bits survived.
9. **Implementation summary** at `docs/plans/runtime-distribution-implementation.md`, then Phase 5.
10. **Final re-pin** (critique S1), after Phase 5's last `lsf-runtime/` change. Tag `lsf-runtime-v0.0.1-rc2` (confirm first), let CI publish it, run `dist/pin.sh`, check `dist/check-pin.sh` passes at HEAD, and commit. Skipped if no `lsf-runtime/` file changed since rc1.

`shakedown-fixture` (critique M6): `tools/fixturerun` runs in-tree by `go run`. A shakedown through `phronithm:lsf-drive` takes the launcher's working-tree path, so it runs the tree. No step is needed beyond step 3's test.

Order: 0 → 1 → 2 → 3 (3's `validate` test needs 1) → 4 → 5 → 6 → 7 → 8 → 9 → Phase 5 → 10. Steps 4–6 are independent of each other once 3 is done.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/runtime-distribution-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit runtime-distribution of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim (Phase 4 re-verifies the external gates against these
pins before implementing, recording the outcome in the implementation-summary):
see "Unit entry (verbatim, plan at c7bf8d2)" above; standing obligations: Register dispositions (no rows), Design of record.
Gate pins to re-verify: github-actions (repo public, Actions usable after push); lsf-toolchain Go 1.27.1 (`go version`, go.mod toolchain); git ≥ 2.38 on CI runners (CI step) and locally 2.54.0; podman 6.1.3 rootless — not applicable on CI (live tests skip by decision), locally confirmed for step 8; requester-verdict — D1, given 2026-10-11.
After Phase 5, produce the completion-report (contents per the
phronithm:feature skill's Produces) at docs/plans/runtime-distribution-completion.md,
then present this continuation prompt to the requester:
Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
