package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The heavy damage second-profession rows (docs/strategy.md "Second
// professions (levels 40+)", rows GL WL DS BH): each test compiles the real
// row for its class and level and checks the row's key behavior.

// sonicFocusLevel is the Sonic Focus level a gladiator knows at level: GL's
// grants are 1 at 40, 2 at 43, 3 at 49, 4 at 55, 5 at 60, 6 at 66, 7 at 70.
func sonicFocusLevel(level int32) int32 {
	var n int32
	for _, from := range []int32{40, 43, 49, 55, 60, 66, 70} {
		if level >= from {
			n++
		}
	}
	return n
}

// glBrain is a gladiator at level wielding weapon, holding charges, knowing
// Sonic Focus at its level's grant and skills at 1, with Sonic Focus's
// maxCharges (the level's number) in its travel map.
func glBrain(t *testing.T, level int32, weapon string, charges int32, skills ...int32) *Brain {
	t.Helper()
	b := rowBrain(t, classGladiator, level, weapon)
	b.cfg.Travel.maxCharges = map[datapack.SkillRef]int{}
	for lvl := int32(1); lvl <= 7; lvl++ {
		b.cfg.Travel.maxCharges[datapack.SkillRef{ID: skillSonicFocus, Level: lvl}] = int(lvl)
	}
	known := []KnownSkill{{ID: skillSonicFocus, Level: sonicFocusLevel(level)}}
	for _, id := range skills {
		known = append(known, KnownSkill{ID: id, Level: 1})
	}
	b.world.Apply(SkillList{Skills: known}, t0)
	b.world.Apply(EtcStatusUpdate{Charges: charges}, t0)
	return b
}

// TestRowGL checks the gladiator: out of combat Sonic Focus charges to its
// level's maximum (1 at 42, 2 at 43) and stops; Sonic Blaster opens from
// 600 with 2 charges from 43, Double Sonic Slash with 3 from 49, Triple
// Sonic Slash with 4 from 55, both only with duals; Sonic Buster with two
// attackers and 2 charges, only with its Energy Stones carried, Sonic Storm
// while it cools; in combat Sonic Focus only with every attack step
// cooling and charges under the maximum, never under MPReserve; Duelist
// Spirit from 64 with duals; War Frenzy at engage from 58 above 30 % MP.
func TestRowGL(t *testing.T) {
	now := at(time.Second)
	for _, tt := range []struct {
		name           string
		level, charges int32
		want           bool
	}{
		{"no charge at 42", 42, 0, true},
		{"one charge at 42 is the most", 42, 1, false},
		{"one charge at 43", 43, 1, true},
		{"two charges at 43 are the most", 43, 2, false},
	} {
		t.Run("idle "+tt.name, func(t *testing.T) {
			b := glBrain(t, tt.level, "DUAL", tt.charges, skillTripleSlash)
			b.world.Self.InCombat = false
			a, ok := b.ruleRebuff(now)
			if ok != tt.want {
				t.Fatalf("ruleRebuff = %v kind %d skill %d, want a Sonic Focus %v", ok, a.kind, a.skill, tt.want)
			}
			if ok {
				wantCast(t, a, skillSonicFocus, selfID)
			}
		})
	}

	attacks := []int32{skillSonicBlaster, skillTripleSonicSlash, skillDoubleSonicSlash, skillSonicBuster, skillSonicStorm, skillTripleSlash}
	hold := func(skills ...int32) func(b *Brain) {
		return func(b *Brain) {
			for _, id := range skills {
				b.world.holdSkill(id, now.Add(time.Minute))
			}
		}
	}
	for _, tt := range []struct {
		name          string
		level         int32
		weapon        string
		charges       int32
		fresh         bool
		extra         []int32
		setup         func(b *Brain)
		skill, object int32
	}{
		{"blaster opens from 600 with 2 at 43", 43, "DUAL", 2, true, nil, func(b *Brain) {
			b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 600}}, t0)
		}, skillSonicBlaster, npcID},
		{"no blaster with 1 at 43", 43, "DUAL", 1, true, nil, nil, skillTripleSlash, npcID},
		{"no blaster before 43", 42, "DUAL", 2, true, nil, nil, skillTripleSlash, npcID},
		{"double sonic slash with 3 at 49", 49, "DUAL", 3, false, nil, nil, skillDoubleSonicSlash, npcID},
		{"no double sonic slash with 2", 49, "DUAL", 2, false, nil, nil, skillTripleSlash, npcID},
		{"triple sonic slash with 4 at 55", 55, "DUAL", 4, false, nil, nil, skillTripleSonicSlash, npcID},
		{"no dual step with a sword", 55, "SWORD", 4, false, nil, nil, 0, 0},
		{"buster with two attackers and 2", 43, "DUAL", 2, false, nil, hitByTwo, skillSonicBuster, npcID},
		{"no buster with one attacker", 43, "DUAL", 2, false, nil, nil, skillTripleSlash, npcID},
		{"storm while buster cools", 49, "DUAL", 2, false, nil, func(b *Brain) {
			hitByTwo(b)
			hold(skillSonicBuster)(b)
		}, skillSonicStorm, npcID},
		{"no buster without energy stones", 43, "DUAL", 2, false, nil, func(b *Brain) {
			hitByTwo(b)
			b.cfg.Travel.reagents = indexReagents(reagentSkills)
		}, skillTripleSlash, npcID},
		{"buster with energy stones", 43, "DUAL", 2, false, nil, func(b *Brain) {
			hitByTwo(b)
			b.cfg.Travel.reagents = indexReagents(reagentSkills)
			carrying(b, Item{ObjectID: rowWeapon, TemplateID: rowWeaponID, Count: 1, Equipped: true, BodySlot: partRHand}, reagentStack(energyStone, 3))
		}, skillSonicBuster, npcID},
		{"focus with every attack cooling", 43, "DUAL", 1, false, nil, hold(skillTripleSlash), skillSonicFocus, selfID},
		{"no focus at the most charges", 43, "DUAL", 2, false, nil, hold(skillTripleSlash), 0, 0},
		{"no focus under the reserve", 43, "DUAL", 1, false, nil, func(b *Brain) {
			hold(skillTripleSlash)(b)
			b.world.Self.MP = 5
		}, 0, 0},
		{"duelist spirit from 64 with duals", 64, "DUAL", 0, false, []int32{skillDuelistSpirit}, nil, skillDuelistSpirit, selfID},
		{"no duelist spirit at 63", 63, "DUAL", 0, false, []int32{skillDuelistSpirit}, nil, skillTripleSlash, npcID},
		{"no duelist spirit with a sword", 64, "SWORD", 0, false, []int32{skillDuelistSpirit}, nil, skillSonicFocus, selfID},
		{"war frenzy at engage from 58", 58, "DUAL", 0, false, []int32{skillWarFrenzy}, nil, skillWarFrenzy, selfID},
		{"no war frenzy at 57", 57, "DUAL", 0, false, []int32{skillWarFrenzy}, nil, skillTripleSlash, npcID},
		{"no war frenzy under 30 % MP", 58, "DUAL", 0, false, []int32{skillWarFrenzy}, func(b *Brain) { b.world.Self.MP = 25 }, skillTripleSlash, npcID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := glBrain(t, tt.level, tt.weapon, tt.charges, append(slices.Clone(attacks), tt.extra...)...)
			if !tt.fresh {
				b.opened = npcID
			}
			if tt.setup != nil {
				tt.setup(b)
			}
			a := fightCast(t, b, now, tt.skill, tt.object)
			if tt.skill == skillSonicFocus && a.opens {
				t.Error("the Sonic Focus filler uses up the opener")
			}
		})
	}
}

