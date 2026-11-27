package e2e

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// TestLiveStoreSale has a second client of the run buy out a bot's private
// sell store (#35): it opens the store's window as a player does, clicking
// the owner twice, buys every row at the listed price, gets the steel powder, and
// sees the store close once sold out.
func TestLiveStoreSale(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 10*time.Minute)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("st", 1)
	f := storeBot(t, ctx, tg, gm, account, storeSetup{})
	buyerAccount, _ := tg.Account("st", 2)
	c := enter(t, ctx, tg, buyerAccount, character(t, buyerAccount, nil), nil)
	recallClient(t, ctx, gm, c)
	giveClient(t, ctx, gm, c, adena, 10000)

	// The first click selects the owner, the second opens its store.
	for range 2 {
		if err := c.Send(ctx, botwire.EncodeAction(f.id, c.Pos())); err != nil {
			t.Fatal(err)
		}
	}
	ev, err := c.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		l, ok := ev.(botclient.PrivateStoreListSell)
		return ok && l.OwnerID == f.id
	})
	if err != nil {
		t.Fatalf("%s opens %s's store: %v", c.Self.Name, f.name, err)
	}
	list := ev.(botclient.PrivateStoreListSell)
	var rows []botclient.StoreBuy
	var cost int64
	var powder int32
	for _, it := range list.Items {
		rows = append(rows, botclient.StoreBuy{ObjectID: it.ObjectID, Count: it.Count, Price: it.Price})
		cost += int64(it.Count) * int64(it.Price)
		if it.TemplateID == steelPowder {
			powder += it.Count
		}
	}
	if powder == 0 || cost > int64(list.BuyerAdena) {
		t.Fatalf("%s's store lists %+v for %d adena, %s carries %d: want steel powder it can afford", f.name, list.Items, cost, c.Self.Name, list.BuyerAdena)
	}
	if err := c.Send(ctx, botclient.EncodeRequestPrivateStoreBuy(f.id, rows)); err != nil {
		t.Fatal(err)
	}
	var bought, closed bool
	closedOf := storeClosedOf(f.id)
	if _, err := c.Await(ctx, time.Minute, func(ev botclient.Event) bool {
		if n, _, ok := stock(ev, steelPowder); ok && n >= powder {
			bought = true
		}
		closed = closed || closedOf(ev)
		return bought && closed
	}); err != nil {
		t.Fatalf("%s bought %d steel powder: %v; the store closed: %v", c.Self.Name, powder, bought, closed)
	}
	t.Logf("%s bought %d rows (%d steel powder) for %d adena; %s's store closed sold out", c.Self.Name, len(rows), powder, cost, f.name)
}

// TestLiveStoreClosesOnFollow has the GM, the bot's commander, whisper
// "follow" to a bot in its store (#77): the bot quits its store, and then
// follows the GM when it walks off.
func TestLiveStoreClosesOnFollow(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 10*time.Minute)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("st", 3)
	f := storeBot(t, ctx, tg, gm, account, storeSetup{brain: commandedBy(gm)})
	tell(t, ctx, gm.Client, f.name, "follow")
	if _, err := gm.Await(ctx, 30*time.Second, storeClosedOf(f.id)); err != nil {
		t.Fatalf("%s quits its store on the follow order: %v", f.name, err)
	}
	t.Logf("%s quit its store on the follow order", f.name)
	if _, err := moveToward(ctx, gm.Client, awayFrom(gm.Pos(), 600, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, time.Minute, func(ev botclient.Event) bool {
		m, ok := ev.(botclient.MoveToLocation)
		return ok && m.ObjectID == f.id
	}); err != nil {
		t.Fatalf("%s follows the GM: %v", f.name, err)
	}
}

// TestLiveMerchantSwarm runs one bot of the merchant preset (cmd/bots
// -persona merchant: DefaultPersona with the Damage role farming the
// datapack's spots, with the travel map, a store registry and the store
// budget) on the run's gate and
// cap: with loot in a town it opens its store, which the GM sees (#111).
// GM-given steel powder stands in for the loot of a farming run, which takes tens
// of minutes on live spawns. Cleanup logs it out, which closes the store.
func TestLiveMerchantSwarm(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 10*time.Minute)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("ms", 1)
	f := storeBot(t, ctx, tg, gm, account, storeSetup{merchant: true})
	if s := f.bot.Status(); s.Town == nil || dist(*s.Town, townTI) > 3000 {
		t.Fatalf("%s's store stands in town %v, want Talking Island", f.name, s.Town)
	}
}
