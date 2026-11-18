package e2e

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// gearBound bounds each wait on a bot's gear errand: a UseItem takes a
// second, a merchant visit in the village a minute or two.
const gearBound = 5 * time.Minute

// warriorGear is the persona's class table cut to the entry a warrior
// (class 1) plays by default, the one of every fighter the table does not
// name: swords, blunts and their two-handed kinds, with a shield. The
// scenario's character is a human fighter (class 0): the GM changes no
// class, and the table is a persona default (D6).
func warriorGear(p *botclient.Persona) {
	for _, c := range p.Gear.Classes {
		if len(c.Classes) == 0 {
			p.Gear.Classes = []botclient.GearClass{c}
			return
		}
	}
	panic("the default class table has no catch-all entry")
}

// TestLiveGearWear runs a fighter that wears the warrior's gear in the
// Talking Island village, stripped to its starting set (#79): given a
// shield, a necklace and two Magic Rings, it wears all four, the rings on
// both fingers; given a Ring of Knowledge, it takes a Magic Ring off and
// wears that ring; given a two-handed sword, it wears it and the server
// takes the shield off.
func TestLiveGearWear(t *testing.T) {
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 15*time.Minute)
	travel, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("gw", 1)
	strip(t, ctx, tg, gm, items, account, &botclient.HumanFighter, squiresSword, squiresShirt, squiresPants)
	f := runTownBot(t, ctx, tg, account, &botclient.HumanFighter, travel, func(p *botclient.Persona) {
		warriorGear(p)
		// Wearing only: what it is given, never what it could buy.
		p.Gear.Buy, p.Shop = false, botclient.ShopPolicy{}
	})
	gmRecall(t, ctx, gm, f, townTI)

	gmOn(t, ctx, gm, f)
	gmGiveGear(t, ctx, gm, f, smallShield, 1)
	gmGiveGear(t, ctx, gm, f, necklaceOfMagic, 1)
	gmGiveGear(t, ctx, gm, f, magicRing, 2)
	wears(t, ctx, f, items, "shield, necklace and both rings", func(w map[int32]int) bool {
		return w[smallShield] == 1 && w[necklaceOfMagic] == 1 && w[magicRing] == 2 && w[squiresSword] == 1
	})

	gmGiveGear(t, ctx, gm, f, ringOfKnowledge, 1)
	wears(t, ctx, f, items, "a Ring of Knowledge for a Magic Ring", func(w map[int32]int) bool {
		return w[ringOfKnowledge] == 1 && w[magicRing] == 1
	})

	gmGiveGear(t, ctx, gm, f, redSunsetSword, 1)
	wears(t, ctx, f, items, "the two-handed sword, the shield taken off", func(w map[int32]int) bool {
		return w[redSunsetSword] == 1 && w[smallShield] == 0 && w[squiresSword] == 0
	})
}

// wears waits up to gearBound until what f wears, by template, passes ok.
func wears(t *testing.T, ctx context.Context, f *farmer, items map[int32]datapack.Item, what string, ok func(map[int32]int) bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, gearBound)
	defer cancel()
	until(t, ctx, f.name+" wears "+what, func() string {
		inv := inventory(f.watch.Seen(0))
		if ok(worn(inv)) {
			return ""
		}
		return fmt.Sprintf("wears %s (rule %q)", wornNames(inv, items), f.bot.Status().Rule)
	})
	t.Logf("%s wears %s: %s", f.name, what, wornNames(inventory(f.watch.Seen(0)), items))
}

