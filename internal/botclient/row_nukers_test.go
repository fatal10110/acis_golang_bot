package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The nuker and warcryer rows (docs/strategy.md "Second professions (levels
// 40+)", rows SN SG SH WC; #634): each test compiles the real row for its
// class, level and role and checks the row's key behavior.

// manaRegenOre is the Spirit Ore Mana Regeneration level 1 consumes
// (data/xml/skills 1000-1099.xml, itemConsumeCount 7).
const manaRegenOre = 7

// nukerReagents gives the bot's travel map Death Spike's Cursed Bone and
// Mana Regeneration's Spirit Ore (#627), and makes items its whole
// inventory.
func nukerReagents(b *Brain, items ...Item) {
	b.cfg.Travel.reagents = indexReagents(map[datapack.SkillRef]datapack.Skill{
		{ID: skillDeathSpike, Level: 1}:       {ItemConsumeID: cursedBone, ItemConsumeCount: 1},
		{ID: skillManaRegeneration, Level: 1}: {ItemConsumeID: spiritOre, ItemConsumeCount: manaRegenOre},
	})
	carrying(b, items...)
}

// idleNuker is mysticBrain out of combat with itself selected (a self
// buff selects the bot first).
func idleNuker(t *testing.T, class, level int32, skills ...int32) *Brain {
	t.Helper()
	b := mysticBrain(t, class, level, RoleDamage, "", skills...)
	b.world.Self.InCombat, b.world.Target = false, selfID
	return b
}

// nukeSteps chains setups.
func nukeSteps(fs ...func(b *Brain, now time.Time)) func(b *Brain, now time.Time) {
	return func(b *Brain, now time.Time) {
		for _, f := range fs {
			f(b, now)
		}
	}
}

// nukeOpened marks npcID engaged by an attack that went out; nukeHolding
// keeps skills on reuse; nukeTwo has a second monster hit the bot too.
func nukeOpened(b *Brain, _ time.Time) { b.opened = npcID }

func nukeHolding(skills ...int32) func(b *Brain, now time.Time) {
	return func(b *Brain, now time.Time) { hold(b, now, skills...) }
}

func nukeTwo(b *Brain, _ time.Time) { hitByTwo(b) }

// at900 puts npcID 900 away and gives skill its castRange 900.
func at900(skill int32) func(b *Brain, now time.Time) {
	return func(b *Brain, _ time.Time) {
		moveNPC(b, 900)
		castRange(b, skill, 900)
	}
}

// wantCritical fails unless decide at now is ruleCritical's cast of skill
// on npcID, the bot at hp % HP under an attack and carrying potions.
func wantCritical(t *testing.T, b *Brain, hp int32, now time.Time, skill int32) {
	t.Helper()
	carrying(b, potionsOf(20))
	b.world.Self.HP = hp
	b.world.Apply(Attack{AttackerID: npcID, Pos: Point{X: 30}, Hits: []AttackHit{{TargetID: selfID, Damage: 10}}}, t0)
	rule, a := b.decide(now)
	if rule != "critical" {
		t.Fatalf("decide = %q kind %d skill %d, want critical", rule, a.kind, a.skill)
	}
	wantCast(t, a, skill, npcID)
}

// TestRowSN checks the sorcerer: Prominence opens from 900; Surrender To
// Fire only on a target at 70 % HP or more with MP above 50 %, also after
// Prominence went out, and once per target; Rain of Fire from 58 only with
// two attackers; Aura Bolt only within 400, Aura Flare only within 150;
// Blaze and Ice Bolt only while the others cool; Vampiric Touch under 60 %
// HP; Blazing Skin on itself out of combat.
func TestRowSN(t *testing.T) {
	all := []int32{
		skillProminence, skillSurrenderToFire, skillRainOfFire, skillAuraBolt, skillAuraFlare, skillBlaze, skillIceBolt,
		skillVampiricTouch, skillBodyToMind,
	}
	sn := func(name string, level int32, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classSorcerer, level: level, role: RoleDamage, skills: all, setup: setup, skill: skill, object: object}
	}
	targetHP := func(pct int32) func(b *Brain, now time.Time) {
		return func(b *Brain, _ time.Time) { b.world.Objects[npcID].HP, b.world.Objects[npcID].MaxHP = pct, 100 }
	}
	cool := nukeHolding(skillProminence, skillSurrenderToFire)
	runMysticCases(t, "", []mysticCase{
		sn("prominence from 900", 40, at900(skillProminence), skillProminence, npcID),
		sn("surrender to fire after prominence went out", 40, nukeSteps(nukeOpened, nukeHolding(skillProminence), targetHP(70)), skillSurrenderToFire, npcID),
		sn("no surrender to fire under 70 % target HP", 40, nukeSteps(nukeOpened, nukeHolding(skillProminence), targetHP(69)), skillAuraBolt, npcID),
		sn("no surrender to fire at 50 % MP", 40, nukeSteps(nukeOpened, nukeHolding(skillProminence), mp(50)), skillAuraBolt, npcID),
		sn("rain of fire from 58 with two attackers", 58, nukeSteps(cool, nukeTwo), skillRainOfFire, npcID),
		sn("no rain of fire with one attacker", 58, cool, skillAuraBolt, npcID),
		sn("no rain of fire at 57", 57, nukeSteps(cool, nukeTwo), skillAuraBolt, npcID),
		sn("aura bolt within 400", 40, nukeSteps(cool, func(b *Brain, _ time.Time) { moveNPC(b, 400) }), skillAuraBolt, npcID),
		sn("no aura bolt beyond 400", 40, nukeSteps(cool, func(b *Brain, _ time.Time) { moveNPC(b, 401) }), skillBlaze, npcID),
		sn("aura flare within 150", 40, nukeSteps(cool, nukeHolding(skillAuraBolt), func(b *Brain, _ time.Time) { moveNPC(b, 150) }), skillAuraFlare, npcID),
		sn("no aura flare beyond 150", 40, nukeSteps(cool, nukeHolding(skillAuraBolt), func(b *Brain, _ time.Time) { moveNPC(b, 151) }), skillBlaze, npcID),
		sn("blaze while the others cool", 40, nukeSteps(cool, nukeHolding(skillAuraBolt, skillAuraFlare)), skillBlaze, npcID),
		sn("ice bolt while blaze cools", 40, nukeSteps(cool, nukeHolding(skillAuraBolt, skillAuraFlare, skillBlaze)), skillIceBolt, npcID),
		sn("vampiric touch under 60 % HP", 40, hp(55), skillVampiricTouch, npcID),
		sn("no vampiric touch at 60 % HP", 40, hp(60), skillProminence, npcID),
		sn("body to mind", 40, func(b *Brain, _ time.Time) { b.world.Self.MP, b.world.Self.HP = 20, 80 }, skillBodyToMind, selfID),
		sn("melee under 10 % MP at 70 % HP (no body to mind)", 40, func(b *Brain, _ time.Time) { b.world.Self.MP, b.world.Self.HP = 5, 70 }, 0, 0),
	})

	now := at(time.Second)
	t.Run("surrender to fire once per target", func(t *testing.T) {
		b := mysticBrain(t, classSorcerer, 40, RoleDamage, "", all...)
		b.opened = npcID
		hold(b, now, skillProminence)
		a := fightCast(t, b, now, skillSurrenderToFire, npcID)
		if ok, err := b.send(a, now); err != nil || !ok {
			t.Fatalf("send = %v %v", ok, err)
		}
		b.handleOne(t, MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: skillSurrenderToFire, Level: 1}, now.Add(100*time.Millisecond))
		later := now.Add(time.Minute)
		hold(b, later, skillProminence)
		fightCast(t, b, later, skillAuraBolt, npcID)
	})
	t.Run("blazing skin on itself out of combat", func(t *testing.T) {
		b := idleNuker(t, classSorcerer, 40, skillConcentration, skillBlazingSkin)
		effectsOn(b, skillConcentration)
		a, _ := b.ruleRebuff(now)
		wantCast(t, a, skillBlazingSkin, selfID)
	})
}

