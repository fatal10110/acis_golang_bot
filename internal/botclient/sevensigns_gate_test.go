package botclient

import (
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// cataObj is the Dungeon Gatekeeper 31114 (enter_catacomb1) once in sight.
const cataObj = int32(0x20000050)

// cataPage is the gatekeeper's first page (data/html/gatekeeper/31114.htm,
// %objectId% filled in).
func cataPage(obj int32) string {
	return `<html><body>Gatekeeper Ziggurat:<br>This is the Forbidden Sanctum.<br>` +
		`<a action="bypass -h npc_` + itoa(obj) + `_cata">Teleport.</a><br>` +
		`<a action="bypass -h npc_` + itoa(obj) + `_Quest">Quest</a></body></html>`
}

// TestIndexDungeonGates: the dungeon gatekeepers with a fixed spawn and an
// instant teleport join the travel map, each side by its alias; a boss
// room's, one with no teleport and a standard gatekeeper do not.
func TestIndexDungeonGates(t *testing.T) {
	dp := &datapack.Datapack{
		NPCs: map[int32]datapack.NPC{
			31095: {ID: 31095, Type: npcTypeDungeonGatekeeper, Alias: "enter_necropolis1"},
			31103: {ID: 31103, Type: npcTypeDungeonGatekeeper, Alias: "exit_necropolis1"},
			31111: {ID: 31111, Type: npcTypeDungeonGatekeeper, Alias: "enter_necropolis_boss"},
			31112: {ID: 31112, Type: npcTypeDungeonGatekeeper, Alias: "exit_necropolis_boss"},
			31114: {ID: 31114, Type: npcTypeDungeonGatekeeper, Alias: "enter_catacomb1"},
			31115: {ID: 31115, Type: npcTypeDungeonGatekeeper, Alias: "enter_catacomb2"},
			30006: {ID: 30006, Type: npcTypeGatekeeper, Alias: "enter_catacomb9"},
		},
		InstantTeleports: map[int32][]datapack.Teleport{
			31095: {{X: -41570, Y: 209785, Z: -5089}},
			31103: {{X: -41567, Y: 209292, Z: -5091}},
			31111: {{X: 1, Y: 2, Z: 3}},
			31112: {{X: 183225, Y: -11911, Z: -4897}},
			31114: {{X: -53174, Y: -250275, Z: -7911}},
			30006: {{X: 1, Y: 2, Z: 3}},
		},
	}
	spawns := map[int32][]Point{
		31095: {{X: -41568, Y: 209344, Z: -5085}}, 31103: {{X: -41568, Y: 210128, Z: -5088}},
		31111: {{X: 1}}, 31112: {{X: 2}}, 31114: {{X: -53000, Y: -250000, Z: -7900}}, 31115: {{X: 5}}, 30006: {{X: 6}},
	}
	want := []gatekeeper{
		{npcID: 31095, at: Point{X: -41568, Y: 209344, Z: -5085}, seal: SealAvarice, dests: []destination{{at: Point{X: -41570, Y: 209785, Z: -5089}, cmd: "necro"}}},
		{npcID: 31103, at: Point{X: -41568, Y: 210128, Z: -5088}, dests: []destination{{at: Point{X: -41567, Y: 209292, Z: -5091}, cmd: "exit"}}},
		{npcID: 31114, at: Point{X: -53000, Y: -250000, Z: -7900}, seal: SealGnosis, dests: []destination{{at: Point{X: -53174, Y: -250275, Z: -7911}, cmd: "cata"}}},
	}
	if got := indexDungeonGates(dp, spawns); !reflect.DeepEqual(got, want) {
		t.Fatalf("indexDungeonGates = %+v, want %+v", got, want)
	}
}

// newGateBrain is the travel brain with no adena, its one gatekeeper the
// catacomb's Dungeon Gatekeeper at (2000, 0) taking the bot across the
// wall to travelDest, and its fitting spot "far" there a competition
// catacomb spot (ssq_event); member makes it the one Seven Signs member,
// signed up in the competition.
func newGateBrain(t *testing.T, member bool) *Brain {
	t.Helper()
	b := newTravelBrain(t, 0)
	b.world.Inventory.Items = nil
	b.cfg.Travel.gatekeepers = nil
	b.cfg.Travel.dungeonGates = []gatekeeper{{npcID: 31114, at: Point{X: 2000}, seal: SealGnosis, dests: []destination{{at: Point{X: travelDest}, cmd: "cata"}}}}
	b.cfg.Spots.spots[1].Event = "ssq_event"
	b.world.Self.Name = signsBot
	if member {
		b.persona.SevenSigns = SevenSignsPolicy{Members: []string{signsBot}, MaxMembers: 1}
		b.signs.member, b.signs.period, b.signs.readAt = true, signsCompetition, at(0)
	}
	return b
}

// TestSevenSignsGateLeg: a member plans the Dungeon Gatekeeper as a trip's
// free leg to a catacomb spot, sends the gatekeeper page's cata link under
// the bypass gate, and lands; a non-member plans no such trip.
func TestSevenSignsGateLeg(t *testing.T) {
	if b := newGateBrain(t, false); true {
		b.decide(at(time.Second))
		if b.trip.v.active() {
			t.Fatalf("a non-member planned a trip: %+v", b.trip)
		}
	}
	b := newGateBrain(t, true)
	runTrainerSteps(t, b, []trainerStep{
		{name: "no open spot in reach plans the dungeon trip", at: time.Second},
		{name: "leg to the gatekeeper", at: 2 * time.Second, wantRule: "travel", wantKind: actMove},
		{
			name: "gatekeeper in sight: select", at: 3 * time.Second,
			setup:    func(b *Brain) { b.world.Self.move = stopAt(Point{X: 1900}) },
			events:   []Event{NPCInfo{ObjectID: cataObj, NPCID: 31114, Pos: Point{X: 2000}}},
			wantRule: "travel", wantKind: actSelect,
		},
		{name: "talk", at: 4 * time.Second, events: []Event{MyTargetSelected{ObjectID: cataObj}}, wantRule: "travel", wantKind: actTalk},
		{
			name: "the page's cata link", at: 5 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: cataObj, HTML: cataPage(cataObj)}},
			wantRule: "travel", wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer(npcBypass(cataObj, "cata")),
		},
		{name: "wait for the teleport", at: 6 * time.Second, wantRule: "travel"},
	})
	if b.trip.v.npcID != 31114 {
		t.Fatalf("trip %+v, want the catacomb gatekeeper's", b.trip)
	}
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: travelDest}}}, at(7*time.Second)); err != nil {
		t.Fatal(err)
	}
	b.appearing = false
	if rule, a := b.decide(at(8 * time.Second)); rule != "spot" || a.kind != actMove || !b.spot.held || b.spot.idx != 1 {
		t.Fatalf("after landing = %q %d spot %+v, want a walk to the catacomb spot", rule, a.kind, b.spot)
	}
}

