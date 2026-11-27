package botclient

import (
	"maps"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// fixedSoulshots and fixedSpiritshots are a ShotStock a persona sets, the
// one the default persona carried before the use meter sized shots.
var (
	fixedSoulshots   = ShotStock{Min: 100, Max: 500}
	fixedSpiritshots = ShotStock{Min: 50, Max: 200}
)

// soulOre is Soul Ore, an extra supply of the tests.
const soulOre int32 = 1785

// meterSupply is the supply policy of the use meter tests: an hour of
// fighting, no margin.
var meterSupply = SupplyPolicy{TripMinutes: 60, BuyLoad: 45, Refill: 25, PotionsPerHour: 10, Escapes: 2}

// newMeterBrain is newShotBrain for class with shots sized by the use
// meter (meterSupply), the no-grade sword at attack speed 500 (60 attacks a
// minute), the supplies' weights, and the grocer selling Lesser Healing
// Potions and Scrolls of Escape too.
func newMeterBrain(t *testing.T, class int32, items ...Item) *Brain {
	t.Helper()
	b := newShotBrain(t, class, false, items...)
	b.persona.Shop.Soulshots, b.persona.Shop.Spiritshots = ShotStock{}, ShotStock{}
	b.persona.Supply = meterSupply
	m := b.cfg.Travel
	m.weapons[ngSword] = weapon{soulshots: 1, spiritshots: 1, atkSpd: 500}
	m.weights = map[int32]int32{soulshotNoGrade: 4, blessedNoGrade: 4, lesserHealingPotion: 5, scrollOfEscape: 120, soulOre: 10}
	m.spawnedSold[lesserHealingPotion], m.spawnedSold[scrollOfEscape] = true, true
	return b
}

func potionStack(n int32) Item {
	return Item{ObjectID: 3, TemplateID: lesserHealingPotion, Count: n, SubCategory: 5}
}

// fight has the bot in combat from from to to, spending perMin of stack obj
// a minute in an InventoryUpdate every 10 s.
func fight(t *testing.T, b *Brain, obj int32, from, to time.Duration, perMin int32) {
	t.Helper()
	self := b.world.Self.ObjectID
	handle := func(ev Event, d time.Duration) {
		if err := b.handle([]Event{ev}, at(d)); err != nil {
			t.Fatal(err)
		}
	}
	handle(AutoAttackStart{ObjectID: self}, from)
	for d := from + 10*time.Second; d <= to; d += 10 * time.Second {
		it := b.world.Inventory.Items[obj]
		it.Count -= perMin / 6
		handle(InventoryUpdate{Changes: []ItemChange{{State: ItemModified, Item: it}}}, d)
	}
	handle(AutoAttackStop{ObjectID: self}, to)
}

// TestUseMeterFirstEstimate checks the trip targets before any fight: a
// fighter's soulshots at its weapon's attacks a minute, a mystic's
// spiritshots at castsPerMinute, the potions at PotionsPerHour and the
// Scrolls of Escape, with the margin on top.
func TestUseMeterFirstEstimate(t *testing.T) {
	tests := []struct {
		name   string
		class  int32
		weapon int32
		atkSpd int
		margin int
		want   map[int32]int32
	}{
		{"fighter at 60 attacks a minute", classHumanFight, ngSword, 500, 0, map[int32]int32{soulshotNoGrade: 3600, lesserHealingPotion: 10, scrollOfEscape: 2}},
		{"no attack speed is the base speed", classHumanFight, ngSword, 0, 0, map[int32]int32{soulshotNoGrade: int32(math.Ceil(60 * 60000.0 / (500000 / 333)))}},
		{"margin", classHumanFight, ngSword, 500, 20, map[int32]int32{soulshotNoGrade: 4320, lesserHealingPotion: 12, scrollOfEscape: 2}},
		{"mystic casts", classHumanMystic, ngWand, 500, 0, map[int32]int32{blessedNoGrade: 900}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newMeterBrain(t, tt.class, wielded(1, tt.weapon))
			b.persona.Supply.Margin = tt.margin
			w := b.cfg.Travel.weapons[tt.weapon]
			w.atkSpd = tt.atkSpd
			b.cfg.Travel.weapons[tt.weapon] = w
			for id, want := range tt.want {
				if got := b.tripTarget(id); got != want {
					t.Errorf("tripTarget(%d) = %d, want %d", id, got, want)
				}
			}
			if got, want := b.shotTarget(w), b.tripTarget(b.buyShot(w)); got != want {
				t.Errorf("shotTarget = %d, want the shots' trip target %d", got, want)
			}
		})
	}
}

