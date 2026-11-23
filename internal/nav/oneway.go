package nav

import (
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// RouteOneWay is Route for the consumers that may take a one-way drop
// toward their destination (docs/navigation.md, "Queries"): it also routes
// to a destination only Reachable from the start, not SameSCC with it, such
// as a leader on lower ground past a ledge. Its abstract search runs over
// the SCCs on a way from the start's to the destination's over the
// condensation, crossing from one to the next along the abstract graph's
// directed edges; the legs are refined and pulled as Route's are, under the
// same door handling and the same budget. A destination not Reachable is
// ErrUnreachable.
func (h *Hierarchy) RouteOneWay(p *Planner, from, to location.Location, closed func(door int) bool) ([]location.Location, error) {
	return h.route(p, from, to, closed, true)
}

// span is the abstract SCCs one abstract search may use: where it starts,
// where it may end, and which it may pass.
type span struct {
	// scc is the one SCC of a search between cells of one SCC; it is used
	// when way is nil.
	scc int32
	// from are the SCCs the start reaches inside its cluster, to those
	// reaching the goal inside its cluster.
	from, to []int32
	// way marks with gen the SCCs on a way from a from SCC to a to SCC
	// over the condensation.
	way []uint32
	gen uint32
}

// sameSpan is the span of a search between a and cells of a's SCC.
func sameSpan(a Member) span { return span{scc: a.SCC()} }

// has reports whether the search may pass SCC x.
func (sp *span) has(x int32) bool {
	if sp.way == nil {
		return x == sp.scc
	}
	return sp.way[x] == sp.gen
}

// start reports whether a node of SCC x in the start's cluster may begin
// the search.
func (sp *span) start(x int32) bool {
	if sp.way == nil {
		return x == sp.scc
	}
	return sp.way[x] == sp.gen && slices.Contains(sp.from, x)
}

// goal reports whether a node of SCC x in the goal's cluster may end the
// search.
func (sp *span) goal(x int32) bool {
	if sp.way == nil {
		return x == sp.scc
	}
	return sp.way[x] == sp.gen && slices.Contains(sp.to, x)
}

// condFrame is one SCC on the condensation walk's stack, with the range of
// its condensation edges not walked yet.
type condFrame struct {
	x         int32
	next, end int
}

// oneWaySpan is the span of a route from a to b that may take one-way
// drops: the SCCs a reaches inside its cluster (its own when it is in one),
// those reaching b inside b's, and every SCC on a way from the first to the
// second over the condensation, marked in s. It is a depth-first walk from
// a's SCCs: an SCC is on a way when it is one of b's or an SCC it leads to
// is, which the condensation, a DAG, settles before the SCC is left. ok is
// false when no way exists: b is not Reachable from a.
func (h *Hierarchy) oneWaySpan(s *scratch, a, b Member) (sp span, ok bool) {
	ea, eb := a.res.entries[a.entry], b.res.entries[b.entry]
	sp.from, sp.to = a.res.outs(ea), b.res.ins(eb)
	if ea.kind == InSCC {
		sp.from = []int32{ea.scc}
	}
	if eb.kind == InSCC {
		sp.to = []int32{eb.scc}
	}
	n := h.graph.counts.sccs
	s.nextSCCGen(n)
	if len(s.sccWay) < n {
		s.sccWay = make([]uint32, n)
	}
	gen, seen, way := s.sccGen, s.sccSeen, s.sccWay
	sp.way, sp.gen = way, gen
	stack := s.sccStack[:0]
	push := func(x int32) {
		seen[x] = gen
		lo, hi := h.graph.condRange(x)
		stack = append(stack, condFrame{x: x, next: lo, end: hi})
	}
	for _, r := range sp.from {
		if seen[r] == gen {
			continue
		}
		push(r)
		for len(stack) > 0 {
			f := &stack[len(stack)-1]
			if f.next < f.end {
				t := h.graph.condTarget(f.next)
				f.next++
				switch {
				case seen[t] != gen:
					push(t)
				case way[t] == gen:
					way[f.x] = gen
				}
				continue
			}
			x := f.x
			stack = stack[:len(stack)-1]
			if slices.Contains(sp.to, x) {
				way[x] = gen
			}
			if way[x] == gen && len(stack) > 0 {
				way[stack[len(stack)-1].x] = gen
			}
		}
	}
	s.sccStack = stack
	for _, r := range sp.from {
		if way[r] == gen {
			return sp, true
		}
	}
	return span{}, false
}
