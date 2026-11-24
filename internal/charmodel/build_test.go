package charmodel

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The classes the tests build on, as classes/*.xml gives them; HP and MP
// tables carry only the level 20 entries (424 and 192 for the human
// mystic).
var (
	humanMystic = datapack.Class{
		ID: 10, Parent: -1, STR: 22, CON: 27, DEX: 21, INT: 41, WIT: 20, MEN: 39,
		PAtk: 3, PDef: 54, MAtk: 6, MDef: 41, RunSpeed: 120, HPTable: levelTable(20, 424), MPTable: levelTable(20, 192),
	}
	humanFighter = datapack.Class{
		ID: 0, Parent: -1, STR: 40, CON: 43, DEX: 30, INT: 21, WIT: 11, MEN: 25,
		PAtk: 4, PDef: 80, MAtk: 6, MDef: 41, RunSpeed: 115,
	}
	orcMystic = datapack.Class{
		ID: 49, Parent: -1, STR: 27, CON: 31, DEX: 24, INT: 31, WIT: 15, MEN: 42,
		PAtk: 3, PDef: 54, MAtk: 6, MDef: 41, RunSpeed: 121,
	}
)

func levelTable(level int, v float64) []float64 {
	t := make([]float64, level)
	t[level-1] = v
	return t
}

func armor(part, kind string, pDef float64) datapack.Item {
	return datapack.Item{Type: "Armor", BodyPart: part, Kind: kind, Stats: datapack.Stats{PDef: pDef}}
}

// The masteries as skills/*.xml writes them.
var (
	spellcraft       = datapack.StatFunc{Op: "mul", Stat: "mAtkSpd", Value: 0.5, Using: []string{"MAGIC"}, NotUsing: true}
	magicianMove     = datapack.StatFunc{Op: "mul", Stat: "pAtkSpd", Value: 0.8, Using: []string{"MAGIC"}, NotUsing: true}
	lightMastery     = datapack.StatFunc{Op: "mul", Stat: "mAtkSpd", Value: 1.9, Using: []string{"LIGHT"}}
	heavyMastery     = datapack.StatFunc{Op: "mul", Stat: "mAtkSpd", Value: 1.71, Using: []string{"HEAVY"}}
	armorMastery     = datapack.StatFunc{Op: "add", Stat: "rEvas", Value: 3, Using: []string{"LIGHT"}}
	weaponMastery    = datapack.StatFunc{Op: "mul", Stat: "mAtkSpd", Value: 0.7, Using: []string{"BOW", "POLE"}}
	nightOnlyEvasion = datapack.StatFunc{Op: "add", Stat: "rEvas", Value: 100, OtherCond: true}
)

// TestModelNaked checks a level 20 human mystic with nothing worn against
// values worked out from the server's functions: CON 27 0.98, MEN 39 1.48,
// STR 22 0.61, INT 41 1.21, DEX 21 1.01, WIT 20 1.0, level factor 1.09.
func TestModelNaked(t *testing.T) {
	got := Model(Build{Class: humanMystic, Level: 20, Mage: true})
	want := Profile{
		MaxHP: 415, MaxMP: 284, PAtk: 2, MAtk: 10, PAtkSpd: 303, MAtkSpd: 333, Critical: 40,
		Accuracy: 47, Evasion: 47, PDef: 58, MDef: 66, RunSpeed: 120 * 1.01, WeightLimit: 67620,
	}
	if got != want {
		t.Errorf("Model = %+v, want %+v", got, want)
	}
}

// TestArmorKind checks the masteries' <using kind> against what is worn: a
// chest counts its kind only with legs of the same kind, a full-body armor
// alone.
func TestArmorKind(t *testing.T) {
	robeTop, robeLegs := armor("chest", "MAGIC", 0), armor("legs", "MAGIC", 0)
	lightTop, lightLegs := armor("chest", "LIGHT", 0), armor("legs", "LIGHT", 0)
	heavyTop, heavyLegs := armor("chest", "HEAVY", 0), armor("legs", "HEAVY", 0)
	bow := datapack.Item{Type: "Weapon", BodyPart: "lrhand", Kind: "BOW"}
	mystic := []datapack.StatFunc{spellcraft, magicianMove}
	orc := []datapack.StatFunc{spellcraft, magicianMove, lightMastery, heavyMastery}
	tests := []struct {
		name             string
		class            datapack.Class
		funcs            []datapack.StatFunc
		worn             []datapack.Item
		mAtkSpd, pAtkSpd float64
	}{
		{"robe pair keeps MAGIC", humanMystic, mystic, []datapack.Item{robeTop, robeLegs}, 333, 303},
		{"robe top over light legs loses MAGIC", humanMystic, mystic, []datapack.Item{robeTop, lightLegs}, 166, 242},
		{"robe top alone", humanMystic, mystic, []datapack.Item{robeTop}, 166, 242},
		{"full-body robe counts alone", humanMystic, mystic, []datapack.Item{armor("fullarmor", "MAGIC", 0)}, 333, 303},
		{"naked", humanMystic, mystic, nil, 166, 242},
		{"bow under Weapon Mastery", humanMystic, []datapack.StatFunc{weaponMastery}, []datapack.Item{bow}, 233, 303},
		// WIT 15 0.78: 333*0.78*0.5*1.9 = 246.75, *1.71 = 222.08.
		{"orc mystic light pair", orcMystic, orc, []datapack.Item{lightTop, lightLegs}, 246, 0},
		{"orc mystic heavy pair", orcMystic, orc, []datapack.Item{heavyTop, heavyLegs}, 222, 0},
		{"orc mystic light top over heavy legs", orcMystic, orc, []datapack.Item{lightTop, heavyLegs}, 129, 0},
	}
	for _, tt := range tests {
		got := Model(Build{Class: tt.class, Level: 20, Mage: true, Worn: tt.worn, Funcs: tt.funcs})
		if got.MAtkSpd != tt.mAtkSpd {
			t.Errorf("%s: MAtkSpd = %v, want %v", tt.name, got.MAtkSpd, tt.mAtkSpd)
		}
		if tt.pAtkSpd != 0 && got.PAtkSpd != tt.pAtkSpd {
			t.Errorf("%s: PAtkSpd = %v, want %v", tt.name, got.PAtkSpd, tt.pAtkSpd)
		}
	}
}

// TestArmorMasteryEvasion checks that Armor Mastery's evasion counts only
// in light armor and that a function of a condition the model cannot judge
// counts never: a level 20 human fighter evades sqrt(30)*6+20 = 52.
func TestArmorMasteryEvasion(t *testing.T) {
	funcs := []datapack.StatFunc{armorMastery, nightOnlyEvasion}
	tests := []struct {
		name string
		worn []datapack.Item
		want float64
	}{
		{"light pair", []datapack.Item{armor("chest", "LIGHT", 0), armor("legs", "LIGHT", 0)}, 55},
		{"heavy pair", []datapack.Item{armor("chest", "HEAVY", 0), armor("legs", "HEAVY", 0)}, 52},
		{"light full-body", []datapack.Item{armor("fullarmor", "LIGHT", 0)}, 55},
	}
	for _, tt := range tests {
		if got := Model(Build{Class: humanFighter, Level: 20, Worn: tt.worn, Funcs: funcs}).Evasion; got != tt.want {
			t.Errorf("%s: Evasion = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestArmorPenalties checks the worn pieces' P.Def and M.Def penalties at
// level 1 (factor 0.9) over the human fighter's P.Def 80 and M.Def 41.
func TestArmorPenalties(t *testing.T) {
	set := []datapack.Item{
		armor("head", "", 10), armor("chest", "HEAVY", 20), armor("legs", "HEAVY", 12),
		armor("gloves", "", 4), armor("feet", "", 4),
	}
	tests := []struct {
		name string
		mage bool
		worn []datapack.Item
		want float64
	}{
		// (80+50-12-31-18-8-7)*0.9 = 48.6
		{"fighter five pieces", false, set, 48},
		// (80+30-31-18)*0.9 = 54.9
		{"fighter full-body", false, []datapack.Item{armor("fullarmor", "HEAVY", 30)}, 54},
		// (80+30-15-8)*0.9 = 78.3
		{"mage full-body", true, []datapack.Item{armor("fullarmor", "MAGIC", 30)}, 78},
		{"naked", false, nil, 72},
	}
	for _, tt := range tests {
		if got := Model(Build{Class: humanFighter, Level: 1, Mage: tt.mage, Worn: tt.worn}).PDef; got != tt.want {
			t.Errorf("%s: PDef = %v, want %v", tt.name, got, tt.want)
		}
	}

	jewel := func(part string, mDef float64) datapack.Item {
		return datapack.Item{Type: "Armor", BodyPart: part, Stats: datapack.Stats{MDef: mDef}}
	}
	worn := []datapack.Item{
		jewel("rear;lear", 10), jewel("rear;lear", 10), jewel("rfinger;lfinger", 5),
		jewel("rfinger;lfinger", 5), jewel("neck", 15),
	}
	// (41+45-9-9-5-5-13)*MEN 25 1.28*0.9 = 51.84
	if got := Model(Build{Class: humanFighter, Level: 1, Worn: worn}).MDef; got != 51 {
		t.Errorf("jewelry: MDef = %v, want 51", got)
	}
}

// TestWeapon checks that a weapon sets the base P.Atk, attack speed and
// critical rate: a level 1 human fighter with a 24 P.Atk, 379 speed, 8
// critical sword.
func TestWeapon(t *testing.T) {
	sword := datapack.Item{
		Type: "Weapon", BodyPart: "rhand", Kind: "SWORD",
		Stats: datapack.Stats{PAtk: 24, PAtkSpd: 379, RCrit: 8},
	}
	got := Model(Build{Class: humanFighter, Level: 1, Worn: []datapack.Item{sword}})
	// 24*1.2*0.9 = 25.92; 379*1.1 = 416.9; 8*1.1*10 = 88.
	if got.PAtk != 25 || got.PAtkSpd != 416 || got.Critical != 88 {
		t.Errorf("PAtk, PAtkSpd, Critical = %v %v %v, want 25 416 88", got.PAtk, got.PAtkSpd, got.Critical)
	}
}

// TestSetThenBaseMul checks that a base multiplier after a weapon's set
// scales the weapon's value, not the class default (calcChain sets base to
// the set value): a dagger's rCrit 12 with Critical Chance's basemul 0.4 is
// (12+12*0.4)*1.1*10 = 184.8 at DEX 30, not (12+4*0.4)*1.1*10 = 149.6.
func TestSetThenBaseMul(t *testing.T) {
	dagger := datapack.Item{
		Type: "Weapon", BodyPart: "rhand", Kind: "DAGGER",
		Stats: datapack.Stats{PAtk: 24, PAtkSpd: 433, RCrit: 12},
	}
	criticalChance := datapack.StatFunc{Op: "basemul", Stat: "rCrit", Value: 0.4}
	got := Model(Build{Class: humanFighter, Level: 1, Worn: []datapack.Item{dagger}, Funcs: []datapack.StatFunc{criticalChance}})
	if got.Critical != 184 {
		t.Errorf("Critical = %v, want 184", got.Critical)
	}
}

// TestSetTrade checks a set's stat trade: the Demon set (-270 max HP, +4
// INT, -1 WIT) with Doom Robe's +7 run speed on a level 20 human mystic:
// max HP 424*0.98-270 = 145.52, M.Atk 6*1.09^2*1.31^2 = 12.23 at INT 45,
// cast speed 333*0.95 = 316.35 at WIT 19.
func TestSetTrade(t *testing.T) {
	demon := []datapack.StatFunc{
		{Op: "sub", Stat: "maxHp", Value: 270},
		{Op: "add", Stat: "INT", Value: 4},
		{Op: "sub", Stat: "WIT", Value: 1},
		{Op: "add", Stat: "runSpd", Value: 7},
	}
	got := Model(Build{Class: humanMystic, Level: 20, Mage: true, Funcs: demon})
	if got.MaxHP != 145 || got.MAtk != 12 || got.MAtkSpd != 316 || got.RunSpeed != 120*1.01+7 {
		t.Errorf("MaxHP, MAtk, MAtkSpd, RunSpeed = %v %v %v %v, want 145 12 316 %v", got.MaxHP, got.MAtk, got.MAtkSpd, got.RunSpeed, 120*1.01+7)
	}
}

// TestDyeCap checks that dyes count at most +5 on a base stat.
func TestDyeCap(t *testing.T) {
	sword := datapack.Item{Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Stats: datapack.Stats{PAtk: 100}}
	pAtk := func(dye int) float64 {
		return Model(Build{Class: humanFighter, Level: 20, Worn: []datapack.Item{sword}, Dyes: Attrs{STR: dye}}).PAtk
	}
	if four, five, seven := pAtk(4), pAtk(5), pAtk(7); five == four || seven != five {
		t.Errorf("PAtk with STR dyes +4 +5 +7 = %v %v %v, want +7 like +5 and above +4", four, five, seven)
	}
}

func TestMageClass(t *testing.T) {
	classes := map[int32]datapack.Class{
		0: {ID: 0, Parent: -1}, 1: {ID: 1, Parent: 0},
		10: {ID: 10, Parent: -1}, 11: {ID: 11, Parent: 10}, 12: {ID: 12, Parent: 11},
		49: {ID: 49, Parent: -1}, 50: {ID: 50, Parent: 49},
	}
	for _, tt := range []struct {
		id   int32
		want bool
	}{{0, false}, {1, false}, {10, true}, {12, true}, {50, true}, {99, false}} {
		if got := MageClass(classes, tt.id); got != tt.want {
			t.Errorf("MageClass(%d) = %v, want %v", tt.id, got, tt.want)
		}
	}
}
