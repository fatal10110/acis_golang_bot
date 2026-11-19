package botclient

import (
	"math"
	"testing"
	"time"
)

// newFollowBrain is a started brain whose bot stands at the origin, running,
// in gmID's party; the GM stands at (2000, 0) facing +X, running.
func newFollowBrain(t *testing.T) *Brain {
	t.Helper()
	b, _ := newTestBrain(t, Persona{Follow: defaultFollowPolicy()})
	b.world.Apply(PartySmallWindowAll{LeaderID: gmID, Members: []PartyMember{gmRow}}, t0)
	b.world.Apply(CharInfo{ObjectID: gmID, Name: "GM", Pos: Point{X: 2000}, RunSpeed: 100, WalkSpeed: 50, Running: true}, t0)
	return b
}

// followHold decides at now and wants rule "follow" holding the turn.
func followHold(t *testing.T, b *Brain, now time.Time) {
	t.Helper()
	if name, a := b.decide(now); name != "follow" || a.kind != actHold {
		t.Fatalf("decide(%v) = %q kind %d, want follow holding", now.Sub(t0), name, a.kind)
	}
}

// repath decides at now, which must notice the leader moved and schedule
// the re-path DelayMin to DelayMax later, waking the brain then; it holds
// until just before, and returns the decision at the due time.
func repath(t *testing.T, b *Brain, now time.Time) (Action, time.Time) {
	t.Helper()
	b.follow.wake = false
	followHold(t, b, now)
	due := b.follow.dueAt
	if d := due.Sub(now); d < 200*time.Millisecond || d > 800*time.Millisecond {
		t.Fatalf("re-path due %v after noticing, want 200-800 ms", d)
	}
	if !b.follow.wake {
		t.Fatalf("re-path at %v without its wake-up", due.Sub(t0))
	}
	followHold(t, b, due.Add(-time.Millisecond))
	name, a := b.decide(due)
	if name != "follow" {
		t.Fatalf("decide at due = %q, want follow", name)
	}
	return a, due
}

// checkSlot fails unless a moves to a point MinDist-MaxDist from leader and
// within followArc of straight behind it, away from direction back.
func checkSlot(t *testing.T, a Action, leader Point, back float64) Point {
	t.Helper()
	if a.kind != actMove {
		t.Fatalf("kind %d, want a move", a.kind)
	}
	to := moveTarget(a.payload)
	d := math.Sqrt(float64(dist2(to, leader)))
	if d < 149.5 || d > 350.5 {
		t.Errorf("slot %v is %.0f from leader %v, want 150-350", to, d, leader)
	}
	off := math.Remainder(math.Atan2(float64(to.Y-leader.Y), float64(to.X-leader.X))-back, 2*math.Pi)
	if math.Abs(off) > followArc+0.01 {
		t.Errorf("slot %v is %.2f rad off behind the leader, want within %.2f", to, off, followArc)
	}
	if to.Z != leader.Z {
		t.Errorf("slot z = %d, want the leader's %d", to.Z, leader.Z)
	}
	return to
}

func TestFollowEngages(t *testing.T) {
	tests := []struct {
		name  string
		setup func(b *Brain)
		want  bool
	}{
		{"party leader in sight", func(*Brain) {}, true},
		{"no party", func(b *Brain) { b.world.Apply(PartySmallWindowDeleteAll{}, t0) }, false},
		{"bot leads", func(b *Brain) {
			b.world.Apply(PartySmallWindowAll{LeaderID: selfID, Members: []PartyMember{gmRow}}, t0)
		}, false},
		{"persona does not follow", func(b *Brain) { b.persona.Follow = FollowPolicy{} }, false},
		{"resting comes first", func(b *Brain) {
			b.persona.SitHP, b.persona.StandHP, b.world.Self.HP = 40, 90, 10
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newFollowBrain(t)
			tt.setup(b)
			if name, _ := b.decide(at(time.Second)); (name == "follow") != tt.want {
				t.Errorf("decide = %q, want follow %v", name, tt.want)
			}
		})
	}
}

// TestFollowRepathCadence walks the slot through a moving leader on the
// fake clock: the first slot after the delay, nothing while the leader stays
// within Repath of the anchor, a re-path once it is past it.
func TestFollowRepathCadence(t *testing.T) {
	b := newFollowBrain(t)
	gm := Point{X: 2000}
	a, due := repath(t, b, at(time.Second))
	slot := checkSlot(t, a, gm, math.Pi) // facing +X: behind is -X
	r := math.Sqrt(float64(dist2(slot, gm)))

	// Arrived; the leader steps 200 units on: inside Repath, nothing.
	b.world.Apply(StopMove{ObjectID: selfID, Pos: slot}, due)
	gm = Point{X: 2200}
	b.world.Apply(StopMove{ObjectID: gmID, Pos: gm}, at(3*time.Second))
	b.follow.wake = false
	followHold(t, b, at(4*time.Second))
	if !b.follow.dueAt.IsZero() || b.follow.wake {
		t.Fatalf("re-path scheduled with the leader 200 units from the anchor")
	}

	// 300 units from the anchor: re-path, behind the way it went (+X).
	gm = Point{X: 2300}
	b.world.Apply(StopMove{ObjectID: gmID, Pos: gm}, at(5*time.Second))
	a, _ = repath(t, b, at(6*time.Second))
	slot = checkSlot(t, a, gm, math.Pi)
	if got := math.Sqrt(float64(dist2(slot, gm))); math.Abs(got-r) > 1 {
		t.Errorf("slot radius %.0f, want the drawn %.0f kept for the same leader", got, r)
	}
}

