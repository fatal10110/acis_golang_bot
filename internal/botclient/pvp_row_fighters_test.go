package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The fighter PvP rows (docs/pvp/rows.md "Tests", TestPvPRow<ID>): each
// test plays the row's real compiled build on the rows' skill fixture
// (rowSkills) against a player: its opener against the named archetype,
// its Hold and Int step, its Break step.

// Classes of the row tests' opponents besides the layer's (Warrior 1,
// Human Mystic 10, Cleric 15): the Orc Fighter and the Orc Mystic.
const (
	rowOrcFighter = int32(44)
	rowOrcMystic  = int32(49)
)

// pvpRowBrain is layerBrain against a player of class opp (300 away,
// flagged, hitting the bot) with the bot of class at level playing its real
// compiled build, wielding weapon ("" bare-handed), knowing skills at level
// 1 (rowSkills' entries and cast ranges), and the Orc Fighter, Human Mystic and Orc Mystic
// templates at their real CON and MEN (classes/*.xml).
func pvpRowBrain(t *testing.T, class, level int32, weapon string, opp int32, skills ...int32) *Brain {
	t.Helper()
	b := layerBrain(t, opp)
	if b.cfg.Travel.castRanges == nil {
		b.cfg.Travel.castRanges = map[datapack.SkillRef]int32{}
	}
	for ref, s := range rowSkills {
		b.cfg.Travel.skills[ref] = s
		b.cfg.Travel.castRanges[ref] = s.CastRange
	}
	cl := b.cfg.Travel.classes
	cl[rowOrcFighter] = withStats(layerClass(rowOrcFighter, -1), 47, 27)
	cl[lyrMystic] = withStats(layerClass(lyrMystic, -1), 27, 39)
	cl[rowOrcMystic] = withStats(layerClass(rowOrcMystic, -1), 31, 42)
	s := &b.world.Self
	s.ClassID, s.Level = class, level
	b.builtOK = false
	b.world.Inventory.Items = map[int32]Item{}
	if weapon != "" {
		b.cfg.Travel.gear = map[int32]gearItem{rowWeaponID: {part: partRHand, kind: weapon, tmpl: datapack.Item{Kind: weapon}}}
		b.world.Apply(ItemList{Items: []Item{{ObjectID: rowWeapon, TemplateID: rowWeaponID, Count: 1, Equipped: true, BodySlot: partRHand}}}, t0)
	} else {
		b.cfg.Travel.gear = map[int32]gearItem{}
	}
	known := make([]KnownSkill, len(skills))
	for i, id := range skills {
		known[i] = KnownSkill{ID: id, Level: 1}
	}
	b.world.Apply(SkillList{Skills: known}, t0)
	return b
}

// withStats is c with CON con and MEN men.
func withStats(c datapack.Class, con, men int) datapack.Class {
	c.CON, c.MEN = con, men
	return c
}

// foeOf moves pcID, of class, to p with the abnormal mask.
func foeOf(t *testing.T, b *Brain, class int32, p Point, abnormal int32) {
	t.Helper()
	b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, ClassID: class, Pos: p, RunSpeed: 124, PvPFlag: 1, Abnormal: abnormal, CollisionRadius: 9}, t0)
	b.world.object(pcID).move = stopAt(p)
}

// asArcher makes pcID an archer (a scout with a bow).
func asArcher(b *Brain) {
	r := b.pvp.opponents[pcID]
	r.Archetype, r.Row = charmodel.Archer, charmodel.Row{ID: "RO", Bow: true}
}

// breakingOff meets every break-off condition but a ready sustain: CP
// gone, HP 150 of 1000 (under every row's bound), faster than the
// opponent (150 against 124), the opponent at 800 of 1000; on the test
// geodata, so the flee walks.
func breakingOff(t *testing.T, b *Brain) {
	t.Helper()
	b.cfg.Nav = testNav(t)
	b.walk = newWalker(b.cfg.Nav)
	b.world.SetGround(b.cfg.Nav.Engine)
	s := &b.world.Self
	s.CP, s.MaxCP, s.HP, s.RunSpeed = 0, 500, 150, 150
	r := b.pvp.opponents[pcID]
	r.HP, r.MaxHP = 800, 1000
}

