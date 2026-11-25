package botclient

import (
	"slices"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/geo"
)

// SupplyConfig is what the supply ceiling of every priced template
// (TravelMap.supply, docs/valuables/score.md "Supply ceiling") is computed
// with. It shapes the one travel map all bots of a process share, so it is a
// process setting, not a persona's (docs/valuables/preset.md "Process
// settings").
type SupplyConfig struct {
	// SpawnEvents are the server's SpawnEvents (npcs.properties), which the
	// bot cannot read: the events whose makers run.
	SpawnEvents []string
	// KillFloor is the time to find and kill a spawn once it is back.
	KillFloor time.Duration
	// NoRespawnCycle is how often a spawn that never respawns (respawn 0 or
	// "no") counts as killed.
	NoRespawnCycle time.Duration
	// SpoilWeight is the share of a SPOIL category's drops that counts:
	// only a spoiler rolls it.
	SpoilWeight float64
	// SevenSignsShare is the share of the time the Seven Signs makers
	// (ssq_event, ssq_seal1_none, ssq_seal2_none) run: one week in two.
	SevenSignsShare float64
}

// DefaultSupplyConfig is the SupplyConfig with no flag set: the shipped
// SpawnEvents (the server's DefaultSpawnEvents, data/manager/npcs_events.go)
// and the design's defaults.
func DefaultSupplyConfig() SupplyConfig {
	return SupplyConfig{
		SpawnEvents:     []string{"extra_mob", "18age", "start_weapon"},
		KillFloor:       60 * time.Second,
		NoRespawnCycle:  24 * time.Hour,
		SpoilWeight:     0.2,
		SevenSignsShare: 0.5,
	}
}

// itemSupply is what the travel map knows of a priced template's supply.
type itemSupply struct {
	// perHour is the supply ceiling S: items a server-wide hour drops if
	// every spawn were killed at respawn; 0 for no drop source.
	perHour float64
	// multisell marks a template a spawned NPC's multisell row makes from
	// no weapon or armor; craftable a recipe product.
	multisell, craftable bool
	// scroll is the normal enchant scroll of its grade and kind
	// (normalScrolls), 0 for none: what an enchant level adds to its worth.
	scroll int32
	// safe is its last safe enchant level: enchantSafeMaxFull for a full
	// body armor, else enchantSafeMax.
	safe int
}

// Maker AI types the supply ceiling reads (spawnlist <ai type>, else the
// maker attribute).
const (
	makerNoOnStart          = "no_on_start_maker"
	makerRandomSpawn        = "random_spawn"
	makerRandomSpawnPawn    = "random_spawn_pawn"
	makerDayNight           = "on_day_night_spawn"
	makerInstantSpawn       = "maker_instant_spawn"
	makerInstantSpawnRandom = "maker_instant_spawn_random"
)

// sevenSignsEvents are the events of the makers that run while the Seven
// Signs competition or a seal with no owner does: one week in two.
var sevenSignsEvents = map[string]bool{"ssq_event": true, "ssq_seal1_none": true, "ssq_seal2_none": true}

// linkedMakerParams name the makers of an instant random maker's linked set.
var linkedMakerParams = []string{"maker_name1", "maker_name2", "maker_name3"}

// normalScrolls are the plain enchant scrolls of enchantScrolls (not
// blessed, not crystal), by kind and grade: the price of one enchant level.
var normalScrolls = map[enchantScroll]int32{
	{weapon: true, grade: scrollGradeD}: 955, {weapon: true, grade: scrollGradeC}: 951,
	{weapon: true, grade: scrollGradeB}: 947, {weapon: true, grade: scrollGradeA}: 729,
	{weapon: true, grade: scrollGradeS}: 959,
	{grade: scrollGradeD}:               956, {grade: scrollGradeC}: 952, {grade: scrollGradeB}: 948,
	{grade: scrollGradeA}: 730, {grade: scrollGradeS}: 960,
}

