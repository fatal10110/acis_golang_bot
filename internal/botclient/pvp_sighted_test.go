package botclient

import (
	"testing"
	"time"
)

// calmRowBrain is pvpRowBrain out of combat: the knight row's bot of class
// at level with a sword and a shield, knowing 112 Deflect Arrow, with
// nobody hitting it and nothing selected; the opponent pcID stays in sight
// for the cases to place.
func calmRowBrain(t *testing.T, class, level int32) *Brain {
	t.Helper()
	b := pvpRowBrain(t, class, level, "SWORD", lyrWarrior, skillDeflectArrow)
	wearShield(b)
	b.world.Attackers = map[int32]*Attacker{}
	b.world.Self.InCombat = false
	b.world.Target = 0
	return b
}

// sightedArcher shows pcID at p with PvP flag flag as an archer.
func sightedArcher(t *testing.T, b *Brain, p Point, flag int32) {
	t.Helper()
	b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, ClassID: lyrWarrior, Pos: p, RunSpeed: 124, PvPFlag: flag, CollisionRadius: 9}, t0)
	b.world.object(pcID).move = stopAt(p)
	asArcher(b)
}

// TestPvPSightedDeflectArrow checks the knights' out-of-combat 112 Deflect
// Arrow (#557, docs/pvp/rows.md KN, EK, PK): put up by "rebuff" once a
// flagged archer is within sightedReach, never for a clean archer, a
// flagged Warrior, an archer farther off, before 24 or with the buff on.
func TestPvPSightedDeflectArrow(t *testing.T) {
	for _, tc := range []struct {
		name  string
		class int32
		level int32
		setup func(*testing.T, *Brain)
		want  bool
	}{
		{name: "KN: a flagged archer at 800", class: classKnight, level: 28, setup: func(t *testing.T, b *Brain) { sightedArcher(t, b, Point{Y: 800}, 1) }, want: true},
		{name: "EK: a flagged archer at 800", class: classElvenKnight, level: 28, setup: func(t *testing.T, b *Brain) { sightedArcher(t, b, Point{Y: 800}, 1) }, want: true},
		{name: "PK: a flagged archer at 800", class: classPalusKnight, level: 28, setup: func(t *testing.T, b *Brain) { sightedArcher(t, b, Point{Y: 800}, 1) }, want: true},
		{name: "KN: a clean archer", class: classKnight, level: 28, setup: func(t *testing.T, b *Brain) { sightedArcher(t, b, Point{Y: 800}, 0) }},
		{name: "KN: a flagged Warrior", class: classKnight, level: 28, setup: func(t *testing.T, b *Brain) { foeOf(t, b, lyrWarrior, Point{Y: 800}, 0) }},
		{name: "KN: no archer", class: classKnight, level: 28, setup: func(t *testing.T, b *Brain) { b.handleOne(t, DeleteObject{ObjectID: pcID}, t0) }},
		{name: "KN: a flagged archer at 1500", class: classKnight, level: 28, setup: func(t *testing.T, b *Brain) { sightedArcher(t, b, Point{Y: 1500}, 1) }},
		{name: "KN before 24", class: classKnight, level: 23, setup: func(t *testing.T, b *Brain) { sightedArcher(t, b, Point{Y: 800}, 1) }},
		{
			name: "KN with Deflect Arrow on", class: classKnight, level: 28,
			setup: both(func(t *testing.T, b *Brain) { sightedArcher(t, b, Point{Y: 800}, 1) }, buffed(skillDeflectArrow)),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := calmRowBrain(t, tc.class, tc.level)
			tc.setup(t, b)
			rule, a := b.decide(at(time.Second))
			got := a.kind == actCast && a.skill == skillDeflectArrow
			if got != tc.want || got && rule != "rebuff" {
				t.Fatalf("rule %q action %v skill %d, want Deflect Arrow %v", rule, a.kind, a.skill, tc.want)
			}
		})
	}
}

// TestPvPSightedDeflectArrowOnce checks one cast per buff: after the cast
// goes out, none until its effect lands (buffRetry), and none while it
// holds, the flagged archer still in sight.
func TestPvPSightedDeflectArrowOnce(t *testing.T) {
	b := calmRowBrain(t, classKnight, 28)
	sightedArcher(t, b, Point{Y: 800}, 1)
	now := at(time.Second)
	_, a := b.decide(now)
	if a.kind != actCast || a.skill != skillDeflectArrow {
		t.Fatalf("action %v skill %d, want Deflect Arrow", a.kind, a.skill)
	}
	if _, err := b.send(a, now); err != nil {
		t.Fatal(err)
	}
	for _, d := range []time.Duration{5 * time.Second, 30 * time.Second} {
		if _, a := b.decide(now.Add(d)); a.kind == actCast && a.skill == skillDeflectArrow {
			t.Fatalf("at +%v: Deflect Arrow again", d)
		}
	}
	buffed(skillDeflectArrow)(t, b)
	if _, a := b.decide(now.Add(2 * time.Minute)); a.kind == actCast && a.skill == skillDeflectArrow {
		t.Fatal("Deflect Arrow on: cast again")
	}
}
