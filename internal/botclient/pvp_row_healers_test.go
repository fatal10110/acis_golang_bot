package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The healer second-profession PvP rows (docs/pvp/rows-second.md "Tests",
// X28): BI and EE play their own lists on the mystic rows' harness
// (mysticRowBrain); PP keeps the Cleric's and SE the Shillien Oracle's.

// poisoned is the CharInfo abnormal bit of a poison (dotMask).
const poisoned = int32(0x2)

// checkHealerPvPRows checks the healer rows BI, PP, EE and SE of the row
// table at 56, where Trance comes in (an Int and a Break step, outside
// the table: docs/pvp/rows-second.md, X28); checkSecondPvPRows holds 40,
// 55 and 74.
func checkHealerPvPRows(t *testing.T, classes map[int32]datapack.Class, skills map[datapack.SkillRef]datapack.Skill) {
	t.Helper()
	want := map[string]string{
		"BI": "1201/M 1184/M 1177",
		"PP": "1201/M 1184/M 1177",
		"EE": "1206/M 1201/M 1184/M 1177",
		"SE": "1147 1206/M 1201/M 1168/CH 1184/M 1177",
	}
	m := &TravelMap{classes: classes, skills: skills}
	got := map[string]string{}
	for r, refs := range pvpRowTable(m, shippedKits, 56) {
		if _, ok := want[r.ID]; ok {
			got[r.ID] = refWords(t, skills, refs)
		}
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("level 56 row %s: steps %q, want %q", id, got[id], w)
		}
	}
}

// TestPvPRowHealerKits checks the healers' kits (docs/pvp/rows-second.md,
// X28, O3): BI and EE set their own lists, every skill of which they
// learn; PP plays the Cleric's list and SE the Shillien Oracle's; the CL,
// EO and SO lists stay as they are for their first professions.
func TestPvPRowHealerKits(t *testing.T) {
	cl := []int32{skillMentalShield, skillDryadRoot, skillIceBolt, skillWindStrike, skillDryadRoot}
	eo := []int32{skillMentalShield, skillWindShackle, skillDryadRoot, skillIceBolt, skillWindStrike, skillDryadRoot}
	so := []int32{skillMentalShield, skillVampiricTouch, skillWindShackle, skillDryadRoot, skillCursePoison, skillIceBolt, skillWindStrike, skillDryadRoot}
	for _, tc := range []struct {
		name  string
		class int32
		want  []int32
	}{
		{"BI", classBishop, []int32{skillMentalShield, skillDryadRoot, skillIceBolt, skillWindStrike, skillTrance, skillDryadRoot, skillTrance}},
		{"EE", classElvenElder, []int32{skillMentalShield, skillWindShackle, skillDryadRoot, skillIceBolt, skillWindStrike, skillTrance, skillDryadRoot, skillTrance}},
		{"PP", classProphet, cl},
		{"SE", classShillienElder, so},
		{"CL", classCleric, cl},
		{"EO", classElvenOracle, eo},
		{"SO", classShillienOracle, so},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kit := professionKits[tc.class]
			if got := stepSkills(kit.PvP); !slices.Equal(got, tc.want) {
				t.Errorf("kit PvP %v, want %v", got, tc.want)
			}
			for _, st := range kit.PvP {
				if !slices.Contains(kit.Skills, st.Skill) {
					t.Errorf("PvP step skill %d not in Skills %v", st.Skill, kit.Skills)
				}
			}
		})
	}
	for _, tc := range []struct {
		name string
		got  []Step
		want []int32
	}{
		{"CL", clericPvP, cl},
		{"EO", elvenOraclePvP, eo},
		{"SO", shillienOraclePvP, so},
	} {
		if got := stepSkills(tc.got); !slices.Equal(got, tc.want) {
			t.Errorf("%s list %v, want %v unchanged", tc.name, got, tc.want)
		}
	}
}

