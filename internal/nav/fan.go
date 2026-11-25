package nav

import (
	"math"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

const (
	// FanBand is how far off its stretch's line a fan's origin may lie: the
	// server's position on a walk is a rounded step point within bandRadius
	// of the walk's line, and a catch-up step lands on one too.
	FanBand = 2
	// fanSpacing is the widest gap along the stretch between two lines
	// FanOpen checks: half a cell.
	fanSpacing = block.CellSize / 2
)

// FanOpen reports whether a click to b holds from every point within
// FanBand of the stretch from a0 to a1 (docs/navigation.md, "Click-ahead"):
// the fan of lines a click sent while the bot walks that stretch can start
// from, the server's catch-up point being anywhere on it. It checks lines
// from a0, a1 and points between them at most fanSpacing apart, each point
// on the layer nearest the stretch's height there:
//
//   - CanMove to b at head height, as the server checks a click (the
//     target's Z raised by the collision height, network/movement.go);
//   - the open-corridor test, with the NSWE directions of the union of the
//     quadrants of every line of the fan, and the reach of the cells next to
//     the line widened by how far a line between two checked ones, from a
//     point up to FanBand off the stretch, strays from the nearer one;
//   - no avoided column (Planner.Avoid) on the line.
//
// The exact check vouches for one line only, not for the lines between two
// checked ones, so a fan needs the corridor test throughout. Swimming is not
// covered: the caller keeps fans on land.
func (p *Planner) FanOpen(a0, a1, b location.Location) bool {
	s := getScratch()
	defer putScratch(s)
	s.grid.invalidate()
	req := fanQuadrant(a0, a1, b)
	reach := corridorReach + math.Hypot(fanSpacing/2, FanBand)
	head := b
	head.Z += int(p.CollisionHeight)
	n := max(int(math.Ceil(hypot(a1.X-a0.X, a1.Y-a0.Y)/fanSpacing)), 1)
	for i := range n + 1 {
		t := float64(i) / float64(n)
		at := location.Location{
			X: a0.X + int(math.Round(t*float64(a1.X-a0.X))),
			Y: a0.Y + int(math.Round(t*float64(a1.Y-a0.Y))),
			Z: a0.Z + int(math.Round(t*float64(a1.Z-a0.Z))),
		}
		c, ok := p.Locate(at)
		if !ok {
			return false
		}
		at.Z = int(c.Z)
		if at.X == b.X && at.Y == b.Y {
			continue // the click lands on its own cell: nothing to cross
		}
		if !p.Engine.CanMove(at.X, at.Y, at.Z, head.X, head.Y, head.Z) ||
			p.avoided(s, at, b, at, b) ||
			!p.corridorFillWith(s, at, b, req, reach, true) {
			return false
		}
	}
	return true
}

// fanQuadrant is the union of the quadrants of the lines from every point
// within FanBand of the stretch from a0 to a1 to b: the corners of the band
// around both ends bound every such point's offset to b on each axis.
func fanQuadrant(a0, a1, b location.Location) block.NSWE {
	var n block.NSWE
	for _, a := range [2]location.Location{a0, a1} {
		for _, d := range [4][2]int{{-FanBand, -FanBand}, {-FanBand, FanBand}, {FanBand, -FanBand}, {FanBand, FanBand}} {
			n |= quadrant(location.Location{X: a.X + d[0], Y: a.Y + d[1]}, b)
		}
	}
	return n
}
