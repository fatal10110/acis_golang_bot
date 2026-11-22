package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

const (
	engageMob  int32 = 9001
	engageItem int32 = 9002
	engageCast int32 = 1177 // Wind Strike
)

// TestFightOnStandPoints checks the fight's approach on the synthetic nav:
// an attack or a cast goes out only while the straight line to the target
// is open, else the bot walks through the walker to a stand point, and a
// target with none the bot reaches is blacklisted.
func TestFightOnStandPoints(t *testing.T) {
	n := testNav(t)
	tests := []struct {
		name     string
		self     Point
		mob      Point
		cast     bool
		wantKind actionKind
		wantWalk bool
		// wantReach is how far from the mob the walk's stand point may lie
		// (a melee one also has the straight line to it open).
		wantReach   int32
		wantBlocked bool
	}{
		{
			name: "melee, line open: attack", self: scenePoint(geotest.UBehind, 0, 0), mob: scenePoint(geotest.UBehind, 10, 0),
			wantKind: actAttack,
		},
		{
			name: "melee behind the U's base: walk to a stand point", self: scenePoint(geotest.UInside, 0, 0), mob: scenePoint(geotest.UBehind, 0, 0),
			wantKind: actMove, wantWalk: true, wantReach: meleeStand,
		},
		{
			name: "cast, line open: cast", self: scenePoint(geotest.UBehind, 0, 0), mob: scenePoint(geotest.UBehind, 20, 0), cast: true,
			wantKind: actCast,
		},
		{
			name: "cast behind the U's base, out of sight: walk to a stand point in sight", self: scenePoint(geotest.UInside, 0, 0), mob: scenePoint(geotest.UBehind, 0, 0), cast: true,
			wantKind: actMove, wantWalk: true, wantReach: rangedReach,
		},
		{
			name: "mob inside the walled pocket: blacklisted", self: scenePoint(geotest.PocketOutside, 0, 0), mob: scenePoint(geotest.PocketInside, 0, 0),
			wantKind: actHold, wantBlocked: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Persona{BlacklistFor: time.Minute}
			if tt.cast {
				p.AttackSkills = []int32{engageCast}
			}
			b, _, _ := newNavBrain(t, n, p, tt.self)
			if tt.cast {
				b.world.Skills = map[int32]KnownSkill{engageCast: {}}
			}
			b.world.Apply(NPCInfo{ObjectID: engageMob, NPCID: 20001, Attackable: true, Pos: tt.mob}, t0)
			b.world.Target = engageMob
			now := t0.Add(time.Second)
			a := b.fight(engageMob, now)
			if a.kind != tt.wantKind || a.walk != tt.wantWalk {
				t.Fatalf("fight = kind %v walk %v (%s), want kind %v walk %v", a.kind, a.walk, b.walk.reason, tt.wantKind, tt.wantWalk)
			}
			if tt.wantWalk {
				stand := b.engage.stand
				if d := horizontal2(stand, tt.mob); d > int64(tt.wantReach)*int64(tt.wantReach) {
					t.Errorf("stand point %+v %d away from the mob, want within %d", stand, d, tt.wantReach)
				}
				if !tt.cast && !b.walk.lineOpen(stand, tt.mob) {
					t.Errorf("melee stand point %+v has no open line to the mob", stand)
				}
				if b.walk.dest != stand {
					t.Errorf("walk to %+v, want the stand point %+v", b.walk.dest, stand)
				}
			}
			if blocked := now.Before(b.blacklist[engageMob]); blocked != tt.wantBlocked {
				t.Errorf("blacklisted %v, want %v", blocked, tt.wantBlocked)
			}
		})
	}
}

// TestFightWalksAgainWhenTheLineCloses has an engaged target step behind
// the U's base: the bot, standing still, walks to a new stand point, and the
// move drops the attack so it goes out again from there.
func TestFightWalksAgainWhenTheLineCloses(t *testing.T) {
	n := testNav(t)
	self := scenePoint(geotest.UInside, 0, 0)
	b, _, _ := newNavBrain(t, n, Persona{BlacklistFor: time.Minute}, self)
	b.world.Apply(NPCInfo{ObjectID: engageMob, NPCID: 20001, Attackable: true, Pos: scenePoint(geotest.UInside, 3, 0)}, t0)
	b.world.Target = engageMob
	now := t0.Add(time.Second)
	a := b.fight(engageMob, now)
	if a.kind != actAttack {
		t.Fatalf("line open: kind %v, want an attack", a.kind)
	}
	if ok, err := b.send(a, now); !ok || err != nil {
		t.Fatalf("send attack: %v %v", ok, err)
	}
	b.world.Apply(NPCInfo{ObjectID: engageMob, NPCID: 20001, Attackable: true, Pos: scenePoint(geotest.UBehind, 0, 0)}, now)
	now = now.Add(time.Second)
	a = b.fight(engageMob, now)
	if a.kind != actMove || !a.walk {
		t.Fatalf("line closed: kind %v walk %v (%s), want a walker move", a.kind, a.walk, b.walk.reason)
	}
	if ok, err := b.send(a, now); !ok || err != nil {
		t.Fatalf("send move: %v %v", ok, err)
	}
	if b.engaged != 0 {
		t.Errorf("engaged %d after the move, want 0: the attack goes out again", b.engaged)
	}
}

// TestLootOnStandPoints checks the pick-up on the synthetic nav: it goes
// out only while CanMove onto the item's raw Z and the step chain hold,
// else the bot walks to a pick-up stand point first; an item none reaches
// is given up.
func TestLootOnStandPoints(t *testing.T) {
	n := testNav(t)
	tests := []struct {
		name     string
		self     Point
		item     Point
		wantKind actionKind
		wantWalk bool
		wantSkip bool
	}{
		{name: "open: pick up", self: scenePoint(geotest.UBehind, 0, 0), item: scenePoint(geotest.UBehind, 8, 0), wantKind: actPickup},
		{name: "behind the U's base: walk to a stand point", self: scenePoint(geotest.UInside, 0, 0), item: scenePoint(geotest.UBehind, 0, 0), wantKind: actMove, wantWalk: true},
		{name: "inside the walled pocket: given up", self: scenePoint(geotest.PocketOutside, 0, 0), item: scenePoint(geotest.PocketInside, 0, 0), wantSkip: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Persona{Loot: defaultLootPolicy()}
			b, _, _ := newNavBrain(t, n, p, tt.self)
			delete(b.world.Objects, npcID)
			b.world.Ground = map[int32]GroundItem{engageItem: {ObjectID: engageItem, TemplateID: 57, Pos: tt.item, Count: 1}}
			b.loot.items = []int32{engageItem}
			a, ok := b.ruleLoot(t0.Add(time.Second))
			if tt.wantSkip {
				if ok || len(b.loot.items) != 0 {
					t.Fatalf("ruleLoot = %v %v, queue %v, want the item given up", a.kind, ok, b.loot.items)
				}
				return
			}
			if !ok || a.kind != tt.wantKind || a.walk != tt.wantWalk {
				t.Fatalf("ruleLoot = kind %v walk %v ok %v (%s), want kind %v walk %v", a.kind, a.walk, ok, b.walk.reason, tt.wantKind, tt.wantWalk)
			}
			if tt.wantWalk {
				st := b.loot.stand
				if d := horizontal2(st, tt.item); d > pickupStand*pickupStand || !b.walk.pickUpOpen(st, tt.item) {
					t.Errorf("stand point %+v (%d away) does not hold the pick-up", st, d)
				}
			}
		})
	}
}
