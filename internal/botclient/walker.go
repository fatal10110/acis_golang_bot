package botclient

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// Nav is the navigation data every bot of the process shares
// (docs/navigation.md, D13): the geodata engine, where the door table toggles
// the doors, the hierarchy over the nav cache, and the door table. All three
// are safe for concurrent use; each brain builds its own planner over them.
type Nav struct {
	Engine *engine.Engine
	Hier   *nav.Hierarchy
	// Doors is the door table; nil takes every door as open.
	Doors *DoorTable
	// Water is the water zones the nav cache was built with, which every
	// walker's planner keeps its segments out of; nil is none.
	Water *nav.Water
}

// closed is the door state a route search reads, nil without a door table.
func (n *Nav) closed() func(door int) bool {
	if n.Doors == nil {
		return nil
	}
	return n.Doors.Closed
}

// Walker timing and limits (docs/navigation.md, "Walker").
const (
	// navEmax bounds the gap between two of the server's position updates:
	// N0 measured 141 ms at most on the live server, and the walker takes a
	// margin over it. Lmax, the longest step after the first, is the run
	// speed times navEmax.
	navEmax = 200 * time.Millisecond
	// positionTick is the server's position update interval: the walker
	// waits one past a segment's simulated arrival before the next goes out.
	positionTick = 100 * time.Millisecond
	// walkAnswerWait is how long a segment waits for the server's
	// MoveToLocation before the watchdog counts it as failed (a refused
	// move gets ActionFailed and no MoveToLocation).
	walkAnswerWait = 2 * time.Second
	// walkSlack is the time a segment may run past its simulated arrival
	// before the watchdog counts it as failed.
	walkSlack = 2 * time.Second
	// walkMaxBlocked, walkMaxDesync and walkMaxWatchdog are the failures of
	// each kind one route takes before it ends unreachable; walkMaxDoor
	// bounds the door replans, which never end it as blocked or desync.
	walkMaxBlocked  = 3
	walkMaxDesync   = 3
	walkMaxWatchdog = 3
	walkMaxDoor     = 8
	// walkBudget caps the cells one route search expands: the budget the
	// water measurement of #161 routed up to 8000 units with. The 6800
	// units from the human start point to Talking Island's trainers take
	// more than 60000.
	walkBudget = 400000
	// swimBreath is how long one stretch of a route may swim: half the
	// shortest base breath, 60 s times the dwarf's 0.8 (acis_golang
	// network/taskeffects.go breathTime, model/actor/player/race.go
	// BreathMultiplier). The bot cannot see a Breath stat change, and with
	// the server's AllowWater off it gets no countdown, so the margin is
	// wide (docs/navigation.md, "Swimming").
	swimBreath = 24 * time.Second
	// walkSwimNear is how close to the box a walk spans a water zone must
	// lie for a walk the hierarchy cannot route to try the swimming cell
	// planner: the hierarchy keeps out of water.
	walkSwimNear = 1000
	// walkRetry is how long a walk that ended unreachable stays so for the
	// same destination and purpose: a consumer asking again after it
	// gives up gets a fresh walk.
	walkRetry = time.Minute
	// talkWait bounds how long a talk click's answer is waited for: the
	// straight approach of a click from within visitArrive.
	talkWait = 10 * time.Second
	// walkSameDest is how far a consumer's destination may move without
	// a new route: the server rounds, a stand point snaps.
	walkSameDest = 32
	// walkSlop is how far from a segment's end the server may answer or
	// stop and still count as that end: one geodata cell.
	walkSlop = 16
	// walkDropCells is how far around one-way-out ground, in cells, the
	// walker looks for the abstract SCC it drops into.
	walkDropCells = 32
	// escapeEvery spaces the walker's /unstuck on isolated ground.
	escapeEvery = 10 * time.Minute
	// defaultCollisionHeight is a walker's collision height before its
	// UserInfo reports one.
	defaultCollisionHeight = 23
)

// walkStatus is where a walk stands after a decision.
type walkStatus uint8

const (
	walkWalking walkStatus = iota
	walkArrived
	walkUnreachable
)

// Unreachable reasons a walk ends with, shown in the status snapshot.
const (
	reasonBlocked  = "blocked"
	reasonDesync   = "desync"
	reasonDoor     = "door"
	reasonWatchdog = "watchdog"
	reasonNoRoute  = "no route"
	reasonBudget   = "route budget"
	reasonIsolated = "isolated"
	reasonNoNav    = "no nav"
)

