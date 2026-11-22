package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestWalkerTalkAnswers feeds the server's answers to a talk click and
// checks which count as a blocked stop of the talk approach: only the
// blocked correction (a same-point MoveToLocation) short of talking range
// while the click's answer is awaited (targeting.go onPlayerArrivedBlocked).
func TestWalkerTalkAnswers(t *testing.T) {
	n := testNav(t)
	from := scenePoint(geotest.UInside, 0, 0)
	npc := Point{X: from.X + 600, Y: from.Y, Z: from.Z}
	near := Point{X: npc.X - 60, Y: npc.Y, Z: npc.Z}
	still := func(p Point) MoveToLocation { return MoveToLocation{ObjectID: selfID, Origin: p, Dest: p} }
	tests := []struct {
		name string
		// before runs between the click and the answer at.
		before  func(b *Brain)
		at      time.Duration
		answer  []Event
		blocked uint64
	}{
		{
			name:    "correction short of range is a blocked talk",
			at:      time.Second,
			answer:  []Event{ActionFailed{}, still(from)},
			blocked: 1,
		},
		{
			name:   "correction in range is no blocked talk",
			at:     time.Second,
			answer: []Event{ActionFailed{}, still(near)},
		},
		{
			name:   "in-range StopMove ends the wait",
			at:     time.Second,
			answer: []Event{ActionFailed{}, StopMove{ObjectID: selfID, Pos: near}, still(from)},
		},
		{
			name:   "the page ends the wait",
			at:     time.Second,
			answer: []Event{NpcHtmlMessage{}, still(from)},
		},
		{
			name:   "another action ends the wait",
			before: func(b *Brain) { b.walk.sent(Action{kind: actAttack, object: 9}, t0.Add(time.Second)) },
			at:     2 * time.Second,
			answer: []Event{still(from)},
		},
		{
			name:   "the wait expires",
			at:     talkWait + time.Second,
			answer: []Event{still(from)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _, reg := newNavBrain(t, n, Persona{}, from)
			b.walk.talking(npc, t0)
			b.walk.sent(talkAction(77, from), t0)
			if tt.before != nil {
				tt.before(b)
			}
			now := t0.Add(tt.at)
			b.handle(tt.answer, now)
			if s := reg.Collect(now); s.NavBlocked != tt.blocked {
				t.Errorf("nav blocked %d, want %d", s.NavBlocked, tt.blocked)
			}
			if b.walk.talkFailed != (tt.blocked > 0) {
				t.Errorf("talkFailed %v, want %v", b.walk.talkFailed, tt.blocked > 0)
			}
		})
	}
}

// TestWalkerRetriesUnreachable ends a walk unreachable by the watchdog and
// checks that the same walk is asked again afresh after walkRetry, or at
// once after the bot's own teleport.
func TestWalkerRetriesUnreachable(t *testing.T) {
	n := testNav(t)
	from, dest := scenePoint(geotest.UInside, 0, 0), scenePoint(geotest.UBehind, 0, 0)
	tests := []struct {
		name string
		// after runs once the walk ended, at; the next goTo is at again.
		after func(b *Brain, at time.Time) time.Time
	}{
		{name: "retry after walkRetry", after: func(_ *Brain, at time.Time) time.Time { return at.Add(walkRetry) }},
		{name: "own teleport", after: func(b *Brain, at time.Time) time.Time {
			b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: from}}, at.Add(time.Second))
			return at.Add(2 * time.Second)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _, _ := newNavBrain(t, n, Persona{}, from)
			now := t0
			st := walkWalking
			for i := 0; st != walkUnreachable; i++ {
				if i > 2*walkMaxWatchdog {
					t.Fatalf("walk never ended (%s)", b.walk.reason)
				}
				_, st = walkStep(t, b, dest, now)
				now = now.Add(walkAnswerWait)
			}
			if a, st := b.goTo(dest, 0, walkPlace, now.Add(walkRetry/2)); st != walkUnreachable || a.kind != actHold {
				t.Fatalf("within walkRetry: status %v kind %v, want still unreachable", st, a.kind)
			}
			next := tt.after(b, now)
			if a, st := b.goTo(dest, 0, walkPlace, next); st != walkWalking || a.kind != actMove {
				t.Fatalf("after: status %v kind %v (%s), want a fresh walk", st, a.kind, b.walk.failed())
			}
		})
	}
}
