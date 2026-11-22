package botclient

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Templates of the gear score tests (aCis_datapack/data/xml/items), and
// made-up ones.
const (
	kiteShield   int32 = 629
	stormbringer int32 = 72
	// weakCSword is a C-grade sword weaker than a +3 Artisan's Sword and
	// stronger than a plain one.
	weakCSword int32 = 90010
	// pricyShirt is a light shirt pricier and weaker than the leather shirt.
	pricyShirt int32 = 90011
	// magicSword is a sword of more M. Atk and less P. Atk than the
	// falchion, and cheaper.
	magicSword int32 = 90012
	// enchantRing is a ring weaker than the magic ring, whose enchant
	// raises its M. Def.
	enchantRing int32 = 90013
)

// scoreTemplates are slotTemplates with the score tests' pieces.
var scoreTemplates = func() map[int32]datapack.Item {
	m := maps.Clone(slotTemplates)
	for _, it := range []datapack.Item{
		{ID: kiteShield, Type: "Armor", BodyPart: "lhand", Grade: 1, Price: 78400, Stats: datapack.Stats{SDef: 142}, Enchants: []string{"sDef"}},
		{ID: stormbringer, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Grade: 2, Price: 2290000, Stats: datapack.Stats{PAtk: 107, MAtk: 61, PAtkSpd: 379}, Enchants: []string{"pAtk", "mAtk"}},
		{ID: weakCSword, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Grade: 2, Price: 2000000, Stats: datapack.Stats{PAtk: 55, MAtk: 40, PAtkSpd: 379}, Enchants: []string{"pAtk", "mAtk"}},
		{ID: pricyShirt, Type: "Armor", BodyPart: "chest", Kind: "LIGHT", Price: 50000, Stats: datapack.Stats{PDef: 40}},
		{ID: magicSword, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Price: 200000, Stats: datapack.Stats{PAtk: 25, MAtk: 30, PAtkSpd: 379}},
		{ID: enchantRing, Type: "Armor", BodyPart: "rfinger;lfinger", Price: 100, Stats: datapack.Stats{MDef: 6}, Enchants: []string{"mDef"}},
	} {
		m[it.ID] = it
	}
	return m
}()

// newScoreBrain is newSlotsBrain with the score templates indexed and
// Expertise at level expertise.
func newScoreBrain(t *testing.T, class, expertise int32, items ...Item) *Brain {
	t.Helper()
	b := newSlotsBrain(t, class, items...)
	b.cfg.Travel.gear, b.cfg.Travel.shopGear = indexGear(scoreTemplates, b.cfg.Travel.cheapest)
	if expertise > 0 {
		b.world.Apply(SkillList{Skills: []KnownSkill{{ID: expertiseSkill, Level: expertise, Passive: true}}}, t0)
	}
	return b
}

// enchanted is template id worn, object obj, in body part mask part, at
// enchant level enchant.
func enchanted(obj, id, part int32, enchant uint16) Item {
	it := wornAt(obj, id, part)
	it.Enchant = enchant
	return it
}

// fighterSwords and casterSwords are class tables wielding swords, the
// one weighing P. Atk and attack speed, the other M. Atk.
var (
	fighterSwords = []GearClass{{Weapons: []string{"SWORD"}, Armor: "LIGHT", Weights: fighterWeights}}
	casterSwords  = []GearClass{{Weapons: []string{"SWORD"}, Armor: "LIGHT", Weights: casterWeights}}
)

