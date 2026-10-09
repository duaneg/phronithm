# s2-boundary-common — completion report

- **Work unit:** `s2-boundary-common` of `docs/plans/lsf-improvements-breakdown.md` @ `e19f366`.
- **Branch:** `lsf-workflow`.
- **Plan:** `docs/plans/s2-boundary-common-plan.md`.
- **Implementation summary:** `docs/plans/s2-boundary-common-implementation.md`.

## Done-condition verdict

**Met (proven; the documentation clause evidenced).** The implementation summary lists the test behind each clause. In brief:
- **Proxy.** `runtime/proxy.go` is a Go port of `inject-proxy.py`, opt-in by the config's `proxy` block. It starts after the launch lock on the configured `listen` address and shuts down before the summary.
  - It refuses with 403, sending nothing upstream, any request whose `Authorization` is not exactly `Bearer <placeholder>`: an absent, empty, wrong or doubled header included.
  - It substitutes the live OAuth token, re-reads the credentials file on a change of mtime, size or inode, and streams unbuffered.
- **401 stop.** A permanent verdict with status 401 or `authentication_failed`, or a guard kill during 401 retries, sets the new stop `auth-failed` before any journal record. A relaunch re-runs the call.
- **Git isolation.** The runtime's and the orchestrator's git in a clone run with `GIT_CONFIG_GLOBAL=/dev/null` and `GIT_CONFIG_NOSYSTEM=1` (`internal/gitenv`). A globally defined filter selected from a clone's `.gitattributes` does not run in the runtime's `status` or the merge step; each test has a control in which the filter does run.
- **Gate checkout.** `RunGates` fetches the commit by object ID into a new plain repository, with no worktree registered. A commit reachable only from `refs/lsf`, or from no ref, gates.
- **Parser.** The `lsf-gates` block takes `hosts`, alone or beside gates. A hosts-only block is handled as no block (`Declaration.Gated`). Loopback and unspecified entries fail the load.
- **The pin.** The `gates-pin` code step reads the plan at `args.planSha` before any agent call and hands `hosts` to the runtime (`PinHosts`). Every load, the first included, compares against it and stops on `declared-gates-changed`.
- **Docs.** `plan-graph.md`'s Gate block and `phronithm:breakdown`'s Phase 1 gate paragraph name `hosts`. The paragraph asks for the hosts builds and tests fetch from.
- **Tests.** promptparity (55 scenarios) and graphparity (145 cases) are identical with node required. `go vet` is clean. The full race suite passed before the last fix commit. Three timing tests fail intermittently under heavy load on the base `e19f366` at rates comparable to HEAD's (implementation summary, Timing tests under load).

Every step's critic gate, the Phase 5 all-lens review, the skill-artefact gate and the full-diff critique ran on sonnet. None found anything Critical. Every Major was fixed except one: the skill gate's "escalation's plan SHA doesn't exist", which was rejected because `escalation.plan_sha` exists.

## Deviations from the unit entry

The deviations agreed at planning (critique round 1, disposition 16):
- **Files outside Scope:** `lsf-runtime/internal/gitenv` (new), `testdata/claude` and `testdata/streams/` (the 401 fixture), `tools/promptparity` (the `gates-pin` label), the `validate` tests, and `CLAUDE.md`.

Deviations taken during implementation:
- **More files outside Scope:** `skills/lsf-drive/SKILL.md`, `docs/design/lsf-workflow.md` and a `plan/schema.go` comment, edited so their pin semantics agree with D31.
- **Termination prompt.** A declared plan's termination prompt carries the frozen-block clause, as the decision re-plan's does. Undeclared plans' prompts are unchanged.
- **Global excludes and attributes.** They are copied to `.git/lsf-global/` and set as the clone's own `core.excludesFile` and `core.attributesFile`. The plan appended them to `info/*`, which inverts their precedence.
- **Identity of a hosts-only block.** `Declaration.Gates` is `omitempty`. Existing blocks' identities are unchanged.
- **Auth on the guard path.** A call that the guard timeout kills while the CLI retries a 401 is an auth stop. The plan classed only the result.
- **Gate checkout.** It takes the main repo's object format, and fails on a stale directory it cannot remove.
- **Proxy hardening** beyond the plan: `CONNECT` refused, `Proxy-Authorization` dropped, an `http` upstream only on loopback, its own transport, an `IdleTimeout`.