// walkPurpose is what a walk is for.
type walkPurpose uint8

const (
	walkPlace walkPurpose = iota
	walkTalk
	walkMelee
	walkPickup
	walkRanged
	// walkFollow is a follower's walk to its slot behind the leader, and
	// back to the leader after a death; walkFlee and walkIdle are a run away
	// from an attacker and an idle drift.
	walkFollow
	walkFlee
	walkIdle
)

// drops reports whether a walk for p may take a one-way drop toward its
// destination: only following the leader does (docs/navigation.md,
// "Queries"), and never down to isolated ground only /unstuck leaves.
func (p walkPurpose) drops() bool { return p == walkFollow }

// segment is one move the walker sent: from the server-confirmed standstill
// from to to, both on their floor.
type segment struct {
	from, to Point
	sentAt   time.Time
	// answered is set by the server's MoveToLocation for it; eta is the
	// simulated arrival from then and deadline the watchdog's limit.
	answered      bool
	eta, deadline time.Time
	door          bool
}

// walker walks the bot along routes it checked on its own geodata
// (docs/navigation.md, "Walker"; D13). It is owned by the brain goroutine.
// The zero walker has no geodata: every walk is unreachable (reasonNoNav),
// which only non-walking personas run with (D13a).
type walker struct {
	nav  *Nav
	plan nav.Planner
	pins nav.Pins

	// dest, arrive and purpose are the walk asked for; status and reason
	// where it stands.
	dest    Point
	arrive  int32
	purpose walkPurpose
	status  walkStatus
	reason  string
	// endedAt is when a decision first saw the walk unreachable.
	endedAt time.Time
	// route is the rest of the route: route[0] is the next segment's end.
	route []Point
	// next is the segment the last decision returned, seg the one in
	// flight (seg.sentAt zero for none).
	next, seg segment
	// floor is the standstill the last segment arrived at, on its floor:
	// the server's answer carries the click's height plus the collision
	// height (network/movement.go moveLivePlayer).
	floor Point
	// The failures of the route so far.
	blocked, desync, watchdog, doors int
	// talkTo is where the NPC stood when the bot's last talk click went
	// out, talkAt when (zero for none pending); talkFailed is set when the
	// server's blocked correction ended that click's straight approach short
	// of talking range, so the visit walks to a new stand point.
	talkTo     Point
	talkAt     time.Time
	talkFailed bool
	// escaping is set from the /unstuck the walker asked until its landing;
	// escapeAt is when the last went out.
	escaping bool
	escapeAt time.Time
}

func newWalker(n *Nav) walker {
	w := walker{nav: n}
	if n != nil {
		w.plan = nav.Planner{Engine: n.Engine, Budget: walkBudget, Water: n.Water}
	}
	return w
}

// reset starts a new walk to dest; a segment in flight runs out first.
func (w *walker) reset(dest Point, arrive int32, purpose walkPurpose) {
	w.dest, w.arrive, w.purpose = dest, arrive, purpose
	w.status, w.reason, w.endedAt = walkWalking, "", time.Time{}
	w.route = nil
	w.blocked, w.desync, w.watchdog, w.doors = 0, 0, 0, 0
	w.plan.Avoid.Clear()
}

// unreachable ends the walk with reason.
func (w *walker) unreachable(reason string) {
	w.status, w.reason, w.route = walkUnreachable, reason, nil
}

// failed reports the walk's reason when it ended unreachable, "" otherwise:
// the status snapshot shows it.
func (w *walker) failed() string {
	if w.status != walkUnreachable {
		return ""
	}
	return "unreachable: " + w.reason
}

// goTo is the next packet of a walk to dest: done once the bot stands within
// arrive of it (horizontally), unreachable when no route there holds. Each
// segment goes out from standstill, after the previous one's simulated
// arrival plus one position tick (no click-ahead: a click while moving runs
// the server's catch-up step of any length, creature.go catchUpLocked), never
// during the bot's own swing or cast, and only after CanMove and the
// step-chain check pass from where the bot stands. The bot stands up first.
//
// Pacing: one MoveBackwardToLocation per segment through the frame pacer
// (gateNone); a segment waits for the previous one's arrival, so a running
// bot sends about one move a second, far below the server's 80 frames/s.
//
// A walk that ended unreachable stays so for walkRetry, then a goTo to the
// same place starts afresh; a new destination or purpose, or the bot's own
// teleport, starts afresh at once.
func (b *Brain) goTo(dest Point, arrive int32, purpose walkPurpose, now time.Time) (Action, walkStatus) {
	a, st := b.walkStep(dest, arrive, purpose, now)
	if w := &b.walk; st == walkUnreachable && w.endedAt.IsZero() {
		w.endedAt = now
	}
	return a, st
}

