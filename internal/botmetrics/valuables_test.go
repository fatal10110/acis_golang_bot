package botmetrics

import (
	"encoding/json"
	"maps"
	"testing"
	"time"
)

// TestRegistryValuables checks that the valuables counters sum over the
// bots, the sales keyed by via, and that the held gauge sums each bot's
// latest value.
func TestRegistryValuables(t *testing.T) {
	r := NewRegistry()
	a, b := r.NewBot(), r.NewBot()
	var none *Bot
	none.ValuablesAd()
	none.ValuablesHeld(3)
	a.ValuablesAd()
	b.ValuablesAd()
	a.ValuablesReply()
	a.ValuablesSold(ViaTrade)
	b.ValuablesSold(ViaStore)
	b.ValuablesSold(ViaStore)
	b.ValuablesSold("other")
	a.ValuablesDeposited()
	a.ValuablesReleased(2)
	b.ValuablesReleased(0)
	a.ValuablesHeld(4)
	a.ValuablesHeld(3)
	b.ValuablesHeld(2)

	s := r.Collect(time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC))
	type counts struct {
		ads, replies, deposits, released uint64
		held                             int64
	}
	if got, want := (counts{s.ValuablesAds, s.ValuablesReplies, s.ValuablesDeposits, s.ValuablesReleased, s.ValuablesHeld}), (counts{2, 1, 1, 2, 5}); got != want {
		t.Errorf("valuables = %+v, want %+v", got, want)
	}
	if want := map[string]uint64{ViaTrade: 1, ViaStore: 2}; !maps.Equal(s.ValuablesSold, want) {
		t.Errorf("sold = %v, want %v", s.ValuablesSold, want)
	}

	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"valuables_ads_total", "valuables_replies_total", "valuables_sold_total", "valuables_deposits_total", "valuables_released_total", "valuables_held"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("snapshot JSON has no %s: %s", name, raw)
		}
	}
	if got := string(fields["valuables_sold_total"]); got != `{"store":2,"trade":1}` {
		t.Errorf("valuables_sold_total = %s, want by via", got)
	}
}
