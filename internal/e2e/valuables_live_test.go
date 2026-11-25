package e2e

import (
	"context"
	"encoding/binary"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// The valuables scenarios (#402, docs/valuables/preset.md "Live scenarios
// (V9)"): a merchant-preset human mystic in Talking Island village, a
// buyer with an unmarked name (buyerCharacter) and the GM; three characters,
// under the cap of 4.

// Items and keepers of the scenarios (datapack items/0100-0199.xml,
// items/0300-0399.xml; spawnlist/17_25.xml): two valuables no mystic wields,
// a non-magic sword and an ETC weapon, and Talking Island's freightman Rant,
// whose pages carry no private link, beside its private keeper Rolfe.
const (
	swordOfWatershadow int32 = 121
	tearsOfEva         int32 = 309
	keeperRant         int32 = 30054
	keeperRolfe        int32 = 30055

	swordName = "Sword of Watershadow"
	tearsName = "Tears of Eva"
	// valuablesBuyerAdena pays for one valuable at its highest first ask
	// (150 % of 136000) by trade or store, within one GM give.
	valuablesBuyerAdena int32 = 300000
	// privateFee is the fee of a private warehouse deposit per row
	// (network/warehouse.go).
	privateFee int32 = 30
	// opSetPrivateStoreListSell is the packet that lists and opens a sell
	// store (clientpackets SetPrivateStoreListSell).
	opSetPrivateStoreListSell byte = 0x74
)

// TestLiveValuablesTradeSale: the GM gives the merchant in town a Sword of
// Watershadow; the buyer hears its Trade line, whispers an offer under the
// ask, gets a counter, whispers the counter, gets the agreement and the
// merchant's TradeRequest, accepts it and pays: the trade settles at that
// price, the sword in the buyer's inventory and the adena in the merchant's.
func TestLiveValuablesTradeSale(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 25*time.Minute)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("vm", 1)
	f := valuablesMerchant(t, ctx, tg, gm, account, nil)
	b := enterBuyer(t, ctx, tg, gm, f)
	swords := f.carries(swordOfWatershadow)
	from := f.seen()
	giveMerchant(t, ctx, gm, f, swordOfWatershadow, 1)

	ad := b.hearAd(t, ctx, swordName)
	t.Logf("%s heard %s: %q", b.c.Self.Name, f.name, ad)
	// The buyer stands by the merchant at its anchor, within InviteReach.
	gmTo(t, ctx, gm, f)
	recallClient(t, ctx, gm, b.c)
	reply := b.ask(t, ctx, "watershadow 140k")
	counter, word, ok := replyPrice(reply, " lowest")
	if !ok {
		t.Fatalf("%s answered the offer %q, want a counter \"<price> lowest\"", f.name, reply)
	}
	reply = b.ask(t, ctx, "watershadow "+word)
	agreed, _, ok := replyPrice(reply, ", trade me")
	if !ok || !strings.HasPrefix(reply, "ok ") || agreed != counter {
		t.Fatalf("%s answered the counter %s with %q, want \"ok %s, trade me\"", f.name, word, reply, word)
	}
	t.Logf("%s agreed at %d", f.name, agreed)

	b.until(t, ctx, f.name+" asks "+b.c.Self.Name+" to trade", 3*time.Minute, func() bool { return b.requested })
	if err := b.c.Send(ctx, botclient.EncodeAnswerTradeRequest(true)); err != nil {
		t.Fatal(err)
	}
	paid := b.buyByTrade(t, ctx, int32(agreed))
	waitFor(t, ctx, f.name+" is paid and hands the sword over", func() bool {
		run := f.counts(adena, from)
		return run[len(run)-1]-run[0] == paid && f.carries(swordOfWatershadow) == swords
	})
	t.Logf("%s sold its sword to %s for %d: adena %v", f.name, b.c.Self.Name, paid, f.counts(adena, from))
	wearsNeither(t, f)
	privateLinks(t, f, 0)
}

