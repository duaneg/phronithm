# s2-sandbox — completion report

- **Work unit:** `s2-sandbox` of `docs/plans/lsf-improvements-breakdown.md` @ `db4c7b3`.
- **Branch:** `lsf-workflow`.
- **Plan, implementation summary:** none; the unit stopped at Phase 1.

## Done-condition verdict

**Not met; nothing implemented.** The unit stopped at Phase 1 (scope confirmation) on a requester decision, 2026-10-09. The requester questioned B2's premise: the only container option probed in `s2-permissions-design`, `ic` as it stood, was rejected for tool-specific reasons (a headless hang, a stale image Claude, credentials mounted read-write). A plain container was never probed. The requester chose to stop the unit and re-plan to reopen the permissions decision.

P-C5 and P-C6 are not disposed of.

## Deviations from the unit entry

None; no work began.

## Deferrals

The whole unit, pending the re-plan's permissions decision.

## Requester decisions taken at scoping (2026-10-09)

These stand whatever substrate the re-plan chooses:
- **401 with no refresh.** A call failing 401 on an expired, unrefreshed token stops the run. It is not journaled, so a relaunch resumes. This settles the entry's open decision.
- **Gate checkout.** It becomes a plain clone, not a worktree: a worktree's `.git` points into the main repo, which the boundary hides.
- **Hosts pin.** `hosts` and the gate block are pinned from the plan at the launch's `args.planSha` before any agent call. Every load, the first included, compares against that pin. This closes the gap where the decision re-plan could widen its own network (D27's first-load baseline).

## Discoveries affecting other units

- **Containers are unprobed.** The rejection of `ic` doesn't cover containers in general. A plain podman or docker container per call and per gate would replace `--restricted`, the `/tmp` sharing and the `/run/containerd` workaround with kernel confinement of the whole process. It still needs the credential proxy, an allow-list egress proxy for `hosts`, and an image carrying the pinned `claude` and the project's toolchain. The requester uses podman elsewhere.
- **Requester's goal** for the substrate: "whatever works, is simple, and robust".
- **Linux only.** The requester stated only Linux matters. This contradicts `runtime-language-design`'s "Linux + macOS", and affects R26 and `runtime-distribution`'s macOS CI.
- **Bash HOME under B2.** Writes to a masked path are discarded per command, so caches don't persist. A container makes this moot.
- **Credential proxy.** The Go port of `inject-proxy.py`, with refusal of an empty bearer, is needed in either design: the probe's proxy accepts a request with no Authorization header.
- **Orchestrator scope.** The hosts pin and a hosts-only block need changes beyond `orchestrator/git.go`: `loop.go`, `gates.go`, the `Runtime` interface and `orchtest`. The entry's Scope does not name them.
- **`s2-clones` discoveries** handed to this unit (read-only `.git` in every clone, merge clone write access, `pluginDir` resolution, the git config isolation clause) remain open for whichever unit implements the boundary.

## Standing obligations

- **Headless pin:** checked; `~/.local/share/claude/versions/2.1.294` with `DISABLE_AUTOUPDATER=1` reports 2.1.294.
- **Design of record:** not touched; no behaviour changed.
- **R14, R17:** still open. Runtime stages still run under the spike's deny list, and the recorded streams are not re-checked against 2.1.294.

## External-gate re-verification

- `sandbox-runtime` (`srt` 0.0.79): installed to `~/.local/bin` (`npm -g --prefix ~/.local`); `srt --version` reports 0.0.79. R31 discharged.
- `bubblewrap` (0.13.0): `bwrap --version` reports 0.13.0. No drift.
- `lsf-toolchain` (Go 1.27.1): `go version` reports go1.27.1. No drift.
- `claude-code-headless` (2.1.294): present and reports 2.1.294. No drift.

## Execution signals

- **Context:** comfortable.
- **Design decisions taken mid-implementation:** none.

Run `/phronithm:breakdown` in re-plan mode with the decomposition plan and this report before starting any dependent unit.