func (b *Brain) walkStep(dest Point, arrive int32, purpose walkPurpose, now time.Time) (Action, walkStatus) {
	w := &b.walk
	if w.nav == nil {
		w.status, w.reason = walkUnreachable, reasonNoNav
		return Action{}, walkUnreachable
	}
	if w.dest == (Point{}) || horizontal2(w.dest, dest) > walkSameDest*walkSameDest || w.purpose != purpose ||
		w.status == walkUnreachable && !w.endedAt.IsZero() && now.Sub(w.endedAt) >= walkRetry {
		w.reset(dest, arrive, purpose)
	}
	w.arrive = arrive
	if w.status == walkUnreachable {
		return Action{}, walkUnreachable
	}
	s := &b.world.Self
	if w.seg.sentAt != (time.Time{}) {
		if !w.segmentDone(now) {
			return Action{}, w.status
		}
		if w.status == walkUnreachable {
			return Action{}, walkUnreachable
		}
	}
	if w.escaping {
		if s.Casting(now) || now.Sub(escapeWait(w.escapeAt, s.castEnd)) < escapeRetry {
			return Action{}, walkWalking
		}
		// The /unstuck left the bot where it stood.
		w.escaping = false
		w.unreachable(reasonIsolated)
		return Action{}, walkUnreachable
	}
	pos := b.standing(now)
	if horizontal2(pos, w.dest) <= int64(arrive)*int64(arrive) && !s.Moving(now) {
		w.status, w.route = walkArrived, nil
		return Action{}, walkArrived
	}
	w.status = walkWalking
	if s.Moving(now) || s.Casting(now) || s.Swinging(now) {
		return Action{}, walkWalking
	}
	if s.Sitting {
		return b.toggle(now), walkWalking
	}
	b.walker()
	if len(w.route) == 0 {
		if a, ok := b.planWalk(pos, now); !ok {
			return a, w.status
		}
	}
	to := w.route[0]
	if !w.segmentOK(pos, to) {
		// The door state or the bot's position changed since the route
		// was planned: plan again from here next decision.
		w.route = nil
		if w.watchdog++; w.watchdog >= walkMaxWatchdog {
			w.unreachable(reasonWatchdog)
			return Action{}, walkUnreachable
		}
		return Action{}, walkWalking
	}
	w.next = segment{from: pos, to: to}
	a := moveAction(to, pos)
	a.walk = true
	return a, walkWalking
}

// walker is the bot's walker with its checks sized for the bot as it is
// now: the longest later step is the bot's speed times Emax, and the
// collision height its own. Every check of a segment or a straight line
// goes through it.
func (b *Brain) walker() *walker {
	s, w := &b.world.Self, &b.walk
	w.plan.Lmax = float64(max(s.RunSpeed, s.WalkSpeed)) * navEmax.Seconds()
	w.plan.CollisionHeight = cmpOr(s.CollisionHeight, defaultCollisionHeight)
	w.plan.Swim = float64(s.SwimSpeed) * swimBreath.Seconds()
	return w
}

// standing is where the bot stands at now, on its floor: the end of the
// last segment it arrived at when the server's position agrees, else the
// server's position snapped to the layer nearest it.
func (b *Brain) standing(now time.Time) Point {
	w, pos := &b.walk, b.world.Self.Position(now)
	if w.floor.X == pos.X && w.floor.Y == pos.Y {
		return w.floor
	}
	if c, ok := w.plan.Locate(toLoc(pos)); ok {
		pos.Z = int32(c.Z)
	}
	return pos
}

// segmentOK reports whether the segment from from to to passes CanMove on
// the shared engine, doors included, and the step-chain check: the open
// corridor test, else the exact check.
func (w *walker) segmentOK(from, to Point) bool {
	a, b := toLoc(from), toLoc(to)
	if !w.nav.Engine.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) {
		return false
	}
	return w.plan.CorridorOpen(a, b) || w.plan.StepChainOK(a, b)
}

