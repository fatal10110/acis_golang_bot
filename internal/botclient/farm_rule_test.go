package botclient

import (
	"testing"
	"time"
)

const (
	// mob2ID is a second monster, peacefulID an NPC no one may attack.
	mob2ID     = int32(0x20000011)
	peacefulID = int32(0x20000012)
)

// TestRuleFarm checks the farm errand at the spot: attackers before fresh
// targets, the nearest fresh target, and a monster another player fights
// taken all the same (kill stealing is allowed, #138).
func TestRuleFarm(t *testing.T) {
	now := at(time.Minute)
	const far = int32(0x20000021)
	// outsiderFights has another player, outside the bot's party, swing at
	// monster id a second before now.
	outsiderFights := func(id int32) func(b *Brain) {
		return func(b *Brain) {
			b.world.Apply(Attack{AttackerID: pcID, Hits: []AttackHit{{TargetID: id}}}, now.Add(-time.Second))
		}
	}
	tests := []struct {
		name     string
		setup    func(b *Brain)
		wantRule string
		wantKind actionKind
		// wantObject is the creature selected or attacked.
		wantObject int32
	}{
		{name: "nearest fresh monster", wantRule: "farm", wantKind: actSelect, wantObject: npcID},
		{name: "another player's monster: the nearest all the same", setup: outsiderFights(npcID), wantRule: "farm", wantKind: actSelect, wantObject: npcID},
		{
			name:     "every monster fought by another player: the nearest all the same",
			setup:    func(b *Brain) { outsiderFights(npcID)(b); outsiderFights(mob2ID)(b) },
			wantRule: "farm", wantKind: actSelect, wantObject: npcID,
		},
		{
			name: "an attacker before fresh targets",
			setup: func(b *Brain) {
				b.world.Apply(Attack{AttackerID: mob2ID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, now)
			},
			wantRule: "farm", wantKind: actSelect, wantObject: mob2ID,
		},
		{
			name: "in combat an attacker another player fights is fought back",
			setup: func(b *Brain) {
				outsiderFights(npcID)(b)
				b.world.Apply(Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, now)
				b.world.Self.InCombat = true
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: npcID,
		},
		{
			name:     "selected target attacked",
			setup:    func(b *Brain) { b.world.Target = mob2ID },
			wantRule: "farm", wantKind: actAttack, wantObject: mob2ID,
		},
		{
			name: "selected target another player fights: attacked",
			setup: func(b *Brain) {
				b.world.Target = npcID
				outsiderFights(npcID)(b)
			},
			wantRule: "farm", wantKind: actAttack, wantObject: npcID,
		},
		{
			name: "own fight kept when another player joins it",
			setup: func(b *Brain) {
				b.world.Target, b.engaged, b.progressAt = mob2ID, mob2ID, now
				outsiderFights(mob2ID)(b)
			},
			wantRule: "farm", wantKind: actHold,
		},
		{
			name:     "blacklisted: the next one",
			setup:    func(b *Brain) { b.blacklist[npcID] = now.Add(time.Minute) },
			wantRule: "farm", wantKind: actSelect, wantObject: mob2ID,
		},
		{
			name: "beyond the leash: none",
			setup: func(b *Brain) {
				delete(b.world.Objects, npcID)
				delete(b.world.Objects, mob2ID)
				b.world.Apply(NPCInfo{ObjectID: far, NPCID: 20003, Attackable: true, Pos: Point{X: 1600}}, now)
			},
			wantRule: "",
		},
		{
			name: "a flagged player's summon is no farm target",
			setup: func(b *Brain) {
				delete(b.world.Objects, npcID)
				delete(b.world.Objects, mob2ID)
				b.world.Apply(NPCInfo{ObjectID: far, NPCID: 12077, Attackable: true, Summon: true, Pos: Point{X: 200}}, now)
			},
			wantRule: "",
		},
		{
			name:     "sitting stands up first",
			setup:    func(b *Brain) { b.world.Self.Sitting = true },
			wantRule: "farm", wantKind: actSitStand,
		},
		{
			name:     "not at the spot yet: walks there",
			setup:    func(b *Brain) { b.spot.arrived = false; b.world.Self.move = stopAt(Point{X: 1000}) },
			wantRule: "spot", wantKind: actMove,
		},
		{
			name:     "no spot registry: no farming",
			setup:    func(b *Brain) { b.cfg.Spots = nil },
			wantRule: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 5, testSpot("s", 0, 0, 3, 6, 9))
			b.cfg.Spots.claim([]int{0})
			b.spot.idx, b.spot.held, b.spot.arrived = 0, true, true
			b.world.Objects = map[int32]*Object{}
			b.world.Apply(CharInfo{ObjectID: pcID, Name: "other", Pos: Point{Y: 1000}}, t0)
			b.world.Apply(NPCInfo{ObjectID: peacefulID, NPCID: 30001, Pos: Point{X: 100}}, t0)
			b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 300}}, t0)
			b.world.Apply(NPCInfo{ObjectID: mob2ID, NPCID: 20002, Attackable: true, Pos: Point{X: 600}}, t0)
			if tt.setup != nil {
				tt.setup(b)
			}
			rule, a := b.decide(now)
			if rule != tt.wantRule || a.kind != tt.wantKind {
				t.Fatalf("decide = rule %q action %d, want rule %q action %d", rule, a.kind, tt.wantRule, tt.wantKind)
			}
			if a.object != tt.wantObject {
				t.Fatalf("action on %#x, want %#x", a.object, tt.wantObject)
			}
		})
	}
}
