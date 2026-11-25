package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestShopVisitLeavesWantsElsewhere replays the merchant visit of a live
// TestLiveTownErrands run (#441): with 39004 adena past a 19000 floor and
// 993 of the 10000 Lesser Healing Potions it restocks to, the bot visited
// the merchant in sight first, which sells none of them, bought there, and
// waited shopRetry for its next visit; a town hop in between spent the
// adena the potions needed. A visit that leaves a want it can afford that
// another merchant sells is followed at once by a visit to that merchant;
// the merchant just visited waits shopRetry.
func TestShopVisitLeavesWantsElsewhere(t *testing.T) {
	const (
		woodenArrow int32 = 17
		armorer     int32 = 30001
		armorerObj  int32 = 0x20000041
	)
	p := DefaultPersona()
	p.IdleChance, p.AutoShots = 0, false
	p.Shop.Restock = []Restock{{Item: lesserHealingPotion, Min: 10000, Max: 10000}, {Item: woodenArrow, Min: 100, Max: 100}}
	p.Shop.KeepAdena = 19000
	b, _ := newTestBrain(t, p)
	b.world.Self.ClassID, b.world.Self.Level = classHumanMystic, 15
	shops, cheapest := indexShops([]datapack.BuyList{
		{ID: 13, NPCID: armorer, Products: []datapack.Product{{ItemID: woodenArrow, Price: 2}}},
		{ID: 14, NPCID: 30004, Products: []datapack.Product{{ItemID: lesserHealingPotion, Price: 103}}},
	})
	spawns := map[int32][]Point{armorer: {{X: 300}}, 30004: {{X: 5000}}}
	b.cfg.Travel = &TravelMap{spawns: spawns, shops: shops, cheapest: cheapest, spawnedSold: spawnedSold(shops, spawns), trainers: testTrainers}
	b.world.Apply(ItemList{Items: []Item{adenaStack(39004), potionsOf(993)}}, t0)
	b = walkOn(t, b)

	page := NpcHtmlMessage{ObjectID: armorerObj, HTML: merchantPage(armorerObj, 13)}
	runTrainerSteps(t, b, []trainerStep{
		{
			name: "armorer in sight: select", at: time.Second,
			events:   []Event{NPCInfo{ObjectID: armorerObj, NPCID: armorer, Pos: Point{X: 300}}},
			wantRule: "shop", wantKind: actSelect,
		},
		{name: "talk", at: 2 * time.Second, events: []Event{MyTargetSelected{ObjectID: armorerObj}}, wantRule: "shop", wantKind: actTalk},
		{name: "buy link", at: 3 * time.Second, events: []Event{page}, wantRule: "shop", wantKind: actBypass, wantPayload: encodeRequestBypassToServer(buyCommand(armorerObj, 13))},
		{
			name: "buy the arrows", at: 4 * time.Second,
			events:   []Event{BuyList{Money: 39004, ListID: 13, Items: []ShopItem{{Category: 5, ObjectID: woodenArrow, TemplateID: woodenArrow, SubCategory: 5, Price: 2}}}},
			wantRule: "shop", wantKind: actBuyItem, wantPayload: encodeRequestBuyItem(13, []buyRow{{woodenArrow, 100}}),
		},
		{
			name: "bought: the visit ends", at: 5 * time.Second,
			events: []Event{ItemList{Items: []Item{adenaStack(38804), potionsOf(993), {ObjectID: 107, TemplateID: woodenArrow, Count: 100, SubCategory: 5}}}},
		},
		{name: "the potions: leg to the grocer's spawn", at: 6 * time.Second, wantRule: "shop", wantKind: actMove},
	})
	if b.shop.v.npcID != 30004 || !b.shop.shunned[armorer].Equal(at(5*time.Second+shopRetry)) {
		t.Fatalf("visit to %d, armorer left alone until %v; want the grocer's, the armorer's shopRetry after its visit", b.shop.v.npcID, b.shop.shunned[armorer].Sub(t0))
	}
}
