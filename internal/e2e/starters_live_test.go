package e2e

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestLiveFarmStarters runs a level 1 farmer of each of three races that
// start outside Talking Island, an elf, a dark elf and an orc, on one spot
// registry: each claims a spot that fits level 1 in its own newbie zone,
// where nearly every monster is aggressive, and kills a monster there
// (#133). Their villages are not the Talking Island newbie spots, so the
// scenario keeps no group. The farmers go back to level 1 at the end, so
// the next run finds them new characters again.
func TestLiveFarmStarters(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 20*time.Minute)
	dp, err := datapack.Load(tg.Config().Datapack)
	if err != nil {
		t.Fatal(err)
	}
	spots := botclient.NewSpotRegistry(dp.Spots(), 1)
	byName := map[string]datapack.Spot{}
	for _, s := range dp.Spots() {
		byName[s.Maker] = s
	}
	leash := botclient.DefaultPersona().Spots.Leash
	gm := enterGM(t, ctx, tg)
	classes := []*botclient.StartingClass{&botclient.ElvenFighter, &botclient.DarkFighter, &botclient.OrcFighter}
	farmers := make([]*farmer, len(classes))
	for i, class := range classes {
		account, _ := tg.Account("sr", i+1)
		setup := botSetup{
			class: class,
			persona: func() botclient.Persona {
				p := botclient.DefaultPersona()
				// No loot: carried run after run, it would weigh a
				// character set back to level 1 down.
				p.Loot.Pickup = false
				return p
			},
			brain: func(cfg *botclient.BrainConfig) { cfg.Spots = spots },
		}
		farmers[i] = runFresh(t, ctx, gm, 1, func() *farmer { return runWatched(t, ctx, tg, account, setup) })
	}
	for _, f := range farmers {
		waitFor(t, ctx, f.name+" holding a spot", func() bool { return f.bot.Status().Spot != "" })
		held := f.bot.Status().Spot
		st := spots.Statuses()
		i := slices.IndexFunc(st, func(s botclient.SpotStatus) bool { return s.Name == held })
		if i < 0 {
			t.Fatalf("%s holds %q, not a spot of the registry", f.name, held)
		}
		s, level := st[i], levelOf(t, ctx, f)
		t.Logf("%s (level %d) holds %s: levels %d-%d, aggressive up to %d, %d of %d", f.name, level, s.Name, s.MinLevel, s.MaxLevel, s.AggroLevel, s.Held, s.Cap)
		if !fitsAny(s, []int{level}) {
			t.Errorf("%s holds %s, which does not fit its level %d", f.name, s.Name, level)
		}
	}
	for _, f := range farmers {
		waitFor(t, ctx, f.name+" killing a monster at its spot", func() bool {
			sp, ok := byName[f.bot.Status().Spot]
			return ok && killedAtSpot(f, sp, leash)
		})
		t.Logf("%s killed a monster at %s", f.name, f.bot.Status().Spot)
	}
}

// killedAtSpot reports whether a monster f swung at from its spot sp died:
// from within leash of its centre or inside its territory, where the farmer
// takes farm targets too (#447). A kill on the way to the spot, when
// attacked there, does not count.
func killedAtSpot(f *farmer, sp datapack.Spot, leash int32) bool {
	swungAt := map[int32]bool{}
	for _, s := range f.watch.Seen(0) {
		switch e := s.Event.(type) {
		case botclient.Attack:
			if e.AttackerID == f.id && len(e.Hits) > 0 && atSpot(e.Pos, sp, int64(leash)) {
				swungAt[e.Hits[0].TargetID] = true
			}
		case botclient.Die:
			if swungAt[e.ObjectID] {
				return true
			}
		}
	}
	return false
}

// backToLevel1 has the GM set f's level back to 1, for the next run, and
// waits until the bot's status shows it. OnTarget, not a bare Command: the
// server may clear the GM's target as it lands next to f, and then runs
// "set level" on the GM itself (#206), which left f at its level (#268).
func backToLevel1(t *testing.T, gm *GMClient, f *farmer) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := gm.Command(ctx, "teleportto "+f.name); err != nil {
		t.Logf("cleanup: level of %s: %v", f.name, err)
		return
	}
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(f.id)); err != nil {
		t.Logf("cleanup: level of %s: %v", f.name, err)
		return
	}
	err := gm.OnTarget(ctx, f.id, "set level 1")
	if err == nil {
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for err == nil && f.bot.Status().Level != 1 {
			select {
			case <-ctx.Done():
				err = fmt.Errorf("still level %d: %w", f.bot.Status().Level, ctx.Err())
			case <-tick.C:
			}
		}
	}
	if err != nil {
		t.Logf("cleanup: level of %s: %v", f.name, err)
	}
}

// atSpot reports whether p is at spot sp: within leash of its centre or,
// on the X/Y plane, inside its territory, where the farmer takes farm
// targets and stays after a fight too (#447).
func atSpot(p botclient.Point, sp datapack.Spot, leash int64) bool {
	c := botclient.Point{X: sp.Center.X, Y: sp.Center.Y}
	return within(p, c, leash) || sp.Territory != nil && sp.Territory.Contains(datapack.Point{X: p.X, Y: p.Y})
}
