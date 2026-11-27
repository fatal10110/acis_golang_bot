package botclient

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The tank rows (docs/strategy.md "Second professions (levels 40+)", rows
// PL, TK and SK): each test compiles the real row for its level and checks
// the row's key behavior.

// tankCase is one fight decision of a tank row: the bot at level with
// weapon, a shield worn or not, a fresh target or one it opened on,
// knowing skills, after setup; it casts skill on object (0: a melee click).
type tankCase struct {
	name          string
	level         int32
	weapon        string
	shield, fresh bool
	skills        []int32
	setup         func(b *Brain)
	skill, object int32
}

// checkTankFight runs the fight cases of class.
func checkTankFight(t *testing.T, class int32, now time.Time, cases []tankCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, class, tt.level, tt.weapon, tt.skills...)
			if tt.shield {
				wearShield(b)
			}
			if !tt.fresh {
				b.opened = npcID
			}
			if tt.setup != nil {
				tt.setup(b)
			}
			fightCast(t, b, now, tt.skill, tt.object)
		})
	}
}

// checkToggleRest checks a tank at level with toggle on, out of combat
// under SitHP, turns it off before it sits.
func checkToggleRest(t *testing.T, class, level, toggle int32, now time.Time) {
	t.Helper()
	b := rowBrain(t, class, level, "SWORD", toggle)
	togglesOn(b, toggle)
	s := &b.world.Self
	s.InCombat, s.HP = false, 30
	rule, a := b.decide(now)
	if rule != "rest" {
		t.Fatalf("decide = %q kind %d, want rest", rule, a.kind)
	}
	wantCast(t, a, toggle, selfID)
}

// checkIdleBuff checks a tank at level out of combat knowing skills puts
// up skill on itself (0: nothing).
func checkIdleBuff(t *testing.T, class, level int32, skills []int32, skill int32, now time.Time) {
	t.Helper()
	b := rowBrain(t, class, level, "SWORD", skills...)
	b.world.Self.InCombat = false
	b.world.Target = selfID // the select before a buff is castOn's
	a, ok := b.ruleRebuff(now)
	if ok != (skill != 0) {
		t.Fatalf("ruleRebuff = %v kind %d skill %d, want a cast of %d", ok, a.kind, a.skill, skill)
	}
	if ok {
		wantCast(t, a, skill, selfID)
	}
}

// tankMP sets the bot's MP percent.
func tankMP(pct int32) func(b *Brain) { return func(b *Brain) { b.world.Self.MP = pct } }

// tankHP sets the bot's HP percent.
func tankHP(pct int32) func(b *Brain) { return func(b *Brain) { b.world.Self.HP = pct } }

