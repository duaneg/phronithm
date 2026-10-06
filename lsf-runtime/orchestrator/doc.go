// Package orchestrator is the wave loop and its prompt builders, ported
// from workflows/lsf/implement.js, which stays the behavioural reference
// until script-retirement. Run is the JS run(runtime); Body adapts it to
// lsfrt.Run. tools/promptparity runs the same scenarios through both and
// compares every prompt, option, log and return byte for byte.
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
//     with a TypeError. A schema integer accepts what the runtime validator
//     does: a finite integral number, 3.0 included, within ±2^53. A missing
//     field decodes to its zero value, where JS reads undefined, and
//     encoding/json matches keys case-insensitively. Under the runtime,
//     schema validation rejects such returns before the orchestrator sees
//     them (retried, then an unjournaled null), so this arises from args.seed
//     returns, which bypass validation in both languages (a journal replay
//     was validated when recorded): a seed is trusted, as the caller vouches
//     for it.
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
//     Agent's only error is a runtime stop, which ends Run at once; a panic
//     in a unit's stages is that unit's null (Parallel recovers it,
//     TestPanickingUnitIsUnitNull), and a panic elsewhere ends the launch
//     crashed.
package orchestrator
