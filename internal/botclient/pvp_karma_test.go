package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// karmaFight is a bot with karma 240 hit by pcID (flag and karma as
// given) on the test geodata, so a flee walks, its kit's Flee Dash.
func karmaFight(t *testing.T, flag uint8, karma int32) *Brain {
	t.Helper()
	b := guardBrain(t, false, flag, karma)
	b.cfg.Nav = testNav(t)
	b.walk = newWalker(b.cfg.Nav)
	b.world.SetGround(b.cfg.Nav.Engine)
	s := &b.world.Self
	s.Karma, s.RunSpeed = 240, 150
	from := Point{X: 4000, Y: 2000}
	s.move = stopAt(from)
	place(t, b, pcID, Point{X: 4000, Y: 2300})
	withKit(b, ClassKit{Flee: skillDash})
	return b
}

// fled reports whether a is a run away (a move, or the Flee skill Dash on
// the bot itself) and no Action, AttackRequest or cast on an opponent.
func fled(a Action) bool {
	return a.kind == actMove || a.kind == actCast && a.skill == skillDash && a.object == selfID
}

// actsOn reports whether a acts on player id: a click, an attack or a cast.
func actsOn(a Action, id int32) bool {
	return a.object == id && (a.kind == actSelect || a.kind == actAttack || a.kind == actCast)
}

