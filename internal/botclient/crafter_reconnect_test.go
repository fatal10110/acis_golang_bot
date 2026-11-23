package botclient

import (
	"testing"
	"time"
)

// reconnectCrafter is the next session's brain of the crafter whose link l
// the last session's brain played on: a fresh world and brain, the same
// Bot's link and board, started at now.
func reconnectCrafter(t *testing.T, board *CraftBoard, l *botLink, now time.Time) *Brain {
	t.Helper()
	fresh := newCrafterBrain(t)
	cfg := fresh.cfg
	cfg.link, cfg.CraftBoard = l, board
	b := NewBrain(fresh.world, NewEvents(64), &recordSender{}, newFakeClock(now), cfg)
	b.start(now)
	return b
}

// TestBrainCrafterReconnect checks that the crafter's last cycle end and
// its breaker's hold outlive the session (#233): a hold tripped in one
// session holds the next check of the next one, a board order with its
// materials in too, until BreakerHold ends; past a cycle that did not trip
// the next session checks at once; the panel's status keeps the last cycle
// and the hold.
func TestBrainCrafterReconnect(t *testing.T) {
	end := at(10 * time.Second)
	tests := []struct {
		name string
		// last ends the first session's cycle at end.
		last      func(b *Brain)
		reconnect bool
		// held: a breaker's hold holds the next check, else it is at once.
		held     bool
		wantLast CraftOutcome
	}{
		{"breaker tripped, same session", func(b *Brain) { b.tripCrafter(end) }, false, true, CraftCycleTripped},
		{"breaker tripped, next session", func(b *Brain) { b.tripCrafter(end) }, true, true, CraftCycleTripped},
		{"nothing to craft, next session", func(b *Brain) { b.endCraftCycle(end, true) }, true, false, CraftCycleNothing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := makingBrain(t)
			board := b.cfg.CraftBoard
			l := linkBrain(b, standing{})
			tt.last(b)
			hold, until := b.persona.Crafter.BreakerHold, time.Time{}
			if tt.held {
				until = end.Add(hold)
			}
			if want := (craftEnded{last: CraftCycleEnd{At: end, Outcome: tt.wantLast}, holdUntil: until}); l.crafter != want {
				t.Fatalf("link keeps %+v, want %+v", l.crafter, want)
			}
			// A command order is posted with its materials in, and the bot
			// can make it.
			if _, ok := board.PostCommand(77, OrderByGM, "Gm", steel, 1); !ok {
				t.Fatal("post refused")
			}
			first := end.Add(time.Minute)
			if tt.reconnect {
				b = reconnectCrafter(t, board, l, first)
			}

			b.publishStatus(first, true)
			st := l.board.load().Crafter
			if st == nil || st.Last != (CraftCycleEnd{At: end, Outcome: tt.wantLast}) {
				t.Fatalf("status crafter = %+v, want the last cycle %v at %v", st, tt.wantLast, end)
			}
			if !st.BreakerUntil.Equal(until) {
				t.Fatalf("status breaker until %v, want %v", st.BreakerUntil, until)
			}
			if !tt.held {
				if rule, _ := b.decide(first); rule != "craft" {
					t.Fatalf("decide at %v = %q, want the check at once", first.Sub(t0), rule)
				}
				return
			}
			for _, now := range []time.Time{first, end.Add(hold - time.Second)} {
				if rule, _ := b.decide(now); rule == "craft" {
					t.Fatalf("decide at %v = craft, want the breaker's hold to %v", now.Sub(t0), end.Add(hold).Sub(t0))
				}
			}
			if rule, _ := b.decide(end.Add(hold)); rule != "craft" {
				t.Fatalf("decide at the hold's end = %q, want the check", rule)
			}
		})
	}
}
