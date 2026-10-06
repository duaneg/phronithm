package orchtest

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

// Unit is a feature unit with scope src/<id>, no dependencies and no gates.
func Unit(id string) *plan.Unit {
	return &plan.Unit{ID: id, Type: "feature", Dependencies: []plan.Dependency{}, Scope: []string{"src/" + id}, Publishes: []string{}, Gates: []plan.Gate{}}
}

// Graph is a plan at sha over units, in that execution order.
func Graph(sha string, units ...*plan.Unit) *plan.Plan {
	order := []string{}
	for _, u := range units {
		order = append(order, u.ID)
	}
	return &plan.Plan{Sha: sha, Path: "docs/plans/f-breakdown.md", Units: units, ExecutionOrder: order}
}

// Deps sets u's dependencies and returns u.
func Deps(u *plan.Unit, ids ...string) *plan.Unit {
	for _, id := range ids {
		u.Dependencies = append(u.Dependencies, plan.Dependency{Unit: id})
	}
	return u
}

// Typed sets u's type and returns u.
func Typed(u *plan.Unit, t string) *plan.Unit {
	u.Type = t
	return u
}

// TwoWaves is A (foundation) then B after A.
func TwoWaves() *plan.Plan {
	return Graph("plan0", Typed(Unit("A"), "foundation"), Deps(Unit("B"), "A"))
}

// TwoParallel is A and B, then C after both.
func TwoParallel() *plan.Plan {
	return Graph("plan0", Unit("A"), Unit("B"), Deps(Unit("C"), "A", "B"))
}

func q(s string) string {
	b, err := plan.MarshalJS(s, "")
	if err != nil {
		panic(err)
	}
	return string(b)
}

func list(xs []string) string {
	qs := []string{}
	for _, x := range xs {
		qs = append(qs, q(x))
	}
	return "[" + strings.Join(qs, ", ") + "]"
}

// ToBlock serialises p as the plan's fenced block, as the copy agent
// returns it.
func ToBlock(p *plan.Plan) string {
	lines := []string{"```yaml lsf-plan", "units:"}
	for _, u := range p.Units {
		deps := []string{}
		for _, d := range u.Dependencies {
			deps = append(deps, d.Unit)
		}
		lines = append(lines, "  - id: "+u.ID, "    type: "+u.Type, fmt.Sprintf("    complete: %t", u.Complete),
			"    dependencies: "+list(deps), "    scope: "+list(u.Scope), "    publishes: "+list(u.Publishes))
		if len(u.Gates) > 0 {
			lines = append(lines, "    gates:")
			for _, g := range u.Gates {
				lines = append(lines, "      - name: "+q(g.Name), "        kind: "+g.Kind)
				if g.Pin != nil && *g.Pin != "" {
					lines = append(lines, "        pin: "+q(*g.Pin))
				}
			}
		} else {
			lines = append(lines, "    gates: []")
		}
	}
	lines = append(lines, "execution_order: "+list(p.ExecutionOrder), "```")
	return strings.Join(lines, "\n")
}

// Copied is the copy stage's return for p.
func Copied(p *plan.Plan, extra M) M {
	r := M{"planBlock": ToBlock(p), "decisionsBlock": "", "headBranch": "integration", "architectureSha": "arch1", "pauseRequested": false}
	for k, v := range extra {
		r[k] = v
	}
	return Ret(r)
}

// NoDistress is an implement stage's distress with nothing to report.
func NoDistress() M {
	return M{"contextNearExhaustion": false, "midImplementationDesignDecisions": 0, "deviations": 0, "deferrals": 0}
}

