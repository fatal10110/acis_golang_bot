package botclient

import (
	"maps"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/craftplan"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// memberState is the clan member option's state (docs/crafting.md section
// 7, Persona.ClanMember). It is owned by the brain goroutine: every hook
// wireMember installs runs on it.
type memberState struct {
	// due is set by a supply trip's arrival in town (the trip in town hook)
	// until the warehouse visit it asks for is handed to the errand.
	due bool
	// asked is set from the visit's request handed to the errand until it
	// is settled (ruleMember); askedAt is when. orders are the shot orders
	// (template to count) whose ore the visit's deposit carries, and lease
	// is set when the visit took the board's warehouse lease to withdraw.
	asked   bool
	askedAt time.Time
	orders  map[int32]int64
	lease   bool
	// skipWithdraw leaves the withdrawal out of the next visit after one
	// the server left unanswered (no warehouse privilege): a silent
	// refusal fails the whole request, and the deposit must not wait on a
	// withdrawal that never comes.
	skipWithdraw bool
	// fees are the deposit fees the shop visit leaves (memberFees): the
	// planned visit's on the trip's arrival in town, the asked visit's
	// exact ones once handed to the errand, zero once it is settled. They
	// are fixed at those two points because reserve runs on every tick.
	fees int64
	// scrolls maps each recipe scroll template of the travel map to the
	// recipes it teaches, built on first use.
	scrolls map[int32][]int32
	// inHold and inShots guard the hooks against their own re-entry: the
	// keep hook asks the gear errand whether it would wear an item, which
	// asks the keep hook back; the shot orders are sized by trip targets,
	// which read the extra supply hook that asks for their ore.
	inHold, inShots bool
	// holds is the persona's own keep hook, which wireMember wraps; wired
	// is set once it did. The valuable test asks only the own hook
	// (Brain.ownHold): the member's hold comes after valuable (D31).
	holds func(w *World, it Item) Hold
	wired bool
}

// wireMember installs the clan member option on the brain's persona: it
// wraps the keep hook (Holds), the supply trip's hooks (Extra, InTown,
// HandOff) and the warehouse errand's Want, each after the persona's own.
// The hooks do nothing unless the option is active (memberActive).
func (b *Brain) wireMember() {
	p := &b.persona
	holds, extra, inTown, handOff, want := p.Holds, p.Supply.Extra, p.Supply.InTown, p.Supply.HandOff, p.Warehouse.Want
	b.member.holds, b.member.wired = holds, true
	p.Holds = func(w *World, it Item) Hold {
		var h Hold
		if holds != nil {
			h = holds(w, it)
		}
		m := b.memberHold(it)
		return Hold{Keep: h.Keep || m.Keep, NoWear: h.NoWear || m.NoWear}
	}
	p.Supply.Extra = func(w *World) map[int32]int32 {
		out := make(map[int32]int32)
		if extra != nil {
			maps.Copy(out, extra(w))
		}
		for id, n := range b.memberOre(w) {
			out[id] = int32(min(int64(out[id])+int64(n), int64(1<<31-1)))
		}
		return out
	}
	p.Supply.InTown = func(w *World) {
		if inTown != nil {
			inTown(w)
		}
		if b.memberActive() {
			b.member.due = true
			b.member.fees = depositFees(b.memberDeposit(b.memberOrders()))
		}
	}
	p.Supply.HandOff = func(w *World) int64 {
		var n int64
		if handOff != nil {
			n = max(handOff(w), 0)
		}
		return n + b.memberHandOff()
	}
	p.Warehouse.Want = func(w *World, st WarehouseStatus, now time.Time) WarehouseRequest {
		if want != nil {
			if r := want(w, st, now); !r.empty() {
				return r
			}
		}
		return b.memberWant(now)
	}
}

// memberActive reports whether the clan member option plays: the persona
// sets it, the bot is in a clan with a crafter on the board, and the travel
// map has the recipes.
func (b *Brain) memberActive() bool {
	board, m, clan := b.cfg.CraftBoard, b.cfg.Travel, b.world.Clan.ID
	return b.persona.ClanMember && board != nil && m != nil && m.recipes != nil && clan != 0 && len(board.Crafters(clan)) > 0
}

// memberHold is the member option's keep hook answer for carried item it:
// keep every recipe scroll and every material of a recipe in a clan
// crafter's book or whose scroll the clan warehouse holds (as the bot last
// saw it); keep, and never wear, graded gear (gear the server
// crystallizes) the bot would not wear, kept to deposit for crystallizing.
// Worn and augmented items are none of these.
func (b *Brain) memberHold(it Item) Hold {
	st := &b.member
	if st.inHold || it.Equipped || it.Augmentation != 0 || !b.memberActive() {
		return Hold{}
	}
	return b.memberKeep(it, b.memberMaterials())
}

// memberKeep is memberHold for an item none of its guards excludes, with
// the materials the member keeps (memberMaterials) built by the caller. A
// valuable is kept for none (D31): the member's hold comes after it, so
// memberDeposit, memberHandOff and the fees leave it out. The valuable
// test and the gear errand ask the keep hook back, so both run under
// inHold.
func (b *Brain) memberKeep(it Item, materials map[int32]bool) Hold {
	st := &b.member
	st.inHold = true
	defer func() { st.inHold = false }()
	if b.valuable(it) {
		return Hold{}
	}
	if _, ok := b.recipeScrolls()[it.TemplateID]; ok || materials[it.TemplateID] {
		return Hold{Keep: true}
	}
	if _, graded := b.cfg.Travel.crystals[it.TemplateID]; !graded {
		return Hold{}
	}
	w := b.worn()
	if b.upgrade(it, &w) || b.fallsBack(it, &w) || b.keepsForSet(it, &w) {
		return Hold{}
	}
	return Hold{Keep: true, NoWear: true}
}

// recipeScrolls maps the travel map's recipe scroll templates to the
// recipes each teaches.
func (b *Brain) recipeScrolls() map[int32][]int32 {
	st := &b.member
	if st.scrolls == nil {
		st.scrolls = make(map[int32][]int32)
		for _, id := range slices.Sorted(maps.Keys(b.cfg.Travel.recipes)) {
			if s := b.cfg.Travel.recipes[id].ScrollID; s != 0 {
				st.scrolls[s] = append(st.scrolls[s], id)
			}
		}
	}
	return st.scrolls
}

// memberMaterials are the material templates the member keeps: those of
// the recipes in the books of its clan's crafters, and of the recipes
// whose scrolls the clan warehouse holds as the bot last saw it.
func (b *Brain) memberMaterials() map[int32]bool {
	m := b.cfg.Travel
	out := make(map[int32]bool)
	add := func(id int32) {
		for _, g := range m.recipes[id].Materials {
			out[g.ItemID] = true
		}
	}
	for _, e := range b.cfg.CraftBoard.Crafters(b.world.Clan.ID) {
		for _, id := range e.Recipes {
			add(id)
		}
	}
	if b.wh.status.Known {
		scrolls := b.recipeScrolls()
		for _, it := range b.wh.status.Contents {
			for _, id := range scrolls[it.TemplateID] {
				add(id)
			}
		}
	}
	return out
}

// memberShotGrades are the shot grades the crafters of the bot's clan can
// make now (CrafterEntry.ShotGrades), none unless the option is active.
func (b *Brain) memberShotGrades() []int {
	if !b.memberActive() {
		return nil
	}
	var out []int
	for _, e := range b.cfg.CraftBoard.Crafters(b.world.Clan.ID) {
		for _, g := range e.ShotGrades {
			if !slices.Contains(out, g) {
				out = append(out, g)
			}
		}
	}
	return out
}

// clanShots is the clan crafter as a shot source (I2, docs/crafting.md
// section 7): a shot of a grade some crafter of the clan can make now.
type clanShots struct {
	grades []int
	table  map[int32]shotItem
}

func (c clanShots) sellsShot(id int32) bool {
	s, ok := c.table[id]
	return ok && !s.beginner && slices.Contains(c.grades, int(s.grade))
}

// shotGradesNow are the crystal grades of the shots a crafter planning
// with in can make now (CrafterEntry.ShotGrades), ascending: a shot recipe
// in its book within its craft level, with the crystals of a craft in its
// pool (its crafting stock and the clan warehouse as last seen, less the
// board's reservations), the members bringing the ore, or the shots in
// stock there.
func (b *Brain) shotGradesNow(in craftplan.Input) []int {
	pool := maps.Clone(in.Inventory)
	if pool == nil {
		pool = make(map[int32]int64)
	}
	if b.wh.status.Known {
		for _, it := range b.wh.status.Contents {
			pool[it.TemplateID] += int64(it.Count)
		}
	}
	for item, n := range in.Reserved {
		pool[item] -= n
	}
	table := b.shotTable()
	var out []int
	for _, id := range in.Book {
		r, ok := in.Recipes[id]
		s, shot := table[r.Product.ItemID]
		level := in.CommonLevel
		if r.Dwarven {
			level = in.DwarvenLevel
		}
		if !ok || !shot || s.beginner || r.Level > level || slices.Contains(out, int(s.grade)) {
			continue
		}
		crystals := !slices.ContainsFunc(r.Materials, func(g datapack.Ingredient) bool {
			return isCrystal(g.ItemID) && pool[g.ItemID] < int64(g.Count)
		})
		if crystals || pool[r.Product.ItemID] > 0 {
			out = append(out, int(s.grade))
		}
	}
	slices.Sort(out)
	return out
}

// memberShotWants are the shots the member orders from the clan crafter
// (template to count): each of its own shot supplies a clan crafter can
// make now, up to its trip target (tripTarget) less what its open orders
// already ask for. Not capped by the ore it carries: the extra supply hook
// buys the ore for them, and shotOrder caps the order.
func (b *Brain) memberShotWants() map[int32]int64 {
	st := &b.member
	if st.inShots {
		return nil
	}
	grades := b.memberShotGrades()
	if len(grades) == 0 {
		return nil
	}
	st.inShots = true
	defer func() { st.inShots = false }()
	src := clanShots{grades: grades, table: b.shotTable()}
	open := make(map[int32]int64)
	for _, o := range b.cfg.CraftBoard.Orders(b.world.Clan.ID, b.clock.Now()) {
		if o.By == OrderByMember && o.Member == b.world.Self.Name {
			open[o.Product] += o.Count
		}
	}
	out := make(map[int32]int64)
	for _, s := range b.supplies() {
		if s.extra || !src.sellsShot(s.item) {
			continue
		}
		if n := int64(s.count) - open[s.item]; n > 0 {
			out[s.item] = n
		}
	}
	return out
}

// memberOre is the member's extra supply (SupplyPolicy.Extra): the ore its
// shot orders need (shotOreSupply).
func (b *Brain) memberOre(w *World) map[int32]int32 {
	if !b.memberActive() {
		return nil
	}
	return shotOreSupply(b.cfg.Travel.recipes, func(*World) map[int32]int64 { return b.memberShotWants() })(w)
}

// memberOrders are the shot orders the member posts at its visit: each
// wanted shot (memberShotWants) as many as the ore it carries covers
// (shotOrder), taking each order's ore before sizing the next.
func (b *Brain) memberOrders() map[int32]int64 {
	wants := b.memberShotWants()
	out := make(map[int32]int64)
	carried := make(map[int32]int64)
	for _, it := range b.world.Inventory.Items {
		carried[it.TemplateID] += int64(it.Count)
	}
	for _, shot := range slices.Sorted(maps.Keys(wants)) {
		r, ok := shotRecipe(b.cfg.Travel.recipes, shot)
		if !ok {
			continue
		}
		n := min(wants[shot], oreShots(r, carried))
		if n <= 0 {
			continue
		}
		for item, c := range shotOre(r, n) {
			carried[item] -= c
		}
		out[shot] = n
	}
	return out
}

// memberDeposit is what the member's visit deposits: every unequipped
// item its keep hook keeps that the bot does not keep for its own use
// (keeps), quest items aside, by object; then the ore of orders.
func (b *Brain) memberDeposit(orders map[int32]int64) []WarehouseRow {
	var rows []WarehouseRow
	materials := b.memberMaterials()
	for _, it := range b.world.Inventory.Items {
		if it.Equipped || it.Augmentation != 0 || it.SubCategory == subCategoryQuest || b.member.inHold ||
			b.keeps(it.TemplateID) || !b.memberKeep(it, materials).Keep {
			continue
		}
		rows = append(rows, WarehouseRow{TemplateID: it.TemplateID, Count: it.Count, Object: it.ObjectID})
	}
	slices.SortFunc(rows, func(x, y WarehouseRow) int { return int(x.Object - y.Object) })
	ore := make(map[int32]int64)
	for _, shot := range slices.Sorted(maps.Keys(orders)) {
		if r, ok := shotRecipe(b.cfg.Travel.recipes, shot); ok {
			for item, n := range shotOre(r, orders[shot]) {
				ore[item] += n
			}
		}
	}
	for _, item := range slices.Sorted(maps.Keys(ore)) {
		rows = append(rows, WarehouseRow{TemplateID: item, Count: int32(min(ore[item], int64(1<<31-1)))})
	}
	return rows
}

// memberHandOff is the weight the member's next visit deposits
// (SupplyPolicy.HandOff), so the supply trip's load trigger counts the
// materials and gear it keeps.
func (b *Brain) memberHandOff() int64 {
	if !b.memberActive() {
		return 0
	}
	var w int64
	for _, r := range b.memberDeposit(b.memberOrders()) {
		w += int64(r.Count) * int64(b.cfg.Travel.weights[r.TemplateID])
	}
	return w
}

// memberFees are the deposit fees of the member's visit due or asked
// (memberState.fees), none without one: the shop visit leaves them
// (reserve). reserve runs on every tick, so this never plans the visit.
func (b *Brain) memberFees() int64 {
	return b.member.fees
}

// depositFees are the fees of depositing rows, warehouseFee a row up to
// warehouseMaxRows.
func depositFees(rows []WarehouseRow) int64 {
	return int64(warehouseFee) * int64(min(len(rows), warehouseMaxRows))
}

// memberWant is the member's warehouse visit, once a supply trip's arrival
// made it due: a withdrawal of the counts the board credits to it (Owned)
// under the board's warehouse lease, then a deposit of what it keeps for
// the clan with the ore of the shot orders it posts once the deposit is in
// (ruleMember). Without the lease, or after a visit whose withdrawal went
// unanswered, it deposits only.
func (b *Brain) memberWant(now time.Time) WarehouseRequest {
	st := &b.member
	if !st.due || st.asked {
		return WarehouseRequest{}
	}
	st.due, st.fees = false, 0
	if !b.memberActive() {
		return WarehouseRequest{}
	}
	board, clan, self := b.cfg.CraftBoard, b.world.Clan.ID, b.world.Self.ObjectID
	var req WarehouseRequest
	lease := false
	if owned := board.Owned(clan, b.world.Self.Name); len(owned) > 0 && !st.skipWithdraw {
		if lease = board.TakeLease(clan, self, now); lease {
			for _, item := range slices.Sorted(maps.Keys(owned)) {
				req.Withdraw = append(req.Withdraw, WarehouseRow{TemplateID: item, Count: int32(min(owned[item], int64(1<<31-1)))})
			}
		}
	}
	st.skipWithdraw = false
	orders := b.memberOrders()
	req.Deposit = b.memberDeposit(orders)
	if req.empty() {
		if lease {
			board.ReleaseLease(clan, self)
		}
		return WarehouseRequest{}
	}
	st.asked, st.askedAt, st.orders, st.lease = true, now, orders, lease
	st.fees = depositFees(req.Deposit)
	return req
}

// ruleMember is the member option's rule "member", after the warehouse
// errand: once the visit's request ended it settles it with the board. It
// lowers the member's credited counts by what the withdrawal moved, posts
// the shot orders whose ore the deposit moved with their materials in, and
// gives the warehouse lease back. It never takes the turn.
func (b *Brain) ruleMember(now time.Time) (Action, bool) {
	st := &b.member
	if !st.asked || b.wh.running {
		return Action{}, false
	}
	board, clan, name := b.cfg.CraftBoard, b.world.Clan.ID, b.world.Self.Name
	if r := b.wh.status.Withdraw; !r.At.Before(st.askedAt) && st.lease {
		if r.Outcome == WarehouseDone {
			for _, row := range r.Rows {
				board.Take(clan, name, row.TemplateID, int64(row.Count))
			}
		}
		st.skipWithdraw = r.Outcome == WarehouseTimeout
	}
	if st.lease {
		board.ReleaseLease(clan, b.world.Self.ObjectID)
	}
	if r := b.wh.status.Deposit; !r.At.Before(st.askedAt) && r.Outcome == WarehouseDone {
		moved := make(map[int32]int64)
		for _, row := range r.Rows {
			moved[row.TemplateID] += int64(row.Count)
		}
		for _, shot := range slices.Sorted(maps.Keys(st.orders)) {
			rc, ok := shotRecipe(b.cfg.Travel.recipes, shot)
			if !ok {
				continue
			}
			n := min(st.orders[shot], oreShots(rc, moved))
			if n <= 0 {
				continue
			}
			for item, c := range shotOre(rc, n) {
				moved[item] -= c
			}
			if id, ok := board.Post(clan, name, shot, n); ok {
				board.MaterialsIn(clan, id)
			}
		}
	}
	st.asked, st.orders, st.lease, st.fees = false, nil, false, 0
	return Action{}, false
}
