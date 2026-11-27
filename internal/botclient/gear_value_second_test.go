package botclient

import "testing"

// The second-profession rows' set verdicts (#626), at 40 against the
// shipped datapack, each set in each role against a harmless target and
// against the level's reference monster. They are computed on the kits
// shipped with #626: until a row of its own lands, a second profession
// compiles its first profession's kit (professionKit), so gearRole reads
// that kit's heals and first damage step. A row PR re-runs its classes'
// tests and updates the verdicts it flips, with the model-chosen list in
// docs/strategy.md.

// setVerdict is set chest in role, model gain against a harmless target
// and against the reference monster.
func setVerdict(role Role, chest int32, harmless, real bool) []setGain {
	return []setGain{{role, chest, true, harmless}, {role, chest, false, real}}
}

// bothRoles is setVerdict in the Damage and the Support role alike.
func bothRoles(chest int32, harmless, real bool) []setGain {
	return append(setVerdict(RoleDamage, chest, harmless, real), setVerdict(RoleSupport, chest, harmless, real)...)
}

// checkSecondSets is checkRowSets at 40 over the verdict groups.
func checkSecondSets(t *testing.T, class int32, groups ...[]setGain) {
	t.Helper()
	var sets []setGain
	for _, g := range groups {
		sets = append(sets, g...)
	}
	checkRowSets(t, class, 40, sets...)
}

// Full Plate (+270 max HP) gains in both roles against the reference
// monster, where HP buys survival; against a harmless target HP buys
// nothing and the set is model-chosen. Composite (+5795 weight limit) is
// model-chosen everywhere: no role's value reads the weight limit.
var (
	fullPlateVerdicts = bothRoles(setFullPlate, false, true)
	compositeVerdicts = bothRoles(setComposite, false, false)
)

// TestRowPLSetValue: Full Plate, else Composite (kit: its own, kits_tanks.go;
// its Support build stays a tank with Holy Blessing known, gearRole).
func TestRowPLSetValue(t *testing.T) {
	checkSecondSets(t, 5, fullPlateVerdicts, compositeVerdicts)
}

// TestRowTKSetValue: Full Plate (kit: its own, kits_tanks.go).
func TestRowTKSetValue(t *testing.T) { checkSecondSets(t, 20, fullPlateVerdicts) }

// TestRowSKSetValue: Full Plate (kit: its own, kits_tanks.go).
func TestRowSKSetValue(t *testing.T) { checkSecondSets(t, 33, fullPlateVerdicts) }

// TestRowSSSetValue: Full Plate (kit: EK's).
func TestRowSSSetValue(t *testing.T) { checkSecondSets(t, 21, fullPlateVerdicts) }

// TestRowGLSetValue: Full Plate (kit: its own, kits_heavy.go).
func TestRowGLSetValue(t *testing.T) { checkSecondSets(t, 2, fullPlateVerdicts) }

// TestRowWLSetValue: Full Plate (kit: its own, kits_heavy.go).
func TestRowWLSetValue(t *testing.T) { checkSecondSets(t, 3, fullPlateVerdicts) }

// TestRowDSSetValue: Full Plate (kit: its own, kits_heavy.go).
func TestRowDSSetValue(t *testing.T) { checkSecondSets(t, 46, fullPlateVerdicts) }

// TestRowBDSetValue: Full Plate (kit: PK's).
func TestRowBDSetValue(t *testing.T) { checkSecondSets(t, 34, fullPlateVerdicts) }

// TestRowBHSetValue: Composite, else Full Plate (kit: its own, kits_heavy.go); the
// preferred Composite is model-chosen, so the bounty hunter buys Full
// Plate where the model values it.
func TestRowBHSetValue(t *testing.T) {
	checkSecondSets(t, 55, compositeVerdicts, fullPlateVerdicts)
}

// Plated Leather (+4 STR, -1 CON): against the reference monster the CON
// it takes costs more HP than the STR adds damage, in either role, and the
// set is model-chosen; against a harmless target the STR gains for the
// Damage builds of HK, SR and TY (strDamage; TY's with its own row, #633)
// and stays at the tie-break for TH, PW, AW and PR's (the daggers' first
// damage step at 40 is Deadly Blow, #631) and every Support build (a
// tank: no heal the model times), model-chosen. Mithril Light (+4 evasion) gains against the
// reference monster and is model-chosen against a harmless one.
func platedLeatherVerdicts(strDamage bool) [][]setGain {
	return [][]setGain{setVerdict(RoleDamage, setPlatedLeather, strDamage, false), setVerdict(RoleSupport, setPlatedLeather, false, false)}
}

var mithrilLightVerdicts = bothRoles(setMithrilLight, false, true)

// TestRowTHSetValue: Plated Leather, else Mithril Light (kit: TH's).
func TestRowTHSetValue(t *testing.T) {
	checkSecondSets(t, 8, append(platedLeatherVerdicts(false), mithrilLightVerdicts)...)
}

// TestRowHKSetValue: Plated Leather (kit: HK's).
func TestRowHKSetValue(t *testing.T) { checkSecondSets(t, 9, platedLeatherVerdicts(true)...) }

// TestRowPWSetValue: Plated Leather, else Mithril Light (kit: PW's).
func TestRowPWSetValue(t *testing.T) {
	checkSecondSets(t, 23, append(platedLeatherVerdicts(false), mithrilLightVerdicts)...)
}