// TestRowSG checks the spellsinger: Wind Shackle opens above 50 % MP, else
// Hydro Blast from 900; Ice Dagger from 44; Aqua Splash from 58 with two
// attackers; Self Heal under 40 % HP; Mana Regeneration out of combat with
// its Spirit Ore carried.
func TestRowSG(t *testing.T) {
	all := []int32{
		skillWindShackle, skillHydroBlast, skillIceDagger, skillSolarFlare, skillAquaSplash, skillAuraBolt, skillAuraFlare,
		skillSolarSpark, skillAquaSwirl, skillSelfHeal,
	}
	sg := func(name string, level int32, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classSpellsinger, level: level, role: RoleDamage, skills: all, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "", []mysticCase{
		sg("wind shackle opens", 40, nil, skillWindShackle, npcID),
		sg("hydro blast from 900 at 50 % MP", 40, nukeSteps(at900(skillHydroBlast), mp(50)), skillHydroBlast, npcID),
		sg("hydro blast once opened", 40, nukeOpened, skillHydroBlast, npcID),
		sg("ice dagger from 44", 44, nukeSteps(nukeOpened, nukeHolding(skillHydroBlast)), skillIceDagger, npcID),
		sg("solar flare before 44", 43, nukeSteps(nukeOpened, nukeHolding(skillHydroBlast)), skillSolarFlare, npcID),
		sg("aqua splash from 58 with two attackers", 58, nukeSteps(nukeOpened, nukeHolding(skillHydroBlast, skillIceDagger, skillSolarFlare), nukeTwo), skillAquaSplash, npcID),
		sg("no aqua splash with one attacker", 58, nukeSteps(nukeOpened, nukeHolding(skillHydroBlast, skillIceDagger, skillSolarFlare)), skillAuraBolt, npcID),
		sg("solar spark while the others cool", 40, nukeSteps(nukeOpened, nukeHolding(skillHydroBlast, skillSolarFlare, skillAuraBolt, skillAuraFlare)), skillSolarSpark, npcID),
		sg("self heal under 40 % HP", 40, hp(30), skillSelfHeal, selfID),
	})

	now := at(time.Second)
	for _, tt := range []struct {
		name string
		ore  int32
		cast bool
	}{{"mana regeneration with its spirit ore", manaRegenOre, true}, {"no mana regeneration short of spirit ore", manaRegenOre - 1, false}} {
		t.Run(tt.name, func(t *testing.T) {
			b := idleNuker(t, classSpellsinger, 40, skillConcentration, skillManaRegeneration, skillFreezingSkin)
			nukerReagents(b, reagentStack(spiritOre, tt.ore))
			effectsOn(b, skillConcentration, skillFreezingSkin)
			if !tt.cast {
				wantNoRebuff(t, b, now)
				return
			}
			a, _ := b.ruleRebuff(now)
			wantCast(t, a, skillManaRegeneration, selfID)
		})
	}
}

