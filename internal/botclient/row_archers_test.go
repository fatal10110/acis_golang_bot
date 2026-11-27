package botclient

import (
	"slices"
	"testing"
	"time"
)

// bowFight is one fight decision of an archer row: the bot at level
// wearing weapon ("" a bow) with npcID at x, its HP at hp % (0: full),
// hit by two monsters or not, opened on npcID or not, holding the skills
// of hold; skill on object is the cast fight decides (0: a melee click),
// opens that it uses the opener up.
type bowFight struct {
	name   string
	level  int32
	weapon string
	x      int32
	hp     int32
	two    bool
	opened bool
	hold   []int32
	skill  int32
	object int32
	opens  bool
}

// runBowFights checks each fight of fights for class, knowing skills; a
// zero object is npcID.
func runBowFights(t *testing.T, class int32, skills []int32, fights []bowFight) {
	t.Helper()
	now := at(time.Second)
	for _, tt := range fights {
		t.Run(tt.name, func(t *testing.T) {
			weapon := tt.weapon
			if weapon == "" {
				weapon = "BOW"
			}
			b := rowBrain(t, class, tt.level, weapon, skills...)
			if tt.x != 0 {
				b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: tt.x}}, t0)
			}
			if tt.hp != 0 {
				b.world.Self.HP = tt.hp
			}
			if tt.two {
				hitByTwo(b)
			}
			if tt.opened {
				b.opened = npcID
			}
			for _, id := range tt.hold {
				b.world.holdSkill(id, now.Add(time.Minute))
			}
			object := tt.object
			if object == 0 && tt.skill != 0 {
				object = npcID
			}
			if a := fightCast(t, b, now, tt.skill, object); tt.opens && !a.opens {
				t.Errorf("cast of %d does not use the opener up", tt.skill)
			}
		})
	}
}

// archerSelf are the in-combat self steps every archer row casts first:
// the fights of the attacks hold them.
var archerSelf = []int32{skillRapidShot, skillSpiritOfSagittarius}

// plus is ids then more, in a slice of its own.
func plus(ids []int32, more ...int32) []int32 { return append(slices.Clip(ids), more...) }

// idleRow is rowBrain out of combat with itself selected (a self buff
// selects the bot first).
func idleRow(t *testing.T, class, level int32, skills ...int32) *Brain {
	t.Helper()
	b := rowBrain(t, class, level, "BOW", skills...)
	b.world.Self.InCombat, b.world.Target = false, selfID
	return b
}

