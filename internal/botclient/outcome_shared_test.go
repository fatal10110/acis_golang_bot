package botclient

import (
	"testing"
	"time"
)

// TestOutcomeShared checks which kills the record counts shared: another
// player's or a summon's hit or cast before or after the bot's first hit,
// but not a monster's cast on itself or on another monster, nor a hit
// before a teleport away.
func TestOutcomeShared(t *testing.T) {
	const (
		stayHealer = int32(0x7000003)
		staySummon = int32(0x7000004)
	)
	tests := []struct {
		name   string
		before []Event // at 5s, before the bot's first hit at 10s
		away   bool    // a teleport away and back between before and the hit
		after  []Event // at 15s, after the bot's first hit
		shared int
	}{
		{"solo", nil, false, nil, 0},
		{"player hits first", []Event{Attack{AttackerID: pcID, Hits: []AttackHit{{TargetID: stayMob1, Damage: 10}}}}, false, nil, 1},
		{"player casts first", []Event{MagicSkillUse{CasterID: pcID, TargetID: stayMob1}}, false, nil, 1},
		{"player hits after", nil, false, []Event{Attack{AttackerID: pcID, Hits: []AttackHit{{TargetID: stayMob1, Damage: 10}}}}, 1},
		{"summon hits first", []Event{Attack{AttackerID: staySummon, Hits: []AttackHit{{TargetID: stayMob1, Damage: 10}}}}, false, nil, 1},
		{"self-cast", []Event{MagicSkillUse{CasterID: stayMob1, TargetID: stayMob1}}, false, []Event{MagicSkillUse{CasterID: stayMob1, TargetID: stayMob1}}, 0},
		{"monster heals it", []Event{MagicSkillUse{CasterID: stayHealer, TargetID: stayMob1}}, false, []Event{MagicSkillUse{CasterID: stayHealer, TargetID: stayMob1}}, 0},
		{"player hit before a teleport", []Event{Attack{AttackerID: pcID, Hits: []AttackHit{{TargetID: stayMob1, Damage: 10}}}}, true, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, q, r := newStayBrain(t)
			self := b.world.Self.ObjectID
			r.arrive(0)
			r.at(0, NPCInfo{ObjectID: stayMob1, NPCID: 20001, Attackable: true, Pos: Point{X: 50}},
				NPCInfo{ObjectID: stayHealer, NPCID: 20002, Pos: Point{X: 80}},
				NPCInfo{ObjectID: staySummon, NPCID: 12077, Summon: true, Pos: Point{X: 20}})
			r.at(5*time.Second, tt.before...)
			if tt.away {
				r.at(6*time.Second, TeleportToLocation{ObjectID: self, Pos: Point{X: 4000}, Fast: true})
				r.at(8*time.Second, TeleportToLocation{ObjectID: self, Pos: Point{}, Fast: true})
				r.arrive(8 * time.Second)
			}
			r.at(10*time.Second, Attack{AttackerID: self, Hits: []AttackHit{{TargetID: stayMob1, Damage: 10}}})
			r.at(15*time.Second, tt.after...)
			r.at(30*time.Second, Die{ObjectID: stayMob1})
			r.clockTo(time.Minute)
			b.leaveSpot()
			got := drainOutcomes(q)
			if len(got) != 1 {
				t.Fatalf("records = %d, want 1: %+v", len(got), got)
			}
			o := got[0]
			if o.Kills != 1 || o.Shared != tt.shared || o.KillSeconds != 20 {
				t.Errorf("kills %d shared %d kill_seconds %v; want 1, %d, 20", o.Kills, o.Shared, o.KillSeconds, tt.shared)
			}
			if wantSolo := float64(20 * (1 - tt.shared)); o.SoloKillSeconds != wantSolo {
				t.Errorf("solo_kill_seconds %v, want %v", o.SoloKillSeconds, wantSolo)
			}
			if len(b.stay.others) != 0 {
				t.Errorf("others left after the stay: %v", b.stay.others)
			}
		})
	}
}

// TestOutcomeSharedForget checks that a monster another attacker hit is
// forgotten when it dies or leaves sight before the bot hits it, so a
// reused object id starts solo.
func TestOutcomeSharedForget(t *testing.T) {
	for _, gone := range []Event{Die{ObjectID: stayMob1}, DeleteObject{ObjectID: stayMob1}} {
		b, _, r := newStayBrain(t)
		r.arrive(0)
		r.at(0, NPCInfo{ObjectID: stayMob1, NPCID: 20001, Attackable: true, Pos: Point{X: 50}})
		r.at(5*time.Second, Attack{AttackerID: pcID, Hits: []AttackHit{{TargetID: stayMob1, Damage: 10}}})
		if _, ok := b.stay.others[stayMob1]; !ok {
			t.Fatalf("%T: monster not noted after the player's hit", gone)
		}
		r.at(6*time.Second, gone)
		if len(b.stay.others) != 0 {
			t.Errorf("%T: others = %v, want empty", gone, b.stay.others)
		}
	}
}
