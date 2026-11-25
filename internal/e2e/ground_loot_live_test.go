package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// groundGrace is botclient's lootGrace: how long after a bot first sees a
// ground item it did not fight for it leaves the item alone, the server's
// 15 s lock of a kill's drops to the killer (killrewards.go
// regularLootProtection).
const groundGrace = 15 * time.Second

// groundKills bounds the wolves the GM kills next to the farmer: many a
// kill drops nothing, and one the farmer fought, or that fought it, does
// not count.
const groundKills = 9

// TestLiveGroundLoot checks the ground loot source (#435): a bot that picks
// up any loot (Loot.Ground GroundAny) takes the drop of a monster the GM
// killed next to it, one it never fought, and clicks it no sooner than
// groundGrace after the drop, while the server still locks it to the GM.
func TestLiveGroundLoot(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 20*time.Minute)
	gm := enterGM(t, ctx, tg)
	var f *farmer
	t.Cleanup(func() { forgetKilled(gm, f) }) // after the bot stops, before the GM deletes
	account, _ := tg.Account("g", 1)
	// No spots: it stays where it enters, and fights only what attacks it.
	f = runFarmer(t, ctx, tg, account, nil, func(p *botclient.Persona) {
		p.Loot.Ground = botclient.GroundAny
		p.Loot.StopAbove = 0 // the weight cap is the unit tests'
	})
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
		switch {
		case fought:
			t.Logf("kill %d: %s fought wolf %d first", i, f.name, mob)
			continue
		case len(drops) == 0:
			t.Logf("kill %d: wolf %d dropped nothing", i, mob)
			continue
		}
		pctx, cancel := context.WithTimeout(ctx, groundGrace+45*time.Second)
		_, got, err := f.watch.Await(pctx, 0, func(ev botclient.Event) bool {
			g, ok := ev.(botclient.GetItem)
			_, dropped := drops[g.ObjectID]
			return ok && g.PickerID == f.id && dropped
		})
		cancel()
		if err != nil {
			t.Fatalf("kill %d: %s picks up a drop of wolf %d (%d items): %v\n%s", i, f.name, mob, len(drops), err, f.digest())
		}
		item := got.Event.(botclient.GetItem).ObjectID
		dropped := drops[item]
		for _, s := range f.watch.Sent() {
			if s.Op == botclient.OpAction && s.Target == item && s.At.Before(dropped.Add(groundGrace-time.Second)) {
				t.Fatalf("%s clicked item %d %v after its drop, want %v or more", f.name, item, s.At.Sub(dropped), groundGrace)
			}
		}
		t.Logf("kill %d: %s picked up item %d of wolf %d %v after its drop", i, f.name, item, mob, got.At.Sub(dropped))
		return
	}
	t.Fatalf("no drop of a wolf %s never fought in %d kills", f.name, groundKills)
}

// groundDrops are the items f saw mob drop, by object id, with when, and
// whether f and mob exchanged a blow or a cast.
func groundDrops(f *farmer, mob int32) (map[int32]time.Time, bool) {
	drops := map[int32]time.Time{}
	fought := false
	for _, s := range f.watch.Seen(0) {
		switch e := s.Event.(type) {
		case botclient.DropItem:
			if e.DropperID == mob {
				drops[e.Item.ObjectID] = s.At
			}
		case botclient.Attack:
			for _, h := range e.Hits {
				fought = fought || e.AttackerID == f.id && h.TargetID == mob || e.AttackerID == mob && h.TargetID == f.id
			}
		case botclient.MagicSkillUse:
			fought = fought || e.CasterID == f.id && e.TargetID == mob || e.CasterID == mob && e.TargetID == f.id
		}
	}
	return drops, fought
}
