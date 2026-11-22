package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// Supplies the scenarios seed (docs/items.md section 4, defaultSupplyPolicy):
// tripAdena pays a trip's no-grade shots for a Squire's Sword (about 3300 at
// 7 adena), its Lesser Healing Potions and its Scrolls of Escape past the
// reserve; tripPotions is a trip's potions (10 an hour, a fifth more);
// lowShots runs out within ReturnMinutes (10) at the sword's first rate
// (45 a minute) but stays above the persona's Refill of 1 % of the trip
// target, so the supply trip fires and the shop errand does not.
const (
	tripAdena   = 30000
	tripPotions = 12
	lowShots    = 200
	// escapes is DefaultPersona's Supply.Escapes.
	escapes = 2
	// buyLoad is DefaultPersona's Supply.BuyLoad, in percent.
	buyLoad = 45
)

// TestLiveTownPrep logs a farmer stripped of every consumable in next to
// Talking Island's grocer with adena (#170, #182): before it claims a
// spot, its town preparation buys no-grade shots, Lesser Healing Potions
// and two Scrolls of Escape up to its trip targets, under 45 % of its weight
// limit; it then farms, the scrolls kept, never sold back.
func TestLiveTownPrep(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &newbieSpots)
	ctx := scenarioCtx(t, 15*time.Minute)
	_, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("tp", 1)
	strip(t, ctx, tg, gm, with(items, consumables...), account, &botclient.HumanFighter, stripEnd{at: nearGrocer, adena: tripAdena}, squiresSword, squiresShirt, squiresPants)
	f := runFieldFarmer(t, ctx, tg, account, func(p *botclient.Persona) {
		p.Potions = []int32{lesserHealingPotion}
	})

	var farmAt time.Time
	until(t, ctx, "the town preparation, then a spot", func() string {
		sent := sentOf(f.watch.Sent(), botclient.OpRequestBuyItem)
		switch {
		case len(sent) == 0:
			return fmt.Sprintf("no RequestBuyItem (rule %q)", f.bot.Status().Rule)
		case f.carries(soulshotNG) == 0:
			return "no shots bought"
		case f.carries(lesserHealingPotion) == 0:
			return "no potions bought"
		case f.carries(scrollOfEscape) < escapes:
			return fmt.Sprintf("%d Scrolls of Escape", f.carries(scrollOfEscape))
		case f.bot.Status().Rule != "farm":
			return fmt.Sprintf("not farming (rule %q)", f.bot.Status().Rule)
		}
		farmAt = time.Now()
		return ""
	})
	buys := sentOf(f.watch.Sent(), botclient.OpRequestBuyItem)
	if last := buys[len(buys)-1].At; last.After(farmAt) {
		t.Errorf("%s bought at %v, after it farmed at %v", f.name, last, farmAt)
	}
	if cur, limit := f.load(); limit == 0 || int64(cur)*100 > int64(limit)*buyLoad {
		t.Errorf("%s carries %d of %d, over %d %%", f.name, cur, limit, buyLoad)
	}
	t.Logf("%s farms with %d shots, %d potions, %d scrolls (%d buys)", f.name,
		f.carries(soulshotNG), f.carries(lesserHealingPotion), f.carries(scrollOfEscape), len(buys))
	// Farming a while does not sell or read a scroll.
	waitFor(t, ctx, "a minute of farming", func() bool { return time.Since(farmAt) > time.Minute })
	if soldTemplate(f.watch.Seen(0), f.watch.Sent(), scrollOfEscape) || f.carries(scrollOfEscape) < escapes {
		t.Errorf("%s kept %d Scrolls of Escape", f.name, f.carries(scrollOfEscape))
	}
}

