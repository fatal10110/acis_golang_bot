package nav

import (
	"math"
	"slices"
	"sort"

	"github.com/fatal10110/acis_golang_bot/internal/geo"
	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

const (
	// bandRadius bounds how far a rounded step point lies from the segment's
	// line. The float position stays within √2/2 of the line (each step
	// re-aims from the rounded point, a rounding error of at most √2/2 off
	// the line, and moves a fraction below 1 of the way back), and rounding
	// adds at most √2/2 more: √2 in all, rounded up. docs/navigation.md
	// says "about one unit"; this is the bound.
	bandRadius = 1.5
	// stepSlack is how much longer than its distance travelled a step's
	// rounded points can be apart: the float position inside the origin's
	// rounded point and the landing rounding, √2/2 each, rounded up.
	stepSlack = 1.5
	// stretchPad widens an exact stretch past its failing line cells by one
	// cell and the band, so its start states and its far side lie where the
	// corridor test holds.
	stretchPad = block.CellSize + 2
	// maxChainStates caps the exact check's states; a stretch that needs
	// more fails, and the planner tries another way past it.
	maxChainStates = 1 << 14
)

// chainState is a rounded step point with the Z the step chain carries there.
type chainState struct {
	x, y int32
	z    int16
}

// bandPoint is a rounded point within bandRadius of a segment's line, with its
// distance along the line from the segment's start.
type bandPoint struct {
	x, y int
	t    float64
}

// StepChainOK reports whether every sequence of position updates the server
// can take along the segment from a to b reaches b without a blocked step:
// the open-corridor test where it holds, and the exact check on each stretch
// where it fails (docs/navigation.md, "Step-chain check"). a is a standstill
// position on its floor; b's Z is snapped to the floor as the server snaps a
// destination. It does not check CanMove from a to b itself.
func (p *Planner) StepChainOK(a, b location.Location) bool {
	s := getScratch()
	defer putScratch(s)
	s.grid.invalidate()
	return p.stepChainOK(s, a, b)
}

func (p *Planner) stepChainOK(s *scratch, a, b location.Location) bool {
	if a.X == b.X && a.Y == b.Y {
		return true // the first update lands on b: CanMove alone decides
	}
	return p.exactStretches(s, a, b, p.corridorStretches(s, a, b))
}

// exactStretches runs the exact check over the stretches the corridor test
// of the segment from a to b just failed, s.grid still holding that
// corridor.
func (p *Planner) exactStretches(s *scratch, a, b location.Location, stretches []stretch) bool {
	if len(stretches) == 0 {
		return true
	}
	if p.Lmax <= 0 {
		return false
	}
	// Merge stretches whose exact checks would overlap: a start region must
	// lie where the corridor test holds.
	merged := stretches[:1]
	for _, st := range stretches[1:] {
		last := &merged[len(merged)-1]
		if st.t0-last.t1 < 2*stretchPad+p.Lmax+2*stepSlack {
			last.t1 = st.t1
			continue
		}
		merged = append(merged, st)
	}
	for _, st := range merged {
		if !p.exactStretch(s, a, b, st) {
			return false
		}
	}
	return true
}

// exactStretch is the exact check of one stretch: a search over every
// rounded (X, Y, Z) state the step chain can reach, starting before the
// stretch and stopping past it. From each state, every rounded point that a
// step of at most Lmax can reach must pass CanMove with the Z the chain
// carries, Height(next, Z + 2*CellHeight), the state's own point included (a
// short update rounds back onto it); a state within Lmax of b must also pass
// CanMove onto b itself (the final step lands on the destination).
//
// Reachable points are rounded points within bandRadius of the line, at most
// Lmax + stepSlack ahead, and between the state and b on each axis (the chain
// is monotone per axis, see quadrant). The first step is bounded like any
// other: the server times the first update from the move request (acis_golang
// model/actor/move/creature.go, lastUpdate set with the move). So a stretch
// near a starts from a alone; any other starts from every band point within
// Lmax + stepSlack before it, on the walking layer the corridor gave its cell
// (corridorAdd), and every state it reaches past the stretch must stand on
// that layer too, where the corridor test takes over. A state in water fails
// the stretch, the server would swim the walker from there, unless the
// planner swims: then the state goes on as a swimmer (swimStates). Each
// CanMove it makes counts toward s.work.
func (p *Planner) exactStretch(s *scratch, a, b location.Location, st stretch) bool {
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	length := math.Hypot(dx, dy)
	reach := p.Lmax + stepSlack
	t0, t1 := st.t0-stretchPad, st.t1+stretchPad
	lo := t0 - reach
	if t0 <= 0 {
		lo = 0
	}
	s.band = appendBand(s.band[:0], a, b, lo-stepSlack, t1+reach)
	bz := int(p.Engine.Height(b.X, b.Y, b.Z))

	s.seen.reset()
	s.queue = s.queue[:0]
	if t0 <= 0 {
		s.addState(chainState{int32(a.X), int32(a.Y), int16(a.Z)})
	} else {
		for _, bp := range s.band {
			if bp.t >= lo && bp.t <= t0 {
				z, ok := s.grid.floorAt(engine.GeoX(bp.x), engine.GeoY(bp.y))
				if !ok {
					return s.failedAt(bp.x, bp.y)
				}
				s.addState(chainState{int32(bp.x), int32(bp.y), z})
			}
		}
	}

	for head := 0; head < len(s.queue); head++ {
		cur := s.queue[head]
		cx, cy, cz := int(cur.x), int(cur.y), int(cur.z)
		if len(s.queue) > maxChainStates {
			return s.failedAt(cx, cy)
		}
		ct := (float64(cx-a.X)*dx + float64(cy-a.Y)*dy) / length
		if p.Water.Contains(cx, cy, cz) {
			if !p.swims() {
				return s.failedAt(cx, cy)
			}
			if ct <= t1 {
				p.swimStates(s, cur, b, bz, ct, reach)
			}
			continue
		}
		if ct > t1 {
			// Past the stretch the corridor test holds from here, on its
			// walking layers.
			if z, ok := s.grid.floorAt(engine.GeoX(cx), engine.GeoY(cy)); !ok || p.Engine.Height(cx, cy, cz) != z {
				return s.failedAt(cx, cy)
			}
			continue
		}
		s.work += 2
		if hypot(b.X-cx, b.Y-cy) <= p.Lmax && !p.Engine.CanMove(cx, cy, cz, b.X, b.Y, bz) {
			return false
		}
		// A short update rounds back onto cur itself, and the chain re-floors
		// it like any other step: a layer just above cz wins and blocks.
		if sz := p.Engine.Height(cx, cy, cz+2*block.CellHeight); !p.Engine.CanMove(cx, cy, cz, cx, cy, int(sz)) {
			return s.failedAt(cx, cy)
		}
		first := sort.Search(len(s.band), func(i int) bool { return s.band[i].t >= ct-stepSlack })
		for _, bp := range s.band[first:] {
			if bp.t > ct+reach {
				break
			}
			if (bp.x == cx && bp.y == cy) || !between(cx, bp.x, b.X) || !between(cy, bp.y, b.Y) ||
				hypot(bp.x-cx, bp.y-cy) > reach {
				continue
			}
			nz := p.Engine.Height(bp.x, bp.y, cz+2*block.CellHeight)
			s.work++
			if !p.Engine.CanMove(cx, cy, cz, bp.x, bp.y, int(nz)) {
				return s.failedAt(cx, cy)
			}
			s.addState(chainState{int32(bp.x), int32(bp.y), nz})
		}
	}
	return true
}

// failedAt records (x, y) as where the exact check failed and returns
// false.
func (s *scratch) failedAt(x, y int) bool {
	s.failAt, s.failed = [2]int{x, y}, true
	return false
}

func (s *scratch) addState(st chainState) {
	if _, seen := s.seen.getOrPut(st.key(), 0); seen {
		return
	}
	s.queue = append(s.queue, st)
}

// key packs the state into an indexTable key: world coordinates offset into
// the world fit in 24 bits each, the Z in 16.
func (st chainState) key() uint64 {
	x, y := uint32(int(st.x)-geo.WorldXMin)&0xFFFFFF, uint32(int(st.y)-geo.WorldYMin)&0xFFFFFF
	return uint64(x)<<40 | uint64(y)<<16 | uint64(uint16(st.z))
}

// appendBand appends every rounded point within bandRadius of the line from
// a to b, inside the box a and b span, at a distance along the line in
// [lo, hi], sorted by that distance.
func appendBand(dst []bandPoint, a, b location.Location, lo, hi float64) []bandPoint {
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	length := math.Hypot(dx, dy)
	minX, maxX := min(a.X, b.X), max(a.X, b.X)
	minY, maxY := min(a.Y, b.Y), max(a.Y, b.Y)
	add := func(x, y int) {
		if x < minX || x > maxX || y < minY || y > maxY {
			return
		}
		rx, ry := float64(x-a.X), float64(y-a.Y)
		t := (rx*dx + ry*dy) / length
		if t < lo || t > hi || math.Abs(rx*dy-ry*dx)/length > bandRadius {
			return
		}
		dst = append(dst, bandPoint{x, y, t})
	}
	// Walk the major axis; the band is at most 2·bandRadius·length/|major|
	// ≤ 2√2·bandRadius wide across it.
	span := int(math.Ceil(bandRadius*math.Sqrt2)) + 1
	// A band point's distance along the line is within bandRadius of its
	// major coordinate's projection; clip the walk to [lo, hi] with margin.
	clip := func(from, delta, lower, upper int) (int, int) {
		e0 := float64(from) + (lo-2*bandRadius)*float64(delta)/length
		e1 := float64(from) + (hi+2*bandRadius)*float64(delta)/length
		return max(lower, int(math.Floor(min(e0, e1)))), min(upper, int(math.Ceil(max(e0, e1))))
	}
	if math.Abs(dx) >= math.Abs(dy) {
		x0, x1 := clip(a.X, b.X-a.X, minX, maxX)
		for x := x0; x <= x1; x++ {
			yl := float64(a.Y) + float64(x-a.X)*dy/dx
			for y := int(math.Floor(yl)) - span; y <= int(math.Ceil(yl))+span; y++ {
				add(x, y)
			}
		}
	} else {
		y0, y1 := clip(a.Y, b.Y-a.Y, minY, maxY)
		for y := y0; y <= y1; y++ {
			xl := float64(a.X) + float64(y-a.Y)*dx/dy
			for x := int(math.Floor(xl)) - span; x <= int(math.Ceil(xl))+span; x++ {
				add(x, y)
			}
		}
	}
	slices.SortFunc(dst, func(p, q bandPoint) int {
		switch {
		case p.t < q.t:
			return -1
		case p.t > q.t:
			return 1
		}
		return 0
	})
	return dst
}

// between reports whether v lies between from and to, inclusive.
func between(from, v, to int) bool {
	return min(from, to) <= v && v <= max(from, to)
}

func hypot(dx, dy int) float64 {
	return math.Hypot(float64(dx), float64(dy))
}

// swimStates adds the states a swimmer at cur reaches on its way to b
// (floor bz): it moves straight in 3D toward b and nothing under way stops it
// (acis_golang model/actor/move/creature.go stepLocked, catchUpLocked), so
// while the line stays in water it needs no check. Where the line leaves
// water the server walks it again: every band point ahead within a step past
// the exit is a state, at the height the line has there, from which the
// chain goes on by the ground rules. A line in water as far as b adds none.
func (p *Planner) swimStates(s *scratch, cur chainState, b location.Location, bz int, ct, reach float64) {
	c := location.Location{X: int(cur.x), Y: int(cur.y), Z: int(cur.z)}
	end := location.Location{X: b.X, Y: b.Y, Z: bz}
	t := p.Water.exit(c, end)
	if t >= 1 {
		return
	}
	total := hypot(end.X-c.X, end.Y-c.Y)
	if total == 0 {
		return
	}
	out := t * total
	first := sort.Search(len(s.band), func(i int) bool { return s.band[i].t >= ct-stepSlack })
	for _, bp := range s.band[first:] {
		if bp.t > ct+out+reach {
			break
		}
		if (bp.x == c.X && bp.y == c.Y) || !between(c.X, bp.x, end.X) || !between(c.Y, bp.y, end.Y) {
			continue
		}
		d := hypot(bp.x-c.X, bp.y-c.Y)
		if d < out-stepSlack || d > out+reach {
			continue
		}
		z := float64(c.Z) + float64(end.Z-c.Z)*d/total
		s.addState(chainState{int32(bp.x), int32(bp.y), int16(math.Round(z))})
	}
}