// TestRowPL checks the paladin: Aggression opens from its 400 cast range;
// Aura of Hate only with two attackers; Shield Stun only with a shield
// worn; Tribunal from 55, not at 54; Drain Health under 60 % HP and Divine
// Heal under 40 %; Shield Fortress on at engage from 64 above 30 % MP, off
// under it and before it sits; Holy Blessing from ruleHeal on a party
// leader under 50 % HP before the fight's next step, and on itself; Iron
// Will out of combat from 43; below SitHP it casts Relax.
func TestRowPL(t *testing.T) {
	now := at(time.Second)
	attacks := []int32{skillAggression, skillAuraOfHate, skillShieldStun, skillTribunal, skillPowerStrike}
	withFortress := append(slices.Clone(attacks), skillShieldFortress)
	sustain := append(slices.Clone(attacks), skillDrainHealth, skillDivineHeal)
	checkTankFight(t, classPaladin, now, []tankCase{
		{"aggression opens from 400", 40, "SWORD", true, true, attacks, func(b *Brain) {
			b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 400}}, t0)
		}, skillAggression, npcID},
		{"aura of hate with two attackers", 40, "SWORD", true, false, attacks, hitByTwo, skillAuraOfHate, npcID},
		{"no aura of hate with one attacker", 40, "SWORD", true, false, attacks, nil, skillShieldStun, npcID},
		{"no shield stun without a shield", 40, "SWORD", false, false, attacks, nil, skillPowerStrike, npcID},
		{"tribunal at 55", 55, "SWORD", true, false, attacks, func(b *Brain) { hold(b, now, skillShieldStun) }, skillTribunal, npcID},
		{"no tribunal at 54", 54, "SWORD", true, false, attacks, func(b *Brain) { hold(b, now, skillShieldStun) }, skillPowerStrike, npcID},
		{"drain health under 60 %", 40, "SWORD", true, false, sustain, tankHP(55), skillDrainHealth, npcID},
		{"divine heal under 40 %", 40, "SWORD", true, false, sustain, tankHP(35), skillDivineHeal, selfID},
		{"shield fortress on at engage from 64", 64, "SWORD", true, true, withFortress, nil, skillShieldFortress, selfID},
		{"no shield fortress at 63", 63, "SWORD", true, true, withFortress, nil, skillAggression, npcID},
		{"no shield fortress at engage under 30 % MP", 64, "SWORD", true, true, withFortress, tankMP(25), skillAggression, npcID},
		{"shield fortress off under 30 % MP", 64, "SWORD", true, false, withFortress, func(b *Brain) {
			togglesOn(b, skillShieldFortress)
			b.world.Self.MP = 25
		}, skillShieldFortress, selfID},
	})
	t.Run("shield fortress off before it sits", func(t *testing.T) {
		checkToggleRest(t, classPaladin, 64, skillShieldFortress, now)
	})

	for _, tt := range []struct {
		name   string
		party  bool
		selfHP int32
		object int32
	}{
		{"holy blessing on the party leader under 50 %", true, 100, pcID},
		{"holy blessing on itself under 50 %", false, 45, selfID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classPaladin, 40, "SWORD", append(slices.Clone(attacks), skillHolyBlessing)...)
			wearShield(b)
			b.world.Self.HP = tt.selfHP
			if tt.party {
				inParty(b, 45, 100)
			}
			rule, a := b.decide(now)
			if rule != "heal" {
				t.Fatalf("decide = %q kind %d skill %d, want heal", rule, a.kind, a.skill)
			}
			// The select of the patient goes out before the cast.
			b.world.Target = tt.object
			a, _ = b.ruleHeal(now)
			wantCast(t, a, skillHolyBlessing, tt.object)
		})
	}

	t.Run("iron will out of combat from 43", func(t *testing.T) {
		checkIdleBuff(t, classPaladin, 43, []int32{skillIronWill}, skillIronWill, now)
	})
	t.Run("no iron will at 42", func(t *testing.T) {
		checkIdleBuff(t, classPaladin, 42, []int32{skillIronWill}, 0, now)
	})
	t.Run("majesty out of combat", func(t *testing.T) {
		checkIdleBuff(t, classPaladin, 40, []int32{skillMajesty}, skillMajesty, now)
	})

	for _, tt := range []struct {
		name string
		mp   int32
		want actionKind
	}{
		{"relax below SitHP", 100, actCast},
		{"no MP for relax sits", 10, actSitStand},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classPaladin, 40, "SWORD", skillRelax)
			s := &b.world.Self
			s.InCombat, s.HP, s.MP = false, 30, tt.mp
			rule, a := b.decide(now)
			if rule != "rest" || a.kind != tt.want {
				t.Fatalf("decide = %q kind %d, want rest kind %d", rule, a.kind, tt.want)
			}
			if tt.want == actCast {
				wantCast(t, a, skillRelax, selfID)
			}
		})
	}
}

