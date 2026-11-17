package botclient

import (
	"math"
	"time"
)

// NPC visit timing, shared by the gatekeeper and Newbie Guide errands.
const (
	// visitArrive is how close to an NPC's spawn the bot walks looking for
	// it: there and still out of sight, the NPC is not there.
	visitArrive = 300
	// visitClicks is how many clicks (selections and talks) a visit spends
	// before it leaves the NPC alone for visitShunFor.
	visitClicks  = 6
	visitShunFor = 10 * time.Minute
)

// npcVisit walks to an NPC's spawn, then selects and talks to the NPC
// until its page opens: the way in of the gatekeeper and Newbie Guide
// errands. It is owned by the brain goroutine. The zero npcVisit is no
// visit.
type npcVisit struct {
	// npcID is the NPC template visited, standing at at.
	npcID int32
	at    Point
	// obj is the NPC's object id once it is in sight, 0 before.
	obj int32
	// opened is set when the NPC's page came back for the last click.
	opened bool
	// waitUntil, when set, is when the answer to the last click or bypass
	// of the visit is given up on.
	waitUntil time.Time
	clicks    int
	// lastDist is the distance left when the last leg went out, 0 before
	// one; stalls counts legs in a row that got nowhere.
	lastDist float64
	stalls   int
}

// active reports whether a visit runs.
func (v *npcVisit) active() bool { return v.npcID != 0 }

// sent records a packet of the visit that went out at now: a click or a
// bypass acting on the visited NPC.
func (v *npcVisit) sent(a Action, now time.Time) {
	if v.obj == 0 || a.object != v.obj {
		return
	}
	switch a.kind {
	case actSelect:
		v.clicks++
	case actTalk:
		v.clicks++
		v.opened, v.waitUntil = false, now.Add(trainerTalkTimeout)
	case actBypass:
		v.opened, v.waitUntil = false, now.Add(trainerReplyTimeout)
	}
}

// page moves the visit on with page ev, reporting whether it is the
// visited NPC's.
func (v *npcVisit) page(ev NpcHtmlMessage) bool {
	if v.obj == 0 || ev.ObjectID != v.obj {
		return false
	}
	v.opened, v.waitUntil = true, time.Time{}
	return true
}

// bypassTo sends bypass command cmd, a link of the page npc obj opened.
func bypassTo(obj int32, cmd string) Action {
	a := bypassAction(cmd)
	a.object = obj
	return a
}

// approach is the next packet of visit v short of the NPC's page: a leg
// toward its spawn while it is out of sight, then a selection and a talk.
// It reports false when the visit is over without a page: the NPC is not
// at its spawn, the legs got nowhere or the clicks ran out. The bot stands
// up first. Legs go through the frame pacer like the spot rule's; clicks are
// bounded by visitClicks.
func (b *Brain) approach(v *npcVisit, now time.Time) (Action, bool) {
	s := &b.world.Self
	if v.obj != 0 {
		if o := b.world.object(v.obj); o == nil || o.Dead {
			// Out of sight (a teleport clears the world): look again.
			v.obj, v.opened, v.waitUntil = 0, false, time.Time{}
		}
	}
	if v.obj == 0 {
		v.obj = b.nearestNPC(v.npcID, v.at, now)
	}
	if v.obj == 0 {
		return b.walkTo(v, now)
	}
	if !v.waitUntil.IsZero() && now.Before(v.waitUntil) {
		return Action{}, true
	}
	// Unanswered: the server refused in silence; click again.
	v.waitUntil = time.Time{}
	if v.clicks >= visitClicks {
		return Action{}, false
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	from := s.Position(now)
	if b.world.Target != v.obj {
		return selectAction(v.obj, from), true
	}
	return talkAction(v.obj, from), true
}

// walkTo is the next leg of visit v toward its NPC's spawn, the bot
// standing up first. It reports false once the bot is within visitArrive
// of the spawn, or when the legs got nowhere. Legs go through the frame
// pacer like the spot rule's.
func (b *Brain) walkTo(v *npcVisit, now time.Time) (Action, bool) {
	s := &b.world.Self
	from := s.Position(now)
	d := math.Sqrt(float64(dist2(from, v.at)))
	if d <= visitArrive {
		return Action{}, false
	}
	if s.Moving(now) {
		return Action{}, true
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	if v.lastDist > 0 && v.lastDist-d < spotProgress {
		v.stalls++
	} else {
		v.stalls = 0
	}
	if v.stalls >= spotStalls {
		return Action{}, false
	}
	v.lastDist = d
	return moveAction(legToward(from, v.at, d), from), true
}

// nearestNPC is the object id of the live NPC of template npcID in sight
// nearest to at, 0 when none is in sight.
func (b *Brain) nearestNPC(npcID int32, at Point, now time.Time) int32 {
	var best int32
	var bestDist int64
	for id, o := range b.world.Objects {
		if o.Kind != KindNPC || o.NPCID != npcID || o.Dead {
			continue
		}
		if d := dist2(at, o.Position(now)); best == 0 || d < bestDist {
			best, bestDist = id, d
		}
	}
	return best
}
