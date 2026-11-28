package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestShopTeleportedVisit replays the frames TestLiveClassChangeDyes
// recorded (#732): a new Warrior walking to Talking Island's grocer to sell
// its junk is recalled by the GM to Gludin (TeleportToLocation, not Fast),
// where the grocer's spawn is out of every walk's reach and Trader Poesia
// (30315, list 68) is near. The visit is dropped, the grocer not shunned,
// and the next visit, to Poesia, starts at the landing: before the fix the
// walk to the grocer's spawn ended unreachable, the visit shunned it and
// held the shop errand shopRetry, and the bot stood five minutes in Gludin
// (11:00:12.97 to 11:05:12.97 of the record). A Fast teleport is an
// in-place correction: the visit goes on. On the split navigation the
// grocer's spawn (5000, 0) is west of the wall and the landing east of it.
func TestShopTeleportedVisit(t *testing.T) {
	landing := Point{X: 9000, Y: 1000}
	poesia := Point{X: 9400, Y: 1000}
	grocer := Point{X: 5000}
	tests := []struct {
		name   string
		to     TeleportToLocation
		wantTo Point
	}{
		{name: "recalled across the wall", to: TeleportToLocation{ObjectID: selfID, Pos: landing}, wantTo: poesia},
		{name: "fast correction in place", to: TeleportToLocation{ObjectID: selfID, Pos: Point{X: 100}, Fast: true}, wantTo: grocer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := walkSplit(t, newShopBrain(t, adenaStack(5000), potionsOf(2), skin, sword))
			shops, cheapest := indexShops([]datapack.BuyList{
				{ID: 14, NPCID: 30004, Products: []datapack.Product{{ItemID: 1060, Price: 103}}},
				{ID: 68, NPCID: 30315, Products: []datapack.Product{{ItemID: 1060, Price: 103}}},
			})
			spawns := map[int32][]Point{30004: {grocer}, 30315: {poesia}}
			tr := b.cfg.Travel
			tr.shops, tr.cheapest, tr.spawns, tr.spawnedSold = shops, cheapest, spawns, spawnedSold(shops, spawns)
			runTrainerSteps(t, b, []trainerStep{
				{name: "leg to the grocer's spawn", at: time.Second, wantRule: "shop", wantKind: actMove, wantTo: grocer, wantWithin: visitArrive},
				{name: "teleported", at: 3 * time.Second, events: []Event{tt.to}, wantRule: "shop", wantKind: actMove, wantTo: tt.wantTo, wantWithin: visitArrive},
			})
			if b.shop.shunned.held(30004, at(3*time.Second)) {
				t.Errorf("grocer shunned until %v, want it left as it was", b.shop.shunned[30004])
			}
			// The drop ends no round: the town preparation gate holds.
			if b.shop.rounds != 0 {
				t.Errorf("shop rounds = %d, want 0", b.shop.rounds)
			}
		})
	}
}
