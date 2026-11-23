package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// sceneFollower is a started brain walking the synthetic scenes, its bot
// standing at bot, running, in gmID's party; the GM stands at leader facing
// +X (behind it is -X). The slot is drawn 120 units out, the re-path goes
// out as soon as it is due.
func sceneFollower(t *testing.T, bot, leader Point) *Brain {
	t.Helper()
	p := Persona{Follow: FollowPolicy{MinDist: 120, MaxDist: 120, Repath: 250}}
	b, _, _ := newNavBrain(t, testNav(t), p, bot)
	b.world.Self.Running = true
	b.world.Apply(PartySmallWindowAll{LeaderID: gmID, Members: []PartyMember{gmRow}}, t0)
	b.world.Apply(CharInfo{ObjectID: gmID, Name: "GM", Pos: leader, RunSpeed: 100, WalkSpeed: 50, Running: true}, t0)
	return b
}

// walkRule runs rule decisions from now, each through decide (wantRule) or
// a call of its own, sending each walker segment and answering it as the
// server answers an open click, until a decision holds with the walk no
// longer under way (walking reports it). Every segment must pass CanMove. It
// returns the segment ends and the time after the last.
func walkRule(t *testing.T, b *Brain, now time.Time, decide func(time.Time) Action, walking func() bool) ([]Point, time.Time) {
	t.Helper()
	n := b.walk.nav
	var ends []Point
	for i := range 80 {
		a := decide(now)
		switch a.kind {
		case actHold:
			if !walking() {
				return ends, now
			}
			now = now.Add(200 * time.Millisecond)
			continue
		case actMove:
		default:
			t.Fatalf("decision %d: kind %d, want a segment or a hold", i, a.kind)
		}
		origin, to := moveOrigin(a.payload), moveTarget(a.payload)
		if !a.walk || !n.Engine.CanMove(int(origin.X), int(origin.Y), int(origin.Z), int(to.X), int(to.Y), int(to.Z)) {
			t.Fatalf("segment %d %+v -> %+v: walker %v, want a walker segment passing CanMove", len(ends), origin, to, a.walk)
		}
		if ok, err := b.send(a, now); !ok || err != nil {
			t.Fatalf("send segment %d: %v, %v", len(ends), ok, err)
		}
		b.handle([]Event{segAnswer(origin, to)}, now)
		ends = append(ends, to)
		now = now.Add(segRun(b, now) + time.Millisecond)
	}
	t.Fatal("the walk did not end after 80 decisions")
	return nil, now
}

// followDecide is one decision that must be rule "follow"'s.
func followDecide(t *testing.T, b *Brain) func(time.Time) Action {
	return func(now time.Time) Action {
		t.Helper()
		name, a := b.decide(now)
		if name != "follow" {
			t.Fatalf("decide at %v = %q, want follow", now.Sub(t0), name)
		}
		return a
	}
}

