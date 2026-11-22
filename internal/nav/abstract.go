package nav

import (
	"errors"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// abstractBudget caps the abstract nodes one distance or route query
// expands; a route's door-aware and door-open searches share it.
const abstractBudget = 1 << 18

// anode is one abstract node the search reached; index 0 is the goal.
type anode struct {
	id     int32
	g      int32
	parent int32
	closed bool
}

// Distance returns the abstract-graph distance from a to b in step costs
// (tenths of a cell), for ranking destinations: the cheapest way over the
// entrance nodes of their SCC, every door open, with the legs inside the
// first and last cluster taken as straight lines and nothing refined
// (docs/navigation.md, "Distances for ranking"). Cells of one cluster
// answer their straight-line distance. a and b must be SameSCC, else it is
// ErrUnreachable.
func (h *Hierarchy) Distance(a, b Member) (int, error) {
	return h.distance(a, b, nil)
}

// distance is Distance under the door state closed (nil: every door open).
func (h *Hierarchy) distance(a, b Member, closed func(door int) bool) (int, error) {
	if !h.SameSCC(a, b) {
		return 0, ErrUnreachable
	}
	s := getScratch()
	defer putScratch(s)
	cost, _, err := h.abstractSearch(s, a, b, closed, abstractBudget)
	return int(cost), err
}

// abstractSearch runs A* over the entrance nodes of a's SCC from a's cell to
// b's, expanding at most budget nodes, and leaves the node path, first node
// first, in s.apath. It returns the cost and the nodes it expanded. Cells of
// one cluster answer their straight-line distance and an empty path.
func (h *Hierarchy) abstractSearch(s *scratch, a, b Member, closed func(door int) bool, budget int) (int32, int, error) {
	s.apath = s.apath[:0]
	if a.res.cluster == b.res.cluster {
		return heuristic(a.cell, b.cell), 0, nil
	}
	g := h.graph
	x := a.SCC()
	s.anodes = append(s.anodes[:0], anode{id: -1, g: -1, parent: -1})
	s.open = s.open[:0]
	clear(s.aindex)
	relax := func(id int32, ng int32, parent int32) {
		idx, seen := s.aindex[id]
		switch {
		case id < 0:
			idx = 0
			if n := &s.anodes[0]; n.g >= 0 && ng >= n.g {
				return
			}
			s.anodes[0].g, s.anodes[0].parent = ng, parent
		case !seen:
			idx = int32(len(s.anodes))
			s.aindex[id] = idx
			s.anodes = append(s.anodes, anode{id: id, g: ng, parent: parent})
		default:
			if n := &s.anodes[idx]; n.closed || ng >= n.g {
				return
			}
			s.anodes[idx].g, s.anodes[idx].parent = ng, parent
		}
		f := ng
		if id >= 0 {
			f += heuristic(g.nodeRep(int(id)), b.cell)
		}
		s.push(openItem{f: f, g: ng, idx: idx})
	}
	first, end := g.clusterNodes(int(a.res.cluster))
	for n := first; n < end; n++ {
		if g.nodeSCC(n) == x {
			relax(int32(n), heuristic(a.cell, g.nodeRep(n)), -1)
		}
	}
	expanded := 0
	for len(s.open) > 0 {
		it := s.pop()
		n := &s.anodes[it.idx]
		if n.closed || it.g != n.g {
			continue
		}
		if it.idx == 0 {
			for i := n.parent; i > 0; i = s.anodes[i].parent {
				s.apath = append(s.apath, s.anodes[i].id)
			}
			slices.Reverse(s.apath)
			return n.g, expanded, nil
		}
		if expanded >= budget {
			return 0, expanded, ErrBudget
		}
		expanded++
		n.closed = true
		id, gv := n.id, n.g
		rep := g.nodeRep(int(id))
		if g.nodeCluster(int(id)) == b.res.cluster {
			relax(-1, gv+heuristic(rep, b.cell), it.idx)
		}
		lo, hi := g.nodeEdges(int(id))
		for i := lo; i < hi; i++ {
			e := g.edge(i)
			if g.nodeSCC(e.to) != x {
				continue
			}
			if c, ok := g.edgeCost(e, closed); ok {
				relax(int32(e.to), gv+int32(c), it.idx)
			}
		}
	}
	return 0, expanded, ErrUnreachable
}

// Route plans a ground route from from to to across clusters, as
// Planner.Route does inside one, for the default consumers: it refuses a
// destination not SameSCC with the start (ErrUnreachable), so a route never
// enters ground it cannot leave, such as a pit. Between clusters it searches
// the abstract graph of their SCC, then refines the legs between the
// entrance nodes it chose with the cell search and string-pulls the whole
// path (docs/navigation.md, "Routes").
//
// closed is the door state (nil: every door taken as open), such as the
// door table's: an abstract edge whose path crosses a closed door costs its
// door-free way, and the cell legs read p's engine, where the door table
// toggles the doors. Only when that search finds no way does a second one
// run with every door open, over the static engine; if it finds a way the
// answer is ErrDoor, never ErrUnreachable (docs/navigation.md, "Doors in the
// hierarchy"). The two share one budget: p.Budget cells and abstractBudget
// abstract nodes.
func (h *Hierarchy) Route(p *Planner, from, to location.Location, closed func(door int) bool) ([]location.Location, error) {
	s := getScratch()
	defer putScratch(s)
	start, ok := p.Locate(from)
	if !ok {
		return nil, ErrNotWalkable
	}
	goal, ok := p.Locate(to)
	if !ok {
		return nil, ErrNotWalkable
	}
	if !p.clickable(goal, &s.layers) {
		return nil, ErrUnclickable
	}
	ma, err := h.Locate(start, nil)
	if err != nil {
		return nil, ErrNotWalkable
	}
	mb, err := h.Locate(goal, nil)
	if err != nil {
		return nil, ErrNotWalkable
	}
	if !h.SameSCC(ma, mb) {
		return nil, ErrUnreachable
	}
	from.Z, to.Z = int(start.Z), int(goal.Z)
	q := routeQuery{a: ma, b: mb, start: start, goal: goal, from: from, to: to}
	b := routeBudget{cells: p.Budget, nodes: abstractBudget}
	route, err := h.search(s, p, &q, closed, &b)
	if err == nil {
		return slices.Clone(route), nil
	}
	if !errors.Is(err, ErrUnreachable) {
		return nil, err
	}
	open := *p
	open.Engine = h.static.Engine
	switch _, err := h.search(s, &open, &q, nil, &b); {
	case err == nil:
		return nil, ErrDoor
	case errors.Is(err, ErrBudget):
		return nil, ErrBudget
	}
	return nil, ErrUnreachable
}

// routeQuery is one Route's endpoints: the members, their cells, and the
// world points snapped to their floors.
type routeQuery struct {
	a, b        Member
	start, goal Cell
	from, to    location.Location
}

// routeBudget is what is left of one Route's budget.
type routeBudget struct {
	cells, nodes int
}

// search is one search of Route under the door state closed: the abstract
// path, then its legs refined over p's engine and string-pulled, in s.route.
// It replans up to maxReplans times with the failing cell edge avoided, and
// spends from b.
func (h *Hierarchy) search(s *scratch, p *Planner, q *routeQuery, closed func(door int) bool, b *routeBudget) ([]location.Location, error) {
	_, used, err := h.abstractSearch(s, q.a, q.b, closed, b.nodes)
	b.nodes -= used
	if err != nil {
		return nil, err
	}
	s.waypoints = append(s.waypoints[:0], q.start)
	for _, n := range s.apath {
		if c := h.graph.nodeRep(int(n)); c != s.waypoints[len(s.waypoints)-1] {
			s.waypoints = append(s.waypoints, c)
		}
	}
	if q.goal != s.waypoints[len(s.waypoints)-1] || len(s.waypoints) == 1 {
		s.waypoints = append(s.waypoints, q.goal)
	}
	var avoid map[edgeKey]struct{}
	for range maxReplans + 1 {
		s.path = s.path[:0]
		for i := 0; i+1 < len(s.waypoints); i++ {
			cells, used, err := p.search(s, s.waypoints[i], s.waypoints[i+1], b.cells, avoid)
			b.cells -= used
			if err != nil {
				return nil, err
			}
			if len(s.path) > 0 {
				cells = cells[1:]
			}
			s.path = append(s.path, cells...)
		}
		bad, ok := p.pull(s, s.path, q.from, q.to)
		if ok {
			return s.route, nil
		}
		if avoid == nil {
			avoid = make(map[edgeKey]struct{}, maxReplans)
		}
		avoid[bad] = struct{}{}
	}
	return nil, ErrUnreachable
}
