package botclient

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
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
	// MaxTravel is the farthest spot, in straight line, the bot picks while
	// one fits it with room: a new character stands in its race's newbie
	// zone, so it farms there. Within it the spots rank by the walk there
	// (pickSpot). It also bounds the straight line of a gatekeeper trip's
	// walks (planTrip).
	MaxTravel int32
	// Wander is the farthest spot, in straight line, the bot walks to once
	// every spot that fits it within MaxTravel is full; a smaller one than
	// MaxTravel adds nothing.
	Wander int32
	// Overflow: once every spot that fits the bot within Wander is full and
	// it takes no trip, it shares the one its bots fill least for its cap,
	// up to one bot per monster the server keeps there, rather than wait
	// (#137). Each spotRetry it looks again for a spot with room.
	Overflow bool
	// Crowd is the number of other players within Leash of the spot's
	// centre, party members and the bots sharing the spot's claim aside,
	// that makes a spot crowded on arrival: the bot moves on and avoids the
	// spot for AvoidFor (one whose walk ended unreachable, walkRetry). 0
	// never finds one crowded.
	Crowd    int
	AvoidFor time.Duration
	// DeathAvoid is how long a bot that died holding a spot, at it or on
	// its way there, avoids that spot: one whose packs killed it once
	// likely kill it again, and it would walk straight back after the
	// respawn (#258). 0 never avoids one.
	DeathAvoid time.Duration
	// Leash is how far from the spot's centre the bot may end a fight
	// before it walks back.
	Leash int32
	// PackShare widens a death's DeathAvoid to the spots like the one the
	// bot died holding, when that one's aggressive monsters run in a pack
	// (spotPack above 1): for DeathAvoid it also passes over every spot
	// whose pack is at least PackShare of that one's and whose aggressive
	// monsters stand as high above the bot or higher (packBound). Talking
	// Island's orc and werewolf makers kill a starter-geared fighter of
	// level 14 at each in turn (#281); the gear its stats come from is not
	// in the datapack, so a death is what tells it a pack is too much. 0
	// avoids the one spot alone.
	PackShare float64
	// PackRisk passes over a spot whose pack deals the bot more than
	// PackRisk times what it can take while it kills one of them: its max
	// HP plus what the healing potions it carries restore in that time
	// (packRisk, from the bot's own stats against the monsters'). It judges
	// a pack before a death there (#294): to the level 14 fighter in
	// Squire's gear of TestLivePackSpot, with 13 Lesser Healing Potions,
	// Talking Island's werewolf pack that killed it is about 4.9 and the
	// orc pack it farmed until its potions ran out about 3.4; to a level 6
	// one with 12 its toad spots are 1.8 to 2.3. Regeneration, shots, the
	// fight's lulls and a pack drawn in turn are left out, so it is a ratio
	// against the worst case rather than a death line. Holding a spot over
	// it, the bot moves to one that fits (overRisk). A mystic's spells are
	// left out too, so a mystic is not judged. 0, the default, never passes
	// one over: on the worst case, a healthy level 6 squire with no potion
	// is over 3 at the toads too (3.5 to 4.3), and a character weakened by
	// its death penalty fits nothing (TestLiveSupplyTrip's, at level 6 with
	// P.Atk 2, is over 12 at the toads with 12 potions); 3 is the value the
	// cases above are judged at. The per-build margins of #330 replace it.
	PackRisk float64
}

