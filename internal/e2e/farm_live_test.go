package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveFarmFight runs a farmer seeded by the GM with Power Strike,
// healing potions, soulshots and junk, and watches it farm its spot: it
// drops the junk where the GM sees it (#29), turns its soulshots on and
// spends them (#23), kills monsters, a GM-spawned one among them, with
// steady swings and no click per swing, casting Power Strike no sooner
// than the last cast's reuse allows (#20), drinks a potion once a hit takes
// it under CriticalHP (#23), and picks up a kill's drop into its inventory
// (#29), never sending more than the pacer allows.
func TestLiveFarmFight(t *testing.T) {
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 20*time.Minute)
	spots := spotRegistry(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("f", 1)
	f := runFarmer(t, ctx, tg, account, spots, func(p *botclient.Persona) {
		p.CriticalHP = 95 // the first hit that lands makes it drink
		p.AttackSkills = []int32{powerStrike}
		p.MPReserve = 0
		p.Loot.Junk = []int32{stem}
		p.Loot.DropAbove = 1 // any load: the stems go at once
	})
	gmOn(t, ctx, gm, f, "skill set 3 1", "give 1061 10", "give 1835 500")
	home := gm.Pos()
	waitFor(t, ctx, "the farmer at its spot", func() bool { return f.bot.Status().Rule == "farm" })

	// Junk is dropped where the farmer stands still: at its spot.
	gmOn(t, ctx, gm, f, "give 1864 3")
	if _, _, err := f.watch.Await(ctx, 0, func(ev botclient.Event) bool {
		n, _, ok := stock(ev, stem)
		return ok && n > 0
	}); err != nil {
		t.Fatalf("the farmer gets the stems: %v", err)
	}
	if _, err := gm.Await(ctx, 3*time.Minute, func(ev botclient.Event) bool {
		d, ok := ev.(botclient.DropItem)
		return ok && d.DropperID == f.id && d.Item.TemplateID == stem
	}); err != nil {
		t.Fatalf("the GM sees the junk dropped: %v", err)
	}
	t.Logf("%s dropped its junk next to the GM", f.name)

	mob, err := gm.Spawn(ctx, gremlin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { gmBackFor(t, gm, f, mob) })
	// Out of the farmer's way: a monster that attacks the GM is engaged
	// by another player, which the farmer leaves alone.
	gmTeleport(t, ctx, gm, home)

	until(t, ctx, "the farm fight", func() string {
		r := readFight(f, mob)
		if r.spawnKilled {
			gm.Forget(mob)
		}
		return r.missing()
	})
	r := readFight(f, mob)
	t.Logf("%s: %d kills, %d swings for %d attack clicks, %d casts, %d frames at most in a second, %d ActionFailed",
		f.name, len(r.kills), r.swings, r.clicks, len(r.casts), r.perSecond, r.failed)
	if r.clicks >= r.swings {
		t.Errorf("%d attack clicks for %d swings on the monsters it killed, want fewer clicks than swings", r.clicks, r.swings)
	}
	for _, v := range r.early {
		t.Errorf("Power Strike cast too soon: %s", v)
	}
	if r.perSecond > frameCap {
		t.Errorf("%d frames within a second, want at most %d", r.perSecond, frameCap)
	}
}

// gmBackFor brings the GM back next to f while mob, a monster the run
// spawned, is still alive, so that cleanup can select and delete it.
func gmBackFor(t *testing.T, gm *GMClient, f *farmer, mob int32) {
	if !gm.owned(mob) || gm.Closed() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := gm.Command(ctx, "teleportto "+f.name); err != nil {
		t.Logf("cleanup: GM back to %s: %v", f.name, err)
		return
	}
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(f.id)); err != nil {
		t.Logf("cleanup: GM back to %s: %v", f.name, err)
	}
}

// fight is what a farmer's watch shows of its fights so far.
type fight struct {
	shotsOn, shotsSpent   bool
	potionUsed, potionLow bool
	// kills are the monsters it swung at that died; swings and clicks
	// are its swings at them (server Attack) and its AttackRequests on
	// them.
	kills         map[int32]bool
	swings        int
	clicks        int
	spawnKilled   bool
	casts         []Seen
	early         []string
	pickedUp      bool
	perSecond     int
	failed        int
	pickupPending int
}

