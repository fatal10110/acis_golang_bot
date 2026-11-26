package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The break-off fixtures (docs/pvp/layer.md "Tests"): test-built Break
// steps (a magic sleep of hit 4000, a fear of hit 1000), a DOT of the
// bot's own and Ultimate Defense.
const (
	brkSleep = int32(9320) // a magic sleep, hit 4000, landing at effect power 90
	brkFear  = int32(9321) // a magic fear, hit 1000, landing at effect power 90
	brkDot   = int32(9322) // a magic DOT ticking 10 times 3 s apart
)

// breakSkills are the datapack entries the break-off cases cast.
var breakSkills = map[datapack.SkillRef]datapack.Skill{
	{ID: brkSleep, Level: 1}:             {Type: "SLEEP", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, EffectPower: 90, Effects: []datapack.Effect{{Name: "Sleep", Time: 30, EffectPower: -1}}},
	{ID: brkFear, Level: 1}:              {Type: "FEAR", Target: "ONE", Magic: true, Offensive: true, HitTime: time.Second, EffectPower: 90, Effects: []datapack.Effect{{Name: "Fear", Time: 10, EffectPower: -1}}},
	{ID: brkDot, Level: 1}:               {Type: "MDOT", Target: "ONE", Magic: true, Offensive: true, HitTime: time.Second, Effects: dot("", "", 10)},
	{ID: skillUltimateDefense, Level: 1}: {Type: "BUFF", Target: "SELF", HitTime: time.Second, Effects: []datapack.Effect{{Name: "ImobileBuff", Time: 30}}},
}

// breakBrain is layerBrain against a Warrior 300 away with steps, a normal
// row (breaks off under 30 %) of margin, its CP gone, HP 250 of 1000, run
// speed 150 against the Warrior's 124, and the Warrior at 800 of 1000 HP:
// every break-off condition holds; on the test geodata, so the flee walks.
func breakBrain(t *testing.T, margin float64, steps ...Step) *Brain {
	t.Helper()
	b := layerBrain(t, lyrWarrior, steps...)
	b.cfg.Nav = testNav(t)
	b.walk = newWalker(b.cfg.Nav)
	b.world.SetGround(b.cfg.Nav.Engine)
	for ref, s := range breakSkills {
		b.cfg.Travel.skills[ref] = s
	}
	withKit(b, ClassKit{Rotation: steps, SpotMargin: margin})
	b.world.Inventory.Items = map[int32]Item{}
	s := &b.world.Self
	s.CP, s.MaxCP, s.HP, s.RunSpeed = 0, 500, 250, 150
	r := b.pvp.opponents[pcID]
	r.HP, r.MaxHP = 800, 1000
	return b
}

