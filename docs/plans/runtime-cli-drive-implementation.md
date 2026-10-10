# runtime-cli-drive: implementation summary

- **Working branch:** `lsf-workflow` (base b56f427).
- **Plan:** `docs/plans/runtime-cli-drive-plan.md`.
- **Commits:**
  - 50414f3 (step 1)
  - c0ee2f1 (step 1 critique fixes)
  - ff2ec5b (step 2)
  - 41f8a2e (step 3, and the step 2 critique fixes)
  - d6506f1 (step 3 critique fixes)
  - b291d38 (step 5)
  - 5c3be0f (step 5 critique fixes)

## External gates (Phase 4 entry)

All pins were confirmed on 2026-10-10. None had drifted.

| Gate | Pin | Probed |
|---|---|---|
| `claude-code-headless` | 2.1.294 | `DISABLE_AUTOUPDATER=1 ~/.local/share/claude/versions/2.1.294 --version` → `2.1.294 (Claude Code)` |
| `lsf-toolchain` | Go 1.27.1 (`lsf-runtime/go.mod`) | `go version go1.27.1` |
| `git` | 2.38 or later | `git version 2.54.0` |

The images `localhost/lsf-ref@sha256:949f2220…` and `localhost/lsf-fixture@sha256:344c825a…` were present.

## Files modified

- `CLAUDE.md`
- `docs/design/lsf-runtime.md`
- `docs/plans/runtime-cli-drive-plan.md` (new; the plan, saved from plan mode)
- `docs/plans/runtime-cli-drive-implementation.md` (new)
- `lsf-runtime/cli/main.go`
- `lsf-runtime/cli/config.go` (new)
- `lsf-runtime/cli/defaults.json` (new)
- `lsf-runtime/cli/drive.go` (new)
- `lsf-runtime/cli/launch.go` (new)
- `lsf-runtime/cli/status.go` (new)
- `lsf-runtime/cli/control.go` (new)
- `lsf-runtime/cli/helpers_test.go` (new)
- `lsf-runtime/cli/config_test.go` (new)
- `lsf-runtime/cli/launch_test.go` (new)
- `lsf-runtime/cli/control_test.go` (new)
- `skills/lsf-drive/SKILL.md`

## Per-step record

### Step 1: CLI core

- **What landed.**
  - Subcommand dispatch, with exit codes 0, 1, 2, and 3 and 4 for `status` and `watch`.
  - Run addressing, and the default state dir `${XDG_STATE_HOME:-~/.local/state}/lsf-runtime/<12 hex>/<name>`.
  - The name rule, checked with a regex and `git check-ref-format`.
  - `compose`, the claudeBin resolve and version check, and `checkGit`.
- **Deviation: `drive.go` landed in step 1, not step 2.** `TestClaudeBinReplacedMidLaunch` needs the driver to run a composed file.
- **Decision: the embedded defaults copy `testdata/config.example.json`.** They copy its `defaultRole`, `roles`, `roleRules` and `skills`, and `TestDefaultsMatchExample` keeps the two equal. `fixturerun` is unchanged.
- **Surprise: the overwritten-binary case (ii) needs no live test.** The stub podman's smoke container runs the mounted claude path, so the case is proven by the runtime's smoke-time version refusal. The repointed-symlink case (i) is proven by the recorded run argv, which mounts v1.
- **Surprise: `"a."` is a valid ref component.** It came out of the invalid-name list.

### Step 2: launch, drive, watch, status

- **What landed.**
  - `launch` composes the config and starts the driver detached (`Setsid`). It waits up to 5 minutes for the launch checks, polling `status.json` and the child's exit.
  - `-dry-run` runs in the scratch state dir `<state>.dry-run`.
  - `status` gives the driver-state table, the summary and outcome when the driver has `ended`, the pause ref, leftovers, xfer refs and labelled containers.
  - `watch` blocks on a shared `flock`.
- **Decision.** `watch` waits again while the status says `starting` or `running`. A driver spawned but not yet holding the lock would otherwise end the wait at once.
- **Surprise.** `syscall.Getsid` does not exist in Go's `syscall`. The detach test reads the session from `/proc/<pid>/stat` (`sessionOf`).

### Step 3: pause, stop, resume

- **What landed.**
  - `pause` and `pause -lift` work through `git update-ref` and verify the result.
  - `stop` sends SIGTERM only to a `running` driver this CLI launched. It checks `/proc/<pid>/cmdline` first.
  - `resume` replays `launch.json`'s args against the current host config. It refuses while a driver runs, and refuses after a returned outcome.
