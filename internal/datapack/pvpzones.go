package datapack

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
)

// Zones are the zones that decide where a character may fight, read from
// the datapack's zones directory, each kind in file order: the arenas
// (ArenaZone.xml), the peace zones (PeaceZone.xml), the towns
// (TownZone.xml), the Derby track (DerbyTrackZone.xml), the castle and clan
// hall battlefields (SiegeZone.xml) and the no-restart zones
// (NoRestartZone.xml). The server registers each file's zones as the kind
// its base name names (data/xml/zone.go zoneBuilders).
type Zones struct {
	Arenas, Peace, Towns, DerbyTracks, Sieges, NoRestart []Zone
}

// Zone is one zone of Zones: a vertical prism over a polygon or a box, and
// the settings (<stat> children) its kind reads.
type Zone struct {
	// Cuboid is set for a Cuboid zone: X and Y then hold its low and high
	// corner, both edges inclusive, as the server's rectangle contains a
	// point (model/geometry Rectangle.Contains). For an NPoly zone X and Y
	// are its vertex ring in file order.
	Cuboid bool
	X, Y   []int32
	// MinZ and MaxZ bound the heights the zone holds, inclusive. A Cuboid's
	// band is normalized as NewCuboid does; an NPoly's is kept as written,
	// so one with MinZ over MaxZ holds no point (model/zone/form.go
	// NewPolygon, model/geometry Territory.Contains).
	MinZ, MaxZ int32
	// TownID and CastleID are a town's townId and castleId (default 0);
	// Peaceful is its isPeaceZone (default true: a town is a peace zone
	// unless it says otherwise, model/zone NewTown).
	TownID, CastleID int
	Peaceful         bool
	// ResidenceCastle and ResidenceClanHall are a siege zone's castleId and
	// clanHallId, 0 when absent: the battlefield of that castle's or clan
	// hall's siege (data/xml/zone.go buildSiegeZone).
	ResidenceCastle, ResidenceClanHall int
}

// Contains reports whether the point lies in the zone: its height within
// MinZ..MaxZ, and its X and Y inside the box, edges inclusive, or inside
// the polygon by the server's even-odd ray cast.
func (z Zone) Contains(x, y, h int32) bool {
	if h < z.MinZ || h > z.MaxZ {
		return false
	}
	if z.Cuboid {
		return x >= z.X[0] && x <= z.X[1] && y >= z.Y[0] && y <= z.Y[1]
	}
	return ringContains(z.X, z.Y, x, y)
}

// ringContains is the server's zone polygon test (model/geometry
// vertexRingContains): even-odd ray casting over the vertex ring, half-open
// on the boundary. The arithmetic is int32, as on the server, so a point
// near a long edge classifies the same way on both sides.
func ringContains(xs, ys []int32, px, py int32) bool {
	inside := false
	for i, j := 0, len(xs)-1; i < len(xs); j, i = i, i+1 {
		yi, yj := ys[i], ys[j]
		if ((yi <= py && py < yj) || (yj <= py && py < yi)) &&
			px < (xs[j]-xs[i])*(py-yi)/(yj-yi)+xs[i] {
			inside = !inside
		}
	}
	return inside
}

// FindZone returns the first of zones that contains the point.
func FindZone(zones []Zone, x, y, h int32) (Zone, bool) {
	for _, z := range zones {
		if z.Contains(x, y, h) {
			return z, true
		}
	}
	return Zone{}, false
}

type pvpZoneElement struct {
	Attrs []xml.Attr     `xml:",any,attr"`
	Stats []attrsElement `xml:"stat"`
	Nodes []attrsElement `xml:"node"`
}

type pvpZonesFile struct {
	Zones []pvpZoneElement `xml:"zone"`
}

