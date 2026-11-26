package botclient

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Made-up weapons of the second-profession weapon tests, of one grade,
// price and stats each kind but the two-handers, which outscore the rest.
const (
	secondSword    int32 = 93001
	secondBlunt    int32 = 93002
	secondBigSword int32 = 93003
	secondBigBlunt int32 = 93004
)

// secondWeapons are the weapon tests' templates: a two-handed sword and
// blunt of the same stats, and a sword and blunt of the same stats.
var secondWeapons = map[int32]datapack.Item{
	secondSword:    {ID: secondSword, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Price: 20000, Stats: datapack.Stats{PAtk: 12, MAtk: 9, PAtkSpd: 379}},
	secondBlunt:    {ID: secondBlunt, Type: "Weapon", BodyPart: "rhand", Kind: "BLUNT", Price: 20000, Stats: datapack.Stats{PAtk: 12, MAtk: 9, PAtkSpd: 379}},
	secondBigSword: {ID: secondBigSword, Type: "Weapon", BodyPart: "lrhand", Kind: "BIGSWORD", Price: 30000, Stats: datapack.Stats{PAtk: 40, MAtk: 20, PAtkSpd: 325}},
	secondBigBlunt: {ID: secondBigBlunt, Type: "Weapon", BodyPart: "lrhand", Kind: "BIGBLUNT", Price: 30000, Stats: datapack.Stats{PAtk: 40, MAtk: 20, PAtkSpd: 325}},
}

// newSecondWeaponBrain is newGearBrain for a bot of class at 40 under its
// compiled build (no persona gear classes), merchant 30001 selling every
// secondWeapons template.
func newSecondWeaponBrain(t *testing.T, class int32, items ...Item) *Brain {
	t.Helper()
	b := newGearBrain(t, 0, items...)
	b.persona.Gear.Classes = nil
	b.world.Self.ClassID, b.world.Self.Level = class, 40
	var products []datapack.Product
	for _, id := range slices.Sorted(maps.Keys(secondWeapons)) {
		products = append(products, datapack.Product{ItemID: id, Price: secondWeapons[id].Price})
	}
	shops, cheapest := indexShops([]datapack.BuyList{{ID: 1, NPCID: gearMerchant, Products: products}})
	m := b.cfg.Travel
	m.shops, m.cheapest = shops, cheapest
	m.gear, m.shopGear = indexGear(secondWeapons, cheapest)
	return b
}

// TestRowDSWeaponOrder checks that the destroyer (46) takes the two-handed
// blunt over a two-handed sword of the same score (Hammer Crush takes only
// the blunt): first to buy, and first to wear of the two carried. The
// sword has the lower template and object ids, so neither breaks the tie.
func TestRowDSWeaponOrder(t *testing.T) {
	b := newSecondWeaponBrain(t, 46)
	var big []int32
	for _, id := range b.gearCandidates() {
		if secondWeapons[id].BodyPart == "lrhand" {
			big = append(big, id)
		}
	}
	if want := []int32{secondBigBlunt, secondBigSword}; !slices.Equal(big, want) {
		t.Errorf("two-handers to buy = %v, want %v (blunt first)", big, want)
	}

	b = newSecondWeaponBrain(t, 46, carried(301, secondBigSword, partLRHand), carried(302, secondBigBlunt, partLRHand))
	a, ok := b.ruleGear(t0.Add(time.Second))
	if !ok || a.object != 302 {
		t.Errorf("ruleGear = %+v, %v; want the blunt (302) worn", a, ok)
	}
}

// TestRowShieldWeapons checks the second professions that keep a shield:
// the paladin buys swords or blunts, the temple knight, sword singer and
// shillien knight swords, the bounty hunter blunts, and none buys or
// wears a two-handed weapon, unarmed or wielding a one-handed one, though
// the two-handers outscore it.
func TestRowShieldWeapons(t *testing.T) {
	for _, tt := range []struct {
		class int32
		want  []int32
	}{
		{5, []int32{secondSword, secondBlunt}},
		{20, []int32{secondSword}},
		{21, []int32{secondSword}},
		{33, []int32{secondSword}},
		{55, []int32{secondBlunt}},
	} {
		race, _ := datapack.ClassRace(tt.class)
		if gc := CompileBuild(race, tt.class, 40, RoleDamage, BranchChoice{}).Kit.Gear; !gc.Shield || gc.Armor != "HEAVY" {
			t.Errorf("class %d: gear %+v, want a shield in heavy armor", tt.class, gc)
		}
		b := newSecondWeaponBrain(t, tt.class)
		if got := b.gearCandidates(); !slices.Equal(slices.Sorted(slices.Values(got)), tt.want) {
			t.Errorf("class %d unarmed: buys %v, want %v", tt.class, got, tt.want)
		}
		one := tt.want[0]
		b = newSecondWeaponBrain(t, tt.class, wornAt(301, one, partRHand), carried(302, secondBigSword, partLRHand), carried(303, secondBigBlunt, partLRHand))
		if got := b.gearCandidates(); slices.Contains(got, secondBigSword) || slices.Contains(got, secondBigBlunt) {
			t.Errorf("class %d with a one-hander: buys %v, want no two-hander", tt.class, got)
		}
		for _, obj := range []int32{302, 303} {
			if it := b.world.Inventory.Items[obj]; b.wearsNext(it) {
				t.Errorf("class %d with a one-hander: wears two-hander %d", tt.class, it.TemplateID)
			}
		}
	}
}

// TestRowWCWeapons checks that the warcryer (52) wields magical blunts
// alone, in robes with the orc mystic's armor grants, ahead of the mystic
// entry that lists it.
func TestRowWCWeapons(t *testing.T) {
	for _, role := range []Role{RoleDamage, RoleSupport} {
		bd := CompileBuild(3, 52, 40, role, BranchChoice{})
		if gc := bd.Kit.Gear; !slices.Equal(gc.Weapons, []string{"BLUNT/MAGIC"}) || gc.Armor != "MAGIC" || gc.Shield || gc.BuffPotion != magicHastePotion {
			t.Errorf("%v: gear %+v, want magical blunts in robes", role, gc)
		}
		if heavy := slices.ContainsFunc(bd.Race.MysticArmor, func(g ArmorGrant) bool { return g.Kind == "HEAVY" }); heavy != (role == RoleSupport) {
			t.Errorf("%v: heavy armor grant %v, want it in the Support role only", role, heavy)
		}
	}
}
