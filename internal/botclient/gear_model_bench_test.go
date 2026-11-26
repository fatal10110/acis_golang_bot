package botclient

import (
	"math/rand/v2"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// benchModelBrain is newCacheBrain's bot for a benchmark: a level 20 Human
// Mystic support knowing cacheSkills, wearing robe top and legs, its gear
// valued by the model on the model tests' travel map.
func benchModelBrain(tb testing.TB) *Brain {
	tb.Helper()
	w, _ := NewWorld(Self{ObjectID: selfID, Name: "bot7"}, t0)
	p := DefaultPersona()
	p.IdleChance, p.AutoShots, p.Role = 0, false, RoleSupport
	p.Gear.Classes = nil
	b := NewBrain(w, NewEvents(64), &recordSender{}, newFakeClock(t0), BrainConfig{Persona: p, Rand: rand.New(rand.NewPCG(1, 2))})
	b.world.Apply(cacheUserInfo(20), t0)
	b.world.Apply(SkillList{Skills: cacheSkills}, t0)
	b.world.Apply(ItemList{Items: []Item{wornItem(301, robeTop), wornItem(302, robeBottom)}}, t0)
	shops, cheapest := indexShops(nil)
	gear, shopGear := indexGear(modelItems, cheapest)
	sets, pieces := indexSets(modelSets, gear)
	b.cfg.Travel = &TravelMap{
		shops: shops, cheapest: cheapest, gear: gear, shopGear: shopGear,
		sets: sets, setPieces: pieces, classes: modelClasses, skills: modelSkills, setSkills: indexSetSkills(modelSets),
		refMobs: map[int]charmodel.Profile{20: modelTarget}, hennas: map[int32]datapack.Henna{},
	}
	return b
}

// BenchmarkGearValuer scores the model tests' armor against what the bot
// wears (pieceScore, as the gear, shop and shots rules do) after each of a
// stream of events: own HP updates, which change none of the model's
// inputs (#494), and MaxHP changes, which rebuild it every time. The pvp
// cases give PvP value a weight (PvPPolicy.GearWeight): own HP updates with
// nobody seen and with one player seen, which keep the PvP side, and own HP
// updates each after the one player seen changed class, which make it
// again every time.
func BenchmarkGearValuer(bm *testing.B) {
	candidates := []int32{lightTop, lightBottom, demonChest, demonLegs, doomChest, doomLegs}
	ownHP := func(i int) Event { return StatusUpdate{ObjectID: selfID, Set: StatusHP, HP: int32(1 + i%20)} }
	foe := func(i int) Event {
		return CharInfo{ObjectID: 0x1000, Name: "Foe", ClassID: int32(i%2) * classHumanMystic, Pos: Point{Y: 900}}
	}
	for _, bc := range []struct {
		name   string
		weight float64
		seen   func(i int) Event // a player seen before ev, or nil
		ev     func(i int) Event
	}{
		{name: "own-hp", ev: ownHP},
		{name: "max-hp", ev: func(i int) Event { return StatusUpdate{ObjectID: selfID, Set: StatusMaxHP, MaxHP: int32(20 + i%2)} }},
		{name: "pvp-own-hp", weight: 0.5, ev: ownHP},
		{name: "pvp-seen-own-hp", weight: 0.5, seen: func(int) Event { return foe(1) }, ev: ownHP},
		{name: "pvp-mix-change", weight: 0.5, seen: foe, ev: ownHP},
	} {
		bm.Run(bc.name, func(bm *testing.B) {
			b := benchModelBrain(bm)
			b.persona.PvP.GearWeight = bc.weight
			if b.valuer() == nil {
				bm.Fatal("no gear model")
			}
			gear := b.cfg.Travel.gear
			bm.ReportAllocs()
			for i := 0; bm.Loop(); i++ {
				if bc.seen != nil {
					ev := bc.seen(i)
					b.world.Apply(ev, t0)
					b.pvpEvent(ev, t0)
				}
				b.world.Apply(bc.ev(i), t0)
				w := b.rawWorn()
				for k, id := range candidates {
					g := gear[id]
					b.pieceScore(&w, &g, carriedPiece(int32(400+k), id))
				}
			}
		})
	}
}
