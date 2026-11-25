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
	w, err := NewWorld(Self{ObjectID: selfID, Name: "bot7"}, t0)
	if err != nil {
		tb.Fatal(err)
	}
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
// inputs (#494), and MaxHP changes, which rebuild it every time.
func BenchmarkGearValuer(bm *testing.B) {
	candidates := []int32{lightTop, lightBottom, demonChest, demonLegs, doomChest, doomLegs}
	for _, bc := range []struct {
		name string
		ev   func(i int) Event
	}{
		{"own-hp", func(i int) Event { return StatusUpdate{ObjectID: selfID, Set: StatusHP, HP: int32(1 + i%20)} }},
		{"max-hp", func(i int) Event { return StatusUpdate{ObjectID: selfID, Set: StatusMaxHP, MaxHP: int32(20 + i%2)} }},
	} {
		bm.Run(bc.name, func(bm *testing.B) {
			b := benchModelBrain(bm)
			if b.valuer() == nil {
				bm.Fatal("no gear model")
			}
			gear := b.cfg.Travel.gear
			bm.ReportAllocs()
			for i := 0; bm.Loop(); i++ {
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
