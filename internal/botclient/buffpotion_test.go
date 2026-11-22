package botclient

import (
	"bytes"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Skills of the buff potion tests (aCis_datapack/data/xml/skills): Haste,
// Acumen and their beginners' kinds from the Newbie Guide.
const (
	skillHaste         int32 = 1086
	skillAcumen        int32 = 1085
	skillHasteNewbie   int32 = 4327
	skillAcumenNewbie  int32 = 4329
	skillAlacrity      int32 = 2012
	skillMagicHaste    int32 = 2169
	alacrityStackOrder       = 1
)

// buffPotionData is the datapack slice of the buff potion tests: the
// potions with their skills, the support and beginner buffs of the same
// stack types at their orders per level, a healing potion and a buff that
// stacks with any.
func buffPotionData() (map[int32]datapack.EtcItem, map[datapack.SkillRef]datapack.Skill) {
	buff := func(kind string, order float64, secs int) datapack.Skill {
		return datapack.Skill{Effects: []datapack.Effect{{Name: "Buff", StackType: kind, StackOrder: order, Time: secs}}}
	}
	skills := map[datapack.SkillRef]datapack.Skill{
		{ID: skillAlacrity, Level: 1}:     buff("attack_time_down", alacrityStackOrder, 1200),
		{ID: skillMagicHaste, Level: 1}:   buff("casting_time_down", 2, 1200),
		{ID: skillMagicHaste, Level: 2}:   buff("casting_time_down", 3, 1200),
		{ID: skillHaste, Level: 1}:        buff("attack_time_down", 1, 1200),
		{ID: skillHaste, Level: 2}:        buff("attack_time_down", 2, 1200),
		{ID: skillAcumen, Level: 1}:       buff("casting_time_down", 1, 1200),
		{ID: skillAcumen, Level: 3}:       buff("casting_time_down", 3, 1200),
		{ID: skillHasteNewbie, Level: 1}:  buff("attack_time_down", 2, 3600),
		{ID: skillAcumenNewbie, Level: 1}: buff("casting_time_down", 4, 3600),
		{ID: skillMight, Level: 1}:        buff("pa_up", 1, 1200),
		{ID: 1040, Level: 1}:              buff("none", 0, 1200),
		{ID: 2032, Level: 1}:              {Effects: []datapack.Effect{{Name: "HealOverTime", StackType: hpRecover, StackOrder: 2, Time: 2}}},
	}
	potion := func(skill int32) datapack.EtcItem {
		return datapack.EtcItem{Kind: datapack.EtcPotion, Skills: []datapack.SkillRef{{ID: skill, Level: 1}}}
	}
	etc := map[int32]datapack.EtcItem{
		alacrityPotion:   potion(skillAlacrity),
		magicHastePotion: potion(skillMagicHaste),
		healingPotion:    potion(2032),
		1374:             potion(1040), // a potion of a stackless buff
		1835:             {Kind: datapack.EtcSoulshot, Skills: []datapack.SkillRef{{ID: skillAlacrity, Level: 1}}},
	}
	return etc, skills
}

// buffPotionMap is a travel map holding buffPotionData's index.
func buffPotionMap() *TravelMap {
	m := &TravelMap{}
	m.buffPotions, m.stacks = indexBuffPotions(buffPotionData())
	return m
}

func TestIndexBuffPotions(t *testing.T) {
	m := buffPotionMap()
	wantPotions := map[int32]buffPotion{
		alacrityPotion:   {skill: datapack.SkillRef{ID: skillAlacrity, Level: 1}, stack: stackEffect{"attack_time_down", 1}, lasts: 20 * time.Minute},
		magicHastePotion: {skill: datapack.SkillRef{ID: skillMagicHaste, Level: 1}, stack: stackEffect{"casting_time_down", 2}, lasts: 20 * time.Minute},
	}
	if !maps.Equal(m.buffPotions, wantPotions) {
		t.Errorf("buff potions = %v, want %v", m.buffPotions, wantPotions)
	}
	if got, want := slices.Sorted(maps.Keys(m.stacks)), []int32{skillAcumen, skillHaste, skillAlacrity, skillMagicHaste, skillHasteNewbie, skillAcumenNewbie}; !slices.Equal(got, want) {
		t.Errorf("stack skills = %v, want %v", got, want)
	}
	tests := []struct {
		skill, level int32
		want         stackEffect
		ok           bool
	}{
		{skillHaste, 1, stackEffect{"attack_time_down", 1}, true},
		{skillHaste, 2, stackEffect{"attack_time_down", 2}, true},
		{skillHaste, 115, stackEffect{"attack_time_down", 2}, true}, // enchanted: the top level's
		{skillHaste, 3, stackEffect{}, false},
		{skillAcumen, 3, stackEffect{"casting_time_down", 3}, true},
		{skillHasteNewbie, 1, stackEffect{"attack_time_down", 2}, true},
		{skillMight, 1, stackEffect{}, false},
	}
	for _, tt := range tests {
		if got, ok := m.stackOf(tt.skill, tt.level); got != tt.want || ok != tt.ok {
			t.Errorf("stackOf(%d, %d) = %v, %v, want %v, %v", tt.skill, tt.level, got, ok, tt.want, tt.ok)
		}
	}
}

// newBuffPotionBrain is a brain of class at its farming spot, out of
// combat, with the default build table, buffPotionData's index and items.
func newBuffPotionBrain(t *testing.T, class int32, items ...Item) *Brain {
	t.Helper()
	b, _ := newTestBrain(t, Persona{Gear: GearPolicy{Classes: defaultGearClasses()}})
	b.world.Self.ClassID = class
	b.cfg.Travel = buffPotionMap()
	b.cfg.Spots = NewSpotRegistry([]datapack.Spot{{MaximumNPCs: 9}}, 1)
	b.spot.idx, b.spot.held, b.spot.arrived = 0, true, true
	b.world.Apply(ItemList{Items: items}, t0)
	return b
}

// effects is an AbnormalStatusUpdate of skill and level pairs with 20
// minutes left each.
func effects(pairs ...int32) AbnormalStatusUpdate {
	var e AbnormalStatusUpdate
	for i := 0; i+1 < len(pairs); i += 2 {
		e.Effects = append(e.Effects, Effect{SkillID: pairs[i], Level: pairs[i+1], Left: 20 * time.Minute})
	}
	return e
}

// TestRuleBuffPotion checks when the bot drinks its build's buff potion: at
// its spot, out of combat, while no effect of the potion's stack type with
// an equal or higher order at its level is on it.
func TestRuleBuffPotion(t *testing.T) {
	alacrity := Item{ObjectID: 0x30000021, TemplateID: alacrityPotion, Count: 4}
	magicHaste := Item{ObjectID: 0x30000022, TemplateID: magicHastePotion, Count: 4}
	tests := []struct {
		name  string
		class int32
		items []Item
		setup func(b *Brain)
		want  int32 // the object drunk; 0 for none
	}{
		{name: "fighter drinks Alacrity when absent", class: classHumanFight, items: []Item{alacrity, magicHaste}, want: alacrity.ObjectID},
		{name: "mystic drinks Magic Haste", class: classHumanMystic, items: []Item{alacrity, magicHaste}, want: magicHaste.ObjectID},
		{
			name: "not over a support's Haste of equal order", class: classHumanFight, items: []Item{alacrity},
			setup: func(b *Brain) { b.world.Apply(effects(skillHaste, 1), t0) },
		},
		{
			name: "not over a support's higher Haste", class: classHumanFight, items: []Item{alacrity},
			setup: func(b *Brain) { b.world.Apply(effects(skillHaste, 2), t0) },
		},
		{
			name: "not over an enchanted Haste", class: classHumanFight, items: []Item{alacrity},
			setup: func(b *Brain) { b.world.Apply(effects(skillHaste, 101), t0) },
		},
		{
			name: "not over the beginner Haste", class: classHumanFight, items: []Item{alacrity},
			setup: func(b *Brain) { b.world.Apply(effects(skillMight, 1, skillHasteNewbie, 1), t0) },
		},
		{
			name: "not over its own effect", class: classHumanFight, items: []Item{alacrity},
			setup: func(b *Brain) { b.world.Apply(effects(skillAlacrity, 1), t0) },
		},
		{
			name: "over another stack type", class: classHumanFight, items: []Item{alacrity},
			setup: func(b *Brain) { b.world.Apply(effects(skillAcumenNewbie, 1, skillMight, 1), t0) },
			want:  alacrity.ObjectID,
		},
		{
			name: "over a lower Acumen", class: classHumanMystic, items: []Item{magicHaste},
			setup: func(b *Brain) { b.world.Apply(effects(skillAcumen, 1), t0) },
			want:  magicHaste.ObjectID,
		},
		{
			name: "not over a higher Acumen level", class: classHumanMystic, items: []Item{magicHaste},
			setup: func(b *Brain) { b.world.Apply(effects(skillAcumen, 3), t0) },
		},
		{
			name: "not over the beginner Acumen", class: classHumanMystic, items: []Item{magicHaste},
			setup: func(b *Brain) { b.world.Apply(effects(skillAcumenNewbie, 1), t0) },
		},
		{
			name: "once the Haste ran out", class: classHumanFight, items: []Item{alacrity},
			setup: func(b *Brain) {
				b.world.Apply(AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillHaste, Level: 2, Left: time.Second}}}, t0)
			},
			want: alacrity.ObjectID,
		},
		{name: "in combat", class: classHumanFight, items: []Item{alacrity}, setup: func(b *Brain) { b.world.Self.InCombat = true }},
		{name: "sitting", class: classHumanFight, items: []Item{alacrity}, setup: func(b *Brain) { b.world.Self.Sitting = true }},
		{name: "not at the spot", class: classHumanFight, items: []Item{alacrity}, setup: func(b *Brain) { b.spot.arrived = false }},
		{name: "no spot registry", class: classHumanFight, items: []Item{alacrity}, setup: func(b *Brain) { b.cfg.Spots = nil }},
		{name: "none carried", class: classHumanFight, items: []Item{magicHaste}},
		{
			name: "a build without one", class: classHumanFight, items: []Item{alacrity},
			setup: func(b *Brain) { b.persona.Gear.Classes = []GearClass{{}} },
		},
		{
			name: "a potion the datapack lacks", class: classHumanFight, items: []Item{alacrity},
			setup: func(b *Brain) { delete(b.cfg.Travel.buffPotions, alacrityPotion) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newBuffPotionBrain(t, tt.class, tt.items...)
			if tt.setup != nil {
				tt.setup(b)
			}
			a, ok := b.ruleBuffPotion(at(10 * time.Second))
			if tt.want == 0 {
				if ok {
					t.Fatalf("ruleBuffPotion = %v, want none", a)
				}
				return
			}
			if !ok || a.kind != actUseItem || !bytes.Equal(a.payload, encodeUseItem(tt.want)) {
				t.Fatalf("ruleBuffPotion = %v, %v, want a UseItem of %#x", a, ok, tt.want)
			}
			if rule, _ := b.decide(at(10 * time.Second)); rule != "buffpotion" {
				t.Fatalf("decided rule %q, want buffpotion", rule)
			}
		})
	}
}