// TestPvPRowBI checks the bishop: Dryad Root opens on melee, Ice Bolt
// (kite) while it is on reuse; Mental Shield on itself against a caster, then Wind Strike;
// Trance from 56 answers a Cleric's Heal (5 s), none at 55; Break: Dryad
// Root on a melee opponent, Trance on a caster carrying no DOT, the flee
// from a poisoned one.
func TestPvPRowBI(t *testing.T) {
	shielded := func(t *testing.T, b *Brain) {
		b.handleOne(t, AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillMentalShield, Level: 1, Left: 1200 * time.Second}}}, at(time.Second))
	}
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs melee: Dryad Root", class: classBishop, level: 56, opp: lyrWarrior, skill: skillDryadRoot},
		{name: "vs melee, Dryad Root on reuse: Ice Bolt (kite)", class: classBishop, level: 56, opp: lyrWarrior, setup: reusing(skillDryadRoot), skill: skillIceBolt},
		{name: "vs a caster: Mental Shield", class: classBishop, level: 56, opp: lyrMystic, skill: skillMentalShield, onSelf: true},
		{name: "vs a caster, shielded: Wind Strike, no Trance", class: classBishop, level: 56, opp: lyrMystic, setup: shielded, skill: skillWindStrike},
		{name: "@56 vs a Cleric's Heal (5 s): Trance", class: classBishop, level: 56, opp: lyrCleric, setup: both(opened, healCast(5*time.Second)), skill: skillTrance},
		{name: "@55 vs a Cleric's Heal (5 s): no Trance, Wind Strike", class: classBishop, level: 55, opp: lyrCleric, setup: both(opened, healCast(5*time.Second)), skill: skillWindStrike},
		{name: "breaking off vs melee: Dryad Root", class: classBishop, level: 56, opp: lyrWarrior, setup: breaking, skill: skillDryadRoot},
		{name: "breaking off vs a caster, no DOT: Trance", class: classBishop, level: 56, opp: lyrMystic, setup: both(breaking, foeSeen(lyrMystic, Point{Y: 300}, 0)), skill: skillTrance},
		{
			name: "breaking off vs a poisoned caster: no Trance, the flee", class: classBishop, level: 56, opp: lyrMystic,
			setup: both(breaking, foeSeen(lyrMystic, Point{Y: 300}, poisoned)), move: true,
		},
		{name: "@55 breaking off vs a caster: the flee", class: classBishop, level: 55, opp: lyrMystic, setup: breaking, move: true},
	})
}

// TestPvPRowEE checks the elven elder: Wind Shackle, then Dryad Root on
// melee, then Ice Bolt (kite) while Dryad Root is on reuse; Trance from 56 answers a Cleric's Heal;
// Break: Dryad Root on a melee opponent, else Trance.
func TestPvPRowEE(t *testing.T) {
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs melee: Wind Shackle", class: classElvenElder, level: 56, opp: lyrWarrior, skill: skillWindShackle},
		{name: "vs melee, shackled: Dryad Root", class: classElvenElder, level: 56, opp: lyrWarrior, setup: landedOn(skillWindShackle), skill: skillDryadRoot},
		{
			name: "vs melee, shackled, Dryad Root on reuse: Ice Bolt (kite)", class: classElvenElder, level: 56, opp: lyrWarrior,
			setup: both(landedOn(skillWindShackle), reusing(skillDryadRoot)), skill: skillIceBolt,
		},
		{name: "@56 vs a Cleric's Heal (5 s): Trance", class: classElvenElder, level: 56, opp: lyrCleric, setup: both(opened, healCast(5*time.Second)), skill: skillTrance},
		{name: "@55 vs a Cleric's Heal (5 s): no Trance, Wind Strike", class: classElvenElder, level: 55, opp: lyrCleric, setup: both(opened, healCast(5*time.Second)), skill: skillWindStrike},
		{name: "breaking off vs melee: Dryad Root", class: classElvenElder, level: 56, opp: lyrWarrior, setup: breaking, skill: skillDryadRoot},
		{name: "breaking off vs a caster, no DOT: Trance", class: classElvenElder, level: 56, opp: lyrMystic, setup: breaking, skill: skillTrance},
	})
}
