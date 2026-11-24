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

// gearAdena is the adena a buying scenario's bot carries at least when it
// logs in (stripEnd).
const gearAdena int32 = 30000

// warriorGear pins the persona's class table to the entry a warrior
// (class 1) plays by default, the build's: swords, blunts and their
// two-handed kinds, with a shield. The scenario's character is a human
// fighter (class 0): the GM changes no class, and the table is a persona
// default (D6). The default persona holds no table of its own since the
// builds (S4, #323): the warrior's build gives the entry.
func warriorGear(p *botclient.Persona) {
	g := botclient.CompileBuild(0, 1, 20, botclient.RoleDamage, botclient.BranchChoice{}).Kit.Gear
	if len(g.Weapons) == 0 {
		panic("the warrior's build wields nothing")
	}
	p.Gear.Classes = []botclient.GearClass{g}
}

// TestLiveGearWear runs a fighter that wears the warrior's gear in the
// Talking Island village, stripped to its starting set (#79): given a
// shield, a necklace and two Magic Rings, it wears all four, the rings on
// both fingers; given a Ring of Knowledge, it takes a Magic Ring off and
// wears that ring; given a two-handed sword, it wears it and the server
// takes the shield off.
func TestLiveGearWear(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 15*time.Minute)
	travel, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("gw", 1)
	strip(t, ctx, tg, gm, items, account, &botclient.HumanFighter, stripEnd{}, squiresSword, squiresShirt, squiresPants)
	f := runTownBot(t, ctx, tg, account, &botclient.HumanFighter, travel, func(p *botclient.Persona) {
		warriorGear(p)
		// Wearing only: what it is given, never what it could buy.
		p.Gear.Buy, p.Shop = false, botclient.ShopPolicy{}
	})
	toTown(t, ctx, tg, gm, f)

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
// starting sets, the first one with a Short Sword, a Shirt and Pants for
// its Squire's set, which no merchant buys back. That one plays the
// default persona (#40): it buys a better piece for a slot of its set,
// wears it, and sells the piece it took off. The other plays a rogue's
// class table (#79: daggers in light armor, class 7) unarmed: it buys a
// dagger and wears it.
func TestLiveGearBuy(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 20*time.Minute)
	travel, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	buyerAcc, _ := tg.Account("gb", 1)
	rogueAcc, _ := tg.Account("gb", 2)
	// The buyer wears pieces a merchant buys back once they are replaced
	// (live, 30000 adena buys it better armor before a better sword).
	strip(t, ctx, tg, gm, items, buyerAcc, &botclient.HumanFighter, stripEnd{at: tiArmory, adena: gearAdena}, shortSword, shirt, pants)
	// The rogue's first visit is the weapon merchant's: the nearer one.
	strip(t, ctx, tg, gm, items, rogueAcc, &botclient.HumanFighter, stripEnd{at: tiWeapons, adena: gearAdena}, squiresShirt, squiresPants)
	starting := []int32{shortSword, shirt, pants}

	buyer := runTownBot(t, ctx, tg, buyerAcc, &botclient.HumanFighter, travel, func(p *botclient.Persona) {
		// Gear only, and the piece it takes off sold at the next visit.
		// No jewelry: from the trainer both bots visit first, the
		// jewelry merchant stands nearest, and its visits took the
		// scenario's time (on main d9410f0 too).
		p.Shop.Restock, p.Shop.SellAt = nil, 1
		p.Gear.Jewelry = false
	})
	rogue := runTownBot(t, ctx, tg, rogueAcc, &botclient.HumanFighter, travel, func(p *botclient.Persona) {
		p.Gear.Classes = []botclient.GearClass{{Weapons: []string{"DAGGER"}, Armor: "LIGHT"}}
		p.Shop.Restock = nil
		p.Gear.Jewelry = false
	})
	for _, f := range []*farmer{buyer, rogue} {
		t.Logf("%s has its adena at %v", f.name, f.bot.Status().Position)
	}

	var replaced, sold []string
	untilBy(t, ctx, buyer.name+" buys, wears and sells", time.Now().Add(2*gearBound), func() string {
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
