package e2e

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveNavFacts measures two server facts the walker rests on (#152):
// Emax, the longest time one position update of a running player covers,
// and that a direct walk's MoveToLocation echoes the clicked X and Y.
//
// The server sends no per-update position. It answers a ValidatePosition
// that drifts more than the run speed from its own position with a
// ValidateLocation carrying that position, which only the position updates
// move (network/movement.go validateLivePlayerPosition, move/creature.go
// Position). On a straight run that position is the move's origin plus the
// run speed times the time from the move's start to the last update
// (playerPassed, no cap), so a reply's age, the time since the last update,
// is the time since the move started less distance/speed, up to a constant
// per leg (the start speed of the first updates, the network delay). Polled
// at random phases, the age runs from about 0, a poll just after an update,
// to about the longest gap between updates: max minus min age per leg
// estimates Emax, give or take the network jitter between two polls. The
// polls are spread at random phases of the updates.
//
// The polls go through the client's pacer (5 frames a second), far inside
// the server's 160 a second; the reported position sits two run speeds off
// the leg's line, at its floor height, so it is corrected and never taken.
// navLegA and navLegB end a 2000-long straight leg on flat open ground
// south-west of openGround, open both ways in the bot's geodata (CanMove
// from every eighth of the way to either end), so each click is a direct
// walk.
var (
	navLegA = botclient.Point{X: -74338, Y: 255271, Z: -3216}
	navLegB = botclient.Point{X: -72338, Y: 255271, Z: -3272}
)

func TestLiveNavFacts(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 10*time.Minute)
	gm := enterGM(t, ctx, tg)
	ui := userInfo(t, gm.Client)
	if err := gm.Command(ctx, fmt.Sprintf("teleport %d %d %d", navLegA.X, navLegA.Y, navLegA.Z)); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, teleportOf(gm.Self.ObjectID)); err != nil {
		t.Fatalf("GM teleported to the leg's end: %v", err)
	}
	if !ui.Running {
		if err := gm.Send(ctx, botclient.EncodeWalkRunToggle()); err != nil {
			t.Fatal(err)
		}
	}
	speed := float64(max(ui.RunSpeed, ui.WalkSpeed, 1))
	const (
		wantLegs = 6
		// warmup skips the first updates of a leg, run at the start speed.
		warmup = 700 * time.Millisecond
	)
	own := func(ev botclient.Event) bool {
		v, ok := ev.(botclient.ValidateLocation)
		return ok && v.ObjectID == gm.Self.ObjectID
	}
	var ages []time.Duration
	echoed, tried := 0, 0
	for echoed < wantLegs {
		if tried++; tried > 2*wantLegs {
			t.Fatalf("only %d direct legs in %d tries", echoed, tried-1)
		}
		from := gm.Pos()
		to := navLegB
		if tried%2 == 0 {
			to = navLegA
		}
		heading := math.Atan2(float64(to.Y-from.Y), float64(to.X-from.X))
		sent := time.Now()
		m, err := gm.Move(ctx, to)
		if errors.Is(err, ErrMoveRefused) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if m.Dest.X != to.X || m.Dest.Y != to.Y {
			// The leg is open both ways in geodata: a destination other
			// than the click is a broken echo, or geodata that changed.
			t.Errorf("leg %d: destination %v for the click %v on a direct walk", tried, m.Dest, to)
			continue
		}
		echoed++
		t.Logf("leg %d: direct walk from %v, destination %v echoes the click %v (Z +%d)", tried, m.Origin, m.Dest, to, m.Dest.Z-to.Z)
		length := dist(m.Origin, m.Dest)
		off := 2*speed + 100
		report := botclient.Point{
			X: m.Origin.X + int32(-off*math.Sin(heading)),
			Y: m.Origin.Y + int32(off*math.Cos(heading)),
			Z: from.Z,
		}
		var ts, ds []float64 // seconds since the click, distance from the origin
		for {
			// A random delay spreads the polls over the update phase: the
			// pacer alone would send them every 200 ms, locked to the 100 ms
			// updates, and every reply would show the same age.
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(rand.N(100 * time.Millisecond)):
			}
			poll := time.Now()
			if err := gm.Send(ctx, botclient.EncodeValidatePosition(report, 0)); err != nil {
				t.Fatal(err)
			}
			ev, err := gm.Await(ctx, EntryTimeout, own)
			if err != nil {
				t.Fatalf("ValidateLocation for a report %0.f off: %v", off, err)
			}
			at := ev.(botclient.ValidateLocation).Pos
			d := math.Hypot(float64(at.X-m.Origin.X), float64(at.Y-m.Origin.Y))
			if at.X == m.Dest.X && at.Y == m.Dest.Y || d >= length-speed/5 {
				break
			}
			if elapsed := poll.Sub(sent); elapsed >= warmup {
				ts = append(ts, elapsed.Seconds())
				ds = append(ds, d)
			}
		}
		if len(ts) < 10 {
			continue
		}
		// The speed is fitted, not taken from UserInfo: the age, uniform
		// over the gaps, does not depend on the time, so the slope of
		// distance over time is the speed the server steps with.
		v := slope(ts, ds)
		leg := make([]time.Duration, len(ts))
		for i := range ts {
			leg[i] = time.Duration((ts[i] - ds[i]/v) * float64(time.Second))
		}
		low := slices.Min(leg)
		for _, a := range leg {
			ages = append(ages, a-low)
		}
		t.Logf("leg %d: %d polls, fitted speed %.1f (UserInfo run %d, walk %d)", tried, len(ts), v, ui.RunSpeed, ui.WalkSpeed)
	}
	if len(ages) < 50 {
		t.Fatalf("%d update ages, want at least 50", len(ages))
	}
	slices.Sort(ages)
	q := func(p float64) time.Duration { return ages[int(p*float64(len(ages)-1))] }
	t.Logf("Emax estimate %v over %d polls on %d direct legs at UserInfo run speed %v: age p1 %v, p50 %v, p90 %v, p99 %v, max %v",
		ages[len(ages)-1].Round(time.Millisecond), len(ages), echoed, ui.RunSpeed,
		q(0.01).Round(time.Millisecond), q(0.5).Round(time.Millisecond), q(0.9).Round(time.Millisecond), q(0.99).Round(time.Millisecond), ages[len(ages)-1].Round(time.Millisecond))
}

// slope is the least-squares slope of ys over xs.
func slope(xs, ys []float64) float64 {
	var mx, my float64
	for i := range xs {
		mx += xs[i]
		my += ys[i]
	}
	n := float64(len(xs))
	mx, my = mx/n, my/n
	var sxy, sxx float64
	for i := range xs {
		sxy += (xs[i] - mx) * (ys[i] - my)
		sxx += (xs[i] - mx) * (xs[i] - mx)
	}
	return sxy / sxx
}
