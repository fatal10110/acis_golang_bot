package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// box is a cuboid zone from (x0, y0) to (x1, y1).
func box(x0, y0, x1, y1 int32) datapack.Zone {
	return datapack.Zone{Cuboid: true, X: []int32{x0, x1}, Y: []int32{y0, y1}, MinZ: -1000, MaxZ: 1000}
}

// duelGroundBrain is guardBrain fighting its duel with pcID at foe, with
// the zones set by zones and the kit of steps; the fight starts at 7 s.
func duelGroundBrain(t *testing.T, foe Point, zones func(z *datapack.Zones), steps ...Step) *Brain {
	t.Helper()
	b := guardBrain(t, false, 0, 0, steps...)
	zones(&b.cfg.Travel.zones)
	duelUntil(t, b, DuelFighting, t0)
	place(t, b, pcID, foe)
	b.reactAt = time.Time{}
	return b
}

// TestPvPDuelFightGround checks duel ground (docs/pvp/finding.md "Duel
// ground", the X4b cases of TestPvPDuelFight): the stand points and the
// disengage moves keep 100 off peace, town, arena and foreign siege ground
// and within 1800 of the opponent, and the bot stops the server's follow
// onto it.
func TestPvPDuelFightGround(t *testing.T) {
	fight := at(7 * time.Second)
	t.Run("a stand point within 100 of a town is skipped, the first 100 outside taken", func(t *testing.T) {
		b := duelGroundBrain(t, Point{X: 60}, func(z *datapack.Zones) { z.Towns = []datapack.Zone{box(-500, -500, 500, -120)} })
		wieldPole(b)
		player(t, b, pvpThird, "x", Point{X: 50, Y: 75}, 0, 0, 9)
		_, a := b.decide(fight)
		if got, want := moveDest(t, a), (Point{X: 8, Y: 30}); got.X != want.X || got.Y != want.Y {
			t.Fatalf("stand point %v, want %v", got, want)
		}
	})
	t.Run("no stand point on duel ground: the hold", func(t *testing.T) {
		b := duelGroundBrain(t, Point{X: 60}, func(z *datapack.Zones) {
			z.Towns = []datapack.Zone{box(-500, -500, 500, -120)}
			z.Arenas = []datapack.Zone{box(-500, 120, 500, 500)}
			z.Sieges = []datapack.Zone{box(100, -100, 500, 100)}
		})
		wieldPole(b)
		player(t, b, pvpThird, "x", Point{X: 50, Y: 75}, 0, 0, 9)
		if _, a := b.decide(fight); a.kind != actHold {
			t.Fatalf("decision %v, want the hold", a.kind)
		}
	})
	t.Run("the bot within 100 of a peace zone while attacking: the disengage move leads out", func(t *testing.T) {
		b := duelGroundBrain(t, Point{Y: 300}, func(z *datapack.Zones) { z.Peace = []datapack.Zone{box(50, -500, 500, 500)} })
		b.engaged, b.pvp.guard.intent = pcID, pcID
		rule, a := b.decide(fight)
		if rule != "guard" {
			t.Fatalf("rule %q, want guard", rule)
		}
		if d := moveDest(t, a); b.groundDist(d, d) <= 50 {
			t.Fatalf("move to %v leads no further out", d)
		}
	})
	t.Run("every heading beyond the leash: orStay", func(t *testing.T) {
		b := duelGroundBrain(t, Point{X: -1830}, func(z *datapack.Zones) { z.Peace = []datapack.Zone{box(50, -500, 500, 500)} })
		b.pvp.guard.intent = pcID
		b.world.Self.move = stopAt(Point{X: 10})
		rule, a := b.decide(fight)
		if d := moveDest(t, a); rule != "guard" || d.X != 10 || d.Y != 0 {
			t.Fatalf("rule %q move to %v, want guard and the bot's own spot", rule, d)
		}
	})
	t.Run("the opponent 30 inside a peace zone: only in-range steps; back 200 out: Action", func(t *testing.T) {
		b := duelGroundBrain(t, Point{Y: 300}, func(z *datapack.Zones) { z.Peace = []datapack.Zone{box(-500, 270, 500, 1000)} })
		if _, a := b.decide(fight); a.kind == actAttack {
			t.Fatal("attacked across the peace zone's margin")
		}
		place(t, b, pcID, Point{Y: 70})
		if _, a := b.decide(fight); a.kind != actAttack {
			t.Fatalf("decision %v back 200 out, want the attack", a.kind)
		}
	})
	t.Run("the segment to a walking opponent nears a town: the disengage at once, Action once clear", func(t *testing.T) {
		b := duelGroundBrain(t, Point{X: -300, Y: 300}, func(z *datapack.Zones) { z.Towns = []datapack.Zone{box(60, 100, 500, 200)} })
		b.engaged, b.pvp.guard.intent = pcID, pcID
		b.handleOne(t, MoveToLocation{ObjectID: pcID, Origin: Point{X: -300, Y: 300}, Dest: Point{Y: 300}}, fight)
		if b.reactAt.IsZero() || b.reactAt.After(fight.Add(time.Second/frameRate)) {
			t.Errorf("woke at %v, want the next frame", b.reactAt.Sub(t0))
		}
		place(t, b, pcID, Point{Y: 300})
		play(t, b, []beat{
			{at: 7 * time.Second, kind: actMove, rule: "guard"},
			{at: 8 * time.Second, ev: StopMove{ObjectID: pcID, Pos: Point{X: -300, Y: 300}}, kind: actAttack},
		})
	})
	t.Run("its own leg ending 60 from an arena: the disengage move", func(t *testing.T) {
		b := duelGroundBrain(t, Point{Y: 300}, func(z *datapack.Zones) { z.Arenas = []datapack.Zone{box(500, -1000, 1000, 1000)} })
		b.pvp.guard.intent = pcID
		b.world.Self.RunSpeed = 100
		b.handleOne(t, MoveToLocation{ObjectID: selfID, Origin: Point{}, Dest: Point{X: 440}}, fight)
		if rule, a := b.decide(fight); rule != "guard" || a.kind != actMove {
			t.Fatalf("rule %q kind %v, want the guard's move", rule, a.kind)
		}
	})
}
