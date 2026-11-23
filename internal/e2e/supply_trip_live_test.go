package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// Supplies the scenarios seed (docs/items.md section 4, defaultSupplyPolicy):
// tripAdena pays a trip's no-grade shots for a Squire's Sword (about 3300 at
// 7 adena), its Lesser Healing Potions and its Scrolls of Escape past the
// reserve; tripPotions is a trip's potions (10 an hour, a fifth more);
// lowShots runs out within ReturnMinutes (10) at the sword's first rate
// (45 a minute) but stays above the persona's Refill of 1 % of the trip
// target, so the supply trip fires and the shop errand does not.
const (
	tripAdena   = 30000
	tripPotions = 12
	lowShots    = 200
	// escapes is DefaultPersona's Supply.Escapes.
	escapes = 2
	// buyLoad is DefaultPersona's Supply.BuyLoad, in percent.
	buyLoad = 45
)

// TestLiveTownPrep logs a farmer stripped of every consumable in next to
// Talking Island's grocer with adena (#170, #182): before it claims a
// spot, its town preparation buys no-grade shots, Lesser Healing Potions
// and two Scrolls of Escape up to its trip targets, under 45 % of its weight
// limit; it then farms, the scrolls kept, never sold back.
func TestLiveTownPrep(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &newbieSpots)
	ctx := scenarioCtx(t, 15*time.Minute)
	_, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("tp", 1)
	strip(t, ctx, tg, gm, with(items, consumables...), account, &botclient.HumanFighter, stripEnd{at: nearGrocer, adena: tripAdena}, squiresSword, squiresShirt, squiresPants)
	f := runFieldFarmer(t, ctx, tg, account, func(p *botclient.Persona) {
		p.Potions = []int32{lesserHealingPotion}
	})

	var farmAt time.Time
	untilServed(t, ctx, "the town preparation, then a spot", f.watch, merchantBuyRefused(f.watch), func() string {
		sent := sentOf(f.watch.Sent(), botclient.OpRequestBuyItem)
		switch {
		case len(sent) == 0:
			return fmt.Sprintf("no RequestBuyItem (rule %q)", f.bot.Status().Rule)
		case f.carries(soulshotNG) == 0:
			return "no shots bought"
		case f.carries(lesserHealingPotion) == 0:
			return "no potions bought"
		case f.carries(scrollOfEscape) < escapes:
			return fmt.Sprintf("%d Scrolls of Escape", f.carries(scrollOfEscape))
		case f.bot.Status().Rule != "farm":
			return fmt.Sprintf("not farming (rule %q)", f.bot.Status().Rule)
		}
		farmAt = time.Now()
		return ""
	})
	buys := sentOf(f.watch.Sent(), botclient.OpRequestBuyItem)
	if last := buys[len(buys)-1].At; last.After(farmAt) {
		t.Errorf("%s bought at %v, after it farmed at %v", f.name, last, farmAt)
	}
	if cur, limit := f.load(); limit == 0 || int64(cur)*100 > int64(limit)*buyLoad {
		t.Errorf("%s carries %d of %d, over %d %%", f.name, cur, limit, buyLoad)
	}
	t.Logf("%s farms with %d shots, %d potions, %d scrolls (%d buys)", f.name,
		f.carries(soulshotNG), f.carries(lesserHealingPotion), f.carries(scrollOfEscape), len(buys))
	// Farming a while does not sell or read a scroll.
	waitFor(t, ctx, "a minute of farming", func() bool { return time.Since(farmAt) > time.Minute })
	if soldTemplate(f.watch.Seen(0), f.watch.Sent(), scrollOfEscape) || f.carries(scrollOfEscape) < escapes {
		t.Errorf("%s kept %d Scrolls of Escape", f.name, f.carries(scrollOfEscape))
	}
}

