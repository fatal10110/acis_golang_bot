package botclient

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// The navigation the spot and trip tests walk on: open flat ground east and
// south of the world origin (the fixture region's north-west corner), split
// by a wall along region-local columns 512-515 that runs the whole region,
// so world X 0-8191 and 8256-16383 (Y 0-8191) are two abstract SCCs no walk
// joins (synthetic geodata, never real). East of the wall a second wall
// along columns 766-769 (world X 12256-12319), on the border of two
// clusters, runs from the region's north edge to row 450 (world Y 7215):
// the walk between its sides goes around its south end.
var (
	splitOnce   sync.Once
	splitShared *Nav
	splitErr    error
)

// splitNav is the shared split navigation, built once per test binary.
func splitNav(tb testing.TB) *Nav {
	tb.Helper()
	splitOnce.Do(func() {
		t := (&geotest.Terrain{}).
			Raise(512, 0, 515, block.RegionBlocksY*block.CellsY-1, geotest.Wall).
			Raise(766, 0, 769, 450, geotest.Wall)
		splitShared, splitErr = buildTestNav(t, navWindow(0, 0, 3, 1))
	})
	if splitErr != nil {
		tb.Fatal(splitErr)
	}
	return splitShared
}

// walkSplit has brain b walk on the split navigation.
func walkSplit(t *testing.T, b *Brain) *Brain {
	t.Helper()
	n := splitNav(t)
	b.walk = newWalker(n)
	b.world.SetGround(n.Engine)
	return b
}

// TestSplitNav pins the ground the spot tests stand on: each side of the
// wall is one SCC, the two are not.
func TestSplitNav(t *testing.T) {
	b := newSpotBrain(t, 1)
	west, east := []Point{{}, {X: 4000}, {Y: 5000}, {X: 8000, Y: 8000}}, []Point{{X: 9000}, {X: 16000, Y: 8000}}
	var first, other navPoint
	for i, p := range append(west, east...) {
		np, ok := b.locate(p)
		if !ok {
			t.Fatalf("%+v not on the nav", p)
		}
		switch {
		case i == 0:
			first = np
		case i == len(west):
			other = np
		}
		side := first
		if i >= len(west) {
			side = other
		}
		if !b.walkable(side, np) {
			t.Fatalf("%+v not in its side's SCC", p)
		}
	}
	if b.walkable(first, other) || b.walkable(other, first) {
		t.Fatal("the two sides join")
	}
}

// pickStraight is pickSpot on open ground where every spot lies in the
// bot's SCC, its walk the straight line: what the tests over the real
// datapack's starter spots pick with, since unit tests have no real
// geodata. It plans no trip.
func pickStraight(b *Brain, now time.Time) {
	v, p := &b.spot, &b.persona.Spots
	from := b.world.Self.Position(now)
	far := max(float64(p.MaxTravel), float64(p.Wander))
	var cands []spotCand
	for i := range b.cfg.Spots.spots {
		sp := &b.cfg.Spots.spots[i]
		if (v.held && i == v.idx) || now.Before(v.avoid[i]) || !b.spotFits(sp, now) {
			continue
		}
		c := Point{X: sp.Center.X, Y: sp.Center.Y, Z: sp.Z}
		if d := math.Sqrt(float64(horizontal2(from, c))); d <= far {
			cands = append(cands, spotCand{i: i, d: d, line: d, at: c})
		}
	}
	b.claimSpot(cands, now, nil)
}

// TestSpotPickReachable checks pickSpot claims only spots in the bot's SCC,
// nearest by the abstract graph rather than the straight line, and with
// fitting spots only in another SCC plans a trip (#158).
func TestSpotPickReachable(t *testing.T) {
	now := at(time.Second)
	tests := []struct {
		name  string
		self  Point
		spots []datapack.Spot
		// want is the spot held after, -1 for none.
		want int
	}{
		{
			name: "nearest across the wall: never claimed", self: Point{X: 7900},
			spots: []datapack.Spot{testSpot("across", 8700, 0, 3, 6, 9), testSpot("west", 4000, 0, 3, 6, 9)},
			want:  1,
		},
		{
			name: "only across the wall: none", self: Point{X: 7900},
			spots: []datapack.Spot{testSpot("across", 8700, 0, 3, 6, 9)},
			want:  -1,
		},
		{
			// 800 away behind the second wall, 3000 on open ground: the
			// walk around the wall's end is the longer.
			name: "nearest by walking, not straight line", self: Point{X: 11840},
			spots: []datapack.Spot{testSpot("behind", 12640, 0, 3, 6, 9), testSpot("open", 11840, 3000, 3, 6, 9)},
			want:  1,
		},
		{
			name: "behind the wall within Wander: claimed", self: Point{X: 11840},
			spots: []datapack.Spot{testSpot("behind", 12640, 0, 3, 6, 9)},
			want:  0,
		},
		{
			name: "off the navigation: none", self: Point{X: 4000},
			spots: []datapack.Spot{testSpot("off", 4000, 20000, 3, 6, 9)},
			want:  -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 5, tt.spots...)
			b.world.Self.move = stopAt(tt.self)
			b.pickSpot(now)
			got := -1
			if b.spot.held {
				got = b.spot.idx
			}
			if got != tt.want {
				t.Fatalf("spot %d, want %d", got, tt.want)
			}
			if got < 0 && !b.spot.retryAt.Equal(now.Add(spotRetry)) {
				t.Fatalf("no spot: looks again at %v, want spotRetry later", b.spot.retryAt)
			}
		})
	}
	t.Run("walk distances", func(t *testing.T) {
		b := newSpotBrain(t, 5, testSpot("behind", 12640, 0, 3, 6, 9), testSpot("open", 11840, 3000, 3, 6, 9))
		b.world.Self.move = stopAt(Point{X: 11840})
		here, ok := b.locate(Point{X: 11840})
		if !ok {
			t.Fatal("bot off the navigation")
		}
		cands := b.spotCands(here, now)
		if len(cands) != 2 || cands[0].d < 2*7000 || math.Abs(cands[1].d-3000) > 50 {
			t.Fatalf("candidates %+v, want behind around the wall's end (over 14000) and open at 3000", cands)
		}
	})
}

