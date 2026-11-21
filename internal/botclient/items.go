package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// potionReuse is how long the bot drinks no potion after it drank one. The
// default persona's potions (items 1060, 1061 and 1539 in the server
// datapack) each have a 10 s reuse_delay, in separate shared reuse groups,
// so the server would accept one of each in a row; but their skills (2031,
// 2032, 2037) are all a HealOverTime of stack type hp_recover, 7 ticks of
// 2 s, and each replaces the one before (aCis_datapack skills/2000-2099.xml).
// One cooldown across every persona potion, as long as the heal, keeps the
// bot from wasting the potions the heal already covers. The server also
// silently drops a UseItem while the item's reuse cools (live.ItemDisabled,
// network/inventory.go).
const potionReuse = 14 * time.Second

// shotRetry is how long a RequestAutoSoulShot waits for its ExAutoSoulShot
// before the bot asks again: the server ignores the request, answering
// nothing, while the character is dead, in a store or holding a request
// (handleAutoSoulShot, network/inventory.go).
const shotRetry = 30 * time.Second

// Healing potions and shots by template id (aCis_datapack/data/xml/items).
// The wire carries no item class that tells them apart from other etc
// items, so the persona lists them by id. These are the lists of a bot
// without a datapack; with one, TravelMap.WithItemIndex puts the datapack's
// in their place.
var (
	// healingPotions are Greater Healing Potion, Healing Potion and Lesser
	// Healing Potion, best first.
	healingPotions = []int32{1539, 1061, 1060}
	// shotTemplates are the soulshots, spiritshots and blessed spiritshots
	// of every grade, the beginners' ones included.
	shotTemplates = []int32{
		5789, 1835, 1463, 1464, 1465, 1466, 1467, // soulshots
		5790, 2509, 2510, 2511, 2512, 2513, 2514, // spiritshots
		3947, 3948, 3949, 3950, 3951, 3952, // blessed spiritshots
	}
)

// Item is one item the bot carries, as ItemList and InventoryUpdate report
// it.
type Item struct {
	ObjectID, TemplateID int32
	Count                int32
	// Category and SubCategory are the client's item classes
	// (item.Template.Category on the server): weapon or jewelry, armor,
	// money or etc item; and weapon, armor, accessory, quest, money or
	// other.
	Category, SubCategory    uint16
	CustomType1, CustomType2 uint16
	// Equipped is set for an item on the paperdoll, in BodySlot.
	Equipped bool
	BodySlot int32
	Enchant  uint16
	// Augmentation is the augment's attributes, 0 for none.
	Augmentation int32
	// ManaLeft is the shadow item's mana in minutes as the client shows it.
	ManaLeft int32
}

// ItemList is the whole inventory, sent at world entry and whenever the
// server replaces the client's item state wholesale.
type ItemList struct {
	// ShowWindow asks the client to open the inventory window.
	ShowWindow bool
	Items      []Item
}

// ItemState is what an InventoryUpdate entry did to its item
// (itemcontainer.UpdateState on the server).
type ItemState uint16

// InventoryUpdate states; 0 is the client's "unchanged" and is never sent.
const (
	ItemAdded    ItemState = 1
	ItemModified ItemState = 2
	ItemRemoved  ItemState = 3
)

// ItemChange is one entry of an InventoryUpdate: the item as it is after
// the change.
type ItemChange struct {
	State ItemState
	Item  Item
}

// InventoryUpdate reports items added, changed or removed since the last
// report: after a use, a pickup, a purchase, an equip.
type InventoryUpdate struct{ Changes []ItemChange }

// ExAutoSoulShot acknowledges an auto-use toggle of the shot template
// TemplateID, and reports a disable when its stock runs out.
type ExAutoSoulShot struct {
	TemplateID int32
	Enabled    bool
}

func (ItemList) event()        {}
func (InventoryUpdate) event() {}
func (ExAutoSoulShot) event()  {}

// readItem reads an item row: category, object id, template id, count,
// subcategory, custom type 1, equipped, body slot, enchant, custom type 2,
// augmentation, mana (writeItemList and writeInventoryUpdate).
func readItem(r *wire.Reader) Item {
	return Item{
		Category: r.ReadUint16(), ObjectID: r.ReadInt32(), TemplateID: r.ReadInt32(),
		Count: r.ReadInt32(), SubCategory: r.ReadUint16(), CustomType1: r.ReadUint16(),
		Equipped: r.ReadUint16() != 0, BodySlot: r.ReadInt32(), Enchant: r.ReadUint16(),
		CustomType2: r.ReadUint16(), Augmentation: r.ReadInt32(), ManaLeft: r.ReadInt32(),
	}
}

// decodeItemList: show-window flag and item count as uint16, then the
// rows. The count comes off the wire, so the loop ends at the first short
// read.
func decodeItemList(r *wire.Reader) Event {
	e := ItemList{ShowWindow: r.ReadUint16() != 0}
	for n := r.ReadUint16(); n > 0 && r.Err() == nil; n-- {
		e.Items = append(e.Items, readItem(r))
	}
	return e
}

// decodeInventoryUpdate: entry count as uint16, then per entry its state
// as uint16 and the item row. The loop ends at the first short read.
func decodeInventoryUpdate(r *wire.Reader) Event {
	var e InventoryUpdate
	for n := r.ReadUint16(); n > 0 && r.Err() == nil; n-- {
		e.Changes = append(e.Changes, ItemChange{State: ItemState(r.ReadUint16()), Item: readItem(r)})
	}
	return e
}

