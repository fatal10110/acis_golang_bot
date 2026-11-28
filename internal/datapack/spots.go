package datapack

import (
	"math"
	"strings"
)

// Spot is a farming spot: the territory of one spawnlist maker and the
// levels of the monsters it spawns there.
type Spot struct {
	// Maker is the maker's name.
	Maker string
	// Event is the Seven Signs event that switches the maker on
	// (SevenSignsEvent), empty for a maker that no event switches.
	Event string
	// Territory is the maker's first territory; Center is a point inside it
	// near its middle and Z the middle of its height band.
	Territory *Territory
	Center    Point
	Z         int32
	// MinLevel and MaxLevel bound the levels of its monsters.
	MinLevel, MaxLevel int
	// AggroLevel is the highest level of an aggressive monster (AggroRange
	// above 0) among them, 0 when none is aggressive.
	AggroLevel int
	// MaximumNPCs is the maker's declared cap, before the server's spawn
	// multiplier.
	MaximumNPCs int
	// Area is the territory's area on the X/Y plane.
	Area float64
	// Aggressive is the number of aggressive monsters the maker keeps, its
	// declared totals capped at MaximumNPCs; AggroRange, ClanRange and
	// RunSpeed are the largest aggro range, clan call range and run speed
	// among them.
	Aggressive, AggroRange, ClanRange int
	RunSpeed                          float64
	// Mobs are its aggressive monsters, one per spawn: what a bot weighs a
	// fight there with.
	Mobs []Mob
	// Monsters are all its monsters, aggressive or not, one per spawn: what
	// a bot weighs the spot's earnings with (#527).
	Monsters []Mob
}

// Mob is one kind of monster a spot spawns: its template, its level, its
// combat stats and how many of it the spawn declares.
type Mob struct {
	NPCID        int32
	Level, Count int
	Combat       Combat
}

// Spots lists the makers of d fit for farming, in spawnlist order. It
// leaves out a maker that an event other than a Seven Signs dungeon event
// switches on (event=, SevenSignsEvent), that carves areas
// out of its territories (ban=), that has no territory, or that spawns a
// raid or grand boss, or that likely spawns quest mobs (questMaker); of the
// rest it counts only the monsters (Type "Monster") that are not likely
// quest mobs (questMob), and keeps a maker with at least one.
//
// Whether a spot fits a given bot (its level against the monsters', the
// aggressive ones above it) is the caller's choice: Spot carries the levels.
func (d *Datapack) Spots() []Spot {
	var spots []Spot
	for _, m := range d.Spawnlist.Makers {
		if s, ok := d.spot(m); ok {
			spots = append(spots, s)
		}
	}
	return spots
}

// spot is m as a farming spot, if it is one (see Spots).
func (d *Datapack) spot(m *Maker) (Spot, bool) {
	if (m.Event != "" && !SevenSignsEvent(m.Event)) || len(m.Banned) > 0 || len(m.Territories) == 0 || len(m.Territories[0].Nodes) < 3 || questMaker(m) {
		return Spot{}, false
	}
	t := m.Territories[0]
	s := Spot{Maker: m.Name, Event: m.Event, Territory: t, Center: t.inner(), Z: t.MinZ + (t.MaxZ-t.MinZ)/2, MaximumNPCs: m.MaximumNPCs, Area: t.area()}
	monsters := 0
	for _, sp := range m.Spawns {
		npc, ok := d.NPCs[sp.NPCID]
		if !ok {
			continue
		}
		switch npc.Type {
		case "RaidBoss", "GrandBoss":
			return Spot{}, false
		case "Monster":
		default:
			continue
		}
		if questMob(npc, sp) {
			continue
		}
		if monsters == 0 || npc.Level < s.MinLevel {
			s.MinLevel = npc.Level
		}
		s.MaxLevel = max(s.MaxLevel, npc.Level)
		mob := Mob{NPCID: sp.NPCID, Level: npc.Level, Count: sp.Total, Combat: npc.Combat}
		if sp.Total > 0 {
			s.Monsters = append(s.Monsters, mob)
		}
		if npc.AggroRange > 0 {
			s.AggroLevel = max(s.AggroLevel, npc.Level)
			s.Aggressive += sp.Total
			s.AggroRange = max(s.AggroRange, npc.AggroRange)
			s.ClanRange = max(s.ClanRange, npc.ClanRange)
			s.RunSpeed = max(s.RunSpeed, npc.RunSpeed)
			if sp.Total > 0 {
				s.Mobs = append(s.Mobs, mob)
			}
		}
		monsters++
	}
	if m.MaximumNPCs > 0 {
		s.Aggressive = min(s.Aggressive, m.MaximumNPCs)
	}
	return s, monsters > 0
}

