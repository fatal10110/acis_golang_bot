package botclient

import (
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/craftplan"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The member tests' clan and crafter, and a recipe of the crafter's book
// beside Soulshot D: Iron Ore into Steel, taught by its own scroll.
const (
	memberClan   int32 = 88
	memberCrafty int32 = 0x10000900
	ironRecipe   int32 = 600
	ironScroll   int32 = 7101
	memberName         = "member"
)

// newMemberBrain is a clan member fighter of newMeterBrain with the
// option on, in memberClan, whose crafter has Soulshot D (recipe 20) and
// the iron recipe in its book and makes D-grade shots now. The D sword and
// blade crystallize; no merchant in reach sells Soulshot D.
func newMemberBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newMeterBrain(t, classHumanFight, items...)
	b.world.Clan.ID, b.world.Self.Name = memberClan, memberName
	m := b.cfg.Travel
	m.recipes = map[int32]datapack.Recipe{
		20: shotRecipes[20], 317: shotRecipes[317],
		ironRecipe: {
			ID: ironRecipe, ScrollID: ironScroll, Level: 1, SuccessRate: 100, Dwarven: true,
			Materials: []datapack.Ingredient{{ItemID: ironOre, Count: 2}}, Product: datapack.Ingredient{ItemID: steel, Count: 1},
		},
	}
	m.crystals = map[int32]crystalYield{dSword: {grade: 1, crystal: crystalD, count: 100}, dBlade: {grade: 1, crystal: crystalD, count: 200}}
	m.weights[ironOre], m.weights[ironScroll], m.weights[dSword] = 20, 30, 1500
	b.cfg.CraftBoard = NewCraftBoard()
	b.cfg.CraftBoard.Publish(memberClan, CrafterEntry{Bot: memberCrafty, Name: "crafter", Recipes: []int32{20, ironRecipe}, ShotGrades: []int{1}})
	b.persona.ClanMember = true
	b.wireMember()
	return b
}

// TestMemberHold checks the member's keep hook per carried item: recipe
// scrolls and the materials of the crafter's recipes are kept, graded gear
// the bot would not wear is kept and never worn, and nothing is kept
// outside the option.
func TestMemberHold(t *testing.T) {
	spareSword := Item{ObjectID: 20, TemplateID: dSword, Count: 1, BodySlot: partRHand}
	tests := []struct {
		name  string
		wield int32
		it    Item
		setup func(b *Brain)
		want  Hold
	}{
		{"scroll of a book recipe", dSword, lootStack(10, ironScroll, 1), nil, Hold{Keep: true}},
		{"any recipe scroll", dSword, lootStack(10, 3032, 1), nil, Hold{Keep: true}},
		{"material of a book recipe", dSword, lootStack(10, ironOre, 7), nil, Hold{Keep: true}},
		{"crystal of the shot recipe", dSword, lootStack(10, crystalD, 3), nil, Hold{Keep: true}},
		{"material of no book recipe", dSword, lootStack(10, spiritOre, 9), nil, Hold{}},
		{"material of a recipe whose scroll is in the warehouse", dSword, lootStack(10, spiritOre, 9), func(b *Brain) {
			b.wh.status.Known, b.wh.status.Contents = true, []WarehouseItem{{ObjectID: 0x10000500, TemplateID: 3032, Count: 1}}
		}, Hold{Keep: true}},
		{"unworn graded gear it would not wear", dBlade, spareSword, nil, Hold{Keep: true, NoWear: true}},
		{"graded gear it would wear", ngSword, spareSword, nil, Hold{}},
		{"worn graded gear", dSword, wielded(1, dSword), nil, Hold{}},
		{"ungraded loot", dSword, lootStack(10, 1864, 5), nil, Hold{}},
		{"no crafter on the board", dSword, lootStack(10, ironScroll, 1), func(b *Brain) { b.cfg.CraftBoard = NewCraftBoard() }, Hold{}},
		{"no clan", dSword, lootStack(10, ironOre, 7), func(b *Brain) { b.world.Clan.ID = 0 }, Hold{}},
		{"option off", dSword, lootStack(10, ironOre, 7), func(b *Brain) { b.persona.ClanMember = false }, Hold{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := []Item{wielded(1, tt.wield)}
			if tt.it.ObjectID != 1 {
				items = append(items, tt.it)
			}
			b := newMemberBrain(t, items...)
			if tt.setup != nil {
				tt.setup(b)
			}
			if got := b.hold(tt.it); got != tt.want {
				t.Errorf("hold = %+v, want %+v", got, tt.want)
			}
			if tt.want.NoWear && b.wearsNext(tt.it) {
				t.Error("the gear errand wears a piece kept for crystallizing")
			}
		})
	}
}

