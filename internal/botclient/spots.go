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
	// Beside: a bot holding no spot that finds every spot that fits it
	// within Wander full (at its limit, or at its cap without Overflow) and
	// takes no trip farms beside one of them with no claim (owner decision
	// on #142): it walks to the spot and fights its monsters as a holder
	// does, under the same level fit, leash and pacing, and counts toward
	// neither the spot's cap nor its limit. The bots beside spread over the
	// full spots, fewest per monster first (SpotRegistry.farmBeside). Each
	// spotRetry it looks again for a spot to claim.
	Beside bool
	// Crowd is the number of other players within Leash of the spot's
	// centre, party members and the bots sharing the spot's claim aside,
	// that makes a spot crowded on arrival: the bot moves on and avoids the
	// spot for AvoidFor (one whose walk ended unreachable, walkRetry). 0
	// never finds one crowded.
	Crowd    int
	AvoidFor time.Duration
	// Idle is how long a bot at its spot may go out of combat with no farm
	// target before it moves on and avoids the spot for AvoidFor: other
	// players take every monster there, or none comes within reach (#360).
	// 0 never moves on.
	Idle time.Duration
	// DeathAvoid is how long a bot that died holding a spot, at it or on
	// its way there, avoids that spot: one whose packs killed it once
	// likely kill it again, and it would walk straight back after the
	// respawn (#258). 0 never avoids one.
	DeathAvoid time.Duration
	// Leash is how far from the spot's centre the bot may end a fight
	// before it walks back, and how far from it a monster is a farm target;
	// inside the spot's territory both hold however far it reaches (#447).
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
	// Margin replaces the build's spot margin (ClassKit.SpotMargin) when
	// not 0; below 0 it turns the model bound off. A spot fits only while
	// the bot lasts against its likely pack at least Margin times as long as
	// it takes to kill one of its monsters (packFits, docs/strategy.md
	// Design section 11): a persona default (D6), not a safety boundary.
	Margin float64
}

// defaultSpotPolicy is the farmer's spot choice: monsters up to two levels
// above the bot, aggressive ones not above it (plan, farmer) once it is
// past level 5, leaving at five levels over; spread over every spot that
// fits within twice the newbie zone's reach, sharing full ones rather than
// waiting (#137) and farming beside them past their limit (#142); a death
// at a pack passes over its like (#281); five minutes at a spot with
// nothing to fight moves it on (#360). The model bound takes the build's
// margin (Margin 0).
func defaultSpotPolicy() SpotPolicy {
	return SpotPolicy{
		Below: 2, AggroAbove: 0, Above: 3, Newbie: 5, Outlevel: 5,
		MaxTravel: 20000, Wander: 40000, Overflow: true, Beside: true,
		Crowd: 4, AvoidFor: 10 * time.Minute, Idle: 5 * time.Minute, DeathAvoid: 30 * time.Minute, Leash: 1500,
		PackShare: 0.8,
	}
}

// fits reports whether s fits a bot of level.
func (p *SpotPolicy) fits(s *datapack.Spot, level int) bool { return p.fitsBand(s, level, level) }

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
// (packFits).
func (b *Brain) spotFits(s *datapack.Spot, now time.Time) bool {
	level := int(b.world.Self.Level)
	return b.spotFitsBand(s, level, level, now)
}

// spotFitsBand is spotFits for the levels lo to hi of a leader's bot
// members (fitBand): the pack bounds stay the bot's own.
func (b *Brain) spotFitsBand(s *datapack.Spot, lo, hi int, now time.Time) bool {
	self := &b.world.Self
	return b.persona.Spots.fitsBand(s, lo, hi) && !b.spot.pack.covers(s, int(self.Level), self.RunSpeed, now) && b.packFits(s, now)
}

// lowerFits reports whether s fits the bot at now as a fallback (claimLower):
// its monsters all under its level window (SpotPolicy.Above), no pack a
// death taught it to pass over, and no pack too much for its stats
// (packFits). packFits stays the gate: the fallback only widens the level
// window downward.
func (b *Brain) lowerFits(s *datapack.Spot, now time.Time) bool {
	self := &b.world.Self
	level := int(self.Level)
	return s.MaxLevel < level-b.persona.Spots.Above && !b.spot.pack.covers(s, level, self.RunSpeed, now) && b.packFits(s, now)
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
	// spotFailMax is how many walks to spots in a row may end unreachable
	// before the bot stops looking for spotRetry (#497): each costs a route
	// search of up to walkBudget cells, and a bot the cell planner cannot
	// route from would otherwise plan to every candidate at once.
	spotFailMax = 3
)

