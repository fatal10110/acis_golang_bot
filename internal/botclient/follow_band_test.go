package botclient

import (
	"cmp"
	"math"
	"testing"
	"time"
)

// TestFollowBand has a bot follow a leader out of sight at (0, 5000) across
// ground that refuses its legs cell after cell along a band (#141, the
// Talking Island band of acis_golang#5412): legs that reach the band stop
// at its edge, and from there every move is refused in place, sidesteps
// along the band too, except one straight back. Legs aimed straight at the
// slot again only crawl along it; the bot goes around the band in a bounded
// number of moves, or gives up the slot when there is no way around.
func TestFollowBand(t *testing.T) {
	tests := []struct {
		name   string
		ground fakeGround
		// around: the bot gets to its slot within maxMoves moves, else it
		// gives the slot up within maxMoves.
		around   bool
		maxMoves int
		// leader is the leader's Y, 5000 when zero.
		leader int32
		// minDetours: the trip takes more detours than one wall allows.
		minDetours int
	}{
		{name: "600 wide", ground: fakeGround{{-300, 2000, 300, 2400}}, around: true, maxMoves: 20},
		{name: "1200 wide, the near end on the detour side", ground: fakeGround{{-200, 2000, 1000, 2400}}, around: true, maxMoves: 20},
		{name: "1200 wide, the near end on the other side", ground: fakeGround{{-1000, 2000, 200, 2400}}, around: true, maxMoves: 30},
		{name: "2400 wide", ground: fakeGround{{-1200, 2000, 1200, 2400}}, around: true, maxMoves: 40},
		{name: "4800 wide", ground: fakeGround{{-2400, 2000, 2400, 2400}}, around: true, maxMoves: 60},
		{
			name:   "a wall on the detour side: the detour turns to the other",
			ground: fakeGround{{-300, 2000, 300, 2400}, {-500, 1000, -400, 2400}},
			around: true, maxMoves: 25,
		},
		{
			name:   "one wall after another: detours counted per wall",
			ground: fakeGround{{-2400, 2000, 2400, 2400}, {-3600, 5000, 3600, 5400}, {-4800, 8000, 4800, 8400}},
			leader: 12000, around: true, maxMoves: 200, minDetours: followDetours + 1,
		},
		{name: "no way around", ground: fakeGround{{-1e6, 2000, 1e6, 2400}}, around: false, maxMoves: 250},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newFollowBrain(t)
			b.world.Apply(DeleteObject{ObjectID: gmID}, t0)
			leader := cmp.Or(tt.leader, 5000)
			b.world.Apply(PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: gmID, Pos: Point{Y: leader}}}}, t0)
			a, now := repath(t, b, at(time.Second))
			slot := b.follow.dest
			moves, detours := 0, 0
			for a.kind == actMove {
				if moves++; moves > tt.maxMoves {
					t.Fatalf("still on the way after %d moves at %v, slot %v", tt.maxMoves, b.world.Self.Position(now), slot)
				}
				from := b.world.Self.Position(now)
				if b.follow.detouring && b.follow.detourLegs == followDetourLegs-1 && !b.follow.flipped {
					detours++
				}
				if to := tt.ground.walk(from, moveTarget(a.payload)); budged(to, from) {
					b.world.Apply(StopMove{ObjectID: selfID, Pos: to}, now)
				} else {
					echoInPlace(b, from, now)
				}
				now = now.Add(time.Second)
				var name string
				if name, a = b.decide(now); name != "follow" {
					t.Fatalf("decide(%v) = %q, want follow", now.Sub(t0), name)
				}
			}
			t.Logf("%d moves, %d detours", moves, detours)
			if detours < tt.minDetours {
				t.Errorf("%d detours over the trip, want at least %d", detours, tt.minDetours)
			}
			if b.follow.detours > followDetours {
				t.Errorf("%d detours at the last wall, want at most %d", b.follow.detours, followDetours)
			}
			pos := b.world.Self.Position(now)
			if arrived := dist2(pos, slot) <= followArrive*followArrive; arrived != tt.around || b.follow.stuck == tt.around {
				t.Fatalf("at %v, slot %v: arrived %v stuck %v, want arrived %v", pos, slot, arrived, b.follow.stuck, tt.around)
			}
		})
	}
}

// fakeGround is open ground with bands of refused cells, each a rectangle
// {minX, minY, maxX, maxY}.
type fakeGround [][4]float64

// bandFringe is how close to a band a move stops: legs into it end in its
// fringe, and from there only moves clearly away from it are taken.
const bandFringe = 64