// segmentDone reports whether the segment in flight is over at now: its
// simulated arrival plus one tick passed. The watchdog counts a segment
// unanswered past walkAnswerWait, or still on its way past its deadline,
// and ends the route after walkMaxWatchdog of them.
func (w *walker) segmentDone(now time.Time) bool {
	seg := &w.seg
	switch {
	case !seg.answered && now.Sub(seg.sentAt) >= walkAnswerWait,
		seg.answered && now.After(seg.deadline):
		w.seg, w.route = segment{}, nil
		if w.watchdog++; w.watchdog >= walkMaxWatchdog {
			w.unreachable(reasonWatchdog)
		}
		return true
	case !seg.answered || now.Before(seg.eta.Add(positionTick)):
		return false
	}
	w.floor = seg.to
	w.seg = segment{}
	if len(w.route) > 0 {
		w.route = w.route[1:]
	}
	return true
}

// planWalk plans the route from pos, the bot standing there, to the walk's
// destination. Isolated ground takes one /unstuck; one-way-out ground first
// drops into the nearest abstract SCC it reaches. It reports false with the
// packet to send (none, or the /unstuck) when there is no route to walk yet.
func (b *Brain) planWalk(pos Point, now time.Time) (Action, bool) {
	w := &b.walk
	h := w.nav.Hier
	from := toLoc(pos)
	cell, ok := w.plan.Locate(from)
	if !ok {
		w.unreachable(reasonNoNav)
		return Action{}, false
	}
	cur, err := h.Locate(cell, &w.pins)
	if err != nil {
		w.unreachable(reasonNoNav)
		return Action{}, false
	}
	w.pins.Current = cur
	var route []location.Location
	switch cur.Kind() {
	case nav.Isolated:
		// The hierarchy keeps out of water, so a bot in water stands on
		// ground it calls isolated: it swims out.
		if w.nav.Water.Contains(from.X, from.Y, from.Z) {
			if route, ok := w.swimRoute(from); ok {
				w.take(route)
				return Action{}, true
			}
		}
		// No abstract SCC is reachable from here (a closed pit, the restart
		// area of acis_golang#5412): only /unstuck leaves it.
		if !w.escapeAt.IsZero() && now.Sub(w.escapeAt) < escapeEvery {
			w.unreachable(reasonIsolated)
			return Action{}, false
		}
		w.escaping = true
		return escapeAction(), false
	default:
		to, same, ok := w.target(w.dest)
		switch {
		case cur.Kind() == nav.OneWayOut:
			// Drop into the nearest abstract SCC first, whatever the
			// destination: the next plan routes from there with the
			// hierarchy.
			route, err = w.drop(from, cur)
		case ok && !same:
			// A one-way drop toward the destination: the hierarchy routes
			// across it over the SCCs between, so a far leader costs no
			// more cells than one in the bot's SCC.
			route, err = h.RouteOneWay(&w.plan, from, to, w.nav.closed())
		case ok:
			route, err = h.Route(&w.plan, from, to, w.nav.closed())
		default:
			if route, ok = w.swimRoute(from); !ok {
				w.unreachable(reasonNoRoute)
				return Action{}, false
			}
		}
	}
	if errors.Is(err, nav.ErrWater) || errors.Is(err, nav.ErrUnreachable) || errors.Is(err, nav.ErrBudget) {
		if r, ok := w.swimRoute(from); ok {
			route, err = r, nil
		}
	}
	switch {
	case errors.Is(err, nav.ErrDoor):
		w.unreachable(reasonDoor)
		return Action{}, false
	case errors.Is(err, nav.ErrBudget):
		w.unreachable(reasonBudget)
		return Action{}, false
	case errors.Is(err, nav.ErrUnreachable) && w.plan.Avoid.Len() > 0:
		// The refused segments' columns cut every way left.
		w.unreachable(reasonBlocked)
		return Action{}, false
	case err != nil || len(route) < 2:
		w.unreachable(reasonNoRoute)
		return Action{}, false
	}
	w.take(route)
	return Action{}, true
}

// take makes route, from the bot's standing point on, the walk's route.
func (w *walker) take(route []location.Location) {
	w.route = w.route[:0]
	for _, l := range route[1:] {
		w.route = append(w.route, fromLoc(l))
	}
}

