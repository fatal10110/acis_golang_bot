package botclient

import (
	"testing"
	"time"
)

// joinMember links b under slot and hands it party at t0 with the slot's
// leader (Leader) in sight at lead.
func joinMember(t *testing.T, b *Brain, slot Command, party PartySmallWindowAll, lead Point) {
	t.Helper()
	linkBrain(b, standing{}).party = slot
	handleAt(t, b, t0,
		CharInfo{ObjectID: slotLeaderID, Name: "Leader", Pos: lead, RunSpeed: 100, WalkSpeed: 50, Running: true},
		party)
}

// playerParty is a party a player leads with the bot and Leader in it.
var playerParty = slotParty(pcID, map[int32]string{pcID: "other", slotLeaderID: "Leader"})

// reportLeader hands b the window's position of Leader at now.
func reportLeader(t *testing.T, b *Brain, now time.Time, pos Point) {
	t.Helper()
	handleAt(t, b, now, PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: slotLeaderID, Pos: pos}}})
}

// noLostLeaderAction fails on a whisper or an /unstuck.
func noLostLeaderAction(t *testing.T, rule string, a Action, now time.Time) {
	t.Helper()
	if a.kind == actTell || a.kind == actUserCommand {
		t.Fatalf("at %v: rule %q sent kind %d, want no whisper or /unstuck", now.Sub(t0), rule, a.kind)
	}
}

