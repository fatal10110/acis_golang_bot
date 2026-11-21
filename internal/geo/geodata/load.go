package geodata

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/config"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/reader"
)

// GeoType selects which geodata region-file format geopack reads.
type GeoType string

const (
	L2OFF GeoType = "L2OFF"
	L2J   GeoType = "L2J"
)

// Region names one geodata region tile by its file coordinates.
type Region struct {
	X, Y int
}

// ListedRegions returns the region tiles geoengine.properties enables, in tile
// order: every in-range tile whose exact "X_Y" key is present, whatever its
// value; a commented-out "#X_Y" line is no key and leaves the tile unlisted.
func ListedRegions(props *config.Properties) []Region {
	if props == nil {
		return nil
	}
	var regions []Region
	for x := engine.TileXMin; x <= engine.TileXMax; x++ {
		for y := engine.TileYMin; y <= engine.TileYMax; y++ {
			if _, ok := props.Lookup(fmt.Sprintf("%d_%d", x, y)); ok {
				regions = append(regions, Region{X: x, Y: y})
			}
		}
	}
	return regions
}

// Valid reports whether t names a geodata format the server reads.
func (t GeoType) Valid() bool {
	return t == L2OFF || t == L2J
}

// RegionPath returns the file of region (x, y) in dir for geoType.
func RegionPath(dir string, geoType GeoType, x, y int) string {
	if geoType == L2J {
		return filepath.Join(dir, fmt.Sprintf("%d_%d.l2j", x, y))
	}
	return filepath.Join(dir, fmt.Sprintf("%d_%d_conv.dat", x, y))
}

// readRegion returns the region alongside the count of unread trailing bytes
// in its file, which only the L2OFF format reports.
func readRegion(path string, geoType GeoType) (*block.Region, int, error) {
	if geoType == L2J {
		f, err := os.Open(path)
		if err != nil {
			return nil, 0, err
		}
		defer f.Close()
		region, err := reader.ReadL2J(f)
		return region, 0, err
	}
	return reader.ReadL2OFF(path)
}
