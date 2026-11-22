package nav

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// lake is a water box over the whole squares of region-local cells x0..x1,
// y0..y1, with the Z band lo..hi.
func lake(x0, y0, x1, y1, lo, hi int) WaterBox {
	return WaterBox{
		X1: geotest.CellWorldX(x0) - 8, Y1: geotest.CellWorldY(y0) - 8,
		X2: geotest.CellWorldX(x1) + 7, Y2: geotest.CellWorldY(y1) + 7,
		MinZ: lo, MaxZ: hi,
	}
}

// The water scene: open ground around Open, a lake across the straight line
// from WaterWest to WaterEast.
var (
	WaterWest   = geotest.Point{CellX: 100, CellY: 100, Z: geotest.Ground}
	WaterEast   = geotest.Point{CellX: 160, CellY: 100, Z: geotest.Ground}
	WaterInLake = geotest.Point{CellX: 130, CellY: 100, Z: geotest.Ground}
	waterLake   = lake(125, 70, 135, 130, -20, 20)
)

// TestWaterContains checks the server's containment rule for a point:
// bounds inclusive on every axis, corners and Z band normalized.
func TestWaterContains(t *testing.T) {
	box := WaterBox{X1: 100, Y1: 200, X2: 300, Y2: 400, MinZ: -50, MaxZ: 50}
	swapped := WaterBox{X1: 300, Y1: 400, X2: 100, Y2: 200, MinZ: 50, MaxZ: -50}
	tests := []struct {
		name    string
		x, y, z int
		want    bool
	}{
		{"inside", 200, 300, 0, true},
		{"low corner, low Z", 100, 200, -50, true},
		{"high corner, high Z", 300, 400, 50, true},
		{"west of the box", 99, 300, 0, false},
		{"south of the box", 200, 401, 0, false},
		{"above the band", 200, 300, 51, false},
		{"below the band", 200, 300, -51, false},
	}
	for _, w := range []*Water{NewWater([]WaterBox{box}), NewWater([]WaterBox{swapped})} {
		for _, tt := range tests {
			if got := w.Contains(tt.x, tt.y, tt.z); got != tt.want {
				t.Errorf("%s: Contains(%d, %d, %d) = %v, want %v", tt.name, tt.x, tt.y, tt.z, got, tt.want)
			}
		}
	}
	var none *Water
	if none.Contains(200, 300, 0) {
		t.Error("nil water contains a point")
	}
}

// TestWaterWet checks the cell rule: a cell is wet when any point of its
// square lies in a box at the cell's floor height, also across a region
// tile border.
func TestWaterWet(t *testing.T) {
	half := WaterBox{ // from cell 10's centre to cell 20's: both overlap, 9 and 21 do not
		X1: geotest.CellWorldX(10), Y1: geotest.CellWorldY(10),
		X2: geotest.CellWorldX(20), Y2: geotest.CellWorldY(20),
		MinZ: -10, MaxZ: 10,
	}
	w := NewWater([]WaterBox{half, lake(2040, 10, 2050, 20, -10, 10)})
	tests := []struct {
		name string
		x, y int
		z    int16
		want bool
	}{
		{"inside", 15, 15, 0, true},
		{"partly over the east edge", 20, 15, 0, true},
		{"partly over the west edge", 10, 15, 0, true},
		{"east of the box", 21, 15, 0, false},
		{"west of the box", 9, 15, 0, false},
		{"floor above the band", 15, 15, 11, false},
		{"floor below the band", 15, 15, -11, false},
		{"last cell of the region", 2047, 15, 0, true},
		{"first cell of the next region", 2048, 15, 0, true},
	}
	for _, tt := range tests {
		c := geoCell(tt.x, tt.y, tt.z)
		if got := w.wet(c.X, c.Y, c.Z); got != tt.want {
			t.Errorf("%s: wet(%d, %d, %d) = %v, want %v", tt.name, tt.x, tt.y, tt.z, got, tt.want)
		}
	}
	var none *Water
	if c := geoCell(15, 15, 0); none.wet(c.X, c.Y, c.Z) {
		t.Error("nil water wets a cell")
	}
}

// TestWaterHash checks the nav cache key's water half: zero for no water,
// stable for the same boxes, different for other boxes.
func TestWaterHash(t *testing.T) {
	var none *Water
	if none.Hash() != [8]byte{} || NewWater(nil).Hash() != [8]byte{} {
		t.Error("no water hashes to non-zero")
	}
	a, b := NewWater([]WaterBox{waterLake}), NewWater([]WaterBox{waterLake})
	if a.Hash() != b.Hash() || a.Hash() == [8]byte{} {
		t.Errorf("same boxes hash to %x and %x", a.Hash(), b.Hash())
	}
	moved := waterLake
	moved.MaxZ++
	if NewWater([]WaterBox{moved}).Hash() == a.Hash() {
		t.Error("a changed box keeps the hash")
	}
}