// indexSupply builds the supply of every template dp prices: the supply
// ceiling summed over every maker, every entry of a template with drops and
// every private of an entry, the marks multisell (a row of a multisell an NPC
// with a fixed spawn opens) and craftable, and the enchant scroll and safe
// level of a graded weapon or armor (docs/valuables/score.md).
func indexSupply(dp *datapack.Datapack, spawns map[int32][]Point, cfg SupplyConfig) map[int32]itemSupply {
	out := make(map[int32]itemSupply, len(dp.Prices))
	for id := range dp.Prices {
		s := itemSupply{safe: enchantSafeMax}
		if it, ok := dp.Items[id]; ok && (it.Type == "Weapon" || it.Type == "Armor") {
			s.scroll = normalScrolls[enchantScroll{weapon: it.Type == "Weapon", grade: int32(it.Grade)}]
			if it.BodyPart == "fullarmor" {
				s.safe = enchantSafeMaxFull
			}
		}
		out[id] = s
	}
	for id, perHour := range supplyPerHour(dp, cfg) {
		if s, ok := out[id]; ok {
			s.perHour = perHour
			out[id] = s
		}
	}
	for _, ms := range dp.Multisells {
		if !slices.ContainsFunc(ms.NPCs, func(npc int32) bool { return len(spawns[npc]) > 0 }) {
			continue
		}
		for _, row := range ms.Rows {
			if slices.ContainsFunc(row.Ingredients, func(in datapack.MultisellItem) bool {
				t := dp.Items[in.ItemID].Type
				return t == "Weapon" || t == "Armor"
			}) {
				continue
			}
			for _, p := range row.Products {
				if s, ok := out[p.ItemID]; ok {
					s.multisell = true
					out[p.ItemID] = s
				}
			}
		}
	}
	for _, r := range dp.Recipes {
		if s, ok := out[r.Product.ItemID]; ok {
			s.craftable = true
			out[r.Product.ItemID] = s
		}
	}
	return out
}

// supplyPerHour sums S, items per hour, of every item dropped by a spawn:
// share × total × 3600 / cycle × p × mean over every maker, entry and private
// (docs/valuables/score.md "Supply ceiling").
func supplyPerHour(dp *datapack.Datapack, cfg SupplyConfig) map[int32]float64 {
	out := make(map[int32]float64)
	byName := make(map[string]*datapack.Maker, len(dp.Spawnlist.Makers))
	linked := make(map[string]bool)
	for _, mk := range dp.Spawnlist.Makers {
		if _, ok := byName[mk.Name]; !ok {
			byName[mk.Name] = mk
		}
		if mk.AIType == makerInstantSpawnRandom {
			linked[mk.Name] = true
			for _, k := range linkedMakerParams {
				if n := mk.Params[k]; n != "" {
					linked[n] = true
				}
			}
		}
	}
	for _, mk := range dp.Spawnlist.Makers {
		share := makerShare(mk, byName, linked, cfg)
		if share <= 0 {
			continue
		}
		n := 0
		for _, sp := range mk.Spawns {
			n += sp.Total
		}
		for _, sp := range mk.Spawns {
			cycle := entryCycle(mk, sp, n, cfg)
			addDrops(out, dp.NPCs[sp.NPCID], share*float64(sp.Total), cycle, cfg)
			privates := sp.Privates
			if len(privates) == 0 {
				privates = dp.NPCs[sp.NPCID].Privates
			}
			for _, pr := range privates {
				c := cycle
				if own := pr.Respawn + cfg.KillFloor; pr.Respawn > 0 && own < c {
					c = own
				}
				addDrops(out, dp.NPCs[pr.NPCID], share*float64(sp.Total*pr.Weight), c, cfg)
			}
		}
	}
	return out
}

// addDrops adds to out what count NPCs of npc killed every cycle drop in an
// hour: each category's chance times each drop's times its mean quantity,
// DROP fully, SPOIL by cfg.SpoilWeight, HERB and CURRENCY not at all.
func addDrops(out map[int32]float64, npc datapack.NPC, count float64, cycle time.Duration, cfg SupplyConfig) {
	if count <= 0 || cycle <= 0 {
		return
	}
	perHour := count * float64(time.Hour) / float64(cycle)
	for _, cat := range npc.Drops {
		weight := 0.0
		switch cat.Kind {
		case datapack.DropKindDrop:
			weight = 1
		case datapack.DropKindSpoil:
			weight = cfg.SpoilWeight
		}
		if weight == 0 {
			continue
		}
		for _, d := range cat.Drops {
			p := cat.Chance * d.Chance / 10000
			out[d.ItemID] += perHour * weight * p * float64(d.Min+d.Max) / 2
		}
	}
}