// TestFollowRoutes follows a leader across the synthetic scenes: the slot is
// snapped into the leader's SCC and walked as checked segments, around a
// wall, down a one-way ledge toward the leader, and never up a ledge the bot
// cannot climb or into a pit.
func TestFollowRoutes(t *testing.T) {
	baseEast := scenePoint(geotest.Point{CellX: 543, CellY: 285}, 0, 0).X
	tests := []struct {
		name        string
		bot, leader Point
		// arrive is set when the bot must reach its slot, check what the
		// slot and the route must hold.
		arrive bool
		check  func(t *testing.T, slot Point, ends []Point)
	}{
		{
			name: "slot across a wall routes around it",
			bot:  scenePoint(geotest.UInside, 0, 0), leader: scenePoint(geotest.UBehind, 0, 0),
			arrive: true,
			check: func(t *testing.T, slot Point, ends []Point) {
				if slot.X <= baseEast {
					t.Errorf("slot %+v, want past the U's base (x > %d), on the leader's side", slot, baseEast)
				}
				if len(ends) < 2 {
					t.Errorf("%d segments, want the route around the U", len(ends))
				}
			},
		},
		{
			name: "leader below a ledge is followed down",
			bot:  scenePoint(geotest.LedgeHigh, 0, 0), leader: scenePoint(geotest.LedgeLow, 0, 0),
			arrive: true,
			check: func(t *testing.T, slot Point, ends []Point) {
				if last := ends[len(ends)-1]; slot.Z != geotest.Ground || last.Z != geotest.Ground {
					t.Errorf("slot %+v, last segment end %+v, want both on the ground below", slot, last)
				}
			},
		},
		{
			name: "leader on a one-way-up ledge is not chased",
			bot:  scenePoint(geotest.LedgeLow, 0, 0), leader: scenePoint(geotest.LedgeHigh, 7, 0),
			check: func(t *testing.T, slot Point, _ []Point) {
				if slot.Z != geotest.Plateau {
					t.Errorf("slot %+v, want on the plateau with the leader", slot)
				}
			},
		},
		{
			name: "leader in a pit is not chased into it",
			bot:  scenePoint(geotest.PitOutside, 0, 0), leader: scenePoint(geotest.PitInside, 0, 0),
			check: func(t *testing.T, slot Point, _ []Point) {
				if slot.Z != geotest.PitDown {
					t.Errorf("slot %+v, want in the pit with the leader", slot)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := sceneFollower(t, tt.bot, tt.leader)
			ends, now := walkRule(t, b, at(time.Second), followDecide(t, b), func() bool { return b.follow.traveling })
			slot := b.follow.dest
			tt.check(t, slot, ends)
			if !tt.arrive {
				if len(ends) != 0 || !b.follow.stuck || b.walk.status != walkUnreachable {
					t.Fatalf("%d segments, stuck %v, walk %v (%s); want no move and the slot given up", len(ends), b.follow.stuck, b.walk.status, b.walk.reason)
				}
				// It waits for the leader to move on, not re-pathing at the
				// same slot.
				for range 5 {
					now = now.Add(time.Second)
					if a := followDecide(t, b)(now); a.kind != actHold || !b.follow.dueAt.IsZero() {
						t.Fatalf("kind %d, re-path at %v; want holding with the leader put", a.kind, b.follow.dueAt.Sub(t0))
					}
				}
				return
			}
			last := ends[len(ends)-1]
			if b.follow.stuck || horizontal2(last, slot) > followArrive*followArrive {
				t.Fatalf("ended at %+v (stuck %v, %s), want within %d of the slot %+v", last, b.follow.stuck, b.walk.reason, followArrive, slot)
			}
		})
	}
}

// TestFollowLedgeLeaderMovesOn: a slot given up on a ledge the bot cannot
// climb is taken again once the leader comes down and moves Repath on.
func TestFollowLedgeLeaderMovesOn(t *testing.T) {
	b := sceneFollower(t, scenePoint(geotest.LedgeLow, 0, 0), scenePoint(geotest.LedgeHigh, 7, 0))
	_, now := walkRule(t, b, at(time.Second), followDecide(t, b), func() bool { return b.follow.traveling })
	if !b.follow.stuck {
		t.Fatal("the slot on the plateau was not given up")
	}
	down := scenePoint(geotest.LedgeLow, -20, 0)
	b.world.Apply(StopMove{ObjectID: gmID, Pos: down}, now)
	ends, _ := walkRule(t, b, now.Add(time.Second), followDecide(t, b), func() bool { return b.follow.traveling })
	if len(ends) == 0 || b.follow.stuck || b.follow.dest.Z != geotest.Ground {
		t.Fatalf("%d segments, stuck %v, slot %+v; want the walk to the leader on the ground", len(ends), b.follow.stuck, b.follow.dest)
	}
}

// TestFleeRoutes flees from an attacker toward a pit: the point is on ground
// of the bot's SCC, past the pit, walked around it as checked segments.
func TestFleeRoutes(t *testing.T) {
	from := scenePoint(geotest.PitOutside, 0, 0)
	b, _, _ := newNavBrain(t, testNav(t), Persona{FleeDistance: 600}, from)
	b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: scenePoint(geotest.PitOutside, -10, 0), RunSpeed: 200, WalkSpeed: 50}, t0)
	// The run ends where the bot reaches the point it picked first: a flee
	// still asked for there runs on to a fresh one.
	var target Point
	walking := func() bool {
		return target == (Point{}) || horizontal2(b.world.Self.move.to, target) > walkSlop*walkSlop
	}
	flee := func(now time.Time) Action {
		if !walking() {
			return Action{}
		}
		a := b.flee(npcID, now)
		if target == (Point{}) {
			target = b.fleeing.to
		}
		return a
	}
	ends, _ := walkRule(t, b, at(time.Second), flee, walking)
	if len(ends) < 2 {
		t.Fatalf("%d segments, want the route around the pit", len(ends))
	}
	for i, e := range ends {
		if e.Z != geotest.Ground {
			t.Errorf("segment %d ends at %+v, want on the ground, never in the pit", i, e)
		}
	}
	if last := ends[len(ends)-1]; last.X <= from.X || 4*horizontal2(last, from) < 600*600 {
		t.Errorf("fled to %+v from %+v, want at least 300 away from the attacker's side", last, from)
	}
}

