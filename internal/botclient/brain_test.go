package botclient

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

// countGoal is a persona whose goal counts its turns and sends nothing.
func countGoal(n *int) Persona {
	return Persona{Goal: func(*World, time.Time) (Action, bool) { *n++; return Action{}, true }}
}

func TestBrainTickJitter(t *testing.T) {
	b, _ := newTestBrain(t, Persona{})
	for range 1000 {
		if d := b.tickDelay(); d < defaultTick || d > defaultTick+defaultTickJitter {
			t.Fatalf("tick delay %v outside [%v, %v]", d, defaultTick, defaultTick+defaultTickJitter)
		}
	}
}

// TestBrainReactionDelay checks that a decision acting on news about the
// bot comes 300-1200 ms after it, never sooner, and that later news folds
// into the same decision.
func TestBrainReactionDelay(t *testing.T) {
	for seed := range uint64(200) {
		var turns int
		b, _ := newTestBrain(t, countGoal(&turns))
		b.rng = rand.New(rand.NewPCG(seed, seed))
		b.nextTick = at(time.Hour)
		ev := at(100 * time.Millisecond)
		if err := b.handle([]Event{StatusUpdate{ObjectID: selfID, Set: StatusHP, HP: 50}}, ev); err != nil {
			t.Fatal(err)
		}
		react := b.reactAt
		if d := react.Sub(ev); d < defaultReactMin || d > defaultReactMax {
			t.Fatalf("seed %d: reaction delay %v outside [%v, %v]", seed, d, defaultReactMin, defaultReactMax)
		}
		if err := b.handle([]Event{MyTargetSelected{ObjectID: npcID}}, ev.Add(200*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		if b.reactAt != react {
			t.Fatalf("seed %d: later news moved the reaction from %v to %v", seed, react, b.reactAt)
		}
		if err := b.wake(react.Add(-time.Millisecond)); err != nil || turns != 0 {
			t.Fatalf("seed %d: decided before the reaction delay (turns %d, err %v)", seed, turns, err)
		}
		if err := b.wake(react); err != nil || turns != 1 {
			t.Fatalf("seed %d: no decision at the reaction time (turns %d, err %v)", seed, turns, err)
		}
		if !b.reactAt.IsZero() {
			t.Fatalf("seed %d: reaction still armed after the decision", seed)
		}
	}
}

func TestBrainIgnoresOthersNews(t *testing.T) {
	b, _ := newTestBrain(t, Persona{})
	evs := []Event{
		MoveToLocation{ObjectID: npcID, Dest: Point{X: 5}},
		StatusUpdate{ObjectID: npcID, Set: StatusHP, HP: 1},
		AutoAttackStart{ObjectID: pcID},
		CreatureSay{},
	}
	if err := b.handle(evs, at(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if !b.reactAt.IsZero() {
		t.Fatal("news about other objects armed a reaction")
	}
}

func TestBrainLogout(t *testing.T) {
	type step struct {
		at     time.Duration
		events []Event // handled at at; nil wakes the brain instead
	}
	tests := []struct {
		name    string
		steps   []step
		wantErr error
		// wantLogouts is how many Logout frames went out.
		wantLogouts int
	}{
		{"LeaveWorld ends the loop", []step{{100 * time.Millisecond, []Event{LeaveWorld{}}}}, errLoggedOut, 1},
		{"ServerClose ends the session", []step{{100 * time.Millisecond, []Event{ServerClose{}}}}, errServerClose, 1},
		{"no answer times out", []step{{defaultLogoutTimeout, nil}}, errLogoutTimeout, 1},
		{"no answer before the deadline waits", []step{{defaultLogoutTimeout - time.Millisecond, nil}}, nil, 1},
		{"refusal waits out the grace, then retries", []step{
			{100 * time.Millisecond, []Event{ActionFailed{}}},
			{100*time.Millisecond + logoutRefusalGrace, nil},
			{100*time.Millisecond + logoutRefusalGrace + logoutRetry - time.Millisecond, nil},
			{100*time.Millisecond + logoutRefusalGrace + logoutRetry, nil},
		}, nil, 2},
		{"LeaveWorld within the refusal grace still ends the loop", []step{
			{100 * time.Millisecond, []Event{ActionFailed{}}},
			{900 * time.Millisecond, []Event{LeaveWorld{}}},
		}, errLoggedOut, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, out := newTestBrain(t, Persona{})
			b.requestLogout(t0)
			if err := b.wake(t0); err != nil {
				t.Fatal(err)
			}
			var err error
			for _, s := range tc.steps {
				if s.events != nil {
					err = b.handle(s.events, at(s.at))
				} else {
					err = b.wake(at(s.at))
				}
				if err != nil {
					break
				}
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			logouts := 0
			for _, f := range out.frames {
				if f[0] == opGameLogout {
					logouts++
				}
			}
			if logouts != tc.wantLogouts {
				t.Fatalf("%d Logout frames, want %d", logouts, tc.wantLogouts)
			}
		})
	}
}

// TestBrainPlaysWhileLogoutRefused checks that a refused logout hands the
// turn back to the rules until the retry, and that none run while a Logout
// waits for its answer.
func TestBrainPlaysWhileLogoutRefused(t *testing.T) {
	var turns int
	b, _ := newTestBrain(t, countGoal(&turns))
	b.requestLogout(t0)
	_ = b.wake(t0)
	if err := b.wake(at(900 * time.Millisecond)); err != nil || turns != 0 {
		t.Fatalf("decided while Logout waited for its answer (turns %d, err %v)", turns, err)
	}
	_ = b.handle([]Event{ActionFailed{}}, at(950*time.Millisecond))
	for d := 2 * time.Second; d < 10*time.Second; d += time.Second {
		if err := b.wake(at(d)); err != nil {
			t.Fatal(err)
		}
	}
	if turns == 0 {
		t.Fatal("no decision while the refused logout waited to retry")
	}
}

func TestBrainUnaskedLeaveWorld(t *testing.T) {
	b, _ := newTestBrain(t, Persona{})
	if err := b.handle([]Event{LeaveWorld{}}, at(time.Second)); !errors.Is(err, errLeftWorld) {
		t.Fatalf("err = %v, want %v", err, errLeftWorld)
	}
}

// TestBrainEngagedResets checks that a new target, an unselect or the end
// of combat lets the bot attack again.
func TestBrainEngagedResets(t *testing.T) {
	tests := []struct {
		name string
		ev   Event
		want int32
	}{
		{"same target kept", MyTargetSelected{ObjectID: npcID}, npcID},
		{"new target", MyTargetSelected{ObjectID: pcID}, 0},
		{"unselected", TargetUnselected{ObjectID: selfID}, 0},
		{"combat over", AutoAttackStop{ObjectID: selfID}, 0},
		{"someone else's combat over", AutoAttackStop{ObjectID: npcID}, npcID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := newTestBrain(t, Persona{})
			b.engaged = npcID
			_ = b.handle([]Event{tc.ev}, at(time.Second))
			if b.engaged != tc.want {
				t.Fatalf("engaged = %#x, want %#x", b.engaged, tc.want)
			}
		})
	}
}

// TestBrainSoak runs an hour of simulated play with a goal that wants to
// send on every 5 ms decision and news about the bot every 7 ms, and checks
// every frame that went out against the server's limits.
func TestBrainSoak(t *testing.T) {
	var cur time.Time
	var frames []time.Time
	var emotes, says []time.Time
	out := senderFunc(func(p []byte) error {
		frames = append(frames, cur)
		switch p[0] {
		case opGameRequestSocialAction:
			emotes = append(emotes, cur)
		case opGameSay2:
			says = append(says, cur)
		}
		return nil
	})
	i := 0
	goal := func(*World, time.Time) (Action, bool) {
		i++
		switch i % 3 {
		case 0:
			return emoteAction(2), true
		case 1:
			return sayAction("hello"), true
		}
		return moveAction(Point{X: int32(i % 100)}, Point{}), true
	}
	w := newTestWorld(t)
	b := NewBrain(w, NewEvents(64), out, newFakeClock(t0), BrainConfig{
		Persona: Persona{Goal: goal}, Tick: 5 * time.Millisecond, TickJitter: time.Millisecond,
		ReactMin: time.Millisecond, ReactMax: 2 * time.Millisecond, Rand: rand.New(rand.NewPCG(3, 4)),
	})
	b.start(t0)
	end, nextEvent := at(time.Hour), t0
	for cur = t0; cur.Before(end); {
		if due := b.due(); due.Before(nextEvent) {
			cur = due
			if err := b.wake(cur); err != nil {
				t.Fatal(err)
			}
			continue
		}
		cur = nextEvent
		if err := b.handle([]Event{StatusUpdate{ObjectID: selfID, Set: StatusHP, HP: 15}}, cur); err != nil {
			t.Fatal(err)
		}
		nextEvent = cur.Add(7 * time.Millisecond)
	}
	if len(frames) < frameRate*3000 {
		t.Fatalf("only %d frames in an hour: the soak did not saturate the pacer", len(frames))
	}
	assertUnderServerLimits(t, frames)
	if gap := minGap(emotes); gap < serverSocialGap {
		t.Errorf("emotes %v apart, server gate is %v", gap, serverSocialGap)
	}
	if gap := minGap(says); gap < chatGap {
		t.Errorf("chat lines %v apart, want at least %v", gap, chatGap)
	}
}

// TestBrainKeepalive runs two simulated hours of a store bot that decides
// nothing and checks that it never stays silent near the server's 15 minute
// idle timeout.
func TestBrainKeepalive(t *testing.T) {
	b, out := newTestBrain(t, Persona{Store: true})
	var sends []time.Time
	for now := t0; now.Before(at(2 * time.Hour)); now = b.due() {
		before := len(out.frames)
		if err := b.wake(now); err != nil {
			t.Fatal(err)
		}
		if len(out.frames) > before {
			sends = append(sends, now)
		}
	}
	if len(sends) < 2 {
		t.Fatalf("%d keepalives in two hours", len(sends))
	}
	for _, f := range out.frames {
		if f[0] != opGameValidatePosition {
			t.Fatalf("store bot sent opcode %#x, want only ValidatePosition", f[0])
		}
	}
	prev := t0
	for _, s := range sends {
		if gap := s.Sub(prev); gap > keepaliveAfter+defaultTick+defaultTickJitter || gap >= 15*time.Minute {
			t.Fatalf("silent for %v", gap)
		}
		prev = s
	}
}

func TestBrainRun(t *testing.T) {
	t.Run("returns on done", func(t *testing.T) {
		b, done, _, errc := runTestBrain(t, Persona{})
		_ = b
		close(done)
		if err := <-errc; err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	})
	t.Run("logs out inside the loop", func(t *testing.T) {
		b, _, logout, errc := runTestBrain(t, Persona{})
		close(logout)
		if f := <-b.out.(chanSender); f[0] != opGameLogout {
			t.Fatalf("sent opcode %#x, want Logout", f[0])
		}
		b.events.push(LeaveWorld{}, nil)
		if err := <-errc; err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	})
	t.Run("ends on ServerClose", func(t *testing.T) {
		b, _, _, errc := runTestBrain(t, Persona{})
		b.events.push(ServerClose{}, nil)
		if err := <-errc; !errors.Is(err, errServerClose) {
			t.Fatalf("Run = %v, want %v", err, errServerClose)
		}
	})
	t.Run("decides on its tick", func(t *testing.T) {
		goal := func(*World, time.Time) (Action, bool) { return emoteAction(7), true }
		b, done, _, errc := runTestBrain(t, Persona{Goal: goal})
		clock := b.clock.(*fakeClock)
		<-clock.armed
		clock.Advance(defaultTick + defaultTickJitter)
		if f := <-b.out.(chanSender); f[0] != opGameRequestSocialAction {
			t.Fatalf("sent opcode %#x, want RequestSocialAction", f[0])
		}
		close(done)
		if err := <-errc; err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	})
}

// runTestBrain starts Run over newTestWorld with a fake clock at t0 and a
// sender that hands each payload to the test.
func runTestBrain(t *testing.T, p Persona) (b *Brain, done, logout chan struct{}, errc chan error) {
	t.Helper()
	b = NewBrain(newTestWorld(t), NewEvents(64), make(chanSender, 16), newFakeClock(t0), BrainConfig{Persona: p, Rand: rand.New(rand.NewPCG(5, 6))})
	done, logout, errc = make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() { errc <- b.Run(done, logout) }()
	return b, done, logout, errc
}

type chanSender chan []byte

func (c chanSender) Send(p []byte) error {
	c <- slices.Clone(p)
	return nil
}

type senderFunc func([]byte) error

func (f senderFunc) Send(p []byte) error { return f(p) }

func minGap(times []time.Time) time.Duration {
	gap := time.Duration(1<<63 - 1)
	for i := 1; i < len(times); i++ {
		gap = min(gap, times[i].Sub(times[i-1]))
	}
	return gap
}