// Gate is one gate run.
type Gate struct {
	Command string `json:"command"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

// GreenGates are the implement stage's default gate runs.
func GreenGates(tests string) []Gate {
	return []Gate{{"pytest -q", "green", tests}, {"ruff check .", "green", ""}}
}

// Implemented is unit u's implement result, with over applied.
func Implemented(u string, over M) M {
	r := M{
		"branch": "impl-" + u, "sha": "i", "gateProbe": []any{}, "gates": GreenGates("412 passed"),
		"distress": NoDistress(), "deviations": []any{}, "writesOutsideScope": []any{}, "deferrals": []any{}, "discoveries": []any{},
		"doneCondition": M{"evaluable": true, "met": true, "evidence": "tests pass"}, "standingObligations": []any{}, "summary": "done",
	}
	for k, v := range over {
		r[k] = v
	}
	return r
}

// NoFindings is a clean review return.
func NoFindings() M { return Ret(M{"findings": []any{}, "systemic": false, "summary": "clean"}) }

// Finding is a review finding in schema key order.
func Finding(severity, statement, location string) plan.Finding {
	return plan.Finding{Severity: severity, Statement: statement, Location: location}
}

var itemID = regexp.MustCompile(`"id": "(rp-\d+)"`)
var decisionID = regexp.MustCompile(`"id": "(d\d+)"`)

// ItemIDs are the re-plan item ids in a re-plan prompt.
func ItemIDs(prompt string) []string {
	out := []string{}
	for _, m := range itemID.FindAllStringSubmatch(prompt, -1) {
		out = append(out, m[1])
	}
	return out
}

// VerdictsFor gives every item in a re-plan prompt the same verdict.
func VerdictsFor(prompt, v string) []M {
	out := []M{}
	for _, id := range ItemIDs(prompt) {
		out = append(out, M{"id": id, "verdict": v, "statement": "ok"})
	}
	return out
}

// Discovering is an implement handler reporting one discovery, so the wave
// has an item to absorb and the re-plan runs.
func Discovering() map[string]Handler {
	return map[string]Handler{"implement": func(c Call, s *State) any {
		return Ret(Implemented(c.Unit, M{"discoveries": []M{{"statement": "note from " + c.Unit, "affects": "B"}}}))
	}}
}

// withComplete is p at sha with units complete where done says.
func withComplete(p *plan.Plan, sha string, done func(id string) bool) *plan.Plan {
	np := *p
	np.Sha = sha
	np.Units = nil
	for _, u := range p.Units {
		cu := *u
		cu.Complete = done(u.ID)
		np.Units = append(np.Units, &cu)
	}
	return &np
}

// Defaults are the handlers by label prefix, ported from the JS defaults().
func Defaults() map[string]Handler {
	return map[string]Handler{
		"load": func(c Call, s *State) any { return Copied(s.Plan, nil) },
		"plan": func(c Call, s *State) any {
			return Ret(M{"branch": "plan-" + c.Unit, "sha": "p", "path": "docs/plans/" + c.Unit + "-plan.md", "bytes": 0, "readings": []any{}})
		},
		"critique-plan": func(Call, *State) any { return NoFindings() },
		"fix-plan": func(c Call, s *State) any {
			return Ret(M{"branch": "fixed-" + c.Unit, "sha": "f", "applied": []M{{"finding": "x-" + c.Unit, "fix": "y"}}, "notApplied": []any{}})
		},
		"implement":       func(c Call, s *State) any { return Ret(Implemented(c.Unit, nil)) },
		"critique-code":   func(Call, *State) any { return NoFindings() },
		"lens-review":     func(Call, *State) any { return NoFindings() },
		"critique-design": func(Call, *State) any { return NoFindings() },
		"fix-code": func(c Call, s *State) any {
			return Ret(M{"branch": "fixed-code-" + c.Unit, "sha": "f", "applied": []any{}, "notApplied": []any{}, "gates": GreenGates("413 passed")})
		},
		"completion-report": func(c Call, s *State) any {
			return Ret(M{"branch": "done-" + c.Unit, "sha": "c", "path": "docs/plans/" + c.Unit + "-completion.md"})
		},
		"cleanup": func(Call, *State) any { return Ret(M{"removed": 0, "kept": []any{}}) },
		"merge": func(c Call, s *State) any {
			s.Seq++
			in := map[string]bool{}
			for _, id := range s.MergeOrder {
				in[id] = true
			}
			s.Plan = withComplete(s.Plan, fmt.Sprintf("merged%d", s.Seq), func(id string) bool { return s.Completed[id] || in[id] })
			merges := []plan.Merge{}
			for _, id := range s.MergeOrder {
				merges = append(merges, plan.Merge{Unit: id, Status: "clean", Files: []string{}, Sha: "m"})
			}
			return Ret(M{"integrationBranch": "integration", "merges": merges, "planSha": s.Plan.Sha, "architectureSha": "arch1", "pauseRequested": false})
		},
		"review-gate": func(Call, *State) any {
			return Ret(M{"invalidations": []any{}, "notes": []any{}, "amendments": []any{}})
		},
		"replan": func(c Call, s *State) any {
			s.Seq++
			s.Plan = withComplete(s.Plan, fmt.Sprintf("replan%d", s.Seq), func(id string) bool { return s.Completed[id] })
			return Ret(M{"planSha": s.Plan.Sha, "amendments": []any{}, "overBudget": []any{}, "verdicts": VerdictsFor(c.Prompt, "amended"), "directEdits": []any{}, "summary": "no change"})
		},
		"ledger": func(Call, *State) any {
			return Ret(M{"path": "docs/plans/f-signals.md", "sha": "l", "pauseRequested": false})
		},
		"termination": func(c Call, s *State) any {
			return Ret(M{"planSha": s.Plan.Sha, "appendedUnits": []any{}, "directEdits": []any{}})
		},
		"direct-edits": func(Call, *State) any { return Ret(M{"path": "docs/plans/f-breakdown.md", "sha": "e"}) },
		"obligations":  func(Call, *State) any { return Ret(M{"obligations": []any{}}) },
		"decision-log": func(Call, *State) any { return Ret(M{"path": "docs/plans/f-decisions.md", "sha": "dec1"}) },
		"decision-replan": func(c Call, s *State) any {
			verdicts := []M{}
			for _, m := range decisionID.FindAllStringSubmatch(c.Prompt, -1) {
				verdicts = append(verdicts, M{"id": m[1], "verdict": "amended", "statement": "edge added"})
			}
			return Ret(M{"planSha": "decided1", "amendments": []M{{"kind": "new-edge", "unit": "B", "statement": "B after A"}}, "overBudget": []any{},
				"verdicts": verdicts, "directEdits": []any{}, "summary": "applied"})
		},
	}
}
