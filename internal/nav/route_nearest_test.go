package nav

import (
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// within reports whether c is at most r cells from o in both axes.
func within(c, o Cell, r int) bool { return abs(c.X-o.X) <= r && abs(c.Y-o.Y) <= r }

// landing is the cell of a route's last point on the floor it names.
func landing(route []location.Location) Cell {
	l := route[len(route)-1]
	return Cell{X: engine.GeoX(l.X), Y: engine.GeoY(l.Y), Z: int16(l.Z)}
}

// TestRouteToNearestCheapest takes the cheapest landing by path cost: from
// inside the U, a cell behind its base is nearer by distance than one out
// past its mouth, but the way round the arms costs more.
func TestRouteToNearestCheapest(t *testing.T) {
	p := testPlanner(t)
	behind, mouth := pointCell(geotest.UBehind), geoCell(geotest.UInside.CellX-35, geotest.UInside.CellY, geotest.Ground)
	route, err := p.RouteToNearest(loc(geotest.UInside), func(c Cell) bool { return c == behind || c == mouth })
	if err != nil {
		t.Fatal(err)
	}
	if got := landing(route); got != mouth {
		t.Fatalf("landing %+v, want the cell past the mouth %+v (behind the base: %+v)", got, mouth, behind)
	}
	checkRoute(t, p, rand.New(rand.NewPCG(1, 2)), route, loc(geotest.UInside), route[len(route)-1])
}

// TestRouteToNearestDoorBudget looks for a cell past the wall whose only
// gap a door fills: with the door open it walks through the gap, and with
// it closed every cell it may take lies behind the door, so the search
// expands at most the one budget and answers ErrBudget.
func TestRouteToNearestDoorBudget(t *testing.T) {
	e, doors := doorOnlyScene(t)
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 2000}
	from := loc(geotest.Point{CellX: 1815, CellY: 1823, Z: geotest.Ground})
	wall, _ := regionOrigin()
	wall += 1824
	calls := 0
	past := func(c Cell) bool {
		calls++
		return c.X > wall
	}
	route, err := p.RouteToNearest(from, past)
	if err != nil {
		t.Fatalf("door open: %v", err)
	}
	if got := landing(route); got.X != wall+1 {
		t.Fatalf("door open: landing %+v, want the column past the wall", got)
	}
	checkRoute(t, p, rand.New(rand.NewPCG(3, 4)), route, from, route[len(route)-1])

	closeDoor(t, e, doors[0])
	calls = 0
	if _, err := p.RouteToNearest(from, past); !errors.Is(err, ErrBudget) {
		t.Fatalf("door closed: %v, want ErrBudget", err)
	}
	// accept sees each popped cell once: every expanded cell and the one
	// popped when the budget is spent.
	if calls > p.Budget+1 {
		t.Errorf("door closed: %d cells popped, want at most the budget %d and one", calls, p.Budget)
	}
	s := getScratch()
	defer putScratch(s)
	if _, used, err := p.routeToNearest(s, from, past); !errors.Is(err, ErrBudget) || used > p.Budget {
		t.Errorf("door closed: %d cells expanded (%v), want at most the budget %d", used, err, p.Budget)
	}
}

// TestRouteToNearestLedgeLeavesBox drops off a walled plateau strip whose
// only way down is far north of the bot: the landing must lie within 32
// cells of the bot, the way down may not, and the route walks north, drops
// and walks back south.
func TestRouteToNearestLedgeLeavesBox(t *testing.T) {
	ter := &geotest.Terrain{}
	ter.Raise(1000, 1000, 1003, 1100, geotest.Plateau).
		Raise(999, 999, 999, 1090, geotest.Wall).
		Raise(1004, 999, 1004, 1090, geotest.Wall).
		Raise(999, 999, 1004, 999, geotest.Wall)
	e, err := buildEngine(ter)
	if err != nil {
		t.Fatal(err)
	}
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 40000}
	start := geotest.Point{CellX: 1001, CellY: 1005, Z: geotest.Plateau}
	sc := pointCell(start)
	route, err := p.RouteToNearest(loc(start), func(c Cell) bool { return c.Z == geotest.Ground && within(c, sc, 32) })
	if err != nil {
		t.Fatal(err)
	}
	if got := landing(route); got.Z != geotest.Ground || !within(got, sc, 32) {
		t.Fatalf("landing %+v, want ground within 32 cells of %+v", got, sc)
	}
	left := false
	for _, l := range route {
		if c := (Cell{X: engine.GeoX(l.X), Y: engine.GeoY(l.Y)}); !within(c, sc, 32) {
			left = true
		}
	}
	if !left {
		t.Errorf("route %v never leaves the box: the strip drops only past it", route)
	}
	checkRoute(t, p, rand.New(rand.NewPCG(5, 6)), route, loc(start), route[len(route)-1])
}

