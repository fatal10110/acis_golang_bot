package geotest

// The wall band, a terrain of its own in region-local cells, after the Orc
// village's wall (#504): a walkway BandTop high over columns 300-329 and
// rows 316-319, open ground south of it, and at its east end a ramp down to
// the ground in steps of 40 (columns 330-332), which a walker can climb. Its
// north, south and west edges drop to the ground, which no step climbs.
// North of it lies a yard, columns 300-329 and rows 296-315, walled in on
// its other three sides, so only a drop off the band reaches it. At clusters
// of 32 cells the band's south edge is the border of cluster rows 9 and 10,
// and the ramp stands in the next cluster column.
//
// WallBandTop, on the band, and WallBandOutside, on the ground below its
// south edge, are mutually reachable over the ramp; WallBandYard is
// reachable from the band one way only.
var (
	WallBandTop     = Point{CellX: 305, CellY: 317, Z: BandTop}
	WallBandOutside = Point{CellX: 305, CellY: 340, Z: Ground}
	WallBandYard    = Point{CellX: 315, CellY: 305, Z: Ground}
)

// BandTop is the wall band's walkway height.
const BandTop = 160

// WallBand returns the wall band's terrain.
func WallBand() *Terrain {
	t := &Terrain{}
	t.Raise(300, 316, 329, 319, BandTop)
	for i, h := range []int16{BandTop - 40, BandTop - 80, BandTop - 120} {
		t.Raise(330+i, 316, 330+i, 319, h)
	}
	return t.Raise(299, 295, 330, 295, Wall).Raise(299, 295, 299, 315, Wall).Raise(330, 295, 330, 315, Wall)
}
