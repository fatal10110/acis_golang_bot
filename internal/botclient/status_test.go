package botclient

import (
	"errors"
	"runtime"
	"slices"
	"testing"
	"time"
)

// TestBrainStatusRate checks that a session's brain publishes its snapshot
// at most once every statusEvery, at once at its start, with what it sees.
func TestBrainStatusRate(t *testing.T) {
	b, _ := newCommandBrain(t, DefaultPersona())
	l := linkBrain(b, standing{cmd: Command{Kind: CommandStay}})
	l.board.store(Status{Preset: "fighter", Reconnects: 2})
	b.cfg.Travel = &TravelMap{gatekeepers: []gatekeeper{{at: Point{X: -9000}}, {at: Point{X: 4000, Y: 100}}}}
	b.world.Apply(PartySmallWindowAll{LeaderID: gmID, Members: []PartyMember{gmRow}}, t0)
	b.lastRule = "follow"
	tests := []struct {
		at        time.Duration
		force     bool
		published bool
	}{
		{0, true, true},
		{500 * time.Millisecond, false, false},
		{time.Second - 1, false, false},
		{time.Second, false, true},
		{1500 * time.Millisecond, false, false},
		{1600 * time.Millisecond, true, true},
		{2500 * time.Millisecond, false, false},
		{2600 * time.Millisecond, false, true},
	}
	for _, tt := range tests {
		prev := l.board.cur.Load()
		b.publishStatus(at(tt.at), tt.force)
		if got := l.board.cur.Load() != prev; got != tt.published {
			t.Fatalf("at %v force %v: published %v, want %v", tt.at, tt.force, got, tt.published)
		}
	}
	st := b.Status()
	s := &b.world.Self
	want := Status{
		At: at(2600 * time.Millisecond), State: StateOnline, Character: s.Name, ClassID: s.ClassID, Level: s.Level,
		HP: 100, MaxHP: 100, MP: 100, MaxMP: 100, Position: s.Position(at(2600 * time.Millisecond)),
		Preset: "fighter", Rule: "follow", Leader: "GM", Members: []string{"GM"},
		Order: Command{Kind: CommandStay}, Reconnects: 2,
	}
	if st.Town == nil || *st.Town != (Point{X: 4000, Y: 100}) {
		t.Fatalf("town %v, want the one at (4000, 100)", st.Town)
	}
	st.Town = nil
	if st.At != want.At || st.State != want.State || st.Character != want.Character || st.Level != want.Level ||
		st.HP != want.HP || st.MaxMP != want.MaxMP || st.Position != want.Position || st.Preset != want.Preset ||
		st.Rule != want.Rule || st.Leader != want.Leader || !slices.Equal(st.Members, want.Members) ||
		st.Order != want.Order || st.Reconnects != want.Reconnects {
		t.Fatalf("status %+v\nwant %+v", st, want)
	}
}

// TestBrainStatusNoTravel checks that a snapshot without a travel map has
// no town, and a brain without a Bot publishes nothing.
func TestBrainStatusNoTravel(t *testing.T) {
	b, _ := newTestBrain(t, Persona{})
	b.publishStatus(t0, true) // no link: no-op
	linkBrain(b, standing{})
	b.publishStatus(t0, true)
	if st := b.Status(); st.Town != nil || st.State != StateOnline {
		t.Fatalf("status %+v, want online with no town", st)
	}
}

// TestBotStatusOffline checks the snapshot between sessions: waiting for
// the gate, entering, reconnecting with why the attempt ended, and stopped
// with why, while another goroutine reads it.
func TestBotStatusOffline(t *testing.T) {
	playFail := &LoginError{Packet: "PlayFail", Reason: 0x0f}
	n := &fakeNet{logins: []error{playFail}, cancelAt: 1}
	b, clock, ctx := startBot(t, true, n)
	if st := b.Status(); st.State != StateNew {
		t.Fatalf("before Run: %v, want new", st.State)
	}
	var atLogin []Status
	n.onLogin = func() { atLogin = append(atLogin, b.Status()) }
	readDone := make(chan struct{})
	stopRead := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			select {
			case <-stopRead:
				return
			default:
				_ = b.Status().State
				runtime.Gosched()
			}
		}
	}()
	errc := make(chan error, 1)
	go func() { errc <- b.Run(ctx) }()
	var reconnecting *Status
	for done := false; !done; {
		select {
		case err := <-errc:
			if err != nil {
				t.Fatalf("Run = %v", err)
			}
			done = true
		case <-clock.armed:
			if st := b.Status(); st.State == StateReconnecting && reconnecting == nil {
				reconnecting = &st
			}
			clock.advanceToNext()
		}
	}
	close(stopRead)
	<-readDone
	if reconnecting == nil || !errors.Is(reconnecting.Err, playFail) || reconnecting.Reconnects != 0 {
		t.Fatalf("reconnecting status %+v, want one after the PlayFail", reconnecting)
	}
	if len(atLogin) != 2 || atLogin[0].State != StateEntering || atLogin[1].State != StateEntering ||
		atLogin[1].Reconnects != 1 || !errors.Is(atLogin[1].Err, playFail) {
		t.Fatalf("statuses at the logins %+v, want entering, the second after a reconnect", atLogin)
	}
	st := b.Status()
	if st.State != StateStopped || !errors.Is(st.Err, ctx.Err()) || st.Reconnects != 1 || st.Character == "" {
		t.Fatalf("status %+v, want stopped by the cancel", st)
	}
}
