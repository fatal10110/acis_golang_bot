package botclient

import (
	"os"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

func TestIndexPotions(t *testing.T) {
	hot := func(order float64) datapack.Skill {
		return datapack.Skill{Potion: true, Effects: []datapack.Effect{{Name: "HealOverTime", StackType: "hp_recover", StackOrder: order}}}
	}
	skills := map[datapack.SkillRef]datapack.Skill{
		{ID: 2031, Level: 1}: hot(1),
		{ID: 2032, Level: 1}: hot(2),
		{ID: 2037, Level: 1}: hot(3),
		{ID: 2012, Level: 1}: {Potion: true, Effects: []datapack.Effect{{Name: "Buff", StackType: "attack_time_down", StackOrder: 1}}},
		{ID: 2279, Level: 1}: {Effects: []datapack.Effect{{Name: "ManaHealOverTime", StackType: "mp_recover", StackOrder: 1}}},
	}
	potion := func(skill int32) datapack.EtcItem {
		return datapack.EtcItem{Kind: datapack.EtcPotion, Skills: []datapack.SkillRef{{ID: skill, Level: 1}}}
	}
	etc := map[int32]datapack.EtcItem{
		1060: potion(2031),
		1073: potion(2031), // Beginner's Potion: Lesser's skill
		1061: potion(2032),
		1539: potion(2037),
		9206: potion(2037), // the event Greater Healing Potion
		735:  potion(2012), // a buff
		728:  potion(2279), // heals MP
		1540: potion(2038), // no such skill level
		65:   {Kind: datapack.EtcPotion},
		1835: {Kind: datapack.EtcSoulshot, Skills: []datapack.SkillRef{{ID: 2031, Level: 1}}},
	}
	want := []int32{1539, 9206, 1061, 1060, 1073}
	if got := indexPotions(etc, skills); !slices.Equal(got, want) {
		t.Errorf("indexPotions = %v, want %v", got, want)
	}
}

func TestIndexShots(t *testing.T) {
	shot := func(kind datapack.EtcKind, grade int, sellable bool) datapack.EtcItem {
		return datapack.EtcItem{Kind: kind, Grade: grade, Sellable: sellable}
	}
	etc := map[int32]datapack.EtcItem{
		1467: shot(datapack.EtcSoulshot, 5, true),
		1463: shot(datapack.EtcSoulshot, 1, true),
		1835: shot(datapack.EtcSoulshot, 0, true),
		5789: shot(datapack.EtcSoulshot, 0, false),
		2510: shot(datapack.EtcSpiritshot, 1, true),
		2509: shot(datapack.EtcSpiritshot, 0, true),
		5790: shot(datapack.EtcSpiritshot, 0, false),
		3948: shot(datapack.EtcBlessedSpiritshot, 1, true),
		3947: shot(datapack.EtcBlessedSpiritshot, 0, true),
		1061: {Kind: datapack.EtcPotion},
	}
	want := []int32{5789, 1835, 1463, 1467, 5790, 2509, 2510, 3947, 3948}
	if got := indexShots(etc); !slices.Equal(got, want) {
		t.Errorf("indexShots = %v, want %v", got, want)
	}
}

func TestWithItemIndex(t *testing.T) {
	index := &TravelMap{potions: []int32{1539, 1061}, shots: []int32{5789, 1835}}
	tests := []struct {
		name                string
		m                   *TravelMap
		potions, shots      []int32
		wantPots, wantShots []int32
	}{
		{"built-in lists", index, healingPotions, shotTemplates, []int32{1539, 1061}, []int32{5789, 1835}},
		{"own lists", index, []int32{1060}, []int32{1835}, []int32{1060}, []int32{1835}},
		{"none", index, nil, nil, nil, nil},
		{"index lists none", &TravelMap{}, healingPotions, shotTemplates, healingPotions, shotTemplates},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.m.WithItemIndex(Persona{Potions: tt.potions, Shots: tt.shots, CriticalHP: 20})
			if !slices.Equal(p.Potions, tt.wantPots) || !slices.Equal(p.Shots, tt.wantShots) || p.CriticalHP != 20 {
				t.Errorf("WithItemIndex = potions %v shots %v critical %d, want %v %v 20", p.Potions, p.Shots, p.CriticalHP, tt.wantPots, tt.wantShots)
			}
		})
	}
}

// TestItemIndexShipped checks the index of a real datapack when
// BOT_DATAPACK_XML names its data/xml directory, and skips otherwise: the
// shots are the built-in list, in its order; the potions are the built-in
// ones, best first, with the Beginner's Potion and the event Greater
// Healing Potion, which heal alike.
func TestItemIndexShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	dp, err := datapack.Load(dir)
	if err != nil {
		t.Fatalf("Load(%s): %v", dir, err)
	}
	m := NewTravelMap(dp)
	if !slices.Equal(m.shots, shotTemplates) {
		t.Errorf("shots = %v, want %v", m.shots, shotTemplates)
	}
	if want := []int32{1539, 9206, 1061, 1060, 1073}; !slices.Equal(m.potions, want) {
		t.Errorf("potions = %v, want %v", m.potions, want)
	}
}
