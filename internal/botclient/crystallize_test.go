package botclient

import (
	"bytes"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The crystallize tests' gear: a D weapon whose 1170 crystals (650 each)
// are worth more than its 644000 adena, D boots worth less as crystals, a
// C weapon worth it at Crystallize 2, and an unpriced D piece.
const (
	czWeapon   int32 = 69
	czBoots    int32 = 40
	czCWeapon  int32 = 71
	czUnpriced int32 = 72
	crystalC   int32 = 1459
)

func czMap() *TravelMap {
	return &TravelMap{
		crystals: map[int32]crystalYield{
			czWeapon: {1, crystalD, 1170}, czBoots: {1, crystalD, 38}, czCWeapon: {2, crystalC, 300}, czUnpriced: {1, crystalD, 100},
		},
		prices:  map[int32]int32{czWeapon: 644000, czBoots: 30000, czCWeapon: 500000, crystalD: 650, crystalC: 3000},
		weights: map[int32]int32{czWeapon: 1500, czBoots: 1000, czCWeapon: 1500},
	}
}

func TestCrystallizeEncoder(t *testing.T) {
	got := encodeRequestCrystallizeItem(0x10000400, 1)
	if want := []byte{0x72, 0x00, 0x04, 0x00, 0x10, 1, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("payload % x, want % x", got, want)
	}
	a := crystallizeAction(500)
	if a.kind != actCrystallize || a.object != 500 || a.gate() != gateCraft {
		t.Fatalf("crystallizeAction = %+v, want a crystallize of 500 through the craft gate", a)
	}
}

// TestIndexCrystals keeps the gear the server crystallizes: a grade and a
// crystal count, never a hero or shadow item, never an arrow.
func TestIndexCrystals(t *testing.T) {
	items := map[int32]datapack.Item{
		czWeapon:  {ID: czWeapon, Type: "Weapon", Grade: 1, CrystalCount: 1170},
		czCWeapon: {ID: czCWeapon, Type: "Armor", Grade: 2, CrystalCount: 300},
		1:         {ID: 1, Type: "Weapon", CrystalCount: 10},
		2:         {ID: 2, Type: "Weapon", Grade: 1},
		6611:      {ID: 6611, Type: "Weapon", Grade: 5, CrystalCount: 100},
		3:         {ID: 3, Type: "Weapon", Grade: 1, CrystalCount: 10, Duration: 60},
		4:         {ID: 4, Type: "EtcItem", Grade: 1, CrystalCount: 10},
	}
	want := map[int32]crystalYield{czWeapon: {1, crystalD, 1170}, czCWeapon: {2, crystalC, 300}}
	if got := indexCrystals(items); !reflect.DeepEqual(got, want) {
		t.Fatalf("indexCrystals = %v, want %v", got, want)
	}
}

// TestCrystallizeWant checks what the crafter withdraws to crystallize:
// crystals worth more than the item, within the skill's grade, +0 and
// unaugmented, never the ledger's counts, bounded by the cycle's cap and
// the weight room.
func TestCrystallizeWant(t *testing.T) {
	two := []WarehouseItem{{ObjectID: 1, TemplateID: czWeapon, Count: 1}, {ObjectID: 2, TemplateID: czWeapon, Count: 1}}
	all := append(slices.Clone(two),
		WarehouseItem{ObjectID: 3, TemplateID: czBoots, Count: 1},
		WarehouseItem{ObjectID: 4, TemplateID: czCWeapon, Count: 1},
		WarehouseItem{ObjectID: 5, TemplateID: czUnpriced, Count: 1},
		WarehouseItem{ObjectID: 6, TemplateID: crystalD, Count: 50},
	)
	many := make([]WarehouseItem, 0, 30)
	for i := range int32(30) {
		many = append(many, WarehouseItem{ObjectID: 100 + i, TemplateID: czWeapon, Count: 1})
	}
	tests := []struct {
		name     string
		contents []WarehouseItem
		reserved map[int32]int64
		level    int32
		room     int64
		want     map[int32]int32
	}{
		{"no skill (D18)", all, nil, 0, 0, nil},
		{"Artisan: D worth it only", all, nil, 1, 0, map[int32]int32{czWeapon: 2}},
		{"Warsmith: C too", all, nil, 2, 0, map[int32]int32{czWeapon: 2, czCWeapon: 1}},
		{"a member's ledger item untouched", two, map[int32]int64{czWeapon: 1}, 1, 0, map[int32]int32{czWeapon: 1}},
		{"every one the ledger's", two, map[int32]int64{czWeapon: 2}, 1, 0, nil},
		{
			"enchanted and augmented stay whole",
			[]WarehouseItem{{ObjectID: 1, TemplateID: czWeapon, Count: 1, Enchant: 3}, {ObjectID: 2, TemplateID: czWeapon, Count: 1, Augmentation: 7}},
			nil, 1, 0, nil,
		},
		{
			"the ledger owns the +0 one first",
			[]WarehouseItem{{ObjectID: 1, TemplateID: czWeapon, Count: 1, Enchant: 3}, {ObjectID: 2, TemplateID: czWeapon, Count: 1}},
			map[int32]int64{czWeapon: 1},
			1, 0, nil,
		},
		{
			"a +0 beside an enchanted one stays whole",
			[]WarehouseItem{{ObjectID: 1, TemplateID: czWeapon, Count: 1, Enchant: 3}, {ObjectID: 2, TemplateID: czWeapon, Count: 1}, {ObjectID: 3, TemplateID: czCWeapon, Count: 1}},
			nil, 2, 0,
			map[int32]int32{czCWeapon: 1},
		},
		{"the weight room", two, nil, 1, 2000, map[int32]int32{czWeapon: 1}},
		{"the cycle's cap", many, nil, 1, 0, map[int32]int32{czWeapon: crystallizePerCycle}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := czMap().crystallizeWant(tt.contents, tt.reserved, tt.level, tt.room); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("crystallizeWant = %v, want %v", got, tt.want)
			}
		})
	}
}