// TestRowSRSetValue: Plated Leather (kit: SR's).
func TestRowSRSetValue(t *testing.T) { checkSecondSets(t, 24, platedLeatherVerdicts(true)...) }

// TestRowAWSetValue: Plated Leather, else Mithril Light (kit: AW's).
func TestRowAWSetValue(t *testing.T) {
	checkSecondSets(t, 36, append(platedLeatherVerdicts(false), mithrilLightVerdicts)...)
}

// TestRowPRSetValue: Plated Leather (kit: PR's).
func TestRowPRSetValue(t *testing.T) { checkSecondSets(t, 37, platedLeatherVerdicts(false)...) }

// TestRowTYSetValue: Plated Leather (kit: its own, kits_tyrant.go).
func TestRowTYSetValue(t *testing.T) { checkSecondSets(t, 48, platedLeatherVerdicts(true)...) }

// Divine (+171 MP, +1 WIT -1 INT, P. Def) and Karmian (x1.15 cast speed,
// P. Def) gain for a healer in both roles, against the reference monster
// too: at 40 the naked healer lasts its survival floor.
var healerVerdicts = append(bothRoles(setDivine, true, true), bothRoles(setKarmian, true, true)...)

// TestRowBISetValue: Divine, else Karmian (kit: CL's).
func TestRowBISetValue(t *testing.T) { checkSecondSets(t, 16, healerVerdicts) }

// TestRowPPSetValue: Karmian, else Divine (kit: CL's).
func TestRowPPSetValue(t *testing.T) { checkSecondSets(t, 17, healerVerdicts) }

// TestRowEESetValue: Divine, else Karmian (kit: EO's).
func TestRowEESetValue(t *testing.T) { checkSecondSets(t, 30, healerVerdicts) }

// TestRowSESetValue: Divine, else Karmian (kit: SO's).
func TestRowSESetValue(t *testing.T) { checkSecondSets(t, 43, healerVerdicts) }

// Karmian on a nuker: its cast speed gains in the Damage role against
// either target; the Support build knows no heal (a tank), so its P. Def
// gains against the reference monster and it is model-chosen against a
// harmless one. Demon (+4 INT, -1 WIT, -270 max HP) loses value against
// the reference monster in either role, and Karmian outranks it at every
// max HP (TestRowSNDemonRule): model-chosen.
var nukerKarmianVerdicts = append(setVerdict(RoleDamage, setKarmian, true, true), setVerdict(RoleSupport, setKarmian, false, true)...)

// TestRowSNSetValue: Demon, else Karmian (kit: SN's). Against a harmless
// target the sorcerer's Demon INT gains in the Damage role.
func TestRowSNSetValue(t *testing.T) {
	checkSecondSets(t, 12, setVerdict(RoleDamage, setDemon, true, false), setVerdict(RoleSupport, setDemon, false, false), nukerKarmianVerdicts)
}

// TestRowSGSetValue: Karmian, else Demon (kit: SG's). The spellsinger's
// Demon loses value against a harmless target too, its lost WIT costing
// more cast speed than the INT adds.
func TestRowSGSetValue(t *testing.T) {
	checkSecondSets(t, 27, nukerKarmianVerdicts, bothRoles(setDemon, false, false))
}

// TestRowSHSetValue: Karmian (kit: SH's).
func TestRowSHSetValue(t *testing.T) { checkSecondSets(t, 40, nukerKarmianVerdicts) }

// TestRowWCSetValue: Full Plate, else Composite, in the Support role (the
// orc mystic's heavy armor grant); the Karmian robe, the Damage role's
// set and the Support role's fallback (kit: WC's, whose Support build is
// a tank: Chant of Life heals over time with no skill power). Karmian
// gains against the reference monster in either role and stays at the
// tie-break against a harmless target, model-chosen there.
func TestRowWCSetValue(t *testing.T) {
	checkSecondSets(t, 52,
		setVerdict(RoleSupport, setFullPlate, false, true),
		setVerdict(RoleSupport, setComposite, false, false),
		setVerdict(RoleSupport, setKarmian, false, true),
		setVerdict(RoleDamage, setKarmian, false, true),
	)
}

// TestRowSNDemonRule checks the sorcerer's Demon rule through the model
// (docs/strategy.md row SN: Demon once max HP covers its -270, over 810,
// else Karmian), at 40 and 60 against the reference monster with max HP
// reported: at 810 or less the model picks Karmian, Demon's lost HP
// counted as lost survival. Over 810 it picks Karmian too: Karmian's cast
// speed is worth more than Demon's +4 INT -1 WIT at every max HP, so
// Demon is model-chosen (docs/strategy.md, "Model-chosen sets"; the
// rule's fate is #676).
func TestRowSNDemonRule(t *testing.T) {
	for _, level := range []int32{40, 60} {
		for _, hp := range []int32{500, 810, 811, 1500, 5000} {
			demon := shippedSetBonusHP(t, 12, level, RoleDamage, setDemon, false, hp)
			karmian := shippedSetBonusHP(t, 12, level, RoleDamage, setKarmian, false, hp)
			if demon >= karmian {
				t.Errorf("level %d max HP %d: Demon bonus %v, Karmian %v; want Karmian picked", level, hp, demon, karmian)
			}
			if hp <= 810 && demon > statTieBreakOnly {
				t.Errorf("level %d max HP %d: Demon bonus %v, want no model gain", level, hp, demon)
			}
		}
	}
}
