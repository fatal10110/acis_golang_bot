package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

// valuablesCounts are a snapshot's valuables counters.
type valuablesCounts struct {
	ads, replies, trade, store, deposits, released uint64
}

func valuablesCountsOf(s *botmetrics.Snapshot) valuablesCounts {
	return valuablesCounts{
		ads: s.ValuablesAds, replies: s.ValuablesReplies,
		trade: s.ValuablesSold[botmetrics.ViaTrade], store: s.ValuablesSold[botmetrics.ViaStore],
		deposits: s.ValuablesDeposits, released: s.ValuablesReleased,
	}
}

// TestValuablesMetrics checks that each valuables counter steps once per
// ad line, reply, sale by kind, deposit and released entry
// (docs/valuables/preset.md "Wiring"), and nothing else steps it.
func TestValuablesMetrics(t *testing.T) {
	sword := valuableKey{template: ledgerSword}
	tests := []struct {
		name  string
		items []Item
		step  func(t *testing.T, b *Brain)
		want  valuablesCounts
	}{
		{
			name: "ad line, the next held by AdEvery", items: []Item{swordUnit(101, 0)},
			step: func(t *testing.T, b *Brain) {
				openAdRound(b, t0)
				b.syncValuables()
				if _, ok := b.adTurn(t0); !ok {
					t.Fatal("adTurn said no line")
				}
				if _, ok := b.adTurn(t0.Add(time.Second)); ok {
					t.Fatal("adTurn said a line within AdEvery")
				}
			},
			want: valuablesCounts{ads: 1},
		},
		{
			name: "whisper reply sent",
			step: func(_ *testing.T, b *Brain) { b.whisperSent(whisperOut{to: 7}) },
			want: valuablesCounts{replies: 1},
		},
		{
			name: "trade sale: a failed trade first", items: []Item{swordUnit(101, 0)},
			step: func(_ *testing.T, b *Brain) {
				b.world.Trade = Trade{PartnerID: 9, valuable: true, key: sword, sale: 136000}
				b.valuableSaleEvent(SendTradeDone{})
				b.valuableSaleEvent(SendTradeDone{Success: true})
			},
			want: valuablesCounts{trade: 1},
		},
		{
			name: "trade of no valuable",
			step: func(_ *testing.T, b *Brain) {
				b.world.Trade = Trade{PartnerID: 9, sale: 136000}
				b.valuableSaleEvent(SendTradeDone{Success: true})
			},
		},
		{
			name: "store close: two of three valuable rows sold", items: []Item{adenaStack(301000), swordUnit(103, 0)},
			step: func(_ *testing.T, b *Brain) {
				b.store.step, b.store.adenaAtOpen = storeListed, 1000
				for _, obj := range []int32{101, 102, 103} {
					b.store.valuables = append(b.store.valuables, listedValuable{row: storeRow{object: obj, count: 1, price: 150000}, key: sword, atList: 1})
				}
				b.countSold()
			},
			want: valuablesCounts{store: 2},
		},
		{
			name: "store close: nothing sold", items: []Item{adenaStack(1000), swordUnit(101, 0)},
			step: func(_ *testing.T, b *Brain) {
				b.store.step, b.store.adenaAtOpen = storeListed, 1000
				b.store.valuables = []listedValuable{{row: storeRow{object: 101, count: 1, price: 150000}, key: sword, atList: 1}}
				b.countSold()
			},
		},
		{
			name: "private deposit made", items: []Item{swordUnit(101, 0)},
			step: func(_ *testing.T, b *Brain) { depositAnswered(b, true) },
			want: valuablesCounts{deposits: 1},
		},
		{
			name: "clan deposit made", items: []Item{swordUnit(101, 0)},
			step: func(_ *testing.T, b *Brain) { depositAnswered(b, false) },
		},
		{
			name: "cap releases two entries once", items: []Item{swordUnit(101, 0), swordUnit(102, 3)},
			step: func(_ *testing.T, b *Brain) {
				b.syncValuables()
				for _, e := range b.cfg.valuables.entries {
					e.rounds = b.persona.Valuables.MaxRounds
				}
				b.ranRound()
				b.ranRound()
			},
			want: valuablesCounts{released: 2},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newAdBrain(t, NewAdBoard(DefaultAdRegionGap, DefaultAdProcessPerHour), 1, tc.items...)
			reg := botmetrics.NewRegistry()
			b.metrics = reg.NewBot()
			tc.step(t, b)
			if got := valuablesCountsOf(reg.Collect(t0)); got != tc.want {
				t.Errorf("counters = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// depositAnswered has b's warehouse errand send a deposit of sword 101,
// private or to the clan, and take the server's InventoryUpdate removing
// it.
func depositAnswered(b *Brain, private bool) {
	e := &b.wh
	e.req = WarehouseRequest{Private: private, Deposit: []WarehouseRow{{TemplateID: ledgerSword, Count: 1, Object: 101}}}
	e.running, e.step, e.rows = true, whSent, []warehouseRow{{object: 101, item: ledgerSword, count: 1}}
	b.warehouseEvent(InventoryUpdate{Changes: []ItemChange{{ItemRemoved, swordUnit(101, 0)}}}, t0)
}

// TestValuablesHeldView checks that the held valuables gauge and the
// inspect's valuables follow the ledger: carried and stored units by
// entry, its rounds, ask and release, none while the policy is off.
func TestValuablesHeldView(t *testing.T) {
	b := newAdBrain(t, nil, 1, swordUnit(101, 0), swordUnit(102, 3), swordUnit(103, 0))
	reg := botmetrics.NewRegistry()
	b.metrics = reg.NewBot()
	b.cfg.valuables.stored = func() (map[valuableKey]int64, bool) {
		return map[valuableKey]int64{{template: ledgerSword, enchant: 3}: 2}, true
	}
	b.syncValuables()
	if got := reg.Collect(t0).ValuablesHeld; got != 5 {
		t.Fatalf("held gauge = %d, want 5 (3 carried, 2 stored)", got)
	}
	plain := b.cfg.valuables.entries[valuableKey{template: ledgerSword}]
	plain.rounds = 2
	plain.released = true
	enchanted := entryOf(t, b, ledgerSword, 3)
	want := []HeldValuable{
		{TemplateID: ledgerSword, Carried: 2, Rounds: 2, Ask: plain.ask, Released: true},
		{TemplateID: ledgerSword, Enchant: 3, Carried: 1, Stored: 2, Ask: enchanted.ask},
	}
	got := b.inspection(t0, false).Valuables
	if len(got) != len(want) {
		t.Fatalf("inspect valuables = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("inspect valuable %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	// The ledger follows a sold unit: the gauge on the next decision.
	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{ItemRemoved, swordUnit(103, 0)}}}, t0)
	b.syncValuables()
	if got := reg.Collect(t0).ValuablesHeld; got != 4 {
		t.Errorf("held gauge after a sale = %d, want 4", got)
	}

	b.persona.Valuables = ValuablePolicy{}
	if got := b.inspection(t0, false).Valuables; got != nil {
		t.Errorf("inspect valuables with the policy off = %+v, want none", got)
	}
}
