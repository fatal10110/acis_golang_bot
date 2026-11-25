package e2e

import (
	"context"
	"encoding/binary"
	"fmt"
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
	gmGive(t, ctx, gm, f, swordOfWatershadow, 1)

	ad := b.hearAd(t, ctx, swordName)
	t.Logf("%s heard %s: %q", b.c.Self.Name, f.name, ad)
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
// the sword gone); the GM gives it Tears of Eva; its next round withdraws
// the sword at Rolfe, and one Trade line the buyer hears names both. No
// private link names Rant.
func TestLiveValuablesWarehouse(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 30*time.Minute)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("vm", 2)
	f := valuablesMerchant(t, ctx, tg, gm, account, func(v *botclient.ValuablePolicy) {
		// Persona defaults (D6): two rounds within the scenario.
		v.RoundGap, v.RoundWait, v.Grace = 3*time.Minute, time.Minute, 30*time.Second
	})
	gmGiveAdena(t, ctx, gm, f, 1000)
	b := enterBuyer(t, ctx, tg, gm, f)
	from, sent := f.seen(), len(f.watch.Sent())
	gmGive(t, ctx, gm, f, swordOfWatershadow, 1)

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

	gmOn(t, ctx, gm, f)
	lines, sent := len(b.ads), len(f.watch.Sent())
	gmGive(t, ctx, gm, f, tearsOfEva, 1)
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
// then releases both to its store: it deposits nothing and lists them; the
// buyer buys the sword there at the listed price.
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
	gmGive(t, ctx, gm, f, swordOfWatershadow, 1)
	gmGive(t, ctx, gm, f, tearsOfEva, 1)

	b.until(t, ctx, b.c.Self.Name+" hears the round's Trade line", 12*time.Minute, func() bool { return b.adSince(0, swordName) != "" })
	if _, err := gm.Await(ctx, 8*time.Minute, storeOf(f.id)); err != nil {
		t.Fatalf("the GM sees %s's store open after the round (rule %q): %v", f.name, f.bot.Status().Rule, err)
	}
	gmTo(t, ctx, gm, f)
	if hasLink(privateLinks(t, f, sent), "_DepositP") {
		t.Error("the merchant deposited a released valuable")
	}

	recallClient(t, ctx, gm, b.c)
	list := b.storeList(t, ctx, f.id)
	var row botclient.StoreItem
	for i, it := range list.Items {
		if it.TemplateID == swordOfWatershadow {
			row = it
		}
		if i < 2 && it.TemplateID != swordOfWatershadow && it.TemplateID != tearsOfEva {
			t.Errorf("store row %d lists template %d ahead of a released valuable: %+v", i, it.TemplateID, list.Items)
		}
	}
	if row.ObjectID == 0 {
		t.Fatalf("%s's store lists no %s: %+v", f.name, swordName, list.Items)
	}
	if row.ReferencePrice > 0 && row.Price < row.ReferencePrice {
		t.Errorf("%s lists the sword at %d, under its reference price %d", f.name, row.Price, row.ReferencePrice)
	}
	t.Logf("%s's store: %+v", f.name, list.Items)
	b.buyFromStore(t, ctx, f.id, row)
	waitFor(t, ctx, f.name+" is paid for the sword", func() bool {
		run := f.counts(adena, from)
		return run[len(run)-1]-run[0] == row.Price
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
	gmOn(t, ctx, gm, f)
	return f
}

// valuablesBuyer is the buyer's client and what it heard from the merchant.
type valuablesBuyer struct {
	c          *Client
	merchant   string
	merchantID int32
	ads, tells []string
	requested  bool
	inv        map[int32]botclient.Item
}

// enterBuyer enters the buyer (<prefix>vb1, buyerCharacter) next to the GM,
// near the merchant f, with adena for one valuable.
func enterBuyer(t *testing.T, ctx context.Context, tg *Target, gm *GMClient, f *farmer) *valuablesBuyer {
	t.Helper()
	account, _ := tg.Account("vb", 1)
	c := enter(t, ctx, tg, account, buyerCharacter(t, account), nil)
	recallClient(t, ctx, gm, c)
	b := &valuablesBuyer{c: c, merchant: f.name, merchantID: f.id, inv: make(map[int32]botclient.Item)}
	if have := b.carried(adena); have < valuablesBuyerAdena {
		giveClient(t, ctx, gm, c, adena, valuablesBuyerAdena-have)
	}
	return b
}

// note records ev: the merchant's Trade lines, its whispers and trade
// request, and the buyer's inventory.
func (b *valuablesBuyer) note(ev botclient.Event) {
	applyInventory(b.inv, ev)
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

// carried is the buyer's count of tmpl as far as it read.
func (b *valuablesBuyer) carried(tmpl int32) int32 { return carriedOf(b.inv, tmpl) }

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

// buyFromStore buys one unit of row from owner's sell store at its listed
// price and waits until the buyer carries it and paid.
func (b *valuablesBuyer) buyFromStore(t *testing.T, ctx context.Context, owner int32, row botclient.StoreItem) {
	t.Helper()
	before := b.carried(row.TemplateID)
	coins := b.carried(adena)
	if err := b.c.Send(ctx, botclient.EncodeRequestPrivateStoreBuy(owner, []botclient.StoreBuy{{ObjectID: row.ObjectID, Count: 1, Price: row.Price}})); err != nil {
		t.Fatal(err)
	}
	b.await(t, ctx, b.c.Self.Name+" buys the "+swordName+" from the store", 30*time.Second, func(ev botclient.Event) bool {
		if sm, ok := ev.(botclient.SystemMessage); ok && (sm.ID == msgNotEnoughAdena || sm.ID == msgNotAuthorized) {
			t.Fatalf("the store refused the buy: system message %d", sm.ID)
		}
		return b.carried(row.TemplateID) > before && b.carried(adena) == coins-row.Price
	})
	t.Logf("%s bought the %s for %d", b.c.Self.Name, swordName, row.Price)
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