// swimRoute is the swimming cell planner's route from from to the walk's
// destination, for a walk the hierarchy cannot route: the hierarchy keeps
// out of water, so a destination only water reaches, or a bot in water, has
// no hierarchy route (docs/navigation.md, "Swimming"). It tries only with a
// breath to swim on and water near the way, as a failed search spends up
// to walkBudget cells. Only a follower swims to a destination in water,
// after a leader who went there: a bot left standing in water drowns once
// its breath runs out, so any other place in water (a farming spot on the
// sea floor) stays unreachable.
func (w *walker) swimRoute(from location.Location) ([]location.Location, bool) {
	to := toLoc(w.dest)
	if w.plan.Swim <= 0 || !w.nav.Water.Near(from, to, walkSwimNear) {
		return nil, false
	}
	if c, ok := w.plan.Locate(to); ok && w.purpose != walkFollow && w.nav.Water.Contains(to.X, to.Y, int(c.Z)) {
		return nil, false
	}
	route, err := w.plan.Route(from, to)
	return route, err == nil && len(route) >= 2
}

// target is dest on walkable ground in the bot's SCC: dest itself, or the
// walkable cell nearest it within the walk's arrive radius. A walk that may
// take one-way drops (walkPurpose.drops) also takes ground the bot only
// reaches one way, unless that ground is isolated; same is false for it.
func (w *walker) target(dest Point) (to location.Location, same, ok bool) {
	at := toLoc(dest)
	h := w.nav.Hier
	try := func(c nav.Cell) (location.Location, bool, bool) {
		m, err := h.Locate(c, &w.pins)
		if err != nil {
			return location.Location{}, false, false
		}
		same := h.SameSCC(w.pins.Current, m)
		if !same && (!w.purpose.drops() || m.Kind() == nav.Isolated || !h.Reachable(w.pins.Current, m)) {
			return location.Location{}, false, false
		}
		w.pins.Destination = m
		return location.Location{X: at.X, Y: at.Y, Z: int(c.Z)}, same, true
	}
	if c, ok := w.plan.Locate(at); ok {
		if l, same, ok := try(c); ok {
			return l, same, true
		}
	}
	c, ok := w.plan.NearestWalkable(at, float64(max(w.arrive, walkSlop)))
	if !ok {
		return location.Location{}, false, false
	}
	l, same, ok := try(c)
	if ok {
		l.X, l.Y = engine.WorldX(c.X), engine.WorldY(c.Y)
	}
	return l, same, ok
}

// inSCC is p moved toward at onto ground of at's SCC: the first point of the
// line from p to at, a cell apart, whose cell (on the layer nearest at's
// height) shares at's SCC; at itself when none does or at has no member, p
// as it is without geodata. A follow slot drawn behind the leader across a
// wall or down a ledge, or a flee or drift point past a wall's far side,
// becomes ground the walker plans to.
func (w *walker) inSCC(at, p Point) Point {
	if w.nav == nil {
		return p
	}
	h := w.nav.Hier
	ac, ok := w.plan.Locate(toLoc(at))
	if !ok {
		return p
	}
	am, err := h.Locate(ac, nil)
	if err != nil {
		return p
	}
	n := int(math.Sqrt(float64(horizontal2(at, p))) / walkSlop)
	for i := range n {
		f := float64(i) / float64(n)
		q := Point{
			X: p.X + int32(math.Round(float64(at.X-p.X)*f)),
			Y: p.Y + int32(math.Round(float64(at.Y-p.Y)*f)),
			Z: at.Z,
		}
		c, ok := w.plan.Locate(toLoc(q))
		if !ok {
			continue
		}
		if m, err := h.Locate(c, nil); err == nil && h.SameSCC(am, m) {
			q.Z = int32(c.Z)
			return q
		}
	}
	at.Z = int32(ac.Z)
	return at
}

