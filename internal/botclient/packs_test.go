package botclient

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// tiPack and tiOrcs carry the pack facts of Talking Island's werewolf
// maker gludio31_1625_10m1 and orc maker gludio31_1625_02m1 (spawnlist
// 16_25.xml, npcs 20000-20999.xml): 10 and 9 aggressive level 10-12
// monsters, aggro range 1000, clan range 300, running at 118.8 and 108.9.
var (
	tiPack = datapack.Spot{MinLevel: 10, MaxLevel: 12, AggroLevel: 12, Aggressive: 10, AggroRange: 1000, ClanRange: 300, RunSpeed: 118.8, Area: 1.08e7}
	tiOrcs = datapack.Spot{MinLevel: 10, MaxLevel: 12, AggroLevel: 12, Aggressive: 9, AggroRange: 1000, ClanRange: 300, RunSpeed: 108.9, Area: 1.32e7}
)

func TestSpotPack(t *testing.T) {
	tests := []struct {
		name string
		spot datapack.Spot
		want float64
	}{
		{"werewolves", tiPack, 4.92},
		{"orcs", tiOrcs, 3.61},
		{"reach covers the territory", datapack.Spot{Aggressive: 4, AggroRange: 1000, Area: 1e6}, 4},
		{"no aggressive monster", datapack.Spot{AggroRange: 1000, Area: 1e6}, 0},
		{"no area", datapack.Spot{Aggressive: 4, AggroRange: 1000}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := spotPack(&tt.spot); math.Abs(got-tt.want) > 0.01 {
				t.Fatalf("spotPack = %.3f, want %.2f", got, tt.want)
			}
		})
	}
}

// TestPackBound checks which spots a bound learnt from a death covers, for
// a human fighter (run 115) of level 14.
func TestPackBound(t *testing.T) {
	now := at(time.Minute)
	with := func(s datapack.Spot, f func(*datapack.Spot)) datapack.Spot { f(&s); return s }
	// fastBound is a death at tiPack's like, slowBound one at tiOrcs'.
	fastBound := packBound{pack: 0.8 * 4.92, above: -2, fast: true, until: at(30 * time.Minute)}
	slowBound := packBound{pack: 0.8 * 3.61, above: -2, until: at(30 * time.Minute)}
	tests := []struct {
		name  string
		bound packBound
		spot  datapack.Spot
		want  bool
	}{
		{"the werewolf pack itself", fastBound, tiPack, true},
		{"a pack of 4.4 as fast", fastBound, with(tiPack, func(s *datapack.Spot) { s.Aggressive = 9 }), true},
		{"a pack of 3.6 as fast", fastBound, with(tiPack, func(s *datapack.Spot) { s.Area = 1.48e7 }), false},
		{"a slower pack, after a death at a faster one", fastBound, with(tiPack, func(s *datapack.Spot) { s.RunSpeed = 108.9 }), false},
		{"a faster pack, after a death at a slower one", slowBound, tiPack, true},
		{"lower monsters", fastBound, with(tiPack, func(s *datapack.Spot) { s.AggroLevel = 11 }), false},
		{"no aggressive monster", slowBound, with(tiPack, func(s *datapack.Spot) { s.Aggressive = 0 }), false},
		{"the time is up", packBound{pack: 1, above: -9, until: now}, tiPack, false},
		{"no bound", packBound{}, tiPack, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.bound.covers(&tt.spot, 14, 115, now); got != tt.want {
				t.Fatalf("covers = %v, want %v (pack %.2f)", got, tt.want, spotPack(&tt.spot))
			}
		})
	}
}

