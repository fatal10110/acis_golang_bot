package e2e

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// darkElvenStart is where a new dark elf enters the world
// (classes/darkFighter.xml <spawns>).
var darkElvenStart = botclient.Point{X: 28295, Y: 11063, Z: -4224}

// TestLiveSpotSpread runs a level 1 dark elf farmer on a registry that
// lacks the spots fitting it within fullNear of its village's start, as
// though bots filled them all: brought to the start, it claims a fitting
// spot further away and walks there (#137). The farmer goes back to level 1
// at the end, so the next run finds it a new character again; each run
// brings it to the start first.
//
// The full spots are the nearest ones, not all within MaxTravel: from the
// dark elven village the server routes no walk across to the elven
// village's spots beyond it (#144), which TestSpotSpread covers as a choice.
func TestLiveSpotSpread(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 20*time.Minute)
	dp, err := datapack.Load(tg.Config().Datapack)
	if err != nil {
		t.Fatal(err)
	}
	p := botclient.DefaultPersona().Spots
	fromStart := func(s datapack.Spot) int64 {
		dx, dy := int64(s.Center.X-darkElvenStart.X), int64(s.Center.Y-darkElvenStart.Y)
		return dx*dx + dy*dy
	}
	fitsNewbie := func(s datapack.Spot) bool {
		return fitsAny(botclient.SpotStatus{MinLevel: s.MinLevel, MaxLevel: s.MaxLevel, AggroLevel: s.AggroLevel}, []int{1})
	}
	// fullNear takes the three nearest spots out (3389-4228 from the
	// start); the next ones lie 5464 and 6778 away.
	const fullNear = 5000
	near := int64(fullNear) * int64(fullNear)
	var kept []datapack.Spot
	full := 0
	for _, s := range dp.Spots() {
		if fitsNewbie(s) && fromStart(s) <= near {
			full++
			continue
		}
		kept = append(kept, s)
	}
	if full == 0 {
		t.Fatalf("no spot fits level 1 within %d of the dark elven start", fullNear)
	}
	t.Logf("%d spots fitting level 1 within %d of the start taken out as full", full, fullNear)
	spots := botclient.NewSpotRegistry(kept, 1)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("sp", 1)
	f := runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.DarkFighter,
		persona: func() botclient.Persona {
			p := botclient.DefaultPersona()
			// No loot: carried run after run, it would weigh a character
			// set back to level 1 down.
			p.Loot.Pickup = false
			return p
		},
		brain: func(cfg *botclient.BrainConfig) { cfg.Spots = spots },
	})
	t.Cleanup(func() { backToLevel1(t, gm, f) })
	if lvl := levelOf(t, ctx, f); lvl > 1 {
		t.Logf("%s is level %d from an earlier run: back to 1", f.name, lvl)
		gmOn(t, ctx, gm, f, "set level 1")
	}
	gmTeleport(t, ctx, gm, darkElvenStart)
	if err := gm.Command(ctx, "recall "+f.name); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(f.id)); err != nil {
		t.Fatalf("%s recalled to the start: %v", f.name, err)
	}
	waitFor(t, ctx, f.name+" at the start", func() bool { return within(f.bot.Status().Position, darkElvenStart, 3000) })

	// The spot the farmer holds, checked each time it changes: a spot it
	// cannot walk to it gives up for another.
	idx := func(name string) int {
		return slices.IndexFunc(kept, func(s datapack.Spot) bool { return s.Maker == name })
	}
	wander := int64(p.Wander) * int64(p.Wander)
	checked := ""
	check := func(name string) {
		if name == checked {
			return
		}
		checked = name
		i := idx(name)
		if i < 0 {
			t.Fatalf("%s holds %q, not a spot of the registry", f.name, name)
		}
		s := kept[i]
		d := fromStart(s)
		t.Logf("%s holds %s: levels %d-%d, aggressive up to %d, squared distance from the start %d", f.name, s.Maker, s.MinLevel, s.MaxLevel, s.AggroLevel, d)
		if !fitsNewbie(s) {
			t.Fatalf("%s holds %s, which does not fit level 1", f.name, s.Maker)
		}
		if d <= near || d > wander {
			t.Fatalf("%s holds %s, squared %d from the start, not past the full spots and within Wander", f.name, s.Maker, d)
		}
	}
	leash := int64(p.Leash) * int64(p.Leash)
	walk, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	waitFor(t, walk, f.name+" at a spot past the full ones", func() bool {
		st := f.bot.Status()
		if st.Spot == "" {
			return false
		}
		check(st.Spot)
		s := kept[idx(st.Spot)]
		dx, dy := int64(st.Position.X-s.Center.X), int64(st.Position.Y-s.Center.Y)
		return dx*dx+dy*dy <= leash
	})
	t.Logf("%s reached %s at %v", f.name, checked, f.bot.Status().Position)
}
