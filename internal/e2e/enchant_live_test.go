package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// enchantWeaponD is Scroll: Enchant Weapon (Grade D) (enchant/service.go
// scrolls).
const enchantWeaponD int32 = 955

// enchantHold is how long a bot at the safe level is watched using no
// more scrolls: past many enchant rule decisions.
const enchantHold = 20 * time.Second

// System messages the server cancels an enchant with
// (network/enchant.go sendEnchantMessage): the cancel of a missing target
// or scroll, an inappropriate condition, a store or trade under way.
const (
	msgInappropriateEnchant   int32 = 355
	msgEnchantScrollCancelled int32 = 423
	msgCannotEnchantWhileBusy int32 = 1688
)

// enchantRefused is what refuses the bot's RequestEnchantItem: the cancel
// messages EnchantResult 2 comes with.
func enchantRefused(w *Watch) RefusalCheck {
	return RefusalCheck{
		From:     len(w.Sent()),
		Packet:   func(s Sent) bool { return s.Op == botclient.OpRequestEnchantItem },
		Messages: []int32{msgInappropriateEnchant, msgEnchantScrollCancelled, msgCannotEnchantWhileBusy},
	}
}

// TestLiveEnchantSafe runs a level 20 fighter in the Talking Island
// village, stripped and given a D-grade Saber and the D shots it fires
// (#181): it wears the saber. Given four D weapon enchant scrolls, it
// enchants the saber to +3, the safe level, one scroll per exchange, and
// keeps the fourth.
func TestLiveEnchantSafe(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 15*time.Minute)
	travel, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("en", 1)
	// Earlier runs' scrolls and shots go with the gear: the bot enchants
	// with the four given only.
	strip(t, ctx, tg, gm, with(items, enchantWeaponD, soulshotD), account, &botclient.HumanFighter,
		stripEnd{level: 20, carry: map[int32]int32{saber: 1, soulshotD: 400}}, squiresShirt, squiresPants)
	f := runTownBot(t, ctx, tg, account, &botclient.HumanFighter, travel, func(p *botclient.Persona) {
		warriorGear(p)
		errandsOff(p)
		// Wearing only what it is given, never buying or selling.
		p.Shop = botclient.ShopPolicy{}
	})
	toTown(t, ctx, tg, gm, f)
	wears(t, ctx, f, items, "the saber", func(w map[int32]int) bool { return w[saber] == 1 })
	if e := wornEnchant(f, saber); e != 0 {
		t.Fatalf("%s wears the saber at +%d, want a fresh +0 one", f.name, e)
	}

	gmOn(t, ctx, gm, f)
	had := f.carries(enchantWeaponD) // 0 after the strip
	from := len(f.watch.Sent())
	check := enchantRefused(f.watch)
	gmGive(t, ctx, gm, f, enchantWeaponD, 4)
	ectx, cancel := context.WithTimeout(ctx, gearBound)
	defer cancel()
	untilServed(t, ectx, f.name+" enchants the saber to +3", f.watch, check, func() string {
		if e := wornEnchant(f, saber); e < 3 {
			return fmt.Sprintf("saber at +%d, %d scrolls (rule %q)", e, f.carries(enchantWeaponD), f.bot.Status().Rule)
		}
		return ""
	})
	t.Logf("%s's saber at +3", f.name)

	hold, stop := context.WithTimeout(ctx, enchantHold)
	defer stop()
	<-hold.Done()
	requests := 0
	for _, s := range f.watch.Sent()[from:] {
		if s.Op == botclient.OpRequestEnchantItem {
			requests++
		}
	}
	if e, left := wornEnchant(f, saber), f.carries(enchantWeaponD); e != 3 || requests != 3 || left != had+1 {
		t.Fatalf("%s: saber at +%d after %d enchant requests, %d scrolls left; want +3 after 3, %d left", f.name, e, requests, left, had+1)
	}
}

// wornEnchant is the enchant level of the worn item of template tmpl f's
// inventory shows, -1 when none is worn.
func wornEnchant(f *farmer, tmpl int32) int {
	for _, it := range inventory(f.watch.Seen(0)) {
		if it.Equipped && it.TemplateID == tmpl {
			return int(it.Enchant)
		}
	}
	return -1
}
