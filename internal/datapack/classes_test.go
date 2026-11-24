package datapack

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLoadClasses(t *testing.T) {
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	elfFighter := []SkillGrant{{ID: 77, Level: 1, MinLevel: 10, Cost: 910}, {ID: 142, Level: 1, MinLevel: 5, Cost: 160}}
	elfKnight := append([]SkillGrant{{ID: 77, Level: 2, MinLevel: 28, Cost: 5000}}, elfFighter...)
	// 19's later <set pAtk> wins and its runSpd defaults to 1; 19 holds
	// its own grants, then 18's; 20 its own, then 19's merged tree; 49
	// takes the default base stats.
	want := map[int32]Class{
		18: {
			ID: 18, Parent: -1, Race: 1, STR: 36, CON: 36, DEX: 35, INT: 23, WIT: 14, MEN: 26,
			PAtk: 4, PDef: 80, MAtk: 6, MDef: 41, RunSpeed: 125,
			HPTable: []float64{89, 101.74, 114.62}, MPTable: []float64{30, 35.46, 40.98}, Skills: elfFighter,
		},
		19: {
			ID: 19, Parent: 18, Race: 1, STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
			PAtk: 5, PDef: 80, MAtk: 6, MDef: 41, RunSpeed: 1,
			HPTable: []float64{89}, MPTable: []float64{30}, Skills: elfKnight,
		},
		20: {
			ID: 20, Parent: 19, Race: 1, STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
			PAtk: 4, PDef: 80, MAtk: 6, MDef: 41, RunSpeed: 1,
			HPTable: []float64{89}, MPTable: []float64{30},
			Skills: append([]SkillGrant{{ID: 10, Level: 1, MinLevel: 40, Cost: 25000}}, elfKnight...),
		},
		49: {
			ID: 49, Parent: -1, Race: 3, STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
			PAtk: 3, PDef: 54, MAtk: 6, MDef: 41, RunSpeed: 1,
			HPTable: []float64{95, 106.5}, MPTable: []float64{40, 47.4},
		},
	}
	if !reflect.DeepEqual(dp.Classes, want) {
		t.Errorf("Classes = %+v, want %+v", dp.Classes, want)
	}
}

