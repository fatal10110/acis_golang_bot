package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// Stand point ranges of the fight's approach (docs/navigation.md, "4.
// Consumers"). An attack or a cast on a target out of range walks the bot
// straight at it, never routed (creature.go, the tracking walk), so the bot
// sends one only while that straight line is open, or the target is in range
// and in sight, and else walks to a stand point first.
const (
	// meleeStand is how far from its target a melee stand point may lie:
	// from there the server's straight approach covers the rest.
	meleeStand = 100
	// bowRange is a bow's attack range: the server's by weapon type
	// (player/character_attack.go weaponRange), not an item stat.
	bowRange = 500
	// rangedReach is the range of a cast whose skill level the travel map
	// lacks: below the common attack spell's 600.
	rangedReach = 400
	// standMargin is how far inside its reach a ranged stand point lies:
	// the walk stops within walkSlop of it, and the target may step
	// walkSameDest away before another one is chosen.
	standMargin = walkSlop + walkSameDest
	// defaultTargetHeight is a target's collision height for the sight
	// check: the server does not send a monster's.
	defaultTargetHeight = 25
)

// reach is the range the server lets the bot attack or cast on its target
// from without walking it there first. The server adds both collision radii
// (and, to an attack, a grace of 10 or 60) to it; the bot decodes no
// collision radius, so it stays inside the bare range.
type reach struct {
	// rng is the range in world units; 0 is a melee attack's, or a cast
	// that must touch its target.
	rng float64
	// flat: the server measures it horizontally (an attack,
	// attack/range.go InPhysicalRange), else in 3D (a cast,
	// network/magic_skill.go walkToCastTarget).
	flat bool
}

// holds reports whether to lies strictly inside r from from, as the
// server's In2DRadius and In3DRadius check.
func (r reach) holds(from, to Point) bool {
	d := horizontal2(from, to)
	if !r.flat {
		dz := int64(to.Z) - int64(from.Z)
		d += dz * dz
	}
	return float64(d) < r.rng*r.rng
}

// stand is the stand point search r takes: one in sight within r less
// standMargin, or, for a range too short for that, a melee one with the
// straight line open, from where the server's approach covers the rest.
func (r reach) stand() (nav.StandMode, float64) {
	if rng := r.rng - standMargin; rng > meleeStand {
		return nav.StandSight, rng
	}
	return nav.StandLine, meleeStand
}

// engageState is the stand point the fight walks to before it attacks or
// casts on target id, standFor where id stood when it was chosen, mode and
// rng the search that chose it. In a duel with no stand point passing
// moveOK, stand is zero and held is when that search ran. It is owned by
// the brain goroutine.
type engageState struct {
	id              int32
	mode            nav.StandMode
	rng             float64
	stand, standFor Point
	held            time.Time
}

// duelStandRetry is how long a duel's hold for want of a stand point lasts
// before the search runs again (the monsters around move; the opponent
// moving walkSameDest searches at once).
const duelStandRetry = 2 * time.Second

