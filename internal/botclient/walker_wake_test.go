package botclient

import (
	"math"
	"testing"
	"time"
)

// wakeAnswerLag is how long after a segment goes out the tests' server
// answers it.
const wakeAnswerLag = 50 * time.Millisecond

// wakeWalk is a brain on the open flat ground east of the origin whose walk
// to dest sent its first segment at t0, answered by answer at
// t0+wakeAnswerLag; the regular tick is a minute away.
func wakeWalk(t *testing.T, dest Point, answer func(origin, to Point) MoveToLocation) (*Brain, segment) {
	t.Helper()
	b, _, _ := newNavBrain(t, testNav(t), Persona{}, Point{})
	a, st := walkStep(t, b, dest, t0)
	if st != walkWalking || a.kind != actMove {
		t.Fatalf("first decision: status %v kind %v (%s), want a move", st, a.kind, b.walk.reason)
	}
	// The wake at the segment's end: the click-ahead's earlier wake is
	// TestClickAheadAtCheckedCorner's.
	b.walk.seg.noAhead = true
	seg := b.walk.seg
	b.handleOne(t, answer(moveOrigin(a.payload), moveTarget(a.payload)), t0.Add(wakeAnswerLag))
	b.nextTick, b.reactAt = t0.Add(time.Minute), time.Time{}
	return b, seg
}

// landTicks is the server's update count for a run of dist units from
// standstill at the test speeds (run 100, walk 50).
func landTicks(dist float64) time.Duration {
	if dist <= 25 {
		return time.Duration(math.Ceil(dist / 5))
	}
	return 5 + time.Duration(math.Ceil((dist-25)/10))
}

