package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
)

// The navigation scenarios of docs/navigation.md (Testing) that no other
// scenario covers. Every one runs its bot through runWatched, which asserts
// nav_desync_total 0, nav_blocked_total at most 1 per 100 segments, and
// every blocked stop one WalkSteps predicts or one after a step longer than
// Emax (the step-chain check). The long walk across a region is
// TestLiveSpotSpread's trip to a far spot (#144), the ledges
// TestLiveFollowLedge and TestLiveFollowAcrossBand (#147, #148), the grocer
// visit TestLiveTownErrands (#122).

// Gludin's clan hall gludin_agit_002 (doors.xml) is a passage between two
// gates, each a pair of leaves that start closed and never close on a
// timer: hallSouth and hallNorth stand outside its south and north gate,
// hallInside between them. With a gate open the way through it is about 450
// long; with it closed the way round the building about 1900, and with
// both closed hallInside reaches neither side.
var (
	hallSouth  = botclient.Point{X: -83910, Y: 152880, Z: -3168}
	hallInside = botclient.Point{X: -83900, Y: 153330, Z: -3160}
	hallNorth  = botclient.Point{X: -83890, Y: 153780, Z: -3168}
	hallGateS  = []int32{17220005, 17220006}
	hallGateN  = []int32{17220007, 17220008}
	// hallGateSY and hallGateNY are the gates' lines; hallSouthFar and
	// hallNorthFar stand farther out than the bot's follow slots reach.
	hallGateSY   int32 = 153064
	hallGateNY   int32 = 153598
	hallSouthFar       = botclient.Point{X: -83910, Y: 152700, Z: -3150}
	hallNorthFar       = botclient.Point{X: -83890, Y: 154000, Z: -3100}
	// gludinCentre is Gludin's teleport point (teleports.xml), about 4400
	// from hallSouth through the town's streets.
	gludinCentre = botclient.Point{X: -80826, Y: 149775, Z: -3043}
)

// TestLiveNavStepChain has a bot follow the GM from outside the clan hall to
// Gludin's centre, out of sight, on segments along the town's walls: each
// blocked stop of the trip is compared with what WalkSteps predicts for the
// step the server took (runWatched).
func TestLiveNavStepChain(t *testing.T) {
	t.Parallel()
	followAcross(t, "nw", hallSouth, gludinCentre)
}

// TestLiveNavDoor walks a bot through the clan hall's doors as the GM opens
// and closes them. The bot follows the GM close (30 to 60 out, a persona
// default, D6), so its slot lies where the GM stands. Through the open
// south gate it walks into the hall to the GM; shut in with both gates
// closed, its walks to the GM north of the hall end unreachable for the
// door alone, never blocked, desync, watchdog or no route; once the north
// gate opens and the GM steps up to it, it walks out through it. The doors
// are left closed, as the datapack starts them. Two runs of this scenario
// at once would toggle each other's doors.
func TestLiveNavDoor(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 15*time.Minute)
	e := tg.Nav(t).Engine
	gm := enterGM(t, ctx, tg)
	setGate := func(ctx context.Context, gate []int32, open bool) error {
		verb := "close"
		if open {
			verb = "open"
		}
		for _, id := range gate {
			tg.OwnDoor(id)
			if err := gm.Command(ctx, fmt.Sprintf("%s %d", verb, id)); err != nil {
				return err
			}
		}
		return nil
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		for _, g := range [][]int32{hallGateS, hallGateN} {
			if err := setGate(ctx, g, false); err != nil {
				t.Logf("cleanup: closing the gate %v: %v", g, err)
			}
		}
	})
	gate := func(g []int32, open bool) {
		t.Helper()
		if err := setGate(ctx, g, open); err != nil {
			t.Fatal(err)
		}
	}
	gate(hallGateS, true)
	gate(hallGateN, false)

	gmTeleport(t, ctx, gm, ground(e, hallSouthFar))
	account, _ := tg.Account("nd", 1)
	f := runWatched(t, ctx, tg, account, botSetup{persona: func() botclient.Persona {
		p := partyPersona()
		p.Follow.MinDist, p.Follow.MaxDist, p.Follow.Repath = 30, 60, 120
		return p
	}})
	b, name := f.bot, f.name
	recallBot(t, ctx, tg, gm, name)
	botLanded(t, ctx, gm, b, gm.Pos())
	gmTeleport(t, ctx, gm, hallInside)
	start := time.Now()
	t.Cleanup(func() { logRoute(t, f, 0, start) })
	if r := invite(t, ctx, gm.Client, name); r != 1 {
		t.Fatalf("%s answered %d, want 1", name, r)
	}

	// Every walk that ends unreachable in the scenario ends so for a door.
	reasons := map[string]bool{}
	walkSeen := func() string {
		w := b.Status().Walk
		if w != "" {
			reasons[w] = true
		}
		return w
	}
	reach := func(what string, there func(botclient.Point) bool, within time.Duration) {
		t.Helper()
		wctx, stop := context.WithTimeout(ctx, within)
		defer stop()
		watchUntil(t, wctx, gm.Client, what, func() bool {
			walkSeen()
			return there(b.Status().Position)
		}, nil)
		t.Logf("%s: %s at %v after %v", what, name, b.Status().Position, time.Since(start).Round(time.Second))
	}
	reach("into the hall through the open south gate", inHall, 3*time.Minute)

	// Shut in: both gates closed, the GM north of the hall.
	gate(hallGateS, false)
	gmTeleport(t, ctx, gm, ground(e, hallNorthFar))
	shut := time.Now()
	for time.Since(shut) < 45*time.Second && !strings.HasPrefix(walkSeen(), "unreachable") {
		// Reads the GM's frames meanwhile; nothing matches.
		_, _ = gm.Await(ctx, 2*time.Second, func(botclient.Event) bool { return false })
		if err := ctx.Err(); err != nil {
			t.Fatal(err)
		}
	}
	at := b.Status().Position
	t.Logf("%s shut in at %v after %v, walk %q", name, at, time.Since(shut).Round(time.Second), b.Status().Walk)
	if !inHall(at) {
		t.Errorf("%s at %v, out of the hall with both gates closed", name, at)
	}

	// The north gate opens; the GM steps up to it, a leader move the
	// follower walks again on.
	gate(hallGateN, true)
	gmTeleport(t, ctx, gm, hallNorth)
	reach("out of the hall through the opened north gate", func(p botclient.Point) bool {
		return p.Y > hallGateNY && dist(p, hallNorth) <= 200
	}, 4*time.Minute)
	for w := range reasons {
		if w != "unreachable: door" {
			t.Errorf("%s: walk %q, want only door ends", name, w)
		}
	}
}

