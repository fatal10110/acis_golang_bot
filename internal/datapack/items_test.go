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
		1:    {ID: 1, Name: "Short Sword", Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Price: 768},
		6:    {ID: 6, Name: "Apprentice's Wand", Type: "Weapon", BodyPart: "rhand", Kind: "BLUNT", Magical: true, Price: 138},
		49:   {ID: 49, Name: "Gloves", Type: "Armor", BodyPart: "gloves", Price: 2650},
		398:  {ID: 398, Name: "Plate Leather", Type: "Armor", BodyPart: "chest", Kind: "LIGHT", Grade: 1, Price: 74600},
		6611: {ID: 6611, Name: "Infinity Blade", Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Grade: 5, Conditional: true},
		6702: {ID: 6702, Name: "Shadow Item: Falchion", Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Duration: 120, Conditional: true},
	}
	if !reflect.DeepEqual(dp.Items, want) {
		t.Errorf("Items = %+v, want %+v", dp.Items, want)
	}
}

func TestLoadItemsRejects(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"grade", `<item id="1" type="Weapon" name="x"><set name="crystal_type" val="Z"/></item>`, "crystal_type"},
		{"no id", `<item type="Armor" name="x"/>`, `"id"`},
		{"price", `<item id="1" type="Armor" name="x"><set name="price" val="many"/></item>`, `"price"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.xml"), []byte("<list>"+tt.body+"</list>"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadItems(dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadItems err = %v, want one naming %s", err, tt.want)
			}
		})
	}
}
