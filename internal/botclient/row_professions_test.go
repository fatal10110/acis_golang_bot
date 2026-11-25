package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The fighter first-profession rows (docs/strategy.md, "Behavior per
// class", rows KN WA RO EK ES PK AS OR MO SC; "Testing", TestRow<ID>): each
// test compiles the real row for its class and level and checks the row's
// key behavior.

// Class ids of the fighter first professions.
const (
	classKnight       int32 = 4
	classElvenKnight  int32 = 19
	classElvenScout   int32 = 22
	classPalusKnight  int32 = 32
	classAssassin     int32 = 35
	classOrcRaider    int32 = 45
	classMonk         int32 = 47
	classScavenger    int32 = 54
	rowShield               = int32(0x30000101)
	rowShieldID             = int32(9001)
	rowEffectDuration       = 20 * time.Minute
)

// wearShield puts a shield on the bot's left hand beside its weapon.
func wearShield(b *Brain) {
	b.cfg.Travel.gear[rowShieldID] = gearItem{part: partLHand}
	items := []Item{{ObjectID: rowShield, TemplateID: rowShieldID, Count: 1, Equipped: true, BodySlot: partLHand}}
	for _, it := range b.world.Inventory.Items {
		items = append(items, it)
	}
	b.world.Apply(ItemList{Items: items}, t0)
}

// effectOn puts an effect of skill on the bot.
func effectOn(b *Brain, skill int32) {
	b.world.Apply(AbnormalStatusUpdate{Effects: []Effect{{SkillID: skill, Level: 1, Left: rowEffectDuration}}}, t0)
}

// togglesOn puts the toggles on the bot.
func togglesOn(b *Brain, skills ...int32) {
	ef := make([]Effect, len(skills))
	for i, id := range skills {
		ef[i] = Effect{SkillID: id, Level: 1, Permanent: true}
	}
	b.world.Apply(AbnormalStatusUpdate{Effects: ef}, t0)
}

// fightCast fails unless fight on npcID at now is a cast of skill on
// object, or a melee click when skill is 0.
func fightCast(t *testing.T, b *Brain, now time.Time, skill, object int32) Action {
	t.Helper()
	a := b.fight(npcID, now)
	if skill == 0 {
		if a.kind != actAttack {
			t.Fatalf("fight = kind %d skill %d, want a melee click", a.kind, a.skill)
		}
		return a
	}
	wantCast(t, a, skill, object)
	return a
}

// TestRowKN checks the knight: Aggression opens from 24 (Drain Health
// before 24 and while Aggression cools); Shield Stun only with a shield
// worn, else Power Strike.
func TestRowKN(t *testing.T) {
	now := at(time.Second)
	all := []int32{skillAggression, skillDrainHealth, skillShieldStun, skillPowerStrike}
	for _, tt := range []struct {
		name   string
		level  int32
		shield bool
		setup  func(b *Brain)
		skill  int32
	}{
		{"aggression opens from 400", 24, true, func(b *Brain) {
			b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 400}}, t0)
		}, skillAggression},
		{"drain health while aggression cools", 24, true, func(b *Brain) { b.world.holdSkill(skillAggression, now.Add(time.Minute)) }, skillDrainHealth},
		{"drain health opens before 24", 23, true, nil, skillDrainHealth},
		{"shield stun with a shield", 24, true, func(b *Brain) { b.opened = npcID }, skillShieldStun},
		{"no shield stun without one", 24, false, func(b *Brain) { b.opened = npcID }, skillPowerStrike},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classKnight, tt.level, "SWORD", all...)
			if tt.shield {
				wearShield(b)
			}
			if tt.setup != nil {
				tt.setup(b)
			}
			a := fightCast(t, b, now, tt.skill, npcID)
			if tt.skill != skillShieldStun && tt.skill != skillPowerStrike && !a.opens {
				t.Error("the opener does not use the opener up")
			}
		})
	}
}

