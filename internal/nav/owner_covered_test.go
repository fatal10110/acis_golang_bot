package nav

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// Orc village points of #640: the town point the swarm stood around, the
// trainers inside the west building, and two outdoor points flanking that
// building west and east on its row (y about -113656, x -45820 to -47300,
// floor -200 under the roofs): the hall through the village's middle.
var (
	orcTown     = location.Location{X: -45264, Y: -112512, Z: -200}
	orcTrainers = location.Location{X: -45840, Y: -113504, Z: -200}
	orcWest     = location.Location{X: -47700, Y: -113656, Z: -200}
	orcEast     = location.Location{X: -45400, Y: -113656, Z: -200}
)

// coveredPoints counts the points, every 16 units along route, whose cell is
// covered ground (Planner.isCovered) on the layer nearest the route's Z.
func coveredPoints(p *Planner, route []location.Location) int {
	var buf [block.MaxLayers]block.Cell
	n := 0
	for i := 1; i < len(route); i++ {
		a, b := route[i-1], route[i]
		steps := max(1, int(segLen(a, b)/16))
		for k := 0; k < steps; k++ {
			f := float64(k) / float64(steps)
			at := location.Location{X: a.X + int(f*float64(b.X-a.X)), Y: a.Y + int(f*float64(b.Y-a.Y)), Z: a.Z + int(f*float64(b.Z-a.Z))}
			if c, ok := p.Locate(at); ok && p.isCovered(c, &buf) {
				n++
			}
		}
	}
	return n
}

// nearestOpen is the point nearest at, within r cells, whose cell (on the
// layer nearest at's Z) is open ground, a click target and of target's SCC.
func nearestOpen(t *testing.T, h *Hierarchy, p *Planner, at location.Location, target Member, r int) (location.Location, bool) {
	t.Helper()
	var buf [block.MaxLayers]block.Cell
	gx, gy := engine.GeoX(at.X), engine.GeoY(at.Y)
	for d := 0; d <= r; d++ {
		for dy := -d; dy <= d; dy++ {
			for dx := -d; dx <= d; dx++ {
				if max(abs(dx), abs(dy)) != d {
					continue
				}
				l := location.Location{X: engine.WorldX(gx + dx), Y: engine.WorldY(gy + dy), Z: at.Z}
				c, ok := p.Locate(l)
				if !ok || p.isCovered(c, &buf) || !p.clickable(c, &buf) || p.Water.wet(c.X, c.Y, c.Z) {
					continue
				}
				if m, err := h.Locate(c, nil); err == nil && h.SameSCC(m, target) {
					l.Z = int(c.Z)
					return l, true
				}
			}
		}
	}
	return location.Location{}, false
}

