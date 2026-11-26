package botclient

import (
	"testing"
	"time"
)

// effectsOn puts effects of skills on the bot, rowEffectDuration left on
// each; an update replaces the bot's whole effect list.
func effectsOn(b *Brain, skills ...int32) {
	ef := make([]Effect, len(skills))
	for i, id := range skills {
		ef[i] = Effect{SkillID: id, Level: 1, Left: rowEffectDuration}
	}
	b.world.Apply(AbnormalStatusUpdate{Effects: ef}, t0)
}

// wantNoRebuff fails if ruleRebuff decides a cast at now.
func wantNoRebuff(t *testing.T, b *Brain, now time.Time) {
	t.Helper()
	if a, ok := b.ruleRebuff(now); ok {
		t.Fatalf("ruleRebuff = kind %d skill %d, want no cast", a.kind, a.skill)
	}
}

// TestRowSS checks the sword singer: out of combat at 49 it sings Warding,
// Wind and Hunter, one per decision, each on itself, none while all three
// are on with more than RebuffBefore left, and Earth only from 55; under
// 30 % MP (MPReserve) no attack step goes out, but a missing song does;
// in a party Spirit Barrier (SELF) is never cast on a member; Aggression
// opens and Power Strike follows with a sword.
func TestRowSS(t *testing.T) {
	now := at(time.Second)
	songs := []int32{skillSongOfWarding, skillSongOfWind, skillSongOfHunter, skillSongOfEarth, skillSongOfVitality}
	all := append([]int32{skillAggression, skillEntangle, skillPowerStrike, skillDefenseAura, skillAttackAura, skillSpiritBarrier}, songs...)
	selfBuffs := []int32{skillDefenseAura, skillAttackAura, skillSpiritBarrier}
	idle := func(level int32) *Brain {
		b := rowBrain(t, classSwordSinger, level, "SWORD", all...)
		b.world.Self.InCombat = false
		return b
	}

	b := idle(49)
	var on []int32
	for _, song := range songs[:3] {
		effectsOn(b, append(selfBuffs, on...)...)
		a, ok := b.ruleRebuff(now)
		if !ok {
			t.Fatalf("ruleRebuff casts nothing, want %d", song)
		}
		wantCast(t, a, song, selfID)
		on = append(on, song)
	}
	effectsOn(b, append(selfBuffs, on...)...)
	wantNoRebuff(t, b, now)

	b = idle(55)
	effectsOn(b, append(selfBuffs, songs[:3]...)...)
	a, _ := b.ruleRebuff(now)
	wantCast(t, a, skillSongOfEarth, selfID)

	b = idle(49)
	effectsOn(b, selfBuffs...)
	b.world.Self.MP, b.world.Self.MaxMP = 80, 300
	a, _ = b.ruleRebuff(now)
	wantCast(t, a, skillSongOfWarding, selfID)
	b.world.Self.InCombat = true
	fightCast(t, b, now, 0, 0)

	b = idle(49)
	inParty(b, 100, 100)
	b.world.Target = pcID
	effectsOn(b, append(selfBuffs, songs[:3]...)...)
	for range 2 * len(all) {
		a, ok := b.ruleRebuff(now)
		if !ok {
			break
		}
		if a.skill == skillSpiritBarrier && a.object != selfID {
			t.Fatalf("ruleRebuff casts Spirit Barrier on %#x, want it on the bot only", a.object)
		}
		b.buffSent(a, now)
	}

	b = rowBrain(t, classSwordSinger, 49, "SWORD", all...)
	if a := fightCast(t, b, now, skillAggression, npcID); !a.opens {
		t.Error("Aggression does not use the opener up")
	}
	b.opened = npcID
	fightCast(t, b, now, skillPowerStrike, npcID)
}

// TestRowBD checks the bladedancer: out of combat Dance of Fire with duals
// worn, no dance with a sword; Drain Health opens from 600; the blade
// dances only from 55, with two attackers and duals; Drain Health under
// 60 % HP ranks first.
func TestRowBD(t *testing.T) {
	now := at(time.Second)
	dances := []int32{skillDanceOfFire, skillDanceOfInspiration, skillDanceOfWarrior, skillDanceOfFury, skillDanceOfVampire}
	attacks := []int32{skillDrainHealth, skillPowerBreak, skillPoisonBladeDance, skillDemonicBladeDance, skillSting, skillFreezingStrike, skillUltimateDefense}
	for _, tt := range []struct {
		weapon string
		cast   bool
	}{{"DUAL", true}, {"SWORD", false}} {
		t.Run("dance with "+tt.weapon, func(t *testing.T) {
			b := rowBrain(t, classBladedancer, 49, tt.weapon, append([]int32{skillDefenseAura, skillAttackAura}, dances...)...)
			b.world.Self.InCombat = false
			effectsOn(b, skillDefenseAura, skillAttackAura)
			if !tt.cast {
				wantNoRebuff(t, b, now)
				return
			}
			a, _ := b.ruleRebuff(now)
			wantCast(t, a, skillDanceOfFire, selfID)
		})
	}

	b := rowBrain(t, classBladedancer, 40, "DUAL", attacks...)
	b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 600}}, t0)
	if a := fightCast(t, b, now, skillDrainHealth, npcID); !a.opens {
		t.Error("Drain Health does not use the opener up")
	}

	for _, tt := range []struct {
		name   string
		level  int32
		weapon string
		two    bool
		hp     int32
		skill  int32
	}{
		{"blade dance", 55, "DUAL", true, 100, skillPoisonBladeDance},
		{"before 55", 54, "DUAL", true, 100, skillSting},
		{"one attacker", 55, "DUAL", false, 100, skillSting},
		{"sword", 55, "SWORD", true, 100, skillSting},
		{"drain under 60", 55, "DUAL", true, 50, skillDrainHealth},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classBladedancer, tt.level, tt.weapon, attacks...)
			b.opened = npcID
			if tt.two {
				hitByTwo(b)
			}
			b.world.Self.HP, b.world.Self.MaxHP = tt.hp, 100
			fightCast(t, b, now, tt.skill, npcID)
		})
	}
}