func TestFollowSlotBehindMovingLeader(t *testing.T) {
	b := newFollowBrain(t)
	b.world.Apply(MoveToLocation{ObjectID: gmID, Origin: Point{X: 2000}, Dest: Point{X: 2000, Y: 3000}}, at(time.Second))
	a, due := repath(t, b, at(time.Second))
	checkSlot(t, a, b.world.Objects[gmID].Position(due), -math.Pi/2) // heading +Y: behind is -Y
}

func TestFollowOutOfSight(t *testing.T) {
	b := newFollowBrain(t)
	b.world.Apply(DeleteObject{ObjectID: gmID}, t0)
	followHold(t, b, at(time.Second))
	if !b.follow.dueAt.IsZero() {
		t.Fatal("re-path scheduled before any leader position")
	}

	b.world.Apply(PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: gmID, Pos: Point{X: 10000, Z: -50}}}}, at(6*time.Second))
	a, due := repath(t, b, at(7*time.Second))
	if a.kind != actMove {
		t.Fatalf("kind %d, want a move", a.kind)
	}
	slot, leg := b.follow.dest, moveTarget(a.payload)
	whole := math.Sqrt(float64(dist2(slot, Point{})))
	if d, rest := math.Sqrt(float64(dist2(leg, Point{}))), math.Sqrt(float64(dist2(slot, leg))); math.Abs(d-spotLeg) > 2 || math.Abs(d+rest-whole) > 2 {
		t.Fatalf("first leg to %v, want spotLeg along the way to the slot %v", leg, slot)
	}
	if d := math.Sqrt(float64(dist2(slot, Point{X: 10000}))); d < 149.5 || d > 350.5 || slot.X >= 10000 {
		t.Errorf("slot %v, want 150-350 short of the report on the bot's side", slot)
	}

	// The leg ends: the next one goes at once. Then legs that get nowhere,
	// and the sidesteps after them (TestFollowUnstick), give the slot up
	// until the leader moves on.
	stop := Point{X: spotLeg}
	b.world.Apply(StopMove{ObjectID: selfID, Pos: stop}, due.Add(time.Second))
	now := due.Add(2 * time.Second)
	for i := range spotStalls + len(sidestepTurns) {
		name, a := b.decide(now)
		if name != "follow" || a.kind != actMove {
			t.Fatalf("leg %d: decide = %q kind %d, want follow moving", i+2, name, a.kind)
		}
		now = now.Add(time.Second)
	}
	followHold(t, b, now)
	b.follow.wake = false
	followHold(t, b, now.Add(time.Second))
	if !b.follow.dueAt.IsZero() {
		t.Fatal("a stuck bot scheduled a re-path while the leader stayed put")
	}
	b.world.Apply(PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: gmID, Pos: Point{X: 10000, Y: 1000}}}}, now)
	if a, _ := repath(t, b, now.Add(2*time.Second)); a.kind != actMove {
		t.Errorf("after the leader moved on: kind %d, want a move", a.kind)
	}
}

func TestFollowDrift(t *testing.T) {
	b := newFollowBrain(t)
	a, due := repath(t, b, at(time.Second))
	slot := checkSlot(t, a, Point{X: 2000}, math.Pi)
	b.world.Apply(StopMove{ObjectID: selfID, Pos: slot}, due)
	followHold(t, b, due.Add(time.Second))

	// A fight carried the bot off: farther than MaxDist+Repath, it walks back.
	b.world.Apply(StopMove{ObjectID: selfID, Pos: Point{X: 2000, Y: -700}}, due.Add(2*time.Second))
	a, _ = repath(t, b, due.Add(3*time.Second))
	checkSlot(t, a, Point{X: 2000}, math.Pi)
}

func TestFollowStandsUp(t *testing.T) {
	b := newFollowBrain(t)
	b.world.Self.Sitting = true
	a, _ := repath(t, b, at(time.Second))
	if a.kind != actSitStand {
		t.Fatalf("kind %d, want the stand toggle before moving", a.kind)
	}
}

func TestFollowMatchesWalkRun(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(b *Brain)
		selfRunning bool
		want        bool
	}{
		{"both run", func(*Brain) {}, true, false},
		{"leader walks", func(b *Brain) { b.world.Objects[gmID].Running = false }, true, true},
		{"leader runs", func(*Brain) {}, false, true},
		{"leader out of sight: run", func(b *Brain) {
			b.world.Apply(DeleteObject{ObjectID: gmID}, t0)
			b.world.Apply(PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: gmID, Pos: Point{X: 9000}}}}, t0)
		}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newFollowBrain(t)
			tt.setup(b)
			b.world.Self.Running = tt.selfRunning
			now := at(time.Second)
			name, a := b.decide(now)
			if got := name == "follow" && a.kind == actWalkRun; got != tt.want {
				t.Fatalf("decide = %q kind %d, want toggle %v", name, a.kind, tt.want)
			}
			if !tt.want {
				return
			}
			if sent, err := b.send(a, now); !sent || err != nil {
				t.Fatalf("send = %v, %v", sent, err)
			}
			// Unanswered, the toggle is not sent again before it settles.
			if _, a := b.decide(now.Add(toggleSettle - time.Millisecond)); a.kind == actWalkRun {
				t.Error("toggled again before the first settled")
			}
			if _, a := b.decide(now.Add(toggleSettle)); a.kind != actWalkRun {
				t.Errorf("kind %d after toggleSettle with no answer, want the toggle again", a.kind)
			}
		})
	}
}
