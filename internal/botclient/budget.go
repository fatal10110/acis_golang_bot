package botclient

import "slices"

// tier is a step of a shop visit's one budget (docs/items.md section 5):
// the visit spends on the tiers in order, each above its floor.
type tier uint8

const (
	// tierSupplies is one trip's supplies, above the reserve: the wanted
	// spellbooks, the Restock rows, the shots, the HP potion and Scrolls
	// of Escape, the arrows, then the Extra supplies.
	tierSupplies tier = iota
	// tierGear is the gear goal and the other upgrades (gearWants), above
	// the reserve floor.
	tierGear
	// tierComfort tops the supplies up (comfortWants), above the reserve
	// floor. A visit saving for the gear goal skips it.
	tierComfort
)

// want is a purchase the shop errand plans and the budget tier it is spent
// in.
type want struct {
	buyRow
	tier tier
}

// reserve is the adena a shop visit never spends: ShopPolicy.KeepAdena,
// and at least the trip home, one Scroll of Escape at a merchant's base
// price. The clan warehouse fees join it (M9).
func (b *Brain) reserve() int64 {
	r := int64(b.persona.Shop.KeepAdena)
	if m := b.cfg.Travel; m != nil {
		r = max(r, int64(m.cheapest[scrollOfEscape]))
	}
	return r
}

// floor is the adena the purchases of tier t never spend: the reserve, and
// for the gear and comfort tiers at least GearPolicy.KeepAdena, the
// reserve floor that pays the next trips' supplies.
func (b *Brain) floor(t tier) int64 {
	if t == tierSupplies {
		return b.reserve()
	}
	return max(b.reserve(), int64(b.persona.Gear.KeepAdena))
}

// wants are the shop errand's planned purchases in budget order: the
// supplies tier (the wanted spellbooks it lacks, by id, the restocks of
// its class under their minimum, in persona order, then shotWants,
// supplyWants, arrowWants and extraWants), then the gear tier
// (gearWants), then, with comfort, the comfort tier (comfortWants).
func (b *Brain) wants(comfort bool) []want {
	p := &b.persona.Shop
	var out []want
	add := func(t tier, rows []buyRow) {
		for _, r := range rows {
			out = append(out, want{buyRow: r, tier: t})
		}
	}
	if p.Books {
		var books []buyRow
		for id := range b.shop.books {
			if b.carried(id) == 0 {
				books = append(books, buyRow{item: id, count: 1})
			}
		}
		slices.SortFunc(books, func(a, b buyRow) int { return int(a.item - b.item) })
		add(tierSupplies, books)
	}
	for _, r := range p.Restock {
		if _, shot := b.shotTable()[r.Item]; !r.For.fits(b.world.Self.ClassID) || shot && !b.usableShot(r.Item) {
			continue
		}
		if have := b.carried(r.Item); have < r.Min && r.Max > have {
			add(tierSupplies, []buyRow{{item: r.Item, count: r.Max - have}})
		}
	}
	add(tierSupplies, b.shotWants())
	add(tierSupplies, b.supplyWants())
	add(tierSupplies, b.arrowWants())
	add(tierSupplies, b.extraWants())
	add(tierGear, b.gearWants())
	if comfort {
		add(tierComfort, b.comfortWants())
	}
	return out
}

// shopWants are the items the bot buys now and how many: the supplies and
// gear tiers of wants, in order. The comfort tier never makes a visit due
// nor opens a list: it only tops up what a window opened anyway sells
// (buyRows). Unless anyAdena, only what the bot's adena past its tier's
// floor buys one of at a merchant's base price is listed.
func (b *Brain) shopWants(anyAdena bool) []buyRow {
	adena := int64(b.carried(adenaID))
	var rows []buyRow
	for _, w := range b.wants(false) {
		if !anyAdena && b.cfg.Travel != nil {
			price, ok := b.cfg.Travel.cheapest[w.item]
			if !ok || int64(price) > adena-b.floor(w.tier) {
				continue
			}
		}
		rows = append(rows, w.buyRow)
	}
	return rows
}

