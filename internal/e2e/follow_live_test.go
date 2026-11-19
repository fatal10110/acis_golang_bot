package e2e

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// The follow policy the scenarios assert (botclient defaultFollowPolicy):
// a slot MinDist to MaxDist behind the leader, taken again once the leader
// moved Repath from where the bot took it, after a delay up to DelayMax.
const (
	followMaxDist  = 350
	followRepath   = 250
	followDelayMax = 800 * time.Millisecond
	// followReach is the farthest a settled bot stands from its leader:
	// the leader's stop may land within Repath of the bot's last anchor,
	// where the bot keeps its old slot (#5, the #18 correction).
	followReach = followMaxDist + followRepath
)

// leg is one move of a leader the bot follows: where the leader ended and
// how far the bot fell behind the leader's path on the way.
type leg struct {
	stop botclient.Point
	gap  float64
}

// lead moves leader toward to at speed (world units per second, its walk
// or run) and reads its connection until it should have arrived, sampling
// how far b stands from where the leader is on its path.
func lead(t *testing.T, ctx context.Context, leader *Client, b *botclient.Bot, to botclient.Point, speed int32) leg {
	t.Helper()
	m, err := moveToward(ctx, leader, to)
	if err != nil {
		t.Fatal(err)
	}
	length := dist(m.Origin, m.Dest)
	start := time.Now()
	travel := time.Duration(length / float64(speed) * float64(time.Second))
	var gap float64
	for time.Since(start) < travel+time.Second {
		if err := leader.Watch(ctx, 250*time.Millisecond, func(botclient.Event) {}); err != nil {
			t.Fatal(err)
		}
		f := math.Min(1, time.Since(start).Seconds()*float64(speed)/math.Max(length, 1))
		at := botclient.Point{
			X: m.Origin.X + int32(f*float64(m.Dest.X-m.Origin.X)),
			Y: m.Origin.Y + int32(f*float64(m.Dest.Y-m.Origin.Y)),
		}
		gap = math.Max(gap, dist(b.Status().Position, at))
	}
	return leg{stop: m.Dest, gap: gap}
}

// moveToward moves c toward to or, where geodata refuses that move, as far
// in the next of eight directions around c: the scenarios start wherever
// the last run left the GM, so no one heading is open everywhere.
func moveToward(ctx context.Context, c *Client, to botclient.Point) (botclient.MoveToLocation, error) {
	from := c.Pos()
	d := dist(from, to)
	a := math.Atan2(float64(to.Y-from.Y), float64(to.X-from.X))
	var err error
	for i := range 8 {
		var m botclient.MoveToLocation
		m, err = c.Move(ctx, awayFrom(from, d, a+float64(i)*math.Pi/4))
		if !errors.Is(err, ErrMoveRefused) {
			return m, err
		}
	}
	return botclient.MoveToLocation{}, err
}

// awayFrom is from moved d along the ground at angle a (radians).
func awayFrom(from botclient.Point, d float64, a float64) botclient.Point {
	return botclient.Point{X: from.X + int32(d*math.Cos(a)), Y: from.Y + int32(d*math.Sin(a)), Z: from.Z}
}

