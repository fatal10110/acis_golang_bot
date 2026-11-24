package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// loseLeader takes the GM out of the party at lost and steps the brain to
// rule "leader"'s first /unstuck: the whisper at the end of LeaderWait, the
// ask chatGap later. It returns when the ask went out.
func loseLeader(t *testing.T, b *Brain, lost time.Time) time.Time {
	t.Helper()
	deathApply(t, b, lost, PartySmallWindowDeleteAll{}, DeleteObject{ObjectID: gmID})
	now := lost.Add(b.persona.Recover.LeaderWait)
	if name, a := deathStep(t, b, now); name != "leader" || a.kind != actTell {
		t.Fatalf("after the wait: %q kind %v, want the whisper", name, a.kind)
	}
	now = now.Add(chatGap)
	if name, a := deathStep(t, b, now); name != "leader" || a.kind != actUserCommand {
		t.Fatalf("after the whisper: %q kind %v, want /unstuck", name, a.kind)
	}
	return now
}

// TestLeaderEscapeBreaker bounds rule "leader"'s /unstuck (#335): the
// server's petition refusal stops the asks until a teleport, the shared
// breaker allows escapeTries in escapeEvery, and a recall that landed
// allows none for escapeEvery, even in a later loss.
func TestLeaderEscapeBreaker(t *testing.T) {
	type step struct {
		at     time.Duration // from the first /unstuck
		events []Event
		kind   actionKind // of rule "leader"
		// noAsk: rule "leader" is not in charge (the bot is in town, or
		// followed again); only no /unstuck is checked.
		noAsk bool
	}
	petition := []Event{SystemMessage{ID: msgNoUnstuckPetition}}
	land := []Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 80000}}}
	rejoin := []Event{PartySmallWindowAll{LeaderID: gmID, Members: []PartyMember{gmRow}}}
	leave := []Event{PartySmallWindowDeleteAll{}}
	wait := DefaultPersona().Recover.LeaderWait
	tests := []struct {
		name  string
		steps []step
	}{
		{"petition refusal stops the asks", []step{
			{at: time.Second, events: petition, kind: actHold},
			{at: escapeRetry + time.Second, kind: actHold},
			{at: escapeEvery + time.Minute, kind: actHold},
			{at: 2 * escapeEvery, kind: actHold},
		}},
		{"breaker after escapeTries asks", []step{
			{at: escapeRetry, kind: actUserCommand},
			{at: 2 * escapeRetry, kind: actUserCommand},
			{at: 3 * escapeRetry, kind: actHold},
			{at: escapeEvery - time.Second, kind: actHold},
			{at: escapeEvery, kind: actUserCommand},
		}},
		{"no ask for escapeEvery after a landed recall", []step{
			{at: time.Minute, events: land, noAsk: true},
			{at: 2 * time.Minute, events: rejoin, noAsk: true},
			{at: 3 * time.Minute, events: leave, kind: actHold},
			{at: 3*time.Minute + wait, kind: actTell},
			{at: 3*time.Minute + wait + chatGap, kind: actHold},
			{at: time.Minute + escapeEvery - time.Second, kind: actHold},
			{at: time.Minute + escapeEvery, kind: actUserCommand},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newLeaderBrain(t)
			first := loseLeader(t, b, at(time.Second))
			for i, s := range tt.steps {
				now := first.Add(s.at)
				if len(s.events) > 0 {
					deathApply(t, b, now, s.events...)
				}
				name, a := deathStep(t, b, now)
				if s.noAsk {
					if a.kind == actUserCommand {
						t.Fatalf("step %d at %v: %q sent /unstuck", i, s.at, name)
					}
					continue
				}
				if name != "leader" || a.kind != s.kind {
					t.Fatalf("step %d at %v: %q kind %v, want leader kind %v", i, s.at, name, a.kind, s.kind)
				}
			}
		})
	}
}

// TestLeaderEscapeRefusalScoped keeps each rule's petition refusal its own
// (#335): rule "leader"'s refused /unstuck, asked in a boss zone the bot may
// walk out of, does not stop the walker on isolated ground, and the
// walker's refused one does not stop rule "leader".
func TestLeaderEscapeRefusalScoped(t *testing.T) {
	petition := SystemMessage{ID: msgNoUnstuckPetition}
	t.Run("leader refusal leaves the walker asking", func(t *testing.T) {
		from, dest := scenePoint(geotest.PitInside, 0, 0), scenePoint(geotest.PitOutside, 0, 0)
		b, _, _ := newNavBrain(t, testNav(t), Persona{}, from)
		b.lead = leaderWatch{id: gmID, lost: t0}
		if ok, err := b.send(escapeAction(), t0); !ok || err != nil {
			t.Fatalf("leader /unstuck: sent %v, %v", ok, err)
		}
		deathApply(t, b, t0.Add(time.Second), petition)
		if !b.lead.refused || b.walk.escapeRefused {
			t.Fatalf("leader refused %v, walker refused %v: want the leader's only", b.lead.refused, b.walk.escapeRefused)
		}
		now := t0.Add(2 * time.Second)
		if a, st := walkStep(t, b, dest, now); a.kind != actUserCommand || st != walkWalking {
			t.Fatalf("walker on isolated ground: kind %v status %v (%s), want /unstuck", a.kind, st, b.walk.reason)
		}
		deathApply(t, b, now.Add(time.Second), petition)
		if !b.walk.escapeRefused {
			t.Fatal("walker's own refusal not kept")
		}
	})
	t.Run("walker refusal leaves the leader asking", func(t *testing.T) {
		b := newLeaderBrain(t)
		walk := escapeAction()
		walk.walk = true
		if ok, err := b.send(walk, t0); !ok || err != nil {
			t.Fatalf("walker /unstuck: sent %v, %v", ok, err)
		}
		deathApply(t, b, t0.Add(time.Second), petition)
		if !b.walk.escapeRefused {
			t.Fatal("walker's refusal not kept")
		}
		first := loseLeader(t, b, at(time.Minute))
		if b.lead.refused {
			t.Fatal("walker's refusal kept by rule leader")
		}
		deathApply(t, b, first.Add(time.Second), petition)
		if !b.lead.refused {
			t.Fatal("leader's own refusal not kept")
		}
	})
}