// TestRowWA checks the warrior: War Cry in combat, then with a two-handed
// blunt Stun Attack, then Power Smash while it cools; Wild Sweep only with
// a pole and two attackers; Accuracy and Vicious Stance on at engage above
// 30 % MP, off under it and before it sits; its gear its line's weapon in
// heavy armor without a shield, a two-handed blunt on the Gladiator line and
// a pole on the Warlord line, while its human fighter row keeps a sword or
// a blunt with a shield (column 1, rows WA and HF).
func TestRowWA(t *testing.T) {
	now := at(time.Second)
	attacks := []int32{skillStunAttack, skillPowerSmash, skillWildSweep}
	for _, tt := range []struct {
		name   string
		weapon string
		skills []int32
		setup  func(b *Brain)
		skill  int32
		object int32
	}{
		{"war cry", "BIGBLUNT", append([]int32{skillWarCry}, attacks...), nil, skillWarCry, selfID},
		{"stun attack", "BIGBLUNT", attacks, nil, skillStunAttack, npcID},
		{"power smash", "BIGBLUNT", attacks, func(b *Brain) { b.world.holdSkill(skillStunAttack, now.Add(time.Minute)) }, skillPowerSmash, npcID},
		{"pole, one attacker", "POLE", attacks, nil, 0, 0},
		{"pole, two attackers", "POLE", attacks, hitByTwo, skillWildSweep, npcID},
		{"toggle on at engage", "BIGBLUNT", []int32{skillAccuracy, skillViciousStance, skillStunAttack}, nil, skillAccuracy, selfID},
		{"toggle off under 30 % MP", "BIGBLUNT", []int32{skillAccuracy, skillViciousStance, skillStunAttack}, func(b *Brain) {
			togglesOn(b, skillAccuracy, skillViciousStance)
			b.world.Self.MP = 25
		}, skillAccuracy, selfID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classWarrior, 24, tt.weapon, tt.skills...)
			if tt.setup != nil {
				tt.setup(b)
			}
			fightCast(t, b, now, tt.skill, tt.object)
		})
	}

	b := rowBrain(t, classWarrior, 24, "BIGBLUNT", skillAccuracy, skillViciousStance, skillRelax)
	togglesOn(b, skillAccuracy, skillViciousStance)
	s := &b.world.Self
	s.InCombat, s.HP = false, 30
	rule, a := b.decide(now)
	if rule != "rest" {
		t.Fatalf("decide = %q kind %d, want rest", rule, a.kind)
	}
	wantCast(t, a, skillAccuracy, selfID)

	for _, tt := range []struct {
		name    string
		second  int32
		weapons []string
	}{
		{"Gladiator line", classGladiator, []string{"BIGBLUNT"}},
		{"Warlord line", classWarlord, []string{"POLE"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := BranchChoice{Account: accountFor(t, HumanFighter.ID, RoleDamage, line{first: classWarrior, second: tt.second})}
			for _, level := range []int32{20, 39} {
				if g := CompileBuild(RaceHuman, classWarrior, level, RoleDamage, c).Kit.Gear; !slices.Equal(g.Weapons, tt.weapons) || g.Shield || g.Armor != "HEAVY" {
					t.Errorf("warrior gear at %d %+v, want %v in heavy armor without a shield", level, g, tt.weapons)
				}
			}
			if g := CompileBuild(RaceHuman, HumanFighter.ID, 19, RoleDamage, c).Kit.Gear; !slices.Equal(g.Weapons, []string{"SWORD", "BLUNT"}) || !g.Shield {
				t.Errorf("human fighter gear %+v, want a sword or a blunt with a shield", g)
			}
		})
	}
}

// TestRowRO checks the rogue: with a dagger Bleed above 50 % target HP,
// then Mortal Blow; with a bow Power Shot and never Bleed.
func TestRowRO(t *testing.T) {
	now := at(time.Second)
	all := []int32{skillBleed, skillMortalBlow, skillPowerShot}
	for _, tt := range []struct {
		name     string
		weapon   string
		targetHP int32
		skill    int32
	}{
		{"dagger, target above 50 %", "DAGGER", 80, skillBleed},
		{"dagger, target under 50 %", "DAGGER", 40, skillMortalBlow},
		{"bow", "BOW", 80, skillPowerShot},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classRogue, 24, tt.weapon, all...)
			o := b.world.Objects[npcID]
			o.HP, o.MaxHP = tt.targetHP, 100
			fightCast(t, b, now, tt.skill, npcID)
		})
	}
}

