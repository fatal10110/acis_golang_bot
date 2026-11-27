package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// travelGK is testTravelMap's gatekeeper template.
const travelGK = int32(30006)

// gkSteps are a trip's steps under rule from the gatekeeper in sight to
// its destination link, from start on one second apart: the bot, standing
// at stand, selects gatekeeper obj (template npcID at npcAt), talks, opens
// the list; last is the step the destination's list page arrives at.
func gkSteps(rule string, start time.Duration, npcID int32, npcAt, stand Point, last trainerStep) []trainerStep {
	s := func(i int) time.Duration { return start + time.Duration(i)*time.Second }
	last.at, last.wantRule = s(5), rule
	last.events = append(last.events, NpcHtmlMessage{ObjectID: gkObj, HTML: gkList(gkObj, 2)})
	return []trainerStep{
		{
			name: "gatekeeper in sight: select", at: s(0),
			setup:    func(b *Brain) { b.world.Self.move = stopAt(stand) },
			events:   []Event{NPCInfo{ObjectID: gkObj, NPCID: npcID, Pos: npcAt}},
			wantRule: rule, wantKind: actSelect, wantPayload: botwire.EncodeAction(gkObj, stand),
		},
		{name: "talk", at: s(1), events: []Event{MyTargetSelected{ObjectID: gkObj}}, wantRule: rule, wantKind: actTalk},
		{name: "wait for the page", at: s(2), wantRule: rule},
		{
			name: "list link", at: s(3), events: []Event{NpcHtmlMessage{ObjectID: gkObj, HTML: gkPage(gkObj)}},
			wantRule: rule, wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer(teleportListBypass(gkObj)),
		},
		{name: "wait for the list", at: s(4), wantRule: rule},
		last,
	}
}

// destLink is the trip's destination link to testTravelMap's index 2.
var destLink = botwire.EncodeRequestBypassToServer(teleportBypass(gkObj, 2))

// newPartyTripLeader is a level 20 bot at the origin carrying 5000 adena,
// with spot "low" in walking reach that fits it not and spot "far" past the
// wall, near testTravelMap's destination 2: given ms, it leads them under a
// lead slot naming Abel, else it is solo.
func newPartyTripLeader(t *testing.T, ms ...PartyMember) *Brain {
	t.Helper()
	b := newSpotBrain(t, 20, testSpot("low", 1000, 0, 1, 5, 9), testSpot("far", travelFar, 0, 18, 21, 9))
	if len(ms) > 0 {
		b.cfg.link = newSlotLink(standing{})
		b.setPartySlot(lead("Abel"))
		feed(t, b, 0, leadWindow(ms...))
	}
	b.persona.Newbie.MaxLevel = 0
	b.cfg.Travel = testTravelMap()
	b.world.Inventory.Items = map[int32]Item{1: {ObjectID: 1, TemplateID: adenaID, Count: 5000}}
	b.world.Objects = map[int32]*Object{}
	return b
}

// TestPartyTripGather checks the leader's gather gate at the gatekeeper
// (docs/parties/spots.md, "Gatekeeper trip"): with no spot in walking reach
// it plans its gatekeeper trip as a solo bot does, and holds the
// destination's link while a bot member stands beyond GatherRange (but
// within FarAway), at most GatherWait; a member near or past FarAway, or
// none, holds nothing.
func TestPartyTripGather(t *testing.T) {
	const start = 2 * time.Second
	tests := []struct {
		name   string
		ms     []PartyMember
		member Point
		// holds: the link waits GatherWait.
		holds bool
	}{
		{name: "member near: at once", ms: []PartyMember{mate(abelID, 20)}, member: Point{X: 1500}},
		{name: "member behind: waits", ms: []PartyMember{mate(abelID, 20)}, member: Point{X: -2000}, holds: true},
		{name: "member past FarAway: at once", ms: []PartyMember{mate(abelID, 20)}, member: Point{X: -12000}},
		{name: "solo: at once"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newPartyTripLeader(t, tt.ms...)
			b.ruleSpot(at(time.Second))
			if !b.trip.v.active() || b.trip.dest.index != 2 || b.spot.farming() {
				t.Fatalf("trip %+v spot %+v: want the trip to destination 2, no spot", b.trip, b.spot)
			}
			if len(tt.ms) > 0 {
				handleAt(t, b, at(time.Second), CharInfo{ObjectID: abelID, Name: "Abel", Pos: tt.member, RunSpeed: 100, WalkSpeed: 50})
			}
			last := trainerStep{name: "destination link", wantKind: actBypass, wantPayload: destLink}
			if tt.holds {
				last.name, last.wantKind, last.wantPayload = "destination link held", actHold, nil
			}
			runTrainerSteps(t, b, gkSteps("travel", start, travelGK, Point{X: 2000}, Point{X: 2100}, last))
			if !tt.holds {
				return
			}
			linked := start + 5*time.Second
			gw := b.persona.Lead.withDefaults().GatherWait
			runTrainerSteps(t, b, []trainerStep{
				{name: "still held", at: linked + gw - time.Second, wantRule: "travel"},
				{name: "GatherWait out: the link", at: linked + gw, wantRule: "travel", wantKind: actBypass, wantPayload: destLink},
			})
		})
	}
}

