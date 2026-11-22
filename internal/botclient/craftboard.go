package botclient

import (
	"cmp"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/craftplan"
)

const (
	// craftLeaseTTL bounds how long one bot of a clan holds the warehouse
	// lease (docs/crafting.md section 3). One withdraw cycle in reach of the
	// keeper is an open, its list and one request, each reply waiting at most
	// warehouseReplyTimeout; the holder renews the lease (TakeLease again)
	// for a longer errand. A lapsed lease is free for another bot, and the
	// old holder needs it again, and a fresh list under it, to withdraw.
	craftLeaseTTL = 2 * time.Minute
	// craftClaimTTL bounds an order claim: the crafter renews it (Claim
	// again) on its tick while it works the order, resting for MP between
	// steps. A lapsed claim frees the order for another crafter.
	craftClaimTTL = 10 * time.Minute
	// craftCommandCap bounds the open leader and GM orders of a clan
	// (PostCommand): the board keeps them in memory until delivered or
	// cancelled.
	craftCommandCap = 10
	// craftHeardTTL is how long a clan chat line stays taken (Heard): the
	// crafters of a clan hear one line at once, and one answers it. A few
	// seconds cover that; the same line said again later is a new order.
	craftHeardTTL = 5 * time.Second
)

// OrderID names an order on a CraftBoard, unique in its process.
type OrderID int64

// OrderState is where an order is: posted → materials deposited → claimed
// → crafted → delivered.
type OrderState uint8

// The order states, in their order.
const (
	// OrderPosted: a member asked for the product.
	OrderPosted OrderState = iota
	// OrderMaterialsIn: the member deposited the materials; a crafter may
	// claim it.
	OrderMaterialsIn
	// OrderClaimed: a crafter holds the claim and works the order.
	OrderClaimed
	// OrderCrafted: the crafter made the products and carries them.
	OrderCrafted
	// OrderDelivered: the crafter deposited the products. A delivered order
	// leaves the board and its products count in the member's ledger.
	OrderDelivered
)

// String is the state's name, for logs and the panel.
func (s OrderState) String() string {
	switch s {
	case OrderPosted:
		return "posted"
	case OrderMaterialsIn:
		return "materials deposited"
	case OrderClaimed:
		return "claimed"
	case OrderCrafted:
		return "crafted"
	case OrderDelivered:
		return "delivered"
	}
	return "unknown"
}

// OrderSource is who placed an order.
type OrderSource uint8

// The order sources.
const (
	// OrderByMember: a member's order, crafted from the materials it
	// deposited.
	OrderByMember OrderSource = iota
	// OrderByLeader and OrderByGM: an order the clan leader (D20) or a GM
	// (D3) whispered or said in clan chat (docs/crafting.md section 8),
	// crafted from the clan's pool; a finished GM order is announced in
	// clan chat.
	OrderByLeader
	OrderByGM
)

// CraftOrder is an order: Count units of Product for Member, who placed it
// or, for a leader or GM order, whose name ordered it.
type CraftOrder struct {
	ID      OrderID
	Member  string
	Product int32
	Count   int64
	State   OrderState
	By      OrderSource
	// Crafter is the bot that claimed the order, from OrderClaimed on, and
	// ClaimUntil the claim's expiry while it is OrderClaimed. A crafted
	// order keeps its Crafter without a claim: the products are in that
	// character's inventory (its object id is the character's, the same
	// after a relog) until it delivers them.
	Crafter    int32
	ClaimUntil time.Time
}

// CrafterEntry is what a crafter of a clan publishes on the board.
type CrafterEntry struct {
	// Bot is the crafter's object id, Name its character name.
	Bot  int32
	Name string
	// Recipes are the recipe ids in its book, CreateItem its Create Item
	// level.
	Recipes    []int32
	CreateItem int32
	// ShotGrades are the crystal grades (datapack.Item.Grade) of the shots
	// it can make now: the recipe in its book and the materials or stock in
	// the warehouse.
	ShotGrades []int
}

// CraftBoard is the crafting order board of a process (docs/crafting.md
// section 3), keyed by clan id: each clan's warehouse lease, crafters,
// orders and ledger. Every bot of the process shares one; bots in another
// process (a "bots swarm" beside "bots ui") have their own and do not see
// it. It lives in memory only: after a restart the open orders are lost and
// their items count as the clan's.
//
// mu owns every clan's state. Methods hand out copies; nothing waits on the
// board: brains read it on their own tick, every lease and claim expires on
// the now its callers pass (the brain's injected clock), and ReleaseBot
// gives back what a stopping bot holds. Clan id 0 (no clan) holds nothing.
type CraftBoard struct {
	mu     sync.Mutex
	clans  map[int32]*clanBoard
	nextID OrderID
}

