package datapack

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Point is one polygon node of a territory.
type Point struct {
	X, Y int32
}

// Territory is a spawn area: a polygon on the X/Y plane between MinZ and
// MaxZ.
type Territory struct {
	Name       string
	MinZ, MaxZ int32
	Nodes      []Point
}

// Maker is one <npcmaker>: a group of spawns sharing territories.
type Maker struct {
	Name string
	// Territories are the areas its random-position spawns appear in.
	Territories []*Territory
	// Banned are areas carved out of Territories (the ban attribute).
	Banned []*Territory
	// MaximumNPCs is the declared cap of live NPCs for the maker, before
	// the server's spawn multiplier.
	MaximumNPCs int
	// Event names the event that switches the maker on; empty for an
	// always-on maker.
	Event string
	// SpawnTime is the spawnTime attribute, such as
	// "siege_warfare_start(8)"; empty for none.
	SpawnTime string
	// AIType is the last <ai type> of the maker, else its maker attribute;
	// empty for neither.
	AIType string
	// Params are the maker's <ai> <set> parameters named in makerParams, the
	// last value of a name winning and "@" stripped, as the server's
	// flattenAI does; nil for none.
	Params map[string]string
	Spawns []Spawn
}

// makerParams are the maker <ai> parameters bots read: the event gate
// (EventName), the day or night half (IsNight), whether a linked maker
// starts at boot (on_start_spawn) and the linked set of an instant random
// maker (maker_name1 to maker_name3, maker_cnt).
var makerParams = map[string]bool{
	"EventName": true, "IsNight": true, "on_start_spawn": true,
	"maker_name1": true, "maker_name2": true, "maker_name3": true, "maker_cnt": true,
}

// Spawn is one <npc> entry of a maker.
type Spawn struct {
	NPCID int32
	// Total is the declared count, before the server's spawn multiplier
	// (which scales only spawns without Positions).
	Total int
	// Positions is empty for a spawn placed randomly in the maker's
	// territories, one fixed position, or several weighted ones.
	Positions []Position
	// Respawn is the respawn delay: 0 when absent, NoRespawn for "no".
	Respawn       time.Duration
	RespawnRandom time.Duration
	// Privates are the entry's own <privates><private> rows in file order,
	// which replace its template's NPC.Privates; nil for none.
	Privates []Private
}

// Position is one pos="x;y;z;heading[;chance%]" tuple.
type Position struct {
	X, Y, Z, Heading int32
	// Chance is the weight in percent of a weighted position; 0 for a
	// fixed one.
	Chance int
}

// Spawnlist is every territory and maker of the spawnlist files.
type Spawnlist struct {
	// Territories lists every built territory in file and declaration
	// order, same-name duplicates included.
	Territories []*Territory
	// Makers lists every maker in file and declaration order.
	Makers []*Maker
	// Warnings describes what the server would also skip with a warning:
	// territories it cannot build and territory names a maker refers to
	// that do not exist. The caller logs them.
	Warnings []string
}

type spawnlistFile struct {
	Territories []territoryElement `xml:"territory"`
	Makers      []makerElement     `xml:"npcmaker"`
}

type territoryElement struct {
	Attrs []xml.Attr     `xml:",any,attr"`
	Nodes []attrsElement `xml:"node"`
}

// makerElement is one <npcmaker>: its attributes, its <ai> blocks and its
// <npc> entries.
type makerElement struct {
	Attrs []xml.Attr     `xml:",any,attr"`
	AI    []aiElement    `xml:"ai"`
	NPCs  []entryElement `xml:"npc"`
}

// aiElement is one <ai>: its type attribute and its <set> parameters.
type aiElement struct {
	Attrs []xml.Attr   `xml:",any,attr"`
	Sets  []setElement `xml:"set"`
}

// entryElement is one <npc> entry of a maker: its attributes and its
// <privates><private> rows (repeated <privates> blocks merge). Its <ai> is
// not read.
type entryElement struct {
	Attrs    []xml.Attr     `xml:",any,attr"`
	Privates []attrsElement `xml:"privates>private"`
}

// attrsElement is an element read only for its attributes.
type attrsElement struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

