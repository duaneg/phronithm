// Package orchestrator is the wave loop, its prompt builders and its code
// stages, ported from workflows/lsf/implement.js, which stays the
// behavioural reference until script-retirement. Run is the JS
// run(runtime); Body adapts it to lsfrt.Run. Agent stages go through
// Runtime.Call, which returns each call's CallResult (flags, denials) with
// its value. Load, the decision log, ledger appends and completion reports
// are code steps (Runtime.Step, stages.go, git.go; D18). tools/promptparity
// runs the same scenarios through both and compares every agent prompt,
// option, log and return byte for byte; the JS calls for the code stages
// are answered from the Go steps' values and left out.
//
// # Divergences from the JavaScript
//
// D1 to D8 are the plan package's (plan/doc.go).
//
//   - D9. Stage results are typed. A return that does not decode — a
//     wrong-typed field (a number for a string, "3" or 2.5 for an integer),
//     null for an integer, or no result — is that stage's null, logged
//     ("<label>: return does not decode (…); treated as null"), so it
//     becomes the stage's agent-null (TestDecodeFailureIsAgentNull; the
//     {1: 42} case of TestUnmovedDocumentKeepsPin). JS ignores the type
//     where it tests with typeof or truthiness, and otherwise fails later
//     with a TypeError. A schema integer accepts a finite integral number,
//     3.0 included, within ±2^53; the runtime validator has no bound, so a
//     larger integral value is validated and journaled and then decodes as
//     null on every replay. A missing
//     field decodes to its zero value, where JS reads undefined, and
//     encoding/json matches keys case-insensitively. Under the runtime,
//     schema validation rejects such returns before the orchestrator sees
//     them (retried, then an unjournaled null), so, past ±2^53, this
//     arises from args.seed returns: JS trusts a seed, and Go validates
//     it against its schema (D21), so a seed outside it is a
//     schema-violation on every launch until it is fixed. A journal replay
//     is validated again when replayed.
//   - D10. The nth real call (n ≥ 2) with a label in a launch is label#n,
//     so the journal keeps one record per call. JS reuses ledger:w<n> when a
//     stop or pause scribes after the wave's own scribe, and at wave 0 after
//     the decision re-plan's scribe. Seeds match the label before renaming
//     and stand in on every use of it, as in JS.
//   - D11. Prompt JSON carries objects as typed structs: schema fields only,
//     in schema order. That covers the criticals, conflicts and direct edits
//     agents return, and the caller's args.decisions, whose clears is kept
//     when present (including []) and dropped when absent or null. JS embeds
//     each object as it arrived, with any extra keys, in its key order.
//   - D12. Args are typed. A non-string string field, a fractional
//     maxUnitsPerWave, a non-object seed or a wrong-typed args.unscribed
//     row fails the decode, and a row that signal() rejects is an args
//     error; each ends the launch crashed with the message. JS coerces,
//     ignores or throws. A tier field that is null or "" counts as unset.
//   - D13. A merge row naming a unit outside the wave still completes the
//     unit, as in JS; its entry in the gate and re-plan prompts has empty
//     fields where JS renders "undefined".
//   - D14. A list an agent returns as null, or omits, reads as empty. JS
//     throws on its .length where it reads the list unguarded (directEdits,
//     amendments, appendedUnits), ending the run. The stage schemas require
//     these lists, so under the runtime the case does not reach either.
//   - D15. Exceptions. A JS exception in agent() or in a unit's stages
//     becomes null through parallel's catch, or rejects run(). In Go,
//     Call's only error is a runtime stop, which ends Run at once; a panic
//     in a unit's stages is that unit's null (Parallel recovers it,
//     TestPanickingUnitIsUnitNull), and a panic elsewhere ends the launch
//     crashed.
//
// D16 and D17 fix JS defects, recorded rather than fixed in the frozen JS,
// as the plan package's D1 and D2 do.
//
//   - D16. args.maxUnitsPerWave below 1 is an args error. JS slices the
//     ready set with it: 0 runs no unit, finds nothing blocked, and returns
//     complete with the plan unexecuted; a negative value drops units from
//     the end of each wave.
//   - D17. A completion-report return with an empty branch or no commit SHA
//     is the stage's agent-null (a no-commit signal on the report's path),
//     and the unit aborts. JS leaves such a unit out of the merge with no
//     signal, so it is never completed and runs again every wave.
//
// D18 and D19 are the code stages (unit code-stage-interface).
//
//   - D18. Load, the decision log, ledger appends and completion reports
//     are journaled code steps, not agent calls. Their values keep the
//     agent-return shape; planBlock and decisionsBlock hold the whole
//     documents, from which LoadGraph extracts the blocks. A seed naming
//     one is logged ("<label>: args.seed ignored; code stage") and not
//     used. Load is journaled, so a resume replays the texts and the
//     architecture SHA it saw; HEAD's branch and the pause ref are re-read
//     live after the load and ledger steps, replayed or not. The decision
//     log drops a decision already present with identical content, and when
//     none remains returns the last commit touching the log. Each commit
//     carries Lsf-Step: <label> and Lsf-Content: sha256:<digest of the
//     bytes written> trailers (a commit of several paths, one
//     "Lsf-Content: sha256:<digest> <path>" per path, D24); a stage commit
//     whose content no longer matches its digest, or a bare digest's
//     commit that did not touch the path, is not intact. An interrupted rerun adopts its own
//     intact commit instead of committing again; without an intent, a
//     ledger whose tip already holds the block refuses to append it again.
//     The completion report's input carries the unit branch's tip, so a
//     moved branch reruns the step instead of replaying a stale report.
//     Returned SHAs are full, and checked when made or adopted; a replayed
//     SHA is not re-checked, so a history rewrite between launches can leave
//     the outcome naming a commit that no longer exists.
//     A state dir journaled before this unit holds call records for these
//     labels, which Step treats as stale and reruns: such a state dir is
//     not resumable. A relaunch with changed input after a kill between a
//     ledger commit and its record is not interrupted, so it appends its
//     rows after the committed ones; the repeat is visible in the ledger.
//   - D19. A code stage's failure is a stage-fault stop whose
//     statement is "<stage> failed: <reason>", its referent the path, else
//     the unit, else the stage. The reasons: a git error; HEAD off the
//     integration branch; the path holding changes the stage did not make;
//     a broken existing decision block; a rejected commit (the file is
//     restored and unstaged); an earlier stage commit that is not intact
//     (the guard: repair it with a new commit without an Lsf-Content
//     trailer); a commit that fails verification (it stays, named);
//     content already committed; ledger rows already at the tip. The
//     completion report has no guard: its branch is its own, so a relaunch
//     resets a bad tip away and commits again (the reflog keeps it). A
//     project whose hooks rewrite these files (formatters, whitespace
//     fixers) or stage other paths gets the verification stop, and then the
//     guard's, on every launch until it exempts the paths: the bytes must
//     be exact. JS reports these only as a null or an empty SHA, without
//     the reason.
//
// D20 to D22 are the driver-event signals (unit runtime-signals). D20, the
// fail-closed mappers, is the plan package's.
//
//   - D21. Every agent call's CallResult becomes signals at its call site
//     (site, callSigs; plan.SignalsFromCall). Uncommitted changes at return
//     stop a writer (uncommitted-at-return) and are recorded for a reader
//     (residue-at-return); a skill not loaded stops; permission denials and
//     a guard kill after a result are recorded. Unit stages check the
//     call's stops once, after the stage's own records; loop stages add
//     them to their existing evaluation, so a merge stop still re-plans
//     over what merged. A return outside its schema is a schema-violation
//     stop beside agent-null: the runtime reports a live one as
//     CallResult.Violations, and agent checks a seed itself. JS has no
//     CallResult, validates nothing and has none of these kinds;
//     promptparity's scenarios set no flags, and their seeds satisfy their
//     schemas. The validator knows type, properties, required, items and
//     enum only: a schema using another keyword is not checked for it.
//   - D22. A gate a stage left out of its return (none reported, or one
//     the implement stage ran that a fix did not report) is gate-omitted;
//     gate-not-run is a gate reported not run. JS has one gate-not-run
//     stop for both, listing the not-run gates first; Go raises
//     gate-omitted, then gate-not-run, so a fix with a gate not run and a
//     gate dropped is two stops. promptparity maps gate-omitted back, with
//     its statement; no scenario has both. A plan that declares its gates
//     raises neither from a stage's report (D27).
//
// D23 to D25 are unit code-merge-cleanup's.
//
//   - D23. A worktree writer (plan, implement, a plan or code fix) that
//     reports a branch other than the one the runtime gave its call
//     (CallResult.Branch), or whose branch does not resolve, raises a
//     branch-mismatch stop at its return; the unit aborts there and the
//     wave's stop rule stops the run; the stage's own records from the
//     rejected return (readings, deviations, distress, gate drift) are
//     kept, as for every unit-stage stop (D21). Its SHA is checked against
//     the runtime's branch (checkReturn), and the runtime's branch is the
//     one carried forward. The tip it is checked against is read once, as
//     a code step (branch-tip:<call label>), so a replay checks against the
//     same tip, with the same correction and rows, after cleanup has
//     deleted the branch (D25). A fix's mismatch stops the critique
//     loop before its branch is taken; its critical-fixed rows are already
//     recorded. Without a runtime branch (a stub, a seed, a call journaled
//     before the branch was) the SHA is checked against the reported
//     branch as before, and an unresolvable one is silent. The readers
//     report no branch; a repo writer's fetched branch is the ff: step's
//     (D29), not this check's. The
//     completion report's tip read faults with "does not resolve" on exit
//     1, unless its branch's tip was journaled by a writer's check this
//     launch (a merged unit's replay after cleanup), and with the git error
//     otherwise. A branch that resolves is read live, so a moved branch
//     still reruns the report. JS trusts the reported branch.
//   - D24. The merge is a journaled code step (merge.go), not an agent: it
//     merges each unit's completion-report commit (the report step's journaled
//     SHA, so a replay needs no branch) into the integration branch in the
//     wave's merge clone (D29), with a merge commit carrying Lsf-Step, Lsf-Unit and
//     Lsf-Merge trailers (Lsf-Conflict per resolved path). A conflict goes to
//     a model call (merge-conflict:w<n>:<unit>, default tier) that may only
//     edit and stage; the step accepts the resolution only with no unmerged
//     path, no conflict marker, nothing staged outside the paths git's own
//     merge touches, and every non-conflicted one as git's merge-tree has it,
//     commits it, and otherwise aborts the merge (unresolvable). A conflicted
//     file's content is the call's judgement: only its markers are checked. A
//     call that changes the merge clone outside the merge's paths (a new,
//     changed or deleted file) is a stage-fault naming
//     them, the merge aborted; what it did there cannot be undone by the
//     stage. The call's driver events travel in the value
//     (mergeResult.Signals). A clean merge's tree must be git's merge-tree of
//     the two; a path a hook leaves staged, and a merge git stops with nothing
//     in conflict (a refusing pre-merge-commit hook), are faults. A merge git
//     refuses over local changes (which the merge clone does not hold) is
//     unresolvable; any other failure is a stage-fault, with a merge this
//     step started aborted.
//     Without declared gates no project gate runs and no merge is
//     regressed; with them, see D26 and D27. The
//     bookkeeping (bookkeeping.go) is rendered for the units still to merge
//     before the first merge a run makes, with the plan and archive clean, so
//     a plan it cannot record merges nothing, and committed once over the plan
//     and the archive (commitFiles, one Lsf-Content trailer per path); a unit
//     whose stub at its merge commit is already in the plan (an earlier run of
//     the step that failed later) is not recorded again. An interrupted rerun
//     resumes only a merge in progress whose MERGE_MSG is this step's, adopts
//     its own merge commits (verified again) and bookkeeping commit, and
//     refuses a unit merged under another commit or outside the stage. A merge
//     SHA, planSha or architectureSha that is not hex, does not resolve or is
//     not on the integration branch is rejected (checkMergeShas, live or
//     replayed, with a repository), and a clean merge without its SHA too: the
//     value is a stage-fault and nothing in it is merged. There is no tip to
//     correct a merge SHA against, so unlike checkSha it is not corrected; a
//     journaled SHA a history rewrite removed faults until its journal record
//     (journal/merge_w<n>-*.json) is removed. A seed for the label is ignored
//     (D18). JS's merge agent reports what it likes.
//   - D25. Cleanup is a journaled code step (cleanup.go), run beside the wave
//     gate. Its input is each merged unit's clone and worktree labels from the
//     run of its stages that merged (noteWorktree: every isolated call made or
//     replayed whose clone is not discarded, not a seeded one, and the
//     completion-report step's worktree, sorted so a relaunch's input is the
//     same; a repo call's clone is not a unit's, D29). Each clone or worktree is removed with every untracked, ignored and uncommitted
//     file in it (lsfrt.Runtime.RemoveWorktree, which keeps one whose branch
//     is not merged or that is not the runtime's), and its branch is deleted:
//     the unit is complete, and its replay reads the tips the checks journaled
//     (D23), not the branches. The step logs each removal's paths and each
//     kept worktree as it runs, a failed removal's included, and its value
//     lists them for a replay. A failure is logged, not signalled, except a
//     failed retained move (D33). JS's
//     cleanup agent matches worktree-wf_* branches the runtime never makes,
//     and its denials and residue were ledger rows; the removed-path list
//     takes their place. A replay after cleanup re-checks no uncommitted
//     paths: the clones and worktrees, and their dirt, are gone. A failed removal is not
//     retried.
//
// D26 and D27 are unit declared-gates'; D28 to D30 are unit s2-clones'; D31
// is unit s2-boundary-common's; D32 and D33 are unit retained-artefacts-impl's.
//
//   - D26. With declared gates a merge is gated before it is committed, so a
//     regressed merge committed nothing: the wave rewrites the
//     merge-regression statement to "merge of X failed its gates (…);
//     nothing committed", and a merge regressed because the runtime could
//     not run a gate (mergeResult.GateNotRun) to a gate-not-run stop.
//     plan.SignalsFromMerge keeps the JS statement ("was clean but the test
//     suite failed …; merge undone"), which describes JS's merge agent; an
//     undeclared Go merge never reports regressed. A merge step result with a
//     regressed unit is not journaled: a relaunch reruns the step, adopting
//     what it committed and gating the regressed unit's merge again: its
//     record replays for an unchanged tree, and a moved integration branch
//     (a unit the step merged first, a ledger commit) gives a new one.
//     The rerun is interrupted (the step's intent is left), which the
//     start checks treat as a crash recovery. In either mode, each unit the merge step
//     does not merge (regressed, unresolvable, refused) is journaled as a
//     nested step merge-outcome:<label>:<unit>, so a rerun resuming a later
//     unit's merge in progress replays it rather than fail for want of a
//     merge to adopt; the record is read only for a unit before such a
//     merge, so a unit re-evaluated after a kill between merges is gated
//     afresh against the moved branch. Every merge commit the step lands or
//     adopts must have a gated tree (gatedMerge): its own run, replayed, or
//     the unit's under the reuse rule; a commit no run covers (one that
//     raced the merge, or a hook's change) is gated then, and a failing one
//     is a stage-fault, left for the requester to revert. A stop during a
//     resolved merge's gates aborts the merge, as a stop in its conflict
//     call does; a kill leaves it in progress for the resume.
//   - D27. A plan may declare its gate commands in a ```yaml lsf-gates block
//     (docs/plan-graph.md, plan.ParseDeclaration), which the frozen JS
//     ignores. The launch pins the declaration before any agent call (D31);
//     a load whose block differs is a declared-gates-changed stop (the
//     relaunch takes the new block). A block holding hosts alone declares no
//     gates (Declaration.Gated) and is handled as no block, except that it
//     is pinned like any other. With gate commands the runtime runs the gates
//     itself (lsfrt.Runtime.RunGates) on a clean detached checkout of a
//     non-design unit's implement commit, of each code fix's commit, and of
//     each merge's tree before its commit (merge.go), once per tree: a run
//     is shared within the launch and journaled as gates:<tree[:12]> (a
//     stable label, not made launch-unique), and a set with a gate not run
//     is neither kept nor journaled, so it reruns. A red set is kept and
//     replays. Reviewers and the completion report read the runtime's
//     results, the reviewers with each failing gate's output tail. Implement
//     and code-fix prompts name the declared gates and ask for an empty
//     `gates`: a stage's report of a declared command with another status is
//     a gate-misreported record, and a declared gate it omits or reports
//     not-run raises no gate-omitted or gate-not-run. A runtime not-run is
//     gate-not-run; a gate run that cannot start (an unresolvable tree, a
//     failed checkout) is a stage-fault. A merge that differs
//     from the unit's last gated tree only at its completion report reuses
//     that result. Design units run no unit-level gates; their merges are
//     gated. JS has none of this; an undeclared plan's prompts and steps are
//     unchanged (promptparity), the gates-pin step aside (D31).
//   - D28. Every agent call runs in its own clone under the state dir
//     (lsfrt: clone.go), as docs/design/lsf-runtime.md Permissions →
//     Placement assigns it: a role's clone is kept (fetched back to
//     lsf/<plan8>/<key> after each return) or discarded (removed after the
//     call, with no uncommitted check). A repo call's clone is on the
//     integration branch, and a kept one has CallResult.Branch as an
//     isolated call's does. The uncommitted check reads only a kept clone,
//     every dirty path in which is the call's. A replay re-fetches a kept clone's branch while the clone
//     exists. JS's calls run in worktrees or the main checkout.
//   - D29. A writer repo call (direct-edits, decision-replan, replan,
//     termination) reaches the integration branch only by the ff: step
//     (ff.go), run when it returned a value that decodes with its clone clean
//     and checked: the main checkout must be on the branch, clean (no staged
//     or unstaged change to a tracked file, no merge, cherry-pick or revert in
//     progress), and behind the fetched tip, and the clone's HEAD the tip;
//     the step fast-forwards, then the clone and branch go. A fault, a held
//     commit (uncommitted changes in the clone) or a writer on a discarded
//     role is a stage-fault and the value null, with no agent-null beside it
//     (nullUnlessFaulted); the clone stays. A reviewer repo call's kept
//     clone is removed after it. The merge step runs in <stateDir>/merge/w<n>
//     (lsfrt.Runtime.MergeClone): the main checkout is checked first, a reused
//     clone is synced (commits past main must be this step's, adopted unit by
//     unit), each verified and gated commit crosses to main by
//     refs/lsf/<plan8>/xfer/w<n> and is fast-forwarded to, the conflict call
//     runs in the clone (Opts.Dir), and the clone goes when the wave's merges
//     finish. A conflict call that changes the clone's git config, hooks or
//     info files is a fault before any more git runs there; the step's git
//     runs with core.fsmonitor=false. A writer, isolated or not, on a
//     discarded role is a stage-fault. A requester edit in the main checkout faults at the next
//     fast-forward instead of making a merge unresolvable. JS merges, and its
//     repo stages commit, in the main checkout.
//   - D30. The prompts describe the clones: isolated stages run in a clone
//     with every named branch local, which the runtime fetches back; writer
//     repo calls in a clone on the integration branch, fast-forwarded after
//     they return; the commit-by-name clause has no main-checkout wording; the
//     conflict call is in the merge clone. promptparity rewrites the JS prompts
//     by its d30 table. The wording is part of those calls' input hash.
//   - D31. The gate block is pinned from the plan at args.planSha by the code
//     step gates-pin (input {sha, planPath}), run after the carried rows are
//     registered and before their scribe, the decision log and every agent
//     call. Its value is the plan document; Run parses it, so a replay pins
//     alike. A block with problems, or the step's fault or null, stops the
//     run at load. The pin's hosts go to the runtime (Runtime.PinHosts), for
//     the boundary that confines calls. Every load, the first included,
//     compares its block with the pin, so a decision re-plan that edits the
//     block stops at the first load with declared-gates-changed; the
//     decision re-plan's prompt, like the wave re-plan's, says not to edit a
//     block the plan has. A requester changes the block by committing the
//     plan and relaunching at the new SHA. The termination prompt says so
//     too. A plan with no block gets no such clause (an undeclared plan's
//     prompts are JS's), so a re-plan that adds one stops the run at the
//     next load. The pin reports gate-block problems only; the plan block's
//     are the first load's. JS has no pin; promptparity leaves the step out.
//   - D32. A unit may declare `retains: [paths]` in its lsf-plan entry, and
//     the gate block a `store:` (plan.Unit.Retains, plan.Declaration.Store);
//     the frozen JS rejects both as unknown keys. A block holding store
//     alone declares no gates and no hosts. The store is pinned with the
//     block (a change is declared-gates-changed) and checked at every
//     launch's pin (lsfrt.Runtime.CheckStore: an existing directory clear
//     of the repository and the state dir; otherwise an invalid-plan load
//     stop). Every load checks the plan's retains against the pinned block
//     (plan.CheckRetains: retains need the store, and a retaining unit's id
//     must name one directory), so a re-plan cannot add retains the pin
//     has no store for. The retains of the plan at args.planSha, as the pin
//     read it, are a baseline: a load that drops a path from an incomplete
//     unit's retains is a retains-changed stop (a Go-only stop kind); an
//     added path joins the baseline. A unit no longer in the plan is not
//     compared. An undeclared plan's steps and prompts are unchanged.
//   - D33. Cleanup moves each merged unit's retains to <store>/<unit>/
//     before removing its clones (lsfrt.Runtime.MoveRetained), from its
//     writer clones (writerSources: plan, fix-plan:rN, implement,
//     fix-code:rN, oldest first). The cleanup input carries each unit's
//     retains and the store, both omitted when empty, so an undeclared
//     plan's input, and its journal, are as before. The step's value lists
//     each move. A failed move keeps every clone of that unit, fails the
//     step, and is a retained-move-failed record (a Go-only record kind)
//     naming the clones and the by-hand remedy; a stop during it stops. A
//     path in no clone is logged as missing, not signalled. JS's cleanup
//     agent retains nothing.
package orchestrator
