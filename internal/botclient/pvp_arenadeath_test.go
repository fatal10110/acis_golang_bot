package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestPvPArenaDeath checks the arena return loop (docs/pvp/karma.md "By
// fight"): arena deaths one to four within an hour restart the bot and keep
// the trip, so it walks back; the fifth ends the trip, sending it to its
// spot; a death after the first aged past the hour keeps the trip.
func TestPvPArenaDeath(t *testing.T) {
	arena := box(-500, -500, 500, 500)
	// die kills the bot in the arena at d, then restarts it in the village
	// (a loading-screen teleport) a second later.
	die := func(t *testing.T, b *Brain, d time.Duration) {
		t.Helper()
		b.world.Self.move = stopAt(Point{})
		b.handleOne(t, hitSelf(pcID, 50), at(d))
		b.handleOne(t, Die{ObjectID: selfID, Restart: RestartPoints{Village: true}}, at(d+time.Second))
		if rule, a := b.decide(at(d + 2*time.Second)); rule != "dead" || a.kind != actRestart {
			t.Fatalf("death at %v: rule %q action %v, want the village restart", d, rule, a.kind)
		}
		b.handleOne(t, TeleportToLocation{ObjectID: selfID, Pos: Point{X: 20000, Y: 20000}}, at(d+3*time.Second))
		b.handleOne(t, Revive{ObjectID: selfID}, at(d+3*time.Second))
	}
	tripBrain := func(t *testing.T) *Brain {
		b := huntBrain(t)
		b.persona.Death = restartToVillage
		b.spot.held = false // released for the trip
		arenaTripAt(b, arena)
		return b
	}
	t.Run("deaths one to four walk back, the fifth goes to its spot", func(t *testing.T) {
		b := tripBrain(t)
		for i := range arenaDeaths - 1 {
			d := time.Duration(i) * 10 * time.Minute
			die(t, b, d)
			if tr := &b.pvp.arena; !tr.active() || tr.arena != 1 || tr.died {
				t.Fatalf("death %d: trip active %v arena %d died %v, want the walk back", i+1, tr.active(), tr.arena, tr.died)
			}
		}
		die(t, b, 40*time.Minute)
		if tr := &b.pvp.arena; tr.active() || tr.due.Before(at(40*time.Minute).Add(arenaEvery-arenaJitter)) {
			t.Fatalf("fifth death: trip active %v, next due %v: want it ended and the next one armed", tr.active(), tr.due)
		}
	})
	t.Run("a death after the first aged past the hour walks back", func(t *testing.T) {
		b := tripBrain(t)
		for _, d := range []time.Duration{0, 10 * time.Minute, 20 * time.Minute, 30 * time.Minute, 61 * time.Minute} {
			die(t, b, d)
		}
		if !b.pvp.arena.active() {
			t.Fatal("the trip ended with four deaths in the last hour")
		}
	})
	t.Run("a resurrection inside keeps the trip and needs no walk back", func(t *testing.T) {
		b := tripBrain(t)
		b.handleOne(t, Die{ObjectID: selfID}, at(time.Second))
		b.handleOne(t, Revive{ObjectID: selfID}, at(2*time.Second))
		b.ruleArena(at(3 * time.Second))
		if tr := &b.pvp.arena; !tr.active() || tr.died {
			t.Fatalf("trip active %v died %v, want the trip on, nothing to walk back from", tr.active(), tr.died)
		}
	})
	t.Run("an arena death off a trip ends nothing", func(t *testing.T) {
		b := huntBrain(t)
		b.spot.held = false
		b.cfg.Travel.zones.Arenas = []datapack.Zone{arena}
		b.handleOne(t, Die{ObjectID: selfID}, at(time.Second))
		if len(b.karma.arenaDeaths) != 0 || b.pvp.arena.active() {
			t.Fatal("an arena death off a trip counted")
		}
	})
}
