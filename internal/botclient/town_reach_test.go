package botclient

import (
	"testing"
	"time"
)

// TestPlanHopFromVillageMerchant checks that the town persona's default
// reach takes a bot standing at a village's merchant to that village's
// gatekeeper: on Talking Island the grocer Katerina (30004) spawns at
// (-84204,240403) and the gatekeeper Roxxy (30006) at (-84108,244604)
// (spawnlist/17_25.xml), 4202 apart, and a bot recalled to the grocer
// never hopped (#441).
func TestPlanHopFromVillageMerchant(t *testing.T) {
	due := at(2 * time.Minute)
	b := newTownBrain(t, 50000)
	b.persona.Town.Reach = TownPersona().Town.Reach
	gk := b.cfg.Travel.gatekeepers[0].at
	b.world.Self.move = stopAt(Point{X: gk.X - 96, Y: gk.Y - 4201, Z: gk.Z})
	b.trip.hopAt = due
	if !b.planHop(due) || b.trip.gk != 0 {
		t.Fatalf("hop planned %v from gatekeeper %d, want one from gatekeeper 0 (reach %d)", b.trip.v.active(), b.trip.gk, b.persona.Town.Reach)
	}
}