// spotVisit is the bot's farming spot (rule "spot"). It is owned by the
// brain goroutine; the claim it holds is counted in the SpotRegistry.
type spotVisit struct {
	// idx is the spot held, when held, and at its centre on walkable
	// ground inside its territory, in the bot's SCC when it was claimed
	// (spotPoint): where the bot walks and stays.
	idx  int
	held bool
	// weight is how many places of the spot the bot took (spotWeight): 1
	// solo, its party for a leader; the zero value is 1 (claimWeight).
	weight int
	// beside is set while the bot farms spot idx with no claim
	// (SpotPolicy.Beside); it is counted in the registry's beside counts,
	// not its held ones. held and beside are never both set.
	beside bool
	at     Point
	// lower is set while the spot held is a fallback under the bot's level
	// window (claimLower): the bot looks again every spotRetry for one of
	// its level.
	lower bool
	// earning is set while the spot held is an earning spot (claimEarn,
	// #527): the bot looks again every spotRetry for one that fits.
	earning bool
	// arrived is set once the bot reached the spot's centre.
	arrived bool
	// idleSince is when the bot's idle stretch at the spot began, zero
	// outside one, and idleSeen the last decision in it (idleOut).
	idleSince, idleSeen time.Time
	// retryAt is when the bot may look for a spot again after finding none.
	retryAt time.Time
	// avoid are the spots the bot moved on from, until a time.
	avoid map[int]time.Time
	// failed are the spots of the bot's current run of walks that ended
	// unreachable (failedWalk), oldest first, each once: avoided while the run lasts,
	// until a walk arrives or no candidate is left.
	failed []int
	// pack is the pack its deaths taught it to pass over.
	pack packBound
	// gatherSince is when a bot member of the party the bot leads fell
	// behind it at its spot, zero while none is (gathering).
	gatherSince time.Time
}

// claimWeight is how many places the spot farmed takes in the registry.
func (v *spotVisit) claimWeight() int { return max(v.weight, 1) }

// farming reports whether the bot has a spot to farm: one it holds or one
// it farms beside.
func (v *spotVisit) farming() bool { return v.held || v.beside }

