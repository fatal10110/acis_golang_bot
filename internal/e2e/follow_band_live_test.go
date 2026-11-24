package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// On Talking Island, the way from the village to the human fighters' start
// crosses a band around x -71700..-70600, y 252800..253400 where the
// server's pathfinder refuses ground moves cell after cell (acis_golang#5412),
// and past it ledges drop onto the ground below. The ground below sea level
// there (Z under -3779) lies in the region's water zone (WaterZone.xml
// 17_25_water1), where the server makes a character swim and the bot does
// not walk (#161, #223): the old bandApproach (-74167,250518,-3975) and #147's
// low ground (-72050,251600,-4632) are in it. bandWest, on dry ground west of
// the band, and beyondBand, about 1500 past it where the server reported a
// follower on its way (#141), are not; ledgeLeader is the recoverGround of
// #128, 3300 farther on over the ledges, the leader #147 asks a follower to
// reach.
var (
	bandWest    = botclient.Point{X: -72139, Y: 253199, Z: -3533}
	beyondBand  = botclient.Point{X: -70459, Y: 254943, Z: -3533}
	ledgeLeader = botclient.Point{X: -70744, Y: 258264, Z: -3112}
)

// TestLiveFollowAcrossBand has a bot in the GM's party at bandWest follow
// the GM to beyondBand, out of sight, across the band on a route it plans on
// its own geodata, never the server's pathfinder (#141, #159).
func TestLiveFollowAcrossBand(t *testing.T) {
	t.Parallel()
	followAcross(t, "fb", bandWest, beyondBand)
}

// TestLiveFollowLedge has a bot at beyondBand follow the GM to ledgeLeader,
// out of sight, over the ledges between: its route takes the ramps, and
// never walks off a ledge it would have to climb back (#147).
func TestLiveFollowLedge(t *testing.T) {
	t.Parallel()
	followAcross(t, "fl", beyondBand, ledgeLeader)
}

// followAcross has a bot of account prefix prefix recalled to the GM at
// from, then invited into the GM's party once the GM teleported to to, out
// of its sight: it follows the GM there within 8 minutes. The GM teleports
// before the invite, since a leader seen teleporting away is waited for
// where the bot stands (rule "leader", #24). runWatched asserts the walk
// desynced from the server nowhere.
func followAcross(t *testing.T, prefix string, from, to botclient.Point) {
	t.Helper()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 10*time.Minute)
	gm := enterGM(t, ctx, tg)
	gmTeleport(t, ctx, gm, from)
	account, _ := tg.Account(prefix, 1)
	f := runWatched(t, ctx, tg, account, botSetup{persona: partyPersona})
	b, name := f.bot, f.name
	recallBot(t, ctx, tg, gm, name)
	botLanded(t, ctx, gm, b, gm.Pos())
	if d := dist(b.Status().Position, to); d <= followReach {
		t.Fatalf("%s at %v, %.0f from %v before the trip; want farther than %d", name, b.Status().Position, d, to, followReach)
	}
	gmTeleport(t, ctx, gm, to)

	at, seen := b.Status().Position, f.watch.Len()
	if r := invite(t, ctx, gm.Client, name); r != 1 {
		t.Fatalf("%s answered %d, want 1", name, r)
	}
	start := time.Now()
	t.Cleanup(func() { logRoute(t, f, seen, start) })
	t.Logf("%s at %v, %.0f from the GM at %v", name, at, dist(at, gm.Pos()), gm.Pos())
	watchUntilBy(t, ctx, gm.Client, "the bot at the GM", time.Now().Add(8*time.Minute), func() bool {
		return dist(b.Status().Position, gm.Pos()) <= followReach
	}, nil)
	t.Logf("%s at the GM after %v", name, time.Since(start).Round(time.Second))
	if n, at, _ := f.echoes(seen); n > 0 {
		t.Logf("%s: %d ground moves refused in place on the way, first at %v", name, n, at)
	}
}

// botLanded reads gm until b's own session has it within followReach of
// at, where the GM recalled it. recallBot returns once the GM sees the bot,
// before the bot's session has applied the teleport, and a bot that logged
// out where the last run left it still stands there until then.
func botLanded(t *testing.T, ctx context.Context, gm *GMClient, b *botclient.Bot, at botclient.Point) {
	t.Helper()
	wctx, stop := context.WithTimeout(ctx, EntryTimeout)
	defer stop()
	watchUntil(t, wctx, gm.Client, "the bot landed at the GM", func() bool {
		return dist(b.Status().Position, at) <= followReach
	}, nil)
}

// logRoute logs where the server moved f since index from, one line per
// half minute of the trip: where its last move in it ended, and how many of
// its moves the server refused in place.
func logRoute(t *testing.T, f *farmer, from int, start time.Time) {
	var at botclient.Point
	refused, next := 0, start.Add(30*time.Second)
	flush := func(now time.Time) {
		t.Logf("%s %v: at %v, %d moves refused in place", f.name, now.Sub(start).Round(time.Second), at, refused)
		refused = 0
	}
	for _, s := range f.watch.Seen(from) {
		m, ok := s.Event.(botclient.MoveToLocation)
		if !ok || m.ObjectID != f.id {
			continue
		}
		for !s.At.Before(next) {
			flush(next)
			next = next.Add(30 * time.Second)
		}
		at = m.Dest
		if m.Origin == m.Dest {
			refused++
		}
	}
	flush(next)
}
