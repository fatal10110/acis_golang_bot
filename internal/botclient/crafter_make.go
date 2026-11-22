package botclient

import (
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/craftplan"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// craftNext is the cycle's next craft: after the answer to the last, or
// its timeout; the weight guard's deposit first when the bot carries 50 %
// or more of its limit; a rest first when its MP is short of the recipe's
// cost; and at least craftGap plus jitter after the last craft.
func (b *Brain) craftNext(now time.Time) (Action, bool) {
	c, p, s := &b.craft, &b.persona.Crafter, &b.world.Self
	if c.pending {
		if now.Before(c.sentAt.Add(craftReplyTimeout)) {
			return Action{}, true
		}
		c.pending = false
		if c.timeouts++; c.timeouts >= p.TimeoutStreak {
			b.tripCrafter(now)
			return Action{}, true
		}
	}
	if c.replan {
		c.replan = false
		b.craftPlan(now, false)
		if c.phase != craftMake {
			return Action{}, true
		}
	}
	step, ok := c.current()
	if !ok {
		b.finishCraftCycle(now)
		return Action{}, true
	}
	if c.made && s.MaxLoad > 0 && int64(s.Load)*100 >= int64(s.MaxLoad)*crafterWeightPercent {
		if rows := b.craftDepositRows(c.remainingNeeds(b.cfg.Travel.recipes)); len(rows) > 0 {
			c.resume = true
			b.beginDeposit(rows, now)
			return Action{}, true
		}
	}
	if r := b.cfg.Travel.recipes[step.Recipe]; s.MP < r.MPCost {
		// Rule "rest" sits until StandMP from the next decision.
		b.resting = true
		return Action{}, true
	}
	if now.Before(c.nextAt) {
		return Action{}, true
	}
	return craftAction(step.Recipe), true
}

// remainingNeeds are the materials the plan's crafts left to go use, by
// item, from recipes: what the weight guard keeps.
func (c *crafterCycle) remainingNeeds(recipes map[int32]datapack.Recipe) map[int32]int64 {
	needs := make(map[int32]int64)
	for i := c.chain; i < len(c.plan.Chains); i++ {
		for j, st := range c.plan.Chains[i].Steps {
			times := st.Times
			switch {
			case i == c.chain && j < c.step:
				continue
			case i == c.chain && j == c.step:
				times -= c.done
			}
			if r, ok := recipes[st.Recipe]; ok && times > 0 {
				for _, g := range r.Materials {
					needs[g.ItemID] += int64(g.Count) * times
				}
			}
		}
	}
	return needs
}

// craftSent records a craft that went out at now, and renews the board's
// warehouse lease on every action the withdrawal's errand sends.
func (b *Brain) craftSent(a Action, now time.Time) {
	c := &b.craft
	if c.phase == craftWithdraw && b.wh.running {
		b.cfg.CraftBoard.TakeLease(b.world.Clan.ID, b.world.Self.ObjectID, now)
	}
	if a.kind != actCraft || c.phase != craftMake {
		return
	}
	c.pending, c.sentAt, c.failure = true, now, CraftFailNone
	c.nextAt = now.Add(craftGap + time.Duration(b.rng.Int64N(int64(craftJitter)+1)))
}

// craftEvent moves the craft under way on with the server's answer ev: its
// system message (craftFailure), then RecipeItemMakeInfo; the combat
// refusal comes alone.
func (b *Brain) craftEvent(ev Event, now time.Time) {
	c := &b.craft
	if !c.pending {
		return
	}
	switch m := ev.(type) {
	case SystemMessage:
		switch f := craftFailure(m.ID); f {
		case CraftFailNone:
		case CraftInCombat:
			c.pending, c.timeouts = false, 0
			b.craftFailed(now)
		default:
			if c.failure == CraftFailNone {
				c.failure = f
			}
		}
	case RecipeItemMakeInfo:
		step, ok := c.current()
		if !ok || m.RecipeID != step.Recipe {
			return
		}
		c.pending, c.timeouts = false, 0
		switch {
		case m.Status == RecipeMadeSuccess, c.failure == CraftMixingFailed:
			// A failed roll spent the materials as a success does.
			c.fails, c.made = 0, true
			c.done++
			b.craftAdvance(now)
		case c.failure == CraftNotEnoughMP:
			b.resting = true
			b.craftFailed(now)
		case c.failure == CraftMissingMaterial:
			c.replan = true
			b.craftFailed(now)
		default:
			b.craftFailed(now)
		}
	}
}

// craftFailed counts a craft result that is neither a success nor a
// failed roll: FailStreak of them in a row stop the cycle.
func (b *Brain) craftFailed(now time.Time) {
	c := &b.craft
	if c.fails++; c.fails >= b.persona.Crafter.FailStreak {
		b.tripCrafter(now)
	}
}

// tripCrafter stops the cycle on a breaker: it deposits, then holds the
// next check for BreakerHold.
func (b *Brain) tripCrafter(now time.Time) {
	b.craft.tripped = true
	b.finishCraftCycle(now)
}

// finishCraftCycle deposits at the end of the cycle what the next plan
// does not need: the products, the crafted orders' among them, and the
// leftovers. The next plan is the planner's on the warehouse as the errand
// last knew it, so what it would withdraw again stays carried.
func (b *Brain) finishCraftCycle(now time.Time) {
	c := &b.craft
	c.pending, c.replan = false, false
	next := craftplan.Build(b.craftInput(nil, b.wh.status.Known, now), b.persona.Crafter.Plan)
	rows := b.craftDepositRows(ingredientCounts(next.Keep))
	if len(rows) == 0 {
		b.endCraftCycle(now, true)
		return
	}
	c.resume = false
	b.beginDeposit(rows, now)
}

// beginDeposit starts the deposit of rows, noting the products of the
// crafted orders it carries for their delivery.
func (b *Brain) beginDeposit(rows []WarehouseRow, now time.Time) {
	b.noteDelivering(rows, now)
	b.craft.begin(craftDeposit, WarehouseRequest{Deposit: rows}, now)
}

// noteDelivering notes the products of the bot's crafted orders that the
// deposit of rows carries, by order, for deliverOrders once it lands.
func (b *Brain) noteDelivering(rows []WarehouseRow, now time.Time) {
	c := &b.craft
	c.delivering = nil
	carried := make(map[int32]int64)
	for _, r := range rows {
		carried[r.TemplateID] += int64(r.Count)
	}
	for _, o := range b.cfg.CraftBoard.Orders(b.world.Clan.ID, now) {
		if o.State != OrderCrafted || o.Crafter != b.world.Self.ObjectID || carried[o.Product] <= 0 {
			continue
		}
		n := min(carried[o.Product], o.Count)
		carried[o.Product] -= n
		if c.delivering == nil {
			c.delivering = make(map[OrderID]int64)
		}
		c.delivering[o.ID] = n
	}
}

// deliverOrders closes the crafted orders the deposit carried at now and
// announces the GM orders among them.
func (b *Brain) deliverOrders(now time.Time) {
	for id, n := range b.craft.delivering {
		if o, ok := b.cfg.CraftBoard.Deliver(b.world.Clan.ID, id, b.world.Self.ObjectID, n); ok {
			b.announceOrder(o, now)
		}
	}
}

// endCraftCycle ends the cycle and sets the next check: CheckEvery after a
// cycle that crafted or failed, ReturnEvery after one with nothing to
// craft (the bot farms meanwhile), BreakerHold at least after a breaker
// tripped, plus up to CheckJitter; never under MinOpenGap.
func (b *Brain) endCraftCycle(now time.Time, ok bool) {
	c, p := &b.craft, &b.persona.Crafter
	b.cfg.CraftBoard.ReleaseLease(b.world.Clan.ID, b.world.Self.ObjectID)
	wait := p.CheckEvery
	if ok && !c.crafted && len(c.plan.Learn) == 0 && p.ReturnEvery > 0 {
		wait = p.ReturnEvery
	}
	if c.tripped {
		wait = max(wait, p.BreakerHold)
	}
	wait = max(wait, warehouseOpenGap)
	if p.CheckJitter > 0 {
		wait += time.Duration(b.rng.Int64N(int64(p.CheckJitter) + 1))
	}
	*c = crafterCycle{nextCheck: now.Add(wait), bookAt: c.bookAt, learn: c.learn}
}

// craftLearn takes the first recipe scroll of the plan's to learn that the
// bot carries, 0 for none (rule "recipe"); one still to withdraw waits.
func (b *Brain) craftLearn() int32 {
	c := &b.craft
	for i, s := range c.learn {
		if _, ok := b.world.Inventory.ByTemplate(s); ok {
			c.learn = slices.Delete(c.learn, i, i+1)
			return s
		}
	}
	return 0
}