// TestPvPKarma checks the karma state (docs/pvp/karma.md "With karma").
func TestPvPKarma(t *testing.T) {
	t.Run("karma 240, PK mode off, hit by a clean player: flee, Dash if ready", func(t *testing.T) {
		b := karmaFight(t, 0, 0)
		rule, a := b.decide(at(time.Second))
		if rule != "attacked" || a.kind != actMove {
			t.Fatalf("rule %q action %v, want the attacked rule's flee move", rule, a.kind)
		}
		b.world.Apply(SkillList{Skills: []KnownSkill{{ID: skillDash, Level: 1}}}, t0)
		if _, a := b.decide(at(time.Second)); !fled(a) || a.kind != actCast {
			t.Fatalf("action %v skill %d, want Dash", a.kind, a.skill)
		}
	})
	t.Run("a clean mystic's Wind Strike only: flee", func(t *testing.T) {
		b := karmaFight(t, 0, 0)
		b.cfg.Travel.skills[datapack.SkillRef{ID: skillWindStrike, Level: 1}] = mysticPvPSkills[skillWindStrike]
		clear(b.world.Attackers)
		b.handleOne(t, castAt(pcID, selfID, skillWindStrike), at(500*time.Millisecond))
		if !b.world.attacking(pcID, at(time.Second)) {
			t.Fatal("the Wind Strike put no attacker in the table")
		}
		if _, a := b.decide(at(time.Second)); !fled(a) {
			t.Fatalf("action %v on %d, want a flee", a.kind, a.object)
		}
	})
	t.Run("karma 0, a clean player's Wind Strike: held, then fought once flagged", func(t *testing.T) {
		b := karmaFight(t, 0, 0)
		b.world.Self.Karma = 0
		b.cfg.Travel.skills[datapack.SkillRef{ID: skillWindStrike, Level: 1}] = mysticPvPSkills[skillWindStrike]
		clear(b.world.Attackers)
		b.handleOne(t, castAt(pcID, selfID, skillWindStrike), at(500*time.Millisecond))
		if rule, a := b.decide(at(time.Second)); rule != "attacked" || a.kind != (Action{}).kind {
			t.Fatalf("rule %q action %v, want held", rule, a.kind)
		}
		b.handleOne(t, RelationChanged{ObjectID: pcID, PvPFlag: 1}, at(1500*time.Millisecond))
		if _, a := b.decide(at(2 * time.Second)); !actsOn(a, pcID) {
			t.Fatalf("action %v on %d, want the flagged attacker fought", a.kind, a.object)
		}
	})
	t.Run("unable to outrun: keeps fleeing", func(t *testing.T) {
		b := karmaFight(t, 0, 0)
		b.persona.FleeOutrun = true
		b.world.object(pcID).RunSpeed = 200
		if _, a := b.decide(at(time.Second)); !fled(a) {
			t.Fatalf("action %v, want a flee", a.kind)
		}
	})
	pkRow := func(t *testing.T, kills int32, last time.Duration) *Brain {
		b := pvpRowBrain(t, 1, 36, "BIGBLUNT", lyrWarrior, skillPowerSmash, skillStunAttack)
		b.cfg.Nav = testNav(t)
		b.walk = newWalker(b.cfg.Nav)
		b.world.SetGround(b.cfg.Nav.Engine)
		b.persona.PvP.ForceAttack = true
		s := &b.world.Self
		s.Karma, s.PKKills, s.RunSpeed = 240, kills, 150
		s.move = stopAt(Point{X: 4000, Y: 2000})
		cleanFoe(t, b, lyrWarrior, Point{X: 4000, Y: 2090})
		b.handleOne(t, hitSelf(pcID, 10), t0)
		b.karma.lastPK = at(time.Second).Add(-last)
		return b
	}
	t.Run("PK mode on, 1 PK kill, last PK 2 hours ago: the PK fight back", func(t *testing.T) {
		b := pkRow(t, 1, 2*time.Hour)
		now := at(time.Second)
		_, a := b.decide(now)
		if a.kind != actCast || a.skill != skillPowerSmash || string(a.payload) != string(botwire.EncodeRequestMagicSkillUse(skillPowerSmash, true)) {
			t.Fatalf("action %v skill %d, want the ctrl-held Power Smash", a.kind, a.skill)
		}
		if b.karma.victim != pcID || !b.karma.lastPK.Equal(now) {
			t.Fatalf("victim %d PK clock %v, want %d restarted at %v", b.karma.victim, b.karma.lastPK, pcID, now)
		}
		b.world.holdSkill(skillPowerSmash, at(time.Minute))
		if _, a := b.decide(at(2 * time.Second)); a.kind != actAttack || a.object != pcID {
			t.Fatalf("action %v skill %d, want the AttackRequest, never Stun Attack", a.kind, a.skill)
		}
	})
	t.Run("the victim of the PK 2 hours ago attacks again: the PK clock restarts", func(t *testing.T) {
		b := pkRow(t, 1, 2*time.Hour)
		b.karma.victim = pcID
		now := at(time.Second)
		if _, a := b.decide(now); !actsOn(a, pcID) {
			t.Fatalf("action %v on %d, want the PK fight back", a.kind, a.object)
		}
		if b.karma.victim != pcID || !b.karma.lastPK.Equal(now) {
			t.Fatalf("victim %d PK clock %v, want %d restarted at %v", b.karma.victim, b.karma.lastPK, pcID, now)
		}
	})
	for _, tc := range []struct {
		name  string
		kills int32
		last  time.Duration
	}{
		{"last PK 30 minutes ago: flee", 1, 30 * time.Minute},
		{"4 PK kills: flee", pkCap, 2 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := pkRow(t, tc.kills, tc.last)
			if _, a := b.decide(at(time.Second)); !fled(a) {
				t.Fatalf("action %v skill %d, want a flee", a.kind, a.skill)
			}
			if b.karma.victim != 0 {
				t.Fatal("a PK started past the limits")
			}
		})
	}
	t.Run("a flagged attacker: fought under the guard", func(t *testing.T) {
		b := karmaFight(t, 1, 0)
		if _, a := b.decide(at(time.Second)); !actsOn(a, pcID) {
			t.Fatalf("action %v, want the flagged attacker fought", a.kind)
		}
		b.handleOne(t, RelationChanged{ObjectID: pcID, PvPFlag: 2}, at(1500*time.Millisecond))
		if o := b.world.object(pcID); !b.blinking(pcID, o, at(2*time.Second)) {
			t.Fatal("the flag guard is off for a blinking attacker")
		}
	})
	t.Run("a karma attacker: fought", func(t *testing.T) {
		b := karmaFight(t, 0, 500)
		if _, a := b.decide(at(time.Second)); !actsOn(a, pcID) {
			t.Fatalf("action %v, want the karma attacker fought", a.kind)
		}
	})
	t.Run("dead with karma: the village restart", func(t *testing.T) {
		b := karmaFight(t, 0, 0)
		b.persona.Death = restartToVillage
		b.spot.held = false
		b.handleOne(t, Die{ObjectID: selfID, Restart: RestartPoints{Village: true}}, at(time.Second))
		if rule, a := b.decide(at(2 * time.Second)); rule != "dead" || a.kind != actRestart {
			t.Fatalf("rule %q action %v, want the village restart", rule, a.kind)
		}
		if !b.avoided(foeName, at(29*time.Minute)) {
			t.Fatal("the clean killer is not avoided")
		}
	})
	t.Run("dead with karma: restarted outside town, no town trip, a walk to its spot, farming", func(t *testing.T) {
		for _, karma := range []int32{0, 240} {
			b := newTripBrain(t)
			b.persona.Death = restartToVillage
			b.persona.Spots.DeathAvoid = 0 // its one spot stays its own
			b.world.Self.Karma = karma
			b.handleOne(t, Die{ObjectID: selfID, Restart: RestartPoints{Village: true}}, at(time.Second))
			if rule, a := b.decide(at(2 * time.Second)); rule != "dead" || a.kind != actRestart {
				t.Fatalf("karma %d: rule %q action %v, want the village restart", karma, rule, a.kind)
			}
			if err := b.handle([]Event{Revive{ObjectID: selfID}, TeleportToLocation{ObjectID: selfID, Pos: Point{X: 2000}}}, at(3*time.Second)); err != nil {
				t.Fatal(err)
			}
			b.appearing = false // the Appearing went out
			b.supplyTrip.potions = true
			rule, a := b.decide(at(4 * time.Second))
			if rule != "spot" || a.kind != actMove {
				t.Fatalf("karma %d: rule %q action %v, want the walk to its spot", karma, rule, a.kind)
			}
			b.world.Self.move = stopAt(Point{})
			b.handleOne(t, NPCInfo{ObjectID: mobObj, NPCID: mob, Attackable: true, Pos: Point{X: 100}}, at(5*time.Second))
			var rules []string
			for i := range 3 {
				rule, _ := b.decide(at(time.Duration(6+i) * time.Second))
				rules = append(rules, rule)
			}
			if karma == 0 {
				if !slices.Contains(rules, "supply") {
					t.Fatalf("karma 0 at the spot: rules %q, want the supply trip", rules)
				}
				continue
			}
			if !slices.Contains(rules, "farm") || slices.ContainsFunc(rules, karmaRule) {
				t.Fatalf("karma %d at the spot: rules %q, want farming, no town trip", karma, rules)
			}
		}
	})
	t.Run("a flee heading into a town margin: the first keeping out", func(t *testing.T) {
		b := karmaFight(t, 0, 0)
		from := b.world.Self.Position(at(time.Second))
		b.cfg.Travel.zones.Towns = []datapack.Zone{box(3500, 600, 4500, 1000)}
		b.persona.FleeDistance = 1000
		to := b.fleePoint(pcID, at(time.Second))
		if b.karmaLine(from, to) {
			t.Fatalf("flee point %v: the line enters the town's margin", to)
		}
		if to.Y <= from.Y-900 && to.X == from.X {
			t.Fatalf("flee point %v: straight away, into the margin", to)
		}
		b.world.Self.Karma = 0
		if to := b.fleePoint(pcID, at(time.Second)); to.X != from.X || to.Y >= from.Y {
			t.Fatalf("flee point %v without karma, want straight away", to)
		}
	})
	t.Run("no hunt, challenge or arena trip until karma 0, then all resume", func(t *testing.T) {
		hunt := func(karma int32) bool {
			b := huntBrain(t)
			b.world.Self.Karma = karma
			b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, PvPFlag: 1}, t0)
			return fightsWith(b, "hunt", pcID, at(time.Second))
		}
		if hunt(240) || !hunt(0) {
			t.Fatalf("hunted with karma %v, at 0 %v: want no, then yes", hunt(240), hunt(0))
		}
		challenge := func(karma int32) string {
			b := findBrain(t, nil)
			b.world.Self.Karma = karma
			return challenged(t, b, at(time.Second))
		}
		if challenge(240) != "" || challenge(0) != foeName {
			t.Fatalf("challenged %q with karma, %q at 0: want none, then %q", challenge(240), challenge(0), foeName)
		}
		b := findBrain(t, nil)
		b.world.Self.Karma = 240
		ask(t, b, time.Second)
		if answered, yes := duelAnswer(b, at(time.Second)); !answered || yes {
			t.Fatalf("answered %v yes %v: want declined at once", answered, yes)
		}
		arena := func(karma int32) bool {
			b := huntBrain(t)
			b.cfg.Travel.zones.Arenas = []datapack.Zone{box(5000, 5000, 6000, 6000)}
			b.spot.arrived = true
			b.pvp.arena.due = t0
			b.world.Self.Karma = karma
			return b.arenaReady(at(time.Second))
		}
		if arena(240) || !arena(0) {
			t.Fatalf("arena trip ready with karma %v, at 0 %v: want no, then yes", arena(240), arena(0))
		}
		b = huntBrain(t)
		arenaTripAt(b, box(5000, 5000, 6000, 6000))
		b.world.Self.Karma = 240
		b.decide(at(time.Second))
		if b.pvp.arena.active() {
			t.Fatal("an arena trip goes on with karma")
		}
	})
}

