package botclient

import (
	"bytes"
	"testing"
	"time"
)

// TestSevenSignsPriestHeld checks that a link of the priest stop the
// bypass gate holds is offered again, the stop where it was, and moves the
// stop on only once it goes out: the stone exchange or multisell 500 from
// the greeting, and SevenSigns 17 4 from the exchange page, decided within
// bypassGap of the exchange link.
func TestSevenSignsPriestHeld(t *testing.T) {
	page := func(html string) []Event { return []Event{NpcHtmlMessage{ObjectID: priestObj, HTML: html}} }
	tests := []struct {
		name     string
		items    []Item
		steps    []trainerStep
		page     string
		at       time.Duration
		gateAt   time.Duration // a bypass that went out then, 0 for none
		want     string
		wantStep priestStep
	}{
		{
			name: "exchange from the greeting", items: []Item{redStones(30), ancientAA(2400), priestAdena(10000)},
			page: duskPriest2a, at: 2 * time.Second, gateAt: 1900 * time.Millisecond,
			want: priestExchange, wantStep: priestExchanging,
		},
		{
			name: "shop from the greeting", items: []Item{ancientAA(2400), priestAdena(10000)},
			page: duskPriest2a, at: 2 * time.Second, gateAt: 1900 * time.Millisecond,
			want: priestShop, wantStep: priestList,
		},
		{
			name: "all stones from the exchange page", items: []Item{redStones(30), ancientAA(2400), priestAdena(10000)},
			steps: []trainerStep{
				{name: "exchange stones", at: 2 * time.Second, events: page(duskPriest2a), wantRule: "signs", wantKind: actBypass, wantPayload: priestBypass(priestExchange)},
			},
			page: signs16Dusk, at: 2300 * time.Millisecond,
			want: priestAllStone, wantStep: priestExchanged,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := priestBrain(t, tt.items...)
			runTrainerSteps(t, b, append([]trainerStep{{name: "talk", at: time.Second, wantRule: "signs", wantKind: actTalk}}, tt.steps...))
			if tt.gateAt > 0 {
				b.pace.sent(gateBypass, at(tt.gateAt))
			}
			now := at(tt.at)
			if err := b.handle(page(tt.page), now); err != nil {
				t.Fatal(err)
			}
			step := b.signs.step
			for i, d := range []time.Duration{0, 100 * time.Millisecond, bypassGap} {
				now := now.Add(d)
				rule, a := b.decide(now)
				if rule != "signs" || a.kind != actBypass || !bytes.Equal(a.payload, priestBypass(tt.want)) {
					t.Fatalf("decide %d at +%v = %q %d % x, want %q", i, d, rule, a.kind, a.payload, tt.want)
				}
				sent, err := b.send(a, now)
				if err != nil {
					t.Fatal(err)
				}
				if last := i == 2; sent != last {
					t.Fatalf("decide %d at +%v: sent %v, want %v (bypass gate)", i, d, sent, last)
				}
				if !sent && b.signs.step != step {
					t.Fatalf("decide %d: a held link moved the stop to %d", i, b.signs.step)
				}
			}
			if b.signs.step != tt.wantStep {
				t.Fatalf("after the link went out: step %d, want %d", b.signs.step, tt.wantStep)
			}
		})
	}
}
