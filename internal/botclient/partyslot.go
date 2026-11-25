package botclient

import (
	"slices"
	"strings"
)

// setPartySlot makes c, a lead or a join, the Bot's party slot. It replaces
// the panel's standing order as a new panel order does: the standing order
// is gone and the session drops its stay, roam, follow and assist, keeping a
// reply or a withdrawal not sent yet, so neither applyStanding nor followed
// keeps the old order. A join's leader is known for the rest of the run
// (slotAnswer), as is every member a lead lists (oust).
func (b *Brain) setPartySlot(c Command) {
	l := b.cfg.link
	l.order = standing{}
	b.orders = orders{reply: b.orders.reply, withdraw: b.orders.withdraw}
	b.orderPending = false
	l.party, l.partyLeft = c, ""
	switch c.Kind {
	case CommandJoin:
		if l.leaders == nil {
			l.leaders = make(map[string]bool)
		}
		l.leaders[slotLeader(c)] = true
	case CommandLead:
		if l.listed == nil {
			l.listed = make(map[string]bool)
		}
		for _, name := range strings.Fields(c.Arg) {
			l.listed[name] = true
		}
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
	l.party, l.partyLeft = Command{}, ""
	b.leaveSlot(slot)
}

// leaveSlot ends the bot's part in the party slot just cleared: in the bot
// party slot named it withdraws (orders.withdraw, sent by ruleCommand ahead
// of any reply); in no party it resets the leader watch, so a bot freed
// from a dispersed bot party neither whispers its old leader nor casts
// /unstuck. A join member the server made leader withdraws too, ending its
// handover (docs/parties/leader.md). In any other party it does nothing: it
// never takes a bot out of a player's party.
func (b *Brain) leaveSlot(slot Command) {
	switch {
	case !b.world.Party.In():
		b.lead = leaderWatch{}
	case b.inSlotParty(slot), slot.Kind == CommandJoin && b.leading():
		b.orders.withdraw = true
	}
}

// clearSlot is the step of every slot clearing event (docs/parties/orders.md,
// "Slot clearing"): a standing order of the panel or a GM, a leave, a GM's
// invite me, or the bot in a party its slot does not name. It clears the
// slot, records why (Status.PartyLeft "left") and leaves the slot's party
// as leaveSlot does, ahead of the order that follows. With no slot it does
// nothing and returns false.
func (b *Brain) clearSlot() bool {
	l := b.cfg.link
	if l == nil || l.party.Kind == 0 {
		return false
	}
	slot := l.party
	l.party, l.partyLeft = Command{}, partyLeftLeft
	b.leaveSlot(slot)
	return true
}

// Status.PartyLeft: a slot a clearing event cleared, or the join hold's
// JoinWait (joinHold).
const (
	partyLeftLeft    = "left"
	partyLeftTimeout = "timeout"
)

// spotJoinEvent gives back the spot the bot holds or farms beside when it
// joins a party it does not lead (docs/parties/members.md, "Joining"): a
// member follows its leader, so a spot it kept would sit unused. wasIn is
// whether the bot was in a party before ev. A leader change (the server's
// PartySmallWindowDeleteAll then PartySmallWindowAll) is a join too: the
// bot leading the party keeps its spot.
func (b *Brain) spotJoinEvent(ev Event, wasIn bool) {
	if _, ok := ev.(PartySmallWindowAll); !ok || wasIn || !b.spot.held && !b.spot.beside {
		return
	}
	if p := &b.world.Party; p.In() && p.LeaderID != b.world.Self.ObjectID {
		b.leaveSpot()
	}
}

// slotPartyEvent applies the player-led-party rule on each party window: a
// bot in a party led by anyone its slot does not name (a player's or a
// GM's) clears its slot, players first (D28). A join member the server
// made leader keeps it for the handover (docs/parties/leader.md).
func (b *Brain) slotPartyEvent(ev Event) {
	switch ev.(type) {
	case PartySmallWindowAll, PartySmallWindowAdd, PartySmallWindowDelete:
	default:
		return
	}
	if l := b.cfg.link; l != nil && l.party.Kind != 0 && b.ledByStranger(l.party) {
		b.clearSlot()
	}
}

// ledByStranger reports whether the bot is in a party led by anyone slot
// does not name: under a lead anyone but itself, under a join anyone but
// itself, its leader and its listed members. Names match exactly, never by
// the bot marker; a leader whose name the window has not given yet is not
// told a stranger.
func (b *Brain) ledByStranger(slot Command) bool {
	p := &b.world.Party
	if !p.In() || p.LeaderID == b.world.Self.ObjectID {
		return false
	}
	if slot.Kind == CommandLead {
		return true
	}
	m := p.Members[p.LeaderID]
	return m != nil && m.Name != "" && !slotNames(slot, m.Name)
}

// slotNames reports whether slot names name: a join's leader or listed
// member, or a lead's listed member.
func slotNames(slot Command, name string) bool {
	return slices.Contains(strings.Fields(slot.Arg), name)
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

// inBotParty reports whether the bot is in its own bot party: the party its
// lead slot leads, or the named leader's under a join (inSlotParty). A bot
// with no slot, or in a player's party, is not.
func (b *Brain) inBotParty() bool {
	l := b.cfg.link
	return l != nil && b.inSlotParty(l.party)
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
