package botclient

import (
	"slices"
	"time"
)

// The replies to a join (docs/parties/players.md).
const (
	replyPartyFull = "party is full"
	replyAskPrefix = "ask "
)

// slotted reports whether the bot holds a party slot, a lead or a join.
func (b *Brain) slotted() bool {
	l := b.cfg.link
	return l != nil && l.party.Kind != 0
}

// heardJoin takes a player's join whispered at now to a bot holding a
// party slot (D28, docs/parties/players.md): a lead bot queues the player
// for its party rule while a place is free (freePlaces, less the players
// queued or invited) and answers "party is full" otherwise; a join member
// answers "ask <leader>", naming its slot's leader. Each sender gets one
// answer per InviteRetry from when it is heard: a join sooner, from a
// queued player, the one invited or a member, gets nothing. No authority
// is asked: the whisper costs no database query.
func (b *Brain) heardJoin(t tell, now time.Time) {
	v, slot := &b.partyLead, b.cfg.link.party
	if now.Before(v.heard[t.name]) || slices.Contains(v.queue, t.name) ||
		v.player && v.invitee == t.name || b.partyNamed(t.name) {
		return
	}
	pol := b.persona.Lead.withDefaults()
	for n, until := range v.heard {
		if !until.After(now) {
			delete(v.heard, n)
		}
	}
	if v.heard == nil {
		v.heard = make(map[string]time.Time)
	}
	v.heard[t.name] = now.Add(pol.InviteRetry)
	if slot.Kind == CommandJoin {
		b.joinReply(t.name, replyAskPrefix+slotLeader(slot), now)
		return
	}
	queued := len(v.queue)
	if v.player {
		queued++
	}
	if b.freePlaces(slot)-queued <= 0 {
		b.joinReply(t.name, replyPartyFull, now)
		return
	}
	v.queue = append(v.queue, t.name)
	b.wakeBy(now.Add(b.reactDelay()))
}

// joinReply whispers text back to the player named name who asked to join,
// through the reply slot (rule "command", held only by the chat gate). None
// to a name with the bot marker, none while another reply waits, none past
// LeadPolicy.JoinReplies within the hour; the reply counts there and not in
// ChatPolicy.Total.
func (b *Brain) joinReply(name, text string, now time.Time) {
	if b.cfg.markers.Marks(name) || b.orders.reply.kind != actHold ||
		!b.cfg.chat.joinAllows(now, b.persona.Lead.withDefaults().JoinReplies) {
		return
	}
	b.orders.reply = tellAction(name, clipChat(text))
	b.wakeBy(now.Add(b.reactDelay()))
}

// playerInviteMe obeys a player's invite me (D28): the authority refused
// the sender, or there is none. Only a Support member in its bot party
// (inSlotParty under a join) acts, and only when it claims the player
// first in the claim table (claimPending): it clears its slot, withdraws
// (rule "command") and whispers "invite me" back, but not to a name with
// the bot marker. Any other bot ignores it (D3), as does every bot of a
// process without a claim table.
func (b *Brain) playerInviteMe(t tell, now time.Time) {
	c, l := b.cfg.PartyClaims, b.cfg.link
	if c == nil || l == nil || l.party.Kind != CommandJoin || b.persona.Role != RoleSupport || !b.inSlotParty(l.party) {
		return
	}
	if !c.claimPending(t.name, b.world.Self.ObjectID, now) {
		return
	}
	b.clearSlot()
	if !b.cfg.markers.Marks(t.name) {
		b.orders.reply = tellAction(t.name, inviteMeText)
	}
	b.wakeBy(now.Add(b.reactDelay()))
}

// handoverToPlayer is the step of a bot whose slot a party window cleared
// at now with no invitation answer of its own: the server, or a bot made
// leader, passed the leadership of its bot party to a player. It asks its
// party hook about the new leader as an invitation would (Persona.JoinParty:
// a known GM keeps every bot, the claim table one bot for anyone else) and
// stays when accepted; else it withdraws (orders.withdraw).
func (b *Brain) handoverToPlayer(now time.Time) {
	p := &b.world.Party
	m := p.Members[p.LeaderID]
	if m == nil {
		return
	}
	if join := b.persona.JoinParty; join != nil && join(b.world, m.Name, now) {
		return
	}
	b.orders.withdraw = true
}