// engageWalk is the walker's packet while the bot must walk before it may
// attack or cast on id from r: the straight line to id is closed, and id is
// out of r or out of sight. The moving target is checked again at each
// decision: a stand point is chosen again once id moved walkSameDest from
// where it stood. A target with no stand point the bot can reach is
// blacklisted for BlacklistFor, as one out of attempts. In a duel the
// stand point and the straight segment to it pass moveOK (duel ground and
// monster clearance, docs/pvp/finding.md): a point failing it is skipped
// like a blocked one, one already chosen that fails it is chosen again, and
// with none the bot holds where it stands (no walk, and no attack the
// server would walk for) until id moves or duelStandRetry passed; a hold
// that starts while the bot still walks a segment toward a point it dropped
// sends one move to where it stands first, since the server walks a sent
// segment to its end. It
// reports false when the bot may attack or cast now, and always without
// geodata (the bot acts as it would) or while Ultimate Defense holds the bot
// (immobile: no walk starts).
func (b *Brain) engageWalk(id int32, r reach, now time.Time) (Action, bool) {
	w := b.walker()
	o := b.world.object(id)
	if w.nav == nil || o == nil || b.immobile(now) {
		return Action{}, false
	}
	from, to := b.world.Self.Position(now), o.Position(now)
	if w.lineOpen(from, to) {
		return Action{}, false
	}
	if r.rng > 0 && r.holds(from, to) && w.sightOpen(from, to, defaultTargetHeight) {
		return Action{}, false
	}
	e := &b.engage
	mode, rng := r.stand()
	duel := b.inDuel(now)
	if e.id != id || e.mode != mode || e.rng != rng || horizontal2(to, e.standFor) > walkSameDest*walkSameDest ||
		e.stand == (Point{}) && (!duel || e.held.IsZero() || now.Sub(e.held) >= duelStandRetry) ||
		duel && e.stand != (Point{}) && !b.moveOK(from, e.stand, now) {
		var accept func(Point) bool
		if duel {
			accept = func(p Point) bool { return b.moveOK(from, p, now) }
		}
		st, ok := w.standPointWhere(from, to, rng, mode, defaultTargetHeight, accept)
		switch {
		case !ok && duel:
			*e = engageState{id: id, mode: mode, rng: rng, standFor: to, held: now}
			if b.world.Self.Moving(now) {
				return b.orStay(Action{}, now), true
			}
		case !ok:
			b.unreachableTarget(id, now)
			return Action{}, true
		default:
			*e = engageState{id: id, mode: mode, rng: rng, stand: st, standFor: to}
		}
	}
	if e.stand == (Point{}) {
		return Action{}, true
	}
	purpose := walkMelee
	if mode == nav.StandSight {
		purpose = walkRanged
	}
	a, st := b.goTo(e.stand, walkSlop, purpose, now)
	switch st {
	case walkArrived:
		return Action{}, false
	case walkUnreachable:
		b.unreachableTarget(id, now)
		return Action{}, true
	}
	return a, true
}

// castReach is the reach of a cast of skill at the level the bot knows it:
// its cast range from the travel map, or rangedReach when the map lacks it.
// It reports false for a negative cast range: the server casts that skill
// with no approach.
func (b *Brain) castReach(skill int32) (reach, bool) {
	r := reach{rng: rangedReach}
	if b.cfg.Travel == nil {
		return r, true
	}
	cr, ok := b.cfg.Travel.castRanges[datapack.SkillRef{ID: skill, Level: b.world.Skills[skill].Level}]
	switch {
	case !ok:
		return r, true
	case cr < 0:
		return reach{}, false
	}
	return reach{rng: float64(cr)}, true
}

// attackReach is the reach of the bot's attack: a bow's range while it
// wields one, else a melee attack's.
func (b *Brain) attackReach() reach {
	if b.bowEquipped() {
		return reach{rng: bowRange, flat: true}
	}
	return reach{}
}

// unreachableTarget blacklists id for BlacklistFor: no walk reaches a
// stand point to fight it from.
func (b *Brain) unreachableTarget(id int32, now time.Time) {
	b.blacklist[id] = now.Add(b.persona.BlacklistFor)
	b.engage = engageState{}
}

// bowEquipped reports whether the bot wields a bow: a weapon it shoots
// arrows from.
func (b *Brain) bowEquipped() bool {
	if b.cfg.Travel == nil {
		return false
	}
	for _, it := range b.world.Inventory.Items {
		if _, bow := b.cfg.Travel.arrows[it.TemplateID]; bow && it.Equipped {
			return true
		}
	}
	return false
}

// indexCastRanges maps every skill level of skills to its cast range.
func indexCastRanges(skills map[datapack.SkillRef]datapack.Skill) map[datapack.SkillRef]int32 {
	out := make(map[datapack.SkillRef]int32, len(skills))
	for ref, sk := range skills {
		out[ref] = sk.CastRange
	}
	return out
}
