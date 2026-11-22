package botclient

import (
	"errors"
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
	// walkBudget caps the cells one route search expands.
	walkBudget = 60000
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
	reasonIsolated = "isolated"
	reasonNoNav    = "no nav"
)

// walkPurpose is what a walk is for. A talk approach also takes the
// StopMove that ends a blocked click on the NPC as a blocked stop.
type walkPurpose uint8

const (
	walkPlace walkPurpose = iota
	walkTalk
	walkMelee
	walkPickup
	walkRanged
)

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
	// StopMove of that click ended its straight approach short of talking
	// range, so the visit walks to a new stand point.
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
		w.plan = nav.Planner{Engine: n.Engine, Budget: walkBudget}
	}
	return w
}

// reset starts a new walk to dest; a segment in flight runs out first.
func (w *walker) reset(dest Point, arrive int32, purpose walkPurpose) {
	w.dest, w.arrive, w.purpose = dest, arrive, purpose
	w.status, w.reason = walkWalking, ""
	w.route = nil
	w.blocked, w.desync, w.watchdog, w.doors = 0, 0, 0, 0
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
func (b *Brain) goTo(dest Point, arrive int32, purpose walkPurpose, now time.Time) (Action, walkStatus) {
	w := &b.walk
	if w.nav == nil {
		w.status, w.reason = walkUnreachable, reasonNoNav
		return Action{}, walkUnreachable
	}
	if w.dest == (Point{}) || horizontal2(w.dest, dest) > walkSameDest*walkSameDest || w.purpose != purpose {
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
			w.unreachable(reasonBlocked)
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
		// No abstract SCC is reachable from here (a closed pit, the restart
		// area of acis_golang#5412): only /unstuck leaves it.
		if !w.escapeAt.IsZero() && now.Sub(w.escapeAt) < escapeEvery {
			w.unreachable(reasonIsolated)
			return Action{}, false
		}
		w.escaping = true
		return escapeAction(), false
	case nav.OneWayOut:
		route, err = w.drop(from, cur)
	default:
		to, ok := w.target(w.dest)
		if !ok {
			w.unreachable(reasonNoRoute)
			return Action{}, false
		}
		route, err = h.Route(&w.plan, from, to, w.nav.closed())
	}
	switch {
	case errors.Is(err, nav.ErrDoor):
		w.unreachable(reasonDoor)
		return Action{}, false
	case err != nil || len(route) < 2:
		w.unreachable(reasonNoRoute)
		return Action{}, false
	}
	w.route = w.route[:0]
	for _, l := range route[1:] {
		w.route = append(w.route, fromLoc(l))
	}
	return Action{}, true
}

// target is dest on walkable ground in the bot's SCC: dest itself, or the
// walkable cell nearest it within the walk's arrive radius.
func (w *walker) target(dest Point) (location.Location, bool) {
	to := toLoc(dest)
	try := func(c nav.Cell) (location.Location, bool) {
		m, err := w.nav.Hier.Locate(c, &w.pins)
		if err != nil || !w.nav.Hier.SameSCC(w.pins.Current, m) {
			return location.Location{}, false
		}
		w.pins.Destination = m
		return location.Location{X: to.X, Y: to.Y, Z: int(c.Z)}, true
	}
	if c, ok := w.plan.Locate(to); ok {
		if l, ok := try(c); ok {
			return l, true
		}
	}
	c, ok := w.plan.NearestWalkable(to, float64(max(w.arrive, walkSlop)))
	if !ok {
		return location.Location{}, false
	}
	l, ok := try(c)
	if ok {
		l.X, l.Y = engine.WorldX(c.X), engine.WorldY(c.Y)
	}
	return l, ok
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

// sent records the walker's packet that went out at now.
func (w *walker) sent(a Action, now time.Time) {
	switch {
	case a.kind == actMove && a.walk:
		w.seg = w.next
		w.seg.sentAt = now
		w.seg.door = w.nav.Doors != nil && w.nav.Doors.Crosses(w.seg.from, w.seg.to)
	case a.kind == actMove, a.kind == actAttack:
		// Another rule moved the bot (or clicked, which may walk it): the
		// route no longer starts where the bot stands.
		w.seg, w.route = segment{}, nil
	case a.kind == actUserCommand && w.escaping:
		w.escapeAt = now
	}
}

// observe reads the server's answers to the walker's own segments
// (docs/navigation.md, "Server answers to its own moves"): a MoveToLocation
// whose origin is its destination is a blocked stop, one toward another
// destination a desync (a door replan when the segment crosses a door), and
// a StopMove short of a talk approach's NPC is a blocked stop too. A move of
// the bot the walker did not send drops the route; a teleport too.
// ActionFailed is no navigation signal.
func (b *Brain) observeWalk(ev Event, now time.Time) {
	w, s := &b.walk, &b.world.Self
	switch e := ev.(type) {
	case MoveToLocation:
		if e.ObjectID != s.ObjectID {
			return
		}
		seg := &w.seg
		if seg.sentAt.IsZero() || seg.answered {
			w.seg, w.route = segment{}, nil
			return
		}
		seg.answered = true
		switch {
		case e.Origin.X == e.Dest.X && e.Origin.Y == e.Dest.Y:
			b.walkBlocked()
		case horizontal2(e.Dest, seg.to) > walkSlop*walkSlop:
			b.walkDesync(seg.door)
		default:
			speed := float64(pace(s.Running, s.RunSpeed, s.WalkSpeed))
			if speed <= 0 {
				speed = 1
			}
			run := time.Duration(math.Sqrt(float64(horizontal2(e.Origin, e.Dest))) / speed * float64(time.Second))
			seg.eta = now.Add(run)
			seg.deadline = seg.eta.Add(run/2 + walkSlack)
		}
	case StopMove:
		if e.ObjectID != s.ObjectID {
			return
		}
		switch {
		case !w.seg.sentAt.IsZero():
			if horizontal2(e.Pos, w.seg.to) > walkSlop*walkSlop {
				b.walkBlocked()
			}
		case !w.talkAt.IsZero():
			// The talk click's straight approach stopped: short of talking
			// range it opens no page (targeting.go onPlayerArrivedBlocked).
			if horizontal2(e.Pos, w.talkTo) > visitTalkRange*visitTalkRange {
				b.talkBlocked()
			}
			w.talkAt = time.Time{}
		}
	case TeleportToLocation:
		if e.ObjectID == s.ObjectID {
			w.seg, w.route, w.escaping = segment{}, nil, false
		}
	}
}

// walkBlocked takes a blocked stop of the segment in flight: the step chain
// differed from the walker's check. It replans from the stop point.
func (b *Brain) walkBlocked() {
	w := &b.walk
	b.metrics.NavBlocked()
	w.seg, w.route = segment{}, nil
	if w.blocked++; w.blocked >= walkMaxBlocked {
		w.unreachable(reasonBlocked)
	}
}

// walkDesync takes a segment the server answered with another destination:
// it routed or truncated, so the bot's geodata or door state differs from
// the server's. The walker replans once the server's move ends.
func (b *Brain) walkDesync(door bool) {
	w := &b.walk
	w.seg, w.route = segment{}, nil
	if door {
		b.metrics.NavDoorReplan()
		if w.doors++; w.doors >= walkMaxDoor {
			w.unreachable(reasonDoor)
		}
		return
	}
	b.metrics.NavDesync()
	if w.desync++; w.desync >= walkMaxDesync {
		w.unreachable(reasonDesync)
	}
}

// talking records the talk click about to go out on an NPC standing at
// npc at now.
func (w *walker) talking(npc Point, now time.Time) { w.talkTo, w.talkAt = npc, now }

// talkBlocked takes the StopMove that ends a click on an NPC whose straight
// approach stopped short of talking range (targeting.go
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