// TestRowTK checks the temple knight: Aggression opens, and with it on
// reuse Entangle; Aura of Hate with two attackers; Tribunal from 55;
// Elemental Heal under 50 % HP; Guard Stance on at engage from 43 above 30
// % MP, off under it and before it sits; Shield Fortress at engage from
// 64; Spirit Barrier out of combat.
func TestRowTK(t *testing.T) {
	now := at(time.Second)
	attacks := []int32{skillAggression, skillEntangle, skillAuraOfHate, skillTribunal, skillPowerStrike}
	withStance := append(slices.Clone(attacks), skillGuardStance)
	checkTankFight(t, classTempleKnight, now, []tankCase{
		{"aggression opens", 40, "SWORD", true, true, attacks, nil, skillAggression, npcID},
		{"entangle opens with aggression on reuse", 40, "SWORD", true, true, attacks, func(b *Brain) { hold(b, now, skillAggression) }, skillEntangle, npcID},
		{"aura of hate with two attackers", 40, "SWORD", true, false, attacks, hitByTwo, skillAuraOfHate, npcID},
		{"power strike with one attacker", 40, "SWORD", true, false, attacks, nil, skillPowerStrike, npcID},
		{"tribunal at 55", 55, "SWORD", true, false, attacks, nil, skillTribunal, npcID},
		{"elemental heal under 50 %", 40, "SWORD", true, false, append(slices.Clone(attacks), skillElementalHeal), tankHP(45), skillElementalHeal, selfID},
		{"guard stance on at engage from 43", 43, "SWORD", true, true, withStance, nil, skillGuardStance, selfID},
		{"no guard stance at 42", 42, "SWORD", true, true, withStance, nil, skillAggression, npcID},
		{"no guard stance at engage under 30 % MP", 43, "SWORD", true, true, withStance, tankMP(25), skillAggression, npcID},
		{"guard stance off under 30 % MP", 43, "SWORD", true, false, withStance, func(b *Brain) {
			togglesOn(b, skillGuardStance)
			b.world.Self.MP = 25
		}, skillGuardStance, selfID},
		{"shield fortress at engage from 64", 64, "SWORD", true, true, append(slices.Clone(attacks), skillShieldFortress), nil, skillShieldFortress, selfID},
	})
	t.Run("guard stance off before it sits", func(t *testing.T) {
		checkToggleRest(t, classTempleKnight, 43, skillGuardStance, now)
	})
	t.Run("spirit barrier out of combat", func(t *testing.T) {
		checkIdleBuff(t, classTempleKnight, 40, []int32{skillSpiritBarrier}, skillSpiritBarrier, now)
	})
}

// TestRowSK checks the shillien knight: Life Leech under 70 % HP before
// Drain Health under 60 %, in fight and from ruleCritical before a potion;
// Aggression opens, then Drain Health; Life Leech leads the attacks;
// Judgment from 55 with a sword; Lightning Strike from 58; Sting with a
// sword and never with a blunt, Freezing Strike in its place.
func TestRowSK(t *testing.T) {
	now := at(10 * time.Second)
	attacks := []int32{skillAggression, skillDrainHealth, skillAuraOfHate, skillLifeLeech, skillJudgment, skillLightningStrike, skillSting, skillFreezingStrike, skillPowerStrike}
	checkTankFight(t, classShillienKnight, now, []tankCase{
		{"aggression opens", 40, "SWORD", true, true, attacks, nil, skillAggression, npcID},
		{"drain health opens with aggression on reuse", 40, "SWORD", true, true, attacks, func(b *Brain) { hold(b, now, skillAggression) }, skillDrainHealth, npcID},
		{"aura of hate with two attackers", 40, "SWORD", true, false, attacks, hitByTwo, skillAuraOfHate, npcID},
		{"life leech leads the attacks", 40, "SWORD", true, false, attacks, nil, skillLifeLeech, npcID},
		{"life leech under 70 %", 40, "SWORD", true, false, attacks, func(b *Brain) {
			b.world.Self.HP = 65
			hitByTwo(b) // the sustain step goes before Aura of Hate
		}, skillLifeLeech, npcID},
		{"life leech before drain health under 60 %", 40, "SWORD", true, false, attacks, func(b *Brain) {
			b.world.Self.HP = 55
			hitByTwo(b)
		}, skillLifeLeech, npcID},
		{"drain health under 60 % with life leech on reuse", 40, "SWORD", true, false, attacks, func(b *Brain) {
			b.world.Self.HP = 55
			hitByTwo(b)
			hold(b, now, skillLifeLeech)
		}, skillDrainHealth, npcID},
		{"judgment from 55 with a sword", 55, "SWORD", true, false, attacks, func(b *Brain) { hold(b, now, skillLifeLeech) }, skillJudgment, npcID},
		{"no judgment at 54", 54, "SWORD", true, false, attacks, func(b *Brain) { hold(b, now, skillLifeLeech) }, skillSting, npcID},
		{"lightning strike from 58", 58, "SWORD", true, false, attacks, func(b *Brain) { hold(b, now, skillLifeLeech, skillJudgment) }, skillLightningStrike, npcID},
		{"no lightning strike at 57", 57, "SWORD", true, false, attacks, func(b *Brain) { hold(b, now, skillLifeLeech, skillJudgment) }, skillSting, npcID},
		{"sting with a sword", 40, "SWORD", true, false, attacks, func(b *Brain) { hold(b, now, skillLifeLeech) }, skillSting, npcID},
		{"no sting with a blunt", 40, "BLUNT", true, false, attacks, func(b *Brain) { hold(b, now, skillLifeLeech) }, skillFreezingStrike, npcID},
	})

	for _, tt := range []struct {
		name    string
		cooling []int32
		skill   int32 // 0: a potion
	}{
		{"critical life leech first", nil, skillLifeLeech},
		{"critical drain health with life leech on reuse", []int32{skillLifeLeech}, skillDrainHealth},
		{"critical potion with both on reuse", []int32{skillLifeLeech, skillDrainHealth}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classShillienKnight, 40, "SWORD", skillLifeLeech, skillDrainHealth, skillPowerStrike)
			b.world.Apply(ItemList{Items: []Item{potionsOf(20), {ObjectID: rowWeapon, TemplateID: rowWeaponID, Count: 1, Equipped: true, BodySlot: partRHand}}}, t0)
			b.opened = npcID
			b.world.Self.HP = 20
			b.world.Apply(Attack{AttackerID: npcID, Pos: Point{X: 30}, Hits: []AttackHit{{TargetID: selfID, Damage: 10}}}, t0)
			hold(b, now, tt.cooling...)
			rule, a := b.decide(now)
			if rule != "critical" {
				t.Fatalf("decide = %q kind %d skill %d, want critical", rule, a.kind, a.skill)
			}
			if tt.skill == 0 {
				if a.kind != actUseItem {
					t.Fatalf("decide = kind %d skill %d, want a potion", a.kind, a.skill)
				}
				return
			}
			wantCast(t, a, tt.skill, npcID)
		})
	}
}

