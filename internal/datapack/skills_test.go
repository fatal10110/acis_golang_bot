package datapack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadSkills(t *testing.T) {
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	haste := func(order float64) []Effect {
		return []Effect{{Name: "Buff", StackType: "attack_time_down", StackOrder: order, Time: 1200, Count: 1}}
	}
	hot := func(order float64) []Effect {
		return []Effect{{Name: "HealOverTime", StackType: "hp_recover", StackOrder: order, Time: 2, Count: 7}}
	}
	magicHaste := func(order float64) []Effect {
		return []Effect{{Name: "Buff", StackType: "casting_time_down", StackOrder: order, Time: 1200, Count: 1}}
	}
	// The stack order of 1086 and 2169 differs by level (#stackOrder);
	// 1086's enchanted levels are not read; 4327 keeps its last
	// declaration.
	want := map[SkillRef]Skill{
		{1086, 1}: {Name: "Haste", Effects: haste(1)},
		{1086, 2}: {Name: "Haste", Effects: haste(2)},
		{2002, 1}: {Name: "Healing Drug", Potion: true, Effects: []Effect{{Name: "HealOverTime", StackType: "hp_recover", StackOrder: 1.5, Time: 5, Count: 4}}},
		{2012, 1}: {Name: "Potion of Alacrity", Potion: true, Effects: haste(1)},
		{2013, 1}: {Name: "Scroll of Escape"},
		{2031, 1}: {Name: "Lesser Healing Potion", Potion: true, Effects: hot(1)},
		{2032, 1}: {Name: "Healing potion", Potion: true, Effects: hot(2)},
		{2037, 1}: {Name: "Greater Healing Potion", Potion: true, Effects: hot(3)},
		{2169, 1}: {Name: "Magic Haste Potion", Potion: true, Effects: magicHaste(2)},
		{2169, 2}: {Name: "Magic Haste Potion", Potion: true, Effects: magicHaste(3)},
		{4327, 1}: {Name: "Haste for Beginners", Effects: []Effect{{Name: "Buff", StackType: "attack_time_down", StackOrder: 2, Time: 3600, Count: 1}}},
	}
	if !reflect.DeepEqual(dp.Skills, want) {
		t.Errorf("Skills = %+v, want %+v", dp.Skills, want)
	}
}

func TestLoadSkillsRejects(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"no id", `<skill levels="1" name="x"/>`, `"id"`},
		{"no levels", `<skill id="1" name="x"/>`, `"levels"`},
		{"negative levels", `<skill id="1" levels="-1" name="x"/>`, `"levels"`},
		{"no name", `<skill id="1" levels="1"/>`, `"name"`},
		{"table name", `<skill id="1" levels="1" name="x"><table name="order"> 1 </table></skill>`, "table name"},
		{"stack order", `<skill id="1" levels="1" name="x"><for><effect name="Buff" stackOrder="high"/></for></skill>`, `"stackOrder"`},
		{"time", `<skill id="1" levels="1" name="x"><for><effect name="Buff" time="1.5"/></for></skill>`, `"time"`},
		{"count", `<skill id="1" levels="1" name="x"><for><effect name="Buff" count="many"/></for></skill>`, `"count"`},
		{"effect name", `<skill id="1" levels="1" name="x"><for><effect name="" time="1"/></for></skill>`, `"name"`},
		{"undefined table", `<skill id="1" levels="1" name="x"><for><effect name="Buff" stackOrder="#order"/></for></skill>`, "undefined table"},
		{"short table", `<skill id="1" levels="2" name="x"><table name="#order"> 1 </table><for><effect name="Buff" stackOrder="#order"/></for></skill>`, "no row 2"},
		{"potion table", `<skill id="1" levels="1" name="x"><set name="isPotion" val="#potion"/></skill>`, "undefined table"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.xml"), []byte("<list>"+tt.body+"</list>"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadSkills(dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadSkills err = %v, want one naming %s", err, tt.want)
			}
		})
	}
}