// TestBuffPotionGate checks the buff potion's pacing: one UseItem, then
// none until its effect shows in the abnormals or buffPotionWait passes.
func TestBuffPotionGate(t *testing.T) {
	alacrity := Item{ObjectID: 0x30000021, TemplateID: alacrityPotion, Count: 4}
	b := newBuffPotionBrain(t, classHumanFight, alacrity)
	drink := func(d time.Duration, want bool) {
		t.Helper()
		now := at(d)
		a, ok := b.ruleBuffPotion(now)
		if ok != want {
			t.Fatalf("at %v: ruleBuffPotion drinks %v, want %v", d, ok, want)
		}
		if ok {
			if sent, err := b.send(a, now); err != nil || !sent {
				t.Fatalf("at %v: send = %v, %v", d, sent, err)
			}
		}
	}
	drink(10*time.Second, true)
	drink(10*time.Second+500*time.Millisecond, false)
	drink(10*time.Second+buffPotionWait-time.Millisecond, false)
	drink(10*time.Second+buffPotionWait, true) // no effect came: once more
	b.world.Apply(effects(skillAlacrity, 1), at(16*time.Second))
	drink(16*time.Second, false)
	drink(10*time.Minute, false) // the effect holds
	b.world.Apply(AbnormalStatusUpdate{}, at(35*time.Minute))
	drink(35*time.Minute, true) // the effect ended
}

