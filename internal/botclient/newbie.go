package botclient

import (
	"math"
	"time"
)

// supportMagicBypass is the Newbie Guide page's link that asks for the
// newbie buffs (data/html/script/feature/NewbieHelper/30598.htm; the
// NewbieHelper script's SupportMagic event).
const supportMagicBypass = "Quest NewbieHelper SupportMagic"

// newbieGuides are the NPC template ids whose first page has the buff
// link: the Newbie Guides of the five starting villages (30598-30602) and
// the two of 31076 and 31077 (newbiehelper.go onFirstTalk). The guides of
// the other villages (30009, 30019, ...) show pages without it.
var newbieGuides = []int32{30598, 30599, 30600, 30601, 30602, 31076, 31077}

// NewbiePolicy is when a bot asks a Newbie Guide for the newbie buffs (rule
// "newbie"). Every value is a configurable default (D6).
type NewbiePolicy struct {
	// MinLevel and MaxLevel bound the levels at which the bot asks, once
	// per level; MaxLevel 0 never asks. The server casts the buffs that
	// fit the level and answers a page for a bot it does not buff.
	MinLevel, MaxLevel int32
	// Guides are the NPC template ids of the guides the bot asks.
	Guides []int32
	// Reach is how far the bot walks to a guide's spawn (from the travel
	// map, BrainConfig.Travel); a guide in sight is asked from anywhere,
	// and 0 asks only one in sight.
	Reach int32
}

// defaultNewbiePolicy asks for the buffs at levels 8-24 (docs/plan.md, town
// persona), walking to a guide nearby.
func defaultNewbiePolicy() NewbiePolicy {
	return NewbiePolicy{MinLevel: 8, MaxLevel: 24, Guides: newbieGuides, Reach: 8000}
}

// newbieVisit is the bot's errand at a Newbie Guide (rule "newbie"). It is
// owned by the brain goroutine.
type newbieVisit struct {
	v npcVisit
	// doneLevel is the level the bot last asked at, 0 before it asked.
	doneLevel int32
	// shunned are the guides, by template id, that buffed the bot nothing,
	// until a time.
	shunned map[int32]time.Time
}

// shun ends the visit and leaves its guide alone for visitShunFor.
func (n *newbieVisit) shun(now time.Time) {
	if n.shunned == nil {
		n.shunned = make(map[int32]time.Time)
	}
	n.shunned[n.v.npcID] = now.Add(visitShunFor)
	n.v = npcVisit{}
}

// sent records a packet of the visit that went out at now: the buff link
// ends the visit for level.
func (n *newbieVisit) sent(a Action, now time.Time, level int32) {
	if !n.v.active() {
		return
	}
	if a.kind == actBypass && a.object != 0 && a.object == n.v.obj {
		n.doneLevel, n.v = level, npcVisit{}
		return
	}
	n.v.sent(a, now)
}

// ruleNewbie is a rule 9 errand: once per level in the persona's window,
// the bot walks to a Newbie Guide, talks to it and sends its buff link;
// the server casts the buffs. A guide whose page has no buff link, or that
// is not at its spawn or that no route reaches, is shunned.
// The bypass passes the bypass gate
// (gateBypass); legs and clicks the frame pacer, a visit spending at most
// visitClicks clicks.
func (b *Brain) ruleNewbie(now time.Time) (Action, bool) {
	n := &b.newbie
	if b.world.Self.InCombat || (!n.v.active() && !b.startNewbie(now)) {
		return Action{}, false
	}
	v := &n.v
	if v.opened {
		if b.world.Dialog.Has(supportMagicBypass) {
			return bypassTo(v.obj, supportMagicBypass), true
		}
		// The guide's page has no buff link.
		n.shun(now)
		return Action{}, false
	}
	a, ok := b.approach(v, now)
	if !ok {
		n.shun(now)
		return Action{}, false
	}
	return a, true
}

// startNewbie starts a visit when one is due: at the nearest guide in
// sight, else at the nearest guide spawn within Reach unless a walk got
// nowhere at this level, reporting whether it did.
func (b *Brain) startNewbie(now time.Time) bool {
	n, s, p := &b.newbie, &b.world.Self, &b.persona.Newbie
	if p.MaxLevel == 0 || s.Level < p.MinLevel || s.Level > p.MaxLevel || s.Level == n.doneLevel {
		return false
	}
	from := s.Position(now)
	var best npcVisit
	bestDist := int64(math.MaxInt64)
	for _, npcID := range p.Guides {
		if now.Before(n.shunned[npcID]) {
			continue
		}
		if id := b.nearestNPC(npcID, from, now); id != 0 {
			at := b.world.Objects[id].Position(now)
			if d := dist2(from, at); d < bestDist || best.obj == 0 {
				best, bestDist = npcVisit{npcID: npcID, at: at, obj: id}, d
			}
			continue
		}
		if b.cfg.Travel == nil || best.obj != 0 {
			continue
		}
		reach := int64(p.Reach) * int64(p.Reach)
		for _, at := range b.cfg.Travel.spawns[npcID] {
			if d := dist2(from, at); d <= reach && d < bestDist {
				best, bestDist = npcVisit{npcID: npcID, at: at}, d
			}
		}
	}
	if !best.active() {
		return false
	}
	n.v = best
	return true
}

// newbiePage moves the visit on with the guide's page.
func (b *Brain) newbiePage(ev NpcHtmlMessage) {
	if b.newbie.v.active() {
		b.newbie.v.page(ev)
	}
}