// TestRowEK checks the elven knight: Elemental Heal on itself under 50 %
// HP; Aggression opens.
func TestRowEK(t *testing.T) {
	now := at(time.Second)
	all := []int32{skillAggression, skillElementalHeal, skillPowerStrike}
	b := rowBrain(t, classElvenKnight, 24, "SWORD", all...)
	b.world.Self.HP = 45
	fightCast(t, b, now, skillElementalHeal, selfID)

	b = rowBrain(t, classElvenKnight, 24, "SWORD", all...)
	if a := fightCast(t, b, now, skillAggression, npcID); !a.opens {
		t.Error("Aggression does not use the opener up")
	}
}

// TestRowES checks the elven scout: at 32-35 the bow branch casts Rapid
// Shot in combat, then opens with Power Shot from 700; from 36 Entangle
// opens for both branches.
func TestRowES(t *testing.T) {
	now := at(time.Second)
	all := []int32{skillEntangle, skillBleed, skillMortalBlow, skillPowerShot, skillRapidShot}
	b := rowBrain(t, classElvenScout, 32, "BOW", all...)
	b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 700}}, t0)
	fightCast(t, b, now, skillRapidShot, selfID)
	effectOn(b, skillRapidShot)
	if a := fightCast(t, b, now, skillPowerShot, npcID); !a.opens {
		t.Error("Power Shot does not use the opener up")
	}

	for _, weapon := range []string{"BOW", "DAGGER"} {
		t.Run("entangle with "+weapon, func(t *testing.T) {
			b := rowBrain(t, classElvenScout, 36, weapon, all...)
			effectOn(b, skillRapidShot)
			fightCast(t, b, now, skillEntangle, npcID)
		})
	}
}

// TestRowPK checks the palus knight: on an opened target Sting with a
// sword, never with a blunt (gate); Aggression opens in the Support role
// only, Drain Health in the Damage role.
func TestRowPK(t *testing.T) {
	now := at(time.Second)
	all := []int32{skillAggression, skillDrainHealth, skillSting, skillPowerStrike}
	for _, tt := range []struct {
		weapon string
		skill  int32
	}{{"SWORD", skillSting}, {"BLUNT", skillPowerStrike}} {
		t.Run(tt.weapon, func(t *testing.T) {
			b := rowBrain(t, classPalusKnight, 24, tt.weapon, all...)
			b.opened = npcID
			fightCast(t, b, now, tt.skill, npcID)
		})
	}
	for _, tt := range []struct {
		role  Role
		skill int32
	}{{RoleSupport, skillAggression}, {RoleDamage, skillDrainHealth}} {
		t.Run(tt.role.String(), func(t *testing.T) {
			b := rowBrain(t, classPalusKnight, 24, "SWORD", all...)
			b.persona.Role, b.builtOK = tt.role, false
			fightCast(t, b, now, tt.skill, npcID)
		})
	}
}

// TestRowAS checks the assassin: with a dagger Drain Health pulls from 600
// on a fresh target, then Bleed, then Mortal Blow while Bleed cools; with a
// bow Power Shot opens instead.
func TestRowAS(t *testing.T) {
	now := at(time.Second)
	all := []int32{skillDrainHealth, skillBleed, skillMortalBlow, skillSting, skillPowerShot}
	b := rowBrain(t, classAssassin, 24, "DAGGER", all...)
	b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 600}}, t0)
	if a := fightCast(t, b, now, skillDrainHealth, npcID); !a.opens {
		t.Error("Drain Health does not use the opener up")
	}
	b.opened = npcID
	fightCast(t, b, now, skillBleed, npcID)
	b.world.holdSkill(skillBleed, now.Add(time.Minute))
	fightCast(t, b, now, skillMortalBlow, npcID)

	b = rowBrain(t, classAssassin, 24, "BOW", all...)
	fightCast(t, b, now, skillPowerShot, npcID)
}