// healing has pcID start a Heal (1011, hit 5000) on itself at 2 s.
func healing(t *testing.T, b *Brain) {
	t.Helper()
	b.handleOne(t, MagicSkillUse{CasterID: pcID, TargetID: pcID, SkillID: skillHeal, Level: 1, HitTime: 5 * time.Second}, at(2*time.Second))
}

// rowCase is one decision of a row against a player: the bot of class at
// level wielding weapon (a shield too when shield), knowing skills,
// against opp, after setup, decides at 1 s (2.6 s with heal: the
// opponent's Heal started at 2 s); want is the cast it makes, or with
// want 0 the action kind (a cast of none of not when kind is 0).
type rowCase struct {
	name   string
	level  int32
	weapon string
	shield bool
	skills []int32
	opp    int32
	setup  func(t *testing.T, b *Brain)
	heal   bool
	want   int32
	kind   actionKind
	not    []int32
}

// playRow plays the cases of class's row.
func playRow(t *testing.T, class int32, cases []rowCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpRowBrain(t, class, tc.level, tc.weapon, tc.opp, tc.skills...)
			if tc.shield {
				wearShield(b)
			}
			if tc.setup != nil {
				tc.setup(t, b)
			}
			now := at(time.Second)
			if tc.heal {
				healing(t, b)
				now = at(2600 * time.Millisecond)
			}
			_, a := b.decide(now)
			switch {
			case tc.want != 0:
				if a.kind != actCast || a.skill != tc.want {
					t.Fatalf("action %v skill %d, want the cast of %d", a.kind, a.skill, tc.want)
				}
			case tc.kind != 0:
				if a.kind != tc.kind {
					t.Fatalf("action %v skill %d, want %v", a.kind, a.skill, tc.kind)
				}
			}
			for _, n := range tc.not {
				if a.kind == actCast && a.skill == n {
					t.Fatalf("cast %d", n)
				}
			}
		})
	}
}

// opened marks pcID as attacked already: no opener goes.
func opened(_ *testing.T, b *Brain) { b.opened = pcID }

// cooling holds skills for a minute.
func cooling(skills ...int32) func(*testing.T, *Brain) {
	return func(_ *testing.T, b *Brain) {
		for _, s := range skills {
			b.world.holdSkill(s, at(time.Minute))
		}
	}
}

// both runs every setup.
func both(setups ...func(*testing.T, *Brain)) func(*testing.T, *Brain) {
	return func(t *testing.T, b *Brain) {
		for _, f := range setups {
			f(t, b)
		}
	}
}

// buffed puts effects of skills on the bot.
func buffed(skills ...int32) func(*testing.T, *Brain) {
	return func(_ *testing.T, b *Brain) {
		ef := make([]Effect, len(skills))
		for i, id := range skills {
			ef[i] = Effect{SkillID: id, Level: 1, Left: rowEffectDuration}
		}
		b.world.Apply(AbnormalStatusUpdate{Effects: ef}, t0)
	}
}

// fighterSkills are HF's, EF's and DF's attacks.
var fighterSkills = []int32{skillPowerShot, skillMortalBlow, skillPowerStrike}

// TestPvPRowHF checks the human fighter's PvP row: with a bow Power Shot
// opens on a Cleric, with a sword Power Strike against a Warrior; no
// Break step: breaking off it flees.
func TestPvPRowHF(t *testing.T) {
	playRow(t, int32(HumanFighter.ID), []rowCase{
		{name: "bow: Power Shot opens", level: 20, weapon: "BOW", skills: fighterSkills, opp: lyrCleric, want: skillPowerShot},
		{name: "sword: Power Strike", level: 20, weapon: "SWORD", skills: fighterSkills, opp: lyrWarrior, want: skillPowerStrike},
		{name: "breaking off: the flee", level: 20, weapon: "SWORD", skills: fighterSkills, opp: lyrWarrior, setup: breakingOff, kind: actMove},
	})
}