// TestSevenSignsGateRefused: the gatekeeper turning the bot away (a page,
// no teleport) ends the trip and closes the seal's dungeons and spots
// until the bot reads the period anew; a page without the dungeon link
// shuns the gatekeeper as a standard one's would.
func TestSevenSignsGateRefused(t *testing.T) {
	inSight := func(b *Brain) {
		b.world.Self.move = stopAt(Point{X: 1900})
		b.world.Apply(NPCInfo{ObjectID: cataObj, NPCID: 31114, Pos: Point{X: 2000}}, t0)
		b.world.Target = cataObj
	}
	cataNo := `<html><body>Gatekeeper Ziggurat:<br>Only those who have joined the Seven Signs may enter.</body></html>`
	b := newGateBrain(t, true)
	runTrainerSteps(t, b, []trainerStep{
		{name: "plan", at: time.Second, setup: inSight},
		{name: "talk", at: 2 * time.Second, wantRule: "travel", wantKind: actTalk},
		{name: "cata", at: 3 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: cataObj, HTML: cataPage(cataObj)}}, wantRule: "travel", wantKind: actBypass},
		{name: "the refusal page", at: 4 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: cataObj, HTML: cataNo}}, wantRule: "travel"},
		{name: "no teleport", at: 3*time.Second + tripDepartTimeout},
	})
	now := at(time.Minute)
	if b.trip.v.active() || b.signsEnters(SealGnosis, now) {
		t.Fatalf("after the refusal: trip %+v, catacombs open %v", b.trip, b.signsEnters(SealGnosis, now))
	}
	// The competition's ssq_event makers fill necropolises and catacombs
	// alike: their spots close once neither way in is open.
	if !b.signsEnters(SealAvarice, now) || !b.spotOpen(&b.cfg.Spots.spots[1], now) {
		t.Fatal("the refusal closed the necropolises too")
	}
	b.signsRefused(SealAvarice, now)
	if b.spotOpen(&b.cfg.Spots.spots[1], now) {
		t.Fatal("a competition spot open with both ways in refused")
	}

	b = newGateBrain(t, true)
	runTrainerSteps(t, b, []trainerStep{
		{name: "plan", at: time.Second, setup: inSight},
		{name: "talk", at: 2 * time.Second, wantRule: "travel", wantKind: actTalk},
		{name: "a page without the link", at: 3 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: cataObj, HTML: gkPage(cataObj)}}},
	})
	if b.trip.v.active() || !b.trip.shunned.held(0, at(time.Hour)) {
		t.Fatalf("after a page without the link: trip %+v shunned %v", b.trip, b.trip.shunned)
	}
}
