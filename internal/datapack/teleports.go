package datapack

import (
	"encoding/xml"
	"fmt"
)

// TeleportStandard is the destination kind a gatekeeper's teleport list
// shows (travel.KindStandard); the other kinds (clan hall, noble, newbie
// token, ally) are reached through other links.
const TeleportStandard = "STANDARD"

// Teleport is one gatekeeper destination (teleports.xml <loc>).
type Teleport struct {
	Desc string
	// Kind is the type attribute, TeleportStandard when absent, as on the
	// server.
	Kind string
	// PriceID is the item template the trip costs (57 adena), PriceCount its
	// base count: the server halves an adena price in weekend core time and
	// never raises it (travel.Teleport.CalculatedPrice).
	PriceID    int32
	PriceCount int32
	X, Y, Z    int32
}

type teleportsFile struct {
	Lists []teleportListElement `xml:"telPosList"`
}

type teleportListElement struct {
	Attrs []xml.Attr     `xml:",any,attr"`
	Locs  []attrsElement `xml:"loc"`
}

// LoadTeleports reads teleports.xml at path: every gatekeeper's
// destinations by NPC template id, in file order. The order is the
// server's: a destination's index in its list is the index of its
// "teleport <index>" link. A list missing npcId, or a <loc> missing or
// mangling a field, fails the load, as on the server (data/xml/travel.go).
// A list declared twice keeps its last declaration.
func LoadTeleports(path string) (map[int32][]Teleport, error) {
	var f teleportsFile
	if err := decodeFile(path, &f); err != nil {
		return nil, err
	}
	out := make(map[int32][]Teleport, len(f.Lists))
	for _, l := range f.Lists {
		a := foldAttrs(l.Attrs)
		npcID := a.int32("npcId")
		if a.err != nil {
			return nil, fmt.Errorf("datapack: %s: telPosList: %w", path, a.err)
		}
		list := make([]Teleport, 0, len(l.Locs))
		for _, loc := range l.Locs {
			r := foldAttrs(loc.Attrs)
			t := Teleport{
				Desc:       r.str("desc"),
				Kind:       r.strDefault("type", TeleportStandard),
				PriceID:    r.int32("priceId"),
				PriceCount: r.int32("priceCount"),
				X:          r.int32("x"),
				Y:          r.int32("y"),
				Z:          r.int32("z"),
			}
			if r.err != nil {
				return nil, fmt.Errorf("datapack: %s: telPosList %d: %w", path, npcID, r.err)
			}
			list = append(list, t)
		}
		out[npcID] = list
	}
	return out, nil
}

// LoadInstantTeleports reads instantTeleports.xml at path: the points each
// NPC template teleports its talker to with no list page, by template id,
// in file order (the Seven Signs dungeon gatekeepers take a talker to their
// first, server gatekeeper.Instant). Only X, Y and Z of a Teleport are set.
// A list missing npcId, or a <loc> missing or mangling a coordinate, fails
// the load.
func LoadInstantTeleports(path string) (map[int32][]Teleport, error) {
	var f teleportsFile
	if err := decodeFile(path, &f); err != nil {
		return nil, err
	}
	out := make(map[int32][]Teleport, len(f.Lists))
	for _, l := range f.Lists {
		a := foldAttrs(l.Attrs)
		npcID := a.int32("npcId")
		if a.err != nil {
			return nil, fmt.Errorf("datapack: %s: telPosList: %w", path, a.err)
		}
		list := make([]Teleport, 0, len(l.Locs))
		for _, loc := range l.Locs {
			r := foldAttrs(loc.Attrs)
			t := Teleport{X: r.int32("x"), Y: r.int32("y"), Z: r.int32("z")}
			if r.err != nil {
				return nil, fmt.Errorf("datapack: %s: telPosList %d: %w", path, npcID, r.err)
			}
			list = append(list, t)
		}
		out[npcID] = list
	}
	return out, nil
}
