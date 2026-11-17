package botclient

import "github.com/fatal10110/acis_golang_bot/internal/datapack"

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

// arrowWants is the arrow purchase of the shop errand
// (ShopPolicy.ArrowsMin, ArrowsMax): the arrows the wielded bow shoots,
// up to ArrowsMax, once fewer than ArrowsMin are carried, worn or not.
func (b *Brain) arrowWants() []buyRow {
	p := &b.persona.Shop
	id := b.bowArrows()
	if id == 0 {
		return nil
	}
	if have := b.carried(id); have < p.ArrowsMin && p.ArrowsMax > have {
		return []buyRow{{item: id, count: p.ArrowsMax - have}}
	}
	return nil
}
