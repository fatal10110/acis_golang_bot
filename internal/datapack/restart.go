package datapack

import (
	"fmt"
	"strconv"
	"strings"
)

// RestartPoint is one <point> of restartPointAreas.xml, reduced to its name
// and the map tiles it covers: a player standing on one of the tiles is in
// the point's restart region.
type RestartPoint struct {
	Name string
	// Tiles are its <set name="map" val="X;Y"/> tiles in file order; nil for
	// none.
	Tiles []MapTile
}

// MapTile is one map tile (region) of the world grid, by its X and Y index.
type MapTile struct {
	X, Y int
}

type restartFile struct {
	Points []restartPointElement `xml:"point"`
}

type restartPointElement struct {
	Sets []setElement `xml:"set"`
}

// LoadRestartPoints reads the restart points of restartPointAreas.xml at
// path in file order, the way the server's buildRestartPoint does
// (data/xml/restart.go): every map value is "X;Y", two base-10 numbers, and
// the name is required. The restart areas and the other point values are
// not read. A file that is not well-formed or a mangled value fails the
// load.
func LoadRestartPoints(path string) ([]RestartPoint, error) {
	var f restartFile
	if err := decodeFile(path, &f); err != nil {
		return nil, err
	}
	out := make([]RestartPoint, 0, len(f.Points))
	for i, el := range f.Points {
		vals := make(map[string]string, len(el.Sets))
		var tiles []MapTile
		for _, s := range el.Sets {
			if s.Name != "map" {
				vals[s.Name] = s.Val
				continue
			}
			tile, err := parseMapTile(s.Val)
			if err != nil {
				return nil, fmt.Errorf("datapack: %s: point #%d: %w", path, i+1, err)
			}
			tiles = append(tiles, tile)
		}
		a := &attrReader{vals: vals}
		p := RestartPoint{Name: a.str("name"), Tiles: tiles}
		if a.err != nil {
			return nil, fmt.Errorf("datapack: %s: point #%d: %w", path, i+1, a.err)
		}
		out = append(out, p)
	}
	return out, nil
}

// parseMapTile parses a map value as the server's restart.ParsePointValue
// does.
func parseMapTile(raw string) (MapTile, error) {
	x, y, ok := strings.Cut(raw, ";")
	if !ok {
		return MapTile{}, fmt.Errorf("map %q must be formatted x;y", raw)
	}
	xn, err := strconv.Atoi(x)
	if err != nil {
		return MapTile{}, fmt.Errorf("map %q: %w", raw, err)
	}
	yn, err := strconv.Atoi(y)
	if err != nil {
		return MapTile{}, fmt.Errorf("map %q: %w", raw, err)
	}
	return MapTile{X: xn, Y: yn}, nil
}
