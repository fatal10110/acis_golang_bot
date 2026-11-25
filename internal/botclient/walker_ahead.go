package botclient

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// Click-ahead at checked corners (#308; docs/navigation.md, "Click-ahead").
//
// A segment that is not the route's last sends the next one before the
// server's move ends, lead before its modeled end: the click reaches the
// server while it still walks the last stretch of the segment, so the
// server retargets the walk in flight from its catch-up point (acis_golang
// model/actor/move creature.go moveToLocationLocked, catchUpLocked) and
// keeps the start-speed update count, or, if the click comes late, starts
// it afresh from the segment's end. Either way the click's origin lies on
// the fan's stretch, and every line from there to the next waypoint was
// checked before the click went out (nav.Planner.FanOpen).
//
// Pacing: a click-ahead is the segment's one MoveBackwardToLocation, sent
// earlier through the same frame pacer (gateNone); it adds no packet.

// aheadFanSlack is how far back, as time at the bot's speed, the fan
// reaches past the click's lead: navEmax for the error of the modeled
// arrival, and navEmax more for a catch-up step the server refuses, which
// leaves the walk at its last update point (creature.go catchUpLocked).
const aheadFanSlack = 2 * navEmax

// aheadLead is how long before the modeled end of the segment in flight a
// click-ahead goes out: the answer latency A and one position tick, so it
// reaches the server about a tick before the server's own arrival (the
// modeled end errs late by the downlink).
func (s *SelfState) aheadLead() time.Duration { return s.lag() + positionTick }

// aheadAt is when the segment in flight may click ahead: aheadLead before
// its modeled end, while it is answered, from a server origin on its line,
// not refused a fan yet, and the route has a waypoint after its end.
func (b *Brain) aheadAt() (time.Time, bool) {
	w := &b.walk
	if !w.aheadable() {
		return time.Time{}, false
	}
	s := &b.world.Self
	return s.arrival().Add(-s.aheadLead()), true
}

// aheadable reports whether the segment in flight is one a click-ahead may
// follow (aheadAt).
func (w *walker) aheadable() bool {
	seg := &w.seg
	return w.status == walkWalking && !w.escaping && !seg.sentAt.IsZero() && seg.answered &&
		!seg.noAhead && len(w.route) >= 2 && horizontal2(w.route[0], seg.to) <= walkSlop*walkSlop
}

// clickAhead is the click to the waypoint after the segment in flight's end,
// when it may go out at now (aheadAt): on land, through no door, the bot
// neither swinging, casting nor sitting, and every line from the fan's
// stretch to the waypoint checked. A segment whose fan fails is marked, and
// the next segment goes out from standstill as before.
func (b *Brain) clickAhead(now time.Time) (Action, bool) {
	w, s := &b.walk, &b.world.Self
	at, ok := b.aheadAt()
	if !ok || now.Before(at) || !s.Moving(now) || s.Casting(now) || s.Swinging(now) || s.Sitting {
		return Action{}, false
	}
	seg := &w.seg
	to := w.route[1]
	a0, a1, ok := w.fan(s)
	if !ok || s.move.wet() || w.crossesWater(seg.to, to) || w.crossesDoor(seg.origin, seg.to) || w.crossesDoor(seg.to, to) {
		seg.noAhead = true
		return Action{}, false
	}
	b.walker()
	if !w.plan.FanOpen(toLoc(a0), toLoc(a1), toLoc(to)) {
		seg.noAhead = true
		return Action{}, false
	}
	w.next = segment{from: seg.to, to: to, ahead: true, fan0: a0, fan1: a1}
	a := moveAction(to, s.Position(now))
	a.walk = true
	return a, true
}

// fan is the stretch of the segment in flight a click-ahead going out now
// can start from: the last speed × (aheadLead + aheadFanSlack) of the
// server's line from its answer's origin to the segment's end, the whole
// line when shorter. It reports false when the bot cannot move.
func (w *walker) fan(s *SelfState) (a0, a1 Point, ok bool) {
	speed := float64(max(s.RunSpeed, s.WalkSpeed))
	if speed <= 0 {
		return Point{}, Point{}, false
	}
	seg := &w.seg
	from, to := seg.origin, seg.to
	length := math.Sqrt(float64(horizontal2(from, to)))
	back := speed * (s.aheadLead() + aheadFanSlack).Seconds()
	if back >= length {
		return from, to, true
	}
	t := 1 - back/length
	a0 = Point{
		X: from.X + int32(math.Round(t*float64(to.X-from.X))),
		Y: from.Y + int32(math.Round(t*float64(to.Y-from.Y))),
		Z: from.Z + int32(math.Round(t*float64(to.Z-from.Z))),
	}
	return a0, to, true
}

// inFan reports whether p, the origin of the server's answer to a
// click-ahead, lies on the fan the click was checked for: within
// nav.FanBand of its stretch, ends included.
func (seg *segment) inFan(p Point) bool {
	ax, ay := float64(seg.fan0.X), float64(seg.fan0.Y)
	dx, dy := float64(seg.fan1.X)-ax, float64(seg.fan1.Y)-ay
	px, py := float64(p.X)-ax, float64(p.Y)-ay
	t := 0.0
	if l2 := dx*dx + dy*dy; l2 > 0 {
		t = min(max((px*dx+py*dy)/l2, 0), 1)
	}
	return math.Hypot(px-t*dx, py-t*dy) <= nav.FanBand
}

// crossesWater reports whether the line from a to b passes a water zone of
// the walker's nav data: a click-ahead stays on land, where the step bounds
// FanOpen checks hold.
func (w *walker) crossesWater(a, b Point) bool {
	if w.nav.Water == nil {
		return false
	}
	_, _, ok := w.nav.Water.Span(toLoc(a), toLoc(b))
	return ok
}

// crossesDoor reports whether the line from a to b crosses a door cell: a
// door's state may change under a click in flight, so a door segment goes
// out from standstill.
func (w *walker) crossesDoor(a, b Point) bool {
	return w.nav.Doors != nil && w.nav.Doors.Crosses(a, b)
}

// aheadAnswered takes the server's answer to a click-ahead toward the
// segment's end: its origin is where the server took the click. Off the
// checked fan the line the server walks was not checked: the walker keeps
// to the server's move, sends nothing more on this route and replans from
// where the move ends (a blocked stop on the way still builds its avoided
// line from the server's origin).
func (b *Brain) aheadAnswered(origin Point) {
	w := &b.walk
	seg := &w.seg
	seg.from = origin
	if seg.inFan(origin) {
		return
	}
	seg.noAhead = true
	w.route = nil
	b.log.Info("click-ahead outside its fan", "fan0", logPoint(seg.fan0), "fan1", logPoint(seg.fan1),
		"server_origin", logPoint(origin), "to", logPoint(seg.to))
}

// staleStop takes a blocked correction of the previous segment received
// while the click-ahead after it is unanswered: the server sent it before it
// took the click (the connection keeps the order), so it is never the
// click's answer, which moves. At the previous segment's end it is that
// segment's arrival, a catch-up into a closed line having made the block
// sticky (creature.go moveToLocationLocked): nothing to charge. Short of it
// the previous segment was stopped blocked, charged to it; the click then
// starts from the stop point, off the fan (aheadAnswered).
func (b *Brain) staleStop(stop Point) {
	w := &b.walk
	prev := w.prev
	w.prev = segment{}
	if prev.sentAt.IsZero() || horizontal2(stop, prev.to) <= walkSlop*walkSlop {
		return
	}
	b.chargeBlocked(prev, stop)
}
