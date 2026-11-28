package botclient

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Arena trips (docs/pvp/finding.md "Sources", PvPPolicy.Arenas): every
// arenaEvery, give or take arenaJitter, the bot walks (or takes a
// gatekeeper) to the nearest arena (a long-cast row to the deepest one,
// pickArena), fights its arena targets while another player is inside, and
// leaves after arenaAlone alone or arenaStay in all. arenaArrive is how
// close to the arena's middle a walk there ends; arenaLanding how far from
// it a gatekeeper's landing may lie: the Colosseum's (Aden's gatekeeper,
// 146440 46723) lies about 3060 from its middle, past storeTownRadius.
const (
	arenaEvery   = 90 * time.Minute
	arenaJitter  = 15 * time.Minute
	arenaAlone   = 5 * time.Minute
	arenaStay    = 20 * time.Minute
	arenaArrive  = 100
	arenaLanding = 3500
)

// arenaTrip is the bot's arena trip: when the next one is due, when this
// one started (zero for none), its arena (1-based, the travel map's
// arenas), the arena's middle and where the bot set out from, whether the
// travel errand carries it by gatekeeper now (gate) or did (gated), when it
// got inside by arenaEdge, since when it is alone there, and its leave: on
// from the decision to go, stopAt the stop move's time.
type arenaTrip struct {
	due, since time.Time
	arena      int
	to, from   Point
	gate       bool
	gated      bool
	inside     time.Time
	alone      time.Time
	leaving    bool
	stopAt     time.Time
	// died is set by an arena death that keeps the trip (arenaDied): the
	// restart's teleport does not end it, the bot walks back.
	died bool
}

// active reports whether an arena trip is under way.
func (t *arenaTrip) active() bool { return !t.since.IsZero() }

// arenaEnd ends the arena trip at now and arms the next one.
func (b *Brain) arenaEnd(now time.Time) {
	b.pvp.arena = arenaTrip{}
	b.arenaArm(now)
}

// arenaArm arms the next arena trip arenaEvery from now, give or take
// arenaJitter.
func (b *Brain) arenaArm(now time.Time) {
	jitter := time.Duration(b.rng.Int64N(int64(2*arenaJitter)+1)) - arenaJitter
	b.pvp.arena.due = now.Add(arenaEvery + jitter)
}

// arenaMiddle is the middle of arena z at height h: its box's centre, or
// its vertex ring's mean.
func arenaMiddle(z datapack.Zone, h int32) Point {
	var x, y float64
	for i := range z.X {
		x += float64(z.X[i])
		y += float64(z.Y[i])
	}
	n := float64(max(len(z.X), 1))
	return Point{X: int32(math.Round(x / n)), Y: int32(math.Round(y / n)), Z: h}
}

// nearestArena is the travel map's arena whose middle lies nearest p
// (1-based, 0 for none) and that middle.
func (b *Brain) nearestArena(p Point) (int, Point) {
	best, at := 0, Point{}
	var bestD int64
	for i, z := range b.cfg.Travel.zones.Arenas {
		mid := arenaMiddle(z, p.Z)
		if d := horizontal2(p, mid); best == 0 || d < bestD {
			best, at, bestD = i+1, mid, d
		}
	}
	return best, at
}

// deepArena is the travel map's arena whose middle lies farthest from its
// edge (1-based, 0 for none) and that middle at height h: the Colosseum,
// about 1370 from an edge against about 490 for Dion, Gludin and Giran
// (docs/pvp/guards.md "Arena edge").
func (b *Brain) deepArena(h int32) (int, Point) {
	best, at := 0, Point{}
	var bestEdge float64
	for i, z := range b.cfg.Travel.zones.Arenas {
		mid := arenaMiddle(z, h)
		if e := zoneEdge(z, mid); best == 0 || e > bestEdge {
			best, at, bestEdge = i+1, mid, e
		}
	}
	return best, at
}

