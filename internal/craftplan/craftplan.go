// Package craftplan is the clan crafter's craft planner (docs/crafting.md,
// section 4): a pure function from a fresh clan warehouse list, the
// crafter's inventory, the recipes, its recipe book and Create Item levels,
// the board's orders and reservations, to what to withdraw, which recipe
// scrolls to learn, the crafts to run and what to keep. It sends nothing
// and keeps no state: the crafter re-plans every cycle from fresh lists.
package craftplan

import (
	"cmp"
	"maps"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// adenaID is the adena template: never taken (D19).
const adenaID int32 = 57

// maxDepth bounds a chain's levels. The shipped chains are at most 5 deep
// and the recipes reader rejects a cycle; the bound only keeps a datapack
// edit from recursing far.
const maxDepth = 8

// Order is a craft the board asks for: Count units of Product, crafted
// whatever the margin. Its chain may use items without a reference price
// (D19: an order names them).
type Order struct {
	Product int32
	Count   int64
}

// Input is everything a plan reads. Counts are per item template.
type Input struct {
	// Warehouse is the clan warehouse as its last fresh list showed it.
	Warehouse map[int32]int64
	// Inventory is what the crafter carries. A plan takes from it before
	// the warehouse, so leftovers of a past cycle are used first.
	Inventory map[int32]int64
	// Reserved are the warehouse counts the board holds for members
	// (products delivered to or reserved for them): never in the pool, so
	// the compressed shot packs never eat a member's shots.
	Reserved map[int32]int64
	// Recipes are every recipe by id (datapack.Datapack.Recipes).
	Recipes map[int32]datapack.Recipe
	// Prices are the datapack reference prices (datapack.Datapack.Prices);
	// an item absent from it has no reference price.
	Prices map[int32]int32
	// Demand is a value per unit added to the reference price, for what
	// the clan uses (shots and arrows of the members' grades, the board's
	// order products).
	Demand map[int32]int64
	// Book is the recipe ids in the crafter's book, both pages.
	Book []int32
	// DwarvenLevel is the Create Item level and CommonLevel the Common
	// Craft level; 0 means the crafter has no such craft.
	DwarvenLevel, CommonLevel int32
	// DwarvenLimit and CommonLimit are each page's room (50 plus stat
	// bonuses on the server).
	DwarvenLimit, CommonLimit int
	// MaxMP is the crafter's max MP: a recipe costing more is never
	// crafted. 0 is unknown and limits nothing.
	MaxMP int32
	// Orders are the board's member orders, then the leader's and GMs'
	// orders, in the order they are served.
	Orders []Order
}

// Config are the planner's knobs.
type Config struct {
	// MinGain is the margin, in adena of value, a chain for the clan must
	// exceed to be crafted.
	MinGain int64
	// MaxChains is how many chains for the clan one plan holds (orders
	// are not counted).
	MaxChains int
	// MaxWork is how many nodes (an item resolved for a chain) one plan
	// may visit; past it the plan stops with what it holds.
	MaxWork int
}

// DefaultConfig is any positive margin, 20 chains and 200000 nodes a plan.
func DefaultConfig() Config {
	return Config{MinGain: 0, MaxChains: 20, MaxWork: 200000}
}

// Step is one recipe crafted Times times in a row.
type Step struct {
	Recipe int32
	Times  int64
}

// Chain is one end product and every step under it, bottom-up.
type Chain struct {
	// Product is the end product and Count what the top step makes when
	// every roll succeeds.
	Product int32
	Count   int64
	// Order is the index in Input.Orders the chain serves, -1 for a chain
	// for the clan.
	Order int
	// Margin is the end product's value minus the expected value used up.
	Margin int64
	// Steps run in order: every step's inputs are made by steps before
	// it or taken from the pool.
	Steps []Step
}

// Plan is what the crafter does in one cycle.
type Plan struct {
	// Learn are the recipe scroll templates to use, one copy each,
	// before any chain; a copy not carried is in Withdraw.
	Learn []int32
	// Withdraw is what to take from the warehouse, by item id.
	Withdraw []datapack.Ingredient
	// Keep is what the crafter holds and must not deposit: the inventory
	// the plan uses and the intermediates its steps make beyond need.
	Keep []datapack.Ingredient
	// Chains run in order: orders first, then the clan's.
	Chains []Chain
	// Work is the nodes the plan visited.
	Work int
	// Exhausted reports the plan stopped at Config.MaxWork.
	Exhausted bool
}

// Build plans one cycle: the orders, then one copy of every recipe scroll
// the crafter can register now, then the chains for the clan, best margin
// per value used first, while one is worth it and up to MaxChains. Only
// recipes in the book at plan time are crafted: a scroll learned now
// counts from the next plan, after the server answered.
func Build(in Input, cfg Config) Plan {
	p := newPlanner(in, cfg)
	var plan Plan
	for i, o := range in.Orders {
		if p.exhausted {
			break
		}
		if c, ok := p.order(o); ok {
			c.Order = i
			plan.Chains = append(plan.Chains, c)
		}
	}
	if !p.exhausted {
		plan.Learn = p.learn()
	}
	for free := 0; free < cfg.MaxChains && !p.exhausted; free++ {
		c, ok := p.best()
		if !ok {
			break
		}
		plan.Chains = append(plan.Chains, c)
	}
	plan.Withdraw = ingredients(p.pool.whUsed)
	keep := maps.Clone(p.pool.invUsed)
	for id, n := range p.pool.made {
		keep[id] += n
	}
	plan.Keep = ingredients(keep)
	plan.Work, plan.Exhausted = p.work, p.exhausted
	return plan
}

// planner is one Build's state.
type planner struct {
	in  Input
	cfg Config
	// makers maps a product to the recipes the crafter can craft now that
	// make it, by id.
	makers map[int32][]datapack.Recipe
	// tops are the recipes the crafter can craft now, by id.
	tops      []datapack.Recipe
	book      map[int32]bool
	pool      pool
	work      int
	exhausted bool
}

func newPlanner(in Input, cfg Config) *planner {
	p := &planner{in: in, cfg: cfg, makers: map[int32][]datapack.Recipe{}, book: map[int32]bool{}}
	for _, id := range in.Book {
		p.book[id] = true
	}
	for _, id := range slices.Sorted(maps.Keys(in.Recipes)) {
		r := in.Recipes[id]
		if p.craftable(r) {
			p.tops = append(p.tops, r)
			p.makers[r.Product.ItemID] = append(p.makers[r.Product.ItemID], r)
		}
	}
	p.pool = pool{
		made: map[int32]int64{}, inv: map[int32]int64{}, wh: map[int32]int64{},
		invUsed: map[int32]int64{}, whUsed: map[int32]int64{},
	}
	for id, n := range in.Inventory {
		if n > 0 && id != adenaID {
			p.pool.inv[id] = n
		}
	}
	for id, n := range in.Warehouse {
		if n -= in.Reserved[id]; n > 0 && id != adenaID {
			p.pool.wh[id] = n
		}
	}
	return p
}

// craftable reports whether the crafter can craft r now: in its book, at
// or under its page's craft level, within its max MP.
func (p *planner) craftable(r datapack.Recipe) bool {
	if !p.book[r.ID] || r.SuccessRate <= 0 || r.Product.Count <= 0 {
		return false
	}
	if p.in.MaxMP > 0 && r.MPCost > p.in.MaxMP {
		return false
	}
	return r.Level <= p.level(r.Dwarven) && p.level(r.Dwarven) > 0
}

func (p *planner) level(dwarven bool) int32 {
	if dwarven {
		return p.in.DwarvenLevel
	}
	return p.in.CommonLevel
}

// value is one unit of item's worth: its reference price plus its demand.
func (p *planner) value(item int32) int64 {
	return int64(p.in.Prices[item]) + p.in.Demand[item]
}

// learn takes one copy of every recipe scroll in the pool whose recipe the
// crafter can register now (craft ability, level, room on its page, not in
// the book), by recipe id, as the server's Register checks.
func (p *planner) learn() []int32 {
	room := map[bool]int{true: p.in.DwarvenLimit, false: p.in.CommonLimit}
	for id := range p.book {
		if r, ok := p.in.Recipes[id]; ok {
			room[r.Dwarven]--
		}
	}
	var out []int32
	seen := map[int32]bool{}
	for _, id := range slices.Sorted(maps.Keys(p.in.Recipes)) {
		r := p.in.Recipes[id]
		lvl := p.level(r.Dwarven)
		if p.book[id] || seen[r.ScrollID] || lvl <= 0 || r.Level > lvl || room[r.Dwarven] <= 0 {
			continue
		}
		if p.pool.take(r.ScrollID, 1, true) == 1 {
			seen[r.ScrollID] = true
			room[r.Dwarven]--
			out = append(out, r.ScrollID)
		}
	}
	return out
}

// eval is one evaluated chain.
type eval struct {
	recipe datapack.Recipe
	chain  Chain
	// used is the expected value the chain uses up.
	used int64
}

// better ranks a over b: the best margin per value used, then fewer
// steps, then the surer recipe (the 100 % variant first), then the lower
// recipe id.
func better(a, b eval) bool {
	ra, rb := ratio(a), ratio(b)
	if ra != rb {
		return ra > rb
	}
	if len(a.chain.Steps) != len(b.chain.Steps) {
		return len(a.chain.Steps) < len(b.chain.Steps)
	}
	if a.recipe.SuccessRate != b.recipe.SuccessRate {
		return a.recipe.SuccessRate > b.recipe.SuccessRate
	}
	return a.recipe.ID < b.recipe.ID
}

func ratio(e eval) float64 {
	if e.used <= 0 {
		return float64(e.chain.Margin) * 1e18
	}
	return float64(e.chain.Margin) / float64(e.used)
}

// best picks and takes the clan chain worth it with the best margin per
// value used, each top recipe crafted once.
func (p *planner) best() (Chain, bool) {
	var top eval
	found := false
	for _, r := range p.tops {
		mark := p.pool.mark()
		e, ok := p.chain(r, 1, false)
		p.pool.rollback(mark)
		if p.exhausted {
			return Chain{}, false
		}
		if ok && e.chain.Margin > p.cfg.MinGain && (!found || better(e, top)) {
			top, found = e, true
		}
	}
	if !found {
		return Chain{}, false
	}
	mark := p.pool.mark()
	e, ok := p.chain(top.recipe, 1, false)
	if !ok {
		p.pool.rollback(mark)
		return Chain{}, false
	}
	e.chain.Order = -1
	return e.chain, true
}

// order plans o: the variant that makes the most of its count, then the
// lowest expected cost per unit, then better's order. A count the pool
// does not cover is cut to what it covers.
func (p *planner) order(o Order) (Chain, bool) {
	var top eval
	found := false
	for _, r := range p.makers[o.Product] {
		full := ceilDiv(o.Count, int64(r.Product.Count))
		times := p.coverable(r, full)
		if times == 0 {
			continue
		}
		mark := p.pool.mark()
		e, ok := p.chain(r, times, true)
		p.pool.rollback(mark)
		if !ok {
			continue
		}
		if !found || orderBetter(e, top) {
			top, found = e, true
		}
	}
	if !found || p.exhausted {
		return Chain{}, false
	}
	mark := p.pool.mark()
	e, ok := p.chain(top.recipe, top.chain.Steps[len(top.chain.Steps)-1].Times, true)
	if !ok {
		p.pool.rollback(mark)
	}
	return e.chain, ok
}

func orderBetter(a, b eval) bool {
	if a.chain.Count != b.chain.Count {
		return a.chain.Count > b.chain.Count
	}
	if a.used != b.used {
		return a.used < b.used
	}
	return better(a, b)
}

// coverable is the most top crafts of r, up to full, the pool covers: a
// binary search, since a chain of more crafts needs at least as much.
func (p *planner) coverable(r datapack.Recipe, full int64) int64 {
	lo, hi := int64(0), full
	for lo < hi && !p.exhausted {
		mid := lo + (hi-lo+1)/2
		mark := p.pool.mark()
		_, ok := p.chain(r, mid, true)
		p.pool.rollback(mark)
		if ok {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// chain takes from the pool what times crafts of top recipe r need, crafting
// the rest from below. open lets it use items without a reference price
// (an order's chain). The caller rolls the pool back on failure.
func (p *planner) chain(r datapack.Recipe, times int64, open bool) (eval, bool) {
	var used int64
	var steps []Step
	for _, m := range r.Materials {
		c, s, ok := p.need(m.ItemID, int64(m.Count)*times, open, 1)
		if !ok {
			return eval{}, false
		}
		used += c
		steps = append(steps, s...)
	}
	// Only the top step may fail; a failed roll is crafted again, so its
	// inputs are expected to be spent 100/rate times.
	used = ceilDiv(used*100, int64(r.SuccessRate))
	count := times * int64(r.Product.Count)
	steps = append(steps, Step{Recipe: r.ID, Times: times})
	return eval{recipe: r, used: used, chain: Chain{
		Product: r.Product.ItemID, Count: count, Margin: count*p.value(r.Product.ItemID) - used, Steps: steps,
	}}, true
}

// need resolves n units of item for a chain: what the pool holds, then the
// rest crafted by a 100 % recipe (the shipped intermediates all are). It
// returns the value used up and the steps, bottom-up. A step making more
// than the need credits the leftover to the pool.
func (p *planner) need(item int32, n int64, open bool, depth int) (int64, []Step, bool) {
	if p.work >= p.cfg.MaxWork {
		p.exhausted = true
		return 0, nil, false
	}
	p.work++
	got := p.pool.take(item, n, open || p.priced(item))
	used := got * p.value(item)
	rest := n - got
	if rest == 0 {
		return used, nil, true
	}
	if depth >= maxDepth {
		return 0, nil, false
	}
	var makers []datapack.Recipe
	for _, r := range p.makers[item] {
		if r.SuccessRate >= 100 {
			makers = append(makers, r)
		}
	}
	switch len(makers) {
	case 0:
		return 0, nil, false
	case 1:
		c, s, ok := p.craft(makers[0], item, rest, open, depth)
		return used + c, s, ok
	}
	// Several 100 % recipes: the one using up the least value, then the
	// fewer steps.
	var bestR datapack.Recipe
	bestC, bestN, found := int64(0), 0, false
	for _, r := range makers {
		mark := p.pool.mark()
		c, s, ok := p.craft(r, item, rest, open, depth)
		p.pool.rollback(mark)
		if ok && (!found || c < bestC || c == bestC && len(s) < bestN) {
			bestR, bestC, bestN, found = r, c, len(s), true
		}
	}
	if !found {
		return 0, nil, false
	}
	c, s, ok := p.craft(bestR, item, rest, open, depth)
	return used + c, s, ok
}

// craft makes rest units of item with r, ceil(rest/yield) times.
func (p *planner) craft(r datapack.Recipe, item int32, rest int64, open bool, depth int) (int64, []Step, bool) {
	times := ceilDiv(rest, int64(r.Product.Count))
	var used int64
	var steps []Step
	for _, m := range r.Materials {
		c, s, ok := p.need(m.ItemID, int64(m.Count)*times, open, depth+1)
		if !ok {
			return 0, nil, false
		}
		used += c
		steps = append(steps, s...)
	}
	if left := times*int64(r.Product.Count) - rest; left > 0 {
		p.pool.credit(item, left)
		used -= left * p.value(item)
	}
	return used, append(steps, Step{Recipe: r.ID, Times: times}), true
}

func (p *planner) priced(item int32) bool {
	_, ok := p.in.Prices[item]
	return ok
}

// pool is what a plan may take: the leftovers its steps made first, then
// the inventory, then the warehouse. Every change is journaled so an
// evaluation rolls back.
type pool struct {
	made, inv, wh   map[int32]int64
	invUsed, whUsed map[int32]int64
	log             []move
}

// move is one change: the units taken from each layer (a credit is a
// negative take from made).
type move struct {
	item          int32
	made, inv, wh int64
}

// take takes up to n units of item and returns how many. stock is false
// for an item a chain may use only when its own steps made it: the
// inventory and warehouse are not touched then.
func (p *pool) take(item int32, n int64, stock bool) int64 {
	m := move{item: item}
	m.made = min(n, p.made[item])
	if stock {
		m.inv = min(n-m.made, p.inv[item])
		m.wh = min(n-m.made-m.inv, p.wh[item])
	}
	if m.made == 0 && m.inv == 0 && m.wh == 0 {
		return 0
	}
	p.apply(m, 1)
	p.log = append(p.log, m)
	return m.made + m.inv + m.wh
}

// credit gives n units of item made beyond a need back to the pool.
func (p *pool) credit(item int32, n int64) {
	m := move{item: item, made: -n}
	p.apply(m, 1)
	p.log = append(p.log, m)
}

func (p *pool) apply(m move, sign int64) {
	p.made[m.item] -= sign * m.made
	p.inv[m.item] -= sign * m.inv
	p.wh[m.item] -= sign * m.wh
	p.invUsed[m.item] += sign * m.inv
	p.whUsed[m.item] += sign * m.wh
}

func (p *pool) mark() int { return len(p.log) }

func (p *pool) rollback(mark int) {
	for i := len(p.log) - 1; i >= mark; i-- {
		p.apply(p.log[i], -1)
	}
	p.log = p.log[:mark]
}

// ingredients lists the positive counts of m by item id.
func ingredients(m map[int32]int64) []datapack.Ingredient {
	var out []datapack.Ingredient
	for id, n := range m {
		if n > 0 {
			out = append(out, datapack.Ingredient{ItemID: id, Count: int32(min(n, int64(1<<31-1)))})
		}
	}
	slices.SortFunc(out, func(a, b datapack.Ingredient) int { return cmp.Compare(a.ItemID, b.ItemID) })
	return out
}

func ceilDiv(a, b int64) int64 { return (a + b - 1) / b }
