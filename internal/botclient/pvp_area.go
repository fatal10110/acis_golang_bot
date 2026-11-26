package botclient

import (
	"math"
	"strings"
	"time"
)

// Area hits and bystanders (docs/pvp/guards.md "Area hits and
// bystanders"): an area skill or a pole swing hits everyone in reach the
// bot may attack without force, so it waits while one whose death could be
// a PK, or whose hit could cancel the bot's duel, stands in reach.

// Pole reach (attack/controller.go): a pole swing hits inside the 120°
// cone the bot faces, out to its attack range (a spear's 80); a pole
// wielder repositions around its target in repositionStep steps.
const (
	poleRange      = 80
	poleHalfCone   = math.Pi / 3
	repositionStep = math.Pi / 6
	// meleeRange is a non-pole melee attack's range, bow aside.
	meleeRange = 40
)

// radiusOf is the decoded collision radius of id (the bot's own for its
// object id), 0 when none was seen.
func (b *Brain) radiusOf(id int32) float64 {
	if id == b.world.Self.ObjectID {
		return b.world.Self.CollisionRadius
	}
	return b.pvp.radii[id]
}

// areaReach is skill's area for the area rule: its radius, and whether it
// is anchored on the bot (AURA) rather than on its target (AREA, a cone);
// ok is false for a skill with no area, or one the rule leaves alone: in a
// duel any offensive area skill counts, elsewhere only a damage one.
func (b *Brain) areaReach(skill int32, now time.Time) (radius float64, aura, ok bool) {
	s, known := b.skillData(skill)
	if !known || !strings.Contains(s.Target, "AREA") && !strings.Contains(s.Target, "AURA") {
		return 0, false, false
	}
	if b.inDuel(now) && !s.Offensive || !b.inDuel(now) && !isDamageTypes[s.Type] {
		return 0, false, false
	}
	return float64(s.Radius), strings.Contains(s.Target, "AURA"), true
}

// areaHeld reports whether the bot's area step of skill at id would reach
// a bystander at now.
func (b *Brain) areaHeld(id, skill int32, now time.Time) bool {
	radius, aura, ok := b.areaReach(skill, now)
	if !ok {
		return false
	}
	anchor := id
	if aura {
		anchor = b.world.Self.ObjectID
	}
	at, ok := b.positionOf(anchor, now)
	if !ok {
		return false
	}
	return b.bystander(id, at, radius+b.radiusOf(anchor), nil, now)
}

// positionOf is where id (the bot's own object id included) is at now.
func (b *Brain) positionOf(id int32, now time.Time) (Point, bool) {
	if id == b.world.Self.ObjectID {
		return b.world.Self.Position(now), true
	}
	o := b.world.object(id)
	if o == nil {
		return Point{}, false
	}
	return o.Position(now), true
}

// poleWielded reports whether the bot wields a pole.
func (b *Brain) poleWielded() bool {
	kind, _ := b.wornKind()
	return kind == "POLE"
}

// poleHeld reports whether a pole swing at id from where the bot stands
// would reach a bystander at now. A swing at a monster hits no player,
// summon or pet, so outside a duel only a swing at a player or summon
// counts.
func (b *Brain) poleHeld(id int32, now time.Time) bool {
	o := b.world.object(id)
	if o == nil || !guarded(o) && !b.inDuel(now) || !b.poleWielded() {
		return false
	}
	return b.coneHit(id, b.world.Self.Position(now), o.Position(now), now)
}

// coneHit reports whether a pole swing from from at target id standing at
// to would reach a bystander at now.
func (b *Brain) coneHit(id int32, from, to Point, now time.Time) bool {
	face := math.Atan2(float64(to.Y-from.Y), float64(to.X-from.X))
	return b.bystander(id, from, poleRange+b.world.Self.CollisionRadius, &face, now)
}