// buyRows is the purchase from window l, the budget spent tier by tier:
// per wanted item it sells at a price, as many as the window's stock and
// the adena past the tier's floor allow; of the gear, the first piece per
// slot, a bow followed by its arrows (arrowRow) unless already wanted, and
// a piece of the planned set (gearGoal) only with the rest of the set
// still paid at its cheapest base price, wherever it is sold. The
// comfort tier is skipped while the visit saves for the gear goal, or in
// the grade jump's window for the jump goal (jumpGoal, saves), and tops up
// nothing an earlier row bought.
func (b *Brain) buyRows(l BuyList) []buyRow {
	var spent int64
	var rows []buyRow
	var bought [slotCount]bool
	wants := b.wants(true)
	goal, set := b.gearGoal()
	if jump := b.jumpGoal(); len(jump) > 0 {
		goal = jump
	}
	checked, saving := false, false
	for i := 0; i < len(wants); i++ {
		w := wants[i]
		t := w.tier
		if t == tierComfort {
			if !checked {
				checked = true
				left := int64(l.Money) - spent - b.floor(tierComfort)
				saving = b.saves(goal, left, rows)
			}
			if saving || slices.ContainsFunc(rows, func(r buyRow) bool { return r.item == w.item }) {
				continue
			}
		}
		slot := slotNone
		if g, ok := b.gearItem(w.item); ok {
			if slot = slotOf(g.part); bought[slot] {
				continue
			}
			t = max(t, tierGear)
		}
		var rest int64 // the planned set's pieces still to buy after this one
		if slices.Contains(set, w.item) {
			for _, id := range set {
				if id != w.item && !slices.ContainsFunc(rows, func(r buyRow) bool { return r.item == id }) {
					rest += int64(b.cfg.Travel.cheapest[id])
				}
			}
		}
		j := slices.IndexFunc(l.Items, func(it ShopItem) bool { return it.TemplateID == w.item })
		if j < 0 || l.Items[j].Price <= 0 {
			continue
		}
		it := l.Items[j]
		n := int64(w.count)
		if it.Count > 0 {
			n = min(n, int64(it.Count))
		}
		n = min(n, (int64(l.Money)-spent-b.floor(t)-rest)/int64(it.Price))
		if n < 1 {
			continue
		}
		bought[slot] = true
		spent += n * int64(it.Price)
		rows = append(rows, buyRow{item: w.item, count: int32(n)})
		if slot == slotWeapon {
			r, ok := b.arrowRow(b.cfg.Travel.arrows[w.item])
			if ok && !slices.ContainsFunc(wants, func(x want) bool { return x.item == r.item }) {
				wants = slices.Insert(wants, i+1, want{buyRow: r, tier: tierSupplies})
			}
		}
		if len(rows) == shopMaxRows {
			break
		}
	}
	return rows
}

// saves reports whether a visit that bought rows saves for gear goal
// (gearGoal), so it skips the comfort tier: part of the goal is not among
// rows and left, the adena past the reserve floor, has that part's
// cheapest base price within reach (reaches).
func (b *Brain) saves(goal []int32, left int64, rows []buyRow) bool {
	var rest []int32
	for _, id := range goal {
		if !slices.ContainsFunc(rows, func(r buyRow) bool { return r.item == id }) {
			rest = append(rest, id)
		}
	}
	return len(rest) > 0 && b.reaches(b.price(rest), left)
}

// reaches reports whether left adena has a goal costing price within
// reach: it is at least GearPolicy.SaveFrom percent of the price, so the
// bot saves for the goal.
func (b *Brain) reaches(price, left int64) bool {
	from := int64(b.persona.Gear.SaveFrom)
	return from > 0 && left > 0 && left*100 >= price*from
}

// extraWants are the Extra supplies (SupplyPolicy.Extra) the shop errand
// buys after the bot's own (refillRow).
func (b *Brain) extraWants() []buyRow {
	var rows []buyRow
	for _, s := range b.supplies() {
		if r, ok := b.refillRow(s); ok && s.extra {
			rows = append(rows, r)
		}
	}
	return rows
}

// comfortWants are the comfort tier's purchases (ShopPolicy.Comfort): the
// bot's own supplies the use meter sizes (the HP potion, the Scrolls of
// Escape and shots without a ShotStock Max), each from a spawned merchant
// and topped up to its trip target where the supplies tier, buying only
// under Refill percent, left it short; then the build's buff potion
// (buffPotionRow).
func (b *Brain) comfortWants() []buyRow {
	if !b.persona.Shop.Comfort {
		return nil
	}
	fixedShots := b.shotStock().Max != 0
	var rows []buyRow
	for _, s := range b.supplies() {
		if _, shot := b.shotTable()[s.item]; s.extra || shot && fixedShots {
			continue
		}
		if _, ok := b.restockOf(s.item); ok || b.cfg.Travel != nil && !b.cfg.Travel.spawnedSold[s.item] {
			continue
		}
		if have := b.carried(s.item); have < s.count {
			rows = append(rows, buyRow{item: s.item, count: s.count - have})
		}
	}
	if r, ok := b.buffPotionRow(); ok {
		rows = append(rows, r)
	}
	return rows
}

// inKeepSet reports whether template id is in the loadout's keep set: what
// the bot buys beyond its trip targets and never sells. The Scroll of
// Escape is, the trip home, and the build's buff potion
// (GearClass.BuffPotion); the planned dyes (#179) join them.
func (b *Brain) inKeepSet(id int32) bool {
	return id == scrollOfEscape || id != 0 && id == b.gearClass().BuffPotion
}