// TestPvPKarmaRoute checks the karma route (docs/pvp/karma.md "With
// karma"): on foot, every segment karmaClear from the town polygon and the
// guard spawns; with no such route the walk fails, and the spot it failed
// for is the bot's again at karma 0.
func TestPvPKarmaRoute(t *testing.T) {
	from, dest := Point{X: 1000, Y: 2000}, Point{X: 7000, Y: 2000}
	town := box(3000, 1500, 4000, 2500)
	guards := []Point{{X: 3500, Y: 500}}
	route := func(t *testing.T, karma int32, guards []Point, dest Point) (*Brain, []Point, walkStatus) {
		b, _, _ := newNavBrain(t, testNav(t), Persona{}, from)
		b.cfg.Travel = &TravelMap{guards: guards}
		b.cfg.Travel.zones.Towns = []datapack.Zone{town}
		b.world.Self.Karma = karma
		a, st := walkStep(t, b, dest, t0)
		if st != walkWalking {
			return b, nil, st
		}
		return b, append([]Point{moveOrigin(a.payload), moveTarget(a.payload)}, routePoints(b)...), st
	}
	t.Run("karma 0: straight through the town", func(t *testing.T) {
		_, pts, st := route(t, 0, guards, dest)
		if st != walkWalking || clearOf(pts, town, guards) >= karmaClear {
			t.Fatalf("status %v, route %v: want the straight walk through the town", st, pts)
		}
	})
	t.Run("karma 240: walking only, every segment 600 from the town and the guards", func(t *testing.T) {
		b, pts, st := route(t, 240, guards, dest)
		if st != walkWalking {
			t.Fatalf("status %v (%s), want a walk", st, b.walk.reason)
		}
		if d := clearOf(pts, town, guards); d < karmaClear {
			t.Fatalf("route %v passes %.0f from the town or a guard, want at least %d", pts, d, karmaClear)
		}
	})
	t.Run("a gatekeeper hop shorter than the walk: walking only, the gatekeeper untouched", func(t *testing.T) {
		for _, karma := range []int32{0, 240} {
			b := newTravelBrain(t, 5000)
			b.world.Self.Karma = karma
			b.decide(at(time.Second))
			planned := b.trip.v.active()
			rule, a := b.decide(at(2 * time.Second))
			if karma == 0 {
				if !planned || rule != "travel" || a.kind != actMove {
					t.Fatalf("karma 0: trip %v, rule %q action %v: want the leg to the gatekeeper", planned, rule, a.kind)
				}
				continue
			}
			if planned || rule == "travel" {
				t.Fatalf("karma %d: trip %v, rule %q: want no gatekeeper trip", karma, planned, rule)
			}
			// The gatekeeper in sight: no Action, talk or bypass on it.
			b.world.Self.move = stopAt(Point{X: 1900})
			b.handleOne(t, NPCInfo{ObjectID: gkObj, NPCID: townGK, Pos: Point{X: 2000}}, at(3*time.Second))
			for i := range 3 {
				d := at(time.Duration(4+i) * time.Second)
				if rule, a := b.decide(d); a.object == gkObj || a.kind == actTalk || a.kind == actBypass {
					t.Fatalf("karma %d: rule %q action %v on %d, want the gatekeeper untouched", karma, rule, a.kind, a.object)
				}
			}
		}
	})
	t.Run("no such route: the walk fails", func(t *testing.T) {
		mid := Point{X: 5500, Y: 2000}
		var ring []Point
		for i := range 16 {
			ring = append(ring, offset(mid, float64(i)*2*3.141592653589793/16, 1000))
		}
		b, _, st := route(t, 240, ring, mid)
		if st != walkUnreachable {
			t.Fatalf("status %v, want unreachable inside a ring of guards", st)
		}
		_, _, st = route(t, 0, ring, mid)
		if st != walkWalking {
			t.Fatalf("status %v at karma 0, want the walk", st)
		}
		// The spot it failed for is avoided while the bot has karma; at
		// karma 0 it is its own again.
		b.spot.idx = 3
		b.failedWalk(at(time.Second))
		if _, ok := b.spot.avoid[3]; !ok {
			t.Fatal("the failed spot is not avoided")
		}
		b.karmaTick(at(time.Second))
		b.world.Self.Karma = 0
		b.karmaTick(at(time.Second))
		if _, ok := b.spot.avoid[3]; ok {
			t.Fatal("the failed spot is still avoided at karma 0")
		}
	})
}

