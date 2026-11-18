package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// stemPrice is the unit price the trading bot asks for a stem: a persona
// value (TradePolicy.Prices).
const stemPrice = 5

// TestLiveTradeFromBot has a second client of the run buy stems from a bot
// by trade (#34, #82): the bot accepts the request and offers its stems; a
// buyer that confirms with adena short of the price gets the trade
// cancelled, never confirmed by the bot; a buyer that pays the price gets
// the bot's confirm, and the trade settles with the stems in the buyer's
// inventory and the adena in the bot's.
func TestLiveTradeFromBot(t *testing.T) {
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 10*time.Minute)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("td", 1)
	f := runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.HumanFighter,
		persona: func() botclient.Persona {
			p := partyPersona()
			// Persona defaults (D6): a price for stems, and the buyer's
			// second request taken at once.
			p.Trade.Prices = map[int32]int32{stem: stemPrice}
			p.Trade.Every = 0
			return p
		},
	})
	toTown(t, ctx, tg, gm, f)
	if err := gm.Select(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	gmGive(t, ctx, gm, f, stem, 4)
	buyerAccount, _ := tg.Account("td", 2)
	c := enter(t, ctx, tg, buyerAccount, character(t, buyerAccount, nil), nil)
	recallClient(t, ctx, gm, c)
	giveClient(t, ctx, gm, c, adena, 1000)

	// Short of the price: the buyer's confirm locks the offers, and the
	// bot cancels.
	tr := openTrade(t, ctx, c, f)
	price := stemPrice * tr.offer.Count
	t.Logf("%s offers %d stems for %d adena", f.name, tr.offer.Count, price)
	botOk, done, _ := closeTrade(t, ctx, c, tr, price-1)
	if botOk || done.Success {
		t.Fatalf("a trade short of the price: bot confirmed %v, settled %v; want cancelled", botOk, done.Success)
	}

	// The price: the bot confirms first, then the buyer, and it settles.
	from := f.seen()
	tr = openTrade(t, ctx, c, f)
	price = stemPrice * tr.offer.Count
	if err := c.Send(ctx, botclient.EncodeAddTradeItem(tr.adena, price)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.TradePressOtherOk)
		return ok
	}); err != nil {
		t.Fatalf("%s confirms a paid trade: %v", f.name, err)
	}
	_, done, stems := closeTrade(t, ctx, c, tr, 0)
	if !done.Success {
		t.Fatal("the paid trade was cancelled")
	}
	want := tr.stems + tr.offer.Count
	if stems < want {
		if _, err := c.Await(ctx, 10*time.Second, func(ev botclient.Event) bool {
			if n, _, ok := stock(ev, stem); ok {
				stems = n
			}
			return stems >= want
		}); err != nil {
			t.Fatalf("%s has %d stems after the trade, want %d: %v", c.Self.Name, stems, want, err)
		}
	}
	waitFor(t, ctx, "the bot paid", func() bool { return rose(f.counts(adena, from)) })
	t.Logf("%s bought %d stems for %d adena: it holds %d", c.Self.Name, tr.offer.Count, price, stems)
}

// trade is a trade window a buyer opened with a bot.
type trade struct {
	// adena is the buyer's adena object, stems how many stems it held.
	adena, stems int32
	// offer is what the bot offered.
	offer botclient.TradeItem
}

// openTrade has c ask the bot f to trade and waits for the bot's offer.
func openTrade(t *testing.T, ctx context.Context, c *Client, f *farmer) trade {
	t.Helper()
	if err := c.Send(ctx, botclient.EncodeTradeRequest(f.id)); err != nil {
		t.Fatal(err)
	}
	ev, err := c.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		ts, ok := ev.(botclient.TradeStart)
		return ok && ts.PartnerID == f.id
	})
	if err != nil {
		t.Fatalf("%s accepts %s's trade: %v", f.name, c.Self.Name, err)
	}
	var tr trade
	for _, it := range ev.(botclient.TradeStart).Items {
		switch it.TemplateID {
		case adena:
			tr.adena = it.ObjectID
		case stem:
			tr.stems += it.Count
		}
	}
	if tr.adena == 0 {
		t.Fatalf("%s offers no adena", c.Self.Name)
	}
	ev, err = c.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.TradeOtherAdd)
		return ok
	})
	if err != nil {
		t.Fatalf("%s offers its goods: %v", f.name, err)
	}
	tr.offer = ev.(botclient.TradeOtherAdd).Item
	if tr.offer.TemplateID != stem {
		t.Fatalf("%s offers template %d, want stems", f.name, tr.offer.TemplateID)
	}
	return tr
}

// closeTrade adds pay adena to c's offer when not 0, confirms, and waits
// for the trade's end; botOk reports a confirm of the bot meanwhile, and
// stems the stems c's inventory showed last.
func closeTrade(t *testing.T, ctx context.Context, c *Client, tr trade, pay int32) (botOk bool, done botclient.SendTradeDone, stems int32) {
	t.Helper()
	if pay > 0 {
		if err := c.Send(ctx, botclient.EncodeAddTradeItem(tr.adena, pay)); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Send(ctx, botclient.EncodeTradeDone(true)); err != nil {
		t.Fatal(err)
	}
	stems = tr.stems
	ev, err := c.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		if _, ok := ev.(botclient.TradePressOtherOk); ok {
			botOk = true
		}
		if n, _, ok := stock(ev, stem); ok {
			stems = n
		}
		_, ok := ev.(botclient.SendTradeDone)
		return ok
	})
	if err != nil {
		t.Fatalf("the trade ends: %v", err)
	}
	return botOk, ev.(botclient.SendTradeDone), stems
}
