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
//     arises from args.seed returns, which bypass validation in both
//     languages (a journal replay was validated when recorded): a seed is
//     trusted, as the caller vouches for it.
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
//     bytes written> trailers; a stage commit whose content no longer
//     matches its digest is not intact. An interrupted rerun adopts its own
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
//     its statement; no scenario has both.
//     declared-gates raises gate-omitted for a declared gate missing.
package orchestrator
