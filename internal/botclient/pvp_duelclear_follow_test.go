package botclient

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestPvPDuelClearFollow checks the server's follow against the monster
// clearance (docs/pvp/finding.md "The server's follow", #606): in a duel,
// an opponent out of reach whose straight segment from the bot crosses an
// aggressive monster's aggro range plus 300 gets no swing or cast the
// server would walk the bot for, and an intention on it drops with a
// disengage move; a passive monster holds nothing.
func TestPvPDuelClearFollow(t *testing.T) {
	fight := at(7 * time.Second)
	noZones := func(*datapack.Zones) {}
	// The monster 395 east of the segment from the bot at (0, 0) to the
	// opponent at (0, 300): its disc (400) crosses the segment, not either
	// end (422 off).
	mob := Point{X: 395, Y: 150}

	t.Run("the opponent out of reach across the disc: no swing; back clear of it: the swing", func(t *testing.T) {
		b := duelGroundBrain(t, Point{Y: 300}, noZones)
		aggroMob(t, b, mob, clearAggro)
		if _, a := b.decide(fight); a.kind == actAttack {
			t.Fatal("attacked across the monster's clearance")
		}
		// At (0, 70) the segment ends 403 off the monster.
		place(t, b, pcID, Point{Y: 70})
		if _, a := b.decide(fight); a.kind != actAttack {
			t.Fatalf("decision %v clear of the disc, want the attack", a.kind)
		}
	})
	t.Run("a passive monster there: the swing", func(t *testing.T) {
		b := duelGroundBrain(t, Point{Y: 300}, noZones)
		aggroMob(t, b, mob, 0)
		if _, a := b.decide(fight); a.kind != actAttack {
			t.Fatalf("decision %v, want the attack", a.kind)
		}
	})
	t.Run("the intention on it: the disengage move, clear of the disc", func(t *testing.T) {
		b := duelGroundBrain(t, Point{Y: 300}, noZones)
		aggroMob(t, b, mob, clearAggro)
		b.engaged, b.pvp.guard.intent = pcID, pcID
		rule, a := b.decide(fight)
		if rule != "guard" {
			t.Fatalf("rule %q, want guard", rule)
		}
		if d := moveDest(t, a); !b.moveOK(Point{}, d, fight) {
			t.Fatalf("disengage move to %v fails moveOK", d)
		}
	})
	t.Run("a walking opponent whose walk ends across the disc: the disengage at once", func(t *testing.T) {
		b := duelGroundBrain(t, Point{X: -300, Y: 300}, noZones)
		aggroMob(t, b, mob, clearAggro)
		b.engaged, b.pvp.guard.intent = pcID, pcID
		b.handleOne(t, MoveToLocation{ObjectID: pcID, Origin: Point{X: -300, Y: 300}, Dest: Point{Y: 300}}, fight)
		if rule, a := b.decide(fight); rule != "guard" || a.kind != actMove {
			t.Fatalf("rule %q kind %v, want the guard's move", rule, a.kind)
		}
	})

	// The cast: an out-of-range cast approaches like the swing (castReach).
	const skill = int32(1177)
	far := Point{Y: 1000}
	tests := []struct {
		name  string
		aggro int
		want  bool
	}{
		{name: "a cast out of range across an aggressive monster's disc: held", aggro: clearAggro, want: true},
		{name: "a cast out of range by a passive monster: not held", aggro: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := duelGroundBrain(t, far, noZones)
			aggroMob(t, b, Point{X: 395, Y: 500}, tc.aggro)
			if got := b.stepHeld(pcID, skill, fight); got != tc.want {
				t.Fatalf("stepHeld = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPvPDuelClearRoute checks the walked route of a duel's chase (#606):
// the bot inside the U, its opponent behind the U's base; the route to the
// nearest stand point bends around an arm, and an aggressive monster beyond
// that arm reaches the route but not the straight segment. Outside a duel
// the bot walks it; in a duel that stand point is skipped (no segment goes
// out toward it), and any walk after keeps every segment of its route clear.
func TestPvPDuelClearRoute(t *testing.T) {
	inside, behind := scenePoint(geotest.UInside, 0, 0), scenePoint(geotest.UBehind, 0, 0)
	now := at(7 * time.Second)
	r := reach{rng: 50}
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

	// The route to the nearest stand point with no monster around.
	b0 := brain(t, true)
	if a, walk := b0.engageWalk(pcID, r, now); !walk || a.kind != actMove || !a.walk {
		t.Fatalf("walk %v kind %v, want a walker segment", walk, a.kind)
	}
	nearest, route := b0.engage.stand, append([]Point(nil), b0.walk.planned...)
	// The monster 380 beyond the route's waypoint farthest off the
	// straight line inside-nearest (a horizontal line): it reaches that
	// waypoint, not the line, the bot or the stand point.
	far := route[0]
	for _, p := range route {
		if abs32(p.Y-inside.Y) > abs32(far.Y-inside.Y) {
			far = p
		}
	}
	off := int32(380)
	if far.Y < inside.Y {
		off = -off
	}
	mob := Point{X: far.X, Y: far.Y + off, Z: far.Z}
	disc := float64(clearAggro + duelClear)
	if d := segmentDist(float64(mob.X), float64(mob.Y), float64(inside.X), float64(inside.Y), float64(nearest.X), float64(nearest.Y)); d <= disc {
		t.Fatalf("the straight segment %v-%v passes %.0f from the monster at %v: the route bends too little", inside, nearest, d, mob)
	}
	crosses := func(b *Brain, route []Point) bool {
		for i := 0; i+1 < len(route); i++ {
			if !b.moveOK(route[i], route[i+1], now) {
				return true
			}
		}
		return false
	}

	t.Run("outside a duel: the walk along the route", func(t *testing.T) {
		b := brain(t, false)
		aggroMob(t, b, mob, clearAggro)
		a, walk := b.engageWalk(pcID, r, now)
		if !walk || a.kind != actMove || !a.walk || b.engage.stand != nearest {
			t.Fatalf("walk %v kind %v stand %v, want a walk to %v", walk, a.kind, b.engage.stand, nearest)
		}
	})
	t.Run("in a duel: the stand point skipped, every later walk clear", func(t *testing.T) {
		b := brain(t, true)
		aggroMob(t, b, mob, clearAggro)
		if !b.moveOK(inside, nearest, now) || !crosses(b, route) {
			t.Fatalf("scene: the straight segment must pass moveOK and the route fail it")
		}
		a, walk := b.engageWalk(pcID, r, now)
		if !walk || a.kind != actHold {
			t.Fatalf("walk %v kind %v, want no segment out", walk, a.kind)
		}
		if b.engage.stand != (Point{}) || len(b.engage.skipped) != 1 || b.engage.skipped[0] != nearest {
			t.Fatalf("stand %v skipped %v, want %v skipped", b.engage.stand, b.engage.skipped, nearest)
		}
		if _, ok := b.blacklist[pcID]; ok {
			t.Fatal("the duel opponent blacklisted")
		}
		for i := 1; i <= duelStandSkips; i++ {
			at := now.Add(time.Duration(i) * 100 * time.Millisecond)
			a, walk := b.engageWalk(pcID, r, at)
			if !walk {
				t.Fatalf("decision %d: no walk or hold", i)
			}
			if a.kind == actMove && a.walk {
				st := b.engage.stand
				if horizontal2(st, nearest) <= duelSkipNear*duelSkipNear {
					t.Fatalf("decision %d: walk to %v within %d of the skipped %v", i, st, duelSkipNear, nearest)
				}
				if crosses(b, append([]Point{b.walk.next.from}, b.walk.route...)) {
					t.Fatalf("decision %d: the route to %v fails moveOK", i, st)
				}
				return
			}
		}
		if b.engage.held.IsZero() {
			t.Fatalf("no clear stand point and no hold after %d skips: %v", duelStandSkips, b.engage.skipped)
		}
	})
}

// abs32 is |v|.
func abs32(v int32) int32 {
	return int32(math.Abs(float64(v)))
}
