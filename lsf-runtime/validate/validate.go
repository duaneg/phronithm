// Package validate checks a plan document before launch: the load stage's
// checks, run by the orchestrator's own loader, and the agreement of the
// `## Graph` block with the prose entries it mirrors, which nothing at load
// compares. Command lsf-validate runs it.
package validate

import "github.com/duaneg/phronithm/lsf-runtime/plan"

// Check runs the load stage's checks and the entry agreement check on a
// plan document, in the load stage's order. decisions is nil when there is
// no decision log.
func Check(planMarkdown string, decisions *string) []plan.Problem {
	g := plan.LoadGraph(planMarkdown, decisions)
	problems := append([]plan.Problem{}, g.Problems...)
	_, ps := plan.ParseDeclaration(planMarkdown)
	problems = append(problems, ps...)
	if g.Plan != nil {
		problems = append(problems, plan.ValidatePlan(g.Plan)...)
		problems = append(problems, Agreement(planMarkdown, g.Plan)...)
	}
	return problems
}
