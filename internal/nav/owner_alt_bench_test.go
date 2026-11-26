package nav

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// benchNoLandmarks measures the warm distances and routes of the sampled
// pairs again over h's graph without its landmark section (#315), and
// reports how many answers differ from h's: no distance may, a route only
// where the abstract search broke a tie another way. routes are h's warm
// routes. It returns the hierarchy without landmarks.
func benchNoLandmarks(b *testing.B, h *Hierarchy, p *Planner, from, to []Cell, routes [][]location.Location) *Hierarchy {
	n := len(from)
	plain := *h.graph
	plain.counts.landmarks = 0
	ph := NewHierarchy(&plain, h.static.Engine, h.static.Water, 4<<30)
	members := func(h *Hierarchy, i int) (Member, Member) {
		ma, err := h.Locate(from[i], nil)
		if err != nil {
			b.Fatal(err)
		}
		mb, err := h.Locate(to[i], nil)
		if err != nil {
			b.Fatal(err)
		}
		return ma, mb
	}
	// Warm, as h is: every cluster located and the pages read once.
	for i := range n {
		ma, mb := members(ph, i)
		if _, err := ph.Distance(ma, mb); err != nil {
			b.Fatal(err)
		}
	}
	dist := make([]int, n)
	b.Run("DistanceWarmNoLandmarks", func(b *testing.B) {
		measure(b, n, func(i int) int64 {
			ma, mb := members(ph, i)
			d, err := ph.Distance(ma, mb)
			if err != nil {
				b.Fatal(err)
			}
			dist[i] = d
			return 0
		})
	})
	differ := 0
	for i := range n {
		ma, mb := members(h, i)
		if d, _ := h.Distance(ma, mb); d != dist[i] {
			differ++
		}
	}
	b.Logf("distances with %d landmarks differing from the octile search's: %d of %d", h.graph.counts.landmarks, differ, n)
	differ = 0
	failed, plainFailed := 0, 0
	b.Run("RouteWarmNoLandmarks", func(b *testing.B) {
		measure(b, n, func(i int) int64 {
			r, err := ph.Route(p, from[i].World(), to[i].World(), nil)
			if err != nil {
				plainFailed++
			}
			if routes[i] == nil {
				failed++
			}
			if !slices.Equal(r, routes[i]) {
				differ++
			}
			return 0
		})
	})
	b.Logf("warm routes with landmarks differing from the octile search's: %d of %d; failed %d with landmarks, %d without", differ, n, failed, plainFailed)
	return ph
}
