# runtime-distribution: implementation summary

- **Branch:** `lsf-workflow` (pushed to `origin`, 2026-10-11).
- **Plan:** `docs/plans/runtime-distribution-plan.md`.
- **Work unit:** `runtime-distribution` of `docs/plans/lsf-improvements-breakdown.md`, entry at `c7bf8d2` (the plan cites `f0f9e8e`, its SHA before the history rewrite below).

## External gates (Phase 4 entry)

| Gate | Pin | Probed | Outcome |
|---|---|---|---|
| `github-actions` | repo public, Actions usable | `gh repo view`: public; Actions enabled, all actions allowed; three runs on `lsf-workflow` | confirmed |
| `lsf-toolchain` | Go 1.27.1 | `go version` 1.27.1; `go.mod` toolchain go1.27.1; `setup-go` from `go.mod` on CI | confirmed |
| `git` | ≥ 2.38 on CI runners | local 2.54.0; CI 2.55.0 on both runners (checked by a CI step) | confirmed |
| `podman` | 6.1.3 rootless | local 6.1.3; on CI not applicable (live tests skip, requester decision) | confirmed |
| `requester-verdict` | D1 | given 2026-10-11, before implementation | confirmed |

## Files modified

- `.github/workflows/lsf-runtime.yml` (new)
- `.gitignore`
- `CLAUDE.md`
- `docs/design/lsf-runtime.md`
- `docs/plans/runtime-distribution-plan.md` (new), `docs/plans/runtime-distribution-implementation.md` (new)
- `lsf-runtime/lsf-runtime` (new, the launcher)
- `lsf-runtime/dist/{release,pin.sh,check-pin.sh,doc.go,launcher_test.go}` (new)
- `lsf-runtime/cli/{validate.go,validate_test.go,version.go,configinit.go,configinit_test.go}` (new), `lsf-runtime/cli/{config.go,status.go}`
- `lsf-runtime/validate/command.go` (new), `lsf-runtime/validate/validate.go`, `lsf-runtime/validate/cmd/lsf-validate/main.go`
- `lsf-runtime/runtime/retain.go`
- `lsf-runtime/runtime/gates_test.go`, `lsf-runtime/internal/testcfg/podman_test.go` (test portability fixes, outside Scope)
- `skills/breakdown/SKILL.md`, `skills/lsf-drive/SKILL.md`
- `lsf-runtime/cli/cli` (removed from history, below)
- Outside the repo: the memory files citing rewritten SHAs.

## History rewrite (requester, 2026-10-11)

Before the first push, the requester chose to rewrite history so that the 13.7 MB `lsf-runtime/cli/cli` binary would not be published. `git filter-branch` dropped it from the 20 commits from `e442c43` (now `c4ba790`) on to the then HEAD. A tree filter remapped every SHA of those commits that `docs/**/*.md` cites to its new SHA, at the same length. The verification:
- every changed line differs only in a SHA;
- no ref from `main..lsf-workflow` reaches the blob;
- no old SHA is cited.

No other branch or tag held the commits, and the `desktop` remote has no `lsf-workflow`. The memory files citing `f0f9e8e`, `3348829`, `8208ccf` and `fa16e31` were remapped. The pre-rewrite branch is kept at `refs/original/refs/heads/lsf-workflow` until the requester drops it.

| Old | New | Commit |
|---|---|---|
| a38a464 | fc6dc3f | step 0 |
| b624a20 | d30a2d5 | step 1 |
| 7cf6482 | 4b2acfb | step 2 |
| de4a8b8 | 3ecc857 | step 3 |
| ea88ad6 | 9991bb7 | step 4 |
| 061b235 | f27991a | step 5 |
| 8ca1cb7 | 062e920 | steps 3–5 critique fixes |
| ffd0356 | ee0903d | step 6 |
| 2faf161 | 70f4075 | step 6 critique fixes |

## Per-step record

### Step 0 (`fc6dc3f`)
- Saved the plan, untracked `lsf-runtime/cli/cli`, and gitignored it with `lsf-runtime/.build/`.

### Step 1 (`d30a2d5`): `validate`
- `validate.Command(name, args, stdout, stderr)` holds `lsf-validate`'s former `run`. The command name appears in usage and error prefixes.
- The `cmd/lsf-validate` tests are unchanged and pass. The CLI test covers 0, 1, 2 and usage.
- Accept: a built binary's `validate docs/plans/lsf-improvements-breakdown.md` exits 0.

