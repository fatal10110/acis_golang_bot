package botclient

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Duel ground (docs/pvp/finding.md "Duel ground"): the server cancels a
// duel whose duellist stands in a peace, siege or PvP zone, or strays more
// than 2000 from its opponent (character_duel.go InDuelBlockedZone), so
// from the countdown to the end the bot's own moves keep duelEdge off such
// ground and within duelLeash of its opponent, and it stops the server's
// follow before it walks the bot there.
const (
	duelEdge  = 100
	duelLeash = 1800
	// groundSample is the spacing of the points a move's straight segment
	// is checked at.
	groundSample = 20
)

// zoneDist is the plane distance from p to z's edge, negative inside z.
func zoneDist(z datapack.Zone, p Point) float64 {
	if z.Contains(p.X, p.Y, z.MinZ) || z.Contains(p.X, p.Y, z.MaxZ) {
		return -zoneEdge(z, p)
	}
	x, y := float64(p.X), float64(p.Y)
	if z.Cuboid {
		dx := max(float64(z.X[0])-x, 0, x-float64(z.X[1]))
		dy := max(float64(z.Y[0])-y, 0, y-float64(z.Y[1]))
		return math.Hypot(dx, dy)
	}
	return zoneEdge(z, p)
}

// groundDist is the plane distance from p to the nearest ground a duellist
// may not stand on (negative inside it): every peace, town, Derby track and
// arena zone, and every siege zone but those holding from, the bot's own
// spot (a running siege is invisible from outside). Without a travel map
// nothing is forbidden.
func (b *Brain) groundDist(p, from Point) float64 {
	d := math.Inf(1)
	if b.cfg.Travel == nil {
		return d
	}
	zs := &b.cfg.Travel.zones
	for _, set := range [...][]datapack.Zone{zs.Peace, zs.Towns, zs.DerbyTracks, zs.Arenas} {
		for _, z := range set {
			d = min(d, zoneDist(z, p))
		}
	}
	for _, z := range zs.Sieges {
		if !z.Contains(from.X, from.Y, from.Z) {
			d = min(d, zoneDist(z, p))
		}
	}
	return d
}

// segmentNear reports whether a point of the straight segment from from to
// to, every groundSample, comes within duelEdge of forbidden ground.
func (b *Brain) segmentNear(from, to Point) bool {
	n := int(math.Ceil(math.Sqrt(float64(dist2(from, to))) / groundSample))
	for i := 0; i <= n; i++ {
		t := 1.0
		if n > 0 {
			t = float64(i) / float64(n)
		}
		p := Point{X: from.X + int32(math.Round(t*float64(to.X-from.X))), Y: from.Y + int32(math.Round(t*float64(to.Y-from.Y))), Z: from.Z}
		if b.groundDist(p, from) < duelEdge {
			return true
		}
	}
	return false
}

// duelGroundOK reports whether the bot may step from from to to at now
// (outside a duel always): every point of the step keeps duelEdge off
// forbidden ground, or, from a spot already within duelEdge of it, the step
// leads out; and its end stays within duelLeash of the opponent's last
// position.
func (b *Brain) duelGroundOK(from, to Point, now time.Time) bool {
	if !b.inDuel(now) {
		return true
	}
	if o := b.world.object(b.duelOpponent(now)); o != nil && dist2(to, o.Position(now)) > duelLeash*duelLeash {
		return false
	}
	if b.groundDist(from, from) < duelEdge {
		return b.groundDist(to, from) > b.groundDist(from, from)
	}
	return !b.segmentNear(from, to)
}

// aggroDisc is an aggressive monster's duel clearance: where it stands and
// its aggro range plus duelClear.
type aggroDisc struct {
	at Point
	r  int64
}

// aggroDiscs are the clearances at now of the aggressive monsters in sight
// (docs/pvp/finding.md "Clear of monsters"): every live, attackable NPC but
// a summon whose template has an aggro range.
func (b *Brain) aggroDiscs(now time.Time) []aggroDisc {
	var ds []aggroDisc
	for _, o := range b.world.Objects {
		if o.Kind != KindNPC || o.Dead || o.Summon || !o.Attackable {
			continue
		}
		if r := int64(b.cfg.Travel.aggroOf(o.NPCID)); r > 0 {
			ds = append(ds, aggroDisc{at: o.Position(now), r: r + duelClear})
		}
	}
	return ds
}