// TestRowOR checks the orc raider: Rage in combat, Frenzy under 30 % HP
// from 32; at 36 Guts only while Frenzy cools and is not on the bot (the
// pinch stack); with two attackers it fights on down to 25 % HP.
func TestRowOR(t *testing.T) {
	now := at(time.Second)
	all := []int32{skillRage, skillFrenzy, skillGuts, skillStunAttack}
	b := rowBrain(t, classOrcRaider, 32, "BIGBLUNT", all...)
	fightCast(t, b, now, skillRage, selfID)
	b.world.Self.HP = 25
	fightCast(t, b, now, skillFrenzy, selfID)

	b = rowBrain(t, classOrcRaider, 31, "BIGBLUNT", all...)
	effectOn(b, skillRage)
	b.world.Self.HP = 25
	fightCast(t, b, now, skillStunAttack, npcID)

	for _, tt := range []struct {
		name   string
		frenzy bool
		skill  int32
		object int32
	}{
		{"guts while frenzy cools", false, skillGuts, selfID},
		{"no guts over frenzy", true, skillStunAttack, npcID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classOrcRaider, 36, "BIGBLUNT", all...)
			b.cfg.Travel.skillStacks = stackTravel().skillStacks
			b.world.Self.HP = 25
			b.world.holdSkill(skillFrenzy, now.Add(time.Minute))
			ef := []Effect{{SkillID: skillRage, Level: 1, Left: rowEffectDuration}}
			if tt.frenzy {
				ef = append(ef, Effect{SkillID: skillFrenzy, Level: 1, Left: time.Minute})
			}
			b.world.Apply(AbnormalStatusUpdate{Effects: ef}, t0)
			fightCast(t, b, now, tt.skill, tt.object)
		})
	}

	for _, tt := range []struct {
		hp   int32
		flee bool
	}{{26, false}, {24, true}} {
		b := rowBrain(t, classOrcRaider, 24, "BIGBLUNT")
		b.world.Self.HP = tt.hp
		hitByTwo(b)
		onGround(t, b)
		rule, a := b.decide(now)
		if rule != "outnumbered" || (a.kind == actMove) != tt.flee {
			t.Errorf("HP %d: decide = %q kind %d, want outnumbered with a flee %v", tt.hp, rule, a.kind, tt.flee)
		}
	}
}

// monkBrain is a monk at level with DUALFIST, knowing Focused Force at
// ffLevel and its other skills at 1, holding charges, with Focused Force's
// maxCharges of levels 1 and 2 in its travel map.
func monkBrain(t *testing.T, level, ffLevel, charges int32) *Brain {
	t.Helper()
	b := rowBrain(t, classMonk, level, "DUALFIST", skillForceBlaster, skillCripple, skillHurricaneAssault, skillStunningFist, skillIronPunch)
	b.cfg.Travel.maxCharges = map[datapack.SkillRef]int{{ID: skillFocusedForce, Level: 1}: 1, {ID: skillFocusedForce, Level: 2}: 2}
	known := []KnownSkill{{ID: skillFocusedForce, Level: ffLevel}}
	for _, id := range []int32{skillForceBlaster, skillCripple, skillHurricaneAssault, skillStunningFist, skillIronPunch} {
		known = append(known, KnownSkill{ID: id, Level: 1})
	}
	b.world.Apply(SkillList{Skills: known}, t0)
	b.world.Apply(EtcStatusUpdate{Charges: charges}, t0)
	return b
}