// bystander reports whether anyone but target the bot's hit could harm
// stands within reach of anchor at now, reach plus its own collision
// radius in 3D (world/visibility.go rangeBody3D), and, with face set,
// inside the cone of poleHalfCone each side of that heading.
func (b *Brain) bystander(target int32, anchor Point, reach float64, face *float64, now time.Time) bool {
	for id, o := range b.world.Objects {
		if id == target || o.Dead || !b.bystanderCounts(id, o, now) {
			continue
		}
		p := o.Position(now)
		if dist3(anchor, p) > reach+b.radiusOf(id) {
			continue
		}
		if face != nil && (p.X != anchor.X || p.Y != anchor.Y) {
			d := math.Abs(math.Remainder(math.Atan2(float64(p.Y-anchor.Y), float64(p.X-anchor.X))-*face, 2*math.Pi))
			if d > poleHalfCone {
				continue
			}
		}
		return true
	}
	return false
}

// bystanderCounts reports whether a hit on o (object id) at now is one the
// area rule avoids: in a duel any player but the opponent, any summon or
// pet and any attackable NPC (a hit on any cancels the duel); in an arena a
// karma-free player not at least arenaEdge inside the bot's arena (outside
// its party), and a karma-free summon whose owner is not; elsewhere a
// flagged karma-free player outside its party, clan and alliance, and a
// summon showing a flag and no karma.
func (b *Brain) bystanderCounts(id int32, o *Object, now time.Time) bool {
	if b.inDuel(now) {
		switch {
		case o.Kind == KindPlayer:
			return id != b.duelOpponent(now)
		case o.Summon:
			return true
		}
		return o.Kind == KindNPC && o.Attackable
	}
	if !guarded(o) || o.Karma > 0 {
		return false
	}
	if own := b.arenaAt(b.world.Self.Position(now)); own.Arena != 0 {
		w := o
		if o.Summon {
			w = b.world.object(b.playerNamed(b.pvp.owners[id]))
		} else if _, mate := b.world.Party.Members[id]; mate {
			return false
		}
		if w == nil {
			return true
		}
		at := b.arenaAt(w.Position(now))
		return at.Arena != own.Arena || at.Edge < arenaEdge
	}
	if o.Summon {
		return o.PvPFlag != 0
	}
	return o.PvPFlag != 0 && !b.partyMate(id) && !b.clanMate(id)
}

// poleStand is the pole wielder's last reposition: the target it was for,
// the stand point and when it went out.
type poleStand struct {
	target int32
	at     Point
	sent   time.Time
}

// poleReposition walks a pole wielder whose swing at id would reach a
// bystander to a stand point in range with none in its cone toward id,
// trying repositionStep steps around id from the nearest, each holding on
// the geodata and on duel ground; with none it holds. A new stand point
// goes out only once the bot reached the last or a swing time passed. It
// holds while Ultimate Defense holds the bot (immobile).
func (b *Brain) poleReposition(id int32, now time.Time) Action {
	o := b.world.object(id)
	if o == nil || b.immobile(now) {
		return Action{}
	}
	from, to := b.world.Self.Position(now), o.Position(now)
	st := &b.pvp.stand
	if st.target == id && b.world.Self.Moving(now) && now.Sub(st.sent) < swingTime(b.world.Self.PAtkSpd) {
		return Action{}
	}
	ring := float64(poleRange) + b.world.Self.CollisionRadius + b.radiusOf(id) - 10
	ring = max(min(ring, math.Sqrt(float64(dist2(from, to)))), b.world.Self.CollisionRadius+b.radiusOf(id))
	base := math.Atan2(float64(from.Y-to.Y), float64(from.X-to.X))
	w := b.walker()
	for k := 1; k <= 6; k++ {
		for _, sign := range [2]float64{1, -1} {
			p := offset(to, base+sign*float64(k)*repositionStep, int32(math.Round(ring)))
			if b.coneHit(id, p, to, now) || !b.moveOK(from, p, now) {
				continue
			}
			if w.nav != nil && !(w.segmentOK(from, p) && w.segmentOK(p, from)) {
				continue
			}
			*st = poleStand{target: id, at: p, sent: now}
			return moveAction(p, from)
		}
	}
	return Action{}
}
