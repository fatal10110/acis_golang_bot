package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The nuker second-profession PvP rows (docs/pvp/rows-second.md "Tests",
// X27): SN, SG, SH and WC play their own lists on the mystic rows'
// harness (mysticRowBrain).

// muted is the CharInfo abnormal bit of a mute (server
// model/skill/abnormal.go AbnormalMute).
const muted = int32(0x100)

// checkNukerPvPRows checks the nuker rows SN, SG, SH and WC of the row
// table at 44, where Ice Dagger and Death Spike come in, and at 58, where
// Rain of Fire and Aqua Splash do (docs/pvp/rows-second.md, X27);
// checkSecondPvPRows holds 40, 55 and 74.
func checkNukerPvPRows(t *testing.T, classes map[int32]datapack.Class, skills map[datapack.SkillRef]datapack.Skill) {
	t.Helper()
	want := map[int]map[string]string{
		44: {
			"SN": "1184/MA 1230/CH 1160/M 1222/MA 1184/M 1230 1275 1231 1147 1220",
			"SG": "1206/M 1206/A 1235 1236/M 1164/M 1235 1237 1265 1275 1231 1264 1175",
			"SH": "1206/M 1206/A 1064/CHB 1168/CH 1160/M 1184/M 1239 1148 1267 1266 1178 1172 1234 1147",
			"WC": "260 100 1245 1090 1245 1107",
		},
		58: {
			"SN": "1184/MA 1230/CH 1160/M 1222/MA 1184/M 1230 1296 1275 1231 1147 1220",
			"SG": "1206/M 1206/A 1235 1236/M 1164/M 1235 1237 1265 1295 1275 1231 1264 1175",
			"SH": "1206/M 1206/A 1064/CHB 1168/CH 1160/M 1184/M 1239 1148 1267 1176 1266 1178 1172 1234 1147",
			"WC": "260 100 1245 1090 1245 1107",
		},
	}
	m := &TravelMap{classes: classes, skills: skills}
	for level, rows := range want {
		got := map[string]string{}
		for r, refs := range pvpRowTable(m, shippedKits, level) {
			if _, ok := rows[r.ID]; ok {
				got[r.ID] = refWords(t, skills, refs)
			}
		}
		for id, w := range rows {
			if got[id] != w {
				t.Errorf("level %d row %s: steps %q, want %q", level, id, got[id], w)
			}
		}
	}
}

// TestPvPRowNukerKits checks the nukers' kits (docs/pvp/rows-second.md,
// X27): SN, SG, SH and WC set their own lists, every skill of which they
// learn (SG Frost Bolt and Curse: Weakness, SH Silence added); the HW, EW,
// DW and OS lists stay as they are for their first professions.
func TestPvPRowNukerKits(t *testing.T) {
	sn := []int32{
		skillIceBolt, skillProminence, skillSlow, skillCurseChaos, skillIceBolt,
		skillProminence, skillSurrenderToFire, skillRainOfFire, skillAuraBolt, skillAuraFlare,
		skillAuraBolt, skillAuraFlare, skillIceBolt,
	}
	sg := []int32{
		skillWindShackle, skillWindShackle, skillHydroBlast, skillFrostBolt, skillCurseWeakness,
		skillHydroBlast, skillIceDagger, skillSolarFlare, skillAquaSplash, skillAuraBolt, skillAuraFlare,
		skillAuraBolt, skillAuraFlare, skillFrostBolt,
	}
	sh := []int32{
		skillWindShackle, skillWindShackle, skillSilence, skillCursePoison, skillSlow, skillIceBolt,
		skillHurricane, skillDeathSpike, skillShadowFlare, skillTempest, skillShadowSpark, skillTwister, skillAuraBurn,
		skillAuraBurn, skillIceBolt,
	}
	wc := []int32{
		skillChantFire, skillAuraSink, skillHammerCrush, skillStunAttack, skillDreamingSpirit,
		skillSealBinding, skillDreamingSpirit, skillFear,
	}
	for _, tc := range []struct {
		name  string
		class int32
		want  []int32
	}{
		{"SN", classSorcerer, sn},
		{"SG", classSpellsinger, sg},
		{"SH", classSpellhowler, sh},
		{"WC", classWarcryer, wc},
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
		{"HW", humanWizardPvP, []int32{
			skillIceBolt, skillBlaze, skillSlow, skillCurseChaos, skillIceBolt, skillBlaze,
			skillEnergyBolt, skillAuraBurn, skillEnergyBolt, skillAuraBurn, skillIceBolt,
		}},
		{"EW", elvenWizardPvP, []int32{
			skillWindShackle, skillWindShackle, skillSolarSpark, skillAquaSwirl, skillIceBolt, skillSolarSpark,
			skillAquaSwirl, skillEnergyBolt, skillAuraBurn, skillEnergyBolt, skillAuraBurn, skillIceBolt,
		}},
		{"DW", darkWizardPvP, []int32{
			skillWindShackle, skillWindShackle, skillCursePoison, skillShadowSpark, skillSlow, skillIceBolt,
			skillShadowSpark, skillTwister, skillAuraBurn, skillAuraBurn, skillIceBolt,
		}},
		{"OS", orcShamanPvP, []int32{
			skillLifeDrain, skillChantFire, skillFrostFlame, skillAuraSink, skillFrostFlame, skillLifeDrain,
			skillStunAttack, skillDreamingSpirit, skillSealBinding, skillDreamingSpirit, skillFear,
		}},
	} {
		if got := stepSkills(tc.got); !slices.Equal(got, tc.want) {
			t.Errorf("%s list %v, want %v unchanged", tc.name, got, tc.want)
		}
	}
}

// TestPvPRowSN checks the sorcerer: on a Warrior Slow first (due control
// goes ahead of the Ice Bolt opener, docs/pvp/layer.md "PvP order"), the
// kite on it, Curse Chaos, Ice Bolt and the kite on its slow; Prominence opens on a
// Cleric and stays ahead of Blaze, as Aura Bolt does; Aura Bolt answers a
// Cleric's Heal when its hit passes MinBreak, not under it; Break: Ice
// Bolt, the flee while the opponent is still slowed.
func TestPvPRowSN(t *testing.T) {
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs melee: Slow first", class: classSorcerer, level: 40, opp: lyrWarrior, skill: skillSlow},
		{name: "vs melee, Slow on reuse: Curse Chaos", class: classSorcerer, level: 40, opp: lyrWarrior, setup: reusing(skillSlow), skill: skillCurseChaos},
		{name: "vs melee, slowed: the kite", class: classSorcerer, level: 40, opp: lyrWarrior, setup: landedOn(skillSlow), move: true},
		{name: "vs melee, Slow and Curse Chaos on reuse: Ice Bolt opens", class: classSorcerer, level: 40, opp: lyrWarrior, setup: reusing(skillSlow, skillCurseChaos), skill: skillIceBolt},
		{name: "vs melee, Ice Bolt landed: the kite", class: classSorcerer, level: 40, opp: lyrWarrior, setup: both(landedOn(skillIceBolt), reusing(skillSlow, skillCurseChaos, skillIceBolt)), move: true},
		{name: "vs an archer: Curse Chaos first", class: classSorcerer, level: 40, opp: rowRogue, bow: true, skill: skillCurseChaos},
		{name: "vs a Cleric: Prominence opens", class: classSorcerer, level: 40, opp: lyrCleric, skill: skillProminence},
		{name: "vs a Cleric, opened: Prominence ahead of Blaze", class: classSorcerer, level: 40, opp: lyrCleric, setup: opened, skill: skillProminence},
		{
			name: "vs a Cleric, Prominence on reuse: Aura Bolt ahead of Blaze", class: classSorcerer, level: 40, opp: lyrCleric,
			setup: both(opened, reusing(skillProminence, skillSurrenderToFire)), skill: skillAuraBolt,
		},
		{name: "vs a Cleric's Heal, a strong hit: Aura Bolt", class: classSorcerer, level: 40, opp: lyrCleric, setup: both(opened, matk(5000), healCast(5*time.Second)), skill: skillAuraBolt},
		{name: "vs a Cleric's Heal, a weak hit: no Int", class: classSorcerer, level: 40, opp: lyrCleric, setup: both(opened, healCast(5*time.Second)), skill: skillProminence},
		{name: "breaking off: Ice Bolt", class: classSorcerer, level: 40, opp: lyrWarrior, setup: both(breaking, reusing(skillVampiricTouch)), skill: skillIceBolt},
		{name: "breaking off, still slowed: the flee", class: classSorcerer, level: 40, opp: lyrWarrior, setup: both(breaking, reusing(skillVampiricTouch), landedOn(skillIceBolt)), move: true},
	})
}

