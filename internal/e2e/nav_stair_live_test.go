package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// The stair of TestLiveNavStair, in the Dark Elf village, a starting town
// (TownZone and PeaceZone, no water zone, no door within 2000): a straight
// flight running north up from the village ground at Z -4568 to a terrace
// at -4336, 232 higher over 336 (y 15432 to 15096), every 16-unit cell 8 or
// 16 above the one before, so each step of the server's chain lands within
// its two cell heights, and CanMove holds up and down every cell of it. The
// terrace runs flat about 240 on north and 480 wide. Measured on the bot's
// own geodata, cell by cell along x 10520. The village lies under a cavern
// roof, so every cell of the flight is multilayer and no stretch of it
// passes the open-corridor test: the planner checks the climb exactly.
// stairFoot stands on the ground 128 south of the flight's first step,
// stairTop on the terrace 136 north of its last.
var (
	stairFoot = botclient.Point{X: 10520, Y: 15560, Z: -4568}
	stairTop  = botclient.Point{X: 10520, Y: 14960, Z: -4336}
)

// A bot within stairReach on the ground and stairHeight in height of the
// GM stands on the GM's floor in its close follow slot: within stairReach
// of stairFoot it is off the flight, which starts 128 from it, and within
// stairReach of stairTop it is on the terrace, which starts 136 from it.
// The positions the server reported in the measured runs sat 24 above the
// geodata floor; the terrace is 232 above the ground.
const (
	stairReach  = 100
	stairHeight = 64
)

// TestLiveNavStair walks a bot up and down the stair: it follows the GM
// close (30 to 60 out, a persona default, D6) from stairFoot up the flight
// to stairTop, then back down to stairFoot. The terrace is 232 above the
// ground, so a bot standing on it climbed a stair; runWatched holds every
// blocked stop of the walk to what WalkSteps predicts for the step the
// server took (the step chain, #243), and its desyncs to none.
func TestLiveNavStair(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 12*time.Minute)
	e := tg.Nav(t).Engine
	for _, leg := range [][2]botclient.Point{{stairFoot, stairTop}, {stairTop, stairFoot}} {
		if err := stairOpen(e, leg[0], leg[1]); err != nil {
			t.Fatalf("the geodata changed: %v", err)
		}
	}
	gm := enterGM(t, ctx, tg)
	gmTeleport(t, ctx, gm, stairFoot)
	account, _ := tg.Account("sf", 1)
	f := runWatched(t, ctx, tg, account, botSetup{persona: func() botclient.Persona {
		p := partyPersona()
		p.Follow.MinDist, p.Follow.MaxDist, p.Follow.Repath = 30, 60, 120
		return p
	}})
	b, name := f.bot, f.name
	recallBot(t, ctx, tg, gm, name)
	botLanded(t, ctx, gm, b, gm.Pos())
	// A character over its weight limit has speed 0, and the server
	// refuses its every move (movement.go moveLivePlayer).
	if u, ok := lastUserInfo(f.watch.Seen(0)); !ok || u.RunSpeed == 0 || u.MaxLoad == 0 || u.Load >= u.MaxLoad {
		t.Fatalf("%s cannot walk: run speed %d, load %d/%d", name, u.RunSpeed, u.Load, u.MaxLoad)
	}
	start := time.Now()
	t.Cleanup(func() { logRoute(t, f, 0, start) })

	reach := func(what string, at botclient.Point) {
		t.Helper()
		wctx, stop := context.WithTimeout(ctx, 3*time.Minute)
		defer stop()
		watchUntil(t, wctx, gm.Client, what, func() bool {
			return onFloor(b.Status().Position, at)
		}, nil)
		t.Logf("%s: %s at %v after %v", what, name, b.Status().Position, time.Since(start).Round(time.Second))
	}
	gmTeleport(t, ctx, gm, stairTop)
	if r := invite(t, ctx, gm.Client, name); r != 1 {
		t.Fatalf("%s answered %d, want 1", name, r)
	}
	reach("up the stair to the GM on the terrace", stairTop)
	gmTeleport(t, ctx, gm, stairFoot)
	reach("down the stair to the GM on the ground", stairFoot)
}

// onFloor reports whether p stands within stairReach of at on the ground
// and within stairHeight of its floor.
func onFloor(p, at botclient.Point) bool {
	dz := p.Z - at.Z
	return dist(p, at) <= stairReach && dz >= -stairHeight && dz <= stairHeight
}

// stairOpen checks the walk from from to to on e as the scenario measured
// it: both points on their floor, CanMove open along the straight line, and
// the server's step chain reaching to unblocked at a level-1 run speed, in
// position updates 100 ms apart (the server's interval) and 200 ms apart
// (the bot's Emax bound).
func stairOpen(e *engine.Engine, from, to botclient.Point) error {
	a := location.Location{X: int(from.X), Y: int(from.Y), Z: int(from.Z)}
	z := location.Location{X: int(to.X), Y: int(to.Y), Z: int(to.Z)}
	for _, p := range []location.Location{a, z} {
		if h := int(e.Height(p.X, p.Y, p.Z)); h != p.Z {
			return fmt.Errorf("floor at %v is %d", p, h)
		}
	}
	if !e.CanMove(a.X, a.Y, a.Z, z.X, z.Y, z.Z) {
		return fmt.Errorf("no straight walk from %v to %v", a, z)
	}
	const speed = 122
	for _, step := range []time.Duration{100 * time.Millisecond, 200 * time.Millisecond} {
		steps := make([]time.Duration, 100)
		for i := range steps {
			steps[i] = step
		}
		if w := e.WalkSteps(a, z, speed, speed, steps); !w.Reached || w.Blocked {
			return fmt.Errorf("the step chain from %v to %v in %v steps stops at %v after %d (blocked %v)", a, z, step, w.Stop, w.Steps, w.Blocked)
		}
	}
	return nil
}