// clanBoard is one clan's part of a CraftBoard.
type clanBoard struct {
	// leaseBot holds the warehouse lease until leaseUntil; 0 is free.
	leaseBot   int32
	leaseUntil time.Time
	crafters   map[int32]CrafterEntry
	orders     map[OrderID]CraftOrder
	// members are the warehouse counts per item id that belong to each
	// member (delivered products); chains the intermediates each crafter
	// reserved for its chains. The rest of the warehouse is the clan's.
	members map[string]map[int32]int64
	chains  map[int32]map[int32]int64
	// heard are the clan chat lines a crafter took, by speaker and text,
	// until when (Heard).
	heard map[string]time.Time
}

// NewCraftBoard returns an empty board.
func NewCraftBoard() *CraftBoard {
	return &CraftBoard{clans: make(map[int32]*clanBoard)}
}

// clan returns clan id's board, made on first use; the caller holds mu.
func (b *CraftBoard) clan(id int32) *clanBoard {
	c, ok := b.clans[id]
	if !ok {
		c = &clanBoard{
			crafters: make(map[int32]CrafterEntry),
			orders:   make(map[OrderID]CraftOrder),
			members:  make(map[string]map[int32]int64),
			chains:   make(map[int32]map[int32]int64),
			heard:    make(map[string]time.Time),
		}
		b.clans[id] = c
	}
	return c
}

// TakeLease gives bot the warehouse lease of clan until now+craftLeaseTTL:
// when it is free, lapsed or bot's already (a renewal). False when another
// bot holds it.
func (b *CraftBoard) TakeLease(clan, bot int32, now time.Time) bool {
	if clan == 0 || bot == 0 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.clan(clan)
	if c.leaseBot != 0 && c.leaseBot != bot && now.Before(c.leaseUntil) {
		return false
	}
	c.leaseBot, c.leaseUntil = bot, now.Add(craftLeaseTTL)
	return true
}

// HoldsLease reports whether bot holds the warehouse lease of clan at now.
func (b *CraftBoard) HoldsLease(clan, bot int32, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.clans[clan]
	return ok && bot != 0 && c.leaseBot == bot && now.Before(c.leaseUntil)
}

// ReleaseLease gives back clan's warehouse lease if bot holds it.
func (b *CraftBoard) ReleaseLease(clan, bot int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c, ok := b.clans[clan]; ok && c.leaseBot == bot {
		c.leaseBot, c.leaseUntil = 0, time.Time{}
	}
}

// Publish sets e as the entry of crafter e.Bot in clan, replacing its last.
func (b *CraftBoard) Publish(clan int32, e CrafterEntry) {
	if clan == 0 || e.Bot == 0 {
		return
	}
	e.Recipes, e.ShotGrades = slices.Clone(e.Recipes), slices.Clone(e.ShotGrades)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clan(clan).crafters[e.Bot] = e
}

// Crafters returns copies of clan's crafter entries by object id.
func (b *CraftBoard) Crafters(clan int32) []CrafterEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.clans[clan]
	if !ok {
		return nil
	}
	out := make([]CrafterEntry, 0, len(c.crafters))
	for _, e := range c.crafters {
		e.Recipes, e.ShotGrades = slices.Clone(e.Recipes), slices.Clone(e.ShotGrades)
		out = append(out, e)
	}
	slices.SortFunc(out, func(x, y CrafterEntry) int { return cmp.Compare(x.Bot, y.Bot) })
	return out
}

