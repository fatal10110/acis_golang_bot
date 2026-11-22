package datapack

import (
	"encoding/xml"
	"fmt"
)

// WaterZone is one water zone (zones/WaterZone.xml): a box, bounds
// inclusive, inside which the server switches a player to swim moves. A
// position is in the zone when its X and Y lie within the box and its Z
// within MinZ..MaxZ, as the server's cuboid form contains it (acis_golang
// model/zone/form.go NewCuboid, model/geometry Territory.Contains and
// Rectangle.Contains).
type WaterZone struct {
	Name       string
	X1, Y1     int32 // the low corner
	X2, Y2     int32 // the high corner
	MinZ, MaxZ int32
}

// Contains reports whether the point lies in the zone, bounds inclusive.
func (z WaterZone) Contains(x, y, h int32) bool {
	return x >= z.X1 && x <= z.X2 && y >= z.Y1 && y <= z.Y2 && h >= z.MinZ && h <= z.MaxZ
}

type zonesFile struct {
	Zones []zoneElement `xml:"zone"`
}

type zoneElement struct {
	Attrs []xml.Attr     `xml:",any,attr"`
	Nodes []attrsElement `xml:"node"`
}

// LoadWaterZones reads the water zones file at path (the datapack's
// zones/WaterZone.xml), in file order. The server builds each zone's form
// from its shape (data/xml/zone.go buildZoneForm); the bot models the one
// shape the shipped file uses, a cuboid of two nodes, with its corners and Z
// band normalized as NewCuboid does. Any other shape, a cuboid without
// exactly two nodes, or a missing or mangled field fails the load: a water
// zone the planner cannot see would let a route into water.
func LoadWaterZones(path string) ([]WaterZone, error) {
	var f zonesFile
	if err := decodeFile(path, &f); err != nil {
		return nil, err
	}
	out := make([]WaterZone, 0, len(f.Zones))
	for i, el := range f.Zones {
		z, err := buildWaterZone(el)
		if err != nil {
			return nil, fmt.Errorf("datapack: %s: zone %d: %w", path, i, err)
		}
		out = append(out, z)
	}
	return out, nil
}

func buildWaterZone(el zoneElement) (WaterZone, error) {
	a := foldAttrs(el.Attrs)
	z := WaterZone{Name: a.strDefault("name", "")}
	shape := a.str("shape")
	z.MinZ, z.MaxZ = a.literal("minZ"), a.literal("maxZ")
	if a.err != nil {
		return WaterZone{}, a.err
	}
	if shape != "Cuboid" {
		return WaterZone{}, fmt.Errorf("water zone shape %q is not modeled", shape)
	}
	if len(el.Nodes) != 2 {
		return WaterZone{}, fmt.Errorf("cuboid zone wants 2 nodes, has %d", len(el.Nodes))
	}
	var xs, ys [2]int32
	for i, n := range el.Nodes {
		r := foldAttrs(n.Attrs)
		xs[i], ys[i] = r.int32("x"), r.int32("y")
		if r.err != nil {
			return WaterZone{}, fmt.Errorf("zone node: %w", r.err)
		}
	}
	z.X1, z.X2 = min(xs[0], xs[1]), max(xs[0], xs[1])
	z.Y1, z.Y2 = min(ys[0], ys[1]), max(ys[0], ys[1])
	if z.MinZ > z.MaxZ {
		z.MinZ, z.MaxZ = z.MaxZ, z.MinZ
	}
	return z, nil
}
