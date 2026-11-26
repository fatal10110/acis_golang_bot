package botclient

import (
	"testing"
	"time"
)

// daggerFight is one fight decision of a dagger row: the bot at level
// with npcID at x, its HP at hp % (0: full) and the target's at targetHP %
// (0: unseen), opened on it or not, holding the skills of hold; skill on
// object is the cast fight decides, opens that it uses the opener up.
type daggerFight struct {
	name         string
	level        int32
	x            int32
	hp, targetHP int32
	opened       bool
	hold         []int32
	skill        int32
	object       int32
	opens        bool
}

// runDaggerFights checks each fight of fights for class with a dagger
// worn, knowing skills; a zero object is npcID.
func runDaggerFights(t *testing.T, class int32, skills []int32, fights []daggerFight) {
	t.Helper()
	now := at(time.Second)
	for _, tt := range fights {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, class, tt.level, "DAGGER", skills...)
			if tt.x != 0 {
				b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: tt.x}}, t0)
			}
			if tt.hp != 0 {
				b.world.Self.HP = tt.hp
			}
			if tt.targetHP != 0 {
				o := b.world.Objects[npcID]
				o.HP, o.MaxHP = tt.targetHP, 100
			}
			if tt.opened {
				b.opened = npcID
			}
			for _, id := range tt.hold {
				b.world.holdSkill(id, now.Add(time.Minute))
			}
			object := tt.object
			if object == 0 {
				object = npcID
			}
			if a := fightCast(t, b, now, tt.skill, object); tt.opens && !a.opens {
				t.Errorf("cast of %d does not use the opener up", tt.skill)
			}
		})
	}
}

// wantFleeCast fails unless flee from npcID casts skill on the bot first.
func wantFleeCast(t *testing.T, b *Brain, skill int32) {
	t.Helper()
	wantCast(t, b.flee(npcID, at(time.Second)), skill, selfID)
}

// restDecision is decide out of combat for b at hp % HP and mp % MP; it
// fails unless the rule is rest.
func restDecision(t *testing.T, b *Brain, hp, mp int32, now time.Time) Action {
	t.Helper()
	s := &b.world.Self
	s.InCombat, s.HP, s.MP = false, hp, mp
	rule, a := b.decide(now)
	if rule != "rest" {
		t.Fatalf("decide = %q kind %d skill %d, want rest", rule, a.kind, a.skill)
	}
	return a
}

// TestRowTH checks the treasure hunter: at 52 Lure opens from 400, at 51
// it walks in; Critical Blow from 55, then Deadly Blow, Bleed only on a
// target at 50 % HP or more, then Mortal Blow; Ultimate Evasion under 30 %
// HP; Dash first when it flees; below SitHP out of combat it casts Relax.
func TestRowTH(t *testing.T) {
	all := []int32{skillLure, skillCriticalBlow, skillDeadlyBlow, skillBleed, skillMortalBlow, skillUltimateEvasion, skillDash, skillRelax}
	runDaggerFights(t, classTreasureHunter, all, []daggerFight{
		{name: "Lure opens from 400 at 52", level: 52, x: 400, skill: skillLure, opens: true},
		{name: "Critical Blow from 55", level: 55, opened: true, skill: skillCriticalBlow},
		{name: "no Critical Blow at 54", level: 54, opened: true, skill: skillDeadlyBlow},
		{name: "Deadly Blow while Critical Blow cools", level: 55, opened: true, hold: []int32{skillCriticalBlow}, skill: skillDeadlyBlow},
		{name: "Bleed on a target at 50 %", level: 55, opened: true, targetHP: 50, hold: []int32{skillCriticalBlow, skillDeadlyBlow}, skill: skillBleed},
		{name: "Mortal Blow on a target under 50 %", level: 55, opened: true, targetHP: 49, hold: []int32{skillCriticalBlow, skillDeadlyBlow}, skill: skillMortalBlow},
		{name: "Ultimate Evasion under 30 % HP", level: 55, opened: true, hp: 25, skill: skillUltimateEvasion, object: selfID},
	})

	t.Run("walks in before 52", func(t *testing.T) {
		b := rowBrain(t, classTreasureHunter, 51, "DAGGER", all...)
		b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 400}}, t0)
		if a := b.fight(npcID, at(time.Second)); a.kind == actCast && a.skill == skillLure {
			t.Fatal("fight casts Lure at 51, want no Lure before 52")
		}
	})
	t.Run("Dash to flee", func(t *testing.T) {
		wantFleeCast(t, rowBrain(t, classTreasureHunter, 52, "DAGGER", all...), skillDash)
	})
	t.Run("rests with Relax", func(t *testing.T) {
		b := rowBrain(t, classTreasureHunter, 52, "DAGGER", all...)
		wantCast(t, restDecision(t, b, 30, 100, at(time.Second)), skillRelax, selfID)
	})
}

// pwBuffsOn puts the plains walker's auras and Spirit Barrier on the bot,
// and Chameleon Rest (a toggle) when rest.
func pwBuffsOn(b *Brain, rest bool) {
	ef := []Effect{
		{SkillID: skillDefenseAura, Level: 1, Left: rowEffectDuration},
		{SkillID: skillAttackAura, Level: 1, Left: rowEffectDuration},
		{SkillID: skillSpiritBarrier, Level: 1, Left: rowEffectDuration},
	}
	if rest {
		ef = append(ef, Effect{SkillID: skillChameleonRest, Level: 1, Permanent: true})
	}
	b.world.Apply(AbnormalStatusUpdate{Effects: ef}, t0)
}

