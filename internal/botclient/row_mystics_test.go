package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The mystic rows (docs/strategy.md, "Behavior per class", rows HM EM DM OM
// CL HW EO EW SO DW OS; "Testing", TestRow<ID>): each test compiles the real
// row for its class, level and role and checks the row's key behavior.

// rowPC2 is the third member of the row tests' party.
const rowPC2 = int32(0x20000003)

// mysticBrain is rowBrain for a mystic of class at level playing role.
func mysticBrain(t *testing.T, class, level int32, role Role, weapon string, skills ...int32) *Brain {
	t.Helper()
	b := rowBrain(t, class, level, weapon, skills...)
	b.persona.Role = role
	b.builtOK = false
	return b
}

// inParty puts the bot in pcID's party with rowPC2, both beside it, at the
// HP given out of 100.
func inParty(b *Brain, leaderHP, otherHP int32) {
	b.world.Apply(CharInfo{ObjectID: pcID, Name: "other", Pos: Point{Y: 300}}, t0)
	b.world.Apply(CharInfo{ObjectID: rowPC2, Name: "third", Pos: Point{X: -300}}, t0)
	b.world.Apply(PartySmallWindowAll{LeaderID: pcID, Members: []PartyMember{
		{ObjectID: pcID, HP: leaderHP, MaxHP: 100, ClassID: HumanFighter.ID},
		{ObjectID: rowPC2, HP: otherHP, MaxHP: 100, ClassID: classHumanMystic},
	}}, t0)
}

// moveNPC puts npcID x away from the bot.
func moveNPC(b *Brain, x int32) {
	b.world.Apply(StopMove{ObjectID: npcID, Pos: Point{X: x}}, t0)
}

// castRange gives the travel map skill's castRange at level 1.
func castRange(b *Brain, skill, r int32) {
	if b.cfg.Travel.castRanges == nil {
		b.cfg.Travel.castRanges = make(map[datapack.SkillRef]int32)
	}
	b.cfg.Travel.castRanges[datapack.SkillRef{ID: skill, Level: 1}] = r
}

// hold keeps skills on reuse for a minute after now.
func hold(b *Brain, now time.Time, skills ...int32) {
	for _, id := range skills {
		b.world.holdSkill(id, now.Add(time.Minute))
	}
}

// fightNothing fails unless fight on npcID at now sends nothing.
func fightNothing(t *testing.T, b *Brain, now time.Time) {
	t.Helper()
	if a := b.fight(npcID, now); a.kind != actHold {
		t.Fatalf("fight = kind %d skill %d object %#x, want nothing", a.kind, a.skill, a.object)
	}
}

// mysticCase is one fight decision of a mystic row: the bot of class at
// level in role, knowing skills, after setup, casts skill on object, or
// melees for skill 0, or sends nothing with none set.
type mysticCase struct {
	name         string
	class, level int32
	role         Role
	skills       []int32
	setup        func(b *Brain, now time.Time)
	skill        int32
	object       int32
	none         bool
}

func runMysticCases(t *testing.T, weapon string, cases []mysticCase) {
	t.Helper()
	now := at(time.Second)
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			b := mysticBrain(t, tt.class, tt.level, tt.role, weapon, tt.skills...)
			if tt.setup != nil {
				tt.setup(b, now)
			}
			if tt.none {
				fightNothing(t, b, now)
				return
			}
			fightCast(t, b, now, tt.skill, tt.object)
		})
	}
}

// mp sets the bot's MP percent; hp its HP percent.
func mp(pct int32) func(b *Brain, now time.Time) {
	return func(b *Brain, _ time.Time) { b.world.Self.MP = pct }
}

func hp(pct int32) func(b *Brain, now time.Time) {
	return func(b *Brain, _ time.Time) { b.world.Self.HP = pct }
}