// TestSpotPackDeath checks a death holding a pack spot passes over its like
// for DeathAvoid (#281): a level 14 fighter that died at a werewolf pack
// skips a werewolf pack like it for a smaller orc pack it outruns; dead
// there too, it passes over every pack at least 0.8 of the orcs'. With
// PackShare 0 it avoids the spots it died at alone.
func TestSpotPackDeath(t *testing.T) {
	at3 := func(s datapack.Spot, name string, x, y int32) datapack.Spot {
		s.Maker, s.Center = name, datapack.Point{X: x, Y: y}
		return s
	}
	wolves2 := at3(tiPack, "wolves2", 0, 3000)
	wolves2.Aggressive = 9
	spots := []datapack.Spot{at3(tiPack, "wolves", 1000, 0), wolves2, at3(tiOrcs, "orcs", 3000, 3000)}
	type step struct {
		name string
		die  bool
		at   time.Duration
		want int
	}
	tests := []struct {
		name  string
		share float64
		steps []step
	}{
		{
			name: "the like passed over", share: 0.8,
			steps: []step{
				{name: "the near wolves", at: time.Second, want: 0},
				{name: "died there: the orcs, not the wolves like them", die: true, at: 2 * time.Second, want: 2},
				{name: "died at the orcs: no pack left", die: true, at: 5 * time.Minute, want: -1},
			},
		},
		{
			name: "PackShare 0: the spot alone",
			steps: []step{
				{name: "the near wolves", at: time.Second, want: 0},
				{name: "died there: the wolves like them", die: true, at: 2 * time.Second, want: 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 14, spots...)
			b.world.Self.RunSpeed = 115
			b.persona.Spots.PackShare = tt.share
			for _, s := range tt.steps {
				now := at(s.at)
				if s.die {
					if err := b.handle([]Event{Die{ObjectID: selfID}}, now); err != nil {
						t.Fatal(err)
					}
					if err := b.handle([]Event{Revive{ObjectID: selfID}}, now); err != nil {
						t.Fatal(err)
					}
				}
				b.decide(now)
				got := -1
				if b.spot.held {
					got = b.spot.idx
				}
				if got != s.want {
					t.Fatalf("%s: spot %d, want %d (bound %+v)", s.name, got, s.want, b.spot.pack)
				}
			}
		})
	}
}

// TestFleeOutrun checks FleeOutrun in rules 3 and 4: a bot flees only
// attackers it outruns, and fights on, drinking, against one as fast as it
// or faster. The bot runs at 100.
func TestFleeOutrun(t *testing.T) {
	const npc2, npc3 = int32(0x20000005), int32(0x20000006)
	now := at(time.Second)
	potion := Item{ObjectID: 0x30000011, TemplateID: healingPotion, Count: 5}
	tests := []struct {
		name string
		off  bool
		// speeds are the run speeds of npcID, npc2 and npc3; 0 leaves one
		// out of the fight.
		speeds   [3]int32
		hp       int32
		setup    func(b *Brain)
		wantRule string
		wantKind actionKind
	}{
		{name: "outnumbered by slower ones: flees", speeds: [3]int32{80, 80}, hp: 40, wantRule: "outnumbered", wantKind: actMove},
		{name: "outnumbered, one as fast: fights", speeds: [3]int32{80, 100}, hp: 40, wantRule: "outnumbered", wantKind: actSelect},
		{name: "outnumbered, one faster: fights", speeds: [3]int32{200, 80}, hp: 40, wantRule: "outnumbered", wantKind: actSelect},
		{name: "FleeOutrun off: flees faster ones", off: true, speeds: [3]int32{200, 200}, hp: 40, wantRule: "outnumbered", wantKind: actMove},
		{
			name: "a dead faster one holds no one", speeds: [3]int32{80, 80, 200}, hp: 40,
			setup:    func(b *Brain) { b.world.Apply(Die{ObjectID: npc3}, t0) },
			wantRule: "outnumbered", wantKind: actMove,
		},
		{
			name: "a faster one done hitting holds no one", speeds: [3]int32{80, 80}, hp: 40,
			setup: func(b *Brain) {
				b.world.Apply(NPCInfo{ObjectID: npc3, NPCID: 20001, Attackable: true, Pos: Point{Y: 1000}, RunSpeed: 200, Running: true}, t0)
				b.world.Apply(Attack{AttackerID: npc3, Pos: Point{Y: 1000}, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, now.Add(-attackerWindow-time.Second))
			},
			wantRule: "outnumbered", wantKind: actMove,
		},
		{name: "critical, slower: flees", speeds: [3]int32{80}, hp: 10, wantRule: "critical", wantKind: actMove},
		{name: "critical, faster: fights", speeds: [3]int32{200}, hp: 10, wantRule: "attacked", wantKind: actAttack},
		{
			name: "critical, faster, potion ready: drinks", speeds: [3]int32{200}, hp: 10,
			setup:    func(b *Brain) { b.world.Apply(ItemList{Items: []Item{potion}}, t0) },
			wantRule: "critical", wantKind: actUseItem,
		},
		{
			name: "critical, faster, sitting: stands to fight", speeds: [3]int32{200}, hp: 10,
			setup:    func(b *Brain) { b.world.Self.Sitting = true },
			wantRule: "attacked", wantKind: actSitStand,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := newTestBrain(t, Persona{CriticalHP: 20, OutmatchedHP: 50, FleeDistance: 600, FleeOutrun: !tt.off, Potions: []int32{healingPotion}})
			b.world.Self.InCombat, b.world.Self.HP = true, tt.hp
			for i, id := range []int32{npcID, npc2, npc3} {
				if tt.speeds[i] == 0 {
					continue
				}
				pos := Point{X: 1000 - 2000*int32(i%2), Y: 500 * int32(i/2)}
				b.world.Apply(NPCInfo{ObjectID: id, NPCID: 20001, Attackable: true, Pos: pos, RunSpeed: tt.speeds[i], Running: true}, t0)
				// npcID hits hardest.
				b.world.Apply(Attack{AttackerID: id, Pos: pos, Hits: []AttackHit{{TargetID: selfID, Damage: 10 - int32(i)}}}, t0)
			}
			b.world.Target = npcID
			if tt.wantKind == actSelect {
				b.world.Target = 0
			}
			if tt.setup != nil {
				tt.setup(b)
			}
			if tt.wantKind == actMove {
				onGround(t, b)
			}
			rule, a := b.decide(now)
			if rule != tt.wantRule || a.kind != tt.wantKind {
				t.Fatalf("decide = %q %v, want %q %v", rule, a.kind, tt.wantRule, tt.wantKind)
			}
		})
	}
}

