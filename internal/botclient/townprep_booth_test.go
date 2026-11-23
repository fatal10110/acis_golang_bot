package botclient

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// The navigation of the booth tests: open flat ground at height 0 east and
// south of the world origin with a merchant's booth, a ring of wall two
// cells thick (region-local columns 309-315, rows 47-53) around a 3x3-cell
// pocket no walk enters and no straight line reaches, its centre at
// boothNPC. Talking Island's grocer stands so (synthetic geodata, never
// real; #253).
var (
	boothOnce   sync.Once
	boothShared *Nav
	boothErr    error

	boothNPC = Point{X: int32(geotest.CellWorldX(312)), Y: int32(geotest.CellWorldY(50))}
)

// boothNav is the shared booth navigation, built once per test binary.
func boothNav(tb testing.TB) *Nav {
	tb.Helper()
	boothOnce.Do(func() {
		t := (&geotest.Terrain{}).
			Raise(309, 47, 315, 48, geotest.Wall).
			Raise(309, 52, 315, 53, geotest.Wall).
			Raise(309, 49, 310, 51, geotest.Wall).
			Raise(314, 49, 315, 51, geotest.Wall)
		boothShared, boothErr = buildTestNav(t, navWindow(0, 0, 1, 0))
	})
	if boothErr != nil {
		tb.Fatal(boothErr)
	}
	return boothShared
}

// walkBooth has brain b walk on the booth navigation from from.
func walkBooth(t *testing.T, b *Brain, from Point) *Brain {
	t.Helper()
	n := boothNav(t)
	b.walk = newWalker(n)
	b.world.SetGround(n.Engine)
	b.world.Self.move = stopAt(from)
	return b
}

// TestWalkerBoothTargets checks where a visit walks for an NPC in a pocket
// no walk enters: its spawn is unreachable as it stands, so the walk heads
// for the bot's ground nearest it within talkReach; with no stand
// point offering an open line, the talk point is the bot's own place within
// talkReach of the NPC, else the cell of the bot's ground nearest the NPC.
func TestWalkerBoothTargets(t *testing.T) {
	near := Point{X: boothNPC.X - 80, Y: boothNPC.Y}
	tests := []struct {
		name string
		from Point
		// talkAt, when set, is the talk point wanted exactly.
		talkAt Point
	}{
		{name: "far west", from: Point{X: 2000, Y: boothNPC.Y}},
		{name: "far south", from: Point{X: boothNPC.X, Y: boothNPC.Y + 2000}},
		{name: "within reach", from: near, talkAt: near},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := walkBooth(t, newShopBrain(t), tt.from)
			w := b.walker()
			if _, ok := w.standPoint(tt.from, boothNPC, visitTalkRange, nav.StandLine, 0); ok {
				t.Fatal("stand point with an open line to the booth, want none")
			}
			v := npcVisit{npcID: 30004, at: boothNPC}
			if b.walkTo(&v, at(time.Second)); w.status == walkUnreachable || horizontal2(v.walkAt, boothNPC) > talkReach*talkReach {
				t.Fatalf("walk to the booth %s, to %+v, want one to ground within %d of it", w.failed(), v.walkAt, talkReach)
			}
			p, ok := w.talkPoint(tt.from, boothNPC, talkReach)
			if !ok {
				t.Fatal("no talk point")
			}
			if horizontal2(p, boothNPC) > talkReach*talkReach {
				t.Errorf("talk point %+v, %.0f from the NPC, want within %d", p, dist2D(p, boothNPC), talkReach)
			}
			if tt.talkAt != (Point{}) && (p.X != tt.talkAt.X || p.Y != tt.talkAt.Y) {
				t.Errorf("talk point %+v, want the bot's own place %+v", p, tt.talkAt)
			}
			from, ok1 := b.locate(tt.from)
			to, ok2 := b.locate(p)
			if !ok1 || !ok2 || !b.walkable(from, to) {
				t.Errorf("talk point %+v off the bot's ground", p)
			}
		})
	}
}

