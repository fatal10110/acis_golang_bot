package botclient

import (
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Shop visit timing and limits.
const (
	// shopReplyTimeout is how long a merchant link, a buy or a sell waits
	// for its answer. The server answers nothing to most refusals (an
	// unreachable or untargeted merchant, an item off the list, a stock
	// overrun; every sale refusal), and a purchase it refuses for weight,
	// slots or adena answers a system message and no item list
	// (network/merchant.go requestBuyItem, network/sell.go requestSellItem).
	shopReplyTimeout = 5 * time.Second
	// shopRetry is how long after a shop visit ends, or after no merchant
	// was found for one, the bot starts no other: a refused or short
	// purchase is tried again only then.
	shopRetry = 5 * time.Minute
	// shopMaxRows bounds the rows of one buy or sell request: the server
	// drops a request of more than MaxItemInPacket rows, the larger base
	// inventory size (80 slots unless configured).
	shopMaxRows = 80
)

// mysticClasses are the mystic class ids, every profession of the four
// mystic base classes (aCis_datapack/data/xml/classes/*Mystic.xml).
var mysticClasses = []int32{
	10, 11, 12, 13, 14, 15, 16, 17, 94, 95, 96, 97, 98, // human
	25, 26, 27, 28, 29, 30, 103, 104, 105, // elven
	38, 39, 40, 41, 42, 43, 110, 111, 112, // dark elven
	49, 50, 51, 52, 115, 116, // orc
}

// Items the default persona restocks (aCis_datapack/data/xml/items and
// buyLists.xml, the grocers' lists).
const (
	lesserHealingPotion int32 = 1060
	soulshotNoGrade     int32 = 1835
	spiritshotNoGrade   int32 = 2509
)

// ClassKind narrows a restock to mystic or fighter classes.
type ClassKind uint8

// Class kinds of a restock.
const (
	// AnyClass restocks for every class.
	AnyClass ClassKind = iota
	// MysticClasses restocks for the mystic classes only.
	MysticClasses
	// FighterClasses restocks for every class but the mystic ones.
	FighterClasses
)

// fits reports whether class id classID is of kind k.
func (k ClassKind) fits(classID int32) bool {
	switch k {
	case MysticClasses:
		return slices.Contains(mysticClasses, classID)
	case FighterClasses:
		return !slices.Contains(mysticClasses, classID)
	}
	return true
}

// ShotStock is a shot restock: under Min carried, the bot buys up to Max.
type ShotStock struct {
	Min, Max int32
}

// Restock is an item the bot buys at a merchant: under Min carried, it
// buys up to Max, as far as its adena goes. Item must be stackable: the
// server refuses a purchase of a non-stackable item by more than one.
type Restock struct {
	Item     int32
	Min, Max int32
	For      ClassKind
}

// ShopPolicy is what the bot buys and sells at merchants (rule 9 errand
// "shop"). Every value is a configurable default (D6).
type ShopPolicy struct {
	// Restock are the items the bot keeps in stock, first listed bought
	// first. A shot no weapon the bot wields or wears next charges is not
	// bought (usableShot). A row is the explicit override of its item's
	// trip target (Persona.Supply): the bot buys it by the row instead.
	Restock []Restock
	// Soulshots and Spiritshots restock the shots of the bot's class's kind
	// that the weapon it wields and the one the gear errand wears next
	// charge (shotWants), after Restock: under Min of them carried, the bot
	// buys up to Max of the one a merchant sells (buyShot). Min is also how
	// many carried shots alone back a weapon's grade (shotsBacked). A zero
	// Max, the default, sizes them by the use meter instead
	// (Persona.Supply).
	Soulshots, Spiritshots ShotStock
	// ArrowsMin and ArrowsMax restock the arrows the bow the bot wields
	// shoots, the ones of its grade (bowArrows), after Restock: under
	// ArrowsMin carried, worn or not, the bot buys up to ArrowsMax. So
	// they do for the bow the gear errand wears next (nextBowArrows), and
	// for a bow a visit buys, in the same purchase: the gear errand wears
	// no bow without its arrows. They are never junk; a zero ArrowsMax
	// buys none.
	ArrowsMin, ArrowsMax int32
	// ReagentCastsMin and ReagentCastsMax restock the reagents of the kit's
	// skills (reagentWants), after the potions, shots and arrows, counted
	// in casts per skill at the level the bot knows it and summed per item:
	// under the ReagentCastsMin sum carried, the bot buys up to the
	// ReagentCastsMax sum at a spawned merchant (docs/strategy.md, "Skill
	// reagents"). They are never junk; a zero ReagentCastsMax buys none.
	ReagentCastsMin, ReagentCastsMax int32
	// Books buys the spellbooks a trainer asked for and the bot lacks
	// (AcquireSkillInfo requirements), ahead of Restock.
	Books bool
	// SellAt is how many junk stacks make a visit to sell them due; a
	// visit to buy sells any junk too. 0 never sells. Junk is what the bot
	// carries unequipped, past Keep, adena, quest items, augmented items,
	// Persona.Potions, Persona.Shots, Restock items, its supplies and the
	// keep set (keeps), the wielded bow's arrows, its skills' reagents, the shots it keeps
	// (keepsShot), wanted books and what the keep hook keeps
	// (Persona.Holds); only what the merchant's sell window lists is sold.
	SellAt int
	// Keep are item templates the bot never sells.
	Keep []int32
	// KeepAdena is the adena a purchase never spends, the least of the
	// budget's reserve (docs/items.md section 5), which also keeps the
	// price of one Scroll of Escape, the trip home.
	KeepAdena int32
	// Comfort lets the budget's comfort tier top the trip's supplies up to
	// their trip targets in a window a visit opened anyway (comfortWants),
	// unless the visit saves for the gear goal (GearPolicy.SaveFrom).
	Comfort bool
	// Reach is how far the bot walks to a merchant's spawn (from the travel
	// map, BrainConfig.Travel); a merchant in sight is visited from
	// anywhere.
	Reach int32
}

// defaultShopPolicy restocks the arrows of a wielded bow (weight 6 each at
// most), buys spellbooks and sells junk at a merchant nearby (docs/plan.md,
// town persona); the shots, HP potions and Scrolls of Escape it buys are
// sized by the use meter (defaultSupplyPolicy).
func defaultShopPolicy() ShopPolicy {
	return ShopPolicy{
		ArrowsMin: 200,
		ArrowsMax: 1000,
		// docs/strategy.md, "Skill reagents" (#627).
		ReagentCastsMin: 20,
		ReagentCastsMax: 100,
		Books:           true,
		SellAt:          5,
		KeepAdena:       1000,
		Comfort:         true,
		Reach:           8000,
	}
}

// shopList is a buylist as the travel map keeps it.
type shopList struct {
	id       int32
	products []datapack.Product
}

// sells reports whether the list sells item, and its base price.
func (l shopList) sells(item int32) (int32, bool) {
	for _, p := range l.products {
		if p.ItemID == item {
			return p.Price, true
		}
	}
	return 0, false
}

// indexShops indexes buylists for the travel map: by their NPC template,
// list ids ascending, and the cheapest base price of each item, leaving
// out the game masters' lists no NPC sells.
func indexShops(lists []datapack.BuyList) (map[int32][]shopList, map[int32]int32) {
	out, cheapest := make(map[int32][]shopList), make(map[int32]int32)
	for _, l := range lists {
		if l.NPCID <= 0 {
			continue
		}
		out[l.NPCID] = append(out[l.NPCID], shopList{id: l.ID, products: l.Products})
		for _, p := range l.Products {
			if c, ok := cheapest[p.ItemID]; !ok || p.Price < c {
				cheapest[p.ItemID] = p.Price
			}
		}
	}
	for _, ls := range out {
		slices.SortFunc(ls, func(a, b shopList) int { return int(a.id - b.id) })
	}
	return out, cheapest
}

// shopStep is the answer a shop visit waits for.
type shopStep uint8

const (
	// shopIdle waits for nothing: the next step goes out from the
	// merchant's page.
	shopIdle shopStep = iota
	shopAskSell
	shopSelling
	shopAskBuy
	shopBuying
)

// shopVisit is the bot's errand at a merchant (rule 9 errand "shop"). It
// is owned by the brain goroutine.
type shopVisit struct {
	v    npcVisit
	step shopStep
	// next is the step the action the rule returned starts once sent.
	next shopStep
	// talked is set from a talk to the merchant until its page: only that
	// page has the merchant's links. One after a sale or a purchase (its
	// -sold or -bought page) may not.
	talked bool
	// sell is set while the visit is to sell junk; lists are the merchant's
	// buylists still to open, first first.
	sell  bool
	lists []int32
	// sellList and buyList are the window the server opened and the bot
	// has not answered yet.
	sellList *SellList
	buyList  *BuyList
	// asked counts the windows the visit asked for.
	asked int
	// nextAt is when the bot may start the next visit.
	nextAt time.Time
	// armed is set when the gear errand wore a bow during the visit and
	// the bot carries too few of its arrows (armedBow): the visit ends
	// with the next one due at once instead of after shopRetry.
	armed bool
	// rounds counts the visits ended and the searches that found no
	// merchant (startShop): the town preparation gate opens on one.
	rounds int
	// shunned are the merchants, by template id, not at their spawn or
	// whose page had none of the links the bot needed, until a time.
	shunned holds[int32]
	// unsellable are the item templates a sell window left out.
	unsellable map[int32]bool
	// books are the spellbooks a trainer asked for and the bot lacks.
	books map[int32]bool
	// held are the bought spellbooks the bot carries until a learn uses
	// them; they are never junk.
	held map[int32]bool
}

// end ends the visit; shun leaves its merchant alone for visitShunFor. No
// visit starts before shopRetry, unless the visit is armed.
func (sh *shopVisit) end(now time.Time, shun bool) {
	if shun {
		sh.shunned.set(sh.v.npcID, now.Add(visitShunFor))
	}
	next := now.Add(shopRetry)
	if sh.armed {
		next = now
	}
	*sh = shopVisit{
		nextAt: next, shunned: sh.shunned, unsellable: sh.unsellable, books: sh.books, held: sh.held,
		rounds: sh.rounds + 1,
	}
}

// dropList leaves the first buylist of the visit out of it.
func (sh *shopVisit) dropList() {
	if len(sh.lists) > 0 {
		sh.lists = sh.lists[1:]
	}
}

// wantBook records spellbook item as wanted.
func (sh *shopVisit) wantBook(item int32) {
	if sh.books == nil {
		sh.books = make(map[int32]bool)
	}
	sh.books[item] = true
}

// sent records a packet of the visit that went out at now.
func (sh *shopVisit) sent(a Action, now time.Time) {
	v := &sh.v
	if !v.active() || v.obj == 0 || a.object != v.obj {
		return
	}
	v.sent(a, now)
	switch a.kind {
	case actTalk:
		sh.talked = true
	case actBypass:
		sh.step, sh.talked = sh.next, false
		sh.asked++
		v.waitUntil = now.Add(shopReplyTimeout)
	case actSellItem:
		sh.step, sh.sellList = shopSelling, nil
		v.opened, v.waitUntil = false, now.Add(shopReplyTimeout)
	case actBuyItem:
		sh.step, sh.buyList = shopBuying, nil
		v.opened, v.waitUntil = false, now.Add(shopReplyTimeout)
	}
}

// sellCommand and buyCommand are the merchant page's sell link and its
// link to buylist listID (bypass.go bypassNpc: npc_<object id>_<command>;
// model/actor/npc/folk_bypass.go merchantCommand).
func sellCommand(obj int32) string { return npcBypass(obj, "Sell") }

func buyCommand(obj, listID int32) string {
	return npcBypass(obj, "Buy "+strconv.Itoa(int(listID)))
}

// ruleShop is a rule 9 errand: with junk to sell or items to restock, the
// bot walks to a merchant that sells them, talks to it, sells its junk
// through the page's sell link and buys through its buylist links, one
// request per window the server opened. Links pass the bypass gate
// (gateBypass); buy and sell requests have no reuse gate on the server and
// go out once per BuyList or SellList; legs and clicks pass the frame
// pacer, a visit spending at most visitClicks clicks; visits are shopRetry
// apart, but for one that leaves wants another merchant sells (endShop).
func (b *Brain) ruleShop(now time.Time) (Action, bool) {
	sh := &b.shop
	if b.world.Self.InCombat || (!sh.v.active() && !b.startShop(now)) {
		return Action{}, false
	}
	v := &sh.v
	if sh.step != shopIdle {
		if now.Before(v.waitUntil) {
			return Action{}, true
		}
		// Unanswered: the server refused in silence or for weight, slots
		// or adena. That part of the visit is not tried again.
		switch sh.step {
		case shopAskSell, shopSelling:
			sh.sell = false
		case shopAskBuy, shopBuying:
			sh.dropList()
		}
		sh.step, v.opened, v.waitUntil = shopIdle, false, time.Time{}
	}
	if l := sh.sellList; l != nil {
		if rows := b.sellRows(*l); len(rows) > 0 {
			return Action{kind: actSellItem, payload: botwire.EncodeRequestSellItem(l.ListID, rows), object: v.obj}, true
		}
		sh.sellList, sh.sell = nil, false
	}
	if l := sh.buyList; l != nil {
		if rows := b.buyRows(*l); len(rows) > 0 {
			return Action{kind: actBuyItem, payload: botwire.EncodeRequestBuyItem(l.ListID, rows), object: v.obj}, true
		}
		sh.buyList = nil
		sh.dropList()
	}
	if !sh.sell && len(sh.lists) == 0 {
		b.endShop(now, false)
		return Action{}, false
	}
	if v.opened {
		if sh.sell {
			if cmd := sellCommand(v.obj); b.world.Dialog.Has(cmd) {
				sh.next = shopAskSell
				return bypassTo(v.obj, cmd), true
			}
			// This merchant buys nothing.
			sh.sell = false
		}
		for len(sh.lists) > 0 {
			// A list whose wants an earlier purchase met, or that the page
			// does not link, is left out.
			if cmd := buyCommand(v.obj, sh.lists[0]); b.listWanted(v.npcID, sh.lists[0]) && b.world.Dialog.Has(cmd) {
				sh.next = shopAskBuy
				return bypassTo(v.obj, cmd), true
			}
			sh.dropList()
		}
		// A merchant whose page had none of the links the visit needed
		// is left alone.
		b.endShop(now, sh.asked == 0)
		return Action{}, false
	}
	a, ok := b.approach(v, now)
	if !ok {
		// Not there, the clicks ran out or no route there holds: that
		// merchant is left alone.
		sh.end(now, true)
		return Action{}, false
	}
	return a, true
}

// endShop ends the visit (shopVisit.end). A visit that leaves a want the
// bot can afford that its merchant does not sell and another merchant does
// (wantsElsewhere: gear bought at the armorer, potions still due at the
// grocer) is followed at once by a visit to another merchant, and the
// merchant just visited is left alone for shopRetry, so a chain of such
// visits passes each merchant once (#441).
func (b *Brain) endShop(now time.Time, shun bool) {
	sh := &b.shop
	npcID := sh.v.npcID
	sh.end(now, shun)
	if shun || !b.wantsElsewhere(npcID) {
		return
	}
	sh.shunned.set(npcID, now.Add(shopRetry))
	sh.nextAt = now
}

// wantsElsewhere reports whether the bot wants an item now, at a price its
// adena pays (shopWants), that no list of merchant npcID sells and another
// merchant's does.
func (b *Brain) wantsElsewhere(npcID int32) bool {
	shops := b.cfg.Travel.shops
	sold := func(id, item int32) bool {
		return slices.ContainsFunc(shops[id], func(l shopList) bool { _, ok := l.sells(item); return ok })
	}
	for _, r := range b.shopWants(false) {
		if sold(npcID, r.Item) {
			continue
		}
		for id := range shops {
			if id != npcID && sold(id, r.Item) {
				return true
			}
		}
	}
	return false
}

// startShop starts a visit when one is due: at the merchant in sight, else
// at the merchant spawn within Reach, nearest the bot that sells something
// of the first tier it wants (the budget's order, buyRows, kept across
// merchants: supplies at the grocer before gear at the armorer, #441), else
// something it wants, or for a visit only to sell, at any merchant. It
// reports whether it started one.
func (b *Brain) startShop(now time.Time) bool {
	sh, p := &b.shop, &b.persona.Shop
	if b.cfg.Travel == nil || now.Before(sh.nextAt) {
		return false
	}
	junk := len(b.junk())
	due := b.dueWants(junk > 0 && p.SellAt > 0)
	var want, first []buyRow
	for _, w := range due {
		want = append(want, w.buyRow)
		if w.tier == due[0].tier {
			first = append(first, w.buyRow)
		}
	}
	// A bag at or near its slot limit sells what junk it has (#528), and
	// so does a load that sends the bot home (loadSendsHome): the load
	// trip sheds that junk (shed), and the half-speed stash leaves it to
	// the visit (stashRows, #449).
	sellDue := p.SellAt > 0 && (junk >= p.SellAt || junk > 0 && (b.bagFull() || b.loadSendsHome()))
	sell := sellDue || (p.SellAt > 0 && len(want) > 0 && junk > 0)
	if len(want) == 0 && !sell {
		return false
	}
	shops, from := b.cfg.Travel.shops, b.world.Self.Position(now)
	sells := func(l shopList, rows []buyRow) bool {
		return slices.ContainsFunc(rows, func(r buyRow) bool { _, ok := l.sells(r.Item); return ok })
	}
	// lists returns the lists of merchant npcID the visit opens; ok is
	// false for a merchant the visit has no use for: one selling none of
	// need, unless anyMerchant and selling is due on its own.
	lists := func(npcID int32, need []buyRow, anyMerchant bool) (ids []int32, ok bool) {
		if sh.shunned.held(npcID, now) {
			return nil, false
		}
		for _, l := range shops[npcID] {
			if sells(l, want) {
				ids = append(ids, l.id)
			}
			ok = ok || sells(l, need)
		}
		return ids, ok || (anyMerchant && sellDue && len(shops[npcID]) > 0)
	}
	pick := func(need []buyRow, anyMerchant bool) (best shopVisit) {
		bestDist := int64(math.MaxInt64)
		for id, o := range b.world.Objects {
			if o.Kind != KindNPC || o.Dead {
				continue
			}
			at := o.Position(now)
			if d := dist2(from, at); d < bestDist {
				if ids, ok := lists(o.NPCID, need, anyMerchant); ok {
					best, bestDist = shopVisit{v: npcVisit{npcID: o.NPCID, at: at, obj: id}, lists: ids}, d
				}
			}
		}
		if best.v.active() {
			return best
		}
		reach := int64(p.Reach) * int64(p.Reach)
		for npcID := range shops {
			for _, at := range b.cfg.Travel.spawns[npcID] {
				if d := dist2(from, at); d <= reach && d < bestDist {
					if ids, ok := lists(npcID, need, anyMerchant); ok {
						best, bestDist = shopVisit{v: npcVisit{npcID: npcID, at: at}, lists: ids}, d
					}
				}
			}
		}
		return best
	}
	best := pick(first, false)
	if !best.v.active() {
		best = pick(want, true)
	}
	if !best.v.active() {
		sh.nextAt = now.Add(shopRetry)
		sh.rounds++
		return false
	}
	sh.v, sh.lists, sh.sell = best.v, best.lists, sell
	return true
}

// listWanted reports whether buylist listID of merchant npcID sells an
// item the bot wants now.
func (b *Brain) listWanted(npcID, listID int32) bool {
	want := b.shopWants(true)
	for _, l := range b.cfg.Travel.shops[npcID] {
		if l.id == listID {
			return slices.ContainsFunc(want, func(r buyRow) bool { _, ok := l.sells(r.Item); return ok })
		}
	}
	return false
}

// carried is how many of item template the bot carries, every stack
// counted.
func (b *Brain) carried(item int32) int32 {
	var n int32
	for _, it := range b.world.Inventory.Items {
		if it.TemplateID == item {
			n += it.Count
		}
	}
	return n
}

// junk are the object ids of the stacks the bot would sell (ShopPolicy
// SellAt). A piece the gear errand wears next is not junk, past the
// pieces its jewelry slot takes (spareJewelry), nor a weapon it
// falls back to (fallsBack), nor a piece of the set it buys towards
// (keepsForSet), nor a valuable (Brain.valuable); the one it replaced
// otherwise is.
func (b *Brain) junk() map[int32]Item {
	out := make(map[int32]Item)
	w := b.worn()
	spare := b.spareJewelry(&w)
	for id, it := range b.world.Inventory.Items {
		if it.Equipped || it.SubCategory == subCategoryQuest || it.Augmentation != 0 || b.keepsItem(it) || b.shop.unsellable[it.TemplateID] || b.upgrade(it, &w) && !spare[id] || b.fallsBack(it, &w) || b.keepsForSet(it, &w) || b.valuable(it) {
			continue
		}
		out[id] = it
	}
	return out
}

// keeps reports whether the bot keeps item template id rather than sell
// it: adena, Shop.Keep, Potions, Shots, Restock items, the keep set
// (inKeepSet), every template with a trip target (supplies, the Extra
// ones included), the PvP cures and CP potions (keepsPvPItem), the arrows
// the wielded bow and the one worn next shoot, the shots keepsShot keeps,
// spellbooks wanted or held for a learn and the enchant scrolls of a grade
// the bot wears (keepsScroll).
func (b *Brain) keeps(id int32) bool {
	return id == adenaID || slices.Contains(b.persona.Shop.Keep, id) || b.uses(id)
}

// uses reports whether the bot keeps item template id for its own use:
// keeps but adena and Shop.Keep, which it keeps with no use of its own
// (the half-speed stash deposits those, stashes).
func (b *Brain) uses(id int32) bool {
	p := &b.persona
	if _, shot := b.shotTable()[id]; shot && b.keepsShot(id) {
		return true
	}
	return b.inKeepSet(id) || slices.Contains(p.Potions, id) ||
		slices.Contains(p.Shots, id) || b.shop.books[id] || b.shop.held[id] ||
		slices.ContainsFunc(p.Shop.Restock, func(r Restock) bool { return r.Item == id }) ||
		id == b.bowArrows() || id == b.nextBowArrows() || b.keepsScroll(id) || b.keepsPvPItem(id) || b.usesReagent(id) ||
		slices.ContainsFunc(b.supplies(), func(s supply) bool { return s.item == id })
}

// keepsItem reports whether the bot keeps carried item it rather than sell,
// list or offer it: its template is kept (keeps) or the persona's keep
// hook keeps it (Persona.Holds).
func (b *Brain) keepsItem(it Item) bool {
	return b.keeps(it.TemplateID) || b.hold(it).Keep
}

// hold is the persona's keep hook's answer for carried item it
// (Persona.Holds): the zero Hold without a hook.
func (b *Brain) hold(it Item) Hold {
	if b.persona.Holds == nil {
		return Hold{}
	}
	return b.persona.Holds(b.world, it)
}

// sellRows is the sale from window l: every junk stack it lists, whole.
// Junk it leaves out is not sellable and stays junk no more.
func (b *Brain) sellRows(l SellList) []sellRow {
	junk := b.junk()
	var rows []sellRow
	for _, it := range l.Items {
		if j, ok := junk[it.ObjectID]; ok && len(rows) < shopMaxRows {
			rows = append(rows, sellRow{Object: it.ObjectID, Item: j.TemplateID, Count: it.Count})
			delete(junk, it.ObjectID)
		}
	}
	if len(rows) < shopMaxRows {
		b.markUnsellable(junk)
	}
	return rows
}

// markUnsellable records the templates of stacks a sell window left out.
func (b *Brain) markUnsellable(items map[int32]Item) {
	sh := &b.shop
	for _, it := range items {
		if sh.unsellable == nil {
			sh.unsellable = make(map[int32]bool)
		}
		sh.unsellable[it.TemplateID] = true
	}
}

// shopEvent moves the visit on with the server's answer ev, and hands a
// bought spellbook back to the trainer.
func (b *Brain) shopEvent(ev Event) {
	sh := &b.shop
	switch e := ev.(type) {
	case NpcHtmlMessage:
		if !sh.v.active() || sh.v.obj == 0 || e.ObjectID != sh.v.obj {
			return
		}
		switch {
		case sh.step == shopAskSell:
			// The merchant's -empty page: nothing is sellable.
			b.markUnsellable(b.junk())
			sh.sell, sh.step = false, shopIdle
			sh.v.waitUntil = time.Time{}
		case sh.talked:
			sh.talked = false
			sh.v.page(e)
		}
	case SellList:
		if sh.step == shopAskSell {
			sh.sellList, sh.step, sh.v.waitUntil = &e, shopIdle, time.Time{}
		}
	case BuyList:
		if sh.step == shopAskBuy && len(sh.lists) > 0 && e.ListID == sh.lists[0] {
			sh.buyList, sh.step, sh.v.waitUntil = &e, shopIdle, time.Time{}
		}
	case ItemList:
		if sh.step == shopBuying {
			sh.dropList()
			sh.step, sh.v.waitUntil = shopIdle, time.Time{}
		}
		b.booksArrived()
	case InventoryUpdate:
		if sh.step == shopSelling && slices.ContainsFunc(e.Changes, func(c ItemChange) bool { return c.Item.TemplateID == adenaID }) {
			sh.sell, sh.step, sh.v.waitUntil = false, shopIdle, time.Time{}
		}
		b.booksArrived()
	}
}

// booksArrived moves the wanted spellbooks the bot now carries to held,
// kept out of junk, and makes a trainer visit due again for them. A held
// book the bot no longer carries, used by a learn, is held no more.
func (b *Brain) booksArrived() {
	sh := &b.shop
	for id := range sh.held {
		if b.carried(id) == 0 {
			delete(sh.held, id)
		}
	}
	for id := range sh.books {
		if b.carried(id) > 0 {
			delete(sh.books, id)
			if sh.held == nil {
				sh.held = make(map[int32]bool)
			}
			sh.held[id] = true
			b.train.doneLevel = 0
			// A visit under way may have skipped the book's skill
			// already: it ends due again.
			b.train.again = b.train.npc != 0
		}
	}
}

// missingItem is the first item requirement of a skill level the bot
// lacks, reporting false when it has them all.
func (b *Brain) missingItem(reqs []SkillRequirement) (SkillRequirement, bool) {
	for _, r := range reqs {
		if r.ItemID > 0 && b.carried(r.ItemID) < max(r.Count, 1) {
			return r, true
		}
	}
	return SkillRequirement{}, false
}