// TestGearScoreBeats checks that pieces compare by score, the stats and
// enchant weighed by the class's build, not by price.
func TestGearScoreBeats(t *testing.T) {
	artisan := func(enchant uint16) Item { return enchanted(201, artisansSword, partRHand, enchant) }
	kite := wornAt(204, kiteShield, partLHand)
	tests := []struct {
		name      string
		class     int32
		expertise int32
		classes   []GearClass
		items     []Item
		id        int32
		want      bool
	}{
		{name: "higher stats over a pricier piece", class: classRogue, items: []Item{wornAt(202, pricyShirt, partChest)}, id: leatherShirt22, want: true},
		{name: "pricier, weaker piece under higher stats", class: classRogue, items: []Item{wornAt(202, leatherShirt22, partChest)}, id: pricyShirt},
		{name: "+3 D sword under a better C sword", class: classHumanFight, expertise: 2, items: []Item{artisan(3)}, id: stormbringer, want: true},
		{name: "+3 D sword over a worse C sword", class: classHumanFight, expertise: 2, items: []Item{artisan(3)}, id: weakCSword},
		{name: "plain D sword under the worse C sword", class: classHumanFight, expertise: 2, items: []Item{artisan(0)}, id: weakCSword, want: true},
		{name: "two-hander over sword and shield", class: classWarrior, expertise: 1, items: []Item{artisan(0), kite}, id: twoHanded, want: true},
		{name: "two-hander over +3 sword and shield", class: classWarrior, expertise: 1, items: []Item{artisan(3), kite}, id: twoHanded, want: true},
		{name: "two-hander under +4 sword and shield", class: classWarrior, expertise: 1, items: []Item{artisan(4), kite}, id: twoHanded},
		{name: "sword and carried shield under a two-hander", class: classWarrior, expertise: 1, items: []Item{wornAt(201, twoHanded, partLRHand), carried(220, kiteShield, partLHand)}, id: artisansSword},
		{name: "fighter: P. Atk over M. Atk", classes: fighterSwords, items: []Item{wornAt(201, magicSword, partRHand)}, id: falchion, want: true},
		{name: "fighter: M. Atk under P. Atk", classes: fighterSwords, items: []Item{wornAt(201, falchion, partRHand)}, id: magicSword},
		{name: "caster: P. Atk under M. Atk", classes: casterSwords, items: []Item{wornAt(201, magicSword, partRHand)}, id: falchion},
		{name: "caster: M. Atk over P. Atk", classes: casterSwords, items: []Item{wornAt(201, falchion, partRHand)}, id: magicSword, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newScoreBrain(t, tt.class, tt.expertise, tt.items...)
			if tt.classes != nil {
				b.persona.Gear.Classes = tt.classes
			}
			w := b.worn()
			if got := b.beats(tt.id, &w); got != tt.want {
				t.Fatalf("beats(%d) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

// TestGearScoreEnchant checks that the enchant of a carried piece and of a
// worn pair counts in their scores.
func TestGearScoreEnchant(t *testing.T) {
	plain := wornAt(201, artisansSword, partRHand)
	for _, tt := range []struct {
		name    string
		enchant uint16
		want    bool
	}{
		{name: "carried +1 over the plain one worn", enchant: 1, want: true},
		{name: "carried plain one, the same", enchant: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			it := carried(210, artisansSword, partRHand)
			it.Enchant = tt.enchant
			b := newScoreBrain(t, classHumanFight, 1, plain, it)
			w := b.worn()
			if got := b.upgrade(it, &w); got != tt.want {
				t.Fatalf("upgrade = %v, want %v", got, tt.want)
			}
		})
	}
	for _, tt := range []struct {
		name    string
		enchant uint16
		want    int32
	}{
		{name: "+3 ring over the magic ring", enchant: 3, want: magicRing},
		{name: "plain ring under the magic ring", want: enchantRing},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newScoreBrain(t, classMysticHuman, 0, wornAt(208, magicRing, partLRFinger), enchanted(209, enchantRing, partLRFinger, tt.enchant))
			if got := b.worn()[slotFinger].TemplateID; got != tt.want {
				t.Fatalf("ring replaced = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestBrainGearPicksByBuild has an unarmed fighter wear the sword of more
// P. Atk, and a caster the one of more M. Atk.
func TestBrainGearPicksByBuild(t *testing.T) {
	strong, magic := carried(210, falchion, partRHand), carried(211, magicSword, partRHand)
	for _, tt := range []struct {
		name    string
		classes []GearClass
		want    int32
	}{
		{name: "fighter", classes: fighterSwords, want: strong.ObjectID},
		{name: "caster", classes: casterSwords, want: magic.ObjectID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newScoreBrain(t, classHumanFight, 0, strong, magic)
			b.persona.Gear.Classes = tt.classes
			a, ok := b.ruleGear(at(time.Second))
			if !ok || a.object != tt.want {
				t.Fatalf("ruleGear = %+v, %v, want a UseItem of %d", a, ok, tt.want)
			}
		})
	}
}

// TestGearCandidatesByScore checks that the shop errand buys the piece of
// the higher score first, not the pricier one.
func TestGearCandidatesByScore(t *testing.T) {
	b := newScoreBrain(t, classRogue, 0, wornAt(202, squireShirt, partChest))
	shops, cheapest := indexShops([]datapack.BuyList{{ID: 1, NPCID: gearMerchant, Products: []datapack.Product{
		{ItemID: leatherShirt22, Price: 2430}, {ItemID: pricyShirt, Price: 50000},
	}}})
	m := b.cfg.Travel
	m.shops, m.cheapest = shops, cheapest
	m.gear, m.shopGear = indexGear(scoreTemplates, cheapest)
	if want := []int32{leatherShirt22, pricyShirt}; !slices.Equal(b.gearCandidates(), want) {
		t.Errorf("gearCandidates = %v, want %v", b.gearCandidates(), want)
	}
}
