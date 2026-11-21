package geotest

import (
	"github.com/fatal10110/acis_golang_bot/internal/geo/door"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// Fixture spots, in region-local cells. Each scene sits in its own window
// of the region, well apart from the others, on Ground.
var (
	// Corridor: a 4-cell-wide east-west passage between two walls, rows
	// 260-263, columns 256-319. CorridorDoor spans it at column 288.
	CorridorWest = Point{CellX: 258, CellY: 261, Z: Ground}
	CorridorEast = Point{CellX: 317, CellY: 261, Z: Ground}
	// AboveCorridor is open ground north of the corridor's north wall.
	AboveCorridor = Point{CellX: 288, CellY: 250, Z: Ground}

	// UWall: a U open to the west; its base is columns 540-543. A straight
	// line from inside the U to the east of the base is blocked; walking
	// around the arms is open.
	UInside = Point{CellX: 530, CellY: 285, Z: Ground}
	UBehind = Point{CellX: 560, CellY: 285, Z: Ground}

	// Ledge: a plateau Plateau high from column 800 east. Dropping off it is
	// open, climbing onto it is not.
	LedgeHigh = Point{CellX: 805, CellY: 285, Z: Plateau}
	LedgeLow  = Point{CellX: 790, CellY: 285, Z: Ground}

	// Pit: a 16x16 hole PitDown deep. Jumping in is open, climbing out not.
	PitOutside = Point{CellX: 1030, CellY: 287, Z: Ground}
	PitInside  = Point{CellX: 1047, CellY: 287, Z: PitDown}

	// Bridge: an east-west deck at Deck over open ground, a multilayer strip
	// at rows 286-289, columns 1290-1330.
	BridgeWestDeck   = Point{CellX: 1292, CellY: 287, Z: Deck}
	BridgeEastDeck   = Point{CellX: 1328, CellY: 287, Z: Deck}
	BridgeWestGround = Point{CellX: 1292, CellY: 287, Z: Ground}
	BridgeEastGround = Point{CellX: 1328, CellY: 287, Z: Ground}

	// Pocket: open ground enclosed by a ring of wall, columns 1550-1581 and
	// rows 270-301, two cells thick.
	PocketInside  = Point{CellX: 1565, CellY: 285, Z: Ground}
	PocketOutside = Point{CellX: 1540, CellY: 285, Z: Ground}
)

// Scenes returns the terrain holding every fixture scene.
func Scenes() *Terrain {
	t := &Terrain{}
	wall := func(x0, y0, x1, y1 int) {
		t.patches = append(t.patches, patch{rect{x0, y0, x1, y1}, Wall})
	}
	// Corridor.
	wall(256, 256, 319, 259)
	wall(256, 264, 319, 267)
	// U-wall: base, then the two arms reaching west.
	wall(540, 270, 543, 300)
	wall(520, 270, 543, 273)
	wall(520, 297, 543, 300)
	// Ledge.
	t.patches = append(t.patches, patch{rect{800, 256, 831, 319}, Plateau})
	// Pit.
	t.patches = append(t.patches, patch{rect{1040, 280, 1055, 295}, PitDown})
	// Bridge.
	t.decks = append(t.decks, patch{rect{1290, 286, 1330, 289}, Deck})
	// Pocket ring.
	wall(1550, 270, 1581, 271)
	wall(1550, 300, 1581, 301)
	wall(1550, 270, 1551, 301)
	wall(1580, 270, 1581, 301)
	return t
}

// CorridorDoor returns a closed door template across the corridor at
// column 288, as doors.xml would describe it: a thin rectangle footprint
// spanning the passage, its position at the footprint's centre.
func CorridorDoor() *door.Template {
	x0, x1 := CellWorldX(288)-8, CellWorldX(288)+7
	y0, y1 := CellWorldY(260)-8, CellWorldY(263)+7
	return &door.Template{
		ID:       1,
		Name:     "corridor_door",
		Kind:     door.KindDoor,
		Position: location.Location{X: (x0 + x1) / 2, Y: (y0 + y1) / 2, Z: Ground},
		Coordinates: []location.Point{
			{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1},
		},
		Height:   200,
		OpenKind: door.OpenNPC,
	}
}