// TestLiveFollow has the GM invite a bot from out of its sight, then walk
// and run with it in tow: the bot travels to the GM's last
// PartyMemberPosition until it sees the GM, settles within MaxDist+Repath
// of every stop, keeps within that plus one re-path delay of travel while
// the GM moves, and matches the GM's walk or run (#18).
func TestLiveFollow(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 10*time.Minute)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("fo", 1)
	f := runWatched(t, ctx, tg, account, botSetup{persona: partyPersona})
	b, name := f.bot, f.name
	ui := userInfo(t, gm.Client)

	// Out of sight first: the bot joins the GM on open ground, then the GM
	// runs off in legs until the bot leaves its sight.
	gmToOpenGround(t, ctx, gm)
	botID := recallBot(t, ctx, tg, gm, name)
	if !ui.Running {
		if err := gm.Send(ctx, botclient.EncodeWalkRunToggle()); err != nil {
			t.Fatal(err)
		}
	}
	gone := false
	for heading, legs := 0.0, 0; !gone; legs++ {
		if legs == 12 {
			t.Fatalf("%s still in the GM's sight after %d legs", name, legs)
		}
		from := gm.Pos()
		m, err := gm.Move(ctx, awayFrom(from, 1000, heading))
		if errors.Is(err, ErrMoveRefused) {
			heading += math.Pi / 2 // no path: turn
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if dist(m.Origin, m.Dest) < 300 {
			heading += math.Pi / 2 // blocked: turn
			continue
		}
		travel := time.Duration(dist(m.Origin, m.Dest)/float64(ui.RunSpeed)*float64(time.Second)) + time.Second
		if err := gm.Watch(ctx, travel, func(ev botclient.Event) {
			if d, ok := ev.(botclient.DeleteObject); ok && d.ObjectID == botID {
				gone = true
			}
		}); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("%s out of the GM's sight, %.0f away", name, dist(b.Status().Position, gm.Pos()))

	if r := invite(t, ctx, gm.Client, name); r != 1 {
		t.Fatalf("%s answered %d, want 1", name, r)
	}
	watchUntil(t, ctx, gm.Client, "the bot back in the GM's sight", func() bool { return !gone }, func(ev botclient.Event) {
		if ci, ok := ev.(botclient.CharInfo); ok && ci.ObjectID == botID {
			gone = false
		}
	})
	t.Logf("%s back in the GM's sight at %v", name, b.Status().Position)
	at := settledNear(t, ctx, gm.Client, b, gm.Pos(), name+" after the travel")
	t.Logf("%s settled %.0f from the GM after the travel", name, dist(at, gm.Pos()))

	// Walk, then run.
	for _, walk := range []bool{true, false} {
		if err := gm.Send(ctx, botclient.EncodeWalkRunToggle()); err != nil {
			t.Fatal(err)
		}
		if _, err := gm.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
			ci, ok := ev.(botclient.CharInfo)
			return ok && ci.ObjectID == botID && ci.Running == !walk
		}); err != nil {
			t.Fatalf("%s matches the GM's walk=%v: %v", name, walk, err)
		}
		speed := ui.RunSpeed
		if walk {
			speed = ui.WalkSpeed
		}
		for i := range 2 {
			l := lead(t, ctx, gm.Client, b, awayFrom(gm.Pos(), 700, float64(i)*math.Pi), speed)
			bound := followReach + float64(speed)*followDelayMax.Seconds()*2
			if l.gap > bound {
				t.Fatalf("walk=%v leg %d: %s fell %.0f behind the GM's path, want at most %.0f", walk, i, name, l.gap, bound)
			}
			at := settledNear(t, ctx, gm.Client, b, l.stop, fmt.Sprintf("walk=%v leg %d: %s", walk, i, name))
			d := dist(at, l.stop)
			t.Logf("walk=%v leg %d: at most %.0f behind on the way, settled %.0f from the GM", walk, i, l.gap, d)
		}
	}
}

// followSettle bounds how long a following bot takes to settle near its
// leader's stop: the trip from out of sight waits for the first
// PartyMemberPosition, up to half the server's 12 s report period after the
// party forms, then runs a few thousand units.
const followSettle = time.Minute

// settledNear reads c until b's position stays put for settleFor within
// followReach of stop and returns it; what names the wait in a failure. A
// pause on the way is not the end of the trip: the bot walks a server path
// one segment at a time, and a segment the server is late to start leaves
// it standing at a waypoint (#131). Only a bot that has not settled near
// stop after followSettle fails, with where it stands then.
func settledNear(t *testing.T, ctx context.Context, c *Client, b *botclient.Bot, stop botclient.Point, what string) botclient.Point {
	t.Helper()
	deadline := time.Now().Add(followSettle)
	last, since := b.Status().Position, time.Now()
	for {
		if p := b.Status().Position; p != last {
			last, since = p, time.Now()
		}
		d := dist(last, stop)
		if d <= followReach && time.Since(since) >= settleFor {
			return last
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %.0f from the GM after %v, want settled within %d", what, d, followSettle, followReach)
		}
		if err := c.Watch(ctx, 500*time.Millisecond, func(botclient.Event) {}); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
}
