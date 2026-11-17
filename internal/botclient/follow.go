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
	// dest is the slot the bot is on its way to while traveling;
	// lastDist and stalls track the legs as rule "spot" does. A stuck
	// bot that gave up waits for the leader to move on.
	dest      Point
	traveling bool
	stuck     bool
	lastDist  float64
	stalls    int
	// runToggledAt is the last walk/run toggle not yet settled.
	runToggledAt time.Time
}

// leaderView is what the bot knows of its leader at a decision.
type leaderView struct {
	pos Point
	// obj is the leader while in sight, nil when pos is its last
	// PartyMemberPosition.
	obj *Object
}

// ruleFollow keeps a party member in its slot behind a leader other than
// itself: it takes the slot when it joins, and again only once the leader
// moved Repath away from where it stood then (or the bot drifted farther
// than MaxDist+Repath from it, after a fight), DelayMin to DelayMax after
// noticing. It matches the leader's walk or run, and stands up before it
// moves. A leader out of sight is followed to its last PartyMemberPosition
// (sent every 12 s), in legs as rule "spot" walks. While following, the rule
// holds the turn: no farming spot, goal or idle drift pulls the bot away.
// Server-side follow (a second Action on the leader) is not used: it glues
// the bot to the leader.
//
// Each move is one MoveBackwardToLocation and each toggle one
// RequestActionUse, at most one per decision, through the frame pacer
// (gateNone); a toggle waits toggleSettle for the server's answer, and a
// slot that spotStalls legs bring no closer is given up until the leader
// moves on.
func (b *Brain) ruleFollow(now time.Time) (Action, bool) {
	p, w, v := &b.persona.Follow, b.world, &b.follow
	id := w.Party.LeaderID
	if p.MaxDist <= 0 || !w.Party.In() || id == w.Self.ObjectID {
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
	s := &w.Self
	if run := lead.obj == nil || lead.obj.Running; run != s.Running && now.Sub(v.runToggledAt) >= toggleSettle {
		return walkRunAction(), true
	}
	from := s.Position(now)
	moved := !v.anchored || dist2(lead.pos, v.anchor) > int64(p.Repath)*int64(p.Repath)
	far := max(int64(p.MaxDist)+int64(p.Repath), 0)
	drifted := !v.traveling && !v.stuck && !s.Moving(now) && dist2(from, lead.pos) > far*far
	if (moved || drifted) && v.dueAt.IsZero() {
		v.dueAt = now.Add(b.followDelay())
		b.wakeBy(v.dueAt)
	}
	if !v.dueAt.IsZero() {
		if now.Before(v.dueAt) {
			return Action{}, true
		}
		if s.Sitting {
			return b.toggle(now), true
		}
		v.dest = offset(lead.pos, b.behind(lead, from, now)+v.angle, v.radius)
		v.dueAt, v.anchor, v.anchored = time.Time{}, lead.pos, true
		v.traveling, v.stuck, v.lastDist, v.stalls = true, false, 0, 0
		return b.followLeg(from), true
	}
	if v.traveling && !s.Moving(now) {
		if s.Sitting {
			return b.toggle(now), true
		}
		return b.followLeg(from), true
	}
	return Action{}, true
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

// followLeg is the next move toward the slot, or nothing once the bot is
// there or the last legs got it no closer.
func (b *Brain) followLeg(from Point) Action {
	v := &b.follow
	d := math.Sqrt(float64(dist2(from, v.dest)))
	if d <= followArrive {
		v.traveling = false
		return Action{}
	}
	if v.lastDist > 0 && v.lastDist-d < spotProgress {
		v.stalls++
	} else {
		v.stalls = 0
	}
	if v.stalls >= spotStalls {
		v.traveling, v.stuck = false, true
		return Action{}
	}
	v.lastDist = d
	return moveAction(legToward(from, v.dest, d), from)
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