// TestPvPRowSG checks the spellsinger: on a Warrior Wind Shackle, Curse:
// Weakness, then Frost Bolt and the kite on its slow; Hydro Blast opens on
// a caster and stays ahead of Solar Spark; Aura Bolt answers a Cleric's
// Heal on a strong hit; Break: Frost Bolt unless the opponent is still
// slowed, then the flee.
func TestPvPRowSG(t *testing.T) {
	controlled := landedOn(skillWindShackle, skillCurseWeakness)
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs melee: Wind Shackle", class: classSpellsinger, level: 40, opp: lyrWarrior, skill: skillWindShackle},
		{name: "vs melee, shackled: Curse: Weakness", class: classSpellsinger, level: 40, opp: lyrWarrior, setup: landedOn(skillWindShackle), skill: skillCurseWeakness},
		{name: "vs melee, shackled and weakened: Frost Bolt", class: classSpellsinger, level: 40, opp: lyrWarrior, setup: controlled, skill: skillFrostBolt},
		{
			name: "vs melee, Frost Bolt landed: the kite", class: classSpellsinger, level: 40, opp: lyrWarrior,
			setup: both(landedOn(skillWindShackle, skillCurseWeakness, skillFrostBolt), reusing(skillFrostBolt)), move: true,
		},
		{name: "vs an archer: Wind Shackle", class: classSpellsinger, level: 40, opp: rowRogue, bow: true, skill: skillWindShackle},
		{name: "vs a caster: Hydro Blast opens", class: classSpellsinger, level: 40, opp: lyrMystic, skill: skillHydroBlast},
		{name: "vs a caster, opened: Hydro Blast ahead of Solar Spark", class: classSpellsinger, level: 40, opp: lyrMystic, setup: opened, skill: skillHydroBlast},
		{name: "vs a caster, Hydro Blast on reuse: Solar Flare, no Frost Bolt", class: classSpellsinger, level: 40, opp: lyrMystic, setup: both(opened, reusing(skillHydroBlast)), skill: skillSolarFlare},
		{name: "vs a Cleric's Heal, a strong hit: Aura Bolt", class: classSpellsinger, level: 40, opp: lyrCleric, setup: both(opened, matk(5000), healCast(5*time.Second)), skill: skillAuraBolt},
		{name: "breaking off: Frost Bolt", class: classSpellsinger, level: 40, opp: lyrWarrior, setup: both(breaking, reusing(skillSelfHeal)), skill: skillFrostBolt},
		{name: "breaking off, still slowed: the flee", class: classSpellsinger, level: 40, opp: lyrWarrior, setup: both(breaking, reusing(skillSelfHeal), landedOn(skillFrostBolt)), move: true},
	})
}

