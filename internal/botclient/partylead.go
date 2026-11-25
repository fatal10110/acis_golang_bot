package botclient

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// The loot rules of a party (party/party.go on the server): finders
// keepers, random, random with spoil, by turn, by turn with spoil.
const (
	lootFindersKeepers int32 = iota
	lootRandom
	lootRandomSpoil
	lootByTurn
	lootByTurnSpoil
)

// partyLoot is the loot rule every invitation of a lead bot carries (D27):
// the server gives the party the rule of the invitation that formed it and
// keeps it for later ones.
const partyLoot = lootByTurn

// The system messages a lead bot reads (server network/party.go
// requestJoinParty, party/party.go BeginInvite and remove).
const (
	msgLeftParty         = 108 // S1, a member, left the party
	msgWrongTarget       = 152 // self, invisible or cursed
	msgOnlyLeaderInvites = 154
	msgPartyFull         = 155
	msgAlreadyInParty    = 160 // S1 is in a party
	msgWaitingForReply   = 164 // the bot's own invitation or request is open
	msgNoSuchInvitee     = 185 // offline or unknown
	msgIgnoredBy         = 619 // S1 blocks the bot
	msgBlockedEverything = 940 // S1 blocks everyone
	msgS1                = 1987
)

// The texts of the 1987 refusals: the invitee is detached (offline mode)
// or either side is jailed (server network/party.go).
const (
	inviteDetachedText = "The player you tried to invite is in offline mode."
	inviteJailedText   = "The player you tried to invite is currently jailed."
)

// LeadPolicy times a lead bot's invitations and a join bot's handover
// (docs/parties/leader.md) and how long a leader waits for its members at
// its spot (docs/parties/spots.md, "Gather gate"). A zero field takes its
// default, so no persona invites faster than the server's invitation
// window.
type LeadPolicy struct {
	// InviteWait ends an invitation with no answer: the server's 15 s
	// window counts from when it processed the request. After a 164 (the
	// bot's own window still open) the next invitation waits for it too.
	InviteWait time.Duration
	// InviteRetry is how long a member who refused, declined, did not
	// answer or left the party waits for its next invitation.
	InviteRetry time.Duration
	// HandoverWait is how long a join bot made leader waits for a window
	// naming the player it handed the party to before it asks the next;
	// HandoverFor bounds the handover, past a duel's 2 minutes.
	HandoverWait time.Duration
	HandoverFor  time.Duration
	// GatherRange: at its spot a leader picks no new farm target while a
	// living bot member's reported position is farther from it than
	// GatherRange (inside the server's 1500 party range), for at most
	// GatherWait; a member farther than FarAway does not hold it.
	GatherRange int32
	GatherWait  time.Duration
	FarAway     int32
	// JoinReplies caps the whispers answering a join ("party is full",
	// "ask <leader>") within any joinReplyWindow, across the bot's
	// sessions and apart from ChatPolicy.Total (docs/parties/players.md).
	JoinReplies int
}

// defaultLeadPolicy is every persona's.
func defaultLeadPolicy() LeadPolicy {
	return LeadPolicy{
		InviteWait:   partyInviteTimeout + 2*time.Second,
		InviteRetry:  30 * time.Second,
		HandoverWait: 5 * time.Second,
		HandoverFor:  150 * time.Second,
		GatherRange:  1200,
		GatherWait:   time.Minute,
		FarAway:      10000,
		JoinReplies:  10,
	}
}

// withDefaults is p with each field not above zero the default's.
func (p LeadPolicy) withDefaults() LeadPolicy {
	d := defaultLeadPolicy()
	return LeadPolicy{
		InviteWait:   positiveOr(p.InviteWait, d.InviteWait),
		InviteRetry:  positiveOr(p.InviteRetry, d.InviteRetry),
		HandoverWait: positiveOr(p.HandoverWait, d.HandoverWait),
		HandoverFor:  positiveOr(p.HandoverFor, d.HandoverFor),
		GatherRange:  positiveOr(p.GatherRange, d.GatherRange),
		GatherWait:   positiveOr(p.GatherWait, d.GatherWait),
		FarAway:      positiveOr(p.FarAway, d.FarAway),
		JoinReplies:  positiveOr(p.JoinReplies, d.JoinReplies),
	}
}

