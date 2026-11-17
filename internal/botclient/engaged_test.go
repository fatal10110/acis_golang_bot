package botclient

import (
	"cmp"
	"testing"
	"time"
)

const (
	// mob2ID is a second monster, peacefulID an NPC no one may attack.
	mob2ID     = int32(0x20000011)
	peacefulID = int32(0x20000012)
)

// TestWorldEngaged checks the anti-KS record: who fought which monster,
// and for how long it counts.
func TestWorldEngaged(t *testing.T) {
	type step struct {
		at time.Duration
		ev Event
	}
	hit := func(target int32) []AttackHit { return []AttackHit{{TargetID: target}} }
	party := func(w *World) {
		w.Party = Party{LeaderID: selfID, Members: map[int32]*PartyMate{pcID: {}}}
	}
	const outsider = int32(0x20000013)
	const summonID = int32(0x20000014)
	summon := func(w *World) {
		w.Apply(NPCInfo{ObjectID: summonID, NPCID: 12077, Attackable: true, Summon: true, Pos: Point{X: 900}}, t0)
	}
	tests := []struct {
		name  string
		setup func(w *World)
		steps []step
		// mob is the monster asked about, npcID when 0.
		mob   int32
		check time.Duration
		want  bool
	}{
		{name: "nothing seen", check: time.Second},
		{name: "monster chasing a player", steps: []step{{0, MoveToPawn{ObjectID: npcID, TargetID: pcID}}}, check: time.Second, want: true},
		{name: "player closing in on a monster", steps: []step{{0, MoveToPawn{ObjectID: pcID, TargetID: npcID}}}, check: time.Second, want: true},
		{name: "player hits a monster", steps: []step{{0, Attack{AttackerID: pcID, Hits: hit(npcID)}}}, check: time.Second, want: true},
		{name: "monster hits a player", steps: []step{{0, Attack{AttackerID: npcID, Hits: hit(pcID)}}}, check: time.Second, want: true},
		{name: "unknown player hits a monster", steps: []step{{0, Attack{AttackerID: outsider, Hits: hit(npcID)}}}, check: time.Second, want: true},
		{
			name:  "pole splash engages the second monster",
			steps: []step{{0, Attack{AttackerID: pcID, Hits: []AttackHit{{TargetID: npcID}, {TargetID: mob2ID}}}}},
			mob:   mob2ID, check: time.Second, want: true,
		},
		{name: "player casts at a monster", steps: []step{{0, MagicSkillUse{CasterID: pcID, TargetID: npcID}}}, check: time.Second, want: true},
		{name: "monster casts at a player", steps: []step{{0, MagicSkillUse{CasterID: npcID, TargetID: pcID}}}, check: time.Second, want: true},
		{name: "still engaged at the window's end", steps: []step{{0, Attack{AttackerID: pcID, Hits: hit(npcID)}}}, check: 10 * time.Second, want: true},
		{name: "expired after the window", steps: []step{{0, Attack{AttackerID: pcID, Hits: hit(npcID)}}}, check: 10*time.Second + time.Millisecond},
		{
			name:  "a later sign renews it",
			steps: []step{{0, Attack{AttackerID: pcID, Hits: hit(npcID)}}, {8 * time.Second, MoveToPawn{ObjectID: npcID, TargetID: pcID}}},
			check: 15 * time.Second, want: true,
		},
		{name: "the bot's own swing", steps: []step{{0, Attack{AttackerID: selfID, Hits: hit(npcID)}}}, check: time.Second},
		{name: "the bot's own cast", steps: []step{{0, MagicSkillUse{CasterID: selfID, TargetID: npcID}}}, check: time.Second},
		{name: "monster hits the bot", steps: []step{{0, Attack{AttackerID: npcID, Hits: hit(selfID)}}}, check: time.Second},
		{name: "monster chasing the bot", steps: []step{{0, MoveToPawn{ObjectID: npcID, TargetID: selfID}}}, check: time.Second},
		{name: "party member's fight", setup: party, steps: []step{{0, Attack{AttackerID: pcID, Hits: hit(npcID)}}}, check: time.Second},
		{
			name:  "party member and an outsider",
			setup: party,
			steps: []step{{0, Attack{AttackerID: pcID, Hits: hit(npcID)}}, {0, Attack{AttackerID: outsider, Hits: hit(npcID)}}},
			check: time.Second, want: true,
		},
		{
			name:  "a member who left the party counts",
			setup: party,
			steps: []step{{0, Attack{AttackerID: pcID, Hits: hit(npcID)}}, {time.Second, PartySmallWindowDeleteAll{}}},
			check: 2 * time.Second, want: true,
		},
		{name: "a flagged player's summon hits a monster", setup: summon, steps: []step{{0, Attack{AttackerID: summonID, Hits: hit(npcID)}}}, check: time.Second, want: true},
		{name: "a monster chasing a flagged player's summon", setup: summon, steps: []step{{0, MoveToPawn{ObjectID: npcID, TargetID: summonID}}}, check: time.Second, want: true},
		{name: "monster minion following its master", steps: []step{{0, MoveToPawn{ObjectID: mob2ID, TargetID: npcID}}}, check: time.Second},
		{name: "player walks to a peaceful NPC", steps: []step{{0, MoveToPawn{ObjectID: pcID, TargetID: peacefulID}}}, mob: peacefulID, check: time.Second},
		{name: "dead monster", steps: []step{{0, Die{ObjectID: npcID}}, {0, MoveToPawn{ObjectID: pcID, TargetID: npcID}}}, check: time.Second},
		{name: "monster dies", steps: []step{{0, Attack{AttackerID: pcID, Hits: hit(npcID)}}, {0, Die{ObjectID: npcID}}}, check: time.Second},
		{name: "monster leaves sight", steps: []step{{0, Attack{AttackerID: pcID, Hits: hit(npcID)}}, {0, DeleteObject{ObjectID: npcID}}}, check: time.Second},
		{
			name:  "bot teleports",
			steps: []step{{0, Attack{AttackerID: pcID, Hits: hit(npcID)}}, {0, TeleportToLocation{ObjectID: selfID, Pos: Point{X: 9000}}}},
			check: time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorld(t)
			w.Apply(NPCInfo{ObjectID: mob2ID, NPCID: 20002, Attackable: true, Pos: Point{X: 1200}}, t0)
			w.Apply(NPCInfo{ObjectID: peacefulID, NPCID: 30001, Pos: Point{X: 800}}, t0)
			if tt.setup != nil {
				tt.setup(w)
			}
			for _, s := range tt.steps {
				w.Apply(s.ev, at(s.at))
			}
			mob := cmp.Or(tt.mob, npcID)
			if got := w.engagedByOther(mob, 10*time.Second, at(tt.check)); got != tt.want {
				t.Fatalf("engagedByOther = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRuleFarm checks the farm errand at the spot: attackers before fresh
// targets, the nearest fresh target, and the anti-KS skip.
func TestRuleFarm(t *testing.T) {
	now := at(time.Minute)
	const far = int32(0x20000021)
	outsiderHit := func(id int32, ago time.Duration) func(b *Brain) {
		return func(b *Brain) {
			b.world.Apply(Attack{AttackerID: pcID, Hits: []AttackHit{{TargetID: id}}}, now.Add(-ago))
		}
	}
	tests := []struct {
		name     string
		persona  func(p *Persona)
		setup    func(b *Brain)
		wantRule string
		wantKind actionKind
		// wantObject is the creature selected or attacked.
		wantObject int32
	}{
		{name: "nearest fresh monster", wantRule: "farm", wantKind: actSelect, wantObject: npcID},
		{name: "engaged by another player: the next one", setup: outsiderHit(npcID, 5*time.Second), wantRule: "farm", wantKind: actSelect, wantObject: mob2ID},
		{name: "engagement expired", setup: outsiderHit(npcID, 11*time.Second), wantRule: "farm", wantKind: actSelect, wantObject: npcID},
		{
			name:     "every monster engaged: the goal's turn",
			setup:    func(b *Brain) { outsiderHit(npcID, time.Second)(b); outsiderHit(mob2ID, time.Second)(b) },
			wantRule: "",
		},
		{
			name:     "window 0 skips none",
			persona:  func(p *Persona) { p.EngagedWindow = 0 },
			setup:    outsiderHit(npcID, time.Second),
			wantRule: "farm", wantKind: actSelect, wantObject: npcID,
		},
		{
			name: "engaged by a party member",
			setup: func(b *Brain) {
				b.world.Party = Party{LeaderID: selfID, Members: map[int32]*PartyMate{pcID: {}}}
				outsiderHit(npcID, time.Second)(b)
			},
			wantRule: "farm", wantKind: actSelect, wantObject: npcID,
		},
		{
			name: "an attacker before fresh targets, engaged or not",
			setup: func(b *Brain) {
				outsiderHit(mob2ID, time.Second)(b)
				b.world.Apply(Attack{AttackerID: mob2ID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, now)
			},
			wantRule: "farm", wantKind: actSelect, wantObject: mob2ID,
		},
		{
			name: "in combat an engaged attacker is fought back",
			setup: func(b *Brain) {
				outsiderHit(npcID, time.Second)(b)
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
			name: "selected target engaged since: the next one",
			setup: func(b *Brain) {
				b.world.Target = npcID
				outsiderHit(npcID, time.Second)(b)
			},
			wantRule: "farm", wantKind: actSelect, wantObject: mob2ID,
		},
		{
			name: "own fight kept when another player joins it",
			setup: func(b *Brain) {
				b.world.Target, b.engaged, b.progressAt = mob2ID, mob2ID, now
				outsiderHit(mob2ID, time.Second)(b)
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
			if tt.persona != nil {
				tt.persona(&b.persona)
			}
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
