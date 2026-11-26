package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestPvPArenaFirst checks BrainConfig.ArenaFirst, the live arena
// scenario's seam: set, the first arena trip falls due that long after the
// first decision, and the trip after it keeps the arenaEvery timer; unset,
// the first one keeps the timer too.
func TestPvPArenaFirst(t *testing.T) {
	for _, tc := range []struct {
		name   string
		first  time.Duration
		lo, hi time.Duration
	}{
		{"unset", 0, arenaEvery - arenaJitter, arenaEvery + arenaJitter},
		{"set", 30 * time.Second, 30 * time.Second, 30 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newTripBrain(t)
			b.persona.PvP.Arenas = true
			b.cfg.ArenaFirst = tc.first
			b.cfg.Nav = testNav(t)
			b.walk = newWalker(b.cfg.Nav)
			b.world.SetGround(b.cfg.Nav.Engine)
			b.cfg.Travel.zones.Arenas = []datapack.Zone{box(1000, -500, 2000, 500)}
			if rule, _ := b.decide(at(time.Second)); rule == "arena" {
				t.Fatal("an arena trip at the first decision")
			}
			due := b.pvp.arena.due
			if d := due.Sub(at(time.Second)); d < tc.lo || d > tc.hi {
				t.Fatalf("first trip in %v, want %v to %v", d, tc.lo, tc.hi)
			}
			if rule, _ := b.decide(due.Add(-time.Second)); rule == "arena" {
				t.Fatal("an arena trip before it is due")
			}
			if rule, a := b.decide(due); rule != "arena" || a.kind != actMove {
				t.Fatalf("rule %q action %v when due, want the walk", rule, a.kind)
			}
			end := due.Add(time.Minute)
			b.arenaEnd(end)
			if d := b.pvp.arena.due.Sub(end); d < arenaEvery-arenaJitter || d > arenaEvery+arenaJitter {
				t.Fatalf("next trip in %v, want %v give or take %v", d, arenaEvery, arenaJitter)
			}
		})
	}
}
