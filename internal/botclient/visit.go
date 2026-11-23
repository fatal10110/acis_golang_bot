package botclient

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// NPC visit timing, shared by the gatekeeper and Newbie Guide errands.
const (
	// visitArrive is how close to an NPC's spawn the bot walks looking for
	// it: there and still out of sight, the NPC is not there. It is also
	// how close to an NPC in sight the bot walks before it clicks.
	visitArrive = 300
	// visitClicks is how many clicks (selections and talks) a visit spends
	// before it leaves the NPC alone for visitShunFor.
	visitClicks  = 6
	visitShunFor = 10 * time.Minute
	// visitLook is how long after the bot appeared at a landing point a
	// visit there waits for the NPC to come in sight before it calls the
	// NPC absent: a loading-screen teleport empties the world, and the
	// server shows the objects around the landing point only once the
	// client reports Appearing.
	visitLook = 3 * time.Second
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
	// stand is the stand point the visit walks to before it clicks the NPC
	// in sight, standFor where the NPC stood when it was chosen; both zero
	// before one.
	stand, standFor Point
	// noLine is set when the stand point offers no open line to the NPC
	// (a talk point).
	noLine bool
	// failedAt is the walker's endedAt of the last unreachable walk to a
	// talk point a click was spent on; zero before one.
	failedAt time.Time
	// walkAt is where the walk toward the spawn heads (walkTo): at, or the
	// bot's ground nearest it; zero before the walk.
	walkAt Point
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

// pickVisit is a visit to the nearest NPC of the templates npcs in sight,
// else to the nearest of their spawns within reach (from the travel map),
// reporting whether there is one. A party member stays with its party and
// a bot told to follow stays with its commander: neither walks to an NPC
// out of sight.
func (b *Brain) pickVisit(npcs []int32, reach int32, now time.Time) (npcVisit, bool) {
	from := b.world.Self.Position(now)
	var best npcVisit
	bestDist := int64(math.MaxInt64)
	for _, npcID := range npcs {
		if id := b.nearestNPC(npcID, from, now); id != 0 {
			at := b.world.Objects[id].Position(now)
			if d := dist2(from, at); d < bestDist || best.obj == 0 {
				best, bestDist = npcVisit{npcID: npcID, at: at, obj: id}, d
			}
		}
	}
	if !best.active() && b.cfg.Travel != nil && !b.world.Party.In() && b.orders.follow == 0 {
		r := int64(reach) * int64(reach)
		for _, npcID := range npcs {
			for _, at := range b.cfg.Travel.spawns[npcID] {
				if d := dist2(from, at); d <= r && d < bestDist {
					best, bestDist = npcVisit{npcID: npcID, at: at}, d
				}
			}
		}
	}
	return best, best.active()
}

// bypassTo sends bypass command cmd, a link of the page npc obj opened.
func bypassTo(obj int32, cmd string) Action {
	a := bypassAction(cmd)
	a.object = obj
	return a
}

// approach is the next packet of visit v short of the NPC's page: a walk
// toward its spawn while it is out of sight, then a walk to a stand point
// with an open straight line to it when there is none yet, then a selection
// and a talk. It reports false when the visit is over without a page: the
// NPC is not at its spawn, the walk there is unreachable or the clicks ran
// out. The bot stands up first. Walks go through the walker (goTo); clicks
// are bounded by visitClicks.
func (b *Brain) approach(v *npcVisit, now time.Time) (Action, bool) {
	s := &b.world.Self
	if v.obj != 0 {
		if o := b.world.object(v.obj); o == nil || o.Dead {
			// Out of sight (a teleport clears the world): look again.
			v.obj, v.opened, v.waitUntil = 0, false, time.Time{}
			v.stand, v.standFor, v.noLine, v.failedAt = Point{}, Point{}, false, time.Time{}
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
	if a, ok := b.closeIn(v, visitArrive, now); ok {
		return a, true
	}
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
	b.walk.talking(b.world.object(v.obj).Position(now), now)
	return talkAction(v.obj, from), true
}

// walkTo is the next segment of visit v toward its NPC's spawn, through the
// walker. It reports false once the bot is within visitArrive of the spawn
// and visitLook passed since the bot appeared (the NPC is not there), or
// when the walk there is unreachable. A spawn off the bot's ground, in a
// pocket of a stall or counter no walk enters, is walked to by the bot's
// ground nearest it within talkReach, where a click still talks (#253).
func (b *Brain) walkTo(v *npcVisit, now time.Time) (Action, bool) {
	if v.walkAt == (Point{}) {
		v.walkAt = v.at
		if g, ok := b.walker().groundNear(b.world.Self.Position(now), v.at, talkReach); ok && horizontal2(g, v.at) > walkSlop*walkSlop {
			v.walkAt = g
		}
	}
	a, st := b.goTo(v.walkAt, visitArrive, walkPlace, now)
	switch st {
	case walkUnreachable:
		return Action{}, false
	case walkArrived:
		if now.Before(b.appearedAt.Add(visitLook)) {
			return Action{}, true
		}
		return Action{}, false
	}
	return a, true
}

// visitTalkRange is how far from an NPC a visit's stand point may lie: the
// server's interaction distance (network/targeting.go interactionDistance).
const visitTalkRange = 150

// talkReach is how far from an NPC a stand point with no open line to it
// may lie: the server's interaction distance less the walk's slop at the
// stand point and the approach's last position update.
const talkReach = visitTalkRange - 2*walkSlop

// closeIn is the next segment toward a stand point of visit v's NPC in
// sight. A click on the NPC walks the bot straight at it, never routed, and
// a closed line ends that walk blocked (creature.go, MoveToPawn), so the bot
// clicks only from within arrive with the straight line open, and else walks
// first, through the walker, to the stand point nearest it within
// visitTalkRange with an open line (nav StandLine), or with none, to its
// talk point (walker.talkPoint). It reports false once the bot may click, or
// when no stand point or walk there holds: the clicks are tried then,
// bounded by visitClicks. A walk to a talk point that ends unreachable is
// not clicked past, since the straight approach from afar ends blocked: the
// visit waits out the walker's walkRetry and walks again, each failed walk
// (each unreachable end, by the walker's endedAt) spending a click.
func (b *Brain) closeIn(v *npcVisit, arrive float64, now time.Time) (Action, bool) {
	s := &b.world.Self
	from := s.Position(now)
	to := b.world.object(v.obj).Position(now)
	w := b.walker()
	if !w.talkFailed && float64(dist2(from, to)) <= arrive*arrive && w.lineOpen(from, to) {
		return Action{}, false
	}
	if w.talkFailed || v.stand == (Point{}) || horizontal2(to, v.standFor) > walkSameDest*walkSameDest {
		// A new stand point: none yet, the NPC moved, or the last click from
		// the old one ended blocked short of the NPC.
		w.talkFailed = false
		st, ok := w.standPoint(from, to, visitTalkRange, nav.StandLine, 0)
		v.noLine = !ok
		if !ok {
			// No open line: a click from within talkReach ends blocked
			// short of the NPC and still talks; one from further away
			// ends blocked across the town (#253).
			if st, ok = w.talkPoint(from, to, talkReach); !ok {
				return Action{}, false
			}
		}
		v.stand, v.standFor = st, to
	}
	a, st := b.goTo(v.stand, walkSlop, walkTalk, now)
	switch {
	case st == walkUnreachable && v.noLine:
		// A retry whose plan fails at once ends within the same goTo, with
		// no walking in between: its fresh endedAt still spends a click.
		if ended := b.walk.endedAt; !ended.Equal(v.failedAt) {
			v.failedAt = ended
			v.clicks++
		}
		return Action{}, v.clicks < visitClicks
	case st != walkWalking:
		return Action{}, false
	}
	return a, true
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
