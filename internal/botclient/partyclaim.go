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
}

// partyClaim is the bot holding a leader, and when it last claimed it.
type partyClaim struct {
	bot int32
	at  time.Time
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
	c.mu.Lock()
	defer c.mu.Unlock()
	if h, ok := c.byLeader[leader]; ok && h.bot != bot {
		return false
	}
	if old, ok := c.byBot[bot]; ok && old != leader {
		delete(c.byLeader, old)
	}
	c.byLeader[leader] = partyClaim{bot: bot, at: now}
	c.byBot[bot] = leader
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
	if _, at, ok := c.held(self); ok && now.Sub(at) >= partyInviteTimeout {
		c.release(self)
	}
}

// partyClaimEvent gives the claim back on PartySmallWindowDeleteAll: the bot
// left its party, the party ended, or its leader changed. The server sends a
// leader change as PartySmallWindowDeleteAll then PartySmallWindowAll to
// every member (party Registry.changeLeader), so a claim never follows a new
// leader: the bot stays in the party unclaimed, and the new leader is free
// for another bot.
func (b *Brain) partyClaimEvent(ev Event) {
	if _, ok := ev.(PartySmallWindowDeleteAll); ok {
		b.releasePartyClaim()
	}
}

// releasePartyClaim gives back the leader the bot holds, if any: when its
// party ends or its leader changes, and when the brain stops.
func (b *Brain) releasePartyClaim() {
	if c := b.cfg.PartyClaims; c != nil {
		c.release(b.world.Self.ObjectID)
	}
}
