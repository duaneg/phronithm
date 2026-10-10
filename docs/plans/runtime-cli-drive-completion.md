# runtime-cli-drive: completion report

Unit `runtime-cli-drive` of `docs/plans/lsf-improvements-breakdown.md`, on branch `lsf-workflow`, base b56f427.

- **Plan:** `docs/plans/runtime-cli-drive-plan.md`.
- **Implementation summary:** `docs/plans/runtime-cli-drive-implementation.md`.
- **Commits:** 50414f3, c0ee2f1, ff2ec5b, 41f8a2e, d6506f1, b291d38, 5c3be0f, c4ba790, and the commit that carries this report.

## Done-condition verdict

**Met.** Each clause is proven by a test or evidenced as the plan targeted, with one exception: the quiet-machine condition on the suite is not met (last bullet).

- **`phronithm:lsf-drive` launches, pauses, resumes and reads status through the CLI.** *Evidenced* by a paid fixture run that followed the skill's commands:
  - `pause`, then `launch`, which returned `paused` with 0 calls;
  - `status`, then `pause -lift`;
  - a relaunch with `launch -args` from the outcome's `planSha`, which returned `complete` (8 calls, $1.02);
  - a background `watch`;
  - `resume`, which refused after the returned outcome.

  `resume`'s replay of a run with no outcome is proven by `TestResume`, whose journal replays the completed call.
- **Under the decided permission configuration.** The run used the container boundary: image `localhost/lsf-fixture@sha256:344c825a…`, the credential proxy, and no leftover containers.
- **P-F1.** *Evidenced.* The run was launched in the turn in which the requester typed "Go ahead". None of its 8 call transcripts holds that text (case-insensitive) or `Workflow harness`, and a positive control shows the transcripts are searchable. Detail: implementation summary, Success criteria → P-F1.
- **The driver outlives the launching session.** *Proven* by `TestLaunchDetachedCrash` (d): the driver is in a session of its own (`Setsid`) and runs on after `launch` exits. The skill runs `watch` as a background Bash; a later session runs `status`, then `watch` again.
- **Relaunch section.**
  - `gate-not-run` keeps its route, whether a stage or the runtime's gate run raised it.
  - The `declared-gates-changed` row is kept as written.
  - The code-stage integrity stop has its route: a repair commit, and exempting the code stages' paths from hooks that rewrite or stage files.
  - The stop-kind table lists `auth-failed`, `uncommitted-at-return` (commit or remove the named paths, not a decision), `skill-not-loaded`, `schema-violation`, `gate-omitted`, `stage-fault` and `branch-mismatch`.
  - A requester edit in the main checkout stops the run at the next fast-forward.
  - The merge and clone faults are named, each with the remedy quoted from its statement in code.

  *Evidenced* by a critique-skill round (13 findings, fixed or checked), the fault texts grepped from code, and a kinds diff (all 23 `plan.StopKinds`).
- **`auth-failed`'s answer.** It is renewing the credential, then `resume`, which is the CLI's relaunch for a run with no outcome. A relaunch from an outcome is `launch -args`.
- **Status reports leftovers.** It reports `clone/`, `scratch/`, `merge/`, `*.partial`, `refs/lsf/<plan8>/xfer/w<n>` refs and labelled containers. *Proven* by `TestStatusLeftovers`.
- **The configuration carries the container fields, and refusals reach the requester with their message.** This covers an old config and a launch-check refusal, a dry run's included. *Proven* by `TestComposeLayering`, `TestLaunchOldConfig` and `TestLaunchPausedThenRefused`.
- **`claudeBin` is filled once per launch.** It is resolved to its versioned path, with `claudeVersion` to match, and a binary replaced mid-launch is not used. *Proven* by `TestComposeClaudeBinPrecedence`, `TestComposeClaudeVersion` and `TestClaudeBinReplacedMidLaunch` (repointed symlink and in-place overwrite).
- **The configuration's documentation** says that `setup` and gate commands see only the image's environment and the `env` passthrough, so a private registry's credential is passed by name, and that their toolchain must be in the image. Doc text: `lsf-runtime.md` `## CLI` → Configuration.
- **A launch refuses git older than 2.38, naming the version.** *Proven* by `TestCheckGit` and `TestLaunchRefusesOldGit`.
- **The tests pass.**
  - `go test -race -count=1 ./...` passed at 5c3be0f with `LSF_LIVE_REQUIRE=1`, `PROMPTPARITY_REQUIRE_NODE=1` and `GRAPHPARITY_REQUIRE_NODE=1`.
  - `go test -race ./cli` passed after the Phase 5 fixes.
  - The host was loaded by an unrelated `xtask` process throughout: load average 8.5 at the start and 10.3 at the end, on 8 cores. R35's quiet-machine condition is **not met**, so the achieved rigour is evidenced.
  - Upgrade path: re-run `cd lsf-runtime && LSF_TEST_IMAGE=localhost/lsf-ref@sha256:949f2220… LSF_LIVE_REQUIRE=1 PROMPTPARITY_REQUIRE_NODE=1 GRAPHPARITY_REQUIRE_NODE=1 go test -race -count=1 ./...` on an idle host.

## Deviations from the unit entry's scope and publishes

