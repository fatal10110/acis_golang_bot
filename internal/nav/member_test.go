package nav

import (
	"log/slog"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/door"
	"github.com/fatal10110/acis_golang_bot/internal/geo/dynamic"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// randomTerrain is plain ground with random raised, sunk and walled patches
// and decks in the region's first 192×192 cells: ledges both ways, pits,
// wall tops a walker can only drop off, and multilayer cells.
func randomTerrain(seed uint64) *geotest.Terrain {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b9))
	heights := []int16{geotest.Wall, geotest.Plateau, 32, -32, 64, geotest.PitDown, 96}
	t := &geotest.Terrain{}
	for range 40 {
		x, y := rng.IntN(180), rng.IntN(180)
		t.Raise(x, y, x+1+rng.IntN(14), y+1+rng.IntN(14), heights[rng.IntN(len(heights))])
	}
	for range 4 {
		x, y := rng.IntN(170), rng.IntN(170)
		t.AddDeck(x, y, x+4+rng.IntN(16), y+1+rng.IntN(4), geotest.Deck)
	}
	return t
}

var (
	randomOnce   sync.Once
	randomEngine *engine.Engine
	randomErr    error
)

func randomTerrainEngine(tb testing.TB) *engine.Engine {
	tb.Helper()
	randomOnce.Do(func() { randomEngine, randomErr = buildEngine(randomTerrain(7)) })
	if randomErr != nil {
		tb.Fatal(randomErr)
	}
	return randomEngine
}

// TestMemberMatchesBruteForceSCC checks the cluster membership rule against
// a whole-window SCC computed by brute force: two cells share a Member key
// exactly when they are mutually reachable, a cell is InSCC exactly when its
// brute SCC holds an entrance, one-way out exactly when it reaches such an
// SCC otherwise, and Reachable agrees with a search over the window.
func TestMemberMatchesBruteForceSCC(t *testing.T) {
	cases := []struct {
		name string
		th   func(testing.TB) (*testHier, *engine.Engine)
	}{
		{"scenes/32", func(tb testing.TB) (*testHier, *engine.Engine) { return hierScenes32(tb), testEngine(tb) }},
		{"scenes/256", func(tb testing.TB) (*testHier, *engine.Engine) { return hierScenes256(tb), testEngine(tb) }},
		{"random/32", func(tb testing.TB) (*testHier, *engine.Engine) {
			e := randomTerrainEngine(tb)
			return buildHier(tb, e, 32, nil, window(32, 0, 0, 5, 5)), e
		}},
		{"oneway/64", func(tb testing.TB) (*testHier, *engine.Engine) {
			e := onewayEngine(tb)
			return buildHier(tb, e, 64, nil, onewayClusters()), e
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			th, e := tc.th(t)
			b := bruteForce(e, th.g.Key().ClusterCells, th.cfg.Clusters)
			toKey := map[int32]memberKey{}
			toBrute := map[memberKey]int32{}
			members := make([]Member, len(b.cells))
			kinds := map[Kind]int{}
			for i, c := range b.cells {
				m, err := th.h.Locate(c, nil)
				if err != nil {
					t.Fatalf("Locate %v: %v", c, err)
				}
				members[i] = m
				k, l := keyOf(m), b.label[i]
				if old, ok := toKey[l]; ok && old != k {
					t.Fatalf("cell %v: brute SCC %d split over members %+v and %+v", c, l, old, k)
				}
				if old, ok := toBrute[k]; ok && old != l {
					t.Fatalf("cell %v: member %+v joins brute SCCs %d and %d", c, k, old, l)
				}
				toKey[l], toBrute[k] = k, l
				want := Isolated
				switch {
				case b.inSCC[l]:
					want = InSCC
				case b.reaches[l]:
					want = OneWayOut
				}
				if m.Kind() != want {
					t.Fatalf("cell %v: kind %v, brute force says %v", c, m.Kind(), want)
				}
				kinds[want]++
			}
			t.Logf("%d cells, %d SCCs, kinds %v, nodes %d, edges %d", len(b.cells), len(toKey), kinds, th.g.Nodes(), th.g.Edges())

			rng := rand.New(rand.NewPCG(1, 2))
			for range 40 {
				i, j := int32(rng.IntN(len(b.cells))), int32(rng.IntN(len(b.cells)))
				if members[j].Kind() != InSCC {
					continue // Reachable sees a drop between two non-SCC cells of one cluster only through an SCC
				}
				if got, want := th.h.Reachable(members[i], members[j]), b.reach(i, j); got != want {
					t.Errorf("Reachable(%v, %v) = %v, brute force %v", b.cells[i], b.cells[j], got, want)
				}
			}
		})
	}
}

