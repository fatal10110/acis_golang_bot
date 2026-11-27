package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Skills and reagents of the reagent tests (aCis_datapack/data/xml skills
// and items ids).
const (
	skillStormCubic     int32 = 10
	skillGreaterMight   int32 = 1388
	skillMajorHeal      int32 = 1401
	skillMajorGroupHeal int32 = 1402
	energyStone         int32 = 5589
	cursedBone          int32 = 2508
)

// reagentSkills are the reagent tests' skill levels: Sonic Buster 3
// Energy Stones, Greater Might 1 to 3 Spirit Ore by level, Major Heal 1
// and Major Group Heal 4 Spirit Ore, Death Spike 1 Cursed Bone, the Storm
// Cubic 5 Crystal D, Haste none, and a level with an id but count 0 (no
// cost, cast/controller.go CanCast).
var reagentSkills = map[datapack.SkillRef]datapack.Skill{
	{ID: skillSonicBuster, Level: 1}:    {ItemConsumeID: energyStone, ItemConsumeCount: 3},
	{ID: skillGreaterMight, Level: 1}:   {ItemConsumeID: spiritOre, ItemConsumeCount: 1},
	{ID: skillGreaterMight, Level: 2}:   {ItemConsumeID: spiritOre, ItemConsumeCount: 2},
	{ID: skillGreaterMight, Level: 3}:   {ItemConsumeID: spiritOre, ItemConsumeCount: 3},
	{ID: skillMajorHeal, Level: 1}:      {ItemConsumeID: spiritOre, ItemConsumeCount: 1},
	{ID: skillMajorGroupHeal, Level: 1}: {ItemConsumeID: spiritOre, ItemConsumeCount: 4},
	{ID: skillDeathSpike, Level: 1}:     {ItemConsumeID: cursedBone, ItemConsumeCount: 1},
	{ID: skillStormCubic, Level: 1}:     {ItemConsumeID: crystalD, ItemConsumeCount: 5},
	{ID: skillHaste, Level: 1}:          {},
	{ID: 2234, Level: 1}:                {ItemConsumeID: 1785},
}

// reagentTravel is a travel map of the reagent skills and grocer 30004,
// spawned at (5000, 0), selling Energy Stones, Spirit Ore and Cursed Bones;
// only the Sales Cat, with no spawn, sells Crystal D.
func reagentTravel() *TravelMap {
	shops, cheapest := indexShops([]datapack.BuyList{
		{ID: 14, NPCID: 30004, Products: []datapack.Product{{ItemID: energyStone, Price: 115}, {ItemID: spiritOre, Price: 400}, {ItemID: cursedBone, Price: 360}}},
		{ID: 5720, NPCID: 31075, Products: []datapack.Product{{ItemID: crystalD, Price: 100}}},
	})
	spawns := map[int32][]Point{30004: {{X: 5000}}}
	return &TravelMap{spawns: spawns, shops: shops, cheapest: cheapest, spawnedSold: spawnedSold(shops, spawns), reagents: indexReagents(reagentSkills)}
}

// carrying makes items the bot's whole inventory.
func carrying(b *Brain, items ...Item) {
	b.world.Apply(ItemList{Items: items}, t0)
}

// reagentStack is a stack of n of reagent template id.
func reagentStack(id, n int32) Item { return Item{ObjectID: 700 + id, TemplateID: id, Count: n} }

// TestTravelReagentOf checks the reagent of a skill level: by level, an
// enchanted level's that of the top plain level, none for a level with
// count 0, a skill without one or no travel map.
func TestTravelReagentOf(t *testing.T) {
	m := reagentTravel()
	for _, tt := range []struct {
		skill, level int32
		want         reagent
	}{
		{skillGreaterMight, 1, reagent{spiritOre, 1}},
		{skillGreaterMight, 3, reagent{spiritOre, 3}},
		{skillGreaterMight, 4, reagent{}},
		{skillGreaterMight, 115, reagent{spiritOre, 3}},
		{skillHaste, 1, reagent{}},
		{2234, 1, reagent{}},
		{skillWindStrike, 1, reagent{}},
	} {
		if got := m.reagentOf(tt.skill, tt.level); got != tt.want {
			t.Errorf("reagentOf(%d, %d) = %v, want %v", tt.skill, tt.level, got, tt.want)
		}
	}
	if _, ok := m.reagents[skillHaste]; ok {
		t.Error("Haste, with no reagent at any level, is indexed")
	}
	var none *TravelMap
	if got := none.reagentOf(skillSonicBuster, 1); got != (reagent{}) {
		t.Errorf("nil map reagentOf = %v, want none", got)
	}
}

// TestReagentGateRotation checks a Gladiator with 2 force charges casts
// Sonic Buster only with its 3 Energy Stones carried, else the next step,
// and that a cast it never sent leaves the charges alone on a refusal.
func TestReagentGateRotation(t *testing.T) {
	now := at(time.Second)
	rot := []Step{{Skill: skillSonicBuster, Charges: 2}, {Skill: skillPowerStrike}}
	for _, tt := range []struct {
		name   string
		stones int32
		want   int32
	}{
		{"no Energy Stone", 0, skillPowerStrike},
		{"2 Energy Stones", 2, skillPowerStrike},
		{"3 Energy Stones", 3, skillSonicBuster},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := kitBrain(t, ClassKit{Rotation: rot}, 80, skillSonicBuster, skillPowerStrike)
			b.cfg.Travel = reagentTravel()
			b.world.Apply(EtcStatusUpdate{Charges: 2}, t0)
			if tt.stones > 0 {
				carrying(b, reagentStack(energyStone, tt.stones))
			}
			a, ok := b.rotationCast(npcID, rot, false, now)
			if !ok {
				t.Fatal("rotationCast: no cast")
			}
			wantCast(t, a, tt.want, npcID)
			if sent, err := b.send(a, now); !sent || err != nil {
				t.Fatalf("send = %v, %v", sent, err)
			}
			b.castRefused(msgCannotBeUsed, now)
			if want := int32(2); tt.want != skillSonicBuster && b.world.Self.Charges != want {
				t.Errorf("charges %d after a refused %d, want %d", b.world.Self.Charges, tt.want, want)
			}
		})
	}
}

