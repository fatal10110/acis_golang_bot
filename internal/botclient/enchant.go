package botclient

import (
	"cmp"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// ChooseInventoryItem asks the client to pick the item to enchant with the
// scroll it used: the scroll's template id, not its object id
// (serverpackets/enchant.go FrameChooseInventoryItem).
type ChooseInventoryItem struct{ ScrollItemID int32 }

// EnchantResult closes the enchant window with the outcome of a
// RequestEnchantItem (serverpackets/enchant.go FrameEnchantResult).
type EnchantResult struct{ Code EnchantCode }

// EnchantCode is an EnchantResult's outcome (serverpackets.EnchantResult).
type EnchantCode int32

// EnchantResult codes.
const (
	EnchantSuccess            EnchantCode = 0
	EnchantBrokenWithCrystals EnchantCode = 1
	EnchantCancelled          EnchantCode = 2
	EnchantUnsuccess          EnchantCode = 3
	EnchantBrokenNoCrystals   EnchantCode = 4
)

func (ChooseInventoryItem) event() {}
func (EnchantResult) event()       {}

// decodeChooseInventoryItem: the scroll's template id as int32.
func decodeChooseInventoryItem(r *wire.Reader) Event {
	return ChooseInventoryItem{ScrollItemID: r.ReadInt32()}
}

// decodeEnchantResult: the result code as int32.
func decodeEnchantResult(r *wire.Reader) Event {
	return EnchantResult{Code: EnchantCode(r.ReadInt32())}
}

// encodeRequestEnchantItem enchants the carried or worn item objectID with
// the scroll the last UseItem selected (clientpackets/item_ops.go
// DecodeRequestEnchantItem: one int32; the server ignores a 0).
func encodeRequestEnchantItem(objectID int32) []byte {
	w := wire.NewPacketWriter(opGameRequestEnchantItem)
	w.WriteInt32(objectID)
	return w.Bytes()
}

// enchantAction enchants it with the selected scroll.
func enchantAction(it Item) Action {
	return Action{kind: actEnchant, payload: encodeRequestEnchantItem(it.ObjectID), item: it.TemplateID}
}

// enchantScroll is an enchant scroll the server takes: a weapon or an armor
// scroll of a crystal grade (datapack.Item.Grade: 1 for D up to 5 for S).
type enchantScroll struct {
	weapon, blessed bool
	grade           int32
}

// enchantScrolls are the scrolls the server enchants with, copied from its
// scrolls table (enchant/service.go:56-87) and owned here (D5): plain,
// blessed and crystal scrolls of each grade, weapon and armor.
var enchantScrolls = map[int32]enchantScroll{
	729: {weapon: true, grade: scrollGradeA}, 947: {weapon: true, grade: scrollGradeB}, 951: {weapon: true, grade: scrollGradeC},
	955: {weapon: true, grade: scrollGradeD}, 959: {weapon: true, grade: scrollGradeS},
	730: {grade: scrollGradeA}, 948: {grade: scrollGradeB}, 952: {grade: scrollGradeC}, 956: {grade: scrollGradeD}, 960: {grade: scrollGradeS},
	6569: {weapon: true, blessed: true, grade: scrollGradeA}, 6571: {weapon: true, blessed: true, grade: scrollGradeB},
	6573: {weapon: true, blessed: true, grade: scrollGradeC}, 6575: {weapon: true, blessed: true, grade: scrollGradeD},
	6577: {weapon: true, blessed: true, grade: scrollGradeS},
	6570: {blessed: true, grade: scrollGradeA}, 6572: {blessed: true, grade: scrollGradeB}, 6574: {blessed: true, grade: scrollGradeC},
	6576: {blessed: true, grade: scrollGradeD}, 6578: {blessed: true, grade: scrollGradeS},
	731: {weapon: true, grade: scrollGradeA}, 949: {weapon: true, grade: scrollGradeB}, 953: {weapon: true, grade: scrollGradeC},
	957: {weapon: true, grade: scrollGradeD}, 961: {weapon: true, grade: scrollGradeS},
	732: {grade: scrollGradeA}, 950: {grade: scrollGradeB}, 954: {grade: scrollGradeC}, 958: {grade: scrollGradeD}, 962: {grade: scrollGradeS},
}

// Crystal grades as datapack.Item.Grade and gearItem.grade hold them.
const (
	scrollGradeD int32 = 1 + iota
	scrollGradeC
	scrollGradeB
	scrollGradeA
	scrollGradeS
)

// The server's default safe enchant levels (enchant/service.go
// DefaultConfig SafeMax, SafeMaxFull): below them a scroll always succeeds
// (scroll.chance), so the last safe use ends at +3, +4 for full-body armor.
// The server's values are its config; the bot plays to the defaults.
const (
	enchantSafeMax     = 3
	enchantSafeMaxFull = 4
)

// enchantReplyTimeout is how long the enchant rule waits for each answer:
// ChooseInventoryItem after the scroll's UseItem, EnchantResult after
// RequestEnchantItem, and the InventoryUpdate that shows the new level
// after a success (the server flushes it from its 333 ms ticker,
// task/inventoryupdates.go, so it may come before EnchantResult or after).
const enchantReplyTimeout = 5 * time.Second

// EnchantPolicy is how the bot enchants the gear it wears with the enchant
// scrolls it carries (rule "enchant"). Every value is a configurable
// default (D6).
type EnchantPolicy struct {
	// Risk lets the bot enchant past the safe level (+3, +4 for full-body
	// armor) with every scroll it carries, where a failure breaks a plain
	// scroll's piece (or sets a blessed scroll's back to +0). Off by
	// default: the bot stops at the safe level and keeps the rest.
	Risk bool
	// RetryAfter is how long the bot enchants nothing after an attempt
	// that could not finish: a request unanswered within
	// enchantReplyTimeout, an EnchantResult other than a success, or a
	// target gone before the request.
	RetryAfter time.Duration
}

func defaultEnchantPolicy() EnchantPolicy {
	return EnchantPolicy{RetryAfter: 5 * time.Minute}
}

// enchantStep is where the enchant rule's exchange stands.
type enchantStep uint8

const (
	enchantIdle enchantStep = iota
	// enchantUsed: the scroll's UseItem went out; ChooseInventoryItem is
	// awaited.
	enchantUsed
	// enchantChosen: the server selected the scroll; RequestEnchantItem
	// goes out next.
	enchantChosen
	// enchantRequested: RequestEnchantItem went out; EnchantResult is
	// awaited.
	enchantRequested
	// enchantSettling: a success came; the InventoryUpdate showing the
	// target past its old level is awaited.
	enchantSettling
)

// enchantState is the enchant rule's exchange. The brain goroutine owns it.
type enchantState struct {
	step enchantStep
	// scroll is the scroll template the exchange uses, target the item it
	// enchants, at level when planned.
	scroll, target int32
	level          uint16
	// waitUntil is when the awaited answer is given up on.
	waitUntil time.Time
	// retryAt is when the next exchange may start after one that could
	// not finish.
	retryAt time.Time
}

// backOff ends the exchange and enchants nothing for dur.
func (e *enchantState) backOff(now time.Time, dur time.Duration) {
	*e = enchantState{retryAt: now.Add(dur)}
}

// sent moves the exchange on with the packet that went out at now: the
// planned scroll's UseItem, then RequestEnchantItem.
func (e *enchantState) sent(a Action, now time.Time) {
	switch {
	case e.step == enchantIdle && e.scroll != 0 && a.kind == actUseItem && a.item == e.scroll:
		e.step, e.waitUntil = enchantUsed, now.Add(enchantReplyTimeout)
	case e.step == enchantChosen && a.kind == actEnchant:
		e.step, e.waitUntil = enchantRequested, now.Add(enchantReplyTimeout)
	}
}

// enchantChosen takes the ChooseInventoryItem the scroll's UseItem asked
// for.
func (b *Brain) enchantChosen(ev ChooseInventoryItem) {
	if e := &b.ench; e.step == enchantUsed && ev.ScrollItemID == e.scroll {
		e.step, e.waitUntil = enchantChosen, time.Time{}
	}
}

// enchantResult takes the EnchantResult of the request: a success waits for
// the inventory to show the new level, anything else ends the exchange for
// RetryAfter.
func (b *Brain) enchantResult(ev EnchantResult, now time.Time) {
	e := &b.ench
	if e.step == enchantChosen && ev.Code == EnchantCancelled {
		// A move, bypass, trade or warehouse of a higher rule cancelled
		// the selection (network/movement.go cancelActiveEnchant): a
		// request now would go unanswered. A cancel while enchantUsed
		// belongs to an older selection.
		e.backOff(now, b.persona.Enchant.RetryAfter)
		return
	}
	if e.step != enchantRequested {
		return
	}
	if ev.Code != EnchantSuccess {
		e.backOff(now, b.persona.Enchant.RetryAfter)
		return
	}
	e.step, e.waitUntil = enchantSettling, now.Add(enchantReplyTimeout)
}

// ruleEnchant enchants the gear the bot wears with the enchant scrolls it
// carries (docs/items.md section 10): in town (inTown), out of combat,
// standing, with no attacker, no private store and no trade open, the
// weapon first, then the armor piece whose next level adds the most score
// (enchantPlan), each only while below the safe level unless Risk is set.
// One exchange at a time: the scroll's UseItem, then ChooseInventoryItem;
// RequestEnchantItem, then EnchantResult; after a success the
// InventoryUpdate showing the new level, so the next plan reads it. Each
// answer is waited for enchantReplyTimeout; a silence or a result other
// than a success ends the exchange for RetryAfter.
// Pacing: UseItem and RequestEnchantItem have no server flood protector
// (network/client_dispatch.go); the rule sends one of them per answer it
// waits for, the frame pacer bounds the frames, and RetryAfter spaces the
// exchanges that could not finish.
func (b *Brain) ruleEnchant(now time.Time) (Action, bool) {
	e := &b.ench
	switch e.step {
	case enchantUsed, enchantRequested:
		if now.Before(e.waitUntil) {
			return Action{}, true
		}
		e.backOff(now, b.persona.Enchant.RetryAfter)
		return Action{}, false
	case enchantChosen:
		// The server holds the scroll selected until a request comes (it
		// refuses a logout meanwhile, network/exitguard.go): send it while
		// the target is the one planned, at the level planned. With the
		// target gone, send it anyway: the server cancels the selection
		// and answers EnchantResult(2) (enchant/service.go EnchantItem,
		// Cancel). With the level changed, send nothing, since the scroll
		// would go past the plan; a later move cancels the selection.
		it, ok := b.world.Inventory.Items[e.target]
		if !ok {
			return enchantAction(Item{ObjectID: e.target}), true
		}
		if it.Enchant != e.level {
			e.backOff(now, b.persona.Enchant.RetryAfter)
			return Action{}, false
		}
		return enchantAction(it), true
	case enchantSettling:
		if it, ok := b.world.Inventory.Items[e.target]; ok && it.Enchant == e.level {
			if now.Before(e.waitUntil) {
				return Action{}, true
			}
			e.backOff(now, b.persona.Enchant.RetryAfter)
			return Action{}, false
		}
		// The inventory shows the new level (or the piece gone): the next
		// plan reads it.
		*e = enchantState{}
	}
	if now.Before(e.retryAt) || !b.enchantFree(now) {
		return Action{}, false
	}
	scroll, target, ok := b.enchantPlan()
	if !ok {
		return Action{}, false
	}
	e.scroll, e.target, e.level = scroll.TemplateID, target.ObjectID, target.Enchant
	return useItemAction(scroll), true
}

// enchantFree reports whether the bot may start an enchant: in town, out
// of combat, standing, with no attacker, no private store, no trade open
// and no gear UseItem awaiting its answer.
func (b *Brain) enchantFree(now time.Time) bool {
	s := &b.world.Self
	return !s.InCombat && !s.Sitting && s.HP > 0 && len(b.attackers(now)) == 0 && !b.store.active() &&
		!b.world.Trade.Open() && !now.Before(b.gear.until) && b.inTown(now)
}

// inTown reports whether the bot stands in a town: within storeTownRadius
// of a town of the travel map (TravelMap.Towns, the gatekeepers' spawns).
func (b *Brain) inTown(now time.Time) bool {
	at := b.world.Self.Position(now)
	t := b.nearestTown(at)
	return t != nil && dist2D(at, *t) <= storeTownRadius
}

// enchantCap is the level the bot enchants worn item it up to, excluded:
// the safe level of its body part, or no cap with Risk.
func (b *Brain) enchantCap(it Item) int {
	switch {
	case b.persona.Enchant.Risk:
		return 1 << 16
	case it.BodySlot == partFullArmor:
		return enchantSafeMaxFull
	}
	return enchantSafeMax
}

// enchantable reports whether the server enchants template id at all
// (enchant/service.go Enchantable): hero items and the weapons 7822-7831
// are left out. Shadow items and fishing rods the gear index leaves out
// already.
func enchantable(id int32) bool {
	return (id < 6611 || id > 6621) && id != 6842 && (id < 7822 || id > 7831)
}

// enchantPlan is the next enchant: a scroll the bot carries and the worn
// piece it goes on, of the scroll's grade and kind (scroll.valid), below
// its cap (enchantCap). The weapon goes first, then the armor piece
// (shield and jewelry included) whose next level adds the most score
// (StatWeights.score), the body piece first on a tie. Plain and crystal
// scrolls go before blessed ones, each by template id.
func (b *Brain) enchantPlan() (scroll, target Item, ok bool) {
	wt := b.gearWeights()
	type cand struct {
		it     Item
		weapon bool
		grade  int32
		gain   float64
		slot   gearSlot
	}
	var targets []cand
	for _, it := range b.world.Inventory.Items {
		if !it.Equipped || !enchantable(it.TemplateID) || int(it.Enchant) >= b.enchantCap(it) {
			continue
		}
		g, known := b.gearItem(it.TemplateID)
		if !known || g.grade <= 0 {
			continue
		}
		next := it
		next.Enchant++
		s := slotOf(it.BodySlot)
		targets = append(targets, cand{it: it, weapon: s == slotWeapon, grade: g.grade, gain: g.score(wt, next) - g.score(wt, it), slot: s})
	}
	slices.SortFunc(targets, func(x, y cand) int {
		return cmp.Or(-cmpBool(x.weapon, y.weapon), -cmp.Compare(x.gain, y.gain), cmp.Compare(x.slot, y.slot), cmp.Compare(x.it.ObjectID, y.it.ObjectID))
	})
	var scrolls []Item
	for _, it := range b.world.Inventory.Items {
		if _, ok := enchantScrolls[it.TemplateID]; ok && it.Count > 0 {
			scrolls = append(scrolls, it)
		}
	}
	slices.SortFunc(scrolls, func(x, y Item) int {
		return cmp.Or(cmpBool(enchantScrolls[x.TemplateID].blessed, enchantScrolls[y.TemplateID].blessed),
			cmp.Compare(x.TemplateID, y.TemplateID), cmp.Compare(x.ObjectID, y.ObjectID))
	})
	for _, t := range targets {
		for _, sc := range scrolls {
			if s := enchantScrolls[sc.TemplateID]; s.weapon == t.weapon && s.grade == t.grade {
				return sc, t.it, true
			}
		}
	}
	return Item{}, Item{}, false
}

// keepsScroll reports whether template id is an enchant scroll of a grade
// the bot wears: a weapon scroll for its worn weapon's grade, an armor
// scroll for a worn armor piece's. Those are not junk.
func (b *Brain) keepsScroll(id int32) bool {
	s, ok := enchantScrolls[id]
	if !ok {
		return false
	}
	for _, it := range b.world.Inventory.Items {
		if !it.Equipped {
			continue
		}
		if g, known := b.gearItem(it.TemplateID); known && g.grade == s.grade && (slotOf(it.BodySlot) == slotWeapon) == s.weapon {
			return true
		}
	}
	return false
}
