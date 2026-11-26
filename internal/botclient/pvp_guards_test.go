package botclient

import (
	"encoding/binary"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The guards' fixtures (docs/pvp/guards.md): Poison (129, a pure DOT),
// Dryad Root (1201), test-built nextActionAttack steps (a PDAM, Shield Stun
// 92 and Bleed 96), a test-built 2000 ms magic damage step, War Cry (78, a
// self buff) and blessed spiritshots (3947).
const (
	grdPoison  = int32(129)
	grdRoot    = int32(1201)
	grdSmash   = int32(255)
	grdStun    = int32(92)
	grdBleed   = int32(96)
	grdNuke2s  = int32(9102)
	grdWarCry  = int32(78)
	grdBlessed = int32(3947)
	grdBSpSObj = int32(0x30000201)
	grdThird2  = int32(0x20000005)
)

// guardSkills are the datapack entries the guard tests cast: Wind Strike as
// shipped (MDAM, magic, hit 4000), the DOT, the root (30 s), the
// test-built steps and Wild Sweep (AREA PDAM, radius 150, no hit time).
var guardSkills = map[datapack.SkillRef]datapack.Skill{
	{ID: skillWindStrike, Level: 1}: {Type: "MDAM", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second},
	{ID: grdPoison, Level: 1}:       {Type: "POISON", Target: "ONE", Magic: true, Offensive: true, HitTime: time.Second},
	{ID: grdRoot, Level: 1}:         {Type: "ROOT", Target: "ONE", Magic: true, Offensive: true, HitTime: time.Second, Effects: []datapack.Effect{{Name: "Root", Time: 30}}},
	{ID: grdSmash, Level: 1}:        {Type: "PDAM", Target: "ONE", Offensive: true, NextActionAttack: true, HitTime: 1500 * time.Millisecond},
	{ID: grdStun, Level: 1}:         {Type: "STUN", Target: "ONE", Offensive: true, NextActionAttack: true, HitTime: 1500 * time.Millisecond},
	{ID: grdBleed, Level: 1}:        {Type: "BLEED", Target: "ONE", Offensive: true, NextActionAttack: true, HitTime: 1500 * time.Millisecond},
	{ID: grdNuke2s, Level: 1}:       {Type: "MDAM", Target: "ONE", Magic: true, Offensive: true, HitTime: 2 * time.Second},
	{ID: grdWarCry, Level: 1}:       {Type: "BUFF", Target: "SELF"},
	{ID: skillWildSweep, Level: 1}:  {Type: "PDAM", Target: "AREA", Offensive: true, Radius: 150},
}

// guardBrain is pvpBrain with the guard skills, a Human Mystic's speeds
// (cast and attack speed 333) and collision radius 9, the kit of steps and
// the opponent pcID 300 away (run speed 124, radius 9) with flag and karma,
// hitting the bot and selected. Its regular tick is pushed past the test,
// so only the test decides.
func guardBrain(t *testing.T, arena bool, flag uint8, karma int32, steps ...Step) *Brain {
	t.Helper()
	b := pvpBrain(t, arena, nil)
	for ref, s := range guardSkills {
		b.cfg.Travel.skills[ref] = s
	}
	b.cfg.Travel.shots = builtinShots
	s := &b.world.Self
	s.CastSpd, s.PAtkSpd, s.CollisionRadius = 333, 333, 9
	known := []KnownSkill{{ID: grdRoot, Level: 1}}
	for _, st := range steps {
		known = append(known, KnownSkill{ID: st.Skill, Level: 1})
	}
	b.world.Apply(SkillList{Skills: known}, t0)
	withKit(b, ClassKit{Rotation: steps})
	b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, RunSpeed: 124, Running: true, PvPFlag: int32(flag), Karma: karma, CollisionRadius: 9}, t0)
	b.handleOne(t, hitSelf(pcID, 10), t0)
	b.world.Self.InCombat = true
	b.world.Target = pcID
	b.nextTick = t0.Add(time.Hour)
	b.reactAt = time.Time{}
	return b
}

// beat is one step of a scripted fight: ev handled at at (none when nil),
// then, unless noDecide, the decision at at, sent, checked against kind
// (and skill for a cast, object when set, rule when set); wake, when set,
// is the latest the brain must have woken after ev.
type beat struct {
	at       time.Duration
	ev       Event
	noDecide bool
	kind     actionKind
	skill    int32
	rule     string
	wake     time.Duration
}