// TestReagentGateIdleStep checks an Idle step whose reagent the bot lacks is
// not cast between fights.
func TestReagentGateIdleStep(t *testing.T) {
	now := at(time.Second)
	rot := []Step{{Skill: skillGreaterMight, Idle: true, Target: TargetSelf}}
	for _, ore := range []int32{0, 1} {
		b := kitBrain(t, ClassKit{Rotation: rot}, 80, skillGreaterMight)
		b.cfg.Travel = reagentTravel()
		b.world.Self.InCombat = false
		if ore > 0 {
			carrying(b, reagentStack(spiritOre, ore))
		}
		if _, ok := b.idleStep(rot, now); ok != (ore > 0) {
			t.Errorf("with %d Spirit Ore idleStep casts %v, want %v", ore, ok, ore > 0)
		}
	}
}

// TestReagentGateBuffs checks a Prophet with no Spirit Ore casts no Greater
// Might but still casts Haste, and Greater Might first with its ore.
func TestReagentGateBuffs(t *testing.T) {
	now := at(time.Second)
	for _, tt := range []struct {
		name string
		ore  int32
		want int32
	}{
		{"no Spirit Ore", 0, skillHaste},
		{"Spirit Ore", 5, skillGreaterMight},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := DefaultPersona()
			p.IdleChance = 0
			b, _ := newTestBrain(t, p)
			withKit(b, ClassKit{Buffs: []Buff{{Skill: skillGreaterMight}, {Skill: skillHaste}}})
			b.world.Target = selfID
			b.cfg.Travel = reagentTravel()
			b.world.Apply(SkillList{Skills: []KnownSkill{{ID: skillGreaterMight, Level: 1}, {ID: skillHaste, Level: 1}}}, t0)
			if tt.ore > 0 {
				carrying(b, reagentStack(spiritOre, tt.ore))
			}
			a, ok := b.ruleRebuff(now)
			if !ok {
				t.Fatal("ruleRebuff: no buff")
			}
			wantCast(t, a, tt.want, selfID)
		})
	}
}

