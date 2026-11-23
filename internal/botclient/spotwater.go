package botclient

import (
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// DrySpots returns spots without those whose centre snaps to a wet cell
// (nav.Planner.WetAt), and how many it dropped (#275). A walk to a place in
// water ends unreachable, as the planner swims only after a leader, and a bot
// left in water drowns once its breath runs out (docs/navigation.md,
// "Swimming"): no farmer can hold such a spot, so dropping it up front spares
// every farmer the walk there and the search it costs. A spot whose centre
// has no geodata is kept, as the walker judges it. A nil n drops none.
func (n *Nav) DrySpots(spots []datapack.Spot) ([]datapack.Spot, int) {
	if n == nil || n.Water == nil {
		return spots, 0
	}
	p := nav.Planner{Engine: n.Engine, Water: n.Water}
	dry := make([]datapack.Spot, 0, len(spots))
	for _, sp := range spots {
		if !p.WetAt(toLoc(Point{X: sp.Center.X, Y: sp.Center.Y, Z: sp.Z})) {
			dry = append(dry, sp)
		}
	}
	return dry, len(spots) - len(dry)
}