// TestPartyTripRegroup checks the leader's regroup at the landing (docs/
// parties/spots.md, "Gatekeeper trip"): landed from its trip with a bot
// member it hunts nothing while the member is reported at the trip's start
// or not reported, until the member stands within GatherRange, at most
// GatherWait; a solo bot hunts at once.
func TestPartyTripRegroup(t *testing.T) {
	land := at(20 * time.Second)
	dest := Point{X: travelDest}
	tests := []struct {
		name string
		ms   []PartyMember
		// report is where the member is reported before the landing, nil
		// for nowhere; arrive, when set, is when it comes in sight near.
		report *Point
		arrive time.Duration
	}{
		{name: "member at the start: waits it out", ms: []PartyMember{mate(abelID, 20)}, report: &Point{X: 2100}},
		{name: "member not reported: waits it out", ms: []PartyMember{mate(abelID, 20)}},
		{name: "member lands: hunts", ms: []PartyMember{mate(abelID, 20)}, report: &Point{X: 2100}, arrive: 15 * time.Second},
		{name: "solo: hunts at once"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newPartyTripLeader(t, tt.ms...)
			b.ruleSpot(at(time.Second))
			if tt.report != nil {
				handleAt(t, b, at(2*time.Second), PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: abelID, Pos: *tt.report}}})
			}
			b.trip.step = tripGone
			handleAt(t, b, land, TeleportToLocation{ObjectID: selfID, Pos: dest})
			if b.trip.v.active() {
				t.Fatal("trip still on after its landing")
			}
			gw := b.persona.Lead.withDefaults().GatherWait
			free := gw
			switch {
			case len(tt.ms) == 0:
				free = 0
			case tt.arrive > 0:
				free = tt.arrive
			}
			for d := time.Duration(0); d <= gw; d += 5 * time.Second {
				now := land.Add(d)
				if tt.arrive > 0 && d == tt.arrive {
					handleAt(t, b, now, CharInfo{ObjectID: abelID, Name: "Abel", Pos: Point{X: travelDest + 300}, RunSpeed: 100, WalkSpeed: 50})
				}
				_, held := b.ruleTravel(now)
				if want := d < free; held != want {
					t.Fatalf("at landing+%v: travel holds %v, want %v", d, held, want)
				}
			}
		})
	}
}

// newPartyTripMember is a join member of Leader's bot party at the origin
// holding no spot, carrying adena and a Scroll of Escape, with
// testTravelMap's gatekeeper (destination 2 for 1000 adena) at (2000, 0)
// and Leader in sight at lead.
func newPartyTripMember(t *testing.T, adena int32, lead Point) *Brain {
	t.Helper()
	b := newTripBrain(t, Item{ObjectID: 1, TemplateID: adenaID, Count: adena}, soeStack(2))
	b.leaveSpot()
	b.cfg.Travel.gatekeepers = testTravelMap().gatekeepers
	joinMember(t, b, joinSlot, memberParty, lead)
	return b
}

// jump is when the leader teleports in the member tests, and leaderLanding where
// it lands: within the gatekeeper's scatter of destination 2.
var (
	jump          = at(time.Minute)
	leaderLanding = Point{X: travelDest + 15, Y: -10}
)

