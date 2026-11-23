package botclient

import (
	"cmp"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/craftplan"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Skill ids of the crafts (data/xml/classes/dwarfFighter.xml): Create Item,
// the dwarven craft a Dwarven Fighter learns at its trainer from level 5,
// and Create Common Item, granted with the levels.
const (
	skillCreateItem  int32 = 172
	skillCommonCraft int32 = 1320
)

// Crafter cycle limits.
const (
	// recipeBookRoom is the room of each recipe book page before stat
	// bonuses (craft/craft.go: 50 dwarven, 50 common): the planner's
	// lower bound.
	recipeBookRoom = 50
	// craftReplyTimeout is how long a craft waits for its
	// RecipeItemMakeInfo or its combat refusal. The server drops a craft
	// inside ManufactureDelay in silence (client_ingame.go
	// handleRequestRecipeItemMakeSelf); craftGap keeps clear of it, so an
	// unanswered craft is a timeout of the breaker's.
	craftReplyTimeout = 5 * time.Second
	// craftJitter is the most added to craftGap between two crafts
	// (docs/crafting.md, "Pacing": 500 ms plus jitter).
	craftJitter = 250 * time.Millisecond
	// crafterPhaseTimeout bounds a warehouse step of the cycle (the errand
	// never started: no keeper, no lease); past it the cycle ends.
	crafterPhaseTimeout = 10 * time.Minute
	// crafterWeightPercent is the weight percent at which the cycle
	// deposits before crafting on (docs/crafting.md section 5: before
	// 50 %).
	crafterWeightPercent = 50
)

// CrafterPolicy is the clan crafter's cycle (rule "craft", docs/crafting.md
// section 5): check the clan warehouse, plan (craftplan), take the board's
// lease, withdraw, release it, craft, deposit. The zero CrafterPolicy
// crafts nothing; a brain crafts only with a craft board and a travel map
// (BrainConfig.CraftBoard, Travel), whose recipes and prices the planner
// reads.
type CrafterPolicy struct {
	// CheckEvery is the time from the end of a cycle that crafted to the
	// next check, plus up to CheckJitter; ReturnEvery replaces it after a
	// cycle with nothing to craft, while the bot farms on its spot. A
	// check comes sooner when the board holds an order with its materials
	// in that the bot can make. Opens stay MinOpenGap (warehouseOpenGap)
	// apart whatever these say.
	CheckEvery, CheckJitter time.Duration
	ReturnEvery             time.Duration
	// FailStreak is how many craft results in a row that are neither a
	// success nor a failed roll (a missing material, short MP, the combat
	// refusal) stop the cycle, and TimeoutStreak how many unanswered
	// crafts in a row; a stopped cycle deposits, then holds the next check
	// for BreakerHold.
	FailStreak, TimeoutStreak int
	BreakerHold               time.Duration
	// Plan are the planner's knobs.
	Plan craftplan.Config
}

func defaultCrafterPolicy() CrafterPolicy {
	return CrafterPolicy{
		CheckEvery: 5 * time.Minute, CheckJitter: 2 * time.Minute, ReturnEvery: 30 * time.Minute,
		FailStreak: 5, TimeoutStreak: 3, BreakerHold: 30 * time.Minute,
		Plan: craftplan.DefaultConfig(),
	}
}

// dwarvenTrainers are the NPC template ids whose teachTo lists the Dwarven
// Fighter (class 53) and whose first page links the skill list
// (data/xml/npcs, data/html/warehouse and data/html/trainer): the
// warehouse keepers and blacksmiths. The Trader 30516, Warehouse Chief
// 30520 and Gatekeeper 30320 teach it too but link no skill list.
var dwarvenTrainers = []int32{
	30057, 30058, 30103, 30104, 30182, 30183, 30210, 30232, 30255, 30298, 30300, 30316,
	30317, 30322, 30458, 30471, 30521, 30522, 30526, 30527, 30678, 30686, 30688, 30843,
	30844, 30846, 30895, 30896, 30898, 31267, 31268, 31270, 31271, 31311, 31312, 31313,
	31315, 31316, 31583, 31956, 31957, 31959, 31960, 31990,
}

// artisanTrainers are the dwarvenTrainers whose teachTo lists the Artisan
// (class 56): the blacksmiths and warehouse keepers of the crafter line,
// not the fighter-only ones. The Trader 30516 teaches it too but links no
// skill list, as for the Dwarven Fighter.
var artisanTrainers = []int32{
	30298, 30300, 30317, 30458, 30471, 30526, 30527, 30678, 30688, 30846, 30898, 31267,
	31268, 31270, 31271, 31311, 31312, 31313, 31315, 31316, 31583, 31960, 31990,
}

// warsmithTrainers are the dwarvenTrainers whose teachTo lists the Warsmith
// (class 57): the Artisan's list without 31990, which teaches 53, 56 and
// 118 only.
var warsmithTrainers = []int32{
	30298, 30300, 30317, 30458, 30471, 30526, 30527, 30678, 30688, 30846, 30898, 31267,
	31268, 31270, 31271, 31311, 31312, 31313, 31315, 31316, 31583, 31960,
}

// CrafterPersona is the clan crafter (docs/crafting.md section 5), played
// by a Dwarven Fighter: DefaultPersona plus the crafter's cycle, Create
// Item learned at the dwarven trainers, and the clan invitation of any
// player while clanless, or only of a name of inviters when it names some
// (D20, AcceptClanFrom). With nothing to craft it farms the spot registry
// the preset gives it.
func CrafterPersona(inviters ...string) Persona {
	p := DefaultPersona()
	p.Crafter = defaultCrafterPolicy()
	p.JoinClan = AcceptClanFrom(inviters...)
	// The crafter line, Artisan at 20 and Warsmith at 40 (D18), learns
	// Create Item at the trainers whose teachTo lists its class.
	p.ClassChange.Builds[DwarvenFighter.ID] = classArtisan
	p.ClassChange.Builds[classArtisan] = classWarsmith
	for c, trainers := range map[int32][]int32{
		DwarvenFighter.ID: dwarvenTrainers,
		classArtisan:      artisanTrainers,
		classWarsmith:     warsmithTrainers,
	} {
		p.LearnSkills[c] = []int32{skillCreateItem}
		p.Trainers[c] = trainers
	}
	return p
}

// crafterPhase is where the crafter's cycle stands.
type crafterPhase uint8

const (
	// craftIdle: no cycle; the next check is due at nextCheck.
	craftIdle crafterPhase = iota
	// craftLook: the check's look at the warehouse (WarehouseRequest.Look).
	craftLook
	// craftWithdraw: the plan's withdrawal under the lease, then the
	// deposit of what no plan needs.
	craftWithdraw
	// craftMake: crafting the plan's steps.
	craftMake
	// craftDeposit: the deposit of the weight guard or of the cycle's end.
	craftDeposit
)

// crafterCycle is the crafter's cycle (rule "craft"). It is owned by the
// brain goroutine.
type crafterCycle struct {
	phase crafterPhase
	// req is the warehouse request of the phase, asked once
	// (warehouseWant), askedAt when; deadline is when the phase gives up.
	req      WarehouseRequest
	asked    bool
	askedAt  time.Time
	deadline time.Time
	// nextCheck is when the next check is due; zero checks at once.
	nextCheck time.Time
	// bookAt is when the last recipe book open went out.
	bookAt time.Time
	// plan is the cycle's plan; orders the board orders it serves, by the
	// index the plan's chains name.
	plan   craftplan.Plan
	orders []CraftOrder
	// learn are the plan's recipe scrolls not handed to the recipe errand
	// yet.
	learn []int32
	// chain and step are the step crafting, done its crafts answered
	// (successes and failed rolls).
	chain, step int
	done        int64
	// pending is set from a craft's send to its answer; sentAt when it
	// went out, failure the system message ahead of its answer, nextAt
	// when the next craft may go out.
	pending bool
	sentAt  time.Time
	failure CraftFailure
	nextAt  time.Time
	// fails and timeouts are the breaker's streaks; tripped is set once
	// one stopped the cycle.
	fails, timeouts int
	tripped         bool
	// replan re-plans from the inventory before the next craft.
	replan bool
	// resume returns to crafting after the weight guard's deposit; made is
	// set by a craft since the last guard.
	resume, made bool
	// crafted is set once a chain was crafted this cycle.
	crafted bool
	// crystal is the cycle's crystallizing, ahead of its crafts.
	crystal crystalStep
	// delivering are the products of crafted orders the deposit under way
	// carries, by order.
	delivering map[OrderID]int64
	// ended is how the last cycle ended; it survives the cycle's reset.
	ended craftEnded
}

// begin starts phase ph with warehouse request req at now.
func (c *crafterCycle) begin(ph crafterPhase, req WarehouseRequest, now time.Time) {
	c.phase, c.req, c.asked, c.askedAt = ph, req, false, time.Time{}
	c.deadline = now.Add(crafterPhaseTimeout)
}

// crafting reports whether the brain plays the crafter's cycle.
func (b *Brain) crafting() bool {
	return b.persona.Crafter.CheckEvery > 0 && b.cfg.CraftBoard != nil && b.cfg.Travel != nil && b.cfg.Travel.recipes != nil
}

// warehouseWant is the warehouse request the errand starts: the crafter's
// cycle owns the warehouse while it runs, else Persona.Warehouse.Want.
func (b *Brain) warehouseWant(now time.Time) WarehouseRequest {
	if b.crafting() && b.craft.phase != craftIdle {
		return b.crafterWant(now)
	}
	if want := b.persona.Warehouse.Want; want != nil {
		return want(b.world, b.wh.status, now)
	}
	return WarehouseRequest{}
}

// crafterWant hands the phase's request to the errand once. A withdrawal
// waits for the board's warehouse lease (CraftBoard.TakeLease).
func (b *Brain) crafterWant(now time.Time) WarehouseRequest {
	c := &b.craft
	if c.asked || c.req.empty() {
		return WarehouseRequest{}
	}
	if c.phase == craftWithdraw && !b.cfg.CraftBoard.TakeLease(b.world.Clan.ID, b.world.Self.ObjectID, now) {
		return WarehouseRequest{}
	}
	c.asked, c.askedAt = true, now
	return c.req
}

// ruleCraft is the crafter's rule "craft" (docs/crafting.md section 5),
// after the warehouse and recipe errands that run its requests: out of
// combat, a check every CheckEvery (sooner for an order with its materials
// in) looks at the clan warehouse, plans, withdraws under the board's
// lease, crafts the plan's steps, and deposits the products and leftovers
// no plan needs, before the weight passes 50 % and at the end. It holds
// the turn while a cycle runs, so the bot farms only between cycles.
//
// It sends RequestRecipeItemMakeSelf, one per answer (RecipeItemMakeInfo,
// the combat refusal or craftReplyTimeout), at least craftGap plus up to
// craftJitter apart (gateCraft keeps craftGap, over the server's silent
// 300 ms ManufactureDelay), and a RequestRecipeBookOpen per unknown page,
// one per recipeReplyTimeout. The warehouse errand paces its opens
// (MinOpenGap) and bypasses.
func (b *Brain) ruleCraft(now time.Time) (Action, bool) {
	if !b.crafting() || b.world.Self.InCombat || b.world.Self.Dead {
		return Action{}, false
	}
	c := &b.craft
	// A craft step bounds itself (craftReplyTimeout, a rest ends): the
	// deadline bounds the warehouse steps.
	if c.phase != craftIdle && (b.world.Clan.ID == 0 || c.phase != craftMake && !now.Before(c.deadline)) {
		b.endCraftCycle(now, false)
		return Action{}, false
	}
	b.renewCraftClaims(now)
	switch c.phase {
	case craftIdle:
		if b.world.Clan.ID == 0 || !b.craftCheckDue(now) {
			return Action{}, false
		}
		if a, ok := b.openBook(now); ok {
			return a, true
		}
		c.begin(craftLook, WarehouseRequest{Look: true}, now)
		return Action{}, true
	case craftMake:
		return b.craftNext(now)
	}
	if !c.asked || b.wh.running {
		return Action{}, true
	}
	b.craftErrandDone(now)
	if c.phase == craftMake {
		return b.craftNext(now)
	}
	return Action{}, true
}

// openBook opens a page of the recipe book the bot does not know yet, one
// per recipeReplyTimeout: the planner plans only recipes in the book.
func (b *Brain) openBook(now time.Time) (Action, bool) {
	book, c := &b.world.Recipes, &b.craft
	if book.DwarvenKnown && book.CommonKnown || now.Before(c.bookAt.Add(recipeReplyTimeout)) && !c.bookAt.IsZero() {
		return Action{}, false
	}
	c.bookAt = now
	return recipeBookAction(!book.DwarvenKnown), true
}

// craftCheckDue reports whether a check is due: nextCheck passed, or the
// board holds an order with its materials in that the bot can make. A
// breaker's hold holds every check, that order's too.
func (b *Brain) craftCheckDue(now time.Time) bool {
	if now.Before(b.craft.ended.holdUntil) {
		return false
	}
	if !now.Before(b.craft.nextCheck) {
		return true
	}
	for _, o := range b.cfg.CraftBoard.Orders(b.world.Clan.ID, now) {
		if o.State == OrderMaterialsIn && b.canMake(o.Product) {
			return true
		}
	}
	return false
}

// canMake reports whether a recipe in the bot's book makes product.
func (b *Brain) canMake(product int32) bool {
	for _, id := range slices.Concat(b.world.Recipes.Dwarven, b.world.Recipes.Common) {
		if r, ok := b.cfg.Travel.recipes[id]; ok && r.Product.ItemID == product {
			return true
		}
	}
	return false
}

// craftErrandDone moves the cycle on once its warehouse request ended.
func (b *Brain) craftErrandDone(now time.Time) {
	c, st := &b.craft, &b.wh.status
	switch c.phase {
	case craftLook:
		r := st.Withdraw
		if r.At.Before(c.askedAt) || r.Outcome != WarehouseListed && r.Outcome != WarehouseEmpty {
			b.endCraftCycle(now, false)
			return
		}
		b.craftPlan(now, true)
	case craftWithdraw:
		b.cfg.CraftBoard.ReleaseLease(b.world.Clan.ID, b.world.Self.ObjectID)
		// The deposit after the withdrawal can carry the products of
		// orders an earlier cycle's failed deposit left crafted.
		if r := st.Deposit; !r.At.Before(c.askedAt) && r.Outcome == WarehouseDone {
			b.deliverOrders(now)
		}
		c.delivering = nil
		if r := st.Withdraw; r.At.Before(c.askedAt) || r.Outcome != WarehouseDone {
			// Nothing came: plan again from what the bot carries.
			b.craftPlan(now, false)
			return
		}
		b.startMake(now)
	case craftDeposit:
		if r := st.Deposit; !r.At.Before(c.askedAt) && r.Outcome == WarehouseDone {
			b.deliverOrders(now)
		}
		c.delivering = nil
		if c.resume {
			c.resume, c.made = false, false
			c.phase, c.deadline = craftMake, now.Add(crafterPhaseTimeout)
			return
		}
		b.endCraftCycle(now, true)
	}
}

// craftPlan plans the cycle from the fresh warehouse list (fromWarehouse)
// or from the carried stock alone, claims the board orders it can make and
// publishes the bot's crafter entry, then withdraws or crafts. From the
// list it picks the gear to crystallize too (planCrystallize), withdrawn
// with the plan's materials, its crystals planned with as carried.
func (b *Brain) craftPlan(now time.Time, fromWarehouse bool) {
	c := &b.craft
	c.orders = b.claimCraftOrders(now)
	if fromWarehouse {
		b.planCrystallize()
	}
	in := b.craftInput(c.orders, fromWarehouse, now)
	c.plan = craftplan.Build(in, b.persona.Crafter.Plan)
	if fromWarehouse && !c.plan.Exhausted {
		b.cancelUnplanned()
	}
	c.learn = slices.Clone(c.plan.Learn)
	b.cfg.CraftBoard.Publish(b.world.Clan.ID, CrafterEntry{
		Bot: b.world.Self.ObjectID, Name: b.world.Self.Name, Recipes: in.Book, CreateItem: in.DwarvenLevel,
		ShotGrades: b.shotGradesNow(in),
	})
	if fromWarehouse && (len(c.plan.Withdraw) > 0 || len(c.crystal.want) > 0) {
		rows := make([]WarehouseRow, 0, len(c.plan.Withdraw)+len(c.crystal.want))
		for _, g := range c.plan.Withdraw {
			rows = append(rows, WarehouseRow{TemplateID: g.ItemID, Count: g.Count})
		}
		rows = append(rows, c.crystal.rows()...)
		deposit := b.craftDepositRows(ingredientCounts(c.plan.Keep), now)
		b.noteDelivering(deposit, now)
		c.begin(craftWithdraw, WarehouseRequest{Withdraw: rows, Deposit: deposit}, now)
		return
	}
	b.startMake(now)
}

// claimCraftOrders claims (or renews) the board orders with their
// materials in whose product a recipe in the book makes, and returns the
// orders the bot holds a live claim on: the members' in posting order, then
// the clan leader's and GMs' (docs/crafting.md section 4).
func (b *Brain) claimCraftOrders(now time.Time) []CraftOrder {
	board, clan, self := b.cfg.CraftBoard, b.world.Clan.ID, b.world.Self.ObjectID
	var mine, commanded []CraftOrder
	for _, o := range board.Orders(clan, now) {
		switch {
		case o.State == OrderMaterialsIn && b.canMake(o.Product), o.State == OrderClaimed && o.Crafter == self:
			if !board.Claim(clan, o.ID, self, now) {
				continue
			}
			if o.By == OrderByMember {
				mine = append(mine, o)
			} else {
				commanded = append(commanded, o)
			}
		}
	}
	return append(mine, commanded...)
}

// renewCraftClaims renews the claims of the cycle's orders on the tick.
func (b *Brain) renewCraftClaims(now time.Time) {
	c := &b.craft
	if c.phase == craftIdle {
		return
	}
	for _, o := range c.orders {
		b.cfg.CraftBoard.Claim(b.world.Clan.ID, o.ID, b.world.Self.ObjectID, now)
	}
}

// craftInput is the planner's input: the carried crafting stock less the
// products of the orders the bot crafted, the fresh warehouse list when
// fromWarehouse, the board's reservations and orders, the recipes and
// reference prices, the book and the craft levels; the crystals the
// cycle's crystallizing will give count as carried (crystalsDue).
func (b *Brain) craftInput(orders []CraftOrder, fromWarehouse bool, now time.Time) craftplan.Input {
	m, clan, self := b.cfg.Travel, b.world.Clan.ID, b.world.Self.ObjectID
	inv := b.craftStock()
	for _, o := range b.cfg.CraftBoard.Orders(clan, now) {
		if o.State == OrderCrafted && o.Crafter == self {
			inv[o.Product] = max(inv[o.Product]-o.Count, 0)
		}
	}
	in := craftplan.Input{
		Inventory: inv,
		Reserved:  b.cfg.CraftBoard.Reserved(clan, self),
		Recipes:   m.recipes,
		Prices:    m.prices,
		Book:      slices.Concat(b.world.Recipes.Dwarven, b.world.Recipes.Common),
		// A skill not known has level 0: no such craft.
		DwarvenLevel: b.world.Skills[skillCreateItem].Level,
		CommonLevel:  b.world.Skills[skillCommonCraft].Level,
		DwarvenLimit: recipeBookRoom, CommonLimit: recipeBookRoom,
		MaxMP: b.world.Self.MaxMP,
	}
	if fromWarehouse {
		in.Warehouse = make(map[int32]int64)
		for _, it := range b.wh.status.Contents {
			in.Warehouse[it.TemplateID] += int64(it.Count)
		}
	}
	b.crystalsDue(inv, in.Warehouse, fromWarehouse)
	for _, o := range orders {
		in.Orders = append(in.Orders, craftplan.Order{Product: o.Product, Count: o.Count})
	}
	return in
}

// craftStock is the bot's crafting stock by template: its unequipped items
// a recipe uses or makes, less adena, every shot, its healing potions and
// Scrolls of Escape, which are its own supplies (craftplan.Input.Inventory).
func (b *Brain) craftStock() map[int32]int64 {
	m, p := b.cfg.Travel, &b.persona
	stock := make(map[int32]int64)
	for _, it := range b.world.Inventory.Items {
		t := it.TemplateID
		_, shot := m.shots[t]
		if it.Equipped || !m.craftItems[t] || t == adenaID || t == scrollOfEscape || shot ||
			slices.Contains(p.Shots, t) || slices.Contains(p.Potions, t) {
			continue
		}
		stock[t] += int64(it.Count)
	}
	return stock
}

// craftDepositRows are the deposit rows of the crafting stock beyond keep,
// of the gear withdrawn to crystallize and left whole (crystalLeftovers),
// and of the bot's crafted orders whose product the stock leaves out as
// its own supplies (shots, potions, Scrolls of Escape), as many as it
// carries, by template.
func (b *Brain) craftDepositRows(keep map[int32]int64, now time.Time) []WarehouseRow {
	rows := b.crystalLeftovers()
	stock := b.craftStock()
	for t, n := range stock {
		if left := n - keep[t]; left > 0 {
			rows = append(rows, WarehouseRow{TemplateID: t, Count: int32(min(left, int64(1<<31-1)))})
		}
	}
	ordered := make(map[int32]int64)
	for _, o := range b.cfg.CraftBoard.Orders(b.world.Clan.ID, now) {
		if _, inStock := stock[o.Product]; o.State == OrderCrafted && o.Crafter == b.world.Self.ObjectID && !inStock {
			ordered[o.Product] += o.Count
		}
	}
	for t, n := range ordered {
		var carried int64
		for _, it := range b.world.Inventory.Items {
			if it.TemplateID == t && !it.Equipped {
				carried += int64(it.Count)
			}
		}
		if n = min(n, carried); n > 0 {
			rows = append(rows, WarehouseRow{TemplateID: t, Count: int32(min(n, int64(1<<31-1)))})
		}
	}
	slices.SortFunc(rows, func(a, b WarehouseRow) int {
		return cmp.Or(cmp.Compare(a.TemplateID, b.TemplateID), cmp.Compare(a.Object, b.Object))
	})
	return rows
}

// indexCraftItems is every template a recipe of recipes uses or makes, its
// scroll included.
func indexCraftItems(recipes map[int32]datapack.Recipe) map[int32]bool {
	items := make(map[int32]bool)
	for _, r := range recipes {
		items[r.ScrollID], items[r.Product.ItemID] = true, true
		for _, g := range r.Materials {
			items[g.ItemID] = true
		}
	}
	return items
}

// ingredientCounts sums gs by item.
func ingredientCounts(gs []datapack.Ingredient) map[int32]int64 {
	out := make(map[int32]int64, len(gs))
	for _, g := range gs {
		out[g.ItemID] += int64(g.Count)
	}
	return out
}

// startMake starts crystallizing the withdrawn gear, then crafting the
// plan's first step.
func (b *Brain) startMake(now time.Time) {
	c := &b.craft
	c.phase, c.req, c.deadline = craftMake, WarehouseRequest{}, now.Add(crafterPhaseTimeout)
	c.chain, c.step, c.done, c.pending, c.replan = 0, 0, 0, false, false
	b.notePoolGear()
	b.craftAdvance(now)
	if _, ok := c.current(); !ok && len(b.crystalTargets()) == 0 {
		b.finishCraftCycle(now)
	}
}

// current is the step crafting, false once the plan is done.
func (c *crafterCycle) current() (craftplan.Step, bool) {
	if c.chain >= len(c.plan.Chains) {
		return craftplan.Step{}, false
	}
	return c.plan.Chains[c.chain].Steps[c.step], true
}

// craftAdvance moves past finished steps and chains to the next craft: a
// finished chain for an order moves the order to crafted on the board.
func (b *Brain) craftAdvance(now time.Time) {
	c := &b.craft
	for c.chain < len(c.plan.Chains) {
		ch := c.plan.Chains[c.chain]
		if c.step < len(ch.Steps) {
			if c.done < ch.Steps[c.step].Times {
				return
			}
			c.step, c.done = c.step+1, 0
			continue
		}
		if ch.Order >= 0 && ch.Order < len(c.orders) {
			b.cfg.CraftBoard.Crafted(b.world.Clan.ID, c.orders[ch.Order].ID, b.world.Self.ObjectID, now)
		}
		c.crafted = true
		c.chain, c.step, c.done = c.chain+1, 0, 0
	}
}