// LoadSpawnlist reads every *.xml spawnlist file directly under dir, the way
// the server's LoadSpawnlist does: territories of all files first, then the
// makers, which refer to territories by case-insensitive name across files.
// A file that is not well-formed or a missing or mangled field bots read
// fails the whole load; what the server skips with a warning is skipped and
// recorded in Spawnlist.Warnings.
func LoadSpawnlist(dir string) (*Spawnlist, error) {
	paths, err := xmlFiles(dir, "spawnlist")
	if err != nil {
		return nil, err
	}
	files := make([]spawnlistFile, len(paths))
	for i, path := range paths {
		if err := decodeFile(path, &files[i]); err != nil {
			return nil, err
		}
	}
	return buildSpawnlist(paths, files)
}

func buildSpawnlist(paths []string, files []spawnlistFile) (*Spawnlist, error) {
	sl := &Spawnlist{}
	byName := make(map[string]*Territory)
	for i, f := range files {
		for _, el := range f.Territories {
			t, skip, err := buildTerritory(el)
			if err != nil {
				return nil, fmt.Errorf("datapack: %s: %w", paths[i], err)
			}
			if skip != "" {
				sl.Warnings = append(sl.Warnings, fmt.Sprintf("%s: skipping %s", paths[i], skip))
				continue
			}
			// Same-name territories are all kept; a name resolves to the
			// first declared, as on the server.
			key := strings.ToLower(t.Name)
			if _, ok := byName[key]; !ok {
				byName[key] = t
			}
			sl.Territories = append(sl.Territories, t)
		}
	}
	for i, f := range files {
		for _, el := range f.Makers {
			m, err := buildMaker(el, byName, &sl.Warnings)
			if err != nil {
				return nil, fmt.Errorf("datapack: %s: %w", paths[i], err)
			}
			sl.Makers = append(sl.Makers, m)
		}
	}
	return sl, nil
}

// buildTerritory reads one <territory>. A territory with fewer than 3 nodes
// or maxZ below minZ is one the server cannot build: it returns a non-empty
// skip reason instead of an error.
func buildTerritory(el territoryElement) (t *Territory, skip string, err error) {
	nodes := make([]Point, 0, len(el.Nodes))
	for _, n := range el.Nodes {
		a := foldAttrs(n.Attrs)
		p := Point{X: a.literal("x"), Y: a.literal("y")}
		if a.err != nil {
			return nil, "", fmt.Errorf("territory node: %w", a.err)
		}
		nodes = append(nodes, p)
	}
	a := foldAttrs(el.Attrs)
	t = &Territory{Name: a.str("name")}
	t.MinZ = a.literal("minZ")
	t.MaxZ = a.literal("maxZ")
	if a.err != nil {
		return nil, "", fmt.Errorf("territory %q: %w", t.Name, a.err)
	}
	t.Nodes = nodes
	switch {
	case len(nodes) < 3:
		return nil, fmt.Sprintf("territory %q: needs at least 3 nodes", t.Name), nil
	case t.MaxZ < t.MinZ:
		return nil, fmt.Sprintf("territory %q: maxZ must be >= minZ", t.Name), nil
	}
	return t, "", nil
}

func buildMaker(el makerElement, territories map[string]*Territory, warnings *[]string) (*Maker, error) {
	a := foldAttrs(el.Attrs)
	logName := a.strDefault("name", "?")
	refs := resolveTerritories(logName, a.strDefault("territory", ""), territories, warnings)
	banned := resolveTerritories(logName, a.strDefault("ban", ""), territories, warnings)

	spawns := make([]Spawn, 0, len(el.NPCs))
	for _, npcEl := range el.NPCs {
		s, err := buildSpawn(npcEl)
		if err != nil {
			return nil, fmt.Errorf("maker %q: %w", logName, err)
		}
		spawns = append(spawns, s)
	}
	aiType, params := flattenMakerAI(el.AI)
	if aiType == "" {
		aiType = a.strDefault("maker", "")
	}
	m := &Maker{
		Name:        a.str("name"),
		Territories: refs,
		Banned:      banned,
		MaximumNPCs: a.int("maximumNpcs"),
		Event:       a.strDefault("event", ""),
		SpawnTime:   a.strDefault("spawnTime", ""),
		AIType:      aiType,
		Params:      params,
		Spawns:      spawns,
	}
	if a.err != nil {
		return nil, fmt.Errorf("maker %q: %w", logName, a.err)
	}
	return m, nil
}