func TestLoadClassesRejects(t *testing.T) {
	const stats = `pAtk="1" pDef="1" mAtk="1" mDef="1" hpTable="1" mpTable="1"`
	tests := []struct {
		name, body, want string
	}{
		{"no id", `<class><set ` + stats + `/></class>`, `"id"`},
		{"unknown id", `<class><set id="60" ` + stats + `/></class>`, "class 60: unknown profession id"},
		{"parent not loaded", `<class><set id="19" ` + stats + `/></class>`, "parent class 18 not loaded"},
		{"no pAtk", `<class><set id="18" pDef="1" mAtk="1" mDef="1" hpTable="1" mpTable="1"/></class>`, `"pAtk"`},
		{"no mDef", `<class><set id="18" pAtk="1" pDef="1" mAtk="1" hpTable="1" mpTable="1"/></class>`, `"mDef"`},
		{"str", `<class><set id="18" str="strong" ` + stats + `/></class>`, `"str"`},
		{"run speed", `<class><set id="18" runSpd="fast" ` + stats + `/></class>`, `"runSpd"`},
		{"no hpTable", `<class><set id="18" pAtk="1" pDef="1" mAtk="1" mDef="1" mpTable="1"/></class>`, `"hpTable"`},
		{"empty mpTable row", `<class><set id="18" pAtk="1" pDef="1" mAtk="1" mDef="1" hpTable="1" mpTable="1;;2"/></class>`, `"mpTable"`},
		{"grant cost", `<class><set id="18" ` + stats + `/><skills><skill id="1" lvl="1" minLvl="1"/></skills></class>`, `"cost"`},
		{"grant level", `<class><set id="18" ` + stats + `/><skills><skill id="1" lvl="x" minLvl="1" cost="1"/></skills></class>`, `"lvl"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.xml"), []byte("<list>"+tt.body+"</list>"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadClasses(dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadClasses err = %v, want one naming %s", err, tt.want)
			}
		})
	}
}

// TestClassParentsOrder checks the invariant the single ascending merge
// pass of LoadClasses rests on: every parent ID is a known profession
// smaller than its child's, and every line roots in a base class of known
// race.
func TestClassParentsOrder(t *testing.T) {
	for id, parent := range classParents {
		if parent < 0 {
			if _, ok := baseClassRaces[id]; !ok {
				t.Errorf("base class %d has no race", id)
			}
			continue
		}
		if _, ok := classParents[parent]; !ok || parent >= id {
			t.Errorf("class %d: parent %d unknown or not smaller", id, parent)
		}
	}
}

// TestLoadShippedClassesAndCombat checks the M10 readers against a real
// datapack when BOT_DATAPACK_XML names its data/xml directory, and skips
// otherwise: every profession loads with its line's race, Elven Knight's
// merged tree holds Attack Aura from both tiers, Focused Force,
// Hurricane Assault and Accuracy carry their charge and drain fields,
// Spellcraft and Armor Mastery their armor kinds, every set skill its
// stat functions, and Auron teaches the human fighter line.
func TestLoadShippedClassesAndCombat(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	dp, err := Load(dir)
	if err != nil {
		t.Fatalf("Load(%s): %v", dir, err)
	}
	if len(dp.Classes) != len(classParents) {
		t.Errorf("loaded %d classes, want %d", len(dp.Classes), len(classParents))
	}
	for id, c := range dp.Classes {
		if len(c.HPTable) < 80 || len(c.MPTable) < 80 {
			t.Errorf("class %d: %d hp and %d mp table rows, want 80 or more", id, len(c.HPTable), len(c.MPTable))
		}
	}
	if r := dp.Classes[116].Race; r != 3 {
		t.Errorf("Doom Cryer race = %d, want 3 (Orc)", r)
	}
	knight := dp.Classes[19]
	for _, g := range []SkillGrant{{ID: 77, Level: 1, MinLevel: 10, Cost: 910}, {ID: 77, Level: 2, MinLevel: 28}} {
		i := slices.IndexFunc(knight.Skills, func(s SkillGrant) bool {
			return s.ID == g.ID && s.Level == g.Level && s.MinLevel == g.MinLevel
		})
		if i < 0 {
			t.Errorf("Elven Knight tree lacks Attack Aura level %d from %d", g.Level, g.MinLevel)
		}
	}

	focused := dp.Skills[SkillRef{50, 1}]
	if focused.MaxCharges != 1 || focused.NumCharges != 1 || focused.Charges != (ChargeCond{Min: 1, Not: true}) {
		t.Errorf("Focused Force 1 = %+v, want maxCharges 1 refusing at 1 charge", focused)
	}
	if got := dp.Skills[SkillRef{50, 7}].MaxCharges; got != 7 {
		t.Errorf("Focused Force 7 maxCharges = %d, want 7", got)
	}
	hurricane := dp.Skills[SkillRef{284, 1}]
	if hurricane.Charges != (ChargeCond{Min: 2}) || hurricane.NumCharges != 2 || hurricane.Type != "CHARGEDAM" ||
		!slices.Equal(hurricane.WeaponsAllowed, []string{"DUALFIST"}) || hurricane.ReuseDelay != 17*time.Second {
		t.Errorf("Hurricane Assault 1 = %+v, want a dual-fist CHARGEDAM needing 2 charges", hurricane)
	}
	accuracy := dp.Skills[SkillRef{256, 1}]
	drain := Effect{Name: "ManaDamOverTime", StackType: "none", Time: 5, Count: 0x7fffffff, Value: 4.8}
	if accuracy.OperateType != "TOGGLE" || !slices.Equal(accuracy.Effects, []Effect{drain}) {
		t.Errorf("Accuracy = %+v, want a toggle draining 4.8 MP every 5 s", accuracy)
	}
	spellcraft := StatFunc{Op: "mul", Stat: "mAtkSpd", Value: 0.5, Using: []string{"MAGIC"}, NotUsing: true}
	if got := dp.Skills[SkillRef{163, 1}].Funcs; !reflect.DeepEqual(got, []StatFunc{spellcraft}) {
		t.Errorf("Spellcraft funcs = %+v, want %+v", got, spellcraft)
	}
	armor := StatFunc{Op: "add", Stat: "rEvas", Value: 3, Using: []string{"LIGHT"}}
	if got := dp.Skills[SkillRef{142, 5}].Funcs; !slices.ContainsFunc(got, func(f StatFunc) bool { return reflect.DeepEqual(f, armor) }) {
		t.Errorf("Armor Mastery 5 funcs = %+v, want one %+v", got, armor)
	}

	for _, set := range dp.ArmorSets {
		if len(dp.Skills[SkillRef{set.SkillID, 1}].Funcs) == 0 {
			t.Errorf("set %q skill %d has no stat functions", set.Name, set.SkillID)
		}
	}
	devotion := []StatFunc{{Op: "mul", Stat: "mAtkSpd", Value: 1.15}}
	if got := dp.Skills[SkillRef{3501, 1}].Funcs; !reflect.DeepEqual(got, devotion) {
		t.Errorf("Devotion Set funcs = %+v, want %+v", got, devotion)
	}

	if got := dp.NPCs[30010].TeachTo; !slices.Equal(got, []int32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}) {
		t.Errorf("Auron teaches %v, want classes 0-9", got)
	}
	trainers := 0
	for _, n := range dp.NPCs {
		if len(n.TeachTo) > 0 {
			trainers++
		}
	}
	if trainers != 216 {
		t.Errorf("%d NPCs teach, want 216", trainers)
	}
}

// TestParentClass checks ParentClass reads the profession table: a
// profession's parent, -1 for a base class, not ok for an unknown id.
func TestParentClass(t *testing.T) {
	for _, tt := range []struct {
		id, want int32
		ok       bool
	}{{46, 45, true}, {45, 44, true}, {44, -1, true}, {118, 57, true}, {60, 0, false}} {
		if got, ok := ParentClass(tt.id); got != tt.want || ok != tt.ok {
			t.Errorf("ParentClass(%d) = %d, %v, want %d, %v", tt.id, got, ok, tt.want, tt.ok)
		}
	}
}