// TestRowHM checks the human mystic: the Damage role nukes Ice Bolt, then
// Wind Strike while it cools, down to its 10 % MPReserve and melees under
// it; the Support role stops at 50 %; Self Heal under 40 % HP, Vampiric
// Touch under 60 % from 14; only the Support role heals.
func TestRowHM(t *testing.T) {
	all := []int32{skillIceBolt, skillWindStrike, skillSelfHeal, skillVampiricTouch}
	hm := func(name string, level int32, role Role, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classHumanMystic, level: level, role: role, skills: all, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "", []mysticCase{
		hm("damage ice bolt", 7, RoleDamage, nil, skillIceBolt, npcID),
		hm("damage wind strike while ice bolt cools", 7, RoleDamage, func(b *Brain, now time.Time) { hold(b, now, skillIceBolt) }, skillWindStrike, npcID),
		hm("damage before 7", 6, RoleDamage, nil, skillWindStrike, npcID),
		hm("damage at 15 % MP", 7, RoleDamage, mp(15), skillIceBolt, npcID),
		hm("damage under 10 % MP melees", 7, RoleDamage, mp(5), 0, 0),
		hm("support at 60 % MP", 7, RoleSupport, mp(60), skillIceBolt, npcID),
		hm("support under 50 % MP melees", 7, RoleSupport, mp(40), 0, 0),
		hm("self heal under 40 % HP", 7, RoleDamage, hp(30), skillSelfHeal, selfID),
		hm("vampiric touch under 60 % HP", 14, RoleDamage, hp(50), skillVampiricTouch, npcID),
		hm("no vampiric touch before 14", 13, RoleDamage, hp(50), skillIceBolt, npcID),
		{
			name: "support above 50 % MP with its nukes cooling sends nothing", class: classHumanMystic, level: 7, role: RoleSupport, skills: all,
			setup: func(b *Brain, now time.Time) { hold(b, now, skillIceBolt, skillWindStrike) }, none: true,
		},
		// TestLiveSupport: a level 15 mystic seeded only its heals and
		// buffs knows no nuke, and melees its leader's target.
		{name: "support knowing no nuke melees", class: classHumanMystic, level: 15, role: RoleSupport, skills: []int32{skillHeal, skillBattleHeal}},
		{name: "damage knowing only self heal melees", class: classHumanMystic, level: 15, role: RoleDamage, skills: []int32{skillSelfHeal}},
	})
	now := at(time.Second)
	for _, tt := range []struct {
		role Role
		heal bool
	}{{RoleSupport, true}, {RoleDamage, false}} {
		b := mysticBrain(t, classHumanMystic, 14, tt.role, "", skillHeal, skillBattleHeal, skillGroupHeal)
		inParty(b, 30, 100)
		b.world.Target = pcID
		a, ok := b.ruleHeal(now)
		if ok != tt.heal {
			t.Fatalf("%v: ruleHeal = kind %d skill %d %v, want a heal %v", tt.role, a.kind, a.skill, ok, tt.heal)
		}
		if tt.heal {
			wantCast(t, a, skillBattleHeal, pcID)
		}
	}
}

// TestRowEM checks the elven mystic: the Damage role opens with Wind
// Shackle from 14 above 50 % MP, else Ice Bolt; the Support role never
// shackles; Self Heal under 50 % HP.
func TestRowEM(t *testing.T) {
	all := []int32{skillWindShackle, skillIceBolt, skillWindStrike, skillSelfHeal}
	em := func(name string, level int32, role Role, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classElvenMystic, level: level, role: role, skills: all, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "", []mysticCase{
		em("damage opens with wind shackle", 14, RoleDamage, nil, skillWindShackle, npcID),
		em("damage at 50 % MP", 14, RoleDamage, mp(50), skillIceBolt, npcID),
		em("damage before 14", 13, RoleDamage, nil, skillIceBolt, npcID),
		em("damage once opened", 14, RoleDamage, func(b *Brain, _ time.Time) { b.opened = npcID }, skillIceBolt, npcID),
		em("support never shackles", 14, RoleSupport, nil, skillIceBolt, npcID),
		em("self heal under 50 % HP", 14, RoleDamage, hp(45), skillSelfHeal, selfID),
	})
}

// TestRowDM checks the dark mystic: the Damage role opens with Curse:
// Poison from 7 above 60 % MP; Vampiric Touch under 70 % HP from 14, Self
// Heal under 40 %.
func TestRowDM(t *testing.T) {
	all := []int32{skillCursePoison, skillIceBolt, skillWindStrike, skillVampiricTouch, skillSelfHeal}
	dm := func(name string, level int32, role Role, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classDarkMystic, level: level, role: role, skills: all, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "", []mysticCase{
		dm("damage opens with curse: poison", 7, RoleDamage, nil, skillCursePoison, npcID),
		dm("damage at 60 % MP", 7, RoleDamage, mp(60), skillIceBolt, npcID),
		dm("support never curses", 7, RoleSupport, nil, skillIceBolt, npcID),
		dm("vampiric touch under 70 % HP", 14, RoleDamage, hp(65), skillVampiricTouch, npcID),
		dm("self heal under 40 % HP", 14, RoleDamage, hp(35), skillSelfHeal, selfID),
	})
}