// TestPvPBreakOff checks the break-off (docs/pvp/layer.md "Break off"): in
// the open world with every condition met the ready Break step, then the
// flee; a duel, an arena, CP left, HP over the bound, winning, a potion
// ready or a faster opponent: it fights on; a sleep Break only on an
// opponent carrying no DOT, else the fear.
func TestPvPBreakOff(t *testing.T) {
	sleep, fear := Step{Skill: brkSleep, Break: true}, Step{Skill: brkFear, Break: true}
	root, ws := Step{Skill: grdRoot, Break: true}, Step{Skill: skillWindStrike}
	tests := []struct {
		name  string
		steps []Step
		setup func(t *testing.T, b *Brain)
		kind  actionKind
		skill int32
	}{
		{name: "open world, every condition: the Break step", steps: []Step{root, ws}, kind: actCast, skill: grdRoot},
		{name: "no Break step: the flee at once", steps: []Step{ws}, kind: actMove},
		{
			name: "a duel: fights on", steps: []Step{root, ws}, kind: actCast, skill: skillWindStrike,
			setup: func(t *testing.T, b *Brain) { duelUntil(t, b, DuelFighting, t0) },
		},
		{
			name: "an arena: fights on", steps: []Step{root, ws}, kind: actCast, skill: skillWindStrike,
			setup: func(_ *testing.T, b *Brain) {
				b.cfg.Travel.zones.Arenas = []datapack.Zone{box(-2000, -2000, 2000, 2000)}
			},
		},
		{
			name: "CP left: fights on", steps: []Step{root, ws}, kind: actCast, skill: skillWindStrike,
			setup: func(_ *testing.T, b *Brain) { b.world.Self.CP = 1 },
		},
		{
			name: "HP at the bound: fights on", steps: []Step{root, ws}, kind: actCast, skill: skillWindStrike,
			setup: func(_ *testing.T, b *Brain) { b.world.Self.HP = 300 },
		},
		{
			name: "a glass cannon at 35 %: breaks off", steps: []Step{root, ws}, kind: actCast, skill: grdRoot,
			setup: func(_ *testing.T, b *Brain) {
				b.world.Self.HP = 350
				withKit(b, ClassKit{Rotation: b.built.Kit.Rotation, SpotMargin: marginGlassCannon})
			},
		},
		{
			name: "winning: fights on", steps: []Step{root, ws}, kind: actCast, skill: skillWindStrike,
			setup: func(_ *testing.T, b *Brain) { b.pvp.opponents[pcID].HP = 100 },
		},
		{
			name: "the opponent's HP unknown: fights on", steps: []Step{root, ws}, kind: actCast, skill: skillWindStrike,
			setup: func(_ *testing.T, b *Brain) { b.pvp.opponents[pcID].MaxHP = 0 },
		},
		{
			name: "an HP potion ready: fights on", steps: []Step{root, ws}, kind: actCast, skill: skillWindStrike,
			setup: func(_ *testing.T, b *Brain) {
				b.world.Inventory.Items[1] = Item{ObjectID: 1, TemplateID: b.persona.Potions[0], Count: 3}
			},
		},
		{
			name: "the opponent faster: fights on", steps: []Step{root, ws}, kind: actCast, skill: skillWindStrike,
			setup: func(_ *testing.T, b *Brain) { b.world.Self.RunSpeed = 124 },
		},
		{
			name: "a sleep Break on an opponent free of DOTs: the sleep", steps: []Step{sleep, fear, ws}, kind: actCast, skill: brkSleep,
			setup: func(t *testing.T, b *Brain) { b.handleOne(t, foeAt(Point{Y: 900}, 1, 0, 0), at(time.Second)) },
		},
		{
			name: "a sleep Break on a poison mask bit: the fear", steps: []Step{sleep, fear, ws}, kind: actCast, skill: brkFear,
			setup: func(t *testing.T, b *Brain) { b.handleOne(t, foeAt(Point{Y: 900}, 1, 0, 0x2), at(time.Second)) },
		},
		{
			name: "a sleep Break 5 s after its own DOT landed: the fear", steps: []Step{sleep, fear, ws, {Skill: brkDot}}, kind: actCast, skill: brkFear,
			setup: func(t *testing.T, b *Brain) {
				b.handleOne(t, foeAt(Point{Y: 900}, 1, 0, 0), at(time.Second))
				b.handleOne(t, selfCast(brkDot, time.Second), at(4*time.Second))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := breakBrain(t, marginNormal, tc.steps...)
			if tc.setup != nil {
				tc.setup(t, b)
			}
			_, a := b.decide(at(10 * time.Second))
			if a.kind != tc.kind || tc.kind == actCast && a.skill != tc.skill {
				t.Fatalf("action %v skill %d, want %v skill %d", a.kind, a.skill, tc.kind, tc.skill)
			}
		})
	}
	t.Run("the Break step, then the flee", func(t *testing.T) {
		b := breakBrain(t, marginNormal, root, ws)
		play(t, b, []beat{
			{at: 10 * time.Second, kind: actCast, skill: grdRoot},
			{at: 10*time.Second + 100*time.Millisecond, ev: selfCast(grdRoot, time.Second), noDecide: true},
			{at: 11*time.Second + 200*time.Millisecond, ev: foeAt(Point{Y: 300}, 1, 0, rootAbnormal), kind: actMove},
		})
	})
}