// drop is the one checked route from one-way-out ground at from into the
// nearest abstract SCC its member cur reaches: the nearest cell of one in
// rings of cells around from that a route reaches.
func (w *walker) drop(from location.Location, cur nav.Member) ([]location.Location, error) {
	h := w.nav.Hier
	cx, cy := engine.GeoX(from.X), engine.GeoY(from.Y)
	for r := 1; r <= walkDropCells; r++ {
		for dx := -r; dx <= r; dx++ {
			for dy := -r; dy <= r; dy++ {
				if max(abs(dx), abs(dy)) != r {
					continue
				}
				x, y := engine.WorldX(cx+dx), engine.WorldY(cy+dy)
				c, ok := w.plan.Locate(location.Location{X: x, Y: y, Z: from.Z})
				if !ok {
					continue
				}
				m, err := h.Locate(c, nil)
				if err != nil || m.Kind() != nav.InSCC || !h.Reachable(cur, m) {
					continue
				}
				if route, err := w.plan.Route(from, location.Location{X: x, Y: y, Z: int(c.Z)}); err == nil {
					return route, nil
				}
			}
		}
	}
	return nil, nav.ErrUnreachable
}

// sent records the walker's packet that went out at now. Any packet but a
// talk click ends the wait for a talk click's answer: its answer can no
// longer be told from the new action's.
func (w *walker) sent(a Action, now time.Time) {
	if a.kind != actTalk {
		w.talkAt = time.Time{}
	}
	switch {
	case a.kind == actMove && a.walk:
		w.seg = w.next
		w.seg.sentAt = now
		w.seg.door = w.nav.Doors != nil && w.nav.Doors.Crosses(w.seg.from, w.seg.to)
	case a.kind == actMove, a.kind == actAttack, a.kind == actTalk:
		// Another rule moved the bot (or clicked, which may walk it): the
		// route no longer starts where the bot stands.
		w.seg, w.route = segment{}, nil
	case a.kind == actUserCommand && w.escaping:
		w.escapeAt = now
	}
}

// observeWalk reads the server's answers to the walker's own segments
// (docs/navigation.md, "Server answers to its own moves"). The server ends a
// move whose step fails with its blocked correction, a MoveToLocation whose
// origin is its destination (move/controller.go blocked,
// BroadcastBlockedCorrection): before or after it answered the segment,
// short of the segment's end that is a blocked stop. An answer toward
// another destination is a desync (a door replan when the segment crosses
// a door). With no segment in flight, the blocked correction of a talk
// click's straight approach short of talking range is a blocked stop of the
// talk approach (targeting.go onPlayerArrivedBlocked; in range the server
// sends StopMove and opens the page). Any other move of the bot the walker
// did not send, a StopMove mid-segment (a stun, a stop) and a teleport drop
// the route; a teleport also starts the next walk afresh. ActionFailed is no
// navigation signal.
func (b *Brain) observeWalk(ev Event, now time.Time) {
	w, s := &b.walk, &b.world.Self
	if !w.talkAt.IsZero() && now.Sub(w.talkAt) > talkWait {
		w.talkAt = time.Time{}
	}
	switch e := ev.(type) {
	case MoveToLocation:
		if e.ObjectID != s.ObjectID {
			return
		}
		still := e.Origin.X == e.Dest.X && e.Origin.Y == e.Dest.Y
		seg := &w.seg
		switch {
		case !seg.sentAt.IsZero() && !seg.answered:
			seg.answered = true
			switch {
			case still:
				b.walkBlocked(e.Dest)
			case horizontal2(e.Dest, seg.to) > walkSlop*walkSlop:
				b.walkDesync(e)
			default:
				run := s.legTime(e.Origin, e.Dest)
				seg.eta = now.Add(run)
				seg.deadline = seg.eta.Add(run/2 + walkSlack)
			}
		case !seg.sentAt.IsZero() && still:
			if horizontal2(e.Dest, seg.to) > walkSlop*walkSlop {
				b.walkBlocked(e.Dest)
				return
			}
			// Stopped on the segment's end: it arrived.
			seg.eta, seg.deadline = now, now.Add(walkSlack)
		case still && !w.talkAt.IsZero():
			if horizontal2(e.Dest, w.talkTo) > visitTalkRange*visitTalkRange {
				b.talkBlocked()
			}
			w.talkAt = time.Time{}
			w.route = nil
		default:
			w.seg, w.route = segment{}, nil
		}
	case StopMove:
		if e.ObjectID != s.ObjectID {
			return
		}
		// A talk click's approach in range: the page follows.
		w.talkAt = time.Time{}
		if !w.seg.sentAt.IsZero() && horizontal2(e.Pos, w.seg.to) > walkSlop*walkSlop {
			w.seg, w.route = segment{}, nil
		}
	case NpcHtmlMessage:
		w.talkAt = time.Time{}
	case TeleportToLocation:
		if e.ObjectID == s.ObjectID {
			w.seg, w.route, w.escaping = segment{}, nil, false
			w.dest = Point{}
		}
	}
}

