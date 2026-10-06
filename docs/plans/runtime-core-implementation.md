# runtime-core — implementation summary

- **Branch:** `lsf-workflow`.
- **Plan:** `docs/plans/runtime-core-plan.md` (`5588468`).
- **Base:** `e5cfd56`.

## External-gate re-probe (Phase 4 entry)

- **`lsf-toolchain`:** `go version` reports go1.27.1 (`GOTOOLCHAIN=auto`). Confirmed.
- **`claude-code-headless`:** **drift found.**
  - Claude Code auto-updated from 2.1.289 to 2.1.290 during the session. The first step-0 recording ran on 2.1.290 (`claude_code_version` in its init events).
  - Resolution, requester's choice: re-record on the pin by calling `~/.local/share/claude/versions/2.1.289` directly with `DISABLE_AUTOUPDATER=1` (`703b295`). Every fixture records 2.1.289.
  - The installed default is still 2.1.290. Later units' gate checks should call the versioned binary or confirm the pin.

## Per-step record

### Step 0 — plan saved, probes, fixtures (`5588468`, `567a32a`, `703b295`)

All probes ran with haiku/low, the prompt on stdin, `--strict-mcp-config` and `--permission-mode dontAsk`. Stdin delivery works, slash command included.

**P1, unknown model.**
- The assistant event carries `error: "model_not_found"`. The result has `is_error: true`, `api_error_status: 404` and `terminal_reason: api_error`, and the exit code is 1.
- The stream has no `unrecognized_model` value. The done condition's term corresponds to `model_not_found` in the stream.

**P3, ok call.**
- Valid `structured_output`.
- A real `rate_limit_event` with `status`, `resetsAt` (unix seconds) and `rateLimitType`, matching the schema in the binary.

