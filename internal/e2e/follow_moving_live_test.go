package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

const (
	// movingLeg is how far the GM paces each way around the route's end of
	// TestLiveFollowMovingLeader: past the bot's Repath, so every leg
	// re-paths it.
	movingLeg = 300
	// movingNear is how near the route's end the bot ends the measure: the
	// GM stops pacing and the bot settles in its slot.
	movingNear = 800
	// movingDelay is the bot's follow delay in the scenario: long, so a
	// re-path is pending for most of each leg and a hold through it shows.
	movingDelay = 1500 * time.Millisecond
	// movingGapMax bounds a gap between two of the bot's moves on the way:
	// the walk wake's tick, the server's walk-speed start and the round
	// trip fit well under it, a hold through movingDelay does not.
	movingGapMax = time.Second
)

// TestLiveFollowMovingLeader measures a follower walking a long route to a
// leader that keeps moving (#306, docs/walking.md "Follower and moving
// destinations"): TestLiveWalkEfficiency's town route, with the GM pacing
// movingLeg back and forth around the route's end while the bot is farther
// than movingNear from it, so a re-path is pending for most of the walk. It
// logs one parseable "follow-moving" line: the GM's legs, the bot's
// segments, the gaps between its moves on the way as the server answered
// them (a move's start less the end of the one before at run speed), and
// the bot's own idle histogram (nav_walk_idle_ms, which a new slot's reset
// leaves out). No gap exceeds movingGapMax and every walker answer matches.
func TestLiveFollowMovingLeader(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 15*time.Minute)
	tg.Nav(t)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("wm", 1)
	r := walkRoutes[0]

	gmTeleport(t, ctx, gm, r.from)
	reg := botmetrics.NewRegistry()
	f := runWatched(t, ctx, tg, account, botSetup{
		class:   &botclient.HumanFighter,
		metrics: reg.NewBot(),
		persona: func() botclient.Persona {
			p := partyPersona()
			p.Follow.MinDist, p.Follow.MaxDist, p.Follow.Repath = walkFollowMin, walkFollowMax, 120
			p.Follow.DelayMin, p.Follow.DelayMax = movingDelay, movingDelay
			return p
		},
	})
	b, name := f.bot, f.name
	recallBot(t, ctx, tg, gm, name)
	botLanded(t, ctx, gm, b, gm.Pos())
	u, ok := lastUserInfo(f.watch.Seen(0))
	if !ok || u.RunSpeed == 0 || u.MaxLoad == 0 || u.Load >= u.MaxLoad {
		t.Fatalf("%s cannot walk: run speed %d, load %d/%d", name, u.RunSpeed, u.Load, u.MaxLoad)
	}
	from := len(f.watch.Seen(0))
	start := time.Now()
	t.Cleanup(func() { logRoute(t, f, from, start) })
	gmTeleport(t, ctx, gm, r.to)
	speed := float64(max(userInfo(t, gm.Client).RunSpeed, 1))
	if a := invite(t, ctx, gm.Client, name); a != 1 {
		t.Fatalf("%s answered %d, want 1", name, a)
	}

	wctx, stop := context.WithTimeout(ctx, 4*time.Minute)
	defer stop()
	legs := 0
	for side := int32(1); dist(b.Status().Position, r.to) > movingNear; side = -side {
		to := botclient.Point{X: r.to.X + side*movingLeg/2, Y: r.to.Y, Z: r.to.Z}
		m, err := moveToward(wctx, gm.Client, to)
		if err != nil {
			t.Fatalf("leg %d: %v", legs, err)
		}
		legs++
		if err := gm.Watch(wctx, seconds(dist(m.Origin, m.Dest)/speed)+200*time.Millisecond, func(botclient.Event) {}); err != nil {
			t.Fatalf("leg %d: %v", legs, err)
		}
	}
	near := time.Now()
	watchUntil(t, wctx, gm.Client, "the bot behind the GM", func() bool {
		return dist(b.Status().Position, gm.Pos()) <= walkArrive
	}, nil)

	gaps := moveGaps(f.watch.Seen(from), f.id, float64(u.RunSpeed), near)
	var sum, worst time.Duration
	for _, g := range gaps {
		sum += g
		worst = max(worst, g)
	}
	s := reg.Collect(time.Now())
	var idleMean int64
	if s.NavWalkIdleMillis.Count > 0 {
		idleMean = int64(s.NavWalkIdleMillis.Sum / s.NavWalkIdleMillis.Count)
	}
	t.Logf("follow-moving route=%s legs=%d segments=%d gaps=%d gap_mean_ms=%d gap_max_ms=%d idle_count=%d idle_mean_ms=%d idle_max_ms=%d blocked=%d desync=%d run_speed=%d",
		r.name, legs, s.NavSegments, len(gaps), (sum / time.Duration(max(len(gaps), 1))).Milliseconds(), worst.Milliseconds(),
		s.NavWalkIdleMillis.Count, idleMean, s.NavWalkIdleMillis.Max, s.NavBlocked, s.NavDesync, u.RunSpeed)
	if legs < 3 || len(gaps) < 3 {
		t.Errorf("%d legs, %d gaps: want the GM moving on through a walk of several segments", legs, len(gaps))
	}
	if worst > movingGapMax {
		t.Errorf("a gap of %v between two moves on the way, want at most %v: the bot held through a follow delay", worst, movingGapMax)
	}
	if s.NavBlocked != 0 || s.NavDesync != 0 {
		t.Errorf("nav_blocked_total %d, nav_desync_total %d over %d segments, want both 0", s.NavBlocked, s.NavDesync, s.NavSegments)
	}
}

// moveGaps reads the server's answers to the moves of character id in seen
// answered before until, running at run: for each move after the first, how
// long after the modeled end of the one before (its answer plus its length
// at run speed) it started. The server runs the first half second of a move
// at walk speed, so each gap is a little longer than the bot's own idle.
func moveGaps(seen []Seen, id int32, run float64, until time.Time) []time.Duration {
	var gaps []time.Duration
	var end time.Time
	for _, s := range seen {
		e, ok := s.Event.(botclient.MoveToLocation)
		if !ok || e.ObjectID != id || e.Origin == e.Dest || !s.At.Before(until) {
			continue
		}
		if !end.IsZero() {
			gaps = append(gaps, max(s.At.Sub(end), 0))
		}
		end = s.At.Add(seconds(dist(e.Origin, e.Dest) / run))
	}
	return gaps
}