- **Deviation: `stop` added.** It is decision 3; the plan lists it in its Scope adjustments.
- **Deviation: `launch.lock` added.** The step 2 critique found that an overlapping launch could overwrite `driver.json` before its driver lost the lock (S1 below). `launch.lock` serialises launches. It is not in the plan.
- **Surprise: `TestResume` first stopped with `skill-not-loaded`.** The defaults' `skills` rules require skills the fake claude does not report. The test's host config sets `"skills": []`.

### Step 5: the `lsf-drive` skill

- **What landed.**
  - The Route paragraph: the CLI route when a host config exists, the Workflow route otherwise. The binary is built to `${XDG_CACHE_HOME:-~/.cache}/lsf-runtime/lsf-runtime`, and a run keeps its route.
  - Startup step 4 split by route.
  - Launch, Pause, Read the return and Recovery got their CLI lines.
  - Relaunch got the new kinds, the `uncommitted-at-return` row, the code-stage integrity stop, a requester edit in the main checkout, a runtime-stops table, and the merge and clone faults with the remedies quoted from code.
- **Deviation.** The Pause, Read the return and Recovery lines are outside the entry's Scope; the plan lists them in its Scope adjustments.
- **Kinds diff.** All 23 `plan.StopKinds` appear in Relaunch, and so do all of the done condition's kinds.
- **Remedies not stated in the code's own text:**
  - **A stale `index.lock`.** git's own error carries it; the skill gives the remedy, removing the lock once no git process holds it.
  - **The merge-SHA fault's siblings.** For example, `… is not on <branch>`. The remedy for the main text, removing the journal record, is in an `orchestrator/doc.go` comment. For the siblings the skill says the code gives no remedy and has them presented to the requester.
  - **A branch the agent renamed.** The fetch-back text only says "the clone is kept". The skill's remedy, restoring the branch name or recovering the work, is its own.

### Step 6: evidence run

The run used a fixture repo built by `workflows/lsf/spike/fixture.sh`, with plan SHA `608f480` on `main`. The host config named the image `localhost/lsf-fixture@sha256:344c825a…`, the versioned `claudeBin` `~/.local/share/claude/versions/2.1.294` with `claudeVersion` 2.1.294, and the proxy with the live credentials file. The caps were 40 calls, $25 and 1 h. The steps followed the skill's commands, from the built binary:

1. **Pause, then launch.** `pause -name stats` warned that it used the default ref, since there was no `launch.json`, and set the ref. `launch` returned a stateDir under `~/.local/state/lsf-runtime/80cf8f7f351e/stats`. `watch` reported `driver` `ended`, `stop_reason` `returned`, 0 calls and `outcome.status` `paused`, with no leftovers or containers.
2. **Lift and relaunch.** `pause -lift` deleted the ref. The relaunch was made in the turn in which the requester typed "Go ahead": `launch -args -` from the outcome's `planSha`, launch `20261010T085919-3098916`, exit 0. The marker the plan proposed, `PF1-MARKER-quince-7319`, was not typed, so the turn's own text served as the marker.
3. **Watch to the end.** `watch`, run as a background Bash, exited 0: `driver` `ended`, `stop_reason` `returned`, 8 calls, $1.02, 2 min 38 s, `outcome.status` `complete` at `92d2f92`, pause not set. Leftovers, `xferRefs` and `containers` were empty, and so was `podman ps -a --filter label=lsf.state`.
4. **Resume refused.** `resume` exited 1: "the last launch returned an outcome (status complete): relaunch with launch -args from its planSha 92d2f92…".

## Per-step critique findings

The critic was sonnet `phronithm:critic`. Steps 1–3 used critique-code; step 5 used critique-skill. No round raised a Critical finding.

### Step 1 (50414f3)