// leaderJumps hands b its leader's teleport to to at jump, and the leader's
// leaving its sight.
func leaderJumps(t *testing.T, b *Brain, to Point) {
	t.Helper()
	handleAt(t, b, jump, TeleportToLocation{ObjectID: slotLeaderID, Pos: to}, DeleteObject{ObjectID: slotLeaderID})
}

// TestPartyTripMember checks a join member's answer to its leader's
// gatekeeper teleport (docs/parties/members.md, "Events"): it takes the
// same destination of the gatekeeper the leader stood at, under rule
// travel with no recall, whisper or /unstuck, and on landing follows its
// leader again.
func TestPartyTripMember(t *testing.T) {
	b := newPartyTripMember(t, 5000, Point{X: 2100})
	leaderJumps(t, b, leaderLanding)
	if tr := &b.trip; !b.partyTrip() || tr.gk != 0 || tr.dest.index != 2 {
		t.Fatalf("trip %+v: want the leader's, destination 2", b.trip)
	}
	steps := append([]trainerStep{{name: "leg to the gatekeeper", at: time.Minute + time.Second, wantRule: "travel", wantKind: actMove}},
		gkSteps("travel", time.Minute+2*time.Second, travelGK, Point{X: 2000}, Point{X: 2100},
			trainerStep{name: "destination link", wantKind: actBypass, wantPayload: destLink})...)
	steps = append(steps, trainerStep{name: "wait for the teleport", at: time.Minute + 9*time.Second, wantRule: "travel"})
	runTrainerSteps(t, b, steps)
	landed := at(time.Minute + 10*time.Second)
	b.world.Objects = map[int32]*Object{}
	handleAt(t, b, landed, TeleportToLocation{ObjectID: selfID, Pos: Point{X: travelDest}},
		CharInfo{ObjectID: slotLeaderID, Name: "Leader", Pos: leaderLanding, RunSpeed: 100, WalkSpeed: 50, Running: true})
	if b.trip.v.active() || b.trip.follow || !b.lead.seen {
		t.Fatalf("trip %+v watch %+v: want the trip over, the leader seen", b.trip, b.lead)
	}
	if b.cfg.link.party != joinSlot || !b.world.Party.In() {
		t.Fatalf("slot %+v in party %v: want both kept", b.cfg.link.party, b.world.Party.In())
	}
	for d := time.Second; d < 8*time.Minute; d += 10 * time.Second {
		now := landed.Add(d)
		rule, a := decideSend(t, b, now)
		noLostLeaderAction(t, rule, a, now)
		if rule == "supply" || rule == "travel" {
			t.Fatalf("at %v: rule %q kind %d, want neither a recall nor a trip", now.Sub(t0), rule, a.kind)
		}
	}
}

// TestPartyTripMemberLeaves checks a join member that cannot take its
// leader's gatekeeper teleport (docs/parties/members.md, "Events"): short
// of the fee, with karma, or refused by the server, it clears its slot
// (PartyLeft "left"), withdraws, and neither whispers its leader nor casts
// /unstuck; it plays solo.
func TestPartyTripMemberLeaves(t *testing.T) {
	tests := []struct {
		name  string
		adena int32
		edit  func(b *Brain)
		// refused: the member takes the trip and the server answers no
		// teleport.
		refused bool
	}{
		{name: "short of the fee", adena: 999},
		{name: "past its MaxPrice", adena: 5000, edit: func(b *Brain) { b.persona.Travel.MaxPrice = 500 }},
		{name: "karma", adena: 5000, edit: func(b *Brain) { b.world.Self.Karma = 100 }},
		{name: "refused", adena: 5000, refused: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newPartyTripMember(t, tt.adena, Point{X: 2100})
			if tt.edit != nil {
				tt.edit(b)
			}
			leaderJumps(t, b, leaderLanding)
			now := jump
			if tt.refused {
				if !b.partyTrip() {
					t.Fatalf("trip %+v: want the leader's", b.trip)
				}
				runTrainerSteps(t, b, gkSteps("travel", time.Minute+2*time.Second, travelGK, Point{X: 2000}, Point{X: 2100},
					trainerStep{name: "destination link", wantKind: actBypass, wantPayload: destLink}))
				if b.cfg.link.party != joinSlot {
					t.Fatalf("slot %+v cleared before the refusal", b.cfg.link.party)
				}
				now = at(time.Minute + 7*time.Second).Add(tripDepartTimeout)
				b.ruleTravel(now)
			}
			if l := b.cfg.link; l.party.Kind != 0 || l.partyLeft != partyLeftLeft || b.trip.v.active() {
				t.Fatalf("slot %+v left %q trip %v: want the slot cleared as left, no trip", l.party, l.partyLeft, b.trip.v.active())
			}
			now = now.Add(time.Second)
			if rule, a := decideSend(t, b, now); a.kind != actLeaveParty {
				t.Fatalf("decide = rule %q kind %d, want the withdrawal", rule, a.kind)
			}
			handleAt(t, b, now.Add(time.Second), PartySmallWindowDeleteAll{})
			for d := 2 * time.Second; d < 8*time.Minute; d += 10 * time.Second {
				rule, a := decideSend(t, b, now.Add(d))
				noLostLeaderAction(t, rule, a, now.Add(d))
				if a.kind == actUseItem && a.item == scrollOfEscape {
					t.Fatalf("at %v: a recall for the leader it left", now.Add(d).Sub(t0))
				}
			}
		})
	}
}