// TestPvPBreakCastGate checks the break-cast gate (docs/pvp/layer.md): a
// Break step of hit time 4 s or more goes only on an opponent stunned,
// asleep or rooted, a melee one that cannot reach the bot before it lands,
// or one whose hits break it under breakOdds; else the bot flees at once.
func TestPvPBreakCastGate(t *testing.T) {
	sleep := Step{Skill: brkSleep, Break: true}
	tests := []struct {
		name string
		foe  CharInfo
		edit func(b *Brain)
		cast bool
	}{
		{name: "a melee opponent next to it: flees", foe: foeAt(Point{Y: 300}, 1, 0, 0)},
		{name: "the opponent stunned: the sleep", foe: foeAt(Point{Y: 300}, 1, 0, 0x40), cast: true},
		{name: "the opponent rooted: the sleep", foe: foeAt(Point{Y: 300}, 1, 0, rootAbnormal), cast: true},
		{name: "a melee opponent 900 away: the sleep", foe: foeAt(Point{Y: 900}, 1, 0, 0), cast: true},
		{
			name: "a fast cast: under 4 s the sleep goes", foe: foeAt(Point{Y: 300}, 1, 0, 0), cast: true,
			edit: func(b *Brain) { b.world.Self.CastSpd = 400 },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := breakBrain(t, marginNormal, sleep)
			b.handleOne(t, tc.foe, at(time.Second))
			if tc.edit != nil {
				tc.edit(b)
			}
			_, a := b.decide(at(10 * time.Second))
			if got := a.kind == actCast && a.skill == brkSleep; got != tc.cast {
				t.Fatalf("action %v skill %d, want the sleep %v", a.kind, a.skill, tc.cast)
			}
			if !tc.cast && a.kind != actMove {
				t.Fatalf("action %v, want the flee", a.kind)
			}
		})
	}
	t.Run("the odds decide a melee opponent in reach", func(t *testing.T) {
		b := breakBrain(t, marginNormal, sleep)
		bot, opp, ok := b.pvpFighters(pcID)
		if !ok {
			t.Fatal("no fighters")
		}
		s, _ := b.skillData(brkSleep)
		window := b.hitTime(brkSleep) - escLead
		want := charmodel.CastBreakOdds(opp, bot, window) < breakOdds
		if got := b.breakCastOK(pcID, s, brkSleep, bot, opp, true, at(10*time.Second)); got != want {
			t.Fatalf("gate %v, want %v (odds %.1f)", got, want, charmodel.CastBreakOdds(opp, bot, window))
		}
		bot.MEN = 99
		if !b.breakCastOK(pcID, s, brkSleep, bot, opp, true, at(10*time.Second)) {
			t.Fatal("a MEN no hit breaks through does not pass the gate")
		}
	})
}

