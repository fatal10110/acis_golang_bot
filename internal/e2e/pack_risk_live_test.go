package e2e

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// riskWatch is how long TestLivePackRisk watches its farmer pick: past a
// few spotRetry rounds.
const riskWatch = 4 * time.Minute

// TestLivePackRisk logs a level 14 human fighter in Squire's Sword, Shirt
// and Pants, carrying 13 Lesser Healing Potions, in at Talking Island's
// village with SpotPolicy.PackRisk 3 (#294): its own stats pass over
// gludio31_1625_10m1, the werewolf pack that killed it in
// TestLivePackSpot, before any death there. It fails at the werewolf spot
// held, or at a death, within riskWatch.
func TestLivePackRisk(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	const wolves = "gludio31_1625_10m1"
	ctx := scenarioCtx(t, riskWatch+10*time.Minute)
	_, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("pr", 1)
	strip(t, ctx, tg, gm, with(items, consumables...), account, &botclient.HumanFighter, stripEnd{
		at:    townTI,
		level: 14,
		carry: map[int32]int32{lesserHealingPotion: 13},
	}, squiresSword, squiresShirt, squiresPants)
	f := runFieldFarmer(t, ctx, tg, account, func(p *botclient.Persona) {
		p.Potions = []int32{lesserHealingPotion}
		p.Spots.PackRisk = 3
	})
	logStats(t, f)

	start := time.Now()
	var last string
	claimed := false
	seen := 0
	for time.Since(start) < riskWatch {
		if err := ctx.Err(); err != nil {
			t.Fatalf("%s: %v", f.name, err)
		}
		st := f.bot.Status()
		if st.Spot != last {
			last = st.Spot
			claimed = claimed || st.Spot != ""
			t.Logf("%s rule %q spot %q at %v, HP %d/%d", f.name, st.Rule, st.Spot, st.Position, st.HP, st.MaxHP)
		}
		if st.Spot == wolves {
			t.Log(f.digest())
			t.Fatalf("%s holds %s, a pack its stats pass over", f.name, wolves)
		}
		events := f.watch.Seen(seen)
		for _, s := range events {
			if d, ok := s.Event.(botclient.Die); ok && d.ObjectID == f.id && s.At.After(start) {
				t.Log(f.digest())
				t.Fatalf("%s died holding %q", f.name, st.Spot)
			}
		}
		seen += len(events)
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
	if !claimed {
		t.Log(f.digest())
		t.Fatalf("%s claimed no spot in %v", f.name, riskWatch)
	}
	t.Logf("%s: spot %q after %v", f.name, last, riskWatch)
}