// longCastRow reports whether the bot plays a row whose damage steps the
// arena-edge guard holds in a square arena (docs/pvp/guards.md "Arena
// edge"): the OM row, a mystic row with no spiritshot expected charged
// (its hit times 1/0.70 longer), or a row that shoots (a scout's bow
// branch, an archer row: Power Shot, Stunning Shot).
func (b *Brain) longCastRow() bool {
	row, ok := charmodel.ClassRow(b.world.Self.ClassID, b.bowEquipped())
	switch {
	case !ok || row.Archetype() == charmodel.Melee:
		return false
	case row.ID == "OM" || row.Shoots():
		return true
	}
	return !b.spiritshotCharged()
}

// pickArena sets the arena trip starting at now from from on its arena: a
// long-cast row's the deepest arena (deepArena) when its walk reaches it
// (walkReach) or a gatekeeper trip there plans now (planArena), else the
// nearest arena (nearestArena).
func (b *Brain) pickArena(from Point, now time.Time) {
	t, tr := &b.pvp.arena, &b.trip
	i, to := b.nearestArena(from)
	if b.longCastRow() {
		if d, dto := b.deepArena(from.Z); d != i {
			t.arena, t.to = d, dto
			if b.walkReach(from, dto) {
				return
			}
			onward := tr.onward
			if b.planArena(now) {
				return
			}
			tr.onward = onward
		}
	}
	t.arena, t.to = i, to
}

// walkReach reports whether the bot walks from at to to: both on the
// navigation, within SpotPolicy.MaxTravel in a straight line, in one SCC.
func (b *Brain) walkReach(at, to Point) bool {
	from, ok := b.locate(at)
	if !ok {
		return false
	}
	dst, ok := b.locatePlace(to)
	reach := int64(b.persona.Spots.MaxTravel)
	if !ok || horizontal2(at, to) > reach*reach {
		return false
	}
	_, ok = b.navDistance(from, dst)
	return ok
}

// arenaReady reports whether an arena trip may start at now: the bot farms
// at its spot, leads no party nor follows one, is out of combat, has no
// karma (everyone's target there, docs/pvp/karma.md) and is on no other
// trip.
func (b *Brain) arenaReady(now time.Time) bool {
	v := &b.spot
	return v.farming() && v.arrived && !b.inBotParty() && !b.world.Self.InCombat && b.world.Self.Karma == 0 &&
		!b.supplyTrip.active() && !b.trip.v.active() && b.duel(now).Phase == DuelIdle
}

// ruleArena is the arena trip, a rule 9 errand after the supply trip: due
// and ready, the bot releases its spot (as a supply trip, D16) and goes to
// its arena (pickArena, arenaWalk); inside it by arenaEdge it holds every
// later rule while it stays (ruleArenaFight attacks), and leaves
// (arenaLeave) after arenaAlone with no other player inside or arenaStay
// in all. Walks are the walker's (goTo, the frame pacer); a gatekeeper trip
// is the travel errand's (ruleTravel: the bypass gate, Travel.Gap).
func (b *Brain) ruleArena(now time.Time) (Action, bool) {
	t := &b.pvp.arena
	if !b.persona.PvP.Arenas || b.cfg.Travel == nil || len(b.cfg.Travel.zones.Arenas) == 0 {
		return Action{}, false
	}
	if !t.active() {
		if t.due.IsZero() {
			if d := b.cfg.ArenaFirst; d > 0 {
				t.due = now.Add(d)
			} else {
				b.arenaArm(now)
			}
			return Action{}, false
		}
		if now.Before(t.due) || !b.arenaReady(now) {
			return Action{}, false
		}
		from := b.world.Self.Position(now)
		*t = arenaTrip{due: t.due, since: now, from: from}
		b.pickArena(from, now)
		b.leaveSpot()
	}
	if t.leaving {
		return b.arenaLeave(now)
	}
	if own := b.arenaAt(b.world.Self.Position(now)); own.Arena == t.arena && own.Edge >= arenaEdge {
		// Back on its feet inside (a resurrection): no restart to walk
		// back from.
		t.died = t.died && b.world.Self.Dead
		if t.inside.IsZero() {
			t.inside = now
		}
		if b.arenaCompany(now) {
			t.alone = time.Time{}
		} else if t.alone.IsZero() {
			t.alone = now
		}
		if b.arenaOver(now) {
			t.leaving = true
			return b.arenaLeave(now)
		}
		return Action{}, true
	}
	return b.arenaWalk(now)
}

