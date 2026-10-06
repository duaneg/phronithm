# LSF empirical work

How lsf plans, runs and records work whose outcome is a measurement. Sections are owned by units of the [LSF improvements programme](../plans/lsf-improvements-breakdown.md): Criteria (`empirical-criteria-design`), Units (`empirical-unit-design`), Retained artefacts (`retained-artefacts-design`).

## Retained artefacts

Unit `retained-artefacts-design`, plan @ `b7f5332`.

**Status:** decided, 2026-10-07. Requester verdict: **recipe-cited**. A record cites regenerable data by recipe and never calls it must-keep. Only non-regenerable data may be cited by path, and only under a store outside every checkout. Cleanup deletes ignored files unless a unit has declared them kept (requester, same day).

### Problem

P-D5 ([register](lsf-problem-register.md)): archipeligo records treat regenerable run series as must-keep, and cite them through a worktree path. Keeping them costs 80 GB in `~/src/archipeligo-runs/` and the 372 MB worktree `wf_b1de45fb-5d4-20`. Separately, cleanup force-removed a 190-series set (83 GB, P-C9) that nothing in the unit entry or the implement return had named.

### Evidence

Archipeligo citations are at `c220c62`.

- **Regeneration cost.**
  - The 48 GB `stage2-horizon-confirm` series (40 runs at 20k ticks, 40 at 100k) ran in 4 h 27 min (`archipeligo-runs/stage2-horizon-confirm/launch.log`: start 2026-09-29T06:02:34+13:00, finish 10:29:08).
  - The postmortem puts the removed 190-series set at about 8 h.
- **Determinism is shown only on a prefix.**
  - `trigger.md:51-58` records a 2 000-tick run at engine commit `950be81`, byte-identical (`cmp`) to the first 2 001 lines of one retained 20k-tick `population.csv`.
  - `identity.log` shows a 2 000-tick override run `dup-of` the committed run.
  - No full-length 20k or 100k series has been regenerated, and no `flows.csv` or `intake.csv` has been compared.
- **Recipe-cited has been chosen once already.** After the 190 series, their binaries and `launch.sh` were lost, the requester chose to regenerate them only on demand (`bid-era.md:82-92`, erratum 2026-10-01). The recipe given was:
  - build `xtask` at `dd8932c`;
  - run the record's four *Launch* commands;
  - check every file against `bid-era-series.txt`.

  That recipe has never been run. It depends on launch commands inlined in the record, because the launch script is gone.
- **Worktree-path citations.** `trigger.md:54-55` and `bid-era-runs.txt:192` onward cite paths under `wf_b1de45fb-5d4-20`; `bid-era.md:90-92` relies on the worktree's symlink to keep those paths valid. `foundation-reconciliation.md:84-87` keeps the worktree and its branch because "they still anchor `2f220af`". Tag `stage2-horizon-registration` already points at `2f220af` (`foundation-close-out.md:127`).
- **The store's contents.**
  - `stage2-horizon-confirm` is 48 GB, cited by `horizon.md`'s *Series retained* table, by size and blake3.
  - `b1-frozen-regime` is 26 GB, cited by `b1-frozen-regime-results.md:5`.
  - `hoard-cell` is 6.6 GB, cited by `hoard-cell/record.md:120-125`, by realpath and sha256.
  - The store's README asks that anything regenerated for a record go in its own subdirectory, with the record naming the path.
- **Non-regenerable data.** The maths campaign (`~/src/maths-proofs/tools/campaign/README.md`, Telemetry and artefacts) records model transcripts, cost, tokens and latency, which no recipe reproduces. Its runner writes them to a host `--out-dir`, which plays the store's role. The agent, in the `ic` container, can write only the clone. lsf's own call streams and telemetry are already in the runtime's state dir, outside the worktree (`lsf-runtime.md`, Journal and state dir).
- **Cartomancer** cites no retained data (register, P-D5 probe).

### Options

| Option | Record may cite as must-keep | Cost |
|---|---|---|
| Recipe-cited | non-regenerable data only, under a store | each record carries a full recipe. The recipe can rot (toolchain drift, a lost script), and regenerating takes time when a check is wanted and the cache is gone |
| Retained path in a store | any data, under a store outside every checkout | storage grows without bound; records depend on disk state |
| Retained path in a worktree | any data, in the producing worktree | as above, plus cleanup must keep the worktree; this is where the archipeligo records are now |
| Recipe plus cost threshold | regenerable data above a regeneration-cost threshold the plan declares | a threshold no evidence places |
| Nothing beyond cleanup | unconstrained | cleanup stops deleting, but records may still cite worktrees |

Weighing regeneration against storage: under recipe-cited, a record stays checkable whichever cost is paid. Keeping regenerable data as a cache becomes a storage decision the requester can revisit. It is no longer an obligation a record creates. At the measured rate, dropping a 48 GB series costs about 4.5 h of reruns, paid only when a check is wanted.

### Decision

**Classes.**
- *Regenerable:* the record carries a recipe, and a full-length regeneration of at least one output has matched every recorded hash. A prefix or a shorter run does not qualify.
- *Non-regenerable:* everything else.