// TestLiveSupplyTrip logs a level supplyLevel farmer in at the west of
// Talking Island, out of every merchant's reach, carrying potions, two
// Scrolls of Escape, adena and shots that run out within ten minutes of
// fighting (#175, #182): at its starter spot, where nearly every monster
// attacks on sight,
// it reads a scroll (#251), lands in the village, restocks its shots and
// scrolls before it leaves, then farms at a spot again. It carries them
// from its login, so its use meter never sees it fight without shots,
// which would leave its shot rate at zero. Any other way home fails: a
// walk (no safe moment within Supply.SafeWait), or a death at its spot,
// before the trip or during the read.
func TestLiveSupplyTrip(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &newbieSpots)
	// Farming to the run-out trigger takes about ten minutes, then the
	// safe wait, the walk home and the restock.
	ctx := scenarioCtx(t, 30*time.Minute)
	_, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("su", 1)
	start := time.Now()
	strip(t, ctx, tg, gm, with(items, consumables...), account, &botclient.HumanFighter, stripEnd{
		at:    tiToads,
		level: supplyLevel,
		adena: tripAdena,
		carry: map[int32]int32{lesserHealingPotion: tripPotions, scrollOfEscape: escapes, soulshotNG: lowShots},
	}, squiresSword, squiresShirt, squiresPants)
	f := runFieldFarmer(t, ctx, tg, account, func(p *botclient.Persona) {
		// Every value is a persona default (D6): at its spot the shop
		// errand wants no supply until 1 % of its trip target is left,
		// no gear and no sale, so the supply trip is what sends it home.
		p.Supply.Refill = 1
		p.Gear.Buy = false
		p.Shop.SellAt = 0
		p.Potions = []int32{lesserHealingPotion}
	})
	gmTeleport(t, ctx, gm, townTI)
	t.Logf("%s low on shots at %v", f.name, f.bot.Status().Position)

	// Home is the recall's landing after a Scroll of Escape. A walk to the
	// village, or a death and the respawn there, fails at once with the way
	// it took.
	var read, landed, tripAt time.Time
	var how string
	var last string
	var lastAt time.Time
	// note logs the bot's rule, spot and walk on every change, and with its
	// supplies, position and HP at least every noteEvery, so a bot that
	// stands still shows in the log.
	note := func() {
		st := f.bot.Status()
		if now := fmt.Sprintf("rule %q spot %q walk %q", st.Rule, st.Spot, st.Walk); now != last || time.Since(lastAt) >= noteEvery {
			last, lastAt = now, time.Now()
			t.Logf("%s %s, %d shots, %d potions, %d scrolls, %d adena at %v, level %d, HP %d/%d, state %v", f.name, now,
				f.carries(soulshotNG), f.carries(lesserHealingPotion), f.carries(scrollOfEscape), f.carries(adena), st.Position, st.Level, st.HP, st.MaxHP, st.State)
		}
	}
	untilServed(t, ctx, "the trip home", f.watch, escapeRefused(f.watch, objectsOf(f.watch.Seen(0), scrollOfEscape)), func() string {
		note()
		st := f.bot.Status()
		if tripAt.IsZero() && st.Rule == "supply" {
			tripAt = time.Now()
		}
		// The landing is the first teleport after the latest read. The
		// server's respawn is a teleport too (Revive, then the teleport to
		// the restart point), so a Revive of the bot between the read and
		// that teleport is a death during the read, not the recall.
		if used := f.usedSince(scrollOfEscape, start); len(used) > 0 {
			read = used[len(used)-1].At
			var revived time.Time
			for _, s := range f.watch.Seen(0) {
				if !s.At.After(read) {
					continue
				}
				if r, ok := s.Event.(botclient.Revive); ok && r.ObjectID == f.id && revived.IsZero() {
					revived = s.At
				}
				if tp, ok := s.Event.(botclient.TeleportToLocation); ok && tp.ObjectID == f.id {
					if !revived.IsZero() {
						t.Log(f.digest())
						t.Fatalf("%s home with no recall: died during the read (scroll read at %v, revived at %v)", f.name, read.Format(time.TimeOnly), revived.Format(time.TimeOnly))
					}
					landed, how = s.At, fmt.Sprintf("read its scroll at %v", read.Format(time.TimeOnly))
					return ""
				}
			}
		}
		if dist(st.Position, townTI) < walkedHome {
			way := "walked home, no safe moment for its scroll"
			for _, s := range f.watch.Seen(0) {
				if r, ok := s.Event.(botclient.Revive); ok && r.ObjectID == f.id && s.At.After(start) {
					way = "died on the trip and respawned in the village"
					if tripAt.IsZero() || s.At.Before(tripAt) {
						way = "died at its spot before the trip and respawned in the village"
					}
				}
			}
			t.Log(f.digest())
			t.Fatalf("%s home with no recall: %s (scroll read %v, trip seen %v, rule %q)", f.name, way, !read.IsZero(), !tripAt.IsZero(), st.Rule)
		}
		return fmt.Sprintf("not home: scroll read %v, trip seen %v, rule %q at %v", !read.IsZero(), !tripAt.IsZero(), st.Rule, st.Position)
	})
	t.Logf("%s %s, home at %v", f.name, how, landed.Format(time.TimeOnly))
	cur, limit := f.load()
	t.Logf("%s home with %d shots, %d potions, %d scrolls, %d adena, load %d/%d", f.name, f.carries(soulshotNG), f.carries(lesserHealingPotion), f.carries(scrollOfEscape), f.carries(adena), cur, limit)
	from := 0
	for i, s := range f.watch.Seen(0) {
		if !s.At.Before(landed) {
			from = i
			break
		}
	}
	var farmAt time.Time
	untilServed(t, ctx, "the restock, then a spot", f.watch, merchantBuyRefused(f.watch), func() string {
		note()
		var bought bool
		for _, p := range sentOf(f.watch.Sent(), botclient.OpRequestBuyItem) {
			bought = bought || p.At.After(landed)
		}
		st := f.bot.Status()
		switch {
		case !bought && st.Spot != "" && dist(st.Position, townTI) > walkedHome:
			// Out of town again with nothing bought: fail at once, with
			// the visits the bot made.
			t.Log(f.digest())
			t.Log(f.visits())
			t.Fatalf("%s left town for spot %q with nothing bought since the landing", f.name, st.Spot)
		case !bought:
			return fmt.Sprintf("no RequestBuyItem since the landing (rule %q)", f.bot.Status().Rule)
		case !rose(f.counts(soulshotNG, from)):
			return "no shots bought"
		case f.carries(scrollOfEscape) < escapes:
			return fmt.Sprintf("%d Scrolls of Escape", f.carries(scrollOfEscape))
		case f.bot.Status().Rule != "farm":
			return fmt.Sprintf("not farming (rule %q)", f.bot.Status().Rule)
		}
		farmAt = time.Now()
		return ""
	})
	for _, p := range sentOf(f.watch.Sent(), botclient.OpRequestBuyItem) {
		if p.At.After(farmAt) {
			t.Errorf("%s bought at %v, after it farmed again at %v", f.name, p.At, farmAt)
		}
	}
	t.Logf("%s farms again with %d shots (%v), %d scrolls", f.name, f.carries(soulshotNG), f.counts(soulshotNG, from), f.carries(scrollOfEscape))
}