// TestRowWL checks the warlord: with a pole Thunder Storm, then Whirlwind,
// Wild Sweep only with two attackers, and no pole step with another
// weapon; Shock Stomp and Focus Attack are never in its kit; Polearm
// Accuracy on at engage from 58 only with a pole; Revival under 15 % HP
// from 55, from fight and from ruleCritical before a potion; two attackers
// at 28 % HP: it fights on (Outmatched 25) where a gladiator flees (30).
func TestRowWL(t *testing.T) {
	now := at(time.Second)
	attacks := []int32{skillThunderStorm, skillWhirlwind, skillWildSweep}
	hold := func(skills ...int32) func(b *Brain) {
		return func(b *Brain) {
			for _, id := range skills {
				b.world.holdSkill(id, now.Add(time.Minute))
			}
		}
	}
	for _, tt := range []struct {
		name          string
		level         int32
		weapon        string
		extra         []int32
		setup         func(b *Brain)
		skill, object int32
	}{
		{"thunder storm", 40, "POLE", nil, nil, skillThunderStorm, npcID},
		{"whirlwind while thunder storm cools", 40, "POLE", nil, hold(skillThunderStorm), skillWhirlwind, npcID},
		{"no wild sweep with one attacker", 40, "POLE", nil, hold(skillThunderStorm, skillWhirlwind), 0, 0},
		{"wild sweep with two attackers", 40, "POLE", nil, func(b *Brain) {
			hold(skillThunderStorm, skillWhirlwind)(b)
			hitByTwo(b)
		}, skillWildSweep, npcID},
		{"no pole step with a sword", 40, "SWORD", nil, hitByTwo, 0, 0},
		{"polearm accuracy at engage from 58", 58, "POLE", []int32{skillPolearmAccuracy}, nil, skillPolearmAccuracy, selfID},
		{"no polearm accuracy at 57", 57, "POLE", []int32{skillPolearmAccuracy}, nil, skillThunderStorm, npcID},
		{"no polearm accuracy with a sword", 58, "SWORD", []int32{skillPolearmAccuracy}, nil, 0, 0},
		{"revival under 15 % from 55", 55, "POLE", []int32{skillRevival}, func(b *Brain) { b.world.Self.HP = 10 }, skillRevival, selfID},
		{"no revival at 54", 54, "POLE", []int32{skillRevival}, func(b *Brain) { b.world.Self.HP = 10 }, skillThunderStorm, npcID},
		{"no revival at 20 %", 55, "POLE", []int32{skillRevival}, func(b *Brain) { b.world.Self.HP = 20 }, skillThunderStorm, npcID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classWarlord, tt.level, tt.weapon, append(slices.Clone(attacks), tt.extra...)...)
			if tt.setup != nil {
				tt.setup(b)
			}
			fightCast(t, b, now, tt.skill, tt.object)
		})
	}

	k := CompileBuild(RaceHuman, classWarlord, 75, RoleDamage, BranchChoice{}).Kit
	for _, dropped := range []int32{452, 317} { // Shock Stomp, Focus Attack (owner decision 4 on #624)
		if slices.Contains(k.Skills, dropped) || slices.ContainsFunc(k.Rotation, func(s Step) bool { return s.Skill == dropped }) {
			t.Errorf("the warlord kit has skill %d", dropped)
		}
	}

	t.Run("revival before a potion", func(t *testing.T) {
		b := rowBrain(t, classWarlord, 55, "POLE", skillRevival)
		carrying(b, Item{ObjectID: rowWeapon, TemplateID: rowWeaponID, Count: 1, Equipped: true, BodySlot: partRHand}, potionsOf(20))
		b.world.Self.HP = 10
		b.world.Apply(Attack{AttackerID: npcID, Pos: Point{X: 30}, Hits: []AttackHit{{TargetID: selfID, Damage: 10}}}, t0)
		rule, a := b.decide(now)
		if rule != "critical" {
			t.Fatalf("decide = %q kind %d skill %d, want critical", rule, a.kind, a.skill)
		}
		wantCast(t, a, skillRevival, selfID)
	})

	for _, tt := range []struct {
		name  string
		class int32
		flee  bool
	}{
		{"warlord fights on", classWarlord, false},
		{"gladiator flees", classGladiator, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, tt.class, 40, "POLE")
			b.world.Self.HP = 28
			hitByTwo(b)
			onGround(t, b)
			rule, a := b.decide(now)
			if rule != "outnumbered" || (a.kind == actMove) != tt.flee {
				t.Errorf("decide = %q kind %d, want outnumbered with a flee %v", rule, a.kind, tt.flee)
			}
		})
	}
}