// SevenSignsEvent reports whether event switches on the monsters of a
// Seven Signs dungeon, a catacomb or a necropolis: ssq_event during
// recruitment and the competition, and during seal validation
// ssq_seal1_<owner> in the necropolises (the Seal of Avarice) and
// ssq_seal2_<owner> in the catacombs (the Seal of Gnosis), by the seal's
// owner: dawn, twilight (Dusk) or none. Only a Seven Signs member enters
// those dungeons (server sevensigns/dungeon.go), so the caller keeps
// their spots for one.
func SevenSignsEvent(event string) bool { return strings.HasPrefix(event, "ssq_") }

// questMaker reports whether a maker likely spawns quest mobs: the
// spawnlist names quest monster makers <area>_qm<region>_<n> (the tutorial
// gremlins of gludio32_qm1725_00m1, for one).
func questMaker(m *Maker) bool {
	return strings.Contains(m.Name, "_qm")
}

// questMob reports whether a monster spawn is likely a quest mob. The
// datapack has no marker for one, so it is a heuristic: a template in the
// 27000-27999 block (the quest monster block of the NPC tables) or a spawn
// that never respawns (respawn="no", spawned once for a scene).
func questMob(npc NPC, sp Spawn) bool {
	return (npc.ID >= 27000 && npc.ID <= 27999) || sp.Respawn == NoRespawn
}

// Contains reports whether p lies inside the territory's polygon on the
// X/Y plane (even-odd rule).
func (t *Territory) Contains(p Point) bool {
	in := false
	n := len(t.Nodes)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		a, b := t.Nodes[i], t.Nodes[j]
		if (a.Y > p.Y) == (b.Y > p.Y) {
			continue
		}
		// X of the edge at p.Y.
		x := float64(a.X) + float64(p.Y-a.Y)*float64(b.X-a.X)/float64(b.Y-a.Y)
		if float64(p.X) < x {
			in = !in
		}
	}
	return in
}

// area is the territory's area on the X/Y plane (shoelace formula).
func (t *Territory) area() float64 {
	var a int64
	n := len(t.Nodes)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		a += int64(t.Nodes[j].X)*int64(t.Nodes[i].Y) - int64(t.Nodes[i].X)*int64(t.Nodes[j].Y)
	}
	return math.Abs(float64(a)) / 2
}

// innerGrid is the number of grid cells per side inner scans.
const innerGrid = 16

// inner is a point inside the polygon near the mean of its nodes: the mean
// itself when inside (any convex polygon), else the nearest cell centre of
// an innerGrid grid over the bounding box that is inside, else the first
// node.
func (t *Territory) inner() Point {
	var sx, sy int64
	lo := Point{X: math.MaxInt32, Y: math.MaxInt32}
	hi := Point{X: math.MinInt32, Y: math.MinInt32}
	for _, n := range t.Nodes {
		sx, sy = sx+int64(n.X), sy+int64(n.Y)
		lo.X, lo.Y = min(lo.X, n.X), min(lo.Y, n.Y)
		hi.X, hi.Y = max(hi.X, n.X), max(hi.Y, n.Y)
	}
	mean := Point{X: int32(sx / int64(len(t.Nodes))), Y: int32(sy / int64(len(t.Nodes)))}
	if t.Contains(mean) {
		return mean
	}
	best, bestD := t.Nodes[0], int64(-1)
	w, h := int64(hi.X-lo.X), int64(hi.Y-lo.Y)
	for i := range int64(innerGrid) {
		for j := range int64(innerGrid) {
			p := Point{
				X: lo.X + int32((2*i+1)*w/(2*innerGrid)),
				Y: lo.Y + int32((2*j+1)*h/(2*innerGrid)),
			}
			if !t.Contains(p) {
				continue
			}
			dx, dy := int64(p.X-mean.X), int64(p.Y-mean.Y)
			if d := dx*dx + dy*dy; bestD < 0 || d < bestD {
				best, bestD = p, d
			}
		}
	}
	return best
}