// arenaOver reports whether the arena trip's stay is over at now: inside
// arenaStay in all, or alone there arenaAlone.
func (b *Brain) arenaOver(now time.Time) bool {
	t := &b.pvp.arena
	return t.active() && !t.inside.IsZero() && (now.Sub(t.inside) >= arenaStay || !t.alone.IsZero() && now.Sub(t.alone) >= arenaAlone)
}

// ruleArenaLeave is the arena trip's leave once its stay is over
// (arenaLeave), ahead of the attacked rule: the leave stops the fight in
// flight, so an attack must not start it again.
func (b *Brain) ruleArenaLeave(now time.Time) (Action, bool) {
	t := &b.pvp.arena
	if !t.leaving && !b.arenaOver(now) {
		return Action{}, false
	}
	t.leaving = true
	return b.arenaLeave(now)
}

// arenaCompany reports whether another live player stands in the trip's
// arena at now.
func (b *Brain) arenaCompany(now time.Time) bool {
	a := b.pvp.arena.arena
	for id, o := range b.world.Objects {
		if o.Kind == KindPlayer && !o.Dead && id != b.world.Self.ObjectID && b.arenaAt(o.Position(now)).Arena == a {
			return true
		}
	}
	return false
}

// arenaWalk takes the bot to the trip's arena: by gatekeeper when the arena
// is out of walking reach (onFoot) or the walker answers it unreachable,
// once, through the travel errand, else on foot to its middle. A trip that
// can get nowhere ends.
func (b *Brain) arenaWalk(now time.Time) (Action, bool) {
	t, tr := &b.pvp.arena, &b.trip
	if t.gate {
		if !tr.v.active() && (tr.onward == 0 || !b.planArena(now)) {
			t.gate = false
		} else {
			return b.ruleTravel(now)
		}
	}
	if !t.gated && !b.onFoot(b.world.Self.Position(now), t.to) && b.planArena(now) {
		return b.ruleTravel(now)
	}
	a, st := b.goTo(t.to, arenaArrive, walkPlace, now)
	switch st {
	case walkWalking:
		return a, true
	case walkUnreachable:
		if !t.gated && b.planArena(now) {
			return b.ruleTravel(now)
		}
	case walkArrived:
		if b.arenaAt(b.world.Self.Position(now)).Arena == t.arena {
			return Action{}, true
		}
	}
	b.arenaEnd(now)
	return Action{}, false
}

// planArena plans a gatekeeper trip toward the trip's arena from where the
// bot stands (planPath) and marks the trip a gatekeeper's.
func (b *Brain) planArena(now time.Time) bool {
	t := &b.pvp.arena
	here, ok := b.locate(b.world.Self.Position(now))
	if !ok || !b.planPath(here, now, []Point{t.to}, arenaLanding) {
		return false
	}
	t.gate, t.gated = true, true
	return true
}

// arenaTeleported ends the arena trip on the bot's own loading-screen
// teleport (a restart, a GM), unless it is the gatekeeper trip's landing
// (gk), from which the walk or the next teleport goes on, or the restart
// after an arena death that keeps the trip (arenaDied): from there the bot
// walks back (docs/pvp/karma.md "By fight").
func (b *Brain) arenaTeleported(now time.Time, gk bool) {
	t := &b.pvp.arena
	switch {
	case !t.active() || t.gate && gk:
	case t.died:
		t.died, t.gate, t.gated, t.alone, t.leaving, t.stopAt = false, false, false, time.Time{}, false, time.Time{}
	default:
		b.arenaEnd(now)
	}
}

