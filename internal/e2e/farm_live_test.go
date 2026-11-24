package e2e

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveFarmFight runs a farmer seeded by the GM with Power Strike,
// healing potions, soulshots and junk, and watches it farm its spot: it
// drops the junk where the GM sees it (#29), turns its soulshots on and
// spends them (#23), kills monsters, the first of a GM-spawned orc grunt
// pack among them, one at least in several swings of one attack click,
// casting Power Strike no sooner than the last cast's reuse allows (#20), drinks a potion
// once a hit takes it under CriticalHP (#23), and picks up a kill's drop
// into its inventory (#29), never sending more than the pacer allows.
func TestLiveFarmFight(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &newbieSpots)
	ctx := scenarioCtx(t, 30*time.Minute) // run 8 passed after 19m50s: a kill with a non-herb drop is rare
	spots := spotRegistry(t, tg)
	gm := enterGM(t, ctx, tg)
	var f *farmer
	t.Cleanup(func() { forgetKilled(gm, f) }) // after the farmer stops, before the GM deletes
	account, _ := tg.Account("f", 1)
	f = runFarmer(t, ctx, tg, account, spots, func(p *botclient.Persona) {
		p.CriticalHP = 99 // any hit that lands makes it drink
		p.AttackSkills = []int32{powerStrike}
		p.MPReserve = -1 // off over the support build's 50: it casts and fights at any MP
		// No merchant visits here: what it loots stays, run after run,
		// until its inventory refuses the GM's gifts. All it carries but
		// adena, potions and shots is junk, dropped at any load.
		p.Loot.Junk = junkBut(slices.Concat([]int32{adena, soulshotNG}, p.Potions)...)
		p.Loot.DropAbove = 1
		// The GM tops the shots up (gmTopUp), but earlier runs gave 500
		// every run (4 weight each, #339): the pinned character still
		// carries over the default pickup cap of 65 %, and the scenario
		// checks the pickup, not the cap.
		p.Loot.StopAbove = 0
	})
	waitFor(t, ctx, "the farmer at its spot", func() bool { return f.bot.Status().Rule == "farm" })
	t.Logf("%s farms at %v", f.name, f.bot.Status().Position)

	// Junk is dropped where the farmer stands still: at its spot. A new
	// farmer carries none: the stems are its junk then (a full inventory
	// refuses them, and holds junk already).
	gmOn(t, ctx, gm, f, fmt.Sprintf("give %d 3", stem))
	if _, err := gm.Await(ctx, 3*time.Minute, func(ev botclient.Event) bool {
		d, ok := ev.(botclient.DropItem)
		return ok && d.DropperID == f.id
	}); err != nil {
		t.Fatalf("the GM sees the junk dropped: %v", err)
	}
	t.Logf("%s dropped its junk next to the GM", f.name)

	// Level 5, about 200 HP, against a pack of orc grunts: each takes a few
	// swings, and hits back meanwhile, so the farmer drinks without dying.
	// Alone against level 1 monsters it one-shots each before it is ever
	// hit.
	gmOn(t, ctx, gm, f, "set level 5", "skill set 3 1")
	gmGive(t, ctx, gm, f, healingPotion, 10)
	gmTopUp(t, ctx, gm, f, soulshotNG, 500)
	t.Logf("%s seeded at level 5", f.name)

	// The farmer may have moved on: the pack spawns at it.
	gmOn(t, ctx, gm, f)
	pack := make([]int32, 0, packSize)
	for range packSize {
		mob, err := gm.Spawn(ctx, orcGrunt)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { gmBackFor(t, gm, f, mob) })
		pack = append(pack, mob)
	}
	mob := pack[0]
	// Out of the farmer's way, in a peace zone out of any monster's reach:
	// a monster that keeps attacking the GM is engaged by another player,
	// which the farmer leaves alone.
	gmTeleport(t, ctx, gm, townTI)
	alive := time.Now()

	// The step's bound: run 8 passed after 19m50s (a kill with a non-herb
	// drop is rare), so the fight alone may take most of the scenario.
	untilBy(t, ctx, "the farm fight", time.Now().Add(25*time.Minute), func() string {
		if time.Since(alive) > gmKeepAlive {
			gmTeleport(t, ctx, gm, townTI)
			alive = time.Now()
		}
		for _, id := range pack {
			if gm.owned(id) && readFight(f, id).spawnKilled {
				gm.Forget(id)
			}
		}
		return readFight(f, mob).missing()
	})
	r := readFight(f, mob)
	t.Logf("%s: %d kills (%d of 2 swings or more on fewer clicks), %d swings for %d attack clicks, %d casts, %d frames at most in a second, %d ActionFailed",
		f.name, len(r.kills), r.melee, r.swings, r.clicks, len(r.casts), r.perSecond, r.failed)
	for _, v := range r.early {
		t.Errorf("Power Strike cast too soon: %s", v)
	}
	if r.perSecond > frameCap {
		t.Errorf("%d frames within a second, want at most %d", r.perSecond, frameCap)
	}
}

// townTI is Talking Island Village (teleports.xml), a peace zone far from
// the newbie spots around it.
var townTI = botclient.Point{X: -84141, Y: 244623, Z: -3729}

// gmKeepAlive is how long the GM may wait in silence: the server drops a
// connection it read nothing from for 15 minutes (session.go
// clientReadIdleTimeout), and a teleport reads what queued up meanwhile.
const gmKeepAlive = 3 * time.Minute