// TestIdleDriftInSCC draws idle drifts by a ledge: each walks to a point of
// the bot's SCC, never up onto the plateau it cannot climb.
func TestIdleDriftInSCC(t *testing.T) {
	n := testNav(t)
	from := scenePoint(geotest.LedgeLow, 0, 0)
	b, _, _ := newNavBrain(t, n, Persona{IdleChance: 1, IdleRadius: 400}, from)
	b.anchor = from
	w := &b.walk
	home := member(t, b, from)
	var drifts int
	for i := range 60 {
		b.walk = newWalker(n)
		a, ok := b.ruleIdle(at(time.Duration(i) * time.Second))
		if !ok || w.purpose != walkIdle {
			continue
		}
		drifts++
		if !n.Hier.SameSCC(home, member(t, b, w.dest)) || w.dest.Z != geotest.Ground {
			t.Fatalf("drift %d to %+v, want ground of the bot's SCC", i, w.dest)
		}
		if a.kind == actMove && !a.walk {
			t.Fatalf("drift %d: a move outside the walker", i)
		}
	}
	if drifts < 10 {
		t.Errorf("%d drifts in 60 draws, want about half", drifts)
	}
}

// member is the hierarchy member of the cell at p.
func member(t *testing.T, b *Brain, p Point) nav.Member {
	t.Helper()
	c, ok := b.walk.plan.Locate(toLoc(p))
	if !ok {
		t.Fatalf("%+v off the geodata", p)
	}
	m, err := b.walk.nav.Hier.Locate(c, nil)
	if err != nil {
		t.Fatalf("%+v: %v", p, err)
	}
	return m
}

// TestFollowOffOneWayGround: a follower on one-way-out ground (a deck it
// cannot climb back onto) whose leader stands in an abstract SCC drops into
// the nearest SCC first, as every walk does, rather than routing the whole
// way with the cell search alone; the next plan routes from there.
func TestFollowOffOneWayGround(t *testing.T) {
	n := testNav(t)
	from := scenePoint(geotest.BridgeWestDeck, 0, 0)
	b := sceneFollower(t, from, scenePoint(geotest.PitOutside, 0, 0))
	now := at(time.Second)
	a := followDecide(t, b)(now)
	if a.kind != actMove {
		t.Fatalf("kind %d (%s), want the first segment", a.kind, b.walk.reason)
	}
	end := moveTarget(a.payload)
	if r := b.walk.route; len(r) > 0 {
		end = r[len(r)-1]
	}
	c, ok := b.walk.plan.Locate(toLoc(end))
	if !ok {
		t.Fatalf("plan end %+v off the geodata", end)
	}
	if m, err := n.Hier.Locate(c, nil); err != nil || m.Kind() != nav.InSCC {
		t.Errorf("plan end %+v: kind %v err %v, want InSCC ground", end, m.Kind(), err)
	}
	if reach := int64(walkDropCells * 16); horizontal2(end, from) > reach*reach {
		t.Errorf("plan end %+v, %v from the deck; want the drop within %d, not the route to the slot %+v",
			end, horizontal2(end, from), reach, b.follow.dest)
	}
	// The whole walk, on a fresh brain, reaches the slot.
	b = sceneFollower(t, from, scenePoint(geotest.PitOutside, 0, 0))
	ends, _ := walkRule(t, b, now, followDecide(t, b), func() bool { return b.follow.traveling })
	if len(ends) == 0 {
		t.Fatalf("no segment (%s), want the walk to the slot", b.walk.reason)
	}
	last := ends[len(ends)-1]
	if b.follow.stuck || horizontal2(last, b.follow.dest) > followArrive*followArrive {
		t.Fatalf("ended at %+v (stuck %v, %s), want within %d of the slot %+v", last, b.follow.stuck, b.walk.reason, followArrive, b.follow.dest)
	}
}
