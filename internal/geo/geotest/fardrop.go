package geotest

// The far drop, a terrain of its own in region-local cells: a plateau
// Plateau high over columns 450-550 and rows 300-400, whose edges drop to
// the ground but cannot be climbed, and east of it a wall Wall high along
// columns 762-767 from the region's north edge down to row 511, so the
// ground on its two sides meets only south of row 511. At clusters of 256
// cells, columns 1-3 and rows 1-2 hold the scene: the plateau crosses the
// border of columns 1 and 2, so it is an abstract SCC of its own, and the
// wall runs along the east edge of column 2 and ends on the border of rows
// 1 and 2, so no cluster holds ground on both of its sides.
//
// FarDropHigh, on the plateau, reaches FarDropLow, on the ground past the
// wall, one way only. The cell search from one to the other floods the
// ground west of the wall before it finds the way around its south end.
var (
	FarDropHigh = Point{CellX: 470, CellY: 350, Z: Plateau}
	FarDropLow  = Point{CellX: 900, CellY: 350, Z: Ground}
)

// FarDrop returns the far drop's terrain.
func FarDrop() *Terrain {
	return (&Terrain{}).Raise(450, 300, 550, 400, Plateau).Raise(762, 0, 767, 511, Wall)
}
