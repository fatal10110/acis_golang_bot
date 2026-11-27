package botclient

import "time"

// buffPotionWait is how long the bot drinks no buff potion after one
// UseItem of it unless its effect shows in the bot's abnormals, which then
// stops the next one by its stack (buffCovered). The server accepts a buff
// potion only every 500 ms (its reuse_delay) and drops one still cooling in
// silence (live.ItemDisabled, network/inventory.go); an item skill cast
// that finds another busy waits for it (itemAICastBusy,
// network/item_skill_cast.go), so the effect may come late or not at all.
const buffPotionWait = 5 * time.Second

// buffPotion is the build's buff potion (GearClass.BuffPotion) and its
// datapack entry; false when the build drinks none or the travel map
// knows no such potion.
func (b *Brain) buffPotion() (int32, buffPotion, bool) {
	id := b.gearClass().BuffPotion
	if id == 0 || b.cfg.Travel == nil {
		return 0, buffPotion{}, false
	}
	bp, ok := b.cfg.Travel.buffPotions[id]
	return id, bp, ok
}

// ruleBuffPotion drinks the build's buff potion (buffPotion) at the bot's
// spot, out of combat with no attacker on it and standing, while no effect
// of the potion's stack type with an equal or higher order is on the bot
// (buffCovered): a support bot's Haste or Acumen, the Newbie Guide's
// beginner buffs or the potion's own effect. One UseItem goes out per
// potion, then none for buffPotionWait unless its effect shows; the frame
// pacer bounds the rest.
func (b *Brain) ruleBuffPotion(now time.Time) (Action, bool) {
	s, v := &b.world.Self, &b.spot
	if b.cfg.Spots == nil || !v.farming() || !v.arrived || s.InCombat || s.Sitting || len(b.attackers(now)) > 0 {
		return Action{}, false
	}
	id, bp, ok := b.buffPotion()
	if !ok {
		return Action{}, false
	}
	if at, used := b.usedAt[id]; used && now.Sub(at) < buffPotionWait {
		return Action{}, false
	}
	if b.buffCovered(bp.stack, now) {
		return Action{}, false
	}
	it, ok := b.world.Inventory.ByTemplate(id)
	if !ok || it.Count <= 0 {
		return Action{}, false
	}
	return useItemAction(it), true
}

// buffCovered reports whether an effect on the bot that has not run out at
// now has the stack type of want with an equal or higher order, at the
// level the bot's abnormals list it at (TravelMap.stackOf): the server
// would not land want over it.
func (b *Brain) buffCovered(want stackEffect, now time.Time) bool {
	for id, end := range b.world.Buffs {
		if !end.IsZero() && !end.After(now) {
			continue
		}
		st, ok := b.cfg.Travel.stackOf(id, b.world.BuffLevels[id])
		if ok && st.kind == want.kind && st.order >= want.order {
			return true
		}
	}
	return false
}

// buffPotionRow is the comfort tier's buff potion purchase: a trip's worth
// of the build's buff potion (one per effect time of TripMinutes, margin
// included), from a spawned merchant, less what the bot carries, within
// the weight the supplies leave under BuyLoad. A Restock row of it buys it
// instead.
func (b *Brain) buffPotionRow() (buyRow, bool) {
	id, bp, ok := b.buffPotion()
	if !ok || bp.lasts <= 0 || !b.cfg.Travel.spawnedSold[id] {
		return buyRow{}, false
	}
	if _, ok := b.restockOf(id); ok {
		return buyRow{}, false
	}
	n := b.tripCount(1/bp.lasts.Minutes()) - b.carried(id)
	sup := b.supplies()
	if room, capped := b.weightCap(sup); capped {
		for _, s := range sup {
			room -= int64(s.count) * int64(b.cfg.Travel.weights[s.item])
		}
		if wt := b.cfg.Travel.weights[id]; wt > 0 {
			n = min(n, int32(max(room, 0)/int64(wt)))
		}
	}
	return buyRow{Item: id, Count: n}, n > 0
}