// TestLiveValuablesWarehouse: with RoundGap lowered, the buyer stays silent;
// after its round the merchant deposits the sword at Rolfe (30 adena a row,
// the sword gone); the GM gives it Tears of Eva unless it carried one from
// an earlier run; its next round withdraws the sword at Rolfe, and one Trade
// line the buyer hears names both. No private link names Rant.
func TestLiveValuablesWarehouse(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 30*time.Minute)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("vm", 2)
	f := valuablesMerchant(t, ctx, tg, gm, account, func(v *botclient.ValuablePolicy) {
		// Persona defaults (D6): two rounds within the scenario, and no
		// cap release, which would send what earlier runs left with the
		// merchant to its store instead of the warehouse.
		v.RoundGap, v.RoundWait, v.Grace = 3*time.Minute, time.Minute, 30*time.Second
		v.HoldRows, v.HoldValue, v.MaxRounds = 1000, 1<<40, 1000
	})
	if have := f.carries(adena); have < 1000 {
		giveMerchant(t, ctx, gm, f, adena, 1000-have)
	}
	b := enterBuyer(t, ctx, tg, gm, f)
	from, sent := f.seen(), len(f.watch.Sent())
	// A sword and Tears an earlier run's second round withdrew serve as
	// well, and the merchant's stock (and the deposit's fee) stays bounded.
	if f.carries(swordOfWatershadow) == 0 {
		giveMerchant(t, ctx, gm, f, swordOfWatershadow, 1)
	}
	hadTears := f.carries(tearsOfEva) > 0

	untilServedBy(t, ctx, f.name+" deposits the sword after its round", time.Now().Add(12*time.Minute), f.watch, warehouseRefused(f.watch), func() string {
		switch {
		case len(sentOf(f.watch.Sent()[sent:], opWHDeposit)) == 0:
			return fmt.Sprintf("no RequestWarehouseDeposit (rule %q)", f.bot.Status().Rule)
		case f.carries(swordOfWatershadow) != 0:
			return fmt.Sprintf("it carries %d swords", f.carries(swordOfWatershadow))
		}
		return ""
	})
	var rows int32
	for _, p := range sentOf(f.watch.Sent()[sent:], opWHDeposit) {
		rows += p.Target // the deposit's row count (clientpackets RequestWarehouseDeposit)
	}
	waitFor(t, ctx, f.name+" pays the deposit's fee", func() bool {
		run := f.counts(adena, from)
		return run[0]-run[len(run)-1] == rows*privateFee
	})
	t.Logf("%s deposited %d rows: adena %v", f.name, rows, f.counts(adena, from))

	// The round's lines the buyer has not read yet are no next round's.
	if err := b.c.Watch(ctx, 3*time.Second, b.note); err != nil {
		t.Fatal(err)
	}
	lines, sent := len(b.ads), len(f.watch.Sent())
	if !hadTears {
		giveMerchant(t, ctx, gm, f, tearsOfEva, 1)
	}
	b.until(t, ctx, b.c.Self.Name+" hears a Trade line naming "+tearsName, 12*time.Minute, func() bool {
		return b.adSince(lines, tearsName) != ""
	})
	ad := b.adSince(lines, tearsName)
	if !strings.Contains(ad, swordName) {
		t.Errorf("the next round's Trade line %q names no %s", ad, swordName)
	}
	t.Logf("%s heard %s: %q", b.c.Self.Name, f.name, ad)
	links := privateLinks(t, f, sent)
	t.Logf("%s's private links: %+v", f.name, privateLinks(t, f, 0))
	if !hasLink(links, "_WithdrawP") {
		t.Errorf("the next round withdrew nothing first: private links %q", links)
	}
	if n := f.carries(swordOfWatershadow); n == 0 {
		t.Errorf("%s carries no sword after the withdrawal", f.name)
	}
	if !hasLink(privateLinks(t, f, 0), "_DepositP") {
		t.Error("no _DepositP link sent")
	}
	wearsNeither(t, f)
}

