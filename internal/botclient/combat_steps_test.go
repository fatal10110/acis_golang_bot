package botclient

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestCheckedSteps sends the combat side step, the disengage step and the
// idle turn step only where they hold on the geodata both ways (D13): next to
// the ledge or the pit, whatever side or heading the bot's rng picks first,
// a step never goes off the edge, and in the pit, where every side step would
// leave it, nothing goes out.
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
		// wantZ is the floor every step sent must end on; none means
		// nothing is sent.
		wantZ int32
		none  bool
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
			name: "side step in the pit, both sides out of it: nothing",
			self: scenePoint(geotest.PitInside, 0, 0), mob: scenePoint(geotest.PitInside, 0, -5),
			step: sideStepOn, none: true,
		},
		{
			name: "side step in the corridor, walls on both sides: nothing",
			self: scenePoint(geotest.CorridorWest, 10, 0), mob: scenePoint(geotest.CorridorWest, 15, 0),
			step: sideStepOn, none: true,
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
				if tt.none {
					if a.kind != actHold || a.payload != nil {
						t.Fatalf("seed %d: kind %v, want nothing sent", seed, a.kind)
					}
					continue
				}
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
				if tt.dist != 0 {
					if d := math.Hypot(float64(to.X-tt.self.X), float64(to.Y-tt.self.Y)); math.Abs(d-float64(tt.dist)) > 1.5 {
						t.Errorf("seed %d: step to %+v is %.1f long, want %d", seed, to, d, tt.dist)
					}
				}
			}
			if !tt.none && moves == 0 {
				t.Error("no step sent for any seed")
			}
		})
	}
}