// TestLiveGearBuy runs two fighters with adena in the Talking Island
// village next to the weapon and armor merchants, stripped to their
// starting sets. One plays the default persona (#40): it buys a better
// piece for a slot of its starting set, wears it, and sells the piece it
// took off. The other plays a rogue's class table (#79: daggers in light
// armor, class 7) unarmed: it buys a dagger and wears it.
func TestLiveGearBuy(t *testing.T) {
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 20*time.Minute)
	travel, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	buyerAcc, _ := tg.Account("gb", 1)
	rogueAcc, _ := tg.Account("gb", 2)
	strip(t, ctx, tg, gm, items, buyerAcc, &botclient.HumanFighter, squiresSword, squiresShirt, squiresPants)
	strip(t, ctx, tg, gm, items, rogueAcc, &botclient.HumanFighter, squiresShirt, squiresPants)
	starting := []int32{squiresSword, squiresShirt, squiresPants}

	buyer := runTownBot(t, ctx, tg, buyerAcc, &botclient.HumanFighter, travel, func(p *botclient.Persona) {
		// Gear only, and the piece it takes off sold at the next visit.
		p.Shop.Restock, p.Shop.SellAt = nil, 1
	})
	rogue := runTownBot(t, ctx, tg, rogueAcc, &botclient.HumanFighter, travel, func(p *botclient.Persona) {
		p.Gear.Classes = []botclient.GearClass{{Weapons: []string{"DAGGER"}, Armor: "LIGHT"}}
		p.Shop.Restock = nil
	})
	for _, f := range []*farmer{buyer, rogue} {
		gmRecall(t, ctx, gm, f, tiArmory)
		gmOn(t, ctx, gm, f)
		gmAdena(t, ctx, gm, f, 3)
	}

	bctx, cancel := context.WithTimeout(ctx, 2*gearBound)
	defer cancel()
	var replaced, sold []string
	until(t, bctx, buyer.name+" buys, wears and sells", func() string {
		seen, sent := buyer.watch.Seen(0), buyer.watch.Sent()
		inv := inventory(seen)
		w := worn(inv)
		replaced, sold = nil, nil
		for _, tmpl := range starting {
			if w[tmpl] != 0 {
				continue
			}
			part := items[tmpl].BodyPart
			if !slices.ContainsFunc(slices.Collect(maps.Values(inv)), func(it botclient.Item) bool {
				return it.Equipped && items[it.TemplateID].BodyPart == part
			}) {
				continue
			}
			replaced = append(replaced, items[tmpl].Name)
			if soldTemplate(seen, sent, tmpl) {
				sold = append(sold, items[tmpl].Name)
			}
		}
		switch {
		case len(sentOf(sent, botclient.OpRequestBuyItem)) == 0:
			return fmt.Sprintf("no RequestBuyItem (rule %q)", buyer.bot.Status().Rule)
		case len(replaced) == 0:
			return fmt.Sprintf("wears %s", wornNames(inv, items))
		case len(sold) == 0:
			return fmt.Sprintf("took off %v, sold none of it (rule %q)", replaced, buyer.bot.Status().Rule)
		}
		return ""
	})
	t.Logf("%s wears %s; replaced %v, sold %v", buyer.name, wornNames(inventory(buyer.watch.Seen(0)), items), replaced, sold)

	// It may wear the Dagger every human fighter carries first.
	had := atLogin(rogue)
	wears(t, ctx, rogue, items, "a dagger it bought", func(w map[int32]int) bool {
		for id := range w {
			if items[id].Kind == "DAGGER" && carriedOf(had, id) == 0 {
				return len(sentOf(rogue.watch.Sent(), botclient.OpRequestBuyItem)) > 0
			}
		}
		return false
	})
}

// soldTemplate reports whether a RequestSellItem of sent named an item of
// template tmpl that the inventory seen leaves is gone.
func soldTemplate(seen []Seen, sent []Sent, tmpl int32) bool {
	objects := make(map[int32]bool)
	for _, s := range seen {
		one := make(map[int32]botclient.Item)
		applyInventory(one, s.Event)
		for id, it := range one {
			if it.TemplateID == tmpl {
				objects[id] = true
			}
		}
	}
	inv := inventory(seen)
	for _, p := range sentOf(sent, botclient.OpRequestSellItem) {
		for _, id := range soldObjects(p) {
			if _, carried := inv[id]; objects[id] && !carried {
				return true
			}
		}
	}
	return false
}

// atLogin is the inventory f's first ItemList shows.
func atLogin(f *farmer) map[int32]botclient.Item {
	inv := make(map[int32]botclient.Item)
	for _, s := range f.watch.Seen(0) {
		if l, ok := s.Event.(botclient.ItemList); ok {
			applyInventory(inv, l)
			break
		}
	}
	return inv
}
