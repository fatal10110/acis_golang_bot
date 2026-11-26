package botclient

import (
	"cmp"
	"slices"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// dyeReplyTimeout is how long the dye errand waits for the answer to its
// last request: the draw window for the Symbol Maker's Draw link, and
// HennaInfo then the InventoryUpdate for a draw or a removal. The server
// drops a draw of an unknown symbol and a removal of a symbol not worn in
// silence (network/henna.go drawHenna, deleteHenna).
const dyeReplyTimeout = 5 * time.Second

// symbolMakers are the Symbol Makers' NPC template ids (title "Symbol
// Maker", data/xml/npcs/31000-31999.xml).
var symbolMakers = []int32{31046, 31047, 31048, 31049, 31050, 31051, 31052, 31053, 31264, 31308, 31953}

// drawCommand is the Symbol Maker page's link that opens the draw window
// (data/html/symbolmaker/SymbolMaker.htm; npc/folk_bypass.go answers it
// with HennaEquipList).
const drawCommand = "Draw"

// DyePolicy is which dye symbols the bot wears and where it draws them
// (rule 9 errand "dyes", D15). Every value is a configurable default (D6).
type DyePolicy struct {
	// Symbols is the persona's dye table: the symbol ids the bot wears, by
	// the class it has, drawn in order into its free slots; an id may
	// repeat. An id the datapack does not hold, or whose class list leaves
	// the class out, is skipped. A class with no entry draws nothing. nil
	// plays the build's symbols (Persona, overrides).
	Symbols map[int32][]int32
	// Makers are the NPC template ids of the Symbol Makers.
	Makers []int32
	// Reach is how far the bot walks to a maker's spawn (from the travel
	// map, BrainConfig.Travel); a maker in sight is visited from anywhere,
	// and 0 visits only one in sight.
	Reach int32
	// RetryAfter is how long the bot leaves the makers alone after a visit
	// that could not finish: a maker not at its spawn or out of reach, a
	// page without the Draw link, a request refused or unanswered within
	// dyeReplyTimeout, or a symbol the draw window does not offer.
	RetryAfter time.Duration
}

// defaultDyeTable is the dye table builds compile (CompileBuild), each row checked
// against hennas.xml's class lists. First professions (2 slots) wear the
// 37000-adena +1/-3 rows: fighters STR+1 CON-3 (symbol 1) and DEX+1 CON-3
// (6); nukers INT+1 WIT-3 (8); healers, whom the INT/WIT rows leave out,
// MEN+1 WIT-3 (10). Second professions (3 slots) wear the 87000-adena
// symbols 37+, which list only them, of their row's direction
// (docs/strategy.md, "Gear and range", column 11): CON+1 DEX-3 (40) for the
// knights; STR+1 DEX-3 (38) for the sword singer, bladedancer, plains
// walker, silver ranger, abyss walker and phantom ranger; STR+1 CON-3 (37) for the other
// fighters; DEX+1 CON-3 (42) twice and 37 for the treasure hunter and
// hawkeye; WIT+1 MEN-3 (48) for the bishop, prophet and elven elder; MEN+1
// WIT-3 (46) for the shillien elder and warcryer; INT+1 WIT-3 (44) for the
// spellhowler, INT+1 MEN-3 (43) for the spellsinger, 44 twice and 43 for
// the sorcerer. The professions without a row of their own (dark avenger,
// warsmith, the servitor lines, overlord) keep the fighters' 37, 42, 37,
// the nukers' 44 and the healers' 46.
func defaultDyeTable() map[int32][]int32 {
	t := make(map[int32][]int32)
	set := func(symbols []int32, classes ...int32) {
		for _, c := range classes {
			t[c] = slices.Clone(symbols)
		}
	}
	set([]int32{1, 6}, 1, 4, 7, 19, 22, 32, 35, 45, 47, 54, 56)
	set([]int32{8, 8}, 11, 26, 39)
	set([]int32{10, 10}, 15, 29, 42, 50)
	set([]int32{40, 40, 40}, 5, 20, 33)
	set([]int32{38, 38, 38}, 21, 23, 24, 34, 36, 37)
	set([]int32{37, 37, 37}, 2, 3, 46, 48, 55)
	set([]int32{42, 37, 42}, 8, 9)
	set([]int32{48, 48, 48}, 16, 17, 30)
	set([]int32{46, 46, 46}, 43, 52)
	set([]int32{44, 43, 44}, 12)
	set([]int32{43, 43, 43}, 27)
	set([]int32{44, 44, 44}, 40)
	set([]int32{37, 42, 37}, 6, 57)
	set([]int32{44, 44, 44}, 13, 14, 28, 41)
	set([]int32{46, 46, 46}, 51)
	return t
}

// defaultDyePolicy draws the default dye table at a Symbol Maker nearby; a
// visit that could not finish waits visitShunFor before the next.
func defaultDyePolicy() DyePolicy {
	return DyePolicy{
		Makers:     slices.Clone(symbolMakers),
		Reach:      8000,
		RetryAfter: visitShunFor,
	}
}

// dyeVisit is the bot's errand at a Symbol Maker (rule "dyes"). It is
// owned by the brain goroutine.
type dyeVisit struct {
	v npcVisit
	// listed is set once the draw window came for the Draw link, list
	// holding its symbols.
	listed bool
	list   []HennaRow
	// asked is the kind of the request whose answer is awaited (actBypass
	// for the Draw link, actHennaEquip, actHennaUnequip), actHold for
	// none, and waitUntil when it is given up on. answered is set once a
	// draw's or a removal's HennaInfo came: the visit then waits for the
	// InventoryUpdate that follows it (task/inventoryupdates.go), so its
	// dye and adena counts are current.
	asked     actionKind
	waitUntil time.Time
	answered  bool
	// retryAt is when the next visit may start after one that could not
	// finish.
	retryAt time.Time
}

// end ends the visit.
func (d *dyeVisit) end() {
	d.v, d.listed, d.list = npcVisit{}, false, nil
	d.asked, d.waitUntil, d.answered = actHold, time.Time{}, false
}

// backOff ends the visit and leaves the makers alone for dur.
func (d *dyeVisit) backOff(now time.Time, dur time.Duration) {
	d.end()
	d.retryAt = now.Add(dur)
}

// sent records a packet of the visit that went out at now: the Draw link
// awaits the draw window, a draw or a removal its HennaInfo.
func (d *dyeVisit) sent(a Action, now time.Time) {
	if !d.v.active() {
		return
	}
	switch {
	case a.kind == actBypass && a.object != 0 && a.object == d.v.obj:
		d.asked, d.waitUntil, d.listed, d.list = actBypass, now.Add(dyeReplyTimeout), false, nil
	case a.kind == actHennaEquip || a.kind == actHennaUnequip:
		d.asked, d.waitUntil, d.answered = a.kind, now.Add(dyeReplyTimeout), false
	}
	d.v.sent(a, now)
}

// ruleDyes is a rule 9 errand (D15): with a worn symbol its class no
// longer uses, or the dyes and the fee of a symbol of its table in hand,
// the bot walks to a Symbol Maker and talks to it, as a player does,
// although the server does not require one. It removes the inactive
// symbols (RequestHennaUnequip), follows the page's Draw link and draws
// the table's symbols the window offers into its free slots
// (RequestHennaEquip). One request per answer: the draw window, or
// HennaInfo then the InventoryUpdate after it, waited for dyeReplyTimeout
// (HennaInfo alone settles the request at the timeout: the update may
// have come first).
// A refusal (CantDrawSymbol, SymbolsFull, YouNotEnoughAdena,
// NotEnoughItems), a silence, a page without the link or a maker not found
// ends the visit for RetryAfter. Since the server takes the adena before
// the dyes and answers a missing dye like a class refusal, the bot checks
// the class list, its dyes and its adena past the reserve before it sends.
// The Draw link passes the bypass gate (gateBypass); legs and clicks the
// frame pacer, a visit spending at most visitClicks clicks.
func (b *Brain) ruleDyes(now time.Time) (Action, bool) {
	d, p := &b.dyes, &b.persona.Dyes
	if b.world.Self.InCombat || (!d.v.active() && !b.startDyes(now)) {
		return Action{}, false
	}
	if d.asked != actHold {
		if now.Before(d.waitUntil) {
			return Action{}, true
		}
		if !d.answered {
			// Unanswered: the server dropped the request in silence.
			d.backOff(now, p.RetryAfter)
			return Action{}, false
		}
		// HennaInfo came but no InventoryUpdate after it: the server
		// flushes inventory updates from its own 333 ms ticker, so the
		// update can come before HennaInfo (task/inventoryupdates.go).
		d.asked, d.waitUntil, d.answered = actHold, time.Time{}, false
	}
	v := &d.v
	if !v.opened && !d.listed {
		a, ok := b.approach(v, now)
		if !ok {
			d.backOff(now, p.RetryAfter)
			return Action{}, false
		}
		return a, true
	}
	remove, draw := b.dyePlan()
	if id, ok := b.nextRemoval(remove); ok {
		return hennaUnequipAction(id), true
	}
	if _, ok := b.nextDraw(draw, nil); !ok {
		// Nothing left to draw this visit.
		d.end()
		return Action{}, false
	}
	if !d.listed {
		if cmd := "npc_" + strconv.Itoa(int(v.obj)) + "_" + drawCommand; b.world.Dialog.Has(cmd) {
			return bypassTo(v.obj, cmd), true
		}
		d.backOff(now, p.RetryAfter)
		return Action{}, false
	}
	if id, ok := b.nextDraw(draw, d.list); ok {
		return hennaEquipAction(id), true
	}
	// The window does not offer the symbol: the server does not let the
	// class draw it.
	d.backOff(now, p.RetryAfter)
	return Action{}, false
}

// dyesDue reports whether a visit has work: a removal or a draw the bot
// can pay for now.
func (b *Brain) dyesDue() bool {
	remove, draw := b.dyePlan()
	_, r := b.nextRemoval(remove)
	_, d := b.nextDraw(draw, nil)
	return r || d
}

// startDyes starts a visit when one is due, at the nearest maker in sight,
// else at the nearest maker spawn within Reach, reporting whether it did.
func (b *Brain) startDyes(now time.Time) bool {
	d, p := &b.dyes, &b.persona.Dyes
	if now.Before(d.retryAt) || !b.dyesDue() {
		return false
	}
	v, ok := b.pickVisit(p.Makers, p.Reach, now)
	if !ok {
		return false
	}
	d.end()
	d.v = v
	return true
}

// dyePlan is what the bot does with its symbols: remove, the worn symbols
// its class cannot use (HennaInfo active id 0) that the datapack holds,
// in slot order; draw, as many symbols as the slots free after the
// removals hold. With the model (valuer) and no persona table, draw are
// the symbols of the highest value gain (dyeDraws); else the symbols of the
// table (DyePolicy.Symbols, else the build's) the bot does not wear. Both
// are empty without a travel map and before the first HennaInfo; draw is
// empty for a class with no slot.
func (b *Brain) dyePlan() (remove, draw []datapack.Henna) {
	m, sy := b.cfg.Travel, &b.world.Symbols
	if m == nil || !sy.Known {
		return nil, nil
	}
	var kept []int32
	free := sy.MaxSlots
	for _, w := range sy.Worn {
		if h, ok := m.hennas[w.SymbolID]; ok && w.Active == 0 {
			remove = append(remove, h)
			continue
		}
		free--
		kept = append(kept, w.SymbolID)
	}
	if free <= 0 {
		return remove, nil
	}
	if b.persona.Dyes.Symbols == nil {
		if v := b.valuer(); v != nil {
			return remove, b.dyeDraws(v, int(free), remove)
		}
	}
	class := b.world.Self.ClassID
	var want []datapack.Henna
	for _, id := range b.build().Kit.Dyes {
		if h, ok := m.hennas[id]; ok && slices.Contains(h.Classes, class) {
			want = append(want, h)
		}
	}
	for _, id := range kept {
		if i := slices.IndexFunc(want, func(h datapack.Henna) bool { return h.SymbolID == id }); i >= 0 {
			want = slices.Delete(want, i, i+1)
		}
	}
	return remove, want[:min(int(free), len(want))]
}

// dyeDraws are the symbols to draw into free slots by the model v
// (docs/strategy.md, Design section 5): slot by slot, of the symbols
// hennas.xml lets the bot's class draw that raise one of its kit's dye
// stats (ClassKit.DyeStats; any symbol with none), the one whose stat
// changes, added to the active symbols' and those drawn before it, add the
// most value (the model holds each stat's dye bonus to +5, as the server
// does), ties to the cheaper then the lower symbol id; only a symbol whose
// draw price and the dyes it lacks, at a merchant's base price, the adena
// past the gear tier's floor still pays after the removals' fees and the
// symbols before it. The kit's dye stats are the row's direction, drawn
// whatever the model makes of the stat they pay with; with none, a symbol
// is drawn only while it adds value.
func (b *Brain) dyeDraws(v *gearModel, free int, remove []datapack.Henna) []datapack.Henna {
	m, class := b.cfg.Travel, b.world.Self.ClassID
	stats := b.build().Kit.DyeStats
	var cands []datapack.Henna
	for _, h := range m.hennas {
		if slices.Contains(h.Classes, class) && (len(stats) == 0 || slices.ContainsFunc(stats, func(s string) bool { return dyeStat(h, s) > 0 })) {
			cands = append(cands, h)
		}
	}
	slices.SortFunc(cands, func(x, y datapack.Henna) int { return cmp.Compare(x.SymbolID, y.SymbolID) })
	left := int64(b.carried(adenaID)) - b.floor(tierGear)
	for _, h := range remove {
		left -= int64(h.Price / hennaRemoveFraction)
	}
	cur := v.build.Dyes
	w := b.rawWorn()
	need := make(map[int32]int32)
	var draw []datapack.Henna
	for range free {
		var best datapack.Henna
		var bestGain float64
		var bestCost int64
		for _, h := range cands {
			price, ok := m.cheapest[h.DyeID]
			if !ok {
				continue
			}
			have := b.carried(h.DyeID)
			buy := max(need[h.DyeID]+hennaDrawDyes-have, 0) - max(need[h.DyeID]-have, 0)
			cost := int64(buy)*int64(price) + int64(h.Price)
			if cost > left {
				continue
			}
			gain := v.delta(m, &w, true, addDye(cur, h), &w, true, cur)
			if gain <= 0 && len(stats) == 0 {
				continue
			}
			if best.SymbolID == 0 || cmp.Or(cmp.Compare(gain, bestGain), cmp.Compare(best.Price, h.Price)) > 0 {
				best, bestGain, bestCost = h, gain, cost
			}
		}
		if best.SymbolID == 0 {
			break
		}
		draw = append(draw, best)
		cur, left = addDye(cur, best), left-bestCost
		need[best.DyeID] += hennaDrawDyes
	}
	return draw
}

// dyeStat is how much symbol h changes the base stat named stat (STR,
// CON, DEX, INT, WIT or MEN).
func dyeStat(h datapack.Henna, stat string) int {
	switch stat {
	case "STR":
		return h.STR
	case "CON":
		return h.CON
	case "DEX":
		return h.DEX
	case "INT":
		return h.INT
	case "WIT":
		return h.WIT
	case "MEN":
		return h.MEN
	}
	return 0
}

// dyeAdena is the adena the bot may spend at a Symbol Maker: its adena
// past the reserve (the trip home).
func (b *Brain) dyeAdena() int64 {
	return int64(b.carried(adenaID)) - b.reserve()
}

// nextRemoval is the first symbol of remove whose removal, a fifth of its
// draw price, the bot can pay.
func (b *Brain) nextRemoval(remove []datapack.Henna) (int32, bool) {
	for _, h := range remove {
		if int64(h.Price/hennaRemoveFraction) <= b.dyeAdena() {
			return h.SymbolID, true
		}
	}
	return 0, false
}

// nextDraw is the first symbol of draw the bot can draw now: it carries
// hennaDrawDyes of its dye and can pay its price, and, when list is not
// nil, the draw window offers it.
func (b *Brain) nextDraw(draw []datapack.Henna, list []HennaRow) (int32, bool) {
	for _, h := range draw {
		if b.carried(h.DyeID) < hennaDrawDyes || int64(h.Price) > b.dyeAdena() {
			continue
		}
		if list != nil && !slices.ContainsFunc(list, func(r HennaRow) bool { return r.SymbolID == h.SymbolID }) {
			continue
		}
		return h.SymbolID, true
	}
	return 0, false
}

// dyeWants are the dyes the shop errand buys for the symbols the bot draws
// next (dyePlan) and how many (hennaDrawDyes per symbol, less what it
// carries), and fee, the adena the purchase leaves for the Symbol Maker:
// the draw prices of those symbols and the removals ahead of them. A
// symbol is planned, in table order, only while the adena past the gear
// tier's floor pays its fee and its dyes at a merchant's base price, so a
// symbol the bot cannot pay in full is not bought for; nor one whose dye
// no merchant sells.
func (b *Brain) dyeWants() (rows []buyRow, fee int64) {
	m := b.cfg.Travel
	if m == nil {
		return nil, 0
	}
	remove, draw := b.dyePlan()
	for _, h := range remove {
		fee += int64(h.Price / hennaRemoveFraction)
	}
	left := int64(b.carried(adenaID)) - b.floor(tierGear) - fee
	need := make(map[int32]int32)
	var order []int32
	for _, h := range draw {
		price, ok := m.cheapest[h.DyeID]
		if !ok {
			continue
		}
		have := b.carried(h.DyeID)
		buy := max(need[h.DyeID]+hennaDrawDyes-have, 0) - max(need[h.DyeID]-have, 0)
		cost := int64(buy)*int64(price) + int64(h.Price)
		if cost > left {
			break
		}
		left -= cost
		fee += int64(h.Price)
		if need[h.DyeID] == 0 {
			order = append(order, h.DyeID)
		}
		need[h.DyeID] += hennaDrawDyes
	}
	for _, id := range order {
		if n := need[id] - b.carried(id); n > 0 {
			rows = append(rows, buyRow{item: id, count: n})
		}
	}
	return rows, fee
}

// plannedDye reports whether template id is the dye of a symbol the bot
// draws next (dyePlan): the keep set holds it until it is drawn. A template
// that is no dye never is, and asks no plan: the plan's budget reads the
// gear floor, whose valuables deposit fees (valuableFees) ask the keep set
// of every carried item, so asking the plan for each of them recursed
// without end and overflowed the stack (a merchant with valuables and a
// free dye slot; a dye is never a valuable, valuableTemplate).
func (b *Brain) plannedDye(id int32) bool {
	if m := b.cfg.Travel; m == nil || !m.dyes[id] {
		return false
	}
	_, draw := b.dyePlan()
	return slices.ContainsFunc(draw, func(h datapack.Henna) bool { return h.DyeID == id })
}

// dyePage moves the visit on with the maker's page.
func (b *Brain) dyePage(ev NpcHtmlMessage) {
	if b.dyes.v.active() {
		b.dyes.v.page(ev)
	}
}

// dyeList takes the draw window the Draw link asked for.
func (b *Brain) dyeList(ev HennaEquipList) {
	if d := &b.dyes; d.v.active() && d.asked == actBypass {
		d.listed, d.list, d.asked, d.waitUntil = true, ev.Symbols, actHold, time.Time{}
	}
}

// dyeAnswered takes the HennaInfo a draw or a removal answers: the visit
// then waits for the InventoryUpdate behind it.
func (b *Brain) dyeAnswered() {
	if d := &b.dyes; d.v.active() && (d.asked == actHennaEquip || d.asked == actHennaUnequip) {
		d.answered = true
	}
}

// dyeSettled ends the wait of an answered draw or removal once the
// inventory shows what it spent and gave.
func (b *Brain) dyeSettled() {
	if d := &b.dyes; d.answered {
		d.asked, d.waitUntil, d.answered = actHold, time.Time{}, false
	}
}

// dyeRefused ends the visit on a system message refusing its pending draw
// or removal: the server sends no HennaInfo after one.
func (b *Brain) dyeRefused(id int32, now time.Time) {
	d := &b.dyes
	if !d.v.active() || d.asked != actHennaEquip && d.asked != actHennaUnequip || d.answered {
		return
	}
	switch id {
	case msgCantDrawSymbol, msgSymbolsFull, msgNotEnoughAdena, msgNotEnoughItems:
		d.backOff(now, b.persona.Dyes.RetryAfter)
	}
}
