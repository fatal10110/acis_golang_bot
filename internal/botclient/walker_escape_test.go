package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestWalkerEscapeRetry asks /unstuck again on isolated ground when the
// recall does not land the bot: escapeRetry after the ask when no cast ran,
// or after the end of a cast cut short; the breaker stops the asks after
// escapeTries in escapeEvery or for escapeEvery after a recall landed back
// on isolated ground, and the server's petition refusal stops them until a
// teleport (#286).
func TestWalkerEscapeRetry(t *testing.T) {
	n := testNav(t)
	from, dest := scenePoint(geotest.PitInside, 0, 0), scenePoint(geotest.PitOutside, 0, 0)
	type step struct {
		at      time.Duration
		events  []Event
		castEnd time.Duration // > 0: the recall cast ends then
		kind    actionKind
		status  walkStatus
	}
	ask := func(at time.Duration) step { return step{at: at, kind: actUserCommand, status: walkWalking} }
	hold := func(at time.Duration) step { return step{at: at, kind: actHold, status: walkWalking} }
	isolated := func(at time.Duration) step { return step{at: at, kind: actHold, status: walkUnreachable} }
	petition := []Event{SystemMessage{ID: msgNoUnstuckPetition}}
	teleport := []Event{TeleportToLocation{ObjectID: selfID, Pos: from}}
	cut := 2 * time.Second
	tests := []struct {
		name  string
		steps []step
	}{
		{"no cast asks again", []step{ask(0), hold(time.Second), hold(escapeRetry - time.Second), ask(escapeRetry)}},
		{"cast cut short asks again after its end", []step{
			ask(0),
			{at: time.Second, castEnd: 5 * time.Minute, kind: actHold, status: walkWalking},
			{at: cut, castEnd: cut, kind: actHold, status: walkWalking},
			hold(cut + escapeRetry - time.Second),
			ask(cut + escapeRetry),
		}},
		{"breaker opens after escapeTries", []step{
			ask(0), ask(escapeRetry), ask(2 * escapeRetry),
			isolated(3 * escapeRetry),
			isolated(3*escapeRetry + walkRetry),
			ask(escapeEvery),
		}},
		{"recall landing on isolated ground opens the breaker", []step{
			ask(0),
			{at: time.Second, castEnd: 5 * time.Minute, kind: actHold, status: walkWalking},
			{at: 5 * time.Minute, events: teleport, kind: actHold, status: walkUnreachable},
			isolated(5*time.Minute + walkRetry),
			isolated(5*time.Minute + escapeEvery - walkRetry/2),
			ask(5*time.Minute + escapeEvery + walkRetry/2),
		}},
		{"petition refusal gives up until a teleport", []step{
			ask(0),
			{at: time.Second, events: petition, kind: actHold, status: walkUnreachable},
			isolated(escapeEvery + walkRetry),
			{at: escapeEvery + walkRetry + time.Second, events: teleport, kind: actUserCommand, status: walkWalking},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _, _ := newNavBrain(t, n, Persona{}, from)
			for i, s := range tt.steps {
				now := t0.Add(s.at)
				if len(s.events) > 0 {
					if err := b.handle(s.events, now); err != nil {
						t.Fatal(err)
					}
				}
				if s.castEnd > 0 {
					b.world.Self.castEnd = t0.Add(s.castEnd)
				}
				a, st := walkStep(t, b, dest, now)
				if a.kind != s.kind || st != s.status {
					t.Fatalf("step %d at %v: kind %v status %v (%s), want kind %v status %v",
						i, s.at, a.kind, st, b.walk.reason, s.kind, s.status)
				}
				if st == walkUnreachable && b.walk.reason != reasonIsolated {
					t.Fatalf("step %d at %v: reason %q, want %q", i, s.at, b.walk.reason, reasonIsolated)
				}
			}
		})
	}
}