### Step 2 (`4b2acfb`): `version`, `config init`, the REQUIRED refusal, `status` retain leftovers
- **Deviation:** `config init` also takes `-plugin-dir DIR`, which names the Containerfile path in the image instruction. Step 5's critique then added `-config` to the skill's command line. Both are recorded in the CLI Commands table.
- `lsfrt.RetainPrefix` is shared by `retainDirs` and `status`.
- `status` hashes `launch.json`'s `stateDir` spelling. A test proves the CLI-address spelling differs, through a symlink.

### Step 3 (`3ecc857`, fixes in `062e920`, `70f4075`): launcher, pin file, launcher tests
- **Decision:** the working-tree build uses `-buildvcs=false` and `GOFLAGS=''`, so an unchanged tree builds a byte-identical binary and the `cmp` check holds.
  - An invocation costs about 0.5 s with a warm build cache.
- **Additions beyond the plan, all from the critique:**
  - `curl -q` with connect, total-time and size limits;
  - a 0700 cache;
  - pin validation (64 lowercase hex characters; the first match wins);
  - `LSF_RUNTIME_BIN` must contain a `/`;
  - a HUP trap;
  - an unset `HOME` exits 125;
  - a non-Linux `uname -s` exits 125;
  - `pin.sh` checks the binaries against `SHA256SUMS` and writes the pin atomically;
  - `check-pin.sh` tells a missing commit from drift.
- **Surprise:** a `go` binary found in the same directory as the other tools (`/usr/bin`) meant the no-go test needed a `PATH` of symlinks to the tools it uses.

### Step 4 (`9991bb7`, fixes in `062e920`): CI
- **Decision:** actions are pinned by SHA to releases at least two weeks old:
  - checkout v7.0.1 and setup-go v7.0.0;
  - upload-artifact v7.0.1 and download-artifact v8.0.1, not their 2026-10-07 releases.
- **Additions beyond the plan:**
  - a `v*` tag trigger for `pin-drift`;
  - a per-ref concurrency group;
  - `--latest=false` on releases;
  - a pre-release for any `-` suffix.

### Step 5 (`f27991a`, fixes in `062e920`): skills
- Accept: no `go -C`, `go run` or `go build` remains in either skill (grep exit 1), and the quality check's command exits 0 on the breakdown plan.
- The critique's three Significant findings are fixed:
  - exit 125 has one rule: report stderr, then check by hand;
  - a starter still holding a `REQUIRED:` value counts as no config;
  - `retain` entries are never removed while a run is live.

### Step 6 (`ee0903d`, fixes in `70f4075`, observed facts in `36bf8e2`): design of record and `CLAUDE.md`
- **Deviation (recorded in the plan):** `CLAUDE.md` and the CLI intro line are outside Scope.
- The Distribution section states that the pin is committed before the branch merges to `main` or a plugin tag is made.

### Step 7: CI and release on GitHub
- **Push 1** (`70f4075`), run 38094835906: green on both runners except `TestRunGatesStatuses`.
  - Ubuntu's `/bin/sh` is dash, which prints `Killed` for a SIGKILLed child. Bash, this host's `/bin/sh`, prints nothing with its output redirected.
  - The test now accepts either (`fee8015`).
  - **Deviation:** a test outside Scope. The Debian reference image's `/bin/sh` is also dash.
- **Push 2** (`fee8015`), run 38095317858: green.
  - Each of the three orphan-reaping tests showed `--- PASS` on both runners.
  - Both binaries are statically linked, and the runners have git 2.55.0.
  - node is on the arm image: the parity tools passed with `*_REQUIRE_NODE=1`.
- **Tag** `lsf-runtime-v0.0.1-rc1` at `fee8015`, run 38095629745: the arm test job failed `TestStubDetached`.
  - The stub records the detached command's pid as soon as `setsid` is forked, so the test's SIGTERM to the client's group could reach it before it left the group.
  - Fixed in `e1a71f5` (outside Scope, `internal/testcfg`): the test waits until the command leads its session. It passed 30 of 30 under `-race` locally.
  - The tag run's failed job was rerun once (the plan's R35 rule; the fix is in a test harness, not the shipped code) and passed. The release published as a pre-release with both binaries and `SHA256SUMS`.
  - The tag run had no path filter applied.