// clearMargin is how far p stands outside the nearest of ds (negative or
// zero inside one), +Inf with none.
func clearMargin(p Point, ds []aggroDisc) float64 {
	m := math.Inf(1)
	for _, d := range ds {
		m = min(m, math.Sqrt(float64(dist2(p, d.at)))-float64(d.r))
	}
	return m
}

// segmentDist2 is the squared plane distance from c to the straight
// segment from a to b.
func segmentDist2(a, b, c Point) float64 {
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	t := 0.0
	if l := dx*dx + dy*dy; l > 0 {
		t = min(max((float64(c.X-a.X)*dx+float64(c.Y-a.Y)*dy)/l, 0), 1)
	}
	ex, ey := float64(a.X)+t*dx-float64(c.X), float64(a.Y)+t*dy-float64(c.Y)
	return ex*ex + ey*ey
}

// duelClearOK reports whether the bot may step from from to to at now
// (outside a duel always) by the monster clearance (docs/pvp/finding.md
// "Clear of monsters"): the straight segment keeps out of every aggressive
// monster's aggro range plus duelClear, or, from a spot already inside one,
// the step ends further out.
func (b *Brain) duelClearOK(from, to Point, now time.Time) bool {
	if !b.inDuel(now) {
		return true
	}
	ds := b.aggroDiscs(now)
	if m := clearMargin(from, ds); m <= 0 {
		return clearMargin(to, ds) > m
	}
	for _, d := range ds {
		if segmentDist2(from, to, d.at) <= float64(d.r*d.r) {
			return false
		}
	}
	return true
}

// inReach reports whether a hit of reach rng from the bot reaches id at now:
// the 3D distance within rng and both collision radii.
func (b *Brain) inReach(id int32, rng float64, now time.Time) bool {
	o := b.world.object(id)
	return o != nil && dist3(b.world.Self.Position(now), o.Position(now)) <= rng+b.world.Self.CollisionRadius+b.radiusOf(id)
}

// attackRange is the reach of the bot's swing: a bow's, a pole's or else a
// melee weapon's.
func (b *Brain) attackRange() float64 {
	switch {
	case b.bowEquipped():
		return bowRange
	case b.poleWielded():
		return poleRange
	}
	return meleeRange
}

// meleeInRange reports whether the bot's swing reaches id from where it
// stands at now.
func (b *Brain) meleeInRange(id int32, now time.Time) bool {
	return b.inReach(id, b.attackRange(), now)
}

// castInRange reports whether the bot's cast of skill reaches id from where
// it stands at now; a skill cast with no approach always does.
func (b *Brain) castInRange(id, skill int32, now time.Time) bool {
	r, approach := b.castReach(skill)
	return !approach || b.inReach(id, r.rng, now)
}

// duelHeld reports whether the server's follow toward duel opponent id
// would walk the bot onto forbidden ground at now: the bot stands within
// duelEdge of it, or the straight segment to id comes within duelEdge. The
// bot then sends no swing or cast it would walk for (docs/pvp/finding.md
// "The server's follow").
func (b *Brain) duelHeld(id int32, now time.Time) bool {
	if !b.inDuel(now) || id != b.duelOpponent(now) {
		return false
	}
	o := b.world.object(id)
	if o == nil {
		return false
	}
	from := b.world.Self.Position(now)
	return b.groundDist(from, from) < duelEdge || b.segmentNear(from, o.Position(now))
}

// duelGroundStop reports whether the bot must drop its intention on duel
// opponent id at now: it stands within duelEdge of forbidden ground, or id
// is out of its reach and the straight segment to it or to the end of its
// walk, or the bot's own leg's destination, comes within duelEdge of it.
func (b *Brain) duelGroundStop(id int32, now time.Time) bool {
	if !b.inDuel(now) || id != b.duelOpponent(now) || id != b.pvp.guard.intent {
		return false
	}
	s := &b.world.Self
	from := s.Position(now)
	if b.groundDist(from, from) < duelEdge {
		return true
	}
	if b.meleeInRange(id, now) {
		return false
	}
	if s.Moving(now) && b.groundDist(s.move.to, from) < duelEdge {
		return true
	}
	// The follow walks the bot after the opponent along its path: a walk
	// toward ground near forbidden stops as soon as it is seen.
	if o := b.world.object(id); o != nil && o.Moving(now) && b.segmentNear(from, o.move.to) {
		return true
	}
	return b.duelHeld(id, now)
}