// TestPvPRowEF checks the elven fighter's PvP row: as HF; Elemental Heal
// under 50 % HP with no melee opponent in reach (TestPvPLongCastRuleEF has
// the rule); breaking off it flees.
func TestPvPRowEF(t *testing.T) {
	heal := append([]int32{skillElementalHeal}, fighterSkills...)
	playRow(t, int32(ElvenFighter.ID), []rowCase{
		{name: "bow: Power Shot opens", level: 20, weapon: "BOW", skills: fighterSkills, opp: lyrMystic, want: skillPowerShot},
		{name: "sword: Power Strike", level: 20, weapon: "SWORD", skills: fighterSkills, opp: lyrWarrior, want: skillPowerStrike},
		{
			name: "under 50 %, the opponent out of reach: Elemental Heal", level: 20, weapon: "SWORD", skills: heal, opp: lyrWarrior,
			setup: func(_ *testing.T, b *Brain) { b.world.Self.HP = 450 }, want: skillElementalHeal,
		},
		{name: "breaking off: the flee", level: 20, weapon: "SWORD", skills: fighterSkills, opp: lyrWarrior, setup: breakingOff, kind: actMove},
	})
}

// TestPvPRowDF checks the dark elven fighter's PvP row: Drain Health opens
// against any opponent from 300, then Power Strike, and under 60 % HP;
// a glass cannon, it breaks off under 40 % and flees once Drain Health
// cools.
func TestPvPRowDF(t *testing.T) {
	all := append([]int32{skillDrainHealth}, fighterSkills...)
	playRow(t, int32(DarkFighter.ID), []rowCase{
		{name: "Drain Health opens on a Warrior", level: 20, weapon: "SWORD", skills: all, opp: lyrWarrior, want: skillDrainHealth},
		{name: "opened: Power Strike", level: 20, weapon: "SWORD", skills: all, opp: lyrWarrior, setup: opened, want: skillPowerStrike},
		{
			name: "under 60 %: Drain Health", level: 20, weapon: "SWORD", skills: all, opp: lyrWarrior,
			setup: both(opened, func(_ *testing.T, b *Brain) { b.world.Self.HP = 550 }), want: skillDrainHealth,
		},
		{
			name: "at 35 %, Drain Health cooling: the flee", level: 20, weapon: "SWORD", skills: all, opp: lyrWarrior,
			setup: both(breakingOff, cooling(skillDrainHealth), func(_ *testing.T, b *Brain) { b.world.Self.HP = 350 }), kind: actMove,
		},
	})
}

// TestPvPRowOF checks the orc fighter's PvP row: Iron Punch with fist
// weapons; a tank, it fights on at 25 % HP and flees under 20 %.
func TestPvPRowOF(t *testing.T) {
	all := []int32{skillIronPunch, skillPowerStrike}
	playRow(t, int32(OrcFighter.ID), []rowCase{
		{name: "fists: Iron Punch", level: 20, weapon: "DUALFIST", skills: all, opp: lyrMystic, want: skillIronPunch},
		{
			name: "at 25 %: fights on", level: 20, weapon: "DUALFIST", skills: all, opp: lyrWarrior,
			setup: both(breakingOff, func(_ *testing.T, b *Brain) { b.world.Self.HP = 250 }), want: skillIronPunch,
		},
		{name: "under 20 %: the flee", level: 20, weapon: "DUALFIST", skills: all, opp: lyrWarrior, setup: breakingOff, kind: actMove},
	})
}

// TestPvPRowWF checks the dwarven fighter's PvP row: no Spoil on a fresh
// player, the plain attack.
func TestPvPRowWF(t *testing.T) {
	playRow(t, int32(DwarvenFighter.ID), []rowCase{
		{name: "no Spoil: the attack", level: 20, weapon: "BLUNT", skills: []int32{skillSpoil}, opp: lyrWarrior, kind: actAttack, not: []int32{skillSpoil}},
	})
}

