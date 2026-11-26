package botclient

import (
	"math"
	"time"
)

// TownPolicy is how a town bot takes a gatekeeper to another town between
// idle spells (the town hop, planned by rule "travel"). Every value is a
// configurable default (D6).
type TownPolicy struct {
	// HopMin and HopMax bound the idle spell, from the bot's first
	// decision or the end of its last trip, before it hops; HopMax 0
	// never hops.
	HopMin, HopMax time.Duration
	// MaxPrice is the most adena one hop may cost.
	MaxPrice int32
	// Reach is how far the bot walks to the gatekeeper. The town persona's
	// reaches the gatekeeper of its village from any of the village's
	// merchants, as Shop.Reach and TrainerReach do: on Talking Island the
	// grocer stands 4202 from the gatekeeper (#441).
	Reach int32
	// StayMin and StayMax bound the town spell, FarmMin and FarmMax the
	// farming spell, of a town bot whose brain has a spot registry: it
	// rotates between the two (townRota, #618). StayMax 0 never rotates:
	// such a bot farms all the time where it has a spot registry, and
	// stays in town without one.
	StayMin, StayMax time.Duration
	FarmMin, FarmMax time.Duration
}

// townRota is a town bot's rotation between a town spell and a farming
// spell (#618). The town spell starts at the bot's first decision and
// whenever the bot is in town with its farming spell over; it claims no
// spot (ruleSpot), drifts around the nearest town (idleCenter) and may hop
// (planHop). The farming spell starts when the town spell ends: the bot
// farms as any farmer and takes no hop, and once the spell is over its
// supply trip takes it home (causeRotation). The brain goroutine owns it.
type townRota struct {
	// farm is set during the farming spell; until is when the spell under
	// way ends, zero before the first decision.
	farm  bool
	until time.Time
}

// TownPersona is a bot that lives in town (docs/plan.md, town persona): the
// default persona's errands (buying potions and shots, selling loot to
// merchants, the trainer, the Newbie Helper's buffs at levels 8-24), more
// idle drift and emotes in between, and now and then a gatekeeper hop to
// another town. Where its brain has a spot registry it rotates between
// town spells of 10-20 minutes and farming spells of 30-60 (townRota,
// #618); without one it stays in town.
func TownPersona() Persona {
	p := DefaultPersona()
	p.IdleChance = 0.3
	p.IdleRadius = 400
	// greet, yes, bow, waiting, laugh, applaud, dance
	p.Emotes = []int32{2, 6, 7, 9, 10, 11, 12}
	p.Town = TownPolicy{
		HopMin: 10 * time.Minute, HopMax: 30 * time.Minute, MaxPrice: 50000, Reach: 8000,
		StayMin: 10 * time.Minute, StayMax: 20 * time.Minute, FarmMin: 30 * time.Minute, FarmMax: 60 * time.Minute,
	}
	// A town bot stays in town: it takes a player's invitation, but asks
	// to enter no party-matching room (docs/parties/rooms.md).
	p.Rooms.Join = false
	return p
}

// idleCenter is where the idle drift from from stays around. A bot that
// lives in town (townLife) drifts around the travel map's town nearest it,
// so one that logged in out in the field walks to town on its first drift
// and an errand's end does not send it back (#567). Any other bot, or a
// town bot without a travel map, drifts around its anchor: where it
// logged in, arrived at its spot or landed from a teleport.
func (b *Brain) idleCenter(from Point) Point {
	if b.townLife() {
		if t := b.nearestTown(from); t != nil {
			return *t
		}
	}
	return b.anchor
}

// townLife reports whether the bot lives in town now: a persona that hops
// between towns with no farming spots, or a rotating one (rotates) in its
// town spell.
func (b *Brain) townLife() bool {
	if b.persona.Town.HopMax <= 0 {
		return false
	}
	return b.cfg.Spots == nil || b.rotates() && !b.rota.farm
}

// rotates reports whether the bot rotates between town and farming spells
// (townRota): a persona with town spells and a brain with a spot registry.
func (b *Brain) rotates() bool {
	return b.persona.Town.StayMax > 0 && b.cfg.Spots != nil
}

