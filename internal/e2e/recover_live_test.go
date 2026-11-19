package e2e

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// The scenarios' shortened recovery waits (persona defaults, D6: 2 and 5
// minutes).
const (
	recoverResurrectWait = 20 * time.Second
	recoverLeaderWait    = 30 * time.Second
)

// skillEscape is the five-minute recall cast /unstuck starts for a player
// (server network/usercommand.go escapeSkillRef).
const skillEscape int32 = 2099

// recoverGround is where the GM waits for the bot it killed: on open
// ground whose dead restart in Talking Island village (restartPointAreas:
// outside Cedric's Training Hall) and walk back over the island (#128,
// #131), about 8000 out on the road the bots took toward the human newbie
// start. Past it, beyond (-75767,251135), the ground drops into a pit, and
// around x -71700..-70600, y 252800..253400 the server refuses the legs
// cell after cell (acis_golang#5412): the walk back from there took longer
// than the scenario.
var recoverGround = botclient.Point{X: -78500, Y: 248900, Z: -3600}

func recoverPersona() botclient.Persona {
	p := partyPersona()
	p.Recover.ResurrectWait = recoverResurrectWait
	p.Recover.LeaderWait = recoverLeaderWait
	return p
}

// TestLiveRecoverVillageRestart kills a bot in the GM's party with //kill:
// nobody offers a resurrection, so the bot restarts in the village once its
// ResurrectWait ran, and comes back to the GM (#24), sidestepping where the
// server refuses its legs in place (acis_golang#5412, #128).
func TestLiveRecoverVillageRestart(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 8*time.Minute)
	gm := enterGM(t, ctx, tg)
	// The way back from the village depends on where the bot died: the GM
	// waits at recoverGround, not wherever the last scenario left it (#131).
	gmTeleport(t, ctx, gm, recoverGround)
	account, _ := tg.Account("rv", 1)
	f := runWatched(t, ctx, tg, account, botSetup{persona: recoverPersona})
	b, name := f.bot, f.name
	botID := recallBot(t, ctx, tg, gm, name)
	if r := invite(t, ctx, gm.Client, name); r != 1 {
		t.Fatalf("%s answered %d, want 1", name, r)
	}
	watchUntil(t, ctx, gm.Client, "the GM leading the bot's party", leaderIs(b, gm.Self.Name), nil)

	killed := f.watch.Len()
	if err := gm.Command(ctx, "kill "+name); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		d, ok := ev.(botclient.Die)
		return ok && d.ObjectID == botID
	}); err != nil {
		t.Fatalf("bot killed: %v", err)
	}
	died := time.Now()
	if _, err := gm.Await(ctx, recoverResurrectWait+time.Minute, func(ev botclient.Event) bool {
		r, ok := ev.(botclient.Revive)
		return ok && r.ObjectID == botID
	}); err != nil {
		t.Fatalf("bot restarted: %v", err)
	}
	if waited := time.Since(died); waited < recoverResurrectWait-time.Second {
		t.Fatalf("bot restarted %v after its death, want after its ResurrectWait %v", waited, recoverResurrectWait)
	} else {
		t.Logf("%s restarted %v after its death", name, waited)
	}
	// The restart teleports the bot to the village (#131).
	_, tp, err := f.watch.Await(ctx, killed, func(ev botclient.Event) bool {
		e, ok := ev.(botclient.TeleportToLocation)
		return ok && e.ObjectID == f.id
	})
	if err != nil {
		t.Fatalf("%s teleported to the village: %v", name, err)
	}
	tpPos := tp.Event.(botclient.TeleportToLocation).Pos
	t.Logf("%s restarted at %v, %.0f from the GM", name, tpPos, dist(tpPos, gm.Pos()))
	// The brain publishes its snapshots on its own clock, not the tap's: a
	// snapshot published while it applied the Revive, before the teleport,
	// can be stamped after tp.At and still stand by the GM. Only one in the
	// village is past the teleport.
	watchUntil(t, ctx, gm.Client, "the bot in the village", func() bool {
		return dist(b.Status().Position, tpPos) <= followReach
	}, nil)
	watchUntil(t, ctx, gm.Client, "the bot back at the GM", func() bool {
		s := b.Status()
		return s.HP > 0 && dist(s.Position, gm.Pos()) <= followReach
	}, nil)
	if n, at, _ := f.echoes(0); n > 0 {
		t.Logf("%s: %d ground moves refused in place on the way back, first at %v (acis_golang#5412)", name, n, at)
	}
}

// TestLiveRecoverLeaderLogout has the player leading a bot's party of two
// log out, which disperses it: the bot holds for its LeaderWait, whispers
// the gone leader once, then casts /unstuck, whose five-minute recall lands
// it in town. The GM watches (#24).
func TestLiveRecoverLeaderLogout(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 12*time.Minute)
	gm := enterGM(t, ctx, tg)
	acc, _ := tg.Account("rv", 2)
	leader := enter(t, ctx, tg, acc, character(t, acc, &botclient.HumanFighter), nil)
	account, _ := tg.Account("rv", 3)
	b := runBotWith(t, ctx, tg, account, botSetup{persona: recoverPersona})
	name := botOnline(t, ctx, tg, b)
	recallClient(t, ctx, gm, leader)
	botID := recallBot(t, ctx, tg, gm, name)
	if r := invite(t, ctx, leader, name); r != 1 {
		t.Fatalf("%s answered %d, want 1", name, r)
	}
	watchUntil(t, ctx, gm.Client, "the player leading the bot's party", leaderIs(b, leader.Self.Name), nil)

	if err := leader.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	gone := time.Now()
	ev, err := gm.Await(ctx, recoverLeaderWait+time.Minute, castBy(botID, skillEscape, 0))
	if err != nil {
		t.Fatalf("bot casts /unstuck: %v", err)
	}
	if waited := time.Since(gone); waited < recoverLeaderWait-time.Second {
		t.Fatalf("bot cast /unstuck %v after its leader left, want after its LeaderWait %v", waited, recoverLeaderWait)
	} else {
		t.Logf("%s cast /unstuck %v after its leader left", name, waited)
	}
	cast := ev.(botclient.MagicSkillUse)
	if _, err := gm.Await(ctx, cast.HitTime+time.Minute, func(ev botclient.Event) bool {
		switch e := ev.(type) {
		case botclient.TeleportToLocation:
			return e.ObjectID == botID
		case botclient.DeleteObject:
			return e.ObjectID == botID
		}
		return false
	}); err != nil {
		t.Fatalf("bot recalled to town: %v", err)
	}
	t.Logf("%s recalled to town", name)
}