// TestPotionsThroughAFight checks that a bot held in a fight by a faster
// pack drinks a potion each potionReuse while its HP stays under the
// default CrowdedHP, and never runs: 30 s of two werewolves hitting it at
// 45 % HP is three potions.
func TestPotionsThroughAFight(t *testing.T) {
	const npc2 = int32(0x20000005)
	d, th := DefaultPersona(), defaultThresholds
	b, _ := newTestBrain(t, Persona{
		CriticalHP: th.CriticalHP, CrowdedHP: th.CrowdedHP, OutmatchedHP: th.OutmatchedHP,
		FleeDistance: d.FleeDistance, FleeOutrun: d.FleeOutrun, Potions: []int32{healingPotion},
	})
	b.world.Apply(ItemList{Items: []Item{{ObjectID: 0x30000011, TemplateID: healingPotion, Count: 13}}}, t0)
	b.world.Self.InCombat, b.world.Self.HP = true, 45
	wolves := map[int32]Point{npcID: {X: 40}, npc2: {X: -40}}
	for id, pos := range wolves {
		b.world.Apply(NPCInfo{ObjectID: id, NPCID: 20342, Attackable: true, Pos: pos, RunSpeed: 118, Running: true}, t0)
	}
	var drinks []time.Duration
	for s := 1; s <= 30; s++ {
		now := at(time.Duration(s) * time.Second)
		for id, pos := range wolves {
			b.world.Apply(Attack{AttackerID: id, Pos: pos, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, now)
		}
		rule, a := b.decide(now)
		switch a.kind {
		case actMove:
			t.Fatalf("at %ds: rule %q runs away from a faster pack", s, rule)
		case actUseItem:
			if _, err := b.send(a, now); err != nil {
				t.Fatal(err)
			}
			drinks = append(drinks, now.Sub(t0))
		}
	}
	want := []time.Duration{time.Second, time.Second + potionReuse, time.Second + 2*potionReuse}
	if len(drinks) != len(want) {
		t.Fatalf("drank at %v, want %v", drinks, want)
	}
	for i := range want {
		if drinks[i] != want[i] {
			t.Fatalf("drank at %v, want %v", drinks, want)
		}
	}
}
