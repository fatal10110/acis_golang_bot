package botclient

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// partyRow is a party window row of the healer tests: id of class at hp and
// mp out of 100.
func partyRow(id, class, hp, mp int32) PartyMember {
	return PartyMember{ObjectID: id, HP: hp, MaxHP: 100, MP: mp, MaxMP: 100, ClassID: class}
}

// healerParty puts the bot in leader's party with other, both beside it,
// as inParty, with their classes and MP.
func healerParty(b *Brain, leader, other PartyMember) {
	b.world.Apply(CharInfo{ObjectID: leader.ObjectID, Name: "other", Pos: Point{Y: 300}}, t0)
	b.world.Apply(CharInfo{ObjectID: other.ObjectID, Name: "third", Pos: Point{X: -300}}, t0)
	b.world.Apply(PartySmallWindowAll{LeaderID: leader.ObjectID, Members: []PartyMember{leader, other}}, t0)
}

// healCase is a ruleHeal case of a healer row: the bot of class at level
// knowing skills, in a party set up by setup, casts skill on object, or
// nothing with none.
type healCase struct {
	name   string
	level  int32
	skills []int32
	setup  func(b *Brain, now time.Time)
	skill  int32
	object int32
	none   bool
}

func runHealCases(t *testing.T, class int32, cases []healCase) {
	t.Helper()
	now := at(time.Second)
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			b := mysticBrain(t, class, tt.level, RoleSupport, "", tt.skills...)
			tt.setup(b, now)
			b.world.Target = tt.object
			a, ok := b.ruleHeal(now)
			if tt.none {
				if ok {
					t.Fatalf("ruleHeal = kind %d skill %d on %#x, want none", a.kind, a.skill, a.object)
				}
				return
			}
			if !ok {
				t.Fatal("ruleHeal: no heal")
			}
			wantCast(t, a, tt.skill, tt.object)
		})
	}
}

// party is a healCase setup of inParty at leaderHP and otherHP, holding
// skills on reuse.
func party(leaderHP, otherHP int32, held ...int32) func(b *Brain, now time.Time) {
	return func(b *Brain, now time.Time) {
		inParty(b, leaderHP, otherHP)
		hold(b, now, held...)
	}
}

// rebuffCase is a ruleRebuff case of a healer row out of combat: the bot of
// class at level knowing skills, in inParty's party (a fighter leader and a
// mystic third member) set up by setup, buffs object with skill, or casts
// nothing with none.
type rebuffCase struct {
	name   string
	level  int32
	skills []int32
	setup  func(b *Brain)
	skill  int32
	object int32
	none   bool
}

func runRebuffCases(t *testing.T, class int32, cases []rebuffCase) {
	t.Helper()
	now := at(time.Second)
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			b := mysticBrain(t, class, tt.level, RoleSupport, "", tt.skills...)
			inParty(b, 100, 100)
			b.world.Self.InCombat = false
			if tt.setup != nil {
				tt.setup(b)
			}
			b.world.Target = tt.object
			a, ok := b.ruleRebuff(now)
			if tt.none {
				if ok {
					t.Fatalf("ruleRebuff = kind %d skill %d on %#x, want none", a.kind, a.skill, a.object)
				}
				return
			}
			if !ok {
				t.Fatal("ruleRebuff: no buff")
			}
			wantCast(t, a, tt.skill, tt.object)
		})
	}
}

// selfHolds gives the bot skills' effects for an hour.
func selfHolds(skills ...int32) func(b *Brain) {
	return func(b *Brain) {
		ef := make([]Effect, len(skills))
		for i, id := range skills {
			ef[i] = Effect{SkillID: id, Level: 1, Left: time.Hour}
		}
		b.world.Apply(AbnormalStatusUpdate{Effects: ef}, t0)
	}
}