// TestOwnerOrcCovered is the offline check of #640 on the owner's data
// (ownerNav's variables): around the Orc village's town point it routes the
// pair flanking the west building, the trainers inside it, and the routes
// from the town point to open ground on a 200-unit grid within 2500 of it,
// and logs how many points of each route lie under a roof, its length
// against the straight line and the open-ground targets whose route runs
// under one. Run it against nav caches built with different -covered costs
// to compare. The west building is the village's east-west passage: its
// flanking pair's only way round circles the village (8764 units for a line
// of 2304), so the pair keeps the hall below a covered cost of about 60. It
// fails when a route fails or the trainers' route never enters the building.
func TestOwnerOrcCovered(t *testing.T) {
	e, g, w := ownerNav(t)
	h := NewHierarchy(g, e, w, 1<<30)
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 400000, Water: w}
	t.Logf("nav cache covered cost %d", g.CoveredCost())
	ps := p.withCovered(g.covered)
	townCell, ok := p.Locate(orcTown)
	if !ok {
		t.Fatal("town point is not walkable")
	}
	town, err := h.Locate(townCell, nil)
	if err != nil {
		t.Fatal(err)
	}
	route := func(from, to location.Location) ([]location.Location, time.Duration, error) {
		began := time.Now()
		r, err := h.Route(p, from, to, nil)
		return r, time.Since(began), err
	}

	west, okW := nearestOpen(t, h, &ps, orcWest, town, 24)
	east, okE := nearestOpen(t, h, &ps, orcEast, town, 24)
	if !okW || !okE {
		t.Fatalf("no open ground of the town's SCC near %v (%v) or %v (%v)", orcWest, okW, orcEast, okE)
	}
	for _, pair := range [][2]location.Location{{west, east}, {east, west}} {
		r, took, err := route(pair[0], pair[1])
		if err != nil {
			t.Fatalf("%v -> %v: %v", pair[0], pair[1], err)
		}
		line, l, in := segLen(pair[0], pair[1]), routeLength(r), coveredPoints(&ps, r)
		t.Logf("flank %v -> %v: %d covered points, length %.0f (line %.0f, %.2fx), %d segments, %s", pair[0], pair[1], in, l, line, l/line, len(r)-1, took.Round(time.Microsecond))
	}
	r, took, err := route(orcTown, orcTrainers)
	if err != nil {
		t.Fatalf("town -> trainers: %v", err)
	}
	in := coveredPoints(&ps, r)
	t.Logf("town -> trainers: %d covered points, length %.0f, %s", in, routeLength(r), took.Round(time.Microsecond))
	if in == 0 {
		t.Errorf("trainers' route %v never enters the building", r)
	}

	// Town point to open ground nearby, as the swarm walked to the market,
	// its spots and the gatekeeper.
	var routes, indoor, points, failed int
	var length, lines float64
	var times []time.Duration
	var under []string
	tc, _ := p.Locate(orcTown)
	var buf [block.MaxLayers]block.Cell
	for dy := -2400; dy <= 2400; dy += 200 {
		for dx := -2400; dx <= 2400; dx += 200 {
			if math.Hypot(float64(dx), float64(dy)) > 2500 {
				continue
			}
			to := location.Location{X: orcTown.X + dx, Y: orcTown.Y + dy, Z: int(tc.Z)}
			c, ok := p.Locate(to)
			if !ok || ps.isCovered(c, &buf) || !p.clickable(c, &buf) || p.Water.wet(c.X, c.Y, c.Z) {
				continue
			}
			if m, err := h.Locate(c, nil); err != nil || !h.SameSCC(m, town) {
				continue
			}
			to.Z = int(c.Z)
			r, took, err := route(orcTown, to)
			if err != nil {
				failed++
				continue
			}
			routes++
			times = append(times, took)
			length += routeLength(r)
			lines += segLen(orcTown, to)
			if n := coveredPoints(&ps, r); n > 0 {
				indoor++
				points += n
				under = append(under, fmt.Sprintf("%+d%+d:%d", dx, dy, n))
			}
		}
	}
	slices.Sort(times)
	if len(times) == 0 {
		t.Fatal("no open ground routed around the town point")
	}
	t.Logf("town -> open ground: %d routes (%d failed), %d run under a roof (%d points), length %.0f (lines %.0f, %.3fx); time p50 %s, p99 %s",
		routes, failed, indoor, points, length, lines, length/lines,
		times[len(times)/2].Round(time.Microsecond), times[len(times)*99/100].Round(time.Microsecond))
	t.Logf("under a roof (offset from the town point: covered points): %s", strings.Join(under, " "))
}

// BenchmarkOwnerCovered routes BenchmarkOwnerNav's pairs once through a cold
// hierarchy and logs what weighing covered ground changes on the owner's
// data (#640): the routes that run under a roof and their points there, the
// total length, and the failures. Run it once against nav caches built with
// different -covered costs:
// go test -run '^$' -bench OwnerCovered -benchtime 1x ./internal/nav/
func BenchmarkOwnerCovered(b *testing.B) {
	e, g, w := ownerNav(b)
	from, to := ownerSample(b, e, g, w, 200, 2500)
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 400000, Water: w}
	ps := p.withCovered(g.covered)
	for b.Loop() {
		h := NewHierarchy(g, e, w, DefaultMemberCacheBytes)
		var indoor, points, failed int
		var length float64
		for i := range from {
			r, err := h.Route(p, from[i].World(), to[i].World(), nil)
			if err != nil {
				failed++
				b.Logf("pair %d %v -> %v: %v", i, from[i].World(), to[i].World(), err)
				continue
			}
			length += routeLength(r)
			if n := coveredPoints(&ps, r); n > 0 {
				indoor++
				points += n
			}
		}
		b.Logf("covered cost %d: %d pairs, %d failed, %d run under a roof (%d points), total length %.0f",
			g.CoveredCost(), len(from), failed, indoor, points, length)
	}
}
