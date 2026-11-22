package nav

import (
	"math"
	"slices"
	"sort"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
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
	return p.stepChainOK(s, a, b)
}

func (p *Planner) stepChainOK(s *scratch, a, b location.Location) bool {
	if a.X == b.X && a.Y == b.Y {
		return true // the first update lands on b: CanMove alone decides
	}
	stretches := p.corridorStretches(s, a, b)
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
// is monotone per axis, see quadrant). The first step can be any length, so
// a stretch near a starts from a alone; any other starts from every band
// point within Lmax + stepSlack before it, on its one floor. A state in
// water fails the stretch: the server would swim the walker from there.
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

	clear(s.seen)
	s.queue = s.queue[:0]
	if t0 <= 0 {
		s.addState(chainState{int32(a.X), int32(a.Y), int16(a.Z)})
	} else {
		for _, bp := range s.band {
			if bp.t >= lo && bp.t <= t0 {
				z := p.Engine.Height(bp.x, bp.y, a.Z)
				s.addState(chainState{int32(bp.x), int32(bp.y), z})
			}
		}
	}

	for head := 0; head < len(s.queue); head++ {
		if len(s.queue) > maxChainStates {
			return false
		}
		cur := s.queue[head]
		cx, cy, cz := int(cur.x), int(cur.y), int(cur.z)
		if p.Water.Contains(cx, cy, cz) {
			return false
		}
		ct := (float64(cx-a.X)*dx + float64(cy-a.Y)*dy) / length
		if ct > t1 {
			continue // past the stretch: the corridor test holds from here
		}
		if hypot(b.X-cx, b.Y-cy) <= p.Lmax && !p.Engine.CanMove(cx, cy, cz, b.X, b.Y, bz) {
			return false
		}
		// A short update rounds back onto cur itself, and the chain re-floors
		// it like any other step: a layer just above cz wins and blocks.
		if sz := p.Engine.Height(cx, cy, cz+2*block.CellHeight); !p.Engine.CanMove(cx, cy, cz, cx, cy, int(sz)) {
			return false
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
			if !p.Engine.CanMove(cx, cy, cz, bp.x, bp.y, int(nz)) {
				return false
			}
			s.addState(chainState{int32(bp.x), int32(bp.y), nz})
		}
	}
	return true
}

func (s *scratch) addState(st chainState) {
	if _, ok := s.seen[st]; ok {
		return
	}
	s.seen[st] = struct{}{}
	s.queue = append(s.queue, st)
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