**P2, skill load (deviation from §2.4's candidate markers).**
- A slash-command expansion is **not on stdout.** No `user` event carries it. Only the session transcript on disk has `<command-name>`.
- init's `slash_commands` lists every command that will expand.
- An unknown slash command (`/phronithm:nonexistent-skill`) is sent as plain text, and the model still returns a valid output. Without a check, a missing skill would go unnoticed.
- **Marker:** a call loaded its skill when either
  - the prompt was prefixed and init's `slash_commands` contains the skill, or
  - an assistant `tool_use` named `Skill` has `input.skill` equal to it.
- **Counts on 2.1.289:**
  - prefixed and known: listed in 3/3 runs, expansion on disk in 3/3;
  - prefixed and unknown: listed in 0/1, expansion in 0/1;
  - no prefix: the Skill tool loaded the skill in 4/5 runs (1/5 absent).
- **Earlier counts on 2.1.290:** prefixed 3/3; no prefix 3/3 via the Skill tool.

**Fixtures.** `lsf-runtime/testdata/streams/` holds `ok`, `permanent`, `skill-loaded`, `skill-tool`, `skill-absent` and `skill-unknown`, with trimming per its README.

### Step 1 — config, validator, stream, classification (`ee3c051`; fixes in `1b05eb6`)

- **Deviation:** classification reads the `result` event first, as the plan's critique required. The skill-load rule follows step 0.
- **Critique (round 1, no Critical).** Fixed in `1b05eb6`:
  - Significant: a type-drifted stream field dropped the whole result line, so it now drops only that field; enum numbers now compare by value; negative durations, an empty `backoff` and trailing config data are now rejected.
  - Minor: messages truncate on rune boundaries without HTML escaping; a non-string `type` is reported; regex docs say "unanchored".
- **Noted, not changed:**
  - property errors come out in sorted, not insertion, order (documented in `validate.go`);
  - `tools` stays required, so a tool-less role cannot be expressed. That is `s2-permissions-design`'s to raise if it needs one.

### Step 2 — Run skeleton, spawn, stub (`1b05eb6`; fixes in `5a05a65`)

- **Deviation:** the reaper (`children.go`) and signal watcher (`supervise.go`) were written here because `Run` needs them. Their tests are step 5's.
- **Surprise:** a mutation check (the post-`Wait` group SIGKILL removed) left TERM-ignoring `sleep 300` processes. They were killed by pid, and the mutation failed the test as it should.
- **Critique (round 1, no Critical).** Fixed in `5a05a65`:
  - Significant:
    - a cancellation before `Start` was classed `spawn-error`;
    - a transcript write error could stall the child on a full pipe;
    - the stop event came from a goroutine and could be lost;
    - a `ps` failure reset `children.json`;
    - orphan matching was a bare substring on possibly truncated `ps` output (now `--session-id <id>` with `ps -ww`, and only a group leader's group is signalled).
  - Minor:
    - the cause is taken only if the context ended while the process ran;
    - an invalid outcome no longer suppresses `summary.json`;
    - panic stacks are logged;
    - signal handlers are installed before reaping;
    - stub lock, stdin and wait fixes.
  - Minor, comment only: the pgid-reuse claim is corrected to an accepted residual window.

### Steps 3–4 — Call, gate, Parallel, journal, Step, worktrees (`5a05a65`; fixes in `ff839cb`, `b8a8ea4`)

- **Deviation:** steps 3 and 4 landed in one commit, because `Call` needs the journal and `cwdFor`.
- **Surprise:** `json.MarshalIndent` re-indented `RawMessage` returns, so replays were not byte-identical. Values are now compacted on both paths.
- **Critique (round 1, no Critical).** Fixed in `ff839cb`:
  - Significant:
    - a call queued for a slot did not re-check the session-limit gate;
    - relative `StateDir` and `Repo` paths are now made absolute;
    - a stale `wt/<key>` is now validated, and a non-worktree stops the run;
    - branch names are made ref-safe and keys length-bounded;
    - label uniqueness was an unstated assumption: it is now stated, and reuse is warned as `label-reused`.
  - Minor:
    - a cancellation only counts when it hit a running process;
    - a stale `resetsAt` still waits the margin;
    - every limit hit moves the gate;
    - nil and `null` step returns are unified;
    - typed-nil-safe `stopErr`;
    - stop re-checked after the worktree mutex;
    - the session id is in transcript names;
    - `Parallel` logs panic stacks.
  - Noted only: `children.json` is rewritten with fsync three times per spawn, which is accepted.
- **Surprise (`b8a8ea4`):** the new queued-call test failed once in five runs. A session-limited call released its slot before moving the gate, so a queued call could spawn into the limit. The gate now moves before the slot is released.

### Step 5 — supervision tests (`ff839cb`; fixes in `8964f20`)

- Subprocess tests re-exec the test binary as a launch.
- **Critique (round 1, no Critical).** Fixed in `8964f20`:
  - Significant:
    - the dry-run test's no-worktree and no-branch assertions were vacuous, because a sticky step stop returned first, so the test is split;
    - the orphan test's readiness check could race the stubs' pid records;
    - the reuse of a valid worktree on resume had no test, and its check now reads git's stdout only.
  - Minor:
    - new tests for a session limit that resets past the wall-clock cap, and for caps under `Parallel` (exactly `MaxCalls` spawns, sticky);
    - a dry run validates an isolated call's args;
    - an empty step return is treated as null;
    - failed tests kill their stubs;
    - replay-after-stop documented.
  - Not changed: a final group SIGKILL in `killGroup` after the grace period. It is the same accepted residual as in `spawn`.

### Step 6 — docs (`b8a8ea4`)

`docs/design/lsf-runtime.md` §Runtime and `testdata/config.example.json`. A test loads the example config.

## Files modified

- `lsf-runtime/runtime/`:
  - code: `api.go`, `config.go`, `validate.go`, `stream.go`, `classify.go`, `fsutil.go`, `runtime.go`, `call.go`, `children.go`, `supervise.go`, `agent.go`, `parallel.go`, `journal.go`, `worktree.go`;
  - tests: `config_test.go`, `validate_test.go`, `classify_test.go`, `helpers_test.go`, `call_test.go`, `agent_test.go`, `journal_test.go`, `supervise_test.go`.
- `lsf-runtime/testdata/`: `claude` (the stub), `config.example.json`, and `streams/` (six streams plus a README).
- Docs: `docs/design/lsf-runtime.md` (the Runtime section), and `docs/plans/runtime-core-plan.md` and this file.

**Size:** 2,190 lines of code and 1,730 of tests, against the spike's estimate of 1,500–2,500 for the whole runtime including tests (`conjectured`). The estimate's scope also included code stages, signals and the CLI.

## Success criteria verification

55 tests pass under `go test -race` from `lsf-runtime/` and are stable over repeated runs (`-count=2` after the Phase 5 fixes). `go vet` and `gofmt -l` are clean; `staticcheck` is not installed.

| # | Status | Target → achieved | Evidence |
|---|---|---|---|
| S1 invalid retried once, then null | pass | proven → proven | `TestRetryClasses` (`invalid,ok`, `invalid,invalid,ok`, `nullout,ok`) |
| S2 transient backoff | pass | proven → proven | `TestRetryClasses` (`transient…`: spawns and minimum delay) |
| S3 session-limit wait | pass | proven against the stub → proven; the real `-p` shape stays `conjectured` | `TestSessionLimitWaitsForReset`, `TestSessionLimitWaitsAreCapped`, `TestQueuedCallRechecksLimit`, `TestSessionLimitPastWallClockStops`, and the classification tests. **Upgrade path:** when a real run hits a limit, its transcript under `calls/` fixes the shape |
| S4 permanent not retried | pass | proven → proven | `fixture:permanent` (real 2.1.289 stream) gives 1 spawn and null. The stream code is `model_not_found`, not `unrecognized_model` |
| S5 slash command and load check | pass | proven → proven | `TestSkillLoadCheck` (real loaded, Skill-tool and unknown streams; shared budget); `TestFlagsSurviveReplay` |
| S6 `--strict-mcp-config` and role flags | pass | proven → proven | `TestArgvCarriesRoleAndSkill` |
| S7 resume replays calls and steps | pass | proven → proven | `TestResumeReplaysCompletedCalls`, `TestNullIsNotJournaled`, `TestStepJournal`, `TestStepInterruptedAndFailed` |
| S8 orphans reaped after SIGKILL | pass (Linux) | proven → proven on Linux; macOS `conjectured` | `TestOrphansReapedByNextLaunch`. **Upgrade path:** `runtime-distribution` CI on macOS |
| S9 caps | pass | proven → proven | `TestCaps`, `TestCapsUnderConcurrencyAreSticky`, `TestReplayDoesNotCountTowardCaps` |
| S10 guard timeout kills the group | pass | proven → proven | `TestGuardTimeoutKillsGroup` (three hang modes; mutation-checked), `TestGuardTimeoutIsNotRetried` |
| S11 signals stop at the boundary | pass | proven → proven | `TestSignalStopsAtNextBoundary` (SIGINT, SIGTERM), `TestSignalDuringBackoff`, `TestSecondSignalKillsInFlight` |
| S12 `stop_reason` recorded | pass | proven → proven | the caps, signal, dry-run and runtime-error tests, `TestSwallowedStopIsRecorded`, `TestBodyOutcomes` |
| S13 dry run spawns nothing | pass | proven → proven | `TestDryRunSpawnsNothing` (no spawn, worktree, branch or step) |
| S14 roles are input | pass | proven → proven; code reading `evidenced` | two roles' flags in `TestArgvCarriesRoleAndSkill`; `prepare` has no role defaults, and no non-test file names a tool or rule |
| S15 worktree isolation | pass | proven → proven | `TestWorktreeIsolation`, `TestWorktreeReusedOnResume`, `TestParallelWorktrees`, `TestStaleWorktreeDirectoryStops`, `TestWorktreeNeedsArgs` |
| S16 vet and test pass | pass | proven → proven | `go vet ./...`; `go test -race ./...` |
| S17 Runtime section | pass | evidenced → evidenced | `docs/design/lsf-runtime.md` §Runtime |

## Phase 5 — review

**Full-diff review** (phronithm:review, seven lenses, `go vet` and `gofmt` pre-scan) and **full-diff critique** (critique-code, round 1). Neither found anything Critical. Fixed in `5d61691`:

- **Found by both.** `children.json` lost a killed launch's orphan records whenever `ps` failed, because the next launch's first spawn rewrote the file. Unreaped records are now carried forward, and a test makes `ps` fail.
- **Found by both.** A guard timeout after a valid result event discarded the result. The value now stands, flagged `guard-killed-after-result`. This refines the requester's guard decision, which covered a hung call (one without a result). **Requester to confirm.**
- **Review.** A hung git command could block the run with no way to signal it. Git now runs under a timeout and ends on a repeated signal, and a semaphore that yields to a stop replaces the mutex.
- **Review and critique.** A worktree was created before admission. It is now created at the first admitted attempt.
- **Minor:**
  - the slot is released on panic;
  - the schema must be a JSON object;
  - an integration branch may not begin with `-`;
  - `prune` runs only after a failed add;
  - the hash covers the skill and isolation;
  - an unreadable step record stops the run;
  - the reaper treats zombies as gone and is tested with TERM-ignoring orphans;
  - state files are 0644;
  - a panic outranks cancellation;
  - child bookkeeping lives in `children.go`;
  - `CLAUDE.md` gains an `lsf-runtime/` entry.
- **Noted, not changed:**
  - `Call`'s size: the attempt loop was split into `attempt`, but the verdict switch remains;
  - a session-limit wait is unbounded except by the wall-clock cap (now logged with its length);
  - a normal exit whose descendant holds stdout costs `killGrace` and leaves the descendant running;
  - `is_error` subtypes such as `error_max_turns` are unprobed.

  The last three are in the design's Known limits.

**End to end.** One launch ran against the real 2.1.289 binary, using the example roles narrowed to `Read` and `Read,Grep,Glob,Skill`:
- an isolated haiku call ran in its worktree on `lsf/01234567/<key>`;
- a `critique-code:` call was sent as `/phronithm:critique`, and its init event listed the command;
- both returned `ok` on attempt 1, for $0.013;
- a second launch replayed both with 0 spawns.

The same check ran before the Phase 5 fixes ($0.06). Rigour: `evidenced`.

## Remaining phases
Phase 5 complete. The completion report is `docs/plans/runtime-core-completion.md`.