// arenaLeave is the arena-edge leave (docs/pvp/guards.md "Arena edge"): an
// Esc for its damage cast while the server still honours it (else the cast
// lands first), a stop move that keeps arenaEdge inside (disengage,
// moveOK), one attack interval after it, then the walk back out toward
// where the trip set out once Ultimate Defense ends (immobile); outside
// the arena the trip ends and the spot rule picks again.
func (b *Brain) arenaLeave(now time.Time) (Action, bool) {
	t, s, g := &b.pvp.arena, &b.world.Self, &b.pvp.guard
	if t.stopAt.IsZero() {
		if b.casting(now) {
			if s.Casting(now) && now.Before(s.castEnd.Add(-escLead)) && !g.escFor.Equal(s.castEnd) {
				return Action{kind: actTargetCancel, payload: botwire.EncodeRequestTargetCancel()}, true
			}
			return Action{}, true
		}
		t.stopAt = now
		return b.disengage(now), true
	}
	if now.Sub(t.stopAt) < swingTime(s.PAtkSpd) {
		return Action{}, true
	}
	if b.arenaAt(s.Position(now)).Arena == 0 {
		b.arenaEnd(now)
		return Action{}, false
	}
	if b.immobile(now) {
		return Action{}, true
	}
	a, st := b.goTo(t.from, arenaArrive, walkPlace, now)
	if st == walkWalking {
		return a, true
	}
	b.arenaEnd(now)
	return Action{}, false
}

// ruleArenaFight attacks arena targets on an arena trip
// (docs/pvp/targets.md "Arena targets"): inside the trip's arena, with no
// attacker and no leave under way. Under its stand levels it rests, first
// stepping at least arenaEdge inside. Else it keeps its target while it
// stays one, or picks the nearest player of its tier rule 3 lets it fight,
// inside the same arena by arenaEdge. No engage estimate; an attacker is
// the attacked rule's.
func (b *Brain) ruleArenaFight(now time.Time) (Action, bool) {
	t, s := &b.pvp.arena, &b.world.Self
	if !t.active() || t.leaving || b.arenaOver(now) || t.inside.IsZero() || s.InCombat || b.inDuel(now) || len(b.attackers(now)) > 0 {
		return Action{}, false
	}
	own := b.arenaAt(s.Position(now))
	if own.Arena != t.arena {
		return Action{}, false
	}
	if !b.aboveStand() {
		if own.Edge < arenaEdge {
			a, st := b.goTo(t.to, arenaArrive, walkPlace, now)
			if st == walkWalking {
				return a, true
			}
		}
		if !s.Sitting {
			return b.sitDown(now), true
		}
		return Action{}, true
	}
	id := b.world.Target
	if !b.arenaTarget(id, own.Arena, now) {
		id = b.nearestPlayer(now, func(id int32, _ *Object) bool { return b.arenaTarget(id, own.Arena, now) })
	}
	if id == 0 {
		return Action{}, false
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	return b.fight(id, now), true
}

// arenaTarget reports whether player id is an arena target in arena at
// now: one the bot may fight by rule 3, of its class tier, seen inside the
// same arena by at least arenaEdge.
func (b *Brain) arenaTarget(id int32, arena int, now time.Time) bool {
	o := b.world.object(id)
	if o == nil || o.Kind != KindPlayer || !b.mayFight(id, now) || b.fightRule(id, o, now) != FightArena ||
		classTier(o.ClassID) != classTier(b.world.Self.ClassID) {
		return false
	}
	at := b.arenaAt(o.Position(now))
	return at.Arena == arena && at.Edge >= arenaEdge
}