// gap is how far (x, y) stands from the nearest band.
func (g fakeGround) gap(x, y float64) float64 {
	d := math.Inf(1)
	for _, r := range g {
		dx := math.Max(math.Max(r[0]-x, 0), x-r[2])
		dy := math.Max(math.Max(r[1]-y, 0), y-r[3])
		d = math.Min(d, math.Hypot(dx, dy))
	}
	return d
}

// walk is where a straight move from from to to ends: one step into the
// first band fringe it closes in on, as the server ends a move at the first wall
// (geo.ValidLocation). From inside a fringe, only a move leaving the band
// behind at half its pace or faster is taken; any other is refused in place,
// ending where it started.
func (g fakeGround) walk(from, to Point) Point {
	const step = 8
	fx, fy := float64(from.X), float64(from.Y)
	l := math.Hypot(float64(to.X)-fx, float64(to.Y)-fy)
	ux, uy := (float64(to.X)-fx)/l, (float64(to.Y)-fy)/l
	if g0 := g.gap(fx, fy); g0 < bandFringe && g.gap(fx+ux*step, fy+uy*step) < g0+step/2 {
		return from
	}
	for s, last := float64(step), g.gap(fx, fy); s < l+step; s += step {
		s = math.Min(s, l)
		x, y := fx+ux*s, fy+uy*s
		gap := g.gap(x, y)
		if gap < bandFringe && gap < last {
			return Point{X: int32(math.Round(x)), Y: int32(math.Round(y)), Z: from.Z}
		}
		last = gap
	}
	return to
}

// TestFollowDetour has a follower whose legs stalled in place at the origin
// sidestep until one sidestep moves it: a detour of followDetourLegs legs of
// followDetourStep follows, followDetourTurn off the line to the slot on the
// side of that sidestep (counterclockwise for straight back), before the legs
// go again. The detour moved the bot off that stall, so the next stall has
// every sidestep again.
func TestFollowDetour(t *testing.T) {
	tests := []struct {
		name    string
		refused int // sidesteps refused before the one that moves the bot
		side    float64
	}{
		{name: "square left moves", refused: 0, side: 1},
		{name: "square right moves", refused: 1, side: -1},
		{name: "straight back moves", refused: 2, side: 1},
		{name: "back right moves", refused: 4, side: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, slot, now := stalledFollower(t)
			for range tt.refused {
				followMove(t, b, now)
				echoInPlace(b, Point{}, now)
				now = now.Add(time.Second)
			}
			pos := moveTarget(followMove(t, b, now).payload)
			now = now.Add(time.Second)
			b.world.Apply(StopMove{ObjectID: selfID, Pos: pos}, now)
			heading := math.Atan2(float64(slot.Y-pos.Y), float64(slot.X-pos.X)) + tt.side*followDetourTurn
			for i := range followDetourLegs {
				to := moveTarget(followMove(t, b, now).payload)
				if l := math.Sqrt(float64(dist2(to, pos))); math.Abs(l-followDetourStep) > 1 {
					t.Errorf("detour leg %d to %v is %.0f long, want %d", i+1, to, l, followDetourStep)
				}
				if off := math.Remainder(math.Atan2(float64(to.Y-pos.Y), float64(to.X-pos.X))-heading, 2*math.Pi); math.Abs(off) > 0.01 {
					t.Errorf("detour leg %d to %v is %.3f rad off its heading", i+1, to, off)
				}
				pos = to
				now = now.Add(time.Second)
				b.world.Apply(StopMove{ObjectID: selfID, Pos: pos}, now)
			}
			leg := legToward(pos, slot, math.Sqrt(float64(dist2(pos, slot))))
			if got := moveTarget(followMove(t, b, now).payload); got != leg {
				t.Fatalf("after the detour: move to %v, want the leg to %v", got, leg)
			}
			for range spotStalls - 1 {
				echoInPlace(b, pos, now)
				now = now.Add(time.Second)
				followMove(t, b, now)
			}
			echoInPlace(b, pos, now)
			checkSidestep(t, followMove(t, b, now.Add(time.Second)), pos, slot, sidestepTurns[0], 0)
		})
	}
}

// refuseDetour has the server refuse in place, at pos, both ways of the
// detour a follower starts once a sidestep got it to pos; it returns the time
// of the next decision, which sends the legs again.
func refuseDetour(t *testing.T, b *Brain, pos Point, now time.Time) time.Time {
	t.Helper()
	for i := range 2 {
		to := moveTarget(followMove(t, b, now).payload)
		if l := math.Sqrt(float64(dist2(to, pos))); math.Abs(l-followDetourStep) > 1 {
			t.Fatalf("detour leg %d to %v is %.0f long, want %d", i+1, to, l, followDetourStep)
		}
		echoInPlace(b, pos, now)
		now = now.Add(time.Second)
	}
	return now
}
