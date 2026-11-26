package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The mystic PvP rows (docs/pvp/rows.md "Tests", TestPvPRow<ID>): each
// test compiles the real row of its class at a level, fights a player of
// an archetype and checks the row's opener, its Hold and Int steps and its
// Break step. The opponents: lyrWarrior (melee), rowRogue with a bow
// (archer), lyrMystic (caster), lyrCleric (healer), rowOrcMy (battle
// caster).

// rowNow is when the row cases decide; their setups play events before it.
var rowNow = at(5 * time.Second)

// pvpRowCase is one decision of a mystic PvP row: the bot of class at
// level against a player of class opp (with a bow when bow), after setup,
// casts skill, or moves (the flee or the kite) with move set.
type pvpRowCase struct {
	name         string
	class, level int32
	opp          int32
	bow          bool
	setup        func(t *testing.T, b *Brain)
	skill        int32
	move         bool
}

func runPvPRowCases(t *testing.T, cases []pvpRowCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := mysticRowBrain(t, tc.class, tc.level, tc.opp, tc.bow)
			if tc.setup != nil {
				tc.setup(t, b)
			}
			_, a := b.decide(rowNow)
			switch {
			case tc.move && a.kind != actMove:
				t.Fatalf("action %v skill %d, want a move", a.kind, a.skill)
			case !tc.move && (a.kind != actCast || a.skill != tc.skill):
				t.Fatalf("action %v skill %d, want a cast of %d", a.kind, a.skill, tc.skill)
			}
		})
	}
}

// landedOn plays the bot's casts of skills on pcID, each landing 1 s after
// its MagicSkillUse at 1 s.
func landedOn(skills ...int32) func(t *testing.T, b *Brain) {
	return func(t *testing.T, b *Brain) {
		for _, sk := range skills {
			b.handleOne(t, selfCast(sk, time.Second), at(time.Second))
		}
		b.opened = pcID
	}
}

// foeSeen has pcID, a player of class, seen at p showing abnormal at 4 s.
func foeSeen(class int32, p Point, abnormal int32) func(t *testing.T, b *Brain) {
	return func(t *testing.T, b *Brain) { b.handleOne(t, rowFoe(class, false, p, abnormal), at(4*time.Second)) }
}

// healCast has pcID (a Cleric) start 1011 Heal on itself where it stands
// at rowNow, landing after hit (5 s without a spiritshot, 3.5 s with one).
func healCast(hit time.Duration) func(t *testing.T, b *Brain) {
	return func(t *testing.T, b *Brain) {
		p := b.world.object(pcID).Position(rowNow)
		b.handleOne(t, MagicSkillUse{CasterID: pcID, TargetID: pcID, SkillID: skillHeal, Level: 1, HitTime: hit, CasterPos: p, TargetPos: p}, rowNow)
	}
}

// matk sets the bot's M. Atk: a damage Int passes MinBreak only on a
// strong enough hit.
func matk(v int32) func(t *testing.T, b *Brain) {
	return func(_ *testing.T, b *Brain) { b.world.Self.MAtk = v }
}

// breaking meets every break-off condition but the row's: CP gone, HP 200
// of 1000 (under every row's bound), no potion, running 150 against the
// opponent's 124, which stands at 800 of 1000 HP; on the test geodata, so
// the flee walks.
func breaking(t *testing.T, b *Brain) {
	b.cfg.Nav = testNav(t)
	b.walk = newWalker(b.cfg.Nav)
	b.world.SetGround(b.cfg.Nav.Engine)
	b.world.Inventory.Items = map[int32]Item{}
	s := &b.world.Self
	s.CP, s.MaxCP, s.HP, s.RunSpeed = 0, 500, 200, 150
	r := b.pvp.opponents[pcID]
	r.HP, r.MaxHP = 800, 1000
}

// reusing keeps skills on reuse past rowNow.
func reusing(skills ...int32) func(t *testing.T, b *Brain) {
	return func(_ *testing.T, b *Brain) { hold(b, rowNow, skills...) }
}

// wield puts a weapon of datapack kind in the bot's right hand.
func wield(kind string) func(t *testing.T, b *Brain) {
	return func(_ *testing.T, b *Brain) {
		b.cfg.Travel.gear = map[int32]gearItem{rowWeaponID: {part: partRHand, kind: kind, tmpl: datapack.Item{Kind: kind}}}
		b.world.Apply(ItemList{Items: []Item{{ObjectID: rowWeapon, TemplateID: rowWeaponID, Count: 1, Equipped: true, BodySlot: partRHand}}}, t0)
	}
}

