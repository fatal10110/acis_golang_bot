package botclient

import (
	"bytes"
	"testing"
	"time"
)

// TestCombatEncoders pins the combat client packets to the server decoders
// (clientpackets targetaction.go, magic_skill_use.go).
func TestCombatEncoders(t *testing.T) {
	from := Point{X: -1, Y: 2, Z: 3}
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"AttackRequest", encodeAttackRequest(0x10203040, from), le(byte(0x0a), 0x10203040, -1, 2, 3, byte(0))},
		{"RequestMagicSkillUse", encodeRequestMagicSkillUse(1177, false), le(byte(0x2f), 1177, 0, byte(0))},
		{"RequestMagicSkillUse forced", encodeRequestMagicSkillUse(1177, true), le(byte(0x2f), 1177, 1, byte(0))},
	}
	for _, tc := range tests {
		if !bytes.Equal(tc.got, tc.want) {
			t.Errorf("%s\n got % x\nwant % x", tc.name, tc.got, tc.want)
		}
	}
}

// TestWorldCombat checks what the world model keeps of combat events:
// attackers with their damage streaks, the bot's own casts and skill reuse,
// and players' PvP state.
func TestWorldCombat(t *testing.T) {
	hit := func(attacker, target, dmg int32, flags uint8) Attack {
		return Attack{AttackerID: attacker, Hits: []AttackHit{{TargetID: target, Damage: dmg, Flags: flags}}}
	}
	type step struct {
		at time.Duration
		ev Event
	}
	tests := []struct {
		name  string
		setup func(w *World)
		steps []step
		check func(t *testing.T, w *World)
	}{
		{
			name:  "hits on the bot add up per attacker, misses included",
			steps: []step{{0, hit(npcID, selfID, 10, 0)}, {time.Second, hit(npcID, selfID, 0, HitMiss)}, {2 * time.Second, hit(npcID, selfID, 15, 0)}},
			check: func(t *testing.T, w *World) {
				if a := w.Attackers[npcID]; a == nil || a.Damage != 25 || !a.Last.Equal(at(2*time.Second)) {
					t.Fatalf("attacker = %+v, want 25 damage, last at 2s", a)
				}
				if !w.attacking(npcID, at(12*time.Second)) || w.attacking(npcID, at(13*time.Second)) {
					t.Error("attacker window is not 10s after the last hit")
				}
			},
		},
		{
			name:  "a streak starts over after the window",
			steps: []step{{0, hit(npcID, selfID, 10, 0)}, {11 * time.Second, hit(npcID, selfID, 3, 0)}},
			check: func(t *testing.T, w *World) {
				if a := w.Attackers[npcID]; a == nil || a.Damage != 3 {
					t.Fatalf("attacker = %+v, want 3 damage", a)
				}
			},
		},
		{
			name:  "hits on others and the bot's own swings are no attackers",
			steps: []step{{0, hit(npcID, pcID, 10, 0)}, {0, hit(selfID, npcID, 10, 0)}},
			check: func(t *testing.T, w *World) {
				if len(w.Attackers) != 0 {
					t.Fatalf("attackers = %v, want none", w.Attackers)
				}
			},
		},
		{
			name:  "a monster's cast at the bot is an attack",
			steps: []step{{0, MagicSkillUse{CasterID: npcID, TargetID: selfID, SkillID: 4001}}},
			check: func(t *testing.T, w *World) {
				if !w.attacking(npcID, t0) {
					t.Fatal("monster cast not counted")
				}
			},
		},
		{
			name:  "an unflagged player's cast at the bot is not",
			steps: []step{{0, MagicSkillUse{CasterID: pcID, TargetID: selfID, SkillID: 1204}}},
			check: func(t *testing.T, w *World) {
				if w.attacking(pcID, t0) {
					t.Fatal("buff counted as an attack")
				}
			},
		},
		{
			// The brain tells an offensive cast from the datapack
			// (Brain.castSeen, TestPvPHostileCast).
			name:  "a flagged player's cast at the bot is left to the brain",
			steps: []step{{0, RelationChanged{ObjectID: pcID, PvPFlag: 1}}, {0, MagicSkillUse{CasterID: pcID, TargetID: selfID, SkillID: 1177}}},
			check: func(t *testing.T, w *World) {
				if w.attacking(pcID, t0) || w.Objects[pcID].PvPFlag != 1 {
					t.Fatal("the world counted a player's cast")
				}
			},
		},
		{
			name:  "death and removal forget attackers",
			steps: []step{{0, hit(npcID, selfID, 1, 0)}, {0, hit(pcID, selfID, 1, 0)}, {0, Die{ObjectID: npcID}}, {0, DeleteObject{ObjectID: pcID}}},
			check: func(t *testing.T, w *World) {
				if len(w.Attackers) != 0 {
					t.Fatalf("attackers = %v, want none", w.Attackers)
				}
			},
		},
		{
			name:  "the bot's own death forgets attackers",
			steps: []step{{0, hit(npcID, selfID, 1, 0)}, {0, Die{ObjectID: selfID}}},
			check: func(t *testing.T, w *World) {
				if len(w.Attackers) != 0 {
					t.Fatalf("attackers = %v, want none", w.Attackers)
				}
			},
		},
		{
			name: "own cast: casting for the hit time, skill held for hit time plus reuse",
			steps: []step{
				{0, SkillList{Skills: []KnownSkill{{ID: 1177, Level: 3}}}},
				{time.Second, MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: 1177, HitTime: 2 * time.Second, Reuse: 5 * time.Second}},
			},
			check: func(t *testing.T, w *World) {
				if !w.Self.Casting(at(2999*time.Millisecond)) || w.Self.Casting(at(3*time.Second)) {
					t.Error("cast does not end at the hit time")
				}
				if w.SkillReady(1177, at(7999*time.Millisecond)) || !w.SkillReady(1177, at(8*time.Second)) {
					t.Error("skill not ready exactly hit time plus reuse after the cast")
				}
			},
		},
		{
			name: "SkillCoolTime holds a skill before or after the list",
			steps: []step{
				{0, SkillCoolTime{Timers: []SkillTimer{{SkillID: 1177, Remaining: 4 * time.Second}}}},
				{time.Second, SkillList{Skills: []KnownSkill{{ID: 1177}, {ID: 2, Passive: true}, {ID: 3, Disabled: true}}}},
			},
			check: func(t *testing.T, w *World) {
				// 4 s remaining may be up to 4.999 s: the server truncates.
				if w.SkillReady(1177, at(4999*time.Millisecond)) || !w.SkillReady(1177, at(5*time.Second)) {
					t.Error("cool time not kept and rounded up")
				}
				if w.SkillReady(2, t0) || w.SkillReady(3, t0) || w.SkillReady(99, t0) {
					t.Error("a passive, disabled or unknown skill is ready")
				}
			},
		},
		{
			name: "a new list drops forgotten skills",
			steps: []step{
				{0, SkillList{Skills: []KnownSkill{{ID: 1177}}}},
				{0, SkillList{Skills: []KnownSkill{{ID: 1178}}}},
			},
			check: func(t *testing.T, w *World) {
				if w.SkillReady(1177, t0) || !w.SkillReady(1178, t0) {
					t.Error("skill list not replaced")
				}
			},
		},
		{
			name:  "RelationChanged updates a player's PvP state",
			steps: []step{{0, RelationChanged{ObjectID: pcID, Karma: 50, PvPFlag: 1}}, {time.Second, RelationChanged{ObjectID: pcID, PvPFlag: 0}}},
			check: func(t *testing.T, w *World) {
				if o := w.Objects[pcID]; o.PvPFlag != 0 || o.Karma != 0 {
					t.Fatalf("player flag %d karma %d, want 0 0", o.PvPFlag, o.Karma)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newTestWorld(t)
			for _, s := range tc.steps {
				w.Apply(s.ev, at(s.at))
			}
			tc.check(t, w)
		})
	}
}
