package botclient

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestCheckedSteps sends the combat side step, the disengage step and the
// idle turn step only where they hold on the geodata both ways (D13): next to
// the ledge or the pit, whatever side or heading the bot's rng picks first,
// a step never goes off the edge, and in the pit, where every side step would
// leave it, the bot steps nowhere: a move to where it stands, which still
// drops the attack.
func TestCheckedSteps(t *testing.T) {
	n := testNav(t)
	sideStepOn := func(b *Brain) Action { return b.sideStep(engageMob, t0) }
	disengageOn := func(b *Brain) Action { return b.disengage(t0) }
	idleOn := func(b *Brain) Action {
		a, ok := b.ruleIdle(t0)
		if !ok {
			t.Fatal("idle rule passed with IdleChance 1")
		}
		return a
	}
	tests := []struct {
		name string
		self Point
		// mob is the target the side step crosses the line to, north of the
		// bot so the step goes east or west.
		mob     Point
		persona Persona
		step    func(b *Brain) Action
		// dist is the step's length; 0 skips the length check (the idle
		// rule may also emote or sit).
		dist int32
		// wantZ is the floor every step sent must end on; stay means the
		// step goes to where the bot stands.
		wantZ int32
		stay  bool
	}{
		{
			name: "side step on the plateau two cells from its edge stays on it",
			self: scenePoint(geotest.LedgeHigh, -3, 0), mob: scenePoint(geotest.LedgeHigh, -3, -5),
			step: sideStepOn, dist: sideStep, wantZ: geotest.Plateau,
		},
		{
			name: "side step five cells west of the pit does not jump in",
			self: scenePoint(geotest.PitOutside, 5, 0), mob: scenePoint(geotest.PitOutside, 5, -5),
			step: sideStepOn, dist: sideStep, wantZ: geotest.Ground,
		},
		{
			name: "side step in the pit, both sides out of it: stays",
			self: scenePoint(geotest.PitInside, 0, 0), mob: scenePoint(geotest.PitInside, 0, -5),
			step: sideStepOn, wantZ: geotest.PitDown, stay: true,
		},
		{
			name: "side step in the corridor, walls on both sides: stays",
			self: scenePoint(geotest.CorridorWest, 10, 0), mob: scenePoint(geotest.CorridorWest, 15, 0),
			step: sideStepOn, wantZ: geotest.Ground, stay: true,
		},
		{
			name: "disengage on the plateau's edge stays on it",
			self: scenePoint(geotest.LedgeHigh, -5, 0),
			step: disengageOn, dist: turnStep, wantZ: geotest.Plateau,
		},
		{
			name: "disengage next to the pit does not jump in",
			self: scenePoint(geotest.PitOutside, 9, 0),
			step: disengageOn, dist: turnStep, wantZ: geotest.Ground,
		},
		{
			name: "disengage in the pit steps within it",
			self: scenePoint(geotest.PitInside, 0, 0),
			step: disengageOn, dist: turnStep, wantZ: geotest.PitDown,
		},
		{
			name:    "idle turn on the plateau's edge stays on it",
			self:    scenePoint(geotest.LedgeHigh, -5, 0),
			persona: Persona{IdleChance: 1},
			step:    idleOn, wantZ: geotest.Plateau,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			moves := 0
			for seed := range uint64(32) {
				b, _, _ := newNavBrain(t, n, tt.persona, tt.self)
				b.rng = rand.New(rand.NewPCG(seed, seed))
				if tt.mob != (Point{}) {
					b.world.Apply(NPCInfo{ObjectID: engageMob, NPCID: 20001, Attackable: true, Pos: tt.mob}, t0)
				}
				a := tt.step(b)
				if a.kind != actMove {
					if tt.dist != 0 {
						t.Fatalf("seed %d: kind %v, want a move", seed, a.kind)
					}
					continue
				}
				moves++
				to := moveTarget(a.payload)
				c, ok := b.walk.plan.Locate(toLoc(to))
				if !ok {
					t.Fatalf("seed %d: step to %+v off the geodata", seed, to)
				}
				if int32(c.Z) != tt.wantZ {
					t.Errorf("seed %d: step to %+v lands at Z %d, want %d", seed, to, c.Z, tt.wantZ)
				}
				if tt.stay {
					if to.X != tt.self.X || to.Y != tt.self.Y {
						t.Errorf("seed %d: step to %+v, want where the bot stands %+v", seed, to, tt.self)
					}
				} else if tt.dist != 0 {
					if d := math.Hypot(float64(to.X-tt.self.X), float64(to.Y-tt.self.Y)); math.Abs(d-float64(tt.dist)) > 1.5 {
						t.Errorf("seed %d: step to %+v is %.1f long, want %d", seed, to, d, tt.dist)
					}
				}
			}
			if moves == 0 {
				t.Error("no step sent for any seed")
			}
		})
	}
}