// TestRowPW checks the plains walker: Entangle opens, else Lure from 52;
// Mortal Strike on itself in combat from 58; Blinding Blow from 66, then
// Deadly Blow; Elemental Heal under 50 % HP; Sprint first when it flees.
// Out of combat below SitHP it casts Chameleon Rest from 46 and sits
// plainly at 45 or with MP under SitMP; it stands at StandHP and StandMP
// with Chameleon Rest on (standing ends the toggle, server side), and
// stays seated, regaining MP, once the toggle ended for lack of MP.
func TestRowPW(t *testing.T) {
	all := []int32{
		skillEntangle, skillLure, skillBlindingBlow, skillDeadlyBlow, skillBleed, skillMortalBlow,
		skillElementalHeal, skillUltimateEvasion, skillMortalStrike, skillSprint, skillChameleonRest,
	}
	runDaggerFights(t, classPlainsWalker, all, []daggerFight{
		{name: "Entangle opens from 400", level: 46, x: 400, skill: skillEntangle, opens: true},
		{name: "Lure from 52 while Entangle cools", level: 52, x: 400, hold: []int32{skillEntangle}, skill: skillLure, opens: true},
		{name: "no Lure at 51", level: 51, hold: []int32{skillEntangle}, skill: skillDeadlyBlow},
		{name: "Mortal Strike from 58", level: 58, opened: true, skill: skillMortalStrike, object: selfID},
		{name: "no Mortal Strike at 57", level: 57, opened: true, skill: skillDeadlyBlow},
		{name: "Blinding Blow from 66", level: 66, opened: true, hold: []int32{skillMortalStrike}, skill: skillBlindingBlow},
		{name: "no Blinding Blow at 65", level: 65, opened: true, hold: []int32{skillMortalStrike}, skill: skillDeadlyBlow},
		{name: "Elemental Heal under 50 % HP", level: 46, opened: true, hp: 45, skill: skillElementalHeal, object: selfID},
	})

	t.Run("Sprint to flee", func(t *testing.T) {
		wantFleeCast(t, rowBrain(t, classPlainsWalker, 46, "DAGGER", all...), skillSprint)
	})
	now := at(time.Second)
	for _, tt := range []struct {
		name  string
		level int32
		mp    int32
		cast  bool
	}{
		{"Chameleon Rest at 46", 46, 100, true},
		{"plain sit at 45", 45, 100, false},
		{"plain sit under SitMP", 46, 20, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classPlainsWalker, tt.level, "DAGGER", all...)
			pwBuffsOn(b, false)
			a := restDecision(t, b, 30, tt.mp, now)
			if tt.cast {
				wantCast(t, a, skillChameleonRest, selfID)
				return
			}
			if a.kind != actSitStand {
				t.Fatalf("rest = kind %d skill %d, want a plain sit", a.kind, a.skill)
			}
		})
	}
	for _, tt := range []struct {
		name   string
		on     bool
		hp, mp int32
		stand  bool
	}{
		{"seated with Chameleon Rest on", true, 60, 50, false},
		{"stands at StandHP and StandMP", true, 90, 80, true},
		{"seated once it ended for lack of MP", false, 60, 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classPlainsWalker, 46, "DAGGER", all...)
			pwBuffsOn(b, tt.on)
			b.resting, b.world.Self.Sitting = true, true
			a := restDecision(t, b, tt.hp, tt.mp, now)
			switch {
			case tt.stand && a.kind != actSitStand:
				t.Fatalf("rest = kind %d skill %d, want a stand", a.kind, a.skill)
			case !tt.stand && a.kind != 0:
				t.Fatalf("rest = kind %d skill %d, want the bot seated", a.kind, a.skill)
			}
		})
	}
}

// TestRowAW checks the abyss walker: Drain Health opens from 600 and
// ranks first under 60 % HP; Mortal Strike on
// itself from 58; Blinding Blow from 66, then Deadly Blow, Bleed only on a
// target at 50 % HP or more, Sting, Mortal Blow.
func TestRowAW(t *testing.T) {
	all := []int32{
		skillDrainHealth, skillBlindingBlow, skillDeadlyBlow, skillBleed, skillSting, skillMortalBlow,
		skillUltimateEvasion, skillMortalStrike,
	}
	cool := []int32{skillMortalStrike, skillBlindingBlow, skillDeadlyBlow, skillBleed, skillSting}
	runDaggerFights(t, classAbyssWalker, all, []daggerFight{
		{name: "Drain Health opens from 600", level: 40, x: 600, skill: skillDrainHealth, opens: true},
		{name: "Drain Health under 60 % HP first", level: 40, opened: true, hp: 50, skill: skillDrainHealth},
		{name: "Mortal Strike from 58", level: 58, opened: true, skill: skillMortalStrike, object: selfID},
		{name: "Blinding Blow from 66", level: 66, opened: true, hold: cool[:1], skill: skillBlindingBlow},
		{name: "no Blinding Blow at 65", level: 65, opened: true, hold: cool[:1], skill: skillDeadlyBlow},
		{name: "Deadly Blow", level: 66, opened: true, hold: cool[:2], skill: skillDeadlyBlow},
		{name: "Bleed on a target at 50 %", level: 66, opened: true, targetHP: 50, hold: cool[:3], skill: skillBleed},
		{name: "Sting on a target under 50 %", level: 66, opened: true, targetHP: 49, hold: cool[:3], skill: skillSting},
		{name: "Sting", level: 66, opened: true, hold: cool[:4], skill: skillSting},
		{name: "Mortal Blow", level: 66, opened: true, hold: cool, skill: skillMortalBlow},
	})
}