// memberHolds gives party member id skills' effects for an hour.
func memberHolds(id int32, skills ...int32) func(b *Brain) {
	return func(b *Brain) {
		ef := make([]Effect, len(skills))
		for i, s := range skills {
			ef[i] = Effect{SkillID: s, Level: 1, Left: time.Hour}
		}
		b.world.Apply(PartySpelled{Kind: partySpelledPlayer, ObjectID: id, Effects: ef}, t0)
	}
}

// TestRowBI checks the bishop: Restore Life from 44 on a member under
// 20 %, after a leader under 35 % got Greater Battle Heal; Major Group Heal
// from 58 with two members under 60 %, Greater Group Heal before; Major
// Heal from 56, Greater Heal while it cools; Vitalize from 48 only; Prayer
// from 66.
func TestRowBI(t *testing.T) {
	heals := []int32{
		skillRestoreLife, skillGreaterBattleHeal, skillMajorGroupHeal, skillGreaterGroupHeal, skillMajorHeal,
		skillGreaterHeal, skillVitalize,
	}
	runHealCases(t, classBishop, []healCase{
		{name: "restore life on a member under 20 %", level: 44, skills: heals, setup: party(100, 15), skill: skillRestoreLife, object: rowPC2},
		{name: "no restore life at 43", level: 43, skills: heals, setup: party(100, 15), skill: skillGreaterBattleHeal, object: rowPC2},
		{name: "the leader under 35 % first", level: 44, skills: heals, setup: party(30, 15), skill: skillGreaterBattleHeal, object: pcID},
		{name: "major group heal at 58", level: 58, skills: heals, setup: party(50, 50), skill: skillMajorGroupHeal, object: selfID},
		{name: "greater group heal at 57", level: 57, skills: heals, setup: party(50, 50), skill: skillGreaterGroupHeal, object: selfID},
		{name: "major heal at 56", level: 56, skills: heals, setup: party(50, 100), skill: skillMajorHeal, object: pcID},
		{name: "greater heal while major heal cools", level: 56, skills: heals, setup: party(50, 100, skillMajorHeal), skill: skillGreaterHeal, object: pcID},
		{name: "vitalize at 48", level: 48, skills: heals, setup: party(50, 100, skillGreaterHeal), skill: skillVitalize, object: pcID},
		{name: "no vitalize at 47", level: 47, skills: heals, setup: party(50, 100, skillGreaterHeal), none: true},
	})
	runRebuffCases(t, classBishop, []rebuffCase{
		{name: "prayer at 66", level: 66, skills: []int32{skillPrayer}, skill: skillPrayer, object: selfID},
		{name: "no prayer at 65", level: 65, skills: []int32{skillPrayer}, none: true},
	})
}