// TestRowOM checks the orc mystic: Soul Cry goes on at engage above 30 %
// MP and off before it sits; Chill Flame opens from 7, then Life Drain;
// the Support role melees under its 50 % MPReserve, the Damage role only
// under its 30 %.
func TestRowOM(t *testing.T) {
	nukes := []int32{skillChillFlame, skillLifeDrain}
	om := func(name string, role Role, skills []int32, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classOrcMystic, level: 7, role: role, skills: skills, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "", []mysticCase{
		om("soul cry on at engage", RoleDamage, append([]int32{skillSoulCry}, nukes...), nil, skillSoulCry, selfID),
		om("soul cry stays off at 30 % MP", RoleDamage, append([]int32{skillSoulCry}, nukes...), mp(30), skillChillFlame, npcID),
		om("chill flame opens", RoleDamage, nukes, nil, skillChillFlame, npcID),
		om("life drain once opened", RoleDamage, nukes, func(b *Brain, _ time.Time) { b.opened = npcID }, skillLifeDrain, npcID),
		om("damage at 40 % MP", RoleDamage, nukes, mp(40), skillChillFlame, npcID),
		om("support under 50 % MP melees", RoleSupport, nukes, mp(40), 0, 0),
		om("life drain under 70 % HP", RoleSupport, nukes, func(b *Brain, _ time.Time) { b.world.Self.HP, b.world.Self.MP = 60, 40 }, skillLifeDrain, npcID),
	})
	now := at(time.Second)
	b := mysticBrain(t, classOrcMystic, 7, RoleDamage, "", skillSoulCry)
	togglesOn(b, skillSoulCry)
	s := &b.world.Self
	s.InCombat, s.HP = false, 30
	rule, a := b.decide(now)
	if rule != "rest" {
		t.Fatalf("decide = %q kind %d, want rest", rule, a.kind)
	}
	wantCast(t, a, skillSoulCry, selfID)
}

// checkHealerHeals checks a healer row of class at level: a leader under 35 %
// gets Battle Heal first, two members under 60 % a Group Heal on the bot,
// one a Heal.
func checkHealerHeals(t *testing.T, class, level int32) {
	t.Helper()
	now := at(time.Second)
	for _, tt := range []struct {
		name              string
		leaderHP, otherHP int32
		skill, object     int32
	}{
		{"leader under 35 %", 30, 50, skillBattleHeal, pcID},
		{"two under 60 %", 50, 50, skillGroupHeal, selfID},
		{"one under 60 %", 50, 100, skillHeal, pcID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := mysticBrain(t, class, level, RoleSupport, "", skillHeal, skillBattleHeal, skillGroupHeal)
			inParty(b, tt.leaderHP, tt.otherHP)
			b.world.Target = pcID
			a, ok := b.ruleHeal(now)
			if !ok {
				t.Fatal("ruleHeal: no heal")
			}
			wantCast(t, a, tt.skill, tt.object)
		})
	}
}

// TestRowCL checks the cleric: the healers' heals; Wind Strike on the
// leader's target in a party, none on another, and no melee there; alone
// it nukes above its 50 % MPReserve and melees under it; Acumen goes to the
// mystic members only.
func TestRowCL(t *testing.T) {
	checkHealerHeals(t, classCleric, 20)
	assisting := func(b *Brain, now time.Time) {
		inParty(b, 100, 100)
		b.assist = assistPlan{target: npcID, at: t0, seen: now}
	}
	cl := func(name string, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classCleric, level: 20, role: RoleSupport, skills: []int32{skillWindStrike}, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "", []mysticCase{
		cl("the leader's target", assisting, skillWindStrike, npcID),
		{
			name: "another target in a party", class: classCleric, level: 20, role: RoleSupport, skills: []int32{skillWindStrike},
			setup: func(b *Brain, _ time.Time) { inParty(b, 100, 100) }, none: true,
		},
		{
			name: "in a party under 50 % MP", class: classCleric, level: 20, role: RoleSupport, skills: []int32{skillWindStrike},
			setup: func(b *Brain, now time.Time) { assisting(b, now); b.world.Self.MP = 40 }, none: true,
		},
		cl("alone", nil, skillWindStrike, npcID),
		cl("alone under 50 % MP", mp(40), 0, 0),
	})

	now := at(time.Second)
	b := mysticBrain(t, classCleric, 20, RoleSupport, "", skillAcumen)
	inParty(b, 100, 100)
	b.world.Self.InCombat = false
	b.world.Apply(AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillAcumen, Level: 1, Left: time.Hour}}}, t0)
	b.world.Target = rowPC2
	a, ok := b.ruleRebuff(now)
	if !ok {
		t.Fatal("ruleRebuff: no buff")
	}
	wantCast(t, a, skillAcumen, rowPC2)
}