// TestPartyLeaderTeleported checks a join member's answer to its bot
// leader teleporting away (docs/parties/members.md, "Events"): no wait,
// whisper or /unstuck; with no spot, a Scroll of Escape and no attacker it
// takes a supply trip, else it walks to the leader's last
// PartyMemberPosition. A bot with no join slot and no spot takes no trip,
// nor does a member whose leader is back in sight, that landed already or
// that stands in town.
func TestPartyLeaderTeleported(t *testing.T) {
	reported := Point{X: 3000}
	lost := at(time.Minute)
	tests := []struct {
		name  string
		slot  Command
		items []Item
		edit  func(b *Brain)
		// trip: the scroll goes out under rule supply; else for 8 minutes
		// (past LeaderWait) the rule deciding is never supply nor, under a
		// slot, sends a whisper or /unstuck, and is follow when follows; an
		// attacked member is checked at once only.
		trip, follows, once bool
	}{
		{name: "a scroll, no attacker: supply trip", slot: joinSlot, items: []Item{soeStack(2)}, trip: true},
		{name: "no scroll: follows the last report", slot: joinSlot, follows: true},
		{
			name: "attacked: no trip", slot: joinSlot, items: []Item{soeStack(2)}, once: true,
			edit: func(b *Brain) {
				b.world.Apply(NPCInfo{ObjectID: mobObj, NPCID: mob, Attackable: true, Pos: Point{X: 100}}, lost)
				b.world.Apply(Attack{AttackerID: mobObj, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, lost)
			},
		},
		{
			name: "within supplyTripGap: follows", slot: joinSlot, items: []Item{soeStack(2)}, follows: true,
			edit: func(b *Brain) { b.supplyTrip.endedAt = lost },
		},
		{
			name: "the leader back in sight: follows", slot: joinSlot, items: []Item{soeStack(2)}, follows: true,
			edit: func(b *Brain) {
				handleAt(t, b, lost, CharInfo{ObjectID: slotLeaderID, Name: "Leader", Pos: Point{X: 1500}, RunSpeed: 100, WalkSpeed: 50, Running: true})
			},
		},
		{
			name: "landed already: follows", slot: joinSlot, items: []Item{soeStack(2)}, follows: true,
			edit: func(b *Brain) { b.lead.home = true },
		},
		{name: "no join slot, no spot: no trip", items: []Item{soeStack(2)}},
		{
			name: "in town: no trip", slot: joinSlot, items: []Item{soeStack(2)}, once: true,
			edit: func(b *Brain) { b.world.Self.move = stopAt(town) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTripBrain(t, tt.items...)
			b.leaveSpot()
			joinMember(t, b, tt.slot, memberParty, Point{X: 300})
			reportLeader(t, b, lost.Add(-time.Second), reported)
			handleAt(t, b, lost, TeleportToLocation{ObjectID: slotLeaderID, Pos: Point{X: 90000}}, DeleteObject{ObjectID: slotLeaderID})
			if !b.lead.teleported {
				t.Fatalf("watch = %+v, want the leader lost by teleport", b.lead)
			}
			if tt.edit != nil {
				tt.edit(b)
			}
			now := lost.Add(time.Second)
			rule, a := decideSend(t, b, now)
			if tt.trip {
				if rule != "supply" || a.kind != actUseItem || a.item != scrollOfEscape || b.supplyTrip.cause != causeLeader {
					t.Fatalf("decide = rule %q kind %d item %d cause %d, want supply reading the scroll for the leader", rule, a.kind, a.item, b.supplyTrip.cause)
				}
				if b.cfg.link.party != tt.slot || !b.world.Party.In() {
					t.Fatalf("slot %+v in party %v, want both kept", b.cfg.link.party, b.world.Party.In())
				}
				return
			}
			if tt.once {
				if rule == "supply" {
					t.Fatalf("decide = rule %q kind %d, want no trip", rule, a.kind)
				}
				return
			}
			moved := false
			for d := time.Duration(0); d <= 8*time.Minute; d += 5 * time.Second {
				if d > 0 {
					now = lost.Add(time.Second + d)
					rule, a = decideSend(t, b, now)
				}
				if tt.slot.Kind != 0 {
					noLostLeaderAction(t, rule, a, now)
				}
				if rule == "supply" {
					t.Fatalf("at %v: a supply trip, want none", now.Sub(t0))
				}
				if tt.follows && tt.slot.Kind != 0 && rule != "follow" {
					t.Fatalf("at %v: rule %q, want follow", now.Sub(t0), rule)
				}
				moved = moved || rule == "follow" && a.kind == actMove
			}
			if tt.follows && (!moved || !b.follow.anchored) {
				t.Fatalf("never walked: follow %+v", b.follow)
			}
			if tt.name == "no scroll: follows the last report" && b.follow.anchor != reported {
				t.Fatalf("follow anchor %v, want the last report %v", b.follow.anchor, reported)
			}
		})
	}
}

// TestPartyMemberSupply checks a join member's own supply triggers
// (docs/parties/members.md, "Events"): a Damage and a Support member with
// no spot, within GatherRange of the leader in sight or reported, potions
// gone, a consumable out or the load past ReturnLoad, go home at the next
// supplyCheck keeping slot and party, and follow after; beyond the range,
// within supplyTripGap, before the check, outside its bot party, in town,
// or with no slot and no spot, none goes.
func TestPartyMemberSupply(t *testing.T) {
	now := at(time.Minute)
	gone := func(b *Brain) { b.supplyTrip.potions = true }
	tests := []struct {
		name  string
		class int32
		slot  Command
		party PartySmallWindowAll
		lead  Point
		items []Item
		load  int32
		edit  func(b *Brain)
		want  tripCause
	}{
		{name: "damage member, potions gone", slot: joinSlot, lead: Point{X: -1000}, edit: gone, want: causePotions},
		{name: "support member, potions gone", class: classHumanMystic, slot: joinSlot, lead: Point{X: -1000}, edit: gone, want: causePotions},
		{name: "a consumable runs out", slot: joinSlot, lead: Point{X: -1000}, items: []Item{potionStack(1)}, want: causeRunOut},
		{
			name: "load past ReturnLoad", slot: joinSlot, lead: Point{X: -1000},
			items: []Item{potionStack(20), lootStack(60, junkMat, 1000)}, load: 60001, want: causeLoad,
		},
		{
			name: "the leader reported within range", slot: joinSlot, lead: Point{X: -300}, want: causePotions,
			edit: func(b *Brain) {
				gone(b)
				handleAt(t, b, now.Add(-time.Second), DeleteObject{ObjectID: slotLeaderID})
				reportLeader(t, b, now.Add(-time.Second), Point{X: 1100})
			},
		},
		{name: "beyond GatherRange", slot: joinSlot, lead: Point{X: -1300}, edit: gone},
		{
			name: "within supplyTripGap", slot: joinSlot, lead: Point{X: -1000},
			edit: func(b *Brain) { gone(b); b.supplyTrip.endedAt = now.Add(-time.Minute) },
		},
		{
			name: "before the next supplyCheck", slot: joinSlot, lead: Point{X: -1000},
			edit: func(b *Brain) { gone(b); b.supplyTrip.checkAt = now.Add(time.Second) },
		},
		{name: "a join slot in a player's party", slot: joinSlot, party: playerParty, lead: Point{X: -1000}, edit: gone},
		{name: "no slot, no spot", lead: Point{X: -1000}, edit: gone},
		{
			name: "in town beside the leader", slot: joinSlot, lead: Point{X: 3700},
			edit: func(b *Brain) { gone(b); b.world.Self.move = stopAt(town) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTripBrain(t, append([]Item{soeStack(1)}, tt.items...)...)
			b.leaveSpot()
			b.world.Self.ClassID, b.world.Self.Load = tt.class, tt.load
			party := tt.party
			if party.LeaderID == 0 {
				party = memberParty
			}
			joinMember(t, b, tt.slot, party, tt.lead)
			if tt.edit != nil {
				tt.edit(b)
			}
			rule, a := decideSend(t, b, now)
			if tt.want == causeNone {
				if rule == "supply" || b.supplyTrip.active() {
					t.Fatalf("decide = rule %q kind %d, trip %+v: want none", rule, a.kind, b.supplyTrip)
				}
				return
			}
			if rule != "supply" || a.kind != actUseItem || a.item != scrollOfEscape || b.supplyTrip.cause != tt.want {
				t.Fatalf("decide = rule %q kind %d item %d cause %d, want supply reading the scroll for %d", rule, a.kind, a.item, b.supplyTrip.cause, tt.want)
			}
			if b.cfg.link.party != tt.slot || !b.world.Party.In() {
				t.Fatalf("slot %+v in party %v, want both kept", b.cfg.link.party, b.world.Party.In())
			}
			// The recall lands it in town; it follows the leader back by the
			// window's positions.
			land := now.Add(20 * time.Second)
			handleAt(t, b, land, TeleportToLocation{ObjectID: selfID, Pos: town})
			reportLeader(t, b, land, Point{X: 1000})
			if b.supplyTrip.active() || b.cfg.link.party != tt.slot || !b.world.Party.In() {
				t.Fatalf("after landing: trip %+v slot %+v party %v, want ended, both kept", b.supplyTrip, b.cfg.link.party, b.world.Party.In())
			}
			if rule, _ := decideSend(t, b, land.Add(time.Second)); rule != "follow" {
				t.Fatalf("after landing: rule %q, want follow", rule)
			}
		})
	}
}

// TestPartyErrandWalks checks the errand gate in a bot party
// (docs/parties/members.md, "Errands in a bot party"): a join member walks
// to a trainer out of sight within TrainerReach, talks to it there and
// follows once done; at 20 to a class master's spawn, with dyes due to a
// dye maker's; a lead leader likewise. A bot in a player's party, a join
// slot in a party a player leads, or a member told to follow walks to none
// out of sight.
func TestPartyErrandWalks(t *testing.T) {
	near := Point{X: -300}
	trainer := func(t *testing.T) *Brain { return newTrainerWalkBrain(t) }
	master := func(t *testing.T) *Brain { return newClassBrain(t, classHumanMystic, 20) }
	dyes := func(t *testing.T) *Brain {
		return newDyeBrain(t, 1, 30, 2, nil, adenaStack(200000), dyeStack(200, dyeStr, 10), dyeStack(201, dyeDex, 10))
	}
	tests := []struct {
		name  string
		brain func(t *testing.T) *Brain
		slot  Command
		party PartySmallWindowAll
		edit  func(b *Brain)
		rule  string // "": no walk out of sight
	}{
		{name: "join member: trainer", brain: trainer, slot: joinSlot, party: memberParty, rule: "train"},
		{name: "join member: class master", brain: master, slot: joinSlot, party: memberParty, rule: "classchange"},
		{name: "join member: dye maker", brain: dyes, slot: joinSlot, party: memberParty, rule: "dyes"},
		{name: "lead leader: trainer", brain: trainer, slot: leadSlot, party: leadParty, rule: "train"},
		{name: "lead leader: class master", brain: master, slot: leadSlot, party: leadParty, rule: "classchange"},
		{name: "a player's party: trainer", brain: trainer, party: playerParty},
		{name: "a player's party: class master", brain: master, party: playerParty},
		{name: "a player's party: dye maker", brain: dyes, party: playerParty},
		{name: "join slot, a player leads: trainer", brain: trainer, slot: joinSlot, party: playerParty},
		{
			name: "join member told to follow: trainer", brain: trainer, slot: joinSlot, party: memberParty,
			edit: func(b *Brain) { b.orders.follow = slotLeaderID },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.brain(t)
			joinMember(t, b, tt.slot, tt.party, near)
			if tt.edit != nil {
				tt.edit(b)
			}
			rule, a := b.decide(at(time.Second))
			if tt.rule == "" {
				if rule == "train" || rule == "classchange" || rule == "dyes" {
					t.Fatalf("decide = %q %d, want no errand walk", rule, a.kind)
				}
				return
			}
			if rule != tt.rule || a.kind != actMove {
				t.Fatalf("decide = %q %d, want %q walking", rule, a.kind, tt.rule)
			}
		})
	}

	// A join member walks to the trainer, talks to it in sight and follows
	// its leader again once the visit is done.
	b := newTrainerWalkBrain(t)
	joinMember(t, b, joinSlot, memberParty, near)
	runTrainerSteps(t, b, []trainerStep{
		{name: "a segment toward the trainer's spawn", at: time.Second, wantRule: "train", wantKind: actMove},
		{
			name: "trainer in sight: select", at: 40 * time.Second,
			setup:    func(b *Brain) { b.world.Self.move = stopAt(Point{X: 4800}) },
			events:   []Event{NPCInfo{ObjectID: trainerObj, NPCID: 30022, Pos: Point{X: 5000}}},
			wantRule: "train", wantKind: actSelect,
		},
		{name: "talk", at: 41 * time.Second, events: []Event{MyTargetSelected{ObjectID: trainerObj}}, wantRule: "train", wantKind: actTalk},
	})
	b.train = trainerVisit{doneLevel: 15}
	reportLeader(t, b, at(50*time.Second), near)
	if rule, _ := b.decide(at(51 * time.Second)); rule != "follow" {
		t.Fatalf("visit done: rule %q, want follow", rule)
	}
}
