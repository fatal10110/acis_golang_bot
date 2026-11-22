package botclient

import (
	"math"
	"time"
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
	// lastDist is the distance left when the last leg went out, 0 before
	// one, and legFrom where the bot stood then; stalls counts legs in a
	// row that got nowhere.
	lastDist float64
	legFrom  Point
	stalls   int
	// unsticks counts the tries at leaving a spot the legs stall in (see
	// unstick), stuckAt is where the bot stood for the last one and escapeAt
	// is when the visit's /unstuck went out, zero before.
	unsticks int
	stuckAt  Point
	escapeAt time.Time
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
			// Out of sight (a teleport clears the world): look again. The
			// stalls counted toward the NPC say nothing of the legs toward
			// its spawn.
			v.obj, v.opened, v.waitUntil = 0, false, time.Time{}
			v.lastDist, v.stalls = 0, 0
		}
	}
	if v.obj == 0 {
		if v.obj = b.nearestNPC(v.npcID, v.at, now); v.obj != 0 {
			v.lastDist, v.stalls = 0, 0
		}
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
	return talkAction(v.obj, from), true
}

// walkTo is the next leg of visit v toward its NPC's spawn, the bot
// standing up first. It reports false once the bot is within visitArrive
// of the spawn, once visitLook passed since the bot appeared, or when the
// legs got nowhere: legs that stall where the bot stands try an unstick
// first. Legs go through the frame pacer like the spot rule's.
func (b *Brain) walkTo(v *npcVisit, now time.Time) (Action, bool) {
	s := &b.world.Self
	from := s.Position(now)
	d := math.Sqrt(float64(dist2(from, v.at)))
	if d <= visitArrive {
		if now.Before(b.appearedAt.Add(visitLook)) {
			return Action{}, true
		}
		return Action{}, false
	}
	if s.Moving(now) {
		return Action{}, true
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	if v.stalls >= spotStalls {
		if !budged(from, v.stuckAt) {
			return b.unstick(v, from, d, now)
		}
		// An unstick moved the bot: legs again from where it stands.
		v.lastDist, v.stalls = 0, 0
	}
	if v.lastDist > 0 && v.lastDist-d < spotProgress {
		v.stalls++
	} else {
		v.stalls = 0
	}
	if v.stalls >= spotStalls {
		if budged(from, v.legFrom) {
			// The bot walks, just no closer: no way there.
			return Action{}, false
		}
		return b.unstick(v, from, d, now)
	}
	v.lastDist, v.legFrom = d, from
	return moveAction(legToward(from, v.at, d), from), true
}

// walkStuck reports whether visit v, over without a page, ended on a walk
// toward its NPC's spawn that got nowhere (walkTo): the NPC out of sight and
// the bot farther than visitArrive from the spawn. The errand backs off its
// walks then, not just that NPC: the other spawns lie past the same ground,
// and the recall of an /unstuck can land the bot back where it stood (a new
// human mystic's start is its own restart area, whose ground moves the
// pathfinder cannot route out of: acis_golang#5412), so a walk to the next
// spawn would only spend another /unstuck there (#134, #135).
func (b *Brain) walkStuck(v *npcVisit, now time.Time) bool {
	return v.obj == 0 && dist2(b.world.Self.Position(now), v.at) > visitArrive*visitArrive
}

// npcAt reports whether an NPC of template npcID stands in sight within
// visitArrive of at: a visit to it walks no leg out of sight.
func (b *Brain) npcAt(npcID int32, at Point, now time.Time) bool {
	id := b.nearestNPC(npcID, at, now)
	return id != 0 && dist2(b.world.object(id).Position(now), at) <= visitArrive*visitArrive
}

// closeIn is the next leg toward visit v's NPC, in sight but farther than
// arrive (visitArrive for an NPC): the server walks the approach of a click straight at the
// NPC and ends it at the first wall (PlayerMove.moveToPawn; acis_golang
// move.CreatureMove.MoveToPawnWithPathOutcome), while it routes a ground
// move around obstacles. It reports false once the bot is close enough to
// click, or when spotStalls legs in a row got nowhere: the clicks are tried
// then. Legs go through the frame pacer like the spot rule's.
func (b *Brain) closeIn(v *npcVisit, arrive float64, now time.Time) (Action, bool) {
	s := &b.world.Self
	from := s.Position(now)
	to := b.world.object(v.obj).Position(now)
	d := math.Sqrt(float64(dist2(from, to)))
	if d <= arrive || v.stalls >= spotStalls {
		return Action{}, false
	}
	if s.Moving(now) {
		return Action{}, true
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	if v.lastDist > 0 && v.lastDist-d < spotProgress {
		if v.stalls++; v.stalls >= spotStalls {
			return Action{}, false
		}
	} else {
		v.stalls = 0
	}
	v.lastDist = d
	return moveAction(legToward(from, to, d), from), true
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
