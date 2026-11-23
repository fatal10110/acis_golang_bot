package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// opAppearing is the client's Appearing (clientpackets/appearing.go).
const opAppearing = 0x30

// recallRounds is how many times TestLiveRecallRefill recalls its bot.
const recallRounds = 5

// TestLiveRecallRefill recalls a whole bot to the GM, its commander, five
// times, the GM standing at open ground and in the village by turns (#199).
// The first time the GM invites the bot into its party right after the
// recall, so the invitation reaches the bot before its Appearing: the bot
// answers once the landing shows it the GM, and accepts; answered with the
// Appearing, it found nobody in sight and declined. Each time the bot sends
// one Appearing for its loading-screen teleport and the GM's CharInfo
// reaches it after that teleport, with no later teleport of its own to wipe
// the world it refilled. It logs the frames of each recall: the bot's own
// teleports, its Appearing and the objects shown and forgotten.
func TestLiveRecallRefill(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 5*time.Minute)
	gm := enterGM(t, ctx, tg)
	gmToOpenGround(t, ctx, gm)
	w := NewWatch(botclient.SystemClock{})
	acc, _ := tg.Account("rw", 1)
	// The bot joins only a party of a commander it knows: the GM, once in
	// sight, as it never whispers.
	commanderOnly := func() botclient.Persona {
		p := partyPersona()
		p.JoinParty = nil
		return p
	}
	b := runBotWith(t, ctx, tg, acc, botSetup{persona: commanderOnly, tap: w, brain: commandedBy(gm)})
	name := botOnline(t, ctx, tg, b)
	_, ui, err := w.Await(ctx, 0, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.UserInfo)
		return ok
	})
	if err != nil {
		t.Fatalf("the bot's UserInfo: %v", err)
	}
	self := ui.Event.(botclient.UserInfo).ObjectID
	tg.Own(name, self)
	for round := 1; round <= recallRounds; round++ {
		if round%2 == 0 {
			if err := gm.Command(ctx, fmt.Sprintf("teleport %d %d %d", townTI.X, townTI.Y, townTI.Z)); err != nil {
				t.Fatal(err)
			}
			if _, err := gm.Await(ctx, EntryTimeout, teleportOf(gm.Self.ObjectID)); err != nil {
				t.Fatalf("round %d: GM in the village: %v", round, err)
			}
		} else if round > 1 {
			gmToOpenGround(t, ctx, gm)
		}
		from, sentFrom := w.Len(), len(w.Sent())
		if err := gm.Command(ctx, "recall "+name); err != nil {
			t.Fatal(err)
		}
		if round == 1 {
			if r := invite(t, ctx, gm.Client, name); r != 1 {
				traceRecall(t, round, w, from, sentFrom, self, gm.Self.ObjectID)
				t.Fatalf("round %d: the recalled bot answered its commander's invitation %d, want 1 (accepted)", round, r)
			}
		}
		tp, _, err := w.Await(ctx, from, func(ev botclient.Event) bool {
			e, ok := ev.(botclient.TeleportToLocation)
			return ok && e.ObjectID == self && !e.Fast
		})
		if err != nil {
			t.Fatalf("round %d: the bot's own teleport: %v", round, err)
		}
		shown, cancel := context.WithTimeout(ctx, EntryTimeout)
		_, _, err = w.Await(shown, tp+1, func(ev botclient.Event) bool {
			e, ok := ev.(botclient.CharInfo)
			return ok && e.ObjectID == gm.Self.ObjectID
		})
		cancel()
		if err != nil {
			traceRecall(t, round, w, from, sentFrom, self, gm.Self.ObjectID)
			t.Fatalf("round %d: the GM shown to the recalled bot: %v", round, err)
		}
		// A teleport of the bot's own after the landing would wipe what it
		// showed: the bot settles a moment for one to show.
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(3 * time.Second):
		}
		tr := traceRecall(t, round, w, from, sentFrom, self, gm.Self.ObjectID)
		if tr.teleports != 1 || tr.appearing != 1 {
			t.Fatalf("round %d: %d own loading-screen teleports and %d Appearing sent, want one each", round, tr.teleports, tr.appearing)
		}
		if !tr.gmAfter {
			t.Fatalf("round %d: the bot no longer sees the GM after its own teleport at event %d", round, tr.last)
		}
	}
}

// recallTrace is what traceRecall read of one recall.
type recallTrace struct {
	last      int  // index of the bot's last own loading-screen teleport, -1 if none
	gmAfter   bool // whether the bot sees the GM since that teleport
	teleports int  // the bot's own loading-screen teleports
	appearing int  // Appearing packets the bot sent
}

// traceRecall logs the frames of one recall, from event from and packet
// sentFrom on: each own teleport, each Appearing sent and the objects shown
// and forgotten after the last teleport.
func traceRecall(t *testing.T, round int, w *Watch, from, sentFrom int, self, gm int32) recallTrace {
	t.Helper()
	appear := 0
	for _, s := range w.Sent()[sentFrom:] {
		if s.Op == opAppearing {
			appear++
			t.Logf("round %d: Appearing sent at %s", round, s.At.Format("15:04:05.000"))
		}
	}
	last, gmAfter, shown, deleted, own, slow := -1, false, 0, 0, 0, 0
	for i, s := range w.Seen(from) {
		switch e := s.Event.(type) {
		case botclient.TeleportToLocation:
			if e.ObjectID != self {
				continue
			}
			own++
			t.Logf("round %d: own teleport fast=%v at %s after %d sent, %d objects shown before", round, e.Fast, s.At.Format("15:04:05.000"), s.SentBefore, shown)
			if !e.Fast {
				slow++
				last, gmAfter, shown, deleted = from+i, false, 0, 0
			}
		case botclient.CharInfo:
			shown++
			if e.ObjectID == gm {
				gmAfter = true
			}
		case botclient.NPCInfo, botclient.SpawnItem:
			shown++
		case botclient.AskJoinParty:
			t.Logf("round %d: party invitation read at %s", round, s.At.Format("15:04:05.000"))
		case botclient.DeleteObject:
			deleted++
			if e.ObjectID == gm {
				gmAfter = false
			}
		}
	}
	t.Logf("round %d: %d own teleports, %d Appearing sent, %d objects shown and %d forgotten after the last", round, own, appear, shown, deleted)
	return recallTrace{last: last, gmAfter: gmAfter, teleports: slow, appearing: appear}
}