// TestPvPRowHM checks the human mystic: Ice Bolt opens on melee and
// archers, Wind Strike on the others; from 14 Curse: Weakness goes first
// on melee (control before the opener); Curse: Poison on a healer, not
// again while it ticks. No Int, Hold or Break step.
func TestPvPRowHM(t *testing.T) {
	runPvPRowCases(t, []pvpRowCase{
		{name: "@10 vs melee: Ice Bolt", class: classHumanMystic, level: 10, opp: lyrWarrior, skill: skillIceBolt},
		{name: "@10 vs an archer: Ice Bolt", class: classHumanMystic, level: 10, opp: rowRogue, bow: true, skill: skillIceBolt},
		{name: "@15 vs melee: Curse: Weakness first", class: classHumanMystic, level: 15, opp: lyrWarrior, skill: skillCurseWeakness},
		{name: "@15 vs melee, weakened: Ice Bolt (kite)", class: classHumanMystic, level: 15, opp: lyrWarrior, setup: landedOn(skillCurseWeakness), skill: skillIceBolt},
		{name: "@15 vs a caster: Wind Strike", class: classHumanMystic, level: 15, opp: lyrMystic, skill: skillWindStrike},
		{name: "@15 vs a healer, opened: Curse: Poison", class: classHumanMystic, level: 15, opp: lyrCleric, setup: opened, skill: skillCursePoison},
		{name: "@15 vs a healer, poisoned: Wind Strike", class: classHumanMystic, level: 15, opp: lyrCleric, setup: landedOn(skillCursePoison), skill: skillWindStrike},
		{name: "@15 vs a caster, opened: no poison, no Ice Bolt", class: classHumanMystic, level: 15, opp: lyrMystic, setup: opened, skill: skillWindStrike},
	})
}

// TestPvPRowEM checks the elven mystic: Wind Shackle from 14 opens on
// melee and archers, Ice Bolt on a caster (the M10 opener's Wind Shackle
// is replaced, so no shackle on a caster); Curse: Weakness on a shackled
// melee opponent.
func TestPvPRowEM(t *testing.T) {
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs melee: Wind Shackle", class: classElvenMystic, level: 15, opp: lyrWarrior, skill: skillWindShackle},
		{name: "vs an archer: Wind Shackle", class: classElvenMystic, level: 15, opp: rowRogue, bow: true, skill: skillWindShackle},
		{name: "vs a caster: Ice Bolt", class: classElvenMystic, level: 15, opp: lyrMystic, skill: skillIceBolt},
		{name: "vs melee, shackled: Curse: Weakness", class: classElvenMystic, level: 15, opp: lyrWarrior, setup: landedOn(skillWindShackle), skill: skillCurseWeakness},
		{name: "vs an archer, opened: Wind Strike", class: classElvenMystic, level: 15, opp: rowRogue, bow: true, setup: opened, skill: skillWindStrike},
	})
}

// TestPvPRowDM checks the dark mystic: Wind Shackle from 14 on melee,
// Curse: Poison on a caster or healer; no poison on melee before 14 (Ice
// Bolt) or on a battle caster (Wind Strike): the M10 poison opener is
// replaced.
func TestPvPRowDM(t *testing.T) {
	runPvPRowCases(t, []pvpRowCase{
		{name: "@15 vs melee: Wind Shackle", class: classDarkMystic, level: 15, opp: lyrWarrior, skill: skillWindShackle},
		{name: "@15 vs a caster: Curse: Poison", class: classDarkMystic, level: 15, opp: lyrMystic, skill: skillCursePoison},
		{name: "@15 vs a healer: Curse: Poison", class: classDarkMystic, level: 15, opp: lyrCleric, skill: skillCursePoison},
		{name: "@10 vs melee: Ice Bolt, no poison", class: classDarkMystic, level: 10, opp: lyrWarrior, skill: skillIceBolt},
		{name: "@15 vs a battle caster: Wind Strike, no poison", class: classDarkMystic, level: 15, opp: rowOrcMy, skill: skillWindStrike},
	})
}