// TestReagentGateHeals checks a Bishop with too little Spirit Ore for
// Major Heal heals with Heal instead.
func TestReagentGateHeals(t *testing.T) {
	for _, tt := range []struct {
		name string
		ore  int32
		want int32
	}{
		{"no Spirit Ore", 0, skillHeal},
		{"Spirit Ore", 1, skillMajorHeal},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := DefaultPersona()
			p.IdleChance = 0
			b, _ := newTestBrain(t, p)
			withKit(b, ClassKit{Heals: []Heal{{Skill: skillMajorHeal, HP: 60}, {Skill: skillHeal, HP: 60}}})
			b.cfg.Travel = reagentTravel()
			b.world.Self.HP = 50
			b.world.Apply(SkillList{Skills: []KnownSkill{{ID: skillMajorHeal, Level: 1}, {ID: skillHeal, Level: 1}}}, t0)
			if tt.ore > 0 {
				carrying(b, reagentStack(spiritOre, tt.ore))
			}
			if got, _ := b.healSkill(selfID, at(time.Second)); got.Skill != tt.want {
				t.Errorf("healSkill = %d, want %d", got.Skill, tt.want)
			}
		})
	}
}

// TestReagentWants checks the reagents the shop errand buys: per item, the
// ReagentCastsMin and ReagentCastsMax casts of every kit skill consuming
// it summed at its known level, up to the max once under the min, only
// what a spawned merchant sells, and none at a zero ReagentCastsMax.
func TestReagentWants(t *testing.T) {
	bishop := ClassKit{Heals: []Heal{{Skill: skillMajorHeal, HP: 60}, {Skill: skillMajorGroupHeal, HP: 60, Members: 2}}}
	for _, tt := range []struct {
		name   string
		kit    ClassKit
		skills []KnownSkill
		items  []Item
		edit   func(*Brain)
		want   []buyRow
	}{
		{
			name: "Death Spike, no Cursed Bone", kit: ClassKit{Rotation: []Step{{Skill: skillDeathSpike}}},
			skills: []KnownSkill{{ID: skillDeathSpike, Level: 1}}, want: []buyRow{{item: cursedBone, count: 100}},
		},
		{
			name: "Death Spike, Cursed Bones at the min", kit: ClassKit{Rotation: []Step{{Skill: skillDeathSpike}}},
			skills: []KnownSkill{{ID: skillDeathSpike, Level: 1}}, items: []Item{reagentStack(cursedBone, 20)},
		},
		{
			name: "Death Spike unknown", kit: ClassKit{Rotation: []Step{{Skill: skillDeathSpike}}},
		},
		{
			name: "Bishop heals summed", kit: bishop, items: []Item{reagentStack(spiritOre, 50)},
			skills: []KnownSkill{{ID: skillMajorHeal, Level: 1}, {ID: skillMajorGroupHeal, Level: 1}},
			want:   []buyRow{{item: spiritOre, count: 450}},
		},
		{
			name: "Bishop heals at the min", kit: bishop, items: []Item{reagentStack(spiritOre, 100)},
			skills: []KnownSkill{{ID: skillMajorHeal, Level: 1}, {ID: skillMajorGroupHeal, Level: 1}},
		},
		{
			name: "Greater Might at its known level, listed twice", kit: ClassKit{Rotation: []Step{{Skill: skillGreaterMight, Idle: true}}, Buffs: []Buff{{Skill: skillGreaterMight}}},
			skills: []KnownSkill{{ID: skillGreaterMight, Level: 3}}, want: []buyRow{{item: spiritOre, count: 300}},
		},
		{
			name: "Storm Cubic: no Crystal D seller", kit: ClassKit{Buffs: []Buff{{Skill: skillStormCubic}}},
			skills: []KnownSkill{{ID: skillStormCubic, Level: 1}},
		},
		{
			name: "zero ReagentCastsMax", kit: ClassKit{Rotation: []Step{{Skill: skillDeathSpike}}},
			skills: []KnownSkill{{ID: skillDeathSpike, Level: 1}}, edit: func(b *Brain) { b.persona.Shop.ReagentCastsMax = 0 },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newGearBrain(t, 0, append([]Item{adenaStack(1_000_000)}, tt.items...)...)
			b.persona.Gear.Buy = false
			b.cfg.Travel = reagentTravel()
			withKit(b, tt.kit)
			b.world.Apply(SkillList{Skills: tt.skills}, t0)
			if tt.edit != nil {
				tt.edit(b)
			}
			if got := b.shopWants(false); !slices.Equal(got, tt.want) {
				t.Errorf("shopWants = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestReagentBudget checks reagents are bought only from the adena past
// the supplies tier's floor, after the potions.
func TestReagentBudget(t *testing.T) {
	window := BuyList{Money: 1000 + 5*20 + 360*2, Items: []ShopItem{
		{TemplateID: lesserHealingPotion, Price: 20},
		{TemplateID: cursedBone, Price: 360},
	}}
	b := newGearBrain(t, 0, adenaStack(window.Money))
	b.persona.Gear.Buy = false
	b.persona.Shop.Restock = []Restock{{Item: lesserHealingPotion, Min: 1, Max: 5}}
	b.cfg.Travel = reagentTravel()
	withKit(b, ClassKit{Rotation: []Step{{Skill: skillDeathSpike}}})
	b.world.Apply(SkillList{Skills: []KnownSkill{{ID: skillDeathSpike, Level: 1}}}, t0)
	want := []buyRow{{item: lesserHealingPotion, count: 5}, {item: cursedBone, count: 2}}
	if got := b.buyRows(window); !slices.Equal(got, want) {
		t.Errorf("buyRows = %v, want %v", got, want)
	}
}

// TestReagentNeverJunk checks the kit's reagents, Crystal D with no seller
// included, are never junk, and a reagent of no kit skill follows the junk
// rule.
func TestReagentNeverJunk(t *testing.T) {
	b := newGearBrain(t, 0)
	b.cfg.Travel = reagentTravel()
	withKit(b, ClassKit{Rotation: []Step{{Skill: skillDeathSpike}}, Buffs: []Buff{{Skill: skillStormCubic}}})
	b.world.Apply(SkillList{Skills: []KnownSkill{{ID: skillDeathSpike, Level: 1}, {ID: skillStormCubic, Level: 1}}}, t0)
	for _, tt := range []struct {
		item int32
		uses bool
	}{
		{cursedBone, true},
		{crystalD, true},
		{energyStone, false},
	} {
		if got := b.uses(tt.item); got != tt.uses {
			t.Errorf("uses(%d) = %v, want %v", tt.item, got, tt.uses)
		}
	}
}

// TestReagentShort checks a reagent of the kit carried under its
// ReagentCastsMin sum sends a farmer on a supply trip (runsOut), and none
// carried, enough, or one no spawned merchant sells does not.
func TestReagentShort(t *testing.T) {
	for _, tt := range []struct {
		name  string
		skill int32
		item  int32
		have  int32
		want  bool
	}{
		{"Cursed Bones under the min", skillDeathSpike, cursedBone, 10, true},
		{"Cursed Bones at the min", skillDeathSpike, cursedBone, 20, false},
		{"no Cursed Bone", skillDeathSpike, cursedBone, 0, false},
		{"Crystal D under the min, no seller", skillStormCubic, crystalD, 10, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var items []Item
			if tt.have > 0 {
				items = append(items, reagentStack(tt.item, tt.have))
			}
			b := newGearBrain(t, 0, items...)
			b.persona.Supply.ReturnMinutes = 10
			b.cfg.Travel = reagentTravel()
			withKit(b, ClassKit{Buffs: []Buff{{Skill: tt.skill}}})
			b.world.Apply(SkillList{Skills: []KnownSkill{{ID: tt.skill, Level: 1}}}, t0)
			if got := b.runsOut(); got != tt.want {
				t.Errorf("runsOut = %v, want %v", got, tt.want)
			}
		})
	}
}
