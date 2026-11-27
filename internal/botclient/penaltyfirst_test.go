package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// recoveryPrice is a Scroll: Recovery's base price at the merchants
// (buyLists.xml, 2300 at the cheapest).
const recoveryPrice int32 = 2300

// recoveryMerchant is the template of the merchant selling the Scroll:
// Recovery in the penalty trip tests.
const recoveryMerchant int32 = 30001

// newPenaltyTripBrain is newTripBrain (a level 5 farmer at its spot) with
// death penalty level dp, a merchant spawned in its town selling the
// no-grade Scroll: Recovery at recoveryPrice and the Scroll of Escape at
// soePrice, and
// items carried, plus adena past the supplies tier's floor.
func newPenaltyTripBrain(t *testing.T, dp, past int32, items ...Item) *Brain {
	t.Helper()
	b := newTripBrain(t)
	m := b.cfg.Travel
	m.shops, m.cheapest = indexShops([]datapack.BuyList{{ID: 1, NPCID: recoveryMerchant, Products: []datapack.Product{
		{ItemID: recoveryNoGrade, Price: recoveryPrice}, {ItemID: scrollOfEscape, Price: soePrice},
	}}})
	m.spawns = map[int32][]Point{recoveryMerchant: {{X: town.X + 500}}}
	m.spawnedSold[recoveryNoGrade] = true
	m.spawnedSold[scrollOfEscape] = true
	b.world.Apply(EtcStatusUpdate{DeathPenalty: dp}, t0)
	adena := int32(b.floor(tierSupplies)) + past
	b.world.Apply(ItemList{Items: append([]Item{potionStack(20), adenaStack(adena)}, items...)}, t0)
	return b
}

