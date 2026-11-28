package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// siegeRouteBrain stands a bot at from on the synthetic navigation with the
// battlefields fields, each seen running until until.
func siegeRouteBrain(t *testing.T, from Point, until time.Time, fields ...datapack.Zone) *Brain {
	t.Helper()
	b, _, _ := newNavBrain(t, testNav(t), Persona{}, from)
	b.cfg.Travel = &TravelMap{}
	b.cfg.Travel.zones.Sieges = fields
	b.pvp.siege.fields = make(map[int]time.Time, len(fields))
	for i := range fields {
		b.pvp.siege.fields[i+1] = until
	}
	return b
}

// siegeRoute is the route of b's walk to dest planned at now: the first
// segment's ends, then the rest of the route.
func siegeRoute(t *testing.T, b *Brain, dest Point, now time.Time) ([]Point, walkStatus) {
	t.Helper()
	a, st := walkStep(t, b, dest, now)
	if st != walkWalking {
		return nil, st
	}
	return append([]Point{moveOrigin(a.payload), moveTarget(a.payload)}, routePoints(b)...), st
}

// TestPvPSiegeRoute checks the walk past a running battlefield the bot does
// not fight in (docs/pvp/siege.md "Not taking part"): around it, and around
// every one running; straight out of the one it stands in and into the one
// it walks to; straight through its own castle's; around a karma margin
// and a battlefield together; straight across, as before, when no way
// around is left; straight again once the block lapsed.
func TestPvPSiegeRoute(t *testing.T) {
	from, dest := Point{X: 1000, Y: 2000}, Point{X: 7000, Y: 2000}
	running := t0.Add(siegeLength)
	field := box(3000, 1500, 4000, 2500)

	t.Run("running: around it, no segment inside", func(t *testing.T) {
		b := siegeRouteBrain(t, from, running, field)
		pts, st := siegeRoute(t, b, dest, t0)
		if st != walkWalking {
			t.Fatalf("status %v (%s), want a walk", st, b.walk.reason)
		}
		if d := clearOf(pts, field, nil); d <= 0 {
			t.Fatalf("route %v enters the battlefield, want it kept out", pts)
		}
		if end := pts[len(pts)-1]; horizontal2(end, dest) > 100*100 {
			t.Fatalf("route ends at %+v, want %+v", end, dest)
		}
	})
	t.Run("two running: around both", func(t *testing.T) {
		west, east := box(2500, 1500, 3200, 2500), box(5000, 1500, 5700, 2500)
		b := siegeRouteBrain(t, from, running, west, east)
		pts, st := siegeRoute(t, b, dest, t0)
		if st != walkWalking {
			t.Fatalf("status %v (%s), want a walk", st, b.walk.reason)
		}
		if clearOf(pts, west, nil) <= 0 || clearOf(pts, east, nil) <= 0 {
			t.Fatalf("route %v enters a battlefield, want both kept out", pts)
		}
	})
	t.Run("standing inside or walking into it: not avoided", func(t *testing.T) {
		b := siegeRouteBrain(t, Point{X: 3500, Y: 2000}, running, field)
		if b.siegeArea(Point{X: 3500, Y: 2000}, dest, t0) != nil {
			t.Fatal("the battlefield the bot stands in is avoided, want the walk out straight")
		}
		if b.siegeArea(from, Point{X: 3500, Y: 2000}, t0) != nil {
			t.Fatal("the battlefield the walk ends in is avoided")
		}
		if _, st := siegeRoute(t, b, dest, t0); st != walkWalking {
			t.Fatalf("status %v (%s), want the walk out", st, b.walk.reason)
		}
	})
	t.Run("taking part: straight through its castle's", func(t *testing.T) {
		own := field
		own.ResidenceCastle = int(siegeCastle)
		b := siegeRouteBrain(t, from, running, own)
		b.persona.PvP.Siege = true
		b.world.Clan.ID = siegeClan
		b.pvp.siege.side, b.pvp.siege.castle = botwire.SiegeAttacker, siegeCastle
		pts, st := siegeRoute(t, b, dest, t0)
		if st != walkWalking || clearOf(pts, own, nil) > 0 {
			t.Fatalf("status %v, route %v: want the straight walk through its own battlefield", st, pts)
		}
	})
	t.Run("karma: around the town margin and the battlefield", func(t *testing.T) {
		town, bf := box(2500, 1500, 3200, 2500), box(5000, 1500, 5700, 2500)
		b := siegeRouteBrain(t, from, running, bf)
		b.cfg.Travel.zones.Towns = []datapack.Zone{town}
		b.world.Self.Karma = 240
		pts, st := siegeRoute(t, b, dest, t0)
		if st != walkWalking {
			t.Fatalf("status %v (%s), want a walk", st, b.walk.reason)
		}
		if d := clearOf(pts, town, nil); d < karmaClear {
			t.Fatalf("route %v passes %.0f from the town, want at least %d", pts, d, karmaClear)
		}
		if clearOf(pts, bf, nil) <= 0 {
			t.Fatalf("route %v enters the battlefield, want it kept out", pts)
		}
	})
	t.Run("no way around: straight across, as before", func(t *testing.T) {
		// Four battlefields frame the destination: every way in crosses one.
		frame := []datapack.Zone{
			box(4500, 1000, 6500, 1600), box(4500, 2400, 6500, 3000),
			box(4500, 1000, 5100, 3000), box(5900, 1000, 6500, 3000),
		}
		mid := Point{X: 5500, Y: 2000}
		b := siegeRouteBrain(t, from, running, frame...)
		pts, st := siegeRoute(t, b, mid, t0)
		if st != walkWalking {
			t.Fatalf("status %v (%s), want the walk across", st, b.walk.reason)
		}
		if clearOf(pts, frame[2], nil) > 0 {
			t.Fatalf("route %v keeps out of the west battlefield, want the straight walk across it", pts)
		}
	})
	t.Run("lapsed: straight through, forgotten", func(t *testing.T) {
		b := siegeRouteBrain(t, from, running, field)
		pts, st := siegeRoute(t, b, dest, running)
		if st != walkWalking || clearOf(pts, field, nil) > 0 {
			t.Fatalf("status %v, route %v: want the straight walk once the block lapsed", st, pts)
		}
		if len(b.pvp.siege.fields) != 0 {
			t.Fatalf("fields %v, want the lapsed block forgotten", b.pvp.siege.fields)
		}
	})
}