// Post adds member's order for count units of product to clan, posted.
// False for no clan, no member or nothing to make.
func (b *CraftBoard) Post(clan int32, member string, product int32, count int64) (OrderID, bool) {
	if clan == 0 || member == "" || product <= 0 || count <= 0 {
		return 0, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	b.clan(clan).orders[b.nextID] = CraftOrder{ID: b.nextID, Member: member, Product: product, Count: count}
	return b.nextID, true
}

// PostCommand adds the order of the clan leader or a GM (by), named name,
// for count units of product to clan. It needs no member materials (the
// clan's pool pays), so it starts with them in: a crafter may claim it at
// once. False for no clan, no name, nothing to make, a member source, or
// craftCommandCap such orders open in clan.
func (b *CraftBoard) PostCommand(clan int32, by OrderSource, name string, product int32, count int64) (OrderID, bool) {
	if clan == 0 || by == OrderByMember || name == "" || product <= 0 || count <= 0 {
		return 0, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.clan(clan)
	open := 0
	for _, o := range c.orders {
		if o.By != OrderByMember {
			open++
		}
	}
	if open >= craftCommandCap {
		return 0, false
	}
	b.nextID++
	c.orders[b.nextID] = CraftOrder{ID: b.nextID, Member: name, Product: product, Count: count, State: OrderMaterialsIn, By: by}
	return b.nextID, true
}

// Cancel drops leader or GM order id of clan, claimed by bot: one its
// plan from a fresh warehouse list cannot start, the pool short of every
// material. A member order is never cancelled.
func (b *CraftBoard) Cancel(clan int32, id OrderID, bot int32) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.clans[clan]
	if !ok {
		return false
	}
	o, ok := c.orders[id]
	if !ok || o.By == OrderByMember || o.State != OrderClaimed || o.Crafter != bot || bot == 0 {
		return false
	}
	delete(c.orders, id)
	return true
}

// Heard takes the clan chat line text that speaker said in clan, at now:
// true for the first crafter of the clan to ask within craftHeardTTL, which
// answers it; false for the others, and for no clan.
func (b *CraftBoard) Heard(clan int32, speaker, text string, now time.Time) bool {
	if clan == 0 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.clan(clan)
	for k, until := range c.heard {
		if !now.Before(until) {
			delete(c.heard, k)
		}
	}
	k := speaker + "\x00" + text
	if _, ok := c.heard[k]; ok {
		return false
	}
	c.heard[k] = now.Add(craftHeardTTL)
	return true
}

// MaterialsIn moves posted order id of clan to materials deposited.
func (b *CraftBoard) MaterialsIn(clan int32, id OrderID) bool {
	return b.update(clan, id, time.Time{}, func(o *CraftOrder) bool {
		if o.State != OrderPosted {
			return false
		}
		o.State = OrderMaterialsIn
		return true
	})
}

// Claim gives bot the claim on order id of clan until now+craftClaimTTL:
// an order with its materials deposited and no live claim, or bot's own
// claim (a renewal). False otherwise.
func (b *CraftBoard) Claim(clan int32, id OrderID, bot int32, now time.Time) bool {
	if bot == 0 {
		return false
	}
	return b.update(clan, id, now, func(o *CraftOrder) bool {
		if o.State == OrderMaterialsIn || o.State == OrderClaimed && o.Crafter == bot {
			o.State, o.Crafter, o.ClaimUntil = OrderClaimed, bot, now.Add(craftClaimTTL)
			return true
		}
		return false
	})
}

// Crafted moves order id of clan, claimed by bot and the claim live at now,
// to crafted.
func (b *CraftBoard) Crafted(clan int32, id OrderID, bot int32, now time.Time) bool {
	return b.update(clan, id, now, func(o *CraftOrder) bool {
		if o.State != OrderClaimed || o.Crafter != bot || bot == 0 {
			return false
		}
		o.State, o.ClaimUntil = OrderCrafted, time.Time{}
		return true
	})
}

// Deliver closes order id of clan that bot crafted, once bot deposited
// deposited of its products: the order leaves the board and the deposited
// count, at most the order's Count (a chain crafts as much of the count as
// the pool covers, docs/crafting.md section 4), counts as its member's. It
// returns the delivered order, its Count the credited count. A negative
// deposited is refused.
func (b *CraftBoard) Deliver(clan int32, id OrderID, bot int32, deposited int64) (CraftOrder, bool) {
	if deposited < 0 {
		return CraftOrder{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.clans[clan]
	if !ok {
		return CraftOrder{}, false
	}
	o, ok := c.orders[id]
	if !ok || o.State != OrderCrafted || o.Crafter != bot || bot == 0 {
		return CraftOrder{}, false
	}
	delete(c.orders, id)
	o.Count = min(deposited, o.Count)
	addCount(c.members, o.Member, o.Product, o.Count)
	o.State = OrderDelivered
	return o, true
}

// update applies f to order id of clan under mu, once the claims lapsed at
// now (zero: none checked) are released; it keeps f's change when f
// reports true.
func (b *CraftBoard) update(clan int32, id OrderID, now time.Time, f func(*CraftOrder) bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.clans[clan]
	if !ok {
		return false
	}
	if !now.IsZero() {
		c.lapse(now)
	}
	o, ok := c.orders[id]
	if !ok || !f(&o) {
		return false
	}
	c.orders[id] = o
	return true
}

// lapse releases the claims that expired at now; the caller holds mu.
func (c *clanBoard) lapse(now time.Time) {
	for id, o := range c.orders {
		if o.State == OrderClaimed && !now.Before(o.ClaimUntil) {
			c.orders[id] = unclaimed(o)
		}
	}
}

// unclaimed is o with its claim released: back to materials deposited.
func unclaimed(o CraftOrder) CraftOrder {
	o.State, o.Crafter, o.ClaimUntil = OrderMaterialsIn, 0, time.Time{}
	return o
}

// Orders returns copies of clan's open orders in posting order, the claims
// lapsed at now released.
func (b *CraftBoard) Orders(clan int32, now time.Time) []CraftOrder {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.clans[clan]
	if !ok {
		return nil
	}
	c.lapse(now)
	return slices.SortedFunc(maps.Values(c.orders), func(x, y CraftOrder) int { return cmp.Compare(x.ID, y.ID) })
}

// PlanOrders are the orders of clan bot holds a live claim on at now, in
// posting order, as the planner reads them (craftplan.Input.Orders).
func (b *CraftBoard) PlanOrders(clan, bot int32, now time.Time) []craftplan.Order {
	var out []craftplan.Order
	for _, o := range b.Orders(clan, now) {
		if o.State == OrderClaimed && o.Crafter == bot {
			out = append(out, craftplan.Order{Product: o.Product, Count: o.Count})
		}
	}
	return out
}

// Owned returns a copy of the counts per item id in clan's warehouse that
// belong to member: what it may withdraw.
func (b *CraftBoard) Owned(clan int32, member string) map[int32]int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[int32]int64)
	if c, ok := b.clans[clan]; ok {
		maps.Copy(out, c.members[member])
	}
	return out
}

// Take lowers member's count of item in clan by the count it withdrew, at
// most to zero, and returns how many of them were its own.
func (b *CraftBoard) Take(clan int32, member string, item int32, count int64) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.clans[clan]
	if !ok || count <= 0 {
		return 0
	}
	took := min(count, c.members[member][item])
	addCount(c.members, member, item, -took)
	return took
}

