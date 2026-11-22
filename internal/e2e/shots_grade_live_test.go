package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// shotHold is how long a farmer carrying a saber and no D shots is watched
// keeping its falchion: past several gear errand decisions at its spot.
const shotHold = 45 * time.Second

// dShots is how many D shots back the saber: the persona's soulshot
// minimum (ShotStock.Min, shotTarget) is dShotsMin.
const dShots, dShotsMin = 400, 200

// TestLiveShotsByGrade levels a farmer stripped to a no-grade Falchion to
// 20 at a newbie spot and gives it no-grade soulshots and a D-grade Saber
// (#169, #180): with no D shot on sale (D14 b) it keeps the Falchion, its
// no-grade shots on and spent. Given D shots past its minimum, it wears the
// Saber, turns the D shots on and the no-grade ones off, and spends the D
// shots.
func TestLiveShotsByGrade(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &newbieSpots)
	ctx := scenarioCtx(t, 25*time.Minute)
	_, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("sg", 1)
	strip(t, ctx, tg, gm, with(items, soulshotNG, soulshotD), account, &botclient.HumanFighter, stripEnd{}, falchion, squiresShirt, squiresPants)
	f := runFieldFarmer(t, ctx, tg, account, func(p *botclient.Persona) {
		lowSpots(p)
		errandsOff(p)
		p.Supply.ReturnMinutes = 0
		p.Shop.Soulshots = botclient.ShotStock{Min: dShotsMin, Max: dShots}
	})
	waitFor(t, ctx, "the farmer at its spot", func() bool { return f.bot.Status().Rule == "farm" })
	levelTo(t, ctx, gm, f, 20)
	gmOn(t, ctx, gm, f)
	gmGive(t, ctx, gm, f, soulshotNG, 2000)
	gmGiveGear(t, ctx, gm, f, saber, 1)
	from := f.seen()
	gmTeleport(t, ctx, gm, townTI)
	t.Logf("%s at level 20 with a saber and no-grade shots at %v", f.name, f.bot.Status().Position)

	until(t, ctx, "no-grade shots on and spent", func() string {
		switch {
		case f.wields(saber):
			return "wears the saber with no D shots"
		case !f.wields(falchion):
			return "the falchion off"
		case !f.autoShot(soulshotNG):
			return "no-grade shots off"
		case !fell(f.counts(soulshotNG, from)):
			return fmt.Sprintf("no no-grade shot spent (rule %q)", f.bot.Status().Rule)
		}
		return ""
	})
	seen := f.watch.Seen(0)
	inv := inventory(seen)
	hold, cancel := context.WithTimeout(ctx, shotHold)
	defer cancel()
	// The match runs under the watch's lock: it folds each event itself.
	if _, _, err := f.watch.Await(hold, len(seen), func(ev botclient.Event) bool {
		applyInventory(inv, ev)
		return worn(inv)[saber] > 0
	}); err == nil {
		t.Fatalf("%s wears the saber with no D shots", f.name)
	}
	t.Logf("%s keeps the falchion: no-grade shots %v", f.name, f.counts(soulshotNG, from))

	gmOn(t, ctx, gm, f)
	gmGive(t, ctx, gm, f, soulshotD, dShots)
	from = f.seen()
	gmTeleport(t, ctx, gm, townTI)
	until(t, ctx, "the saber worn and D shots spent", func() string {
		switch {
		case !f.wields(saber):
			return "the saber not worn"
		case !f.autoShot(soulshotD):
			return "D shots off"
		case f.autoShot(soulshotNG):
			return "no-grade shots still on"
		case !fell(f.counts(soulshotD, from)):
			return fmt.Sprintf("no D shot spent (rule %q)", f.bot.Status().Rule)
		}
		return ""
	})
	t.Logf("%s wears the saber: D shots %v", f.name, f.counts(soulshotD, from))
}
