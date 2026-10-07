package main

import (
	"fmt"
	"strings"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

type (
	M  = orchtest.M
	H  = orchtest.Handler
	Hs = map[string]orchtest.Handler
	C  = orchtest.Call
	S  = orchtest.State
)

var (
	R     = orchtest.Ret
	unit  = orchtest.Unit
	graph = orchtest.Graph
)

// A scenario is one launch: a plan, handler overrides and extra args. It is
// built fresh for each run, so handler state starts clean.
type scenario struct {
	name  string
	build func() (*plan.Plan, Hs, M)
}

// odd is string data with the characters JSON encoders disagree on.
const odd = "x <y> & z \u2028 \u2029 \x01 \x7f \U0001F600 `tick` \"q\" \\ \nline2"

func merge(hs ...Hs) Hs {
	out := Hs{}
	for _, h := range hs {
		for k, v := range h {
			out[k] = v
		}
	}
	return out
}

func null(C, *S) any { return nil }

// once is a handler that returns first on its first call and then on later
// calls.
func once(first, then func() any) H {
	n := 0
	return func(C, *S) any {
		n++
		if n == 1 {
			return first()
		}
		return then()
	}
}

func findings(fs ...plan.Finding) M {
	return R(M{"findings": fs, "systemic": false, "summary": "s"})
}

func critical() any { return findings(orchtest.Finding("Critical", "bad "+odd, "a.py:1")) }
func clean() any    { return orchtest.NoFindings() }

func implemented(over M) H {
	return func(c C, s *S) any { return R(orchtest.Implemented(c.Unit, over)) }
}

func gates(gs ...orchtest.Gate) []orchtest.Gate { return gs }

func g(cmd, status, summary string) orchtest.Gate {
	return orchtest.Gate{Command: cmd, Status: status, Summary: summary}
}

func completeFrom(s *S, sha string) *plan.Plan {
	np := *s.Plan
	np.Sha = sha
	np.Units = nil
	for _, u := range s.Plan.Units {
		cu := *u
		cu.Complete = s.Completed[u.ID]
		np.Units = append(np.Units, &cu)
	}
	return &np
}

func replanWith(edits []orchtest.DirectEdit, verdict string) H {
	n := 0
	return func(c C, s *S) any {
		n++
		s.Plan = completeFrom(s, fmt.Sprintf("replan%d", n))
		e := []orchtest.DirectEdit{}
		if n == 1 {
			e = edits
		}
		return R(M{"planSha": s.Plan.Sha, "amendments": []M{{"kind": "new-edge", "unit": "B", "statement": "B after A"}},
			"overBudget": []M{{"unit": "B", "disposition": "flagged", "statement": "wall-clock"}},
			"verdicts":   orchtest.VerdictsFor(c.Prompt, verdict), "directEdits": e, "summary": "absorbed"})
	}
}

func discovering() Hs {
	return Hs{"implement": implemented(M{"discoveries": []M{{"statement": "note " + odd, "affects": "B"}}})}
}

func mergeMoving(arch map[int]string, withSha bool) H {
	n := 0
	return func(_ C, s *S) any {
		n++
		in := map[string]bool{}
		for _, id := range s.MergeOrder {
			in[id] = true
		}
		np := *s.Plan
		np.Sha = fmt.Sprintf("merged%d", n)
		np.Units = nil
		for _, u := range s.Plan.Units {
			cu := *u
			cu.Complete = s.Completed[u.ID] || in[u.ID]
			np.Units = append(np.Units, &cu)
		}
		s.Plan = &np
		ms := []plan.Merge{}
		for _, id := range s.MergeOrder {
			m := plan.Merge{Unit: id, Status: "clean", Files: []string{}}
			if withSha {
				m.Sha = "m-" + id
			}
			ms = append(ms, m)
		}
		r := M{"integrationBranch": "integration", "merges": ms, "planSha": np.Sha, "pauseRequested": false}
		if a, ok := arch[n]; ok {
			r["architectureSha"] = a
		}
		return R(r)
	}
}

func loadWith(extra M) H {
	return func(_ C, s *S) any { return orchtest.Copied(s.Plan, extra) }
}

func decisionsBlock(clears string) string {
	lines := []string{"```yaml lsf-decisions", "decisions:", "  - id: d1", "    date: 2026-09-19", "    question: q", "    decision: yes"}
	if clears != "" {
		lines = append(lines, "    clears: ["+clears+"]")
	}
	return strings.Join(append(lines, "```"), "\n")
}

// decision is an args decision in the schema's key order (the Go
// orchestrator's D11: prompt JSON follows the schema, not the caller).
type decision struct {
	ID       string    `json:"id"`
	Question string    `json:"question"`
	Decision string    `json:"decision"`
	Clears   *[]string `json:"clears,omitempty"`
}

func clears(xs ...string) *[]string {
	out := append([]string{}, xs...)
	return &out
}

func typed(id, t string) *plan.Unit { return orchtest.Typed(unit(id), t) }

var scenarios = []scenario{
	{"clean two waves", func() (*plan.Plan, Hs, M) { return orchtest.TwoWaves(), nil, nil }},
	{"discovery re-plans; re-plan direct edits; flagged over budget", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), merge(discovering(), Hs{"replan": replanWith([]orchtest.DirectEdit{{Path: "src/A/README.md", Description: "note " + odd}}, "amended")}), nil
	}},
	{"plan and code fixes with odd strings; findings; full completion report", func() (*plan.Plan, Hs, M) {
		return graph("plan0", unit("A")), Hs{
			"plan": func(c C, _ *S) any {
				return R(M{"branch": "plan-" + c.Unit, "sha": "p", "path": "docs/plans/" + c.Unit + "-plan.md", "bytes": 40000, "readings": []M{{"clause": "c", "reading": "r " + odd}}})
			},
			"critique-plan": once(critical, clean),
			"implement": implemented(M{
				"gateProbe": []M{{"name": "node", "pin": "22", "observed": "22", "drifted": false}}, "writesOutsideScope": []string{"CLAUDE.md"},
				"deviations": []string{"dev " + odd}, "deferrals": []M{{"statement": "later", "belongsTo": "B"}},
				"standingObligations": []M{{"statement": "ob", "status": "discharged"}}, "summary": "",
				"distress": M{"contextNearExhaustion": true, "midImplementationDesignDecisions": 4, "deviations": 3, "deferrals": 2},
			}),
			"critique-code": once(critical, func() any {
				return findings(orchtest.Finding("Significant", "sig "+odd, "a.py:2"), orchtest.Finding("Minor", "minor", "a.py:3"))
			}),
			"fix-code": func(c C, _ *S) any {
				return R(M{"branch": "fixed-code-" + c.Unit, "sha": "f", "applied": []M{{"finding": "bad", "fix": "ok"}}, "notApplied": []any{},
					"gates": gates(g("pytest -q", "red", "1 failed"), g("ruff check .", "green", ""))})
			},
		}, nil
	}},
	{"design unit fix", func() (*plan.Plan, Hs, M) {
		return graph("plan0", typed("D", "design")), Hs{
			"implement":       implemented(M{"gates": []any{}}),
			"critique-design": once(critical, clean),
			"fix-code": func(c C, _ *S) any {
				return R(M{"branch": "fixed-" + c.Unit, "sha": "f", "applied": []M{{"finding": "bad", "fix": "ok"}}, "notApplied": []any{}})
			},
		}, nil
	}},
	{"lenses by type", func() (*plan.Plan, Hs, M) {
		return graph("plan0", typed("R", "refactor"), typed("P", "polish"), typed("I", "integration"), typed("G", "bug-fix"), typed("T", "meta")), nil, M{"maxUnitsPerWave": 3}
	}},
	{"wave stop still re-plans, with stops in the prompt", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoParallel(), merge(discovering(), Hs{"merge": func(C, *S) any {
			return R(M{"integrationBranch": "integration", "merges": []plan.Merge{{Unit: "A", Status: "unresolvable", Files: []string{"src/x.js"}}, {Unit: "B", Status: "clean", Files: []string{}, Sha: "mb"}}, "planSha": "m1"})
		}}), nil
	}},
	{"resolved conflicts reach the re-plan", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"merge": func(_ C, s *S) any {
			ms := []plan.Merge{}
			for _, id := range s.MergeOrder {
				ms = append(ms, plan.Merge{Unit: id, Status: "resolved", Files: []string{"CLAUDE.md"}, Sha: "m"})
			}
			s.Plan = completeFrom(s, "m1")
			return R(M{"integrationBranch": "integration", "merges": ms, "planSha": "m1"})
		}}, nil
	}},
	{"decisions with and without clears; decision re-plan direct edits", func() (*plan.Plan, Hs, M) {
		u := unit("A")
		u.Gates = []plan.Gate{{Name: "legal", Kind: "organisational"}}
		return graph("plan0", u, orchtest.Deps(unit("B"), "A")), Hs{
			"load": loadWith(M{"decisionsBlock": decisionsBlock("A/legal")}),
			"decision-replan": func(c C, _ *S) any {
				return R(M{"planSha": "decided1", "amendments": []any{}, "overBudget": []any{},
					"verdicts":    []M{{"id": "d1", "verdict": "amended", "statement": "ok"}, {"id": "d2", "verdict": "document-text", "statement": "§2 reworded"}, {"id": "d3", "verdict": "no-change", "statement": "already"}},
					"directEdits": []orchtest.DirectEdit{{Path: "docs/debt.md", Description: "file " + odd}}, "summary": "s"})
			},
		}, M{"decisions": []decision{
			{"d1", "q " + odd, "yes", clears("A/legal")},
			{"d2", "q2", "no", clears()},
			{"d3", "q3", "maybe", nil},
		}}
	}},
	{"termination appends a meta unit and direct edits", func() (*plan.Plan, Hs, M) {
		done := false
		return graph("plan0", unit("A")), Hs{"termination": func(_ C, s *S) any {
			if done {
				return R(M{"planSha": s.Plan.Sha, "appendedUnits": []any{}, "directEdits": []any{}})
			}
			done = true
			np := *s.Plan
			np.Sha, np.Units, np.ExecutionOrder = "term1", append(append([]*plan.Unit{}, s.Plan.Units...), orchtest.Deps(typed("M", "meta"), "A")), append(append([]string{}, s.Plan.ExecutionOrder...), "M")
			s.Plan = &np
			return R(M{"planSha": "term1", "appendedUnits": []string{"M"}, "directEdits": []orchtest.DirectEdit{{Path: "README.md", Description: "mention M"}}})
		}}, nil
	}},
	{"architecture moved with per-unit diffs; amendment and gate notes", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{
			"merge": mergeMoving(map[int]string{1: "arch2", 2: "arch2"}, true),
			"review-gate": func(c C, _ *S) any {
				as := []M{}
				if strings.Contains(c.Prompt, "changed during this wave") {
					as = append(as, M{"unit": "A", "section": "§7.2", "statement": "settled"})
				}
				return R(M{"invalidations": []M{{"section": "§1", "decision": "d", "premise": "p", "evidence": "e", "amendment": ""}}, "notes": []M{{"statement": "note", "referent": "§2"}}, "amendments": as})
			},
			"replan": replanWith([]orchtest.DirectEdit{}, "no-change"),
		}, nil
	}},
	{"merge without SHA against an args pin", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"merge": mergeMoving(map[int]string{1: "arch2"}, false)}, M{"architectureSha": "argpin"}
	}},
	{"unpinned document, with and without merge SHAs", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoParallel(), Hs{"load": loadWith(M{"architectureSha": ""}), "merge": mergeMoving(map[int]string{1: "arch2", 2: "arch3"}, false)}, nil
	}},
	{"unpinned document with merge SHAs", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"load": loadWith(M{"architectureSha": ""}), "merge": mergeMoving(map[int]string{1: "arch2"}, true)}, nil
	}},
	{"complete invalidation stops", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"review-gate": func(C, *S) any {
			return R(M{"invalidations": []M{{"section": "§4", "decision": "d", "premise": "p", "evidence": "e " + odd, "amendment": "a"}}, "notes": []any{}})
		}}, nil
	}},
	{"wrong branch at load; ledger without commit at wave 0", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"load": loadWith(M{"headBranch": "main"}), "ledger": func(C, *S) any { return R(M{"path": "x", "sha": ""}) }}, nil
	}},
	// The merge is a code step in Go (D24), which ignores a seed, so the
	// seed that stood in for the merge agent is gone; the pause a seed
	// reports is stripped on both sides (the obligations seed's).
	{"carried rows; seeded plan and pausing obligations", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"ledger": null}, M{
			"unscribed": []M{{"kind": "deviation", "source": "reported", "stage": "implement", "unit": "Z", "wave": 3, "statement": "dev-Z", "referent": "src/Z", "raisedBy": "implement", "disposition": "recorded"}},
			"seed": M{
				"plan:A":      R(M{"branch": "salvaged-A", "sha": "s", "path": "docs/plans/A-plan.md", "bytes": 0, "readings": []any{}}),
				"obligations": R(M{"obligations": []any{}, "pauseRequested": true}),
			},
		}
	}},
	{"pause at merge with a null scribe", func() (*plan.Plan, Hs, M) {
		base := orchtest.Defaults()["merge"]
		return orchtest.TwoWaves(), Hs{
			"merge": func(c C, s *S) any {
				r := base(c, s).(M)
				r["result"].(M)["pauseRequested"] = true
				return r
			},
			"implement": implemented(M{"deviations": []string{"d"}}),
			"ledger":    null,
		}, M{"pauseRef": "refs/custom/pause"}
	}},
	{"wave cap", func() (*plan.Plan, Hs, M) { return orchtest.TwoWaves(), nil, M{"maxWaves": 1} }},
	{"systemic review; tiers override", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"critique-plan": func(C, *S) any {
			return R(M{"findings": []any{}, "systemic": true, "summary": "structural " + odd})
		}}, M{"tiers": M{"critique": M{"model": "sonnet"}, "planning": nil}}
	}},
	{"declined Critical", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{
			"critique-plan": once(critical, clean),
			"fix-plan": func(c C, _ *S) any {
				return R(M{"branch": "f", "sha": "f", "applied": []any{}, "notApplied": []M{{"finding": "bad", "reason": "outside"}}})
			},
		}, nil
	}},
	{"hard cap", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"critique-plan": func(C, *S) any { return critical() }}, nil
	}},
	{"gated and waiting units", func() (*plan.Plan, Hs, M) {
		u := unit("A")
		u.Gates = []plan.Gate{{Name: "legal sign-off", Kind: "organisational"}}
		return graph("plan0", u, orchtest.Deps(unit("B"), "A")), nil, nil
	}},
	{"cannot verdicts and an unrepaired unit", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), merge(discovering(), Hs{"replan": func(c C, _ *S) any {
			return R(M{"planSha": "rp", "amendments": []any{}, "overBudget": []M{{"unit": "B", "disposition": "unrepaired", "statement": "too big"}},
				"verdicts": []M{{"id": "rp-1", "verdict": "cannot", "statement": "needs the requester"}}, "directEdits": []any{}, "summary": "s"})
		}}), nil
	}},
	{"carried and undischarged obligations", func() (*plan.Plan, Hs, M) {
		return graph("plan0", unit("A")), Hs{"obligations": func(C, *S) any {
			return R(M{"obligations": []M{
				{"statement": "perf", "disposition": "carried", "carrier": "docs/debt.md row 7", "evidence": "e"},
				{"statement": "docs", "disposition": "carried", "carrier": "  ", "evidence": "e"},
				{"statement": "tests", "disposition": "discharged", "carrier": "", "evidence": "e"},
			}})
		}}, nil
	}},
	{"drift, unevaluable, gate not run and a null stage across units", func() (*plan.Plan, Hs, M) {
		return graph("plan0", unit("A"), unit("B"), unit("C"), unit("D"), unit("E")), Hs{
			"implement": func(c C, _ *S) any {
				switch c.Unit {
				case "A":
					return R(orchtest.Implemented("A", M{"gateProbe": []M{{"name": "node", "pin": "22", "observed": "20", "drifted": true}}}))
				case "B":
					return R(orchtest.Implemented("B", M{"doneCondition": M{"evaluable": false, "met": false, "evidence": "no such test"}}))
				case "C":
					return R(orchtest.Implemented("C", M{"gates": gates(g("xtask ci", "not-run", "timeout"))}))
				case "D":
					return nil
				}
				return R(orchtest.Implemented(c.Unit, nil))
			},
			"completion-report": func(c C, _ *S) any {
				if c.Unit == "E" {
					return nil
				}
				return R(M{"branch": "done-" + c.Unit, "sha": "c", "path": "docs/plans/" + c.Unit + "-completion.md"})
			},
		}, nil
	}},
	{"code fix drops a gate", func() (*plan.Plan, Hs, M) {
		return graph("plan0", unit("A")), Hs{
			"critique-code": once(critical, clean),
			"fix-code": func(c C, _ *S) any {
				return R(M{"branch": "fx", "sha": "f", "applied": []M{{"finding": "bad", "fix": "ok"}}, "notApplied": []any{}, "gates": gates(g("pytest -q", "green", "1"))})
			},
		}, nil
	}},
	{"null plan, null merge, null re-plan and null termination", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), merge(discovering(), Hs{"replan": null}), nil
	}},
	{"null merge", func() (*plan.Plan, Hs, M) { return orchtest.TwoWaves(), Hs{"merge": null}, nil }},
	{"null termination", func() (*plan.Plan, Hs, M) { return graph("plan0", unit("A")), Hs{"termination": null}, nil }},
	{"null plan stage and cleanup", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoParallel(), Hs{"plan": func(c C, _ *S) any {
			if c.Unit == "B" {
				return nil
			}
			return R(M{"branch": "plan-" + c.Unit, "sha": "p", "path": "docs/plans/" + c.Unit + "-plan.md", "bytes": 1, "readings": []any{}})
		}, "cleanup": null}, nil
	}},
	{"bad plan block", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"load": func(C, *S) any {
			return R(M{"planBlock": "units: [", "decisionsBlock": "", "headBranch": "integration", "architectureSha": "a"})
		}}, nil
	}},
	{"decision log null", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"decision-log": null}, M{"decisions": []decision{{"d1", "q", "yes", nil}}}
	}},
	{"decision log without a commit", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"decision-log": func(C, *S) any { return R(M{"path": "p", "sha": ""}) }}, M{"decisions": []decision{{"d1", "q", "yes", nil}}}
	}},
	{"null decision re-plan", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"decision-replan": null}, M{"decisions": []decision{{"d1", "q", "yes", nil}}}
	}},
	{"decisions applied, then the wave-0 scribe leaves no commit", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"ledger": func(C, *S) any { return R(M{"path": "x", "sha": ""}) }}, M{"decisions": []decision{{"d1", "q", "yes", nil}}}
	}},
	{"null load", func() (*plan.Plan, Hs, M) { return orchtest.TwoWaves(), Hs{"load": null}, nil }},
	{"carried rows, a null scribe, a pause at load, then a scribe without a commit", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{
			"load":   loadWith(M{"pauseRequested": true}),
			"ledger": once(func() any { return nil }, func() any { return R(M{"path": "x", "sha": ""}) }),
		}, M{"unscribed": []M{{"kind": "deviation", "source": "reported", "stage": "implement", "statement": "dev", "referent": "r", "raisedBy": "implement", "disposition": "recorded"}}}
	}},
	{"carried rows whose scribe leaves no commit", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"ledger": func(C, *S) any { return R(M{"path": "x", "sha": ""}) }},
			M{"unscribed": []M{{"kind": "deviation", "source": "reported", "stage": "implement", "statement": "dev", "referent": "r", "raisedBy": "implement", "disposition": "recorded"}}}
	}},
	{"termination reload fails validation; a pre-completed unit", func() (*plan.Plan, Hs, M) {
		a := unit("A")
		a.Complete = true
		return graph("plan0", a, orchtest.Deps(unit("B"), "A")), Hs{"load": func(c C, s *S) any {
			if c.Label == "load:termination" {
				x, y := unit("X"), unit("Y")
				x.Scope, y.Scope = []string{"src"}, []string{"src/y"}
				return orchtest.Copied(graph("t", x, y), nil)
			}
			return orchtest.Copied(s.Plan, nil)
		}}, nil
	}},
	{"termination direct edits null", func() (*plan.Plan, Hs, M) {
		return graph("plan0", unit("A")), Hs{
			"termination": func(_ C, s *S) any {
				return R(M{"planSha": s.Plan.Sha, "appendedUnits": []any{}, "directEdits": []orchtest.DirectEdit{{Path: "a", Description: "b"}}})
			},
			"direct-edits": null,
		}, nil
	}},
	{"termination direct edits without a commit", func() (*plan.Plan, Hs, M) {
		return graph("plan0", unit("A")), Hs{
			"termination": func(_ C, s *S) any {
				return R(M{"planSha": s.Plan.Sha, "appendedUnits": []any{}, "directEdits": []orchtest.DirectEdit{{Path: "a", Description: "b"}, {Path: "c", Description: "d"}}})
			},
			"direct-edits": func(C, *S) any { return R(M{"path": "a", "sha": ""}) },
		}, nil
	}},
	{"null wave gate", func() (*plan.Plan, Hs, M) { return orchtest.TwoWaves(), Hs{"review-gate": null}, nil }},
	{"records only, scribe without a commit", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"implement": implemented(M{"deviations": []string{"d"}}), "ledger": func(C, *S) any { return R(M{"path": "x", "sha": ""}) }}, nil
	}},
	{"re-plan wave, scribe without a commit", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), merge(discovering(), Hs{"ledger": func(C, *S) any { return R(M{"path": "x", "sha": ""}) }}), nil
	}},
	{"null obligations", func() (*plan.Plan, Hs, M) { return graph("plan0", unit("A")), Hs{"obligations": null}, nil }},
	{"carried obligation, final scribe without a commit", func() (*plan.Plan, Hs, M) {
		return graph("plan0", unit("A")), Hs{
			"obligations": func(C, *S) any {
				return R(M{"obligations": []M{{"statement": "perf", "disposition": "carried", "carrier": "row 7", "evidence": "e"}}})
			},
			"ledger": func(C, *S) any { return R(M{"path": "x", "sha": ""}) },
		}, nil
	}},
	{"obligations with no carrier field, and undischarged", func() (*plan.Plan, Hs, M) {
		return graph("plan0", unit("A")), Hs{"obligations": func(C, *S) any {
			return R(M{"obligations": []M{{"statement": "a", "disposition": "carried", "evidence": "e"}, {"statement": "b", "disposition": "undischarged", "carrier": "", "evidence": "e"}}})
		}}, nil
	}},
	{"null plan critique and null plan fix", func() (*plan.Plan, Hs, M) {
		return graph("plan0", unit("A"), unit("B")), Hs{
			"critique-plan": func(c C, _ *S) any {
				if c.Unit == "A" {
					return nil
				}
				return critical()
			},
			"fix-plan": null,
		}, nil
	}},
	{"implement reports no gates; done condition not met", func() (*plan.Plan, Hs, M) {
		return graph("plan0", unit("A"), unit("B")), Hs{"implement": func(c C, _ *S) any {
			if c.Unit == "A" {
				return R(orchtest.Implemented("A", M{"gates": []any{}}))
			}
			return R(orchtest.Implemented("B", M{"doneCondition": M{"evaluable": true, "met": false, "evidence": "half"}}))
		}}, nil
	}},
	{"code fix with no gates field", func() (*plan.Plan, Hs, M) {
		return graph("plan0", unit("A")), Hs{
			"critique-code": once(critical, clean),
			"fix-code":      func(C, *S) any { return R(M{"branch": "fx", "sha": "f", "applied": []any{}, "notApplied": []any{}}) },
		}, nil
	}},
	{"re-plan return missing lists; an unknown disposition", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), merge(discovering(), Hs{"replan": func(_ C, s *S) any {
			s.Plan = completeFrom(s, "rp")
			return R(M{"planSha": "rp", "overBudget": []M{{"unit": "B", "disposition": "weird", "statement": "s"}}, "summary": "s"})
		}}), nil
	}},
	{"plan that loads but fails validation", func() (*plan.Plan, Hs, M) {
		a, b := unit("A"), unit("B")
		a.Scope, b.Scope = []string{"src"}, []string{"src/b"}
		return graph("plan0", a, b), nil, nil
	}},
	{"a decision the re-plan cannot carry out", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), Hs{"decision-replan": func(C, *S) any {
			return R(M{"planSha": "decided1", "amendments": []any{}, "overBudget": []any{}, "verdicts": []M{{"id": "d1", "verdict": "cannot", "statement": "changes §4"}}, "directEdits": []any{}, "summary": "s"})
		}}, M{"decisions": []decision{{"d1", "q", "rewrite", nil}}}
	}},
	{"re-plan return with no over-budget list", func() (*plan.Plan, Hs, M) {
		return orchtest.TwoWaves(), merge(discovering(), Hs{"replan": func(c C, s *S) any {
			s.Plan = completeFrom(s, "rp")
			return R(M{"planSha": "rp", "amendments": []any{}, "verdicts": orchtest.VerdictsFor(c.Prompt, "amended"), "summary": "s"})
		}}), nil
	}},
	{"required args", func() (*plan.Plan, Hs, M) { return orchtest.TwoWaves(), nil, M{"architectureDoc": ""} }},
}
