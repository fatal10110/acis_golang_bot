package e2e

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveRecallReappear recalls a bot to the GM: the bot is teleported
// next to the GM, answers Appearing, and the GM then sees it (#11).
func TestLiveRecallReappear(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 3*time.Minute)
	account, _ := tg.Account("w", 1)
	bot := enter(t, ctx, tg, account, character(t, account, nil), nil)
	gm := enterGM(t, ctx, tg)
	if err := gm.Command(ctx, "recall "+bot.Self.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.Await(ctx, EntryTimeout, teleportOf(bot.Self.ObjectID)); err != nil {
		t.Fatalf("bot recalled: %v", err)
	}
	if !within(bot.Pos(), gm.Pos(), 300) {
		t.Fatalf("bot recalled to %v, the GM stands at %v", bot.Pos(), gm.Pos())
	}
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(bot.Self.ObjectID)); err != nil {
		t.Fatalf("GM sees the recalled bot: %v", err)
	}
}

// TestLiveKilledBotRevivesInVillage kills a bot with the GM's //kill: it
// dies, asks to restart in the village, revives and is teleported there
// (#11).
func TestLiveKilledBotRevivesInVillage(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 3*time.Minute)
	account, _ := tg.Account("w", 2)
	bot := enter(t, ctx, tg, account, character(t, account, nil), nil)
	gm := enterGM(t, ctx, tg)
	if err := gm.Command(ctx, "kill "+bot.Self.Name); err != nil {
		t.Fatal(err)
	}
	ev, err := bot.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		d, ok := ev.(botclient.Die)
		return ok && d.ObjectID == bot.Self.ObjectID
	})
	if err != nil {
		t.Fatalf("bot killed: %v", err)
	}
	if !ev.(botclient.Die).Restart.Village {
		t.Fatalf("Die %+v offers no village", ev)
	}
	if err := bot.Send(ctx, botclient.EncodeRequestRestartPoint(botclient.RestartVillage)); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		r, ok := ev.(botclient.Revive)
		return ok && r.ObjectID == bot.Self.ObjectID
	}); err != nil {
		t.Fatalf("bot revived: %v", err)
	}
	if _, err := bot.Await(ctx, EntryTimeout, teleportOf(bot.Self.ObjectID)); err != nil {
		t.Fatalf("bot teleported to the village: %v", err)
	}
	t.Logf("%s revived in the village at %v", bot.Self.Name, bot.Pos())
}
