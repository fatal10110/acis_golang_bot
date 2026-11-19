package botclient

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// SpotPolicy is how a bot with a spot registry (BrainConfig.Spots) picks
// and keeps its farming spot (rule "spot"). Every value is a configurable
// default (D6).
type SpotPolicy struct {
	// A spot fits a bot of level L when none of its monsters is above
	// L+Below, none of its aggressive ones (aggroRange above 0) above
	// L+AggroAbove, and its highest one is at least L-Above.
	Below, AggroAbove, Above int
	// Newbie is the highest level at which aggressive monsters are bound
	// by Below rather than AggroAbove: the server takes any monster whose
	// template has an aggro range for aggressive (npc Hostile.Aggressive),
	// and nearly every starter monster has one, so a new character held to
	// AggroAbove finds no spot near its village (#133).
	Newbie int
	// Outlevel: the bot leaves its spot for a fitting one once its level is
	// above the spot's highest monster by more than Outlevel. Larger than
	// Above, it keeps the bot from flipping between spots: the spot it
	// leaves no longer fits.
	Outlevel int
	// MaxTravel is the farthest spot the bot picks, from where it stands,
	// while one fits it with room: a new character stands in its race's
	// newbie zone, so it farms there. It is also the gatekeeper trip's walk
	// at either end (planTrip).
	MaxTravel int32
	// Wander is the farthest spot the bot walks to once every spot that
	// fits it within MaxTravel is full; a smaller one than MaxTravel adds
	// nothing.
	Wander int32
	// Overflow: once every spot that fits the bot within Wander is full and
	// it takes no trip, it shares the one its bots fill least for its cap,
	// up to one bot per monster the server keeps there, rather than wait
	// (#137). Each spotRetry it looks again for a spot with room.
	Overflow bool
	// Crowd is the number of other players within Leash of the spot's
	// centre, party members and the bots sharing the spot's claim aside,
	// that makes a spot crowded on arrival: the bot moves on and avoids the
	// spot for AvoidFor, as it does a spot it cannot walk to. 0 never
	// finds one crowded.
	Crowd    int
	AvoidFor time.Duration
	// Leash is how far from the spot's centre the bot may end a fight
	// before it walks back.
	Leash int32
}

// defaultSpotPolicy is the farmer's spot choice: monsters up to two levels
// above the bot, aggressive ones not above it (plan, farmer) once it is
// past level 5, leaving at five levels over; spread over every spot that
// fits within twice the newbie zone's reach, sharing full ones rather than
// waiting (#137).
func defaultSpotPolicy() SpotPolicy {
	return SpotPolicy{
		Below: 2, AggroAbove: 0, Above: 3, Newbie: 5, Outlevel: 5,
		MaxTravel: 20000, Wander: 40000, Overflow: true,
		Crowd: 4, AvoidFor: 10 * time.Minute, Leash: 1500,
	}
}

// fits reports whether s fits a bot of level.
func (p *SpotPolicy) fits(s *datapack.Spot, level int) bool {
	aggro := p.AggroAbove
	if level <= p.Newbie {
		aggro = max(aggro, p.Below)
	}
	return s.MaxLevel <= level+p.Below && s.AggroLevel <= level+aggro && s.MaxLevel >= level-p.Above
}

const (
	// spotArrive is how close to a spot's centre the bot has arrived.
	spotArrive = 200
	// spotLeg is the longest move toward a spot, well under the 9900 the
	// server refuses (movement.go moveLivePlayer).
	spotLeg = 3000
	// spotDirect is the distance from which the bot walks to the spot's
	// centre in one move, under the 9900 cap: the server's pathfinder
	// routes the whole way around cliffs and walls, where a leg's end on
	// the straight line may lie on ground no path reaches (the orc start,
	// #133).
	spotDirect = 9000
	// spotProgress is how much closer each leg must bring the bot;
	// spotStalls legs in a row short of it give the spot up.
	spotProgress = 100
	spotStalls   = 3
	// spotRetry is how long the bot waits to pick again after finding no
	// spot free that fits.
	spotRetry = time.Minute
)

