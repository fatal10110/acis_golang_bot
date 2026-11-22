package nav

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// BenchmarkLocateCold measures one cluster result computed from nothing: a
// 256-cell cluster of the synthetic region, Tarjan included.
func BenchmarkLocateCold(b *testing.B) {
	th := hierScenes256(b)
	c := pointCell(geotest.PitOutside)
	b.ReportAllocs()
	for b.Loop() {
		h := NewHierarchy(th.g, testEngine(b), DefaultMemberCacheBytes)
		if _, err := h.Locate(c, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLocateSameSCCWarm measures two Locates and SameSCC on a warm LRU.
func BenchmarkLocateSameSCCWarm(b *testing.B) {
	th := hierScenes256(b)
	a, c := pointCell(geotest.PitOutside), pointCell(geotest.PocketOutside)
	b.ReportAllocs()
	for b.Loop() {
		ma, err := th.h.Locate(a, nil)
		if err != nil {
			b.Fatal(err)
		}
		mc, err := th.h.Locate(c, nil)
		if err != nil {
			b.Fatal(err)
		}
		if !th.h.SameSCC(ma, mc) {
			b.Fatal("not SameSCC")
		}
	}
}

// BenchmarkHierRoute measures a route across 32-cell clusters, around the
// U: abstract search, refinement and pulling.
func BenchmarkHierRoute(b *testing.B) {
	th := hierScenes32(b)
	p := testPlanner(b)
	from, to := loc(geotest.UInside), loc(geotest.UBehind)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := th.h.Route(p, from, to, nil); err != nil {
			b.Fatal(err)
		}
	}
}
