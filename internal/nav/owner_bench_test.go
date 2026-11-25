package nav

import (
	"math/rand/v2"
	"os"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geodata"
)

// ownerNav opens the owner's geodata cache and nav cache for the local
// performance benchmarks (docs/navigation.md, "nav performance"). The
// owner's geodata is private and never in CI, so the benchmarks skip
// unless BOT_NAV_GEODATA (region files dir), BOT_NAV_GEOENGINE,
// BOT_NAV_GEODATA_CACHE, BOT_NAV_DOORS (doors.xml) and BOT_NAV_CACHE are
// set. BOT_NAV_WATER (zones/WaterZone.xml) is the water the nav cache was
// built with; unset is no water.
func ownerNav(b testing.TB) (*engine.Engine, *Graph, *Water) {
	b.Helper()
	env := func(k string) string {
		v := os.Getenv(k)
		if v == "" {
			b.Skipf("%s not set: the owner-geodata benchmarks run locally only", k)
		}
		return v
	}
	dir, props, cachePath, doors, navPath := env("BOT_NAV_GEODATA"), env("BOT_NAV_GEOENGINE"), env("BOT_NAV_GEODATA_CACHE"), env("BOT_NAV_DOORS"), env("BOT_NAV_CACHE")
	settings, err := geodata.LoadSettings(props)
	if err != nil {
		b.Fatal(err)
	}
	c, err := geodata.Open(dir, settings.GeoType, settings.Regions, cachePath)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(c.Close)
	xml, err := os.ReadFile(doors)
	if err != nil {
		b.Fatal(err)
	}
	var water *Water
	if path := os.Getenv("BOT_NAV_WATER"); path != "" {
		zones, err := datapack.LoadWaterZones(path)
		if err != nil {
			b.Fatal(err)
		}
		boxes := make([]WaterBox, len(zones))
		for i, z := range zones {
			boxes[i] = WaterBox{X1: int(z.X1), Y1: int(z.Y1), X2: int(z.X2), Y2: int(z.Y2), MinZ: int(z.MinZ), MaxZ: int(z.MaxZ)}
		}
		water = NewWater(boxes)
	}
	g, err := OpenNavCache(navPath, NavKey{GeoCRC: c.CRC(), DoorsHash: DoorsHash(xml), ClusterCells: DefaultClusterCells, WaterHash: water.Hash()})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(g.Close)
	return c.Engine, g, water
}