// TestLiveValuablesRelease: with HoldRows lowered to 1, the merchant
// advertises the Sword of Watershadow and Tears of Eva for a round, its cap
// then releases both to its store: it deposits nothing and lists only them,
// at their floor; the buyer buys there at the listed prices.
func TestLiveValuablesRelease(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 25*time.Minute)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("vm", 3)
	f := valuablesMerchant(t, ctx, tg, gm, account, func(v *botclient.ValuablePolicy) {
		// Persona defaults (D6): two valuables take more rows than the
		// cap holds, and a short round.
		v.HoldRows, v.RoundWait, v.Grace = 1, time.Minute, 30*time.Second
	})
	b := enterBuyer(t, ctx, tg, gm, f)
	from, sent := f.seen(), len(f.watch.Sent())
	giveMerchant(t, ctx, gm, f, swordOfWatershadow, 1)
	giveMerchant(t, ctx, gm, f, tearsOfEva, 1)

	b.until(t, ctx, b.c.Self.Name+" hears the round's Trade line", 12*time.Minute, func() bool { return b.adSince(0, swordName) != "" })
	waitForBy(t, ctx, f.name+" lists its store after the round", time.Now().Add(8*time.Minute), func() bool {
		return len(sentOf(f.watch.Sent()[sent:], opSetPrivateStoreListSell)) > 0
	})
	gmTo(t, ctx, gm, f)
	if _, err := gm.Await(ctx, time.Minute, storeOf(f.id)); err != nil {
		t.Fatalf("the GM sees %s's store open: %v", f.name, err)
	}
	if hasLink(privateLinks(t, f, sent), "_DepositP") {
		t.Error("the merchant deposited a released valuable")
	}

	recallClient(t, ctx, gm, b.c)
	list := b.storeList(t, ctx, f.id)
	t.Logf("%s's store: %+v", f.name, list.Items)
	// It buys every listed valuable its adena pays for, at least one: what
	// earlier runs left with the merchant is listed too, and goes.
	var rows []botclient.StoreItem
	coins := list.BuyerAdena
	for i, it := range list.Items {
		if it.TemplateID != swordOfWatershadow && it.TemplateID != tearsOfEva {
			t.Errorf("store row %d lists template %d, no released valuable: %+v", i, it.TemplateID, list.Items)
			continue
		}
		if it.ReferencePrice > 0 && it.Price < it.ReferencePrice {
			t.Errorf("%s lists template %d at %d, under its reference price %d", f.name, it.TemplateID, it.Price, it.ReferencePrice)
		}
		if it.Price <= coins {
			rows, coins = append(rows, it), coins-it.Price
		}
	}
	if len(rows) == 0 {
		t.Fatalf("%s's store lists no valuable %s pays for with %d adena: %+v", f.name, b.c.Self.Name, list.BuyerAdena, list.Items)
	}
	paid := b.buyFromStore(t, ctx, f.id, rows, list.BuyerAdena)
	waitFor(t, ctx, f.name+" is paid for its valuables", func() bool {
		run := f.counts(adena, from)
		return run[len(run)-1]-run[0] == paid
	})
	wearsNeither(t, f)
	privateLinks(t, f, 0)
}

// valuablesMerchant runs the merchant of account: a human mystic with the
// merchant preset's valuables policy (tune adjusts it), its shared ad board,
// the travel map and a store registry, recalled to Talking Island village
// with the GM next to it. Its persona is partyPersona's otherwise, so it
// moves only for its valuables, with the gear errand's wearing on and,
// persona defaults (D6), no market visits, supply trips or loot store: the
// store opens only for released valuables.
func valuablesMerchant(t *testing.T, ctx context.Context, tg *Target, gm *GMClient, account string, tune func(*botclient.ValuablePolicy)) *farmer {
	t.Helper()
	travel := travelMap(t, tg)
	stores := botclient.NewStoreRegistry(travel.Towns(), storesPerTown, storeSpacing)
	f := runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.HumanMystic,
		persona: func() botclient.Persona {
			p := partyPersona()
			p.Role = botclient.RoleDamage
			p.Gear = botclient.DefaultPersona().Gear
			p.Gear.Buy = false
			p.Market, p.Supply = botclient.MarketPolicy{}, botclient.SupplyPolicy{}
			p.Store.Budget = 0
			p.Valuables = botclient.DefaultValuablePolicy()
			if tune != nil {
				tune(&p.Valuables)
			}
			return p
		},
		brain: func(cfg *botclient.BrainConfig) {
			cfg.Travel, cfg.Stores = travel, stores
			cfg.Ads = botclient.NewAdBoard(botclient.DefaultAdRegionGap, botclient.DefaultAdProcessPerHour)
		},
	})
	toTown(t, ctx, tg, gm, f)
	return f
}

