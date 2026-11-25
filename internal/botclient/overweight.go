package botclient

import (
	"slices"
	"time"
)

// immobileBand is the server's weight band from 100 % of the weight
// limit: the character's speed is 0, so it cannot walk to a merchant or a
// warehouse (refreshWeightPenalty, character_weight_penalty.go).
const immobileBand = 4

// ruleOverweight is the overweight recovery (#488), an operational limit
// rather than a persona default: from the immobile band the bot drops
// what it carries where it stands, one stack per decision, until its load
// is under the half-speed band again. A town trip cannot shed the load of
// a bot that cannot move, and a Scroll of Escape would leave it as stuck
// in town. It drops its Loot.Junk first, lowest object id first, then the
// heaviest stack it would sell (shedItem); worn, kept, quest, augmented,
// enchanted and valuable items are never dropped. It waits out a move the
// bot believes is under way, since the drop names the bot's position, and
// leaves the decision to the next rules when nothing is left to drop.
//
// It sends RequestDropItem: gateDrop spaces the drops at the server's
// drop gate (config_gates.go DropItemDelay), and dropRetry spaces a
// repeated drop of one stack. The server's drop never checks weight
// (network/inventory.go dropLiveItem), so an immobile bot may drop.
func (b *Brain) ruleOverweight(now time.Time) (Action, bool) {
	s := &b.world.Self
	switch band := weightBand(s.Load, s.MaxLoad); {
	case band >= immobileBand:
		b.shedding = true
	case band < slowBand:
		b.shedding = false
	}
	if !b.shedding {
		return Action{}, false
	}
	it, ok := b.shedItem(now)
	if !ok {
		return Action{}, false
	}
	if s.Moving(now) {
		return Action{}, true
	}
	return dropAction(it, s.Position(now)), true
}

// shedItem is the stack ruleOverweight drops next: the Loot.Junk stack
// with the lowest object id, else the heaviest stack of a known weight
// (TravelMap.weights) the bot would sell rather than keep; never a worn,
// quest, augmented, enchanted or valuable item, adena, nor a stack it asked
// to drop within dropRetry. With no item weights known it drops only junk.
func (b *Brain) shedItem(now time.Time) (Item, bool) {
	var junk, heavy Item
	var heaviest int64
	t := b.cfg.Travel
	for _, it := range b.world.Inventory.Items {
		if it.Equipped || it.TemplateID == adenaID || it.SubCategory == subCategoryQuest || it.Augmentation != 0 {
			continue
		}
		if at, ok := b.loot.dropped[it.ObjectID]; ok && now.Sub(at) < dropRetry {
			continue
		}
		if slices.Contains(b.persona.Loot.Junk, it.TemplateID) {
			if junk.ObjectID == 0 || it.ObjectID < junk.ObjectID {
				junk = it
			}
			continue
		}
		if t == nil || it.Enchant > 0 || b.keepsItem(it) || b.wearsNext(it) || b.valuable(it) {
			continue
		}
		w := int64(t.weights[it.TemplateID]) * int64(max(it.Count, 1))
		if w > heaviest || (w == heaviest && w > 0 && it.ObjectID < heavy.ObjectID) {
			heavy, heaviest = it, w
		}
	}
	if junk.ObjectID != 0 {
		return junk, true
	}
	return heavy, heavy.ObjectID != 0
}