- **Pin** (`36bf8e2`): `dist/pin.sh` verified both binaries against `SHA256SUMS`.
  - From a `git archive` export with an empty cache: the launcher downloaded from GitHub (following a redirect to another https host), verified and ran the binary. `version` printed `lsf-runtime-v0.0.1-rc1 fee8015…` and `validate` exited 0, in 2.5 s for the first run.
  - A tampered cache file was refused with exit 125.
- `check-pin.sh` failed at `36bf8e2`, because `e1a71f5` changed `lsf-runtime/` after the tag. Step 10 re-pinned.

### Step 8: local-marketplace install
- `git archive HEAD` was exported to a scratch directory and added as a directory marketplace with `CLAUDE_CONFIG_DIR` pointed at a scratch dir. `claude plugin install phronithm@phronithm` installed version 0.3.1 into the scratch config's plugin cache as a copy.
- **The copy:** no `.git`; the launcher, `dist/*.sh` and `tools/*.py` are 0755. The release path ran.
- **Through the installed launcher with a fresh cache:**
  - `version` printed the rc1 tag (exit 0);
  - `validate` on the breakdown plan exited 0;
  - `status -name x` exited 4.
- **The starter config:** `config init -config … -plugin-dir <installed root>` wrote it, with `proxy.credentialsFile` filled from `~/.claude/.credentials.json`.
  - `launch -dry-run` on the spike fixture repo was refused, naming `image`.
  - With `image` set to `localhost/lsf-ref@sha256:949f2220…`, `launch -dry-run` exited 0 with `stop_reason` `dry-run`, 0 calls and $0.
- **Not run:** the two-stage image build in Distribution (a project image `FROM` the reference image by digest). The reference image's digest recipe is the one s2-sandbox used. The second stage is conjectured (step 6 critique M11).

