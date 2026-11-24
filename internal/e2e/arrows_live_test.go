package e2e

import (
	"context"
	"errors"
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
// once it carries the arrows (checked over its inventory events in
// order). Then, out of town, told to assist the GM, it shoots the monster
// the GM hits until it dies, and never reads NotEnoughArrows.
func TestLiveArrows(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &newbieSpots)
	ctx := scenarioCtx(t, 20*time.Minute)
	travel, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	var f *farmer
	t.Cleanup(func() { forgetKilled(gm, f) }) // after the archer stops, before the GM deletes
	account, _ := tg.Account("ar", 1)
	// Next to the weapon merchant, whose visit is the first.
	strip(t, ctx, tg, gm, items, account, &botclient.HumanFighter, stripEnd{at: tiWeapons, adena: gearAdena}, squiresSword, squiresShirt, squiresPants)
	minArrows := botclient.DefaultPersona().Shop.ArrowsMin
	f = runTownBot(t, ctx, tg, account, &botclient.HumanFighter, travel, func(p *botclient.Persona) {
		p.Gear.Classes = []botclient.GearClass{{Weapons: []string{"BOW"}, Armor: "LIGHT"}}
		// Bow and arrows only: no jewelry, whose merchant stands nearer
		// the trainer than the weapon merchant does and would take the
		// first visit and the bow's adena (it did on main too).
		p.Shop.Restock = nil
		p.Gear.Jewelry = false
	})
	t.Logf("%s has its adena at %v", f.name, f.bot.Status().Position)

	// The bow at the weapon merchant, then, five minutes on (shopRetry),
	// the arrows next to the grocer: the village's merchants stand too
	// far apart for the merchant in sight to be the right one.
	wctx, cancel := context.WithTimeout(ctx, 2*gearBound)
	defer cancel()
	until(t, wctx, f.name+" buys a bow", func() string {
		for _, it := range inventory(f.watch.Seen(0)) {
			if items[it.TemplateID].Kind == "BOW" {
				return ""
			}
		}
		return fmt.Sprintf("no bow (rule %q)", f.bot.Status().Rule)
	})
	gmTeleport(t, ctx, gm, tiGrocery)
	recallBot(t, ctx, tg, gm, f.name)

	var bow, arrows int32
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
	// #85: the bow goes on only once arrows of its grade are carried.
	inv := make(map[int32]botclient.Item)
	for _, s := range f.watch.Seen(0) {
		applyInventory(inv, s.Event)
		var worn int32
		for _, it := range inv {
			if it.Equipped && items[it.TemplateID].Kind == "BOW" {
				worn = it.TemplateID
			}
		}
		if worn == 0 {
			continue
		}
		var carried int32
		for _, it := range inv {
			if items[it.TemplateID].Kind == "ARROW" && items[it.TemplateID].Grade == items[worn].Grade {
				carried += it.Count
			}
		}
		if carried == 0 {
			t.Errorf("%s wore %s at %s with none of its arrows", f.name, items[worn].Name, s.At.Format("15:04:05.000"))
		}
		break
	}
	if n := len(sentOf(f.watch.Sent(), botclient.OpRequestBuyItem)); n < 2 {
		t.Errorf("%d RequestBuyItem: want the bow's and the arrows' at least", n)
	}
	armed := len(f.watch.Seen(0))

	// Out of the peace zone, where the GM is seen (//hide off): a bot
	// assists only a player in its sight.
	gmVisibleAt(t, ctx, gm, tiSpots)
	from := len(f.watch.Seen(0))
	recallBot(t, ctx, tg, gm, f.name)
	// The GM is in the archer's sight once its CharInfo reached the bot:
	// the tap sees the frame just before the brain does.
	sctx, cancel := context.WithTimeout(ctx, EntryTimeout)
	defer cancel()
	if _, _, err := f.watch.Await(sctx, from, charInfoOf(gm.Self.ObjectID)); err != nil {
		t.Fatalf("%s never saw the GM: %v", f.name, err)
	}
	assist := botclient.Command{Kind: botclient.CommandAssist, Arg: gm.Self.Name}
	var err error
	for range 5 {
		var answer <-chan error
		if answer, err = f.bot.Command(assist); err == nil {
			select {
			case err = <-answer:
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		if !errors.Is(err, botclient.ErrNotInSight) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("%s told %q: %v", f.name, assist, err)
	}
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
