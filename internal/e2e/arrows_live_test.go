package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveArrows runs a fighter that plays an archer's class table (#85,
// #102: bows in light armor, classes 9, 24, 37 and their third classes)
// with adena in the Talking Island village, stripped to its starting set:
// it wields a sword, a weapon type an archer does not. It buys a bow and
// the arrows of the bow's grade, at two merchants, and wears the bow only
// once it carries the arrows. Then, out of town, told to assist the GM, it
// shoots the monster the GM hits until it dies, and never reads
// NotEnoughArrows.
func TestLiveArrows(t *testing.T) {
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 20*time.Minute)
	travel, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	var f *farmer
	t.Cleanup(func() { forgetKilled(gm, f) }) // after the archer stops, before the GM deletes
	account, _ := tg.Account("ar", 1)
	strip(t, ctx, tg, gm, items, account, &botclient.HumanFighter, squiresSword, squiresShirt, squiresPants)
	minArrows := botclient.DefaultPersona().Shop.ArrowsMin
	f = runTownBot(t, ctx, tg, account, &botclient.HumanFighter, travel, func(p *botclient.Persona) {
		p.Gear.Classes = []botclient.GearClass{{Weapons: []string{"BOW"}, Armor: "LIGHT"}}
		// Bow and arrows only.
		p.Shop.Restock = nil
	})
	gmRecall(t, ctx, gm, f, tiMarketWay)
	gmOn(t, ctx, gm, f)
	gmAdena(t, ctx, gm, f, 3)
	t.Logf("%s has its adena at %v", f.name, f.bot.Status().Position)

	var bow, arrows int32
	wctx, cancel := context.WithTimeout(ctx, 2*gearBound)
	defer cancel()
	until(t, wctx, f.name+" buys a bow and its arrows and wears the bow", func() string {
		inv := inventory(f.watch.Seen(0))
		bow, arrows = 0, 0
		for _, it := range inv {
			if it.Equipped && items[it.TemplateID].Kind == "BOW" {
				bow = it.TemplateID
			}
		}
		for _, it := range inv {
			if items[it.TemplateID].Kind == "ARROW" && items[it.TemplateID].Grade == items[bow].Grade {
				arrows += it.Count
			}
		}
		if bow == 0 || worn(inv)[squiresSword] != 0 {
			return fmt.Sprintf("wears %s (rule %q)", wornNames(inv, items), f.bot.Status().Rule)
		}
		if arrows < minArrows {
			return fmt.Sprintf("wears %s with %d arrows", items[bow].Name, arrows)
		}
		return ""
	})
	t.Logf("%s wears %s and carries %d of its arrows", f.name, items[bow].Name, arrows)
	if n := len(sentOf(f.watch.Sent(), botclient.OpRequestBuyItem)); n < 2 {
		t.Errorf("%d RequestBuyItem: want the bow's and the arrows' at least", n)
	}
	armed := len(f.watch.Seen(0))

	// Out of the peace zone, where the GM is seen (//hide off): a bot
	// assists only a player in its sight.
	gmVisibleAt(t, ctx, gm, tiSpots)
	if err := gm.Command(ctx, "recall "+f.name); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(f.id)); err != nil {
		t.Fatalf("%s recalled: %v", f.name, err)
	}
	order(t, ctx, f, botclient.Command{Kind: botclient.CommandAssist, Arg: gm.Self.Name})
	mob, err := gm.Spawn(ctx, wolf)
	if err != nil {
		t.Fatal(err)
	}
	gmProvoke(t, ctx, gm, mob)
	awaitDeath(t, ctx, gm, f, mob)

	shots := 0
	for _, s := range f.watch.Seen(armed) {
		switch e := s.Event.(type) {
		case botclient.Attack:
			if e.AttackerID == f.id && len(e.Hits) > 0 && e.Hits[0].TargetID == mob {
				shots++
			}
		case botclient.SystemMessage:
			if e.ID == msgNotEnoughArrows {
				t.Errorf("%s read NotEnoughArrows", f.name)
			}
		}
	}
	if shots == 0 {
		t.Errorf("%s never shot the wolf", f.name)
	}
	t.Logf("%s shot the wolf %d times", f.name, shots)
}