// TestRowHW checks the human wizard: Blaze opens from 750; Energy Bolt
// only within 400 and Aura Burn only within 150, nothing between;
// Vampiric Touch under 60 % HP; Body To Mind from 25 with MP under 30 %
// and HP above 70 %; melee only under its 10 % MPReserve.
func TestRowHW(t *testing.T) {
	all := []int32{skillBlaze, skillIceBolt, skillEnergyBolt, skillAuraBurn, skillVampiricTouch, skillBodyToMind}
	near := func(x int32, held ...int32) func(b *Brain, now time.Time) {
		return func(b *Brain, now time.Time) {
			moveNPC(b, x)
			castRange(b, skillBlaze, 750)
			hold(b, now, held...)
		}
	}
	hw := func(name string, level int32, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classHumanWizard, level: level, role: RoleDamage, skills: all, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "", []mysticCase{
		hw("blaze from 700", 20, near(700), skillBlaze, npcID),
		hw("energy bolt within 400", 20, near(300, skillBlaze, skillIceBolt), skillEnergyBolt, npcID),
		{name: "nothing at 450", class: classHumanWizard, level: 20, role: RoleDamage, skills: all, setup: near(450, skillBlaze, skillIceBolt), none: true},
		hw("aura burn within 150", 20, near(100, skillBlaze, skillIceBolt, skillEnergyBolt), skillAuraBurn, npcID),
		{name: "no aura burn at 200", class: classHumanWizard, level: 20, role: RoleDamage, skills: all, setup: near(200, skillBlaze, skillIceBolt, skillEnergyBolt), none: true},
		hw("vampiric touch under 60 % HP", 20, hp(50), skillVampiricTouch, npcID),
		hw("body to mind", 25, func(b *Brain, _ time.Time) { b.world.Self.MP, b.world.Self.HP = 20, 80 }, skillBodyToMind, selfID),
		hw("no body to mind at 70 % HP", 25, func(b *Brain, _ time.Time) { b.world.Self.MP, b.world.Self.HP = 20, 70 }, skillBlaze, npcID),
		hw("melee under 10 % MP", 20, mp(5), 0, 0),
	})
}

// TestRowEO checks the elven oracle: the healers' heals; Dryad Root from
// 25 on a second attacker, once: it selects that attacker, casts on it,
// then nukes the first; with one attacker Wind Strike.
func TestRowEO(t *testing.T) {
	checkHealerHeals(t, classElvenOracle, 25)
	now := at(time.Second)
	b := mysticBrain(t, classElvenOracle, 25, RoleSupport, "", skillDryadRoot, skillWindStrike)
	fightCast(t, b, now, skillWindStrike, npcID)
	hitByTwo(b)
	if a := b.fight(npcID, now); a.kind != actSelect || a.object != rowNPC2 {
		t.Fatalf("fight = kind %d object %#x, want a select of the second attacker", a.kind, a.object)
	}
	b.world.Target = rowNPC2
	wantCast(t, b.fight(npcID, now), skillDryadRoot, rowNPC2)
	if a := b.fight(npcID, now); a.kind != actSelect || a.object != npcID {
		t.Fatalf("fight = kind %d object %#x, want the fight's target selected again", a.kind, a.object)
	}
	b.world.Target = npcID
	fightCast(t, b, now, skillWindStrike, npcID)

	// The rooted attacker dies; the server gives its object id to the
	// respawn, which attacks as a fresh second attacker and is rooted too.
	b = mysticBrain(t, classElvenOracle, 25, RoleSupport, "", skillDryadRoot, skillWindStrike)
	fightCast(t, b, now, skillWindStrike, npcID)
	hitByTwo(b)
	b.fight(npcID, now)
	b.world.Target = rowNPC2
	wantCast(t, b.fight(npcID, now), skillDryadRoot, rowNPC2)
	mustHandle(t, b, now, Die{ObjectID: rowNPC2})
	b.world.Target = npcID
	hitByTwo(b)
	if a := b.fight(npcID, now); a.kind != actSelect || a.object != rowNPC2 {
		t.Fatalf("after the respawn: fight = kind %d object %#x, want a select of the second attacker", a.kind, a.object)
	}

	b = mysticBrain(t, classElvenOracle, 24, RoleSupport, "", skillDryadRoot, skillWindStrike)
	hitByTwo(b)
	fightCast(t, b, now, skillWindStrike, npcID)
}

