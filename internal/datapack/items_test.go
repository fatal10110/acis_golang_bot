package datapack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadItems(t *testing.T) {
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := map[int32]Item{
		1:    {ID: 1, Name: "Short Sword", Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Price: 768, Weight: 1600, Stats: Stats{PAtk: 8}},
		6:    {ID: 6, Name: "Apprentice's Wand", Type: "Weapon", BodyPart: "rhand", Kind: "BLUNT", Magical: true, Price: 138},
		17:   {ID: 17, Name: "Wooden Arrow", Type: "EtcItem", BodyPart: "lhand", Kind: "ARROW", Price: 2},
		49:   {ID: 49, Name: "Gloves", Type: "Armor", BodyPart: "gloves", Price: 2650},
		398:  {ID: 398, Name: "Plate Leather", Type: "Armor", BodyPart: "chest", Kind: "LIGHT", Grade: 1, Price: 74600},
		6611: {ID: 6611, Name: "Infinity Blade", Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Grade: 5, Conditional: true},
		6702: {ID: 6702, Name: "Shadow Item: Falchion", Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Duration: 120, Conditional: true},
		10:   {ID: 10, Name: "Dagger", Type: "Weapon", BodyPart: "rhand", Kind: "DAGGER", Price: 138},
		287: {
			ID: 287, Name: "Bow of Peril", Type: "Weapon", BodyPart: "lrhand", Kind: "BOW", Grade: 3, Price: 13100000, Weight: 1700,
			Stats: Stats{PAtk: 400, MAtk: 99, PAtkSpd: 293, RCrit: 12}, Enchants: []string{"pAtk", "mAtk"}, Soulshots: 3, Spiritshots: 1,
		},
		352: {
			ID: 352, Name: "Brigandine Tunic", Type: "Armor", BodyPart: "chest", Kind: "HEAVY", Grade: 1, Price: 299000, Weight: 8320,
			Stats: Stats{PDef: 103}, Enchants: []string{"pDef"},
		},
		629: {
			ID: 629, Name: "Kite Shield", Type: "Armor", BodyPart: "lhand", Grade: 1, Price: 78400, Weight: 1320,
			Stats: Stats{SDef: 142}, Enchants: []string{"sDef"},
		},
	}
	if !reflect.DeepEqual(dp.Items, want) {
		t.Errorf("Items = %+v, want %+v", dp.Items, want)
	}
	// Every priced template, the steel powder too; never the untradable
	// dagger, nor 6611 and 6702 priced 0.
	if want := map[int32]int32{1: 768, 6: 138, 17: 2, 49: 2650, 398: 74600, 1664: 80, 287: 13100000, 352: 299000, 629: 78400, 1061: 330}; !reflect.DeepEqual(dp.Prices, want) {
		t.Errorf("Prices = %v, want %v", dp.Prices, want)
	}
}

func TestLoadItemsRejects(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"grade", `<item id="1" type="Weapon" name="x"><set name="crystal_type" val="Z"/></item>`, "crystal_type"},
		{"no id", `<item type="Armor" name="x"/>`, `"id"`},
		{"price", `<item id="1" type="Armor" name="x"><set name="price" val="many"/></item>`, `"price"`},
		{"arrow grade", `<item id="1" type="EtcItem" name="x"><set name="etcitem_type" val="ARROW"/><set name="crystal_type" val="Z"/></item>`, "crystal_type"},
		{"stat", `<item id="1" type="Weapon" name="x"><for><set stat="pAtk" val="high"/></for></item>`, `stat="pAtk"`},
		{"baseadd", `<item id="1" type="Armor" name="x"><for><baseadd stat="pDef" val="1,5"/></for></item>`, `stat="pDef"`},
		{"enchant stat", `<item id="1" type="Armor" name="x"><for><enchant val="0"/></for></item>`, "<enchant>"},
		{"soulshots", `<item id="1" type="Weapon" name="x"><set name="soulshots" val="x"/></item>`, `"soulshots"`},
		{"etc grade", `<item id="1" type="EtcItem" name="x"><set name="etcitem_type" val="POTION"/><set name="crystal_type" val="Z"/></item>`, "crystal_type"},
		{"etc weight", `<item id="1" type="EtcItem" name="x"><set name="etcitem_type" val="DYE"/><set name="weight" val="1.5"/></item>`, `"weight"`},
		{"etc reuse", `<item id="1" type="EtcItem" name="x"><set name="etcitem_type" val="POTION"/><set name="reuse_delay" val="10s"/></item>`, `"reuse_delay"`},
		{"etc name", `<item id="1" type="EtcItem"><set name="etcitem_type" val="MATERIAL"/></item>`, `"name"`},
		{"item skill pair", `<item id="1" type="EtcItem" name="x"><set name="etcitem_type" val="SCROLL"/><set name="item_skill" val="2013"/></item>`, "item_skill"},
		{"item skill level", `<item id="1" type="EtcItem" name="x"><set name="handler" val="SoulShots"/><set name="item_skill" val="2039-1;2040-x"/></item>`, "item_skill"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.xml"), []byte("<list>"+tt.body+"</list>"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, _, _, err := LoadItems(dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadItems err = %v, want one naming %s", err, tt.want)
			}
		})
	}
}