// inHall reports whether p is inside the clan hall, between its gates.
func inHall(p botclient.Point) bool {
	return p.X > -84000 && p.X < -83800 && p.Y > hallGateSY+16 && p.Y < hallGateNY-16
}

// ground is p on the floor of the bot's geodata nearest above its Z.
func ground(e *engine.Engine, p botclient.Point) botclient.Point {
	p.Z = int32(e.Height(int(p.X), int(p.Y), int(p.Z)))
	return p
}

// The obstacle of TestLiveNavMeleeObstacle, north-west of the newbie spot
// gludio32_1725_03m1 on Talking Island: obstacleMob stands 110 from
// obstacleStand with no straight walk either way in the bot's geodata, from
// any point within a cell of either (CanMove fails), the way round about
// 390 long.
var (
	obstacleStand = botclient.Point{X: -83111, Y: 249956, Z: -3648}
	obstacleMob   = botclient.Point{X: -83153, Y: 249855, Z: -3640}
)

// TestLiveNavMeleeObstacle has a farmer whose one spot is centred at
// obstacleStand fight a wolf the GM spawned at obstacleMob, behind the
// obstacle: the bot walks round to a stand point with an open line, and
// the kill lands. runWatched holds its blocked stops to at most one and its
// desyncs to none. Whether its pick-up needs a stand point of its own is
// logged: a drop lands where the server's geodata lets it fall
// (dropToGround), which the scenario does not choose.
func TestLiveNavMeleeObstacle(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 15*time.Minute)
	e := tg.Nav(t).Engine
	a, m := obstacleStand, obstacleMob
	if e.CanMove(int(a.X), int(a.Y), int(a.Z), int(m.X), int(m.Y), int(m.Z)) || e.CanMove(int(m.X), int(m.Y), int(m.Z), int(a.X), int(a.Y), int(a.Z)) {
		t.Fatalf("a straight walk between %v and %v holds: no obstacle", a, m)
	}
	spot := datapack.Spot{
		Maker: "e2e_obstacle",
		Territory: &datapack.Territory{Name: "e2e_obstacle", MinZ: a.Z - 100, MaxZ: a.Z + 100, Nodes: []datapack.Point{
			{X: a.X - 300, Y: a.Y - 300}, {X: a.X + 300, Y: a.Y - 300}, {X: a.X + 300, Y: a.Y + 300}, {X: a.X - 300, Y: a.Y + 300},
		}},
		Center:   datapack.Point{X: a.X, Y: a.Y},
		Z:        a.Z,
		MinLevel: 1, MaxLevel: 3, MaximumNPCs: 1,
	}
	spots := botclient.NewSpotRegistry([]datapack.Spot{spot}, 1)
	gm := enterGM(t, ctx, tg)
	var f *farmer
	t.Cleanup(func() { forgetKilled(gm, f) })
	account, _ := tg.Account("nm", 1)
	gmTeleport(t, ctx, gm, a)
	f = runFarmer(t, ctx, tg, account, spots, nil)
	recallBot(t, ctx, tg, gm, f.name)
	botLanded(t, ctx, gm, f.bot, gm.Pos())
	gmOn(t, ctx, gm, f, "set level 5")
	waitFor(t, ctx, "the farmer at its spot", func() bool { return f.bot.Status().Rule == "farm" })
	t.Logf("%s farms at %v", f.name, f.bot.Status().Position)

	spawned := time.Now()
	gmTeleport(t, ctx, gm, m)
	mob, err := gm.Spawn(ctx, wolf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { gmBackFor(t, gm, f, mob) })
	gmTeleport(t, ctx, gm, a)
	until(t, ctx, "the wolf behind the obstacle killed", func() string {
		if readFight(f, mob).spawnKilled {
			return ""
		}
		return "no kill of the wolf yet"
	})
	gm.Forget(mob)
	r := readFight(f, mob)
	segs := 0
	for _, s := range f.watch.Sent() {
		if s.Op == opMoveBackwardToLocation && s.At.After(spawned) {
			segs++
		}
	}
	t.Logf("%s killed the wolf after %d ground moves, %d swings; picked up a drop %v", f.name, segs, r.swings, r.pickedUp)
	if segs == 0 {
		t.Errorf("%s killed the wolf behind the obstacle without a ground move round it", f.name)
	}
}