### Step 10 (`4b4b069`, `75e3ada`): structured checks and the rc3 pin
- Structured checks (requester's scope: rows 1–3 of the Phase 5 proposal) in `4b4b069`: `TestRunGatesStatuses` asserts status and code only for the killed child, plus an `exit 4` case; CI's orphan-reaping step requires `go test -json` pass events; `dist`'s `TestStaticBuild` checks the ELF headers (it reported both faults against `/bin/ls`). Branch run 38099462205 was green, with both new checks seen in its logs.
- `lsf-runtime-v0.0.1-rc3` tagged at `4b4b069`; tag run 38100199084 published the pre-release. `dist/pin.sh` verified both binaries; `check-pin.sh` passes at `75e3ada`.
- From a `git archive` export with an empty cache, the launcher downloaded, verified and ran rc3 (`version` printed `lsf-runtime-v0.0.1-rc3 4b4b069…`), `validate` exited 0, and a tampered cache file was refused with exit 125.

## Per-step critique findings

All gates ran at sonnet, round 1. None had a Critical finding, so all passed.
- **Fixed before exit** means fixed in the commits named.
- **Phase 5** means noted for Phase 5.

| Step | Severity | Finding | Disposition |
|---|---|---|---|
| 1 | Minor | CLI test lacks the sibling-default and unknown-flag cases | Phase 5 (`cmd/lsf-validate` covers them) |
| 1 | Minor | `-h` exits 2 | no change: unchanged from `lsf-validate` |
| 1 | Minor | `validate` owns flag and I/O handling; bypasses the CLI's `fail` | no change: the plan's single implementation |
| 1 | Minor | cryptic stdout assertion | Phase 5 |
| 2 | Minor | `REQUIRED:` matched with a trailing space | fixed (`3ecc857`) |
| 2 | Minor | stores deduplicated on the raw string | fixed (Clean), tested |
| 2 | Minor | `Glob` error discarded | fixed for retain (ReadDir); the older `*.partial` globs share it, Phase 5 |
| 2 | Minor | a live run's staging dirs listed as leftovers | fixed in skill text (step 5) |
| 2 | Minor | test gaps: image-only starter, nested `REQUIRED:` | Phase 5 (missing store added) |
| 2 | Minor | `-plugin-dir` beyond the plan | recorded (deviation) |
| 3 | Significant | unset `HOME` escapes exit 125 | fixed, tested |
| 3 | Significant | changed-tree rebuild untested | fixed (`TestWorkingTreeRebuilds`) |
| 3 | Significant | download failure, default URL and cache defaults untested | fixed (three tests); `shasum` fallback, missing `curl` and signal cleanup remain untested, Phase 5 |
| 3 | Minor ×13 | curl hardening, cache TOCTOU, non-path `LSF_RUNTIME_BIN`, exec failure codes, HUP, `go -C` floor, pin validation, `pin.sh` atomicity and trust, `check-pin` conflation, test robustness, stale D1 text, `$0` symlinks | fixed, except: the TOCTOU (mitigated by 0700, documented); exec failure 126/127 (documented); stale temp files are not swept, Phase 5 |
| 4 | Significant | doc wrong for `v*` tags; the release tag is not checked to be on `main` | doc fixed; not enforced, stated |
| 4 | Significant | orphan rerun doubles timing-sensitive tests | kept (plan S8) |
| 4 | Minor | duplicate runs; pre-release and latest; `check-pin`; trigger and arm-node unknowns; no Dependabot | fixed, or observed in step 7; no Dependabot, Phase 5 |
| 5 | Significant ×3 | contradictory 125 rule; no `retain` remedy; unfilled starter takes the CLI route | fixed (`062e920`) |
| 5 | Minor ×4 | `-config`; full path; first-use download; step 2 density | fixed |
| 6 | Significant | unpinned bootstrap fails `pin-drift` and blocks users | ordering stated; `pin-drift` fails on an unpinned tree by design |
| 6 | Significant | D4 rationale ignored D1's own `breakdown` coupling and exec bit | reworded with evidence |
| 6 | Significant | no OS check (macOS would exit 126) | fixed, tested |
| 6 | Significant | `.git` in the cache asserted, not observed | scoped to the observation; step 8 confirmed it |
| 6 | Minor | `pin.sh` takes the commit from the local tag, not the binary's embedded commit | Phase 5 |
| 6 | Minor | a pre-existing cache dir's mode is not checked | Phase 5 |
| 6 | Minor | podman minimum version, and whether 4.9 is supported | Phase 5 (unknown) |
| 6 | Minor | two-stage image recipe unevidenced | open (step 8 note) |
| 6 | Minor | any `lsf-runtime/` change forces a release before `main` | Phase 5 |
| 6 | Minor | long `CLAUDE.md` bullet | Phase 5 |
| 6 | Minor | stale text, fixture claim, prerequisite utilities, `proxy.listen` | fixed |

## Success criteria

| # | Criterion | Target | Achieved | Status | Evidence |
|---|---|---|---|---|---|
| SC1 | Distribution section commits D1 with the verdict | evidenced | evidenced | pass | `docs/design/lsf-runtime.md` Distribution, "Decision: D1 (requester verdict, 2026-10-11)" |
| SC2 | CI cross-builds static amd64 and arm64 | proven + evidenced | proven + evidenced | pass | `build` job's `file … statically linked` check; runs 38095317858 and 38095629745 |
| SC3 | CI runs `go test -race` on Linux, orphan reaping included | proven + evidenced | proven + evidenced | pass | `test` matrix on both runners; the three orphan tests showed `--- PASS` |
| SC4 | launcher runs the binary, and `validate` through it | proven | proven | pass | `dist` tests: `TestReleaseDownload`, `TestWorkingTree` (`validate` 1 and 2 through the launcher) |
| SC5 | skills run `validate` through the launcher, exit 1 vs 2/125 | evidenced | evidenced | pass | quality-check command exited 0 on the breakdown plan (working tree and installed copy); text critiqued in step 5 |
| SC6 | `LSF_RUNTIME_BIN`, then a working tree, before a release | proven | proven | pass | `TestBinOverride`, `TestWorkingTree`, `TestWorkingTreeBuildFails`, `TestWorkingTreeWithoutGo` |
| SC7 | refuses a sha256 mismatch | proven | proven + evidenced | pass | `TestWrongPinRefused`, `TestTamperedCacheRefused`; a tampered real release refused (step 7) |
| SC8 | Distribution names podman with `pasta`, the image build and the digest | evidenced | evidenced (reference image) / conjectured (project-image stage) | pass with gap | upgrade path: run the two-stage build once on a project image and record its digest |
| SC9 | Distribution names the live tests CI skips | evidenced | evidenced | pass | 13 `TestLive*` names plus `TestMoveRetainedCrossFilesystem` |
| SC10 | lsf-drive uses the launcher, the interim build is gone, no top-level `bin/` | evidenced | evidenced | pass | grep for `go -C\|go run\|go build` in both skills found nothing; no `bin/` |
| SC11 | `config init` writes a marked starter, never overwrites, offered before the Workflow fallback | proven + evidenced | proven + evidenced | pass | `TestConfigInit`, `TestLaunchRefusesStarter`; step 8 install refusal; lsf-drive Startup text |
| SC12 | `status` lists this state dir's `.lsf-retain-*` | proven | proven | pass | `TestStatusRetainLeftovers` |
| SC13 | an install from a local marketplace runs the runtime here | evidenced | evidenced | pass | step 8 |
| SC14 | the `lsf-runtime` tests pass | proven | proven | pass | local `go test -race ./...` green at each step; CI green on both runners; quiet-machine run below |

**Quiet-machine run (R35):** `go test -race -count=1 ./...` at `36bf8e2`, 2026-10-11 12:50–12:54 NZDT: every package passed (13 `ok` lines, exit 0). The load average was 1.01 at the start and 1.71 at the end, on 8 cores; Phase 5's read-only subagents started during the run.

## Phase 5 (review)

The multi-lens review and the full-diff critique ran in sonnet subagents, followed by a round-2 critique of the fixes. No round had a Critical finding.

- **Requester decisions (2026-10-11).** Two points were raised by both reviewers or by the critique:
  - **Live-run rebuild.** From a working tree that no longer builds, `status`, `watch`, `pause` and `stop` run the last `.build/` binary with a warning; other commands still exit 125. The design states the version skew between a self-modifying run's controls and its driver. Fixed in `2017584`.
  - **Launcher in stage containers.** The launcher cannot run inside a stage container (read-only plugin mount, no GitHub egress), so a re-plan stage's quality check falls back to the hand check, as under `go run` before. This is documented and deferred.
- **Fixed in `2017584`:**
  - A missing utility exited 127, and the EXIT trap overwrote the status. Tools are now checked first and the trap keeps the status.
  - A cache directory another user owns or can write is refused.
  - `check-pin.sh` takes the first `commit` line, requires hex, and ignores tests and testdata.
  - `pin.sh` refuses a binary whose embedded commit is not the local tag's.
  - `config init` with no usable `HOME` wrote a junk path under the cwd; it now refuses.
  - In `lsf-drive`, the leftovers are a list and `resume` passes `-plugin-dir`.
  - The design records the `.git` evidence from the real install, the path-filtered `pin-drift`, and the launch rebuild race.
- **Review S1, the `.git` premise:** re-evidenced, no code change.
  - This plugin's real 0.3.1 install comes from a directory marketplace whose source is this git working tree, and its cache copy has no `.git`.
  - The GitHub-sourced official marketplace checkout has a `.gcs-sha`, not a `.git`.
- **Round 2** (`5f09d84`): `check-pin`'s `**` exclusions were plain pathspecs and missed the top-level `testdata/`; they now use glob magic, verified. The fallback test now asserts that `launch`, `resume`, `validate`, `config` and `version` are refused.
- **Left open (Minor):**
  - Duplicate push and pull-request CI runs on a same-repo pull request.
  - `private()` does not check the cache's parent.
  - The shell defers a trap until `go build` or `curl` exits.
  - `pin.sh` checks only the host architecture's embedded commit.
  - The older `status` globs discard errors.
  - No Dependabot for the action pins.
  - Podman's minimum version is unknown.
  - The two-stage image recipe has not been run.
- **Live suite:** `LSF_TEST_IMAGE=localhost/lsf-ref@sha256:949f2220… LSF_LIVE_REQUIRE=1 PROMPTPARITY_REQUIRE_NODE=1 GRAPHPARITY_REQUIRE_NODE=1 go test -race -count=1 ./...` at `2017584` passed every package, 13:18–13:22 NZDT. The load average was 4.7–6.5, so this is not a quiet run.

## Deferrals and discoveries (for the completion report)

- `TestRunGatesStatuses` assumed bash as `/bin/sh`. Real gates run under the image's dash, which prints `Killed`, so a gate whose child is SIGKILLed has the summary `Killed`, not an empty one. No runtime change is needed.
- Stub-podman detach race (`TestStubDetached`): fixed in the test. The real runtime's detach goes through podman, not this stub.
- Every change under `lsf-runtime/` other than `dist/` and the launcher needs a release and a re-pin before `main`. That is a process cost for every later runtime unit (`script-retirement`, `empirical-script-support`).