// herb reports whether tmpl is a herb (8600-8614): used up on pickup, it
// never reaches the inventory.
func herb(tmpl int32) bool { return tmpl >= 8600 && tmpl <= 8614 }

func readFight(f *farmer, mob int32) fight {
	seen, sent := f.watch.Seen(0), f.watch.Sent()
	r := fight{kills: map[int32]bool{}, perSecond: maxPerSecond(sent)}
	r.shotsSpent, _ = stockSpent(seen, soulshotNG)
	var potion int32
	r.potionLow, potion = stockSpent(seen, healingPotion)
	swungAt := map[int32]int{}
	drops := map[int32]int32{} // ground item -> template, dropped by a kill
	picked := map[int32]int{}  // template picked up -> index of its GetItem
	for i, s := range seen {
		switch e := s.Event.(type) {
		case botclient.ExAutoSoulShot:
			r.shotsOn = r.shotsOn || e.TemplateID == soulshotNG && e.Enabled
		case botclient.Attack:
			if e.AttackerID == f.id && len(e.Hits) > 0 {
				swungAt[e.Hits[0].TargetID]++
			}
		case botclient.Die:
			if swungAt[e.ObjectID] > 0 {
				r.kills[e.ObjectID] = true
				r.spawnKilled = r.spawnKilled || e.ObjectID == mob
			}
		case botclient.DropItem:
			if r.kills[e.DropperID] {
				drops[e.Item.ObjectID] = e.Item.TemplateID
			}
		case botclient.GetItem:
			if tmpl, ok := drops[e.ObjectID]; ok && e.PickerID == f.id && !herb(tmpl) {
				picked[tmpl] = i
			}
		case botclient.InventoryUpdate:
			for _, c := range e.Changes {
				if at, ok := picked[c.Item.TemplateID]; ok && at < i && c.State != botclient.ItemRemoved {
					r.pickedUp = true
				}
			}
		case botclient.MagicSkillUse:
			if e.CasterID == f.id && e.SkillID == powerStrike {
				r.casts = append(r.casts, s)
			}
		case botclient.ActionFailed:
			r.failed++
		}
	}
	r.pickupPending = len(drops)
	for id := range r.kills {
		r.swings += swungAt[id]
	}
	for _, p := range sent {
		switch {
		case p.Op == botclient.OpAttackRequest && r.kills[p.Target]:
			r.clicks++
		case p.Op == botclient.OpUseItem && potion != 0 && p.Target == potion:
			r.potionUsed = true
		case p.Op == botclient.OpRequestMagicSkillUse && p.Target == powerStrike:
			// The cast it asks for comes no sooner than the reuse of the
			// last one it saw start.
			var last *Seen
			for i := range r.casts {
				if r.casts[i].At.Before(p.At) {
					last = &r.casts[i]
				}
			}
			if last == nil {
				continue
			}
			c := last.Event.(botclient.MagicSkillUse)
			if gap := p.At.Sub(last.At); gap < max(c.HitTime, c.Reuse) {
				r.early = append(r.early, fmt.Sprintf("asked %v after the last cast, hit time %v, reuse %v", gap, c.HitTime, c.Reuse))
			}
		}
	}
	return r
}

// missing lists what the fight has not shown yet, "" once it showed all.
func (r fight) missing() string {
	var m []string
	for _, c := range []struct {
		ok   bool
		what string
	}{
		{r.shotsOn, "ExAutoSoulShot enabling soulshots"},
		{r.shotsSpent, "soulshots spent (InventoryUpdate)"},
		{len(r.kills) > 0, "a kill"},
		{r.spawnKilled, "the GM-spawned monster killed"},
		{len(r.casts) >= 2, "two Power Strike casts"},
		{r.potionUsed, "UseItem of a potion"},
		{r.potionLow, "a potion spent (InventoryUpdate)"},
		{r.pickedUp, fmt.Sprintf("a kill's drop picked up into the inventory (%d drops seen)", r.pickupPending)},
	} {
		if !c.ok {
			m = append(m, c.what)
		}
	}
	return strings.Join(m, ", ")
}
