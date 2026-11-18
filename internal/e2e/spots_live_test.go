package e2e

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveSpots runs two farmers on one spot registry: each claims a spot
// that fits its level, none over its cap (#27). Then the GM sets the first
// one's level far over its spot's monsters: it gives the spot up and does
// not claim it back (#38). The first farmer goes back to level 1 at the
// end, so the next run finds it a newbie again.
func TestLiveSpots(t *testing.T) {
	tg := Live(t, NeedGM, NeedDatapack)
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
	waitFor(t, ctx, "the first farmer's spot", func() bool { return heldTotal(spots) == 1 })
	first := heldBy(spots)
	var spot int
	for i := range first {
		spot = i
	}

	f2 := runFarmer(t, ctx, tg, a2, spots, nil)
	waitFor(t, ctx, "the second farmer's spot", func() bool { return heldTotal(spots) == 2 })
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

	before := spots.Held(spot)
	level := st[spot].MaxLevel + botclient.DefaultPersona().Spots.Outlevel + 10
	gmOn(t, ctx, gm, f1, "set level "+strconv.Itoa(level))
	waitFor(t, ctx, "the first farmer at its new level", func() bool { return int(f1.bot.Status().Level) >= level })
	waitFor(t, ctx, "the outleveled spot given up", func() bool { return spots.Held(spot) < before })
	after := spots.Held(spot)
	t.Logf("%s at level %d gave %s up (%d of it held now, %d spots held in all)", f1.name, level, st[spot].Name, after, heldTotal(spots))
	stay := time.NewTicker(2 * time.Second)
	defer stay.Stop()
	for end := time.Now().Add(time.Minute); time.Now().Before(end); {
		if n := spots.Held(spot); n > after {
			t.Fatalf("%s flipped back: %d of %s held", f1.name, n, st[spot].Name)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-stay.C:
		}
	}
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

func heldTotal(r *botclient.SpotRegistry) int {
	n := 0
	for _, h := range heldBy(r) {
		n += h
	}
	return n
}

// fitsAny reports whether s fits a farmer of one of levels as
// DefaultPersona's SpotPolicy picks.
func fitsAny(s botclient.SpotStatus, levels []int) bool {
	p := botclient.DefaultPersona().Spots
	for _, l := range levels {
		if s.MaxLevel <= l+p.Below && s.AggroLevel <= l+p.AggroAbove && s.MaxLevel >= l-p.Above {
			return true
		}
	}
	return false
}
