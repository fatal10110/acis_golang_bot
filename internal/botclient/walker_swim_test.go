package botclient

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// swimLake cuts the open ground of row 0 across X 5000..6000, inside its
// second cluster, its floor at height 0 inside the band: the ground east of
// it has no abstract node.
var swimLake = nav.WaterBox{X1: 5000, Y1: -100, X2: 6000, Y2: 5000, MinZ: -50, MaxZ: 50}

// TestWalkerSwims walks across the lake, into it after a leader, and out of
// it: the hierarchy keeps out of water and holds no ground in it, so the
// walker takes the swimming cell planner's route, each segment passing its
// checks, the wet ones timed at the swim speed, with no desync and no
// blocked stop. Without a swim speed the far shore is unreachable, and so
// is a place in the lake for any walk but a follower's: no move is sent.
func TestWalkerSwims(t *testing.T) {
	n, err := buildTestNavWater(geotest.Scenes(), navWindow(0, 0, 1, 0), nav.NewWater([]nav.WaterBox{swimLake}))
	if err != nil {
		t.Fatal(err)
	}
	shore, lake, far := Point{X: 3500, Y: 500}, Point{X: 5500, Y: 500}, Point{X: 6500, Y: 500}
	for _, tc := range []struct {
		name        string
		start, dest Point
		purpose     walkPurpose
		swim        int32
		arrive      bool
	}{
		{"across the lake", shore, far, walkPlace, 50, true},
		{"into the lake after a leader", shore, lake, walkFollow, 50, true},
		{"out of the lake", lake, shore, walkPlace, 50, true},
		{"across without a swim speed", shore, far, walkPlace, 0, false},
		{"a place in the lake", shore, lake, walkPlace, 50, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _, reg := newNavBrain(t, n, Persona{}, tc.start)
			b.world.Self.SwimSpeed = tc.swim
			if !tc.arrive {
				// The bot would drown in the lake, or cannot swim at all.
				if a, st := b.goTo(tc.dest, 0, tc.purpose, t0); st != walkUnreachable || a.kind == actMove {
					t.Fatalf("status %v kind %v (%s), want unreachable and no move", st, a.kind, b.walk.reason)
				}
				return
			}
			now, prev, swum := t0, Point{}, false
			var segs int
			for i := 0; ; i++ {
				if i > 60 {
					t.Fatal("no arrival after 60 decisions")
				}
				a, st := b.goTo(tc.dest, 0, tc.purpose, now)
				if a.kind != actHold {
					if ok, err := b.send(a, now); !ok || err != nil {
						t.Fatalf("send %v: sent %v, %v", a.kind, ok, err)
					}
				}
				if st == walkArrived {
					break
				}
				if st != walkWalking || a.kind != actMove {
					t.Fatalf("decision %d: status %v kind %v (%s), want a move", i, st, a.kind, b.walk.reason)
				}
				origin, to := moveOrigin(a.payload), moveTarget(a.payload)
				if !b.walk.segmentOK(origin, to) {
					t.Fatalf("segment %d %+v -> %+v fails the walker's checks", segs, origin, to)
				}
				segs++
				ans := segAnswer(origin, to)
				b.handle([]Event{ans}, now)
				run := b.world.Self.legTime(ans.Origin, ans.Dest)
				if t0, t1, ok := n.Water.Span(toLoc(origin), toLoc(to)); ok && t1 > t0 {
					swum = true
					if dry := runFor(origin, to) - positionTick; run <= dry {
						t.Errorf("segment %d in water takes %v, want longer than the %v it runs dry", segs, run, dry)
					}
				}
				now, prev = now.Add(run+positionTick+time.Millisecond), to
			}
			if !swum {
				t.Error("the walk never swam")
			}
			if horizontal2(prev, tc.dest) > walkSlop*walkSlop {
				t.Errorf("ended at %+v, want %+v", prev, tc.dest)
			}
			if s := reg.Collect(now); s.NavBlocked+s.NavDesync != 0 {
				t.Errorf("blocked %d desync %d, want none", s.NavBlocked, s.NavDesync)
			}
		})
	}
}

// TestCheckedStepKeepsDry checks that a flee or kite step never ends in
// water, though a bot with a swim speed may swim on a walk: a bot left
// standing in water drowns. From the shore, the step into the lake is
// skipped and the step back onto dry ground taken.
func TestCheckedStepKeepsDry(t *testing.T) {
	n, err := buildTestNavWater(geotest.Scenes(), navWindow(0, 0, 1, 0), nav.NewWater([]nav.WaterBox{swimLake}))
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := newNavBrain(t, n, Persona{}, Point{X: 4900, Y: 500})
	b.world.Self.SwimSpeed = 50
	if a := b.checkedStep(300, t0, 0); a.kind != 0 {
		t.Errorf("step into the lake: kind %v to %+v, want none", a.kind, moveTarget(a.payload))
	}
	if a := b.checkedStep(300, t0, 0, math.Pi); a.kind != actMove || moveTarget(a.payload).X >= 4900 {
		t.Errorf("step away from the lake: kind %v, want a move west", a.kind)
	}
}

// TestMotionSwims checks the bot's own dead reckoning through water: the
// dry part at the run speed in the plane, the wet part at the swim speed in
// 3D, and the height on the swim line rather than the floor.
func TestMotionSwims(t *testing.T) {
	water := nav.NewWater([]nav.WaterBox{{X1: 500, Y1: -100, X2: 1500, Y2: 100, MinZ: -200, MaxZ: 50}})
	tests := []struct {
		name     string
		from, to Point
		swim     int32
		total    time.Duration
		at       time.Duration
		want     Point
	}{
		{"dry", Point{}, Point{X: 1000}, 0, 10 * time.Second, 5 * time.Second, Point{X: 500}},
		{"wades in", Point{}, Point{X: 1000}, 50, 15 * time.Second, 7500 * time.Millisecond, Point{X: 625}},
		{"swims down", Point{X: 600}, Point{X: 600, Z: -150}, 50, 3 * time.Second, time.Second, Point{X: 600, Z: -50}},
	}
	for _, tt := range tests {
		m := motion{speed: 100, swim: &swimLine{water: water, speed: tt.swim}}.withLine(tt.from, tt.to, t0)
		if got := m.duration(); got != tt.total {
			t.Errorf("%s: duration %v, want %v", tt.name, got, tt.total)
		}
		if got := m.at(t0.Add(tt.at)); got != tt.want {
			t.Errorf("%s: at %v = %+v, want %+v", tt.name, tt.at, got, tt.want)
		}
	}
}