// TestRowSH checks the spellhowler: Curse: Poison opens above 60 % MP,
// else Hurricane from 900; Death Spike from 44 with a Cursed Bone carried;
// Tempest from 48 with two attackers; Vampiric Claw under 70 % HP before
// Vampiric Touch under 60 %, in fight and from ruleCritical before a
// potion.
func TestRowSH(t *testing.T) {
	all := []int32{
		skillCursePoison, skillHurricane, skillDeathSpike, skillShadowFlare, skillTempest, skillShadowSpark, skillTwister,
		skillAuraBurn, skillVampiricClaw, skillVampiricTouch, skillBodyToMind,
	}
	sh := func(name string, level int32, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classSpellhowler, level: level, role: RoleDamage, skills: all, setup: setup, skill: skill, object: object}
	}
	bone := func(n int32) func(b *Brain, now time.Time) {
		return func(b *Brain, _ time.Time) {
			if n == 0 {
				nukerReagents(b)
				return
			}
			nukerReagents(b, reagentStack(cursedBone, n))
		}
	}
	runMysticCases(t, "", []mysticCase{
		sh("curse: poison opens", 40, nil, skillCursePoison, npcID),
		sh("hurricane from 900 at 60 % MP", 40, nukeSteps(at900(skillHurricane), mp(60)), skillHurricane, npcID),
		sh("death spike from 44 with a cursed bone", 44, nukeSteps(nukeOpened, nukeHolding(skillHurricane), bone(1)), skillDeathSpike, npcID),
		sh("no death spike without a cursed bone", 44, nukeSteps(nukeOpened, nukeHolding(skillHurricane), bone(0)), skillShadowFlare, npcID),
		sh("no death spike at 43", 43, nukeSteps(nukeOpened, nukeHolding(skillHurricane), bone(1)), skillShadowFlare, npcID),
		sh("tempest from 48 with two attackers", 48, nukeSteps(nukeOpened, nukeHolding(skillHurricane, skillDeathSpike, skillShadowFlare), nukeTwo), skillTempest, npcID),
		sh("no tempest with one attacker", 48, nukeSteps(nukeOpened, nukeHolding(skillHurricane, skillDeathSpike, skillShadowFlare)), skillShadowSpark, npcID),
		sh("vampiric claw under 70 % HP", 40, hp(65), skillVampiricClaw, npcID),
		sh("vampiric claw before vampiric touch", 40, hp(55), skillVampiricClaw, npcID),
		sh("vampiric touch under 60 % while vampiric claw cools", 40, nukeSteps(hp(55), nukeHolding(skillVampiricClaw)), skillVampiricTouch, npcID),
		sh("no vampiric touch at 65 % while vampiric claw cools", 40, nukeSteps(hp(65), nukeHolding(skillVampiricClaw)), skillCursePoison, npcID),
	})

	now := at(10 * time.Second)
	t.Run("vampiric claw from ruleCritical before a potion", func(t *testing.T) {
		wantCritical(t, mysticBrain(t, classSpellhowler, 40, RoleDamage, "", all...), 30, now, skillVampiricClaw)
	})
	t.Run("vampiric touch from ruleCritical while vampiric claw cools", func(t *testing.T) {
		b := mysticBrain(t, classSpellhowler, 40, RoleDamage, "", all...)
		hold(b, now, skillVampiricClaw)
		wantCritical(t, b, 30, now, skillVampiricTouch)
	})
}

