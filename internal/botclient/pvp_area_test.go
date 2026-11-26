package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The area tests' fixtures: a pole (kind POLE) and a monster.
const (
	grdPoleID  = int32(9201)
	grdPoleObj = int32(0x30000202)
	grdMob     = int32(0x20000006)
)

// wieldPole puts a pole in the bot's hands.
func wieldPole(b *Brain) {
	b.cfg.Travel.gear = map[int32]gearItem{grdPoleID: {part: partRHand, kind: "POLE", tmpl: datapack.Item{Kind: "POLE"}}}
	b.world.Apply(ItemList{Items: []Item{{ObjectID: grdPoleObj, TemplateID: grdPoleID, Count: 1, Equipped: true, BodySlot: partRHand}}}, t0)
}

// place stands id at p.
func place(t *testing.T, b *Brain, id int32, p Point) {
	t.Helper()
	b.handleOne(t, StopMove{ObjectID: id, Pos: p}, t0)
}

// player puts a player id named name at p with flag, karma and radius.
func player(t *testing.T, b *Brain, id int32, name string, p Point, flag uint8, karma int32, radius float64) {
	t.Helper()
	b.handleOne(t, CharInfo{ObjectID: id, Name: name, Pos: p, RunSpeed: 124, PvPFlag: int32(flag), Karma: karma, CollisionRadius: radius}, t0)
	place(t, b, id, p)
}

// summonAt puts the summon pvpSummon of owner at p with flag and karma.
func summonAt(t *testing.T, b *Brain, owner string, p Point, flag uint8, karma int32) {
	t.Helper()
	b.handleOne(t, NPCInfo{ObjectID: pvpSummon, NPCID: 12077, Summon: true, Attackable: true, Title: owner, Pos: p, CollisionRadius: 9}, t0)
	b.handleOne(t, RelationChanged{ObjectID: pvpSummon, PvPFlag: flag, Karma: karma}, t0)
	place(t, b, pvpSummon, p)
}

// areaBrain is guardBrain wielding a pole, its target pcID (flag 1, unless
// in a duel) at target and the kit of steps.
func areaBrain(t *testing.T, arena, duel bool, target Point, steps ...Step) *Brain {
	t.Helper()
	flag := uint8(1)
	if duel {
		flag = 0
	}
	b := guardBrain(t, arena, flag, 0, steps...)
	wieldPole(b)
	if duel {
		duelUntil(t, b, DuelFighting, t0)
		b.reactAt = time.Time{}
	}
	place(t, b, pcID, target)
	return b
}

