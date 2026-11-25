package charmodel

import (
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

// Row is a behavior row (docs/strategy.md "Behavior per class"): ID is the
// row id of a base class or first profession (HF, WA, ...); Bow marks the
// bow branch of the Rogue, Elven Scout and Assassin rows, which fights as
// an archer.
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
}

// Archetype is the row's archetype: a scout row's bow branch is an Archer;
// an unknown row fights as Melee.
func (r Row) Archetype() Archetype {
	if r.Bow {
		return Archer
	}
	return rowArchetypes[r.ID]
}

// classRows are the row of every base class and first profession
// (docs/strategy.md "Behavior per class"); the Artisan (56) plays SC's.
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
}

// scoutRows are the rows a bow in the right hand makes archers.
var scoutRows = map[string]bool{"RO": true, "ES": true, "AS": true}

// ClassRow is the row class id plays: its first profession's (second and
// third professions play their first's), the bow branch for a scout line
// with a bow in the right hand (bow), SC's for the Artisan line (56, 57,
// 118). ok is false for an id the profession table does not hold.
func ClassRow(id int32, bow bool) (Row, bool) {
	for {
		if row, ok := classRows[id]; ok {
			return Row{ID: row, Bow: bow && scoutRows[row]}, true
		}
		parent, ok := datapack.ClassParent(id)
		if !ok || parent < 0 {
			return Row{}, false
		}
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
