package datapack

import "testing"

// TestEnchantGain checks the copy of the server's applyEnchant
// (skill/effect/mod.go) case by case.
func TestEnchantGain(t *testing.T) {
	weapon := func(kind string, grade int) Item {
		return Item{Type: "Weapon", Kind: kind, Grade: grade, Enchants: []string{"pAtk", "mAtk"}}
	}
	armor := func(grade int) Item {
		return Item{Type: "Armor", Kind: "HEAVY", Grade: grade, Enchants: []string{"pDef", "mDef", "sDef"}}
	}
	tests := []struct {
		name    string
		item    Item
		stat    string
		enchant int
		want    int
	}{
		{"not enchanted", weapon("SWORD", gradeS), "pAtk", 0, 0},
		{"negative", weapon("SWORD", gradeS), "pAtk", -1, 0},
		{"stat no entry names", Item{Type: "Weapon", Kind: "SWORD", Grade: gradeS, Enchants: []string{"mAtk"}}, "pAtk", 3, 0},
		{"S bow +3", weapon("BOW", gradeS), "pAtk", 3, 30},
		{"S bow +6", weapon("BOW", gradeS), "pAtk", 6, 90},
		{"S two-handed sword +4", weapon("BIGSWORD", gradeS), "pAtk", 4, 30},
		{"S dual +4", weapon("DUAL", gradeS), "pAtk", 4, 30},
		{"S sword +5", weapon("SWORD", gradeS), "pAtk", 5, 35},
		{"A bow +4", weapon("BOW", gradeA), "pAtk", 4, 40},
		{"A two-handed blunt +4", weapon("BIGBLUNT", gradeA), "pAtk", 4, 25},
		{"A dagger +4", weapon("DAGGER", gradeA), "pAtk", 4, 20},
		{"B bow +2", weapon("BOW", gradeB), "pAtk", 2, 12},
		{"B fist +4", weapon("DUALFIST", gradeB), "pAtk", 4, 20},
		{"C blunt +4", weapon("BLUNT", gradeC), "pAtk", 4, 15},
		{"D bow +4", weapon("BOW", gradeD), "pAtk", 4, 20},
		{"D two-handed sword +4", weapon("BIGSWORD", gradeD), "pAtk", 4, 10},
		{"no grade weapon", weapon("SWORD", 0), "pAtk", 5, 0},
		{"S mAtk +4", weapon("SWORD", gradeS), "mAtk", 4, 20},
		{"A mAtk +4", weapon("BOW", gradeA), "mAtk", 4, 15},
		{"C mAtk +3", weapon("BLUNT", gradeC), "mAtk", 3, 9},
		{"D mAtk +4", weapon("BLUNT", gradeD), "mAtk", 4, 10},
		{"no grade mAtk", weapon("BLUNT", 0), "mAtk", 4, 0},
		{"pDef +3", armor(gradeD), "pDef", 3, 3},
		{"pDef +5", armor(gradeA), "pDef", 5, 9},
		{"no grade mDef +4", armor(0), "mDef", 4, 6},
		{"shield sDef", armor(gradeB), "sDef", 5, 0},
		{"pAtk on armor", Item{Type: "Armor", Grade: gradeS, Enchants: []string{"pAtk"}}, "pAtk", 4, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.EnchantGain(tt.stat, tt.enchant); got != tt.want {
				t.Errorf("EnchantGain(%s, %d) = %d, want %d", tt.stat, tt.enchant, got, tt.want)
			}
		})
	}
}