// ruleSpot is a rule 9 errand ahead of the persona's goal: with a spot
// registry, the bot claims a fitting spot it can walk to (pickSpot), walks
// there through the walker, moves on when it finds the spot crowded, walks
// back when a fight left it beyond the leash and outside the spot's
// territory (not while it walks around to a farm target, farmWalking), and
// picks a new spot once it outlevels its own or, sharing one past its cap,
// every spotRetry. At its spot it leaves the turn to rule "farm", which
// moves it on after Idle with nothing to fight. A bot holding none claims
// none while the town preparation gate holds it (preparing).
//
// Every move is the walker's (goTo): one MoveBackwardToLocation per checked
// segment through the frame pacer (gateNone). A walk that ends unreachable
// gives the spot up for walkRetry, spotFailMax of them in a row wait
// spotRetry (failedWalk), and a fruitless search waits spotRetry, so a bot
// that cannot get anywhere stops sending and stops planning.
func (b *Brain) ruleSpot(now time.Time) (Action, bool) {
	r, s, v := b.cfg.Spots, &b.world.Self, &b.spot
	if r == nil || s.InCombat {
		return Action{}, false
	}
	if b.inBotParty() && !b.leading() {
		// A member follows its leader: it never claims nor farms beside.
		return Action{}, false
	}
	if !v.farming() && b.preparing() {
		// The town preparation gate: no spot before the errands ahead
		// stocked the bot up.
		return Action{}, false
	}
	if !now.Before(v.retryAt) && (!v.held || v.lower || v.earning || b.outleveled() || b.misfit() || b.overRisk(now) || r.overCap(v.idx)) {
		b.pickSpot(now)
	}
	if !v.farming() {
		return Action{}, false
	}
	if v.arrived {
		leash, pos := int64(b.persona.Spots.Leash), s.Position(now)
		if horizontal2(pos, v.at) <= leash*leash || inTerritory(&r.spots[v.idx], pos) || s.Moving(now) || b.farmWalking(now) {
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
		// walkRetry anyway, so look for another, or after spotFailMax in a
		// row wait spotRetry first.
		b.failedWalk(now)
		return Action{}, false
	}
	if v.arrived {
		return Action{}, false
	}
	v.arrived, v.idleSince, v.failed = true, time.Time{}, v.failed[:0]
	b.anchor = s.Position(now)
	if b.crowded(v.at, now) {
		b.moveOn(now, b.persona.Spots.AvoidFor)
		return Action{}, true
	}
	return Action{}, false
}

// farmWalking reports whether the bot is under way to the stand point it
// fights its farm target from: the target lies within the leash or inside
// the spot's territory (#447), the walk to it need not (around a wall, a rock, a slope). Walked back to the spot
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
// false without navigation or off the nav cache. The walker's pins answer
// for the ground it stands on and walks to.
func (b *Brain) locate(p Point) (navPoint, bool) {
	w := &b.walk
	if w.nav == nil {
		return navPoint{}, false
	}
	c, ok := w.plan.Locate(toLoc(p))
	if !ok {
		return navPoint{}, false
	}
	m, err := w.nav.Hier.Locate(c, &w.pins)
	if err != nil {
		return navPoint{}, false
	}
	p.Z = int32(c.Z)
	return navPoint{at: p, m: m}, true
}

// locatePlace is locate for a static place, the same for every bot (a
// gatekeeper, a teleport's landing, a town, a spot's centre): the
// hierarchy's place table answers it after the first time
// (Hierarchy.LocatePlace). Never where a bot stands: the table keeps every
// cell it is asked.
func (b *Brain) locatePlace(p Point) (navPoint, bool) {
	w := &b.walk
	if w.nav == nil {
		return navPoint{}, false
	}
	c, ok := w.plan.Locate(toLoc(p))
	if !ok {
		return navPoint{}, false
	}
	m, err := w.nav.Hier.LocatePlace(c)
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

// snapSpot is spotPoint's search.
func (b *Brain) snapSpot(sp *datapack.Spot, from navPoint) (navPoint, bool) {
	c := Point{X: sp.Center.X, Y: sp.Center.Y, Z: sp.Z}
	if p, ok := b.locatePlace(c); ok && b.walkable(from, p) {
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
		if at, ok := b.spotPoint(i, here); ok {
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
// holding the spot or farming beside it: the registry's cap already counts
// the holders, so a spot under its cap is not crowded by its own holders,
// and the bots beside it went there knowing it full (SpotPolicy.Beside). A
// leader's own weight (its party) is no other bot's; a party farming there
// is its players and its weight alike.
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
	r := b.cfg.Spots
	return n-(r.Held(b.spot.idx)+r.Beside(b.spot.idx)-b.spot.claimWeight()) >= p.Crowd
}

// outleveled reports whether the bot's level is past its spot's highest
// monster by more than Persona.Spots.Outlevel.
func (b *Brain) outleveled() bool {
	v := &b.spot
	return v.held && int(b.world.Self.Level) > b.cfg.Spots.spots[v.idx].MaxLevel+b.persona.Spots.Outlevel
}

// overRisk reports whether the pack of the spot the bot holds no longer
// fits its stats (packFits), as when it took the spot before its stats came
// in or its potions ran out: it looks for one that fits, of its level, else
// a trip, else the highest under its level window (claimLower), and keeps
// the one it holds with none.
func (b *Brain) overRisk(now time.Time) bool {
	v := &b.spot
	return v.held && !b.packFits(&b.cfg.Spots.spots[v.idx], now)
}

// pickSpot claims a spot that fits the bot's level, is not avoided and that
// the bot can walk to, in place of the one it holds (#137): within MaxTravel
// in straight line, then within Wander, the nearest by the abstract graph
// (navDistance) that no bot holds, else the nearest with room. With none,
// it looks again after spotRetry and plans a trip by gatekeeper (rule
// "travel", planTrip), giving up the spot it holds for the trip; with no
// trip either and no spot of its level that passes the model, it falls back
// to the highest spot under its level window that does (claimLower, #446),
// and a bot with a death penalty and no such fallback that holds no spot
// (or an earning one) farms for adena (claimEarn, #527);
// else a bot holding no spot that fits shares, past its cap, the spot in
// Wander its bots fill least (SpotPolicy.Overflow), one with every such spot
// at its limit farms beside one (SpotPolicy.Beside, farmBeside), and one
// that holds a spot that fits keeps it. A spot in another SCC is never
// claimed. A leader of its bot party claims for its whole party
// (spotWeight), fits its bot members' band (fitBand, misfit), takes no trip
// and no fallback, and gives back a spot it no longer fits with nothing in
// reach (docs/parties/spots.md).
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

// spotCands are the spots that fit the bot (a leader: its bot members'
// band, fitBand) standing at here, are not
// avoided and lie in its SCC within Wander in straight line, but the one it
// holds.
func (b *Brain) spotCands(here navPoint, now time.Time) []spotCand {
	lo, hi, _ := b.fitBand()
	return b.candsWhere(here, now, func(s *datapack.Spot, now time.Time) bool { return b.spotFitsBand(s, lo, hi, now) })
}

// lowerCands are the spots under the bot's level window that pass the
// model (lowerFits) the bot may claim standing at here, as spotCands.
func (b *Brain) lowerCands(here navPoint, now time.Time) []spotCand {
	return b.candsWhere(here, now, b.lowerFits)
}

// candsWhere are the spots fit passes, as spotCands.
func (b *Brain) candsWhere(here navPoint, now time.Time, fit func(*datapack.Spot, time.Time) bool) []spotCand {
	r, v, p := b.cfg.Spots, &b.spot, &b.persona.Spots
	far := max(float64(p.MaxTravel), float64(p.Wander))
	var cands []spotCand
	for i := range r.spots {
		sp := &r.spots[i]
		if (v.held && i == v.idx) || now.Before(v.avoid[i]) || !fit(sp, now) {
			continue
		}
		line := math.Sqrt(float64(horizontal2(here.at, Point{X: sp.Center.X, Y: sp.Center.Y})))
		if line > far {
			continue
		}
		at, ok := b.spotPoint(i, here)
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
	n, w, party := len(order), b.spotWeight(), b.leadsBotMembers()
	order = append(order, beyond...)
	i, ok := r.claimN(order[:n], w)
	if !ok {
		i, ok = r.claimN(order[n:], w)
	}
	if !ok {
		// No candidate left: the run of unreachable walks ends, its spots
		// avoided only as long as moveOn said.
		v.retryAt, v.failed = now.Add(spotRetry), v.failed[:0]
		if here != nil && !party {
			// A leader takes no gatekeeper trip: its members follow by
			// walking only.
			b.planTrip(*here, now)
		}
		if b.trip.v.active() {
			// A bot on a trip farms near its destination: it gives back the
			// spot it holds (outlevelled, or shared past its cap) rather
			// than block a place there for the trip and walk back to it.
			b.leaveSpot()
			return
		}
		unfit := b.outleveled() || b.misfit()
		if len(cands) == 0 && here != nil {
			if party {
				// The gated trip's give-back: a leader that would have
				// planned one gives back a spot it no longer fits rather
				// than keep it for good; the spot fails the fit, so it is
				// not claimed again.
				if unfit {
					b.leaveSpot()
					return
				}
			} else if b.claimLower(*here, now) {
				return
			} else if (!v.held || v.earning) && b.claimEarn(*here, now) {
				return
			}
		}
		if v.held && !unfit {
			return
		}
		if p.Overflow {
			i, ok = r.claimOverN(order, w)
		}
		if !ok {
			if !v.held {
				b.farmBeside(cands, order, w)
			}
			return
		}
	}
	// A spot claimed after a landing ends the path the trip was on.
	b.trip.onward = 0
	b.leaveSpot()
	v.idx, v.held, v.at, v.weight = i, true, candAt(cands, i), w
}

// claimLower is pickSpot's fallback for a bot with no spot of its level to
// claim and no trip (#446): every spot of its level fails the model
// (packFits) or the bot outlevelled them. It claims the spot under its level
// window that passes the model (lowerCands) with the highest monsters, the
// nearest among those: a monster more than 5 levels under its killer cuts
// its drop and spoil chances by 18 % a level (server item/drop.go
// LevelPenaltyMultiplier), so the highest that fits gives up least: it
// claims level by level (SpotRegistry.claimTiers), so an empty spot is
// taken before a shared one only among spots of one level. With every such
// spot full it shares, past its cap, the one its bots fill least
// (SpotPolicy.Overflow). A bot holding a spot of its level that fits keeps
// it, and one holding a spot under its level window that fits (a fallback,
// or the spot it outlevelled) keeps it until a higher one that fits has
// room under its cap. It reports whether the bot holds a fallback.
func (b *Brain) claimLower(here navPoint, now time.Time) bool {
	r, v, p := b.cfg.Spots, &b.spot, &b.persona.Spots
	if v.held && b.spotFits(&r.spots[v.idx], now) {
		return false
	}
	cands := b.lowerCands(here, now)
	if len(cands) == 0 {
		return false
	}
	beyond := func(c spotCand) bool { return c.line > float64(p.MaxTravel) }
	slices.SortStableFunc(cands, func(x, y spotCand) int {
		if c := cmp.Compare(r.spots[y.i].MaxLevel, r.spots[x.i].MaxLevel); c != 0 {
			return c
		}
		if bx, by := beyond(x), beyond(y); bx != by {
			if bx {
				return 1
			}
			return -1
		}
		return cmp.Compare(x.d, y.d)
	})
	// A held fallback that fits is left only for a higher one with room:
	// never for one as high or lower, nor past a cap, so the bot does not
	// hop between fallbacks.
	held := &r.spots[v.idx]
	keep := v.held && b.lowerFits(held, now)
	order := make([]int, 0, len(cands))
	for _, c := range cands {
		if !keep || r.spots[c.i].MaxLevel > held.MaxLevel {
			order = append(order, c.i)
		}
	}
	i, ok := r.claimTiers(order, 1)
	if !ok && keep {
		v.lower = true
		return true
	}
	if !ok && p.Overflow {
		i, ok = r.claimOverN(order, 1)
	}
	if !ok {
		return false
	}
	b.trip.onward = 0
	b.leaveSpot()
	v.idx, v.held, v.lower, v.at = i, true, true, candAt(cands, i)
	return true
}

// farmBeside makes a bot holding no spot, with every spot of order (the
// candidates, best first) full, farm beside one of them with no claim
// (SpotPolicy.Beside), counting w (spotWeight): the one it farms beside
// while that one is still a candidate, else the one the bots beside fill
// least per monster. With none, or Beside off, it farms beside none.
func (b *Brain) farmBeside(cands []spotCand, order []int, w int) {
	r, v := b.cfg.Spots, &b.spot
	if v.beside && slices.Contains(order, v.idx) {
		return
	}
	b.leaveSpot()
	if !b.persona.Spots.Beside {
		return
	}
	if i, ok := r.farmBesideN(order, w); ok {
		v.idx, v.beside, v.at, v.weight = i, true, candAt(cands, i), w
	}
}

// candAt is where the bot walks for candidate spot i.
func candAt(cands []spotCand, i int) Point {
	for _, c := range cands {
		if c.i == i {
			return c.at
		}
	}
	return Point{}
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

// failedWalk gives the spot up after its walk ended unreachable, avoiding
// it for walkRetry. Every spotFailMax walks in a row that so end, the bot
// looks for no spot for spotRetry, the whole run's spots avoided past that,
// so each window tries the next spotFailMax candidates: at most spotFailMax
// route plans to spots per spotRetry (#497). A spot that fails again within
// the run (its avoid ran out during a long walk) is not added twice, so the
// run holds each spot once; the candidates are cycling, so it waits too.
func (b *Brain) failedWalk(now time.Time) {
	v := &b.spot
	b.karmaWalkFailed()
	repeat := slices.Contains(v.failed, v.idx)
	if !repeat {
		v.failed = append(v.failed, v.idx)
	}
	b.moveOn(now, walkRetry)
	if !repeat && len(v.failed)%spotFailMax != 0 {
		return
	}
	v.retryAt = now.Add(spotRetry)
	until := v.retryAt.Add(walkRetry)
	for _, i := range v.failed {
		if v.avoid[i].Before(until) {
			v.avoid[i] = until
		}
	}
}

// diedAtSpot gives back the spot the bot holds or farms beside, if any, on
// its death, avoiding it, and the packs like it (learnPack), for
// SpotPolicy.DeathAvoid.
func (b *Brain) diedAtSpot(now time.Time) {
	if d := b.persona.Spots.DeathAvoid; b.spot.farming() && d > 0 {
		b.learnPack(&b.cfg.Spots.spots[b.spot.idx], now)
		b.moveOn(now, d)
		return
	}
	b.leaveSpot()
}

// leaveSpot gives back the spot the bot holds or farms beside, if any, its
// whole weight (a leader's party): on a move, on death and when the brain
// stops.
func (b *Brain) leaveSpot() {
	v := &b.spot
	switch {
	case v.held:
		b.cfg.Spots.releaseN(v.idx, v.claimWeight())
	case v.beside:
		b.cfg.Spots.leaveBesideN(v.idx, v.claimWeight())
	}
	v.idx, v.held, v.beside, v.lower, v.earning, v.at, v.arrived, v.weight = 0, false, false, false, false, Point{}, false, 0
	v.idleSince, v.idleSeen, v.gatherSince = time.Time{}, time.Time{}, time.Time{}
}