// TestPvPRowOM checks the orc mystic: Chill Flame opens; Venom on a caster
// or healer, Chill Flame, Life Drain, each DOT not again while it ticks;
// Dreaming Spirit answers a Cleric's Heal cast without a spiritshot (5 s),
// not one with (3.5 s); Break: Dreaming Spirit on an opponent free of
// DOTs that cannot reach it first.
func TestPvPRowOM(t *testing.T) {
	far := foeSeen(lyrWarrior, Point{Y: 900}, 0)
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs melee: Chill Flame", class: classOrcMystic, level: 15, opp: lyrWarrior, skill: skillChillFlame},
		{name: "vs a caster, opened: Venom", class: classOrcMystic, level: 15, opp: lyrMystic, setup: opened, skill: skillVenom},
		{name: "vs a caster, Venom ticking: Chill Flame", class: classOrcMystic, level: 15, opp: lyrMystic, setup: landedOn(skillVenom), skill: skillChillFlame},
		{name: "vs a caster, both ticking: Life Drain", class: classOrcMystic, level: 15, opp: lyrMystic, setup: landedOn(skillVenom, skillChillFlame), skill: skillLifeDrain},
		{name: "vs a Cleric's Heal (5 s): Dreaming Spirit", class: classOrcMystic, level: 15, opp: lyrCleric, setup: both(opened, healCast(5*time.Second)), skill: skillDreamingSpirit},
		{name: "vs a Cleric's Heal with a spiritshot (3.5 s): no Int", class: classOrcMystic, level: 15, opp: lyrCleric, setup: both(opened, healCast(3500*time.Millisecond)), skill: skillVenom},
		{name: "breaking off, no DOT, out of reach: Dreaming Spirit", class: classOrcMystic, level: 15, opp: lyrWarrior, setup: both(breaking, far, reusing(skillLifeDrain)), skill: skillDreamingSpirit},
	})
}

// TestPvPRowCL checks the cleric: Dryad Root from 25 opens on melee;
// Mental Shield on itself against a caster, then Wind Strike; Break:
// Dryad Root (hit 2500, under the break-cast gate's 4 s).
func TestPvPRowCL(t *testing.T) {
	shielded := func(t *testing.T, b *Brain) {
		b.handleOne(t, AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillMentalShield, Level: 1, Left: 1200 * time.Second}}}, at(time.Second))
	}
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs melee: Dryad Root", class: classCleric, level: 30, opp: lyrWarrior, skill: skillDryadRoot},
		{name: "@22 vs melee: Ice Bolt", class: classCleric, level: 22, opp: lyrWarrior, skill: skillIceBolt},
		{name: "vs a caster: Mental Shield", class: classCleric, level: 30, opp: lyrMystic, skill: skillMentalShield},
		{name: "vs a caster, shielded: Wind Strike", class: classCleric, level: 30, opp: lyrMystic, setup: shielded, skill: skillWindStrike},
		{name: "vs a healer: no Mental Shield, Wind Strike", class: classCleric, level: 30, opp: lyrCleric, skill: skillWindStrike},
		{name: "breaking off: Dryad Root", class: classCleric, level: 30, opp: lyrWarrior, setup: breaking, skill: skillDryadRoot},
		{name: "breaking off, rooted: the flee", class: classCleric, level: 30, opp: lyrWarrior, setup: both(breaking, landedOn(skillDryadRoot), foeSeen(lyrWarrior, Point{Y: 300}, rootAbnormal)), move: true},
	})
}

