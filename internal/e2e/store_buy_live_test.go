package e2e

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// The store a second client opens for TestLiveStoreBuy: no-grade shots at
// the grocers' price (7, the market cap: a spawned merchant's base price
// caps a store's), and a Scroll of Escape at more than 1.5 times its
// reference price of 400 (MarketPolicy.MaxOverReference 150).
const (
	storeShots      = 300
	storeShotPrice  = 7
	storeScrollCost = 1000
	buyerAdena      = 5000
)

// TestLiveStoreBuy has a second client open a private sell store on open
// ground, out of sight of any merchant, listing no-grade shots within the
// market's price cap and a Scroll of Escape over it (#177, #180). A bot
// stripped of its consumables, wanting both for its trip, logs in next to
// it: it buys the shots from the store at the listed price, and not the
// scroll.
func TestLiveStoreBuy(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 12*time.Minute)
	travel, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	buyerAcc, _ := tg.Account("sb", 1)
	sellerAcc, _ := tg.Account("sb", 2)
	strip(t, ctx, tg, gm, with(items, consumables...), buyerAcc, &botclient.HumanFighter, stripEnd{at: openGround, adena: buyerAdena}, squiresSword, squiresShirt, squiresPants)

	seller := enter(t, ctx, tg, sellerAcc, character(t, sellerAcc, nil), nil)
	recallClient(t, ctx, gm, seller)
	shots := giveClient(t, ctx, gm, seller, soulshotNG, storeShots)
	giveClient(t, ctx, gm, seller, scrollOfEscape, 1)
	openSellStore(t, ctx, gm, seller, map[int32]int32{soulshotNG: storeShotPrice, scrollOfEscape: storeScrollCost}, storeShots)
	t.Logf("%s sells %d of its %d shots at %d and a scroll at %d", seller.Self.Name, storeShots, shots, storeShotPrice, storeScrollCost)

	buyer := runTownBot(t, ctx, tg, buyerAcc, &botclient.HumanFighter, travel, func(p *botclient.Persona) {
		// No merchant visit: a merchant in sight only, and none is.
		p.Shop.Reach = 0
		p.Gear.Buy = false
	})
	from := buyer.seen()
	until(t, ctx, buyer.name+" buys the shots", func() string {
		switch {
		case len(sentOf(buyer.watch.Sent(), botclient.OpRequestPrivateStoreBuy)) == 0:
			return fmt.Sprintf("no RequestPrivateStoreBuy (rule %q)", buyer.bot.Status().Rule)
		case slices.Max(buyer.counts(soulshotNG, from)) < storeShots:
			// Its auto use charges one as soon as they come.
			return fmt.Sprintf("shots %v", buyer.counts(soulshotNG, from))
		}
		return ""
	})
	for _, p := range sentOf(buyer.watch.Sent(), botclient.OpRequestPrivateStoreBuy) {
		if p.Target != seller.Self.ObjectID {
			t.Errorf("%s bought from %d, want %s's store %d", buyer.name, p.Target, seller.Self.Name, seller.Self.ObjectID)
		}
	}
	if n := buyer.carries(scrollOfEscape); n != 0 {
		t.Errorf("%s bought %d Scrolls of Escape at %d, over the cap", buyer.name, n, storeScrollCost)
	}
	spent := buyer.counts(adena, from)
	// The counts start at 0 when from comes before the ItemList: the
	// most it held is what it had before it paid.
	if paid := slices.Max(spent) - spent[len(spent)-1]; paid != storeShots*storeShotPrice {
		t.Errorf("%s paid %d adena (%v), want %d", buyer.name, paid, spent, storeShots*storeShotPrice)
	}
	t.Logf("%s bought shots %v for adena %v", buyer.name, buyer.counts(soulshotNG, from), spent)
}

// openSellStore opens c's private sell store listing, of each template of
// prices it carries, count (1 for a single item) at its price, as a player
// does: the manage window first, then the title and the list. It waits
// until the GM, next to c, sees the store open.
func openSellStore(t *testing.T, ctx context.Context, gm *GMClient, c *Client, prices map[int32]int32, count int32) {
	t.Helper()
	if err := c.Send(ctx, botclient.EncodeRequestPrivateStoreManageSell()); err != nil {
		t.Fatal(err)
	}
	ev, err := c.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.PrivateStoreManageListSell)
		return ok
	})
	if err != nil {
		t.Fatalf("%s's store manage window: %v", c.Self.Name, err)
	}
	var rows []botclient.StoreBuy
	for _, it := range ev.(botclient.PrivateStoreManageListSell).Candidates {
		if price, ok := prices[it.TemplateID]; ok {
			rows = append(rows, botclient.StoreBuy{ObjectID: it.ObjectID, Count: min(count, it.Count), Price: price})
		}
	}
	if len(rows) != len(prices) {
		t.Fatalf("%s's manage window offers %d of the %d templates", c.Self.Name, len(rows), len(prices))
	}
	if err := c.Send(ctx, botclient.EncodeSetPrivateStoreMsgSell("e2e")); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(ctx, botclient.EncodeSetPrivateStoreListSell(rows)); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, 30*time.Second, storeOf(c.Self.ObjectID)); err != nil {
		t.Fatalf("the GM sees %s's store open: %v", c.Self.Name, err)
	}
}
