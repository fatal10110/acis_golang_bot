package botclient

import "math"

// ValuablePolicy is how a merchant persona tells a valuable among its loot
// (docs/valuables/score.md, D31). The zero ValuablePolicy is disabled: it
// finds nothing valuable. Every value is a configurable default (D6).
//
// PriceFloor is the least reference price of a valuable, and ValueMin its
// least score V. CommonSupply is the supply, items per hour, of an item no
// rarer than common, and RMax the most rarity R counts; CraftDivisor divides
// the rarity of a recipe product, and EnchantRare is the rarity each enchant
// level past safe adds.
type ValuablePolicy struct {
	PriceFloor   int64
	ValueMin     int64
	CommonSupply float64
	RMax         float64
	CraftDivisor float64
	EnchantRare  float64
}

// defaultValuablePolicy is the policy at the design's defaults
// (docs/valuables/preset.md); no preset enables it yet.
func defaultValuablePolicy() ValuablePolicy {
	return ValuablePolicy{
		PriceFloor: 10000, ValueMin: 100000, CommonSupply: 100, RMax: 4, CraftDivisor: 2, EnchantRare: 0.25,
	}
}

// valueScore is an item's score and what it is made of: its reference price,
// supply ceiling S, rarity R, worth W and score V = W × 2^R.
type valueScore struct {
	ref        int32
	s, r, w, v float64
	// priced is set for a template with a reference price.
	priced bool
}

// valueScore scores one item of template id at enchant level enchant under
// p (docs/valuables/score.md "Rarity, marks and worth"): R is
// log10(CommonSupply / S) clamped to [0, RMax], 0 for no drop source, an item
// a counted buylist sells or a spawned NPC's non-upgrade multisell makes,
// divided by CraftDivisor for a recipe product, plus EnchantRare per level
// past safe; W is the reference price plus a normal scroll of the item's
// grade and kind per level. An unpriced template scores nothing.
func (m *TravelMap) valueScore(p ValuablePolicy, id int32, enchant int) valueScore {
	ref, ok := m.prices[id]
	if !ok {
		return valueScore{}
	}
	sup := m.supply[id]
	sc := valueScore{ref: ref, s: sup.perHour, priced: true}
	_, shopSells := m.cheapest[id]
	if sup.perHour > 0 && !shopSells && !sup.multisell {
		sc.r = min(max(math.Log10(p.CommonSupply/sup.perHour), 0), p.RMax)
		if sup.craftable && p.CraftDivisor > 0 {
			sc.r /= p.CraftDivisor
		}
	}
	if past := enchant - sup.safe; past > 0 {
		sc.r += p.EnchantRare * float64(past)
	}
	sc.w = float64(ref)
	if sup.scroll != 0 && enchant > 0 {
		sc.w += float64(enchant) * float64(m.prices[sup.scroll])
	}
	sc.v = sc.w * math.Exp2(sc.r)
	return sc
}

// valuable reports whether sc passes p's numeric test: a reference price of
// at least PriceFloor and a score of at least ValueMin. The zero policy
// passes nothing. What else the valuable test excludes (equipped, kept, sold
// by a buylist, ...) is the caller's.
func (p ValuablePolicy) valuable(sc valueScore) bool {
	if p == (ValuablePolicy{}) || !sc.priced {
		return false
	}
	return int64(sc.ref) >= p.PriceFloor && sc.v >= float64(p.ValueMin)
}