// walkBlocked takes a blocked stop at stop of the segment in flight: the
// step chain differed from the walker's check. It replans from the stop
// point, and every plan of this walk keeps out of the columns of the rest of
// the segment, from the stop point to its end (docs/navigation.md, "Server
// answers to its own moves").
func (b *Brain) walkBlocked(stop Point) {
	w := &b.walk
	b.metrics.NavBlocked()
	w.plan.Avoid.AddLine(toLoc(stop), toLoc(w.seg.to))
	w.seg, w.route = segment{}, nil
	if w.blocked++; w.blocked >= walkMaxBlocked {
		w.unreachable(reasonBlocked)
	}
}

// walkDesync takes the server's answer to the segment in flight toward
// another destination: it routed or truncated, so the bot's geodata or door
// state differs from the server's. A desync off a door logs the segment and
// the server's answer, so a run names the leg its check got wrong
// (docs/navigation.md, "Server answers to its own moves"). The walker
// replans once the server's move ends.
func (b *Brain) walkDesync(answer MoveToLocation) {
	w := &b.walk
	seg := w.seg
	w.seg, w.route = segment{}, nil
	if seg.door {
		b.metrics.NavDoorReplan()
		if w.doors++; w.doors >= walkMaxDoor {
			w.unreachable(reasonDoor)
		}
		return
	}
	b.metrics.NavDesync()
	b.log.Warn("nav desync", "from", logPoint(seg.from), "to", logPoint(seg.to),
		"server_origin", logPoint(answer.Origin), "server_dest", logPoint(answer.Dest))
	if w.desync++; w.desync >= walkMaxDesync {
		w.unreachable(reasonDesync)
	}
}

// logPoint is p as a log value: x,y,z.
func logPoint(p Point) string { return fmt.Sprintf("%d,%d,%d", p.X, p.Y, p.Z) }

// talking records the talk click about to go out on an NPC standing at
// npc at now.
func (w *walker) talking(npc Point, now time.Time) { w.talkTo, w.talkAt = npc, now }

// talkBlocked takes the blocked correction that ends a click on an NPC whose
// straight approach stopped short of talking range (targeting.go
// onPlayerArrivedBlocked): a blocked stop of the talk approach.
func (b *Brain) talkBlocked() {
	b.metrics.NavBlocked()
	w := &b.walk
	w.talkFailed = true
	if w.blocked++; w.blocked >= walkMaxBlocked {
		w.unreachable(reasonBlocked)
	}
}

// lineOpen reports whether the straight line from the bot at from to a
// target at to holds for the server's straight, un-routed walks (a click on
// a creature or NPC, MoveToPawn): CanMove and the step-chain check. Without
// geodata there is nothing to check against: the bot acts as it would.
func (w *walker) lineOpen(from, to Point) bool {
	if w.nav == nil {
		return true
	}
	a, b := toLoc(from), toLoc(to)
	if c, ok := w.plan.Locate(b); ok {
		b.Z = int(c.Z)
	}
	if c, ok := w.plan.Locate(a); ok {
		a.Z = int(c.Z)
	}
	return w.segmentOK(fromLoc(a), fromLoc(b))
}

// sightOpen reports whether the bot at from sees a target of collision
// height height at to (CanSeeActor with both heights): a bow shot or a
// cast in range needs no walk then. Without geodata it is true.
func (w *walker) sightOpen(from, to Point, height float64) bool {
	if w.nav == nil {
		return true
	}
	return w.nav.Engine.CanSeeActor(int(from.X), int(from.Y), int(from.Z), w.plan.CollisionHeight,
		int(to.X), int(to.Y), int(to.Z), height)
}

// pickUpOpen reports whether a pick-up of the item at item may go out from
// from: the server routes the walk to an item out of range
// (network/targeting.go), so the bot sends it only when CanMove onto the
// item's raw Z and the step chain hold from where it stands. Without
// geodata it is true.
func (w *walker) pickUpOpen(from, item Point) bool {
	if w.nav == nil {
		return true
	}
	a := toLoc(from)
	if c, ok := w.plan.Locate(a); ok {
		a.Z = int(c.Z)
	}
	return w.segmentOK(fromLoc(a), item)
}

