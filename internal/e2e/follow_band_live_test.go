package e2e

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// On Talking Island, the way from the village to the human fighters' start
// crosses a band around x -71700..-70600, y 252800..253400 where the server
// refuses ground moves cell after cell (acis_golang#5412). bandApproach is
// on that way, about 2500 short of the band, where the server reported a
// follower walking back from the village (#141); beyondBand, the old
// recoverGround of #128, is past it.
var (
	bandApproach = botclient.Point{X: -74167, Y: 250518, Z: -3975}
	beyondBand   = botclient.Point{X: -70744, Y: 258264, Z: -3112}
)

// TestLiveFollowAcrossBand has a bot in the GM's party at bandApproach
// follow the GM to beyondBand, out of sight: its legs stall at the band,
// and it detours along it until it gets past and back to the GM (#141).
func TestLiveFollowAcrossBand(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 8*time.Minute)
	gm := enterGM(t, ctx, tg)
	gmTeleport(t, ctx, gm, bandApproach)
	account, _ := tg.Account("fb", 1)
	f := runWatched(t, ctx, tg, account, botSetup{persona: partyPersona})
	b, name := f.bot, f.name
	recallBot(t, ctx, tg, gm, name)
	if r := invite(t, ctx, gm.Client, name); r != 1 {
		t.Fatalf("%s answered %d, want 1", name, r)
	}
	watchUntil(t, ctx, gm.Client, "the GM leading the bot's party", leaderIs(b, gm.Self.Name), nil)

	from, seen := b.Status().Position, f.watch.Len()
	gmTeleport(t, ctx, gm, beyondBand)
	start := time.Now()
	t.Cleanup(func() { logRoute(t, f, seen, start) })
	t.Logf("%s at %v, %.0f from the GM past the band", name, from, dist(from, gm.Pos()))
	watchUntil(t, ctx, gm.Client, "the bot past the band at the GM", func() bool {
		return dist(b.Status().Position, gm.Pos()) <= followReach
	}, nil)
	t.Logf("%s back at the GM after %v", name, time.Since(start).Round(time.Second))
	if n, at, _ := f.echoes(seen); n > 0 {
		t.Logf("%s: %d ground moves refused in place on the way, first at %v (acis_golang#5412)", name, n, at)
	}
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