// TestPvPSiegeFields checks that two battlefields running at once are kept
// apart (docs/pvp/siege.md "Telling 283 and 284 apart"): a 283 in the
// second keeps the first's block, a 284 or a lapse ends only its own.
func TestPvPSiegeFields(t *testing.T) {
	entered, left := SystemMessage{ID: msgEnteredCombatZone}, SystemMessage{ID: msgLeftCombatZone}
	hall, castle := Point{X: 22000, Y: 2000}, Point{X: 3000, Y: 2000}
	b := siegeBrain(t, false, hall)
	b.handleOne(t, entered, t0)
	b.world.Self.move = stopAt(castle)
	b.handleOne(t, entered, at(time.Second))
	now := at(2 * time.Second)
	if !b.siegeBlocks(hall, now) || !b.siegeBlocks(castle, now) || b.duelGround(hall) || b.duelGround(castle) {
		t.Fatalf("fields %v: want both battlefields blocked", b.pvp.siege.fields)
	}
	if got := b.runningFields(now); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("running %v, want [1 2]", got)
	}
	b.handleOne(t, left, at(3*time.Second))
	if b.siegeBlocks(castle, at(4*time.Second)) || !b.siegeBlocks(hall, at(4*time.Second)) {
		t.Fatalf("fields %v after 284 in the castle's: want only the hall's blocked", b.pvp.siege.fields)
	}
	b.handleOne(t, entered, at(5*time.Second))
	if b.siegeBlocks(hall, t0.Add(siegeLength)) || !b.siegeBlocks(castle, t0.Add(siegeLength)) {
		t.Fatalf("fields %v at the hall's lapse: want only the castle's blocked", b.pvp.siege.fields)
	}
}
