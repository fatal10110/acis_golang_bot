package botclient

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/door"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// roomDoorAt returns a door template, closed by default, whose footprint
// is region-local column x, rows y0..y1.
func roomDoorAt(id, x, y0, y1 int, opened bool) *door.Template {
	t := geotest.CorridorDoor()
	t.ID, t.Opened = id, opened
	wx0, wx1 := geotest.CellWorldX(x)-8, geotest.CellWorldX(x)+7
	wy0, wy1 := geotest.CellWorldY(y0)-8, geotest.CellWorldY(y1)+7
	t.Position.X, t.Position.Y = (wx0+wx1)/2, (wy0+wy1)/2
	t.Coordinates[0].X, t.Coordinates[0].Y = wx0, wy0
	t.Coordinates[1].X, t.Coordinates[1].Y = wx1, wy0
	t.Coordinates[2].X, t.Coordinates[2].Y = wx1, wy1
	t.Coordinates[3].X, t.Coordinates[3].Y = wx0, wy1
	return t
}

// TestWalkerDoorGoal walks to a walled room inside one cluster whose only
// way in is a door (#649): closed, the walk ends unreachable for the door,
// where the route's free search used to flood its budget away and end it
// for the route budget; open, the walk moves.
func TestWalkerDoorGoal(t *testing.T) {
	ter := &geotest.Terrain{}
	ter.Raise(850, 850, 880, 850, geotest.Wall).Raise(850, 880, 880, 880, geotest.Wall).Raise(880, 850, 880, 880, geotest.Wall).
		Raise(850, 850, 850, 862, geotest.Wall).Raise(850, 867, 850, 880, geotest.Wall)
	base, err := buildTestNav(ter, navWindow(2, 3, 3, 3))
	if err != nil {
		t.Fatal(err)
	}
	from := scenePoint(geotest.Point{CellX: 600, CellY: 865, Z: geotest.Ground}, 0, 0)
	dest := scenePoint(geotest.Point{CellX: 865, CellY: 865, Z: geotest.Ground}, 0, 0)
	// Open first: the closed table closes the door in the shared engine.
	for _, opened := range []bool{true, false} {
		doors, err := door.NewTable([]*door.Template{roomDoorAt(41, 850, 863, 866, opened)})
		if err != nil {
			t.Fatal(err)
		}
		n := *base
		n.Doors = NewDoorTable(n.Engine, doors, discardLog())
		b, _, _ := newNavBrain(t, &n, Persona{}, from)
		a, st := walkStep(t, b, dest, t0)
		switch {
		case opened && (st != walkWalking || a.kind != actMove):
			t.Errorf("door open: status %v kind %v (%s), want a move", st, a.kind, b.walk.reason)
		case !opened && (st != walkUnreachable || b.walk.reason != reasonDoor):
			t.Errorf("door closed: status %v kind %v (%s), want unreachable for the door", st, a.kind, b.walk.reason)
		}
	}
}