// TestMemberKindsAtDefaultClusters pins the scene kinds at the default
// cluster side, where each scene lies inside one cluster.
func TestMemberKindsAtDefaultClusters(t *testing.T) {
	th := hierScenes256(t)
	for _, tc := range []struct {
		name string
		at   geotest.Point
		want Kind
	}{
		{"pit floor", geotest.PitInside, Isolated},
		{"pocket", geotest.PocketInside, Isolated},
		{"bridge deck", geotest.BridgeWestDeck, OneWayOut},
		{"ground by the pit", geotest.PitOutside, InSCC},
		{"ground under the bridge", geotest.BridgeWestGround, InSCC},
		{"ground by the pocket", geotest.PocketOutside, InSCC},
	} {
		m, err := th.h.Locate(pointCell(tc.at), nil)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if m.Kind() != tc.want {
			t.Errorf("%s: kind %v, want %v", tc.name, m.Kind(), tc.want)
		}
	}
	out, _ := th.h.Locate(pointCell(geotest.PitOutside), nil)
	in, _ := th.h.Locate(pointCell(geotest.PitInside), nil)
	deck, _ := th.h.Locate(pointCell(geotest.BridgeWestDeck), nil)
	if th.h.SameSCC(out, in) || !th.h.Reachable(out, in) || th.h.Reachable(in, out) {
		t.Error("pit: want reachable one way only, from outside")
	}
	if !th.h.Reachable(deck, out) || th.h.Reachable(out, deck) {
		t.Error("bridge deck: want a one-way drop to the ground only")
	}
}

// The one-way border: clusters A and B of 64 cells side by side; on A's side
// of their border, rows 0..26 are a plateau whose cells drop into B, while
// rows 27..63 are ground crossing both ways.
func onewayClusters() []ClusterKey { return window(64, 26, 26, 27, 26) }

var (
	onewayOnce sync.Once
	onewayEng  *engine.Engine
	onewayErr  error
)

func onewayEngine(tb testing.TB) *engine.Engine {
	tb.Helper()
	onewayOnce.Do(func() {
		t := &geotest.Terrain{}
		t.Raise(1700, 1664, 1727, 1690, geotest.Plateau)
		onewayEng, onewayErr = buildEngine(t)
	})
	if onewayErr != nil {
		tb.Fatal(onewayErr)
	}
	return onewayEng
}

// TestEntrancesKeepOneWayTerrainApart checks that no entrance node merges
// cells that are not mutually reachable inside their cluster, and that the
// one-way plateau run keeps its own node apart from the two-way ground run.
func TestEntrancesKeepOneWayTerrainApart(t *testing.T) {
	for _, tc := range []struct {
		name string
		e    func(testing.TB) *engine.Engine
		size int
		keys []ClusterKey
	}{
		{"oneway/64", onewayEngine, 64, onewayClusters()},
		{"scenes/32", testEngine, 32, sceneWindows32()},
		{"random/32", randomTerrainEngine, 32, window(32, 0, 0, 5, 5)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := tc.e(t)
			th := buildHier(t, e, tc.size, nil, tc.keys)
			b := th.builder()
			outs, err := b.clusters(t.Context(), 1, nil)
			if err != nil {
				t.Fatal(err)
			}
			var g localGraph
			var s sccScratch
			for ci, o := range outs {
				k := b.keys[ci]
				g.load(&b.p, int(k.X)*tc.size, int(k.Y)*tc.size, tc.size)
				s.localSCCs(&g)
				for _, bc := range o.border {
					lx, ly, _, _ := b.sidePos(&g, bc.side, bc.pos)
					cell := g.index(Cell{X: g.x0 + lx, Y: g.y0 + ly, Z: bc.z})
					rep := g.index(o.nodes[bc.node].rep)
					if cell < 0 || rep < 0 || s.label[cell] != s.label[rep] {
						t.Fatalf("cluster %v: node %d merges cell side %d pos %d z %d not mutually reachable with its rep", k, bc.node, bc.side, bc.pos, bc.z)
					}
				}
			}
			if tc.name != "oneway/64" {
				return
			}
			// A is cluster 0, B cluster 1 (sorted by row, then column).
			nodeAt := func(o clusterOut, side, pos int, z int16) int32 {
				n, ok := o.nodeAt(side, pos, z)
				if !ok {
					t.Fatalf("no node at side %d pos %d z %d", side, pos, z)
				}
				return n
			}
			plateau := nodeAt(outs[0], sideEast, 10, geotest.Plateau)
			ground := nodeAt(outs[0], sideEast, 40, geotest.Ground)
			if plateau == ground {
				t.Fatal("A merged the one-way plateau run with the ground run")
			}
			if n := outs[0].nodes[plateau]; n.pos0 != 0 || n.pos1 != 26 {
				t.Errorf("plateau node spans %d..%d, want 0..26", n.pos0, n.pos1)
			}
			if bw1, bw2 := nodeAt(outs[1], sideWest, 10, geotest.Ground), nodeAt(outs[1], sideWest, 40, geotest.Ground); bw1 != bw2 {
				t.Error("B split its mutually reachable ground run")
			}
		})
	}
}

// builder returns a builder over the same clusters as the hierarchy's
// build, for tests that inspect clusters before assembly.
func (th *testHier) builder() *builder {
	size := th.g.Key().ClusterCells
	static := th.cfg.Engine.Static()
	b := &builder{size: size, keys: th.cfg.Clusters, p: Planner{Engine: static}, built: map[ClusterKey]bool{}}
	b.keys = slices.Clone(b.keys)
	slices.SortFunc(b.keys, cmpKey)
	b.keys = slices.Compact(b.keys)
	for _, k := range b.keys {
		b.built[k] = true
	}
	b.loadDoors(th.cfg.Doors, static, slog.New(slog.DiscardHandler))
	return b
}