1. **`stop` is published beside launch, status, watch, pause and resume** (requester decision 3: pause is the ref, stop is SIGTERM). The CLI route needs a halt to replace TaskStop.
2. **Lines outside the entry's skill scope.** `skills/lsf-drive/SKILL.md` gains CLI lines in *Pause*, *Read the return* and *Recovery*, outside `::launch`, `::relaunch` and `::startup`, and the Interface's Requires gains the CLI-route inputs. `drive-mechanics` owns the whole file later.
3. **`CLAUDE.md`'s Structure line for `cli/`** describes the command.
4. **`resume` serves only runs with no outcome.** A returned outcome (`paused`, `wave-cap`, `stopped`, `complete`) is relaunched with `launch -args` from its `planSha` and `unscribed`, and `resume` refuses it. The entry's "resumes" is met by relaunching a paused run.
5. **The CLI's state files** are `launch.lock`, `driver.json`, `driver.log`, `launch.json` and `<state>/dry-run`. They sit in the state dir beside the runtime's files. `launch.lock` was not in the plan; a step 2 critique finding added it.
6. **The P-F1 marker** was the requester's typed "Go ahead", not the distinctive phrase the plan proposed. Zero matches still show that no relay carried the turn's text. The marker is common English, so a match would have needed reading in context.

## Deferrals

- **The launcher.** The skill builds the binary into `${XDG_CACHE_HOME:-~/.cache}/lsf-runtime/lsf-runtime` and runs it by full path. It belongs to `runtime-distribution`.
- **The Workflow route.** It stays beside the CLI route; deleting it belongs to `script-retirement` (requester decision 1).

## Discoveries affecting sibling or downstream units

1. **Three remedies are not stated in the code's own fault texts.** The skill states them itself:
   - a stale `index.lock` (git's own error);
   - the siblings of the merge-SHA fault, which the skill presents to the requester with the journal record's path;
   - a branch the agent renamed (the text says only "the clone is kept").

   A unit touching `orchestrator/wave.go` or the fetch-back could put the remedies in the texts.
2. **`lsfrt.Run` writes `summary.json` and then `status.json`, and only logs a failed summary write.** The CLI therefore matches `summary.json` to `status.json`'s launch id by start time. Recording the launch id in `Summary` would make that direct; it is a candidate for the unit that next touches `runtime.go`.
3. **`resolveBin` is unexported in `lsfrt`.** The CLI keeps its own copy, `resolveClaude`, with the same checks (`LookPath`, `Abs`, `EvalSymlinks`, regular file, no `:`). `runtime-distribution` may want one exported resolver.
4. **The skill's CLI route needs a runtime host config, which nothing creates.** A requester writes it by hand from `lsf-runtime.md` `## CLI` → Configuration. `runtime-distribution`, or a setup unit, may want to generate it.
5. **Phase 5 notes, minor and not fixed.**
   - `spawn` and `statusOf` are long and mix concerns.
   - A stale `status.json` that says running, met by a launch not made by this CLI, is reported `running` for an instant.
   - Stop's refusal is not tested for an `ended`, `crashed` or `failed-to-start` driver; the `driverRunning` check covers all three.
   - A dry run's composed-file assertions, test (f)'s `driverLog` content, and the fake claude's recognition by `--session-id` are untested risks.
   - `restoreDriver` has no test: its race, a driver winning the lock between `launch`'s probe and its own driver's `Run`, cannot be forced from the CLI's tests. The state it handles is tested by `TestStopAfterDriverRecordReplaced`.
6. **The requester's manual detach check** (launch from a shell, close it, run `status` from another) was not done this session. `TestLaunchDetachedCrash` proves the session detach.

## Standing obligations

- **Register dispositions: P-F1.**
  - *Disposed: not reproduced on the CLI route.* No call transcript of a run launched in a requester-typed turn carries the turn's text or the relay frame. The relay was a Workflow-tool frame added at launch, and a headless call has no Workflow launch.
  - It remains on the Workflow route until `script-retirement` deletes that route.
  - The evidence is n = 1 run of 8 calls, with a common-English marker.
- **Headless pin.** *Discharged.* All 8 transcripts carry `"claude_code_version":"2.1.294"`. The CLI resolved `~/.local/share/claude/versions/2.1.294` and ran it with `DISABLE_AUTOUPDATER=1`, and the smoke container's version matched `claudeVersion`.
- **Design of record.** *Discharged.* `docs/design/lsf-runtime.md` `## CLI` was amended in the same commit as each behaviour: 50414f3, c0ee2f1, ff2ec5b, 41f8a2e, d6506f1 and c4ba790.
- **R35.** *Not met:* see the last done-condition bullet.

## External-gate re-verification

All pins were confirmed at Phase 4 entry (2026-10-10), with no drift:

- `claude-code-headless`: `DISABLE_AUTOUPDATER=1 ~/.local/share/claude/versions/2.1.294 --version` → `2.1.294 (Claude Code)`.
- `lsf-toolchain`: `go version go1.27.1`.
- `git`: 2.54.0, at least 2.38.

## Execution signals

- **Context at completion:** one compaction during the unit; comfortable after it.
- **Decisions taken mid-implementation, not at a design pass:**
  - `launch.lock`;
  - `drive.go` in step 1;
  - `watch` waiting again while `starting`;
  - `resume` judging `summary.json` by `status.json`'s state;
  - restoring `driver.json` after a refused launch, and the `previous` view;
  - the dry run moved to `<state>/dry-run` under `launch.lock`;
  - refusing an address that conflicts with `-state`'s run.

  Each answered a critique or review finding; the approach did not change.
- **Critique gates:**
  - six per-step rounds (steps 1–3, the skill, and their fix commits);
  - a Phase 5 multi-lens review: 10 Minor findings;
  - a full-diff critique: 3 Significant and 3 Minor findings;
  - a critique of the Phase 5 fixes: 8 Minor findings. Six were fixed in the report's commit. The other two:
    - the dry run touching a live run's refs is answered by the design: under `dryRun` a step that is not replayed stops without running;
    - the overlap between `restoreDriver` and the new `driverState` row is kept, so that `driver.json` stays accurate on disk.

  No round raised a Critical finding. The Significant findings were fixed; the remainder are listed above and in the implementation summary.

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