// TestPvPRowHW checks the human wizard: Ice Bolt opens on melee, Blaze on
// a caster; from 35 Slow goes first on melee and Curse Chaos on an archer;
// Energy Bolt answers a Cleric's Heal when its hit passes MinBreak, not
// under it; Break: Ice Bolt, the flee while its slow or Slow's holds.
func TestPvPRowHW(t *testing.T) {
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs melee: Ice Bolt", class: classHumanWizard, level: 25, opp: lyrWarrior, skill: skillIceBolt},
		{name: "vs a caster: Blaze", class: classHumanWizard, level: 25, opp: lyrMystic, skill: skillBlaze},
		{name: "@35 vs melee: Slow first", class: classHumanWizard, level: 35, opp: lyrWarrior, skill: skillSlow},
		{name: "@35 vs an archer: Curse Chaos first", class: classHumanWizard, level: 35, opp: rowRogue, bow: true, skill: skillCurseChaos},
		{name: "vs a Cleric's Heal, a strong hit: Energy Bolt", class: classHumanWizard, level: 25, opp: lyrCleric, setup: both(opened, matk(5000), healCast(5*time.Second)), skill: skillEnergyBolt},
		{name: "vs a Cleric's Heal, a weak hit: no Int", class: classHumanWizard, level: 25, opp: lyrCleric, setup: both(opened, healCast(5*time.Second)), skill: skillBlaze},
		{name: "breaking off: Ice Bolt", class: classHumanWizard, level: 25, opp: lyrWarrior, setup: both(breaking, reusing(skillVampiricTouch)), skill: skillIceBolt},
		{name: "breaking off, still slowed: the flee", class: classHumanWizard, level: 25, opp: lyrWarrior, setup: both(breaking, reusing(skillVampiricTouch), landedOn(skillIceBolt)), move: true},
		{name: "@35 breaking off, slowed by Slow: the flee", class: classHumanWizard, level: 35, opp: lyrWarrior, setup: both(breaking, reusing(skillVampiricTouch), landedOn(skillSlow)), move: true},
	})
}

// TestPvPRowEO checks the elven oracle: Wind Shackle, then Dryad Root on
// melee; Mental Shield against a battle caster; Break: Dryad Root.
func TestPvPRowEO(t *testing.T) {
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs melee: Wind Shackle", class: classElvenOracle, level: 30, opp: lyrWarrior, skill: skillWindShackle},
		{name: "vs melee, shackled: Dryad Root", class: classElvenOracle, level: 30, opp: lyrWarrior, setup: landedOn(skillWindShackle), skill: skillDryadRoot},
		{name: "vs a battle caster: Mental Shield", class: classElvenOracle, level: 30, opp: rowOrcMy, skill: skillMentalShield},
		{name: "breaking off: Dryad Root", class: classElvenOracle, level: 30, opp: lyrWarrior, setup: breaking, skill: skillDryadRoot},
	})
}

// TestPvPRowEW checks the elven wizard: Wind Shackle on melee; Solar
// Spark opens on a caster from 25, Aqua Swirl before; Energy Bolt answers
// a Cleric's Heal on a strong hit; Break: Ice Bolt.
func TestPvPRowEW(t *testing.T) {
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs melee: Wind Shackle", class: classElvenWizard, level: 30, opp: lyrWarrior, skill: skillWindShackle},
		{name: "vs a caster: Solar Spark", class: classElvenWizard, level: 30, opp: lyrMystic, skill: skillSolarSpark},
		{name: "@22 vs a caster: Aqua Swirl", class: classElvenWizard, level: 22, opp: lyrMystic, skill: skillAquaSwirl},
		{name: "vs a Cleric's Heal, a strong hit: Energy Bolt", class: classElvenWizard, level: 30, opp: lyrCleric, setup: both(opened, matk(5000), healCast(5*time.Second)), skill: skillEnergyBolt},
		{name: "breaking off: Ice Bolt", class: classElvenWizard, level: 30, opp: lyrWarrior, setup: both(breaking, reusing(skillSelfHeal)), skill: skillIceBolt},
	})
}

// TestPvPRowSO checks the shillien oracle: Wind Shackle on melee, Curse:
// Poison on a caster; Vampiric Touch under 60 % HP; Break: Dryad Root.
func TestPvPRowSO(t *testing.T) {
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs melee: Wind Shackle", class: classShillienOracle, level: 30, opp: lyrWarrior, skill: skillWindShackle},
		{name: "vs a caster, shielded: Curse: Poison", class: classShillienOracle, level: 30, opp: lyrMystic, setup: reusing(skillMentalShield), skill: skillCursePoison},
		{name: "under 60 % HP: Vampiric Touch", class: classShillienOracle, level: 30, opp: lyrWarrior, setup: func(_ *testing.T, b *Brain) { b.world.Self.HP = 500 }, skill: skillVampiricTouch},
		{name: "breaking off: Dryad Root", class: classShillienOracle, level: 30, opp: lyrWarrior, setup: both(breaking, reusing(skillVampiricTouch)), skill: skillDryadRoot},
	})
}

