package nav

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// shortDists are the partner distances, in cells along the axes, of the
// short pairs: 160 to 480 units, the walks a follower re-plans and an NPC
// approach takes (#565).
var shortDists = []int{10, 20, 30}

// benchShortRoutes runs the short pairs through the warm hierarchy h, once
// unmeasured with the full search alone to warm what the routes read, then
// measured with the full search alone (RouteShortSearch, Route before #565)
// and with the straight segment (RouteShort). Each measured pass logs the
// share of one-segment routes and the mean time of those and of the rest,
// and fails when a route differs from the warm-up's.
func benchShortRoutes(b *testing.B, h *Hierarchy, p *Planner, e *engine.Engine, g *Graph, w *Water) {
	const samples = 100
	var from, to []Cell
	for _, d := range shortDists {
		f, t := ownerSample(b, e, g, w, samples, d)
		from, to = append(from, f...), append(to, t...)
	}
	n := len(from)
	routes := make([][]location.Location, n)
	errs := make([]error, n)
	h.searchOnly = true
	for i := range n {
		routes[i], errs[i] = h.Route(p, from[i].World(), to[i].World(), nil)
	}
	h.searchOnly = false
	pass := func(name string, searchOnly bool) {
		b.Run(name, func(b *testing.B) {
			h.searchOnly = searchOnly
			defer func() { h.searchOnly = false }()
			var single, rest time.Duration
			singles, failed, differ := 0, 0, 0
			measure(b, n, func(i int) int64 {
				t := time.Now()
				r, err := h.Route(p, from[i].World(), to[i].World(), nil)
				d := time.Since(t)
				switch {
				case err != nil:
					failed++
					rest += d
				case len(r) == 2:
					singles++
					single += d
				default:
					rest += d
				}
				if (err == nil) != (errs[i] == nil) || !slices.Equal(r, routes[i]) {
					differ++
					b.Errorf("pair %d %v -> %v: route %v %v, the full search's %v %v", i, from[i], to[i], r, err, routes[i], errs[i])
				}
				return 0
			})
			mean := func(d time.Duration, k int) float64 { return float64(d.Microseconds()) / 1000 / float64(max(k, 1)) }
			b.Logf("%d pairs: %d single-segment (%.1f%%), mean %.3f ms; the other %d mean %.3f ms; %d failed, %d differ from the full search",
				n, singles, 100*float64(singles)/float64(n), mean(single, singles), n-singles, mean(rest, n-singles), failed, differ)
		})
	}
	pass("RouteShortSearch", true)
	pass("RouteShort", false)
}
