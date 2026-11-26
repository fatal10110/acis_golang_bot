package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The sustain fixtures (docs/pvp/layer.md "Tests"): the players' DOTs as the
// datapack has them (Poison 129, Bleed 96, Sting 223), a test poison with
// neither abnormal level, and the cure and CP potion stacks.
const (
	susBleed   = int32(96)
	susSting   = int32(223)
	susNoLevel = int32(9310)
)

// dot is a DamOverTime effect of stack type stack ticking count times 3 s
// apart.
func dot(stack, effectType string, count int) []datapack.Effect {
	return []datapack.Effect{{Name: "DamOverTime", StackType: stack, Time: 3, Count: count, EffectType: effectType, EffectPower: -1}}
}

// sustainSkills are the DOT levels the cure cases see on the bot.
var sustainSkills = map[datapack.SkillRef]datapack.Skill{
	{ID: grdPoison, Level: 1}:  {Type: "POISON", AbnormalLevel: 3, EffectAbnormalLevel: -1, Effects: dot("poison", "", 10)},
	{ID: grdPoison, Level: 3}:  {Type: "POISON", AbnormalLevel: 6, EffectAbnormalLevel: -1, Effects: dot("poison", "", 10)},
	{ID: susBleed, Level: 3}:   {Type: "BLEED", AbnormalLevel: 5, EffectAbnormalLevel: -1, Effects: dot("bleeding", "", 7)},
	{ID: susBleed, Level: 6}:   {Type: "BLEED", AbnormalLevel: 8, EffectAbnormalLevel: -1, Effects: dot("bleeding", "", 7)},
	{ID: susSting, Level: 1}:   {Type: "PDAM", EffectType: "BLEED", AbnormalLevel: -1, EffectAbnormalLevel: 3, Effects: dot("bleeding", "BLEED", 7)},
	{ID: susSting, Level: 20}:  {Type: "PDAM", EffectType: "BLEED", AbnormalLevel: -1, EffectAbnormalLevel: 5, Effects: dot("bleeding", "BLEED", 7)},
	{ID: susSting, Level: 45}:  {Type: "PDAM", EffectType: "BLEED", AbnormalLevel: -1, EffectAbnormalLevel: 8, Effects: dot("bleeding", "BLEED", 7)},
	{ID: susNoLevel, Level: 1}: {Type: "POISON", AbnormalLevel: -1, EffectAbnormalLevel: -1, Effects: dot("poison", "", 10)},
}

// sustainBrain is layerBrain against a Warrior with the DOT levels and the
// bot carrying one of each of items.
func sustainBrain(t *testing.T, items ...int32) *Brain {
	t.Helper()
	b := layerBrain(t, lyrWarrior)
	for ref, s := range sustainSkills {
		b.cfg.Travel.skills[ref] = s
	}
	b.world.Inventory.Items = map[int32]Item{}
	for i, id := range items {
		oid := int32(0x30000000 + i)
		b.world.Inventory.Items[oid] = Item{ObjectID: oid, TemplateID: id, Count: 5}
	}
	return b
}

// allCures are the four cure items.
var allCures = []int32{itemAntidote, itemGreaterAntidote, itemBandage, itemEmergencyDressing}

// itemUsed is the template of the item b's decision at now uses, 0 for any
// other action.
func itemUsed(t *testing.T, b *Brain, now time.Time) int32 {
	t.Helper()
	_, a := b.decide(now)
	if a.kind != actUseItem {
		return 0
	}
	return a.item
}

// TestPvPCPPotion checks the CP potion (docs/pvp/layer.md "CP"): under
// CPPotion CP in a PvP fight the greater one, under the item reuse none;
// none against a monster or in a duel, where a poison still gets its cure.
func TestPvPCPPotion(t *testing.T) {
	tests := []struct {
		name  string
		cp    int32
		items []int32
		setup func(t *testing.T, b *Brain)
		want  int32
	}{
		{name: "CP under 50 %: the greater CP potion", cp: 100, items: []int32{itemCPPotion, itemGreaterCPPotion}, want: itemGreaterCPPotion},
		{name: "only the plain one carried", cp: 100, items: []int32{itemCPPotion}, want: itemCPPotion},
		{name: "CP at 50 %: none", cp: 200, items: []int32{itemGreaterCPPotion}},
		{
			name: "inside the item reuse: none", cp: 100, items: []int32{itemGreaterCPPotion},
			setup: func(_ *testing.T, b *Brain) {
				b.usedAt[itemCPPotion] = at(10*time.Second - pvpItemReuse + time.Millisecond)
			},
		},
		{
			name: "the item reuse over: the potion", cp: 100, items: []int32{itemGreaterCPPotion}, want: itemGreaterCPPotion,
			setup: func(_ *testing.T, b *Brain) { b.usedAt[itemGreaterCPPotion] = at(10*time.Second - pvpItemReuse) },
		},
		{
			name: "PvE: none", cp: 100, items: []int32{itemGreaterCPPotion},
			setup: func(t *testing.T, b *Brain) {
				b.handleOne(t, DeleteObject{ObjectID: pcID}, t0)
				b.handleOne(t, NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 30}}, t0)
				b.handleOne(t, hitSelf(npcID, 10), t0)
				b.world.Target = npcID
			},
		},
		{
			name: "a duel: none", cp: 100, items: []int32{itemGreaterCPPotion},
			setup: func(t *testing.T, b *Brain) { duelUntil(t, b, DuelFighting, t0) },
		},
		{
			name: "a duel: a Poison 129 gets its cure", cp: 100, items: append([]int32{itemGreaterCPPotion}, allCures...), want: itemAntidote,
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelFighting, t0)
				b.handleOne(t, AbnormalStatusUpdate{Effects: []Effect{{SkillID: grdPoison, Level: 1, Left: 28 * time.Second}}}, at(10*time.Second))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := sustainBrain(t, tc.items...)
			b.world.Self.CP, b.world.Self.MaxCP = tc.cp, 400
			if tc.setup != nil {
				tc.setup(t, b)
			}
			if got := itemUsed(t, b, at(10*time.Second)); got != tc.want {
				t.Errorf("item used %d, want %d", got, tc.want)
			}
		})
	}
}