// TestRowEW checks the elven wizard: Wind Shackle opens above 50 % MP,
// else Solar Spark from 25, Aqua Swirl before; Self Heal under 40 % HP.
func TestRowEW(t *testing.T) {
	all := []int32{skillWindShackle, skillSolarSpark, skillAquaSwirl, skillIceBolt, skillEnergyBolt, skillAuraBurn, skillSelfHeal}
	ew := func(name string, level int32, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classElvenWizard, level: level, role: RoleDamage, skills: all, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "", []mysticCase{
		ew("wind shackle opens", 25, nil, skillWindShackle, npcID),
		ew("solar spark at 50 % MP", 25, mp(50), skillSolarSpark, npcID),
		ew("aqua swirl before 25", 24, mp(50), skillAquaSwirl, npcID),
		ew("self heal under 40 % HP", 25, hp(30), skillSelfHeal, selfID),
	})
}

// TestRowSO checks the shillien oracle: the healers' heals; Wind Shackle
// once on the assist target, then Wind Strike; nothing on another target
// in a party.
func TestRowSO(t *testing.T) {
	checkHealerHeals(t, classShillienOracle, 20)
	assisting := func(b *Brain, now time.Time) {
		inParty(b, 100, 100)
		b.assist = assistPlan{target: npcID, at: t0, seen: now}
	}
	all := []int32{skillWindShackle, skillWindStrike}
	runMysticCases(t, "", []mysticCase{
		{name: "shackles the assist target", class: classShillienOracle, level: 20, role: RoleSupport, skills: all, setup: assisting, skill: skillWindShackle, object: npcID},
		{
			name: "once", class: classShillienOracle, level: 20, role: RoleSupport, skills: all,
			setup: func(b *Brain, now time.Time) { assisting(b, now); b.opened = npcID }, skill: skillWindStrike, object: npcID,
		},
		{
			name: "not another target", class: classShillienOracle, level: 20, role: RoleSupport, skills: all,
			setup: func(b *Brain, _ time.Time) { inParty(b, 100, 100) }, none: true,
		},
	})
}

// TestRowDW checks the dark wizard: Curse: Poison opens above 60 % MP,
// else Shadow Spark from 25, Twister before; Vampiric Touch under 70 % HP.
func TestRowDW(t *testing.T) {
	all := []int32{skillCursePoison, skillShadowSpark, skillTwister, skillIceBolt, skillAuraBurn, skillVampiricTouch, skillBodyToMind}
	dw := func(name string, level int32, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classDarkWizard, level: level, role: RoleDamage, skills: all, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "", []mysticCase{
		dw("curse: poison opens", 25, nil, skillCursePoison, npcID),
		dw("shadow spark at 60 % MP", 25, mp(60), skillShadowSpark, npcID),
		dw("twister before 25", 24, mp(60), skillTwister, npcID),
		dw("vampiric touch under 70 % HP", 25, hp(65), skillVampiricTouch, npcID),
	})
}

