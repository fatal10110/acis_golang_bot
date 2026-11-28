package botclient

import (
	"math"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Sieges (docs/pvp/siege.md, D41). Every bot walks out of a running
// battlefield it does not fight in; a bot of a registered clan with Siege
// mode on finds its castle from the siege lists, walks in and fights the
// other side. The bot never registers: it sends no RequestJoinSiege (0xa4)
// or RequestConfirmSiegeWaitingList (0xa5).
//
// siegeLength is the server's SiegeLength (siege.properties, 120 minutes):
// a battlefield the bot saw running is blocked that long at most, its end
// being invisible from outside (model/zone/siege.go). siegeCastles are the
// castle ids the lists are asked for (SiegeZone.xml castleId 1-9).
// siegeListWait is how long the last list request waits for its answer.
// siegeDeaths is SiegeDeaths: the fifth death in a siege sends the bot back
// to its spot. siegeMargin is how far past the polygon's edge a walk out
// ends, siegeDepth how far inside it a walk in does, siegeArrive how close
// to either point the walk ends. siegeClear is how far other walks keep
// from a running battlefield they do not end in (siegeArea): past a
// geodata cell's diagonal, so a segment between two columns kept out
// never clips the polygon, and short of siegeMargin less siegeArrive, so a
// bot that walked out stands clear of it.
const (
	siegeLength   = 120 * time.Minute
	siegeCastles  = 9
	siegeListWait = 5 * time.Second
	siegeDeaths   = 5
	siegeMargin   = 300
	siegeDepth    = 400
	siegeArrive   = 100
	siegeClear    = 100
)

// siegeState is the bot's siege memory. fields are the battlefields (1-based,
// the travel map's Sieges) a 283 showed running, each with when its block
// lapses at the latest: two sieges may run at once, and a 283 in one keeps
// the other's block. leaving is the battlefield the bot walks out of, one
// it does not fight in (0 for none), entering the walk into its castle's;
// to is that walk's end and tried how many of its candidate points
// (siegeExits, siegeEntries) went unreachable; stuck is the battlefield
// whose exits all went unreachable, latched until the bot stands outside
// it, takes a loading-screen teleport or that field's siege ends, so the
// walk out is not planned again every decide. side is the bot's own siege state
// (UserInfo); asked counts the list requests sent since it last changed,
// askedAt the last one's time, answered the answers read; castle is the
// castle id whose list holds the bot's clan, 0 while unknown; deaths count
// its deaths in the siege; quit is set when it takes no part in this siege
// after all (no castle found, no way in, siegeDeaths deaths).
type siegeState struct {
	fields   map[int]time.Time
	leaving  int
	entering bool
	to       Point
	tried    int
	stuck    int
	side     botwire.SiegeSide
	asked    int
	askedAt  time.Time
	answered int
	castle   int32
	deaths   int
	quit     bool
}

// siegeEvent keeps the siege memory with ev at now (after the World applied
// it) and reports when the reaction to it is due: the bot's own siege state
// from its UserInfo, the castle from the list answers.
func (b *Brain) siegeEvent(ev Event, now time.Time) time.Time {
	s := &b.pvp.siege
	switch e := ev.(type) {
	case UserInfo:
		side := e.SiegeSide()
		if side == s.side {
			return time.Time{}
		}
		b.log.Info("siege: own state", "side", side, "was", s.side)
		if f := b.castleField(); side == botwire.SiegeNone && f != 0 {
			// Its siege ended (siege/lifecycle.go clears the states at
			// the end): the battlefield is open again.
			b.siegeEnded(f)
		}
		*s = siegeState{fields: s.fields, leaving: s.leaving, to: s.to, tried: s.tried, stuck: s.stuck, side: side}
		return b.pvpReact(now)
	case SiegeAttackerList:
		if b.siegeListed(e.CastleID, e.Clans, botwire.SiegeAttacker) {
			return b.pvpReact(now)
		}
	case SiegeDefenderList:
		if b.siegeListed(e.CastleID, e.Clans, botwire.SiegeDefender) {
			return b.pvpReact(now)
		}
	}
	return time.Time{}
}

// siegeListed reads a list answer of castle for side and reports whether it
// found the bot's clan there: an attacker in the attacker list, a defender
// in the defender list as its owner or an approved defender (a clan waiting
// for approval takes no part).
func (b *Brain) siegeListed(castle int32, clans []botwire.SiegeClan, side botwire.SiegeSide) bool {
	s, clan := &b.pvp.siege, b.world.Clan.ID
	if s.side != side || s.castle != 0 || s.quit || castle < 1 || castle > siegeCastles {
		return false
	}
	s.answered++
	for _, c := range clans {
		if clan != 0 && c.ClanID == clan && c.Side != botwire.SiegeDefenderPending {
			s.castle = castle
			b.log.Info("siege: own castle", "castle", castle, "side", side)
			return true
		}
	}
	return false
}

// siegeSent records a list request that went out at now.
func (b *Brain) siegeSent(a Action, now time.Time) {
	if a.kind == actSiegeList {
		b.pvp.siege.asked++
		b.pvp.siege.askedAt = now
	}
}

// siegeTeleported starts the siege walks afresh after the bot's own
// loading-screen teleport (a restart, a banishment): from there each picks
// its point again.
func (b *Brain) siegeTeleported() {
	s := &b.pvp.siege
	s.leaving, s.entering, s.to, s.tried, s.stuck = 0, false, Point{}, 0, 0
}

// siegeDied keeps a death at now while the bot takes part in a siege: the
// siegeDeaths-th sends it back to its spot (docs/pvp/siege.md "Deaths");
// the first ones keep it in, it walks back from its village restart.
func (b *Brain) siegeDied() {
	s := &b.pvp.siege
	if !b.siegeOn() || s.castle == 0 {
		return
	}
	if s.deaths++; s.deaths >= siegeDeaths {
		b.log.Info("siege: deaths", "deaths", s.deaths)
		s.quit = true
	}
}

// combatZoneSeen reads a 283 (entered a combat zone) or 284 (left it)
// outside an Olympiad match by where the bot stands (docs/pvp/siege.md
// "Telling 283 and 284 apart"): inside an arena polygon it is the arena's;
// inside a siege polygon a 283 means that siege runs (blocked from now
// until siegeLength at most), a 284 while the bot does not walk out of it
// that it ended (the zone turned off around the bot, model/zone/siege.go
// replay); elsewhere it is logged and ignored. A 284 at the edge of the
// battlefield the bot walks out of keeps the block: the siege still runs.
// Each battlefield is kept apart: a 283 in one leaves another's block.
func (b *Brain) combatZoneSeen(entered bool, now time.Time) {
	if b.cfg.Travel == nil {
		return
	}
	p := b.world.Self.Position(now)
	if b.arenaAt(p).Arena != 0 {
		return
	}
	s, i := &b.pvp.siege, b.siegeAt(p)
	switch {
	case entered && i != 0:
		if _, ok := s.fields[i]; !ok {
			b.log.Info("siege: battlefield running", "field", i, "castle", b.fieldZone(i).ResidenceCastle, "hall", b.fieldZone(i).ResidenceClanHall)
		}
		if s.fields == nil {
			s.fields = make(map[int]time.Time)
		}
		s.fields[i] = now.Add(siegeLength)
	case entered:
		b.log.Info("siege: combat zone notice outside every arena and battlefield", "at", p)
	case i != 0 && s.leaving != i:
		if _, ok := s.fields[i]; ok {
			b.log.Info("siege: battlefield ended", "field", i)
			b.siegeEnded(i)
		}
	}
}

// siegeEnded forgets battlefield f's siege: open again for spots, duels
// and walks.
func (b *Brain) siegeEnded(f int) {
	s := &b.pvp.siege
	delete(s.fields, f)
	if s.stuck == f {
		s.stuck = 0
	}
}

// siegeAt is the travel map's siege polygon holding p (1-based, 0 for none).
func (b *Brain) siegeAt(p Point) int {
	if b.cfg.Travel == nil {
		return 0
	}
	for i, z := range b.cfg.Travel.zones.Sieges {
		if z.Contains(p.X, p.Y, p.Z) {
			return i + 1
		}
	}
	return 0
}

// fieldZone is battlefield i (1-based) of the travel map.
func (b *Brain) fieldZone(i int) datapack.Zone { return b.cfg.Travel.zones.Sieges[i-1] }

// runningFields are the battlefields the bot saw running at now, in
// travel map order: each block lapses siegeLength after the last 283
// inside it.
func (b *Brain) runningFields(now time.Time) []int {
	s := &b.pvp.siege
	if len(s.fields) == 0 || b.cfg.Travel == nil {
		return nil
	}
	var fs []int
	for f := 1; f <= len(b.cfg.Travel.zones.Sieges); f++ {
		until, ok := s.fields[f]
		switch {
		case !ok:
		case !now.Before(until):
			b.log.Info("siege: battlefield block lapsed", "field", f)
			b.siegeEnded(f)
		default:
			fs = append(fs, f)
		}
	}
	return fs
}

// siegeRunning reports whether battlefield f runs at now (runningFields).
func (b *Brain) siegeRunning(f int, now time.Time) bool {
	return f != 0 && slices.Contains(b.runningFields(now), f)
}

// siegeBlocks reports whether p lies in a battlefield the bot saw running
// at now and does not fight in: no spot, duel or walk in there.
func (b *Brain) siegeBlocks(p Point, now time.Time) bool {
	for _, f := range b.runningFields(now) {
		if !b.takesPart(f) && b.fieldZone(f).Contains(p.X, p.Y, p.Z) {
			return true
		}
	}
	return false
}

// siegeArea is the ground a walk from from to dest keeps out of at now
// (docs/pvp/siege.md "Not taking part"), for the walker's avoid set: within
// siegeClear of a running battlefield the bot does not fight in, but those
// from or dest lie in or by, which the walk must leave (ruleSiegeLeave's
// walk out) or enter. A battlefield is read flat, as zoneDist reads it:
// the planner's area callback carries no height. nil when no such
// battlefield runs.
func (b *Brain) siegeArea(from, dest Point, now time.Time) func(x, y int) bool {
	type keep struct {
		z                      datapack.Zone
		minX, minY, maxX, maxY int32
	}
	var ks []keep
	for _, f := range b.runningFields(now) {
		z := b.fieldZone(f)
		if b.takesPart(f) || zoneDist(z, from) < siegeClear || zoneDist(z, dest) < siegeClear {
			continue
		}
		k := keep{z: z, minX: slices.Min(z.X) - siegeClear, minY: slices.Min(z.Y) - siegeClear, maxX: slices.Max(z.X) + siegeClear, maxY: slices.Max(z.Y) + siegeClear}
		ks = append(ks, k)
	}
	if len(ks) == 0 {
		return nil
	}
	return func(x, y int) bool {
		p := Point{X: int32(x), Y: int32(y)}
		for _, k := range ks {
			if p.X >= k.minX && p.X <= k.maxX && p.Y >= k.minY && p.Y <= k.maxY && zoneDist(k.z, p) < siegeClear {
				return true
			}
		}
		return false
	}
}

// battlefield is the running battlefield the bot stands in at now, 0 for
// none: anyone attacks anyone there without force but its own side, with
// no flag and no karma (model/zone/siege.go setBattlefieldHolds).
func (b *Brain) battlefield(now time.Time) int {
	p := b.world.Self.Position(now)
	for _, f := range b.runningFields(now) {
		if b.fieldZone(f).Contains(p.X, p.Y, p.Z) {
			return f
		}
	}
	return 0
}

// sameField reports whether player id stands in the running battlefield
// the bot stands in at now.
func (b *Brain) sameField(id int32, now time.Time) bool {
	o := b.world.object(id)
	if o == nil || o.Kind != KindPlayer {
		return false
	}
	f := b.battlefield(now)
	if f == 0 {
		return false
	}
	p := o.Position(now)
	return b.fieldZone(f).Contains(p.X, p.Y, p.Z)
}

// siegeOn reports whether the bot takes part in its clan's siege, wherever
// it stands: Siege mode on, a siege state of its own, and not quit.
func (b *Brain) siegeOn() bool {
	s := &b.pvp.siege
	return b.persona.PvP.Siege && s.side != botwire.SiegeNone && !s.quit && b.cfg.Travel != nil
}

// takesPart reports whether the bot fights in battlefield f (1-based): it
// takes part (siegeOn) and f is its castle's. A clan hall's siege is left
// only.
func (b *Brain) takesPart(f int) bool {
	c := b.pvp.siege.castle
	return b.siegeOn() && c != 0 && f != 0 && b.fieldZone(f).ResidenceCastle == int(c)
}

// castleField is the battlefield of the castle the bot fights for, 0 when
// none is known.
func (b *Brain) castleField() int {
	c := b.pvp.siege.castle
	if c == 0 || b.cfg.Travel == nil {
		return 0
	}
	for i, z := range b.cfg.Travel.zones.Sieges {
		if z.ResidenceCastle == int(c) {
			return i + 1
		}
	}
	return 0
}

// siegeFoe reports whether player id is a siege target at now
// (docs/pvp/siege.md "Taking part"): the bot takes part in the battlefield
// it stands in, and id, inside it by arenaEdge, shows InSiege on the other
// side, never Ally. The side is read from the Attacker bit against the
// bot's own: the server resends a player's relation when its own state
// changes, not the observer's (network/relations.go broadcastRelations),
// so Ally and Enemy may still show the bot's state before the siege.
func (b *Brain) siegeFoe(id int32, now time.Time) bool {
	f := b.battlefield(now)
	r, o := b.pvp.opponents[id], b.world.object(id)
	if f == 0 || r == nil || o == nil || !b.takesPart(f) {
		return false
	}
	rel := r.Relation
	if rel&botwire.RelationInSiege == 0 || rel&botwire.RelationAlly != 0 {
		return false
	}
	if attacker := rel&botwire.RelationAttacker != 0; attacker == (b.pvp.siege.side == botwire.SiegeAttacker) {
		return false
	}
	z, p := b.fieldZone(f), o.Position(now)
	return z.Contains(p.X, p.Y, p.Z) && zoneEdge(z, p) >= arenaEdge
}

// siegeAlly reports whether player id shows Ally or the bot's own side in
// a siege: never fought there, even when it hits (it needs force to).
func (b *Brain) siegeAlly(id int32) bool {
	r := b.pvp.opponents[id]
	s := &b.pvp.siege
	if r == nil || r.Relation&botwire.RelationInSiege == 0 {
		return false
	}
	if r.Relation&botwire.RelationAlly != 0 {
		return true
	}
	return s.side != botwire.SiegeNone && (r.Relation&botwire.RelationAttacker != 0) == (s.side == botwire.SiegeAttacker)
}

// ruleSiegeLeave walks the bot out of a running battlefield it does not
// fight in (docs/pvp/siege.md "Not taking part"), every bot, by the nearest
// point outside (siegeExits), the next one when the walker answers one
// unreachable. It goes after "attacked": an attacker that hits it on the
// way is fought back (MayFight rule 3b). Walks are the walker's (goTo, the
// frame pacer).
func (b *Brain) ruleSiegeLeave(now time.Time) (Action, bool) {
	s, self := &b.pvp.siege, &b.world.Self
	f := b.battlefield(now)
	if f != 0 && b.takesPart(f) || b.siegeOn() && s.castle == 0 {
		// It fights here, or its lists are still out: a defender's
		// castle holds it from the start (BanishForeigners spares the
		// owner's clan), and the 283 comes ahead of its siege state.
		s.leaving, s.to = 0, Point{}
		return Action{}, false
	}
	inside := f != 0
	if s.stuck != f {
		s.stuck = 0
	}
	if s.stuck != 0 {
		// No way out of this battlefield: the later rules run.
		return Action{}, false
	}
	if inside && s.leaving != f {
		s.leaving, s.to, s.tried = 0, Point{}, 0
	}
	if !inside && s.leaving == 0 || !b.siegeRunning(max(f, s.leaving), now) || self.Dead || b.inOlympiad(now) || b.inDuel(now) {
		s.leaving = 0
		return Action{}, false
	}
	if s.leaving == 0 {
		s.leaving = f
		b.log.Info("siege: leaving a running battlefield", "field", f)
	}
	if b.immobile(now) {
		return Action{}, true
	}
	if s.to == (Point{}) {
		exits := siegeExits(b.fieldZone(s.leaving), self.Position(now))
		if s.tried >= len(exits) {
			b.log.Info("siege: no way out of the battlefield", "field", s.leaving)
			s.leaving, s.stuck = 0, f
			return Action{}, false
		}
		s.to = exits[s.tried]
	}
	a, st := b.goTo(s.to, siegeArrive, walkPlace, now)
	switch st {
	case walkWalking:
		return a, true
	case walkUnreachable:
		s.to, s.tried = Point{}, s.tried+1
		return Action{}, true
	}
	s.leaving, s.to = 0, Point{}
	return Action{}, false
}

// ruleSiege is the siege errand (docs/pvp/siege.md "Taking part"): with
// Siege mode on and a siege state of its own the bot asks its side's list
// for each castle, one request per bypassGap (gateBypass, actSiegeList),
// at most siegeCastles per siege state change, until one holds its clan;
// then it walks into that castle's battlefield on foot (the castle towns'
// gatekeepers refuse a trip there while the siege runs) and holds every
// later rule inside it while it takes part ("siege fight" fights). No list
// holding its clan, or no way in, and it takes no part.
func (b *Brain) ruleSiege(now time.Time) (Action, bool) {
	s, self := &b.pvp.siege, &b.world.Self
	if !b.siegeOn() || self.Dead || b.inOlympiad(now) || b.inDuel(now) {
		return Action{}, false
	}
	if s.castle == 0 {
		if s.asked < siegeCastles {
			return b.siegeListAction(int32(s.asked + 1)), true
		}
		if s.answered < s.asked && now.Sub(s.askedAt) < siegeListWait {
			return Action{}, false
		}
		b.log.Info("siege: own clan in no castle's list", "side", s.side, "answered", s.answered)
		s.quit = true
		return Action{}, false
	}
	f := b.castleField()
	if f == 0 {
		b.log.Info("siege: no battlefield for the castle", "castle", s.castle)
		s.quit = true
		return Action{}, false
	}
	z, p := b.fieldZone(f), self.Position(now)
	if z.Contains(p.X, p.Y, p.Z) && zoneEdge(z, p) >= arenaEdge && (!s.entering || zoneEdge(z, p) >= siegeDepth/2) {
		s.entering, s.to = false, Point{}
		return Action{}, true
	}
	if !s.entering {
		s.entering, s.to, s.tried = true, Point{}, 0
		b.leaveSpot()
		b.log.Info("siege: walking into the battlefield", "castle", s.castle)
	}
	if b.immobile(now) {
		return Action{}, true
	}
	if s.to == (Point{}) {
		entries := siegeEntries(z, p)
		if s.tried >= len(entries) {
			b.log.Info("siege: no way into the battlefield", "castle", s.castle)
			s.quit, s.entering = true, false
			return Action{}, false
		}
		s.to = entries[s.tried]
	}
	a, st := b.goTo(s.to, siegeArrive, walkPlace, now)
	switch st {
	case walkWalking:
		return a, true
	case walkUnreachable:
		s.to, s.tried = Point{}, s.tried+1
		return Action{}, true
	}
	s.entering, s.to = false, Point{}
	return Action{}, true
}

// siegeListAction asks the list of castle for the bot's side.
func (b *Brain) siegeListAction(castle int32) Action {
	payload := botwire.EncodeRequestSiegeAttackerList(castle)
	if b.pvp.siege.side == botwire.SiegeDefender {
		payload = botwire.EncodeRequestSiegeDefenderList(castle)
	}
	return Action{kind: actSiegeList, payload: payload}
}

// ruleSiegeFight attacks siege targets (docs/pvp/siege.md "Taking part"):
// inside its castle's battlefield, with no attacker (the attacked rule's)
// and out of combat stance, it keeps its target while it stays one, or
// picks the nearest player siegeFoe accepts, of any tier, like an arena
// target. No door, tower, flag or NPC; a siege guard only when it hits
// (the attacked rule).
func (b *Brain) ruleSiegeFight(now time.Time) (Action, bool) {
	s := &b.world.Self
	f := b.battlefield(now)
	if f == 0 || !b.takesPart(f) || s.InCombat || b.inDuel(now) || len(b.attackers(now)) > 0 || !b.aboveStand() {
		return Action{}, false
	}
	id := b.world.Target
	if !b.siegeTarget(id, now) {
		id = b.nearestPlayer(now, func(id int32, _ *Object) bool { return b.siegeTarget(id, now) })
	}
	if id == 0 {
		return Action{}, false
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	return b.fight(id, now), true
}

// siegeTarget reports whether player id is one the bot fights in its siege
// at now: MayFight lets it by rule 3b's siege target.
func (b *Brain) siegeTarget(id int32, now time.Time) bool {
	o := b.world.object(id)
	return o != nil && o.Kind == KindPlayer && b.mayFight(id, now) && b.fightRule(id, o, now) == FightSiege
}

// siegeExits are the points siegeMargin outside zone z nearest p, one per
// edge of its polygon (or box), the nearest first: where a walk out of the
// battlefield ends.
func siegeExits(z datapack.Zone, p Point) []Point { return edgePoints(z, p, siegeMargin, false) }

// siegeEntries are the points siegeDepth inside zone z nearest p, one per
// edge, the nearest first: where a walk into the battlefield ends.
func siegeEntries(z datapack.Zone, p Point) []Point { return edgePoints(z, p, siegeDepth, true) }

// edgePoints projects p on each edge of z and steps d along the edge's
// normal, inward or outward, keeping the points on the wanted side of z at
// p's height (clamped into z's band), the nearest p first.
func edgePoints(z datapack.Zone, p Point, d float64, inward bool) []Point {
	var xs, ys []float64
	if z.Cuboid {
		x0, x1, y0, y1 := float64(z.X[0]), float64(z.X[1]), float64(z.Y[0]), float64(z.Y[1])
		xs, ys = []float64{x0, x1, x1, x0}, []float64{y0, y0, y1, y1}
	} else {
		for i := range z.X {
			xs, ys = append(xs, float64(z.X[i])), append(ys, float64(z.Y[i]))
		}
	}
	h := max(z.MinZ, min(z.MaxZ, p.Z))
	px, py := float64(p.X), float64(p.Y)
	type cand struct {
		at Point
		d  float64
	}
	var cands []cand
	for i := range xs {
		j := (i + 1) % len(xs)
		ex, ey := xs[j]-xs[i], ys[j]-ys[i]
		l := math.Hypot(ex, ey)
		if l == 0 {
			continue
		}
		t := max(0, min(1, ((px-xs[i])*ex+(py-ys[i])*ey)/(l*l)))
		qx, qy := xs[i]+t*ex, ys[i]+t*ey
		nx, ny := ey/l, -ex/l
		for _, sign := range [...]float64{1, -1} {
			at := Point{X: int32(math.Round(qx + sign*d*nx)), Y: int32(math.Round(qy + sign*d*ny)), Z: p.Z}
			if z.Contains(at.X, at.Y, h) == inward {
				cands = append(cands, cand{at, math.Hypot(qx-px, qy-py)})
				break
			}
		}
	}
	slices.SortStableFunc(cands, func(a, b cand) int {
		switch {
		case a.d < b.d:
			return -1
		case a.d > b.d:
			return 1
		}
		return 0
	})
	out := make([]Point, len(cands))
	for i, c := range cands {
		out[i] = c.at
	}
	return out
}
