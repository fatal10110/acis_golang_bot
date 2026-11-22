package botclient

import (
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Gear templates of the gear tests (aCis_datapack/data/xml/items).
const (
	shortSword     int32 = 1
	longSword      int32 = 2
	falchion       int32 = 68
	twoHanded      int32 = 124
	artisansSword  int32 = 126
	squireShirt    int32 = 1146
	squirePants    int32 = 1147
	leatherShirt22 int32 = 22
	bronzePlate    int32 = 26
	woodenGaiters  int32 = 2386
	salamanderMail int32 = 396
	leatherHelmet  int32 = 44
	leatherBoots   int32 = 40
	club           int32 = 4
	apprenticeWand int32 = 6
	wandOfAdept    int32 = 747
	// gearMerchant sells weapons and armor.
	gearMerchant int32 = 30001
)

// gearTemplates are the templates of the gear tests, and some the bot
// never wears.
var gearTemplates = map[int32]datapack.Item{
	shortSword:     {ID: shortSword, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Price: 768},
	longSword:      {ID: longSword, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Price: 136000},
	falchion:       {ID: falchion, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Price: 244000},
	twoHanded:      {ID: twoHanded, Type: "Weapon", BodyPart: "lrhand", Kind: "BIGSWORD", Grade: 1, Price: 967000},
	artisansSword:  {ID: artisansSword, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Grade: 1, Price: 644000},
	squireShirt:    {ID: squireShirt, Type: "Armor", BodyPart: "chest", Kind: "LIGHT", Price: 26},
	squirePants:    {ID: squirePants, Type: "Armor", BodyPart: "legs", Kind: "LIGHT", Price: 6},
	leatherShirt22: {ID: leatherShirt22, Type: "Armor", BodyPart: "chest", Kind: "LIGHT", Price: 2430},
	bronzePlate:    {ID: bronzePlate, Type: "Armor", BodyPart: "chest", Kind: "HEAVY", Price: 49200},
	woodenGaiters:  {ID: woodenGaiters, Type: "Armor", BodyPart: "legs", Kind: "LIGHT", Price: 4970},
	salamanderMail: {ID: salamanderMail, Type: "Armor", BodyPart: "fullarmor", Kind: "LIGHT", Grade: 1, Price: 429000},
	leatherHelmet:  {ID: leatherHelmet, Type: "Armor", BodyPart: "head", Price: 10200},
	leatherBoots:   {ID: leatherBoots, Type: "Armor", BodyPart: "feet", Grade: 1, Price: 20900},
	club:           {ID: club, Type: "Weapon", BodyPart: "rhand", Kind: "BLUNT", Price: 768},
	apprenticeWand: {ID: apprenticeWand, Type: "Weapon", BodyPart: "rhand", Kind: "BLUNT", Magical: true, Price: 138},
	wandOfAdept:    {ID: wandOfAdept, Type: "Weapon", BodyPart: "rhand", Kind: "BLUNT", Magical: true, Price: 25500},
	// A necklace and a shield.
	906: {ID: 906, Type: "Armor", BodyPart: "neck", Price: 1},
	18:  {ID: 18, Type: "Armor", BodyPart: "lhand", Price: 1},
	// Never worn: a hero weapon, a shadow weapon, a fishing rod, a pet's
	// armor and a priceless weapon.
	6611: {ID: 6611, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Grade: 5, Price: 1, Conditional: true},
	6702: {ID: 6702, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Price: 1, Duration: 120},
	6529: {ID: 6529, Type: "Weapon", BodyPart: "lrhand", Kind: "FISHINGROD", Price: 1},
	7001: {ID: 7001, Type: "Armor", BodyPart: "chest", Kind: "PET", Price: 1},
	7:    {ID: 7, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD"},
}

// worn and carried gear of the gear tests: a level 1 fighter's.
var (
	wornSword  = Item{ObjectID: 201, TemplateID: shortSword, Count: 1, Equipped: true, BodySlot: partRHand}
	wornShirt  = Item{ObjectID: 202, TemplateID: squireShirt, Count: 1, Equipped: true, BodySlot: partChest}
	wornPants  = Item{ObjectID: 203, TemplateID: squirePants, Count: 1, Equipped: true, BodySlot: partLegs}
	newSword   = Item{ObjectID: 210, TemplateID: longSword, Count: 1, BodySlot: partRHand}
	newFalchon = Item{ObjectID: 211, TemplateID: falchion, Count: 1, BodySlot: partRHand}
)

// gearShop is merchant 30001's buylist 1.
var gearShop = []datapack.Product{
	{ItemID: longSword, Price: 136000},
	{ItemID: falchion, Price: 244000},
	{ItemID: artisansSword, Price: 644000},
	{ItemID: leatherShirt22, Price: 2430},
	{ItemID: woodenGaiters, Price: 4970},
	{ItemID: leatherHelmet, Price: 10200},
}

// newGearBrain is newShopBrain with no restocks or supplies, Expertise at level
// expertise, and a travel map of merchant 30001 (list 1, gearShop) spawned
// at (5000, 0) and the gear templates. Its mystic wears the fighter's gear
// of the gear tests: its class entry lists no weapon type, so a worn
// weapon is matched by its type only (the class rules have their own
// tests, over defaultGearClasses).
func newGearBrain(t *testing.T, expertise int32, items ...Item) *Brain {
	t.Helper()
	b := newShopBrain(t, items...)
	b.persona.Shop.Restock = nil
	b.persona.Shop.Soulshots, b.persona.Shop.Spiritshots = ShotStock{}, ShotStock{}
	b.persona.Supply = SupplyPolicy{}
	b.persona.Gear.Classes = append([]GearClass{{Classes: []int32{classHumanMystic}, Armor: "MAGIC"}}, defaultGearClasses()...)
	if expertise > 0 {
		b.world.Apply(SkillList{Skills: []KnownSkill{{ID: expertiseSkill, Level: expertise, Passive: true}}}, t0)
	}
	shops, cheapest := indexShops([]datapack.BuyList{{ID: 1, NPCID: gearMerchant, Products: gearShop}})
	gear, shopGear := indexGear(gearTemplates, cheapest)
	b.cfg.Travel = &TravelMap{spawns: map[int32][]Point{gearMerchant: {{X: 5000}}}, shops: shops, cheapest: cheapest, gear: gear, shopGear: shopGear}
	return b
}

func TestIndexGear(t *testing.T) {
	gear, sold := indexGear(gearTemplates, map[int32]int32{longSword: 1, falchion: 1, leatherHelmet: 1, squireShirt: 1, woodenGaiters: 1, 6611: 1, lesserHealingPotion: 1})
	want := []int32{shortSword, longSword, falchion, twoHanded, artisansSword, squireShirt, squirePants, leatherShirt22, bronzePlate, woodenGaiters, salamanderMail, leatherHelmet, leatherBoots, club, apprenticeWand, wandOfAdept, 906, 18}
	if got := slices.Sorted(maps.Keys(gear)); !reflect.DeepEqual(got, slices.Sorted(slices.Values(want))) {
		t.Errorf("gear = %v, want %v", got, want)
	}
	if g := gear[twoHanded]; g != (gearItem{part: partLRHand, kind: "BIGSWORD", grade: 1, price: 967000}) {
		t.Errorf("two-handed sword = %+v", g)
	}
	if g := gear[apprenticeWand]; g.kind != "BLUNT/MAGIC" {
		t.Errorf("wand kind = %q, want a magical blunt", g.kind)
	}
	if g := gear[leatherHelmet]; g.kind != "" || g.part != partHead {
		t.Errorf("helmet = %+v, want a head piece of any kind", g)
	}
	// By slot, then price, best first.
	if wantSold := []int32{falchion, longSword, squireShirt, woodenGaiters, leatherHelmet}; !reflect.DeepEqual(sold, wantSold) {
		t.Errorf("shopGear = %v, want %v", sold, wantSold)
	}
}

// TestGearBeats checks which pieces beat what the bot wears.
func TestGearBeats(t *testing.T) {
	std := []Item{wornSword, wornShirt, wornPants}
	mail := Item{ObjectID: 204, TemplateID: salamanderMail, Count: 1, Equipped: true, BodySlot: partFullArmor}
	wand := Item{ObjectID: 205, TemplateID: apprenticeWand, Count: 1, Equipped: true, BodySlot: partRHand}
	tests := []struct {
		name      string
		worn      []Item
		expertise int32
		maxGrade  int32
		refused   int32
		id        int32
		want      bool
	}{
		{name: "better sword", worn: std, id: longSword, want: true},
		{name: "cheaper sword", worn: []Item{{ObjectID: 201, TemplateID: falchion, Equipped: true, BodySlot: partRHand}}, id: longSword},
		{name: "two-handed over one-handed", worn: std, expertise: 1, id: twoHanded},
		{name: "D grade without expertise", worn: std, id: artisansSword},
		{name: "D grade with expertise", worn: std, expertise: 1, id: artisansSword, want: true},
		{name: "MaxGrade caps expertise", worn: std, expertise: 1, maxGrade: -1, id: artisansSword},
		{name: "enchanted sword kept", worn: []Item{{ObjectID: 201, TemplateID: shortSword, Equipped: true, BodySlot: partRHand, Enchant: 3}}, id: longSword},
		{name: "unknown worn sword kept", worn: []Item{{ObjectID: 201, TemplateID: 2369, Equipped: true, BodySlot: partRHand}}, id: longSword},
		{name: "unarmed", worn: []Item{wornShirt}, id: longSword},
		{name: "physical blunt over a wand", worn: []Item{wand}, id: club},
		{name: "better wand", worn: []Item{wand}, id: wandOfAdept, want: true},
		{name: "wand over a sword", worn: std, id: wandOfAdept},
		{name: "heavy over light", worn: std, id: bronzePlate},
		{name: "better shirt", worn: std, id: leatherShirt22, want: true},
		{name: "full armor over shirt and pants", worn: std, expertise: 1, id: salamanderMail, want: true},
		{name: "full armor over enchanted pants", worn: []Item{wornSword, wornShirt, {ObjectID: 203, TemplateID: squirePants, Equipped: true, BodySlot: partLegs, Enchant: 1}}, expertise: 1, id: salamanderMail},
		{name: "legs under full armor", worn: []Item{wornSword, mail}, expertise: 1, id: woodenGaiters},
		{name: "bare body takes the legs' type", worn: []Item{wornPants}, id: leatherShirt22, want: true},
		{name: "bare body, other type", worn: []Item{wornPants}, id: bronzePlate},
		{name: "bare legs take the body's type", worn: []Item{wornShirt}, id: woodenGaiters, want: true},
		{name: "bare body and legs", worn: []Item{wornSword}, id: leatherShirt22},
		{name: "bare head", worn: std, id: leatherHelmet, want: true},
		{name: "D grade boots without expertise", worn: std, id: leatherBoots},
		{name: "refused template", worn: std, refused: longSword, id: longSword},
		{name: "never worn template", worn: std, id: 6611},
		{name: "unknown template", worn: std, id: 9999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newGearBrain(t, tt.expertise, tt.worn...)
			if tt.maxGrade < 0 {
				b.persona.Gear.MaxGrade = 0
			}
			if tt.refused != 0 {
				b.gear.refused = map[int32]bool{tt.refused: true}
			}
			w := b.worn()
			if got := b.beats(tt.id, &w); got != tt.want {
				t.Fatalf("beats(%d) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

// TestBrainGearEquip has a bot wear the better sword it carries, wait for
// the server's InventoryUpdate, and count the sword it took off as junk.
func TestBrainGearEquip(t *testing.T) {
	b := newGearBrain(t, 0, wornSword, wornShirt, wornPants, newSword)
	if _, junk := b.junk()[newSword.ObjectID]; junk {
		t.Fatal("the sword to wear is junk")
	}
	b.world.Self.InCombat = true
	if a, ok := b.ruleGear(at(time.Second)); ok {
		t.Fatalf("in combat: ruleGear = %+v, want no match", a)
	}
	b.world.Self.InCombat = false
	runTrainerSteps(t, b, []trainerStep{
		{
			name: "wear the long sword", at: 2 * time.Second,
			wantRule: "gear", wantKind: actUseItem, wantPayload: encodeUseItem(newSword.ObjectID),
		},
		{name: "wait for the answer", at: 3 * time.Second},
		{
			name: "worn: nothing left to wear", at: 4 * time.Second,
			events: []Event{InventoryUpdate{Changes: []ItemChange{
				{ItemModified, Item{ObjectID: 210, TemplateID: longSword, Count: 1, Equipped: true, BodySlot: partRHand}},
				{ItemModified, Item{ObjectID: 201, TemplateID: shortSword, Count: 1, BodySlot: partRHand}},
			}}},
		},
	})
	junk := b.junk()
	if _, ok := junk[wornSword.ObjectID]; !ok || len(junk) != 1 {
		t.Errorf("junk = %v, want the short sword", junk)
	}
	if b.gear.obj != 0 {
		t.Errorf("gear wait = %d, want none", b.gear.obj)
	}
}

// TestBrainGearNotMoving holds the UseItem while the bot moves: a weapon
// toggled during a cast's approach is toggled again on arrival.
func TestBrainGearNotMoving(t *testing.T) {
	b := newGearBrain(t, 0, wornSword, newSword)
	b.world.Apply(MoveToLocation{ObjectID: selfID, Origin: Point{}, Dest: Point{X: 3000}}, at(time.Second))
	if !b.world.Self.Moving(at(time.Second)) {
		t.Fatal("the bot is not moving")
	}
	if a, ok := b.ruleGear(at(time.Second)); ok {
		t.Fatalf("moving: ruleGear = %+v, want no match", a)
	}
	if b.world.Self.Moving(at(time.Hour)) {
		t.Fatal("the bot never arrives")
	}
	if a, ok := b.ruleGear(at(time.Hour)); !ok || a.object != newSword.ObjectID {
		t.Fatalf("arrived: ruleGear = %+v, %v, want the long sword", a, ok)
	}
}

// TestGearStoreLoot keeps a looted upgrade no merchant sells out of the
// private store, and stores the pieces it replaced once they are off.
func TestGearStoreLoot(t *testing.T) {
	mail := Item{ObjectID: 220, TemplateID: salamanderMail, Count: 1, BodySlot: partFullArmor}
	b := newGearBrain(t, 1, wornSword, wornShirt, wornPants, mail)
	if b.storeLoot(mail) {
		t.Error("the mail to wear is store loot")
	}
	if b.storeLoot(wornShirt) || b.storeLoot(wornPants) {
		t.Error("worn pieces are store loot")
	}
	b.world.Apply(InventoryUpdate{Changes: []ItemChange{
		{ItemModified, Item{ObjectID: 220, TemplateID: salamanderMail, Count: 1, Equipped: true, BodySlot: partFullArmor}},
		{ItemModified, Item{ObjectID: 202, TemplateID: squireShirt, Count: 1, BodySlot: partChest}},
		{ItemModified, Item{ObjectID: 203, TemplateID: squirePants, Count: 1, BodySlot: partLegs}},
	}}, at(time.Second))
	inv := b.world.Inventory.Items
	if !b.storeLoot(inv[202]) || !b.storeLoot(inv[203]) {
		t.Error("the replaced shirt and pants are not store loot")
	}
	if b.storeLoot(inv[220]) {
		t.Error("the worn mail is store loot")
	}
}

// TestBrainGearRefused gives up on a piece the server does not equip
// within gearTimeout: it is never used again and is junk.
func TestBrainGearRefused(t *testing.T) {
	b := newGearBrain(t, 0, wornSword, newSword)
	runTrainerSteps(t, b, []trainerStep{
		{name: "wear", at: time.Second, wantRule: "gear", wantKind: actUseItem, wantPayload: encodeUseItem(newSword.ObjectID)},
		{name: "waiting", at: time.Second + gearTimeout - time.Millisecond},
		{name: "refused", at: time.Second + gearTimeout},
		{name: "not again", at: time.Minute},
	})
	if !b.gear.refused[longSword] {
		t.Errorf("refused = %v, want the long sword", b.gear.refused)
	}
	if _, ok := b.junk()[newSword.ObjectID]; !ok {
		t.Error("the refused sword is not junk")
	}
}

// TestBrainGearBestFirst wears the best of two better swords, then counts
// the other as junk.
func TestBrainGearBestFirst(t *testing.T) {
	b := newGearBrain(t, 0, wornSword, newSword, newFalchon)
	runTrainerSteps(t, b, []trainerStep{
		{name: "wear the falchion", at: time.Second, wantRule: "gear", wantKind: actUseItem, wantPayload: encodeUseItem(newFalchon.ObjectID)},
		{
			name: "worn", at: 2 * time.Second,
			events: []Event{InventoryUpdate{Changes: []ItemChange{
				{ItemModified, Item{ObjectID: 211, TemplateID: falchion, Count: 1, Equipped: true, BodySlot: partRHand}},
				{ItemModified, Item{ObjectID: 201, TemplateID: shortSword, Count: 1, BodySlot: partRHand}},
			}}},
		},
	})
	if junk := b.junk(); len(junk) != 2 {
		t.Errorf("junk = %v, want both swords", junk)
	}
}

// TestGearWants checks the gear a bot buys, and what a merchant's window
// sells it.
func TestGearWants(t *testing.T) {
	std := []Item{wornSword, wornShirt, wornPants}
	window := func(items ...int32) []ShopItem {
		var rows []ShopItem
		for _, id := range items {
			i := slices.IndexFunc(gearShop, func(p datapack.Product) bool { return p.ItemID == id })
			rows = append(rows, ShopItem{ObjectID: id, TemplateID: id, Price: gearShop[i].Price})
		}
		return rows
	}
	tests := []struct {
		name    string
		carried []Item
		adena   int32
		buy     bool
		want    []buyRow
		window  []ShopItem
		bought  []buyRow
	}{
		{
			// 290000 past the 10000 kept: both swords, best first, then a
			// piece per slot; no D grade without expertise.
			name: "rich", carried: std, adena: 300000, buy: true,
			want:   []buyRow{{falchion, 1}, {longSword, 1}, {leatherShirt22, 1}, {woodenGaiters, 1}, {leatherHelmet, 1}},
			window: window(longSword, falchion, leatherShirt22, woodenGaiters, leatherHelmet),
			bought: []buyRow{{falchion, 1}, {leatherShirt22, 1}, {woodenGaiters, 1}, {leatherHelmet, 1}},
		},
		{
			// 149000 past the 1000 the shop keeps: the long sword leaves
			// 13000, the shirt 10570, then nothing past the 9000 more gear
			// keeps.
			name: "the window spends what is left", carried: std, adena: 150000, buy: true,
			want:   []buyRow{{longSword, 1}, {leatherShirt22, 1}, {woodenGaiters, 1}, {leatherHelmet, 1}},
			window: window(longSword, leatherShirt22, woodenGaiters, leatherHelmet),
			bought: []buyRow{{longSword, 1}, {leatherShirt22, 1}},
		},
		{
			name: "a better sword carried", carried: append(slices.Clone(std), newSword), adena: 300000, buy: true,
			want: []buyRow{{leatherShirt22, 1}, {woodenGaiters, 1}, {leatherHelmet, 1}},
		},
		{name: "nothing past the gear's reserve", carried: std, adena: 10000, buy: true},
		{name: "buying off", carried: std, adena: 300000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newGearBrain(t, 0, append(tt.carried, adenaStack(tt.adena))...)
			b.persona.Gear.Buy = tt.buy
			if got := b.shopWants(false); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("shopWants = %v, want %v", got, tt.want)
			}
			if tt.window == nil {
				return
			}
			if got := b.buyRows(BuyList{Money: tt.adena, ListID: 1, Items: tt.window}); !reflect.DeepEqual(got, tt.bought) {
				t.Fatalf("buyRows = %v, want %v", got, tt.bought)
			}
		})
	}
}

// TestBrainGearShopVisit walks a bot with adena to spare to the gear
// merchant's spawn.
func TestBrainGearShopVisit(t *testing.T) {
	b := newGearBrain(t, 0, wornSword, wornShirt, wornPants, adenaStack(300000))
	runTrainerSteps(t, b, []trainerStep{
		{name: "leg to the gear merchant's spawn", at: time.Second, wantRule: "shop", wantKind: actMove},
	})
	if b.shop.v.npcID != gearMerchant || !slices.Equal(b.shop.lists, []int32{1}) {
		t.Fatalf("visit = %+v, want merchant %d list 1", b.shop.v, gearMerchant)
	}
}
