package botclient

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

const (
	// adenaDrop and junkDrop are ground items, junkTemplate a junk item's
	// template id.
	adenaDrop    = int32(0x40000001)
	junkDrop     = int32(0x40000002)
	junkTemplate = int32(1864) // Stem
)

var (
	adenaOnGround = GroundItem{ObjectID: adenaDrop, TemplateID: 57, Pos: Point{X: 900, Y: 20, Z: -5}, Stackable: true, Count: 120}
	junkOnGround  = GroundItem{ObjectID: junkDrop, TemplateID: junkTemplate, Pos: Point{X: 950}, Stackable: true, Count: 3}
)

func TestDecodeGroundItems(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		{
			"SpawnItem bytes",
			[]byte{
				0x0b, 1, 0, 0, 0x40, 57, 0, 0, 0, 0x84, 3, 0, 0, 20, 0, 0, 0, 0xfb, 0xff, 0xff, 0xff,
				1, 0, 0, 0, 120, 0, 0, 0, 0, 0, 0, 0,
			},
			SpawnItem{Item: adenaOnGround},
		},
		{
			"DropItem",
			packet(opGameDropItem, npcID, junkDrop, junkTemplate, 950, 0, 0, 1, 3, 1),
			DropItem{DropperID: npcID, Item: junkOnGround},
		},
		{
			"DropItem not stackable",
			packet(opGameDropItem, selfID, 7, 2369, 1, 2, 3, 0, 1, 1),
			DropItem{DropperID: selfID, Item: GroundItem{ObjectID: 7, TemplateID: 2369, Pos: Point{1, 2, 3}, Count: 1}},
		},
		{"GetItem", packet(opGameGetItem, pcID, adenaDrop, 900, 20, -5), GetItem{PickerID: pcID, ObjectID: adenaDrop, Pos: Point{900, 20, -5}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := decodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("decodeEvent = %v, %v, %v", got, ok, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("decodeEvent = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeGroundItemsRejects(t *testing.T) {
	spawn := packet(opGameSpawnItem, adenaDrop, 57, 1, 2, 3, 1, 5, 0)
	drop := packet(opGameDropItem, npcID, adenaDrop, 57, 1, 2, 3, 1, 5, 1)
	get := packet(opGameGetItem, pcID, adenaDrop, 1, 2, 3)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"SpawnItem short", spawn[:len(spawn)-1], wire.ErrShortPacket},
		{"SpawnItem trailing", append(spawn, 0), errTrailingBytes},
		{"DropItem short", drop[:len(drop)-4], wire.ErrShortPacket},
		{"DropItem trailing", append(drop, 0), errTrailingBytes},
		{"GetItem short", get[:len(get)-1], wire.ErrShortPacket},
		{"GetItem trailing", append(get, 0), errTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("decodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestEncodeRequestDropItem(t *testing.T) {
	got := encodeRequestDropItem(0x40000003, 25, Point{X: 1, Y: -2, Z: 300})
	want := []byte{0x12, 3, 0, 0, 0x40, 25, 0, 0, 0, 1, 0, 0, 0, 0xfe, 0xff, 0xff, 0xff, 0x2c, 1, 0, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("encodeRequestDropItem = % x, want % x", got, want)
	}
}

func TestWorldGround(t *testing.T) {
	tests := []struct {
		name string
		evs  []Event
		want map[int32]GroundItem
	}{
		{"spawned", []Event{SpawnItem{Item: adenaOnGround}}, map[int32]GroundItem{adenaDrop: adenaOnGround}},
		{"dropped", []Event{DropItem{DropperID: npcID, Item: junkOnGround}}, map[int32]GroundItem{junkDrop: junkOnGround}},
		{
			"picked up",
			[]Event{SpawnItem{Item: adenaOnGround}, DropItem{DropperID: npcID, Item: junkOnGround}, GetItem{PickerID: pcID, ObjectID: adenaDrop}},
			map[int32]GroundItem{junkDrop: junkOnGround},
		},
		{"out of sight", []Event{SpawnItem{Item: adenaOnGround}, DeleteObject{ObjectID: adenaDrop}}, map[int32]GroundItem{}},
		{"loading-screen teleport", []Event{SpawnItem{Item: adenaOnGround}, TeleportToLocation{ObjectID: selfID, Pos: Point{X: 9}}}, map[int32]GroundItem{}},
		{
			"in-place correction keeps them",
			[]Event{SpawnItem{Item: adenaOnGround}, TeleportToLocation{ObjectID: selfID, Pos: Point{X: 9}, Fast: true}},
			map[int32]GroundItem{adenaDrop: adenaOnGround},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorld(t)
			for _, ev := range tt.evs {
				w.Apply(ev, t0)
			}
			if !reflect.DeepEqual(w.Ground, tt.want) {
				t.Fatalf("Ground = %+v, want %+v", w.Ground, tt.want)
			}
		})
	}
}

// lootPersona picks up loot with the default policy and does nothing else.
func lootPersona() Persona { return Persona{Loot: defaultLootPolicy()} }

// timed is an event handled at t0 plus at.
type timed struct {
	at time.Duration
	ev Event
}

// kill is the bot's swing on npcID, its death and its drop of items.
func kill(items ...GroundItem) []timed {
	evs := []timed{{0, Attack{AttackerID: selfID, Hits: []AttackHit{{TargetID: npcID, Damage: 30}}}}, {0, Die{ObjectID: npcID}}}
	for _, it := range items {
		evs = append(evs, timed{0, DropItem{DropperID: npcID, Item: it}})
	}
	return evs
}

func handleAll(t *testing.T, b *Brain, evs []timed) {
	t.Helper()
	for _, e := range evs {
		if err := b.handle([]Event{e.ev}, at(e.at)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRuleLoot(t *testing.T) {
	now := at(time.Second)
	junk := Item{ObjectID: 0x40000010, TemplateID: junkTemplate, Count: 7}
	carry := func(items ...Item) func(b *Brain) {
		return func(b *Brain) {
			b.world.Self.Load, b.world.Self.MaxLoad = 85, 100
			b.world.Inventory.Items = map[int32]Item{}
			for _, it := range items {
				b.world.Inventory.Items[it.ObjectID] = it
			}
		}
	}
	dropper := func(p *Persona) { p.Loot.Junk = []int32{junkTemplate} }
	tests := []struct {
		name    string
		persona func(*Persona)
		setup   func(b *Brain)
		evs     []timed
		// wantRule is the matching rule, want its payload (nil for none).
		wantRule string
		want     []byte
	}{
		{name: "own kill's drop is picked up", evs: kill(adenaOnGround), wantRule: "loot", want: encodeAction(adenaDrop, Point{})},
		{name: "a bot told to stay leaves the drop", setup: func(b *Brain) { b.orders.stay = true }, evs: kill(adenaOnGround), wantRule: "stay"},
		{
			name:     "drop of a monster that hit the bot",
			evs:      []timed{{0, Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}}, {0, Die{ObjectID: npcID}}, {0, DropItem{DropperID: npcID, Item: adenaOnGround}}},
			wantRule: "loot", want: encodeAction(adenaDrop, Point{}),
		},
		{
			name:     "drop of a monster the bot cast at",
			evs:      []timed{{0, MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: 1177}}, {0, Die{ObjectID: npcID}}, {0, DropItem{DropperID: npcID, Item: adenaOnGround}}},
			wantRule: "loot", want: encodeAction(adenaDrop, Point{}),
		},
		{name: "drop of a monster the bot never fought", evs: []timed{{0, Die{ObjectID: npcID}}, {0, DropItem{DropperID: npcID, Item: adenaOnGround}}}},
		{
			name: "drop of a player who buffed the bot",
			evs:  []timed{{0, MagicSkillUse{CasterID: pcID, TargetID: selfID, SkillID: 1068}}, {0, DropItem{DropperID: pcID, Item: adenaOnGround}}},
		},
		{
			name: "drop of a player the bot healed",
			evs:  []timed{{0, MagicSkillUse{CasterID: selfID, TargetID: pcID, SkillID: 1011}}, {0, DropItem{DropperID: pcID, Item: adenaOnGround}}},
		},
		{
			// ForceAttack lets the bot fight any player, but never its own party.
			name:    "drop of a party member the bot healed",
			persona: func(p *Persona) { p.PvP.ForceAttack = true },
			setup: func(b *Brain) {
				b.world.Party = Party{LeaderID: selfID, Loot: 1, Members: map[int32]*PartyMate{pcID: {}}}
			},
			evs: []timed{{0, MagicSkillUse{CasterID: selfID, TargetID: pcID, SkillID: 1011}}, {0, DropItem{DropperID: pcID, Item: adenaOnGround}}},
		},
		{
			name:     "drop of a flagged player the bot fought",
			persona:  func(p *Persona) { p.PvP.FightFlagged = true },
			setup:    func(b *Brain) { b.world.Objects[pcID].PvPFlag = 1 },
			evs:      []timed{{0, Attack{AttackerID: pcID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}}, {0, Die{ObjectID: pcID}}, {0, DropItem{DropperID: pcID, Item: adenaOnGround}}},
			wantRule: "loot", want: encodeAction(adenaDrop, Point{}),
		},
		{
			name: "drop of a monster fought too long ago",
			evs: []timed{
				{-lootFoughtFor - time.Millisecond, Attack{AttackerID: selfID, Hits: []AttackHit{{TargetID: npcID, Damage: 30}}}},
				{0, DropItem{DropperID: npcID, Item: adenaOnGround}},
			},
		},
		{name: "the bot's own drop", evs: append(kill(), timed{0, DropItem{DropperID: selfID, Item: adenaOnGround}})},
		{name: "drop out of the loot radius", evs: kill(GroundItem{ObjectID: adenaDrop, TemplateID: 57, Pos: Point{X: 1001}, Count: 1})},
		{name: "loot off", persona: func(p *Persona) { p.Loot.Pickup = false }, evs: kill(adenaOnGround)},
		{name: "picked up by another", evs: append(kill(adenaOnGround), timed{0, GetItem{PickerID: pcID, ObjectID: adenaDrop}})},
		{name: "out of sight", evs: append(kill(adenaOnGround), timed{0, DeleteObject{ObjectID: adenaDrop}})},
		{name: "oldest drop first", evs: kill(junkOnGround, adenaOnGround), wantRule: "loot", want: encodeAction(junkDrop, Point{})},
		{
			name: "finders keepers party member leaves it",
			setup: func(b *Brain) {
				b.world.Party = Party{LeaderID: pcID, Loot: partyLootFindersKeepers, Members: map[int32]*PartyMate{pcID: {}}}
			},
			evs: kill(adenaOnGround),
		},
		{
			name: "finders keepers party leader loots",
			setup: func(b *Brain) {
				b.world.Party = Party{LeaderID: selfID, Loot: partyLootFindersKeepers, Members: map[int32]*PartyMate{pcID: {}}}
			},
			evs:      kill(adenaOnGround),
			wantRule: "loot", want: encodeAction(adenaDrop, Point{}),
		},
		{
			name: "random loot party member loots",
			setup: func(b *Brain) {
				b.world.Party = Party{LeaderID: pcID, Loot: 1, Members: map[int32]*PartyMate{pcID: {}}}
			},
			evs:      kill(adenaOnGround),
			wantRule: "loot", want: encodeAction(adenaDrop, Point{}),
		},
		{
			name:    "finders keepers member loots when the persona says so",
			persona: func(p *Persona) { p.Loot.LeaveFindersKeepers = false },
			setup: func(b *Brain) {
				b.world.Party = Party{LeaderID: pcID, Loot: partyLootFindersKeepers, Members: map[int32]*PartyMate{pcID: {}}}
			},
			evs:      kill(adenaOnGround),
			wantRule: "loot", want: encodeAction(adenaDrop, Point{}),
		},
		{
			name:     "combat stance after the kill yields to the loot",
			setup:    func(b *Brain) { b.world.Self.InCombat = true },
			evs:      kill(adenaOnGround),
			wantRule: "loot", want: encodeAction(adenaDrop, Point{}),
		},
		{name: "combat stance with nothing queued holds", setup: func(b *Brain) { b.world.Self.InCombat = true }, evs: kill(), wantRule: "attacked"},
		{
			name: "an attacker left comes first",
			setup: func(b *Brain) {
				b.world.Self.InCombat = true
				b.world.Apply(NPCInfo{ObjectID: mob2ID, NPCID: 20002, Attackable: true, Pos: Point{X: 30}}, t0)
			},
			evs:      append(kill(adenaOnGround), timed{0, Attack{AttackerID: mob2ID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}}),
			wantRule: "attacked", want: selectAction(mob2ID, Point{}).payload,
		},
		{name: "sitting stands up first", setup: func(b *Brain) { b.world.Self.Sitting = true }, evs: kill(adenaOnGround), wantRule: "loot", want: sitStandAction().payload},
		{
			name:    "overweight drops junk first",
			persona: dropper, setup: carry(junk, Item{ObjectID: 0x40000011, TemplateID: 57, Count: 9}),
			evs:      kill(adenaOnGround),
			wantRule: "loot", want: encodeRequestDropItem(junk.ObjectID, 7, Point{}),
		},
		{
			name:    "lowest junk stack first",
			persona: dropper, setup: carry(Item{ObjectID: 0x40000012, TemplateID: junkTemplate, Count: 1}, junk),
			wantRule: "loot", want: encodeRequestDropItem(junk.ObjectID, 7, Point{}),
		},
		{name: "equipped junk is kept", persona: dropper, setup: carry(Item{ObjectID: junk.ObjectID, TemplateID: junkTemplate, Count: 1, Equipped: true})},
		{name: "no junk carried", persona: dropper, setup: carry(Item{ObjectID: 0x40000011, TemplateID: 57, Count: 9})},
		{
			name:    "under the weight limit",
			persona: dropper,
			setup: func(b *Brain) {
				carry(junk)(b)
				b.world.Self.Load = 79
			},
		},
		{name: "drop off", persona: func(p *Persona) { dropper(p); p.Loot.DropAbove = 0 }, setup: carry(junk)},
		{
			name:    "no junk dropped on the move",
			persona: dropper,
			setup: func(b *Brain) {
				carry(junk)(b)
				b.world.Apply(MoveToLocation{ObjectID: selfID, Origin: Point{}, Dest: Point{X: 1000}}, now)
			},
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
			rule, a := b.decide(now)
			if rule != tt.wantRule || !bytes.Equal(a.payload, tt.want) {
				t.Fatalf("decide = %q % x, want %q % x", rule, a.payload, tt.wantRule, tt.want)
			}
		})
	}
}

// TestLootPickupSequence follows a queue of two drops: clicks spaced by
// lootRetry and by the walk the server starts, the first item given up
// after the timeout, the second taken by another player.
func TestLootPickupSequence(t *testing.T) {
	b, out := newTestBrain(t, lootPersona())
	handleAll(t, b, kill(adenaOnGround, junkOnGround))
	steps := []struct {
		at   time.Duration
		evs  []Event
		rule string
		want []byte
	}{
		{at: time.Second, rule: "loot", want: encodeAction(adenaDrop, Point{})},
		// The server walks the bot to the item.
		{at: 1100 * time.Millisecond, evs: []Event{MoveToLocation{ObjectID: selfID, Dest: Point{X: 3000}}}, rule: "loot"},
		{at: 2 * time.Second, rule: "loot"},
		{at: 5 * time.Second, rule: "loot"},
		// Stopped short: lootRetry is long gone, click again from there.
		{at: 6 * time.Second, evs: []Event{StopMove{ObjectID: selfID, Pos: Point{X: 400}}}, rule: "loot", want: encodeAction(adenaDrop, Point{X: 400})},
		{at: 8 * time.Second, rule: "loot"},
		{at: 9 * time.Second, rule: "loot", want: encodeAction(adenaDrop, Point{X: 400})},
		// Timeout after the first click: on to the next item.
		{at: 16 * time.Second, rule: "loot", want: encodeAction(junkDrop, Point{X: 400})},
		{at: 17 * time.Second, evs: []Event{GetItem{PickerID: pcID, ObjectID: junkDrop}}, rule: ""},
	}
	for _, s := range steps {
		now := at(s.at)
		if err := b.handle(s.evs, now); err != nil {
			t.Fatal(err)
		}
		sent := len(out.frames)
		rule, a := b.decide(now)
		if a.kind != actHold {
			if ok, err := b.send(a, now); !ok || err != nil {
				t.Fatalf("at %v: send = %v, %v", s.at, ok, err)
			}
		}
		var got []byte
		if len(out.frames) > sent {
			got = out.frames[len(out.frames)-1]
		}
		if rule != s.rule || !bytes.Equal(got, s.want) {
			t.Fatalf("at %v: decide = %q, sent % x; want %q, % x", s.at, rule, got, s.rule, s.want)
		}
	}
}

// TestLootDropSequence: junk stacks go one per dropGap, and a stack asked
// once is asked again only after dropRetry.
func TestLootDropSequence(t *testing.T) {
	p := lootPersona()
	p.Loot.Junk = []int32{junkTemplate}
	b, out := newTestBrain(t, p)
	first := Item{ObjectID: 0x40000010, TemplateID: junkTemplate, Count: 7}
	second := Item{ObjectID: 0x40000011, TemplateID: junkTemplate, Count: 2}
	b.world.Self.Load, b.world.Self.MaxLoad = 90, 100
	b.world.Inventory.Items = map[int32]Item{first.ObjectID: first, second.ObjectID: second}
	steps := []struct {
		at   time.Duration
		rule string
		// paced marks a drop the pacer holds back.
		paced bool
		want  []byte
	}{
		{at: time.Second, rule: "loot", want: encodeRequestDropItem(first.ObjectID, 7, Point{})},
		{at: 2 * time.Second, rule: "loot", paced: true},
		{at: 3 * time.Second, rule: "loot", want: encodeRequestDropItem(second.ObjectID, 2, Point{})},
		{at: 4 * time.Second, rule: ""},
		{at: time.Second + dropRetry, rule: "loot", want: encodeRequestDropItem(first.ObjectID, 7, Point{})},
	}
	for _, s := range steps {
		now := at(s.at)
		sent := len(out.frames)
		rule, a := b.decide(now)
		if a.kind != actHold {
			if ok, err := b.send(a, now); ok == s.paced || err != nil {
				t.Fatalf("at %v: send = %v, %v; paced %v", s.at, ok, err, s.paced)
			}
		}
		var got []byte
		if len(out.frames) > sent {
			got = out.frames[len(out.frames)-1]
		}
		if rule != s.rule || !bytes.Equal(got, s.want) {
			t.Fatalf("at %v: decide = %q, sent % x; want %q, % x", s.at, rule, got, s.rule, s.want)
		}
	}
}
