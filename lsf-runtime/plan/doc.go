// Package plan is the plan model: the plan graph, its YAML-subset parser,
// the shared stage schema, and the scheduling and stop rules. Ported from
// workflows/lsf/schema.js, graph.js and scheduling.js, which stay the
// behavioural reference until script-retirement. tools/graphparity runs
// both over the fixtures in testdata and the repository's plan blocks and
// compares the JSON: the parser and loader, plan validation and the
// scheduling views, block extraction, the signal mappers, stop rule, digest
// and ledger, and the exported constants.
//
// Signals are *Signal: the orchestrator relies on identity. EvaluateWave
// and ApplyClearances write through their arguments; a Plan and its
// signals are not safe for concurrent use.
//
// NewSignal, StageSchema and the JS-semantics helpers panic on a
// programmer error (an empty kind, an unknown source, a non-object result
// schema), where the JS throws. Everything driven by input returns an
// error or a Problem.
//
// # Divergences from the JavaScript
//
// D1 to D4 arise only on a plan block that loads with problems.
//
//   - D1. A YAML key naming an Object.prototype member (constructor,
//     toString, __proto__, valueOf, …) is a "duplicate key" in JS, which
//     tests `key in out`. Go reports only a real repeat.
//   - D2. readySet and blockedUnits throw on a dependency naming an unknown
//     unit, and readySet and mergeOrder sort with a NaN comparator for an id
//     missing from the execution order. Go treats the dependency as
//     incomplete and sorts the id last, stably. ValidatePlan rejects both
//     before the loop runs.
//   - D3. JS multiline ^ and $ also break at a lone \r, U+2028 and U+2029;
//     Go's break only at \n (CRLF is handled), so ExtractBlock misses a
//     closing fence next to one of them.
//   - D4. A unit type or gate kind that is not a string, or is a quoted "",
//     is kept as is in JS. Go holds "" and omits it from JSON. An
//     invalid-plan problem with identical text always accompanies it.
//
// D5 to D8 can arise on any run.
//
//   - D5. JS distinguishes null, undefined and "" for a signal's unit,
//     raisedBy, stage, statement and disposition and for an escalation's
//     SHAs; Go has "" alone, with omitempty where JS omits undefined. A
//     finding with no unit is keyed "undefined" in the digest's counts, as
//     in JS, but a distress row with no unit is listed as "" in
//     distressedUnits where JS has null. NewEscalation makes nil lists []
//     where JS omits or keeps them. The orchestrator tests these fields for
//     truthiness only.
//   - D6. JS ledgerLabel tests wave === undefined while ledgerWave uses
//     ?? (null too). Go's Wave is a nil pointer for both; signal() never
//     stores a null wave, so the difference cannot arise.
//   - D7. encoding/json escapes <, > and &, and U+2028 and U+2029;
//     JSON.stringify escapes none of them. MarshalJS (and jsJSON, used in
//     problem text) match JS; json.Marshal does not.
//   - D8. Inputs are typed. Stage results decode into ImplementResult,
//     GateResult, Finding and the rest, and a wrong-typed field fails the
//     decode where JS coerces with String() or a typeof test.
//     SignalsFromReadings takes the count, not the readings. At.TargetKB is
//     an int. ConfigFrom keeps the default for a null field (JS compares
//     against null, i.e. 0) and rejects a non-integer (JS coerces).
//
// D9 onward are the orchestrator's: see orchestrator/doc.go.
//
// Not replicated: a YAML document nested thousands of levels deep, on
// which JS throws a RangeError. The parser reads Go strings as bytes; its
// inputs arrive through JSON decoding, so they are valid UTF-8.
package plan