// TestBuffPotionComfort checks the comfort tier's buff potion: a trip's
// worth of the build's potion from a spawned merchant, less what the bot
// carries, within the weight left, and none while saving for the goal.
func TestBuffPotionComfort(t *testing.T) {
	const gearKeep = 10000
	carriedSoE := Item{ObjectID: 300, TemplateID: scrollOfEscape, Count: 1}
	soePotion := []ShopItem{{TemplateID: scrollOfEscape, Price: soePrice}, {TemplateID: magicHastePotion, Price: 2400}}
	goal := append([]ShopItem{{TemplateID: falchion, Price: 244000}}, soePotion...)
	tests := []struct {
		name   string
		adena  int32
		items  []Item
		edit   func(b *Brain)
		window []ShopItem
		want   []buyRow
	}{
		// 60 minutes of a 20-minute effect.
		{name: "a trip's worth", adena: gearKeep + 100000, items: []Item{carriedSoE}, window: soePotion, want: []buyRow{{scrollOfEscape, 1}, {magicHastePotion, 3}}},
		{
			name: "less what it carries", adena: gearKeep + 100000,
			items:  []Item{carriedSoE, {ObjectID: 301, TemplateID: magicHastePotion, Count: 2}},
			window: soePotion, want: []buyRow{{scrollOfEscape, 1}, {magicHastePotion, 1}},
		},
		{
			name: "within the weight left", adena: gearKeep + 100000, items: []Item{carriedSoE}, window: soePotion,
			edit: func(b *Brain) {
				b.persona.Supply.BuyLoad = 45
				b.world.Self.Load, b.world.Self.MaxLoad = 410, 1000
				b.cfg.Travel.weights = map[int32]int32{magicHastePotion: 20, scrollOfEscape: 10}
			},
			want: []buyRow{{scrollOfEscape, 1}, {magicHastePotion, 1}},
		},
		{name: "saving for the goal skips it", adena: gearKeep + 130000, items: []Item{carriedSoE}, window: goal},
		{
			name: "not from an unspawned merchant", adena: gearKeep + 100000, items: []Item{carriedSoE}, window: soePotion,
			edit: func(b *Brain) { delete(b.cfg.Travel.spawnedSold, magicHastePotion) },
			want: []buyRow{{scrollOfEscape, 1}},
		},
		{
			name: "comfort off", adena: gearKeep + 100000, items: []Item{carriedSoE}, window: soePotion,
			edit: func(b *Brain) { b.persona.Shop.Comfort = false },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newBudgetBrain(t, tt.adena, tt.items...)
			b.persona.Supply.TripMinutes = 60
			b.persona.Gear.Classes[0].BuffPotion = magicHastePotion
			m := b.cfg.Travel
			m.buffPotions = buffPotionMap().buffPotions
			m.cheapest[magicHastePotion] = 2400
			m.spawnedSold[magicHastePotion] = true
			if tt.edit != nil {
				tt.edit(b)
			}
			if got := b.buyRows(BuyList{Money: tt.adena, ListID: 1, Items: tt.window}); !slices.Equal(got, tt.want) {
				t.Fatalf("buyRows = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBuffPotionKept checks that the build's buff potion is in the keep
// set: never junk, while another build's is.
func TestBuffPotionKept(t *testing.T) {
	alacrity := Item{ObjectID: 0x30000021, TemplateID: alacrityPotion, Count: 4}
	magicHaste := Item{ObjectID: 0x30000022, TemplateID: magicHastePotion, Count: 4}
	b := newBuffPotionBrain(t, classHumanFight, alacrity, magicHaste)
	b.persona.Shop.SellAt = 1
	if !b.inKeepSet(alacrityPotion) || b.inKeepSet(magicHastePotion) {
		t.Fatalf("inKeepSet(Alacrity, Magic Haste) = %v, %v, want true, false", b.inKeepSet(alacrityPotion), b.inKeepSet(magicHastePotion))
	}
	junk := b.junk()
	if _, sold := junk[alacrity.ObjectID]; sold {
		t.Fatal("the build's Alacrity is junk")
	}
	if _, sold := junk[magicHaste.ObjectID]; !sold {
		t.Fatal("another build's Magic Haste is not junk")
	}
	if b.storeLoot(alacrity) {
		t.Fatal("the build's Alacrity is store loot")
	}
	if _, ok := b.tradePrice(alacrity); ok {
		t.Fatal("the build's Alacrity has a trade price")
	}
}

// TestCrowdedPotion checks rule 3's early drink: with two or more
// attackers on the bot and HP under CrowdedHP it drinks a potion, and
// leaves the turn to rule 4 when none is ready.
func TestCrowdedPotion(t *testing.T) {
	const npc2 = int32(0x20000005)
	potion := Item{ObjectID: 0x30000011, TemplateID: healingPotion, Count: 5}
	hitBy := func(b *Brain, id int32) {
		pos := b.world.Objects[id].Position(t0)
		b.world.Apply(Attack{AttackerID: id, Pos: pos, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, t0)
	}
	tests := []struct {
		name     string
		hp       int32
		setup    func(b *Brain)
		wantRule string
		wantKind actionKind
	}{
		{name: "two attackers: drinks early", hp: 55, setup: func(b *Brain) { hitBy(b, npcID); hitBy(b, npc2) }, wantRule: "critical", wantKind: actUseItem},
		{name: "one attacker: no early drink", hp: 55, setup: func(b *Brain) { hitBy(b, npcID) }, wantRule: "attacked"},
		{name: "two attackers over CrowdedHP", hp: 65, setup: func(b *Brain) { hitBy(b, npcID); hitBy(b, npc2) }, wantRule: "outnumbered"},
		{
			name: "potion cooling: rule 4 flees", hp: 45,
			setup:    func(b *Brain) { hitBy(b, npcID); hitBy(b, npc2); b.usedAt[healingPotion] = at(500 * time.Millisecond) },
			wantRule: "outnumbered", wantKind: actMove,
		},
		{
			name: "sitting: rule 4 stands up", hp: 55,
			setup:    func(b *Brain) { hitBy(b, npcID); hitBy(b, npc2); b.world.Self.Sitting = true },
			wantRule: "outnumbered", wantKind: actSitStand,
		},
		{
			name: "CrowdedHP 0 never drinks early", hp: 55,
			setup:    func(b *Brain) { hitBy(b, npcID); hitBy(b, npc2); b.persona.CrowdedHP = 0 },
			wantRule: "outnumbered",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := newTestBrain(t, Persona{CriticalHP: 20, CrowdedHP: 60, OutmatchedHP: 50, FleeDistance: 600, Potions: []int32{healingPotion}})
			b.world.Apply(NPCInfo{ObjectID: npc2, NPCID: 20001, Attackable: true, Pos: Point{X: -1000}, RunSpeed: 200, WalkSpeed: 50, Running: true}, t0)
			b.world.Apply(ItemList{Items: []Item{potion}}, t0)
			b.world.Self.InCombat, b.world.Self.HP = true, tt.hp
			tt.setup(b)
			rule, a := b.decide(at(time.Second))
			if rule != tt.wantRule || tt.wantKind != 0 && a.kind != tt.wantKind {
				t.Fatalf("decide = %q %v, want %q kind %v", rule, a.kind, tt.wantRule, tt.wantKind)
			}
			if tt.wantKind == actUseItem && !bytes.Equal(a.payload, encodeUseItem(potion.ObjectID)) {
				t.Fatalf("UseItem payload %x, want one of %#x", a.payload, potion.ObjectID)
			}
		})
	}
}