// TestPvPCure checks the cures (docs/pvp/layer.md "Cures"): the cure of the
// lower set abnormal level for a poison or bleed with half its ticks or
// more left, the plain one first, none past level 7, none with neither
// level set, none against a monster, one per effect seen.
func TestPvPCure(t *testing.T) {
	tests := []struct {
		name   string
		effect Effect
		items  []int32
		setup  func(t *testing.T, b *Brain)
		want   int32
	}{
		{name: "Poison 129 level 1 (abnormalLvl 3), most ticks left: Antidote", effect: Effect{SkillID: grdPoison, Level: 1, Left: 28 * time.Second}, want: itemAntidote},
		{name: "Poison 129 level 3 (abnormalLvl 6): Greater Antidote", effect: Effect{SkillID: grdPoison, Level: 3, Left: 28 * time.Second}, want: itemGreaterAntidote},
		{name: "Poison 129 level 1, only the greater carried: Greater Antidote", effect: Effect{SkillID: grdPoison, Level: 1, Left: 28 * time.Second}, items: []int32{itemGreaterAntidote}, want: itemGreaterAntidote},
		{name: "Poison 129 level 3, only the plain carried: none", effect: Effect{SkillID: grdPoison, Level: 3, Left: 28 * time.Second}, items: []int32{itemAntidote}},
		{name: "Poison 129 with under half its ticks left: none", effect: Effect{SkillID: grdPoison, Level: 1, Left: 14 * time.Second}},
		{name: "Bleed abnormalLvl 5: Emergency Dressing", effect: Effect{SkillID: susBleed, Level: 3, Left: 20 * time.Second}, want: itemEmergencyDressing},
		{name: "Bleed abnormalLvl 8: none", effect: Effect{SkillID: susBleed, Level: 6, Left: 20 * time.Second}},
		{name: "Sting level 1 (effectAbnormalLvl 3): Bandage", effect: Effect{SkillID: susSting, Level: 1, Left: 20 * time.Second}, want: itemBandage},
		{name: "Sting level 20 (5): Emergency Dressing", effect: Effect{SkillID: susSting, Level: 20, Left: 20 * time.Second}, want: itemEmergencyDressing},
		{name: "Sting level 45 (8): none", effect: Effect{SkillID: susSting, Level: 45, Left: 20 * time.Second}},
		{name: "a poison with neither level: none", effect: Effect{SkillID: susNoLevel, Level: 1, Left: 28 * time.Second}},
		{
			name: "against a monster: none", effect: Effect{SkillID: grdPoison, Level: 1, Left: 28 * time.Second},
			setup: func(t *testing.T, b *Brain) {
				b.handleOne(t, DeleteObject{ObjectID: pcID}, t0)
				b.handleOne(t, NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 30}}, t0)
				b.handleOne(t, hitSelf(npcID, 10), t0)
				b.world.Target = npcID
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			items := tc.items
			if items == nil {
				items = allCures
			}
			b := sustainBrain(t, items...)
			if tc.setup != nil {
				tc.setup(t, b)
			}
			b.handleOne(t, AbnormalStatusUpdate{Effects: []Effect{tc.effect}}, at(time.Second))
			if got := itemUsed(t, b, at(time.Second)); got != tc.want {
				t.Errorf("item used %d, want %d", got, tc.want)
			}
		})
	}
	t.Run("one cure per effect seen", func(t *testing.T) {
		b := sustainBrain(t, allCures...)
		poison := Effect{SkillID: grdPoison, Level: 1, Left: 28 * time.Second}
		b.handleOne(t, AbnormalStatusUpdate{Effects: []Effect{poison}}, at(time.Second))
		_, a := b.decide(at(time.Second))
		if a.kind != actUseItem || a.item != itemAntidote {
			t.Fatalf("action %v item %d, want the Antidote", a.kind, a.item)
		}
		if _, err := b.send(a, at(time.Second)); err != nil {
			t.Fatal(err)
		}
		// A list resent for another effect: the same poison, a second
		// less, gets no second cure.
		poison.Left = 27 * time.Second
		b.handleOne(t, AbnormalStatusUpdate{Effects: []Effect{poison, {SkillID: skillWindStrike, Level: 1, Left: time.Minute}}}, at(2*time.Second))
		if got := itemUsed(t, b, at(2*time.Second)); got != 0 {
			t.Fatalf("item used %d for the poison cured, want none", got)
		}
		// The poison landing anew is a new effect.
		poison.Left = 30 * time.Second
		b.handleOne(t, AbnormalStatusUpdate{Effects: []Effect{poison}}, at(10*time.Second))
		if got := itemUsed(t, b, at(10*time.Second)); got != itemAntidote {
			t.Fatalf("item used %d for a new poison, want the Antidote", got)
		}
	})
}