// TestPartyTripMemberOther checks the leader's teleports a member does not
// read as a gatekeeper trip, left to its loss (P2b, TestPartyLeaderTeleported):
// a recall away from every gatekeeper, a landing at none of the
// gatekeeper's destinations, the leader out of sight, and a leader not its
// slot's.
func TestPartyTripMemberOther(t *testing.T) {
	tests := []struct {
		name     string
		lead, to Point
		edit     func(t *testing.T, b *Brain)
	}{
		{name: "recall in the field", lead: Point{X: 300}, to: town},
		{name: "landing at no destination", lead: Point{X: 2100}, to: Point{X: travelDest + 1000}},
		{
			name: "leader out of sight", lead: Point{X: 2100}, to: leaderLanding,
			edit: func(t *testing.T, b *Brain) {
				handleAt(t, b, jump.Add(-time.Second), DeleteObject{ObjectID: slotLeaderID})
			},
		},
		{
			name: "no slot", lead: Point{X: 2100}, to: leaderLanding,
			edit: func(t *testing.T, b *Brain) { b.cfg.link.party = Command{} },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newPartyTripMember(t, 5000, tt.lead)
			if tt.edit != nil {
				tt.edit(t, b)
			}
			leaderJumps(t, b, tt.to)
			if b.trip.v.active() {
				t.Fatalf("trip %+v: want none", b.trip)
			}
			if rule, _ := decideSend(t, b, jump.Add(time.Second)); rule == "travel" {
				t.Fatal("decide = rule travel, want no trip")
			}
		})
	}
}