The burden of proof is on regenerability.

**A recipe** names:
- the source commit, reachable from a tag or a merged branch, not only from a unit branch;
- the toolchain (pinned file or version), the dependency lockfile, and the platform the hashes were taken on;
- the build command, or the binary's hash if a built binary is used;
- the launch command with every seed and input, in version control or inlined in the record;
- each output file's size and hash.

**Records.**
- Regenerable data is cited by recipe. A record may say where a cached copy is, but must not require that copy to exist.
- Non-regenerable data is cited by a path under the project's store.
- No record cites a worktree path or a path inside any checkout, directly or through a symlink.

**Store.** The project declares one store directory outside every checkout; `archipeligo-runs/` is one. A `Retains:` line in a project with no store is a plan error.

**Unit entry.** A `Retains:` line names the worktree paths a unit keeps. Nothing else a unit produces outside version control is kept.

**Cleanup**, for each worktree whose branch merged:
- paths named by `Retains:`: the runtime moves them to `<store>/<unit>/`, then removes the worktree;
- every other ignored or untracked file: removed with the worktree.

Cleanup lists what it removed in its report. Keeping data is a deliberate act, the declaration; the default is deletion.

**The move** is a runtime step, journaled like any other, so a kill mid-move resumes it. `<unit>` is the plan's unit id, not the runtime's worktree key. If the destination already exists, the move stops rather than merge into it. The implementing unit settles cross-filesystem copy and verification.

**On a substrate with no storage outside the worktree** (the maths exemplar's `ic` container): the runtime does the move, on the host; the agent does not. The agent writes under its worktree and declares the paths. The policy holds without the agent getting any write access outside the worktree.

### Walk against P-D5's records

| Record | Under the policy | Change |
|---|---|---|
| `horizon.md` *Series retained* | not yet regenerable: no full-length run has been regenerated | add the recipe: commit `2f220af` (tagged `stage2-horizon-registration`), toolchain, the two launch commands inlined (they are in `launch.sh` in the store, not in git). Regenerate one h100k run and match its three hashes; until then the store copy is must-keep |
| `bid-era.md:51` | the hash-equality condition holds for a regenerated series | none |
| `bid-era.md:90-99`, `bid-era-runs.txt` | non-conforming: paths under the worktree, kept valid by a symlink | cite the store or the recipe, not the worktree path |
| `trigger.md:54-55` | non-conforming: a worktree path | cite the `horizon.md` recipe and the file's hash |
| `bid-era.md:82-92` erratum (190 series) | non-regenerable by the class rule: never regenerated | as written it is the policy's shape; it becomes regenerable after one full-length check |
| `hoard-cell/record.md:120-125` | non-regenerable until checked; cited by store path, which is allowed | none required |
| `b1-frozen-regime-results.md:5` | non-regenerable until checked; cited by store path | none required |
| `foundation-close-out.md:122-126` | keeps the worktree and branch for the path citations and to anchor `2f220af` | once the three path citations change, the worktree and branch can go; the tag anchors `2f220af` |

Under the policy, none of the 80 GB may be dropped before a full-length check. After one passes for a series, that series becomes an optional cache. Changing the archipeligo records is the requester's call and lies outside this programme.

### Generality

P-D5 has not been shown outside archipeligo. Cartomancer produced no retained data. The maths campaign shows that a second project has non-regenerable data, not that it has the problem.

The two parts of the decision are adopted on different bases:
- **The citation rule** is scoped to programmes whose records cite measured data. A project with none declares nothing.
- **The cleanup rule** is adopted generally. Deleting undeclared files is cleanup's present behaviour, so a project with no `Retains:` line sees no change. The 83 GB loss (P-C9) is prevented by the declaration and by the ban on citing worktree paths, not by cleanup keeping what nobody classified.

### Plan consequences

The policy splits across units. The re-plan names or adds carriers:
- **`code-merge-cleanup`:** cleanup deletes every undeclared ignored and untracked file and lists them in its report. Its done condition currently says the opposite ("keeps a worktree holding ignored files unless the retained-artefact policy names them rebuildable") and is rewritten at the re-plan. Its scope also needs `lsf-workflow.md`'s merge-and-cleanup section, where "a failure leaves residue, not risk" is restated: the risk is now confined to undeclared data.
- **`Retains:`, the store and the move:**
  - the `Retains:` field in the plan block and its parse;
  - the store declaration;
  - the runtime's journaled move to `<store>/<unit>/`.

  These need a carrier. `empirical-unit-design` decides where the declaration lives. The move is runtime work that falls after `code-merge-cleanup`; until it lands, a `Retains:` line keeps the worktree.
- **`empirical-unit-impl`:** the recipe as a record's citation form, and the regenerable/non-regenerable classes in the breakdown skill.
- **`long-running-design`:** a job's outputs follow the same classes.
- **`lsf-runtime.md`:** Not in this unit still assigns the ignored-file policy to `code-stage-interface`, so the pointer moves to `code-merge-cleanup`.