// positiveOr is v when above zero, else d.
func positiveOr[T int | int32 | time.Duration](v, d T) T {
	if v > 0 {
		return v
	}
	return d
}

// partyLead is a lead bot's invitations and oustings, its join queue and a
// join bot's handover, owned by the brain goroutine (rule "party"). It
// lasts the session: a new session invites its listed members afresh and
// starts with an empty join queue.
type partyLead struct {
	// invitee is the member the invitation in flight went to, at sentAt;
	// "" for none. JoinParty names no one, so the bot remembers it. player
	// marks an invitee taken off the join queue.
	invitee string
	sentAt  time.Time
	player  bool
	// queue are the players who whispered join, in order (heardJoin), and
	// heard when each sender may be answered again, by name.
	queue []string
	heard map[string]time.Time
	// next is when the next invitation may go after a 164.
	next time.Time
	// retry is when each member may be invited again, by name.
	retry map[string]time.Time
	// ousted are the members expelled whose window has not dropped them.
	ousted   map[string]bool
	handover handover
}

// handover is a join bot's handing of the party it was made leader of to
// a player (docs/parties/leader.md, "Handover"). start is zero while none
// runs; asked is the object id last asked, at at; left marks the
// withdrawal that ends it.
type handover struct {
	start time.Time
	asked int32
	at    time.Time
	left  bool
}

// waitFor keeps name from the next invitation for d after now. It drops
// the waits that ended by now, so a bot that never leads keeps the map
// to the members of its recent parties.
func (v *partyLead) waitFor(name string, now time.Time, d time.Duration) {
	if name == "" {
		return
	}
	if v.retry == nil {
		v.retry = make(map[string]time.Time)
	}
	for n, t := range v.retry {
		if !t.After(now) {
			delete(v.retry, n)
		}
	}
	if t := now.Add(d); t.After(v.retry[name]) {
		v.retry[name] = t
	}
}

// partyLeadEvent ends the invitation in flight on its answer or refusal and
// notes the members who left. It runs before the world applies ev: a
// dispersal (PartySmallWindowDeleteAll) counts every member of the party
// before it as left.
func (b *Brain) partyLeadEvent(ev Event, now time.Time) {
	v, pol := &b.partyLead, b.persona.Lead.withDefaults()
	switch e := ev.(type) {
	case JoinParty:
		if v.invitee == "" {
			return
		}
		if e.Response == 1 {
			// The window adding it follows the answer; a lost add frees
			// it for another invitation one InviteWait later.
			v.endInvite(now, pol.InviteWait)
		} else {
			v.endInvite(now, pol.InviteRetry)
		}
	case SystemMessage:
		switch {
		case e.ID == msgLeftParty:
			// A withdrawal or a disconnect (party/party.go remove).
			v.waitFor(e.Text[0], now, pol.InviteRetry)
		case v.invitee == "":
		case e.ID == msgWaitingForReply:
			// The bot's own window: a join player goes back to the head.
			if v.player {
				v.queue = slices.Insert(v.queue, 0, v.invitee)
			}
			v.invitee, v.player, v.next = "", false, now.Add(pol.InviteWait)
		case inviteRefused(e, v.invitee):
			v.endInvite(now, pol.InviteRetry)
		}
	case PartySmallWindowDeleteAll:
		for _, m := range b.world.Party.Members {
			v.waitFor(m.Name, now, pol.InviteRetry)
		}
		v.ousted = nil
	case PartySmallWindowDelete:
		delete(v.ousted, e.Name)
	}
}

// endInvite ends the invitation in flight: a listed member waits d for its
// next one, a join player already off the queue gets none.
func (v *partyLead) endInvite(now time.Time, d time.Duration) {
	if !v.player {
		v.waitFor(v.invitee, now, d)
	}
	v.invitee, v.player = "", false
}

// dropJoins forgets the join queue of a lead slot just cleared, so a later
// lead slot never invites a player who asked to join another party (plan
// section 6: never unasked). A player's invitation in flight still ends on
// its answer, but a 164 no longer puts it back on the queue.
func (v *partyLead) dropJoins() {
	v.queue, v.player = nil, false
}

