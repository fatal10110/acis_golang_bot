package e2e

import (
	"context"
	"hash/fnv"
	"math/rand/v2"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

// Helpers of the live scenarios. Time is real here: every scenario bounds
// itself with a context deadline.

// gremlin is an aggressive level 1 monster of the datapack
// (data/xml/npcs/20000-20999.xml, id 20001).
const gremlin int32 = 20001

func scenarioCtx(t *testing.T, d time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(t.Context(), d)
	t.Cleanup(cancel)
	return ctx
}

// character is a new character of account named as a bot's, of class when
// set; its draws derive from the account, so a rerun asks for the same one.
func character(t *testing.T, account string, class *botclient.StartingClass) botclient.Character {
	t.Helper()
	h := fnv.New64a()
	_, _ = h.Write([]byte(account))
	ch, err := botclient.NewCharacter(botnames.DefaultMarker, account, rand.New(rand.NewPCG(h.Sum64(), 1)))
	if err != nil {
		t.Fatal(err)
	}
	if class != nil {
		ch.Class = *class
	}
	return ch
}

// enter enters the world as ch on account, retrying while the account is
// still in use, and logs the character out when the test ends.
func enter(t *testing.T, ctx context.Context, tg *Target, account string, ch botclient.Character, rec *botclient.Recorder) *Client {
	t.Helper()
	c, err := tg.EnterWhenFree(ctx, account, ch, rec)
	if err != nil {
		t.Fatal(err)
	}
	logoutAtEnd(t, c)
	if userInfo(t, c).HP == 0 {
		revive(t, ctx, c)
	}
	return c
}

// logoutAtEnd has c leave the world once the test ends (Client.Leave).
func logoutAtEnd(t *testing.T, c *Client) {
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := c.Leave(ctx); err != nil {
			t.Logf("cleanup: logout %s: %v", c.Account, err)
		}
	})
}

// enterGM logs the GM in; when the test ends it deletes every monster the
// run spawned and still owns, then leaves the world before the GM goes
// back to the pool (Client.Leave).
func enterGM(t *testing.T, ctx context.Context, tg *Target) *GMClient {
	t.Helper()
	gm, err := tg.EnterGM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("GM of the pool: %s, karma %d at entry, reset to 0", filepath.Base(gm.file), gm.karmaIn)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		for _, id := range gm.Spawned() {
			if err := gm.Delete(ctx, id); err != nil {
				t.Logf("cleanup: delete monster %d: %v", id, err)
			}
		}
		if err := gm.Leave(ctx); err != nil {
			t.Logf("cleanup: GM logout: %v", err)
		}
	})
	if userInfo(t, gm.Client).HP == 0 {
		revive(t, ctx, gm.Client)
	}
	return gm
}

// revive restarts a character that entered dead (a scenario of another
// run left it so) in the village, as any player would: a dead character
// is no party member to buff or heal, and a dead GM sees and commands but
// its moves are refused. The run's filter does not let the GM revive
// itself by command.
func revive(t *testing.T, ctx context.Context, c *Client) {
	t.Helper()
	if err := c.Send(ctx, botclient.EncodeRequestRestartPoint(botclient.RestartVillage)); err != nil {
		t.Fatal(err)
	}
	revived, landed := false, false
	if _, err := c.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		switch e := ev.(type) {
		case botclient.Revive:
			revived = revived || e.ObjectID == c.Self.ObjectID
		case botclient.TeleportToLocation:
			landed = landed || e.ObjectID == c.Self.ObjectID
		}
		return revived && landed
	}); err != nil {
		t.Fatalf("%s restarted in the village: %v", c.Self.Name, err)
	}
	t.Logf("%s entered dead; restarted in the village", c.Self.Name)
}

func teleportOf(id int32) func(botclient.Event) bool {
	return func(ev botclient.Event) bool {
		tp, ok := ev.(botclient.TeleportToLocation)
		return ok && tp.ObjectID == id
	}
}

func charInfoOf(id int32) func(botclient.Event) bool {
	return func(ev botclient.Event) bool {
		ci, ok := ev.(botclient.CharInfo)
		return ok && ci.ObjectID == id
	}
}

// waitFor is waitForBy with the default step bound.
func waitFor(t *testing.T, ctx context.Context, what string, cond func() bool) {
	t.Helper()
	waitForBy(t, ctx, what, time.Now().Add(stepBound), cond)
}

// waitForBy polls cond every half second until it holds, or fails the test
// once the step is overdue at due or ctx ends.
func waitForBy(t *testing.T, ctx context.Context, what string, due time.Time, cond func() bool) {
	t.Helper()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	runStep(t, ctx, what, due, tickPoll(tick.C), pending(cond), nil)
}
