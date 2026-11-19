package e2e

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestLiveSpots runs two farmers on one spot registry: each claims a spot
// that fits its level, none over its cap (#27). Then the GM sets the first
// one's level far over its spot's monsters: it gives the spot up and does
// not claim it back (#38). The first farmer goes back to level 1 at the
// end, so the next run finds it a newbie again.
func TestLiveSpots(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &newbieSpots)
	ctx := scenarioCtx(t, 15*time.Minute)
	spots := spotRegistry(t, tg)
	gm := enterGM(t, ctx, tg)
	a1, _ := tg.Account("n", 1)
	a2, _ := tg.Account("n", 2)

	f1 := runFarmer(t, ctx, tg, a1, spots, nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := gm.Command(ctx, "teleportto "+f1.name); err != nil {
			t.Logf("cleanup: level of %s: %v", f1.name, err)
			return
		}
		if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(f1.id)); err != nil {
			t.Logf("cleanup: level of %s: %v", f1.name, err)
			return
		}
		err := gm.Select(ctx, f1.id)
		if err == nil {
			err = gm.Command(ctx, "set level 1")
		}
		if err != nil {
			t.Logf("cleanup: level of %s: %v", f1.name, err)
		}
	})
	if lvl := levelOf(t, ctx, f1); lvl > 5 {
		t.Logf("%s is level %d from an earlier run: back to 1", f1.name, lvl)
		gmOn(t, ctx, gm, f1, "set level 1")
	}
	waitFor(t, ctx, "the first farmer's spot", func() bool { return f1.bot.Status().Spot != "" })

	f2 := runFarmer(t, ctx, tg, a2, spots, nil)
	waitFor(t, ctx, "the second farmer's spot", func() bool { return f2.bot.Status().Spot != "" })
	levels := []int{levelOf(t, ctx, f1), levelOf(t, ctx, f2)}
	st := spots.Statuses()
	for i, n := range heldBy(spots) {
		s := st[i]
		if n > s.Cap {
			t.Errorf("spot %s held by %d, over its cap %d", s.Name, n, s.Cap)
		}
		if !fitsAny(s, levels) {
			t.Errorf("spot %s (levels %d-%d, aggressive up to %d) fits neither farmer, levels %v", s.Name, s.MinLevel, s.MaxLevel, s.AggroLevel, levels)
		}
		t.Logf("spot %s (levels %d-%d): %d of %d", s.Name, s.MinLevel, s.MaxLevel, n, s.Cap)
	}

	// The farmer's own claim, not the registry's count: the other farmer
	// may take or leave a spot meanwhile.
	waitFor(t, ctx, "the first farmer holding a spot", func() bool { return f1.bot.Status().Spot != "" })
	held := f1.bot.Status().Spot
	spot := slices.IndexFunc(st, func(s botclient.SpotStatus) bool { return s.Name == held })
	if spot < 0 {
		t.Fatalf("%s holds %q, not a spot of the registry", f1.name, held)
	}
	level, next := outlevelTo(t, tg, spots.Statuses(), spot, f1.bot.Status().Position)
	t.Logf("%s holds %s; goes to level %d: %s fits it", f1.name, held, level, next)
	gmOn(t, ctx, gm, f1, "set level "+strconv.Itoa(level))
	waitFor(t, ctx, "the first farmer at its new level", func() bool { return int(f1.bot.Status().Level) >= level })
	waitFor(t, ctx, "the outleveled spot given up", func() bool { return f1.bot.Status().Spot != held })
	t.Logf("%s at level %d gave %s up for %q", f1.name, level, held, f1.bot.Status().Spot)
	stay := time.NewTicker(2 * time.Second)
	defer stay.Stop()
	for end := time.Now().Add(time.Minute); time.Now().Before(end); {
		if now := f1.bot.Status().Spot; now == held {
			t.Fatalf("%s flipped back to %s", f1.name, held)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-stay.C:
		}
	}
}

// outlevelTo is the lowest level that outlevels spot from but fits another
// spot with room in a farmer's reach from at, and that spot's name: an
// outleveled farmer with no fitting spot keeps the one it has
// (Brain.pickSpot).
func outlevelTo(t *testing.T, tg *Target, st []botclient.SpotStatus, from int, at botclient.Point) (int, string) {
	t.Helper()
	dp, err := datapack.Load(tg.Config().Datapack)
	if err != nil {
		t.Fatal(err)
	}
	all := dp.Spots()
	p := botclient.DefaultPersona().Spots
	reach := int64(p.MaxTravel) * int64(p.MaxTravel)
	for level := all[from].MaxLevel + p.Outlevel + 1; level <= 80; level++ {
		for i, s := range all {
			if i == from || st[i].Held >= st[i].Cap || !fitsAny(st[i], []int{level}) {
				continue
			}
			dx, dy := int64(s.Center.X-at.X), int64(s.Center.Y-at.Y)
			if dx*dx+dy*dy <= reach {
				return level, s.Maker
			}
		}
	}
	t.Fatalf("no level outlevels %s and fits another spot within %d of %v", st[from].Name, p.MaxTravel, at)
	return 0, ""
}

// levelOf is f's level once its status knows it.
func levelOf(t *testing.T, ctx context.Context, f *farmer) int {
	t.Helper()
	waitFor(t, ctx, f.name+"'s level", func() bool { return f.bot.Status().Level > 0 })
	return int(f.bot.Status().Level)
}

// heldBy maps every held spot of r to how many bots hold it.
func heldBy(r *botclient.SpotRegistry) map[int]int {
	held := map[int]int{}
	for i, s := range r.Statuses() {
		if s.Held > 0 {
			held[i] = s.Held
		}
	}
	return held
}

// fitsAny reports whether s fits a farmer of one of levels as
// DefaultPersona's SpotPolicy picks.
func fitsAny(s botclient.SpotStatus, levels []int) bool {
	p := botclient.DefaultPersona().Spots
	for _, l := range levels {
		aggro := p.AggroAbove
		if l <= p.Newbie {
			aggro = max(aggro, p.Below)
		}
		if s.MaxLevel <= l+p.Below && s.AggroLevel <= l+aggro && s.MaxLevel >= l-p.Above {
			return true
		}
	}
	return false
}