// TestRowMO checks the monk: Focused Force out of combat only below its
// known level's maxCharges (1 at 24-31, 2 from 32) and never in combat;
// Force Blaster opens only with a charge (else Cripple), Hurricane Assault
// only with two charges, Force Blaster with one, else Stunning Fist; a
// refused charge cast corrects the count, so it is not asked again.
func TestRowMO(t *testing.T) {
	now := at(time.Second)
	for _, tt := range []struct {
		name                    string
		level, ffLevel, charges int32
		want                    bool
	}{
		{"no charge at 24", 24, 1, 0, true},
		{"one charge at 24 is the most", 24, 1, 1, false},
		{"one charge at 32", 32, 2, 1, true},
		{"two charges at 32 are the most", 32, 2, 2, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := monkBrain(t, tt.level, tt.ffLevel, tt.charges)
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

	for _, tt := range []struct {
		name    string
		charges int32
		opened  bool
		skill   int32
	}{
		{"no charge opens with cripple", 0, false, skillCripple},
		{"a charge opens with force blaster", 1, false, skillForceBlaster},
		{"two charges, hurricane assault", 2, true, skillHurricaneAssault},
		{"one charge, force blaster", 1, true, skillForceBlaster},
		{"no charge, stunning fist", 0, true, skillStunningFist},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := monkBrain(t, 36, 2, tt.charges)
			if tt.opened {
				b.opened = npcID
			}
			fightCast(t, b, now, tt.skill, npcID)
		})
	}

	// Another player's EtcStatusUpdate left the bot thinking it has no
	// charge while it holds its maximum: the refused Focused Force sets the
	// count to the maximum, so the bot does not ask again.
	b := monkBrain(t, 24, 1, 0)
	b.world.Self.InCombat = false
	b.castAt, b.castSkill = now, skillFocusedForce
	b.castRefused(msgCannotBeUsed, now)
	if c := b.world.Self.Charges; c != 1 {
		t.Errorf("charges after a refused Focused Force = %d, want 1", c)
	}
	later := now.Add(castRefusedHold + time.Second)
	if a, ok := b.ruleRebuff(later); ok {
		t.Errorf("ruleRebuff after the refusal = kind %d skill %d, want no Focused Force", a.kind, a.skill)
	}
	// The other way: a refused Force Blaster means no charge.
	b = monkBrain(t, 36, 2, 2)
	b.castAt, b.castSkill = now, skillHurricaneAssault
	b.castRefused(msgCannotBeUsed, now)
	if c := b.world.Self.Charges; c != 1 {
		t.Errorf("charges after a refused Hurricane Assault = %d, want 1", c)
	}
	b.castAt, b.castSkill = now, skillForceBlaster
	b.castRefused(msgCannotBeUsed, now)
	if c := b.world.Self.Charges; c != 0 {
		t.Errorf("charges after a refused Force Blaster = %d, want 0", c)
	}
}

// TestRowSC checks the scavenger: Spoil opens on a fresh target, then Stun
// Attack with a blunt (spoil and sweep: row_spoil_test.go), melee with a
// sword, and its row's thresholds.
func TestRowSC(t *testing.T) {
	now := at(time.Second)
	b := rowBrain(t, classScavenger, 20, "BLUNT", skillSpoil, skillStunAttack)
	fightCast(t, b, now, skillSpoil, npcID)
	b.opened = npcID
	fightCast(t, b, now, skillStunAttack, npcID)
	if th := b.build().Thresholds; th != professionKits[classScavenger].Thresholds {
		t.Errorf("thresholds %+v, want the row's %+v", th, professionKits[classScavenger].Thresholds)
	}
	b = rowBrain(t, classScavenger, 20, "SWORD", skillStunAttack)
	fightCast(t, b, now, 0, 0)
}

