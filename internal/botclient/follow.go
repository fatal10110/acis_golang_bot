package botclient

import (
	"math"
	"time"
)

// FollowPolicy is how a party member follows its leader (rule "follow"):
// from a slot behind the leader, at an angle and a distance between MinDist
// and MaxDist drawn once per leader. Every value is a configurable default
// (D6); a zero MaxDist never follows.
type FollowPolicy struct {
	MinDist, MaxDist int32
	// Repath is how far the leader moves from where the bot last took its
	// slot before the bot takes it again, DelayMin to DelayMax later.
	Repath             int32
	DelayMin, DelayMax time.Duration
}

// defaultFollowPolicy is the support persona's slot (docs/plan.md,
// personas): 150-350 units out, again once the leader is ~250 units on,
// after 200-800 ms.
func defaultFollowPolicy() FollowPolicy {
	return FollowPolicy{
		MinDist: 150, MaxDist: 350, Repath: 250,
		DelayMin: 200 * time.Millisecond, DelayMax: 800 * time.Millisecond,
	}
}

const (
	// followArc is the widest angle off straight behind the leader a slot
	// is drawn at, either side.
	followArc = math.Pi / 3
	// followArrive is how close to its slot the bot has arrived.
	followArrive = 50
	// headingUnits is a full turn in the units of a heading.
	headingUnits = 65536
	// followStill is how long a leader in sight stands still before a
	// follow walk toward it takes long segments (walkFollowStill, #344);
	// followReportStill is the same for a leader out of sight, whose
	// position the server reports every 12 s (PartyMemberPosition): a report
	// that moved means the leader may still be moving until the next one.
	followStill       = 3 * time.Second
	followReportStill = 13 * time.Second
)

// followState is the bot's slot behind its party leader.
type followState struct {
	// leader is the leader the slot is drawn for; angle (off straight
	// behind) and radius are drawn again for a new one.
	leader int32
	angle  float64
	radius int32
	// anchor is where the leader stood when the bot last took its slot;
	// anchored is false until the first time.
	anchor   Point
	anchored bool
	// dueAt is when the pending re-path goes out, zero for none.
	dueAt time.Time
	// wake is set until a decision runs at or after dueAt: the re-path's
	// own wake-up, which pulls decisionAt earlier and never later.
	wake bool
	// dest is the slot the bot walks to while traveling; stuck is set once
	// the walk there ended unreachable, until the leader moves on.
	dest      Point
	traveling bool
	stuck     bool
	// runToggledAt is the last walk/run toggle not yet settled.
	runToggledAt time.Time
	// seen is where the leader was at the last decision, seenOK false
	// before the first; movedAt is when the leader was last seen moving or
	// found away from seen, zero for never (followState.still).
	seen    Point
	seenOK  bool
	movedAt time.Time
}

// still reports whether the leader has stood still long enough at now for
// a follow walk toward it to take long segments (#344): it was not seen
// moving, nor found more than walkSameDest from where it was at the last
// decision, for followStill in sight and followReportStill out of sight. A
// leader never seen moving stands still. It records the leader's position
// for the next decision, so it runs once per decision.
func (v *followState) still(lead leaderView, now time.Time) bool {
	if lead.obj != nil && lead.obj.Moving(now) ||
		v.seenOK && horizontal2(lead.pos, v.seen) > walkSameDest*walkSameDest {
		v.movedAt = now
	}
	v.seen, v.seenOK = lead.pos, true
	wait := followStill
	if lead.obj == nil {
		wait = followReportStill
	}
	return v.movedAt.IsZero() || now.Sub(v.movedAt) >= wait
}

// leaderView is what the bot knows of its leader at a decision.
type leaderView struct {
	pos Point
	// obj is the leader while in sight, nil when pos is its last
	// PartyMemberPosition.
	obj *Object
}