// newCzBrain is a crafter with Crystallize 1, the soulshot recipes in its
// travel map (crystals are craft items) and the crystallize test gear,
// carrying items.
func newCzBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newCrafterBrain(t, items...)
	m, cz := b.cfg.Travel, czMap()
	m.crystals, m.weights = cz.crystals, cz.weights
	for id, p := range cz.prices {
		m.prices[id] = p
	}
	for id, r := range shotRecipes {
		m.recipes[id] = r
	}
	m.craftItems = indexCraftItems(m.recipes)
	b.world.Skills[skillCrystallize] = KnownSkill{ID: skillCrystallize, Level: 1}
	return b
}

var (
	ownWeapon = Item{ObjectID: 400, TemplateID: czWeapon, Count: 1}
	gotWeapon = Item{ObjectID: 500, TemplateID: czWeapon, Count: 1}
	gotOther  = Item{ObjectID: 501, TemplateID: czWeapon, Count: 1}
)

// crystallizingBrain has planned from a warehouse holding two D weapons and
// withdrawn them; it carries a spare of its own.
func crystallizingBrain(t *testing.T) *Brain {
	t.Helper()
	b := newCzBrain(t, adenaStack(1000), ownWeapon)
	b.wh.status.Known = true
	b.wh.status.Contents = []WarehouseItem{
		{ObjectID: 0x10000500, TemplateID: czWeapon, Count: 1}, {ObjectID: 0x10000501, TemplateID: czWeapon, Count: 1},
	}
	b.craft.begin(craftLook, WarehouseRequest{Look: true}, t0)
	b.craftPlan(t0, true)
	if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{State: ItemAdded, Item: gotWeapon}, {State: ItemAdded, Item: gotOther}}}}, t0); err != nil {
		t.Fatal(err)
	}
	b.startMake(t0)
	return b
}