// tiToads is open ground in the west of Talking Island, past the
// Shop.Reach (8000) of every village merchant, about 1000 from the centre
// of gludio32_1725_28m1 (a farmer walked through it live): four Giant
// Toads (20121, level 5, aggressive), one of the toad spots a level
// supplyLevel farmer fits.
var tiToads = botclient.Point{X: -94872, Y: 246888, Z: -3633}

// supplyLevel is the level TestLiveSupplyTrip's farmer starts at, set by
// the GM each run (#258). Left to the level it had (TestLiveSupport sets
// the same character to 15 for its leader, and farming adds or takes
// more), the farmer in its Squire's gear stood at 14 to 16, where every Talking Island
// spot that fits it is a pack of 9 to 11 aggressive level 10 to 12 orcs or
// werewolves that assist each other within 300 and outrun it (runSpd
// 118.8): it died there again and again and never farmed. Level 6 fits
// the toad spots only, whose monsters are slower than it.
const supplyLevel = 6

// walkedHome is how close to the village centre a walk home has arrived:
// the walk ends within 600 of the gatekeeper (townArrive), next to it.
const walkedHome = 2000

// noteEvery is how often the supply trip scenario logs the bot's position
// while nothing else about it changes.
const noteEvery = 30 * time.Second

// load is f's carried weight and weight limit as its last UserInfo or
// StatusUpdate showed them.
func (f *farmer) load() (cur, limit int32) {
	for _, s := range f.watch.Seen(0) {
		switch e := s.Event.(type) {
		case botclient.UserInfo:
			cur, limit = e.Load, e.MaxLoad
		case botclient.StatusUpdate:
			if e.ObjectID != f.id {
				continue
			}
			if e.Set&botclient.StatusLoad != 0 {
				cur = e.Load
			}
			if e.Set&botclient.StatusMaxLoad != 0 {
				limit = e.MaxLoad
			}
		}
	}
	return cur, limit
}
