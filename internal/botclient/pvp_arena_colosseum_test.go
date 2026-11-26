package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The arena pick tests' arenas on the split navigation, the bot standing at
// gateFrom: nearArena, 500 deep, on the bot's side in walking reach;
// deepGated, 1500 deep, across the wall, its middle about 3270 from the
// gatekeeper's landing at travelDest (past storeTownRadius, within
// arenaLanding); deepOnFoot, 2000 deep, on the bot's side.
var (
	nearArena  = box(4500, 0, 5500, 1000)
	deepGated  = box(9000, 1400, 12000, 4400)
	deepOnFoot = box(1000, 2000, 5000, 6000)
)

// TestPvPArenaColosseum checks the arena an arena trip picks per row
// (#422): the OM row, a mystic row with no spiritshot charged and a bow
// branch go to the deepest arena (the Colosseum) when a gatekeeper trip
// there plans or a walk reaches it; every other row, and those rows when
// neither does, go to the nearest arena.
func TestPvPArenaColosseum(t *testing.T) {
	spiritshot := func(b *Brain) {
		b.world.Inventory.Items[2] = Item{ObjectID: 2, TemplateID: 3947, Count: 100, SubCategory: 5}
		b.world.Inventory.AutoShots = map[int32]bool{3947: true}
		b.persona.AutoShots = false
	}
	bow := func(b *Brain) {
		b.cfg.Travel.arrows = map[int32]int32{pvpBow: 17}
		b.world.Inventory.Items[3] = Item{ObjectID: 3, TemplateID: pvpBow, Equipped: true}
	}
	tests := []struct {
		name   string
		class  int32
		adena  int32
		arenas []datapack.Zone
		edit   func(b *Brain)
		want   int
		gate   bool
	}{
		{name: "HF row: nearest", class: 0, adena: 5000, want: 1},
		{name: "OM row: deepest by gatekeeper", class: 49, adena: 5000, want: 2, gate: true},
		{name: "OM row with spiritshots: deepest", class: 49, adena: 5000, edit: spiritshot, want: 2, gate: true},
		{name: "HM row, no spiritshot: deepest", class: 10, adena: 5000, want: 2, gate: true},
		{name: "HM row with spiritshots: nearest", class: 10, adena: 5000, edit: spiritshot, want: 1},
		{name: "CL row, no spiritshot: deepest", class: 15, adena: 5000, want: 2, gate: true},
		{name: "OS row, no spiritshot: deepest", class: 50, adena: 5000, want: 2, gate: true},
		{name: "RO row with a bow: deepest", class: 7, adena: 5000, edit: bow, want: 2, gate: true},
		{name: "RO row with a dagger: nearest", class: 7, adena: 5000, want: 1},
		{name: "OM row, no adena for the gatekeeper: nearest", class: 49, want: 1},
		{
			name: "OM row within Travel.Gap of its last trip: nearest", class: 49, adena: 5000, want: 1,
			edit: func(b *Brain) { b.trip.endedAt, b.persona.Travel.Gap = t0, 3*time.Hour },
		},
		{name: "OM row, deepest in walking reach: deepest on foot", class: 49, arenas: []datapack.Zone{nearArena, deepOnFoot}, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTripBrain(t, Item{ObjectID: 1, TemplateID: adenaID, Count: tt.adena})
			b.persona.PvP.Arenas = true
			b.world.Self.ClassID = tt.class
			b.world.Self.move = stopAt(gateFrom)
			b.cfg.Travel.gatekeepers = []gatekeeper{
				{npcID: townGK, at: Point{X: 2000}, dests: []destination{{index: 2, at: Point{X: travelDest}, price: 1000}}},
			}
			b.cfg.Travel.zones.Arenas = tt.arenas
			if tt.arenas == nil {
				b.cfg.Travel.zones.Arenas = []datapack.Zone{nearArena, deepGated}
			}
			if tt.edit != nil {
				tt.edit(b)
			}
			b.decide(at(time.Second))
			due := b.pvp.arena.due
			if rule, _ := b.decide(due); rule != "arena" {
				t.Fatalf("rule %q when due, want the arena trip", rule)
			}
			a := &b.pvp.arena
			if a.arena != tt.want || a.gate != tt.gate || b.trip.v.active() != tt.gate {
				t.Fatalf("arena %d, gatekeeper trip %v (travel errand %v): want arena %d, %v",
					a.arena, a.gate, b.trip.v.active(), tt.want, tt.gate)
			}
			if want := arenaMiddle(b.cfg.Travel.zones.Arenas[tt.want-1], gateFrom.Z); a.to != want {
				t.Fatalf("trip to %v, want the arena's middle %v", a.to, want)
			}
		})
	}
}
