package botclient

import (
	"maps"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestGearRoleSkipsHeals checks that a Damage build's model attack is its
// first known rotation step that deals damage: a human mystic's Self Heal
// (HEAL, first in its rotation) is passed over for Wind Strike (#369).
func TestGearRoleSkipsHeals(t *testing.T) {
	skills := []KnownSkill{{ID: skillSelfHeal, Level: 1}, {ID: skillWindStrike, Level: 1}}
	b := newModelBrain(t, classHumanMystic, 15, RoleDamage, skills, nil)
	m := b.cfg.Travel
	m.skills = maps.Clone(m.skills)
	m.skills[datapack.SkillRef{ID: skillSelfHeal, Level: 1}] = datapack.Skill{Type: "HEAL", OperateType: "ACTIVE", Power: 42, HitTime: 5 * time.Second, ReuseDelay: 10 * time.Second, MPConsume: 7}
	role, tank := b.gearRole()
	want := charmodel.Attack{Magic: true, Power: 12, HitTime: 4 * time.Second, Reuse: 6 * time.Second, MPCost: 10}
	if tank || role.Kind != charmodel.Damage || role.Attack != want {
		t.Errorf("gearRole = %+v, tank %v; want the Wind Strike attack %+v", role, tank, want)
	}
}

// shippedTravel is the travel map of the shipped datapack that
// BOT_DATAPACK_XML names, loaded once for the row tests.
var shippedTravel = sync.OnceValues(func() (*TravelMap, error) {
	dp, err := datapack.Load(os.Getenv("BOT_DATAPACK_XML"))
	if err != nil {
		return nil, err
	}
	return NewTravelMap(dp), nil
})

// shippedSetBonus is setBonus, against the shipped datapack, of the set
// with chest worn whole by a bot of class at level in role that knows
// every skill of its tree up to that level, naked otherwise and with no
// stats reported (no calibration). Its target is the level's reference
// monster, with no P. Atk and M. Atk when harmless, so the set's value to
// the role shows apart from survival. It skips without BOT_DATAPACK_XML.
func shippedSetBonus(t *testing.T, class, level int32, role Role, chest int32, harmless bool) float64 {
	t.Helper()
	if os.Getenv("BOT_DATAPACK_XML") == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	shipped, err := shippedTravel()
	if err != nil {
		t.Fatal(err)
	}
	m := *shipped
	target, ok := m.refMob(int(level))
	if !ok {
		t.Fatalf("no reference monster at level %d", level)
	}
	if harmless {
		target.PAtk, target.MAtk = 0, 0
	}
	m.refMobs = map[int]charmodel.Profile{int(level): target}
	b := newGearBrain(t, 0)
	b.persona.Gear.Classes = nil
	b.persona.Role = role
	s := &b.world.Self
	s.ClassID, s.Level = class, level
	s.MaxHP, s.MaxMP, s.PAtk, s.MAtk, s.PAtkSpd, s.CastSpd, s.Critical, s.Accuracy, s.Evasion, s.PDef, s.MDef, s.RunSpeed, s.MaxLoad = 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
	tree := make(map[int32]int)
	for _, g := range m.classes[class].Skills {
		if g.MinLevel <= int(level) && g.Level > tree[g.ID] {
			tree[g.ID] = g.Level
		}
	}
	var known []KnownSkill
	for id, lvl := range tree {
		sk := m.skills[datapack.SkillRef{ID: id, Level: int32(lvl)}]
		known = append(known, KnownSkill{ID: id, Level: int32(lvl), Passive: sk.OperateType == "PASSIVE"})
	}
	b.world.Apply(SkillList{Skills: known}, t0)
	b.cfg.Travel = &m
	set, ok := m.sets[chest]
	if !ok {
		t.Fatalf("set of chest %d not indexed", chest)
	}
	var w worn
	for _, slot := range setSlots {
		if id := set.pieces[slot]; id != 0 {
			w[slot] = Item{ObjectID: 400 + int32(slot), TemplateID: id, Count: 1, Equipped: true}
		}
	}
	return b.setBonus(&w)
}

// statTieBreakOnly is the largest set bonus the StatWeights tie-break
// alone gives a row's set (statTieBreak): a bonus above it is the model's.
const statTieBreakOnly = 1e-3

// setGain is one row's set in one role: whether the model values it
// (gains) or the row records it as model-chosen in docs/strategy.md
// (only the tie-break).
type setGain struct {
	role     Role
	chest    int32
	harmless bool
	gains    bool
}

// checkRowSets checks each of the row's sets against the shipped
// datapack (#369).
func checkRowSets(t *testing.T, class, level int32, sets ...setGain) {
	t.Helper()
	for _, g := range sets {
		got := shippedSetBonus(t, class, level, g.role, g.chest, g.harmless)
		if gains := got > statTieBreakOnly; gains != g.gains {
			t.Errorf("class %d level %d %v: set %d bonus %v, model gain %v, want %v", class, level, g.role, g.chest, got, gains, g.gains)
		}
	}
}

// TestRowHMSetValue: the human mystic's Devotion Set (cast speed) gains in
// both roles: its Damage attack is Wind Strike, not Self Heal, and its
// Support heals faster. Against the level's reference monster the naked
// support dies inside the survival floor, so the set is model-chosen there
// (docs/strategy.md).
func TestRowHMSetValue(t *testing.T) {
	checkRowSets(t, classHumanMystic, 15, setGain{RoleDamage, setDevotion, true, true}, setGain{RoleSupport, setDevotion, true, true}, setGain{RoleSupport, setDevotion, false, false})
}

// TestRowEMSetValue: as HM, for the elven mystic.
func TestRowEMSetValue(t *testing.T) {
	checkRowSets(t, 25, 15, setGain{RoleDamage, setDevotion, true, true}, setGain{RoleSupport, setDevotion, true, true}, setGain{RoleSupport, setDevotion, false, false})
}

// TestRowDMSetValue: as HM, for the dark mystic.
func TestRowDMSetValue(t *testing.T) {
	checkRowSets(t, classDarkMystic, 15, setGain{RoleDamage, setDevotion, true, true}, setGain{RoleSupport, setDevotion, true, true}, setGain{RoleSupport, setDevotion, false, false})
}

// TestRowOMSetValue: the orc mystic's Devotion Set gains in its Damage
// role, even against the real reference monster.
func TestRowOMSetValue(t *testing.T) {
	checkRowSets(t, classOrcMystic, 15, setGain{RoleDamage, setDevotion, false, true}, setGain{RoleDamage, setDevotion, true, true})
}

// TestRowCLSetValue: the cleric's Mithril Robe Set (+1 WIT, -1 INT)
// heals faster more than its lost M. Atk costs the heal. Against the
// level's reference monster the naked support dies inside the survival
// floor, so the set is model-chosen there (docs/strategy.md).
func TestRowCLSetValue(t *testing.T) {
	checkRowSets(t, 15, 30, setGain{RoleSupport, setMithril, true, true}, setGain{RoleSupport, setMithril, false, false})
}

// TestRowEOSetValue: as CL, for the elven oracle.
func TestRowEOSetValue(t *testing.T) {
	checkRowSets(t, 29, 30, setGain{RoleSupport, setMithril, true, true}, setGain{RoleSupport, setMithril, false, false})
}

// TestRowSOSetValue: as CL, for the shillien oracle.
func TestRowSOSetValue(t *testing.T) {
	checkRowSets(t, 42, 30, setGain{RoleSupport, setMithril, true, true}, setGain{RoleSupport, setMithril, false, false})
}

// TestRowHWSetValue: the human wizard's Knowledge Set (M. Atk) gains in
// its Damage role; its Support build knows no heal and values survival
// alone (a tank), so the set is model-chosen there (docs/strategy.md).
func TestRowHWSetValue(t *testing.T) {
	checkRowSets(t, 11, 30, setGain{RoleDamage, setKnowledge, true, true}, setGain{RoleSupport, setKnowledge, false, false})
}

// TestRowEWSetValue: as HW, for the elven wizard.
func TestRowEWSetValue(t *testing.T) {
	checkRowSets(t, 26, 30, setGain{RoleDamage, setKnowledge, true, true}, setGain{RoleSupport, setKnowledge, false, false})
}

// TestRowDWSetValue: as HW, for the dark wizard.
func TestRowDWSetValue(t *testing.T) {
	checkRowSets(t, 39, 30, setGain{RoleDamage, setKnowledge, true, true}, setGain{RoleSupport, setKnowledge, false, false})
}

// TestRowOSSetValue: the orc shaman's Knowledge robe gains in its Damage
// role; its Support heal, Chant of Life, heals over time (no skill power),
// so that build is a tank and the robe model-chosen there.
func TestRowOSSetValue(t *testing.T) {
	checkRowSets(t, classOrcShaman, 30, setGain{RoleDamage, setKnowledge, false, true}, setGain{RoleSupport, setKnowledge, false, false})
}

// rogueSets checks the light rows' Manticore Skin and Reinforced Leather
// sets (max MP): in the Damage role the model values the MP where the
// kill's MP binds the rest (a harmless target), and the sets are
// model-chosen where HP binds it or the fight does not pay off (the
// reference monster); the Support build knows no heal and is a tank, the
// sets model-chosen.
func rogueSets(t *testing.T, class int32) {
	t.Helper()
	var sets []setGain
	for _, chest := range []int32{setManticore, setReinforced} {
		sets = append(sets,
			setGain{RoleDamage, chest, true, true},
			setGain{RoleDamage, chest, false, false},
			setGain{RoleSupport, chest, false, false},
		)
	}
	checkRowSets(t, class, 30, sets...)
}

// TestRowROSetValue: rogueSets for the rogue.
func TestRowROSetValue(t *testing.T) { rogueSets(t, 7) }

// TestRowESSetValue: rogueSets for the elven scout.
func TestRowESSetValue(t *testing.T) { rogueSets(t, 22) }

// TestRowASSetValue: rogueSets for the assassin.
func TestRowASSetValue(t *testing.T) { rogueSets(t, 35) }

// TestRowMOSetValue: rogueSets for the monk.
func TestRowMOSetValue(t *testing.T) { rogueSets(t, 47) }