// giveMerchant has the GM, brought next to f wherever it walks (a look or
// round may have taken it to a keeper), give it count of tmpl, and waits
// until f carries more of it. It counts by object (farmer.carries), not by
// gmGive's stock, which reads one row: a sword an earlier run left with f
// would hide the new one.
func giveMerchant(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, tmpl, count int32) {
	t.Helper()
	gmOn(t, ctx, gm, f)
	before := f.carries(tmpl)
	if err := gm.Give(ctx, f.id, tmpl, count); err != nil {
		t.Fatal(err)
	}
	waitForBy(t, ctx, fmt.Sprintf("%s gets %d of %d", f.name, count, tmpl), time.Now().Add(time.Minute), func() bool {
		return f.carries(tmpl) > before
	})
}

// valuablesBuyer is the buyer's client and what it heard from the merchant.
type valuablesBuyer struct {
	c          *Client
	merchant   string
	merchantID int32
	ads, tells []string
	requested  bool
	// bought are the object ids of the valuables it bought this run.
	bought []int32
}

// enterBuyer enters the buyer (<prefix>vb1, buyerCharacter) next to the GM,
// near the merchant f, with adena for one valuable.
func enterBuyer(t *testing.T, ctx context.Context, tg *Target, gm *GMClient, f *farmer) *valuablesBuyer {
	t.Helper()
	account, _ := tg.Account("vb", 1)
	c := enter(t, ctx, tg, account, buyerCharacter(t, account), nil)
	recallClient(t, ctx, gm, c)
	// Its entry's ItemList is read already: the buyer counts its stock from
	// the trade window and the store list, and gets the adena every run.
	giveClient(t, ctx, gm, c, adena, valuablesBuyerAdena)
	b := &valuablesBuyer{c: c, merchant: f.name, merchantID: f.id}
	// Registered after enter's, so it runs while the buyer is still in
	// the world; t.Context, and so ctx, is done by then.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		b.destroyBought(t, ctx)
	})
	return b
}

// destroyBought destroys the valuables the buyer bought this run, one at a
// time, so the shared buyer's weight and rows stay bounded across runs.
func (b *valuablesBuyer) destroyBought(t *testing.T, ctx context.Context) {
	t.Helper()
	for _, id := range b.bought {
		if err := b.c.Send(ctx, botclient.EncodeRequestDestroyItem(id, 1)); err != nil {
			t.Errorf("%s destroys bought item %d: %v", b.c.Self.Name, id, err)
			return
		}
		ev, err := b.c.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
			switch e := ev.(type) {
			case botclient.SystemMessage:
				return slices.Contains(destroyRefusals, e.ID)
			case botclient.InventoryUpdate:
				for _, c := range e.Changes {
					if c.Item.ObjectID == id && c.State == botclient.ItemRemoved {
						return true
					}
				}
			}
			return false
		})
		if err != nil {
			t.Errorf("%s destroys bought item %d: %v", b.c.Self.Name, id, err)
			continue
		}
		if m, ok := ev.(botclient.SystemMessage); ok {
			t.Errorf("%s destroys bought item %d: refused with system message %d", b.c.Self.Name, id, m.ID)
		}
	}
}

