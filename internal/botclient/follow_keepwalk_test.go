package botclient

import (
	"testing"
	"time"
)

// followRun wakes b at each decision it schedules from now, answering each
// walker segment at once as the server answers an open click, until done
// reports true after a decision. sent sees each segment at its send time,
// with the modeled end of the move before it. It returns the time of the
// last decision.
func followRun(t *testing.T, b *Brain, now time.Time, sent func(seg segment, prevEnd time.Time), done func() bool) time.Time {
	t.Helper()
	for range 600 {
		prevEnd := b.world.Self.arrival()
		if err := b.wake(now); err != nil {
			t.Fatal(err)
		}
		if seg := b.walk.seg; seg.sentAt.Equal(now) {
			sent(seg, prevEnd)
			b.handleOne(t, segAnswer(seg.from, seg.to), now)
		}
		if done() {
			return now
		}
		now = b.decisionAt()
	}
	t.Fatal("the follow did not settle after 600 decisions")
	return now
}

// arrivedAtSlot reports whether b stands at its follow slot with no re-path
// pending.
func arrivedAtSlot(b *Brain) bool {
	v := &b.follow
	return v.anchored && !v.traveling && v.dueAt.IsZero() && b.walk.status == walkArrived
}

// TestFollowKeepsWalkingThroughDelay: the leader moving Repath on while the
// bot walks to its slot does not stop the bot at the end of its segment for
// the follow delay. Each segment goes out one position tick past the end of
// the one before, during the delay too, and the walk turns to the new slot
// once the delay ends (#306). The leader moves from the start, so the walk
// takes segments of at most movingSegment while it counts as moving (#344).
func TestFollowKeepsWalkingThroughDelay(t *testing.T) {
	b := newFollowBrain(t)
	const delay = 8 * time.Second
	b.persona.Follow.DelayMin, b.persona.Follow.DelayMax = delay, delay
	b.world.Apply(MoveToLocation{ObjectID: gmID, Origin: followGM(0, 0), Dest: followGM(2000, 0)}, t0)
	now := at(time.Second)
	b.nextTick, b.reactAt = now, time.Time{}

	var sends, pending, capped int
	var movedAt time.Time
	followRun(t, b, now, func(seg segment, prevEnd time.Time) {
		// Once the leader stood still for followStill, the walk takes long
		// segments again (walkFollowStill).
		if b.walk.purpose == walkFollow {
			if capped++; horizontal2(seg.from, seg.to) > movingSegment*movingSegment {
				t.Fatalf("segment %d %+v -> %+v longer than %d", sends, seg.from, seg.to, movingSegment)
			}
		}
		if sends > 0 {
			if idle := seg.sentAt.Sub(prevEnd); idle > positionTick {
				t.Fatalf("segment %d at %v: idle %v after the last one ended (re-path at %v), want at most %v",
					sends, seg.sentAt.Sub(t0), idle, b.follow.dueAt.Sub(t0), positionTick)
			}
		}
		if !movedAt.IsZero() && b.follow.dueAt.After(seg.sentAt) {
			pending++
		}
		if sends++; sends == 2 {
			movedAt = seg.sentAt
			b.world.Apply(StopMove{ObjectID: gmID, Pos: followGM(2300, 0)}, movedAt)
		}
	}, func() bool { return !movedAt.IsZero() && arrivedAtSlot(b) })

	if pending == 0 || capped == 0 {
		t.Fatalf("%d of %d segments went out during the follow delay and %d toward the leader moving, want some of each", pending, sends, capped)
	}
	if lead := followGM(2300, 0); b.follow.anchor != lead {
		t.Fatalf("slot drawn for the leader at %+v, want at %+v", b.follow.anchor, lead)
	}
}

// TestFollowAtSlotWaitsDelay: a bot standing at its slot still waits the
// follow delay after the leader moves Repath on before it moves (#306).
func TestFollowAtSlotWaitsDelay(t *testing.T) {
	b := newFollowBrain(t)
	const delay = 2 * time.Second
	b.persona.Follow.DelayMin, b.persona.Follow.DelayMax = delay, delay
	now := at(time.Second)
	b.nextTick, b.reactAt = now, time.Time{}
	now = followRun(t, b, now, func(segment, time.Time) {}, func() bool { return arrivedAtSlot(b) })

	movedAt := now.Add(time.Millisecond)
	b.world.Apply(StopMove{ObjectID: gmID, Pos: followGM(300, 0)}, movedAt)
	var first time.Time
	followRun(t, b, b.decisionAt(), func(seg segment, _ time.Time) {
		if first.IsZero() {
			first = seg.sentAt
		}
	}, func() bool { return !first.IsZero() })
	if first.Before(movedAt.Add(delay)) {
		t.Fatalf("moved %v after the leader, want the %v delay first", first.Sub(movedAt), delay)
	}
}

// TestWalkSegmentCap: a walk toward a destination that moves (following a
// leader that moves, melee, flee) takes no segment longer than
// movingSegment; a walk to a fixed place, or following a leader that stands
// still, keeps segments up to navSegment (#306, #344).
func TestWalkSegmentCap(t *testing.T) {
	dest := Point{X: 4000}
	tests := []struct {
		purpose walkPurpose
		// long is set when some segment must be longer than movingSegment.
		long bool
	}{
		{walkFollow, false},
		{walkFollowStill, true},
		{walkMelee, false},
		{walkFlee, false},
		{walkPlace, true},
	}
	for _, tt := range tests {
		b, _, _ := newNavBrain(t, testNav(t), Persona{}, Point{})
		now, longest := t0, int64(0)
		for range 60 {
			a, st := b.goTo(dest, 50, tt.purpose, now)
			if st != walkWalking {
				if st != walkArrived {
					t.Fatalf("purpose %d: walk ended %v (%s)", tt.purpose, st, b.walk.reason)
				}
				break
			}
			if a.kind != actMove {
				now = now.Add(positionTick)
				continue
			}
			if ok, err := b.send(a, now); !ok || err != nil {
				t.Fatalf("purpose %d: send: %v, %v", tt.purpose, ok, err)
			}
			origin, to := moveOrigin(a.payload), moveTarget(a.payload)
			b.handleOne(t, segAnswer(origin, to), now)
			longest = max(longest, horizontal2(origin, to))
			now = now.Add(segRun(b, now) + time.Millisecond)
		}
		if b.walk.status != walkArrived {
			t.Fatalf("purpose %d: walk %v after 60 decisions, want arrived", tt.purpose, b.walk.status)
		}
		if long := longest > movingSegment*movingSegment; long != tt.long {
			t.Errorf("purpose %d: longest segment² %d, over %d² = %v, want %v", tt.purpose, longest, movingSegment, long, tt.long)
		}
	}
}
