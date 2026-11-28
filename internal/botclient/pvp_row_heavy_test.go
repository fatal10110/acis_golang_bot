package botclient

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The heavy damage second-profession PvP rows (docs/pvp/rows-second.md
// "Tests", X23): each test plays the row's real compiled build on the
// rows' skill fixture (rowSkills) against a player.

// heavyRowTable is the GL, WL, DS and BH rows of the row table at the
// levels the issue names (#721): Sonic Blaster from 43, Wrath from 66,
// Crush of Doom from 60; Stun Attack never on GL or WL; DS and BH hold
// Hammer Crush ahead of Stun Attack.
var heavyRowTable = map[int]map[string]string{
	40: {"GL": "1", "WL": "48 36 245", "DS": "260 100 190 255", "BH": "260 100"},
	60: {"GL": "6 261 5 9 7 1", "WL": "48 36 245", "DS": "260 100 315 190 255", "BH": "260 100"},
	65: {"GL": "6 261 5 9 7 1", "WL": "48 36 245", "DS": "260 100 315 190 255", "BH": "260 100"},
	66: {"GL": "6 261 5 9 7 1", "WL": "320 48 36 245", "DS": "260 100 315 190 255", "BH": "260 100"},
	74: {"GL": "6 261 5 9 7 1", "WL": "320 48 36 245", "DS": "260 100 315 190 255", "BH": "260 100"},
}

// checkHeavyPvPRows checks the GL, WL, DS and BH rows of the row table
// (pvpRowTable) at each level of heavyRowTable.
func checkHeavyPvPRows(t *testing.T, classes map[int32]datapack.Class, skills map[datapack.SkillRef]datapack.Skill) {
	t.Helper()
	m := &TravelMap{classes: classes, skills: skills}
	for _, level := range slices.Sorted(maps.Keys(heavyRowTable)) {
		got := map[string]string{}
		for r, refs := range pvpRowTable(m, shippedKits, level) {
			if _, ok := heavyRowTable[level][r.ID]; ok {
				got[r.ID] = refWords(t, skills, refs)
			}
		}
		if want := heavyRowTable[level]; !maps.Equal(got, want) {
			t.Errorf("level %d: heavy rows %v, want %v", level, got, want)
		}
	}
}

// TestPvPRowHeavyKits checks the heavy rows' kits: GL, WL, DS and BH set
// their own lists, WL learns Wrath, GL and WL no longer learn the Stun
// Attack they never pass; the WA, OR and SC lists stay as they are.
func TestPvPRowHeavyKits(t *testing.T) {
	for _, tc := range []struct {
		name string
		kit  ClassKit
		pvp  []Step
	}{
		{"GL", gladiatorKit, gladiatorPvP},
		{"WL", warlordKit, warlordSecondPvP},
		{"DS", destroyerKit, destroyerPvP},
		{"BH", bountyHunterKit, bountyHunterPvP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !slices.EqualFunc(tc.kit.PvP, tc.pvp, func(a, b Step) bool { return a.Skill == b.Skill && a.Interrupt == b.Interrupt }) {
				t.Errorf("kit PvP %+v, want the %s list", tc.kit.PvP, tc.name)
			}
		})
	}
	if !slices.Contains(warlordKit.Skills, skillWrath) {
		t.Errorf("WL Skills %v without Wrath", warlordKit.Skills)
	}
	for _, k := range []ClassKit{gladiatorKit, warlordKit} {
		if slices.Contains(k.Skills, skillStunAttack) {
			t.Errorf("Skills %v learn Stun Attack", k.Skills)
		}
	}
	for _, tc := range []struct {
		name string
		got  []Step
		want []int32
	}{
		{"WA", warlordPvP, []int32{skillLionheart, skillStunAttack}},
		{"OR", overlordPvP, []int32{skillRage, skillLionheart, skillStunAttack}},
		{"SC", scavengerPvP, []int32{skillStunAttack}},
	} {
		got := make([]int32, len(tc.got))
		for i, st := range tc.got {
			got[i] = st.Skill
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s list %v, want %v unchanged", tc.name, got, tc.want)
		}
	}
}

