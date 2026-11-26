package botclient

import (
	"time"
)

// A bot party's gatekeeper trip (docs/parties/spots.md, "Gatekeeper trip"):
// the leader plans it as a solo bot does, holds the destination's link at
// the gatekeeper for its bot members behind (the gather gate), lands, and
// holds at the landing until they come (regrouping). There is no party
// teleport on the server and bots of a bot party never whisper each other
// (D29): each member reads its leader's TeleportToLocation, which the
// server shows every player in sight of the jumper (restartpoint.go
// relocateLivePlayer), takes the same destination of the gatekeeper the
// leader stood at, and pays its own fee (gatekeeper.go Service.Teleport,
// pay). A member short of the fee, or refused, leaves the party; one that
// saw no such teleport answers it as today (members.md, "Events").

const (
	// partyTripGateRange is how far from a gatekeeper of the travel map the
	// leader may have stood, as the member saw it, when it jumped for that
	// jump to be that gatekeeper's teleport: the leader talks to it from
	// within the server's interaction distance.
	partyTripGateRange = 400
	// partyTripLandRange is how far from a destination of that gatekeeper
	// the leader may land: the server scatters a gatekeeper's landing by
	// gatekeeper.ScatterRadius (20) and moves it onto walkable ground.
	partyTripLandRange = 200
)

// partyTrip reports whether the bot is on its leader's gatekeeper trip as a
// member (travelTrip.follow): rules "leader" and "follow" leave it to rule
// "travel", and its supply trip does not answer the leader's teleport.
func (b *Brain) partyTrip() bool {
	return b.trip.follow && b.trip.v.active()
}

// partyTripEvent answers, ahead of the world's Apply, a join member's bot
// leader jumping by gatekeeper: the leader in sight within
// partyTripGateRange of a gatekeeper of the travel map, landing within
// partyTripLandRange of one of its destinations. A member carrying the fee
// (within its TravelPolicy.MaxPrice) and free of karma
// (KarmaPlayerCanUseGK) plans the same destination of the same gatekeeper;
// one that cannot leaves the party (leaveParty). Any other teleport of the
// leader is left to its loss (leaderEvent).
func (b *Brain) partyTripEvent(ev Event, now time.Time) {
	e, ok := ev.(TeleportToLocation)
	l, w := b.cfg.link, b.world
	if !ok || e.Fast || b.cfg.Travel == nil || l == nil || l.party.Kind != CommandJoin || !b.inBotParty() || b.leading() ||
		e.ObjectID != w.Party.LeaderID || b.trip.v.active() {
		return
	}
	if m := w.Party.Members[e.ObjectID]; m == nil || m.Name != slotLeader(l.party) {
		return
	}
	o := w.Objects[e.ObjectID]
	if o == nil {
		return
	}
	gi, d, ok := b.cfg.Travel.tripTaken(o.Position(now), e.Pos)
	if !ok {
		return
	}
	var budget int32
	if it, ok := w.Inventory.ByTemplate(adenaID); ok {
		budget = min(b.persona.Travel.MaxPrice, it.Count)
	}
	if d.price > budget || w.Self.Karma > 0 {
		b.leaveParty("cannot follow its leader's gatekeeper trip", d.price)
		return
	}
	g := &b.cfg.Travel.gatekeepers[gi]
	tr := &b.trip
	tr.v = npcVisit{npcID: g.npcID, at: g.at}
	tr.step, tr.gk, tr.dest, tr.onward, tr.follow = tripTalk, gi, d, 0, true
}

// tripTaken finds the gatekeeper trip a jump from from to to took: the
// nearest gatekeeper within partyTripGateRange of from with a destination
// within partyTripLandRange of to, by index, and that destination.
func (m *TravelMap) tripTaken(from, to Point) (int, destination, bool) {
	gate, land := int64(partyTripGateRange)*partyTripGateRange, int64(partyTripLandRange)*partyTripLandRange
	best, bestDist := -1, gate+1
	var dest destination
	for gi := range m.gatekeepers {
		g := &m.gatekeepers[gi]
		dg := horizontal2(from, g.at)
		if dg >= bestDist {
			continue
		}
		for _, d := range g.dests {
			if horizontal2(to, d.at) <= land {
				best, bestDist, dest = gi, dg, d
				break
			}
		}
	}
	return best, dest, best >= 0
}

// leaveParty takes a join member that cannot follow its leader's gatekeeper
// trip out of its bot party: it clears its slot (Status.PartyLeft "left",
// so the organizer drops it) and withdraws, and forgets its leader, so it
// neither whispers it nor casts /unstuck for it (D29); it plays solo.
func (b *Brain) leaveParty(why string, fee int32) {
	b.log.Info("party trip: leaving the party", "why", why, "fee", fee)
	b.clearSlot()
	b.lead = leaderWatch{}
}

// regrouping reports whether the leader, landed from a gatekeeper trip with
// bot members (travelTrip.regroupUntil), holds at the landing for them
// before it hunts: until every living bot member stands within GatherRange
// of it (memberAway), at most GatherWait from the landing.
func (b *Brain) regrouping(now time.Time) bool {
	tr := &b.trip
	if tr.regroupUntil.IsZero() {
		return false
	}
	if !now.Before(tr.regroupUntil) || !b.memberAway(now) {
		tr.regroupUntil = time.Time{}
		return false
	}
	return true
}

// memberAway reports whether a living bot member of the party the bot leads
// stands farther than GatherRange from it, where it is in sight, else where
// it was last reported (PartyMemberPosition), however far: a member still
// at the trip's start counts. One not reported yet counts too.
func (b *Brain) memberAway(now time.Time) bool {
	if !b.leadsBotParty() {
		return false
	}
	slot, from := b.cfg.link.party, b.world.Self.Position(now)
	r := int64(b.persona.Lead.withDefaults().GatherRange)
	for id, m := range b.world.Party.Members {
		if !slotNames(slot, m.Name) || m.HP <= 0 {
			continue
		}
		pos := m.Pos
		if o := b.world.Objects[id]; o != nil {
			if o.Dead {
				continue
			}
			pos = o.Position(now)
		} else if m.PosAt.IsZero() {
			return true
		}
		if dist2(from, pos) > r*r {
			return true
		}
	}
	return false
}
