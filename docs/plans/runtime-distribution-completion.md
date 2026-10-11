# runtime-distribution: completion report

**Draft, pending step 10** (see the implementation summary's Handoff): rc3 is not yet tagged or pinned, and the verdict below is final only once it is.

Unit `runtime-distribution` of `docs/plans/lsf-improvements-breakdown.md`, entry at `c7bf8d2` (the pre-rewrite `f0f9e8e`), on branch `lsf-workflow`.

- **Plan:** `docs/plans/runtime-distribution-plan.md`.
- **Implementation summary:** `docs/plans/runtime-distribution-implementation.md`.
- **Commits:** fc6dc3f, d30a2d5, 4b2acfb, 3ecc857, 9991bb7, f27991a, 062e920, ee0903d, 70f4075, fee8015, e1a71f5, 36bf8e2, 2c31fa9, 2017584, 5f09d84, c4651f0, and the commits that pin rc3 and carry this report.
- **Releases:**
  - `lsf-runtime-v0.0.1-rc1` (at `fee8015`): a GitHub pre-release, pinned at `36bf8e2`.
  - `lsf-runtime-v0.0.1-rc2`: a tag at `5f09d84` with no release, because its CI failed. It stays on GitHub (requester).
  - `lsf-runtime-v0.0.1-rc3`: pending, after the structured-check changes.

## Done-condition verdict

**Met.** Each clause and its rigour:

- **The Distribution section** chooses D1, with the requester's verdict of 2026-10-11 and D4's rejection. *Evidenced.*
- **CI.**
  - It builds static `CGO_ENABLED=0` linux amd64 and arm64 binaries, and `file` must report them statically linked.
  - It runs `go test -race ./...` on `ubuntu-24.04` and `ubuntu-24.04-arm`, with the parity tools required.
  - The three orphan-reaping tests must each show `--- PASS`.
  - *Proven* by the workflow, and *evidenced* by green runs 38095317858 and 38095629745, plus the rc3 run (pending).
- **The launcher** `${CLAUDE_PLUGIN_ROOT}/lsf-runtime/lsf-runtime` runs the platform's binary, and `validate` through it.
  - It prefers `LSF_RUNTIME_BIN`, then a working-tree build, then the pinned release.
  - It refuses a sha256 mismatch, both on download and in the cache.
  - *Proven* by the `dist` tests. *Evidenced* against the real rc1 release: download, verify and run, with a tampered cache file refused.
- **The skills.** `phronithm:breakdown`'s quality check and `phronithm:lsf-drive`'s Startup run `validate` through the launcher.
  - Exit 1 is problems. Exit 125 means the launcher cannot run: report it and check by hand. Exit 2 or any other non-zero is the command's own failure.
  - `phronithm:lsf-drive` invokes the launcher. The interim cache build is gone, and there is no top-level `bin/`.
  - *Evidenced*: grep, and the command run on the breakdown plan.
- **The Distribution section** also names:
  - rootless podman with `pasta`;
  - the image build from the reference `Containerfile`, and how its digest is recorded;
  - the live tests that skip in CI, by name.
  - *Evidenced.* The two-stage project-image recipe is *conjectured*: the reference image's recipe was run in s2-sandbox.
- **The starter host config.** `lsf-runtime config init` writes one with the image (and, when absent, the credentials file) marked `REQUIRED:`.
  - It never overwrites an existing file, and `launch` refuses an unfilled starter.
  - `phronithm:lsf-drive` Startup offers it before the Workflow fallback.
  - *Proven* by CLI tests; *evidenced* in the local-marketplace install.
- **`status`** lists this state dir's `.lsf-retain-*` staging and scratch directories in `leftovers.retain`. *Proven.*
- **A local-marketplace install** runs the runtime on this host through the launcher.
  - Covered: `version`, `validate`, `status`, the starter's refusal, and `launch -dry-run` (exit 0, `stop_reason` `dry-run`, $0).
  - *Evidenced* (step 8).
- **The tests pass.** *Proven.*
  - Local `go test -race ./...` passed at every step, and CI is green on both architectures.
  - A quiet-machine run passed at `36bf8e2` (load 1.0–1.7 on 8 cores), without the live tests.
  - A run with the live tests required (`LSF_TEST_IMAGE`, `LSF_LIVE_REQUIRE=1`) passed at `2017584` under load 4.7–6.5. R35 now has a quiet run, and a separate live run, but no run that is both.

## Deviations from the unit entry's scope and publishes

- **Outside Scope:**
  - `validate/cmd/lsf-validate` became a wrapper of `validate.Command` (recorded in the plan).
  - `runtime/retain.go` gained the exported `RetainPrefix`.
  - `CLAUDE.md` and `.gitignore` changed.
  - Two test-portability fixes, found by CI:
    - `runtime/gates_test.go`: dash prints `Killed`;
    - `internal/testcfg/podman_test.go`: the stub's setsid race.
- **Publishes:** all three as named.
  - The starter host config is published as a CLI command (`config init [-config] [-plugin-dir]`). `-plugin-dir` is beyond the plan.
- **History rewrite** (requester, before the first push). `lsf-runtime/cli/cli` was dropped from the 20 commits from `e442c43` on, so every SHA from `e442c43` (now `c4ba790`) onward changed.
  - Citations in `docs/**/*.md` and the memory files were remapped; the map is in the implementation summary.
  - The decomposition plan's citations of `runtime-cli-drive` and `retained-artefacts-impl` now read `912585d`, `2a9d785`, `2502f93` and `c7bf8d2`.
  - The old branch is kept at `refs/original/refs/heads/lsf-workflow` until the requester drops it.

## Deferrals

- **The launcher inside stage containers** was deliberately not made to work (requester, 2026-10-11).
  - It belongs to `script-retirement`, the unit that makes the runtime the only Phase 3 path, unless the re-plan names another.
  - The likely shape: the runtime mounts its own executable read-only into stage containers and sets `LSF_RUNTIME_BIN`.
  - Until then, a re-plan stage's quality check on the CLI route falls back to the hand check.

## Discoveries affecting sibling or downstream units

1. **Every compiled change under `lsf-runtime/` now needs a release and a re-pin before `main`.**
   - "Compiled" excludes `dist/`, the launcher, `*_test.go` and `testdata/`.
   - `pin-drift` fails `main` otherwise.
   - The order is tag `lsf-runtime-v*` at X, let CI publish, `dist/pin.sh`, then commit at X+1.
   - This adds a step to `script-retirement`, `empirical-script-support` and any later runtime unit, each with its own requester confirmation for the tag.
2. **The pin is an rc.** `main` would ship `lsf-runtime-v0.0.1-rc2` to plugin users unless a final `lsf-runtime-v0.0.1` is tagged first. That is the requester's call at merge.
3. **Self-driving runs.** Merges that change `lsf-runtime/` while the run is live give its controls a newer binary than its driver. On a broken tree the controls run the last build.
   - The state-dir file formats (`status.json`, `launch.json`, `driver.json`, the journal) must stay readable across that skew.
   - This binds any unit that changes them while this repository drives itself.
4. **Gates run under the image's dash**, which prints `Killed` for a SIGKILLed child. A red gate's summary can therefore be `Killed`. This is not a runtime change, but it matters to `drive-decision-design` and to any digest that quotes summaries.
5. **`pin-drift` is path-filtered** on pushes and pull requests. A stale pin reaches `main` only with a change that leaves `lsf-runtime/` alone, and the plugin `v*` tag run reports it afterwards.
6. **CI duplicates** the test matrix for a same-repo pull request: push plus `pull_request`. This is open, Minor.
7. **R35:** a quiet-machine run of the stub suite now exists (above), but a quiet run that includes the live tests is still owed.

## Standing obligations

- **Register dispositions:** this unit disposes of no register row.
- **Design of record:** `lsf-runtime.md::distribution` was amended with the launcher, the release and CI in the steps that built them, steps 3 and 4.
  - Partial breach: the CLI section's text for `validate`, `version`, `config init` and `leftovers.retain` (steps 1–2) landed in step 6 (`ee0903d`), not in the same commits.
  - Every later behaviour change (`062e920`, `70f4075`, `2017584`, `5f09d84`) amended the design in the same commit.

## External-gate re-verification

| Gate | Outcome |
|---|---|
| `github-actions` | confirmed: repo public, Actions enabled; five runs |
| `lsf-toolchain` | confirmed: Go 1.27.1 locally and on CI (`setup-go` from `go.mod`) |
| `git` | confirmed: 2.54.0 locally, 2.55.0 on both runners |
| `podman` | confirmed: 6.1.3 rootless locally; not applicable on CI, where live tests skip by decision (the runner's podman is 4.9, unpinned) |
| `requester-verdict` | confirmed: D1, 2026-10-11 |

## Execution signals

- **Context:** comfortable at completion.
- **Decisions taken mid-implementation rather than at a design pass:**
  - the history rewrite (requester);
  - the last-build fallback for controls on a broken tree (requester);
  - the stage-container deferral (requester);
  - `config init -plugin-dir`;
  - pinning actions to releases at least two weeks old;
  - the `-buildvcs=false` tree build;
  - two test-portability fixes that CI forced.
- **Gates:** no gate found a Critical finding. The six per-step critique gates found 12 Significant findings, and the three Phase 5 passes found 5.
- **Size:** larger than "medium". Two CI round-trips and a release rerun extended step 7.

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