// decodeExAutoSoulShot: template id, enabled as int32.
func decodeExAutoSoulShot(r *wire.Reader) Event {
	return ExAutoSoulShot{TemplateID: r.ReadInt32(), Enabled: r.ReadInt32() != 0}
}

// encodeUseItem uses the carried item objectID with ctrl released.
func encodeUseItem(objectID int32) []byte {
	w := wire.NewPacketWriter(opGameUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(0) // ctrl not held
	return w.Bytes()
}

// encodeRequestAutoSoulShot turns auto use of the shot template
// templateID on or off. The server needs the template in the inventory.
func encodeRequestAutoSoulShot(templateID int32, enable bool) []byte {
	w := wire.NewPacketWriter(opGameClientExtended)
	w.WriteUint16(opGameRequestAutoSoulShot)
	w.WriteInt32(templateID)
	var v int32
	if enable {
		v = 1
	}
	w.WriteInt32(v)
	return w.Bytes()
}

// useItemAction uses it.
func useItemAction(it Item) Action {
	return Action{kind: actUseItem, payload: encodeUseItem(it.ObjectID), item: it.TemplateID}
}

// autoShotAction turns auto use of the shot template templateID on.
func autoShotAction(templateID int32) Action {
	return Action{kind: actAutoShot, payload: encodeRequestAutoSoulShot(templateID, true), item: templateID}
}

// Inventory is what the bot carries. Its weight is the character's:
// SelfState.Load and MaxLoad, which UserInfo and StatusUpdate report; the
// wire carries no per-item weight.
type Inventory struct {
	// Items are the carried items by object id, equipped ones included.
	Items map[int32]Item
	// AutoShots are the shot templates the server reported an auto-use
	// state for (ExAutoSoulShot), with that state. A template leaves it with
	// its last stack, and a disabled one when its stock grows.
	AutoShots map[int32]bool
}

// ByTemplate returns the carried stack of templateID with the lowest
// object id, and false when none is carried.
func (inv *Inventory) ByTemplate(templateID int32) (Item, bool) {
	var found Item
	for _, it := range inv.Items {
		if it.TemplateID == templateID && (found.ObjectID == 0 || it.ObjectID < found.ObjectID) {
			found = it
		}
	}
	return found, found.ObjectID != 0
}

// applyItems updates the inventory with an item event.
func (w *World) applyItems(ev Event) {
	inv := &w.Inventory
	switch e := ev.(type) {
	case ItemList:
		inv.Items = make(map[int32]Item, len(e.Items))
		for _, it := range e.Items {
			inv.Items[it.ObjectID] = it
		}
		for id := range inv.AutoShots {
			inv.forgetShot(id)
		}
	case InventoryUpdate:
		if inv.Items == nil {
			inv.Items = make(map[int32]Item, len(e.Changes))
		}
		for _, c := range e.Changes {
			switch c.State {
			case ItemAdded, ItemModified:
				prev := inv.Items[c.Item.ObjectID]
				inv.Items[c.Item.ObjectID] = c.Item
				inv.restockShot(c.Item.TemplateID, c.Item.Count-prev.Count)
			case ItemRemoved:
				delete(inv.Items, c.Item.ObjectID)
				inv.forgetShot(c.Item.TemplateID)
			}
		}
	case ExAutoSoulShot:
		if inv.AutoShots == nil {
			inv.AutoShots = make(map[int32]bool)
		}
		inv.AutoShots[e.TemplateID] = e.Enabled
	}
}

// forgetShot drops the auto-use state of templateID once no stack of it is
// carried, so a new stack is turned on again.
func (inv *Inventory) forgetShot(templateID int32) {
	if _, ok := inv.ByTemplate(templateID); !ok {
		delete(inv.AutoShots, templateID)
	}
}

// restockShot drops a disabled auto-use state of templateID when its
// carried count grew by added. The server disables auto use while a short
// stack is still carried (ShotNotEnoughItems, network/item_shot_use.go) and
// shots stack, so a restock arrives as an ItemModified of that same stack:
// the template is turned on again.
func (inv *Inventory) restockShot(templateID, added int32) {
	if on, known := inv.AutoShots[templateID]; known && !on && added > 0 {
		delete(inv.AutoShots, templateID)
	}
}

// drinkPotion is the persona's first potion carried, once potionReuse has
// run out at now since the bot drank any of them; false when none is.
func (b *Brain) drinkPotion(now time.Time) (Action, bool) {
	for _, id := range b.persona.Potions {
		if at, used := b.usedAt[id]; used && now.Sub(at) < potionReuse {
			return Action{}, false
		}
	}
	for _, id := range b.persona.Potions {
		if it, ok := b.world.Inventory.ByTemplate(id); ok && it.Count > 0 {
			return useItemAction(it), true
		}
	}
	return Action{}, false
}

// ruleShots turns auto use on for each of the persona's shot templates the
// bot carries and the server reported no state for. A template the server
// disabled (its stock ran out) stays off until a new stack arrives; a
// request left unanswered is repeated after shotRetry.
func (b *Brain) ruleShots(now time.Time) (Action, bool) {
	inv := &b.world.Inventory
	for _, id := range b.persona.Shots {
		if _, known := inv.AutoShots[id]; known {
			continue
		}
		if _, ok := inv.ByTemplate(id); !ok {
			continue
		}
		if at, asked := b.shotAskedAt[id]; asked && now.Sub(at) < shotRetry {
			continue
		}
		return autoShotAction(id), true
	}
	return Action{}, false
}