// play plays beats on b.
func play(t *testing.T, b *Brain, beats []beat) {
	t.Helper()
	for i, bt := range beats {
		now := at(bt.at)
		b.reactAt = time.Time{}
		if bt.ev != nil {
			b.handleOne(t, bt.ev, now)
		}
		if bt.wake > 0 && (b.reactAt.IsZero() || b.reactAt.After(at(bt.wake))) {
			t.Errorf("beat %d (%T at %v): woke at %v, want by %v", i, bt.ev, bt.at, b.reactAt.Sub(t0), bt.wake)
		}
		if bt.noDecide {
			continue
		}
		rule, a := b.decide(now)
		if a.kind != bt.kind || bt.kind == actCast && a.skill != bt.skill || bt.rule != "" && rule != bt.rule {
			t.Fatalf("beat %d at %v: rule %q kind %v skill %d, want kind %v skill %d rule %q", i, bt.at, rule, a.kind, a.skill, bt.kind, bt.skill, bt.rule)
		}
		if a.kind != actHold {
			if _, err := b.send(a, now); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// selfCast is the bot's own MagicSkillUse of skill at pcID lasting hit.
func selfCast(skill int32, hit time.Duration) MagicSkillUse {
	return MagicSkillUse{CasterID: selfID, TargetID: pcID, SkillID: skill, Level: 1, HitTime: hit}
}

// flagTo is pcID's RelationChanged to flag.
func flagTo(flag uint8) RelationChanged {
	return RelationChanged{ObjectID: pcID, PvPFlag: flag, AutoAttackable: flag != 0}
}

// moveDest is where a move action goes.
func moveDest(t *testing.T, a Action) Point {
	t.Helper()
	if a.kind != actMove || len(a.payload) < 13 {
		t.Fatalf("action %v is no move", a.kind)
	}
	p := a.payload[1:]
	return Point{X: int32(binary.LittleEndian.Uint32(p)), Y: int32(binary.LittleEndian.Uint32(p[4:])), Z: int32(binary.LittleEndian.Uint32(p[8:]))}
}

// TestPvPNoKarmaFlagEnding checks the flag guard (docs/pvp/guards.md "Flag
// guard"): a blink stops damage on a karma-free opponent with an Esc in the
// cast's window, then a move; DOTs still go; a karma opponent, a duel or an
// arena keep fighting; a late Esc re-selects once.
func TestPvPNoKarmaFlagEnding(t *testing.T) {
	ws, poison := Step{Skill: skillWindStrike}, Step{Skill: grdPoison}
	frame := time.Second / frameRate
	tests := []struct {
		name  string
		arena bool
		karma int32
		flag  uint8
		steps []Step
		setup func(t *testing.T, b *Brain)
		beats []beat
	}{
		{
			name: "a blink Esc-cancels Wind Strike in its window, then a move, then the DOT; flag 1 resumes",
			flag: 1, steps: []Step{ws, poison},
			beats: []beat{
				{at: 0, kind: actCast, skill: skillWindStrike},
				{at: 100 * time.Millisecond, ev: selfCast(skillWindStrike, 4*time.Second), noDecide: true},
				{at: time.Second, ev: flagTo(2), kind: actTargetCancel, rule: "guard", wake: time.Second + frame},
				{at: time.Second + 50*time.Millisecond, kind: actHold, rule: "guard"},
				{at: time.Second + 100*time.Millisecond, ev: MagicSkillCanceled{CasterID: selfID}, kind: actMove, rule: "guard", wake: time.Second + 100*time.Millisecond + frame},
				{at: time.Second + 300*time.Millisecond, kind: actCast, skill: grdPoison},
				{at: time.Second + 400*time.Millisecond, ev: selfCast(grdPoison, time.Second), noDecide: true},
				{at: 3 * time.Second, kind: actHold},
				{at: 5 * time.Second, ev: flagTo(1), kind: actCast, skill: skillWindStrike},
			},
		},
		{
			name: "the flag gone drops the swing with a move and attacks no more",
			flag: 1,
			beats: []beat{
				{at: 0, kind: actAttack},
				{at: time.Second, ev: flagTo(2), kind: actMove, rule: "guard"},
				{at: 2 * time.Second, kind: actHold},
				{at: 3 * time.Second, ev: flagTo(0), noDecide: true},
			},
		},
		{
			name: "a karma opponent's blink: the fight goes on",
			flag: 2, karma: 100, steps: []Step{ws, poison},
			beats: []beat{{at: 0, kind: actCast, skill: skillWindStrike}},
		},
		{
			name: "a blink in an arena: the fight goes on",
			flag: 2, arena: true, steps: []Step{ws, poison},
			beats: []beat{{at: 0, kind: actCast, skill: skillWindStrike}},
		},
		{
			name: "first seen blinking in its CharInfo: no damage step, the DOT",
			flag: 2, steps: []Step{ws, poison},
			beats: []beat{{at: 0, kind: actCast, skill: grdPoison}},
		},
		{
			name: "no nextActionAttack step after the blink, Shield Stun and Bleed included",
			flag: 2, steps: []Step{{Skill: grdSmash}, {Skill: grdStun}, {Skill: grdBleed}},
			beats: []beat{{at: 0, kind: actHold}},
		},
		{
			name: "a nextActionAttack step in the window: cancel, then on MagicSkillCanceled a move",
			flag: 1, steps: []Step{{Skill: grdStun}},
			beats: []beat{
				{at: 0, kind: actCast, skill: grdStun},
				{at: 100 * time.Millisecond, ev: selfCast(grdStun, 1500*time.Millisecond), noDecide: true},
				{at: 500 * time.Millisecond, ev: flagTo(2), kind: actTargetCancel},
				{at: 600 * time.Millisecond, ev: MagicSkillCanceled{CasterID: selfID}, kind: actMove},
			},
		},
		{
			name: "a nextActionAttack step past the window: no cancel, a move once its hit time passed",
			flag: 1, steps: []Step{{Skill: grdStun}},
			beats: []beat{
				{at: 0, kind: actCast, skill: grdStun},
				{at: 100 * time.Millisecond, ev: selfCast(grdStun, 1500*time.Millisecond), noDecide: true},
				{at: 1500 * time.Millisecond, ev: flagTo(2), kind: actHold, rule: "guard"},
				{at: 1600 * time.Millisecond, kind: actMove, rule: "guard"},
			},
		},
		{
			name: "a pure DOT in flight: cast, no cancel and no move",
			flag: 1, steps: []Step{poison},
			beats: []beat{
				{at: 0, kind: actCast, skill: grdPoison},
				{at: 100 * time.Millisecond, ev: selfCast(grdPoison, time.Second), noDecide: true},
				{at: 500 * time.Millisecond, ev: flagTo(2), kind: actHold, rule: "attacked"},
				{at: 1200 * time.Millisecond, kind: actHold, rule: "attacked"},
			},
		},
		{
			name: "a late Esc: the move, one selecting Action before the next DOT, never a second",
			flag: 1, steps: []Step{ws, poison},
			beats: []beat{
				{at: 0, kind: actCast, skill: skillWindStrike},
				{at: 100 * time.Millisecond, ev: selfCast(skillWindStrike, 4*time.Second), noDecide: true},
				{at: 3700 * time.Millisecond, ev: flagTo(2), kind: actTargetCancel},
				{at: 3750 * time.Millisecond, ev: TargetUnselected{ObjectID: selfID}, noDecide: true},
				{at: 4100 * time.Millisecond, kind: actMove, rule: "guard"},
				{at: 4300 * time.Millisecond, kind: actSelect},
				{at: 4500 * time.Millisecond, kind: actHold},
				{at: 4600 * time.Millisecond, ev: MyTargetSelected{ObjectID: pcID}, kind: actCast, skill: grdPoison},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := guardBrain(t, tc.arena, tc.flag, tc.karma, tc.steps...)
			play(t, b, tc.beats)
		})
	}
}

// TestPvPFlagGoneNoAttack checks that a karma-free opponent whose flag
// ended is fought no more: no cast, swing or selection.
func TestPvPFlagGoneNoAttack(t *testing.T) {
	b := guardBrain(t, false, 2, 0, Step{Skill: skillWindStrike})
	b.handleOne(t, flagTo(0), at(time.Second))
	if b.mayFight(pcID, at(time.Second)) {
		t.Fatal("may fight an opponent whose flag ended")
	}
	_, a := b.decide(at(time.Second))
	switch a.kind {
	case actCast, actAttack, actSelect:
		t.Fatalf("decision %v on an opponent whose flag ended", a.kind)
	}
}

// TestPvPHitTime checks the hit-time predictor (docs/pvp/layer.md
// "Interrupts"): the template time scaled by cast or attack speed,
// truncated, × 0.70 with a spiritshot expected charged, floored at 500 ms
// for a template time of 500 or more.
func TestPvPHitTime(t *testing.T) {
	tests := []struct {
		name           string
		skill          int32
		castSpd, atkSp int32
		shot           bool
		want           time.Duration
	}{
		{"magic at cast speed 333", skillWindStrike, 333, 300, false, 4000 * time.Millisecond},
		{"magic at cast speed 500, truncated", skillWindStrike, 500, 300, false, 2664 * time.Millisecond},
		{"magic with a blessed spiritshot charged", skillWindStrike, 333, 300, true, 2800 * time.Millisecond},
		{"physical at attack speed 666", grdSmash, 333, 666, false, 750 * time.Millisecond},
		{"physical takes no spiritshot", grdSmash, 333, 666, true, 750 * time.Millisecond},
		{"floored at 500", grdNuke2s, 2000, 300, true, 500 * time.Millisecond},
		{"unknown skill", 4242, 333, 333, false, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := guardBrain(t, false, 1, 0, Step{Skill: tc.skill})
			b.world.Self.CastSpd, b.world.Self.PAtkSpd = tc.castSpd, tc.atkSp
			if tc.shot {
				carryBlessed(b)
			}
			if got := b.hitTime(tc.skill); got != tc.want {
				t.Errorf("hit time %v, want %v", got, tc.want)
			}
		})
	}
}

// carryBlessed gives the bot a stack of blessed spiritshots with auto-use
// reported on (ExAutoSoulShot).
func carryBlessed(b *Brain) {
	b.persona.AutoShots = false
	b.world.Apply(ItemList{Items: []Item{{ObjectID: grdBSpSObj, TemplateID: grdBlessed, Count: 100}}}, t0)
	b.world.Apply(ExAutoSoulShot{TemplateID: grdBlessed, Enabled: true}, t0)
}

// foeAt is pcID's CharInfo at p with flag, karma and abnormal mask.
func foeAt(p Point, flag uint8, karma, abnormal int32) CharInfo {
	return CharInfo{ObjectID: pcID, Name: foeName, Pos: p, RunSpeed: 124, Running: true, PvPFlag: int32(flag), Karma: karma, Abnormal: abnormal, CollisionRadius: 9}
}

// TestPvPArenaAttackable checks the arena-edge rule and MayFight's rule 3
// (docs/pvp/guards.md "Arena edge", docs/pvp/targets.md "MayFight") in the
// test arena, a box 2000 to each side of the origin, the bot at its middle.
func TestPvPArenaAttackable(t *testing.T) {
	ws, nuke, poison := Step{Skill: skillWindStrike}, Step{Skill: grdNuke2s}, Step{Skill: grdPoison}
	edge := func(d int32) Point { return Point{Y: 2000 - d} }
	tests := []struct {
		name  string
		karma int32
		steps []Step
		setup func(t *testing.T, b *Brain)
		beats []beat
	}{
		{
			name: "both inside, a stale RelationChanged: a plain attack; the player walks out: a disengage",
			setup: func(t *testing.T, b *Brain) {
				b.handleOne(t, RelationChanged{ObjectID: pcID}, t0)
			},
			beats: []beat{
				{at: 0, kind: actAttack},
				{at: time.Second, ev: foeAt(Point{Y: 2100}, 0, 0, 0), kind: actMove, rule: "guard"},
			},
		},
		{
			name:  "100 from the edge at 120: no Wind Strike, the DOT",
			steps: []Step{ws, poison},
			setup: func(t *testing.T, b *Brain) {
				c := foeAt(edge(100), 0, 0, 0)
				c.RunSpeed = 120
				b.handleOne(t, c, t0)
			},
			beats: []beat{{at: 0, kind: actCast, skill: grdPoison}},
		},
		{
			name:  "shot-less at 490, speed 124: no Wind Strike (558), the 2000 ms step",
			steps: []Step{ws, nuke},
			setup: func(t *testing.T, b *Brain) { b.handleOne(t, foeAt(edge(490), 0, 0, 0), t0) },
			beats: []beat{{at: 0, kind: actCast, skill: grdNuke2s}},
		},
		{
			name:  "blessed spiritshot at 490: Wind Strike (409)",
			steps: []Step{ws, nuke},
			setup: func(t *testing.T, b *Brain) {
				carryBlessed(b)
				b.handleOne(t, foeAt(edge(490), 0, 0, 0), t0)
			},
			beats: []beat{{at: 0, kind: actCast, skill: skillWindStrike}},
		},
		{
			name:  "blessed spiritshot at 380: no Wind Strike",
			steps: []Step{ws, nuke},
			setup: func(t *testing.T, b *Brain) {
				carryBlessed(b)
				b.handleOne(t, foeAt(edge(380), 0, 0, 0), t0)
			},
			beats: []beat{{at: 0, kind: actCast, skill: grdNuke2s}},
		},
		{
			name:  "its MagicSkillUse carries 4000 ms with the opponent at 450: cancel at once",
			steps: []Step{ws},
			setup: func(t *testing.T, b *Brain) {
				carryBlessed(b)
				b.handleOne(t, foeAt(edge(450), 0, 0, 0), t0)
			},
			beats: []beat{
				{at: 0, kind: actCast, skill: skillWindStrike},
				{at: 100 * time.Millisecond, ev: selfCast(skillWindStrike, 4*time.Second), kind: actTargetCancel, rule: "guard", wake: 300 * time.Millisecond},
			},
		},
		{
			name:  "under its Dryad Root with 20 s left: Wind Strike; the root ends at 2.5 s to landing at 300: cancel",
			steps: []Step{ws},
			setup: func(t *testing.T, b *Brain) {
				b.handleOne(t, foeAt(edge(490), 0, 0, 0), t0)
				b.handleOne(t, selfCast(grdRoot, time.Second), t0)
				b.handleOne(t, foeAt(edge(490), 0, 0, rootAbnormal), at(1200*time.Millisecond))
				// Attack stands its attacker at its position: the
				// CharInfo puts it back.
				b.handleOne(t, hitSelf(pcID, 10), at(10*time.Second))
				b.handleOne(t, foeAt(edge(490), 0, 0, rootAbnormal), at(10*time.Second))
			},
			beats: []beat{
				{at: 11 * time.Second, kind: actCast, skill: skillWindStrike},
				{at: 11 * time.Second, ev: selfCast(skillWindStrike, 4*time.Second), noDecide: true},
				{at: 12 * time.Second, ev: StopMove{ObjectID: pcID, Pos: edge(300)}, kind: actHold, rule: "attacked"},
				{at: 12500 * time.Millisecond, ev: foeAt(edge(300), 0, 0, 0), kind: actTargetCancel, rule: "guard"},
			},
		},
		{
			name:  "the root ends with 0.4 s to landing at 300: no cancel",
			steps: []Step{ws},
			setup: func(t *testing.T, b *Brain) {
				b.handleOne(t, foeAt(edge(490), 0, 0, 0), t0)
				b.handleOne(t, selfCast(grdRoot, time.Second), t0)
				b.handleOne(t, foeAt(edge(490), 0, 0, rootAbnormal), at(1200*time.Millisecond))
				// Attack stands its attacker at its position: the
				// CharInfo puts it back.
				b.handleOne(t, hitSelf(pcID, 10), at(10*time.Second))
				b.handleOne(t, foeAt(edge(490), 0, 0, rootAbnormal), at(10*time.Second))
			},
			beats: []beat{
				{at: 11 * time.Second, kind: actCast, skill: skillWindStrike},
				{at: 11 * time.Second, ev: selfCast(skillWindStrike, 4*time.Second), noDecide: true},
				{at: 12 * time.Second, ev: StopMove{ObjectID: pcID, Pos: edge(300)}, noDecide: true},
				{at: 14600 * time.Millisecond, ev: foeAt(edge(300), 0, 0, 0), kind: actHold, rule: "attacked"},
			},
		},
		{
			name:  "the opponent nears the edge in the window: cancel, a move at least 150 inside; back by the margin: attacks",
			steps: []Step{ws},
			setup: func(t *testing.T, b *Brain) {
				b.world.Self.move = stopAt(Point{X: 1840})
				b.handleOne(t, foeAt(Point{X: 1000}, 0, 0, 0), t0)
			},
			beats: []beat{
				{at: 0, kind: actCast, skill: skillWindStrike},
				{at: 100 * time.Millisecond, ev: selfCast(skillWindStrike, 4*time.Second), noDecide: true},
				{at: time.Second, ev: StopMove{ObjectID: pcID, Pos: Point{X: 1000, Y: 1700}}, kind: actTargetCancel, rule: "guard", wake: 1200 * time.Millisecond},
				{at: 1100 * time.Millisecond, ev: MagicSkillCanceled{CasterID: selfID}, kind: actMove, rule: "guard"},
				{at: 5 * time.Second, ev: StopMove{ObjectID: pcID, Pos: Point{X: 1000}}, kind: actCast, skill: skillWindStrike},
			},
		},
		{
			name: "a karma opponent at the edge: the fight goes on", karma: 50,
			steps: []Step{ws},
			setup: func(t *testing.T, b *Brain) { b.handleOne(t, foeAt(edge(50), 0, 50, 0), t0) },
			beats: []beat{{at: 0, kind: actCast, skill: skillWindStrike}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := guardBrain(t, true, 0, tc.karma, tc.steps...)
			if tc.setup != nil {
				tc.setup(t, b)
			}
			b.reactAt = time.Time{}
			play(t, b, tc.beats)
		})
	}
}

// TestPvPArenaEdgeMove checks that the arena stop move ends at least
// arenaEdge inside, the bot standing 160 from the edge.
func TestPvPArenaEdgeMove(t *testing.T) {
	for seed := range 8 {
		b := guardBrain(t, true, 0, 0)
		b.world.Self.move = stopAt(Point{X: 1840})
		b.handleOne(t, foeAt(Point{X: 1600, Y: 1950}, 0, 0, 0), t0)
		b.pvp.guard.intent, b.engaged = pcID, pcID
		b.rng = rand.New(rand.NewPCG(uint64(seed), 1))
		rule, a := b.decide(at(time.Second))
		if rule != "guard" {
			t.Fatalf("seed %d: rule %q, want guard", seed, rule)
		}
		if e := b.arenaAt(moveDest(t, a)).Edge; e < arenaEdge {
			t.Errorf("seed %d: move ends %v from the edge, want at least %d", seed, e, arenaEdge)
		}
	}
}

// TestPvPArenaRule3 checks MayFight's rule 3 (docs/pvp/targets.md): in the
// bot's arena a clan mate and an ally are fought, a party member never; an
// unflagged player healing the bot is no attacker, one casting Wind Strike
// at it is.
func TestPvPArenaRule3(t *testing.T) {
	t.Run("clan mate", func(t *testing.T) {
		b := guardBrain(t, true, 0, 0)
		b.world.Clan.ID = 77
		b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, ClanID: 77}, at(time.Second))
		if k := b.fightKind(pcID, b.world.object(pcID), at(time.Second)); k != FightArena {
			t.Errorf("clan mate in the arena: kind %v, want arena", k)
		}
	})
	t.Run("ally", func(t *testing.T) {
		b := guardBrain(t, true, 0, 0)
		b.world.Self.AllyID = 5
		b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, AllyID: 5}, at(time.Second))
		if k := b.fightKind(pcID, b.world.object(pcID), at(time.Second)); k != FightArena {
			t.Errorf("ally in the arena: kind %v, want arena", k)
		}
	})
	t.Run("clan mate outside", func(t *testing.T) {
		b := guardBrain(t, false, 1, 0)
		b.world.Clan.ID = 77
		b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, ClanID: 77, PvPFlag: 1}, at(time.Second))
		if b.mayFight(pcID, at(time.Second)) {
			t.Error("a flagged clan mate outside an arena may be fought")
		}
	})
	t.Run("party member", func(t *testing.T) {
		b := guardBrain(t, true, 0, 0)
		b.world.Apply(PartySmallWindowAll{LeaderID: pcID, Members: []PartyMember{{ObjectID: pcID, HP: 100, MaxHP: 100}}}, t0)
		if b.mayFight(pcID, at(time.Second)) {
			t.Error("a party member in the arena may be fought")
		}
	})
	t.Run("healer and caster", func(t *testing.T) {
		b := pvpBrain(t, true, nil)
		b.cfg.Travel.skills[datapack.SkillRef{ID: skillWindStrike, Level: 1}] = guardSkills[datapack.SkillRef{ID: skillWindStrike, Level: 1}]
		b.handleOne(t, MagicSkillUse{CasterID: pcID, TargetID: selfID, SkillID: skillHeal, Level: 1, HitTime: time.Second}, at(time.Second))
		if b.world.attacking(pcID, at(time.Second)) {
			t.Fatal("a healer is in the attackers table")
		}
		b.handleOne(t, castAt(pcID, selfID, skillWindStrike), at(2*time.Second))
		if !b.world.attacking(pcID, at(2*time.Second)) {
			t.Fatal("a Wind Strike caster is not in the attackers table")
		}
	})
}
