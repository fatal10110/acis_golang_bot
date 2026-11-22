package nav

import (
	"math/rand/v2"
	"os"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geodata"
)

// ownerNav opens the owner's geodata cache and nav cache for the local
// performance benchmarks (docs/navigation.md, "nav performance"). The
// owner's geodata is private and never in CI, so the benchmarks skip
// unless BOT_NAV_GEODATA (region files dir), BOT_NAV_GEOENGINE,
// BOT_NAV_GEODATA_CACHE, BOT_NAV_DOORS (doors.xml) and BOT_NAV_CACHE are
// set.
func ownerNav(b *testing.B) (*engine.Engine, *Graph) {
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
	g, err := OpenNavCache(navPath, NavKey{GeoCRC: c.CRC(), DoorsHash: DoorsHash(xml), ClusterCells: DefaultClusterCells})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(g.Close)
	return c.Engine, g
}

// ownerSample picks n cells of the largest abstract SCC, each in its own
// random cluster, and for each a partner about dist cells away in the same
// SCC with a click target on it.
func ownerSample(b *testing.B, e *engine.Engine, g *Graph, n, dist int) (from, to []Cell) {
	b.Helper()
	h := NewHierarchy(g, e, nil, 1<<30)
	p := &Planner{Engine: e, CollisionHeight: 23}
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

// report records the p50 and p99 of the timings and the largest heap
// allocation of one query.
func report(b *testing.B, times []time.Duration, maxAlloc uint64) {
	slices.Sort(times)
	q := func(f float64) float64 { return float64(times[int(f*float64(len(times)-1))].Microseconds()) / 1000 }
	b.ReportMetric(q(0.5), "p50-ms")
	b.ReportMetric(q(0.99), "p99-ms")
	b.ReportMetric(float64(maxAlloc)/1e6, "max-alloc-MB")
	b.Logf("%d queries: p50 %.2f ms, p99 %.2f ms, max %.2f ms, largest allocation per query %.2f MB",
		len(times), q(0.5), q(0.99), q(1), float64(maxAlloc)/1e6)
}

// measure times fn over every sample and tracks the bytes it allocates.
func measure(b *testing.B, n int, fn func(i int)) {
	times := make([]time.Duration, 0, n)
	var maxAlloc uint64
	var m0, m1 runtime.MemStats
	for i := range n {
		runtime.ReadMemStats(&m0)
		t := time.Now()
		fn(i)
		times = append(times, time.Since(t))
		runtime.ReadMemStats(&m1)
		maxAlloc = max(maxAlloc, m1.TotalAlloc-m0.TotalAlloc)
	}
	report(b, times, maxAlloc)
}

// BenchmarkOwnerNav measures the hierarchy on the owner's geodata, cold
// (an empty LRU, first touch of each cluster) and warm. Run it once:
// go test -run '^$' -bench OwnerNav -benchtime 1x ./internal/nav/
func BenchmarkOwnerNav(b *testing.B) {
	e, g := ownerNav(b)
	const samples = 200
	from, to := ownerSample(b, e, g, samples, 2500) // 40000 units
	n := len(from)
	b.Logf("graph: %d clusters, %d nodes, %d edges, %d SCCs; %d pairs", g.Clusters(), g.Nodes(), g.Edges(), g.SCCs(), n)
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 400000}
	for b.Loop() {
		b.Run("LocateCold", func(b *testing.B) {
			measure(b, n, func(i int) {
				h := NewHierarchy(g, e, nil, DefaultMemberCacheBytes)
				if _, err := h.Locate(from[i], nil); err != nil {
					b.Fatal(err)
				}
			})
		})
		// Warm: an LRU holding every sampled cluster.
		h := NewHierarchy(g, e, nil, 4<<30)
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
		b.Run("LocateSameSCCWarm", func(b *testing.B) {
			measure(b, n, func(i int) {
				ma, _ := h.Locate(from[i], nil)
				mb, _ := h.Locate(to[i], nil)
				if !h.SameSCC(ma, mb) {
					b.Fatal("sampled pair not SameSCC")
				}
			})
		})
		b.Run("DistanceWarm", func(b *testing.B) {
			measure(b, n, func(i int) {
				ma, _ := h.Locate(from[i], nil)
				mb, _ := h.Locate(to[i], nil)
				if _, err := h.Distance(ma, mb); err != nil {
					b.Fatal(err)
				}
			})
		})
		failed := 0
		route := func(h *Hierarchy) func(i int) {
			return func(i int) {
				if _, err := h.Route(p, from[i].World(), to[i].World(), nil); err != nil {
					failed++
				}
			}
		}
		b.Run("RouteCold", func(b *testing.B) {
			measure(b, n, func(i int) { route(NewHierarchy(g, e, nil, DefaultMemberCacheBytes))(i) })
		})
		b.Run("RouteWarm", func(b *testing.B) { measure(b, n, route(h)) })
		b.Logf("routes failed: %d of %d (both passes)", failed, 2*n)
	}
}