// TestDisengageWalledIn drops the attack with a move to where the bot stands
// when no disengage heading holds: in a one-cell closet every step runs into
// the wall around it, and the bot still sends the move that ends the attack.
func TestDisengageWalledIn(t *testing.T) {
	closet := geotest.Point{CellX: 42, CellY: 42, Z: geotest.Ground}
	terrain := (&geotest.Terrain{}).Raise(40, 40, 44, 44, geotest.Wall).Raise(42, 42, 42, 42, geotest.Ground)
	n, err := buildTestNav(terrain, navWindow(0, 0, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	self := scenePoint(closet, 0, 0)
	for seed := range uint64(8) {
		b, _, _ := newNavBrain(t, n, Persona{}, self)
		b.rng = rand.New(rand.NewPCG(seed, seed))
		if a := b.turnAround(b.rng.Float64()*2*math.Pi, t0); a.kind != actHold {
			t.Fatalf("seed %d: turn step kind %v, want none to hold in the closet", seed, a.kind)
		}
		a := b.disengage(t0)
		if a.kind != actMove {
			t.Fatalf("seed %d: kind %v, want a move", seed, a.kind)
		}
		if to := moveTarget(a.payload); to.X != self.X || to.Y != self.Y || to.Z != geotest.Ground {
			t.Errorf("seed %d: disengage to %+v, want where the bot stands %+v", seed, to, self)
		}
	}
}

// TestSideStepWalledInBlacklists keeps a stuck attack counting where no side
// step holds: in the corridor, walls on both sides, each stuck attack ends in
// a move to where the bot stands, the next decision attacks again, and after
// BlacklistAfter attempts the target is blacklisted instead of the bot
// holding on it forever.
func TestSideStepWalledInBlacklists(t *testing.T) {
	p := Persona{BlacklistAfter: 4, BlacklistFor: time.Minute}
	b, _, _ := newNavBrain(t, testNav(t), p, scenePoint(geotest.CorridorWest, 10, 0))
	b.world.Apply(NPCInfo{ObjectID: engageMob, NPCID: 20001, Attackable: true, Pos: scenePoint(geotest.CorridorWest, 15, 0)}, t0)
	b.world.Target = engageMob
	attacks, moves := 0, 0
	now := t0
	for range 4 * p.BlacklistAfter {
		if !b.mayFight(engageMob, now) {
			break
		}
		a := b.fight(engageMob, now)
		switch a.kind {
		case actAttack:
			attacks++
		case actMove:
			moves++
		case actHold:
		default:
			t.Fatalf("fight sent kind %v, want an attack, a move or nothing", a.kind)
		}
		if a.kind != actHold {
			if sent, err := b.send(a, now); err != nil || !sent {
				t.Fatalf("send %v: sent %v, err %v", a.kind, sent, err)
			}
		}
		now = now.Add(stuckAfter)
	}
	if b.mayFight(engageMob, now) {
		t.Fatalf("target still fought after %d attacks and %d stays, want it blacklisted", attacks, moves)
	}
	if attacks != p.BlacklistAfter || moves != p.BlacklistAfter {
		t.Errorf("attacks %d, stays %d, want %d each", attacks, moves, p.BlacklistAfter)
	}
}
