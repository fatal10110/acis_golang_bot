package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// hasteHold is how long a hasted farmer carrying Potions of Alacrity is
// watched drinking none: several times the potion rule's wait
// (buffPotionWait, 5 s) at its spot.
const hasteHold = 45 * time.Second

// TestLiveBuffPotion runs two level 20 fighters at newbie spots, each given
// Potions of Alacrity, the fighters' buff potion (#176, #180). The first,
// its effects cancelled by the GM, drinks one: its UseItem goes out and the
// potion's effect shows on it. The second was given Haste by the GM and
// keeps it on itself; Haste is of the potion's stack type and order, so
// while it shows it drinks none.
func TestLiveBuffPotion(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &newbieSpots)
	ctx := scenarioCtx(t, 20*time.Minute)
	_, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	plainAcc, _ := tg.Account("bp", 1)
	hastedAcc, _ := tg.Account("bp", 2)
	for _, acc := range []string{plainAcc, hastedAcc} {
		strip(t, ctx, tg, gm, with(items, alacrityPotion), acc, &botclient.HumanFighter, stripEnd{}, squiresSword, squiresShirt, squiresPants)
	}
	plain := runFieldFarmer(t, ctx, tg, plainAcc, func(p *botclient.Persona) {
		lowSpots(p)
		errandsOff(p)
	})
	hasted := runFieldFarmer(t, ctx, tg, hastedAcc, func(p *botclient.Persona) {
		lowSpots(p)
		errandsOff(p)
		p.Buffs = []botclient.Buff{{Skill: skillHaste}}
	})
	for _, f := range []*farmer{plain, hasted} {
		waitFor(t, ctx, f.name+" at its spot", func() bool { return f.bot.Status().Rule == "farm" })
		levelTo(t, ctx, gm, f, 20)
	}

	gmOn(t, ctx, gm, hasted, fmt.Sprintf("skill set %d 1", skillHaste))
	waitFor(t, ctx, hasted.name+" hasted", func() bool { return hasted.effect(skillHaste) })
	start := time.Now()
	gmOn(t, ctx, gm, hasted)
	gmGive(t, ctx, gm, hasted, alacrityPotion, 3)
	gmTeleport(t, ctx, gm, townTI)
	until(t, ctx, hasted.name+" drinks no potion while hasted", func() string {
		if used := hasted.usedSince(alacrityPotion, start); len(used) > 0 {
			t.Fatalf("%s drank a Potion of Alacrity at %v (Haste on now: %v)", hasted.name, used[0].At, hasted.effect(skillHaste))
		}
		if time.Since(start) < hasteHold {
			return "watching"
		}
		return ""
	})
	t.Logf("%s, hasted, drank no potion in %v", hasted.name, time.Since(start).Round(time.Second))

	gmOn(t, ctx, gm, plain, "cancel")
	start = time.Now()
	gmGive(t, ctx, gm, plain, alacrityPotion, 3)
	gmTeleport(t, ctx, gm, townTI)
	until(t, ctx, plain.name+" drinks a potion", func() string {
		switch {
		case len(plain.usedSince(alacrityPotion, start)) == 0:
			return fmt.Sprintf("no UseItem (rule %q)", plain.bot.Status().Rule)
		case !plain.effect(alacritySkill):
			return "no Alacrity effect"
		}
		return ""
	})
	t.Logf("%s drank a Potion of Alacrity: %d left", plain.name, plain.carries(alacrityPotion))
}
