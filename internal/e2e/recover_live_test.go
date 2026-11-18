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

func recoverPersona() botclient.Persona {
	p := partyPersona()
	p.Recover.ResurrectWait = recoverResurrectWait
	p.Recover.LeaderWait = recoverLeaderWait
	return p
}

// TestLiveRecoverVillageRestart kills a bot in the GM's party with //kill:
// nobody offers a resurrection, so the bot restarts in the village once its
// ResurrectWait ran, and comes back to the GM (#24).
func TestLiveRecoverVillageRestart(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 8*time.Minute)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("rv", 1)
	b := runBotWith(t, ctx, tg, account, botSetup{persona: recoverPersona})
	name := botOnline(t, ctx, tg, b)
	botID := recallBot(t, ctx, tg, gm, name)
	if r := invite(t, ctx, gm.Client, name); r != 1 {
		t.Fatalf("%s answered %d, want 1", name, r)
	}
	watchUntil(t, ctx, gm.Client, "the GM leading the bot's party", leaderIs(b, gm.Self.Name), nil)

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
	watchUntil(t, ctx, gm.Client, "the bot back at the GM", func() bool {
		s := b.Status()
		return s.HP > 0 && dist(s.Position, gm.Pos()) <= followReach
	}, nil)
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
