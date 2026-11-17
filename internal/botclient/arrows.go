package botclient

import (
	"cmp"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// indexArrows maps every bow template to the arrow template it shoots: the
// arrow of the bow's crystal grade, the only one the server arms it with
// (itemcontainer/inventory.go arrowIDForCrystal, FindArrowForBow). Of
// several arrows of one grade the lowest id is kept; the shipped datapack
// has one per grade, the server's.
func indexArrows(items map[int32]datapack.Item) map[int32]int32 {
	byGrade := make(map[int]int32)
	for id, it := range items {
		if it.Type != "EtcItem" || it.Kind != "ARROW" {
			continue
		}
		if cur, ok := byGrade[it.Grade]; !ok || id < cur {
			byGrade[it.Grade] = id
		}
	}
	arrows := make(map[int32]int32)
	for id, it := range items {
		if it.Type != "Weapon" || it.Kind != "BOW" {
			continue
		}
		if a, ok := byGrade[it.Grade]; ok {
			arrows[id] = a
		}
	}
	return arrows
}

// bowArrows is the arrow template the bow the bot wields shoots; 0 when it
// wields none, or the travel map knows no arrow of its grade. The bot only
// carries them: the server arms the bow with them by itself, when the bow
// is equipped or at the next attack (itemcontainer/inventory.go
// pullOffHandLocked, attack/controller.go CheckAndEquipArrows), and
// answers a UseItem of arrows with ActionFailed
// (network/inventory.go useOffHandItem).
func (b *Brain) bowArrows() int32 {
	if b.cfg.Travel == nil {
		return 0
	}
	for _, it := range b.world.Inventory.Items {
		if !it.Equipped {
			continue
		}
		if id, ok := b.cfg.Travel.arrows[it.TemplateID]; ok {
			return id
		}
	}
	return 0
}

// nextBowArrows is the arrow template of the bow the gear errand wears
// next: the best carried weapon that beats the worn one, in ruleGear's
// order, when it is a bow; 0 otherwise.
func (b *Brain) nextBowArrows() int32 {
	if b.cfg.Travel == nil {
		return 0
	}
	w := b.worn()
	var best Item
	var bestG gearItem
	for _, it := range b.world.Inventory.Items {
		g, ok := b.gearItem(it.TemplateID)
		if !ok || slotOf(g.part) != slotWeapon || !b.upgrade(it, &w) {
			continue
		}
		if best.ObjectID == 0 || cmp.Or(cmp.Compare(bestG.price, g.price), cmp.Compare(it.ObjectID, best.ObjectID)) < 0 {
			best, bestG = it, g
		}
	}
	return b.cfg.Travel.arrows[best.TemplateID]
}

// bowWaits reports whether carried item it is a bow the bot carries no
// arrows of its grade for: the server would refuse its every attack
// (NotEnoughArrows, attack/controller.go CheckAndEquipArrows), so the gear
// errand keeps what the bot wields until the shop errand has bought them
// (arrowWants), with the bow or after it.
func (b *Brain) bowWaits(it Item) bool {
	a, ok := b.cfg.Travel.arrows[it.TemplateID]
	return ok && b.carried(a) == 0
}

// unarmedBow reports whether gear id is a bow the bot carries none of the
// arrows of and the shop errand never buys them for (ShopPolicy.ArrowsMin
// or ArrowsMax 0): bowWaits would keep it off for good, so the gear
// errand does not buy it.
func (b *Brain) unarmedBow(id int32) bool {
	a, ok := b.cfg.Travel.arrows[id]
	if !ok || b.carried(a) != 0 {
		return false
	}
	_, buys := b.arrowRow(a)
	return !buys
}

// arrowWants is the arrow purchase of the shop errand
// (ShopPolicy.ArrowsMin, ArrowsMax): the arrows the wielded bow shoots,
// then the ones of the bow the gear errand wears next (nextBowArrows),
// each up to ArrowsMax once fewer than ArrowsMin are carried, worn or not.
func (b *Brain) arrowWants() []buyRow {
	var rows []buyRow
	worn, next := b.bowArrows(), b.nextBowArrows()
	if r, ok := b.arrowRow(worn); ok {
		rows = append(rows, r)
	}
	if r, ok := b.arrowRow(next); ok && next != worn {
		rows = append(rows, r)
	}
	return rows
}

// arrowRow is the purchase of arrow template id: up to ShopPolicy.ArrowsMax
// once fewer than ArrowsMin are carried; ok false for none or id 0.
func (b *Brain) arrowRow(id int32) (buyRow, bool) {
	p := &b.persona.Shop
	if have := b.carried(id); id != 0 && have < p.ArrowsMin && p.ArrowsMax > have {
		return buyRow{item: id, count: p.ArrowsMax - have}, true
	}
	return buyRow{}, false
}

// armedBow lets the shop errand start its next visit at once when the gear
// errand has just worn bow it and the bot carries too few of the arrows it
// shoots: the server refuses every attack of a bow without them
// (NotEnoughArrows), and the visit that bought the bow, ended or ending,
// would hold the next one off for shopRetry. A visit in progress is armed
// (it ends with the next one due), else the next one is due now. The wear
// happens once per bow, so this adds at most one visit per bow worn.
func (b *Brain) armedBow(it Item) {
	a, ok := b.cfg.Travel.arrows[it.TemplateID]
	if _, short := b.arrowRow(a); !ok || !short {
		return
	}
	if b.shop.v.active() {
		b.shop.armed = true
		return
	}
	b.shop.nextAt = time.Time{}
}