// makerShare is the share of the time mk's NPCs are up: its event's, its
// start's and its type's (docs/valuables/score.md "Share").
func makerShare(mk *datapack.Maker, byName map[string]*datapack.Maker, linked map[string]bool, cfg SupplyConfig) float64 {
	share := 1.0
	ev := makerEvent(mk)
	switch {
	case ev == "" || slices.Contains(cfg.SpawnEvents, ev):
	case sevenSignsEvents[ev]:
		share = cfg.SevenSignsShare
	default:
		return 0
	}
	if mk.AIType == makerNoOnStart {
		return 0
	}
	if ev == "" && (mk.SpawnTime != "" || offAtStart(mk) && !linked[mk.Name]) {
		return 0
	}
	switch mk.AIType {
	case makerRandomSpawn, makerRandomSpawnPawn:
		if len(mk.Spawns) > 0 {
			share /= float64(len(mk.Spawns))
		}
	case makerDayNight:
		if mk.Params["IsNight"] == "1" {
			share *= 0.25
		} else {
			share *= 0.75
		}
	case makerInstantSpawnRandom:
		set := []string{mk.Name}
		for _, k := range linkedMakerParams {
			if n := mk.Params[k]; n != "" && byName[n] != nil && !slices.Contains(set, n) {
				set = append(set, n)
			}
		}
		starters := 0
		for _, n := range set {
			if startsAtBoot(byName[n]) {
				starters++
			}
		}
		share *= float64(starters) / float64(len(set))
	}
	return share
}

// makerEvent is the event that switches mk on: its event attribute, else its
// EventName parameter (the server's Maker.EventName); empty for none.
func makerEvent(mk *datapack.Maker) string {
	if mk.Event != "" {
		return mk.Event
	}
	return mk.Params["EventName"]
}

// offAtStart reports whether mk's on_start_spawn parameter is 0.
func offAtStart(mk *datapack.Maker) bool {
	v, ok := mk.Params["on_start_spawn"]
	if !ok {
		return false
	}
	n, err := strconv.Atoi(v)
	return err == nil && n == 0
}

// startsAtBoot reports whether mk starts at boot, as the server's
// Maker.OnStart (model/spawn/group.go): no event attribute, no spawn time
// and on_start_spawn not 0.
func startsAtBoot(mk *datapack.Maker) bool {
	return mk != nil && mk.Event == "" && mk.SpawnTime == "" && !offAtStart(mk)
}

// entryCycle is how long one NPC of entry sp of mk takes to come back once
// killed: an instant group whole every KillFloor × n (n the group's NPCs),
// an instant random group after its respawn on top, else its respawn and
// KillFloor, or NoRespawnCycle for a spawn that does not respawn.
func entryCycle(mk *datapack.Maker, sp datapack.Spawn, n int, cfg SupplyConfig) time.Duration {
	switch mk.AIType {
	case makerInstantSpawn:
		return cfg.KillFloor * time.Duration(n)
	case makerInstantSpawnRandom:
		return max(sp.Respawn, 0) + cfg.KillFloor*time.Duration(n)
	}
	if sp.Respawn > 0 {
		return sp.Respawn + cfg.KillFloor
	}
	return cfg.NoRespawnCycle
}

// indexRegions maps each map tile of the restart points to the index of the
// first point listing it, as the server's Table.PointIndexAt scans them.
func indexRegions(points []datapack.RestartPoint) map[datapack.MapTile]int {
	out := make(map[datapack.MapTile]int)
	for i, p := range points {
		for _, t := range p.Tiles {
			if _, ok := out[t]; !ok {
				out[t] = i
			}
		}
	}
	return out
}

// restartRegion is the index of the restart point whose map tiles hold
// (x, y), -1 for none: the server's mapRegion and PointIndexAt
// (model/restart/restart.go), copied with the bot's geo bounds.
func (m *TravelMap) restartRegion(x, y int32) int {
	t := datapack.MapTile{
		X: int((x-geo.WorldXMin)/geo.TileSize + geo.TileXMin),
		Y: int((y-geo.WorldYMin)/geo.TileSize + geo.TileYMin),
	}
	if i, ok := m.regions[t]; ok {
		return i
	}
	return -1
}