// spotVisit is the bot's farming spot (rule "spot"). It is owned by the
// brain goroutine; the claim it holds is counted in the SpotRegistry.
type spotVisit struct {
	// idx is the spot held, when held.
	idx  int
	held bool
	// arrived is set once the bot reached the spot's centre.
	arrived bool
	// lastDist is the distance left when the last leg went out, 0 before
	// one; stalls counts legs in a row that got nowhere.
	lastDist float64
	stalls   int
	// retryAt is when the bot may look for a spot again after finding none.
	retryAt time.Time
	// avoid are the spots the bot moved on from, until a time.
	avoid map[int]time.Time
}

// ruleSpot is a rule 9 errand ahead of the persona's goal: with a spot
// registry, the bot claims a fitting spot (pickSpot), walks there in legs,
// moves on when it finds the spot crowded, walks back when a fight left it
// beyond the leash, and picks a new spot once it outlevels its own or,
// sharing one past its cap, every spotRetry. At its spot it leaves the turn
// to rule "farm".
//
// Each leg is one MoveBackwardToLocation through the frame pacer (gateNone);
// a spot that spotStalls legs bring no closer is avoided, and a fruitless
// search waits spotRetry, so a bot that cannot get anywhere stops sending.
func (b *Brain) ruleSpot(now time.Time) (Action, bool) {
	r, s, v := b.cfg.Spots, &b.world.Self, &b.spot
	if r == nil || s.InCombat {
		return Action{}, false
	}
	if !now.Before(v.retryAt) && (!v.held || b.outleveled() || r.overCap(v.idx)) {
		b.pickSpot(now)
	}
	if !v.held {
		return Action{}, false
	}
	sp := &r.spots[v.idx]
	dest := Point{X: sp.Center.X, Y: sp.Center.Y, Z: sp.Z}
	from := s.Position(now)
	d := math.Sqrt(float64(dist2(from, dest)))
	if !v.arrived {
		if s.Moving(now) {
			return Action{}, true
		}
		if d > spotArrive {
			return b.spotLeg(dest, from, d, now), true
		}
		v.arrived, v.lastDist, v.stalls = true, 0, 0
		b.anchor = from
		if b.crowded(dest, now) {
			b.moveOn(now)
			return Action{}, true
		}
		return Action{}, false
	}
	if d > float64(b.persona.Spots.Leash) && !s.Moving(now) {
		return b.spotLeg(dest, from, d, now), true
	}
	v.lastDist, v.stalls = 0, 0
	return Action{}, false
}

// spotLeg is the next move toward dest, d away from from: standing up
// first, giving the spot up when the last legs got nowhere, straight to
// dest within spotDirect in 3D (d is the distance on the X/Y plane).
func (b *Brain) spotLeg(dest, from Point, d float64, now time.Time) Action {
	v := &b.spot
	if b.world.Self.Sitting {
		return b.toggle(now)
	}
	if v.lastDist > 0 && v.lastDist-d < spotProgress {
		v.stalls++
	} else {
		v.stalls = 0
	}
	if v.stalls >= spotStalls {
		b.moveOn(now)
		return Action{}
	}
	v.lastDist = d
	// The server checks the move's 3D length (movement.go), so the height
	// between here and the centre counts too.
	if math.Hypot(d, float64(dest.Z-from.Z)) <= spotDirect {
		return moveAction(dest, from)
	}
	return moveAction(legToward(from, dest, d), from)
}

// legToward is the point at most spotLeg along the line from from to to, d
// away, z in proportion.
func legToward(from, to Point, d float64) Point {
	if d <= spotLeg {
		return to
	}
	f := spotLeg / d
	return Point{
		X: from.X + int32(math.Round(float64(to.X-from.X)*f)),
		Y: from.Y + int32(math.Round(float64(to.Y-from.Y)*f)),
		Z: from.Z + int32(math.Round(float64(to.Z-from.Z)*f)),
	}
}