// TestUseMeterRate checks that shots spent in combat move the rate and the
// trip target follows it, and that time and spending out of combat do not.
func TestUseMeterRate(t *testing.T) {
	b := newMeterBrain(t, classHumanFight, wielded(1, ngSword), shotStack(2, soulshotNoGrade, 100000))
	first := b.tripTarget(soulshotNoGrade)
	fight(t, b, 2, time.Minute, 31*time.Minute, 30)
	mid := b.tripTarget(soulshotNoGrade)
	if mid >= first || mid <= 1800 {
		t.Fatalf("after 30 min at 30 a minute the target is %d, want under %d and over 1800", mid, first)
	}
	// An hour out of combat, shots sold or deposited meanwhile.
	it := b.world.Inventory.Items[2]
	it.Count -= 5000
	if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{State: ItemModified, Item: it}}}}, at(61*time.Minute)); err != nil {
		t.Fatal(err)
	}
	fight(t, b, 2, 91*time.Minute, 91*time.Minute, 30)
	if got := b.tripTarget(soulshotNoGrade); got != mid {
		t.Fatalf("out of combat the target moved from %d to %d", mid, got)
	}
	fight(t, b, 2, 91*time.Minute, 400*time.Minute, 30)
	if got := b.tripTarget(soulshotNoGrade); got < 1800 || got > 1818 {
		t.Fatalf("after 5 h at 30 a minute the target is %d, want 1800 within 1 %%", got)
	}
	// A removed stack counts whole.
	before := b.meter.used[soulshotNoGrade]
	self := b.world.Self.ObjectID
	n := b.world.Inventory.Items[2].Count
	if err := b.handle([]Event{AutoAttackStart{ObjectID: self}, InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: Item{ObjectID: 2, TemplateID: soulshotNoGrade}}}}}, at(400*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := b.meter.used[soulshotNoGrade] - before; got != float64(n) {
		t.Fatalf("a removed stack of %d counted %v", n, got)
	}
}

// TestUseMeterDecay checks the meter's average against its formula: the
// first estimate weighs meterPrior minutes, both halving every
// meterHalfLife minutes of fighting.
func TestUseMeterDecay(t *testing.T) {
	tests := []struct {
		name  string
		steps []float64 // minutes of fighting, then 30 shots spent, each
		est   float64
	}{
		{"no fight", nil, 60},
		{"one minute", []float64{1}, 60},
		{"two half hours", []float64{30, 30}, 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := useMeter{at: t0, fighting: true}
			now := t0
			var used, minutes, fought float64
			for _, s := range tt.steps {
				now = now.Add(time.Duration(s * float64(time.Minute)))
				m.accrue(now)
				m.spend(soulshotNoGrade, 30)
				f := math.Exp2(-s / meterHalfLife)
				used, minutes, fought = used*f+30, minutes*f+s, fought+s
			}
			prior := meterPrior * math.Exp2(-fought/meterHalfLife)
			want := (used + tt.est*prior) / (minutes + prior)
			if got := m.rate([]int32{soulshotNoGrade}, tt.est); math.Abs(got-want) > 1e-9 {
				t.Fatalf("rate = %v, want %v", got, want)
			}
		})
	}
}