// TestCompileTankBuilds checks the tank rows compile, at every level and in
// both roles, none of the skills the plan leaves out: no cubic summon (S17,
// #636) and none of PL's, TK's or SK's other dropped skills.
func TestCompileTankBuilds(t *testing.T) {
	for _, tt := range []struct {
		class int32
		race  int32
		never []int32
	}{
		// Holy Strike, Sanctuary, Banish Undead, Shackle, Mass Shackling,
		// Sacrifice.
		{classPaladin, RaceHuman, []int32{49, 97, 405, 403, 404, 69}},
		// Holy Aura, Arrest, Summon Attractive Cubic, Storm Cubic, Life
		// Cubic.
		{classTempleKnight, RaceElf, []int32{107, 402, 449, 10, 67}},
		// Corpse Plague, Arrest, Hex, Summon Viper Cubic, Phantom Cubic,
		// Vampiric Cubic.
		{classShillienKnight, RaceDarkElf, []int32{103, 402, 122, 278, 33, 22}},
	} {
		for _, level := range []int32{40, 55, 64, 75} {
			for _, role := range []Role{RoleDamage, RoleSupport} {
				k := CompileBuild(tt.race, tt.class, level, role, BranchChoice{}).Kit
				var buffs []int32
				for _, bf := range k.Buffs {
					buffs = append(buffs, bf.Skill)
				}
				for _, never := range tt.never {
					if slices.Contains(k.Skills, never) || slices.Contains(stepSkills(k.Rotation), never) || slices.Contains(buffs, never) {
						t.Errorf("class %d at %d %v has skill %d, want none", tt.class, level, role, never)
					}
				}
			}
		}
	}
}

// TestGearRoleTankHeals checks a Support build that melees always (a
// fighter's kit) is a tank whatever heal it knows: PL knowing Holy Blessing
// keeps valuing its plate and sword, while a cleric's Support build that
// knows Heal values the HP a minute it restores.
func TestGearRoleTankHeals(t *testing.T) {
	for _, tt := range []struct {
		name  string
		class int32
		heal  int32
		kind  charmodel.RoleKind
		tank  bool
	}{
		{"paladin with holy blessing is a tank", classPaladin, skillHolyBlessing, charmodel.Damage, true},
		{"cleric with heal is a healer", classCleric, skillHeal, charmodel.Support, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newModelBrain(t, tt.class, 40, RoleSupport, []KnownSkill{{ID: tt.heal, Level: 1}}, nil)
			m := b.cfg.Travel
			m.skills = maps.Clone(m.skills)
			m.skills[datapack.SkillRef{ID: skillHolyBlessing, Level: 1}] = datapack.Skill{Type: "HEAL", OperateType: "ACTIVE", Power: 200, HitTime: 2 * time.Second, ReuseDelay: 10 * time.Second, MPConsume: 30}
			role, tank := b.gearRole()
			if role.Kind != tt.kind || tank != tt.tank {
				t.Errorf("gearRole = %+v, tank %v; want kind %v, tank %v", role, tank, tt.kind, tt.tank)
			}
		})
	}
}
