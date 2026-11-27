package botclient

import (
	"reflect"
	"testing"
	"time"
)

// TestBrainCrystallizePoolGear checks the clan's gear a cycle withdrew and
// ended without depositing (a deadline, then a reconnect): the board keeps
// it, and the next session's cycle crystallizes it, or deposits it whole
// when it cannot, never keeping it as the bot's own.
func TestBrainCrystallizePoolGear(t *testing.T) {
	first := crystallizingBrain(t)
	first.endCraftCycle(t0, false)
	board := first.cfg.CraftBoard
	if got, want := board.PoolGear(77, first.world.Self.ObjectID), map[int32]bool{gotWeapon.ObjectID: true, gotOther.ObjectID: true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pool gear %v, want %v", got, want)
	}
	next := t0.Add(time.Hour)
	reconnect := func(t *testing.T, level int32) *Brain {
		t.Helper()
		b := newCzBrain(t, adenaStack(1000), ownWeapon, gotWeapon, gotOther)
		b.cfg.CraftBoard = board
		b.world.Self.ObjectID = first.world.Self.ObjectID
		b.world.Skills[skillCrystallize] = KnownSkill{ID: skillCrystallize, Level: level}
		b.wh.status.Known = true
		b.craft.begin(craftLook, WarehouseRequest{Look: true}, next)
		b.craftPlan(next, true)
		return b
	}
	t.Run("crystallized", func(t *testing.T) {
		b := reconnect(t, 1)
		sendCz(t, b, next, gotWeapon.ObjectID)
	})
	t.Run("deposited whole", func(t *testing.T) {
		b := reconnect(t, 0)
		c := &b.craft
		if c.phase != craftDeposit || !reflect.DeepEqual(c.req.Deposit, bothWhole) {
			t.Fatalf("phase %d deposit %+v, want both pool weapons deposited, the spare kept", c.phase, c.req.Deposit)
		}
		if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: gotWeapon}, {State: ItemRemoved, Item: gotOther}}}}, next); err != nil {
			t.Fatal(err)
		}
		b.endCraftCycle(next, true)
		if got := board.PoolGear(77, b.world.Self.ObjectID); got != nil {
			t.Fatalf("pool gear %v after the deposit, want none", got)
		}
	})
}
