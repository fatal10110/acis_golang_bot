package charmodel

import (
	"maps"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Archetype is how a player fights, what a PvP step's Vs gate names
// (docs/pvp/model.md "Archetypes", docs/pvp/layer.md "Step fields").
type Archetype int

const (
	Melee Archetype = iota
	Archer
	Caster
	Healer
	BattleCaster
)

// String is the archetype's letter in the rows' `vs` notation
// (docs/pvp/rows.md): M, A, C, H, B.
func (a Archetype) String() string {
	switch a {
	case Melee:
		return "M"
	case Archer:
		return "A"
	case Caster:
		return "C"
	case Healer:
		return "H"
	case BattleCaster:
		return "B"
	}
	return "?"
}

// Row is a behavior row (docs/strategy.md "Behavior per class",
// docs/pvp/rows-second.md): ID is the row id of a base class, first or
// second profession (HF, WA, GL, ...); Bow marks the bow branch of the
// Rogue, Elven Scout and Assassin rows, which fights as an archer.
type Row struct {
	ID  string
	Bow bool
}

// rowArchetypes are the archetype of every row id, the scouts' dagger
// branch for RO, ES and AS.
var rowArchetypes = map[string]Archetype{
	"HF": Melee, "EF": Melee, "DF": Melee, "OF": Melee, "WF": Melee,
	"WA": Melee, "KN": Melee, "RO": Melee, "EK": Melee, "ES": Melee,
	"PK": Melee, "AS": Melee, "OR": Melee, "MO": Melee, "SC": Melee,
	"HM": Caster, "EM": Caster, "DM": Caster, "HW": Caster, "EW": Caster, "DW": Caster,
	"CL": Healer, "EO": Healer, "SO": Healer,
	"OM": BattleCaster, "OS": BattleCaster,
	"PL": Melee, "TK": Melee, "SK": Melee, "SS": Melee, "BD": Melee,
	"GL": Melee, "WL": Melee, "DS": Melee, "BH": Melee,
	"TH": Melee, "PW": Melee, "AW": Melee, "TY": Melee,
	"HK": Archer, "SR": Archer, "PR": Archer,
	"SN": Caster, "SG": Caster, "SH": Caster,
	"BI": Healer, "PP": Healer, "EE": Healer, "SE": Healer,
	"WC": BattleCaster,
}

// Archetype is the row's archetype: a scout row's bow branch is an Archer;
// an unknown row fights as Melee.
func (r Row) Archetype() Archetype {
	if r.Bow {
		return Archer
	}
	return rowArchetypes[r.ID]
}

// Shoots reports whether the row fights with a bow: a scout row's bow
// branch or an archer row (HK, SR, PR).
func (r Row) Shoots() bool {
	return r.Archetype() == Archer
}

// classRows are the row of every base class, first profession and second
// profession with a row of its own (docs/strategy.md "Behavior per class",
// docs/pvp/rows-second.md); the Artisan (56) plays SC's. The servitor
// lines (Dark Avenger 6, Warlock 14, Necromancer 13, Elemental Summoner
// 28, Phantom Summoner 41), the Overlord (51) and the Warsmith (57) have
// none: they play their first profession's.
var classRows = map[int32]string{
	0: "HF", 1: "WA", 4: "KN", 7: "RO",
	10: "HM", 11: "HW", 15: "CL",
	18: "EF", 19: "EK", 22: "ES",
	25: "EM", 26: "EW", 29: "EO",
	31: "DF", 32: "PK", 35: "AS",
	38: "DM", 39: "DW", 42: "SO",
	44: "OF", 45: "OR", 47: "MO",
	49: "OM", 50: "OS",
	53: "WF", 54: "SC", 56: "SC",
	2: "GL", 3: "WL", 5: "PL", 8: "TH", 9: "HK",
	12: "SN", 16: "BI", 17: "PP",
	20: "TK", 21: "SS", 23: "PW", 24: "SR",
	27: "SG", 30: "EE",
	33: "SK", 34: "BD", 36: "AW", 37: "PR",
	40: "SH", 43: "SE",
	46: "DS", 48: "TY",
	52: "WC",
	55: "BH",
}

// scoutRows are the rows a bow in the right hand makes archers.
var scoutRows = map[string]bool{"RO": true, "ES": true, "AS": true}

// bowPairs are the same race's dagger and archer second rows, each to the
// other: a bow in the right hand plays the archer row, any other weapon
// the dagger row.
var bowPairs = map[string]string{
	"TH": "HK", "HK": "TH",
	"PW": "SR", "SR": "PW",
	"AW": "PR", "PR": "AW",
}

// ClassRow is the row class id plays: its own (classRows), else the
// nearest ancestor's (a servitor line, the Overlord and the Warsmith play
// their first profession's, a third profession its second's); with a bow
// in the right hand (bow) a scout row's bow branch, and the archer row of
// a dagger second row (TH to HK, PW to SR, AW to PR); without one the
// dagger row of an archer second row (HK to TH, SR to PW, PR to AW). ok
// is false for an id the profession table does not hold.
func ClassRow(id int32, bow bool) (Row, bool) {
	for {
		if row, ok := classRows[id]; ok {
			if pair, ok := bowPairs[row]; ok && bow != (rowArchetypes[row] == Archer) {
				row = pair
			}
			return Row{ID: row, Bow: bow && scoutRows[row]}, true
		}
		parent, ok := datapack.ClassParent(id)
		if !ok || parent < 0 {
			return Row{}, false
		}
		id = parent
	}
}

// OwnRow is the row class id plays as its own, whatever it wields: a scout
// row's dagger branch, an archer second row as itself. ok is false for a
// class that plays an ancestor's row (a servitor line, the Overlord, the
// Warsmith, a third profession) or that the table does not hold.
func OwnRow(id int32) (Row, bool) {
	row, ok := classRows[id]
	return Row{ID: row}, ok
}

// RowClasses are one class id per row a fighter of profession tier tier
// meets when nobody has been seen (docs/pvp/model.md "Row table", the mix):
// the base classes' rows for tier 0, the first professions' for tier 1,
// the second professions' for tier 2 and up (a third profession meets the
// second rows), the lowest class id for a row two classes share
// (Scavenger and Artisan, SC), in class id order. Each is the class's own
// row (OwnRow): a scout row is its dagger branch, an archer row itself.
func RowClasses(tier int) []int32 {
	tier = min(tier, 2)
	var out []int32
	seen := make(map[string]bool)
	for _, id := range slices.Sorted(maps.Keys(classRows)) {
		if professionTier(id) != tier || seen[classRows[id]] {
			continue
		}
		seen[classRows[id]] = true
		out = append(out, id)
	}
	return out
}

// professionTier is the depth of class id's parent chain: 0 for a base
// class, 1 for a first profession, 2 for a second.
func professionTier(id int32) int {
	tier := 0
	for {
		parent, ok := datapack.ClassParent(id)
		if !ok || parent < 0 {
			return tier
		}
		tier++
		id = parent
	}
}

// RefStep is one damage, DOT or control step a reference opponent plays
// (docs/pvp/model.md "Row table"): Skill at the skill level the row casts,
// from character level Level on, only against the archetypes in Vs (any
// when empty).
type RefStep struct {
	Skill datapack.Skill
	Level int
	Vs    []Archetype
}

// RowTable is the damage, DOT and control steps of each row; a row it
// lacks plays plain attacks only.
type RowTable map[Row][]RefStep

// steps are the steps of f's row it plays at its level against an opponent
// of archetype vs.
func (t RowTable) steps(f Fighter, vs Archetype) []RefStep {
	var out []RefStep
	for _, s := range t[f.Row] {
		if s.Level <= f.Level && gated(s.Vs, vs) {
			out = append(out, s)
		}
	}
	return out
}

// gated reports whether a step of gate vs is cast against archetype a.
func gated(vs []Archetype, a Archetype) bool {
	return len(vs) == 0 || slices.Contains(vs, a)
}