// TestPvPUltimateDefense checks Ultimate Defense around the break-off
// (docs/pvp/layer.md "Ultimate Defense"): a tank row able to outrun its
// opponent flees and casts no Ultimate Defense; unable, or in a duel, it
// casts it; while it is on no flee, chase, kite or arena walk starts, but
// the guards' Esc and disengage move still go, and the arena leave walks
// out once it ends.
func TestPvPUltimateDefense(t *testing.T) {
	ud := Step{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20}
	ws := Step{Skill: skillWindStrike}
	udOn := AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillUltimateDefense, Level: 1, Left: 30 * time.Second}}}
	tests := []struct {
		name  string
		setup func(t *testing.T, b *Brain)
		kind  actionKind
		skill int32
	}{
		{name: "CP gone, HP under 20 %, losing, able to outrun: flees, no Ultimate Defense", kind: actMove},
		{
			name: "unable to outrun: Ultimate Defense", kind: actCast, skill: skillUltimateDefense,
			setup: func(_ *testing.T, b *Brain) { b.world.Self.RunSpeed = 100 },
		},
		{
			name: "in a duel: Ultimate Defense", kind: actCast, skill: skillUltimateDefense,
			setup: func(t *testing.T, b *Brain) { duelUntil(t, b, DuelFighting, t0) },
		},
		{
			name: "under it: no flee, the next cast in place", kind: actCast, skill: skillWindStrike,
			setup: func(t *testing.T, b *Brain) {
				// Its reuse holds Ultimate Defense itself.
				b.handleOne(t, udOn, at(5*time.Second))
				withKit(b, ClassKit{Rotation: []Step{ws}, SpotMargin: marginTank})
			},
		},
		{
			name: "under it, no cast left: no chase", kind: actHold,
			setup: func(t *testing.T, b *Brain) {
				b.handleOne(t, udOn, at(5*time.Second))
				withKit(b, ClassKit{SpotMargin: marginTank})
			},
		},
		{
			name: "no cast left and free to move: the chase", kind: actAttack,
			setup: func(t *testing.T, b *Brain) {
				b.world.Self.RunSpeed = 100
				withKit(b, ClassKit{SpotMargin: marginTank})
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := breakBrain(t, marginTank, ud, ws)
			b.world.Self.HP = 150
			if tc.setup != nil {
				tc.setup(t, b)
			}
			_, a := b.decide(at(10 * time.Second))
			if a.kind != tc.kind || tc.kind == actCast && a.skill != tc.skill {
				t.Fatalf("action %v skill %d, want %v skill %d", a.kind, a.skill, tc.kind, tc.skill)
			}
		})
	}
	t.Run("under it: flee sends nothing", func(t *testing.T) {
		b := breakBrain(t, marginTank, ud)
		b.handleOne(t, udOn, at(5*time.Second))
		if a := b.flee(pcID, at(10*time.Second)); a.kind != actHold {
			t.Fatalf("flee %v under Ultimate Defense, want nothing", a.kind)
		}
	})
	t.Run("under it: no kite walk", func(t *testing.T) {
		foe := Point{Y: 300}
		b := kiteBrain(t, false, foe, nil, nil)
		b.handleOne(t, udOn, at(9*time.Second))
		rootLands(t, b, foe)
		if _, a := b.decide(at(11*time.Second + 500*time.Millisecond)); a.kind == actMove {
			t.Fatal("a kite walk under Ultimate Defense")
		}
	})
	t.Run("under it: a karma-free opponent's blink still gets the Esc and the move, no damage step", func(t *testing.T) {
		b := guardBrain(t, false, 1, 0, ws)
		b.handleOne(t, udOn, t0)
		play(t, b, []beat{
			{at: 0, kind: actCast, skill: skillWindStrike},
			{at: 100 * time.Millisecond, ev: selfCast(skillWindStrike, 4*time.Second), noDecide: true},
			{at: time.Second, ev: flagTo(2), kind: actTargetCancel, rule: "guard"},
			{at: time.Second + 100*time.Millisecond, ev: MagicSkillCanceled{CasterID: selfID}, kind: actMove, rule: "guard"},
			{at: time.Second + 200*time.Millisecond, ev: ActionFailed{}, kind: actHold},
			{at: 3 * time.Second, kind: actHold},
		})
	})
	t.Run("under it: ArenaStay sends the stop move, then walks out once it ends", func(t *testing.T) {
		b := huntBrain(t)
		b.world.Self.ClassID = 1
		b.handleOne(t, DeleteObject{ObjectID: pcID}, t0)
		arenaTripAt(b, box(-500, -500, 500, 500))
		b.cfg.Nav = testNav(t)
		b.walk = newWalker(b.cfg.Nav)
		b.world.SetGround(b.cfg.Nav.Engine)
		b.handleOne(t, CharInfo{ObjectID: pvpThird, Name: "near", Pos: Point{Y: 100}, ClassID: 1, RunSpeed: 124}, t0)
		place(t, b, pvpThird, Point{Y: 100})
		b.world.Target = pvpThird
		stay := at(arenaStay + time.Second)
		b.handleOne(t, udOn, stay)
		rule, a := b.decide(stay)
		if rule != "arena leave" || a.kind != actMove {
			t.Fatalf("rule %q action %v, want the stop move", rule, a.kind)
		}
		if _, err := b.send(a, stay); err != nil {
			t.Fatal(err)
		}
		interval := swingTime(b.world.Self.PAtkSpd)
		if rule, a := b.decide(stay.Add(interval + 100*time.Millisecond)); rule != "arena leave" || a.kind != actHold {
			t.Fatalf("rule %q action %v under Ultimate Defense after the interval, want the hold", rule, a.kind)
		}
		end := stay.Add(interval + 200*time.Millisecond)
		b.handleOne(t, AbnormalStatusUpdate{}, end)
		if rule, a := b.decide(end); rule != "arena leave" || a.kind != actMove || moveTarget(a.payload).X <= 0 {
			t.Fatalf("rule %q action %v once it ended, want the walk out", rule, a.kind)
		}
	})
}