// TestRowOS checks the orc shaman: Frost Flame opens from 750, then Life
// Drain; Stun Attack with a blunt at any distance (it melees anyway), else
// melee; Chant of Life
// on the bot with a member under 50 %; holding Soul Shield level 2 it casts
// no Chant of Shielding while no member qualifies.
func TestRowOS(t *testing.T) {
	all := []int32{skillFrostFlame, skillLifeDrain, skillStunAttack}
	osc := func(name string, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classOrcShaman, level: 20, role: RoleDamage, skills: all, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "BLUNT", []mysticCase{
		osc("frost flame from 700", func(b *Brain, _ time.Time) { moveNPC(b, 700); castRange(b, skillFrostFlame, 750) }, skillFrostFlame, npcID),
		osc("life drain once opened", func(b *Brain, _ time.Time) { b.opened = npcID }, skillLifeDrain, npcID),
		osc("stun attack in melee", func(b *Brain, now time.Time) { b.opened = npcID; hold(b, now, skillLifeDrain) }, skillStunAttack, npcID),
		osc("stun attack beyond 40", func(b *Brain, now time.Time) { b.opened = npcID; moveNPC(b, 60); hold(b, now, skillLifeDrain) }, skillStunAttack, npcID),
		osc("melee under 30 % MP", func(b *Brain, _ time.Time) { b.opened = npcID; b.world.Self.MP = 20 }, 0, 0),
	})
	runMysticCases(t, "", []mysticCase{{
		name: "no stun attack bare-handed", class: classOrcShaman, level: 20, role: RoleDamage, skills: all,
		setup: func(b *Brain, now time.Time) { b.opened = npcID; hold(b, now, skillLifeDrain) },
	}})

	now := at(time.Second)
	for _, tt := range []struct {
		leaderHP int32
		cast     bool
	}{{40, true}, {60, false}} {
		b := mysticBrain(t, classOrcShaman, 20, RoleSupport, "", skillChantLife)
		inParty(b, tt.leaderHP, 100)
		a, ok := b.ruleHeal(now)
		if ok != tt.cast {
			t.Fatalf("leader at %d: ruleHeal = kind %d skill %d %v, want a cast %v", tt.leaderHP, a.kind, a.skill, ok, tt.cast)
		}
		if tt.cast {
			wantCast(t, a, skillChantLife, selfID)
		}
	}

	for _, tt := range []struct {
		name string
		x    int32
		cast bool
	}{{"no member qualifies", 2000, false}, {"a member within reach", 300, true}} {
		t.Run(tt.name, func(t *testing.T) {
			b := mysticBrain(t, classOrcShaman, 20, RoleSupport, "", skillChantShielding)
			b.cfg.Travel = stackTravel()
			s := &b.world.Self
			s.InCombat = false
			b.world.Apply(SkillList{Skills: []KnownSkill{{ID: skillChantShielding, Level: 1}}}, t0)
			b.world.Apply(AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillSoulShield, Level: 2, Left: time.Hour}}}, t0)
			b.world.Apply(CharInfo{ObjectID: pcID, Name: "mate", Pos: Point{X: tt.x}}, t0)
			b.world.Apply(PartySmallWindowAll{LeaderID: selfID, Members: []PartyMember{{ObjectID: pcID, HP: 100, MaxHP: 100}}}, t0)
			a, ok := b.ruleRebuff(now)
			if ok != tt.cast {
				t.Fatalf("ruleRebuff = kind %d skill %d %v, want a cast %v", a.kind, a.skill, ok, tt.cast)
			}
			if tt.cast {
				wantCast(t, a, skillChantShielding, selfID)
			}
		})
	}
}

