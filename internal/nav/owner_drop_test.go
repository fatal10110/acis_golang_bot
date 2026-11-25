package nav

import (
	"math"
	"os"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// TestOwnerOrcWallDrops is the offline probe of #504: the place routes from
// the Orc village town point to every spot within 20000 of it, each with its
// largest single-cell descent. It runs on the owner's data only
// (ownerNav's variables, and BOT_NAV_DATAPACK for the spots), and fails
// when a place route drops past dropBound.
func TestOwnerOrcWallDrops(t *testing.T) {
	xmlDir := os.Getenv("BOT_NAV_DATAPACK")
	if xmlDir == "" {
		t.Skip("BOT_NAV_DATAPACK not set: the owner-geodata probe runs locally only")
	}
	e, g, w := ownerNav(t)
	dp, err := datapack.Load(xmlDir)
	if err != nil {
		t.Fatal(err)
	}
	town := location.Location{X: -45264, Y: -112512, Z: -240}
	h := NewHierarchy(g, e, w, 1<<30)
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 400000, Water: w}
	routed, over := 0, 0
	for _, s := range dp.Spots() {
		to := location.Location{X: int(s.Center.X), Y: int(s.Center.Y), Z: int(s.Z)}
		if math.Hypot(float64(to.X-town.X), float64(to.Y-town.Y)) > 20000 {
			continue
		}
		r, err := h.Route(p, town, to, nil)
		if err != nil {
			t.Logf("%s %v: %v", s.Maker, to, err)
			continue
		}
		routed++
		d, at := routeDescent(e, r)
		if d > dropBound {
			over++
		}
		t.Logf("%s %v: worst descent %d at %v; route %v", s.Maker, to, d, at.World(), r)
	}
	t.Logf("%d routed, %d with a descent past %d", routed, over, dropBound)
	if over > 0 {
		t.Errorf("%d place routes drop past %d", over, dropBound)
	}
}
