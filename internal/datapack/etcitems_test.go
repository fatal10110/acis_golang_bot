package datapack

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLoadEtcItems(t *testing.T) {
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Not kept: arrows (an Item), adena and steel powder (no etcitem_type),
	// the fishing shot and the elixir.
	want := map[int32]EtcItem{
		1061: {
			ID: 1061, Name: "Healing Potion", Kind: EtcPotion, Type: "POTION", Weight: 5,
			Skills: []SkillRef{{2032, 1}}, Reuse: 10 * time.Second, Tradable: true, Sellable: true,
		},
		735: {
			ID: 735, Name: "Potion of Alacrity", Kind: EtcPotion, Type: "POTION", Weight: 20,
			Skills: []SkillRef{{2012, 1}}, Reuse: 500 * time.Millisecond, Tradable: true, Sellable: true,
		},
		736: {
			ID: 736, Name: "Scroll of Escape", Kind: EtcScroll, Type: "SCROLL", Weight: 120,
			Skills: []SkillRef{{2013, 1}}, Tradable: true, Sellable: true,
		},
		5789: {
			ID: 5789, Name: "Soulshot: No Grade for Beginners", Kind: EtcSoulshot, Weight: 1,
			Skills: []SkillRef{{2039, 1}},
		},
		1835: {
			ID: 1835, Name: "Soulshot: No Grade", Kind: EtcSoulshot, Weight: 2,
			Skills: []SkillRef{{2039, 1}}, Tradable: true, Sellable: true,
		},
		2510: {
			ID: 2510, Name: "Spiritshot: D-Grade", Kind: EtcSpiritshot, Grade: 1, Weight: 5,
			Skills: []SkillRef{{2047, 1}}, Tradable: true, Sellable: true,
		},
		3947: {
			ID: 3947, Name: "Blessed Spiritshot: No Grade", Kind: EtcBlessedSpiritshot, Weight: 5,
			Skills: []SkillRef{{2061, 1}}, Tradable: true, Sellable: true,
		},
		729: {
			ID: 729, Name: "Scroll: Enchant Weapon (Grade A)", Kind: EtcEnchantScroll, Type: "SCRL_ENCHANT_WP", Weight: 120,
			Skills: []SkillRef{{2006, 1}}, Tradable: true, Sellable: true,
		},
		731: {
			ID: 731, Name: "Crystal Scroll: Enchant Weapon (Grade A)", Kind: EtcEnchantScroll, Type: "SCROLL", Weight: 120,
			Tradable: true, Sellable: true,
		},
		1864: {ID: 1864, Name: "Stem", Kind: EtcMaterial, Type: "MATERIAL", Weight: 2, Tradable: true, Sellable: true},
		4445: {ID: 4445, Name: "Dye of STR (Str+1 Con-3)", Kind: EtcDye, Type: "DYE", Weight: 150, Tradable: true, Sellable: true},
		8786: {
			ID: 8786, Name: "Primeval Potion", Kind: EtcPotion, Type: "POTION",
			Skills: []SkillRef{{2305, 1}, {2306, 2}}, Tradable: true, Sellable: true,
		},
	}
	if !reflect.DeepEqual(dp.EtcItems, want) {
		t.Errorf("EtcItems = %+v, want %+v", dp.EtcItems, want)
	}
	if _, ok := dp.Items[1835]; ok {
		t.Error("Items keeps 1835, declared again as a soulshot")
	}
}
