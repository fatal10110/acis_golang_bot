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
	// followDetourStep and followDetourLegs are a detour around a wall the
	// legs stall at: followDetourLegs legs of followDetourStep along the
	// wall, followDetourTurn off the line to the slot: past square to it,
	// so a wall the line meets aslant does not stop the detour at once.
	// followDetours bounds the detours at one wall: the legs leave it
	// behind once they bring the bot followDetourStep closer than where its
	// first detour started.
	followDetourTurn = 2 * math.Pi / 3
	followDetourStep = 600
	followDetourLegs = 2
	followDetours    = 8
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
	// dest is the slot the bot is on its way to while traveling;
	// lastDist and stalls track the legs as rule "spot" does, and legFrom
	// is where the bot stood when the last leg went out. Legs that stall
	// where the bot stands try the sidesteps of an unstick first: unsticks
	// counts them at the stall the bot is getting past, stuckAt is where
	// the bot stood for the last one and stuckDist how far from dest it
	// stood for the first. Legs that bring it spotProgress closer than
	// stuckDist got past that stall: the next one has every sidestep again.
	// A stuck bot that gave up waits for the leader to move on.
	//
	// A sidestep that got the bot off a stall starts a detour along the
	// wall (followDetour): side is the way around it, +1 counterclockwise
	// from the line to dest and -1 clockwise, the way of that sidestep or,
	// for one straight back, the way the trip went last; it is kept for the
	// leader, so the walls of a trip are gone around on one side. detour is
	// the direction of the detour under way (radians), detourLegs its legs
	// still to go and detourFrom where it started; detouring is set while
	// one is under way and flipped once it turned to the other side.
	// detours counts those at the wall the bot is getting past, and wallDist
	// is how far from dest the first of them started.
	dest      Point
	traveling bool
	stuck     bool
	lastDist  float64
	legFrom   Point
	stalls    int
	unsticks  int
	stuckAt   Point
	stuckDist float64

	side       float64
	detour     float64
	detourLegs int
	detourFrom Point
	detouring  bool
	flipped    bool
	detours    int
	wallDist   float64
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
// itself, or any bot behind the commander a GM told it to follow (followed): it takes the slot when it joins, and again only once the leader
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
// (gateNone); a toggle waits toggleSettle for the server's answer. Legs
// that stall where the bot stands sidestep as a visit does (followUnstick),
// and a sidestep that moves the bot detours along the wall (startDetour), as
// moves through the same pacer; a slot the legs, sidesteps and detours bring
// no closer is given up until the leader moves on.
func (b *Brain) ruleFollow(now time.Time) (Action, bool) {
	p, w, v := &b.persona.Follow, b.world, &b.follow
	id := b.followed()
	if p.MaxDist <= 0 || id == 0 || id == w.Self.ObjectID {
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
		v.dueAt, v.wake = now.Add(b.followDelay()), true
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
		v.traveling, v.stuck, v.lastDist, v.stalls, v.unsticks = true, false, 0, 0, 0
		// A detour was around a wall on the way to the old slot.
		v.detouring, v.flipped, v.detours, v.wallDist = false, false, 0, 0
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
// there or the last legs got it no closer. Legs that stall where the bot
// stands, the server echoing its own position (acis_golang#5412), try an
// unstick first; a sidestep that moves the bot starts a detour along the wall
// the legs stalled at (startDetour) before the legs go again.
func (b *Brain) followLeg(from Point) Action {
	v := &b.follow
	d := math.Sqrt(float64(dist2(from, v.dest)))
	if d <= followArrive {
		v.traveling = false
		return Action{}
	}
	if v.detouring {
		if a, ok := b.followDetour(from); ok {
			return a
		}
	}
	if v.stalls >= spotStalls {
		if !budged(from, v.stuckAt) {
			return b.followUnstick(from, d)
		}
		// A sidestep moved the bot: around the wall, then legs again from
		// where it stands.
		v.lastDist, v.stalls = 0, 0
		if v.detours < followDetours {
			return b.startDetour(from, d)
		}
	}
	if v.lastDist > 0 && v.lastDist-d < spotProgress {
		v.stalls++
	} else {
		v.stalls = 0
		if v.unsticks > 0 && v.stuckDist-d >= spotProgress {
			// Past the stall the sidesteps were for.
			v.unsticks = 0
		}
		if v.detours > 0 && v.wallDist-d >= followDetourStep {
			// Past the wall the detours were for.
			v.detours = 0
		}
	}
	if v.stalls >= spotStalls {
		if budged(from, v.legFrom) {
			// The bot walks, just no closer: no way there.
			v.traveling, v.stuck = false, true
			return Action{}
		}
		return b.followUnstick(from, d)
	}
	v.lastDist, v.legFrom = d, from
	return moveAction(legToward(from, v.dest, d), from)
}

// startDetour starts a detour of a bot at from, d away from its slot, that a
// sidestep just got off a stall. Legs aimed straight at the slot from anywhere near hit the same
// wall again (acis_golang#5412 refuses them cell after cell along a band
// several hundred units wide, #141), so the detour walks along it first: on
// the trip's side, or for the trip's first on the side of that sidestep
// (counterclockwise for one straight back).
func (b *Brain) startDetour(from Point, d float64) Action {
	v := &b.follow
	if v.side == 0 {
		v.side = 1
		if sidestepTurns[v.unsticks-1] < 0 {
			v.side = -1
		}
	}
	if v.detours == 0 {
		v.wallDist = d
	}
	v.detours++
	v.detouring, v.flipped, v.detourFrom = true, false, from
	v.detourLegs = followDetourLegs
	return b.detourLeg(from)
}

// followDetour is the next leg of the detour under way, of a bot at from,
// false once it is over. A leg that moved the bot less than spotProgress
// found a wall on that side too: the trip goes around on the other side from
// then on, and a detour whose first leg did not move the bot turns to it at
// once. A detour that moved the bot ends where the legs did not stall yet, so
// the next stall has every sidestep again; followDetours bound how often at
// one wall.
func (b *Brain) followDetour(from Point) (Action, bool) {
	v := &b.follow
	blocked := dist2(from, v.legFrom) < spotProgress*spotProgress
	if blocked {
		// A wall on this side too: the other side, at once when this one
		// did not move the bot at all, else for the next detour.
		v.side = -v.side
		if !v.flipped && v.detourLegs == followDetourLegs-1 {
			v.flipped, v.detourLegs = true, followDetourLegs
			return b.detourLeg(from), true
		}
	} else if v.detourLegs > 0 {
		return b.detourLeg(from), true
	}
	v.detouring = false
	if dist2(from, v.detourFrom) >= spotProgress*spotProgress {
		v.unsticks = 0
	}
	v.lastDist, v.stalls = 0, 0
	return Action{}, false
}

// detourLeg is the next leg of the detour under way from from:
// followDetourStep along v.detour.
func (b *Brain) detourLeg(from Point) Action {
	v := &b.follow
	if v.detourLegs == followDetourLegs {
		v.detour = math.Atan2(float64(v.dest.Y-from.Y), float64(v.dest.X-from.X)) + v.side*followDetourTurn
	}
	v.detourLegs--
	v.legFrom = from
	sin, cos := math.Sincos(v.detour)
	to := Point{
		X: from.X + int32(math.Round(cos*followDetourStep)),
		Y: from.Y + int32(math.Round(sin*followDetourStep)),
		Z: from.Z,
	}
	return moveAction(to, from)
}

// followUnstick is the next sidestep of a bot at from, d away from its
// slot, whose legs stall where it stands: one in each of sidestepTurns at
// each stall on the way to the slot, then the slot is given up until the
// leader moves on. Each stall the legs get past brings the bot spotProgress
// closer, so a trip has at most d/spotProgress of them.
// No /unstuck: its recall lands in the nearest town, farther from a leader
// out in the field, and the leader may be in that town already.
func (b *Brain) followUnstick(from Point, d float64) Action {
	v := &b.follow
	if v.unsticks >= len(sidestepTurns) {
		v.traveling, v.stuck = false, true
		return Action{}
	}
	if v.unsticks == 0 {
		v.stuckDist = d
	}
	turn := sidestepTurns[v.unsticks]
	v.unsticks++
	v.stuckAt = from
	return moveAction(sidestep(from, v.dest, d, turn), from)
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
