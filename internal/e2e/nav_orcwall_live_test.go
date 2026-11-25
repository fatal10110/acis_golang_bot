package e2e

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
)

// The Orc village (#504): its town point, and the spot lyonn07_18_14_15m1
// north of the village wall, about 1700 below it. On the format-3 cache of
// main the bot's route there climbed a ramp onto the wall's walkway and
// dropped 312 off its outer edge; it now leaves by the north gate.
var (
	orcTown = botclient.Point{X: -45264, Y: -112512, Z: -240}
	orcSpot = botclient.Point{X: -42592, Y: -109016, Z: -1776}
)

// orcDropBound is the largest single-cell descent a place walk takes where
// a way without it exists: nav's dropBound, twice the climb limit.
const orcDropBound = 2 * block.CellIgnoreHeight

// TestLiveNavOrcWall walks a bot out of the Orc village (#504): recalled to
// the town point, it follows the GM, who stands still at the spot north of
// the wall, so the walk is one place route (the spot is SameSCC with the
// town point). Every move the server answers for the bot, walked over the
// bot's geodata as CanMove walks it, descends at most orcDropBound in one
// cell, and the bot arrives with nav_blocked_total and nav_desync_total 0.
func TestLiveNavOrcWall(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 15*time.Minute)
	e := tg.Nav(t).Engine
	gm := enterGM(t, ctx, tg)
	gmTeleport(t, ctx, gm, orcTown)
	account, _ := tg.Account("ow", 1)
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
	from := len(f.watch.Seen(0))
	start := time.Now()
	t.Cleanup(func() { logRoute(t, f, from, start) })
	to := ground(e, orcSpot)
	gmTeleport(t, ctx, gm, to)
	if a := invite(t, ctx, gm.Client, name); a != 1 {
		t.Fatalf("%s answered %d, want 1", name, a)
	}
	wctx, stop := context.WithTimeout(ctx, 6*time.Minute)
	defer stop()
	watchUntil(t, wctx, gm.Client, "the walk out of the Orc village to the GM", func() bool {
		return dist(b.Status().Position, to) <= walkArrive
	}, nil)
	moves := 0
	for _, s := range f.watch.Seen(from) {
		m, ok := s.Event.(botclient.MoveToLocation)
		if !ok || m.ObjectID != f.id || m.Origin == m.Dest {
			continue
		}
		moves++
		if d := lineDescent(e, m.Origin, m.Dest); d > orcDropBound {
			t.Errorf("move %v -> %v descends %d in one cell, want at most %d (the wall's drop)", m.Origin, m.Dest, d, orcDropBound)
		}
	}
	s := reg.Collect(time.Now())
	t.Logf("orc-wall moves=%d segments=%d blocked=%d desync=%d", moves, s.NavSegments, s.NavBlocked, s.NavDesync)
	if moves < 3 {
		t.Errorf("%d moves, want the route's corners (at least 3)", moves)
	}
	if s.NavBlocked != 0 || s.NavDesync != 0 {
		t.Errorf("nav_blocked_total %d, nav_desync_total %d over %d segments, want both 0", s.NavBlocked, s.NavDesync, s.NavSegments)
	}
}

// lineDescent is the largest single-cell descent of a walk along the line
// from a to b over e, each cell landing on the layer nearest at or below
// the last floor plus block.CellIgnoreHeight, as CanMove lands it. It
// samples the line every quarter cell.
func lineDescent(e *engine.Engine, a, b botclient.Point) int {
	n := max(1, int(math.Ceil(dist(a, b)/(block.CellSize/4))))
	gx, gy := engine.GeoX(int(a.X)), engine.GeoY(int(a.Y))
	z := int(e.Height(int(a.X), int(a.Y), int(a.Z)))
	worst := 0
	for i := 1; i <= n; i++ {
		x := int(a.X) + (int(b.X)-int(a.X))*i/n
		y := int(a.Y) + (int(b.Y)-int(a.Y))*i/n
		nx, ny := engine.GeoX(x), engine.GeoY(y)
		if nx == gx && ny == gy {
			continue
		}
		h, _, ok := e.NodeBelow(nx, ny, z+block.CellIgnoreHeight)
		if !ok {
			break
		}
		worst = max(worst, z-int(h))
		z, gx, gy = int(h), nx, ny
	}
	return worst
}
