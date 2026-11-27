package botclient

import (
	"bytes"
	"testing"
	"time"
)

// strayDrop is a ground item no creature the bot fought dropped.
var strayDrop = GroundItem{ObjectID: junkDrop, TemplateID: junkTemplate, Pos: Point{X: 950}, Stackable: true, Count: 3}

// TestLootGround checks the ground loot source (#435): ground items the bot
// did not fight for are queued lootGrace after it first sees them, any of
// them for GroundAny, only those it keeps or would wear for GroundFit,
// never its own drops or Junk, within the loot radius and outside a finders
// keepers party it does not lead.
func TestLootGround(t *testing.T) {
	stray := func(it GroundItem) []timed {
		return []timed{{0, Die{ObjectID: npcID}}, {0, DropItem{DropperID: npcID, Item: it}}}
	}
	anyLoot := func(p *Persona) { p.Loot.Ground = GroundAny }
	potion := GroundItem{ObjectID: junkDrop, TemplateID: 1060, Pos: Point{X: 950}, Stackable: true, Count: 2}
	tests := []struct {
		name    string
		persona func(*Persona)
		setup   func(b *Brain)
		evs     []timed
		at      time.Duration // when the bot decides; lootGrace when 0
		want    int32         // the item picked up, 0 for none
	}{
		{name: "farmer takes a drop of a monster it never fought", persona: anyLoot, evs: stray(strayDrop), want: junkDrop},
		{name: "farmer leaves it within the grace", persona: anyLoot, evs: stray(strayDrop), at: lootGrace - time.Millisecond},
		{name: "farmer takes an item that came into sight", persona: anyLoot, evs: []timed{{0, SpawnItem{Item: strayDrop}}}, want: junkDrop},
		{name: "farmer takes a player's drop after the grace", persona: anyLoot, evs: []timed{{0, DropItem{DropperID: pcID, Item: strayDrop}}}, want: junkDrop},
		{name: "farmer leaves its own drop", persona: anyLoot, evs: []timed{{0, DropItem{DropperID: selfID, Item: strayDrop}}}},
		{name: "farmer leaves its junk", persona: func(p *Persona) { anyLoot(p); p.Loot.Junk = []int32{junkTemplate} }, evs: stray(strayDrop)},
		{name: "farmer leaves it out of the loot radius", persona: anyLoot, evs: stray(GroundItem{ObjectID: junkDrop, TemplateID: junkTemplate, Pos: Point{X: 1001}, Count: 1})},
		{name: "farmer leaves it picked up by another", persona: anyLoot, evs: append(stray(strayDrop), timed{time.Second, GetItem{PickerID: pcID, ObjectID: junkDrop}})},
		{name: "farmer leaves it out of sight", persona: anyLoot, evs: append(stray(strayDrop), timed{time.Second, DeleteObject{ObjectID: junkDrop}})},
		{
			name: "farmer in a finders keepers party it does not lead leaves it", persona: anyLoot,
			setup: func(b *Brain) {
				b.world.Party = Party{LeaderID: pcID, Loot: partyLootFindersKeepers, Members: map[int32]*PartyMate{pcID: {}}}
			},
			evs: stray(strayDrop),
		},
		{name: "loot off", persona: func(p *Persona) { anyLoot(p); p.Loot.Pickup = false }, evs: stray(strayDrop)},
		{name: "GroundNone leaves it", persona: func(p *Persona) { p.Loot.Ground = GroundNone }, evs: stray(adenaOnGround)},
		{name: "a fitting persona leaves what it does not keep", evs: stray(strayDrop)},
		{name: "a fitting persona takes adena", evs: stray(adenaOnGround), want: adenaDrop},
		{name: "a fitting persona takes its potions", persona: func(p *Persona) { p.Potions = []int32{1060} }, evs: stray(potion), want: junkDrop},
		{name: "a fitting persona leaves potions it does not drink", evs: stray(potion)},
		{
			name: "a fitting persona takes what its keep hook holds",
			persona: func(p *Persona) {
				p.Holds = func(_ *World, it Item) Hold { return Hold{Keep: it.TemplateID == junkTemplate} }
			},
			evs: stray(strayDrop), want: junkDrop,
		},
		{
			name: "own kill's drop stays queued first", persona: anyLoot,
			evs:  append(stray(strayDrop), kill(adenaOnGround)...),
			want: adenaDrop,
		},
		{
			name: "another bot of the process is after it", persona: anyLoot,
			setup: func(b *Brain) {
				b.cfg.Ground = NewGroundClaims()
				b.cfg.Ground.claim(junkDrop, pcID, at(lootGrace))
			},
			evs: stray(strayDrop),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := lootPersona()
			if tt.persona != nil {
				tt.persona(&p)
			}
			b, _ := newTestBrain(t, p)
			if tt.setup != nil {
				tt.setup(b)
			}
			handleAll(t, b, tt.evs)
			d := tt.at
			if d == 0 {
				d = lootGrace
			}
			rule, a := b.decide(at(d))
			var want []byte
			if tt.want != 0 {
				want = encodeAction(tt.want, Point{})
			}
			if !bytes.Equal(a.payload, want) || (want != nil) != (rule == "loot") {
				t.Fatalf("decide = %q % x, want loot % x", rule, a.payload, want)
			}
		})
	}
}

