# s2-sandbox: completion report

- **Work unit:** `s2-sandbox` of `docs/plans/lsf-improvements-breakdown.md` @ `3e7b8bb`.
- **Branch:** `lsf-workflow`, `3e7b8bb..13c1e4d`.
- **Plan:** `docs/plans/s2-sandbox-plan.md`. **Implementation summary:** `docs/plans/s2-sandbox-implementation.md`.

## Done-condition verdict

**Met.** Achieved rigour is **proven** for the config, argv, launch-failure, teardown, reaping and launch-check clauses. These are stub tests over a strict stub `podman` plus 8 live tests on podman 6.1.3. **Evidenced** applies to the `Containerfile`, the fixture run and the R17 re-check. The Phase 5 full-diff critique judged every clause met; the R17 clause carries an open risk (see Discoveries).

| Clause | Verdict |
|---|---|
| Every agent call in its own rootless podman container, launched as A call's launch states | met; the launch adds `--pull=never`, `--log-driver=none`, the host-path mount and a `.git` self-bind (Deviations) |
| Config: image by digest (tag refused), `claudeBin`, `env`; roles `tools` and `clone` only; `allow`, `deny`, `permissionMode`, `<plugin>` gone | met; `claudeVersion`, `podmanBin` added; `pluginDir` and `proxy` required |
| Launch refuses: podman or `pasta` missing, not rootless, image absent, smoke fails `claude --version` or `bash`, `git`, `rg` | met; the smoke also checks the `.git` binds and the `.git` mountpoint |
| 125/126/127 before the stream → `runtime-error`, not retried | met |
| API only through the credential proxy spliced by `pasta`, placeholder bearer; a non-`127.0.0.1` `listen` refused | met; live L2 showed the upstream receiving the live token, not the placeholder |
| `.git/config`, `hooks`, `info` read-only in every clone, the merge clone included; missing `hooks`/`info` created; the conflict call writes the merge clone | met, and hardened beyond it (Discoveries 1) |
| Cancel: SIGTERM to the group, then after `killGrace` `podman rm -f`; fetch-back, the uncommitted check and removal wait; a grace-expiry kill leaves no container | met (stub and live L3) |
| A launch reaps its label's containers; `--session-id` reaping gone | met |
| `/tmp` placement refusal dropped; shallow, submodule, LFS refusals stay | met; a gitlink at the tip is refused too |
| Reference `Containerfile` under `lsf-runtime/container` | met; built, and the live suite passes against it |
| Tests cover each call's argv and environment | met: argv in stub goldens; environment live (L2) |
| `tools/fixturerun` end to end in containers on 2.1.294, no `hosts`, `pin` 2.1.294 | met: run 2 complete (9 calls, $1.19). Run 1 (7 calls, $1.15) did the work but stopped on a re-plan escalation about the fixture brief's wording, a model judgement |
| Streams re-checked against 2.1.294 (R17) | met, with open risk: no field the runtime reads moved, but the container path emits no `rate_limit_event` (Discoveries 3) |
| Runtime changes names this unit and `s2-egress-gates` as carriers | met |
| The tests pass | met: full suite with `LSF_LIVE_REQUIRE=1`, `PROMPTPARITY_REQUIRE_NODE=1`, `GRAPHPARITY_REQUIRE_NODE=1` |

## Deviations from the unit entry's scope and publishes

- **The clone is mounted at its host path, not `/work`** (requester decision D-path, this session). `lsf-runtime.md` is amended.
- **Launch argv additions:** `--pull=never`, `--log-driver=none`, `-v <clone>/.git:<clone>/.git:rw` (Discoveries 1), and `-t 0` with a 60 s bound on `podman rm`.
- **Hardening the host's git in clones, beyond the decided design:**
  - `gitenv.CloneGitDir`, `CloneGitlinks` and `RevGitlinks`;
  - `--ignore-submodules=all` and `-c diff.ignoreSubmodules=all -c submodule.recurse=false`;
  - `GIT_CEILING_DIRECTORIES`.

  These touch `internal/gitenv` and `orchestrator/{git,merge}.go`. The orchestrator files are outside the scope's "its tests' role configs".
- **Config additions:** `claudeVersion` (D-pin), `podmanBin`, required `pluginDir` and `proxy`.
- **Paths:** only `:` is refused in `stateDir` and `pluginDir`; `,` works in a podman bind source.
- **The state dir is made `0700`**, and an existing group- or other-readable one is tightened.
- **A failed `podman ps` or `rm` during reaping refuses the launch.** The plan had a failed `ps` logged and the launch going on.
- **A failed `rm`** whose container is gone (`--rm` won) is not a failure.
- **`CLAUDE.md` was edited** (`internal/`, `testdata/podman`, the streams), outside the unit's scope.
- **The 2.1.289 streams are kept**, with the 2.1.294 re-recordings beside them in `testdata/streams/2.1.294/`, because other tests depend on the old outputs.
- **`lsfrt.StateLabel`** is a new export, for `fixturerun`'s leftover check.
- **Publishes:** all four as listed. The container configuration adds `claudeVersion` and `podmanBin`; the launch checks add the `.git` checks.

## Deferrals