// gmBackFor brings the GM back next to f while mob, a monster the run
// spawned, is still alive, so that cleanup can select and delete it.
func gmBackFor(t *testing.T, gm *GMClient, f *farmer, mob int32) {
	forgetKilled(gm, f)
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
	// kills are the monsters it swung or cast at that died; swings and clicks
	// are its swings at them (server Attack) and its attack clicks on them:
	// every Action on one after the one that selected it, and every
	// AttackRequest. melee counts the kills of 2 swings or more with fewer
	// clicks than swings: the server's auto-attack kept swinging (#20).
	kills       map[int32]bool
	swings      int
	clicks      int
	melee       int
	spawnKilled bool
	casts       []Seen
	early       []string
	pickedUp    bool
	loot        int
	perSecond   int
	failed      int
	// What the farmer saw of the GM-spawned monster: in sight, out of it,
	// its hits by target, and the farmer's clicks on it.
	mobSeen, mobGone bool
	mobHits          map[int32]int
	mobClicks        int
}

// groundItem is a DropItem: who dropped which template.
type groundItem struct{ dropper, template int32 }

// herb reports whether tmpl is a herb (8600-8614): used up on pickup, it
// never reaches the inventory.
func herb(tmpl int32) bool { return tmpl >= 8600 && tmpl <= 8614 }

func readFight(f *farmer, mob int32) fight {
	seen, sent := f.watch.Seen(0), f.watch.Sent()
	r := fight{kills: map[int32]bool{}, mobHits: map[int32]int{}, perSecond: maxPerSecond(sent)}
	r.shotsSpent, _ = stockSpent(seen, soulshotNG)
	var potion int32
	r.potionLow, potion = stockSpent(seen, healingPotion)
	swungAt := map[int32]int{}
	castAt := map[int32]bool{} // monsters it cast Power Strike at
	// The server sends a kill's DropItems before its Die
	// (Creature.doDie: rewards first): the drops are matched to the kills
	// once all are read.
	drops := map[int32]groundItem{} // ground item -> its drop
	picked := map[int32]int{}       // ground item -> index of its GetItem
	gained := map[int32][]int{}     // template -> indexes of InventoryUpdates adding it
	for i, s := range seen {
		switch e := s.Event.(type) {
		case botclient.ExAutoSoulShot:
			r.shotsOn = r.shotsOn || e.TemplateID == soulshotNG && e.Enabled
		case botclient.Attack:
			if e.AttackerID == f.id && len(e.Hits) > 0 {
				swungAt[e.Hits[0].TargetID]++
			}
		case botclient.Die:
			if swungAt[e.ObjectID] > 0 || castAt[e.ObjectID] {
				r.kills[e.ObjectID] = true
				r.spawnKilled = r.spawnKilled || e.ObjectID == mob
			}
		case botclient.DropItem:
			drops[e.Item.ObjectID] = groundItem{dropper: e.DropperID, template: e.Item.TemplateID}
		case botclient.GetItem:
			if e.PickerID == f.id {
				picked[e.ObjectID] = i
			}
		case botclient.InventoryUpdate:
			for _, c := range e.Changes {
				if c.State != botclient.ItemRemoved {
					gained[c.Item.TemplateID] = append(gained[c.Item.TemplateID], i)
				}
			}
		case botclient.NPCInfo:
			r.mobSeen = r.mobSeen || e.ObjectID == mob
		case botclient.DeleteObject:
			r.mobGone = r.mobGone || e.ObjectID == mob
		case botclient.MagicSkillUse:
			if e.CasterID == f.id && e.SkillID == powerStrike {
				r.casts = append(r.casts, s)
				castAt[e.TargetID] = true
			}
		case botclient.ActionFailed:
			r.failed++
		}
		if a, ok := s.Event.(botclient.Attack); ok && a.AttackerID == mob && len(a.Hits) > 0 {
			r.mobHits[a.Hits[0].TargetID]++
		}
	}
	for item, d := range drops {
		if !r.kills[d.dropper] || herb(d.template) {
			continue
		}
		r.loot++
		at, ok := picked[item]
		if ok && slices.ContainsFunc(gained[d.template], func(i int) bool { return i > at }) {
			r.pickedUp = true
		}
	}
	actions, requests := map[int32]int{}, map[int32]int{}
	for _, p := range sent {
		switch p.Op {
		case botclient.OpAction:
			actions[p.Target]++
		case botclient.OpAttackRequest:
			requests[p.Target]++
		}
		if (p.Op == botclient.OpAction || p.Op == botclient.OpAttackRequest) && p.Target == mob {
			r.mobClicks++
		}
		switch {
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
	for id := range r.kills {
		clicks := requests[id] + max(actions[id]-1, 0)
		r.swings += swungAt[id]
		r.clicks += clicks
		if swungAt[id] >= 2 && clicks < swungAt[id] {
			r.melee++
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
		{r.spawnKilled, fmt.Sprintf("the GM-spawned monster killed (seen %v, gone %v, its hits %v, clicked %d times)", r.mobSeen, r.mobGone, r.mobHits, r.mobClicks)},
		{len(r.casts) >= 2, "two Power Strike casts"},
		{r.melee > 0, fmt.Sprintf("a kill of 2 swings or more with fewer attack clicks than swings (%d swings, %d clicks on %d kills)", r.swings, r.clicks, len(r.kills))},
		{r.potionUsed, "UseItem of a potion"},
		{r.potionLow, "a potion spent (InventoryUpdate)"},
		{r.pickedUp, fmt.Sprintf("a kill's drop picked up into the inventory (%d non-herb drops of kills seen)", r.loot)},
	} {
		if !c.ok {
			m = append(m, c.what)
		}
	}
	return strings.Join(m, ", ")
}
