// Package plan is the plan model: the plan graph, its YAML-subset parser,
// the shared stage schema, and the scheduling and stop rules. Ported from
// workflows/lsf/schema.js, graph.js and scheduling.js, which stay the
// behavioural reference until script-retirement; tools/graphparity checks
// the two agree.
//
// # Divergences from the JavaScript
//
// The port follows the JavaScript except where the JavaScript is defective
// on an error path, or where a JavaScript value has no Go counterpart. None
// arises on a plan that loads without problems.
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
//   - D5. JS distinguishes null, undefined and "" for a signal's unit,
//     raisedBy, stage, statement and disposition and for an escalation's
//     SHAs; Go has "" alone, with omitempty where JS omits undefined. A
//     finding with no unit, for instance, is keyed "undefined" in the
//     digest's counts, as in JS, but listed as "" in distressedUnits where
//     JS has null. NewEscalation makes nil signals [] where JS omits them.
//     The orchestrator tests these fields for truthiness only.
//   - D6. JS ledgerLabel tests wave === undefined while ledgerWave uses
//     ?? (null too). Go's Wave is a nil pointer for both; signal() never
//     stores a null wave, so the difference cannot arise.
//   - D7. encoding/json escapes <, > and &, and U+2028 and U+2029;
//     JSON.stringify escapes none of them. MarshalJS (and jsJSON, used in
//     problem text) match JS; json.Marshal does not.
//
// Not replicated: a YAML document nested thousands of levels deep, on
// which JS throws a RangeError. The parser reads Go strings as bytes; its
// inputs arrive through JSON decoding, so they are valid UTF-8.
package plan
