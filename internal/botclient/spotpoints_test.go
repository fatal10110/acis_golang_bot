package botclient

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// TestSpotPointComputedOnce checks that a spot's point, asked by two bots
// sharing the spot registry from the same SCC, computes clusters the first
// time only, though the hierarchy's LRU keeps nothing (#496): the second
// answer is the first's, from the registry, its centre from the place table.
func TestSpotPointComputedOnce(t *testing.T) {
	tests := []struct {
		name string
		x    int32 // the spot's centre
	}{
		{name: "centre on open ground", x: 7000},
		{name: "centre on the wall, snapped west", x: 8224},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 5, testSpot("spot", tt.x, 2000, 3, 6, 9))
			n := *b.walk.nav
			n.Hier = nav.NewHierarchy(b.walk.nav.Hier.Graph(), n.Engine, n.Water, 0)
			b.walk.nav = &n
			other := newSpotBrain(t, 5)
			other.cfg.Spots, other.walk.nav = b.cfg.Spots, &n
			from := Point{X: 7900, Y: 2000}
			here, ok := b.locate(from)
			if !ok {
				t.Fatal("bot off the navigation")
			}
			there, ok := other.locate(Point{X: 7800, Y: 2100})
			if !ok {
				t.Fatal("other bot off the navigation")
			}
			first, ok := b.spotPoint(0, here)
			if !ok {
				t.Fatal("no spot point")
			}
			before := n.Hier.Stats()
			for i, ask := range []func() (navPoint, bool){
				func() (navPoint, bool) { return b.spotPoint(0, here) },
				func() (navPoint, bool) { return other.spotPoint(0, there) },
			} {
				got, ok := ask()
				if !ok || got.at != first.at || !n.Hier.SameSCC(got.m, here.m) {
					t.Fatalf("ask %d: %+v %v, want %+v", i, got.at, ok, first.at)
				}
			}
			if s := n.Hier.Stats(); s.Computed != before.Computed {
				t.Errorf("asking again computed %d clusters, want none", s.Computed-before.Computed)
			}
		})
	}
}