// TestSpotTripOtherSCC checks that with the only fitting spot in another SCC,
// nearer in straight line than MaxTravel, the bot claims nothing and plans
// the trip by gatekeeper to it (#158).
func TestSpotTripOtherSCC(t *testing.T) {
	b := newTravelBrain(t, 5000)
	b.decide(at(time.Second))
	if b.spot.held || !b.trip.v.active() || b.trip.v.npcID != 30006 || b.trip.dest.index != 2 {
		t.Fatalf("spot %+v trip %+v, want no spot and the trip to destination 2", b.spot, b.trip)
	}
}

// TestSpotSnapFiltered checks that a spot centre off the bot's SCC, on the
// wall or across it, snaps to the nearest cell inside the spot's territory
// on the bot's side, not just to the nearest walkable cell (#158).
func TestSpotSnapFiltered(t *testing.T) {
	now := at(time.Second)
	square := func(x0, x1 int32) *datapack.Territory {
		return &datapack.Territory{Name: "t", Nodes: []datapack.Point{{X: x0, Y: 1800}, {X: x1, Y: 1800}, {X: x1, Y: 2200}, {X: x0, Y: 2200}}}
	}
	tests := []struct {
		name      string
		x         int32
		territory *datapack.Territory
		// claimed reports whether the bot claims the spot, at a point west
		// of the wall.
		claimed bool
	}{
		{name: "centre on the wall, territory reaching west", x: 8224, territory: square(7800, 8300), claimed: true},
		{name: "centre across the wall, territory reaching west", x: 8300, territory: square(7800, 8400), claimed: true},
		{name: "centre on the wall, no territory", x: 8224, claimed: true},
		{name: "territory only on the wall and across it", x: 8224, territory: square(8200, 8600)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sp := testSpot("wall", tt.x, 2000, 3, 6, 9)
			sp.Territory = tt.territory
			b := newSpotBrain(t, 5, sp)
			b.world.Self.move = stopAt(Point{X: 7900, Y: 2000})
			here, ok := b.locate(Point{X: 7900, Y: 2000})
			if !ok {
				t.Fatal("bot off the navigation")
			}
			cands := b.spotCands(here, now)
			if !tt.claimed {
				if len(cands) != 0 {
					t.Fatalf("candidates %+v, want none", cands)
				}
				return
			}
			if len(cands) != 1 || cands[0].at.X >= 8192 || 8192-cands[0].at.X > 100 {
				t.Fatalf("candidates %+v, want the spot at a point just west of the wall", cands)
			}
			b.pickSpot(now)
			if !b.spot.held {
				t.Fatal("spot not claimed")
			}
		})
	}
}

// TestTravelOnward checks that on a path of two teleports the first
// landing plans the second at once, not after TravelPolicy.Gap (#158).
func TestTravelOnward(t *testing.T) {
	b := newTravelBrain(t, 50000)
	b.persona.Spots.MaxTravel = 5000
	b.cfg.Travel.gatekeepers[0].dests = b.cfg.Travel.gatekeepers[0].dests[:1]
	b.cfg.Travel.gatekeepers = append(b.cfg.Travel.gatekeepers, gatekeeper{
		npcID: 30059, at: Point{X: 6500, Y: 7000}, dests: []destination{{index: 5, at: Point{X: travelDest}, price: 1000}},
	})
	b.decide(at(time.Second))
	if tr := &b.trip; tr.v.npcID != 30006 || tr.dest.index != 0 || tr.onward != 1 {
		t.Fatalf("trip %+v, want gatekeeper 30006 destination 0 with one teleport onward", b.trip)
	}
	b.world.Self.move = stopAt(Point{X: 6000, Y: 7000})
	b.tripTeleported(at(2 * time.Second))
	b.decide(at(3 * time.Second))
	if tr := &b.trip; tr.v.npcID != 30059 || tr.dest.index != 5 || tr.onward != 0 {
		t.Fatalf("after the first landing: trip %+v, want gatekeeper 30059 destination 5, the last teleport", b.trip)
	}
}