// TestPvPAreaBystander checks the area rule (docs/pvp/guards.md "Area hits
// and bystanders") on Wild Sweep (AREA, radius 150) and a pole (80, the
// 120° cone), radii 9 unless named.
func TestPvPAreaBystander(t *testing.T) {
	sweep := Step{Skill: skillWildSweep}
	near, far := Point{X: 40}, Point{X: 60}
	tests := []struct {
		name         string
		arena, duel  bool
		target       Point
		setup        func(t *testing.T, b *Brain)
		kind         actionKind
		skill        int32
		notAttack    bool
		notSweepCast bool
	}{
		{
			name: "a flagged bystander 165 beyond the target: no Wild Sweep, the swing goes", target: near,
			setup: func(t *testing.T, b *Brain) { player(t, b, pvpThird, "x", Point{X: 205}, 1, 0, 9) },
			kind:  actAttack,
		},
		{
			name: "175 beyond: Wild Sweep", target: near,
			setup: func(t *testing.T, b *Brain) { player(t, b, pvpThird, "x", Point{X: 215}, 1, 0, 9) },
			kind:  actCast, skill: skillWildSweep,
		},
		{
			name: "a male Orc Fighter (11) 169 beyond: no Wild Sweep", target: near,
			setup: func(t *testing.T, b *Brain) { player(t, b, pvpThird, "x", Point{X: 209}, 1, 0, 11) },
			kind:  actAttack,
		},
		{
			name: "a female Dwarven Fighter (5) 165 beyond: Wild Sweep", target: near,
			setup: func(t *testing.T, b *Brain) { player(t, b, pvpThird, "x", Point{X: 205}, 1, 0, 5) },
			kind:  actCast, skill: skillWildSweep,
		},
		{
			name: "the bystander with karma: Wild Sweep", target: near,
			setup: func(t *testing.T, b *Brain) { player(t, b, pvpThird, "x", Point{X: 205}, 1, 20, 9) },
			kind:  actCast, skill: skillWildSweep,
		},
		{
			name: "a flagged party member: Wild Sweep", target: near,
			setup: func(t *testing.T, b *Brain) {
				player(t, b, pvpThird, "x", Point{X: 205}, 1, 0, 9)
				b.world.Apply(PartySmallWindowAll{LeaderID: pvpThird, Members: []PartyMember{{ObjectID: pvpThird, HP: 100, MaxHP: 100}}}, t0)
			},
			kind: actCast, skill: skillWildSweep,
		},
		{
			name: "a flagged karma-free summon 165 beyond: no Wild Sweep", target: near,
			setup: func(t *testing.T, b *Brain) { summonAt(t, b, "nobody", Point{X: 205}, 1, 0) },
			kind:  actAttack,
		},
		{
			name: "the summon with karma: Wild Sweep", target: near,
			setup: func(t *testing.T, b *Brain) { summonAt(t, b, "nobody", Point{X: 205}, 1, 5) },
			kind:  actCast, skill: skillWildSweep,
		},
		{
			name: "the summon clean: Wild Sweep", target: near,
			setup: func(t *testing.T, b *Brain) { summonAt(t, b, "nobody", Point{X: 205}, 0, 0) },
			kind:  actCast, skill: skillWildSweep,
		},
		{
			name: "the pole at a monster with a flagged player in its cone: the swing goes",
			setup: func(t *testing.T, b *Brain) {
				b.handleOne(t, NPCInfo{ObjectID: grdMob, NPCID: 20001, Attackable: true, Pos: far, CollisionRadius: 9}, t0)
				place(t, b, grdMob, far)
				b.handleOne(t, hitSelf(grdMob, 5), t0)
				b.handleOne(t, DeleteObject{ObjectID: pcID}, t0)
				b.world.Target = grdMob
				player(t, b, pvpThird, "x", Point{X: 40, Y: 10}, 1, 0, 9)
			},
			kind: actAttack,
		},
		{
			name: "a flagged bystander 95 from the bot in the cone: no swing", target: far,
			setup:     func(t *testing.T, b *Brain) { player(t, b, pvpThird, "x", Point{X: 95, Y: 5}, 1, 0, 9) },
			notAttack: true,
		},
		{
			name: "arena: a summon whose owner is seen outside: no Wild Sweep", arena: true, target: near,
			setup: func(t *testing.T, b *Brain) {
				player(t, b, pvpThird, "owner", Point{X: 2500}, 0, 0, 9)
				summonAt(t, b, "owner", Point{X: 205}, 0, 0)
			},
			kind: actAttack,
		},
		{
			name: "arena: a summon whose owner is inside by more than 150: Wild Sweep", arena: true, target: near,
			setup: func(t *testing.T, b *Brain) {
				player(t, b, pvpThird, "owner", Point{X: 1000, Y: 1000}, 0, 0, 9)
				summonAt(t, b, "owner", Point{X: 205}, 0, 0)
			},
			kind: actCast, skill: skillWildSweep,
		},
		{
			name: "arena: target 160 inside, a flagged player 165 from it just outside: no Wild Sweep", arena: true, target: Point{X: 1840},
			setup: func(t *testing.T, b *Brain) {
				b.world.Self.move = stopAt(Point{X: 1800})
				player(t, b, pvpThird, "x", Point{X: 2005}, 1, 0, 9)
			},
			notSweepCast: true,
		},
		{
			name: "arena: an unflagged player 100 inside: no Wild Sweep", arena: true, target: Point{X: 1840},
			setup: func(t *testing.T, b *Brain) {
				b.world.Self.move = stopAt(Point{X: 1800})
				player(t, b, pvpThird, "x", Point{X: 1900}, 0, 0, 9)
			},
			notSweepCast: true,
		},
		{
			name: "arena: an unflagged player 200 inside: Wild Sweep", arena: true, target: Point{X: 1840},
			setup: func(t *testing.T, b *Brain) {
				b.world.Self.move = stopAt(Point{X: 1800})
				player(t, b, pvpThird, "x", Point{X: 1800, Y: 100}, 0, 0, 9)
			},
			kind: actCast, skill: skillWildSweep,
		},
		{
			name: "arena: a player with karma outside: Wild Sweep", arena: true, target: Point{X: 1840},
			setup: func(t *testing.T, b *Brain) {
				b.world.Self.move = stopAt(Point{X: 1800})
				player(t, b, pvpThird, "x", Point{X: 2005}, 1, 10, 9)
			},
			kind: actCast, skill: skillWildSweep,
		},
		{
			name: "arena: an unflagged player deep inside: Wild Sweep", arena: true, target: near,
			setup: func(t *testing.T, b *Brain) { player(t, b, pvpThird, "x", Point{X: 205}, 0, 0, 9) },
			kind:  actCast, skill: skillWildSweep,
		},
		{
			name: "duel: an unflagged third player 165 beyond: no Wild Sweep, the swing goes", duel: true, target: near,
			setup: func(t *testing.T, b *Brain) { player(t, b, pvpThird, "x", Point{X: 205}, 0, 0, 9) },
			kind:  actAttack,
		},
		{
			name: "duel: a monster 165 beyond: no Wild Sweep", duel: true, target: near,
			setup: func(t *testing.T, b *Brain) {
				b.handleOne(t, NPCInfo{ObjectID: grdMob, NPCID: 20001, Attackable: true, Pos: Point{X: 205}, CollisionRadius: 9}, t0)
				place(t, b, grdMob, Point{X: 205})
			},
			kind: actAttack,
		},
		{
			name: "duel: an NPC of radius 30 at 115 in the cone: the reposition move", duel: true, target: near,
			setup: func(t *testing.T, b *Brain) {
				b.handleOne(t, NPCInfo{ObjectID: grdMob, NPCID: 20001, Attackable: true, Pos: Point{X: 115}, CollisionRadius: 30}, t0)
				place(t, b, grdMob, Point{X: 115})
			},
			kind: actMove,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := areaBrain(t, tc.arena, tc.duel, tc.target, sweep)
			if tc.setup != nil {
				tc.setup(t, b)
			}
			now := at(time.Second)
			if tc.duel {
				now = at(7 * time.Second)
			}
			_, a := b.decide(now)
			switch {
			case tc.notAttack:
				if a.kind == actAttack {
					t.Fatal("swung with a bystander in the cone")
				}
			case tc.notSweepCast:
				if a.kind == actCast && a.skill == skillWildSweep {
					t.Fatal("Wild Sweep cast with a bystander in reach")
				}
			case a.kind != tc.kind || tc.kind == actCast && a.skill != tc.skill:
				t.Fatalf("decision %v skill %d, want %v skill %d", a.kind, a.skill, tc.kind, tc.skill)
			}
		})
	}
}