// TestWalkWake checks the walk wake (docs/walking.md, "Walk wake"): the
// decision after a segment comes one position tick past the modeled end of
// the bot's own move, moves with that end, fires once, survives a decision a
// higher rule takes mid-segment, and covers a reset, a desync and a blocked
// stop; a walk the counters end stops waking.
func TestWalkWake(t *testing.T) {
	dest := Point{X: 1000}
	answer := segAnswer
	tests := []struct {
		name   string
		answer func(origin, to Point) MoveToLocation
		// act plays the case on b, whose segment seg was answered; it returns
		// when the next decision is due.
		act func(t *testing.T, b *Brain, seg segment) time.Time
	}{
		{
			name:   "answered segment: the end plus a tick, not the next tick",
			answer: answer,
			act: func(t *testing.T, b *Brain, seg segment) time.Time {
				end := t0.Add(wakeAnswerLag + landTicks(math.Sqrt(float64(horizontal2(seg.from, seg.to))))*positionTick)
				if got := b.world.Self.arrival(); got != end {
					t.Fatalf("arrival at %v, want %v", got.Sub(t0), end.Sub(t0))
				}
				due := end.Add(positionTick)
				if a, _ := walkStep(t, b, dest, due.Add(-time.Millisecond)); a.kind != actHold {
					t.Fatalf("%v sent before the end plus a tick", a.kind)
				}
				return due
			},
		},
		{
			name:   "slower speed mid-walk moves the wake later",
			answer: answer,
			act: func(t *testing.T, b *Brain, _ segment) time.Time {
				before := b.world.Self.arrival()
				now := at(500 * time.Millisecond)
				b.handleOne(t, UserInfo{
					ObjectID: selfID, Name: "bot7", RunSpeed: 80, WalkSpeed: 50, Running: true,
					HP: 100, MaxHP: 100, Pos: b.world.Self.Position(now),
				}, now)
				if !b.world.Self.arrival().After(before) {
					t.Fatalf("arrival %v, want later than %v", b.world.Self.arrival().Sub(t0), before.Sub(t0))
				}
				return b.world.Self.arrival().Add(positionTick)
			},
		},
		{
			name:   "ValidateLocation behind moves the wake later",
			answer: answer,
			act: func(t *testing.T, b *Brain, _ segment) time.Time {
				before := b.world.Self.arrival()
				b.handleOne(t, ValidateLocation{ObjectID: selfID, Pos: Point{X: -250}}, at(500*time.Millisecond))
				if !b.world.Self.arrival().After(before) {
					t.Fatalf("arrival %v, want later than %v", b.world.Self.arrival().Sub(t0), before.Sub(t0))
				}
				return b.world.Self.arrival().Add(positionTick)
			},
		},
		{
			name:   "held decision spends the wake and sleeps until the tick",
			answer: answer,
			act: func(t *testing.T, b *Brain, _ segment) time.Time {
				due := b.world.Self.arrival().Add(positionTick)
				if got := b.decisionAt(); got != due {
					t.Fatalf("decision at %v, want the wake at %v", got.Sub(t0), due.Sub(t0))
				}
				if err := b.wake(due); err != nil {
					t.Fatal(err)
				}
				return b.nextTick
			},
		},
		{
			name:   "salient self StatusUpdate keeps the arrival wake",
			answer: answer,
			act: func(t *testing.T, b *Brain, _ segment) time.Time {
				b.handleOne(t, StatusUpdate{ObjectID: selfID, Set: StatusHP, HP: 60}, at(200*time.Millisecond))
				react := b.decisionAt()
				if !react.Before(b.world.Self.arrival()) {
					t.Fatalf("reaction at %v, want before the arrival", react.Sub(t0))
				}
				if err := b.wake(react); err != nil {
					t.Fatal(err)
				}
				b.nextTick = t0.Add(time.Minute)
				return b.world.Self.arrival().Add(positionTick)
			},
		},
		{
			name:   "reset mid-segment wakes at that segment's end",
			answer: answer,
			act: func(t *testing.T, b *Brain, seg segment) time.Time {
				other := Point{X: 2000}
				b.walk.asked = false
				if a, _ := walkStep(t, b, other, at(time.Second)); a.kind != actHold {
					t.Fatalf("%v sent mid-segment", a.kind)
				}
				due := b.world.Self.arrival().Add(positionTick)
				a, _ := walkStep(t, b, other, due)
				if a.kind != actMove || horizontal2(moveOrigin(a.payload), seg.to) > walkSlop*walkSlop {
					t.Fatalf("at the wake: %v from %+v, want the next segment from %+v", a.kind, moveOrigin(a.payload), seg.to)
				}
				return b.world.Self.arrival().Add(positionTick)
			},
		},
		{
			name: "desync holds past the end of the server's move, then replans",
			answer: func(origin, _ Point) MoveToLocation {
				return MoveToLocation{ObjectID: selfID, Origin: origin, Dest: Point{X: 600, Z: defaultCollisionHeight}}
			},
			act: func(t *testing.T, b *Brain, _ segment) time.Time {
				end := t0.Add(wakeAnswerLag + landTicks(600)*positionTick)
				if got := b.world.Self.arrival(); got != end {
					t.Fatalf("arrival at %v, want the server's move's end %v", got.Sub(t0), end.Sub(t0))
				}
				// The hold (#501): a next leg of the server's route may still
				// come, so no decision before the end plus a tick plus A.
				due := end.Add(positionTick + b.world.Self.lag())
				if got := b.decisionAt(); got != due {
					t.Fatalf("decision at %v, want the hold's end %v", got.Sub(t0), due.Sub(t0))
				}
				if a, _ := walkStep(t, b, dest, due.Add(-time.Millisecond)); a.kind != actHold {
					t.Fatalf("%v sent before the hold's end", a.kind)
				}
				if a, _ := walkStep(t, b, dest, due); a.kind != actMove {
					t.Fatalf("at the wake: %v, want the replan's segment", a.kind)
				}
				return b.world.Self.arrival().Add(positionTick)
			},
		},
		{
			name: "blocked stop replans one tick later; the counter ends the walk",
			answer: func(_, _ Point) MoveToLocation {
				return MoveToLocation{ObjectID: selfID, Origin: Point{X: 300}, Dest: Point{X: 300}}
			},
			act: func(t *testing.T, b *Brain, _ segment) time.Time {
				now := t0.Add(wakeAnswerLag)
				for i := 1; i < walkMaxBlocked; i++ {
					due := now.Add(positionTick)
					if got := b.decisionAt(); got != due {
						t.Fatalf("blocked stop %d: decision at %v, want %v", i, got.Sub(t0), due.Sub(t0))
					}
					a, st := walkStep(t, b, dest, due)
					if st != walkWalking || a.kind != actMove {
						t.Fatalf("blocked stop %d: status %v kind %v, want the replan's segment", i, st, a.kind)
					}
					now = due.Add(wakeAnswerLag)
					stop := b.world.Self.Position(due)
					b.handleOne(t, MoveToLocation{ObjectID: selfID, Origin: stop, Dest: stop}, now)
				}
				if b.walk.status != walkUnreachable || b.walk.reason != reasonBlocked {
					t.Fatalf("after %d blocked stops: status %v (%s), want unreachable blocked", walkMaxBlocked, b.walk.status, b.walk.reason)
				}
				return b.nextTick
			},
		},
		{
			name:   "paced Appearing does not spin",
			answer: answer,
			act: func(t *testing.T, b *Brain, _ segment) time.Time {
				due := b.world.Self.arrival().Add(positionTick)
				b.appearing = true
				b.pace.refill(due)
				b.pace.tokens = 0
				if err := b.wake(due); err != nil {
					t.Fatal(err)
				}
				if !b.appearing {
					t.Fatal("Appearing went out with no pacer token")
				}
				return due.Add(time.Second / frameRate)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, seg := wakeWalk(t, dest, tt.answer)
			want := tt.act(t, b, seg)
			if got := b.decisionAt(); got != want {
				t.Fatalf("next decision at %v, want %v", got.Sub(t0), want.Sub(t0))
			}
		})
	}
}

// TestWalkWakeZero: a walk wake never fired (zero wokeAt) offers the end of
// a move long over once, and the decision it runs spends it.
func TestWalkWakeZero(t *testing.T) {
	b, _, _ := newNavBrain(t, testNav(t), Persona{}, Point{})
	now := at(time.Second)
	b.nextTick, b.reactAt = now.Add(time.Minute), time.Time{}
	b.walk.asked = true
	if got := b.decisionAt(); got.After(now) {
		t.Fatalf("decision at %v, want the unfired wake by %v", got.Sub(t0), now.Sub(t0))
	}
	if err := b.wake(now); err != nil {
		t.Fatal(err)
	}
	b.walk.asked = true
	if got := b.decisionAt(); got != b.nextTick {
		t.Fatalf("decision at %v after the extra one, want the tick at %v", got.Sub(t0), b.nextTick.Sub(t0))
	}
}