// TestPartyGatekeeperTrip: a leader with bot members plans its gatekeeper
// trip as a solo bot does, for a spot with room for its party; an
// outlevelled one gives its spot back for the trip, and with no trip
// either gives it back with nothing in reach fitting (docs/parties/
// spots.md, "Gatekeeper trip").
func TestPartyGatekeeperTrip(t *testing.T) {
	low, far, s15 := testSpot("low", 1000, 0, 1, 5, 9), testSpot("far", travelFar, 0, 18, 21, 9), testSpot("s15", 2000, 2000, 10, 15, 9)
	mid := testSpot("mid", 0, 3000, 18, 21, 3) // cap 1, limit 3
	trio := []PartyMember{mate(abelID, 21), mate(bethID, 21)}
	tests := []struct {
		name  string
		level int32
		ms    []PartyMember
		spots []datapack.Spot
		run   func(t *testing.T, b *Brain)
	}{
		{
			name: "alone it plans a trip", level: 20, spots: []datapack.Spot{low, far},
			run: func(t *testing.T, b *Brain) {
				b.ruleSpot(at(time.Second))
				if !b.trip.v.active() {
					t.Fatal("no trip")
				}
			},
		},
		{
			name: "with members it plans one", level: 20, ms: []PartyMember{mate(abelID, 20)}, spots: []datapack.Spot{low, far},
			run: func(t *testing.T, b *Brain) {
				b.ruleSpot(at(time.Second))
				if !b.trip.v.active() || b.spot.farming() {
					t.Fatalf("trip %v spot %+v: want a trip, no spot", b.trip.v.active(), b.spot)
				}
			},
		},
		{
			name: "no room there for its party: none", level: 20, ms: []PartyMember{mate(abelID, 20)}, spots: []datapack.Spot{low, far},
			run: func(t *testing.T, b *Brain) {
				b.cfg.Spots.held[1] = b.cfg.Spots.caps[1] - 1
				b.ruleSpot(at(time.Second))
				if b.trip.v.active() || b.spot.farming() {
					t.Fatalf("trip %v spot %+v: want neither", b.trip.v.active(), b.spot)
				}
			},
		},
		{
			name: "outlevelled with nothing in reach: a trip, its spot given back", level: 21, ms: trio, spots: []datapack.Spot{low, far, s15},
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 2, false, 3)
				b.ruleSpot(at(time.Second))
				if !b.trip.v.active() || b.spot.farming() {
					t.Fatalf("trip %v spot %+v: want a trip, the spot given back", b.trip.v.active(), b.spot)
				}
				wantCounts(t, b, "given back", 2, 0, 0)
			},
		},
		{
			name: "outlevelled with no trip and nothing in reach: gives its spot back", level: 21, ms: trio, spots: []datapack.Spot{low, far, s15},
			run: func(t *testing.T, b *Brain) {
				b.world.Inventory.Items = nil
				spotAt(b, 2, false, 3)
				b.ruleSpot(at(time.Second))
				if b.trip.v.active() || b.spot.farming() {
					t.Fatalf("trip %v spot %+v: want neither", b.trip.v.active(), b.spot)
				}
				for i := range 3 {
					wantCounts(t, b, "given back", i, 0, 0)
				}
				b.ruleSpot(at(time.Second + spotRetry))
				if b.spot.farming() {
					t.Fatalf("spot %+v: reclaimed", b.spot)
				}
			},
		},
		{
			name: "no trip, one candidate too near its limit: keeps its spot", level: 21, ms: trio, spots: []datapack.Spot{low, far, s15, mid},
			run: func(t *testing.T, b *Brain) {
				b.world.Inventory.Items = nil
				spotAt(b, 2, false, 3)
				b.cfg.Spots.held[3] = 1
				b.ruleSpot(at(time.Second))
				if b.trip.v.active() || !b.spot.held || b.spot.idx != 2 || !b.spot.retryAt.Equal(at(time.Second+spotRetry)) {
					t.Fatalf("trip %v spot %+v: want s15 kept, a re-pick after spotRetry", b.trip.v.active(), b.spot)
				}
				wantCounts(t, b, "kept", 2, 3, 0)
			},
		},
		{
			name: "solo outlevelled: a trip and its give-back", level: 21, spots: []datapack.Spot{low, far, s15},
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 2, false, 1)
				b.ruleSpot(at(time.Second))
				if !b.trip.v.active() || b.spot.farming() {
					t.Fatalf("trip %v spot %+v: want a trip, the spot given back", b.trip.v.active(), b.spot)
				}
				wantCounts(t, b, "given back", 2, 0, 0)
			},
		},
		{
			name: "solo outlevelled with no adena: no trip, kept", level: 21, spots: []datapack.Spot{low, far, s15},
			run: func(t *testing.T, b *Brain) {
				b.world.Inventory.Items = nil
				spotAt(b, 2, false, 1)
				b.ruleSpot(at(time.Second))
				if b.trip.v.active() || !b.spot.held || b.spot.idx != 2 {
					t.Fatalf("trip %v spot %+v: want s15 kept", b.trip.v.active(), b.spot)
				}
				wantCounts(t, b, "kept", 2, 1, 0)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newLeadSpotBrain(t, tt.level, tt.ms, tt.spots...)
			b.persona.Newbie.MaxLevel = 0
			b.cfg.Travel = testTravelMap()
			b.world.Inventory.Items = map[int32]Item{1: {ObjectID: 1, TemplateID: adenaID, Count: 5000}}
			b.world.Objects = map[int32]*Object{}
			tt.run(t, b)
		})
	}
}
