package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// packWatch is how long TestLivePackSpot watches its farmer: about the 25
// minutes in which the farmer of #281 died 4 to 6 times.
const packWatch = 20 * time.Minute

// TestLivePackSpot logs a level 14 human fighter in Squire's Sword, Shirt
// and Pants, carrying 13 Lesser Healing Potions, in at the centre of
// gludio31_1625_10m1, Talking Island's pack of ten aggressive level 10-12
// werewolves (run 118.8, faster than its 115) that call each other within
// 300 (#281). The default persona has it fight back rather than flee from
// the faster pack, drink as it fights, pass over the packs like the one a
// death taught it to, and judge each pack by the model with its row's
// margin (#330): its death penalty lifted (strip), the model has it last
// about 84 s against the two kills' 76 s its margin of 2 asks for
// (TestPackFit), so it farms the werewolves. The watch is the fit's check
// (#294): it fails at a third death, or at a spot it died holding held
// again, within packWatch. With its death penalty on, the farmer died at
// the werewolves, then the orcs, then gludio31_1624_01m1 in 9 minutes.
func TestLivePackSpot(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	dp, err := datapack.Load(tg.Config().Datapack)
	if err != nil {
		t.Fatal(err)
	}
	var pack datapack.Spot
	for _, s := range dp.Spots() {
		if s.Maker == "gludio31_1625_10m1" {
			pack = s
		}
	}
	if pack.Maker == "" {
		t.Fatal("no spot gludio31_1625_10m1 in the datapack")
	}
	ctx := scenarioCtx(t, packWatch+10*time.Minute)
	_, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("pk", 1)
	strip(t, ctx, tg, gm, with(items, consumables...), account, &botclient.HumanFighter, stripEnd{
		at:    botclient.Point{X: pack.Center.X, Y: pack.Center.Y, Z: pack.Z},
		level: 14,
		carry: map[int32]int32{lesserHealingPotion: 13},
	}, squiresSword, squiresShirt, squiresPants)
	f := runFieldFarmer(t, ctx, tg, account, func(p *botclient.Persona) {
		p.Potions = []int32{lesserHealingPotion}
	})
	logStats(t, f)
	gmTeleport(t, ctx, gm, townTI)
	t.Logf("%s at %v", f.name, f.bot.Status().Position)

	start := time.Now()
	died := map[string]bool{}
	var deaths int
	var held, last string
	var lastAt, diedAt time.Time
	seen := 0
	for time.Since(start) < packWatch {
		if err := ctx.Err(); err != nil {
			t.Fatalf("%s: %v after %d deaths", f.name, err, deaths)
		}
		st := f.bot.Status()
		// A watch over a bot that is not online proves nothing.
		if st.State != botclient.StateOnline && !st.At.IsZero() && time.Since(st.At) > noteEvery {
			t.Log(f.digest())
			t.Fatalf("%s %v since %v after %d deaths", f.name, st.State, st.At.Format(time.TimeOnly), deaths)
		}
		if now := fmt.Sprintf("rule %q spot %q", st.Rule, st.Spot); now != last || time.Since(lastAt) >= noteEvery {
			last, lastAt = now, time.Now()
			t.Logf("%s %s, %d potions at %v, level %d, HP %d/%d", f.name, now, f.carries(lesserHealingPotion), st.Position, st.Level, st.HP, st.MaxHP)
		}
		// The status board lags the bot by up to statusEvery: one from
		// before its last death still shows the spot it died holding.
		if st.Spot != "" && st.At.After(diedAt) {
			if died[st.Spot] {
				t.Log(f.digest())
				t.Fatalf("%s holds %s again, a spot it died holding", f.name, st.Spot)
			}
			held = st.Spot
		}
		events := f.watch.Seen(seen)
		for _, s := range events {
			if d, ok := s.Event.(botclient.Die); ok && d.ObjectID == f.id && s.At.After(start) {
				deaths++
				died[held], diedAt = true, s.At
				t.Logf("%s died holding %q at %v (death %d)", f.name, held, s.At.Format(time.TimeOnly), deaths)
			}
		}
		seen += len(events)
		if deaths > 2 {
			t.Log(f.digest())
			t.Fatalf("%s died %d times in %v", f.name, deaths, time.Since(start).Round(time.Second))
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
	t.Logf("%s: %d deaths in %v, last spot %q, %d potions left", f.name, deaths, packWatch, held, f.carries(lesserHealingPotion))
}

// logStats logs, at the end of the test, the combat stats of every UserInfo
// of f that changed them: the ones its spot fit weighs packs with
// (packFits; TestPackFit's squires).
func logStats(t *testing.T, f *farmer) {
	t.Helper()
	t.Cleanup(func() {
		var last botclient.UserInfo
		for _, s := range f.watch.Seen(0) {
			u, ok := s.Event.(botclient.UserInfo)
			if !ok || u.ObjectID != f.id || (u.Level == last.Level && u.PAtk == last.PAtk && u.PDef == last.PDef &&
				u.Evasion == last.Evasion && u.Accuracy == last.Accuracy && u.PAtkSpd == last.PAtkSpd && u.MaxHP == last.MaxHP) {
				continue
			}
			last = u
			t.Logf("%s at %v level %d: pAtk %d, pDef %d, evasion %d, accuracy %d, attack speed %d, max HP %d",
				f.name, s.At.Format(time.TimeOnly), u.Level, u.PAtk, u.PDef, u.Evasion, u.Accuracy, u.PAtkSpd, u.MaxHP)
		}
	})
}