// TestPvPRowGL checks the gladiator's PvP row: Lionheart at engage on a
// caster, not on a Warrior; Triple Slash as the interrupt of a Cleric's
// Heal (1011, a magic cast) when its cast-break chance passes MinBreak,
// not under it; Triple Slash still a damage step.
func TestPvPRowGL(t *testing.T) {
	all := []int32{skillLionheart, skillTripleSlash}
	playRow(t, classGladiator, []rowCase{
		{name: "a Human Mystic: Lionheart", level: 40, weapon: "DUAL", skills: all, opp: lyrMystic, want: skillLionheart},
		{name: "a Warrior: no Lionheart, Triple Slash", level: 40, weapon: "DUAL", skills: all, opp: lyrWarrior, want: skillTripleSlash, not: []int32{skillLionheart}},
	})
	for _, tc := range []struct {
		name      string
		patk      int32
		men       int
		interrupt bool
	}{
		{"the break chance passes MinBreak: Triple Slash interrupts", 1000, 0, true},
		{"a weak hit on a Cleric of MEN 60, under MinBreak: no interrupt", 1, 60, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpRowBrain(t, classGladiator, 40, "DUAL", lyrCleric, skillTripleSlash)
			foeOf(t, b, lyrCleric, Point{Y: 60}, 0)
			b.opened = pcID
			b.world.Self.PAtk = tc.patk
			if tc.men > 0 {
				c := b.cfg.Travel.classes[lyrCleric]
				b.cfg.Travel.classes[lyrCleric] = withStats(c, c.CON, tc.men)
			}
			healing(t, b)
			now := at(2600 * time.Millisecond)
			bot, opp, ok := b.pvpFighters(pcID)
			if !ok {
				t.Fatal("no fighters")
			}
			chance := charmodel.CastBreakChance(charmodel.SkillDamage(bot, rowSkills[datapack.SkillRef{ID: skillTripleSlash, Level: 1}], opp), opp.MEN)
			if chance >= MinBreak != tc.interrupt {
				t.Fatalf("break chance %.1f %%, want it on the %v side of MinBreak", chance, tc.interrupt)
			}
			_, a := b.decide(now)
			if a.interrupts != tc.interrupt || tc.interrupt && (a.kind != actCast || a.skill != skillTripleSlash) {
				t.Fatalf("action %v skill %d interrupts %v at a break chance of %.1f %%, want the interrupt %v", a.kind, a.skill, a.interrupts, chance, tc.interrupt)
			}
		})
	}
}

// TestPvPRowWL checks the warlord's PvP row: Wrath from 66 opens in a
// duel, not at 65; in the open world it opens with no bystander, and not
// with a flagged karma-free bystander within its 80 of the target (the
// area rule); Lionheart at engage on a caster; Thunder Storm held against
// a Cleric until its Heal, at reuse against a Warrior.
func TestPvPRowWL(t *testing.T) {
	near := func(t *testing.T, b *Brain) { foeOf(t, b, lyrMystic, Point{X: 30}, 0) }
	duel := func(t *testing.T, b *Brain) {
		t.Helper()
		duelUntil(t, b, DuelFighting, t0)
		b.reactAt = time.Time{}
	}
	bystander := func(t *testing.T, b *Brain) { player(t, b, pvpThird, "x", Point{X: 90}, 1, 0, 9) }
	for _, tc := range []struct {
		name  string
		level int32
		setup []func(*testing.T, *Brain)
		want  int32
	}{
		{"a duel at 66: Wrath opens", 66, []func(*testing.T, *Brain){near, duel}, skillWrath},
		{"a duel at 65: no Wrath", 65, []func(*testing.T, *Brain){near, duel}, 0},
		{"the open world at 66: Wrath opens", 66, []func(*testing.T, *Brain){near}, skillWrath},
		{"a flagged bystander in Wrath's reach: no Wrath", 66, []func(*testing.T, *Brain){near, bystander}, 0},
		{"at 66, opened: no Wrath", 66, []func(*testing.T, *Brain){near, opened}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpRowBrain(t, classWarlord, tc.level, "POLE", lyrMystic, skillWrath, skillThunderStorm)
			for _, f := range tc.setup {
				f(t, b)
			}
			now := at(6 * time.Second)
			got := castOf(t, b, now)
			if tc.want != 0 && got != tc.want || tc.want == 0 && got == skillWrath {
				t.Fatalf("cast %d, want %d", got, tc.want)
			}
		})
	}
	all := []int32{skillLionheart, skillThunderStorm}
	playRow(t, classWarlord, []rowCase{
		{name: "a Human Mystic: Lionheart", level: 60, weapon: "POLE", skills: all, opp: lyrMystic, want: skillLionheart},
		{name: "a Cleric, Lionheart cooling: Thunder Storm held", level: 60, weapon: "POLE", skills: all, opp: lyrCleric, setup: cooling(skillLionheart), not: []int32{skillThunderStorm}},
		{
			name: "the Cleric's Heal: Thunder Storm", level: 60, weapon: "POLE", skills: all, opp: lyrCleric, heal: true,
			setup: both(cooling(skillLionheart), func(t *testing.T, b *Brain) { foeOf(t, b, lyrCleric, Point{Y: 60}, 0) }), want: skillThunderStorm,
		},
		{name: "a Warrior: Thunder Storm at reuse", level: 60, weapon: "POLE", skills: all, opp: lyrWarrior, setup: warriorAt(60), want: skillThunderStorm, not: []int32{skillLionheart}},
	})
}