- The egress proxy, `hosts` confinement, gates in containers and their teardown: deliberately not implemented. They belong to unit `s2-egress-gates`. Until then a call's Bash has no network, even with `hosts` set, and gates run on the host (R14).
- Gate-marker reaping in `children.json`: stays until gates run in containers, which belongs to unit `s2-egress-gates`.

## Discoveries affecting sibling or downstream units

1. **The decided boundary's read-only `.git` binds were not enough (R34 claim incomplete).** All three bypasses were confirmed live or reproduced:
   - an agent renames `.git` away and supplies its own, config included;
   - an agent writes `.git/commondir`;
   - an agent commits a nested repository as a gitlink with a filter in its own config. Host `git status` runs that filter, and a committed `.gitmodules` with `ignore = none` overrides `-c diff.ignoreSubmodules=all`.

   Each made the runtime's host git run an agent-defined filter. Fixes:
   - `.git` is a mountpoint;
   - host git in a clone refuses a redirected or emptied `.git` and any gitlink;
   - `status` and `diff` get `--ignore-submodules=all`;
   - the fetch-back refuses a gitlinked branch.

   **For `s2-egress-gates`:** the gate checkout's design mounts at `/work` and runs no git after its first command. It should still apply the same `.git` reasoning and the host-path question to anything git does there. **For any unit that adds a host git call in a clone:** go through `cloneCmd` or `execGit`, or call the guards. An unguarded site is silently exposed.
2. **An agent that commits a nested repository ends the run.** The fetch-back stops with `runtime-error` naming the path. Routing it back to the unit as a stage fault is not done: candidate work for `runtime-signals` or the orchestrator.
3. **No `rate_limit_event` on the container path.** None of 16 container calls emitted one; every host-direct recording does. A real session limit would wait `sessionLimitFallback` (15 min), not `resetsAt`. Whether the CLI still reports the limit as an assistant `rate_limit` error through the proxy is unverified. **Decision for the requester:** whether a 429 `result` with no assistant error should be classed `session-limit` (fallback wait) rather than `transient`. This affects `runtime-signals` and `runtime-cli-drive`.
4. **Project hooks run on the host over agent-written trees.** The merge step's commit in the merge clone runs the copied hooks on the host (Known limit added). Relevant to `s2-egress-gates`, which could run them in a container.
5. **Configs must change.** Any existing runtime config fails to load until it gains `image`, `claudeVersion`, `pluginDir` and `proxy` and drops `allow`, `deny` and `permissionMode`. Affects `runtime-cli-drive` and `phronithm:lsf-drive`, which build or document configs. `Proxy()` is never zero inside a body now.
6. **`podman` is now a launch prerequisite,** even for a dry run (the smoke container runs). So is a user-built image named by digest.
7. **Per-command guard cost.** Each host git command in a clone runs `ls-files --stage` and `ls-tree -r HEAD` first. This is unmeasured on large repos; caching per phase is a candidate.

## Standing obligations

| Obligation | Status |
|---|---|
| Register dispositions P-C5, P-C6 | **Disposed.** P-C5 (`git branch -d` by an agent): an agent call cannot reach the main repo's refs: only its own clone is mounted (argv golden tests; live L2), and its commits come back only as its own branch by fetch. P-C6 (settings edit after a denial): the main checkout is not mounted and the plugin is read-only, so no agent can write the main checkout's settings. The container-probe's settings probe held, and the mounts are asserted here. Both are enforced by the kernel, not by agent behaviour. |
| Headless pin: check against `~/.local/share/claude/versions/2.1.294` with `DISABLE_AUTOUPDATER=1`, or cite a transcript's `version` | **Met.** Probed at Phase 4 entry and after the fixture runs (`2.1.294 (Claude Code)`). All 16 fixture transcripts record `claude_code_version` 2.1.294, as do the R17 recordings. |
| Design of record: amend `lsf-runtime.md::runtime` and `::permissions` in the same commits as the behaviour | **Met.** Every behaviour commit (`db5b5dd`, `cf2dde1`, `524faf6`, `6776b42`, `6c90a5f`, `a75371c`, `79c3e7b`, `13c1e4d`) carries its amendments. |

## External-gate re-verification

| Gate | Pin | Result |
|---|---|---|
| `lsf-toolchain` | Go 1.27.1 | confirmed (`go version`, `go.mod`) |
| `claude-code-headless` | 2.1.294 | confirmed at Phase 4 entry and end. 2.1.295 and 2.1.296 are installed too; 2.1.294 is not pruned |
| `podman` | 6.1.3 rootless, `pasta` 2026_10_02.cba3570 | confirmed at Phase 4 entry and end; log driver `journald` |

No drift.

## Execution signals

- **Context:** comfortable at completion; no compaction occurred.
- **Design decisions taken mid-implementation, not at a design pass:**
  - the `.git` self-bind and the git guards (Discoveries 1), across three critique rounds and one Phase 5 round;
  - reaping pulled forward into step 2b;
  - reap failure refusing the launch;
  - smoke without API variables;
  - keeping the 2.1.289 streams;
  - the fetch-back gitlink stop;
  - tightening an existing state dir's mode.

  The git-hardening thread was the largest. A critique gate found a Critical class in three successive rounds before converging, which suggests the boundary's git trust model warranted its own design pass.
- **Paid spend:**
  - R17 recordings: about $0.02;
  - fixture runs: $1.15 + $1.19.

---

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
