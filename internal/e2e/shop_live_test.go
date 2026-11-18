package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveMerchantBuySell has the GM recall a bot next to Talking Island's grocer
// and give it adena and stems: the bot visits a merchant of the travel map,
// buys Lesser Healing Potions with the adena and sells the stems, its junk
// (#31). The evidence is the bot's own inventory packets: its potions rise
// as its adena falls, and its stems go once the merchant's sell window
// opened, with adena coming in.
func TestLiveMerchantBuySell(t *testing.T) {
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 9*time.Minute)
	travel := travelMap(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("m", 1)
	f := runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.HumanFighter,
		persona: func() botclient.Persona {
			p := partyPersona()
			// A minimum it never reaches: every run buys, as far as its
			// adena goes (the potions it bought before stay with it).
			// Every value is a persona default (D6).
			p.Shop = botclient.ShopPolicy{
				Restock: []botclient.Restock{{Item: lesserHealingPotion, Min: 10000, Max: 10000}},
				SellAt:  1,
				Reach:   8000,
			}
			return p
		},
		brain: withTravel(travel),
	})
	toGrocer(t, ctx, tg, gm, f)
	from := f.seen()
	if err := gm.Select(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	gmGive(t, ctx, gm, f, stem, 3)
	gmGive(t, ctx, gm, f, adena, 1000)
	t.Logf("%s seeded with stems and adena in the village", f.name)

	until(t, ctx, "the merchant visit", func() string {
		var missing []string
		if !rose(f.counts(lesserHealingPotion, from)) {
			missing = append(missing, "potions bought")
		}
		if !fell(f.counts(adena, from)) {
			missing = append(missing, "adena spent")
		}
		if !f.soldStems(from) {
			missing = append(missing, "stems sold")
		}
		if len(missing) == 0 {
			return ""
		}
		return fmt.Sprint(missing)
	})
	t.Logf("%s: potions %v, adena %v, stems %v", f.name,
		f.counts(lesserHealingPotion, from), f.counts(adena, from), f.counts(stem, from))
}

// soldStems reports whether, since index from, f opened a merchant's sell
// window, then its stems left its inventory and adena came in at or after
// that.
func (f *farmer) soldStems(from int) bool {
	seen := f.watch.Seen(0)
	window, gone := false, -1
	for i := from; i < len(seen); i++ {
		ev := seen[i].Event
		if _, ok := ev.(botclient.SellList); ok {
			window = true
		}
		if n, _, ok := stock(ev, stem); ok && n == 0 && window && f.stockAt(stem, i) > 0 {
			gone = i
			break
		}
	}
	if gone < 0 {
		return false
	}
	return rose(f.counts(adena, gone))
}