// TestLiveSupplyTrip has a farmer at its spot given potions, two Scrolls
// of Escape, adena and shots that run out within ten minutes (#175, #182):
// it reads a scroll, lands in Talking Island village, restocks its shots
// and scrolls before it leaves, then farms at a spot again.
func TestLiveSupplyTrip(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &newbieSpots)
	ctx := scenarioCtx(t, 20*time.Minute)
	_, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("su", 1)
	strip(t, ctx, tg, gm, with(items, consumables...), account, &botclient.HumanFighter, stripEnd{}, squiresSword, squiresShirt, squiresPants)
	f := runFieldFarmer(t, ctx, tg, account, func(p *botclient.Persona) {
		// Every value is a persona default (D6): at its spot the shop
		// errand wants no supply until 1 % of its trip target is left,
		// no gear and no sale, so the supply trip is what sends it home.
		p.Supply.Refill = 1
		p.Gear.Buy = false
		p.Shop.SellAt = 0
		p.Potions = []int32{lesserHealingPotion}
	})
	waitFor(t, ctx, "the farmer at its spot", func() bool { return f.bot.Status().Rule == "farm" })
	gmOn(t, ctx, gm, f)
	gmGive(t, ctx, gm, f, lesserHealingPotion, tripPotions)
	gmGive(t, ctx, gm, f, scrollOfEscape, escapes)
	gmGiveAdena(t, ctx, gm, f, tripAdena)
	start := time.Now()
	gmGive(t, ctx, gm, f, soulshotNG, lowShots)
	gmTeleport(t, ctx, gm, townTI)
	t.Logf("%s low on shots at %v", f.name, f.bot.Status().Position)

	var read, landed time.Time
	until(t, ctx, "the Scroll of Escape read and the landing", func() string {
		used := f.usedSince(scrollOfEscape, start)
		if len(used) == 0 {
			return fmt.Sprintf("no scroll read (rule %q)", f.bot.Status().Rule)
		}
		read = used[0].At
		for _, s := range f.watch.Seen(0) {
			if tp, ok := s.Event.(botclient.TeleportToLocation); ok && tp.ObjectID == f.id && s.At.After(read) {
				landed = s.At
				return ""
			}
		}
		return "no landing"
	})
	t.Logf("%s read its scroll at %v, landed at %v", f.name, read.Format(time.TimeOnly), landed.Format(time.TimeOnly))
	from := 0
	for i, s := range f.watch.Seen(0) {
		if !s.At.Before(landed) {
			from = i
			break
		}
	}
	var farmAt time.Time
	until(t, ctx, "the restock, then a spot", func() string {
		var bought bool
		for _, p := range sentOf(f.watch.Sent(), botclient.OpRequestBuyItem) {
			bought = bought || p.At.After(landed)
		}
		switch {
		case !bought:
			return fmt.Sprintf("no RequestBuyItem since the landing (rule %q)", f.bot.Status().Rule)
		case !rose(f.counts(soulshotNG, from)):
			return "no shots bought"
		case f.carries(scrollOfEscape) < escapes:
			return fmt.Sprintf("%d Scrolls of Escape", f.carries(scrollOfEscape))
		case f.bot.Status().Rule != "farm":
			return fmt.Sprintf("not farming (rule %q)", f.bot.Status().Rule)
		}
		farmAt = time.Now()
		return ""
	})
	for _, p := range sentOf(f.watch.Sent(), botclient.OpRequestBuyItem) {
		if p.At.After(farmAt) {
			t.Errorf("%s bought at %v, after it farmed again at %v", f.name, p.At, farmAt)
		}
	}
	t.Logf("%s farms again with %d shots (%v), %d scrolls", f.name, f.carries(soulshotNG), f.counts(soulshotNG, from), f.carries(scrollOfEscape))
}

// load is f's carried weight and weight limit as its last UserInfo or
// StatusUpdate showed them.
func (f *farmer) load() (cur, limit int32) {
	for _, s := range f.watch.Seen(0) {
		switch e := s.Event.(type) {
		case botclient.UserInfo:
			cur, limit = e.Load, e.MaxLoad
		case botclient.StatusUpdate:
			if e.ObjectID != f.id {
				continue
			}
			if e.Set&botclient.StatusLoad != 0 {
				cur = e.Load
			}
			if e.Set&botclient.StatusMaxLoad != 0 {
				limit = e.MaxLoad
			}
		}
	}
	return cur, limit
}
