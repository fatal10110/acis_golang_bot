package nav

import (
	"errors"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// ownerSameClusterSample picks n pairs of cells of the largest abstract
// SCC, both ends in one random cluster and fewer than ClusterCells cells
// apart (4096 units at the default side), each with a click target.
func ownerSameClusterSample(b *testing.B, e *engine.Engine, g *Graph, w *Water, n int) (from, to []Cell) {
	b.Helper()
	h := NewHierarchy(g, e, w, 1<<30)
	p := &Planner{Engine: e, CollisionHeight: 23, Water: w}
	sizes := map[int32]int{}
	for i := range g.Nodes() {
		sizes[g.nodeSCC(i)]++
	}
	big, most := int32(0), 0
	for s, c := range sizes {
		if c > most {
			big, most = s, c
		}
	}
	rng := rand.New(rand.NewPCG(13, 14))
	var buf [block.MaxLayers]block.Cell
	pick := func(x, y int) (Cell, bool) {
		if !p.hasGeo(x, y) {
			return Cell{}, false
		}
		layers := e.AppendLayers(buf[:0], x, y)
		c := Cell{X: x, Y: y, Z: layers[rng.IntN(len(layers))].Height}
		if !p.clickable(c, &buf) {
			return Cell{}, false
		}
		m, err := h.Locate(c, nil)
		return c, err == nil && m.Kind() == InSCC && m.SCC() == big
	}
	size := g.Key().ClusterCells
	for tries := 0; len(from) < n && tries < 200*n; tries++ {
		x0, y0 := g.clusterOrigin(rng.IntN(g.Clusters()))
		a, ok := pick(x0+rng.IntN(size), y0+rng.IntN(size))
		if !ok {
			continue
		}
		for range 20 {
			x, y := x0+rng.IntN(size), y0+rng.IntN(size)
			if hypot(x-a.X, y-a.Y) >= float64(size) {
				continue
			}
			if c, ok := pick(x, y); ok {
				from, to = append(from, a), append(to, c)
				break
			}
		}
	}
	if len(from) < n/2 {
		b.Fatalf("sampled only %d pairs", len(from))
	}
	return from, to
}

// routeLength is the length of route in units, along the ground plane.
func routeLength(route []location.Location) float64 {
	l := 0.0
	for k := 1; k < len(route); k++ {
		l += hypot(route[k].X-route[k-1].X, route[k].Y-route[k-1].Y)
	}
	return l
}

// BenchmarkOwnerNavSameCluster routes pairs of one cluster on the owner's
// geodata twice through one warm hierarchy: before #648, the free cell
// search alone (no cap), and after, the free search up to the cap and the
// seated route past it. It times both and reports how many pairs the free
// search takes past the cap, how many outcomes change, and how many routes
// get longer and by how much. Run it once:
// go test -run '^$' -bench OwnerNavSameCluster -benchtime 1x ./internal/nav/
func BenchmarkOwnerNavSameCluster(b *testing.B) {
	e, g, w := ownerNav(b)
	const samples = 1000
	from, to := ownerSameClusterSample(b, e, g, w, samples)
	n := len(from)
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 400000, Water: w}
	h := NewHierarchy(g, e, w, 4<<30)
	s := getScratch()
	defer putScratch(s)
	limit := h.sameClusterCap()
	past := 0
	for i := range n {
		if _, err := h.Locate(from[i], nil); err != nil {
			b.Fatal(err)
		}
		if _, err := h.Locate(to[i], nil); err != nil {
			b.Fatal(err)
		}
		dry := *p
		dry.Swim = 0
		if _, _, err := dry.search(s, from[i], to[i], limit, nil, anywhere); errors.Is(err, ErrBudget) {
			past++
		}
	}
	before, after := make([]sameClusterOutcome, n), make([]sameClusterOutcome, n)
	for b.Loop() {
		pass := func(name string, sameCap int, out []sameClusterOutcome) {
			h.sameCap = sameCap
			defer func() { h.sameCap = 0 }()
			b.Run(name, func(b *testing.B) {
				failed := 0
				measure(b, n, func(i int) int64 {
					r, err := h.Route(p, from[i].World(), to[i].World(), nil)
					if err != nil {
						failed++
					}
					out[i] = sameClusterOutcome{r, err}
					return 0
				})
				b.Logf("routes failed: %d of %d", failed, n)
			})
		}
		pass("RouteBefore", math.MaxInt, before)
		pass("RouteAfter", 0, after)
		b.Run("Compare", func(b *testing.B) { compareSameCluster(b, from, to, before, after, limit, past) })
	}
}

// sameClusterOutcome is one route of BenchmarkOwnerNavSameCluster.
type sameClusterOutcome struct {
	route []location.Location
	err   error
}

// compareSameCluster logs how the routes after #648 differ from those
// before: outcomes changed, routes changed, and how much longer.
func compareSameCluster(b *testing.B, from, to []Cell, before, after []sameClusterOutcome, limit, past int) {
	n := len(from)
	b.Logf("%d same-cluster pairs; the free search passes the cap of %d cells for %d", n, limit, past)
	fixed, broke, differ, longer := 0, 0, 0, 0
	var extra, ratio []float64
	for i := range n {
		bo, ao := before[i], after[i]
		switch {
		case bo.err != nil && ao.err == nil:
			fixed++
		case bo.err == nil && ao.err != nil:
			broke++
			b.Logf("pair %d %v -> %v: routed before, %v after", i, from[i], to[i], ao.err)
		case bo.err == nil && !slices.Equal(bo.route, ao.route):
			differ++
			lb, la := routeLength(bo.route), routeLength(ao.route)
			if la > lb+0.5 {
				longer++
				extra = append(extra, la-lb)
				ratio = append(ratio, la/lb)
			}
		case bo.err != nil && !errors.Is(ao.err, bo.err):
			b.Logf("pair %d %v -> %v: %v before, %v after", i, from[i], to[i], bo.err, ao.err)
		}
	}
	b.Logf("outcomes: %d routed after failing before, %d failed after routing before; %d routes differ, %d longer",
		fixed, broke, differ, longer)
	if len(extra) > 0 {
		slices.Sort(extra)
		slices.Sort(ratio)
		sum := 0.0
		for _, x := range extra {
			sum += x
		}
		b.Logf("longer routes: extra units mean %.0f, median %.0f, max %.0f; length ratio median %.3f, max %.3f",
			sum/float64(len(extra)), extra[len(extra)/2], extra[len(extra)-1], ratio[len(ratio)/2], ratio[len(ratio)-1])
	}
}