// TestSupplyCausePenalty checks the death penalty's supply trip (#491): a
// farmer with a penalty and no Scroll: Recovery of its band goes home
// when the town sells one its adena past the supplies floor pays, ahead of
// every other trigger; not with the scroll carried, nor when it cannot buy
// one, nor when its persona keeps the penalty.
func TestSupplyCausePenalty(t *testing.T) {
	tests := []struct {
		name  string
		dp    int32
		past  int32 // adena past the supplies floor
		items []Item
		edit  func(b *Brain)
		want  tripCause
	}{
		{name: "penalty, no scroll, adena pays one", dp: 1, past: recoveryPrice, want: causePenalty},
		{name: "ahead of potions gone", dp: 2, past: recoveryPrice, edit: func(b *Brain) {
			b.world.Apply(ItemList{Items: []Item{adenaStack(int32(b.floor(tierSupplies)) + recoveryPrice)}}, t0)
			b.supplyTrip.potions = true
		}, want: causePenalty},
		{name: "no penalty", dp: 0, past: recoveryPrice},
		{name: "carries its band's scroll", dp: 2, past: recoveryPrice, items: []Item{{ObjectID: 0x30000031, TemplateID: recoveryNoGrade, Count: 1}}},
		{name: "carries another band's scroll", dp: 1, past: recoveryPrice, items: []Item{{ObjectID: 0x30000032, TemplateID: recoveryD, Count: 1}}, want: causePenalty},
		{name: "adena short of one", dp: 1, past: recoveryPrice - 1},
		{name: "no spawned merchant sells it", dp: 1, past: recoveryPrice, edit: func(b *Brain) {
			delete(b.cfg.Travel.spawnedSold, recoveryNoGrade)
			b.cfg.Travel.spawns = nil
		}},
		{name: "sold only in a town the trip does not reach", dp: 1, past: recoveryPrice, edit: func(b *Brain) {
			far := Point{X: 60000}
			b.cfg.Travel.gatekeepers = append(b.cfg.Travel.gatekeepers, gatekeeper{npcID: townGK + 1, at: far})
			b.cfg.Travel.spawns = map[int32][]Point{recoveryMerchant: {far}}
		}},
		{name: "not sold", dp: 1, past: recoveryPrice, edit: func(b *Brain) { delete(b.cfg.Travel.cheapest, recoveryNoGrade) }},
		{name: "the persona keeps its penalty", dp: 1, past: recoveryPrice, edit: func(b *Brain) { b.persona.Recover.LiftDeathPenalty = false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newPenaltyTripBrain(t, tt.dp, tt.past, tt.items...)
			if tt.edit != nil {
				tt.edit(b)
			}
			if got := b.supplyCause(); got != tt.want {
				t.Fatalf("supplyCause = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestSupplyDuePenalty checks that the penalty's trip starts at the spot's
// next trigger read and keeps the trip limiter: none within supplyTripGap
// of the last trip.
func TestSupplyDuePenalty(t *testing.T) {
	b := newPenaltyTripBrain(t, 1, recoveryPrice)
	if got := b.supplyDue(at(time.Second)); got != causePenalty {
		t.Fatalf("supplyDue = %d, want causePenalty", got)
	}
	b = newPenaltyTripBrain(t, 1, recoveryPrice)
	b.supplyTrip.endedAt = at(time.Second)
	if got := b.supplyDue(at(time.Second + supplyTripGap - time.Millisecond)); got != causeNone {
		t.Fatalf("supplyDue within supplyTripGap = %d, want none", got)
	}
	if got := b.supplyDue(at(time.Second + supplyTripGap)); got != causePenalty {
		t.Fatalf("supplyDue past supplyTripGap = %d, want causePenalty", got)
	}
}

// TestRecoveryFirstInBudget checks that the Scrolls: Recovery come first
// in the supplies tier (#491): adena that pays one scroll and one Scroll
// of Escape past the reserve buys the scroll before the second Scroll of
// Escape, where the old order bought both Scrolls of Escape and no
// Scroll: Recovery.
func TestRecoveryFirstInBudget(t *testing.T) {
	const keep = 1000
	adena := keep + recoveryPrice + soePrice
	b := newBudgetBrain(t, adena)
	b.cfg.Travel.cheapest[recoveryNoGrade] = recoveryPrice
	b.cfg.Travel.spawnedSold[recoveryNoGrade] = true
	b.world.Apply(EtcStatusUpdate{DeathPenalty: 1}, t0)
	if first := b.wants(false)[0]; first.buyRow != (buyRow{Item: recoveryNoGrade, Count: 1}) || first.tier != tierSupplies {
		t.Fatalf("first want = %v, want one Scroll: Recovery in the supplies tier", first)
	}
	window := []ShopItem{{TemplateID: scrollOfEscape, Price: soePrice}, {TemplateID: recoveryNoGrade, Price: recoveryPrice}}
	want := []buyRow{{Item: recoveryNoGrade, Count: 1}, {Item: scrollOfEscape, Count: 1}}
	if got := b.buyRows(BuyList{Money: adena, ListID: 1, Items: window}); !slices.Equal(got, want) {
		t.Fatalf("buyRows = %v, want %v", got, want)
	}
}

// TestRecoverAheadOfRest checks the rule order (#491): a bot low enough on
// HP to sit reads its Scroll: Recovery first.
func TestRecoverAheadOfRest(t *testing.T) {
	b := newRecoveryBrain(t, 1, Item{ObjectID: 0x30000031, TemplateID: recoveryNoGrade, Count: 1})
	b.world.Self.MaxHP, b.world.Self.HP = 100, 20
	b.world.Self.MaxMP, b.world.Self.MP = 100, 20
	if rule, _ := b.decide(at(10 * time.Second)); rule != "recover" {
		t.Fatalf("decided rule %q, want recover", rule)
	}
	b.world.Apply(SystemMessage{ID: msgDeathPenaltyLifted}, at(11*time.Second))
	if rule, _ := b.decide(at(12 * time.Second)); rule != "rest" {
		t.Fatalf("decided rule %q with the penalty lifted, want rest", rule)
	}
}