// TestPvPRowSH checks the spellhowler: Silence on a Cleric, not again
// while its mute shows, again once it wore off; no Silence on a Warrior
// (Wind Shackle, then Slow, then Ice Bolt); Silence on a battle caster,
// but Curse: Poison only on a caster or healer; Hurricane ahead of Shadow
// Spark; Aura Burn answers a Cleric's Heal within 150 on a strong hit;
// Break: Ice Bolt.
func TestPvPRowSH(t *testing.T) {
	silenced := both(landedOn(skillSilence), foeSeen(lyrCleric, Point{Y: 300}, muted))
	sustain := reusing(skillVampiricClaw, skillVampiricTouch)
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs a Cleric: Silence", class: classSpellhowler, level: 40, opp: lyrCleric, skill: skillSilence},
		{name: "vs a Cleric, muted: Curse: Poison", class: classSpellhowler, level: 40, opp: lyrCleric, setup: silenced, skill: skillCursePoison},
		{
			name: "vs a Cleric, the mute worn off: Silence again", class: classSpellhowler, level: 40, opp: lyrCleric,
			setup: both(landedOn(skillSilence), foeSeen(lyrCleric, Point{Y: 300}, 0)), skill: skillSilence,
		},
		{
			name: "vs a Cleric, muted and poisoned: Hurricane ahead of Shadow Spark", class: classSpellhowler, level: 40, opp: lyrCleric,
			setup: both(silenced, landedOn(skillCursePoison)), skill: skillHurricane,
		},
		{name: "vs melee: Wind Shackle, no Silence", class: classSpellhowler, level: 40, opp: lyrWarrior, skill: skillWindShackle},
		{name: "vs melee, shackled: Slow", class: classSpellhowler, level: 40, opp: lyrWarrior, setup: landedOn(skillWindShackle), skill: skillSlow},
		{
			name: "vs melee, shackled, Slow on reuse: Ice Bolt, no poison", class: classSpellhowler, level: 40, opp: lyrWarrior,
			setup: both(landedOn(skillWindShackle), reusing(skillSlow)), skill: skillIceBolt,
		},
		{name: "vs a battle caster: Silence", class: classSpellhowler, level: 40, opp: rowOrcMy, skill: skillSilence},
		{
			name: "vs a battle caster, muted: Hurricane, no poison", class: classSpellhowler, level: 40, opp: rowOrcMy,
			setup: both(landedOn(skillSilence), foeSeen(rowOrcMy, Point{Y: 300}, muted)), skill: skillHurricane,
		},
		{
			name: "vs a Cleric's Heal within 150, a strong hit: Aura Burn", class: classSpellhowler, level: 40, opp: lyrCleric,
			setup: both(landedOn(skillSilence), foeSeen(lyrCleric, Point{Y: 50}, muted), matk(5000), healCast(5*time.Second)), skill: skillAuraBurn,
		},
		{name: "breaking off: Ice Bolt", class: classSpellhowler, level: 40, opp: lyrWarrior, setup: both(breaking, sustain), skill: skillIceBolt},
		{name: "breaking off, still slowed: the flee", class: classSpellhowler, level: 40, opp: lyrWarrior, setup: both(breaking, sustain, landedOn(skillIceBolt)), move: true},
	})
}