// TestRowPP checks the prophet: Haste from 44, Guidance, Death Whisper and
// Greater Might from 58 on the fighter leader; Acumen, Blessed Soul from 44
// and Greater Shield from 58 on the mystic member, never Haste; no Greater
// Might on a member holding Greater Shield (one stack); Dryad Root on a
// second attacker.
func TestRowPP(t *testing.T) {
	const pp = classProphet
	stacks := func(b *Brain) {
		b.cfg.Travel.skillStacks = map[int32]map[int32]stackEffect{
			skillGreaterMight:  {1: {"pa_pd_up", 1}},
			skillGreaterShield: {1: {"pa_pd_up", 1}},
		}
	}
	mystic := selfHolds(skillAcumen, skillBlessedSoul, skillGreaterShield)
	runRebuffCases(t, pp, []rebuffCase{
		{name: "haste on the fighter at 44", level: 44, skills: []int32{skillHaste}, skill: skillHaste, object: pcID},
		{name: "no haste at 43", level: 43, skills: []int32{skillHaste}, none: true},
		{name: "never haste on a mystic", level: 44, skills: []int32{skillHaste}, setup: memberHolds(pcID, skillHaste), none: true},
		{name: "guidance on the fighter", level: 40, skills: []int32{skillGuidance}, skill: skillGuidance, object: pcID},
		{name: "death whisper on the fighter", level: 40, skills: []int32{skillDeathWhisper}, skill: skillDeathWhisper, object: pcID},
		{name: "greater might on the fighter at 58", level: 58, skills: []int32{skillGreaterMight}, setup: stacks, skill: skillGreaterMight, object: pcID},
		{name: "no greater might at 57", level: 57, skills: []int32{skillGreaterMight}, setup: stacks, none: true},
		{
			name: "no greater might over greater shield", level: 58, skills: []int32{skillGreaterMight},
			setup: func(b *Brain) { stacks(b); memberHolds(pcID, skillGreaterShield)(b) }, none: true,
		},
		{name: "acumen on the mystic", level: 40, skills: []int32{skillAcumen}, setup: mystic, skill: skillAcumen, object: rowPC2},
		{name: "blessed soul on the mystic at 44", level: 44, skills: []int32{skillBlessedSoul}, setup: mystic, skill: skillBlessedSoul, object: rowPC2},
		{name: "greater shield on the mystic at 58", level: 58, skills: []int32{skillGreaterShield}, setup: mystic, skill: skillGreaterShield, object: rowPC2},
		{name: "no greater shield on the fighter", level: 58, skills: []int32{skillGreaterShield}, setup: func(b *Brain) { mystic(b); memberHolds(rowPC2, skillGreaterShield)(b) }, none: true},
	})

	now := at(time.Second)
	b := mysticBrain(t, pp, 40, RoleSupport, "", skillDryadRoot, skillWindStrike)
	fightCast(t, b, now, skillWindStrike, npcID)
	hitByTwo(b)
	if a := b.fight(npcID, now); a.kind != actSelect || a.object != rowNPC2 {
		t.Fatalf("fight = kind %d object %#x, want a select of the second attacker", a.kind, a.object)
	}
	b.world.Target = rowNPC2
	wantCast(t, b.fight(npcID, now), skillDryadRoot, rowNPC2)
}

// TestRowEE checks the elven elder: Recharge on a member under 40 % MP,
// never on the bot nor a member whose class knows it, nor one with no MP
// reported; an HP heal on any patient goes out first; Greater Heal from
// 48 ahead of Heal, Major Heal from 56; Clarity on the mystic members from
// 58.
func TestRowEE(t *testing.T) {
	heals := []int32{skillBattleHeal, skillGroupHeal, skillMajorHeal, skillGreaterHeal, skillVitalize, skillHeal, skillRecharge}
	mpParty := func(leader, other PartyMember) func(b *Brain, now time.Time) {
		return func(b *Brain, _ time.Time) { healerParty(b, leader, other) }
	}
	fighter := partyRow(pcID, HumanFighter.ID, 100, 100)
	runHealCases(t, classElvenElder, append(rechargeCases(heals), []healCase{
		{name: "greater heal at 48", level: 48, skills: heals, setup: party(100, 50), skill: skillGreaterHeal, object: rowPC2},
		{name: "heal at 47", level: 47, skills: heals, setup: party(100, 50), skill: skillHeal, object: rowPC2},
		{name: "major heal at 56", level: 56, skills: heals, setup: party(100, 50), skill: skillMajorHeal, object: rowPC2},
		{name: "greater heal at 55", level: 55, skills: heals, setup: party(100, 50), skill: skillGreaterHeal, object: rowPC2},
		{
			name: "an HP heal before recharge", level: 40, skills: heals,
			setup: mpParty(partyRow(pcID, HumanFighter.ID, 100, 30), partyRow(rowPC2, classHumanMystic, 50, 100)), skill: skillHeal, object: rowPC2,
		},
		{
			name: "recharge on the healthy leader", level: 40, skills: heals,
			setup: mpParty(partyRow(pcID, HumanFighter.ID, 100, 30), partyRow(rowPC2, classHumanMystic, 100, 100)), skill: skillRecharge, object: pcID,
		},
		{
			name: "no recharge with no MP reported", level: 40, skills: heals,
			setup: mpParty(fighter, PartyMember{ObjectID: rowPC2, HP: 100, MaxHP: 100, ClassID: classHumanMystic}), none: true,
		},
	}...))
	runRebuffCases(t, classElvenElder, []rebuffCase{
		{name: "clarity on the mystic at 58", level: 58, skills: []int32{skillClarity}, setup: selfHolds(skillClarity), skill: skillClarity, object: rowPC2},
		{name: "no clarity at 57", level: 57, skills: []int32{skillClarity}, setup: selfHolds(skillClarity), none: true},
		{name: "no clarity on the fighter", level: 58, skills: []int32{skillClarity}, setup: func(b *Brain) { selfHolds(skillClarity)(b); memberHolds(rowPC2, skillClarity)(b) }, none: true},
	})
}