// TestTripTargetWeight checks that the trip targets fill the weight left
// under BuyLoad in priority order, the shots first, the extra supplies
// last, and that no weight limit caps nothing.
func TestTripTargetWeight(t *testing.T) {
	tests := []struct {
		name      string
		load, max int32
		want      []supply
	}{
		{"room for all", 10000, 100000, []supply{{soulshotNoGrade, 3600, false}, {lesserHealingPotion, 10, false}, {scrollOfEscape, 2, false}, {soulOre, 500, true}}},
		{"extra capped", 30000, 100000, []supply{{soulshotNoGrade, 3600, false}, {lesserHealingPotion, 10, false}, {scrollOfEscape, 2, false}, {soulOre, 431, true}}},
		{"shots capped", 40000, 100000, []supply{{soulshotNoGrade, 2250, false}, {lesserHealingPotion, 0, false}, {scrollOfEscape, 0, false}, {soulOre, 0, true}}},
		{"over the cap", 47000, 100000, []supply{{soulshotNoGrade, 500, false}, {lesserHealingPotion, 0, false}, {scrollOfEscape, 0, false}, {soulOre, 0, true}}},
		{"no weight limit yet", 47000, 0, []supply{{soulshotNoGrade, 3600, false}, {lesserHealingPotion, 10, false}, {scrollOfEscape, 2, false}, {soulOre, 500, true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1000 shots carried weigh 4000 of the load.
			b := newMeterBrain(t, classHumanFight, wielded(1, ngSword), shotStack(2, soulshotNoGrade, 1000))
			b.persona.Supply.Extra = func(*World) map[int32]int32 { return map[int32]int32{soulOre: 500} }
			b.world.Self.Load, b.world.Self.MaxLoad = tt.load, tt.max
			if got := b.supplies(); !slices.Equal(got, tt.want) {
				t.Fatalf("supplies = %v, want %v", got, tt.want)
			}
			if got, want := b.shotTarget(b.cfg.Travel.weapons[ngSword]), tt.want[0].count; got != want {
				t.Fatalf("shotTarget = %d, want %d", got, want)
			}
		})
	}
}

// TestSupplyWants checks what the shop errand buys of the supplies: each up
// to its trip target once under Refill percent of it, a Restock row's
// item by the row alone, and the extra supplies after the bot's own.
func TestSupplyWants(t *testing.T) {
	tests := []struct {
		name    string
		items   []Item
		restock []Restock
		want    []buyRow
		target  int32 // tripTarget of the Lesser Healing Potion
	}{
		{"low on everything", []Item{shotStack(2, soulshotNoGrade, 500)}, nil, []buyRow{{Item: soulshotNoGrade, Count: 3100}, {Item: lesserHealingPotion, Count: 10}, {Item: scrollOfEscape, Count: 2}, {Item: soulOre, Count: 500}}, 10},
		{"enough of everything", []Item{shotStack(2, soulshotNoGrade, 900), potionStack(3), {ObjectID: 4, TemplateID: scrollOfEscape, Count: 1}, {ObjectID: 5, TemplateID: soulOre, Count: 200}}, nil, nil, 10},
		{"restock overrides the potion", []Item{shotStack(2, soulshotNoGrade, 900)}, []Restock{{Item: lesserHealingPotion, Min: 1, Max: 5}}, []buyRow{{Item: lesserHealingPotion, Count: 5}, {Item: scrollOfEscape, Count: 2}, {Item: soulOre, Count: 500}}, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newMeterBrain(t, classHumanFight, append([]Item{wielded(1, ngSword)}, tt.items...)...)
			b.persona.Shop.Restock = tt.restock
			b.persona.Supply.Extra = func(*World) map[int32]int32 { return map[int32]int32{soulOre: 500} }
			b.cfg.Travel.spawnedSold[soulOre] = true
			got := slices.DeleteFunc(b.shopWants(true), func(r buyRow) bool {
				return !slices.Contains([]int32{soulshotNoGrade, lesserHealingPotion, scrollOfEscape, soulOre}, r.Item)
			})
			if !slices.Equal(got, tt.want) {
				t.Fatalf("shopWants = %v, want %v", got, tt.want)
			}
			if got := b.tripTarget(lesserHealingPotion); got != tt.target {
				t.Fatalf("tripTarget(potion) = %d, want %d", got, tt.target)
			}
		})
	}
}

// TestKeepsEscapes checks that the bot keeps its Scrolls of Escape, the
// trip home in the keep set, whether or not it carries any for a trip.
func TestKeepsEscapes(t *testing.T) {
	for _, n := range []int32{0, 2} {
		b := newMeterBrain(t, classHumanFight)
		b.persona.Supply.Escapes = n
		if got := b.keeps(scrollOfEscape); !got {
			t.Errorf("Escapes %d: keeps(SoE) = %v", n, got)
		}
	}
}

// TestIndexWeights checks the weights the travel map keeps: every item's
// and etc item's, 0 for one that weighs nothing, and the weapons' attack
// speed.
func TestIndexWeights(t *testing.T) {
	got := indexWeights(map[int32]datapack.Item{
		ngSword: {ID: ngSword, Weight: 1600},
		dSword:  {ID: dSword},
	}, map[int32]datapack.EtcItem{
		soulshotNoGrade: {ID: soulshotNoGrade, Weight: 4},
		scrollOfEscape:  {ID: scrollOfEscape, Weight: 120},
		dSoulshot:       {ID: dSoulshot},
	})
	if want := map[int32]int32{ngSword: 1600, dSword: 0, soulshotNoGrade: 4, scrollOfEscape: 120, dSoulshot: 0}; !maps.Equal(got, want) {
		t.Fatalf("indexWeights = %v, want %v", got, want)
	}
	w := indexWeapons(map[int32]datapack.Item{ngSword: {ID: ngSword, Type: "Weapon", Soulshots: 1, Stats: datapack.Stats{PAtkSpd: 379}}})
	if w[ngSword].atkSpd != 379 {
		t.Fatalf("atkSpd = %d, want 379", w[ngSword].atkSpd)
	}
}