// ruleFollow keeps a party member in its slot behind a leader other than
// itself, or any bot behind the commander a GM told it to follow (followed):
// it takes the slot when it joins, and again only once the leader moved
// Repath away from where it stood then (or the bot drifted farther than
// MaxDist+Repath from it, after a fight), DelayMin to DelayMax after
// noticing. A bot still walking to its last slot when the leader moves on
// keeps walking there through the delay, and turns to the new slot once it
// ends (the segment in flight runs out first). It matches the leader's walk
// or run, and stands up before it moves. A leader out of sight is followed to its last PartyMemberPosition
// (sent every 12 s); a bot that died and came back in town walks back to it
// the same way. While following, the rule holds the turn: no farming spot,
// goal or idle drift pulls the bot away. Server-side follow (a second Action
// on the leader) is not used: it glues the bot to the leader.
//
// The slot is snapped into the leader's SCC (walker.inSCC) and walked
// through the walker (D13), which may take one-way drops toward it
// (walkPurpose.drops): a slot across a wall is routed around it, a leader
// below a ledge is followed down, and a leader on ground the bot cannot
// climb to is not chased: the walk ends unreachable and the bot waits for
// the leader to move on. The walk takes segments of at most movingSegment
// toward a leader that moves (walkFollow), and the planner's long ones
// toward a leader that stands still (walkFollowStill, followState.still); a
// leader that starts moving resets the walk, which lets the segment in
// flight run out and replans from its end with the cap (#344).
//
// Each move is one walker segment (MoveBackwardToLocation) and each toggle
// one RequestActionUse, at most one per decision, through the frame pacer
// (gateNone); a toggle waits toggleSettle for the server's answer.
func (b *Brain) ruleFollow(now time.Time) (Action, bool) {
	p, w, v := &b.persona.Follow, b.world, &b.follow
	id := b.followed()
	// On its leader's gatekeeper trip the member walks to that gatekeeper
	// under rule "travel" (partyTrip).
	if p.MaxDist <= 0 || id == 0 || id == w.Self.ObjectID || b.partyTrip() {
		v.leader = 0
		return Action{}, false
	}
	if v.leader != id {
		*v = followState{
			leader:       id,
			angle:        (2*b.rng.Float64() - 1) * followArc,
			radius:       p.MinDist + b.rng.Int32N(max(p.MaxDist-p.MinDist, 0)+1),
			runToggledAt: v.runToggledAt,
		}
	}
	lead, ok := b.leader(id, now)
	if !ok {
		return Action{}, true
	}
	purpose := walkFollow
	if v.still(lead, now) {
		purpose = walkFollowStill
	}
	s := &w.Self
	if run := lead.obj == nil || lead.obj.Running; run != s.Running && now.Sub(v.runToggledAt) >= toggleSettle {
		return walkRunAction(), true
	}
	from := s.Position(now)
	moved := !v.anchored || dist2(lead.pos, v.anchor) > int64(p.Repath)*int64(p.Repath)
	far := max(int64(p.MaxDist)+int64(p.Repath), 0)
	drifted := !v.traveling && !v.stuck && !s.Moving(now) && dist2(from, lead.pos) > far*far
	if (moved || drifted) && v.dueAt.IsZero() {
		v.dueAt, v.wake = now.Add(b.followDelay()), true
	}
	// A re-path pending while the bot travels keeps it walking to the old
	// slot, so its next segment goes out; the new slot resets the walk once
	// the delay ends. Only a bot standing still waits the delay out.
	if !v.dueAt.IsZero() && !now.Before(v.dueAt) {
		if s.Sitting {
			return b.toggle(now), true
		}
		slot := offset(lead.pos, b.behind(lead, from, now)+v.angle, v.radius)
		v.dest = b.walker().inSCC(lead.pos, slot)
		v.dueAt, v.anchor, v.anchored = time.Time{}, lead.pos, true
		v.traveling, v.stuck = true, false
	}
	if !v.traveling {
		return Action{}, true
	}
	a, st := b.goTo(v.dest, followArrive, purpose, now)
	switch st {
	case walkArrived:
		v.traveling = false
	case walkUnreachable:
		v.traveling, v.stuck = false, true
	}
	return a, true
}

// leader is where the leader is: in sight, or at its last reported
// position; false before either is known.
func (b *Brain) leader(id int32, now time.Time) (leaderView, bool) {
	if o := b.world.Objects[id]; o != nil {
		return leaderView{pos: o.Position(now), obj: o}, true
	}
	if m := b.world.Party.Members[id]; m != nil && !m.PosAt.IsZero() {
		return leaderView{pos: m.Pos}, true
	}
	return leaderView{}, false
}

// behind is the direction from the leader to straight behind it: opposite
// where it is going, where it came from since the last slot, or the way it
// faces. A leader out of sight standing where it stood has no known front:
// the slot is on the bot's side of it.
func (b *Brain) behind(lead leaderView, from Point, now time.Time) float64 {
	v, o, at := &b.follow, lead.obj, lead.pos
	switch {
	case o != nil && o.Moving(now):
		return math.Atan2(float64(at.Y-o.move.to.Y), float64(at.X-o.move.to.X))
	case v.anchored && (at.X != v.anchor.X || at.Y != v.anchor.Y):
		return math.Atan2(float64(v.anchor.Y-at.Y), float64(v.anchor.X-at.X))
	case o != nil:
		return float64(o.Heading)*2*math.Pi/headingUnits + math.Pi
	}
	return math.Atan2(float64(from.Y-at.Y), float64(from.X-at.X))
}

// followDelay is the wait between noticing the leader moved and moving.
func (b *Brain) followDelay() time.Duration {
	p := &b.persona.Follow
	return p.DelayMin + time.Duration(b.rng.Int64N(int64(max(p.DelayMax-p.DelayMin, 0))+1))
}

// wakeBy brings the next decision forward to t at the latest.
func (b *Brain) wakeBy(t time.Time) {
	if b.reactAt.IsZero() || t.Before(b.reactAt) {
		b.reactAt = t
	}
}

func walkRunAction() Action {
	return Action{kind: actWalkRun, payload: encodeRequestActionUse(actionWalkRun)}
}
