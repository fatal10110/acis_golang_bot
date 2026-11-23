package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

// walkRoute is one fixed walk of TestLiveWalkEfficiency: the bot stands at
// from and walks to its leader at to. Each route takes several corners on
// the bot's own planner (measured on the format-3 nav cache of main): town,
// 11 segments over 3305 (3077 straight) from the Talking Island armory
// round the village houses to the master; field, 14 segments over 6483
// (5720 straight) across the open ground north-east of the village, from
// the newbie spots to the open slope.
type walkRoute struct {
	name     string
	from, to botclient.Point
}

var walkRoutes = []walkRoute{
	{name: "town", from: tiArmory, to: tiMaster},
	{name: "field", from: tiSpots, to: openGround},
}

// walkArrive is how near its leader at the route's end the bot ends its
// walk: its follow slot (walkFollowMax) plus the walker's own slop.
const (
	walkFollowMin, walkFollowMax = 30, 60
	walkArrive                   = walkFollowMax + 64
)

// TestLiveWalkEfficiency measures the walker on two fixed multi-corner
// routes, a town one and an open-field one (#304, docs/walking.md
// "Measurement (#304)"): a fresh human fighter per route, recalled to the
// route's start, follows the GM, who stands still at its end, so the walk
// is one walk to a fixed point (the follow hold comes before its first
// segment, outside the measure). For each route it logs one parseable
// "walk-efficiency" line: segments, route length and straight distance,
// the elapsed route time from the first segment's answer to the last
// segment's end at run speed, the running times of the route and of the
// straight line, the efficiency, the bot's own per-walk efficiency
// (nav_walk_efficiency_permille) and its idle histogram
// (nav_walk_idle_ms). Every walker answer must match: nav_blocked_total
// and nav_desync_total are both 0. Later walking lanes rerun it and append
// the line's numbers to docs/navigation.md, "Walking efficiency".
func TestLiveWalkEfficiency(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 15*time.Minute)
	tg.Nav(t)
	gm := enterGM(t, ctx, tg)
	for i, r := range walkRoutes {
		t.Run(r.name, func(t *testing.T) {
			account, _ := tg.Account("we", i+1)
			walkMeasure(t, ctx, tg, gm, account, r)
		})
	}
}

// walkMeasure walks a fresh bot of account along r and logs and checks its
// numbers. The bot logs out when t ends.
func walkMeasure(t *testing.T, ctx context.Context, tg *Target, gm *GMClient, account string, r walkRoute) {
	gmTeleport(t, ctx, gm, r.from)
	reg := botmetrics.NewRegistry()
	f := runWatched(t, ctx, tg, account, botSetup{
		class:   &botclient.HumanFighter,
		metrics: reg.NewBot(),
		persona: func() botclient.Persona {
			p := partyPersona()
			p.Follow.MinDist, p.Follow.MaxDist, p.Follow.Repath = walkFollowMin, walkFollowMax, 120
			return p
		},
	})
	b, name := f.bot, f.name
	recallBot(t, ctx, tg, gm, name)
	botLanded(t, ctx, gm, b, gm.Pos())
	// A character over its weight limit has speed 0, and the server
	// refuses its every move (movement.go moveLivePlayer).
	u, ok := lastUserInfo(f.watch.Seen(0))
	if !ok || u.RunSpeed == 0 || u.MaxLoad == 0 || u.Load >= u.MaxLoad {
		t.Fatalf("%s cannot walk: run speed %d, load %d/%d", name, u.RunSpeed, u.Load, u.MaxLoad)
	}
	from := len(f.watch.Seen(0))
	start := time.Now()
	t.Cleanup(func() { logRoute(t, f, from, start) })
	gmTeleport(t, ctx, gm, r.to)
	if a := invite(t, ctx, gm.Client, name); a != 1 {
		t.Fatalf("%s answered %d, want 1", name, a)
	}
	wctx, stop := context.WithTimeout(ctx, 4*time.Minute)
	defer stop()
	watchUntil(t, wctx, gm.Client, "the walk to the GM at the route's end", func() bool {
		return dist(b.Status().Position, r.to) <= walkArrive && reg.Collect(time.Now()).NavWalkEfficiency.Count > 0
	}, nil)
	s := reg.Collect(time.Now())
	m := walkMoves(f.watch.Seen(from), f.id, float64(u.RunSpeed))
	if m.segments < 3 {
		t.Errorf("%d segments, want the route's corners (at least 3): the geodata or the route changed", m.segments)
	}
	run := float64(u.RunSpeed)
	elapsed := m.end.Sub(m.start)
	idle, err := json.Marshal(s.NavWalkIdleMillis)
	if err != nil {
		t.Fatal(err)
	}
	var idleMean int64
	if s.NavWalkIdleMillis.Count > 0 {
		idleMean = int64(s.NavWalkIdleMillis.Sum / s.NavWalkIdleMillis.Count)
	}
	t.Logf("walk-efficiency route=%s segments=%d length=%.0f straight=%.0f elapsed=%v route_run=%v straight_run=%v efficiency=%.3f bot_efficiency_permille=%d walks=%d idle_count=%d idle_mean_ms=%d idle_max_ms=%d blocked=%d desync=%d run_speed=%d idle_ms=%s",
		r.name, m.segments, m.length, m.straight, elapsed.Round(10*time.Millisecond),
		seconds(m.length/run), seconds(m.straight/run), m.length/(elapsed.Seconds()*run),
		s.NavWalkEfficiency.Sum/max(s.NavWalkEfficiency.Count, 1), s.NavWalkEfficiency.Count,
		s.NavWalkIdleMillis.Count, idleMean, s.NavWalkIdleMillis.Max, s.NavBlocked, s.NavDesync, u.RunSpeed, idle)
	if s.NavBlocked != 0 || s.NavDesync != 0 {
		t.Errorf("nav_blocked_total %d, nav_desync_total %d over %d segments, want both 0", s.NavBlocked, s.NavDesync, s.NavSegments)
	}
}

// walkTrip is a walk as the server answered it: its segments, the summed
// ground length of their answers, the ground distance from the first
// answer's origin to the last answer's end, and the route time from the
// first answer to the last segment's end at run speed (the server runs the
// first half second of each segment at walk speed, so the true end is a
// little later).
type walkTrip struct {
	segments         int
	length, straight float64
	start, end       time.Time
}

// walkMoves reads the server's answers to the moves of character id in
// seen, running at run: every MoveToLocation of it that moves.
func walkMoves(seen []Seen, id int32, run float64) walkTrip {
	var w walkTrip
	var first, last botclient.MoveToLocation
	for _, s := range seen {
		e, ok := s.Event.(botclient.MoveToLocation)
		if !ok || e.ObjectID != id || e.Origin == e.Dest {
			continue
		}
		if w.segments == 0 {
			first, w.start = e, s.At
		}
		w.segments++
		w.length += dist(e.Origin, e.Dest)
		w.end = s.At.Add(seconds(dist(e.Origin, e.Dest) / run))
		last = e
	}
	if w.segments > 0 {
		w.straight = dist(first.Origin, last.Dest)
	}
	return w
}

// seconds is s seconds as a duration, to the millisecond.
func seconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second)).Round(time.Millisecond)
}