// TestMemberShotSource checks the board as a shot source: Soulshot D
// counts while a crafter of the clan makes D-grade shots now, and the
// member buys it then (buyShot); other grades, another clan and the
// option off count not.
func TestMemberShotSource(t *testing.T) {
	tests := []struct {
		name   string
		grades []int
		setup  func(b *Brain)
		want   bool
	}{
		{"grade made now", []int{1}, nil, true},
		{"other grade only", []int{2}, nil, false},
		{"no grade made now", nil, nil, false},
		{"no clan", []int{1}, func(b *Brain) { b.world.Clan.ID = 0 }, false},
		{"option off", []int{1}, func(b *Brain) { b.persona.ClanMember = false }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newMemberBrain(t, wielded(1, dSword))
			b.cfg.CraftBoard.Publish(memberClan, CrafterEntry{Bot: memberCrafty, Name: "crafter", Recipes: []int32{20}, ShotGrades: tt.grades})
			if tt.setup != nil {
				tt.setup(b)
			}
			if got := b.sourced(dSoulshot); got != tt.want {
				t.Errorf("sourced(Soulshot D) = %v, want %v", got, tt.want)
			}
			if got := b.buyShot(b.weaponOf(dSword)) == dSoulshot; got != tt.want {
				t.Errorf("buys Soulshot D = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMemberOrderSizing checks the member's shot orders: its Soulshot D
// trip target less its open orders, the extra supply hook asking the ore
// of that, and the order posted capped at the ore it carries.
func TestMemberOrderSizing(t *testing.T) {
	r := shotRecipes[20]
	tests := []struct {
		name string
		ore  int32
		open int64
	}{
		{"ore for the whole target", 60, 0},
		{"short of ore", 4, 0},
		{"an open order lowers it", 60, 300},
		{"no ore", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := []Item{wielded(1, dSword)}
			if tt.ore > 0 {
				items = append(items, lootStack(3, soulOre, tt.ore))
			}
			b := newMemberBrain(t, items...)
			if tt.open > 0 {
				if _, ok := b.cfg.CraftBoard.Post(memberClan, memberName, dSoulshot, tt.open); !ok {
					t.Fatal("post refused")
				}
			}
			target := int64(b.tripTarget(dSoulshot))
			if target <= tt.open {
				t.Fatalf("trip target %d, want above the open order %d", target, tt.open)
			}
			want := target - tt.open
			if got := b.memberShotWants(); !reflect.DeepEqual(got, map[int32]int64{dSoulshot: want}) {
				t.Fatalf("memberShotWants = %v, want %d Soulshots D", got, want)
			}
			if got, ore := b.persona.Supply.Extra(b.world), shotOre(r, want)[soulOre]; int64(got[soulOre]) != ore {
				t.Errorf("Extra = %v, want %d Soul Ore", got, ore)
			}
			order := min(want, oreShots(r, map[int32]int64{soulOre: int64(tt.ore)}))
			got := b.memberOrders()
			if order == 0 && len(got) != 0 || order > 0 && !reflect.DeepEqual(got, map[int32]int64{dSoulshot: order}) {
				t.Errorf("memberOrders = %v, want %d Soulshots D", got, order)
			}
		})
	}
}

// TestMemberHandOffAndFees checks the weight the member's visit hands off
// (what it deposits: kept materials, scrolls, unworn graded gear, the
// orders' ore) and the deposit fees the shop leaves for it; neither
// outside the option.
func TestMemberHandOffAndFees(t *testing.T) {
	items := []Item{
		wielded(1, dBlade), lootStack(3, soulOre, 60), lootStack(10, ironOre, 7), lootStack(11, ironScroll, 1),
		{ObjectID: 12, TemplateID: dSword, Count: 1, BodySlot: partRHand},
		adenaStack(5000),
	}
	b := newMemberBrain(t, items...)
	orders := b.memberOrders()
	ore := shotOre(shotRecipes[20], orders[dSoulshot])[soulOre]
	if ore == 0 {
		t.Fatal("no order")
	}
	want := 7*20 + 30 + 1500 + ore*10
	if got := b.persona.Supply.HandOff(b.world); got != want {
		t.Errorf("HandOff = %d, want %d", got, want)
	}
	if got := b.shed(); got < want {
		t.Errorf("shed = %d, want at least the hand-off %d", got, want)
	}
	base := int64(b.persona.Shop.KeepAdena)
	if m := b.cfg.Travel; int64(m.cheapest[scrollOfEscape]) > base {
		base = int64(m.cheapest[scrollOfEscape])
	}
	// The fees are reserved from the trip's arrival in town until the
	// visit is settled, never planned on the reserve's own (per tick) path.
	now := at(time.Minute)
	steps := []struct {
		name string
		step func()
		want int64
	}{
		{"before a trip", func() {}, base},
		{"arrived in town", b.townArrival, base + 4*warehouseFee},
		{"visit asked", func() {
			if req := b.persona.Warehouse.Want(b.world, b.wh.status, now); len(req.Deposit) != 4 {
				t.Fatalf("deposit = %+v, want four rows", req.Deposit)
			}
		}, base + 4*warehouseFee},
		{"visit settled", func() { b.ruleMember(now.Add(10 * time.Second)) }, base},
	}
	for _, s := range steps {
		s.step()
		if got := b.reserve(); got != s.want {
			t.Errorf("%s: reserve = %d, want %d", s.name, got, s.want)
		}
	}
	b.world.Clan.ID = 0
	if got := b.persona.Supply.HandOff(b.world); got != 0 {
		t.Errorf("HandOff without a clan = %d, want 0", got)
	}
	b.townArrival()
	if got := b.reserve(); got != base {
		t.Errorf("reserve without a clan = %d, want %d", got, base)
	}
}

// TestMemberVisitLeaseAndRunning checks the visit's lease and settle
// guards: with the board's warehouse lease held by a crafter the visit
// deposits only and leaves that lease alone, and nothing is settled while
// the errand still runs (its deposit after the withdrawal).
func TestMemberVisitLeaseAndRunning(t *testing.T) {
	const crafterObj = 0x5000
	tests := []struct {
		name         string
		crafterLease bool
	}{
		{"lease held by a crafter", true},
		{"errand still running", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newMemberBrain(t, wielded(1, dSword), lootStack(3, soulOre, 60), lootStack(10, ironOre, 7), lootStack(11, ironScroll, 1), adenaStack(5000))
			board, now := b.cfg.CraftBoard, at(time.Minute)
			deliver(t, board, memberName, dSoulshot, 312, now)
			if tt.crafterLease && !board.TakeLease(memberClan, crafterObj, now) {
				t.Fatal("crafter did not take the lease")
			}
			b.townArrival()
			req := b.persona.Warehouse.Want(b.world, b.wh.status, now)
			if len(req.Deposit) == 0 {
				t.Fatalf("visit = %+v, want a deposit", req)
			}
			self := b.world.Self.ObjectID
			if tt.crafterLease {
				if len(req.Withdraw) != 0 {
					t.Errorf("withdraw = %+v, want none without the lease", req.Withdraw)
				}
				if board.HoldsLease(memberClan, self, now) {
					t.Error("member took the crafter's lease")
				}
			} else if len(req.Withdraw) == 0 || !board.HoldsLease(memberClan, self, now) {
				t.Fatalf("visit = %+v, want a withdrawal under the lease", req)
			}
			end := now.Add(10 * time.Second)
			b.wh.status.Withdraw = WarehouseResult{Outcome: WarehouseDone, At: end, Rows: []WarehouseRow{{TemplateID: dSoulshot, Count: 312}}}
			if !tt.crafterLease {
				// The withdrawal ended, the deposit still runs.
				b.wh.running = true
				b.ruleMember(end)
				if !b.member.asked || !board.HoldsLease(memberClan, self, end) {
					t.Fatal("visit settled while the errand runs")
				}
				if got := board.Owned(memberClan, memberName); got[dSoulshot] != 312 {
					t.Errorf("owned while running = %v, want 312 Soulshots D", got)
				}
				b.wh.running = false
			}
			b.ruleMember(end)
			wantOwned := map[int32]int64{}
			if tt.crafterLease {
				wantOwned = map[int32]int64{dSoulshot: 312}
			}
			if got := board.Owned(memberClan, memberName); !reflect.DeepEqual(got, wantOwned) {
				t.Errorf("owned = %v, want %v", got, wantOwned)
			}
			if board.HoldsLease(memberClan, self, end) {
				t.Error("member still holds the lease")
			}
			if tt.crafterLease && !board.HoldsLease(memberClan, crafterObj, end) {
				t.Error("member released the crafter's lease")
			}
			if b.member.asked {
				t.Error("visit not settled")
			}
		})
	}
}

// deliver puts n of product in clan's ledger for member, as a crafter's
// deposit does.
func deliver(t *testing.T, board *CraftBoard, member string, product int32, n int64, now time.Time) {
	t.Helper()
	id, ok := board.Post(memberClan, member, product, n)
	if !ok || !board.MaterialsIn(memberClan, id) || !board.Claim(memberClan, id, memberCrafty, now) || !board.Crafted(memberClan, id, memberCrafty, now) {
		t.Fatal("order not crafted")
	}
	if _, ok := board.Deliver(memberClan, id, memberCrafty, n); !ok {
		t.Fatal("order not delivered")
	}
}

// TestMemberVisit runs the member's warehouse visit on a supply trip's
// arrival in town: it withdraws its delivered counts under the lease and
// deposits what it keeps with its orders' ore; once the errand ended, the
// withdrawn counts leave its ledger, the orders the deposited ore covers
// are posted with their materials in, and the lease is free.
func TestMemberVisit(t *testing.T) {
	r := shotRecipes[20]
	tests := []struct {
		name string
		// depositOre is the share of the orders' ore the deposit moved, in
		// percent; withdraw the withdrawal's outcome.
		depositOre int64
		withdraw   WarehouseOutcome
		wantOwned  map[int32]int64
	}{
		{"both moved", 100, WarehouseDone, map[int32]int64{}},
		{"part of the ore moved", 50, WarehouseDone, map[int32]int64{}},
		{"withdrawal unanswered", 100, WarehouseTimeout, map[int32]int64{dSoulshot: 312}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newMemberBrain(t, wielded(1, dSword), lootStack(3, soulOre, 60), lootStack(10, ironOre, 7), lootStack(11, ironScroll, 1), adenaStack(5000))
			board, now := b.cfg.CraftBoard, at(time.Minute)
			deliver(t, board, memberName, dSoulshot, 312, now)
			if got := b.persona.Warehouse.Want(b.world, b.wh.status, now); !got.empty() {
				t.Fatalf("Want before a trip = %+v, want none", got)
			}
			b.townArrival()
			req := b.persona.Warehouse.Want(b.world, b.wh.status, now)
			orders := b.memberOrders()
			ore := shotOre(r, orders[dSoulshot])[soulOre]
			want := WarehouseRequest{
				Withdraw: []WarehouseRow{{TemplateID: dSoulshot, Count: 312}},
				Deposit:  []WarehouseRow{{TemplateID: ironOre, Count: 7, Object: 10}, {TemplateID: ironScroll, Count: 1, Object: 11}, {TemplateID: soulOre, Count: int32(ore)}},
			}
			if !reflect.DeepEqual(req, want) {
				t.Fatalf("visit = %+v, want %+v", req, want)
			}
			if !board.HoldsLease(memberClan, b.world.Self.ObjectID, now) {
				t.Fatal("withdrawal without the warehouse lease")
			}
			if got := b.persona.Warehouse.Want(b.world, b.wh.status, now); !got.empty() {
				t.Fatalf("second Want = %+v, want none while the visit runs", got)
			}
			end := now.Add(10 * time.Second)
			b.wh.status.Withdraw = WarehouseResult{Outcome: tt.withdraw, At: end}
			if tt.withdraw == WarehouseDone {
				b.wh.status.Withdraw.Rows = []WarehouseRow{{TemplateID: dSoulshot, Count: 312, Object: 0x10000600}}
				b.wh.status.Deposit = WarehouseResult{Outcome: WarehouseDone, At: end, Rows: []WarehouseRow{
					{TemplateID: ironOre, Count: 7, Object: 10}, {TemplateID: ironScroll, Count: 1, Object: 11}, {TemplateID: soulOre, Count: int32(ore * tt.depositOre / 100), Object: 3},
				}}
			} else {
				b.wh.status.Deposit = WarehouseResult{Outcome: tt.withdraw, At: end}
			}
			if _, ok := b.ruleMember(end); ok {
				t.Fatal("ruleMember took the turn")
			}
			if got := board.Owned(memberClan, memberName); !reflect.DeepEqual(got, tt.wantOwned) {
				t.Errorf("owned = %v, want %v", got, tt.wantOwned)
			}
			if board.HoldsLease(memberClan, b.world.Self.ObjectID, end) {
				t.Error("lease still held")
			}
			var posted []CraftOrder
			for _, o := range board.Orders(memberClan, end) {
				if o.Member == memberName {
					posted = append(posted, o)
				}
			}
			wantCount := int64(0)
			if tt.withdraw == WarehouseDone {
				wantCount = min(orders[dSoulshot], oreShots(r, map[int32]int64{soulOre: ore * tt.depositOre / 100}))
			}
			switch {
			case wantCount == 0 && len(posted) != 0:
				t.Errorf("posted %+v, want none without the ore deposited", posted)
			case wantCount > 0 && (len(posted) != 1 || posted[0].Product != dSoulshot || posted[0].Count != wantCount || posted[0].State != OrderMaterialsIn):
				t.Errorf("posted %+v, want %d Soulshots D with their materials in", posted, wantCount)
			}
			// The next trip's visit: after an unanswered withdrawal it
			// deposits only.
			b.townArrival()
			next := b.persona.Warehouse.Want(b.world, b.wh.status, end.Add(time.Minute))
			if tt.withdraw == WarehouseTimeout && len(next.Withdraw) != 0 {
				t.Errorf("visit after an unanswered withdrawal = %+v, want a deposit only", next)
			}
		})
	}
}

// TestMemberNothingOutsideOption checks that without a crafter on the
// board a trip's arrival asks no visit and the hooks add nothing.
func TestMemberNothingOutsideOption(t *testing.T) {
	b := newMemberBrain(t, wielded(1, dSword), lootStack(3, soulOre, 60), lootStack(10, ironOre, 7))
	b.cfg.CraftBoard = NewCraftBoard()
	b.townArrival()
	if got := b.persona.Warehouse.Want(b.world, b.wh.status, at(time.Minute)); !got.empty() {
		t.Errorf("Want = %+v, want none", got)
	}
	if got := b.persona.Supply.Extra(b.world); len(got) != 0 {
		t.Errorf("Extra = %v, want none", got)
	}
	if got := b.persona.Supply.HandOff(b.world); got != 0 {
		t.Errorf("HandOff = %d, want 0", got)
	}
}

// TestShotGradesNow checks the shot grades a crafter publishes: a shot
// recipe in its book within its level, with the crystals of a craft in its
// pool or the shots in the warehouse's stock.
func TestShotGradesNow(t *testing.T) {
	tests := []struct {
		name     string
		level    int32
		inv      map[int32]int64
		reserved map[int32]int64
		stock    []WarehouseItem
		want     []int
	}{
		{"crystal carried", 2, map[int32]int64{crystalD: 1}, nil, nil, []int{1}},
		{"crystal in the warehouse", 2, nil, nil, []WarehouseItem{{TemplateID: crystalD, Count: 4}}, []int{1}},
		{"no crystal", 2, nil, nil, nil, nil},
		{"crystal reserved", 2, nil, map[int32]int64{crystalD: 4}, []WarehouseItem{{TemplateID: crystalD, Count: 4}}, nil},
		{"shots in stock", 2, nil, nil, []WarehouseItem{{TemplateID: dSoulshot, Count: 500}}, []int{1}},
		{"level too low", 1, map[int32]int64{crystalD: 1}, nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newMemberBrain(t)
			b.wh.status.Known, b.wh.status.Contents = true, tt.stock
			in := craftplan.Input{Book: []int32{20, ironRecipe}, Recipes: b.cfg.Travel.recipes, DwarvenLevel: tt.level, Inventory: tt.inv, Reserved: tt.reserved}
			if got := b.shotGradesNow(in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("shotGradesNow = %v, want %v", got, tt.want)
			}
		})
	}
}
