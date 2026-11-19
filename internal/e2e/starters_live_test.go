package e2e

import (
	"context"
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
	centres := map[string]botclient.Point{}
	for _, s := range dp.Spots() {
		centres[s.Maker] = botclient.Point{X: s.Center.X, Y: s.Center.Y, Z: s.Z}
	}
	leash := botclient.DefaultPersona().Spots.Leash
	gm := enterGM(t, ctx, tg)
	classes := []*botclient.StartingClass{&botclient.ElvenFighter, &botclient.DarkFighter, &botclient.OrcFighter}
	farmers := make([]*farmer, len(classes))
	for i, class := range classes {
		account, _ := tg.Account("sr", i+1)
		f := runWatched(t, ctx, tg, account, botSetup{
			class: class,
			persona: func() botclient.Persona {
				p := botclient.DefaultPersona()
				// No loot: carried run after run, it would weigh a
				// character set back to level 1 down.
				p.Loot.Pickup = false
				return p
			},
			brain: func(cfg *botclient.BrainConfig) { cfg.Spots = spots },
		})
		farmers[i] = f
		t.Cleanup(func() { backToLevel1(t, gm, f) })
		if lvl := levelOf(t, ctx, f); lvl > 1 {
			t.Logf("%s is level %d from an earlier run: back to 1", f.name, lvl)
			gmOn(t, ctx, gm, f, "set level 1")
		}
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
			centre, ok := centres[f.bot.Status().Spot]
			return ok && killedAtSpot(f, centre, leash)
		})
		t.Logf("%s killed a monster at %s", f.name, f.bot.Status().Spot)
	}
}

// killedAtSpot reports whether a monster f swung at from within leash of
// its spot's centre died: a kill on the way to the spot, when attacked
// there, does not count.
func killedAtSpot(f *farmer, centre botclient.Point, leash int32) bool {
	swungAt := map[int32]bool{}
	for _, s := range f.watch.Seen(0) {
		switch e := s.Event.(type) {
		case botclient.Attack:
			dx, dy := int64(e.Pos.X-centre.X), int64(e.Pos.Y-centre.Y)
			if e.AttackerID == f.id && len(e.Hits) > 0 && dx*dx+dy*dy <= int64(leash)*int64(leash) {
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

// backToLevel1 has the GM set f's level back to 1, for the next run.
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
	err := gm.Select(ctx, f.id)
	if err == nil {
		err = gm.Command(ctx, "set level 1")
	}
	if err != nil {
		t.Logf("cleanup: level of %s: %v", f.name, err)
	}
}