// flattenMakerAI merges a maker's <ai> blocks the way the server's
// flattenAI does for a maker: the last type named wins, a later value of a
// parameter replaces an earlier one and "@" is removed from every value. It
// keeps only the makerParams; nil params for none.
func flattenMakerAI(ai []aiElement) (kind string, params map[string]string) {
	for _, el := range ai {
		if t, ok := foldAttrs(el.Attrs).vals["type"]; ok {
			kind = t
		}
		for _, s := range el.Sets {
			if !makerParams[s.Name] {
				continue
			}
			if params == nil {
				params = make(map[string]string)
			}
			params[s.Name] = strings.ReplaceAll(s.Val, "@", "")
		}
	}
	return kind, params
}

func buildSpawn(el entryElement) (Spawn, error) {
	a := foldAttrs(el.Attrs)
	id := a.strDefault("id", "?")
	positions, err := parsePositions(a.strDefault("pos", ""))
	if err != nil {
		return Spawn{}, fmt.Errorf("npc %s: %w", id, err)
	}
	s := Spawn{
		NPCID:         a.literal("id"),
		Total:         int(a.literal("total")),
		Positions:     positions,
		Respawn:       a.duration("respawn"),
		RespawnRandom: a.duration("respawnRand"),
	}
	if a.err != nil {
		return Spawn{}, fmt.Errorf("npc %s: %w", id, a.err)
	}
	for _, p := range el.Privates {
		private, err := buildSpawnPrivate(p)
		if err != nil {
			return Spawn{}, fmt.Errorf("npc %s: %w", id, err)
		}
		s.Privates = append(s.Privates, private)
	}
	return s, nil
}

// buildSpawnPrivate mirrors the server's buildSpawnPrivate
// (data/xml/spawnlist.go): an entry's <private> needs its id and weight; its
// respawn defaults to 0.
func buildSpawnPrivate(el attrsElement) (Private, error) {
	a := foldAttrs(el.Attrs)
	p := Private{NPCID: a.int32("id"), Weight: a.int("weight"), Respawn: a.duration("respawn")}
	if a.err != nil {
		return Private{}, fmt.Errorf("private: %w", a.err)
	}
	return p, nil
}

// resolveTerritories looks up each ";"-separated name in raw the way the
// server does: an unknown single name resolves to nothing, and one unknown
// name in a multi-name list drops the whole list. Each unknown name adds a
// warning. Nothing resolved is nil.
func resolveTerritories(makerName, raw string, territories map[string]*Territory, warnings *[]string) []*Territory {
	var names []string
	for name := range strings.SplitSeq(raw, ";") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	resolved := make([]*Territory, 0, len(names))
	for _, name := range names {
		t, ok := territories[strings.ToLower(name)]
		if !ok {
			*warnings = append(*warnings, fmt.Sprintf("maker %q: territory %q does not exist", makerName, name))
			if len(names) > 1 {
				return nil
			}
			continue
		}
		resolved = append(resolved, t)
	}
	if len(resolved) == 0 {
		return nil
	}
	return resolved
}

// parsePositions decodes a pos attribute the way the server does
// (acis_golang internal/gameserver/model/spawn/entry.go ParsePositions):
// trailing ";" are trimmed, 4 fields are one fixed x;y;z;heading tuple, and
// 5 or more are floor(n/5) weighted x;y;z;heading;chance% tuples, leftover
// fields ignored.
func parsePositions(raw string) ([]Position, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), ";")
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ";")
	if len(parts) == 4 {
		p, err := parsePosition(parts, false)
		if err != nil {
			return nil, err
		}
		return []Position{p}, nil
	}
	if len(parts) < 5 {
		return nil, fmt.Errorf("malformed pos %q", raw)
	}
	positions := make([]Position, 0, len(parts)/5)
	for i := 0; i+5 <= len(parts); i += 5 {
		p, err := parsePosition(parts[i:i+5], true)
		if err != nil {
			return nil, err
		}
		positions = append(positions, p)
	}
	return positions, nil
}

func parsePosition(parts []string, weighted bool) (Position, error) {
	var nums [4]int32
	for i, name := range []string{"x", "y", "z", "heading"} {
		n, err := strconv.ParseInt(parts[i], 10, 32)
		if err != nil {
			return Position{}, fmt.Errorf("pos %s: %w", name, err)
		}
		nums[i] = int32(n)
	}
	p := Position{X: nums[0], Y: nums[1], Z: nums[2], Heading: nums[3]}
	if weighted {
		n, err := strconv.Atoi(strings.TrimSuffix(parts[4], "%"))
		if err != nil {
			return Position{}, fmt.Errorf("pos chance: %w", err)
		}
		p.Chance = n
	}
	return p, nil
}
