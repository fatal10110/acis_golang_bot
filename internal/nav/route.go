package nav

import (
	"errors"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// Route errors. Each is distinct, so the walker can tell "unreachable for
// now" (the budget) from "no way at all" inside what the search covered.
var (
	// ErrNotWalkable: an endpoint is outside the world or on a cell without
	// geodata.
	ErrNotWalkable = errors.New("nav: point is not on walkable geodata")
	// ErrUnclickable: the destination's layer has another layer too close
	// above it, so a click there lands on the upper one.
	ErrUnclickable = errors.New("nav: destination cannot be a click target")
	// ErrBudget: the search expanded its node budget without reaching the
	// destination. docs/navigation.md: unreachable for now.
	ErrBudget = errors.New("nav: node budget exhausted")
	// ErrUnreachable: the search ran out of cells to expand, or every way it
	// found failed the step-chain check.
	ErrUnreachable = errors.New("nav: no route")
	// ErrDoor: no route with the doors as they are, but one with every door
	// open, so a closed door is in the way. It is distinct from
	// ErrUnreachable: the walker replans on it, and a door replan never
	// counts toward unreachable (docs/navigation.md, "Walker").
	ErrDoor = errors.New("nav: unreachable: door")
)

const (
	// navSegment caps a segment's length in world units: about one click on
	// screen, far under the server's 9900 move limit (docs/navigation.md,
	// "Routes").
	navSegment = 2500
	// exactCells caps how many cells ahead a segment the corridor test fails
	// may reach, so each exact check covers a short stretch.
	exactCells = 8
	// maxReplans caps how many times a route searches again after a step of
	// its cell path failed the step-chain check, the failing edge avoided.
	maxReplans = 8
)

// Route plans a ground route from from (a standstill position) to to. It
// returns the waypoints, from first (on its floor) and to last (snapped to
// the floor as the server snaps a destination). An endpoint whose cell is in
// water is ErrWater. Each consecutive pair is one
// segment of at most navSegment units that passes CanMove and StepChainOK;
// the planner takes a segment only after both checks pass, and every
// intermediate waypoint is a click target on its layer.
func (p *Planner) Route(from, to location.Location) ([]location.Location, error) {
	s := getScratch()
	defer putScratch(s)
	route, err := p.route(s, from, to)
	if err != nil {
		return nil, err
	}
	return slices.Clone(route), nil
}

// route is Route on the given scratch, returning s.route.
func (p *Planner) route(s *scratch, from, to location.Location) ([]location.Location, error) {
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
	if p.Water.wet(start.X, start.Y, start.Z) || p.Water.wet(goal.X, goal.Y, goal.Z) {
		return nil, ErrWater
	}
	from.Z, to.Z = int(start.Z), int(goal.Z)

	var avoid map[edgeKey]struct{}
	budget := p.Budget
	for range maxReplans + 1 {
		cells, used, err := p.search(s, start, goal, budget, avoid)
		if err != nil {
			return nil, err
		}
		budget -= used
		bad, ok := p.pull(s, cells, from, to)
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

// pull string-pulls the cell path into segments, in s.route. From each
// waypoint it takes the farthest cell ahead, within navSegment, that is a
// click target and passes CanMove and the corridor test. When none does, it
// takes the farthest within exactCells that passes CanMove and the full
// step-chain check. Neither crosses a column of p.Avoid but the route's
// endpoints'. When even the next cell fails, it reports that edge.
func (p *Planner) pull(s *scratch, cells []Cell, from, to location.Location) (edgeKey, bool) {
	n := len(cells) - 1
	s.points = s.points[:0]
	for i, c := range cells {
		switch i {
		case 0:
			s.points = append(s.points, from)
		case n:
			s.points = append(s.points, to)
		default:
			s.points = append(s.points, c.World())
		}
	}
	if n == 0 {
		s.points = append(s.points, to)
		cells = append(cells, cells[0])
		n = 1
	}
	s.route = append(s.route[:0], from)
	s.grid.invalidate()
	for i := 0; i < n; {
		next := -1
		// The corridor test runs before CanMove: on the grid it is the
		// cheaper check, it stops at its first failing cell, and most
		// candidates fail it. Both must pass, so the order changes nothing.
		for j := n; j > i && next < 0; j-- {
			a, b := s.points[i], s.points[j]
			if p.inReach(s, cells, i, j, n) && !p.avoided(s, a, b, from, to) && p.corridorClear(s, a, b) && p.Engine.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) {
				next = j
			}
		}
		for j := min(n, i+exactCells); j > i && next < 0; j-- {
			a, b := s.points[i], s.points[j]
			if p.inReach(s, cells, i, j, n) && !p.avoided(s, a, b, from, to) && p.Engine.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) && p.stepChainOK(s, a, b) {
				next = j
			}
		}
		if next < 0 {
			return edgeKey{cells[i], cells[i+1]}, false
		}
		s.route = append(s.route, s.points[next])
		i = next
	}
	return edgeKey{}, true
}

// inReach reports whether the segment from waypoint i to cell j is within
// navSegment and ends on a click target.
func (p *Planner) inReach(s *scratch, cells []Cell, i, j, n int) bool {
	a, b := s.points[i], s.points[j]
	if hypot(b.X-a.X, b.Y-a.Y) > navSegment {
		return false
	}
	return j == n || p.clickable(cells[j], &s.layers)
}