// rechargeCases are the Elders' Recharge cases: a mystic member under 40 %
// MP gets it; the bot itself under 40 % does not, nor does a member of a
// class that knows Recharge; a member at 40 % does not.
func rechargeCases(heals []int32) []healCase {
	fighter := partyRow(pcID, HumanFighter.ID, 100, 100)
	mpParty := func(other PartyMember, selfMP int32) func(b *Brain, now time.Time) {
		return func(b *Brain, _ time.Time) {
			healerParty(b, fighter, other)
			b.world.Self.MP = selfMP
		}
	}
	cases := []healCase{
		{name: "recharge on a mystic under 40 % MP", level: 40, skills: heals, setup: mpParty(partyRow(rowPC2, classHumanMystic, 100, 30), 100), skill: skillRecharge, object: rowPC2},
		{name: "no recharge at 40 % MP", level: 40, skills: heals, setup: mpParty(partyRow(rowPC2, classHumanMystic, 100, 40), 100), none: true},
		{name: "never on the bot", level: 40, skills: heals, setup: mpParty(partyRow(rowPC2, classHumanMystic, 100, 100), 30), none: true},
	}
	for _, c := range rechargeClasses {
		cases = append(cases, healCase{
			name: fmt.Sprintf("never on class %d", c), level: 40, skills: heals,
			setup: mpParty(partyRow(rowPC2, c, 100, 10), 100), none: true,
		})
	}
	return cases
}

// TestRowSE checks the shillien elder: Wind Shackle once on the leader's
// target, then Wind Strike; Greater Group Heal from 48 with two members
// under 60 %, Group Heal before; Recharge as the elven elder; Wild Magic on
// the mystic members from 62.
func TestRowSE(t *testing.T) {
	assisting := func(b *Brain, now time.Time) {
		inParty(b, 100, 100)
		b.assist = assistPlan{alarm: alarm{at: t0}, target: npcID, seen: now}
	}
	nukes := []int32{skillWindShackle, skillWindStrike}
	runMysticCases(t, "", []mysticCase{
		{name: "shackles the assist target", class: classShillienElder, level: 40, role: RoleSupport, skills: nukes, setup: assisting, skill: skillWindShackle, object: npcID},
		{
			name: "once", class: classShillienElder, level: 40, role: RoleSupport, skills: nukes,
			setup: func(b *Brain, now time.Time) { assisting(b, now); b.opened = npcID }, skill: skillWindStrike, object: npcID,
		},
	})
	heals := []int32{skillBattleHeal, skillGreaterGroupHeal, skillGroupHeal, skillGreaterHeal, skillHeal, skillRecharge}
	runHealCases(t, classShillienElder, append(rechargeCases(heals), []healCase{
		{name: "greater group heal at 48", level: 48, skills: heals, setup: party(50, 50), skill: skillGreaterGroupHeal, object: selfID},
		{name: "group heal at 47", level: 47, skills: heals, setup: party(50, 50), skill: skillGroupHeal, object: selfID},
		{name: "greater heal at 48", level: 48, skills: heals, setup: party(50, 100), skill: skillGreaterHeal, object: pcID},
	}...))
	runRebuffCases(t, classShillienElder, []rebuffCase{
		{name: "wild magic on the mystic at 62", level: 62, skills: []int32{skillWildMagic}, setup: selfHolds(skillWildMagic), skill: skillWildMagic, object: rowPC2},
		{name: "no wild magic at 61", level: 61, skills: []int32{skillWildMagic}, setup: selfHolds(skillWildMagic), none: true},
		{name: "guidance on the fighter", level: 40, skills: []int32{skillGuidance}, skill: skillGuidance, object: pcID},
	})
}