// Reserve holds count of intermediate item in clan's warehouse for crafter
// bot's chains.
func (b *CraftBoard) Reserve(clan, bot, item int32, count int64) {
	if clan == 0 || bot == 0 || count <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	addCount(b.clan(clan).chains, bot, item, count)
}

// Unreserve gives back up to count of item crafter bot reserved in clan.
func (b *CraftBoard) Unreserve(clan, bot, item int32, count int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c, ok := b.clans[clan]; ok && count > 0 {
		addCount(c.chains, bot, item, -min(count, c.chains[bot][item]))
	}
}

// Reserved returns the counts per item id of clan's warehouse the planner
// of crafter must leave alone (craftplan.Input.Reserved): every member's
// counts and the chain reservations of the other crafters. Crafter's own
// reservations stay in its pool.
func (b *CraftBoard) Reserved(clan, crafter int32) map[int32]int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[int32]int64)
	c, ok := b.clans[clan]
	if !ok {
		return out
	}
	for _, counts := range c.members {
		for item, n := range counts {
			out[item] += n
		}
	}
	for bot, counts := range c.chains {
		if bot == crafter {
			continue
		}
		for item, n := range counts {
			out[item] += n
		}
	}
	return out
}

// ReleaseBot gives back everything bot holds in every clan, when it stops:
// the warehouse lease, its crafter entry, its claims (their orders back to
// materials deposited) and its chain reservations (their items the clan's).
// An order it crafted keeps it as Crafter: it carries the products.
func (b *CraftBoard) ReleaseBot(bot int32) {
	if bot == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.clans {
		if c.leaseBot == bot {
			c.leaseBot, c.leaseUntil = 0, time.Time{}
		}
		delete(c.crafters, bot)
		delete(c.chains, bot)
		for id, o := range c.orders {
			if o.State == OrderClaimed && o.Crafter == bot {
				c.orders[id] = unclaimed(o)
			}
		}
	}
}

// addCount adds n to counts[key][item], dropping counts that reach zero.
func addCount[K comparable](counts map[K]map[int32]int64, key K, item int32, n int64) {
	m := counts[key]
	if m == nil {
		if n <= 0 {
			return
		}
		m = make(map[int32]int64)
		counts[key] = m
	}
	if m[item] += n; m[item] <= 0 {
		delete(m, item)
	}
	if len(m) == 0 {
		delete(counts, key)
	}
}

// releaseCraftBoard gives back what the bot holds on the board, when the
// brain stops.
func (b *Brain) releaseCraftBoard() {
	if c := b.cfg.CraftBoard; c != nil {
		c.ReleaseBot(b.world.Self.ObjectID)
	}
}
