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
	sp := sameSpan(a)
	cost, _, err := h.abstractSearch(s, a, b, &sp, closed, abstractBudget, &seating{})
	return int(cost), err
}

// abstractSearch runs A* over the entrance nodes of the SCCs of span sp
// from a's cell to b's, expanding at most budget nodes, and leaves the node
// path, first node first, in s.apath. It returns the cost and the nodes it
// expanded. Cells of one cluster answer their straight-line distance and an
// empty path.
func (h *Hierarchy) abstractSearch(s *scratch, a, b Member, sp *span, closed func(door int) bool, budget int, seat *seating) (int32, int, error) {
	s.apath = s.apath[:0]
	if a.res.cluster == b.res.cluster {
		return heuristic(a.cell, b.cell), 0, nil
	}
	g := h.graph
	s.anodes = append(s.anodes[:0], anode{id: -1, g: -1, parent: -1})
	s.open = s.open[:0]
	s.aindex.reset()
	relax := func(id int32, ng int32, parent int32) {
		var idx int32
		var seen bool
		if id >= 0 {
			idx, seen = s.aindex.getOrPut(uint64(id), int32(len(s.anodes)))
		}
		switch {
		case id < 0:
			idx = 0
			if n := &s.anodes[0]; n.g >= 0 && ng >= n.g {
				return
			}
			s.anodes[0].g, s.anodes[0].parent = ng, parent
		case !seen:
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
		if sp.start(g.nodeSCC(n)) && allows(seat.starts, int32(n)) {
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
		if g.nodeCluster(int(id)) == b.res.cluster && sp.goal(g.nodeSCC(int(id))) && allows(seat.goals, id) {
			relax(-1, gv+heuristic(rep, b.cell), it.idx)
		}
		lo, hi := g.nodeEdges(int(id))
		for i := lo; i < hi; i++ {
			e := g.edge(i)
			if !sp.has(g.nodeSCC(e.to)) {
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
	return h.route(p, from, to, closed, false)
}

// route is Route, and RouteOneWay when oneWay is set.
func (h *Hierarchy) route(p *Planner, from, to location.Location, closed func(door int) bool, oneWay bool) ([]location.Location, error) {
	// The abstract graph holds no wet cell, so its legs keep out of water
	// too: a swim is the cell planner's (docs/navigation.md, "Swimming").
	dry := *p
	dry.Swim = 0
	p = &dry
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
	if h.static.Water.wet(start.X, start.Y, start.Z) || h.static.Water.wet(goal.X, goal.Y, goal.Z) {
		return nil, ErrWater
	}
	ma, err := h.Locate(start, nil)
	if err != nil {
		return nil, ErrNotWalkable
	}
	mb, err := h.Locate(goal, nil)
	if err != nil {
		return nil, ErrNotWalkable
	}
	from.Z, to.Z = int(start.Z), int(goal.Z)
	q := routeQuery{a: ma, b: mb, start: start, goal: goal, from: from, to: to}
	switch {
	case h.SameSCC(ma, mb):
		q.span = sameSpan(ma)
	case !oneWay:
		return nil, ErrUnreachable
	default:
		if q.span, ok = h.oneWaySpan(s, ma, mb); !ok {
			return nil, ErrUnreachable
		}
	}
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
	q.seat = seating{} // seated under the door state closed
	switch _, err := h.search(s, &open, &q, nil, &b); {
	case err == nil:
		return nil, ErrDoor
	case errors.Is(err, ErrBudget):
		return nil, ErrBudget
	}
	return nil, ErrUnreachable
}

// routeQuery is one Route's endpoints: the members, their cells, and the
// world points snapped to their floors; and the SCCs its abstract search
// may use.
type routeQuery struct {
	a, b        Member
	start, goal Cell
	from, to    location.Location
	span        span
	seat        seating
}

// routeBudget is what is left of one Route's budget.
type routeBudget struct {
	cells, nodes int
}

// search is one search of Route under the door state closed: the abstract
// path, then its legs refined over p's engine, keeping out of p.Avoid, and
// string-pulled, in s.route.
// It replans up to maxReplans times with the failing cell edge avoided, and
// spends from b. When the first or last leg finds that its endpoint and
// entrance node do not join inside their cluster, it seats that end on the
// nodes that do (seating) and searches again.
func (h *Hierarchy) search(s *scratch, p *Planner, q *routeQuery, closed func(door int) bool, b *routeBudget) ([]location.Location, error) {
	for {
		route, err := h.searchSeated(s, p, q, closed, b)
		if !errors.Is(err, errReseat) {
			return route, err
		}
	}
}

// errReseat asks search to run again: an end of q was seated.
var errReseat = errors.New("nav: route end seated")

func (h *Hierarchy) searchSeated(s *scratch, p *Planner, q *routeQuery, closed func(door int) bool, b *routeBudget) ([]location.Location, error) {
	_, used, err := h.abstractSearch(s, q.a, q.b, &q.span, closed, b.nodes, &q.seat)
	b.nodes -= used
	if err != nil {
		return nil, err
	}
	s.waypoints = append(s.waypoints[:0], q.start)
	for _, n := range s.apath {
		// A representative in an avoided column is left out: the leg
		// searches around it from the waypoint before to the one after.
		if c := h.graph.nodeRep(int(n)); c != s.waypoints[len(s.waypoints)-1] && !p.Avoid.has(c) {
			s.waypoints = append(s.waypoints, c)
		}
	}
	if q.goal != s.waypoints[len(s.waypoints)-1] || len(s.waypoints) == 1 {
		s.waypoints = append(s.waypoints, q.goal)
	}
	var avoid map[edgeKey]struct{}
	s.farEnd = s.work + farWork
	for range maxReplans + 1 {
		s.path = s.path[:0]
		legs := len(s.waypoints) - 1
		for i := range legs {
			a, c := s.waypoints[i], s.waypoints[i+1]
			var cells []Cell
			var used int
			var err error
			switch {
			case i == 0 && legs > 1 && !q.seat.startDone && h.clusterBox(a).has(c):
				cells, used, err = h.firstLeg(s, p, q, a, c, b.cells, avoid)
			case i == legs-1 && legs > 1 && !q.seat.goalDone && h.clusterBox(c).has(a):
				cells, used, err = h.lastLeg(s, p, q, a, c, b.cells, avoid)
			case i > 0 && i < legs-1:
				cells, used, err = h.middleLeg(s, p, a, c, b.cells, avoid)
			default:
				cells, used, err = p.search(s, a, c, b.cells, avoid, anywhere)
			}
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

// seating narrows a route's abstract search to the entrance nodes its start
// reaches, and those reaching its goal, inside their clusters. Without it
// the search takes every node of the endpoint's SCC in its cluster as joined
// to the endpoint by a straight line; for a node joined only by a way out
// of the cluster and back, the leg's cell search floods until the budget
// runs out (#256).
type seating struct {
	// starts and goals are the nodes the search may start and end on,
	// sorted; nil allows every node.
	starts, goals []int32
	// startDone and goalDone record that the first or last leg was checked
	// inside its cluster.
	startDone, goalDone bool
}

// allows reports whether node n is in nodes; nil allows every node.
func allows(nodes []int32, n int32) bool {
	if nodes == nil {
		return true
	}
	_, ok := slices.BinarySearch(nodes, n)
	return ok
}

// firstLeg refines the leg from the start to the first entrance node inside
// the start's cluster. It looks for a leg without a drop first: the abstract
// search joins the start to its cluster's nodes by a straight line, which
// does not see a drop off the band the start stands on (#504). When none
// joins them there, the search has flooded everything the start reaches in
// the cluster, so the nodes it reached are the ones the start may seat on:
// it seats the start there and answers errReseat. Only a start that reaches
// no node without a drop looks again with drops, and seats the same way; a
// start that reaches no node at all keeps the free search.
func (h *Hierarchy) firstLeg(s *scratch, p *Planner, q *routeQuery, a, c Cell, budget int, avoid map[edgeKey]struct{}) ([]Cell, int, error) {
	q.seat.startDone = true
	box := h.clusterBox(a)
	used := 0
	for _, dry := range [2]bool{true, false} {
		lp := *p
		lp.noDrops = dry
		cells, n, err := lp.search(s, a, c, budget-used, avoid, box)
		used += n
		if !errors.Is(err, ErrUnreachable) {
			return cells, used, err
		}
		if starts := h.seats(s, q.a, q.span.start); len(starts) > 0 {
			q.seat.starts = starts
			return nil, used, errReseat
		}
	}
	cells, more, err := p.search(s, a, c, budget-used, avoid, anywhere)
	return cells, used + more, err
}

// lastLeg is firstLeg for the leg from the last entrance node to the goal:
// when they do not join inside the goal's cluster, without a drop and then
// with drops, it walks back from the goal over the cluster to the nodes that
// reach it and seats the goal there.
func (h *Hierarchy) lastLeg(s *scratch, p *Planner, q *routeQuery, c, goal Cell, budget int, avoid map[edgeKey]struct{}) ([]Cell, int, error) {
	q.seat.goalDone = true
	box := h.clusterBox(goal)
	used := 0
	for _, dry := range [2]bool{true, false} {
		lp := *p
		lp.noDrops = dry
		cells, n, err := lp.search(s, c, goal, budget-used, avoid, box)
		used += n
		if !errors.Is(err, ErrUnreachable) {
			return cells, used, err
		}
		used += lp.reachers(s, goal, box)
		if goals := h.seats(s, q.b, q.span.goal); len(goals) > 0 {
			q.seat.goals = goals
			return nil, used, errReseat
		}
	}
	cells, more, err := p.search(s, c, goal, budget-used, avoid, anywhere)
	return cells, used + more, err
}

// seats returns the nodes of m's cluster whose representative the last
// flood reached (s.index) and whose SCC in reports true.
func (h *Hierarchy) seats(s *scratch, m Member, in func(scc int32) bool) []int32 {
	g := h.graph
	first, end := g.clusterNodes(int(m.res.cluster))
	var nodes []int32
	for n := first; n < end; n++ {
		if _, ok := s.index.get(cellKey(g.nodeRep(n))); ok && in(g.nodeSCC(n)) {
			nodes = append(nodes, int32(n))
		}
	}
	return nodes
}

// middleLeg refines a leg between two entrance nodes inside the clusters
// holding them, where the build costed the abstract edge it stands for
// (HPA* refinement): an edge inside a cluster is the cheapest path inside
// it, and an edge between two clusters is one crossing. Only when the box
// does not join them (an edge avoided after a failed pull, or a waypoint
// left out in an avoided column) does it search anywhere.
func (h *Hierarchy) middleLeg(s *scratch, p *Planner, a, c Cell, budget int, avoid map[edgeKey]struct{}) ([]Cell, int, error) {
	ba, bc := h.clusterBox(a), h.clusterBox(c)
	box := cellBox{min(ba.x0, bc.x0), min(ba.y0, bc.y0), max(ba.x1, bc.x1), max(ba.y1, bc.y1)}
	cells, used, err := p.search(s, a, c, budget, avoid, box)
	if !errors.Is(err, ErrUnreachable) {
		return cells, used, err
	}
	cells, more, err := p.search(s, a, c, budget-used, avoid, anywhere)
	return cells, used + more, err
}

// clusterBox is the box of the cluster holding c.
func (h *Hierarchy) clusterBox(c Cell) cellBox {
	size := h.graph.key.ClusterCells
	x0, y0 := floorDiv(c.X, size)*size, floorDiv(c.Y, size)*size
	return cellBox{x0, y0, x0 + size, y0 + size}
}