// TestGearRoleHealerHeal checks gearRole values a healer by its first
// known HP heal of type HEAL: a Bishop at 44 knowing Restore Life
// (HEAL_PERCENT, first in its heals) and Greater Battle Heal by Greater
// Battle Heal; an Elder knowing Recharge (MANARECHARGE) and Heal by Heal;
// one knowing only Recharge heals nobody, a tank.
func TestGearRoleHealerHeal(t *testing.T) {
	restoreLife := datapack.Skill{Type: "HEAL_PERCENT", OperateType: "ACTIVE", Power: 20, HitTime: 4 * time.Second, ReuseDelay: 120 * time.Second, MPConsume: 60}
	greaterBattleHeal := datapack.Skill{Type: "HEAL", OperateType: "ACTIVE", Power: 300, HitTime: 2 * time.Second, ReuseDelay: 3 * time.Second, MPConsume: 40}
	heal := datapack.Skill{Type: "HEAL", OperateType: "ACTIVE", Power: 200, HitTime: 5 * time.Second, ReuseDelay: 2 * time.Second, MPConsume: 30}
	rechargeSk := datapack.Skill{Type: "MANARECHARGE", OperateType: "ACTIVE", Power: 100, HitTime: 3 * time.Second, ReuseDelay: 12 * time.Second, MPConsume: 50}
	for _, tt := range []struct {
		name   string
		class  int32
		level  int32
		known  map[int32]datapack.Skill
		want   datapack.Skill
		isTank bool
	}{
		{"bishop by greater battle heal", classBishop, 44, map[int32]datapack.Skill{skillRestoreLife: restoreLife, skillGreaterBattleHeal: greaterBattleHeal}, greaterBattleHeal, false},
		{"elder by heal", classElvenElder, 44, map[int32]datapack.Skill{skillRecharge: rechargeSk, skillHeal: heal}, heal, false},
		{"elder with only recharge", classElvenElder, 44, map[int32]datapack.Skill{skillRecharge: rechargeSk}, datapack.Skill{}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var known []KnownSkill
			for id := range tt.known {
				known = append(known, KnownSkill{ID: id, Level: 1})
			}
			b := newModelBrain(t, tt.class, tt.level, RoleSupport, known, nil)
			m := b.cfg.Travel
			m.skills = maps.Clone(m.skills)
			for id, sk := range tt.known {
				m.skills[datapack.SkillRef{ID: id, Level: 1}] = sk
			}
			role, tank := b.gearRole()
			if tank != tt.isTank {
				t.Fatalf("gearRole tank %v, want %v (%+v)", tank, tt.isTank, role)
			}
			if tt.isTank {
				return
			}
			w := tt.want
			if role.Kind != charmodel.Support || role.HealPower != w.Power || role.HealMP != float64(w.MPConsume) ||
				role.HealHitTime != w.HitTime || role.HealReuse != w.ReuseDelay || !role.HealMAtk {
				t.Errorf("gearRole = %+v, want the heal %+v", role, w)
			}
		})
	}
}

// TestFighterClasses checks fighterClasses and mysticClasses split every
// class id of the profession table, each in one of them.
func TestFighterClasses(t *testing.T) {
	for id := range int32(lastClassID) {
		_, ok := datapack.ParentClass(id)
		f, m := slices.Contains(fighterClasses, id), slices.Contains(mysticClasses, id)
		if ok && f == m || !ok && (f || m) {
			t.Errorf("class %d: in the table %v, fighter %v, mystic %v", id, ok, f, m)
		}
	}
}