// TestCompileProfessionBuild checks a first profession compiles its own
// row: the monk's Wolf Spirit Totem until 27 and only Bear Spirit Totem
// from 28; the palus knight's Aggression in the Support role only; a
// profession without a row (a mystic's, until S7) keeps its line's kit.
func TestCompileProfessionBuild(t *testing.T) {
	buffs := func(bd Build) []int32 {
		var ids []int32
		for _, bf := range bd.Kit.Buffs {
			ids = append(ids, bf.Skill)
		}
		return ids
	}
	for _, tt := range []struct {
		level int32
		want  []int32
	}{{27, []int32{skillWolfTotem}}, {28, []int32{skillBearTotem}}} {
		if got := buffs(CompileBuild(RaceOrc, classMonk, tt.level, RoleDamage, BranchChoice{})); !slices.Equal(got, tt.want) {
			t.Errorf("monk at %d buffs %v, want %v", tt.level, got, tt.want)
		}
	}
	for _, tt := range []struct {
		role Role
		want bool
	}{{RoleSupport, true}, {RoleDamage, false}} {
		bd := CompileBuild(RaceDarkElf, classPalusKnight, 24, tt.role, BranchChoice{})
		if got := slices.Contains(stepSkills(bd.Kit.Rotation), skillAggression); got != tt.want {
			t.Errorf("palus knight %v rotation %v, want Aggression %v", tt.role, stepSkills(bd.Kit.Rotation), tt.want)
		}
		if bd.Thresholds != professionKits[classPalusKnight].Thresholds {
			t.Errorf("palus knight %v thresholds %+v, want the row's", tt.role, bd.Thresholds)
		}
	}
	if bd := CompileBuild(RaceHuman, classCleric, 25, RoleSupport, BranchChoice{}); !slices.Equal(stepSkills(bd.Kit.Rotation), []int32{skillWindStrike}) {
		t.Errorf("cleric rotation %v, want the cleric row's", stepSkills(bd.Kit.Rotation))
	}
	// A second profession plays its first profession's row until it has
	// its own kit (docs/strategy.md "Second professions").
	for _, tt := range []struct {
		race, class, first int32
	}{{RaceOrc, 46, classOrcRaider}, {RaceHuman, 2, classWarrior}, {RaceDwarf, 55, classScavenger}} {
		bd, want := CompileBuild(tt.race, tt.class, 45, RoleDamage, BranchChoice{}), CompileBuild(tt.race, tt.first, 45, RoleDamage, BranchChoice{})
		if !slices.Equal(stepSkills(bd.Kit.Rotation), stepSkills(want.Kit.Rotation)) || bd.Thresholds != professionKits[tt.first].Thresholds {
			t.Errorf("class %d at 45 rotation %v thresholds %+v, want class %d's %v %+v",
				tt.class, stepSkills(bd.Kit.Rotation), bd.Thresholds, tt.first, stepSkills(want.Kit.Rotation), professionKits[tt.first].Thresholds)
		}
	}
	if bd, want := CompileBuild(RaceHuman, 16, 45, RoleSupport, BranchChoice{}), CompileBuild(RaceHuman, classCleric, 45, RoleSupport, BranchChoice{}); !slices.Equal(stepSkills(bd.Kit.Rotation), stepSkills(want.Kit.Rotation)) {
		t.Errorf("bishop rotation %v, want the cleric row's %v", stepSkills(bd.Kit.Rotation), stepSkills(want.Kit.Rotation))
	}
}

// TestIndexMaxCharges checks the maxCharges index keeps every skill level
// with one and maxChargesOf reads it, 0 for none or no travel map.
func TestIndexMaxCharges(t *testing.T) {
	m := &TravelMap{maxCharges: indexMaxCharges(map[datapack.SkillRef]datapack.Skill{
		{ID: skillFocusedForce, Level: 1}: {MaxCharges: 1},
		{ID: skillFocusedForce, Level: 2}: {MaxCharges: 2},
		{ID: skillForceBlaster, Level: 1}: {NumCharges: 1},
	})}
	for _, tt := range []struct {
		m            *TravelMap
		skill, level int32
		want         int
	}{
		{m, skillFocusedForce, 1, 1},
		{m, skillFocusedForce, 2, 2},
		{m, skillForceBlaster, 1, 0},
		{nil, skillFocusedForce, 1, 0},
	} {
		if got := tt.m.maxChargesOf(tt.skill, tt.level); got != tt.want {
			t.Errorf("maxChargesOf(%d, %d) = %d, want %d", tt.skill, tt.level, got, tt.want)
		}
	}
	if len(m.maxCharges) != 2 {
		t.Errorf("index %v, want the two Focused Force levels", m.maxCharges)
	}
}