// TestToggleWeapons checks a toggle with a weapon gate (Toggle.Weapons, the
// warlord's Polearm Accuracy at 58): with a pole it goes on at engage;
// with another weapon it stays off, and one left on goes off.
func TestToggleWeapons(t *testing.T) {
	now := at(time.Second)
	for _, tt := range []struct {
		name   string
		weapon string
		on     bool
		skill  int32
		object int32
	}{
		{"pole, off: on", "POLE", false, skillPolearmAccuracy, selfID},
		{"pole, on: stays on", "POLE", true, skillThunderStorm, npcID},
		{"sword, off: stays off", "SWORD", false, 0, 0},
		{"sword, on: off", "SWORD", true, skillPolearmAccuracy, selfID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classWarlord, 58, tt.weapon, skillPolearmAccuracy, skillThunderStorm)
			if tt.on {
				togglesOn(b, skillPolearmAccuracy)
			}
			fightCast(t, b, now, tt.skill, tt.object)
		})
	}
}

// TestRowDS checks the destroyer: with a two-handed blunt Crush of Doom
// from 60, then Hammer Crush, Fatal Strike, Stun Attack and Power Smash as
// each cools; with a two-handed sword never Hammer Crush or Stun Attack;
// Frenzy under 30 % HP, Guts only while Frenzy cools and is not on the bot;
// Zealot from 58 under 30 % HP in combat.
func TestRowDS(t *testing.T) {
	now := at(time.Second)
	attacks := []int32{skillCrushOfDoom, skillHammerCrush, skillFatalStrike, skillStunAttack, skillPowerSmash}
	for _, tt := range []struct {
		name   string
		level  int32
		weapon string
		order  []int32
	}{
		{"two-handed blunt", 60, "BIGBLUNT", attacks},
		{"two-handed blunt at 59", 59, "BIGBLUNT", attacks[1:]},
		{"two-handed sword", 60, "BIGSWORD", []int32{skillCrushOfDoom, skillFatalStrike, skillPowerSmash}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classDestroyer, tt.level, tt.weapon, attacks...)
			for _, skill := range tt.order {
				fightCast(t, b, now, skill, npcID)
				b.world.holdSkill(skill, now.Add(time.Minute))
			}
			fightCast(t, b, now, 0, 0)
		})
	}

	for _, tt := range []struct {
		name          string
		level, hp     int32
		known         []int32
		frenzyCooling bool
		frenzyOn      bool
		skill, object int32
	}{
		{"frenzy under 30 %", 50, 25, []int32{skillFrenzy, skillGuts}, false, false, skillFrenzy, selfID},
		{"guts while frenzy cools", 50, 25, []int32{skillFrenzy, skillGuts}, true, false, skillGuts, selfID},
		{"no guts over frenzy", 50, 25, []int32{skillFrenzy, skillGuts}, true, true, skillHammerCrush, npcID},
		{"no frenzy at 30 %", 50, 30, []int32{skillFrenzy, skillGuts}, false, false, skillHammerCrush, npcID},
		{"zealot from 58 under 30 %", 58, 25, []int32{skillZealot}, false, false, skillZealot, selfID},
		{"no zealot at 57", 57, 25, []int32{skillZealot}, false, false, skillHammerCrush, npcID},
		{"no zealot at 50 %", 58, 50, []int32{skillZealot}, false, false, skillHammerCrush, npcID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classDestroyer, tt.level, "BIGBLUNT", append(slices.Clone(attacks), tt.known...)...)
			b.cfg.Travel.skillStacks = stackTravel().skillStacks
			b.world.Self.HP = tt.hp
			if tt.frenzyCooling {
				b.world.holdSkill(skillFrenzy, now.Add(time.Minute))
			}
			if tt.frenzyOn {
				effectOn(b, skillFrenzy)
			}
			fightCast(t, b, now, tt.skill, tt.object)
		})
	}
}

// TestRowBH checks the bounty hunter's fight past the Spoil opener
// (TestSpoilOpensThenSweeps covers Spoil and Sweeper): with a blunt Hammer
// Crush, then Stun Attack while it cools; War Frenzy at engage from 58.
func TestRowBH(t *testing.T) {
	now := at(time.Second)
	all := []int32{skillSpoil, skillHammerCrush, skillStunAttack, skillSweeper}
	b := rowBrain(t, classBountyHunter, 40, "BLUNT", all...)
	fightCast(t, b, now, skillSpoil, npcID)
	b.world.holdSkill(skillSpoil, now.Add(time.Minute))
	fightCast(t, b, now, skillHammerCrush, npcID)
	b.world.holdSkill(skillHammerCrush, now.Add(time.Minute))
	fightCast(t, b, now, skillStunAttack, npcID)

	for _, tt := range []struct {
		level int32
		skill int32
	}{{58, skillWarFrenzy}, {57, skillSpoil}} {
		b := rowBrain(t, classBountyHunter, tt.level, "BLUNT", append(slices.Clone(all), skillWarFrenzy)...)
		obj := npcID
		if tt.skill == skillWarFrenzy {
			obj = selfID
		}
		fightCast(t, b, now, tt.skill, obj)
	}
}
