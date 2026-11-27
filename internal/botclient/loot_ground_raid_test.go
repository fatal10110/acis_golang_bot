package botclient

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// TestLootGroundRaid checks the raid loot lock (#460): a drop of a raid or
// grand boss the bot did not fight for waits lootRaidGrace before the bot
// takes it; every other ground item it did not fight for waits lootGrace,
// and a boss's drop it fought for is taken at once.
func TestLootGroundRaid(t *testing.T) {
	const raidTpl, grandTpl = 25001, 29001
	bosses := &TravelMap{bosses: map[int32]bool{raidTpl: true, grandTpl: true}}
	dropper := func(tpl int32, travel *TravelMap) func(b *Brain) {
		return func(b *Brain) {
			b.world.Objects[npcID].NPCID = tpl
			b.cfg.Travel = travel
		}
	}
	stray := []timed{{0, Die{ObjectID: npcID}}, {0, DropItem{DropperID: npcID, Item: strayDrop}}}
	tests := []struct {
		name  string
		setup func(b *Brain)
		evs   []timed
		at    time.Duration // when the bot decides
		want  int32         // the item picked up, 0 for none
	}{
		{name: "raid boss drop within the regular grace", setup: dropper(raidTpl, bosses), evs: stray, at: lootGrace},
		{name: "raid boss drop within the raid grace", setup: dropper(raidTpl, bosses), evs: stray, at: lootRaidGrace - time.Millisecond},
		{name: "raid boss drop after the raid grace", setup: dropper(raidTpl, bosses), evs: stray, at: lootRaidGrace, want: junkDrop},
		{name: "grand boss drop within the raid grace", setup: dropper(grandTpl, bosses), evs: stray, at: lootRaidGrace - time.Millisecond},
		{name: "grand boss drop after the raid grace", setup: dropper(grandTpl, bosses), evs: stray, at: lootRaidGrace, want: junkDrop},
		{name: "monster drop after the regular grace", setup: dropper(20001, bosses), evs: stray, at: lootGrace, want: junkDrop},
		{name: "no travel map keeps the regular grace", setup: dropper(raidTpl, nil), evs: stray, at: lootGrace, want: junkDrop},
		{
			name: "dropper out of sight keeps the regular grace", setup: dropper(raidTpl, bosses),
			evs: []timed{{0, DeleteObject{ObjectID: npcID}}, {0, DropItem{DropperID: npcID, Item: strayDrop}}}, at: lootGrace, want: junkDrop,
		},
		{
			name: "player drop after the regular grace", setup: dropper(raidTpl, bosses),
			evs: []timed{{0, DropItem{DropperID: pcID, Item: strayDrop}}}, at: lootGrace, want: junkDrop,
		},
		{
			name: "item come into sight near a boss after the regular grace", setup: dropper(raidTpl, bosses),
			evs: []timed{{0, Die{ObjectID: npcID}}, {0, SpawnItem{Item: strayDrop}}}, at: lootGrace, want: junkDrop,
		},
		{name: "raid boss drop it fought for", setup: dropper(raidTpl, bosses), evs: kill(strayDrop), at: time.Second, want: junkDrop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := lootPersona()
			p.Loot.Ground = GroundAny
			b, _ := newTestBrain(t, p)
			tt.setup(b)
			handleAll(t, b, tt.evs)
			rule, a := b.decide(at(tt.at))
			var want []byte
			if tt.want != 0 {
				want = botwire.EncodeAction(tt.want, Point{})
			}
			if !bytes.Equal(a.payload, want) || (want != nil) != (rule == "loot") {
				t.Fatalf("decide = %q % x, want loot % x", rule, a.payload, want)
			}
		})
	}
}
