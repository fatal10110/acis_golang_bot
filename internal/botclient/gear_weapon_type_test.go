package botclient

import (
	"maps"
	"slices"
	"testing"
	"time"
)

// pricyDagger is a dagger worth more than every bow of the weapon type
// tests.
const pricyDagger int32 = 90002

// newDaggerArcher is newArcherBrain with daggers and a shield indexed too:
// a Hawkeye coming from a dagger line.
func newDaggerArcher(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newArcherBrain(t, 0, items...)
	m := maps.Clone(arrowTemplates)
	m[dagger] = slotTemplates[dagger]
	m[leatherShield] = slotTemplates[leatherShield]
	m[pricyDagger] = slotTemplates[dagger]
	d := m[pricyDagger]
	d.ID, d.Price = pricyDagger, 50000
	m[pricyDagger] = d
	b.cfg.Travel.gear, b.cfg.Travel.shopGear = indexGear(m, b.cfg.Travel.cheapest)
	return b
}

// TestGearBeatsUnlistedWeapon checks the replacement of a worn weapon of a
// type the class does not wield: any listed type beats it whatever the
// prices, nothing unlisted does, and a listed one is never replaced by an
// unlisted one.
func TestGearBeatsUnlistedWeapon(t *testing.T) {
	wornDagger := wornAt(220, dagger, partRHand)
	tests := []struct {
		name  string
		class int32
		items []Item
		edit  func(*Brain)
		id    int32
		want  bool
	}{
		{name: "archer: bow over a dagger", class: classHawkeye, items: []Item{wornDagger}, id: plainBow, want: true},
		{name: "archer: cheap bow over a pricy dagger", class: classHawkeye, items: []Item{wornAt(220, pricyDagger, partRHand)}, id: shortBow, want: true},
		{name: "archer: no better dagger", class: classHawkeye, items: []Item{wornDagger}, id: pricyDagger},
		{name: "archer: no dagger over a bow", class: classHawkeye, items: []Item{wornShortBow}, id: pricyDagger},
		{name: "archer: enchanted dagger kept", class: classHawkeye, items: []Item{{ObjectID: 220, TemplateID: dagger, Equipped: true, BodySlot: partRHand, Enchant: 1}}, id: plainBow},
		{name: "archer: bow above expertise", class: classHawkeye, items: []Item{wornDagger}, id: strengthenedBow},
		{name: "archer: bow takes a shield off", class: classHawkeye, items: []Item{wornDagger, wornAt(221, leatherShield, partLHand)}, id: plainBow, want: true},
		{name: "archer: enchanted shield kept", class: classHawkeye, items: []Item{wornDagger, {ObjectID: 221, TemplateID: leatherShield, Equipped: true, BodySlot: partLHand, Enchant: 1}}, id: plainBow},
		{name: "dagger user: dagger over a bow", class: classTreasureHunter, items: []Item{wornShortBow}, id: dagger, want: true},
		{name: "dagger user: better dagger as before", class: classTreasureHunter, items: []Item{wornDagger}, id: pricyDagger, want: true},
		{name: "class with no weapons keeps its own", class: classHawkeye, items: []Item{wornDagger}, edit: func(b *Brain) { b.persona.Gear.Classes = []GearClass{{Armor: "LIGHT"}} }, id: plainBow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newDaggerArcher(t, tt.items...)
			b.world.Self.ClassID = tt.class
			if tt.edit != nil {
				tt.edit(b)
			}
			w := b.worn()
			if got := b.beats(tt.id, &w); got != tt.want {
				t.Errorf("beats(%d) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

// TestBrainDaggerArcherToBow drives an archer wielding a dagger: a visit
// buys a bow and its arrows together, the bow waits for its arrows, the
// dagger is not sold while worn, and once the bow is worn the dagger is
// junk and never worn back.
func TestBrainDaggerArcherToBow(t *testing.T) {
	wornDagger := wornAt(220, dagger, partRHand)
	b := newDaggerArcher(t, wornDagger, adenaStack(30000))
	b.persona.Shop.KeepAdena = 0
	if got, want := b.gearWants(), (buyRow{item: plainBow, count: 1}); !slices.Contains(got, want) {
		t.Fatalf("gearWants = %v, want %v among them", got, want)
	}
	window := BuyList{Money: 30000, Items: []ShopItem{{TemplateID: plainBow, Price: 12500}, {TemplateID: woodenArrow, Price: 2}}}
	want := []buyRow{{item: plainBow, count: 1}, {item: woodenArrow, count: 1000}}
	if got := b.buyRows(window); !slices.Equal(got, want) {
		t.Fatalf("buyRows = %v, want %v", got, want)
	}

	// The bow came without arrows (looted, or the arrows sold out).
	bow := carried(303, plainBow, partLRHand)
	b.world.Inventory.Items[bow.ObjectID] = bow
	if a, ok := b.ruleGear(at(time.Second)); ok {
		t.Fatalf("bow, no arrows: ruleGear = %+v, want the dagger kept", a)
	}
	if got, want := b.shopWants(true), (buyRow{item: woodenArrow, count: 1000}); !slices.Contains(got, want) {
		t.Errorf("bow, no arrows: shopWants = %v, want %v among them", got, want)
	}
	if got := slices.Sorted(maps.Keys(b.junk())); len(got) != 0 {
		t.Errorf("bow carried: junk = %v, want none", got)
	}

	b.world.Inventory.Items[carriedArrows.ObjectID] = carriedArrows
	if got := slices.Sorted(maps.Keys(b.junk())); len(got) != 0 {
		t.Errorf("bow and arrows carried: junk = %v, want none", got)
	}
	a, ok := b.ruleGear(at(2 * time.Second))
	if !ok || a.object != bow.ObjectID {
		t.Fatalf("bow and arrows: ruleGear = %+v %v, want the bow used", a, ok)
	}
	b.gear.sent(a, at(2*time.Second))
	// The server equips the bow and takes the dagger off
	// (itemcontainer/inventory.go equipItemLocked).
	bow.Equipped = true
	b.world.Inventory.Items[bow.ObjectID] = bow
	off := wornDagger
	off.Equipped = false
	b.world.Inventory.Items[off.ObjectID] = off
	if a, ok := b.ruleGear(at(3 * time.Second)); ok {
		t.Fatalf("bow worn: ruleGear = %+v, want no UseItem", a)
	}
	if b.wearsNext(off) {
		t.Error("bow worn: wearsNext(dagger) = true")
	}
	if got, want := slices.Sorted(maps.Keys(b.junk())), []int32{off.ObjectID}; !slices.Equal(got, want) {
		t.Errorf("bow worn: junk = %v, want %v", got, want)
	}
}

// TestArrowsOfNextBow checks the arrows of the bow the gear errand wears
// next: wanted besides the wielded bow's, and never junk.
func TestArrowsOfNextBow(t *testing.T) {
	strongBow := carried(304, strengthenedBow, partLRHand)
	b := newDaggerArcher(t, wornShortBow, carriedArrows, strongBow, boneArrowsOf(50))
	b.world.Apply(SkillList{Skills: []KnownSkill{{ID: expertiseSkill, Level: 1, Passive: true}}}, t0)
	b.persona.Gear.Buy = false
	want := []buyRow{{item: boneArrow, count: 950}}
	if got := b.shopWants(true); !slices.Equal(got, want) {
		t.Errorf("shopWants = %v, want %v", got, want)
	}
	if got := b.junk(); len(got) != 0 {
		t.Errorf("junk = %v, want none", got)
	}
}
