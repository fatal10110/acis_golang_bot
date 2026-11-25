package botclient

import (
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Made-up light armor of the set tests: set S (setChest, setLegs, setHead,
// setShield), the full armor set F (fullChest, fullHead), and pieces
// outside any set.
const (
	setChest  int32 = 91001 // P. Def 30
	setLegs   int32 = 91002 // P. Def 20
	setHead   int32 = 91003 // P. Def 10
	setShield int32 = 91004 // S. Def 100
	// mixLegs beats setLegs alone and is pricier, but not by S's bonus.
	mixLegs int32 = 91005 // P. Def 25
	// bigLegs beats setLegs by more than S's bonus.
	bigLegs     int32 = 91006 // P. Def 40
	plainShield int32 = 91007 // S. Def 110
	fullChest   int32 = 91010 // full armor, P. Def 45
	fullHead    int32 = 91011 // P. Def 8
	oldShirt    int32 = 91020 // P. Def 10
	oldPants    int32 = 91021 // P. Def 6
	oldHead     int32 = 91022 // P. Def 12
	// strongShirt beats setChest alone.
	strongShirt int32 = 91023 // P. Def 32
	// Pieces of the index test's dropped sets.
	unsoldHead int32 = 91030
	heavyLegs  int32 = 91031
)

// setTemplates are the set tests' pieces.
var setTemplates = func() map[int32]datapack.Item {
	m := make(map[int32]datapack.Item)
	add := func(id int32, part, kind string, price int32, st datapack.Stats) {
		m[id] = datapack.Item{ID: id, Type: "Armor", BodyPart: part, Kind: kind, Price: price, Stats: st}
	}
	add(setChest, "chest", "LIGHT", 1000, datapack.Stats{PDef: 30})
	add(setLegs, "legs", "LIGHT", 800, datapack.Stats{PDef: 20})
	add(setHead, "head", "", 500, datapack.Stats{PDef: 10})
	add(setShield, "lhand", "", 600, datapack.Stats{SDef: 100})
	add(mixLegs, "legs", "LIGHT", 3000, datapack.Stats{PDef: 25})
	add(bigLegs, "legs", "LIGHT", 5000, datapack.Stats{PDef: 40})
	add(plainShield, "lhand", "", 700, datapack.Stats{SDef: 110})
	add(fullChest, "fullarmor", "LIGHT", 4000, datapack.Stats{PDef: 45})
	add(fullHead, "head", "", 400, datapack.Stats{PDef: 8})
	add(oldShirt, "chest", "LIGHT", 100, datapack.Stats{PDef: 10})
	add(oldPants, "legs", "LIGHT", 100, datapack.Stats{PDef: 6})
	add(oldHead, "head", "", 100, datapack.Stats{PDef: 12})
	add(strongShirt, "chest", "LIGHT", 2000, datapack.Stats{PDef: 32})
	add(heavyLegs, "legs", "HEAVY", 100, datapack.Stats{PDef: 6})
	return m
}()

// setDefs are the set tests' armorSets.xml rows.
var setDefs = map[int32]datapack.ArmorSet{
	setChest:  {Name: "S", Chest: setChest, Legs: setLegs, Head: setHead, Shield: setShield},
	fullChest: {Name: "F", Chest: fullChest, Head: fullHead},
}

// setShop sells S, mixLegs and plainShield.
var setShop = []datapack.Product{
	{ItemID: setChest, Price: 1000},
	{ItemID: setLegs, Price: 800},
	{ItemID: setHead, Price: 500},
	{ItemID: setShield, Price: 600},
	{ItemID: mixLegs, Price: 3000},
	{ItemID: plainShield, Price: 700},
}

// newSetBrain is newGearBrain for a human fighter (light armor and a
// shield, by P. Def) of the set tests' templates and sets, its merchant
// selling products.
func newSetBrain(t *testing.T, products []datapack.Product, items ...Item) *Brain {
	t.Helper()
	b := newGearBrain(t, 0, items...)
	b.persona.Gear.Classes = defaultGearClasses()
	b.world.Self.ClassID = classHumanFight
	m := b.cfg.Travel
	m.shops, m.cheapest = indexShops([]datapack.BuyList{{ID: 1, NPCID: gearMerchant, Products: products}})
	m.gear, m.shopGear = indexGear(setTemplates, m.cheapest)
	m.sets, m.setPieces = indexSets(setDefs, m.gear)
	return b
}

func TestIndexSets(t *testing.T) {
	gear, _ := indexGear(setTemplates, nil)
	defs := maps.Clone(setDefs)
	defs[oldShirt] = datapack.ArmorSet{Chest: oldShirt, Legs: oldPants, Head: unsoldHead}  // a piece the bot never wears
	defs[fullChest+1] = datapack.ArmorSet{Chest: fullChest, Legs: oldPants}                // keyed apart: a full armor chest with legs
	defs[strongShirt] = datapack.ArmorSet{Chest: strongShirt, Legs: setHead}               // a head as legs
	defs[oldHead] = datapack.ArmorSet{Chest: oldHead}                                      // a head as chest
	defs[setChest] = datapack.ArmorSet{Chest: setChest, Legs: setLegs, Shield: unsoldHead} // a shield that is no gear
	sets, pieces := indexSets(defs, gear)
	want := map[int32]armorSet{
		setChest:  {pieces: [slotCount]int32{slotBody: setChest, slotLegs: setLegs}},
		fullChest: {pieces: [slotCount]int32{slotBody: fullChest, slotHead: fullHead}},
	}
	if !maps.Equal(sets, want) {
		t.Errorf("sets = %v, want %v", sets, want)
	}
	if got, want := slices.Sorted(maps.Keys(pieces)), []int32{setChest, setLegs, fullChest, fullHead}; !slices.Equal(got, slices.Sorted(slices.Values(want))) {
		t.Errorf("pieces = %v, want %v", got, want)
	}
}

// TestGearSetBeats checks set scoring where the bot wears gear: a set it
// carries whole is worn over pricier mixed pieces, a piece that breaks a
// worn set must beat its bonus, a piece that completes one gains it, a set
// shield counts only with the set, and a full armor set competes with a
// chest and legs one.
func TestGearSetBeats(t *testing.T) {
	sWorn := []Item{wornAt(1, setChest, partChest), wornAt(2, setLegs, partLegs), wornAt(3, setHead, partHead)}
	sCarried := []Item{carried(11, setChest, partChest), carried(12, setLegs, partLegs), carried(13, setHead, partHead)}
	old := []Item{wornAt(1, oldShirt, partChest), wornAt(2, oldPants, partLegs)}
	tests := []struct {
		name     string
		items    []Item
		id       int32
		want     bool
		noBonus  bool
		products []datapack.Product
	}{
		{name: "carried set: its legs over the old", items: append(slices.Clone(old), append(sCarried, carried(14, mixLegs, partLegs))...), id: setLegs, want: true},
		{name: "carried set: no pricier mixed legs", items: append(slices.Clone(old), append(sCarried, carried(14, mixLegs, partLegs))...), id: mixLegs},
		{name: "no sets: the mixed legs", items: append(slices.Clone(old), append(sCarried, carried(14, mixLegs, partLegs))...), id: mixLegs, want: true, noBonus: true},
		{name: "carried set: its chest though weaker than the worn", items: append([]Item{wornAt(1, strongShirt, partChest), wornAt(2, oldPants, partLegs)}, sCarried...), id: setChest, want: true},
		{name: "breaking under the bonus: kept", items: append(slices.Clone(sWorn), carried(14, mixLegs, partLegs)), id: mixLegs},
		{name: "breaking over the bonus: worn", items: append(slices.Clone(sWorn), carried(14, bigLegs, partLegs)), id: bigLegs, want: true},
		{name: "completing gains the bonus", items: []Item{wornAt(1, setChest, partChest), wornAt(2, setLegs, partLegs), wornAt(3, oldHead, partHead), carried(13, setHead, partHead)}, id: setHead, want: true},
		{name: "completing: without sets the stronger head stays", items: []Item{wornAt(1, setChest, partChest), wornAt(2, setLegs, partLegs), wornAt(3, oldHead, partHead), carried(13, setHead, partHead)}, id: setHead, noBonus: true},
		{name: "set shield kept over a better plain one", items: append(slices.Clone(sWorn), wornAt(4, setShield, partLHand), carried(15, plainShield, partLHand)), id: plainShield},
		{name: "set shield over a plain one with the set", items: append(slices.Clone(sWorn), wornAt(4, plainShield, partLHand), carried(15, setShield, partLHand)), id: setShield, want: true},
		{name: "set shield: no bonus without the set", items: append(slices.Clone(old), wornAt(4, plainShield, partLHand), carried(15, setShield, partLHand)), id: setShield},
		{name: "chest and legs set over a worn full armor set", items: append([]Item{wornAt(1, fullChest, partFullArmor), wornAt(3, fullHead, partHead)}, sCarried...), id: setChest, want: true},
		{name: "full armor set: not over a worn set worth more", items: append(slices.Clone(sWorn), carried(11, fullChest, partFullArmor), carried(13, fullHead, partHead)), id: fullChest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSetBrain(t, nil, tt.items...)
			if tt.noBonus {
				b.persona.Gear.SetBonus = 0
			}
			i := slices.IndexFunc(tt.items, func(it Item) bool { return it.TemplateID == tt.id && !it.Equipped })
			w := b.worn()
			if got := b.upgrade(tt.items[i], &w); got != tt.want {
				t.Errorf("upgrade(%d) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

// TestGearSetWearsWhole checks that the gear errand, carrying a whole set
// and a pricier mixed piece, wears the set piece by piece and never the
// mixed piece, and stops once the set is worn.
func TestGearSetWearsWhole(t *testing.T) {
	b := newSetBrain(t, nil, wornAt(1, oldShirt, partChest), wornAt(2, oldPants, partLegs),
		carried(11, setChest, partChest), carried(12, setLegs, partLegs), carried(13, setHead, partHead), carried(14, mixLegs, partLegs))
	var used []int32
	for range 5 {
		a, ok := b.ruleGear(t0)
		if !ok {
			break
		}
		it := b.world.Inventory.Items[a.object]
		used = append(used, it.TemplateID)
		// The server equips it, taking off what it replaces.
		for id, o := range b.world.Inventory.Items {
			if o.Equipped && slotOf(o.BodySlot) == slotOf(it.BodySlot) {
				o.Equipped = false
				b.world.Inventory.Items[id] = o
			}
		}
		it.Equipped = true
		b.world.Inventory.Items[a.object] = it
		b.gear.obj = 0
	}
	if want := []int32{setChest, setLegs, setHead}; !slices.Equal(used, want) {
		t.Fatalf("worn %v, want %v", used, want)
	}
}

// TestGearSetBuy checks set buying: the gear goal plans the whole set,
// its shield too, over a pricier mixed piece when the budget covers it,
// and otherwise saves for it, buying no part of it nor the mixed piece.
func TestGearSetBuy(t *testing.T) {
	const keep = 10000
	set := []int32{setShield, setChest, setLegs, setHead}
	old := []Item{wornAt(1, oldShirt, partChest), wornAt(2, oldPants, partLegs)}
	t.Run("affordable: the whole set", func(t *testing.T) {
		b := newSetBrain(t, setShop, append(old, adenaStack(keep+2900))...)
		var got []int32
		for _, r := range b.gearWants() {
			got = append(got, r.item)
		}
		if !slices.Equal(got, set) {
			t.Fatalf("gearWants = %v, want the set %v", got, set)
		}
		if goal, _ := b.gearGoal(); !slices.Equal(goal, set) {
			t.Fatalf("gearGoal = %v, want the set %v", goal, set)
		}
		rows := b.buyRows(BuyList{Money: keep + 2900, ListID: 1, Items: shopItems(setShop)})
		want := []buyRow{{setShield, 1}, {setChest, 1}, {setLegs, 1}, {setHead, 1}}
		if !slices.Equal(rows, want) {
			t.Fatalf("buyRows = %v, want %v", rows, want)
		}
	})
	t.Run("no sets: the mixed legs", func(t *testing.T) {
		b := newSetBrain(t, setShop, append(old, adenaStack(keep+5000))...)
		b.persona.Gear.SetBonus = 0
		got := b.gearCandidates()
		if i, j := slices.Index(got, mixLegs), slices.Index(got, setLegs); i < 0 || j >= 0 && j < i {
			t.Fatalf("gearCandidates = %v, want mixLegs before setLegs", got)
		}
	})
	t.Run("short: saves for the set", func(t *testing.T) {
		b := newSetBrain(t, setShop, append(old, adenaStack(keep+2000))...)
		if got := b.gearWants(); len(got) != 0 {
			t.Fatalf("gearWants = %v, want none: saving for the set", got)
		}
		if goal, _ := b.gearGoal(); !slices.Equal(goal, set) {
			t.Fatalf("gearGoal = %v, want the set %v", goal, set)
		}
		if !b.saves(set, 2000, nil) {
			t.Fatal("saves = false, want saving for the set within reach")
		}
		// A window with the money for a piece but not the rest buys none.
		rows := b.buyRows(BuyList{Money: keep + 2000, ListID: 1, Items: shopItems(setShop)})
		if len(rows) != 0 {
			t.Fatalf("buyRows = %v, want none", rows)
		}
	})
	t.Run("out of reach: single pieces in the set's slots", func(t *testing.T) {
		// 1000 is under SaveFrom (50 %) of the set's 2900.
		b := newSetBrain(t, setShop, append(old, adenaStack(keep+1000))...)
		if goal, set := b.gearGoal(); len(set) != 0 || slices.Equal(goal, []int32{setShield, setChest, setLegs, setHead}) {
			t.Fatalf("gearGoal = %v, set %v, want no set out of reach", goal, set)
		}
		var got []int32
		for _, r := range b.gearWants() {
			got = append(got, r.item)
		}
		if !slices.Contains(got, setLegs) {
			t.Fatalf("gearWants = %v, want the affordable setLegs over oldPants", got)
		}
		if rows := b.buyRows(BuyList{Money: keep + 1000, ListID: 1, Items: shopItems(setShop)}); len(rows) == 0 {
			t.Fatal("buyRows = none, want an affordable upgrade")
		}
	})
	t.Run("carried upgrade in a set slot: the set still whole", func(t *testing.T) {
		b := newSetBrain(t, setShop, append(slices.Clone(old), carried(14, mixLegs, partLegs), adenaStack(keep+2900))...)
		if goal, s := b.gearGoal(); !slices.Equal(goal, set) || !slices.Equal(s, set) {
			t.Fatalf("gearGoal = %v, set %v, want the whole set %v", goal, s, set)
		}
		var got []int32
		for _, r := range b.gearWants() {
			got = append(got, r.item)
		}
		if !slices.Equal(got, set) {
			t.Fatalf("gearWants = %v, want the set %v", got, set)
		}
	})
	t.Run("part sold here: the rest still paid", func(t *testing.T) {
		b := newSetBrain(t, setShop, append(old, adenaStack(keep+2900))...)
		window := shopItems(setShop[:1]) // the chest only
		if rows := b.buyRows(BuyList{Money: keep + 2900, ListID: 1, Items: window}); !slices.Equal(rows, []buyRow{{setChest, 1}}) {
			t.Fatalf("buyRows = %v, want the chest", rows)
		}
		if rows := b.buyRows(BuyList{Money: keep + 1500, ListID: 1, Items: window}); len(rows) != 0 {
			t.Fatalf("buyRows = %v, want none: the rest of the set unpaid", rows)
		}
	})
}

// TestGearSetKeepsParts checks that a carried piece of the set the bot buys
// towards is not junk though it beats nothing alone, and is without sets.
func TestGearSetKeepsParts(t *testing.T) {
	part := carried(11, setChest, partChest)
	for _, bonus := range []float64{0.15, 0} {
		b := newSetBrain(t, setShop, wornAt(1, strongShirt, partChest), wornAt(2, oldPants, partLegs), part, adenaStack(100))
		b.persona.Gear.SetBonus = bonus
		if _, junk := b.junk()[part.ObjectID]; junk != (bonus == 0) {
			t.Errorf("SetBonus %v: setChest junk = %v, want %v", bonus, junk, bonus == 0)
		}
	}
}

// TestGearSetStoreKeepsParts checks that the store does not list a carried
// set piece no merchant sells while the rest of its set is bought, and
// lists it without sets.
func TestGearSetStoreKeepsParts(t *testing.T) {
	var shop []datapack.Product // setShop without setHead
	for _, p := range setShop {
		if p.ItemID != setHead {
			shop = append(shop, p)
		}
	}
	part := carried(11, setHead, partHead)
	for _, bonus := range []float64{0.15, 0} {
		b := newSetBrain(t, shop, wornAt(1, strongShirt, partChest), wornAt(2, oldPants, partLegs), wornAt(3, oldHead, partHead), part, adenaStack(100))
		b.persona.Gear.SetBonus = bonus
		if got := b.storeLoot(part); got != (bonus == 0) {
			t.Errorf("SetBonus %v: storeLoot(setHead) = %v, want %v", bonus, got, bonus == 0)
		}
	}
}

// shopItems is the window of a merchant selling products.
func shopItems(products []datapack.Product) []ShopItem {
	out := make([]ShopItem, len(products))
	for i, p := range products {
		out[i] = ShopItem{TemplateID: p.ItemID, Price: p.Price}
	}
	return out
}

// TestIndexSetsShipped checks the set index against a real datapack when
// BOT_DATAPACK_XML names its data/xml directory, and skips otherwise: the
// sets whose pieces are all gear bots wear: every set but the clan and
// noble ones (Clan Oath, Apella), whose pieces have an equip condition.
func TestIndexSetsShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	dp, err := datapack.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := NewTravelMap(dp, DefaultSupplyConfig())
	for chest, s := range dp.ArmorSets {
		if _, ok := m.sets[chest]; !ok && !strings.HasPrefix(s.Name, "Clan Oath") && !strings.HasPrefix(s.Name, "Apella") {
			t.Errorf("set %q not indexed", s.Name)
		}
	}
	if len(m.sets) != 45 {
		t.Errorf("indexed %d of %d sets, want 45", len(m.sets), len(dp.ArmorSets))
	}
}
