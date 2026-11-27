package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The tyrant row (docs/strategy.md "Second professions (levels 40+)", row
// TY): the test compiles the real row for its level and checks the row's
// key behavior.

// focusedForceLevel is the Focused Force level a tyrant knows at level:
// MO's 1 at 24 and 2 at 32, then TY's 3 at 40, 4 at 52, 5 at 60, 6 at 66
// and 7 at 72; each level's maxCharges is its number.
func focusedForceLevel(level int32) int32 {
	var n int32
	for _, from := range []int32{24, 32, 40, 52, 60, 66, 72} {
		if level >= from {
			n++
		}
	}
	return n
}

// tyrantAttacks are the tyrant's attack steps, known at level 1.
var tyrantAttacks = []int32{
	skillForceBlaster, skillCripple, skillForceStorm, skillForceBurst, skillHurricaneAssault,
	skillBurningFist, skillSoulBreaker, skillStunningFist, skillIronPunch,
}

// tyBrain is a tyrant at level wielding weapon, holding charges, knowing
// Focused Force at its level's grant and skills at 1, with Focused Force's
// maxCharges in its travel map.
func tyBrain(t *testing.T, level int32, weapon string, charges int32, skills ...int32) *Brain {
	t.Helper()
	b := rowBrain(t, classTyrant, level, weapon)
	b.cfg.Travel.maxCharges = map[datapack.SkillRef]int{}
	for lvl := int32(1); lvl <= 7; lvl++ {
		b.cfg.Travel.maxCharges[datapack.SkillRef{ID: skillFocusedForce, Level: lvl}] = int(lvl)
	}
	known := []KnownSkill{{ID: skillFocusedForce, Level: focusedForceLevel(level)}}
	for _, id := range skills {
		known = append(known, KnownSkill{ID: id, Level: 1})
	}
	b.world.Apply(SkillList{Skills: known}, t0)
	b.world.Apply(EtcStatusUpdate{Charges: charges}, t0)
	return b
}

// tyrantReagents are Force Burst's and Force Storm's 2 Energy Stones a
// cast (aCis_datapack skills 0000-0099.xml, itemConsumeId 5589).
var tyrantReagents = map[datapack.SkillRef]datapack.Skill{
	{ID: skillForceBurst, Level: 1}: {ItemConsumeID: energyStone, ItemConsumeCount: 2},
	{ID: skillForceStorm, Level: 1}: {ItemConsumeID: energyStone, ItemConsumeCount: 2},
}

// TestRowTY checks the tyrant: out of combat Focused Force charges to its
// level's maximum (3 at 40, 4 at 52) and stops; Force Blaster opens from
// 600 with a charge, else Cripple; Force Storm from 49 and Force Burst from
// 43 only with two attackers and a charge, and only with their Energy
// Stones carried; Hurricane Assault only with 2 charges; Burning Fist and
// Soul Breaker between it and Force Blaster with one; Stunning Fist, Iron
// Punch; in combat Focused Force only with every attack step cooling and
// charges under the maximum, never under MPReserve; Zealot from 58 under
// 30 % HP; War Frenzy at engage from 58; Puma Spirit Totem out of combat
// with fists only, Bison Spirit Totem in its place from 68.
func TestRowTY(t *testing.T) {
	now := at(time.Second)
	for _, tt := range []struct {
		name           string
		level, charges int32
		want           bool
	}{
		{"no charge at 40", 40, 0, true},
		{"two charges at 40", 40, 2, true},
		{"three charges at 40 are the most", 40, 3, false},
		{"three charges at 52", 52, 3, true},
		{"four charges at 52 are the most", 52, 4, false},
	} {
		t.Run("idle "+tt.name, func(t *testing.T) {
			b := tyBrain(t, tt.level, "DUALFIST", tt.charges, tyrantAttacks...)
			b.world.Self.InCombat = false
			a, ok := b.ruleRebuff(now)
			if ok != tt.want {
				t.Fatalf("ruleRebuff = %v kind %d skill %d, want a Focused Force %v", ok, a.kind, a.skill, tt.want)
			}
			if ok {
				wantCast(t, a, skillFocusedForce, selfID)
			}
		})
	}

	hold := func(skills ...int32) func(b *Brain) {
		return func(b *Brain) {
			for _, id := range skills {
				b.world.holdSkill(id, now.Add(time.Minute))
			}
		}
	}
	both := func(fs ...func(b *Brain)) func(b *Brain) {
		return func(b *Brain) {
			for _, f := range fs {
				f(b)
			}
		}
	}
	allCooling := hold(tyrantAttacks...)
	for _, tt := range []struct {
		name           string
		level, charges int32
		weapon         string
		fresh          bool
		extra          []int32
		setup          func(b *Brain)
		skill, object  int32
	}{
		{"blaster opens from 600 with a charge", 40, 1, "DUALFIST", true, nil, func(b *Brain) {
			b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 600}}, t0)
		}, skillForceBlaster, npcID},
		{"no charge opens with cripple", 40, 0, "DUALFIST", true, nil, nil, skillCripple, npcID},
		{"storm from 49 with two attackers and a charge", 49, 1, "DUALFIST", false, nil, hitByTwo, skillForceStorm, npcID},
		{"burst while storm cools", 49, 1, "DUALFIST", false, nil, both(hitByTwo, hold(skillForceStorm)), skillForceBurst, npcID},
		{"burst from 43 before storm", 43, 1, "DUALFIST", false, nil, hitByTwo, skillForceBurst, npcID},
		{"no burst at 42", 42, 1, "DUALFIST", false, nil, hitByTwo, skillBurningFist, npcID},
		{"no storm or burst with one attacker", 49, 1, "DUALFIST", false, nil, nil, skillBurningFist, npcID},
		{"no storm or burst without a charge", 49, 0, "DUALFIST", false, nil, hitByTwo, skillBurningFist, npcID},
		{"no burst without energy stones", 43, 1, "DUALFIST", false, nil, func(b *Brain) {
			hitByTwo(b)
			b.cfg.Travel.reagents = indexReagents(tyrantReagents)
		}, skillBurningFist, npcID},
		{"burst with energy stones", 43, 1, "DUALFIST", false, nil, func(b *Brain) {
			hitByTwo(b)
			b.cfg.Travel.reagents = indexReagents(tyrantReagents)
			carrying(b, Item{ObjectID: rowWeapon, TemplateID: rowWeaponID, Count: 1, Equipped: true, BodySlot: partRHand}, reagentStack(energyStone, 2))
		}, skillForceBurst, npcID},
		{"hurricane assault with 2", 40, 2, "DUALFIST", false, nil, nil, skillHurricaneAssault, npcID},
		{"no hurricane assault with 1", 40, 1, "DUALFIST", false, nil, nil, skillBurningFist, npcID},
		{"soul breaker while burning fist cools", 40, 1, "DUALFIST", false, nil, hold(skillBurningFist), skillSoulBreaker, npcID},
		{"blaster with one after them", 40, 1, "DUALFIST", false, nil, hold(skillBurningFist, skillSoulBreaker), skillForceBlaster, npcID},
		{"stunning fist without a charge", 40, 0, "DUALFIST", false, nil, hold(skillBurningFist, skillSoulBreaker), skillStunningFist, npcID},
		{"iron punch while stunning fist cools", 40, 0, "DUALFIST", false, nil, hold(skillBurningFist, skillSoulBreaker, skillStunningFist), skillIronPunch, npcID},
		{"focus with every attack cooling", 40, 2, "DUALFIST", false, nil, allCooling, skillFocusedForce, selfID},
		{"no focus at the most charges", 40, 3, "DUALFIST", false, nil, allCooling, 0, 0},
		{"no focus under the reserve", 40, 2, "DUALFIST", false, nil, both(allCooling, func(b *Brain) { b.world.Self.MP = 5 }), 0, 0},
		{"no fist step with a sword", 49, 2, "SWORD", false, nil, hitByTwo, 0, 0},
		{"zealot from 58 under 30 %", 58, 0, "DUALFIST", false, []int32{skillZealot}, func(b *Brain) { b.world.Self.HP = 25 }, skillZealot, selfID},
		{"no zealot at 57", 57, 0, "DUALFIST", false, []int32{skillZealot}, func(b *Brain) { b.world.Self.HP = 25 }, skillBurningFist, npcID},
		{"no zealot at 50 %", 58, 0, "DUALFIST", false, []int32{skillZealot}, func(b *Brain) { b.world.Self.HP = 50 }, skillBurningFist, npcID},
		{"war frenzy at engage from 58", 58, 0, "DUALFIST", false, []int32{skillWarFrenzy}, nil, skillWarFrenzy, selfID},
		{"no war frenzy at 57", 57, 0, "DUALFIST", false, []int32{skillWarFrenzy}, nil, skillBurningFist, npcID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := tyBrain(t, tt.level, tt.weapon, tt.charges, append(slices.Clone(tyrantAttacks), tt.extra...)...)
			if !tt.fresh {
				b.opened = npcID
			}
			if tt.setup != nil {
				tt.setup(b)
			}
			a := fightCast(t, b, now, tt.skill, tt.object)
			if tt.skill == skillFocusedForce && a.opens {
				t.Error("the Focused Force filler uses up the opener")
			}
		})
	}

	for _, tt := range []struct {
		name     string
		level    int32
		weapon   string
		inCombat bool
		skill    int32
	}{
		{"puma out of combat", 40, "DUALFIST", false, skillPumaTotem},
		{"no puma with a sword", 40, "SWORD", false, 0},
		{"no puma in combat", 40, "DUALFIST", true, 0},
		{"bison in its place from 68", 68, "DUALFIST", false, skillBisonTotem},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// At its most charges, so Focused Force does not go first.
			b := tyBrain(t, tt.level, tt.weapon, focusedForceLevel(tt.level), skillPumaTotem, skillBisonTotem)
			b.world.Self.InCombat = tt.inCombat
			b.world.Target = selfID // the select before a buff is castOn's
			a, ok := b.ruleRebuff(now)
			if ok != (tt.skill != 0) {
				t.Fatalf("ruleRebuff = %v kind %d skill %d, want a cast of %d", ok, a.kind, a.skill, tt.skill)
			}
			if ok {
				wantCast(t, a, tt.skill, selfID)
			}
		})
	}
}

// TestCompileTyrantBuild checks the tyrant's compiled row: Puma Spirit
// Totem until 67 and only Bison Spirit Totem from 68, never MO's Wolf or
// Bear Spirit Totem; none of the skills the plan leaves out (Punch of Doom,
// Fist Fury, Break Duress).
func TestCompileTyrantBuild(t *testing.T) {
	for _, tt := range []struct {
		level int32
		want  []int32
	}{{40, []int32{skillPumaTotem}}, {67, []int32{skillPumaTotem}}, {68, []int32{skillBisonTotem}}, {75, []int32{skillBisonTotem}}} {
		k := CompileBuild(RaceOrc, classTyrant, tt.level, RoleDamage, BranchChoice{}).Kit
		var got []int32
		for _, bf := range k.Buffs {
			got = append(got, bf.Skill)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("tyrant at %d buffs %v, want %v", tt.level, got, tt.want)
		}
		for _, never := range []int32{skillWolfTotem, skillBearTotem, 81, 222, 461} {
			if slices.Contains(k.Skills, never) || slices.Contains(stepSkills(k.Rotation), never) || slices.Contains(got, never) {
				t.Errorf("tyrant at %d has skill %d, want none", tt.level, never)
			}
		}
	}
}
