# LSF: proposed improvements

Proposals raised from running `phronithm:large-scale-feature`, `phronithm:lsf-implement` and `phronithm:lsf-drive` on archipeligo's foundation programme (Phase 3 closed 2026-10-02: 58 work units and 5 termination units). Each one names the failure it addresses; the evidence for §2–§9 is in [the archipeligo postmortem](lsf-postmortem-archipeligo-foundation.md). None is designed yet; the design of record is [lsf-workflow](lsf-workflow.md). §10 orders them.

Several proposals change a principle the design records. Each says which, so the change is argued rather than slipped in.

## 1. lsf-drive: out-of-plan commits while a run is live

**Gap.** *Commit discipline* says the session commits nothing in Phase 3. That covers plan, architecture and log edits. It says nothing about a change the requester asks for that lies outside the plan, made while a run is live. The example was a tooling fix: adding `b3sum` to the devshell's `flake.nix`.

**Risk.** Committing on the integration branch mid-run makes the session one more writer on the main checkout, alongside the merge, scribe, ledger and plan-copy stages. Pathspec-scoped commits ([Constraints](lsf-workflow.md#constraints-from-the-workflow-tool)) keep each stage's own commit clean. They do not stop a merge or a re-plan from landing between the session's edit and its commit. They also do not stop the session's commit from moving the integration branch under a stage that has read its HEAD.

**Proposal.** Add a short section to `skills/lsf-drive/SKILL.md`:

- Make the change on a side branch in its own worktree, and verify it there.
- Merge it into the integration branch only after the run returns. This includes a `stopped` or `paused` return, and it happens before the next launch's pre-flight.
- Record the pending merge in the run report. If the session might end first, record it in memory too, so a session change does not lose it.
- If the change bears on a unit in flight, it is a decision for the next launch, not a side commit.

## 2. Mechanical stages transcribe through an LLM

**Cause.** The script has no filesystem or shell access ([Constraints](lsf-workflow.md#constraints-from-the-workflow-tool)), so every git or file step is an `agent()` call. In one archipeligo run (`wf_09b12cc9-894`, 60 agents, five waves), 26 agents did load, merge, cleanup, ledger or similar stages. The `mechanical` tier runs on haiku at low effort ([Model tiers](lsf-workflow.md#model-tiers)). That suits stages that run a command and return a SHA. The load stage is different: its prompt (`workflows/lsf/implement.js`, copy stage) asks the agent to return the whole `yaml lsf-plan` block verbatim.

**Failure.** With a plan block of about 26 KB, 2 of 3 consecutive loads failed. Each failure stopped the run with a spurious `invalid-plan`, although the committed plan was valid:

- `wf_405a2c0f-0ec`, wave 2 load: `planBlock` was the literal string `"see tool results file"`.
- `wf_b21a2e97-f5f`, initial load: `planBlock` stopped after the first unit, with no closing fence.

Each stop cost a requester round trip and a relaunch. The design's premise that "the parser checks it on the next load" holds, but the check turns a transcription fault into a stop, not a retry.

Across the whole programme the count is at least 14 faults in 218 haiku invocations (postmortem §C). Beyond the load echo, ledger appends overwrote, deleted and dropped rows, one never restored; a denied commit was reported with HEAD as its SHA; a copy stage dropped a completion report's findings line; the decision-log scribe invented a `clears` entry; a cleanup agent tried to grant itself permissions by writing `.claude/settings.json`; and another attempted a nested launch (§8). Outside that count, a transient API 529 stopped a run as `agent-null`.

Proposals, in order of effort:

1. **Verify and retry.** The load agent runs a committed, dependency-free extractor, for example `tools/lsf-extract`. It prints the block and, on a separate line, the block's byte length and line count, and the agent returns all of them. The script compares the returned block against the extractor's counts. On a mismatch it retries the stage, up to a small bound, before it raises `invalid-plan`. The same pattern fits the decision-log block and any other verbatim copy.
2. **Shrink the payload.** For a completed unit the script needs only `id` and `complete: true`. Unit prose entries are already archived on completion. Archiving completed units out of the `lsf-plan` block as well, or collapsing them to a `completed: [ids]` list, keeps the block in proportion to the remaining work, not the programme's history. In archipeligo, 57 of 58 units were complete at the failing load. `docs/plan-graph.md` and `graph.js` would need to accept the collapsed form, and `phronithm:breakdown` re-plan mode would need to emit it.
3. **A host-side driver.** Run the loop as a host program, not a Workflow script. It does git, YAML parsing, merge, cleanup and ledger appends in code, and calls a model only for judgement stages: plan, critique, implement, review, gate, re-plan and termination. This removes the mechanical-agent class of fault and its per-agent floor ([Constraints](lsf-workflow.md#constraints-from-the-workflow-tool), the floor item). It gives up the Workflow tool's journal resume and `/workflows` progress view, so the driver must rebuild both.

   The exemplar is `~/src/maths-proofs/tools/campaign/` (`run.py`, `README.md`): a host-side Python loop in which each iteration is one bounded `claude -p` turn, run through the `ic` isolation container with `--output-format stream-json`. State across turns is git plus the rules file, re-read each turn. The runner owns caps, the kill switch, resume, per-iteration telemetry (latency, cost, tokens, commit before and after) and transcripts. It supplies the mechanism and the evidence, never the content. A host-side lsf driver should start from that structure.
4. **Script-rendered decision log.** The script holds the decision objects, so it renders the decision-log YAML itself, as it already renders ledger rows and the completion report, and the agent only writes and commits it.
5. **Retry a null agent.** Retry a null `agent()` return once or twice with backoff before raising `agent-null` (`implement.js:641`, `:669`). lsf-drive already relaunches on `agent-null`, so nothing is lost.
6. **Denials are reported, never worked around.** Mechanical prompts say: on a permission denial, stop and return `denied` with the command; never edit settings or retry another way. A denial in a stage the loop depends on (load, ledger, decision log, copy, merge) is a structural stop. A cleanup denial is a ledger row, consistent with cleanup never signalling, and §7.7's pre-flight check makes it rare.

Proposals 1, 2, 4, 5 and 6 are small. Proposals 1 and 2 reach only the load echo; the faults in four other stage kinds (ledger append, completion-report copy, decision log, cleanup) are outside them. Interim checks on ledger and copy stages (a returned diff stat, or hash, compared by the script) would be reported by the same agent whose transcription is in doubt, so they are weaker than proposal 1's committed extractor. Proposal 3 warrants its own design note; it also bears on §4.

## 3. Empirical work: criteria, measurement units, sizing

**Failure.** Every one of archipeligo's 8 Phase 2 re-entries followed a stop on a measurement, tuning or gate unit: about 92 h stop-to-relaunch (an upper bound, including requester absence), and 20 of the 38 units added after planning. Three gate designs had defects that arithmetic showed without a run, found only after a stop. Phase 1 evidence was measured on a different engine with an instrument the brief had called invalid. Postmortem §A.

Proposals, small and largely skill text:

1. **Criteria and evidence** (`skills/large-scale-feature/SKILL.md`, Phases 1–2). A success criterion judged by measurement states its statistic, sample, horizon and decision rule. Evidence cited for an architecture claim names the system and instrument it was measured on; evidence from another system is marked as a premise for Phase 3 to test.
2. **A `measurement` unit type** (`skills/breakdown/SKILL.md`). Its entry carries a pre-registered design and its pass probability at the effect the requester needs (not at the certified bar, where a correctly sized gate passes with probability α by construction); an explicit fail branch naming the unit or decision it routes to; and the artefacts it retains (§6.5). A cheap feasibility probe precedes an expensive gate. Existing tuning infrastructure collapses the companion design unit only if it searches the parameters jointly or justifies separability, reports a confidence for its estimate, and has run on the system as it will exist.
3. **Size by outcome** (`skills/breakdown/SKILL.md`, *Unit size budget*). A unit with more than one independently consumed Publishes item, or whose measurement outcome selects among later units, splits at that seam. The wall-clock flag does not exempt a measurement unit with an uncertain outcome. The decision re-plan already runs this quality check (`implement.js:574`); the change is to what the check counts.
4. **Superseded evidence is not an invalidation** (`implement.js` gate prompt). When a measurement supersedes a figure cited as evidence, and no decision or rule changes, the gate records it as `document-text`, not `architecture-invalidated`. In archipeligo this plausibly covers R1 and R2, which invalidated §13 evidence paragraphs; it does not cover R8, which changed a rule. A separate calibration register beside the plan, holding measured values and committed constants, is the larger structural form of the same idea; propose it only if the prompt clause proves insufficient.

## 4. Long-running work inside a wave (design note needed)

**Failure.** A campaign of hours runs in one single-shot implement stage. 18 such stages took 39.7 of 62.1 implement agent-hours, the longest 9.1 h on one core of eight. stage2-horizon-confirm's 80 runs outlived the stage; review found no results, the run stopped `gate-non-convergence`, and an orphan process the workflow had no record of ran on. Postmortem §A.

**Proposal.** A design note for running measurement units in stages (plan, critique of the registration, run, analysis, review), where the run stage is a registered external job. This is not a small change: a unit parked across waves breaks the wave barrier ("units in a wave execute against a pinned plan version"), re-plan semantics (the plan is amended under a parked unit) and prompt-keyed resume, and the script cannot check liveness without an agent. A host-side driver (§2.3) can wait on an external job natively, which removes most of that machinery, so this note and §2.3's should be written together.

## 5. Significant findings need an owner

**Failure.** Below Critical, findings are recorded and read by no stage. The fix stage gets Criticals only, implement never sees the plan critique, the gate does not re-evaluate findings, the re-plan's item kinds exclude them, and lsf-drive shows counts. In archipeligo two came true unaddressed, the requester patched the gap by hand twice, and the close-out audit missed review-stage rows on units it did not cover (postmortem §B). Precision is unmeasured.

The design chose this deliberately: the Critical-only fix bar gives the critique loop its convergence, and "there are no volume or recurrence rules" keeps stops from scaling with reviewer verbosity. Proposals 1, 2 and 4 keep both principles; proposal 3 is where they would change.

1. **Text, not counts** (`renderCompletionReport`; lsf-drive *Read the return* and *Present*). List each unfixed Significant finding's one-line text and referent. This alone would have shown the requester the two that came true.
2. **Finding identity across rounds** (`implement.js`). Rows repeat per review round; give a finding an id that later rounds reuse, so any consumer sees findings, not rows.
3. **One consumer, designed** (design note amending `lsf-workflow.md`'s Stop rule and Surfacing). Choose between: the re-plan absorbing unfixed Significants with a verdict set that includes a non-stopping `deferred-with-reason`; or the gate receiving them as candidate premises, with one that falsifies an architecture premise raised as an invalidation in the existing five-field form. Not both. Before choosing, sample archipeligo's ledger rows for precision.
4. **Programme close-out** (`skills/large-scale-feature/SKILL.md`, *Termination*). Before the requester accepts, review what was delivered against the request record and the vision document, and point to a process retrospective. lsf has never had a programme-level step for either; the per-feature retrospective (`fcabec2`) does not cover a programme.

## 6. Workflow state: refresh it, verify it, keep what matters

**Failure.** State the script holds went stale or was destroyed. Postmortem §D.

1. **Architecture pin (phronithm#2).** `ctx.architectureSha` is set once at load (`implement.js:1019`), so a unit's legitimate amendment is judged as an invalidation. Merge reports any architecture-document diff the unit carried, and the gate receives that diff: wording is recorded as `document-text`, and a decision-moving diff is an invalidation candidate, since units have no licence to change decisions. After the gate, the script advances the pin to the merged SHA. Refreshing the pin without passing the diff would let a unit's own decision change become the gate's baseline.
2. **Gate results after a fix.** Keep a `gates` variable, replace it from the fix stage's return, and pass it to the next review round and to `renderCompletionReport` (`implement.js:759-760`, `:474`). Add `gates` to the fix return schema.
3. **Gates that outlive the tool timeout.** Agent Bash calls move to the background after 600 s, and `xtask ci` exceeds that, so seven agents across all unit types returned with gates not run. Implement and fix prompts say: poll a backgrounded gate to completion with bounded checks before returning, and never end a turn waiting on it. A return with a `not-run` gate retries the stage rather than going to review.
4. **Plan checks before launch.** Ship load's graph, overlap and `clears` checks as a host-side script sharing `workflows/lsf/`'s parser. breakdown runs it before committing an lsf plan, and lsf-drive's *Startup* pre-flight runs it on `git show <planSha>:<plan>`. Two archipeligo plans fresh from Phase 2 stopped at load on overlap.
5. **Keep data cleanup cannot see.** Cleanup runs `git status --porcelain --ignored` per worktree and keeps, and reports, any worktree holding ignored files over a threshold, instead of force-removing it (`implement.js:609`). This changes the recorded premise that cleanup failure "leaves residue, not risk" (`lsf-workflow.md:266-268`): an 83 GB loss falsified it, and the text should say so. A `Retains:` line in the unit entry, naming a destination outside any checkout, lets a unit move its data before merge; a hashed manifest is deferred.

**Deferred: merge gates.** Merge re-runs the project's gates after each merge in series (13.6 agent-hours, which also covers archive and bookkeeping commits; no regression caught in 53 merges). Batching gates per wave would save at most about a third of gate runs, and it conflicts with per-merge undo (`git reset --merge` of exactly the regressed merge, `lsf-workflow.md:39-40`, `:261-262`). Not proposed until a multi-merge undo is designed.

## 7. lsf-drive: presenting stops and handing over

**Failure.** Decision options hid the consequence that mattered, despite the existing free-text rule (`SKILL.md:124`); twice the requester took a decision to a more capable model, which reframed it. Options stayed inside a frame that had failed. Pre-clearable gates cost a stop each. Handoff state went unverified. Postmortem §E.

The design makes the session a presenter at any model tier (`lsf-workflow.md:86-90`), and *Present* says not to rank or filter the agenda. Proposals 1 and 2 change that role and need the design text amended with them.

1. **Model floor and delegated analysis** (`lsf-workflow.md` session role; lsf-drive *Present*). For a stop that routes to Phase 2 or sets a numeric target, the session runs on opus or above, or spawns a judgement agent (or `phronithm:investigate`) to state each option's consequence for the request record's success criterion on one common metric. The session presents that analysis in prose before any question. A recommendation states its basis and any asymmetry a critic or adversary found, and marks an option that reverses an earlier requester ruling.
2. **Reframe** (*Present*; `skills/large-scale-feature/SKILL.md` Phase 2 re-entry; the design's relaunch-target table). On a second stop against the same architecture section or unit line, list levers outside the current options and offer "investigate first". An investigation becomes a relaunch target, whose committed result the reconciliation record cites. This is a presentation prompt, not a stop kind, so it does not add a recurrence rule to the script.
3. **Pre-clear gates** (*Launch*). List the uncleared organisational gates reachable in this launch and the open "requester matter" notes from the last digest, and offer to clear any whose evidence is already in hand, as decisions with `clears`.
4. **Handoff outside the working tree** (*Return to Phase 2*, *Termination*). Write the next-session prompt to auto-memory or the session scratchpad, never the working tree (the re-plan pre-flight relies on a clean tree), so the startup prompt is "continue <name>". Verify every worktree, URL and command in it by running it before writing it down.
5. **Recovery cause** (*Recovery*). Establish who ended the run (a TaskStop in the launching session's transcript, a usage limit, a crash) before stating a cause, and mark it observed or inferred in any commit or memory.
6. **Uncommitted decisions** (*Relaunch*). After a TaskStop, compare the decision log's last id with the launched decisions and carry any uncommitted ones into the next launch.
7. **Permissions** (*Startup* pre-flight). Name the permissions the workflow needs (commits on the integration branch; `git branch -d` and `git worktree remove` for `worktree-wf_*`) and check them before launch.

## 8. Relayed requester messages

**Failure.** The Workflow harness relays the message that triggered the launch turn to every agent, ranked above the computed task. 20 of archipeligo's 28 runs carried one. About 4 were off-topic, and in one run two implement agents answered the relayed question instead of implementing. 9 were the launch instruction itself; in the five of these launched from a pasted startup prompt, 24 haiku agents invoked the lsf skill, and one cleanup agent attempted a nested launch. Runs launched from a tool result carried none. Postmortem §F.

**Proposal.**

1. **Launch from a tool result** (lsf-drive *Launch*, *Relaunch*). Always confirm the launch through AskUserQuestion, so the launch turn is triggered by a tool result and the relay is the computed task alone. This covers the pasted-prompt case too.
2. **Defence in depth** (`implement.js`). A shared stage preamble saying a relayed request is the requester's message to the driving session, not addressed to this stage. It competes with a harness instruction ranked above it, so it is a hedge; the fix that matters is proposal 1 and the harness report already filed.

## 9. Phase 2 gate discipline

**Failure.** The one re-entry gated by a sonnet critic alone passed a design reversed about 25 h later, though adversary-gated re-entries also introduced reversed defects. A Phase 1 review instrument ran only as Phase 3's first unit and forced four amendment rounds. No criterion, original or amended, was checked against the vision; the certified world turned out frozen, a defect no version of the criterion measured. Postmortem §A, §G.

**Proposal.**

1. **Adversary by requester direction, and the re-run** (`skills/large-scale-feature/SKILL.md`; `docs/critique-gate.md`). `critique-gate.md` holds that reinstating the adversary "waits on an opus-tier re-run, not on precedent". Keep that. In lsf, recommend that the requester direct an opus adversary pass at Phase 2 and every re-entry, under the existing requester-directed clause, with findings adjudicated independently. Record archipeligo's passes, with confirm and refute counts, as the reason to schedule the opus re-run.
2. **Run Phase 1 instruments in Phase 2** (Phase 2 step 3). Where Phase 1 produced a review instrument for the architecture's subject, run it before the critique gate closes, not as a Phase 3 unit.
3. **Criteria against the vision** (Phase 1 step 6; re-entry step a). Every success criterion, original or amended, gets one adversarial question: describe a system that passes the criterion and fails the vision. The brief's change log records each relaxation. After a criterion's second relaxation, recommend a vision review before the next launch, as a prompt, not a stop.
4. **Record what the design has run on** (`docs/design/lsf-workflow.md` *Status*). List the unit kinds the workflow has run on. Before a programme dominated by measurement units, dry-run one gate unit.

## 10. Order

| Tier | Items |
|---|---|
| Small; do first | §2.1, §2.4, §2.5, §2.6; §3.1–§3.4; §5.1, §5.2, §5.4; §6.1–§6.4; §7.3–§7.7; §8.1; §9.2, §9.4 |
| Change a recorded principle; amend the design text with them | §6.5 (cleanup keeps data; its premise changes); §7.1, §7.2 (session role); §9.1 (adversary recommendation) |
| Own design note | §2.3 with §4 (host-side driver, long-running units); §5.3 (a consumer for Significant findings) |
| Deferred | §2.2 (moot if §2.3 lands); merge-gate batching (§6); hashed artefact manifest (§6.5) |

§1, §8.2 and §9.3 are small and can go with the first tier.