// TestRowWC checks the warcryer: Steal Essence opens from 900 and drains
// under 70 % HP, Life Drain while it cools; Frost Flame, then with a blunt
// Hammer Crush, then Stun Attack, at any distance (they melee anyway),
// else melee; the Support role stops attack steps at 50 % MP, the Damage
// role at 30 %; Soul Cry on at engage above 30 % MP; Chant of Life with a
// member under 50 %; Chant of Vampire from 44, Chant of Fury from 48 and
// War Chant from 58 out of combat; Steal Essence from ruleCritical before
// a potion.
func TestRowWC(t *testing.T) {
	all := []int32{skillStealEssence, skillFrostFlame, skillHammerCrush, skillStunAttack, skillLifeDrain}
	wc := func(name string, role Role, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classWarcryer, level: 40, role: role, skills: all, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "BLUNT", []mysticCase{
		wc("steal essence opens from 900", RoleDamage, at900(skillStealEssence), skillStealEssence, npcID),
		wc("steal essence under 70 % HP", RoleDamage, nukeSteps(nukeOpened, hp(65)), skillStealEssence, npcID),
		wc("life drain under 70 % while steal essence cools", RoleDamage, nukeSteps(nukeOpened, hp(65), nukeHolding(skillStealEssence)), skillLifeDrain, npcID),
		wc("frost flame once opened", RoleDamage, nukeOpened, skillFrostFlame, npcID),
		wc("hammer crush with a blunt", RoleDamage, nukeSteps(nukeOpened, nukeHolding(skillFrostFlame)), skillHammerCrush, npcID),
		wc("hammer crush beyond 40", RoleDamage, nukeSteps(nukeOpened, nukeHolding(skillFrostFlame), func(b *Brain, _ time.Time) { moveNPC(b, 60) }), skillHammerCrush, npcID),
		wc("stun attack while hammer crush cools", RoleDamage, nukeSteps(nukeOpened, nukeHolding(skillFrostFlame, skillHammerCrush)), skillStunAttack, npcID),
		wc("damage at 30 % MP", RoleDamage, nukeSteps(nukeOpened, mp(30)), skillFrostFlame, npcID),
		wc("damage under 30 % MP melees", RoleDamage, nukeSteps(nukeOpened, mp(25)), 0, 0),
		wc("support at 50 % MP", RoleSupport, nukeSteps(nukeOpened, mp(50)), skillFrostFlame, npcID),
		wc("support under 50 % MP melees", RoleSupport, nukeSteps(nukeOpened, mp(45)), 0, 0),
		{
			name: "soul cry at engage above 30 % MP", class: classWarcryer, level: 40, role: RoleDamage, skills: append([]int32{skillSoulCry}, all...),
			setup: mp(35), skill: skillSoulCry, object: selfID,
		},
		{
			name: "no soul cry at 30 % MP", class: classWarcryer, level: 40, role: RoleDamage, skills: append([]int32{skillSoulCry}, all...),
			setup: mp(30), skill: skillStealEssence, object: npcID,
		},
	})
	runMysticCases(t, "", []mysticCase{
		wc("melee bare-handed", RoleDamage, nukeSteps(nukeOpened, nukeHolding(skillFrostFlame)), 0, 0),
	})

	now := at(10 * time.Second)
	for _, tt := range []struct {
		leaderHP int32
		cast     bool
	}{{45, true}, {50, false}} {
		b := mysticBrain(t, classWarcryer, 40, RoleSupport, "", skillChantLife)
		inParty(b, tt.leaderHP, 100)
		a, ok := b.ruleHeal(now)
		if ok != tt.cast {
			t.Fatalf("leader at %d: ruleHeal = kind %d skill %d %v, want a cast %v", tt.leaderHP, a.kind, a.skill, ok, tt.cast)
		}
		if tt.cast {
			wantCast(t, a, skillChantLife, selfID)
		}
	}

	chants := []int32{skillChantBattle, skillChantShielding, skillChantFire, skillFlameChant, skillSoulShield}
	for _, tt := range []struct {
		name  string
		level int32
		on    []int32
		skill int32
	}{
		{"chant of vampire from 44", 44, nil, skillChantVampire},
		{"no chant of vampire at 43", 43, nil, 0},
		{"chant of fury from 48", 48, []int32{skillChantVampire}, skillChantFury},
		{"no chant of fury at 47", 47, []int32{skillChantVampire}, 0},
		{"war chant from 58", 58, []int32{skillChantVampire, skillChantFury}, skillWarChant},
		{"no war chant at 57", 57, []int32{skillChantVampire, skillChantFury}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := idleNuker(t, classWarcryer, tt.level, append(chants, skillChantVampire, skillChantFury, skillWarChant)...)
			effectsOn(b, append(chants, tt.on...)...)
			if tt.skill == 0 {
				wantNoRebuff(t, b, now)
				return
			}
			a, _ := b.ruleRebuff(now)
			wantCast(t, a, tt.skill, selfID)
		})
	}

	t.Run("steal essence from ruleCritical before a potion", func(t *testing.T) {
		b := mysticBrain(t, classWarcryer, 40, RoleDamage, "BLUNT", all...)
		b.opened = npcID
		wantCritical(t, b, 20, now, skillStealEssence)
	})
}