- **Significant:** a wrong-typed `claudeVersion` or `claudeBin` was silently ignored. *Fixed* (c0ee2f1): a non-string `claudeBin`, `claudeVersion` or `pluginDir` is refused.
- **Significant:** the doc's git 2.32 note said nothing checks the version. *Fixed*: it now says the CLI checks it.
- **Minor:** a load error named the deleted composed file. *Fixed*: it reads "config composed from <host>: …".
- **Minor:** `: <nil>` in the not-an-object message. *Fixed*.
- **Minor:** `resolveClaude` lacked the `:` check. *Fixed*.
- **Minor:** a resolve error did not name its source. *Fixed*.
- **Minor:** the first field of `--version` is taken as the version. *Kept*, as the plan says: a binary that prints a warning line first gives a mismatch, which is a refusal, not a silent pass.
- **Minor:** the plural refused-keys message. *Fixed*.
- **Minor:** test gaps (relative `claudeBin`, `dryRun`, default host path, a hard-coded count, a stale comment). *Fixed*.
- **Minor:** every bad flag was printed twice. *Fixed*.
- **Minor:** `-state` silently overrides `-repo` and `-name`. *Noted for Phase 5*.

### Step 2 (ff2ec5b)

- **Significant S1:** `driver.json` could be overwritten by a launch whose driver then lost the lock. *Fixed* (41f8a2e): `launch.lock`.
- **Significant S2:** the lock-collision retry was one-shot. *Fixed*: up to four starts.
- **Significant S3:** test (a) did not prove the fast-exit branch. *Fixed*: `TestSpawnExitBranch` drives the driver-exit branch in process.
- **Significant S4:** a driver that died with no output gave an empty refusal. *Fixed*: the driver's exit is reported.
- **Significant S5:** `status` and `watch` could block on git or podman. *Fixed*: 10 s bounds.
- **Minor M1:** a late rename to `launch.json`. *Fixed*: `launch` waits up to 5 s and warns.
- **Minor M2:** `crashed` was judged from stale probe data. *Fixed*: the lock is re-probed.
- **Minor M3:** composed files leaked by a launch that gave up. *Fixed*: the driver removes them.
- **Minor M4:** test gaps and risks.
  - *Fixed*: the cleanup ordering.
  - *Noted*: the dry run's scratch-dir and composed-file assertions, test (f)'s `driverLog` assertion, and the fake claude being recognised by `--session-id`.
- **Minor M5:** doc gaps.
  - *Fixed*: the watch loop is documented, and so is the composed file a launch leaves.
  - *Noted*: a lone `launch.json` or `status.json` reporting `none`, and a missing file not being named in `errors`.

### Step 3 (41f8a2e)

- **1:** `resume` failed after a crash followed by a failed start. *Fixed* (d6506f1): `summary.json` is judged by `status.json`'s state. Tested by `TestResumeAfterCrashThenRefusal`.
- **2:** test gaps. *Fixed*: `TestStopRefusesForeignDriver` (no `driver.json`, and a pid that is not the driver), `TestLaunchLock`, `TestResumeAfterCrashThenRefusal`. *Noted*: stop's refusal for `ended`, `crashed` and `failed-to-start` is untested; the `driverRunning` check covers all three alike.
- **3:** a launch that gives up releases `launch.lock` before its driver holds `lock`. *Documented* in `## CLI` → Launch, item 3: `status` reports the first driver `starting` until it ends.
- **4:** any `flock` error read as "another launch". *Fixed*: only `EWOULDBLOCK`.
- **5:** the rename wait ended silently. *Fixed*: a warning.
- **6:** a garbled refusal message. *Fixed*.
- **7:** pause's git was unbounded. *Fixed*.
- **8:** pause's ref resolution without `launch.json`. *Fixed*: a warning naming the default ref, the repo error returned, and "not set" when there was nothing to lift.
- **9:** the one-SIGTERM negative check is a 300 ms sleep. *Kept*: it cannot fail spuriously.
- **10:** a crash between `summary.json` and the final `status.json`. *Noted for Phase 5*: the window is in `lsfrt.Run`, and a resume replays from the journal.

### Step 5 (b291d38)

All 13 findings were *fixed* in 5c3be0f, except finding 6, which was *checked*.

- **1:** the merge-SHA text was misquoted. Now quoted as emitted, with its siblings.
- **2:** exit 4 before a first launch. The skill now gives `status`'s exit codes.
- **3:** a launch exit 1 can leave the driver running. The skill now checks `status` first.
- **4:** stop's semantics. The graceful first `stop`, and the refusal while `starting`, are now stated.
- **5:** route stickiness. A run keeps its route.
- **6:** the `merge failed:` prefix. *Checked*: the merge stage's label is `merge`, and the statement is `<stage> failed: <reason>`.
- **7:** uncovered fault texts. Two more are now quoted.
- **8:** the hook remedy. The skill now says it is the project's hook, and that removing the clone does not help.
- **9:** dated provenance. Removed.
- **10:** the dry run. Its pass criterion is now given.
- **11:** a `crashed` driver at Startup. It now goes to Recovery first.
- **12:** Requires. The CLI-route inputs are now listed.
- **13:** the `crashed` row. Both meanings are now stated.