// TestPvPRowKN checks the knight's PvP row: Drain Health opens on a
// caster, never Aggression (a Peel only); Shield Stun at reuse against a
// Warrior, held against a Cleric until its Heal; Deflect Arrow against an
// archer; Ultimate Defense only vs M (TestPvPBreakOffUltimateDefense has
// the break-off side).
func TestPvPRowKN(t *testing.T) {
	all := []int32{skillDrainHealth, skillShieldStun, skillPowerStrike, skillAggression, skillUltimateDefense, skillDeflectArrow}
	stuck := func(_ *testing.T, b *Brain) { b.world.Self.RunSpeed = 100 }
	playRow(t, classKnight, []rowCase{
		{name: "Drain Health opens on a Human Mystic", level: 28, weapon: "SWORD", shield: true, skills: all, opp: lyrMystic, want: skillDrainHealth, not: []int32{skillAggression}},
		{name: "a Warrior: Shield Stun at reuse", level: 28, weapon: "SWORD", shield: true, skills: all, opp: lyrWarrior, want: skillShieldStun},
		{name: "a Warrior, Shield Stun cooling: Power Strike, no Aggression", level: 28, weapon: "SWORD", shield: true, skills: all, opp: lyrWarrior, setup: cooling(skillShieldStun), want: skillPowerStrike},
		{name: "a Cleric: Shield Stun held", level: 28, weapon: "SWORD", shield: true, skills: all, opp: lyrCleric, want: skillPowerStrike},
		{
			name: "the Cleric's Heal: Shield Stun", level: 28, weapon: "SWORD", shield: true, skills: all, opp: lyrCleric, heal: true,
			setup: func(t *testing.T, b *Brain) { foeOf(t, b, lyrCleric, Point{Y: 60}, 0) }, want: skillShieldStun,
		},
		{name: "an archer: Deflect Arrow", level: 28, weapon: "SWORD", shield: true, skills: all, opp: lyrWarrior, setup: func(_ *testing.T, b *Brain) { asArcher(b) }, want: skillDeflectArrow},
		{name: "before 24: no Deflect Arrow", level: 23, weapon: "SWORD", shield: true, skills: all, opp: lyrWarrior, setup: func(_ *testing.T, b *Brain) { asArcher(b) }, not: []int32{skillDeflectArrow}},
		{
			name: "under 20 %, unable to outrun a caster: no Ultimate Defense", level: 28, weapon: "SWORD", shield: true, skills: all, opp: lyrMystic,
			setup: both(breakingOff, stuck), not: []int32{skillUltimateDefense},
		},
	})
}

// TestPvPRowWA checks the warlord's PvP row: Lionheart at engage against a
// caster from 36, War Cry against a Warrior; Stun Attack at reuse against
// a Warrior, held against a Cleric until its Heal.
func TestPvPRowWA(t *testing.T) {
	all := []int32{skillLionheart, skillWarCry, skillStunAttack, skillPowerSmash}
	cries := buffed(skillWarCry, skillLionheart)
	playRow(t, int32(1), []rowCase{
		{name: "a Human Mystic: Lionheart", level: 36, weapon: "BIGBLUNT", skills: all, opp: lyrMystic, want: skillLionheart},
		{name: "before 36: War Cry", level: 35, weapon: "BIGBLUNT", skills: all, opp: lyrMystic, want: skillWarCry},
		{name: "a Warrior: no Lionheart, War Cry", level: 36, weapon: "BIGBLUNT", skills: all, opp: lyrWarrior, want: skillWarCry},
		{name: "a Warrior: Stun Attack at reuse", level: 36, weapon: "BIGBLUNT", skills: all, opp: lyrWarrior, setup: cries, want: skillStunAttack},
		{name: "a Cleric: Stun Attack held", level: 36, weapon: "BIGBLUNT", skills: all, opp: lyrCleric, setup: cries, want: skillPowerSmash},
		{
			name: "the Cleric's Heal: Stun Attack", level: 36, weapon: "BIGBLUNT", skills: all, opp: lyrCleric, heal: true,
			setup: both(cries, func(t *testing.T, b *Brain) { foeOf(t, b, lyrCleric, Point{Y: 60}, 0) }), want: skillStunAttack,
		},
	})
}