// crowded reports whether Persona.Spots.Crowd players or more stand within
// Leash of the spot's centre at, party members aside and less the other bots
// holding the spot: the registry's cap already counts those, so a spot under
// its cap is not crowded by its own holders.
func (b *Brain) crowded(at Point, now time.Time) bool {
	p := &b.persona.Spots
	if p.Crowd <= 0 {
		return false
	}
	leash := int64(p.Leash) * int64(p.Leash)
	n := 0
	for id, o := range b.world.Objects {
		if o.Kind != KindPlayer || o.Dead || b.world.Party.Members[id] != nil {
			continue
		}
		if dist2(o.Position(now), at) <= leash {
			n++
		}
	}
	return n-(b.cfg.Spots.Held(b.spot.idx)-1) >= p.Crowd
}

// outleveled reports whether the bot's level is past its spot's highest
// monster by more than Persona.Spots.Outlevel.
func (b *Brain) outleveled() bool {
	v := &b.spot
	return v.held && int(b.world.Self.Level) > b.cfg.Spots.spots[v.idx].MaxLevel+b.persona.Spots.Outlevel
}

// pickSpot claims a spot that fits the bot's level and is not avoided, in
// place of the one it holds (#137): within MaxTravel, then within Wander,
// the nearest that no bot holds, else the nearest with room. With none, it
// looks again after spotRetry and plans a trip by gatekeeper to a spot out
// of walking reach (rule "travel"); with no trip either, a bot holding no
// spot that fits shares, past its cap, the spot in Wander its bots fill
// least (SpotPolicy.Overflow), and one that holds a spot that fits keeps it.
func (b *Brain) pickSpot(now time.Time) {
	r, v, p := b.cfg.Spots, &b.spot, &b.persona.Spots
	level, from := int(b.world.Self.Level), b.world.Self.Position(now)
	near := int64(p.MaxTravel) * int64(p.MaxTravel)
	far := max(near, int64(p.Wander)*int64(p.Wander))
	type cand struct {
		i int
		d int64
	}
	var cands []cand
	for i := range r.spots {
		sp := &r.spots[i]
		if (v.held && i == v.idx) || now.Before(v.avoid[i]) || !p.fits(sp, level) {
			continue
		}
		if d := dist2(from, Point{X: sp.Center.X, Y: sp.Center.Y}); d <= far {
			cands = append(cands, cand{i, d})
		}
	}
	slices.SortStableFunc(cands, func(a, b cand) int { return cmp.Compare(a.d, b.d) })
	order := make([]int, len(cands))
	n := 0
	for k, c := range cands {
		order[k] = c.i
		if c.d <= near {
			n = k + 1
		}
	}
	i, ok := r.claim(order[:n])
	if !ok {
		i, ok = r.claim(order[n:])
	}
	if !ok {
		v.retryAt = now.Add(spotRetry)
		b.planTrip(now)
		if !p.Overflow || b.trip.v.active() || (v.held && !b.outleveled()) {
			return
		}
		if i, ok = r.claimOver(order); !ok {
			return
		}
	}
	b.leaveSpot()
	v.idx, v.held = i, true
}

// moveOn gives the spot up and avoids it for Persona.Spots.AvoidFor.
func (b *Brain) moveOn(now time.Time) {
	v := &b.spot
	if v.avoid == nil {
		v.avoid = make(map[int]time.Time)
	}
	for i, until := range v.avoid {
		if !now.Before(until) {
			delete(v.avoid, i)
		}
	}
	v.avoid[v.idx] = now.Add(b.persona.Spots.AvoidFor)
	b.leaveSpot()
}

// leaveSpot gives back the spot the bot holds, if any: on a move, on death
// and when the brain stops.
func (b *Brain) leaveSpot() {
	v := &b.spot
	if v.held {
		b.cfg.Spots.release(v.idx)
	}
	v.idx, v.held, v.arrived, v.lastDist, v.stalls = 0, false, false, 0, 0
}
