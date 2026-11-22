package nav

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/door"
	"github.com/fatal10110/acis_golang_bot/internal/geo/dynamic"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// doorOnlyScene is doorScene with the wall at column 1824 run across the
// whole region, so door 7's gap at rows 1822..1825 is the only way across.
func doorOnlyScene(tb testing.TB) (*engine.Engine, []*door.Template) {
	tb.Helper()
	t := &geotest.Terrain{}
	t.Raise(1824, 0, 1824, 1821, geotest.Wall).Raise(1824, 1826, 1824, regionCells-1, geotest.Wall)
	e, err := buildEngine(t)
	if err != nil {
		tb.Fatal(err)
	}
	return e, []*door.Template{doorAt(doorID, 1824, 1822, 1825)}
}

// closeDoor toggles door tmpl closed in e, as the door table does.
func closeDoor(tb testing.TB, e *engine.Engine, tmpl *door.Template) {
	tb.Helper()
	obj, err := dynamic.NewDoorObject(tmpl, e.Static())
	if err != nil {
		tb.Fatal(err)
	}
	e.AddObject(obj)
}

// wallCrossings returns the region-local rows at which the segments of route
// reach the wall column 1824 (two for a waypoint on it).
func wallCrossings(route []location.Location) []int {
	ox, oy := regionOrigin()
	var rows []int
	for i := 1; i < len(route); i++ {
		a, b := route[i-1], route[i]
		for s := 0; s <= 256; s++ {
			x := engine.GeoX(a.X+(b.X-a.X)*s/256) - ox
			if x == 1824 {
				rows = append(rows, engine.GeoY(a.Y+(b.Y-a.Y)*s/256)-oy)
				break
			}
		}
	}
	return rows
}

// TestRouteDoors checks Route under a door state: a closed door on the
// cheapest way detours by the door-free way; a closed door that is the only
// way gives ErrDoor, never ErrUnreachable, even when only the door state
// says so; an open door routes through; and a budget the searches exhaust
// is reported as ErrBudget, not as a door.
func TestRouteDoors(t *testing.T) {
	closedDoor := func(d int) bool { return d == doorID }
	from := loc(geotest.Point{CellX: 1760, CellY: 1823, Z: geotest.Ground})
	to := loc(geotest.Point{CellX: 1888, CellY: 1823, Z: geotest.Ground})
	tests := []struct {
		name       string
		scene      func(testing.TB) (*engine.Engine, []*door.Template)
		engineShut bool // the door object is in the engine
		closed     func(door int) bool
		budget     int
		want       error
		inGap      bool // the route crosses the wall through the door gap
	}{
		{"open door on the cheapest way", doorScene, false, nil, 40000, nil, true},
		{"closed door on the cheapest way", doorScene, true, closedDoor, 40000, nil, false},
		{"only way, open", doorOnlyScene, false, nil, 40000, nil, true},
		{"only way, open by the door state", doorOnlyScene, false, func(int) bool { return false }, 40000, nil, true},
		{"only way, closed", doorOnlyScene, true, closedDoor, 40000, ErrDoor, false},
		{"only way, closed by the door state alone", doorOnlyScene, false, closedDoor, 40000, ErrDoor, false},
		{"detour over budget", doorScene, true, closedDoor, 1, ErrBudget, false},
		{"door-open search over budget", doorOnlyScene, true, closedDoor, 1, ErrBudget, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, doors := tt.scene(t)
			th := buildHier(t, e, 64, doors, window(64, 27, 28, 29, 28))
			if tt.engineShut {
				closeDoor(t, e, doors[0])
			}
			p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: tt.budget}
			route, err := th.h.Route(p, from, to, tt.closed)
			if !errors.Is(err, tt.want) || (tt.want == nil) != (err == nil) {
				t.Fatalf("Route = %v, want %v", err, tt.want)
			}
			if errors.Is(err, ErrDoor) && errors.Is(err, ErrUnreachable) {
				t.Fatal("ErrDoor counts as ErrUnreachable")
			}
			if err != nil {
				return
			}
			rows := wallCrossings(route)
			if len(rows) == 0 {
				t.Fatalf("route %v never reaches the wall column", route)
			}
			for _, y := range rows {
				if gap := y >= 1822 && y <= 1825; gap != tt.inGap {
					t.Fatalf("route %v crosses the wall at row %d, through the door gap %v, want %v", route, y, gap, tt.inGap)
				}
			}
		})
	}
}
