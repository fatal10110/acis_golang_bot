package e2e

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// The store a second client opens for TestLivePvPCPStore: CP Potions at
// their reference price 240 and Greater CP Potions at 800 of 600, both
// under the market cap (MarketPolicy.MaxOverReference 150: 360 and 900).
// A Greater one is the cheaper per CP (200 for each CP Potion it counts
// for), so a bot short of its whole stock of 10 buys 2 Greater ones and
// then 2 CP Potions.
const (
	cpPotion        int32 = 5591
	greaterCPPotion int32 = 5592
	cpStorePlain          = 240
	cpStoreGreater        = 800
	cpStoreCount          = 5
	cpBuyerAdena          = 10000
	cpBuyerShots          = 2000
)

// TestLivePvPCPStore has a second client open a private sell store on open
// ground listing CP potions the GM gave it (#612, D43). A bot stocking 10
// CP Potions (PvPPolicy.CPStock, the pvp preset's) and carrying none logs
// in next to it: it buys the Greater ones first, the cheaper per CP, then
// CP Potions for the rest of its stock, at the listed prices, and no more.
func TestLivePvPCPStore(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 12*time.Minute)
	travel, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	buyerAcc, _ := tg.Account("pcs", 1)
	sellerAcc, _ := tg.Account("pcs", 2)
	// Shots past their trip target: CP potions are the only thing it wants.
	strip(t, ctx, tg, gm, with(items, soulshotNG, cpPotion, greaterCPPotion, adena), buyerAcc, &botclient.HumanFighter,
		stripEnd{at: openGround, adena: cpBuyerAdena, carry: map[int32]int32{soulshotNG: cpBuyerShots}}, squiresSword, squiresShirt, squiresPants)

	seller := enter(t, ctx, tg, sellerAcc, character(t, sellerAcc, nil), nil)
	recallClient(t, ctx, gm, seller)
	giveClient(t, ctx, gm, seller, cpPotion, cpStoreCount)
	giveClient(t, ctx, gm, seller, greaterCPPotion, cpStoreCount)
	openSellStore(t, ctx, gm, seller, map[int32]int32{cpPotion: cpStorePlain, greaterCPPotion: cpStoreGreater}, cpStoreCount)
	t.Logf("%s sells %d CP Potions at %d and %d Greater ones at %d", seller.Self.Name, cpStoreCount, cpStorePlain, cpStoreCount, cpStoreGreater)

	buyer := runTownBot(t, ctx, tg, buyerAcc, &botclient.HumanFighter, travel, func(p *botclient.Persona) {
		// No merchant visit and no other supply: the store is the errand.
		p.Shop.Reach = 0
		p.Gear.Buy = false
		p.Potions, p.Supply.Escapes = nil, 0
		p.PvP = botclient.PvPPolicy{FightFlagged: true, CPStock: 10, KeepCPPotions: true}
	})
	from := buyer.seen()
	untilServed(t, ctx, buyer.name+" buys its CP stock", buyer.watch, storeBuyRefused(buyer.watch, seller.Self.ObjectID), func() string {
		switch {
		case len(sentOf(buyer.watch.Sent(), botclient.OpRequestPrivateStoreBuy)) == 0:
			return fmt.Sprintf("no RequestPrivateStoreBuy (rule %q)", buyer.bot.Status().Rule)
		case buyer.carries(greaterCPPotion) < 2 || buyer.carries(cpPotion) < 2:
			return fmt.Sprintf("Greater CP Potions %d, CP Potions %d", buyer.carries(greaterCPPotion), buyer.carries(cpPotion))
		}
		return ""
	})
	for _, p := range sentOf(buyer.watch.Sent(), botclient.OpRequestPrivateStoreBuy) {
		if p.Target != seller.Self.ObjectID {
			t.Errorf("%s bought from %d, want %s's store %d", buyer.name, p.Target, seller.Self.Name, seller.Self.ObjectID)
		}
	}
	if g, c := buyer.carries(greaterCPPotion), buyer.carries(cpPotion); g != 2 || c != 2 {
		t.Errorf("%s carries %d Greater CP Potions and %d CP Potions, want 2 and 2 (a stock of 10)", buyer.name, g, c)
	}
	spent := buyer.counts(adena, from)
	// The counts start at 0 when from comes before the ItemList: the
	// most it held is what it had before it paid.
	if want := int32(2*cpStoreGreater + 2*cpStorePlain); slices.Max(spent)-spent[len(spent)-1] != want {
		t.Errorf("%s paid %d adena (%v), want %d", buyer.name, slices.Max(spent)-spent[len(spent)-1], spent, want)
	}
	t.Logf("%s bought CP potions for adena %v", buyer.name, spent)
}
