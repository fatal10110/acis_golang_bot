package botclient

import (
	"sync"
	"time"
)

// PartyClaims is the party leaders the bots of a process joined, so that at
// most one of them joins each leader's party (plan, support persona). Every
// bot of the process shares one; mu owns the claims.
//
// A claim is keyed by the leader's name. AskJoinParty names its requester
// and nothing else, and the server lets only a party's leader, or a player
// starting a party, invite (InviteNotLeader otherwise), so the requester is
// the leader. Names are unique on the server, so a requester out of the
// bot's sight is keyed the same as one in sight and needs no object id.
type PartyClaims struct {
	mu       sync.Mutex
	byLeader map[string]partyClaim
	byBot    map[int32]string
	// listed is when a bot of the process last asked for the
	// party-matching rooms (listTurn).
	listed time.Time
}

// partyClaim is the bot holding a leader, and when it last claimed it.
// pending marks a claim taken on a player's invite me, before the bot's
// own withdrawal (claimPending).
type partyClaim struct {
	bot     int32
	at      time.Time
	pending bool
}

// NewPartyClaims returns an empty claim table.
func NewPartyClaims() *PartyClaims {
	return &PartyClaims{byLeader: make(map[string]partyClaim), byBot: make(map[int32]string)}
}

// Apply points cfg at c: the brain gives its claim back (BrainConfig
// PartyClaims), and cfg.Persona's JoinParty accepts only an invitation it
// accepts itself and whose leader no other bot of c holds, claiming that
// leader for the bot. Apply before Authority.Hooks: the hooks accept a GM's
// invitation before they ask the persona, so a GM takes no claim and is
// never limited. A nil JoinParty declines every invitation and stays nil.
func (c *PartyClaims) Apply(cfg *BrainConfig) {
	cfg.PartyClaims = c
	join := cfg.Persona.JoinParty
	if join == nil {
		return
	}
	cfg.Persona.JoinParty = func(w *World, requester string, now time.Time) bool {
		return join(w, requester, now) && c.claim(requester, w.Self.ObjectID, now)
	}
}

// Held is the number of leaders the bots hold.
func (c *PartyClaims) Held() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.byLeader)
}

// claim takes leader for bot at now and gives back any other leader bot
// held. It reports false, taking nothing, when another bot holds leader; a
// bot claiming the leader it holds keeps it, from now.
func (c *PartyClaims) claim(leader string, bot int32, now time.Time) bool {
	return c.take(leader, partyClaim{bot: bot, at: now})
}

// claimPending claims leader, a player whose invite me the bot obeys, for
// bot at now as claim does, marked pending: the window that answers the
// bot's own withdrawal keeps it (settle), so a second member hearing the
// same player stays.
func (c *PartyClaims) claimPending(leader string, bot int32, now time.Time) bool {
	return c.take(leader, partyClaim{bot: bot, at: now, pending: true})
}

// take gives leader to h.bot unless another bot holds it.
func (c *PartyClaims) take(leader string, h partyClaim) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if o, ok := c.byLeader[leader]; ok && o.bot != h.bot {
		return false
	}
	if old, ok := c.byBot[h.bot]; ok && old != leader {
		delete(c.byLeader, old)
	}
	c.byLeader[leader] = h
	c.byBot[h.bot] = leader
	return true
}

// settle drops the pending mark of the claim bot holds and reports whether
// there was one.
func (c *PartyClaims) settle(bot int32) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	leader, ok := c.byBot[bot]
	if !ok || !c.byLeader[leader].pending {
		return false
	}
	h := c.byLeader[leader]
	h.pending = false
	c.byLeader[leader] = h
	return true
}

// held returns the leader bot holds and when it claimed it.
func (c *PartyClaims) held(bot int32) (string, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	leader, ok := c.byBot[bot]
	if !ok {
		return "", time.Time{}, false
	}
	return leader, c.byLeader[leader].at, true
}

// listTurn reports whether a bot may ask for the party-matching rooms at
// now, and takes the turn: at most one list per roomListGap across the
// bots of the process. A nil table gives no turn.
func (c *PartyClaims) listTurn(now time.Time) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if d := now.Sub(c.listed); !c.listed.IsZero() && d >= 0 && d < roomListGap {
		return false
	}
	c.listed = now
	return true
}

// release gives back the leader bot holds, if any.
func (c *PartyClaims) release(bot int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if leader, ok := c.byBot[bot]; ok {
		delete(c.byLeader, leader)
		delete(c.byBot, bot)
	}
}

// tidyPartyClaim gives back a claim the server did not answer with a party
// window within partyInviteTimeout, so that no claim outlives its
// invitation: the invitation lapsed, or the server refused the join (a busy
// inviter, a full party). A claim the bot joined a party with is kept until
// that party ends (partyClaimEvent).
func (b *Brain) tidyPartyClaim(now time.Time) {
	c, self := b.cfg.PartyClaims, b.world.Self.ObjectID
	if c == nil || b.world.Party.In() {
		return
	}
	// A room join waits RoomPolicy.Wait for its invitation and gives its
	// claim back itself (ruleRooms).
	if leader, at, ok := c.held(self); ok && leader != b.rooms.asked.leader && now.Sub(at) >= partyInviteTimeout {
		c.release(self)
	}
}

// partyClaimEvent gives the claim back on PartySmallWindowDeleteAll: the bot
// left its party, the party ended, or its leader changed. The server sends a
// leader change as PartySmallWindowDeleteAll then PartySmallWindowAll to
// every member (party Registry.changeLeader), so a claim never follows a new
// leader: the bot stays in the party unclaimed unless its handover step
// claims the new leader (handoverToPlayer). A pending claim (claimPending)
// is kept, its mark dropped: the window answers the bot's own withdrawal
// (party/party.go remove), and the claim lapses in tidyPartyClaim unless
// the player's invitation comes.
func (b *Brain) partyClaimEvent(ev Event) {
	if _, ok := ev.(PartySmallWindowDeleteAll); !ok {
		return
	}
	if c := b.cfg.PartyClaims; c != nil && c.settle(b.world.Self.ObjectID) {
		return
	}
	b.releasePartyClaim()
}

// releasePartyClaim gives back the leader the bot holds, if any: when its
// party ends or its leader changes, and when the brain stops.
func (b *Brain) releasePartyClaim() {
	if c := b.cfg.PartyClaims; c != nil {
		c.release(b.world.Self.ObjectID)
	}
}