// TestBrainCrystallizePlan checks the look's plan: the weapons withdrawn
// with the plan, their crystals planned with as carried and gone from the
// warehouse the planner reads.
func TestBrainCrystallizePlan(t *testing.T) {
	b := newCzBrain(t, adenaStack(1000), ownWeapon)
	b.wh.status.Known = true
	b.wh.status.Contents = []WarehouseItem{{ObjectID: 0x10000500, TemplateID: czWeapon, Count: 2}}
	b.craft.begin(craftLook, WarehouseRequest{Look: true}, t0)
	b.craftPlan(t0, true)
	c := &b.craft
	if c.phase != craftWithdraw || !reflect.DeepEqual(c.req.Withdraw, []WarehouseRow{{TemplateID: czWeapon, Count: 2}}) {
		t.Fatalf("phase %d withdraw %+v, want the two weapons", c.phase, c.req.Withdraw)
	}
	c.phase = craftLook
	in := b.craftInput(nil, true, t0)
	if in.Inventory[crystalD] != 2*1170 || in.Warehouse[czWeapon] != 0 {
		t.Fatalf("planner inventory %v warehouse %v, want 2340 crystals carried and no weapon", in.Inventory, in.Warehouse)
	}
}

// TestBrainCrystallize crystallizes the withdrawn weapons one per answer,
// craftGap apart, never the bot's own spare, then deposits the crystals.
func TestBrainCrystallize(t *testing.T) {
	b := crystallizingBrain(t)
	rule, a := b.decide(t0)
	if rule != "craft" || a.kind != actCrystallize || !bytes.Equal(a.payload, encodeRequestCrystallizeItem(gotWeapon.ObjectID, 1)) {
		t.Fatalf("decide = %q %d % x, want the first withdrawn weapon crystallized", rule, a.kind, a.payload)
	}
	if ok, err := b.send(a, t0); err != nil || !ok {
		t.Fatalf("send = %v, %v", ok, err)
	}
	if rule, a := b.decide(t0.Add(time.Second)); rule != "craft" || a.kind != actHold {
		t.Fatalf("waiting for the answer: %q %d, want a hold", rule, a.kind)
	}
	crystals := Item{ObjectID: 600, TemplateID: crystalD, Count: 1170}
	if err := b.handle([]Event{
		SystemMessage{ID: msgItemCrystallized},
		InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: gotWeapon}, {State: ItemAdded, Item: crystals}}},
	}, t0.Add(100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if rule, a := b.decide(t0.Add(craftGap - time.Millisecond)); rule != "craft" || a.kind != actHold {
		t.Fatalf("inside craftGap: %q %d, want a hold", rule, a.kind)
	}
	next := t0.Add(craftGap + craftJitter)
	rule, a = b.decide(next)
	if a.kind != actCrystallize || a.object != gotOther.ObjectID {
		t.Fatalf("decide = %q %d %d, want the second withdrawn weapon", rule, a.kind, a.object)
	}
	if ok, err := b.send(a, next); err != nil || !ok {
		t.Fatalf("send = %v, %v", ok, err)
	}
	crystals.Count = 2340
	if err := b.handle([]Event{
		SystemMessage{ID: msgItemCrystallized},
		InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: gotOther}, {State: ItemModified, Item: crystals}}},
	}, next.Add(100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	b.decide(next.Add(craftGap + craftJitter))
	c := &b.craft
	if c.phase != craftDeposit || !reflect.DeepEqual(c.req.Deposit, []WarehouseRow{{TemplateID: crystalD, Count: 2340}}) {
		t.Fatalf("phase %d deposit %+v, want the crystals deposited, the spare kept", c.phase, c.req.Deposit)
	}
}

