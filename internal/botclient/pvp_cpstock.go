package botclient

// cpGreaterUnits is how many CP Potions (5591, 50 CP) a Greater CP Potion
// (5592, 200 CP) counts for on the CP stock (PvPPolicy.CPStock;
// items/5500-5599.xml).
const cpGreaterUnits = 4

// cpUnits is how many CP Potions template id counts for on the CP stock:
// cpGreaterUnits for a Greater CP Potion, 1 for a CP Potion, 0 for any
// other template.
func cpUnits(id int32) int32 {
	switch id {
	case itemGreaterCPPotion:
		return cpGreaterUnits
	case itemCPPotion:
		return 1
	}
	return 0
}

// cpHeld is the CP stock the bot carries, in CP Potions.
func (b *Brain) cpHeld() int32 {
	return b.carried(itemCPPotion) + cpGreaterUnits*b.carried(itemGreaterCPPotion)
}

// cpShort is what the supply list asks for of the CP stock
// (PvPPolicy.CPStock, docs/pvp/cp-potions.md "Stock", D43), in CP
// Potions: nothing at a CPStock of 0 or while the bot carries Supply.Refill
// percent of it or more (refills); else the difference, at most what the
// weight left under BuyLoad once the other supplies are carried takes (as
// buffPotionRow). No spawned merchant sells either potion, so only the
// market errand buys it (marketWants), cheapest per CP and Greater first
// at an equal price (offers).
func (b *Brain) cpShort() int32 {
	stock, have := b.persona.PvP.CPStock, b.cpHeld()
	if stock <= 0 || !b.refills(have, stock) {
		return 0
	}
	return b.cpLack()
}

// cpLack is what the CP stock lacks, in CP Potions, at most what the
// weight left under BuyLoad once the other supplies are carried takes: the
// amount a purchase under way fills up to, cpShort without its refill
// threshold (the priest stop's second row, priestAmount).
func (b *Brain) cpLack() int32 {
	n := max(b.persona.PvP.CPStock-b.cpHeld(), 0)
	sup := b.supplies()
	if room, capped := b.weightCap(sup); capped {
		for _, s := range sup {
			room -= int64(s.count) * int64(b.cfg.Travel.weights[s.item])
		}
		if wt := b.cfg.Travel.weights[itemCPPotion]; wt > 0 {
			n = min(n, int32(max(room, 0)/int64(wt)))
		}
	}
	return n
}