// rotate advances the rotation at now and reports whether the bot is in its
// town spell. The first call starts a town spell; a town spell over starts
// the farming spell; a farming spell over with the bot in town (its supply
// trip home, a death, a hop) starts the next town spell and gives up any
// spot still held. False for a bot that does not rotate.
func (b *Brain) rotate(now time.Time) bool {
	if !b.rotates() {
		return false
	}
	r, p := &b.rota, &b.persona.Town
	switch {
	case r.until.IsZero():
		r.farm, r.until = false, now.Add(b.spell(p.StayMin, p.StayMax))
	case !r.farm && !now.Before(r.until):
		r.farm, r.until = true, now.Add(b.spell(p.FarmMin, p.FarmMax))
	case r.farm && !now.Before(r.until) && b.inTown(now):
		r.farm, r.until = false, now.Add(b.spell(p.StayMin, p.StayMax))
		b.leaveSpot()
	}
	return !r.farm
}

// farmOver reports whether a rotating bot's farming spell is over at now:
// the supply trip's rotation cause (causeRotation).
func (b *Brain) farmOver(now time.Time) bool {
	return b.rotates() && b.rota.farm && !now.Before(b.rota.until)
}

// spell is a random spell between lo and hi.
func (b *Brain) spell(lo, hi time.Duration) time.Duration {
	if hi <= lo {
		return lo
	}
	return lo + time.Duration(b.rng.Int64N(int64(hi-lo)))
}

// hopSpell is a random idle spell between town hops.
func (b *Brain) hopSpell() time.Duration {
	p := &b.persona.Town
	return b.spell(p.HopMin, p.HopMax)
}

// planHop plans a town hop when one is due, reporting whether it did: from
// the nearest gatekeeper in walking reach with a destination in another
// town at a price the bot carries and the persona allows, to one of those
// destinations at random. The idle spell to the next hop starts at the
// bot's first decision and again once the trip is over; a hop with no
// gatekeeper to take it waits a spell as well. A rotating bot hops only in
// its town spells (townRota).
func (b *Brain) planHop(now time.Time) bool {
	m, tr, p := b.cfg.Travel, &b.trip, &b.persona.Town
	if m == nil || p.HopMax <= 0 || b.rotates() && b.rota.farm {
		// A rotating bot hops in its town spells only.
		return false
	}
	if tr.hopAt.IsZero() {
		tr.hopAt = now.Add(b.hopSpell())
		return false
	}
	if now.Before(tr.hopAt) {
		return false
	}
	tr.hopAt = time.Time{}
	var budget int32
	if it, ok := b.world.Inventory.ByTemplate(adenaID); ok {
		budget = min(p.MaxPrice, it.Count)
	}
	reach := int64(p.Reach) * int64(p.Reach)
	from := b.world.Self.Position(now)
	best, bestDist := -1, int64(math.MaxInt64)
	var bestDests []destination
	for gi := range m.gatekeepers {
		g := &m.gatekeepers[gi]
		d := dist2(from, g.at)
		if d > reach || d >= bestDist || now.Before(tr.shunned[gi]) {
			continue
		}
		if dests := m.hopDests(g, budget); len(dests) > 0 {
			best, bestDist, bestDests = gi, d, dests
		}
	}
	if best < 0 {
		tr.hopAt = now.Add(b.hopSpell())
		return false
	}
	g := &m.gatekeepers[best]
	tr.v = npcVisit{npcID: g.npcID, at: g.at}
	tr.step, tr.gk, tr.dest = tripTalk, best, bestDests[b.rng.IntN(len(bestDests))]
	return true
}

// hopDests are the destinations of gatekeeper g priced within budget that
// lie in another town: near a gatekeeper of the map (the towns of Towns)
// and away from g's own.
func (m *TravelMap) hopDests(g *gatekeeper, budget int32) []destination {
	var out []destination
	for _, d := range g.dests {
		if d.price > budget || dist2D(d.at, g.at) <= storeTownRadius {
			continue
		}
		for i := range m.gatekeepers {
			if dist2D(d.at, m.gatekeepers[i].at) <= storeTownRadius {
				out = append(out, d)
				break
			}
		}
	}
	return out
}
