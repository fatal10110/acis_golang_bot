package datapack

import "slices"

// EnchantGain is how much enchant level enchant adds to the stat named stat
// (as the files spell it: pAtk, mAtk, pDef, mDef, ...) of it; 0 when none of
// its <enchant> entries names the stat. Every <enchant> val in the files is
// 0: the amount is the server's formula (skill/effect/mod.go applyEnchant),
// copied here. Levels above +3 count as overenchant, worth more each:
//
//   - P. Def and M. Def, any item: +1 a level, +3 an overenchant level.
//   - M. Atk, by crystal grade: S +4/+8; A, B and C +3/+6; D +2/+4.
//   - Any other stat (P. Atk), weapons only, by grade and weapon type: S bow
//     +10/+20, two-handed or dual +6/+12, other +5/+10; A +8/+16, +5/+10,
//     +4/+8; B and C +6/+12, +4/+8, +3/+6; D bow +4/+8, other +2/+4.
//
// No grade gains nothing but defense.
func (it Item) EnchantGain(stat string, enchant int) int {
	if enchant <= 0 || !slices.Contains(it.Enchants, stat) {
		return 0
	}
	over := 0
	if enchant > 3 {
		over, enchant = enchant-3, 3
	}
	if stat == "mDef" || stat == "pDef" {
		return enchant + 3*over
	}
	gain := func(per int) int { return per*enchant + 2*per*over }
	if stat == "mAtk" {
		switch it.Grade {
		case gradeS:
			return gain(4)
		case gradeA, gradeB, gradeC:
			return gain(3)
		case gradeD:
			return gain(2)
		}
		return 0
	}
	if it.Type != "Weapon" {
		return 0
	}
	bow := it.Kind == "BOW"
	bigOrDual := slices.Contains([]string{"BIGBLUNT", "BIGSWORD", "DUALFIST", "DUAL"}, it.Kind)
	switch it.Grade {
	case gradeS:
		switch {
		case bow:
			return gain(10)
		case bigOrDual:
			return gain(6)
		}
		return gain(5)
	case gradeA:
		switch {
		case bow:
			return gain(8)
		case bigOrDual:
			return gain(5)
		}
		return gain(4)
	case gradeB, gradeC:
		switch {
		case bow:
			return gain(6)
		case bigOrDual:
			return gain(4)
		}
		return gain(3)
	case gradeD:
		if bow {
			return gain(4)
		}
		return gain(2)
	}
	return 0
}

// Item.Grade values (grades).
const (
	gradeD = 1 + iota
	gradeC
	gradeB
	gradeA
	gradeS
)
