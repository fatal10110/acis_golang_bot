package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// Duel clearance fixtures: an aggressive monster clearMob of template
// clearNPC, whose aggro range clearAggro plus duelClear makes its disc 400.
const (
	clearMob   = int32(0x20000100)
	clearNPC   = int32(20999)
	clearAggro = 100
)

// aggroMob puts clearMob at p with aggro range aggro on b's travel map.
func aggroMob(t *testing.T, b *Brain, p Point, aggro int) {
	t.Helper()
	if b.cfg.Travel.aggro == nil {
		b.cfg.Travel.aggro = map[int32]int{}
	}
	b.cfg.Travel.aggro[clearNPC] = aggro
	b.world.Apply(NPCInfo{ObjectID: clearMob, NPCID: clearNPC, Attackable: true, Pos: p}, t0)
}

// TestPvPDuelClearMove checks the monster clearance of a duel's moves
// (docs/pvp/finding.md "Clear of monsters", #552): moveOK refuses a step
// whose end or straight segment comes within an aggressive monster's aggro
// range plus 300, lets a step from inside such a disc out, and checks
// nothing outside a duel or for a passive monster.
func TestPvPDuelClearMove(t *testing.T) {
	fight := at(7 * time.Second)
	tests := []struct {
		name     string
		duel     bool
		mob      Point
		aggro    int
		from, to Point
		want     bool
	}{
		{name: "the end within aggro plus 300", duel: true, mob: Point{X: 1000}, aggro: clearAggro, to: Point{X: 650}},
		{name: "the end beyond it", duel: true, mob: Point{X: 1000}, aggro: clearAggro, to: Point{X: 590}, want: true},
		{name: "the path crosses the disc", duel: true, mob: Point{X: 500, Y: 300}, aggro: clearAggro, to: Point{X: 1000}},
		{name: "the path passes beside it", duel: true, mob: Point{X: 500, Y: 450}, aggro: clearAggro, to: Point{X: 1000}, want: true},
		{name: "from inside, a step out", duel: true, mob: Point{X: 200}, aggro: clearAggro, to: Point{X: -300}, want: true},
		{name: "from inside, a step deeper in", duel: true, mob: Point{X: 200}, aggro: clearAggro, to: Point{X: 150}},
		{name: "a passive monster", duel: true, mob: Point{X: 1000}, to: Point{X: 650}, want: true},
		{name: "outside a duel", mob: Point{X: 1000}, aggro: clearAggro, to: Point{X: 650}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := guardBrain(t, false, 0, 0)
			if tc.duel {
				duelUntil(t, b, DuelFighting, t0)
			}
			place(t, b, pcID, Point{X: 60})
			aggroMob(t, b, tc.mob, tc.aggro)
			if got := b.moveOK(tc.from, tc.to, fight); got != tc.want {
				t.Fatalf("moveOK(%v, %v) = %v, want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

// TestPvPDuelClearStand checks the duel's chase stand point (engageWalk,
// #552): the bot inside the U, its duel opponent behind the U's base and
// out of sight; outside a duel the nearest stand point is taken whatever
// the monsters; in a duel a stand point within an aggressive monster's
// aggro range plus 300 is skipped like a blocked one and the nearest
// passing one taken; with none passing the bot holds where it stands, the
// opponent not blacklisted.
func TestPvPDuelClearStand(t *testing.T) {
	inside, behind := scenePoint(geotest.UInside, 0, 0), scenePoint(geotest.UBehind, 0, 0)
	now := at(7 * time.Second)
	r := reach{rng: 50} // melee: stand points within meleeStand (100)
	brain := func(t *testing.T, duel bool) *Brain {
		t.Helper()
		b, _, _ := newNavBrain(t, testNav(t), Persona{BlacklistFor: time.Minute}, inside)
		b.cfg.Travel = &TravelMap{}
		b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: behind, RunSpeed: 100, CollisionRadius: 9}, t0)
		if duel {
			duelUntil(t, b, DuelFighting, t0)
		}
		return b
	}
	mode, rng := r.stand()
	if mode != nav.StandLine {
		t.Fatalf("stand mode %v, want the straight line", mode)
	}
	nearest, ok := brain(t, false).walker().standPoint(inside, behind, rng, mode, defaultTargetHeight)
	if !ok {
		t.Fatal("no stand point behind the U")
	}
	// The monster 350 north of the nearest stand point: its disc holds
	// that point, not the bot nor those south of the opponent.
	mob := Point{X: nearest.X, Y: nearest.Y - 350, Z: nearest.Z}
	disc := int64(clearAggro + duelClear)

	t.Run("outside a duel: the nearest stand point", func(t *testing.T) {
		b := brain(t, false)
		aggroMob(t, b, mob, clearAggro)
		a, walk := b.engageWalk(pcID, r, now)
		if !walk || a.kind != actMove || b.engage.stand != nearest {
			t.Fatalf("walk %v kind %v stand %v, want a walk to %v", walk, a.kind, b.engage.stand, nearest)
		}
	})
	t.Run("in a duel: the nearest stand point clear of the monster", func(t *testing.T) {
		b := brain(t, true)
		aggroMob(t, b, mob, clearAggro)
		a, walk := b.engageWalk(pcID, r, now)
		st := b.engage.stand
		if !walk || a.kind != actMove || st == (Point{}) {
			t.Fatalf("walk %v kind %v stand %v, want a walk", walk, a.kind, st)
		}
		if st == nearest || dist2(st, mob) <= disc*disc || segmentDist(float64(mob.X), float64(mob.Y), float64(inside.X), float64(inside.Y), float64(st.X), float64(st.Y)) <= float64(disc) {
			t.Fatalf("stand point %v within %d of the monster at %v", st, disc, mob)
		}
		if !b.moveOK(inside, st, now) {
			t.Fatalf("stand point %v fails moveOK", st)
		}
	})
	t.Run("in a duel, a chosen stand point the monster's disc reaches: chosen again", func(t *testing.T) {
		b := brain(t, true)
		if _, walk := b.engageWalk(pcID, r, now); !walk || b.engage.stand != nearest {
			t.Fatalf("stand %v, want %v before the monster", b.engage.stand, nearest)
		}
		aggroMob(t, b, mob, clearAggro)
		if _, walk := b.engageWalk(pcID, r, now.Add(time.Second)); !walk || b.engage.stand == nearest || b.engage.stand == (Point{}) {
			t.Fatalf("stand %v, want a new one clear of the monster", b.engage.stand)
		}
	})
	t.Run("in a duel with no stand point passing: the hold", func(t *testing.T) {
		b := brain(t, true)
		// The monster at the opponent: its disc (350) holds every stand
		// point, not the bot (480 off).
		aggroMob(t, b, behind, 50)
		a, walk := b.engageWalk(pcID, r, now)
		if !walk || a.kind != actHold {
			t.Fatalf("walk %v kind %v, want the hold", walk, a.kind)
		}
		if _, ok := b.blacklist[pcID]; ok {
			t.Fatal("the duel opponent blacklisted")
		}
		if _, walk := b.engageWalk(pcID, r, now.Add(time.Second)); !walk || !b.engage.held.Equal(now) {
			t.Fatalf("searched again within duelStandRetry: held %v", b.engage.held)
		}
	})
	t.Run("in a duel, a walk under way when no stand point passes: one stop, then the hold", func(t *testing.T) {
		b := brain(t, true)
		if a, walk := b.engageWalk(pcID, r, now); !walk || a.kind != actMove || b.engage.stand != nearest {
			t.Fatalf("walk %v kind %v stand %v, want a walk to %v", walk, a.kind, b.engage.stand, nearest)
		}
		// The server walks the bot toward the stand point; then the
		// monster at the opponent fails every stand point mid-walk.
		b.world.Apply(MoveToLocation{ObjectID: selfID, Origin: inside, Dest: nearest}, now)
		aggroMob(t, b, behind, 50)
		mid := now.Add(200 * time.Millisecond)
		if !b.world.Self.Moving(mid) {
			t.Fatal("the bot not moving mid-walk")
		}
		pos := b.world.Self.Position(mid)
		a, walk := b.engageWalk(pcID, r, mid)
		if !walk || a.kind != actMove || moveDest(t, a) != b.standing(mid) || horizontal2(moveDest(t, a), pos) > 1 {
			t.Fatalf("walk %v kind %v to %v, want a stop at %v", walk, a.kind, a, pos)
		}
		if _, ok := b.blacklist[pcID]; ok {
			t.Fatal("the duel opponent blacklisted")
		}
		// The server's answer: the bot stands where it stopped.
		b.world.Apply(MoveToLocation{ObjectID: selfID, Origin: pos, Dest: pos}, mid)
		next := mid.Add(100 * time.Millisecond)
		if b.world.Self.Moving(next) {
			t.Fatal("the bot still moving after the stop")
		}
		if a, walk := b.engageWalk(pcID, r, next); !walk || a.kind != actHold {
			t.Fatalf("walk %v kind %v, want the hold", walk, a.kind)
		}
	})
}

// TestPvPDuelClearKite checks the kite's stand point (#552): in a duel the
// point straight away from the opponent within an aggressive monster's
// aggro range plus 300 is skipped, the first one clear of it taken.
func TestPvPDuelClearKite(t *testing.T) {
	foe := Point{Y: 300}
	b := kiteBrain(t, true, foe, nil, nil)
	aggroMob(t, b, Point{Y: -500}, clearAggro)
	rootLands(t, b, foe)
	_, a := b.decide(at(11*time.Second + 500*time.Millisecond))
	got := moveDest(t, a)
	if got == (Point{Y: 300 - 584}) || dist2(got, Point{Y: -500}) <= 400*400 {
		t.Fatalf("kite to %v, within 400 of the monster", got)
	}
}