// note records ev: the merchant's Trade lines, its whispers and trade
// request.
func (b *valuablesBuyer) note(ev botclient.Event) {
	switch e := ev.(type) {
	case botclient.CreatureSay:
		if !strings.EqualFold(e.Name, b.merchant) {
			return
		}
		switch e.Type {
		case botclient.ChatTrade:
			b.ads = append(b.ads, e.Text)
		case chatTell:
			b.tells = append(b.tells, e.Text)
		}
	case botclient.SendTradeRequest:
		b.requested = b.requested || e.SenderID == b.merchantID
	}
}

// until reads the buyer's connection, noting what it hears, until cond
// holds; the step fails once d passes.
func (b *valuablesBuyer) until(t *testing.T, ctx context.Context, what string, d time.Duration, cond func() bool) {
	t.Helper()
	watchUntilBy(t, ctx, b.c, what, time.Now().Add(d), cond, b.note)
}

// await reads the buyer's connection, noting what it hears, until match
// accepts an event.
func (b *valuablesBuyer) await(t *testing.T, ctx context.Context, what string, d time.Duration, match func(botclient.Event) bool) botclient.Event {
	t.Helper()
	ev, err := b.c.Await(ctx, d, func(ev botclient.Event) bool {
		b.note(ev)
		return match(ev)
	})
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	return ev
}

// adSince is the first Trade line of the merchant from index from on that
// names item, "" for none.
func (b *valuablesBuyer) adSince(from int, item string) string {
	for _, l := range b.ads[min(from, len(b.ads)):] {
		if strings.Contains(l, item) {
			return l
		}
	}
	return ""
}

// hearAd waits for the merchant's Trade line naming item: the round's
// first line comes after the session's look and the next private open.
func (b *valuablesBuyer) hearAd(t *testing.T, ctx context.Context, item string) string {
	t.Helper()
	from := len(b.ads)
	b.until(t, ctx, b.c.Self.Name+" hears "+b.merchant+" advertise "+item, 12*time.Minute, func() bool { return b.adSince(from, item) != "" })
	return b.adSince(from, item)
}

// ask whispers text to the merchant and returns its whispered answer.
func (b *valuablesBuyer) ask(t *testing.T, ctx context.Context, text string) string {
	t.Helper()
	n := len(b.tells)
	tell(t, ctx, b.c, b.merchant, text)
	b.until(t, ctx, b.merchant+" answers "+strconv.Quote(text), 2*time.Minute, func() bool { return len(b.tells) > n })
	t.Logf("%s: %q, %s: %q", b.c.Self.Name, text, b.merchant, b.tells[n])
	return b.tells[n]
}

// buyByTrade runs the trade window the buyer accepted: the merchant offers
// the sword, the buyer adds price adena, waits for the merchant's confirm,
// confirms, and the trade settles with the sword in the buyer's inventory
// and price adena gone from it. It returns the adena paid.
func (b *valuablesBuyer) buyByTrade(t *testing.T, ctx context.Context, price int32) int32 {
	t.Helper()
	ev := b.await(t, ctx, b.merchant+"'s trade window opens", 30*time.Second, func(ev botclient.Event) bool {
		ts, ok := ev.(botclient.TradeStart)
		return ok && ts.PartnerID == b.merchantID
	})
	var coins, had int32
	for _, it := range ev.(botclient.TradeStart).Items {
		if it.TemplateID == adena {
			coins, had = it.ObjectID, it.Count
		}
	}
	if coins == 0 || had < price {
		t.Fatalf("%s offers %d adena, want %d", b.c.Self.Name, had, price)
	}
	ev = b.await(t, ctx, b.merchant+" offers its valuable", 30*time.Second, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.TradeOtherAdd)
		return ok
	})
	if got := ev.(botclient.TradeOtherAdd).Item; got.TemplateID != swordOfWatershadow || got.Count != 1 {
		t.Fatalf("%s offers %d of template %d, want one %s", b.merchant, got.Count, got.TemplateID, swordName)
	}
	if err := b.c.Send(ctx, botclient.EncodeAddTradeItem(coins, price)); err != nil {
		t.Fatal(err)
	}
	b.await(t, ctx, b.merchant+" confirms a paid trade", 30*time.Second, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.TradePressOtherOk)
		return ok
	})
	if err := b.c.Send(ctx, botclient.EncodeTradeDone(true)); err != nil {
		t.Fatal(err)
	}
	var done, sword bool
	adenaLeft := had
	b.await(t, ctx, "the trade settles", 30*time.Second, func(ev botclient.Event) bool {
		switch e := ev.(type) {
		case botclient.SendTradeDone:
			if !e.Success {
				t.Fatal("the paid trade was cancelled")
			}
			done = true
		case botclient.InventoryUpdate:
			for _, c := range e.Changes {
				switch {
				case c.Item.TemplateID == swordOfWatershadow && c.State == botclient.ItemAdded:
					sword = true
					b.bought = append(b.bought, c.Item.ObjectID)
				case c.Item.TemplateID == adena && c.State != botclient.ItemRemoved:
					adenaLeft = c.Item.Count
				case c.Item.TemplateID == adena:
					adenaLeft = 0
				}
			}
		}
		return done && sword && adenaLeft == had-price
	})
	return had - adenaLeft
}