// TestPvPPoleReposition checks a pole wielder's answer to a flagged
// karma-free bystander in its cone (docs/pvp/guards.md "Area hits and
// bystanders"): no Wild Sweep, swing or nextActionAttack step; the
// disengage move, a walk to a stand point with the bystander out of the
// cone, then Action; with a bystander next to the target, the self buff
// and a hold until it leaves, then the damage steps again.
func TestPvPPoleReposition(t *testing.T) {
	t.Run("reposition", func(t *testing.T) {
		b := areaBrain(t, false, false, Point{X: 90}, Step{Skill: skillWildSweep})
		player(t, b, pvpThird, "x", Point{X: 40, Y: 10}, 1, 0, 9)
		b.world.Apply(SkillList{Skills: []KnownSkill{{ID: skillWildSweep, Level: 1}, {ID: grdSmash, Level: 1}}}, t0)
		b.engaged, b.pvp.guard.intent = pcID, pcID
		now := at(time.Second)
		if !b.stepHeld(pcID, grdSmash, now) {
			t.Error("a nextActionAttack step goes with a bystander in the pole's cone")
		}
		play(t, b, []beat{{at: time.Second, kind: actMove, rule: "guard"}})
		rule, a := b.decide(at(1200 * time.Millisecond))
		if rule != "attacked" {
			t.Fatalf("rule %q, want attacked", rule)
		}
		st := moveDest(t, a)
		if b.coneHit(pcID, st, Point{X: 90}, now) {
			t.Fatalf("stand point %v has the bystander in its cone", st)
		}
		b.world.Self.move = stopAt(st)
		if _, a := b.decide(at(2 * time.Second)); a.kind != actAttack {
			t.Fatalf("at the stand point: %v, want the attack", a.kind)
		}
	})
	t.Run("hold", func(t *testing.T) {
		b := areaBrain(t, false, false, Point{X: 60}, Step{Skill: grdWarCry, Target: TargetSelf}, Step{Skill: skillWildSweep})
		player(t, b, pvpThird, "x", Point{X: 60, Y: 15}, 1, 0, 9)
		play(t, b, []beat{
			{at: time.Second, kind: actCast, skill: grdWarCry},
			{at: 1100 * time.Millisecond, ev: MagicSkillUse{CasterID: selfID, TargetID: selfID, SkillID: grdWarCry, Level: 1}, noDecide: true},
			{at: 1100 * time.Millisecond, ev: AbnormalStatusUpdate{Effects: []Effect{{SkillID: grdWarCry, Level: 1, Left: time.Hour}}}, noDecide: true},
			{at: 1200 * time.Millisecond, kind: actHold},
			{at: 2 * time.Second, ev: DeleteObject{ObjectID: pvpThird}, kind: actCast, skill: skillWildSweep},
		})
	})
}
