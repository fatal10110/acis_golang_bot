package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// slotsNear is botclient's bagNear: a bag within it of its inventory limit
// sells its junk.
const slotsNear = 5

// TestLiveSlotLimit runs a level 1 fighter that picks up any ground item
// in town, whose bag a GM fills to the inventory limit the server sends in
// UserInfo with Magic Rings, unworn and not stacking (#528): the bot clicks
// no ground item the server refuses for want of a slot (SystemMessage
// 129), wears two rings and sells the rest, and once under the limit
// picks up the drop of a monster the GM kills next to it again.
func TestLiveSlotLimit(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 20*time.Minute)
	travel, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	var f *farmer
	t.Cleanup(func() { forgetKilled(gm, f) })
	account, _ := tg.Account("sl", 1)
	strip(t, ctx, tg, gm, items, account, &botclient.HumanFighter, stripEnd{level: 1}, squiresSword, squiresShirt, squiresPants)
	f = runTownBot(t, ctx, tg, account, &botclient.HumanFighter, travel, func(p *botclient.Persona) {
		fighterGear(p)
		p.Gear.Buy = false
		p.Loot.Ground = botclient.GroundAny
		p.Loot.StopAbove = 0 // the weight cap is the unit tests'
	})
	toTown(t, ctx, tg, gm, f)

	var limit int32
	until(t, ctx, f.name+" shows its inventory limit", func() string {
		for _, s := range f.watch.Seen(0) {
			if u, ok := s.Event.(botclient.UserInfo); ok && u.ObjectID == f.id {
				limit = u.InvLimit
			}
		}
		if limit <= 0 {
			return "no inventory limit yet"
		}
		return ""
	})
	held := int32(len(inventory(f.watch.Seen(0))))
	if held >= limit {
		t.Fatalf("%s holds %d items of %d before the fill", f.name, held, limit)
	}
	from := f.seen()
	check := RefusalCheck{
		From:     len(f.watch.Sent()),
		Packet:   func(s Sent) bool { return s.Op == opAction },
		Messages: []int32{msgSlotsFull},
	}
	gmGiveGear(t, ctx, gm, f, magicRing, limit-held)

	untilServed(t, ctx, f.name+" wears two rings and sells the rest", f.watch, check, func() string {
		inv := inventory(f.watch.Seen(0))
		w, n := worn(inv)[magicRing], carriedOf(inv, magicRing)
		if w != 2 || n != 2 || int32(len(inv)) > limit-slotsNear {
			return fmt.Sprintf("%d rings, %d worn, %d of %d items", n, w, len(inv), limit)
		}
		return ""
	})
	peak := 0
	inv := inventory(f.watch.Seen(0)[:from])
	for _, s := range f.watch.Seen(0)[from:] {
		applyInventory(inv, s.Event)
		peak = max(peak, len(inv))
	}
	if peak < int(limit) {
		t.Fatalf("%s: the bag peaked at %d items, want the fill to reach the limit %d", f.name, peak, limit)
	}
	pickupDrop(t, ctx, gm, f, check)
	t.Logf("%s: limit %d, peaked at %d items, ends with %d", f.name, limit, peak, len(inventory(f.watch.Seen(0))))
}

// pickupDrop has the GM kill wolves next to f, up to groundKills, until f
// picks up the drop of one it never fought; no pickup is refused for want
// of a slot (check).
func pickupDrop(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, check RefusalCheck) {
	t.Helper()
	for i := range groundKills {
		gmOn(t, ctx, gm, f)
		mob, err := gm.Spawn(ctx, wolf)
		if err != nil {
			t.Fatal(err)
		}
		if err := gm.Select(ctx, mob); err != nil {
			t.Fatal(err)
		}
		if err := gm.Command(ctx, "kill"); err != nil {
			t.Fatal(err)
		}
		dctx, cancel := context.WithTimeout(ctx, time.Minute)
		_, _, err = f.watch.Await(dctx, 0, func(ev botclient.Event) bool {
			d, ok := ev.(botclient.Die)
			return ok && d.ObjectID == mob
		})
		cancel()
		if err != nil {
			t.Fatalf("kill %d: %s sees wolf %d die: %v", i, f.name, mob, err)
		}
		gm.Forget(mob)
		drops, fought := groundDrops(f, mob)
		if fought || len(drops) == 0 {
			t.Logf("kill %d: wolf %d fought %v, %d drops", i, mob, fought, len(drops))
			continue
		}
		due := time.Now().Add(groundGrace + 45*time.Second)
		untilServedBy(t, ctx, fmt.Sprintf("%s picks up a drop of wolf %d", f.name, mob), due, f.watch, check, func() string {
			for _, s := range f.watch.Seen(0) {
				if g, ok := s.Event.(botclient.GetItem); ok && g.PickerID == f.id {
					if _, dropped := drops[g.ObjectID]; dropped {
						return ""
					}
				}
			}
			return fmt.Sprintf("%d drops of wolf %d on the ground", len(drops), mob)
		})
		return
	}
	t.Fatalf("no drop of a wolf %s never fought in %d kills", f.name, groundKills)
}