// TestPvPRowWC checks the warcryer: with a blunt, Hammer Crush held on a
// Cleric within its reach, cast before the Cleric's Heal lands, Stun
// Attack while it is on reuse; Hammer Crush at reuse on a Warrior in
// reach; Steal Essence opens ahead of Frost Flame; Aura Sink on an opened
// Cleric; Dreaming Spirit answers a Cleric's Heal without a blunt; Break:
// Seal of Binding on a stunned melee opponent next to it, Dreaming Spirit
// on an opponent free of DOTs, else Fear.
func TestPvPRowWC(t *testing.T) {
	near := foeSeen(lyrCleric, Point{Y: 50}, 0)
	blunt := wield("BLUNT")
	sustain := reusing(skillStealEssence, skillLifeDrain)
	far := func(abnormal int32) func(*testing.T, *Brain) { return foeSeen(lyrWarrior, Point{Y: 900}, abnormal) }
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs a Cleric in reach: no Hammer Crush at reuse", class: classWarcryer, level: 40, opp: lyrCleric, setup: both(blunt, opened, near, reusing(skillAuraSink)), skill: skillFrostFlame},
		{name: "vs a Cleric's Heal in reach: Hammer Crush", class: classWarcryer, level: 40, opp: lyrCleric, setup: both(blunt, opened, near, healCast(5*time.Second)), skill: skillHammerCrush},
		{
			name: "vs a Cleric's Heal in reach, Hammer Crush on reuse: Stun Attack", class: classWarcryer, level: 40, opp: lyrCleric,
			setup: both(blunt, opened, near, reusing(skillHammerCrush), healCast(5*time.Second)), skill: skillStunAttack,
		},
		{name: "vs melee in reach: Hammer Crush at reuse", class: classWarcryer, level: 40, opp: lyrWarrior, setup: both(blunt, foeSeen(lyrWarrior, Point{Y: 50}, 0)), skill: skillHammerCrush},
		{name: "vs melee at 300: Steal Essence opens", class: classWarcryer, level: 40, opp: lyrWarrior, setup: blunt, skill: skillStealEssence},
		{name: "vs melee at 300, opened: Frost Flame", class: classWarcryer, level: 40, opp: lyrWarrior, setup: both(blunt, opened), skill: skillFrostFlame},
		{name: "vs a Cleric, opened: Aura Sink", class: classWarcryer, level: 40, opp: lyrCleric, setup: opened, skill: skillAuraSink},
		{name: "vs a caster: Chant of Fire", class: classWarcryer, level: 40, opp: lyrMystic, skill: skillChantFire},
		{name: "vs a Cleric's Heal, no blunt: Dreaming Spirit", class: classWarcryer, level: 40, opp: lyrCleric, setup: both(opened, healCast(5*time.Second)), skill: skillDreamingSpirit},
		{
			name: "breaking off, a stunned melee opponent next to it: Seal of Binding", class: classWarcryer, level: 40, opp: lyrWarrior,
			setup: both(breaking, foeSeen(lyrWarrior, Point{Y: 50}, 0x40), sustain, matk(2000)), skill: skillSealBinding,
		},
		{name: "breaking off, no DOT, out of reach: Dreaming Spirit", class: classWarcryer, level: 40, opp: lyrWarrior, setup: both(breaking, far(0), sustain), skill: skillDreamingSpirit},
		{name: "breaking off, poisoned, out of reach: Fear", class: classWarcryer, level: 40, opp: lyrWarrior, setup: both(breaking, far(poisoned), sustain), skill: skillFear},
	})
}
