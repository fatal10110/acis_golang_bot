package botclient

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// The basin, a terrain of its own in region-local cells: a floor
// basinFloor deep over columns 300-309 and rows 300-309, walled by its own
// depth on every side but the west, where a stair of three columns, each
// basinStep lower than the last, leads down from the ground. A step rises
// more than geotest.Climb, so the stair goes down only: the floor and the
// stair are isolated, as the slope below the #278 kill site is.
const (
	basinStep  = 56
	basinFloor = -3 * basinStep
)

var (
	basinOnce sync.Once
	basinNav  *Nav
	basinErr  error
)

func basinTestNav(tb testing.TB) *Nav {
	tb.Helper()
	basinOnce.Do(func() {
		t := (&geotest.Terrain{}).
			Raise(297, 300, 297, 309, -basinStep).
			Raise(298, 300, 298, 309, -2*basinStep).
			Raise(299, 300, 309, 309, basinFloor)
		basinNav, basinErr = buildTestNav(t, navWindow(1, 1, 2, 1))
	})
	if basinErr != nil {
		tb.Fatal(basinErr)
	}
	return basinNav
}

// basinPoint is region-local cell (x, y) at height z as a world point.
func basinPoint(x, y, z int) Point {
	return scenePoint(geotest.Point{CellX: x, CellY: y, Z: z}, 0, 0)
}

// TestLootKeepsToSCC pins #278: a straight pick-up the server walks the
// bot down holds CanMove and the step chain, yet strands the bot when the
// item lies outside its SCC. On the live kill site the bot at
// (-87183,247106,-2976), cell {2743 31828 -2976} in its SCC, picked up
// adena at (-87241,247119,-3040), cell {2739 31828 -3040}: isolated, with
// no in-SCC cell within 32 cells that a straight segment or a cell route
// reaches back (offline probe on the live geodata and nav cache). Down the
// basin's stair the item is given up instead; one on the bot's own ground,
// the basin's floor included for a bot already on it, is still picked up.
func TestLootKeepsToSCC(t *testing.T) {
	n := basinTestNav(t)
	tests := []struct {
		name     string
		self     Point
		item     Point
		itemKind nav.Kind
		same     bool
	}{
		{name: "down the stair onto the floor", self: basinPoint(290, 305, 0), item: basinPoint(302, 305, basinFloor), itemKind: nav.Isolated},
		{name: "down onto the stair", self: basinPoint(290, 304, 0), item: basinPoint(298, 304, -2*basinStep), itemKind: nav.Isolated},
		{name: "on the basin's floor", self: basinPoint(301, 305, basinFloor), item: basinPoint(308, 305, basinFloor), itemKind: nav.Isolated, same: true},
		{name: "on open ground", self: basinPoint(290, 305, 0), item: basinPoint(290, 296, 0), itemKind: nav.InSCC, same: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _, _ := newNavBrain(t, n, Persona{Loot: defaultLootPolicy()}, tt.self)
			w := b.walker()
			if !w.segmentOK(tt.self, tt.item) {
				t.Fatalf("segment %+v -> %+v closed: the case needs the straight walk open", tt.self, tt.item)
			}
			if within3(tt.self, tt.item, pickupStand) {
				t.Fatalf("item %+v within %d of %+v: the server would not walk", tt.item, pickupStand, tt.self)
			}
			sc, _ := w.plan.Locate(toLoc(tt.self))
			ic, _ := w.plan.Locate(toLoc(tt.item))
			sm, err1 := n.Hier.Locate(sc, nil)
			im, err2 := n.Hier.Locate(ic, nil)
			if err1 != nil || err2 != nil || im.Kind() != tt.itemKind || n.Hier.SameSCC(sm, im) != tt.same {
				t.Fatalf("item kind %v same SCC %v (%v, %v), want %v %v", im.Kind(), n.Hier.SameSCC(sm, im), err1, err2, tt.itemKind, tt.same)
			}

			delete(b.world.Objects, npcID)
			b.world.Ground = map[int32]GroundItem{engageItem: {ObjectID: engageItem, TemplateID: 57, Pos: tt.item, Count: 1}}
			b.loot.items = []int32{engageItem}
			a, ok := b.ruleLoot(t0.Add(time.Second))
			switch {
			case tt.same && (!ok || a.kind != actPickup):
				t.Errorf("ruleLoot = kind %v ok %v (%s), want the pick-up", a.kind, ok, b.walk.reason)
			case !tt.same && (ok || len(b.loot.items) != 0):
				t.Errorf("ruleLoot = kind %v ok %v, queue %v, want the item given up", a.kind, ok, b.loot.items)
			}
		})
	}
}

// TestWalkerIsolatedRecallHolds has a bot on isolated ground ask /unstuck
// and then, while the server's five-minute recall cast runs
// (network/usercommand.go userCommandEscape), hold every walk, a new
// destination's included, instead of ending it unreachable: the target a
// walk is for is not given up while the recall may still land.
func TestWalkerIsolatedRecallHolds(t *testing.T) {
	n := testNav(t)
	b, _, _ := newNavBrain(t, n, Persona{}, scenePoint(geotest.PitInside, 0, 0))
	if a, _ := walkStep(t, b, scenePoint(geotest.PitOutside, 0, 0), t0); a.kind != actUserCommand {
		t.Fatalf("kind %v (%s), want /unstuck", a.kind, b.walk.reason)
	}
	b.world.Self.castEnd = t0.Add(5 * time.Minute)
	for _, at := range []time.Duration{time.Second, 2 * time.Minute, 4 * time.Minute} {
		dest := scenePoint(geotest.PitOutside, 0, int(at/time.Minute)+1)
		if a, st := b.goTo(dest, walkSlop, walkMelee, t0.Add(at)); a.kind != actHold || st != walkWalking {
			t.Fatalf("at %v: kind %v status %v (%s), want a hold through the recall", at, a.kind, st, b.walk.reason)
		}
	}
}