// storeList clicks owner, which stands within reach, to select it, clicks
// it again to open its sell store (as the bot's market visit does) and
// returns the store's list.
func (b *valuablesBuyer) storeList(t *testing.T, ctx context.Context, owner int32) botclient.PrivateStoreListSell {
	t.Helper()
	if err := b.c.Send(ctx, botclient.EncodeAction(owner, b.c.Pos())); err != nil {
		t.Fatal(err)
	}
	b.await(t, ctx, b.c.Self.Name+" selects "+b.merchant, 30*time.Second, func(botclient.Event) bool { return b.c.Target() == owner })
	if err := b.c.Send(ctx, botclient.EncodeAction(owner, b.c.Pos())); err != nil {
		t.Fatal(err)
	}
	ev := b.await(t, ctx, b.c.Self.Name+" reads "+b.merchant+"'s store list", 30*time.Second, func(ev botclient.Event) bool {
		l, ok := ev.(botclient.PrivateStoreListSell)
		return ok && l.OwnerID == owner
	})
	return ev.(botclient.PrivateStoreListSell)
}

// buyFromStore buys rows, one unit each, from owner's sell store at their
// listed prices, the buyer holding coins adena (the list's BuyerAdena), and
// waits until the buyer carries each and paid. It returns the adena paid.
func (b *valuablesBuyer) buyFromStore(t *testing.T, ctx context.Context, owner int32, rows []botclient.StoreItem, coins int32) int32 {
	t.Helper()
	buy := make([]botclient.StoreBuy, len(rows))
	want := make(map[int32]bool, len(rows))
	var price int32
	for i, r := range rows {
		buy[i] = botclient.StoreBuy{ObjectID: r.ObjectID, Count: 1, Price: r.Price}
		want[r.TemplateID] = true
		price += r.Price
	}
	added, left := 0, coins
	if err := b.c.Send(ctx, botclient.EncodeRequestPrivateStoreBuy(owner, buy)); err != nil {
		t.Fatal(err)
	}
	b.await(t, ctx, fmt.Sprintf("%s buys %d valuables from the store", b.c.Self.Name, len(rows)), 30*time.Second, func(ev botclient.Event) bool {
		if sm, ok := ev.(botclient.SystemMessage); ok && (sm.ID == msgNotEnoughAdena || sm.ID == msgNotAuthorized) {
			t.Fatalf("the store refused the buy: system message %d", sm.ID)
		}
		if u, ok := ev.(botclient.InventoryUpdate); ok {
			for _, c := range u.Changes {
				switch {
				case want[c.Item.TemplateID] && c.State == botclient.ItemAdded:
					added++
					b.bought = append(b.bought, c.Item.ObjectID)
				case c.Item.TemplateID == adena && c.State != botclient.ItemRemoved:
					left = c.Item.Count
				}
			}
		}
		return added == len(rows) && left == coins-price
	})
	t.Logf("%s bought %d valuables for %d", b.c.Self.Name, len(rows), price)
	return price
}