## Success criteria

| # | Criterion | Status | Target | Achieved | Evidence |
|---|---|---|---|---|---|
| S1a | `lsf-drive` launches, pauses, resumes and reads status through the CLI | pass | evidenced | evidenced | step 6: pause, launch (paused), status, lift, relaunch from the outcome (complete), background watch, resume refused; `resume` replay proven by `TestResume` |
| S1b | P-F1: no call transcript of a run launched from a requester-typed turn carries the message or `Workflow harness` | pass | evidenced | evidenced | see P-F1 below |
| S1c | The driver outlives the launching session | pass | proven | proven | `TestLaunchDetachedCrash` (d): the driver's session differs from the test's, and it runs on after `launch` exits |
| S1d | Relaunch content | pass | evidenced | evidenced | critique-skill round (13 findings, all fixed or checked); the fault texts grepped from code; the kinds diff |
| S1e | Status reports leftovers, xfer refs and labelled containers | pass | proven | proven | `TestStatusLeftovers` (e) |
| S1f | Container config fields; old-config and launch-check refusals (dry run included) reach the requester | pass | proven | proven | `TestComposeLayering`, `TestLaunchOldConfig` (c), `TestLaunchPausedThenRefused` (b and the dry run) |
| S1g | `claudeBin` filled once, versioned path, version to match; a binary replaced mid-launch not used | pass | proven | proven | `TestComposeClaudeBinPrecedence`, `TestComposeClaudeVersion`, `TestClaudeBinReplacedMidLaunch` (i and ii) |
| S1h | Config doc: setup and gates see only the image's environment and `env`; toolchain in the image | pass | evidenced | evidenced | `lsf-runtime.md` `## CLI` → Configuration |
| S1i | A launch refuses git < 2.38, naming the version | pass | proven | proven | `TestCheckGit`, `TestLaunchRefusesOldGit` |
| S1j | `go test -race ./...` passes on a quiet machine | pass, not quiet | proven | evidenced | see R35 below |
| S1k | Headless pin confirmed | pass | evidenced | evidenced | all 8 transcripts carry `"claude_code_version":"2.1.294"`; the CLI resolved the versioned path and the smoke container matched it |

**R35.** At 5c3be0f, `go test -race -count=1 ./...` passed with `LSF_LIVE_REQUIRE=1`, `PROMPTPARITY_REQUIRE_NODE=1` and `GRAPHPARITY_REQUIRE_NODE=1`, taking 6.5 min. An unrelated `xtask` process held 7 of the 8 cores: load average 8.5 at the start and 10.3 at the end. The quiet-machine condition is therefore not met. The upgrade path is to re-run the same command on an idle host.

**P-F1.** The run's 8 call transcripts are `<stateDir>/calls/*.20261010T085919-3098916.*.jsonl`:

- `plan_add-mean`
- `critique-plan_r1_add-mean`
- `implement_add-mean`
- `critique-code_r1_add-mean`
- `lens-review_r1_add-mean`
- `review-gate_w1`
- `obligations`
- `termination`

The greps found 0 matches in every transcript, for "Go ahead" (case-insensitive), for `Workflow harness`, and for the proposed marker `quince`. A positive control shows the transcripts are searchable: `add-mean` matched 4–9 lines in each, and each holds its user turns. The marker is weaker than the plan's distinctive phrase, since "go ahead" is common English. Even so, 0 matches in all 8 transcripts means no relay carried the turn's text.

## Handoff to Phase 5

- **Noted items:**
  - `-state` overrides `-repo`/`-name` silently.
  - The step 2 M4 and M5 remainders.
  - Stop's refusal for ended runs is untested.
  - The `lsfrt.Run` crash window between `summary.json` and `status.json`.
- **The requester's manual detach check** is still to do: launch from a shell, close it, then run `status` from another shell.
- **Review scope:** `lsf-runtime/cli/`, `docs/design/lsf-runtime.md` `## CLI`, `skills/lsf-drive/SKILL.md`.

## Remaining phases
Complete Phase 5 (Review) as defined in the phronithm:feature skill.