// routePoints are the points of b's walk route after its first segment.
func routePoints(b *Brain) []Point {
	var pts []Point
	for _, l := range b.walk.route {
		pts = append(pts, Point{X: int32(l.X), Y: int32(l.Y), Z: int32(l.Z)})
	}
	return pts
}

// clearOf is the least distance from the polyline pts to the town and the
// guards, sampled every 2 units.
func clearOf(pts []Point, town datapack.Zone, guards []Point) float64 {
	d := 1e18
	for i := 1; i < len(pts); i++ {
		a, c := pts[i-1], pts[i]
		n := int(dist3(a, c)/2) + 1
		for j := 0; j <= n; j++ {
			f := float64(j) / float64(n)
			p := Point{X: a.X + int32(float64(c.X-a.X)*f), Y: a.Y + int32(float64(c.Y-a.Y)*f)}
			d = min(d, zoneDist(town, p))
			for _, g := range guards {
				d = min(d, dist3(Point{X: p.X, Y: p.Y}, Point{X: g.X, Y: g.Y}))
			}
		}
	}
	return d
}

// TestPvPAvoidAfterDeath checks the avoid list (docs/pvp/karma.md "By
// fight"): killed by a player in the open world, the bot neither hunts nor
// challenges it for avoidAfterDeath, and still fights it back.
func TestPvPAvoidAfterDeath(t *testing.T) {
	killed := func(t *testing.T, b *Brain) {
		t.Helper()
		held := b.spot.held
		b.spot.held = false // no spot registry to give it back to
		b.handleOne(t, hitSelf(pcID, 50), t0)
		b.handleOne(t, Die{ObjectID: selfID}, at(time.Second))
		b.spot.held = held
		b.handleOne(t, Revive{ObjectID: selfID}, at(2*time.Second))
		b.world.Self.InCombat = false
		clear(b.world.Attackers)
	}
	t.Run("not hunted for 30 minutes", func(t *testing.T) {
		b := huntBrain(t)
		b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, PvPFlag: 1}, t0)
		killed(t, b)
		if fightsWith(b, "hunt", pcID, at(time.Minute)) {
			t.Fatal("hunted its killer within 30 minutes")
		}
		if !fightsWith(b, "hunt", pcID, at(time.Second+avoidAfterDeath)) {
			t.Fatal("not hunted once 30 minutes passed")
		}
	})
	t.Run("still fought back", func(t *testing.T) {
		b := huntBrain(t)
		b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, PvPFlag: 1}, t0)
		killed(t, b)
		b.handleOne(t, hitSelf(pcID, 50), at(time.Minute))
		b.world.Self.InCombat = true
		if !fightsWith(b, "attacked", pcID, at(time.Minute+time.Second)) {
			t.Fatal("its killer not fought back")
		}
	})
	t.Run("not challenged for 30 minutes", func(t *testing.T) {
		b := findBrain(t, nil)
		if b.challengeFoe(at(time.Second)) != pcID {
			t.Fatal("the candidate is not challenged before the death")
		}
		killed(t, b)
		if b.challengeFoe(at(time.Minute)) != 0 {
			t.Fatal("challenged its killer within 30 minutes")
		}
		if b.challengeFoe(at(time.Second+avoidAfterDeath)) != pcID {
			t.Fatal("not challenged once 30 minutes passed")
		}
	})
	t.Run("a monster's kill avoids nobody, nor an arena death", func(t *testing.T) {
		b := huntBrain(t)
		b.spot.held = false
		b.handleOne(t, Die{ObjectID: selfID}, at(time.Second))
		if len(b.karma.avoid) != 0 {
			t.Fatalf("avoid %v after a death with no player attacker", b.karma.avoid)
		}
		b = huntBrain(t)
		b.spot.held = false
		b.cfg.Travel.zones.Arenas = []datapack.Zone{box(-500, -500, 500, 500)}
		b.handleOne(t, hitSelf(pcID, 50), t0)
		b.handleOne(t, Die{ObjectID: selfID}, at(time.Second))
		if b.avoided(foeName, at(time.Minute)) {
			t.Fatal("an arena killer is avoided")
		}
	})
}

// karmaRule reports whether the playRules row named name is one a bot with
// karma skips.
func karmaRule(name string) bool {
	return slices.ContainsFunc(playRules, func(r rule) bool { return r.name == name && r.karma })
}