// LoadZones reads the six zone files of Zones from dir, the datapack's
// zones directory. The server builds a zone's form from its shape
// (data/xml/zone.go buildZoneForm); the bot models the two shapes these
// files use: Cuboid, two nodes, and NPoly, more than two. Any other shape
// (a Cylinder), a wrong node count, or a missing or mangled field fails the
// load: a zone the bot cannot see would put it in a fight it did not
// choose. Water zones are LoadWaterZones'.
func LoadZones(dir string) (Zones, error) {
	var zs Zones
	for _, f := range []struct {
		name string
		dst  *[]Zone
	}{
		{"ArenaZone", &zs.Arenas},
		{"PeaceZone", &zs.Peace},
		{"TownZone", &zs.Towns},
		{"DerbyTrackZone", &zs.DerbyTracks},
		{"SiegeZone", &zs.Sieges},
		{"NoRestartZone", &zs.NoRestart},
	} {
		path := filepath.Join(dir, f.name+".xml")
		var file pvpZonesFile
		if err := decodeFile(path, &file); err != nil {
			return Zones{}, err
		}
		out := make([]Zone, 0, len(file.Zones))
		for i, el := range file.Zones {
			z, err := buildZone(f.name, el)
			if err != nil {
				return Zones{}, fmt.Errorf("datapack: %s: zone %d: %w", path, i, err)
			}
			out = append(out, z)
		}
		*f.dst = out
	}
	return zs, nil
}

// buildZone reads one zone of the kind file names: its form, then the
// settings that kind reads. Settings fold into one map, the last of a
// repeated name winning, as on the server.
func buildZone(file string, el pvpZoneElement) (Zone, error) {
	a := foldAttrs(el.Attrs)
	shape := a.str("shape")
	z := Zone{MinZ: a.literal("minZ"), MaxZ: a.literal("maxZ")}
	if a.err != nil {
		return Zone{}, a.err
	}
	z.X, z.Y = make([]int32, len(el.Nodes)), make([]int32, len(el.Nodes))
	for i, n := range el.Nodes {
		r := foldAttrs(n.Attrs)
		z.X[i], z.Y[i] = r.int32("x"), r.int32("y")
		if r.err != nil {
			return Zone{}, fmt.Errorf("zone node: %w", r.err)
		}
	}
	switch {
	case len(el.Nodes) == 0:
		return Zone{}, fmt.Errorf("zone has no nodes")
	case shape == "Cuboid":
		if len(el.Nodes) != 2 {
			return Zone{}, fmt.Errorf("cuboid zone wants 2 nodes, has %d", len(el.Nodes))
		}
		z.Cuboid = true
		z.X[0], z.X[1] = min(z.X[0], z.X[1]), max(z.X[0], z.X[1])
		z.Y[0], z.Y[1] = min(z.Y[0], z.Y[1]), max(z.Y[0], z.Y[1])
		if z.MinZ > z.MaxZ {
			z.MinZ, z.MaxZ = z.MaxZ, z.MinZ
		}
	case shape == "NPoly":
		if len(el.Nodes) <= 2 {
			return Zone{}, fmt.Errorf("polygon zone wants more than 2 nodes, has %d", len(el.Nodes))
		}
	default:
		return Zone{}, fmt.Errorf("zone shape %q is not modeled", shape)
	}
	set := &attrReader{vals: make(map[string]string, len(el.Stats))}
	for _, s := range el.Stats {
		r := foldAttrs(s.Attrs)
		set.vals[r.strDefault("name", "")] = r.strDefault("val", "")
	}
	switch file {
	case "TownZone":
		z.TownID, z.CastleID = set.intDefault("townId", 0), set.intDefault("castleId", 0)
		z.Peaceful = set.boolDefault("isPeaceZone", true)
	case "SiegeZone":
		z.ResidenceCastle, z.ResidenceClanHall = set.intDefault("castleId", 0), set.intDefault("clanHallId", 0)
	}
	if set.err != nil {
		return Zone{}, fmt.Errorf("zone settings: %w", set.err)
	}
	return z, nil
}