// TestPvPRowRO checks the rogue's PvP row: with a dagger Dash at engage on
// a caster beyond 400, not within it nor on a Warrior; with a bow
// Stunning Shot at reuse on a Warrior, held on a Cleric until its Heal;
// Dash to break off; Ultimate Evasion under 30 % only vs M, A.
func TestPvPRowRO(t *testing.T) {
	dagger := []int32{skillDash, skillBleed, skillMortalBlow, skillUltimateEvasion}
	bow := []int32{skillStunningShot, skillPowerShot}
	far := func(class int32) func(*testing.T, *Brain) {
		return func(t *testing.T, b *Brain) { foeOf(t, b, class, Point{Y: 900}, 0) }
	}
	low := func(_ *testing.T, b *Brain) { b.world.Self.HP = 250 }
	playRow(t, classRogue, []rowCase{
		{name: "dagger, a Human Mystic at 900: Dash", level: 36, weapon: "DAGGER", skills: dagger, opp: lyrMystic, setup: far(lyrMystic), want: skillDash},
		{name: "dagger, a Human Mystic at 300: Bleed", level: 36, weapon: "DAGGER", skills: dagger, opp: lyrMystic, want: skillBleed},
		{name: "dagger, a Warrior at 900: no Dash", level: 36, weapon: "DAGGER", skills: dagger, opp: lyrWarrior, setup: far(lyrWarrior), want: skillBleed},
		{name: "bow, a Warrior: Stunning Shot at reuse", level: 36, weapon: "BOW", skills: bow, opp: lyrWarrior, want: skillStunningShot},
		{name: "bow, a Cleric: Stunning Shot held", level: 36, weapon: "BOW", skills: bow, opp: lyrCleric, want: skillPowerShot},
		{name: "bow, the Cleric's Heal: Stunning Shot", level: 36, weapon: "BOW", skills: bow, opp: lyrCleric, heal: true, want: skillStunningShot},
		{name: "breaking off: Dash", level: 36, weapon: "DAGGER", skills: dagger, opp: lyrWarrior, setup: breakingOff, want: skillDash},
		{name: "under 30 % against a Warrior: Ultimate Evasion", level: 36, weapon: "DAGGER", skills: dagger, opp: lyrWarrior, setup: low, want: skillUltimateEvasion},
		{name: "under 30 % against a Human Mystic: no Ultimate Evasion", level: 36, weapon: "DAGGER", skills: dagger, opp: lyrMystic, setup: low, not: []int32{skillUltimateEvasion}},
	})
}

// TestPvPRowEK checks the elven knight's PvP row: Entangle on a caster
// from 36, not on a Warrior; never Aggression; Entangle to break off.
func TestPvPRowEK(t *testing.T) {
	all := []int32{skillEntangle, skillPowerStrike, skillAggression}
	playRow(t, classElvenKnight, []rowCase{
		{name: "a Human Mystic: Entangle", level: 36, weapon: "SWORD", skills: all, opp: lyrMystic, want: skillEntangle},
		{name: "a Warrior: no Entangle, no Aggression", level: 36, weapon: "SWORD", skills: all, opp: lyrWarrior, want: skillPowerStrike},
		{name: "breaking off: Entangle", level: 36, weapon: "SWORD", skills: all, opp: lyrWarrior, setup: breakingOff, want: skillEntangle},
	})
}

// TestPvPRowES checks the elven scout's PvP row: Entangle from 36 on a
// Warrior with a bow, on a caster with a dagger; with a bow Stunning Shot
// held on a Cleric until its Heal; Entangle, then Sprint, to break off.
func TestPvPRowES(t *testing.T) {
	bow := []int32{skillEntangle, skillStunningShot, skillPowerShot}
	dagger := []int32{skillEntangle, skillBleed, skillMortalBlow, skillSprint}
	playRow(t, classElvenScout, []rowCase{
		{name: "bow, a Warrior: Entangle", level: 36, weapon: "BOW", skills: bow, opp: lyrWarrior, want: skillEntangle},
		{name: "bow, a Human Mystic: no Entangle, Stunning Shot held", level: 36, weapon: "BOW", skills: bow, opp: lyrMystic, want: skillPowerShot},
		{name: "bow, the Cleric's Heal: Stunning Shot", level: 36, weapon: "BOW", skills: bow, opp: lyrCleric, heal: true, want: skillStunningShot},
		{name: "dagger, a Human Mystic: Entangle", level: 36, weapon: "DAGGER", skills: dagger, opp: lyrMystic, want: skillEntangle},
		{name: "dagger, a Warrior: Bleed", level: 36, weapon: "DAGGER", skills: dagger, opp: lyrWarrior, want: skillBleed},
		{name: "breaking off: Entangle", level: 36, weapon: "DAGGER", skills: dagger, opp: lyrWarrior, setup: breakingOff, want: skillEntangle},
		{name: "breaking off, Entangle cooling: Sprint", level: 36, weapon: "DAGGER", skills: dagger, opp: lyrWarrior, setup: both(breakingOff, cooling(skillEntangle)), want: skillSprint},
	})
}

