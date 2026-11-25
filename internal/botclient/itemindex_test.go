package botclient

import (
	"maps"
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
	etc := map[int32]datapack.EtcItem{
		1463: {Kind: datapack.EtcSoulshot, Grade: 1, Sellable: true},
		1835: {Kind: datapack.EtcSoulshot, Sellable: true},
		5789: {Kind: datapack.EtcSoulshot},
		2510: {Kind: datapack.EtcSpiritshot, Grade: 1, Sellable: true},
		3948: {Kind: datapack.EtcBlessedSpiritshot, Grade: 1, Sellable: true},
		1061: {Kind: datapack.EtcPotion},
	}
	want := map[int32]shotItem{
		1463: {datapack.EtcSoulshot, 1, false},
		1835: {datapack.EtcSoulshot, 0, false},
		5789: {datapack.EtcSoulshot, 0, true},
		2510: {datapack.EtcSpiritshot, 1, false},
		3948: {datapack.EtcBlessedSpiritshot, 1, false},
	}
	if got := indexShots(etc); !maps.Equal(got, want) {
		t.Errorf("indexShots = %v, want %v", got, want)
	}
}

func TestWithItemIndex(t *testing.T) {
	index := &TravelMap{potions: []int32{1539, 1061}}
	tests := []struct {
		name     string
		m        *TravelMap
		potions  []int32
		wantPots []int32
	}{
		{"built-in list", index, healingPotions, []int32{1539, 1061}},
		{"own list", index, []int32{1060}, []int32{1060}},
		{"none", index, nil, nil},
		{"index lists none", &TravelMap{}, healingPotions, healingPotions},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.m.WithItemIndex(Persona{Potions: tt.potions, Shots: []int32{1835}, CriticalHP: 20})
			if !slices.Equal(p.Potions, tt.wantPots) || !slices.Equal(p.Shots, []int32{1835}) || p.CriticalHP != 20 {
				t.Errorf("WithItemIndex = potions %v shots %v critical %d, want %v [1835] 20", p.Potions, p.Shots, p.CriticalHP, tt.wantPots)
			}
		})
	}
}

// TestItemIndexShipped checks the index of a real datapack when
// BOT_DATAPACK_XML names its data/xml directory, and skips otherwise: the
// shots are the built-in table, no spawned merchant sells D-grade shots
// and the grocers sell no-grade ones; the potions are the built-in
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
	m := NewTravelMap(dp, DefaultSupplyConfig())
	if !maps.Equal(m.shots, builtinShots) {
		t.Errorf("shots = %v, want %v", m.shots, builtinShots)
	}
	for id, sold := range map[int32]bool{1835: true, 2509: true, 1463: false, 2510: false, 3948: false} {
		if m.sellsShot(id) != sold {
			t.Errorf("sellsShot(%d) = %v, want %v", id, !sold, sold)
		}
	}
	if want := []int32{1539, 9206, 1061, 1060, 1073}; !slices.Equal(m.potions, want) {
		t.Errorf("potions = %v, want %v", m.potions, want)
	}
}
