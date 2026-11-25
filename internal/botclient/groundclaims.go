package botclient

import (
	"maps"
	"sync"
	"time"
)

// lootClaimFor is how long a bot's claim on a ground item outlives the last
// decision that renewed it (GroundClaims.claim): a bot that stops looting,
// logs out or dies frees its item for the others this long after.
const lootClaimFor = 15 * time.Second

// GroundClaims are the ground items the bots of a process are after, so no
// two of them chase one item (#435); every bot of the process shares one.
// Bots of other processes are left to the server, which gives a contested
// item to the first pick-up (network/pickup.go, ground.Claim) and answers
// the others with ActionFailed. mu owns claims.
type GroundClaims struct {
	mu     sync.Mutex
	claims map[int32]groundClaim
}

// groundClaim is bot's claim on an item, held until until.
type groundClaim struct {
	bot   int32
	until time.Time
}

// NewGroundClaims returns an empty claim table.
func NewGroundClaims() *GroundClaims {
	return &GroundClaims{claims: make(map[int32]groundClaim)}
}

// claim gives ground item id to bot until lootClaimFor after now, renewing
// a claim bot holds, and reports whether bot holds it: false while another
// bot's claim on it runs. Lapsed claims are dropped here, which bounds the
// table by the items the bots are after. A nil table gives every claim.
func (c *GroundClaims) claim(id, bot int32, now time.Time) bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	maps.DeleteFunc(c.claims, func(_ int32, g groundClaim) bool { return !now.Before(g.until) })
	if g, ok := c.claims[id]; ok && g.bot != bot {
		return false
	}
	c.claims[id] = groundClaim{bot: bot, until: now.Add(lootClaimFor)}
	return true
}

// release gives back bot's claim on item id, if it holds one.
func (c *GroundClaims) release(id, bot int32) {
	if c == nil || id == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if g, ok := c.claims[id]; ok && g.bot == bot {
		delete(c.claims, id)
	}
}

// drop ends every claim on item id: it left the ground.
func (c *GroundClaims) drop(id int32) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.claims, id)
}