// TestLocateIgnoresDoorState checks that cluster labels computed while a
// door is closed equal those computed with it open: Locate reads the static
// base blocks only.
func TestLocateIgnoresDoorState(t *testing.T) {
	e, err := buildEngine(terrain())
	if err != nil {
		t.Fatal(err)
	}
	keys := window(64, 3, 3, 5, 5)
	open := buildHier(t, e, 64, []*door.Template{geotest.CorridorDoor()}, keys)
	cells := clusterCells(e, 64, keys)
	want := make([]memberKey, len(cells))
	for i, c := range cells {
		m, err := open.h.Locate(c, nil)
		if err != nil {
			t.Fatal(err)
		}
		want[i] = keyOf(m)
	}
	obj, err := dynamic.NewDoorObject(geotest.CorridorDoor(), e)
	if err != nil {
		t.Fatal(err)
	}
	e.AddObject(obj)
	defer e.RemoveObject(obj)
	door := pointCell(geotest.Point{CellX: 288, CellY: 261, Z: geotest.Ground})
	if n := e.NSWENearest(door.X, door.Y, 0); n != 0 {
		t.Fatalf("door cell NSWE %v with the door closed, want none", n)
	}
	closed := NewHierarchy(open.g, e, DefaultMemberCacheBytes)
	for i, c := range cells {
		m, err := closed.Locate(c, nil)
		if err != nil {
			t.Fatal(err)
		}
		if keyOf(m) != want[i] {
			t.Fatalf("cell %v: label %+v with the door closed, %+v open", c, keyOf(m), want[i])
		}
	}
}

// TestLocatePinsAndConcurrency locates cells from many goroutines through a
// tiny LRU and checks every answer against an unbounded hierarchy, then
// checks that a pinned result is used though the LRU holds nothing.
func TestLocatePinsAndConcurrency(t *testing.T) {
	th := hierScenes32(t)
	cells := clusterCells(testEngine(t), 32, sceneWindows32())
	want := make([]memberKey, len(cells))
	for i, c := range cells {
		m, err := th.h.Locate(c, nil)
		if err != nil {
			t.Fatal(err)
		}
		want[i] = keyOf(m)
	}
	small := NewHierarchy(th.g, testEngine(t), 64<<10)
	var wg sync.WaitGroup
	for w := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(w), 3))
			for range 300 {
				i := rng.IntN(len(cells))
				m, err := small.Locate(cells[i], nil)
				if err != nil {
					t.Error(err)
					return
				}
				if keyOf(m) != want[i] {
					t.Errorf("cell %v: %+v, want %+v", cells[i], keyOf(m), want[i])
					return
				}
			}
		}()
	}
	wg.Wait()
	if small.cache.used > small.cache.max {
		t.Errorf("LRU holds %d bytes over its %d bound", small.cache.used, small.cache.max)
	}

	none := NewHierarchy(th.g, testEngine(t), 0)
	m, err := none.Locate(pointCell(geotest.PitInside), nil)
	if err != nil {
		t.Fatal(err)
	}
	pins := &Pins{Current: m}
	again, err := none.Locate(pointCell(geotest.PitInside), pins)
	if err != nil {
		t.Fatal(err)
	}
	if again.res != m.res {
		t.Error("Locate recomputed a pinned cluster")
	}
	if fresh, _ := none.Locate(pointCell(geotest.PitInside), nil); fresh.res == m.res {
		t.Error("an LRU bounded at 0 bytes kept a result")
	}
}

// TestMemberCacheEvictsByBytes checks the LRU's byte bound, its recency
// order and that a second insert of a cluster is dropped.
func TestMemberCacheEvictsByBytes(t *testing.T) {
	res := func(cluster int32, bytes int64) *clusterResult {
		return &clusterResult{cluster: cluster, bytes: bytes}
	}
	c := newMemberCache(100)
	c.put(res(1, 40))
	c.put(res(2, 40))
	if c.get(1) == nil { // 1 is now the most recent
		t.Fatal("cluster 1 missing")
	}
	c.put(res(3, 40)) // evicts 2, the least recent
	if c.get(2) != nil || c.get(1) == nil || c.get(3) == nil {
		t.Errorf("after the third insert: want 1 and 3 kept, 2 evicted")
	}
	dup := res(3, 10)
	c.put(dup)
	if c.get(3) == dup || c.used != 80 {
		t.Errorf("a second insert replaced the first or changed the size: used %d", c.used)
	}
	c.put(res(4, 101))
	if c.get(4) != nil || c.used != 80 {
		t.Error("a result over the whole bound was kept")
	}
	c.put(res(5, 100))
	if c.get(1) != nil || c.get(3) != nil || c.get(5) == nil || c.used != 100 {
		t.Errorf("a full-bound insert: used %d", c.used)
	}
}