// TestPvPRowDW checks the dark wizard: Curse: Poison opens on a caster,
// Shadow Spark on a battle caster; from 36 Wind Shackle, then Slow on
// melee; Aura Burn answers a Cleric's Heal within 150 on a strong hit;
// Break: Ice Bolt.
func TestPvPRowDW(t *testing.T) {
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs a caster: Curse: Poison", class: classDarkWizard, level: 30, opp: lyrMystic, skill: skillCursePoison},
		{name: "vs a battle caster: Shadow Spark", class: classDarkWizard, level: 30, opp: rowOrcMy, skill: skillShadowSpark},
		{name: "@36 vs melee: Wind Shackle", class: classDarkWizard, level: 36, opp: lyrWarrior, skill: skillWindShackle},
		{name: "@36 vs melee, shackled: Slow", class: classDarkWizard, level: 36, opp: lyrWarrior, setup: landedOn(skillWindShackle), skill: skillSlow},
		{name: "vs a Cleric's Heal within 150, a strong hit: Aura Burn", class: classDarkWizard, level: 30, opp: lyrCleric, setup: both(opened, foeSeen(lyrCleric, Point{Y: 50}, 0), matk(5000), healCast(5*time.Second)), skill: skillAuraBurn},
		{name: "vs a Cleric's Heal at 300, a strong hit: no Aura Burn", class: classDarkWizard, level: 30, opp: lyrCleric, setup: both(opened, matk(5000), healCast(5*time.Second)), skill: skillShadowSpark},
		{name: "breaking off: Ice Bolt", class: classDarkWizard, level: 30, opp: lyrWarrior, setup: both(breaking, reusing(skillVampiricTouch)), skill: skillIceBolt},
	})
}

// TestPvPRowOS checks the orc shaman: Frost Flame opens on melee out of
// Stun Attack's reach; Stun Attack with a blunt on melee in reach, held
// against a Cleric until its Heal; Chant of Fire on itself against a
// caster, then Aura Sink; Break: Seal of Binding on a stunned melee
// opponent next to it (hit 4000, only under the break-cast gate; a strong
// M. Atk lifts its base 40 over MinLand), the flee when the opponent
// would break it.
func TestPvPRowOS(t *testing.T) {
	near := foeSeen(lyrWarrior, Point{Y: 50}, 0)
	chanted := func(t *testing.T, b *Brain) {
		b.handleOne(t, AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillChantFire, Level: 1, Left: 1200 * time.Second}}}, at(time.Second))
	}
	runPvPRowCases(t, []pvpRowCase{
		{name: "vs melee at 300: Frost Flame", class: classOrcShaman, level: 30, opp: lyrWarrior, setup: wield("BLUNT"), skill: skillFrostFlame},
		{name: "vs melee in reach: Stun Attack", class: classOrcShaman, level: 30, opp: lyrWarrior, setup: both(wield("BLUNT"), near), skill: skillStunAttack},
		{name: "vs a Cleric in reach: no Stun Attack at reuse", class: classOrcShaman, level: 30, opp: lyrCleric, setup: both(wield("BLUNT"), opened, foeSeen(lyrCleric, Point{Y: 50}, 0), reusing(skillAuraSink, skillFrostFlame)), skill: skillLifeDrain},
		{name: "vs a Cleric's Heal in reach: Stun Attack", class: classOrcShaman, level: 30, opp: lyrCleric, setup: both(wield("BLUNT"), opened, foeSeen(lyrCleric, Point{Y: 50}, 0), healCast(5*time.Second)), skill: skillStunAttack},
		{name: "vs a caster: Chant of Fire", class: classOrcShaman, level: 30, opp: lyrMystic, skill: skillChantFire},
		{name: "vs a caster, chanted, opened: Aura Sink", class: classOrcShaman, level: 30, opp: lyrMystic, setup: both(chanted, opened), skill: skillAuraSink},
		{name: "breaking off, a stunned melee opponent next to it: Seal of Binding", class: classOrcShaman, level: 30, opp: lyrWarrior, setup: both(breaking, foeSeen(lyrWarrior, Point{Y: 50}, 0x40), reusing(skillLifeDrain), matk(2000)), skill: skillSealBinding},
		{name: "breaking off, a melee opponent next to it: the flee", class: classOrcShaman, level: 30, opp: lyrWarrior, setup: both(breaking, foeSeen(lyrWarrior, Point{Y: 50}, 0), reusing(skillLifeDrain)), move: true},
	})
}
