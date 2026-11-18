package e2e

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// hopAway is how far from Talking Island village a teleport must land to
// count as a hop to another town: farther than every destination of the
// island's gatekeeper on the island itself (at most 38000 away), nearer
// than the closest town, Gludin (95000; teleports.xml, npc 30006).
const hopAway = 50000

// TestLiveTownErrands runs a town bot (cmd/bots -persona town: TownPersona
// and the travel map, no spots) in Talking Island village with adena from
// the GM: it runs its errand cycle, a merchant visit that buys potions,
// then a gatekeeper hop to another town once its idle spell is over (#33).
// The hop is #32's teleport: the bot reappears at the destination, where
// the GM, teleported to it, sees it. Next to Katerina the server refuses
// every routed ground move of the bot, and the bot unsticks itself on the
// way to the gatekeeper (#122).
func TestLiveTownErrands(t *testing.T) {
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 20*time.Minute)
	travel := travelMap(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("tw", 1)
	f := runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.HumanFighter,
		persona: func() botclient.Persona {
			p := botclient.TownPersona()
			// Persona defaults (D6), shortened for a bounded run: a hop a
			// minute after the bot's first decision, and a purchase every
			// run that leaves the price of the island's cheapest town hop
			// (Gludin, 18000 adena).
			p.Town.HopMin, p.Town.HopMax = time.Minute, time.Minute
			p.Shop.Restock = []botclient.Restock{{Item: lesserHealingPotion, Min: 10000, Max: 10000}}
			p.Shop.KeepAdena = 19000
			return p
		},
		brain: withTravel(travel),
	})
	toGrocer(t, ctx, tg, gm, f)
	from := f.seen()
	if err := gm.Select(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	gmGive(t, ctx, gm, f, adena, 10000)
	gmGive(t, ctx, gm, f, adena, 10000)
	t.Logf("%s seeded with adena in the village", f.name)

	until(t, ctx, "the merchant visit", func() string {
		if !rose(f.counts(lesserHealingPotion, from)) || !fell(f.counts(adena, from)) {
			return "potions bought"
		}
		return ""
	})
	var landed botclient.Point
	until(t, ctx, "the gatekeeper hop", func() string {
		p, ok := f.hopped(from)
		if !ok {
			return "a teleport away"
		}
		landed = p
		return ""
	})
	t.Logf("%s hopped to %v; potions %v, adena %v", f.name, landed,
		f.counts(lesserHealingPotion, from), f.counts(adena, from))
	gmTo(t, ctx, gm, f)
	t.Logf("the GM sees %s at %v", f.name, landed)
}

// hopped reports where f's first teleport since index from landed farther
// than hopAway from Talking Island village.
func (f *farmer) hopped(from int) (botclient.Point, bool) {
	for _, s := range f.watch.Seen(from) {
		if tp, ok := s.Event.(botclient.TeleportToLocation); ok && tp.ObjectID == f.id && dist(tp.Pos, townTI) > hopAway {
			return tp.Pos, true
		}
	}
	return botclient.Point{}, false
}

// TestLiveNewbieGuideBuffs sets a bot in Talking Island village to level 8:
// it asks the Newbie Guide (#32), which casts the newbie buffs on it.
func TestLiveNewbieGuideBuffs(t *testing.T) {
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 10*time.Minute)
	travel := travelMap(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("g", 1)
	f := runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.HumanFighter,
		persona: func() botclient.Persona {
			p := partyPersona()
			p.Newbie = botclient.DefaultPersona().Newbie
			return p
		},
		brain: withTravel(travel),
	})
	toTown(t, ctx, tg, gm, f)
	if err := gm.Select(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	if err := gm.Command(ctx, "set level 8"); err != nil {
		t.Fatal(err)
	}
	// Every event since login: a bot that entered at level 8 asks the
	// guide on its own. The guide has the player cast its buffs on itself
	// (newbiehelper.go SupportMagic: CallSkill(p, p, buff)), so the caster
	// is the bot; at level 8 a fighter gets Wind Walk alone
	// (newbieBuffs.xml).
	_, seen, err := f.watch.Await(ctx, 0, func(ev botclient.Event) bool {
		m, ok := ev.(botclient.MagicSkillUse)
		return ok && m.TargetID == f.id && m.SkillID == windWalk
	})
	if err != nil {
		t.Fatalf("the guide buffs %s: %v", f.name, err)
	}
	m := seen.Event.(botclient.MagicSkillUse)
	t.Logf("%s got skill %d level %d from the guide at level %d", f.name, m.SkillID, m.Level, f.bot.Status().Level)
}

// windWalk is the newbie buff of levels 8-24 (newbieBuffs.xml).
const windWalk int32 = 4322