// TestBrainCrystallizeAnswers takes the other answers: the skill too low
// stops the crystallizing and deposits the weapons whole; an unanswered
// request moves on to the next weapon and never asks again; TimeoutStreak
// of them trip the breaker.
func TestBrainCrystallizeAnswers(t *testing.T) {
	t.Run("level too low", func(t *testing.T) {
		b := crystallizingBrain(t)
		sendCz(t, b, t0, gotWeapon.ObjectID)
		if err := b.handle([]Event{SystemMessage{ID: msgCrystallizeLevelTooLow}}, t0.Add(100*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		b.decide(t0.Add(time.Second))
		c := &b.craft
		if c.fails != 1 || c.phase != craftDeposit || !reflect.DeepEqual(c.req.Deposit, bothWhole) {
			t.Fatalf("fails %d phase %d deposit %+v, want both weapons deposited whole", c.fails, c.phase, c.req.Deposit)
		}
	})
	t.Run("timeouts", func(t *testing.T) {
		b := crystallizingBrain(t)
		b.persona.Crafter.TimeoutStreak = 2
		sendCz(t, b, t0, gotWeapon.ObjectID)
		if rule, a := b.decide(t0.Add(crystallizeTimeout - time.Millisecond)); rule != "craft" || a.kind != actHold {
			t.Fatalf("before the timeout: %q %d, want a hold", rule, a.kind)
		}
		later := t0.Add(crystallizeTimeout)
		sendCz(t, b, later, gotOther.ObjectID)
		b.decide(later.Add(crystallizeTimeout))
		c := &b.craft
		if !c.tripped || c.phase != craftDeposit || !reflect.DeepEqual(c.req.Deposit, bothWhole) {
			t.Fatalf("tripped %v phase %d deposit %+v, want the breaker and both weapons deposited", c.tripped, c.phase, c.req.Deposit)
		}
	})
}

// bothWhole deposits the two withdrawn weapons, never the bot's own spare.
var bothWhole = []WarehouseRow{{TemplateID: czWeapon, Count: 1, Object: gotWeapon.ObjectID}, {TemplateID: czWeapon, Count: 1, Object: gotOther.ObjectID}}

// sendCz decides and sends the crystallizing of object at now.
func sendCz(t *testing.T, b *Brain, now time.Time, object int32) {
	t.Helper()
	rule, a := b.decide(now)
	if rule != "craft" || a.kind != actCrystallize || a.object != object {
		t.Fatalf("decide at %v = %q %d %d, want a crystallize of %d", now.Sub(t0), rule, a.kind, a.object, object)
	}
	if ok, err := b.send(a, now); err != nil || !ok {
		t.Fatalf("send = %v, %v", ok, err)
	}
}

// TestBrainCrafterDepositsOrderedShots checks that the products of a
// crafted shot order, which the crafting stock leaves out as the bot's own
// supplies, are deposited as many as the order asks, which delivers it.
func TestBrainCrafterDepositsOrderedShots(t *testing.T) {
	b := newCzBrain(t, adenaStack(1000), Item{ObjectID: 700, TemplateID: soulshotD, Count: 200})
	b.cfg.Travel.shots = map[int32]shotItem{soulshotD: {}}
	board, clan := b.cfg.CraftBoard, b.world.Clan.ID
	id, ok := board.PostCommand(clan, OrderByGM, "gm", soulshotD, 156)
	board.MaterialsIn(clan, id)
	if !ok || !board.Claim(clan, id, selfID, t0) || !board.Crafted(clan, id, selfID, t0) {
		t.Fatal("order not crafted on the board")
	}
	b.startMake(t0)
	c := &b.craft
	if c.phase != craftDeposit || !reflect.DeepEqual(c.req.Deposit, []WarehouseRow{{TemplateID: soulshotD, Count: 156}}) {
		t.Fatalf("phase %d deposit %+v, want the 156 ordered soulshots", c.phase, c.req.Deposit)
	}
	if c.delivering[id] != 156 {
		t.Fatalf("delivering %v, want the order's 156", c.delivering)
	}
}
