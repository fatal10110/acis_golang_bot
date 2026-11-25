package botclient

import "strings"

// setPartySlot makes c, a lead or a join, the Bot's party slot. It replaces
// the panel's standing order as a new panel order does: the standing order
// is gone and the session drops its stay, roam, follow and assist, keeping a
// reply or a withdrawal not sent yet, so neither applyStanding nor followed
// keeps the old order. A join's leader is known for the rest of the run
// (slotAnswer).
func (b *Brain) setPartySlot(c Command) {
	l := b.cfg.link
	l.order = standing{}
	b.orders = orders{reply: b.orders.reply, withdraw: b.orders.withdraw}
	b.orderPending = false
	l.party = c
	if c.Kind == CommandJoin {
		if l.leaders == nil {
			l.leaders = make(map[string]bool)
		}
		l.leaders[slotLeader(c)] = true
	}
}

// unparty carries out the panel's unparty: it clears the party slot and
// withdraws only from the bot party the slot named (inSlotParty), never from
// a player's; ruleCommand sends the withdrawal. A slot cleared in no party
// resets the leader watch, so a bot freed from a dispersed bot party neither
// whispers its old leader nor casts /unstuck (docs/parties/members.md,
// "Clearing from no party"). With no slot it does nothing.
func (b *Brain) unparty() {
	l := b.cfg.link
	slot := l.party
	if slot.Kind == 0 {
		return
	}
	l.party = Command{}
	switch {
	case !b.world.Party.In():
		b.lead = leaderWatch{}
	case b.inSlotParty(slot):
		b.orders.withdraw = true
	}
}

// inSlotParty reports whether the bot is in the bot party slot names: the
// party it leads under a lead, the named leader's under a join. Names match
// exactly, never by the bot marker.
func (b *Brain) inSlotParty(slot Command) bool {
	p := &b.world.Party
	if !p.In() {
		return false
	}
	switch slot.Kind {
	case CommandLead:
		return p.LeaderID == b.world.Self.ObjectID
	case CommandJoin:
		m := p.Members[p.LeaderID]
		return m != nil && m.Name == slotLeader(slot)
	}
	return false
}

// slotLeader is the leader a join names, its first name.
func slotLeader(c Command) string {
	name, _, _ := strings.Cut(c.Arg, " ")
	return name
}

// slotAnswer is the party slot's answer to an invitation from requester, ok
// false for one it leaves to the persona and the claim table: a join accepts
// its named leader's ahead of the persona, outside the claim table (D29), and
// any other leader a join of this run named is declined, slot set or not.
// Names match exactly: the bot marker plays no part.
func (b *Brain) slotAnswer(requester string) (accept, ok bool) {
	l := b.cfg.link
	if l == nil {
		return false, false
	}
	if l.party.Kind == CommandJoin && slotLeader(l.party) == requester {
		return true, true
	}
	if l.leaders[requester] {
		return false, true
	}
	return false, false
}