## Deferrals

None.

## Register dispositions

None. P-C5 and P-C6 stay with `s2-sandbox`.

## Discoveries affecting other units

- **`s2-sandbox`:**
  - **Hand-offs.** `Runtime.Proxy()` returns `{URL, Placeholder}`, or the zero value with no `proxy` block. `Hosts()` returns the pinned hosts, and returns nil both before the pin and for a plan with no `hosts`.
  - **Agent-written clone config.** An agent can write a filter driver or `core.hooksPath` into its own kept clone's `.git/config`, and the runtime's next git in that clone runs it. The isolation stops only global and system config. The Phase 5 runtime reviewer rated this Significant. It is the read-only `.git/config`, `hooks` and `info` clause already in this unit's done condition.
  - **Stream pin.** `auth-401.jsonl` is from 2.1.294. The other streams and `tools/fixturerun`'s `pin` constant are still 2.1.289, which is no longer installed. The done condition's re-check against 2.1.294 covers both.
- **`s2-boundary-decision`:**
  - **`listen` is not restricted to loopback**, so a container can reach it. A wildcard `listen` (`0.0.0.0:0`) gives a URL a call cannot use.
  - **`hosts` may name private and link-local addresses.** Only loopback and unspecified ones are refused.
- **`runtime-cli-drive` and `phronithm:lsf-drive`:**
  - **`auth-failed` is a new stop reason.** Present it as "renew the credential and relaunch".
  - **Retry delay.** 2.1.294 retries a 401 ten times, about 3 minutes, before the result arrives.
- **Gate block frozen at launch.** A decision re-plan can no longer change the gate block within a launch. A requester changes `hosts` by committing the plan and relaunching at the new SHA.
- **Register R29 is out of date.** Its last clause says that a clone's runtime git reads no global `core.excludesFile` once this unit lands. The clone now takes the global excludes and attributes files as its own, so that clause should be dropped at re-plan.

## Standing obligations

- **Register dispositions:** none for this unit (see above).
- **Design of record:** `lsf-runtime.md::runtime` was amended in the commits that changed the behaviour it describes.

## External-gate re-verification

- **`lsf-toolchain`** (Go 1.27.1): `go version` reports go1.27.1. Probed at plan time and at Phase 5 close; the Phase 4 entry probe's outcome was not recorded. No drift.
- **`git`** (2.38 or later): `git --version` reports 2.54.0. No drift.

## Execution signals

- **Context:** the unit was paused once, on 2026-10-10, for machine load, and resumed in a fresh session from the dispositions record.
- **Design decisions taken mid-implementation:** about eight, each recorded per step. Those that changed behaviour:
  - the global-files precedence;
  - auth on the guard path;
  - the termination prompt's clause;
  - the object-format gate checkout;
  - the proxy hardening.
- **Size:** medium, as estimated: 50 files and about 2,300 lines added, roughly half of them tests.

## Known limitations

Recorded in `docs/design/lsf-runtime.md` Known limits unless marked *(review note)*:
- **Hooks in a clone** see no global or system git config.
- **Host config still applies** to code-step worktrees and the main checkout's `merge --ff-only`. The main repo's fetch from a clone runs `upload-pack` under host config *(review note)*.
- **Git version.** Isolation needs git 2.32 or later, and the runtime does not check the version.
- **`safe.directory`.** A global `safe.directory` does not reach clone git.
- **Proxy credential rotation.** An in-place, same-length rewrite of the credentials file within one mtime tick is not seen *(review note)*.
- **Host checks.** Private addresses pass the check; so do names that resolve to loopback *(review note)*.
- **Agent-written clone config** (see Discoveries, `s2-sandbox`).
- **Gate checkout cost.** The checkout fetches the commit's whole history, without tags, once per gated tree.
- **Wildcard `proxy.listen`** gives a URL no client can dial.

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