// ownerSample picks n cells of the largest abstract SCC, each in its own
// random cluster, and for each a partner about dist cells away in the same
// SCC with a click target on it.
func ownerSample(b *testing.B, e *engine.Engine, g *Graph, w *Water, n, dist int) (from, to []Cell) {
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
	rng := rand.New(rand.NewPCG(11, 12))
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
			dx := rng.IntN(2*dist+1) - dist
			dy := dist - abs(dx)
			if rng.IntN(2) == 0 {
				dy = -dy
			}
			if c, ok := pick(a.X+dx, a.Y+dy); ok {
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

// report records the p50 and p99 of the timings and of the heap one query
// allocates, the cluster results it added to the LRU excluded (they are
// held, bounded by the LRU, not per-query heap), and the largest such heap.
func report(b *testing.B, times []time.Duration, heap []uint64) {
	slices.Sort(times)
	slices.Sort(heap)
	at := func(f float64) int { return int(f * float64(len(times)-1)) }
	ms := func(f float64) float64 { return float64(times[at(f)].Microseconds()) / 1000 }
	mb := func(f float64) float64 { return float64(heap[at(f)]) / 1e6 }
	b.ReportMetric(ms(0.5), "p50-ms")
	b.ReportMetric(ms(0.99), "p99-ms")
	b.ReportMetric(mb(0.99), "p99-heap-MB")
	b.ReportMetric(mb(1), "max-heap-MB")
	b.Logf("%d queries: p50 %.2f ms, p99 %.2f ms, max %.2f ms; heap per query p50 %.2f MB, p99 %.2f MB, max %.2f MB",
		len(times), ms(0.5), ms(0.99), ms(1), mb(0.5), mb(0.99), mb(1))
}

// measure times fn over every sample and tracks the bytes it allocates. fn
// returns the bytes of cluster results it added to an LRU.
func measure(b *testing.B, n int, fn func(i int) int64) {
	times := make([]time.Duration, 0, n)
	heap := make([]uint64, 0, n)
	var m0, m1 runtime.MemStats
	for i := range n {
		runtime.ReadMemStats(&m0)
		t := time.Now()
		cached := fn(i)
		times = append(times, time.Since(t))
		runtime.ReadMemStats(&m1)
		alloc := m1.TotalAlloc - m0.TotalAlloc
		heap = append(heap, alloc-min(alloc, uint64(max(cached, 0))))
	}
	report(b, times, heap)
}

// coldCache empties h's cluster result LRU, as a fresh one would be, and
// keeps the rest of h (its idle scratch), as a long-running process does.
func coldCache(h *Hierarchy) *Hierarchy {
	h.cache = newMemberCache(DefaultMemberCacheBytes)
	return h
}

// BenchmarkOwnerNav measures the hierarchy on the owner's geodata, cold
// (an empty LRU, first touch of each cluster) and warm. Run it once:
// go test -run '^$' -bench OwnerNav -benchtime 1x ./internal/nav/
func BenchmarkOwnerNav(b *testing.B) {
	e, g, w := ownerNav(b)
	const samples = 200
	from, to := ownerSample(b, e, g, w, samples, 2500) // 40000 units
	n := len(from)
	b.Logf("graph: %d clusters, %d nodes, %d edges, %d SCCs; %d pairs", g.Clusters(), g.Nodes(), g.Edges(), g.SCCs(), n)
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 400000, Water: w}
	cold := NewHierarchy(g, e, w, DefaultMemberCacheBytes)
	for b.Loop() {
		b.Run("LocateCold", func(b *testing.B) {
			measure(b, n, func(i int) int64 {
				h := coldCache(cold)
				if _, err := h.Locate(from[i], nil); err != nil {
					b.Fatal(err)
				}
				return h.cache.used
			})
		})
		// Warm: an LRU holding every sampled cluster.
		h := NewHierarchy(g, e, w, 4<<30)
		for i := range n {
			if _, err := h.Locate(from[i], nil); err != nil {
				b.Fatal(err)
			}
			if _, err := h.Locate(to[i], nil); err != nil {
				b.Fatal(err)
			}
		}
		b.Logf("LRU: %d cluster results, %.1f MB, %.0f KB each on average",
			len(h.cache.items), float64(h.cache.used)/1e6, float64(h.cache.used)/1e3/float64(max(len(h.cache.items), 1)))
		logLabelLayers(b, h)
		// A static place located again once the LRU dropped its cluster
		// (#496): LocateCold is what Locate pays for it, this what
		// LocatePlace pays.
		places := NewHierarchy(g, e, w, 0)
		for i := range n {
			if _, err := places.LocatePlace(from[i]); err != nil {
				b.Fatal(err)
			}
		}
		b.Run("LocatePlaceAgain", func(b *testing.B) {
			computed := places.Stats().Computed
			measure(b, n, func(i int) int64 {
				if _, err := places.LocatePlace(from[i]); err != nil {
					b.Fatal(err)
				}
				return 0
			})
			s := places.Stats()
			b.Logf("place table: %d cells, %d computed again of %d places located again", s.PlaceCells, s.Computed-computed, n)
		})
		b.Run("LocateSameSCCWarm", func(b *testing.B) {
			measure(b, n, func(i int) int64 {
				ma, _ := h.Locate(from[i], nil)
				mb, _ := h.Locate(to[i], nil)
				if !h.SameSCC(ma, mb) {
					b.Fatal("sampled pair not SameSCC")
				}
				return 0
			})
		})
		// Warm means the pages resident too: one pass reads the abstract
		// graph the distances cross.
		for i := range n {
			ma, _ := h.Locate(from[i], nil)
			mb, _ := h.Locate(to[i], nil)
			if _, err := h.Distance(ma, mb); err != nil {
				b.Fatal(err)
			}
		}
		b.Run("DistanceWarm", func(b *testing.B) {
			measure(b, n, func(i int) int64 {
				ma, _ := h.Locate(from[i], nil)
				mb, _ := h.Locate(to[i], nil)
				if _, err := h.Distance(ma, mb); err != nil {
					b.Fatal(err)
				}
				return 0
			})
		})
		failed := 0
		var segments []float64
		route := func(h *Hierarchy, i int) int64 {
			used := h.cache.used
			r, err := h.Route(p, from[i].World(), to[i].World(), nil)
			if err != nil {
				failed++
			}
			for k := 1; k < len(r); k++ {
				segments = append(segments, hypot(r[k].X-r[k-1].X, r[k].Y-r[k-1].Y))
			}
			return h.cache.used - used
		}
		b.Run("RouteCold", func(b *testing.B) {
			measure(b, n, func(i int) int64 { return route(coldCache(cold), i) })
		})
		segments = segments[:0]
		b.Run("RouteWarm", func(b *testing.B) {
			measure(b, n, func(i int) int64 { return route(h, i) })
		})
		reportSegments(b, segments)
		b.Logf("routes failed: %d of %d (both passes)", failed, 2*n)
	}
}

// reportSegments logs the length of the warm routes' segments: the median,
// the quartile and the share under 300 units (#307).
func reportSegments(b *testing.B, lengths []float64) {
	if len(lengths) == 0 {
		return
	}
	slices.Sort(lengths)
	at := func(f float64) float64 { return lengths[int(f*float64(len(lengths)-1))] }
	short := 0
	for _, l := range lengths {
		if l < 300 {
			short++
		}
	}
	b.Logf("segments: %d, length p25 %.0f, median %.0f, p75 %.0f; under 300: %.1f%%",
		len(lengths), at(0.25), at(0.5), at(0.75), 100*float64(short)/float64(len(lengths)))
}