// defaultSpotPolicy is the farmer's spot choice: monsters up to two levels
// above the bot, aggressive ones not above it (plan, farmer) once it is
// past level 5, leaving at five levels over; spread over every spot that
// fits within twice the newbie zone's reach, sharing full ones rather than
// waiting (#137); a death at a pack passes over its like (#281). PackRisk
// is off: see its doc.
func defaultSpotPolicy() SpotPolicy {
	return SpotPolicy{
		Below: 2, AggroAbove: 0, Above: 3, Newbie: 5, Outlevel: 5,
		MaxTravel: 20000, Wander: 40000, Overflow: true,
		Crowd: 4, AvoidFor: 10 * time.Minute, DeathAvoid: 30 * time.Minute, Leash: 1500,
		PackShare: 0.8,
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

// spotPack is how many of s's aggressive monsters a fight there likely
// draws: those of them, spread evenly over its territory, within its aggro
// range plus its clan call range of the bot, as a monster that sees the bot
// attacks it (npc hostile_target.go) and calls the members of its clan
// within ClanRange around it (npc/clan.go).
func spotPack(s *datapack.Spot) float64 {
	if s.Aggressive == 0 || s.Area <= 0 {
		return 0
	}
	r := float64(s.AggroRange + s.ClanRange)
	return float64(s.Aggressive) * min(1, math.Pi*r*r/s.Area)
}

// spotFits reports whether s fits the bot at now: its level, no pack a
// death taught it to pass over, and no pack too much for its stats
// (SpotPolicy.PackRisk).
func (b *Brain) spotFits(s *datapack.Spot, now time.Time) bool {
	self := &b.world.Self
	level := int(self.Level)
	return b.persona.Spots.fits(s, level) && !b.spot.pack.covers(s, level, self.RunSpeed, now) && b.packFits(s)
}

// packFits reports whether s's pack is within SpotPolicy.PackRisk.
func (b *Brain) packFits(s *datapack.Spot) bool {
	p := &b.persona.Spots
	return p.PackRisk <= 0 || b.packRisk(s) <= p.PackRisk
}

// packBound is the pack a death at one taught the bot to pass over
// (SpotPolicy.PackShare), until a time.
type packBound struct {
	// pack is the smallest spotPack covered, above how many levels over
	// the bot its aggressive monsters stand at least.
	pack  float64
	above int
	// fast covers only the packs the bot cannot outrun: it died at one, so
	// that a flee might yet save it from a slower one. A death at a pack it
	// outran covers them all.
	fast  bool
	until time.Time
}

// covers reports whether s is a pack the bound has a bot of level that runs
// at run pass over at now.
func (k *packBound) covers(s *datapack.Spot, level int, run int32, now time.Time) bool {
	return now.Before(k.until) && s.Aggressive > 0 && spotPack(s) >= k.pack && s.AggroLevel-level >= k.above &&
		(!k.fast || s.RunSpeed >= float64(run))
}

// learnPack widens the bound to the packs like sp on a death holding it at
// now, for DeathAvoid; a bound still running keeps covering what it did.
func (b *Brain) learnPack(sp *datapack.Spot, now time.Time) {
	p, self, k := &b.persona.Spots, &b.world.Self, &b.spot.pack
	n := spotPack(sp)
	if p.PackShare <= 0 || p.DeathAvoid <= 0 || n <= 1 {
		return
	}
	next := packBound{pack: n * p.PackShare, above: sp.AggroLevel - int(self.Level), fast: sp.RunSpeed >= float64(self.RunSpeed), until: now.Add(p.DeathAvoid)}
	if now.Before(k.until) {
		next.pack, next.above, next.fast = min(next.pack, k.pack), min(next.above, k.above), next.fast && k.fast
	}
	*k = next
}

const (
	// spotArrive is how close to a spot's centre the bot has arrived.
	spotArrive = 200
	// spotRetry is how long the bot waits to pick again after finding no
	// spot free that fits.
	spotRetry = time.Minute
	// spotSnap is how far from a spot's centre the bot looks for walkable
	// ground inside the spot's territory when the centre itself lies on
	// none (a wall, a ledge, another SCC).
	spotSnap = 400
)

// spotVisit is the bot's farming spot (rule "spot"). It is owned by the
// brain goroutine; the claim it holds is counted in the SpotRegistry.
type spotVisit struct {
	// idx is the spot held, when held, and at its centre on walkable
	// ground inside its territory, in the bot's SCC when it was claimed
	// (spotPoint): where the bot walks and stays.
	idx  int
	held bool
	at   Point
	// arrived is set once the bot reached the spot's centre.
	arrived bool
	// retryAt is when the bot may look for a spot again after finding none.
	retryAt time.Time
	// avoid are the spots the bot moved on from, until a time.
	avoid map[int]time.Time
	// pack is the pack its deaths taught it to pass over.
	pack packBound
}

// ruleSpot is a rule 9 errand ahead of the persona's goal: with a spot
// registry, the bot claims a fitting spot it can walk to (pickSpot), walks
// there through the walker, moves on when it finds the spot crowded, walks
// back when a fight left it beyond the leash (not while it walks around to
// a farm target, farmWalking), and picks a new spot once it
// outlevels its own or, sharing one past its cap, every spotRetry. At its
// spot it leaves the turn to rule "farm". A bot holding none claims none
// while the town preparation gate holds it (preparing).
//
// Every move is the walker's (goTo): one MoveBackwardToLocation per checked
// segment through the frame pacer (gateNone). A walk that ends unreachable
// gives the spot up for walkRetry, and a fruitless search waits spotRetry,
// so a bot that cannot get anywhere stops sending.
func (b *Brain) ruleSpot(now time.Time) (Action, bool) {
	r, s, v := b.cfg.Spots, &b.world.Self, &b.spot
	if r == nil || s.InCombat {
		return Action{}, false
	}
	if !v.held && b.preparing() {
		// The town preparation gate: no spot before the errands ahead
		// stocked the bot up.
		return Action{}, false
	}
	if !now.Before(v.retryAt) && (!v.held || b.outleveled() || b.overRisk() || r.overCap(v.idx)) {
		b.pickSpot(now)
	}
	if !v.held {
		return Action{}, false
	}
	if v.arrived {
		leash := int64(b.persona.Spots.Leash)
		if horizontal2(s.Position(now), v.at) <= leash*leash || s.Moving(now) || b.farmWalking(now) {
			return Action{}, false
		}
	}
	a, st := b.goTo(v.at, spotArrive, walkPlace, now)
	switch st {
	case walkWalking:
		return a, true
	case walkUnreachable:
		// The walk the abstract graph promised does not hold (blocked,
		// desync, a closed door): the walker refuses this spot for
		// walkRetry anyway, so look for another.
		b.moveOn(now, walkRetry)
		return Action{}, false
	}
	if v.arrived {
		return Action{}, false
	}
	v.arrived = true
	b.anchor = s.Position(now)
	if b.crowded(v.at, now) {
		b.moveOn(now, b.persona.Spots.AvoidFor)
		return Action{}, true
	}
	return Action{}, false
}

// farmWalking reports whether the bot is under way to the stand point it
// fights its farm target from: the target lies within the leash, the walk
// to it need not (around a wall, a rock, a slope). Walked back to the spot
// from a segment's end beyond the leash, the bot set out again on the same
// walk at the next decision, back and forth with no attack (#269). The walk
// ends at the stand point, or unreachable, which blacklists the target
// (engageWalk); the leash holds again from there.
func (b *Brain) farmWalking(now time.Time) bool {
	w, e := &b.walk, &b.engage
	return e.id != 0 && (w.purpose == walkMelee || w.purpose == walkRanged) && w.status == walkWalking &&
		horizontal2(w.dest, e.stand) <= walkSameDest*walkSameDest && e.id == b.farmTarget(now)
}

// navPoint is a point on walkable ground and its place in the hierarchy.
type navPoint struct {
	at Point
	m  nav.Member
}

// locate is p on the layer nearest its Z with its place in the hierarchy;
// false without navigation or off the nav cache.
func (b *Brain) locate(p Point) (navPoint, bool) {
	w := &b.walk
	if w.nav == nil {
		return navPoint{}, false
	}
	c, ok := w.plan.Locate(toLoc(p))
	if !ok {
		return navPoint{}, false
	}
	m, err := w.nav.Hier.Locate(c, nil)
	if err != nil {
		return navPoint{}, false
	}
	p.Z = int32(c.Z)
	return navPoint{at: p, m: m}, true
}

// navDistance is the walking distance from from to to in world units by the
// abstract graph, every door open and nothing refined (docs/navigation.md,
// "Distances for ranking"); false when to is not in from's SCC or the search
// ran out of its budget. From ground in no abstract SCC (one way out) it is
// the straight-line distance to a to that ground reaches at all: the walker
// drops into an SCC first.
func (b *Brain) navDistance(from, to navPoint) (float64, bool) {
	h := b.walk.nav.Hier
	if from.m.Kind() == nav.OneWayOut {
		if !h.Reachable(from.m, to.m) {
			return 0, false
		}
		return math.Sqrt(float64(horizontal2(from.at, to.at))), true
	}
	d, err := h.Distance(from.m, to.m)
	if err != nil {
		return 0, false
	}
	// Distance counts step costs: tenths of a cell.
	return float64(d) * block.CellSize / 10, true
}

// spotPoint is spot sp's centre on walkable ground inside its territory and
// reachable from from: the centre itself, else the walkable cell nearest it
// within spotSnap that is inside the territory and reachable; false when
// none is.
func (b *Brain) spotPoint(sp *datapack.Spot, from navPoint) (navPoint, bool) {
	c := Point{X: sp.Center.X, Y: sp.Center.Y, Z: sp.Z}
	if p, ok := b.locate(c); ok && b.walkable(from, p) {
		return p, true
	}
	var got navPoint
	_, ok := b.walk.plan.NearestWalkableFunc(toLoc(c), spotSnap, func(cell nav.Cell) bool {
		at := Point{X: int32(engine.WorldX(cell.X)), Y: int32(engine.WorldY(cell.Y)), Z: int32(cell.Z)}
		if sp.Territory != nil && !sp.Territory.Contains(datapack.Point{X: at.X, Y: at.Y}) {
			return false
		}
		p, ok := b.locate(at)
		if !ok || !b.walkable(from, p) {
			return false
		}
		got = p
		return true
	})
	return got, ok
}

// spotDest is where the bot walks for spot i when it is told to go there:
// its centre on walkable ground in the bot's SCC (spotPoint), else the
// centre as the datapack has it, which the walker finds unreachable.
func (b *Brain) spotDest(i int, now time.Time) Point {
	sp := &b.cfg.Spots.spots[i]
	if here, ok := b.locate(b.standing(now)); ok {
		if at, ok := b.spotPoint(sp, here); ok {
			return at.at
		}
	}
	return Point{X: sp.Center.X, Y: sp.Center.Y, Z: sp.Z}
}

// walkable reports whether the bot standing at from can walk to to: both in
// one SCC, or to reached by dropping off one-way-out ground.
func (b *Brain) walkable(from, to navPoint) bool {
	h := b.walk.nav.Hier
	if from.m.Kind() == nav.OneWayOut {
		return h.Reachable(from.m, to.m)
	}
	return h.SameSCC(from.m, to.m)
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

// overRisk reports whether the pack of the spot the bot holds is over
// SpotPolicy.PackRisk, as when it took the spot before its stats came in
// or its potions ran out: it looks for one that fits, and keeps the one it
// holds with none.
func (b *Brain) overRisk() bool {
	v := &b.spot
	return v.held && !b.packFits(&b.cfg.Spots.spots[v.idx])
}

// pickSpot claims a spot that fits the bot's level, is not avoided and that
// the bot can walk to, in place of the one it holds (#137): within MaxTravel
// in straight line, then within Wander, the nearest by the abstract graph
// (navDistance) that no bot holds, else the nearest with room. With none,
// it looks again after spotRetry and plans a trip by gatekeeper (rule
// "travel", planTrip), giving up the spot it holds for the trip; with no
// trip either, a bot holding no spot that fits shares, past its cap, the
// spot in Wander its bots fill least (SpotPolicy.Overflow), and one that
// holds a spot that fits keeps it. A spot in another SCC is never claimed.
func (b *Brain) pickSpot(now time.Time) {
	here, ok := b.locate(b.standing(now))
	if !ok {
		b.claimSpot(nil, now, nil)
		return
	}
	b.claimSpot(b.spotCands(here, now), now, &here)
}

// spotCand is a spot the bot may claim: its index, the walk there by the
// abstract graph (navDistance), the straight line there, and its centre on
// walkable ground (spotPoint).
type spotCand struct {
	i       int
	d, line float64
	at      Point
}

// spotCands are the spots that fit the bot standing at here, are not
// avoided and lie in its SCC within Wander in straight line, but the one it
// holds.
func (b *Brain) spotCands(here navPoint, now time.Time) []spotCand {
	r, v, p := b.cfg.Spots, &b.spot, &b.persona.Spots
	far := max(float64(p.MaxTravel), float64(p.Wander))
	var cands []spotCand
	for i := range r.spots {
		sp := &r.spots[i]
		if (v.held && i == v.idx) || now.Before(v.avoid[i]) || !b.spotFits(sp, now) {
			continue
		}
		line := math.Sqrt(float64(horizontal2(here.at, Point{X: sp.Center.X, Y: sp.Center.Y})))
		if line > far {
			continue
		}
		at, ok := b.spotPoint(sp, here)
		if !ok {
			continue
		}
		if d, ok := b.navDistance(here, at); ok {
			cands = append(cands, spotCand{i: i, d: d, line: line, at: at.at})
		}
	}
	return cands
}

// claimSpot claims one of cands as pickSpot says; here is where the bot
// stands for a trip, nil for none.
func (b *Brain) claimSpot(cands []spotCand, now time.Time, here *navPoint) {
	r, v, p := b.cfg.Spots, &b.spot, &b.persona.Spots
	slices.SortStableFunc(cands, func(a, b spotCand) int { return cmp.Compare(a.d, b.d) })
	order := make([]int, 0, len(cands))
	var beyond []int
	for _, c := range cands {
		if c.line <= float64(p.MaxTravel) {
			order = append(order, c.i)
		} else {
			beyond = append(beyond, c.i)
		}
	}
	n := len(order)
	order = append(order, beyond...)
	i, ok := r.claim(order[:n])
	if !ok {
		i, ok = r.claim(order[n:])
	}
	if !ok {
		v.retryAt = now.Add(spotRetry)
		if here != nil {
			b.planTrip(*here, now)
		}
		if b.trip.v.active() {
			// A bot on a trip farms near its destination: it gives back the
			// spot it holds (outlevelled, or shared past its cap) rather
			// than block a place there for the trip and walk back to it.
			b.leaveSpot()
		}
		if !p.Overflow || b.trip.v.active() || (v.held && !b.outleveled()) {
			return
		}
		if i, ok = r.claimOver(order); !ok {
			return
		}
	}
	// A spot claimed after a landing ends the path the trip was on.
	b.trip.onward = 0
	b.leaveSpot()
	v.idx, v.held = i, true
	for _, c := range cands {
		if c.i == i {
			v.at = c.at
		}
	}
}

// moveOn gives the spot up and avoids it for d.
func (b *Brain) moveOn(now time.Time, d time.Duration) {
	v := &b.spot
	if v.avoid == nil {
		v.avoid = make(map[int]time.Time)
	}
	for i, until := range v.avoid {
		if !now.Before(until) {
			delete(v.avoid, i)
		}
	}
	v.avoid[v.idx] = now.Add(d)
	b.leaveSpot()
}

// diedAtSpot gives back the spot the bot holds, if any, on its death,
// avoiding it, and the packs like it (learnPack), for
// SpotPolicy.DeathAvoid.
func (b *Brain) diedAtSpot(now time.Time) {
	if d := b.persona.Spots.DeathAvoid; b.spot.held && d > 0 {
		b.learnPack(&b.cfg.Spots.spots[b.spot.idx], now)
		b.moveOn(now, d)
		return
	}
	b.leaveSpot()
}

// leaveSpot gives back the spot the bot holds, if any: on a move, on death
// and when the brain stops.
func (b *Brain) leaveSpot() {
	v := &b.spot
	if v.held {
		b.cfg.Spots.release(v.idx)
	}
	v.idx, v.held, v.at, v.arrived = 0, false, Point{}, false
}