// assertDry fails when any point a ground walker passes on route, sampled
// every world unit with the floor the step chain carries, lies in w.
func assertDry(t *testing.T, p *Planner, w *Water, route []location.Location) {
	t.Helper()
	for i := 1; i < len(route); i++ {
		a, b := route[i-1], route[i]
		n := max(abs(b.X-a.X), abs(b.Y-a.Y), 1)
		z := a.Z
		for s := 0; s <= n; s++ {
			x, y := a.X+(b.X-a.X)*s/n, a.Y+(b.Y-a.Y)*s/n
			z = int(p.Engine.Height(x, y, z))
			if w.Contains(x, y, z) {
				t.Fatalf("segment %d (%v to %v) passes water at %d,%d,%d", i, a, b, x, y, z)
			}
		}
	}
}

// TestRouteKeepsOutOfWater checks that the cell planner goes around a lake
// it would otherwise cross, refuses a wet endpoint, ignores water above the
// floor, and that the segment checks fail across water.
func TestRouteKeepsOutOfWater(t *testing.T) {
	w := NewWater([]WaterBox{waterLake})
	dry := testPlanner(t)
	wet := testPlanner(t)
	wet.Water = w

	straight, err := dry.Route(loc(WaterWest), loc(WaterEast))
	if err != nil || len(straight) != 2 {
		t.Fatalf("without water: %v, %v; want one straight segment", straight, err)
	}
	route, err := wet.Route(loc(WaterWest), loc(WaterEast))
	if err != nil {
		t.Fatalf("with water: %v", err)
	}
	if len(route) < 3 {
		t.Fatalf("with water the route %v does not go around the lake", route)
	}
	assertDry(t, wet, w, route)

	if dry.StepChainOK(loc(WaterWest), loc(WaterEast)) != true || dry.CorridorOpen(loc(WaterWest), loc(WaterEast)) != true {
		t.Error("without water the straight segment fails the segment checks")
	}
	if wet.StepChainOK(loc(WaterWest), loc(WaterEast)) || wet.CorridorOpen(loc(WaterWest), loc(WaterEast)) {
		t.Error("the straight segment across the lake passes the segment checks")
	}

	for _, tc := range []struct {
		name     string
		from, to location.Location
	}{
		{"into the lake", loc(WaterWest), loc(WaterInLake)},
		{"out of the lake", loc(WaterInLake), loc(WaterEast)},
	} {
		if _, err := wet.Route(tc.from, tc.to); !errors.Is(err, ErrWater) {
			t.Errorf("%s: %v, want ErrWater", tc.name, err)
		}
	}

	high := testPlanner(t)
	high.Water = NewWater([]WaterBox{lake(125, 70, 135, 130, 1, 100)})
	if r, err := high.Route(loc(WaterWest), loc(WaterEast)); err != nil || len(r) != 2 {
		t.Errorf("water above the floor: %v, %v; want one straight segment", r, err)
	}
}

// TestNearestWalkableSkipsWater checks that a target in the lake snaps to a
// dry cell.
func TestNearestWalkableSkipsWater(t *testing.T) {
	p := testPlanner(t)
	p.Water = NewWater([]WaterBox{waterLake})
	c, ok := p.NearestWalkable(loc(WaterInLake), 200)
	if !ok {
		t.Fatal("no dry cell within 200")
	}
	if p.Water.wet(c.X, c.Y, c.Z) {
		t.Errorf("NearestWalkable = %+v, a wet cell", c)
	}
	if _, ok := p.NearestWalkable(loc(WaterInLake), 32); ok {
		t.Error("NearestWalkable found a cell within 32 of the lake's middle")
	}
}

// TestHierarchyRouteKeepsOutOfWater builds the hierarchy with the lake: the
// abstract graph has no edge into it, a route goes around it, a wet endpoint
// is ErrWater, and the nav cache is keyed by the water.
func TestHierarchyRouteKeepsOutOfWater(t *testing.T) {
	e := testEngine(t)
	w := NewWater([]WaterBox{waterLake})
	cfg := BuildConfig{Engine: e, Water: w, Clusters: window(32, 2, 2, 6, 6), ClusterCells: 32, Workers: 2}
	image, _, err := Build(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	g, err := parseGraph(image)
	if err != nil {
		t.Fatal(err)
	}
	if g.Key().WaterHash != w.Hash() {
		t.Fatalf("key water hash %x, want %x", g.Key().WaterHash, w.Hash())
	}
	h := NewHierarchy(g, e, w, DefaultMemberCacheBytes)
	p := testPlanner(t)
	p.Water = w

	route, err := h.Route(p, loc(WaterWest), loc(WaterEast), nil)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	assertDry(t, p, w, route)
	if _, err := h.Route(p, loc(WaterWest), loc(WaterInLake), nil); !errors.Is(err, ErrWater) {
		t.Errorf("into the lake: %v, want ErrWater", err)
	}

	path := filepath.Join(t.TempDir(), "nav.cache")
	if err := WriteNavCache(path, image); err != nil {
		t.Fatal(err)
	}
	opened, err := OpenNavCache(path, NavKey{ClusterCells: 32, WaterHash: w.Hash()})
	if err != nil {
		t.Fatalf("OpenNavCache: %v", err)
	}
	opened.Close()
	if _, err := OpenNavCache(path, NavKey{ClusterCells: 32}); !errors.Is(err, ErrNavCacheStale) {
		t.Errorf("opened without the water: %v, want ErrNavCacheStale", err)
	}
}
