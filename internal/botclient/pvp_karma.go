package botclient

import (
	"math"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Death and karma (docs/pvp/karma.md, X11, D39): the arena return loop,
// the avoid list, PK mode (MayFight rule 7) and the karma state.
//
// arenaDeaths is the arena death within arenaDeathWindow that ends an arena
// trip, avoidAfterDeath how long a player who killed the bot in the open
// world is neither hunted nor challenged, pkEvery and pkCap PK mode's
// limits (one PK per pkEvery, none at pkCap PK kills: one under the
// server's MinimumPKRequiredToDrop of 5, players.properties, so the bot's
// items never drop), karmaClear the margin a karma bot keeps from every
// town polygon and guard spawn (the guards' 450 see range,
// model/actor/npc/ai_config.go, plus a margin).
const (
	arenaDeaths      = 5
	arenaDeathWindow = time.Hour
	avoidAfterDeath  = 30 * time.Minute
	pkEvery          = time.Hour
	pkCap            = 4
	karmaClear       = 600
	// karmaSlack widens karmaClear by about a geodata cell's diagonal: the
	// walker's avoid set tests column centres, so a line it lets through
	// passes at least karmaClear from the ground kept.
	karmaSlack = 16
	// karmaBucket is the side of the grid the guard spawns are bucketed
	// in, over twice the margin, so a point's guards lie in its own bucket
	// or one next to it.
	karmaBucket = 2048
	// karmaStep is the spacing a flee line is sampled at against the
	// margin: a geodata cell.
	karmaStep = 16
)

// npcTypeGuard is the guards' NPC template type (datapack.NPC.Type).
const npcTypeGuard = "Guard"

// karmaState is the death and karma memory, owned by the brain goroutine:
// the arena deaths of the last arenaDeathWindow, the killers avoided (by
// name) until when, PK mode's victim and when its PK began (the PK
// clock), whether the bot had karma at the last decision, the spots whose
// walk failed under karma, and the karma margin's ground (built on first
// use).
type karmaState struct {
	arenaDeaths []time.Time
	avoid       holds[string]
	victim      int32
	lastPK      time.Time
	had         bool
	failed      []int
	keep        *karmaKeep
}

// karmaEvent keeps the death and karma memory with ev at now, before the
// World applies it: the bot's own death reads the attackers table the
// World clears on it.
func (b *Brain) karmaEvent(ev Event, now time.Time) {
	k := &b.karma
	switch e := ev.(type) {
	case Die:
		switch e.ObjectID {
		case b.world.Self.ObjectID:
			b.diedPvP(now)
		case k.victim:
			k.victim = 0
		}
	case DeleteObject:
		if e.ObjectID == k.victim {
			k.victim = 0
		}
	}
}

// diedPvP keeps the bot's own death at now (docs/pvp/karma.md "By fight"):
// none in a duel; else a death to a player counted by its kind
// (deathCounted), and in an arena the arena death (arenaDied), in the open
// world the player who killed it avoided for avoidAfterDeath.
func (b *Brain) diedPvP(now time.Time) {
	k := &b.karma
	k.victim = 0
	if b.inDuel(now) {
		return
	}
	name := b.killer(now)
	b.deathCounted(name)
	if b.inArena(now) {
		b.arenaDied(now)
		return
	}
	if name == "" {
		return
	}
	k.avoid.prune(now)
	k.avoid.set(name, now.Add(avoidAfterDeath))
}

// killer is the name of the player who killed the bot at now: the hardest
// hitter of its attackers table that is a player, or a summon's owner by
// its title; "" when no player hit it.
func (b *Brain) killer(now time.Time) string {
	var (
		name   string
		damage int64
	)
	for id, a := range b.world.Attackers {
		o := b.world.object(id)
		if o == nil || !b.world.attacking(id, now) {
			continue
		}
		n := o.Name
		switch {
		case o.Summon:
			n = b.pvp.owners[id]
		case o.Kind != KindPlayer:
			n = ""
		}
		if n != "" && (name == "" || a.Damage > damage || a.Damage == damage && n < name) {
			name, damage = n, a.Damage
		}
	}
	return name
}

// avoided reports whether the player named name killed the bot within
// avoidAfterDeath of now: neither hunted nor challenged (fighting back
// stays).
func (b *Brain) avoided(name string, now time.Time) bool {
	return b.karma.avoid.held(name, now)
}

// arenaDied keeps an arena death at now on an arena trip: the first
// arenaDeaths-1 within arenaDeathWindow keep the trip, so the bot walks back
// from its restart (arenaTeleported); the arenaDeaths-th ends it, and the
// spot rule sends the bot to its spot.
func (b *Brain) arenaDied(now time.Time) {
	t, k := &b.pvp.arena, &b.karma
	if !t.active() {
		return
	}
	k.arenaDeaths = slices.DeleteFunc(k.arenaDeaths, func(at time.Time) bool { return now.Sub(at) >= arenaDeathWindow })
	k.arenaDeaths = append(k.arenaDeaths, now)
	if len(k.arenaDeaths) >= arenaDeaths {
		b.arenaEnd(now)
		return
	}
	t.died = true
}

// pkAllowed reports whether PK mode's limits pass at now
// (docs/pvp/karma.md "PK mode"): under pkCap PK kills (UserInfo) and pkEvery
// since the bot's last PK.
func (b *Brain) pkAllowed(now time.Time) bool {
	k := &b.karma
	return b.world.Self.PKKills < pkCap && (k.lastPK.IsZero() || now.Sub(k.lastPK) >= pkEvery)
}

// pkVictim reports whether player id is PK mode's victim at now: the one
// its last PK began on, within pkEvery of it.
func (b *Brain) pkVictim(id int32, now time.Time) bool {
	k := &b.karma
	return id != 0 && id == k.victim && now.Sub(k.lastPK) < pkEvery
}

// pkStart makes player id PK mode's victim at now and restarts the PK
// clock: the fight that may end in the PK.
func (b *Brain) pkStart(id int32, now time.Time) {
	b.karma.victim, b.karma.lastPK = id, now
}

// pkFightBack starts the PK of a clean attacker rule 7 lets the bot fight
// at now (a karma bot's, docs/pvp/karma.md "With karma"): the PK clock
// restarts as the bot answers it, unless the attacker is the victim of a
// PK within pkEvery (a victim past it is a new PK).
func (b *Brain) pkFightBack(id int32, now time.Time) {
	o := b.world.object(id)
	if o == nil || o.Kind != KindPlayer || b.pkVictim(id, now) || b.fightRule(id, o, now) != FightPK {
		return
	}
	b.pkStart(id, now)
}

// pkHunt is PK mode's victim at now, 0 for none (docs/pvp/karma.md "PK
// mode"): the victim of a PK under way while in sight, alive and off the
// ground hunting refuses, else, with the limits passing, the nearest clean
// player of the bot's tier that rules 2 to 5 do not refuse, not avoided,
// off that ground, that the engage estimate passes. ruleHunt checked the
// bot's own preconditions.
func (b *Brain) pkHunt(now time.Time) int32 {
	if !b.persona.PvP.ForceAttack {
		return 0
	}
	if v := b.karma.victim; b.pkVictim(v, now) {
		if o := b.world.object(v); o != nil && !o.Dead && !b.noHunt(o.Position(now)) {
			return v
		}
		return 0
	}
	if !b.pkAllowed(now) {
		return 0
	}
	tier := classTier(b.world.Self.ClassID)
	return b.nearestPlayer(now, func(id int32, o *Object) bool {
		return clean(o) && !b.friend(id) && !b.cfg.markers.Marks(o.Name) && !b.sameArena(id, now) &&
			!b.avoided(o.Name, now) && classTier(o.ClassID) == tier && !b.noHunt(o.Position(now)) && b.engages(id)
	})
}

// pkSkill reports whether the bot's skill may go on a clean player under a
// forced attack (docs/pvp/karma.md "PK mode"): a direct-damage skill
// (IsDamage) that is not isDebuff, which the server refuses on a clean
// player even with ctrl (data/xml/skill.go), and lands no effect, so no
// control, Hold or Int stun, slow or DOT. One with no datapack entry may
// not.
func (b *Brain) pkSkill(skill int32) bool {
	s, ok := b.skillData(skill)
	return ok && isDamageTypes[s.Type] && !s.Debuff && len(s.Effects) == 0
}

// pkSteps are the steps of rot the bot plays on a clean player it attacks
// by force: its self and sustain steps, and the attacks pkSkill allows.
func (b *Brain) pkSteps(rot []Step) []Step {
	return slices.DeleteFunc(slices.Clone(rot), func(st Step) bool {
		return st.rank() == rankAttack && !b.pkSkill(st.Skill)
	})
}

// karmaTick keeps the karma state at the decision at now
// (docs/pvp/karma.md "With karma"): karma ends an arena trip under way
// outside its arena (a karma bot in an arena is everyone's target); once
// karma is back to 0 the spots the walk failed under karma are no longer
// avoided, and a spot the bot fell back to is left, so it picks its own
// again.
func (b *Brain) karmaTick(now time.Time) {
	k, has := &b.karma, b.world.Self.Karma > 0
	if t := &b.pvp.arena; has && t.active() && !b.inArena(now) {
		b.arenaEnd(now)
	}
	if k.had && !has && len(k.failed) > 0 {
		v := &b.spot
		for _, i := range k.failed {
			delete(v.avoid, i)
		}
		v.failed, v.retryAt = v.failed[:0], time.Time{}
		b.leaveSpot()
		k.failed = nil
	}
	k.had = has
}

// karmaWalkFailed keeps the spot whose walk just failed while the bot has
// karma: no karma route reaches it (karmaArea).
func (b *Brain) karmaWalkFailed() {
	if k := &b.karma; b.world.Self.Karma > 0 && !slices.Contains(k.failed, b.spot.idx) {
		k.failed = append(k.failed, b.spot.idx)
	}
}

// karmaGround is the karma margin's ground of the travel map, nil without
// one, built on first use.
func (b *Brain) karmaGround() *karmaKeep {
	k := &b.karma
	if k.keep == nil && b.cfg.Travel != nil {
		k.keep = newKarmaKeep(b.cfg.Travel.zones.Towns, b.cfg.Travel.guards)
	}
	return k.keep
}

// karmaArea is the ground a walk from from to dest keeps out of at karma
// above 0 (docs/pvp/karma.md "With karma"), for the walker's avoid set:
// within karmaClear of a town polygon or guard spawn, but the margins from
// or dest already lie in, which the walk must leave or enter. nil without
// karma or a travel map.
func (b *Brain) karmaArea(from, dest Point) func(x, y int) bool {
	if b.world.Self.Karma <= 0 {
		return nil
	}
	k := b.karmaGround()
	if k == nil || k.empty() {
		return nil
	}
	skip := append(k.within(from), k.within(dest)...)
	return func(x, y int) bool { return k.near(Point{X: int32(x), Y: int32(y)}, skip) }
}

// karmaLine reports whether the straight line from from to to enters the
// karma margin at karma above 0, but the margins from already lies in: a
// flee heading the bot skips like a blocked one.
func (b *Brain) karmaLine(from, to Point) bool {
	if b.world.Self.Karma <= 0 {
		return false
	}
	k := b.karmaGround()
	if k == nil || k.empty() {
		return false
	}
	skip := k.within(from)
	dx, dy := float64(to.X-from.X), float64(to.Y-from.Y)
	n := int(math.Hypot(dx, dy) / karmaStep)
	for i := 1; i <= n+1; i++ {
		f := min(float64(i)/float64(n+1), 1)
		p := Point{X: from.X + int32(math.Round(dx*f)), Y: from.Y + int32(math.Round(dy*f))}
		if k.near(p, skip) {
			return true
		}
	}
	return false
}

// karmaKeep is the ground a karma bot keeps karmaClear from: the town
// polygons (X2) and the guard spawns, each with its box grown by the
// margin, the guards bucketed by karmaBucket. An area's id is its town's
// index, or the towns' count plus its guard's index.
type karmaKeep struct {
	towns  []datapack.Zone
	boxes  [][4]float64
	guards []Point
	bucket map[[2]int32][]int
}

// newKarmaKeep is the karma margin's ground of towns and guards.
func newKarmaKeep(towns []datapack.Zone, guards []Point) *karmaKeep {
	k := &karmaKeep{towns: towns, guards: guards, bucket: make(map[[2]int32][]int)}
	r := float64(karmaClear + karmaSlack)
	for _, z := range towns {
		bx := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
		for i := range z.X {
			bx[0], bx[2] = min(bx[0], float64(z.X[i])-r), max(bx[2], float64(z.X[i])+r)
		}
		for i := range z.Y {
			bx[1], bx[3] = min(bx[1], float64(z.Y[i])-r), max(bx[3], float64(z.Y[i])+r)
		}
		k.boxes = append(k.boxes, bx)
	}
	for i, g := range guards {
		key := bucketOf(g)
		k.bucket[key] = append(k.bucket[key], i)
	}
	return k
}

// bucketOf is p's karma bucket.
func bucketOf(p Point) [2]int32 {
	return [2]int32{int32(math.Floor(float64(p.X) / karmaBucket)), int32(math.Floor(float64(p.Y) / karmaBucket))}
}

// empty reports whether k keeps nothing.
func (k *karmaKeep) empty() bool { return len(k.towns) == 0 && len(k.guards) == 0 }

// visit calls f with the id of every area whose margin holds p, until f
// returns false.
func (k *karmaKeep) visit(p Point, f func(id int) bool) {
	r := float64(karmaClear + karmaSlack)
	x, y := float64(p.X), float64(p.Y)
	for i, z := range k.towns {
		bx := k.boxes[i]
		if x < bx[0] || x > bx[2] || y < bx[1] || y > bx[3] || zoneDist(z, p) > r {
			continue
		}
		if !f(i) {
			return
		}
	}
	c := bucketOf(p)
	for dx := int32(-1); dx <= 1; dx++ {
		for dy := int32(-1); dy <= 1; dy++ {
			for _, i := range k.bucket[[2]int32{c[0] + dx, c[1] + dy}] {
				g := k.guards[i]
				if math.Hypot(x-float64(g.X), y-float64(g.Y)) > r {
					continue
				}
				if !f(len(k.towns) + i) {
					return
				}
			}
		}
	}
}

// within is the ids of the areas whose margin holds p.
func (k *karmaKeep) within(p Point) []int {
	var ids []int
	k.visit(p, func(id int) bool {
		ids = append(ids, id)
		return true
	})
	return ids
}

// near reports whether p lies in the margin of an area not in skip.
func (k *karmaKeep) near(p Point, skip []int) bool {
	found := false
	k.visit(p, func(id int) bool {
		found = !slices.Contains(skip, id)
		return !found
	})
	return found
}