// inviteRefused reports whether m refuses the invitation to invitee: a
// refusal naming no one, one naming the invitee, or the 1987 text of a
// detached or jailed invitee.
func inviteRefused(m SystemMessage, invitee string) bool {
	switch m.ID {
	case msgWrongTarget, msgOnlyLeaderInvites, msgPartyFull, msgNoSuchInvitee:
		return m.Text[0] == ""
	case msgS1IsBusyTryLater, msgAlreadyInParty, msgIgnoredBy, msgBlockedEverything:
		return m.Text[0] == invitee
	case msgS1:
		return m.Text[0] == inviteDetachedText || m.Text[0] == inviteJailedText
	}
	return false
}

// ruleParty sends a lead bot's invitations and oustings and runs a join
// bot's handover (docs/parties/leader.md). It claims the decision only for
// a packet it sends.
//
// Pacing: one RequestJoinParty in flight (InviteWait, or the answer), one
// RequestOustPartyMember per decision, one RequestChangePartyLeader per
// HandoverWait for at most HandoverFor, the withdrawal through the
// withdraw step; all under the frame bucket. The server gates none of them
// beyond the frame flood limits (network/client_dispatch.go:147,152,324).
func (b *Brain) ruleParty(now time.Time) (Action, bool) {
	l := b.cfg.link
	var slot Command
	if l != nil {
		slot = l.party
	}
	if slot.Kind != CommandJoin || !b.leading() {
		b.partyLead.handover = handover{}
	}
	switch slot.Kind {
	case CommandLead:
		if a, ok := b.oust(slot, now); ok {
			return a, true
		}
		return b.invite(slot, now)
	case CommandJoin:
		if b.leading() {
			return b.handOver(slot, now)
		}
	}
	return Action{}, false
}

// leading reports whether the bot leads a party.
func (b *Brain) leading() bool {
	p := &b.world.Party
	return p.In() && p.LeaderID == b.world.Self.ObjectID
}

// invite sends the next invitation of a lead bot in no party or leading its
// own (inside another party the server answers 154): one at a time, to
// nextInvitee, under loot rule partyLoot. An invitation with no answer
// within InviteWait ends and its invitee waits InviteRetry.
func (b *Brain) invite(slot Command, now time.Time) (Action, bool) {
	p, v := &b.world.Party, &b.partyLead
	if p.In() && !b.leading() {
		return Action{}, false
	}
	pol := b.persona.Lead.withDefaults()
	if v.invitee != "" {
		if now.Before(v.sentAt.Add(pol.InviteWait)) {
			return Action{}, false
		}
		// No answer: a silent client, or the Olympiad's silence
		// (network/party.go).
		v.endInvite(now, pol.InviteRetry)
	}
	if now.Before(v.next) || len(p.Members)+1 >= maxPartyMembers {
		return Action{}, false
	}
	name, player := b.nextInvitee(slot, now)
	if name == "" {
		return Action{}, false
	}
	a := inviteAction(name)
	if !b.pace.allow(a.gate(), now) {
		return Action{}, false
	}
	if player {
		v.queue = v.queue[1:]
	}
	v.invitee, v.sentAt, v.player = name, now, player
	return a, true
}

// nextInvitee is who the next invitation goes to: the head of the join
// queue (player true) while the party has a free place (freePlaces), else
// the first member slot lists who is not the bot, not in the party and not
// waiting out an InviteRetry; "" for none. A queued player found in the
// party leaves the queue.
func (b *Brain) nextInvitee(slot Command, now time.Time) (name string, player bool) {
	v := &b.partyLead
	v.queue = slices.DeleteFunc(v.queue, b.partyNamed)
	if len(v.queue) > 0 && b.freePlaces(slot) > 0 {
		return v.queue[0], true
	}
	for _, name := range strings.Fields(slot.Arg) {
		if name == b.world.Self.Name || b.partyNamed(name) || now.Before(v.retry[name]) {
			continue
		}
		return name, false
	}
	return "", false
}

// freePlaces is how many places of the party a lead bot leads, or is
// about to, are free for join players: maxPartyMembers less the members,
// the bot among them, less every bot slot lists not in the party
// (reconnecting, in the join hold, between invitations), whose places stay
// theirs. The server answers an invitation into a full party 155
// (party/party.go BeginInvite).
func (b *Brain) freePlaces(slot Command) int {
	n := maxPartyMembers - 1 - len(b.world.Party.Members)
	for _, name := range strings.Fields(slot.Arg) {
		if name != b.world.Self.Name && !b.partyNamed(name) {
			n--
		}
	}
	return n
}