// TestRouteToNearestClickTarget refuses a landing that is no click target:
// the ground under the deck 60 above it, with a collision height of 40,
// takes a click on the deck, so the route walks under it to the clickable
// cell past it, although accept takes the ground under the deck first.
func TestRouteToNearestClickTarget(t *testing.T) {
	p := testPlanner(t)
	p.CollisionHeight = 40
	from := loc(geotest.Point{CellX: Floors60.CellX - 10, CellY: Floors60.CellY, Z: geotest.Ground})
	past := geoCell(Floors60.CellX+10, Floors60.CellY, geotest.Ground)
	ox, oy := regionOrigin()
	under := 0
	route, err := p.RouteToNearest(from, func(c Cell) bool {
		x, y := c.X-ox, c.Y-oy
		if c.Z == geotest.Ground && abs(x-Floors60.CellX) <= 5 && y >= 1000 && y <= 1010 {
			under++
			return true
		}
		return c == past
	})
	if err != nil {
		t.Fatal(err)
	}
	if under == 0 {
		t.Fatal("the search never popped the ground under the deck")
	}
	if got := landing(route); got != past {
		t.Fatalf("landing %+v, want the click target past the deck %+v", got, past)
	}
}

// TestRouteToNearestAvoid takes no cell in an avoided column, the landing's
// included: with no fixed goal there is no goal column to exempt.
func TestRouteToNearestAvoid(t *testing.T) {
	p := testPlanner(t)
	near, far := geoCell(Open.CellX+5, Open.CellY, geotest.Ground), geoCell(Open.CellX+10, Open.CellY, geotest.Ground)
	p.Avoid.Area = func(x, y int) bool { return engine.GeoX(x) == near.X && engine.GeoY(y) == near.Y }
	route, err := p.RouteToNearest(loc(Open), func(c Cell) bool { return c == near || c == far })
	if err != nil {
		t.Fatal(err)
	}
	if got := landing(route); got != far {
		t.Fatalf("landing %+v, want %+v past the avoided column", got, far)
	}
}

// TestRouteToNearestBreath swims to the shore past the lake with breath for
// the crossing, and with too little the swim fails breathOK and the route
// retries dry, walking round the lake.
func TestRouteToNearestBreath(t *testing.T) {
	shore := pointCell(WaterEast)
	accept := func(c Cell) bool { return within(c, shore, 1) }
	p := swimPlanner(t, 5000)
	route, err := p.RouteToNearest(loc(WaterWest), accept)
	if err != nil {
		t.Fatal(err)
	}
	assertSwimRoute(t, p, route)

	short := swimPlanner(t, 40)
	s := getScratch()
	if _, _, err := short.routeToNearest(s, loc(WaterWest), accept); !errors.Is(err, ErrBreath) {
		t.Errorf("breath 40, one search: %v, want ErrBreath", err)
	}
	putScratch(s)
	route, err = short.RouteToNearest(loc(WaterWest), accept)
	if err != nil {
		t.Fatalf("breath 40: %v", err)
	}
	if !within(landing(route), shore, 1) {
		t.Errorf("breath 40: landing %+v, want by %+v", landing(route), shore)
	}
	assertDry(t, short, short.Water, route)
}
