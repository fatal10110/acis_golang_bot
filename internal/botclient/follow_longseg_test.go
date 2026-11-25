package botclient

import (
	"testing"
	"time"
)

// TestFollowStill: a leader counts as standing still once it was neither
// seen moving nor found away from where it was at the last decision for
// followStill in sight, followReportStill out of sight; a leader never seen
// moving stands still from the first decision (#344).
func TestFollowStill(t *testing.T) {
	away := func(b *Brain) { delete(b.world.Objects, gmID) }
	report := func(b *Brain, p Point, now time.Time) {
		b.world.Apply(PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: gmID, Pos: p}}}, now)
	}
	type step struct {
		at   time.Duration
		do   func(b *Brain, now time.Time)
		want bool
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{"in sight, never moved", []step{{at: time.Second, want: true}, {at: 10 * time.Second, want: true}}},
		{"in sight, moving", []step{
			{at: time.Second, do: func(b *Brain, now time.Time) {
				b.world.Apply(MoveToLocation{ObjectID: gmID, Origin: followGM(0, 0), Dest: followGM(0, 1500)}, now)
			}, want: false},
			{at: 10 * time.Second, want: false},
		}},
		{"in sight, stopped", []step{
			{at: time.Second, do: func(b *Brain, now time.Time) {
				b.world.Apply(MoveToLocation{ObjectID: gmID, Origin: followGM(0, 0), Dest: followGM(0, 1500)}, now)
			}, want: false},
			{at: 2 * time.Second, do: func(b *Brain, now time.Time) {
				b.world.Apply(StopMove{ObjectID: gmID, Pos: followGM(0, 100)}, now)
			}, want: false},
			{at: 4*time.Second + 999*time.Millisecond, want: false},
			{at: 5 * time.Second, want: true},
		}},
		{"in sight, stepped aside between decisions", []step{
			{at: time.Second, want: true},
			{at: 2 * time.Second, do: func(b *Brain, now time.Time) {
				b.world.Apply(StopMove{ObjectID: gmID, Pos: followGM(300, 0)}, now)
			}, want: false},
			{at: 5 * time.Second, want: true},
		}},
		{"in sight, a nudge within walkSameDest", []step{
			{at: time.Second, want: true},
			{at: 2 * time.Second, do: func(b *Brain, now time.Time) {
				b.world.Apply(StopMove{ObjectID: gmID, Pos: followGM(walkSameDest, 0)}, now)
			}, want: true},
		}},
		{"out of sight, first report", []step{
			{at: time.Second, do: func(b *Brain, now time.Time) { away(b); report(b, Point{X: 7000, Y: followY}, now) }, want: true},
		}},
		{"out of sight, a report that moved", []step{
			{at: time.Second, do: func(b *Brain, now time.Time) { away(b); report(b, Point{X: 7000, Y: followY}, now) }, want: true},
			{at: 13 * time.Second, do: func(b *Brain, now time.Time) { report(b, Point{X: 7500, Y: followY}, now) }, want: false},
			{at: 25 * time.Second, do: func(b *Brain, now time.Time) { report(b, Point{X: 7500, Y: followY}, now) }, want: false},
			{at: 26 * time.Second, want: true},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newFollowBrain(t)
			v := &b.follow
			for _, s := range tt.steps {
				now := at(s.at)
				if s.do != nil {
					s.do(b, now)
				}
				lead, ok := b.leader(gmID, now)
				if !ok {
					t.Fatalf("at %v: no leader", s.at)
				}
				if got := v.still(lead, now); got != s.want {
					t.Fatalf("at %v: still = %v, want %v (moved at %v)", s.at, got, s.want, v.movedAt.Sub(t0))
				}
			}
		})
	}
}

// TestFollowLongSegments: a follow walk toward a leader that stands still
// takes segments longer than movingSegment, one toward a leader that moves
// none (#344).
func TestFollowLongSegments(t *testing.T) {
	tests := []struct {
		name   string
		moving bool
	}{
		{"leader standing still", false},
		{"leader moving", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newFollowBrain(t)
			if tt.moving {
				// 15 s at the GM's run speed of 100: moving through the walk.
				b.world.Apply(MoveToLocation{ObjectID: gmID, Origin: followGM(0, 0), Dest: followGM(0, 1500)}, t0)
			}
			now := at(time.Second)
			b.nextTick, b.reactAt = now, time.Time{}
			var sends int
			var longest int64
			followRun(t, b, now, func(seg segment, _ time.Time) {
				sends++
				longest = max(longest, horizontal2(seg.from, seg.to))
				want := walkFollowStill
				if tt.moving {
					want = walkFollow
				}
				if b.walk.purpose != want {
					t.Fatalf("segment %d: walk purpose %d, want %d", sends, b.walk.purpose, want)
				}
			}, func() bool { return sends >= 3 || arrivedAtSlot(b) })
			if long := longest > movingSegment*movingSegment; long == tt.moving {
				t.Fatalf("longest segment² %d over %d² = %v after %d segments, want %v", longest, movingSegment, long, sends, !tt.moving)
			}
		})
	}
}

// TestFollowLeaderStartsMoving: a leader that starts moving while the bot
// walks a long segment toward it resets the walk: the segment in flight
// runs out, and the walk goes on from its end with segments of at most
// movingSegment (#344).
func TestFollowLeaderStartsMoving(t *testing.T) {
	b := newFollowBrain(t)
	// 3700 to the slot: a route of more than one long segment. The GM runs
	// at 10, so its move below lasts past the bot's first segment.
	b.world.Apply(CharInfo{ObjectID: gmID, Name: "GM", Pos: followGM(2000, 0), RunSpeed: 10, WalkSpeed: 5, Running: true}, t0)
	now := at(time.Second)
	b.nextTick, b.reactAt = now, time.Time{}
	var segs []segment
	var ends []time.Time
	followRun(t, b, now, func(seg segment, prevEnd time.Time) {
		segs, ends = append(segs, seg), append(ends, prevEnd)
		if len(segs) == 1 {
			b.world.Apply(MoveToLocation{ObjectID: gmID, Origin: followGM(2000, 0), Dest: followGM(2000, 1500)}, seg.sentAt)
		}
	}, func() bool { return len(segs) >= 2 })

	first, second := segs[0], segs[1]
	if horizontal2(first.from, first.to) <= movingSegment*movingSegment {
		t.Fatalf("first segment %+v -> %+v: not longer than %d toward a leader standing still", first.from, first.to, movingSegment)
	}
	if second.sentAt.Before(ends[1]) {
		t.Fatalf("second segment sent at %v, before the first ended at %v", second.sentAt.Sub(t0), ends[1].Sub(t0))
	}
	if horizontal2(second.from, first.to) > walkSlop*walkSlop {
		t.Fatalf("second segment from %+v, want the first's end %+v", second.from, first.to)
	}
	if d := horizontal2(second.from, second.to); d > movingSegment*movingSegment {
		t.Fatalf("second segment %+v -> %+v longer than %d toward a leader moving", second.from, second.to, movingSegment)
	}
	if b.walk.purpose != walkFollow {
		t.Fatalf("walk purpose %d, want walkFollow", b.walk.purpose)
	}
}
