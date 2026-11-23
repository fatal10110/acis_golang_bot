package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// TestSelfArrival checks the arrival model of the bot's own move against
// the server's arrival timer (creature.go travelTicksLocked,
// moveToLocationLocked, retimeLocked): whole 100 ms updates, the first five
// of a move from standstill at the walk speed on land, one more update per
// change between land and water, the anchor kept by a retarget and reset by a
// move received at the modeled end, and the re-time of a resample. The test
// self runs at 100 and walks at 50 from the origin; water lies from x 500 to
// 1500, where it swims at 50.
func TestSelfArrival(t *testing.T) {
	type step struct {
		at time.Duration
		ev Event
	}
	move := func(origin, dest Point) MoveToLocation {
		return MoveToLocation{ObjectID: selfID, Origin: origin, Dest: dest}
	}
	speeds := func(run, walk int32, running bool, pos Point) UserInfo {
		return UserInfo{ObjectID: selfID, Name: "bot7", RunSpeed: run, WalkSpeed: walk, SwimSpeed: 50, Running: running, Pos: pos}
	}
	tests := []struct {
		name   string
		water  bool
		steps  []step
		want   time.Duration
		moving time.Duration
	}{
		{
			// 5 updates at 5 units, then 475 at 10: 5 + 48.
			name:  "running on land from standstill",
			steps: []step{{0, move(Point{}, Point{X: 500})}},
			want:  5300 * time.Millisecond,
		},
		{
			name:  "whole updates without rounding error",
			steps: []step{{0, move(Point{}, Point{X: 525})}},
			want:  5500 * time.Millisecond,
		},
		{
			name:  "short move within the start updates",
			steps: []step{{0, move(Point{}, Point{X: 20})}},
			want:  400 * time.Millisecond,
		},
		{
			name: "walk mode: start speed is the speed",
			steps: []step{
				{0, speeds(100, 50, false, Point{})},
				{0, move(Point{}, Point{X: 500})},
			},
			want: 10 * time.Second,
		},
		{
			// The estimate (the origin) is 500 from dest, the server's origin
			// 650: 5 + 63.
			name:  "the larger distance is timed",
			steps: []step{{0, move(Point{X: -150}, Point{X: 500})}},
			want:  6800 * time.Millisecond,
		},
		{
			name: "blocked correction stands at its receipt",
			steps: []step{
				{0, move(Point{}, Point{X: 500})},
				{time.Second, move(Point{X: 90}, Point{X: 90})},
			},
			want: time.Second,
		},
		{
			// From the estimate at 200 (20 updates done), 300 at 10.
			name: "retarget keeps the anchor",
			steps: []step{
				{0, move(Point{}, Point{X: 1000})},
				{2 * time.Second, move(Point{X: 200}, Point{X: 500})},
			},
			want: 5 * time.Second,
		},
		{
			// Within positionTick + A of the end: 460 left, 5 + 44.
			name: "attack click at the modeled end starts afresh",
			steps: []step{
				{0, move(Point{}, Point{X: 500})},
				{5200 * time.Millisecond, MoveToPawn{ObjectID: selfID, TargetID: npcID, Distance: 40, Origin: Point{X: 500}}},
			},
			want: 10100 * time.Millisecond,
		},
		{
			name: "talk click before the end retargets",
			steps: []step{
				{0, move(Point{}, Point{X: 500})},
				{4 * time.Second, MoveToPawn{ObjectID: selfID, TargetID: npcID, Distance: 40, Origin: Point{X: 400}}},
			},
			want: 9600 * time.Millisecond,
		},
		{
			// From 200 at 2 s, 800 at 20: 2 + 4.
			name: "faster run speed re-times earlier",
			steps: []step{
				{0, move(Point{}, Point{X: 1000})},
				{2 * time.Second, speeds(200, 50, true, Point{X: 200})},
			},
			want: 6 * time.Second,
		},
		{
			// From 200 at 2 s, 800 at 5: 2 + 16.
			name: "slower speed re-times later",
			steps: []step{
				{0, move(Point{}, Point{X: 1000})},
				{2 * time.Second, speeds(50, 50, true, Point{X: 200})},
			},
			want: 18 * time.Second,
		},
		{
			// Snapped back to -50: 1050 left at 2 s.
			name: "ValidateLocation behind moves the end later",
			steps: []step{
				{0, move(Point{}, Point{X: 1000})},
				{2 * time.Second, ValidateLocation{ObjectID: selfID, Pos: Point{X: -50}}},
			},
			want: 12500 * time.Millisecond,
		},
		{
			name: "ValidateLocation ahead keeps the end",
			steps: []step{
				{0, move(Point{}, Point{X: 1000})},
				{2 * time.Second, ValidateLocation{ObjectID: selfID, Pos: Point{X: 450}}},
			},
			// 5 + 98 from the start.
			want: 10300 * time.Millisecond,
		},
		{
			// 500 dry (52.5 updates), 500 swum at 5, one change: 153 + 1.
			name:  "wades in",
			water: true,
			steps: []step{{0, move(Point{}, Point{X: 1000})}},
			want:  15400 * time.Millisecond,
		},
		{
			// 52.5 + 200 swum + 50 dry, two changes: 303 + 2.
			name:  "wades in and out",
			water: true,
			steps: []step{{0, move(Point{}, Point{X: 2000})}},
			want:  30500 * time.Millisecond,
		},
		{
			// In water the start speed is the swim speed: 800 at 5.
			name:  "swims",
			water: true,
			steps: []step{
				{0, StopMove{ObjectID: selfID, Pos: Point{X: 600}}},
				{0, move(Point{X: 600}, Point{X: 1400})},
			},
			want: 16 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorld(t)
			if tt.water {
				w.SetWater(nav.NewWater([]nav.WaterBox{{X1: 500, Y1: -100, X2: 1500, Y2: 100, MinZ: -200, MaxZ: 50}}))
				w.Apply(speeds(100, 50, true, Point{}), t0)
			}
			for _, s := range tt.steps {
				w.Apply(s.ev, at(s.at))
			}
			end := w.Self.arrival()
			if got := end.Sub(t0); got != tt.want {
				t.Fatalf("arrival at %v, want %v", got, tt.want)
			}
			if !w.Self.Moving(end.Add(-time.Nanosecond)) && tt.want > tt.steps[len(tt.steps)-1].at {
				t.Errorf("not moving just before the arrival")
			}
			if w.Self.Moving(end) {
				t.Errorf("still moving at the arrival")
			}
		})
	}
}

// TestSelfApproachKeepsSwim: the bot's own MoveToPawn keeps its swim, which
// a fresh motion literal dropped.
func TestSelfApproachKeepsSwim(t *testing.T) {
	w := newTestWorld(t)
	w.SetWater(nav.NewWater([]nav.WaterBox{{X1: 500, Y1: -100, X2: 1500, Y2: 100, MinZ: -200, MaxZ: 50}}))
	w.Apply(UserInfo{ObjectID: selfID, Name: "bot7", RunSpeed: 100, WalkSpeed: 50, SwimSpeed: 50, Running: true}, t0)
	w.Apply(MoveToPawn{ObjectID: selfID, TargetID: npcID, Distance: 40}, t0)
	if !w.Self.move.wet() {
		t.Fatal("the approach through water does not swim")
	}
	// 500 dry (52.5 updates), 460 swum at 5, one change: 145 + 1.
	if got := w.Self.arrival().Sub(t0); got != 14600*time.Millisecond {
		t.Errorf("arrival at %v, want 14.6s", got)
	}
}