// TestTownPrepBoothShop walks a farmer back in town on a supply trip, short
// of potions, to a grocer in a booth no straight line reaches (#253): the
// walk to its spawn ends by the booth, the grocer in sight from afar is not
// clicked from there (the click's straight approach would end blocked
// across the town) but from a talk point within talkReach, and the visit
// buys before the preparation gate opens.
func TestTownPrepBoothShop(t *testing.T) {
	b := walkBooth(t, newPrepBrain(t, boothNPC, adenaStack(10000), potionStack(5)), Point{X: 2000, Y: boothNPC.Y})
	b.townArrival()
	rounds := b.shop.rounds
	page := NpcHtmlMessage{ObjectID: merchantObj, HTML: merchantPage(merchantObj, 14)}
	runTrainerSteps(t, b, []trainerStep{
		{name: "arrived: walk toward the grocer's spawn", at: time.Second, wantRule: "shop", wantKind: actMove},
		{
			name: "grocer in sight from afar: walk to a talk point by the booth", at: 40 * time.Second,
			setup:    func(b *Brain) { b.world.Self.move = stopAt(Point{X: 3800, Y: boothNPC.Y}) },
			events:   []Event{NPCInfo{ObjectID: merchantObj, NPCID: 30004, Pos: boothNPC}},
			wantRule: "shop", wantKind: actMove, wantTo: boothNPC, wantWithin: talkReach,
		},
		{
			name: "at the talk point: select", at: 80 * time.Second,
			setup:    func(b *Brain) { b.world.Self.move = stopAt(b.shop.v.stand) },
			wantRule: "shop", wantKind: actSelect,
		},
		{name: "talk", at: 81 * time.Second, events: []Event{MyTargetSelected{ObjectID: merchantObj}}, wantRule: "shop", wantKind: actTalk},
		{name: "wait for the page", at: 82 * time.Second, wantRule: "shop"},
		{name: "buy link", at: 83 * time.Second, events: []Event{page}, wantRule: "shop", wantKind: actBypass, wantPayload: encodeRequestBypassToServer(buyCommand(merchantObj, 14))},
		{name: "wait for the window", at: 84 * time.Second, wantRule: "shop"},
		{
			name: "buy", at: 85 * time.Second,
			events:   []Event{BuyList{Money: 10000, ListID: 14, Items: []ShopItem{potionOffer, {TemplateID: 1835, Price: 8}}}},
			wantRule: "shop", wantKind: actBuyItem,
		},
	})
	if !b.preparing() || b.shop.rounds != rounds {
		t.Errorf("gate armed %v, rounds %d (armed at %d), want the gate held until the visit ends", b.prep.armed, b.shop.rounds, rounds)
	}
}

// TestTownPrepBoothWalkFails checks a walk to a talk point that ends
// unreachable (the walker's watchdog, #253): the grocer behind its closed
// line is not clicked from afar, where the straight approach ends blocked;
// the visit spends a click, waits out walkRetry and walks again.
func TestTownPrepBoothWalkFails(t *testing.T) {
	b := walkBooth(t, newPrepBrain(t, boothNPC, adenaStack(10000), potionStack(5)), Point{X: 3800, Y: boothNPC.Y})
	b.townArrival()
	failed := 41 * time.Second
	runTrainerSteps(t, b, []trainerStep{
		{
			name: "grocer in sight from afar: walk to a talk point", at: time.Second,
			events:   []Event{NPCInfo{ObjectID: merchantObj, NPCID: 30004, Pos: boothNPC}},
			wantRule: "shop", wantKind: actMove, wantTo: boothNPC, wantWithin: talkReach,
		},
		{
			name: "the walk ends unreachable: no click from afar", at: failed,
			setup: func(b *Brain) {
				b.walk.seg = segment{}
				b.walk.unreachable(reasonWatchdog)
			},
			wantRule: "shop",
		},
		{name: "still unreachable: no click", at: failed + 30*time.Second, wantRule: "shop"},
		{name: "walkRetry later: walk again", at: failed + walkRetry + time.Second, wantRule: "shop", wantKind: actMove, wantTo: boothNPC, wantWithin: talkReach},
	})
	if v := &b.shop.v; v.clicks != 1 || !b.preparing() {
		t.Errorf("clicks %d, gate armed %v, want one click spent and the gate held", v.clicks, b.prep.armed)
	}
}
