package nav

import (
	"errors"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
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

// RouteToNearest plans a ground route from from (a standstill position) to
// the nearest cell accept takes, for a walker that has a kind of place to
// reach rather than a point. The cell search expands outward from from with
// no heuristic, so in order of path cost, for at most p.Budget cells across
// its replans, and ends at the first cell it pops that accept takes and
// that is a click target (the pull exempts a route's last cell from that
// test); a planner that does not swim takes no cell in water. accept is
// called once for each cell the search pops. The route's last point is the
// landing cell's centre on its floor.
//
// Otherwise it behaves as Route does: the cell path is pulled into checked
// segments, a failed pull avoids the failing edge and searches again within
// what is left of the budget, and a swimming planner whose route swims
// farther than p.Swim retries dry. With no fixed goal there is no goal
// column to exempt from p.Avoid: the search takes no cell in an avoided
// column. It reports ErrBudget when the search expanded the budget without
// popping a cell accept takes, ErrUnreachable when it ran out of cells or
// every way it found failed the step-chain check.
func (p *Planner) RouteToNearest(from location.Location, accept func(Cell) bool) ([]location.Location, error) {
	s := getScratch()
	defer putScratch(s)
	route, _, err := p.routeToNearest(s, from, accept)
	if errors.Is(err, ErrBreath) {
		dry := *p
		dry.Swim = 0
		if route, _, err = dry.routeToNearest(s, from, accept); errors.Is(err, ErrWater) {
			err = ErrBreath
		}
	}
	if err != nil {
		return nil, err
	}
	return slices.Clone(route), nil
}

// routeToNearest is RouteToNearest on the given scratch, returning s.route
// and the cells its searches expanded.
func (p *Planner) routeToNearest(s *scratch, from location.Location, accept func(Cell) bool) ([]location.Location, int, error) {
	start, ok := p.Locate(from)
	if !ok {
		return nil, 0, ErrNotWalkable
	}
	if !p.swims() && p.Water.wet(start.X, start.Y, start.Z) {
		return nil, 0, ErrWater
	}
	from.Z = int(start.Z)
	lands := func(c Cell) bool {
		return accept(c) && p.clickable(c, &s.layers) && (p.swims() || !p.Water.wet(c.X, c.Y, c.Z))
	}

	var avoid map[edgeKey]struct{}
	budget, expanded := p.Budget, 0
	s.farEnd = s.work + farWork
	for range maxReplans + 1 {
		cells, used, err := p.searchNearest(s, start, budget, avoid, lands)
		expanded += used
		if err != nil {
			return nil, expanded, err
		}
		budget -= used
		bad, ok := p.pull(s, cells, from, cells[len(cells)-1].World())
		if ok {
			if p.swims() && !p.breathOK(s.route) {
				return nil, expanded, ErrBreath
			}
			return s.route, expanded, nil
		}
		if avoid == nil {
			avoid = make(map[edgeKey]struct{}, maxReplans)
		}
		avoid[bad] = struct{}{}
	}
	return nil, expanded, ErrUnreachable
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
// route's endpoints', none drops (dropBound) where the cell path it
// stands for does not, and none crosses covered ground where that path does
// not. When even the next cell fails, it reports that edge.
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
	s.drops = append(s.drops[:0], 0)
	s.covers = append(s.covers[:0], 0)
	weighCover := p.coveredCost() > 0
	for k := 1; k <= n; k++ {
		s.drops = append(s.drops, s.drops[k-1]+min(dropCost(cells[k-1].Z, cells[k].Z), 1))
		cover := int32(0)
		if weighCover && p.isCovered(cells[k-1], &s.layers) {
			cover = 1
		}
		s.covers = append(s.covers, s.covers[k-1]+cover)
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
			if p.inReach(s, cells, i, j, n) && !p.avoided(s, a, b, from, to) && p.corridorClear(s, a, b) && p.Engine.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) && p.dropsAsPath(s, i, j) && p.coveredAsPath(s, i, j) {
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
				if !p.Engine.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) || !p.dropsAsPath(s, i, j) || !p.coveredAsPath(s, i, j) {
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
			if p.inReach(s, cells, i, j, n) && !p.avoided(s, a, b, from, to) && p.Engine.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) && p.dropsAsPath(s, i, j) && p.coveredAsPath(s, i, j) && p.stepChainOK(s, a, b) {
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

// straight reports whether the route from from to to, both on their
// floors, may be the one segment between them without a search: it passes
// what pull asks of its first candidate, the whole path at once, being
// within the segment cap, crossing no avoided column, passing the corridor
// test and CanMove over p's engine (where the door table toggles the doors),
// and taking no drop nor crossing covered ground, since without a cell path
// no drop or covered cell of the path can stand for one of the line
// (dropsAsPath, coveredAsPath). The goal is a click target
// already. When it passes, pull would take the same segment after the
// search; when it fails, the search decides as it always did (#565).
func (p *Planner) straight(s *scratch, from, to location.Location) bool {
	if segLen(from, to) > p.segmentCap() {
		return false
	}
	s.grid.invalidate()
	return !p.avoided(s, from, to, from, to) && p.corridorClear(s, from, to) &&
		p.Engine.CanMove(from.X, from.Y, from.Z, to.X, to.Y, to.Z) && !p.lineDrops(s, from, to) &&
		!(p.coveredCost() > 0 && p.lineCovered(s, from, to))
}

// coveredAsPath reports whether the segment from waypoint i to cell j may be
// taken as far as covered ground goes: the cell path from i to j leaves a
// covered cell, or the segment's line leaves none. A route that weighed the
// way round a building over the way through keeps to it, where a straight
// line through its doorways would pass CanMove (#640).
func (p *Planner) coveredAsPath(s *scratch, i, j int) bool {
	return p.coveredCost() <= 0 || s.covers[j] > s.covers[i] || !p.lineCovered(s, s.points[i], s.points[j])
}

// lineCovered reports whether the walk along the line from a to b, landing
// on each cell's layer as CanMove does, leaves a covered cell
// (Planner.isCovered) before b's.
func (p *Planner) lineCovered(s *scratch, a, b location.Location) bool {
	s.dropLine = appendLineCells(s.dropLine[:0], a, b)
	c := Cell{X: engine.GeoX(a.X), Y: engine.GeoY(a.Y), Z: p.Engine.Height(a.X, a.Y, a.Z)}
	for k := 0; ; k++ {
		if p.isCovered(c, &s.layers) {
			return true
		}
		if k+1 >= len(s.dropLine) {
			return false
		}
		next := s.dropLine[k+1]
		h, _, ok := p.Engine.NodeBelow(next.x, next.y, int(c.Z)+block.CellIgnoreHeight)
		if !ok || k+2 >= len(s.dropLine) {
			return false // CanMove refuses the line, or next is b's cell
		}
		c = Cell{X: next.x, Y: next.y, Z: h}
	}
}

// dropsAsPath reports whether the segment from waypoint i to cell j may be
// taken as far as drops go: the cell path from i to j holds a drop, or the
// segment's line holds none. A route that weighed a ramp over a drop keeps
// to it, where a straight line off the band's edge would pass CanMove (#504).
func (p *Planner) dropsAsPath(s *scratch, i, j int) bool {
	return s.drops[j] > s.drops[i] || !p.lineDrops(s, s.points[i], s.points[j])
}

// lineDrops reports whether the walk along the line from a to b, landing on
// each cell's layer as CanMove does (the nearest at or below the last floor
// plus block.CellIgnoreHeight), takes a drop.
func (p *Planner) lineDrops(s *scratch, a, b location.Location) bool {
	s.dropLine = appendLineCells(s.dropLine[:0], a, b)
	z := int16(p.Engine.Height(a.X, a.Y, a.Z))
	for _, c := range s.dropLine[1:] {
		h, _, ok := p.Engine.NodeBelow(c.x, c.y, int(z)+block.CellIgnoreHeight)
		if !ok {
			return false // CanMove refuses the line
		}
		if dropCost(z, h) > 0 {
			return true
		}
		z = h
	}
	return false
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
