package nav

import (
	"errors"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
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
	// may reach once a route's farWork is spent, so each exact check covers
	// a short stretch.
	exactCells = 8
	// farStretch caps, in world units of line, the stretches failing the
	// corridor test that a farther candidate may take into the exact check:
	// its cost grows with them.
	farStretch = 16 * block.CellSize
	// farWork is a route's budget for farther candidates, in corridor cells
	// filled and CanMove calls (s.work): once it is spent, the rest of the
	// route takes exactCells candidates only.
	farWork = 800_000
	// farShort is how long a corridor segment must be for the pull to take
	// it without trying farther candidates: the budget goes where segments
	// are short.
	farShort = 600
	// farMiss is how near a farther candidate's line may pass a point where
	// the exact check of a candidate from the same waypoint failed before
	// the candidate is skipped untried.
	farMiss = 2 * block.CellSize
	// maxReplans caps how many times a route searches again after a step of
	// its cell path failed the step-chain check, the failing edge avoided.
	maxReplans = 8
)

// Route plans a ground route from from (a standstill position) to to. It
// returns the waypoints, from first (on its floor) and to last (snapped to
// the floor as the server snaps a destination). Unless the planner swims, an
// endpoint whose cell is in water is ErrWater; a swimming planner's route
// that swims a stretch longer than p.Swim is ErrBreath. Each consecutive pair is one
// segment of at most navSegment units (p.MaxSegment when shorter) that passes
// CanMove and StepChainOK; the planner takes a segment only after both checks
// pass, and every intermediate waypoint is a click target on its layer.
func (p *Planner) Route(from, to location.Location) ([]location.Location, error) {
	s := getScratch()
	defer putScratch(s)
	route, err := p.route(s, from, to)
	if errors.Is(err, ErrBreath) {
		// The search weighs water but cannot count breath: the cheapest way
		// swam too far, so look for a dry one.
		dry := *p
		dry.Swim = 0
		if route, err = dry.route(s, from, to); errors.Is(err, ErrWater) {
			err = ErrBreath
		}
	}
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
	if !p.swims() && (p.Water.wet(start.X, start.Y, start.Z) || p.Water.wet(goal.X, goal.Y, goal.Z)) {
		return nil, ErrWater
	}
	from.Z, to.Z = int(start.Z), int(goal.Z)

	var avoid map[edgeKey]struct{}
	budget := p.Budget
	s.farEnd = s.work + farWork
	for range maxReplans + 1 {
		cells, used, err := p.search(s, start, goal, budget, avoid, anywhere)
		if err != nil {
			return nil, err
		}
		budget -= used
		bad, ok := p.pull(s, cells, from, to)
		if ok {
			if p.swims() && !p.breathOK(s.route) {
				return nil, ErrBreath
			}
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
// waypoint it takes the farthest cell ahead, within the segment cap, that is a
// click target and passes CanMove and the corridor test. When none does, a
// swimming planner at a waypoint in water takes the farthest its swim line
// reaches (swimLeg). With nothing, or a segment shorter than farShort, it
// tries the cells past exactCells along the path while the route's farWork
// lasts, farthest first: one passing CanMove whose stretches failing the corridor
// test total at most farStretch and that passes the exact check over them is
// taken; a candidate passing within farMiss of where an earlier one's exact
// check failed is skipped. With nothing yet, it
// takes the farthest within exactCells that passes CanMove and the full
// step-chain check. None crosses a column of p.Avoid but the
// route's endpoints'. When even the next cell fails, it reports that edge.
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
		// A swimmer takes the farthest point its straight line reaches in
		// water: no step of a swim is ever blocked.
		if next < 0 && p.swims() && p.inWater(s.points[i]) {
			for j := n; j > i && next < 0; j-- {
				a, b := s.points[i], s.points[j]
				if p.inReach(s, cells, i, j, n) && !p.avoided(s, a, b, from, to) && p.swimLeg(a, b) {
					next = j
				}
			}
		}
		if p.Lmax > 0 && !(p.swims() && p.inWater(s.points[i])) && (next < 0 || segLen(s.points[i], s.points[next]) < farShort) {
			far := -1
			s.misses = s.misses[:0]
			for j := n; j > max(next, i+exactCells) && far < 0 && s.work < s.farEnd; j-- {
				a, b := s.points[i], s.points[j]
				if !p.inReach(s, cells, i, j, n) || p.avoided(s, a, b, from, to) || s.missed(a, b) {
					continue
				}
				s.work++
				if !p.Engine.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) {
					continue
				}
				st := p.corridorStretches(s, a, b)
				if failing(st) > farStretch {
					continue
				}
				s.failed = false
				if p.exactStretches(s, a, b, st) {
					far = j
				} else if s.failed {
					s.misses = append(s.misses, s.failAt)
				}
			}
			next = max(next, far)
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

// segLen is the ground length of the segment from a to b.
func segLen(a, b location.Location) float64 { return hypot(b.X-a.X, b.Y-a.Y) }

// missed reports whether the segment from a to b passes within farMiss of a
// point where an exact check of the current waypoint's candidates failed.
func (s *scratch) missed(a, b location.Location) bool {
	for _, m := range s.misses {
		if newSegment(a, b).dist(m) <= farMiss {
			return true
		}
	}
	return false
}

// failing is the length of line the stretches cover.
func failing(stretches []stretch) float64 {
	sum := 0.0
	for _, st := range stretches {
		sum += st.t1 - st.t0
	}
	return sum
}

// inReach reports whether the segment from waypoint i to cell j is within
// the planner's segment cap and ends on a click target.
func (p *Planner) inReach(s *scratch, cells []Cell, i, j, n int) bool {
	a, b := s.points[i], s.points[j]
	if hypot(b.X-a.X, b.Y-a.Y) > p.segmentCap() {
		return false
	}
	return j == n || p.clickable(cells[j], &s.layers)
}

// segmentCap is the longest segment the planner takes: MaxSegment, or
// navSegment when that is zero or longer.
func (p *Planner) segmentCap() float64 {
	if p.MaxSegment > 0 && p.MaxSegment < navSegment {
		return p.MaxSegment
	}
	return navSegment
}
