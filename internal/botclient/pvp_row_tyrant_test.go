package botclient

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// checkTyrantPvPRow checks the tyrant row TY of the row table at 58, where
// the M10 Zealot comes in (a self step, outside the table) and Force Storm
// and Force Burst play (docs/pvp/rows-second.md, X26); checkSecondPvPRows
// holds 40, 55 and 74.
func checkTyrantPvPRow(t *testing.T, classes map[int32]datapack.Class, skills map[datapack.SkillRef]datapack.Skill) {
	t.Helper()
	const want = "54/CA 95/CA 281 120 35 17 284 280 54 29"
	m := &TravelMap{classes: classes, skills: skills}
	got := ""
	for r, refs := range pvpRowTable(m, shippedKits, 58) {
		if r.ID == "TY" {
			got = refWords(t, skills, refs)
		}
	}
	if got != want {
		t.Errorf("level 58 row TY: steps %q, want %q", got, want)
	}
}

// TestPvPRowTYKit checks the tyrant's kit (docs/pvp/rows-second.md, X26):
// it sets its own list, every skill of which it learns, and the monk's
// list stays as it is for MO.
func TestPvPRowTYKit(t *testing.T) {
	want := []int32{
		skillForceBlaster, skillCripple, skillSoulBreaker, skillStunningFist, skillCripple,
		skillForceStorm, skillForceBurst, skillHurricaneAssault, skillBurningFist, skillForceBlaster,
	}
	if got := stepSkills(tyrantKit.PvP); !slices.Equal(got, want) {
		t.Errorf("TY kit PvP %v, want %v", got, want)
	}
	for _, st := range tyrantKit.PvP {
		if !slices.Contains(tyrantKit.Skills, st.Skill) {
			t.Errorf("PvP step skill %d not in Skills %v", st.Skill, tyrantKit.Skills)
		}
	}
	mo := []int32{skillForceBlaster, skillCripple, skillStunningFist, skillHurricaneAssault, skillForceBlaster}
	if got := stepSkills(monkPvP); !slices.Equal(got, mo) {
		t.Errorf("MO list %v, want %v unchanged", got, mo)
	}
	if got := stepSkills(professionKits[classMonk].PvP); !slices.Equal(got, mo) {
		t.Errorf("MO kit PvP %v, want monkPvP %v", got, mo)
	}
}

// TestPvPRowTY checks the tyrant's PvP row at 40: Cripple on a caster and
// an archer, not on a Warrior, then Force Blaster opens with a charge;
// Soul Breaker at reuse on a Warrior, then Stunning Fist; both held on a
// Cleric until its Heal, Soul Breaker first; the M10 attacks (Hurricane
// Assault with two charges, Burning Fist, Force Blaster with one, Iron
// Punch) in M10 order with no list damage step ahead; Cripple to break
// off.
func TestPvPRowTY(t *testing.T) {
	all := []int32{
		skillForceBlaster, skillCripple, skillSoulBreaker, skillStunningFist,
		skillHurricaneAssault, skillBurningFist, skillIronPunch,
	}
	charges := func(n int32) func(*testing.T, *Brain) {
		return func(_ *testing.T, b *Brain) { b.world.Self.Charges = n }
	}
	stuns := cooling(skillSoulBreaker, skillStunningFist)
	healer := func(t *testing.T, b *Brain) { foeOf(t, b, lyrCleric, Point{Y: 60}, 0) }
	playRow(t, classTyrant, []rowCase{
		{name: "a Human Mystic: Cripple", level: 40, weapon: "DUALFIST", skills: all, opp: lyrMystic, setup: charges(1), want: skillCripple},
		{
			name: "a Human Mystic, Cripple cooling, a charge: Force Blaster opens", level: 40, weapon: "DUALFIST", skills: all, opp: lyrMystic,
			setup: both(charges(1), cooling(skillCripple)), want: skillForceBlaster,
		},
		{name: "an archer: Cripple", level: 40, weapon: "DUALFIST", skills: all, opp: lyrWarrior, setup: both(charges(1), func(_ *testing.T, b *Brain) { asArcher(b) }), want: skillCripple},
		{
			name: "a Warrior: no Cripple, no Force Blaster opener, Soul Breaker at reuse", level: 40, weapon: "DUALFIST", skills: all, opp: lyrWarrior,
			setup: charges(1), want: skillSoulBreaker, not: []int32{skillCripple, skillForceBlaster},
		},
		{name: "a Warrior, Soul Breaker cooling: Stunning Fist at reuse", level: 40, weapon: "DUALFIST", skills: all, opp: lyrWarrior, setup: both(charges(1), cooling(skillSoulBreaker)), want: skillStunningFist},
		{
			name: "a Cleric: Soul Breaker and Stunning Fist held, Burning Fist", level: 40, weapon: "DUALFIST", skills: all, opp: lyrCleric,
			setup: both(opened, charges(1)), want: skillBurningFist,
		},
		{name: "the Cleric's Heal: Soul Breaker", level: 40, weapon: "DUALFIST", skills: all, opp: lyrCleric, heal: true, setup: both(opened, charges(1), healer), want: skillSoulBreaker},
		{
			name: "the Cleric's Heal, Soul Breaker cooling: Stunning Fist", level: 40, weapon: "DUALFIST", skills: all, opp: lyrCleric, heal: true,
			setup: both(opened, charges(1), healer, cooling(skillSoulBreaker)), want: skillStunningFist,
		},
		{
			name: "a Warrior, stuns cooling, two charges: Hurricane Assault", level: 40, weapon: "DUALFIST", skills: all, opp: lyrWarrior,
			setup: both(opened, charges(2), stuns), want: skillHurricaneAssault,
		},
		{
			name: "a Warrior, stuns cooling, one charge: Burning Fist", level: 40, weapon: "DUALFIST", skills: all, opp: lyrWarrior,
			setup: both(opened, charges(1), stuns), want: skillBurningFist,
		},
		{
			name: "a Warrior, stuns and Burning Fist cooling, one charge: Force Blaster", level: 40, weapon: "DUALFIST", skills: all, opp: lyrWarrior,
			setup: both(opened, charges(1), stuns, cooling(skillBurningFist)), want: skillForceBlaster,
		},
		{
			name: "a Warrior, stuns and Burning Fist cooling, no charge: Iron Punch", level: 40, weapon: "DUALFIST", skills: all, opp: lyrWarrior,
			setup: both(opened, stuns, cooling(skillBurningFist)), want: skillIronPunch,
		},
		{name: "breaking off: Cripple", level: 40, weapon: "DUALFIST", skills: all, opp: lyrWarrior, setup: both(opened, charges(1), breakingOff), want: skillCripple},
	})
}
