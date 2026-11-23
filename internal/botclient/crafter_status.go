package botclient

import (
	"fmt"
	"time"
)

// CrafterStatus is a crafter's part of its Status (docs/crafting.md
// section 9), nil for a bot that plays no crafter's cycle. It is immutable
// once published.
type CrafterStatus struct {
	// CreateItem is the Create Item level, 0 before the bot learned it.
	CreateItem int32
	// Recipes is how many recipes the book holds on both pages, and
	// BookKnown is set once both pages were read (RecipeBook).
	Recipes   int
	BookKnown bool
	// Orders is how many orders of the clan's craft board are open, and
	// Claimed how many of them the bot claimed or crafted and has not
	// delivered yet.
	Orders, Claimed int
	// Phase is where the cycle stands: idle, look, withdraw, make or
	// deposit.
	Phase string
	// Chains is how many chains the cycle's plan holds and ChainsDone how
	// many of them it crafted, both 0 outside a plan.
	Chains, ChainsDone int
	// NextCheck is when the next check is due while the cycle is idle,
	// zero otherwise.
	NextCheck time.Time
	// Last is the last cycle that ended, the zero CraftCycleEnd before one.
	Last CraftCycleEnd
	// BreakerUntil is when a tripped breaker's hold on the next check
	// ends (CrafterPolicy.BreakerHold), zero while none holds.
	BreakerUntil time.Time
}

// CraftCycleEnd is how and when a crafter's cycle ended.
type CraftCycleEnd struct {
	At      time.Time
	Outcome CraftOutcome
}

// CraftOutcome is how a crafter's cycle ended.
type CraftOutcome uint8

const (
	// CraftCycleNone: no cycle ended yet.
	CraftCycleNone CraftOutcome = iota
	// CraftCycleCrafted: the cycle crafted at least one chain.
	CraftCycleCrafted
	// CraftCycleNothing: the cycle found nothing to craft.
	CraftCycleNothing
	// CraftCycleAborted: a warehouse step failed or ran past its deadline,
	// or the bot left its clan.
	CraftCycleAborted
	// CraftCycleTripped: a breaker (FailStreak, TimeoutStreak) stopped it.
	CraftCycleTripped
)

var craftOutcomeNames = [...]string{
	CraftCycleNone:    "none",
	CraftCycleCrafted: "crafted",
	CraftCycleNothing: "nothing to craft",
	CraftCycleAborted: "aborted",
	CraftCycleTripped: "breaker tripped",
}

func (o CraftOutcome) String() string {
	if int(o) < len(craftOutcomeNames) {
		return craftOutcomeNames[o]
	}
	return fmt.Sprintf("CraftOutcome(%d)", uint8(o))
}

var crafterPhaseNames = [...]string{
	craftIdle:     "idle",
	craftLook:     "look",
	craftWithdraw: "withdraw",
	craftMake:     "make",
	craftDeposit:  "deposit",
}

func (p crafterPhase) String() string {
	if int(p) < len(crafterPhaseNames) {
		return crafterPhaseNames[p]
	}
	return fmt.Sprintf("crafterPhase(%d)", uint8(p))
}

// craftEnded is what the crafter keeps of its last cycle past the cycle's
// reset (endCraftCycle): how it ended, and until when a tripped breaker
// holds the next check.
type craftEnded struct {
	last      CraftCycleEnd
	holdUntil time.Time
}

// noteCraftEnd records the end of the cycle c at now, its next check due
// at next.
func (c *crafterCycle) noteCraftEnd(now, next time.Time, ok bool) {
	out := CraftCycleNothing
	switch {
	case c.tripped:
		out = CraftCycleTripped
	case !ok:
		out = CraftCycleAborted
	case c.crafted:
		out = CraftCycleCrafted
	}
	c.ended = craftEnded{last: CraftCycleEnd{At: now, Outcome: out}}
	if c.tripped {
		c.ended.holdUntil = next
	}
}

// resumeFrom starts a session's cycle from the last cycle end e of an earlier
// session: a breaker's hold still holds the next check; past a cycle that
// did not trip, the new session checks at once.
func (c *crafterCycle) resumeFrom(e craftEnded) {
	c.ended, c.nextCheck = e, e.holdUntil
}

// crafterStatus is the crafter's part of the snapshot at now, nil when the
// brain plays no crafter's cycle. It is fresh on every call.
func (b *Brain) crafterStatus(now time.Time) *CrafterStatus {
	if !b.crafting() {
		return nil
	}
	c, book := &b.craft, &b.world.Recipes
	st := &CrafterStatus{
		CreateItem: b.world.Skills[skillCreateItem].Level,
		Recipes:    len(book.Dwarven) + len(book.Common),
		BookKnown:  book.DwarvenKnown && book.CommonKnown,
		Phase:      c.phase.String(),
		Last:       c.ended.last,
	}
	if c.phase == craftIdle {
		st.NextCheck = c.nextCheck
		if now.Before(c.ended.holdUntil) {
			st.BreakerUntil = c.ended.holdUntil
		}
	} else {
		st.Chains, st.ChainsDone = len(c.plan.Chains), min(c.chain, len(c.plan.Chains))
	}
	self := b.world.Self.ObjectID
	for _, o := range b.cfg.CraftBoard.Orders(b.world.Clan.ID, now) {
		st.Orders++
		if o.Crafter == self && (o.State == OrderClaimed || o.State == OrderCrafted) {
			st.Claimed++
		}
	}
	return st
}
