package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// Stand point ranges of the fight's approach (docs/navigation.md, "4.
// Consumers"). An attack or a cast on a target out of range walks the bot
// straight at it, never routed (creature.go, the tracking walk), so the bot
// sends one only while that straight line is open, and else walks to a
// stand point first.
const (
	// meleeStand is how far from its target a melee stand point may lie:
	// from there the server's straight approach covers the rest.
	meleeStand = 100
	// rangedReach is how far from its target the bot shoots or casts from
	// a stand point with line of sight: below the bow's 500 and the
	// common attack spell's 600, as the bot decodes neither the weapon's
	// nor the skill's own range.
	rangedReach = 400
	// defaultTargetHeight is a target's collision height for the sight
	// check: the server does not send a monster's.
	defaultTargetHeight = 25
)

// engageState is the stand point the fight walks to before it attacks or
// casts on target id, standFor where id stood when it was chosen. It is
// owned by the brain goroutine.
type engageState struct {
	id              int32
	ranged          bool
	stand, standFor Point
}

// engageWalk is the walker's packet while the bot must walk before it may
// attack or cast on id (ranged: a cast, or a bow shot): the straight line to
// id is closed (and, ranged, id is out of rangedReach or out of sight). The
// moving target is checked again at each decision: a stand point is chosen
// again once id moved walkSameDest from where it stood. A target with no
// stand point the bot can reach is blacklisted for BlacklistFor, as one
// out of attempts. It reports false when the bot may attack or cast now,
// and always without geodata (the bot acts as it would).
func (b *Brain) engageWalk(id int32, ranged bool, now time.Time) (Action, bool) {
	w := b.walker()
	o := b.world.object(id)
	if w.nav == nil || o == nil {
		return Action{}, false
	}
	from, to := b.world.Self.Position(now), o.Position(now)
	if w.lineOpen(from, to) {
		return Action{}, false
	}
	if ranged && horizontal2(from, to) <= rangedReach*rangedReach && w.sightOpen(from, to, defaultTargetHeight) {
		return Action{}, false
	}
	e := &b.engage
	if e.id != id || e.ranged != ranged || e.stand == (Point{}) || horizontal2(to, e.standFor) > walkSameDest*walkSameDest {
		mode, rng := nav.StandLine, float64(meleeStand)
		if ranged {
			mode, rng = nav.StandSight, rangedReach
		}
		st, ok := w.standPoint(from, to, rng, mode, defaultTargetHeight)
		if !ok {
			b.unreachableTarget(id, now)
			return Action{}, true
		}
		*e = engageState{id: id, ranged: ranged, stand: st, standFor: to}
	}
	purpose := walkMelee
	if ranged {
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
