package nav

import (
	"errors"
	"math"

	"github.com/fatal10110/acis_golang_bot/internal/geo"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// ErrBreath: the only way the search found swims a stretch longer than
// Planner.Swim, the breath the walker allows one stretch of water.
var ErrBreath = errors.New("nav: route swims farther than breath allows")

const (
	// swimCost multiplies the cost of a step into a wet cell. A swimmer
	// moves at its swim speed, about half its run speed for the shipped
	// templates, and spends breath, so a route swims only where the dry way
	// round is over three times as long (docs/navigation.md, "Swimming").
	swimCost = 3
	// maxSwimBoxes bounds how many touching water zones one swim line is
	// followed through.
	maxSwimBoxes = 8
)

// line3 is the straight line from p to p+d, at parameter t in [0, 1]: the
// path a swimmer takes, its height moving straight toward the destination's
// (acis_golang model/actor/move/creature.go stepLocked).
type line3 struct {
	p, d [3]float64
}

func newLine3(a, b location.Location) line3 {
	return line3{
		p: [3]float64{float64(a.X), float64(a.Y), float64(a.Z)},
		d: [3]float64{float64(b.X - a.X), float64(b.Y - a.Y), float64(b.Z - a.Z)},
	}
}

func (l line3) length() float64 {
	return math.Sqrt(l.d[0]*l.d[0] + l.d[1]*l.d[1] + l.d[2]*l.d[2])
}

// clip returns the part [t0, t1] of l, within [0, 1], inside the box; ok is
// false when l misses it.
func (b *WaterBox) clip(l line3) (t0, t1 float64, ok bool) {
	lo := [3]float64{float64(b.X1), float64(b.Y1), float64(b.MinZ)}
	hi := [3]float64{float64(b.X2), float64(b.Y2), float64(b.MaxZ)}
	t0, t1 = 0, 1
	for k := range 3 {
		if l.d[k] == 0 {
			if l.p[k] < lo[k] || l.p[k] > hi[k] {
				return 0, 0, false
			}
			continue
		}
		u, v := (lo[k]-l.p[k])/l.d[k], (hi[k]-l.p[k])/l.d[k]
		if u > v {
			u, v = v, u
		}
		t0, t1 = max(t0, u), min(t1, v)
		if t0 > t1 {
			return 0, 0, false
		}
	}
	return t0, t1, true
}

// lineBoxes calls fn with every box in a bucket the line's horizontal extent
// overlaps; a box in several such buckets comes more than once.
func (w *Water) lineBoxes(l line3, fn func(b *WaterBox)) {
	x0, x1 := l.p[0], l.p[0]+l.d[0]
	y0, y1 := l.p[1], l.p[1]+l.d[1]
	bx0, bx1 := bucketIndex(int(min(x0, x1)), geo.WorldXMin, bucketsX), bucketIndex(int(max(x0, x1)), geo.WorldXMin, bucketsX)
	by0, by1 := bucketIndex(int(min(y0, y1)), geo.WorldYMin, bucketsY), bucketIndex(int(max(y0, y1)), geo.WorldYMin, bucketsY)
	for bx := bx0; bx <= bx1; bx++ {
		for by := by0; by <= by1; by++ {
			for _, i := range w.bucket(bx, by) {
				fn(&w.boxes[i])
			}
		}
	}
}

// Span returns the part of the straight line from a to b inside water: t0
// is where it first enters a water zone and t1 where it last leaves one, as
// fractions of the line. ok is false when the line touches no water. A
// stretch of dry line between two zones counts as water.
func (w *Water) Span(a, b location.Location) (t0, t1 float64, ok bool) {
	if w == nil {
		return 0, 0, false
	}
	l := newLine3(a, b)
	t0, t1 = 2, -1
	w.lineBoxes(l, func(bx *WaterBox) {
		if u, v, hit := bx.clip(l); hit {
			t0, t1 = min(t0, u), max(t1, v)
		}
	})
	return t0, t1, t1 >= t0
}

// Near reports whether a water zone lies within margin of the box a and b
// span, horizontally and in height. It is a cheap test for whether a route
// between them may involve water at all.
func (w *Water) Near(a, b location.Location, margin int) bool {
	if w == nil {
		return false
	}
	x0, x1 := min(a.X, b.X)-margin, max(a.X, b.X)+margin
	y0, y1 := min(a.Y, b.Y)-margin, max(a.Y, b.Y)+margin
	z0, z1 := min(a.Z, b.Z)-margin, max(a.Z, b.Z)+margin
	found := false
	l := line3{p: [3]float64{float64(x0), float64(y0)}, d: [3]float64{float64(x1 - x0), float64(y1 - y0)}}
	w.lineBoxes(l, func(bx *WaterBox) {
		if bx.X1 <= x1 && bx.X2 >= x0 && bx.Y1 <= y1 && bx.Y2 >= y0 && bx.MinZ <= z1 && bx.MaxZ >= z0 {
			found = true
		}
	})
	return found
}

// exit returns how far a swimmer starting at a in water swims along the
// straight line to b before it leaves water, as a fraction of the line: 1
// when it is still in water at b, 0 when a is not in water. Touching zones
// (one unit apart, as the server's integer positions see them) chain.
func (w *Water) exit(a, b location.Location) float64 {
	if w == nil {
		return 0
	}
	l := newLine3(a, b)
	length := l.length()
	if length == 0 {
		if w.Contains(a.X, a.Y, a.Z) {
			return 1
		}
		return 0
	}
	eps := 1 / length
	t := 0.0
	for range maxSwimBoxes {
		far := -1.0
		w.lineBoxes(l, func(bx *WaterBox) {
			if u, v, hit := bx.clip(l); hit && u <= t+eps && v >= t {
				far = max(far, v)
			}
		})
		if far < 0 {
			return t
		}
		if far >= 1 {
			return 1
		}
		if far <= t {
			return t
		}
		t = far
	}
	return t
}

// swims reports whether the planner may route through water: it has water
// and a breath to swim on.
func (p *Planner) swims() bool {
	return p.Swim > 0 && p.Water != nil
}

// inWater reports whether the world point is in a water zone.
func (p *Planner) inWater(at location.Location) bool {
	return p.Water.Contains(at.X, at.Y, at.Z)
}

// swimLeg reports whether the segment from a, in water, to b is one swim the
// server takes straight: CanMove from a to b, a floor at or below b
// (resolvePathLocked's swim branch), and the straight line from a to b in
// water all the way, so the swimmer never turns walker on the way and no
// closed line under way stops it.
func (p *Planner) swimLeg(a, b location.Location) bool {
	return p.Water.exit(a, b) >= 1 &&
		p.Engine.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) &&
		p.Engine.HasLayerAtOrBelow(b.X, b.Y, b.Z)
}

// breathOK reports whether no stretch of route in water is longer than
// p.Swim: the in-water part of each segment, measured in 3D as the server
// measures a swimmer's leg, adds up until a segment ends out of water.
func (p *Planner) breathOK(route []location.Location) bool {
	run := 0.0
	for i := 1; i < len(route); i++ {
		a, b := route[i-1], route[i]
		if t0, t1, ok := p.Water.Span(a, b); ok {
			run += (t1 - t0) * newLine3(a, b).length()
		}
		if run > p.Swim {
			return false
		}
		if !p.inWater(b) {
			run = 0
		}
	}
	return true
}