// TestRowHK checks the hawkeye: with a bow Stunning Shot opens from 900,
// then Double Shot, Burst Shot from 46 only with two attackers, Stunning
// Shot, and Power Shot only while the others cool; Rapid Shot in combat,
// Spirit of Sagittarius from 58, Ultimate Evasion under 30 % HP; out of
// combat Hawk Eye, Soul of Sagittarius from 46 and Blessing of Sagittarius
// from 58; Dash first when it flees; below SitHP it casts Relax.
func TestRowHK(t *testing.T) {
	all := []int32{
		skillStunningShot, skillDoubleShot, skillBurstShot, skillPowerShot, skillUltimateEvasion,
		skillRapidShot, skillSpiritOfSagittarius, skillDash, skillRelax,
	}
	cool := append([]int32{}, archerSelf...)
	runBowFights(t, classHawkeye, all, []bowFight{
		{name: "Stunning Shot opens from 900", level: 40, x: 900, hold: cool, skill: skillStunningShot, opens: true},
		{name: "Double Shot after the opener", level: 40, opened: true, hold: cool, skill: skillDoubleShot},
		{name: "Burst Shot from 46 with two attackers", level: 46, opened: true, two: true, hold: plus(cool, skillDoubleShot), skill: skillBurstShot},
		{name: "no Burst Shot with one attacker", level: 46, opened: true, hold: plus(cool, skillDoubleShot), skill: skillStunningShot},
		{name: "no Burst Shot at 45", level: 45, opened: true, two: true, hold: plus(cool, skillDoubleShot), skill: skillStunningShot},
		{name: "Power Shot while the others cool", level: 46, opened: true, two: true, hold: plus(cool, skillDoubleShot, skillBurstShot, skillStunningShot), skill: skillPowerShot},
		{name: "melee while every shot cools", level: 46, opened: true, hold: plus(cool, skillDoubleShot, skillStunningShot, skillPowerShot)},
		{name: "Rapid Shot in combat", level: 40, opened: true, skill: skillRapidShot, object: selfID},
		{name: "Spirit of Sagittarius from 58", level: 58, opened: true, hold: []int32{skillRapidShot}, skill: skillSpiritOfSagittarius, object: selfID},
		{name: "no Spirit of Sagittarius at 57", level: 57, opened: true, hold: []int32{skillRapidShot}, skill: skillDoubleShot},
		{name: "Ultimate Evasion under 30 % HP", level: 40, opened: true, hp: 25, skill: skillUltimateEvasion, object: selfID},
	})

	now := at(time.Second)
	buffs := []int32{skillHawkEye, skillSoulOfSagittarius, skillBlessingOfSagittarius}
	t.Run("Hawk Eye out of combat", func(t *testing.T) {
		a, _ := idleRow(t, classHawkeye, 40, buffs...).ruleRebuff(now)
		wantCast(t, a, skillHawkEye, selfID)
	})
	t.Run("Soul of Sagittarius out of combat from 46", func(t *testing.T) {
		b := idleRow(t, classHawkeye, 46, buffs...)
		effectsOn(b, skillHawkEye)
		a, _ := b.ruleRebuff(now)
		wantCast(t, a, skillSoulOfSagittarius, selfID)
	})
	t.Run("no Soul of Sagittarius at 45", func(t *testing.T) {
		b := idleRow(t, classHawkeye, 45, buffs...)
		effectsOn(b, skillHawkEye)
		wantNoRebuff(t, b, now)
	})
	t.Run("Blessing of Sagittarius from 58", func(t *testing.T) {
		b := idleRow(t, classHawkeye, 58, buffs...)
		effectsOn(b, skillHawkEye, skillSoulOfSagittarius)
		a, _ := b.ruleRebuff(now)
		wantCast(t, a, skillBlessingOfSagittarius, selfID)
	})
	t.Run("Dash to flee", func(t *testing.T) {
		wantFleeCast(t, rowBrain(t, classHawkeye, 40, "BOW", all...), skillDash)
	})
	t.Run("rests with Relax", func(t *testing.T) {
		b := rowBrain(t, classHawkeye, 40, "BOW", all...)
		wantCast(t, restDecision(t, b, 30, 100, now), skillRelax, selfID)
	})
}

// TestRowSR checks the silver ranger: Entangle opens, then Stunning Shot
// from 900 while Entangle cools, then Double Shot and Burst Shot from 46
// with two attackers; Rapid Fire on itself in combat from 60 with a bow,
// never without one; Elemental Heal under 50 % HP; out of combat Spirit
// Barrier and the Sagittarius buffs after the auras; Sprint first when it
// flees; below SitHP it sits plainly.
func TestRowSR(t *testing.T) {
	all := []int32{
		skillEntangle, skillStunningShot, skillDoubleShot, skillBurstShot, skillPowerShot,
		skillElementalHeal, skillUltimateEvasion, skillRapidShot, skillSpiritOfSagittarius, skillRapidFire, skillSprint,
	}
	cool := append([]int32{}, archerSelf...)
	selfAll := append(append([]int32{}, archerSelf...), skillRapidFire)
	runBowFights(t, classSilverRanger, all, []bowFight{
		{name: "Entangle opens", level: 40, hold: cool, skill: skillEntangle, opens: true},
		{name: "Stunning Shot opens from 900 while Entangle cools", level: 40, x: 900, hold: plus(cool, skillEntangle), skill: skillStunningShot, opens: true},
		{name: "Double Shot after the opener", level: 40, opened: true, hold: cool, skill: skillDoubleShot},
		{name: "Burst Shot from 46 with two attackers", level: 46, opened: true, two: true, hold: plus(cool, skillDoubleShot), skill: skillBurstShot},
		{name: "Rapid Fire from 60 with a bow", level: 60, opened: true, hold: cool, skill: skillRapidFire, object: selfID},
		{name: "no Rapid Fire at 59", level: 59, opened: true, hold: cool, skill: skillDoubleShot},
		{name: "no Rapid Fire nor shot with a dagger", level: 60, weapon: "DAGGER", opened: true, hold: cool},
		{name: "Power Shot while the others cool", level: 60, opened: true, hold: plus(selfAll, skillDoubleShot, skillStunningShot), skill: skillPowerShot},
		{name: "Elemental Heal under 50 % HP", level: 40, opened: true, hp: 45, skill: skillElementalHeal, object: selfID},
	})

	now := at(time.Second)
	auras := []int32{skillDefenseAura, skillAttackAura}
	buffs := append(append([]int32{}, auras...), skillSpiritBarrier, skillSoulOfSagittarius, skillBlessingOfSagittarius)
	t.Run("Spirit Barrier after the auras", func(t *testing.T) {
		b := idleRow(t, classSilverRanger, 40, buffs...)
		effectsOn(b, auras...)
		a, _ := b.ruleRebuff(now)
		wantCast(t, a, skillSpiritBarrier, selfID)
	})
	t.Run("Blessing of Sagittarius from 58", func(t *testing.T) {
		b := idleRow(t, classSilverRanger, 58, buffs...)
		effectsOn(b, append(auras, skillSpiritBarrier, skillSoulOfSagittarius)...)
		a, _ := b.ruleRebuff(now)
		wantCast(t, a, skillBlessingOfSagittarius, selfID)
	})
	t.Run("Sprint to flee", func(t *testing.T) {
		wantFleeCast(t, rowBrain(t, classSilverRanger, 40, "BOW", all...), skillSprint)
	})
	t.Run("sits plainly", func(t *testing.T) {
		b := rowBrain(t, classSilverRanger, 40, "BOW", all...)
		effectsOn(b, auras...)
		if a := restDecision(t, b, 30, 100, now); a.kind != actSitStand {
			t.Fatalf("rest = kind %d skill %d, want a plain sit", a.kind, a.skill)
		}
	})
}