// TestPvPRowPK checks the palus knight's PvP row: Drain Health opens on
// any opponent, then Poison on a Cleric; Power Break, then Freezing
// Strike, on a Warrior; never Aggression; Deflect Arrow against an archer.
func TestPvPRowPK(t *testing.T) {
	all := []int32{skillPoison, skillPowerBreak, skillFreezingStrike, skillDrainHealth, skillSting, skillPowerStrike, skillAggression, skillDeflectArrow}
	playRow(t, classPalusKnight, []rowCase{
		{name: "a Cleric: Drain Health opens", level: 36, weapon: "SWORD", skills: all, opp: lyrCleric, want: skillDrainHealth},
		{name: "a Cleric, opened: Poison", level: 36, weapon: "SWORD", skills: all, opp: lyrCleric, setup: opened, want: skillPoison},
		{name: "a Warrior: Power Break", level: 36, weapon: "SWORD", skills: all, opp: lyrWarrior, want: skillPowerBreak},
		{name: "a Warrior, opened, Power Break cooling: Freezing Strike", level: 36, weapon: "SWORD", skills: all, opp: lyrWarrior, setup: both(opened, cooling(skillPowerBreak)), want: skillFreezingStrike},
		{name: "a Human Mystic, opened: Poison, no Power Break", level: 36, weapon: "SWORD", skills: all, opp: lyrMystic, setup: opened, want: skillPoison},
		{
			name: "a Human Mystic, Poison cooling: no Freezing Strike, no Aggression", level: 36, weapon: "SWORD", skills: all, opp: lyrMystic,
			setup: both(opened, cooling(skillPoison, skillDrainHealth)), want: skillSting,
		},
		{name: "an archer: Deflect Arrow", level: 36, weapon: "SWORD", skills: all, opp: lyrWarrior, setup: func(_ *testing.T, b *Brain) { asArcher(b) }, want: skillDeflectArrow},
	})
}

// TestPvPRowAS checks the assassin's PvP row: with a dagger Drain Health
// opens, then Poison on a Cleric, Power Break on a Warrior; with a bow
// Stunning Shot held on a Cleric until its Heal (as RO).
func TestPvPRowAS(t *testing.T) {
	dagger := []int32{skillDrainHealth, skillPoison, skillPowerBreak, skillBleed, skillMortalBlow}
	bow := []int32{skillStunningShot, skillPowerShot}
	playRow(t, classAssassin, []rowCase{
		{name: "dagger, a Cleric: Drain Health opens", level: 36, weapon: "DAGGER", skills: dagger, opp: lyrCleric, want: skillDrainHealth},
		{name: "dagger, a Cleric, opened: Poison", level: 36, weapon: "DAGGER", skills: dagger, opp: lyrCleric, setup: opened, want: skillPoison},
		{name: "dagger, a Warrior: Power Break", level: 36, weapon: "DAGGER", skills: dagger, opp: lyrWarrior, want: skillPowerBreak},
		{name: "bow, a Cleric: Stunning Shot held", level: 36, weapon: "BOW", skills: bow, opp: lyrCleric, want: skillPowerShot},
		{name: "bow, the Cleric's Heal: Stunning Shot", level: 36, weapon: "BOW", skills: bow, opp: lyrCleric, heal: true, want: skillStunningShot},
	})
}

