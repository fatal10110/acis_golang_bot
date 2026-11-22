package nav

import (
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/door"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// The door scene: in the 64-cell cluster at region-local cluster (28, 28),
// cells 1792..1855 each way, a north-south wall at column 1824 with a gap
// at rows 1822..1825, closed by door 7, and a second gap at rows
// 1852..1855, far to the south.
const doorID = 7

func doorScene(tb testing.TB) (*engine.Engine, []*door.Template) {
	tb.Helper()
	t := &geotest.Terrain{}
	t.Raise(1824, 1792, 1824, 1821, geotest.Wall).Raise(1824, 1826, 1824, 1851, geotest.Wall)
	e, err := buildEngine(t)
	if err != nil {
		tb.Fatal(err)
	}
	return e, []*door.Template{doorAt(doorID, 1824, 1822, 1825)}
}

// TestClosedDoorFallsBackToDoorFreeCost checks the two costs of an
// entrance-to-entrance edge whose cheapest path crosses a door: with the
// door closed the edge costs its door-free way, never unreachable.
func TestClosedDoorFallsBackToDoorFreeCost(t *testing.T) {
	e, doors := doorScene(t)
	th := buildHier(t, e, 64, doors, window(64, 27, 28, 29, 28))
	g := th.g
	ci := g.cluster(geoCell(1800, 1800, 0).X, geoCell(1800, 1800, 0).Y)
	first, end := g.clusterNodes(ci)
	west, east := -1, -1
	for n := first; n < end; n++ {
		switch g.node(n).side {
		case sideWest:
			west = n
		case sideEast:
			east = n
		}
	}
	if west < 0 || east < 0 {
		t.Fatalf("door cluster nodes %d..%d: no west and east entrance", first, end)
	}
	var e2 edgeRec
	found := false
	lo, hi := g.nodeEdges(west)
	for i := lo; i < hi; i++ {
		if r := g.edge(i); r.to == east {
			e2, found = r, true
		}
	}
	if !found {
		t.Fatal("no edge west to east")
	}
	if e2.nDoors != 1 || g.door(e2.firstDoor) != doorID {
		t.Fatalf("edge doors %d (first %d), want door %d", e2.nDoors, g.door(e2.firstDoor), doorID)
	}
	if e2.free == noCost || e2.free <= e2.open {
		t.Fatalf("door-free cost %d, all-open cost %d: want a dearer door-free way", e2.free, e2.open)
	}
	closed := func(d int) bool { return d == doorID }
	if c, ok := g.edgeCost(e2, nil); !ok || c != e2.open {
		t.Errorf("open door: cost %d %v, want %d", c, ok, e2.open)
	}
	if c, ok := g.edgeCost(e2, closed); !ok || c != e2.free {
		t.Errorf("closed door: cost %d %v, want the door-free %d", c, ok, e2.free)
	}

	a, err := th.h.Locate(geoCell(1760, 1823, 0), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := th.h.Locate(geoCell(1890, 1823, 0), nil)
	if err != nil {
		t.Fatal(err)
	}
	dOpen, err := th.h.distance(a, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	dClosed, err := th.h.distance(a, b, closed)
	if err != nil {
		t.Fatalf("distance with the door closed: %v, want the door-free way", err)
	}
	if dClosed <= dOpen {
		t.Errorf("distance %d with the door closed, %d open: want dearer", dClosed, dOpen)
	}
}

// TestRouteSucceedsExactlyWhenSameSCC routes between random cells of the
// door-free scene windows: a route is found exactly when the two cells are
// SameSCC, and every route passes CanMove segment by segment.
func TestRouteSucceedsExactlyWhenSameSCC(t *testing.T) {
	th := hierScenes32(t)
	p := testPlanner(t)
	p.Budget = 400000
	// The scene windows without the stair, whose exact stretches N2's
	// tests cover on their own.
	keys := append(window(32, 24, 8, 49, 9), window(32, 8, 8, 17, 9)...)
	cells := clusterCells(testEngine(t), 32, keys)
	rng := rand.New(rand.NewPCG(5, 6))
	pairs := [][2]Cell{
		{pointCell(geotest.UInside), pointCell(geotest.UBehind)},
		{pointCell(geotest.LedgeHigh), pointCell(geotest.LedgeLow)},
		{pointCell(geotest.CorridorWest), pointCell(geotest.CorridorEast)},
		{pointCell(geotest.PocketOutside), pointCell(geotest.PocketInside)},
	}
	for len(pairs) < 60 {
		pairs = append(pairs, [2]Cell{cells[rng.IntN(len(cells))], cells[rng.IntN(len(cells))]})
	}
	same := 0
	for _, pr := range pairs {
		from, to := pr[0].World(), pr[1].World()
		var buf [block.MaxLayers]block.Cell
		if !p.clickable(pr[1], &buf) {
			continue
		}
		ma, err := th.h.Locate(pr[0], nil)
		if err != nil {
			t.Fatal(err)
		}
		mb, err := th.h.Locate(pr[1], nil)
		if err != nil {
			t.Fatal(err)
		}
		route, err := th.h.Route(p, from, to, nil)
		if want := th.h.SameSCC(ma, mb); (err == nil) != want {
			t.Errorf("%v -> %v: route error %v, SameSCC %v", pr[0], pr[1], err, want)
			continue
		}
		if err != nil {
			if !errors.Is(err, ErrUnreachable) {
				t.Errorf("%v -> %v: %v, want ErrUnreachable", pr[0], pr[1], err)
			}
			continue
		}
		same++
		for i := 1; i < len(route); i++ {
			a, b := route[i-1], route[i]
			if !p.Engine.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) {
				t.Errorf("%v -> %v: segment %d fails CanMove", pr[0], pr[1], i)
			}
		}
	}
	if same < 10 {
		t.Errorf("only %d routable pairs: the sample says little", same)
	}
}

// TestRouteNeverEntersThePit checks that a default route passes the pit by
// and that the pit floor, the ledge below a plateau and the bridge deck are
// refused as one-way destinations.
func TestRouteNeverEntersThePit(t *testing.T) {
	th := hierScenes32(t)
	p := testPlanner(t)
	beyond := geotest.Point{CellX: 1065, CellY: 287, Z: geotest.Ground}
	route, err := th.h.Route(p, loc(geotest.PitOutside), loc(beyond), nil)
	if err != nil {
		t.Fatal(err)
	}
	ox, oy := regionOrigin()
	for i := 1; i < len(route); i++ {
		a, b := route[i-1], route[i]
		for s := 0; s <= 32; s++ {
			x := engine.GeoX(a.X+(b.X-a.X)*s/32) - ox
			y := engine.GeoY(a.Y+(b.Y-a.Y)*s/32) - oy
			if x >= 1040 && x <= 1055 && y >= 280 && y <= 295 {
				t.Fatalf("segment %d enters the pit at %d,%d", i, x, y)
			}
		}
	}
	for _, tc := range []struct {
		name     string
		from, to location.Location
	}{
		{"into the pit", loc(geotest.PitOutside), loc(geotest.PitInside)},
		{"out of the pit", loc(geotest.PitInside), loc(geotest.PitOutside)},
		{"off the ledge", loc(geotest.LedgeHigh), loc(geotest.LedgeLow)},
	} {
		if _, err := th.h.Route(p, tc.from, tc.to, nil); !errors.Is(err, ErrUnreachable) {
			t.Errorf("%s: %v, want ErrUnreachable", tc.name, err)
		}
	}
}

// TestNavCacheRoundTrip writes a built image, opens it back through the
// mapping, and refuses a stale key, a corrupt byte and a missing file.
func TestNavCacheRoundTrip(t *testing.T) {
	th := buildHier(t, onewayEngine(t), 64, nil, onewayClusters())
	key := NavKey{GeoCRC: 0x1234, DoorsHash: [8]byte{1, 2, 3}, ClusterCells: 64}
	image, _, err := Build(context.Background(), BuildConfig{Engine: onewayEngine(t), Clusters: onewayClusters(), ClusterCells: 64, Key: key})
	if err != nil {
		t.Fatal(err)
	}
	if len(image) != len(th.image) {
		t.Fatalf("a rebuild gave %d bytes, first build %d", len(image), len(th.image))
	}
	path := filepath.Join(t.TempDir(), "nav.cache")
	if err := WriteNavCache(path, image); err != nil {
		t.Fatal(err)
	}
	g, err := OpenNavCache(path, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(g.b) != string(image) || g.Nodes() != th.g.Nodes() || g.Edges() != th.g.Edges() {
		t.Error("the mapped graph differs from the written image")
	}
	h := NewHierarchy(g, onewayEngine(t), DefaultMemberCacheBytes)
	if _, err := h.Locate(geoCell(1700, 1700, 0), nil); err != nil {
		t.Errorf("Locate over the mapped cache: %v", err)
	}
	g.Close()

	for _, tc := range []struct {
		name string
		key  NavKey
	}{
		{"geodata cache changed", NavKey{GeoCRC: 0x1235, DoorsHash: key.DoorsHash, ClusterCells: 64}},
		{"doors.xml changed", NavKey{GeoCRC: key.GeoCRC, DoorsHash: [8]byte{9}, ClusterCells: 64}},
		{"build parameters changed", NavKey{GeoCRC: key.GeoCRC, DoorsHash: key.DoorsHash, ClusterCells: 256}},
	} {
		if _, err := OpenNavCache(path, tc.key); !errors.Is(err, ErrNavCacheStale) {
			t.Errorf("%s: %v, want ErrNavCacheStale", tc.name, err)
		}
	}
	corrupt := append([]byte(nil), image...)
	corrupt[len(corrupt)-1] ^= 0xff
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenNavCache(path, key); !errors.Is(err, ErrNavCacheStale) {
		t.Errorf("corrupt: %v, want ErrNavCacheStale", err)
	}
	if _, err := OpenNavCache(path+".missing", key); !errors.Is(err, ErrNavCacheMissing) {
		t.Errorf("missing: %v, want ErrNavCacheMissing", err)
	}
}

// TestBuildJoinsWorkersOnCancel cancels a build after its first cluster
// and checks Build reports the cancellation only after every worker has
// stopped.
func TestBuildJoinsWorkersOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var returned atomic.Bool
	var calls atomic.Int32
	cfg := BuildConfig{Engine: testEngine(t), Clusters: sceneWindows32(), ClusterCells: 32, Workers: 2}
	cfg.onCluster = func() {
		if returned.Load() {
			t.Error("a worker ran after Build returned")
		}
		if calls.Add(1) == 1 {
			cancel()
		}
	}
	_, _, err := Build(ctx, cfg)
	returned.Store(true)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Build after cancel: %v, want context.Canceled", err)
	}
	if n := calls.Load(); n > 2 {
		t.Errorf("%d clusters built after the cancel, want at most one per worker", n)
	}
}