// TestRowPR checks the phantom ranger: Stunning Shot opens from 900;
// Drain Health under 60 % HP ranks first, before Fatal Counter; Fatal Counter only from 60 and
// only under 50 % HP, then Double Shot; Dead Eye on itself in combat from
// 60; out of combat Soul of Sagittarius from 46 after the auras; it sits
// plainly.
func TestRowPR(t *testing.T) {
	all := []int32{
		skillStunningShot, skillFatalCounter, skillDoubleShot, skillPowerShot, skillDrainHealth,
		skillUltimateEvasion, skillRapidShot, skillSpiritOfSagittarius, skillDeadEye,
	}
	cool := append([]int32{}, archerSelf...)
	selfAll := append(append([]int32{}, archerSelf...), skillDeadEye)
	runBowFights(t, classPhantomRanger, all, []bowFight{
		{name: "Stunning Shot opens from 900", level: 40, x: 900, hold: cool, skill: skillStunningShot, opens: true},
		{name: "Double Shot after the opener", level: 40, opened: true, hold: cool, skill: skillDoubleShot},
		{name: "Drain Health under 60 % HP before Fatal Counter", level: 60, opened: true, hp: 45, hold: selfAll, skill: skillDrainHealth},
		{name: "Fatal Counter from 60 under 50 % HP", level: 60, opened: true, hp: 45, hold: plus(selfAll, skillDrainHealth), skill: skillFatalCounter},
		{name: "no Fatal Counter at 59", level: 59, opened: true, hp: 45, hold: plus(selfAll, skillDrainHealth), skill: skillDoubleShot},
		{name: "no Fatal Counter at 50 % HP", level: 60, opened: true, hp: 50, hold: plus(selfAll, skillDrainHealth), skill: skillDoubleShot},
		{name: "Dead Eye from 60", level: 60, opened: true, hold: cool, skill: skillDeadEye, object: selfID},
		{name: "no Dead Eye at 59", level: 59, opened: true, hold: cool, skill: skillDoubleShot},
		{name: "Power Shot while the others cool", level: 60, opened: true, hold: plus(selfAll, skillDoubleShot, skillStunningShot), skill: skillPowerShot},
	})

	now := at(time.Second)
	auras := []int32{skillDefenseAura, skillAttackAura}
	buffs := append(append([]int32{}, auras...), skillSoulOfSagittarius)
	t.Run("Soul of Sagittarius from 46", func(t *testing.T) {
		b := idleRow(t, classPhantomRanger, 46, buffs...)
		effectsOn(b, auras...)
		a, _ := b.ruleRebuff(now)
		wantCast(t, a, skillSoulOfSagittarius, selfID)
	})
	t.Run("no Soul of Sagittarius at 45", func(t *testing.T) {
		b := idleRow(t, classPhantomRanger, 45, buffs...)
		effectsOn(b, auras...)
		wantNoRebuff(t, b, now)
	})
	t.Run("sits plainly", func(t *testing.T) {
		b := rowBrain(t, classPhantomRanger, 40, "BOW", all...)
		effectsOn(b, auras...)
		if a := restDecision(t, b, 30, 100, now); a.kind != actSitStand {
			t.Fatalf("rest = kind %d skill %d, want a plain sit", a.kind, a.skill)
		}
	})
}