// heavyStunCases are DS's and BH's shared Hammer Crush and Stun Attack
// cases with weapon: held against a Cleric, Hammer Crush before its Heal
// lands, Stun Attack only with Hammer Crush cooling; both at reuse against
// a Warrior, Hammer Crush first.
func heavyStunCases(weapon string, skills []int32, after int32, afterKind actionKind) []rowCase {
	stuns := []int32{skillHammerCrush, skillStunAttack}
	nearCleric := func(t *testing.T, b *Brain) { foeOf(t, b, lyrCleric, Point{Y: 60}, 0) }
	return []rowCase{
		{name: "a Cleric, opened: both held", level: 55, weapon: weapon, skills: skills, opp: lyrCleric, setup: opened, want: after, kind: afterKind, not: stuns},
		{name: "the Cleric's Heal: Hammer Crush", level: 55, weapon: weapon, skills: skills, opp: lyrCleric, heal: true, setup: both(opened, nearCleric), want: skillHammerCrush},
		{
			name: "the Cleric's Heal, Hammer Crush cooling: Stun Attack", level: 55, weapon: weapon, skills: skills, opp: lyrCleric, heal: true,
			setup: both(opened, nearCleric, cooling(skillHammerCrush)), want: skillStunAttack,
		},
		{name: "a Warrior: Hammer Crush at reuse", level: 55, weapon: weapon, skills: skills, opp: lyrWarrior, setup: opened, want: skillHammerCrush},
		{name: "a Warrior, Hammer Crush cooling: Stun Attack", level: 55, weapon: weapon, skills: skills, opp: lyrWarrior, setup: both(opened, cooling(skillHammerCrush)), want: skillStunAttack},
		{
			name: "a Warrior, both cooling: no stun", level: 55, weapon: weapon, skills: skills, opp: lyrWarrior,
			setup: both(opened, cooling(stuns...)), want: after, kind: afterKind, not: stuns,
		},
	}
}

// TestPvPRowDS checks the destroyer's PvP row: Rage at engage on a
// Cleric, not on a Warrior; the shared stun cases (heavyStunCases), Power
// Smash once both stuns are held or cooling.
func TestPvPRowDS(t *testing.T) {
	all := []int32{skillHammerCrush, skillStunAttack, skillPowerSmash}
	playRow(t, classDestroyer, heavyStunCases("BIGBLUNT", all, skillPowerSmash, 0))
	withRage := append(slices.Clone(all), skillRage)
	playRow(t, classDestroyer, []rowCase{
		{name: "a Cleric: Rage", level: 55, weapon: "BIGBLUNT", skills: withRage, opp: lyrCleric, want: skillRage},
		{name: "a Warrior: no Rage, Hammer Crush", level: 55, weapon: "BIGBLUNT", skills: withRage, opp: lyrWarrior, want: skillHammerCrush, not: []int32{skillRage}},
	})
}

// TestPvPRowBH checks the bounty hunter's PvP row: the shared stun cases
// (heavyStunCases), a plain swing once both stuns are held or cooling.
func TestPvPRowBH(t *testing.T) {
	playRow(t, classBountyHunter, heavyStunCases("BLUNT", []int32{skillHammerCrush, skillStunAttack}, 0, actAttack))
}

// TestPvPControlLandRateHammerCrush checks the Hammer Crush case of the
// control steps' land rate (docs/pvp/layer.md "PvP order"): Hammer Crush
// (Stun effectPower 50, resisted by CON, as Stun Attack) lands on an Orc
// Fighter (CON 47) at about 33 %, under MinLand: skipped; on a Human
// Mystic (CON 27) at about 50 %: cast.
func TestPvPControlLandRateHammerCrush(t *testing.T) {
	for _, tc := range []struct {
		name     string
		opp      int32
		min, max float64
		want     bool
	}{
		{"an Orc Fighter", rowOrcFighter, 30, MinLand, false},
		{"a Human Mystic", lyrMystic, 48, 52, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpRowBrain(t, classDestroyer, 40, "BLUNT", tc.opp, skillHammerCrush)
			withKit(b, ClassKit{Rotation: []Step{{Skill: skillHammerCrush, Weapons: stunWeapons}}})
			bot, opp, ok := b.pvpFighters(pcID)
			if !ok {
				t.Fatal("no fighters")
			}
			rate := charmodel.LandRate(rowSkills[datapack.SkillRef{ID: skillHammerCrush, Level: 1}], bot, opp)
			if rate < tc.min || rate >= tc.max {
				t.Fatalf("Hammer Crush lands at %.1f %%, want [%v, %v)", rate, tc.min, tc.max)
			}
			if got := castOf(t, b, at(time.Second)) == skillHammerCrush; got != tc.want {
				t.Errorf("Hammer Crush at %.1f %%: cast %v, want %v", rate, got, tc.want)
			}
		})
	}
}
