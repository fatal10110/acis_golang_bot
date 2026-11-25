package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// A leader's spot for its bot party (docs/parties/spots.md): it claims room
// for the whole party, fits spots to its bot members' levels, takes no
// gatekeeper trip and waits at its spot for far members. A bot that leads
// no bot party is a solo bot: weight 1, its own level, its trips.

// leadsBotParty reports whether the bot leads its bot party: its slot is a
// lead and the window names it leader.
func (b *Brain) leadsBotParty() bool {
	l := b.cfg.link
	return l != nil && l.party.Kind == CommandLead && b.leading()
}

// spotWeight is how many places of a spot the bot takes: its party
// window's size, players included, while it leads its bot party, else 1. A
// member's death, raise or trip leaves the window as it is, so only a
// window add or delete, the party's end or the slot's change weighs it anew
// (reweighSpot).
func (b *Brain) spotWeight() int {
	if !b.leadsBotParty() {
		return 1
	}
	return 1 + len(b.world.Party.Members)
}

// reweighSpot moves the count the bot holds or farms beside to its weight
// in one step, keeping the spot: on every event (a window add or delete,
// the party's end) and slot change. A held count weighed up may pass the
// spot's cap; the spot rule then looks for room for the party (overCap).
func (b *Brain) reweighSpot() {
	v := &b.spot
	if b.cfg.Spots == nil || !v.farming() {
		return
	}
	old, n := v.claimWeight(), b.spotWeight()
	if n == old {
		return
	}
	b.cfg.Spots.reweigh(v.idx, v.beside, old, n)
	v.weight = n
}

// botLevels are the lowest and highest levels of the bot and the bot
// members of the party it leads, the names its lead slot lists; ok false
// while it leads no bot member. A player member counts for none.
func (b *Brain) botLevels() (lo, hi int, ok bool) {
	if !b.leadsBotParty() {
		return 0, 0, false
	}
	slot := b.cfg.link.party
	lo, hi = int(b.world.Self.Level), int(b.world.Self.Level)
	for _, m := range b.world.Party.Members {
		if slotNames(slot, m.Name) {
			lo, hi, ok = min(lo, int(m.Level)), max(hi, int(m.Level)), true
		}
	}
	return lo, hi, ok
}

// fitBand is the levels a spot must fit (SpotPolicy.fitsBand): those of the
// bot members it leads (botLevels) while a spot of the map fits them, by
// level only; else the bot's own. band reports the members'.
func (b *Brain) fitBand() (lo, hi int, band bool) {
	own := int(b.world.Self.Level)
	lo, hi, ok := b.botLevels()
	if !ok {
		return own, own, false
	}
	r, p := b.cfg.Spots, &b.persona.Spots
	for i := range r.spots {
		if p.fitsBand(&r.spots[i], lo, hi) {
			return lo, hi, true
		}
	}
	return own, own, false
}

// misfit reports whether the spot the bot holds fails its members' band
// while a spot of the map fits it: the leader looks for one that does, as
// an outlevelled bot does. A spot farmed beside is no claim to keep.
func (b *Brain) misfit() bool {
	v := &b.spot
	if !v.held {
		return false
	}
	lo, hi, band := b.fitBand()
	return band && !b.persona.Spots.fitsBand(&b.cfg.Spots.spots[v.idx], lo, hi)
}

// leadsBotMembers reports whether the bot leads its bot party with a bot
// member in it: such a leader takes no gatekeeper trip, its members follow
// by walking only.
func (b *Brain) leadsBotMembers() bool {
	_, _, ok := b.botLevels()
	return ok
}

// gathering reports whether the leader at its spot holds its next farm
// target for a bot member behind (memberBehind): at most GatherWait from
// when the member fell behind.
func (b *Brain) gathering(now time.Time) bool {
	v, pol := &b.spot, b.persona.Lead.withDefaults()
	if !b.memberBehind(now, pol) {
		v.gatherSince = time.Time{}
		return false
	}
	if v.gatherSince.IsZero() {
		v.gatherSince = now
	}
	return now.Sub(v.gatherSince) < pol.GatherWait
}

// memberBehind reports whether a living bot member of the party the bot
// leads was last reported (PartyMemberPosition) farther from it than
// GatherRange and no farther than FarAway. A member with no report yet, or
// dead, holds nothing.
func (b *Brain) memberBehind(now time.Time, pol LeadPolicy) bool {
	if !b.leadsBotParty() {
		return false
	}
	slot, from := b.cfg.link.party, b.world.Self.Position(now)
	near, far := int64(pol.GatherRange)*int64(pol.GatherRange), int64(pol.FarAway)*int64(pol.FarAway)
	for id, m := range b.world.Party.Members {
		if !slotNames(slot, m.Name) || m.PosAt.IsZero() || m.HP <= 0 {
			continue
		}
		if o := b.world.Objects[id]; o != nil && o.Dead {
			continue
		}
		if d := dist2(from, m.Pos); d > near && d <= far {
			return true
		}
	}
	return false
}

// fitsBand reports whether s fits bots of levels lo to hi: Below and
// AggroAbove (Newbie) against lo, Above against hi. fits is a band of one.
func (p *SpotPolicy) fitsBand(s *datapack.Spot, lo, hi int) bool {
	aggro := p.AggroAbove
	if lo <= p.Newbie {
		aggro = max(aggro, p.Below)
	}
	return s.MaxLevel <= lo+p.Below && s.AggroLevel <= lo+aggro && s.MaxLevel >= hi-p.Above
}