// TestLootGroundWeight checks that a ground item the farmer did not fight
// for is given up when it would bring the load to Loot.StopAbove, and taken
// under it, with no supply short (TestLootGroundSupplies).
func TestLootGroundWeight(t *testing.T) {
	item := GroundItem{ObjectID: junkDrop, TemplateID: keptMat, Pos: Point{X: 900}, Stackable: true, Count: 5}
	for _, tt := range []struct {
		name string
		load int32
		want int32
	}{
		{name: "over the cap", load: 64950},
		{name: "under the cap", load: 64949, want: junkDrop},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newTripBrain(t, potionStack(20), soeStack(2))
			b.persona.Loot = defaultLootPolicy()
			b.persona.Loot.Ground = GroundAny
			b.world.Self.Load = tt.load
			b.world.Ground = map[int32]GroundItem{item.ObjectID: item}
			b.lootEvent(DropItem{DropperID: npcID, Item: item}, t0)
			var got int32
			if a, ok := b.ruleLoot(t0.Add(lootGrace)); ok && a.kind == actPickup {
				got = a.object
			}
			if got != tt.want {
				t.Fatalf("ruleLoot picks up %d, want %d", got, tt.want)
			}
		})
	}
}

// TestLootGroundClaims checks that two bots of a process sharing
// GroundClaims never chase one item: the first to decide claims it, the
// other leaves it, and takes the next item instead.
func TestLootGroundClaims(t *testing.T) {
	claims := NewGroundClaims()
	brains := make([]*Brain, 2)
	for i := range brains {
		p := lootPersona()
		p.Loot.Ground = GroundAny
		b, _ := newTestBrain(t, p)
		b.cfg.Ground = claims
		handleAll(t, b, []timed{{0, DropItem{DropperID: npcID, Item: strayDrop}}, {0, DropItem{DropperID: npcID, Item: adenaOnGround}}})
		brains[i] = b
	}
	// Both brains are the same character in the test world: the second
	// claims as another bot.
	brains[1].world.Self.ObjectID = selfID + 1
	now := at(lootGrace)
	for i, want := range []int32{adenaDrop, junkDrop} {
		if got := brains[i].nextLoot(now); got != want {
			t.Fatalf("bot %d loots %d, want %d", i, got, want)
		}
	}
	// The first one's item leaves the ground: its claim ends, and the
	// second keeps its own.
	brains[0].lootEvent(GetItem{PickerID: selfID, ObjectID: adenaDrop}, now)
	if claims.claim(adenaDrop, pcID, now) != true || claims.claim(junkDrop, pcID, now) {
		t.Fatal("claims after the pick-up: want adena free and the stem held")
	}
}

// TestGroundClaims checks the claim table: one holder per item, renewed by
// its holder, released by it alone, lapsing lootClaimFor after the last
// renewal, and dropped for everyone when the item leaves the ground.
func TestGroundClaims(t *testing.T) {
	const item, a, b = int32(7), int32(1), int32(2)
	c := NewGroundClaims()
	steps := []struct {
		name string
		do   func() bool
		want bool
	}{
		{"a claims", func() bool { return c.claim(item, a, at(0)) }, true},
		{"b is refused", func() bool { return c.claim(item, b, at(time.Second)) }, false},
		{"a renews", func() bool { return c.claim(item, a, at(10*time.Second)) }, true},
		{"b is refused past the first claim's lapse", func() bool { return c.claim(item, b, at(lootClaimFor)) }, false},
		{"b releasing does nothing", func() bool { c.release(item, b); return c.claim(item, b, at(lootClaimFor)) }, false},
		{"b claims once a's renewal lapsed", func() bool { return c.claim(item, b, at(10*time.Second+lootClaimFor)) }, true},
		{"b releases, a claims", func() bool { c.release(item, b); return c.claim(item, a, at(30*time.Second)) }, true},
		{"the item leaves the ground, b claims", func() bool { c.drop(item); return c.claim(item, b, at(30*time.Second)) }, true},
	}
	for _, s := range steps {
		if got := s.do(); got != s.want {
			t.Fatalf("%s: claim = %v, want %v", s.name, got, s.want)
		}
	}
	var none *GroundClaims
	if !none.claim(item, a, at(0)) {
		t.Fatal("a nil table refused a claim")
	}
	none.release(item, a)
	none.drop(item)
}

// TestLootGroundGear checks GroundFit's gear: a ground piece is taken when
// it beats what the bot wears (Brain.beats), left otherwise; GroundAny
// takes either.
func TestLootGroundGear(t *testing.T) {
	sword := Item{ObjectID: 201, TemplateID: shortSword, Equipped: true, BodySlot: partRHand}
	for _, tt := range []struct {
		name   string
		ground GroundLoot
		id     int32
		want   bool
	}{
		{name: "a better sword", ground: GroundFit, id: longSword, want: true},
		{name: "a two-handed sword it does not wear", ground: GroundFit, id: twoHanded},
		{name: "heavy armor over light", ground: GroundFit, id: bronzePlate},
		{name: "the farmer takes it anyway", ground: GroundAny, id: bronzePlate, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newGearBrain(t, 0, sword)
			b.persona.Loot.Ground = tt.ground
			if got := b.groundTakes(GroundItem{ObjectID: junkDrop, TemplateID: tt.id, Count: 1}); got != tt.want {
				t.Fatalf("groundTakes(%d) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}
