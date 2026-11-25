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
}

// TownPersona is a bot that lives in town (docs/plan.md, town persona): the
// default persona's errands (buying potions and shots, selling loot to
// merchants, the trainer, the Newbie Helper's buffs at levels 8-24), more
// idle drift and emotes in between, and now and then a gatekeeper hop to
// another town. It farms only where its brain has a spot registry; a town
// swarm runs without one.
func TownPersona() Persona {
	p := DefaultPersona()
	p.IdleChance = 0.3
	p.IdleRadius = 400
	// greet, yes, bow, waiting, laugh, applaud, dance
	p.Emotes = []int32{2, 6, 7, 9, 10, 11, 12}
	p.Town = TownPolicy{HopMin: 10 * time.Minute, HopMax: 30 * time.Minute, MaxPrice: 50000, Reach: 8000}
	return p
}

// hopSpell is a random idle spell between town hops.
func (b *Brain) hopSpell() time.Duration {
	p := &b.persona.Town
	if p.HopMax <= p.HopMin {
		return p.HopMin
	}
	return p.HopMin + time.Duration(b.rng.Int64N(int64(p.HopMax-p.HopMin)))
}

// planHop plans a town hop when one is due, reporting whether it did: from
// the nearest gatekeeper in walking reach with a destination in another
// town at a price the bot carries and the persona allows, to one of those
// destinations at random. The idle spell to the next hop starts at the
// bot's first decision and again once the trip is over; a hop with no
// gatekeeper to take it waits a spell as well.
func (b *Brain) planHop(now time.Time) bool {
	m, tr, p := b.cfg.Travel, &b.trip, &b.persona.Town
	if m == nil || p.HopMax <= 0 {
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