// TestPvPRowOR checks the overlord's PvP row: Rage, then Lionheart, at
// engage against a caster, neither against a Warrior; Stun Attack at
// reuse on a Warrior, held on a Cleric until its Heal.
func TestPvPRowOR(t *testing.T) {
	all := []int32{skillRage, skillLionheart, skillStunAttack, skillPowerSmash}
	playRow(t, classOrcRaider, []rowCase{
		{name: "a Human Mystic: Rage", level: 36, weapon: "BIGBLUNT", skills: all, opp: lyrMystic, want: skillRage},
		{name: "a Human Mystic, raging: Lionheart", level: 36, weapon: "BIGBLUNT", skills: all, opp: lyrMystic, setup: buffed(skillRage), want: skillLionheart},
		{name: "an Orc Mystic: Rage", level: 36, weapon: "BIGBLUNT", skills: all, opp: rowOrcMystic, want: skillRage},
		{name: "a Warrior: no Rage, Stun Attack", level: 36, weapon: "BIGBLUNT", skills: all, opp: lyrWarrior, want: skillStunAttack},
		{name: "a Cleric: Stun Attack held", level: 36, weapon: "BIGBLUNT", skills: all, opp: lyrCleric, setup: buffed(skillRage, skillLionheart), want: skillPowerSmash},
		{
			name: "the Cleric's Heal: Stun Attack", level: 36, weapon: "BIGBLUNT", skills: all, opp: lyrCleric, heal: true,
			setup: both(buffed(skillRage, skillLionheart), func(t *testing.T, b *Brain) { foeOf(t, b, lyrCleric, Point{Y: 60}, 0) }), want: skillStunAttack,
		},
	})
}

// TestPvPRowMO checks the monk's PvP row: Cripple on a caster, then Force
// Blaster opens with a charge; neither on a Warrior, Stunning Fist at
// reuse, Hurricane Assault with two charges; Stunning Fist held on a
// Cleric until its Heal.
func TestPvPRowMO(t *testing.T) {
	all := []int32{skillForceBlaster, skillCripple, skillStunningFist, skillHurricaneAssault, skillIronPunch}
	charges := func(n int32) func(*testing.T, *Brain) {
		return func(_ *testing.T, b *Brain) { b.world.Self.Charges = n }
	}
	playRow(t, classMonk, []rowCase{
		{name: "a Human Mystic: Cripple", level: 36, weapon: "DUALFIST", skills: all, opp: lyrMystic, setup: charges(1), want: skillCripple},
		{name: "a Human Mystic, Cripple cooling: Force Blaster opens", level: 36, weapon: "DUALFIST", skills: all, opp: lyrMystic, setup: both(charges(1), cooling(skillCripple)), want: skillForceBlaster},
		{name: "a Warrior: Stunning Fist at reuse", level: 36, weapon: "DUALFIST", skills: all, opp: lyrWarrior, setup: charges(1), want: skillStunningFist},
		{
			name: "a Warrior, opened, two charges: Hurricane Assault", level: 36, weapon: "DUALFIST", skills: all, opp: lyrWarrior,
			setup: both(opened, charges(2), cooling(skillStunningFist)), want: skillHurricaneAssault,
		},
		{name: "a Cleric: Stunning Fist held", level: 36, weapon: "DUALFIST", skills: all, opp: lyrCleric, setup: opened, want: skillIronPunch},
		{
			name: "the Cleric's Heal: Stunning Fist", level: 36, weapon: "DUALFIST", skills: all, opp: lyrCleric, heal: true,
			setup: both(opened, func(t *testing.T, b *Brain) { foeOf(t, b, lyrCleric, Point{Y: 60}, 0) }), want: skillStunningFist,
		},
	})
}

// TestPvPRowSC checks the scavenger's PvP row: no Spoil on a player; Stun
// Attack at reuse on a Warrior, held on a Cleric until its Heal.
func TestPvPRowSC(t *testing.T) {
	all := []int32{skillSpoil, skillStunAttack}
	playRow(t, classScavenger, []rowCase{
		{name: "a Warrior: no Spoil, Stun Attack", level: 20, weapon: "BLUNT", skills: all, opp: lyrWarrior, want: skillStunAttack},
		{name: "a Cleric: Stun Attack held, the attack", level: 20, weapon: "BLUNT", skills: all, opp: lyrCleric, kind: actAttack, not: []int32{skillSpoil}},
		{
			name: "the Cleric's Heal: Stun Attack", level: 20, weapon: "BLUNT", skills: all, opp: lyrCleric, heal: true,
			setup: func(t *testing.T, b *Brain) { foeOf(t, b, lyrCleric, Point{Y: 60}, 0) }, want: skillStunAttack,
		},
	})
}
