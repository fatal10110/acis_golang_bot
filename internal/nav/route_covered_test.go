package nav

import (
	"context"
	"encoding/binary"
	"hash/crc32"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// The covered hall, after the Orc village's west building (#640): a hall
// walled on four sides, roofed by a deck coveredRoof over its floor, with a
// doorway in its west wall and one in its east wall on the same rows. The
// line between the two outdoor points west and east of it runs through both
// doorways, so before #640 a route took it straight through the hall.
const (
	hallX0, hallY0 = 400, 400 // the walls' outer corner, region-local cells
	hallX1, hallY1 = 460, 420
	hallDoor0      = 408 // the doorways' rows, in both end walls
	hallDoor1      = 412
	coveredRoof    = 300 // over the floor by more than coverClear
)

var (
	hallWest   = geotest.Point{CellX: 380, CellY: 410, Z: geotest.Ground}
	hallEast   = geotest.Point{CellX: 480, CellY: 410, Z: geotest.Ground}
	hallInside = geotest.Point{CellX: 430, CellY: 410, Z: geotest.Ground}
)

// coveredHall returns the hall's terrain.
func coveredHall() *geotest.Terrain {
	t := &geotest.Terrain{}
	t.Raise(hallX0, hallY0, hallX1, hallY0, geotest.Wall).Raise(hallX0, hallY1, hallX1, hallY1, geotest.Wall)
	for _, x := range []int{hallX0, hallX1} {
		t.Raise(x, hallY0, x, hallDoor0-1, geotest.Wall).Raise(x, hallDoor1+1, x, hallY1, geotest.Wall)
	}
	return t.AddDeck(hallX0+1, hallY0+1, hallX1-1, hallY1-1, coveredRoof)
}

// inHall reports whether world point l lies under the hall's roof.
func inHall(l location.Location) bool {
	ox, oy := regionOrigin()
	x, y := engine.GeoX(l.X)-ox, engine.GeoY(l.Y)-oy
	return x > hallX0 && x < hallX1 && y > hallY0 && y < hallY1
}

// hallCells counts the points, every 8 units along route, under the roof.
func hallCells(route []location.Location) int {
	n := 0
	for i := 1; i < len(route); i++ {
		a, b := route[i-1], route[i]
		steps := max(1, int(segLen(a, b)/8))
		for k := 0; k <= steps; k++ {
			f := float64(k) / float64(steps)
			at := location.Location{X: a.X + int(f*float64(b.X-a.X)), Y: a.Y + int(f*float64(b.Y-a.Y)), Z: a.Z}
			if inHall(at) {
				n++
			}
		}
	}
	return n
}

var (
	hallOnce sync.Once
	hallEng  *engine.Engine
	hallErr  error
)

func hallEngine(tb testing.TB) *engine.Engine {
	tb.Helper()
	hallOnce.Do(func() { hallEng, hallErr = buildEngine(coveredHall()) })
	if hallErr != nil {
		tb.Fatalf("covered hall failed to build: %v", hallErr)
	}
	return hallEng
}

// hallHier builds the hierarchy over the hall at 32-cell clusters with the
// given BuildConfig.CoveredCost.
func hallHier(tb testing.TB, e *engine.Engine, covered int) *Hierarchy {
	tb.Helper()
	image, _, err := Build(context.Background(), BuildConfig{Engine: e, Clusters: window(32, 11, 11, 15, 14), ClusterCells: 32, Workers: 2, CoveredCost: covered})
	if err != nil {
		tb.Fatal(err)
	}
	g, err := parseGraph(image)
	if err != nil {
		tb.Fatal(err)
	}
	h := NewHierarchy(g, e, nil, DefaultMemberCacheBytes)
	h.sameCap = testSameCap(tb)
	return h
}

// TestIsCovered pins the covered test: a layer of the cell's column more
// than coverClear over its floor, the walker's open test.
func TestIsCovered(t *testing.T) {
	e := hallEngine(t)
	p := &Planner{Engine: e}
	var buf [block.MaxLayers]block.Cell
	for _, tc := range []struct {
		name string
		c    Cell
		want bool
	}{
		{"hall floor", pointCell(hallInside), true},
		{"roof deck", geoCell(hallInside.CellX, hallInside.CellY, coveredRoof), false},
		{"doorway", geoCell(hallX0, hallDoor0, geotest.Ground), false},
		{"outside", pointCell(hallWest), false},
	} {
		if got := p.isCovered(tc.c, &buf); got != tc.want {
			t.Errorf("%s: isCovered = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestRouteCoveredGoesRound routes between the outdoor points either side of
// the hall (#640): by the cell search and by the hierarchy, a route weighing
// covered ground goes round the hall within a bounded extra length, a route
// to a point inside still enters it, and a planner and cache weighing none
// take the line through both doorways, as before.
func TestRouteCoveredGoesRound(t *testing.T) {
	e := hallEngine(t)
	weighed, unweighed := hallHier(t, e, 0), hallHier(t, e, -1)
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 400000}
	off := p.withCovered(0)
	west, east, inside := loc(hallWest), loc(hallEast), loc(hallInside)
	line := segLen(west, east)
	rng := rand.New(rand.NewPCG(7, 8))
	for _, tc := range []struct {
		name     string
		p        *Planner
		h        *Hierarchy // nil: the cell search
		from, to location.Location
		indoor   bool
	}{
		{"cell search round", p, nil, west, east, false},
		{"cell search back", p, nil, east, west, false},
		{"hierarchy round", p, weighed, west, east, false},
		{"hierarchy back", p, weighed, east, west, false},
		{"cell search inside", p, nil, west, inside, true},
		{"hierarchy inside", p, weighed, west, inside, true},
		{"hierarchy out", p, weighed, inside, east, true},
		{"cell search unweighed", &off, nil, west, east, true},
		// The hierarchy weighs covered ground as its cache was built,
		// whatever the planner it is given.
		{"hierarchy unweighed", p, unweighed, west, east, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var route []location.Location
			var err error
			if tc.h != nil {
				route, err = tc.h.Route(tc.p, tc.from, tc.to, nil)
			} else {
				route, err = tc.p.Route(tc.from, tc.to)
			}
			if err != nil {
				t.Fatal(err)
			}
			checkRoute(t, tc.p, rng, route, tc.from, tc.to)
			if in := hallCells(route); (in > 0) != tc.indoor {
				t.Errorf("route %v passes %d points under the roof, want indoor %v", route, in, tc.indoor)
			}
			if !tc.indoor {
				// Round the hall's 21 rows: a third longer than the line
				// at most.
				if l := routeLength(route); l > 4*line/3 {
					t.Errorf("route %v is %.0f long, the line %.0f", route, l, line)
				}
			}
		})
	}
}

// TestGraphCoveredCost pins the covered cost a nav cache records: the default
// for 0, none for a negative BuildConfig.CoveredCost, the value given
// otherwise; a build past maxCoveredCost and an image recording more are
// refused.
func TestGraphCoveredCost(t *testing.T) {
	e := hallEngine(t)
	keys := window(32, 11, 11, 11, 11)
	for _, tc := range []struct {
		cfg, want int
	}{
		{0, DefaultCoveredCost},
		{-1, 0},
		{25, 25},
		{maxCoveredCost, maxCoveredCost},
	} {
		image, _, err := Build(context.Background(), BuildConfig{Engine: e, Clusters: keys, ClusterCells: 32, CoveredCost: tc.cfg})
		if err != nil {
			t.Fatalf("CoveredCost %d: %v", tc.cfg, err)
		}
		g, err := parseGraph(image)
		if err != nil {
			t.Fatalf("CoveredCost %d: %v", tc.cfg, err)
		}
		if got := g.CoveredCost(); got != tc.want {
			t.Errorf("CoveredCost %d: graph records %d, want %d", tc.cfg, got, tc.want)
		}
		if tc.cfg == 25 {
			binary.LittleEndian.PutUint32(image[68:], maxCoveredCost+1)
			binary.LittleEndian.PutUint32(image[12:], crc32.Checksum(image[16:], navCRC))
			if _, err := parseGraph(image); err == nil {
				t.Errorf("an image recording covered cost %d parsed", maxCoveredCost+1)
			}
		}
	}
	if _, _, err := Build(context.Background(), BuildConfig{Engine: e, Clusters: keys, ClusterCells: 32, CoveredCost: maxCoveredCost + 1}); err == nil {
		t.Errorf("a build with covered cost %d succeeded", maxCoveredCost+1)
	}
}