// replyPrice reads the price of a whispered reply ending in tail, in the
// merchant's price words (100, 157k, 1.2kk): the first word that reads as
// one. word is that word as written.
func replyPrice(reply, tail string) (price int64, word string, ok bool) {
	if !strings.HasSuffix(reply, tail) {
		return 0, "", false
	}
	for _, w := range strings.Fields(strings.TrimSuffix(reply, tail)) {
		w = strings.TrimRight(w, ",")
		mult, digits := int64(1), w
		switch {
		case strings.HasSuffix(w, "kk"):
			mult, digits = 1000000, strings.TrimSuffix(w, "kk")
		case strings.HasSuffix(w, "k"):
			mult, digits = 1000, strings.TrimSuffix(w, "k")
		}
		v, err := strconv.ParseFloat(digits, 64)
		if err != nil || v <= 0 {
			continue
		}
		return int64(v * float64(mult)), w, true
	}
	return 0, "", false
}

// keeperLink is a private warehouse link the merchant sent and the
// template of the NPC its object names.
type keeperLink struct {
	text string
	npc  int32
}

// privateLinks is f's private warehouse links (npc_<object>_DepositP,
// _WithdrawP) from its sent packet index from on. It fails the scenario
// for a link that names no Rolfe: Rant's pages carry none, so the bot
// marks him and goes on to Rolfe (docs/valuables/round.md "Keepers by
// town").
func privateLinks(t *testing.T, f *farmer, from int) []keeperLink {
	t.Helper()
	npcs := make(map[int32]int32)
	for _, s := range f.watch.Seen(0) {
		if n, ok := s.Event.(botclient.NPCInfo); ok {
			npcs[n.ObjectID] = n.Template()
		}
	}
	var links []keeperLink
	for _, p := range sentOf(f.watch.Sent()[from:], opBypass) {
		text := bypassText(p.Payload)
		if !strings.Contains(text, "_DepositP") && !strings.Contains(text, "_WithdrawP") {
			continue
		}
		obj, _, _ := strings.Cut(strings.TrimPrefix(text, "npc_"), "_")
		id, _ := strconv.ParseInt(obj, 10, 32)
		l := keeperLink{text: text, npc: npcs[int32(id)]}
		links = append(links, l)
		switch l.npc {
		case keeperRolfe:
		case keeperRant:
			t.Errorf("%s sent %q to Rant (%d), a freightman", f.name, text, keeperRant)
		default:
			t.Errorf("%s sent %q to NPC template %d, want Rolfe (%d)", f.name, text, l.npc, keeperRolfe)
		}
	}
	return links
}

// hasLink reports whether a link of links holds cmd.
func hasLink(links []keeperLink, cmd string) bool {
	for _, l := range links {
		if strings.Contains(l.text, cmd) {
			return true
		}
	}
	return false
}

// bypassText is the command of a RequestBypassToServer payload: a UTF-16LE
// string after the opcode.
func bypassText(payload []byte) string {
	var b strings.Builder
	for i := 1; i+1 < len(payload); i += 2 {
		r := binary.LittleEndian.Uint16(payload[i:])
		if r == 0 {
			break
		}
		b.WriteRune(rune(r))
	}
	return b.String()
}

// wearsNeither fails the scenario when f's inventory ever showed a Sword
// of Watershadow or Tears of Eva worn: a mystic's gear errand wields
// neither (gear.go, mystic weapons; ETC weapons skipped).
func wearsNeither(t *testing.T, f *farmer) {
	t.Helper()
	for _, s := range f.watch.Seen(0) {
		var items []botclient.Item
		switch e := s.Event.(type) {
		case botclient.ItemList:
			items = e.Items
		case botclient.InventoryUpdate:
			for _, c := range e.Changes {
				items = append(items, c.Item)
			}
		}
		for _, it := range items {
			if it.Equipped && (it.TemplateID == swordOfWatershadow || it.TemplateID == tearsOfEva) {
				t.Errorf("%s wears template %d, a GM-given valuable", f.name, it.TemplateID)
				return
			}
		}
	}
}
