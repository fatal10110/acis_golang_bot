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
		p.CriticalHP = 99 // any hit that lands makes it drink
		p.AttackSkills = []int32{powerStrike}
		p.MPReserve = 0
		// No merchant visits here: what it loots stays, run after run,
		// until its inventory refuses the GM's gifts. All it carries but
		// adena, potions and shots is junk, dropped at any load.
		p.Loot.Junk = junkBut(slices.Concat([]int32{adena}, p.Potions, p.Shots)...)
		p.Loot.DropAbove = 1
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

	// Level 1 against level 1 monsters: they hit back before they die.
	gmOn(t, ctx, gm, f, "set level 1", "skill set 3 1")
	gmGive(t, ctx, gm, f, healingPotion, 10)
	gmGive(t, ctx, gm, f, soulshotNG, 500)
	t.Logf("%s seeded at level 1", f.name)

	// The farmer may have moved on: the monster spawns at it.
	gmOn(t, ctx, gm, f)
	mob, err := gm.Spawn(ctx, wolf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { gmBackFor(t, gm, f, mob) })
	// Out of the farmer's way, in a peace zone out of any monster's reach:
	// a monster that keeps attacking the GM is engaged by another player,
	// which the farmer leaves alone.
	gmTeleport(t, ctx, gm, townTI)
	alive := time.Now()

	until(t, ctx, "the farm fight", func() string {
		if time.Since(alive) > gmKeepAlive {
			gmTeleport(t, ctx, gm, townTI)
			alive = time.Now()
		}
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
	// are its swings at them (server Attack) and its AttackRequests on
	// them.
	kills       map[int32]bool
	swings      int
	clicks      int
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
	for id := range r.kills {
		r.swings += swungAt[id]
	}
	for _, p := range sent {
		if p.Op == botclient.OpAttackRequest && r.kills[p.Target] {
			r.clicks++
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