// TestCompileMysticBuild checks a damage and a support mystic of the same
// base class follow their own row parts: the Support role's heals, buffs,
// learn table, 50 % MPReserve and Sit MP, the Damage role's opener and
// lower reserve; the orc mystic's Support role learns and wears the armor
// masteries and stands in a fight, its Damage role stays in a robe. A
// Crafter plays the row's role-neutral part with the Damage values.
func TestCompileMysticBuild(t *testing.T) {
	for _, tt := range []struct {
		race, class         int32
		role                Role
		heals, buffs, armor int
		opener              int32
		reserve, sitMP      int
		melee               MeleePolicy
		learns              []int32
	}{
		{RaceHuman, classHumanMystic, RoleSupport, 3, 2, 0, 0, 50, 30, MeleeUnderReserve, []int32{skillGroupHeal}},
		{RaceHuman, classHumanMystic, RoleDamage, 0, 0, 0, 0, 10, 20, MeleeUnderReserve, []int32{skillIceBolt}},
		{RaceHuman, classHumanMystic, RoleCrafter, 0, 0, 0, 0, 10, 20, MeleeUnderReserve, []int32{skillIceBolt}},
		{RaceElf, classElvenMystic, RoleSupport, 3, 2, 0, 0, 50, 30, MeleeUnderReserve, []int32{skillHeal}},
		{RaceElf, classElvenMystic, RoleDamage, 0, 0, 0, skillWindShackle, 10, 20, MeleeUnderReserve, []int32{skillWindShackle}},
		{RaceDarkElf, classDarkMystic, RoleSupport, 3, 2, 0, 0, 50, 30, MeleeUnderReserve, []int32{skillHeal}},
		{RaceDarkElf, classDarkMystic, RoleDamage, 0, 0, 0, skillCursePoison, 10, 20, MeleeUnderReserve, []int32{skillCursePoison}},
		{RaceOrc, classOrcMystic, RoleSupport, 0, 2, 2, skillChillFlame, 50, 30, MeleeAlways, []int32{skillLightArmorMastery, skillHeavyArmorMastery}},
		{RaceOrc, classOrcMystic, RoleDamage, 0, 2, 0, skillChillFlame, 30, 25, MeleeUnderReserve, []int32{skillRobeMastery}},
		{RaceOrc, classOrcShaman, RoleSupport, 1, 4, 2, skillFrostFlame, 50, 25, MeleeAlways, []int32{skillHeavyArmorMastery}},
		{RaceOrc, classOrcShaman, RoleDamage, 1, 4, 0, skillFrostFlame, 30, 25, MeleeAlways, []int32{skillHeavyArmorMastery}},
	} {
		level := int32(19)
		if tt.class == classOrcShaman {
			level = 25
		}
		bd := CompileBuild(tt.race, tt.class, level, tt.role, BranchChoice{})
		k := bd.Kit
		opener := int32(0)
		if i := slices.IndexFunc(k.Rotation, func(s Step) bool { return s.Opener }); i >= 0 {
			opener = k.Rotation[i].Skill
		}
		if len(k.Heals) != tt.heals || len(k.Buffs) != tt.buffs || len(bd.Race.MysticArmor) != tt.armor || opener != tt.opener {
			t.Errorf("class %d %v: heals %d buffs %d armor %d opener %d, want %d %d %d %d",
				tt.class, tt.role, len(k.Heals), len(k.Buffs), len(bd.Race.MysticArmor), opener, tt.heals, tt.buffs, tt.armor, tt.opener)
		}
		if bd.Thresholds.MPReserve != tt.reserve || bd.Thresholds.SitMP != tt.sitMP || k.Melee != tt.melee {
			t.Errorf("class %d %v: MPReserve %d SitMP %d melee %d, want %d %d %d",
				tt.class, tt.role, bd.Thresholds.MPReserve, bd.Thresholds.SitMP, k.Melee, tt.reserve, tt.sitMP, tt.melee)
		}
		for _, id := range tt.learns {
			if !slices.Contains(k.Skills, id) {
				t.Errorf("class %d %v learns %v, want %d among them", tt.class, tt.role, k.Skills, id)
			}
		}
	}
	if k := CompileBuild(RaceOrc, classOrcMystic, 19, RoleDamage, BranchChoice{}).Kit; slices.Contains(k.Skills, skillHeavyArmorMastery) {
		t.Errorf("orc mystic Damage learns %v, want no armor mastery but the robe's", k.Skills)
	}
	// Second professions play their first profession's row (#332).
	for _, tt := range []struct{ class, first int32 }{{16, classCleric}, {12, classHumanWizard}, {30, classElvenOracle}, {41, classDarkWizard}, {52, classOrcShaman}} {
		race, _ := datapack.ClassRace(tt.class)
		bd, want := CompileBuild(race, tt.class, 45, RoleSupport, BranchChoice{}), CompileBuild(race, tt.first, 45, RoleSupport, BranchChoice{})
		if !slices.Equal(stepSkills(bd.Kit.Rotation), stepSkills(want.Kit.Rotation)) || bd.Thresholds != want.Thresholds {
			t.Errorf("class %d rotation %v, want class %d's %v", tt.class, stepSkills(bd.Kit.Rotation), tt.first, stepSkills(want.Kit.Rotation))
		}
	}
}
