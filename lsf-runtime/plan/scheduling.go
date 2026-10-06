package plan

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Scheduling: plan validation, the ready set, merge order and the wave plan.
// See docs/design/lsf-workflow.md, "Wave loop".

var BlockingGateKinds = []string{"organisational", "release"}

// Completed holds unit ids completed during a run. Only true entries count;
// nil is empty.
type Completed map[string]bool

func unitIndex(p *Plan) map[string]*Unit {
	byID := make(map[string]*Unit, len(p.Units))
	for _, u := range p.Units {
		byID[u.ID] = u // last duplicate wins, as Map.set does
	}
	return byID
}

// orderIndex maps id to its execution-order position; a repeated id takes
// its last position, as new Map(...) does.
func orderIndex(p *Plan) map[string]int {
	order := make(map[string]int, len(p.ExecutionOrder))
	for i, id := range p.ExecutionOrder {
		order[id] = i
	}
	return order
}

// position is id's execution-order position, +Inf when absent.
func position(order map[string]int, id string) int {
	if i, ok := order[id]; ok {
		return i
	}
	return math.MaxInt
}

// FindCycle returns a dependency cycle as a list of unit ids, first id
// repeated at the end, or nil. Edges are dependencies of every type; the
// union must be acyclic.
func FindCycle(p *Plan) []string {
	byID := unitIndex(p)
	const active, done = 1, 2
	state := map[string]int{}
	var stack []string
	var visit func(id string) []string
	visit = func(id string) []string {
		switch state[id] {
		case done:
			return nil
		case active:
			return append(slices.Clone(stack[slices.Index(stack, id):]), id)
		}
		state[id] = active
		stack = append(stack, id)
		if u := byID[id]; u != nil {
			for _, d := range u.Dependencies {
				if _, ok := byID[d.Unit]; !ok {
					continue
				}
				if cycle := visit(d.Unit); cycle != nil {
					return cycle
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = done
		return nil
	}
	for _, u := range p.Units {
		if cycle := visit(u.ID); cycle != nil {
			return cycle
		}
	}
	return nil
}

// Ancestors is the set of unit ids reachable from id along dependency
// edges, id excluded.
func Ancestors(p *Plan, id string) map[string]bool {
	byID := unitIndex(p)
	seen := map[string]bool{}
	queue := []string{id}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if u := byID[cur]; u != nil {
			for _, d := range u.Dependencies {
				if _, known := byID[d.Unit]; known && !seen[d.Unit] {
					seen[d.Unit] = true
					queue = append(queue, d.Unit)
				}
			}
		}
	}
	delete(seen, id)
	return seen
}

// splitScope splits a scope entry, a path optionally narrowed to one
// symbol: `path::symbol`.
func splitScope(entry string) (path, symbol string, hasSymbol bool) {
	path, symbol, hasSymbol = strings.Cut(entry, "::")
	return strings.TrimRight(path, "/"), symbol, hasSymbol
}

// PathsOverlap reports whether one scope entry contains or equals the
// other. A directory contains the paths under it; a bare file contains its
// symbols; two symbols of one file overlap only when they are the same
// symbol.
func PathsOverlap(a, b string) bool {
	pa, sa, ha := splitScope(a)
	pb, sb, hb := splitScope(b)
	if pa != pb {
		return strings.HasPrefix(pa, pb+"/") || strings.HasPrefix(pb, pa+"/")
	}
	return !ha || !hb || sa == sb
}

func ScopesOverlap(a, b *Unit) bool {
	for _, pa := range a.Scope {
		for _, pb := range b.Scope {
			if PathsOverlap(pa, pb) {
				return true
			}
		}
	}
	return false
}

// Pair is two incomplete units with overlapping scope, earlier first by
// execution order. Serialised is true when a dependency path in either
// direction orders them.
type Pair struct {
	Units      [2]string `json:"units"`
	Serialised bool      `json:"serialised"`
}

// OverlappingPairs lists the incomplete units with overlapping scope. A
// completed unit is already merged and cannot conflict, so it pairs with
// nothing.
func OverlappingPairs(p *Plan, done Completed) []Pair {
	order := orderIndex(p)
	pairs := []Pair{}
	for i, a := range p.Units {
		for _, b := range p.Units[i+1:] {
			if isComplete(a, done) || isComplete(b, done) || !ScopesOverlap(a, b) {
				continue
			}
			serialised := Ancestors(p, a.ID)[b.ID] || Ancestors(p, b.ID)[a.ID]
			x, y := a, b
			if position(order, a.ID) > position(order, b.ID) {
				x, y = b, a
			}
			pairs = append(pairs, Pair{Units: [2]string{x.ID, y.ID}, Serialised: serialised})
		}
	}
	return pairs
}

// ValidatePlan is the load stage's check before the wave loop starts; no
// problems means the plan is loadable.
func ValidatePlan(p *Plan) []Problem {
	problems := []Problem{}
	byID := unitIndex(p)
	ids := make([]string, len(p.Units))
	for i, u := range p.Units {
		ids[i] = u.ID
	}
	for _, id := range repeated(ids) {
		problems = append(problems, problem("duplicate unit id "+id, id))
	}
	for _, u := range p.Units {
		for _, d := range u.Dependencies {
			if byID[d.Unit] == nil {
				problems = append(problems, problem(fmt.Sprintf("%s depends on unknown unit %s", u.ID, d.Unit), u.ID))
			}
		}
		if !slices.Contains(p.ExecutionOrder, u.ID) {
			problems = append(problems, problem(u.ID+" missing from execution order", u.ID))
		}
	}
	for _, id := range p.ExecutionOrder {
		if byID[id] == nil {
			problems = append(problems, problem("execution order names unknown unit "+id, id))
		}
	}
	if cycle := FindCycle(p); cycle != nil {
		problems = append(problems, Problem{Kind: "cycle", Statement: "dependency cycle " + strings.Join(cycle, " -> "), Referent: cycle[0]})
	}
	for _, pr := range OverlappingPairs(p, nil) {
		if !pr.Serialised {
			problems = append(problems, Problem{
				Kind:      "unserialised-overlap",
				Statement: fmt.Sprintf("%s and %s have overlapping scope and no dependency path between them", pr.Units[0], pr.Units[1]),
				Referent:  pr.Units[0] + "+" + pr.Units[1],
			})
		}
	}
	return problems
}

func isComplete(u *Unit, done Completed) bool {
	return u.Complete || done[u.ID]
}

// depsComplete reports whether every dependency of u is complete. An
// unknown dependency is incomplete (D2: the JS throws; ValidatePlan rejects
// it first).
func depsComplete(u *Unit, byID map[string]*Unit, done Completed) bool {
	for _, d := range u.Dependencies {
		if dep := byID[d.Unit]; dep == nil || !isComplete(dep, done) {
			return false
		}
	}
	return true
}

func uncleared(u *Unit) []Gate {
	var out []Gate
	for _, g := range u.Gates {
		if slices.Contains(BlockingGateKinds, g.Kind) && !g.Cleared {
			out = append(out, g)
		}
	}
	return out
}

// sortByOrder sorts xs stably by execution order, absent ids last (D2: the
// JS comparator yields NaN for them).
func sortByOrder[T any](xs []T, order map[string]int, id func(T) string) {
	slices.SortStableFunc(xs, func(a, b T) int {
		pa, pb := position(order, id(a)), position(order, id(b))
		switch {
		case pa < pb:
			return -1
		case pa > pb:
			return 1
		}
		return 0
	})
}

// ReadySet is the units whose dependencies are all complete and which
// carry no uncleared organisational or release gate, in execution order.
func ReadySet(p *Plan, done Completed) []*Unit {
	byID := unitIndex(p)
	ready := []*Unit{}
	for _, u := range p.Units {
		if !isComplete(u, done) && depsComplete(u, byID, done) && len(uncleared(u)) == 0 {
			ready = append(ready, u)
		}
	}
	sortByOrder(ready, orderIndex(p), func(u *Unit) string { return u.ID })
	return ready
}

// Blocked is an incomplete unit that is not ready. Reason is "waiting"
// when a dependency is incomplete, "gated" when every dependency is
// complete and a gate is uncleared.
type Blocked struct {
	Unit      string   `json:"unit"`
	Reason    string   `json:"reason"`
	WaitingOn []string `json:"waitingOn"`
	Gates     []string `json:"gates"`
}

func BlockedUnits(p *Plan, done Completed) []Blocked {
	byID := unitIndex(p)
	ready := map[string]bool{}
	for _, u := range ReadySet(p, done) {
		ready[u.ID] = true
	}
	out := []Blocked{}
	for _, u := range p.Units {
		if isComplete(u, done) || ready[u.ID] {
			continue
		}
		b := Blocked{Unit: u.ID, Reason: "gated", WaitingOn: []string{}, Gates: []string{}}
		for _, d := range u.Dependencies {
			if dep := byID[d.Unit]; dep == nil || !isComplete(dep, done) {
				b.WaitingOn = append(b.WaitingOn, d.Unit)
			}
		}
		for _, g := range uncleared(u) {
			b.Gates = append(b.Gates, g.Name)
		}
		if len(b.WaitingOn) > 0 {
			b.Reason = "waiting"
		}
		out = append(out, b)
	}
	return out
}

// MergeOrder is a wave's units in execution order, never a choice.
func MergeOrder(p *Plan, ids []string) []string {
	out := slices.Clone(ids)
	sortByOrder(out, orderIndex(p), func(id string) string { return id })
	return out
}

// WavePlan partitions the remaining plan into waves, assuming every gate
// clears and nothing is re-planned. For narration and critical-path display
// only. done is not modified.
func WavePlan(p *Plan, done Completed) [][]string {
	d := Completed{}
	for id, ok := range done {
		if ok {
			d[id] = true
		}
	}
	waves := [][]string{}
	for {
		ready := ReadySet(p, d)
		if len(ready) == 0 {
			return waves
		}
		wave := make([]string, len(ready))
		for i, u := range ready {
			wave[i] = u.ID
			d[u.ID] = true
		}
		waves = append(waves, wave)
	}
}