// partyNamed reports whether a member of the bot's party is named name.
func (b *Brain) partyNamed(name string) bool {
	for _, m := range b.world.Party.Members {
		if m.Name == name {
			return true
		}
	}
	return false
}

// oust expels from the party the bot leads one member a lead of this run
// listed and slot no longer does: a bot, never a player nobody listed.
func (b *Brain) oust(slot Command, now time.Time) (Action, bool) {
	l, v := b.cfg.link, &b.partyLead
	if !b.leading() || len(l.listed) == 0 {
		return Action{}, false
	}
	for _, id := range slices.Sorted(maps.Keys(b.world.Party.Members)) {
		name := b.world.Party.Members[id].Name
		if name == "" || !l.listed[name] || slotNames(slot, name) || v.ousted[name] {
			continue
		}
		a := oustAction(name)
		if !b.pace.allow(a.gate(), now) {
			return Action{}, false
		}
		if v.ousted == nil {
			v.ousted = make(map[string]bool)
		}
		v.ousted[name] = true
		return a, true
	}
	return Action{}, false
}

// handOver runs the handover of a join bot made leader: with every other
// member a bot its slot names it withdraws at once, dispersing the party;
// else it hands the party to its players by object id, the lowest first,
// one each HandoverWait and back to the lowest after the last, and
// withdraws past HandoverFor or once no player is left. The join hold
// keeps it where it stands meanwhile (joinHold).
func (b *Brain) handOver(slot Command, now time.Time) (Action, bool) {
	v, pol := &b.partyLead.handover, b.persona.Lead.withDefaults()
	if v.start.IsZero() {
		v.start = now
	}
	if v.left {
		return Action{}, false
	}
	players := b.partyPlayers(slot)
	if len(players) == 0 || now.Sub(v.start) >= pol.HandoverFor {
		v.left = true
		b.orders.withdraw = true
		return b.ruleCommand(now)
	}
	if !v.at.IsZero() && now.Before(v.at.Add(pol.HandoverWait)) {
		return Action{}, false
	}
	id := players[0]
	if i := slices.IndexFunc(players, func(p int32) bool { return p > v.asked }); i >= 0 {
		id = players[i]
	}
	a := changeLeaderAction(b.world.Party.Members[id].Name)
	if !b.pace.allow(a.gate(), now) {
		return Action{}, false
	}
	v.asked, v.at = id, now
	return a, true
}

// partyPlayers are the object ids, in order, of the members of the bot's
// party slot does not name: players.
func (b *Brain) partyPlayers(slot Command) []int32 {
	var ids []int32
	for id, m := range b.world.Party.Members {
		if !slotNames(slot, m.Name) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// encodeRequestOustPartyMember expels the member named name from the
// party the bot leads; the server ignores it from anyone else or for a
// name no member has (network/party.go requestOustPartyMember).
func encodeRequestOustPartyMember(name string) []byte {
	w := wire.NewPacketWriter(opGameRequestOustPartyMember)
	w.WriteString(name)
	return w.Bytes()
}

// encodeRequestChangePartyLeader hands the party the bot leads to the
// member named name; a dueling member is refused in silence
// (party/party.go ChangeLeader).
func encodeRequestChangePartyLeader(name string) []byte {
	w := wire.NewPacketWriter(opGameClientExtended)
	w.WriteUint16(opGameRequestChangePartyLeader)
	w.WriteString(name)
	return w.Bytes()
}

// inviteAction invites the player named name under partyLoot.
func inviteAction(name string) Action {
	return Action{kind: actInviteParty, payload: encodeRequestJoinParty(name, partyLoot)}
}

// oustAction expels the member named name.
func oustAction(name string) Action {
	return Action{kind: actOustParty, payload: encodeRequestOustPartyMember(name)}
}

// changeLeaderAction hands the party to the member named name.
func changeLeaderAction(name string) Action {
	return Action{kind: actChangeLeader, payload: encodeRequestChangePartyLeader(name)}
}