// standPoint is a point within rng of target at offering mode toward it,
// the nearest to the bot at from, in the bot's SCC.
func (w *walker) standPoint(from, at Point, rng float64, mode nav.StandMode, height float64) (Point, bool) {
	if w.nav == nil {
		return Point{}, false
	}
	l, ok := w.plan.StandPoint(toLoc(from), nav.Target{At: toLoc(at), Range: rng, Mode: mode, CollisionHeight: height})
	if !ok {
		return Point{}, false
	}
	c, ok := w.plan.Locate(l)
	if !ok {
		return Point{}, false
	}
	m, err := w.nav.Hier.Locate(c, &w.pins)
	if err != nil {
		return Point{}, false
	}
	// StandPoint is not restricted to the bot's SCC: refuse a point not
	// reachable from where the bot stands.
	fc, ok := w.plan.Locate(toLoc(from))
	if !ok {
		return Point{}, false
	}
	cur, err := w.nav.Hier.Locate(fc, &w.pins)
	if err != nil {
		return Point{}, false
	}
	w.pins.Current = cur
	if cur.Kind() == nav.InSCC && !w.nav.Hier.SameSCC(cur, m) {
		return Point{}, false
	}
	return fromLoc(l), true
}

// talkPoint is where the bot at from clicks an NPC at at when no stand
// point offers an open straight line to it (an NPC behind its counter or in
// its stall): the click's straight approach ends blocked, and a blocked stop
// within the server's interaction distance still talks (targeting.go
// onPlayerArrivedBlocked). It is from itself when that lies within rng of
// the NPC (in 3D, on its floor), else groundNear.
func (w *walker) talkPoint(from, at Point, rng float64) (Point, bool) {
	if w.nav == nil {
		return Point{}, false
	}
	fl := toLoc(from)
	if c, ok := w.plan.Locate(fl); ok {
		if fl.Z = int(c.Z); within3(fromLoc(fl), at, rng) {
			return fromLoc(fl), true
		}
	}
	return w.groundNear(from, at, rng)
}

// groundNear is the walkable cell of the bot's SCC nearest at within rng
// (in 3D, on its floor), the bot standing at from: at's own cell when at
// lies on that ground.
func (w *walker) groundNear(from, at Point, rng float64) (Point, bool) {
	if w.nav == nil {
		return Point{}, false
	}
	fc, ok := w.plan.Locate(toLoc(from))
	if !ok {
		return Point{}, false
	}
	h := w.nav.Hier
	cur, err := h.Locate(fc, &w.pins)
	if err != nil {
		return Point{}, false
	}
	w.pins.Current = cur
	c, ok := w.plan.NearestWalkableFunc(toLoc(at), rng, func(c nav.Cell) bool {
		if !within3(fromLoc(c.World()), at, rng) {
			return false
		}
		m, err := h.Locate(c, &w.pins)
		return err == nil && m.Kind() != nav.Isolated && (cur.Kind() != nav.InSCC || h.SameSCC(cur, m))
	})
	if !ok {
		return Point{}, false
	}
	return fromLoc(c.World()), true
}

// within3 reports whether p lies within rng of at in 3D.
func within3(p, at Point, rng float64) bool {
	return math.Hypot(math.Sqrt(float64(horizontal2(p, at))), float64(p.Z-at.Z)) <= rng
}

func toLoc(p Point) location.Location {
	return location.Location{X: int(p.X), Y: int(p.Y), Z: int(p.Z)}
}

func fromLoc(l location.Location) Point {
	return Point{X: int32(l.X), Y: int32(l.Y), Z: int32(l.Z)}
}

// horizontal2 is the squared horizontal distance between a and b.
func horizontal2(a, b Point) int64 {
	dx, dy := int64(a.X-b.X), int64(a.Y-b.Y)
	return dx*dx + dy*dy
}

func cmpOr(v, def float64) float64 {
	if v > 0 {
		return v
	}
	return def
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// escapeWait is when the wait for an /unstuck asked at asked counts from:
// the end of the recall cast when one ran after it (the server teleports
// when the cast completes, so the TeleportToLocation lands just after
// castEnd), else when it was asked.
func escapeWait(asked, castEnd time.Time) time.Time {
	if castEnd.After(asked) {
		return castEnd
	}
	return asked
}
